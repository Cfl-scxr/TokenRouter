package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupAvailabilityProbeConsecutiveFailuresMigration(t *testing.T) {
	content, err := FS.ReadFile("265_group_availability_probe_consecutive_failures.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS consecutive_failures BIGINT NOT NULL DEFAULT 0")
	require.Contains(t, sql, "WITH latest_success AS")
	require.Contains(t, sql, "results.success = FALSE")
	require.Contains(t, sql, "results.started_at > success.started_at")
	require.Contains(t, sql, "SET consecutive_failures = failure_counts.consecutive_failures")
}
