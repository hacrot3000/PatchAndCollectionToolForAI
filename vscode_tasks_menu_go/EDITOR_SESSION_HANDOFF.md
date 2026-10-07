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

- [ ] Add editor session/swap backend types and API.
- [ ] Implement atomic swap create/update/read/delete.
- [ ] Ensure `.git/info/exclude` contains `*.taskdeck-swap` when a swap is created in a Git repo.
- [ ] Hide swap files from Project Explorer/file search/indexes.
- [ ] Persist editor tab order, active tab, dirty state, cursor/selection.
- [ ] Debounce dirty-buffer swap writes.
- [ ] Restore tabs + dirty contents + cursor after reload/restart/update.
- [ ] Remove swap after Save/discard and keep conflict-safe behavior.
- [ ] Add tests.
- [ ] Document behavior.

## Progress

### 2026-10-07 — start
- User requested crash/update-safe editor restoration with adjacent swap files and preserved cursor positions.
- Handoff committed before code changes.
