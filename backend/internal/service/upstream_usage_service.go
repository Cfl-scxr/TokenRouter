package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/errors"
	"github.com/TokenFlux/TokenRouter/internal/pkg/tlsfingerprint"
	"github.com/TokenFlux/TokenRouter/internal/util/urlvalidator"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

const (
	// UpstreamUsageQueryExtraKey 是 API Key 账号用量查询的持久化配置键。
	UpstreamUsageQueryExtraKey = "upstream_usage_query"

	UpstreamUsageAdapterSub2API = "sub2api"
	UpstreamUsageAdapterNewAPI  = "new_api"

	upstreamUsageDefaultAdapter = UpstreamUsageAdapterSub2API
	upstreamUsageMaxBodyBytes   = 512 * 1024
	upstreamUsageTimeout        = 10 * time.Second
	upstreamUsageStatusTimeout  = 2 * time.Second
	upstreamUsageBatchLimit     = 100
	upstreamUsageConcurrency    = 4
)

var (
	ErrUpstreamUsageUnavailable = infraerrors.ServiceUnavailable(
		"UPSTREAM_USAGE_UNAVAILABLE", "upstream usage query service is unavailable",
	)
	ErrUpstreamUsageAccountInvalid = infraerrors.BadRequest(
		"UPSTREAM_USAGE_ACCOUNT_INVALID", "account is not a supported API key account",
	)
	ErrUpstreamUsageAccountDisabled = infraerrors.New(http.StatusUnprocessableEntity,
		"UPSTREAM_USAGE_ACCOUNT_DISABLED", "account is disabled",
	)
	ErrUpstreamUsageDisabled = infraerrors.New(http.StatusUnprocessableEntity,
		"UPSTREAM_USAGE_DISABLED", "upstream usage query is disabled for this account",
	)
	ErrUpstreamUsageUnsupported = infraerrors.New(http.StatusUnprocessableEntity,
		"UPSTREAM_USAGE_ADAPTER_UNSUPPORTED", "upstream usage adapter is unsupported",
	)
	ErrUpstreamUsageAuthFailed = infraerrors.New(http.StatusBadGateway,
		"UPSTREAM_USAGE_AUTH_FAILED", "upstream rejected the account API key",
	)
	ErrUpstreamUsageRateLimited = infraerrors.ServiceUnavailable(
		"UPSTREAM_USAGE_RATE_LIMITED", "upstream usage query was rate limited",
	)
	ErrUpstreamUsageTimeout = infraerrors.GatewayTimeout(
		"UPSTREAM_USAGE_TIMEOUT", "upstream usage query timed out",
	)
	ErrUpstreamUsageInvalidResponse = infraerrors.New(http.StatusBadGateway,
		"UPSTREAM_USAGE_INVALID_RESPONSE", "upstream returned an invalid usage response",
	)
	ErrUpstreamUsageRequestFailed = infraerrors.New(http.StatusBadGateway,
		"UPSTREAM_USAGE_REQUEST_FAILED", "upstream usage request failed",
	)
	ErrUpstreamUsageIdentityChanged = infraerrors.Conflict(
		"UPSTREAM_USAGE_IDENTITY_CHANGED", "account credentials or connection settings changed during the query",
	)
	ErrUpstreamUsageConfigInvalid = infraerrors.BadRequest(
		"UPSTREAM_USAGE_CONFIG_INVALID", "upstream usage query configuration is invalid",
	)
	ErrUpstreamUsageBatchInvalid = infraerrors.BadRequest(
		"UPSTREAM_USAGE_BATCH_INVALID", "upstream usage batch request is invalid",
	)
	ErrUpstreamUsageBatchTooLarge = infraerrors.BadRequest(
		"UPSTREAM_USAGE_BATCH_TOO_LARGE", "too many accounts in one upstream usage query",
	)
)

// UpstreamUsageQueryConfig 是账号 Extra 中公开给管理员的非敏感查询配置。
type UpstreamUsageQueryConfig struct {
	Enabled bool   `json:"enabled"`
	Adapter string `json:"adapter"`
	BaseURL string `json:"base_url,omitempty"`
}

// UpstreamUsageAmount 表示余额或累计限额的三个可选维度。
type UpstreamUsageAmount struct {
	Used      *float64 `json:"used,omitempty"`
	Total     *float64 `json:"total,omitempty"`
	Remaining *float64 `json:"remaining,omitempty"`
}

// UpstreamUsageLimit 表示上游返回的某个周期限额，不使用 OAuth 的窗口命名。
type UpstreamUsageLimit struct {
	Name      string     `json:"name"`
	Used      *float64   `json:"used,omitempty"`
	Limit     *float64   `json:"limit,omitempty"`
	Remaining *float64   `json:"remaining,omitempty"`
	ResetAt   *time.Time `json:"reset_at,omitempty"`
}

// UpstreamUsageSubscription 表示订阅余额和订阅周期限额。
type UpstreamUsageSubscription struct {
	PlanName  string               `json:"plan_name"`
	Unlimited bool                 `json:"unlimited,omitempty"`
	Remaining *float64             `json:"remaining,omitempty"`
	ExpiresAt *time.Time           `json:"expires_at,omitempty"`
	Limits    []UpstreamUsageLimit `json:"limits,omitempty"`
}

// UpstreamUsageInfo 是适配器归一化后的上游用量模型。
type UpstreamUsageInfo struct {
	Provider     string                     `json:"provider"`
	Mode         string                     `json:"mode"`
	Unit         string                     `json:"unit,omitempty"`
	Balance      *UpstreamUsageAmount       `json:"balance,omitempty"`
	Limits       []UpstreamUsageLimit       `json:"limits,omitempty"`
	Subscription *UpstreamUsageSubscription `json:"subscription,omitempty"`
	ExpiresAt    *time.Time                 `json:"expires_at,omitempty"`
}

// UpstreamUsageQueryResult 是管理员查询接口的成功响应。
type UpstreamUsageQueryResult struct {
	AccountID    int64                      `json:"account_id"`
	Adapter      string                     `json:"adapter"`
	ObservedAt   time.Time                  `json:"observed_at"`
	Provider     string                     `json:"provider,omitempty"`
	Mode         string                     `json:"mode,omitempty"`
	Unit         string                     `json:"unit,omitempty"`
	Balance      *UpstreamUsageAmount       `json:"balance,omitempty"`
	Limits       []UpstreamUsageLimit       `json:"limits,omitempty"`
	Subscription *UpstreamUsageSubscription `json:"subscription,omitempty"`
	ExpiresAt    *time.Time                 `json:"expires_at,omitempty"`
	// Usage 仅供服务内部复用归一化对象，不暴露到管理员响应，避免把协议内部模型
	// 再套一层 API Key 的“窗口”语义。
	Usage *UpstreamUsageInfo `json:"-"`
}

// UpstreamUsageMetrics 是进程内的查询计数快照；只保存适配器和错误分类，不保存凭据或响应内容。
type UpstreamUsageMetrics struct {
	Counts map[string]int64 `json:"counts"`
}

