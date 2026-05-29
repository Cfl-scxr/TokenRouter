package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type adminDataShareSettingRepoStub struct {
	values map[string]string
}

func (s *adminDataShareSettingRepoStub) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get call")
}

func (s *adminDataShareSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", service.ErrSettingNotFound
}

func (s *adminDataShareSettingRepoStub) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func (s *adminDataShareSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := s.values[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

func (s *adminDataShareSettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *adminDataShareSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *adminDataShareSettingRepoStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestParseAdminDataShareFiltersIncludesRequestPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/data-sharing?request_path=/v1/responses&user_agent=claude-code&model=claude-sonnet-4-5&search=/v1&user_name=alice&api_key_name=主Key&group_name=共享分组", nil)

	filters, ok := parseAdminDataShareFilters(c)
	require.True(t, ok)
	require.Equal(t, "/v1/responses", filters.RequestPath)
	require.Equal(t, "claude-code", filters.UserAgent)
	require.Equal(t, "claude-sonnet-4-5", filters.Model)
	require.Equal(t, "/v1", filters.Search)
	require.Equal(t, "alice", filters.UserName)
	require.Equal(t, "主Key", filters.APIKeyName)
	require.Equal(t, "共享分组", filters.GroupName)
}

func TestAdminDataShareSessionToResponseIncludesRequestPathAndUserAgent(t *testing.T) {
	resp := adminDataShareSessionToResponse(&service.DataShareSession{
		ID:              1,
		SessionID:       "sess_1",
		RequestPath:     "/v1/messages",
		UserAgent:       "claude-code/2.0",
		PayloadEncoding: "zstd",
		PayloadBytes:    5678,
	}, false)

	require.Equal(t, "/v1/messages", resp.RequestPath)
	require.Equal(t, "claude-code/2.0", resp.UserAgent)
	require.Equal(t, "zstd", resp.PayloadEncoding)
	require.Equal(t, int64(5678), resp.PayloadBytes)
}

func TestDataShareSkipRulesHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &adminDataShareSettingRepoStub{values: map[string]string{}}
	h := NewDataSharingHandler(service.NewDataSharingService(nil, repo))

	getRecorder := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRecorder)
	getCtx.Request = httptest.NewRequest(http.MethodGet, "/admin/data-sharing/skip-rules", nil)
	h.GetSkipRules(getCtx)
	require.Equal(t, http.StatusOK, getRecorder.Code)

	var getEnvelope struct {
		Code int                                `json:"code"`
		Data []service.DataShareCaptureSkipRule `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRecorder.Body.Bytes(), &getEnvelope))
	require.NotEmpty(t, getEnvelope.Data)

	body := bytes.NewBufferString(`{"rules":[{"id":"custom","name":"自定义","enabled":true,"request_paths":["v1/responses"],"field_scopes":["input"],"patterns":["Warmup"],"match_mode":"equals"}]}`)
	putRecorder := httptest.NewRecorder()
	putCtx, _ := gin.CreateTestContext(putRecorder)
	putCtx.Request = httptest.NewRequest(http.MethodPut, "/admin/data-sharing/skip-rules", body)
	putCtx.Request.Header.Set("Content-Type", "application/json")
	h.UpdateSkipRules(putCtx)
	require.Equal(t, http.StatusOK, putRecorder.Code)
	require.NotEmpty(t, repo.values[service.SettingKeyDataSharingCaptureSkipRules])
}

func TestCreateSessionExportTicketReturnsJSONFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &adminDataShareSettingRepoStub{values: map[string]string{}}
	h := NewDataSharingHandler(service.NewDataSharingService(nil, repo))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/data-sharing/sessions/7/export-ticket", nil)
	h.CreateSessionExportTicket(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Filename string `json:"filename"`
			Encoding string `json:"encoding"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Equal(t, "admin-data-sharing-session-7.json", envelope.Data.Filename)
	require.Equal(t, string(service.DataShareExportEncodingJSON), envelope.Data.Encoding)
}

func TestDataShareStorageLimitHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &adminDataShareSettingRepoStub{values: map[string]string{}}
	h := NewDataSharingHandler(service.NewDataSharingService(nil, repo))

	getRecorder := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRecorder)
	getCtx.Request = httptest.NewRequest(http.MethodGet, "/admin/data-sharing/storage-limit", nil)
	h.GetStorageLimit(getCtx)
	require.Equal(t, http.StatusOK, getRecorder.Code)

	var getEnvelope struct {
		Code int                           `json:"code"`
		Data service.DataShareStorageLimit `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRecorder.Body.Bytes(), &getEnvelope))
	require.False(t, getEnvelope.Data.Enabled)

	body := bytes.NewBufferString(`{"limit_bytes":1048576}`)
	putRecorder := httptest.NewRecorder()
	putCtx, _ := gin.CreateTestContext(putRecorder)
	putCtx.Request = httptest.NewRequest(http.MethodPut, "/admin/data-sharing/storage-limit", body)
	putCtx.Request.Header.Set("Content-Type", "application/json")
	h.UpdateStorageLimit(putCtx)
	require.Equal(t, http.StatusOK, putRecorder.Code)
	require.Equal(t, "1048576", repo.values[service.SettingKeyDataSharingStorageLimit])
}

func TestDataShareCaptureRuntimeSettingsHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &adminDataShareSettingRepoStub{values: map[string]string{}}
	pool := service.NewDataSharingCaptureWorkerPoolWithOptions(service.DataSharingCaptureWorkerPoolOptions{
		WorkerCount: 1,
		QueueSize:   1,
		TaskTimeout: 15 * time.Second,
	})
	t.Cleanup(pool.Stop)
	h := NewDataSharingHandler(service.NewDataSharingService(nil, repo, pool))

	getRecorder := httptest.NewRecorder()
	getCtx, _ := gin.CreateTestContext(getRecorder)
	getCtx.Request = httptest.NewRequest(http.MethodGet, "/admin/data-sharing/runtime-settings", nil)
	h.GetCaptureRuntimeSettings(getCtx)
	require.Equal(t, http.StatusOK, getRecorder.Code)

	var getEnvelope struct {
		Code int                                     `json:"code"`
		Data service.DataShareCaptureRuntimeSettings `json:"data"`
	}
	require.NoError(t, json.Unmarshal(getRecorder.Body.Bytes(), &getEnvelope))
	require.Equal(t, 15, getEnvelope.Data.TaskTimeoutSeconds)

	body := bytes.NewBufferString(`{"worker_count":3,"queue_size":8,"task_timeout_seconds":60}`)
	putRecorder := httptest.NewRecorder()
	putCtx, _ := gin.CreateTestContext(putRecorder)
	putCtx.Request = httptest.NewRequest(http.MethodPut, "/admin/data-sharing/runtime-settings", body)
	putCtx.Request.Header.Set("Content-Type", "application/json")
	h.UpdateCaptureRuntimeSettings(putCtx)
	require.Equal(t, http.StatusOK, putRecorder.Code)
	require.JSONEq(t, `{"worker_count":3,"queue_size":8,"task_timeout_seconds":60}`, repo.values[service.SettingKeyDataSharingCaptureRuntime])
	require.Equal(t, 3, pool.Stats().WorkerCount)
	require.Equal(t, 8, pool.Stats().QueueCapacity)
	require.Equal(t, 60, pool.Stats().TaskTimeoutSeconds)
}
