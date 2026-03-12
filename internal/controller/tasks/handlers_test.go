package tasks

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid strong password",
			password: "MyStr0ng!Pass",
			wantErr:  false,
		},
		{
			name:        "too short",
			password:    "Short1!",
			wantErr:     true,
			errContains: "at least 8 characters",
		},
		{
			name:        "missing uppercase",
			password:    "lowercase1!",
			wantErr:     true,
			errContains: "uppercase",
		},
		{
			name:        "missing lowercase",
			password:    "UPPERCASE1!",
			wantErr:     true,
			errContains: "lowercase",
		},
		{
			name:        "missing digit",
			password:    "NoDigits!",
			wantErr:     true,
			errContains: "digit",
		},
		{
			name:        "missing special character",
			password:    "NoSpecial1",
			wantErr:     true,
			errContains: "special character",
		},
		{
			name:     "empty password",
			password: "",
			wantErr:  false, // Returns empty string, not error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := hashPassword(tt.password)

			if tt.wantErr {
				if err == nil {
					t.Errorf("hashPassword() expected error but got none")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("hashPassword() error = %v, should contain %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("hashPassword() unexpected error = %v", err)
				return
			}

			if tt.password == "" {
				if hash != "" {
					t.Errorf("hashPassword() with empty password should return empty hash, got %q", hash)
				}
				return
			}

			// Verify hash is not empty and looks like argon2id
			if hash == "" {
				t.Error("hashPassword() returned empty hash for valid password")
			}
			if !strings.HasPrefix(hash, "$argon2id$") {
				t.Errorf("hashPassword() returned hash without argon2id prefix: %q", hash)
			}
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	// Create a valid hash first
	validPassword := "MyStr0ng!Pass123"
	hash, err := hashPassword(validPassword)
	if err != nil {
		t.Fatalf("Failed to create test hash: %v", err)
	}

	tests := []struct {
		name     string
		password string
		hash     string
		want     bool
		wantErr  bool
	}{
		{
			name:     "correct password",
			password: validPassword,
			hash:     hash,
			want:     true,
			wantErr:  false,
		},
		{
			name:     "incorrect password",
			password: "WrongPass!123",
			hash:     hash,
			want:     false,
			wantErr:  false,
		},
		{
			name:     "empty password",
			password: "",
			hash:     hash,
			want:     false,
			wantErr:  true,
		},
		{
			name:     "empty hash",
			password: validPassword,
			hash:     "",
			want:     false,
			wantErr:  true,
		},
		{
			name:     "both empty",
			password: "",
			hash:     "",
			want:     false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := verifyPassword(tt.password, tt.hash)

			if tt.wantErr {
				if err == nil {
					t.Errorf("verifyPassword() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("verifyPassword() unexpected error = %v", err)
				return
			}

			if match != tt.want {
				t.Errorf("verifyPassword() = %v, want %v", match, tt.want)
			}
		})
	}
}

func TestVerifyPasswordDifferentHashes(t *testing.T) {
	// Verify that the same password produces different hashes (due to random salt)
	password := "MyStr0ng!Pass456"

	hash1, err := hashPassword(password)
	if err != nil {
		t.Fatalf("Failed to create first hash: %v", err)
	}

	hash2, err := hashPassword(password)
	if err != nil {
		t.Fatalf("Failed to create second hash: %v", err)
	}

	if hash1 == hash2 {
		t.Error("Same password produced identical hashes - salt may not be random")
	}

	// Both should verify correctly
	match1, err := verifyPassword(password, hash1)
	if err != nil {
		t.Errorf("verifyPassword() with first hash failed: %v", err)
	}
	if !match1 {
		t.Error("verifyPassword() with first hash returned false")
	}

	match2, err := verifyPassword(password, hash2)
	if err != nil {
		t.Errorf("verifyPassword() with second hash failed: %v", err)
	}
	if !match2 {
		t.Error("verifyPassword() with second hash returned false")
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid strong password",
			password: "MyStr0ng!Pass",
			wantErr:  false,
		},
		{
			name:     "valid with multiple special chars",
			password: "C0mpl3x!@#",
			wantErr:  false,
		},
		{
			name:        "too short - 7 chars",
			password:    "Short1!",
			wantErr:     true,
			errContains: "at least 8 characters",
		},
		{
			name:        "no uppercase",
			password:    "lowercase1!",
			wantErr:     true,
			errContains: "uppercase",
		},
		{
			name:        "no lowercase",
			password:    "UPPERCASE1!",
			wantErr:     true,
			errContains: "lowercase",
		},
		{
			name:        "no digits",
			password:    "NoDigits!@#",
			wantErr:     true,
			errContains: "digit",
		},
		{
			name:        "no special chars",
			password:    "NoSpecial123",
			wantErr:     true,
			errContains: "special character",
		},
		{
			name:        "empty password",
			password:    "",
			wantErr:     true,
			errContains: "at least 8 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePasswordStrength(tt.password)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validatePasswordStrength() expected error but got none")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validatePasswordStrength() error = %v, should contain %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("validatePasswordStrength() unexpected error = %v", err)
			}
		})
	}
}
