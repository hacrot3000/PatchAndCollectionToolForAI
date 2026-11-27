# HANDOFF 21 — Autonomous lifecycle recovery + task plan

Status: IN_PROGRESS / external verification pending
Baseline: fbcd887080bd7402c1fbc36ce68e58dc7c771536 (2026-11-27)
Working branch: main
Goal: TaskDeck owns execution lifecycle and recovery; AI owns analysis, source changes and tests. A normal failed test or missing internal metadata must not require the owner to manage the native queue.

## Rules for continuation

Read this file before each implementation turn; reconcile current main, choose the first feasible unfinished item, implement and test, then update this same file with exact commits, evidence, remaining work and blockers. Do not create a separate handoff for each turn. Never mark runtime verification done based only on authored tests. Do not re-run ambiguous native executions or replay unknown provider egress; preserve user approval and verified acceptance. No unattended destructive queue cleanup. Source repair and queue retirement are distinct.

## Task plan (three levels)

- [ ] P1. Authoritative lifecycle state
  - [ ] P1.1 Reconcile terminal outcomes from durable native History, archive, registry and exact owner/session/project/run/package binding.
    - [x] P1.1.a Recover archive/unlink interruption; exclude proven retired LAST_RUN predecessor from native planning (baseline fbcd8870, source authored).
    - [ ] P1.1.b Cover aged History, multiple attempts, requeued bytes, archived calls and malformed registry; run native regression matrix.
  - [ ] P1.2 Make failed test vs source-changing failure explicit.
    - [x] P1.2.a SHA-bound strict manifest classification and transient evidence projection (baseline).
    - [ ] P1.2.b Consolidate decision/diagnosis into typed lifecycle projection and verify native planner refusal reasons.
- [ ] P2. Autonomous continuation
  - [ ] P2.1 Recover a model-only BLOCKED or UNKNOWN response after exact acknowledged terminal PATCH evidence.
    - [x] P2.1.a Bounded fresh recheck, exact canonical source/output and retirement evidence; never retry old native command. SOURCE_IMPLEMENTED (1c9eac36, 03080c07).
    - [x] P2.1.b Preserve explicit Auto continue preference on model-only blocker; keep real unresolved work and no-progress brakes. SOURCE_IMPLEMENTED (ee5fb42e).
    - [ ] P2.1.c Test scheduler admission, owner revocation, restart, repeated blocker and genuine user question. TEST_AUTHORED for consent/ambiguity/no-progress, immediate failure and post-COLLECT, unrelated blocker, and retained preference (a36523e6, 6dbff386, 4fbaaf67); execution and restart integration pending.
  - [ ] P2.2 Carry lifecycle evidence across provider requests, context compaction, COLLECT, Ask User, and Run Manager.
    - [x] P2.2.a Local legacy COLLECT/Ask User evidence continuity (baseline).
    - [ ] P2.2.b Audit all provider request modes, prepared payload immutability, archive compaction and Run Manager parity.
- [ ] P3. Recovery visibility and handoff
  - [ ] P3.1 Extend existing journal-derived per-session handoff, including TaskPlan.
    - [x] P3.1.a Record native run, feedback, retirement and actual vs advisory blocker; don't expose raw source or secrets. SOURCE_IMPLEMENTED (4adef771, 8094d23b); existing handoff now exposes lifecycle candidate and journal retirement, not source rollback.
    - [ ] P3.1.b Verify automatic refresh per committed revision and restart regeneration. Existing Save/Handoff regeneration path retained; TEST_AUTHORED for plan + retirement semantics (b42ddf66), full execution pending.
  - [ ] P3.2 Expose precise lifecycle status in UI/debug (automatic recovery vs needs owner, not misleading BLOCKED).
    - [x] P3.2.a Add diagnostic and UI presentation, no new authorization controls. SOURCE_IMPLEMENTED (e579f803, f90c2d16); JavaScript syntax check PASS (V8), actual Firefox pending.
- [ ] P4. Verification and rollout
  - [ ] P4.1 Go/Python regressions including interrupted retirement, stale LAST_RUN, failed Nim test → source correction → retest, no-progress loop and ambiguous native/provider execution.
  - [ ] P4.2 Real local Agent Chat, old and new sessions, provider modes and independent Run Manager; test reported run 1795779890654600_1055702 if still available.
  - [ ] P4.3 Confirm clean self-update, report CI/real execution evidence and close only verified milestones.

## Invariants and acceptance

* Exact bound terminal receipt before lifecycle mutation; no filename-only assumptions; retirement is not rollback.
* Failed compile/test is useful evidence; native planner decides successor eligibility, not speculative model narration.
* Fresh continuation uses existing approval and consent, no duplicate provider output or PATCH run; unknown/active work remains fenced.
* Repeated no-work turns must stop; a real owner decision/question must be surfaced rather than bypassed.
* Handoff is a derivative of committed canonical state; failure to write it must not create duplicate work; no API credentials or full code bodies.
* Distinguish SOURCE_IMPLEMENTED, TEST_AUTHORED, TEST_EXECUTED_PASS and EXTERNAL_VERIFICATION_PENDING.

## Progress journal

### Turn 1 — 2026-11-27
- Baseline verified on main. Existing `HandoffMarkdown` already combines TaskPlan + session, and Store writes it after committed Save; improve rather than introduce a second runtime store.
- Confirmed source: `StandaloneAutoContinueCandidate` accepts specific UNKNOWN recovery but excludes BLOCKED; standalone scheduler turns AutoContinueText off for model-reported BLOCKED. Next implement narrowly evidence-bound fresh correction and handoff enrichment.
- Actual user run and full native/runtime CI are not accessible in this environment.

### Turn 1 — implementation checkpoint (continued)
- Handoff plan created 31496c75. Latest implementation checkpoint 4fbaaf6764b546acbfffddf50ea1f44cd5fc24ba.
- P2.1: new `RetiredPatchBlockerRecheck` accepts one fresh no-plan recovery message after exact acknowledged terminal PATCH feedback with durable retirement; also after exact native COLLECT feedback when latest assistant references exactly one retired run in the active canonical ledger. The recheck does not re-execute the failed package or replay its result. The next ordinary provider message cannot rearm it. Existing owner opt-in, unresolved work, provider ambiguity and no-progress guards remain.
- A model BLOCKED without matching recovery evidence pauses scheduling but no longer flips the user's AutoContinueText setting OFF. No-progress and nonretryable admission brakes retain existing behavior. This does not enable formerly disabled preferences.
- P3: handoff remains one derivative per session revision and includes TaskPlan, lifecycle observation, disposition and next action. Changed label from "Recent verified progress" to distinguish model narration. Debug reports a recheck flag; UI distinguishes ON/review-recovery.
- Regression source authored; only V8 JavaScript parse check executed PASS. Go/Python full suites, actual Firefox/provider/Nim, run-specific reproduction and restart tests not verified. A model-only recovery is deliberately limited to local no-plan Agent Chat; Run Manager and plan-mode parity remain P2.2.b.
- Next turn: P1.1.b/P1.2.b, audit native failure evidence when registry/History are missing/pruned and retirement archive exists; add deterministic lifecycle resolution and precise native refusal classification without broadening queue authorization. Then P2.2.b (Run Manager, plan mode, context compaction). Preserve this file and append the next turn's exact evidence.
