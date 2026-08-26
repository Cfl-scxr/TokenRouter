package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/ctxkey"
	"github.com/TokenFlux/TokenRouter/internal/pkg/logger"
	"github.com/TokenFlux/TokenRouter/internal/pkg/servertiming"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	clientRequestIDHeader   = "X-Client-Request-ID"
	internalRequestIDHeader = "X-Sub2API-Request-ID"
)

// ClientRequestID 为请求生成内部关联 ID，并把调用方 ID 单独保存为 parent_client_request_id。
// 外部 ID 只用于跨服务排障，不能影响结算幂等或内部身份判断。
func ClientRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil {
			c.Next()
			return
		}

		// 入口时间必须在读取请求体和账号调度之前记录，便于拆分请求体上传与应用内耗时。
		ctx := c.Request.Context()
		if _, ok := ctx.Value(ctxkey.RequestStartedAt).(time.Time); !ok {
			ctx = context.WithValue(ctx, ctxkey.RequestStartedAt, time.Now())
		}
		ctx = servertiming.WithHTTPTrace(ctx)

		// 已存在的 context 值只可能来自受信任的内部调用；HTTP Header 永远不能覆盖它。
		internalID, valid := normalizeCorrelationIDFromContext(ctx, ctxkey.ClientRequestID)
		if !valid {
			internalID = uuid.NewString()
		}
		parentID, _ := normalizeCorrelationID(c.GetHeader(clientRequestIDHeader))
		ctx = context.WithValue(ctx, ctxkey.ClientRequestID, internalID)
		if parentID != "" {
			ctx = context.WithValue(ctx, ctxkey.ParentClientRequestID, parentID)
		}
		requestLogger := logger.FromContext(ctx).With(zap.String("client_request_id", internalID))
		if parentID != "" {
			requestLogger = requestLogger.With(zap.String("parent_client_request_id", parentID))
		}
		ctx = logger.IntoContext(ctx, requestLogger)
		c.Request = c.Request.WithContext(ctx)
		// 保留合法的调用方 ID供上下游串联；没有调用方 ID 时才回写内部 ID。
		if c.Request.Header == nil {
			c.Request.Header = make(http.Header)
		}
		if parentID != "" {
			c.Request.Header.Set(clientRequestIDHeader, parentID)
			c.Header(clientRequestIDHeader, parentID)
		} else {
			c.Request.Header.Set(clientRequestIDHeader, internalID)
			c.Header(clientRequestIDHeader, internalID)
		}
		// 专用内部头不复用客户端可控的 X-Client-Request-ID，便于下游同时记录两类 ID。
		c.Request.Header.Set(internalRequestIDHeader, internalID)
		c.Header(internalRequestIDHeader, internalID)
		c.Next()
	}
}

func normalizeCorrelationIDFromContext(ctx context.Context, key ctxkey.Key) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v, _ := ctx.Value(key).(string)
	return normalizeCorrelationID(v)
}
