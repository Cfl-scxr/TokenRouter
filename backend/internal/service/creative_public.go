package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"sort"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/domain"
	"github.com/TokenFlux/TokenRouter/internal/pkg/ctxkey"
	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/errors"
	"github.com/TokenFlux/TokenRouter/internal/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/image/webp"
)

const (
	defaultCreativeMaxPromptChars = 8000
	defaultCreativeResponseMime   = "image/png"
	defaultCreativeImageSize      = "1K"
	maxCreativeErrorMessageChars  = 500
)

// ErrCreativeContentBlocked 是内容审核命中后的拒绝错误。
var ErrCreativeContentBlocked = infraerrors.New(403, "CREATIVE_CONTENT_BLOCKED", "creative content failed moderation")

// CreativePublicService 是创作台的用户侧服务：模型列表、任务创建、查询与输出获取。
type CreativePublicService struct {
	Repo              CreativeRunRepository
	ApiKeyRepo        CreativeManagedKeyRepository
	UserRepo          CreativeUserRepository
	AccountRepo       CreativeAccountRepository
	GroupRepo         CreativeGroupRepository
	UserGroupRateRepo CreativeUserGroupRateRepository
	Queue             CreativeRunQueue
	TransientStore    CreativeTransientStore
	BillingRepo       UsageBillingRepository
	UsageLogRepo      UsageLogRepository
	Pricing           *BillingService
	PricingResolver   *ModelPricingResolver
	Moderation        *ContentModerationService
	AuthCache         APIKeyAuthCacheInvalidator
	Settings          CreativeSettingReader
	Config            *config.Config
}

// CreativeSettingReader 是创作台运行时开关的读取接口，由 SettingService 实现。
type CreativeSettingReader interface {
	// IsCreativeEnabled 读取数据库开关 creative_enabled，缺省视为开启。
	IsCreativeEnabled(ctx context.Context) bool
	// GetCreativeModelSettings 读取创作台模型白名单；缺失或异常时返回空列表。
	GetCreativeModelSettings(ctx context.Context) []CreativeModelSetting
}

// 以下窄接口只依赖真正用到的方法，便于单测替身实现；生产环境由现有仓储实现。
type CreativeUserRepository interface {
	GetByID(ctx context.Context, id int64) (*User, error)
}

type CreativeGroupRepository interface {
	GetByIDLite(ctx context.Context, id int64) (*Group, error)
	ListActive(ctx context.Context) ([]Group, error)
}

type CreativeAccountRepository interface {
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error)
}

type CreativeUserGroupRateRepository interface {
	GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error)
}

// CreativeManagedKeyRepository 供应创作台隐藏执行 Key（managed_by = 'creative_studio'）。
type CreativeManagedKeyRepository interface {
	GetManagedKeyByUserAndGroup(ctx context.Context, userID, groupID int64, managedBy string) (*APIKey, error)
	CreateManagedKey(ctx context.Context, key *APIKey) error
}

// CreativeManagedKeyAPIKey 是 ApiKeyRepo 生产实现的组合接口（由 apiKeyRepository 实现）。
type CreativeManagedKeyAPIKey interface {
	CreativeManagedKeyRepository
}

func NewCreativePublicService(
	repo CreativeRunRepository,
	apiKeyRepo CreativeManagedKeyRepository,
	userRepo CreativeUserRepository,
	accountRepo CreativeAccountRepository,
	groupRepo CreativeGroupRepository,
	userGroupRateRepo CreativeUserGroupRateRepository,
	queue CreativeRunQueue,
	transientStore CreativeTransientStore,
	billingRepo UsageBillingRepository,
	usageLogRepo UsageLogRepository,
	pricing *BillingService,
	pricingResolver *ModelPricingResolver,
	moderation *ContentModerationService,
	authCache APIKeyAuthCacheInvalidator,
	settings CreativeSettingReader,
	cfg *config.Config,
) *CreativePublicService {
	return &CreativePublicService{
		Repo:              repo,
		ApiKeyRepo:        apiKeyRepo,
		UserRepo:          userRepo,
		AccountRepo:       accountRepo,
		GroupRepo:         groupRepo,
		UserGroupRateRepo: userGroupRateRepo,
		Queue:             queue,
		TransientStore:    transientStore,
		BillingRepo:       billingRepo,
		UsageLogRepo:      usageLogRepo,
		Pricing:           pricing,
		PricingResolver:   pricingResolver,
		Moderation:        moderation,
		AuthCache:         authCache,
		Settings:          settings,
		Config:            cfg,
	}
}

// enabled 判定创作台是否可用：进程配置 creative.enabled 为前置条件，
// 再叠加数据库运行时开关 creative_enabled（缺省开启）。
func (s *CreativePublicService) enabled(ctx context.Context) bool {
	if s == nil || s.Repo == nil || s.GroupRepo == nil || s.Config == nil || !s.Config.Creative.Enabled {
		return false
	}
	// 设置服务缺失时无法确认白名单，按 fail-closed 处理。
	if s.Settings == nil {
		return false
	}
	return s.Settings.IsCreativeEnabled(ctx)
}

// ---------------------------------------------------------------------------
// 模型列表
// ---------------------------------------------------------------------------

// ListModels 返回当前用户可用的分组与图片模型组合。
func (s *CreativePublicService) ListModels(ctx context.Context, userID int64) (*CreativeModelsResponse, error) {
	if !s.enabled(ctx) {
		// 开关关闭时返回空列表而非错误：前端据此展示"已停用"空态，而不是报错。
		return &CreativeModelsResponse{Data: make([]CreativeModelPublic, 0)}, nil
	}
	user, err := s.UserRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, ErrUserNotFound
	}
	modelSettings := creativeModelSettingsIndex(s.creativeModelSettings(ctx))
	if len(modelSettings) == 0 {
		return &CreativeModelsResponse{Data: make([]CreativeModelPublic, 0)}, nil
	}
	groups, err := s.GroupRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	out := &CreativeModelsResponse{Data: make([]CreativeModelPublic, 0)}
	for i := range groups {
		group := &groups[i]
		if !user.CanBindGroup(group.ID, group.IsExclusive) {
			continue
		}
		if !group.AllowImageGeneration || !group.IsActive() {
			continue
		}
		platformOperations := creativeOperationsForPlatform(group.Platform)
		if len(platformOperations) == 0 {
			continue
		}
		models, err := s.creativeModelsForGroup(ctx, group)
		if err != nil {
			return nil, err
		}
		modelNames := make([]string, 0, len(models))
		for model := range models {
			modelNames = append(modelNames, model)
		}
		sort.Strings(modelNames)
		for _, model := range modelNames {
			operations, configured := creativeOperationsForModel(modelSettings, group.ID, model, platformOperations)
			if !configured || len(operations) == 0 {
				continue
			}
			// 尺寸按“分组+模型”解析：渠道/分组未配置覆盖价时回退平台默认档位。
			imageSizes := creativeImageSizesForGroupModel(group, model)
			if len(imageSizes) == 0 {
				continue
			}
			out.Data = append(out.Data, CreativeModelPublic{
				GroupID:    group.ID,
				GroupName:  group.Name,
				Model:      model,
				Operations: operations,
				ImageSizes: imageSizes,
				Qualities:  creativeQualitiesForPlatform(group.Platform),
				Price1K:    s.creativePrice(ctx, group, model, "1K"),
				Price2K:    s.creativePrice(ctx, group, model, "2K"),
				Price4K:    s.creativePrice(ctx, group, model, "4K"),
			})
		}
	}
	return out, nil
}

