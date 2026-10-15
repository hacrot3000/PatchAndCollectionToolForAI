# TaskDeck Semantic Browser (experimental, opt-in)

**Branch:** `feature/semantic-browser-tab` (based on `main`). Do not merge into `main` until runtime/security acceptance. Not implemented in Restricted Developer mode.

## Thiết kế / Architecture

- TaskDeck Go backend exposes `GET/POST/DELETE /api/semantic-browser`, with permission `browser.use` in shared-server deployments. Only admin has this new permission by default; it must be explicitly granted to other roles.
- Browser Worker is a separate Node.js/Playwright/Chromium service reached **only through an authenticated Unix-domain socket**. Browser contexts are isolated per session; Go binds opaque session IDs to the authenticated project/user.
- Site JavaScript, cookies, HTTP requests and websockets run inside Chromium. The TaskDeck frontend **never executes website JavaScript or inserts website HTML**; it builds native controls from bounded JSON snapshots.
- Supports text, links, buttons, native/custom inputs, **contenteditable / role=textbox and CSS DIV editors**, atomic Login/Submit/Enter, JavaScript form validation, ARIA checkboxes, iframe forms, navigation and SPA updates.
- **Popup windows:** a window switcher displays Main/Popup sessions, allows switching and explicit closing, and returns automatically to the opener after OAuth/SSO completes with native browser redirects, cookies, or `window.opener.postMessage()`.
- **Website dialogs:** `alert`, `confirm`, `prompt` and `beforeunload` are rendered as controlled user dialogs; Playwright input actions yield to the UI instead of deadlocking on modal browser prompts. Unanswered dialogs are dismissed after 60 seconds.
- **HTTP Basic (401):** shows username/password controls only after Chromium actually observes a matching HTTPS Basic authentication challenge. Credentials are confined to the challenged origin, never persisted or echoed to snapshots.
- Popup/OAuth provider, CDN and callback hosts must be in the explicit browser-worker allowlist.
- Browser sessions are in memory (worker idle expiry: 15 min). UI reload can reattach within the same Go daemon/session; **a Go restart currently invalidates browser ownership mappings**.

## Local development (trusted, disposable websites only)

Requirements: Node.js 20+, Playwright browser dependencies, TaskDeck Go server, access to a secure private Unix socket path.

```sh
cd vscode_tasks_menu_go/browser-worker
npm install
npx playwright install chromium
node --test semantic_extract.test.mjs
```

Run the worker in an **isolated, non-root OS/container account**, with an egress policy blocking private/internal addresses at the network namespace/firewall layer, no project mounts and no credentials:

```sh
export TASKDECK_BROWSER_WORKER_SOCKET=/absolute/private/path/browser.sock
export TASKDECK_BROWSER_WORKER_TOKEN='<strong random secret at least 24 characters>'
export TASKDECK_BROWSER_ALLOWED_HOSTS='example.com,*.example.com,accounts.google.com'
npm start
```

Browsing defaults to **HTTPS only**. For an isolated development-only HTTP fixture you may set `TASKDECK_BROWSER_ALLOW_HTTP=1` in the worker; do not enable it for credential-bearing production sessions. Browsing follows only the configured host allowlist (including OAuth domains, CDN and API hosts that a given login page needs).

Start TaskDeck with the **same** `TASKDECK_BROWSER_WORKER_SOCKET` and `TASKDECK_BROWSER_WORKER_TOKEN` in its environment. The socket directory must be private; the TaskDeck process must have permission to connect to the socket. The worker rejects unlisted destinations, local/private IP addresses and disallowed WebSocket targets.

Visit TaskDeck. When worker configuration is present and `browser.use` is granted, the header contains a **Browser** button; open the tab, enter an HTTPS URL and choose **Go**.

To try the optional container image (no Playwright in the slim default TaskDeck image):

```sh
docker build -t taskdeck-semantic-browser vscode_tasks_menu_go/browser-worker
```

This image is **not** a turn-key production security boundary. Use a hardened network-isolated runtime, a restricted `seccomp` profile compatible with Chromium sandbox, cgroups and a deliberately provisioned private socket mount. The published Microsoft Playwright image is intended primarily for development/testing; it is not recommended for untrusted-site browsing without additional controls.

## Giới hạn chưa triển khai / Not yet supported

