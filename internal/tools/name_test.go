package tools

import "testing"

func TestEncodeName(t *testing.T) {
	got := EncodeName("kb", "search")
	if got != "kb__search" {
		t.Errorf("EncodeName = %q", got)
	}
}

func TestDecodeName(t *testing.T) {
	mcp, tool, err := DecodeName("audit_db__search")
	if err != nil {
		t.Fatal(err)
	}
	if mcp != "audit_db" || tool != "search" {
		t.Errorf("mcp=%q tool=%q", mcp, tool)
	}
}

func TestDecodeMissingSeparator(t *testing.T) {
	if _, _, err := DecodeName("oops"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeEmptyMCP(t *testing.T) {
	if _, _, err := DecodeName("__search"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeEmptyTool(t *testing.T) {
	if _, _, err := DecodeName("kb__"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRoundTrip(t *testing.T) {
	cases := []struct{ mcp, tool string }{
		{"kb", "search"},
		{"audit_db", "search"},
		{"crm", "update_contact"},
	}
	for _, c := range cases {
		mcp, tool, err := DecodeName(EncodeName(c.mcp, c.tool))
		if err != nil || mcp != c.mcp || tool != c.tool {
			t.Errorf("%q.%q round-trip failed: mcp=%q tool=%q err=%v", c.mcp, c.tool, mcp, tool, err)
		}
	}
}
