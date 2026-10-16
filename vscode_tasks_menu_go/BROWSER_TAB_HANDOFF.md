# Semantic Browser Tab — Development Handoff

**Branch:** `feature/semantic-browser-tab` from `main`
**Date:** 2026-10-15
**Status:** IMPLEMENTED AS EXPERIMENTAL / OPT-IN — NOT RELEASE-READY
**Branch restriction:** Do not merge to main without explicit approval. Do not implement in `feature/restricted-developer-security`.

## Accepted requirements

1. Run real JavaScript inside remote Chromium on TaskDeck server/worker only.
2. Send back bounded rendered text, links, buttons and form controls as JSON, never live website HTML/JavaScript in TaskDeck frontend.
3. Map UI controls to the real page and perform Playwright click/fill/select/check/navigation.
4. Detect nonsemantic custom DIV/SPAN controls, including CSS pointer hints, DOM listener events and React/Vue/Angular patterns. Do not assume every clickable element is a BUTTON/A.
5. Keep Go TaskDeck runtime slim with opt-in separately deployed Chromium worker.
6. Per-user/per-project ownership and a dedicated `browser.use` permission. Default deny on shared server except admin.
7. Preserve existing modules, leave Restricted Developer feature branch unchanged.

## Implemented and committed on this branch

- `internal/server/semantic_browser.go`: bounded JSON API, signed-by-worker-secret Unix socket transport, random 192-bit IDs, session owner checks, action/revision forwarding, timeout and response limit.
- `internal/server/server.go`: new API route and owned-session registry.
- `internal/server/shared_authorization.go`, `internal/identity/permissions.go`: mapped dedicated shared-server `browser.use` permission.
- `browser-worker/worker.mjs`: Playwright Chromium engine, JS on remote page, context-per-session, URL+DNS allowlist, HTTP and WebSocket interception, 15-minute expiry, dialogs/download/popups blocked for now.
- `browser-worker/semantic_extract.mjs`: visible DOM semantic extractor, reusable event instrumentation, custom click detection and form descriptor generation. Snapshot budget: 3,500 examined elements and 450 returned elements.
- `web/featuremods/semantic_browser.js`: secure DOM-only viewer, URL bar, tab, input/form actions, 4-second active-tab refresh and sessionStorage reattachment. All user-supplied website text inserted through `textContent`.
- `web/featuremods/next.js`, `tabcolors.js`, `workspacetabs.js`: tab import, color and workspace tab order integration.
- `browser-worker/Dockerfile`: optional worker image distinct from TaskDeck's default slim Go runtime.
- `internal/server/semantic_browser_test.go`: owner isolation, default-off, permission mapping, forged IDs and fake Unix worker interactions.
- `browser-worker/semantic_extract.test.mjs`: real Chromium tests for onclick, addEventListener, CSS cursor, React synthetic handlers, password redaction and DOM changes.
- `browser-worker/README.md`: deployment prerequisites, security warnings, testing and known limits.

## Verification to date

- PASS: JavaScript syntax parsing checks for worker, extractor and frontend source (through V8 parser).
- PASS: New branch created from `main`; commits pushed to that branch only.
- NOT VERIFIED: Go compiler, `go test`, Playwright Chromium installation/runtime, end-to-end GUI and cross-user HTTP tests. Cannot claim the feature works in production yet.
- NOT VERIFIED: Docker image build and sandbox/hardening on actual Linux or Docker Desktop.

## Remaining priority work (do in order)

1. Run `go test ./internal/server ./internal/identity` and `go build ./cmd/vscode_tasks_menu`, fix compilation/regression problems in small commits. Run `node --check worker.mjs`, `node --check semantic_extract.mjs` and `npm test` after Playwright install.
2. Execute real headless browser tests against fixture pages (React/Vue/div-onclick/delegated DOM click, input/change/submit, async SPA routes); reproduce and fix failures.
3. Test actual TaskDeck UI: header Browser button, tab activation, switching to existing DB/FTP/terminal/editor tabs, tab drag/order, reload and interrupted browser session.
4. Deploy worker with restricted unprivileged UID, Chromium sandbox/seccomp, cgroups, private socket and host/container network policy blocking internal networks independently of DNS/URL checks; verify DNS rebinding and WebSocket egress.
5. Implement user-approved upload/download bridge, interactive JavaScript dialogs, robust cookie/session persistence, and a screenshot fallback for complex nontext UI. OAuth popup adoption is implemented but requires real-site acceptance.
6. Add per-page event-driven DOM deltas / WebSocket instead of polling; add UI status indicator for truncated/unsupported controls and a log/audit event for browser actions without secrets.
7. Check shared-server role migration and dedicated permission UI. Browser must stay unavailable under Restricted Developer unless separately reviewed later.

## Safety and compatibility conditions

- **Never run the worker in the main privileged TaskDeck Go daemon process**. Browser Worker may run third-party JavaScript; text-only delivery protects frontend, not the host.
- Worker allowlist and DNS inspection are defense in depth, NOT a complete network isolation mechanism.
- The optional `mcr.microsoft.com/playwright` base image is not itself certified for untrusted-site operation; use a hardened container/network policy.
- No external web page may call arbitrary local file/SSH/terminal/Git APIs or receive raw HTML/script from the browser worker.
- Normal TaskDeck deployments remain unchanged when the two worker environment variables are unset.
- Restricted Developer branch and security controls MUST NOT be modified as part of this feature.

## Follow-up checkpoint (2026-10-15)

- Form Enter now uses a single `fill_press` action against the exact previously identified element rather than trying to infer a replacement field from matching label text.
- Go session ownership now uses timestamped records (consistent with `Server` field type), prunes records after 17 minutes, and limits active/recent tabs to four per user; worker independently caps global contexts and expires idle contexts after 15 minutes.
- Fixed wildcard host matching for one-character subdomains; URL allowlist now also guards WebSocket destinations using Playwright's websocket routing.
- Semantic extractor handles HTML submit/button input types as real actions, derives labels from associated `label` tags, recognizes delegated-action attributes, and reduces duplicate nested buttons.
- Pin Playwright runtime and sidecar image to v1.64.0 (matching published Playwright Docker documentation).
- Rechecked all three frontend/worker JavaScript files plus the Chromium test's syntax with V8 parser: PASS. **Go test/build and browser runtime tests still pending.**

