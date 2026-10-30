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

## Three-condition release gate — Agent-side real TLS/WSS app mailbox (2026-10-30)

**Production source under test:** [`1c342061460eeb27dc147256af6d2b6e9684e2bb`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/1c342061460eeb27dc147256af6d2b6e9684e2bb).

**Changes:** [`ad785811`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/ad785811182d67bac26418739d1f8daee3fb60b5) connects the existing opaque `Registry.AppPing` mailbox to the *real Agent-side* `/v1/device` WSS handler. Strict JSON allowlists add only `app_ready`, `app_ping_ack`, `app_ping_close` and the server's outgoing `app_ping` ciphertext envelope; PHP/Relay cannot supply arbitrary command, HTTP path, TaskDeck principal, cookie or plaintext. `Registry.ActivateAppPing` validates the authentic Agent's Ed25519 signature, actual registered connection nonce and last login probe; one pending action, deadline and quota are retained. [`1c342061`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/1c342061460eeb27dc147256af6d2b6e9684e2bb) tests actual TLS/WSS Agent registration and app ciphertext delivery and shows a hacked PHP with its own signing seed cannot forge the separate Agent-ready signature. The test **injects a staged last-login marker internally**, so it is NOT proof of real TaskDeck password login over app WSS.

Three release questions:
1. **Hacked PHP access to TaskDeck:** independently authenticated Agent private key is required for application mailbox readiness. PHP's signer cannot create it. **Partial PASS** (network Agent-side, not yet full app login/session).
2. **Confidentiality:** real Agent↔Relay WSS carries only bounded opaque base64 E2EE-shaped frames; the Relay neither decrypts nor embeds TaskDeck plaintext. Existing Go/WebCrypto tests prove AES-GCM of login. **Partial PASS** (new app WSS test uses an opaque ciphertext-shaped fixture; browser↔Agent real application frame/end-to-end data still unimplemented). Portal account and routing metadata remain visible to hacked PHP.
3. **TaskDeck login bypass:** the per-action TaskDeck local verifier and host-only client-key checks stay mandatory; the new Relay route does not expose a TaskDeck command dispatcher. **Pending full real-session end-to-end test** — the in-memory test's login slot is not a production login.

**Deliberately NOT exposed:** The Browser side has NO public application endpoint or `session.ping` producer for this flow, Agent does not send `app_ready` from a real `StartEncryptedRemoteSession` yet, no response is decrypted by any browser application route and no Terminal/Files/Git/Patch command is enabled. Keep `remote_control_available=false`; do not release as remote control, change flags, merge to main or claim three full PASS results based solely on these tests.

