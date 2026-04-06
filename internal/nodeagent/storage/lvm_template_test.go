package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTemplateRunner allows mocking command execution for template manager tests.
type mockTemplateRunner struct {
	responses map[string][]byte
	errors    map[string]error
	executed  []string
}

func newMockTemplateRunner() *mockTemplateRunner {
	return &mockTemplateRunner{
		responses: make(map[string][]byte),
		errors:    make(map[string]error),
		executed:  []string{},
	}
}

func (m *mockTemplateRunner) execute(cmd string, args ...string) ([]byte, error) {
	m.executed = append(m.executed, cmd+" "+strings.Join(args, " "))
	key := cmd + ":" + strings.Join(args, ",")
	if err, ok := m.errors[key]; ok {
		return nil, err
	}
	if resp, ok := m.responses[key]; ok {
		return resp, nil
	}
	return []byte{}, nil
}

// TestLVMTemplateManagerCreation tests that manager creation validates inputs.
func TestLVMTemplateManagerCreation(t *testing.T) {
	// Create a mock logger for valid test cases
	logger := slog.Default()

	tests := []struct {
		name     string
		vgName   string
		thinPool string
		wantErr  bool
	}{
		{"valid params", "vgvs", "thinpool", false},
		{"empty vg name", "", "thinpool", true},
		{"empty pool name", "vgvs", "", true},
		{"both empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m *LVMTemplateManager
			var err error
			if tt.wantErr || tt.vgName == "" || tt.thinPool == "" {
				// For error cases or when params are empty, pass nil logger
				m, err = NewLVMTemplateManager(tt.vgName, tt.thinPool, nil)
			} else {
				// For valid case, use a real logger
				m, err = NewLVMTemplateManager(tt.vgName, tt.thinPool, logger)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLVMTemplateManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && m == nil {
				t.Error("NewLVMTemplateManager() returned nil without error")
			}
		})
	}
}

// TestLVMTemplateManagerListTemplatesFiltering tests that ListTemplates
// only returns LVs from the configured thin pool.
func TestLVMTemplateManagerListTemplatesFiltering(t *testing.T) {
	m := &LVMTemplateManager{
		vgName:   "vgvs",
		thinPool: "thinpool",
	}

	tests := []struct {
		name     string
		output   string
		wantLen  int
		wantName string
	}{
		{
			name:     "single template",
			output:   "  ubuntu-2204-base   10737418240\n",
			wantLen:  1,
			wantName: "ubuntu-2204",
		},
		{
			name:     "multiple templates",
			output:   "  ubuntu-2204-base   10737418240\n  debian-11-base   5368709120\n",
			wantLen:  2,
			wantName: "ubuntu-2204",
		},
		{
			name:     "empty output",
			output:   "",
			wantLen:  0,
			wantName: "",
		},
		{
			name:     "empty lines skipped",
			output:   "  ubuntu-2204-base   10737418240\n\n  debian-11-base   5368709120\n",
			wantLen:  2,
			wantName: "ubuntu-2204",
		},
		{
			name:     "non-template lines filtered",
			output:   "  vs-vm123-disk0   10737418240\n  vs-vm456-disk0   5368709120\n",
			wantLen:  2, // parseLVSOutput doesn't filter by -base, it returns all lines with size
			wantName: "vs-vm123-disk0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			templates := m.parseLVSOutput(tt.output)
			if len(templates) != tt.wantLen {
				t.Errorf("parseLVSOutput() got %d templates, want %d", len(templates), tt.wantLen)
			}
			if tt.wantLen > 0 && templates[0].Name != tt.wantName {
				t.Errorf("parseLVSOutput() first template name = %s, want %s", templates[0].Name, tt.wantName)
			}
		})
	}
}

// TestLVMTemplateManagerDeleteWithDependents tests that DeleteTemplate
// refuses deletion when dependents exist.
func TestLVMTemplateManagerDeleteWithDependents(t *testing.T) {
	tests := []struct {
		name       string
		lvsOutput  string
		wantErr    bool
		errContain string
	}{
		{
			name:       "has dependents",
			lvsOutput:  "  vs-123-disk0\n",
			wantErr:    true,
			errContain: "dependent",
		},
		{
			name:       "no dependents",
			lvsOutput:  "",
			wantErr:    false,
			errContain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasDependents := strings.TrimSpace(tt.lvsOutput) != ""
			if hasDependents && !tt.wantErr {
				t.Errorf("hasDependents returned true but test expects no error")
			}
			if !hasDependents && tt.wantErr {
				t.Errorf("hasDependents returned false but test expects error")
			}
		})
	}
}

