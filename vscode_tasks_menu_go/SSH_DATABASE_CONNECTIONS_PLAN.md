# TaskDeck SSH & Database Connections Plan

Status: SSH backend, connection testing, terminal target metadata, and Connections panel implemented; database adapter foundation is next

Branch: `feat/ssh-database-connections`

Base: `main` at `129a1d2991035b866b37def2670594f6db063816`

## 1. Purpose

Extend TaskDeck with reusable connection profiles for remote SSH terminals and database access while preserving the current local terminal workflow.

Priority order:

1. SSH
   1. Integrate remote SSH terminals into the existing Terminal experience.
   2. A new terminal can be local or remote using a saved SSH profile.
   3. SSH profiles store connection metadata, authentication method, display name, optional custom home directory, and preset commands executed after a successful connection.
   4. Evaluate and, when it improves usability without destabilizing the existing layout, add a left-side connection panel for SSH profiles and local-terminal home directories instead of relying only on menu/settings dialogs. Open terminals remain ordinary TaskDeck terminal tabs.
2. MySQL database connections.
3. MySQL through remote SSH tunnel.
4. Other database adapters (MongoDB, Redis, SQLite) and remote SSH tunnel where the database is TCP based.

## 2. Mandatory engineering constraints

These rules are requirements, not preferences:

- Minimize third-party libraries.
- Do not add an npm package when at least one viable non-npm solution exists.
- Prefer Go and the Go standard library.
- Python may be used only for a capability that would otherwise be unreasonably complex or unavailable in Go under the dependency constraints.
- Python usage must prefer the Python standard library; do not introduce pip packages merely for convenience.
- Preserve TaskDeck's offline/self-update build properties.
- Do not silently introduce build-time network dependency installation.
- To avoid losing progress on errors/timeouts, implement and commit small independent slices. Do not accumulate many unrelated changes before committing.
- Each phase must be decomposed into checkpoints small enough to test/review independently.
- New security-sensitive behavior must fail closed.
- Secrets must never be exposed by profile-list APIs, normal logs, terminal titles, audit output, process arguments, or browser-visible configuration payloads.
- Frontend hiding is never a security boundary; sensitive actions must be checked on the server.
- Preserve current local terminal behavior unless a user explicitly selects a remote profile.

## 3. Architecture principles

### 3.1 Keep TaskDeck core in Go

The TaskDeck server, lifecycle managers, HTTP APIs, terminal sessions, tunnel manager, adapter manager and security checks remain Go.

Do not move the application to Python.

Allowed exception: a narrowly isolated helper/adapter may use Python stdlib when it substantially reduces complexity and does not contaminate TaskDeck core dependencies. SQLite is the expected first example because Go stdlib has no SQLite driver while Python stdlib provides `sqlite3`.

### 3.2 Use system OpenSSH instead of implementing SSH protocol

Do not add an SSH protocol library to TaskDeck while the system `ssh` client is available.

TaskDeck should supervise OpenSSH with `os/exec` and PTY integration.

Reasons:

- no new Go dependency;
- mature support for key auth, ssh-agent, known_hosts, password prompts, ProxyJump and port forwarding;
- reuse of the user's/server's existing OpenSSH configuration;
- same mechanism can power interactive terminals and database tunnels;
- avoids implementing SSH protocol/authentication/host-key validation ourselves.

TaskDeck must probe OpenSSH availability and return an explicit actionable error when it is absent.

### 3.3 Database adapters are separate processes

Database-specific logic must not become a large switch in TaskDeck core.

Use an adapter protocol over JSON Lines on stdin/stdout:

```text
TaskDeck
  |
  +-- DB Adapter Manager
          |
          +-- JSONL IPC
                  |
                  +-- mysql adapter
                  +-- mongo adapter
                  +-- redis adapter
                  +-- sqlite adapter
```

Each adapter advertises capabilities. TaskDeck renders generic database UI from those capabilities and normalized results.

Adding a future adapter should normally require adding a new adapter implementation/manifest, not editing database-specific logic throughout TaskDeck.

### 3.4 SSH tunnel is transport, not database behavior

For TCP databases:

```text
DB profile
   |
   +-- optional SSH profile
           |
           +-- Tunnel Manager
                   |
                   +-- ssh -N -L 127.0.0.1:<local>:<remote-host>:<remote-port>
                           |
                           +-- local endpoint
                                   |
                                   +-- DB adapter
```

