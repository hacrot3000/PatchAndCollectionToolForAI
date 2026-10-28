# TaskDeck Remote Portal — Phase 1

**Development only. NOT READY for deployment as an Internet remote-control service.**

This initial implementation supplies the shared-hosting PHP 8.1+ / PDO SQLite3 / Sodium control plane with Portal user accounts, password-hashed authentication, admin-created users, machine enrollment/claim, device-scoped grants, signed heartbeat and a responsive UI.

**Protocol update:** Portal issues only signed browser tickets **v2**. Generate the browser's private P-256 ephemeral key and 128-bit nonce first; POST `device_id`, `client_nonce`, `browser_key` and CSRF to `/api/remote/ticket`. Relay and Agent enforce a signature-bound browser-key fingerprint; v1 tickets and substituted keys are rejected. An Agent with no locally pinned Portal public signing key will reject the diagnostic handshake. The three E2EE echo frames still grant no TaskDeck login or command access.

**Implemented in this development branch:** Portal, integrated TaskDeck Settings, outbound Go Agent, WSS Relay with challenge-response device proof, Portal-signed short leases, signed one-time browser tickets and a safe browser-to-Agent reachability diagnostic. **Experimental login-only diagnostic:** an opt-in strict AES-GCM browser-to-Agent credential frame calls the real TaskDeck SQLite/scrypt verifier and returns only an encrypted boolean. The Portal UI and Relay carry this frame **only** when all experimental flags are explicitly enabled. No TaskDeck session survives the check. No TaskDeck cookies, access tokens, HTTP actions or terminal commands are available. **Earlier internal preflight:** TaskDeck shared-server login verification using real SQLite/scrypt, channel-bound 5-minute session and per-action RBAC. The verifier is not connected to Browser or Agent yet. **Not implemented:** interactive TaskDeck remote sessions, HTTP/WS proxying, WebAuthn/passkeys (Portal TOTP is implemented), E2EE **for application traffic** (only the fixed encrypted diagnostic ready proof and three bounded echo rounds are wired), immediate live-session revocation or account recovery. Consequently there is **no remote execution endpoint**.

**Trust-root setup for future Remote access:** run `php remote_portal/bin/admin.php relay-public-key` using the trusted Portal administrator CLI. Copy the resulting base64 Ed25519 **PUBLIC** key into each TaskDeck instance's authenticated Settings → Security → Remote Access → Portal signing public key. Do not copy the private signing seed, and never accept a substituted verification key from Relay. This enables Agent-side signature verification only; it does **not** activate remote TaskDeck login or execution.

See [architecture](../docs/TASKDECK_REMOTE_ACCESS_ARCHITECTURE.md) and [security risks](../docs/TASKDECK_REMOTE_SECURITY_RISK_ASSESSMENT.md). Do not bypass the existing TaskDeck authentication to expedite connectivity.

## Shared-host setup

Requirements: PHP 8.1+, pdo_sqlite, sodium, TLS. Configure document root to **remote_portal/public ONLY**. Put private DB and PHP session storage outside webroot. Apache needs mod_rewrite and AllowOverride for the provided public/.htaccess; configure equivalent rewrite on nginx/LiteSpeed. Deny direct access to all private files.

Set REMOTE_PORTAL_DB_PATH to an **absolute** file path in a writable private directory outside webroot, for example /home/account/private/taskdeck-remote/portal.sqlite3.

**Required storage permissions (security gate):** The Portal refuses to open a database in a group/world-accessible directory, a symlinked directory, or a database that is world/group-readable or symlinked. Create the directory with `chmod 700 /home/account/private/taskdeck-remote` and set an existing database to `chmod 600 /home/account/private/taskdeck-remote/portal.sqlite3` before restarting PHP-FPM. New databases are created owner-only using an exclusive file creation and restrictive umask. Keep the private PHP session directory protected as well. The directory itself must be owned/writable by the PHP execution account; this also protects SQLite journal files. If the new check reports an error on an existing installation, correct filesystem permissions **rather than bypassing the guard**. Do not place Portal storage under the repository or the public webroot.

Initialize from CLI:

    php remote_portal/bin/admin.php init
    php remote_portal/bin/admin.php create-admin sysadmin

Existing installations must rerun `php remote_portal/bin/admin.php init` once after updating this branch. This preserves all Portal accounts and grants while adding an `auth_epoch` counter for browser-session revocation. Portal users can change their **Portal** password in their account panel; it does not touch TaskDeck credentials.

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

