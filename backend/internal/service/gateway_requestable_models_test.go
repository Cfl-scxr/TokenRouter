package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type requestableModelsChannelRepoStub struct {
	ChannelRepository
	err error
}

func (s *requestableModelsChannelRepoStub) ListAll(context.Context) ([]Channel, error) {
	return nil, s.err
}

// newRequestableModelsChannelService 构造只读渠道缓存，避免测试依赖数据库仓储。
func newRequestableModelsChannelService(groupID int64, platform string, channel Channel) *ChannelService {
	channel.GroupIDs = []int64{groupID}
	cache := populateChannelCache([]Channel{channel}, map[int64]string{groupID: platform})
	service := &ChannelService{}
	service.cache.Store(cache)
	return service
}

// requestableModelByID 从解析结果中查找指定客户端模型。
func requestableModelByID(models []RequestableModel, id string) (RequestableModel, bool) {
	for _, model := range models {
		if model.ID == id {
			return model, true
		}
	}
	return RequestableModel{}, false
}

func TestResolveRequestableModels_UsesConfiguredPricingBasis(t *testing.T) {
	groupID := int64(4101)
	inputPrice := 0.01
	for _, test := range []struct {
		name          string
		billingSource string
		pricingModel  string
	}{
		{name: "requested", billingSource: BillingModelSourceRequested, pricingModel: "client-alias"},
		{name: "channel mapped", billingSource: BillingModelSourceChannelMapped, pricingModel: "channel-model"},
		{name: "upstream", billingSource: BillingModelSourceUpstream, pricingModel: "upstream-model"},
	} {
		t.Run(test.name, func(t *testing.T) {
			channel := Channel{
				ID:                 51,
				Status:             StatusActive,
				BillingModelSource: test.billingSource,
				RestrictModels:     true,
				ModelMapping: map[string]map[string]string{
					PlatformOpenAI: {"client-alias": "channel-model"},
				},
				ModelPricing: []ChannelModelPricing{{
					Platform:   PlatformOpenAI,
					Models:     []string{test.pricingModel},
					InputPrice: &inputPrice,
				}},
			}
			account := Account{
				ID:       61,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"model_mapping":   map[string]any{"channel-model": "upstream-model"},
					"model_whitelist": []any{"upstream-model"},
				},
			}
			repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}}
			svc := &GatewayService{
				accountRepo:    repo,
				channelService: newRequestableModelsChannelService(groupID, PlatformOpenAI, channel),
			}

			result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformOpenAI)
			model, ok := requestableModelByID(result.Models, "client-alias")
			require.True(t, ok)
			require.True(t, result.Restricted)
			require.Equal(t, test.pricingModel, model.PricingModel)
			require.False(t, model.PricingAmbiguous)
		})
	}
}

func TestResolveRequestableModels_WildcardsMatchConcreteCandidateOnly(t *testing.T) {
	groupID := int64(4102)
	price := 0.02
	channel := Channel{
		ID:                 52,
		Status:             StatusActive,
		BillingModelSource: BillingModelSourceChannelMapped,
		RestrictModels:     true,
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"client-*": "channel-model"},
		},
		ModelPricing: []ChannelModelPricing{{
			Platform:   PlatformOpenAI,
			Models:     []string{"channel-*"},
			InputPrice: &price,
		}},
	}
	account := Account{
		ID:       62,
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"client-one": "client-one",
				"channel-*":  "upstream-model",
			},
			"model_whitelist": []any{"upstream-model"},
		},
	}
	svc := &GatewayService{
		accountRepo:    &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}},
		channelService: newRequestableModelsChannelService(groupID, PlatformOpenAI, channel),
	}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformOpenAI)
	model, ok := requestableModelByID(result.Models, "client-one")
	require.True(t, ok)
	require.Equal(t, "channel-model", model.PricingModel)
	require.NotContains(t, RequestableModelIDs(result.Models), "client-*")
	require.NotContains(t, RequestableModelIDs(result.Models), "channel-*")
}

func TestResolveRequestableModels_UnrestrictedAccountAddsDefaultsAndMappingSource(t *testing.T) {
	groupID := int64(4103)
	account := Account{
		ID:       63,
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"CUSTOM-Alias": "gpt-5.5",
				"custom-alias": "gpt-5.6",
				"gpt-*":        "gpt-5.5",
			},
		},
	}
	svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}}}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformOpenAI)
	ids := RequestableModelIDs(result.Models)
	require.Contains(t, ids, "CUSTOM-Alias")
	require.Contains(t, ids, "gpt-5.5")
	require.NotContains(t, ids, "custom-alias")
	require.NotContains(t, ids, "gpt-*")
}

