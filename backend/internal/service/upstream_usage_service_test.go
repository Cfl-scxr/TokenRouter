package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type upstreamUsageAccountRepoStub struct {
	AccountRepository
	mu       sync.Mutex
	account  *Account
	getEvent chan struct{}
}

func (s *upstreamUsageAccountRepoStub) GetByID(_ context.Context, _ int64) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.account == nil {
		return nil, ErrAccountNotFound
	}
	if s.getEvent != nil {
		select {
		case s.getEvent <- struct{}{}:
		default:
		}
	}
	copy := *s.account
	return &copy, nil
}

type blockingUpstreamUsageHTTP struct {
	started chan struct{}
	release chan struct{}
	body    string
	once    sync.Once
	calls   atomic.Int32
}

func (s *blockingUpstreamUsageHTTP) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	return s.DoWithTLS(req, proxyURL, accountID, concurrency, nil)
}

func (s *blockingUpstreamUsageHTTP) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	s.calls.Add(1)
	s.once.Do(func() { close(s.started) })
	select {
	case <-s.release:
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(s.body))}, nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

type upstreamUsageHTTPStub struct {
	mu        sync.Mutex
	requests  []*http.Request
	responses []struct {
		status int
		body   string
		err    error
	}
}

func (s *upstreamUsageHTTPStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return s.DoWithTLS(req, "", 0, 0, nil)
}

func (s *upstreamUsageHTTPStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req.Clone(req.Context()))
	if len(s.responses) == 0 {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	if response.err != nil {
		return nil, response.err
	}
	return &http.Response{StatusCode: response.status, Body: io.NopCloser(strings.NewReader(response.body))}, nil
}

func testUpstreamUsageConfig() *config.Config {
	return &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled:           false,
		AllowInsecureHTTP: true,
	}}}
}

func TestEffectiveUpstreamUsageConfigDefaultsAndNormalization(t *testing.T) {
	account := &Account{Type: AccountTypeAPIKey}
	config, err := EffectiveUpstreamUsageConfig(account)
	require.NoError(t, err)
	require.Equal(t, UpstreamUsageQueryConfig{Enabled: true, Adapter: UpstreamUsageAdapterSub2API}, config)

	extra := map[string]any{UpstreamUsageQueryExtraKey: map[string]any{
		"enabled":  false,
		"adapter":  UpstreamUsageAdapterNewAPI,
		"base_url": "https://usage.example/v1",
	}}
	require.NoError(t, NormalizeUpstreamUsageExtra(extra))
	require.Equal(t, map[string]any{
		"enabled":  false,
		"adapter":  UpstreamUsageAdapterNewAPI,
		"base_url": "https://usage.example/v1",
	}, extra[UpstreamUsageQueryExtraKey])

	bad := map[string]any{UpstreamUsageQueryExtraKey: map[string]any{"api_key": "secret"}}
	require.Error(t, NormalizeUpstreamUsageExtra(bad))

	disabledAccount := &Account{Extra: map[string]any{UpstreamUsageQueryExtraKey: map[string]any{"enabled": false}}}
	disabled, err := EffectiveUpstreamUsageConfig(disabledAccount)
	require.NoError(t, err)
	require.False(t, disabled.Enabled)
	require.Equal(t, UpstreamUsageAdapterSub2API, disabled.Adapter)

	unknown := map[string]any{UpstreamUsageQueryExtraKey: map[string]any{"adapter": "custom-script"}}
	require.ErrorIs(t, NormalizeUpstreamUsageExtra(unknown), ErrUpstreamUsageConfigInvalid)
	_, err = EffectiveUpstreamUsageConfig(&Account{Extra: unknown})
	require.ErrorIs(t, err, ErrUpstreamUsageUnsupported)

	unsafeURL := map[string]any{UpstreamUsageQueryExtraKey: map[string]any{
		"base_url": "https://user:secret@gateway.example/v1?token=secret",
	}}
	require.ErrorIs(t, NormalizeUpstreamUsageExtra(unsafeURL), ErrUpstreamUsageConfigInvalid)
	_, ok := normalizedUpstreamUsageConfigValue(map[string]any{"api_key": "secret"})
	require.False(t, ok)

	require.Equal(t, []UpstreamUsageAdapterOption{
		{Name: UpstreamUsageAdapterSub2API, Label: "Sub2API / TokenRouter"},
		{Name: UpstreamUsageAdapterNewAPI, Label: "New API"},
	}, UpstreamUsageAdapterOptions())
}

