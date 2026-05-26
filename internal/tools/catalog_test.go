package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lee-mcfaul2/agent-sandbox/internal/schemas"
)

func TestBuildCatalog(t *testing.T) {
	b, _ := schemas.LoadEmbedded()
	cat, err := BuildCatalog(b, []string{"agent-sql-mcp.list_orders", "agent-sql-mcp.lookup_customer"})
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	if len(cat.OpenAITools) != 2 {
		t.Errorf("OpenAITools len = %d", len(cat.OpenAITools))
	}
	if len(cat.Refs) != 2 {
		t.Errorf("Refs len = %d", len(cat.Refs))
	}

	first := cat.OpenAITools[0]
	if first["type"] != "function" {
		t.Errorf("type = %v", first["type"])
	}
	fn := first["function"].(map[string]any)
	if fn["name"] != "agent-sql-mcp__list_orders" && fn["name"] != "agent-sql-mcp__lookup_customer" {
		t.Errorf("function name = %v", fn["name"])
	}
}

func TestBuildCatalogUnknownTool(t *testing.T) {
	b, _ := schemas.LoadEmbedded()
	if _, err := BuildCatalog(b, []string{"nope.absent"}); err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestBuildCatalogMalformedEntry(t *testing.T) {
	b, _ := schemas.LoadEmbedded()
	if _, err := BuildCatalog(b, []string{"missing_dot"}); err == nil {
		t.Fatal("expected error for entry without dot")
	}
}

func TestCatalogContains(t *testing.T) {
	b, _ := schemas.LoadEmbedded()
	cat, _ := BuildCatalog(b, []string{"agent-sql-mcp.list_orders"})
	if !cat.Contains("agent-sql-mcp", "list_orders") {
		t.Error("expected to contain agent-sql-mcp.list_orders")
	}
	if cat.Contains("agent-sql-mcp", "get_order") {
		t.Error("did not expect agent-sql-mcp.get_order")
	}
}

// The OpenAI tool format presents description text to the LLM as the only
// semantic hint about what a tool does. Using the bare "mcp.tool" identifier
// as the description gives the model nothing -- it then guesses tool wiring
// from the parameter schema alone and routinely passes the wrong arg type
// (e.g. a customer name where an integer customer_id is required), because
// it has no way to know it must call search_customer first to convert a
// name to an id. Description text MUST come from the bundle's per-tool
// meta.json, not from a placeholder.
func TestBuildCatalogUsesBundleDescription(t *testing.T) {
	b, err := schemas.LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	cat, err := BuildCatalog(b, []string{"agent-sql-mcp.search_customer"})
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	fn, _ := cat.OpenAITools[0]["function"].(map[string]any)
	desc, _ := fn["description"].(string)
	if desc == "" {
		t.Fatal("function.description is empty -- bundle meta.json must provide one")
	}
	if desc == "agent-sql-mcp.search_customer" || desc == "agent-sql-mcp/search_customer" {
		t.Errorf("function.description = %q is still the mcp.tool placeholder; "+
			"the LLM can't infer tool wiring from the name alone", desc)
	}
	// Shape check: search_customer's description should make clear it
	// converts a name/email/phone into a customer_id, so the model can
	// chain it before list_orders / lookup_customer / list_transactions.
	low := strings.ToLower(desc)
	if !strings.Contains(low, "customer_id") {
		t.Errorf("search_customer description should mention customer_id so the "+
			"model knows the tool's output is what other tools need as input; got %q", desc)
	}
}

func TestOpenAIToolsParametersAreJSON(t *testing.T) {
	b, _ := schemas.LoadEmbedded()
	cat, _ := BuildCatalog(b, []string{"agent-sql-mcp.list_orders"})
	raw, err := json.Marshal(cat.OpenAITools)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("empty tools json")
	}
}