// UpstreamUsageAdapter 描述一个完整的上游请求与响应协议。
// 适配器拥有请求格式和解析规则，管理员配置只选择适配器和查询根地址。
type UpstreamUsageAdapter interface {
	Name() string
	Query(ctx context.Context, client *upstreamUsageHTTPClient) (*UpstreamUsageInfo, error)
}

// UpstreamUsageAdapterOption 用于前端或诊断页面展示可用适配器。
type UpstreamUsageAdapterOption struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

type upstreamUsageAdapterRegistration struct {
	Name    string
	Label   string
	Factory func() UpstreamUsageAdapter
}

// upstreamUsageAdapterRegistry 是协议实现的唯一注册表。
// 新协议在此登记工厂后即可参与配置校验和服务初始化。
var upstreamUsageAdapterRegistry = []upstreamUsageAdapterRegistration{
	{Name: UpstreamUsageAdapterSub2API, Label: "Sub2API / TokenRouter", Factory: func() UpstreamUsageAdapter { return &sub2APIUsageAdapter{} }},
	{Name: UpstreamUsageAdapterNewAPI, Label: "New API", Factory: func() UpstreamUsageAdapter { return &newAPIUsageAdapter{} }},
}

// UpstreamUsageAdapterOptions 返回稳定排序的内置适配器列表。
func UpstreamUsageAdapterOptions() []UpstreamUsageAdapterOption {
	options := make([]UpstreamUsageAdapterOption, 0, len(upstreamUsageAdapterRegistry))
	for _, registration := range upstreamUsageAdapterRegistry {
		options = append(options, UpstreamUsageAdapterOption{Name: registration.Name, Label: registration.Label})
	}
	return options
}

// UpstreamUsageService 负责 API Key 上游用量查询；结果只在当前请求中存在。
type UpstreamUsageService struct {
	accountRepo         AccountRepository
	httpUpstream        HTTPUpstream
	cfg                 *config.Config
	tlsFPProfileService *TLSFingerprintProfileService
	adapters            map[string]UpstreamUsageAdapter
	queryFlight         singleflight.Group
	querySlots          chan struct{}
	now                 func() time.Time
	adapterMu           sync.RWMutex
	metricsMu           sync.Mutex
	metrics             map[string]int64
}

// NewUpstreamUsageService 创建上游用量查询服务。
func NewUpstreamUsageService(
	accountRepo AccountRepository,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
	tlsFPProfileService *TLSFingerprintProfileService,
) *UpstreamUsageService {
	service := &UpstreamUsageService{
		accountRepo:         accountRepo,
		httpUpstream:        httpUpstream,
		cfg:                 cfg,
		tlsFPProfileService: tlsFPProfileService,
		adapters:            make(map[string]UpstreamUsageAdapter),
		querySlots:          make(chan struct{}, upstreamUsageConcurrency),
		now:                 time.Now,
		metrics:             make(map[string]int64),
	}
	for _, registration := range upstreamUsageAdapterRegistry {
		service.RegisterAdapter(registration.Factory())
	}
	return service
}

// RegisterAdapter 注册一个内置适配器，重复名称会覆盖旧实现以便测试替换。
func (s *UpstreamUsageService) RegisterAdapter(adapter UpstreamUsageAdapter) {
	if s == nil || adapter == nil || strings.TrimSpace(adapter.Name()) == "" {
		return
	}
	s.adapterMu.Lock()
	defer s.adapterMu.Unlock()
	if s.adapters == nil {
		s.adapters = make(map[string]UpstreamUsageAdapter)
	}
	s.adapters[strings.TrimSpace(adapter.Name())] = adapter
}

func (s *UpstreamUsageService) recordMetric(adapter, outcome string) {
	if s == nil {
		return
	}
	adapter = strings.TrimSpace(adapter)
	if adapter == "" {
		adapter = "unknown"
	}
	outcome = strings.TrimSpace(outcome)
	if outcome == "" {
		outcome = "unknown"
	}
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	if s.metrics == nil {
		s.metrics = make(map[string]int64)
	}
	s.metrics[adapter+":"+outcome]++
}

// SnapshotMetrics 返回脱敏的适配器/结果分类计数，供监控或测试读取。
func (s *UpstreamUsageService) SnapshotMetrics() UpstreamUsageMetrics {
	result := UpstreamUsageMetrics{Counts: make(map[string]int64)}
	if s == nil {
		return result
	}
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	for key, value := range s.metrics {
		result.Counts[key] = value
	}
	return result
}

// EffectiveUpstreamUsageConfig 解析账号的生效配置。缺少配置时使用安全的默认适配器。
func EffectiveUpstreamUsageConfig(account *Account) (UpstreamUsageQueryConfig, error) {
	config := UpstreamUsageQueryConfig{Enabled: true, Adapter: upstreamUsageDefaultAdapter}
	if account == nil || account.Extra == nil {
		return config, nil
	}
	raw, exists := account.Extra[UpstreamUsageQueryExtraKey]
	if !exists || raw == nil {
		return config, nil
	}
	object, ok := upstreamUsageConfigMap(raw)
	if !ok {
		return UpstreamUsageQueryConfig{}, ErrUpstreamUsageConfigInvalid
	}
	if value, exists := object["enabled"]; exists {
		parsed, ok := value.(bool)
		if !ok {
			return UpstreamUsageQueryConfig{}, ErrUpstreamUsageConfigInvalid
		}
		config.Enabled = parsed
	}
	if value, exists := object["adapter"]; exists {
		parsed, ok := value.(string)
		if !ok || strings.TrimSpace(parsed) == "" {
			return UpstreamUsageQueryConfig{}, ErrUpstreamUsageConfigInvalid
		}
		config.Adapter = strings.TrimSpace(parsed)
	}
	if value, exists := object["base_url"]; exists {
		parsed, ok := value.(string)
		if !ok {
			return UpstreamUsageQueryConfig{}, ErrUpstreamUsageConfigInvalid
		}
		config.BaseURL = strings.TrimSpace(parsed)
	}
	for key := range object {
		if key != "enabled" && key != "adapter" && key != "base_url" {
			return UpstreamUsageQueryConfig{}, ErrUpstreamUsageConfigInvalid
		}
	}
	if !isKnownUpstreamUsageAdapter(config.Adapter) {
		return UpstreamUsageQueryConfig{}, ErrUpstreamUsageUnsupported
	}
	if config.BaseURL != "" {
		if err := validateUsageBaseURLFormat(config.BaseURL); err != nil {
			return UpstreamUsageQueryConfig{}, ErrUpstreamUsageConfigInvalid.WithCause(err)
		}
	}
	return config, nil
}

