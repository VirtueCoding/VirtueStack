// Package tasks provides shared helper functions for task handlers.
// This file contains utility functions used across multiple handlers including
// password hashing, MAC address generation, and VM lifecycle helpers.
package tasks

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"unicode"

	"github.com/alexedwards/argon2id"
	crypt_sha512 "github.com/tredoe/osutil/user/crypt/sha512_crypt"

	sharedcrypto "github.com/AbuGosok/VirtueStack/internal/shared/crypto"
)

// generateMACAddress generates a MAC address from a VM ID.
// Uses crypto/sha256 of the VM ID to derive a deterministic, collision-resistant
// MAC address. The previous polynomial rolling hash could produce collisions for
// VM IDs that differ only in suffix characters.
func generateMACAddress(vmID string) string {
	// Try the shared crypto package first (produces a random MAC).
	// Fall back to a deterministic SHA-256-derived MAC when the VMID is known
	// and reproducibility is required.
	mac, err := sharedcrypto.GenerateMACAddress()
	if err == nil {
		return mac
	}

	// Fallback: derive MAC deterministically from the VM ID via SHA-256.
	h := sha256.Sum256([]byte(vmID))
	return fmt.Sprintf("%s:%02x:%02x:%02x", MACPrefix, h[0], h[1], h[2])
}

// hashPasswordParams holds the parameters for Argon2id password hashing.
// These parameters are tuned for security (memory=65536, iterations=3, parallelism=4).
var hashPasswordParams = &argon2id.Params{
	Memory:      65536, // 64MB
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

// hashPassword creates a secure password hash using Argon2id.
// Returns an empty string if the password is empty or fails validation.
func hashPassword(password string) (string, error) {
	if err := validatePasswordStrength(password); err != nil {
		return "", err
	}

	hash, err := argon2id.CreateHash(password, hashPasswordParams)
	if err != nil {
		return "", fmt.Errorf("creating password hash: %w", err)
	}
	return hash, nil
}

// hashPasswordForCloudInit creates a SHA-512 crypt hash suitable for cloud-init.
// Returns an empty string if the password is empty.
func hashPasswordForCloudInit(password string) (string, error) {
	if password == "" {
		return "", nil
	}

	if err := validatePasswordStrength(password); err != nil {
		return "", err
	}

	salt, err := generateShadowSalt(16)
	if err != nil {
		return "", fmt.Errorf("generating SHA-512 crypt salt: %w", err)
	}
	return sha512Crypt(password, salt, 5000), nil
}

// generateShadowSalt generates a random salt string for shadow password hashing.
func generateShadowSalt(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("salt length must be positive")
	}

	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789./"
	buf := make([]byte, length)

	// Use rejection sampling to avoid modulo bias regardless of alphabet length.
	// maxAcceptable is the largest byte value that won't cause bias when doing modulo.
	// For alphabet length 64: 256 % 64 = 0, so maxAcceptable = 256 (all values accepted).
	// This approach is correct even if the alphabet changes to a length that doesn't divide 256.
	maxAcceptable := 256 - 256%len(alphabet)

	for i := 0; i < length; i++ {
		for {
			var b byte
			if err := binary.Read(rand.Reader, binary.LittleEndian, &b); err != nil {
				return "", fmt.Errorf("reading random byte: %w", err)
			}
			if b < byte(maxAcceptable) {
				buf[i] = alphabet[int(b)%len(alphabet)]
				break
			}
			// Reject and retry - this is the rejection sampling loop
		}
	}

	return string(buf), nil
}

// sha512Crypt implements the SHA-512 crypt algorithm for password hashing.
// This is compatible with the $6$ format used in /etc/shadow.
// Uses the vetted github.com/tredoe/osutil/user/crypt/sha512_crypt library
// to ensure compliance with the SHA-512 crypt specification.
func sha512Crypt(password, salt string, rounds int) string {
	crypt := crypt_sha512.New()
	hash, err := crypt.Generate([]byte(password), []byte(fmt.Sprintf("$6$rounds=%d$%s$", rounds, salt)))
	if err != nil {
		// Fallback to empty hash on error (should never happen with valid inputs)
		return ""
	}
	return hash
}

// verifyPassword verifies a password against an Argon2id hash.
// Returns true if the password matches the hash.
func verifyPassword(password, hash string) (bool, error) {
	if password == "" || hash == "" {
		return false, fmt.Errorf("password and hash cannot be empty")
	}

	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, fmt.Errorf("comparing password: %w", err)
	}
	return match, nil
}

// stopVMGracefully attempts a graceful shutdown of a VM, falling back to a
// force-stop when the graceful attempt fails. The timeoutSeconds argument
// controls how long the node agent waits for the guest to shut down cleanly.
//
// This helper centralises the stop-then-force-stop pattern that is required in
// every task handler that must quiesce a VM before performing destructive
// operations (delete, resize, reinstall, revert, migrate).
//
// Return value: the first error that could not be recovered from, or nil if the
// VM was stopped (whether gracefully or by force).
func stopVMGracefully(ctx context.Context, nodeClient NodeAgentClient, nodeID, vmID string, timeoutSeconds int, logger *slog.Logger) error {
	if err := nodeClient.StopVM(ctx, nodeID, vmID, timeoutSeconds); err != nil {
		logger.Warn("graceful stop failed, attempting force stop", "vm_id", vmID, "error", err)
		if forceErr := nodeClient.ForceStopVM(ctx, nodeID, vmID); forceErr != nil {
			return fmt.Errorf("force stopping VM %s: %w", vmID, forceErr)
		}
	}
	return nil
}

// shortID returns the first 8 characters of id, or the full id if it is shorter than 8 characters.
func shortID(id string) string {
	if len(id) < 8 {
		return id
	}
	return id[:8]
}

// validatePasswordStrength validates that a password meets minimum security requirements.
// Minimum 12 characters with at least one uppercase, one lowercase, one digit, and one special character.
func validatePasswordStrength(password string) error {
	if len(password) < 12 {
		return fmt.Errorf("password must be at least 12 characters long")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}
	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special character")
	}

	return nil
}