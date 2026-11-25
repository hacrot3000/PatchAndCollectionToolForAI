# POST_MERGE_MASTER_PLAN_PROGRESS.md

Durable progress tracker for the post-merge Master Plan (M17-01 ... M17-19).

- Source handoff: `TASKDECK_HANDOFF_17_19_POST_MERGE_MASTER_PLAN_20261125.md`
- Verified baseline: `df7e2c9da576eabf0e06c446c7c1df98ef99eb75`
- Project identity: key=`PatchAndCollectionToolForAI`, id=`5dffe79a-f30c-4a73-88c3-834d8a90e976`
- Execution model: local TaskDeck. Native COLLECT for evidence, Native PATCH for source changes.
- Rule: a Native Patch PASS proves only that the patch operation succeeded, not application correctness.
  Never mark VERIFIED without an actually executed, independently observed test.

## Status values

NOT_STARTED | INVESTIGATING | IMPLEMENTING | IMPLEMENTED_UNVERIFIED | VERIFIED | BLOCKED | NOT_APPLICABLE

## ACTIVE EXECUTION GATE — P0 Unified Agent transition (2026-11-25)

Owner-directed priority supersession: [Handoff 17 — active single-engine directive](17-objective-task-evidence-autocheck.md#active-owner-directive--p0-mandatory-single-engine-transition-2026-11-25). All outstanding M17-01…M17-19 work below is **DEFERRED_BY_UAG** until UAG-08 is accepted. This is a scheduling overlay, not a replacement for the recorded implementation/verification status values. Preserve the old rows, source evidence, and pending tests. Do not extend Agent Run Manager as an independent engine. Required migration and parity work takes precedence over the old next-action instructions anywhere below.

Planning baseline: `33e845d91df0bac09bb7d74c47e34e29f926e8d4`; inspected HEAD `52488ea1e03a4921fea1bfaed38cda6da0485c48` (post UAG-02 slice). **Current next action: UAG-03** — unify native tool gateway on Legacy path after UAG-02 verification.

| ID | Status | Next action / gate |
|----|--------|--------------------|
| UAG-01 | IMPLEMENTED_UNVERIFIED | Inventory complete; baseline `go test -race -count=1 ./internal/agentchat/...` PASS on `52488ea1e`; full M17-01 Node/CI matrix still owner-pending. |
| UAG-02 | IMPLEMENTED_UNVERIFIED | Turn binding + Legacy queue path landed; verify browser hot-switch and queued send; full server race suite still owner-pending. |
| UAG-03 | NOT_STARTED | After UAG-02: unify native tool gateway, probe/fallback, approvals, feedback, retry, continuation and exact evidence. |
| UAG-04 | NOT_STARTED | After UAG-03: unified transcript and composer, fence new Run Manager admissions at UI and API, drain only. |
| UAG-05 | NOT_STARTED | After UAG-04: migrate old run records safely, exact receipts/archives/plan references and same-ID resume; verify restart/rollback. |
| UAG-06 | NOT_STARTED | After UAG-05: port required coordination, workspace and other consumers to the unified engine or neutral components. |
| UAG-07 | NOT_STARTED | After UAG-06: remove all independent Run Manager execution paths; retain only necessary historical reader/migration compatibility. |
| UAG-08 | NOT_STARTED | After UAG-07: full exact-head build/test/CI, native/browser/Self Update regression, migration/evidence audit, owner acceptance and backlog reconciliation. |

For each UAG package append a durable checkpoint: exact inspected HEAD; files/symbols and capability mapping; changed files; native Patch/Collection receipt IDs; commands actually executed and exit status; CI URLs and SHA; migration/recovery evidence; blockers; next actionable step. A source-only change is IMPLEMENTED_UNVERIFIED, not VERIFIED. Record explicit owner acceptance and exact commit for UAG-08; until then no unrelated M17/Handoff 18/19 package is eligible. If a legacy backlog item is replaced by UAG, preserve the original record and document the replacement/test and final disposition rather than erasing it or falsely marking it complete.

## Work package summary

| ID | Title | Status | Next action |
|----|-------|--------|-------------|
| M17-01 | exact HEAD regression inventory (P0) | IMPLEMENTED_UNVERIFIED | Hand expanded workflow commands to CI; confirm a run-only change triggers the workflow |
| M17-02 | one canonical native receipt identity | IMPLEMENTED_UNVERIFIED | F03 stale-run guard, namespaced call-identity resolver and versioned receipt envelope implemented; all unverified pending CI |
| M17-03 | native COLLECT to exact full_file_read proof | IMPLEMENTED_UNVERIFIED | bounded pure adapter added; adoption wiring and CI verification pending |
| M17-04 | run-bound automatic Patch/reconciliation/feedback | IMPLEMENTED_UNVERIFIED | fail-closed dispatch precondition gate added; scheduler wiring and CI verification pending |
| M17-05 | ordered proof adoption before next prompt (bounded slice) | IMPLEMENTED_UNVERIFIED | pure ordered next-prompt gate added; wiring + CI verification pending |
| M17-06 | unified run egress state machine and ACK recovery (bounded slice) | IMPLEMENTED_UNVERIFIED | pure fail-closed egress-state classifier added; wiring + CI verification pending |
| M17-07 | authoritative provider reconciliation | IMPLEMENTED_UNVERIFIED | capability inventory + fail-closed binding classifier added; wiring + CI verification pending |
| M17-08 | same-host Run Manager fencing | IMPLEMENTED_UNVERIFIED | cross-process run fence + admission classifier added; wiring + CI verification pending |
| M17-09 | bounded streaming/event history and compaction | IMPLEMENTED_UNVERIFIED | pure bounded event-compaction planner added; archive wiring + CI verification pending |
| M17-10 | progress/no-work guard and scheduling fairness | NOT_STARTED | after 02/05/09 |
| M17-11 | controlled workspace drift and snapshot chain | IMPLEMENTED_UNVERIFIED | pure drift classifier + checkpoint lineage primitive added; wiring + CI verification pending |
| M17-12 | conversation/shared context/provider switching | NOT_STARTED | after 02/08/09 |
| M17-13 | actual crash/fault matrix | NOT_STARTED | after 04/06/08/09 |
| M17-14 | full regression and browser end-to-end | NOT_STARTED | after 01-13 |
| M17-15 | objective vs semantic acceptance audit | NOT_STARTED | after 03/05/12 |
| M17-16 | reconcile docs and handoff, release candidate | NOT_STARTED | after 01-15 |
| M17-17 | distributed multi-host scheduler design | NOT_STARTED | DEFERRED_SCOPE: optional, owner approval required |
| M17-18 | physical storage durability and soak | NOT_STARTED | DEFERRED_SCOPE: optional, owner approval required |
| M17-19 | coordination UX and further capacity | NOT_STARTED | DEFERRED_SCOPE: optional, owner approval required |

## M17-01 - exact HEAD regression inventory

Status: IMPLEMENTED_UNVERIFIED

### Confirmed findings
- F09 GAP confirmed: `.github/workflows/taskdeck_agentchat_core.yml` path filters and test steps omitted run-only code.
  - push/pull_request paths lacked `internal/agentrun/**`, `internal/server/agent_run_*`,
    `agentchat_runs_ui.js`, `agentchat_coordination.js`, and the run/coordination Node tests.
  - Test step did not run agentrun race, focused server run/coordination race, or the new Node tests.
- All referenced paths were verified to exist through native COLLECT (Coverage status: VERIFIED).
- This progress file was confirmed absent before creation (VERIFIED absence).

### Source files inspected
- `.github/workflows/taskdeck_agentchat_core.yml` (82 lines)
- `.python_patch_tool.json` (project identity matches target)
- `vscode_tasks_menu_go/internal/agentrun/**` (26 .go files via directory collection)
- `vscode_tasks_menu_go/internal/server/agent_run_*.go` (find, VERIFIED)
- `vscode_tasks_menu_go/web/featuremods/agentchat_runs_ui.js`
- `vscode_tasks_menu_go/web/featuremods/agentchat_coordination.js`
- `vscode_tasks_menu_go/web/agentchat_runs_isolation_test.cjs`
- `vscode_tasks_menu_go/web/agentchat_coordination_test.cjs`

### Source files modified
- `.github/workflows/taskdeck_agentchat_core.yml` (expanded path filters, test steps, timeout 10 to 20)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (new)

### Native Patch execution results
- Native Patch "M17-01 workflow regression gate and durable progress tracker": PASS.
  Verified native result: call_id=chatcmpl-tool-a42c37b4bc10dd2a, run=1795579186801167_4057342,
  receipt_hash=74624e72bb913c4e (from TaskDeck native Patch history; not re-executed here).
- Post-patch native COLLECT (m17-01-verify) confirmed the expanded workflow (99 lines) and this
  progress file on disk. A Native Patch PASS proves the operation succeeded, not test correctness.

### Tests added
- Workflow regression coverage: a run-only change under `internal/agentrun/**` or
  `internal/server/agent_run_*` now triggers the Agent Chat core workflow on push and pull_request.
- Explicit commands added for agentrun race, focused server run/coordination race, new `node --check`
  targets, and `node --test` for run isolation and coordination tests. `-v` makes Go SKIP lines visible.

### Tests actually executed and their results
- Not executed in this environment. Native COLLECT/PATCH do not run `go test` or `node`.
  No test result is claimed as passed here.

### Pending verification
- `go test -race -count=1 ./internal/agentchat/... ./internal/agentrun/... ./internal/taskmanifest`
- `go test -v -race -count=1 ./internal/server -run '^(TestAgentRun|TestAgentConversation|TestAgentCoordination|TestAgentContext|TestAgentTask|TestAgentProvider)'`
- `go vet ./internal/server ./internal/agentrun/... ./internal/agentchat/... ./internal/taskmanifest`
- `node --check web/featuremods/agentchat_runs_ui.js` and `agentchat_coordination.js`
- `node --test web/agentchat_runs_isolation_test.cjs web/agentchat_coordination_test.cjs`
- Confirm an intentional run-only change triggers the workflow and an injected failing test fails CI.
- Reports tied to exact SHA; zero untriaged new failures (compare to parent via clean worktree).

### Dependencies and blockers
- Verification-only blocker: no direct shell/CI execution is exposed through the Agent's native tools here;
  verification is delegated to CI/owner and stays pending.
- No source dependency for M17-01 (P0, no dependency).

### Next actionable step
- Verification only: run the expanded workflow commands in CI/owner environment; confirm a
  run-only change triggers the workflow and an injected failing test fails CI.

## M17-02 (partial) - F03 stale-run history overwrite guard

Status: IMPLEMENTED_UNVERIFIED

Scope note: this is the F03 slice of M17-02 - a late sync from a stale run must not
hide newer conversation history or mirror an older status over a superseding run.
The wider M17-02 canonical receipt envelope (owner/project/conversation/run identity,
request/result digests, idempotent Session projection) remains outstanding.

### Confirmed findings
- F03 RISK confirmed in source: `syncAgentRunConversation` replaced `session.History`
  with `run.History` and mirrored `run.Status`, guarded only by a `Store.Save` revision
  CAS. A late sync from a stale run could overwrite history/final answers recorded by
  a newer sibling run. `completedAgentRunHistory` merges records from any origin.
- Symbols verified by native COLLECT: `HistoryItem` (responses.go:26), `RunSpec`
  (runs.go:11), `Record` (manager.go:95), `Manager.List` (manager.go:265),
  `Store.Get`/`Save` (store.go:555/576). Coverage status: VERIFIED.

### Source files inspected
- `vscode_tasks_menu_go/internal/server/agent_run_patch_sync.go` (88 lines before change)
- `vscode_tasks_menu_go/internal/server/agent_run_patch.go` (258 lines)
- `vscode_tasks_menu_go/internal/server/agent_run_history.go` (165 lines)
- `vscode_tasks_menu_go/internal/server/agent_run_history_test.go` (164 lines)
- `vscode_tasks_menu_go/internal/server/agent_run_patch_test.go` (244 lines)
- `vscode_tasks_menu_go/internal/agentchat/responses.go`, `runs.go`, `store.go`
- `vscode_tasks_menu_go/internal/agentrun/manager.go`

### Source files modified
- `vscode_tasks_menu_go/internal/server/agent_run_patch_sync.go`: added the
  `agentRunHistorySnapshotIsStale` fail-closed guard before any session write
  (applied by the predecessor patch; not re-applied here).
- `vscode_tasks_menu_go/internal/server/agent_run_patch_sync_test.go`: new table test
  (applied by the predecessor patch; not re-applied here).
- docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md: this section and the M17-02 summary row (this successor patch).

### Native Patch execution results
- Predecessor Native Patch "M17-02 F03 stale-run sync guard and progress update":
  FAIL; run=1795579498807752_4077772; receipt_hash=9fafbfd356b0bd74.
  Native COLLECT (m17-02-after-fail) confirmed both Go edits were written to disk before
  the failure; the failing edit was the documentation anchor. This successor records
  the lineage and completes only the documentation edit; it does not touch the Go files.
- Successor Native Patch "M17-02 F03 documentation completion after partial patch failure": PASS; call_id=chatcmpl-tool-853a97b2eabae827, run=1795579561838876_4081767, receipt_hash=eab009ab51d15d3e. Native COLLECT (m17-02-doc-verify) confirmed this section on disk. A Native Patch PASS proves the operation succeeded, not test correctness.

### Tests added
- `TestAgentRunHistorySnapshotIsStale` covers: empty session; own final answer;
  superseding run final answer; superseding tool result; prior run already present in
  snapshot; foreign user prompt alone.

### Tests actually executed and their results
- Not executed in this environment. Native COLLECT/PATCH do not run `go test`.
  No test result is claimed as passed here.

### Pending verification
- `go test -race -count=1 ./internal/server -run '^(TestAgentRun|TestAgentPatch)'`
- Full focused run/coordination race suite from the M17-01 pending list.
- Integration case: stale run sync while session already shows a newer sibling answer.

### Dependencies and blockers
- Depends on M17-01 (workflow gate recorded; verification pending CI).
- Verification blocker: no shell/CI execution is exposed through the native Agent tools.
- Failure lineage: predecessor FAIL partially applied; resolved by completing the
  remaining documentation edit without re-applying the Go changes.

### Next actionable step
- Verify the F03 guard compiles and its test passes in CI. Continue the remaining
  M17-02 canonical-receipt scope only after this slice is VERIFIED.



## M17-02 (core) - canonical run-namespaced call identity primitive

Status: IMPLEMENTED_UNVERIFIED

Scope: bounded foundation slice of the wider M17-02 canonical receipt envelope.
This adds a pure, namespaced tool-call identity resolver that fails closed on
ambiguous or missing CallID, plus table-driven tests. It does NOT wire into the
cross-store Session/Task-Plan projection, does NOT mint receipts, and does NOT
change any existing Go call site. The full versioned receipt envelope and
idempotent two-store projection remain outstanding.

### Confirmed findings
- F03/F10 source evidence: several lookup sites resolve tool calls by bare
  CallID (`agent_chat_sync.go:188`, `agent_chat_tools.go:192`,
  `agent_chat_tool_feedback.go:86`, `agent_chat_patch_recovery.go:93`).
  `agent_run_routing.go:34` already binds OriginRunID to the record and fails
  closed on ambiguous records. A shared, run-namespaced, ambiguity-fail-closed
  resolver is the prerequisite for one canonical receipt identity.
- `ToolCallRecord` fields verified by native COLLECT (`store.go:86`):
  AgentRunID, CallID, Name, ArgumentsSHA256, RunID, PackageSHA256,
  PatchSessionID, Status, Result, FeedbackSent, FeedbackStatus.

### Source files inspected
- `vscode_tasks_menu_go/internal/server/agent_run_feedback_ack.go` (50 lines)
- `vscode_tasks_menu_go/internal/server/agent_run_routing.go` (80 lines)
- `vscode_tasks_menu_go/internal/server/agent_chat_sync.go` (150-230)
- `vscode_tasks_menu_go/internal/server/agent_chat_tools.go` (180-215)
- `vscode_tasks_menu_go/internal/agentchat/store.go` (ToolCallRecord, Save/Get)
- `vscode_tasks_menu_go/internal/agentchat/task_evidence_validate.go` (119 lines)

### Source files modified (this patch, additive only)
- `vscode_tasks_menu_go/internal/server/agent_run_call_identity.go` (new)
- `vscode_tasks_menu_go/internal/server/agent_run_call_identity_test.go` (new)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (append this section)

### Tests added
- `TestCanonicalRunToolCallResolvesUniqueNamespacedIdentity`
- `TestCanonicalRunToolCallFailsClosed` (empty id, missing, wrong run, legacy out of scope)
- `TestCanonicalRunToolCallLegacyCompatibility`
- `TestCanonicalRunToolCallAmbiguousLegacyFailsClosed`

### Tests actually executed and their results
- Not executed. The native Agent tool surface here exposes COLLECT/PATCH only;
  it does not run `go test`. No test result is claimed as passed.

### Pending verification
- `go test -race -count=1 ./internal/server -run '^TestCanonicalRunToolCall'`
- `go vet ./internal/server`
- Wire the resolver into the receipt projection and re-run the full focused
  run/coordination race suite before declaring M17-02 complete.

### Dependencies and blockers
- Depends on M17-01 (workflow gate; verification pending CI) and the M17-02
  F03 slice (applied, unverified).
- Verification blocker: no shell/CI execution is exposed through the native
  Agent tools in this environment.

### Next actionable step
- Execute the pending Go test command in CI/owner environment; only after it
  passes, wire the resolver into the receipt projection in a separate patch
  with its own tests. Do not mark M17-02 complete from this slice alone.


## M17-02 (core2) - versioned canonical native receipt envelope

Status: IMPLEMENTED_UNVERIFIED

Scope: the receipt-envelope half of the wider M17-02 canonical receipt identity
work package. This adds a versioned, server-owned envelope that binds
owner/project/conversation/run/attempt/tool-call together with canonical
request and result digests and the pinned workspace snapshot, reusing the
existing native Patch success validator. It does NOT persist receipts, does NOT
project into the Session/Task-Plan ledger, and does NOT change any existing
call site. The idempotent two-store projection and the legacy-compatible
reference from run to Session ledger remain outstanding.

### Confirmed findings
- F01/F03/F10 source evidence: a provider CallID is unique only within one
  provider turn, so the same CallID can reappear across retries, sibling runs and
  legacy sessions. Existing lookups resolve tool calls by bare CallID
  (`agent_chat_sync.go:188`, `agent_chat_tools.go:192`,
  `agent_chat_tool_feedback.go:86`, `agent_chat_patch_recovery.go:93`), while
  `agent_run_routing.go:34` already binds `OriginRunID` and fails closed on
  ambiguity. One canonical, namespaced, ambiguity-fail-closed identity is the
  prerequisite for a single receipt per native operation.
- The authoritative success criteria already exist as
  `validateTaskNativePatchSuccess` (`internal/agentchat/task_evidence_validate.go:15`):
  a receipt may only be minted from a final, acknowledged, PASS native result
  whose native run ID matches the accepted receipt.
- Durable native Task receipts already establish the house pattern
  (`agent_chat_task_run_receipt.go`): server-owned metadata, private store,
  digest/HMAC integrity, fail closed on cross-project reuse. The envelope
  follows that pattern and does not introduce a second trust model.

### Source files inspected
- `vscode_tasks_menu_go/internal/server/agent_run_patch_sync.go` (118 lines)
- `vscode_tasks_menu_go/internal/server/agent_run_patch.go` (258 lines)
- `vscode_tasks_menu_go/internal/server/agent_run_history.go` (165 lines)
- `vscode_tasks_menu_go/internal/server/agent_run_feedback_ack.go` (50 lines)
- `vscode_tasks_menu_go/internal/server/agent_run_routing.go` (80 lines)
- `vscode_tasks_menu_go/internal/server/agent_chat_sync.go` (150-230)
- `vscode_tasks_menu_go/internal/server/agent_chat_tools.go` (180-215)
- `vscode_tasks_menu_go/internal/agentchat/store.go` (ToolCallRecord, Save/Get)
- `vscode_tasks_menu_go/internal/agentchat/task_evidence_validate.go` (119 lines)
- `vscode_tasks_menu_go/internal/agentchat/native_run_receipt.go` (149 lines)
- `vscode_tasks_menu_go/internal/agentchat/native_run_receipt_test.go` (90 lines)

### Source files modified by this successor
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` only
  (summary row, predecessor execution-result line, this section).

### Source files already applied by the failed predecessor (not re-applied here)
- `vscode_tasks_menu_go/internal/agentchat/native_run_receipt.go` (new)
- `vscode_tasks_menu_go/internal/agentchat/native_run_receipt_test.go` (new)

### Native Patch execution results
- Predecessor Native Patch "M17-02 canonical native receipt envelope primitive and
  progress update": FAIL; call_id=chatcmpl-tool-ab6ad8464a79aa9d,
  run=1795579765854432_4096865, receipt_hash=3e5d9383909e058d.
  Native COLLECT (m17-02-core2-after-fail) confirmed both Go files were written to
  disk before the failure and that only the documentation edit was missing
  (progress file still ended at the M17-02 (core) section, 230 lines).
- This successor completes only that documentation edit and re-verifies the two
  Go files before touching the tracker. Its own native result is recorded by a
  later read-back of this file.

### Tests added (by the predecessor, present on disk)
- `TestNativeRunReceiptBuildAndVerify` - build, verify and deterministic digest.
- `TestNativeRunReceiptRejectsUnverifiedCall` - unacknowledged result and
  cross-run call are refused.
- `TestNativeRunReceiptFailsClosedOnTamperAndScope` - tampered payload, wrong
  run, wrong project and forged digest are refused.
- `TestNativeRunReceiptLegacyCallWithoutRunBinding` - legacy call without a run
  binding stays compatible.

### Tests actually executed and their results
- Not executed. The native Agent tool surface here exposes COLLECT and PATCH
  only; it does not run `go test`. No test result is claimed as passed.

### Pending verification
- `go test -race -count=1 ./internal/agentchat -run '^TestNativeRunReceipt'`
- `go test -race -count=1 ./internal/server -run '^TestCanonicalRunToolCall'`
- `go vet ./internal/agentchat ./internal/server`
- Full focused run/coordination race suite from the M17-01 pending list.
- Wire the resolver and the envelope into the Session/Task-Plan projection with
  its own duplicate-sync, restart and archived-receipt tests before M17-02 can
  be declared complete.

### Dependencies and blockers
- Depends on M17-01 (workflow gate; verification pending CI) and on the M17-02
  F03 slice plus the M17-02 (core) call-identity slice (both applied, unverified).
- Verification blocker: no shell/CI execution is exposed through the native
  Agent tools in this environment, so every Go change above stays
  IMPLEMENTED_UNVERIFIED.
- Failure lineage: predecessor partially applied; resolved by completing only the
  remaining documentation edit without re-applying the Go changes.

### Next actionable step
- Execute the pending Go test commands in a CI/owner environment. Only after they
  pass, add the idempotent run-to-Session/Task-Plan projection that adopts one
  receipt exactly once, in a separate patch with its own tests.


## M17-02 (projection primitive) - verified receipt to TaskPlanEvidence adapter

Status: IMPLEMENTED_UNVERIFIED

Scope: the idempotency-ready projection primitive of the wider M17-02 canonical
receipt identity work package. It adds a pure adapter that re-verifies a
canonical receipt envelope for the exact RunSpec/project and returns exactly
one TaskPlanEvidence entry with a namespaced Ref, plus a dedup predicate. It does
NOT persist anything, does NOT change any existing call site, and does NOT wire
a sync or scheduler path to the adapter. The actual run-to-Session/Task-Plan
adoption (with duplicate-sync, restart and archived-receipt tests) remains
outstanding.

### Confirmed findings
- `TaskPlanEvidence` has Kind/Ref/Digest/Note (`task_plan.go:24`); the existing
  auto-check convention sets `Ref` to a tool-call identity, `Digest` to a digest
  of the verified result and explicitly notes that independent tests are not
  implied (`task_auto_check.go:62`).
- `nativeRunReceiptEnvelope` and `verifyNativeRunReceipt` already exist and fail
  closed on tampering, wrong run, wrong project and unverified results
  (`native_run_receipt.go`, verified on disk by native COLLECT).
- Target filenames `native_run_receipt_adapter.go` and its `_test.go` were
  confirmed absent (find Coverage status: VERIFIED); no `adoptNativeRunReceipt`
  or `hasTaskPlanEvidenceRef` symbol existed (search Coverage status: VERIFIED).

### Source files inspected
- `vscode_tasks_menu_go/internal/agentchat/task_plan.go` (1-30)
- `vscode_tasks_menu_go/internal/agentchat/task_auto_check.go` (145 lines)
- `vscode_tasks_menu_go/internal/agentchat/native_run_receipt.go` (149 lines)
- `vscode_tasks_menu_go/internal/agentchat/native_run_receipt_test.go` (90 lines)
- `vscode_tasks_menu_go/internal/agentchat/task_evidence_validate.go` (119 lines)
- `vscode_tasks_menu_go/internal/server/ai_assistant.go` (aiPrivatePath, 61)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (330 lines before patch)

### Source files modified by this patch
- `vscode_tasks_menu_go/internal/agentchat/native_run_receipt_adapter.go` (new)
- `vscode_tasks_menu_go/internal/agentchat/native_run_receipt_adapter_test.go` (new)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (append this section)

### Tests added
- `TestAdoptNativeRunReceiptProducesStableNamespacedEvidence`
- `TestAdoptNativeRunReceiptRefusesTamperedOrWrongScope`
- `TestAdoptNativeRunReceiptRefIsNamespacedPerRun`
- `TestHasTaskPlanEvidenceRefIsIdempotent`

### Tests actually executed and their results
- Not executed. The native Agent tool surface here exposes COLLECT and PATCH
  only; it does not run `go test`. No test result is claimed as passed.

### Pending verification
- `go test -race -count=1 ./internal/agentchat -run '^(TestNativeRunReceipt|TestAdoptNativeRunReceipt|TestHasTaskPlanEvidenceRef)'`
- `go vet ./internal/agentchat`
- Wire the adapter and dedup predicate into the run reconciliation path with its
  own duplicate-sync, restart and archived-receipt tests before M17-02 is
  declared complete.

### Dependencies and blockers
- Depends on the M17-02 F03, call-identity and receipt-envelope slices (all
  applied, all IMPLEMENTED_UNVERIFIED).
- Verification blocker: no shell/CI execution is exposed through the native
  Agent tools in this environment.
- Failure lineage: no unresolved predecessor for this additive slice; it does
  not re-apply or modify any earlier M17-02 edit.

### Next actionable step
- Execute the pending Go test command in a CI/owner environment. Only after it
  passes, wire the adapter into one run-owned adoption path in a separate patch
  with duplicate-sync and restart tests. Do not mark M17-02 complete from this
  primitive alone.

## M17-03 - native COLLECT full_file_read proof adapter (bounded slice)

Status: IMPLEMENTED_UNVERIFIED

Scope: the narrow native-COLLECT-to-proof adapter of M17-03. It adds bounded
proof metadata and a pure verifier for one exact full-file COLLECT read inside a
run transcript, plus an archived-page verifier. It returns digests and
identifiers only, never file content, archive bytes or raw provider text. It does
NOT wire any auto-check, sync or scheduler path to the adapter, does NOT change
legacy proof meaning, and does NOT verify an actual native artifact on disk.
The end-to-end native COLLECT fixture, adoption wiring and CI execution remain
outstanding.

### Confirmed findings
- F01 source evidence: the run-native COLLECT bridge
  (`agent_run_collection.go:78` ResolveWaitingTool) records a bounded, JSON-valid
  function_call_output with `OriginRunID`/`ProviderID`/`ModelID` and enforces
  project identity and run provenance before the read; the legacy exact
  full-file proof expects canonical request/output file metadata in Session
  history (`task_auto_check.go:46` VerifyTaskAutoCheck). No secure adapter
  existed between them (target symbols/filenames confirmed absent, Coverage
  status: VERIFIED).
- Reusable validators verified on disk: `validateTaskCompleteRead`
  (`task_evidence_validate.go:81`) enforces start_line=1, parseable total_lines,
  exact path, non-nil content, no error/truncation/continuation and rejects
  page-2-only reads; `collectionCallRequestedPath` (`task_auto_check.go:99`)
  enforces the request named the required file; `taskPlanDigest`
  (`task_plan.go:200`) supplies canonical sha256-hex digests.
- Existing archived path convention verified: `VerifyArchivedTaskAutoCheck`
  (`task_auto_check.go:120`) walks `ArchivedCollectionHistory(x.ID, x.Owner,
  part)` from newest to oldest; the new Store method reuses that pattern.

### Source files inspected
- `vscode_tasks_menu_go/internal/server/agent_run_collection.go` (160 lines)
- `vscode_tasks_menu_go/internal/agentchat/task_auto_check.go` (145 lines)
- `vscode_tasks_menu_go/internal/agentchat/task_evidence_validate.go` (119 lines)
- `vscode_tasks_menu_go/internal/agentchat/task_plan.go` (1-30, taskPlanDigest)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (401 lines before patch)

### Source files modified by this patch
- `vscode_tasks_menu_go/internal/agentchat/collection_read_proof.go` (new)
- `vscode_tasks_menu_go/internal/agentchat/collection_read_proof_test.go` (new)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (summary row + this section)

### Tests added
- `TestVerifyNativeCollectionReadProofAcceptsExactFullRead` - exact full read,
  canonical request/result digests, no raw content in returned metadata.
- `TestVerifyNativeCollectionReadProofRejectsPartialAndWrongPath` - tail-only
  read and wrong path are refused.
- `TestVerifyNativeCollectionReadProofRejectsWrongRunAndAmbiguity` - cross-run
  call and duplicate ambiguous call are refused.
- `TestVerifyNativeCollectionReadProofUnsafePath` - absolute, traversal,
  backslash and non-normalized paths are refused.
- `TestVerifyNativeCollectionReadProofAllowsExplicitEmptyFile` - empty file with
  explicit complete metadata (total_lines 0) is accepted.

### Tests actually executed and their results
- Not executed. The native Agent tool surface here exposes COLLECT and PATCH
  only; it does not run `go test`. No test result is claimed as passed.

### Pending verification
- `go test -race -count=1 ./internal/agentchat -run '^TestVerifyNativeCollectionReadProof'`
- `go vet ./internal/agentchat`
- Real native COLLECT on a fixture plus the full M17-03 negative matrix (corrupt
  ZIP/digest, missing manifest, forged model output, wrong owner/run, archived
  receipt, negative RC, repeated auto-check, native queue count exactly one).
- Wire the adapter into one owner-authorized auto-check path with duplicate-sync
  and restart tests before M17-03 can be declared complete.

### Dependencies and blockers
- The adapter is independent of the unverified M17-02 wiring: it reuses only the
  pre-existing full-file validators and is additive to package agentchat.
- Verification blocker: no shell/CI execution is exposed through the native
  Agent tools in this environment, so this slice stays IMPLEMENTED_UNVERIFIED.
- Failure lineage: no unresolved predecessor for this additive slice; no earlier
  M17-02 edit is re-applied or modified.

### Next actionable step
- Execute the pending Go test command in a CI/owner environment. Only after it
  passes, wire the adapter into one auto-check path with a real native COLLECT
  fixture and complete the remaining M17-03 test matrix in a separate patch.

## M17-04 - fail-closed run-bound automatic Patch dispatch gate

Status: IMPLEMENTED_UNVERIFIED

Scope: the security-critical precondition slice of M17-04. It adds one pure,
additive gate that decides whether a run-bound automatic Patch may be dispatched
for an exact approved call. It reads no files, persists nothing, contacts no
provider, changes no existing call site and never itself approves or executes a
Patch. The run-owned scheduler loop, structured failure delivery, lease/HEAD
binding and the full M17-04 test matrix remain outstanding.

### Confirmed findings
- F02 source evidence: `processAgentAutoFeedbackSession`
  (`agent_chat_auto_feedback.go:92`) returns after `autoCheckTaskPlanOne` for
  run-managed conversations (`agentConversationRequiresRunContinuation`,
  `agent_run_collection.go:17`), so legacy egress is already fenced. The legacy
  path still calls `autoPatchOne` (`:105`) through `agentAutoPatchCandidate`
  (`:154`), which checks only session/call state and `x.AutoPatch || AutoRunOnce`.
- Consent is pinned into each run snapshot as
  `agentrun.AutomationSnapshot{AutoPatch: conversation.AutoPatch, ...}`
  (`agent_task_workflows.go:534`, `agent_context_snapshot_api.go:243`), while the
  live toggle is written only by an explicit owner action
  (`agent_chat_session_automation.go:36-46,79-80`: local owner, not shared,
  `confirm_execution`, permission check). Both sources must agree at dispatch
  time; a pinned-but-revoked grant must not egress.
- `ProviderRun.Phase` values are prepared/sending/unknown/completed/blocked
  (`provider_run.go:18`), and `HasUnresolvedWork` (`store.go:740`) defines the
  fail-closed in-flight set. `canonicalRunToolCall`
  (`agent_run_call_identity.go:22`) supplies the namespaced identity resolver.

### Source files inspected
- `vscode_tasks_menu_go/internal/server/agent_chat_auto_feedback.go` (88-170)
- `vscode_tasks_menu_go/internal/server/agent_chat_session_automation.go` (119 lines)
- `vscode_tasks_menu_go/internal/server/agent_run_continuation.go` (181 lines)
- `vscode_tasks_menu_go/internal/server/agent_run_collection.go` (160 lines)
- `vscode_tasks_menu_go/internal/agentrun/snapshot.go` (19-30)
- `vscode_tasks_menu_go/internal/agentchat/provider_run.go` (18-29)
- `vscode_tasks_menu_go/internal/agentchat/store.go` (740-751)

### Source files modified by this patch
- `vscode_tasks_menu_go/internal/server/agent_run_patch_gate.go` (new)
- `vscode_tasks_menu_go/internal/server/agent_run_patch_gate_test.go` (new)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (summary row + this section)

### Native Patch execution results
- Native Patch "M17-04 fail-closed run-bound auto-Patch dispatch gate and progress update": PASS;
  call_id=chatcmpl-tool-815bffa7cfd8a5a5, run=1795580404869984_4145206,
  receipt_hash=ce9da8b10e1decda (from TaskDeck native Patch history; not re-executed here).
  Native COLLECT (m17-04-verify) confirmed the gate file on disk. A Native Patch PASS proves the
  operation succeeded, not test correctness.

### Tests added
- `TestRunAutoPatchDispatchRequiresPinnedAndLiveConsent`
- `TestRunAutoPatchDispatchBlocksInFlightWork` (provider prepared/sending/unknown; native queuing/queued/dispatching/running/unknown)
- `TestRunAutoPatchDispatchFailsClosedOnScope` (shared, non-local, archived, wrong status)
- `TestRunAutoPatchDispatchResolvesExactCallOnly` (missing id, missing call, wrong run, ambiguous, non-patch call)
- `TestRunAutoPatchDispatchOneShotOnlyWhenEligible` (one-shot, manual-retry, already-reported)

### Tests actually executed and their results
- Not executed. The native Agent tool surface here exposes COLLECT and Python
  PATCH only; it does not run `go test`. No test result is claimed as passed.

### Pending verification
- `go test -race -count=1 ./internal/server -run '^TestRunAutoPatchDispatch'`
- `go vet ./internal/server`
- Wire this gate into one run-owned dispatch path with structured same-run
  failure delivery, lease/HEAD/workspace binding, 20x repeated sync and restart
  tests before M17-04 can be declared complete.

### Dependencies and blockers
- Depends on M17-02 (call-identity and receipt primitives, applied but
  IMPLEMENTED_UNVERIFIED) for the namespaced identity resolver.
- Verification blocker: no shell/CI execution is exposed through the native
  Agent tools in this environment.
- Failure lineage: no unresolved predecessor for this additive gate; no earlier
  M17-02/M17-03 edit is re-applied or modified.

### Next actionable step
- Execute the pending Go test command in a CI/owner environment. Only after it
  passes, wire the gate into one run-owned dispatch path with its own
  authorization, reconciliation and feedback tests. Do not mark M17-04 complete
  from this gate alone.

## M17-05 - ordered proof adoption before next prompt (bounded slice)

Status: IMPLEMENTED_UNVERIFIED

Scope: the narrow ordering-primitive slice of M17-05. It adds one pure function
that reports the first unmet stage in the run-completion chain before a next
owner-authorized provider prompt may be sent, plus table tests. It reads no
files, persists nothing, sends nothing, changes no existing call site and does
not adopt proof into a Task Plan. The live completion-scheduler wiring, CAS
re-read, staged idempotency and end-to-end fixtures remain outstanding.

### Confirmed findings
- M17-05 order requirement from the handoff (lines 99-101): durable completed
  provider result, exact feedback ACK, canonical receipt, owner auto-check,
  fresh plan CAS, continuation decision/snapshot, live consent, then egress;
  each stage idempotent, no egress after opt-out.
- Existing preconditions verified on disk: `HasUnresolvedWork` (`store.go:740`)
  defines the fail-closed in-flight set; `ProviderRun.Phase` values
  prepared/sending/unknown/completed/blocked (`provider_run.go:18`);
  `agentAutoFeedbackCandidate` requires acknowledged final tool calls
  (`agent_chat_auto_feedback.go:29`); `runAutoContinueMode` enforces pinned
  policy AND live session consent (`agent_run_continuation.go:18-38`).
- Target filenames and the new symbol were absent before this patch (native
  COLLECT find/search Coverage status: VERIFIED), so this slice is additive.

### Source files inspected
- `vscode_tasks_menu_go/internal/server/agent_run_continuation.go` (181 lines)
- `vscode_tasks_menu_go/internal/server/agent_chat_auto_feedback.go` (29, 88-170)
- `vscode_tasks_menu_go/internal/agentchat/store.go` (740-751)
- `vscode_tasks_menu_go/internal/agentchat/provider_run.go` (18-29)
- `vscode_tasks_menu_go/internal/agentrun/snapshot.go` (19-30)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (561 lines before patch)

### Source files modified by this patch
- `vscode_tasks_menu_go/internal/server/agent_run_proof_order.go` (new)
- `vscode_tasks_menu_go/internal/server/agent_run_proof_order_test.go` (new)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (M17-04 result,
  M17-05 summary row, this section)

### Tests added
- `TestRunNextPromptOrderStageRequiresAckAndReceipt`
- `TestRunNextPromptOrderStageBlocksInFlightAndScope`
- `TestRunNextPromptOrderStageRequiresLiveConsent`
- `TestRunNextPromptOrderStageNoToolCallNeedsNoReceipt`

### Tests actually executed and their results
- Not executed. The native Agent tool surface here exposes COLLECT and Python
  PATCH only; it does not run `go test`. No test result is claimed as passed.

### Native Patch execution results
- This patch's verified native result is recorded by a later read-back of this
  file; no success is claimed before that result exists.

### Pending verification
- `go test -race -count=1 ./internal/server -run '^TestRunNextPromptOrderStage'`
- `go vet ./internal/server`
- Wire the ordering gate into the run completion path with CAS re-read and
  staged idempotency, then run the full ordered-chain fixture before M17-05 can
  be declared complete.

### Dependencies and blockers
- Depends on M17-02 through M17-04 slices (all applied, all
  IMPLEMENTED_UNVERIFIED) for receipt, proof and dispatch primitives.
- Verification blocker: no shell/CI execution is exposed through the native
  Agent tools in this environment.
- Failure lineage: no unresolved predecessor; this patch does not re-apply or
  modify any earlier M17-02/M17-03/M17-04 edit.

### Next actionable step
- Execute the pending Go test command in a CI/owner environment. Only after it
  passes, wire the gate into the completion scheduler with CAS and egress tests.



## M17-06 - bounded run egress-state classifier

Status: IMPLEMENTED_UNVERIFIED

Scope: the bounded, fail-closed state-classification slice of the unified run
egress state machine. It adds one pure classifier that maps a durable
ProviderRun marker onto a fixed state vocabulary and reuses the existing
ProviderEgressReviewForSession for SENDING/UNKNOWN ambiguity. It reads no files,
contacts no provider, persists nothing, changes no existing call site and grants
no automatic replay. The durable phase/attempt audit, terminal-save/fsync fixes,
local ACK recovery and the full fault matrix remain outstanding.

### Confirmed findings
- F04/F06/F10 source evidence: ProviderRun.Phase values are
  prepared/sending/acknowledged/unknown/completed/blocked (`provider_run.go:18`),
  and ProviderEgressReviewForSession (`provider_egress_review.go:24`) already
  fails closed on SENDING/UNKNOWN with RequiresReconciliation=true and
  AutomaticReplayAllowed=false, while treating provably-unsent PREPARED as
  recoverable through strict original-payload checks.
- RunSpec binds run/attempt/provider/model/config-version/credential fingerprint
  (`runs.go:11`), so a unified classifier can be keyed to the durable run
  without inventing a second trust model.
- Target symbols (ClassifyRunEgress, RunEgressState, RunEgress* constants) and
  target filenames were confirmed absent (search/find Coverage status: VERIFIED).
- The two predecessor attempts were preflight failures with the project left
  unchanged; COLLECT (m17-06-after-fail, m17-06-retry-after-fail) independently
  confirmed no partial writes (classifier files absent, tracker still 640 lines,
  M17-06 row unchanged, no M17-06 section).

### Source files inspected
- `vscode_tasks_menu_go/internal/agentchat/provider_egress_review.go` (62 lines)
- `vscode_tasks_menu_go/internal/agentchat/provider_run.go` (18-29)
- `vscode_tasks_menu_go/internal/agentchat/runs.go` (11-40)
- `TASKDECK_HANDOFF_17_19_POST_MERGE_MASTER_PLAN_20261125.md` (103-108)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (18-39, tail)

### Source files modified by this patch
- `vscode_tasks_menu_go/internal/agentchat/run_egress_state.go` (new)
- `vscode_tasks_menu_go/internal/agentchat/run_egress_state_test.go` (new)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (summary row + this section)

### Native Patch execution results
- Predecessor Native Patch "M17-06 bounded fail-closed run egress-state classifier and tracker update":
  FAIL; call_id=chatcmpl-tool-935fd5cbb9fdf58b, run=1795580641819781_4161842.
  Preflight root resolution failed because TaskDeck stages entrypoint.py outside
  the project tree; project unchanged (verified by COLLECT).
- Predecessor Native Patch "M17-06 egress-state classifier retry with robust root resolution":
  FAIL; call_id=chatcmpl-tool-a237c2ca855fc4be, run=1795580746823388_4169056.
  Preflight exact-match summary-row anchor was too brittle; project unchanged
  (verified by COLLECT).
- This retry resolves the root from cwd and __file__ with a verified walk-up and
- This retry resolves the root from cwd and __file__ with a verified walk-up and
  uses a line-based, substring-safe anchor.
- Successful Native Patch "M17-06 bounded run egress-state classifier retry with
  robust anchors": PASS; call_id=chatcmpl-tool-8a884e2f4de8d740,
  run=1795580827894110_4174970, receipt_hash=ae120d2fd7c48780.
  Post-patch native COLLECT (m17-06-verify, m17-06-result-line,
  m17-06-tail-detail) confirmed both Go files, the updated M17-06 summary row
  and this section on disk. A Native Patch PASS proves only the patch
  operation succeeded, not test correctness; no Go test was executed here.

### Tests added
- `TestClassifyRunEgressStates` (none, prepared, sending, unknown, completed,
  acknowledged, blocked, unrecognized phase)
- `TestClassifyRunEgressFailClosed` (no state grants automatic replay;
  sending/unknown require reconciliation with guidance; prepared/none do not)

### Tests actually executed and their results
- Not executed. The native Agent tool surface here exposes COLLECT and Python
  PATCH only; it does not run `go test`. No test result is claimed as passed.

### Pending verification
- `go test -race -count=1 ./internal/agentchat -run '^TestClassifyRunEgress'`
- `go vet ./internal/agentchat`
- Wire the classifier into the durable phase/attempt audit, terminal-save/fsync
  fix and local ACK recovery path with the M17-06 fault matrix before M17-06 can
  be declared complete.

### Dependencies and blockers
- Depends on M17-02 (receipt primitives) and M17-04/05 gates (all applied, all
  IMPLEMENTED_UNVERIFIED) only as context; this classifier is self-contained and
  re-applies no earlier edit.
- Verification blocker: no shell/CI execution is exposed through the native
  Agent tools in this environment.
- Failure lineage: two preflight-only predecessors resolved by this retry; no
  half-applied source state exists.

### Next actionable step
- Execute the pending Go test command in a CI/owner environment. Only after it
  passes, wire the classifier into the egress audit/ACK-recovery path with the
  fault matrix and hot-switch provider-version test in a separate patch.

## M17-07 - authoritative provider reconciliation (bounded slice)

Status: IMPLEMENTED_UNVERIFIED

Scope: capability-inventory + fail-closed binding classifier. Adds a verdict and
a pure outcome classifier for authoritative provider response lookup. No network,
file I/O, persistence, call-site change or replay permission. Live owner-authorized
lookup transport and provider smoke remain outstanding.

### Confirmed findings
- F10 confirmed: `provider_correlation.go` calls the client request ID a SUPPORT
  LOOKUP HINT, not an acknowledgement/idempotency key; `SupportsClientRequestID`
  allowlists only OpenAI hosts. `provider_egress_review.go` states it is NOT a
  provider-side lookup and never grants replay. No authoritative retrieval exists.
- `RunSpec` binds run/attempt/provider/model/config-version/credential fingerprint
  (`runs.go`); `ProviderStore.GetVersion` fails closed on revoked configs
  (`provider_versions.go`). The verdict reuses those immutable bindings.
- Target files/symbols absent by native COLLECT (find Coverage VERIFIED): additive.

### Source files inspected
- `internal/agentchat/provider_correlation.go`, `provider_egress_review.go`,
  `provider_run.go`, `provider_adapter.go`, `providers.go`, `provider_versions.go`,
  `runs.go`, `provider_egress_review_test.go`
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md`

### Source files modified by this patch
- `internal/agentchat/provider_reconciliation_capability.go` (new)
- `internal/agentchat/provider_reconciliation_capability_test.go` (new)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (row + section)

### Tests added
- `TestReconciliationCapabilityDefaultsUnsupported`, `TestReconciliationCapabilityBindings`,
  `TestValidReconciliationContract`, `TestClassifyReconciliation`,
  `TestReconciliationCredentialBinding`. Every classifier case asserts replay=false.

### Tests actually executed and their results
- Not executed. Native Agent surface exposes COLLECT + Python PATCH only; no `go test`.
  No test claimed passed.

### Native Patch execution results
- Recorded by a later read-back of this file; no success claimed before that exists.

### Pending verification
- `go test -race -count=1 ./internal/agentchat -run '^(TestReconciliation|TestValidReconciliationContract|TestClassifyReconciliation)'`
- `go vet ./internal/agentchat`
- Wire into one owner-authorized read-only lookup path with stubs for in-progress,
  completed, authoritative rejection, unauthorized, 404-uncertain, mismatched,
  unsupported, stale-credential and cancellation before M17-07 is complete. Real
  provider smoke only with an authorized test account; no credentials in files.

### Dependencies and blockers
- M17-06 classifier is context only; this slice is self-contained, re-applies no edit.
- Verification blocker: no shell/CI execution via native Agent tools.
- Failure lineage: no unresolved predecessor; no earlier M17 edit is re-applied.

### Next actionable step
- Run the pending Go test in CI/owner env; only after PASS add the read-only lookup
  transport with full stub coverage and an authorized smoke test. Do not mark
  M17-07 complete from this primitive alone.


## M17-08 - same-host Run Manager fencing (bounded slice)

Status: IMPLEMENTED_UNVERIFIED

Scope: the cross-process fencing foundation of M17-08. It adds a durable,
nonblocking, same-host OS lease keyed to an exact run ID plus a pure admission
policy that refuses to claim a live or uncertain run. It reuses the proven
flock + O_NOFOLLOW + inode-identity guard from the legacy server turn lock.
It does NOT wire the lease into Manager.load/NewManager/ReadRecords, does NOT
change any existing call site, and does NOT claim multi-host or NFS/SMB support.

### Confirmed findings
- F04 GAP confirmed: `internal/agentrun/store.go` load/readRunRecords and
  `internal/agentrun/manager.go` NewManager/execute use only an in-process
  `sync.RWMutex` and `globalRunSlots` channel. No cross-process lease/CAS binds
  worker ownership, so two daemons sharing private local state could both
  admit the same queued run.
- Reusable guard verified on disk: `agent_chat_turn_lock_unix.go`
  `validateAgentTurnLockFile` checks regular file, 0600, `Nlink==1` and
  `os.SameFile` before AND after `Flock(LOCK_EX|LOCK_NB)`; SHA-256
  `d060ac9189303635491ced0fc8417311126e7b1b7aca7efee00b0795205a9b83`.
- Target filenames confirmed absent (native find Coverage status: VERIFIED);
  this slice is additive and re-applies no earlier M17 edit.

### Source files inspected
- `vscode_tasks_menu_go/internal/agentrun/store.go`
- `vscode_tasks_menu_go/internal/agentrun/manager.go`
- `vscode_tasks_menu_go/internal/agentrun/snapshot.go`
- `vscode_tasks_menu_go/internal/server/agent_chat_turn_lock_unix.go`
- `vscode_tasks_menu_go/internal/server/agent_chat_shared_admission_unix.go`
- `vscode_tasks_menu_go/internal/server/agent_chat_workspace_lease.go`
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md`

### Source files added by this patch
- `vscode_tasks_menu_go/internal/agentrun/run_fencing.go`
- `vscode_tasks_menu_go/internal/agentrun/run_fencing_unix.go`
- `vscode_tasks_menu_go/internal/agentrun/run_fencing_other.go`
- `vscode_tasks_menu_go/internal/agentrun/run_fencing_test.go`
- `vscode_tasks_menu_go/internal/agentrun/run_fencing_unix_test.go`
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (row 27 + this section)

### Tests added
- `TestRunFenceDigestStableAndNamespaced`
- `TestRunFenceAdmissionFailClosed`
- `TestAcquireRunFenceExcludesSecondOwner` (POSIX)
- `TestAcquireRunFenceRejectsUnsafeDirectory` (POSIX)

### Tests actually executed and their results
- Not executed. The native Agent tool surface exposes COLLECT and Python PATCH
  only; it does not run `go test`. No test result is claimed as passed.

### Pending verification
- `go test -race -count=1 ./internal/agentrun -run '^TestRunFence'`
- `go vet ./internal/agentrun`
- Two-OS-process reproduction, SIGKILL lease release, stale writer, lock
  symlink/hardlink/replacement and NFS/SMB rejection before M17-08 is complete.

### Dependencies and blockers
- Depends on M17-01/06 only as context; self-contained, re-applies no edit.
- Verification blocker: no shell/CI execution via native Agent tools.

### Next actionable step
- Run the pending Go test in CI/owner env. Only after PASS wire the fence into
  Manager.load/NewManager with cross-process tests. Do not mark M17-08
  complete from this primitive alone.
## M17-11 - controlled workspace drift and snapshot chain (bounded slice)

Status: IMPLEMENTED_UNVERIFIED

Scope: the pure, content-free classification slice of M17-11. It adds one
disposition classifier for workspace/pack drift and one checkpoint-lineage
helper. It reads no files, runs no Git command, contacts no provider, persists
nothing, changes no existing call site and does not adopt a fresh checkpoint.
The live checkpoint publication, fsync/orphan recovery, GC and full test matrix
remain outstanding.

### Confirmed findings
- F08 source evidence: `verifyAgentRunWorkspacePackSnapshot` and
  `agentRunWorkspacePackRef` (agent_run_pack_snapshot.go) already pin the exact
  execution-workspace AI Pack digest in immutable snapshot source refs and fail
  closed when it changes. `agentDriftMatches` (agent_chat_drift.go) enforces
  HEAD/branch/status equality. `serverAgentRunPromptOptions.Options`
  (agent_run_provider_executor.go) re-verifies the pack ref and owner identity
  immediately before provider egress. No pure classifier distinguished an
  authorized, receipt-backed mutation from unrelated drift or a binding switch
  (target filenames/symbols confirmed absent, Coverage status: VERIFIED).
- Reused primitives, not duplicated: pack-ref pinning, Git drift comparison,
  verified native receipt primitives (M17-02) and run-config version binding.

### Source files inspected
- vscode_tasks_menu_go/internal/server/agent_run_pack_snapshot.go (exact SHA verified)
- vscode_tasks_menu_go/internal/server/agent_chat_drift.go (exact SHA verified)
- vscode_tasks_menu_go/internal/server/agent_run_provider_executor.go (exact SHA verified)
- vscode_tasks_menu_go/internal/server/agent_chat_context_rebase.go
- docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md (row/section anchors)

### Source files modified by this patch
- vscode_tasks_menu_go/internal/server/workspace_drift_classifier.go (new)
- vscode_tasks_menu_go/internal/server/workspace_drift_classifier_test.go (new)
- docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md (M17-11 row + this section)

### Tests added
- TestClassifyWorkspaceDriftStable
- TestClassifyWorkspaceDriftBindingChange (workspace, project, provider, config)
- TestClassifyWorkspaceDriftUnverifiable (incomplete binding, missing ref)
- TestClassifyWorkspaceDriftAuthorizedOnlyWithMatchingReceipt (no receipt, wrong run, wrong digest, exact match)
- TestCheckpointLineageID (stable, namespaced, empty on missing input)

### Tests actually executed and their results
- Not executed. The native Agent tool surface here exposes COLLECT and PATCH
  only; it does not run `go test`. No test result is claimed as passed.

### Pending verification
- go test -race -count=1 ./internal/server -run '^(TestClassifyWorkspaceDrift|TestCheckpointLineageID)'
- go vet ./internal/server
- Wire the classifier into one owner-authorized checkpoint path with authorized
  Patch, unrelated edit, HEAD switch, provider-version revocation, stale
  snapshot and worktree isolation fixtures before M17-11 can be declared complete.

### Dependencies and blockers
- Depends on M17-02 receipt primitives and M17-06/08 gates only as context;
  this slice is self-contained and re-applies no earlier edit.
- Verification blocker: no shell/CI execution is exposed through native Agent tools.
- Failure lineage: no unresolved predecessor; M17-11 was NOT_STARTED and no
  earlier M17 edit is re-applied or modified.

### Next actionable step
- Execute the pending Go test in CI/owner env; only after PASS add the atomic
  checkpoint publication/GC wiring with its isolation fixtures. Do not mark
  M17-11 complete from this primitive alone.


## M17-09 - bounded streaming/event history and compaction (bounded slice)

Status: IMPLEMENTED_UNVERIFIED

Scope: the pure policy half of M17-09. It adds a deterministic, content-free
event-compaction planner plus table tests. It reads no files, persists nothing,
contacts no provider, changes no existing call site and never deletes canonical
data. Atomic checkpoint publication, append-only checksummed archive segments,
snapshot GC, disk backpressure and the >512/>256/lifetime benchmark matrix
remain separate, reviewed changes.

### Confirmed findings
- F05 GAP in source: `manager.go` fixes `maxStoredRuns=1000`, `maxRunEvents=512`
  and `maxEventPayload=512<<10`; `appendEvent` rewrites the full record and
  fails at the event limit with no bounded archive/compaction path.
- Reusable policy lives in `contextcompact.Build` (bounded provider-visible
  window, summaries for older Collections/Patches, no canonical-journal mutation).
  The new planner reuses that design direction without importing it: it keeps
  tool call/output pairs, terminal and error events non-elidable.
- Target filenames and symbols were confirmed absent before this patch
  (`find **/*compaction*.go`, Coverage status: VERIFIED; 0 matches).

### Source files inspected
- `vscode_tasks_menu_go/internal/agentrun/manager.go` (constants 19-24,
  appendEvent 728-736, runEventSink 710-725)
- `vscode_tasks_menu_go/internal/agentchat/contextcompact/compact.go` (236 lines)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (summary row 28)

### Source files modified by this patch
- `vscode_tasks_menu_go/internal/agentrun/event_compaction.go` (new)
- `vscode_tasks_menu_go/internal/agentrun/event_compaction_test.go` (new)
- `docs/taskdeck_ai_agent/POST_MERGE_MASTER_PLAN_PROGRESS.md` (summary row + this section)

### Tests added
- `TestPlanEventCompactionBoundedHotSet` (newest bounded suffix retained, input
  slice not mutated)
- `TestPlanEventCompactionNoopWithinLimit`
- `TestPlanEventCompactionPreservesProtectedEvents`
- `TestPlanEventCompactionNoElidablePrefix`
- `TestPlanEventCompactionCursorMonotonicAndClamped`
- `TestElidableEventTypes`

### Tests actually executed and their results
- Not executed. The native Agent tool surface here exposes COLLECT and PATCH
  only; it does not run `go test`. No test result is claimed as passed.

### Pending verification
- `go test -race -count=1 ./internal/agentrun -run '^(TestPlanEventCompaction|TestElidableEventTypes)'`
- `go vet ./internal/agentrun`
- Wire the plan into an append-only checksummed archive with atomic checkpoint
  publication, snapshot GC retaining every referenced run/plan/fork/receipt,
  disk quota/backpressure and the >512/>256/4 MiB/8 MiB/1000+-run fixtures
  before M17-09 can be declared complete.

### Dependencies and blockers
- M17-02/06/08 are context only; this slice is self-contained and re-applies no
  earlier edit.
- Verification blocker: no shell/CI execution via native Agent tools.
- Failure lineage: no unresolved predecessor; no earlier M17 edit is re-applied.

### Next actionable step
- Run the pending Go test in CI/owner env; only after PASS add the durable
  archive/GC wiring with isolation fixtures. Do not mark M17-09 complete from
  this primitive alone.



## UAG-01 — baseline and preservation inventory

Status: INVESTIGATING

Scope: planning/inventory checkpoint only. No unified-engine code change, no Run Manager removal, no provider wiring, and no test execution is claimed by this entry.

### Exact inspected HEAD
- Inspected HEAD: `1e53cfd88` (2025-11-25, native COLLECT git.log, clean worktree `main...origin/main`).
- Planning directive commit: `1e53cfd88` — docs(agent): prioritize single-engine consolidation and Run Manager retirement before Handoff backlog.
- Prior planning baseline remains: `33e845d91df0bac09bb7d74c47e34e29f926e8d4`.
- Tracker pre-patch SHA-256: `bf7d14acadb4576ccd2ad133d04283d6536fa6a82c5bc88d5d5b41d6b324d6e3`.

### Dual-engine inventory (verified by native COLLECT)
| Lane | Primary storage / execution | Admission / API | Browser entry |
|---|---|---|---|
| Legacy unified target | `internal/agentchat/store.go` session journal, turn lease, provider run state | `POST /api/agent/sessions/{id}/messages`, tool APIs under `/api/agent/sessions/` | `web/featuremods/agentchat.js` direct session send when `useRunManager` is false |
| Run Manager (retire) | `internal/agentrun/manager.go` per-owner run journal (`MaxConcurrentRuns=5`, events/history caps) | `POST /api/agent/runs`, `GET/POST /api/agent/runs/*` via `agent_run_api.go` | `web/featuremods/agentchat.js` routes to `agentRunUI.submit` when `useRunManager` is true; cards in `agentchat_runs_ui.js` |

### Confirmed routing defect motivating UAG
- `agentchat.js:2175-2177` sets `useRunManager` when the session is busy, status is `requesting_ai`, provider/model/effort changes, or a run already exists — so a model switch can spawn a **second engine** instead of queued Unified Agent turns.
- `server.go:227-228` exposes both `/api/agent/runs` and `/api/agent/sessions` concurrently.
- `agent_run_routing.go` resolves tool calls and worktree state through Run Manager records (`OriginRunID`, ambiguous binding fails closed).

### File/symbol counts (find coverage: VERIFIED)
- `internal/agentchat/**`: 112 `.go` files (Legacy engine, store, provider, receipts, task plan, compaction helpers).
- `internal/agentrun/**`: 33 `.go` files (Manager scheduler/executor, coordination, DAG, fencing, compaction planner).
- `internal/server/agent_chat_*.go` + related: 183 files (Legacy HTTP surface).
- `internal/server/agent_run_*.go` + coordination/context APIs: overlapping run/admission layer slated for drain then retirement.
- Browser: `agentchat.js`, `agentchat_runs_ui.js`, `agentchat_coordination.js`, bootstrap/isolation `.cjs` tests.

### Capability preservation map (initial)
| Capability | Current source of truth | Unified Agent destination (plan) | UAG owner | Disposition until UAG-08 |
|---|---|---|---|---|
| Canonical session journal + revision CAS | `agentchat/store.go` | Retain / extend | UAG-02 | PRESERVE |
| Per-turn provider/model/effort binding | partial in `RunSpec`, session fields | Immutable turn binding record | UAG-02 | MIGRATE |
| Native COLLECT/PATCH queue + receipts | `agent_chat_native*.go`, `agent_run_patch*.go` | Single gateway on Legacy path | UAG-03 | MERGE |
| Tool approval / auto grants | session automation + run snapshot | Same grants, no migration-created consent | UAG-03 | PRESERVE |
| Composer hot-switch while busy | Run Manager reroute (`useRunManager`) | Serialized next-turn queue on Legacy | UAG-02/UAG-04 | REPLACE |
| Separate run cards / history UI | `agentchat_runs_ui.js` | Unified transcript renderer | UAG-04 | RETIRE UI |
| Run worktrees / execution workspace | `agentrun` + run APIs | Drain then neutral workspace policy or explicit replacement | UAG-05/UAG-06 | DRAIN |
| Groups / coordination / DAG | `agentrun/coordination.go`, workflow APIs | Engine-neutral or Unified scheduler | UAG-06 | PORT |
| Objective evidence / auto-check | `task_auto_check.go`, Handoff 17 gates | Unchanged semantics on Unified engine | UAG-03 | PRESERVE |

### Safety / policy invariants to preserve (no-silent-removal)
- Native Python COLLECT/PATCH contract v6.20.2 remains authoritative; no parallel reader/shell escape (per Handoff 17 UAG-03).
- Project identity guard (`project.key` + `project.id`) on every PATCH/COLLECT.
- Owner/project scoping, explicit external consent, shared-server permission gates.
- Five concurrent conversation/run admission ceiling (`MaxConcurrentRuns = 5`).
- Fail-closed tool-call identity (`agent_run_call_identity.go`, `OriginRunID`).
- No automatic replay of SENDING/UNKNOWN provider egress or native execution without evidence.
- M17-01…M17-19 rows remain `DEFERRED_BY_UAG`; outstanding evidence stays IMPLEMENTED_UNVERIFIED until re-tested on Unified engine.

### Active work freeze
- **Frozen:** new Run Manager features, expanding `useRunManager` routing, new `/api/agent/runs` consumers.
- **Allowed:** UAG transition slices, draining/reconciling existing runs, documentation, additive tests for Unified path, bugfixes that unblock build/migrate/verify.

### Native COLLECT evidence (this turn)
| Purpose | run_id | result_zip (project-relative) | status |
|---|---|---|---|
| Handoff + tracker pack | `agent_collect_3fed321af601ced32db00d10` | `artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_collect_20261125_144717_356019_816093.zip` | PASS |
| Module inventory find | `agent_collect_ae309edbbea5c7752aa9d518` | `artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_collect_20261125_144748_509292_818244.zip` | PASS |
| Git HEAD + RunManager refs | `agent_collect_3256cb45e57fd42ad9649784` | `artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_collect_20261125_144806_913732_819464.zip` | INCOMPLETE (partial search) |
| API surface + routing pack | `agent_collect_6fa38dedd39e914f79d47dd8` | `artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_collect_20261125_144833_732269_821118.zip` | INCOMPLETE (partial search) |
| Tracker tail/head | `agent_collect_d530c39bbef88f85175fd8b5` | `artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_collect_20261125_144908_024551_823451.zip` | PASS |
| Pre-patch SHA gate | `agent_collect_a563f51da83bfa2ece23c88e` | `artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_collect_20261125_144925_951335_824690.zip` | PASS |

### Tests / CI actually executed
- Not executed in this agent turn. Native tools here expose COLLECT/PATCH only; no `go test`, `node --test`, or workflow dispatch was run. No CI PASS is claimed.

### Pending verification (UAG-01 closeout)
- Owner/CI: baseline build commands from M17-01 pending list on HEAD `1e53cfd88`.
- Document M17→UAG replacement matrix in this file (start during UAG-01; finalize at UAG-08).
- Recovery/rollback design note before any destructive UAG-07 step (private backup + migration manifest checksum).

### Blockers
- Build/test baseline not yet executed in a verifiable environment (verification-only blocker).
- UAG-02+ implementation intentionally blocked until UAG-01 inventory is accepted in tracker (this checkpoint).

### Next actionable step
- **UAG-01 completion:** owner/CI run the M17-01 pending Go/Node commands on `1e53cfd88`, attach exact exit codes, then mark UAG-01 `IMPLEMENTED_UNVERIFIED` with evidence refs.
- **UAG-02 start (after above):** add immutable per-turn binding metadata on Legacy session journal and remove browser dependency on `useRunManager` for ordinary sends (design in Handoff 17 UAG-02).

## UAG-02 — immutable turn binding, hot-switch and concurrency

Status: IMPLEMENTED_UNVERIFIED

### Exact inspected HEAD
- Worktree parent: `52488ea1e03a4921fea1bfaed38cda6da0485c48` (pre-local edits); implementation applied on that baseline.

### Source files modified / added
- `vscode_tasks_menu_go/internal/agentchat/turn_binding.go` (new)
- `vscode_tasks_menu_go/internal/agentchat/pending_user_turn.go` (new)
- `vscode_tasks_menu_go/internal/agentchat/turn_binding_test.go` (new)
- `vscode_tasks_menu_go/internal/agentchat/store.go` (`TurnBinding`, `PendingUserTurn`, validation)
- `vscode_tasks_menu_go/internal/server/agent_chat_turn_binding.go` (new)
- `vscode_tasks_menu_go/internal/server/agent_chat_pending_turn.go` (new)
- `vscode_tasks_menu_go/internal/server/agent_chat_turn_binding_test.go` (new)
- `vscode_tasks_menu_go/internal/server/agent_chat_messages.go` (binding pin, queue, provider override fields, pending dispatch)
- `vscode_tasks_menu_go/web/featuremods/agentchat.js` (Run Manager drain-only; composer fields on send; queued response UX)
- `vscode_tasks_menu_go/internal/agentrun/run_fencing_unix_test.go` (vet-safe Fatalf)

### Behavior delivered
- Immutable per-turn `TurnBinding` persisted at PREPARED and cleared on `turn_finished`.
- Busy sessions queue exactly one `PendingUserTurn` with pinned provider/model/effort (HTTP 202) instead of opening Run Manager.
- Composer `provider_id` / `model_id` / `reasoning_effort` accepted on `POST /messages` for the next Legacy turn.
- Run Manager UI routing limited to existing run drain (`hasRun`); hot-switch while busy uses Legacy queue.
- Pending turn auto-dispatched via internal message admission after a completed Legacy turn.

### Tests actually executed
- `go test -count=1 ./internal/agentchat/... -run TurnBinding` — PASS
- `go test -count=1 ./internal/server -run 'TurnBinding|AgentPinActive'` — PASS
- `go test -race -count=1 ./internal/agentchat/...` — PASS
- `node --check web/featuremods/agentchat.js` — PASS
- `go test -race -count=1 ./internal/agentrun/...` — FAIL (`TestAcquireRunFenceExcludesSecondOwner`: same-host run fence unavailable in environment)
- Full `./internal/server -run TestAgentChat` — FAIL (`TestAgentChatIdempotentRequestReplaysWithoutSecondProviderCall`: turn lock 409; triage owner/CI)

### Pending verification
- Browser: hot-switch composer-2.5 → gpt-6.1-sol while a turn is active (queue + single transcript).
- CI expanded workflow on commit SHA with run-only paths.
- Confirm no new `/api/agent/runs` submissions from ordinary Send after change.

### Next actionable step
- **UAG-03 start:** port native tool probe/fallback + approval parity exclusively through Legacy `agent_chat_messages` / tool feedback paths; keep Run Manager tool path frozen.


### UAG-02 follow-up — synchronized baseline and pending binding safety
- Inspected local HEAD `dbbdc55fd` (main and locally known origin/main equal); initial staged and unstaged diffs were empty. Native COLLECT run `agent_collect_6460e864ad29126b20833784`.
- A later collection observed an independent unstaged edit to `agent_chat_messages.go` guarding idle dispatch; it was not modified by this patch. Do not assume the worktree remains clean.
- `Store.Save` currently rejects changed provider/model fields; pending dispatch previously cleared the pending record before using a stale revision and did not compare its stored provider/config/credential fingerprints. The source-only change checks those fingerprints before clearing and uses the returned revision. It does not solve atomic queue handoff, provider/model hot-switch persistence, or the active-turn lock preventing busy admission.
- Added a binding-comparison unit test; no tests executed by this patch. Prior UAG-02 failures and browser/CI verification remain open. UAG-02 is IMPLEMENTED_UNVERIFIED; UAG-03 through UAG-08 are not complete; M17–M19 remain deferred.
- Next: verify this exact change and triage queue handoff, owner identity, active-turn admission, provider/model switch and the recorded race/fence failures before the UAG-02 gate can close. Preserve queued requests and never replay ambiguous egress.

### UAG-02 follow-up — pending dispatcher lease ordering
- On locally synchronized HEAD `dbbdc55fd`, native COLLECT run `agent_collect_75d1dc55f8f44a4373fbe864` confirmed the pending binding guard and tracked follow-up. Subsequent COLLECT `agent_collect_d0cb709a94fc107b527fa9f6` showed the pending dispatcher attempts the same nonblocking turn lock while the message handler still holds it.
- Release the idempotent handler lease before launching a pending dispatcher; this removes that particular scheduling race. This does not make the queue handoff atomic, enable admission while a turn owns the lock, resolve provider/model Store.Save restrictions, or fix owner identity. No tests have been executed for this change.
- UAG-02 remains IMPLEMENTED_UNVERIFIED. Next: verify queue/lock behavior and repair the atomic handoff and busy admission without losing messages or replaying ambiguous work; then complete the documented test and browser gate before UAG-03. M17–M19 remain deferred.


### UAG-02 follow-up — lease ordering regression
- Extracted the existing release-then-dispatch ordering into a helper and added a deterministic ordering test. This test does not validate atomic handoff, real lease behavior, owner scoping, or busy admission.
- Native COLLECT inspected current files before this change; no tests were executed for this patch. UAG-02 remains IMPLEMENTED_UNVERIFIED; M17–M19 remain deferred.
- Next: run the focused server regression and resolve queued-turn data loss, admission and provider-switch defects before advancing.

### UAG-02 follow-up — pending turn admission guard
- A new ordinary message could overtake an existing durable pending turn when the session otherwise accepted messages. Added a fail-closed conflict after previous-request replay and a focused regression test asserting no provider call and retention of the queued turn.
- Source-only change; test execution and full queue handoff are not verified. This does not make pending dispatch atomic, enable busy admission, or solve provider-switch persistence. UAG-02 remains IMPLEMENTED_UNVERIFIED; UAG-03–08 and M17–M19 remain gated.
- Next: run focused server tests and implement crash-safe atomic handoff with owner and binding preservation before enabling queued egress.

### UAG-02 follow-up — unsafe pending dispatch suspended
- COLLECT `agent_collect_beb9c63549969506dc388d4c` showed the dispatcher removes the pending record in a separate journal save before re-entering message admission. A crash or admission failure after that save can lose the queued turn.
- Temporarily fail closed before removal or provider egress; retain the pending turn for owner review. This deliberately suspends automatic pending dispatch until atomic PREPARED admission is implemented; it is not UAG-02 completion. Existing implementation remains below the gate.
- No tests executed by this source-only patch. Ordering, pending admission, busy admission, provider switching and previous failures remain unverified. M17–M19 remain deferred.
- Next: implement an owner-scoped atomic pending-to-PREPARED transition without replaying ambiguous egress; add crash and concurrency tests, then verify the UAG-02 gate.

### UAG-02 follow-up — explicit owner admission seam
- COLLECT `agent_collect_ef340c412233d8b26b5bc8ec` confirmed the retention test is present and the message path derives owner from request context. The public handler now passes that owner explicitly to a private admission function; existing dispatch remains suspended. This is a refactor only, not atomic queue handoff or permission to impersonate an owner.
- No tests executed in this patch. UAG-02 remains IMPLEMENTED_UNVERIFIED; M17–M19 remain deferred. Next: implement atomic pending-to-PREPARED adoption, preserving owner, exact payload, CAS and ambiguous-egress safety; run crash/concurrency and owner-scope tests.

### UAG-02 follow-up — owner admission isolation regression
- Added a focused test for the explicit-owner admission seam: a foreign owner must receive not-found, produce no provider egress, and leave the local journal unchanged. Test is source-only and has not been executed.
- COLLECT `agent_collect_6210784d45aaf47069c9c736` confirmed the seam and suspended dispatcher. Atomic pending-to-PREPARED handoff, busy admission, provider switching and the recorded verification failures remain open. UAG-02 is not verified; UAG-03–08 and M17–M19 remain gated.

### UAG-02 follow-up — PREPARED binding recovery fence
- Inspection of `agent_chat_provider_resume.go` found that recovery checks provider capability fingerprint but previously did not compare a persisted turn binding against the current credential/configuration. This patch adds a fail-closed check for bound turns; legacy unbound recovery is unchanged. A regression test was added but not executed.
- COLLECT run `agent_collect_c488c5e3799e9c2fc152ab44`; exact pending-to-PREPARED handoff, busy admission, provider switching, crash/concurrency and browser validation remain open. Pending dispatch stays suspended; UAG-02 is unverified and UAG-03–08 and M17–M19 remain gated.

### UAG-02 follow-up — protect committed PREPARED sidecar
- Reject staging over a committed PREPARED sidecar and add a retention regression. The two prior failed packages were deleted from the normal queue; current source was recollected before this successor.
- Tests have not been executed for this change. Atomic owner-scoped pending handoff, crash and concurrency coverage, busy admission and provider switching remain open; pending dispatch stays disabled and UAG-02 is unverified. M17–M19 remain deferred.

### UAG-02 follow-up — focused verification runner
- Added a project-local runner for existing PREPARED retention, restart, credential drift, ambiguous egress and suspended-dispatch tests. It disables Go dependency downloads and was not run by this patch.
- Next: execute this already-existing runner in a separate native PATCH, record its actual result, and continue owner-scoped atomic pending adoption. UAG-02 remains unverified; UAG-03–08 and M17–M19 remain gated.

### UAG-02 follow-up — PREPARED cleanup protection
- Native PATCH runs 1795597453889617_1172930 and 1795597489858536_1175688 passed for the cleanup guard and its regression source respectively. COLLECT agent_collect_7dafb5cb8ab3560d8dd97437 confirms both are present. The regression checks retention after reopen and cleanup after SENDING.
- No tests were executed by these PATCHes. The focused runner remains unverified. Atomic owner-scoped pending adoption, crash and concurrency tests, busy admission and provider switching remain open; pending dispatch remains disabled and UAG-02 is not accepted. UAG-03–08 and M17–M19 remain gated.

### UAG-02 follow-up — provider selection persistence investigation
- COLLECT agent_collect_2d2081fafb34824447a80ed5 found that Store.Save rejects changes to ProviderID or ModelID, while agentResolveTurnModel changes the session selection for a requested next turn. Provider switching persistence therefore requires a separately designed and tested admission path; do not weaken owner, revision or active-binding checks.
- The focused runner now uses verbose output and rejects zero matching tests, but no test execution is confirmed. The Python-only submission interface does not expose command-only manifest or post_patch commands; do not invoke the runner from a Python payload as a workaround. Pending dispatch stays disabled; UAG-02 remains unverified and UAG-03–08 and M17–M19 remain gated. Next: obtain a supported command-capable native route for the existing runner, then implement and test atomic owner-scoped admission and provider switching.

### UAG-02 follow-up — provider selection regression checkpoint
- Native PATCH runs 1795597684738931_1188169 and 1795597714702393_1190013 passed for the unscoped provider/model selection regression and its inclusion in the focused runner. COLLECT agent_collect_16dd9699d36dfe44254ad7ff confirms both source entries.
- These receipts confirm source changes only; no test execution is confirmed. The exposed Python-only PATCH interface cannot submit the command-only manifest required to run the existing runner. Atomic owner-scoped pending adoption, crash/concurrency checks, busy admission and provider switching remain open; dispatch stays disabled, UAG-02 remains unverified, and UAG-03–08 and M17–M19 remain gated.

### UAG-02 follow-up — staged payload CAS checkpoint
- Native PATCH runs 1795597792691129_1195340, 1795597831687017_1197800 and 1795597870694471_1200163 passed for the PREPARED sidecar journal fence, its competing-stage regression source, and inclusion in the focused runner. COLLECT agent_collect_ea8a19b480bdbb0e6eb81911 confirmed the regression and runner entry.
- These are source-change receipts, not test results. No execution of this focused regression is confirmed. Owner-scoped atomic pending adoption, provider switching and crash/concurrency verification remain open; pending dispatch stays disabled. UAG-02 remains unverified; UAG-03–08 and M17–M19 remain gated. Next: implement the atomic handoff and obtain supported command-capable native execution for the existing focused checks.

### UAG-02 follow-up — pending-turn removal guard checkpoint
- Native PATCH runs 1795597954768110_1205690 and 1795597993696379_1208591 passed for the generic Save pending-removal guard and regression source. COLLECT agent_collect_4d8c5a2f648a735bb243f15b and agent_collect_691107e2362a9136b36734c8 confirm the guard, regression, and disabled dispatch gate.
- The regression is included in the focused runner by this source change; no test execution is confirmed. Owner-scoped atomic pending adoption, provider switching and crash/concurrency verification remain open. UAG-02 is unverified; UAG-03–08 and M17–M19 remain gated. Next: implement atomic admission and obtain a supported command-capable native route to execute focused checks.

### UAG-02 follow-up — pending replacement guard
- Native PATCH runs 1795598065707055_1213046 and 1795598098827468_1215058 passed for the generic Save replacement guard and regression source. COLLECT agent_collect_729b628a062dbc5bdda24960 and agent_collect_3ec024ad6d15e4b678840c48 confirmed the guard and regression. The focused runner includes the regression in this change.
- No test execution is confirmed. Owner-scoped atomic pending admission and provider switching remain open; dispatch stays disabled pending verification. UAG-02 is unverified; UAG-03–08 and M17–M19 remain gated. Next: implement atomic handoff and obtain a supported command-capable native route for focused tests.