The database adapter receives only the effective host/port and does not implement SSH.

SQLite is file-based and does not use TCP local forwarding. Remote SQLite, if implemented, must use remote execution/helper semantics rather than pretending it is a tunneled TCP database.

## 4. SSH profile model

Initial conceptual model:

```text
SSHProfile
    id
    name
    host
    port
    username

    authentication
        agent
        private_key
        password

    identity_file
    secret_ref

    custom_home_dir

    preset_commands[]
        id
        name
        command
        cwd / optional

    connect_timeout
    server_alive_interval
    server_alive_count_max
    proxy_jump / optional
```

Rules:

- default port: 22;
- `name` is display-only and must not be used as a security identity;
- passwords/private-key passphrases are stored by reference, not inline in public profile metadata;
- private key paths may be stored, but private key contents should not be copied into ordinary profile JSON unless an explicit secure import feature is designed later;
- custom home dir is optional;
- when custom home dir is empty, the remote login shell default is used;
- preset commands run only after SSH connection/session establishment;
- preset commands are never concatenated into the `ssh` command line;
- command/cwd quoting must happen on the remote shell boundary, not by ad-hoc string concatenation on the local process argv.

## 5. SSH authentication strategy

Preferred order:

1. ssh-agent / normal OpenSSH resolution;
2. explicit `IdentityFile`;
3. password/passphrase through an isolated askpass mechanism.

Never use a password directly in:

- process argv;
- terminal title;
- connection URL;
- logs;
- browser query string;
- environment variable with long-lived scope.

For password/passphrase auth, design an internal one-time askpass bridge:

```text
ssh process
  |
  +-- SSH_ASKPASS=<TaskDeck helper>
        |
        +-- one-time token
              |
              +-- in-memory/local secret broker
                    |
                    +-- secret
```

Requirements:

- one-time token;
- short expiry;
- process-bound where practical;
- no plaintext secret persisted in temporary files;
- no fallback to printing the secret;
- clear timeout/cancellation semantics.

Host-key verification remains enabled by default. Do not silently use `StrictHostKeyChecking=no`.

## 6. Terminal integration

Remote terminals should reuse the current terminal/session/tab infrastructure.

User-level concept:

```text
New Terminal
  Local
    - Workspace
    - saved local home directories
  Remote
    - Production
    - Staging
    - ...
```

After a remote terminal is opened, it behaves like an existing terminal tab:

- resize;
- input;
- reconnect/error status as designed;
- close/kill;
- file/URL tracking only where semantically valid;
- tab title derived from profile/display host, never credentials.

The remote transport must remain distinguishable in session metadata:

```text
TerminalTarget
    type = local | ssh
    profile_id
```

This prepares future RBAC/audit integration.

## 7. Left-side connections panel decision

The existing Terminal menu/settings should not become the only place to discover and use many saved profiles.

Preferred UX direction after inspecting the existing activity/sidebar layout:

```text
CONNECTIONS

LOCAL
  Workspace
  ~/568E/M3
  ~/My_projects

SSH
  Production
  Staging
  DatBike

DATABASES
  MySQL Production
  ...
```

Initial implementation should start with LOCAL + SSH. DATABASES are added when their phase begins.

Expected actions:

- single/double click or explicit open button to create a terminal;
- context/edit affordance for profile management;
- add profile;
- edit profile;
- duplicate profile later if useful;
- delete profile with confirmation;
- no secret values rendered in the panel.

The panel must integrate with the current activity bar/sidebar patterns rather than introducing an unrelated navigation system.

If implementation evidence shows the current left sidebar cannot accommodate this cleanly without a large UI rewrite, keep the data/API model independent and ship a minimal connection panel checkpoint first. Do not block SSH backend work on a major UI redesign.

## 8. Local terminal home-directory profiles

Local terminal shortcuts belong in the same connection model/UI because the user chooses them for the same reason: where to open a terminal.

Conceptual model:

```text
LocalTerminalProfile
    id
    name
    cwd
    preset_commands[]
```

Rules:

- path must pass existing project/server path validation where applicable;
- do not weaken traversal/symlink safety;
- local preset commands are optional;
- existing default terminal remains unchanged.

## 9. Secret storage

Do not store connection passwords in plaintext INI/profile JSON.

Introduce a secret-store abstraction before password auth is enabled:

```text
Profile metadata
     |
     +-- secret_ref
             |
             +-- SecretStore
```

