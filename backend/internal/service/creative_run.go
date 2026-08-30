package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/domain"
	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/errors"
	"github.com/google/uuid"
)

// 创作台（Creative Studio）操作类型。
const (
	CreativeOperationGenerate = "generate"
	CreativeOperationEdit     = "edit"
	CreativeOperationInpaint  = "inpaint"
)

// 创作台任务状态。
const (
	CreativeRunStatusQueued     = "queued"
	CreativeRunStatusRunning    = "running"
	CreativeRunStatusSucceeded  = "succeeded"
	CreativeRunStatusFailed     = "failed"
	CreativeRunStatusCancelled  = "cancelled"
	CreativeRunStatusResultLost = "result_lost"
)

// 创作台输出状态。
const (
	CreativeRunOutputStatusPending   = "pending"
	CreativeRunOutputStatusSucceeded = "succeeded"
	CreativeRunOutputStatusFailed    = "failed"
	CreativeRunOutputStatusLost      = "lost"
	CreativeRunOutputStatusAcked     = "acked"
)

// CreativeManagedBy 是 api_keys.managed_by 的取值，标记创作台隐藏执行 Key。
const CreativeManagedBy = "creative_studio"

var (
	ErrCreativeDisabled          = infraerrors.New(http.StatusNotFound, "CREATIVE_DISABLED", "creative studio is disabled")
	ErrCreativeRunNotFound       = infraerrors.New(http.StatusNotFound, "CREATIVE_RUN_NOT_FOUND", "creative run not found")
	ErrCreativeRunExists         = infraerrors.New(http.StatusConflict, "CREATIVE_RUN_EXISTS", "creative run already exists")
	ErrCreativeOutputExists      = infraerrors.New(http.StatusConflict, "CREATIVE_OUTPUT_EXISTS", "creative run output already exists")
	ErrCreativeWorkspaceRequired = infraerrors.New(http.StatusBadRequest, "CREATIVE_WORKSPACE_REQUIRED", "creative workspace id is required")
	ErrCreativeWorkspaceInvalid  = infraerrors.New(http.StatusBadRequest, "CREATIVE_WORKSPACE_INVALID", "creative workspace id is invalid")

	ErrCreativeInvalidTransition    = infraerrors.New(http.StatusBadRequest, "CREATIVE_INVALID_TRANSITION", "invalid creative run status transition")
	ErrCreativeInvalidParams        = infraerrors.New(http.StatusBadRequest, "CREATIVE_INVALID_PARAMS", "invalid creative run parameters")
	ErrCreativePromptTooLong        = infraerrors.New(http.StatusBadRequest, "CREATIVE_PROMPT_TOO_LONG", "creative prompt is too long")
	ErrCreativeInvalidMime          = infraerrors.New(http.StatusBadRequest, "CREATIVE_INVALID_MIME", "creative source image mime type is invalid")
	ErrCreativeAssetTooLarge        = infraerrors.New(http.StatusBadRequest, "CREATIVE_ASSET_TOO_LARGE", "creative asset is too large")
	ErrCreativeInputTooLarge        = infraerrors.New(http.StatusBadRequest, "CREATIVE_INPUT_TOO_LARGE", "creative total input is too large")
	ErrCreativeMaskRequired         = infraerrors.New(http.StatusBadRequest, "CREATIVE_MASK_REQUIRED", "creative inpaint requires source image and png mask")
	ErrCreativeMaskSizeMismatch     = infraerrors.New(http.StatusBadRequest, "CREATIVE_MASK_SIZE_MISMATCH", "creative mask dimensions must match the first source image")
	ErrCreativeOperationUnsupported = infraerrors.New(http.StatusBadRequest, "CREATIVE_OPERATION_UNSUPPORTED", "creative operation is not supported by this group")
	ErrCreativeInvalidModel         = infraerrors.New(http.StatusBadRequest, "CREATIVE_INVALID_MODEL", "creative model is not available in this group")
	ErrCreativeGroupForbidden       = infraerrors.New(http.StatusForbidden, "CREATIVE_GROUP_FORBIDDEN", "creative studio is not available for this group")
	ErrCreativeGroupImageDisabled   = infraerrors.New(http.StatusForbidden, "CREATIVE_GROUP_IMAGE_DISABLED", "image generation is disabled for this group")

	ErrCreativeRunIdempotencyConflict = infraerrors.New(http.StatusConflict, "CREATIVE_IDEMPOTENCY_CONFLICT", "idempotency key reused with different creative request")
	ErrCreativeOutputNotFound         = infraerrors.New(http.StatusNotFound, "CREATIVE_OUTPUT_NOT_FOUND", "creative run output not found")
	ErrCreativeOutputNotReady         = infraerrors.New(http.StatusConflict, "CREATIVE_OUTPUT_NOT_READY", "creative run output is not ready")
	ErrCreativeOutputExpired          = infraerrors.New(http.StatusGone, "CREATIVE_OUTPUT_EXPIRED", "creative run output has expired")
	ErrCreativeResultLost             = infraerrors.New(http.StatusGone, "CREATIVE_RESULT_LOST", "creative run result has been lost")
	ErrCreativeTransientFailed        = infraerrors.New(http.StatusBadGateway, "CREATIVE_TRANSIENT_FAILED", "creative transient store operation failed")

	ErrCreativeBillingHoldFailed     = infraerrors.New(http.StatusBadGateway, "CREATIVE_BILLING_HOLD_FAILED", "creative balance hold failed")
	ErrCreativeInsufficientBalance   = infraerrors.New(http.StatusPaymentRequired, "CREATIVE_INSUFFICIENT_BALANCE", "insufficient balance for creative run")
	ErrCreativeSettlementBillingFail = infraerrors.New(http.StatusBadGateway, "CREATIVE_SETTLEMENT_BILLING_FAILED", "creative settlement billing failed")
)

