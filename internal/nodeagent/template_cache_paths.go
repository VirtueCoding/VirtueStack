package nodeagent

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/AbuGosok/VirtueStack/internal/shared/config"
)

func effectiveTemplateStorageBackend(requested, configured string) string {
	if requested == "" {
		return configured
	}
	return requested
}

func validateTemplateRequestBackend(requested, configured string) error {
	if requested == "" {
		return nil
	}
	if requested != configured {
		return fmt.Errorf("requested storage backend %q does not match configured backend %q", requested, configured)
	}
	return nil
}

func resolveTemplateLocalPath(storageBackend, storagePath, lvmVG, ref string) (string, error) {
	switch effectiveTemplateStorageBackend(storageBackend, "ceph") {
	case "qcow":
		return filepath.Join(storagePath, config.DefaultTemplatesDir, ref+".qcow2"), nil
	case "lvm":
		return canonicalLVMTemplatePath(lvmVG, ref)
	case "ceph":
		return ref, nil
	default:
		return "", fmt.Errorf("unsupported storage backend %q", storageBackend)
	}
}

func canonicalLVMTemplatePath(vgName, ref string) (string, error) {
	lvName, err := normalizeLVMTemplateRef(vgName, ref)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/%s/%s", vgName, lvName), nil
}

func normalizeLVMTemplateRef(vgName, ref string) (string, error) {
	if !strings.HasPrefix(ref, "/") {
		if !strings.HasSuffix(ref, "-base") {
			return ref + "-base", nil
		}
		return ref, nil
	}

	expectedPrefix := fmt.Sprintf("/dev/%s/", vgName)
	if !strings.HasPrefix(ref, expectedPrefix) {
		return "", fmt.Errorf("validating template ref: expected volume group %q in %q", vgName, ref)
	}

	lvName := strings.TrimPrefix(ref, expectedPrefix)
	if lvName == "" {
		return "", fmt.Errorf("validating template ref: missing logical volume name in %q", ref)
	}
	return lvName, nil
}
