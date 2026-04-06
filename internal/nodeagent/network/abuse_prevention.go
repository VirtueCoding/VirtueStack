package network

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/abuseprevention"
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

	if err := abuseprevention.ApplyVMRules(ctx, tapInterface, abuseprevention.RuleOperations{
		EnsureTable:      m.ensureTable,
		EnsureChain:      m.ensureChain,
		AddSMTPBlock:     m.addSMTPBlock,
		AddMetadataBlock: m.addMetadataBlock,
	}); err != nil {
		logger.Warn("failed to apply abuse prevention rules", "error", err)
		return err
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

// addSMTPBlock blocks outbound TCP port 25 on the tap interface.
func (m *AbusePreventionManager) addSMTPBlock(ctx context.Context, tapInterface string) error {
	return m.runNft(ctx,
		"add rule inet %s %s oifname %s tcp dport 25 drop",
		abusePreventionTable, tapInterface, tapInterface)
}

// addMetadataBlock blocks outbound traffic to 169.254.169.254/32 on the tap interface.
func (m *AbusePreventionManager) addMetadataBlock(ctx context.Context, tapInterface string) error {
	return m.runNft(ctx,
		"add rule inet %s %s oifname %s ip daddr 169.254.169.254 drop",
		abusePreventionTable, tapInterface, tapInterface)
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