// CreativeWorkspaceHeader 是创作台浏览器工作区请求头名称。
const CreativeWorkspaceHeader = "X-Creative-Workspace-ID"

// CreativeRunScope 将用户身份与浏览器工作区绑定，所有用户侧任务访问都必须携带。
type CreativeRunScope struct {
	UserID      int64
	WorkspaceID string
}

// NormalizeCreativeWorkspaceID 校验并规范化创作台工作区 UUID。
func NormalizeCreativeWorkspaceID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrCreativeWorkspaceRequired
	}
	parsed, err := uuid.Parse(raw)
	if err != nil || parsed == uuid.Nil {
		return "", ErrCreativeWorkspaceInvalid
	}
	return strings.ToLower(parsed.String()), nil
}

// ValidateCreativeRunScope 防止内部调用绕过工作区校验。
func ValidateCreativeRunScope(scope CreativeRunScope) error {
	_, err := NormalizeCreativeRunScope(scope)
	return err
}

// NormalizeCreativeRunScope 校验用户身份并将工作区 UUID 规范化为小写。
func NormalizeCreativeRunScope(scope CreativeRunScope) (CreativeRunScope, error) {
	if scope.UserID <= 0 {
		return CreativeRunScope{}, ErrCreativeRunNotFound
	}
	normalizedWorkspaceID, err := NormalizeCreativeWorkspaceID(scope.WorkspaceID)
	if err != nil {
		return CreativeRunScope{}, err
	}
	scope.WorkspaceID = normalizedWorkspaceID
	return scope, nil
}

// CreativeRun 是创作台异步任务的元数据。
// 隐私红线：结构体中禁止出现 prompt 明文、图片字节、mask 或 provider 响应本体。
type CreativeRun struct {
	ID     int64
	RunID  string
	UserID int64
	// WorkspaceID 为空表示迁移前旧任务，用户侧工作区查询会将其隐藏。
	WorkspaceID          *string
	GroupID              int64
	APIKeyID             int64
	AccountID            *int64
	Model                string
	RequestedModel       string
	Operation            string
	RequestedOutputCount int
	ImageSize            string
	AspectRatio          string
	ResponseMIMEType     string

	PromptHash         string
	RequestFingerprint string
	IdempotencyKey     *string

	Status string

	EstimatedCost               float64
	HoldAmount                  *float64
	ActualCost                  *float64
	BalanceHoldAmount           float64
	SubscriptionHoldAllocations []domain.BillingAllocation
	// AllowanceReserved 与 batch_image_jobs.allowance_reserved 同语义：额度预记标记。
	AllowanceReserved          bool
	BaseUnitPrice              float64
	SubscriptionRateMultiplier float64
	BalanceRateMultiplier      float64
	PlanGroupRateEnabled       bool

	ErrorCode    *string
	ErrorMessage *string
	AttemptCount int

	Version int64

	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	CancelledAt *time.Time
}

