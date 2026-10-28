# TaskDeck Remote Portal — Phase 1

**Development only. NOT READY for deployment as an Internet remote-control service.**

This initial implementation supplies the shared-hosting PHP 8.1+ / PDO SQLite3 / Sodium control plane with Portal user accounts, password-hashed authentication, admin-created users, machine enrollment/claim, device-scoped grants, signed heartbeat and a responsive UI.

**Not implemented:** Go Relay, TaskDeck Settings integration, actual Agent connection from TaskDeck, remote browser sessions, second-factor authentication, E2EE, reliable revocation of live sessions, secure password recovery/rotation. Consequently there is **no Remote button or remote execution endpoint** in this phase.

See [architecture](../docs/TASKDECK_REMOTE_ACCESS_ARCHITECTURE.md) and [security risks](../docs/TASKDECK_REMOTE_SECURITY_RISK_ASSESSMENT.md). Do not bypass the existing TaskDeck authentication to expedite connectivity.

## Shared-host setup

Requirements: PHP 8.1+, pdo_sqlite, sodium, TLS. Configure document root to **remote_portal/public ONLY**. Put private DB and PHP session storage outside webroot. Apache needs mod_rewrite and AllowOverride for the provided public/.htaccess; configure equivalent rewrite on nginx/LiteSpeed. Deny direct access to all private files.

Set REMOTE_PORTAL_DB_PATH to an **absolute** file path in a writable private directory outside webroot, for example /home/account/private/taskdeck-remote/portal.sqlite3.

Initialize from CLI:

    php remote_portal/bin/admin.php init
    php remote_portal/bin/admin.php create-admin sysadmin

Enter the long admin password on STDIN at the prompt, never in a process argument. For shell-only shared hosts, ensure environment variable is set in PHP-FPM as well as CLI. PHP sessions and SQLite files require restrictive OS permissions; check webserver file exposure with a deliberate negative test.

### Local development only

    export REMOTE_PORTAL_DB_PATH="$(mktemp -d)/portal.sqlite3"
    php remote_portal/bin/admin.php init
    php remote_portal/bin/admin.php create-admin sysadmin
    REMOTE_PORTAL_DEV_HTTP=1 php -S 127.0.0.1:8088 -t remote_portal/public remote_portal/bin/dev-router.php

Open http://127.0.0.1:8088. The HTTP exception is loopback-only on port 8088. Never set REMOTE_PORTAL_DEV_HTTP on the public hosting server.

### Tests

    find remote_portal -name '*.php' -print0 | xargs -0 -n1 php -l
    php remote_portal/tests/smoke.php

The CI workflow performs these checks; tests are only for the Portal store. Production requires integration/security tests and all risk release gates.

## Enrollment wire protocol v1

Future TaskDeck Agent generates Ed25519 keypair (private key remains on client) and 16 random pairing bytes displayed as lowercase 32-char hex on TaskDeck. Device ID = lowercase hex SHA-256 of the raw Ed25519 public key (32 bytes). The server receives ONLY the hash SHA-256 of the raw 16 pairing bytes, not plaintext, from the Agent.

POST /api/agent/enroll JSON fields:
- device_id: lowercase 64-char hex
- public_key: base64 of raw 32-byte Ed25519 public key
- display_name: 1–80 bytes, no control characters
- pairing_hash: lowercase hex SHA-256 of 16 raw pairing bytes
- timestamp: Unix seconds (JSON integer)
- nonce: fresh random lowercase 32–64 hex characters
- signature: base64 Ed25519 detached signature

Message bytes to sign, separated by literal newline bytes, with NO trailing newline:

    taskdeck-remote-v1
    enroll
    {device_id}
    {display_name}
    {pairing_hash}
    {timestamp}
    {nonce}

POST /api/agent/heartbeat has device_id/public_key/timestamp/nonce/signature and signs:

    taskdeck-remote-v1
    heartbeat
    {device_id}
    {timestamp}
    {nonce}

A signed enrollment is **pending** only. Portal admin must type device_id AND the local pairing code on the website within 5 minutes before the device can heartbeat. Clock skew ±60s, nonce is consume-once for replay defense, presence TTL 60s, JSON body <=4096 bytes. Device fingerprint must match public key and Ed25519 signature. Re-pair and rotation deliberately fail closed until specifically implemented.

SQLite users contain **Portal password hashes**, not TaskDeck credentials. Devices contain ID, name, enabled/creation time, never host, port, IP, token, cookie or TaskDeck private key. Portal and relay HTTP access logs may nevertheless include transport addresses, requiring operational privacy review.

## Developer-only Go Agent protocol probe

This probe is NOT yet connected to TaskDeck Settings or the TaskDeck lifecycle. It exists to exercise the signed enrollment and heartbeat protocol only. Use one private absolute identity path per TaskDeck instance; never place it in Git or share a credential file between projects.

From the repository:

    cd vscode_tasks_menu_go
    go run ./cmd/taskdeck_remote_probe -action enroll -portal https://YOUR-PORTAL-DOMAIN -identity /home/USER/.config/taskdeck-remote/project-A.seed -name "Project A"

The command prints a Device ID and a temporary pairing code. Enter both on the Portal. After claiming, send a signed heartbeat:

    go run ./cmd/taskdeck_remote_probe -action heartbeat -portal https://YOUR-PORTAL-DOMAIN -identity /home/USER/.config/taskdeck-remote/project-A.seed

A single heartbeat shows "Online" for approximately 60 seconds. Continuous presence must be wired into the authenticated TaskDeck service in a later phase; do not confuse this probe with a connected TaskDeck Remote session.

## Integrated TaskDeck Settings (current branch)

The TaskDeck Go daemon now supports an **opt-in outbound signed presence** service, and the existing browser Settings menu has **Remote Access…** under the Security section (project admin only in Multi-user mode; local Basic Auth is required in single-user mode).

1. Open TaskDeck → Settings → Security → Remote Access.
2. Provide the HTTPS Portal origin and enable outbound presence. Saving modifies only the [remote_access] section of the project INI and schedules a TaskDeck web daemon reload; the independent terminal broker remains separate.
3. Reopen Remote Access after TaskDeck has reloaded. Click **Generate pairing code**, then enter the Device ID and pairing code on the PHP Portal while logged in as its administrator.
4. Once claimed, TaskDeck heartbeats every 20 seconds while its local daemon is healthy. The Portal reports online while a valid heartbeat remains fresh (up to approximately 60 seconds).

If TaskDeck authentication is off, enabling Remote is rejected. The key is stored in the OS user configuration directory outside the workspace, using a stable per-workspace fingerprint. The Portal never receives the key or TaskDeck password.

**This is ONLY presence and pairing**. Clicking a device on the PHP Portal does not yet open a Remote session, and no TaskDeck command API is exposed to the Portal or relay. Go Relay, session tickets, TaskDeck-side remote authorization, browser transport, E2EE/MFA and production penetration testing remain release blockers.

## Remaining release blockers

- Go Relay and HTTP/WebSocket session proxy; hardened TaskDeck-side remote session authorization.
- MFA/passkeys, hardened password lifecycle, rate limiting at edge, key rotation/recovery.
- Verified TaskDeck-side per-action authorization and strict project/instance isolation.
- Revocation of active sessions; E2EE if Relay must not read payload; frontend JS trust model.
- Production deployment tests on actual shared hosting and private storage permissions.
