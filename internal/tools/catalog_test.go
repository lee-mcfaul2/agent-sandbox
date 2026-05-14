package tools

import (
	"encoding/json"
	"testing"

	"github.com/lee-mcfaul2/agent-sandbox/internal/schemas"
)

func TestBuildCatalog(t *testing.T) {
	b, _ := schemas.LoadEmbedded()
	cat, err := BuildCatalog(b, []string{"kb.search", "audit_db.search"})
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
	if fn["name"] != "kb__search" && fn["name"] != "audit_db__search" {
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
	cat, _ := BuildCatalog(b, []string{"kb.search"})
	if !cat.Contains("kb", "search") {
		t.Error("expected to contain kb.search")
	}
	if cat.Contains("kb", "fetch") {
		t.Error("did not expect kb.fetch")
	}
}

func TestOpenAIToolsParametersAreJSON(t *testing.T) {
	b, _ := schemas.LoadEmbedded()
	cat, _ := BuildCatalog(b, []string{"kb.search"})
	raw, err := json.Marshal(cat.OpenAITools)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("empty tools json")
	}
}
