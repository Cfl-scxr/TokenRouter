package service

import "testing"

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
		Provider:      PlatformOpenAI,
		Model:         "gpt-5-alias",
		UpstreamModel: "gpt-5-2026-05-01",
		SessionID:     "session-1",
		RequestID:     "request-1",
		RequestBody:   []byte(`{"model":"gpt-5-alias","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"exec_command","description":"运行命令","parameters":{"type":"object"}}]}`),
		ResponseBody:  []byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`),
		InputTokens:   10,
		OutputTokens:  5,
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

	if len(session.Messages) != 4 {
		t.Fatalf("message count = %d, want 4: %#v", len(session.Messages), session.Messages)
	}
	if got := session.Messages[0]["role"]; got != "user" {
		t.Fatalf("first role = %v, want user", got)
	}
	if got := session.Messages[1]["role"]; got != "assistant" {
		t.Fatalf("function_call role = %v, want assistant", got)
	}
	if got := session.Messages[2]["role"]; got != "tool" {
		t.Fatalf("function_call_output role = %v, want tool", got)
	}
	if got := session.Messages[3]["role"]; got != "assistant" {
		t.Fatalf("response role = %v, want assistant", got)
	}
	if !session.Exportable {
		t.Fatalf("exportable = false, quality_errors = %v", session.QualityErrors)
	}
}
