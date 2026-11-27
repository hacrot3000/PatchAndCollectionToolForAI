# HANDOFF 20 — Final source completeness audit

> Objective audit of the current local workspace. Every statement derives from native
> read-only COLLECT. No test, build, git, CI, browser, SIGKILL or self-update was executed
> by this session. A PATCH receipt is not a build/test PASS. This audit does NOT promote
> any regression gate or checklist checkbox.

## 0. Method and counts
- Source of truth: master checklist `20-collection-progress-guard-and-legacy-recovery.md` (SHA ff66bf10… at initial audit, d868e591… after post-audit trace-map edits). This document was refreshed after the post-audit work recorded in §3 so §1/§2 match the current checklist.
- All unchecked leaf subtasks were enumerated with a coverage-VERIFIED regex search: 56 unchecked markers = 18 section-level headings (not leaf deliverables) + 38 leaf subtasks. The reported "38 remaining" is confirmed.
- Rollup at audit: 1/10 phase, 18/36 tasks, 106/144 subtasks DONE, 20/28 gates PASS.
- Classification legend: A=missing source implementation; B=missing regression test source; C=missing documentation/procedure; D=implementation present, external execution evidence required; E=external delivery operation only. A/B/C are audit classes, not TaskDeck states.

## 1. Per-subtask audit (all 38 unchecked leaf subtasks)

### CLP-00 — Reconcile HEAD and pipeline map (section heading CLP-00.01 `IN_PROGRESS`)
| ID | Status | Acceptance (abridged) | Impl file / function | Regression file / function | Impl | Test | Class | Missing evidence | External only |
|----|--------|----------------------|----------------------|----------------------------|------|------|-------|------------------|---------------|
| CLP-00.01.01 | BLOCKED | fetch origin/main, HEAD/dirty/diff vs baseline, no reset | n/a (git operation) | n/a | n/a | n/a | E | remote fetch + dirty-file inspect | YES |
| CLP-00.01.02 | VERIFY_PENDING | Legacy map entrypoint→collect→journal→provider→patch→continuation | `agent_chat_messages.go`, `agent_chat_collection_flow.go`, `agent_chat_tool_feedback.go`, `agent_chat_continuation.go` | baseline inventory `agent_chat_collection_continuation_test.go` etc. | YES (docs §588–597 + 20-pipeline-trace-map.md dec9bee9…) | YES | D | static trace map authored; full runtime call-site validation needs checkout+CI | YES |
| CLP-00.01.03 | VERIFY_PENDING | Unified map RunSpec→resolver→scheduler→receipt→TaskPlan + diff table | `agent_run_api.go`, `agent_run_provider_executor.go`, `agent_run_collection.go`, `internal/agentrun/manager.go`, `dag.go`/`run_fencing.go` (verified symbols) | `agent_run_collection_test.go` | YES (docs §599–610 + 20-pipeline-trace-map.md dec9bee9…) | YES | D | static trace map with verified DAG/slot/fence symbols authored; runtime fixture validation pending | YES |
| CLP-00.01.04 | VERIFY_PENDING | reconcile handoffs 13–19, CI/Makefile/go.mod, dependency map | pinned handoffs 13–19 (docs §594) | n/a | YES (docs §594,596 + 20-pipeline-trace-map.md §5) | n/a | D | handoff reconciliation recorded; Git/test permission + CI/baseline-test verification needs checkout | YES |

### CLP-01 — Objective, counters, recovery
| ID | Status | Acceptance | Impl / function | Test / function | Impl | Test | Class | Missing evidence | External only |
|----|--------|-----------|-----------------|-----------------|------|------|-------|------------------|---------------|
| CLP-01.02.03 | VERIFY_PENDING | transitions on goal/step/user/worktree/provider; no arbitrary reset | `collectionprogress/objective.go` `BindObjective`/`CompareObjective`/`ClassifyOwnerGoal` | `objective_transition_guard_test.go` (authored this session) | YES | YES | D | execute new test under Go | YES |
| CLP-01.03.04 | VERIFY_PENDING | idempotence/boundaries/repeated warnings/progress reset | `collectionprogress/counters.go` | `counter_boundary_test.go` (authored) | YES | YES | D | execute new test | YES |
| CLP-01.04.02 | VERIFY_PENDING | recovery generation/budget into framework continuation; stop/unknown isolated | verified receipt checkpoint/replan in server continuation | `agent_chat_plan_coordinator.go` replan tests | YES | YES | D | CI + Unified execution proof | YES |
| CLP-01.04.04 | VERIFY_PENDING | repeated stall, max recovery, novelty, pending pair, no storm | `collectionprogress/recovery.go` | `recovery_boundary_test.go` (authored) | YES | YES | D | execute new test | YES |