Preferred implementation with Go stdlib:

- `crypto/aes`;
- `crypto/cipher`;
- `crypto/rand`;
- AES-GCM;
- random master key stored outside the workspace;
- master key file permission `0600` on POSIX;
- enclosing data directory `0700` where practical.

Encrypted record includes at least:

```text
version
key_id
nonce
ciphertext
```

The master key must not be stored in the same profile/database record as encrypted secrets.

API rule: a caller may be authorized to USE a saved connection without ever receiving its stored password.

## 10. Database adapter protocol

Start with a deliberately small versioned JSONL protocol.

### 10.1 Process lifecycle

TaskDeck starts one adapter process per active logical connection/session unless later evidence justifies pooling.

Transport:

- stdin: request JSONL;
- stdout: protocol response/event JSONL;
- stderr: bounded diagnostic log, never protocol and never secrets.

Every message includes a protocol version and request/session identity where relevant.

### 10.2 Core operations

Initial generic operations:

```text
hello
capabilities
connect
disconnect
ping
list_catalogs
list_objects
describe_object
execute
cancel
```

Optional transactional capabilities:

```text
begin
commit
rollback
```

Do not make the core interface SQL-only.

Object kinds may include:

```text
database
schema
table
view
collection
keyspace
key
index
```

### 10.3 Normalized execute result

Example:

```json
{
  "columns": [
    {"name": "id", "type": "integer"},
    {"name": "name", "type": "string"}
  ],
  "rows": [
    [1, "Alice"],
    [2, "Bob"]
  ],
  "affected_rows": 0,
  "truncated": false
}
```

Results must be bounded. Large query results require row/byte limits and later paging/streaming rather than unbounded browser payloads.

## 11. MySQL adapter

Priority database: MySQL.

Do not implement MySQL wire protocol in TaskDeck.

Initial strategy:

- use installed `mysql` or `mariadb` CLI through an isolated adapter;
- detect executable/capabilities;
- invoke batch/raw modes suitable for machine parsing;
- use a protected temporary option file or other non-argv credential delivery;
- ensure cleanup on success, failure and cancellation;
- restrict permissions of temporary credential material;
- never print generated option-file contents.

Initial feature scope:

- test connection;
- list databases;
- list tables/views;
- describe table;
- execute query;
- bounded tabular result;
- cancellation/timeout;
- clear error normalization.

If a future requirement needs richer type metadata/prepared statements and CLI is insufficient, use a separate adapter module/binary so the dependency does not enter TaskDeck core.

## 12. MySQL via SSH tunnel

Use OpenSSH local forwarding:

```text
ssh -N   -L 127.0.0.1:<allocated-port>:<db-host>:<db-port>   <ssh-target>
```

Required options/semantics:

- bind forwarding to loopback by default;
- `ExitOnForwardFailure=yes`;
- server-alive settings;
- no password on argv;
- explicit lifecycle ownership;
- release tunnel when no longer needed;
- kill/reap child process on TaskDeck shutdown;
- detect dead tunnel before issuing DB operations where practical.

DB profile stores:

```text
transport = direct | ssh_tunnel
ssh_profile_id
remote_db_host
remote_db_port
```

The effective database endpoint exposed to the adapter is the local forwarded endpoint.

OpenSSH connection multiplexing (`ControlMaster`/`ControlPath`/`ControlPersist`) may be evaluated after the basic tunnel lifecycle is correct. It is an optimization, not a Phase-1 prerequisite.

## 13. Redis adapter

Prefer pure Go stdlib implementation of RESP.

No Redis client library should be added merely for convenience.

Initial RESP support:

- simple string;
- error;
- integer;
- bulk string;
- array;
- null.

Initial commands/features can include:

- PING;
- AUTH;
- SELECT;
- SCAN;
- TYPE;
- TTL;
- GET;
- SET only when write mode is intentionally enabled later;
- key-type-specific reads.

Start read-oriented and capability-based. Do not accidentally expose arbitrary destructive commands merely because the protocol supports them.

SSH tunnel transport reuses the generic Tunnel Manager.

## 14. MongoDB adapter

Do not implement MongoDB wire protocol from scratch in TaskDeck.

Initial preferred strategy:

- isolated adapter invoking installed `mongosh`;
- machine-readable JSON/EJSON output;
- no npm package added to TaskDeck;
- executable presence/capability detection.

