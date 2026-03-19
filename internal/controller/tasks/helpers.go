// Package tasks provides shared helper functions for task handlers.
// This file contains utility functions used across multiple handlers including
// password hashing, MAC address generation, and VM lifecycle helpers.
package tasks

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"fmt"
	"log/slog"
	"unicode"

	"github.com/alexedwards/argon2id"
)

// generateMACAddress generates a MAC address from a VM ID.
// Uses a consistent algorithm to generate reproducible MAC addresses.
func generateMACAddress(vmID string) string {
	// Generate the last 3 octets from the VM ID hash
	// This is a simple deterministic approach
	hash := 0
	for _, c := range vmID {
		hash = hash*31 + int(c)
	}

	octet4 := (hash >> 16) & 0xFF
	octet5 := (hash >> 8) & 0xFF
	octet6 := hash & 0xFF

	return fmt.Sprintf("%s:%02x:%02x:%02x", MACPrefix, octet4, octet5, octet6)
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
	randBytes := make([]byte, length)
	if _, err := rand.Read(randBytes); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}

	for i, b := range randBytes {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}

	return string(buf), nil
}

// sha512Crypt implements the SHA-512 crypt algorithm for password hashing.
// This is compatible with the $6$ format used in /etc/shadow.
func sha512Crypt(password, salt string, rounds int) string {
	passBytes := []byte(password)
	saltBytes := []byte(salt)

	altCtx := sha512.New()
	altCtx.Write(passBytes)
	altCtx.Write(saltBytes)
	altCtx.Write(passBytes)
	altSum := altCtx.Sum(nil)

	ctx := sha512.New()
	ctx.Write(passBytes)
	ctx.Write(saltBytes)

	for i := len(passBytes); i > 0; i -= len(altSum) {
		n := len(altSum)
		if i < n {
			n = i
		}
		ctx.Write(altSum[:n])
	}

	for i := len(passBytes); i > 0; i >>= 1 {
		if i&1 != 0 {
			ctx.Write(altSum)
		} else {
			ctx.Write(passBytes)
		}
	}

	sum := ctx.Sum(nil)

	dpCtx := sha512.New()
	for i := 0; i < len(passBytes); i++ {
		dpCtx.Write(passBytes)
	}
	dpSum := dpCtx.Sum(nil)
	pSeq := repeatToLength(dpSum, len(passBytes))

	dsCtx := sha512.New()
	for i := 0; i < 16+int(sum[0]); i++ {
		dsCtx.Write(saltBytes)
	}
	dsSum := dsCtx.Sum(nil)
	sSeq := repeatToLength(dsSum, len(saltBytes))

	for i := 0; i < rounds; i++ {
		rCtx := sha512.New()

		if i&1 != 0 {
			rCtx.Write(pSeq)
		} else {
			rCtx.Write(sum)
		}

		if i%3 != 0 {
			rCtx.Write(sSeq)
		}

		if i%7 != 0 {
			rCtx.Write(pSeq)
		}

		if i&1 != 0 {
			rCtx.Write(sum)
		} else {
			rCtx.Write(pSeq)
		}

		sum = rCtx.Sum(nil)
	}

	return fmt.Sprintf("$6$rounds=%d$%s$%s", rounds, salt, sha512CryptEncode(sum))
}

// repeatToLength repeats a byte slice to reach the specified length.
func repeatToLength(src []byte, length int) []byte {
	out := make([]byte, 0, length)
	for len(out) < length {
		remaining := length - len(out)
		if remaining >= len(src) {
			out = append(out, src...)
		} else {
			out = append(out, src[:remaining]...)
		}
	}
	return out
}

// sha512CryptEncode encodes the SHA-512 hash to the crypt format.
func sha512CryptEncode(sum []byte) string {
	const alphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	encode24 := func(b2, b1, b0 byte, n int) string {
		v := uint32(b2)<<16 | uint32(b1)<<8 | uint32(b0)
		out := make([]byte, n)
		for i := 0; i < n; i++ {
			out[i] = alphabet[v&0x3f]
			v >>= 6
		}
		return string(out)
	}

	pairs := [][4]int{
		{0, 21, 42, 4},
		{22, 43, 1, 4},
		{44, 2, 23, 4},
		{3, 24, 45, 4},
		{25, 46, 4, 4},
		{47, 5, 26, 4},
		{6, 27, 48, 4},
		{28, 49, 7, 4},
		{50, 8, 29, 4},
		{9, 30, 51, 4},
		{31, 52, 10, 4},
		{53, 11, 32, 4},
		{12, 33, 54, 4},
		{34, 55, 13, 4},
		{56, 14, 35, 4},
		{15, 36, 57, 4},
		{37, 58, 16, 4},
		{59, 17, 38, 4},
		{18, 39, 60, 4},
		{40, 61, 19, 4},
		{62, 20, 41, 4},
	}

	out := ""
	for _, p := range pairs {
		out += encode24(sum[p[0]], sum[p[1]], sum[p[2]], p[3])
	}
	out += encode24(0, 0, sum[63], 2)
	return out
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