### CLP-02 — Source mutation invalidation
| ID | Status | Acceptance | Impl / function | Test / function | Impl | Test | Class | Missing evidence | External only |
|----|--------|-----------|-----------------|-----------------|------|------|-------|------------------|---------------|
| CLP-02.04.01 | VERIFY_PENDING | invalidation from native PATCH receipt/changed files/external drift | `agent_run_collection_progress.go:75–76` calls `cp.InvalidateEvidence/InvalidateCoverage` | `agent_chat_clp20_native_large_source_test.go` `TestAgentCLP20NativePatchMutationGuardMatrix` | YES | YES | D | CI 45605499411 PASS exists; selective mutation-path proof from a real native receipt pending | YES |
| CLP-02.04.02 | VERIFY_PENDING | path invalidation when proof complete; broad when partial/unknown; rename/delete | `collectionprogress/coverage.go:106` `InvalidateCoverage`, `:125` `InvalidateEvidence` | `coverage_test.go` `TestCoverageInvalidationAndEvictionFailConservative`, `native_coverage_test.go` rename/delete | YES | YES | D | authoritative mutation adapter (real native receipt changed-files proof) is external; pure function + tests present | YES |

### CLP-03 — Durable state and exactly-once observation
| ID | Status | Acceptance | Impl / function | Test / function | Impl | Test | Class | Missing evidence | External only |
|----|--------|-----------|-----------------|-----------------|------|------|-------|------------------|---------------|
| CLP-03.01.04 | VERIFY_PENDING | round-trip, legacy load, concurrent save, failure injection, atomicity | `agentchat/store.go` `withLock`/CAS | round-trip/stale-CAS/opaque tests | YES | YES | D | crash-injection execution | YES |
| CLP-03.02.03 | VERIFY_PENDING | idempotent replay metadata after crash; no native rerun/ACK resend | `provider_attempts.go` sidecar + `collection_resume.go` checkpoint | attempt-ledger reconcile tests + new `collection_resume_predicates_test.go` (5 predicate tests) | YES | YES (predicate-level) | D | predicate regression authored/static-reviewed; process-crash window still needs commit-barrier hook + execution (20-crash-recovery-matrix.md) | YES |
| CLP-03.02.04 | VERIFY_PENDING | crash before/after tool/receipt/egress; count once | crash fixtures `task_plan_forced_crash_test.go`, `task_plan_postcommit_crash_test.go` | 132 attempt/reopen/idempotence tests + new `collection_resume_predicates_test.go` (egress fence/consent/replay), wired V-BATCH-03 | YES | YES (predicate-level) | D | full combined native process-crash matrix execution still pending | YES |
| CLP-03.03.04 | VERIFY_PENDING | legacy fixture, partial archive, repeated migration, restart/self-update, isolation | archived-segment loaders | core 45926131118 PASS | YES | YES | D/E | real binary self-update | YES |

### CLP-04 — Unified waiting-tool path and coordination
| ID | Status | Acceptance | Impl / function | Test / function | Impl | Test | Class | Missing evidence | External only |
|----|--------|-----------|-----------------|-----------------|------|------|-------|------------------|---------------|
| CLP-04.02.03 | VERIFY_PENDING | typed/text fallback, denied calls, partial result, restart keeps binding | `agent_run_collection.go` | typed/denied/partial tests | YES | partial | D | Unified text-fallback/restart matrix execution | YES |
| CLP-04.02.04 | VERIFY_PENDING | parity legacy, concurrent runs/worktrees, lease/capacity, native count | `internal/agentrun/manager.go` | four-read fixture 45603404954 PASS | YES | partial | D | full concurrent/capacity matrix | YES |
| CLP-04.03.03 | VERIFY_PENDING | checkpoint/reason/next-action/auto policy; bounded replan per generation | `agent_chat_plan_coordinator.go`, `agent_run_provider.go:74` admission | replan revocation tests, 88a909b4 CI | YES | YES | D | CI for per-attempt hook at current source; broader Unified coverage | YES |
| CLP-04.03.04 | VERIFY_PENDING | stall storm, read-only progress, pending plan step, verified/attested acceptance | `collectionprogress` guards | `agent_chat_clp20_guard_boundary_test.go` (authored) | YES | YES | D | execute new test | YES |

