# TaskDeck Remote — Implementation handoff and deployment guide

> 2026-10-28 · branch feature/taskdeck-remote-access-architecture.
>
> **DEVELOPMENT-ONLY. NO REMOTE CONTROL IS EXPOSED.** This prototype currently implements authentication, pairing, presence and relay rendezvous. It intentionally rejects terminal/file/API forwarding until the remaining security release gates are complete.

## Components added

| Component | Location | State |
| --- | --- | --- |
| PHP management portal | remote_portal/public, src/Store.php | User login + per-instance grants + signed presence |
| Ed25519 Portal ticket signer | remote_portal/src/TicketSigner.php | 30s signed browser ticket and 70s device-presence lease; ticket issuance disabled by default |
| Agent identity/presence | vscode_tasks_menu_go/internal/remoteportal | Ed25519 private identity per workspace, signed enrollment and heartbeat |
| TaskDeck integration | cmd/vscode_tasks_menu/main.go, internal/server/remote_access.go, internal/config/remote.go | Opt-in configuration, administrative pairing UI, daemon-managed heartbeat |
| Go Relay | internal/remoterelay, cmd/taskdeck_remote_relay | WSS challenge-response v2, signed device lease, one-time ticket and live browser↔Agent reachability probe; **TaskDeck traffic forwarding disabled** |
| Web UI | web/featuremods/remoteaccess.js, menus.js, next.js | Desktop + mobile Settings → Security → Remote Access |
| Portal reachability UI | remote_portal/public/remote_probe.js, remote_crypto.js | Browser verifies Agent identity **and decrypts an E2EE ready proof** using WebCrypto; never a remote-control button |
| E2EE envelope core | internal/remotetransport | P-256 ephemeral ECDH, Agent Ed25519 session signature, AES-256-GCM ordered records, 32 KiB max plaintext. **Ready proof and three bounded encrypted echo rounds are wired through Relay** |
| Encrypted echo mailbox | internal/remoterelay/echo.go, internal/remoteportal/relay.go | Three sequential browser-encrypted nonce/Agent-encrypted reply rounds per authenticated session; no HTTP/terminal forwarding |
| TaskDeck RBAC preflight | internal/remotetransport/authorization.go and internal/server/remote_access_authorization.go | Fail-closed Portal role ∩ authenticated TaskDeck project permissions. This guard is **not yet a remote application dispatcher** and single-user mode is deliberately denied pending a principal/session adapter |
| Agent-verified Portal grants | internal/remoteportal/grant.go, config/remote.go | Portal **v2** tickets bind the signed browser nonce and exact ECDH public key. Relay checks binding before rendezvous; Agent independently verifies the forwarded ticket using the locally pinned Portal key and authenticated E2EE Channel; TaskDeck login additionally requires a verified browser AES-GCM record. Remote commands remain disabled. |
| Internal encrypted login wire codec | internal/remotetransport/login.go, internal/server/remote_access_login_probe.go, remote_portal/public/remote_crypto.js | Strict JSON login-only AES-GCM messages, one-time Portal grant + real TaskDeck SQLite credential check, encrypted boolean response; **no HTTP/WS mount or session token** |
| TaskDeck E2EE-bound login core | internal/server/remote_access_session.go | Real TaskDeck SQLite/scrypt password verification, 5-minute channel-bound session, fresh per-action RBAC and local credential-free audit. **Internal only; no Browser login endpoint.** |
| Automated tests | remote_portal/tests, Go package tests, .github/workflows | Syntax, authentication, proof, replay, config, relay handshake and compile checks |

Read the [architecture](TASKDECK_REMOTE_ACCESS_ARCHITECTURE.md) and [security risk assessment](TASKDECK_REMOTE_SECURITY_RISK_ASSESSMENT.md) before enabling any transport features. Portal and Relay remain logically separate: PHP handles accounts/permissions, Relay routes authenticated ephemeral sockets; TaskDeck owns its secrets and will enforce its own local authentication.

## 1. Portal setup on shared hosting

PHP 8.1+, PDO SQLite3 and Sodium, TLS with valid CA certificate, private PHP session storage and a DB directory outside public webroot are required.

Only remote_portal/public may be the web server document root. Set REMOTE_PORTAL_DB_PATH to an absolute, writable, non-public SQLite file path. CLI commands:

    php remote_portal/bin/admin.php init
    php remote_portal/bin/admin.php create-admin sysadmin

Enter admin password interactively via STDIN, not in a CLI argument. For the ticket issuer, configure REMOTE_PORTAL_SIGNING_SEED_PATH as a private absolute path, and run:

    php remote_portal/bin/admin.php init-ticket-key
    php remote_portal/bin/admin.php relay-public-key

The public key printed by the second command (base64-encoded 32-byte Ed25519 public key) is used on the Relay **and must also be pinned independently in each TaskDeck instance** under Settings → Security → Remote Access → Portal Ed25519 signing PUBLIC key. Equivalently set `[remote_access] portal_public_key = <base64>` in its private INI. The key must be copied from a trusted administrator CLI/out-of-band source; never fetch a replacement from Relay or a browser handshake. Absent/invalid key means the Agent refuses signed browser grants. Changing the pin requires a deliberate saved Settings update and daemon restart. **NEVER copy the signing seed to Relay or commit it to Git.** REMOTE_PORTAL_ISSUER must equal the exact HTTPS Portal origin, for example https://portal.example.com.

Portal signs a device-presence lease (70 seconds) only after a valid signed heartbeat from a *claimed* Device ID. This requires REMOTE_PORTAL_SIGNING_SEED_PATH and REMOTE_PORTAL_ISSUER to be configured in PHP-FPM. The Relay checks the lease at connection and on each keepalive: an unpaired device cannot occupy a persistent Relay slot, and revoking/unpairing a device causes access to fail no later than lease expiry (plus network timeouts). This is **not instantaneous live session revocation**, which remains a blocker.

The /api/remote/ticket endpoint is disabled unless REMOTE_PORTAL_EXPERIMENTAL_TICKETS=1. **Do not enable this on a public production deployment:** Portal MFA/passkey, live revocation and the TaskDeck authorization adapter are not complete. The Portal stores only Device ID, name, grants and Portal account data, never TaskDeck password or endpoint.

## 2. Relay prototype

The TLS-only Go Relay executable can run on a VPS/container with a valid public HTTPS certificate and an outbound/inbound 443 policy suitable for the Relay itself (clients still require no inbound ports).

Build:

    cd vscode_tasks_menu_go
    go build -o taskdeck_remote_relay ./cmd/taskdeck_remote_relay

Put the public Portal verification key into an owner-readable text file on the Relay host, then run:

    ./taskdeck_remote_relay \
      -listen 127.0.0.1:8443 \
      -portal-issuer https://portal.example.com \
      -relay-origin https://relay.example.com \
      -portal-public-key-file /path/to/portal-public-key.txt \
      -ticket-replay-dir /var/lib/taskdeck-remote/ticket-replay \
      -tls-cert /path/to/relay.crt \
      -tls-key /path/to/relay.key

The Relay MUST be restarted with the **same** absolute, owner-only `ticket-replay-dir` across deployments. This directory stores only random used ticket JTIs and expiry times and survives process crashes; it is required on production start. Do not put it in a temporary directory or Git checkout. The implementation is for a **single Relay host**, not a distributed cluster; multi-host operation requires an atomic shared replay store. Expired records are pruned automatically. A corrupt, truncated, symlinked or insecure replay record **blocks Relay startup**, rather than being silently deleted and potentially permitting ticket replay. Investigate and restore the private ledger, never clear it during an active token lifetime just to bypass the error.

These are placeholders: substitute the actual domains and paths; if binding to a private port behind a reverse proxy, that proxy must preserve WSS upgrades, Host/Origin checks and TLS security. /healthz returns remote_control_available=false. Relay /v1/device verifies Ed25519 device proofs; /v1/browser validates signed Portal tickets, one-time JTI, Origin and device presence, then reports remote control unavailable. Neither endpoint forwards TaskDeck data. The Agent first receives a fresh 32-byte Relay-generated challenge; its Ed25519 response signs the challenge and Device ID, avoiding the old self-chosen-nonce replay weakness across Relay restarts. Relay additionally checks the Portal-signed device lease. A permitted browser ticket launches a bounded challenge/ack reachability roundtrip through the real Agent WSS channel and returns `agent_reachable`; it does **not** open TaskDeck. Single active browser session per instance is a temporary safety constraint, not final multiplex architecture.

## 3. Enable TaskDeck outbound presence

1. In TaskDeck, enable local authentication (or properly configured Multi-user shared-server auth).
2. Open Settings → Security → Remote Access from desktop or mobile.
3. Enter the Portal HTTPS origin, optionally the Relay HTTPS origin, then enable outbound presence and Save. TaskDeck reloads its web daemon; existing independent session broker remains separate.
4. Open Remote Access again, Generate pairing code, and enter the displayed Device ID and one-time code on Portal while signed in as admin.
5. A claimed TaskDeck reports presence to the PHP Portal with a signed heartbeat every ~20 seconds (TTL ~60 seconds). If the Portal signing key is configured, each successful heartbeat returns a signed, short-lived device lease. The Agent holds this lease only in RAM and uses it for authenticated outbound Relay WSS registration and ping/pong. No TaskDeck password, cookie or private key is transferred.
6. **Developer-only connectivity diagnostic**: on the Portal set `REMOTE_PORTAL_RELAY_ORIGIN=https://relay.example.com` and explicitly `REMOTE_PORTAL_EXPERIMENTAL_TICKETS=1`. An authenticated account holding an explicit device grant can click **Kiểm tra Agent** on an online device. The browser sends its 30-second ticket to the configured Relay through WSS (not URL query). Relay verifies it, sends a one-time reachability probe to the exact Agent, then displays Agent reachable/unreachable. This does not establish an authenticated TaskDeck session. Do not enable this experiment on a public production Portal before MFA, abuse control and end-to-end tests are complete.

Remote is OFF by default. An unauthenticated TaskDeck cannot enable Remote. Each absolute project workspace gets a separate identity under the OS user config directory. No local inbound port or port forwarding is opened by Remote Agent.

## 4. CI and local checks

    cd vscode_tasks_menu_go
    go test -race ./internal/remoteportal ./internal/remoterelay ./internal/remotetransport
    go test ./internal/config -run 'TestRemote|TestSetRemoteSettings'
    go test ./internal/server -run TestRemote
    go build ./cmd/taskdeck_remote_relay ./cmd/taskdeck_remote_probe

    cd ..
    find remote_portal -name '*.php' -print0 | xargs -0 -n1 php -l
    php remote_portal/tests/smoke.php
    php remote_portal/tests/tickets.php
    node --check remote_portal/public/remote_probe.js
    node --check remote_portal/public/remote_crypto.js
    node remote_portal/tests/browser_probe.test.cjs
    node remote_portal/tests/browser_crypto.test.cjs  # invokes Go fixture; Go must be installed
    node remote_portal/tests/browser_echo_crypto.test.cjs  # tests both directions on one keypair

CI files: .github/workflows/taskdeck_remote_agent.yml and taskdeck_remote_portal.yml. The green CI verifies covered behavior only; it is not a penetration test or release approval.

## 5. Release blockers — do not mark READY

1. Browser↔TaskDeck transport with HTTP methods, binary uploads, WebSockets, PTY, Patch, Git, files and correct CORS/Origin/cookie/session behavior, without arbitrary host proxying.
2. End-to-end TaskDeck login/session and action-level RBAC for every HTTP/WS command. An internal authorization adapter now checks Portal role ∩ TaskDeck project permissions and denies missing principal/TLS, but **it has no application traffic attached yet**. Legacy single-user auth remains blocked for remote commands until a secure principal adapter exists.
3. End-to-end encryption is proven only for the fixed diagnostic ready proof and three ordered bidirectional nonce echoes. Extend the verified P-256/AES-GCM channel to *all* future HTTP/WebSocket/application frames before enabling remote commands. Browser-delivered dynamic JavaScript supply-chain trust remains a separate risk.
4. Portal MFA/passkey, abuse throttling, password recovery, key rotation; explicit ownership verification on pairing/recovery.
5. Revocation of live sessions, bounded lease, connected device shutdown and replay prevention across Relay process restarts or cluster replicas.
6. Stream/backpressure limits, authorized multiplex, agent restart/reconnect without repeating non-idempotent operations.
7. Portal password rotation is implemented; the production MFA/passkey, account recovery, request throttling and signing-key rotation controls remain outstanding.
8. Security integration tests on real shared hosting and a reachable Relay under NAT/CGNAT, plus independent security review.

**Do not claim full Remote Access is implemented before each release gate is satisfied.** The browser-to-Agent probe confirms only authenticated reachability. It does not expose TaskDeck terminal/file/HTTP APIs, nor does it prove that TaskDeck user authentication over the tunnel works.

### Diagnostic state distinctions

- **Portal online:** a correctly signed TaskDeck heartbeat was received within the presence TTL.
- **Agent reachable:** after a signed Portal ticket, the browser creates a fresh 128-bit random nonce, and the Agent signs `taskdeck-browser-attestation-v1\n<Device ID>\n<probe ID>\n<browser nonce>` using its per-instance Ed25519 identity key. The browser verifies the Ed25519 signature through WebCrypto and checks `SHA-256(public key) == Device ID` and the exact nonce/probe ID. The Relay cannot fabricate this proof using only its own keys. The browser additionally creates a **non-extractable P-256 private key** and sends only the 65-byte uncompressed public key. The Agent signs the P-256 key transcript with Ed25519, derives ephemeral ECDH/AES-256-GCM secrets, and encrypts the fixed `taskdeck-e2ee-ready-v1` proof. Relay verifies the Agent signature, then forwards only the opaque AES-GCM ciphertext. The browser independently authenticates the signed handshake, derives its own keys and decrypts the ready proof. Then it seals an independent random 128-bit nonce (prefix `taskdeck-e2ee-echo-v1:`) with browser-to-Agent AES-GCM direction 1, sequence 1. Relay transports only ciphertext to Agent. Agent decrypts that *exact* diagnostic nonce, seals a matching `taskdeck-e2ee-echo-reply-v1:` result using direction 2, sequence 2, and advances the encrypted sequence counters. This exchange is limited to exactly three sequential rounds (direction 1 sequence 1–3, direction 2 reply sequences 2–4 after the ready frame). Browser verifies every AEAD tag, sequence and echoed nonce before reporting success. Agent and Relay reject a fourth round. The private session is discarded after completing the bounded diagnostic. Unsupported/disabled WebCrypto Ed25519/ECDH/AES-GCM fails closed; it must **never** fall back to trusting a Relay-only `agent_reachable` flag.
- **Remote control available:** always **false** in this branch. The E2EE ready and echo proofs are fixed diagnostic payloads; **no TaskDeck HTTP request, terminal data, file or user credential is carried inside the encrypted channel.** Future application transport must reuse authenticated per-session keys, enforce bounded streams and require TaskDeck authentication/RBAC independently.

### Relay session lifecycle and quotas

The Relay enforces an upper bound of 256 device sockets and 64 browser sockets, including unauthenticated WebSocket handshakes. A browser closing its WebSocket while waiting for an Agent proof releases its reservation immediately (instead of waiting 27 seconds). Each reservation has its own generation counter; delayed cleanup from an old browser cannot cancel a newer reservation on the same Agent. Corrupt replay markers and write failures fail closed. Delayed Agent acknowledgements from a canceled browser are ignored without dropping Agent presence; forged ACKs on an active session remain invalid. These controls reduce resource-exhaustion and replay risk, but do **not** replace MFA, per-user/IP ingress throttling, live revocation or full stream-level permissions.

### Encrypted diagnostic transport limits

- Portal signed grants only authorize rendezvous, **not TaskDeck authentication or permissions**.
- At most three encrypted nonce echoes are accepted per reserved browser session. Each round is stop-and-wait in the client protocol, and the Relay permits only one queued request at a time; maximum ciphertext 1024 bytes. No arbitrary command or path parser exists. Both Go and browser E2EE channels become permanently unusable after invalid ciphertext, malformed frames or replay; Go's explicit Close also invalidates any channel-bound TaskDeck session.
- Relay never decrypts the payload and Agent only recognizes a fixed nonce challenge. No terminal, file, editor, Patch Tool, Git, cookies or local HTTP endpoints are exposed.
- Agent session keys are unique and temporary. Browser's ECDH private key is non-extractable through WebCrypto.
- CI covers tampering, replay, stale session cleanup, a real TLS/WebSocket three-round encrypted exchange and a separate three-round Go↔WebCrypto compatibility test.
- **This is not yet remote control or a general-purpose application transport.** Full streams, TaskDeck authentication, RBAC, session revocation and production security review remain blockers.

