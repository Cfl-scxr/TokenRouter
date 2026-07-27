//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBatchImageHoldFingerprintIgnoresMutableAllowanceState(t *testing.T) {
	command := &BatchImageBalanceHoldCommand{
		UserID:       1,
		ActorUserID:  2,
		APIKeyID:     3,
		BatchID:      "imgbatch_fingerprint",
		HoldAmount:   1,
		ActualAmount: 0.4,
		ReservedAt:   time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
	}
	command.Normalize()
	initialFingerprint := command.RequestFingerprint

	command.AllowanceReserved = true
	command.RequestFingerprint = ""
	command.Normalize()
	require.Equal(t, initialFingerprint, command.RequestFingerprint)
}