## 2026-10-15 — Universal Login, Fake Textbox and Iframe Support

### Completed in small commits (only `feature/semantic-browser-tab`)

1. **Real input detection:** extended `browser-worker/semantic_extract.mjs` to classify `input`, `textarea`, `contenteditable`, `role=textbox/searchbox`, rich editor roots (CodeMirror, Monaco, Lexical, Slate), ARIA wrappers, and CSS/data-placeholder fake DIV textboxes. Semantic element IDs remain stable while the source DOM element survives. A selector candidate is only considered usable while visible.
2. **Secret handling:** treats password, OTP, verification, passcode, PIN, CVV, bearer/token-like fields as sensitive. Their values are omitted from snapshots; TaskDeck does not persist pending edits. Raw anchor `href` values are not transferred, and OAuth/code/token query parameters and fragments are redacted from displayed URLs.
3. **Atomic form interactions:** new `browser-worker/universal_input.mjs` applies pending changes sequentially before pressing Enter or clicking a Login/Submit target within one serialized Chromium action. Custom fields focus the authentic editable target; failure to prove a valid focus target rejects safely.
4. **Keyboard compatibility:** detect keydown/keyup/beforeinput listeners registered after early instrumentation, use real sequential key events on short validated native/contenteditable fields when required, otherwise Playwright fill. Avoid setting `input.value` directly in website JS.
5. **Race-free frontend:** `web/featuremods/semantic_browser.js` uses an action queue, dirty-input tracking and debounce, focus/selection restoration, IME composition protection, and a secret-aware timer. Enter and Login submit the entire dirty form, preventing lost clicks following blur. Never auto-send password/OTP because a previous username timer fires.
6. **Control variety:** support native select/checkbox/radio, ARIA checkbox/switch DIV by real click, rich textbox, fake role textbox focusing a nested or `aria-controls` linked field; no unsafe typing if the focused element cannot be tied to the selected widget.
7. **Frames/popups:** snapshots enumerate up to eight Playwright frames (including cross-origin frames) with a stable frame-object mapping per snapshot to prevent wrong-frame clicks. OAuth popup windows share the parent session's browser context and return to the opener when closed. Extra OAuth hosts must be explicitly allowed.
8. **Network policy:** HTTPS-only unless explicit development flag `TASKDECK_BROWSER_ALLOW_HTTP=1`; request batching capped consistently at 192 KiB in Go and worker; network sandbox still mandatory independently.
9. **Existing shared installations:** `PermissionUpgradeV5Keys` and a versioned `SeedSystemRoles` migration insert `browser.use` and grant it to admin only. Existing role changes and later DENY/revocation must be preserved on subsequent daemon starts.
10. **New tests:** `universal_input.test.mjs` covers sequential username/password Login and Enter, UTF-8 contenteditable, fake textbox wrappers, keyboard-only validators, ARIA checkboxes, iframe identity and credential redaction. `seed_roles_test.go` covers browser permission migration and revocation.

### Known caveats / next work

- Real Go builds and the full Playwright test suite **have not yet been executed** in this connector-only work environment. Syntax/static reviews cannot prove runtime correctness. There are no claims of full end-to-end test success.
- Dialogs (`alert/confirm/prompt`) are still auto-dismissed; advanced two-factor flows and complex visual CAPTCHAs/passkeys/WebAuthn remain limited.
- Rich editors with heavily customized selection/formatting, inaccessible custom elements and visual canvas widgets may need page-specific fallbacks. A fake DIV that forwards focus to an unrelated, unlinked input will intentionally reject typing.
- Browser popup redirect and cross-origin iframe navigation should be validated on real authenticated sites with a hardened worker. Keep multiuser auth/policy checks enabled.
- The default Go app does not start a worker automatically. Browser Tab stays opt-in and disabled unless its socket/secret and strict allowlist are configured.
- Do not merge to `main` or Restricted Developer until tested and explicitly approved by the user.

### Final static/logic checkpoint (2026-10-15)

- **PASS**: retrieved six current JavaScript files from GitHub and checked V8 parsing (worker, extractor, input adapter, tests and frontend).
- **PASS**: checked nine essential integration points: Dockerfile copies the new module, frontend import, tab restore, Go batch fields, worker snapshot-to-frame binding, v5 permission migration, shared authorization, secret detection and HTTPS deployment documentation.
- **PASS**: executed the *exact* current `universal_input.mjs` functions against a mocked Playwright locator API; observed `fill username → fill password → click Submit` in order, and stale-frame rejection with HTTP 409 semantics. This is a logic test, **not** a Chromium end-to-end test.
- **PASS** (independent API demonstration): the local Chromium/Python Playwright runtime accepts `locator.fill` on `contenteditable` text with Vietnamese Unicode. This does not establish full JavaScript worker acceptance.
- **NOT VERIFIED**: Go compile/unit tests, `npm test` with installed Node Playwright dependencies, worker Docker startup, external OAuth/iframe site tests, or host isolation/egress hardening. These remain prerequisites for release.
- Browser Worker now hides framework exception messages that might include credentials or URLs. HTTPS is required by default; `TASKDECK_BROWSER_ALLOW_HTTP=1` is explicit development opt-in. The frontend masks all sensitive controls as native password inputs (including custom multiline controls).
- The Dockerfile now copies `universal_input.mjs`; previously it would have omitted this runtime import.
- Final branch restriction remains: **never merge into `main` or alter Restricted Developer mode without explicit authorization**.

### Post-audit security and compatibility checkpoint

