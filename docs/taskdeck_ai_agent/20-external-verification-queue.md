# HANDOFF 20 — External verification queue

> **ACTIVE INSTRUCTION (2026-11-27):** follow [20-coder-execution-contract.md](20-coder-execution-contract.md). Local coder owns source/test authoring and repair; operator owns execution/delivery. No external-only `BLOCKED`, no repeated completion audit, and no requirement to rewrite every document per PATCH. Historical claims below are retained as history, not current instructions. Latest reviewed source `9a9c350b`: core/Go FAIL; local source repair IN_PROGRESS. Technical acceptance, permissions and evidence requirements remain intact.

> Operator-run worklist. This session may only author source/tests/docs via native COLLECT + PATCH.
> Every item below is PENDING until the operator returns a real receipt (CI URL + conclusion + job/log or named test result).
> No item is marked PASS from a PATCH receipt, a wrapper rc, or untrusted text.

## Scope and sources
- Master checklist: `docs/taskdeck_ai_agent/20-collection-progress-guard-and-legacy-recovery.md` (section 6.1 regression mapping, lines 487-516; CI commands, line 518).
- Go module: `bletonfc/vscode_tasks_menu`; Go working directory for all `go` commands: `vscode_tasks_menu_go`.
- Node tests run from the repository root unless the workflow states otherwise.
- CI workflows consulted: `.github/workflows/taskdeck_agentchat_core.yml` (lines 88-108), `.github/workflows/taskdeck_remote_agent.yml`, `.github/workflows/taskdeck_macos_native.yml`, `.github/workflows/taskdeck_remote_client_provenance.yml`.
- Baseline at authoring: 1/10 phase, 18/36 tasks, 105/144 subtasks, 20/28 gates. Source commit used for CI correlation: `88a909b4d564b9a1f1e3f35959bc2de55d85647d`; remote HEAD at evidence time: `f37560299dd2f8d257b1ddeb5e0b8980f7d7b24f`.

## EXTERNALLY_SUPPLIED_CI_EVIDENCE (operator-provided, already verified by an external monitor)
- Source commit `88a909b4d564b9a1f1e3f35959bc2de55d85647d`.
- Agent Chat protocol core: run 45937055745 — PASS; core job 140302570352.
- Remote Agent Go: run 45937055780 — PASS.
- Remote Portal PHP: run 45937055761 — PASS.
- Confirmed PASS tests: TestAgentRunAutomaticContinuationRevokedAtTransportAdmission (and /after_build, /before_retry, /explicit_owner_turn), TestAgentCLP20PreparedReplanRevocationWinsEgressCAS, TestAgentRunCollectionProjectionRequiresExactPendingCallAcrossSnapshots.
- This evidence applies ONLY to that source commit. It does not cover later untested source changes and does not by itself close a regression gate whose acceptance is broader than the named tests. No re-verification of these runs is requested.

## V-BATCH-01 — Core agent chat / agent run / server suites (CLP-09.01.02, CLP-09.01.03; gates R01-R09, R12-R17, R19-R23, R26)
- Source files: `vscode_tasks_menu_go/internal/agentchat/...`, `vscode_tasks_menu_go/internal/agentrun/...`, `vscode_tasks_menu_go/internal/server`, `vscode_tasks_menu_go/internal/taskmanifest`.
- Test functions: packages in `./internal/agentchat/...`, `./internal/agentrun/...`, `./internal/server -run ^TestAgentCLP20`, `./internal/server -run ^TestAgent`, race subset `^(TestAgentRun|TestAgentConversation|TestAgentCoordination|TestAgentContext|TestAgentTask|TestAgentProvider)`, `./internal/taskmanifest`.
- Purpose: full targeted + package suites, go vet, and race detector for save/reconcile/lease/counters.
- Working directory: `vscode_tasks_menu_go`.
- Exact commands:
  - `gofmt -l ./internal/agentchat ./internal/agentrun ./internal/server ./internal/taskmanifest` (expect empty output)
  - `go test -race -count=1 ./internal/agentchat/...`
  - `go test -v -race -count=1 ./internal/agentrun/...`
  - `go test -v -count=1 ./internal/server -run '^TestAgentCLP20'`
  - `go test -count=1 ./internal/server -run '^TestAgent'`
  - `go test -v -race -count=1 ./internal/server -run '^(TestAgentRun|TestAgentConversation|TestAgentCoordination|TestAgentContext|TestAgentTask|TestAgentProvider)'`
  - `go test -race -count=1 ./internal/taskmanifest`
  - `go vet ./internal/agentchat/... ./internal/agentrun/... ./internal/taskmanifest`
  - `go vet ./internal/server`
  - `go test -vet=off -run '^$' ./...` (compile every package and test)