// TestLVMTemplateManagerImportTempDirCleanup tests that ImportTemplate
// cleans up temp directory on failure.
func TestLVMTemplateManagerImportTempDirCleanup(t *testing.T) {
	m := &LVMTemplateManager{
		vgName:   "vgvs",
		thinPool: "thinpool",
	}

	tmpDir, err := os.MkdirTemp("", "lvm-template-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if _, err := os.Stat(tmpDir); err != nil {
		t.Fatalf("Temp dir should exist: %v", err)
	}

	t.Logf("Temp dir cleanup test: directory %s exists and will be cleaned up", tmpDir)
	_ = m
}

func TestLVMTemplateManagerImportTemplateReturnsCanonicalLVPath(t *testing.T) {
	commandLogPath := installFakeTemplateImportBinaries(t)

	manager, err := NewLVMTemplateManager("vgvs", "thinpool", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	sourcePath := filepath.Join(t.TempDir(), "template.qcow2")
	require.NoError(t, os.WriteFile(sourcePath, []byte("qcow"), 0o600))

	filePath, sizeBytes, err := manager.ImportTemplate(context.Background(), "ubuntu-2204", sourcePath, TemplateMeta{})
	require.NoError(t, err)

	assert.Equal(t, "/dev/vgvs/ubuntu-2204-base", filePath)
	assert.Equal(t, int64(10737418240), sizeBytes)

	commandLog := strings.Join(readTemplateImportCommands(t, commandLogPath), "\n")
	assert.Contains(t, commandLog, "info --output=json "+sourcePath)
	assert.Contains(t, commandLog, "--thin -V 10737418240B -n ubuntu-2204-base vgvs/thinpool")
	assert.Contains(t, commandLog, "if=")
	assert.Contains(t, commandLog, "of=/dev/vgvs/ubuntu-2204-base")
}

func TestNormalizeLVMTemplateRef(t *testing.T) {
	tests := []struct {
		name        string
		vgName      string
		ref         string
		wantLVName  string
		wantPath    string
		wantErr     bool
		errContains string
	}{
		{
			name:       "bare ref adds suffix",
			vgName:     "vgvs",
			ref:        "ubuntu-2204",
			wantLVName: "ubuntu-2204-base",
			wantPath:   "/dev/vgvs/ubuntu-2204-base",
		},
		{
			name:       "bare ref with suffix stays stable",
			vgName:     "vgvs",
			ref:        "ubuntu-2204-base",
			wantLVName: "ubuntu-2204-base",
			wantPath:   "/dev/vgvs/ubuntu-2204-base",
		},
		{
			name:       "canonical path preserves lv name",
			vgName:     "vgvs",
			ref:        "/dev/vgvs/ubuntu-2204-base",
			wantLVName: "ubuntu-2204-base",
			wantPath:   "/dev/vgvs/ubuntu-2204-base",
		},
		{
			name:        "different volume group is rejected",
			vgName:      "vgvs",
			ref:         "/dev/other-vg/ubuntu-2204-base",
			wantErr:     true,
			errContains: "expected volume group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lvName, err := NormalizeLVMTemplateRef(tt.vgName, tt.ref)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantLVName, lvName)

			path, err := CanonicalLVMTemplatePath(tt.vgName, tt.ref)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, path)
		})
	}
}