func TestResolveRequestableModels_AccountWhitelistRemovesUnsupportedCandidate(t *testing.T) {
	groupID := int64(4104)
	channel := Channel{
		ID:     54,
		Status: StatusActive,
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"blocked-alias": "blocked-final"},
		},
	}
	account := Account{
		ID:       64,
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_whitelist": []any{"allowed-final"},
		},
	}
	svc := &GatewayService{
		accountRepo:    &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}},
		channelService: newRequestableModelsChannelService(groupID, PlatformOpenAI, channel),
	}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformOpenAI)
	require.NotContains(t, RequestableModelIDs(result.Models), "blocked-alias")
}

func TestResolveRequestableModels_UpstreamPricingAmbiguousAcrossAccounts(t *testing.T) {
	groupID := int64(4105)
	channel := Channel{
		ID:                 55,
		Status:             StatusActive,
		BillingModelSource: BillingModelSourceUpstream,
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"client-alias": "channel-model"},
		},
	}
	accounts := []Account{
		{ID: 65, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"channel-model": "upstream-a"}}},
		{ID: 66, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"channel-model": "upstream-b"}}},
	}
	svc := &GatewayService{
		accountRepo:    &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: accounts}},
		channelService: newRequestableModelsChannelService(groupID, PlatformOpenAI, channel),
	}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformOpenAI)
	model, ok := requestableModelByID(result.Models, "client-alias")
	require.True(t, ok)
	require.Empty(t, model.PricingModel)
	require.True(t, model.PricingAmbiguous)
}

func TestResolveRequestableModels_UpstreamUsesBedrockRegionalModel(t *testing.T) {
	groupID := int64(4114)
	price := 0.08
	upstreamModel := "us.anthropic.claude-sonnet-4-5-20250929-v1:0"
	channel := Channel{
		ID:                 62,
		Status:             StatusActive,
		BillingModelSource: BillingModelSourceUpstream,
		RestrictModels:     true,
		ModelMapping: map[string]map[string]string{
			PlatformAnthropic: {"claude-sonnet-4-5": "claude-sonnet-4-5"},
		},
		ModelPricing: []ChannelModelPricing{{
			Platform:   PlatformAnthropic,
			Models:     []string{upstreamModel},
			InputPrice: &price,
		}},
	}
	account := Account{
		ID:       74,
		Platform: PlatformAnthropic,
		Type:     AccountTypeBedrock,
		Credentials: map[string]any{
			"aws_region": "us-east-1",
		},
	}
	svc := &GatewayService{
		accountRepo:    &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}},
		channelService: newRequestableModelsChannelService(groupID, PlatformAnthropic, channel),
	}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformAnthropic)
	model, ok := requestableModelByID(result.Models, "claude-sonnet-4-5")
	require.True(t, ok)
	require.Equal(t, upstreamModel, model.PricingModel)
	require.False(t, model.PricingAmbiguous)
}

func TestResolveRequestableModels_UpstreamMarksAntigravityThinkingVariantAmbiguous(t *testing.T) {
	groupID := int64(4115)
	channel := Channel{
		ID:                 63,
		Status:             StatusActive,
		BillingModelSource: BillingModelSourceUpstream,
	}
	account := Account{ID: 75, Platform: PlatformAntigravity}
	svc := &GatewayService{
		accountRepo:    &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}},
		channelService: newRequestableModelsChannelService(groupID, PlatformAntigravity, channel),
	}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformAntigravity)
	model, ok := requestableModelByID(result.Models, "claude-sonnet-4-5")
	require.True(t, ok)
	require.Empty(t, model.PricingModel)
	require.True(t, model.PricingAmbiguous)
}

func TestResolveRequestableModels_UpstreamNormalizesAnthropicOAuthMapping(t *testing.T) {
	groupID := int64(4116)
	channel := Channel{
		ID:                 64,
		Status:             StatusActive,
		BillingModelSource: BillingModelSourceUpstream,
	}
	account := Account{
		ID:       76,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"client-alias": "claude-sonnet-4-5"},
		},
	}
	svc := &GatewayService{
		accountRepo:    &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}},
		channelService: newRequestableModelsChannelService(groupID, PlatformAnthropic, channel),
	}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformAnthropic)
	model, ok := requestableModelByID(result.Models, "client-alias")
	require.True(t, ok)
	require.Equal(t, "claude-sonnet-4-5-20250929", model.PricingModel)
	require.False(t, model.PricingAmbiguous)
}