// CreativeRunOutput 是创作台单个输出的元数据；图片本体只存在临时存储中。
type CreativeRunOutput struct {
	ID                 int64
	RunID              string
	OutputIndex        int
	Status             string
	MimeType           *string
	ByteSize           *int64
	TransientExpiresAt *time.Time
	AckedAt            *time.Time
	ErrorCode          *string
	ErrorMessage       *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// CreateCreativeRunParams 创建创作台任务及其输出行。
type CreateCreativeRunParams struct {
	RunID                      string
	UserID                     int64
	WorkspaceID                string
	GroupID                    int64
	APIKeyID                   int64
	Model                      string
	RequestedModel             string
	Operation                  string
	RequestedOutputCount       int
	ImageSize                  string
	AspectRatio                string
	ResponseMIMEType           string
	PromptHash                 string
	RequestFingerprint         string
	IdempotencyKey             *string
	EstimatedCost              float64
	HoldAmount                 float64
	BaseUnitPrice              float64
	SubscriptionRateMultiplier float64
	BalanceRateMultiplier      float64
	PlanGroupRateEnabled       bool
}

// CreativeRunTransitionOptions 携带状态转换的可选上下文。
type CreativeRunTransitionOptions struct {
	ErrorCode    *string
	ErrorMessage *string
	Now          *time.Time
}

// CreativeRunFilter 是用户侧任务列表过滤条件。
type CreativeRunFilter struct {
	Status string
	Limit  int
	Offset int
}

// CreativeRunRepository 持久化创作台任务元数据。
type CreativeRunRepository interface {
	CreateCreativeRun(ctx context.Context, params CreateCreativeRunParams) (*CreativeRun, error)
	GetCreativeRunByRunID(ctx context.Context, runID string) (*CreativeRun, error)
	GetCreativeRunByRunIDForOwner(ctx context.Context, scope CreativeRunScope, runID string) (*CreativeRun, error)
	GetCreativeRunByIdempotencyKey(ctx context.Context, scope CreativeRunScope, key string) (*CreativeRun, error)
	ListCreativeRunsForOwner(ctx context.Context, scope CreativeRunScope, filter CreativeRunFilter) ([]*CreativeRun, error)
	// TransitionCreativeRunStatus 带 version 乐观锁与 CanTransitionCreativeRun 校验。
	TransitionCreativeRunStatus(ctx context.Context, runID, toStatus string, opts CreativeRunTransitionOptions) error
	// MarkCreativeRunRunning 幂等地把任务标记为执行中并回填账号，重复调用不产生副作用。
	MarkCreativeRunRunning(ctx context.Context, runID string, accountID int64, now time.Time) error
	// SetCreativeRunAccountID 在执行结果确定后补写真实上游账号，避免 worker 先推进状态时丢失账号信息。
	SetCreativeRunAccountID(ctx context.Context, runID string, accountID int64, now time.Time) error
	// MarkCreativeRunSucceeded 记录实际成本并进入终态，仅在 running 时生效。
	MarkCreativeRunSucceeded(ctx context.Context, runID string, actualCost float64, now time.Time) error
	// UpdateCreativeRunOutput 幂等更新输出行；已 acked 的行不允许被覆盖。
	UpdateCreativeRunOutput(ctx context.Context, runID string, outputIndex int, status, mimeType string, byteSize int64, transientExpiresAt *time.Time, errorCode, errorMessage string) error
	GetCreativeRunOutput(ctx context.Context, runID string, outputIndex int) (*CreativeRunOutput, error)
	ListCreativeRunOutputs(ctx context.Context, runID string) ([]*CreativeRunOutput, error)
	// MarkCreativeRunOutputAcked 幂等标记客户端已确认接收。
	MarkCreativeRunOutputAcked(ctx context.Context, runID string, outputIndex int, now time.Time) error
	// ListCreativeRunsDueForTransientCleanup 返回已进入终态且超过清理时限的任务。
	ListCreativeRunsDueForTransientCleanup(ctx context.Context, cutoff time.Time, limit int) ([]*CreativeRun, error)
	// IncrementCreativeRunAttempt 原子递增执行尝试次数并返回最新值。
	IncrementCreativeRunAttempt(ctx context.Context, runID string) (int, error)
}

// CreativeRunPayload 是保存在临时 Redis 存储中的任务载荷。
// 只有这里和 worker 执行期间允许出现 prompt 明文与图片字节。
// Sources/Mask 仅存在于 worker 内存态（json:"-"），不落 Redis：字节已由 input/mask 键单独保存。
type CreativeRunPayload struct {
	RunID              string `json:"run_id"`
	UserID             int64  `json:"user_id"`
	GroupID            int64  `json:"group_id"`
	APIKeyID           int64  `json:"api_key_id"`
	Model              string `json:"model"`
	Operation          string `json:"operation"`
	Prompt             string `json:"prompt"`
	ImageSize          string `json:"image_size"`
	AspectRatio        string `json:"aspect_ratio"`
	ResponseMIMEType   string `json:"response_mime_type"`
	Quality            string `json:"quality,omitempty"`
	SourceCount        int    `json:"source_count"`
	HasMask            bool   `json:"has_mask"`
	RequestFingerprint string `json:"request_fingerprint"`

	// Sources/Mask 由 worker 从临时存储加载后填充，仅存在于进程内。
	Sources []CreativeInputImage `json:"-"`
	Mask    *CreativeInputImage  `json:"-"`
}

// CreativeInputImage 是客户端上传的源图/mask 输入。
type CreativeInputImage struct {
	Bytes []byte
	Mime  string
}

// CreateCreativeRunParamsPublic 是 CreateRun 的入参（由 handler 解析 multipart 后组装）。
type CreateCreativeRunParamsPublic struct {
	GroupID      int64
	Model        string
	Operation    string
	Prompt       string
	SourceImages []CreativeInputImage
	Mask         *CreativeInputImage
	ImageSize    string
	AspectRatio  string
	ResponseMIME string
	// Quality 为 OpenAI 平台可选画质（low/medium/high/auto），其余平台忽略。
	Quality string
}

// CreativeRunPublic 是任务对客户端的展示结构。
type CreativeRunPublic struct {
	ID                   string                    `json:"id"`
	Status               string                    `json:"status"`
	Model                string                    `json:"model"`
	RequestedModel       string                    `json:"requested_model"`
	Operation            string                    `json:"operation"`
	RequestedOutputCount int                       `json:"requested_output_count"`
	ImageSize            string                    `json:"image_size"`
	AspectRatio          string                    `json:"aspect_ratio"`
	ResponseMIMEType     string                    `json:"response_mime_type"`
	GroupID              int64                     `json:"group_id"`
	EstimatedCost        float64                   `json:"estimated_cost"`
	HoldAmount           float64                   `json:"hold_amount"`
	ActualCost           *float64                  `json:"actual_cost,omitempty"`
	ErrorCode            string                    `json:"error_code,omitempty"`
	ErrorMessage         string                    `json:"error_message,omitempty"`
	Outputs              []CreativeRunOutputPublic `json:"outputs,omitempty"`
	CreatedAt            int64                     `json:"created_at"`
	StartedAt            *int64                    `json:"started_at,omitempty"`
	CompletedAt          *int64                    `json:"completed_at,omitempty"`
	CancelledAt          *int64                    `json:"cancelled_at,omitempty"`
	// IdempotentReplay 为 true 时表示该响应来自幂等键重放。
	IdempotentReplay bool `json:"idempotent_replay,omitempty"`
}

// CreativeRunOutputPublic 是输出元数据对客户端的展示结构。
type CreativeRunOutputPublic struct {
	Index              int    `json:"output_index"`
	Status             string `json:"status"`
	MimeType           string `json:"mime_type,omitempty"`
	ByteSize           int64  `json:"byte_size,omitempty"`
	TransientExpiresAt *int64 `json:"transient_expires_at,omitempty"`
	AckedAt            *int64 `json:"acked_at,omitempty"`
	ErrorCode          string `json:"error_code,omitempty"`
	ErrorMessage       string `json:"error_message,omitempty"`
}

// CreativeModelPublic 是 ListModels 返回的可用模型条目。
type CreativeModelPublic struct {
	GroupID    int64    `json:"group_id"`
	GroupName  string   `json:"group_name"`
	Model      string   `json:"model"`
	Operations []string `json:"operations"`
	ImageSizes []string `json:"image_sizes"`
	Qualities  []string `json:"qualities,omitempty"`
	Price1K    float64  `json:"price_1k"`
	Price2K    float64  `json:"price_2k"`
	Price4K    float64  `json:"price_4k"`
}

// CreativeModelsResponse 是 ListModels 的响应体。
type CreativeModelsResponse struct {
	Data []CreativeModelPublic `json:"data"`
}

// CreativeListRunsResponse 是任务列表响应体。
type CreativeListRunsResponse struct {
	Data    []*CreativeRunPublic `json:"data"`
	HasMore bool                 `json:"has_more"`
}

// NewCreativeRunID 生成 'crun_' + 16 字节 hex 的任务 ID。
func NewCreativeRunID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "crun_" + hex.EncodeToString(b[:]), nil
}

