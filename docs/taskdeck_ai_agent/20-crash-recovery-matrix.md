# HANDOFF 20 — Exactly-once crash/reconcile/retry matrix (CLP-03.02.03/.04)

> Design and gap audit derived from read-only COLLECT of the actual source.
> No test was executed. Do NOT treat this document as PASS evidence for R10/R27/R28.
> Gates: R10 (native observation exact crash window), R27 (restart after Stop), R28 (concurrent save/reconcile/retry of one logical call).

## 1. Verified existing crash-injection seams (exact sources)
- `vscode_tasks_menu_go/internal/agentchat/store.go`:
  - `afterSegmentStatusJournalCommit func()` (test-only hook, declared near line 288) — invoked inside `task_plan_segment_status.go` only, after the canonical journal rename but before post-commit cleanup while `withLock` is held.
  - `syncDirectoryOverride func() error` (test-only, near line 291) — failure injection at directory-fsync boundaries; production uses kernel fsync.
- `task_plan_forced_crash_test.go`: child opens a real `Store`, stages a durable segment via `stageTaskPlanSegments` under `withLock`, prints marker `TASKDECK_SEGMENT_STAGED_BEFORE_JOURNAL_COMMIT`, then sleeps; parent performs a real OS `Process.Kill()` and reopens a fresh `Store` to assert the uncommitted milestone did not survive and orphan recovery reclaims it. Constants: `forcedCrashChildEnv`, `forcedCrashMarker`.
- `task_plan_postcommit_crash_test.go` (`//go:build linux || darwin`): child sets `store.afterSegmentStatusJournalCommit`, calls `SetSegmentTaskPlanStepStatus`, prints marker `TASKDECK_STATUS_JOURNAL_COMMITTED_CLEANUP_NOT_RUN`; parent reads raw journal bytes under the held lock, kills the child, and asserts exactly-once commit plus obsolete-page cleanup with owner isolation. Constants: `postCommitCrashChildEnv`, `postCommitCrashMarker`.
- `provider_attempts.go`: `providerAttemptSidecar` (Version 1, RunID, Total, Attempts, SHA256), validated by `readProviderAttemptsLocked` (max 128 records, 64 KiB, sequential attempt numbers, digest check); `recordProviderAttempt` enforces `record.Total+1==attempt` under the store lock. `appendProviderAttempt` and digest verification already provide idempotent reconciliation of attempt counts across restart (132-attempt test PASS at 45592281917 per tracker).
- `collection_resume.go`: `retainCollectionResumeCheckpoint` runs under the store lock after CAS/owner/staged-payload verification; it copies the attempt audit into `CollectionResumeHistory` (bounded 16) and never overwrites an attempt sidecar. `CollectionResumeReceipt`/`CollectionAutoResumeAllowed` re-check against canonical journal at admission.

## 2. Gap analysis (why a naive crash test cannot be authored here)
- The two proven forced-crash fixtures exercise the TASK PLAN commit path. There is no `afterCollectionObservationCommit`-style hook on the collection observation Save path in the inspected sources. A process-kill test that referenced such a hook would fail to compile.
- Adding a new production test-only hook to the observation Save path is a source change that must be compile- and race-verified by the operator; it is NOT smuggled in here. This document records the precise requirement so that change can be made deliberately.
- Therefore CLP-03.02.03/.04 remain VERIFY_PENDING, not DONE. Test source that depends on a new seam is intentionally not authored in this session.

## 3. Required crash windows and expected exactly-once outcomes
For each window: spawn a child via `os.Args[0]` with a dedicated env marker (pattern from the two fixtures), wait for a stdout marker, real `Process.Kill()`, reopen a fresh `Store`, assert outcomes.

| ID | Window | Expected after restart | Test function (proposed, package agentchat) |
|----|--------|------------------------|---------------------------------------------|
| W1 | before native observation execution | no observation row, counters unchanged, no ACK | TestCollectionObservationCrashBeforeNativeRun |
| W2 | after native run, before observation commit (durable receipt staged, manifest still old) | receipt staged but NOT adopted; next explicit resume re-observes exactly once, no double count | TestCollectionObservationCrashAfterNativeBeforeCommit |
| W3 | after observation commit, before PREPARED | observation counted once; new prepared run retains prior UNKNOWN marker; no replay | TestCollectionObservationCrashAfterCommitBeforePrepared |
| W4 | PREPARED before SENDING | prepared run identity/digest/provider pinned; SENDING not assumed; no egress | TestCollectionPreparedCrashBeforeSending |
| W5 | SENDING before provider ACK | delivery UNKNOWN, fenced; late response cannot publish tool call/ACK | TestCollectionSendingCrashBeforeAck |
| W6 | restart after ACK | ACK preserved exactly once; counters not incremented again | TestCollectionRestartAfterAckExactlyOnce |
| W7 | concurrent save/reconcile/retry of one logical call | one CAS winner, one stale writer, exactly one count after reopen | TestCollectionConcurrentReconcileOneWinner |

Cross-cutting assertions (all windows): canonical receipt byte-exact; owner/project/worktree isolation; immutable provider binding; native reads not double counted; retry/reconcile/reload count once; no fabricated ACK; no native call replayed by the guard.

## 4. Implementation prerequisites (operator / compile-verified follow-up)
1. Add a narrowly scoped test-only hook on the collection observation Save path, mirroring `afterSegmentStatusJournalCommit` (unexported field on `Store`, settable only in-package, nil in production). Invoke it inside the observation commit's `withLock`, after the durable write but before the canonical manifest swap, for W2.
2. Add a pre-commit hook (before the durable write) for W1/W3 boundaries if not already covered by `syncDirectoryOverride`.
3. Reuse the existing child-process marker/kill/reopen pattern verbatim; keep `//go:build linux || darwin` where OS process semantics require it.
4. Run under `go test -race`; confirm no lingering child and no orphaned lock. Do not weaken any existing assertion.

## 5. Regression-gate mapping
- R10 needs W1–W3 with an exact native-observation crash window (currently only separate egress crash/restart + guard idempotence pass).
- R27 needs W4/W5/W6 with explicit owner restart→continue after Stop with a fresh grant.
- R28 needs W7 plus the existing independent-store CAS race (core 45930958336 PASS) and Unified staged/replayed projection (45930921487 PASS).
- Operator worklist: `20-external-verification-queue.md` V-BATCH-03. CI commands are in `.github/workflows/taskdeck_agentchat_core.yml` and `taskdeck_remote_agent.yml` (already collected).

## 6. Honest status
No crash test was authored or executed here. The design above is evidence-backed by the actual seams and fixtures; it does not claim PASS. Runtime execution, race detection, and any new test-only production hook belong to the operator with a compiling Go toolchain.
