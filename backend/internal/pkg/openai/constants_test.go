package openai

import "testing"

func TestDefaultModelsContainsCodexAutoReview(t *testing.T) {
	for _, model := range DefaultModels {
		if model.ID == "codex-auto-review" {
			return
		}
	}
	t.Fatal("默认 OpenAI 模型列表应包含 codex-auto-review")
}
