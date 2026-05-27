package service

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestBuildSessionUsesActualUpstreamModel(t *testing.T) {
	gid := int64(12)
	svc := NewDataSharingService(nil, nil)

	session := svc.buildSession(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group: &Group{
				ID:                 gid,
				Platform:           PlatformOpenAI,
				DataSharingEnabled: true,
			},
		},
		Provider:        PlatformOpenAI,
		Model:           "gpt-5-alias",
		UpstreamModel:   "gpt-5-2026-05-01",
		SessionID:       "session-1",
		RequestID:       "request-1",
		RequestBody:     []byte(`{"model":"gpt-5-alias","messages":[{"role":"system","content":"你是编码助手"},{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"exec_command","arguments":"{\"cmd\":\"ls\"}"}}]},{"role":"tool","tool_call_id":"call_1","content":"README.md"}],"tools":[{"type":"function","function":{"name":"exec_command","description":"运行命令","parameters":{"type":"object"}}}]}`),
		ResponseBody:    []byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"id":"resp_1"}`),
		InboundEndpoint: "v1/chat/completions",
		UserAgent:       "codex-cli/1.0",
		InputTokens:     10,
		OutputTokens:    5,
	})

	if session.Model != "gpt-5-2026-05-01" {
		t.Fatalf("model = %q, want actual upstream model", session.Model)
	}
	if got := session.SessionJSON["model"]; got != "gpt-5-2026-05-01" {
		t.Fatalf("session_json.model = %v, want actual upstream model", got)
	}
	if got := session.Meta["requested_model"]; got != "gpt-5-alias" {
		t.Fatalf("meta.requested_model = %v, want client requested model", got)
	}
	if got := session.RequestPath; got != "/v1/chat/completions" {
		t.Fatalf("request_path = %q, want normalized inbound path", got)
	}
	if got := session.SessionJSON["request_path"]; got != "/v1/chat/completions" {
		t.Fatalf("session_json.request_path = %v, want normalized inbound path", got)
	}
	if got := session.Meta["inbound_endpoint"]; got != "/v1/chat/completions" {
		t.Fatalf("meta.inbound_endpoint = %v, want normalized inbound path", got)
	}
	if got := session.UserAgent; got != "codex-cli" {
		t.Fatalf("user_agent = %q, want captured client user agent", got)
	}
	if got := session.SessionJSON["user_agent"]; got != "codex-cli" {
		t.Fatalf("session_json.user_agent = %v, want captured client user agent", got)
	}
	if got := session.Meta["user_agent"]; got != "codex-cli/1.0" {
		t.Fatalf("meta.user_agent = %v, want captured client user agent", got)
	}
	if got := session.Meta["user_agent_family"]; got != "codex-cli" {
		t.Fatalf("meta.user_agent_family = %v, want normalized client family", got)
	}
	if session.Exportable != true {
		t.Fatalf("exportable = false, quality_errors = %v", session.QualityErrors)
	}
}

func TestBuildSessionCapturesOpenAIResponsesInputAndOutput(t *testing.T) {
	gid := int64(12)
	svc := NewDataSharingService(nil, nil)

	session := svc.buildSession(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group: &Group{
				ID:                 gid,
				Platform:           PlatformOpenAI,
				DataSharingEnabled: true,
			},
		},
		Provider: PlatformOpenAI,
		Model:    "gpt-5.5",
		RequestBody: []byte(`{
			"model":"gpt-5.5",
				"input":[
					{"type":"message","role":"system","content":[{"type":"input_text","text":"你是编码助手"}]},
					{"type":"message","role":"user","content":[{"type":"input_text","text":"请列目录"}]},
					{"type":"function_call","call_id":"call_1","name":"exec_command","arguments":"{\"cmd\":\"ls\"}"},
					{"type":"function_call_output","call_id":"call_1","output":"README.md"}
				],
			"tools":[{"type":"function","name":"exec_command","description":"运行命令","parameters":{"type":"object"}}]
		}`),
		ResponseBody: []byte(`{
			"id":"resp_1",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"看到了 README.md"}]}],
			"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}
		}`),
		InputTokens:  10,
		OutputTokens: 5,
	})

	if len(session.Messages) != 5 {
		t.Fatalf("message count = %d, want 5: %#v", len(session.Messages), session.Messages)
	}
	if got := session.Messages[0]["role"]; got != "system" {
		t.Fatalf("first role = %v, want system", got)
	}
	if got := session.Messages[2]["role"]; got != "assistant" {
		t.Fatalf("function_call role = %v, want assistant", got)
	}
	if calls, ok := session.Messages[2]["tool_calls"].([]map[string]any); !ok || len(calls) != 1 || calls[0]["id"] != "call_1" || calls[0]["name"] != "exec_command" {
		t.Fatalf("tool_calls not normalized: %#v", session.Messages[2]["tool_calls"])
	}
	if got := session.Messages[3]["role"]; got != "tool" {
		t.Fatalf("function_call_output role = %v, want tool", got)
	}
	if got := session.Messages[3]["status"]; got != "success" {
		t.Fatalf("tool status = %v, want success", got)
	}
	if got := session.Messages[3]["is_error"]; got != false {
		t.Fatalf("tool is_error = %v, want false", got)
	}
	if got := session.Messages[4]["role"]; got != "assistant" {
		t.Fatalf("response role = %v, want assistant", got)
	}
	if session.SystemPrompt == nil || *session.SystemPrompt == "" {
		t.Fatalf("system_prompt missing")
	}
	if got := session.Meta["source_request_ids"]; got == nil {
		t.Fatalf("source_request_ids missing")
	}
	if !session.Exportable {
		t.Fatalf("exportable = false, quality_errors = %v", session.QualityErrors)
	}
	if session.QualityStatus != DataShareQualityComplete {
		t.Fatalf("quality_status = %q, want complete", session.QualityStatus)
	}
}

func TestBuildSessionCapturesAnthropicResponseBody(t *testing.T) {
	gid := int64(12)
	svc := NewDataSharingService(nil, nil)

	session := svc.buildSession(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group: &Group{
				ID:                 gid,
				Platform:           PlatformAnthropic,
				DataSharingEnabled: true,
			},
		},
		Provider: PlatformAnthropic,
		Model:    "claude-sonnet-4-5-20250929",
		RequestBody: []byte(`{
			"model":"claude-sonnet-4-5-20250929",
			"system":"你是编码助手",
			"messages":[
				{"role":"user","content":"列目录"},
				{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"exec_command","input":{"cmd":"ls"}}]},
				{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"README.md"}]}
			],
			"tools":[{"name":"exec_command","description":"运行命令","input_schema":{"type":"object"}}]
		}`),
		ResponseBody: []byte(`{
			"id":"msg_1",
			"type":"message",
			"role":"assistant",
			"content":[{"type":"text","text":"看到了 README.md"}],
			"usage":{"input_tokens":10,"output_tokens":5}
		}`),
		InboundEndpoint: "/v1/messages",
		InputTokens:     10,
		OutputTokens:    5,
	})

	if got := session.RequestPath; got != "/v1/messages" {
		t.Fatalf("request_path = %q, want /v1/messages", got)
	}
	if len(session.Messages) != 4 {
		t.Fatalf("message count = %d, want 4: %#v", len(session.Messages), session.Messages)
	}
	if got := session.Messages[1]["role"]; got != "assistant" {
		t.Fatalf("tool_use role = %v, want assistant", got)
	}
	calls, ok := session.Messages[1]["tool_calls"].([]map[string]any)
	if !ok || len(calls) != 1 || calls[0]["id"] != "toolu_1" || calls[0]["name"] != "exec_command" {
		t.Fatalf("tool_calls not normalized: %#v", session.Messages[1]["tool_calls"])
	}
	if got := session.Messages[2]["role"]; got != "tool" {
		t.Fatalf("tool_result role = %v, want tool", got)
	}
	if got := session.Messages[2]["tool_call_id"]; got != "toolu_1" {
		t.Fatalf("tool_call_id = %v, want toolu_1", got)
	}
	if got := session.Messages[3]["role"]; got != "assistant" {
		t.Fatalf("response role = %v, want assistant", got)
	}
	if got := dataShareContentText(session.Messages[3]["content"]); got != "看到了 README.md" {
		t.Fatalf("response content = %q, want assistant text", got)
	}
	if !session.Exportable {
		t.Fatalf("exportable = false, quality_errors = %v", session.QualityErrors)
	}
}

func TestAnthropicStreamAccumulatorBuildsFinalMessage(t *testing.T) {
	acc := &anthropicStreamResponseAccumulator{}
	acc.ObserveData("", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5-20250929","usage":{"input_tokens":10}}}`)
	acc.ObserveData("", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"exec_command","input":{}}}`)
	acc.ObserveData("", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"cmd\""}}`)
	acc.ObserveData("", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":":\"ls\"}"}}`)
	acc.ObserveData("", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`)
	acc.ObserveData("", `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"看到了 README.md"}}`)
	body := acc.ObserveData("", `{"type":"message_delta","usage":{"output_tokens":5},"delta":{"stop_reason":"end_turn"}}`)

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid accumulated body: %v", err)
	}
	messages := normalizeDataShareMessages([]map[string]any{got})
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1: %#v", len(messages), messages)
	}
	calls, ok := messages[0]["tool_calls"].([]map[string]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("tool_calls not normalized: %#v", messages[0]["tool_calls"])
	}
	if calls[0]["id"] != "toolu_1" || calls[0]["name"] != "exec_command" {
		t.Fatalf("tool call mismatch: %#v", calls[0])
	}
	args, ok := calls[0]["arguments"].(map[string]any)
	if !ok || args["cmd"] != "ls" {
		t.Fatalf("tool arguments mismatch: %#v", calls[0]["arguments"])
	}
	if got := dataShareContentText(got["content"]); got != "看到了 README.md" {
		t.Fatalf("content text = %q, want assistant text", got)
	}
}

