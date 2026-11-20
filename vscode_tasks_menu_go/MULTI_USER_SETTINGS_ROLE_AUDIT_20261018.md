# Multi-user Settings / Schedule RBAC audit — 2026-10-18

## Scope and diagnosis

1. The Schedule family (Cron/URL/Git/File/Webhook/AND-OR, background task execution,
   log output, workflow graph, import/export, retries and notifications) previously
   used a single `project.admin` gate and was not represented by Schedule-specific
   entries in the Role permission catalog.
2. The grouped Settings menu appended global configuration controls even for
   roles that had only `settings.read`. Some modules had already inserted
   launchers in the legacy header, so *omitting* the launcher from the menu
   would still leave a visible control at its original DOM location.
3. Portable config backup GET, update branch GET and auth migration GET were
   readable with `settings.read`, exposing global configuration metadata.
4. Command palette shortcuts offered sensitive configuration entry points based
   on `settings.read` or `files.write` rather than `settings.write`.

## Role permission model

New permissions are registered in the same **Role / Edit permissions** list:

| Permission | Read definitions / logs | Manual run / stop / backfill | Create / edit / pause / import / delete |
| --- | --- | --- | --- |
| `schedules.view` | Yes (including Inspector/Simulate and history) | No | No |
| `schedules.run` | Yes | Yes (including authenticated webhook signal) | No |
| `schedules.manage` | Yes | No separate manual run grant | Yes |
| `project.admin` | Yes | Yes | Yes |

The Role UI receives new catalog values through the existing
`/api/admin/permissions` endpoint. A transactional **V5 role seed** inserts
these permission keys in existing SQLite identity databases and grants them
to `system:admin` only. It does not grant them to system developer, operator,
viewer, or existing custom roles. Existing per-user DENY overrides are not
cleared. Re-running the seed does not restore subsequently revoked grants.

**Security:** granting `schedules.manage` allows creating unattended commands
that execute under TaskDeck's server account. Grant it only to a trusted role.
A user with `settings.write` or `tasks.run` alone does **not** automatically
receive any Schedule permission.

All Schedule API actions are checked **server-side**, not only by UI:
- GET schedules, run console and discovered repos: Schedule view (or stronger).
- Inspect, simulate, preview backfill: Schedule view (or stronger).
- Run, stop, backfill and authenticated webhook signal: Schedule run.
- Upsert, delete, import, pause and resume: Schedule manage.
- The existing Project Admin grant remains a full fallback.
- Unauthenticated shared-server requests remain denied at the central router.

The Schedule Activity Bar entry is hidden without any Schedule permission.
View-only users see the full history, condition inspector, dependency graph and
details in read-only mode. Run/manage actions in the tables and editor are
rendered only for the corresponding grants.

## Global Settings

`settings.read` **is not** permission to edit or display workspace-global
configuration controls. The grouped Settings menu now separates:

- Personal UI preferences, appearance, notifications, help and reload:
  available without `settings.write` as appropriate.
- Update source/branch, full validation setting, portable config backup,
  terminal working-directory config, environment/project settings, running-tab
  indicator settings, edit page title, tasks.json editor and AI provider
  configuration: require `settings.write` (or Project Admin UI access).
- Authentication mode migration: **Project Admin only**.
- Check updates: still requires explicit `selfupdate.check`.
  Installing updates still requires `selfupdate.run`.

The original legacy-header launchers are also hidden when a user cannot edit
workspace-global configuration. The command palette filters privileged
configuration commands rather than merely showing disabled options.

Backend GET permissions hardened:
- `GET /api/config/self-update`: `settings.write`
- `GET /api/config-backup`: `settings.write`
- `GET /api/config/auth-mode`: `project.admin`
- Write endpoints retain their previous authorization requirements.

## Verification

Regression coverage:
- `internal/identity/seed_roles_test.go`: V5 permission seed, opt-in defaults,
  repeat-seed preservation.
- `internal/server/schedule_permissions_test.go`: action permissions, least
  privilege matrix and Project Admin fallback.
- `internal/server/shared_authorization_test.go`: updated route policies.
- `internal/server/menus_test.go`,
  `internal/server/selfupdate_branch_ui_test.go`,
  `internal/server/shared_background_permission_ui_test.go`,
  `internal/server/config_backup_ui_test.go`,
  `internal/server/auth_mode_ui_test.go`: menu/route contracts.
- `tests/settings_role_visibility.test.mjs`: viewer, config editor,
  Project Admin and single-user menu visibility, including legacy controls.

During connector-only edits, syntax/unused-import checks passed on **20/20**
affected frontend/Go source and existing tests, and the four primary Settings
menu permission scenarios passed direct JavaScript checks.
**Full Go compilation, unit tests and real Firefox multi-user smoke are still
required** on a repository checkout.

```bash
cd vscode_tasks_menu_go
go build ./...
go test ./internal/identity -run 'TestPermissionSeedV5' -count=1
go test ./internal/server -run 'TestSchedulePermission|TestSharedRoutePermissions|TestGroupedMenus|TestSelfUpdateBranch|TestSharedBackground' -count=1
node --test tests/settings_role_visibility.test.mjs
```

Suggested manual matrix: Admin, settings.read-only user, settings.write editor,
schedules.view-only user, schedules.run user, schedules.manage user,
and user with no Schedule permissions. Attempt direct HTTP API calls
as each account, not just UI clicks. Test updates and Schedule access in
two simultaneous browser sessions to catch stale permission presentation.

## Final hardening and user acceptance notes

Legacy modules can inject Settings launchers into the header *before* grouped
menus are installed. Restricted roles now hide these controls immediately
in the feature modules (`configbackup.js`, `terminalcwd.js`,
`runningindicator.js`, `pagetitle.js`). The menu's `only()` helper
also hides denied controls at their original DOM location and a targeted
`[hidden]{display:none!important}` rule prevents legacy header CSS from
overriding the restriction.

Command Palette entries for global Project Profiles, Workspace Snapshots,
AI configuration, Backup/Restore and the tasks.json editor now require
`settings.write` and are **omitted** rather than shown disabled to limited
shared-server accounts.

`internal/server/shared_security_review_test.go` includes anonymous
GET/POST Schedule requests, confirming that the special action-based RBAC
route must still reject unauthenticated requests.
`internal/server/schedule_ui_test.go` checks read-only Schedule UI hooks.

Focused JS syntax/Go unused-import checks: **18/18 PASS** on the selected
affected modules/tests. Settings menu's four role scenarios passed direct
JavaScript checks. Full repository Go unit tests, Linux build and in-browser
multisession RBAC tests remain outstanding.
