package service

import (
	"context"
	"testing"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/stretchr/testify/require"
)

func TestResolveMessagesDispatchModelCNProvidersSkipOpenAIMapping(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformZhipu, PlatformDeepseek} {
		group := &Group{
			Platform: platform,
			MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
				SonnetMappedModel: "gpt-5.4",
			},
		}
		require.Empty(t, group.ResolveMessagesDispatchModel("claude-sonnet-4-5"), platform)
	}

	openAIGroup := &Group{
		Platform: PlatformOpenAI,
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			SonnetMappedModel: "gpt-5.4",
		},
	}
	require.Equal(t, "gpt-5.4", openAIGroup.ResolveMessagesDispatchModel("claude-sonnet-4-5"))
}

func TestFilterCNProviderBillingModelCandidates(t *testing.T) {
	svc := &OpenAIGatewayService{}
	apiKey := &APIKey{Group: &Group{ID: 1, Platform: PlatformKimi}}
	cnAccount := &Account{ID: 1, Platform: PlatformKimi}

	filtered := svc.filterCNProviderBillingModelCandidates(
		context.Background(),
		cnAccount,
		apiKey,
		[]string{"kimi-k2-0905-preview", "claude-sonnet-4-5", "sonnet-custom", "moonshot-v1-8k"},
	)
	require.Equal(t, []string{"kimi-k2-0905-preview", "moonshot-v1-8k"}, filtered)

	require.Empty(t, svc.filterCNProviderBillingModelCandidates(
		context.Background(), cnAccount, apiKey, []string{"claude-sonnet-4-5"},
	))

	openAIAccount := &Account{ID: 2, Platform: PlatformOpenAI}
	require.Equal(t, []string{"claude-sonnet-4-5"}, svc.filterCNProviderBillingModelCandidates(
		context.Background(), openAIAccount, apiKey, []string{"claude-sonnet-4-5"},
	))
}

func TestFilterCNProviderBillingModelCandidatesKeepsExplicitGroupPricing(t *testing.T) {
	inputPrice := 0.000001
	outputPrice := 0.000002
	billing := NewBillingService(&config.Config{}, nil)
	svc := &OpenAIGatewayService{
		billingService: billing,
		resolver:       NewModelPricingResolver(nil, billing),
	}
	group := &Group{
		ID:       1,
		Platform: PlatformKimi,
		ModelPricing: []ChannelModelPricing{{
			Models:      []string{"claude-sonnet-4-5"},
			BillingMode: BillingModeToken,
			InputPrice:  &inputPrice,
			OutputPrice: &outputPrice,
		}},
	}
	apiKey := &APIKey{Group: group}
	account := &Account{Platform: PlatformKimi}

	require.Equal(t, []string{"claude-sonnet-4-5"}, svc.filterCNProviderBillingModelCandidates(
		context.Background(), account, apiKey, []string{"claude-sonnet-4-5"},
	))
}

func TestCalculateOpenAIRecordUsageCostEmptyCandidatesIsPricingUnavailable(t *testing.T) {
	svc := &OpenAIGatewayService{}
	apiKey := &APIKey{Group: &Group{ID: 1, Platform: PlatformKimi}}

	_, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(), nil, apiKey, nil,
		1, 1, 1, 1, UsageTokens{InputTokens: 100}, "",
	)
	require.Error(t, err)
	require.True(t, isUsagePricingUnavailableError(err), err)
}
