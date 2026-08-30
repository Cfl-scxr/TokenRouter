//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseCreativeOpenAIImageOutputs 解析 OpenAI/grok 兼容响应的 data[].b64_json。
func TestParseCreativeOpenAIImageOutputs(t *testing.T) {
	img1 := base64.StdEncoding.EncodeToString([]byte("png-bytes-1"))
	img2 := base64.StdEncoding.EncodeToString([]byte("png-bytes-2"))
	body, err := json.Marshal(map[string]any{
		"created": 123,
		"data": []map[string]any{
			{"b64_json": img1},
			{"b64_json": img2},
		},
	})
	require.NoError(t, err)

	outputs, err := parseCreativeOpenAIImageOutputs(body, 4)
	require.NoError(t, err)
	require.Len(t, outputs, 2)
	require.Equal(t, 0, outputs[0].Index)
	require.Equal(t, []byte("png-bytes-1"), outputs[0].Bytes)
	require.Equal(t, "image/png", outputs[0].Mime)
	require.Equal(t, 1, outputs[1].Index)

	// maxCount 截断。
	outputs, err = parseCreativeOpenAIImageOutputs(body, 1)
	require.NoError(t, err)
	require.Len(t, outputs, 1)

	// 空 data 报 502 可重试上游错误。
	_, err = parseCreativeOpenAIImageOutputs([]byte(`{"data":[]}`), 4)
	require.Error(t, err)
	var upstreamErr *CreativeUpstreamError
	require.True(t, errors.As(err, &upstreamErr))
	require.Equal(t, 502, upstreamErr.StatusCode)
	require.True(t, upstreamErr.Retryable)
}

// TestParseCreativeGeminiImageOutputs 解析 Gemini generateContent 响应的 inlineData。
func TestParseCreativeGeminiImageOutputs(t *testing.T) {
	img := base64.StdEncoding.EncodeToString([]byte("gemini-image-bytes"))
	body := []byte(fmt.Sprintf(`{
		"candidates": [{"content": {"parts": [
			{"text": "说明"},
			{"inlineData": {"mimeType": "image/png", "data": "%s"}},
			{"inlineData": {"mime_type": "image/webp", "data": "%s"}}
		]}}]
	}`, img, base64.StdEncoding.EncodeToString([]byte("webp-bytes"))))

	outputs, err := parseCreativeGeminiImageOutputs(body, 4)
	require.NoError(t, err)
	require.Len(t, outputs, 2)
	require.Equal(t, []byte("gemini-image-bytes"), outputs[0].Bytes)
	require.Equal(t, "image/png", outputs[0].Mime)
	require.Equal(t, "image/webp", outputs[1].Mime)

	// 无候选内容时报可重试错误。
	_, err = parseCreativeGeminiImageOutputs([]byte(`{"candidates":[]}`), 4)
	require.Error(t, err)
	var upstreamErr *CreativeUpstreamError
	require.True(t, errors.As(err, &upstreamErr))
	require.True(t, upstreamErr.Retryable)
}

// TestNormalizeCreativeOutputs 校验大小上限、去重与截断。
func TestNormalizeCreativeOutputs(t *testing.T) {
	// sha256 去重：相同字节只保留一张。
	outputs, err := normalizeCreativeOutputs([]CreativeOutput{
		{Index: 0, Bytes: []byte("same"), Mime: "image/png"},
		{Index: 1, Bytes: []byte("same"), Mime: "image/png"},
		{Index: 2, Bytes: []byte("other"), Mime: "image/png"},
	}, 4)
	require.NoError(t, err)
	require.Len(t, outputs, 2)
	require.Equal(t, []byte("other"), outputs[1].Bytes)

	// requested 截断并重排行号。
	outputs, err = normalizeCreativeOutputs([]CreativeOutput{
		{Index: 0, Bytes: []byte("a"), Mime: "image/png"},
		{Index: 1, Bytes: []byte("b"), Mime: "image/png"},
		{Index: 2, Bytes: []byte("c"), Mime: "image/png"},
	}, 2)
	require.NoError(t, err)
	require.Len(t, outputs, 2)
	require.Equal(t, 0, outputs[0].Index)
	require.Equal(t, 1, outputs[1].Index)

	// 单张超限（>32MiB）视为失败。
	big := make([]byte, creativeMaxOutputBytes+1)
	_, err = normalizeCreativeOutputs([]CreativeOutput{{Index: 0, Bytes: big, Mime: "image/png"}}, 1)
	require.Error(t, err)
	require.False(t, IsRetryableCreativeError(err))

	// 空输出视为失败。
	_, err = normalizeCreativeOutputs(nil, 1)
	require.Error(t, err)
}

