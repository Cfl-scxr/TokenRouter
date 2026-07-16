package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type partialErrorBody struct {
	data  []byte
	first bool
}

func (b *partialErrorBody) Read(p []byte) (int, error) {
	if len(b.data) == 0 {
		return 0, io.EOF
	}
	limit := len(b.data)
	if !b.first && limit > 5 {
		limit = 5
	}
	n := copy(p, b.data[:limit])
	b.data = b.data[n:]
	if !b.first {
		b.first = true
		return n, errors.New("injected read error")
	}
	return n, nil
}

func (b *partialErrorBody) Close() error { return nil }

func TestDeriveAuditAction(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   string
	}{
		{"PUT", "/api/v1/admin/accounts/:id", "admin.accounts.update"},
		{"POST", "/api/v1/admin/accounts", "admin.accounts.create"},
		{"DELETE", "/api/v1/admin/backups/:id", "admin.backups.delete"},
		{"GET", "/api/v1/admin/users/:id/api-keys", "admin.users.api_keys.read"},
		{"POST", "/api/v1/admin/redeem-codes/batch", "admin.redeem_codes.batch.create"},
	}
	for _, tc := range cases {
		if got := deriveAuditAction(tc.method, tc.path); got != tc.want {
			t.Fatalf("deriveAuditAction(%q, %q) = %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestAuditSensitiveReadsIncludesForkBackupRoutes(t *testing.T) {
	want := map[string]string{
		"GET /api/v1/admin/backups/:id/download":   "admin.backups.download",
		"GET /api/v1/admin/backups/storage-config": "admin.backups.storage_config.read",
	}
	for route, action := range want {
		if got := auditSensitiveReads[route]; got != action {
			t.Fatalf("auditSensitiveReads[%q] = %q, want %q", route, got, action)
		}
	}
}

func TestAuditMiddlewareRestoresPartialBodyAfterReadError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	want := []byte(`{"name":"完整请求"}`)
	var got []byte

	router := gin.New()
	router.POST("/api/v1/admin/test", gin.HandlerFunc(NewAuditLogMiddleware(nil)), func(c *gin.Context) {
		var err error
		got, err = io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/test", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = &partialErrorBody{data: append([]byte(nil), want...)}
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	if string(got) != string(want) {
		t.Fatalf("restored body = %q, want %q", got, want)
	}
}
