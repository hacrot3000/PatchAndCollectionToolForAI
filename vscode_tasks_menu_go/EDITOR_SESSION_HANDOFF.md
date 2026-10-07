# TaskDeck Editor Session Persistence Handoff

Last updated: 2026-10-07
Target branch: main

## Goal

Make file editing crash/update-safe.

Persist:
- open editor tabs;
- active tab;
- dirty/unsaved editor content;
- cursor/selection position;
- enough metadata to restore the editor after TaskDeck self-update, daemon restart, browser reload, or crash.

## Storage design

- Dirty editor text is persisted server-side beside the source file using a dedicated swap suffix: `.taskdeck-swap`.
- Swap payload is JSON and contains source path, original SHA-256, content, cursor/selection state, timestamp, and format metadata.
- Swap write is atomic (temp file + rename).
- Swap is removed after a successful Save or explicit discard/close.
- Project Explorer and project file indexes must hide `*.taskdeck-swap`.
- Git repositories should automatically receive `*.taskdeck-swap` in `.git/info/exclude`, never in tracked `.gitignore`.
- Open-tab/active-tab ordering is also persisted server-side so recovery does not depend on browser localStorage.

## Safety / conflict behavior

- Never overwrite the real file from swap recovery automatically.
- If the source SHA has changed since the swap was created, restore the dirty buffer but mark it as externally changed so the normal conflict flow remains available.
- Remote Workspace editors are out of scope for adjacent local swap files; preserve their existing behavior unless a server-side remote-safe equivalent is explicitly added.

## Implementation plan

- [x] Add editor session/swap backend types and API.
- [x] Implement atomic swap create/update/read/delete.
- [x] Ensure `.git/info/exclude` contains `*.taskdeck-swap*` when a swap is created in a Git repo.
- [x] Hide swap files from Project Explorer/file search/indexes and block direct project API access.
- [x] Persist editor tab order, active tab, dirty state, cursor/selection.
- [x] Throttle dirty-buffer swap writes to continuous ~250 ms checkpoints.
- [x] Restore tabs + dirty contents + cursor after reload/restart/update.
- [x] Remove swap after Save/discard/reload and keep conflict-safe behavior.
- [x] Add backend/frontend/shared-auth regression tests.
- [ ] Document behavior in README and perform final build-oriented import/route review.

## Progress

### 2026-10-07 — start
- User requested crash/update-safe editor restoration with adjacent swap files and preserved cursor positions.
- Handoff committed before code changes.


### 2026-10-07 — implementation checkpoint

- `1cfedbc`: added crash-safe editor-session backend with adjacent dirty swap files, per-user ownership in shared mode, server-cache metadata, atomic writes and external-source SHA detection.
- `db6d28d`, `881bb3a`: registered `/api/project/editor-session` and protected GET with `files.read`, PUT/POST with `files.write`.
- `a1bd345`, `fc419db`, `5cd38d5`, `0821df6`: hid swap files from Explorer/exact search/project file indexes and blocked direct project API access to internal swap paths.
- `65063f7`: editor now checkpoints tab order, active editor, dirty buffers and cursor/selection to the server and restores them automatically after reload/restart.
- `f701755`: persisted editor tab order is reapplied after recovery.
- `d1ba099`: self-update explicitly flushes the editor session before starting the updater.
- `bae8daa`: tab drag/reorder flushes editor order to the server.
- `06786f2`, `660f653`: hardened swap naming/temp files, POST/beacon support, total request bounds and external-change hashing bounds.
- `5b0b674`: Save, discard/close, Reload and conflict-resolution paths flush immediately; pagehide adds a small best-effort keepalive checkpoint.
- `c6c9f04`: backend regression tests cover dirty swap recovery, cursor restore, external source changes, Git exclude, hidden swap files, direct-access blocking and cache location.
- `b828e84`: shared-server route authorization regression coverage.
- `9904114`: frontend tests cover automatic recovery, remote-editor exclusion, self-update flush and drag-order persistence.

## Resume checkpoint

Remaining before declaring complete:
1. document the feature and recovery semantics in README;
2. inspect all touched Go imports/routes for build-time issues;
3. review any stale-swap edge cases around project rename/delete and either fix them safely or document the behavior;
4. update this handoff to complete with final HEAD.