func TestBuildSessionFiltersOrdinaryResponsesChat(t *testing.T) {
	gid := int64(12)
	svc := NewDataSharingService(nil, nil)

	session := svc.buildSession(DataShareCaptureInput{
		APIKey: &APIKey{
			ID:      34,
			UserID:  56,
			GroupID: &gid,
			Group: &Group{
				ID:                 gid,
				Platform:           PlatformOpenAI,
				DataSharingEnabled: true,
			},
		},
		Provider: PlatformOpenAI,
		Model:    "gpt-5.5",
		RequestBody: []byte(`{
			"model":"gpt-5.5",
			"input":[
				{"type":"message","role":"system","content":[{"type":"input_text","text":"你是编码助手"}]},
				{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}
			],
			"tools":[{"type":"function","name":"exec_command","description":"运行命令","parameters":{"type":"object"}}]
		}`),
		ResponseBody: []byte(`{
			"id":"resp_ordinary",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],
			"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}
		}`),
		InputTokens:  10,
		OutputTokens: 5,
	})

	if session.Exportable {
		t.Fatalf("ordinary no-tool session should not be exportable")
	}
	if session.QualityStatus != DataShareQualityInvalid {
		t.Fatalf("quality_status = %q, want invalid", session.QualityStatus)
	}
	if !containsString(session.QualityErrors, "missing_structured_tool_call") {
		t.Fatalf("quality_errors = %v, want missing_structured_tool_call", session.QualityErrors)
	}
}

