// Package abuseprevention contains pure helpers for applying VM egress protections.
package abuseprevention

import (
	"context"
	"fmt"
)

// RuleOperations captures the concrete rule-application steps used by the node-agent.
type RuleOperations struct {
	EnsureTable      func(ctx context.Context) error
	EnsureChain      func(ctx context.Context, tapInterface string) error
	AddSMTPBlock     func(ctx context.Context, tapInterface string) error
	AddMetadataBlock func(ctx context.Context, tapInterface string) error
}

// ApplyVMRules applies abuse-prevention rules in order and fails closed on any step.
func ApplyVMRules(ctx context.Context, tapInterface string, ops RuleOperations) error {
	if err := ops.EnsureTable(ctx); err != nil {
		return fmt.Errorf("ensuring nftables table: %w", err)
	}
	if err := ops.EnsureChain(ctx, tapInterface); err != nil {
		return fmt.Errorf("ensuring nftables chain: %w", err)
	}
	if err := ops.AddSMTPBlock(ctx, tapInterface); err != nil {
		return fmt.Errorf("adding SMTP block rule: %w", err)
	}
	if err := ops.AddMetadataBlock(ctx, tapInterface); err != nil {
		return fmt.Errorf("adding metadata block rule: %w", err)
	}

	return nil
}
