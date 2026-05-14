# agent-sandbox

One-shot Go container dispatched by `agent-gateway` per external request.
Runs the agent loop against gateway-hosted LiteLLM and the gateway's MCP
proxy. Talks to nothing else.

## Architecture

The sandbox container has one purpose: drive the agent loop for a single
inbound request, then exit. The gateway dispatches a fresh Job, injects env
vars, tails the pod logs for the terminate envelope on stdout, and deletes
the Job when done. No state persists.

Egress is locked to the gateway namespace (NetworkPolicy + Linkerd mTLS +
`AuthorizationPolicy`). No direct LLM-API access; no direct MCP access; no
credentials.

```
gateway → spawn Job → sandbox container
                       │
                       ├─ POST LITELLM_URL/v1/chat/completions   (loop)
                       └─ POST GATEWAY_MCP_URL/v1/mcp/{m}/{t}    (per tool_call)

sandbox emits terminate envelope → gateway reads from pod logs → returns to user
```

See `docs/api.md` for the input/output contract, `docs/ops.md` for the
log catalog and runbook, and `docs/threat-model.md` for the security model.

## Build

```bash
make fixtures        # copy in-repo schema fixtures into internal/schemas/embedded/
make build           # cgo-disabled static binary at bin/sandbox
make test            # unit tests
make test-integration   # full binary against mocked LiteLLM + gateway
```

Container image (multi-stage, distroless):

```bash
docker build -f deploy/Dockerfile -t agent-sandbox:dev \
  --build-arg BUNDLE_SOURCE=fixture .
```

Production builds set `BUNDLE_SOURCE=oci` + `LIB_AGENT_PROMPT_REF=...` +
`COSIGN_IDENTITY=...` to pull and cosign-keyless-verify the real
`lib-agent-prompt` OCI bundle.

## Distribution

Both the container image and the Helm chart are signed cosign-keyless on tag
push (`.github/workflows/build-and-sign.yml`):

- `ghcr.io/lee-mcfaul2/agent-sandbox:vX.Y.Z`
- `ghcr.io/lee-mcfaul2/agent-sandbox-chart:vX.Y.Z` (Helm 3.8+ OCI)

The gateway verifies both signatures before consuming.

## Status

This repo implements the design in
`docs/superpowers/specs/2026-05-13-agent-sandbox-design.md`. Production
wiring depends on the coordinated cross-repo changes in §12 of that spec
(`lib-agent-prompt` schema flattening, `agent-gateway` revision to drop
the iteration-output envelope and add the `/v1/bundle_digest` endpoint
and JWT-derived `AVAILABLE_TOOLS` computation, `secure-agent-demo` Helm
chart update). Until those land, the sandbox runs against local fixtures
and mocked gateway endpoints.
