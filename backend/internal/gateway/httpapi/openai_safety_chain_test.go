//go:build unit

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	accountcore "github.com/TokenFlux/TokenRouter/internal/account"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/openaiforward"
	textflow "github.com/TokenFlux/TokenRouter/internal/gateway/text"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSafetyTerminalChainServer 仅供断网容器内的跨层验收启动，普通单测跳过。
// 账号和上游是合成夹具；请求编排、HTTP 交换、流读取及错误输出使用真实实现。
func TestSafetyTerminalChainServer(t *testing.T) {
	if os.Getenv("SUBR_POLICY_CHAIN_FIXTURE") != "synthetic-loopback-only" {
		t.Skip("requires the isolated cross-layer harness")
	}
	_, err := os.Stat("/.dockerenv")
	require.NoError(t, err)
	interfaces, err := os.ReadDir("/sys/class/net")
	require.NoError(t, err)
	require.Len(t, interfaces, 1)
	require.Equal(t, "lo", interfaces[0].Name())
	var requests, attempts, active atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]int64{"requests": requests.Load(), "attempts": attempts.Load(), "active": active.Load()})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		active.Add(1)
		defer active.Add(-1)
		body, readErr := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
		if readErr != nil {
			http.Error(w, "synthetic input read failed", 400)
			return
		}
		c, _ := gin.CreateTestContext(w)
		c.Request = r
		svc := newWSFixture(wsFixtureInputs{options: &wsFixtureOptions{Output: OpenAIResponseOptions{MaxLineSize: openAIResponseDefaultMaxLineSize}}})
		p := &safetyChainAttempts{safetyTerminalAttempts: &safetyTerminalAttempts{ctx: r.Context()}}
		p.forward = func(id int64) error {
			attempts.Add(1)
			account := &gatewayprovider.ExecutionAccount{Record: accountcore.Record{LoadLocation: time.LoadLocation, ID: id, Platform: capability.PlatformOpenAI, Type: capability.AccountTypeAPIKey}}
			channel := "a"
			if id == 2 || strings.HasPrefix(r.URL.Path, "/b/") {
				channel = "b"
			}
			options := openaiforward.HTTPOptions{
				Sink:                  ResponseSink{Writer: c.Writer},
				StreamOptions:         func() openai.StreamOptions { return svc.Output.StreamOptions(r.Context(), c, account, "") },
				WrapResponseBody:      func(*http.Response) {},
				ExtractServiceTier:    func([]byte) *string { return nil },
				ReleaseDecodedRequest: func() {},
				BindResponseOwner:     func(context.Context, string) {},
				ObservedServiceTier:   func() string { return "" },
				ResolvedServiceTier:   func(tier *string) *string { return tier },
				CompactSignal:         func(error) (openaiforward.CompactFailure, bool) { return openaiforward.CompactFailure{}, false },
				Exchange: openai.HTTPExchangeOptions{
					RequestContext: func(ctx context.Context) (context.Context, context.CancelFunc) { return ctx, func() {} },
					Build: func(ctx context.Context, input []byte) (*http.Request, error) {
						return http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:4101/"+channel+"/v1/responses", strings.NewReader(string(input)))
					},
					ApplyHeaders: func(h http.Header) { h.Set("Content-Type", "application/json") },
					Latency:      func(time.Duration) {}, TransportError: func(err error) error { return err },
					Do: (&http.Client{Timeout: 15 * time.Second}).Do,
				},
				ReadErrorBody:    svc.Output.ReadErrorBody,
				IsAgentIdentity:  func(context.Context) bool { return false },
				InvalidAgentTask: func(int, []byte) bool { return false },
				RecoverAgentTask: func(context.Context) error { return errors.New("unexpected recovery") },
				RedactErrorBody:  func(_ context.Context, b []byte) []byte { return b },
				ErrorDetails: func(b []byte) (string, string) {
					return gjson.GetBytes(b, "error.message").String(), gjson.GetBytes(b, "error.code").String()
				},
				CompactRetry:   func([]byte, int, string, []byte, bool) ([]byte, string, bool) { return nil, "", false },
				ShouldFailover: gatewayprovider.ShouldFailoverOpenAIResponse,
				HTTPFailover: func(resp *http.Response, b []byte, msg, _ string) error {
					return gatewayprovider.NewOpenAIUpstreamFailure(resp.StatusCode, resp.Header, b, msg, false)
				},
				ErrorResponse: func(resp *http.Response, b []byte, model string) error {
					_, err := svc.Output.ResponseError(r.Context(), resp, c, account, b, model)
					return err
				},
				Log: func(string, ...any) {},
			}
			_, err := openaiforward.RunHTTP(r.Context(), openaiforward.HTTPInput{Body: body, Stream: true, StartedAt: time.Now(), OriginalModel: "primary", UpstreamModel: "primary"}, options)
			return err
		}
		textflow.RunResponses(textflow.ResponseOptions{MaxSwitches: 1}, p)
	})
	server := &http.Server{Addr: "127.0.0.1:4201", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	require.NoError(t, server.ListenAndServe())
}

// 串联夹具也覆盖成功收尾；原拒绝单测仍对意外成功保持失败关闭。
type safetyChainAttempts struct{ *safetyTerminalAttempts }

func (p *safetyChainAttempts) Success()      {}
func (p *safetyChainAttempts) Completed(int) {}
