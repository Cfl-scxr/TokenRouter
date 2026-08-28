package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
)

// 创作台执行器常量。
const (
	defaultCreativeExecuteTimeout = 5 * time.Minute
	defaultCreativeMaxAttempts    = 3
	// creativeMaxOutputBytes 限制单张输出图片大小（32 MiB）。
	creativeMaxOutputBytes = 32 << 20
)

// CreativeUpstreamError 是执行器向上抛出的上游调用错误，携带可重试判定。
type CreativeUpstreamError struct {
	StatusCode int
	Message    string
	Retryable  bool
}

func (e *CreativeUpstreamError) Error() string {
	if e == nil {
		return "creative upstream error"
	}
	return fmt.Sprintf("creative upstream error status=%d: %s", e.StatusCode, e.Message)
}

// creativeNonRetryableError 生成不可重试的执行错误（如平台不支持该操作）。
func creativeNonRetryableError(format string, args ...any) *CreativeUpstreamError {
	return &CreativeUpstreamError{StatusCode: 0, Message: fmt.Sprintf(format, args...), Retryable: false}
}

// creativeHTTPStatusError 把上游 HTTP 状态码映射为可重试性：网络层错误、429 与 5xx 可重试，其余 4xx 不可重试。
func creativeHTTPStatusError(statusCode int, message string) *CreativeUpstreamError {
	retryable := statusCode == 0 || statusCode == http.StatusTooManyRequests || statusCode >= 500
	return &CreativeUpstreamError{StatusCode: statusCode, Message: sanitizeCreativeMessage(message), Retryable: retryable}
}

// IsRetryableCreativeError 判断错误是否值得 worker 有限重试。
func IsRetryableCreativeError(err error) bool {
	var upstreamErr *CreativeUpstreamError
	if errors.As(err, &upstreamErr) {
		return upstreamErr.Retryable
	}
	// 未知错误（网络层、序列化等）保守视为可重试，由最大次数兜底。
	return err != nil
}

// CreativeExecutor 是创作台的 provider 执行器：按分组平台直接构造上游 HTTP 请求，
// 绝不通过本地 HTTP 回环调用网关。
type CreativeExecutor struct {
	cfg            *config.Config
	accountRepo    CreativeAccountRepository
	groupRepo      CreativeGroupRepository
	gateway        *OpenAIGatewayService
	geminiTokens   *GeminiTokenProvider
	settingService *SettingService
}

// NewCreativeExecutor 创建创作台执行器。
func NewCreativeExecutor(
	cfg *config.Config,
	accountRepo CreativeAccountRepository,
	groupRepo CreativeGroupRepository,
	gateway *OpenAIGatewayService,
	geminiTokens *GeminiTokenProvider,
	settingService *SettingService,
) *CreativeExecutor {
	return &CreativeExecutor{
		cfg:            cfg,
		accountRepo:    accountRepo,
		groupRepo:      groupRepo,
		gateway:        gateway,
		geminiTokens:   geminiTokens,
		settingService: settingService,
	}
}

// Execute 按账号平台分派执行创作台任务。
func (e *CreativeExecutor) Execute(ctx context.Context, run CreativeRun, payload CreativeRunPayload) (*CreativeExecuteResult, error) {
	if e == nil {
		return nil, errors.New("creative executor is not configured")
	}
	account, upstreamModel, err := e.selectAccount(ctx, run)
	if err != nil {
		return nil, err
	}
	execCtx, cancel := context.WithTimeout(ctx, e.executeTimeout())
	defer cancel()

	var outputs []CreativeOutput
	switch account.Platform {
	case PlatformOpenAI:
		outputs, err = e.executeOpenAI(execCtx, run, payload, account, upstreamModel)
	case PlatformGrok:
		outputs, err = e.executeGrok(execCtx, run, payload, account, upstreamModel)
	case PlatformGemini:
		outputs, err = e.executeGemini(execCtx, run, payload, account, upstreamModel)
	default:
		return nil, creativeNonRetryableError("creative executor unsupported account platform %s", account.Platform)
	}
	if err != nil {
		return nil, err
	}
	outputs, err = normalizeCreativeOutputs(outputs, run.RequestedOutputCount)
	if err != nil {
		return nil, err
	}
	return &CreativeExecuteResult{
		Outputs:   outputs,
		AccountID: account.ID,
	}, nil
}

// IsRetryable 实现 CreativeRunExecutor 接口。
func (e *CreativeExecutor) IsRetryable(err error) bool {
	return IsRetryableCreativeError(err)
}

// selectAccount 从分组的可调度账号中选出执行账号，返回账号与账号映射后的上游模型。
func (e *CreativeExecutor) selectAccount(ctx context.Context, run CreativeRun) (*Account, string, error) {
	if e.accountRepo == nil {
		return nil, "", errors.New("creative account repository is not configured")
	}
	group, err := e.resolveGroupPlatform(ctx, run.GroupID)
	if err != nil {
		return nil, "", err
	}
	accounts, err := e.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, run.GroupID, group)
	if err != nil {
		return nil, "", err
	}
	sort.SliceStable(accounts, func(i, j int) bool {
		if accounts[i].Priority != accounts[j].Priority {
			return accounts[i].Priority > accounts[j].Priority
		}
		return accounts[i].ID < accounts[j].ID
	})
	for i := range accounts {
		account := &accounts[i]
		if !account.IsSchedulable() || !account.IsModelSupported(run.Model) {
			continue
		}
		upstreamModel := strings.TrimSpace(resolveAccountUpstreamModel(ctx, account, run.Model))
		if upstreamModel == "" {
			continue
		}
		return account, upstreamModel, nil
	}
	return nil, "", creativeNonRetryableError("no compatible creative account available for group %d model %s", run.GroupID, run.Model)
}