func TestUpstreamUsageBaseURLReusesSecurityPolicy(t *testing.T) {
	service := NewUpstreamUsageService(nil, nil, &config.Config{Security: config.SecurityConfig{
		URLAllowlist: config.URLAllowlistConfig{
			Enabled:           true,
			UpstreamHosts:     []string{"gateway.example"},
			AllowPrivateHosts: false,
		},
	}}, nil)

	value, err := service.validateBaseURL("https://gateway.example/v1/")
	require.NoError(t, err)
	require.Equal(t, "https://gateway.example/v1", value)
	value, err = service.validateBaseURL("HTTPS://gateway.example/v1/")
	require.NoError(t, err)
	require.Equal(t, "https://gateway.example/v1", value)
	_, err = service.validateBaseURL("http://gateway.example/v1")
	require.Error(t, err)
	_, err = service.validateBaseURL("https://127.0.0.1/v1")
	require.Error(t, err)
	_, err = service.validateBaseURL("https://unlisted.example/v1")
	require.Error(t, err)
}

func TestUpstreamUsageBaseURLUsesStrictDefaultsWithoutConfig(t *testing.T) {
	service := NewUpstreamUsageService(nil, nil, nil, nil)
	_, err := service.validateBaseURL("http://gateway.example/v1")
	require.Error(t, err)
	_, err = service.validateBaseURL("https://127.0.0.1/v1")
	require.Error(t, err)
	value, err := service.validateBaseURL("https://gateway.example/v1")
	require.NoError(t, err)
	require.Equal(t, "https://gateway.example/v1", value)
}

func TestParseSub2APIUsageModes(t *testing.T) {
	balance, err := parseSub2APIUsage([]byte(`{"isValid":true,"mode":"unrestricted","unit":"USD","planName":"payg","remaining":12.5,"balance":12.5}`))
	require.NoError(t, err)
	require.Equal(t, "balance", balance.Mode)
	require.Equal(t, 12.5, *balance.Balance.Remaining)

	quota, err := parseSub2APIUsage([]byte(`{"isValid":true,"mode":"quota_limited","status":"active","unit":"USD","remaining":75,"quota":{"limit":100,"used":25,"remaining":75,"unit":"USD"},"rate_limits":[{"window":"5h","limit":10,"used":2,"remaining":8,"window_start":null}]}`))
	require.NoError(t, err)
	require.Equal(t, "quota", quota.Mode)
	require.Len(t, quota.Limits, 1)

	unlimited, err := parseSub2APIUsage([]byte(`{"isValid":true,"mode":"unrestricted","unit":"USD","planName":"pro","remaining":-1,"subscription":{"daily_usage_usd":0,"weekly_usage_usd":0,"monthly_usage_usd":0,"expires_at":"2030-01-01T00:00:00Z","unlimited":true}}`))
	require.NoError(t, err)
	require.True(t, unlimited.Subscription.Unlimited)
	require.Nil(t, unlimited.Subscription.Remaining)
	require.NotContains(t, string(mustJSONMarshal(t, unlimited)), `-1`)

	_, err = parseSub2APIUsage([]byte(`{"isValid":true,"mode":"quota_limited","status":"active","unit":"USD","remaining":90,"quota":{"limit":100,"used":25,"remaining":90,"unit":"USD"}}`))
	require.Error(t, err)
}

