package storage

import "testing"

func TestStorageErrorErrorCode(t *testing.T) {
	err := NewStorageError(ErrCodeNotFound, "snapshot missing", nil)

	if got := err.ErrorCode(); got != string(ErrCodeNotFound) {
		t.Fatalf("ErrorCode() = %q, want %q", got, ErrCodeNotFound)
	}
}
