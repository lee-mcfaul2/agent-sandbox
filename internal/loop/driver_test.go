package loop

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lee-mcfaul2/agent-sandbox/internal/gateway"
	"github.com/lee-mcfaul2/agent-sandbox/internal/llm"
	"github.com/lee-mcfaul2/agent-sandbox/internal/obs"
	"github.com/lee-mcfaul2/agent-sandbox/internal/schemas"
	"github.com/lee-mcfaul2/agent-sandbox/internal/tools"
)

type scriptedLLM struct {
	responses []llm.ChatCompletionResponse
	idx       int
}

func newScriptedLLM(t *testing.T, responses []llm.ChatCompletionResponse) (*llm.Client, *scriptedLLM) {
	s := &scriptedLLM{responses: responses}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.idx >= len(s.responses) {
			t.Fatalf("LLM scripted ran out at idx %d", s.idx)
		}
		_ = json.NewEncoder(w).Encode(s.responses[s.idx])
		s.idx++
	}))
	t.Cleanup(srv.Close)
	return llm.New(srv.URL, 5*time.Second), s
}

func newStubGateway(t *testing.T, handler http.Handler) *gateway.Client {
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return gateway.New(srv.URL, 5*time.Second)
}

func mustBundleAndCatalog(t *testing.T) (*schemas.Bundle, *tools.Catalog, *schemas.Registry) {
	b, err := schemas.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := tools.BuildCatalog(b, []string{"agent-sql-mcp.list_orders"})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := schemas.CompileValidators(b, cat.Refs)
	if err != nil {
		t.Fatal(err)
	}
	return b, cat, reg
}

func runDriver(t *testing.T, llmC *llm.Client, gwC *gateway.Client, cat *tools.Catalog, reg *schemas.Registry, maxIter int) Envelope {
	var stderr bytes.Buffer
	logger := obs.New(&stderr, "req-1")
	d := &Driver{
		Config: Config{
			RequestUUID:      "req-1",
			PromptUUID:       "pmt-1",
			Model:            "test",
			SystemPrompt:     "you are a test agent",
			UserInput:        "do something",
			MaxIterations:    maxIter,
			WallclockTimeout: 5 * time.Second,
		},
		LLM:        llmC,
		Gateway:    gwC,
		Catalog:    cat,
		Validators: reg,
		Logger:     logger,
	}
	env, err := d.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return env
}

func TestDriverHappyTerminate(t *testing.T) {
	_, cat, reg := mustBundleAndCatalog(t)
	llmC, _ := newScriptedLLM(t, []llm.ChatCompletionResponse{
		{
			Choices: []llm.Choice{{FinishReason: "stop", Message: llm.AssistantMessage{Role: "assistant", Content: "the answer"}}},
			Usage:   llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			Model:   "test",
		},
	})
	gwC := newStubGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("gateway should not be called")
	}))
	env := runDriver(t, llmC, gwC, cat, reg, 4)
	if env.Terminate.FinishReason != FinishTerminate {
		t.Errorf("finish_reason = %s", env.Terminate.FinishReason)
	}
	if env.Terminate.Response == nil || *env.Terminate.Response != "the answer" {
		t.Errorf("response = %v", env.Terminate.Response)
	}
}

func TestDriverOneToolCallThenTerminate(t *testing.T) {
	_, cat, reg := mustBundleAndCatalog(t)
	llmC, _ := newScriptedLLM(t, []llm.ChatCompletionResponse{
		{
			Choices: []llm.Choice{
				{
					FinishReason: "tool_calls",
					Message: llm.AssistantMessage{
						Role: "assistant",
						ToolCalls: []llm.ToolCall{{
							ID:       "c1",
							Type:     "function",
							Function: llm.FunctionCall{Name: "agent-sql-mcp__list_orders", Arguments: `{"limit":1}`},
						}},
					},
				},
			},
		},
		{
			Choices: []llm.Choice{{FinishReason: "stop", Message: llm.AssistantMessage{Role: "assistant", Content: "done"}}},
		},
	})
	gwC := newStubGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_result": map[string]any{
				"ok":             true,
				"data":           map[string]any{"orders": []any{}},
				"data_plaintext": map[string]any{"orders": []any{}},
				"mcp":            "agent-sql-mcp",
				"tool":           "list_orders",
				"request_uuid":   "req-1",
			},
		})
	}))
	env := runDriver(t, llmC, gwC, cat, reg, 4)
	if env.Terminate.FinishReason != FinishTerminate {
		t.Errorf("finish_reason = %s", env.Terminate.FinishReason)
	}
	if len(env.Terminate.ToolsCalled) != 1 || env.Terminate.ToolsCalled[0].Tool != "list_orders" {
		t.Errorf("tools_called = %+v", env.Terminate.ToolsCalled)
	}
	if env.Terminate.Iterations != 2 {
		t.Errorf("iterations = %d", env.Terminate.Iterations)
	}
}

