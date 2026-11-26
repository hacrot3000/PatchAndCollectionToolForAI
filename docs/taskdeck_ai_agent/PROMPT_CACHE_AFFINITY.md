# Legacy Agent prompt cache and gateway affinity

## Behavior

- Stable opaque 64-hex affinity ID derived from the canonical conversation, owner/project/workspace, provider ID, endpoint/API type and selected model. It is not a provider conversation reference or an idempotency/authorization token. Different models/providers/conversations intentionally use independent namespaces. No raw local path, owner identity or API key is sent.
- All Legacy SENDING-journal provider attempts use the same key across user messages, tool feedback, auto-continuation, retry and restart recovery. Request correlation remains distinct. Probes have no conversation affinity.
- Custom HTTPS gateways default to session headers `X-Session-Id`, `X-Session-Affinity`, `Session_id`. Official OpenAI hosts retain previous default behavior.
- Provider JSON supports `cache_affinity`: `headers`, `prompt_cache_key`, `both`, or `off`. Optional `cache_session_header` selects exactly one of `X-Session-Id`, `X-Session-Affinity`, `session_id`, `x-opencode-session`. Use the gateway's documented contract. Unknown arbitrary headers are rejected. No impersonating client User-Agent.
- Body key is explicitly opt-in: serialized into the canonical request before PREPARED and carried through Chat Completions conversion, retries and SSE. No mutation of old PREPARED bytes. Explicit settings participate in provider version fingerprints.
- Full-history policy: retain valid active history until an actual serializer/item limit or explicit provider context-limit rejection. No early compaction solely for cache hit or token estimates. Summarize acknowledged failed historical PATCH pairs only when uniquely matched to the ledger's receipt digest; preserve exact current feedback, canonical history and trusted instructions.
- Metadata-only cache epochs persist digests/counts, scope and epoch ID, not duplicate source or receipts. They never authorize replay. Changed authority, provider/config, archive, source or actual compaction may reset epoch. Rewritten retries invalidate the derived checkpoint.
- Optional provider JSON: `prompt_cache_retention` = `in_memory` or `24h`; alternatively compatible Responses gateways may support `prompt_cache_ttl` = `30m` or `12h`. Never set both. Confirm gateway/model capability, cost and retention policy first.
- Token learning is diagnostic under full-history policy. Dynamic trusted Task Plan instructions may still invalidate the prefix; never retain stale authority to improve cache.
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
- Initial isolated execution lacked full checkout (DNS failure); subsequent integrated CI below supersedes that initial limitation. Run `go test ./internal/agentchat/... ./internal/server` from vscode_tasks_menu_go and UI bootstrap tests. No PASS claimed without actual execution.

- Independent execution: exact Git blobs `eba773a92fb707537aaaac29a72640414352406f` (affinity) and `f562e895354f6addebee532feb6e917d2bd9234a` (cache usage) copied to an isolated Go 1.23.2 harness, verified with git hash-object before execution; TestStableScope, TestPolicy and TestUsage PASS (3/3), go vet PASS. Session/provider endpoint/diagnostic dependencies were stubbed, so this verifies helper logic only, not full TaskDeck integration or real gateway behavior. Initial isolated full repository test was unavailable (DNS failure); see integrated CI below. Extended prompt cache retention and WebSocket transport are intentionally not enabled: require explicit compatible provider support and separate verification.


## Phase 2 acceptance — 2026-11-26

Local baseline `3f89d068afdbdf0612c24aeb19d086217dc162c9`: [CI 45786794008](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45786794008) SUCCESS (agentchat-core 139795471293 and agent-failed-queue 139795470928). Prior fixes `f698eb29` ([CI](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45786563066)) and `ad506dab` ([CI](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45786648516)) also SUCCESS. Full workflow includes all agentchat packages with race detection, all TestAgent server tests, targeted CLP20, UI, canonical schema and macOS cross-compilation. Includes 80-turn old/new concurrent local context/store regression (20 turns per conversation across two API types, store reopen every five turns). Native 150 MiB read: 157286400-byte source unchanged, receipt 12558 bytes, UNKNOWN restart/resume exactly once with simulated provider. These tests do not call R2D or prove deployed daemon restarts. Local acceptance PASS at the cited commit; external gateway acceptance OPEN.

### Live acceptance procedure

1. Self-update to a passing revision containing `ad506dab`; record the installed revision. Keep existing provider/model/credential/effort, Stateless and affinity configuration. Never change config mid-request or benchmark with destructive PATCH. Capture default redacted AI Debug before any optional Debug Clear. Clear is a trace watermark, not necessarily a reset of durable cumulative counters. Do not enable raw capture or share secrets/full prompts.
2. Collect at least 20 successful completed attempts per old/new conversation, including normal COLLECT/PATCH feedback, naturally occurring retry, concurrent sessions, and restart/self-update continuity when no native PATCH is active. Observe archive/actual-limit boundaries when naturally encountered. Export before retention overwrites traces; deduplicate by trace ID across exports. Truncated windows are not complete sessions.
3. Inspect `affinity.header`, `affinity.cache_key` if enabled, `cache.epoch`, `cache.prefix`, `cache.last_success_prefix`, `cache.token_learning`, `cache.usage`, `cache.session_stats`, `cache.near_zero`, `cache.backend` if exposed and `cache.observation_unavailable`. Include HTTP status, duration, wire bytes, history items and API type. Failed/pending attempts separate.
4. Per unchanged scope/window: weighted hit = sum(cache read)/sum(comparable inclusive input); uncached input = sum(input - read) over those same eligible attempts. Use exact provider_input_tokens from cache.prefix and valid reported cache.read; never reconstruct exact token totals from rounded hit percentages. Near-zero = count(read*100 < input)/count(comparable attempts). Exclude missing/incomparable usage. Report arithmetic mean of old rounded hit percentages separately. Report median/p95 HTTP duration and error counts. Compare similar workloads, not mixed models/effort. Missing cache-write/read is unreported, not zero. Use same-scope cumulative deltas or deduplicated per-trace data, never mix cumulative totals with truncated windows. Twenty samples are a smoke test, not reliable proof of <1% near-zero.
5. Compare current with last successful request, not just failed retry. Changed instructions/tools/history suggests local prefix drift. Matching local prefix with changed backend suggests routing; matching prefix after long idle suggests expiration. Local common segment bytes are not actual upstream tokenizer bytes. Missing backend data cannot distinguish eviction/failover. These are diagnostic signals, not causal proof.
6. Ask R2D to confirm accepted exact header/name format, forwarding and sticky routing, allowlisted backend response fingerprint, prompt_cache_key and lifetime capability. Only opt in after confirmation; test one config change at a time. No WebSocket enabled.
7. Verify no lost/duplicate current receipts, stable scoped affinity and independent conversations, no checkpoint replay, restart recovery, complete diagnostic coverage and improved absolute uncached tokens/latency/near-zero rate. Historical baseline 72.6% arithmetic mean, 3/62 near-zero; aspirational 85–95% hit and <1% near-zero are targets, not guarantees. Do not claim live acceptance before real evidence.
