//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	out := t.TempDir() + "/sandbox"
	cmd := exec.Command("go", "build", "-o", out, "../../cmd/sandbox")
	cmd.Env = append(cmd.Environ())
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, outBytes)
	}
	return out
}

type stubLLM struct {
	t         *testing.T
	responses []map[string]any
	idx       int32
}

func (s *stubLLM) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		i := atomic.AddInt32(&s.idx, 1) - 1
		if int(i) >= len(s.responses) {
			s.t.Fatalf("LLM scripted exhausted at i=%d", i)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.responses[i])
	}
}

func newGatewayMux(digest string, toolHandler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/bundle_digest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"digest": digest})
	})
	mux.Handle("/v1/mcp/", toolHandler)
	return mux
}

func bundleDigest(t *testing.T, bin string) string {
	t.Helper()
	cmd := exec.Command(bin, "--print-bundle-digest")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("--print-bundle-digest: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func runBinary(t *testing.T, bin, litellm, gateway string, extraEnv map[string]string) (stdout, stderr string, exit int) {
	t.Helper()
	env := []string{
		"REQUEST_UUID=11111111-1111-4111-8111-111111111111",
		"PROMPT_UUID=22222222-2222-4222-8222-222222222222",
		"LITELLM_URL=" + litellm,
		"GATEWAY_MCP_URL=" + gateway,
		"AVAILABLE_TOOLS=kb.search",
		"TOKENIZED_USER_INPUT=hello",
		"MODEL=test-model",
		"MAX_ITERATIONS=" + extraEnv["MAX_ITERATIONS"],
		"WALLCLOCK_TIMEOUT_SECONDS=10",
		"TRACEPARENT=00-aaaa-bbbb-01",
	}
	for k, v := range extraEnv {
		if k == "MAX_ITERATIONS" {
			continue
		}
		env = append(env, k+"="+v)
	}
	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command(bin)
	cmd.Env = env
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exit = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exit = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("cmd.Run unexpected error: %v\nstderr: %s", err, errBuf.String())
	}
	return outBuf.String(), errBuf.String(), exit
}

func TestIntegrationHappyTerminate(t *testing.T) {
	bin := buildBinary(t)
	digest := bundleDigest(t, bin)

	llm := &stubLLM{t: t, responses: []map[string]any{
		{
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": "answer"},
			}},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		},
	}}
	llmSrv := httptest.NewServer(llm.handler())
	defer llmSrv.Close()

	gwMux := newGatewayMux(digest, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("gateway should not be tool-called")
	}))
	gwSrv := httptest.NewServer(gwMux)
	defer gwSrv.Close()

	stdout, _, exit := runBinary(t, bin, llmSrv.URL, gwSrv.URL, map[string]string{"MAX_ITERATIONS": "4"})
	if exit != 0 {
		t.Errorf("exit = %d", exit)
	}
	var env struct {
		Terminate struct {
			FinishReason string `json:"finish_reason"`
			Response     string `json:"response"`
		} `json:"terminate"`
	}
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		_ = json.Unmarshal(scanner.Bytes(), &env)
	}
	if env.Terminate.FinishReason != "terminate" {
		t.Errorf("finish_reason = %q (stdout=%s)", env.Terminate.FinishReason, stdout)
	}
	if env.Terminate.Response != "answer" {
		t.Errorf("response = %q", env.Terminate.Response)
	}
}

func TestIntegrationOneToolCallThenTerminate(t *testing.T) {
	bin := buildBinary(t)
	digest := bundleDigest(t, bin)

	llm := &stubLLM{t: t, responses: []map[string]any{
		{
			"choices": []map[string]any{{
				"finish_reason": "tool_calls",
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []map[string]any{{
						"id":   "c1",
						"type": "function",
						"function": map[string]any{
							"name":      "kb__search",
							"arguments": `{"q":"x"}`,
						},
					}},
				},
			}},
		},
		{
			"choices": []map[string]any{{
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": "done"},
			}},
		},
	}}
	llmSrv := httptest.NewServer(llm.handler())
	defer llmSrv.Close()

	gwMux := newGatewayMux(digest, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_result": map[string]any{
				"ok":           true,
				"data":         map[string]any{"rows": []any{map[string]any{"id": "1", "title": "x"}}},
				"mcp":          "kb",
				"tool":         "search",
				"request_uuid": "11111111-1111-4111-8111-111111111111",
			},
		})
	}))
	gwSrv := httptest.NewServer(gwMux)
	defer gwSrv.Close()

	stdout, stderr, exit := runBinary(t, bin, llmSrv.URL, gwSrv.URL, map[string]string{"MAX_ITERATIONS": "4"})
	if exit != 0 {
		t.Errorf("exit = %d, stderr:\n%s", exit, stderr)
	}
	if !strings.Contains(stdout, `"finish_reason":"terminate"`) {
		t.Errorf("stdout missing terminate: %s", stdout)
	}
	if !strings.Contains(stderr, `"event":"tool_call"`) {
		t.Errorf("stderr missing tool_call log: %s", stderr)
	}
}

func TestIntegrationDigestMismatchExitsBeforeLLM(t *testing.T) {
	bin := buildBinary(t)

	llmCalled := false
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmCalled = true
	}))
	defer llmSrv.Close()

	gwMux := newGatewayMux("sha256:zzz_wrong", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	gwSrv := httptest.NewServer(gwMux)
	defer gwSrv.Close()

	stdout, _, exit := runBinary(t, bin, llmSrv.URL, gwSrv.URL, map[string]string{"MAX_ITERATIONS": "4"})
	if exit != 1 {
		t.Errorf("exit = %d", exit)
	}
	if llmCalled {
		t.Error("LLM was called despite digest mismatch")
	}
	if !strings.Contains(stdout, "BUNDLE_DIGEST_MISMATCH") {
		t.Errorf("stdout missing BUNDLE_DIGEST_MISMATCH: %s", stdout)
	}
}

func TestIntegrationSchemaMismatch(t *testing.T) {
	bin := buildBinary(t)
	digest := bundleDigest(t, bin)

	llm := &stubLLM{t: t, responses: []map[string]any{{
		"choices": []map[string]any{{
			"finish_reason": "tool_calls",
			"message": map[string]any{
				"role": "assistant",
				"tool_calls": []map[string]any{{
					"id":   "c1",
					"type": "function",
					"function": map[string]any{"name": "kb__search", "arguments": `{"q":"x"}`},
				}},
			},
		}},
	}}}
	llmSrv := httptest.NewServer(llm.handler())
	defer llmSrv.Close()

	gwMux := newGatewayMux(digest, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_result": map[string]any{
				"ok":           true,
				"data":         map[string]any{"rows": "not-an-array"},
				"mcp":          "kb",
				"tool":         "search",
				"request_uuid": "11111111-1111-4111-8111-111111111111",
			},
		})
	}))
	gwSrv := httptest.NewServer(gwMux)
	defer gwSrv.Close()

	stdout, _, exit := runBinary(t, bin, llmSrv.URL, gwSrv.URL, map[string]string{"MAX_ITERATIONS": "4"})
	if exit != 1 {
		t.Errorf("exit = %d", exit)
	}
	if !strings.Contains(stdout, "schema_mismatch") {
		t.Errorf("stdout missing schema_mismatch: %s", stdout)
	}
}

func TestIntegrationConfigMissing(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin)
	cmd.Env = []string{"REQUEST_UUID=" + strconv.Itoa(1)}
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "CONFIG_INVALID") {
		t.Errorf("missing CONFIG_INVALID: %s", out)
	}
}
