//go:build unit

package service

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 测试替身
// ---------------------------------------------------------------------------

type creativeFakeRunRepo struct {
	runs         map[string]*CreativeRun
	byIdem       map[string]*CreativeRun
	outputs      map[string][]*CreativeRunOutput
	createErr    error
	createParams []CreateCreativeRunParams
	transition   []string
}

func newCreativeFakeRunRepo() *creativeFakeRunRepo {
	return &creativeFakeRunRepo{
		runs:    make(map[string]*CreativeRun),
		byIdem:  make(map[string]*CreativeRun),
		outputs: make(map[string][]*CreativeRunOutput),
	}
}

func (r *creativeFakeRunRepo) CreateCreativeRun(ctx context.Context, params CreateCreativeRunParams) (*CreativeRun, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	r.createParams = append(r.createParams, params)
	run := &CreativeRun{
		RunID:                      params.RunID,
		UserID:                     params.UserID,
		GroupID:                    params.GroupID,
		APIKeyID:                   params.APIKeyID,
		Model:                      params.Model,
		RequestedModel:             params.RequestedModel,
		Operation:                  params.Operation,
		RequestedOutputCount:       params.RequestedOutputCount,
		ImageSize:                  params.ImageSize,
		AspectRatio:                params.AspectRatio,
		ResponseMIMEType:           params.ResponseMIMEType,
		PromptHash:                 params.PromptHash,
		RequestFingerprint:         params.RequestFingerprint,
		IdempotencyKey:             params.IdempotencyKey,
		Status:                     CreativeRunStatusQueued,
		EstimatedCost:              params.EstimatedCost,
		HoldAmount:                 &params.HoldAmount,
		BaseUnitPrice:              params.BaseUnitPrice,
		SubscriptionRateMultiplier: params.SubscriptionRateMultiplier,
		BalanceRateMultiplier:      params.BalanceRateMultiplier,
		PlanGroupRateEnabled:       params.PlanGroupRateEnabled,
		CreatedAt:                  time.Now(),
	}
	r.runs[run.RunID] = run
	if params.IdempotencyKey != nil {
		r.byIdem[*params.IdempotencyKey] = run
	}
	outputs := make([]*CreativeRunOutput, 0, params.RequestedOutputCount)
	for index := 0; index < params.RequestedOutputCount; index++ {
		outputs = append(outputs, &CreativeRunOutput{RunID: run.RunID, OutputIndex: index, Status: CreativeRunOutputStatusPending})
	}
	r.outputs[run.RunID] = outputs
	return run, nil
}

func (r *creativeFakeRunRepo) GetCreativeRunByRunID(ctx context.Context, runID string) (*CreativeRun, error) {
	run, ok := r.runs[runID]
	if !ok {
		return nil, ErrCreativeRunNotFound
	}
	return run, nil
}

func (r *creativeFakeRunRepo) GetCreativeRunByRunIDForOwner(ctx context.Context, userID int64, runID string) (*CreativeRun, error) {
	run, err := r.GetCreativeRunByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.UserID != userID {
		return nil, ErrCreativeRunNotFound
	}
	return run, nil
}

func (r *creativeFakeRunRepo) GetCreativeRunByIdempotencyKey(ctx context.Context, userID int64, key string) (*CreativeRun, error) {
	run, ok := r.byIdem[key]
	if !ok || run.UserID != userID {
		return nil, ErrCreativeRunNotFound
	}
	return run, nil
}

func (r *creativeFakeRunRepo) ListCreativeRunsForOwner(ctx context.Context, userID int64, filter CreativeRunFilter) ([]*CreativeRun, error) {
	out := make([]*CreativeRun, 0)
	for _, run := range r.runs {
		if run.UserID == userID {
			out = append(out, run)
		}
	}
	return out, nil
}

