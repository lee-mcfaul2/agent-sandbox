package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lee-mcfaul2/agent-sandbox/internal/schemas"
)

type Catalog struct {
	Refs        []schemas.ToolRef
	OpenAITools []map[string]any
	index       map[string]struct{}
}

func (c *Catalog) Contains(mcp, tool string) bool {
	_, ok := c.index[mcp+"."+tool]
	return ok
}

func BuildCatalog(b *schemas.Bundle, entries []string) (*Catalog, error) {
	cat := &Catalog{index: make(map[string]struct{})}
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		dot := strings.Index(e, ".")
		if dot < 0 {
			return nil, fmt.Errorf("AVAILABLE_TOOLS entry %q missing '.' separator", e)
		}
		mcp, tool := e[:dot], e[dot+1:]
		if mcp == "" || tool == "" {
			return nil, fmt.Errorf("AVAILABLE_TOOLS entry %q has empty side", e)
		}
		raw, ok := b.RequestSchema(mcp, tool)
		if !ok {
			return nil, fmt.Errorf("no request schema for %s.%s", mcp, tool)
		}
		var paramSchema map[string]any
		if err := json.Unmarshal(raw, &paramSchema); err != nil {
			return nil, fmt.Errorf("unmarshal %s.%s request schema: %w", mcp, tool, err)
		}
		cat.Refs = append(cat.Refs, schemas.ToolRef{MCP: mcp, Tool: tool})
		cat.OpenAITools = append(cat.OpenAITools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        EncodeName(mcp, tool),
				"description": fmt.Sprintf("%s.%s", mcp, tool),
				"parameters":  paramSchema,
			},
		})
		cat.index[mcp+"."+tool] = struct{}{}
	}
	if len(cat.Refs) == 0 {
		return nil, fmt.Errorf("AVAILABLE_TOOLS parsed to zero tools")
	}
	return cat, nil
}