- Expected result: all commands exit 0; named CLP20 tests PASS; no race reports.
- Evidence to return: CI URL + conclusion, job/log excerpt with per-package PASS lines, exact exit codes, source SHA under test.
- Safety conditions: synthetic fixtures only; no production provider credentials or real native tool side effects; helper-process tests skip by design (documented, not a failure).
- Dependencies: none. Status: PENDING.

## V-BATCH-02 — Web/Node UI contracts (CLP-09.01.02; gate R24 partial)
- Source files: `vscode_tasks_menu_go/web/agentchat_bootstrap_test.cjs`, `vscode_tasks_menu_go/web/terminalchatcontext_test.cjs`, `vscode_tasks_menu_go/web/featuremods/*.js`.
- Test functions: Node CJS test files listed in `taskdeck_agentchat_core.yml` lines 106-110.
- Purpose: report/status/clear, tab and bootstrap UI protocol compatibility.
- Working directory: `vscode_tasks_menu_go` (commands below use `web/...`).
- Exact commands:
  - `node --check web/featuremods/terminalchatcontext.js`
  - `node web/terminalchatcontext_test.cjs`
  - `node web/agentchat_bootstrap_test.cjs`
  - `node --check web/featuremods/agentproviderselector.js`
  - `node --test web/agentchat_coordination_test.cjs web/agentchat_runs_isolation_test.cjs web/agentproviderselector_test.cjs`
- Expected result: all exit 0.
- Evidence to return: CI job/log excerpt, exit codes, source SHA.
- Safety conditions: no production browser session; smoke UI only in a disposable environment.
- Dependencies: none. Status: PENDING.