- Worker lifecycle supervised across daemon restarts; durable session cookie/profile persistence.
- Secure upload/download bridge and file pickers are not yet supported; downloads are canceled. The new popup lifecycle and JavaScript dialog implementation have unit tests, but the full TaskDeck Go→worker→UI flow still requires end-to-end acceptance on a deployed worker.
- **Authentication provider limitations:** standard cookie sessions, email/SMS OTP, first-party login forms, redirects, popup OAuth and HTTP Basic are supported in code. A generic controlled Chromium browser **cannot guarantee Google/Microsoft/FedCM authorization or hardware passkeys**; provider policies and browser-managed UX may require an approved system browser and a website-supported callback. Do not copy cookies, tokens, or try to bypass a provider block.
- CAPTCHA/passkeys, video, canvas/WebGL, complex drag-and-drop and visual layout fidelity. Iframe semantic controls are supported; complex graphical widgets still need a screenshot/native-browser fallback.
- Streaming incremental DOM patches (current implementation polls snapshots every 4 seconds when the tab is visible).
- Network-level egress firewall, DNS pinning after resolution, seccomp enforcement and full penetration testing. **Application-level allowlist is not sufficient for SSRF security by itself.**
- Automated end-to-end launch/Go build, full worker tests and real Firefox acceptance have **not** been verified in this session. Do not treat this branch as production-ready.

## Tests

```sh
cd vscode_tasks_menu_go
go test ./internal/server ./internal/identity
cd browser-worker
npm install
npx playwright install chromium
npm test
```

The suite includes `universal_input.test.mjs`, `popup_runtime.test.mjs`, `popup_flow.test.mjs`, and `auth_runtime.test.mjs`. These cover Login/Enter, simulated popup OAuth completion with `postMessage`, popup focus/close, real confirm/prompt controls, and HTTP Basic origin isolation.

Acceptance: create several Browser tabs, browse a JavaScript-heavy SPA, click native/custom `div` controls, change forms, navigate to allowed/disallowed hosts, validate user A cannot act on user B's session, reload TaskDeck, and confirm no website script runs in the frontend.

## English summary

The feature is deliberately opt-in and isolated from Restricted Developer mode. The backend proxies browser actions through an authenticated Unix socket to a separate Chromium/Playwright worker. The frontend renders bounded text and semantic controls only. The worker executes the real browser click, including custom `div` handlers and framework handlers. Public-network allowlisting and in-worker hostname checks are included, but **a network egress firewall and actual browser sandbox are mandatory before production use**. See `BROWSER_TAB_HANDOFF.md` for progress and outstanding acceptance.

### Browser popup and authentication acceptance checklist

1. Open a website with a `window.open` Login button and confirm a **Popup** window appears in the TaskDeck window strip.
2. Complete a simulated provider login; verify `window.opener.postMessage` updates the **Main** page and popup auto-closes.
3. Open another popup, switch Main↔Popup, explicitly close it; confirm session cookies/context remain isolated from other TaskDeck users.
4. Trigger `alert`, `confirm`, `prompt`; choose OK/Cancel and enter a value. The website must receive the correct JavaScript result without 7-second click timeouts.
5. Open an HTTPS site protected by HTTP Basic 401; supply credentials and verify the backend reload authenticates only the challenged origin.
6. Test first-party forms, OTP, OAuth redirects and popup callback navigation. Google/FedCM/passkey may be blocked by the identity provider and must use its approved integration path.
7. Run `npm test` with Chromium, `go test ./internal/server ./internal/identity`, and a real UI smoke on a hardened worker before production.

### Authentication and visual fallback improvements (2026-10-15)

- **Chained website dialogs**: a first `confirm` may trigger a second `prompt` before the original click resolves. The worker now displays the next dialog without blocking the request queue indefinitely; dialog continuation is bounded by the Go proxy's deadline.
- **Window-bound HTTP Basic**: a 401 challenge must originate from a live Chromium page and is assigned its window ID. Credentials cannot be submitted while a different window is active. The frontend can navigate to the challenged popup before submitting.
- **Concurrent sessions**: Go reserves each user's quota before creating a remote tab. The worker reserves global capacity and de-duplicates a concurrent Chromium process startup, preventing bursts from exceeding its configured context limit.
- **Opt-in static image preview**: click **Image** in the TaskDeck Browser toolbar to display a bounded JPEG screenshot for QR-based 2FA, CAPTCHA instructions or canvas-only login controls. It is a *static image*, not page HTML/JavaScript, and is rendered only in the requesting user's authenticated tab. The image is not saved in sessionStorage, is retained across background text refresh, and is cleared on navigation/window changes or when hidden. CAPTCHA verification still requires the real user and genuine site interaction; a static preview alone does not automate or bypass it.
- **Google OAuth/FedCM/Passkeys**: identity providers may prohibit controlled browser OAuth. The feature does **not** bypass provider protections or transfer private cookies from an external system browser. A site-owned OAuth callback/PKCE integration is required for a compliant external-browser handoff; generic third-party login cannot be guaranteed.
- Added tests for chained dialogs, cross-popup Basic auth, concurrent create quota and visual preview handling. Full `go test`, `npm test`, Docker and on-device Firefox acceptance are still required before release.

