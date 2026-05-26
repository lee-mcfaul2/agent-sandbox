package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lee-mcfaul2/agent-sandbox/internal/gateway"
	"github.com/lee-mcfaul2/agent-sandbox/internal/llm"
	"github.com/lee-mcfaul2/agent-sandbox/internal/obs"
	"github.com/lee-mcfaul2/agent-sandbox/internal/schemas"
	"github.com/lee-mcfaul2/agent-sandbox/internal/tools"
)

// Config holds the per-request parameters for a Driver run.
type Config struct {
	RequestUUID      string
	PromptUUID       string
	Model            string
	SystemPrompt     string
	UserInput        string
	MaxIterations    int
	WallclockTimeout time.Duration
}

// Driver is the main agent loop. It orchestrates LLM calls, tool dispatches,
// schema validation and envelope construction.
type Driver struct {
	Config     Config
	LLM        *llm.Client
	Gateway    *gateway.Client
	Catalog    *tools.Catalog
	Validators *schemas.Registry
	Logger     *obs.Logger
}

// Run executes the agent loop until one of the terminal conditions is reached.
// It never returns a non-nil error; all failure modes are encoded in the Envelope.
func (d *Driver) Run(ctx context.Context) (Envelope, error) {
	ctx, cancel := context.WithTimeout(ctx, d.Config.WallclockTimeout)
	defer cancel()

	messages := []llm.Message{
		{Role: "system", Content: d.Config.SystemPrompt},
		{Role: "user", Content: d.Config.UserInput},
	}

	totalTokens := TokensUsed{}
	var calls []ToolCallSummary
	// priorOutcomes tracks (mcp|tool|arguments) -> outcome for prior tool calls
	// in this Run. If the LLM re-emits an identical call whose prior outcome was
	// non-OK, the loop injects a synthetic DUPLICATE_TOOL_CALL tool-result
	// instead of re-dispatching to the gateway — this forces the next LLM turn
	// to do something different rather than spinning on the same failing call.
	priorOutcomes := map[string]string{}
	iter := 0

	for iter < d.Config.MaxIterations {
		iter++
		d.Logger.Info("llm_call_start", map[string]any{
			"iteration": iter,
			"model":     d.Config.Model,
		})

		start := time.Now()
		resp, err := d.LLM.Call(ctx, llm.ChatCompletionRequest{
			Model:      d.Config.Model,
			Messages:   messages,
			Tools:      d.Catalog.OpenAITools,
			ToolChoice: "auto",
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return d.buildFailure(FinishWallclockTimeout, nil, iter, calls, totalTokens, ""), nil
			}
			d.Logger.Error("llm_call_failed", map[string]any{"iteration": iter, "err": err.Error()})
			return d.buildFailure(FinishLLMError, &ErrorBlock{Category: "LITELLM_UNREACHABLE", Message: err.Error()}, iter, calls, totalTokens, ""), nil
		}
		d.Logger.Info("llm_call_end", map[string]any{
			"iteration":   iter,
			"duration_ms": time.Since(start).Milliseconds(),
			"tokens":      map[string]int{"prompt": resp.Usage.PromptTokens, "completion": resp.Usage.CompletionTokens},
		})
		totalTokens.Prompt += resp.Usage.PromptTokens
		totalTokens.Completion += resp.Usage.CompletionTokens
		totalTokens.Total += resp.Usage.TotalTokens

		msg := resp.Choices[0].Message

		// No tool calls → LLM has terminated naturally.
		if len(msg.ToolCalls) == 0 {
			return BuildEnvelope(EnvelopeInput{
				RequestUUID:  d.Config.RequestUUID,
				PromptUUID:   d.Config.PromptUUID,
				Response:     msg.Content,
				HasResponse:  true,
				Iterations:   iter,
				Tools:        calls,
				Model:        d.Config.Model,
				TokensUsed:   totalTokens,
				FinishReason: FinishTerminate,
			}), nil
		}

		// Append the assistant turn with tool calls into the message history.
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		})

		// Dispatch each tool call sequentially.
		for _, tc := range msg.ToolCalls {
			summary, fatal, fatalEnv := d.dispatchToolCall(ctx, tc, &messages, iter, calls, totalTokens, priorOutcomes)
			calls = append(calls, summary)
			if fatal {
				return fatalEnv, nil
			}
		}
	}

	return d.buildFailure(FinishIterationCap, nil, iter, calls, totalTokens, ""), nil
}

