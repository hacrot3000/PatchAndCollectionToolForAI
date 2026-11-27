# HANDOFF 20 — Unified bounded recovery audit (P2: CLP-01.04.02/.04, CLP-04.03.03/.04, R25)

> Static source audit from native read-only COLLECT. No compile, test or runtime
> execution. This documents the precise supported scope and limit; it does NOT
> claim SOURCE_IMPLEMENTATION_COMPLETE for P2 and promotes no gate.

## 1. Verified current architecture
- `internal/server/agent_run_collection_progress.go` (SHA 486a3473…, 103 lines):
  - `runCollectionObjective` binds a NEUTRAL objective (Goal "Continue", generation 1) from the immutable RunSpec — no reconstruction of owner instructions from generated text.
  - `readRunCollectionProgress` validates a bounded (<=96 KiB) `runCollectionProgress` projection pinned to the exact objective key, canonical history prefix and sequence arithmetic (`LastSequence == 2*LastHistorySize-1`). Tampered/stale/unknown projections fail closed.
  - `runCollectionProgressUpdate` is the observation commit boundary: it requires the single exact unresolved call with matching OriginRunID/ProviderID/ModelID/ParentTurnID, an unchanged history prefix, applies `ApplyCounterEvent` twice (request+observe), invalidates evidence/coverage on intervening Patch mutation, and bounds output. A stale worker receipt cannot manufacture a second observation.
  - `runCollectionProgressAdvice` (line 98) returns `collectionProgressAdvisory(g.Decision, g.Counters, false)` — advisory text only; the `false` explicitly withholds Patch availability.
- Consumer: `internal/server/agent_run_provider_executor.go:168` appends the advice to provider instructions. It is INPUT to the model, not an execution trigger.
- Pure recovery reducer exists: `collectionprogress.AssessRecovery(policy, state, event, RecoveryState, RecoveryGate)` with `RecoveryGate{SafeEgress, CanonicalOutputCommitted, OwnerAllowsContinuation, Stopped}` and decisions Replan/Checkpoint/Continue. It is consumed on the LEGACY path (`agent_chat_plan_coordinator.go`); it is NOT consumed by `runCollectionProgressAdvice`.

## 2. Genuine gap
Unified's progress decision `g.Decision` is produced by `ApplyCounterEvent`, not `AssessRecovery`. Therefore bounded recovery generation/budget (CLP-01.04.02/.04) is NOT integrated into any Unified continuation path that can act on it. Registering `AssessRecovery` for Unified would require: a persisted `RecoveryState` in `runCollectionProgress`, a `RecoveryGate` derived from the live run's Stop/consent/egress/canonical-output state, and a bounded advisory emission — none of which exist today.

## 3. Precise supported limit (per task rule: do not change drain-only policy)
- Unified creation is DRAIN-ONLY: there is no automatic creation/replan execution path, and the review forbids adding one. Consequently recovery in Unified can only be ADVISORY at this architecture state; automatic bounded-recovery execution is NOT feasible without a policy change that is explicitly out of scope.
- Safe feasible scope already present and preserved: neutral objective binding, exact-call observation commit, stale-projection rejection, mutation invalidation, bounded advisory output with Patch withheld, owner/worktree/provider binding, and no replay of native receipts.
- What is NOT supported and must remain documented, not silently assumed: automatic replan admission, recovery-budget carry across generations from the advisory path, and any execution triggered by the advisory text.

## 4. Invariants that MUST hold for any future integration
1. Never enable Unified creation or auto-replan to reach acceptance (drain-only stays).
2. Never bypass owner consent, Stop, or the UNKNOWN egress fence.
3. Never replay a native receipt or create a new execution pipeline.
4. Never change immutable run binding; never reset recovery generation on provider retry or reload.
5. Never produce a continuation storm: one recovery reservation per generation, bounded by MaxRecoveryAttempts/no-progress hysteresis, with Checkpoint at exhaustion.

## 5. Status
This audit closes the documentation half only. CLP-01.04.02/.04, CLP-04.03.03/.04 and R25 remain VERIFY_PENDING / IN_PROGRESS: authoritative source integration beyond advisory is intentionally NOT attempted because the current architecture is drain-only and the task forbids enabling automatic Unified creation or replan. Runtime proof of the advisory path and of Legacy recovery remains operator work (V-BATCH-01/03/05). No gate promoted; no test disabled or weakened.


## 6. Resolution — bounded recovery integrated (drain-only preserved) 2026-11-27

> This section SUPERSEDES the Section 2 "genuine gap" and Section 5 "not attempted" wording above. Those described the state at audit time; the feasible recovery-state integration has since been implemented via native PATCH. The historical audit text is retained unchanged.

Verified current source (`vscode_tasks_menu_go/internal/server/agent_run_collection_progress.go`, native COLLECT Coverage VERIFIED):
- Line 21: `Recovery cp.RecoveryState \`json:"recovery,omitempty"\`` added to the persisted progress projection (omitempty preserves zero-value backward compatibility).
- Lines 54-57: validation bounds on `Recovery.Attempts <= MaxRecoveryAttempts`, `LastSequence <= Counters.LastSequence`, `LastAttemptReads <= Counters.NativeReads`, and paired-zero/nonzero invariants — a tampered/stale recovery state fails closed.
- Line 107: `nextRecovery,_ := cp.AssessRecovery(policy, c, observedEvent, g.Recovery, cp.RecoveryGate{Stopped, SafeEgress, CanonicalOutputCommitted, OwnerAllowsContinuation})` — the pure reducer is now actually consumed on the Unified progress-update path (previously only Legacy consumed it); gates derived from run Status and immutable snapshot Automation.
- Line 108: `g.Recovery = nextRecovery` persists the advanced budget across observations; provider retry/reload cannot rearm it (reducer returns unchanged state at exhaustion).
- Lines 119-122: a reserved recovery generation yields a BOUNDED ADVISORY (`collectionReplanAdvice`) only. Drain-only policy is preserved: no run creation, no replan execution, no permission grant, no new pipeline, no native receipt replay.

Regression source: `internal/server/agent_run_recovery_state_test.go` (receipt 1795769748557946_406767; companion 1795769589553551_397251/1795769628590192_399870) — real assertions: zero-value default, JSON roundtrip equality, bounded permission-neutral advisory, and exhausted-budget-not-rearmed -> Checkpoint. Registered in V-BATCH-01/03. All VERIFY_PENDING (PATCH PASS = applied only; no Go test executed in the authoring session).

Remaining scope unchanged: automatic replan EXECUTION stays deferred by the drain-only policy (explicitly out of scope). R25/CLP-01.04.02/.04, CLP-04.03.03/.04 stay VERIFY_PENDING until the operator runs the advisory-path and Legacy recovery tests with real receipts. No gate promoted; no test weakened.
