package config

import (
	"testing"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func validEnv() map[string]string {
	return map[string]string{
		"REQUEST_UUID":              "11111111-1111-4111-8111-111111111111",
		"PROMPT_UUID":               "22222222-2222-4222-8222-222222222222",
		"LITELLM_URL":               "http://gateway:4000",
		"GATEWAY_MCP_URL":           "http://gateway:8080",
		"AVAILABLE_TOOLS":           "kb.search,audit_db.search",
		"TOKENIZED_USER_INPUT":      "look up customer 123",
		"MODEL":                     "claude-sonnet-4-6",
		"MAX_ITERATIONS":            "8",
		"WALLCLOCK_TIMEOUT_SECONDS": "300",
		"TRACEPARENT":               "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	}
}

func TestLoadValid(t *testing.T) {
	setEnv(t, validEnv())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RequestUUID != "11111111-1111-4111-8111-111111111111" {
		t.Errorf("RequestUUID = %q", cfg.RequestUUID)
	}
	if cfg.MaxIterations != 8 {
		t.Errorf("MaxIterations = %d", cfg.MaxIterations)
	}
	if len(cfg.AvailableTools) != 2 {
		t.Errorf("AvailableTools len = %d", len(cfg.AvailableTools))
	}
}

func TestLoadMissingRequired(t *testing.T) {
	env := validEnv()
	delete(env, "REQUEST_UUID")
	setEnv(t, env)
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing REQUEST_UUID")
	}
}

func TestLoadInvalidUUID(t *testing.T) {
	env := validEnv()
	env["REQUEST_UUID"] = "not-a-uuid"
	setEnv(t, env)
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid UUID")
	}
}

func TestLoadInvalidMaxIterations(t *testing.T) {
	env := validEnv()
	env["MAX_ITERATIONS"] = "0"
	setEnv(t, env)
	if _, err := Load(); err == nil {
		t.Fatal("expected error for MAX_ITERATIONS=0")
	}
}