// dispatchToolCall resolves a single LLM tool call: validates the tool exists,
// calls the gateway, validates the response schema, and appends a tool message.
// Returns (summary, fatal, fatalEnvelope) — fatal=true means the caller should
// return fatalEnvelope immediately.
func (d *Driver) dispatchToolCall(
	ctx context.Context,
	tc llm.ToolCall,
	messages *[]llm.Message,
	iter int,
	calls []ToolCallSummary,
	tokens TokensUsed,
	priorOutcomes map[string]string,
) (ToolCallSummary, bool, Envelope) {
	mcp, tool, err := tools.DecodeName(tc.Function.Name)
	if err != nil || !d.Catalog.Contains(mcp, tool) {
		// Unknown tool: feed an error message back to the LLM and continue.
		d.Logger.Info("tool_call", map[string]any{"iteration": iter, "name": tc.Function.Name, "outcome": "unknown"})
		body, _ := json.Marshal(map[string]string{"error": "UNKNOWN_TOOL", "name": tc.Function.Name})
		*messages = append(*messages, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: string(body)})
		return ToolCallSummary{MCP: "", Tool: tc.Function.Name, Outcome: "unknown"}, false, Envelope{}
	}

	// Duplicate-tool-call guard: if this exact (mcp, tool, arguments) triple
	// already failed in a prior iteration this Run, do not re-dispatch.
	// Inject a synthetic tool-result so the next LLM turn must do something
	// different (different tool, different arguments, or terminate).
	dupKey := mcp + "|" + tool + "|" + tc.Function.Arguments
	if prior, seen := priorOutcomes[dupKey]; seen && prior != "ok" {
		d.Logger.Info("duplicate_tool_call_suppressed", map[string]any{
			"iteration":     iter,
			"mcp":           mcp,
			"tool":          tool,
			"prior_outcome": prior,
		})
		body, _ := json.Marshal(map[string]string{
			"error":  "DUPLICATE_TOOL_CALL",
			"reason": "this exact call already failed earlier in this run; choose a different tool, different arguments, or terminate with what you have",
			"mcp":    mcp,
			"tool":   tool,
		})
		*messages = append(*messages, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: string(body)})
		return ToolCallSummary{MCP: mcp, Tool: tool, Outcome: "duplicate_suppressed"}, false, Envelope{}
	}

	start := time.Now()
	// string→[]byte conversion is allowed because json.RawMessage is []byte.
	res, err := d.Gateway.CallTool(ctx, d.Config.RequestUUID, mcp, tool, json.RawMessage(tc.Function.Arguments))
	if err != nil {
		d.Logger.Error("tool_call_failed", map[string]any{
			"iteration": iter, "mcp": mcp, "tool": tool, "err": err.Error(),
		})
		summary := ToolCallSummary{MCP: mcp, Tool: tool, Outcome: "gateway_unreachable"}
		return summary, true, d.buildFailure(
			FinishInternalError,
			&ErrorBlock{Category: "GATEWAY_UNREACHABLE", MCP: mcp, Tool: tool, Message: err.Error()},
			iter,
			append(calls, summary),
			tokens,
			"",
		)
	}

	// Validate the response schema only on success responses.
	if res.OK {
		v := d.Validators.Response(mcp, tool)
		var validationErr error
		if v == nil {
			validationErr = fmt.Errorf("no validator for %s/%s", mcp, tool)
		} else {
			validationErr = v.Validate(res.Data)
		}
		if validationErr != nil {
			d.Logger.Error("schema_mismatch", map[string]any{
				"iteration": iter, "mcp": mcp, "tool": tool, "err": validationErr.Error(),
			})
			summary := ToolCallSummary{MCP: mcp, Tool: tool, Outcome: "schema_mismatch"}
			return summary, true, d.buildFailure(
				FinishSchemaMismatch,
				&ErrorBlock{
					Category:         "SANDBOX_RESPONSE_SCHEMA_MISMATCH",
					MCP:              mcp,
					Tool:             tool,
					ValidationErrors: []string{validationErr.Error()},
				},
				iter,
				append(calls, summary),
				tokens,
				"",
			)
		}
	}

	outcome := "ok"
	if !res.OK {
		outcome = mapErrorToOutcome(res.Error)
	}
	d.Logger.Info("tool_call", map[string]any{
		"iteration":   iter,
		"mcp":         mcp,
		"tool":        tool,
		"outcome":     outcome,
		"duration_ms": time.Since(start).Milliseconds(),
	})

	body, _ := json.Marshal(res)
	*messages = append(*messages, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: string(body)})

	// Record outcome for duplicate-call detection on the next iteration.
	priorOutcomes[dupKey] = outcome

	return ToolCallSummary{MCP: mcp, Tool: tool, Outcome: outcome}, false, Envelope{}
}

// mapErrorToOutcome translates gateway error codes into canonical outcome strings.
func mapErrorToOutcome(errCode string) string {
	switch errCode {
	case "OPA_DENY":
		return "opa_deny"
	case "MCP_UNAVAILABLE":
		return "mcp_unavailable"
	case "MCP_TIMEOUT":
		return "mcp_timeout"
	default:
		if errCode == "" {
			return "ok"
		}
		return "error"
	}
}

// buildFailure constructs a failure Envelope, synthesising an ErrorBlock when
// the caller passes nil.
func (d *Driver) buildFailure(
	reason FinishReason,
	errBlock *ErrorBlock,
	iter int,
	calls []ToolCallSummary,
	tokens TokensUsed,
	response string,
) Envelope {
	if errBlock == nil {
		errBlock = &ErrorBlock{Category: fmt.Sprintf("FINISH_%s", reason)}
	}
	return BuildEnvelope(EnvelopeInput{
		RequestUUID:  d.Config.RequestUUID,
		PromptUUID:   d.Config.PromptUUID,
		Response:     response,
		HasResponse:  response != "",
		Iterations:   iter,
		Tools:        calls,
		Model:        d.Config.Model,
		TokensUsed:   tokens,
		FinishReason: reason,
		Error:        errBlock,
	})
}
