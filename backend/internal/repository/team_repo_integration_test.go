//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTeamInvitationConcurrentAcceptanceEnforcesMemberLimit(t *testing.T) {
	ctx := context.Background()
	repo := NewTeamRepository(integrationDB)
	owner := mustCreateUser(t, integrationEntClient, &service.User{Email: uniqueTeamTestEmail("owner"), Balance: 10})
	first := mustCreateUser(t, integrationEntClient, &service.User{Email: uniqueTeamTestEmail("first")})
	second := mustCreateUser(t, integrationEntClient, &service.User{Email: uniqueTeamTestEmail("second")})
	teamCtx, err := repo.Create(ctx, "并发邀请团队", owner.ID, 1)
	require.NoError(t, err)

	firstToken := uuid.NewString()
	secondToken := uuid.NewString()
	_, err = repo.CreateInvitation(ctx, teamCtx.Team.ID, owner.ID, first.Email, firstToken, time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = repo.CreateInvitation(ctx, teamCtx.Team.ID, owner.ID, second.Email, secondToken, time.Now().Add(time.Hour))
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	accept := func(token string, user *service.User) {
		defer wg.Done()
		<-start
		_, acceptErr := repo.ResolveInvitation(ctx, token, user.ID, user.Email, "accepted", time.Now())
		results <- acceptErr
	}
	wg.Add(2)
	go accept(firstToken, first)
	go accept(secondToken, second)
	close(start)
	wg.Wait()
	close(results)

	var accepted, limited int
	for result := range results {
		switch {
		case result == nil:
			accepted++
		case errors.Is(result, service.ErrTeamMemberLimitReached):
			limited++
		default:
			require.NoError(t, result)
		}
	}
	require.Equal(t, 1, accepted)
	require.Equal(t, 1, limited)

	var memberCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM team_memberships WHERE team_id = $1 AND left_at IS NULL AND role = 'member'`, teamCtx.Team.ID).Scan(&memberCount))
	require.Equal(t, 1, memberCount)
}

func TestTeamMembershipAndBillingAreIdempotent(t *testing.T) {
	ctx := context.Background()
	teamRepo := NewTeamRepository(integrationDB)
	owner := mustCreateUser(t, integrationEntClient, &service.User{Email: uniqueTeamTestEmail("billing-owner"), Balance: 10})
	member := mustCreateUser(t, integrationEntClient, &service.User{Email: uniqueTeamTestEmail("billing-member")})
	teamCtx, err := teamRepo.Create(ctx, "计费团队", owner.ID, 10)
	require.NoError(t, err)
	token := uuid.NewString()
	_, err = teamRepo.CreateInvitation(ctx, teamCtx.Team.ID, owner.ID, member.Email, token, time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = teamRepo.ResolveInvitation(ctx, token, member.ID, member.Email, "accepted", time.Now())
	require.NoError(t, err)
	oldWindow := time.Now().AddDate(0, -2, 0)
	_, err = integrationDB.ExecContext(ctx, `
		UPDATE team_memberships SET daily_usage_usd = 9, weekly_usage_usd = 9, monthly_usage_usd = 9,
			daily_window_start = $3, weekly_window_start = $3, monthly_window_start = $3
		WHERE team_id = $1 AND user_id = $2`, teamCtx.Team.ID, member.ID, oldWindow)
	require.NoError(t, err)

	otherOwner := mustCreateUser(t, integrationEntClient, &service.User{Email: uniqueTeamTestEmail("other-owner")})
	otherTeam, err := teamRepo.Create(ctx, "其他团队", otherOwner.ID, 10)
	require.NoError(t, err)
	otherToken := uuid.NewString()
	_, err = teamRepo.CreateInvitation(ctx, otherTeam.Team.ID, otherOwner.ID, member.Email, otherToken, time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = teamRepo.ResolveInvitation(ctx, otherToken, member.ID, member.Email, "accepted", time.Now())
	require.ErrorIs(t, err, service.ErrTeamAlreadyJoined)

	teamID := teamCtx.Team.ID
	apiKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: member.ID, TeamID: &teamID, Key: "sk-team-" + uuid.NewString(), Name: "team"})
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "team-account-" + uuid.NewString(), Type: service.AccountTypeAPIKey})
	billingRepo := NewUsageBillingRepository(integrationEntClient, integrationDB)
	command := &service.UsageBillingCommand{
		RequestID:         uuid.NewString(),
		APIKeyID:          apiKey.ID,
		UserID:            owner.ID,
		ActorUserID:       member.ID,
		TeamID:            &teamID,
		AccountID:         account.ID,
		AccountType:       service.AccountTypeAPIKey,
		BillableAmountUSD: 1.25,
	}
	firstResult, err := billingRepo.Apply(ctx, command)
	require.NoError(t, err)
	require.True(t, firstResult.Applied)
	secondResult, err := billingRepo.Apply(ctx, command)
	require.NoError(t, err)
	require.False(t, secondResult.Applied)

	var daily, weekly, monthly float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd
		FROM team_memberships WHERE team_id = $1 AND user_id = $2 AND left_at IS NULL`, teamID, member.ID).Scan(&daily, &weekly, &monthly))
	require.InDelta(t, 1.25, daily, 0.000001)
	require.InDelta(t, 1.25, weekly, 0.000001)
	require.InDelta(t, 1.25, monthly, 0.000001)

	// 请求开始后发生所有权转让，结算仍使用命令中保存的旧 Owner 付款快照。
	transferToken := uuid.NewString()
	_, err = teamRepo.CreateOwnershipTransfer(ctx, teamID, owner.ID, member.ID, transferToken, time.Now().Add(time.Hour))
	require.NoError(t, err)
	_, err = teamRepo.ResolveOwnershipTransfer(ctx, transferToken, member.ID, "accepted", time.Now())
	require.NoError(t, err)
	secondCommand := *command
	secondCommand.RequestID = uuid.NewString()
	secondCommand.BillableAmountUSD = 0.75
	secondCommand.RequestFingerprint = ""
	transferResult, err := billingRepo.Apply(ctx, &secondCommand)
	require.NoError(t, err)
	require.True(t, transferResult.Applied)

	var oldOwnerBalance, newOwnerBalance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = $1`, owner.ID).Scan(&oldOwnerBalance))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = $1`, member.ID).Scan(&newOwnerBalance))
	require.InDelta(t, 8, oldOwnerBalance, 0.000001)
	require.InDelta(t, 0, newOwnerBalance, 0.000001)

	var oldRole, newRole string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT role FROM team_memberships WHERE team_id = $1 AND user_id = $2`, teamID, owner.ID).Scan(&oldRole))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT role FROM team_memberships WHERE team_id = $1 AND user_id = $2`, teamID, member.ID).Scan(&newRole))
	require.Equal(t, service.TeamRoleMember, oldRole)
	require.Equal(t, service.TeamRoleOwner, newRole)
}

