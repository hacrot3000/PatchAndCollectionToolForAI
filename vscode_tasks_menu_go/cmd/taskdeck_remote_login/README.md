# TaskDeck trusted local E2EE login probe (Linux/macOS)

This **local CLI** is the only currently implemented, independently built client
capable of accepting a TaskDeck password in this experimental Remote design.
It is not Remote Control. It authenticates one TaskDeck password and receives
an encrypted yes/no response by default. An independently bundled native
GUI can additionally perform fixed, bounded E2EE `session.ping` and read-only
`git.status`. No cookies, remote shell, Git mutations, file or Patch execution
are exposed.

**Independent artifact trust:** The manual GitHub Actions provenance build and
fail-closed Sigstore verifier are documented in
[TaskDeck native client provenance](../../../docs/TASKDECK_REMOTE_NATIVE_CLIENT_PROVENANCE.md).
Never download a client executable or updater from the PHP Portal. Only run a
verified client from a separately trusted GitHub source SHA, and independently
pin the target Agent Device ID before entering credentials.

## Trust separation for a fully compromised PHP Portal

Assume an attacker can replace the Portal HTML/JavaScript, issue arbitrary
Portal-signed tickets, change the SQLite grant table, and return an arbitrary
grant-check verdict. Portal is **not** a trusted source of executable code or
password input. An E2EE web page *served by PHP* would be vulnerable to
pre-encryption credential theft.

- Build this CLI **from a source revision independently verified outside the
  PHP host**. Do not execute a downloadable binary/script supplied by Portal.
- Pin the target **Device ID directly from the owner-controlled TaskDeck
  instance**, never just trust the Portal's displayed device ID. The CLI
  requires a separate **owner-only local pin file**, and rejects any Device
  ID supplied on the command line that disagrees with it.
- Pass trusted HTTPS origins explicitly; the CLI does not dynamically load
  configuration, JavaScript, a remote binary or client secrets from Portal.
- Only a **public P-256 browser key**, **public nonce** and one-use ticket
  are exchanged via Portal. Private P-256 material stays in CLI process memory.
- The TaskDeck username/password are read **only from the local Unix TTY**.
  The CLI refuses password input via argv, environment variable or redirected
  stdin; password echo is disabled while typing. Requires `stty`.
- The client authenticates Agent's locally pinned Ed25519 identity, signed
  ephemeral ECDH transcript, AES-GCM ready and final encrypted result itself.
  A forged Relay/Portal cannot replace the verified TaskDeck device identity.

## Latest developer-only E2EE session.ping (not Remote Control)

The code now also provides `-test-app-ping` on this independently installed CLI. It requires the Go Relay's explicit `-allow-app-ping` and the TaskDeck daemon's `TASKDECK_REMOTE_LOGIN_PROBE=1 TASKDECK_REMOTE_APP_PING=1` **at daemon startup**, plus a host-approved client Ed25519 key, the real TaskDeck password and local RBAC. `session.ping` is an AEAD-encrypted, read-only diagnostic **not a remote shell, file browser, HTTP proxy or TaskDeck command capability**. Without the flag the CLI remains login-only. Setup and production safety warnings: [Remote installation guide](../../../docs/TASKDECK_REMOTE_INSTALLATION.md).

## Bounded, read-only Git status through real E2EE

On the **trusted TaskDeck host** only, explicitly enable all three
startup variables in the daemon service or runtime:

```sh
TASKDECK_REMOTE_LOGIN_PROBE=1 TASKDECK_REMOTE_APP_PING=1 TASKDECK_REMOTE_GIT_STATUS=1 \
  taskdeck --workspace /ABSOLUTE/REPOSITORY_ROOT
```

The Go Relay additionally needs `-allow-app-ping`. On the independently
installed native CLIENT, use the existing `-relay`, `-portal`, `-device`
and `-pin-file` arguments with `-test-git-status`. The TaskDeck password
is entered only in the client's TTY **after** successful pinned-Agent and
client-key preflight. The result is AES-GCM encrypted until the client.

The host executes only a fixed `git status --short --branch
--untracked-files=no` against the **exact canonical TaskDeck workspace
repository root**, with no remote path or Git arguments. Every request
passes live Portal grant, TaskDeck local password/user/project authorization
and `git.status` RBAC, then appends an audit event. Output is limited to
256 bytes: very large status lists are **refused**, not truncated or
streamed. To use this operation, explicitly select a Git repository as the
TaskDeck workspace; a nested project within another repository is refused.

This is NOT the general Remote Git GUI, nor a license to execute
`git add/commit/push`, shell, PTY, file or HTTP commands. No PHP-rendered
form can unlock or execute the operation. `remote_control_available=false`
and all flags are OFF by default.

## Trusted native Remote GUI (experimental, read-only)

