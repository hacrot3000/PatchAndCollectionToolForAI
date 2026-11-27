# HANDOFF 20 — Regression scenario reconciliation (CLP-09.01.04)

> Reconciliation of R01–R28 against verified evidence. Derived from read-only COLLECT of the
> authoritative §6.1 mapping (checklist lines 487–516). No test was executed here; do NOT
> treat this document as PASS evidence. CLP-09.01.04 stays VERIFY_PENDING because its
> acceptance requires runtime scenario verification (CLP-09.01.02/.03, both TODO here) and
> regression fixes before DONE.

## 1. Baseline
- Regression gates: 20 PASS / 28. Eight VERIFY_PENDING: R10, R11, R18, R22, R24, R25, R27, R28.
- Baseline CI evidence (from §6.1): core 45799935359, Go 45799935418, PHP 45799935450 at HEAD 7965a5ed (PASS). Later operator-supplied CI evidence: core 45937055745 / Go 45937055780 / PHP 45937055761 at source commit 88a909b4d564b9a1f1e3f35959bc2de55d85647d (PASS; job 140302570352; applies ONLY to that commit).
- PASS gates R01–R09, R12–R17, R19–R21, R23, R26 have named test coverage in §6.1; no change.

## 2. The 8 VERIFY_PENDING gates — exact blockers and missing evidence
| Gate | Current verified coverage | Missing evidence to close |
|------|---------------------------|---------------------------|
| R10 | Egress crash/restart + guard/receipt idempotence separately pass (TestAgentProviderRealSIGKILLAtEgressBoundaries, TestCollectionGuardReconcilesAttemptLedgerOnceAcrossRestart). | Native observation EXACT crash window combined test. Design + seam gap audit: `20-crash-recovery-matrix.md` (W1–W3); no collection-observation commit-barrier hook exists yet. |
| R11 | Synthetic old schema/journal restart PASS (TestAgentCLP20ArchivedLegacyPackUnknownResumeAcrossUpgrade, TestAgentCLP20RealPackSchemaUpgradeAcrossRestart). | Real binary self-update; unexecutable here. Procedure: `20-upgrade-rollback-guidance.md`; operator V-BATCH-06. |
| R18 | One bounded max-output recovery + unsafe-retry refusal verified (TestConfiguredAdapterCLP20BoundedRecovery, TestAgentCLP20CrossPipelineRetryMatrix). | Explicit partial Patch / streamed function-call JSON denial test mapping. |
| R22 | Opt-out at multiple gates; persisted CollectionReplan marker + atomic same-revision consent; TestAgentCLP20PreparedReplanRevocationWinsEgressCAS PASS 45931252944; named tests PASS at 88a909b4. | Unified/all-automatic opt-out paths at the transport boundary; acceptance broader than the named PASS tests. Do NOT close on supplied evidence alone. |
| R24 | Core/Node report/status/clear/tab PASS (TestAgentCLP20SafeDebugReportGuardAndRedaction, TestAgentCLP20DebugReportAttemptAuditAfterClear, TestAgentCLP20DebugReportBoundedExportRetainsNewestDomains, agentchat_bootstrap_test.cjs). | Real browser light/dark + self-update smoke; unexecutable here. Operator V-BATCH-06. |
| R25 | 72-case retry parity PASS (TestAgentCLP20CrossPipelineRetryMatrix). | Full native guard/recovery/acceptance parity; Unified creation remains drain-only. |
| R27 | No startup replay on cancelled + Stop wins late provider result (TestAgentCLP20StopDuringPreparedResumePreservesUnknown, TestStopCancelsDurablePreparedRunBeforeEgress, TestStoppedWaitingToolCanContinueOnlyFromReconciledResult). | Combined explicit-owner restart→continue with a fresh Stop grant. Design W4–W6 in `20-crash-recovery-matrix.md`. |
| R28 | Independent Store CAS for one logical receipt + counters PASS under race (core 45930958336); Unified staged/replayed projection PASS (45930921487). | Combined actual native/reconcile/retry under race. Design W7 in `20-crash-recovery-matrix.md`. |

## 3. Failures / skip blockers (from verified tracker notes)
- Helper-process tests TestAgentRunJournalHelperProcess, TestAgentRunRecoveryHelperProcess, TestAgentRunWaitingToolHelperProcess skip in the top-level runner BY DESIGN; their parent process/recovery tests pass. Not an unexplained skip.
- Historical CI failures were repaired, not accepted- weakened: post-signature first-bind regression (45931661571/45931676600/45931695422 → migration-aware reference fix, PASS 45932960307); CLP-10 import path vscode_tasks_menu_go→vscode_tasks_menu; legacy saturation fixture ENOENT → Store.withLock init; unused Legacy import; shrink assertion / typed ACKed failed-Patch summary; non-JSON function_call arguments in debug-redaction fixture. All recorded in §8.1 log; do not reintroduce the failures.
- No regression test was deleted, disabled or relaxed in this session. New test SOURCE was added (see §4) and remains unexecuted.

