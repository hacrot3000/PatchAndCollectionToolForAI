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
5. Implement user-approved upload/download bridge, popup/tab adoption, browser prompts, robust cookie/session persistence, and a screenshot fallback for complex nontext UI.
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