Use `-gui` **instead of** `-test-git-status` or `-test-app-ping` on the
independently built native Linux/macOS client. The GUI opens only AFTER the
local client has verified the pinned Agent's Ed25519 signature, an approved
client software-key signature, and the real TaskDeck password typed at the
local terminal. The ticket, device ID and login preflight remain unchanged.

Example (on trusted client, after establishing the host/pin file as below):

```sh
./taskdeck_remote_login -gui \
  -relay=https://relay.example.com \
  -portal=https://portal.example.com \
  -device=OWNER_VERIFIED_64_HEX_DEVICE_ID \
  -pin-file="$HOME/.config/taskdeck/trusted-remote/device.id"
```

The native executable starts a random-port **127.0.0.1-only HTTP listener**
after authentication and prints two items on the trusted local terminal:
the origin-only local URL and a fresh random 256-bit **activation code**.
The OS opens ONLY the origin URL without the secret in process arguments,
avoiding leakage into system process listings. Paste the activation code
into the bundled local GUI form. Do NOT copy this local code or URL into
the PHP Portal. Alternatively, enter the code only on the genuine
127.0.0.1 origin displayed by the trusted native CLI.
The code stays in the browser's memory (not cookies/storage), is
required in a strict custom header together with the exact local Origin,
and is checked on every POST. The listener rejects rebinding Hosts,
cross-site fetches, request bodies, unknown paths and concurrent operations.
The client bundles its own HTML/JavaScript/CSS (strict CSP, no PHP scripts)
and renders verified host output using text only.