func TestDriverSchemaMismatchHardAbort(t *testing.T) {
	// Plaintext copy fails validation; the tokenized data is irrelevant here.
	_, cat, reg := mustBundleAndCatalog(t)
	llmC, _ := newScriptedLLM(t, []llm.ChatCompletionResponse{
		{
			Choices: []llm.Choice{{
				FinishReason: "tool_calls",
				Message: llm.AssistantMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{{
						ID:       "c1",
						Type:     "function",
						Function: llm.FunctionCall{Name: "agent-sql-mcp__list_orders", Arguments: `{"limit":1}`},
					}},
				},
			}},
		},
	})
	gwC := newStubGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_result": map[string]any{
				"ok":             true,
				"data":           map[string]any{"orders": []any{}},
				"data_plaintext": map[string]any{"orders": "not-an-array"},
				"mcp":            "agent-sql-mcp",
				"tool":           "list_orders",
				"request_uuid":   "req-1",
			},
		})
	}))
	env := runDriver(t, llmC, gwC, cat, reg, 4)
	if env.Terminate.FinishReason != FinishSchemaMismatch {
		t.Errorf("finish_reason = %s", env.Terminate.FinishReason)
	}
	if env.Terminate.Error == nil || env.Terminate.Error.Category != "SANDBOX_RESPONSE_SCHEMA_MISMATCH" {
		t.Errorf("error = %+v", env.Terminate.Error)
	}
}

func TestDriverValidationUsesPlaintextNotTokenized(t *testing.T) {
	// Regression for the 2026-05-26 alice failure: the gateway scrubs
	// plaintext PII before forwarding to the sandbox. Validating the
	// tokenized copy against a schema like `currency: maxLength 3` would
	// false-fail. The sandbox must validate against data_plaintext while
	// passing data (tokenized) to the LLM.
	_, cat, reg := mustBundleAndCatalog(t)
	llmC, _ := newScriptedLLM(t, []llm.ChatCompletionResponse{
		{
			Choices: []llm.Choice{{
				FinishReason: "tool_calls",
				Message: llm.AssistantMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{{
						ID:       "c1",
						Type:     "function",
						Function: llm.FunctionCall{Name: "agent-sql-mcp__list_orders", Arguments: `{"limit":1}`},
					}},
				},
			}},
		},
		{Choices: []llm.Choice{{FinishReason: "stop", Message: llm.AssistantMessage{Role: "assistant", Content: "done"}}}},
	})
	gwC := newStubGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Tokenized 'data' has fields outside the schema (e.g. simulated
		// TOKEN_NAME_<base32> bloat); plaintext copy is in-schema. Validation
		// must use plaintext and pass.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_result": map[string]any{
				"ok":             true,
				"data":           map[string]any{"orders": []any{}, "scrubbed": "TOKEN_NAME_AAAABBBBCCCC"},
				"data_plaintext": map[string]any{"orders": []any{}},
				"mcp":            "agent-sql-mcp",
				"tool":           "list_orders",
				"request_uuid":   "req-1",
			},
		})
	}))
	env := runDriver(t, llmC, gwC, cat, reg, 4)
	if env.Terminate.FinishReason != FinishTerminate {
		t.Fatalf("finish_reason = %s (validation should have used plaintext copy and passed)", env.Terminate.FinishReason)
	}
	if env.Terminate.ToolsCalled[0].Outcome != "ok" {
		t.Errorf("outcome = %s", env.Terminate.ToolsCalled[0].Outcome)
	}
}

