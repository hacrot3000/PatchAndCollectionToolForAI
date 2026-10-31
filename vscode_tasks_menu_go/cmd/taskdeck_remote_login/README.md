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


## Native resumable download — staging only (31 October 2026)

The independently verified native login CLI can now assemble a binary file
from **up to 6 authenticated E2EE windows per login**:

```bash
taskdeck_remote_login -download-file=docs/notes.bin -download-to=/home/YOURUSER/Downloads/notes.bin \
  -relay=https://RELAY -portal=https://PORTAL -device=PINNED_DEVICE_ID -pin-file=/ABS/PIN
```

Each window remains 128 bytes. The source must be an ordinary visible
workspace file no larger than 2 MiB and the local target must be **absolute**,
absent, inside an owner-controlled non-group/world-writable directory without
symlink ancestors. No existing destination is overwritten. During transfer,
only a `0600` owner-only `<target>.taskdeck-part` checkpoint is written.
The checkpoint contains an immutable header binding the approved Device ID,
workspace-relative source path, whole-file SHA-256 and expected file size.
Every authenticated data window must match that version and exact offset.
The local client computes SHA-256 over the complete assembled bytes and
**atomically publishes a final 0600 file only after exact verification**.
Unverified bytes are never exposed under the final filename.

For a file larger than 768 bytes, request a NEW time-limited Portal ticket,
sign in again with the host-approved client key and TaskDeck password, and
repeat the same `-download-file` / `-download-to`. The client automatically
resumes from the local checkpoint. The Relay keeps its existing 6-operation
session quota; there is NO automatic ticket renewal, HTTP proxy, PHP access to
plaintext, generic file upload, arbitrary Terminal or unlimited transfer.
Any change in the host file SHA or device/source binding blocks continuation
and keeps the partial private for manual inspection/removal. A local output
directory under `/tmp` or another world-writable directory is refused.

This is an **experimental, deliberately low-throughput** integrity
milestone, not a practical high-volume file transfer yet. Do not enable
Remote Control or deploy it as a production feature. macOS users should
select a canonical path without symlink ancestors (for example their own
`/Users/.../Downloads`).


## Faster authenticated file download (experimental)

To opt into the separately typed `project.file.chunk` operation, add
`-download-fast` to the native, independently verified client command:

```bash
taskdeck_remote_login -download-file=docs/large.bin \
  -download-to=/home/YOURUSER/Downloads/large.bin -download-fast \
  -relay=https://RELAY -portal=https://PORTAL -device=PINNED_DEVICE_ID -pin-file=/ABS/PIN
```

Each request returns up to **4096 binary bytes** through strict AES-GCM.
The Relay remains oblivious to filenames and plaintext; the Agent separately
checks TaskDeck's local password principal, approved private key and
`FilesRead` project rights every time. The session still has only six
actions, so up to **24 KiB/session** instead of 768 bytes. For larger files,
obtain a fresh signed Portal ticket, re-enter the real TaskDeck password and
repeat exactly the same file/destination path. The existing owner-only
checkpoint pins the full-file SHA, device and path; a changed file cannot
silently resume. No existing destination is overwritten. The legacy
128-byte path remains the default without `-download-fast`.

This is NOT a general terminal, proxy, upload, remote editor, or production
file-transfer release. Keep the experimental host settings OFF until
independent Internet and macOS/Linux tests succeed.


## Scoped file-only E2EE transfer session (experimental)

With `-download-fast`, the FIRST authenticated `project.file.chunk` reply
irreversibly places the **TaskDeck-host-local** principal into file-only mode:
same visible source, same full-file SHA and size, and next sequential offset.
It cannot call `task.run`, `project.file.replace`, `git.status`, or switch
to a different file in this session. Each 4096-byte chunk still requires
TaskDeck FilesRead, account/password/key validity, fresh project RBAC,
uncached Portal grant and an auditable operation; failed checks do not return
plaintext.