func TestParseSub2APIUsageNormalizesNegativeWalletAndRejectsMalformedWindowStart(t *testing.T) {
	balance, err := parseSub2APIUsage([]byte(`{"isValid":true,"mode":"unrestricted","unit":"USD","planName":"wallet","remaining":-2.5,"balance":-2.5,"expires_at":"2030-01-01T00:00:00+08:00"}`))
	require.NoError(t, err)
	require.Equal(t, -2.5, *balance.Balance.Remaining)
	require.Equal(t, time.Date(2029, 12, 31, 16, 0, 0, 0, time.UTC), *balance.ExpiresAt)

	_, err = parseSub2APIUsage([]byte(`{"isValid":true,"mode":"quota_limited","status":"active","rate_limits":[{"window":"5h","limit":10,"used":2,"remaining":8,"window_start":"not-a-time"}]}`))
	require.ErrorIs(t, err, ErrUpstreamUsageInvalidResponse)
	_, err = parseSub2APIUsage([]byte(`{"isValid":true,"mode":"quota_limited","status":"active","rate_limits":[{"window":"5h","limit":10,"used":2,"remaining":8}]}`))
	require.ErrorIs(t, err, ErrUpstreamUsageInvalidResponse)
}

func TestParseSub2APIUsageDistinguishesMissingValidityFromRejectedKey(t *testing.T) {
	_, err := parseSub2APIUsage([]byte(`{"mode":"unrestricted"}`))
	require.ErrorIs(t, err, ErrUpstreamUsageInvalidResponse)

	_, err = parseSub2APIUsage([]byte(`{"isValid":false}`))
	require.ErrorIs(t, err, ErrUpstreamUsageAuthFailed)
}

func TestUpstreamUsageEndpointUsesExistingVersionedBaseURLRules(t *testing.T) {
	tests := map[string]string{
		"https://gateway.example/v1":     "https://gateway.example/v1/usage",
		"https://gateway.example/v4":     "https://gateway.example/v4/usage",
		"https://gateway.example/v1beta": "https://gateway.example/v1beta/usage",
		"https://gateway.example/api":    "https://gateway.example/api/v1/usage",
	}
	for base, want := range tests {
		got, err := upstreamUsageEndpoint(base, "/v1/usage")
		require.NoError(t, err, base)
		require.Equal(t, want, got, base)
	}

	statusTests := map[string]string{
		"https://gateway.example/v1":         "https://gateway.example/api/status",
		"https://gateway.example/subpath/v1": "https://gateway.example/subpath/api/status",
		"https://gateway.example/v4":         "https://gateway.example/api/status",
	}
	for base, want := range statusTests {
		got, err := upstreamUsageStatusEndpoint(base)
		require.NoError(t, err, base)
		require.Equal(t, want, got, base)
	}
}

func TestUpstreamUsageAccountBaseURLUsesPlatformNormalization(t *testing.T) {
	account := &Account{
		Platform:    PlatformAntigravity,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://gateway.example/"},
	}
	require.Equal(t, "https://gateway.example/antigravity", upstreamUsageAccountBaseURL(account))
}

func TestParseNewAPIUsageConvertsCents(t *testing.T) {
	subscription, err := parseNewAPISubscription([]byte(`{"object":"billing_subscription","has_payment_method":true,"soft_limit_usd":10,"hard_limit_usd":20,"system_hard_limit_usd":20,"access_until":1893456000}`))
	require.NoError(t, err)
	usage, err := parseNewAPIUsage([]byte(`{"object":"list","total_usage":1250}`))
	require.NoError(t, err)
	require.Equal(t, float64(20), *subscription.HardLimitUSD)
	require.Equal(t, float64(1250), *usage.TotalUsage)
}

