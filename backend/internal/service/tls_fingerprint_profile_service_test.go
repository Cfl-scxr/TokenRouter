package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSFingerprintProfileService_ResolveTLSProfileOpenAI(t *testing.T) {
	svc := &TLSFingerprintProfileService{}

	openAIOAuth := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"enable_tls_fingerprint": true},
	}
	require.NotNil(t, svc.ResolveTLSProfile(openAIOAuth), "OpenAI OAuth 开启后应返回内置默认 profile")

	openAIAPIKey := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{"enable_tls_fingerprint": true},
	}
	require.Nil(t, svc.ResolveTLSProfile(openAIAPIKey), "OpenAI API Key 不应启用 TLS 指纹伪装")
}
