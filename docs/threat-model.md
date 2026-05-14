# Threat model

The sandbox runs an LLM-controlled loop. Assume it is untrusted from the start.

## Trust boundaries

| Boundary | Mechanism |
|---|---|
| Sandbox ↔ outside world | NetworkPolicy denies all egress except `gateway` namespace + DNS to `kube-system` |
| Sandbox ↔ gateway | Linkerd mTLS + `AuthorizationPolicy` accepting only `spiffe://<trust-domain>/ns/sandbox/sa/agent-sandbox-sa` |
| Process ↔ kernel | gVisor `RuntimeClass=gvisor` |
| Process ↔ image | Distroless, non-root, read-only root FS, all caps dropped, no privilege escalation, seccomp `RuntimeDefault` |

## What an LLM-controlled agent can attempt

- **Exfiltrate data.** Mitigated: no direct egress; tokens replace PII before
  the user input ever reaches the sandbox; gateway-side outbound LLM-Guard +
  scrub re-runs on every tool response before it returns to us.
- **Reach unauthorized data.** Mitigated: AVAILABLE_TOOLS is computed
  server-side from JWT permissions, and the gateway re-checks per-tool
  authz at its MCP-proxy edge (hybrid authz model). The sandbox can call
  only what the gateway will accept.
- **Probe the network.** Mitigated: NetworkPolicy blocks everything else.
- **Persist state.** Mitigated: pod is one-shot; ephemeral volumes only;
  Job is GC'd post-exit.
- **Steal credentials.** Mitigated: there are none mounted. No JWT, no MCP
  creds, no LLM API key. `automountServiceAccountToken: false`.
- **Drift the LLM provider.** Mitigated: LiteLLM lives in the gateway;
  the sandbox sees only an OpenAI-shape endpoint; the gateway audits + scrubs
  + cost-caps every call.

## What the sandbox enforces

- **Self-timer.** WALLCLOCK_TIMEOUT_SECONDS kills the loop if a single LLM
  call hangs.
- **Iteration cap.** MAX_ITERATIONS aborts chatty loops.
- **Response schema validation.** Every successful tool response is
  validated against the embedded `lib-agent-prompt` schema. Mismatch =
  hard abort + paging-class log. Defense-in-depth against gateway
  compromise or unintentional bundle drift.

## What we do not defend against

- **Malicious system prompt.** The system prompt is build-time embedded
  and reviewed via standard PR. Not a runtime concern.
- **Weaponized LLM response that crashes our parser.** Limited blast
  radius: one Job dies, gateway notices, alert fires, traffic continues.
- **Full gateway compromise.** The schema-mismatch alert is one tampering
  signal. Broader gateway compromise is the gateway's own threat model.
- **LLM hallucinations.** The sandbox is the runtime substrate; agent
  behavioral correctness is evaluated by `secure-agent-demo`'s AgentDojo
  CI gate, not here.
