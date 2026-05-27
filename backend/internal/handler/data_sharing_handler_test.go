package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseDataShareSessionFiltersIncludesRequestPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/data-sharing?request_path=/v1/messages&user_agent=codex-cli&model=gpt-5.5&search=/v1&api_key_name=测试Key&group_name=共享分组", nil)

	filters, ok := parseDataShareSessionFilters(c)
	require.True(t, ok)
	require.Equal(t, "/v1/messages", filters.RequestPath)
	require.Equal(t, "codex-cli", filters.UserAgent)
	require.Equal(t, "gpt-5.5", filters.Model)
	require.Equal(t, "/v1", filters.Search)
	require.Equal(t, "测试Key", filters.APIKeyName)
	require.Equal(t, "共享分组", filters.GroupName)
}

func TestDataShareSessionToResponseIncludesRequestPathAndUserAgent(t *testing.T) {
	resp := dataShareSessionToResponse(&service.DataShareSession{
		ID:              1,
		SessionID:       "sess_1",
		RequestPath:     "/v1/chat/completions",
		UserAgent:       "codex-cli/1.0",
		PayloadEncoding: "zstd",
		PayloadBytes:    1234,
	}, false)

	require.Equal(t, "/v1/chat/completions", resp.RequestPath)
	require.Equal(t, "codex-cli/1.0", resp.UserAgent)
	require.Equal(t, "zstd", resp.PayloadEncoding)
	require.Equal(t, int64(1234), resp.PayloadBytes)
}
