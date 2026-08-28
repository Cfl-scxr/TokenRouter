//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 越权访问：非本人任务必须全部拒绝
// ---------------------------------------------------------------------------

// seedOwnedRun 种入一个属于指定用户的 succeeded 任务（含一张 succeeded 输出）。
func seedOwnedRun(t *testing.T, svc *CreativePublicService, runID string, userID int64, status string) {
	t.Helper()
	repo := svc.Repo.(*creativeFakeRunRepo)
	expires := time.Now().Add(30 * time.Minute)
	repo.runs[runID] = &CreativeRun{
		RunID:                runID,
		UserID:               userID,
		GroupID:              12,
		APIKeyID:             900,
		Model:                "gemini-3.1-flash-image",
		Operation:            CreativeOperationGenerate,
		RequestedOutputCount: 1,
		Status:               status,
		EstimatedCost:        0.02,
	}
	repo.outputs[runID] = []*CreativeRunOutput{
		{
			RunID:              runID,
			OutputIndex:        0,
			Status:             CreativeRunOutputStatusSucceeded,
			MimeType:           creativeStringValuePtr("image/png"),
			ByteSize:           creativeInt64ValuePtr(4),
			TransientExpiresAt: &expires,
		},
	}
}

func creativeStringValuePtr(v string) *string { return &v }

func creativeInt64ValuePtr(v int64) *int64 { return &v }

func TestCreativeOwnershipEnforced(t *testing.T) {
	svc := newCreativeTestService()
	ctx := context.Background()
	runID := "crun_ownerother00001"
	// 任务属于用户 99，当前用户是 7。
	seedOwnedRun(t, svc, runID, 99, CreativeRunStatusSucceeded)
	store := svc.TransientStore.(*creativeFakeTransient)
	store.outputs[runID+":0"] = []byte("img")

	_, err := svc.GetRun(ctx, 7, runID)
	require.ErrorIs(t, err, ErrCreativeRunNotFound)

	_, err = svc.GetOutputContent(ctx, 7, runID, 0)
	require.ErrorIs(t, err, ErrCreativeRunNotFound)

	err = svc.AckOutput(ctx, 7, runID, 0)
	require.ErrorIs(t, err, ErrCreativeRunNotFound)

	_, err = svc.CancelRun(ctx, 7, runID)
	require.ErrorIs(t, err, ErrCreativeRunNotFound)

	// 本人访问不受影响。
	got, err := svc.GetRun(ctx, 99, runID)
	require.NoError(t, err)
	require.Equal(t, runID, got.ID)
}

// ---------------------------------------------------------------------------
// 临时结果过期降级
// ---------------------------------------------------------------------------

func TestCreativeGetOutputContentExpiresToResultLost(t *testing.T) {
	svc := newCreativeTestService()
	ctx := context.Background()
	runID := "crun_outputexpired001"
	repo := svc.Repo.(*creativeFakeRunRepo)
	// succeeded 任务，输出已过期且临时键已不存在。
	past := time.Now().Add(-time.Minute)
	repo.runs[runID] = &CreativeRun{
		RunID:                runID,
		UserID:               7,
		GroupID:              12,
		APIKeyID:             900,
		Model:                "gemini-3.1-flash-image",
		Operation:            CreativeOperationGenerate,
		RequestedOutputCount: 1,
		Status:               CreativeRunStatusSucceeded,
		EstimatedCost:        0.02,
	}
	repo.outputs[runID] = []*CreativeRunOutput{
		{RunID: runID, OutputIndex: 0, Status: CreativeRunOutputStatusSucceeded, MimeType: creativeStringValuePtr("image/png"), TransientExpiresAt: &past},
	}

	_, err := svc.GetOutputContent(ctx, 7, runID, 0)
	require.ErrorIs(t, err, ErrCreativeOutputExpired)
	// 成功任务不得伪装成功：必须降级为 result_lost。
	require.Equal(t, CreativeRunStatusResultLost, repo.runs[runID].Status)
}

