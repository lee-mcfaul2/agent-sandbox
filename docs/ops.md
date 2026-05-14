# Operations runbook

## Structured log catalog (stderr JSONL)

| Event | Trigger | Notable fields |
|---|---|---|
| `startup` | Process boot, env validated | `prompt_uuid` |
| `llm_call_start` | Before each POST to LiteLLM | `iteration`, `model` |
| `llm_call_end` | After successful LiteLLM response | `iteration`, `duration_ms`, `tokens.prompt`, `tokens.completion` |
| `llm_call_failed` | LiteLLM call hard-failed (both retries) | `iteration`, `err` |
| `tool_call` | After each gateway MCP call | `mcp`, `tool`, `outcome`, `duration_ms` |
| `tool_call_failed` | Gateway HTTP transport failure | `mcp`, `tool`, `err` |
| `schema_mismatch` | Gateway response failed validation | `mcp`, `tool`, `err` — **paging-class** |

## Gateway-side metric mapping

| Event | Metric |
|---|---|
| `llm_call_end` | `sandbox_llm_iterations_total{model}`, `sandbox_llm_tokens_total{model,kind}` |
| `tool_call` | `gateway_tool_calls_total` (already owned by the gateway) |
| `schema_mismatch` | `sandbox_response_schema_mismatch_total{mcp,tool}` — **paging-class** |
| Envelope `finish_reason` on stdout | `sandbox_finish_reason_total{reason}` |
| Job exit nonzero with no terminate envelope on stdout | `sandbox_failures_total{reason="unparseable"}` — **paging-class** |

## Common failure modes

### `schema_mismatch` from a single mcp/tool

A sandbox saw a gateway response that didn't conform to its embedded schema.
Possible causes:

1. **Gateway/sandbox built against different `lib-agent-prompt` digests.**
   Check the gateway's `gateway_lib_agent_prompt_digest` (publishing the
   digest as a gauge is a gateway-side concern) against the sandbox's
   `--print-bundle-digest` output. Coordinate rebuilds.
2. **Schema bug.** The MCP returned a shape the response schema didn't
   anticipate. Check recent `lib-agent-prompt` PRs.
3. **Gateway compromise.** Rare. Investigate audit logs around the affected
   request UUID for anomalous response payloads.

### `BUNDLE_DIGEST_MISMATCH` at startup

The sandbox image's embedded bundle doesn't match what the gateway reports.
The sandbox refuses to talk to the LLM in this state, so the user sees an
internal-error envelope. Fix: redeploy whichever side is behind, then drain
the affected sandbox image version.

### `iteration_cap` reached repeatedly for a specific prompt

The agent is looping unproductively. Causes: tool returning unhelpful data,
LLM unable to make progress, system prompt too restrictive. Increase
`MAX_ITERATIONS` only after investigation; the cap is a safety net, not a
budget.

### `llm_error` rate elevated

Usually upstream LLM provider issue. The gateway-side LiteLLM logs are the
authoritative source; sandbox-side is downstream of the gateway.

## Drift detection

CI conformance tests verify that the published image's `--print-bundle-digest`
matches the expected value. If the sandbox is deployed independently of the
gateway, the gateway should publish its current `lib-agent-prompt` digest as
a Prometheus gauge, and an alert should fire when sandbox and gateway digests
diverge for more than 5 minutes (rolling upgrade window).
