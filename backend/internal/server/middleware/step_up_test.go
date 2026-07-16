package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStepUpSessionKeyUsesSessionID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	c.Set(ContextKeySessionID, "family-1")
	if got := StepUpSessionKey(c, 42); got != "family-1" {
		t.Fatalf("StepUpSessionKey() = %q, want family-1", got)
	}
}

func TestStepUpSessionKeySeparatesLegacyTokens(t *testing.T) {
	keyFor := func(token string) string {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", nil)
		c.Request.Header.Set("Authorization", "Bearer "+token)
		return StepUpSessionKey(c, 42)
	}

	first := keyFor("legacy-token-a")
	if first != keyFor("legacy-token-a") {
		t.Fatal("同一个旧 token 应生成稳定的会话键")
	}
	if first == keyFor("legacy-token-b") {
		t.Fatal("不同旧 token 不应共享 step-up 会话键")
	}
}

func TestStepUpSessionKeyFallsBackWithoutCredential(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	if got := StepUpSessionKey(c, 42); got != "u42" {
		t.Fatalf("StepUpSessionKey() = %q, want u42", got)
	}
}