func TestNewAPIUsageQueryPreservesOverageAndSubscriptionMetadata(t *testing.T) {
	account := &Account{
		ID: 11, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-new-api", "base_url": "https://new-api.example/v1",
			"header_override_enabled": true,
			"header_overrides":        map[string]any{"x-custom": "usage-query", "authorization": "Bearer wrong-key"},
		},
		Extra: map[string]any{UpstreamUsageQueryExtraKey: map[string]any{"adapter": UpstreamUsageAdapterNewAPI}},
	}
	upstream := &upstreamUsageHTTPStub{responses: []struct {
		status int
		body   string
		err    error
	}{
		{status: http.StatusOK, body: `{"object":"billing_subscription","has_payment_method":true,"soft_limit_usd":1,"hard_limit_usd":10,"system_hard_limit_usd":10,"access_until":1893456000}`},
		{status: http.StatusOK, body: `{"object":"list","total_usage":1250}`},
		{status: http.StatusNotFound, body: `{}`},
	}}
	service := NewUpstreamUsageService(&upstreamUsageAccountRepoStub{account: account}, upstream, testUpstreamUsageConfig(), nil)
	result, err := service.QueryAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, -2.5, *result.Usage.Balance.Remaining)
	require.Equal(t, "New API", result.Usage.Subscription.PlanName)
	require.Equal(t, -2.5, *result.Usage.Subscription.Remaining)
	require.NotContains(t, string(mustJSONMarshal(t, result)), "sk-new-api")

	upstream.mu.Lock()
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "Bearer sk-new-api", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "Bearer sk-new-api", upstream.requests[1].Header.Get("Authorization"))
	require.Empty(t, upstream.requests[2].Header.Get("Authorization"))
	for _, request := range upstream.requests {
		require.Equal(t, "usage-query", getHeaderRaw(request.Header, "x-custom"))
		require.True(t, HTTPUpstreamRedirectsDisabled(request.Context()))
	}
	upstream.mu.Unlock()
}

func TestNewAPIUsageEndpointMissingIsInvalidResponse(t *testing.T) {
	account := &Account{
		ID: 12, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"api_key": "sk-new-api", "base_url": "https://new-api.example/v1"},
		Extra:       map[string]any{UpstreamUsageQueryExtraKey: map[string]any{"adapter": UpstreamUsageAdapterNewAPI}},
	}
	upstream := &upstreamUsageHTTPStub{responses: []struct {
		status int
		body   string
		err    error
	}{
		{status: http.StatusOK, body: `{"object":"billing_subscription","has_payment_method":true,"soft_limit_usd":1,"hard_limit_usd":10,"system_hard_limit_usd":10,"access_until":1893456000}`},
		{status: http.StatusNotFound, body: `{}`},
	}}
	service := NewUpstreamUsageService(&upstreamUsageAccountRepoStub{account: account}, upstream, testUpstreamUsageConfig(), nil)
	_, err := service.QueryAccount(context.Background(), account.ID)
	require.ErrorIs(t, err, ErrUpstreamUsageInvalidResponse)
}

func mustJSONMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

func TestUpstreamUsageServiceQueriesAPIKeyWithoutMutatingAccount(t *testing.T) {
	account := &Account{
		ID:          7,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "http://usage.example/v1"},
		Extra:       map[string]any{},
		Concurrency: 2,
	}
	original := *account
	upstream := &upstreamUsageHTTPStub{responses: []struct {
		status int
		body   string
		err    error
	}{
		{status: http.StatusOK, body: `{"isValid":true,"mode":"unrestricted","unit":"USD","planName":"payg","remaining":12.5,"balance":12.5}`},
	}}
	repo := &upstreamUsageAccountRepoStub{account: account}
	service := NewUpstreamUsageService(repo, upstream, testUpstreamUsageConfig(), nil)
	result, err := service.QueryAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, account.ID, result.AccountID)
	require.Equal(t, UpstreamUsageAdapterSub2API, result.Adapter)
	require.Equal(t, 12.5, *result.Usage.Balance.Remaining)
	require.Equal(t, original.Credentials, account.Credentials)
	require.Equal(t, original.Extra, account.Extra)
	require.Equal(t, int64(1), service.SnapshotMetrics().Counts[UpstreamUsageAdapterSub2API+":success"])

	upstream.mu.Lock()
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "/v1/usage", upstream.requests[0].URL.Path)
	require.Equal(t, "Bearer sk-test", upstream.requests[0].Header.Get("Authorization"))
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.requests[0].Context()))
	upstream.mu.Unlock()
}