// ListCreativeModelCandidates 返回管理端配置创作台白名单时可选择的当前模型。
// 候选不按用户权限过滤，但仍严格复用创作台的分组、账号和平台模型解析逻辑。
func (s *CreativePublicService) ListCreativeModelCandidates(ctx context.Context) ([]CreativeModelCandidate, error) {
	if s == nil || s.GroupRepo == nil {
		return nil, errors.New("creative group repository is not configured")
	}
	groups, err := s.GroupRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CreativeModelCandidate, 0)
	for i := range groups {
		group := &groups[i]
		if !group.IsActive() || !group.AllowImageGeneration {
			continue
		}
		operations := creativeOperationsForPlatform(group.Platform)
		if len(operations) == 0 {
			continue
		}
		models, err := s.creativeModelsForGroup(ctx, group)
		if err != nil {
			return nil, err
		}
		modelNames := make([]string, 0, len(models))
		for model := range models {
			if len(creativeImageSizesForGroupModel(group, model)) == 0 {
				continue
			}
			modelNames = append(modelNames, model)
		}
		sort.Strings(modelNames)
		for _, model := range modelNames {
			out = append(out, CreativeModelCandidate{
				GroupID:    group.ID,
				GroupName:  group.Name,
				Platform:   group.Platform,
				Model:      model,
				Operations: append([]string(nil), operations...),
			})
		}
	}
	return out, nil
}

func (s *CreativePublicService) creativeModelSettings(ctx context.Context) []CreativeModelSetting {
	if s == nil || s.Settings == nil {
		return []CreativeModelSetting{}
	}
	return s.Settings.GetCreativeModelSettings(ctx)
}

// creativePrice 返回指定尺寸的展示单价：统一定价优先，并乘模型广场使用的分组图片倍率。
func (s *CreativePublicService) creativePrice(ctx context.Context, group *Group, model, imageSize string) float64 {
	if unitPrice, ok := s.creativeResolvedImageUnitPrice(ctx, group, model, imageSize); ok {
		return unitPrice * marketplaceImageRateMultiplier(group)
	}
	if group != nil {
		if price := group.GetImagePrice(imageSize); price != nil {
			return *price * marketplaceImageRateMultiplier(group)
		}
	}
	if s.Pricing == nil {
		return 0
	}
	return s.Pricing.CalculateImageCost(model, imageSize, 1, nil, 1).TotalCost * marketplaceImageRateMultiplier(group)
}

// creativeResolvedImageUnitPrice 从统一定价解析器读取渠道或分组的图片单价。
func (s *CreativePublicService) creativeResolvedImageUnitPrice(ctx context.Context, group *Group, model, imageSize string) (float64, bool) {
	if s == nil || s.PricingResolver == nil || group == nil {
		return 0, false
	}
	groupID := group.ID
	resolved := s.PricingResolver.Resolve(ctx, PricingInput{Model: model, GroupID: &groupID, Group: group})
	if resolved == nil || (resolved.Mode != BillingModeImage && resolved.Mode != BillingModePerRequest) {
		return 0, false
	}
	if price, ok := s.PricingResolver.GetRequestTierPriceValue(resolved, imageSize); ok {
		return price, true
	}
	if resolved.DefaultPerRequestPrice > 0 || (resolved.channelPricing != nil && resolved.channelPricing.PerRequestPrice != nil) {
		return resolved.DefaultPerRequestPrice, true
	}
	return 0, false
}

// creativeOperationsForPlatform 返回分组平台支持的操作集合。
// OpenAI 保留 mask inpaint；Gemini 使用普通参考图 edit；Grok 使用 xAI 图片编辑端点。
func creativeOperationsForPlatform(platform string) []string {
	switch strings.TrimSpace(platform) {
	case PlatformOpenAI:
		return []string{CreativeOperationGenerate, CreativeOperationEdit, CreativeOperationInpaint}
	case PlatformGemini, PlatformGrok:
		return []string{CreativeOperationGenerate, CreativeOperationEdit}
	default:
		return nil
	}
}

// creativeImageSizesForGroup 返回分组显式配置了价格的尺寸列表。
func creativeImageSizesForGroup(group *Group) []string {
	if group == nil {
		return nil
	}
	sizes := make([]string, 0, 3)
	if group.ImagePrice1K != nil {
		sizes = append(sizes, "1K")
	}
	if group.ImagePrice2K != nil {
		sizes = append(sizes, "2K")
	}
	if group.ImagePrice4K != nil {
		sizes = append(sizes, "4K")
	}
	return sizes
}

// creativeDefaultImageSizesForPlatform 返回分组未配置图片价时的平台默认尺寸档位，
// 与网关按默认价计费的口径一致：OpenAI GPT Image 2 支持 1K/2K/4K 三档，
// grok 支持 1K/2K，Gemini 先按平台默认开放三档，再由模型能力过滤。
func creativeDefaultImageSizesForPlatform(platform string) []string {
	switch strings.TrimSpace(platform) {
	case PlatformOpenAI:
		return []string{"1K", "2K", "4K"}
	case PlatformGrok:
		return []string{"1K", "2K"}
	case PlatformGemini:
		return []string{"1K", "2K", "4K"}
	default:
		return nil
	}
}

// creativeQualitiesForPlatform 返回平台支持的生图画质档位（OpenAI gpt-image 系列独有）。
func creativeQualitiesForPlatform(platform string) []string {
	if strings.TrimSpace(platform) == PlatformOpenAI {
		return []string{"low", "medium", "high"}
	}
	return nil
}

// creativeImageSizesForGroupModel 返回分组内某模型可用的尺寸档位。
// 分组显式配置了图片价时按配置返回；最终结果会按已知模型能力过滤。
func creativeImageSizesForGroupModel(group *Group, model string) []string {
	if explicitSizes := creativeImageSizesForGroup(group); len(explicitSizes) > 0 {
		if group != nil && group.Platform == PlatformOpenAI && isCreativeGPTImage2Model(model) && !containsCreativeImageSize(explicitSizes, "4K") {
			// GPT Image 2 支持 4K；分组未填写 4K 价格时沿用模型默认价，不因缺少覆盖值隐藏能力。
			explicitSizes = append(explicitSizes, "4K")
		}
		return creativeFilterImageSizesForModel(group.Platform, model, explicitSizes)
	}
	if group == nil {
		return nil
	}
	sizes := creativeDefaultImageSizesForPlatform(group.Platform)
	if group.Platform == PlatformOpenAI && !isCreativeGPTImage2Model(model) {
		sizes = []string{"1K", "2K"}
	}
	return creativeFilterImageSizesForModel(group.Platform, model, sizes)
}

// creativeFilterImageSizesForModel 按已知模型能力收窄 Gemini 尺寸档位。
// 未知模型保留平台/分组配置，避免误伤供应商自定义模型；已知固定 1K 模型永远不开放高分辨率。
func creativeFilterImageSizesForModel(platform, model string, sizes []string) []string {
	if strings.TrimSpace(platform) != PlatformGemini || !isCreativeGemini1KOnlyModel(model) {
		return sizes
	}
	filtered := make([]string, 0, 1)
	for _, size := range sizes {
		if strings.EqualFold(strings.TrimSpace(size), ImageBillingSize1K) {
			filtered = append(filtered, ImageBillingSize1K)
		}
	}
	return filtered
}

// isCreativeGemini1KOnlyModel 判断官方已知只输出 1K 的 Gemini 图片模型。
func isCreativeGemini1KOnlyModel(model string) bool {
	model = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "models/")
	switch {
	case strings.HasPrefix(model, "gemini-2.5-flash-image"):
		return true
	case strings.HasPrefix(model, "gemini-3.1-flash-lite-image"):
		return true
	case model == "gemini-2.0-flash-exp-image-generation":
		return true
	default:
		return false
	}
}

func isCreativeGPTImage2Model(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-image-2")
}

func containsCreativeImageSize(sizes []string, target string) bool {
	for _, size := range sizes {
		if size == target {
			return true
		}
	}
	return false
}

