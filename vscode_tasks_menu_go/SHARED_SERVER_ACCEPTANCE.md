# TaskDeck Shared Server First-Release Acceptance

Status: automated acceptance implemented; deployed reverse-proxy/browser smoke remains operator-run.

This checklist is the release gate for `[shared_server] enabled=true`. It does
not expand scope into Phase 11 / TaskDeck Hub.

## Automated acceptance evidence

The CI matrix runs on Go 1.19.x and Go 1.23.x and includes staged tests,
`go test ./...`, `go vet ./...` and a production build.

Latest boundary-review checkpoint: GitHub Actions run `36289444809` passed the
full matrix at commit `89ccd1fe`.

The first-release criteria in `MULTI_USER_SHARED_SERVER_PLAN.md` are covered as
follows:

1. Legacy/default mode remains the default and retains its existing auth/browser
   lease behavior: config/auth/server regression tests.
2. Shared mode is explicit opt-in and validates HTTPS/project identity before
   serving protected routes: config/shared-auth tests.
3. Multiple users can authenticate concurrently without the legacy single-browser
   lease: `TestSharedHTTPConcurrentUsersAndMembershipIsolation`.
4. One global user can belong to several projects: identity membership tests and
   `TestSharedReleaseAcceptanceTwoDaemonsOneIdentityDB`.
5. A daemon authorizes only its configured project. Browser auth sessions are
   project-bound, and a project-A bearer token is rejected by project B:
   `TestBrowserSessionIsBoundToLoginProject` and the two-daemon acceptance test.
6. Module/action permissions are enforced server-side: shared authorization route
   matrix and direct-denial tests.
7. Terminal create/view/control and WebSocket input are independently authorized:
   shared session tests and
   `TestSharedReleaseAcceptanceHTTPSLoginAndWSSAuthorization`.
8. Patch mutations require Patch-specific permissions and cannot be reached with
   read-only grants: shared session/action/mutation tests.
9. Task/Terminal/Patch session metadata carries owner + project and ordinary
   users cannot take over another user's terminal: session ownership tests.
10. Password plaintext/hash retrieval is not exposed through normal admin/current
    user APIs; password storage is one-way scrypt: password/admin-view tests.
11. Browser bearer tokens are random; only SHA-256 token hashes are stored:
    authentication/session schema tests.
12. User disable, password change/reset and explicit admin session revoke invalidate
    active sessions according to their intended global/project scopes: admin,
    password-change and password-reset tests.
13. Security-relevant actions and denials are attributed in project/user audit
    records without credential material: shared audit tests.
14. Two independent SQLite Store/Server instances operate concurrently against
    one identity DB using WAL/busy-timeout semantics:
    `TestSharedReleaseAcceptanceTwoDaemonsOneIdentityDB`.
15. `shared_server.enabled=false` continues using the legacy code path:
    config/auth/server compatibility tests.

Additional hardening acceptance:

- Full HTTPS login -> Secure/HttpOnly cookie -> WSS terminal flow is tested
  through the complete `Server.Handler()` middleware chain.
- Project admins can only list/revoke `auth_sessions.project_id` belonging to
  their own project; cross-project revoke returns not found.
- Project administration boundaries are regression-tested across users, custom
  roles and audit data as well as sessions: project-B-only resources are absent
  from project A list APIs, and direct-ID access/permission/role mutations from
  project A return not found without changing project B state.
- Cross-project bearer reuse and cross-project logout are both fail-closed: a
  daemon cannot authenticate or revoke a browser session minted for another
  project.
- Shared WebSocket handshakes retain the WebSocket library's same-origin
  protection; an explicit hostile Origin is rejected before attach.
- Identity schema v2 adds session `project_id`; v1 sessions without trustworthy
  project provenance are revoked during migration and cannot be reused.
- Identity schema v3 rejects new auth-session INSERTs without `project_id`,
  preventing an older daemon from minting unscoped sessions after migration.
