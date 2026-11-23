# TASKDECK AI Agent — Unified Review Handoff

Review date: 2026-11-23

## 1. Baseline and Git state

- Repository: `hacrot3000/PatchAndCollectionToolForAI`
- Branch: `review/taskdeck-multiconversation-hotswitch-20261123`
- Required baseline / starting HEAD: `2fdf4dd15a9a436a7fc6388df5e400c2369c3a40` (exact match)
- Initial worktree: clean; no staged changes.
- `git fetch origin` completed. Merge base with `origin/main`: `07fd54a534db573d2a89ef892c4283bc52aa612f`.
- Divergence after fetch: `origin/main` is 59 commits ahead; review is 2 commits ahead (`c10eb6e4`, `2fdf4dd1`).
- No merge, commit, push, reset, or branch switch was performed. The current review changes remain unstaged.

## 2. Integration status

**NOT READY FOR MERGE.** The review branch is substantially behind `origin/main`, and nine Agent Chat files have edits on both sides. The full server test suite also has 24 baseline failures. The feature-specific tests pass, but there is no provider configuration for a real provider smoke test and no browser E2E run in this environment.

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

### Validation performed after the review fixes

- PASS: `go test ./internal/agentchat ./internal/agentrun`
- PASS: `go test -race ./internal/agentchat ./internal/agentrun`
- PASS: focused server integration selection covering 3 conversations, #101/#102 overlap/retry/stop, real native Patch, group snapshot membership, shared route permissions and coordination membership.
- PASS: the same focused server selection under `-race`.
- PASS: `node --test web/*_test.cjs` (9 reported tests, 0 failures).
- PASS: `go build -o /tmp/taskdeck_review_final ./cmd/vscode_tasks_menu`; the temporary binary was removed.
- PASS: `git diff --check` and `git check-ignore` for root and nested `__pycache__` paths.
- BLOCKED: real-provider smoke. The workspace-specific provider store/config was absent; no credential was read or sent.

## 4. Findings by priority

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

- PLAN 01: 1/139 checklist boxes checked: `MC-05.01.02` (fork boundary), with focused/race test evidence. The other 138 remain unchecked because the broader acceptance/UI/recovery requirements are not all proven.
- PLAN 02: 0/126 checked. Provider payload continuation tests pass, but a real provider endpoint, browser E2E and the end-to-end switch/reload acceptance were not available.
- Parent and milestone boxes remain unchecked until every child and acceptance gate is satisfied. The two source plans remain the authoritative requirements.

## 6. Main-branch comparison and conflict plan

The 59 main-only commits include durable provider run/egress recovery, provider heartbeat/review, history/Patch archives, context compaction, task-plan segmentation/handoff and related collection/retry changes. The following nine files are changed on both branches:

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

No main changes were integrated. The owner’s explicit authorization is still required before any merge/rebase into this branch. Recommended next step after authorization: create a protected integration worktree from current `origin/main`, transplant/merge review changes by the conflict table, run all existing main durability tests plus the focused Agent suites, then request browser/provider smoke credentials and acceptance.

## 7. Files changed in this review pass

- `.gitignore`
- `01_MULTI_CONVERSATION_SHARED_CONTEXT_PLAN.md`
- `docs/taskdeck_ai_agent/UNIFIED_REVIEW_HANDOFF.md`
- `vscode_tasks_menu_go/internal/server/shared_authorization.go`
- `vscode_tasks_menu_go/internal/server/agent_conversation_api.go`
- `vscode_tasks_menu_go/internal/server/agent_conversations_api.go`
- `vscode_tasks_menu_go/internal/server/agent_coordination_api.go`
- `vscode_tasks_menu_go/internal/server/agent_context_group_api.go`
- `vscode_tasks_menu_go/internal/server/agent_context_snapshot_api.go`
- `vscode_tasks_menu_go/web/featuremods/agentchat_coordination.js`
- Tests: `agent_coordination_permissions_test.go`, `agent_conversation_group_membership_test.go`, `agent_context_snapshot_group_test.go`, plus the coordination API test update.

No Provider Router/Adapter implementation file was changed in this review pass.