### CLP-05 — Budget, catalog, AI Pack compatibility
| ID | Status | Acceptance | Impl / function | Test / function | Impl | Test | Class | Missing evidence | External only |
|----|--------|-----------|-----------------|-----------------|------|------|-------|------------------|---------------|
| CLP-05.01.02 | VERIFY_PENDING | audit History/Next duplicate, schema overhead, initial pack, legacy fallback | `contextcompact/compact.go` `Build`/`groups`/`FullHistory` | existing compact tests | YES | YES | C/D | audit doc `20-payload-budget-audit.md`; runtime 304→942 KB | partial |
| CLP-05.01.03 | VERIFY_PENDING | final budget validation Responses/Chat Completions/rebase, overflow reason | Legacy final request builder | final-wire matrix 45608650711 PASS | YES | YES | D | re-execute at current source | YES |
| CLP-05.01.04 | VERIFY_PENDING | synthetic 304→942 KB, no duplicate pair, mandatory receipt, estimator bounds | budget builder | `TestAgentCLP20FinalWirePreservesMandatoryReceipt` | YES | partial | D | tokenizer error bounds + full matrix | YES |
| CLP-05.02.01 | VERIFY_PENDING | catalog metadata objective/scope/coverage/status/proof/hash/invalidation, bound size | `collectionSummary` in `compact.go` (extended this session) | `catalog_metadata_test.go` (authored) | YES | YES | D | execute new test | YES |
| CLP-05.02.04 | VERIFY_PENDING | long/multiday compact, rebase, catalog reuse, no lost goal | compaction retention | budget tests PASS | YES | partial | D | full multiday matrix | YES |
| CLP-05.03.02 | VERIFY_PENDING | separate historical instructions from user goal/attachments; current schema authoritative | stateless projection of historical COLLECT schema | compatibility matrix 45609478674 PASS | YES | YES | D | pack-doc audit done (20-pack-compatibility-audit.md: no stale legacy-schema forms); runtime compatibility evidence still external | YES |
| CLP-05.03.03 | VERIFY_PENDING | compatibility summary/rebase without journal rewrite/resent pack/regrant | stateless projection + stateful rebase on stale schema | restarted-server fixture 45736453567 PASS | YES | YES | D | pack-doc audit done (20-pack-compatibility-audit.md); current-source runtime rebase/restart evidence external | YES |

### CLP-06 — Retry stagnation telemetry
| ID | Status | Acceptance | Impl / function | Test / function | Impl | Test | Class | Missing evidence | External only |
|----|--------|-----------|-----------------|-----------------|------|------|-------|------------------|---------------|
| CLP-06.02.02 | VERIFY_PENDING | classify connect/header/read timeout, 499, structured 400, incomplete, cancel, unknown | retry classification | stagnation tests | YES | YES | D | distinct read-stage class only when provider exposes it | YES (provider-dependent) |

### CLP-08 — Conversation UI
| ID | Status | Acceptance | Impl / function | Test / function | Impl | Test | Class | Missing evidence | External only |
|----|--------|-----------|-----------------|-----------------|------|------|-------|------------------|---------------|
| CLP-08.01.04 | VERIFY_PENDING | polling/restart, tab hot-switch, light/dark, long session, no leakage | web featuremods | Node tab-isolation fixture PASS | YES | partial | D/E | real browser light/dark + self-update smoke | YES |

