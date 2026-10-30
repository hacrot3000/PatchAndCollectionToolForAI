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

### Security audit — final TaskDeck authorization boundary (2026-10-30)

**Audited code HEAD:** [`a3531ff3a6837a74155bf52b8cca49b97c69c769`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/a3531ff3a6837a74155bf52b8cca49b97c69c769).

Scope remains the THREE stated conditions; no generic UI redesign or unrelated
app features were changed.

| Required condition | Evidence at this HEAD | Release assessment |
| --- | --- | --- |
| PHP fully compromised AFTER initial trusted pairing cannot authorize TaskDeck | Stolen Portal signer, forged `admin`, positive live grant and even a known local password must not skip owner-approved client Ed25519 possession, immutable host User.ID and local RBAC. Added mandatory `clientProofVerified`/host key identity precondition INSIDE `authorizeRemoteAuthenticatedSession`, not just in `authorizeRemoteTransportAction`. Negative test calls lower-level helper directly and demands session teardown. | PASS for current staged/in-process and login-only test routes, NOT verified end-to-end for a production application command transport |
| TaskDeck data crossing PHP remains secret | Current TaskDeck login payload is AES-256-GCM E2EE with signed/pinned Agent ECDH transcript. PHP is the Portal control plane; actual E2EE frames presently traverse independent Go Relay, not PHP. Password and diagnostic ciphertext have verified wire-only handling. | PASS for implemented login-only frames. Future commands, files, terminal output not yet transported, therefore NOT TESTED. PHP Portal credentials, metadata and traffic timing are not E2EE and cannot be claimed hidden from compromised PHP |
| Cannot bypass TaskDeck login and run commands | Per-action TaskDeck local permission gate now also checks approved-key proof at FINAL authority boundary. A local principal/key/password revocation shuts down the entire E2EE session, not merely the current action. No production generic command runner has been mounted. | PASS for bounded internal session.ping mock and selected real-HTTP denial tests; NO evidence of fully authenticated remote Terminal/Files/Git/Patch operations over a real WSS session |

**Code changes:** [`ffbac25d`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/ffbac25d80e9c4e34e6b51298ec36bc383aa5e78)
(final local authorization key proof), [`fb9e7cc`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/fb9e7cc58ef8b4f1d14c84b0e7c781da7e33660d)
(closes E2EE after failed local identity/key recheck),
[`a3531ff`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/a3531ff3a6837a74155bf52b8cca49b97c69c769)
(test fixture compile fix). Prior `ffbac25d` workflow failed due to one
incorrect test-fixture variable, not due to an assertion bypass. Fixed and
verified at exact code SHA above.

**CI on exact source SHA:**
- [Go Agent / Relay #41032333032](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41032333032): **COMPLETED / SUCCESS**, including the new direct low-level authorization attack regression, the existing PHP-compromise red-team and cross-platform builds.
- [PHP Portal / offline E2EE #41032332923](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41032332923): **COMPLETED / SUCCESS**.

**RELEASE DECISION: REMOTE CONTROL NO-GO.** Three full production
conditions CANNOT all be marked PASS while application command transport
is unmounted. No browser→Relay→Agent→TaskDeck command authorization,
decryption and output encryption live path exists; there is nothing safe
and functional to release as remote Terminal/File/Git/Patch. Do not
enable `remote_control_available`, merge into main, tag release or
deploy Remote Control on the basis of existing login-only and internal
mock PASS. Next step MUST implement the real E2EE application session
via production WSS (and PHP opaque forwarding only if the architecture
actually sends application frames through PHP), then use a controlled
post-pair PHP+Relay compromise simulation to test each permitted command
and file/output frame under incorrect password, unknown/revoked client
private key, replay, altered ciphertext, stale grant and lost network.
After security CI on EXACT release SHA, verify on real Linux/macOS and
public Relay/PHP hosting before a live enablement.
