//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/stretchr/testify/require"
)

// 复现后台删用户的事务编排：删密钥与删用户必须共用外层事务，避免其中一部分提前提交。
func TestUserRepository_DeleteUser_AtomicWithAPIKeys(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	userRepo := NewUserRepository(client, integrationDB)
	apiKeyRepo := NewAPIKeyRepository(client, integrationDB)

	user := mustCreateUser(t, client, &service.User{})
	key1 := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-atomic-a-%d", user.ID)})
	key2 := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-atomic-b-%d", user.ID)})

	t.Cleanup(func() {
		// 集成测试使用全局客户端，补充清理避免残留数据影响后续用例。
		_, _ = integrationDB.Exec(`DELETE FROM deleted_api_key_audits WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.Exec(`DELETE FROM api_keys WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.Exec(`DELETE FROM users WHERE id = $1`, user.ID)
	})

	listParams := pagination.PaginationParams{Page: 1, PageSize: 10}

	tx, err := client.Tx(ctx)
	require.NoError(t, err, "开启外层事务")
	opCtx := dbent.NewTxContext(ctx, tx)

	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx, key1.ID))
	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx, key2.ID))
	require.NoError(t, userRepo.Delete(opCtx, user.ID))

	require.NoError(t, tx.Rollback(), "回滚外层事务")

	gotUser, err := userRepo.GetByID(ctx, user.ID)
	require.NoError(t, err, "回滚后用户必须仍存在")
	require.Equal(t, user.ID, gotUser.ID)

	keys, _, err := apiKeyRepo.ListByUserID(ctx, user.ID, listParams, service.APIKeyListFilters{})
	require.NoError(t, err, "查询回滚后的密钥")
	require.Len(t, keys, 2, "回滚后密钥必须仍为可用状态")

	var auditCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deleted_api_key_audits WHERE user_id = $1`, user.ID).Scan(&auditCount))
	require.Zero(t, auditCount, "回滚后不应留下审计记录")

	tx2, err := client.Tx(ctx)
	require.NoError(t, err, "再次开启外层事务")
	opCtx2 := dbent.NewTxContext(ctx, tx2)

	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx2, key1.ID))
	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx2, key2.ID))
	require.NoError(t, userRepo.Delete(opCtx2, user.ID))

	require.NoError(t, tx2.Commit(), "提交外层事务")

	_, err = userRepo.GetByID(ctx, user.ID)
	require.Error(t, err, "提交后用户应被软删除")

	keysAfter, _, err := apiKeyRepo.ListByUserID(ctx, user.ID, listParams, service.APIKeyListFilters{})
	require.NoError(t, err, "查询提交后的密钥")
	require.Empty(t, keysAfter, "提交后密钥应全部被软删除")

	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deleted_api_key_audits WHERE user_id = $1`, user.ID).Scan(&auditCount))
	require.Equal(t, 2, auditCount, "提交后每个被删密钥都应写入审计记录")
}