func (r *creativeFakeRunRepo) TransitionCreativeRunStatus(ctx context.Context, runID, toStatus string, opts CreativeRunTransitionOptions) error {
	run, ok := r.runs[runID]
	if !ok {
		return ErrCreativeRunNotFound
	}
	if !CanTransitionCreativeRun(run.Status, toStatus) {
		return ErrCreativeInvalidTransition
	}
	run.Status = toStatus
	if opts.ErrorCode != nil {
		run.ErrorCode = opts.ErrorCode
	}
	if opts.ErrorMessage != nil {
		run.ErrorMessage = opts.ErrorMessage
	}
	if toStatus == CreativeRunStatusCancelled && opts.Now != nil {
		run.CancelledAt = opts.Now
	}
	r.transition = append(r.transition, runID+":"+toStatus)
	return nil
}

func (r *creativeFakeRunRepo) MarkCreativeRunRunning(ctx context.Context, runID string, accountID int64, now time.Time) error {
	run, ok := r.runs[runID]
	if !ok {
		return ErrCreativeRunNotFound
	}
	if run.Status == CreativeRunStatusRunning {
		return nil
	}
	if !CanTransitionCreativeRun(run.Status, CreativeRunStatusRunning) {
		return ErrCreativeInvalidTransition
	}
	run.Status = CreativeRunStatusRunning
	run.AccountID = &accountID
	run.StartedAt = &now
	return nil
}

func (r *creativeFakeRunRepo) MarkCreativeRunSucceeded(ctx context.Context, runID string, actualCost float64, now time.Time) error {
	run, ok := r.runs[runID]
	if !ok {
		return ErrCreativeRunNotFound
	}
	if run.Status != CreativeRunStatusRunning {
		return ErrCreativeInvalidTransition
	}
	run.Status = CreativeRunStatusSucceeded
	run.ActualCost = &actualCost
	run.CompletedAt = &now
	return nil
}

func (r *creativeFakeRunRepo) UpdateCreativeRunOutput(ctx context.Context, runID string, outputIndex int, status, mimeType string, byteSize int64, transientExpiresAt *time.Time, errorCode, errorMessage string) error {
	outputs, ok := r.outputs[runID]
	if !ok {
		return ErrCreativeOutputNotFound
	}
	for _, output := range outputs {
		if output.OutputIndex == outputIndex {
			if output.Status == CreativeRunOutputStatusAcked {
				return nil
			}
			output.Status = status
			output.MimeType = &mimeType
			output.ByteSize = &byteSize
			output.TransientExpiresAt = transientExpiresAt
			if errorCode != "" {
				output.ErrorCode = &errorCode
			}
			return nil
		}
	}
	return ErrCreativeOutputNotFound
}

func (r *creativeFakeRunRepo) GetCreativeRunOutput(ctx context.Context, runID string, outputIndex int) (*CreativeRunOutput, error) {
	for _, output := range r.outputs[runID] {
		if output.OutputIndex == outputIndex {
			return output, nil
		}
	}
	return nil, ErrCreativeOutputNotFound
}

func (r *creativeFakeRunRepo) ListCreativeRunOutputs(ctx context.Context, runID string) ([]*CreativeRunOutput, error) {
	return r.outputs[runID], nil
}

func (r *creativeFakeRunRepo) MarkCreativeRunOutputAcked(ctx context.Context, runID string, outputIndex int, now time.Time) error {
	output, err := r.GetCreativeRunOutput(ctx, runID, outputIndex)
	if err != nil {
		return err
	}
	if output.Status != CreativeRunOutputStatusSucceeded {
		return ErrCreativeOutputNotReady
	}
	output.Status = CreativeRunOutputStatusAcked
	output.AckedAt = &now
	return nil
}

func (r *creativeFakeRunRepo) ListCreativeRunsDueForTransientCleanup(ctx context.Context, cutoff time.Time, limit int) ([]*CreativeRun, error) {
	return nil, nil
}

// IncrementCreativeRunAttempt 模拟原子递增并返回最新值。
func (r *creativeFakeRunRepo) IncrementCreativeRunAttempt(ctx context.Context, runID string) (int, error) {
	run, ok := r.runs[runID]
	if !ok {
		return 0, ErrCreativeRunNotFound
	}
	run.AttemptCount++
	return run.AttemptCount, nil
}

type creativeFakeManagedKeyRepo struct {
	key     *APIKey
	getErr  error
	createN int
}

