# TaskDeck Shared Server Deployment

This guide applies to `[shared_server] enabled=true`. Legacy single-user Basic
Auth has different deployment rules.

## Security model

TaskDeck shared mode keeps these boundaries:

- one daemon serves exactly one workspace;
- several daemons may share one identity SQLite DB;
- the identity DB and its backups must stay outside every served workspace;
- TaskDeck authentication remains authoritative even when a reverse proxy is used;
- shared mode requires HTTPS on the TaskDeck listener itself;
- forwarded scheme/client-IP headers are not trusted as security evidence;
- browser permissions are enforced by the backend, including WebSocket control;
- internal self-update handoff/detach requires both loopback transport and the
  daemon's private control token from its `0600` runtime state.

Do not expose the identity DB, backup files, runtime state directory, TLS private
key, or daemon control token through the workspace, web server, shared storage,
or a source repository.

## Minimal shared-mode INI

Example for one project daemon:

```ini
[server]
protocol = https
bind = 127.0.0.1
port = 42881
advertise_host = taskdeck-project-a.example.com
open_browser = false
tls_cert = /etc/taskdeck/project-a/backend.crt
tls_key = /etc/taskdeck/project-a/backend.key

[shared_server]
enabled = true
project_id = project-a
identity_db = /var/lib/taskdeck/identity/identity.db
```

Use a different `project_id` and TCP port for each workspace. Several project
daemons may point at the same absolute `identity_db`.

Recommended filesystem permissions on Unix-like hosts:

```text
/var/lib/taskdeck/identity/        0700
/var/lib/taskdeck/identity/identity.db  0600
/etc/taskdeck/project-a/backend.key     0600
```

TaskDeck validates that the identity DB is outside the workspace and refuses
overly broad identity DB file permissions.

## First administrator

After enabling shared mode and before normal use:

```bash
./vscode_tasks_menu --shared-admin-bootstrap admin
```

For non-interactive secret injection through a protected stdin pipe:

```bash
printf '%s\n' "$TASKDECK_INITIAL_PASSWORD" |
  ./vscode_tasks_menu --shared-admin-bootstrap admin --shared-admin-password-stdin
```

Do not put the password in argv, the INI file, shell history, or environment
used by the long-running daemon.

The account password belongs to the global identity, not to only one project.
Changing it from **Account → Change password** revokes all login sessions for
that identity across projects.

If a user cannot sign in to change their own password, a host operator can reset
the global identity from a trusted local terminal:

```bash
./vscode_tasks_menu --shared-password-reset username
```

For protected non-interactive stdin:

```bash
printf '%s\n' "$TASKDECK_RESET_PASSWORD" |
  ./vscode_tasks_menu --shared-password-reset username --shared-password-reset-stdin
```

This reset also revokes every active login session for that identity across all
projects. It is intentionally a host-operator command rather than a project-admin
web action because the user/password record is global while project roles are
project-scoped.

## Reverse proxy requirements

Shared mode does **not** accept plaintext HTTP from a TLS-terminating proxy as
equivalent to HTTPS. The upstream connection from the proxy to TaskDeck must
also use HTTPS because TaskDeck requires a real TLS connection and deliberately
does not trust `X-Forwarded-Proto`.

The proxy must preserve the public `Host` value because TaskDeck performs exact
same-origin checks against the browser's HTTPS Origin. WebSocket upgrade headers
must also be forwarded.

A proxy may add another authentication layer, but it must not replace or bypass
TaskDeck shared authentication.

### nginx example

```nginx
map $http_upgrade $taskdeck_connection {
    default upgrade;
    ''      close;
}

server {
    listen 443 ssl;
    server_name taskdeck-project-a.example.com;

    # Public certificate configuration omitted here.

    location = /api/health {
        # Optional: keep TaskDeck's minimal loopback health endpoint private.
        return 404;
    }

    location / {
        proxy_pass https://127.0.0.1:42881;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $taskdeck_connection;

        # Never let a public client supply TaskDeck's local-control credential.
        proxy_set_header X-TaskDeck-Internal-Control "";

        # TaskDeck intentionally ignores X-Forwarded-Proto/X-Forwarded-For for
        # authentication decisions. Supplying them is optional for other proxy
        # observability, not a security mechanism.

        # Prefer a private CA or pinned backend certificate.
        proxy_ssl_verify on;
        proxy_ssl_trusted_certificate /etc/nginx/taskdeck-backend-ca.crt;
        proxy_ssl_server_name on;
        proxy_ssl_name taskdeck-project-a.example.com;
    }
}
```

If the backend certificate is self-signed, explicitly trust/pin that certificate
in the proxy. Avoid disabling upstream certificate verification in production.

Because TaskDeck does not trust forwarded client-IP headers, audit `client_ip`
behind a reverse proxy records the proxy's direct address rather than the
original public client address. Do not use that audit field as proof of end-user
network origin in this deployment model.

## Direct TLS without a reverse proxy

TaskDeck may bind directly to a non-loopback address in shared mode as long as
`server.protocol=https`. Use a certificate/key appropriate for the hostname
clients use. Firewall the port to the intended network.

For a self-signed deployment, verify the printed SHA-256 certificate fingerprint
through an independent channel before trusting the certificate in a browser.

## Identity backup

Backups use SQLite's online backup API rather than copying `identity.db`,
`-wal`, and `-shm` files.

```bash
./vscode_tasks_menu \
  --shared-identity-backup /var/backups/taskdeck/identity-$(date +%F).db
```

The destination:

- must be outside the workspace;
- must not already exist;
- is created private (`0600` on Unix-like systems);
- is checked with SQLite `quick_check`.

Keep backup retention and off-host copies according to your own recovery policy.
Backup files contain password hashes, session hashes, memberships, permissions,
and audit data and therefore remain sensitive.

## Identity restore

```bash
./vscode_tasks_menu \
  --shared-identity-restore /var/backups/taskdeck/identity-2026-09-26.db
```

Before replacing the current identity state TaskDeck automatically creates a
private pre-restore safety snapshot next to the live identity DB and prints its
path. Restore also uses SQLite's backup API and integrity checks; if restore
fails, TaskDeck attempts to roll back from that safety snapshot.

After a restore, existing browser sessions may disappear or regain the state
contained in the restored DB. Administrators should treat restore as a
security-sensitive operation and verify users, memberships, active sessions and
audit state immediately afterward.

## Multi-project host

Run one process per workspace, for example:

```text
project-a -> 127.0.0.1:42881 -> project_id=project-a
project-b -> 127.0.0.1:42882 -> project_id=project-b
project-c -> 127.0.0.1:42883 -> project_id=project-c
                         \
                          +-- shared /var/lib/taskdeck/identity/identity.db
```

SQLite WAL and busy-timeout handling are used so separate project daemons can
share the identity DB. Do not place the DB on a filesystem whose SQLite locking
semantics are unreliable.

## Operational checks

After deployment verify all of the following:

1. anonymous public requests cannot access the workspace or admin APIs;
2. the public Origin/Host reaches TaskDeck unchanged;
3. WebSocket terminal reconnect and input work through the proxy;
4. users see only sessions/resources allowed by their permissions and ownership;
5. a browser request to self-update `handoff` or `detach` is rejected;
6. identity DB, backups, runtime state and TLS key are not under the workspace;
7. backup and test restore procedures have been exercised before relying on them;
8. each project daemon uses a unique `project_id` and port.