### Security limitations of browser attestation

The new signed proof authenticates **which Agent answered the browser nonce**; it does not authenticate a user with TaskDeck or grant permission to run any command. The Portal serves the diagnostic JavaScript dynamically: an attacker who compromises Portal content could replace that JavaScript and bypass browser-side checks, so a trusted frontend delivery channel remains a separate security question. The signed P-256/AES-GCM ready proof now provides **E2EE solely for the fixed diagnostic ready and echo frames**. It does not establish an encrypted *application transport* or TaskDeck session and carries no TaskDeck credentials or data. A compromised Portal JavaScript bundle could still mislead the user even though Relay cannot forge a valid Agent signature.


### Actual TaskDeck login implementation (internal-only)

The private `authenticateRemoteTaskDeckCredentials` helper now checks real TaskDeck shared-server accounts against the existing scrypt verifier and SQLite project membership store. It creates a 5-minute, in-memory `remoteTaskDeckSession` tied to exactly one authenticated E2EE Channel. No transferable TaskDeck cookie or Portal-side session is created. Each attempted remote operation revalidates user enablement, password-change epoch, project membership and permissions, session expiry and the Portal role ceiling. The TaskDeck identity audit records login success or denial without usernames, passwords, Portal tickets, plaintext frames or guessed client IPs. Audit failure denies login.

**Agent-side Portal ticket verification is now wired into the existing diagnostic Browser→Relay→Agent handshake.** TaskDeck's real login verifier remains an internal-only helper; no encrypted browser credential form or TaskDeck command handler is mounted. A signed Portal ticket alone does not authenticate TaskDeck. The Agent must independently verify an authentic Portal grant bound to the actual device and browser session, then receive the TaskDeck login encrypted end-to-end, before any HTTP/terminal request can use the new session. Legacy single-password TaskDeck mode is still intentionally blocked for remote commands.

### Portal signing-key verification and the remaining bridge

The Agent now reuses the tested Ed25519 Portal ticket verifier **independently** of Relay, validates the exact Device ID and pins the expected Portal HTTPS origin. Version 1 tickets are rejected; Portal issues only version 2 tickets after the browser has generated its nonce and P-256 ephemeral key. The signed `browser_binding` is SHA-256 of `taskdeck-browser-ticket-v2` + NUL + 32-character lowercase nonce + NUL + 65-byte uncompressed P-256 public key. Go and PHP share a fixed interoperability test vector. Relay checks the signed binding against the live WebSocket request; Agent checks against its own authenticated ECDH Channel. A Relay that swaps the key or nonce cannot redeem the ticket. Used ticket IDs are replay-protected in memory, and the resulting maximum Portal role is bound to a specific authenticated E2EE channel. A grant can authorize at most one TaskDeck password attempt. The TaskDeck internal verifier does not accept a free-form Portal role.

The local TaskDeck administration form persists a public verification key in `remote_access.portal_public_key`; on daemon startup it initializes Agent's verifier from this local configuration. The key cannot be replaced within a running Agent. The public key is not a secret, but it **is** a trust anchor and must be verified out of band.

**Still not wired:** Portal ticket delivery as an encrypted application message (the Relay-visible signed rendezvous ticket is forwarded to Agent), real TaskDeck browser sessions, HTTP/WebSocket application traffic, continuous permission revocation and per-command RBAC in routed handlers. The opt-in Relay route supports only one E2EE password-check request or the fixed diagnostic echo frames; never arbitrary TaskDeck requests. A signed ticket and public ECDH key are *not* enough for local TaskDeck login: the Agent first authenticates a ciphertext record from the browser, proving possession of the ECDH private key. Agent ticket replay tracking is in-memory; Relay's persistent replay ledger is separate. No Remote Control is enabled.

### Signed browser rendezvous protocol v2 (incompatible with old tickets)

The Portal now signs only version-2 browser tickets, which include a commitment to a browser-generated 128-bit nonce and **ephemeral P-256 public key**. The browser must generate both *before* `POST /api/remote/ticket`, submitting `client_nonce` (lowercase 32 hex characters) and `browser_key` (canonical base64 of the 65-byte uncompressed P-256 public point), alongside the authorized `device_id` and CSRF token. A ticket cannot be used with any other nonce/key. The Portal validates encoding; Go checks that the P-256 point is on curve. Relay burns one-time tickets even when an attempted browser binding is incorrect.

The Relay forwards the signed ticket with the already-authenticated encrypted-session probe, and Agent independently validates it using its locally pinned Portal trust root, issuer, Device ID and its own ECDH channel binding. This forwarded signed token is *not a TaskDeck password* and is already visible to Relay at initial rendezvous. **TaskDeck password delivery is disabled by default; an authentication-only E2EE probe requires both Portal and TaskDeck host opt-in flags.**

The login grant can only be consumed after Agent has verified a browser-originated AES-GCM message, proving that the real browser holds the ephemeral ECDH private scalar. Legacy version-1 rendezvous tickets are rejected by both Relay and Agent, so any custom clients or old Portal deployments must update together. In a future command transport the Portal role remains a ceiling only; TaskDeck SQLite login and per-command RBAC are still mandatory. Do not treat the successful three-round diagnostic echo as permission to invoke TaskDeck APIs.

### Internal encrypted TaskDeck login codec — opt-in diagnostic route only

An isolated authentication-only codec now carries the exact operation `taskdeck-e2ee-login-v1`, one bounded username and password, and nothing else. The Agent-side decoder authenticates AES-256-GCM **before parsing JSON** and rejects duplicate/missing/unknown fields, arbitrary methods/paths/commands, trailing data and oversized credentials. Malformed frames permanently invalidate the E2EE channel. The TaskDeck server exposes an **internal** `VerifyEncryptedRemoteLogin` helper that consumes the Agent-verified one-use Portal grant and reuses the real shared-server SQLite/scrypt authentication and audit. It returns only a sealed `taskdeck-e2ee-login-result-v1` boolean with `remote_control_available=false`; any temporary TaskDeck session is immediately closed. It never returns cookies, tokens, user identities, project paths, permissions or TaskDeck API responses.

Only the non-webroot test fixture `remote_portal/tests/fixtures/remote_login_crypto.js` contains `sealRemoteLogin` and `verifyRemoteLoginResult`; it is tested against `cmd/taskdeck_login_crypto_fixture` using dummy credentials. The PHP-served `remote_crypto.js` no longer contains credential-handling primitives. This is **cross-language crypto validation only**, not a deployable credential client. To run just this check:

    node remote_portal/tests/browser_login_crypto.test.cjs

And the Go internal tests:

    cd vscode_tasks_menu_go
    go test ./internal/remotetransport ./internal/server -run 'TestRemoteLogin|TestEncryptedTaskDeckLogin'

**Current test-only wiring:** Relay accepts exactly one opaque `e2ee_login` instead of the first diagnostic echo, then closes the browser reservation after the Agent returns an encrypted boolean. The Agent independently checks the signed Portal grant and E2EE keys, and can call TaskDeck's real SQLite/scrypt verifier through an in-process callback. That callback is registered **only** when the TaskDeck host has `TASKDECK_REMOTE_LOGIN_PROBE=1` at daemon startup. The PHP Portal **never renders a TaskDeck password form**, including if `REMOTE_PORTAL_EXPERIMENTAL_LOGIN=1`. This legacy environment flag cannot re-enable browser credential handling; the Agent-side callback is independent and local-only. No actual TaskDeck browser/login session, cookie, token, HTTP/WS proxy, file access or terminal access is created.

### Historical opt-in E2EE login-only diagnostic (now forbidden on PHP origin)

**Do not deploy the old PHP-hosted password form.** The original experimental configuration was `TASKDECK_REMOTE_LOGIN_PROBE=1` on TaskDeck plus `REMOTE_PORTAL_EXPERIMENTAL_TICKETS=1` and `REMOTE_PORTAL_EXPERIMENTAL_LOGIN=1` on PHP. The legacy PHP login flag no longer enables a form or serves credential-handling JS. Only a separately verified, independently distributed trusted client may someday send password-bearing records. Its POST ticket request contains ONLY Device ID, 128-bit browser nonce, ephemeral public key and CSRF — no TaskDeck credentials. Browser encrypts the TaskDeck username/password with the attested Agent-only P-256/AES-GCM key and sends one `e2ee_login` ciphertext through Relay. Agent rejects the operation unless locally opted in, the Portal-signed grant is valid for the specific E2EE channel, and the browser actually proves possession of its private ECDH key via an authenticated encrypted record. The in-process TaskDeck verifier checks the shared SQLite account, then returns an encrypted result with only `authenticated` and `remote_control_available=false`. Any TaskDeck session is closed immediately. Relay closes its session after that result; a second login on the same grant/channel is rejected.

**This is not Remote login in the sense of opening TaskDeck.** A successful password check does not authorize access to any app API, does not persist a session, and does not reveal a project name or a TaskDeck route. Experimental flags should be disabled on public deployments until operational controls (per-IP/user quotas, MFA, immediate grant revocation, rate limiting and independent security review) are in place.

CI: `go test -race ./internal/remoteportal ./internal/remoterelay ./internal/remotetransport`, `go test -race ./internal/server -run 'TestRemote|TestEncryptedTaskDeckLogin'`, `node remote_portal/tests/browser_login_crypto.test.cjs` (non-webroot fixture), and `node remote_portal/tests/credential_boundary.test.cjs`. The removed browser login UI test must not be restored.

### Browser-only rejection and Agent presence reliability

An invalid TaskDeck login envelope, locally disabled password-check callback or TaskDeck authentication/audit failure causes Agent to send a fixed `login_rejected` message with **no username, password, error detail or ticket**. Relay closes only the exact pending Browser mailbox, while preserving the already-authenticated Agent presence WebSocket. The same applies to rejected Portal grants / ECDH handshakes (`probe_rejected`) and malformed/tampered diagnostic AES-GCM echo records (`echo_rejected`). All three rejections close only the matching Browser and leave the authenticated Agent presence online; stale rejection IDs cannot cancel a newer reservation. These rejections do not return an encrypted positive login result or grant access; a future independent credential client must report a generic failure. A legitimate password mismatch remains an authenticated encrypted `false` response, with no session created.

Tests verify that rejected mailboxes close immediately, that old rejection messages cannot affect new sessions, and that the same Agent can accept a fresh signed Browser ticket after malformed encrypted login data. End-to-end Go tests exercise Browser→TLS Relay→Agent→Relay→Browser for this scenario, plus independently pinned invalid Portal keys. The Relay never forwards TaskDeck HTTP traffic or returns a TaskDeck cookie/credential.

**Implemented Agent-side load guard:** the opt-in login-only callback now has a constant-memory, synchronized token bucket shared by all Portal users, all browser tickets and all TaskDeck usernames for that device: **5 attempts initially, refill 1 attempt per 12 seconds**. The check occurs before SQLite/scrypt, after signed grant and E2EE session checks. Both successful and unsuccessful attempts consume budget. When exhausted, Agent emits the same fixed browser-only `login_rejected` message and retains its Agent presence connection. Unit tests cover 100 concurrent requests, exact refill boundaries, clock rollback and callback invocation limits; the real TLS test confirms that six distinct valid Portal tickets cannot bypass the five-attempt budget.

**Portal ticket quotas are now implemented:** authorization and ticket reservation use one SQLite `BEGIN IMMEDIATE` transaction; 12 browser tickets per 60 seconds per Portal user across devices, 24 per 60 seconds per device across users. The quota ledger survives PHP worker restarts and is shared across workers, while expired entries are pruned. The endpoint returns HTTP 429 with `Retry-After: 60` on exhaustion. Malformed browser nonce/public key encoding is rejected before the shared device quota is consumed. Tests cover durable counters, role revoke, clock rollback, 60-second expiry, and independent PHP processes requesting tickets simultaneously. This rate limit is separate from the Agent's local 5-attempt/12-second-refill password-verification throttle.

**Next security gates:** Portal ingress throttling by trusted client IP and edge infrastructure, continuous live-session/device revocation, MFA, independent security review, and a separate encrypted application session transport enforcing TaskDeck authentication and project-scoped RBAC on every request. Agent login attempts now use an owner-only **durable** rate ledger with cross-process file locking; operators must protect that file from deletion/reset and test storage failures. The PHP `REMOTE_PORTAL_EXPERIMENTAL_LOGIN` flag is obsolete (no credential UI); keep TaskDeck `TASKDECK_REMOTE_LOGIN_PROBE` disabled on production systems.

### Schema upgrade required for Portal ticket quotas

Existing Portal databases must be migrated by rerunning the idempotent private-host CLI:

    php remote_portal/bin/admin.php init

with `REMOTE_PORTAL_DB_PATH` set to the existing private SQLite database path. The new `remote_ticket_limits` table is additive; existing users, password hashes, project grants and claimed devices are preserved. **Do not skip the migration:** experimental ticket issuance now fails closed (HTTP 503) when the table is absent.

The new cap applies to signed browser rendezvous tickets in **both** three-round echo diagnostics and experimental password-only E2EE probes. It does not open TaskDeck application APIs. Revoking an existing Portal grant or unlinking a device prevents **new tickets** immediately, but a previously signed 30-second ticket remains cryptographically valid at Relay/Agent until it expires; immediate invalidation of already-issued tickets and active browser sessions is still an explicit release blocker.

### Malformed echo cannot force Agent offline

The encrypted diagnostic path now has an explicit `echo_rejected` response for invalid echo ciphertext, unexpected plaintext operations, invalid nonce format or a replayed AES-GCM sequence. The Agent closes its E2EE channel and browser grant, not its authenticated Relay presence socket; Relay cancels only the specific Browser's pending echo mailbox. The new automated tests cover a foreign Agent rejection, a stale previous Browser probe ID and a real TLS flow: an echo containing `terminal.run` is rejected and the next separately signed Browser verifies a valid E2EE echo on the **same Agent connection**. This closes an earlier browser-triggered reconnect/availability problem without opening any TaskDeck application API.

### Security hardening follow-up — 2026-10-28

This follow-up addresses two concrete weaknesses in the shared-host Portal and fixes stale test fixtures after Portal browser authorization gained an `auth_epoch` field:

1. **Anonymous login storage exhaustion — fixed:** the older `Store::authenticate()` persisted a `login_attempts` record even for an invented username. An attacker could send arbitrarily many distinct syntactically valid usernames to grow SQLite and exhaust shared-host disk. The Portal now records failures only for existing enabled accounts. The idempotent `admin.php init` migration removes legacy unknown-account rows, preserving the counters of real users. Tests generate 60 distinct unknown names and verify bounded storage.
2. **Shared-host SQLite disclosure — hardened:** before opening any file-backed Portal database, `Store` now requires a real private parent directory with mode **0700** and a regular, non-symlinked database file with mode **0600**. It creates a new DB exclusively under a restrictive umask rather than allowing a transient group/world-readable file. This protects Portal password hashes and permission grants as well as SQLite journal files. An insecure pre-existing installation now fails closed; fix filesystem permissions rather than disabling the check. The test exercises public-readable files, directories and symlink substitution.
3. **Portal authorization epoch regression — fixed:** signed Agent grant-introspection tests and the TLS Relay login fixture now include the current `auth_epoch`, and the PHP smoke test verifies that an old password generation is denied. The Agent compares the signed epoch with the current Portal account immediately before a password-check probe; an old signed ticket cannot retain a grant after a Portal password change.

Upgrade existing private Portal storage before deploying the new PHP code:

    chmod 700 /absolute/private/portal-directory
    chmod 600 /absolute/private/portal-directory/portal.sqlite3
    REMOTE_PORTAL_DB_PATH=/absolute/private/portal-directory/portal.sqlite3 php remote_portal/bin/admin.php init

Use your actual paths and the PHP account which owns these files. Do not change ownership to a public web user or place the DB in a Git checkout. Run `php remote_portal/tests/storage_permissions.php` alongside the existing PHP tests; the Go suite checks Agent ticket authentication and login-only E2EE interoperability.

