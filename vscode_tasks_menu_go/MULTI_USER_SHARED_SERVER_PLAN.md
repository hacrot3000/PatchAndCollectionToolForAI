# TaskDeck Multi-user / Shared Server Plan

Status: design baseline  
Branch: `feat/multi-user-shared-server`

## 1. Purpose

Extend TaskDeck from the current primarily single-user/local model into an optional multi-user shared-project server while preserving the existing local workflow unchanged by default.

The design must support:

- Multiple users accessing the same TaskDeck project/workspace.
- Per-user authorization for modules and sensitive actions.
- Safer credential/session storage so users cannot freely inspect login secrets.
- Installation on a server where multiple users collaborate on the same project.
- A future path where one user can belong to multiple TaskDeck projects and those projects can be administered centrally.
- Backward compatibility: existing TaskDeck behavior remains the default unless shared-server mode is explicitly enabled.

## 2. Core architecture decisions

### 2.1 One daemon = one workspace/project

A TaskDeck process continues to own exactly one workspace.

If a host serves multiple projects, run multiple TaskDeck processes on different ports:

```text
taskdeck process A -> workspace A -> port 42881
taskdeck process B -> workspace B -> port 42882
taskdeck process C -> workspace C -> port 42883
```

This preserves the current workspace-oriented security boundary and avoids introducing cross-project filesystem routing into one daemon.

### 2.2 Shared identity, separate project daemons

Although each daemon serves one workspace, shared-server mode must use an identity database whose schema already supports many projects:

```text
               shared identity database
                 /        |        \
                /         |         \
         project A    project B    project C
         daemon       daemon       daemon
```

A user is global to the identity store. Access is granted through project membership, not by duplicating the same user account inside each project.

This lets a future management service or "TaskDeck Hub" administer users, projects, roles, sessions and audit data without changing the membership model.

### 2.3 Shared-server mode is opt-in

The existing INI remains the source of per-project server configuration.

Proposed section:

```ini
[shared_server]
enabled = false

# Stable project identity used by the common identity database.
# Required when enabled=true.
# project_id = m3-client

# Optional absolute path. If omitted, use the platform TaskDeck data directory.
# Multiple TaskDeck processes on the same host may point to the same DB.
# identity_db = /var/lib/taskdeck/identity.db
```

Default:

```ini
enabled = false
```

No identity database is required or initialized merely by running TaskDeck in the default mode.

## 2.4 Dependency policy

Implementation should prefer the Go standard library and code already present in this repository.

Rules:

- Do not add a third-party library when a reasonable standard-library or existing-code solution exists.
- Any new third-party dependency must be isolated, justified by a capability unavailable in the standard library, and kept to the smallest practical surface.
- Do not add npm packages when any viable non-npm implementation exists.
- Shared-server work must preserve offline/self-update builds; a runtime or build path must not silently start depending on network package installation.
- SQLite is a special case: Go's standard library provides `database/sql` but no SQLite driver. Build the SQL/migration layer against stdlib first, and isolate the eventual concrete SQLite driver decision as a separate checkpoint.

## 3. Compatibility modes

TaskDeck has two authentication modes determined by `[shared_server].enabled`.

### 3.1 Local / legacy mode: shared_server.enabled=false

Behavior stays exactly as today.

The existing section remains authoritative:

```ini
[auth]
enabled = false
username = admin
password = change-me
```

Rules:

- Existing Basic Auth behavior remains unchanged.
- Existing remote-bind validation remains unchanged.
- No shared identity DB lookup is performed.
- No user/project membership model is required.
- Existing installations do not need migration merely to keep working.
- Local users are not forced into a login/session workflow that did not previously exist.

### 3.2 Shared server mode: shared_server.enabled=true

Authentication and authorization come from the shared identity database.

Rules:

- `[auth].username` and `[auth].password` are not the shared-mode login database.
- Shared mode requires authenticated users.
- Every request is associated with a server-side principal/current user.
- Every project daemon identifies itself with a stable `project_id`.
- Authorization is evaluated against that user's membership in the current project.
- Remote use must continue to be protected by TLS.
- Shared-server configuration must satisfy listener security validation independently of legacy `AuthEnabled`.

The implementation must not silently fall back to the legacy Basic Auth account when shared mode is enabled but the DB is unavailable or invalid. Shared mode fails closed.

## 4. Identity database

### 4.1 Storage

