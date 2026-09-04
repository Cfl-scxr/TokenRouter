package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelMarketplaceServiceGetRoutingHealthSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "public-health.json")
	payload := `{
  "schemaVersion": 1,
  "state": "observed",
  "observedAt": "2026-09-04T12:39:31.675Z",
  "routingChainId": "tokenrouter-primary",
  "providers": [{
    "supplierName": "FastLYAI",
    "names": {"group": "fastlyai", "account": "fastlyai", "key": "fastlyai"},
    "manual": {"enabled": true, "groupEnabled": true, "accountEnabled": true, "accountSchedulable": true, "keyEnabled": true},
    "schedulable": true,
    "routeState": "available",
    "healthLevel": "healthy",
    "healthScore": 95,
    "business": {"total": 3, "success": 3, "successRate": 1},
    "health": {"lastLatencyMs": 12.80328799970448, "consecutiveFailures": 0, "cooling": false, "warming": false},
    "scheduledTest": {"kind": "scheduled_test", "result": "success", "observedAt": "2026-09-04T12:38:03.028Z", "latencyMs": 2988}
  }],
  "futureInternalField": "must-not-leak"
}`
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	t.Setenv(marketplaceRoutingHealthFileEnv, path)

	snapshot, err := (&ModelMarketplaceService{}).GetRoutingHealthSnapshot()
	require.NoError(t, err)
	require.True(t, snapshot.Available)
	require.Equal(t, "tokenrouter-primary", snapshot.RoutingChainID)
	require.Len(t, snapshot.Providers, 1)
	require.Equal(t, "fastlyai", snapshot.Providers[0].Names.Group)
	require.InDelta(t, 12.80328799970448, *snapshot.Providers[0].Health.LastLatencyMs, 0.0000001)
	require.EqualValues(t, 2988, *snapshot.Providers[0].ScheduledTest.LatencyMs)
}

func TestModelMarketplaceServiceGetRoutingHealthSnapshotRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "unsupported schema", payload: `{"schemaVersion":2,"routingChainId":"primary","providers":[]}`},
		{name: "missing chain", payload: `{"schemaVersion":1,"providers":[]}`},
		{name: "missing group", payload: `{"schemaVersion":1,"routingChainId":"primary","providers":[{"names":{}}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "public-health.json")
			require.NoError(t, os.WriteFile(path, []byte(tt.payload), 0o600))
			t.Setenv(marketplaceRoutingHealthFileEnv, path)

			_, err := (&ModelMarketplaceService{}).GetRoutingHealthSnapshot()
			require.Error(t, err)
		})
	}
}

func TestModelMarketplaceServiceGetRoutingHealthSnapshotRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "public-health.json")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", maxMarketplaceRoutingHealthFileSize+1)), 0o600))
	t.Setenv(marketplaceRoutingHealthFileEnv, path)

	_, err := (&ModelMarketplaceService{}).GetRoutingHealthSnapshot()
	require.ErrorContains(t, err, "size limit")
}