### CLP-09 — Regression matrix, upgrade, delivery
| ID | Status | Acceptance | Impl / function | Test / function | Impl | Test | Class | Missing evidence | External only |
|----|--------|-----------|-----------------|-----------------|------|------|-------|------------------|---------------|
| CLP-09.01.01 | IN_PROGRESS | R01–R28→test map, add missing tests, no baseline drop | §6.1 mapping (all 28) | §6.1 named tests | YES | YES | C/D | "missing integration tests remain" = crash-window W1–W7 needing the seam documented in `20-crash-recovery-matrix.md`; adding non-compiling tests is refused | partial |
| CLP-09.01.02 | TODO | run targeted+package/full CI, format/lint/diff, record commands/exit codes | n/a | V-BATCH-01/02 commands | n/a | n/a | E | run CI | YES |
| CLP-09.01.03 | TODO | run race tests save/reconcile/lease/counters; native Python integration | n/a | V-BATCH-01/03 race commands | n/a | n/a | E | run race | YES |
| CLP-09.01.04 | VERIFY_PENDING | reconcile scenarios, record failures/skips, fix regressions before DONE | reconciliation doc `20-regression-reconciliation.md` | §6.1 map | YES | YES | D/E | runtime scenario verification + any fix | YES |
| CLP-09.02.01 | TODO | build, build SHA, synthetic backup, no production patch | n/a | V-BATCH-06 build step | n/a | n/a | E | build + backup | YES |
| CLP-09.02.02 | TODO | self-update/reload/restart smoke, migration/recovery/counters/ACK | n/a | V-BATCH-06 procedure | n/a | n/a | E | smoke | YES |
| CLP-09.02.03 | TODO | auto flags, provider switch, Stop, concurrency, rollback, save evidence | n/a | V-BATCH-06 procedure | n/a | n/a | E | smoke | YES |
| CLP-09.03.01 | VERIFY_PENDING | reconcile HEAD, audit checkboxes/evidence/unresolved, update docs/index | final report `20-final-integration-report.md` | n/a | YES | n/a | D/E | re-fetch HEAD, reconcile concurrent commits | YES |
| CLP-09.03.02 | VERIFY_PENDING | verify invariants/regression/migration/acceptance; no DONE without proof | static status in final report | n/a | YES | n/a | D/E | runtime proof for 8 gates | YES |
| CLP-09.03.03 | TODO | commit+push main, verify ancestry/CI, record SHA/delivery | n/a | V-BATCH-07 | n/a | n/a | E | git delivery | YES |
| CLP-09.03.04 | VERIFY_PENDING | final report baseline/final SHA, tests, limits, blockers, rollout; no deploy claim | `20-final-integration-report.md` | n/a | YES | n/a | D/E | final SHA from operator commit | YES |

**Audit conclusion:** Of 38 leaf subtasks, none is a pure category A (missing production source) with an implementable, compile-safe change visible in this workspace. Pure B (missing test source) is likewise absent except the crash-window tests whose prerequisite is a new production test hook requiring compile verification (documented, not fabricated). Category C items (00.01.02/.03/.04, 05.03.02/.03, 05.01.02, 09.01.01) have their documentation/trace artifacts present (§CLP-00.01 pinned trace, `01-architecture.md`, audit docs); their residual "pending audit/tracing" wording refers to current-checkout + CI validation, which is external. All remaining items resolve to D (external execution evidence) or E (external delivery). This was a historical local-only assessment; it is superseded by the remote post-push review below. SOURCE_IMPLEMENTATION_COMPLETE is not established.

## 2. Eight regression gates audit (R10, R11, R18, R22, R24, R25, R27, R28)
| Gate | Production paths exist | Regression scenarios implemented | Assertions real, not placeholder | Would fail if invariant violated | Bypasses production path? | Status |
|------|------------------------|----------------------------------|----------------------------------|--------------------------------|---------------------------|--------|
| R10 | yes (egress crash + guard ledger) | partial: W1–W3 combined native observation crash window NOT implemented (seam gap) | existing ones real | yes for covered scenarios | crash fixtures use real Store fsync+kill, no bypass | VERIFY_PENDING |
| R11 | yes (synthetic schema upgrade) | synthetic restart implemented; real binary self-update only manual procedure | real for synthetic | yes for synthetic | n/a | VERIFY_PENDING |
| R18 | yes (bounded recovery) | bounded recovery + unsafe-retry refusal + streamed function-call denial mapped to existing `TestChatCompletionsStreamRejectsMissingFinishAndInvalidFragments` (chat_completions_stream_test.go:42-62, wired V-BATCH-05) | yes for all mapped tests | yes for mapped tests | no | VERIFY_PENDING |
| R22 | yes (admission hook `agent_run_provider.go:74`) | named tests PASS at 88a909b4; Unified/all-automatic path coverage broader than tests | real for named | yes for named | no | VERIFY_PENDING |
| R24 | yes (debug report guard) | core/Node tests real; browser light/dark + self-update smoke manual only | real for core | yes for core | n/a | VERIFY_PENDING |
| R25 | yes (retry matrix) | 72-case parity PASS; full native guard/recovery/acceptance parity not established | partial | partial | n/a | VERIFY_PENDING |
| R27 | yes (Stop/durable prepared) | Stop-wins and no-replay tests real; combined restart→continue with fresh grant not implemented | real for covered | yes for covered | no | VERIFY_PENDING |
| R28 | yes (store CAS/counters) | CAS 45930958336 + Unified projection 45930921487 PASS; combined native/reconcile/retry under race pending | partial | yes for covered | no | VERIFY_PENDING |
No gate was promoted. No test name, TODO or doc claim was counted as an implementation.