- Password change/reset remains deliberately global because passwords belong to
  the global user identity; those operations revoke that user's sessions in all
  projects.
- Reverse-proxy loopback does not imply internal daemon trust; self-update
  handoff/detach additionally requires the private daemon control token.

## Manual deployed-browser smoke

Run this once on a release candidate in the intended deployment topology. Keep
one identity DB outside both workspaces and start two real project daemons on
different ports.

### 1. Bootstrap and two-project identity

- Start project A in shared mode and bootstrap the first administrator.
- Provision project B and grant the same global user membership there.
- Confirm both daemons point to the same identity DB but have distinct
  `shared_server.project_id` values.
- Sign in to A and B in separate browser profiles.
- Confirm both remain usable concurrently.

Pass condition: neither browser displaces the other and each UI reports the
correct project.

### 2. Project-bound login sessions

- Sign in to project A.
- Copying/renaming A's cookie into project B must not authenticate B.
- Sign in normally to B.
- From project A's admin Sessions UI, verify B's session is not listed and cannot
  be revoked by supplying its ID directly.

Pass condition: session administration and bearer authentication stay
project-scoped.

### 3. Permissions and terminal WSS

Use a non-admin user with own-terminal view permission.

- Without terminal control permission, attach to the user's own terminal and
  verify output remains visible.
- Attempt keyboard input and verify it is rejected/read-only.
- Grant terminal control permission, reconnect and verify input works.
- Verify the user cannot attach/control another user's terminal unless the
  explicit all-users permission is granted.

Pass condition: browser behavior matches the direct API/WebSocket authorization
tests.

### 4. Reverse proxy

With the documented HTTPS -> HTTPS reverse proxy configuration:

- Public login succeeds.
- Browser Origin/Host checks do not reject legitimate requests.
- Terminal WSS connects and survives normal use/reconnect.
- A public request cannot invoke self-update `handoff` or `detach`.
- The proxy strips `X-TaskDeck-Internal-Control` from client input.
- TaskDeck's backend TLS certificate is verified/pinned by the proxy.

Pass condition: no plaintext proxy -> TaskDeck hop and no proxy-loopback trust
bypass.

### 5. Password and session lifecycle

- Change a user's password from Account.
- Verify sessions for that user in both projects are logged out.
- Sign in again with the new password.
- Perform a host-operator password reset and verify the same global logout.
- Disable only the user's project-B membership and verify A remains usable while
  B is denied.

Pass condition: global password operations and project-local access operations
have distinct scopes.

### 6. Backup / restore drill

- Create a shared identity backup with `--shared-identity-backup`.
- Make a recognizable non-secret change such as a test display name or temporary
  membership.
- Restore the backup with `--shared-identity-restore`.
- Verify the pre-restore safety backup path is printed.
- Recheck users, memberships, sessions and audit state.

Pass condition: restored state matches the backup and the safety snapshot exists.

### 7. v1 -> current-schema upgrade drill

For a disposable copy of a pre-v2 identity DB:

- Back up the DB.
- Start upgraded TaskDeck.
- Confirm migration reaches the current schema (v2 project scope followed by
  v3 insert enforcement) without manual SQL.
- Confirm users/memberships/roles are preserved.
- Confirm old browser sessions require one re-login.
- Confirm newly created sessions are project-bound.
- During a disposable mixed-version test, confirm an old-style INSERT lacking
  `project_id` is rejected after v3.

Pass condition: migration is automatic, fail-closed for old bearer sessions and
does not lose identity/access data.

## Release sign-off

Do not mark the first shared-server release accepted until:

- the current branch HEAD has a green CI matrix on both supported Go versions;
- every manual smoke section above has been exercised in the target deployment
  topology;
- any deviation is either fixed or explicitly documented as a release blocker.

Record the tested commit SHA, deployment hostname/topology and date alongside
the release notes rather than placing credentials or private infrastructure
details in this repository.