- Prevented late autosync from transmitting credentials: auto/checkbox/select-only requests exclude all sensitive dirty controls, even when scheduled before password entry. Explicit click/Enter transactions can include credentials.
- Sensitive multi-line custom fields are represented with native password controls on Firefox; WebKit-only masking was removed.
- Improved secret detection for `access_token`, `apiKey`, `confirmPassword`, OTP and similarly named fields; added Chromium tests verifying those values never appear in the semantic snapshot.
- Worker now returns sanitized errors instead of raw Playwright exceptions that might contain credentials or signed URLs.
- The worker's displayed URL is token/auth-code redacted, clickable links use element IDs rather than transmitting raw signed `href` attributes.
- Real keyboard event support, ARIA checkbox DIVs and scoped fake-input focus transfer have corresponding test fixtures.
- Go v5 role migration and the Go Unix worker batch/secret non-echo tests are added.
- Multiline editors support Ctrl/Cmd+Enter submit. Browser Worker Dockerfile copies the new universal input module.
- **Validation scope remains unchanged:** V8 syntax and a mocked-locator atomic-order/stale-frame logic test passed. Full Go and Chromium test suites still require execution in a proper checkout/runtime.

## 2026-10-15 — Popup Window and Authentication Expansion

**Scope:** all code committed to `feature/semantic-browser-tab` only. No change to `main` or `feature/restricted-developer-security`.

### Completed in this phase

1. `browser-worker/popup_runtime.mjs`: manages one primary page and up to five simultaneous popups per browser context, active window switching, manual popup close, native opener tracking and automatic return to the opener when popup closes. Popup remains the real Chromium page with native cookies, redirects, fetch, JavaScript and `window.opener.postMessage()`.
2. Browser Worker now exports bounded `windows[]`, `active_window_id`, `window_event` and `dialog` metadata with each semantic snapshot. Existing worker session ownership and same-user isolation remain in place.
3. `web/featuremods/semantic_browser.js`: displays Main/Popup window controls and window status, allows user switching and closing, and clears draft form values when moving to another page/window to prevent cross-popup credential submissions.
4. `alert`, `confirm`, `prompt`, `beforeunload` handled via a modal message and explicit user OK/Cancel. The blocked Playwright click returns dialog state to TaskDeck without deadlocking. `dialog_respond` bypasses the serialized click queue, handles the modal, then resumes the pending operation. User dialogs auto-dismiss after 60 seconds.
5. `browser-worker/auth_runtime.mjs`: listens for actual HTTPS document HTTP 401 Basic challenges, shows origin-bound username/password prompt; the worker calls Playwright `BrowserContext.setHTTPCredentials` for the **observed origin only**, reloads the active page and redacts credentials from all semantic results. Stale 401 challenge callbacks cannot re-open a prior challenge after Submit/Cancel.
6. Go `internal/server/semantic_browser.go` accepts validated `switch_window`, `close_window`, `dialog_respond`, `http_auth`, `http_auth_cancel` actions, keeping permission checks and owner verification. Password, prompt, window and dialog limits enforced before the private Unix worker request.
7. Browser Worker Dockerfile includes both `popup_runtime.mjs` and `auth_runtime.mjs` in the independent container.
8. New test files `popup_runtime.test.mjs`, `popup_flow.test.mjs`, `auth_runtime.test.mjs` plus Go worker-proxy tests for popup/dialog and HTTP Basic actions. Tests use mock identity provider callbacks; **no real OAuth credentials** are used.

### Executed validation (do not misrepresent)

- PASS: current popup runtime tested directly with a V8 mock page: open/switch/close/return to main and nonblocking `confirm` resolving the original click.
- PASS: six relevant frontend/backend-worker JavaScript modules passed V8 syntax parsing; new module checks will be repeated at the final checkpoint.
- PASS: **independent** installed Chromium/Python Playwright smoke with an about:blank popup: `window.open` → `window.opener.postMessage` → `window.close` → main page state updated. JavaScript `confirm` accepted and main page resumed. This independently verifies native Chromium behavior but does **not** equal a TaskDeck end-to-end test.
- PASS: HTTP Basic helper logic smoke (mock Playwright context): worker receives credentials for the observed HTTPS 401 origin and clears the challenge; no credentials returned through snapshot.
- NOT YET EXECUTED: repository-wide `go test`, `go build`, `npm test` with Node Playwright + installed Chromium, complete browser-worker startup and real TaskDeck Firefox UI acceptance. These are mandatory release gates.
- NOT VERIFIED: sign-in to real third-party Google/Microsoft accounts, complex SAML/enterprise policies, FedCM native account chooser, or hardware passkeys/WebAuthn.

### Provider security boundary

Google OAuth 2.0 policy prohibits directing Google authorization into developer-controlled embedded user agents. TaskDeck worker is a controlled Chromium agent; it must **not** spoof Chrome, extract browser cookies, steal OAuth tokens, suppress provider checks, or claim universal Google login compatibility. Google/FedCM/passkey sign-in can require an approved system browser and an application/website-supported OAuth callback/PKCE session bridge. There is **no safe generic bridge** that can adopt a user's system-browser Google login into an arbitrary third-party website's isolated Playwright session. Do not mark this as fully supported.

### Remaining priorities

1. Execute node tests with Playwright browsers and fix any real Chromium fixture regressions. Execute `go test ./internal/server ./internal/identity` and Go build, fix compile/regressions.
2. Verify popup creation, multi-popup switching, JS confirm/prompt and HTTP Basic in TaskDeck UI through the Unix socket, including delayed dialogs, canceled prompts and race conditions.
3. Harden worker container (sandbox, egress firewall, DNS rebinding mitigations, CPU/memory caps), integrate optional UI screenshot fallback for CAPTCHAs/complex visual widgets and file downloads/uploads if requested.
4. For TaskDeck-owned OAuth integrations only: implement authorized system-browser redirect URI + PKCE and first-party callback if the website/provider supplies the necessary application integration. Do not invent a generic third-party cookie transfer.
5. Keep Restricted Developer mode excluded and `main` unchanged until the user explicitly authorizes merge.

## 2026-10-15 — Continued Auth/Popup Hardening

This checkpoint follows the normal popup, OAuth-style callback and HTTP Basic implementation already recorded above. All changes are committed only to `feature/semantic-browser-tab`; neither `main` nor Restricted Developer was changed.

### Code changes