Initial implementation: SQLite.

Recommended platform location for a shared host:

```text
/var/lib/taskdeck/identity.db
```

or a user-scoped platform data directory for non-system installs.

Requirements:

- DB directory should be restricted to the TaskDeck service account.
- POSIX target permissions:
  - directory: `0700`
  - DB/config secrets: `0600`
- Do not store the DB inside a project workspace/repository.
- Enable SQLite WAL and a bounded busy timeout so multiple TaskDeck project processes on the same host can share the DB safely.
- All schema access goes through an identity-store abstraction so a later central service/PostgreSQL backend can be introduced without rewriting HTTP authorization logic.

### 4.2 Stable IDs

Do not use workspace filesystem paths as primary project identity.

Use stable IDs, e.g.:

```text
project_id = m3-client
project_id = bletonfc
project_id = datbike-ota
```

Internally, database primary keys may use UUID/opaque IDs. A human-readable unique project key may also be stored.

The important rule is:

```text
membership = user + project
```

not:

```text
membership = user + process port
membership = user + workspace path
```

Ports and workspace paths can change without invalidating authorization records.

## 5. Proposed data model

The first schema should already support multi-project membership even though each running daemon is single-project.

### users

```text
id
username
display_name
password_hash
enabled
created_at
updated_at
last_login_at
password_changed_at
```

Constraints:

- username unique.
- password plaintext is never stored.
- password hash is never returned by normal user-management APIs.

### projects

```text
id
project_key
display_name
enabled
created_at
updated_at
```

The running daemon resolves `[shared_server].project_id` to one project row.

A later management layer may additionally store non-security metadata such as known URL/host/port, but a daemon does not trust a URL/port as the project identity.

### roles

```text
id
name
description
system_role
```

Roles are templates/bundles of permissions.

Suggested initial roles:

- admin
- developer
- operator
- viewer

Authorization code must not hard-code behavior based only on those role names.

### permissions

```text
id
permission_key
description
```

Permission keys are explicit capabilities.

### role_permissions

```text
role_id
permission_id
```

### project_members

```text
project_id
user_id
role_id
enabled
created_at
updated_at
```

This is the central relation that makes multi-project users possible.

### member_permissions

Optional per-member overrides:

```text
project_id
user_id
permission_id
effect  # ALLOW / DENY
```

Evaluation rule:

1. User must be enabled.
2. Project must be enabled.
3. Membership must exist and be enabled.
4. Role permissions establish defaults.
5. Explicit member DENY wins over role ALLOW.
6. Explicit member ALLOW can add a permission not present in the role.
7. Default result is DENY.

### auth_sessions

```text
id
user_id
token_hash
created_at
expires_at
last_seen_at
revoked_at
client_metadata
```

Only a hash of the browser session token is stored.

### audit_log

```text
id
timestamp
user_id
project_id
action
resource_type
resource_id
result
client_ip
details
```

Never place password, raw session token or secret values in audit details.

## 6. Authentication

### 6.1 Password storage

Use Argon2id password hashing.

Never provide:

- "view password"
- reversible password encryption
- password hash export through normal APIs

Admin operations are:

- reset password
- disable account
- force logout/revoke sessions
- require password change later if that feature is added

### 6.2 Browser session

After successful login, server creates a cryptographically random opaque session token.

Browser receives an HttpOnly cookie.

Target properties when HTTPS:

```text
Secure
HttpOnly
SameSite=Strict
Path=/
```

The identity DB stores only a token hash.

Logout or force-logout revokes the server-side session.

### 6.3 Request principal

Authentication middleware resolves every shared-mode request to a principal such as:

```go
type Principal struct {
    UserID      string
    Username    string
    ProjectID   string
    Permissions map[string]bool
}
```

The principal is attached to request context.

Handlers must not read roles/password/session cookies directly to make authorization decisions.

## 7. Authorization model

Authorization is enforced on the server/API/session layer.

Hiding a frontend icon is only a UX optimization and is never the security boundary.

Default is deny.

### 7.1 Initial permission registry

#### Tasks

```text
tasks.view
tasks.run
```

#### Terminal

```text
terminal.view_own
terminal.control_own
terminal.create
terminal.view_all
terminal.control_all
```

#### Patch Tool

```text
patch.view
patch.run
patch.collect
patch.history
patch.cleanup
```

#### Files / Project Explorer

