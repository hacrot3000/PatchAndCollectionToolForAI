# TaskDeck trusted local E2EE login probe (Linux/macOS)

This **local CLI** is the only currently implemented, independently built client
capable of accepting a TaskDeck password in this experimental Remote design.
It is not Remote Control. It authenticates one TaskDeck password and receives
only an encrypted yes/no response; no cookies, session tokens, APIs, terminal,
files, Git or Patch execution are exposed.

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

## Experimental, explicitly host-allowlisted TaskDeck task.run

The independent native client also supports `-run-task=TASK_ID` for a
**previously configured** local task whose `.vscode/tasks.json` entry contains
exactly `"taskdeckRemoteAllowed": true` (Boolean). TaskDeck must opt in with
`TASKDECK_REMOTE_TASK_RUN=1` and `TASKDECK_REMOTE_APPROVED_TASKS=ID@SHA256[,ID@SHA256]` on the daemon **in addition to** the login/app
E2EE flags; default OFF. Obtain the full definition hash locally with
`taskdeck remote-task --workspace /absolute/project list` and review the exact
host task before copying its token to the daemon environment. The legacy
`TASKDECK_REMOTE_APPROVED_TASK_IDS` (31-bit numeric ID only) is ignored. Only `type:process` tasks in the current workspace with no
`dependsOn`, `inputs`, custom environment, secret references, shell
override, deployment approval or alternate cwd are admitted. TaskDeck checks
current user `TasksRun` permission, Portal grant and client private-key proof
on every action. The host executes its own configured task (not a command
string/args/env from the Relay) for no more than approximately 7 seconds,
stopping it on logout, denial, timeout or disconnect, returning a bounded
(192-byte) encrypted/escaped output preview. Child processes get only PATH,
HOME, LANG and TERM, not arbitrary daemon secrets. A change to the task
command or any other definition field invalidates the host's SHA-256 pin. No TaskDeck browser session,
persistent PTY, arbitrary Terminal, SSH, Patch or Git write authority is
returned. This is for operator-controlled development testing, NOT production.

## Experimental E2EE Git status and narrow file replacement — NOT live

`-git-status` calls the host's **fixed, read-only** Git status adapter,
with a 192-byte response limit and a fresh `GitStatus` permission check.
It cannot run Git commands, hooks, push/fetch or arbitrary arguments.

`-replace-file=relative/file.txt` reads a small existing file, computes its
SHA-256 locally, prompts for up to **96 UTF-8 bytes** of new text and submits
one AES-GCM optimistic replacement. TaskDeck checks the **local `FilesWrite`
permission**, approved client key, password generation, current Portal grant,
workspace mutation lock, current expected hash and audit before the pinned
atomic commit. The TaskDeck HOST additionally **must** set
`TASKDECK_REMOTE_FILE_REPLACE=1` in its own daemon environment. Otherwise
writes are always forbidden even if Portal claims admin or read-only E2EE
app-ping is enabled. This is not a collaborative editor, full file transfer,
Terminal, Git write, Patch or general Remote Control session. Keep disabled
on production systems pending release acceptance.

## Developer-only authenticated E2EE workspace file preview

`-read-file=relative/path.txt` replaces the diagnostic `session.ping` with one
read-only file preview (up to **192 UTF-8 bytes**) from the TaskDeck workspace,
subject to the host's **FilesRead permission**, locally approved client
private-key proof, current TaskDeck password/account/project membership and
uncached live grant checks. It uses the same authenticated E2EE session,
bounded Relay mailbox and native client. File bytes never enter PHP or the
Go Relay as plaintext. Console output is escaped to prevent terminal control
sequences. Readable text only; no directory traversal, symbolic-link escape,
local HTTP proxy, arbitrary commands, file writes or remote terminal.
Both the Agent's `TASKDECK_REMOTE_LOGIN_PROBE=1 TASKDECK_REMOTE_APP_PING=1`
and Relay's `-allow-app-ping` must be deliberately enabled. This option is
an **experimental narrow feature**, not a production Remote Control launch.

## Latest developer-only E2EE session.ping (not Remote Control)

The code now also provides `-test-app-ping` on this independently installed CLI. It requires the Go Relay's explicit `-allow-app-ping` and the TaskDeck daemon's `TASKDECK_REMOTE_LOGIN_PROBE=1 TASKDECK_REMOTE_APP_PING=1` **at daemon startup**, plus a host-approved client Ed25519 key, the real TaskDeck password and local RBAC. `session.ping` is an AEAD-encrypted, read-only diagnostic **not a remote shell, file browser, HTTP proxy or TaskDeck command capability**. Without the flag the CLI remains login-only. Setup and production safety warnings: [Remote installation guide](../../../docs/TASKDECK_REMOTE_INSTALLATION.md).

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


## Experimental E2EE directory listing — NOT production Remote

The verified native client can request `-list-dir=.` (primary workspace root)
or `-list-dir=docs` for one **visible** directory inside the project.
This uses the existing, pinned Agent + approved client key + password +
`FilesRead` + live Portal grant on every operation. The directory response
is authenticated AES-GCM and never decoded by PHP or the Go Relay.
Each returned line is `D<TAB>name` or `F<TAB>name` (directory/regular file);
output is escaped at the client to avoid malicious terminal escape sequences.
Names beginning with `.`, unsafe paths, symlinks and special files are never
returned. No recursive traversal, hidden directory access, file contents,
write, shell, HTTP forwarding or PTY is enabled. A listing with more than
64 filesystem entries or more than 192 bytes of returned names fails closed;
there is no pagination yet.

The same experimental Relay/Agent application-login opt-in is required as
`-read-file`. This only improves bounded inspection from a trusted client;
it does not authorize production remote access. The old `remote_control_available=false`
and mandatory live-security gates remain unchanged.


## Experimental binary `-read-window` (not large-file transfer)

`-read-window=docs/asset.bin -offset=0` asks TaskDeck for ONE window
of up to **128 raw bytes**, printed safely as **base64** plus the
complete file size and SHA-256 over the entire file. It is confined to
one visible existing workspace file, maximum total size 2 MiB.
Each request is independently reauthorized using the TaskDeck-local
password session, approved client private key and `FilesRead` role;
PHP/Relay see ciphertext only. The client must compare `file_sha256`
across offsets before combining bytes, especially across login sessions.
The existing six-round Relay cap remains, so this does NOT supply large
downloads or file streaming. Never interpret output as terminal commands.
The target must use the existing experimental Agent/Relay app action flags.
`remote_control_available=false`, no LIVE release.
