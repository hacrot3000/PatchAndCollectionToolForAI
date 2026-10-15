# TaskDeck handoff — Transfer Queue scan cleanup & AI Pack rules

Date: 2026-10-15 · Branch: `main`
Scope: Three user-reported improvements after the project identity migration.

## 1. Finished Transfer Queue scan rows

Root cause: `web/featuremods/filetransfer.js` constructs synthetic rows from server jobs using `visibleServerScanJobs`. Old conditions included `scan_done=true` with zero file entries, or any `job.error`, even after scanning ended. Those pseudo-items have `serverID:''` and cannot be removed with the regular transfer-item action, leaving permanent "Upload scan" and "Download scan" rows.

Solution: Only expose statuses `scan_queued` or `scanning` in the pseudo-row projection. Completion, cancellation, or failure causes the scan row to disappear on the next daemon poll; ordinary transfer items and daemon-side job/error history are untouched. Regression test in `internal/server/file_transfer_dual_pane_ui_test.go`.

Commit: `9669e2a`.

## 2. AI Pack first-open invalid-result bug

Root cause: `internal/patchtool/ai_pack.go` populated `AIPackResult.ProjectIdentity` on **cache hit** only. On a **cache miss** (initial ZIP build), the result omitted `project_identity`. `web/featuremods/patchpanel.js:renderAIPack` correctly rejects the missing UUID, causing `TaskDeck returned an invalid AI Pack result`.

Solution: Populate `ProjectIdentity` on both response paths. Keep UUID validation on the browser. The existing initial-build and cached response tests assert stable nonempty IDs.

Commit: `74a6d89`.

## 3. Per-project AI rules

- Persistent config: `<project-root>/.python_patch_tool_ai_rules.json`, independent of generated cache under artifacts. JSON stores `version:1`, current `project_identity`, and `rules`.
- New Go helpers in `internal/patchtool/ai_pack_rules.go`: load/save UTF-8 text (max 32 KiB), reject symlink/unreadable/invalid/mismatched identity, write replacement via existing atomic helper. Rules are bound to the current workspace UUID and cannot be silently transplanted to another project.
- New `GET/PUT /api/patch/project-rules` in `internal/server/patch_ai_pack.go`, registered in server routes and shared-server authorization. `GET` requires `patch.view`, `PUT` requires `patch.run` (single-user unchanged).
- `BuildAIPack` adds `project_rules` response, includes custom rules in Copy Prompt and (when nonempty) `PROJECT_RULES.md` inside AI Pack ZIP. Rules participate in cache fingerprint so changes invalidate stale ZIP only for that workspace. Six core contract docs remain the same.
- UI in `web/featuremods/patchpanel.js`: textarea with PHP 5.5 example, Save project rules button and saved/unsaved indicator. Copy Prompt automatically saves any unsaved edit and prepares the fresh identity-bound prompt before copying. Refresh also preserves unsaved edits by saving before rebuilding.
- New regression tests in `internal/patchtool/ai_pack_test.go`, `internal/server/patch_ai_pack_test.go`, `internal/server/shared_authorization_test.go`, `internal/server/patchpanel_ui_test.go`.
- Never place secrets/tokens in project rules: they are included in prompt/ZIP sent to AI. Project rules cannot override UUID/contract enforcement.

## Verification
- Latest JS `filetransfer.js` and `patchpanel.js`: syntax parse via JavaScript `new Function` PASS.
- Dynamic `visibleServerScanJobs` test: `scan_queued` and `scanning` produce two visible rows, while `done`, `failed`, and completed-with-error produce none: PASS.
- Go regression code is committed but **not executed** in the connected remote repository. No GitHub Actions runs were triggered.

### Suggested local checks
```bash
cd vscode_tasks_menu_go
go test ./internal/patchtool ./internal/server
node --check web/featuremods/filetransfer.js
node --check web/featuremods/patchpanel.js
```

Functional smoke: open existing project AI Pack with deleted cache; ensure no invalid result on first open; enter `PHP 5.5 only`, Save, copy prompt, inspect ZIP `PROJECT_RULES.md`, reload and confirm persistence; open a second project with same directory name and verify rules do not migrate; upload/download folders with an empty scan and with errors, confirm scan disappears while genuine jobs remain.

## Follow-up
If any runtime/compile regression is discovered, record it here, fix the smallest scope in a new commit on `main`, and rerun impacted tests. Do not substitute stale handoffs from other project UUIDs.
