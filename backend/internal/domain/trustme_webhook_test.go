package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTrustMeWebhookEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		contractID string
		status     int
		wantEvent  TrustMeWebhookEvent
		wantErr    error
	}{
		{
			name:       "valid status=0",
			contractID: "doc-1",
			status:     0,
			wantEvent:  TrustMeWebhookEvent{ContractID: "doc-1", Status: 0},
		},
		{
			name:       "valid status=3 signed",
			contractID: "doc-1",
			status:     3,
			wantEvent:  TrustMeWebhookEvent{ContractID: "doc-1", Status: 3},
		},
		{
			name:       "valid status=9 signing_declined",
			contractID: "doc-1",
			status:     9,
			wantEvent:  TrustMeWebhookEvent{ContractID: "doc-1", Status: 9},
		},
		{
			name:       "trims whitespace around contract_id",
			contractID: "  doc-1  ",
			status:     2,
			wantEvent:  TrustMeWebhookEvent{ContractID: "doc-1", Status: 2},
		},
		{
			name:       "empty contract_id rejected",
			contractID: "",
			status:     3,
			wantErr:    ErrContractWebhookUnknownDocument,
		},
		{
			name:       "whitespace-only contract_id rejected",
			contractID: "   ",
			status:     3,
			wantErr:    ErrContractWebhookUnknownDocument,
		},
		{
			name:       "negative status rejected",
			contractID: "doc-1",
			status:     -1,
			wantErr:    ErrContractWebhookInvalidStatus,
		},
		{
			name:       "status 10 rejected",
			contractID: "doc-1",
			status:     10,
			wantErr:    ErrContractWebhookInvalidStatus,
		},
		{
			name:       "status 100 rejected",
			contractID: "doc-1",
			status:     100,
			wantErr:    ErrContractWebhookInvalidStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewTrustMeWebhookEvent(tt.contractID, tt.status)
			if tt.wantErr != nil {
				require.Error(t, err)
				require.True(t, errors.Is(err, tt.wantErr), "expected %v, got %v", tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantEvent, got)
		})
	}
}
