# Legacy Agent prompt cache and gateway affinity

## Behavior

- Stable opaque 64-hex affinity ID derived from the canonical conversation, owner/project/workspace, provider ID, endpoint/API type and selected model. It is not a provider conversation reference or an idempotency/authorization token. Different models/providers/conversations intentionally use independent namespaces. No raw local path, owner identity or API key is sent.
- All Legacy SENDING-journal provider attempts use the same key across user messages, tool feedback, auto-continuation, retry and restart recovery. Request correlation remains distinct. Probes have no conversation affinity.
- Custom HTTPS gateways default to session headers `X-Session-Id`, `X-Session-Affinity`, `Session_id`. Official OpenAI hosts retain previous default behavior.
- Provider JSON supports `cache_affinity`: `headers`, `prompt_cache_key`, `both`, or `off`. Optional `cache_session_header` selects exactly one of `X-Session-Id`, `X-Session-Affinity`, `session_id`, `x-opencode-session`. Use the gateway's documented contract. Unknown arbitrary headers are rejected. No impersonating client User-Agent.
- Body key is explicitly opt-in: serialized into the canonical request before PREPARED and carried through Chat Completions conversion, retries and SSE. No mutation of old PREPARED bytes. Explicit settings participate in provider version fingerprints.
- Context compaction places changing evidence/archive catalogs after stable retained history, before pending native call/output. Canonical history, mandatory current receipt, authority boundaries and budget caps are unchanged. This is a conservative prefix improvement, not a persistent append-only cache epoch implementation. Changing system task-plan instructions, collection scope, archive selection or budget can still invalidate prefix.
- AI Debug: `affinity.header`, `affinity.cache_key`, `cache.usage`, `cache.backend`. Fingerprints are diagnostic only. Cache Read/Write/Hit distinguish absent from explicit zero. Hit percentage is calculated only with known comparable inclusive input totals. Backend fingerprint is available only if gateway exposes an allowlisted response header. No cost estimates or cache-write inference.

## Verify gateway

1. Keep Stateless. Self-update and open the same conversation. Use 10–20 normal turns including COLLECT/PATCH feedback. Inspect gateway logs for a stable affinity value; use a second conversation to prove isolation.
2. Compare backend routing, provider-reported cached tokens, total input, latency and errors. Verify headers are forwarded through the proxy. A routing hint cannot itself guarantee backend affinity/cache; cache expiration, prefix drift and upstream model routing still matter.
3. If gateway supports the JSON key, set `"cache_affinity":"both"` on the provider. If gateway needs one header, set `"cache_session_header":"session_id"` (or its documented supported header). Set off if an endpoint rejects headers. Do not change configuration during an active prepared/requesting turn.
4. Reuse provider-supported stateful modes only if capability verified; no change to rebase/safety behavior. No automatic switch or failover retry introduced.

## References

- OpenCode session headers: https://github.com/anomalyco/opencode/issues/42694 (reported regression in v2; not a claim that all versions forward the headers).
- VS Code cache prefix stability and extended caching: https://code.visualstudio.com/blogs/2026/06/17/improving-token-efficiency-in-github-copilot
- OpenAI prompt caching: https://platform.openai.com/docs/guides/prompt-caching

## Implementation evidence

- f3f00e8d: affinity headers, optional body key and provider JSON.
- bc845984: cache usage JSON/SSE, backend fingerprints and bounded prefix placement.
- Added provider version binding and regression source for isolation, target binding, both API formats, zero/missing usage and current native receipt pairing.
- Full suite and gateway validation pending: checkout DNS failure in execution environment. Run `go test ./internal/agentchat/... ./internal/server` from vscode_tasks_menu_go and UI bootstrap tests. No PASS claimed without actual execution.
