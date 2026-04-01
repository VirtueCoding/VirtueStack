package reinstallutil

import (
	"errors"
	"os"
)

type Sizing struct {
	VCPU     int
	MemoryMB int
}

var DefaultSizing = Sizing{
	VCPU:     1,
	MemoryMB: 1024,
}

func NormalizeSizing(sizing Sizing) Sizing {
	if sizing.VCPU <= 0 {
		sizing.VCPU = DefaultSizing.VCPU
	}
	if sizing.MemoryMB <= 0 {
		sizing.MemoryMB = DefaultSizing.MemoryMB
	}
	return sizing
}

func LookupSizingThenDelete(lookup func() (Sizing, error), deleteFunc func() error) (Sizing, error) {
	sizing, err := lookup()
	switch {
	case err == nil:
		sizing = NormalizeSizing(sizing)
	case errors.Is(err, os.ErrNotExist):
		sizing = DefaultSizing
	default:
		return Sizing{}, err
	}
	if err := deleteFunc(); err != nil {
		return sizing, err
	}
	return sizing, nil
}