// NormalizeUpstreamUsageExtra 校验并规范化创建/更新请求中的查询配置。
func NormalizeUpstreamUsageExtra(extra map[string]any) error {
	if extra == nil {
		return nil
	}
	raw, exists := extra[UpstreamUsageQueryExtraKey]
	if !exists || raw == nil {
		return nil
	}
	object, ok := upstreamUsageConfigMap(raw)
	if !ok {
		return ErrUpstreamUsageConfigInvalid
	}
	normalized := map[string]any{
		"enabled": true,
		"adapter": upstreamUsageDefaultAdapter,
	}
	if value, exists := object["enabled"]; exists {
		parsed, ok := value.(bool)
		if !ok {
			return ErrUpstreamUsageConfigInvalid
		}
		normalized["enabled"] = parsed
	}
	if value, exists := object["adapter"]; exists {
		parsed, ok := value.(string)
		if !ok || !isKnownUpstreamUsageAdapter(strings.TrimSpace(parsed)) {
			return ErrUpstreamUsageConfigInvalid
		}
		normalized["adapter"] = strings.TrimSpace(parsed)
	}
	if value, exists := object["base_url"]; exists {
		parsed, ok := value.(string)
		if !ok {
			return ErrUpstreamUsageConfigInvalid
		}
		parsed = strings.TrimSpace(parsed)
		if parsed != "" {
			if err := validateUsageBaseURLFormat(parsed); err != nil {
				return ErrUpstreamUsageConfigInvalid.WithCause(err)
			}
			normalized["base_url"] = parsed
		}
	}
	// 不接受任何可能把凭据或任意请求模板带入 Extra 的字段。
	for key := range object {
		if key != "enabled" && key != "adapter" && key != "base_url" {
			return ErrUpstreamUsageConfigInvalid
		}
	}
	extra[UpstreamUsageQueryExtraKey] = normalized
	return nil
}

// normalizedUpstreamUsageConfigValue 复制并规范化已有配置。
// 旧记录若含敏感字段或任意请求模板，不得在一次无关编辑中被重新写回。
func normalizedUpstreamUsageConfigValue(value any) (any, bool) {
	if value == nil {
		return nil, false
	}
	extra := map[string]any{UpstreamUsageQueryExtraKey: value}
	if err := NormalizeUpstreamUsageExtra(extra); err != nil {
		return nil, false
	}
	normalized, ok := extra[UpstreamUsageQueryExtraKey]
	return normalized, ok
}

func upstreamUsageConfigMap(raw any) (map[string]any, bool) {
	if object, ok := raw.(map[string]any); ok {
		return object, true
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, false
	}
	return object, object != nil
}

func isKnownUpstreamUsageAdapter(name string) bool {
	for _, registration := range upstreamUsageAdapterRegistry {
		if registration.Name == name {
			return true
		}
	}
	return false
}

func validateUsageBaseURLFormat(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("invalid base URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("base URL must not contain credentials, query, or fragment")
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return errors.New("base URL scheme is not supported")
	}
	if len(raw) > 2048 {
		return errors.New("base URL is too long")
	}
	return nil
}

// QueryAccount 查询单个账号的实时上游用量。
func (s *UpstreamUsageService) QueryAccount(ctx context.Context, accountID int64) (*UpstreamUsageQueryResult, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, ErrUpstreamUsageUnavailable
	}
	if accountID <= 0 {
		return nil, ErrUpstreamUsageAccountInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	queryDeadline := time.Now().Add(upstreamUsageTimeout)
	preflightCtx, cancelPreflight := context.WithDeadline(ctx, queryDeadline)
	defer cancelPreflight()
	// 先读取一次身份快照，用它生成 singleflight 指纹。这样凭据、代理或
	// 查询配置发生变化时，不会把新请求错误地合并到旧请求中。
	account, err := s.loadQueryAccount(preflightCtx, accountID)
	if err != nil {
		return nil, err
	}
	queryConfig, err := EffectiveUpstreamUsageConfig(account)
	if err != nil {
		return nil, err
	}
	if !queryConfig.Enabled {
		return nil, ErrUpstreamUsageDisabled
	}
	if s.adapter(queryConfig.Adapter) == nil {
		return nil, ErrUpstreamUsageUnsupported
	}
	fingerprint := upstreamUsageContextFingerprint(account, queryConfig)
	key := fmt.Sprintf("%d:%s", accountID, fingerprint)
	resultCh := s.queryFlight.DoChan(key, func() (any, error) {
		// 共享操作保留首个调用方的值，但不继承其取消信号；固定截止时间
		// 则把首次身份读取也计入约 10 秒的总预算。
		opCtx, cancel := context.WithDeadline(context.WithoutCancel(ctx), queryDeadline)
		defer cancel()
		return s.queryAccount(opCtx, accountID, account, queryConfig)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		queryResult, ok := result.Val.(*UpstreamUsageQueryResult)
		if !ok || queryResult == nil {
			return nil, ErrUpstreamUsageInvalidResponse
		}
		return queryResult, nil
	}
}

// QueryBatch 查询多个账号；单账号错误通过 errors 返回，不中断其它账号。
func (s *UpstreamUsageService) QueryBatch(ctx context.Context, accountIDs []int64) (map[int64]*UpstreamUsageQueryResult, map[int64]error, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, nil, ErrUpstreamUsageUnavailable
	}
	if len(accountIDs) == 0 {
		return nil, nil, ErrUpstreamUsageBatchInvalid
	}
	if len(accountIDs) > upstreamUsageBatchLimit {
		return nil, nil, ErrUpstreamUsageBatchTooLarge
	}
	unique := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]struct{}, len(accountIDs))
	errorsByID := make(map[int64]error)
	for _, id := range accountIDs {
		if id <= 0 {
			errorsByID[id] = ErrUpstreamUsageAccountInvalid
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	results := make(map[int64]*UpstreamUsageQueryResult, len(unique))
	if len(unique) == 0 {
		return results, errorsByID, nil
	}
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(upstreamUsageConcurrency)
	for _, id := range unique {
		id := id
		group.Go(func() error {
			result, err := s.QueryAccount(groupCtx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errorsByID[id] = err
			} else {
				results[id] = result
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, nil, err
	}
	return results, errorsByID, nil
}

func (s *UpstreamUsageService) queryAccount(ctx context.Context, accountID int64, expected *Account, expectedConfig UpstreamUsageQueryConfig) (result *UpstreamUsageQueryResult, err error) {
	defer func() {
		outcome := "success"
		if err != nil {
			outcome = infraerrors.Reason(err)
			if outcome == "" {
				outcome = "error"
			}
		}
		s.recordMetric(expectedConfig.Adapter, outcome)
	}()
	release, err := s.acquireQuerySlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	account, err := s.loadQueryAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, ErrUpstreamUsageAccountInvalid) || errors.Is(err, ErrUpstreamUsageAccountDisabled) {
			// 预读后账号类型、状态或记录本身发生变化，应报告身份冲突，
			// 而不是把一次进行中的查询误报为普通账号参数错误。
			return nil, ErrUpstreamUsageIdentityChanged
		}
		return nil, err
	}
	if !sameUpstreamUsageIdentity(expected, account, expectedConfig) {
		return nil, ErrUpstreamUsageIdentityChanged
	}
	adapter := s.adapter(expectedConfig.Adapter)
	if adapter == nil {
		return nil, ErrUpstreamUsageUnsupported
	}
	client, err := s.newHTTPClient(account, expectedConfig)
	if err != nil {
		return nil, err
	}
	usage, err := adapter.Query(ctx, client)
	if err != nil {
		return nil, err
	}
	if err := validateNormalizedUsage(usage); err != nil {
		return nil, ErrUpstreamUsageInvalidResponse.WithCause(err)
	}
	current, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			// 查询开始后账号被删除也属于身份快照变化，不能把刚取得的结果
			// 归到一个已经不存在的账号上。
			return nil, ErrUpstreamUsageIdentityChanged
		}
		return nil, upstreamUsageRepositoryError(ctx, err)
	}
	if !sameUpstreamUsageIdentity(account, current, expectedConfig) {
		return nil, ErrUpstreamUsageIdentityChanged
	}
	now := time.Now().UTC()
	if s.now != nil {
		now = s.now().UTC()
	}
	return &UpstreamUsageQueryResult{
		AccountID:    accountID,
		Adapter:      expectedConfig.Adapter,
		ObservedAt:   now,
		Provider:     usage.Provider,
		Mode:         usage.Mode,
		Unit:         usage.Unit,
		Balance:      usage.Balance,
		Limits:       usage.Limits,
		Subscription: usage.Subscription,
		ExpiresAt:    usage.ExpiresAt,
		Usage:        usage,
	}, nil
}