The independently trusted Agent signs a file-only mode proof **only after
TaskDeck commits that reduced scope**. The opaque Relay verifies that proof
against the Agent device public key + active nonce/probe; PHP cannot forge it.
Only then is the limit raised from 6 generic actions to **64 file-only
actions** (up to 256 KiB per login), with one outstanding request, short
per-action deadlines, and the same 5-minute session lifetime. The host also
independently enforces 64, so even a modified Relay cannot expand local
authority. New signed Portal ticket + approved client key + TaskDeck password
are required for any new session. The final local file still appears ONLY
after SHA-256 verification of all received bytes. This does not implement
unbounded streaming or a remote terminal.

A malicious or stalled PHP/Relay can deny service, delay chunks, or suppress
the upgrade; this design cannot guarantee availability under compromise.
The original six-action mode remains for all sessions not locked to one
authenticated E2EE file source.


## Download a full 2 MiB file with 32 KiB blocks (experimental)

The trusted local client now supports a distinct **`-download-full`** option
(which must NOT be combined with `-download-fast`):

```bash
taskdeck_remote_login \
  -download-file=docs/example.bin \
  -download-to=/home/YOURUSER/Downloads/example.bin \
  -download-full \
  -relay=https://RELAY \
  -portal=https://PORTAL \
  -device=PINNED_DEVICE_ID \
  -pin-file=/ABS/PIN
```

This requests the strictly typed **`project.file.block`** E2EE action. Each
authenticated response contains at most **32,768 bytes** of file data and a
full-file SHA-256, with a separate strict 45 KiB authenticated envelope.
The trusted TaskDeck host pins a session to the **exact operation type**, one
visible relative path, full-file SHA, file size and the next byte offset.
It independently limits the session to **64 blocks × 32 KiB = 2 MiB**, with
TaskDeck-local FilesRead, approved client private key, password generation,
uncached Portal grant recheck and local audit **on every block**. After
the first successful block, only the original Agent can sign the one-way
Relay quota increase. The default six-action Relay quota and anti-flood
limits for other sessions remain unchanged.

The Relay and PHP carry ciphertext only. No generic RPC, TaskDeck password,
client private key, command string, PTY, upload or generic HTTP proxy is
introduced. The 128-byte window and 4 KiB chunk protocols remain separate,
with their own smaller authenticated result caps. A previous checkpoint may
be resumed with the same source and local destination if its full-file SHA
matches. A partial file stays owner-only `0600`, and the final file is
published atomically only after the entire SHA-256 matches.

A download is still bounded by one **five-minute** E2EE session and may
fail/need an independently approved new ticket because of network latency,
revocation, file modifications or host load. A hacked Relay can deny
service but cannot decrypt or authorize extra TaskDeck operations.
No live deployment has been approved; `remote_control_available=false`.


## Trusted interactive native console (experimental)

To perform several **typed** operations in a single real TaskDeck login (rather
than repeatedly fetching a Portal ticket for each command), use the native
CLI with `-interactive`, the usual `-relay`, `-portal`, `-device`, and
private `-pin-file` flags. **The compromised PHP Portal is never trusted
as a source of client code or TaskDeck credentials.** The client authenticates
the owner-pinned Agent and the host-approved Ed25519 software key BEFORE
asking for the real TaskDeck password.

Inside the console the following fixed commands are available:
- `ls [workspace-path]` — existing bounded FilesRead listing.
- `view <file>` — full-file **4 KiB** UTF-8 FilesRead preview over a separate authenticated E2EE action, no download.
- `edit <file>` — read current small file, prompt for one replacement line
  of at most 96 UTF-8 bytes, require a local `YES`, then perform an
  optimistic SHA-256 CAS with FilesWrite. Host opt-in
  `TASKDECK_REMOTE_FILE_REPLACE=1` remains required.
- `git status` and **`git diff <file>`** — independent GitStatus AND FilesRead RBAC;
  the diff is at most 4096 UTF-8 bytes, restricted to an existing visible
  regular workspace file. No arbitrary Git flags, hooks, external diff,
  textconv, shell expansion or Git write.
- `terminal task <ID>` — an existing *explicitly host-approved* TaskDeck
  task via the same TasksRun check and 7-second process deadline. Host flag
  `TASKDECK_REMOTE_TASK_RUN=1` and host approval are still required.