**Outstanding high-priority risks:** admission/rate-limiting at the edge for anonymous login and self-signed enrollment, a strong MFA/passkey mechanism for Portal users, trusted browser JavaScript delivery, reliable immediate revocation of active future application sessions, tested key rotation and recovery, a fully encrypted TaskDeck application protocol with per-frame RBAC and bounded stream controls. Multi-node Relay replay prevention requires a shared atomic ledger. No application RPC forwarding is enabled by these changes. Keep this branch **DEVELOPMENT-ONLY** and do not expose terminal, Patch, files or Git through the diagnostic tunnel.


### Live Portal grant validation at login (with password rotation)

Before the Agent invokes TaskDeck's SQLite/scrypt password verifier, it now performs an **independent, synchronous HTTPS** call to \`/api/agent/grant-check\` on its locally configured Portal origin (not via Relay). The Agent signs a fresh \`grant-check\` message using its persistent Ed25519 device identity over the Device ID, Portal user ID, maximum ticket role, ticket JTI, Portal auth generation, timestamp and unique nonce. Portal verifies the signature, nonce replay, device fingerprint, currently enabled user/device, online presence, current grant role, and account \`auth_epoch\` in SQLite. This catches grant removal, role downgrade, device unlink/disable and password rotation even when the browser still holds a previously signed 30-second Portal ticket. The lookup returns only \`ok\` / \`allowed\`; no username, TaskDeck credential, local endpoint or session token is exchanged.

The outbound check is bounded to five seconds, rejects redirects and verifies HTTPS using the Agent's configured Portal origin. If the control plane is unavailable or denies, Agent immediately rejects that *Browser password-check request* without calling TaskDeck scrypt and without disconnecting Agent presence. Legacy epoch-less tickets are not permitted to invoke the experimental login check (diagnostic reachability may still work). The check is **not** a continuous revocation feed for future long-running application sessions; session leases and per-request reauthorization are still required before Remote Control.

### Persistent local Agent password-computation budget

The 5-attempt burst / 1-per-12-second refill Agent budget now lives in a private owner-only JSON ledger beside the existing private device identity seed. It is atomically synced and replaced before an allowed password attempt, with an owner-only advisory lock to coordinate multiple Agent processes using the same identity. Restarting the daemon does not reset the budget. Malformed, missing-field, wrong-device-ID, symlinked, world-readable or unwritable budget storage **fails closed**; tests cover concurrent Agent instances, clock rollback, disk corruption and restart boundaries. No Portal username, password, client IP, ticket or ciphertext is written to this ledger.

Do not delete the rate ledger to recover from a rejected login. A corrupted ledger should be investigated and repaired deliberately by the instance owner; no software path silently resets it. The ledger is not a replacement for trusted proxy/edge ingress limiting or MFA.

### Administrator-owned signed Portal connection check

TaskDeck's authenticated, project-admin-only **Settings → Security → Remote Access → Check Portal connection** button now issues a signed HTTPS heartbeat using that instance's device private key. The result confirms that the Portal accepted the current pairing and that it is reachable. It does not expose the signed device lease, remote password, IP, endpoint or TaskDeck session. This operation is diagnostic only and does not enable Remote Control, HTTP proxy or terminal access.

**Remaining production blockers:** a session transport that preserves actual TaskDeck login state only on authenticated browser E2EE channels; authoritative per-operation project RBAC; continuous revocation of active streams and already-issued grants; hardened browser origin isolation; MFA and independent adversarial review; complete HTTP/WebSocket streaming including Patch/Collection locks, upload/download, terminal ownership and reconnect semantics. Continue keeping command forwarding disabled until those gates are complete.


### Brute-force defense (Portal + Agent)

The Portal now applies two independent *durable* admission controls, in addition to the existing TaskDeck Agent's persistent scrypt budget:

- `POST /login`: 10 attempts / source IP / 60 seconds; 180 attempts / site / 60 seconds.
- `POST /api/agent/enroll`: 12 attempts / source IP / 60 seconds; 120 attempts / site / 60 seconds.
- Shared SQLite `BEGIN IMMEDIATE` transactions prevent concurrent PHP workers from bypassing these counters; quota exhaustion is HTTP 429 with `Retry-After: 60`. Failed storage or missing signing/rate-key prerequisites deny rather than bypass.
- Raw IP addresses are **not written** to the Portal database. Source identifiers are SHA-256 HMACs of the trusted server socket `REMOTE_ADDR`, keyed with an owner-only, independent 32-byte `portal-abuse-hmac.key` outside the webroot. Untrusted forwarding headers are ignored. Site-wide limits cap database row growth even across many addresses.
- Account-level failed-attempt counters are now **reserved before** password verification under an immediate SQLite transaction, preventing races where parallel requests use the same pre-hash failure count. Five unsuccessful guesses lead to a two-minute account lock. A correct login clears its counter. Missing usernames do not create rows; an existing dummy verifier reduces a basic username-enumeration timing difference.
- The Agent still uses its independent persistent file-locked, five-attempt / one-per-12-second password-check budget. Portal credentials and TaskDeck credentials are never stored together.

Migration: on existing deployments, set the private Portal DB directory to mode 0700, DB file to 0600, then rerun `REMOTE_PORTAL_DB_PATH=... php remote_portal/bin/admin.php init`. This additive step creates `portal_ingress_limits` and `portal-abuse-hmac.key`. Protect and back up the key outside the public root. CI now executes `php remote_portal/tests/bruteforce.php`, including 24 concurrent PHP login workers.

**Remaining production blockers:** edge provider IP-based throttling (especially for hosts behind a shared proxy), MFA/passkeys, account recovery, trustworthy browser code supply chain, proper live session revocation and full TaskDeck request/stream RBAC. Remote Control remains disabled; this change only hardens management and enrollment.


### Unpaired Agent grant-introspection storage-abuse guard

A separately identified Portal storage denial-of-service path has been fixed: `checkCurrentAgentGrant` previously accepted a syntactically valid self-signed Ed25519 envelope and inserted the caller's nonce **before** confirming that the Device ID belonged to a paired, enabled TaskDeck instance. Because any untrusted party can generate unlimited Ed25519 identities, this could grow the replay-nonce table without a single valid Portal grant. The endpoint now performs a paired/enabled Device ID lookup inside the SQLite transaction **before writing a nonce**. Unknown Device IDs return a generic denial and cannot allocate per-device replay records; recognized devices still require their signed proof, current user grant and current `auth_epoch`. PHP smoke includes 24 independently generated unpaired identities and checks that the nonce table row count is unchanged. This is not a substitute for edge ingress rate limiting against raw request floods.

### Bounded signed-Agent replay state (paired-device admission)

The Portal now maintains a durable `signed_agent_limits` SQLite row for every paired, enabled Device ID that sends valid heartbeat or successfully authorized grant-check proofs. Both types count against the same **90 signed proofs / 60 seconds / Device ID** budget. The reservation and replay nonce insertion run together under `BEGIN IMMEDIATE` so parallel PHP-FPM workers cannot bypass it. Replayed/over-budget signed requests fail closed without creating more `device_nonces` rows. Revoked, stale-epoch, disabled or unknown grant-checks cannot create nonce records. Unlinking a Device ID cascades deletion of its quota row.

This bounds an attacker who steals a *paired device* signing key from using unlimited valid signatures to fill the nonce ledger. Regular Agent presence intervals are 10 seconds or longer, well within the budget. New installations and upgrades must rerun the idempotent private CLI `admin.php init` to create the `signed_agent_limits` table; until then the signed heartbeat and grant-check fail closed. A dedicated PHP CI regression sends 90 signed heartbeat/grant proofs through independent Store instances, denies the 91st before nonce insertion, checks revocation/expiry and validates unlink cleanup. This does not protect against volumetric traffic before PHP processes reach the authenticated Agent check; edge-level IP/connection throttling remains mandatory before Internet production.


### Pre-signature Agent admission and privileged mutation throttles

Portal HTTP requests to `/api/agent/heartbeat` and `/api/agent/grant-check` now reserve a durable HMAC(source IP) quota **before** reading the request JSON or invoking Ed25519 signature verification. This closes the remaining anonymous CPU-flood exposure not covered by the existing 90/minute *authenticated Device ID* quota: **900 Agent requests per 60 seconds per source; 9000 per 60 seconds site-wide**. The more permissive source cap accounts for many legitimate Agents behind a common NAT. Requests over budget get HTTP 429 and `Retry-After: 60`, while storage/missing key errors fail closed with 503. Never trust unsigned `X-Forwarded-For` headers. On the next CLI `admin.php init` migration, the new `portal_agent_ingress_limits` table is created *additively*: it deliberately does not alter the existing `portal_ingress_limits` action CHECK constraint used by older deployments. `bruteforce.php` tests both per-source/global limits, the unmodified original SQLite CHECK, and absence of raw IP addresses.

Portal administrative writes are additionally budgeted at **60 mutations per admin Portal account per 60 seconds**, shared across sessions/PHP workers, before creating users, claiming or unlinking devices, and adding/revoking grants. `reserveAdminMutation` checks that the current account is still enabled and has admin status inside the same `BEGIN IMMEDIATE` quota reservation. Attempts to exceed quota get HTTP 429 and cannot invoke a mutation. A separate additive `portal_admin_limits` table is required and the CI suite exercises 80 simultaneous attempts in independent PHP processes, expired quota recovery and role disable/revocation. Admin mutation throttling is a mitigation for session compromise and audit/SQLite exhaustion, not authorization for future TaskDeck actions.

The login timing oracle for a locked real username is also reduced: both unknown usernames and already-locked usernames perform the same dummy Argon2 verification under the existing ingress limits. The earlier fast-path for a locked username made known-account enumeration possible after repeated guesses.

**Release safety:** Recent work in this branch also implements opt-in Portal TOTP MFA with encrypted-at-rest secrets and offline-only recovery; the older statement that *no MFA exists* is stale. MFA remains subject to deployment/configuration validation, especially for legacy Portal accounts, and passkeys are not implemented. Production Remote Control is still disabled pending encrypted application transport, per-action RBAC, secure session lifecycle and independent review.


### Portal MFA and pre-session browser DoS hardening

**Portal-account MFA is implemented and enabled by default.** A password-only session for an unenrolled user is limited to TOTP enrollment/logout; privileged GET and POST paths, including ticket issuance and device/grant administration, are blocked until enrollment. The user re-enters their Portal password, confirms a 30-second SHA-1/6-digit RFC 6238 authenticator code, and receives a new Portal auth epoch. The pending secret is valid only for five minutes in a private PHP session; a confirmed secret is encrypted with an independent 0600 libsodium master key stored outside the webroot. All subsequently enrolled logins require a fresh non-reusable TOTP timestep. MFA reset is **CLI-only**, explicitly confirmed, audited and rotates auth_epoch. There is no HTTP reset endpoint. The 256-bit master key must be backed up and cannot be silently regenerated when enrolled users exist.

Sensitive account password/MFA actions have a durable quota of **five / ten minutes / account**, across sessions and PHP workers, reserved before password hashing. Regular Portal login also retains its existing IP/global and account lockout protections.

To prevent an attacker from creating unlimited anonymous PHP session files on a shared host, all browser GET/POST routes now pass an additional durable source/global quota **before `session_start()`**. Limits are **120 browser requests/source/minute** and **2,400 browser requests/site/minute**, separate from login/enrollment/Agent buckets. Unsupported routes/methods are returned 404 and oversized browser forms 413 before session allocation. The browser quota uses an additive `portal_browser_ingress_limits` SQLite table with HMAC-protected network-source identities (never plaintext IP), shared across workers. The HTTP integration test checks 30 unsupported anonymous routes allocate no session files; a separate concurrency test runs 12 worker processes competing for 120 slots.

**Deployment prerequisite:** run `REMOTE_PORTAL_DB_PATH=... php remote_portal/bin/admin.php init` against the existing private DB before activating updated public PHP files, creating MFA columns, account sensitive action counters, browser ingress quota table, and private MFA encryption key. Back up both private key files securely. Default MFA is active unless `REMOTE_PORTAL_REQUIRE_MFA=0` is explicitly set; the opt-out is for isolated local development only. The Portal still never stores TaskDeck user credentials, host addresses or remote session content.

**Remaining blockers before production Remote Control:** phishing-resistant WebAuthn/passkeys, external WAF/DDoS controls, proven long-lived session/stream revocation, trusted browser JS, application E2EE, authoritative per-operation TaskDeck RBAC, hardened key rotation, deployment tests and an independent security audit. Remote command forwarding remains disabled.


### Portal ingress HMAC key fingerprint pinning

The Portal stores a non-secret SHA-256 commitment of its 32-byte private `portal-abuse-hmac.key` in the additive `portal_key_checks` SQLite table on the first `admin.php init`. On every subsequent request, `loadAbuseKey` rejects a substituted key even when the replacement file is a regular 0600 file; CLI initialization rejects missing or mismatched keys once the commitment exists. Without this pinning, loss or replacement of the HMAC key could silently reset per-source quotas by changing the identity of every source. A new/legacy database without a recorded commitment can initialize normally once (and preserve its existing key). Restore the original private key from backup; do not delete the commitment to bypass a failed migration. The `bruteforce.php` regression now tests missing keys, key substitution, and idempotent restoration.

Portal abuse throttling remains *application-layer* defense only. A large unsigned flood can still saturate the reverse proxy or PHP worker pool before rate-limit SQL; edge/WAF infrastructure remains a release requirement.


### Canonical network-source HMAC and pre-JSON enrollment budget

Ingress quota HMACs now use normalized `inet_pton` network-address bytes (IPv4-mapped IPv6 is reduced to IPv4), preventing two textual spellings of one IP from obtaining separate source buckets; no raw address is stored. This updates the source-key format once during deployment, so an active per-source quota may refresh once while global quotas remain in force. Prefer an operator-approved maintenance window with upstream WAF protection.

All Agent API requests, including enrollment, reserve a source/global quota before JSON decoding and Ed25519 signature checks. The HTTP regression verifies malformed enrollment submissions consume the 12/minute source budget and the 13th is HTTP 429.


### Storage-layer administrator authorization guard

Portal administrative actions now have a second, independent authorization check inside the actual Store write transaction. `grant`, `revoke`, `claim` and `removeDevice` require an existing enabled Portal administrator via `requireCurrentAdmin` under SQLite `BEGIN IMMEDIATE`. This prevents a non-admin caller or a stale browser/CLI integration from bypassing the HTTP entrypoint's role check, and ensures a concurrent role change cannot slip between authorization and the grant/unlink mutation. Mutations and their audit records commit together; permission failures roll back. The pre-existing per-admin mutation quota remains an additional outer defense against a compromised admin session, not a substitute for this authorization. Portal smoke tests directly invoke each Store mutation with a non-admin actor and assert no authorization or pairing data is changed.


### Private Portal browser session storage

The Portal now forces its PHP browser session handler to owner-only filesystem storage under `<private SQLite parent>/browser-sessions-<sha256-prefix>/` (mode 0700), instead of using the hosting provider's default `session.save_path`. This is especially important for TOTP enrollment, where a newly generated, unconfirmed MFA seed is held only in `$_SESSION` for five minutes. Existing unsafe/nonprivate directories, symlinks, files masquerading as directories and unwritable storage are rejected with a fail-closed HTTP 503; Agent endpoints never create PHP sessions. The session directory is derived from SHA-256 of the canonical absolute SQLite file path: even two Portal databases sharing one private parent cannot share PHP session records. The session handler is explicitly `files` to avoid unexpectedly storing secrets in a shared host Redis/session service. A storage regression checks directory permissions and unsafe substitutions; the real MFA HTTP test verifies owner-only directory and actual session files even if PHP was launched with a default public temp session path.

Deployment: the PHP host user must have permission to create/write the per-database `browser-sessions-<sha256-prefix>/` directory inside the existing protected DB directory, or an operator must create it with mode 0700. Existing sessions in previous shared PHP storage are not migrated or trusted; users authenticate again. No TaskDeck credentials, IPs or endpoints are persisted on Portal by this change.


### MFA for Relay tickets regardless of Portal bootstrap override

