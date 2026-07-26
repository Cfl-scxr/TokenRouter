//go:build unit

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/stretchr/testify/require"
)

// fakeTeamRepository 只覆盖当前测试需要观察的团队仓储调用。
type fakeTeamRepository struct {
	TeamRepository
	teamContext       *TeamContext
	members           []TeamMembership
	usageSummary      *TeamUsageSummary
	usageQuery        TeamUsageQuery
	teamKeys          []TeamAPIKeyItem
	teamKeyActor      *int64
	teamKeyActorIsNil bool
	status            string
	name              string
	defaultLimits     [3]float64
	defaultLimitsSet  bool
}

func (r *fakeTeamRepository) GetContextByUserID(context.Context, int64) (*TeamContext, error) {
	return r.teamContext, nil
}

func (r *fakeTeamRepository) GetContextByTeamID(context.Context, int64) (*TeamContext, error) {
	return r.teamContext, nil
}

func (r *fakeTeamRepository) ListMembers(context.Context, int64) ([]TeamMembership, error) {
	return r.members, nil
}

func (r *fakeTeamRepository) GetUsageSummary(_ context.Context, _ int64, query TeamUsageQuery) (*TeamUsageSummary, error) {
	r.usageQuery = query
	return r.usageSummary, nil
}

func (r *fakeTeamRepository) ListTeamKeys(_ context.Context, _ int64, actorUserID *int64) ([]TeamAPIKeyItem, error) {
	r.teamKeyActorIsNil = actorUserID == nil
	if actorUserID != nil {
		value := *actorUserID
		r.teamKeyActor = &value
	}
	return r.teamKeys, nil
}

func (r *fakeTeamRepository) SetStatus(_ context.Context, _ int64, status string) error {
	r.status = status
	r.teamContext.Team.Status = status
	return nil
}

func (r *fakeTeamRepository) UpdateName(_ context.Context, _ int64, name string) error {
	r.name = name
	r.teamContext.Team.Name = name
	return nil
}

func (r *fakeTeamRepository) SetDefaultMemberLimits(_ context.Context, _ int64, daily, weekly, monthly float64) error {
	r.defaultLimits = [3]float64{daily, weekly, monthly}
	r.defaultLimitsSet = true
	r.teamContext.Team.DefaultDailyLimitUSD = daily
	r.teamContext.Team.DefaultWeeklyLimitUSD = weekly
	r.teamContext.Team.DefaultMonthlyLimitUSD = monthly
	return nil
}

func teamServiceTestContext(userID int64, role string) *TeamContext {
	return &TeamContext{
		Team:       &Team{ID: 11, Name: "测试团队", Status: TeamStatusActive},
		Membership: &TeamMembership{ID: 21, TeamID: 11, UserID: userID, Role: role},
		Owner:      &TeamMembership{ID: 20, TeamID: 11, UserID: 1, Role: TeamRoleOwner},
	}
}

func TestTeamServiceListMembersMemberOnlySeesOwnerAndSelf(t *testing.T) {
	repo := &fakeTeamRepository{
		teamContext: teamServiceTestContext(2, TeamRoleMember),
		members: []TeamMembership{
			{UserID: 1, Role: TeamRoleOwner},
			{UserID: 2, Role: TeamRoleMember},
			{UserID: 3, Role: TeamRoleMember},
		},
	}
	svc := NewTeamService(repo, nil, nil, nil, nil)

	members, err := svc.ListMembers(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, []int64{members[0].UserID, members[1].UserID})
}

func TestTeamServiceUsageScopeCannotBeExpandedByMember(t *testing.T) {
	otherUserID := int64(99)
	repo := &fakeTeamRepository{
		teamContext:  teamServiceTestContext(2, TeamRoleMember),
		usageSummary: &TeamUsageSummary{},
	}
	svc := NewTeamService(repo, nil, nil, nil, nil)

	_, err := svc.GetUsageSummary(context.Background(), 2, TeamUsageQuery{ActorUserID: &otherUserID})
	require.NoError(t, err)
	require.NotNil(t, repo.usageQuery.ActorUserID)
	require.Equal(t, int64(2), *repo.usageQuery.ActorUserID)
}

