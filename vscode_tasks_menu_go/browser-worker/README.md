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


## Automatic worker detection / Tự xác định thư mục worker

On **new TaskDeck global installations/self-updates**, the installer includes the optional worker runtime in the same release directory as the TaskDeck executable (e.g. the active TaskDeck release's `browser-worker/` folder). The setup wizard locates the actual worker folder by checking the **resolved executable path first**, then the checkout/current working directory and configured workspace. It validates the worker package identity and required runtime files. No absolute path has to be entered manually.

If the existing binary-only release predates this packaging change, or the worker files were deleted, the wizard reports **not installed** and shows a real Git checkout command instead of a fake `cd /path/to/...` placeholder. Update TaskDeck from this branch to install the worker automatically with the release, or run the checkout command manually. Node dependencies and Chromium are intentionally **not** bundled or auto-installed; install those with the wizard commands in the detected directory.

## OAuth popup remains at about:blank / Popup Google đăng nhập bị trống

Some websites implement "Login with Google" by opening a provisional `about:blank` popup and subsequently redirecting it to `https://accounts.google.com/v3/signin/identifier?... `. Snapshotting immediately after the click can capture the empty provisional popup before the redirect. More importantly, the separately sandboxed worker **intentionally blocks HTTPS document requests to public hostnames not in the explicit `TASKDECK_BROWSER_ALLOWED_HOSTS` allowlist**. A blocked provider navigation can leave the popup at `about:blank`, which the old UI misleadingly described as "No textual controls detected".

The worker now briefly waits for an initial popup URL to change away from `about:blank` (bounded timeout); the frontend polls provisional popups for a bounded period even when general auto-refresh is disabled. If a navigation is blocked, the snapshot only includes a sanitized public **hostname** such as `accounts.google.com` (never redirect query values like `state`, `code`, `login_hint`, or `code_verifier`), with a clear reason. The browser UI offers **Mở wizard để xét duyệt accounts.google.com**, opening Step 2 with the existing allowed domains retained and the new hostname suggested (never silently authorized).

To continue:
1. Verify that the provider domain is really expected for the website you opened. Use the warning's wizard link, review the domain and save it if appropriate.
2. Restart the worker so it reads the revised allowlist/token. Other OAuth subresource/redirect domains might need separate, explicit approval; do not blindly enable `*.google.com` or unrestricted outbound network access.
3. Open the original website in a fresh Semantic Browser session and click **Login with Google** again. The old popup/session will not resume after a worker restart.
4. If Google refuses sign-in despite network requests being permitted, respect Google's browser security policy; the current automated Chromium context cannot guarantee that Google accepts a controlled browser. Use an OAuth method supported by the source website, or its approved system-browser callback flow if provided. TaskDeck must not import cookies, circumvent a CAPTCHA, or forge OAuth callback states.

Regression coverage includes unit tests for provisional popup settling, sanitized blocked OAuth hostnames, bounded pending state, and an isolated Chromium popup fixture that redirects to a mocked HTTPS provider domain without exposing real account credentials.

## Reconnect missing Unix socket / Khôi phục kết nối worker

When the browser reports `dial unix .../worker.sock: connect: no such file or directory`, the saved worker path is present in configuration but **there is no reachable authenticated worker at that socket**. This often occurs after TaskDeck restarts, the worker process exits, or the per-login `XDG_RUNTIME_DIR` is cleaned. Having `worker.json` is not proof the worker process is running.

The `GET /api/semantic-browser` endpoint now reports **`configured`**, **`worker_connected`** and **`enabled`** separately. `enabled` requires a successful **token-authenticated /health request**, not merely a socket filename. Unavailable errors direct the user to the setup wizard without showing the private socket path or token.

From **Browser → Setup wizard → Step 3 → Tự động**, choose **Khởi động lại worker · Không cài lại** when worker settings exist and the socket is offline. Confirm that the worker runs in a genuinely isolated, firewalled, unprivileged environment before clicking. This is a **start-only action**: it reuses the installed Playwright/Chromium, does not invoke git/npm/downloads and does not silently change the allowlist or token. If the runtime package is not installed or incompatible with this TaskDeck revision, use normal automatic installation, which can update the private worker files without discarding `node_modules` or the Chromium browser cache.

To recover manually (the paths are examples; use the paths shown by *your* wizard):
```bash
cd "$HOME/taskdeck-browser-source/vscode_tasks_menu_go/browser-worker"
node start-configured.mjs "$HOME/.config/taskdeck-semantic-browser/worker.json"
```
The startup launcher safely re-creates a missing **private leaf socket directory** (mode 0700, owned by the current user) after logout/reboot where the parent runtime base is available; it refuses unsafe or shared directories. Another worker launch will **never delete an already-active listener's socket**. On success, the wizard must show authenticated `Worker socket reachable`.

## Chrome-compatible browser identification

The worker uses the real Playwright Chromium version to set a standard desktop **Chrome User-Agent** (for example `Chrome/156.0.0.0` for Chromium 156) instead of a `HeadlessChrome` UA. For Chromium page targets, it also attempts a matching User-Agent Client Hints override (including `Sec-CH-UA`) using CDP; if the optional CDP interface is unavailable, the context's Chrome-style User-Agent still applies.

**This is browser compatibility, not undetectable automation.** Browser fingerprinting, `navigator.webdriver`, TLS/HTTP behavior, IP reputation, and website bot challenges can still distinguish automated or headless browsers. This does not circumvent CAPTCHA or guarantee acceptance by services such as OAuth providers. The private sandbox/firewall and existing allowlist rules remain unchanged.

## Automatic installer + switch-to-manual / Cài tự động

**Browser → Installation / Setup wizard → Step 3 (Install worker)** now includes **Cài đặt tự động** and **Dừng tự động · Chuyển thủ công**. The backend executes a strictly fixed, cancellable sequence (not arbitrary browser-supplied shell):

1. Validate that the daemon runs as an unprivileged local user (not root), reject conflicting legacy environment secrets, and create a private `worker.json` with a random token and allowlist when it does not exist. This happens **before attempting to start the worker**.
2. Locate the worker automatically from the installed release, existing `~/taskdeck-browser-source`, worktree, or private cache. If missing, use a fixed, shallow Git checkout to an atomic private cache directory. Failed/cancelled checkouts are cleaned up so retry is possible.
3. Reuse a writable worker checkout and existing `node_modules/playwright`. If a system-installed release is read-only, copy only the seven vetted worker runtime files into a private writable installation directory. Install package dependencies with `npm install --ignore-scripts --no-audit --no-fund` (no sudo, no arbitrary npm lifecycle scripts).
4. Install Playwright Chromium via its **local** CLI, `node node_modules/playwright/cli.js install chromium`; already cached browser binaries are reused.
5. **Optional**, only after the user explicitly checks the isolated/firewalled trusted environment acknowledgment: start the worker and verify the token-authenticated Unix socket health endpoint. If not acknowledged, installation completes and shows the exact manual start command.

Each step publishes progress (bounded, token-redacted logs) in the wizard. A user can switch to manual at any stage; cancelling kills the active subprocess **process group**, including npm/download children. If a step fails or times out, automatic installation stops, displays the problem and provides a copyable manual continuation command. An in-flight canceled step must fully exit before automatic installation may restart. Any pre-existing TaskDeck daemon, files, running terminal or session broker are unaffected.

**Manual ENOENT fix:** If `node start-configured.mjs ~/.config/taskdeck-semantic-browser/worker.json` reports a missing file, Node/Chromium downloads are **not** the issue. Open Step 2 and save the allowlisted domains to generate `worker.json`, or click the Step 3 automatic installer, which creates this config itself. Do not hand-enter a worker token.

**Important security restriction:** Automatic install does not execute `sudo`, alter system packages, start as root or configure the required actual worker/network sandbox. Automatic Chromium launch is **opt-in only** and is not proof of network isolation. Keep it in a trusted/test environment until independent egress/firewall, private network blocking, separate service identity, process sandbox and Chromium security checks are completed. Native Windows uses TaskDeck + worker inside WSL2 and cannot auto-configure a native Windows Unix-socket worker. Job progress lives in the TaskDeck daemon (not durable across daemon restart); saved managed settings and installed packages do persist.

## Setup wizard (recommended) / Trình hướng dẫn cài đặt

Open **Browser → Installation / setup guide** (or **Browser settings → Worker setup wizard**). The wizard diagnoses the **TaskDeck server host**, not the remote machine displaying the UI.

1. **Check host:** Server OS (Linux/Fedora/Debian/macOS, container, Windows/WSL2), Node.js 20+, npm, Docker, worker folder, Playwright package and Unix socket connectivity.
2. **Configure:** An authorized administrator enters trusted public DNS hosts (for example: example.com,accounts.example.com). TaskDeck generates a fresh random 256-bit worker token and a short Unix socket path in a new private mode-0700 runtime directory (preferring XDG_RUNTIME_DIR, then TMPDIR or /tmp); it saves the socket path and token in a private, atomic, server-side settings file. This avoids the Unix sockaddr path limit when the TaskDeck config or update staging path is long. On Linux the path is ~/.config/taskdeck-semantic-browser/worker.json or the corresponding XDG_CONFIG_HOME. On macOS it is ~/Library/Application Support/taskdeck-semantic-browser/worker.json. The private directory is mode 0700 and file mode 0600. The token is NEVER shown in the wizard or installation commands.
3. **Install worker:** Run the OS-specific commands from the wizard on the worker machine. Start the worker with node start-configured.mjs followed by the quoted absolute path to the private settings file. The worker reads the same file as the Go backend, without manual socket/token environment variables.
4. **Verify:** Check worker Unix socket reachability, then Browser → Check availability and open an allowlisted HTTPS URL.

The setup endpoint **GET/POST /api/semantic-browser/setup** requires both shared-server **settings.write** and **browser.use** permissions and never returns the token. Saving settings rotates the secret; restart the worker so it reads the new token and allowed hosts. TaskDeck reads the managed settings on new browser requests without a daemon restart. Existing environment variables override the managed file; remove them and restart TaskDeck to switch to wizard-managed settings.

**Security and isolation:** The file is private to the TaskDeck service account. If the worker uses a different unprivileged UID or container, never loosen file permissions: provision a secure, read-only secret mount/mapped UID and private Unix-socket mount, or use your infrastructure secret manager. The wizard does not install packages with root rights, create systemd services, start Chromium on behalf of the user, or configure the required separate firewall, egress restrictions, seccomp, namespaces, cgroups and Chromium sandbox. The direct Node launcher is intended for controlled development until the worker runtime is independently isolated and reviewed.

**Windows:** Native Windows deployments cannot use this Unix-socket configuration directly. The wizard offers **wsl --install -d Ubuntu**; run both TaskDeck and its worker *inside the same WSL2 instance*. WSL installation may require elevated PowerShell and a reboot.

### Legacy environment configuration (advanced)

The environment-based setup below is supported for existing deployments but is no longer required by the wizard.

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

Visit TaskDeck. The header always shows **Files → Browser ▾ → Terminal**, even before worker installation. Open **Browser → Installation / setup guide…** for deployment instructions, **Check availability…** to check worker configuration and `browser.use`, and **Browser settings…** to set a workspace/user-specific homepage and active-page auto-refresh. When the worker is configured and the account is permitted, choose **Browser → Open browser…**, enter an allowed HTTPS URL and press **Go**. When unavailable, Open browser displays the setup guide instead of silently doing nothing.

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

