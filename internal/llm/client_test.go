package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientHappyPath(t *testing.T) {
	var gotInternal, gotUUID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		gotInternal = r.Header.Get("X-Agent-Gateway-Internal")
		gotUUID = r.Header.Get("X-Request-UUID")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ChatCompletionResponse{
			Choices: []Choice{
				{
					FinishReason: "stop",
					Message:      AssistantMessage{Role: "assistant", Content: "done"},
				},
			},
			Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 500*time.Millisecond)
	c.RequestUUID = "test-uuid-1234"
	res, err := c.Call(context.Background(), ChatCompletionRequest{Model: "x"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(res.Choices) != 1 {
		t.Fatalf("choices = %d", len(res.Choices))
	}
	if res.Choices[0].Message.Content != "done" {
		t.Errorf("content = %q", res.Choices[0].Message.Content)
	}
	if gotInternal != "1" {
		t.Errorf("X-Agent-Gateway-Internal = %q, want \"1\"", gotInternal)
	}
	if gotUUID != "test-uuid-1234" {
		t.Errorf("X-Request-UUID = %q, want \"test-uuid-1234\"", gotUUID)
	}
}

func TestClientWithToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ChatCompletionResponse{
			Choices: []Choice{
				{
					FinishReason: "tool_calls",
					Message: AssistantMessage{
						Role: "assistant",
						ToolCalls: []ToolCall{
							{
								ID:   "call_1",
								Type: "function",
								Function: FunctionCall{
									Name:      "kb__search",
									Arguments: `{"q":"hi"}`,
								},
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()
	c := New(srv.URL, 500*time.Millisecond)
	res, err := c.Call(context.Background(), ChatCompletionRequest{Model: "x"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if len(res.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d", len(res.Choices[0].Message.ToolCalls))
	}
	if res.Choices[0].Message.ToolCalls[0].Function.Name != "kb__search" {
		t.Errorf("name = %q", res.Choices[0].Message.ToolCalls[0].Function.Name)
	}
}

func TestClientRetriesOnce(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(ChatCompletionResponse{
			Choices: []Choice{{FinishReason: "stop", Message: AssistantMessage{Role: "assistant", Content: "ok"}}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 500*time.Millisecond)
	c.RetryBackoff = 1 * time.Millisecond
	res, err := c.Call(context.Background(), ChatCompletionRequest{Model: "x"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if res.Choices[0].Message.Content != "ok" {
		t.Errorf("content = %q", res.Choices[0].Message.Content)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("attempts = %d", attempts)
	}
}

func TestClientFailsAfterRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(srv.URL, 500*time.Millisecond)
	c.RetryBackoff = 1 * time.Millisecond
	if _, err := c.Call(context.Background(), ChatCompletionRequest{Model: "x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestClientForwardsTraceparent(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("traceparent")
		_ = json.NewEncoder(w).Encode(ChatCompletionResponse{Choices: []Choice{{FinishReason: "stop", Message: AssistantMessage{Role: "assistant", Content: "ok"}}}})
	}))
	defer srv.Close()
	c := New(srv.URL, 500*time.Millisecond)
	c.Traceparent = "00-aaaa-bbbb-01"
	_, _ = c.Call(context.Background(), ChatCompletionRequest{Model: "x"})
	if got != "00-aaaa-bbbb-01" {
		t.Errorf("traceparent = %q", got)
	}
}
