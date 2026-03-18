package crypto

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key, err := GenerateEncryptionKey()
	require.NoError(t, err)

	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty string", ""},
		{"short text", "hello"},
		{"normal text", "This is a secret message"},
		{"long text", strings.Repeat("abcdefghij", 100)},
		{"unicode", "日本語テスト 🚀"},
		{"with newlines", "line1\nline2\nline3"},
		{"json payload", `{"user_id":"abc","role":"admin"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, err := Encrypt(tt.plaintext, key)
			require.NoError(t, err)
			assert.NotEmpty(t, ciphertext)

			decrypted, err := Decrypt(ciphertext, key)
			require.NoError(t, err)
			assert.Equal(t, tt.plaintext, decrypted)
		})
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1, err := GenerateEncryptionKey()
	require.NoError(t, err)
	key2, err := GenerateEncryptionKey()
	require.NoError(t, err)

	ciphertext, err := Encrypt("secret data", key1)
	require.NoError(t, err)

	_, err = Decrypt(ciphertext, key2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypting")
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key, err := GenerateEncryptionKey()
	require.NoError(t, err)

	ciphertext, err := Encrypt("secret data", key)
	require.NoError(t, err)

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	require.NoError(t, err)

	data[len(data)-5] ^= 0xFF
	tampered := base64.StdEncoding.EncodeToString(data)

	_, err = Decrypt(tampered, key)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypting")
}

func TestEncryptInvalidKey(t *testing.T) {
	tests := []struct {
		name    string
		hexKey  string
		wantErr bool
	}{
		{"too short", "aabbccdd", true},
		{"odd length", "aabbccdde", true},
		{"invalid hex", "gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Encrypt("plaintext", tt.hexKey)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDecryptInvalidCiphertext(t *testing.T) {
	key, err := GenerateEncryptionKey()
	require.NoError(t, err)

	tests := []struct {
		name       string
		ciphertext string
		wantErr    bool
	}{
		{"empty", "", true},
		{"invalid base64", "not-valid-base64!!!", true},
		{"too short ciphertext", "dG9vIHNob3J0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decrypt(tt.ciphertext, key)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateRandomToken(t *testing.T) {
	tests := []struct {
		name       string
		byteLength int
		wantLength int
	}{
		{"16 bytes -> 32 hex", 16, 32},
		{"32 bytes -> 64 hex", 32, 64},
		{"64 bytes -> 128 hex", 64, 128},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := GenerateRandomToken(tt.byteLength)
			require.NoError(t, err)
			assert.Len(t, token, tt.wantLength)
		})
	}

	t.Run("zero length returns error", func(t *testing.T) {
		_, err := GenerateRandomToken(0)
		require.Error(t, err)
	})

	t.Run("negative length returns error", func(t *testing.T) {
		_, err := GenerateRandomToken(-1)
		require.Error(t, err)
	})

	t.Run("uniqueness", func(t *testing.T) {
		seen := make(map[string]struct{}, 100)
		for i := 0; i < 100; i++ {
			token, err := GenerateRandomToken(32)
			require.NoError(t, err)
			_, exists := seen[token]
			assert.False(t, exists, "generated duplicate token")
			seen[token] = struct{}{}
		}
	})
}

func TestGenerateRandomString(t *testing.T) {
	t.Run("correct length", func(t *testing.T) {
		s := GenerateRandomString(32)
		assert.Len(t, s, 64)
	})
}

func TestGenerateRandomBytes(t *testing.T) {
	t.Run("correct length", func(t *testing.T) {
		b, err := GenerateRandomBytes(16)
		require.NoError(t, err)
		assert.Len(t, b, 16)
	})

	t.Run("zero length returns error", func(t *testing.T) {
		_, err := GenerateRandomBytes(0)
		require.Error(t, err)
	})
}

func TestGenerateEncryptionKey(t *testing.T) {
	key, err := GenerateEncryptionKey()
	require.NoError(t, err)
	assert.Len(t, key, HexKeyLength)

	err = ValidateEncryptionKey(key)
	require.NoError(t, err)
}

func TestValidateEncryptionKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid 64-char hex", strings.Repeat("ab", 32), false},
		{"too short", strings.Repeat("ab", 10), true},
		{"invalid hex", strings.Repeat("gg", 32), true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEncryptionKey(tt.key)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConstantTimeCompare(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"equal", "secret", "secret", true},
		{"not equal", "secret", "secref", false},
		{"different lengths", "short", "this-is-much-longer", false},
		{"empty both", "", "", true},
		{"one empty", "", "nonempty", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConstantTimeCompare(tt.a, tt.b)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestHashSHA256(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		h1 := HashSHA256("hello")
		h2 := HashSHA256("hello")
		assert.Equal(t, h1, h2)
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		h1 := HashSHA256("hello")
		h2 := HashSHA256("world")
		assert.NotEqual(t, h1, h2)
	})

	t.Run("correct length", func(t *testing.T) {
		h := HashSHA256("test")
		assert.Len(t, h, 64)
	})
}

func TestGenerateMACAddress(t *testing.T) {
	t.Run("valid format", func(t *testing.T) {
		mac, err := GenerateMACAddress()
		require.NoError(t, err)
		assert.True(t, len(mac) == 17, "MAC should be 17 chars: %s", mac)
		assert.True(t, strings.HasPrefix(mac, "52:54:00"), "MAC should start with QEMU OUI: %s", mac)
	})

	t.Run("uniqueness", func(t *testing.T) {
		m1, _ := GenerateMACAddress()
		m2, _ := GenerateMACAddress()
		assert.NotEqual(t, m1, m2)
	})
}

func TestGenerateHMACSignature(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		sig1 := GenerateHMACSignature("secret", []byte("payload"))
		sig2 := GenerateHMACSignature("secret", []byte("payload"))
		assert.Equal(t, sig1, sig2)
	})

	t.Run("different secrets produce different sigs", func(t *testing.T) {
		sig1 := GenerateHMACSignature("key1", []byte("payload"))
		sig2 := GenerateHMACSignature("key2", []byte("payload"))
		assert.NotEqual(t, sig1, sig2)
	})
}

func TestGenerateUUID(t *testing.T) {
	t.Run("non-empty", func(t *testing.T) {
		u := GenerateUUID()
		assert.NotEmpty(t, u)
	})

	t.Run("unique", func(t *testing.T) {
		u1 := GenerateUUID()
		u2 := GenerateUUID()
		assert.NotEqual(t, u1, u2)
	})
}

func TestGenerateRandomDigits(t *testing.T) {
	t.Run("correct length", func(t *testing.T) {
		d, err := GenerateRandomDigits(8)
		assert.NoError(t, err)
		assert.Len(t, d, 8)
		for _, c := range d {
			assert.True(t, c >= '0' && c <= '9', "expected digit, got %c", c)
		}
	})
}
