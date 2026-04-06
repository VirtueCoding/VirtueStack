package abuseprevention

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyVMRules_FailsClosedWhenRuleInstallFails(t *testing.T) {
	tests := []struct {
		name      string
		wantErr   string
		failStep  string
		wantCalls []string
	}{
		{
			name:     "smtp block failure",
			wantErr:  "adding SMTP block rule",
			failStep: "smtp",
			wantCalls: []string{
				"ensure-table",
				"ensure-chain:vnet0",
				"add-smtp:vnet0",
			},
		},
		{
			name:     "metadata block failure",
			wantErr:  "adding metadata block rule",
			failStep: "metadata",
			wantCalls: []string{
				"ensure-table",
				"ensure-chain:vnet0",
				"add-smtp:vnet0",
				"add-metadata:vnet0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]string, 0, len(tt.wantCalls))
			ops := RuleOperations{
				EnsureTable: func(context.Context) error {
					calls = append(calls, "ensure-table")
					return nil
				},
				EnsureChain: func(context.Context, string) error {
					calls = append(calls, "ensure-chain:vnet0")
					return nil
				},
				AddSMTPBlock: func(context.Context, string) error {
					calls = append(calls, "add-smtp:vnet0")
					if tt.failStep == "smtp" {
						return errors.New("smtp rule failed")
					}
					return nil
				},
				AddMetadataBlock: func(context.Context, string) error {
					calls = append(calls, "add-metadata:vnet0")
					if tt.failStep == "metadata" {
						return errors.New("metadata rule failed")
					}
					return nil
				},
			}

			err := ApplyVMRules(context.Background(), "vnet0", ops)

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.Equal(t, tt.wantCalls, calls)
		})
	}
}