- `popup_runtime.mjs`: a confirm may synchronously open another prompt/alert during `accept()`; the UI now receives the next modal rather than deadlocking on the original Playwright click. A pending popup navigation is allowed to finish asynchronously after a bounded wait, staying under the Go proxy timeout. Dialogs raised in inactive windows activate the correct owner window.
- `auth_runtime.mjs`: HTTPS Basic 401 is attached to the exact live browser window that generated the document response. Credentials cannot be entered from another active window, even though the origin is already verified. Corresponding frontend link leads to the challenged popup. Existing auth and permission isolation still applies.
- `internal/server/semantic_browser.go`: allocate and reserve quota synchronously under the owner mutex *before* contacting Chromium. Failed creates release the reservation. Prevents concurrent requests from exceeding four sessions per user.
- `browser-worker/worker.mjs`: independently reserves the worker's global session capacity while a context is being created, and coalesces simultaneous Chromium launches into one promise.
- **QR/CAPTCHA fallback:** a user-triggered `preview` action produces one bounded JPEG screenshot (at most 400,000 bytes) without shipping source HTML or JavaScript. TaskDeck displays it in a normal image element, keeps it visible across text polling for QR scanning, and clears it when navigating/switching windows. The main Browser Tab remains semantic text/controls; this preview is opt-in.
- `README.md`: deployment notes, limitations and safe JPEG-only preview usage updated.
- Added a Go concurrency quota test, an authentication window-scoping test, and a chained confirm→prompt test. All new code uses explicit user interaction, with no cookie extraction or provider security bypass.

### Checks actually performed

- **PASS**: syntax parsed for 7 updated worker/browser/test JS files from their committed GitHub contents, then reran syntax checks after the preview addition.
- **PASS**: executed current popup runtime functions in V8 against mock pages to confirm popup close→main, chained confirm→prompt without an action deadlock, and pending-action cleanup.
- **PASS**: executed current `auth_runtime.mjs` in V8 against mocked Chromium response+window objects, verifying wrong-window credential rejection and exact challenge-origin assignment.
- **PASS**: local independent Python Playwright/Chromium smoke: `window.open` → `window.opener.postMessage` → popup close → main UI updated; JavaScript `confirm` handled; static JPEG screenshot generated. This **does not** execute TaskDeck's full worker or frontend code.
- **NOT VERIFIED**: checkout-wide `go test`, `npm test`, `go build`, Docker build, Firefox TaskDeck UI, production OAuth provider and hardened egress deployment. These remain required before a production release.
- **Provider restriction**: Google/Microsoft controlled-browser OAuth, FedCM and real hardware passkeys may be blocked by provider policies or inaccessible browser-managed UX. An app-owned, authorized system-browser/PKCE callback is required where offered; no safe universal cookie-sharing bridge exists for arbitrary third-party websites.

### Next acceptance priorities

1. With a real checkout, run `go test ./internal/server ./internal/identity`, `go build ./cmd/vscode_tasks_menu`, `cd browser-worker && npm test`, and Docker build. Record/fix exact errors.
2. Run a local or dedicated HTTPS fixture with cross-origin popup redirect, `postMessage`, multiple chained dialogs, HTTP Basic per popup, TOTP textboxes and image QR preview through **TaskDeck Go → worker socket → Firefox UI**.
3. Validate headful/headless Chromium sandbox and worker egress policy, including DNS rebinding and WebSocket destinations.
4. For first-party websites that explicitly support a system-browser OAuth callback, integrate that site's registered redirect/PKCE flow as a separate, authorized adapter. Do not attempt a generic Google cookie/token transplant.


### Additional reconciliation (same branch; 2026-10-15)

- Worker session creation now rejects a duplicate `session_id` while the first attempt is still in flight, in addition to the existing committed-session check. Global pending reservations are released on both success and failure. Go reserves the per-user quota before contacting the worker.
- A popup's closing event clears any Basic 401 prompt that belonged to it and invalidates outstanding challenge generation, preventing a stale credential request from remaining in the main tab.
- Screenshot preview response limits: JPEG capped at 400,000 bytes; while returning the preview, no more than 120 semantic controls are included, preserving the Go API's 1 MiB response limit. A later ordinary snapshot restores the full semantic control list without removing the user's chosen preview.
- The preview is optional and never stores cookies/tokens. Main website JavaScript still executes only in Chromium.
- **PASS**: parsed 10 current JS worker/frontend/test files fetched from the actual GitHub branch. Checked browser worker image references, Go `preview` action, visual fallback UI, dialog and HTTP auth actions and quota reservation markers.
- **PASS**: current JS module logic tests via V8 and independent local Chromium `setContent` smoke for popup callback, `confirm` and JPEG. A prior attempt to use a `data:` URL failed due to sandbox policy; test fixtures already use `setContent`. These checks are **not** a substitute for checkout-wide `npm test` or `go test`.
- Explicit limitation remains: a third-party site's Google/Microsoft/FedCM/passkey sign-in cannot be universally completed by a controlled Chromium worker; provider-allowed native browser flows must be integrated where the website exposes a compliant redirect/callback mechanism. No security bypass was attempted.



## 2026-10-16 — Browser Header Menu Discoverability

User reported no visible way to launch Semantic Browser while running the experimental branch. Root cause: `semantic_browser.js` inserted its own button after Terminal, but hid the button when worker environment variables were unset or `browser.use` was not granted. Without the worker there was no entry point for setup instructions.

### Implemented on `feature/semantic-browser-tab` (NOT `main`)