func TestTeamServiceUsageScopeOwnerKeepsMemberFilter(t *testing.T) {
	targetUserID := int64(3)
	repo := &fakeTeamRepository{
		teamContext:  teamServiceTestContext(1, TeamRoleOwner),
		usageSummary: &TeamUsageSummary{},
	}
	svc := NewTeamService(repo, nil, nil, nil, nil)

	_, err := svc.GetUsageSummary(context.Background(), 1, TeamUsageQuery{ActorUserID: &targetUserID})
	require.NoError(t, err)
	require.NotNil(t, repo.usageQuery.ActorUserID)
	require.Equal(t, targetUserID, *repo.usageQuery.ActorUserID)
}

func TestTeamServiceAdminUsageKeepsMemberFilter(t *testing.T) {
	targetUserID := int64(3)
	repo := &fakeTeamRepository{
		teamContext:  teamServiceTestContext(1, TeamRoleOwner),
		usageSummary: &TeamUsageSummary{},
	}
	svc := NewTeamService(repo, nil, nil, nil, nil)

	_, err := svc.AdminGetUsageSummary(context.Background(), 11, TeamUsageQuery{ActorUserID: &targetUserID})
	require.NoError(t, err)
	require.NotNil(t, repo.usageQuery.ActorUserID)
	require.Equal(t, targetUserID, *repo.usageQuery.ActorUserID)
}

func TestTeamServiceAdminUpdatesName(t *testing.T) {
	repo := &fakeTeamRepository{teamContext: teamServiceTestContext(1, TeamRoleOwner)}
	svc := NewTeamService(repo, nil, nil, nil, nil)

	err := svc.AdminUpdateName(context.Background(), 11, "  新团队名称  ")
	require.NoError(t, err)
	require.Equal(t, "新团队名称", repo.name)
}

func TestTeamServiceListTeamKeysMasksSecretsAndAppliesRoleFilter(t *testing.T) {
	tests := []struct {
		name            string
		userID          int64
		role            string
		wantActorIsNil  bool
		wantActorUserID int64
	}{
		{name: "owner", userID: 1, role: TeamRoleOwner, wantActorIsNil: true},
		{name: "member", userID: 2, role: TeamRoleMember, wantActorUserID: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeTeamRepository{
				teamContext: teamServiceTestContext(tt.userID, tt.role),
				teamKeys:    []TeamAPIKeyItem{{ID: 31, Key: "sk-team-secret-value", Name: "team"}},
			}
			svc := NewTeamService(repo, nil, nil, nil, nil)

			keys, err := svc.ListTeamKeys(context.Background(), tt.userID)
			require.NoError(t, err)
			require.Len(t, keys, 1)
			require.Empty(t, keys[0].Key)
			require.Equal(t, "sk-tea****alue", keys[0].MaskedKey)
			require.Equal(t, tt.wantActorIsNil, repo.teamKeyActorIsNil)
			if !tt.wantActorIsNil {
				require.NotNil(t, repo.teamKeyActor)
				require.Equal(t, tt.wantActorUserID, *repo.teamKeyActor)
			}
		})
	}
}

