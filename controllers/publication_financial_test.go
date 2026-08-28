package controllers

import (
	"errors"
	"testing"
)

func TestCalculatePublicationRequestAmounts(t *testing.T) {
	tests := []struct {
		name              string
		hasReceivedReward bool
		configuredReward  float64
		revisionFee       float64
		publicationFee    float64
		externalFunding   float64
		wantReward        float64
		wantTotal         float64
		wantErr           error
	}{
		{
			name:             "includes configured reward for a new reward request",
			configuredReward: 10000,
			revisionFee:      2000,
			publicationFee:   3000,
			externalFunding:  1000,
			wantReward:       10000,
			wantTotal:        14000,
		},
		{
			name:              "excludes reward after it was previously requested",
			hasReceivedReward: true,
			configuredReward:  10000,
			revisionFee:       2000,
			publicationFee:    3000,
			externalFunding:   1000,
			wantReward:        0,
			wantTotal:         4000,
		},
		{
			name:              "requires a positive revision fee for a previous reward",
			hasReceivedReward: true,
			configuredReward:  10000,
			wantErr:           errRevisionFeeRequired,
		},
		{
			name:            "rejects a negative total",
			revisionFee:     1000,
			externalFunding: 2000,
			wantErr:         errTotalAmountNonNegative,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reward, total, err := calculatePublicationRequestAmounts(
				tt.hasReceivedReward,
				tt.configuredReward,
				tt.revisionFee,
				tt.publicationFee,
				tt.externalFunding,
				true,
			)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if reward != tt.wantReward {
				t.Fatalf("reward = %v, want %v", reward, tt.wantReward)
			}
			if total != tt.wantTotal {
				t.Fatalf("total = %v, want %v", total, tt.wantTotal)
			}
		})
	}
}