// creativeModelsForGroup 从分组可调度的账号映射中收集图片模型。
func (s *CreativePublicService) creativeModelsForGroup(ctx context.Context, group *Group) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	if s.AccountRepo == nil || group == nil {
		return out, nil
	}
	accounts, err := s.AccountRepo.ListSchedulableByGroupIDAndPlatform(ctx, group.ID, group.Platform)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		account := &accounts[i]
		if !account.IsSchedulable() {
			continue
		}
		switch group.Platform {
		case PlatformGemini:
			if len(account.GetModelMapping()) == 0 {
				// 未配置映射时回退到 gemini 图片模型候选集合（与批量图片默认一致），按账号白名单过滤。
				for _, model := range defaultBatchImageModelCandidates() {
					if account.IsModelSupported(model) {
						out[model] = struct{}{}
					}
				}
				continue
			}
			// 复用批量图片的 gemini 图片模型候选展开逻辑（含 vertex）。
			for _, model := range batchImageModelsFromAccountMapping(account) {
				out[model] = struct{}{}
			}
		case PlatformOpenAI:
			for _, model := range creativeExpandAccountModels(account, defaultCreativeOpenAIModelCandidates(), IsGPTImageGenerationModel) {
				out[model] = struct{}{}
			}
		case PlatformGrok:
			for _, model := range creativeExpandAccountModels(account, defaultCreativeGrokModelCandidates(), isGrokImageGenerationModel) {
				out[model] = struct{}{}
			}
		}
	}
	return out, nil
}

// creativeExpandAccountModels 展开账号模型映射，通配符按候选集合匹配，再按谓词过滤图片模型。
// 账号未配置模型映射时等价于网关全量透传，回退到平台图片模型候选并按账号最终白名单过滤。
func creativeExpandAccountModels(account *Account, candidates []string, matches func(string) bool) []string {
	if account == nil || matches == nil {
		return nil
	}
	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		models := make(map[string]struct{})
		for _, candidate := range candidates {
			if matches(candidate) && account.IsModelSupported(candidate) {
				models[candidate] = struct{}{}
			}
		}
		out := make([]string, 0, len(models))
		for model := range models {
			out = append(out, model)
		}
		sort.Strings(out)
		return out
	}
	models := make(map[string]struct{})
	for model := range mapping {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if strings.ContainsAny(model, "*?") {
			for _, candidate := range candidates {
				if matchWildcard(model, candidate) && matches(candidate) {
					models[candidate] = struct{}{}
				}
			}
			continue
		}
		if matches(model) {
			models[model] = struct{}{}
		}
	}
	out := make([]string, 0, len(models))
	for model := range models {
		out = append(out, model)
	}
	sort.Strings(out)
	return out
}

func defaultCreativeOpenAIModelCandidates() []string {
	return []string{"gpt-image-1", "gpt-image-2"}
}

func defaultCreativeGrokModelCandidates() []string {
	return []string{"grok-imagine", "grok-imagine-edit", "grok-imagine-image-1.0", "grok-imagine-image-2.0"}
}

// ---------------------------------------------------------------------------
// 任务创建
// ---------------------------------------------------------------------------

// validatedCreativeParams 是校验通过的创建参数。
type validatedCreativeParams struct {
	group        *Group
	model        string
	operation    string
	prompt       string
	promptHash   string
	imageSize    string
	aspectRatio  string
	quality      string
	outputCount  int
	responseMIME string
	sources      []CreativeInputImage
	mask         *CreativeInputImage
	fingerprint  string
}

