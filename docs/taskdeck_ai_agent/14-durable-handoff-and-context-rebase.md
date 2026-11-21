# HANDOFF 14 — Automatic session checkpoints and bounded provider context

Date: 2026-11-21  
Repository: `hacrot3000/PatchAndCollectionToolForAI`  
Working PR: [#159](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/159)  
Base main: `c5aab547506d91112457cca6104c6363dae6f274`.

## Goal and scope

Continue the multi-day AI Agent effort documented in [handoff 13](13-context-compact-and-multiday-handoff.md). The current request is to make task transfer genuinely practical after **every provider request** and prevent previously accumulated, server-side Conversations/Responses history from defeating the local `contextcompact` library.

## Implemented in PR #159

1. **Durable handoff per session revision:** `internal/agentchat/handoff.go` derives compact Markdown from the canonical session journal. `Store.Save` materializes a private `<session>.handoff.md` after each successful revision. A failed derivative write **must not fail the already-committed session** or trigger retry of a provider/Patch side effect. The handoff is always regenerable on demand from the source journal.
2. **Usable transfer interface:** `GET /api/agent/sessions/<id>/handoff` returns owner/project-scoped JSON with revision and Markdown; `?format=markdown` returns plaintext Markdown. The Agent Chat **Tools → Open handoff** action opens that latest document in a new tab for copying to another AI/coder.
3. **Content of handoff:** session/project/revision; original human goal; most recent assistant observations, Collection actions and file paths, recent Patch statuses/feedback/run IDs; counts of earlier archived evidence; last event cursor; honest `next action` text. It never copies full source/long Patch scripts or invents task completion. Ambiguous feedback is explicitly marked for reconciliation.
4. **Provider context rebase:** `internal/server/agent_chat_context_rebase.go` switches the *effective* next request to stateless replay once the session contains a Collection/Patch exchange, historical archives, or more than 40 transcript items. It uses `agentContextForRequest` with the earlier token-budgeted `contextcompact` result, rather than reusing the provider's full Conversations/Responses chain. The session's selected mode stays immutable. Rebase clears stale provider references; a pending function-call and its newly verified result remain paired and byte-exact.
5. **Integration:** Applied to user Send, native Patch feedback, and read-only Collection continuation. `BuildResponsesRequest` is still the sole provider adapter, and the native Patch approval/queue is unchanged.
6. **Tests:** new `internal/agentchat/handoff_test.go` and `internal/server/agent_chat_handoff_test.go` validate per-revision persistence, owner scoping, precise current handoff, missing-session behavior, no duplicate Patch instructions, and provider tool-call pairing after rebase. Existing test for unverified Conversation ID updated to disallow adopting a ghost ID from a stateless fallback. CI `TaskDeck Agent Chat protocol core` covers the new files.

## Transfer / recovery workflow

1. Open the TaskDeck AI Agent conversation → Tools → **Open handoff**. Copy the Markdown or use `GET /api/agent/sessions/<id>/handoff?format=markdown` as an authenticated owner.
2. In a new AI session, supply the handoff and tell it to inspect the repo/current worktree, the original task acceptance criteria, and any ambiguous Patch calls. It must not blindly re-run earlier Patch packages or assume the last model turn completed the whole task.
3. A server restart may interrupt in-flight provider HTTP work. `requesting_ai` transitions to failed with an event and `sending` Patch feedback becomes `unknown`; the checkpoint identifies this and requires provider/native receipt reconciliation. No unsafe automatic resend occurs.
4. If another coder is continuing this feature's implementation, pull latest `main` before changes, read handoff 13 and 14, check PR #159's CI and post-merge commit, then update this file with its own PR/commit/tests and remaining risks.

## Remaining important risks — not fixed by this PR

- **P0: Autonomous restart/resume:** there is no durable worker lease/idempotent provider job registry that can safely resume an in-flight model call across TaskDeck process restarts. Current handoff enables safe transfer and explicit repair, but does not keep the model running by itself.
- **P0: Unknown provider acknowledgement:** the system cannot prove a tool-result egress was acknowledged if the network failed between send and receiving a response. Automatically replaying that same result can create duplicate state or charge extra usage. Need provider-specific reconciliation/idempotency and manual approved recovery for ambiguity.
- **P1: Long-term resource pressure:** disk-full, API quota/model errors, unbounded attempts in disabled/limited retry modes, and archive space can still pause a multi-day task. A health/status/alert mechanism is needed.
- **P1: Plan-level completeness:** `completed` means only the current model turn finished. Need a durable task plan with acceptance criteria, task status, and automatic next-task scheduling.
- **P1: Model-safe truncation:** `contextcompact` uses an estimated tokenizer and configurable percentages; large AI Packs or many unresolved Patch calls can still exceed a real model context window. Do not discard pending approvals/evidence just to fit.
- **P2: Archived evidence browser:** historical Patch/Collection segments have owner-scoped Store readers but no complete UI for navigating exact old evidence.

## Verification and merge

PR #159 Agent Chat protocol core: **PASS** on validated head (record exact SHA after merge). Remote Portal (PHP): **PASS**. Remote Agent (Go) check may still be in progress at the time of the final update.

**Delivery constraint:** Do not describe this as a guarantee of uninterrupted multi-day autonomous execution. It provides a durable handoff on every persisted session step and bounded provider context, while preserving the explicit recovery boundary for ambiguous egress.


## Completion record — 2026-11-21

- PR: [#159](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/159), **merged**.
- Merge commit: `25fd122fecc57b710509ec9f38124a111f62d4fd`.
- CI on final functional head `f95afaffbe70f51f9c19c5875871d0dc23f29996`: **TaskDeck Agent Chat protocol core PASS**, **Remote Portal (PHP) PASS**; Remote Agent (Go) check ended **cancelled**, not a verified PASS.
- End-to-end handoff access: AI Agent Chat → Tools → Open handoff; or owner-scoped `GET /api/agent/sessions/{id}/handoff?format=markdown`.
- Validated: canonical session remains authoritative; a completed provider turn updates a durable revision and corresponding Markdown checkpoint; remote provider chains are not reused after a Patch/Collection exchange and new `function_call_output` stays paired with its pending call.
- **Not completed:** automatic safe restart of an ambiguous provider request, authoritative provider acknowledgement reconciliation for unknown feedback, task-level acceptance-plan orchestration, or endurance testing over several real days. These remain next priority work, not claimed fixed here.
