# Restricted Developer / Multi-user release gate

**Branch:** `feature/restricted-developer-security`  
**Status:** NOT production-ready. Keep `/api/restricted/status.release_ready=false`.  
**Do not merge this branch to `main` without an explicit release review.**

## Security boundary

- A Restricted Developer role has immutable grants, does not permit detailed member overrides, and must always use the protected source/command gateway. Denied legacy paths must remain **HTTP 403**.
- Schedule execution currently runs with **TaskDeck process credentials**, not the role's OS identity. For shared-server requests, `schedules.view` exposes only an allowlisted summary (ID, name, type, enabled, paused); it cannot return commands, URLs, trigger expressions, webhook secrets, run output/logs or diagnostics. All inspect/log/repository operations are privileged.
- **Both** `schedules.manage` **and** `schedules.run` are required for configuration changes and for manual execution/stop/backfill/webhook emission in Multi-user. A role with only one of them cannot use Schedule mutations, because **no per-schedule ownership ACL exists**. `project.admin` and local single-user preserve their existing behavior. Only grant the pair to highly trusted operators.
- This intentionally narrows formerly permitted Schedule Role configurations on `main`. Review current Role assignments after upgrade and do not automatically add the missing second grant. A custom Role with only `schedules.view` receives a redacted list rather than a complete editable definition.

## Historical database migration verification

Two incompatible historical states used `identity_role_seeds.version=5`:
1. `main` V5: Schedule keys already registered, possibly revoked by an admin; no Restricted Developer role.
2. Restricted branch V5: Restricted Developer role already registered, possibly with old member overrides; Schedule keys absent.

V6 only seeds Schedule grants on **previously unregistered keys**. This prevents accidentally restoring permissions deliberately removed by an administrator of a `main` V5 DB. V7 ensures the immutable Restricted Role exists and deletes obsolete member overrides. The new real-SQLite test builds both historical states, upgrades them through V6/V7, verifies revoked grants remain revoked, and verifies re-running the seeder cannot widen them.

**Important:** Tests that synthesize both legacy states are not a substitute for an offline upgrade on a copy of each real historical V5 database. Stop TaskDeck, safely back up `identity.db` **with its SQLite WAL/checkpoint**, run the new binary on copies, inspect `identity_role_seeds`, `role_permissions`, `project_members` and `member_permissions`, and verify no withdrawn grant or stale override reappears. Never experiment on live production data.

## Required automated verification

The existing GitHub Actions workflow `VSCode Tasks Menu Go` is **manual-only** (`workflow_dispatch`). It includes the new migration, Schedule HTTP/RBAC, legacy restricted-route, and Role UI checks. Trigger it manually for the **exact candidate HEAD**, and save a successful run URL. A success from a previous commit does not count.

From a checkout of `feature/restricted-developer-security`:

```sh
cd vscode_tasks_menu_go
go test ./internal/identity ./internal/server -count=1
go test ./internal/restrictedpolicy ./internal/restrictedcommand ./internal/restrictedview ./internal/restrictededit -count=1
go test ./...
go vet ./...
go build ./...
node --test test_restricted_ui.cjs
node --test tests/settings_role_visibility.test.mjs
```

Capture the exact `git rev-parse HEAD`, timestamps, Go/Node versions and any failures in the release evidence. An unsuccessful or unexecuted test is **not** a PASS.

## Mandatory interactive Multi-user Firefox smoke

Using a real Firefox and a running HTTPS-enabled Shared Server, provision at least **three separate accounts**: Admin, a Restricted Developer, and an ordinary Role with `settings.read` but no `settings.write`. Keep each account in a separate browser profile/session so cookies do not mix.

| Scenario | Required outcome |
| --- | --- |
| Admin opens Settings, Role management | Allowed |
| Read-only settings user attempts update repository change, including direct HTTP request | HTTP 403; no configuration change |
| Restricted member calls Terminal, SSH, SFTP, legacy Git, Schedule endpoints | HTTP 403 |
| Restricted user tries to edit self permissions | HTTP 403 |
| Admin tries to edit Restricted Role permissions | HTTP 409 |
| Restricted attempts out-of-range file read or unviewed edit | Denied; file unchanged |
| Two distinct restricted users charge different line quotas for the same file | Counters independent |
| Restricted quota after daemon restart and new browser session | Existing usage retained |
| Two restricted users edit the same file from stale SHA | One safe success; one conflict; no lost update |
| Developer's logged-in cookie used after Admin changes Role to Restricted | New API requests immediately lose old permissions |
| Admin switches Restricted user back to Developer | No previously removed member ALLOW override reappears |
| A Role has only `schedules.view` | Safe summary; no task command, trigger URL, output or logs |
| A Role has only `schedules.manage` or only `schedules.run` | HTTP 403 for execution and mutations |
| Role explicitly has both elevated Schedule grants | Actions authorized; must still comply with input validation |

Verify responses in Firefox Network and direct authenticated HTTP requests, not just whether controls are hidden. Check user session isolation, response headers, cross-user quota persistence, and application audit logs.

## OS isolation is a deployment requirement, not a Role feature

**Do not mistake per-user API restrictions for a process sandbox.** A single shared TaskDeck process cannot safely execute privileged tasks against source writable by restricted users merely by changing their Role. Code modified by a restricted user could later run inside a privileged Schedule, build job, installer or deployment workflow.

For production-like evaluation:

1. Use a **dedicated OS account**, isolated daemon, separate workspace/checkout and restricted config/data directory. An example Linux service template is in `deploy/taskdeck-restricted.service.example`; audit every permission, TLS setting and systemd directive before using it.
2. Never mount admin SSH keys, Git credentials, CI/CD tokens, production secrets, privileged Docker sockets, deploy scripts or read/write access to privileged sibling workspaces.
3. Do not let trusted jobs, Scheduled commands, hooks or CI read and execute an unreviewed restricted-writable checkout. Use reviewed commits, separate protected CI checkout/runner and explicit human approval before privileged deployment.
4. Apply operating-system filesystem permissions, minimal writable paths, process/network restrictions and outbound firewall policy; verify with an unprivileged test account. A single mixed-trust shared daemon does **not** provide a per-user OS boundary.
5. Maintain `release_ready=false` until OS isolation and the live multi-account tests are demonstrated. Passing unit tests alone is not sufficient.

## Merge decision

Block merge if any migration test, Go/Node regression, live Firefox scenario, filesystem/OS isolation check or release-candidate CI run is missing. Do not merge to `main` on the basis of this document alone.