## V-BATCH-03 — Durable exactly-once, crash and race matrix (CLP-03.01.04, CLP-03.02.03/.04; gates R10, R27, R28)
- Source files: `vscode_tasks_menu_go/internal/agentchat` (store/CAS, attempt ledger, counters), `vscode_tasks_menu_go/internal/agentrun`, `vscode_tasks_menu_go/internal/server`.
- Test functions: TestCollectionGuardSessionCASAndRestart, TestCollectionGuardReconcilesAttemptLedgerOnceAcrossRestart, TestCounterCursorRejectsConflictAndDoesNotCountOldReplay, TestAcquireRunFenceExcludesSecondOwner, TestAgentProviderRealSIGKILLAtEgressBoundaries, TestAgentCLP20MixedLegacyRestartMatrix, TestAgentCLP20InterruptedCollectionCheckpointProof, TestAgentCLP20StopDuringPreparedResumePreservesUnknown, TestAgentStopCancelsDurablePreparedRunBeforeEgress, TestStoppedWaitingToolCanContinueOnlyFromReconciledResult; resume-fence predicates: TestUnknownCollectionDeliveryMarkerIsTerminalAndNarrow, TestCollectionResumeReceiptRequiresExactCanonicalPair, TestCanSendAfterUnknownCollectionRequiresTerminalToolCalls, TestCollectionAutoResumeAllowedNeedsLiveOwnerConsent, TestCollectionResumeHistoryRejectsReplayAndConflictingProof; W1–W7 crash windows (collection_observation_crash_test.go): TestCLP20ObservationCrashW1BeforeNative, TestCLP20ObservationCrashW2StagedNotCommitted, TestCLP20ObservationCrashW3CommittedBeforePrepared, TestCLP20ObservationCrashW4PreparedBeforeSending, TestCLP20ObservationCrashW5SendingBeforeAck, TestCLP20ObservationCrashW6RestartAfterAck, TestCLP20ObservationCrashW7ConcurrentOneWinner, TestCLP20CrashReopenNeedsFreshOwnerAuthorization.
- Purpose: crash before native execution, after native execution before observation commit, after commit before PREPARED, PREPARED before SENDING, SENDING before ACK, restart after ACK, concurrent save/reconcile of one logical observation, explicit owner continuation after Stop/restart; count exactly once; no stale overwrite; no double count.
- Working directory: `vscode_tasks_menu_go`.
- Exact commands:
  - `go test -race -count=1 ./internal/agentchat -run '^(TestCollectionGuardSessionCASAndRestart|TestCollectionGuardReconcilesAttemptLedgerOnceAcrossRestart|TestUnknownCollectionDeliveryMarkerIsTerminalAndNarrow|TestCollectionResumeReceiptRequiresExactCanonicalPair|TestCanSendAfterUnknownCollectionRequiresTerminalToolCalls|TestCollectionAutoResumeAllowedNeedsLiveOwnerConsent|TestCollectionResumeHistoryRejectsReplayAndConflictingProof)$'` (last five: resume-fence predicate regression, collection_resume_predicates_test.go)
  - `go test -v -race -count=1 ./internal/agentchat/collectionprogress -run '^TestCounterCursorRejectsConflictAndDoesNotCountOldReplay$'`
  - `go test -v -race -count=1 ./internal/agentrun -run '^(TestAcquireRunFenceExcludesSecondOwner|TestStoppedWaitingToolCanContinueOnlyFromReconciledResult)$'`
  - `go test -v -race -count=1 ./internal/server -run '^(TestAgentProviderRealSIGKILLAtEgressBoundaries|TestAgentCLP20MixedLegacyRestartMatrix|TestAgentCLP20InterruptedCollectionCheckpointProof|TestAgentCLP20StopDuringPreparedResumePreservesUnknown|TestAgentStopCancelsDurablePreparedRunBeforeEgress)$'`
  - `go test -v -count=1 ./internal/agentchat -run '^(TestCLP20ObservationCrashW1BeforeNative|TestCLP20ObservationCrashW2StagedNotCommitted|TestCLP20ObservationCrashW3CommittedBeforePrepared|TestCLP20ObservationCrashW4PreparedBeforeSending|TestCLP20ObservationCrashW5SendingBeforeAck|TestCLP20ObservationCrashW6RestartAfterAck|TestCLP20ObservationCrashW7ConcurrentOneWinner|TestCLP20CrashReopenNeedsFreshOwnerAuthorization)$'` (W1-W7 crash windows listed in this batch's Test functions; same command also in V-BATCH-08; Linux/darwin build tag, isolated t.TempDir() fixtures).
- Expected result: all exit 0 under race detector; counter deltas exactly one; canonical receipt unchanged; no replayed native call; no ACK resend; isolation holds.
- Evidence to return: CI URL + conclusion, per-test PASS lines, race report absence, source SHA, exact crash window observed in fixture logs.
- Safety conditions: SIGKILL only against fixture subprocesses; no production daemon; no real provider/network egress; synthetic workspace.
- Dependencies: V-BATCH-01 should be green first. Status: PENDING.

## V-BATCH-04 — Opt-out, revocation and live-consent at transport (CLP-01.04.02, CLP-04.03.03; gate R22)
- Source files: `vscode_tasks_menu_go/internal/server/agent_run_provider.go`, `agent_chat_continuation.go`, `agent_chat_plan_coordinator.go`, `agent_run_continuation.go`.
- Test functions: TestAgentCLP20StallRequiresExactDuplicateAndLiveConsent, TestAgentRunContinuationConsentAndUnknownEgressFence, TestAgentStandaloneContinuationRequiresUserConsent, TestAgentRecoveredCollectionResumeRequiresGrantAndRunsOnce, TestAgentCLP20PreparedReplanRevocationWinsEgressCAS, TestAgentRunAutomaticContinuationRevokedAtTransportAdmission.
- Purpose: confirm AutoCollection/Patch/Feedback/Continue opt-out honored at every gate including recovery, Unified per-attempt admission, after-build and before-retry revocation; no egress after revocation; unknown egress fenced.
- Working directory: `vscode_tasks_menu_go`.
- Exact commands:
  - `go test -v -race -count=1 ./internal/server -run '^(TestAgentCLP20StallRequiresExactDuplicateAndLiveConsent|TestAgentRunContinuationConsentAndUnknownEgressFence|TestAgentStandaloneContinuationRequiresUserConsent|TestAgentRecoveredCollectionResumeRequiresGrantAndRunsOnce|TestAgentCLP20PreparedReplanRevocationWinsEgressCAS|TestAgentRunAutomaticContinuationRevokedAtTransportAdmission)$'`
- Expected result: all exit 0; zero invocations after revocation; canonical receipt intact; explicit owner turn still authorized.
- Evidence to return: CI URL + conclusion, per-subtest PASS (after_build/before_retry/explicit_owner_turn), source SHA. Note: named tests already PASS for commit `88a909b4...` per EXTERNALLY_SUPPLIED_CI_EVIDENCE; remaining scope is Unified/all-automatic-path coverage — return whether that broader scope is now fully exercised before closing R22.
- Safety conditions: fake provider only; no real credentials; consent toggles in synthetic session.
- Dependencies: none. Status: PENDING.

## V-BATCH-05 — Native mutation, selective invalidation and pipeline parity (CLP-02.04.01/.02, CLP-04.02.03/.04; gates R18, R25)
- Source files: `vscode_tasks_menu_go/internal/agentchat/collectionprogress`, `vscode_tasks_menu_go/internal/server/agent_run_collection_test.go`, `agent_chat_collection_flow.go`.
- Test functions: TestAgentCLP20NativePatchMutationGuardMatrix, TestAgentCLP20NativeRangeExternalMutationInvalidatesCoverage, TestAgentRunCollectionRepeatedShapeReadsChangedSource, TestAgentCLP20CrossPipelineRetryMatrix, TestConfiguredAdapterCLP20BoundedRecovery (mutation/parity); R18 streamed function-call denial: TestChatCompletionsStreamRejectsMissingFinishAndInvalidFragments (agentchat/chat_completions_stream_test.go:42 — missing finish reason, malformed fragmented arguments, call-ID change).
- Purpose: authoritative mutation proof, selective invalidation only with full proof, broad invalidation otherwise, rename/delete, external drift, partial PATCH failure, typed/text fallback, partial streamed function call denial, unsupported tool denial, native execution count, Legacy/Unified parity.
- Working directory: `vscode_tasks_menu_go`.
- Exact commands:
  - `go test -v -count=1 ./internal/server -run '^(TestAgentCLP20NativePatchMutationGuardMatrix|TestAgentCLP20NativeRangeExternalMutationInvalidatesCoverage|TestAgentRunCollectionRepeatedShapeReadsChangedSource|TestAgentCLP20CrossPipelineRetryMatrix)$'`
  - `go test -v -count=1 ./internal/agentchat -run '^TestConfiguredAdapterCLP20BoundedRecovery$'`
  - `go test -v -count=1 ./internal/agentchat -run '^TestChatCompletionsStreamRejectsMissingFinishAndInvalidFragments$'` (R18 streamed denial)
  - `go test -v -count=1 ./internal/server -run '^(TestAgentRunCollectionPartialResultSafeRejectionPinnedBinding|TestCLP040204NativeCollectionLeaseContentionAndWorktreeIsolation)$'` (CLP-04.02.03/.04: partial/invalid-result safe rejection; lease contention & worktree isolation).
  - `go test -v -count=1 ./internal/agentchat/collectionprogress -run '^TestMutationProof(CompleteProofAllowsSelective|IncompleteOrForeignFallsBack|TraversalRejected)$'` (CLP-02.04.01/.02: fail-closed authoritative mutation proof adapter; broad invalidation preserved).
  - `go test -v -count=1 ./internal/agentchat -run '^TestR18PartialStreamedPatchCallNeverBecomesExecutable$'` (R18 end-to-end fake-transport partial stream denial through real provider adapter).
- Expected result: all exit 0; no selective invalidation without authoritative proof; UI changed-files report not treated as complete mutation evidence.
- Evidence to return: CI URL + conclusion, per-test PASS lines, source SHA.
- Safety conditions: native calls against synthetic workspace only; no production source mutation.
- Dependencies: none. Status: PENDING.

## V-BATCH-06 — Upgrade smoke and rollback (CLP-09.02; gates R11, R24)
- Source files: self-update paths under `vscode_tasks_menu_go/internal/selfupdate`, installer scripts `install.sh`, `self-install-and-update.sh`.
- Purpose: build from checked-out source, record build SHA, create synthetic backup of legacy state, run self-update/reload/restart smoke, verify migration/recovery/counters/ACK; exercise auto flags, provider switch, Stop, concurrency and rollback compatibility.
- Working directory: repository root.
- Exact commands (manual procedure; do NOT run in this session): build per `.github/workflows/vscode_tasks_menu_go.yml` (native CLI build step), then run the staged self-update smoke described there; run `.github/workflows/taskdeck_selfupdate_multi_instance.yml` steps against a synthetic instance.
- Expected result: upgrade preserves canonical history, receipts, ACK and counters; rollback restores prior state; no production data used.
- Evidence to return: build SHA, synthetic-state snapshots before/after, CI/manual log, exit codes.
- Safety conditions: disposable instance only; never point at production paths or real provider credentials.
- Dependencies: V-BATCH-01 green. Status: PENDING.

## V-BATCH-07 — Delivery (CLP-09.03.03/.04)
- Source files: `docs/taskdeck_ai_agent/20-collection-progress-guard-and-legacy-recovery.md`, `TASKDECK_HANDOFF_20_CHECKPOINT.md`, this queue.
- Purpose: commit code + tracker to `main`, push, verify ancestry and combined CI, then write the final report.
- Working directory: repository root.
- Exact commands (operator-only, not executed here): `git fetch origin main`; `git add -- <task files> docs/taskdeck_ai_agent/20-collection-progress-guard-and-legacy-recovery.md docs/taskdeck_ai_agent/20-external-verification-queue.md`; `git commit -m "fix(agent): HANDOFF 20 source implementation"`; `git push origin main`; `git merge-base --is-ancestor <commit-sha> origin/main`.
- Expected result: commit present on remote, ancestry verified, combined CI green after push, checklist rollups updated only from verified results.
- Evidence to return: commit SHA, remote HEAD, ancestry check exit 0, CI URLs.
- Safety conditions: reconcile concurrent commits; no force push, no reset hard, no `git add .`; preserve changes outside task scope.
- Dependencies: V-BATCH-01..06 resolved. Status: PENDING.

## Notes
- This queue documents commands only. Nothing here was executed by the authoring session.
- Audit refinements (see 20-source-completeness-audit.md §3): V-BATCH-03 needs a new narrowly scoped test-only Store commit-barrier hook for W1–W7 (existing task-plan crash fixtures do NOT cover the collection-observation boundary); V-BATCH-05 must include an explicit partial streamed function-call JSON denial assertion for R18. Compile/test batches (01/02/05) are non-destructive; SIGKILL/self-update/rollback (03/06) require isolated fixtures only; git (07) is operator-only and is not a prerequisite for local batches.
- Return real receipts before changing any PENDING status or promoting a gate/checklist checkbox.


## V-BATCH-00 — Compile regression preflight (operator only)
2026-11-27 — Remote review of user-pushed HEAD `9e4ca30aa299d8538006e9a763fac8d0d3409e01` (17 commits ahead of `f3756029`, 36 changed files). Core 45961114805 FAIL: job 140378893842, `internal/agentchat/collectionprogress/recovery_boundary_test.go:48:28` assignment mismatch (ApplyCounterEvent returns FOUR values; use `replayed, _, applied, err := ...`); job 140378894346, `internal/server/agent_chat_clp20_guard_boundary_test.go:20:10` invalid indirect (NewSession returns Session value; use `return x`). Same failures in core 45953634340/45953522819 and server failure in Remote Go 45953522802. PHP 45953522701 PASS on 190d7d89 only. Fix test source first, no weakened assertions. No full current-source PASS, no gate promoted. Prior CI at 88a909b4 (core 45937055745/Go 45937055780/PHP 45937055761) remains historical PASS. Counted 106/144 checked leaves, 38 unchecked; 1/10 phase, 18/36 tasks, 20/28 historical gates. Current combined acceptance remains unverified.

After coder patches both errors, from `vscode_tasks_menu_go` run `go test -count=1 -run '^$' ./internal/agentchat/collectionprogress ./internal/server` (expect exit 0), then `go test -v -count=1 ./internal/agentchat/collectionprogress -run '^TestRecovery'` and `go test -v -count=1 ./internal/server -run '^TestAgentCLP20(ReadOnlyProgressIsNotCodingStall|AcceptanceResetsStallWithoutRecovery|PendingContinuationBindsNeutralFallbackWithoutPatch)$'`. Capture named PASS/FAIL and source SHA. Do not interpret no tests to run as PASS. Then V-BATCH-01..06. V-BATCH-03 is not yet sufficient for W1–W7: add exact test names/commands after source implementation. V-BATCH-07 delivery partially performed by operator push through 9e4ca30a; final ancestry/CI still pending. Historical package filters corrected: TestCounterCursorRejectsConflictAndDoesNotCountOldReplay in collectionprogress, TestStoppedWaitingToolCanContinueOnlyFromReconciledResult in agentrun. Do not ask local coder to execute any batch.


## V-BATCH-08 — Post-push remediation regression (operator only; PENDING)
Working directory: `vscode_tasks_menu_go`. Prerequisite: V-BATCH-00 compile preflight PASS first (P0 fixes + new marker must compile). Non-destructive except W* crash fixtures — those spawn a child process and SIGKILL it; run only against isolated temp fixtures (tests use t.TempDir()).
Commands:
- `go test -v -count=1 ./internal/agentchat -run '^(TestCLP20ObservationCrashW1BeforeNative|TestCLP20ObservationCrashW2StagedNotCommitted|TestCLP20ObservationCrashW3CommittedBeforePrepared|TestCLP20ObservationCrashW4PreparedBeforeSending|TestCLP20ObservationCrashW5SendingBeforeAck|TestCLP20ObservationCrashW6RestartAfterAck|TestCLP20ObservationCrashW7ConcurrentOneWinner|TestCLP20CrashReopenNeedsFreshOwnerAuthorization)$'` (P1/R10/R27/R28, Linux/darwin build tag).
- `go test -v -count=1 ./internal/server -run '^TestAgentCLP20PatchFeedbackResumeRevocationWinsEgressCAS$'` (P3/R22).
- `go test -v -count=1 ./internal/server -run '^TestAgentCLP20PreparedReplanRevocationWinsEgressCAS$'` (regression check existing CollectionReplan path unaffected by new marker).
- `go test -v -count=1 ./internal/server -run '^TestAgentCLP050204RebaseAfterArchivedHistoryRetainsGoalAndReceipt$'` (CLP-05.02.04: rebase after archived multiday history retains goal + byte-exact receipt, no stale provider refs).
- `go test -v -count=1 ./internal/server -run '^(TestAgentChatLegacyCollectionFeedsPatchProposal|TestAgentToolContinuationExactCallIDAndOnceOnly)$'` (P0 tool-contract assertions after taskdeck_ask_user).
- `go test -v -count=1 ./internal/server -run '^(TestCLP20PipelineW1PreNativeInterruptionNoExecution|TestCLP20PipelineW2NativeReceiptThenInterruptionUnknown|TestCLP20PipelineW3ReducerExactCounterDelta|TestCLP20PipelineW7ConcurrentReconcileOneWinner|TestCLP20PipelineStopRestartExplicitOwnerResume)$'` (P1/R10/R27/R28 native-pipeline integration: real provider->AgentCollectionCall->canonical receipt->counter reducer->startup reconcile->explicit resume; source file agent_chat_clp20_pipeline_crash_test.go).
Expected: exit 0, named tests PASS (not "no tests to run"). Evidence: console log with each named PASS/FAIL and source SHA. Related: CLP-03.01.04/03.02.03/.04/09.01.01 (R10/R27/R28), CLP-04.03.03/R22. Safety: crash tests use isolated t.TempDir() stores; do not point at running production TaskDeck. Failure handling: return FAIL handoff; do not promote gates on PATCH receipt alone. Status: PENDING.


## RESOLUTION — queue fully wired (2026-11-27; SUPERSEDES stale "not yet sufficient" notes)

> The "Audit refinements" Note (line 126) and V-BATCH-00 instructions (line 133) were written at the remote post-push review of `9e4ca30a...` and still say V-BATCH-03 "needs a new test-only Store commit-barrier hook for W1–W7" / "is not yet sufficient for W1–W7: add exact test names/commands after source implementation", and that R18 "must include an explicit partial streamed function-call JSON denial assertion." Those statements describe the state at review time; the source and this queue have since been completed. This note supersedes those specific lines; the historical text is retained unchanged.

Verified current state (native COLLECT, Coverage VERIFIED):
- W1–W7 crash command is present in V-BATCH-03 at line 69 (TestCLP20ObservationCrashW1..W7 + TestCLP20CrashReopenNeedsFreshOwnerAuthorization), so the batch now runs the crash windows it only listed before; receipt 1795774851625418_712988. No separate Store commit-barrier hook was required — the authored tests reuse the existing `Store.syncDirectoryOverride` seam (per CLP-03.02.03/.04 checklist entries) and the agentchat journal CAS layer.
- R18 end-to-end fake-transport denial test `TestR18PartialStreamedPatchCallNeverBecomesExecutable` is registered in V-BATCH-05 at line 98 (receipt 1795770222563964_435876), complementing the earlier parser test at line 95. Source drives the real HTTPProviderInvoker -> DoChatCompletionsStreamWithEvents adapter; a parser-only assertion would not have satisfied R18.
- P1 native-pipeline integration tests (TestCLP20PipelineW1/W2/W3/W7 + StopRestart) are registered in V-BATCH-08 (receipt 1795774665546390_700887).

All nine batch headers V-BATCH-00..08 are present, each with working directory, exact commands, dependencies and Status PENDING (as required — nothing executed by the authoring session). V-BATCH-00 remains the required compile preflight gate; V-BATCH-07 (Git/delivery) is operator-only and not a prerequisite for local test batches. Real per-test receipts still must be returned before any PENDING status, gate or checkbox is promoted; no runtime PASS is claimed from this note.

## Latest operator evidence and priority preflight
At 9a9c350b core 45985473641/Go 45985473473 FAIL; PHP 45985473638 PASS. Source repair FIX-01..05 is now active; see execution contract. After source repair run V-BATCH-00, then from vscode_tasks_menu_go:

```sh
go test -v -race -count=1 ./internal/agentchat -run '^TestCLP20ObservationCrash'
go test -v -race -count=1 ./internal/agentchat/collectionprogress -run '^TestMutationProof'
go test -v -count=1 ./internal/server -run '^TestFeedbackResume'
```

Capture real named results, exit codes and SHA; no-tests-to-run is not acceptance. Then remaining batches as appropriate. Operator-only; local coder does not wait for these results before other independent source changes.