// TestCreativeOpenAIImageSize 校验 OpenAI size 映射。
func TestCreativeOpenAIImageSize(t *testing.T) {
	require.Equal(t, "1024x1024", creativeOpenAIImageSize("1K", "1:1"))
	require.Equal(t, "1536x1024", creativeOpenAIImageSize("1K", "16:9"))
	require.Equal(t, "1024x1536", creativeOpenAIImageSize("1K", "9:16"))
	require.Equal(t, "1536x1536", creativeOpenAIImageSize("2K", ""))
	require.Equal(t, "2880x2880", creativeOpenAIImageSize("4K", "1:1"))
	require.Equal(t, "3840x2160", creativeOpenAIImageSize("4K", "16:9"))
	require.Equal(t, "2160x3840", creativeOpenAIImageSize("4K", "9:16"))
	require.Equal(t, "3264x2448", creativeOpenAIImageSize("4K", "4:3"))
}

// TestCreativeGrokOperationMatrix grok 平台支持 generate 与 edit，但不支持 inpaint。
func TestCreativeGrokOperationMatrix(t *testing.T) {
	executor := &CreativeExecutor{}
	for _, operation := range []string{CreativeOperationInpaint} {
		run := CreativeRun{RunID: "crun_x", Operation: operation, RequestedOutputCount: 1}
		payload := CreativeRunPayload{Prompt: "p"}
		_, err := executor.executeGrok(context.Background(), run, payload, &Account{ID: 1}, "grok-imagine")
		require.Error(t, err)
		require.False(t, IsRetryableCreativeError(err), "grok %s 应当不可重试", operation)
	}
}

// TestCreativeErrorRetryableMatrix 校验状态码到可重试性的映射。
func TestCreativeErrorRetryableMatrix(t *testing.T) {
	retryable := []int{0, 429, 500, 502, 503}
	for _, status := range retryable {
		err := creativeHTTPStatusError(status, "boom")
		require.True(t, IsRetryableCreativeError(err), "status %d 应当可重试", status)
	}
	nonRetryable := []int{400, 401, 403, 404, 422}
	for _, status := range nonRetryable {
		err := creativeHTTPStatusError(status, "bad request")
		require.False(t, IsRetryableCreativeError(err), "status %d 应当不可重试", status)
	}
	require.False(t, IsRetryableCreativeError(nil))
	require.True(t, IsRetryableCreativeError(errors.New("network down")))
	require.True(t, IsRetryableCreativeError(creativeNonRetryableError("x")) == false)
}

// TestCreativeOperationsForPlatform 平台能力矩阵。
func TestCreativeOperationsForPlatform(t *testing.T) {
	require.Equal(t, []string{"generate", "edit"}, creativeOperationsForPlatform(PlatformGemini))
	require.Equal(t, []string{"generate", "edit", "inpaint"}, creativeOperationsForPlatform(PlatformOpenAI))
	require.Equal(t, []string{"generate", "edit"}, creativeOperationsForPlatform(PlatformGrok))
	require.Nil(t, creativeOperationsForPlatform(PlatformAnthropic))
}

// TestCreativeGrokDefaultImageCandidates 校验无映射账号包含官方 Grok Imagine 2.0 候选。
func TestCreativeGrokDefaultImageCandidates(t *testing.T) {
	account := &Account{Platform: PlatformGrok, Credentials: map[string]any{}}
	models := creativeExpandAccountModels(account, defaultCreativeGrokModelCandidates(), isGrokImageGenerationModel)
	require.Contains(t, models, "grok-imagine-image-2.0")
}

// TestBuildCreativeGrokRequest 校验 grok 请求体构造。
func TestBuildCreativeGrokRequest(t *testing.T) {
	run := CreativeRun{ImageSize: "2K", AspectRatio: "16:9", RequestedOutputCount: 2}
	payload := CreativeRunPayload{Prompt: "画猫"}
	request := buildCreativeGrokRequest(run, payload, "grok-imagine")
	require.Equal(t, "grok-imagine", request["model"])
	require.Equal(t, "画猫", request["prompt"])
	require.Equal(t, 2, request["n"])
	require.Equal(t, "b64_json", request["response_format"])
	require.Equal(t, "2k", request["resolution"])
	require.Equal(t, "16:9", request["aspect_ratio"])

	// 不支持的 aspect_ratio 不落字段；1K 映射 1k。
	run = CreativeRun{ImageSize: "1K", AspectRatio: "21:99"}
	request = buildCreativeGrokRequest(run, payload, "grok-imagine")
	require.Equal(t, "1k", request["resolution"])
	_, ok := request["aspect_ratio"]
	require.False(t, ok)
}