Portal users who have not enrolled TOTP cannot mint **any signed Remote browser rendezvous ticket**, even if `REMOTE_PORTAL_REQUIRE_MFA=0` has been configured for legacy bootstrap or recovery. This direct `/api/remote/ticket` gate is separate from the ordinary Portal page restriction; it rejects before signing or consuming the durable ticket quota. The new `mfa_ticket_gate.php` starts a real loopback HTTP Portal with the override, demonstrates that password-only ordinary Portal access is possible, and confirms the Remote ticket remains 403. It is in PHP CI.

Additional browser hardening sets COOP same-origin, CORP same-origin and restrictive Permissions-Policy. Real HTTP tests check these headers and verify session ID regeneration on login, MFA activation, and logout. The loopback PHP router now allowlists the three static diagnostic E2EE JS assets; direct private source paths/unknown JS requests are denied. None of these browser improvements authorizes remote TaskDeck actions or lifts existing production release blockers.


### Strict positive Portal auth epochs for Relay and TaskDeck Agent

The shared signed-ticket verifier used by the Go Relay and TaskDeck Agent now rejects all Portal tickets with `auth_epoch <= 0`, including claims where the field is missing and Go JSON `omitempty` would otherwise silently decode a zero. Portal has always emitted positive `auth_epoch` for real accounts and increments it on password change, TOTP enrollment and offline factor recovery. The old verifier check `AuthEpoch < 0` mistakenly let a signed version-2 ticket without any generation through the Rendezvous/E2EE proof stage, even though later live-grant verification normally refused it. This closes that gap at the earliest signature validation boundary. Regression tests sign tickets with valid Ed25519 keys and deliberately missing/zero/negative generations, and confirm a signed positive generation passes. The TaskDeck shared-session integration fixtures and Relay/Agent tests have been updated to include a legitimate positive generation. The password-only login probe and arbitrary TaskDeck application requests remain controlled separately.

The Portal's legacy `REMOTE_PORTAL_REQUIRE_MFA=0` setting is **only for non-admin bootstrap**. Every administrative account now must enroll TOTP before accessing Portal grants, account creation and pairing actions even if the override is configured. The prior direct `/api/remote/ticket` MFA gate remains unconditional for all users. A live HTTP test covers non-admin bootstrap, admin enrollment-only access, direct administrative POST rejections and ticket issuance denial with MFA override off.


### Agent CPU-budget isolation per TaskDeck identity; Relay source fairness

**Fixed a multi-project regression with security impact:** All local TaskDeck Agent instances previously persisted their scrypt login-probe limiter to the same `remote-login-rate.json` in the shared private identities directory. That JSON deliberately contains a Device ID and rejects a different device, so enabling Remote on two projects running as the same OS account could permanently deny the second Agent's password probe. Agent now uses `remote-login-rate-<DeviceID>.json` and its own lock per device, preserving the existing 5-token burst/1 token per 12s budget. For upgrades, under both the new identity-specific file lock and the old shared file lock, the Agent validates and **moves** (not copies) the legacy ledger only if it belongs to the current Device ID. Exhausted tokens and the previous refill timestamp survive migration and daemon restart; an unrelated project never inherits or overwrites another Agent's quota. Damaged or symlinked legacy records fail closed. The CI race tests create two agents under a single directory and cover isolation, exhausted-budget migration, restarts and unsafe legacy state.

**Relay WebSocket starvation mitigation:** The fixed global pool (256 Agent / 64 Browser concurrent sockets) is now supplemented by peer-scoped admission (64 Agent / 8 Browser sockets per direct TCP source), not persisted and removed when sockets close. The RemoteAddr is taken from the actual connection rather than untrusted Forwarded/X-Forwarded-For headers. The source IP is canonically parsed so IPv4-mapped IPv6 cannot evade the cap. Unauthorized or malformed socket sources fail closed. One exhausted peer cannot monopolize slots and prevent others from connecting, but this is not a substitute for reverse-proxy or CDN connection-level controls. An operator deploying behind a trusted NAT/load balancer should size the peer capacity accordingly. CI tests source exhaustion, XFF spoofing, independent peers, idempotent release, malformed sources and stable global capacities.

Neither change enables TaskDeck HTTP/WS/terminal/file/patch forwarding. They protect the staged identity-and-rendezvous layer.


### Database-level MFA enforcement for signed remote ticket reservations

Portal signed Relay-ticket creation is now blocked at **both** security boundaries. HTTP `POST /api/remote/ticket` already rejects sessions with no enrolled MFA, including the legacy `REMOTE_PORTAL_REQUIRE_MFA=0` override. The authoritative `Store::reserveRemoteTicket` SQL now independently requires `length(users.mfa_secret_enc)>0` **inside** its `BEGIN IMMEDIATE` transaction, alongside account status, per-device grant, paired-device enablement, online presence, and per-user/device issuance quotas. A future CLI integration, accidental alternate route, or concurrent offline MFA reset cannot reserve a valid ticket for a password-only account. Denied reservations do not allocate or consume any ticket quota.

Regression coverage: `tests/mfa.php` uses an actual TOTP-enrolled account and un-enrolled granted account, verifies that only enrolled account can reserve via Store, then executes the existing host-only MFA reset and confirms ticket reservation is immediately denied with quota unchanged. `tests/smoke.php` and `tests/ticket_limits_concurrent.php` preserve their pre-existing concurrency/limit assertions with explicit database-only enrolled-state fixtures; these are not credentials and the crypto enrollment tests remain in `mfa.php`. No schema migration is necessary. This prevents a database-layer policy bypass; it does not enable TaskDeck HTTP/WS/terminal/file forwarding.


### Database-level mandatory TOTP for privileged administration

The Portal's privileged Store methods no longer rely solely on the HTTP MFA gate. `reserveAdminMutation` checks that the actor is **enabled, an administrator, and has active enrolled TOTP** under its SQLite `BEGIN IMMEDIATE` quota reservation. `requireCurrentAdmin` performs the same triple check inside the write transactions of `grant`, `revoke`, `claim`, and `removeDevice`. Therefore a future CLI code path or alternate PHP route cannot use a password-only Portal administrator to change device grants/pairings merely by omitting the HTTP `$needsMfa` check. The bootstrap override `REMOTE_PORTAL_REQUIRE_MFA=0` is not honored at this database authorization boundary.

No schema migration is needed. Existing admins with no factor must enroll TOTP before changing grants or claiming/unlinking devices; if an operator resets their factor offline, new admin mutations are blocked immediately until re-enrollment. The real-enrollment regression `tests/mfa.php` checks pre-enrollment denial, post-enrollment admission and post-reset denial for admin quotas/grant/revoke/unlink. Pairing/nonce and concurrent quota tests use explicitly labeled non-authenticating MFA enrollment markers to focus on their original behaviors, while cryptographic proofs are verified separately. No TaskDeck remote execution dispatcher is enabled by this policy.


### MFA-guarded Portal account creation distinct from offline CLI bootstrap

