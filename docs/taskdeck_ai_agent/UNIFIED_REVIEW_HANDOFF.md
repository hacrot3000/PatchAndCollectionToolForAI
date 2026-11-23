# TASKDECK AI Agent — Unified Review Handoff

Review date: 2026-11-23

## 1. Baseline and Git state

- Repository: `hacrot3000/PatchAndCollectionToolForAI`
- Branch: `review/taskdeck-multiconversation-hotswitch-20261123`
- Original review baseline: `2fdf4dd15a9a436a7fc6388df5e400c2369c3a40`.
- Current pass baseline / HEAD: `9aa87d61dc048532f9604cba313a95d91e56fcc0` (exact match at the start of this pass).
- Worktree: branch `review/taskdeck-multiconversation-hotswitch-20261123`; all changes unstaged.
- `git fetch origin` completed again in this pass. Divergence: `origin/main` is 93 commits ahead and review is 4 commits ahead.
- No merge, commit, push, reset, or branch switch was performed.

## 2. Integration status

**NOT READY FOR MERGE.** The review branch is 93 commits behind `origin/main`; ten Agent Chat/Chat UI files have edits on both sides (see the updated conflict table). The full server suite still has the same 24 pre-existing UI assertion failures. Feature-specific Go and Node tests pass, including a real HTTPS provider stub test executed inside an isolated namespace and process-kill recovery tests. Browser-level end-to-end scenarios and an external provider smoke test remain blocked in this environment.

## 3. Code present and evidence

### PLAN 01 — Multi-Conversation / Shared Context / Coordination

- Conversation registry, owner/project-bound groups, immutable snapshots, run manager, five-run global cap, worktree bindings, coordination hub and task workflow code are present in the baseline.
- `ForkConversation` records lineage before reading its snapshot; selected `OriginRunID` is included through that turn, later turns are excluded, and an unknown boundary fails closed. Tests cover start/middle/end, active source, independent forks and an invalid boundary.
- Completed run history can be read from the persisted journal without constructing a Manager or starting provider workers. Snapshot-after-restart test verifies the persisted prompt and completed output are present.
- Standard context snapshots now include a group’s shared instructions only for member conversations, with revision and SHA-256 provenance in `SourceRefs`. Tests verify member inclusion, independent-conversation isolation and snapshot immutability after a later group revision.
- Shared-server routing now maps conversation/group/coordination routes. Mutations require `settings.write` at both the route and handler gates. Coordination writes validate owner/project and, for group-scoped actions, current group membership. Peer ask/answer, non-member denial and AddMember failure paths have focused tests.
- Groups & Coordination panel is loaded from `next.js`; it supports group creation/membership/instructions, peer progress and ask/answer, contract publish/advance, artifact/dependency registration and feeds. Dependency graph, blocked-item controls, event-to-run cross-links and browser E2E remain incomplete.

### PLAN 02 — Hot-Switch / Provider

- The branch contains the run-scoped provider router/adapter, provider payload conversion, immutable provider/config/credential bindings, retry policy, run cards and provider selector.
- `TestComposerRunCanOverlapGPTRunRetryAndStopIndependently` covers overlapping Composer #101 and GPT #102, independent completion, retry binding and run-specific stop with a deterministic executor.
- Canonical tool continuation serialization is covered for both Responses and Chat Completions: function call and matching output are replayed in order. Provider tool integration tests use a deterministic provider invoker and fake native bridge.
- A separate server test runs the actual repository `python_patch_entry.py` via a real native session manager: approve → one queue package → stop the Agent Run while Patch continues → reconcile result → manager restart → same-run continuation. The model response is deterministic test input; this does not constitute a real provider call.

### Validation performed in this pass (`9aa87d61` baseline)

