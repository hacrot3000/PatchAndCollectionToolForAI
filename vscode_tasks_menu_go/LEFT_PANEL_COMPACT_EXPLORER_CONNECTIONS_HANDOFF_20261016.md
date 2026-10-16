# TaskDeck — Compact Explorer + Collapsible Connections (2026-10-16)

**Branch:** `main`  
**Scope:** Only the left sidebar's Explorer / Connections panels and regression tests.

## UX changes

### EXPLORER

- The former `+R`, `+F`, `+D`, Undo, `Build files`, and Refresh buttons were removed from the **header row** and consolidated into a right-hand **⋯ Explorer actions** dropdown. The Close button remains visible.
- Actions, in order: Attach workspace root, New file, New folder, Undo last operation (disabled when unavailable), Show/Hide build files (label tracks current preference), Refresh Explorer.
- Menu delegates to the **original existing button handlers**, retaining permission/error/undo logic and built-file preference behavior. No new file mutation API was introduced.
- Title is shrinkable with ellipsis, header clips overflow and keeps only two buttons. Reuses the existing Explorer context popup with `taskSidebarKeepOpen`, shared viewport placement and keyboard access (`aria-haspopup`, `aria-expanded`, focus first action, Escape closes popup first and returns focus).

### CONNECTIONS

- Independent fold/unfold controls for `LOCAL`, `SSH`, `SFTP`, `FTP` and `DATABASES`; each heading has a chevron and `aria-expanded`.
- Existing Add (`＋`) buttons remain next to the heading and work even when the section is folded.
- Each section's rows/empty status are wrapped in a distinct content div with `aria-controls`, hidden when folded. No change to connection API, tests, profile mutation or session launch logic.
- The collapsed-section set is stored in localStorage key `taskdeck:connections:collapsed:v1:<workspace>`. It is restored on render, survives refreshes and is separate across workspaces. Malformed storage is ignored; stored section names are limited to five recognized types.

## Commits

- `c09ffb2` — Compact Explorer actions menu.
- `b05ce8d` — Collapsible Connections with per-workspace persistence.
- `996a3f4`, `a5d913c` — Go UI regression tests.
- `150689b` — Explorer accessibility/focus, Escape dismissal.

## Validation executed

- JavaScript syntax parse of both updated frontend modules: PASS.
- Existing new Go UI assertion contracts checked statically against current sources: **38/38 PASS**.
- Dynamic JS simulation of Explorer popup and underlying actions: **8/8 PASS**, including disabled Undo, menu focus, viewport positioning, dispatch and dismiss.
- Dynamic JS simulation of Connections state: **5/5 PASS**, including independent folding, refresh restoration, workspace separation and expansion.
- **NOT EXECUTED HERE:** full `go test ./...` or real Firefox visual resizing/interaction tests. No CI run was started manually.

## Manual acceptance

1. Shrink the left panel to minimum width; header must display `EXPLORER`, `⋯`, `×` without horizontal protrusion.
2. Click `⋯` near screen edges, execute each original action, confirm Undo disabled until a compatible undo record exists, and Build files label matches its state.
3. Escape should dismiss popup without closing Explorer; the second Escape closes Explorer.
4. Collapse SSH and FTP independently, refresh, switch tabs and reopen Connections; their states should remain. Switch workspace: different collapse state.
5. Create a profile via `＋` with a folded group, verify original dialog and reloading continue to work.

No changes to semantic browser, restricted developer or permission policies.
