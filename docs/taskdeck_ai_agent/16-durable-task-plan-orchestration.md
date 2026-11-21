# HANDOFF 16 — Durable task plan and acceptance-gated Agent continuation

**Date:** 2026-11-21  
**Repository:** `hacrot3000/PatchAndCollectionToolForAI`  
**Working pull request:** [#161](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/161)  
**Base:** main `5882b4412898581a40a5f1e7624a4fd17fdc8e52` (handoff 15).

## User goal

TaskDeck must be suitable for long-running research/code tasks requiring many Collection and native Patch operations and many provider requests, without silently declaring the entire user task complete just because an AI turn ended. It must preserve original goals, acceptance gates, evidence and an up-to-date handoff across restarts. It must not blindly resend ambiguous provider requests or native Patch results.

## Implemented in PR #161

1. `internal/agentchat/task_plan.go`: per-session durable `TaskPlan` with a goal, up to 128 user-owned milestones, acceptance criteria, status (`pending/active/blocked/done`), bounded evidence and plan state (`active/paused/completed`). A provider turn's `completed` does **not** mark the plan completed.
2. `internal/server/agent_chat_task_plan.go`: owner/workspace/project-scoped **GET/POST** `/api/agent/sessions/{id}/plan`. Plan changes are revision compare-and-swap and serialized with in-flight AI turns. Actions: `create`, `add_step`, `set_step`, `set_auto_continue`, `pause`, `resume`, `complete` and explicit `reopen`. A completed plan's evidence cannot be modified unless the user reopens it (paused, Auto continue off).
3. Task completion: all steps must have evidence. `patch` evidence references a final Native Patch receipt with acknowledged feedback, `collection` evidence references a paired verified read-only tool observation, and `manual` evidence is an **explicit user-attested note**, clearly distinguished from verified tool proof. Evidence keeps a digest and exact call ID (no full source or log). Historical evidence can be looked up in the SHA-256-checked owner-scoped Collection/Patch archives.
4. `agentChatInstructionsWithAutomation` adds the goal, current milestone and acceptance criteria to provider instructions, without granting tool execution authority.
5. `internal/server/agent_chat_plan_coordinator.go`: explicit *local-user opt-in* Auto continue uses the **existing Agent Chat message API**, including provider consent, request idempotency, native Patch approval gates, project identity checks, 60-second per-attempt timeout, unlimited read-only Collection loop and PREPARED/SENDING provider egress journal. It is not a bypass or independent executor.
6. Model-only loops: after **three provider continuations with no new verified Collection/Patch receipts or plan evidence**, Auto continue pauses, leaving the task incomplete for inspection. Genuine tool progress resets the counter. This is not a Collection/Patch round cap.
7. `internal/agentchat/handoff.go`: every per-revision transferable handoff now includes whole-plan state, next milestone, acceptance criteria, provenance references, progress and no-progress turn count.
8. `web/featuremods/agentchat.js`: **Tools → Task plan** panel. Create a step list using `step-id | title | acceptance`; inspect progress, mark milestones working/blocked/done with evidence, add steps, pause/resume, explicitly authorize Auto continue and confirm completion.
9. **Durable admission backoff:** if the scheduler cannot obtain a new provider request slot (HTTP 429) or faces temporary server/resource unavailability (500/503) **before provider egress**, it persists `NextRetryAt` and exponential delay (starting at 10 seconds, capped at ten minutes). It never uses this timer to replay SENDING/UNKNOWN requests. Invalid/non-retryable requests pause for human review; actual post-egress errors remain governed by the provider egress journal.
10. Regression tests: canonical store reopening, fabricated evidence rejected, real verified Patch/Collection references, archive lookup after hundreds of tool observations, stale CAS, no consent/no implicit auto-run, safe plan continuation, persisted retry-after, paused no-progress behavior and ambiguous provider/Patch feedback suppression.

## Example API workflow

Create:

```json
{
  "action": "create",
  "expected_revision": 1,
  "goal": "Complete the repository migration and verify tests",
  "steps": [
    {"id":"audit","title":"Research the existing code","acceptance":"Verify actual files via Collection"},
    {"id":"fix","title":"Implement changes","acceptance":"Acknowledge verified Native Patch result"},
    {"id":"test","title":"Verify behavior","acceptance":"Record passing tests or explicit user review"}
  ]
}
```

Mark a milestone done only with evidence:

```json
{"action":"set_step","expected_revision":12,"step_id":"audit","status":"done","evidence_kind":"collection","evidence_ref":"actual-verified-call-id","evidence_note":"Read required source"}
```

Opt-in for continued provider requests:

```json
{"action":"set_auto_continue","expected_revision":13,"enabled":true,"confirm_external":true}
```

The user must explicitly enable this for each local conversation. Source editing remains subject to the separately configured Native Patch approval controls; Auto continue never implies permission to apply patches. A user can disable Auto continue or press Stop.

## Strict invariants

- No model output or text-only declaration marks a plan complete automatically.
- No missing/fabricated tool ID counts as verified evidence.
- A human manual acceptance note is user-attested, **not** labeled as tool-verified.
- Full original user instructions, archives and Native Patch receipts remain canonical; a task-plan summary is only provider context and handoff.
- The scheduler does **not** dispatch when provider egress is PREPARED/SENDING/UNKNOWN, native Patch is unresolved, or Patch feedback acknowledgement is unknown. It does not resurrect failed/rejected turns without a fresh approved user action.
- Explicit Auto continue is local-owner only; shared-server sessions never inherit it.

## Next risks — still open

- **P0: ambiguous provider acknowledgement after SENDING/UNKNOWN.** Provider-specific authoritative reconciliation/idempotency remains necessary. Do not guess a receipt or replay native side effects.
- **P0: fully autonomous acceptance verification.** The model cannot mark user-owned milestones done by itself; current plan completion is evidence-checked through the owner API. Future work should introduce model-suggested task updates with deterministic validation and an explicit trusted acceptance evaluator, without trusting a plain textual declaration.
- **P1: fairness under long-running tasks.** Four process-local workers may all run for days, delaying other sessions. Durable FIFO fairness, resource health and multi-daemon worker leases remain separate work.
- **P1: plan schema growth.** Maximum 128 active milestones, 4 MiB session and model context budget still apply; a multi-thousand step plan should be paginated into separate durable plan segments rather than simply raising all size limits.
- **P1: evidence-gated manual review.** Historical result browsing is owner-scoped in the Store but not yet a rich per-step evidence browser in Agent UI.
- **P1: automatic turns may incur significant provider charges.** The three no-progress pause is deliberate; users must explicitly opt in to further provider egress, and can Stop at any time.

## Transfer checklist

1. Fetch HEAD `main`, review PR #161 and run CI for the actual latest PR HEAD.
2. Do not merge a failing build or skip real regression failures.
3. Record exact merged commit, CI status and unresolved risks below.
4. To continue in another AI session, open `Tools → Open handoff` in the actual Agent conversation and attach handoffs 13–16. User goal/acceptance checks remain authoritative.

## Completion record — 2026-11-21

- **Merged PR:** [#161](https://github.com/hacrot3000/PatchAndCollectionToolForAI/pull/161).
- **Merge commit on main:** `7fb3bb4833d857315567a88a04bb7f51457c0915`.
- **Validated feature HEAD:** `99125bc780582cc0dc7fa5c2ccb5847bff95f924`.
- **CI on final feature HEAD:** **TaskDeck Agent Chat protocol core PASS**, **Remote Portal (PHP) PASS**, **Remote Agent (Go) still in progress** at the last check. Do not claim unverified checks passed.
- **Delivered:** user-owned durable task checklist, owner-scoped API, evidence checks across recent and archived Collection/Patch, per-request acceptance guidance, per-revision handoff, Task plan UI, explicitly approved local Auto continue, server-owned continuation, durable admission retry-after, no-progress pause, native approval and ambiguous provider feedback gating.
- **Not delivered:** an AI-driven trustworthy evaluator which independently marks semantic acceptance tests complete; authoritative acknowledgement reconciliation of `SENDING`/`UNKNOWN` requests; fair distributed multi-day worker leasing. These remain concrete next work.
- **Next continuation:** fetch the current `main`, read handoffs 13–16 and the multi-day risk register. Implement authoritative provider acknowledgement reconciliation where the provider supports it, then consider a model-proposed milestone proof protocol with deterministic human-approved acceptance. Never turn a model-only claim into a verified Patch result.