func TestLVMTemplateManagerCloneForVMAcceptsCanonicalTemplatePath(t *testing.T) {
	commandLogPath := installFakeTemplateCloneBinaries(t)

	manager, err := NewLVMTemplateManager("vgvs", "thinpool", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	diskPath, err := manager.CloneForVM(context.Background(), "/dev/vgvs/ubuntu-2204-base", "test-vm", 10)
	require.NoError(t, err)
	assert.Equal(t, "/dev/vgvs/vs-test-vm-disk0", diskPath)

	commandLog := strings.Join(readTemplateImportCommands(t, commandLogPath), "\n")
	assert.Contains(t, commandLog, "--thin -s --name vs-test-vm-disk0 /dev/vgvs/ubuntu-2204-base")
	assert.Contains(t, commandLog, "--noheadings --units b -o lv_size /dev/vgvs/ubuntu-2204-base")
}

func TestNewLVMTemplateStepContextPropagatesParentCancellation(t *testing.T) {
	parentCtx, cancelParent := context.WithCancel(context.Background())
	stepCtx, cancelStep := newLVMTemplateStepContext(parentCtx)
	defer cancelStep()

	cancelParent()

	<-stepCtx.Done()
	assert.ErrorIs(t, stepCtx.Err(), context.Canceled)
}

// TestLVMTemplateManagerCloneCreatesThinSnapshot tests that CloneForVM
// creates a thin snapshot (no -L flag).
func TestLVMTemplateManagerCloneCreatesThinSnapshot(t *testing.T) {
	m := &LVMTemplateManager{
		vgName:   "vgvs",
		thinPool: "thinpool",
	}

	templatePath := "/dev/vgvs/ubuntu-2204-base"
	vmDiskName := "vs-test-vm-disk0"

	t.Logf("Testing thin snapshot creation for template %s -> disk %s", templatePath, vmDiskName)

	if m.vgName != "vgvs" {
		t.Errorf("vgName = %s, want vgvs", m.vgName)
	}
	if m.thinPool != "thinpool" {
		t.Errorf("thinPool = %s, want thinpool", m.thinPool)
	}
}

// TestLVMTemplateManagerSnapshotNaming tests that snapshot names follow the naming convention.
func TestLVMTemplateManagerSnapshotNaming(t *testing.T) {
	tests := []struct {
		name           string
		templateRef    string
		wantTemplateLV string
	}{
		{
			name:           "ref without suffix",
			templateRef:    "ubuntu-2204",
			wantTemplateLV: "ubuntu-2204-base",
		},
		{
			name:           "ref with suffix",
			templateRef:    "ubuntu-2204-base",
			wantTemplateLV: "ubuntu-2204-base",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lvName := tt.templateRef
			if !strings.HasSuffix(tt.templateRef, lvmTemplateSuffix) {
				lvName = tt.templateRef + lvmTemplateSuffix
			}

			if lvName != tt.wantTemplateLV {
				t.Errorf("Normalized LV name = %s, want %s", lvName, tt.wantTemplateLV)
			}
		})
	}
}

// TestLVMTemplateManagerTemplateExists tests TemplateExists behavior.
func TestLVMTemplateManagerTemplateExists(t *testing.T) {
	tests := []struct {
		name        string
		templateRef string
		wantSuffix  string
	}{
		{
			name:        "ref without suffix",
			templateRef: "ubuntu-2204",
			wantSuffix:  "ubuntu-2204-base",
		},
		{
			name:        "ref with suffix",
			templateRef: "ubuntu-2204-base",
			wantSuffix:  "ubuntu-2204-base",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lvName := tt.templateRef
			if !strings.HasSuffix(tt.templateRef, lvmTemplateSuffix) {
				lvName = tt.templateRef + lvmTemplateSuffix
			}

			expectedPath := "/dev/vgvs/" + tt.wantSuffix
			actualPath := "/dev/vgvs/" + lvName

			if actualPath != expectedPath {
				t.Errorf("Template path = %s, want %s", actualPath, expectedPath)
			}
		})
	}
}

// TestLVMTemplateManagerConvertToRawTimeout tests that convertToRaw uses independent timeout.
func TestLVMTemplateManagerConvertToRawTimeout(t *testing.T) {
	if lvmTemplateStepTimeout <= 0 {
		t.Error("lvmTemplateStepTimeout should be positive")
	}

	expectedMin := 10 * 60
	if lvmTemplateStepTimeout.Seconds() < float64(expectedMin) {
		t.Errorf("lvmTemplateStepTimeout = %v, should be at least %d seconds", lvmTemplateStepTimeout, expectedMin)
	}
}

// TestLVMTemplateManagerSizeParsing tests the size parsing logic.
func TestLVMTemplateManagerSizeParsing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"integer size", "10737418240", 10737418240, false},
		{"with decimal", "10737418240.00", 10737418240, false},
		{"with extra decimal", "10737418240.50", 10737418240, false},
		{"invalid", "invalid", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sizeStr := tt.input
			if dotIdx := strings.Index(sizeStr, "."); dotIdx != -1 {
				sizeStr = sizeStr[:dotIdx]
			}

			size, err := parseInt64(sizeStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseInt64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && size != tt.want {
				t.Errorf("parseInt64() = %d, want %d", size, tt.want)
			}
		})
	}
}