- `help`, `exit` — local only, no Relay application request.

All output is escaped for TTY safety; **remote control bytes cannot execute
local terminal ANSI escapes**. Exactly six application actions per login,
including both requests needed by edit; five-minute maximum lifetime, one
outstanding request, full login/key/RBAC and uncached Portal grant checks
on every operation. A denied, revoked, oversized or failed authenticated
action aborts the console rather than continuing with a poisoned channel.

This is a **bounded command console, not a PTY or an unrestricted shell**.
Interactive Terminal streaming/resize/input/control, full editor buffers,
large Git diffs and Git mutation remain distinct unimplemented features.
Do not enable Remote Control LIVE; `remote_control_available=false`.


### Native console navigation (main, 31/10/2026)

`taskdeck_remote_login -interactive` now supports client-side `pwd`, `cd`,
and relative `ls`, `view`, `edit`, and `git diff` from the selected directory.
For example, type `cd source dir`, `ls`, `view config file.txt`,
`git diff module name.go`, then `cd ..`. Spaces in remote paths are accepted
as ordinary path characters; do not add shell quoting, because this console
is **not** a shell and forwards only typed E2EE actions. `cd` without an
argument returns to the workspace root. `cd` never runs on the Agent and
does not count toward the six authorized network actions; the directory
is checked by the host when an actual typed read is requested.

Path resolution refuses absolute paths, traversal beyond the workspace
root, `..` in forwarded path components, NUL and shell execution. Invalid
paths are rejected on the native client. A TTY line is bounded *before*
copying/parsing it; oversized pasted commands, edits and confirmations
abort safely rather than allocating unlimited memory. EOF exits cleanly.
Security tests cover rooted navigation, paths with spaces, traversal,
terminal-command injection, oversized input and canceled edits.

**Unchanged constraints:** each live login permits at most six typed
operations / five minutes; console `view` is a complete regular UTF-8 text file of
at most 4 KiB, `edit` is an existing-file SHA-CAS replacement of at most
96 bytes requiring `YES`, Git diff is read-only and at most 4 KiB, and
`terminal task <ID>` runs only host-approved tasks with local TasksRun RBAC.
This change does not implement PTY, generic Git mutations, full editor or
file downloads. PHP/Relay receive no plaintext. Do **not** set
`remote_control_available=true` or publish a LIVE release without the
outstanding end-to-end deployment/security gates.


### Interactive text preview vs file download (31/10/2026)

The console's `view` now sends a separate fixed `project.file.preview` typed
E2EE action. The host opens a pinned visible regular file, refuses incomplete
or over-4096-byte text, requires the current TaskDeck FilesRead authorization
and records a local `project_file_preview` audit entry. The native client
verifies the distinct AES-GCM reply type, strict JSON shape and full-content
SHA-256, and escapes terminal control bytes before printing. Neither PHP
nor the Relay decrypts the contents.

Unlike `project.file.chunk` and `project.file.block`, this is a bounded
*text viewer*, not a transfer: it does not activate a file-only scoped
download session, does not write a destination, and allows subsequent
authorized Git/terminal/edit actions in the same six-round login. The
single-shot `-read-file` diagnostic still uses its separate 192-byte format,
and the CAS pre-read for `edit` stays at 192 bytes. No general-purpose PTY
or file downloading was added to the console; LIVE remains NO-GO.


### Current staged console: fixed Git metadata and 4KiB text editor (31/10/2026)

The latest native console adds `git log` (last 30 commits) and
`git branches`. These are separate typed E2EE operations with distinct
strict 4KiB response envelopes, pinned Agent identity, host-local `GitLog`
RBAC, uncached Portal grant, host-only audit, fixed OS-managed Git executable
and isolated environment. No Git command-line arguments, shell, hooks,
fetch/pull/push, stage, commit or HTTP proxy are accepted.

