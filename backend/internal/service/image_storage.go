package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/httpclient"
	"github.com/TokenFlux/TokenRouter/internal/util/urlvalidator"
)

const defaultImageMaxDownloadBytes int64 = 32 << 20 // 单张图片默认最多 32 MiB

// ImageStorage 把图片字节写入对象存储并返回可访问 URL。
//
// 这是对象存储的可插拔抽象：适配一个新的对象存储厂商，只需实现本接口
// （例如包一个厂商 SDK），无需改动任务/网关逻辑。仓库内自带一个 S3 兼容实现
// （repository.S3ImageStorage），适用于 AWS S3 / Cloudflare R2 / 阿里云 OSS / MinIO 等。
type ImageStorage interface {
	// Save 把 data 以 key 存入对象存储，返回可下载的 URL（公开直链或 presigned 临时链接）。
	// contentType 为图片 MIME 类型，如 "image/png"。
	Save(ctx context.Context, key, contentType string, data []byte) (url string, err error)
}

// ImageResultUploader 是 ImageStorage 的上层编排器（与具体厂商无关）：
// 把上游生图响应里的每张图片（b64_json 解码 / url 下载）转存到对象存储，
// 并把响应结果改写为只含短链接的紧凑 JSON，从而避免大 base64 落 Redis。
type ImageResultUploader struct {
	storage          ImageStorage
	httpClient       *http.Client
	prefix           string
	maxDownloadBytes int64
}

// NewImageResultUploader 构造一个 uploader；storage 为 nil 时 Rewrite 直接透传。
func NewImageResultUploader(storage ImageStorage, prefix string, maxDownloadBytes int64, httpClient *http.Client) *ImageResultUploader {
	if httpClient == nil {
		httpClient = defaultImageDownloadHTTPClient()
	}
	if maxDownloadBytes <= 0 {
		maxDownloadBytes = defaultImageMaxDownloadBytes
	}
	prefix = strings.TrimSpace(prefix)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &ImageResultUploader{
		storage:          storage,
		httpClient:       httpClient,
		prefix:           prefix,
		maxDownloadBytes: maxDownloadBytes,
	}
}

func defaultImageDownloadHTTPClient() *http.Client {
	client, err := httpclient.GetClient(httpclient.Options{
		Timeout:               60 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ValidateResolvedIP:    true,
		AllowPrivateHosts:     false,
	})
	if err == nil {
		return client
	}

	// 安全客户端构造失败时拒绝所有下载，不能退回到未校验内网地址的默认客户端。
	return &http.Client{Transport: imageDownloadRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("create secure image download client: %w", err)
	})}
}

type imageDownloadRoundTripper func(*http.Request) (*http.Response, error)

func (f imageDownloadRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Rewrite 将 result（上游生图响应 JSON）里的每张图片转存到对象存储，
// 返回改写后的紧凑结果（data[i].url 指向对象存储，b64_json 被移除）。
// 任一图片转存失败即返回 error（调用方据此将任务标记为失败，绝不把大 blob 落 Redis）。
func (u *ImageResultUploader) Rewrite(ctx context.Context, taskID string, result json.RawMessage) (json.RawMessage, error) {
	if u == nil || u.storage == nil {
		return result, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(result, &top); err != nil {
		return nil, fmt.Errorf("parse image response: %w", err)
	}
	rawData, ok := top["data"]
	if !ok {
		return nil, errors.New("image response does not contain data")
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawData, &items); err != nil {
		return nil, fmt.Errorf("parse image response data: %w", err)
	}
	if len(items) == 0 {
		return nil, errors.New("image response data is empty")
	}
	for i, item := range items {
		data, contentType, err := u.fetchImageBytes(ctx, item)
		if err != nil {
			return nil, fmt.Errorf("image %d: %w", i, err)
		}
		key := u.buildKey(taskID, i, contentType)
		url, err := u.storage.Save(ctx, key, contentType, data)
		if err != nil {
			return nil, fmt.Errorf("image %d: upload to object storage: %w", i, err)
		}
		urlRaw, err := json.Marshal(url)
		if err != nil {
			return nil, fmt.Errorf("image %d: encode url: %w", i, err)
		}
		item["url"] = urlRaw
		delete(item, "b64_json")
		items[i] = item
	}
	newData, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("encode image response data: %w", err)
	}
	top["data"] = newData
	out, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("encode image response: %w", err)
	}
	return out, nil
}

