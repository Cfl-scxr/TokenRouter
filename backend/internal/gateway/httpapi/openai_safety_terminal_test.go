//go:build unit

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	accountcore "github.com/TokenFlux/TokenRouter/internal/account"
	forwardcore "github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/openaiforward"
	textflow "github.com/TokenFlux/TokenRouter/internal/gateway/text"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 未实现端口可捕获意外恢复或成功路径；账号与传输为合成夹具，
// 请求编排、HTTP 交换、错误输出及故障转移投影使用真实实现。
type safetyTerminalAttempts struct {
	textflow.ResponsePorts
	ctx                                     context.Context
	forward                                 func(int64) error
	selected                                int64
	selections, completed, failed, switches int
}

func (p *safetyTerminalAttempts) Context() context.Context { return p.ctx }
func (p *safetyTerminalAttempts) CanAttempt() bool         { return true }
func (p *safetyTerminalAttempts) Select(excluded map[int64]struct{}) (textflow.ResponseSelection, error) {
	for _, id := range []int64{1, 2} {
		if _, skip := excluded[id]; !skip {
			p.selected = id
			p.selections++
			return textflow.ResponseSelection{Available: true, Selection: textflow.Selection{Account: accountcore.AccountSnapshot{ID: id}}}, nil
		}
	}
	return textflow.ResponseSelection{}, errors.New("synthetic candidates exhausted")
}
func (p *safetyTerminalAttempts) Acquire() bool { return true }
func (p *safetyTerminalAttempts) Forward() textflow.ResponseOutcome {
	err := p.forward(p.selected)
	out := textflow.ResponseOutcome{Outcome: textflow.Outcome{Err: err}}
	var failure *forwardcore.UpstreamFailoverError
	if errors.As(err, &failure) {
		out.Failure = &textflow.AttemptFailure{Cause: err, Policy: failure.RetryFailure()}
	}
	return out
}
func (p *safetyTerminalAttempts) RetryReady(*textflow.AttemptFailure) bool     { return true }
func (p *safetyTerminalAttempts) Switched()                                    { p.switches++ }
func (p *safetyTerminalAttempts) Switching(*textflow.AttemptFailure, int, int) {}
func (p *safetyTerminalAttempts) OtherFailure(error)                           {}
func (p *safetyTerminalAttempts) Complete()                                    { p.completed++ }
func (p *safetyTerminalAttempts) Failed()                                      { p.failed++ }

// 回环上游与两个合成候选验证真实重试编排和响应输出。
func TestSafetyTerminalHTTPOrchestration(t *testing.T) {
	for _, code := range []string{"cyber_policy", "session_blocked_by_cyber_policy"} {
		for _, status := range []int{400, 401, 403, 429, 500} {
			t.Run(fmt.Sprintf("%s/%d", code, status), func(t *testing.T) {
				runSafetyTerminalHTTP(t, status, `{"error":{"code":"`+code+`","message":"SENSITIVE_SENTINEL invalid agent task previous_response_not_found unsupported parameter instructions","param":"instructions","private":"SENSITIVE_SENTINEL"}}`, code, 1)
			})
		}
	}
	for _, status := range []int{403, 429, 500} {
		t.Run(fmt.Sprintf("ordinary/%d", status), func(t *testing.T) {
			runSafetyTerminalHTTP(t, status, `{"error":{"type":"permission_error","message":"ordinary temporary failure"}}`, "session_blocked_by_cyber_policy", 2)
		})
	}
}

