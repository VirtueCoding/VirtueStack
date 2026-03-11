// Package crypto provides cryptographic utilities for VirtueStack.
// It includes AES-256-GCM encryption, random token generation, and hashing functions.
// All cryptographic operations use Go's stdlib crypto packages for security.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/google/uuid"
)

const (
	// AESKeySize is the required key size in bytes for AES-256 (32 bytes).
	AESKeySize = 32

	// GCMNonceSize is the size of the GCM nonce in bytes (12 bytes is standard for GCM).
	GCMNonceSize = 12

	// HexKeyLength is the expected length of a hex-encoded 256-bit key (64 characters).
	HexKeyLength = 64
)

// Encrypt encrypts plaintext using AES-256-GCM.
// The key must be a 64-character hex-encoded string (representing 32 bytes).
// Returns a base64-encoded string with the nonce prepended to the ciphertext.
//
// The output format is: base64(nonce || ciphertext || tag)
func Encrypt(plaintext string, hexKey string) (string, error) {
	// Decode the hex key to bytes
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return "", fmt.Errorf("decoding hex key: %w", err)
	}

	if len(key) != AESKeySize {
		return "", fmt.Errorf("invalid key size: expected %d bytes, got %d", AESKeySize, len(key))
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher block: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM mode: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, GCMNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	// Encrypt the plaintext
	// Seal appends the ciphertext to the nonce and includes the authentication tag
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode to base64 for safe transmission/storage
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts AES-256-GCM ciphertext.
// The key must be a 64-character hex-encoded string (representing 32 bytes).
// The ciphertext must be base64-encoded with the nonce prepended.
func Decrypt(ciphertext string, hexKey string) (string, error) {
	// Decode the hex key to bytes
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return "", fmt.Errorf("decoding hex key: %w", err)
	}

	if len(key) != AESKeySize {
		return "", fmt.Errorf("invalid key size: expected %d bytes, got %d", AESKeySize, len(key))
	}

	// Decode base64 ciphertext
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decoding base64 ciphertext: %w", err)
	}

	// Validate minimum length (nonce + at least 1 byte + tag)
	minLength := GCMNonceSize + 16 // GCM tag is 16 bytes
	if len(data) < minLength {
		return "", fmt.Errorf("ciphertext too short: expected at least %d bytes, got %d", minLength, len(data))
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher block: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM mode: %w", err)
	}

	// Extract nonce and actual ciphertext
	nonce := data[:GCMNonceSize]
	actualCiphertext := data[GCMNonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting data: %w", err)
	}

	return string(plaintext), nil
}

// GenerateRandomToken generates a cryptographically secure random token.
// Returns a hex-encoded string of the specified byte length.
// For example, GenerateRandomToken(32) returns a 64-character hex string.
func GenerateRandomToken(byteLength int) (string, error) {
	if byteLength <= 0 {
		return "", fmt.Errorf("byte length must be positive, got %d", byteLength)
	}

	bytes := make([]byte, byteLength)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}

	return hex.EncodeToString(bytes), nil
}

// GenerateRandomBytes generates cryptographically secure random bytes.
// Returns the raw bytes for use cases requiring non-hex encoding.
func GenerateRandomBytes(length int) ([]byte, error) {
	if length <= 0 {
		return nil, fmt.Errorf("length must be positive, got %d", length)
	}

	bytes := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return nil, fmt.Errorf("generating random bytes: %w", err)
	}

	return bytes, nil
}

// GenerateUUID generates a new UUID v4.
// Returns the UUID as a standard string format (e.g., "550e8400-e29b-41d4-a716-446655440000").
func GenerateUUID() string {
	return uuid.New().String()
}

// GenerateUUIDBytes returns a new UUID v4 as raw bytes (16 bytes).
func GenerateUUIDBytes() ([]byte, error) {
	u, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("generating UUID: %w", err)
	}
	return u[:], nil
}

// HashSHA256 returns the hex-encoded SHA-256 hash of the input string.
func HashSHA256(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

// HashSHA256Bytes returns the SHA-256 hash of the input as raw bytes.
func HashSHA256Bytes(input []byte) []byte {
	hash := sha256.Sum256(input)
	return hash[:]
}

// GenerateMACAddress generates a random MAC address with the locally-administered bit set.
// Format: 52:54:00:xx:xx:xx (QEMU/KVM range).
// The first three bytes (52:54:00) identify this as a QEMU/KVM virtual NIC.
func GenerateMACAddress() (string, error) {
	// Generate 3 random bytes for the last portion of the MAC
	randomBytes, err := GenerateRandomBytes(3)
	if err != nil {
		return "", fmt.Errorf("generating MAC address: %w", err)
	}

	// Construct MAC: 52:54:00:xx:xx:xx
	// 52:54:00 is the QEMU/KVM OUI prefix
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x",
		randomBytes[0], randomBytes[1], randomBytes[2]), nil
}

// ValidateEncryptionKey checks if a hex-encoded key is valid for AES-256-GCM.
// Returns an error if the key is not exactly 64 hex characters (32 bytes).
func ValidateEncryptionKey(hexKey string) error {
	if len(hexKey) != HexKeyLength {
		return fmt.Errorf("invalid key length: expected %d hex characters, got %d", HexKeyLength, len(hexKey))
	}

	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return fmt.Errorf("invalid hex encoding: %w", err)
	}

	if len(key) != AESKeySize {
		return fmt.Errorf("invalid key size: expected %d bytes, got %d", AESKeySize, len(key))
	}

	return nil
}

// GenerateEncryptionKey generates a new random 256-bit encryption key.
// Returns a hex-encoded string suitable for use with Encrypt/Decrypt.
func GenerateEncryptionKey() (string, error) {
	return GenerateRandomToken(AESKeySize)
}

// ConstantTimeCompare performs a constant-time comparison of two strings.
// This prevents timing attacks when comparing secrets.
func ConstantTimeCompare(a, b string) bool {
	return subtleConstantTimeCompare([]byte(a), []byte(b))
}

// subtleConstantTimeCompare performs a constant-time byte comparison.
func subtleConstantTimeCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	// Simple XOR-based comparison that avoids timing differences
	result := byte(0)
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}