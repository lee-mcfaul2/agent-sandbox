package schemas

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// ToolRef identifies a single MCP tool for schema compilation.
type ToolRef struct {
	MCP  string
	Tool string
}

// Validator wraps a compiled JSON Schema and validates raw JSON bytes against it.
type Validator struct {
	schema *jsonschema.Schema
}

// Validate parses raw and validates it against the compiled schema.
func (v *Validator) Validate(raw []byte) error {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("unmarshal for validation: %w", err)
	}
	return v.schema.Validate(doc)
}

// Registry holds compiled request and response validators keyed by "mcp/tool".
type Registry struct {
	requests  map[string]*Validator
	responses map[string]*Validator
}

// Request returns the compiled request validator for the given mcp+tool pair,
// or nil if it was not compiled.
func (r *Registry) Request(mcp, tool string) *Validator {
	return r.requests[mcp+"/"+tool]
}

// Response returns the compiled response validator for the given mcp+tool pair,
// or nil if it was not compiled.
func (r *Registry) Response(mcp, tool string) *Validator {
	return r.responses[mcp+"/"+tool]
}

// CompileValidators compiles request and response JSON Schema validators for
// every ToolRef against the provided Bundle. It returns an error if any
// referenced tool is absent from the bundle or if schema compilation fails.
func CompileValidators(b *Bundle, refs []ToolRef) (*Registry, error) {
	reg := &Registry{
		requests:  make(map[string]*Validator),
		responses: make(map[string]*Validator),
	}
	for _, ref := range refs {
		reqRaw, ok := b.RequestSchema(ref.MCP, ref.Tool)
		if !ok {
			return nil, fmt.Errorf("missing request schema for %s.%s", ref.MCP, ref.Tool)
		}
		respRaw, ok := b.ResponseSchema(ref.MCP, ref.Tool)
		if !ok {
			return nil, fmt.Errorf("missing response schema for %s.%s", ref.MCP, ref.Tool)
		}
		reqVal, err := compileOne(ref.MCP+"/"+ref.Tool+".request", reqRaw)
		if err != nil {
			return nil, err
		}
		respVal, err := compileOne(ref.MCP+"/"+ref.Tool+".response", respRaw)
		if err != nil {
			return nil, err
		}
		reg.requests[ref.MCP+"/"+ref.Tool] = reqVal
		reg.responses[ref.MCP+"/"+ref.Tool] = respVal
	}
	return reg, nil
}

// compileOne parses and compiles a single JSON Schema document identified by id.
func compileOne(id string, raw []byte) (*Validator, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", id, err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, doc); err != nil {
		return nil, fmt.Errorf("add %s: %w", id, err)
	}
	s, err := c.Compile(id)
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", id, err)
	}
	return &Validator{schema: s}, nil
}
