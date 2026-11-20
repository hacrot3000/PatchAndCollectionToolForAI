# TaskDeck shared-server: stop background 403 request and alert storms
**Date:** 2026-10-16  
**Branch:** `main`

## User-reported evidence

Firefox console log from `m3-local1:10300` shows repeated `403 Forbidden` and `Error: permission denied` while the browser is idle. At least **12 distinct GET endpoints** occur:

| Endpoint | Source module | Required project permission |
|---|---|---|
| `/api/addons` | `addons.js` | `settings.read` |
| `/api/db/adapters` | `database_workbench.js`, `connections.js` | `db.read` |
| `/api/project/tree` | `filewatcher.js` | `files.read` |
| `/api/command-presets` | `commandpresets.js` | `settings.read` |
| `/api/project/editor-session` | `editor.js` | `files.read` (GET), `files.write` (PUT) |
| `/api/config/running-indicator` | `runningindicator.js` | `settings.read` |
| `/api/db/profiles` | `database.js`, `connections.js` | `db.read` |
| `/api/file-transfer/profiles` | `connections.js` | `transfer.read` |
| `/api/config/self-update` | `selfupdatesettings.js` | `settings.read` |
| `/api/config/terminal-cwds` | `terminalcwd.js`, `connections.js` | `settings.read` |
| `/api/ssh/profiles` | `connections.js` | `ssh.use` |
| `/api/config/page-title` | `pagetitle.js` | `settings.read` |

Most requests originated in unconditional page bootstrap. Two had permanent polling loops: file watcher (directory every **2.5 seconds**) and command presets (**5 seconds**). `app.showError` on startup of some modules exposed HTTP 403 as repeated user-facing alerts.

**This is separate from the terminal layout ownership problem fixed earlier.** Do not grant users these permissions merely to silence their background modules.

## Implementation

1. **File Watcher:** must have `files.read` before installing polling timers, fetching editor file metadata or watching directories. A denied response immediately stops both timers; the active directory batch stops rather than continuing through remaining watched paths. Other errors remain visible in the developer console.
2. **Command Presets:** must have `settings.read` before initial load or recurring refresh. On 401/403, mark that source blocked and clear its refresh timer; other command controls still use existing permissions.
3. **Connections:** load each of Local settings, SSH profiles, DB adapters, DB profiles and FTP/SFTP profiles only when the account has that resource's respective permission. Unexpected 401/403 blocks future calls to that exact endpoint for the current page lifetime; other permitted sections are still populated independently.
4. **DB Workbench/DB sessions/Add-ons/Editor restore:** do not auto-fetch their backends without `db.read`, `settings.read`, `files.read` or `files.write` respectively. Editor session autosaves do not attempt forbidden PUTs.
5. **Settings modules:** Page Title, Terminal CWD, Running Indicator and Self-Update Settings do not auto-load when `settings.read` is missing; write controls do not issue requests when `settings.write` is missing.
6. **UI module cache:** bump feature import/query version from `v=2` to `v=3` in `internal/server/ui.go`, including the server-injected dynamic imports in `next.js`. This causes Firefox to request the updated JS module graph even when the previous URL was cached.
7. **No backend permission was relaxed.** The server remains authoritative and returns HTTP 403 to callers without permission. Interactive attempts and unexpected actual network errors remain visible.

## Regression coverage

`internal/server/shared_background_permission_ui_test.go` checks **31 source contracts** across 11 feature modules, exact backend permission mapping for **12 GET endpoints**, and updated `v=3` module graph URLs. This complements existing module-specific tests.

### Validation actually executed in connector-only environment

- JS syntax parse: **11/11 modules PASS**.
- Static frontend guard contracts: **31/31 present**.
- Direct JavaScript simulations: **6/6 PASS**:
  - Missing DB read permission causes zero Connections requests.
  - Live 403 from a permitted Connections source stops future calls to that source.
  - Permitted Connections resource still loads normally.
  - Command Presets with no `settings.read` sends zero requests.
  - Command Presets stops refreshing after unexpected 403.
  - File Watcher with no `files.read` does not register a timer.
- **Not executed here:** Go compilation/full unit tests, two-account browser smoke and production network traces. Run before declaring the incident fully resolved.

### Recommended post-update verification

```sh
cd vscode_tasks_menu_go
go test ./internal/server -run 'TestSharedBackground'
go test ./...
```

Deploy the updated `main` revision, then **hard-refresh Firefox** (`Ctrl+Shift+R`). In Network, confirm feature module URLs end in `?v=3`. Log in with a deliberately restricted account lacking `files.read`, `settings.read`, `db.read`, `ssh.use` and `transfer.read`. Wait 30 seconds: there should be no repeating GET 403 requests to the 12 listed endpoints and no permission popups on idle page load.

Then log in with a full-permission account and confirm file watcher changes, command presets, DB adapters, Connections and settings still load and respond normally. Finally revoke a permission while the page is open and confirm only that source's automatic refresh stops (server should continue enforcing access).

Do not confuse unrelated intentional user-initiated 403 responses with background polling failures. A full permission/security audit of every future add-on remains separate from this patch.
