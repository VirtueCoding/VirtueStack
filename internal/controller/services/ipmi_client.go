// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strings"
	"time"
)

// ErrInvalidIPMIAddress indicates that the IPMI address is not a valid IP format.
var ErrInvalidIPMIAddress = fmt.Errorf("invalid IPMI address format")

type IPMIClient struct {
	host     string
	username string
	password string
	timeout  time.Duration
	logger   *slog.Logger
}

func NewIPMIClient(host, username, password string, logger *slog.Logger) *IPMIClient {
	return &IPMIClient{
		host:     host,
		username: username,
		password: password,
		timeout:  30 * time.Second,
		logger:   logger.With("component", "ipmi-client"),
	}
}

// ValidateHost checks if the host is a valid IP address.
// This prevents command injection attacks through malformed IPMI addresses.
func (c *IPMIClient) ValidateHost() error {
	if ip := net.ParseIP(c.host); ip == nil {
		return fmt.Errorf("%w: %s", ErrInvalidIPMIAddress, c.host)
	}
	return nil
}

func (c *IPMIClient) PowerCycle(ctx context.Context) error {
	if err := c.ValidateHost(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ipmitool",
		"-H", c.host,
		"-U", c.username,
		"-E", // Use IPMITOOL_PASSWORD environment variable
		"chassis", "power", "cycle")
	cmd.Env = append(cmd.Environ(), "IPMITOOL_PASSWORD="+c.password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.logger.Error("ipmitool power cycle failed",
			"host", c.host,
			"error", err,
			"output", string(output))
		return fmt.Errorf("ipmitool power cycle failed: %w", err)
	}

	c.logger.Info("IPMI power cycle command executed successfully",
		"host", c.host)

	return nil
}

func (c *IPMIClient) PowerOn(ctx context.Context) error {
	if err := c.ValidateHost(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ipmitool",
		"-H", c.host,
		"-U", c.username,
		"-E", // Use IPMITOOL_PASSWORD environment variable
		"chassis", "power", "on")
	cmd.Env = append(cmd.Environ(), "IPMITOOL_PASSWORD="+c.password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.logger.Error("ipmitool power on failed",
			"host", c.host,
			"error", err,
			"output", string(output))
		return fmt.Errorf("ipmitool power on failed: %w", err)
	}

	return nil
}

func (c *IPMIClient) PowerOff(ctx context.Context) error {
	if err := c.ValidateHost(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ipmitool",
		"-H", c.host,
		"-U", c.username,
		"-E", // Use IPMITOOL_PASSWORD environment variable
		"chassis", "power", "off")
	cmd.Env = append(cmd.Environ(), "IPMITOOL_PASSWORD="+c.password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.logger.Error("ipmitool power off failed",
			"host", c.host,
			"error", err,
			"output", string(output))
		return fmt.Errorf("ipmitool power off failed: %w", err)
	}

	return nil
}

func (c *IPMIClient) GetPowerStatus(ctx context.Context) (bool, error) {
	if err := c.ValidateHost(); err != nil {
		return false, err
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ipmitool",
		"-H", c.host,
		"-U", c.username,
		"-E", // Use IPMITOOL_PASSWORD environment variable
		"chassis", "power", "status")
	cmd.Env = append(cmd.Environ(), "IPMITOOL_PASSWORD="+c.password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.logger.Error("ipmitool power status failed",
			"host", c.host,
			"error", err,
			"output", string(output))
		return false, fmt.Errorf("ipmitool power status failed: %w", err)
	}

	statusStr := string(output)
	return containsPowerOn(statusStr), nil
}

func containsPowerOn(s string) bool {
	normalized := strings.ToLower(strings.TrimSpace(s))
	if normalized == "" {
		return false
	}

	normalized = strings.Join(strings.Fields(normalized), " ")

	powerOnIndicators := []string{
		"power is on",
		"chassis power is on",
		"system power is on",
		"power on",
	}

	for _, indicator := range powerOnIndicators {
		if strings.Contains(normalized, indicator) {
			return true
		}
	}

	return false
}
