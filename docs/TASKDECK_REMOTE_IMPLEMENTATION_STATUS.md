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
| E2EE envelope core | internal/remotetransport | P-256 ephemeral ECDH, Agent Ed25519 session signature, AES-256-GCM ordered records, 32 KiB max plaintext. **Only the ready proof is wired through Relay** |
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

The public key printed by the second command (base64-encoded 32-byte Ed25519 public key) is used on the Relay. **NEVER copy the signing seed to Relay or commit it to Git.** REMOTE_PORTAL_ISSUER must equal the exact HTTPS Portal origin, for example https://portal.example.com.

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

CI files: .github/workflows/taskdeck_remote_agent.yml and taskdeck_remote_portal.yml. The green CI verifies covered behavior only; it is not a penetration test or release approval.

## 5. Release blockers — do not mark READY

1. Browser↔TaskDeck transport with HTTP methods, binary uploads, WebSockets, PTY, Patch, Git, files and correct CORS/Origin/cookie/session behavior, without arbitrary host proxying.
2. End-to-end TaskDeck local authentication and action-level RBAC for every HTTP/WS command: Portal permission must only narrow TaskDeck's own rights. No bypass for terminal or settings.
3. End-to-end encryption is proven only for the fixed diagnostic handshake. Extend the verified P-256/AES-GCM channel to *all* future HTTP/WebSocket/application frames before enabling remote commands. Browser-delivered dynamic JavaScript supply-chain trust remains a separate risk.
4. Portal MFA/passkey, abuse throttling, password recovery, key rotation; explicit ownership verification on pairing/recovery.
5. Revocation of live sessions, bounded lease, connected device shutdown and replay prevention across Relay process restarts or cluster replicas.
6. Stream/backpressure limits, authorized multiplex, agent restart/reconnect without repeating non-idempotent operations.
7. Portal password rotation is implemented; the production MFA/passkey, account recovery, request throttling and signing-key rotation controls remain outstanding.
8. Security integration tests on real shared hosting and a reachable Relay under NAT/CGNAT, plus independent security review.

**Do not claim full Remote Access is implemented before each release gate is satisfied.** The browser-to-Agent probe confirms only authenticated reachability. It does not expose TaskDeck terminal/file/HTTP APIs, nor does it prove that TaskDeck user authentication over the tunnel works.

### Diagnostic state distinctions

- **Portal online:** a correctly signed TaskDeck heartbeat was received within the presence TTL.
- **Agent reachable:** after a signed Portal ticket, the browser creates a fresh 128-bit random nonce, and the Agent signs `taskdeck-browser-attestation-v1\n<Device ID>\n<probe ID>\n<browser nonce>` using its per-instance Ed25519 identity key. The browser verifies the Ed25519 signature through WebCrypto and checks `SHA-256(public key) == Device ID` and the exact nonce/probe ID. The Relay cannot fabricate this proof using only its own keys. The browser additionally creates a **non-extractable P-256 private key** and sends only the 65-byte uncompressed public key. The Agent signs the P-256 key transcript with Ed25519, derives ephemeral ECDH/AES-256-GCM secrets, and encrypts the fixed `taskdeck-e2ee-ready-v1` proof. Relay verifies the Agent signature, then forwards only the opaque AES-GCM ciphertext. The browser independently authenticates the signed handshake, derives its own keys and decrypts the ready proof. Unsupported/disabled WebCrypto Ed25519/ECDH/AES-GCM fails closed; it must **never** fall back to trusting a Relay-only `agent_reachable` flag.
- **Remote control available:** always **false** in this branch. The E2EE ready proof is a fixed diagnostic payload; **no TaskDeck HTTP request, terminal data, file or user credential is yet carried inside the encrypted channel.** Future application transport must reuse authenticated per-session keys, enforce bounded streams and require TaskDeck authentication/RBAC independently.

### Relay session lifecycle and quotas

The Relay enforces an upper bound of 256 device sockets and 64 browser sockets, including unauthenticated WebSocket handshakes. A browser closing its WebSocket while waiting for an Agent proof releases its reservation immediately (instead of waiting 27 seconds). Each reservation has its own generation counter; delayed cleanup from an old browser cannot cancel a newer reservation on the same Agent. Corrupt replay markers and write failures fail closed. These controls reduce resource-exhaustion and replay risk, but do **not** replace MFA, per-user/IP ingress throttling, live revocation or full stream-level permissions.

### Security limitations of browser attestation

The new signed proof authenticates **which Agent answered the browser nonce**; it does not authenticate a user with TaskDeck or grant permission to run any command. The Portal serves the diagnostic JavaScript dynamically: an attacker who compromises Portal content could replace that JavaScript and bypass browser-side checks, so a trusted frontend delivery channel remains a separate security question. The signed P-256/AES-GCM ready proof now provides **E2EE solely for the diagnostic frame**. It does not establish an encrypted *application transport* or TaskDeck session and carries no TaskDeck credentials or data. A compromised Portal JavaScript bundle could still mislead the user even though Relay cannot forge a valid Agent signature.