func (r *creativeFakeManagedKeyRepo) GetManagedKeyByUserAndGroup(ctx context.Context, userID, groupID int64, managedBy string) (*APIKey, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.key != nil {
		return r.key, nil
	}
	return nil, ErrAPIKeyNotFound
}

func (r *creativeFakeManagedKeyRepo) CreateManagedKey(ctx context.Context, key *APIKey) error {
	r.createN++
	if key.ID == 0 {
		key.ID = 900 + int64(r.createN)
	}
	r.key = key
	return nil
}

type creativeFakeUserRepo struct {
	user *User
}

func (r *creativeFakeUserRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	if r.user == nil {
		return nil, ErrUserNotFound
	}
	return r.user, nil
}

type creativeFakeGroupRepo struct {
	byID   map[int64]*Group
	active []Group
}

func (r *creativeFakeGroupRepo) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	group, ok := r.byID[id]
	if !ok {
		return nil, ErrCreativeGroupForbidden
	}
	return group, nil
}

func (r *creativeFakeGroupRepo) ListActive(ctx context.Context) ([]Group, error) {
	return r.active, nil
}

type creativeFakeAccountRepo struct {
	byGroup map[int64][]Account
}

func (r *creativeFakeAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	return r.byGroup[groupID], nil
}

type creativeFakeRateRepo struct{}

func (r *creativeFakeRateRepo) GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error) {
	return nil, nil
}

type creativeFakeBillingRepo struct {
	reserveN   int
	captureN   int
	releaseN   int
	reserveIDs []string
	captureIDs []string
	releaseIDs []string
}

func (r *creativeFakeBillingRepo) Apply(ctx context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	return nil, nil
}

func (r *creativeFakeBillingRepo) ReserveBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	r.reserveN++
	r.reserveIDs = append(r.reserveIDs, cmd.RequestID)
	return &BatchImageBalanceHoldResult{
		Applied:            true,
		HoldAmountUSD:      cmd.HoldAmount,
		EstimatedAmountUSD: cmd.HoldAmount,
		BalanceAmountUSD:   cmd.HoldAmount,
	}, nil
}

func (r *creativeFakeBillingRepo) CaptureBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	r.captureN++
	r.captureIDs = append(r.captureIDs, cmd.RequestID)
	return &BatchImageBalanceHoldResult{
		Applied:         true,
		ActualAmountUSD: cmd.ActualBaseAmountUSD,
	}, nil
}

func (r *creativeFakeBillingRepo) ReleaseBatchImageBalance(ctx context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	r.releaseN++
	r.releaseIDs = append(r.releaseIDs, cmd.RequestID)
	return &BatchImageBalanceHoldResult{Applied: true}, nil
}

type creativeFakeQueue struct {
	enqueued      []string
	reserveBatch  []string
	acked         []string
	requeued      []string
	locksGranted  int
	lastLock      *creativeFakeJobLock
	reserveFailed bool
}

// creativeFakeJobLock 记录锁的释放。
type creativeFakeJobLock struct {
	released bool
}

func (l *creativeFakeJobLock) Release(ctx context.Context) error {
	l.released = true
	return nil
}

func (q *creativeFakeQueue) Enqueue(ctx context.Context, runID string) error {
	q.enqueued = append(q.enqueued, runID)
	return nil
}

func (q *creativeFakeQueue) Reserve(ctx context.Context, blockTimeout time.Duration) (ReservedCreativeRun, error) {
	if len(q.reserveBatch) == 0 {
		return ReservedCreativeRun{}, ErrCreativeQueueEmpty
	}
	runID := q.reserveBatch[0]
	q.reserveBatch = q.reserveBatch[1:]
	return ReservedCreativeRun{RunID: runID}, nil
}

func (q *creativeFakeQueue) RequeueAfter(ctx context.Context, runID string, delay time.Duration) error {
	q.requeued = append(q.requeued, runID)
	return nil
}

func (q *creativeFakeQueue) Ack(ctx context.Context, runID string) error {
	q.acked = append(q.acked, runID)
	return nil
}