func TestActiveTeamOwnerCannotBeDeleted(t *testing.T) {
	ctx := context.Background()
	repo := NewTeamRepository(integrationDB)
	owner := mustCreateUser(t, integrationEntClient, &service.User{Email: uniqueTeamTestEmail("protected-owner")})
	teamCtx, err := repo.Create(ctx, "删除保护团队", owner.ID, 10)
	require.NoError(t, err)

	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1`, owner.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "TEAM_OWNER_TRANSFER_REQUIRED")

	require.NoError(t, repo.Dissolve(ctx, teamCtx.Team.ID, time.Now()))
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1`, owner.ID)
	require.NoError(t, err)
}

func TestTeamInvitationCopiesCurrentDefaultMemberLimits(t *testing.T) {
	ctx := context.Background()
	repo := NewTeamRepository(integrationDB)
	owner := mustCreateUser(t, integrationEntClient, &service.User{Email: uniqueTeamTestEmail("default-limit-owner")})
	member := mustCreateUser(t, integrationEntClient, &service.User{Email: uniqueTeamTestEmail("default-limit-member")})
	teamCtx, err := repo.Create(ctx, "默认限额团队", owner.ID, 10)
	require.NoError(t, err)
	require.NoError(t, repo.SetDefaultMemberLimits(ctx, teamCtx.Team.ID, 1.5, 8, 30))

	token := uuid.NewString()
	_, err = repo.CreateInvitation(ctx, teamCtx.Team.ID, owner.ID, member.Email, token, time.Now().Add(time.Hour))
	require.NoError(t, err)
	memberCtx, err := repo.ResolveInvitation(ctx, token, member.ID, member.Email, "accepted", time.Now())
	require.NoError(t, err)
	require.InDelta(t, 1.5, memberCtx.Membership.DailyLimitUSD, 0.000001)
	require.InDelta(t, 8, memberCtx.Membership.WeeklyLimitUSD, 0.000001)
	require.InDelta(t, 30, memberCtx.Membership.MonthlyLimitUSD, 0.000001)

	// 后续修改默认值只影响新成员，不追溯覆盖已经加入的成员。
	require.NoError(t, repo.SetDefaultMemberLimits(ctx, teamCtx.Team.ID, 2, 10, 40))
	memberCtx, err = repo.GetContextByUserID(ctx, member.ID)
	require.NoError(t, err)
	require.InDelta(t, 1.5, memberCtx.Membership.DailyLimitUSD, 0.000001)
}

func uniqueTeamTestEmail(prefix string) string {
	return fmt.Sprintf("team-%s-%s@example.com", prefix, uuid.NewString())
}