func (s *UpstreamUsageService) loadQueryAccount(ctx context.Context, accountID int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, upstreamUsageRepositoryError(ctx, err)
	}
	if account == nil || account.Type != AccountTypeAPIKey {
		return nil, ErrUpstreamUsageAccountInvalid
	}
	if account.Status != "" && account.Status != StatusActive {
		return nil, ErrUpstreamUsageAccountDisabled
	}
	return account, nil
}

func upstreamUsageRepositoryError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return ErrUpstreamUsageTimeout
	}
	if errors.Is(err, ErrAccountNotFound) {
		return ErrUpstreamUsageAccountInvalid
	}
	return ErrUpstreamUsageRequestFailed
}

func (s *UpstreamUsageService) acquireQuerySlot(ctx context.Context) (func(), error) {
	s.adapterMu.Lock()
	if s.querySlots == nil {
		s.querySlots = make(chan struct{}, upstreamUsageConcurrency)
	}
	slots := s.querySlots
	s.adapterMu.Unlock()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, upstreamUsageContextError(ctx)
	}
}

func (s *UpstreamUsageService) adapter(name string) UpstreamUsageAdapter {
	s.adapterMu.RLock()
	defer s.adapterMu.RUnlock()
	return s.adapters[name]
}

type upstreamUsageHTTPClient struct {
	account    *Account
	upstream   HTTPUpstream
	baseURL    string
	apiKey     string
	proxyURL   string
	tlsProfile *tlsfingerprint.Profile
}

func (s *UpstreamUsageService) newHTTPClient(account *Account, queryConfig UpstreamUsageQueryConfig) (*upstreamUsageHTTPClient, error) {
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, ErrUpstreamUsageAccountInvalid
	}
	baseURL := strings.TrimSpace(queryConfig.BaseURL)
	if baseURL == "" {
		baseURL = upstreamUsageAccountBaseURL(account)
	}
	validated, err := s.validateBaseURL(baseURL)
	if err != nil {
		return nil, ErrUpstreamUsageConfigInvalid.WithCause(err)
	}
	proxyURL := ""
	if account.ProxyID != nil {
		if account.Proxy == nil {
			return nil, ErrUpstreamUsageRequestFailed
		}
		if account.Proxy.ID != *account.ProxyID {
			return nil, ErrUpstreamUsageIdentityChanged
		}
		proxyURL = account.Proxy.URL()
	}
	var profile *tlsfingerprint.Profile
	if s.tlsFPProfileService != nil {
		profile = s.tlsFPProfileService.ResolveTLSProfile(account)
	}
	return &upstreamUsageHTTPClient{
		account: account, upstream: s.httpUpstream, baseURL: validated,
		apiKey: apiKey, proxyURL: proxyURL, tlsProfile: profile,
	}, nil
}

func (s *UpstreamUsageService) validateBaseURL(raw string) (string, error) {
	if err := validateUsageBaseURLFormat(raw); err != nil {
		return "", err
	}
	if s.cfg == nil {
		// 缺少运行时配置时采用最严格的默认值，不能因为测试或降级构造
		// 绕过 HTTPS 和私网地址护栏。
		return urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{AllowPrivate: false})
	}
	if !s.cfg.Security.URLAllowlist.Enabled {
		return urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
	}
	return urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
		RequireAllowlist: true,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
}

func upstreamUsageAccountBaseURL(account *Account) string {
	if account == nil {
		return ""
	}
	switch account.Platform {
	case PlatformOpenAI:
		return account.GetOpenAIBaseURL()
	case PlatformAnthropic:
		return account.GetBaseURL()
	case PlatformGrok:
		return account.GetGrokBaseURL()
	case PlatformGemini:
		return account.GetGeminiBaseURL("https://generativelanguage.googleapis.com")
	case PlatformAntigravity:
		return account.GetGeminiBaseURL("https://generativelanguage.googleapis.com")
	default:
		// 未知平台没有平台专用的 URL 归一化规则，仍允许复用凭据中的根地址。
		return strings.TrimSpace(account.GetCredential("base_url"))
	}
}

func sameUpstreamUsageIdentity(expected, current *Account, expectedConfig UpstreamUsageQueryConfig) bool {
	if expected == nil || current == nil || expected.ID != current.ID || current.Type != AccountTypeAPIKey ||
		expected.Platform != current.Platform || !reflect.DeepEqual(expected.Credentials, current.Credentials) ||
		!sameUpstreamUsageOptionalInt64(expected.ProxyID, current.ProxyID) || expected.Concurrency != current.Concurrency ||
		expected.Status != current.Status {
		return false
	}
	if expected.ProxyID != nil && !sameUpstreamUsageProxy(expected.Proxy, current.Proxy, *expected.ProxyID) {
		// 仓储可能暂时没有预加载代理详情；两边都缺失时交给客户端构建阶段
		// 返回请求错误，避免把可诊断的配置缺失误报成身份冲突。
		if expected.Proxy != nil || current.Proxy != nil {
			return false
		}
	}
	for _, key := range []string{"enable_tls_fingerprint", "tls_fingerprint_profile_id", "tls_fingerprint_router_id"} {
		if !reflect.DeepEqual(extraValue(expected.Extra, key), extraValue(current.Extra, key)) {
			return false
		}
	}
	currentConfig, err := EffectiveUpstreamUsageConfig(current)
	if err != nil || currentConfig != expectedConfig {
		return false
	}
	return true
}

