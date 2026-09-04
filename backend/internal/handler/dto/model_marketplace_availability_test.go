package dto

import (
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/stretchr/testify/require"
)

func TestModelMarketplaceAvailabilityFromServiceIncludesLatestProbeFields(t *testing.T) {
	checkedAt := time.Date(2026, 9, 4, 9, 5, 0, 0, time.UTC)
	latencyMs := int64(1234)

	got := modelMarketplaceAvailabilityFromService(&service.GroupAvailabilitySummary{
		WindowDays:          1,
		BucketMinutes:       15,
		LastStatus:          service.GroupAvailabilityProbeStatusFailed,
		LastCheckedAt:       &checkedAt,
		LastLatencyMs:       &latencyMs,
		ConsecutiveFailures: 2,
	})

	require.NotNil(t, got)
	require.Equal(t, service.GroupAvailabilityProbeStatusFailed, got.LastStatus)
	require.Equal(t, checkedAt, *got.LastCheckedAt)
	require.EqualValues(t, latencyMs, *got.LastLatencyMs)
	require.EqualValues(t, 2, got.ConsecutiveFailures)
}
