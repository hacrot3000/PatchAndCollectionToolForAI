# HANDOFF — AI Agent contextcompact + multi-day Patch/Collection continuity

**Date:** 2026-11-21  
**Repository:** `hacrot3000/PatchAndCollectionToolForAI`  
**Working PR:** [#158](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/158)  
**Base:** `b6d5786b4ae117d943ede0f9d8f6b305ef47f176` (main at start of this request).

## User requirements / invariant

1. Separate, changeable **context-compaction library**. A newly executed Patch/Collection result goes to the model once in the next tool-result request. Old completed Patches send only name, status, IDs/provenance — no Python source or full execution output. Old Collections: keep the newest **N** full completed observations, and transform older observations to concise summaries containing action, paths and file/evidence list.
2. **N is adaptive**, based on the model's configured `max_input_tokens`, the bytes of instructions, the current `Next` result and the safe Collection percentage of the input token budget. No fixed Collection round limit. Never compact the exact new tool result or an unresolved function-call that still needs the matching output.
3. Multiple days, many sequential Patch attempts and repeated Collection. Keep exact evidence in TaskDeck journals/archives even when model-visible history is reduced.
4. After each work request, update this handoff so another AI session can continue from repository state and explicit blockers.

## What was actually verified about existing behavior

- `internal/agentchat/responses.go:BuildResponsesRequest` sends `History+Next` for stateless or unverified server-side references. When a valid `previous_response_id` or conversation reference exists, the HTTP request sends only `Next`. **The provider may still retain the older state** in that chain; local compaction cannot delete remote provider memory.
- Previously `agentModelVisibleHistory` shortened *old Collection output only when the replay was larger than 384 KiB*. It did not prevent old Patch `function_call` arguments from recurring in stateless requests. `CompactHistory` also sometimes dropped canonical plain text and returned HTTP 413.
- The live session validation rejects `len(ToolCalls)>32`. `history_archive.go` archived completed Collection pairs, but not Native Patch records, hence a multi-Patch task could still hit a hard stop.
- `agentChatPatchRecoveryAPI` already has explicitly approved `send_existing/rerun_patch/rerun_and_send` operations. A feedback marked sent/unknown is intentionally **not** automatically resent: provider acknowledgement could be ambiguous.

## Code changes in PR #158

| Change | Files |
|---|---|
| Reusable `contextcompact.Build` takes `Policy` (model max input tokens, safe percentage, instructions, Next, archive summary), outputs new provider-only history plus `Stats`, leaving canonical journal immutable | `internal/agentchat/contextcompact/compact.go` |
| Estimate tokens conservatively from UTF-8 bytes; default 70% overall input headroom, Collection share 30% (tunable in library). Keep the newest completed Collection pairs **while they fit** and summarize older ones with bounded action + file/evidence lists, hashes. Old completed Patch pairs are replaced by short audited summaries | same |
| Preserve unmodified pending call + exact `Next` result; include archived Patch receipts as metadata only. Exclude synthetic text-only Patch calls from provider history | `internal/server/agent_chat_text_patch.go` |
| Use the new policy on user Send, Collection continuation and Native Patch feedback continuation | `agent_chat_messages.go`, `agent_chat_collection_flow.go`, `agent_chat_tool_feedback.go` |
| Remove old canonical-history deletion that could lose conversation data. Model payload size is controlled separately | `agent_chat_messages.go`, `agent_chat_tool_feedback.go` |
| On save, segment completed **acknowledged** native Patch records into private SHA-256-checked files. Keep all active, pending, unknown, unacknowledged calls in the live session. Patch archive pointer and summary metadata are revision-fenced; deleting session cleans up segments | `internal/agentchat/patch_archive.go`, `store.go` |
| On server restart, mark orphaned `requesting_ai` turns as failed with a durable `turn_interrupted` event and mark `sending` Patch feedback as unknown, never replaying it blindly | `internal/server/agent_chat_sync.go` |
| Regression tests for adaptive N, source/receipt not resent, pending call preserved, 125 acknowledged Patches, unknown intents, owner scope, restart | `contextcompact/compact_test.go`, `patch_archive_test.go`, `agent_chat_sync_test.go`, `agent_chat_text_patch_test.go` |
| Ensure CI runs `go test -race` and vet on new contextcompact package | `.github/workflows/taskdeck_agentchat_core.yml` |

**Important scope:** This PR contains a **safe interrupted-state recovery marker**, not an automatically resumable cross-process provider transaction scheduler.

## Invariants to check before merging

- `go test -race -count=1 ./internal/agentchat/...`
- `go test -count=1 ./internal/server -run '^TestAgent'`
- `go vet ./internal/agentchat/... ./internal/server`
- `node web/agentchat_bootstrap_test.cjs`
- No same `call_id` Patch executed twice, no archive of `running`/`unknown` or `FeedbackStatus=unknown`.
- Canonical `History`, original user task, Patch package hashes and native receipts are not discarded for context compaction.
- Explicit owner checks and revision fences on Patch archive retrieval.

## Remaining work / risks — DO NOT mark complete

1. **P0 — actual cross-process automatic resume.** Startup can now change an orphaned `requesting_ai` into a readable `failed/needs-attention` state. A durable leased job queue, per-step heartbeat, provider acknowledgement correlation, and safe restart replay of *known-unstarted* requests remain a separate substantial feature. Never replay unknown Native Patch or tool-result egress.
2. **P0 — provider unknown feedback acknowledgement.** The existing `send_existing` operation refuses a result already attempted; this protects exactly-once intent. Need provider-supported idempotency/reconciliation or explicit audited manual recovery to remove the stop while avoiding duplicate output.
3. **P1 — provider-side persistent history.** `previous_response_id` and Conversation mode may retain all previously sent Patch/Collection content. If strict removal from provider context/token usage is required, design a **safe stateless rebase** after a verified terminal tool exchange, preserving the pending function call and audit.
4. **P1 — many rejected / never-acknowledged Patch proposals.** Only safe completed acknowledged calls are archived. Live limit 32 intentionally still protects unresolved items; implement a separately indexed permanent-rejection archive before claiming all Patch outcomes unlimited.
5. **P1 — more than 8 manual retries of one Patch** (`ToolCallRecord.Attempts` validator). Needs append-only attempt archive.
6. **P1 — provider errors, quota, disk exhaustion, legacy 30-minute auto-feedback grant**, and the inability to distinguish automatically a completed *whole user plan* from a model's early textual answer. See `docs/taskdeck_ai_agent/12-multiday-agent-run-audit.md`.
7. **P2 — UI to browse archived Patch receipts and old Collection evidence** (the owner-checked Store readers exist, but no UI pagination endpoint yet).
8. **P2 — handoff after EVERY model turn.** This file is the per-**development request** handoff. It is *not* yet a durable automatically generated handoff after every provider response during an autonomous TaskDeck run. Implement a separate concise task-state checkpoint persisted in the Agent journal with acceptance criteria and plan progress; do not confuse this document with runtime checkpointing.

## How another AI coder can continue

1. Fetch latest `main`, PR #158 and this handoff; do not assume HEAD from the date above.
2. Check GitHub Actions for the PR HEAD; fix failures without weakening the safe Patch archive/feedback invariants.
3. Merge only when core tests pass; preserve unrelated concurrent commits to main.
4. Start the next PR with provider-side state rebase and the durable restart/resume scheduler (risk items 1–3).
5. Update this handoff after every completed coding request with new commit/PR IDs, verified tests, changed assumptions, remaining risks and concrete next steps.

## Completion summary — request closed 2026-11-21

- **Merged PR:** [#158](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/158)
- **Merge commit on main:** `cec5061ef88bf4138759ca9048231b39ae88277f`.
- **Validated PR head:** `15c6c4cb3312cab01601329e895e28bf7571a7cc`.
- **CI:** TaskDeck Agent Chat protocol core **PASS**; Remote Portal (PHP) **PASS**; Remote Agent (Go) still running on the final check.
- **Verified features:** isolated model-context policy, exact fresh tool outputs, adaptively selected recent full Collections, older Collection file/evidence catalogs, old Patch metadata instead of repeated source/output, persistent acknowledged Patch archive, and safe restart interruption state.
- **Not implemented:** automatic cross-process resume through ambiguous provider egress; provider-side Conversation history pruning/rebase; user-facing archive browser; automatic runtime handoff after each provider response. These need separate testable work and remain in the risk register.
- **Next handoff action:** inspect recent `main`, read this file and `12-multiday-agent-run-audit.md`, then implement the remaining durable resume/acknowledgement and optional stateless rebase in a new PR. Do not replay an unknown Patch or tool result automatically.


## 2026-11-23 — Compact attachments before Agent provider context

The same `internal/agentchat/contextcompact` library now owns file-attachment summaries via `CompactAttachment`. This is separate from compacting **old** Patch/Collection exchanges. No source file is executed, no source bytes are sent to the provider unless selected as an explicit bounded quote, and canonical Agent history/receipts remain unchanged.

- Project Explorer **Attach to Chat** calls `GET /api/project/attachment-summary?path=...`. This uses `resolveProjectPath` and a regular-file check, not the old 32 KiB preview/text-fetch path. Source files are streamed through a bounded scanner (64 MiB maximum scanned prefix); output is at most 22 KiB, then the existing 32 KiB/attachment and 64 KiB/turn model payload caps still apply.
- Editor snapshots and local File Picker call `POST /api/project/attachment-summary` with a bounded UTF-8 code snapshot or 256 KiB binary metadata prefix. Browser-selected files do not expose their local absolute path, which is recorded as unavailable. Dirty editor buffers are never reread from disk. Full-size files above the 16 MiB local snapshot budget are clearly flagged **PARTIAL to the model**, which must use Native Collection for exact source when available.
- Code-language dispatch supports Nim (import/from/include/export, types and top-level var/let/const names, proc/func/method/iterator/template/macro/converter, `ee.on`/`ee.once`), Python, Go, JS/TS, Java, Kotlin, Swift, Rust, C/C++, PHP, Ruby, Lua, Shell, SQL, Dart, Scala, Vue/Svelte, CSS and HTML. Each indexed declaration is labeled with original one-based line start/end and signature. Function implementation bodies and constant initializers are suppressed. This is a **heuristic structural index, not an AST or compilation proof**: very long lines, exotic syntax, and output budgets yield an explicitly partial index.
- Non-source attachments (including text notes, structured documents and binary files) reveal only filename, project-relative path when known, original byte length and detected format. Images additionally expose basic dimensions and available EXIF camera/time tags from a bounded header read. GPS coordinates are deliberately omitted even if a GPS EXIF directory exists. Nothing claims that absent EXIF means none exists beyond the read prefix.
- Restricted Developer policy denies the summary route to avoid bypassing line visibility budgets. Shared mode maps GET/POST to the existing `files.read` authorization. Attached summaries retain the existing untrusted-data delimiter and explicit user-provider sharing consent.
- Browser UI compacts automatically without another warning or a 32 KiB rejection for the original file. Quoted selections and terminal excerpts remain exact, subject to their existing 32 KiB bounds.

Regression coverage: `contextcompact/attachment_summary_test.go` (multi-line Nim proc/template/ee.on, exact line ranges, >32 KiB source, other languages, metadata-only/noncode, JPEG EXIF); `agent_chat_attachment_summary_test.go` (project path, binary, local image, RBAC, bad input); `web/agentchat_bootstrap_test.cjs` (large editor staging before provider request).


## 2026-11-23 — AI Agent request and conversation token estimates

- The existing `contextcompact.Tokens` conservative UTF-8 bytes/2 heuristic now estimates the **serialized canonical provider request** when PREPARED is durably journaled. `provider_run.estimated_input_tokens` is available while SENDING. It is **not** a provider tokenizer, nor a guaranteed context-window measurement; stateful provider-side retained history may be larger than this local payload.
- On every successful provider response (user message, Native Collection continuation, Native Patch feedback, or recovered PREPARED-only call), `recordAgentTokenUsage` increments persisted `session.token_usage`. Provider `input_tokens` and `output_tokens` win when reported; missing fields are estimated. A per-run `usage_recorded` fence prevents accidental double-counting of a returned reply. Unconfirmed SENDING/UNKNOWN and failed requests are **not** counted (they may nevertheless incur provider-side charges).
- The conversation pane shows the current request's `≈ input tokens` in its live sending/thinking status and a bottom-right cumulative token readout with input/output, total request count, and number using estimates. Separate conversations have separate counters. Old conversations begin at zero *tracked since the update*: no ungrounded backfill of historical billed usage. Stateless replay counts repeated input per request, unlike a one-time snapshot of transcript size.
- Counts are informational only; they do not alter provider retries, consent, tool execution or native Patch safety.
- Tests: `agent_chat_token_usage_test.go` and `web/agentchat_bootstrap_test.cjs`. Verify CI before treating this change as released.