If `mongosh` proves insufficient, a future standalone Mongo adapter may use an official Go driver in its own module/build boundary. It must not force TaskDeck core to adopt that dependency or a newer Go toolchain.

SSH tunnel transport reuses the generic Tunnel Manager.

## 15. SQLite adapter

SQLite is file-based.

Preferred implementation:

- isolated adapter/helper;
- Python stdlib `sqlite3` is allowed and preferred over adding a Go SQLite dependency;
- no pip package.

Capabilities:

- open local database file;
- list tables/views;
- describe table;
- execute bounded query;
- explicit read/write mode.

SQLite does not use SSH TCP forwarding.

Remote SQLite, if added later, requires an explicit remote-execution design through SSH and must account for WAL/locking/consistency. Do not download a live database file and treat the copy as authoritative.

## 16. Security and future multi-user compatibility

Even when implemented on current `main`, design surfaces so they can later integrate with shared-server RBAC.

Candidate future permissions:

```text
terminal.local.create
ssh.profiles.view
ssh.profiles.manage
ssh.connect

db.profiles.view
db.profiles.manage
db.connect
db.query
db.tunnel
```

Profiles should be ready for ownership/scope metadata:

```text
owner_user_id
project_id
scope = private | project
```

Do not hard-code current single-user assumptions into adapter/SSH protocols if avoidable.

## 17. Implementation roadmap

All code work must use small checkpoint commits.

### Phase 0 — Design baseline

- [x] Create branch from `main`.
- [x] Record architecture, constraints, priority order and phases.
- [x] Keep this document updated as decisions change.

### Phase 1 — SSH foundation

1. [x] Inspect/reuse current Terminal session creation and PTY lifecycle.
2. [x] Introduce SSH profile domain model without secrets.
3. [x] Add durable profile store with validation, atomic writes and cross-process mutation locking.
4. [x] Introduce AES-GCM secret-store abstraction and protected master-key lifecycle using Go stdlib only.
5. [x] Add OpenSSH executable discovery/probe.
6. [x] Build safe SSH argv builder for agent/private-key/password modes.
7. [x] Add host-key-safe connection behavior: interactive profiles use `ask`; forced askpass uses OpenSSH `accept-new`, never `no`.
8. [x] Add one-time Unix-socket askpass secret broker for password/private-key passphrase mode.
9. [x] Integrate remote SSH process with the existing PTY Terminal session path.
10. [x] Support optional custom remote home directory.
11. [x] Support preset commands after successful connection.
12. [x] Add secret-safe SSH profile CRUD API.
13. [x] Open SSH profiles through the existing `POST /api/sessions` Terminal API using `ssh_profile_id`.
14. [x] Add an explicit SSH profile connection-test endpoint and Connections-panel Test action without opening a Terminal tab.
15. [x] Add backend regression tests for validation, argv safety, secret non-disclosure, askpass one-time semantics and request isolation.

Implementation notes:

- Stored secrets are never placed in argv or browser-visible profile responses.
- Remote SSH session requests reject browser-provided environment overrides so callers cannot replace the internal askpass socket/token environment.
- SSH authentication secrets are bounded to 4096 bytes and reject NUL/CR/LF before storage.
- SSH connection tests use the same OpenSSH/authentication path with `-T` and remote `true`, bounded diagnostics, and no custom-home/preset execution.
- The current execution environment has no GitHub network access and this repository has no workflow run for these commits, so the newly added Go tests have not been executed by this session; source-level regression tests are committed in small checkpoints and must be run on a normal checkout before release.

### Phase 2 — Connections panel / Terminal UX

1. [x] Inspect current Activity Bar/sidebar composition.
2. [x] Choose the existing Activity Bar/sidebar architecture rather than adding SSH management to Settings-only UI.
3. [x] Add left-side Connections entry/panel using existing UI patterns.
4. [x] Add LOCAL group with current workspace/default terminal.
5. [x] Surface and manage the existing saved workspace-local Terminal directories from the Connections panel without weakening workspace path validation.
6. [x] Add SSH group with saved profiles.
7. [x] Add add/edit/delete profile UI without exposing stored secrets.
8. [x] Open local/remote terminals as existing terminal tabs.
9. [x] Add structured terminal target metadata (`local|ssh`, profile ID) to session metadata.
10. [x] Preserve existing Terminal menu/settings behavior for compatibility.
11. [x] Add source-level UI regression tests for panel wiring, Activity Bar integration and explicit Terminal target handling.