// IsValidCreativeRunID 校验任务 ID 格式，防止队列被注入非法 payload。
func IsValidCreativeRunID(runID string) bool {
	return len(runID) > len("crun_") && runID[:len("crun_")] == "crun_"
}

// IsTerminalCreativeRunStatus 判断任务是否已进入终态。
func IsTerminalCreativeRunStatus(status string) bool {
	switch status {
	case CreativeRunStatusSucceeded, CreativeRunStatusFailed, CreativeRunStatusCancelled, CreativeRunStatusResultLost:
		return true
	default:
		return false
	}
}

// CanTransitionCreativeRun 校验创作台任务状态机：
// queued -> running | cancelled；running -> succeeded | failed | cancelled | result_lost；
// succeeded 任务在临时输出过期时可降级为 result_lost；其余终态不可逆。
func CanTransitionCreativeRun(from, to string) bool {
	if from == "" || to == "" {
		return false
	}
	if IsTerminalCreativeRunStatus(from) {
		// 唯一允许的终态转换：成功任务的临时结果过期，降级为 result_lost。
		return from == CreativeRunStatusSucceeded && to == CreativeRunStatusResultLost
	}
	if to == CreativeRunStatusFailed {
		return true
	}
	switch from {
	case CreativeRunStatusQueued:
		// result_lost 允许从 queued 直接进入：worker 恢复发现载荷已过期时无需先推进 running。
		return to == CreativeRunStatusRunning || to == CreativeRunStatusCancelled || to == CreativeRunStatusResultLost
	case CreativeRunStatusRunning:
		switch to {
		case CreativeRunStatusSucceeded, CreativeRunStatusCancelled, CreativeRunStatusResultLost:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// CreativeRunToPublic 把领域对象转换为客户端展示结构。
func CreativeRunToPublic(run *CreativeRun, outputs []*CreativeRunOutput) *CreativeRunPublic {
	if run == nil {
		return nil
	}
	holdAmount := run.EstimatedCost
	if run.HoldAmount != nil {
		holdAmount = *run.HoldAmount
	}
	out := &CreativeRunPublic{
		ID:                   run.RunID,
		Status:               run.Status,
		Model:                run.Model,
		RequestedModel:       run.RequestedModel,
		Operation:            run.Operation,
		RequestedOutputCount: run.RequestedOutputCount,
		ImageSize:            run.ImageSize,
		AspectRatio:          run.AspectRatio,
		ResponseMIMEType:     run.ResponseMIMEType,
		GroupID:              run.GroupID,
		EstimatedCost:        run.EstimatedCost,
		HoldAmount:           holdAmount,
		ActualCost:           run.ActualCost,
		ErrorCode:            creativeDerefString(run.ErrorCode),
		ErrorMessage:         creativeDerefString(run.ErrorMessage),
		CreatedAt:            run.CreatedAt.Unix(),
		StartedAt:            creativeUnixPtr(run.StartedAt),
		CompletedAt:          creativeUnixPtr(run.CompletedAt),
		CancelledAt:          creativeUnixPtr(run.CancelledAt),
	}
	if len(outputs) > 0 {
		out.Outputs = make([]CreativeRunOutputPublic, 0, len(outputs))
		for _, output := range outputs {
			out.Outputs = append(out.Outputs, CreativeRunOutputToPublic(output))
		}
	}
	return out
}

// CreativeRunOutputToPublic 把输出行转换为客户端展示结构。
func CreativeRunOutputToPublic(output *CreativeRunOutput) CreativeRunOutputPublic {
	if output == nil {
		return CreativeRunOutputPublic{}
	}
	out := CreativeRunOutputPublic{
		Index:              output.OutputIndex,
		Status:             output.Status,
		MimeType:           creativeDerefString(output.MimeType),
		ByteSize:           creativeDerefInt64(output.ByteSize),
		TransientExpiresAt: creativeUnixPtr(output.TransientExpiresAt),
		AckedAt:            creativeUnixPtr(output.AckedAt),
		ErrorCode:          creativeDerefString(output.ErrorCode),
		ErrorMessage:       creativeDerefString(output.ErrorMessage),
	}
	return out
}

func creativeDerefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func creativeDerefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func creativeUnixPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := t.Unix()
	return &v
}

func creativeStringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
