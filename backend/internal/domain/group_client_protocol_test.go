package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupClientProtocolMatrix(t *testing.T) {
	tests := []struct {
		platform  string
		supported []GroupClientProtocol
		required  []GroupClientProtocol
	}{
		{PlatformAnthropic, []GroupClientProtocol{GroupClientProtocolAnthropicMessages, GroupClientProtocolOpenAIResponses, GroupClientProtocolOpenAIChatCompletions}, []GroupClientProtocol{GroupClientProtocolAnthropicMessages}},
		{PlatformOpenAI, []GroupClientProtocol{GroupClientProtocolAnthropicMessages, GroupClientProtocolOpenAIResponses, GroupClientProtocolOpenAIChatCompletions}, []GroupClientProtocol{GroupClientProtocolOpenAIResponses, GroupClientProtocolOpenAIChatCompletions}},
		{PlatformGemini, canonicalGroupClientProtocols, []GroupClientProtocol{GroupClientProtocolGeminiGenerateContent}},
		{PlatformAntigravity, canonicalGroupClientProtocols, []GroupClientProtocol{GroupClientProtocolAnthropicMessages, GroupClientProtocolGeminiGenerateContent}},
		{PlatformQoder, []GroupClientProtocol{GroupClientProtocolAnthropicMessages, GroupClientProtocolOpenAIResponses, GroupClientProtocolOpenAIChatCompletions}, []GroupClientProtocol{}},
		{PlatformGrok, []GroupClientProtocol{GroupClientProtocolAnthropicMessages, GroupClientProtocolOpenAIResponses, GroupClientProtocolOpenAIChatCompletions}, []GroupClientProtocol{GroupClientProtocolOpenAIResponses, GroupClientProtocolOpenAIChatCompletions}},
	}
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			require.Equal(t, tt.supported, SupportedGroupClientProtocols(tt.platform))
			require.Equal(t, tt.required, RequiredGroupClientProtocols(tt.platform))
		})
	}
}

func TestValidateGroupClientProtocols(t *testing.T) {
	validated, err := ValidateGroupClientProtocols(PlatformOpenAI, []GroupClientProtocol{
		GroupClientProtocolOpenAIChatCompletions,
		GroupClientProtocolAnthropicMessages,
		GroupClientProtocolOpenAIResponses,
	})
	require.NoError(t, err)
	require.Equal(t, []GroupClientProtocol{
		GroupClientProtocolAnthropicMessages,
		GroupClientProtocolOpenAIResponses,
		GroupClientProtocolOpenAIChatCompletions,
	}, validated)

	_, err = ValidateGroupClientProtocols(PlatformOpenAI, []GroupClientProtocol{GroupClientProtocolOpenAIResponses})
	require.ErrorContains(t, err, "cannot be disabled")
	_, err = ValidateGroupClientProtocols(PlatformAnthropic, []GroupClientProtocol{GroupClientProtocolAnthropicMessages, GroupClientProtocolGeminiGenerateContent})
	require.ErrorContains(t, err, "not supported")
	_, err = ValidateGroupClientProtocols(PlatformQoder, []GroupClientProtocol{GroupClientProtocolAnthropicMessages, GroupClientProtocolAnthropicMessages})
	require.ErrorContains(t, err, "duplicated")
	_, err = ValidateGroupClientProtocols(PlatformQoder, []GroupClientProtocol{"unknown"})
	require.ErrorContains(t, err, "unknown protocol")

	empty, err := ValidateGroupClientProtocols(PlatformQoder, []GroupClientProtocol{})
	require.NoError(t, err)
	require.NotNil(t, empty)
}

func TestLegacyGroupClientProtocols(t *testing.T) {
	require.False(t, HasGroupClientProtocol(LegacyGroupClientProtocols(PlatformOpenAI, false), GroupClientProtocolAnthropicMessages))
	require.True(t, HasGroupClientProtocol(LegacyGroupClientProtocols(PlatformOpenAI, true), GroupClientProtocolAnthropicMessages))
	require.Equal(t, canonicalGroupClientProtocols, LegacyGroupClientProtocols(PlatformGemini, false))
}
