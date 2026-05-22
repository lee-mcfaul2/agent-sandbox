package schemas

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// embeddedFS holds the lib-agent-prompt bundle the binary ships with.
// In dev/test, `make fixtures` copies internal/schemas/testdata/lib-agent-prompt/
// into internal/schemas/embedded/ before go test/build.
// In prod, the Dockerfile populates the same path from the cosign-verified
// OCI artifact before go build. Either way, this is the single source of
// truth the binary embeds.

//go:embed all:embedded/lib-agent-prompt
var embeddedFS embed.FS

const embeddedRoot = "embedded/lib-agent-prompt"

// Bundle is a loaded set of JSON Schema documents with a deterministic digest.
type Bundle struct {
	Digest  string            // "sha256:<hex>"
	Schemas map[string][]byte // "<mcp>/<tool>.<direction>" → raw bytes
}

// LoadEmbedded loads the bundle from the binary's embedded data.
// Run `make fixtures` if you get "no schemas found" — the embed target
// must be populated at build time.
func LoadEmbedded() (*Bundle, error) {
	return loadFrom(embeddedFS, embeddedRoot)
}

// loadFrom reads the bundle layout under root and computes a digest that
// MUST match the gateway-side computation in lib-agent-prompt (currently
// implemented in agent-gateway/src/ag_gateway/prompts/bundle_view.py).
//
// Expected layout (mirrors the demo bundle in secure-agent-demo):
//
//	<root>/bundle-manifest.json
//	<root>/schemas/user-prompt.json
//	<root>/schemas/final-response.json
//	<root>/schemas/tool-result.json
//	<root>/schemas/shared/*.json
//	<root>/schemas/services/<mcp>/<tool>.{request,response,meta}.json
//
// Digest algorithm (gateway-compatible):
//
//	h = SHA256
//	for envelope in [user-prompt, final-response, tool-result]:
//	  h.write(envelope_bytes); h.write(0x00)
//	for key in sorted(services keys), where key = "<mcp>/<tool>.<direction>":
//	  h.write(schema_bytes); h.write(0x00)
//	return "sha256:" + hex
//
// Natural alphabetic sort of "<mcp>/<tool>.<direction>" keys matches the
// gateway's "sort by mcp dir, sort by tool basename, request before response"
// iteration because '.request' < '.response' lexicographically.
func loadFrom(fsys fs.FS, root string) (*Bundle, error) {
	envelopeNames := []string{"user-prompt.json", "final-response.json", "tool-result.json"}
	envelopeBytes := make([][]byte, 0, len(envelopeNames))
	for _, name := range envelopeNames {
		b, err := fs.ReadFile(fsys, root+"/schemas/"+name)
		if err != nil {
			return nil, fmt.Errorf("read envelope %s: %w", name, err)
		}
		envelopeBytes = append(envelopeBytes, b)
	}

	schemas := make(map[string][]byte)
	servicesRoot := root + "/schemas/services"
	err := fs.WalkDir(fsys, servicesRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".request.json") && !strings.HasSuffix(p, ".response.json") {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, servicesRoot+"/")
		rel = strings.TrimSuffix(rel, ".json")
		schemas[rel] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk bundle %s: %w", servicesRoot, err)
	}
	if len(schemas) == 0 {
		return nil, fmt.Errorf("no schemas found under %s; run `make fixtures`", servicesRoot)
	}

	h := sha256.New()
	for _, b := range envelopeBytes {
		_, _ = h.Write(b)
		_, _ = h.Write([]byte{0})
	}
	keys := make([]string, 0, len(schemas))
	for k := range schemas {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = h.Write(schemas[k])
		_, _ = h.Write([]byte{0})
	}
	digest := "sha256:" + hex.EncodeToString(h.Sum(nil))

	return &Bundle{Digest: digest, Schemas: schemas}, nil
}

// RequestSchema returns the raw JSON Schema for the request side of mcp.tool.
func (b *Bundle) RequestSchema(mcp, tool string) ([]byte, bool) {
	v, ok := b.Schemas[mcp+"/"+tool+".request"]
	return v, ok
}

// ResponseSchema returns the raw JSON Schema for the response side of mcp.tool.
func (b *Bundle) ResponseSchema(mcp, tool string) ([]byte, bool) {
	v, ok := b.Schemas[mcp+"/"+tool+".response"]
	return v, ok
}
