package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/geminicli"
	"github.com/tidwall/gjson"
)

// creativeGeminiGenerationConfig 是创作台 Gemini generateContent 的生成配置。
type creativeGeminiGenerationConfig struct {
	ResponseModalities []string `json:"responseModalities"`
	ImageSize          string   `json:"imageSize,omitempty"`
	AspectRatio        string   `json:"aspectRatio,omitempty"`
	ResponseMimeType   string   `json:"responseMimeType,omitempty"`
}

// creativeGeminiGenerateRequest 是创作台 Gemini generateContent 请求体。
type creativeGeminiGenerateRequest struct {
	Contents         []geminiContent                `json:"contents"`
	GenerationConfig creativeGeminiGenerationConfig `json:"generationConfig"`
}

// executeGemini 执行 Gemini 平台任务（含 vertex 服务账号与 AI Studio apikey/OAuth）。
// 统一使用原生 generateContent：prompt 与源图/mask 以 inlineData 放入 parts。
func (e *CreativeExecutor) executeGemini(ctx context.Context, run CreativeRun, payload CreativeRunPayload, account *Account, upstreamModel string) ([]CreativeOutput, error) {
	if e.gateway == nil {
		return nil, errors.New("creative gemini gateway is not configured")
	}
	request := buildCreativeGeminiRequest(run, payload, upstreamModel)
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	var targetURL string
	projectID := strings.TrimSpace(account.GetCredential("project_id"))
	switch {
	case account.Type == AccountTypeServiceAccount:
		// Vertex 服务账号：{location}-aiplatform.../v1/projects/.../models/{model}:generateContent + Bearer。
		targetURL, err = buildVertexGeminiURL(account.VertexProjectID(), account.VertexLocation(upstreamModel), upstreamModel, "generateContent", false)
		if err != nil {
			return nil, creativeNonRetryableError("build vertex gemini url: %s", err.Error())
		}
	case account.Type == AccountTypeOAuth && projectID != "":
		// Code Assist：v1internal 包装请求。
		baseURL, validateErr := e.gateway.validateUpstreamBaseURL(geminicli.GeminiCliBaseURL)
		if validateErr != nil {
			return nil, creativeHTTPStatusError(0, validateErr.Error())
		}
		targetURL = strings.TrimRight(baseURL, "/") + "/v1internal:generateContent"
		wrapped := map[string]any{"model": upstreamModel, "project": projectID}
		var inner any
		if err := json.Unmarshal(body, &inner); err != nil {
			return nil, err
		}
		wrapped["request"] = inner
		body, err = json.Marshal(wrapped)
		if err != nil {
			return nil, err
		}
	case account.Type == AccountTypeAPIKey:
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return nil, creativeNonRetryableError("gemini api_key not configured")
		}
		baseURL := e.gateway.validateGeminiBaseURL(account.GetGeminiBaseURL(geminicli.AIStudioBaseURL))
		targetURL, err = buildGeminiAIStudioModelActionURL(baseURL, upstreamModel, "generateContent", false)
		if err != nil {
			return nil, creativeNonRetryableError("build gemini url: %s", err.Error())
		}
	default:
		// OAuth（无 project_id）：AI Studio Bearer 模式。
		baseURL := e.gateway.validateGeminiBaseURL(account.GetGeminiBaseURL(geminicli.AIStudioBaseURL))
		targetURL, err = buildGeminiAIStudioModelActionURL(baseURL, upstreamModel, "generateContent", false)
		if err != nil {
			return nil, creativeNonRetryableError("build gemini url: %s", err.Error())
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := e.applyGeminiAuth(ctx, req, account); err != nil {
		return nil, err
	}
	// 账号级请求头覆写最后应用，配置值优先于内置默认头。
	account.ApplyHeaderOverrides(req.Header)

	resp, err := e.gateway.httpUpstream.Do(req, accountProxyURL(account), account.ID, account.Concurrency)
	if err != nil {
		return nil, creativeHTTPStatusError(0, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := readCreativeUpstreamBody(resp.Body, 64<<20)
	if err != nil {
		return nil, creativeHTTPStatusError(0, err.Error())
	}
	if resp.StatusCode >= 400 {
		return nil, creativeHTTPStatusError(resp.StatusCode, extractUpstreamErrorMessage(respBody))
	}
	// Code Assist 模式的响应包裹在 response 字段内。
	if account.Type == AccountTypeOAuth && projectID != "" {
		respBody, err = unwrapGeminiResponse(respBody)
		if err != nil {
			return nil, creativeHTTPStatusError(http.StatusBadGateway, err.Error())
		}
	}
	return parseCreativeGeminiImageOutputs(respBody, run.RequestedOutputCount)
}

// validateGeminiBaseURL 校验 Gemini base URL；失败时回退官方 AI Studio 地址。
func (s *OpenAIGatewayService) validateGeminiBaseURL(raw string) string {
	validated, err := s.validateUpstreamBaseURL(raw)
	if err != nil || strings.TrimSpace(validated) == "" {
		return geminicli.AIStudioBaseURL
	}
	return validated
}

// applyGeminiAuth 按账号类型设置鉴权头：apikey 用 x-goog-api-key，其余用 Bearer token。
func (e *CreativeExecutor) applyGeminiAuth(ctx context.Context, req *http.Request, account *Account) error {
	if account.Type == AccountTypeAPIKey {
		apiKey := strings.TrimSpace(account.GetCredential("api_key"))
		if apiKey == "" {
			return creativeNonRetryableError("gemini api_key not configured")
		}
		req.Header.Set("x-goog-api-key", apiKey)
		return nil
	}
	if e.geminiTokens == nil {
		return creativeNonRetryableError("gemini token provider is not configured")
	}
	token, err := e.geminiTokens.GetAccessToken(ctx, account)
	if err != nil {
		return creativeHTTPStatusError(0, err.Error())
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if account.Type == AccountTypeOAuth {
		req.Header.Set("User-Agent", geminicli.GeminiCLIUserAgent)
	}
	return nil
}

// buildCreativeGeminiRequest 构造 Gemini generateContent 请求体：
// parts 为 prompt 文本 + 每张源图 inlineData；inpaint 时 mask 作为额外图片附加，prompt 描述重绘区域。
func buildCreativeGeminiRequest(run CreativeRun, payload CreativeRunPayload, upstreamModel string) creativeGeminiGenerateRequest {
	parts := make([]geminiPart, 0, len(payload.Sources)+2)
	if prompt := strings.TrimSpace(payload.Prompt); prompt != "" {
		parts = append(parts, geminiPart{Text: prompt})
	}
	for _, source := range payload.Sources {
		mime := strings.TrimSpace(source.Mime)
		if mime == "" {
			mime = "image/png"
		}
		parts = append(parts, geminiPart{InlineData: &geminiInlineData{
			MimeType: mime,
			Data:     base64.StdEncoding.EncodeToString(source.Bytes),
		}})
	}
	if payload.Mask != nil {
		mime := strings.TrimSpace(payload.Mask.Mime)
		if mime == "" {
			mime = "image/png"
		}
		parts = append(parts, geminiPart{InlineData: &geminiInlineData{
			MimeType: mime,
			Data:     base64.StdEncoding.EncodeToString(payload.Mask.Bytes),
		}})
	}
	config := creativeGeminiGenerationConfig{
		ResponseModalities: []string{"TEXT", "IMAGE"},
		ImageSize:          strings.TrimSpace(run.ImageSize),
		AspectRatio:        strings.TrimSpace(run.AspectRatio),
		ResponseMimeType:   strings.TrimSpace(run.ResponseMIMEType),
	}
	return creativeGeminiGenerateRequest{
		Contents:         []geminiContent{{Parts: parts}},
		GenerationConfig: config,
	}
}

// parseCreativeGeminiImageOutputs 从 generateContent 响应中提取 inlineData 图片输出。
func parseCreativeGeminiImageOutputs(body []byte, maxCount int) ([]CreativeOutput, error) {
	parts := gjson.GetBytes(body, "candidates.0.content.parts")
	if !parts.IsArray() || len(parts.Array()) == 0 {
		return nil, creativeHTTPStatusError(http.StatusBadGateway, "gemini upstream returned no content parts")
	}
	outputs := make([]CreativeOutput, 0, len(parts.Array()))
	for _, part := range parts.Array() {
		inline := part.Get("inlineData")
		if !inline.Exists() {
			inline = part.Get("inline_data")
		}
		if !inline.Exists() {
			continue
		}
		raw := strings.TrimSpace(inline.Get("data").String())
		if raw == "" {
			continue
		}
		decoded, err := decodeBase64Image(raw)
		if err != nil || len(decoded.Bytes) == 0 {
			continue
		}
		mime := strings.TrimSpace(inline.Get("mimeType").String())
		if mime == "" {
			mime = strings.TrimSpace(inline.Get("mime_type").String())
		}
		if mime == "" {
			mime = decoded.Mime
		}
		outputs = append(outputs, CreativeOutput{Index: len(outputs), Bytes: decoded.Bytes, Mime: mime})
		if maxCount > 0 && len(outputs) >= maxCount {
			break
		}
	}
	if len(outputs) == 0 {
		return nil, creativeHTTPStatusError(http.StatusBadGateway, "gemini upstream returned no image output")
	}
	return outputs, nil
}