func (q *creativeFakeQueue) Heartbeat(ctx context.Context, runID string) error { return nil }

func (q *creativeFakeQueue) MoveDueDelayedToReady(ctx context.Context, limit int) (int, error) {
	return 0, nil
}

func (q *creativeFakeQueue) RecoverStaleActive(ctx context.Context, staleAfter time.Duration, limit int) (int, error) {
	return 0, nil
}

func (q *creativeFakeQueue) TryAcquireJobLock(ctx context.Context, runID string, ttl time.Duration) (CreativeRunJobLock, bool, error) {
	lock := &creativeFakeJobLock{}
	q.locksGranted++
	q.lastLock = lock
	return lock, true, nil
}

type creativeFakeTransient struct {
	payloads map[string]*CreativeRunPayload
	inputs   map[string][]byte
	masks    map[string][]byte
	outputs  map[string][]byte
}

func newCreativeFakeTransient() *creativeFakeTransient {
	return &creativeFakeTransient{
		payloads: make(map[string]*CreativeRunPayload),
		inputs:   make(map[string][]byte),
		masks:    make(map[string][]byte),
		outputs:  make(map[string][]byte),
	}
}

func (s *creativeFakeTransient) SavePayload(ctx context.Context, runID string, payload *CreativeRunPayload) error {
	s.payloads[runID] = payload
	return nil
}

func (s *creativeFakeTransient) LoadPayload(ctx context.Context, runID string) (*CreativeRunPayload, error) {
	payload, ok := s.payloads[runID]
	if !ok {
		return nil, ErrCreativeTransientFailed
	}
	return payload, nil
}

func (s *creativeFakeTransient) SaveInput(ctx context.Context, runID string, idx int, data []byte) error {
	s.inputs[fmtInputKey(runID, idx)] = data
	return nil
}

func (s *creativeFakeTransient) LoadInputs(ctx context.Context, runID string, count int) ([][]byte, error) {
	out := make([][]byte, 0, count)
	for idx := 0; idx < count; idx++ {
		data, ok := s.inputs[fmtInputKey(runID, idx)]
		if !ok {
			return nil, ErrCreativeTransientFailed
		}
		out = append(out, data)
	}
	return out, nil
}

func (s *creativeFakeTransient) SaveMask(ctx context.Context, runID string, data []byte) error {
	s.masks[runID] = data
	return nil
}

func (s *creativeFakeTransient) LoadMask(ctx context.Context, runID string) ([]byte, error) {
	data, ok := s.masks[runID]
	if !ok {
		return nil, ErrCreativeTransientFailed
	}
	return data, nil
}

func (s *creativeFakeTransient) SaveOutput(ctx context.Context, runID string, index int, data []byte, ttl time.Duration) error {
	s.outputs[fmtInputKey(runID, index)] = data
	return nil
}

func (s *creativeFakeTransient) LoadOutput(ctx context.Context, runID string, index int) ([]byte, error) {
	data, ok := s.outputs[fmtInputKey(runID, index)]
	if !ok {
		return nil, ErrCreativeTransientFailed
	}
	return data, nil
}

func (s *creativeFakeTransient) DeleteOutput(ctx context.Context, runID string, index int) error {
	delete(s.outputs, fmtInputKey(runID, index))
	return nil
}

func (s *creativeFakeTransient) DeleteRunTransient(ctx context.Context, runID string, inputCount, outputCount int) error {
	delete(s.payloads, runID)
	delete(s.masks, runID)
	return nil
}

func fmtInputKey(runID string, idx int) string {
	return runID + ":" + strconv.Itoa(idx)
}

// ---------------------------------------------------------------------------
// 测试场景
// ---------------------------------------------------------------------------

func newCreativeTestGroup() *Group {
	price1k := 0.02
	price2k := 0.04
	return &Group{
		ID:                   12,
		Name:                 "Gemini Image",
		Platform:             PlatformGemini,
		Status:               StatusActive,
		AllowImageGeneration: true,
		RateMultiplier:       1,
		ImagePrice1K:         &price1k,
		ImagePrice2K:         &price2k,
	}
}