The GUI first offers **Check available read-only features** (one E2EE
round). This obtains an authenticated advisory snapshot of host opt-ins
intersected with the current TaskDeck account RBAC; feature cards remain
disabled until checked and unavailable features stay disabled. The check
itself requires either local `tasks.view` or `git.status` (including
TaskDeck's restricted-developer role). Its response also states whether
`session.ping` is allowed independently, so read-only Git access is not
inadvertently blocked for a Git-only account. The scan **never grants** permission: every
later operation independently revalidates approved key, password user,
RBAC and live Portal grant, regardless of the snapshot. An older Agent
without this new fixed protocol will reject the check and close E2EE;
use matching trusted client/host builds. The GUI then offers **Verify E2EE
session**, **Read project info**, **Preview workspace tasks**, **Read Git
Status** (including a small changed-files
table), and **Disconnect**. The task preview reports the total number of
tasks plus up to **24 short menu labels**, automatically trimmed if
JSON escaping would exceed the **unchanged 512-byte AEAD plaintext limit**,
from the host's existing
`.vscode/tasks.json`; it never returns task IDs, commands, arguments,
input defaults, environment variables or secrets. This separate capability
requires host-only `TASKDECK_REMOTE_TASKS_PREVIEW=1` and live `tasks.view`
permission, is limited to a 1 MiB regular `tasks.json` inside the exact
host workspace (not a symlink), and consumes one of the 16 bounded read-only E2EE rounds.
After the first preview, the native GUI can explicitly request **Tải thêm task từ host** using the separate fixed `tasks.preview.next` AEAD operation. The **host owns** the cursor, fingerprints the original `tasks.json` and rejects a changed source between pages. Neither the browser nor PHP can choose a task ID, offset, workspace path or command. The user can also explicitly select **Tải các trang còn lại** to load successive authenticated pages **one at a time**, never concurrently. The separate Stop button stops *after the current action*, while Disconnect immediately revokes the native E2EE session even with a pending page. The bulk loader never retries a failed round, never relogs in and never requests beyond the remaining 16-round allowance. Each additional page costs another one of the **16** authenticated E2EE rounds; local search, local Vietnamese-aware A–Z/Z–A sorting and four-label UI page switches are free. The GUI now shows the authenticated task-label coverage (downloaded / total), whether the current snapshot is complete, and a clear warning when more labels exist but all 16 E2EE rounds have been used. A new login session is required after exhausting the 16 rounds. This remains a bounded read-only browser, **not** task execution or unlimited pagination. The project overview returns only the enabled TaskDeck
shared project's display name, the canonical workspace directory's
basename (never its absolute path), and the host OS. It requires the
separate host daemon opt-in `TASKDECK_REMOTE_PROJECT_INFO=1` and local
`tasks.view` RBAC, rechecked on every E2EE action. It does NOT enumerate
other projects or inspect arbitrary host paths.

It is
intentionally limited to 16 encrypted read-only host operations per authenticated
session, with a maximum lifetime of five minutes; closing the GUI or a
revoked session destroys the local channel. `git.status` requires the
host-only `TASKDECK_REMOTE_GIT_STATUS=1` option, the canonical TaskDeck
Git repository root and live `git.status` RBAC. The UI indicates that
Two explicit authenticated Git Status reads can also be compared locally
in the trusted GUI, including new status entries, changed codes, and
entries absent from the later snapshot. Only each explicit read spends an
E2EE round; inspecting differences uses zero additional Relay/host calls.
A change in the reported branch/tracking header resets the comparison.
All paths are safely rendered as text and scrubbed at disconnect, expiry
or Firefox pagehide. This is not a content diff, fresh upstream status,
or a complete untracked inventory (\`--untracked-files=no\`, 256-byte
response cap). Git Pull, Commit, Push, Terminal and Editor
are **not implemented yet**,
and provides no hidden or unsafe route for them. The PHP Portal can
still deny service; the trusted GUI does not defeat malware already
running as the same client OS user.

**Do not publish or forward this listener or remove its loopback,
Origin, capability, CSP or single-flight checks.** Keep `remote_control_available=false`
until actual functional/security release gates pass.

## Build and manual diagnostic

In the trusted, verified Git working tree:

```sh
cd vscode_tasks_menu_go
go build -o ./taskdeck_remote_login ./cmd/taskdeck_remote_login
# On the CLIENT computer, obtain the Device ID directly from LOCAL TaskDeck,
# not from the shared-host Portal's web page. Protect this independent pin.
mkdir -p "$HOME/.config/taskdeck/trusted-remote"
chmod 700 "$HOME/.config/taskdeck/trusted-remote"
printf '%s\n' 'OWNER_VERIFIED_64_CHARACTER_DEVICE_ID' > "$HOME/.config/taskdeck/trusted-remote/device.id"
chmod 600 "$HOME/.config/taskdeck/trusted-remote/device.id"
./taskdeck_remote_login \
  -relay=https://relay.example.com \
  -portal=https://portal.example.com \
  -device=OWNER_VERIFIED_64_CHARACTER_DEVICE_ID \
  -pin-file="$HOME/.config/taskdeck/trusted-remote/device.id"
```

1. Confirm the browser's Portal account has MFA, a current grant to the device
   and that the Agent/Relay experimental probe is independently enabled.
2. Enter the **TaskDeck username only** in the locally installed CLI.
   The CLI then prints a public client nonce and ephemeral public key.
   **It must NOT request your TaskDeck password yet.**
3. On Portal's authenticated device row, open **"Cấp ticket cho client TaskDeck
   cài riêng"**. Paste **only** the nonce and public key. Click the ticket
   button and copy the `ticket` string from the JSON response.
4. Paste the ticket into the CLI within the ticket's **30-second TTL**.
   The CLI first validates the independently pinned Agent Ed25519 identity,
   its signed P-256 ECDH transcript, and the authenticated AES-GCM ready
   packet. **Only after this succeeds does the CLI prompt locally for the
   TaskDeck password** (TTY echo disabled). It then sends one AES-GCM-encrypted
   password proof and receives a boolean verdict. If PHP/Relay forges the
   Agent identity or drops the connection, no password prompt occurs.
   This never opens a Remote session.

The local pin must be an absolute-path, regular 0600 file inside a real 0700
directory; symlinked or public pins fail closed. A malicious Portal cannot
silently swap its preferred Device ID while this trusted pin remains unchanged.

The HTTP Portal origin is passed for the Relay's exact `Origin` check, **not**
to download or execute Portal code. The CLI connects only to the configured
Relay via WSS. A compromised Portal can refuse service, issue useless tickets
or trick someone into trusting a malicious executable or phishing UI; the CLI
**cannot** protect people who independently give their password to malicious
Portal content.

**Current limitations:** The local Agent's experimental login callback
requires the TaskDeck operator's explicit opt-in
(`TASKDECK_REMOTE_LOGIN_PROBE=1`) and local shared-server identity/config.
It is intentionally disabled by default. The PHP Portal must have
`REMOTE_PORTAL_EXPERIMENTAL_TICKETS=1` and a valid Portal MFA session.
The native client is still a **developer prototype**, not an approved
production release: a manual GitHub build can now create attested archives,
but those attestations MUST actually be generated and verified against an
independently pinned source SHA. No deployment should describe Remote Control as
enabled or as fully secure until the independent client delivery/pinning,
per-action TaskDeck RBAC, stream cancellation and penetration-test gates pass.


## Mandatory trust rule during a PHP Portal compromise

A compromised PHP Portal can **issue signed admin tickets, change grants and
return fabricated `allowed=true` responses**, but these operations are not
TaskDeck authentication. The TaskDeck daemon independently verifies the
user's LOCAL TaskDeck password and project membership; the future action gate
requires that original password-authenticated, in-process E2EE session and a
fresh Portal check. It cannot adopt a normal TaskDeck browser cookie or a
role supplied by PHP. A changed local TaskDeck password invalidates the
staged session even if the Portal continues answering allowed=true.

Do not type TaskDeck credentials into a PHP-served page, a PHP-hosted download,
or a program whose signature/source you have not verified independently. A
fully compromised Portal can replace its own HTML/JS to phish users; no crypto
protocol can guarantee safety if the user voluntarily enters the password
into attacker-controlled code. The local CLI and independent Device ID pin are
therefore not optional components of this threat model.
