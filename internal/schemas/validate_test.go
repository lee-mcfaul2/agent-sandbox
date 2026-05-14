package schemas

import (
	"encoding/json"
	"testing"
)

func TestCompileAllValidators(t *testing.T) {
	b, _ := LoadEmbedded()
	reg, err := CompileValidators(b, []ToolRef{
		{MCP: "kb", Tool: "search"},
		{MCP: "audit_db", Tool: "search"},
	})
	if err != nil {
		t.Fatalf("CompileValidators: %v", err)
	}
	if reg.Response("kb", "search") == nil {
		t.Error("missing kb.search response validator")
	}
	if reg.Response("audit_db", "search") == nil {
		t.Error("missing audit_db.search response validator")
	}
}

func TestCompileUnknownTool(t *testing.T) {
	b, _ := LoadEmbedded()
	_, err := CompileValidators(b, []ToolRef{{MCP: "nope", Tool: "x"}})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestValidateResponseHappy(t *testing.T) {
	b, _ := LoadEmbedded()
	reg, _ := CompileValidators(b, []ToolRef{{MCP: "kb", Tool: "search"}})
	v := reg.Response("kb", "search")
	doc := map[string]any{
		"rows": []any{
			map[string]any{"id": "1", "title": "x"},
		},
	}
	raw, _ := json.Marshal(doc)
	if err := v.Validate(raw); err != nil {
		t.Fatalf("Validate happy: %v", err)
	}
}

func TestValidateResponseBad(t *testing.T) {
	b, _ := LoadEmbedded()
	reg, _ := CompileValidators(b, []ToolRef{{MCP: "kb", Tool: "search"}})
	v := reg.Response("kb", "search")
	doc := map[string]any{"rows": "not-an-array"}
	raw, _ := json.Marshal(doc)
	if err := v.Validate(raw); err == nil {
		t.Fatal("expected validation error")
	}
}
