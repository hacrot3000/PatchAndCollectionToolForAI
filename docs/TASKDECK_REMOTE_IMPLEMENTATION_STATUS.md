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

**Still not wired:** Portal ticket delivery as an encrypted *application* message (currently the already-Relay-visible signed rendezvous ticket is passed to Agent in the diagnostic probe), TaskDeck login requests from the browser, Agent→TaskDeck authenticated dispatcher, continuous session revocation, E2EE transport for arbitrary HTTP/WebSocket payloads, and the complete role/permission checks in actual routed handlers. A signed ticket and public ECDH key are *not* enough for local TaskDeck login: the Agent must first authenticate at least one ciphertext message from the browser to prove its private-key possession. The present Relay protocol deliberately routes only fixed encrypted echo diagnostics. The Agent ticket replay cache is currently process-memory-only; the Relay's own persistent replay ledger remains a separate protection. Do not enable Remote commands based on these internal helpers.

### Signed browser rendezvous protocol v2 (incompatible with old tickets)

The Portal now signs only version-2 browser tickets, which include a commitment to a browser-generated 128-bit nonce and **ephemeral P-256 public key**. The browser must generate both *before* `POST /api/remote/ticket`, submitting `client_nonce` (lowercase 32 hex characters) and `browser_key` (canonical base64 of the 65-byte uncompressed P-256 public point), alongside the authorized `device_id` and CSRF token. A ticket cannot be used with any other nonce/key. The Portal validates encoding; Go checks that the P-256 point is on curve. Relay burns one-time tickets even when an attempted browser binding is incorrect.

The Relay forwards the signed ticket with the already-authenticated encrypted-session probe, and Agent independently validates it using its locally pinned Portal trust root, issuer, Device ID and its own ECDH channel binding. This forwarded signed token is *not a TaskDeck password* and is already visible to Relay at initial rendezvous. **TaskDeck password delivery remains disabled.**

The login grant can only be consumed after Agent has verified a browser-originated AES-GCM message, proving that the real browser holds the ephemeral ECDH private scalar. Legacy version-1 rendezvous tickets are rejected by both Relay and Agent, so any custom clients or old Portal deployments must update together. In a future command transport the Portal role remains a ceiling only; TaskDeck SQLite login and per-command RBAC are still mandatory. Do not treat the successful three-round diagnostic echo as permission to invoke TaskDeck APIs.