The CI workflow runs these checks plus signed ticket/lease tests and browser JavaScript syntax validation. Production requires integration/security tests and all risk release gates.

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

This **standalone CLI probe** is separate from the integrated Agent in TaskDeck Settings. It exists to exercise the signed enrollment and heartbeat protocol manually. Use one private absolute identity path per TaskDeck instance; never place it in Git or share a credential file between projects.

From the repository:

    cd vscode_tasks_menu_go
    go run ./cmd/taskdeck_remote_probe -action enroll -portal https://YOUR-PORTAL-DOMAIN -identity /home/USER/.config/taskdeck-remote/project-A.seed -name "Project A"

The command prints a Device ID and a temporary pairing code. Enter both on the Portal. After claiming, send a signed heartbeat:

    go run ./cmd/taskdeck_remote_probe -action heartbeat -portal https://YOUR-PORTAL-DOMAIN -identity /home/USER/.config/taskdeck-remote/project-A.seed

A single heartbeat shows "Online" for approximately 60 seconds. Continuous presence is already integrated with the authenticated TaskDeck daemon when Remote is enabled; do not confuse this probe with a connected TaskDeck Remote session.

## Integrated TaskDeck Settings (current branch)

The TaskDeck Go daemon now supports an **opt-in outbound signed presence** service, and the existing browser Settings menu has **Remote Access…** under the Security section (project admin only in Multi-user mode; local Basic Auth is required in single-user mode).

1. Open TaskDeck → Settings → Security → Remote Access.
2. Provide the HTTPS Portal origin and enable outbound presence. Saving modifies only the [remote_access] section of the project INI and schedules a TaskDeck web daemon reload; the independent terminal broker remains separate.
3. Reopen Remote Access after TaskDeck has reloaded. Click **Generate pairing code**, then enter the Device ID and pairing code on the PHP Portal while logged in as its administrator.
4. Once claimed, TaskDeck heartbeats every 20 seconds while its local daemon is healthy. The Portal reports online while a valid heartbeat remains fresh (up to approximately 60 seconds).

If TaskDeck authentication is off, enabling Remote is rejected. The key is stored in the OS user configuration directory outside the workspace, using a stable per-workspace fingerprint. The Portal never receives the key or TaskDeck password.

**This is presence, pairing and read-only reachability testing only.** An optional **Kiểm tra Agent** button can confirm that an authorized browser reached the live Agent through Relay, without opening TaskDeck. Set REMOTE_PORTAL_SIGNING_SEED_PATH, REMOTE_PORTAL_ISSUER, and REMOTE_PORTAL_RELAY_ORIGIN on PHP hosting and explicitly set REMOTE_PORTAL_EXPERIMENTAL_TICKETS=1 only in a controlled test environment. The Relay must use the matching Portal public verification key. The Agent must be configured with the same Relay HTTPS origin. On successful heartbeat the Portal issues a 70-second signed device lease; Relay verifies that lease on connection and on every keepalive. The browser now requires a signed per-session P-256 ECDH handshake, decrypts the ready proof, sends three sequential AES-GCM-encrypted random nonce challenges through Relay to Agent, and verifies all encrypted echo responses before reporting success. Relay sees only ciphertext; no TaskDeck application handler exists in this path. This **does not carry TaskDeck API or user commands**; both HTTP/WS transport and TaskDeck-side authorization are still blocked.

## Remaining release blockers

- HTTP/WebSocket session proxy, full browser transport and hardened TaskDeck-side remote authentication + authorization.
- Phishing-resistant WebAuthn/passkeys, edge rate limiting, key rotation/recovery and independent review.
- Verified TaskDeck-side per-action authorization and strict project/instance isolation.
- Revocation of active sessions; extension of proven diagnostic E2EE to all application traffic; frontend JS trust model.
- Production deployment tests on actual shared hosting and private storage permissions.

## Internal encrypted-login validation (not user-facing)

The experimental E2EE login message uses a fixed, bounded JSON payload and the authenticated P-256/AES-GCM browser-Agent keys. Browser WebCrypto and Go tests confirm that passwords are not visible to Relay, that incorrect passwords return an authenticated false result, and that replayed or tampered ciphertext is rejected. This functionality is now wired as an **explicit, authentication-only diagnostic**, not as a TaskDeck session or control API. It requires **both** `REMOTE_PORTAL_EXPERIMENTAL_LOGIN=1` plus `REMOTE_PORTAL_EXPERIMENTAL_TICKETS=1` on PHP Portal, and independently `TASKDECK_REMOTE_LOGIN_PROBE=1` on TaskDeck daemon host. With the second flag off, Agent registers no password-check callback. With either Portal flag off, the login-only form is not rendered. Portal receives only the signed-ticket request without TaskDeck credentials; Relay receives only encrypted password data, cannot decrypt it and returns only encrypted success/failure. A successful password check closes the temporary TaskDeck session and cannot open HTTP, terminal, editor or any project file.