func (u *ImageResultUploader) fetchImageBytes(ctx context.Context, item map[string]json.RawMessage) ([]byte, string, error) {
	if raw, ok := item["b64_json"]; ok {
		var b64 string
		if err := json.Unmarshal(raw, &b64); err == nil {
			if b64 = strings.TrimSpace(b64); b64 != "" {
				limit := u.imageSizeLimit()
				if int64(base64.StdEncoding.DecodedLen(len(b64))) > limit {
					return nil, "", fmt.Errorf("decoded b64_json exceeds %d bytes", limit)
				}
				data, err := base64.StdEncoding.DecodeString(b64)
				if err != nil {
					return nil, "", fmt.Errorf("decode b64_json: %w", err)
				}
				if int64(len(data)) > limit {
					return nil, "", fmt.Errorf("decoded b64_json exceeds %d bytes", limit)
				}
				contentType, err := validateImageContentType(data, "")
				if err != nil {
					return nil, "", err
				}
				return data, contentType, nil
			}
		}
	}
	if raw, ok := item["url"]; ok {
		var rawURL string
		if err := json.Unmarshal(raw, &rawURL); err == nil {
			if rawURL = strings.TrimSpace(rawURL); rawURL != "" {
				return u.download(ctx, rawURL)
			}
		}
	}
	return nil, "", errors.New("image item has neither b64_json nor url")
}

func (u *ImageResultUploader) download(ctx context.Context, rawURL string) ([]byte, string, error) {
	if _, err := urlvalidator.ValidateHTTPURL(rawURL, true, urlvalidator.ValidationOptions{AllowPrivate: false}); err != nil {
		return nil, "", fmt.Errorf("validate image url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build download request: %w", err)
	}
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("download image: unexpected status %d", resp.StatusCode)
	}
	limit := u.imageSizeLimit()
	if resp.ContentLength > limit {
		return nil, "", fmt.Errorf("downloaded image exceeds %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("read image body: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("downloaded image exceeds %d bytes", limit)
	}
	contentType, err := validateImageContentType(data, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", err
	}
	return data, contentType, nil
}

func (u *ImageResultUploader) imageSizeLimit() int64 {
	if u == nil || u.maxDownloadBytes <= 0 {
		return defaultImageMaxDownloadBytes
	}
	return u.maxDownloadBytes
}

func (u *ImageResultUploader) buildKey(taskID string, index int, contentType string) string {
	return u.prefix + taskID + "-" + strconv.Itoa(index) + extensionForContentType(contentType)
}

func validateImageContentType(data []byte, declared string) (string, error) {
	detected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
	if contentType, ok := normalizeImageContentType(detected); ok {
		return contentType, nil
	}

	// Go 无法识别 AVIF 等部分现代格式；仅在内容未被识别且声明类型受支持时采用响应头。
	if detected == "application/octet-stream" {
		if contentType, ok := normalizeImageContentType(declared); ok {
			return contentType, nil
		}
	}
	return "", fmt.Errorf("unsupported image content type %q", detected)
}

func normalizeImageContentType(contentType string) (string, bool) {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "image/png", "image/x-png":
		return "image/png", true
	case "image/jpeg", "image/jpg", "image/pjpeg":
		return "image/jpeg", true
	case "image/webp", "image/gif", "image/avif":
		return contentType, true
	default:
		return "", false
	}
}

func extensionForContentType(ct string) string {
	switch ct {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/avif":
		return ".avif"
	default:
		return ".img"
	}
}
