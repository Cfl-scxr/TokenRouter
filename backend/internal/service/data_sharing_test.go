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
