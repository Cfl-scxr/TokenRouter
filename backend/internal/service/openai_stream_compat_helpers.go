package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/TokenFlux/TokenRouter/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// effectiveOpenAISSEEventType 优先使用 payload 内的 type，兼容 event 行与 data 行分离的 SSE。
func effectiveOpenAISSEEventType(payload []byte, eventType string) string {
	if value := strings.TrimSpace(gjson.GetBytes(payload, "type").String()); value != "" {
		return value
	}
	return strings.TrimSpace(eventType)
}

// (s *OpenAIGatewayService) parseSSEUsageBytesWithType 兼容带 event 类型的用量解析入口。
// 旧实现没有 event 参数，因此先复用原有解析器，保持已有字段合并语义。
func (s *OpenAIGatewayService) parseSSEUsageBytesWithType(data []byte, _ string, usage *OpenAIUsage) {
	if usage == nil || len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return
	}
	s.parseSSEUsageBytes(data, usage)
}

var openAIMissingUsageTotal atomic.Uint64

// logOpenAISuccessMissingUsage 记录成功响应缺失 usage 的低频诊断，避免影响请求路径。
func logOpenAISuccessMissingUsage(ctx context.Context, c *gin.Context, account *Account, resp *http.Response, usage *OpenAIUsage, terminalEvent string, clientDisconnected bool) {
	if resp == nil || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.ImageOutputTokens > 0) {
		return
	}
	terminalEvent = strings.TrimSpace(terminalEvent)
	if terminalEvent != "response.completed" && terminalEvent != "response.done" && terminalEvent != "json" && terminalEvent != "[DONE]" {
		return
	}
	count := openAIMissingUsageTotal.Add(1)
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	logger.FromContext(ctx).With(
		zap.Int64("account_id", accountID),
		zap.String("terminal_event", terminalEvent),
		zap.Bool("client_disconnected", clientDisconnected),
		zap.Uint64("missing_usage_total", count),
	).Debug("openai_usage.success_missing_usage")
	_ = c
}

// buildOpenAIResponseFailedSSE 构造可被客户端解析的 Responses 失败事件。
func buildOpenAIResponseFailedSSE(responseID, model string, source []byte, fallbackMessage string) string {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		responseID = "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	message := strings.TrimSpace(extractOpenAISSEErrorMessage(source))
	if message == "" {
		message = strings.TrimSpace(fallbackMessage)
	}
	if message == "" {
		message = "Upstream response failed"
	}
	code := strings.TrimSpace(gjson.GetBytes(source, "error.code").String())
	if code == "" {
		code = strings.TrimSpace(gjson.GetBytes(source, "response.error.code").String())
	}
	if code == "" {
		code = "upstream_error"
	}
	errorBody := map[string]any{"code": code, "message": message}
	if errorType := strings.TrimSpace(gjson.GetBytes(source, "error.type").String()); errorType != "" {
		errorBody["type"] = errorType
	}
	response := map[string]any{
		"id":     responseID,
		"object": "response",
		"model":  strings.TrimSpace(model),
		"status": "failed",
		"error":  errorBody,
	}
	body, _ := json.Marshal(map[string]any{"type": "response.failed", "response": response})
	return "data: " + string(body) + "\n\n"
}

// openAIStreamGenericFailedEventPayload 返回客户端可识别的通用失败事件。
func openAIStreamGenericFailedEventPayload(_ ...[]byte) []byte {
	return []byte(`{"type":"response.failed","response":{"error":{"message":"Upstream gateway error"}}}`)
}

func openAIUsageHasTokens(usage *OpenAIUsage) bool {
	return usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.ImageOutputTokens > 0 ||
		usage.CacheCreationInputTokens > 0 || usage.CacheReadInputTokens > 0 || usage.ImageInputTokens > 0)
}

// openAIRequestPayloadView 解包 Responses WS 事件，返回实际请求对象视图。
func openAIRequestPayloadView(body []byte) gjson.Result {
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return root
	}
	if root.Get("type").String() == "response.create" && root.Get("response").IsObject() {
		return root.Get("response")
	}
	return root
}
