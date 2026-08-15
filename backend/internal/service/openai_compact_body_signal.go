package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// openAINativeCompactionV2Key 标记本请求是原生 remote compaction v2。handler
// 在识别出真实 wire 形状后写入，出站请求构造器据此补齐协商头。
const openAINativeCompactionV2Key = "openai_native_compaction_v2"

const openAIRemoteCompactionV2Feature = "remote_compaction_v2"

// MarkOpenAINativeCompactionV2 标记当前请求为原生 V2 压缩协议。
func MarkOpenAINativeCompactionV2(c *gin.Context) {
	if c != nil {
		c.Set(openAINativeCompactionV2Key, true)
	}
}

func isOpenAINativeCompactionV2(c *gin.Context) bool {
	return c != nil && c.GetBool(openAINativeCompactionV2Key)
}

// ensureOpenAIRemoteCompactionV2BetaFeature 确保协商头包含 V2 能力，同时保留
// 客户端已声明的其它能力，避免把多行头压缩时丢失 token。
func ensureOpenAIRemoteCompactionV2BetaFeature(h http.Header) {
	if h == nil {
		return
	}
	tokens := make([]string, 0, 4)
	for _, value := range h.Values("x-codex-beta-features") {
		for _, token := range strings.Split(value, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if token == openAIRemoteCompactionV2Feature {
				return
			}
			tokens = append(tokens, token)
		}
	}
	tokens = append(tokens, openAIRemoteCompactionV2Feature)
	h.Set("x-codex-beta-features", strings.Join(tokens, ","))
}

func hasOpenAICodexBetaFeaturesHeader(h http.Header) bool {
	if h == nil {
		return false
	}
	for _, value := range h.Values("x-codex-beta-features") {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

// applyOpenAICodexBetaFeatures 对齐 Codex 的会话级协商行为：原生 V2 压缩请求
// 总会带 remote_compaction_v2；其它 OAuth Responses 请求仅在客户端未声明时补
// 默认能力。非 OAuth 上游不接收额外会话级能力头。
func applyOpenAICodexBetaFeatures(c *gin.Context, account *Account, h http.Header) {
	if h == nil {
		return
	}
	if isOpenAINativeCompactionV2(c) {
		ensureOpenAIRemoteCompactionV2BetaFeature(h)
		return
	}
	if account == nil || !account.IsOpenAIOAuth() || hasOpenAICodexBetaFeaturesHeader(h) {
		return
	}
	h.Set("x-codex-beta-features", openAIRemoteCompactionV2Feature)
}

// HasCompactionTriggerInInput 检测 input 中 type="compaction_trigger" 的条目。
// handler 会结合请求路径和 stream 字段，区分原生 remote compaction v2 流式协议
// 与旧的 /responses/compact 桥接协议。
func HasCompactionTriggerInInput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "compaction_trigger" {
			found = true
			return false
		}
		return true
	})
	return found
}
