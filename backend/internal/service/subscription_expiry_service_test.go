package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionExpiryReminderSendTimeoutDoesNotPoisonNextPageList(t *testing.T) {
	now := time.Now()
	repo := &subscriptionExpiryRepoStub{
		pages: [][]UserSubscription{
			{
				{
					ID:        1,
					UserID:    10,
					PlanID:    100,
					StartsAt:  now.Add(-24 * time.Hour),
					ExpiresAt: now.Add(7*24*time.Hour + time.Hour),
					Status:    SubscriptionStatusActive,
					User:      &User{ID: 10, Email: "first@example.com", Username: "first"},
					Plan:      &SubscriptionPlan{Name: "Pro"},
				},
			},
			{
				{
					ID:        2,
					UserID:    20,
					PlanID:    100,
					StartsAt:  now.Add(-24 * time.Hour),
					ExpiresAt: now.Add(3*24*time.Hour + time.Hour),
					Status:    SubscriptionStatusActive,
					User:      &User{ID: 20, Email: "second@example.com", Username: "second"},
					Plan:      &SubscriptionPlan{Name: "Pro"},
				},
			},
		},
	}
	sender := &subscriptionExpiryBlockingSender{}
	svc := NewSubscriptionExpiryService(repo, time.Minute)
	svc.notificationEmailService = sender
	svc.reminderSendTimeout = 10 * time.Millisecond
	svc.reminderListTimeout = time.Second

	svc.sendExpiryReminders()

	require.Equal(t, 2, sender.calls)
	require.Equal(t, 2, repo.listCalls)
	require.Len(t, repo.listContextErrs, 2)
	require.NoError(t, repo.listContextErrs[0])
	require.NoError(t, repo.listContextErrs[1])
	require.ErrorIs(t, sender.errs[0], context.DeadlineExceeded)
}

type subscriptionExpiryRepoStub struct {
	userSubRepoNoop

	pages           [][]UserSubscription
	listCalls       int
	listContextErrs []error
}

func (r *subscriptionExpiryRepoStub) List(ctx context.Context, params pagination.PaginationParams, _userID, _planID *int64, status, _platform, sortBy, sortOrder string) ([]UserSubscription, *pagination.PaginationResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	requireSubscriptionExpiryListParams(ctx, params, status, sortBy, sortOrder)
	r.listCalls++
	r.listContextErrs = append(r.listContextErrs, ctx.Err())
	pageIndex := params.Page - 1
	if pageIndex < 0 || pageIndex >= len(r.pages) {
		return nil, &pagination.PaginationResult{Page: params.Page, PageSize: params.PageSize, Pages: len(r.pages)}, nil
	}
	return r.pages[pageIndex], &pagination.PaginationResult{Page: params.Page, PageSize: params.PageSize, Pages: len(r.pages)}, nil
}

func requireSubscriptionExpiryListParams(ctx context.Context, params pagination.PaginationParams, status, sortBy, sortOrder string) {
	if ctx == nil {
		panic("subscription expiry list ctx is nil")
	}
	if params.PageSize != 200 || status != SubscriptionStatusActive || sortBy != "expires_at" || sortOrder != "asc" {
		panic("unexpected subscription expiry list params")
	}
}

type subscriptionExpiryBlockingSender struct {
	mu    sync.Mutex
	calls int
	errs  []error
}

func (s *subscriptionExpiryBlockingSender) Send(ctx context.Context, input NotificationEmailSendInput) error {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if input.Event != NotificationEmailEventSubscriptionExpiryReminder {
		return errors.New("unexpected event")
	}
	if call == 1 {
		<-ctx.Done()
		s.recordErr(ctx.Err())
		return ctx.Err()
	}
	s.recordErr(ctx.Err())
	return ctx.Err()
}

func (s *subscriptionExpiryBlockingSender) recordErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errs = append(s.errs, err)
}
