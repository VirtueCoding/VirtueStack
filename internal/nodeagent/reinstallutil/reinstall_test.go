package reinstallutil

import (
	"errors"
	"os"
	"testing"
)

func TestLookupSizingThenDelete(t *testing.T) {
	t.Run("reads sizing before deleting", func(t *testing.T) {
		order := make([]string, 0, 2)
		sizing, err := LookupSizingThenDelete(
			func() (Sizing, error) {
				order = append(order, "lookup")
				return Sizing{VCPU: 4, MemoryMB: 8192}, nil
			},
			func() error {
				order = append(order, "delete")
				return nil
			},
		)
		if err != nil {
			t.Fatalf("LookupSizingThenDelete() error = %v", err)
		}
		if len(order) != 2 || order[0] != "lookup" || order[1] != "delete" {
			t.Fatalf("call order = %v, want [lookup delete]", order)
		}
		if sizing.VCPU != 4 || sizing.MemoryMB != 8192 {
			t.Fatalf("sizing = %+v, want {VCPU:4 MemoryMB:8192}", sizing)
		}
	})

	t.Run("falls back to defaults when lookup fails", func(t *testing.T) {
		sizing, err := LookupSizingThenDelete(
			func() (Sizing, error) {
				return Sizing{}, os.ErrNotExist
			},
			func() error {
				return nil
			},
		)
		if err != nil {
			t.Fatalf("LookupSizingThenDelete() delete error = %v", err)
		}
		if sizing != DefaultSizing {
			t.Fatalf("sizing = %+v, want %+v", sizing, DefaultSizing)
		}
	})

	t.Run("returns lookup error without deleting on unexpected lookup failure", func(t *testing.T) {
		lookupErr := assertErr("lookup failed")
		deleteCalled := false

		sizing, err := LookupSizingThenDelete(
			func() (Sizing, error) {
				return Sizing{}, lookupErr
			},
			func() error {
				deleteCalled = true
				return nil
			},
		)

		if !errors.Is(err, lookupErr) {
			t.Fatalf("LookupSizingThenDelete() error = %v, want %v", err, lookupErr)
		}
		if deleteCalled {
			t.Fatal("LookupSizingThenDelete() should not delete when lookup fails unexpectedly")
		}
		if sizing != (Sizing{}) {
			t.Fatalf("sizing = %+v, want zero value", sizing)
		}
	})
}

func TestNormalizeSizing(t *testing.T) {
	tests := []struct {
		name   string
		input  Sizing
		expect Sizing
	}{
		{
			name:   "keeps valid values",
			input:  Sizing{VCPU: 2, MemoryMB: 2048},
			expect: Sizing{VCPU: 2, MemoryMB: 2048},
		},
		{
			name:   "defaults invalid values",
			input:  Sizing{},
			expect: DefaultSizing,
		},
		{
			name:   "defaults only missing memory",
			input:  Sizing{VCPU: 8},
			expect: Sizing{VCPU: 8, MemoryMB: DefaultSizing.MemoryMB},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeSizing(tt.input)
			if got != tt.expect {
				t.Fatalf("NormalizeSizing() = %+v, want %+v", got, tt.expect)
			}
		})
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
