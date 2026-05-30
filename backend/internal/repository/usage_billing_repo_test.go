package repository

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeUsageBillingWindow_ExpiryTailKeepsMonthlyUsage(t *testing.T) {
	now := time.Date(2026, 5, 30, 0, 5, 0, 0, time.UTC)
	startsAt := time.Date(2026, 4, 30, 8, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 5, 30, 8, 0, 0, 0, time.UTC)
	windowStart := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)

	start, used := normalizeUsageBillingWindow(
		sql.NullTime{Time: windowStart, Valid: true},
		sql.NullFloat64{Float64: 100, Valid: true},
		90,
		startOfDay(now),
		30*24*time.Hour,
		now,
		startsAt,
		expiresAt,
	)

	require.NotNil(t, start)
	require.Equal(t, windowStart, *start)
	require.Equal(t, 90.0, used, "到期尾段不足完整月窗口时不应清零 monthly usage")
}

func TestNormalizeUsageBillingWindow_ExpiryTailMissingMonthlyWindowStaysNil(t *testing.T) {
	now := time.Date(2026, 5, 30, 0, 5, 0, 0, time.UTC)
	startsAt := time.Date(2026, 4, 30, 8, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 5, 30, 8, 0, 0, 0, time.UTC)

	start, used := normalizeUsageBillingWindow(
		sql.NullTime{},
		sql.NullFloat64{Float64: 100, Valid: true},
		90,
		startOfDay(now),
		30*24*time.Hour,
		now,
		startsAt,
		expiresAt,
	)

	require.Nil(t, start, "到期尾段不足完整月窗口时不应补写 monthly window_start")
	require.Equal(t, 90.0, used)
}

func TestNormalizeUsageBillingWindow_DailyCardDoesNotResetAfterMidnight(t *testing.T) {
	now := time.Date(2026, 5, 31, 0, 5, 0, 0, time.UTC)
	startsAt := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	expiresAt := startsAt.Add(24 * time.Hour)
	windowStart := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)

	start, used := normalizeUsageBillingWindow(
		sql.NullTime{Time: windowStart, Valid: true},
		sql.NullFloat64{Float64: 10, Valid: true},
		10,
		startOfDay(now),
		24*time.Hour,
		now,
		startsAt,
		expiresAt,
	)

	require.NotNil(t, start)
	require.Equal(t, windowStart, *start)
	require.Equal(t, 10.0, used, "1 日卡跨 0 点后不应刷新第二份 daily quota")
}

func TestNormalizeUsageBillingWindow_MultiDayDailyStillResetsWhenFullWindowFits(t *testing.T) {
	now := time.Date(2026, 5, 29, 0, 5, 0, 0, time.UTC)
	startsAt := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	windowStart := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	resetStart := startOfDay(now)

	start, used := normalizeUsageBillingWindow(
		sql.NullTime{Time: windowStart, Valid: true},
		sql.NullFloat64{Float64: 10, Valid: true},
		10,
		resetStart,
		24*time.Hour,
		now,
		startsAt,
		expiresAt,
	)

	require.NotNil(t, start)
	require.Equal(t, resetStart, *start)
	require.Equal(t, 0.0, used, "多日订阅仍应在可覆盖完整日窗口时重置 daily usage")
}
