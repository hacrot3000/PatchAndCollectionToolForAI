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

## 6. Honest status
No gate is promoted by this document, by a PATCH receipt, or by the operator-supplied CI text. A gate moves to PASS only when the operator returns a real per-test receipt matching the exact missing evidence in §2. The 20/28 count is unchanged.