## 4. Session-contributed evidence (source only, all VERIFY_PENDING, none DONE)
- CLP-01.03.04: counter_boundary_test.go (saturation/no-wrap, max-sequence replay, progress reset).
- CLP-01.04.04: recovery_boundary_test.go (no continuation storm, novelty resume, pending-pair gates, invalid state).
- CLP-04.03.04: agent_chat_clp20_guard_boundary_test.go (read-only progress vs stall, acceptance reset, neutral pending continuation).
- CLP-05.02.01: compact.go bounded catalog status/proof/coverage metadata + catalog_metadata_test.go (test defect found by static review and corrected).
- CLP-01.02.03: objective_transition_guard_test.go (provider switch / Stop / step+worktree transitions).
- CLP-05.01.02: static payload-budget audit (20-payload-budget-audit.md).
- CLP-03.02.03/.04: crash/reconcile matrix + seam gap audit (20-crash-recovery-matrix.md).
- CLP-09.02.04: upgrade/rollback/diagnostic guidance (20-upgrade-rollback-guidance.md) — the one DONE documentation subtask.

## 5. Operator verification batches
See `20-external-verification-queue.md`: V-BATCH-01 (core suites), 02 (web/Node), 03 (crash/race), 04 (opt-out), 05 (native mutation/parity), 06 (upgrade/rollback), 07 (delivery). Exact commands come from `.github/workflows/taskdeck_agentchat_core.yml`, `taskdeck_remote_agent.yml`, `taskdeck_selfupdate_multi_instance.yml`.

See also docs/taskdeck_ai_agent/20-source-completeness-audit.md (per-subtask and per-gate audit).

## 6. Honest status
No gate is promoted by this document, by a PATCH receipt, or by the operator-supplied CI text. A gate moves to PASS only when the operator returns a real per-test receipt matching the exact missing evidence in §2. The 20/28 count is unchanged.


## 7. Remote review supersedes local-only completeness claim

2026-11-27 — Remote review of user-pushed HEAD `9e4ca30aa299d8538006e9a763fac8d0d3409e01` (17 commits ahead of `f3756029`, 36 changed files). Core 45961114805 FAIL: job 140378893842, `internal/agentchat/collectionprogress/recovery_boundary_test.go:48:28` assignment mismatch (ApplyCounterEvent returns FOUR values; use `replayed, _, applied, err := ...`); job 140378894346, `internal/server/agent_chat_clp20_guard_boundary_test.go:20:10` invalid indirect (NewSession returns Session value; use `return x`). Same failures in core 45953634340/45953522819 and server failure in Remote Go 45953522802. PHP 45953522701 PASS on 190d7d89 only. Fix test source first, no weakened assertions. No full current-source PASS, no gate promoted. Prior CI at 88a909b4 (core 45937055745/Go 45937055780/PHP 45937055761) remains historical PASS. Counted 106/144 checked leaves, 38 unchecked; 1/10 phase, 18/36 tasks, 20/28 historical gates. Current combined acceptance remains unverified.

Actionable source/test gaps (not external-execution-only): (1) CLP-03.01.04/03.02.03/.04/09.01.01 and R10/R27/R28: `20-crash-recovery-matrix.md` explicitly says W1–W7 integrated tests and required narrow injection seams not authored. Implement nil-in-production private seams plus actual native receipt/crash/reconcile tests using existing child fixtures, no shell execution through PATCH. Do not rerun ambiguous native actions or ACK. (2) CLP-01.04.02/04.03.03/R25: `runCollectionProgressAdvice` in agent_run_collection_progress.go is advisory-only, no bounded Unified recovery integration; preserve drain-only creation, existing run ownership/consent/stop/UNKNOWN boundaries. (3) CLP-02.04.01/.02: authoritative complete mutation adapter absent per tracker; source task, not proof supplied by executing pure coverage tests; broad invalidation on uncertainty. (4) CLP-04.02.03/.04,05.01.04,05.02.04: audit itself says test coverage partial; complete missing source/fixtures or document explicit supported limitation, not blanket D. (5) R22: newly added `agentPatchFeedbackAutoResumeReady` / `agentPatchFeedbackResumeReceipt` check at message admission (agent_chat_messages.go around 335), then prepare/admit/mark SENDING (around 605); automatic feedback resume does not have the earlier CollectionReplan marker at this message admission. Audit same-revision opt-out/Stop after preparation and before transport/retry; add narrowly scoped durable admission/consent tests if missing. Preserve native receipt, UNKNOWN ACK and no repeat mutation. (6) CLP-05.02.01 catalog adds status/verified/partial summary only, not per-entry authoritative objective/revision/invalidation proof; keep summary advisory, audit required metadata/bounds. R18 parser test mapping is useful but does not alone prove no native dispatch after partial failed stream; inspect production adapter and add end-to-end fake transport denial tests if missing. No automatic enabling of Unified creation or reduction of safety guards.

All eight gates remain VERIFY_PENDING; the historical PASS count is not current-head acceptance. R18 mapping exists but current source is not build-verified.