func newCreativeTestAccountRepo() *creativeFakeAccountRepo {
	return &creativeFakeAccountRepo{
		byGroup: map[int64][]Account{
			12: {
				{
					ID:          55,
					Status:      StatusActive,
					Schedulable: true,
					Credentials: map[string]any{
						"model_mapping": map[string]any{
							"gemini-3.1-flash-image": "gemini-3.1-flash-image",
						},
					},
				},
			},
		},
	}
}

func newCreativeTestService() *CreativePublicService {
	group := newCreativeTestGroup()
	return &CreativePublicService{
		Repo:              newCreativeFakeRunRepo(),
		ApiKeyRepo:        &creativeFakeManagedKeyRepo{},
		UserRepo:          &creativeFakeUserRepo{user: &User{ID: 7}},
		AccountRepo:       newCreativeTestAccountRepo(),
		GroupRepo:         &creativeFakeGroupRepo{byID: map[int64]*Group{12: group}, active: []Group{*group}},
		UserGroupRateRepo: &creativeFakeRateRepo{},
		Queue:             &creativeFakeQueue{},
		TransientStore:    newCreativeFakeTransient(),
		BillingRepo:       &creativeFakeBillingRepo{},
		Config: &config.Config{
			Creative: config.CreativeConfig{
				Enabled:                 true,
				TransientTTLSeconds:     1800,
				MaxAssetBytes:           33554432,
				MaxTotalInputBytes:      67108864,
				MaxOutputCount:          4,
				MaxPromptChars:          8000,
				DefaultResponseMimeType: "image/png",
				DefaultImageSize:        "1K",
			},
			Default: config.DefaultConfig{APIKeyPrefix: "sk-"},
		},
	}
}

func makeTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func validCreateParams() CreateCreativeRunParamsPublic {
	return CreateCreativeRunParamsPublic{
		GroupID:      12,
		Model:        "gemini-3.1-flash-image",
		Operation:    CreativeOperationGenerate,
		Prompt:       "画一只猫",
		ImageSize:    "1K",
		OutputCount:  1,
		ResponseMIME: "image/png",
	}
}