- PASS: `go build ./cmd/vscode_tasks_menu`, `go vet ./internal/agentrun ./internal/agentchat ./internal/server`.
- PASS: `go test -race -count=1 ./internal/agentchat ./internal/agentrun`.
- PASS: focused server suite for group authorization, migration isolation/idempotency, native recovery and real-provider namespace integration; repeated with `-race`.
- PASS: isolated HTTPS provider integration (`TestRealProviderTransportRunsInsideIsolatedNamespace`) — real TLS handshake, DNS/IP guard, bearer auth and canonical request/response through the configured provider adapter. Explicit SKIP applies only to the individual provider tests when run outside the namespace.
- PASS: `node --test web/*_test.cjs` (10/10).
- PASS: `git diff --check` and `gofmt` on all changed Go files.
- FAIL (pre-existing, 24 tests): full `go test ./internal/server -count=1` still fails on Auth/Editor/Terminal/Patch UI source-contract assertions and `TestSharedAdminAssignRestrictedRoleWithoutEditPermissions`; direct run completed in 595 s with no panic or data race. These failures reproduce on `origin/main` and are outside this pass’s changes.
- BLOCKED: external provider smoke (no configured external endpoint or credential; no credential was read or emitted). Browser-level E2E remains unavailable (no Playwright runtime installed).

### P1 — Fixed and verified in this pass

- Real HTTPS provider stub: certificate SAN matches the provider hostname; TLS, DNS/IP guard and credential assertions remain. The isolated namespace wrapper reports PASS with three subtests passing; outside the namespace the direct cases report explicit SKIP with the reason.
- Process-kill recovery: streaming and uncertain-provider runs are recovered as `interrupted`, are not marked retry-safe and require explicit review; waiting-tool history and native Patch ledger survive. A reviewed retry performs exactly one additional provider attempt.
- Full server suite: 24 failures, identical to the previously recorded baseline set on `origin/main`. No failure was removed or had its assertion weakened; no timeout was increased to hide a failure.

## 4. Findings by priority

### P0 — Fixed in this pass (`9aa87d61` baseline)

- **Group authorization was workspace-only.** Added explicit `PrincipalOwner`, `ManagerPrincipals` and `SharedPrincipals` to `agentrun.ConversationGroup`, plus `GroupAccess`, `ManagedBy`, `ReadableBy`, `AccessibleGroups`, `CreateGroupForPrincipal`, `SetGroupManagers` and `ShareGroup`. Group list/item APIs now hide groups of other owners, require management rights for instructions/membership, reject self-adding to an inaccessible group, and preserve legacy groups by falling back to member-conversation ownership when no principal owner was recorded. Regression tests cover owner A/B, group A/B, read-only managers, shared read access, non-members, foreign conversations and legacy groups.
- **Legacy coordination migration could attribute ambiguous records.** Rewrote the migration in a dedicated file: unscoped/foreign peers, messages, contracts, artifacts and dependencies are quarantined into `unclaimed.json` with a reason instead of being copied to an owner; the legacy journal is never deleted. Migration is idempotent (an existing owner journal is not rewritten) and proven with two owners, two groups, unscoped records and three repeated runs.
- **HTTPS provider test depended on the host and had a hostname-mismatched certificate.** The stub certificate now includes the provider hostname in its SAN, and all transport tests run inside an isolated user+mount+network namespace with a private hosts file. Outside the namespace the tests report explicit SKIP, never silent PASS. The test is labelled a local HTTPS stub integration and does not call an external provider.
- **Recovery after process kill had no direct process-level test.** Added real subprocess-kill coverage for streaming and uncertain provider outcomes, waiting-tool state, reload/retry with exactly one reviewed attempt, and native Patch ledger persistence. Recovered runs are interrupted with `RetrySafe=false` and require explicit review before any side effect is replayed.


### P0 — Fixed in review code / baseline

- Fork boundary leakage: fixed and regression-tested.
- Unpinned child-workspace snapshot response: snapshot creation now rejects a child workspace without provider/model/run binding. The valid bound response reports the child ID and can submit a run from that response.
- Completed run history after restart: read-only journal loading is available when no Manager was opened.
- Native Patch bridge: approval/rejection/cancel/restart/continuation are covered; an actual Python Patch run is verified once, while provider-to-native integration remains deterministic at the provider boundary.
- Shared group instructions leaking or disappearing: member-only snapshot injection is now revision/hash pinned and tested.

### P1 — Fixed in this review pass

- Shared-mode conversation/group/coordination routes were unmapped; route permissions are now explicit.
- POST mutations were gated only by `settings.read`; group/conversation/coordination/context-group handlers now require `settings.write`.
- Coordination events could be attributed to conversations outside the owner/project or group; API writes now validate conversation ownership and group membership for group-scoped operations.
- AddMember now rejects a conversation that does not exist in the owner project; rejected membership no longer returns success.
- `.gitignore` used a one-level `*/__pycache__/*` pattern; it now ignores `__pycache__/` at any depth.