func TestCreativeGetOutputContentMissingTransientToResultLost(t *testing.T) {
	svc := newCreativeTestService()
	ctx := context.Background()
	runID := "crun_outputmissing001"
	repo := svc.Repo.(*creativeFakeRunRepo)
	future := time.Now().Add(30 * time.Minute)
	repo.runs[runID] = &CreativeRun{
		RunID:                runID,
		UserID:               7,
		GroupID:              12,
		APIKeyID:             900,
		Model:                "gemini-3.1-flash-image",
		Operation:            CreativeOperationGenerate,
		RequestedOutputCount: 1,
		Status:               CreativeRunStatusSucceeded,
		EstimatedCost:        0.02,
	}
	repo.outputs[runID] = []*CreativeRunOutput{
		{RunID: runID, OutputIndex: 0, Status: CreativeRunOutputStatusSucceeded, MimeType: creativeStringValuePtr("image/png"), TransientExpiresAt: &future},
	}
	// 注意：临时存储中没有输出字节（worker 丢失 / 已被清理）。

	_, err := svc.GetOutputContent(ctx, 7, runID, 0)
	require.ErrorIs(t, err, ErrCreativeResultLost)
	require.Equal(t, CreativeRunStatusResultLost, repo.runs[runID].Status)
}

func TestCreativeGetOutputContentSuccess(t *testing.T) {
	svc := newCreativeTestService()
	ctx := context.Background()
	runID := "crun_outputok000000001"
	seedOwnedRun(t, svc, runID, 7, CreativeRunStatusSucceeded)
	store := svc.TransientStore.(*creativeFakeTransient)
	store.outputs[runID+":0"] = []byte("png-bytes")

	content, err := svc.GetOutputContent(ctx, 7, runID, 0)
	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), content.Content)
	require.Equal(t, "image/png", content.ContentType)
	// 成功读取不得误降级。
	require.Equal(t, CreativeRunStatusSucceeded, svc.Repo.(*creativeFakeRunRepo).runs[runID].Status)
}

// ---------------------------------------------------------------------------
// 重复结算幂等
// ---------------------------------------------------------------------------

// creativeFakeUsageLogRepo 只记录 Create 调用（内嵌接口使未覆盖方法 panic 于 nil）。
type creativeFakeUsageLogRepo struct {
	UsageLogRepository
	logs []*UsageLog
}

func (r *creativeFakeUsageLogRepo) Create(ctx context.Context, log *UsageLog) (bool, error) {
	r.logs = append(r.logs, log)
	return true, nil
}