func TestResolveRequestableModels_RestrictionEmptyDoesNotFallBack(t *testing.T) {
	groupID := int64(4106)
	channel := Channel{
		ID:                 56,
		Status:             StatusActive,
		BillingModelSource: BillingModelSourceRequested,
		RestrictModels:     true,
	}
	account := Account{ID: 67, Platform: PlatformOpenAI}
	svc := &GatewayService{
		accountRepo:    &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}},
		channelService: newRequestableModelsChannelService(groupID, PlatformOpenAI, channel),
	}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformOpenAI)
	require.True(t, result.Restricted)
	require.Empty(t, result.Models)
}

func TestResolveRequestableModels_PricingDoesNotCrossPlatforms(t *testing.T) {
	groupID := int64(4107)
	price := 0.03
	channel := Channel{
		ID:                 57,
		Status:             StatusActive,
		BillingModelSource: BillingModelSourceRequested,
		RestrictModels:     true,
		ModelPricing: []ChannelModelPricing{{
			Platform:   PlatformOpenAI,
			Models:     []string{"same-model"},
			InputPrice: &price,
		}},
	}
	account := Account{
		ID:       68,
		Platform: PlatformAnthropic,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"same-model": "same-model"},
		},
	}
	svc := &GatewayService{
		accountRepo:    &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}},
		channelService: newRequestableModelsChannelService(groupID, PlatformAnthropic, channel),
	}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformAnthropic)
	require.True(t, result.Restricted)
	require.Empty(t, result.Models)
}

func TestResolveRequestableModels_QoderRequiresEffectivePricing(t *testing.T) {
	groupID := int64(4108)
	effectivePrice := 0.04
	channel := Channel{
		ID:                 58,
		Status:             StatusActive,
		BillingModelSource: BillingModelSourceRequested,
		RestrictModels:     true,
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformQoder, Models: []string{"qoder-model"}},
			{Platform: PlatformQoder, Models: []string{"qoder-*"}, InputPrice: &effectivePrice},
		},
	}
	account := Account{ID: 69, Platform: PlatformQoder}
	svc := &GatewayService{
		accountRepo:    &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}},
		channelService: newRequestableModelsChannelService(groupID, PlatformQoder, channel),
	}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformQoder)
	require.Contains(t, RequestableModelIDs(result.Models), "qoder-model")
}

func TestResolveRequestableModels_AccountQueryFailureKeepsFallback(t *testing.T) {
	groupID := int64(4109)
	svc := &GatewayService{accountRepo: &modelsListAccountRepoStub{err: errors.New("temporary failure")}}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformOpenAI)
	require.False(t, result.Restricted)
	require.Contains(t, RequestableModelIDs(result.Models), "gpt-5.5")
}

func TestResolveRequestableModels_ChannelQueryFailureKeepsAccountCandidates(t *testing.T) {
	groupID := int64(4113)
	account := Account{
		ID:       73,
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping":   map[string]any{"client-alias": "upstream-model"},
			"model_whitelist": []any{"upstream-model"},
		},
	}
	svc := &GatewayService{
		accountRepo: &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}},
		channelService: NewChannelService(&requestableModelsChannelRepoStub{
			err: errors.New("temporary channel failure"),
		}, nil),
	}

	result := svc.ResolveRequestableModels(context.Background(), &groupID, PlatformOpenAI)
	require.False(t, result.Restricted)
	require.Contains(t, RequestableModelIDs(result.Models), "client-alias")
}

