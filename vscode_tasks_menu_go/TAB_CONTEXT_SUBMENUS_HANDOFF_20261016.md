# TaskDeck tab context menu — grouped submenus (2026-10-16)

**Branch:** main  
**Scope:** `web/featuremods/tabcontext.js`, menu regressions in `internal/server`.

## Requested UX

The tab context menu was long and hard to scan. Frequent terminal Console actions (especially Copy/Clear) should be above Session. Place Split and Close/Stop/Terminate/Force Kill into independent groups. Preserve access to existing commands and restrictions.

## Implemented

### Terminal / SSH / task session tab

Right-click (also the pane-header context trigger) now offers:

1. Read only (existing toggle; availability unchanged).
2. **Console ▶**: existing Copy, Search, Save, Clear and any other visible Console toolbar actions.
3. **Session ▶**: command palette, clone, restart, rename, process tree and other non-Split/non-Lifecycle Session toolbar actions.
4. **Split ▶**: vertical/horizontal split, merge, move group, swap, unsplit, conditioned on actual original control availability.
5. **Lifecycle ▶**: Close tab (uses the actual tab-close handler), Stop, Terminate, Force Kill. Destructive process controls retain warning color, and native confirmation/authorization handlers still run.
6. **Preset command ▶** (when available, retains its existing child actions).
7. **Broadcast group ▶** (when available): existing selectable groups and create/remove operations, with their status colors.

These are single-level popout menus, reusing existing `openCustomSubmenu` and viewport placement. `sourceAction` wraps actual existing toolbar buttons and checks `source.isConnected`, `source.disabled`, and `view.closed` immediately before clicking, not just when the menu opens. No terminal command or process action is reimplemented.

### Editor tab

Common **Save**, **Reload**, **Go to line…** remain at the top, followed by **Compare ▶**, **Split ▶**, **Project ▶** and **Lifecycle ▶** (Close). The Project group reuses shared file actions and omits separators, while editor actions activate the editor using its proper editor-specific activation path.

## Verification

- JavaScript syntax parse: PASS.
- Dynamic V8 mock for Session/Split/Lifecycle source grouping, warning colors, deferred underlying-button execution and Close tab delegation: PASS.
- Existing + new 6 focused Go UI tests statically cross-checked against current `tabcontext.js`: **71 expected source contracts / 71 found**. New tests verify ordering, permission/disabled rechecks, editor activation, and no flat menu.
- **Not performed:** repository-wide `go test ./...`, actual visual Firefox/manual pointer/keyboard acceptance. The user must run candidate self-update validation before activation.

## Acceptance checks

- Hover/Click to open each submenu; position should flip inside viewport near screen edges.
- Console Copy/Clear should execute with the same effect as the terminal header toolbar; disabled actions should not execute.
- Try Split, Swap, Unsplit in a split group; close tab preserves existing TaskDeck close semantics.
- Check separate lifecycle options on an active SSH session and verify Force Kill safety; do not accidentally terminate sessions while inspecting.
- Test editor menu (Save, Compare, Project actions, Split, Lifecycle) in normal and read-only editor.
- Check Preset and Broadcast submenus with multiple groups and a long list.

No work on Semantic Browser, Restricted Developer or Git branch logic. No GitHub Actions were triggered.
