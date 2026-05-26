package schemas

import (
	"encoding/json"
	"testing"
)

func TestCompileAllValidators(t *testing.T) {
	b, _ := LoadEmbedded()
	reg, err := CompileValidators(b, []ToolRef{
		{MCP: "agent-sql-mcp", Tool: "list_orders"},
		{MCP: "agent-sql-mcp", Tool: "lookup_customer"},
	})
	if err != nil {
		t.Fatalf("CompileValidators: %v", err)
	}
	if reg.Response("agent-sql-mcp", "list_orders") == nil {
		t.Error("missing agent-sql-mcp.list_orders response validator")
	}
	if reg.Response("agent-sql-mcp", "lookup_customer") == nil {
		t.Error("missing agent-sql-mcp.lookup_customer response validator")
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
	reg, _ := CompileValidators(b, []ToolRef{{MCP: "agent-sql-mcp", Tool: "list_orders"}})
	v := reg.Response("agent-sql-mcp", "list_orders")
	doc := map[string]any{
		"orders": []any{
			map[string]any{
				"id":          1,
				"customer_id": 1,
				"status":      "placed",
				"total_cents": 1000,
				"currency":    "USD",
				"placed_at":   "2026-05-26T00:00:00Z",
			},
		},
	}
	raw, _ := json.Marshal(doc)
	if err := v.Validate(raw); err != nil {
		t.Fatalf("Validate happy: %v", err)
	}
}

func TestValidateResponseBad(t *testing.T) {
	b, _ := LoadEmbedded()
	reg, _ := CompileValidators(b, []ToolRef{{MCP: "agent-sql-mcp", Tool: "list_orders"}})
	v := reg.Response("agent-sql-mcp", "list_orders")
	doc := map[string]any{"orders": "not-an-array"}
	raw, _ := json.Marshal(doc)
	if err := v.Validate(raw); err == nil {
		t.Fatal("expected validation error")
	}
}