// resolveGroupPlatform 读取分组平台；平台决定执行协议分派。
func (e *CreativeExecutor) resolveGroupPlatform(ctx context.Context, groupID int64) (string, error) {
	if e.groupRepo == nil {
		return "", errors.New("creative group repository is not configured")
	}
	group, err := e.groupRepo.GetByIDLite(ctx, groupID)
	if err != nil || group == nil {
		return "", creativeNonRetryableError("creative group %d is unavailable", groupID)
	}
	switch group.Platform {
	case PlatformOpenAI, PlatformGrok, PlatformGemini:
		return group.Platform, nil
	default:
		return "", creativeNonRetryableError("creative group platform %s is not supported", group.Platform)
	}
}

func (e *CreativeExecutor) executeTimeout() time.Duration {
	if e != nil && e.cfg != nil && e.cfg.Creative.ExecuteTimeoutSeconds > 0 {
		return time.Duration(e.cfg.Creative.ExecuteTimeoutSeconds) * time.Second
	}
	return defaultCreativeExecuteTimeout
}

// maxExecuteAttempts 返回最大执行次数（含首次）。
func (e *CreativeExecutor) maxExecuteAttempts() int {
	if e != nil && e.cfg != nil && e.cfg.Creative.MaxExecuteAttempts > 0 {
		return e.cfg.Creative.MaxExecuteAttempts
	}
	return defaultCreativeMaxAttempts
}

// accountProxyURL 返回账号绑定的代理地址（无代理时为空串）。
func accountProxyURL(account *Account) string {
	if account == nil || account.ProxyID == nil || account.Proxy == nil {
		return ""
	}
	return account.Proxy.URL()
}

// readCreativeUpstreamBody 读取上游响应体，限制最大读取量，避免异常响应撑爆内存。
func readCreativeUpstreamBody(body io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = 64 << 20
	}
	return io.ReadAll(io.LimitReader(body, limit))
}

// normalizeCreativeOutputs 对执行器输出做后处理：
// 单张大小上限、按请求数量截断、sha256 去重（同一任务内重复输出只保留一张）。
func normalizeCreativeOutputs(outputs []CreativeOutput, requested int) ([]CreativeOutput, error) {
	if len(outputs) == 0 {
		return nil, creativeNonRetryableError("provider returned no image output")
	}
	seen := make(map[string]struct{}, len(outputs))
	out := make([]CreativeOutput, 0, len(outputs))
	for _, output := range outputs {
		if len(output.Bytes) == 0 {
			continue
		}
		if len(output.Bytes) > creativeMaxOutputBytes {
			return nil, creativeNonRetryableError("creative output %d exceeds size limit %d bytes", output.Index, creativeMaxOutputBytes)
		}
		sum := sha256Hex(output.Bytes)
		if _, ok := seen[sum]; ok {
			continue
		}
		seen[sum] = struct{}{}
		out = append(out, output)
	}
	if len(out) == 0 {
		return nil, creativeNonRetryableError("provider returned no usable image output")
	}
	if requested > 0 && len(out) > requested {
		out = out[:requested]
	}
	for index := range out {
		out[index].Index = index
	}
	return out, nil
}

// creativeOpenAIImageSize 把创作台的尺寸档位映射为 OpenAI images 协议支持的 size 值。
// gpt-image 系列仅支持 1024x1024 / 1536x1024 / 1024x1536 / auto。
func creativeOpenAIImageSize(imageSize, aspectRatio string) string {
	tier := NormalizeImageBillingTierOrDefault(imageSize)
	base := 1024
	if tier != ImageBillingSize1K {
		base = 1536
	}
	switch strings.TrimSpace(aspectRatio) {
	case "16:9", "3:2", "4:3", "2:1", "19.5:9", "20:9":
		return fmt.Sprintf("%dx%d", base+base/2, base)
	case "9:16", "3:4", "2:3", "1:2", "9:19.5", "9:20":
		return fmt.Sprintf("%dx%d", base, base+base/2)
	default:
		return fmt.Sprintf("%dx%d", base, base)
	}
}

// creativeGrokImageResolution 把尺寸档位映射为 grok imagine 的 resolution（1k/2k）。
func creativeGrokImageResolution(imageSize string) string {
	if NormalizeImageBillingTierOrDefault(imageSize) == ImageBillingSize1K {
		return "1k"
	}
	return "2k"
}

// creativeGrokAspectRatio 校验并返回 grok imagine 支持的 aspect_ratio，不支持时返回空串（上游取默认）。
func creativeGrokAspectRatio(aspectRatio string) string {
	aspectRatio = strings.TrimSpace(aspectRatio)
	if aspectRatio == "" {
		return ""
	}
	for _, candidate := range grokImagineAspectRatioValues {
		if candidate.label == aspectRatio {
			return aspectRatio
		}
	}
	return ""
}

// decodedCreativeImage 是 base64 解码后的图片字节与嗅探出的 MIME。
type decodedCreativeImage struct {
	Bytes []byte
	Mime  string
}

// decodeBase64Image 解码上游返回的 base64 图片并按魔数嗅探 MIME。
func decodeBase64Image(raw string) (decodedCreativeImage, error) {
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		// 部分上游会省略 padding，按 RawStdEncoding 再试一次。
		data, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return decodedCreativeImage{}, err
		}
	}
	mime := sniffCreativeImageMime(data)
	if mime == "" {
		mime = "image/png"
	}
	return decodedCreativeImage{Bytes: data, Mime: mime}, nil
}

// creativeFileExtension 返回 MIME 对应的文件扩展名（用于 multipart 文件名）。
func creativeFileExtension(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/jpeg":
		return "jpg"
	case "image/webp":
		return "webp"
	default:
		return "png"
	}
}