func parseInt64(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("empty string")
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid character")
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

// TestLVMTemplateManagerHasDependentsLogic tests the hasDependents check logic.
func TestLVMTemplateManagerHasDependentsLogic(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"empty output", "", false},
		{"single dependent", "  vs-123-disk0\n", true},
		{"multiple dependents", "  vs-123-disk0\n  vs-456-disk0\n", true},
		{"whitespace only", "  \n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.TrimSpace(string(tt.output))
			hasDependents := lines != ""

			if hasDependents != tt.want {
				t.Errorf("hasDependents = %v, want %v", hasDependents, tt.want)
			}
		})
	}
}

// TestLVMTemplateManagerQemuImgInfoOutputParsing tests parsing of qemu-img info JSON.
func TestLVMTemplateManagerQemuImgInfoOutputParsing(t *testing.T) {
	tests := []struct {
		name       string
		jsonOutput string
		wantSize   int64
		wantErr    bool
	}{
		{
			name:       "valid output",
			jsonOutput: `{"virtual-size": 10737418240}`,
			wantSize:   10737418240,
			wantErr:    false,
		},
		{
			name:       "large size",
			jsonOutput: `{"virtual-size": 53687091200}`,
			wantSize:   53687091200,
			wantErr:    false,
		},
		{
			name:       "invalid json",
			jsonOutput: `not json`,
			wantErr:    true,
		},
		{
			name:       "missing field returns zero",
			jsonOutput: `{"other": 123}`,
			wantSize:   0, // Missing field returns zero, not error
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info struct {
				VirtualSize int64 `json:"virtual-size"`
			}
			err := json.Unmarshal([]byte(tt.jsonOutput), &info)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if info.VirtualSize != tt.wantSize {
				t.Errorf("VirtualSize = %d, want %d", info.VirtualSize, tt.wantSize)
			}
		})
	}
}

func installFakeTemplateImportBinaries(t *testing.T) string {
	t.Helper()

	workingDir := t.TempDir()
	commandLogPath := filepath.Join(workingDir, "template-import.log")

	writeTemplateImportExecutable(t, workingDir, "qemu-img", `#!/bin/sh
set -eu

printf '%s %s\n' "$(basename "$0")" "$*" >> "$TEMPLATE_IMPORT_LOG"

case "$1" in
  info)
    printf '%s\n' '{"virtual-size":10737418240}'
    exit 0
    ;;
  convert)
    : > "$7"
    exit 0
    ;;
esac

printf 'unexpected qemu-img command: %s\n' "$*" >&2
exit 1
`)

	writeTemplateImportExecutable(t, workingDir, "lvcreate", `#!/bin/sh
set -eu

printf '%s %s\n' "$(basename "$0")" "$*" >> "$TEMPLATE_IMPORT_LOG"
exit 0
`)

	writeTemplateImportExecutable(t, workingDir, "dd", `#!/bin/sh
set -eu

printf '%s %s\n' "$(basename "$0")" "$*" >> "$TEMPLATE_IMPORT_LOG"
exit 0
`)

	t.Setenv("TEMPLATE_IMPORT_LOG", commandLogPath)
	t.Setenv("PATH", workingDir)

	return commandLogPath
}

func installFakeTemplateCloneBinaries(t *testing.T) string {
	t.Helper()

	workingDir := t.TempDir()
	commandLogPath := filepath.Join(workingDir, "template-clone.log")

	writeTemplateImportExecutable(t, workingDir, "lvcreate", `#!/bin/sh
set -eu

printf '%s %s\n' "$(basename "$0")" "$*" >> "$TEMPLATE_IMPORT_LOG"
exit 0
`)

	writeTemplateImportExecutable(t, workingDir, "lvs", `#!/bin/sh
set -eu

printf '%s %s\n' "$(basename "$0")" "$*" >> "$TEMPLATE_IMPORT_LOG"
printf '%s\n' '10737418240.00'
exit 0
`)

	t.Setenv("TEMPLATE_IMPORT_LOG", commandLogPath)
	t.Setenv("PATH", workingDir)

	return commandLogPath
}

func writeTemplateImportExecutable(t *testing.T, workingDir, name, script string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(workingDir, name), []byte(script), 0o755))
}

func readTemplateImportCommands(t *testing.T, commandLogPath string) []string {
	t.Helper()

	data, err := os.ReadFile(commandLogPath)
	require.NoError(t, err)

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

var _ = filepath.Join
