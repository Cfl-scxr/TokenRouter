package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// executeOpenAI 执行 OpenAI 平台任务：generate 走 /v1/images/generations（JSON），
// edit/inpaint 走 /v1/images/edits（multipart，多源图 + mask）。
func (e *CreativeExecutor) executeOpenAI(ctx context.Context, run CreativeRun, payload CreativeRunPayload, account *Account, upstreamModel string) ([]CreativeOutput, error) {
	if e.gateway == nil {
		return nil, errors.New("creative openai gateway is not configured")
	}
	endpoint := openAIImagesGenerationsEndpoint
	if run.Operation != CreativeOperationGenerate {
		endpoint = openAIImagesEditsEndpoint
	}
	body, contentType, err := buildCreativeOpenAIRequestBody(run, payload, upstreamModel)
	if err != nil {
		return nil, err
	}
	token, _, err := e.gateway.GetAccessToken(ctx, account)
	if err != nil {
		return nil, creativeHTTPStatusError(0, err.Error())
	}
	targetURL, err := e.creativeOpenAIURL(account, endpoint)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	authHeaders, err := e.gateway.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, creativeHTTPStatusError(0, err.Error())
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}
	// 账号级请求头覆写最后应用，配置值优先于内置默认头。
	account.ApplyHeaderOverrides(req.Header)

	resp, err := e.gateway.httpUpstream.DoWithTLS(req, accountProxyURL(account), account.ID, account.Concurrency, e.gateway.resolveOpenAITLSProfile(account))
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
	return parseCreativeOpenAIImageOutputs(respBody, run.RequestedOutputCount)
}

// creativeOpenAIURL 推导 OpenAI images 上游 URL：账号自定义 base_url 优先，否则官方端点。
func (e *CreativeExecutor) creativeOpenAIURL(account *Account, endpoint string) (string, error) {
	targetURL := openAIImagesGenerationsURL
	if endpoint == openAIImagesEditsEndpoint {
		targetURL = openAIImagesEditsURL
	}
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		return targetURL, nil
	}
	validatedURL, err := e.gateway.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return "", creativeNonRetryableError("creative openai base url invalid: %s", err.Error())
	}
	return buildOpenAIImagesURL(validatedURL, endpoint), nil
}

// buildCreativeOpenAIRequestBody 构造 OpenAI images 请求体：
// generate 为 JSON；edit/inpaint 为 multipart（image 多文件、mask、model、prompt）。
// quality 仅 OpenAI 平台校验通过，非空时透传给上游。
func buildCreativeOpenAIRequestBody(run CreativeRun, payload CreativeRunPayload, upstreamModel string) ([]byte, string, error) {
	if run.Operation == CreativeOperationGenerate {
		bodyMap := map[string]any{
			"model":           upstreamModel,
			"prompt":          payload.Prompt,
			"n":               max(run.RequestedOutputCount, 1),
			"response_format": "b64_json",
			"size":            creativeOpenAIImageSize(run.ImageSize, run.AspectRatio),
		}
		if quality := strings.TrimSpace(payload.Quality); quality != "" {
			bodyMap["quality"] = quality
		}
		body, err := json.Marshal(bodyMap)
		if err != nil {
			return nil, "", err
		}
		return body, "application/json", nil
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for i, image := range payload.Sources {
		part, err := writer.CreateFormFile("image", fmt.Sprintf("source_%d.%s", i, creativeFileExtension(image.Mime)))
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(image.Bytes); err != nil {
			return nil, "", err
		}
	}
	if payload.Mask != nil {
		part, err := writer.CreateFormFile("mask", "mask."+creativeFileExtension(payload.Mask.Mime))
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(payload.Mask.Bytes); err != nil {
			return nil, "", err
		}
	}
	if err := writer.WriteField("model", upstreamModel); err != nil {
		return nil, "", err
	}
	if err := writer.WriteField("prompt", payload.Prompt); err != nil {
		return nil, "", err
	}
	if err := writer.WriteField("response_format", "b64_json"); err != nil {
		return nil, "", err
	}
	if quality := strings.TrimSpace(payload.Quality); quality != "" {
		if err := writer.WriteField("quality", quality); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

// parseCreativeOpenAIImageOutputs 解析 OpenAI images 响应（grok 同结构）的 data[].b64_json。
func parseCreativeOpenAIImageOutputs(body []byte, maxCount int) ([]CreativeOutput, error) {
	data := gjson.GetBytes(body, "data")
	if !data.IsArray() || len(data.Array()) == 0 {
		return nil, creativeHTTPStatusError(http.StatusBadGateway, "upstream returned no image output")
	}
	outputs := make([]CreativeOutput, 0, len(data.Array()))
	for _, item := range data.Array() {
		b64 := strings.TrimSpace(item.Get("b64_json").String())
		if b64 == "" {
			continue
		}
		decoded, err := decodeBase64Image(b64)
		if err != nil || len(decoded.Bytes) == 0 {
			continue
		}
		outputs = append(outputs, CreativeOutput{Index: len(outputs), Bytes: decoded.Bytes, Mime: decoded.Mime})
		if maxCount > 0 && len(outputs) >= maxCount {
			break
		}
	}
	if len(outputs) == 0 {
		return nil, creativeHTTPStatusError(http.StatusBadGateway, "upstream returned no decodable image output")
	}
	return outputs, nil
}