func TestExportPayloadNormalizesLegacyRecord(t *testing.T) {
	sys := ""
	session := &DataShareSession{
		TrajectoryID:       "traj",
		SessionID:          "sess",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5.5",
		Status:             DataShareStatusCompleted,
		IsFinalSnapshot:    true,
		SourceRequestCount: 1,
		SystemPrompt:       &sys,
		Tools: []map[string]any{
			{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}, "type": "function"},
			{"name": "apply_patch", "description": "Use the apply_patch tool", "type": "custom"},
			{"name": "mcp__node_repl__", "description": "Node namespace", "type": "namespace", "tools": []any{
				map[string]any{"name": "js", "description": "运行 JavaScript", "parameters": map[string]any{"type": "object"}, "type": "function"},
			}},
			{"type": "web_search"},
		},
		Messages: []map[string]any{
			{"role": "system", "content": "你是编码助手"},
			{"role": "user", "content": "列目录"},
			{"role": "assistant", "type": "function_call", "call_id": "call_1", "name": "exec_command", "arguments": `{"cmd":"ls"}`},
			{"role": "tool", "type": "function_call_output", "call_id": "call_1", "output": "README.md\nProcess exited with code 0"},
			{"role": "assistant", "content": "看到了 README.md"},
		},
		Usage: map[string]any{"input_tokens": 10, "output_tokens": 5, "cache_read_tokens": 2, "total_tokens": 17},
		Meta:  map[string]any{"request_id": "req_1"},
	}

	payload := exportPayloadFromSession(session)
	if got := payload["system_prompt"]; got != "你是编码助手" {
		t.Fatalf("system_prompt = %v", got)
	}
	tools := payload["tools"].([]map[string]any)
	for _, tool := range tools {
		if tool["name"] == "" || tool["description"] == "" || tool["parameters"] == nil {
			t.Fatalf("invalid tool after normalize: %#v", tool)
		}
	}
	messages := payload["messages"].([]map[string]any)
	calls := messages[2]["tool_calls"].([]map[string]any)
	if calls[0]["id"] != "call_1" || calls[0]["name"] != "exec_command" {
		t.Fatalf("tool call not normalized: %#v", calls[0])
	}
	if messages[3]["status"] != "success" || messages[3]["is_error"] != false {
		t.Fatalf("tool result not normalized: %#v", messages[3])
	}
	meta := payload["meta"].(map[string]any)
	sourceIDs := meta["source_request_ids"].([]string)
	if len(sourceIDs) != 1 || sourceIDs[0] != "req_1" {
		t.Fatalf("source_request_ids = %#v", sourceIDs)
	}
	if errs := validateDataSharePayloadQuality(payload); len(errs) != 0 {
		t.Fatalf("payload quality errors = %v", errs)
	}
}

