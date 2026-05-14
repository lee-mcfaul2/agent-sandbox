package loop

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnvelopeHappy(t *testing.T) {
	env := BuildEnvelope(EnvelopeInput{
		RequestUUID: "req-1",
		PromptUUID:  "pmt-1",
		Response:    "hello",
		HasResponse: true,
		Iterations:  2,
		Tools: []ToolCallSummary{
			{MCP: "kb", Tool: "search", Outcome: "ok"},
		},
		Model:        "claude-x",
		TokensUsed:   TokensUsed{Prompt: 100, Completion: 50, Total: 150},
		FinishReason: FinishTerminate,
	})
	raw, _ := json.Marshal(env)
	if !strings.Contains(string(raw), `"finish_reason":"terminate"`) {
		t.Errorf("missing finish_reason: %s", raw)
	}
	if !strings.Contains(string(raw), `"response":"hello"`) {
		t.Errorf("missing response: %s", raw)
	}
}

func TestEnvelopeWithError(t *testing.T) {
	env := BuildEnvelope(EnvelopeInput{
		RequestUUID:  "req-1",
		PromptUUID:   "pmt-1",
		FinishReason: FinishSchemaMismatch,
		Error: &ErrorBlock{
			Category: "SANDBOX_RESPONSE_SCHEMA_MISMATCH",
			MCP:      "kb",
			Tool:     "search",
		},
	})
	raw, _ := json.Marshal(env)
	if !strings.Contains(string(raw), `"category":"SANDBOX_RESPONSE_SCHEMA_MISMATCH"`) {
		t.Errorf("missing category: %s", raw)
	}
}

func TestEnvelopeSerializesAsOneLine(t *testing.T) {
	env := BuildEnvelope(EnvelopeInput{
		RequestUUID:  "req-1",
		PromptUUID:   "pmt-1",
		FinishReason: FinishTerminate,
	})
	raw, _ := json.Marshal(env)
	if strings.Contains(string(raw), "\n") {
		t.Error("envelope serialized with newline")
	}
}
