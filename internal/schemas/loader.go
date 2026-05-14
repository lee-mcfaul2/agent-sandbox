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

// fixtureFS holds the in-package testdata bundle for tests and dev builds.
// Go's //go:embed does not allow ".." path components, so the production
// embedded/ directory (at the module root, two levels above this package)
// cannot be embedded from this file. See LoadFromFS for injecting an
// fs.FS from outside — the build command uses that to wrap os.DirFS over
// the embedded/ directory populated by `make fixtures`.

//go:embed all:testdata/lib-agent-prompt
var fixtureFS embed.FS

const fixtureRoot = "testdata/lib-agent-prompt"

// Bundle is a loaded set of JSON Schema documents with a deterministic digest.
type Bundle struct {
	// Digest is a sha256 content-hash of all schema keys+values, sorted.
	// Format: "sha256:<hex>".
	Digest string

	// Schemas maps "<service>/<tool>.<direction>" → raw JSON Schema bytes.
	// Example key: "kb/search.request"
	Schemas map[string][]byte
}

// LoadEmbedded loads the bundle from the embedded testdata fixtures.
// It is always available without any runtime dependencies and is used by
// tests and by any binary that does not override the schema source.
func LoadEmbedded() (*Bundle, error) {
	return loadFrom(fixtureFS, fixtureRoot)
}

// LoadFromFS loads the bundle from the given fs.FS rooted at root.
// root must point to a lib-agent-prompt directory that contains a
// services/ subdirectory. This is the production entry-point when the
// caller wraps os.DirFS over the embedded/ directory.
func LoadFromFS(fsys fs.FS, root string) (*Bundle, error) {
	return loadFrom(fsys, root)
}

func loadFrom(fsys fs.FS, root string) (*Bundle, error) {
	schemas := make(map[string][]byte)
	servicesRoot := root + "/services"

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
		// Strip "servicesRoot/" prefix and ".json" suffix to form the schema key.
		rel := strings.TrimPrefix(p, servicesRoot+"/")
		rel = strings.TrimSuffix(rel, ".json")
		schemas[rel] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk bundle %s: %w", servicesRoot, err)
	}

	if len(schemas) == 0 {
		return nil, fmt.Errorf("no schemas found under %s", servicesRoot)
	}

	digest, err := computeDigest(schemas)
	if err != nil {
		return nil, err
	}

	return &Bundle{Digest: digest, Schemas: schemas}, nil
}

// computeDigest produces a deterministic sha256 over sorted key/value pairs.
func computeDigest(schemas map[string][]byte) (string, error) {
	keys := make([]string, 0, len(schemas))
	for k := range schemas {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0}) // NUL separator
		h.Write(schemas[k])
		h.Write([]byte{0}) // NUL separator
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// RequestSchema returns the raw JSON Schema for the request side of
// mcp/tool (e.g. mcp="kb", tool="search").
func (b *Bundle) RequestSchema(mcp, tool string) ([]byte, bool) {
	v, ok := b.Schemas[mcp+"/"+tool+".request"]
	return v, ok
}

// ResponseSchema returns the raw JSON Schema for the response side of
// mcp/tool.
func (b *Bundle) ResponseSchema(mcp, tool string) ([]byte, bool) {
	v, ok := b.Schemas[mcp+"/"+tool+".response"]
	return v, ok
}
