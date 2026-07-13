package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	grokConversationIDHeader        = "X-Grok-Conv-Id"
	grokFreeCacheNativeToolsJSON    = `[{"type":"web_search"},{"type":"x_search"}]`
	grokFreeCacheDisabledToolChoice = "none"
)

// resolveGrokCacheIdentity 为 xAI 服务端提示缓存派生稳定且租户隔离的路由身份。
// 返回值不包含客户端原始会话标识，可安全发送到上游。
//
// 必须存在有效的下游 API Key。内部探测或请求上下文不完整时主动关闭缓存，避免生成
// 可能被无关租户共享的缓存身份。
func resolveGrokCacheIdentity(c *gin.Context, body []byte, explicitKey, upstreamModel string) string {
	apiKeyID := getAPIKeyIDFromContext(c)
	if apiKeyID <= 0 {
		return ""
	}
	// /responses/compact 不接受 tool_choice，且不代表正常会话轮次；该路径不得添加
	// 缓存身份或免费层路由增强字段。
	if isOpenAIResponsesCompactPath(c) {
		return ""
	}

	model := strings.ToLower(strings.TrimSpace(upstreamModel))
	if model == "" {
		return ""
	}

	seed := explicitGrokCacheSeed(c, body, explicitKey)
	if seed == "" {
		seed = deriveOpenAIContentSessionSeed(body)
	}
	if seed == "" {
		return ""
	}

	// generateSessionUUID 会先哈希完整种子再格式化为 UUID；加入带版本的命名空间，
	// 避免该身份与 TokenRouter 派生的其它上游会话标识冲突。
	isolatedSeed := fmt.Sprintf("grok-prompt-cache:v1:%d:%s:%s", apiKeyID, model, seed)
	return generateSessionUUID(isolatedSeed)
}

func explicitGrokCacheSeed(c *gin.Context, body []byte, explicitKey string) string {
	seed := ""
	if c != nil {
		seed = strings.TrimSpace(c.GetHeader("session_id"))
		if seed == "" {
			seed = strings.TrimSpace(c.GetHeader("conversation_id"))
		}
		if seed == "" {
			seed = strings.TrimSpace(c.GetHeader(grokConversationIDHeader))
		}
	}
	if seed == "" && len(body) > 0 {
		seed = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	}
	if seed == "" {
		seed = strings.TrimSpace(explicitKey)
	}
	return seed
}

func isGrokRequestContext(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, exists := c.Get("api_key")
	if !exists {
		return false
	}
	apiKey, ok := v.(*APIKey)
	return ok && apiKey != nil && apiKey.Group != nil && apiKey.Group.Platform == PlatformGrok
}

// applyGrokResponsesCacheIdentity 将缓存路由身份写入 xAI Responses 请求。
// 客户端已有值会被租户隔离值替换，防止共享 OAuth 账号上的缓存冲突。
//
// xAI 会把未携带原生搜索工具的免费 OAuth 请求路由到不可缓存的 build-free 模型。
// 对原本无工具的请求添加原生工具并设置 tool_choice=none，可选择支持缓存的层级而不
// 实际执行搜索；客户端明确提供 tools 或 tool_choice 时禁用该增强，以保留函数调用语义。
func applyGrokResponsesCacheIdentity(body, intentSourceBody []byte, identity string, injectFreeTierTools bool) ([]byte, error) {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		if gjson.GetBytes(body, "prompt_cache_key").Exists() {
			return sjson.DeleteBytes(body, "prompt_cache_key")
		}
		return body, nil
	}
	out, err := sjson.SetBytes(body, "prompt_cache_key", identity)
	if err != nil {
		return nil, err
	}
	if !injectFreeTierTools {
		return out, nil
	}
	// 必须检查清理前的原始请求。patchGrokResponsesBody 可能移除不受支持的客户端工具
	// 及其 tool_choice，但不能因此把明确的客户端工具意图误判为可注入原生工具。
	if gjson.GetBytes(intentSourceBody, "tools").Exists() || gjson.GetBytes(intentSourceBody, "tool_choice").Exists() {
		return out, nil
	}
	out, err = sjson.SetRawBytes(out, "tools", []byte(grokFreeCacheNativeToolsJSON))
	if err != nil {
		return nil, err
	}
	return sjson.SetBytes(out, "tool_choice", grokFreeCacheDisabledToolChoice)
}

// applyGrokCacheHeaders 写入 Chat Completions 约定的会话路由头。请求使用全新 header
// 映射构建，因此客户端提供的 x-grok header 无法覆盖服务端派生值。
func applyGrokCacheHeaders(headers http.Header, identity string) {
	if headers == nil {
		return
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		headers.Del(grokConversationIDHeader)
		return
	}
	headers.Set(grokConversationIDHeader, identity)
}

// stripGrokChatPromptCacheKey 在身份种子使用完毕后移除 Responses 专用字段；
// Chat Completions 通过 header 路由缓存。
func stripGrokChatPromptCacheKey(body []byte) ([]byte, error) {
	if !gjson.GetBytes(body, "prompt_cache_key").Exists() {
		return body, nil
	}
	return sjson.DeleteBytes(body, "prompt_cache_key")
}