func TestUpstreamUsageServiceRejectsBedrockAndSupportsBatchErrors(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeBedrock, Credentials: map[string]any{"api_key": "x", "base_url": "https://example.com"}}
	repo := &upstreamUsageAccountRepoStub{account: account}
	service := NewUpstreamUsageService(repo, &upstreamUsageHTTPStub{}, testUpstreamUsageConfig(), nil)
	_, err := service.QueryAccount(context.Background(), 1)
	require.ErrorIs(t, err, ErrUpstreamUsageAccountInvalid)

	_, errorsByID, err := service.QueryBatch(context.Background(), []int64{1, 0, -1})
	require.NoError(t, err)
	require.ErrorIs(t, errorsByID[1], ErrUpstreamUsageAccountInvalid)
}

func TestUpstreamUsageServiceReportsMissingProxyAsRequestFailure(t *testing.T) {
	proxyID := int64(9)
	account := &Account{
		ID: 9, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		ProxyID: &proxyID, Credentials: map[string]any{"api_key": "key", "base_url": "https://example.com"},
	}
	service := NewUpstreamUsageService(&upstreamUsageAccountRepoStub{account: account}, &upstreamUsageHTTPStub{}, testUpstreamUsageConfig(), nil)
	_, err := service.QueryAccount(context.Background(), account.ID)
	require.ErrorIs(t, err, ErrUpstreamUsageRequestFailed)
}

func TestValidateNormalizedUsageRejectsUnknownMode(t *testing.T) {
	err := validateNormalizedUsage(&UpstreamUsageInfo{Provider: "test", Mode: "window"})
	require.Error(t, err)
	err = validateNormalizedUsage(&UpstreamUsageInfo{Provider: "test", Mode: "balance"})
	require.Error(t, err)
}

