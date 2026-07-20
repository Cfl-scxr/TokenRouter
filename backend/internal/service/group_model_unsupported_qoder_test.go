package service

import (
	"testing"

	"github.com/TokenFlux/TokenRouter/internal/pkg/qoder"
	"github.com/stretchr/testify/require"
)

func TestDefaultRequestModelIDsForPlatformQoder(t *testing.T) {
	require.Equal(t, qoder.DefaultRequestModelIDs(), defaultRequestModelIDsForPlatform(PlatformQoder))
}

func TestAvailableRequestModelsFromAccountsUsesQoderAccountSite(t *testing.T) {
	newAccount := func(id int64, site string) Account {
		return Account{
			ID:          id,
			Platform:    PlatformQoder,
			Status:      StatusActive,
			Schedulable: true,
			Credentials: map[string]any{"site": site},
		}
	}

	cnModels := availableRequestModelsFromAccounts([]Account{newAccount(1, "cn")}, PlatformQoder)
	require.ElementsMatch(t, qoder.DefaultRequestModelIDsForSite(qoder.SiteCN), cnModels)
	require.NotContains(t, cnModels, "claude-opus-4-6")

	globalModels := availableRequestModelsFromAccounts([]Account{newAccount(2, "global")}, PlatformQoder)
	require.ElementsMatch(t, qoder.DefaultRequestModelIDsForSite(qoder.SiteGlobal), globalModels)
	require.NotContains(t, globalModels, "minimax-m2.7")

	mixedModels := availableRequestModelsFromAccounts([]Account{newAccount(3, "global"), newAccount(4, "cn")}, PlatformQoder)
	require.ElementsMatch(t, qoder.DefaultRequestModelIDs(), mixedModels)
}