func TestExportPayloadDedupesRepeatedResponsesHistory(t *testing.T) {
	sys := "你是编码助手"
	session := &DataShareSession{
		TrajectoryID:       "traj",
		SessionID:          "sess",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5.5",
		Status:             DataShareStatusCompleted,
		IsFinalSnapshot:    true,
		SourceRequestCount: 7,
		SystemPrompt:       &sys,
		Tools: []map[string]any{
			{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}, "type": "function"},
		},
		Messages: []map[string]any{
			{"role": "system", "content": sys},
			{"role": "user", "content": "列目录"},
			{"role": "assistant", "content": "", "finish_reason": "tool_calls", "tool_calls": []map[string]any{{"id": "call_dup", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}},
			{"role": "tool", "tool_call_id": "call_dup", "content": "README.md", "status": "success", "is_error": false},
			{"role": "assistant", "content": "", "finish_reason": "tool_calls", "tool_calls": []map[string]any{{"id": "call_dup", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}},
			{"role": "tool", "tool_call_id": "call_dup", "content": "README.md", "status": "success", "is_error": false},
			{"role": "assistant", "content": "看到了 README.md"},
		},
		Usage: map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
		Meta:  map[string]any{"source_request_ids": []any{"req_1", "req_2"}},
	}

	payload := exportPayloadFromSession(session)
	messages := payload["messages"].([]map[string]any)
	callCount := 0
	resultCount := 0
	for _, msg := range messages {
		callCount += len(anySlice(msg["tool_calls"]))
		if msg["role"] == "tool" {
			resultCount++
		}
	}
	if callCount != 1 || resultCount != 1 {
		t.Fatalf("dedupe failed, callCount=%d resultCount=%d messages=%#v", callCount, resultCount, messages)
	}
	if errs := validateDataSharePayloadQuality(payload); len(errs) != 0 {
		t.Fatalf("payload quality errors = %v", errs)
	}
}

func TestCompactDataShareMessagesKeepsUnfinishedTail(t *testing.T) {
	sys := map[string]any{"role": "system", "content": "你是编码助手"}
	user := map[string]any{"role": "user", "content": "列目录"}
	firstCall := map[string]any{"role": "assistant", "content": "", "finish_reason": "tool_calls", "tool_calls": []map[string]any{{"id": "call_1", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}}
	firstResult := map[string]any{"role": "tool", "tool_call_id": "call_1", "content": "README.md", "status": "success", "is_error": false}
	final := map[string]any{"role": "assistant", "content": "看到了 README.md"}
	tailCall := map[string]any{"role": "assistant", "content": "", "finish_reason": "tool_calls", "tool_calls": []map[string]any{{"id": "call_2", "name": "exec_command", "arguments": map[string]any{"cmd": "pwd"}}}}
	messages := []map[string]any{sys, user, firstCall, firstResult, sys, user, firstCall, firstResult, final, tailCall}

	compact := CompactDataShareMessages(messages)
	if len(compact) != 6 {
		t.Fatalf("compact len = %d, want 6: %#v", len(compact), compact)
	}
	if compact[5]["tool_calls"] == nil {
		t.Fatalf("unfinished tail call should be retained")
	}
	errs := ValidateDataShareSessionQuality("gpt-5.5", "你是编码助手", compact, []map[string]any{{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}}}, map[string]any{"total_tokens": 1})
	if !containsString(errs, "tool_call_result_unpaired") {
		t.Fatalf("quality_errors = %v, want unfinished tail to remain unpaired", errs)
	}
}

func TestPartialSessionExportsRawSnapshot(t *testing.T) {
	sys := "你是编码助手"
	session := &DataShareSession{
		TrajectoryID:       "traj",
		SessionID:          "sess",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5.5",
		Status:             DataShareStatusTerminated,
		IsFinalSnapshot:    false,
		SourceRequestCount: 1,
		SystemPrompt:       &sys,
		Tools: []map[string]any{
			{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}},
		},
		Messages: []map[string]any{
			{"role": "system", "content": sys},
			{"role": "user", "content": "列目录"},
			{"role": "assistant", "tool_calls": []map[string]any{{"id": "call_1", "name": "exec_command", "arguments": map[string]any{"cmd": "ls"}}}},
			{"role": "tool", "tool_call_id": "call_1", "content": "README.md", "status": "success", "is_error": false},
			{"role": "assistant", "content": "看到了 README.md"},
			{"role": "assistant", "tool_calls": []map[string]any{{"id": "call_tail", "name": "exec_command", "arguments": map[string]any{"cmd": "pwd"}}}},
		},
		Usage: map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
		Meta:  map[string]any{"source_request_ids": []string{"req_1"}},
	}
	status := DataSharePayloadQualityStatus(session.Model, sys, session.Messages, session.Tools, session.Usage)
	if status != DataShareQualityPartial {
		t.Fatalf("quality_status = %q, want partial", status)
	}

	var buf bytes.Buffer
	if err := WriteSingleSessionJSONL(&buf, session); err != nil {
		t.Fatalf("WriteSingleSessionJSONL returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &payload); err != nil {
		t.Fatalf("invalid jsonl: %v", err)
	}
	messages := mapsFromAny(payload["messages"])
	if len(messages) != 6 {
		t.Fatalf("exported messages len = %d, want raw len 6: %#v", len(messages), messages)
	}
	if got := payload["status"]; got != DataShareStatusTerminated {
		t.Fatalf("exported status = %v, want terminated", got)
	}
	if got := payload["is_final_snapshot"]; got != false {
		t.Fatalf("exported is_final_snapshot = %v, want false", got)
	}
	if _, ok := payload["quality_status"]; ok {
		t.Fatalf("quality_status should not be included in JSONL payload")
	}
	if errs := validateDataSharePayloadQuality(payload); !containsString(errs, "tool_call_result_unpaired") {
		t.Fatalf("payload quality errors = %v, want unpaired tail retained", errs)
	}
}

func TestInvalidSessionCanExportWhenSelected(t *testing.T) {
	sys := "你是编码助手"
	session := &DataShareSession{
		TrajectoryID:       "traj",
		SessionID:          "sess",
		Dataset:            defaultDataShareDataset,
		Provider:           PlatformOpenAI,
		Model:              "gpt-5.5",
		Status:             DataShareStatusTerminated,
		IsFinalSnapshot:    false,
		SourceRequestCount: 1,
		SystemPrompt:       &sys,
		Tools:              []map[string]any{{"name": "exec_command", "description": "运行命令", "parameters": map[string]any{"type": "object"}}},
		Messages: []map[string]any{
			{"role": "system", "content": sys},
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "hello"},
		},
		Usage: map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
	}
	if status := DataSharePayloadQualityStatus(session.Model, sys, session.Messages, session.Tools, session.Usage); status != DataShareQualityInvalid {
		t.Fatalf("quality_status = %q, want invalid", status)
	}
	var buf bytes.Buffer
	if err := WriteSingleSessionJSONL(&buf, session); err != nil {
		t.Fatalf("WriteSingleSessionJSONL returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &payload); err != nil {
		t.Fatalf("invalid jsonl: %v", err)
	}
	if got := payload["session_id"]; got != "sess" {
		t.Fatalf("session_id = %v, want sess", got)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
