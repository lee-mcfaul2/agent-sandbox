package schemas

import (
	"strings"
	"testing"
)

func TestLoadBundle(t *testing.T) {
	b, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	if b.Digest == "" {
		t.Fatal("digest empty")
	}
	if !strings.HasPrefix(b.Digest, "sha256:") {
		t.Errorf("digest prefix wrong: %q", b.Digest)
	}
	if _, ok := b.Schemas["kb/search.request"]; !ok {
		t.Error("missing kb/search.request")
	}
	if _, ok := b.Schemas["kb/search.response"]; !ok {
		t.Error("missing kb/search.response")
	}
	if _, ok := b.Schemas["audit_db/search.request"]; !ok {
		t.Error("missing audit_db/search.request")
	}
}

func TestDigestDeterministic(t *testing.T) {
	b1, _ := LoadEmbedded()
	b2, _ := LoadEmbedded()
	if b1.Digest != b2.Digest {
		t.Errorf("digest not deterministic: %q vs %q", b1.Digest, b2.Digest)
	}
}

func TestSchemaLookup(t *testing.T) {
	b, _ := LoadEmbedded()
	raw, ok := b.RequestSchema("kb", "search")
	if !ok {
		t.Fatal("RequestSchema missed")
	}
	if !strings.Contains(string(raw), `"q"`) {
		t.Errorf("unexpected schema content: %s", raw)
	}
}