// TestValidateCreateParams 覆盖 CreateRun 的参数校验矩阵。
func TestValidateCreateParams(t *testing.T) {
	t.Run("模型不存在", func(t *testing.T) {
		svc := newCreativeTestService()
		params := validCreateParams()
		params.Model = "gemini-9.9-unknown"
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeInvalidModel)
	})

	t.Run("分组未开启图片生成", func(t *testing.T) {
		svc := newCreativeTestService()
		group := newCreativeTestGroup()
		group.AllowImageGeneration = false
		svc.GroupRepo.(*creativeFakeGroupRepo).byID[12] = group
		params := validCreateParams()
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeGroupImageDisabled)
	})

	t.Run("grok 平台不支持 inpaint", func(t *testing.T) {
		svc := newCreativeTestService()
		group := newCreativeTestGroup()
		group.Platform = PlatformGrok
		svc.GroupRepo.(*creativeFakeGroupRepo).byID[12] = group
		svc.AccountRepo.(*creativeFakeAccountRepo).byGroup[12] = []Account{{
			ID:          56,
			Status:      StatusActive,
			Schedulable: true,
			Credentials: map[string]any{"model_mapping": map[string]any{"grok-imagine": "grok-imagine"}},
		}}
		params := validCreateParams()
		params.Model = "grok-imagine"
		params.Operation = CreativeOperationInpaint
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeOperationUnsupported)
	})

	t.Run("输出数量超出上限", func(t *testing.T) {
		svc := newCreativeTestService()
		params := validCreateParams()
		params.OutputCount = 5
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeInvalidParams)
	})

	t.Run("prompt 超长", func(t *testing.T) {
		svc := newCreativeTestService()
		params := validCreateParams()
		params.Prompt = strings.Repeat("a", 9000)
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativePromptTooLong)
	})

	t.Run("非法 MIME", func(t *testing.T) {
		svc := newCreativeTestService()
		params := validCreateParams()
		params.SourceImages = []CreativeInputImage{{Bytes: []byte("not-an-image"), Mime: "image/gif"}}
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeInvalidMime)
	})

	t.Run("单文件超限", func(t *testing.T) {
		svc := newCreativeTestService()
		svc.Config.Creative.MaxAssetBytes = 16
		params := validCreateParams()
		params.SourceImages = []CreativeInputImage{{Bytes: makeTestPNG(t, 4, 4), Mime: "image/png"}}
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeAssetTooLarge)
	})

	t.Run("总输入超限", func(t *testing.T) {
		svc := newCreativeTestService()
		svc.Config.Creative.MaxTotalInputBytes = 100
		params := validCreateParams()
		params.SourceImages = []CreativeInputImage{
			{Bytes: makeTestPNG(t, 8, 8), Mime: "image/png"},
			{Bytes: makeTestPNG(t, 8, 8), Mime: "image/png"},
		}
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeInputTooLarge)
	})

	t.Run("inpaint 缺 mask", func(t *testing.T) {
		svc := newCreativeTestService()
		params := validCreateParams()
		params.Operation = CreativeOperationInpaint
		params.SourceImages = []CreativeInputImage{{Bytes: makeTestPNG(t, 8, 8), Mime: "image/png"}}
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeMaskRequired)
	})

	t.Run("mask 非 PNG", func(t *testing.T) {
		svc := newCreativeTestService()
		params := validCreateParams()
		params.Operation = CreativeOperationInpaint
		params.SourceImages = []CreativeInputImage{{Bytes: makeTestPNG(t, 8, 8), Mime: "image/png"}}
		params.Mask = &CreativeInputImage{Bytes: []byte{0xFF, 0xD8, 0xFF, 0x00}, Mime: "image/jpeg"}
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeMaskRequired)
	})

	t.Run("mask 尺寸不一致", func(t *testing.T) {
		svc := newCreativeTestService()
		params := validCreateParams()
		params.Operation = CreativeOperationInpaint
		params.SourceImages = []CreativeInputImage{{Bytes: makeTestPNG(t, 8, 8), Mime: "image/png"}}
		params.Mask = &CreativeInputImage{Bytes: makeTestPNG(t, 16, 16), Mime: "image/png"}
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeMaskSizeMismatch)
	})

	t.Run("非 inpaint 不允许 mask", func(t *testing.T) {
		svc := newCreativeTestService()
		params := validCreateParams()
		params.SourceImages = []CreativeInputImage{{Bytes: makeTestPNG(t, 8, 8), Mime: "image/png"}}
		params.Mask = &CreativeInputImage{Bytes: makeTestPNG(t, 8, 8), Mime: "image/png"}
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeInvalidParams)
	})

	t.Run("edit 必须带源图", func(t *testing.T) {
		svc := newCreativeTestService()
		params := validCreateParams()
		params.Operation = CreativeOperationEdit
		_, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.ErrorIs(t, err, ErrCreativeInvalidParams)
	})

	t.Run("合法 inpaint 通过且默认值生效", func(t *testing.T) {
		svc := newCreativeTestService()
		params := validCreateParams()
		params.Operation = CreativeOperationInpaint
		params.ImageSize = ""
		params.OutputCount = 0
		params.SourceImages = []CreativeInputImage{{Bytes: makeTestPNG(t, 8, 8), Mime: "image/png"}}
		params.Mask = &CreativeInputImage{Bytes: makeTestPNG(t, 8, 8), Mime: "image/png"}
		validated, err := svc.validateCreateParams(context.Background(), 7, &params)
		require.NoError(t, err)
		require.Equal(t, "1K", validated.imageSize)
		require.Equal(t, 1, validated.outputCount)
		require.NotEmpty(t, validated.fingerprint)
		require.NotEmpty(t, validated.promptHash)
	})
}

