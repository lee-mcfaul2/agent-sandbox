package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallToolHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/mcp/kb/search" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var req struct {
			RequestUUID string          `json:"request_uuid"`
			Args        json.RawMessage `json:"args"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.RequestUUID != "req-1" {
			t.Errorf("request_uuid = %q", req.RequestUUID)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_result": map[string]any{
				"ok":           true,
				"data":         map[string]any{"rows": []any{}},
				"mcp":          "kb",
				"tool":         "search",
				"request_uuid": "req-1",
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 500*time.Millisecond)
	env, err := c.CallTool(context.Background(), "req-1", "kb", "search", json.RawMessage(`{"q":"x"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !env.OK {
		t.Errorf("OK = %v", env.OK)
	}
	if env.MCP != "kb" {
		t.Errorf("MCP = %q", env.MCP)
	}
}

func TestCallToolPropagatesErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_result": map[string]any{
				"ok":           false,
				"error":        "OPA_DENY",
				"reason":       "missing_permission:kb:read",
				"mcp":          "kb",
				"tool":         "search",
				"request_uuid": "req-1",
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 500*time.Millisecond)
	env, err := c.CallTool(context.Background(), "req-1", "kb", "search", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if env.OK {
		t.Error("expected OK=false")
	}
	if env.Error != "OPA_DENY" {
		t.Errorf("Error = %q", env.Error)
	}
}

func TestCallToolRetriesOnTransport(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tool_result": map[string]any{"ok": true, "mcp": "kb", "tool": "search", "request_uuid": "req-1", "data": map[string]any{}}})
	}))
	defer srv.Close()

	c := New(srv.URL, 500*time.Millisecond)
	c.RetryBackoff = 1 * time.Millisecond
	if _, err := c.CallTool(context.Background(), "req-1", "kb", "search", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("attempts = %d", attempts)
	}
}

func TestVerifyDigestOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/bundle_digest" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"digest": "sha256:abc"})
	}))
	defer srv.Close()
	c := New(srv.URL, 500*time.Millisecond)
	if err := c.VerifyDigest(context.Background(), "sha256:abc"); err != nil {
		t.Fatalf("VerifyDigest: %v", err)
	}
}

func TestVerifyDigestMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"digest": "sha256:zzz"})
	}))
	defer srv.Close()
	c := New(srv.URL, 500*time.Millisecond)
	err := c.VerifyDigest(context.Background(), "sha256:abc")
	if err == nil {
		t.Fatal("expected mismatch error")
	}
}