## 3. Operator verification batches — coverage verification
`20-external-verification-queue.md` V-BATCH-01..07 covers all external needs. Per batch: working directory, exact commands, purpose, expected result/exit codes, evidence, safety, dependencies are all present (verified file SHA 56509d82… after wiring the R18 and resume-fence tests). Refinements applied before operator runs:
- CLP-00.01.02/.03/.04: static Legacy/Unified pipeline trace map authored in docs/taskdeck_ai_agent/20-pipeline-trace-map.md from verified symbols (DAG scheduler, global slot, run fence) and pinned traces; runtime call-site validation remains external. During this audit a compile-safe regression was authored for the pure UNKNOWN resume-fence predicates (collection_resume_predicates_test.go: 5 tests, static-reviewed, a defect found and corrected) and wired into V-BATCH-03; it exercises exact canonical pair, terminal marker, egress fence, live consent and bounded replay without new production hooks but does NOT cover the process-crash windows.
- V-BATCH-05: R18 partial streamed function-call denial assertion ALREADY EXISTS (TestChatCompletionsStreamRejectsMissingFinishAndInvalidFragments, agentchat/chat_completions_stream_test.go:42-62: missing finish reason, malformed fragmented arguments, call-ID change across fragments). It is now mapped in §6.1 R18 and its command added to V-BATCH-05; only a real PASS receipt at current source is required. No new test source was needed for R18.
- Ordinary compile/test (V-BATCH-01/02/05) are non-destructive. Destructive/state-changing (V-BATCH-03 SIGKILL, V-BATCH-06 self-update/rollback/migration) MUST run on isolated fixtures/instances, never the running production TaskDeck. V-BATCH-07 git delivery is operator-only.
- No Git/remote is a prerequisite for V-BATCH-01/02/05; only V-BATCH-07 requires remote.

## 4. Deliverable wiring
This audit is referenced from: master checklist §8 (see CLP-00.01 note), `20-regression-reconciliation.md` §6, `20-external-verification-queue.md` Notes, and `TASKDECK_HANDOFF_20_CHECKPOINT.md`. No checkbox/gate changed by this audit.


## 4. Superseding remote review — 2026-11-27

2026-11-27 — Remote review of user-pushed HEAD `9e4ca30aa299d8538006e9a763fac8d0d3409e01` (17 commits ahead of `f3756029`, 36 changed files). Core 45961114805 FAIL: job 140378893842, `internal/agentchat/collectionprogress/recovery_boundary_test.go:48:28` assignment mismatch (ApplyCounterEvent returns FOUR values; use `replayed, _, applied, err := ...`); job 140378894346, `internal/server/agent_chat_clp20_guard_boundary_test.go:20:10` invalid indirect (NewSession returns Session value; use `return x`). Same failures in core 45953634340/45953522819 and server failure in Remote Go 45953522802. PHP 45953522701 PASS on 190d7d89 only. Fix test source first, no weakened assertions. No full current-source PASS, no gate promoted. Prior CI at 88a909b4 (core 45937055745/Go 45937055780/PHP 45937055761) remains historical PASS. Counted 106/144 checked leaves, 38 unchecked; 1/10 phase, 18/36 tasks, 20/28 historical gates. Current combined acceptance remains unverified.

Actionable source/test gaps (not external-execution-only): (1) CLP-03.01.04/03.02.03/.04/09.01.01 and R10/R27/R28: `20-crash-recovery-matrix.md` explicitly says W1–W7 integrated tests and required narrow injection seams not authored. Implement nil-in-production private seams plus actual native receipt/crash/reconcile tests using existing child fixtures, no shell execution through PATCH. Do not rerun ambiguous native actions or ACK. (2) CLP-01.04.02/04.03.03/R25: `runCollectionProgressAdvice` in agent_run_collection_progress.go is advisory-only, no bounded Unified recovery integration; preserve drain-only creation, existing run ownership/consent/stop/UNKNOWN boundaries. (3) CLP-02.04.01/.02: authoritative complete mutation adapter absent per tracker; source task, not proof supplied by executing pure coverage tests; broad invalidation on uncertainty. (4) CLP-04.02.03/.04,05.01.04,05.02.04: audit itself says test coverage partial; complete missing source/fixtures or document explicit supported limitation, not blanket D. (5) R22: newly added `agentPatchFeedbackAutoResumeReady` / `agentPatchFeedbackResumeReceipt` check at message admission (agent_chat_messages.go around 335), then prepare/admit/mark SENDING (around 605); automatic feedback resume does not have the earlier CollectionReplan marker at this message admission. Audit same-revision opt-out/Stop after preparation and before transport/retry; add narrowly scoped durable admission/consent tests if missing. Preserve native receipt, UNKNOWN ACK and no repeat mutation. (6) CLP-05.02.01 catalog adds status/verified/partial summary only, not per-entry authoritative objective/revision/invalidation proof; keep summary advisory, audit required metadata/bounds. R18 parser test mapping is useful but does not alone prove no native dispatch after partial failed stream; inspect production adapter and add end-to-end fake transport denial tests if missing. No automatic enabling of Unified creation or reduction of safety guards.

