package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGatewayAccountLayerUsesChannelMappedModelForSupportAndRateLimit(t *testing.T) {
	groupID := int64(4201)
	channel := Channel{
		ID:     71,
		Status: StatusActive,
		ModelMapping: map[string]map[string]string{
			PlatformAnthropic: {"client-alias": "channel-model"},
		},
	}
	svc := &GatewayService{channelService: newRequestableModelsChannelService(groupID, PlatformAnthropic, channel)}
	ctx := svc.withGroupContext(context.Background(), &Group{
		ID:       groupID,
		Platform: PlatformAnthropic,
		Status:   StatusActive,
		Hydrated: true,
	})
	future := time.Now().Add(10 * time.Minute).Format(time.RFC3339)
	account := &Account{
		Platform:    PlatformAnthropic,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"model_mapping":   map[string]any{"channel-model": "upstream-model"},
			"model_whitelist": []any{"upstream-model"},
		},
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"upstream-model": map[string]any{"rate_limit_reset_at": future},
			},
		},
	}

	require.True(t, svc.isModelSupportedByAccountWithContext(ctx, account, "client-alias"))
	require.False(t, svc.isAccountSchedulableForModelSelection(ctx, account, "client-alias"))
	require.True(t, svc.shouldClearStickySessionForAccountLayer(ctx, account, "client-alias"))
}

func TestOpenAIAdvancedSchedulerUsesRoutingModelAndKeepsRequestedModel(t *testing.T) {
	account := &Account{
		ID:       72,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"model_mapping":   map[string]any{"channel-model": "upstream-model"},
			"model_whitelist": []any{"upstream-model"},
		},
	}
	scheduler := &defaultOpenAIAccountScheduler{service: &OpenAIGatewayService{}}
	req := OpenAIAccountScheduleRequest{
		Platform:       PlatformOpenAI,
		RequestedModel: "client-alias",
		RoutingModel:   "channel-model",
	}

	require.Equal(t, "client-alias", req.RequestedModel)
	require.Equal(t, "channel-model", req.routingModel())
	require.True(t, scheduler.isAccountRequestCompatible(context.Background(), account, req))
}

func TestOpenAIUpstreamRestrictionAppliesChannelThenAccountMapping(t *testing.T) {
	groupID := int64(4202)
	price := 0.01
	channel := Channel{
		ID:                 73,
		Status:             StatusActive,
		RestrictModels:     true,
		BillingModelSource: BillingModelSourceUpstream,
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"client-alias": "channel-model"},
		},
		ModelPricing: []ChannelModelPricing{{
			Platform:   PlatformOpenAI,
			Models:     []string{"upstream-model"},
			InputPrice: &price,
		}},
	}
	svc := &OpenAIGatewayService{channelService: newRequestableModelsChannelService(groupID, PlatformOpenAI, channel)}
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"channel-model": "upstream-model"},
		},
	}

	require.False(t, svc.isUpstreamModelRestrictedByChannel(context.Background(), groupID, account, "client-alias", false))
}

func TestModelAvailabilityDiagnosisAcceptsChannelAlias(t *testing.T) {
	groupID := int64(4203)
	channel := Channel{
		ID:     74,
		Status: StatusActive,
		ModelMapping: map[string]map[string]string{
			PlatformOpenAI: {"client-alias": "channel-model"},
		},
	}
	account := Account{
		ID:       75,
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping":   map[string]any{"channel-model": "upstream-model"},
			"model_whitelist": []any{"upstream-model"},
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo:    schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		channelService: newRequestableModelsChannelService(groupID, PlatformOpenAI, channel),
	}

	diagnosis := svc.DiagnoseModelAvailabilityForPlatform(context.Background(), &groupID, "client-alias", PlatformOpenAI)
	require.True(t, diagnosis.HasAccountsInPool)
	require.True(t, diagnosis.HasModelSupport)
}