Developer-only interoperability test:

    node remote_portal/tests/browser_login_crypto.test.cjs

## Browser login/handshake denial does not drop Agent

For a malformed encrypted TaskDeck login or rejected Agent-side signed Portal grant, Relay cancels **only that Browser reservation**. The Agent's authenticated outbound WSS connection remains active, allowing a new separately authorized Browser session without a forced Agent reconnect. A stale rejection cannot terminate a newer Browser session. The rejection packet contains no TaskDeck credentials, Portal ticket or plaintext error details. CI covers these cases with real TLS/WebSocket integration tests.

These protections do **not** activate Remote Control, issue login cookies or remove the requirement for explicit opt-in on both Portal and TaskDeck host.

## Agent-side experimental password-check rate limit

When both experimental flags are set, each TaskDeck Agent permits at most **5** password-verification probes in an initial burst and refills **1 token every 12 seconds** (5/minute sustained), across all Portal users, target usernames and browser tickets. Excess attempts are rejected without calling SQLite/scrypt and without disconnecting the Agent. The shared login-attempt budget is persisted in a private per-Agent identity ledger and synchronized across processes with a file lock; daemon restarts do **not** replenish exhausted tokens. Keep the ledger directory owner-only and treat missing/corrupt/unwritable storage as denial. The usual TaskDeck per-account failed-login limits continue to apply. Portal edge throttling and durable abuse controls are still required before production rollout.

## Portal ticket issuance quotas and upgrading an existing installation

The Portal now uses the private SQLite database for **durable, atomic** signed-ticket quotas: 12 tickets in a 60-second window per Portal account (across devices), and 24 tickets in a 60-second window per device (across users). Authorization is checked and both counters are reserved under one SQLite `BEGIN IMMEDIATE` transaction to prevent concurrent PHP worker races. An exhausted quota produces HTTP 429 and `Retry-After: 60`. Invalid nonce or public-key encoding is rejected before consuming a ticket slot. No raw IP, username, password, browser key, nonce or ticket is stored in this ledger.

**Upgrade:** after deploying updated PHP code, rerun the private CLI `php remote_portal/bin/admin.php init` with `REMOTE_PORTAL_DB_PATH` pointing at the existing database. The migration is additive, and previous user/device data remains in place. Without the new table ticket issuance fails closed with HTTP 503. CI runs a real concurrent multi-process PHP/SQLite quota regression test.

These quotas are independent of the Agent's **durable** 5-attempt E2EE password-check budget. Just before the experimental password verifier runs, Agent now makes a signed HTTPS Portal grant-check which validates current permissions and the account's password-rotation `auth_epoch`, and fails closed if Portal is unreachable. This prevents revoked or password-rotated Portal users from invoking new password probes with older signed tickets. It is **not** a general-purpose active-session revocation protocol. Edge ingress throttling, MFA and immediate cancellation of future TaskDeck application sessions remain release blockers. Do not enable production Remote Control.

## Encrypted echo failure handling

Malformed, replayed or unauthentic diagnostic echo frames are rejected with a fixed `echo_rejected` signal. Relay releases only that Browser reservation, not Agent's authenticated outbound WSS connection. The error carries no TaskDeck command, credential, Portal ticket or plaintext frame. A new Browser session can proceed without waiting for Agent to reconnect; CI verifies this over real TLS/WebSocket and a forged `terminal.run` echo. As before, **no command is executed**.


## Live account/grant revocation and Agent-side persistent quota