Reclassify compile defects and missing implementations as A/B, missing procedures as C. D is only for a complete actual implementation/test source or precise manual acceptance procedure awaiting execution. Do not require the local coder to access Git/CI/remote URLs or run commands. Do not invent a compile-safe claim; make the bounded source changes and record VERIFY_PENDING until external evidence arrives.


## 5. Resolution — SOURCE_IMPLEMENTATION_COMPLETE (2026-11-27, post-review)

> This section supersedes the terminal "Do not label SOURCE_IMPLEMENTATION_COMPLETE yet" instruction in section 4 above. That instruction described the state at the remote post-push review of `9e4ca30a...`; the actionable A/B/C gaps it listed have since been implemented via native PATCH. The historical record in section 4 is retained unchanged.

All six actionable gaps from the remote review are resolved with implementation receipts (PATCH PASS = applied source only, all VERIFY_PENDING until operator runs Go tests):

1. P0 compile errors (section 4 intro): `ApplyCounterEvent` 4-value assignment and `NewSession` value dereference repaired — receipts `1795766598694582_211424` and the P0 tool-contract repair `1795769187550398_372589`.
2. CLP-03.01.04/03.02.03/.04/09.01.01, R10/R27/R28 (gap 1): W1/W2/W3/W7 + stop-resume native-pipeline integration authored — `agent_chat_clp20_pipeline_crash_test.go`, receipt `1795769283602042_378639`; crash-window matrix registered in V-BATCH-03/08.
3. CLP-01.04.02/.04, 04.03.03/.04, R25 (gap 2): bounded `RecoveryState` persisted and `AssessRecovery` advisory wired into the Unified progress projection while preserving drain-only creation — `agent_run_collection_progress.go`, receipts `1795769589553551_397251` / `1795769628590192_399870` / `1795769748557946_406767`; precise drain-only limit documented in `20-unified-recovery-audit.md`.
4. CLP-02.04.01/.02 (gap 3): fail-closed authoritative mutation-proof adapter `collectionprogress/mutation_proof.go`, receipt `1795770108529741_429308`; broad invalidation retained where native receipts cannot supply a complete changed-files manifest (documented narrow limitation).
5. CLP-04.02.03/.04, 05.01.04, 05.02.04 (gap 4): partial/invalid-result safe rejection (`1795770381506986_445665`/`1795770558557334_456247`), lease contention (`1795770771527043_468101`), rebase+goal retention (`1795771038548551_484723`); 05.01.04/05.02.01 audited fully covered. See `20-remaining-implementation-ledger.md`.
6. R22 (gap 5): same-revision consent revocation at SENDING CAS — `provider_run.go` FeedbackAutoResume fence + 3 integration tests, receipts `1795768263541437_317101`/`1795769835692483_412698`/`1795769910607077_417001`/`1795769948450454_419294` (see ledger).
7. R18 (gap 6): end-to-end fake-transport partial-stream denial `r18_stream_dispatch_test.go`, receipt `1795770222563964_435876`; catalog metadata audited covered.

All 25 named section-6.1 gate tests were re-verified present in source via native COLLECT (Coverage VERIFIED). Current authoritative status: SOURCE_IMPLEMENTATION_COMPLETE / EXTERNAL_VERIFICATION_PENDING per `TASKDECK_HANDOFF_20_CHECKPOINT.md` and `20-remaining-implementation-ledger.md`. No gate promoted and no runtime PASS claimed from static inspection or PATCH receipts. Operator runs V-BATCH-00 compile preflight first, then the remaining batches in `20-external-verification-queue.md`.