- `web/featuremods/menus.js`: persistent **Browser ▾** dropdown between **Files** and the existing one-click **Terminal**. Sections **BROWSE** (Open browser, Check availability) and **CONFIGURE** (Browser settings, Installation/setup guide). Uses the same menu dismissal and event routing as Files/Settings.
- `web/featuremods/semantic_browser.js`: removes old hidden Browser button. `TaskMenuSemanticBrowser.open()` waits for initialization, rechecks worker configuration and project permission, opens a Browser tab when usable, and otherwise opens a setup guide instead of failing silently. Supports an accessible modal help/status page and an HTTPS homepage plus 4-second visible-page auto-refresh preference, stored locally per workspace/user.
- Availability explicitly distinguishes socket configured vs `browser.use` access. A configured socket does not prove the Chromium worker is healthy; actual transport is verified during browser requests.
- Setup guide identifies separate Node.js/Playwright Chromium worker, secure private Unix socket, matching socket/token environment on TaskDeck and worker, HTTPS allowlist, role permission and network/sandbox requirements. No web page JS/HTML is executed within TaskDeck.
- `web/featuremods/help.js` and `browser-worker/README.md`: updated navigation and setup instructions.
- `internal/server/menus_test.go`: new regression assertions requiring menu insertion order and setup/options visibility without worker configuration.

### Validation
- JavaScript syntax parse of updated `menus.js`, `semantic_browser.js` and `help.js`: PASS.
- Static integration checks for header Files → Browser → Terminal, documented callback exports and HTTPS-only local homepage controls: PASS.
- Full Go suite, Chromium-worker Node suite, real Firefox tab and container isolation: NOT executed in connector-only environment. Re-run `go test ./...` on the selected branch and perform manual UI acceptance.
- Preserve existing browser worker feature restrictions and do not merge into `main` or Restricted Developer.


### Final access-control review

`GET /api/semantic-browser` is protected by `browser.use` in shared mode. The Browser dropdown remains visible for everyone, but `refreshAvailability()` now checks permission **before** making the protected request, and displays a distinct `configured:null` (unknown) state rather than incorrectly claiming the worker is unconfigured. Server-side authorization remains unchanged. New Go UI regression assertions cover this branch.

Final static integration sweep: JavaScript parser PASS for `menus.js`, `semantic_browser.js`, `help.js`. Browser is between Files and Terminal, with no duplicate standalone button. Full Go and Chromium tests are still outstanding.

## 2026-10-16 — OS-aware setup wizard and managed worker credentials

**User requirement:** replace the static “Semantic Browser · Setup guide” with an OS- and host-state-aware wizard, provide ready-to-copy setup commands, and allow configuring TASKDECK_BROWSER_WORKER_SOCKET/TOKEN automatically without manual environment export.

### Implemented
- `web/featuremods/semantic_browser.js`: 4-step wizard, launched from the existing Browser setup menu or Browser Settings:
  1. Detect *TaskDeck host OS* (Linux/WSL2/container/macOS/Windows), Node.js 20+, npm, Docker, optional worker package and authenticated Unix-socket readiness.
  2. Collect explicit HTTPS website hostname allowlist, securely save managed settings (admin-only) and rotate 256-bit worker token automatically. Neither token nor raw credentials are returned to the UI.
  3. Render OS/package-manager-appropriate copyable commands and separate worker-launch command; never execute package installs or privileged shell commands from the web UI.
  4. Re-probe authenticated worker /health endpoint, showing saved configuration and explicit sandbox/firewall warnings.
- `internal/server/semantic_browser_setup.go` and route `GET/POST /api/semantic-browser/setup`: shared-server permission gate requires BOTH `settings.write` AND `browser.use`, also guarded inside handler. Managed secret stored in a private, atomically replaced file under `os.UserConfigDir()/taskdeck-semantic-browser/worker.json` (0700 dir, 0600 regular file, inode-checked read). Socket defaults to the same private directory. Legacy socket/token environment configuration remains supported with precedence; POST refuses an override instead of misleadingly reporting a successful switch.
- `internal/server/semantic_browser.go`: uses a single resolved socket/token pair loaded from managed settings unless explicitly overridden by valid legacy environment settings. No daemon restart is needed on managed-file updates; worker must be restarted to read a rotated token/allowlist.
- `browser-worker/start-configured.mjs`: reads the wizard-managed private config and sets worker process environment internally before importing the Chromium worker; never prints the secret. Dockerfile copies the optional launcher.
- `browser-worker/worker.mjs`: authenticated `GET /health`; Go wizard checks both Unix socket permissions and a successful token-authenticated response, not just that a socket file exists.
- `browser-worker/README.md`: install/usage and security prerequisites, Linux/macOS/WSL2 paths, Node/Playwright, multiuser and hardened container limitations.

### Validation and security constraints
- Added `semantic_browser_setup_test.go`: managed config file permissions, token redaction, host restrictions, shared authorization, symlink rejection, legacy override and authenticated worker health.
- Added `start-configured.test.mjs` (syntax, mode-0600 guard and no secret emission). JavaScript modules parsed by V8 in this editing session. Go string literals were inspected for malformed escapes, but **native Go test/build and real Firefox / Chromium integration have NOT yet been executed**.
- Manual CI policy from this repository was respected: no GitHub runner or workflow was started by these commits. Run manually, when ready:
  ```sh
  cd vscode_tasks_menu_go
  go test ./internal/server ./internal/identity
  go build ./cmd/vscode_tasks_menu
  node --test browser-worker/start-configured.test.mjs
  node --check web/featuremods/semantic_browser.js
  node --check browser-worker/worker.mjs
  ```
- The wizard intentionally does NOT run `sudo`, npm install, Node, Docker or other host commands itself; the user executes/copies the proposed commands. No production isolation is claimed: rootless container/user separation, secure secret volume/UID mapping, egress firewall, network namespaces, seccomp/cgroups/Chromium sandbox and real OAuth/SSO acceptance remain to be verified.
- Native Windows should use WSL2 with both TaskDeck and worker sharing one Linux Unix socket. Do not claim that native Windows Go-to-Unix-socket communication is already supported.
- Branch-only work: `feature/semantic-browser-tab`; do not merge to `main` or Restricted Developer without explicit approval.

### Follow-up
1. Run manual CI and correct any Go compiler or browser test failures.
2. Test real Linux, Fedora, Ubuntu, macOS and WSL2 host diagnostics; verify launcher and actual browser navigation.
3. If a container deployment is needed, provide an explicitly hardened worker deployment recipe with private secret/socket mounts and restricted network access, not a general-purpose privileged Docker run.
4. Consider a supervised worker lifecycle after security acceptance, with dedicated UID and secure secret distribution, rather than starting untrusted Chromium from inside the TaskDeck Go daemon.