### P0/P1 — Still blocked or incomplete

- No workspace-scoped provider configuration/credential store was found for this repository, so no real endpoint smoke test was attempted. No credential was read or emitted.
- Browser-level #101/#102 switching, tab changes during live polling, server-process kill/restart, and shared-server login through a browser were not exercised. Existing tests use deterministic executors and Node DOM harnesses.
- The one end-to-end native Patch test uses a deterministic model/tool-call executor. The native queue, Python Patch runtime, approval, result reconciliation and continuation are real; the provider network call is not.
- Shared coordination context digests are not assembled into run snapshots. The UI still lacks dependency graph/blocked queue and event-to-run navigation.
- Retry/stop isolation is covered by deterministic tests, but no live-provider ambiguity/retry smoke test was possible.

### P1 — Full server baseline failures

`go test ./internal/server -count=1` failed at the stated baseline after 567.103 seconds with 24 assertion failures. No timeout was hidden by raising a limit. The failures were in existing Auth/Editor/Terminal/Patch UI assertions; their source/test files are unchanged by the two review commits, except `menus.js` has only the Run All/Agent menu insertion. The failure set was:

- `TestAuthenticationModeWizardLoadsBeforeSettingsMenu`
- `TestEditorSessionPersistenceFrontendContract`
- `TestEditorSmartEnterPreservesIndentAndSplitsPairs`
- `TestEditorOpeningBraceKeepsCurrentIndent`
- `TestEditorDirtySaveShortcutsAndCloseFlow`
- `TestEditorClosesCleanTabsForTrashedProjectPaths`
- `TestBuiltInBrowserUIUsesEnglish`
- `TestViewActivationEventsDistinguishExplicitTabSwitches`
- `TestExplicitTerminalAndTaskLaunchesBreakForegroundLock`
- `TestExplicitForceBreaksForegroundLockWithoutDOMTrustHeuristics`
- `TestExplicitWorkspaceOpenPathsForceForegroundWhileRestoreRemainsPassive`
- `TestPatchForegroundLockCoversAllWorkspaceOpenPaths`
- `TestTerminalDetectsReleasePathsWithSpacesAndDecoratedLabels`
- `TestGroupedMenusFeature`
- `TestMobileTabContextCanBeOpenedWithoutRightClick`
- `TestPatchProtocolParityAndHeadlessBackingGate`
- `TestPatchNativeProductAcceptanceGate`
- `TestPatchForegroundRunStaysAboveBackgroundActiveRuns`
- `TestRestartInPlaceRetargetsDetectionState`
- `TestSharedAdminAssignRestrictedRoleWithoutEditPermissions`
- `TestEditorTabContextMenuUsesEditorOnlyActions`
- `TestFeatureTabsSupportPersistentReadOnlyMode`
- `TestEditorTabContextKeepsQuickActionsAndGroupsAdvancedCommands`
- `TestCoreUIRestoresSSHMinusOneTerminalButKeepsPatchHeadless`

Failure causes recorded by the baseline run: Auth/menu/editor/tab tests report missing expected source-contract snippets; browser help still contains Vietnamese text; terminal file download uses `+` for spaces while the test expects `%20`; Patch UI tests report missing headless/foreground gates or wrong run ordering; restart test lacks a `clearLog` discriminator; the restricted-role endpoint returns `release_ready:true` where the test expects false. These are assertion failures (no panic stack); the test names and emitted `t.Fatalf` messages above identify the exact cases. The restricted-role gate and the other affected assets are outside PLAN 01/02 and unchanged by this review. The full suite was not rerun after this review’s narrow Agent-route/snapshot fixes; focused regressions for those files pass.

## 5. PLAN checkbox reconciliation

- PLAN 01: 6/139 child boxes checked, now including process-kill recovery (`MC-02.03.01`, `MC-02.03.02`) in addition to previously evidenced fork, event-log, correlation/TTL, contract gate and shared-context items. Parent/milestone boxes remain unchecked until every child and browser acceptance criterion is satisfied.
- PLAN 02: 14/126 child boxes checked, now including stream reconnect/retry idempotency and interrupted-run recovery (`HS-05.02.02`, `HS-05.02.03`) alongside the previously evidenced contract/binding/UI items. External-provider and browser E2E boxes remain unchecked.
- Parent and milestone boxes remain unchecked until every child and acceptance gate is satisfied. The two source plans remain the authoritative requirements.

