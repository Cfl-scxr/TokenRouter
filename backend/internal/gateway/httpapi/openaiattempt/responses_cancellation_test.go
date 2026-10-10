package openaiattempt

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	gatewayhttp "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestResponsesCallerCancellationHasNoFallbackOrHealthFailure(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(fmt.Sprint(started), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(ctx)
			if started {
				_, _ = c.Writer.WriteString("data: partial\n\n")
			}
			before := rec.Body.String()
			cancel()
			core, logs := observer.New(zap.InfoLevel)
			b := &responsesAttemptBridge{c: c, reqLog: zap.New(core)}
			// 未配置账号或健康依赖；若误走处罚或重试分支会失败。
			b.OtherFailure(fmt.Errorf("stream usage incomplete: %w", context.Canceled))
			b.Failed()
			require.True(t, b.callerCanceled)
			require.False(t, b.wroteFallback)
			require.Equal(t, before, rec.Body.String())
			require.Empty(t, gatewayhttp.GetOpsStreamErrors(c))
			for _, entry := range logs.All() {
				require.Equal(t, zap.InfoLevel, entry.Level)
			}
			require.Equal(t, 1, logs.FilterMessage("openai.client_canceled").Len())
			if !started {
				require.Equal(t, gatewayhttp.StatusClientClosedRequest, c.Writer.Status())
			}
		})
	}
}