func TestUpstreamUsageServiceTimeoutError(t *testing.T) {
	upstream := &upstreamUsageHTTPStub{responses: []struct {
		status int
		body   string
		err    error
	}{{err: context.DeadlineExceeded}}}
	account := &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "x", "base_url": "https://example.com"}}
	service := NewUpstreamUsageService(&upstreamUsageAccountRepoStub{account: account}, upstream, testUpstreamUsageConfig(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := service.QueryAccount(ctx, account.ID)
	require.ErrorIs(t, err, ErrUpstreamUsageTimeout)
}

func TestUpstreamUsageServiceRejectsDisabledQueryAndOversizedResponse(t *testing.T) {
	account := &Account{
		ID: 4, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://example.com"},
		Extra:       map[string]any{UpstreamUsageQueryExtraKey: map[string]any{"enabled": false}},
	}
	upstream := &upstreamUsageHTTPStub{}
	service := NewUpstreamUsageService(&upstreamUsageAccountRepoStub{account: account}, upstream, testUpstreamUsageConfig(), nil)
	_, err := service.QueryAccount(context.Background(), account.ID)
	require.ErrorIs(t, err, ErrUpstreamUsageDisabled)
	require.Empty(t, upstream.requests)

	account.Extra = nil
	upstream.responses = append(upstream.responses, struct {
		status int
		body   string
		err    error
	}{status: http.StatusOK, body: strings.Repeat("x", upstreamUsageMaxBodyBytes+1)})
	_, err = service.QueryAccount(context.Background(), account.ID)
	require.ErrorIs(t, err, ErrUpstreamUsageInvalidResponse)
}

func TestUpstreamUsageServiceSingleflightWaitersCancelIndependently(t *testing.T) {
	account := &Account{
		ID: 5, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Concurrency: 1,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://example.com"},
	}
	repo := &upstreamUsageAccountRepoStub{account: account, getEvent: make(chan struct{}, 16)}
	upstream := &blockingUpstreamUsageHTTP{
		started: make(chan struct{}),
		release: make(chan struct{}),
		body:    `{"isValid":true,"mode":"unrestricted","unit":"USD","planName":"payg","remaining":3,"balance":3}`,
	}
	service := NewUpstreamUsageService(repo, upstream, testUpstreamUsageConfig(), nil)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstErr := make(chan error, 1)
	go func() {
		_, err := service.QueryAccount(firstCtx, account.ID)
		firstErr <- err
	}()
	select {
	case <-upstream.started:
	case <-time.After(time.Second):
		t.Fatal("shared query did not start")
	}
	for len(repo.getEvent) > 0 {
		<-repo.getEvent
	}

	secondResult := make(chan *UpstreamUsageQueryResult, 1)
	secondErr := make(chan error, 1)
	go func() {
		result, err := service.QueryAccount(context.Background(), account.ID)
		secondResult <- result
		secondErr <- err
	}()
	select {
	case <-repo.getEvent:
	case <-time.After(time.Second):
		t.Fatal("second waiter did not load its identity snapshot")
	}
	cancelFirst()
	require.ErrorIs(t, <-firstErr, context.Canceled)
	// 等待方完成预读后给它一次调度机会进入 singleflight。
	time.Sleep(10 * time.Millisecond)
	close(upstream.release)
	require.NoError(t, <-secondErr)
	require.NotNil(t, <-secondResult)
	require.Equal(t, int32(1), upstream.calls.Load())
}

func TestUpstreamUsageServiceRejectsIdentityChangeAfterQuery(t *testing.T) {
	account := &Account{
		ID: 6, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Concurrency: 1,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://example.com"},
	}
	repo := &upstreamUsageAccountRepoStub{account: account}
	upstream := &blockingUpstreamUsageHTTP{
		started: make(chan struct{}),
		release: make(chan struct{}),
		body:    `{"isValid":true,"mode":"unrestricted","unit":"USD","planName":"payg","remaining":3,"balance":3}`,
	}
	service := NewUpstreamUsageService(repo, upstream, testUpstreamUsageConfig(), nil)
	resultErr := make(chan error, 1)
	go func() {
		_, err := service.QueryAccount(context.Background(), account.ID)
		resultErr <- err
	}()
	select {
	case <-upstream.started:
	case <-time.After(time.Second):
		t.Fatal("query did not start")
	}
	repo.mu.Lock()
	repo.account.Status = "inactive"
	repo.mu.Unlock()
	close(upstream.release)
	require.ErrorIs(t, <-resultErr, ErrUpstreamUsageIdentityChanged)
}

func TestUpstreamUsageServiceTreatsDeletionDuringQueryAsIdentityChange(t *testing.T) {
	account := &Account{
		ID: 8, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Concurrency: 1,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://example.com"},
	}
	repo := &upstreamUsageAccountRepoStub{account: account}
	upstream := &blockingUpstreamUsageHTTP{
		started: make(chan struct{}),
		release: make(chan struct{}),
		body:    `{"isValid":true,"mode":"unrestricted","unit":"USD","planName":"payg","remaining":3,"balance":3}`,
	}
	service := NewUpstreamUsageService(repo, upstream, testUpstreamUsageConfig(), nil)
	resultErr := make(chan error, 1)
	go func() {
		_, err := service.QueryAccount(context.Background(), account.ID)
		resultErr <- err
	}()
	select {
	case <-upstream.started:
	case <-time.After(time.Second):
		t.Fatal("query did not start")
	}
	repo.mu.Lock()
	repo.account = nil
	repo.mu.Unlock()
	close(upstream.release)
	require.ErrorIs(t, <-resultErr, ErrUpstreamUsageIdentityChanged)
}