// TestBuildCreativeGrokEditRequest 校验 Grok 单图与多图编辑 JSON 结构。
func TestBuildCreativeGrokEditRequest(t *testing.T) {
	run := CreativeRun{ImageSize: "2K", AspectRatio: "16:9"}
	payload := CreativeRunPayload{
		Prompt: "edit image",
		Sources: []CreativeInputImage{
			{Bytes: []byte("first"), Mime: "image/png"},
			{Bytes: []byte("second"), Mime: "image/jpeg"},
		},
	}
	request := buildCreativeGrokEditRequest(run, payload, "grok-imagine-image-2.0")
	require.Equal(t, "grok-imagine-image-2.0", request["model"])
	require.Equal(t, "edit image", request["prompt"])
	require.Equal(t, "b64_json", request["response_format"])
	require.Equal(t, "2k", request["resolution"])
	require.Equal(t, "16:9", request["aspect_ratio"])
	require.NotContains(t, request, "image")
	images, ok := request["images"].([]map[string]string)
	require.True(t, ok)
	require.Len(t, images, 2)
	require.Equal(t, "image_url", images[0]["type"])
	require.Equal(t, "data:image/png;base64,Zmlyc3Q=", images[0]["url"])
	require.Equal(t, "data:image/jpeg;base64,c2Vjb25k", images[1]["url"])

	one := buildCreativeGrokEditRequest(run, CreativeRunPayload{
		Prompt:  "edit image",
		Sources: []CreativeInputImage{{Bytes: []byte("one"), Mime: "image/png"}},
	}, "grok-imagine-image-2.0")
	require.Contains(t, one, "image")
	require.NotContains(t, one, "images")
}

// TestExecuteCreativeGrokEditUsesJSONEditEndpoint 校验编辑端点、鉴权请求和 b64 输出解析。
func TestExecuteCreativeGrokEditUsesJSONEditEndpoint(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("edited-image"))
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, encoded)))},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, encoded)))},
	}}
	executor := &CreativeExecutor{gateway: &OpenAIGatewayService{httpUpstream: upstream}}
	account := &Account{
		ID:       41,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "grok-test-key",
			"base_url": "https://xai.test/v1",
		},
	}
	run := CreativeRun{Operation: CreativeOperationEdit, RequestedOutputCount: 1, ImageSize: "2K", AspectRatio: "16:9"}
	payload := CreativeRunPayload{Prompt: "edit this", Sources: []CreativeInputImage{{Bytes: []byte("source"), Mime: "image/png"}}}
	outputs, err := executor.executeGrok(context.Background(), run, payload, account, "grok-imagine-image-2.0")
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	require.Equal(t, []byte("edited-image"), outputs[0].Bytes)
	require.Equal(t, "https://xai.test/v1/images/edits", upstream.lastReq.URL.String())
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
	var editBody map[string]any
	require.NoError(t, json.Unmarshal(upstream.lastBody, &editBody))
	require.Equal(t, "grok-imagine-image-2.0", editBody["model"])
	require.Equal(t, "b64_json", editBody["response_format"])
	require.Equal(t, "2k", editBody["resolution"])
	require.Equal(t, "16:9", editBody["aspect_ratio"])
	require.NotContains(t, editBody, "n")
	image, ok := editBody["image"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "image_url", image["type"])
	require.Equal(t, "data:image/png;base64,c291cmNl", image["url"])

	generateRun := CreativeRun{Operation: CreativeOperationGenerate, RequestedOutputCount: 1, ImageSize: "1K"}
	_, err = executor.executeGrok(context.Background(), generateRun, CreativeRunPayload{Prompt: "generate"}, account, "grok-imagine-image-2.0")
	require.NoError(t, err)
	require.Equal(t, "https://xai.test/v1/images/generations", upstream.lastReq.URL.String())
}