Interactive `edit <relative-file>` now uses a complete 4KiB UTF-8 preview
followed by a **separate host-opt-in** 4KiB SHA-256 compare-and-swap edit.
The native editor accepts literal lines, ending on a single `.save` (retain
trailing newline), `.save-noeol` (omit trailing newline) or `.cancel`.
For a literal directive line, prefix another period (for example
`..save`). A local `YES` is required after entering text; cancel/NO
never sends a write. The request is a strictly typed E2EE record, not file
upload, binary transfer, or shell text. Existing native `-replace-file`
and the older `project.file.replace` remain limited to 96 bytes and do
not inherit the expanded permission.

Both `TASKDECK_REMOTE_FILE_REPLACE=1` and `TASKDECK_REMOTE_TEXT_EDIT=1`
**must be explicitly set in the host TaskDeck process environment** to
allow larger text edits; either unset/0 denies them. TaskDeck requires
FilesWrite and a real approved client private key + local password, takes
its usual editor mutation lock, opens only an anchored visible regular
file, checks old SHA both before temp creation and before atomic commit,
rechecks permissions and writes a host-only audit. Up to six E2EE actions
per five-minute login; `edit` consumes two. Larger-than-4KiB edits are
refused, never truncated.

This is **staging**, not general remote control. A complete real Internet
validation, full terminal PTY, unrestricted Git mutation and production
security approval remain absent. The initial editor-write test denial was traced to a test using a
Portal viewer grant (correctly forbidden to write). Positive tests now use
an admin grant with real TaskDeck FilesWrite while a separate negative
test confirms viewer denial. The focused Git/editor CI step has passed;
whole-workflow and real Internet acceptance remain separate gates.
Do not set `remote_control_available=true` or deploy to LIVE.


### Additional native Terminal and Git interactions (31/10/2026)

The trusted `-interactive` console also supports:

- `terminal tasks`: one E2EE action that enumerates **only** runnable
  `tasks.json` process tasks whose entire definition SHA-256 is on the
  **host-only** approval list (`TASKDECK_REMOTE_APPROVED_TASKS`).
  Task IDs and human-readable labels only; no executable paths, arguments,
  task environment or secrets. TaskDeck `TasksRun` RBAC, a live approved
  client-key/password session, Portal role (operator/admin), and
  `TASKDECK_REMOTE_TASK_RUN=1` are required. A viewer cannot enumerate
  approved tasks or invoke them, and PHP alone cannot authorize enumeration.
- `terminal task <ID>`: runs the same host-approved, input-free process
  task with the existing seven-second deadline, cancellation and prelaunch
  audit. The interactive console uses a *separate* authenticated reply type
  for a **complete up-to-4096-byte terminal output**. Longer output is
  refused, not silently reduced to a tail. The separate legacy one-shot
  `task.run` record retains its original 192-byte transcript.
- `git diff --staged <relative-file>`: inspect the exact staged Git diff
  for an existing, visible, regular pinned workspace file. Requires
  `GitStatus` **and** `FilesRead` both before and after execution.
  Uses an isolated, OS-managed Git executable, fixed read-only arguments,
  no shell/external diff/textconv and a distinct authenticated <=4KiB
  envelope. Option spelling is fixed: arbitrary Git flags, `git add`,
  `git commit`, `git push`, etc. are not forwarded.

All new actions use the original per-login six-action/five-minute ceiling,
strict E2EE/Agent pin, host-local TaskDeck RBAC and identity audit. The
native console does NOT provide an arbitrary Terminal PTY, unrestricted
shell, Git stage/commit, file transfer or remote-control web UI.

The source is on `main`, with dedicated regression tests and Go CI.
Check the final same-commit CI before promotion; LIVE is still NO-GO and
`remote_control_available=false`.

 
### Complete 4KiB interactive Git status (31/10/2026)

The native `git status` command now uses the independent
`git.status_full` E2EE operation: fixed, read-only Git
`status --short --branch --untracked-files=no`, bounded complete
4096-byte response, separate AEAD result type, TaskDeck `GitStatus`
permission, local host audit and live grant reauthorization.
More-than-4096-byte output fails closed instead of truncating. Older
one-shot `git.status` still retains its 192-byte response for
compatibility; this does not grant Git stage/commit/push.