func TestModelMarketplaceUsesResolvedChannelMappedPricingModel(t *testing.T) {
	groupID := int64(4110)
	inputPrice := 0.05
	outputPrice := 0.06
	channel := Channel{
		ID:                 59,
		Status:             StatusActive,
		BillingModelSource: BillingModelSourceChannelMapped,
		RestrictModels:     true,
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"client-alias": "channel-model"},
		},
		ModelPricing: []ChannelModelPricing{{
			Platform:    PlatformOpenAI,
			Models:      []string{"channel-model"},
			InputPrice:  &inputPrice,
			OutputPrice: &outputPrice,
		}},
	}
	account := Account{
		ID:       70,
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping":   map[string]any{"channel-model": "upstream-model"},
			"model_whitelist": []any{"upstream-model"},
		},
	}
	channelService := newRequestableModelsChannelService(groupID, PlatformOpenAI, channel)
	billingService := NewBillingService(nil, nil)
	gatewayService := &GatewayService{
		accountRepo:    &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {account}}},
		channelService: channelService,
		resolver:       NewModelPricingResolver(channelService, billingService),
	}
	marketplace := NewModelMarketplaceService(nil, nil, gatewayService, billingService, nil, nil, nil)

	models := marketplace.listPublicModelsForGroup(context.Background(), &Group{ID: groupID, Platform: PlatformOpenAI, RateMultiplier: 1})
	var alias *ModelMarketplaceModel
	for i := range models {
		if models[i].ID == "client-alias" {
			alias = &models[i]
			break
		}
	}
	require.NotNil(t, alias)
	require.Equal(t, "priced", alias.Pricing.PriceStatus)
	require.Equal(t, inputPrice, alias.Pricing.InputPricePerToken)
	require.Equal(t, outputPrice, alias.Pricing.OutputPricePerToken)
}

func TestModelMarketplaceKeepsAmbiguousUpstreamModelUnpriced(t *testing.T) {
	groupID := int64(4111)
	price := 0.07
	channel := Channel{
		ID:                 60,
		Status:             StatusActive,
		BillingModelSource: BillingModelSourceUpstream,
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"client-alias": "channel-model"},
		},
		ModelPricing: []ChannelModelPricing{
			{Platform: PlatformOpenAI, Models: []string{"upstream-a"}, InputPrice: &price},
			{Platform: PlatformOpenAI, Models: []string{"upstream-b"}, InputPrice: &price},
		},
	}
	accounts := []Account{
		{ID: 71, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"channel-model": "upstream-a"}, "model_whitelist": []any{"upstream-a"}}},
		{ID: 72, Platform: PlatformOpenAI, Credentials: map[string]any{"model_mapping": map[string]any{"channel-model": "upstream-b"}, "model_whitelist": []any{"upstream-b"}}},
	}
	channelService := newRequestableModelsChannelService(groupID, PlatformOpenAI, channel)
	billingService := NewBillingService(nil, nil)
	gatewayService := &GatewayService{
		accountRepo:    &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: accounts}},
		channelService: channelService,
		resolver:       NewModelPricingResolver(channelService, billingService),
	}
	marketplace := NewModelMarketplaceService(nil, nil, gatewayService, billingService, nil, nil, nil)

	models := marketplace.listPublicModelsForGroup(context.Background(), &Group{ID: groupID, Platform: PlatformOpenAI, RateMultiplier: 1})
	var alias *ModelMarketplaceModel
	for i := range models {
		if models[i].ID == "client-alias" {
			alias = &models[i]
			break
		}
	}
	require.NotNil(t, alias)
	require.Equal(t, "unpriced", alias.Pricing.PriceStatus)
	require.Equal(t, "unknown", alias.Pricing.PricingMode)
}

func TestModelMarketplaceQoderUsesResolvedRequestedPricingModelWithoutRemapping(t *testing.T) {
	groupID := int64(4112)
	channelMappedPrice := 0.08
	channel := Channel{
		ID:                 61,
		Status:             StatusActive,
		BillingModelSource: BillingModelSourceRequested,
		ModelMapping: map[string]map[string]string{
			PlatformQoder: {"client-model": "channel-model"},
		},
		ModelPricing: []ChannelModelPricing{{
			Platform:   PlatformQoder,
			Models:     []string{"channel-model"},
			InputPrice: &channelMappedPrice,
		}},
	}
	channelService := newRequestableModelsChannelService(groupID, PlatformQoder, channel)
	billingService := NewBillingService(nil, nil)
	marketplace := NewModelMarketplaceService(nil, nil, &GatewayService{
		channelService: channelService,
		resolver:       NewModelPricingResolver(channelService, billingService),
	}, billingService, nil, nil, nil)

	pricing := marketplace.getRequestableModelDisplayPricing(
		context.Background(),
		&Group{ID: groupID, Platform: PlatformQoder, RateMultiplier: 1},
		marketplaceModelDef{ID: "client-model", PricingModel: "client-model"},
		nil,
	)

	require.Equal(t, "unpriced", pricing.PriceStatus)
	require.Equal(t, "unknown", pricing.PricingMode)
}