// TestBuildCreativeOpenAIRequestBody 校验 OpenAI JSON/multipart 请求体。
func TestBuildCreativeOpenAIRequestBody(t *testing.T) {
	// generate：JSON。
	run := CreativeRun{Operation: CreativeOperationGenerate, ImageSize: "1K", RequestedOutputCount: 2}
	payload := CreativeRunPayload{Prompt: "hello"}
	body, contentType, err := buildCreativeOpenAIRequestBody(run, payload, "gpt-image-2")
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	require.Contains(t, string(body), `"model":"gpt-image-2"`)
	require.Contains(t, string(body), `"response_format":"b64_json"`)

	// GPT Image 2 的 4K 横向尺寸使用真实的 3840x2160 像素值。
	run = CreativeRun{Operation: CreativeOperationGenerate, ImageSize: "4K", AspectRatio: "16:9", RequestedOutputCount: 1}
	body, contentType, err = buildCreativeOpenAIRequestBody(run, payload, "gpt-image-2")
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	require.Contains(t, string(body), `"size":"3840x2160"`)

	// inpaint：multipart，含 image/mask/model/prompt 字段。
	run = CreativeRun{Operation: CreativeOperationInpaint, ImageSize: "1K"}
	payload = CreativeRunPayload{Prompt: "inpaint me", Sources: []CreativeInputImage{{Bytes: []byte("img"), Mime: "image/png"}}, Mask: &CreativeInputImage{Bytes: []byte("mask"), Mime: "image/png"}}
	body, contentType, err = buildCreativeOpenAIRequestBody(run, payload, "gpt-image-2")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(contentType, "multipart/form-data"))
	require.Contains(t, string(body), `name="image"`)
	require.Contains(t, string(body), `name="mask"`)
	require.Contains(t, string(body), `name="model"`)
	require.Contains(t, string(body), `name="prompt"`)
}

// TestBuildCreativeGeminiRequest 校验 Gemini edit 请求体构造，且不附加独立 mask。
func TestBuildCreativeGeminiRequest(t *testing.T) {
	run := CreativeRun{Operation: CreativeOperationEdit, ImageSize: "2K", AspectRatio: "16:9", ResponseMIMEType: "image/png"}
	payload := CreativeRunPayload{
		Prompt:  "重绘",
		Sources: []CreativeInputImage{{Bytes: []byte("src"), Mime: "image/jpeg"}},
	}
	request := buildCreativeGeminiRequest(run, payload, "gemini-3.1-flash-image")
	require.Len(t, request.Contents, 1)
	parts := request.Contents[0].Parts
	require.Len(t, parts, 2)
	require.Equal(t, "重绘", parts[0].Text)
	require.Equal(t, "image/jpeg", parts[1].InlineData.MimeType)
	require.Equal(t, []string{"TEXT", "IMAGE"}, request.GenerationConfig.ResponseModalities)
	require.NotNil(t, request.GenerationConfig.ImageConfig)
	require.Equal(t, "2K", request.GenerationConfig.ImageConfig.ImageSize)
	require.Equal(t, "16:9", request.GenerationConfig.ImageConfig.AspectRatio)
	require.Equal(t, "image/png", request.GenerationConfig.ResponseMimeType)

	// 必须校验序列化后的层级，避免只检查内存结构而漏掉真实上游请求格式。
	body, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, `{"contents":[{"parts":[{"text":"重绘"},{"inlineData":{"mimeType":"image/jpeg","data":"c3Jj"}}]}],"generationConfig":{"responseModalities":["TEXT","IMAGE"],"imageConfig":{"imageSize":"2K","aspectRatio":"16:9"},"responseMimeType":"image/png"}}`, string(body))
}

// TestCreativeGeminiInpaintIsRejectedBeforeUpstream 校验历史 Gemini inpaint 任务不会触发上游请求。
func TestCreativeGeminiInpaintIsRejectedBeforeUpstream(t *testing.T) {
	upstream := &httpUpstreamRecorder{}
	executor := &CreativeExecutor{gateway: &OpenAIGatewayService{httpUpstream: upstream}}
	_, err := executor.executeGemini(context.Background(), CreativeRun{Operation: CreativeOperationInpaint}, CreativeRunPayload{}, &Account{ID: 1}, "gemini-3.1-flash-image")
	require.Error(t, err)
	require.False(t, IsRetryableCreativeError(err))
	require.Empty(t, upstream.requests)
}

func TestParseCreativeGeminiImageOutputsUsesFinalImagePart(t *testing.T) {
	thought := base64.StdEncoding.EncodeToString([]byte("thought-image"))
	final := base64.StdEncoding.EncodeToString([]byte("final-image"))
	body := fmt.Sprintf(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":%q}},{"inlineData":{"mimeType":"image/png","data":%q}}]}}]}`, thought, final)

	outputs, err := parseCreativeGeminiImageOutputs([]byte(body), 1)
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	require.Equal(t, []byte("final-image"), outputs[0].Bytes)
}