**Same-source CI:** [Go Agent/Relay #41033901092](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41033901092) = **PENDING**; [PHP Portal/Node #41033901025](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41033901025) = **SUCCESS**. This is the audit state at documentation commit time; update separately if pending.

**Next mandatory implementation:** build an opt-in trusted-client application-login WSS path on the *existing pinned Agent E2EE channel*, keep the non-serializable TaskDeck-authenticated principal in the host process, deliver only typed `session.ping` ciphertext to `executeRemoteE2EEPing`, recheck host-approved private-key proof, TaskDeck password/RBAC and revocation on EVERY operation, return only AEAD ciphertext. Test live Browser WSS→Relay WSS→Agent→local TaskDeck→encrypted response with a compromised PHP signer plus real password but absent client private key; add negative tests for unknown/revoked key, replay/tampered ciphertext, stale Portal roles and cancellation, then safely extend to actual feature operation(s). A working remote function must exist to warrant a LIVE release.

### Same-source CI verified for Agent-side application WSS — 2026-10-30

**Audited source:** [`1c342061460eeb27dc147256af6d2b6e9684e2bb`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/1c342061460eeb27dc147256af6d2b6e9684e2bb).
- [Go Agent/Relay #41033901092](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41033901092): **COMPLETED / SUCCESS**. Includes the newly added actual TLS/WSS encrypted application mailbox transfer and malicious PHP private-key impersonation denial, Go race/security tests and cross-build checks.
- [PHP Portal/Node #41033901025](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41033901025): **COMPLETED / SUCCESS**. No PHP application data-processing or TaskDeck authorization added.

This supersedes the PENDING CI notation in the Agent WSS section above.
This proves ONLY the secure opaque **Agent-side** transport stage; it does
not complete trusted Browser-side app login/command exchange or bind
the new application-ready message to a live TaskDeck password principal
at the host. Do not claim a fully functional or fully audited Remote
Control connection. `remote_control_available=false`; **LIVE NO-GO**.

## Narrow release security audit — three conditions only (2026-10-30)

**Source under test:** [`07112e2d5fb9e7364d1f6caa6cfabc0f95ea59d5`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/07112e2d5fb9e7364d1f6caa6cfabc0f95ea59d5).
**Branch:** `feature/taskdeck-remote-access-architecture`.
No change to `main`, no deployed live control and `remote_control_available=false`.

**1. Compromised PHP cannot access TaskDeck:** Portal admin role, forged ticket/lease, stolen Portal signer, always-positive grant, copied approved public client key and even a known TaskDeck password must not establish TaskDeck app authority without (a) independently pinned Agent Ed25519 E2EE identity, (b) local host-approved client private-key Ed25519 proof for this E2EE transcript, (c) real TaskDeck SQLite user/password/project membership and per-action RBAC. Implemented a host-process-only `EncryptedApplicationSession` callback, registered explicitly by the daemon via `TASKDECK_REMOTE_LOGIN_PROBE=1` plus `TASKDECK_REMOTE_APP_PING=1`. Its only method is encrypted `session.ping`, not a generic command callback. Added real TLS/WSS end-to-end test and attacker with stolen PHP signer/known password/copied client key negative tests.

**2. PHP/Relay cannot read TaskDeck payloads:** All application frames are base64 AES-GCM ciphertext on the real WSS relay. The Portal serves tickets/leases and live grants, NOT TaskDeck plaintext, credentials, HTTP sessions or command streams. Browser↔Agent P-256 ECDH keys are bound to the owner-pinned Agent Ed25519 signature and explicit transcript; Relay only routes bounded opaque records, has no decryption key. A compromised PHP may still read its own Portal account data and traffic metadata (public device identifiers, timing/sizes, ticket), and may disrupt or phish via newly served HTML; user must run the client retained and pinned from trusted first pairing.

**3. No bypass of TaskDeck login or commands:** Agent's successful app-login transition requires the REAL local TaskDeck callback to return a live session, verifies the exact latest successful AEAD login result `authenticated=true` (not merely a valid false boolean), then signs one-use `app_ready` for that exact Agent socket/probe. Most importantly, encrypted diagnostic login v2 and encrypted application-login v1 now have DISTINCT authenticated JSON `type` fields; the host's `OpenRemoteApplicationLogin` strictly refuses a diagnostic v2 frame. Changing only the unauthenticated outer WSS operation from `e2ee_login` to `e2ee_app_login` can NEVER upgrade a diagnostic login into application authority. The host's typed `session.ping` executor checks the in-process TaskDeck identity, revocation, current local permissions and audit; all shell/HTTP/file/PTY/Git/Patch payloads are rejected.

**Real WSS bounded scope:** `taskdeck_remote_login -test-app-ping` tests an authenticated read-only AEAD ping via client→Go Relay→Agent→host TaskDeck and back. Relay `-allow-app-ping` default OFF; daemon flag `TASKDECK_REMOTE_APP_PING=1` default OFF and additionally requires local login-probe opt-in. Browser requests are limited to four sequential `session.ping` messages, timeout/replay/ciphertext length caps; no generic Remote Control routes.

**RELEASE GO/NO-GO:** The three security requirements must be demonstrated on ACTUAL intended live TaskDeck functionality, not only read-only `session.ping`. No production Terminal/Files/Editor/Git/Patch/Collection remote execution/stream has been implemented, so this work is a security gate milestone, NOT a complete Remote Control release. Real shared PHP host and external Relay deployment, signed trusted client distribution, physical client/host tests and destructive compromise/race testing are still absent. **Do not tag/merge/deploy Remote Control now.** If Go/PHP CI on exactly this source fails, fix first and repeat; even if both PASS, that is insufficient proof of a working and secure full Remote Control service.

**CI to verify on source `07112e2d5fb9e7364d1f6caa6cfabc0f95ea59d5`:** check both Go Remote Agent and PHP Portal workflow SUCCESS on the exact same code SHA. CI outcome was not yet verified when this paragraph was created.