## 2026-10-16 — Self-update candidate regression correction

**Reporter evidence:** candidate `cc8a576747019dd1d3d943956fe497bc47d0b77c` failed `go test ./...` in `internal/server` for exactly these two cases, while the remaining listed Go packages passed:

1. `TestSemanticBrowserSettingsAndSetupAreAvailableWithoutWorker` still asserted that the *frontend Setup guide* contained literal `TASKDECK_BROWSER_WORKER_SOCKET` and `TASKDECK_BROWSER_ALLOWED_HOSTS` instructions. This was obsolete after the managed setup wizard replaced the manual environment-variable guide. Commit `a06f1199` updates the test to check the actual wizard contract: setup API, generate/save controls, allowed_hosts, host diagnostics and socket readiness.
2. `TestSemanticBrowserSetupGeneratesPrivateSecretWithoutEcho` attempted to `net.Listen("unix", socket)` on a pathname under Go's very long `t.TempDir()` + `XDG_CONFIG_HOME` directory. Linux Unix-domain socket pathname is limited to about 108 bytes; that triggered `bind: invalid argument`. This was a **runtime path-design issue**, not merely an invalid test. Commits `a9eb6f17` and `e21197a2` separate the settings path from the socket path, allocating an unpredictable private mode-0700 short runtime directory beneath an available short runtime/tmp base and persisting the socket location securely. The same fix protects TaskDeck self-update staging or long home/config directory deployments.

**Additional regression coverage:** commit `3a1888d6` asserts the saved socket path stays below a conservative 100-byte cap, the runtime directory stays private, and a real Unix listener can bind to it. The new long-TMPDIR test verifies fallback to a short path. The test also cleans up its temporary socket directory. Commit `021f1983` cleans up new unused runtime directories when settings persistence fails. Documentation corrected in `browser-worker/README.md`.

**Status:** fixes committed only to `feature/semantic-browser-tab`; `main` and the existing running TaskDeck remain unaffected. The actual self-update Go tests have **NOT YET BEEN RERUN** against the new head in this assistant session. Do not mark candidate as validated until rerunning local self-update. All GitHub Actions remain manual-only, and no CI workflows were dispatched automatically.

**Next:** rerun candidate self-update on the new branch head, confirm both previously failing tests and the entire `go test ./...` pass, then exercise the real setup wizard and authenticate to a real worker. If any test fails, retain the existing TaskDeck via candidate rollback and append exact failures to this handoff.

## 2026-10-16 — Auto-detect Semantic Browser worker directory

**User request:** The setup wizard should replace its fictitious `cd /path/to/PatchAndCollectionToolForAI/vscode_tasks_menu_go/browser-worker` with an automatically discovered real host path.

### Root cause and implementation
- Prior `semanticWorkerDir()` searched only CWD and adjacent source directories. TaskDeck typically starts from a project workspace, while global installs/self-updates contain a compiled binary and Patch runtime **without** the optional Node worker files. A stronger search alone would therefore still return no installed worker.
- `internal/server/semantic_browser_setup.go`: search starts at the **resolved TaskDeck executable** (handles global release symlink), then the executable's lexical path, CWD and configured workspace; visit only a bounded number of ancestors and two known source/release layouts. Verify the package name `taskdeck-semantic-browser-worker` and all required runtime files before returning a canonical path. No broad filesystem search and no invented absolute paths.
- `internal/selfupdate/selfupdate.go`: global release staging now packages the optional worker runtime into `<release>/browser-worker` next to the binary. Copy only the seven necessary runtime files; omit `node_modules`, Playwright binaries and test assets. Older branches without the worker still package successfully.
- `install.sh`: same optional worker-runtime packaging for fresh shell-based global installations, with regular-file checks, no symlink inclusion and no node_modules.
- Wizard shows the detected **actual path** and a copyable `cd '<real absolute path>'`. If the worker truly is absent (e.g. older binary-only release), show a Git checkout command for the feature branch instead of `/path/to/...`. This does **not** silently install packages or run privileged commands.
- Tests: `TestSemanticWorkerAutoDetectionHandlesGlobalReleaseAndDevCheckout`, `TestSemanticWorkerAutoDetectionRejectsIncompleteOrImpostorPackages`, `TestSemanticBrowserSetupCommandsNeverGuessWorkerPath`; selfupdate release packaging tests verify all runtime assets are copied and node_modules/symlinks excluded.

**Validation:** Files and tests committed to `feature/semantic-browser-tab`. **Go build/test and Linux installer smoke have not yet been executed in this assistant session**, and manual-only CI has NOT been dispatched. Existing running TaskDeck remains unaffected until an explicitly triggered update from the new branch revision is installed. The future release must be built from this new commit for the worker files to appear beside its binary.

**Follow-up:** Run local candidate `go test ./...` and `bash -n install.sh`, update TaskDeck, reopen Browser → Setup Wizard and verify that it detects `<release>/browser-worker`, then test `npm install`, Chromium installation and authenticated private socket availability with an isolated worker account/firewall.

## 2026-10-16 — Cancellable automatic Semantic Browser installation

User explicitly requested: (1) TaskDeck executes worker installation steps automatically, (2) user can switch to manual at any time; on error auto stops and exposes manual continuation, (3) fix missing `worker.json` error after npm/Playwright were already installed.

### Root cause reproduced from user's error output
- The user's `~/taskdeck-browser-source/vscode_tasks_menu_go/browser-worker` clone, `npm install --ignore-scripts` and `npx playwright install chromium` had succeeded.
- `start-configured.mjs /home/duongtc/.config/taskdeck-semantic-browser/worker.json` failed `ENOENT` because the managed token/socket file was never created: the old wizard let the user skip configuration and displayed an unconditional start command.
- **Fix:** `semanticSetupInstructions` never prints an executable start command until the managed settings file is valid. `start-configured.mjs` now prints clear instructions to configure the wizard rather than a Node `lstat` stack trace. Auto installer always initializes private `worker.json` **before** touching worker dependencies.

