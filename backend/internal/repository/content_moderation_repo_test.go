package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/stretchr/testify/require"
)

func TestContentModerationRepositoryCreateCyberWarningAndApplyUserBan_DisablesUserAtomically(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &contentModerationRepository{db: db}
	userID := int64(1001)
	accountID := int64(2001)
	createdAt := time.Now()
	warning := &service.ContentModerationCyberWarning{
		RequestID:      "req_1",
		UserID:         &userID,
		UserEmail:      "user@example.com",
		AccountID:      &accountID,
		AccountName:    "openai-1",
		Endpoint:       "/v1/responses",
		Model:          "gpt-5.1",
		UpstreamStatus: 400,
		WarningText:    "This request may pose a cybersecurity risk.",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM users WHERE id = $1 FOR UPDATE")).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(service.StatusActive))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO content_moderation_cyber_warnings")).
		WithArgs(
			warning.RequestID,
			userID,
			warning.UserEmail,
			nil,
			warning.APIKeyName,
			nil,
			warning.GroupName,
			accountID,
			warning.AccountName,
			warning.Endpoint,
			warning.Model,
			warning.UpstreamStatus,
			warning.WarningText,
			1,
			false,
			false,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(99), createdAt))
	mock.ExpectQuery(regexp.QuoteMeta("WITH last_auto_ban AS")).
		WithArgs(userID, sqlmock.AnyArg(), int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE users")).
		WithArgs(userID, service.StatusDisabled).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE content_moderation_cyber_warnings")).
		WithArgs(int64(99), 3, true).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	autoBanned, err := repo.CreateCyberWarningAndApplyUserBan(context.Background(), warning, service.ContentModerationCyberWarningPolicy{
		AutoBanEnabled: true,
		BanThreshold:   3,
		WindowHours:    720,
	})

	require.NoError(t, err)
	require.True(t, autoBanned)
	require.Equal(t, int64(99), warning.ID)
	require.Equal(t, createdAt, warning.CreatedAt)
	require.Equal(t, 3, warning.ViolationCount)
	require.True(t, warning.AutoBanned)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCreateCyberWarningAndApplyUserBan_KeepsBelowThreshold(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &contentModerationRepository{db: db}
	userID := int64(1001)
	warning := &service.ContentModerationCyberWarning{
		RequestID:      "req_1",
		UserID:         &userID,
		WarningText:    "flagged by OpenAI cyber policy",
		ViolationCount: 1,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT status FROM users WHERE id = $1 FOR UPDATE")).
		WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(service.StatusActive))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO content_moderation_cyber_warnings")).
		WithArgs(
			warning.RequestID,
			userID,
			warning.UserEmail,
			nil,
			warning.APIKeyName,
			nil,
			warning.GroupName,
			nil,
			warning.AccountName,
			warning.Endpoint,
			warning.Model,
			warning.UpstreamStatus,
			warning.WarningText,
			1,
			false,
			false,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(100), time.Now()))
	mock.ExpectQuery(regexp.QuoteMeta("WITH last_auto_ban AS")).
		WithArgs(userID, sqlmock.AnyArg(), int64(100)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE content_moderation_cyber_warnings")).
		WithArgs(int64(100), 2, false).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	autoBanned, err := repo.CreateCyberWarningAndApplyUserBan(context.Background(), warning, service.ContentModerationCyberWarningPolicy{
		AutoBanEnabled: true,
		BanThreshold:   3,
		WindowHours:    720,
	})

	require.NoError(t, err)
	require.False(t, autoBanned)
	require.Equal(t, 2, warning.ViolationCount)
	require.False(t, warning.AutoBanned)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryMarkCyberWarningEmailSent(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &contentModerationRepository{db: db}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE content_moderation_cyber_warnings")).
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.MarkCyberWarningEmailSent(context.Background(), 99))
	require.NoError(t, mock.ExpectationsWereMet())
}
