package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	accountcore "github.com/TokenFlux/TokenRouter/internal/account"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/gin-gonic/gin"
)

func TestStreamAuditCancelAfterOutputStopsHeldBodyWithoutPenalty(t *testing.T) {
	for _, firstTimeout := range []int{0, 3} {
		t.Run(map[int]string{0: "synchronous_reader", 3: "guarded_reader"}[firstTimeout], func(t *testing.T) {
			svc := newResponsesFixture(responsesFixtureInputs{options: &responsesFixtureOptions{Response: OpenAIResponseOptions{Configured: true, OpenAIFirstOutputTimeoutSeconds: firstTimeout, MaxLineSize: openAIResponseDefaultMaxLineSize}}})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			proxyID := int64(90871)
			account := &gatewayprovider.ExecutionAccount{Record: accountcore.Record{LoadLocation: time.LoadLocation, ID: 9087101, Platform: capability.PlatformOpenAI, Type: capability.AccountTypeOAuth, ProxyID: &proxyID}}
			reader, writer := io.Pipe()
			defer reader.Close()
			stop := make(chan struct{})
			defer close(stop)
			go func() {
				defer writer.Close()
				io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
				time.Sleep(50 * time.Millisecond)
				cancel()
				select {
				case <-stop:
				case <-time.After(2500 * time.Millisecond):
				}
			}()
			started := time.Now()
			result, err := svc.Output.Stream(ctx, &http.Response{StatusCode: 200, Header: http.Header{}, Body: reader}, c, account, started, "model", "model", "")
			t.Logf("elapsed_ms=%d cancellation_error=%v", time.Since(started).Milliseconds(), err)
			if time.Since(started) > 500*time.Millisecond {
				t.Error("cancellation left upstream reader occupied")
			}
			if err == nil {
				t.Error("canceled partial stream must not count as complete")
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("must preserve caller cancellation: %v", err)
			}
			if result == nil {
				t.Error("partial stream observation was discarded")
			}
			if strings.Contains(rec.Body.String(), "stream_timeout") || strings.Contains(rec.Body.String(), "stream_read_error") {
				t.Error("caller cancellation injected upstream error")
			}
			compatible, reason := selectionDiagnosticForStreamTest(t, svc, account)
			if !compatible || reason != "" {
				t.Errorf("caller cancellation penalized channel: %s", reason)
			}
		})
	}
}

// 临时隔离诊断：只使用合成事件，验证结束与取消边界，不绑定生产入口。
type streamAuditFailingWriter struct{ gin.ResponseWriter }

func (w streamAuditFailingWriter) Write(p []byte) (int, error) { return 0, io.ErrClosedPipe }

func TestStreamAuditTerminalMustFinishWithoutWaitingForEOF(t *testing.T) {
	for _, tc := range []struct {
		name            string
		guard, interval int
		failWriter      bool
	}{
		{"sync_held_body", 0, 0, false},
		{"guarded_held_body", 3, 0, false},
		{"held_body_with_idle_timeout", 3, 1, false},
		{"writer_disconnected_held_body", 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newResponsesFixture(responsesFixtureInputs{options: &responsesFixtureOptions{Response: OpenAIResponseOptions{
				Configured: true, OpenAIFirstOutputTimeoutSeconds: tc.guard, StreamDataIntervalTimeout: tc.interval, MaxLineSize: openAIResponseDefaultMaxLineSize,
			}}})
			reader, writer := io.Pipe()
			defer reader.Close()
			stop := make(chan struct{})
			defer close(stop)
			go func() {
				defer writer.Close()
				io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n")
				io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_audit\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
				select {
				case <-stop:
				case <-time.After(2500 * time.Millisecond):
				}
			}()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			if tc.failWriter {
				c.Writer = streamAuditFailingWriter{c.Writer}
			}
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			start := time.Now()
			result, err := svc.Output.Stream(c.Request.Context(), &http.Response{StatusCode: 200, Header: http.Header{}, Body: reader}, c,
				&gatewayprovider.ExecutionAccount{Record: accountcore.Record{LoadLocation: time.LoadLocation, ID: 1, Platform: capability.PlatformOpenAI}}, start, "model", "model", "")
			elapsed := time.Since(start)
			t.Logf("elapsed_ms=%d completed_delivered=%t timeout_after_completed=%t error=%v", elapsed.Milliseconds(), strings.Contains(rec.Body.String(), "response.completed"), strings.Contains(rec.Body.String(), "stream_timeout"), err)
			if err != nil {
				t.Errorf("completed stream must remain successful: %v", err)
			}
			if result == nil || result.Usage == nil || result.Usage.InputTokens != 1 || result.Usage.OutputTokens != 1 {
				t.Error("terminal usage was lost")
			}
			if elapsed > 500*time.Millisecond {
				t.Errorf("terminal event did not release attempt promptly: %v", elapsed)
			}
		})
	}
}

func TestStreamAuditClientCancelMustCancelHeaderWait(t *testing.T) {
	transport := &blockingOpenAIResponseHeaderUpstream{canceled: make(chan struct{})}
	svc := newResponsesFixture(responsesFixtureInputs{options: &responsesFixtureOptions{Response: OpenAIResponseOptions{
		OpenAIFirstOutputTimeoutSeconds: 1, MaxLineSize: openAIResponseDefaultMaxLineSize,
	}}, transport: transport})
	body := []byte(`{"model":"gpt-5.5","stream":true,"reasoning":{"effort":"low"},"input":"synthetic cancellation diagnostic"}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body)).WithContext(ctx)
	account := &gatewayprovider.ExecutionAccount{Record: accountcore.Record{LoadLocation: time.LoadLocation, ID: 1, Name: "audit", Platform: capability.PlatformOpenAI, Type: capability.AccountTypeOAuth, Status: billing.StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"access_token": "synthetic-not-a-secret", "chatgpt_account_id": "synthetic"}}}
	time.AfterFunc(100*time.Millisecond, cancel)
	start := time.Now()
	_, err := svc.Forward(ctx, c, account, body)
	elapsed := time.Since(start)
	t.Logf("elapsed_ms=%d cancel_at_ms=100 error=%v", elapsed.Milliseconds(), err)
	if elapsed > 500*time.Millisecond {
		t.Errorf("caller cancellation failed to interrupt header wait: %v", elapsed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("caller cancellation became another failure: %v", err)
	}
}