### Implemented changes
- Added `GET/POST /api/semantic-browser/setup/run` with the same strict shared-server `settings.write` + `browser.use` permissions as the managed settings API. POST accepts only enumerated `start` / `cancel` actions plus trusted domain allowlist and optional worker-launch acknowledgment; no raw user-entered commands, script or arbitrary filesystem paths are executed. Status JSON is bounded and contains no token.
- Per-daemon `semanticInstallController` tracks a single job and its `state, active, stage, message, logs, worker_dir, manual_command`. The wizard polls status, shows current step/log and an actionable manual continuation, and offers immediate **Dừng tự động · Chuyển thủ công**. Errors/timeouts stop auto and yield manual mode; cancellations stop the active child process group. A canceled job must actually exit before another install can start.
- Automatic fixed sequence: reject root/native Windows/conflicting legacy env; reuse existing valid managed file or generate mode-0600 `worker.json` with fresh random token and allowlisted hosts; locate valid worker under release/worktree/**user's existing `~/taskdeck-browser-source`** or fixed private cache; if missing use shallow Git clone staged privately then atomic rename; if release worker is read-only, stage only vetted worker runtime to writable private config directory; skip npm if Playwright package already present; otherwise `npm install --ignore-scripts --no-audit --no-fund`; install/reuse Chromium with `node node_modules/playwright/cli.js install chromium`.
- Optional worker launch is **separate opt-in** in wizard with explicit acknowledgment of an already configured isolated/firewalled trusted environment, never as root. Launch command uses `node start-configured.mjs <private settings path>` and checks token-authenticated Unix socket health. Cancel/stop kills the managed worker process group and shows the manual restart command.
- Restricted, line-bounded progress logs; worker secret redaction; helper files with Unix process-group cancellation and a native-Windows build stub. No sudo, package-manager installs, shell `-c` using user input, arbitrary commands, remote privilege escalation, or CI runner dispatch.
- New Go tests cover permission checks, reject client-supplied command fields, explicit isolation acknowledgment, private config creation before dependency installation, token redaction, cancel/manual state, active-command cancellation, duplicate job rejection, writable worker reuse/private staging and untrusted package rejection.
- New Node test checks friendly missing worker.json error without ENOENT stack. Worker README updated with automatic/manual workflow, package download reuse, sandbox restrictions and current limitations.

### Security / validation caveats
- The auto install action runs Node/npm/git **as the TaskDeck OS user only**. Native Windows directs the user to WSL2. Elevated system package installation and real network/firewall/seccomp/Chromium sandbox configuration remain manual. Explicit acknowledgment does not prove isolation; auto worker launch is intended for consciously trusted/test installations, not production security acceptance.
- State/progress are per daemon and do not survive a TaskDeck restart, but private config and downloaded dependencies persist. The existing active TaskDeck is not replaced during installation.
- **Native Go test/build, Node browser tests and end-to-end Firefox/Chromium smoke are NOT VERIFIED in this chat.** The TaskDeck global self-update candidate must run `go test ./...` before activation. Manually run `node --test browser-worker/start-configured.test.mjs` and `node --check web/featuremods/semantic_browser.js`, plus verify network isolation and real login flows.
- Prior user request honored: all GitHub Actions remain **manual-only**, no workflow auto-runs have been enabled. Everything stays on `feature/semantic-browser-tab`; do not merge `main` without explicit approval.

### Next checks
1. Execute the TaskDeck candidate update at this new branch head and resolve any exact failing Go tests before claiming production-ready.
2. On an unprivileged Linux host with existing `~/taskdeck-browser-source` + Playwright, open Setup Wizard step 3, enter the trusted host allowlist, auto install. Confirm it **creates worker.json**, detects/reuses the user's clone, skips npm, reuses Chromium cache and ends with an explicit manual start or approved auto launch.
3. Exercise cancellation during clone/npm/Chromium download, verify subprocess trees exit and manual continuation displays a copyable step. Verify only one job can be active.
4. For real shared-server operation, verify `settings.write` + `browser.use` permissions and require a separately isolated worker identity with independent egress/network restrictions.

### Follow-up stability fixes (2026-10-16)

- Process-group cancellation now applies both to installation subprocesses and to an optionally managed Chromium worker; stopping the wizard auto job no longer intentionally leaves `npm` child downloads behind.
- The install controller uses an independent `active` flag: after Stop/manual switch, a second automatic run is rejected until the old child has actually exited, avoiding concurrent writes to private config/cache. Manual continuation and a copyable restart command remain available even when stopping an already-running managed worker.
- A failed/canceled Git checkout now uses a private staged directory and atomic rename; partial checkout contents cannot block the next retry. Read-only **or active global-release** worker files are copied into a private writable worker directory before running npm, preserving release integrity.
- Installer stdout/stderr are line-bounded and the managed token is redacted from progress logs; no raw environment is returned. The UI refreshes setup status after auto-generating `worker.json` and reopens interrupted jobs in manual mode.
- Tests added for active-job restart rejection, process cancellation, private writable runtime reuse/staging, unsafe fake package rejection and helpful `worker.json` missing error text.
- **Still not verified with native Go/Node test execution** (repository CI remains manual-only). A user-triggered TaskDeck candidate update may expose compiler/test regressions; do not consider installation production-ready without passing `go test ./...`, `node --test browser-worker/start-configured.test.mjs` and actual Linux/Chromium smoke.

## 2026-10-16 — Setup wizard Step 3 split into Automatic / Manual tabs

**User feedback:** Automatic installation was below a long manual command list and console progress did not remain visible/follow new output.

### Implemented UI
- Step 3 now renders **two explicit tabs: `Tự động` (default) and `Thủ công`**. The default Automatic tab displays installation state and a dedicated log console **before** auto-install options and start/stop buttons, not after the manual command list.
- Manual commands, including the precise continuation command when the installer fails or is canceled, render **only on the Manual tab**. Selecting Manual while a job is running or a managed worker is active first sends the reviewed `/api/semantic-browser/setup/run` cancel action, stops the managed process tree, and keeps the manual instructions available. Selecting Auto again does not silently restart the installer.
- Console follows the newest output by default. Scrolling up to review old lines disables tail-follow; scrolling back to the end or pressing `Theo dõi log: Tắt` re-enables follow. The console uses a dedicated vertically scrollable log with a larger height, `role=log`, focusability, dark readable text and `overscroll-behavior:contain`.
- Polling now distinguishes **log-only** progress from job stage/state changes. Log-only updates modify the **existing console node in place** rather than repainting the wizard, preserving the operator's log position and inputs. When the server transitions from running to manual (failed/canceled), the UI automatically selects Manual; on status completion it also refreshes setup health state.
- No backend install commands or permissions were broadened. GitHub Actions remain `workflow_dispatch` only; `main` untouched.

### Regression coverage and verification
- New `test_semantic_browser_setup_wizard.cjs` DOM/VM tests cover: Auto selected by default, Manual commands hidden until selected, default follow-tail, in-place log-only polling, manual scroll position retention, follow re-enable, cancel while switching to Manual, and failed job fallback.
- Extended `internal/server/menus_test.go` to assert the split-tab and follow-tail contract.
- **Executed a live V8 runtime harness** against the latest GitHub wizard function in this assistant session: six interactive checks **PASS**, including auto tab, console scrolling, retaining console DOM across polls and cancellation API on Manual tab.
- Full `go test ./...`, Node test runner and real Firefox/browser visual smoke **not yet executed**. Keep self-update candidate validation and manual-only CI; run manually:
  ```bash
  cd vscode_tasks_menu_go
  node --test test_semantic_browser_setup_wizard.cjs
  go test ./internal/server
  go test ./...
  ```
- Key commits: `9a8dd738` (tabs + console), `138fdd61` (UI tests), `c955777a` (scroll-follow fixes), `f168292f` (Go contract tests).

### Follow-up
Verify on a real Firefox TaskDeck page that Step 3 opens Automatic by default, new progress logs remain at the bottom unless the user scrolls away, that typing trusted domains does not lose focus during log-only updates, and that switching to Manual cancels the active job without leaving a subprocess.

## 2026-10-16 — Semantic Browser offline worker socket and Chrome-style UA

**User report:** Opening a Semantic Browser session failed with `dial unix /run/user/1000/td-semantic-*/worker.sock: connect: no such file or directory`. The user also requested a normal Chrome browser User-Agent rather than a custom/headless one.

### Root cause / behavioral fix
- Browser's `GET /api/semantic-browser` previously reported `enabled:true` based only on a **configured absolute socket filename**. A valid `worker.json` does **not** mean a Node worker is running. If the worker stops, the OS cleans the `/run/user/1000` private runtime directory, or TaskDeck restarts, the UI incorrectly opens a tab that cannot create a session.
- Go now reports `configured`, `worker_connected` and `enabled` independently; `enabled` requires a successful token-authenticated `GET /health` against the real Unix socket. The client displays offline/configured distinctions and opens the setup wizard rather than opening a broken browser tab. Transport errors no longer expose the absolute private socket pathname; they instruct the user to restore the worker.
- The setup wizard now includes **Khởi động lại worker · Không cài lại**: an explicit, authorized `POST /api/semantic-browser/setup/run` with `action:"start_worker"`. It requires existing private worker.json, permissions `settings.write` and `browser.use` on shared servers, and affirmative sandbox/firewall acknowledgment. It locates a validated installed Playwright worker and launches it **without git clone, npm install, or Chromium download**. It preserves the one-job/cancel/manual-mode rules; no silent automatic worker launch or system privileges.
- `start-configured.mjs` recreates only a missing owned, mode-0700 private leaf socket directory under the existing runtime base after session cleanup/reboot; rejects unsafe/shared dirs. The worker refuses to unlink an **active** Unix socket when launched a second time.
- Existing private managed worker runtimes are now refreshed with vetted JS modules on TaskDeck upgrade while **preserving node_modules and Chromium cache**. New Node module is included in global self-update, installer, optional Docker image and runtime validator/test fixtures.

### Browser identity
- New `browser-worker/chrome_identity.mjs` derives a normal Chrome desktop User-Agent from **actual `browser.version()`**, never a fixed or arbitrary Chrome version. A Playwright Chromium version `156.0.8078.4` produces a reduced `Chrome/156.0.0.0` header (rather than `HeadlessChrome`).
- `worker.mjs` sets that Chrome-style User-Agent on browser contexts, including popups, and attempts a consistent `Network.setUserAgentOverride` via Chromium CDP for UA Client Hints. If the optional metadata override is unavailable, the basic standard Chrome User-Agent remains.
- **No guarantee of being indistinguishable from a real human Chrome:** `navigator.webdriver`, network/TLS fingerprints, automation behavior and website bot checks remain detectable. This is compatibility/identity-normalization, not CAPTCHA or anti-bot circumvention.

### Validation
- Added unit tests: authenticated liveness vs mere configured socket and no raw socket path leakage; `start_worker` explicit isolation/config guards; Chrome identity/version formatting; preservation of prior installed Playwright when adding worker runtime assets.
- The Chrome User-Agent helper was executed in V8 with a real Chromium version string, confirming the expected standard Chrome UA and metadata. Native Go tests/full build and real server/Chromium smoke **have not been run** in this assistant session. Do not mark the new candidate production-ready until self-update runs `go test ./...` and both Node/browser worker checks.
- GitHub Actions are still **manual-only**; no CI runner was dispatched. Branch `feature/semantic-browser-tab` only; `main` untouched.

### Next manual verification
```bash
cd vscode_tasks_menu_go
go test ./internal/server ./internal/selfupdate
go test ./...
node --test browser-worker/chrome_identity.test.mjs browser-worker/start-configured.test.mjs
node --test test_semantic_browser_setup_wizard.cjs
```
Then upgrade TaskDeck from this feature branch, verify the GET status changes from configured/offline to healthy after choosing Restart Worker in the wizard, verify the existing cache is reused, and open an HTTPS URL using browser automation test headers. Independently verify worker network egress/firewall and sandbox before enabling on an untrusted site.
