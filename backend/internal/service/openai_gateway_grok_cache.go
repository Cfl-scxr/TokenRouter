package service

import (
	"encoding/json"
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
	grokFreeRolling24hTokenLimit    = int64(2_000_000)
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
		seed = deriveOpenAIStablePrefixSessionSeed(body)
		if seed == "" {
			// 仅使用模型会让缓存路由范围过大；没有可复用前缀时回退到首个用户输入派生身份，
			// 避免无关提示共享同一个租户级缓存键。
			seed = deriveOpenAIAnchoredContentSessionSeed(body)
		}
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
// 实际执行搜索；客户端明确提供的函数工具由下方混合工具策略处理，该策略覆盖
// Messages 桥接、原生 Responses 和 WS HTTP bridge。
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

// applyGrokFreeMessagesFunctionToolCacheRoute 对支持 Responses 函数工具的入口且已知
// 为 Free 的账号启用 xAI 可缓存混合工具路由。auto 选择会允许原生工具被调用，因此不得
// 将该策略隐式扩展到付费账号、未知层级账号或其它工具结构。
func applyGrokFreeMessagesFunctionToolCacheRoute(body, intentSourceBody []byte, account *Account, cacheIdentity string) ([]byte, error) {
	if strings.TrimSpace(cacheIdentity) == "" || !isKnownGrokFreeAccount(account) {
		return body, nil
	}
	intentTools := gjson.GetBytes(intentSourceBody, "tools")
	intentToolChoice := gjson.GetBytes(intentSourceBody, "tool_choice")
	if !isGrokFreeCacheFunctionToolIntent(intentTools, intentToolChoice) {
		return body, nil
	}
	return appendMissingGrokFreeCacheNativeTools(body)
}

func isKnownGrokFreeAccount(account *Account) bool {
	if account == nil || !account.IsGrokOAuth() {
		return false
	}
	freeSignal := false
	paidSignal := false
	inferredFreeSignal := false
	if billing, err := grokBillingSnapshotFromExtra(account.Extra); err == nil && billing != nil {
		if tier := strings.TrimSpace(billing.Plan); tier != "" {
			if isGrokFreeSubscriptionTier(tier) {
				freeSignal = true
			} else if !isGrokUnknownSubscriptionTier(tier) {
				paidSignal = true
			}
		}
		if billing.UsagePercent != nil || billing.UsedPercent != nil ||
			(billing.MonthlyLimitCents != nil && *billing.MonthlyLimitCents > 0) {
			paidSignal = true
		}
		// xAI 会故意为 Free 账号返回空 plan，只有付费订阅才带 SuperGrok plan/月度限额。
		// 因此，成功且没有付费信号的月度计费观测是 Free 的正向证据，而不是未知层级；
		// 部分探测仍按关闭策略处理。
		if strings.TrimSpace(billing.MonthlyUpdatedAt) != "" ||
			(billing.StatusCode >= http.StatusOK && billing.StatusCode < http.StatusMultipleChoices &&
				!billing.Partial && len(billing.FailedWindows) == 0) {
			inferredFreeSignal = true
		}
	}
	if snapshot, err := grokQuotaSnapshotFromExtra(account.Extra); err == nil && snapshot != nil {
		if tier := strings.TrimSpace(snapshot.SubscriptionTier); tier != "" {
			if isGrokFreeSubscriptionTier(tier) {
				freeSignal = true
			} else if !isGrokUnknownSubscriptionTier(tier) {
				paidSignal = true
			}
		}
		if snapshot.Tokens != nil && snapshot.Tokens.Limit != nil &&
			*snapshot.Tokens.Limit == grokFreeRolling24hTokenLimit {
			inferredFreeSignal = true
		}
	}
	if tier := strings.TrimSpace(account.GetCredential("subscription_tier")); tier != "" {
		if isGrokFreeSubscriptionTier(tier) {
			freeSignal = true
		} else if !isGrokUnknownSubscriptionTier(tier) {
			paidSignal = true
		}
	}
	// 明确的付费证据始终覆盖推断的 Free 信号，避免已升级但快照陈旧的账号仍携带历史
	// 200 万 Free token 限额而被误判。
	return !paidSignal && (freeSignal || inferredFreeSignal)
}

func isGrokFreeSubscriptionTier(tier string) bool {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "free", "grok-free", "grok_free", "free-tier", "free_tier", "basic", "grok-basic", "grok_basic":
		return true
	default:
		return false
	}
}

func isGrokUnknownSubscriptionTier(tier string) bool {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "", "unknown", "n/a", "none":
		return true
	default:
		return false
	}
}

func isGrokFreeCacheFunctionToolIntent(tools, toolChoice gjson.Result) bool {
	if !tools.IsArray() {
		return false
	}
	items := tools.Array()
	if len(items) == 0 {
		return false
	}
	for _, tool := range items {
		if !tool.IsObject() || strings.TrimSpace(tool.Get("type").String()) != "function" {
			return false
		}
		// Responses 函数声明的 name 位于顶层；拒绝 Chat Completions 嵌套结构和不完整声明。
		if strings.TrimSpace(tool.Get("name").String()) == "" || tool.Get("function").Exists() {
			return false
		}
	}
	if !toolChoice.Exists() {
		return true
	}
	return toolChoice.Type == gjson.String && strings.TrimSpace(toolChoice.String()) == "auto"
}

func appendMissingGrokFreeCacheNativeTools(body []byte) ([]byte, error) {
	tools := gjson.GetBytes(body, "tools")
	if !tools.Exists() || !tools.IsArray() {
		return body, nil
	}

	items := tools.Array()
	if len(items) == 0 {
		return body, nil
	}
	merged := make([]json.RawMessage, 0, len(items)+2)
	present := make(map[string]bool, 2)
	hasFunction := false
	for _, tool := range items {
		toolType := strings.TrimSpace(tool.Get("type").String())
		switch toolType {
		case "function":
			name := strings.TrimSpace(tool.Get("name").String())
			if !tool.IsObject() || name == "" || tool.Get("function").Exists() {
				return body, nil
			}
			// Grok Build 可能把搜索声明成函数工具；转换为原生工具后既能让 Free OAuth
			// 保持可缓存路由，也能避免同名工具重复。
			if name == "web_search" || name == "x_search" {
				if present[name] {
					continue
				}
				raw, err := json.Marshal(map[string]string{"type": name})
				if err != nil {
					return nil, err
				}
				merged = append(merged, raw)
				present[name] = true
				continue
			}
			hasFunction = true
			merged = append(merged, json.RawMessage(tool.Raw))
		case "web_search", "x_search":
			// 重试该辅助函数时跳过已经存在的原生工具，保证转换操作幂等。
			if present[toolType] {
				continue
			}
			merged = append(merged, json.RawMessage(tool.Raw))
			present[toolType] = true
		default:
			return body, nil
		}
	}
	if !hasFunction {
		return body, nil
	}
	// 仅当请求已有原生或函数形式的搜索工具时补齐缺失项；纯客户端函数工具
	// （例如 view_image）不能触发注入，否则会干扰模型的工具选择。
	if !present["web_search"] && !present["x_search"] {
		return body, nil
	}
	for _, toolType := range []string{"web_search", "x_search"} {
		if present[toolType] {
			continue
		}
		raw, err := json.Marshal(map[string]string{"type": toolType})
		if err != nil {
			return nil, err
		}
		merged = append(merged, raw)
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(body, "tools", encoded)
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
