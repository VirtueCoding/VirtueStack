package transferutil

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	validLVMName = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.+-]*$`)
	validLVName  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

	ErrInvalidOffset = errors.New("invalid transfer offset")
	ErrTransferSize  = errors.New("invalid transfer size")
)

type ReceiveTarget struct {
	OpenPath      string
	CreateImageID string
}

type ReceiveTracker struct {
	total         int64
	bytesReceived int64
}

func ResolveLVMSourcePath(sourceDiskPath, snapshotName, requestedVG, configuredVG string) (string, error) {
	if err := validateVG(configuredVG); err != nil {
		return "", err
	}
	if requestedVG != "" && requestedVG != configuredVG {
		return "", fmt.Errorf("requested volume group %q does not match configured volume group", requestedVG)
	}

	if snapshotName != "" {
		if err := validateLV(snapshotName); err != nil {
			return "", err
		}
		return canonicalLVMPath(configuredVG, snapshotName), nil
	}

	vg, lv, err := splitLVMPath(sourceDiskPath)
	if err != nil {
		return "", err
	}
	if vg != configuredVG {
		return "", fmt.Errorf("source disk path volume group %q does not match configured volume group", vg)
	}
	return canonicalLVMPath(vg, lv), nil
}

func ResolveReceiveTarget(storageBackend, targetPath, storagePath, configuredVG, configuredThinPool, requestedVG, requestedThinPool string) (ReceiveTarget, error) {
	switch storageBackend {
	case "lvm":
		if err := validateVG(configuredVG); err != nil {
			return ReceiveTarget{}, err
		}
		if err := validateVG(configuredThinPool); err != nil {
			return ReceiveTarget{}, fmt.Errorf("invalid thin pool: %w", err)
		}
		if requestedVG != "" && requestedVG != configuredVG {
			return ReceiveTarget{}, fmt.Errorf("requested volume group %q does not match configured volume group", requestedVG)
		}
		if requestedThinPool != "" && requestedThinPool != configuredThinPool {
			return ReceiveTarget{}, fmt.Errorf("requested thin pool %q does not match configured thin pool", requestedThinPool)
		}
		vg, lv, err := splitLVMPath(targetPath)
		if err != nil {
			return ReceiveTarget{}, err
		}
		if vg != configuredVG {
			return ReceiveTarget{}, fmt.Errorf("target disk path volume group %q does not match configured volume group", vg)
		}
		return ReceiveTarget{
			OpenPath:      canonicalLVMPath(vg, lv),
			CreateImageID: lv,
		}, nil
	default:
		if err := validatePathWithin(targetPath, storagePath); err != nil {
			return ReceiveTarget{}, err
		}
		return ReceiveTarget{OpenPath: filepath.Clean(targetPath)}, nil
	}
}

func NewReceiveTracker(total int64) (*ReceiveTracker, error) {
	if total < 0 {
		return nil, fmt.Errorf("%w: total cannot be negative", ErrTransferSize)
	}
	return &ReceiveTracker{total: total}, nil
}

func (t *ReceiveTracker) Accept(offset int64, length int) error {
	if offset < 0 {
		return fmt.Errorf("%w: offset cannot be negative", ErrInvalidOffset)
	}
	if int64(length) < 0 {
		return fmt.Errorf("%w: length cannot be negative", ErrTransferSize)
	}
	if offset != t.bytesReceived {
		return fmt.Errorf("%w: expected offset %d, got %d", ErrInvalidOffset, t.bytesReceived, offset)
	}
	if offset+int64(length) > t.total {
		return fmt.Errorf("%w: chunk ending at %d exceeds total %d", ErrTransferSize, offset+int64(length), t.total)
	}
	t.bytesReceived += int64(length)
	return nil
}

func (t *ReceiveTracker) Finalize() error {
	return ValidateTransferredBytes(t.total, t.bytesReceived)
}

func (t *ReceiveTracker) BytesReceived() int64 {
	return t.bytesReceived
}

func ValidateTransferredBytes(expected, actual int64) error {
	if expected != actual {
		return fmt.Errorf("%w: expected %d bytes, received %d", ErrTransferSize, expected, actual)
	}
	return nil
}

func validatePathWithin(path, allowedPrefix string) error {
	if path == "" {
		return fmt.Errorf("path must not be empty")
	}
	cleaned := filepath.Clean(path)
	if !strings.HasPrefix(cleaned, allowedPrefix+"/") && cleaned != allowedPrefix {
		return fmt.Errorf("path %q is outside the allowed directory %q", cleaned, allowedPrefix)
	}
	return nil
}

func validateVG(name string) error {
	if name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if !validLVMName.MatchString(name) {
		return fmt.Errorf("invalid LVM name %q", name)
	}
	return nil
}

func validateLV(name string) error {
	if name == "" {
		return fmt.Errorf("logical volume name must not be empty")
	}
	if strings.Contains(name, "..") || !validLVName.MatchString(name) {
		return fmt.Errorf("invalid logical volume name %q", name)
	}
	return nil
}

func splitLVMPath(path string) (string, string, error) {
	cleaned := filepath.Clean(path)
	parts := strings.Split(cleaned, string(filepath.Separator))
	if len(parts) != 4 || parts[0] != "" || parts[1] != "dev" {
		return "", "", fmt.Errorf("invalid LVM device path %q", path)
	}
	vg := parts[2]
	lv := parts[3]
	if err := validateVG(vg); err != nil {
		return "", "", err
	}
	if err := validateLV(lv); err != nil {
		return "", "", err
	}
	return vg, lv, nil
}

func canonicalLVMPath(vg, lv string) string {
	return filepath.Join(string(filepath.Separator), "dev", vg, lv)
}
