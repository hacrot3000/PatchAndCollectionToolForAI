# TaskDeck Remote — 3 mandatory security boundaries


## Scope lock — three security invariants (2026-10-30)

**Source commit:** [`0404e79adbce59b82dcb6e12ca146a4815440351`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/0404e79adbce59b82dcb6e12ca146a4815440351).
**Security objective:** protect TaskDeck if PHP shared hosting/Portal is fully compromised AFTER the initial owner-verified pairing; do not accidentally enable remote execution during hardening.

1. **PHP control-plane compromise must not authenticate TaskDeck.** An attacker may read/write PHP, SQLite, MFA/session data, Portal ticket/lease signing seed, and falsely report admin/allowed=true. None is a TaskDeck login. The host validates Agent Ed25519 identity bound to its workspace, owner-approved client public key and proof of matching client **private** Ed25519 key on the exact E2EE transcript, the real TaskDeck-local password hash, account state, immutable user ID, project membership and per-action permission. The PHP grant is an additional deny/ceiling, not a replacement for these requirements. Compromised PHP may still disrupt availability, burn tickets, or phish if the person trusts replacement PHP-served code; preventing the latter requires the offline HTML file verified at first pairing and no automatic code refresh from the compromised PHP origin.

2. **PHP/Relay never receive plaintext TaskDeck application data.** Browser offline client and authenticated host Agent derive ephemeral ECDH P-256/AES-256-GCM keys with signed transcript, directional sequence counters and replay-fail-close. WSS transports a **base64 representation of ciphertext**. The Go Relay currently handles only signed presence, one-shot client-key check, diagnostic echo and encrypted login boolean. The PHP Portal does not currently carry TaskDeck terminal/file/command bytes at all; it is NOT the WebSocket data relay. A compromised Portal/Relay CAN see Portal account data, Device ID, key fingerprint/public keys, traffic timing, sizes, routing IP, nonce/ticket, and can drop/delay/corrupt traffic. These are NOT encrypted by the application envelope and therefore cannot be promised secret. Future file/terminal/output streams may only be mounted as typed, AEAD-encrypted frames AFTER TaskDeck authorization. The CLI/Firefox password prompt must remain in a locally retained trusted client, NEVER in PHP-served post-pair JS.

3. **No remote login bypass / command execution.** Signed Portal admin claims, a successful WSS handshake, Agent proof, key preflight, or a password-only result never creates a TaskDeck principal or remote command. An application action must use the same in-process local session created by approved client key + password + local project RBAC and still live E2EE channel; authorization must be re-evaluated for each operation, with revocation, timeout, audit and bounded resource controls. At this checkpoint the real Internet application dispatcher and Terminal/Files/Git/Patch/Collection routes are UNMOUNTED and `remote_control_available=false`. **No remote command can be claimed operational/securely authorized end-to-end until such a route is created and tested.**

### This audit's concrete additions

- `internal/remoterelay/signed_claims.go` + `tickets.go` + `lease.go`: parse signed ticket/lease JSON as exact-key, exact-type, bounded, one-meaning objects. Invalid/duplicate/escaped-alias/case-aliased/extra claims are refused even when an attacker has the Portal signing seed. A refused token never consumes a JTI replay record.
- `tickets_test.go`, `lease_test.go`: negative forged-signer cases and positive canonical PHP-format claims.
- `remote_portal/tests/offline_e2ee_crypto.test.cjs`: captures actual encrypted login WSS wrapper from offline Firefox crypto, confirms no plaintext TaskDeck password/username is included and that the sequence-numbered ciphertext opens only at the authorized Agent.

CI checks for EXACT code source `0404e79adbce59b82dcb6e12ca146a4815440351`:
- [Go Agent + Relay #41030706501](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41030706501) — **PENDING (not yet a PASS)**.
- [PHP Portal + offline WebCrypto #41030706506](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41030706506) — **SUCCESS**.
These tests validate covered invariants only. If any is pending, do not mark full validation PASS.

**Release decision: NO-GO for remote control.** The three invariants are conditions that every future real application frame/operation must satisfy, not a declaration that the presently unmounted dispatcher has passed. Finish an authenticated remote `session.ping` across actual WSS → host-local TaskDeck permission gate, then negative tests for wrong password, unapproved/forged key, stale grants, replay, PHP authority spoof, local revoke, lost channel, ciphertext corruption; only then consider typed read/write operations. Maintain hardware Firefox `file://`, NAT/CGNAT, shared-host tests, authentic binary/HTML provenance and independent pentest as release gates.

### Confirmed same-source CI result — 2026-10-30

Source SHA [`0404e79adbce59b82dcb6e12ca146a4815440351`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/0404e79adbce59b82dcb6e12ca146a4815440351):
- [Go Agent + Relay #41030706501](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41030706501) — **COMPLETED / SUCCESS**. Includes Agent/Relay cryptographic verification, signed-grant ambiguity regression, host-local login/security integration and cross-platform compile gates.
- [PHP Portal + offline WebCrypto #41030706506](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41030706506) — **COMPLETED / SUCCESS**. Includes actual offline-client captured-wire assertions for encrypted TaskDeck login, plus PHP Portal regressions.

This supersedes the PENDING result captured earlier in the same-source handoff.
Only the **implemented login-only and diagnostic boundaries** are verified:
a valid Portal admin ticket cannot itself authorize TaskDeck, the existing
login request is opaque AES-GCM ciphertext to PHP/Relay, and the current
remote application command dispatcher remains **UNMOUNTED**. Do not infer
that future Terminal/Files/Git/Patch transfer or browser-level phishing
resistance is tested or released. `remote_control_available=false`,
Remote Control **NO-GO**.
