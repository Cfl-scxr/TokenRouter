package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	marketplaceRoutingHealthFileEnv     = "ROUTING_PUBLIC_HEALTH_FILE"
	defaultMarketplaceRoutingHealthFile = "/run/wukong-model-routing/public-health.json"
	maxMarketplaceRoutingHealthFileSize = 1 << 20
)

// MarketplaceRoutingHealthSnapshot 是 Sidecar 输出到模型广场的脱敏综合健康快照。
type MarketplaceRoutingHealthSnapshot struct {
	Available      bool                                `json:"available"`
	SchemaVersion  int                                 `json:"schemaVersion"`
	State          string                              `json:"state"`
	ObservedAt     *time.Time                          `json:"observedAt"`
	RoutingChainID string                              `json:"routingChainId"`
	CurrentHit     *MarketplaceRoutingHealthCurrentHit `json:"currentHit,omitempty"`
	Providers      []MarketplaceRoutingHealthProvider  `json:"providers"`
}

type MarketplaceRoutingHealthCurrentHit struct {
	SupplierName string     `json:"supplierName"`
	Model        string     `json:"model"`
	ObservedAt   *time.Time `json:"observedAt"`
}

type MarketplaceRoutingHealthProvider struct {
	SupplierName      string                                     `json:"supplierName"`
	Names             MarketplaceRoutingHealthNames              `json:"names"`
	Manual            MarketplaceRoutingHealthManual             `json:"manual"`
	Schedulable       bool                                       `json:"schedulable"`
	RouteState        string                                     `json:"routeState"`
	HealthLevel       string                                     `json:"healthLevel"`
	HealthScore       *float64                                   `json:"healthScore"`
	Business          MarketplaceRoutingHealthBusiness           `json:"business"`
	Health            MarketplaceRoutingHealthDetail             `json:"health"`
	ScheduledTest     *MarketplaceRoutingHealthScheduledTest     `json:"scheduledTest,omitempty"`
	AvailabilityProbe *MarketplaceRoutingHealthAvailabilityProbe `json:"availabilityProbe,omitempty"`
}

type MarketplaceRoutingHealthNames struct {
	Group   string `json:"group"`
	Account string `json:"account"`
	Key     string `json:"key"`
}

type MarketplaceRoutingHealthManual struct {
	Enabled            bool `json:"enabled"`
	GroupEnabled       bool `json:"groupEnabled"`
	AccountEnabled     bool `json:"accountEnabled"`
	AccountSchedulable bool `json:"accountSchedulable"`
	KeyEnabled         bool `json:"keyEnabled"`
}

type MarketplaceRoutingHealthBusiness struct {
	Total          int64      `json:"total"`
	Success        int64      `json:"success"`
	SuccessRate    *float64   `json:"successRate"`
	LastObservedAt *time.Time `json:"lastObservedAt"`
}

type MarketplaceRoutingHealthDetail struct {
	LastSuccessAt       *time.Time `json:"lastSuccessAt"`
	LastFailureAt       *time.Time `json:"lastFailureAt"`
	LastLatencyMs       *float64   `json:"lastLatencyMs"`
	ConsecutiveFailures int64      `json:"consecutiveFailures"`
	Classification      *string    `json:"classification"`
	Cooling             bool       `json:"cooling"`
	CooldownUntil       *time.Time `json:"cooldownUntil"`
	Warming             bool       `json:"warming"`
	WarmupUntil         *time.Time `json:"warmupUntil"`
	BusinessSuccessEWMA *float64   `json:"businessSuccessEwma"`
}

type MarketplaceRoutingHealthScheduledTest struct {
	Kind                string     `json:"kind"`
	Result              string     `json:"result"`
	ObservedAt          *time.Time `json:"observedAt"`
	LatencyMs           *int64     `json:"latencyMs"`
	SuccessRate24h      *float64   `json:"successRate24h"`
	AverageLatencyMs24h *float64   `json:"averageLatencyMs24h"`
	P95LatencyMs24h     *float64   `json:"p95LatencyMs24h"`
	SampleCount24h      int64      `json:"sampleCount24h"`
}

type MarketplaceRoutingHealthAvailabilityProbe struct {
	Kind                 string     `json:"kind"`
	Result               string     `json:"result"`
	ObservedAt           *time.Time `json:"observedAt"`
	ConsecutiveSuccesses int64      `json:"consecutiveSuccesses"`
	NextProbeAt          *time.Time `json:"nextProbeAt"`
}

// UnavailableMarketplaceRoutingHealthSnapshot 返回稳定的降级响应，避免健康文件异常拖垮模型广场。
func UnavailableMarketplaceRoutingHealthSnapshot() *MarketplaceRoutingHealthSnapshot {
	return &MarketplaceRoutingHealthSnapshot{
		Available:     false,
		SchemaVersion: 1,
		State:         "unavailable",
		Providers:     []MarketplaceRoutingHealthProvider{},
	}
}

// @project-doc docs/interfaces/model_catalog_and_marketplace.md#routing_health_overview
// GetRoutingHealthSnapshot 只读取 Sidecar 脱敏快照，不触发任何上游探测或模型调用。
func (s *ModelMarketplaceService) GetRoutingHealthSnapshot() (*MarketplaceRoutingHealthSnapshot, error) {
	path := strings.TrimSpace(os.Getenv(marketplaceRoutingHealthFileEnv))
	if path == "" {
		path = defaultMarketplaceRoutingHealthFile
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open routing health snapshot: %w", err)
	}
	defer file.Close()

	payload, err := io.ReadAll(io.LimitReader(file, maxMarketplaceRoutingHealthFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read routing health snapshot: %w", err)
	}
	if len(payload) > maxMarketplaceRoutingHealthFileSize {
		return nil, errors.New("routing health snapshot exceeds size limit")
	}

	var snapshot MarketplaceRoutingHealthSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return nil, fmt.Errorf("decode routing health snapshot: %w", err)
	}
	if snapshot.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported routing health schema version: %d", snapshot.SchemaVersion)
	}
	if strings.TrimSpace(snapshot.RoutingChainID) == "" {
		return nil, errors.New("routing health snapshot is missing routing chain id")
	}
	if len(snapshot.Providers) > 100 {
		return nil, errors.New("routing health snapshot contains too many providers")
	}
	for i := range snapshot.Providers {
		if strings.TrimSpace(snapshot.Providers[i].Names.Group) == "" {
			return nil, fmt.Errorf("routing health provider %d is missing group name", i)
		}
	}

	snapshot.Available = true
	if snapshot.Providers == nil {
		snapshot.Providers = []MarketplaceRoutingHealthProvider{}
	}
	return &snapshot, nil
}
