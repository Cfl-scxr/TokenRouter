package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
)

const (
	subscriptionExpiryUpdateTimeout       = 10 * time.Second
	subscriptionExpiryReminderListTimeout = 10 * time.Second
	subscriptionExpiryReminderSendTimeout = emailSendTimeout
)

type subscriptionExpiryNotificationSender interface {
	Send(ctx context.Context, input NotificationEmailSendInput) error
}

// SubscriptionExpiryService 定期更新过期订阅状态，并发送到期提醒。
type SubscriptionExpiryService struct {
	userSubRepo              UserSubscriptionRepository
	notificationEmailService subscriptionExpiryNotificationSender
	interval                 time.Duration
	updateTimeout            time.Duration
	reminderListTimeout      time.Duration
	reminderSendTimeout      time.Duration
	stopCh                   chan struct{}
	stopOnce                 sync.Once
	wg                       sync.WaitGroup
}

func NewSubscriptionExpiryService(userSubRepo UserSubscriptionRepository, interval time.Duration) *SubscriptionExpiryService {
	return &SubscriptionExpiryService{
		userSubRepo:         userSubRepo,
		interval:            interval,
		updateTimeout:       subscriptionExpiryUpdateTimeout,
		reminderListTimeout: subscriptionExpiryReminderListTimeout,
		reminderSendTimeout: subscriptionExpiryReminderSendTimeout,
		stopCh:              make(chan struct{}),
	}
}

func (s *SubscriptionExpiryService) SetNotificationEmailService(notificationEmailService *NotificationEmailService) {
	s.notificationEmailService = notificationEmailService
}

func (s *SubscriptionExpiryService) Start() {
	if s == nil || s.userSubRepo == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *SubscriptionExpiryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *SubscriptionExpiryService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), s.expiredStatusUpdateTimeout())
	updated, err := s.userSubRepo.BatchUpdateExpiredStatus(ctx)
	cancel()
	if err != nil {
		log.Printf("[SubscriptionExpiry] Update expired subscriptions failed: %v", err)
		return
	}
	if updated > 0 {
		log.Printf("[SubscriptionExpiry] Updated %d expired subscriptions", updated)
	}
	s.sendExpiryReminders()
}

func (s *SubscriptionExpiryService) sendExpiryReminders() {
	if s == nil || s.userSubRepo == nil || s.notificationEmailService == nil {
		return
	}
	for page := 1; ; page++ {
		ctx, cancel := context.WithTimeout(context.Background(), s.expiryReminderListTimeout())
		subs, pag, err := s.userSubRepo.List(ctx, pagination.PaginationParams{Page: page, PageSize: 200}, nil, nil, SubscriptionStatusActive, "", "expires_at", "asc")
		cancel()
		if err != nil {
			log.Printf("[SubscriptionExpiry] List active subscriptions for reminder failed: %v", err)
			return
		}
		for i := range subs {
			s.sendExpiryReminderIfDue(&subs[i])
		}
		if pag == nil || page >= pag.Pages || len(subs) == 0 {
			return
		}
	}
}

func (s *SubscriptionExpiryService) sendExpiryReminderIfDue(sub *UserSubscription) {
	if sub == nil || sub.User == nil || sub.User.Email == "" {
		return
	}
	daysRemaining := sub.DaysRemaining()
	if daysRemaining != 7 && daysRemaining != 3 && daysRemaining != 1 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.expiryReminderSendTimeout())
	defer cancel()
	if err := s.notificationEmailService.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventSubscriptionExpiryReminder,
		RecipientEmail: sub.User.Email,
		RecipientName:  firstNonEmpty(sub.User.Username, sub.User.Email),
		UserID:         sub.UserID,
		SourceType:     "user_subscription",
		SourceID:       strconv.FormatInt(sub.ID, 10),
		ReminderKey:    fmt.Sprintf("%dd", daysRemaining),
		Variables: map[string]string{
			"subscription_group": subscriptionReminderPlanName(sub),
			"expiry_time":        sub.ExpiresAt.Format("2006-01-02 15:04"),
			"days_remaining":     strconv.Itoa(daysRemaining),
		},
	}); err != nil {
		log.Printf("[SubscriptionExpiry] Send expiry reminder failed: subscription=%d user=%d err=%v", sub.ID, sub.UserID, err)
	}
}

func (s *SubscriptionExpiryService) expiredStatusUpdateTimeout() time.Duration {
	if s != nil && s.updateTimeout > 0 {
		return s.updateTimeout
	}
	return subscriptionExpiryUpdateTimeout
}

func (s *SubscriptionExpiryService) expiryReminderListTimeout() time.Duration {
	if s != nil && s.reminderListTimeout > 0 {
		return s.reminderListTimeout
	}
	return subscriptionExpiryReminderListTimeout
}

func (s *SubscriptionExpiryService) expiryReminderSendTimeout() time.Duration {
	if s != nil && s.reminderSendTimeout > 0 {
		return s.reminderSendTimeout
	}
	return subscriptionExpiryReminderSendTimeout
}

func subscriptionReminderPlanName(sub *UserSubscription) string {
	if sub == nil || sub.Plan == nil || sub.Plan.Name == "" {
		return "Subscription"
	}
	return sub.Plan.Name
}
