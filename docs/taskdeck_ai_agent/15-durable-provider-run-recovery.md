# HANDOFF 15 — Durable provider egress, PREPARED-only auto-resume

**Date:** 2026-11-21  
**Repository:** `hacrot3000/PatchAndCollectionToolForAI`  
**Working PR:** [#160](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/160)  
**Base:** main at `8652033e81631a961dad9d79c2abea0b39f9ee0b`.

## User goal

Continue developing TaskDeck AI Agent for very large multi-day tasks with hundreds/thousands of Collection and multiple Native Patch requests. Durable handoff per session revision already exists (handoff 14). Current goal: make restart recovery automatic **only where a request definitely has not crossed the provider boundary**, while preserving exactly-once native Patch execution and verified tool feedback.

## Verified pre-existing problem

- `agent_chat_messages.go`, `agent_chat_collection_flow.go`, and `agent_chat_tool_feedback.go` committed `requesting_ai` before sending to the AI provider but did **not** differentiate prepared vs network-initiated requests.
- On daemon restart, `agentReconcilePendingOnStartup` marked all such sessions failed. This could lose a legitimate safe-to-resume request even if the process crashed before any network attempt.
- A request which **already started sending** is NOT safe to automatically replay: the provider might have received it despite a network timeout or daemon crash.

## Implemented in PR #160

1. **Dedicated egress transaction type:** `internal/agentchat/provider_run.go` introduces immutable ID, kind (`message/collection/feedback`), phase (`prepared/sending/acknowledged/unknown/completed/blocked`), call ID, SHA-256 payload digest, provider config fingerprint and timestamps. The session journal persists these fields.
2. **Exact prepared payload:** a 0600, at-most-2-MiB private sidecar stores the complete model request (never the API credential) with run ID/hash. It is synced **before** PREPARED is committed to the canonical session. A crash before session commit leaves a harmless unreferenced sidecar.
3. **Hard egress fence:** all three request paths commit SENDING to the session journal before provider network I/O. The replayable sidecar is removed after the SENDING save; even if that cleanup fails, the phase prevents automatic replay.
4. **Restart semantics:** startup leaves a PREPARED run available to the background scheduler; it marks SENDING or legacy `requesting_ai` as failed/unknown. Never replay ambiguous feedback or rerun a native Patch.
5. **PREPARED recovery:** `agent_chat_provider_resume.go` uses the existing per-session lock and bounded worker pool. It checks owner, workspace/project identity, provider model/config fingerprint, credential, exact staged bytes and original Patch receipt/feedback status. Only after passing these checks does it commit SENDING and call the existing provider/retry/Collection and Native Patch proposal handlers. It cannot auto-execute a Patch without the original native approval gates.
6. **Stop:** `POST /api/agent/sessions/{id}/cancel` can stop a queued PREPARED run even when no network worker was registered yet. Stop marks the request blocked/cancelled and removes the pending sidecar.
7. **Handoff:** the existing Markdown handoff now shows provider run ID, phase, payload digest, whether the request is safe to auto-resume, and when explicit acknowledgement reconciliation is required.
8. **Tests:** private store restart/owner/hash and sending safety, new server tests for initial user request recovery, Collection-safe continuation, prepared Native Patch feedback, corrupted payload, sending-before-crash and Stop-before-egress.

## Strict safety boundary

| Journal phase | Automatic request replay? | Reason |
|---|---|---|
| PREPARED | Yes, exact bytes only | No durable SENDING marker; the request was staged before any network attempt |
| SENDING | **No** | The provider might have received the request |
| ACKNOWLEDGED / COMPLETED | No | The request already generated a verified response |
| UNKNOWN | **No** | Provider acknowledgement cannot be proven |
| BLOCKED | No | Missing credentials, provider change, failed payload verification or user Stop |

A process may crash *after* SENDING is saved but *before* the first network byte. That scenario remains deliberately blocked rather than risk duplicate execution. **This cannot guarantee automatic progress through every possible restart** without provider-side request-idempotency/acknowledgement reconciliation.

## To resume development in another AI or ChatGPT session

1. Pull latest main and read handoffs 13, 14, 15.
2. Check PR #160's final CI (Agent Chat core, Remote Portal, Remote Agent) and merge status. Do not infer success from earlier failing runs.
3. Work only from exact Git HEAD. Respect local user Auto Patch and Auto-feedback permissions.
4. Next priorities: provider-aware reconciliation for **SENDING/UNKNOWN** without duplicate side effects, durable orchestration of plan/acceptance tests, per-run heartbeat for multi-day provider retries, and user-facing reconciliation for ambiguous feedback.
5. Update this handoff after the next development request with actual commit, tests, known blockers and next tasks.

## Remaining risks

- **P0:** Provider SENDING/UNKNOWN cannot be safely re-sent until the provider supplies an authoritative idempotency or acknowledgment mechanism, or the user explicitly resolves the ambiguity.
- **P0:** Full multi-day plan orchestration is still not implemented. A model can voluntarily finish a turn without completing all user tasks.
- **P1:** Durable live-lease/heartbeat, fair queue capacity and resource availability across daemon restarts should be tracked separately. The existing worker pool limits concurrency but is process-local.
- **P1:** Provider configuration or credential changed between PREPARED and restart intentionally blocks exact automatic replay until corrected; this must be exposed clearly in UI.
- **P1:** Private source-bearing PREPARED sidecars are local 0600 files; ensure backup/retention policy and test crash-orphan cleanup.
- **P1:** Existing provider retries can still stop on explicit Stop, permanent errors, quota, disk-full or session corruption. No software can promise indefinite execution under those conditions.
- **P2:** The visible handoff and event API should eventually display the run phase and liveness details directly as dashboard indicators.