The web `/users/create` POST now calls `Store::createUserByAdmin(actor, username, password)` rather than the unrestricted bootstrap helper. User credentials are validated and Argon2/bcrypt-hashed before acquiring a SQLite write lock, then `requireCurrentAdmin` validates the current enabled/admin/**TOTP-enrolled** actor in the same `BEGIN IMMEDIATE` transaction that inserts a **non-admin-only** account and an actor-attributed `portal.user_create` audit event. Error paths roll back both account and audit. This closes the last web-facing privileged mutation that previously used an offline bootstrap entrypoint.

For initial installation/recovery, `Store::createUser(..., admin)` remains restricted to the **PHP CLI SAPI only**; HTTP PHP-FPM/Apache and the built-in PHP `cli-server` cannot invoke it. The host-only `bin/admin.php create-admin` command continues working without a prior account or MFA, while the web router cannot create admins. Tests cover unauthorized creation, success for MFA-enabled admin, failure after MFA reset, and real HTTP account creation/audit on the PHP development server. The store layer does not grant Remote Control privileges to newly created accounts.


### Authenticated E2EE binding before ticket consumption and live per-action revocation

The Go Relay and Agent now use bound one-time Portal-ticket consumption. Earlier `Verifier.Consume` burned the signed JTI **before** checking that the browser nonce/P-256 public key or the Agent's established channel matched the Portal-signed commitment. A client sending an incorrect key/nonce could prematurely consume an otherwise valid ticket; an unrelated Agent channel also consumed its own verifier's replay allowance. `ConsumeForBrowser` and `ConsumeForAgent` validate the exact signed device/channel binding **before** mutating the durable replay ledger. Incorrect bindings fail without allocating a JTI record, while a correctly bound ticket remains single-use across process restarts. Regression tests check nonce substitution, different P-256 key/invalid curve, Agent channel/device substitution, valid later consumption and duplicate denial. This is protection against premature consumption, **not proof of possession of the browser's private scalar**; actual encrypted AES-GCM peer traffic is still required before TaskDeck authentication.

The staged `remoteTaskDeckSession` previously re-evaluated only TaskDeck local identity/membership/permission; it could have authorized a later action after Portal grant revocation if this backend were mounted unchanged. Now session creation retains the signed `AgentGrant`; **no live Portal authorizer is bound by default**, so application actions fail closed. The session can bind only **one** locally configured Agent after matching exact Device ID, issuer HTTPS origin and the same locally pinned Portal signing-verifier instance that consumed the ticket. Each call to `authorizeRemoteAuthenticatedSession` must obtain a fresh, signed, owner-verified Agent HTTPS `/api/agent/grant-check` authorization before checking local TaskDeck status and per-action RBAC. Any timeout, revoked/downgraded grant, disabled account, changed `auth_epoch`, disconnected E2EE channel or local permission failure denies that action; Portal responses are not cached. Binding is synchronized and cannot be silently replaced. The integration tests exercise valid bind, wrong Device ID/Portal/signer, consecutive allowed actions, revocation between actions, Portal outage and close.

**Important release limitation:** This is a fail-closed **unmounted security adapter**, not active Terminal/File/Git/Patch forwarding. A future dispatcher must bind the real authenticated Agent, enforce per-action checks without bypass, bound streams and cancel active operations; testing this staging helper alone cannot establish production Remote Control safety.


### Device-ID identity pinning inside E2EE channels

The signed E2EE handshake now stores the cryptographically authenticated Ed25519 **Device ID** directly inside the in-memory `remotetransport.Channel`, on both Agent and Browser sides. `BoundDeviceID()` returns it only while the channel is live, and `Close()` erases it alongside the E2EE binding. `AgentGrantVerifier.Consume` rejects the Portal ticket unless its target Device ID matches the authenticated channel's Device ID, **before** consuming the one-use ticket. This closes a subtle cross-Device substitution: two distinct Agents can receive the same Browser public key and nonce, giving equal `BrowserTicketBinding` hashes, even though each signed E2EE handshake authenticates a different device. A valid Portal grant must not be moved between those channels. The regression explicitly creates two signed Agent handshakes with identical browser key/nonce and different Ed25519 Device IDs, verifies a mismatch does not burn the ticket, and then verifies successful one-time consumption on the correct channel.

The Go local permission/authorization fixtures use Channel Device IDs derived from the **same** Ed25519 seed as their Agent/Portal ticket, rather than fabricating a mismatched identity. This guard is in addition to per-action live Portal grant introspection, strict pinned issuer/signing verifier and TaskDeck's own local user/project RBAC. It does not enable remote application command forwarding.


### MFA-aware live grants, race-safe password changes and CLI-only recovery

**Portal live-grant eligibility now requires a currently enrolled MFA factor.** The shared `Store::eligibleRemoteRole` query used by Agent-signed `/api/agent/grant-check` explicitly requires nonempty TOTP enrollment in addition to an enabled account, valid per-device grant, enabled paired device and unexpired presence. Previously only `reserveRemoteTicket` enforced MFA, leaving the live introspection helper inconsistent for future internal callers. A granted but unenrolled account now receives `false` from the signed live check without reserving a nonce; enrollment makes it eligible and an offline MFA reset immediately removes eligibility. Tests cover both synthetic signed Agent proofs and real encrypted TOTP enrollment/reset.

**Portal password-change TOCTOU closed.** Expensive password verification, TOTP verification and password hashing still happen outside SQLite's write lock, with a durable per-user sensitive-action quota reserved before expensive verification. The final password update now uses a short `BEGIN IMMEDIATE` transaction that compare-and-swaps the exact previously verified `password_hash`, `auth_epoch` and `mfa_secret_enc`, checks that the account is enabled, increments the generation and writes the audit event **atomically**. A concurrent offline MFA reset, new enrollment, password change or account disable cannot be overwritten using a stale security snapshot. The method returns the exact generation written by this transaction rather than re-reading an epoch potentially increased by a different worker; the PHP session can therefore never accidentally adopt the recovery generation in that race. Regression tests deterministically exercise stale auth generations, changed MFA state, no audit on denied writes, one audit on success and normal sensitive quotas.

**Offline TOTP reset is protected at the Store method boundary.** `Store::resetTotpForRecovery` itself now refuses any PHP SAPI other than `cli`, matching the host-only `bin/admin.php reset-totp <user> --confirm` policy. An accidental future HTTP/PHP-FPM/Apache route cannot invoke the recovery primitive directly. The live `tests/recovery_http.php` fixture exposes a temporary test-only route through PHP's built-in HTTP server, verifies HTTP denial, unchanged MFA/epoch and absence of recovery audit. Its temporary handler is never part of the production public directory.

These changes harden the existing Portal and staged Agent authorization adapter. TaskDeck application command forwarding remains disabled pending cancellation semantics and full end-to-end authorization integration.


### Agent-owned one-shot encrypted login admission independent of callbacks

The experimental encrypted TaskDeck-password probe now enforces **one admission per Portal-signed grant at the Agent boundary**, not merely inside a correctly implemented TaskDeck callback. `Agent.verifyEncryptedLogin` first requires that the signed grant was verified by the *exact* locally pinned Agent verifier, that the E2EE channel object matches, and that the grant is unexpired, unused and still open. It atomically reserves `loginStarted` **before** any live Portal grant-check or scrypt quota request; this bit is never reset, even on revoked grants, Portal outages or malformed login callbacks. A grant already consumed outside that admission is rejected before either live check or password work. After the callback returns, Agent independently requires both: (1) successful inbound AES-GCM authentication of the Browser's first encrypted record, and (2) the callback actually consumed `AgentGrant.TakeForLogin` exactly once. A callback returning purported success without meeting both conditions has its grant closed and its result discarded. The first encrypted login frame normally supplies the first Browser proof-of-possession, so this check must happen **after** decryption, not before.

The device-wide 5-attempt burst / 12-second refill limiter remains durable and limits scrypt work even when users obtain multiple fresh Portal tickets. Regression tests use six **separately signed** valid grants to verify the shared Agent budget, a malicious callback that skips grant consumption, 40 simultaneous login attempts racing on the same grant, foreign signing-verifier instances with the same Device ID/issuer, a pre-consumed grant and a revoked signed grant. Rejected grants never receive TaskDeck cookies, PTY/file API access or executable capability. This is still login-only diagnostics: the command dispatcher remains disconnected.


### Authenticated outbound E2EE login-result enforcement

The Agent no longer trusts an owner-configured TaskDeck login callback to return a correctly encrypted, narrowly typed result. Before passing ANY callback bytes to Relay, `verifyEncryptedLogin` now calls `remotetransport.VerifyOutgoingRemoteLoginResult`. The Channel authenticates the returned frame against its own AES-GCM **send key**, exact current sequence number, AEAD transcript/direction binding and frame size; this validation does not advance or reset the nonce sequence. The decoded payload must match the same strict, three-key, duplicate-free JSON schema already required by the Browser: `type=taskdeck-e2ee-login-result-v1`, `authenticated` boolean and `remote_control_available=false`, with no trailing or extra properties. Invalid plaintext, malformed bytes, reused/stale ciphertext, tampering, or additional secrets/cookies/bearer fields cause the Agent to discard the response and permanently close the grant **and E2EE channel**, never forwarding callback content to Relay.

The Browser inbound and Agent outbound validators now share one canonical pure login-result JSON parser to prevent policy drift. The existing TaskDeck callback uses `SealRemoteLoginResult`, so production login-only behavior remains unchanged. Regression tests verify successful/failed sealed booleans, outbound AEAD integrity, duplicate/extra JSON properties, `remote_control_available=true`, stale ciphertext, closed-channel denial, and a malicious callback returning a plaintext password or an authenticated ciphertext with a token field. The strict outgoing check is a defense-in-depth control for accidental callback refactors; it does not expose any Remote Control dispatcher.


### Shared signed-Agent proof quota counts revoked grants and replayed nonces

The paired-device **90 signed proofs per 60 seconds** budget is now charged for every signature-valid `/api/agent/grant-check` invocation from a currently paired and enabled Agent, including calls denied due to revoked/downgraded grants, missing MFA, changed `auth_epoch` or an unknown Portal user. Previously only `allowed=true` paths charged capacity. A compromised/obsolete paired Agent could therefore make an unlimited number of *denied* signed HTTPS checks and repeatedly acquire SQLite write locks without spending its proof quota. The quota is reserved atomically under `BEGIN IMMEDIATE` before checking live grant eligibility. Unpaired or disabled Device IDs still fail before allocating any quota/nonce rows.

For both signed Agent `heartbeat` and live grant checks, a **replay of an otherwise valid signature and nonce now also spends device-wide proof capacity**. Before this change, detecting an already-used nonce threw inside the SQLite transaction, rolling back the reserved quota. A repeated valid signed message could incur repeated crypto/SQLite processing at zero cost. The code now commits the proof reservation even when the nonce already exists, then returns a replay rejection. It never inserts a second nonce, refreshes presence on a replay, treats a replay as grant authorization, or allocates nonce rows for denied grants. I/O failures and other database errors still fail closed without producing false success.

The expanded `tests/signed_agent_limits.php` suite uses one real signed paired device and two SQLite connections. It verifies the 90/min limit shared by accepted heartbeats, accepted grant checks, denied/revoked grant checks and replays; exactly the 91st request is rejected; replay/denied requests do not grow the nonce table; and unpairing removes device quota state. Remote Control remains disabled. This is a same-device resource cap, not a replacement for ingress WAF/edge DDoS protection.


### Relay WebSocket handshake-churn and source-cardinality budgets

The standalone Go Relay now has **separate process-local request-rate budgets** for device and browser WebSocket handshake attempts, in addition to the pre-existing concurrent connection budgets. Previously one TCP peer could open and immediately close sockets repeatedly, freeing its 64-Agent / 8-Browser active slots while forcing unlimited new upgrades, challenges and Agent/Portal ticket work. The new `handshakeBudget` (in `internal/remoterelay/handshake_limits.go`) admits only **40 Browser** or **180 Device** attempts per direct TCP source per 60-second window, and at most **300 Browser** or **2400 Device** attempts across the single Relay process per 60 seconds. Rate admission runs **before** allocating active sockets, upgrading WebSocket connections, or verifying cryptographic tickets. Exhausted budgets return HTTP 429 with no new socket reservation. Bad method, browser Origin and URL query fail their original preflight first.

Identity is derived solely from the canonical *direct socket peer* via `net.SplitHostPort` and `net.ParseIP`; never from browser-controlled `X-Forwarded-For`, so IP spoofing cannot replenish a source budget. IPv4 and IPv4-mapped IPv6 share the same budget. Sliding back the clock cannot replenish a budget. The fixed-window counters are bounded to **4096 source entries per socket class**, with expired entry pruning and fail-closed capacity admission. Browser and Agent budgets are independent, and a different IP remains serviceable when one source exhausts its local budget. This guards short-lived handshake churn, but **not distributed DDoS** or multiple Relay replicas; external L7 rate limiting and trusted proxy configuration remain necessary. Reverse proxies presenting all clients as one direct source will share this rate budget, and limits must be adjusted for legitimate large NAT deployments.

Regression tests include per-source/global exhaustion, 250 goroutine races, canonical mapped-IP and forged XFF handling, bounded source-map memory, 60-second reset and rollback, and a **real TLS/WebSocket** integration test that repeatedly connects and disconnects all 40 permitted Browser sockets and then requires HTTP 429 for the 41st despite zero active slots. This feature does not enable Remote Control or TaskDeck terminal/file/git/patch dispatch.

### Bounded authenticated Agent WebSocket frames (single-connection CPU flood)

**Threat closed in this round:** Relay previously limited new WebSocket handshakes and simultaneous sockets, but did not limit the `ping` / `probe_ack` / `login_ack` / `echo_ack` frames arriving on a **single authenticated Agent socket**. A compromised paired Agent could maintain one connection while sending thousands of signed-lease keepalives and registry operations, bypassing the reconnect quotas without ever acquiring a second socket.

`internal/remoterelay/device_frame_limits.go` adds a per-connection token bucket, initialized **after the signed device challenge and successful registration**. It admits an initial burst of 12 text frames and replenishes one frame every two seconds, never accumulating more than 12. The Relay checks admission **before JSON decoding, lease Ed25519 verification or registry mutations**. Exhaustion immediately closes only that Agent's connection, releasing its registration and concurrent socket slot. The bucket uses Go monotonic time and explicitly rejects backwards time observations. No IP address or credential is added to storage. The normal 20-second Agent heartbeat and three-round diagnostic protocol remain within budget; a malicious Agent cannot borrow tokens from another connection.

Coverage: `device_frame_limits_test.go` verifies burst exhaustion, partial refills, clock rollback, long idle recovery and independence across Agent sockets. `device_frame_socket_test.go` performs a genuine TLS/WebSocket device challenge, Portal-signed presence lease and valid Ed25519 Agent proof before verifying that the 13th immediate keepalive closes the socket. `remoteportal/login_relay_test.go` now uses a paced two-second test heartbeat instead of 35ms; legitimate multi-ticket diagnostics must obey the production Relay frame budget too. The isolated Go unit tests passed locally; full GitHub Actions race/integration checks are required for release acceptance.

**Deployment and remaining risk:** Limits are per authenticated socket, not a distributed global CPU budget. An attacker controlling many enrolled devices, a stolen signing seed, or a large botnet can still exhaust Relay/proxy resources; use independent per-device policy, node resource limits and edge L7 controls. Revocation of active long-running TaskDeck work is not implemented. Remote Control, terminal/file/HTTP forwarding, Git/Patch execution and WebSocket multiplexing remain **disabled**.

### Async Portal-grant revocation race closed at Agent and TaskDeck session

**Security finding:** The internal Agent `VerifyCurrentPortalGrant` and TaskDeck `authorizeRemoteAuthenticatedSession` checked channel/grant/session status before issuing a live HTTPS Portal request, but a concurrent E2EE disconnect or local revocation could occur **while that call was pending**. A stale `allowed=true` result might then be returned to a future dispatcher, despite the revoked in-memory session.

The Agent now rechecks ownership, local verifier, `grant.closed`, `loginUsed` and the authenticated Channel **after** the Portal response. The TaskDeck session closes its E2EE Channel (invalidating AEAD keys/bindings) when `Close` is called, and rechecks session closure, Agent-grant liveness, Channel binding and 5-minute expiry after the local TaskDeck permission check. The new `AgentGrant.ActiveForSession` deliberately **does not** apply the original 30-second browser rendezvous ticket expiry to an otherwise valid 5-minute TaskDeck session; each requested action still requires uncached, signed Portal grant introspection.

Deterministic regression tests in `remoteportal/grant_check_test.go` and `server/remote_access_session_test.go` block the live grant callback until another goroutine closes the grant, disconnects the channel or shuts down the local session, then return a stale positive verdict. All must deny. Session closure must make `Channel.Established()` false. Go CI exercises the full `-race` protocol suite and internal server Remote security tests.

**Release blocker unchanged:** These are internal, **unmounted** authorization helpers. A real application dispatcher will additionally need an execution-time revocation/cancellation contract and safe stream cleanup; an action preflight alone cannot retroactively stop a running PTY, HTTP request or file write. All Remote Control dispatch remains disabled.

### One-shot TaskDeck login / E2EE transport ownership regression and post-login grant revalidation

A previous hardening change made `remoteTaskDeckSession.Close()` invalidate its E2EE channel. That is correct for future long-lived authorized application sessions, **but wrong for the current one-shot TaskDeck password probe**: `VerifyEncryptedRemoteLogin` called `Close()` before `SealRemoteLoginResult`. The browser could therefore never receive its authenticated encrypted boolean. The existing server regression was not executed because Remote Agent CI only selected test names starting with `TestRemote`.

**Fix:** `remoteTaskDeckSession.closeWithoutChannel()` retires only the ephemeral TaskDeck principal and the already-consumed Portal grant in the one-shot login probe, leaving the Agent-owned E2EE channel alive just long enough to seal and forward its fixed boolean response. Normal `Close()` still invalidates the E2EE channel for a future app session. Reuse of a closed/expired/mismatched grant is an explicit protocol violation and now poisons the supplied E2EE channel. The current Agent relay caller continues to close the browser E2EE channel after forwarding a single login reply.

**Second revocation check:** The Agent performs an additional live, signed HTTPS Portal grant introspection AFTER TaskDeck's password verification and validation of the restricted encrypted reply, BEFORE releasing ciphertext to the browser. This prevents a credential result from being delivered when Portal access was revoked/downgraded during slow local password hashing. Failures close both grant and E2EE channel; they do not forward a stale successful login reply. This intentionally costs an additional signed proof per completed valid callback and is subject to Portal's existing per-device and ingress quotas.

**CI coverage:** The Remote Agent GitHub workflow now runs `go test -race ./internal/server -run 'TestRemote|TestEncryptedTaskDeckLogin' -count=1` rather than only `TestRemote`. The omitted existing regression `TestEncryptedTaskDeckLoginWithRealAccountHasNoSessionToken` exposed the exact unexpected channel closure. New `TestAgentLoginRejectsPortalRevocationDuringPasswordVerification` uses synchronized goroutines to demonstrate a late revoked answer denying ciphertext release. Retain these tests in every Remote release gate.

**Not enabled:** No browser gets a TaskDeck bearer token, session cookie, terminal/file/git/patch endpoints, stream multiplexing, or actual remote commands. A successful experimental login returns only an encrypted `authenticated` boolean.

### One-shot E2EE login: distinguish principal cleanup from grant revocation

**Security finding (post-async local revocation):** A successful one-shot TaskDeck login probe had to clean up its temporary TaskDeck principal before returning an encrypted boolean. The prior implementation also called `AgentGrant.Close()` as part of that cleanup. Because the Agent's second live Portal check necessarily tolerated an already-closed grant, an **independent local grant revocation during password verification or during the second HTTPS check** could be indistinguishable from normal cleanup; a stale sealed `authenticated=true` might still be forwarded.

**Correction:** The unmounted `remoteTaskDeckSession.closeWithoutChannel()` now retires only its temporary TaskDeck principal; full `Close()` still closes the signed grant and E2EE channel for actual application sessions. The Agent alone owns one-shot grant finalization: it checks local closure, E2EE liveness and caller cancellation before and after a **fresh second** Portal introspection, then atomically `CompareAndSwap(false,true)` on grant closure before releasing the already-encrypted, fixed-schema boolean. Explicit local revocation winning that CAS must return denial and poison the E2EE channel. On successful one-shot login, the grant is consumed before forwarding any bytes. No login cookie/token/session or TaskDeck API is exposed.

**Regression tests:** `TestAgentOneShotLoginFailsClosedWhenGrantCanceledDuringAsyncWork` blocks (a) TaskDeck password callback or (b) second Portal check, explicitly closes the grant, then returns otherwise-positive results. Both must fail closed, return zero ciphertext and close E2EE. `TestAgentOneShotSuccessRetiresGrantAfterFinalPortalCheck` verifies the legitimate signed one-shot result, exactly two Portal checks, and a retired Agent grant. Existing real-account Server encrypted-login tests cover final E2EE boolean and repeated login rejection.

**Release boundary unchanged:** these are security fixes for the **login-only diagnostic**, not authorization to open remote HTTP, file, terminal, git or patch operations. A later dispatcher needs per-action live RBAC, cancellation and bounded execution/stream lifecycle before activation.

**Verification (one-shot grant finalization):** GitHub Actions run [40711010857](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40711010857) for functional commit `47211f4de4f7e4d451891f55059927dccf4d186a` completed **SUCCESS**. It covered Go protocol, Remote HTTP/UI/real-account encrypted login probes, additional Agent grant race tests, server/daemon builds and macOS/Linux ARM cross-compilation. Later documentation-only commits do not alter the verified Go source.

### Compromised-PHP control plane: credential entry removed from untrusted origin

**Security requirement / threat model (2026-10-28):** Assume the shared-host PHP Portal is fully compromised: attacker can read and replace its PHP, HTML, JavaScript, SQLite data, Portal signing seed, and live grant-check responses. The operator MUST NOT rely on signed Portal tickets or `allowed=true` checks as proof of a legitimate TaskDeck user. These are only rendezvous/control-plane signals. The independent TaskDeck daemon must verify a real local TaskDeck password hash, current project membership, user-enabled status, scoped RBAC, and the authenticated E2EE channel, before any application action. No Portal-issued token/role/session may act as a TaskDeck cookie.

**Critical browser-side exception:** Previously the Portal rendered a `taskdeck_password` form and loaded `remote_login.js`, while `remote_crypto.js` exposed `sealRemoteLogin`. Although the ordinary code sent only encrypted data to Relay, a compromised PHP server could replace the JavaScript and **read the real password before WebCrypto encryption**, then send it elsewhere. Therefore an absolute guarantee against a fully compromised PHP origin was **not** true.

**Changes:** Removed the Portal-hosted TaskDeck login form, `REMOTE_PORTAL_EXPERIMENTAL_LOGIN` UI gate and its browser script entirely. The browser `remote_crypto.js` now contains *only* password-free E2EE readiness and diagnostic echo. Moved the Go↔WebCrypto password wire-format codec into `remote_portal/tests/fixtures/remote_login_crypto.js` **outside public/** for testing only; the fixture is not a supported credential client. Updated the dev-router allowlist, PHP HTTP tests, Node browser tests and both CI workflows. The public Portal now explicitly warns **never to enter TaskDeck credentials** into its pages. The local owner-only `TASKDECK_REMOTE_LOGIN_PROBE` callback remains independently disabled by default and supports protocol tests, not an enabled remote application dispatcher.

**Compromised-issuer regression:** `TestRemoteCompromisedPortalSignerCannotBypassTaskDeckPassword` constructs cryptographically valid, attacker-selected admin grants, then submits wrong, missing and empty local TaskDeck credentials over actual encrypted Go channels; none may report a successful TaskDeck login. `TestRemoteCompromisedPortalValidPasswordOnlyGetsOneShotBoolean` checks that even a real TaskDeck password returns only a sealed boolean and no TaskDeck API session, and the same grant cannot be reused. Portal CI enforces that no public login script/form or password codec is shipped, and checks HTTP 404 for the removed asset. These are security boundaries, **not** proof against arbitrary future unreviewed application handlers.

**Non-negotiable release gate:** A full compromised-PHP security guarantee for actual remote use requires a trusted, independently distributed/verified client (e.g. signed native app, authenticated browser extension, or client from an independently secured origin). That trusted client must exclusively handle TaskDeck passwords and E2EE secrets, and must independently pin the TaskDeck device identity; never load credential-handling JS/HTML/config from PHP Portal or another attacker-controlled origin. If a user supplies their TaskDeck password to malicious HTML served by the compromised Portal, it can be stolen; no Agent-only password verification can prevent phishing at that endpoint. Keep Remote Control **OFF** until trusted client + app dispatcher authorization/cancellation are implemented and penetration-tested.


### Trusted local E2EE login CLI: first independent password-entry path (2026-10-28)

**Goal:** make PHP Portal an **untrusted rendezvous and ticket issuer**, not a source of password-entry JavaScript. Its complete compromise may include PHP, HTML, JS, SQLite, live allow/deny checks and Ed25519 Portal ticket-signing seed. None of those materials should become the TaskDeck password, an Agent private signing key, an E2EE private scalar or an application execution capability.

**Implemented this round:**

- `vscode_tasks_menu_go/cmd/taskdeck_remote_login/main.go`: new local Go CLI (Linux/macOS) built independently from a verified source checkout. It opens a **local Unix TTY** and disables password echo using the local `stty` utility; no password flag, stdin pipe, environment variable, PHP page or remote JS is accepted. It retains a fresh P-256 private scalar **only in memory**, displays the browser nonce and public key, then receives a manually copied one-use 30-second Portal ticket.
- `remote_portal/public/index.php`: an authenticated, MFA-gated `/api/remote/ticket` HTML form accepting **only** the CLI's public nonce/key and device ID; the existing atomic user/device 60-second ticket quotas and Ed25519 binding still apply. The response is JSON copied back into the CLI; no TaskDeck username/password is collected by this page.
- The CLI performs an outbound TLS-verified WSS connection, verifies a locally **pinned** Ed25519 TaskDeck Device ID, Agent-signed P-256 ephemeral key transcript, and Agent AES-GCM ready packet, then sends a single encrypted TaskDeck login. It validates the encrypted response against the strict fixed-format `authenticated` boolean; **no TaskDeck HTTP cookie, bearer, remote command, filesystem, terminal, Git/Patch or application session** is supplied.
- `-pin-file` is **mandatory**: a local, absolute path to a regular owner-only 0600 Device ID text file in a non-symlinked, 0700 parent directory. It must be provisioned from the **actual TaskDeck host by an independent owner-verified channel**, never copied blindly from the compromised Portal. The supplied `-device` flag must exactly match the pin or the CLI refuses to read a password. This prevents PHP-origin substitution of a device ID whose private key the attacker controls.
- `cmd/taskdeck_remote_login/main_test.go` covers malformed/untrusted origins, invalid/unpinned device IDs, fake Portal/Relay identity responses, valid Agent ECDH/AEAD proofs, forbidden remote-control fields, replayed encrypted login responses, and tampered/public/symlinked local pins. The Remote Agent CI workflow now tests this command under `-race`, compiles it, and cross-builds for macOS amd64/arm64 and Linux arm64. Portal `credential_boundary.test.cjs` requires that the page contain only public-key ticket inputs and never an embedded TaskDeck credential form.
- Developer instructions and the explicit insecure-PHP trust model are in `vscode_tasks_menu_go/cmd/taskdeck_remote_login/README.md`. **No server-side PHP credentials** are added.

**Current opt-in and limits:** the Agent's `TASKDECK_REMOTE_LOGIN_PROBE=1` is still an **explicit local experimental flag**, and PHP tickets require an authenticated MFA Portal session and `REMOTE_PORTAL_EXPERIMENTAL_TICKETS=1`. The CLI has no application dispatcher and returns only an encrypted login yes/no. It is a **source-build developer prototype**, not a signed, installed, update-protected client release. An attacker in full control of PHP can phish a human with arbitrary malicious HTML, instructions or a counterfeit binary; the trusted client is secure only if the user runs a *separately verified build* and provisions the Device ID pin out-of-band. An already stolen TaskDeck password, local machine compromise, or social engineering cannot be negated by a password-only design.

**Remaining release blockers:** independently signed client distribution/update, safe local pin enrollment UX, TaskDeck password login per-session (not one-shot), per-action local TaskDeck RBAC and grant revocation, cancellable bounded stream gateway and full compromised-PHP E2E penetration tests. Keep Remote Control **OFF**. **Verified:** GitHub Actions Remote Agent Go run [40714909315](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40714909315), testing the out-of-band pin implementation and regression suite at commit `f0d8b02bebc2b5b311602d67da2b815b661d306e`, completed **SUCCESS** (Go `-race`, server credential checks, Linux builds and macOS/Linux ARM cross-compilation). PHP Portal validation [40714633682](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40714633682) also completed **SUCCESS** for the public-key ticket form and credential boundary. Subsequent documentation-only commits do not affect this code evidence.


### TaskDeck password-generation recheck and client pin ownership

The actual TaskDeck-local scrypt verifier remains the decisive credential boundary even if **the entire PHP Portal signing key and its admin grant policy are compromised**. The existing one-shot `VerifyEncryptedRemoteLogin` returned a positive encrypted boolean based on a user snapshot fetched **before** the expensive password verification. An administrator could rotate the local TaskDeck password or disable the account during that calculation. The staged `authenticateRemoteTaskDeckCredentials` now re-reads the authoritative TaskDeck account **after** scrypt and once more immediately before recording success. It requires the account to remain enabled and rejects changes to the stored scrypt password hash, the identity `UpdatedAt` generation and `PasswordChangedAt`. This explicitly prevents an old password proof from adopting a disabled/rotated or disabled-then-re-enabled account snapshot. The local TaskDeck principal and project membership are still independently resolved; the Portal's role alone never creates a TaskDeck cookie or session. The test suite includes deterministic snapshot changes and an actual encrypted login fixture in which an attacker-controlled Portal mints an `admin` ticket while TaskDeck rotates the user's password or disables the account between scrypt verification and the subsequent identity read. The only permissible result is `authenticated=false`.

For the separately built trusted login client `cmd/taskdeck_remote_login`, the local Device ID pin now requires both correct **owner-only permissions** (0600 file, 0700 parent directory) AND ownership by the client's current effective OS user. A foreign-tenant private file is not a trusted anchor merely because its mode is 0600, particularly when the client was inadvertently started with elevated privileges. The Linux/macOS client opens the pin using `O_NOFOLLOW`, validates the actual opened descriptor's mode and UID and bounds its read to 66 bytes. This prevents a path-swap/symlink read between metadata validation and opening the trust anchor. Tests simulate foreign-owner stat records, nonregular pins, symlinks, overlong pin contents and normal trusted pins. **The pin still must be verified out-of-band at the TaskDeck host; users must not generate a trusted pin by copying a Device ID only from the potentially compromised PHP page.**

The remaining limitations are unchanged: the login proof provides an encrypted boolean only, has no Remote Control application dispatcher, and cannot by itself cancel long-running future TaskDeck tasks. The two local identity reads narrow the login race; a future enabled dispatcher will require a transactional account/session validation boundary and live per-action checks before release.


### Owner-verified, no-follow persistent Agent Ed25519 identity loading

The **Agent's persistent private Ed25519 key** has now been hardened to match the independently installed native client's owner-only trust pin. Prior to this change, `LoadOrCreateIdentity` only checked Unix 0700/0600 modes: it did not verify the real UID of the private seed file/directory and it performed an `os.Lstat` followed by a second, symlink-following `os.ReadFile`. A file-system swap between these calls could redirect the Agent to an unintended seed. On shared hosts or accidental elevated execution, mere owner-only permissions are insufficient to establish that the seed belongs to the current TaskDeck operator.

The Agent now requires an actual **owner-UID-matched 0700 directory**, opens the seed via `syscall.Open(O_RDONLY|O_NOFOLLOW|O_NONBLOCK|O_CLOEXEC)`, and validates the **opened file descriptor's** mode, UID, regular-file type and exact 32-byte length before reading. O_NOFOLLOW closes the symlink time-of-check/time-of-use window; O_NONBLOCK ensures that a malicious FIFO cannot hang startup *before* the inode is rejected. New identities are created with `O_CREAT|O_EXCL|O_NOFOLLOW`, a complete seed write is mandatory, and both the file and its containing directory are fsynced before use, preventing silent Device ID rotation after a write crash. Any malformed/partial seed is a hard failure instead of silently generating another private identity.

Regression tests verify persisted identity across restart, forbidden symlink seeds, incorrect 0600/0700 file-directory owner UID even when mode bits look correct, group-readable seeds, excessive length, and prompt rejection of FIFO identity files. These changes add no Portal-readable credentials, no remote TaskDeck interface and no new grant/command privileges; a compromised PHP host still cannot derive the Agent's private Ed25519 key from Portal storage. Security still depends on OS-level isolation between PHP hosting and the TaskDeck Agent.


### Fully compromised PHP Portal: no HTTP-principal fallback, local password epoch pin, trusted CLI proof-first prompt

Security invariant: if the PHP Portal/SQLite, Portal signing key, browser HTML/JS, and HTTPS `/api/agent/grant-check` replies are ALL attacker controlled, their `admin` tickets and `allowed=true` verdicts still do **not** authenticate a TaskDeck user or grant TaskDeck command/file access. The TaskDeck-local password verifier is not delegated to PHP; Remote Control remains unmounted and disabled.

The previously staged `server.authorizeRemoteTransportAction` was a latent unsafe integration boundary: it accepted a regular TaskDeck HTTP request's `PrincipalFromContext` together with a caller-supplied Portal role. A future handler could accidentally treat an ordinary browser cookie and attacker-controlled `admin` role as a remotely authenticated E2EE session. That fallback has been **removed**. Its signature now requires the in-process `remoteTaskDeckSession` returned only by the local TaskDeck password verifier and the original authenticated `remotetransport.Channel`. Every future action is delegated to `authorizeRemoteAuthenticatedSession`, which verifies fresh Portal authorization **and** TaskDeck account status, project membership and per-action permissions. Neither a Portal ticket nor HTTP Principal can be passed as credentials.

The staged E2EE session now stores a SHA-256 commitment to the **locally verified TaskDeck password hash** and the exact TaskDeck account `UpdatedAt` timestamp; each prospective action compares the current TaskDeck-local identity record against both, independently of `PasswordChangedAt`. This closes a legacy/backdated account-rotation case where a password hash changes but a prior or absent `PasswordChangedAt` could have left the old session eligible. A malicious Portal that keeps returning `allowed=true` cannot override this local revocation.

The independent native `cmd/taskdeck_remote_login` CLI now takes the **TaskDeck password only AFTER** the out-of-band pinned Device ID, signed Agent ECDH transcript and encrypted AES-GCM ready message are successfully verified. A forged ticket, substitute Relay or PHP-controlled Device ID cannot trigger password entry before authenticating the genuine Agent. Unit tests cover rejected forged Agent proof, unestablished/closed channels that never invoke the password reader, a genuine Agent which receives one encrypted password frame, compromised Portal `admin` tickets with wrong local passwords, ordinary HTTP principals, absent live authorization, wrong E2EE channels, local RBAC, and backdated TaskDeck password changes with Portal verdict fixed to `allowed=true`.

No browser on the untrusted PHP origin is allowed to handle the TaskDeck password; the trusted CLI must be built/distributed and Device ID pinned independently of that origin. This is protection against compromise of the **PHP host only**, not against phishing a user into entering credentials in malicious PHP-delivered content or compromise of the TaskDeck/client OS. No command dispatcher or remote file/terminal/Git/Patch API has been enabled.


### Compromised-PHP full-path acceptance gate — 2026-10-29

**Threat model:** PHP host source/DB/Portal Ed25519 signing seed and live `allowed=true` grant API are entirely attacker-controlled; Agent private signing seed, trusted CLI binary, out-of-band Device ID pin and local TaskDeck password database are not.

**New end-to-end regression:** `internal/server/remote_compromised_portal_relay_test.go` uses a **real TLS Portal/Relay WSS host**, hacked Portal API that signs Agent presence leases, a fully attacker-signed Portal **admin** rendezvous ticket, and an Agent configured to invoke the real TaskDeck `VerifyEncryptedRemoteLogin` against the local Shared Identity database. Wrong TaskDeck password is rejected; there is no reusable browser application session or remote-control permission. GitHub Actions run [40806204975](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40806204975) completed **SUCCESS**, covering this realistic end-to-end adversarial test.

**Independent device-auth regression:** `internal/remoterelay/compromised_portal_device_test.go` opens real WSS, obtains the Relay challenge, supplies a correctly Portal-signed lease for a genuine Device ID, but signs the device connection with the attacker's own Ed25519 key. The Relay must reject it without registering that Device ID. This explicitly proves that Portal's signing key cannot impersonate an authenticated TaskDeck Agent. The initial Go test string-literal typo was fixed in `9d6d2427e005ce92d1c95895ae7fe6d6d3838114`. Full Go Remote Agent race/integration/cross-build CI [40806726641](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40806726641) completed **SUCCESS** for that commit, including real compromised-PHP Agent-impersonation WSS rejection.

**Release decision recorded:** See [Compromised PHP GO/NO-GO gate](TASKDECK_REMOTE_COMPROMISED_PHP_GO_LIVE_GATE.md). The requirement *“PHP cannot authenticate to TaskDeck without a local TaskDeck password”* is satisfied for the **currently implemented, one-shot login-only transport**, subject to source/trusted-client assumptions. But it is incorrect to claim that a fully compromised PHP host cannot initiate a TLS/WebSocket/E2EE **diagnostic** connection; signed Portal ticket issuance is designed to permit that. The protected resource is an authenticated TaskDeck application session or action. A PHP compromise may still cause DoS and deliver phishing HTML.

**No Remote Control live launch:** There is still no app request dispatcher. Scope currently available: PHP Portal account/device/grant administration, presence, diagnostic E2EE echoes and opt-in password verification in a separately built CLI. No HTTP/WS/PTY/File/Git/Patch dispatch. A real Internet-controlled TaskDeck requires a trusted distributed client and separate action-bound authorization/cancellation implementation. Never set `remote_control_available=true` based solely on these security tests.


### Internal task execution admission and prompt revocation/cancellation (2026-10-29)

**Implementation only; Remote Control remains OFF.** Added `internal/server/remote_access_execution.go` as an internal execution gate for a future encrypted command dispatcher. This gate **does not install any HTTP, WebSocket, terminal, file, Git or Patch route**. Callers must provide the exact in-memory `remoteTaskDeckSession` returned by the local TaskDeck password verifier, the original E2EE `Channel`, a recognized TaskDeck project permission and a cancellation-aware callback. A Portal-signed admin ticket, PHP cookie/role, and successful rendezvous are never substituted for a local TaskDeck principal.

**Execution invariants:**
1. Before callback admission, run uncached signed Portal grant introspection and re-read TaskDeck's local password generation, account enabled state, project membership and permission. The grant/Channel/session must still match after asynchronous checks. Only one active callback is admitted on a session; a second concurrent request is rejected instead of queued. Admission and `session.Close()` share a mutex to stop stale late Portal replies starting new work.
2. Each callback receives a context with a **15-second total execution deadline**, including its final authorization check. The session `Close()` cancels the currently active callback's context before closing its grant and E2EE Channel; it does not wait on a possibly compromised or unresponsive PHP API.
3. E2EE `Channel.Done()` fires on explicit disconnect or **any AES-GCM tamper/replay/malformed packet**; all such poison paths now clear AEAD ciphers and session identity bindings. `AgentGrant.Done()` fires once on explicit local revocation or one-shot grant finalization. The bounded action gate cancels immediately on either signal, even if `session.Close()` was not called.
4. On return, run a **second fresh Portal + TaskDeck-local RBAC/password-generation check**, then recheck cancellation, revocation, Channel identity and session lifetime. A revoked account or grant during slow work cannot yield a successful result. This deliberately consumes additional Agent signed-Portal proof quota; call volume/capacity must be considered before releasing an active dispatcher.

**Tests:** `remote_access_execution_test.go` covers no local TaskDeck session, foreign E2EE Channel, viewer attempting terminal, missing live introspection, concurrent callback rejection, local session shutdown cancelling an active callback, late positive Portal response after shutdown, independent E2EE disconnect, signed AgentGrant revocation, operation timeout, and TaskDeck account disable or Portal revoke during work. `remotetransport/e2ee_close_signal_test.go` verifies close/replay/tag tamper/malformed packet notification and irreversible key poisoning. `remoteportal/grant_cancel_test.go` verifies 32 concurrent revocations emit exactly one close signal. Go Remote Agent GitHub workflow runs all three packages with `-race` and the server security suite.

**Important outstanding release blockers:** Cancellation is **cooperative**: a callback that ignores context may have already performed irreversible filesystem or process side effects before revocation. The current gate is unmounted precisely to avoid presenting that as safe Remote Control. Future dispatch must restrict actual capabilities, support safe streaming and backpressure, explicitly cancel running PTYs/process trees/uploads, and handle partial writes/rollback. Portal-side changes during an active callback are detected on the final live check; there is no signed push-revocation stream yet. The independent client is a developer prototype without signed-distribution and update validation. Do **not** set `remote_control_available=true` or declare Remote Control production-ready until these additional gates and real-host penetration tests pass.


**Validation status for the complete execution-gate source:** GitHub Actions [40814149969](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40814149969), code commit `c923d196742c41b45b5c6d832905e803c596e0c4`, completed **SUCCESS**. It covers the Go Remote protocol and server security `-race` tests, E2EE malformed-record cancellation of running TaskDeck callbacks, grant closure under 32 goroutines, build checks and macOS/Linux ARM cross-compilation. This green run verifies the **unmounted security gate**, not an installed public Remote Control dispatcher.


### Staged encrypted, allowlisted TaskDeck action dispatcher — 2026-10-29

**Status: IMPLEMENTED INTERNALLY; NO PUBLIC ROUTE; NO REMOTE CONTROL.** This milestone does not turn on remote UI, WebSocket action dispatch, terminal, filesystem, Git, Patch, or command invocation. It exercises a secure end-to-end *in-process* application authorization path to reduce the remaining release integration risk.

- `internal/remotetransport/action.go`: authenticated AES-GCM request and response types `taskdeck-e2ee-action-v1` and `taskdeck-e2ee-action-result-v1`. The sole allowed `operation` is `session.ping`, without paths, bodies, commands, URLs, shell arguments or data reads. Envelopes must have exact JSON keys with no duplicate keys, ambiguous case, unknown fields, trailing tokens or malformed request ID (32 lowercase hex). Invalid decrypted messages poison/close the authenticated channel. The fixed response contains no cookie, bearer token or TaskDeck data and explicitly declares `remote_control_available=false`. Browser/client responses are also strictly checked.
- `internal/server/remote_access_dispatch.go`: unmounted `executeRemoteE2EEPing` takes **the exact TaskDeck locally password-authenticated in-process principal and E2EE channel**, not a PHP role/ticket/HTTP principal. It decrypts strict action schema, invokes the existing per-action gate with `tasks.view` permission (fresh Portal HTTPS check and local RBAC/password generation before and after), and seals one bounded token-free reply. The final encryption is serialized against local `session.Close()` to avoid local shutdown interleaving with response creation.
- Locally authenticated successful ping authorization is audited by TaskDeck's own Identity store as `remote.action.session_ping`, with true TaskDeck user/project IDs, `Result=authorized`, and **without** browser request IDs, plaintext usernames/passwords, signed tickets, or session keys. Audit failure fails closed. Slow audit I/O runs *outside* the session shutdown mutex so local revocation is never blocked by the storage operation. The audit does not assert the response was delivered if revocation occurred after the authorization.
- `internal/remotetransport/action_test.go`: valid E2EE roundtrip/replay, forged command/file/Git/HTTP operation types, extra/duplicate JSON fields, mismatched response IDs and injected token fields.
- `internal/server/remote_access_dispatch_test.go`: actual TaskDeck shared-user password proof + project permission and encrypted response, PHP Portal ticket without local password denied, unrelated E2EE channel denied, disabled TaskDeck account denied, revoked Portal grant denied, forged encrypted terminal operation denied, stale compromised Portal `allowed=true` during local session Close denied, and local audit identity/credential exposure regression.

**Still NO-GO for public Remote Control:** the existing native login client remains a one-shot diagnostic and does not obtain a long-lived application principal. No production Agent-Relay authenticated action loop is installed and no native GUI/binary distribution signature is available. A real dispatcher must use the narrow E2EE protocol for each explicitly reviewed capability, enforce cancellation on process/file/stream handlers, and retire the session at logout/timeout/revocation/disconnect. The current session ping is *not* a public feature.


**GREEN CI — staged read-only application path:** [TaskDeck Remote Agent (Go) run 40816666503](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40816666503) completed **SUCCESS** on code commit `afa47ff43dc1d52cb6662b065a41f9078183cefb`. Validated Go Remote Agent/Relay/transport `-race` tests (strict action envelope and cryptographic replay rejection), TaskDeck server `-race` tests (correct local-password-authenticated E2EE ping and TaskDeck-local audit, PHP-ticket-only failure, revocation/identity changes during work), UI boundary tests, daemon build, Relay and local client builds, macOS/Linux ARM cross-compiles. Docs-only subsequent commits do not alter the validated source.


### Independent native TaskDeck session admission (internal, OFF) — 2026-10-29

**Security boundary:** `StartEncryptedRemoteSession` is a new, **unmounted**, TaskDeck-server-owned path distinct from the existing login-only diagnostic. It takes only the actual local `remoteportal.Agent`, its verified `AgentGrant`, the matched established E2EE Channel and a sealed TaskDeck username/password envelope. PHP/Relay have no access to the session pointer and receive no cookie, local identity, token or project data; the only client reply remains the existing strictly checked AES-GCM encrypted boolean.

Before decrypting credentials, the Agent atomically admits exactly one login attempt on its pinned signed Portal grant, performs fresh HTTPS introspection and charges the persistent device-wide scrypt budget (shared `Agent.AdmitEncryptedLogin` also used by legacy boolean probe). The TaskDeck server then decrypts locally, verifies the real scrypt password, current account enablement and project membership, and consumes the same one-use grant. It pins the session's live-check callback to that **exact Agent**; an unrelated Agent, signed PHP admin role, ordinary HTTP principal or E2EE channel cannot substitute. A second live Portal introspection is required before returning an encrypted positive boolean. Denial closes both grant and channel; successful local session is never serialized or stored at PHP.

A successful session has a hard five-minute deadline and its own local termination signal. A watcher calls `Close()` on the first event among AgentGrant revocation, E2EE disconnect/tampered ciphertext, explicit logout and deadline. `Close()` cancels active bounded actions and destroys the E2EE identity binding; the original one-shot diagnostic still retires its temporary principal and does not yield application access. The former Relay error-on-intentional-context-cancellation regression was corrected without changing the denial of unauthorized presence.

**Security regression:** `internal/server/remote_access_session_login_test.go` provisions a real Agent identity seed and owner-pinned signer, signed Portal admin/viewer tickets, a real TLS Portal endpoint that returns `allowed=true`, and a TaskDeck Shared Identity password database. Tests reject a stolen-PHP admin ticket with wrong TaskDeck password; accept only a true local password and an encrypted token-free boolean; require two Portal checks during login and two more for encrypted `session.ping`; verify viewer cannot create terminals; and verify session closure when grant or E2EE disconnects. Another fixture tests deadline expiry with a short time override. The Agent's signed ticket subject must be the canonical Portal numeric user ID, not arbitrary text.

**Release status:** STILL NO-GO for public Remote Control. This helper is not configured into the Agent-Relay long-lived network loop. The native CLI remains a *one-shot* diagnostic. The work to connect authenticated session lifecycle across the Relay, trusted native client and per-action dispatch with bounded stream/cancellation remains incomplete. Do not use PHP-served JavaScript to collect TaskDeck credentials.


**CI proof for independent staged session:** GitHub Actions [40817879376](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40817879376) **SUCCESS** on source commit `23324bbc855f6592414e8b94376266094f6c78fe`. Go race-enabled Agent/Relay/transport and server security tests passed, including full local-password E2EE login, forged Portal admin ticket denial, successful encrypted read-only `session.ping`, auto-revocation, exact identity-pinned Agent checks, Relay cancellation and cross-builds. Later regression adds password-hash rotation (including backdated timestamp) invalidating a live authenticated session; its new CI must pass separately before claiming full completion of that addition. **No public Remote Control route or trusted native long-lived client has been activated.**

**Final staged-session CI:** [Go Remote Agent run 40818162152](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40818162152) **SUCCESS** for source commit `87900fb69a662a025c780bec2f6aecbc43afa4f5`, including the additional backdated local TaskDeck password reset regression, Go `-race` server/Agent/Relay/transport suites, protocol tests and macOS/Linux ARM cross-builds. Source tested with HTTP/PTY/File/Git/Patch public dispatch **still disabled**.


### Async PHP response: fresh TaskDeck-local password and membership check before session issue (2026-10-29)

**Finding:** In staged `StartEncryptedRemoteSession`, the real TaskDeck password/scrypt verifier finished before the Agent made its second, uncached HTTPS Portal grant check. A completely compromised PHP Portal could intentionally stall that check while the TaskDeck owner rotated a local password, disabled the user or revoked project membership, then return `allowed=true`. The old implementation rechecked grant/channel liveness but not the **local TaskDeck principal** immediately before sealing and returning the login-success result. While later per-action authorization still denied stale credentials, a briefly authenticated session and boolean could be issued based on a superseded identity.

**Fix:** `verifyRemoteSessionLocalPrincipal` is an in-process, PHP-independent guard that re-fetches enabled TaskDeck user, compares exact locally verified password-hash commitment and account update generation, resolves current project membership, and validates local authenticated E2EE/grant/session lifetime. The per-action guard now uses this shared function. `StartEncryptedRemoteSession` invokes it after the second Portal HTTPS reply and again at the final encrypted-success boundary. Any mismatch fails closed, closes AgentGrant and E2EE, and retires the ephemeral local session. No session/credential/token is sent to PHP.

**Regression:** `TestRemoteStagedLoginDeniesPasswordRotationDuringLastPortalGrantCheck` deterministically blocks the second HTTPS response, rotates the TaskDeck **local** password with a deliberately backdated password-change timestamp, then releases PHP's stale `allowed=true`. It requires no encrypted success, no retained local session, and closed E2EE/grant. Check GitHub Actions for source commit `7a789e881e16fc29182c12fdd16de7e709dc8fc2`. Release status stays **Remote Control NO-GO**: a dispatcher still needs vetted client/stream/action implementations, bounded cancellable side effects and integration tests.

**Additional local membership race test:** `TestRemoteStagedLoginDeniesTaskDeckMembershipRemovalDuringLastPortalCheck` closes a real local project membership while the deliberately compromised PHP Portal's second HTTPS `grant-check` is blocked; even when PHP then returns `allowed=true`, no authenticated TaskDeck session or encrypted success is issued, and Agent grant/E2EE are closed. The test was committed at `c7365e13fef67964b9845242a37f578c5b8c5e3f`; CI confirmation must match this commit.

**CI failure triage (2026-10-29):** Source run [40819356857](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40819356857) revealed an existing expiry-test synchronization race: `remoteTaskDeckSession.Close()` publishes `closed=true` to block new action admissions **before** its asynchronous cleanup closes PortalGrant and E2EE. The test incorrectly asserted cleanup immediately after seeing that flag. Corrected `TestRemoteSessionLifetimeExpiresWithoutPeerDisconnect` to wait until the flag AND grant/E2EE revocation are all observable, with a one-second deadline; this preserves the necessary fail-closed admission ordering. Code fix/test stabilization at `3db6fdbcc90c72b252160544676554a336d0a175`. Rerun Go CI before claiming green status for the new staged-login regressions.


**CI cuối cùng (29/10/2026):** [TaskDeck Remote Agent run 40819715312](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40819715312) **SUCCESS** cho source commit `3db6fdbcc90c72b252160544676554a336d0a175`, gồm kiểm thử race Agent/Relay/transport, kiểm thử TaskDeck server với local password và membership bị thay đổi khi PHP trả grant-check muộn, hồi quy cleanup khi session hết hạn, build CLI/Relay và cross-compile macOS/Linux ARM. Các commit tài liệu sau đó không làm thay đổi source đã kiểm chứng. **Kết luận chỉ cho ranh giới xác thực**: Portal bị chiếm quyền và có khóa ký/ticket admin nhưng không có mật khẩu TaskDeck thì không thể tạo phiên ứng dụng được TaskDeck chấp nhận. Trường hợp người dùng bị phishing lộ mật khẩu hoặc máy chạy TaskDeck bị chiếm quyền nằm ngoài bảo đảm này. Không có public Remote Control dispatcher, vì vậy vẫn **NO-GO** cho triển khai phiên điều khiển Internet.


### Trusted CLI: strict untrusted Relay envelope parsing + double Agent proof (2026-10-29)

The independently compiled `cmd/taskdeck_remote_login` client now runs `decodeRelayObject` before interpreting either a purported Agent proof or the encrypted login boolean response. The bounded decoder rejects duplicate JSON keys **at every nesting level**, unknown typed fields, appended/trailing JSON values, excessive nesting/tokens and envelopes over 12 KiB. The decoder additionally enforces a canonical case-sensitive key allowlist, because Go's ordinary JSON struct matching accepts case-folded aliases even when `DisallowUnknownFields` is enabled. This prevents a compromised PHP/Relay control plane from exploiting disagreement between JSON parsers or smuggling a future cookie/token/command field into an otherwise valid response.

The client independently verifies **two Ed25519 signatures** from the locally pinned TaskDeck Agent: (1) the explicit browser reachability attestation bound to the exact Device ID, probe ID and local nonce; and (2) the ECDH handshake signature over both ephemeral public keys and challenge data. The attestation's public key must match the ECDH identity exactly; the client must authenticate the AES-GCM encrypted ready record before asking for a TaskDeck password. An untrusted Portal may produce tickets, set role=admin and lie about Agent state, but cannot forge either Agent private-key signature. The legacy PHP-origin credential form remains removed.

Added client regressions for forged/incorrect attestation nonce, tampered attestation signature, duplicated root/nested JSON keys, unknown injected bearer fields, trailing objects and duplicate/extra fields in the encrypted result wrapper. Source changes: `cmd/taskdeck_remote_login/strict_json.go`, `main.go`, `main_test.go`. Verify the **latest** GitHub Actions Go Remote Agent workflow before treating these as accepted; earlier intermediate CI builds with incorrect test fixture line delimiters failed and were fixed. These changes still provide only one-shot authentication, not remote application capability. **Remote Control remains NO-GO.**


### Relay-to-Agent strict JSON admission under compromised PHP (2026-10-29)

The signed Portal/Relay origin is explicitly **attacker-controlled** in this threat model. Hardened remoteportal/relay.go with decodeRelayCommandFrame, which rejects duplicate JSON keys, case-folded aliases, Unicode-escaped duplicate aliases, additional command/token/credential members, nested values, trailing records and noncanonical or missing fields. The only accepted three command types are reachability_probe (requires type/probe_id/client_nonce/browser_key/portal_ticket), e2ee_login and e2ee_echo (each requires type/probe_id/ciphertext). The parser performs no TaskDeck password verification and cannot grant any application session; the separate local TaskDeck scrypt and RBAC gates remain mandatory.

Added 21 legitimate/forged payload cases in internal/remoteportal/relay_test.go. [Go Remote Agent CI 40826224243](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40826224243) **SUCCESS** on source commit 3ad0b73595feddf71fd5836379593cb8242be197 (Go -race remote Agent/Relay/transport, server tests, WebCrypto/Go interoperability, native CLI builds, macOS/Linux ARM cross-compiles). The two earlier runs failed due to a duplicate io import during iteration, which the successful commit fixes.

**Assurance and limit:** Compromised PHP signing keys and Portal admin grants still cannot substitute for an actual TaskDeck-local password or project RBAC. They may initiate a WebSocket/E2EE diagnostic handshake, exhaust login budgets or cause denial of service, and can phish via malicious PHP content. Never reuse the PHP Portal password for TaskDeck, and never enter TaskDeck credentials into PHP-hosted pages or unverified downloads. The trusted local client must pin the Agent Device ID out of band. The internal TaskDeck action dispatcher remains unmounted, and public Remote Control remains **NO-GO**.


### Compromised-PHP authentication bypass re-audit (2026-10-29)

Source commits f6534445bfeadef564f358dc2b810ed4b8666aba, 553c44357fe16136a9e643c2c5e9ca8c02b8573b, 877b5d07a1fe1a4e525a21c447c1d242c69f5865. A forensic pass reviewed PHP control-plane role/tickets, Relay-to-Agent E2EE mailbox, local TaskDeck scrypt + membership, staged session action gate and real TaskDeck HTTP shared-auth cookie middleware.

Found a **stale one-shot login-boolean race**: the local account/password/project may change during slow TaskDeck-local success audit after scrypt verification. The one-shot verifier now checks fresh local user/password-hash commitment and project membership immediately before sealing its boolean; a mismatch gives encrypted authenticated=false. Test blocks audit deterministically and revokes by password rotation (backdated change timestamp), account disable and project-membership removal, independently of a Portal admin ticket. This is a correctness/security-hardening improvement, **not evidence** that a real remote application command was executable before it.

Added a separate real TaskDeck HTTP middleware regression. Even with a Portal admin ticket signed using a key controlled by the PHP attacker, and carried in Authorization bearer, a forged TaskDeck cookie and purported admin headers, seven TaskDeck API routes (including config, tasks, sessions, account and file) must refuse access. No HTTP session cookie can be minted using only Portal authority.

**Validated:** [Go Remote Agent GitHub Actions 40832980357](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40832980357) **SUCCESS** on code commit 877b5d07a1fe1a4e525a21c447c1d242c69f5865, including Go race tests, server HTTP/UI/E2EE security, JS-WebCrypto interop, native CLI builds and Linux ARM/macOS cross-compiles.

Threat-model boundary still holds for *actual application access*: compromised Portal control/signing key and a forged role cannot substitute for the TaskDeck-local password, session cookie and RBAC; the hacker can still initiate WSS diagnostics or cause denial of service and can phish an operator on an untrusted Portal page. A compromised PHP host with an offline-cracked **reused** TaskDeck password is outside this no-password assumption. Production Remote Control dispatcher remains **unmounted / NO-GO**, and there has been no independent production penetration test.

 
### Client software-key approval via TaskDeck host CLI — 2026-10-29

Requirement: Remote login must use the independent native CLI and a TaskDeck-local approved Ed25519 software-client key **as well as** the TaskDeck-local password. Portal/PHP may issue rendezvous tickets but cannot create/approve/revoke TaskDeck client keys. Every client public key is scoped to one local TaskDeck username and one pinned Agent Device ID; private seeds never leave the local client (0600 in 0700 private directory). On first creation the native client displays copyable public key/fingerprint in OS dialog (macOS osascript, Linux zenity/kdialog) or terminal fallback and **does not ask for TaskDeck password**.

Host CLI commands, executed under the OS user running the TaskDeck daemon:
```bash
taskdeck remote-key --workspace /absolute/project list
taskdeck remote-key --workspace /absolute/project add alice BASE64_ED25519_PUBLIC_KEY "Office laptop"
taskdeck remote-key --workspace /absolute/project revoke 64_HEX_FINGERPRINT
```
Per-workspace private registry: owner-only `taskdeck/remote-identities/<SHA256 workspace>.clients.json`, symlink-safe, flock and atomic fsync. Host CLI is local only; no PHP/Relay/HTTP API changes its contents.

Client receives the pinned Agent Ed25519 E2EE proof, then sends a one-time **encrypted software-client-key preflight** with a signature of the channel transcript. Agent checks local allowlist and returns an **AES-GCM authenticated approved boolean** before client ever asks for a password. Unapproved or revoked key triggers copyable public key guidance and zero password prompt. A compromised PHP/Relay cannot forge approved=true. For accepted keys, client sends encrypted TaskDeck password **plus** the same Ed25519 channel proof in strict V2 envelope. TaskDeck server revalidates local client key and password independently; legacy V1 password-only records are denied even with correct password. Session authorization re-reads the host allowlist each action; expired/revoked key denies. One-shot diagnostic also revalidates key after audit to close revocation race.

Tests: Registry owner-only storage, CLI add/list/revoke, wrong account, unapproved attacker key, key signature replay on foreign E2EE channel, stolen password with legacy V1, direct key revoke, revoke during local audit, E2EE preflight forged statuses, strict JSON and real TLS/WSS Relay-Agent login with host-approved key. Workflow `.github/workflows/taskdeck_remote_agent.yml` includes new package race tests and CLI tests. CI link / final completion status must be checked against newest code commit; do not treat previous partial commits as validating all later changes.

Release gate unchanged: Remote Control dispatcher **unmounted and production NO-GO**, password never entered into PHP-supplied page; only independent native client may prompt after real Agent proof and host key approval. No unaudited fallback, no Portal key auto-registration.

 
**Validated release-security gate:** [GitHub Actions run 40845687947](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40845687947) **COMPLETED SUCCESS** on source commit `f6c3ace2376100e9b8b4dbf67bbbb6c7cd7c6851`. Host key allowlist/CLI, cryptographic E2EE preflight and password gating, client enrollment popup, stolen-password and replay negatives, audit-time key revocation, CI race checks and macOS/Linux portable builds all checked. Production Remote Control still **not available**; keep dispatcher disabled until separately audited.


### Security round: Unicode account names, strict policy JSON and immutable host user IDs (2026-10-29)

Found a genuine account-binding discrepancy between Go Unicode `strings.EqualFold` and SQLite's ASCII-only `COLLATE NOCASE`. A TaskDeck-local key approved for `Karl` could be incorrectly treated as approved for distinct SQLite user `Karl`, especially important when PHP could phish the second user's password. Fixed all local client-key comparisons, proof-message username normalization, and login rate-limit normalization using ASCII-only SQLite-compatible folding. Added *real SQLite and real E2EE* negative login test plus other Unicode case tests.

Host registry now rejects duplicate, case-aliased, unknown and trailing JSON members (including nested entries). A corrupt local host policy fails closed. Added regression tests for eight ambiguous-file formats.

Further fixed the account-deletion/recreation case: host CLI enrolls only existing, active local SQLite users with current project membership, recording their immutable `users.id` along with public key. Reusing the same username for a new account does NOT inherit the prior client approval. TaskDeck rechecks the user ID together with the registry before login success and each staged action. Legacy username-only keys remain listable but are denied by Agent preflight until deliberately re-approved by the host CLI, with no PHP fallback. Tests cover user-ID replacement, revoke/re-approve, legacy migration and real host CLI identity lookup.

Code commits include `74e4f82a` (strict policy), `85f37b68` (ASCII-only account folding), `5b69ac1f` (server principal matching), `3c7be8ad` (real SQLite Unicode regression), `af390c88` (immutable User.ID entries), `17c705bd` (server ID gate), `4fd568c8` (host CLI local SQLite resolution), `8abd3ff6` (legacy preflight fail closed), and `34223d11` (real Relay test fixture for new approvals). **CI [40851369672](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40851369672) COMPLETED SUCCESS** on exact latest code `34223d1189073e43764b3da28b80d83b57955d9e`, including host CLI SQLite-binding tests, strict registry/Unicode tests, E2EE negative authentication, real Relay TLS integration, race checks and macOS/Linux cross-platform builds.

Residual preflight UX: Agent's isolated E2EE check knows username/key but does not independently query current SQLite users.id. After a username is reassigned, the old key could reach the password prompt, yet the final immutable user-ID check refuses to create a TaskDeck session. Production Remote Control stays **NO-GO**, public application command dispatcher unmounted.


### Follow-up: SQLite-verified immutable user ID before software client password prompt (2026-10-29)

Closed the preflight gap noted in the prior status update. Previously the Agent's AES-GCM key-check only matched a nonempty stored User.ID in a host allowlist but did not compare to *the currently active SQLite TaskDeck identity*. When an account was deleted/recreated with the same username, this could unnecessarily trigger the trusted client's password prompt; the final TaskDeck authentication already denied session creation.

Now `cmd/vscode_tasks_menu/main.go` configures a **host-local** Agent `ConfigureClientKeyLocalUserLookup(srv.LookupRemoteClientLocalUserID)` in the explicitly opted-in login-only path. The server lookup reads the real local SQLite user, project and membership, checks enabled state, and returns the immutable User.ID. Agent validates the E2EE transcript signature and `remoteclientkey.IsApprovedForUser` against this ID before it sends the encrypted `approved=true` reply. A missing lookup callback **fails closed**; no PHP ticket or JSON field can substitute for this trusted function. User-ID replacement, revoked membership or deleted key can no longer show positive preflight or solicit a password. Login V2 independently re-verifies local identity, password, private-key proof, User.ID, and the allowlist.

New files: `internal/server/remote_access_client_identity.go`, `internal/server/remote_access_client_identity_test.go`, `internal/remoteportal/client_approval_test.go`. Modified Agent config, Relay preflight, main daemon opt-in wiring and TLS/WSS fixture. Test coverage includes real TaskDeck SQLite account/project/membership states, account recreation, missing callback, re-enrollment, cross-account and cross-session signatures. **CI [#40855540805](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40855540805) COMPLETED SUCCESS** on tested code commit `35e6e123620e71c4631a1d61c9eff652a127decd`: race-enabled Agent/Relay/transport, Remote HTTP/UI/auth, trusted CLI and Linux/macOS builds.

No public TaskDeck Remote Control route was enabled. Production Remote Control remains NO-GO.


### Final scope closure: owner-approved software client keys (2026-10-29)

**Completed and verified for the native Remote LOGIN path.** The native client creates a local Ed25519 private seed automatically on first use, shows only the copyable public key/fingerprint in a native macOS/Linux dialog (TTY fallback), and requests **no TaskDeck password** until its registered key passes Agent-signed E2EE preflight. Host-only `taskdeck remote-key --workspace /absolute/project add|list|revoke` manages a private per-workspace allowlist bound to the actual immutable SQLite TaskDeck User.ID. PHP Portal/Relay cannot approve keys, replace the TaskDeck password verifier, or assert its own positive key status. Stolen TaskDeck password alone, legacy V1 password-only login, foreign key/signature, stale username-only entries and recreated accounts fail closed. A revoked key also fails staged-session reauthorization.

**Final UX correction:** After successful positive Agent preflight, a subsequent incorrect password / account or key revocation returns a login rejection **without showing the misleading unapproved-key popup**. The popup remains exclusive to first-time key generation or actual negative Agent-approved-key preflight. Code commit `f7cd848c7d6adea783ee7095825c0512e237357b`.

**Final CI:** [TaskDeck Remote Agent run 40856688278](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40856688278) **COMPLETED SUCCESS** on the exact final source commit `f7cd848c7d6adea783ee7095825c0512e237357b`. Verified race-enabled key/Agent/Relay/transport tests, real SQLite identity and Remote login security, native host CLI, JS crypto interoperability, native client build, and macOS/Linux ARM cross-compile.

**Scope limit:** Login is currently a **one-shot E2EE diagnostic**, with no TaskDeck application token or public Remote Control dispatcher; native client currently uses a manual Portal ticket. The protected Remote Control application session remains **NO-GO** and is not silently enabled by this change. No more implementation work is required for the narrowly specified **host-approved client key enrollment / remote-login gate** feature.


### Release-security verification refresh — 2026-10-29

Final application source `93536acabd61aa37a0f46cdc3d47f2d5d108ecc8` — both [Go Remote Agent 40859169884](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40859169884) and [PHP Remote Portal 40859169940](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40859169940) completed **SUCCESS** on the SAME code commit.

**Security hardening since prior closure:** broaden server CI to run previously excluded `TestCompromised*` attacker cases; mandate positive Ed25519/E2EE host client-key preflight before *any* Agent-to-TaskDeck password callback (the TaskDeck-local V2 check remains independently enforced); malformed or out-of-order login now rejects only the current browser reservation and does not disconnect the authenticated Agent. A real TLS/WSS test exercises skipped preflight, malformed login and valid login recovery; durable Agent login CPU quota and Relay frame flood budgets remain strictly bounded and tested separately. PHP CI is automatically re-run on Go Agent/Relay/protocol changes, avoiding mismatched PHP/Go release verification.

See [TASKDECK_REMOTE_RELEASE_READINESS_20261029.md](TASKDECK_REMOTE_RELEASE_READINESS_20261029.md) for objective gate evidence and uncompleted release prerequisites. **LOGIN-ONLY is eligible for controlled opt-in testing** using a trusted native binary and a host-approved software key. **Remote Control application access is NOT READY and MUST remain unmounted/NO-GO** until secure app transport, per-action dispatcher/RBAC, session lifecycle, trusted binary distribution, actual Linux/macOS deployment tests and independent hostile-PHP penetration review are implemented and verified. Do not confuse `TASKDECK_REMOTE_LOGIN_PROBE=1` with enabling application control.

## 2026-10-30 — Firefox offline authenticated TaskDeck password login-only (PHP-compromised threat model)

### Mục tiêu và phạm vi

- **Trusted bootstrap**: lần pair đầu tiên PHP chưa bị hack. Firefox
  lưu `remote_portal/offline/taskdeck_offline_client.html` thành tệp
  **local**, vault được mã hóa; các lần sau KHÔNG tải HTML/JS từ PHP.
- **Threat model**: sau khi pair, toàn bộ PHP HTML/JS/SQLite/ticket/role
  và Relay forwarding có thể bị hacker sửa. Khóa Ed25519 Agent ở host,
  private key client trên trình duyệt local, TaskDeck password/SQLite
  và host key registry vẫn phải độc lập không bị chiếm.
- Đợt này thêm **E2EE TaskDeck password login-only boolean** cho client
  Firefox offline sau khi Agent và private key client được xác thực;
  KHÔNG cấp remote session token, cookie, TaskDeck action hoặc quyền
  Terminal/Files/Git/Patch.

### Đã triển khai trong mã

1. `vscode_tasks_menu_go/cmd/taskdeck_remote_relay/main.go`:
   cờ `-allow-offline-login-probe` **mặc định false**, chỉ chấp nhận
   nếu đồng thời bật `-allow-offline-file-client`. Đây là opt-in
   chẩn đoán login-only, không thay đổi tính năng native login.
2. `vscode_tasks_menu_go/internal/remoterelay/http.go`: trên
   `/v1/offline`, `Origin:null` chỉ là header origin, **không**
   được coi là danh tính. Relay chỉ chấp nhận login có encrypted
   `e2ee_client_key_check` trước đó; host Agent độc lập xác minh
   approved key. Khi cờ login tắt, ngay cả login sau preflight
   cũng bị chặn. Relay không giải mã mật khẩu hay cấp quyền ứng dụng.
3. `remote_portal/offline/taskdeck_offline_client.html`:
   Firefox chỉ hiển thị **hộp nhập mật khẩu local** sau khi xác
   minh Agent Ed25519/ECDH và nhận `approved:true` *bên trong*
   Agent-authenticated AES-GCM, ký bằng host-approved client Ed25519
   private key trên chính transcript E2EE.
   Hộp mật khẩu có nút hủy/thời hạn 20 giây, xóa input khi đóng,
   bị hủy khi WSS hết hạn. Trình duyệt gửi encrypted
   `taskdeck-e2ee-login-v2` gồm TaskDeck username, password,
   client_public_key + signature mới trên channel binding.
   **Mỗi kênh một key preflight, một login và một result**. Mã
   kiểm `taskdeck-e2ee-login-result-v1` với boolean
   `authenticated`, bắt buộc `remote_control_available:false`.
   Không có bearer token/cookie hay hành động ứng dụng.
4. `remote_portal/tests/offline_e2ee_crypto.test.cjs`:
   chạy thực mã JS nhúng trong HTML, Agent giả lập Ed25519, ECDH,
   HKDF/AES-GCM bằng Node WebCrypto, kiểm login v2 trên đúng
   transcript hiện thời, chống gọi password trước preflight,
   host denied, forged login ciphertext, replay và trả
   `remote_control_available:true`.
5. `vscode_tasks_menu_go/internal/remoterelay/browser_frames_test.go`:
   kiểm các loại frame Terminal/File/Git/Patch không được nhận,
   login-only phải có preflight, login mặc định tắt.

### CI / evidence

- Chốt Go/Relay tại source `1352258b7200dbd196a536d67cbbca5593fd24ca`:
  [Go Agent/Relay #41006883306 — PASS](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41006883306),
  [PHP Portal #41006883310 — PASS](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41006883310)
  cùng code commit (bao gồm flag và fallback).
- Frontend `bd874842` Node/PHP **PASS** qua
  [#41007008017](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41007008017).
- Kiểm thử mới ban đầu FAILED vì lấy transcript của **phiên E2EE
  khác** trong assertion, không phải lỗi mật mã production;
  đã sửa ở `a8326a8c`. Kết luận kiểm thử hiện hành phải
  dựa trên workflow của đúng source mới, không gắn nhãn PASS
  trước khi CI hoàn tất.

### Phần vẫn chưa hoàn thành (NO-GO Remote Control)

- Chưa có thử nghiệm trên Firefox file:// thật / NAT/CGNAT /
  Linux/macOS Agent với Relay production và mật khẩu TaskDeck thực.
- Chưa có authenticated application WSS dispatcher/Terminal/Files/
  Git/Patch/Collection, per-action RBAC/revocation, bounded stream
  và độc lập red-team full compromise PHP.
- Chưa chứng minh signing provenance và update/backup client local
  trong môi trường production hoặc thiết bị thật.

**Release gate:** `remote_control_available=false` giữ nguyên;
login-only không chứng minh 100% an toàn hoặc đủ chuẩn live.