## 6. Main-branch comparison and conflict plan

The 93 main-only commits include durable provider attempt correlation/recovery, HTTP 499 retry recovery, directory-fsync crash boundaries, provider heartbeat/review, context compaction, task-plan segmentation/handoff and related collection/retry changes. Ten files are changed on both branches:

| Shared file | Integration risk / resolution |
|---|---|
| `internal/agentchat/chat_completions_stream.go` | Reconcile review run-scoped streaming/tool events with main’s incomplete-stream recovery; retain one parser and one event contract. |
| `internal/agentchat/providers.go` | Preserve both the review router binding and main’s durable provider configuration/recovery semantics; do not replace either file wholesale. |
| `internal/agentchat/responses.go` | Merge canonical tool/history conversion with main’s durable response/handoff changes; keep function-call/output ordering tests for both APIs. |
| `internal/agentchat/stream.go` | Merge stream event handling with main’s retry/resume/handoff lifecycle; keep one stream state machine. |
| `internal/server/agent_chat_sessions.go` | Preserve legacy session ownership/history while integrating run cards and main’s durable provider resume path. |
| `internal/server/agent_chat_sync.go` | Combine native Patch reconciliation with main’s patch archive/provider recovery; keep uncertain results fail-closed. |
| `internal/server/agent_chat_tool_feedback.go` | Reconcile one-shot feedback/continuation with main’s durable handoff and Patch receipt behavior. |
| `web/featuremods/agentchat.js` | Manually combine the multi-run/provider selector UI with main’s attachments, retry, recovery, and long-run controls. |
| `web/agentchat_bootstrap_test.cjs` | Merge browser contracts from both feature sets; do not discard tests to make strings pass. |
| `internal/patchtool/` and root Patch runtime (main-only) | Preserve main’s durable provider recovery/Patch fixes and this branch’s approval→execution→continuation bridge; run the native Patch end-to-end test after integration. |
| `vscode_tasks_menu_go/internal/server/agent_chat_provider_resume.go`, `agent_chat_retry.go` (main-only) | Do not replace the single Run Manager; map review run bindings and tool reconciliation onto main’s durable resume path and verify no double side effect. |
| `vscode_tasks_menu_go/internal/server/agent_chat_shared_admission*.go` (main-only) | Combine with review’s explicit group owner/manager/member model; shared-daemon capacity admission must not bypass group authorization. |

No main changes were integrated. The owner’s explicit authorization is still required before any merge/rebase into this branch. Recommended next step after authorization: create a protected integration worktree from current `origin/main`, transplant/merge review changes by the conflict table, run all existing main durability tests plus the focused Agent suites, then request browser/provider smoke credentials and acceptance.

## 7. Files changed in this review pass (`9aa87d61` baseline)

Production code:
- `vscode_tasks_menu_go/internal/agentrun/conversations.go`
- `vscode_tasks_menu_go/internal/server/agent_conversation_api.go`
- `vscode_tasks_menu_go/internal/server/agent_conversations_api.go`
- `vscode_tasks_menu_go/internal/server/agent_coordination_api.go` (scope validation retained; no functional change in this pass)
- `vscode_tasks_menu_go/internal/server/agent_coordination_migration.go` (new)

Tests and documentation:
- `vscode_tasks_menu_go/internal/server/agent_conversation_group_authorization_test.go` (new)
- `vscode_tasks_menu_go/internal/server/agent_coordination_migration_scope_test.go` (new)
- `vscode_tasks_menu_go/internal/server/agent_run_real_provider_test.go` (rewritten)
- `vscode_tasks_menu_go/internal/server/agent_run_recovery_test.go` (new)
- `01_MULTI_CONVERSATION_SHARED_CONTEXT_PLAN.md`
- `02_AGENT_PROVIDER_HOT_SWITCH_PLAN.md`
- `docs/taskdeck_ai_agent/UNIFIED_REVIEW_HANDOFF.md`

No Provider Router/Adapter implementation file was changed in this pass. Nothing was staged, committed, pushed, merged or force-pushed.
