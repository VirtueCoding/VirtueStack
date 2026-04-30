package network

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

const (
	abusePreventionTable = "virtuestack-abuse"
)

// AbusePreventionManager manages nftables rules for abuse prevention.
// It blocks outbound SMTP (port 25) and cloud metadata endpoint (169.254.169.254)
// traffic on VM tap interfaces.
type AbusePreventionManager struct {
	logger *slog.Logger
}

// NewAbusePreventionManager creates a new AbusePreventionManager.
func NewAbusePreventionManager(logger *slog.Logger) *AbusePreventionManager {
	return &AbusePreventionManager{
		logger: logger.With("component", "abuse-prevention"),
	}
}

// ApplyVMRules applies abuse prevention nftables rules for a VM's tap interface.
// Rules applied:
//   - Block outbound TCP port 25 (SMTP) to prevent spam
//   - Block outbound to 169.254.169.254/32 (cloud metadata endpoint) to prevent SSRF
func (m *AbusePreventionManager) ApplyVMRules(ctx context.Context, tapInterface string) error {
	if !validIfaceRegex.MatchString(tapInterface) {
		return fmt.Errorf("invalid tap interface name: %s", tapInterface)
	}

	logger := m.logger.With("interface", tapInterface)
	logger.Info("applying abuse prevention rules")

	if err := m.ensureTable(ctx); err != nil {
		return fmt.Errorf("ensuring nftables table: %w", err)
	}

	if err := m.ensureChain(ctx, tapInterface); err != nil {
		return fmt.Errorf("ensuring nftables chain: %w", err)
	}

	if err := m.addSMTPBlock(ctx, tapInterface); err != nil {
		logger.Warn("failed to add SMTP block rule", "error", err)
	}

	if err := m.addMetadataBlock(ctx, tapInterface); err != nil {
		logger.Warn("failed to add metadata block rule", "error", err)
	}

	logger.Info("abuse prevention rules applied")
	return nil
}

// RemoveVMRules removes all abuse prevention nftables rules for a VM.
func (m *AbusePreventionManager) RemoveVMRules(ctx context.Context, tapInterface string) error {
	if !validIfaceRegex.MatchString(tapInterface) {
		return fmt.Errorf("invalid tap interface name: %s", tapInterface)
	}

	logger := m.logger.With("interface", tapInterface)
	logger.Info("removing abuse prevention rules")

	if err := m.runNft(ctx, "delete chain inet %s %s", abusePreventionTable, tapInterface); err != nil {
		if strings.Contains(err.Error(), "No such file or directory") {
			logger.Debug("chain does not exist, nothing to remove")
			return nil
		}
		return fmt.Errorf("deleting nftables chain: %w", err)
	}

	logger.Info("abuse prevention rules removed")
	return nil
}

// ensureTable creates the nftables table if it does not exist.
func (m *AbusePreventionManager) ensureTable(ctx context.Context) error {
	exists, err := m.tableExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	return m.runNft(ctx, "add table inet %s", abusePreventionTable)
}

// ensureChain creates an input hook chain for the given tap interface.
func (m *AbusePreventionManager) ensureChain(ctx context.Context, tapInterface string) error {
	return m.runNft(ctx, "add chain inet %s %s { type filter hook forward priority 0 \\; policy accept \\; }",
		abusePreventionTable, tapInterface)
}

// smtpBlockRule returns the nft rule string that blocks outbound TCP port 25
// for VM-originated traffic entering the host on the given tap interface.
// The forward-chain match must use iifname (ingress on tap = packet leaving
// the VM); oifname would only match host->VM traffic and would not block
// VM-outbound spam.
func smtpBlockRule(tapInterface string) string {
	return fmt.Sprintf("add rule inet %s %s iifname %s tcp dport 25 drop",
		abusePreventionTable, tapInterface, tapInterface)
}

// metadataBlockRule returns the nft rule string that blocks VM-outbound
// traffic to the cloud metadata endpoint 169.254.169.254. As with
// smtpBlockRule, the forward-chain match keys on iifname (the VM's tap).
func metadataBlockRule(tapInterface string) string {
	return fmt.Sprintf("add rule inet %s %s iifname %s ip daddr 169.254.169.254 drop",
		abusePreventionTable, tapInterface, tapInterface)
}

// addSMTPBlock blocks outbound TCP port 25 on the tap interface.
func (m *AbusePreventionManager) addSMTPBlock(ctx context.Context, tapInterface string) error {
	return m.runNft(ctx, "%s", smtpBlockRule(tapInterface))
}

// addMetadataBlock blocks outbound traffic to 169.254.169.254/32 on the tap interface.
func (m *AbusePreventionManager) addMetadataBlock(ctx context.Context, tapInterface string) error {
	return m.runNft(ctx, "%s", metadataBlockRule(tapInterface))
}

// tableExists checks if the abuse prevention nftables table exists.
func (m *AbusePreventionManager) tableExists(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "nft", "list", "tables")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("listing nftables tables: %w, output: %s", err, string(output))
	}
	return strings.Contains(string(output), abusePreventionTable), nil
}

// runNft executes an nft command with the given format string.
func (m *AbusePreventionManager) runNft(ctx context.Context, format string, args ...any) error {
	rule := fmt.Sprintf(format, args...)
	m.logger.Debug("executing nft rule", "rule", rule)

	cmd := exec.CommandContext(ctx, "nft", strings.Fields(rule)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft %q: %w, output: %s", rule, err, string(output))
	}
	return nil
}
