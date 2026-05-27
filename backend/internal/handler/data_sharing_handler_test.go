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
	c.Request = httptest.NewRequest(http.MethodGet, "/data-sharing?request_path=/v1/messages&search=/v1&api_key_name=测试Key&group_name=共享分组", nil)

	filters, ok := parseDataShareSessionFilters(c)
	require.True(t, ok)
	require.Equal(t, "/v1/messages", filters.RequestPath)
	require.Equal(t, "/v1", filters.Search)
	require.Equal(t, "测试Key", filters.APIKeyName)
	require.Equal(t, "共享分组", filters.GroupName)
}

func TestDataShareSessionToResponseIncludesRequestPath(t *testing.T) {
	resp := dataShareSessionToResponse(&service.DataShareSession{
		ID:          1,
		SessionID:   "sess_1",
		RequestPath: "/v1/chat/completions",
	}, false)

	require.Equal(t, "/v1/chat/completions", resp.RequestPath)
}
