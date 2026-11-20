# Multi-user terminal ownership and layout isolation — 2026-10-16

**Branch:** `main`  
**Scope:** TaskDeck shared-server authentication, browser terminal restore, session layout persistence and UI scoping. No terminal commands were changed.

## Observed failure / source-backed diagnosis

TaskDeck already had project/owner filtering for `GET /api/sessions`, item APIs, and WebSocket terminals (`shared_sessions.go`, `server.go`). However:

1. `shared_authorization.go` explicitly returned **nil** for the `/api/state/tasks?scope=terminals` route. The generic API middleware interpreted that as an unmapped route and responded **403 permission denied**.
2. Browser `terminalrestore.js` nevertheless fetched/PUT and attempted POST restore to that endpoint. The terminal snapshots still used global `vscode_tasks_menu.terminals*.json` files inside the project, shared by all accounts.
3. Single-user restore uses snapshots without ownership metadata and can recreate sessions from legacy project files. That mechanism is unsafe in shared-server mode.
4. Browser `split.js`, `tabdrag.js` and the fresh-terminal flag in `ui.go` keyed browser state by workspace but not by authenticated account.

The first three are confirmed code flaws. They explain denied terminal layout requests and cross-account state interference. Exact frequency of the user's observed popups cannot be confirmed without the failing browser's network log.

## Backend changes

- The shared-server middleware recognizes *only* terminal layout scope as an authenticated special route; normal unknown API routes still deny access. Ownership and permission checks are performed by the dedicated handler.
- `shared_terminal_state.go`: per-user/project/workspace state keys derived from SHA-256 of authenticated identities; saved outside browsable workspace under the TaskDeck service account's OS user config directory (`taskdeck-private-terminal-layouts/`). Directory created with 0700, snapshot temp/final mode 0600, size bounded, only regular files permitted.
- GET requires a terminal view permission or `sessions.manage`. PUT requires a control permission or `sessions.manage`; each session ID is verified server-side as an owned terminal in the same project before capturing state. Split groups referencing other users' IDs are dropped.
- POST restores a previously saved account layout only if it contains terminal records and the current principal has `terminal.create`. Existing running terminals belonging to the requesting account are protected against duplicate restoration. Newly started local terminals are stamped with the authenticated `OwnerUserID` and `ProjectID`.
- `terminalStateItem.TargetType` records the source (`local` or `ssh`). **SSH or unknown/legacy targets are not recreated as host shells**. Only local saved directories still within the configured project root are eligible; Split groups are remapped only when both sides were successfully restored.
- During shared-server self-update, the server no longer reads or restores project-global **ownerless** terminal layouts (which remain supported in single-user mode). Live broker sessions remain preserved as before.

## Browser changes

- `terminalrestore.js` scopes saved IDs and split state to terminals owned by `app.currentUser.user_id` in shared mode, never all visible project sessions; skips GET without terminal-view permission, skips repeated PUT without control permission, and skips POST when terminal creation is forbidden.
- `split.js` and `tabdrag.js` append the authenticated user ID to the browser sessionStorage keys in shared mode. Single-user keys remain backwards-compatible.
- `ui.go` scopes the browser localStorage flag for fresh, unused terminal cleanup by authenticated account.

## Automated coverage added (not executed in this connector-only run)

- `shared_terminal_state_test.go`: two users in same project, same user in different project, persistence isolation, owner-check failures, read-only users, private file mode, legacy project snapshot exclusion and explicit local-vs-SSH restore.
- `terminalrestore_ui_test.go`, `shared_terminal_browser_ui_test.go`: client-side ownership guards, no forbidden background state writes, browser Split/tab order scoping.
- Existing `shared_sessions_test.go` already verifies filtering and authorization for sessions belonging to different users/projects.

### Validation actually executed

- Frontend JavaScript syntax parse and 13 terminal restore protection assertions: PASS.
- Static source integration checks for route exceptions, owner/project enforcement, safe restore stamping and target-origin checks: PASS.
- **Not executed:** full Go compilation/tests, live multi-user browser smoke, broker/WebSocket integration. Do not interpret source-level checks as proof of a green update.

Run in checkout:
```sh
cd vscode_tasks_menu_go
go test ./internal/server -run 'TestSharedTerminal|TestTerminalRestore'
go test ./...
```

## Deployment cautions

- This is **application-level session isolation**, not per-user Unix UID/process/container isolation. All shell processes still inherit the configured TaskDeck service account's host filesystem privileges unless an independent OS sandbox is deployed.
- Existing legacy project-global `vscode_tasks_menu.terminals*.json` files are **not migrated or automatically deleted**, since they lack verified owner identities. Shared-server mode no longer uses them for account-specific state.
- If a broker is empty after restart, own saved *local* terminals may be recreated after login. SSH terminals must be reopened explicitly.
- Test with two simultaneous accounts, and verify Network requests no longer repeat HTTP 403 on terminal layout GET/PUT. The acceptance checklist is in `SHARED_SERVER_ACCEPTANCE.md`.