### Phase 3 — Database adapter foundation

1. [x] Define versioned JSONL protocol.
2. [x] Define adapter manifest/capabilities.
3. [x] Implement bounded process supervisor and cancellation.
4. [x] Implement allowlisted adapter registry/discovery boundary plus runtime identity/capability handshake.
5. [x] Add in-process and re-exec dummy/test adapters to lock protocol semantics.
6. [x] Add DB profile model/store with secret references and direct/SSH-tunnel transport schema.
7. [x] Add generic DB connection/session API with browser operation allowlist.
8. [x] Add bounded normalized execute result model and enforce it at the process boundary.
9. [x] Add protocol, profile, process, session and HTTP security regression tests.

Implementation notes:

- Adapter executables/argv exist only in the server-side registry and are omitted from browser JSON.
- Each logical DB session owns one supervised adapter process; identity and capabilities must match the registered manifest before connect.
- Browser callers cannot invoke protocol control operations such as connect/disconnect/cancel/transactions directly.
- Adapter stderr is bounded and never used as protocol.
- Query cancellation currently terminates the owning adapter process, guaranteeing no orphaned long-running query at the cost of closing that logical DB session.

### Phase 4 — MySQL direct

1. [x] Discover `mysql`/`mariadb` client.
2. [x] Implement protected per-invocation option-file credential handoff; password is never placed in argv/env.
3. [x] Connect/test through the real CLI path.
4. [x] List databases/catalogs through `information_schema`.
5. [x] List tables/views.
6. [x] Describe table columns.
7. [x] Execute bounded single-statement queries with XML normalization.
8. [x] Timeout/cancel via request context; oversized stdout/stderr terminate the client.
9. [x] Normalize/redact CLI errors.
10. [x] Add MySQL profile management in Connections plus database workspace tabs, schema browser and query/result UI.
11. [x] Add command-stub/fixture tests; no live DB required for unit tests.

Implementation notes:

- SQL is sent through client stdin and never placed in process argv.
- MySQL option files/directories use POSIX `0600`/`0700` and are deleted after every CLI invocation.
- Read-only profiles are filtered server-side to conservative read operations and also start each CLI connection with `SET SESSION TRANSACTION READ ONLY`.
- User-provided catalog/table names used in internal metadata queries are encoded as hex SQL expressions rather than interpolated as raw SQL literals.
- The adapter uses only Go stdlib plus the already-vendored TaskDeck dependencies; no MySQL Go driver/npm/pip package was added.

### Phase 5 — Generic SSH tunnel manager

1. [x] Define tunnel spec/lifecycle.
2. [x] Reserve an ephemeral IPv4 loopback port until immediately before OpenSSH starts binding it.
3. [x] Start OpenSSH `-N -L` with loopback binding and `ExitOnForwardFailure=yes`.
4. [x] Reuse the existing SSH profile authentication and one-time askpass broker.
5. [x] Detect readiness by the local forwarding listener and report early SSH exits.
6. [x] Kill/reap tunnels on explicit close, manager shutdown, startup cancellation and DB-owned cleanup.
7. [x] Bound SSH tunnel diagnostics to 64 KiB.
8. [x] Add controlled process/manager tests for ready, early exit, timeout, cleanup and askpass environment.
9. [x] Evaluate OpenSSH multiplexing and defer it; correctness and isolated lifecycle ownership remain preferable for the first tunnel release.

Implementation notes:

- Tunnel forwards bind to `127.0.0.1` only.
- Port reservation narrows the bind race by holding the selected loopback port until immediately before `ssh` starts.
- Stored SSH secrets are still delivered only through the existing one-time askpass socket/token path.
- Inherited askpass control variables are sanitized before controlled tunnel auth variables are injected.

### Phase 6 — MySQL through SSH tunnel

1. [x] DB profile selects direct or SSH tunnel.
2. [x] Resolve and validate the referenced SSH profile on the server.
3. [x] Start the generic SSH tunnel before the DB adapter.
4. [x] Pass only the effective loopback host/port to the MySQL adapter.
5. [x] Transfer tunnel cleanup ownership to the DB session and tear it down on connect failure, disconnect, adapter exit or daemon shutdown.
6. [x] Add Connections UI for direct/SSH-tunnel transport, SSH profile selection and remote DB host/port.
7. [x] Add end-to-end lifecycle tests with controlled SSH and DB adapter stubs.

Implementation notes:

- DB adapters remain unaware of SSH internals.
- A tunnel profile stores the remote DB endpoint; the adapter receives only the local loopback forwarding endpoint.
- Missing/unavailable SSH profiles are rejected server-side even if a browser bypasses the UI.

### Phase 7 — Redis

1. [x] Implement bounded RESP2 codec and persistent TCP client in pure Go stdlib.
2. [x] Add built-in `redis-go` adapter with AUTH, SELECT, PING and direct connections.
3. [x] Add bounded SCAN key browser, TYPE/TTL detail and safe key-type previews.
4. [x] Reuse the generic database query workspace with Redis-aware defaults (`PING`, key labels, Redis profile options).
5. [x] Reuse the generic direct/SSH-tunnel transport path; Redis adapter receives only the effective host/port and contains no SSH code.
6. [x] Add codec, config, TCP client, command allowlist, result normalization, handler and UI regression tests.

Implementation notes:

- The initial Redis adapter is deliberately read-oriented even when a profile is marked read/write.
- Arbitrary Redis commands are not exposed. The adapter uses an explicit allowlist for non-destructive reads such as GET/HGET/LRANGE/SCAN/TYPE/TTL.
- Commands such as SET, DEL, FLUSHALL, CONFIG, EVAL and MULTI are rejected inside the adapter before they reach Redis.
- RESP lines, bulk values, arrays, nesting, browse key counts and normalized browser results are bounded.

### Phase 8 — MongoDB

1. [ ] `mongosh` discovery.
2. [ ] EJSON machine-output adapter.
3. [ ] Database/collection discovery.
4. [ ] Bounded query execution.
5. [ ] SSH tunnel reuse.
6. [ ] Tests.
7. [ ] Reassess standalone Go-driver adapter only if CLI limitations are material.

### Phase 9 — SQLite

1. [ ] Python stdlib `sqlite3` adapter/helper.
2. [ ] File/path validation.
3. [ ] Schema browser.
4. [ ] Bounded query execution.
5. [ ] Explicit read/write mode.
6. [ ] Tests.
7. [ ] Remote SQLite remains separate/non-tunnel work.

### Phase 10 — Hardening

1. [ ] Secret rotation/recovery documentation.
2. [ ] Process crash cleanup.
3. [ ] Resource/row/output limits.
4. [ ] Audit hooks.
5. [ ] Shared-server RBAC integration when that architecture reaches the target branch.
6. [ ] Review all browser APIs for secret disclosure.
7. [ ] Review SSH host-key policy.
8. [ ] Review tunnel bind scope.
9. [ ] Offline/self-update build verification.
10. [ ] User documentation.

## 18. Acceptance criteria

### SSH

- Existing local terminal still behaves as before.
- User can create/edit/delete saved SSH profiles.
- User can open a remote terminal from a saved profile.
- Agent/private-key/password auth paths do not expose credentials.
- Host-key verification is not disabled by default.
- Optional custom home directory works.
- Optional preset commands run after connection establishment.
- Remote terminals remain normal TaskDeck terminal tabs.
- Left-side connection UX provides convenient access to local/SSH targets.

### MySQL

- Direct profile can connect, browse schema and execute bounded queries.
- Password is not exposed to argv/browser/logs.
- Missing MySQL client is reported clearly.
- Adapter-specific code is isolated from TaskDeck core.

### SSH tunnel

- MySQL can connect through a saved SSH profile.
- Tunnel binds to loopback by default.
- Tunnel failure is detected.
- Tunnel is cleaned up on disconnect/shutdown.
- DB adapter is unaware of SSH internals.

### Other adapters

- Redis uses no third-party client library unless a later documented blocker requires it.
- Mongo implementation does not add npm dependencies to TaskDeck.
- SQLite uses no new Go SQLite dependency while the stdlib Python helper is viable.
- TCP-based adapters can reuse the same tunnel manager without database-specific SSH code.

## 19. Design invariants

- Go-first.
- No npm package when a viable non-npm solution exists.
- Third-party dependencies are exceptional and justified.
- OpenSSH is the SSH implementation boundary.
- DB adapters are process-isolated and capability-driven.
- SSH tunnel is transport, not DB-specific logic.
- Secrets are referenced and protected, never returned as ordinary profile fields.
- Existing local Terminal workflow is preserved.
- Remote terminals still use the current terminal-tab experience.
- UI convenience never replaces backend security.
- Small commits are mandatory.