// CreateRun 创建创作台任务：校验 → 审核 → 幂等 → 估价 → 供应隐藏 Key → 建行 → 预占 → 暂存 → 入队。
func (s *CreativePublicService) CreateRun(ctx context.Context, userID int64, params CreateCreativeRunParamsPublic, idempotencyKey string) (*CreativeRunPublic, error) {
	if !s.enabled(ctx) {
		return nil, ErrCreativeDisabled
	}
	validated, err := s.validateCreateParams(ctx, userID, &params)
	if err != nil {
		return nil, err
	}
	if err := s.moderateCreativeRequest(ctx, userID, validated); err != nil {
		return nil, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey != "" {
		existing, err := s.Repo.GetCreativeRunByIdempotencyKey(ctx, userID, idempotencyKey)
		if err == nil {
			if existing.RequestFingerprint != validated.fingerprint {
				return nil, ErrCreativeRunIdempotencyConflict
			}
			out, err := s.getRunPublic(ctx, existing.RunID)
			if err != nil {
				return nil, err
			}
			out.IdempotentReplay = true
			return out, nil
		}
		if !errors.Is(err, ErrCreativeRunNotFound) {
			return nil, err
		}
	}
	pricing, err := s.resolveCreativePricing(ctx, userID, validated)
	if err != nil {
		return nil, err
	}
	managedKey, err := s.ensureCreativeManagedKey(ctx, userID, validated.group.ID)
	if err != nil {
		return nil, err
	}
	runID, err := NewCreativeRunID()
	if err != nil {
		return nil, err
	}
	holdAmount := pricing.EstimatedCost
	run, err := s.Repo.CreateCreativeRun(ctx, CreateCreativeRunParams{
		RunID:                      runID,
		UserID:                     userID,
		GroupID:                    validated.group.ID,
		APIKeyID:                   managedKey.ID,
		Model:                      validated.model,
		RequestedModel:             params.Model,
		Operation:                  validated.operation,
		RequestedOutputCount:       validated.outputCount,
		ImageSize:                  validated.imageSize,
		AspectRatio:                validated.aspectRatio,
		ResponseMIMEType:           validated.responseMIME,
		PromptHash:                 validated.promptHash,
		RequestFingerprint:         validated.fingerprint,
		IdempotencyKey:             creativeStringPtr(idempotencyKey),
		EstimatedCost:              pricing.EstimatedCost,
		HoldAmount:                 holdAmount,
		BaseUnitPrice:              pricing.BaseUnitPrice,
		SubscriptionRateMultiplier: pricing.SubscriptionRateMultiplier,
		BalanceRateMultiplier:      pricing.BalanceRateMultiplier,
		PlanGroupRateEnabled:       pricing.PlanGroupRateEnabled,
	})
	if err != nil {
		return nil, err
	}
	// 以下步骤按相反序回滚：释放预占 → 清理暂存 → 标记失败。
	if err := reserveCreativeBalanceHold(ctx, s.BillingRepo, run); err != nil {
		s.failRunAfterCreateError(ctx, run, "BILLING_HOLD_FAILED", err)
		return nil, err
	}
	s.invalidateCreativeAuthCache(ctx, userID)
	if err := s.saveRunTransient(ctx, run, validated); err != nil {
		_ = releaseCreativeBalanceHold(ctx, s.BillingRepo, run)
		s.invalidateCreativeAuthCache(ctx, userID)
		s.failRunAfterCreateError(ctx, run, "TRANSIENT_SAVE_FAILED", err)
		return nil, ErrCreativeTransientFailed
	}
	if s.Queue != nil {
		if err := s.Queue.Enqueue(ctx, run.RunID); err != nil && !errors.Is(err, ErrCreativeAlreadyQueued) {
			_ = releaseCreativeBalanceHold(ctx, s.BillingRepo, run)
			s.invalidateCreativeAuthCache(ctx, userID)
			_ = s.TransientStore.DeleteRunTransient(ctx, run.RunID, len(validated.sources), validated.outputCount)
			s.failRunAfterCreateError(ctx, run, "QUEUE_FAILED", err)
			return nil, err
		}
	}
	out, err := s.getRunPublic(ctx, run.RunID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// failRunAfterCreateError 把创建失败的任务标记为 failed；失败路径允许从 queued 直接转换。
func (s *CreativePublicService) failRunAfterCreateError(ctx context.Context, run *CreativeRun, code string, cause error) {
	message := sanitizeCreativeMessage(cause.Error())
	if err := s.Repo.TransitionCreativeRunStatus(ctx, run.RunID, CreativeRunStatusFailed, CreativeRunTransitionOptions{
		ErrorCode:    &code,
		ErrorMessage: &message,
	}); err != nil {
		logger.L().Warn("creative.create_failure_mark_failed",
			zap.String("run_id", run.RunID),
			zap.Error(err),
		)
	}
}

// saveRunTransient 把任务载荷与输入字节写入临时 Redis 存储。
func (s *CreativePublicService) saveRunTransient(ctx context.Context, run *CreativeRun, validated *validatedCreativeParams) error {
	if s.TransientStore == nil {
		return errors.New("creative transient store is not configured")
	}
	payload := &CreativeRunPayload{
		RunID:              run.RunID,
		UserID:             run.UserID,
		GroupID:            run.GroupID,
		APIKeyID:           run.APIKeyID,
		Model:              run.Model,
		Operation:          run.Operation,
		Prompt:             validated.prompt,
		ImageSize:          run.ImageSize,
		AspectRatio:        run.AspectRatio,
		ResponseMIMEType:   run.ResponseMIMEType,
		Quality:            validated.quality,
		SourceCount:        len(validated.sources),
		HasMask:            validated.mask != nil,
		RequestFingerprint: run.RequestFingerprint,
	}
	if err := s.TransientStore.SavePayload(ctx, run.RunID, payload); err != nil {
		return err
	}
	for i := range validated.sources {
		if err := s.TransientStore.SaveInput(ctx, run.RunID, i, validated.sources[i].Bytes); err != nil {
			return err
		}
	}
	if validated.mask != nil {
		if err := s.TransientStore.SaveMask(ctx, run.RunID, validated.mask.Bytes); err != nil {
			return err
		}
	}
	return nil
}

// validateCreateParams 执行全部服务端校验，返回规范化后的参数与请求指纹。
func (s *CreativePublicService) validateCreateParams(ctx context.Context, userID int64, params *CreateCreativeRunParamsPublic) (*validatedCreativeParams, error) {
	if params == nil {
		return nil, ErrCreativeInvalidParams
	}
	user, err := s.UserRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, ErrUserNotFound
	}
	group, err := s.GroupRepo.GetByIDLite(ctx, params.GroupID)
	if err != nil || group == nil || !group.IsActive() {
		return nil, ErrCreativeGroupForbidden
	}
	if !user.CanBindGroup(group.ID, group.IsExclusive) {
		return nil, ErrCreativeGroupForbidden
	}
	if !group.AllowImageGeneration {
		return nil, ErrCreativeGroupImageDisabled
	}
	operations := creativeOperationsForPlatform(group.Platform)
	if len(operations) == 0 {
		return nil, ErrCreativeGroupImageDisabled
	}
	model := strings.TrimSpace(params.Model)
	modelSettings := creativeModelSettingsIndex(s.creativeModelSettings(ctx))
	configuredOperations, configured := creativeOperationsForModel(modelSettings, group.ID, model, operations)
	if !configured {
		return nil, ErrCreativeInvalidModel
	}
	operations = configuredOperations
	if len(operations) == 0 {
		return nil, ErrCreativeOperationUnsupported
	}
	models, err := s.creativeModelsForGroup(ctx, group)
	if err != nil {
		return nil, err
	}
	if _, ok := models[model]; !ok {
		return nil, ErrCreativeInvalidModel
	}
	operation := strings.TrimSpace(params.Operation)
	operationAllowed := false
	for _, candidate := range operations {
		if candidate == operation {
			operationAllowed = true
			break
		}
	}
	if !operationAllowed {
		return nil, ErrCreativeOperationUnsupported
	}
	if strings.TrimSpace(group.Platform) == PlatformGrok && operation == CreativeOperationEdit && len(params.SourceImages) > grokMediaMaxEditSourceImages {
		return nil, ErrCreativeInvalidParams
	}

	prompt := strings.TrimSpace(params.Prompt)
	if prompt == "" {
		return nil, ErrCreativeInvalidParams
	}
	if len(prompt) > s.maxPromptChars() {
		return nil, ErrCreativePromptTooLong
	}

	// 创作台一次提交固定生成一张，多张图片通过重复提交任务获取。
	outputCount := 1

	imageSize := strings.TrimSpace(params.ImageSize)
	if imageSize == "" {
		imageSize = s.defaultImageSize()
	}
	switch strings.ToUpper(imageSize) {
	case "1K", "2K", "4K":
		imageSize = strings.ToUpper(imageSize)
	default:
		return nil, ErrCreativeInvalidParams
	}
	aspectRatio := strings.TrimSpace(params.AspectRatio)
	if len(aspectRatio) > 16 {
		return nil, ErrCreativeInvalidParams
	}
	// 画质档位仅 OpenAI 平台（gpt-image 系列）支持；其余平台传入视为非法参数。
	quality := strings.ToLower(strings.TrimSpace(params.Quality))
	if quality != "" {
		switch quality {
		case "low", "medium", "high", "auto":
		default:
			return nil, ErrCreativeInvalidParams
		}
		if strings.TrimSpace(group.Platform) != PlatformOpenAI {
			return nil, ErrCreativeInvalidParams
		}
	}
	responseMIME := strings.TrimSpace(params.ResponseMIME)
	if responseMIME == "" {
		responseMIME = s.defaultResponseMimeType()
	}
	switch strings.ToLower(responseMIME) {
	case "image/png", "image/jpeg", "image/webp":
		responseMIME = strings.ToLower(responseMIME)
	default:
		return nil, ErrCreativeInvalidMime
	}

	sources := make([]CreativeInputImage, 0, len(params.SourceImages))
	totalBytes := 0
	for i := range params.SourceImages {
		source, err := normalizeCreativeImageInput(params.SourceImages[i], s.maxAssetBytes())
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
		totalBytes += len(source.Bytes)
	}
	var mask *CreativeInputImage
	if params.Mask != nil && len(params.Mask.Bytes) > 0 {
		normalized, err := normalizeCreativeImageInput(*params.Mask, s.maxAssetBytes())
		if err != nil {
			return nil, err
		}
		if normalized.Mime != "image/png" {
			return nil, ErrCreativeMaskRequired
		}
		mask = &normalized
		totalBytes += len(normalized.Bytes)
	}
	if totalBytes > 0 && int64(totalBytes) > s.maxTotalInputBytes() {
		return nil, ErrCreativeInputTooLarge
	}

	switch operation {
	case CreativeOperationEdit:
		if len(sources) == 0 {
			return nil, ErrCreativeInvalidParams
		}
	case CreativeOperationInpaint:
		if len(sources) == 0 || mask == nil {
			return nil, ErrCreativeMaskRequired
		}
		maskWidth, maskHeight, err := creativeImageDimensions(mask.Bytes, mask.Mime)
		if err != nil {
			return nil, ErrCreativeMaskRequired
		}
		sourceWidth, sourceHeight, err := creativeImageDimensions(sources[0].Bytes, sources[0].Mime)
		if err != nil {
			return nil, ErrCreativeInvalidMime
		}
		if maskWidth != sourceWidth || maskHeight != sourceHeight {
			return nil, ErrCreativeMaskSizeMismatch
		}
	}
	if mask != nil && operation != CreativeOperationInpaint {
		return nil, ErrCreativeInvalidParams
	}

	promptHash := sha256Hex([]byte(prompt))
	fingerprint := buildCreativeRequestFingerprint(creativeFingerprintPayload{
		GroupID:          group.ID,
		Model:            model,
		Operation:        operation,
		PromptSHA256:     promptHash,
		ImageSHA256:      creativeImageHashes(sources),
		MaskSHA256:       creativeImageHash(mask),
		ImageSize:        imageSize,
		AspectRatio:      aspectRatio,
		Quality:          quality,
		ResponseMIMEType: responseMIME,
	})
	return &validatedCreativeParams{
		group:        group,
		model:        model,
		operation:    operation,
		prompt:       prompt,
		promptHash:   promptHash,
		imageSize:    imageSize,
		aspectRatio:  aspectRatio,
		quality:      quality,
		outputCount:  outputCount,
		responseMIME: responseMIME,
		sources:      sources,
		mask:         mask,
		fingerprint:  fingerprint,
	}, nil
}

// normalizeCreativeImageInput 校验单个上传文件：非空、大小上限、MIME 归一化。
func normalizeCreativeImageInput(input CreativeInputImage, maxBytes int64) (CreativeInputImage, error) {
	if len(input.Bytes) == 0 {
		return input, ErrCreativeInvalidMime
	}
	if int64(len(input.Bytes)) > maxBytes {
		return input, ErrCreativeAssetTooLarge
	}
	mime := strings.ToLower(strings.TrimSpace(input.Mime))
	switch mime {
	case "image/png", "image/jpeg", "image/jpg", "image/webp":
	default:
		// 客户端未传 MIME 时按字节嗅探常见格式。
		mime = sniffCreativeImageMime(input.Bytes)
		if mime == "" {
			return input, ErrCreativeInvalidMime
		}
	}
	if mime == "image/jpg" {
		mime = "image/jpeg"
	}
	return CreativeInputImage{Bytes: input.Bytes, Mime: mime}, nil
}

// sniffCreativeImageMime 按魔数识别 PNG/JPEG/WebP。
func sniffCreativeImageMime(data []byte) string {
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return "image/png"
	}
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	return ""
}

// creativeImageDimensions 解析图片尺寸；webp 使用 x/image/webp 解码器。
func creativeImageDimensions(data []byte, mime string) (int, int, error) {
	if mime == "image/webp" {
		cfg, err := webp.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return 0, 0, err
		}
		return cfg.Width, cfg.Height, nil
	}
	switch mime {
	case "image/png", "image/jpeg":
		// image/jpeg 与 image/png 的解码器已通过 side-effect import 注册。
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return 0, 0, err
		}
		return cfg.Width, cfg.Height, nil
	default:
		return 0, 0, fmt.Errorf("unsupported image mime %s", mime)
	}
}

