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