func TestCreativeSucceedRunIdempotentSettlement(t *testing.T) {
	svc := newCreativeTestService()
	ctx := context.Background()
	usageRepo := &creativeFakeUsageLogRepo{}
	svc.UsageLogRepo = usageRepo
	repo := svc.Repo.(*creativeFakeRunRepo)
	billing := svc.BillingRepo.(*creativeFakeBillingRepo)

	runID := "crun_settleidempotent1"
	accountID := int64(55)
	repo.runs[runID] = &CreativeRun{
		RunID:                runID,
		UserID:               7,
		GroupID:              12,
		APIKeyID:             900,
		AccountID:            &accountID,
		Model:                "gemini-3.1-flash-image",
		Operation:            CreativeOperationGenerate,
		RequestedOutputCount: 1,
		Status:               CreativeRunStatusRunning,
		EstimatedCost:        0.02,
		BaseUnitPrice:        0.02,
	}
	repo.outputs[runID] = []*CreativeRunOutput{
		{RunID: runID, OutputIndex: 0, Status: CreativeRunOutputStatusPending},
	}
	results := []CreativeOutputResult{{Index: 0, Success: true, Bytes: []byte("img"), Mime: "image/png"}}

	first, err := svc.SucceedRun(ctx, runID, accountID, results)
	require.NoError(t, err)
	require.Equal(t, CreativeRunStatusSucceeded, first.Status)

	second, err := svc.SucceedRun(ctx, runID, accountID, results)
	require.NoError(t, err)
	require.Equal(t, CreativeRunStatusSucceeded, second.Status)

	// 捕获与用量日志各只发生一次，且幂等键稳定。
	require.Equal(t, 1, billing.captureN)
	require.Equal(t, []string{"creative_capture:" + runID}, billing.captureIDs)
	require.Len(t, usageRepo.logs, 1)
	require.Equal(t, "creative_settle:"+runID, usageRepo.logs[0].RequestID)
	require.Equal(t, "image", stringValue(usageRepo.logs[0].BillingMode))
	require.Equal(t, 1, usageRepo.logs[0].ImageCount)
	// 输出行保持一条 succeeded，重复结算不产生重复输出。
	outputs := repo.outputs[runID]
	require.Len(t, outputs, 1)
	require.Equal(t, CreativeRunOutputStatusSucceeded, outputs[0].Status)
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// ---------------------------------------------------------------------------
// PostgreSQL 不保存素材：CreateRun 落库的只有哈希与元数据
// ---------------------------------------------------------------------------

func TestCreativeCreateRunPersistsOnlyMetadata(t *testing.T) {
	svc := newCreativeTestService()
	ctx := context.Background()
	repo := svc.Repo.(*creativeFakeRunRepo)
	store := svc.TransientStore.(*creativeFakeTransient)

	params := validCreateParams()
	params.Prompt = "这是一段绝不应落库的 prompt 明文"
	params.SourceImages = []CreativeInputImage{{Bytes: makeTestPNG(t, 4, 4), Mime: "image/png"}}
	created, err := svc.CreateRun(ctx, 7, params, "")
	require.NoError(t, err)
	require.True(t, IsValidCreativeRunID(created.ID))

	require.Len(t, repo.createParams, 1)
	stored := repo.createParams[0]
	// prompt 只以 sha256 落库。
	require.Equal(t, sha256Hex([]byte(params.Prompt)), stored.PromptHash)
	require.NotContains(t, stored.PromptHash, "绝不应落库")
	require.NotEmpty(t, stored.RequestFingerprint)
	// 结构体中不存在任何图片字节/prompt 明文字段，输出行只有元数据。
	require.Equal(t, 1, stored.RequestedOutputCount)
	for _, output := range repo.outputs[created.ID] {
		require.Equal(t, CreativeRunOutputStatusPending, output.Status)
		require.Nil(t, output.MimeType)
		require.Nil(t, output.ByteSize)
	}
	// 图片字节只进入临时存储。
	_, err = store.LoadInputs(ctx, created.ID, 1)
	require.NoError(t, err)
	require.Empty(t, store.outputs)
}

// ---------------------------------------------------------------------------
// ListModels 权限与内容
// ---------------------------------------------------------------------------

func TestCreativeListModelsFiltersAndContent(t *testing.T) {
	svc := newCreativeTestService()
	ctx := context.Background()
	groupRepo := svc.GroupRepo.(*creativeFakeGroupRepo)

	// 无图片权限的分组不出现在模型列表。
	noImage := newCreativeTestGroup()
	noImage.ID = 13
	noImage.AllowImageGeneration = false
	groupRepo.byID[13] = noImage
	groupRepo.active = append(groupRepo.active, *noImage)

	// 不支持的图片平台（anthropic）不出现。
	unsupported := newCreativeTestGroup()
	unsupported.ID = 14
	unsupported.Platform = PlatformAnthropic
	unsupported.Name = "Claude"
	groupRepo.byID[14] = unsupported
	groupRepo.active = append(groupRepo.active, *unsupported)

	got, err := svc.ListModels(ctx, 7)
	require.NoError(t, err)
	require.NotEmpty(t, got.Data)
	for _, item := range got.Data {
		require.Equal(t, int64(12), item.GroupID, "无图片权限或不受支持的分组不得进入模型列表")
		require.Equal(t, []string{"generate", "edit", "inpaint"}, item.Operations)
		require.Equal(t, []string{"1K", "2K"}, item.ImageSizes)
		require.InDelta(t, 0.02, item.Price1K, 1e-9)
		require.Equal(t, "gemini-3.1-flash-image", item.Model)
	}
}