```text
files.read
files.download
files.upload
files.write
```

If/when explicit delete/rename APIs exist, give them independent permissions rather than bundling them silently into read access.

#### Git read-only tools

```text
git.status
git.log
git.diff
```

Existing safe-Git constraints continue to apply. RBAC does not broaden the Git command surface.

#### Settings

```text
settings.read
settings.write
```

#### Self update

```text
selfupdate.check
selfupdate.run
```

#### Users / access management

```text
users.view
users.manage
roles.view
roles.manage
audit.view
sessions.manage
```

#### Project administration

```text
project.admin
```

The registry should be centralized in code rather than permission strings being invented independently by each handler.

## 8. HTTP/API enforcement

Introduce reusable authorization middleware/helpers:

```text
Authenticate
  -> ResolvePrincipal
  -> RequirePermission(...)
  -> Handler
```

Examples:

```text
GET  /api/tasks                    -> tasks.view
POST /api/sessions kind=task       -> tasks.run
POST /api/sessions kind=terminal   -> terminal.create
POST /api/sessions kind=patch      -> patch.run
GET  project file                  -> files.read
POST file upload                   -> files.upload / files.write
GET  git status                    -> git.status
POST Patch History cleanup         -> patch.cleanup
POST self-update                   -> selfupdate.run
```

WebSocket/session attach/input/resize/stop/kill must be authorized too.

It is not sufficient to authorize only `POST /api/sessions` at session creation time.

## 9. Session ownership

Multi-user mode requires resource ownership.

Extend session metadata conceptually with:

```text
OwnerUserID
ProjectID
CreatedBy
Visibility
```

### Terminal

Default:

- creator owns the terminal
- owner can view/control if permitted
- another ordinary user cannot subscribe to or type into it
- `terminal.view_all` allows viewing other users' terminals
- `terminal.control_all` allows controlling/stopping other users' terminals

### Patch Tool

Patch history/status can be project-visible to users with `patch.view`/`patch.history`.

Mutating Patch actions require the respective permission regardless of who created the Patch session.

The exact owner/control semantics should be documented per Patch operation before implementation.

## 10. Shared workspace concurrency

Multiple users operate on one filesystem, so authentication alone is insufficient.

Introduce a project mutation-lock concept for operations that can conflict.

Initial candidates:

- PATCH apply/mutation
- destructive file write operations
- self-update
- later, other project-wide mutations as required

Read operations and safe independent operations should not unnecessarily acquire the lock.

Expected behavior:

```text
User A: PATCH mutation running
User B: view History       -> allowed
User B: read files         -> allowed
User B: conflicting PATCH  -> queued/blocked with owner information
```

The lock should be represented server-side and auditable, not only in browser state.

## 11. Audit

Shared mode should produce structured audit events for security-relevant actions.

Examples:

- login success/failure
- logout
- password reset/change
- user enable/disable
- permission/role changes
- terminal create/stop/kill
- task run
- Patch run/COLLECT/cleanup
- file upload/write
- settings changes
- self-update
- authorization deny

Do not audit raw:

- passwords
- session tokens
- secret values
- terminal keystrokes by default

Audit APIs require `audit.view`.

## 12. UI

Shared mode adds an authenticated identity area and an administration section.

Suggested Settings area:

```text
USERS & ACCESS
  Users
  Roles
  Project Access
  Sessions
  Audit Log
```

User editor should provide a simple module-level surface first:

```text
Tasks       [x]
Patch Tool  [x]
Files       [x]
Terminal    [ ]
Git         [x]
Settings    [ ]
Self Update [ ]
```

and an Advanced section for granular permissions.

Frontend module visibility is generated from server-provided current-user capabilities, but API enforcement remains authoritative.

## 13. Future multi-project central management readiness

The first implementation remains single-project per daemon but must prepare these extension points now.

### 13.1 Global user IDs

Users are global to the shared identity database, not local to a workspace.

### 13.2 Project membership relation

Every role/permission grant that is project-specific is scoped by `project_id`.

### 13.3 Shared identity-store abstraction

HTTP handlers should depend on an identity/authz service interface, not raw SQLite queries.

This permits future backends:

- shared SQLite on one host
- TaskDeck Hub identity service
- PostgreSQL/other centralized DB if projects later run on different hosts

### 13.4 Project daemon registration

A later Hub can track:

```text
project_id
display_name
daemon_url
host
health/status
last_seen
```

This is operational metadata and should remain separate from the core membership identity.

### 13.5 No port-derived permissions

Authorization data must not depend on TCP port. Multiple ports are only process endpoints.

## 14. Security boundaries

### Filesystem

- Identity DB outside workspace.
- Restrictive filesystem permissions.
- Existing project traversal/symlink protections remain mandatory.
- RBAC never bypasses path validation.

### Authentication

- Rate-limit login attempts.
- Generic login failure messages.
- No credential disclosure endpoint.
- Session revocation.
- Idle/absolute expiration policy to be defined before production shared deployment.

### Network

- Shared mode is intended for HTTPS.
- Existing self-signed TLS may remain useful for private deployments.
- Reverse proxy support can be added without weakening direct-server validation.
- Cookie security must follow the effective HTTPS deployment.

### Authorization

- Deny by default.
- Every sensitive server action has an explicit capability.
- UI hiding is not authorization.
- Resource ownership is checked in addition to module permission where relevant.

## 15. Configuration compatibility contract

Proposed example:

```ini
[server]
protocol = https
bind = 127.0.0.1
port = 0
open_browser = true

[auth]
# Legacy/local authentication only when shared_server.enabled=false.
enabled = false
username = admin
password = change-me

[shared_server]
# Default false. Existing installations remain legacy/local mode.
enabled = false

# Required when enabled=true.
# Must be stable and unique in the selected identity DB.
# project_id = m3-client

# Optional absolute common DB path.
# If omitted, TaskDeck resolves a platform data-dir default.
# identity_db = /var/lib/taskdeck/identity.db
```

Validation rules to implement:

### shared_server.enabled=false

- Use current validation exactly where possible.
- Remote bind continues to require the current legacy authentication protections.

### shared_server.enabled=true

- Require valid `project_id`.
- Resolve/open identity store.
- Legacy username/password are not accepted as shared-user credentials.
- Listener validation treats shared authentication as the remote-auth mechanism instead of requiring `AuthEnabled=true`.
- Fail closed if project identity/store cannot be initialized.
- Do not auto-create an unrestricted user silently.

Bootstrap of the first admin must be an explicit flow and is a separate implementation decision.

## 16. Initial admin bootstrap

Do not put an initial plaintext admin password in the shared INI.

Preferred direction:

- explicit CLI/bootstrap command
- only works while no usable shared admin exists, unless invoked by an already authorized admin
- password is entered interactively or provided through a safe non-persistent mechanism
- stores only Argon2id hash

Exact command/UI syntax will be finalized in the implementation phase.

## 17. Migration and backward compatibility

No migration is required for existing users while shared mode remains disabled.

Enabling shared mode is an explicit transition.

Required transition checks:

- `project_id` configured
- identity DB initialized
- project registered
- at least one enabled admin/member with access
- shared login available before server exposes protected project APIs

Do not automatically convert the legacy `[auth]` password into a DB user.

## 18. Implementation roadmap

All implementation work should use small checkpoint commits.

### Phase 0 — Design baseline

- [x] Create feature branch.
- [x] Record complete goals, compatibility rules, schema direction, permissions and roadmap in this document.
- [ ] Keep this document updated when implementation decisions change.

### Phase 1 — Config foundation

Small commits:

- [x] Add `SharedServerEnabled`, `SharedProjectID`, `SharedIdentityDB` to config.
- [x] Parse/write default `[shared_server]` section with `enabled=false`.
- [x] Add validation preserving existing legacy mode.
- [x] Add config tests for both modes.

Implementation note: shared mode does not yet bypass the existing remote Basic Auth guard. That guard is intentionally kept until shared authentication middleware exists, so an intermediate feature-branch revision cannot expose an unauthenticated remote listener.

No login/RBAC behavior change yet.

### Phase 2 — Identity store foundation

- [x] Introduce identity package/domain model/interfaces.
- [x] Secure durable data-directory/path resolution outside the workspace.
- [ ] SQLite open/configure/migration infrastructure.
- [x] Initial schema: users/projects/roles/permissions/memberships/member overrides/sessions/audit.
- [ ] WAL/busy-timeout and multi-process DB tests.
- [ ] Password hashing helper using Argon2id.