// TestCreateRunIdempotency 覆盖幂等重放与幂等冲突。
func TestCreateRunIdempotency(t *testing.T) {
	svc := newCreativeTestService()
	ctx := context.Background()

	first, err := svc.CreateRun(ctx, 7, validCreateParams(), "idem-key-1")
	require.NoError(t, err)
	require.False(t, first.IdempotentReplay)
	require.Equal(t, CreativeRunStatusQueued, first.Status)
	require.True(t, IsValidCreativeRunID(first.ID))

	// 相同 Key + 相同请求体：返回原任务并标记重放。
	replay, err := svc.CreateRun(ctx, 7, validCreateParams(), "idem-key-1")
	require.NoError(t, err)
	require.True(t, replay.IdempotentReplay)
	require.Equal(t, first.ID, replay.ID)

	// 相同 Key + 不同请求体：返回冲突。
	conflictParams := validCreateParams()
	conflictParams.Prompt = "完全不同的 prompt"
	_, err = svc.CreateRun(ctx, 7, conflictParams, "idem-key-1")
	require.ErrorIs(t, err, ErrCreativeRunIdempotencyConflict)

	// 计费预占与入队各发生一次（重放不重复扣费/入队）。
	require.Equal(t, 1, svc.BillingRepo.(*creativeFakeBillingRepo).reserveN)
	require.Len(t, svc.Queue.(*creativeFakeQueue).enqueued, 1)
}

// TestEnsureCreativeManagedKey 校验隐藏执行 Key 的幂等供应。
func TestEnsureCreativeManagedKey(t *testing.T) {
	svc := newCreativeTestService()
	ctx := context.Background()

	key, err := svc.ensureCreativeManagedKey(ctx, 7, 12)
	require.NoError(t, err)
	require.NotNil(t, key.ManagedBy)
	require.Equal(t, CreativeManagedBy, *key.ManagedBy)
	require.Equal(t, APIKeyBillingModeAuto, key.BillingMode)
	require.Equal(t, int64(12), *key.GroupID)
	require.Equal(t, "creative-studio:12", key.Name)

	// 第二次供应直接复用已创建的 Key。
	reused, err := svc.ensureCreativeManagedKey(ctx, 7, 12)
	require.NoError(t, err)
	require.Equal(t, key.ID, reused.ID)
	require.Equal(t, 1, svc.ApiKeyRepo.(*creativeFakeManagedKeyRepo).createN)
}

// creativeFakeSettingReader 是 CreativeSettingReader 的测试替身。
type creativeFakeSettingReader struct {
	enabled bool
}

func (f *creativeFakeSettingReader) IsCreativeEnabled(ctx context.Context) bool {
	return f.enabled
}

// TestCreativeEnabledGate 校验数据库运行时开关 creative_enabled 的门控语义：
// 关闭时 ListModels 返回空列表（前端展示"已停用"空态），CreateRun 返回 ErrCreativeDisabled。
func TestCreativeEnabledGate(t *testing.T) {
	t.Run("运行时关闭时 ListModels 返回空列表", func(t *testing.T) {
		svc := newCreativeTestService()
		svc.Settings = &creativeFakeSettingReader{enabled: false}

		models, err := svc.ListModels(context.Background(), 7)
		require.NoError(t, err)
		require.NotNil(t, models)
		require.Empty(t, models.Data)
	})

	t.Run("运行时关闭时 CreateRun 拒绝", func(t *testing.T) {
		svc := newCreativeTestService()
		svc.Settings = &creativeFakeSettingReader{enabled: false}

		_, err := svc.CreateRun(context.Background(), 7, validCreateParams(), "")
		require.ErrorIs(t, err, ErrCreativeDisabled)
	})

	t.Run("运行时开启时 ListModels 正常返回", func(t *testing.T) {
		svc := newCreativeTestService()
		svc.Settings = &creativeFakeSettingReader{enabled: true}

		models, err := svc.ListModels(context.Background(), 7)
		require.NoError(t, err)
		require.NotEmpty(t, models.Data)
	})

	t.Run("进程配置关闭时运行时开关无法打开", func(t *testing.T) {
		svc := newCreativeTestService()
		svc.Config.Creative.Enabled = false
		svc.Settings = &creativeFakeSettingReader{enabled: true}

		models, err := svc.ListModels(context.Background(), 7)
		require.NoError(t, err)
		require.Empty(t, models.Data)
	})
}
