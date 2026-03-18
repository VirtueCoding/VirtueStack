// Package guest provides QEMU Guest Agent integration for VirtueStack.
// It enables communication with guest VMs via the QEMU Guest Agent protocol
// for operations like filesystem freeze, password changes, and graceful shutdown.
package guest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"libvirt.org/go/libvirt"
)

// Constants for QEMU Guest Agent operations.
const (
	// DefaultTimeout is the maximum time to wait for guest agent responses.
	// Guest OS environments are untrusted and can hang indefinitely,
	// so we enforce a strict timeout to prevent blocking operations.
	DefaultTimeout = 10 * time.Second

	// QEMU Agent command strings
	cmdPing             = `{"execute":"guest-ping"}`
	cmdFreezeFilesystem = `{"execute":"guest-fsfreeze-freeze"}`
	cmdThawFilesystem   = `{"execute":"guest-fsfreeze-thaw"}`
	cmdShutdown         = `{"execute":"guest-shutdown"}`
)

// QEMUGuestAgent provides methods to interact with QEMU Guest Agent
// running inside a virtual machine. All operations are wrapped with
// strict timeouts to prevent indefinite blocking on untrusted guest systems.
type QEMUGuestAgent struct {
	domain *libvirt.Domain
	logger *slog.Logger
}

// NewQEMUGuestAgent creates a new QEMU Guest Agent wrapper for the given domain.
// The domain must have the QEMU Guest Agent channel configured and the guest
// agent service running inside the VM.
func NewQEMUGuestAgent(domain *libvirt.Domain, logger *slog.Logger) *QEMUGuestAgent {
	return &QEMUGuestAgent{
		domain: domain,
		logger: logger.With("component", "qemu-guest-agent"),
	}
}

// Ping sends a ping command to the QEMU Guest Agent to verify connectivity.
// Returns nil if the agent responds successfully within the timeout.
func (a *QEMUGuestAgent) Ping(ctx context.Context) error {
	_, err := a.executeCommand(ctx, cmdPing, "ping")
	if err != nil {
		return fmt.Errorf("guest agent ping: %w", err)
	}
	return nil
}

// FreezeFilesystems freezes all filesystems in the guest VM.
// This is typically used before taking consistent snapshots.
// Returns the number of frozen filesystems on success.
func (a *QEMUGuestAgent) FreezeFilesystems(ctx context.Context) (int, error) {
	result, err := a.executeCommand(ctx, cmdFreezeFilesystem, "freeze-filesystems")
	if err != nil {
		return 0, fmt.Errorf("guest agent freeze: %w", err)
	}
	// Parse the response to get the count of frozen filesystems
	var response struct {
		Return int `json:"return"`
	}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		return 0, fmt.Errorf("parsing freeze response: %w", err)
	}
	return response.Return, nil
}

// ThawFilesystems thaws (unfreezes) all filesystems in the guest VM.
// This should be called after taking a snapshot to restore normal I/O operations.
// Returns the number of thawed filesystems on success.
func (a *QEMUGuestAgent) ThawFilesystems(ctx context.Context) (int, error) {
	result, err := a.executeCommand(ctx, cmdThawFilesystem, "thaw-filesystems")
	if err != nil {
		return 0, fmt.Errorf("guest agent thaw: %w", err)
	}
	// Parse the response to get the count of thawed filesystems
	var response struct {
		Return int `json:"return"`
	}
	if err := json.Unmarshal([]byte(result), &response); err != nil {
		return 0, fmt.Errorf("parsing thaw response: %w", err)
	}
	return response.Return, nil
}

// Shutdown initiates a graceful shutdown of the guest VM.
// The guest agent will attempt to shut down the OS cleanly.
// Note: This command does not wait for the shutdown to complete.
func (a *QEMUGuestAgent) Shutdown(ctx context.Context) error {
	_, err := a.executeCommand(ctx, cmdShutdown, "shutdown")
	if err != nil {
		return fmt.Errorf("guest agent shutdown: %w", err)
	}
	return nil
}

// guestSetPasswordCmd is the JSON structure for the guest-set-user-password command.
type guestSetPasswordCmd struct {
	Execute   string                   `json:"execute"`
	Arguments guestSetPasswordCmdArgs  `json:"arguments"`
}

type guestSetPasswordCmdArgs struct {
	Username string `json:"username"`
	Password string `json:"cryptopassword,omitempty"`
}

// SetUserPassword sets the password for a user in the guest VM.
// The password is base64 encoded as required by the QEMU Guest Agent protocol.
// This operation requires the guest agent to have appropriate permissions.
func (a *QEMUGuestAgent) SetUserPassword(ctx context.Context, username, password string) error {
	// Base64 encode the password as required by guest-set-user-password
	encodedPassword := base64.StdEncoding.EncodeToString([]byte(password))

	payload := guestSetPasswordCmd{
		Execute: "guest-set-user-password",
		Arguments: guestSetPasswordCmdArgs{
			Username: username,
			Password: encodedPassword,
		},
	}
	cmdBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling set-user-password command: %w", err)
	}

	_, err = a.executeCommand(ctx, string(cmdBytes), "set-user-password")
	if err != nil {
		return fmt.Errorf("guest agent set-user-password: %w", err)
	}
	return nil
}

// executeCommand executes a QEMU Guest Agent command with strict timeout enforcement.
// It wraps the libvirt call with both a context timeout and the libvirt timeout parameter
// to ensure the operation cannot hang indefinitely on untrusted guest systems.
func (a *QEMUGuestAgent) executeCommand(ctx context.Context, command, operation string) (string, error) {
	// Create a child context with strict 10-second timeout
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	// Use a channel to receive the result asynchronously
	type result struct {
		output string
		err    error
	}
	resultChan := make(chan result, 1)

	go func() {
		// Use DOMAIN_QEMU_AGENT_COMMAND_DEFAULT flag and 10-second timeout
		// The timeout value (10) is passed to libvirt as seconds
		output, err := a.domain.QemuAgentCommand(
			command,
			libvirt.DOMAIN_QEMU_AGENT_COMMAND_DEFAULT,
			10, // 10 second timeout in libvirt
		)
		resultChan <- result{output: output, err: err}
	}()

	select {
	case <-ctx.Done():
		a.logger.Warn("guest agent command timed out", "operation", operation, "timeout", DefaultTimeout)
		return "", fmt.Errorf("guest agent %s: %w", operation, errors.ErrTimeout)
	case res := <-resultChan:
		if res.err != nil {
			a.logger.Error("guest agent command failed", "operation", operation, "error", res.err)
			return "", fmt.Errorf("guest agent %s: %w", operation, res.err)
		}
		return res.output, nil
	}
}