Implementation note: TaskDeck currently keeps its Go dependency surface very small and locally replaced/vendored. The SQLite driver must preserve portable/self-update builds rather than silently introducing a network-only build dependency. Driver integration is therefore isolated as its own checkpoint after the store boundary/schema are stable.

No HTTP login change until this layer is tested.

### Phase 3 — Authentication

1. Shared-mode login/logout/current-user API.
2. Session token generation/hash/storage/revocation.
3. Secure cookie handling.
4. Request Principal middleware.
5. Explicit first-admin bootstrap.
6. Login rate limiting.

Legacy mode remains on the existing Basic Auth path.

### Phase 4 — Authorization core

1. Central permission registry.
2. Seed system roles/permissions.
3. Project membership resolution.
4. `RequirePermission` helpers.
5. Deny-by-default tests.
6. Current-user capabilities endpoint for UI.

### Phase 5 — Module permissions

Apply authorization in small slices:

1. Tasks.
2. Terminal create.
3. Terminal subscribe/input/resize/stop/kill.
4. Patch Tool read/run/collect/history/cleanup.
5. Files/project explorer.
6. Git read-only endpoints.
7. Settings.
8. Self-update.

Each slice includes backend tests before frontend hiding.

### Phase 6 — Session ownership

1. Add owner/project metadata.
2. Own/all authorization rules.
3. Terminal privacy.
4. Session listing visibility filters.
5. Admin control permissions.

### Phase 7 — Audit

1. Audit writer.
2. Authentication/security events.
3. Task/Terminal/Patch/File/admin actions.
4. Authorization denials.
5. Audit query API with permission checks.

### Phase 8 — Administration UI

1. Users.
2. Roles.
3. Project membership.
4. Permission overrides.
5. Active sessions / force logout.
6. Audit log.
7. Module visibility based on current-user capabilities.

### Phase 9 — Shared-workspace mutation coordination

1. Project mutation lock service.
2. Patch mutation lock.
3. Relevant file mutation locks.
4. Lock owner/status UI.
5. Audit/timeout/recovery behavior.

### Phase 10 — Shared-server hardening

1. Session expiration policy.
2. Password policy/reset flow.
3. Backup/restore identity DB.
4. Reverse-proxy/deployment documentation.
5. Additional abuse/rate limits.
6. Security review of all public endpoints/WebSockets.

### Phase 11 — Future central management / Hub

Not part of the first shared-server implementation, but architecture should permit:

- one global user across many projects
- centralized project/member/role management
- project daemon registration/status
- remote identity backend
- central logout/session revocation
- central audit aggregation

## 19. Non-goals for the first implementation

Do not add these prematurely:

- one daemon serving multiple workspaces
- SSO/OIDC/LDAP
- multi-host centralized database
- organization/team hierarchy
- terminal keystroke recording
- cross-project file browser
- automatic legacy password migration
- implicit anonymous shared access

They can be added later without changing the fundamental user/project/membership model.

## 20. Acceptance criteria for the first shared-server release

The feature is not complete until all of the following hold:

1. Default INI behavior remains compatible with current TaskDeck.
2. Shared mode is explicitly opt-in.
3. Multiple users can authenticate concurrently.
4. One user can be modeled as a member of multiple projects in the DB.
5. Running daemon authorizes only its configured project.
6. Module/action permissions are enforced server-side.
7. A user without Terminal permission cannot create, attach to or control a Terminal through direct API/WebSocket calls.
8. A user without Patch permission cannot invoke Patch mutations through direct APIs.
9. Session/resource ownership prevents ordinary users from taking over other users' terminals.
10. Passwords cannot be retrieved from the application.
11. Raw session tokens are not stored in the identity DB.
12. Admin can disable a user and revoke active sessions.
13. Audit records identify who performed security-relevant actions.
14. Shared identity DB works safely with multiple TaskDeck project processes on the same host.
15. Existing local/legacy installations keep working when `shared_server.enabled=false`.

## 21. Design invariants

These invariants should be preserved throughout implementation:

- **Single daemon, single workspace.**
- **Shared user identity, project-scoped authorization.**
- **Legacy mode remains the default.**
- **Shared mode fails closed.**
- **Server authorization is authoritative; frontend visibility is not security.**
- **Passwords are one-way hashed; sessions are revocable.**
- **Identity data is outside the workspace.**
- **Stable project ID, never TCP port/path as authorization identity.**
- **Deny by default.**
- **Small implementation commits with tests at each boundary.**
