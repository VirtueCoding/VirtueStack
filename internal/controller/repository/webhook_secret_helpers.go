package repository

import (
	"fmt"
	"strings"

	sharedcrypto "github.com/AbuGosok/VirtueStack/internal/shared/crypto"
)

const webhookSecretEncryptedPrefix = "enc:"

func encryptWebhookSecret(secret, encryptionKey string) (string, error) {
	if secret == "" || encryptionKey == "" || strings.HasPrefix(secret, webhookSecretEncryptedPrefix) {
		return secret, nil
	}

	encrypted, err := sharedcrypto.Encrypt(secret, encryptionKey)
	if err != nil {
		return "", fmt.Errorf("encrypt webhook secret: %w", err)
	}
	return webhookSecretEncryptedPrefix + encrypted, nil
}

func decryptWebhookSecret(secret, encryptionKey string) (string, error) {
	if secret == "" || !strings.HasPrefix(secret, webhookSecretEncryptedPrefix) {
		return secret, nil
	}
	if encryptionKey == "" {
		return "", fmt.Errorf("decrypt webhook secret: missing encryption key")
	}

	decrypted, err := sharedcrypto.Decrypt(strings.TrimPrefix(secret, webhookSecretEncryptedPrefix), encryptionKey)
	if err != nil {
		return "", fmt.Errorf("decrypt webhook secret: %w", err)
	}
	return decrypted, nil
}
