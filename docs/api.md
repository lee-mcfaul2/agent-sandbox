# API contract

`agent-sandbox` is invoked as a one-shot Job. It has no inbound HTTP surface.
Inputs come from env vars; outputs go to stdout (terminate envelope) and stderr
(structured logs).

## Input env vars

| Var | Required | Description |
|---|---|---|
| `REQUEST_UUID` | yes | UUIDv4 minted by the gateway; parroted on every tool call |
| `PROMPT_UUID` | yes | Instance UUID of the inbound `user_prompt`; echoed in the envelope |
| `LITELLM_URL` | yes | Base URL of the gateway-hosted LiteLLM (`/v1/chat/completions`) |
| `GATEWAY_MCP_URL` | yes | Base URL of the gateway's MCP proxy (`/v1/mcp/{mcp}/{tool}`, `/v1/bundle_digest`) |
| `AVAILABLE_TOOLS` | yes | Comma-separated `mcp.tool` entries, computed by the gateway from the user's JWT permissions |
| `TOKENIZED_USER_INPUT` | yes | The user-role message content, already scrubbed/tokenized and LLM-Guard-checked at the gateway |
| `MODEL` | yes | Model name passed straight to LiteLLM (e.g., `claude-sonnet-4-6`) |
| `MAX_ITERATIONS` | yes | Hard cap on LLM iterations; integer ≥1 |
| `WALLCLOCK_TIMEOUT_SECONDS` | yes | Self-timer; integer ≥1 |
| `TRACEPARENT` | yes | W3C tracecontext forwarded as an outbound header on LiteLLM + gateway calls |

## Terminate envelope (stdout, one JSON line)

```json
{
  "terminate": {
    "request_uuid": "...",
    "prompt_uuid": "...",
    "response": "the LLM's free-text final answer",
    "iterations": 3,
    "tools_called": [
      {"mcp": "kb", "tool": "search", "outcome": "ok"}
    ],
    "model": "claude-sonnet-4-6",
    "tokens_used": {"prompt": 1234, "completion": 567, "total": 1801},
    "finish_reason": "terminate"
  }
}
```

### `finish_reason`

| Value | Meaning | Exit |
|---|---|---|
| `terminate` | LLM emitted a content message with no tool calls — success | 0 |
| `iteration_cap` | `MAX_ITERATIONS` reached without terminate | 1 |
| `wallclock_timeout` | `WALLCLOCK_TIMEOUT_SECONDS` elapsed | 1 |
| `schema_mismatch` | A gateway tool response failed validation against the embedded schema (paging-class) | 1 |
| `llm_error` | LiteLLM unreachable or returned a malformed response after one retry | 1 |
| `internal_error` | Config invalid, bundle digest mismatch, or other startup failure | 1 |

When `finish_reason != "terminate"`, the envelope also has an `error` object:

```json
"error": {
  "category": "SANDBOX_RESPONSE_SCHEMA_MISMATCH",
  "mcp": "kb",
  "tool": "search",
  "validation_errors": ["..."]
}
```

## Stderr structured logs

Newline-delimited JSON. The gateway tails pod logs and translates these into
its own metrics + audit pipeline. See `docs/ops.md` for the catalog.

## Real-world note

The demo ships one `final-response.json` shape. Production deployments with
many capability types would fan out: a `summarize` capability returns text, a
`lookup` capability returns structured records. The contract here is the
substrate; downstream schemas can specialize on top.
