# agent-sandbox

One-shot Go container dispatched by `agent-gateway` per external request.
Runs the agent loop against gateway-hosted LiteLLM and the gateway's MCP proxy.

See `docs/api.md` for the input/output contract and
`docs/threat-model.md` for the security model.
