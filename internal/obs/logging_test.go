package obs

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestLoggerEmitsJSONLine(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "req-1")
	log.Info("llm_call_start", map[string]any{"iteration": 1, "model": "x"})

	line := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
		t.Fatalf("not a JSON line: %q", line)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["request_uuid"] != "req-1" {
		t.Errorf("request_uuid = %v", got["request_uuid"])
	}
	if got["event"] != "llm_call_start" {
		t.Errorf("event = %v", got["event"])
	}
	if got["level"] != "info" {
		t.Errorf("level = %v", got["level"])
	}
	if got["iteration"].(float64) != 1 {
		t.Errorf("iteration = %v", got["iteration"])
	}
	if _, ok := got["ts"]; !ok {
		t.Error("ts missing")
	}
}

func TestLoggerErrorLevel(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "req-2")
	log.Error("schema_mismatch", map[string]any{"mcp": "kb"})

	var got map[string]any
	_ = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got)
	if got["level"] != "error" {
		t.Errorf("level = %v", got["level"])
	}
}

func TestLoggerOmitsNilFields(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "req-3")
	log.Info("plain", nil)
	if !strings.Contains(buf.String(), `"event":"plain"`) {
		t.Error("event missing")
	}
}
