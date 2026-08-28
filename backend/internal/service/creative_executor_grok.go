package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// executeGrok 执行 Grok 平台任务：仅支持 generate（xAI images/generations 兼容 OpenAI 协议）。
func (e *CreativeExecutor) executeGrok(ctx context.Context, run CreativeRun, payload CreativeRunPayload, account *Account, upstreamModel string) ([]CreativeOutput, error) {
	if run.Operation != CreativeOperationGenerate {
		return nil, creativeNonRetryableError("grok platform does not support creative operation %s", run.Operation)
	}
	if e.gateway == nil {
		return nil, errors.New("creative grok gateway is not configured")
	}
	targetURL, err := buildGrokMediaURL(account, e.cfg, GrokMediaEndpointImagesGenerations, "")
	if err != nil {
		return nil, creativeHTTPStatusError(0, err.Error())
	}
	token, _, err := e.gateway.GetAccessToken(ctx, account)
	if err != nil {
		return nil, creativeHTTPStatusError(0, err.Error())
	}
	body, err := json.Marshal(buildCreativeGrokRequest(run, payload, upstreamModel))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileGrok))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if account.IsGrokOAuth() && isGrokCLIProxyTarget(targetURL) {
		applyGrokCLIHeaders(req.Header)
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
	return parseCreativeOpenAIImageOutputs(respBody, run.RequestedOutputCount)
}

// buildCreativeGrokRequest 构造 xAI images/generations 请求体。
func buildCreativeGrokRequest(run CreativeRun, payload CreativeRunPayload, upstreamModel string) map[string]any {
	request := map[string]any{
		"model":           upstreamModel,
		"prompt":          payload.Prompt,
		"n":               max(run.RequestedOutputCount, 1),
		"response_format": "b64_json",
		"resolution":      creativeGrokImageResolution(run.ImageSize),
	}
	if aspectRatio := creativeGrokAspectRatio(run.AspectRatio); aspectRatio != "" {
		request["aspect_ratio"] = aspectRatio
	}
	return request
}