func TestCheckTeamMemberLimitSnapshot(t *testing.T) {
	tests := []struct {
		name   string
		member *TeamMembership
		want   error
	}{
		{name: "unlimited", member: &TeamMembership{Role: TeamRoleMember}},
		{name: "owner_ignores_limits", member: &TeamMembership{Role: TeamRoleOwner, DailyLimitUSD: 1, DailyUsageUSD: 1}},
		{name: "daily", member: &TeamMembership{Role: TeamRoleMember, DailyLimitUSD: 1, DailyUsageUSD: 1}, want: ErrTeamMemberDailyExceeded},
		{name: "weekly", member: &TeamMembership{Role: TeamRoleMember, WeeklyLimitUSD: 2, WeeklyUsageUSD: 3}, want: ErrTeamMemberWeeklyExceeded},
		{name: "monthly", member: &TeamMembership{Role: TeamRoleMember, MonthlyLimitUSD: 4, MonthlyUsageUSD: 4}, want: ErrTeamMemberMonthlyExceeded},
		{name: "below_limits", member: &TeamMembership{Role: TeamRoleMember, DailyLimitUSD: 2, DailyUsageUSD: 1, WeeklyLimitUSD: 5, WeeklyUsageUSD: 4, MonthlyLimitUSD: 10, MonthlyUsageUSD: 9}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTeamMemberLimitSnapshot(tt.member)
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestTeamServiceSuspendedOwnerCanResume(t *testing.T) {
	repo := &fakeTeamRepository{teamContext: teamServiceTestContext(1, TeamRoleOwner)}
	repo.teamContext.Team.Status = TeamStatusSuspended
	svc := NewTeamService(repo, nil, nil, nil, nil)

	teamCtx, err := svc.SetStatus(context.Background(), 1, TeamStatusActive)
	require.NoError(t, err)
	require.Equal(t, TeamStatusActive, repo.status)
	require.Equal(t, TeamStatusActive, teamCtx.Team.Status)
}

func TestTeamServiceSuspendedMemberCannotChangeStatus(t *testing.T) {
	repo := &fakeTeamRepository{teamContext: teamServiceTestContext(2, TeamRoleMember)}
	repo.teamContext.Team.Status = TeamStatusSuspended
	svc := NewTeamService(repo, nil, nil, nil, nil)

	_, err := svc.SetStatus(context.Background(), 2, TeamStatusActive)
	require.ErrorIs(t, err, ErrTeamOwnerRequired)
}

func TestTeamServiceOwnerUpdatesDefaultMemberLimits(t *testing.T) {
	repo := &fakeTeamRepository{teamContext: teamServiceTestContext(1, TeamRoleOwner)}
	svc := NewTeamService(repo, nil, nil, nil, nil)

	teamCtx, err := svc.UpdateDefaultMemberLimits(context.Background(), 1, 1.5, 8, 30)
	require.NoError(t, err)
	require.True(t, repo.defaultLimitsSet)
	require.Equal(t, [3]float64{1.5, 8, 30}, repo.defaultLimits)
	require.Equal(t, 1.5, teamCtx.Team.DefaultDailyLimitUSD)
}

func TestTeamServiceRejectsNegativeDefaultMemberLimits(t *testing.T) {
	repo := &fakeTeamRepository{teamContext: teamServiceTestContext(1, TeamRoleOwner)}
	svc := NewTeamService(repo, nil, nil, nil, nil)

	_, err := svc.UpdateDefaultMemberLimits(context.Background(), 1, -1, 8, 30)
	require.Error(t, err)
	require.False(t, repo.defaultLimitsSet)
}

func TestTeamServiceInvitationEmailUsesCustomNotificationTemplate(t *testing.T) {
	ctx := context.Background()
	settingRepo := newNotificationEmailMemorySettingRepo()
	smtpServer := startNotificationEmailTestSMTPServer(t)
	require.NoError(t, settingRepo.SetMultiple(ctx, smtpServer.settings()))

	emailService := NewEmailService(settingRepo, nil)
	notificationService := NewNotificationEmailService(settingRepo, emailService)
	_, err := notificationService.UpdateTemplate(
		ctx,
		NotificationEmailEventTeamInvitation,
		"en",
		"Custom invitation for {{team_name}}",
		`<h1>Custom team invitation</h1><p>{{recipient_name}}</p><a href="{{invitation_url}}">Join {{team_name}}</a><p>{{expires_at}}</p>`,
	)
	require.NoError(t, err)

	svc := NewTeamService(nil, nil, emailService, nil, &config.Config{
		Server: config.ServerConfig{FrontendURL: "https://router.example"},
	})
	expiresAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	require.NoError(t, svc.sendInvitationEmail(ctx, "member@example.com", "Platform Team", "test-token", expiresAt))

	require.Equal(t, int64(1), smtpServer.messageCount())
	message := smtpServer.lastMessage()
	require.Contains(t, message, "Subject: Custom invitation for Platform Team")
	require.Contains(t, message, "Custom team invitation")
	require.Contains(t, message, "https://router.example/team?invitation=test-token")
	require.Contains(t, message, expiresAt.Format(time.RFC3339))
	require.False(t, strings.Contains(message, "你被邀请加入团队"))
}