Experimental TaskDeck password-only authentication now requires **current Portal authorization**, not just a valid signed short-lived ticket. Agent sends a fresh Ed25519-signed HTTPS request to \`/api/agent/grant-check\`. Portal compares current user-device membership, role, enabled status, device presence and the Portal account's \`auth_epoch\` (incremented when its password changes). This means revoking a grant, downgrading its role, disabling a user/device or rotating the Portal password denies the next password check, even if the old 30-second ticket has not expired. Portal outages deny the check; they do not open a fallback login.

On the TaskDeck host, the Agent login-only scrypt budget is now a private persisted local ledger bound to the device identity and synchronized with a file lock. A daemon restart does not replenish it. State corruption or unsafe permissions block password checks. Portal's separate SQLite per-user/device ticket quotas remain in force.

In authenticated TaskDeck Settings, the new **Check Portal connection** button sends a fresh signed heartbeat and reports whether the Portal recognizes the claimed device. The operation is admin-only and does not grant TaskDeck application access.

**Still not production Remote Control:** the current E2EE login probe returns only an encrypted boolean and immediately closes its TaskDeck session. There is no remote HTTP/WS or command dispatcher, continuous active-session revocation or completed origin-isolation review. Portal ticket introspection protects each experimental login attempt, not future long-running streams.


### Brute-force protection and shared-host ingress quota

Portal authentication is protected at two independent points. First, every `POST /login` reserves a durable SQLite ingress slot **before** looking up or verifying any user password; `POST /api/agent/enroll` reserves a separate slot **before** signed enrollment work. Limits: **login 10/min per network source and 180/min globally**, enrollment **12/min per network source and 120/min globally**. All PHP workers share the same 60-second accounting; a full quota returns HTTP 429 with a 60-second Retry-After. Database/key failures return 503 and fail closed. The site-wide cap bounds the total number of HMAC source rows even against rotating IPv6 addresses.

Second, `Store::authenticate()` now reserves each real account's password attempt inside `BEGIN IMMEDIATE` **before** the expensive password verification; five bad attempts lock the account for two minutes. Parallel requests from distinct PHP workers cannot race past that threshold. Counters are never created for nonexistent Portal users, preventing unbounded username-driven database growth. Unknown-user paths perform a dummy password verification for basic timing equalization, within the ingress budget.

The Portal **never stores the raw client IP** in its SQLite rate ledger: it stores only HMAC-SHA256 values derived from `REMOTE_ADDR` and an independent 32-byte secret at `<private-db-directory>/portal-abuse-hmac.key`. This owner-only key file is generated or checked by the idempotent CLI `php remote_portal/bin/admin.php init`; it must have mode 0600 and must stay outside the public document root. Do not put it in a repository, backup accessible to the public, or diagnostics. Keep the key stable across PHP processes/restarts; rotating it resets per-source key identity, so require an operator change window and edge-level throttling for rotations. Run the CLI migration on every existing deployment before activating the updated Portal, otherwise the ingress endpoints intentionally fail closed until the HMAC key and table exist.

**Reverse proxy / CDN:** Application throttles deliberately use the direct socket's `REMOTE_ADDR` only and ignore incoming `X-Forwarded-For` and `X-Real-IP` values, because clients can forge them. On shared hosting behind a proxy, all users may share a proxy address and hit the same source quota. Configure the *trusted hosting/webserver* to rewrite `REMOTE_ADDR` from a securely defined trusted-proxy list, and enforce equivalent limits at the edge; never trust forwarding headers sent by the public. This mechanism reduces password guessing and storage exhaustion but cannot replace MFA/passkeys, provider-side volumetric DDoS controls, or a proper account recovery protocol.

Validation:

    php remote_portal/tests/bruteforce.php
    php remote_portal/tests/smoke.php
    php remote_portal/tests/storage_permissions.php

The brute-force regression covers source/global limits, IPv4/IPv6 HMACs, no raw IP persistence, 60-second expiry, SQLite persistence across independent PHP processes and 24 simultaneous guesses against the same account.


### Signed Agent proof rate limit (paired devices)

A separate durable control protects the Portal's `device_nonces` replay database after a Device ID has been paired. Across both signed `/api/agent/heartbeat` and authorized `/api/agent/grant-check`, a paired Device ID may consume at most **90 accepted signed proofs per 60 seconds**. These proofs reserve quota inside the same SQLite `BEGIN IMMEDIATE` transaction before inserting a replay nonce, so concurrent PHP workers cannot bypass the limit. Unknown/disabled devices and denied/revoked/stale-grant checks do not allocate nonce records. The quota row is removed when a device is unlinked, and expiry pruning is indexed.

**Migration required:** deploy the updated Portal PHP and run the private administrator CLI with the existing SQLite DB path:

    REMOTE_PORTAL_DB_PATH=/absolute/private/directory/portal.sqlite3 php remote_portal/bin/admin.php init

This creates the `signed_agent_limits` table and the nonce-expiration index without discarding users, permissions or pairing state. Do not bypass the migration; signed heartbeats fail closed when the table is missing. Verify with `php remote_portal/tests/signed_agent_limits.php` and the full CI workflow. A valid signature and a normal heartbeat are not permission to perform TaskDeck actions. Edge-level rate limits are still needed to defend against unauthenticated volumetric traffic.


## Default-on Portal MFA and offline recovery

Portal account MFA is now implemented using RFC 6238 TOTP (six digits, SHA-1, 30 seconds). A logged-in user must re-enter their Portal password, register the displayed 160-bit secret in an Authenticator app and submit one valid TOTP code before MFA is stored. The pending secret expires from the PHP session after five minutes. The confirmed secret is stored in SQLite encrypted with libsodium secretbox, never as plaintext. The separate 0600 32-byte key is `<private DB directory>/portal-mfa.key`, outside the webroot.

**MFA is enforced by default.** Unless `REMOTE_PORTAL_REQUIRE_MFA=0` is explicitly set (isolated local development ONLY), an unenrolled password-only Portal session is restricted to MFA enrollment and logout. Device lists, grants, signed tickets and privileged POST actions are inaccessible in this state. Once enrolled, password-only login is rejected regardless of this environment setting. Each successful TOTP timestep is consumed atomically and cannot be reused across sessions or concurrent workers. Enrolling or resetting MFA advances account `auth_epoch` and invalidates previous sessions and older signed user tickets. A user may have to wait until the next 30-second code before doing another sensitive operation.

Authenticated password changes and MFA enrollment have an independent durable per-account limit of **five attempts in ten minutes**, shared across sessions and PHP workers, reserved before password verification. This complements Portal login and Agent brute-force protection. TOTP is **not phishing-resistant**; WebAuthn/passkeys and independent penetration testing remain release blockers for high-assurance Internet deployment.

**Existing installation upgrade:** Before activating the updated public PHP files, run:

    REMOTE_PORTAL_DB_PATH=/absolute/private/portal.sqlite3 php remote_portal/bin/admin.php init

This additive migration creates MFA columns, `account_sensitive_limits`, and `portal-mfa.key` without changing existing user passwords, device grants or TaskDeck credentials. Back up the original private MFA key alongside SQLite. Init refuses to silently replace a lost MFA key while users are enrolled, or a lost ingress HMAC key while per-source quotas exist; restore the original key from private backup. Never put either secret in Git or public storage.

**Authenticator loss:** a trusted operator with private host CLI access can use:

    REMOTE_PORTAL_DB_PATH=/absolute/private/portal.sqlite3 php remote_portal/bin/admin.php reset-totp USERNAME --confirm

This creates an audit event, invalidates Portal sessions via `auth_epoch`, and forces MFA reenrollment before privileges are restored. There is intentionally no public password-reset or MFA-reset web endpoint. Operators must verify account ownership out of band. Bootstrap the initial administrator over verified HTTPS on a trusted device and enroll MFA immediately; before enrollment a stolen password could otherwise let an attacker register their own second factor.

Verification:

    php remote_portal/tests/sensitive_limits.php
    php remote_portal/tests/mfa.php
    php remote_portal/tests/mfa_http.php

The last test exercises the actual loopback Portal HTTP UI and confirms that unenrolled sessions cannot bypass MFA through direct privileged API POST requests. None of this enables TaskDeck Terminal, File, Git or Patch control through Relay.


### Additional API flood and administrator mutation limits

On top of the existing authenticated 90/minute paired-Device ID budget, **all anonymous HTTP requests to `/api/agent/heartbeat` and `/api/agent/grant-check`** reserve an atomic pre-signature token before JSON processing and Ed25519 verification. Limits are **900/minute per trusted socket source IP** and **9000/minute across the Portal**, returning HTTP 429/Retry-After 60; SQLite/key failures fail closed. The rate ledger stores only HMAC-SHA256(source IP), using the private `portal-abuse-hmac.key`. Large shared NAT deployments should size/test their edge policy carefully. Client-controlled `X-Forwarded-For` cannot override the source.

Administrative POST mutations—create user, claim/unlink device, grant/revoke role—are limited to **60/minute per enabled admin Portal account**. SQLite performs the current admin-status check and quota reservation under the same write lock, across PHP workers and browser sessions. This reduces damage from a stolen session but does not substitute for MFA, CSRF protection or action-level authorization.

**Required database migration for existing installs:** rerun the same private `admin.php init` against the existing `REMOTE_PORTAL_DB_PATH`. The new `portal_agent_ingress_limits` and `portal_admin_limits` tables are strictly additive; old `portal_ingress_limits` login/enroll CHECK constraints stay intact. The endpoints deliberately fail closed before migration. CI validates both with:

    php remote_portal/tests/bruteforce.php
    php remote_portal/tests/admin_limits.php

The real Portal HTTP entrypoint also needs `require_once` for the TOTP class because `Store.php` loads it internally; CI covers the resulting MFA HTTP startup. Portal TOTP MFA is now implemented for enrollment/login and private CLI recovery (see the MFA section above). Passkeys and complete Remote Control session authorization are **not** implemented; keep experimental remote execution disabled.


### Browser-session allocation guard (DoS prevention)

The Portal now **rejects unsupported browser paths/methods before `session_start()`**, and caps accepted browser GET/POST traffic at **120 requests per source per 60 seconds** and **2,400 requests globally per 60 seconds**, regardless of CSRF correctness or whether the browser has a cookie. A separate SQLite `portal_browser_ingress_limits` table is required (additive migration via `php bin/admin.php init`), and the original source IP is represented only by HMAC-SHA256 in the database. Excess traffic receives HTTP 429; DB/key failures return HTTP 503 rather than allocating a session without a quota. The tighter login quota applies independently before session allocation.

This mitigates PHP session-file exhaustion from anonymous GET, forged CSRF POST and unknown URLs on shared hosts. It does not substitute for upstream WAF/reverse-proxy DDoS controls. Test with `php remote_portal/tests/browser_ingress.php` (12 PHP worker races) and `php remote_portal/tests/mfa_http.php` (unsupported anonymous routes must not create session files).


### Protect the ingress HMAC key from silent rotation

The 32-byte owner-only `portal-abuse-hmac.key` is now paired with a non-secret SHA-256 fingerprint in SQLite table `portal_key_checks`. Private-file loss or substitution fails closed on every request; re-running `php remote_portal/bin/admin.php init` does not silently replace an already-pinned key and reset per-IP rate-limit identities. On first installation or an older DB with no fingerprint the CLI registers the existing key (or generates an initial key, if needed). Keep the private key in protected offline backups and restore the original bytes after disk corruption or relocation. Do not edit `portal_key_checks` to bypass a mismatch. The fingerprint is not a TaskDeck password, IP address or authentication token.

Run `php remote_portal/tests/bruteforce.php` and `php remote_portal/tests/admin_limits.php` to verify both key continuity and durable account/site quotas.


### Private PHP browser session storage (required on shared hosting)

Portal browser sessions contain the current Portal account ID/auth epoch, a CSRF secret and, for up to five minutes during TOTP enrollment, a **pending MFA seed**. This release no longer trusts the host's default `session.save_path` or a shared Redis/custom session handler. Before `session_start()`, it forces PHP's `files` session handler and a dedicated **per-Portal database** `browser-sessions-<sha256-prefix>/` folder **inside the protected `REMOTE_PORTAL_DB_PATH` parent directory** (not the public document root). The application creates the directory with mode 0700 on first use; existing non-private folders, files/symlinks or unwritable storage fail closed with HTTP 503. PHP-created session files must be owner-only (normally 0600). The DB and its companion HMAC/MFA keys are unchanged. Two Portal SQLite databases in the same parent directory receive different session folders, preventing cross-instance account/CSRF/TOTP state reuse.

**Upgrade impact:** users signed in before this change will have to log in again, because their old PHP session storage will no longer be read. Do not copy old session files out of a shared system directory. Ensure that the web PHP worker can create/write the per-database `browser-sessions-<sha256-prefix>` under the private DB directory, with the same service identity that owns SQLite. On hosts where PHP disables setting the session save handler/path, the Portal intentionally refuses to serve browser sessions. This folder name uses the first 32 hex digits of SHA-256 of the canonical absolute SQLite DB path; the Portal creates it automatically as the web PHP owner. If the operator must precreate it, calculate the exact name and use mode 0700 under the same PHP host identity. Do **not** place the directory in the webroot or grant group/world access.

Tests `php remote_portal/tests/storage_permissions.php` and `php remote_portal/tests/mfa_http.php` verify path isolation, symlink/public-read rejection and a live PHP HTTP request whose host-level session.save_path points to a shared temporary directory. Agent heartbeat/grant endpoints remain sessionless.