func sameUpstreamUsageOptionalInt64(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func sameUpstreamUsageProxy(expected, current *Proxy, id int64) bool {
	if expected == nil || current == nil || expected.ID != id || current.ID != id {
		return false
	}
	return expected.Protocol == current.Protocol && expected.Host == current.Host && expected.Port == current.Port &&
		expected.Username == current.Username && expected.Password == current.Password && expected.Status == current.Status
}

func extraValue(extra map[string]any, key string) any {
	if extra == nil {
		return nil
	}
	return extra[key]
}

func (c *upstreamUsageHTTPClient) get(ctx context.Context, path string, authenticated bool) ([]byte, int, error) {
	endpoint, err := upstreamUsageEndpoint(c.baseURL, path)
	if err != nil {
		return nil, 0, ErrUpstreamUsageConfigInvalid.WithCause(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, ErrUpstreamUsageRequestFailed
	}
	reqCtx := WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI)
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(reqCtx))
	req.Header.Set("Accept", "application/json")
	c.account.ApplyHeaderOverrides(req.Header)
	if authenticated {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	} else {
		// 状态接口不应携带 API Key，避免把凭据发送到可选诊断路径。
		req.Header.Del("Authorization")
	}
	resp, err := c.upstream.DoWithTLS(req, c.proxyURL, c.account.ID, c.account.Concurrency, c.tlsProfile)
	if err != nil {
		return nil, 0, upstreamUsageOperationError(ctx, err)
	}
	if resp == nil || resp.Body == nil {
		return nil, 0, ErrUpstreamUsageInvalidResponse
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, upstreamUsageMaxBodyBytes+1))
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, 0, upstreamUsageOperationError(ctx, readErr)
	}
	if int64(len(body)) > upstreamUsageMaxBodyBytes {
		return nil, 0, ErrUpstreamUsageInvalidResponse
	}
	return body, resp.StatusCode, nil
}

func upstreamUsageEndpoint(base, path string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid base URL")
	}
	// 账号 Base URL 沿用 OpenAI 兼容端点约定：/v1、/v4、/v1beta
	// 等版本段作为根路径，不能再重复拼接一个 /v1。复用现有端点
	// 构造器，确保用量查询和转发对同一类 Base URL 的解释一致。
	endpoint := buildOpenAIEndpointURL(base, path)
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || parsedEndpoint.Scheme == "" || parsedEndpoint.Host == "" {
		return "", errors.New("invalid endpoint URL")
	}
	// 调用方已经校验过 Base URL；这里再次清理非路径部分，避免未来新增
	// 调用路径时把历史查询串、片段或用户信息带到上游请求。
	parsedEndpoint.User = nil
	parsedEndpoint.RawQuery = ""
	parsedEndpoint.ForceQuery = false
	parsedEndpoint.Fragment = ""
	return strings.TrimRight(parsedEndpoint.String(), "/"), nil
}

func upstreamUsageStatusEndpoint(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid base URL")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if openAIBaseURLHasVersionSuffix(path) {
		if index := strings.LastIndex(path, "/"); index >= 0 {
			path = path[:index]
		} else {
			path = ""
		}
	}
	parsed.Path = strings.TrimRight(path, "/") + "/api/status"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.User = nil
	return strings.TrimRight(parsed.String(), "/"), nil
}

func upstreamUsageHTTPError(status int, unsupported bool) error {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return nil
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUpstreamUsageAuthFailed
	case http.StatusTooManyRequests:
		return ErrUpstreamUsageRateLimited
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		if unsupported {
			return ErrUpstreamUsageUnsupported
		}
	}
	return ErrUpstreamUsageInvalidResponse
}

func upstreamUsageOperationError(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrUpstreamUsageTimeout
	}
	return ErrUpstreamUsageRequestFailed
}

func upstreamUsageContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrUpstreamUsageTimeout
	}
	return ErrUpstreamUsageRequestFailed
}

func validateNormalizedUsage(usage *UpstreamUsageInfo) error {
	if usage == nil || strings.TrimSpace(usage.Provider) == "" || strings.TrimSpace(usage.Mode) == "" {
		return errors.New("missing normalized usage fields")
	}
	switch usage.Mode {
	case "balance", "quota", "limits", "subscription":
	default:
		return errors.New("unknown normalized usage mode")
	}
	switch usage.Mode {
	case "balance", "quota":
		if usage.Balance == nil {
			return errors.New("missing normalized balance")
		}
	case "limits":
		if len(usage.Limits) == 0 && (usage.Subscription == nil || len(usage.Subscription.Limits) == 0) {
			return errors.New("missing normalized limits")
		}
	case "subscription":
		if usage.Subscription == nil {
			return errors.New("missing normalized subscription")
		}
	}
	if usage.Unit != "" && usage.Unit != "USD" && usage.Unit != "CNY" && usage.Unit != "TOKENS" {
		return errors.New("unknown usage unit")
	}
	if usage.Balance != nil {
		if err := validateUsageAmount(usage.Balance); err != nil {
			return err
		}
	}
	if err := validateUsageLimits(usage.Limits); err != nil {
		return err
	}
	if usage.Subscription != nil {
		if strings.TrimSpace(usage.Subscription.PlanName) == "" {
			return errors.New("missing subscription plan")
		}
		if usage.Subscription.Unlimited && (usage.Subscription.Remaining != nil || len(usage.Subscription.Limits) > 0) {
			return errors.New("unlimited subscription must not contain remaining or limits")
		}
		if !usage.Subscription.Unlimited && usage.Subscription.Remaining == nil && len(usage.Subscription.Limits) == 0 {
			return errors.New("limited subscription is missing remaining or limits")
		}
		if usage.Subscription.Remaining != nil && !validFiniteNumber(*usage.Subscription.Remaining) {
			return errors.New("invalid subscription remaining")
		}
		if usage.Subscription.ExpiresAt != nil && usage.Subscription.ExpiresAt.IsZero() {
			return errors.New("invalid subscription expiry")
		}
		if err := validateUsageLimits(usage.Subscription.Limits); err != nil {
			return err
		}
	}
	if usage.ExpiresAt != nil && usage.ExpiresAt.IsZero() {
		return errors.New("invalid expiry")
	}
	return nil
}

func validateUsageAmount(amount *UpstreamUsageAmount) error {
	if amount == nil || (amount.Used == nil && amount.Total == nil && amount.Remaining == nil) {
		return errors.New("missing usage amount values")
	}
	for _, value := range []*float64{amount.Used, amount.Total} {
		if value != nil && !validNonNegativeNumber(*value) {
			return errors.New("invalid usage amount")
		}
	}
	if amount.Remaining != nil && !validFiniteNumber(*amount.Remaining) {
		return errors.New("invalid usage remaining")
	}
	// New API 允许累计使用量超过硬限制，负的 remaining 需要原样展示。
	return nil
}

func validateUsageLimits(limits []UpstreamUsageLimit) error {
	seen := make(map[string]struct{}, len(limits))
	for _, limit := range limits {
		name := strings.TrimSpace(limit.Name)
		if name == "" {
			return errors.New("missing usage limit name")
		}
		if _, exists := seen[name]; exists {
			return errors.New("duplicate usage limit")
		}
		seen[name] = struct{}{}
		for _, value := range []*float64{limit.Used, limit.Limit} {
			if value != nil && !validNonNegativeNumber(*value) {
				return errors.New("invalid usage limit amount")
			}
		}
		if limit.Used == nil && limit.Limit == nil && limit.Remaining == nil {
			return errors.New("missing usage limit values")
		}
		if limit.Remaining != nil && !validFiniteNumber(*limit.Remaining) {
			return errors.New("invalid usage limit remaining")
		}
		if limit.ResetAt != nil && limit.ResetAt.IsZero() {
			return errors.New("invalid usage limit reset")
		}
	}
	return nil
}