func runSafetyTerminalHTTP(t *testing.T, status int, payload, code string, expectedCalls int) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, payload)
		} else {
			w.WriteHeader(403)
			_, _ = io.WriteString(w, `{"error":{"code":"session_blocked_by_cyber_policy"}}`)
		}
	}))
	defer server.Close()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := newWSFixture(wsFixtureInputs{})
	recoveries := 0
	p := &safetyTerminalAttempts{ctx: c.Request.Context()}
	p.forward = func(id int64) error {
		account := &gatewayprovider.ExecutionAccount{Record: accountcore.Record{LoadLocation: time.LoadLocation, ID: id, Platform: capability.PlatformOpenAI, Type: capability.AccountTypeAPIKey}}
		options := openaiforward.HTTPOptions{
			Exchange: openai.HTTPExchangeOptions{
				RequestContext: func(ctx context.Context) (context.Context, context.CancelFunc) { return ctx, func() {} },
				Build: func(ctx context.Context, body []byte) (*http.Request, error) {
					return http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(string(body)))
				},
				ApplyHeaders: func(http.Header) {}, Latency: func(time.Duration) {}, TransportError: func(err error) error { return err },
				Do: server.Client().Do,
			},
			ReadErrorBody:    svc.Output.ReadErrorBody,
			IsAgentIdentity:  func(context.Context) bool { return expectedCalls == 1 },
			InvalidAgentTask: func(int, []byte) bool { return true },
			RecoverAgentTask: func(context.Context) error { recoveries++; return nil },
			RedactErrorBody:  func(_ context.Context, b []byte) []byte { return b },
			ErrorDetails: func(b []byte) (string, string) {
				return gjson.GetBytes(b, "error.message").String(), gjson.GetBytes(b, "error.code").String()
			},
			CompactRetry:   func([]byte, int, string, []byte, bool) ([]byte, string, bool) { recoveries++; return nil, "", false },
			ShouldFailover: gatewayprovider.ShouldFailoverOpenAIResponse,
			HTTPFailover: func(resp *http.Response, b []byte, msg, _ string) error {
				return gatewayprovider.NewOpenAIUpstreamFailure(resp.StatusCode, resp.Header, b, msg, false)
			},
			ErrorResponse: func(resp *http.Response, body []byte, model string) error {
				_, err := svc.Output.ResponseError(c.Request.Context(), resp, c, account, body, model)
				return err
			},
			Log: func(string, ...any) {},
		}
		_, err := openaiforward.RunHTTP(c.Request.Context(), openaiforward.HTTPInput{Body: []byte(`{"model":"synthetic","instructions":"synthetic"}`)}, options)
		return err
	}
	textflow.RunResponses(textflow.ResponseOptions{MaxSwitches: 1}, p)
	require.Equal(t, expectedCalls, int(calls.Load()))
	require.Equal(t, expectedCalls, p.selections)
	require.Equal(t, expectedCalls-1, p.switches)
	if expectedCalls == 1 {
		require.Zero(t, recoveries)
	}
	require.Equal(t, 1, p.completed)
	require.Equal(t, 1, p.failed)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, code, gjson.Get(rec.Body.String(), "error.code").String())
	require.Equal(t, openai.OpenAISafetyTerminalType(code), gjson.Get(rec.Body.String(), "error.type").String())
	require.Len(t, gjson.Get(rec.Body.String(), "error").Map(), 3)
	require.NotContains(t, rec.Body.String(), "SENSITIVE_SENTINEL")
	if code == "session_blocked_by_cyber_policy" {
		require.Nil(t, GetOpsCyberPolicy(c))
	}
}

func TestSafetyTerminalResponsesReaders(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, started := range []bool{false, true} {
			for _, code := range []string{"cyber_policy", "session_blocked_by_cyber_policy"} {
				for _, nested := range []bool{false, true} {
					t.Run(fmt.Sprintf("pass=%v/started=%v/%s/nested=%v", passthrough, started, code, nested), func(t *testing.T) {
						rec := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
						if started {
							c.Writer.WriteHeaderNow()
						}
						errorJSON := `{"code":"` + code + `","message":"SENSITIVE_SENTINEL","private":"SENSITIVE_SENTINEL"}`
						payload := `{"type":"error","error":` + errorJSON + `}`
						if nested {
							payload = `{"type":"response.failed","response":{"status":"failed","error":` + errorJSON + `,"metadata":{"secret":"SENSITIVE_SENTINEL"}}}`
						}
						stream := "data: " + payload + "\n\ndata: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"name\":\"FORBIDDEN_TOOL\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\ndata: [DONE]\n\n"
						resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}
						svc := newWSFixture(wsFixtureInputs{options: &wsFixtureOptions{Output: OpenAIResponseOptions{MaxLineSize: openAIResponseDefaultMaxLineSize}}})
						account := &gatewayprovider.ExecutionAccount{Record: accountcore.Record{LoadLocation: time.LoadLocation, ID: 12, Platform: capability.PlatformOpenAI, Type: capability.AccountTypeOAuth}}
						var err error
						if passthrough {
							_, err = svc.Output.PassthroughStream(c.Request.Context(), resp, c, account, time.Now(), "synthetic", "synthetic")
						} else {
							_, err = svc.Output.Stream(c.Request.Context(), resp, c, account, time.Now(), "synthetic", "synthetic", "")
						}
						require.Error(t, err)
						var retry *forwardcore.UpstreamFailoverError
						require.False(t, errors.As(err, &retry))
						require.Equal(t, 200, rec.Code)
						require.NotContains(t, rec.Body.String(), "SENSITIVE_SENTINEL")
						require.NotContains(t, rec.Body.String(), "FORBIDDEN_TOOL")
						require.NotContains(t, rec.Body.String(), "response.completed")
						require.NotContains(t, rec.Body.String(), "[DONE]")
						count := 0
						for _, line := range strings.Split(rec.Body.String(), "\n") {
							if !strings.HasPrefix(line, "data: ") {
								continue
							}
							data := strings.TrimPrefix(line, "data: ")
							errorObj := gjson.Get(data, "error")
							if !errorObj.Exists() {
								errorObj = gjson.Get(data, "response.error")
							}
							if !errorObj.Exists() {
								continue
							}
							count++
							require.Equal(t, code, errorObj.Get("code").String())
							require.Equal(t, openai.OpenAISafetyTerminalType(code), errorObj.Get("type").String())
							require.Len(t, errorObj.Map(), 3)
						}
						require.Equal(t, 1, count)
						if code == "session_blocked_by_cyber_policy" {
							require.Nil(t, GetOpsCyberPolicy(c))
						}
					})
				}
			}
		}
	}
}
