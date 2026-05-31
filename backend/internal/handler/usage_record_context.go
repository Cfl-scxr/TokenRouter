package handler

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
)

func usageRecordContextFromGin(c *gin.Context) context.Context {
	dst := context.Background()
	if c == nil || c.Request == nil {
		return dst
	}
	src := c.Request.Context()
	for _, key := range []any{
		ctxkey.RequestID,
		ctxkey.ClientRequestID,
	} {
		if value := src.Value(key); value != nil {
			dst = context.WithValue(dst, key, value)
		}
	}
	return dst
}

func wrapUsageRecordTaskContext(c *gin.Context, task func(context.Context)) func(context.Context) {
	if task == nil {
		return nil
	}
	requestCtx := usageRecordContextFromGin(c)
	return func(workerCtx context.Context) {
		base := workerCtx
		if base == nil {
			base = context.Background()
		}
		for _, key := range []any{
			ctxkey.RequestID,
			ctxkey.ClientRequestID,
		} {
			if value := requestCtx.Value(key); value != nil {
				base = context.WithValue(base, key, value)
			}
		}
		task(base)
	}
}