func validNonNegativeNumber(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validFiniteNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// --- Sub2API 适配器 ---

type sub2APIUsageAdapter struct{}

func (*sub2APIUsageAdapter) Name() string { return UpstreamUsageAdapterSub2API }

type sub2APIUsageResponse struct {
	Mode         string               `json:"mode"`
	IsValid      *bool                `json:"isValid"`
	Status       string               `json:"status"`
	PlanName     string               `json:"planName"`
	Unit         string               `json:"unit"`
	Remaining    *float64             `json:"remaining"`
	Balance      *float64             `json:"balance"`
	Quota        *sub2APIQuota        `json:"quota"`
	RateLimits   []sub2APIRateLimit   `json:"rate_limits"`
	Subscription *sub2APISubscription `json:"subscription"`
	ExpiresAt    *time.Time           `json:"expires_at"`
}

type sub2APIQuota struct {
	Limit     *float64 `json:"limit"`
	Used      *float64 `json:"used"`
	Remaining *float64 `json:"remaining"`
	Unit      string   `json:"unit"`
}

type sub2APIRateLimit struct {
	Window      string          `json:"window"`
	Limit       *float64        `json:"limit"`
	Used        *float64        `json:"used"`
	Remaining   *float64        `json:"remaining"`
	WindowStart json.RawMessage `json:"window_start"`
	ResetAt     *time.Time      `json:"reset_at"`
}

type sub2APISubscription struct {
	DailyUsageUSD      *float64   `json:"daily_usage_usd"`
	WeeklyUsageUSD     *float64   `json:"weekly_usage_usd"`
	MonthlyUsageUSD    *float64   `json:"monthly_usage_usd"`
	DailyLimitUSD      *float64   `json:"daily_limit_usd"`
	WeeklyLimitUSD     *float64   `json:"weekly_limit_usd"`
	MonthlyLimitUSD    *float64   `json:"monthly_limit_usd"`
	DailyResetAt       *time.Time `json:"daily_reset_at"`
	WeeklyResetAt      *time.Time `json:"weekly_reset_at"`
	MonthlyResetAt     *time.Time `json:"monthly_reset_at"`
	DailyWindowStart   *time.Time `json:"daily_window_start"`
	WeeklyWindowStart  *time.Time `json:"weekly_window_start"`
	MonthlyWindowStart *time.Time `json:"monthly_window_start"`
	Unlimited          *bool      `json:"unlimited"`
	ExpiresAt          *time.Time `json:"expires_at"`
}

func (a *sub2APIUsageAdapter) Query(ctx context.Context, client *upstreamUsageHTTPClient) (*UpstreamUsageInfo, error) {
	body, status, err := client.get(ctx, "/v1/usage", true)
	if err != nil {
		return nil, err
	}
	if httpErr := upstreamUsageHTTPError(status, true); httpErr != nil {
		return nil, httpErr
	}
	return parseSub2APIUsage(body)
}

func parseSub2APIUsage(body []byte) (*UpstreamUsageInfo, error) {
	var response sub2APIUsageResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, ErrUpstreamUsageInvalidResponse.WithCause(err)
	}
	if response.IsValid == nil {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	if !*response.IsValid {
		return nil, ErrUpstreamUsageAuthFailed
	}
	switch response.Mode {
	case "quota_limited":
		return normalizeSub2APIQuotaLimited(&response)
	case "unrestricted":
		return normalizeSub2APIUnrestricted(&response)
	default:
		return nil, ErrUpstreamUsageInvalidResponse
	}
}

func normalizeSub2APIQuotaLimited(response *sub2APIUsageResponse) (*UpstreamUsageInfo, error) {
	if response.Status != "active" && response.Status != "quota_exhausted" && response.Status != "expired" {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	limits, err := normalizeSub2APIRateLimits(response.RateLimits)
	if err != nil {
		return nil, err
	}
	subscription, err := normalizeSub2APISubscription(response.PlanName, response.Subscription)
	if err != nil {
		return nil, err
	}
	if response.Quota == nil {
		if len(limits) == 0 || response.Remaining != nil || strings.TrimSpace(response.Unit) != "" {
			return nil, ErrUpstreamUsageInvalidResponse
		}
		expiresAt, err := normalizeTime(response.ExpiresAt)
		if err != nil {
			return nil, err
		}
		unit := ""
		if subscription != nil {
			unit = "USD"
		}
		return &UpstreamUsageInfo{Provider: UpstreamUsageAdapterSub2API, Mode: "limits", Unit: unit, Limits: limits, Subscription: subscription, ExpiresAt: expiresAt}, nil
	}
	quota := response.Quota
	if quota.Limit == nil || quota.Used == nil || quota.Remaining == nil || response.Remaining == nil ||
		quota.Unit != "USD" || response.Unit != quota.Unit || *quota.Limit <= 0 ||
		!validNonNegativeNumber(*quota.Limit) || !validNonNegativeNumber(*quota.Used) || !validNonNegativeNumber(*quota.Remaining) ||
		!validNonNegativeNumber(*response.Remaining) || !closeEnough(*quota.Remaining, math.Max(0, *quota.Limit-*quota.Used)) ||
		!closeEnough(*quota.Remaining, *response.Remaining) {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	expiresAt, err := normalizeTime(response.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &UpstreamUsageInfo{
		Provider: UpstreamUsageAdapterSub2API,
		Mode:     "quota",
		Unit:     quota.Unit,
		Balance:  &UpstreamUsageAmount{Used: quota.Used, Total: quota.Limit, Remaining: quota.Remaining},
		Limits:   limits, Subscription: subscription, ExpiresAt: expiresAt,
	}, nil
}

func normalizeSub2APIUnrestricted(response *sub2APIUsageResponse) (*UpstreamUsageInfo, error) {
	if response.Unit != "USD" || strings.TrimSpace(response.PlanName) == "" || response.Remaining == nil || !validFiniteNumber(*response.Remaining) {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	expiresAt, err := normalizeTime(response.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if (response.Subscription == nil) == (response.Balance == nil) {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	if response.Balance != nil {
		if !validFiniteNumber(*response.Balance) || !closeEnough(*response.Balance, *response.Remaining) {
			return nil, ErrUpstreamUsageInvalidResponse
		}
		return &UpstreamUsageInfo{
			Provider:  UpstreamUsageAdapterSub2API,
			Mode:      "balance",
			Unit:      response.Unit,
			Balance:   &UpstreamUsageAmount{Remaining: response.Balance},
			ExpiresAt: expiresAt,
		}, nil
	}
	subscription, err := normalizeSub2APISubscription(response.PlanName, response.Subscription, response.Remaining)
	if err != nil || subscription == nil {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	if subscription.Unlimited {
		if *response.Remaining != -1 {
			return nil, ErrUpstreamUsageInvalidResponse
		}
		return &UpstreamUsageInfo{Provider: UpstreamUsageAdapterSub2API, Mode: "subscription", Unit: response.Unit, Subscription: subscription, ExpiresAt: expiresAt}, nil
	}
	if !validNonNegativeNumber(*response.Remaining) || subscription.Remaining == nil || !closeEnough(*response.Remaining, *subscription.Remaining) {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	return &UpstreamUsageInfo{Provider: UpstreamUsageAdapterSub2API, Mode: "subscription", Unit: response.Unit, Subscription: subscription, ExpiresAt: expiresAt}, nil
}

func normalizeSub2APISubscription(planName string, raw *sub2APISubscription, legacyRemaining ...*float64) (*UpstreamUsageSubscription, error) {
	if raw == nil {
		return nil, nil
	}
	if strings.TrimSpace(planName) == "" || raw.DailyUsageUSD == nil || raw.WeeklyUsageUSD == nil || raw.MonthlyUsageUSD == nil ||
		!validNonNegativeNumber(*raw.DailyUsageUSD) || !validNonNegativeNumber(*raw.WeeklyUsageUSD) || !validNonNegativeNumber(*raw.MonthlyUsageUSD) {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	expiresAt, err := normalizeTime(raw.ExpiresAt)
	if err != nil || expiresAt == nil {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	var remainingSentinel *float64
	if len(legacyRemaining) > 0 {
		remainingSentinel = legacyRemaining[0]
	}
	unlimited := raw.Unlimited != nil && *raw.Unlimited
	if raw.Unlimited == nil && remainingSentinel != nil && *remainingSentinel == -1 {
		unlimited = true
	}
	limits, err := normalizeSub2APISubscriptionLimits(raw)
	if err != nil {
		return nil, err
	}
	if unlimited {
		if len(limits) != 0 {
			return nil, ErrUpstreamUsageInvalidResponse
		}
		return &UpstreamUsageSubscription{PlanName: strings.TrimSpace(planName), Unlimited: true, ExpiresAt: expiresAt}, nil
	}
	if len(limits) == 0 {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	minimum := *limits[0].Remaining
	for _, limit := range limits[1:] {
		if limit.Remaining != nil && *limit.Remaining < minimum {
			minimum = *limit.Remaining
		}
	}
	return &UpstreamUsageSubscription{PlanName: strings.TrimSpace(planName), Remaining: &minimum, ExpiresAt: expiresAt, Limits: limits}, nil
}

func normalizeSub2APISubscriptionLimits(raw *sub2APISubscription) ([]UpstreamUsageLimit, error) {
	type subscriptionInput struct {
		name     string
		used     *float64
		limit    *float64
		resetAt  *time.Time
		start    *time.Time
		duration time.Duration
	}
	inputs := []subscriptionInput{
		{name: "daily", used: raw.DailyUsageUSD, limit: raw.DailyLimitUSD, resetAt: raw.DailyResetAt, start: raw.DailyWindowStart, duration: 24 * time.Hour},
		{name: "weekly", used: raw.WeeklyUsageUSD, limit: raw.WeeklyLimitUSD, resetAt: raw.WeeklyResetAt, start: raw.WeeklyWindowStart, duration: 7 * 24 * time.Hour},
		{name: "monthly", used: raw.MonthlyUsageUSD, limit: raw.MonthlyLimitUSD, resetAt: raw.MonthlyResetAt, start: raw.MonthlyWindowStart, duration: 30 * 24 * time.Hour},
	}
	limits := make([]UpstreamUsageLimit, 0, len(inputs))
	for _, input := range inputs {
		if input.limit == nil || *input.limit == 0 {
			continue
		}
		if input.used == nil || *input.limit < 0 || !validNonNegativeNumber(*input.limit) || !validNonNegativeNumber(*input.used) {
			return nil, ErrUpstreamUsageInvalidResponse
		}
		remaining := math.Max(0, *input.limit-*input.used)
		resetAt, err := normalizeTime(input.resetAt)
		if err != nil {
			return nil, err
		}
		if resetAt == nil && input.start != nil {
			start, startErr := normalizeTime(input.start)
			if startErr != nil {
				return nil, startErr
			}
			if start != nil {
				value := start.Add(input.duration)
				resetAt = &value
			}
		}
		limits = append(limits, UpstreamUsageLimit{Name: input.name, Used: input.used, Limit: input.limit, Remaining: &remaining, ResetAt: resetAt})
	}
	return limits, nil
}

func normalizeSub2APIRateLimits(raw []sub2APIRateLimit) ([]UpstreamUsageLimit, error) {
	limits := make([]UpstreamUsageLimit, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		name := strings.TrimSpace(item.Window)
		if name == "" || (name != "5h" && name != "1d" && name != "7d") {
			return nil, ErrUpstreamUsageInvalidResponse
		}
		if _, exists := seen[name]; exists {
			return nil, ErrUpstreamUsageInvalidResponse
		}
		seen[name] = struct{}{}
		if item.Limit == nil || item.Used == nil || item.Remaining == nil || *item.Limit <= 0 || !validNonNegativeNumber(*item.Limit) ||
			!validNonNegativeNumber(*item.Used) || !validNonNegativeNumber(*item.Remaining) ||
			!closeEnough(*item.Remaining, math.Max(0, *item.Limit-*item.Used)) {
			return nil, ErrUpstreamUsageInvalidResponse
		}
		if err := validateSub2APIWindowStart(item.WindowStart); err != nil {
			return nil, err
		}
		resetAt, err := normalizeTime(item.ResetAt)
		if err != nil {
			return nil, err
		}
		limits = append(limits, UpstreamUsageLimit{Name: name, Used: item.Used, Limit: item.Limit, Remaining: item.Remaining, ResetAt: resetAt})
	}
	return limits, nil
}

func validateSub2APIWindowStart(raw json.RawMessage) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		// 当前 /v1/usage 合约要求字段存在；只有明确的 JSON null 才表示
		// 该窗口没有可用的起始时间。
		return ErrUpstreamUsageInvalidResponse
	}
	if trimmed == "null" {
		return nil
	}
	var value time.Time
	if err := json.Unmarshal(raw, &value); err != nil || value.IsZero() {
		return ErrUpstreamUsageInvalidResponse
	}
	return nil
}

// --- New API 适配器 ---

type newAPIUsageAdapter struct{}

func (*newAPIUsageAdapter) Name() string { return UpstreamUsageAdapterNewAPI }

type newAPISubscriptionResponse struct {
	Object             string          `json:"object"`
	HasPaymentMethod   *bool           `json:"has_payment_method"`
	SoftLimitUSD       *float64        `json:"soft_limit_usd"`
	HardLimitUSD       *float64        `json:"hard_limit_usd"`
	SystemHardLimitUSD *float64        `json:"system_hard_limit_usd"`
	AccessUntil        *int64          `json:"access_until"`
	Error              json.RawMessage `json:"error"`
}

type newAPIUsageResponse struct {
	Object     string          `json:"object"`
	TotalUsage *float64        `json:"total_usage"`
	Error      json.RawMessage `json:"error"`
}

func (a *newAPIUsageAdapter) Query(ctx context.Context, client *upstreamUsageHTTPClient) (*UpstreamUsageInfo, error) {
	subscriptionBody, status, err := client.get(ctx, "/v1/dashboard/billing/subscription", true)
	if err != nil {
		return nil, err
	}
	if httpErr := upstreamUsageHTTPError(status, true); httpErr != nil {
		return nil, httpErr
	}
	subscription, err := parseNewAPISubscription(subscriptionBody)
	if err != nil {
		return nil, err
	}
	usageBody, status, err := client.get(ctx, "/v1/dashboard/billing/usage", true)
	if err != nil {
		return nil, err
	}
	if httpErr := upstreamUsageHTTPError(status, false); httpErr != nil {
		return nil, httpErr
	}
	usage, err := parseNewAPIUsage(usageBody)
	if err != nil {
		return nil, err
	}
	used := *usage.TotalUsage / 100
	total := *subscription.HardLimitUSD
	remaining := total - used
	var expiresAt *time.Time
	if *subscription.AccessUntil > 0 {
		value := time.Unix(*subscription.AccessUntil, 0).UTC()
		expiresAt = &value
	}
	unit := "USD"
	statusCtx, cancel := context.WithTimeout(ctx, upstreamUsageStatusTimeout)
	defer cancel()
	if detected := a.queryUnit(statusCtx, client); detected != "" {
		unit = detected
	}
	// New API 没有独立的套餐名称；用协议名标记订阅来源，并把硬限制
	// 作为一个普通限额返回，避免把站点钱包误称为余额。
	hardLimit := UpstreamUsageLimit{Name: "hard_limit", Used: &used, Limit: &total, Remaining: &remaining}
	subscriptionInfo := &UpstreamUsageSubscription{
		PlanName:  "New API",
		Remaining: &remaining,
		ExpiresAt: expiresAt,
	}
	return &UpstreamUsageInfo{
		Provider:     UpstreamUsageAdapterNewAPI,
		Mode:         "quota",
		Unit:         unit,
		Balance:      &UpstreamUsageAmount{Used: &used, Total: &total, Remaining: &remaining},
		Limits:       []UpstreamUsageLimit{hardLimit},
		Subscription: subscriptionInfo,
		ExpiresAt:    expiresAt,
	}, nil
}

func (*newAPIUsageAdapter) queryUnit(ctx context.Context, client *upstreamUsageHTTPClient) string {
	endpoint, err := upstreamUsageStatusEndpoint(client.baseURL)
	if err != nil {
		return ""
	}
	body, status, err := client.getURL(ctx, endpoint, false)
	if err != nil || status < http.StatusOK || status >= http.StatusMultipleChoices {
		return ""
	}
	var response struct {
		Success *bool `json:"success"`
		Data    *struct {
			QuotaDisplayType *string `json:"quota_display_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.Success == nil || !*response.Success || response.Data == nil || response.Data.QuotaDisplayType == nil {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(*response.Data.QuotaDisplayType)) {
	case "USD", "CNY", "TOKENS":
		return strings.ToUpper(strings.TrimSpace(*response.Data.QuotaDisplayType))
	default:
		return ""
	}
}

func (c *upstreamUsageHTTPClient) getURL(ctx context.Context, endpoint string, authenticated bool) ([]byte, int, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, 0, ErrUpstreamUsageConfigInvalid
	}
	// 复用统一请求实现，但不再拼接路径。
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, ErrUpstreamUsageRequestFailed
	}
	reqCtx := WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI)
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(reqCtx))
	req.Header.Set("Accept", "application/json")
	c.account.ApplyHeaderOverrides(req.Header)
	if authenticated {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	} else {
		req.Header.Del("Authorization")
	}
	resp, err := c.upstream.DoWithTLS(req, c.proxyURL, c.account.ID, c.account.Concurrency, c.tlsProfile)
	if err != nil {
		return nil, 0, upstreamUsageOperationError(ctx, err)
	}
	if resp == nil || resp.Body == nil {
		return nil, 0, ErrUpstreamUsageInvalidResponse
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, upstreamUsageMaxBodyBytes+1))
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, 0, upstreamUsageOperationError(ctx, readErr)
	}
	if int64(len(body)) > upstreamUsageMaxBodyBytes {
		return nil, 0, ErrUpstreamUsageInvalidResponse
	}
	return body, resp.StatusCode, nil
}

func parseNewAPISubscription(body []byte) (*newAPISubscriptionResponse, error) {
	var response newAPISubscriptionResponse
	if err := json.Unmarshal(body, &response); err != nil || response.Object != "billing_subscription" || response.HasPaymentMethod == nil ||
		response.SoftLimitUSD == nil || response.HardLimitUSD == nil || response.SystemHardLimitUSD == nil || response.AccessUntil == nil ||
		!validNonNegativeNumber(*response.SoftLimitUSD) || !validNonNegativeNumber(*response.HardLimitUSD) ||
		!validNonNegativeNumber(*response.SystemHardLimitUSD) || *response.AccessUntil < 0 || *response.AccessUntil > 253402300799 || hasNewAPIError(response.Error) {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	return &response, nil
}

func parseNewAPIUsage(body []byte) (*newAPIUsageResponse, error) {
	var response newAPIUsageResponse
	if err := json.Unmarshal(body, &response); err != nil || response.Object != "list" || response.TotalUsage == nil ||
		!validNonNegativeNumber(*response.TotalUsage) || hasNewAPIError(response.Error) {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	return &response, nil
}

func hasNewAPIError(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func normalizeTime(value *time.Time) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	if value.IsZero() {
		return nil, ErrUpstreamUsageInvalidResponse
	}
	normalized := value.UTC()
	return &normalized, nil
}

func closeEnough(left, right float64) bool {
	return math.Abs(left-right) <= math.Max(0.000001, math.Max(math.Abs(left), math.Abs(right))*0.00001)
}

func upstreamUsageContextFingerprint(account *Account, config UpstreamUsageQueryConfig) string {
	if account == nil {
		return "nil"
	}
	payload := struct {
		ID          int64
		Platform    string
		Type        string
		Credentials map[string]any
		Config      UpstreamUsageQueryConfig
		BaseURL     string
		ProxyID     *int64
		Proxy       any
		Concurrency int
		Transport   map[string]any
	}{
		ID:          account.ID,
		Platform:    account.Platform,
		Type:        account.Type,
		Credentials: account.Credentials,
		Config:      config,
		BaseURL:     upstreamUsageAccountBaseURL(account),
		ProxyID:     account.ProxyID,
		Concurrency: account.Concurrency,
		Transport: map[string]any{
			"enable_tls_fingerprint":     extraValue(account.Extra, "enable_tls_fingerprint"),
			"tls_fingerprint_profile_id": extraValue(account.Extra, "tls_fingerprint_profile_id"),
			"tls_fingerprint_router_id":  extraValue(account.Extra, "tls_fingerprint_router_id"),
		},
	}
	if account.Proxy != nil {
		// 指纹只留在进程内；代理密码不会进入日志、响应或浏览器缓存。
		payload.Proxy = struct {
			ID       int64
			Protocol string
			Host     string
			Port     int
			Username string
			Password string
			Status   string
		}{account.Proxy.ID, account.Proxy.Protocol, account.Proxy.Host, account.Proxy.Port, account.Proxy.Username, account.Proxy.Password, account.Proxy.Status}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(fmt.Sprintf("%d:%s:%s:%v", account.ID, account.Platform, config.Adapter, account.Credentials))
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
