package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
		ID:          1,
		SessionID:   "sess_1",
		RequestPath: "/v1/messages",
		UserAgent:   "claude-code/2.0",
	}, false)

	require.Equal(t, "/v1/messages", resp.RequestPath)
	require.Equal(t, "claude-code/2.0", resp.UserAgent)
}
