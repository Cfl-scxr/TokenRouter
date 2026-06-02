package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func collectStreamEvents(t *testing.T, chunks []string) []ResponsesStreamEvent {
	t.Helper()
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-pro")
	var events []ResponsesStreamEvent
	for _, payload := range chunks {
		var chunk ChatCompletionsChunk
		require.NoError(t, json.Unmarshal([]byte(payload), &chunk))
		events = append(events, ChatCompletionsChunkToResponsesEvents(&chunk, state)...)
	}
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)
	return events
}

// TestStream_ReasoningOpensItemBeforeDelta 防止严格客户端丢弃指向未打开 item
// 的 reasoning delta。
func TestStream_ReasoningOpensItemBeforeDelta(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","content":null,"reasoning_content":""}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_content":"think"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		`{"choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	open := map[int]string{} // output_index 到 item type 的映射
	for _, e := range events {
		switch e.Type {
		case "response.output_item.added":
			require.NotNil(t, e.Item)
			open[e.OutputIndex] = e.Item.Type
		case "response.reasoning_summary_text.delta":
			require.Equalf(t, "reasoning", open[e.OutputIndex], "reasoning delta before its item was opened")
		case "response.output_text.delta":
			require.Equalf(t, "message", open[e.OutputIndex], "text delta before its item was opened")
		}
	}
}

// TestStream_ToolCallLifecycleComplete 确保工具调用会完整关闭，Codex 执行工具
// 前需要收到 function_call_arguments.done 和 output_item.done。
func TestStream_ToolCallLifecycleComplete(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"plan"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"exec","arguments":""}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"cmd\":\"ls\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
	})

	var sawAdded, sawArgsDone, sawItemDone bool
	for _, e := range events {
		switch e.Type {
		case "response.output_item.added":
			if e.Item != nil && e.Item.Type == "function_call" {
				sawAdded = true
			}
		case "response.function_call_arguments.done":
			sawArgsDone = true
			require.Equal(t, `{"cmd":"ls"}`, e.Arguments)
		case "response.output_item.done":
			if e.Item != nil && e.Item.Type == "function_call" {
				sawItemDone = true
				require.Equal(t, `{"cmd":"ls"}`, e.Item.Arguments)
				require.Equal(t, "call_a", e.Item.CallID)
			}
		}
	}
	require.True(t, sawAdded, "function_call output_item.added missing")
	require.True(t, sawArgsDone, "function_call_arguments.done missing")
	require.True(t, sawItemDone, "function_call output_item.done missing")
}

// TestStream_SSEWireComplete 让完整流经过 SSE 编码，确认 function_call 事件在
// wire 上携带完整字段。
func TestStream_SSEWireComplete(t *testing.T) {
	events := collectStreamEvents(t, []string{
		`{"choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"plan"}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"exec","arguments":"{}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
	})

	var addedLine string
	for _, e := range events {
		sse, err := ResponsesEventToSSE(e)
		require.NoError(t, err)
		if e.Type == "response.output_item.added" && e.Item != nil && e.Item.Type == "function_call" {
			addedLine = sse
		}
	}
	require.NotEmpty(t, addedLine)
	// function_call added 事件在 wire 上必须带 arguments:""。
	require.True(t, strings.Contains(addedLine, `"arguments":""`), "added line missing arguments: %s", addedLine)
	require.Contains(t, addedLine, `"call_id":"call_a"`)
}