func TestDriverMissingPlaintextIsSchemaMismatch(t *testing.T) {
	// Defense: if the gateway omits data_plaintext on a success response, the
	// sandbox cannot validate. The contract is gateway-supplied; absence is a
	// schema_mismatch error, not silently OK.
	_, cat, reg := mustBundleAndCatalog(t)
	llmC, _ := newScriptedLLM(t, []llm.ChatCompletionResponse{
		{
			Choices: []llm.Choice{{
				FinishReason: "tool_calls",
				Message: llm.AssistantMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{{
						ID:       "c1",
						Type:     "function",
						Function: llm.FunctionCall{Name: "agent-sql-mcp__list_orders", Arguments: `{"limit":1}`},
					}},
				},
			}},
		},
	})
	gwC := newStubGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_result": map[string]any{
				"ok":           true,
				"data":         map[string]any{"orders": []any{}},
				"mcp":          "agent-sql-mcp",
				"tool":         "list_orders",
				"request_uuid": "req-1",
			},
		})
	}))
	env := runDriver(t, llmC, gwC, cat, reg, 4)
	if env.Terminate.FinishReason != FinishSchemaMismatch {
		t.Errorf("finish_reason = %s (missing data_plaintext should fail)", env.Terminate.FinishReason)
	}
}

func TestDriverGatewayErrorEnvelopeFedBack(t *testing.T) {
	_, cat, reg := mustBundleAndCatalog(t)
	llmC, scripted := newScriptedLLM(t, []llm.ChatCompletionResponse{
		{Choices: []llm.Choice{{FinishReason: "tool_calls", Message: llm.AssistantMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "agent-sql-mcp__list_orders", Arguments: `{"limit":1}`}}}}}}},
		{Choices: []llm.Choice{{FinishReason: "stop", Message: llm.AssistantMessage{Role: "assistant", Content: "i give up"}}}},
	})
	_ = scripted
	gwC := newStubGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_result": map[string]any{
				"ok":           false,
				"error":        "OPA_DENY",
				"reason":       "missing_permission",
				"mcp":          "agent-sql-mcp",
				"tool":         "list_orders",
				"request_uuid": "req-1",
			},
		})
	}))
	env := runDriver(t, llmC, gwC, cat, reg, 4)
	if env.Terminate.FinishReason != FinishTerminate {
		t.Errorf("finish_reason = %s", env.Terminate.FinishReason)
	}
	if len(env.Terminate.ToolsCalled) != 1 || env.Terminate.ToolsCalled[0].Outcome != "opa_deny" {
		t.Errorf("tools_called = %+v", env.Terminate.ToolsCalled)
	}
}

func TestDriverUnknownToolFedBack(t *testing.T) {
	_, cat, reg := mustBundleAndCatalog(t)
	llmC, _ := newScriptedLLM(t, []llm.ChatCompletionResponse{
		{Choices: []llm.Choice{{FinishReason: "tool_calls", Message: llm.AssistantMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "phantom__noop", Arguments: `{}`}}}}}}},
		{Choices: []llm.Choice{{FinishReason: "stop", Message: llm.AssistantMessage{Role: "assistant", Content: "ok"}}}},
	})
	gwC := newStubGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("gateway should not be called for unknown tool")
	}))
	env := runDriver(t, llmC, gwC, cat, reg, 4)
	if env.Terminate.FinishReason != FinishTerminate {
		t.Errorf("finish_reason = %s", env.Terminate.FinishReason)
	}
}