// creativeFingerprintPayload 是请求指纹的 canonical JSON 载体（字段顺序固定）。
type creativeFingerprintPayload struct {
	GroupID          int64    `json:"group_id"`
	Model            string   `json:"model"`
	Operation        string   `json:"operation"`
	PromptSHA256     string   `json:"prompt_sha256"`
	ImageSHA256      []string `json:"image_sha256"`
	MaskSHA256       string   `json:"mask_sha256,omitempty"`
	ImageSize        string   `json:"image_size"`
	AspectRatio      string   `json:"aspect_ratio"`
	Quality          string   `json:"quality,omitempty"`
	ResponseMIMEType string   `json:"response_mime_type"`
}

// buildCreativeRequestFingerprint 计算幂等指纹：canonical JSON 的 sha256。
func buildCreativeRequestFingerprint(payload creativeFingerprintPayload) string {
	body, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return sha256Hex(body)
}

func creativeImageHashes(sources []CreativeInputImage) []string {
	hashes := make([]string, 0, len(sources))
	for i := range sources {
		hashes = append(hashes, sha256Hex(sources[i].Bytes))
	}
	return hashes
}

func creativeImageHash(image *CreativeInputImage) string {
	if image == nil {
		return ""
	}
	return sha256Hex(image.Bytes)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// moderateCreativeRequest 对 prompt 与图片构造 OpenAI Images 协议报文送审。
// 必须开启 NoMediaRetention：审核系统不得留存媒体快照与正文摘录。
func (s *CreativePublicService) moderateCreativeRequest(ctx context.Context, userID int64, validated *validatedCreativeParams) error {
	if s.Moderation == nil {
		return nil
	}
	images := make([]map[string]string, 0, len(validated.sources)+1)
	for i := range validated.sources {
		images = append(images, map[string]string{
			"image_url": "data:" + validated.sources[i].Mime + ";base64," + base64.StdEncoding.EncodeToString(validated.sources[i].Bytes),
		})
	}
	if validated.mask != nil {
		images = append(images, map[string]string{
			"image_url": "data:" + validated.mask.Mime + ";base64," + base64.StdEncoding.EncodeToString(validated.mask.Bytes),
		})
	}
	payload := map[string]any{
		"model":  validated.model,
		"prompt": validated.prompt,
		"images": images,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	requestID := "creative_mod:" + validated.fingerprint
	if clientRequestID, ok := ctx.Value(ctxkey.RequestID).(string); ok && strings.TrimSpace(clientRequestID) != "" {
		requestID = "creative_mod:" + strings.TrimSpace(clientRequestID)
	}
	decision, err := s.Moderation.Check(ctx, ContentModerationCheckInput{
		RequestID:        requestID,
		UserID:           userID,
		BillingUserID:    userID,
		GroupID:          &validated.group.ID,
		GroupName:        validated.group.Name,
		Endpoint:         "/v1/creative/runs",
		Provider:         validated.group.Platform,
		Model:            validated.model,
		Protocol:         ContentModerationProtocolOpenAIImages,
		Body:             body,
		NoMediaRetention: true,
	})
	if err != nil {
		// 审核系统自身失败不阻断创作台（审核服务本身 fail-open）。
		logger.L().Warn("creative.moderation_check_failed",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil
	}
	if decision != nil && !decision.Allowed {
		return ErrCreativeContentBlocked
	}
	return nil
}

// CreativePricingSnapshot 是任务创建时的定价快照。
type CreativePricingSnapshot struct {
	BaseUnitPrice              float64
	SubscriptionRateMultiplier float64
	BalanceRateMultiplier      float64
	PlanGroupRateEnabled       bool
	EstimatedCost              float64
}

// resolveCreativePricing 计算基础单价与有效倍率（订阅倍率 + 用户倍率），与批量图片同口径。
func (s *CreativePublicService) resolveCreativePricing(ctx context.Context, userID int64, validated *validatedCreativeParams) (*CreativePricingSnapshot, error) {
	group := validated.group
	groupDefault := group.RateMultiplier
	if groupDefault < 0 {
		groupDefault = 0
	}
	subscriptionRate := groupDefault
	balanceRate := groupDefault
	if s.UserGroupRateRepo != nil {
		if userRate, err := s.UserGroupRateRepo.GetByUserAndGroup(ctx, userID, group.ID); err == nil && userRate != nil {
			balanceRate = *userRate
		}
	}
	effective := balanceRate
	planGroupRateEnabled := true
	groupID := group.ID
	if subscription := resolveUsageSubscription(ctx, nil, nil, usageSubscriptionResolverFrom(s.BillingRepo), userID, &groupID); subscription != nil {
		effective = resolveUsageRateMultiplier(ctx, userID, &groupID, group, groupDefault, subscription, nil)
	}
	if group.ImageRateIndependent {
		effective = group.ImageRateMultiplier
		subscriptionRate = group.ImageRateMultiplier
		balanceRate = group.ImageRateMultiplier
		planGroupRateEnabled = false
	}
	imagePriceConfig := &ImagePriceConfig{
		Price1K: group.ImagePrice1K,
		Price2K: group.ImagePrice2K,
		Price4K: group.ImagePrice4K,
	}
	baseUnitPrice := 0.0
	estimatedCost := 0.0
	if resolvedUnitPrice, ok := s.creativeResolvedImageUnitPrice(ctx, group, validated.model, validated.imageSize); ok {
		baseUnitPrice = resolvedUnitPrice
		estimatedCost = resolvedUnitPrice * float64(validated.outputCount) * effective
	} else if unit := group.GetImagePrice(validated.imageSize); unit != nil && *unit >= 0 {
		baseUnitPrice = *unit
		estimatedCost = baseUnitPrice * float64(validated.outputCount) * effective
	} else if s.Pricing != nil {
		breakdown := s.Pricing.CalculateImageCost(validated.model, validated.imageSize, validated.outputCount, imagePriceConfig, effective)
		estimatedCost = breakdown.ActualCost
		if validated.outputCount > 0 {
			baseUnitPrice = breakdown.TotalCost / float64(validated.outputCount)
		}
	} else {
		estimatedCost = baseUnitPrice * float64(validated.outputCount) * effective
	}
	return &CreativePricingSnapshot{
		BaseUnitPrice:              baseUnitPrice,
		SubscriptionRateMultiplier: subscriptionRate,
		BalanceRateMultiplier:      balanceRate,
		PlanGroupRateEnabled:       planGroupRateEnabled,
		EstimatedCost:              estimatedCost,
	}, nil
}

// ensureCreativeManagedKey 幂等供应某用户 + 分组的创作台隐藏执行 Key。
func (s *CreativePublicService) ensureCreativeManagedKey(ctx context.Context, userID, groupID int64) (*APIKey, error) {
	if s.ApiKeyRepo == nil {
		return nil, errors.New("creative managed key repository is not configured")
	}
	existing, err := s.ApiKeyRepo.GetManagedKeyByUserAndGroup(ctx, userID, groupID, CreativeManagedBy)
	if err == nil && existing != nil {
		return existing, nil
	}
	if err != nil && !errors.Is(err, ErrAPIKeyNotFound) {
		return nil, err
	}
	prefix := "sk-"
	if s.Config != nil && strings.TrimSpace(s.Config.Default.APIKeyPrefix) != "" {
		prefix = strings.TrimSpace(s.Config.Default.APIKeyPrefix)
	}
	keyString, err := GenerateAPIKeyString(prefix)
	if err != nil {
		return nil, err
	}
	managedBy := CreativeManagedBy
	key := &APIKey{
		UserID:                                userID,
		Key:                                   keyString,
		Name:                                  fmt.Sprintf("creative-studio:%d", groupID),
		GroupID:                               &groupID,
		Status:                                StatusActive,
		BillingMode:                           APIKeyBillingModeAuto,
		ManagedBy:                             &managedBy,
		FastModePolicy:                        "follow_request",
		FallbackToDefaultGroupWhenUnavailable: false,
	}
	if err := s.ApiKeyRepo.CreateManagedKey(ctx, key); err != nil {
		// 并发创建冲突：重查一次即可拿到已存在的 Key（创建幂等）。
		if existing, retryErr := s.ApiKeyRepo.GetManagedKeyByUserAndGroup(ctx, userID, groupID, CreativeManagedBy); retryErr == nil && existing != nil {
			return existing, nil
		}
		return nil, err
	}
	return key, nil
}

// ---------------------------------------------------------------------------
// 查询 / 输出
// ---------------------------------------------------------------------------

func (s *CreativePublicService) getRunPublic(ctx context.Context, runID string) (*CreativeRunPublic, error) {
	run, err := s.Repo.GetCreativeRunByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	outputs, err := s.Repo.ListCreativeRunOutputs(ctx, runID)
	if err != nil {
		return nil, err
	}
	return CreativeRunToPublic(run, outputs), nil
}

// GetRun 返回单个任务（含输出元数据），校验所有权。
func (s *CreativePublicService) GetRun(ctx context.Context, userID int64, runID string) (*CreativeRunPublic, error) {
	if !s.enabled(ctx) {
		return nil, ErrCreativeDisabled
	}
	run, err := s.Repo.GetCreativeRunByRunIDForOwner(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	outputs, err := s.Repo.ListCreativeRunOutputs(ctx, run.RunID)
	if err != nil {
		return nil, err
	}
	return CreativeRunToPublic(run, outputs), nil
}

// ListRuns 返回当前用户的任务列表（created_at desc 分页）。
func (s *CreativePublicService) ListRuns(ctx context.Context, userID int64, filter CreativeRunFilter) (*CreativeListRunsResponse, error) {
	if !s.enabled(ctx) {
		return nil, ErrCreativeDisabled
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	runs, err := s.Repo.ListCreativeRunsForOwner(ctx, userID, filter)
	if err != nil {
		return nil, err
	}
	data := make([]*CreativeRunPublic, 0, len(runs))
	for _, run := range runs {
		// 历史列表同样需要输出元数据，否则前端无法关联本地素材与缺失占位。
		outputs, err := s.Repo.ListCreativeRunOutputs(ctx, run.RunID)
		if err != nil {
			return nil, err
		}
		data = append(data, CreativeRunToPublic(run, outputs))
	}
	return &CreativeListRunsResponse{Data: data, HasMore: len(data) == filter.Limit}, nil
}

// CreativeOutputContent 是输出内容的返回结构。
type CreativeOutputContent struct {
	Content     []byte
	ContentType string
}

// GetOutputContent 校验所有权与输出状态后从临时存储读取图片字节。
// 过期或缺失时：任务为 succeeded 则转 result_lost，并返回明确错误，绝不明示成功。
func (s *CreativePublicService) GetOutputContent(ctx context.Context, userID int64, runID string, outputIndex int) (*CreativeOutputContent, error) {
	if !s.enabled(ctx) {
		return nil, ErrCreativeDisabled
	}
	run, err := s.Repo.GetCreativeRunByRunIDForOwner(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	output, err := s.Repo.GetCreativeRunOutput(ctx, runID, outputIndex)
	if err != nil {
		return nil, err
	}
	switch output.Status {
	case CreativeRunOutputStatusSucceeded:
	case CreativeRunOutputStatusAcked:
		return nil, ErrCreativeOutputExpired
	case CreativeRunOutputStatusLost:
		return nil, ErrCreativeResultLost
	case CreativeRunOutputStatusFailed:
		return nil, ErrCreativeOutputNotReady
	default:
		return nil, ErrCreativeOutputNotReady
	}
	now := time.Now()
	if output.TransientExpiresAt != nil && now.After(*output.TransientExpiresAt) {
		s.markRunResultLostBestEffort(ctx, run)
		return nil, ErrCreativeOutputExpired
	}
	if s.TransientStore == nil {
		return nil, ErrCreativeTransientFailed
	}
	data, err := s.TransientStore.LoadOutput(ctx, runID, outputIndex)
	if err != nil {
		// 临时输出已被 TTL 清理或丢失：成功任务转为 result_lost。
		s.markRunResultLostBestEffort(ctx, run)
		return nil, ErrCreativeResultLost
	}
	contentType := "application/octet-stream"
	if output.MimeType != nil && strings.TrimSpace(*output.MimeType) != "" {
		contentType = strings.TrimSpace(*output.MimeType)
	}
	return &CreativeOutputContent{Content: data, ContentType: contentType}, nil
}

// markRunResultLostBestEffort 把 succeeded 任务降级为 result_lost；失败只记日志。
func (s *CreativePublicService) markRunResultLostBestEffort(ctx context.Context, run *CreativeRun) {
	if run == nil || run.Status != CreativeRunStatusSucceeded {
		return
	}
	if err := s.Repo.TransitionCreativeRunStatus(ctx, run.RunID, CreativeRunStatusResultLost, CreativeRunTransitionOptions{
		ErrorCode:    creativeStringPtr("RESULT_EXPIRED"),
		ErrorMessage: creativeStringPtr("transient output expired before client acknowledgment"),
	}); err != nil {
		logger.L().Warn("creative.mark_result_lost_failed",
			zap.String("run_id", run.RunID),
			zap.Error(err),
		)
	}
}

// AckOutput 在客户端确认保存后删除临时输出并标记 acked，幂等。
func (s *CreativePublicService) AckOutput(ctx context.Context, userID int64, runID string, outputIndex int) error {
	if !s.enabled(ctx) {
		return ErrCreativeDisabled
	}
	run, err := s.Repo.GetCreativeRunByRunIDForOwner(ctx, userID, runID)
	if err != nil {
		return err
	}
	_ = run
	output, err := s.Repo.GetCreativeRunOutput(ctx, runID, outputIndex)
	if err != nil {
		return err
	}
	if output.Status == CreativeRunOutputStatusAcked {
		// 重复 ack 视为成功（幂等）。
		if s.TransientStore != nil {
			_ = s.TransientStore.DeleteOutput(ctx, runID, outputIndex)
		}
		return nil
	}
	if output.Status != CreativeRunOutputStatusSucceeded {
		return ErrCreativeOutputNotReady
	}
	if s.TransientStore != nil {
		if err := s.TransientStore.DeleteOutput(ctx, runID, outputIndex); err != nil {
			logger.L().Warn("creative.ack_delete_output_failed",
				zap.String("run_id", runID),
				zap.Int("output_index", outputIndex),
				zap.Error(err),
			)
		}
	}
	return s.Repo.MarkCreativeRunOutputAcked(ctx, runID, outputIndex, time.Now())
}

func (s *CreativePublicService) invalidateCreativeAuthCache(ctx context.Context, userID int64) {
	if s != nil && s.AuthCache != nil && userID > 0 {
		s.AuthCache.InvalidateAuthCacheByUserID(ctx, userID)
	}
}

// ---------------------------------------------------------------------------
// worker 面向的结算方法（第二阶段 worker runtime 调用，本阶段实现并保证幂等）
// ---------------------------------------------------------------------------

// MarkRunning 把任务从 queued 推进到 running 并回填账号；重复调用幂等。
func (s *CreativePublicService) MarkRunning(ctx context.Context, runID string, accountID int64) error {
	if s == nil || s.Repo == nil {
		return errors.New("creative service is not configured")
	}
	run, err := s.Repo.GetCreativeRunByRunID(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status == CreativeRunStatusRunning {
		return nil
	}
	if run.Status != CreativeRunStatusQueued {
		if IsTerminalCreativeRunStatus(run.Status) {
			return nil
		}
		return ErrCreativeInvalidTransition
	}
	return s.Repo.MarkCreativeRunRunning(ctx, runID, accountID, time.Now())
}

// CreativeOutputResult 是 worker 上报的单个输出结果。
type CreativeOutputResult struct {
	Index        int
	Success      bool
	Bytes        []byte
	Mime         string
	ErrorCode    string
	ErrorMessage string
}

// SucceedRun 结算成功路径：保存临时输出 → 捕获实际费用 → 写 usage_logs → 终态 succeeded。
// 幂等：重复调用或任务已终态时直接返回，不重复扣费。
// 任务在执行期间进入 cancelled 时：仍按 provider 实际成功结果捕获并记录用量，
// 但保留 cancelled 终态，不覆盖为 succeeded。
func (s *CreativePublicService) SucceedRun(ctx context.Context, runID string, accountID int64, results []CreativeOutputResult) (*CreativeRunPublic, error) {
	if s == nil || s.Repo == nil {
		return nil, errors.New("creative service is not configured")
	}
	run, err := s.Repo.GetCreativeRunByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}
	keepCancelled := run.Status == CreativeRunStatusCancelled
	if IsTerminalCreativeRunStatus(run.Status) && !keepCancelled {
		// 已结算/已失败/已丢失：返回当前快照，不重复计费。
		return s.getRunPublic(ctx, runID)
	}
	if accountID > 0 {
		if run.AccountID == nil || *run.AccountID != accountID {
			if err := s.Repo.SetCreativeRunAccountID(ctx, runID, accountID, time.Now()); err != nil {
				return nil, err
			}
		}
		run.AccountID = &accountID
	}
	transientTTL := s.transientTTL()
	now := time.Now()
	expiresAt := now.Add(transientTTL)
	successCount := 0
	for i := range results {
		result := &results[i]
		if result.Success {
			// 成功状态必须建立在输出已写入 transient store 的前提上，
			// 否则客户端会收到 succeeded 却永远无法取回图片的假成功。
			if s.TransientStore == nil {
				return nil, fmt.Errorf("%w: transient store is not configured", ErrCreativeTransientFailed)
			}
			if len(result.Bytes) == 0 {
				return nil, fmt.Errorf("%w: creative output %d is empty", ErrCreativeTransientFailed, result.Index)
			}
			if err := s.TransientStore.SaveOutput(ctx, runID, result.Index, result.Bytes, transientTTL); err != nil {
				logger.L().Warn("creative.save_output_failed",
					zap.String("run_id", runID),
					zap.Int("output_index", result.Index),
					zap.Error(err),
				)
				return nil, fmt.Errorf("%w: save output %d: %v", ErrCreativeTransientFailed, result.Index, err)
			}
			successCount++
		}
	}
	// 所有成功输出都已写入 transient 后再更新数据库状态，避免多输出任务部分成功。
	for i := range results {
		result := &results[i]
		if result.Success {
			if err := s.Repo.UpdateCreativeRunOutput(ctx, runID, result.Index, CreativeRunOutputStatusSucceeded, result.Mime, int64(len(result.Bytes)), &expiresAt, "", ""); err != nil {
				return nil, err
			}
			continue
		}
		if err := s.Repo.UpdateCreativeRunOutput(ctx, runID, result.Index, CreativeRunOutputStatusFailed, "", 0, nil, result.ErrorCode, sanitizeCreativeMessage(result.ErrorMessage)); err != nil {
			return nil, err
		}
	}
	if run.AccountID == nil || *run.AccountID <= 0 {
		// 执行账号是 usage_logs 的必填上下文；缺失时保持未结算而不是写脏数据。
		return nil, ErrBatchImageSettlementMissingAccountID
	}
	billingResult, err := captureCreativeBalanceHold(ctx, s.BillingRepo, run, successCount)
	if err != nil {
		return nil, err
	}
	s.invalidateCreativeAuthCache(ctx, run.UserID)
	actualCost := 0.0
	if run.ActualCost != nil {
		actualCost = *run.ActualCost
	}
	if keepCancelled {
		// 任务在执行期间进入 cancelled：资金已按实际成功结果捕获、用量已记录，
		// 但任务终态保持 cancelled，绝不回写为 succeeded。
		return s.getRunPublic(ctx, runID)
	}
	if err := s.Repo.MarkCreativeRunSucceeded(ctx, runID, actualCost, now); err != nil {
		if errors.Is(err, ErrCreativeInvalidTransition) {
			// 并发取消/失败已把任务推进到终态：资金已捕获，保持计费，不释放。
			return s.getRunPublic(ctx, runID)
		}
		return nil, err
	}
	s.recordCreativeUsageLog(ctx, run, actualCost, successCount, billingResult, now)
	return s.getRunPublic(ctx, runID)
}

// FailRun 失败路径：记录错误 + 释放预占 + 清理暂存；终态任务幂等返回。
func (s *CreativePublicService) FailRun(ctx context.Context, runID, errorCode, errorMessage string) error {
	if s == nil || s.Repo == nil {
		return nil
	}
	run, err := s.Repo.GetCreativeRunByRunID(ctx, runID)
	if err != nil {
		return err
	}
	if IsTerminalCreativeRunStatus(run.Status) {
		return nil
	}
	code := strings.TrimSpace(errorCode)
	if code == "" {
		code = "PROVIDER_FAILED"
	}
	message := sanitizeCreativeMessage(errorMessage)
	if err := s.Repo.TransitionCreativeRunStatus(ctx, runID, CreativeRunStatusFailed, CreativeRunTransitionOptions{
		ErrorCode:    &code,
		ErrorMessage: &message,
	}); err != nil && !errors.Is(err, ErrCreativeInvalidTransition) {
		return err
	}
	if err := releaseCreativeBalanceHold(ctx, s.BillingRepo, run); err != nil {
		logger.L().Warn("creative.fail_release_failed",
			zap.String("run_id", runID),
			zap.Error(err),
		)
		return err
	}
	s.invalidateCreativeAuthCache(ctx, run.UserID)
	if s.TransientStore != nil {
		if err := s.TransientStore.DeleteRunTransient(ctx, runID, 0, run.RequestedOutputCount); err != nil {
			logger.L().Warn("creative.fail_transient_cleanup_failed",
				zap.String("run_id", runID),
				zap.Error(err),
			)
		}
	}
	return nil
}

// CancelRunByWorker 是 worker 侧处理既有 cancelled 状态的入口。
func (s *CreativePublicService) CancelRunByWorker(ctx context.Context, runID string) error {
	if s == nil || s.Repo == nil {
		return nil
	}
	run, err := s.Repo.GetCreativeRunByRunID(ctx, runID)
	if err != nil {
		return err
	}
	if IsTerminalCreativeRunStatus(run.Status) {
		return nil
	}
	if err := s.Repo.TransitionCreativeRunStatus(ctx, runID, CreativeRunStatusCancelled, CreativeRunTransitionOptions{}); err != nil && !errors.Is(err, ErrCreativeInvalidTransition) {
		return err
	}
	if err := releaseCreativeBalanceHold(ctx, s.BillingRepo, run); err != nil {
		return err
	}
	s.invalidateCreativeAuthCache(ctx, run.UserID)
	if s.TransientStore != nil {
		_ = s.TransientStore.DeleteRunTransient(ctx, runID, 0, run.RequestedOutputCount)
	}
	return nil
}

// MarkResultLost 把任务标记为 result_lost。
// providerSucceeded 为 true（上游已确认成功且已捕获）时保持计费，否则释放预占。
func (s *CreativePublicService) MarkResultLost(ctx context.Context, runID string, providerSucceeded bool) error {
	if s == nil || s.Repo == nil {
		return nil
	}
	run, err := s.Repo.GetCreativeRunByRunID(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status == CreativeRunStatusResultLost {
		return nil
	}
	if IsTerminalCreativeRunStatus(run.Status) {
		return nil
	}
	code := "RESULT_LOST"
	message := "transient result expired or worker lost before client acknowledgment"
	if err := s.Repo.TransitionCreativeRunStatus(ctx, runID, CreativeRunStatusResultLost, CreativeRunTransitionOptions{
		ErrorCode:    &code,
		ErrorMessage: &message,
	}); err != nil {
		return err
	}
	if !providerSucceeded {
		if err := releaseCreativeBalanceHold(ctx, s.BillingRepo, run); err != nil {
			return err
		}
		s.invalidateCreativeAuthCache(ctx, run.UserID)
	}
	return nil
}

// recordCreativeUsageLog 按成功输出数写图片用量日志（request_id = creative_settle:{runID}）。
func (s *CreativePublicService) recordCreativeUsageLog(ctx context.Context, run *CreativeRun, actualCost float64, successCount int, billingResult *BatchImageBalanceHoldResult, createdAt time.Time) {
	if s == nil || s.UsageLogRepo == nil || run == nil || run.AccountID == nil || successCount <= 0 {
		return
	}
	billingMode := string(BillingModeImage)
	imageSize := run.ImageSize
	inboundEndpoint := "/v1/creative/runs"
	upstreamEndpoint := "creative:" + run.Operation
	subscriptionAmount := 0.0
	balanceAmount := actualCost
	allocations := []domain.BillingAllocation(nil)
	if billingResult != nil {
		subscriptionAmount = billingResult.SubscriptionAmountUSD
		balanceAmount = billingResult.BalanceAmountUSD
		allocations = cloneBillingAllocations(billingResult.BillingAllocations)
	}
	billingType := BillingTypeBalance
	if subscriptionAmount > 0 {
		billingType = BillingTypeSubscription
	}
	if len(allocations) == 0 && actualCost > 0 {
		allocations = []domain.BillingAllocation{{Type: domain.BillingAllocationTypeBalance, AmountUSD: actualCost}}
	}
	rateMultiplier := run.BalanceRateMultiplier
	if rateMultiplier <= 0 {
		rateMultiplier = 1
	}
	usageLog := &UsageLog{
		UserID:                run.UserID,
		BillingUserID:         run.UserID,
		APIKeyID:              run.APIKeyID,
		AccountID:             *run.AccountID,
		RequestID:             CreativeSettlementRequestID(run.RunID),
		Model:                 run.Model,
		RequestedModel:        run.RequestedModel,
		InboundEndpoint:       &inboundEndpoint,
		UpstreamEndpoint:      &upstreamEndpoint,
		GroupID:               &run.GroupID,
		ImageCount:            successCount,
		ImageOutputCost:       actualCost,
		TotalCost:             actualCost,
		ActualCost:            actualCost,
		SubscriptionAmountUSD: subscriptionAmount,
		BalanceAmountUSD:      balanceAmount,
		BillingAllocations:    allocations,
		SubscriptionID:        firstAllocatedSubscriptionID(allocations),
		RateMultiplier:        rateMultiplier,
		BillingType:           billingType,
		RequestType:           RequestTypeSync,
		BillingMode:           &billingMode,
		ImageSize:             &imageSize,
		CreatedAt:             createdAt,
	}
	writeUsageLogBestEffort(ctx, s.UsageLogRepo, usageLog, "service.creative_settlement")
}

// ---------------------------------------------------------------------------
// 配置访问 helper
// ---------------------------------------------------------------------------

func (s *CreativePublicService) maxPromptChars() int {
	if s != nil && s.Config != nil && s.Config.Creative.MaxPromptChars > 0 {
		return s.Config.Creative.MaxPromptChars
	}
	return defaultCreativeMaxPromptChars
}

func (s *CreativePublicService) maxAssetBytes() int64 {
	if s != nil && s.Config != nil && s.Config.Creative.MaxAssetBytes > 0 {
		return s.Config.Creative.MaxAssetBytes
	}
	return 33554432
}

func (s *CreativePublicService) maxTotalInputBytes() int64 {
	if s != nil && s.Config != nil && s.Config.Creative.MaxTotalInputBytes > 0 {
		return s.Config.Creative.MaxTotalInputBytes
	}
	return 67108864
}

func (s *CreativePublicService) defaultResponseMimeType() string {
	if s != nil && s.Config != nil && strings.TrimSpace(s.Config.Creative.DefaultResponseMimeType) != "" {
		return strings.TrimSpace(s.Config.Creative.DefaultResponseMimeType)
	}
	return defaultCreativeResponseMime
}

func (s *CreativePublicService) defaultImageSize() string {
	if s != nil && s.Config != nil && strings.TrimSpace(s.Config.Creative.DefaultImageSize) != "" {
		return strings.TrimSpace(s.Config.Creative.DefaultImageSize)
	}
	return defaultCreativeImageSize
}

func (s *CreativePublicService) transientTTL() time.Duration {
	if s != nil && s.Config != nil && s.Config.Creative.TransientTTLSeconds > 0 {
		return time.Duration(s.Config.Creative.TransientTTLSeconds) * time.Second
	}
	return 30 * time.Minute
}

// sanitizeCreativeMessage 截断错误消息，避免把上游细节原样抛给客户端。
func sanitizeCreativeMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > maxCreativeErrorMessageChars {
		message = message[:maxCreativeErrorMessageChars]
	}
	return message
}