func TestDriverIterationCap(t *testing.T) {
	_, cat, reg := mustBundleAndCatalog(t)
	llmC, _ := newScriptedLLM(t, []llm.ChatCompletionResponse{
		{Choices: []llm.Choice{{FinishReason: "tool_calls", Message: llm.AssistantMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "agent-sql-mcp__list_orders", Arguments: `{"limit":1}`}}}}}}},
		{Choices: []llm.Choice{{FinishReason: "tool_calls", Message: llm.AssistantMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c2", Type: "function", Function: llm.FunctionCall{Name: "agent-sql-mcp__list_orders", Arguments: `{"limit":1}`}}}}}}},
	})
	gwC := newStubGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_result": map[string]any{"ok": true, "data": map[string]any{"orders": []any{}}, "data_plaintext": map[string]any{"orders": []any{}}, "mcp": "agent-sql-mcp", "tool": "list_orders", "request_uuid": "req-1"},
		})
	}))
	env := runDriver(t, llmC, gwC, cat, reg, 2)
	if env.Terminate.FinishReason != FinishIterationCap {
		t.Errorf("finish_reason = %s", env.Terminate.FinishReason)
	}
}

func TestDriverLLMHardFailure(t *testing.T) {
	_, cat, reg := mustBundleAndCatalog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	llmC := llm.New(srv.URL, 1*time.Second)
	gwC := newStubGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	env := runDriver(t, llmC, gwC, cat, reg, 4)
	if env.Terminate.FinishReason != FinishLLMError {
		t.Errorf("finish_reason = %s", env.Terminate.FinishReason)
	}
}

func TestDriverDuplicateToolCallSuppressed(t *testing.T) {
	_, cat, reg := mustBundleAndCatalog(t)
	// LLM emits list_orders with identical args {} twice in a row, then
	// terminates with a final assistant message.
	llmC, _ := newScriptedLLM(t, []llm.ChatCompletionResponse{
		{Choices: []llm.Choice{{FinishReason: "tool_calls", Message: llm.AssistantMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "agent-sql-mcp__list_orders", Arguments: `{}`}}}}}}},
		{Choices: []llm.Choice{{FinishReason: "tool_calls", Message: llm.AssistantMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c2", Type: "function", Function: llm.FunctionCall{Name: "agent-sql-mcp__list_orders", Arguments: `{}`}}}}}}},
		{Choices: []llm.Choice{{FinishReason: "stop", Message: llm.AssistantMessage{Role: "assistant", Content: "could not retrieve"}}}},
	})
	// Gateway: count how many times it's called and always return an error.
	var gatewayCalls int
	gwC := newStubGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tool_result": map[string]any{
				"ok":           false,
				"error":        "MCP_UNAVAILABLE",
				"reason":       "upstream down",
				"mcp":          "agent-sql-mcp",
				"tool":         "list_orders",
				"request_uuid": "req-1",
			},
		})
	}))
	env := runDriver(t, llmC, gwC, cat, reg, 5)

	if env.Terminate.FinishReason != FinishTerminate {
		t.Errorf("finish_reason = %s", env.Terminate.FinishReason)
	}
	// The second identical call must NOT hit the gateway.
	if gatewayCalls != 1 {
		t.Errorf("gateway calls = %d, want 1 (duplicate should be suppressed)", gatewayCalls)
	}
	if len(env.Terminate.ToolsCalled) != 2 {
		t.Fatalf("tools_called len = %d, want 2", len(env.Terminate.ToolsCalled))
	}
	if env.Terminate.ToolsCalled[0].Outcome != "mcp_unavailable" {
		t.Errorf("tools_called[0].outcome = %s, want mcp_unavailable", env.Terminate.ToolsCalled[0].Outcome)
	}
	if env.Terminate.ToolsCalled[1].Outcome != "duplicate_suppressed" {
		t.Errorf("tools_called[1].outcome = %s, want duplicate_suppressed", env.Terminate.ToolsCalled[1].Outcome)
	}
}

func TestDriverEmitsToStdout(t *testing.T) {
	_, cat, reg := mustBundleAndCatalog(t)
	llmC, _ := newScriptedLLM(t, []llm.ChatCompletionResponse{
		{Choices: []llm.Choice{{FinishReason: "stop", Message: llm.AssistantMessage{Role: "assistant", Content: "x"}}}},
	})
	gwC := newStubGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	env := runDriver(t, llmC, gwC, cat, reg, 4)
	raw, _ := json.Marshal(env)
	if strings.Contains(string(raw), "\n") {
		t.Error("envelope contains newline; would corrupt stdout single-line contract")
	}
}
