# TaskDeck trusted local E2EE login probe and GUI (Linux/macOS)

This **trusted native CLI** accepts TaskDeck credentials locally, verifies the
independently pinned Agent identity and exposes experimental, **explicitly typed**
E2EE operations only when enabled by the host. An optional local interactive
console provides workspace navigation, bounded file view/edit/replace, fixed
read-only Git operations and execution of host-approved tasks. It is **not**
a general-purpose remote shell/PTY, Git writer, file transfer UI or HTTP proxy.
`remote_control_available=false` remains unchanged; never treat this as a
production Remote Control release.

**Independent artifact trust:** The manual GitHub Actions provenance build and
fail-closed Sigstore verifier are documented in
[TaskDeck native client provenance](../../../docs/TASKDECK_REMOTE_NATIVE_CLIENT_PROVENANCE.md).
Never download a client executable or updater from the PHP Portal. Only run a
verified client from a separately trusted GitHub source SHA, and independently
pin the target Agent Device ID before entering credentials.

## Trusted standalone GUI (experimental PR; not LIVE)

The independently built native executable adds `-gui` (mutually exclusive
with `-interactive` and other actions). It still requires the owner's
0600 Agent Device ID pin, locally stored approved Ed25519 software key,
real TaskDeck password entered via native Unix TTY after authenticating
the Agent, and a fresh 30-second Portal rendezvous ticket.

Only AFTER E2EE login, the executable listens on an ephemeral
`127.0.0.1` TCP port and launches an OS browser using `open` (macOS)
or `xdg-open` (Linux). The GUI HTML, JS and CSS are embedded in the
trusted native binary, never downloaded from PHP or a CDN. A random
256-bit GUI token is passed only in the initial local browser fragment,
immediately removed from browser history by the bundled JS. Each
`POST /api/action` must have the exact local Host, Origin, JSON content
type and session-token header. Static assets use strict no-store,
no-referrer, frame protection and CSP disabling inline/remote scripts,
external connections and forms. The local HTTP server NEVER exposes an
arbitrary TaskDeck HTTP proxy, executable command, file transfer, session
cookie, OS process ID or Git argv.

**Working GUI scope:** Files directory listing/preview/line view and exact literal find-lines (read-only E2EE), Git
status/log/branches and per-file diff (read-only), Tasks host-approved
catalogue, independently reviewed RUN TASK / START TASK with a one-use
45-second native confirmation, and managed task status/output/cancel.
Task cancel requires exact `CANCEL TASK <12hex>`. The host independently
checks password-bound user, approved key, TaskDeck RBAC, Portal grant,
auditing and host-only default-OFF feature flags. Six E2EE round trips
per login and a five-minute local GUI lifetime remain; a request after
quota does not reach the host. Logout immediately removes the local GUI
principal and closes the authenticated Agent session.

**Native GUI find-lines (merged PR #58, staging; NOT LIVE):** the locally embedded
Files panel adds one dedicated `file_find_lines` action with a UTF-8 literal
of 1–96 bytes. Only the independently trusted executable handles the
request; the PHP Portal is never allowed to supply JS, a shell/regex,
password or credential-bound GUI action. The native GUI validates the
relative visible workspace path, query, exact JSON fields and the
complete authenticated E2EE host report, including matching TERM_SHA256,
FILE_SHA256, all numbered matching rows, sorted indices and counts.
It rejects truncated or forged report metadata rather than silently
displaying incomplete search results. A query consumes exactly one of
the six allowed E2EE actions; host independently enforces FilesRead
and the active password-bound principal. No file mutation, regex,
arbitrary search command, recursive crawl, upload or download is enabled.

**Not yet supported in GUI:** full editable local TaskDeck layout,
Git writes/merge/pull/push, project trees with infinite paging,
file editing/diff merge, arbitrary terminal PTY/stdin, workflow composer,
screen sharing, long-lived resume, upload/download. Neither CI nor this
GUI is a completed physical-machine or adversarial LIVE assessment.
`remote_control_available=false`; LIVE stays NO-GO.

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

## Interactive console — unique-match text replacement (development-only)

Inside the already authenticated native E2EE interactive console:

```text
view path/to/settings.txt
replace path/to/settings.txt
Chuỗi cần tìm chính xác: host=old.example
Chuỗi thay thế: host=new.example
Xác nhận ... Gõ YES: YES
git status
git diff --staged path/to/settings.txt
terminal tasks
terminal task 1
```

`replace` first retrieves the **complete** UTF-8 file preview (up to 4096
bytes), requires exactly one matching substring on a single input line,
calculates a SHA-256 commitment over the *entire* preview, shows the proposed
complete result, and requires an exact `YES` before invoking the **existing**
host-authenticated `project.file.replace_text` compare-and-swap operation.
No match, multiple matches, overlarge content, cancellation, insufficient
remaining E2EE action quota, stale SHA or an unauthorized host mutation cause
denial without this client sending a write. The host alone enforces
`TASKDECK_REMOTE_FILE_REPLACE=1`, `TASKDECK_REMOTE_TEXT_EDIT=1`,
approved client key, TaskDeck `FilesWrite` RBAC, live grant, mutation lock
and durable audit. The console cannot bypass these controls.

Remote view, diff and task output now preserve ordinary line breaks for
legibility while escaping terminal controls, carriage returns and Unicode
directional override characters. No remote payload is executed as a shell
command or treated as a terminal escape sequence. For files larger than 4 KiB,
this command refuses editing rather than reading a partial file or silently
overwriting unseen content. Each successful replacement consumes one read
and one write E2EE action of the existing six-action login quota.

## Literal file search and safely confirmed replace-all

The same independent, locally trusted E2EE console now also supports:

```text
find src/config.ini
Chuỗi cần tìm chính xác: old-value
replace-all src/config.ini
Chuỗi cần tìm chính xác: old-value
Chuỗi thay thế: new-value
Xác nhận ghi SHA-256 CAS? Gõ chính xác "YES 3": YES 3
```

`find` consumes one read action to fetch the **entire** (at most 4 KiB)
UTF-8 file, and prints matching line numbers and total literal occurrences.
No server grep, shell, regular expression, environment variable or arbitrary
command is created. Remote text is terminal-escaped before display.

`replace-all` uses the same preview and **existing host-side optimistic
SHA-256 write** as `edit` and `replace`. It counts literal matches in the
full preview, displays the entire proposed result, refuses no-match,
no-change and oversized (>4 KiB) results, and requires typing `YES N`
where N is the exact number of matches. Wrong confirmation or cancellation
does not send any write. It needs two remaining E2EE actions and never retries
a failed/non-idempotent write. Authentication, host key approval, live RBAC,
mutation lock, audit and the **two independent host file-write opt-in flags**
remain unchanged. This is **not** filesystem search, large-file editing,
a shell/PTY or remote file download.

## Numbered view and single-line editor (experimental)

The trusted **native** authenticated E2EE console supports:

```text
lines src/config.ini
edit-line src/config.ini
Số dòng cần sửa: 3
Nội dung dòng thay thế: new-value
Xác nhận ghi một dòng bằng SHA-256 CAS? Gõ YES 3: YES 3
```

`lines` reads the **complete** existing UTF-8 text file (at most 4096 bytes),
shows its one-based source line numbers, and consumes exactly one read action.
Its output escapes terminal controls, and a final newline does not produce a
phantom line.

`edit-line` also reads the full file, renders numbered lines, then accepts an
existing positive line number and one replacement **literal line**. It keeps
all other file bytes unchanged, including the original LF/CRLF line endings
and trailing newline, rejects no-op edits and edits that exceed 4096 bytes,
and requires an exact `YES N` where N is the specified line number.
A canceled edit, invalid line number, wrong confirmation or insufficient
remaining E2EE actions does not send a write. The host still enforces the
full-file SHA-256 compare-and-swap, the mutation lock, local user/client key
authorization, file-writing RBAC and live Portal grant.

No new host operation, shell/PTY, remote URL, browser credential handling,
Git write or file download is introduced by these local console features.
Remote Control remains **NO-GO** pending independent release gates.

## Insert and delete exact numbered lines (experimental)

The trusted native E2EE interactive console now also supports these literal
commands on **existing** workspace text files:

```text
lines config/app.ini
insert-line config/app.ini
Chèn trước dòng: 3
Nội dung dòng cần chèn: enabled=true
Xác nhận ... Gõ INSERT 3: INSERT 3

delete-line config/app.ini
Số dòng cần xóa: 4
Xác nhận ... Gõ DELETE 4: DELETE 4
```

Both commands first read the **complete** UTF-8 file (maximum 4096 bytes), show
all numbered lines and display the full proposed result. They mutate only the
native client's in-memory copy, then send **the existing typed**
`project.file.replace_text` E2EE action with SHA-256 of the original full
preview. TaskDeck Agent independently rechecks its local principal, host-approved
client key, FilesWrite RBAC, Portal grant, audit, current full-file hash and
mutation lock before an atomic write. Both host opt-ins
`TASKDECK_REMOTE_FILE_REPLACE=1` **and** `TASKDECK_REMOTE_TEXT_EDIT=1`
remain required and OFF by default.

Insertion happens immediately before selected one-based line number or after
the last line using N = current line count + 1. Deletion requires an existing
line. The entire rest of the file, its existing LF/CRLF line endings, and its
original final newline convention remain byte-for-byte intact. New inserted
lines use the nearest existing EOL style (LF by default in a file with none).
A new line added to an empty file is terminated with LF. Deleting the final
unterminated line also removes its preceding separator, without creating an
unintended trailing newline. No implicit file creation, over-4KiB editor,
arbitrary shell, PTY, server-side sed, Git mutation or file transfer exists.

Every command requires **two** available E2EE actions (preview + CAS write),
a unique explicit local confirmation (`INSERT N` or `DELETE N`), and rejects
invalid line numbers, traversal, spoofing characters, overlarge previews,
overlarge proposed edits and missing host authorization. A wrong confirmation
cannot send the write and a failed write is never automatically retried.
`remote_control_available=false` remains unchanged.


## Exact multiline range editor (experimental)

In the trusted **native E2EE console**, use `edit-range <file>` to replace
an inclusive one-based span of source lines in a complete **at-most-4KiB**
UTF-8 workspace file. Example:

```text
edit-range src/config.ini
Dòng bắt đầu: 2
Dòng kết thúc: 3
alpha=new
beta=new
.save-noeol
Xác nhận ... Gõ RANGE 2-3: RANGE 2-3
```

The client fetches the complete authenticated preview once and displays
numbered source lines. The entered multiline text is **literal**; `.save-noeol`
keeps its final entered line without adding an extra blank line, while
`.save` explicitly adds a blank line at the end of the replacement span
unless the selected range reaches an unterminated EOF (where it is refused
rather than silently adding a newline).
`.cancel` stops before the write. The full resulting file is shown before
the exact `RANGE A-B` confirmation. LF separators entered at the console
are mapped to the selected source line's LF/CRLF style; all bytes outside
the selected span and the final selected line's existing terminator are
preserved. An empty replacement creates one empty line; use `delete-line`
to remove a line entirely.

This is **not** a new Agent/Portal API. It reuses `project.file.preview`
and `project.file.replace_text`, requiring two of the six authenticated
application actions, a full-file SHA-256 CAS, two independent host write
opt-ins, TaskDeck FilesWrite, an approved native client key, TaskDeck
password, a live Portal grant, host audit and the editor mutation lock.
Out-of-range/identical/oversized, EOF-newline-changing or hidden-control-character inputs,
invalid paths, canceled or mismatched confirmations do **not** send a write.
No shell/PTY, file download, Git write or PHP-side decryption was added.
Keep `remote_control_available=false` and **LIVE NO-GO**.


## Move complete lines without changing file line endings (experimental)

The native E2EE console supports `move-lines <file>` to reorder one inclusive
source-line range inside **one existing** UTF-8 workspace file (up to 4096
bytes), preserving original bytes including mixed LF/CRLF.

```text
move-lines src/config.ini
Dòng đầu tiên cần chuyển: 2
Dòng cuối cùng cần chuyển: 4
Chèn trước dòng: 8
Xác nhận ... MOVE 2-4 BEFORE 8: MOVE 2-4 BEFORE 8
```

Destination numbering refers to the **original** source lines; N+1 means
append. Moves into the source range (or directly after it) are rejected as
no-ops. To avoid merging incomplete lines, every existing source line,
including the last, must end in LF/CRLF. Files with unterminated EOF are
refused. The entire outcome is previewed, then exact `MOVE A-B BEFORE N`
confirmation is required before the full-file SHA-256 CAS.

Only existing `project.file.preview` and `project.file.replace_text`
typed actions are used (two of six E2EE rounds). Existing TaskDeck password,
host-approved native client key, independently pinned Agent ID, Portal grant,
FilesWrite RBAC, dual host write opt-ins, audit and mutation lock remain
mandatory. No new PHP/Relay operation, download, arbitrary shell, PTY, Git
write or generic upload has been introduced. **LIVE NO-GO** remains.


## Two-sided Git review without Git write permissions (experimental)

The trusted native E2EE console now supports:

```text
git review src/config.go
```

This obtains the complete up-to-4096-byte *unstaged* diff from the host's
fixed `git.diff` action, and the complete up-to-4096-byte *staged* diff
from its separately authenticated `git.diff_staged` action for the same
workspace-relative visible file. Both views are labeled and printed only
after **both** independent actions succeed. Every line from the repository
is printed through the native terminal-control escape filter.

- Both actions require **GitStatus and FilesRead** from the real TaskDeck
  principal and an uncached live Portal grant for each action, plus a
  host-approved native client key, pinned Agent identity and E2EE.
- The command consumes **two of six actions** in the 5-minute session;
  it refuses to start with only one remaining. Failures/revocation stop
  without printing an incomplete review or retrying a poisoned session.
- It does **not** execute Git commands received from the user. The host
  constructs fixed arguments with a literal path and disables external diff,
  textconv, pager, shell and unsafe environment. `git review` itself
  introduces no new host opcode or PHP/Relay endpoint.
- No Git index/worktree mutation, stage/commit/push, PTY or file downloading
  is enabled. Keep remote control **NO-GO** for LIVE.


## Git HEAD versus working-tree diff (read-only)

Use `git diff --head <file>` in the trusted native E2EE console.
This returns a **single complete** diff between the fixed `HEAD` revision
and the current visible, pinned workspace file, including both staged and
unstaged changes. `HEAD` is fixed by the host implementation and can
**never be supplied or replaced by PHP, Relay or a user-provided revision**.

The command consumes one of six E2EE actions, returns at most 4096 UTF-8
bytes or refuses the output without truncation, and is distinct from
`git diff <file>` (unstaged), `git diff --staged <file>` (staged), and
`git review <file>` (two separately authenticated reads). It requires
the TaskDeck host's GitStatus **and** FilesRead permissions, live Portal
grant, host-approved client key, locally verified TaskDeck password,
pinned Agent and host audit. The host runs fixed safe Git arguments with
`--no-ext-diff`, `--no-textconv`, `--no-color` and `HEAD -- <path>`;
there is no generic command or HTTP/PTY mount.

The command refuses missing HEAD, unsafe/symlinked file, oversized output
and revoked/invalid sessions. It does **not** stage, unstage, commit,
upload/download, change index, write files or enable Remote Control LIVE.


## Experimental SHA-pinned Git stage (disabled by default)

The native E2EE console supports `git stage <file>` for **one existing**
workspace-relative UTF-8 file of up to 4096 bytes. It first reads the
complete trusted preview, displays it with terminal escape filtering and
requires the **exact** confirmation `STAGE <workspace-relative-file>`.
It then submits a distinct typed `git.stage` E2EE action with the
full-file expected SHA-256 (never the file bytes). This consumes two of
the six E2EE actions in the five-minute authenticated session.

**Both host environment switches must be explicitly enabled locally**
(`TASKDECK_REMOTE_GIT_WRITE=1` and `TASKDECK_REMOTE_GIT_STAGE=1`);
they are OFF by default. The host verifies GitWrite, FilesRead and
FilesWrite permissions against the actual TaskDeck principal and live
Portal grant, requires the native host-approved Ed25519 client key,
pinned Agent identity, and writes a durable TaskDeck audit **before**
any Git object/index mutation. The file SHA is checked against a pinned,
non-symlink source immediately before staging. The host serializes the
index update with revocation and uses the same shared file mutation lease
as the local editor when shared mode is active.

**Never use `git add` for this command:** it can execute arbitrary
repository-defined clean filters. Instead, the host writes the exact
authenticated bytes with trusted Git's `hash-object --no-filters
-w --stdin`, then invokes fixed `update-index --add --cacheinfo` for
that specific path and confirms the resulting staged OID. It disables
hooks/fsmonitor and all inherited Git/hook/credential environment
variables, rejects `.git` symlinks/linked worktrees, uses no shell,
and requires the workspace to be the exact Git repository root.
The existing staged snapshot for **the selected file** is replaced in
full (no partial-hunk stage). Other staged files and working-tree bytes
are not intentionally modified.

All other Git write commands, including unstage, commit, push, pull,
fetch, reset and checkout, remain unavailable in the native E2EE console.
This is a staging-only experimental capability, not arbitrary Terminal,
file transfer, PHP-executed Git or authorization to launch LIVE.
`remote_control_available=false` is unchanged.


### Critical read-only Git safety gate (01/11/2026)

Git **status** and **diff** can invoke repository-configured Git clean filters,
even when no write is requested and `--no-ext-diff` is set. A legitimate
local repository with `.gitattributes` and malicious `filter.*.clean`
configuration can therefore execute shell code during a remote read.
To prevent GitStatus/FilesRead from becoming a command-execution bypass,
all E2EE remote Git read actions (status/status_full/log/branches/worktree,
staged and HEAD diff) now **fail closed** if repository Git configuration,
including `include.path` and conditional includes, defines any
`filter.*.(clean|process|smudge)` executable driver. The check uses the
trusted Git executable, a sanitized environment, no shell, and rejects
invalid/oversized configuration output. This also means that some legitimate
repos using Git LFS or custom clean/process filters cannot use remote Git
read views until a future independently verified filter-free implementation
is available. Local Git and the separately gated filter-free stage protocol
remain independent of this read guard.

Test `TestRemoteReadOnlyGitRefusesCleanFilterExecution` ensures none of
the seven remote Git reads can execute a malicious clean filter, while
`TestRemoteReadOnlyGitRejectsIncludedExecutableFilterConfig` verifies
config includes cannot hide such a driver. Both run in required Go CI.


## Experimental E2EE unstage one file (default OFF)

The trusted native console adds `git unstage <file>`. It reads the
**entire** staged diff for one workspace-relative existing regular file
(maximum 4096 UTF-8 bytes; refuses truncation), displays it with terminal
control escape filtering and requires an exact `UNSTAGE <file>` confirmation.
The client then sends `git.unstage` with a SHA-256 of the authenticated
staged diff and the exact literal path, consuming **two of six** E2EE actions.

The host requires **both host-only flags** `TASKDECK_REMOTE_GIT_WRITE=1` and
`TASKDECK_REMOTE_GIT_UNSTAGE=1`, in addition to per-action GitWrite,
FilesRead and FilesWrite TaskDeck RBAC, local password proof, client
Ed25519 key allowlist, pinned Agent, uncached live Portal grant, and host
audit *before* an index mutation. The flags default OFF. Index mutation
is serialized with the authenticated session's revoke lock and the
existing shared TaskDeck file mutation lease where enabled.

The host checks a pinned workspace file (no symlink traversal), a single
literal Git path (no pathspec patterns, `-flags` or `.git/` metadata),
a real local `.git` directory, safe Git executable and configuration
without executable filters. It recalculates and compares the full staged
diff and index entry immediately before mutation. Uses only fixed Git
plumbing to update that **one** entry to its committed HEAD blob/mode,
or removes the entry if the file is newly staged. Does not run
`git reset`, `git checkout`, shell, Git hooks, clean filters or arbitrary
revision/commands. Other staged entries and worktree bytes are untouched.
Unmerged index records, symlinks, submodules, absent worktree files, no
HEAD commit, binary diffs and race/conflict inputs are explicitly unsupported.
External writers not cooperating with TaskDeck's lock remain a documented
CAS race boundary; do not treat this feature as a distributed Git index
transaction.

This is **not** Git commit/push/pull, a PTY or permission to launch LIVE.
Keep `remote_control_available=false`.

## Read-only Git commit review (experimental)

The native trusted E2EE console supports `git commit-review`: a **single**
bounded read-only request, using a dedicated authenticated result type.
It returns the exact committed `HEAD` object ID, the symbolic
`refs/heads/<branch>`, SHA-256 of the **entire raw Git index**, and the
**complete staged diff** for all files (maximum combined 4096 UTF-8 bytes).
It refuses clean staged indexes, a detached/missing HEAD, linked worktrees,
symlinked or oversized indexes (>1 MiB), external object stores (Git\nsymlinks/alternates), replace-object refs, modified index/HEAD/branch during
read, executable filter configurations (including includes), non-UTF-8 or
binary diffs, and diffs that would require truncation.

This operation is *more privileged than per-file diff*: a full staged
patch can include all project files. It needs the real local TaskDeck
principal's GitWrite **and** FilesRead, live Portal grant, host-approved
client key, verified native TaskDeck login, E2EE channel and host audit.
The host uses trusted Git and fixed flags, never a shell or PHP-provided
revision/path. The command **never runs commit, Git hooks, clean filters,
Git index writes or any mutation**. It is distinct from `git commit`,
which remains explicitly unavailable and host-disabled. The preview
index hash is a read-time CAS hint; external Git writers may race after
preview. Never treat a printed review as permission for a commit without
a new host-side comparison and dedicated mutation authorization.

`remote_control_available=false`; not LIVE.

## Git commit via pinned, reviewed index (experimental, host OFF by default)

The trusted native E2EE console accepts the **literal** command `git commit`
without flags. It first runs `git.commit_review`, and prints the **entire**
bounded staged patch, exact old HEAD OID, branch reference and raw index
SHA-256. The client then requests a 3–96 byte ASCII message and exact
`COMMIT <first-12-hex-of-reviewed-HEAD>` confirmation. Only after both
prompts it sends a distinct typed `git.commit` operation with the full
review SHA-256 and message. Two of the six E2EE rounds are consumed.

**Host gate, default OFF:** BOTH `TASKDECK_REMOTE_GIT_WRITE=1` and
`TASKDECK_REMOTE_GIT_COMMIT=1` must be explicitly configured on the
TaskDeck host. The PHP Portal cannot change these values. Requires real
TaskDeck local login, host-allowlisted client Ed25519 key, Agent key pin,
uncached live Portal grant, host GitWrite + FilesRead + FilesWrite RBAC,
audit **before Git creates an object**, and session execution/revocation
lock. The entire review is rebuilt and hash-compared with the request.

The host copies the SHA-256-pinned Git index into a private temporary index,
runs **fixed trusted Git plumbing** `write-tree` and `commit-tree` (no
shell, no `git commit`, no hooks, no signing, no external filters, no
editor, no arbitrary Git identity or client Git argv), checks the review
and local index again, and runs `update-ref <branch> <new> <old>` with
Git's atomic expected-old-OID CAS. Commit author/committer are the
fixed host identity `TaskDeck Remote <taskdeck-remote@localhost.invalid>`,
not PHP or browser-supplied values. The working tree and original Git
index are not intentionally changed. No operation is allowed with a
detached/missing HEAD, executable Git filters, linked worktree, Git
alternates, symlinked Git object store, empty staged diff, oversized
review, binary diff or stale index/HEAD.

The temporary Git tree and commit objects can remain unreachable if the
final CAS fails. A separate Git process that writes the index outside
TaskDeck's locks can still race the last index verification; CAS protects
HEAD, but does **not** give a cross-process atomic index+HEAD transaction.
This limitation must be tested and addressed during production security
acceptance. No arbitrary Git push, pull, reset, checkout or PTY is opened.
`remote_control_available=false`. Not LIVE.

The host also owns a Git-standard exclusive `.git/index.lock` for the
entire final snapshot/commit/ref transaction, rejecting any preexisting
lock and releasing its own lock on completion. Normal Git index writers
honor this lock, reducing the external-writer race; a hostile process
that ignores Git locks or directly modifies refs remains outside the
atomicity guarantee.

## Terminal TaskDeck — preview và xác nhận host (chưa LIVE)

`terminal task <ID>` không thực thi ngay. Native trusted console
dùng **một action E2EE riêng** `task.list` để đọc toàn bộ danh mục
input-free process task đang được host phê duyệt bằng SHA-256 định nghĩa.
Client từ chối catalogue bị cắt / sai schema / có ID trùng, không
canonical, nhãn có control/bidi/escape, hoặc không có ID yêu cầu.
Client hiển thị tên task đã xác thực và chỉ gửi `task.run_text`
sau khi người dùng gõ chính xác `RUN TASK <ID>`. Hai action trên tổng
sáu action mỗi phiên; nếu thiếu quota thì không đọc catalogue.

Việc người dùng xác nhận chỉ là điều kiện giao diện: TaskDeck host
**luôn xác minh lại toàn bộ định nghĩa task** trước khi chạy, bao gồm
local `taskdeckRemoteAllowed=true`, approval digest trên biến môi trường
host, quyền TasksRun, grant Portal hiện hành, session password/client key,
allowlist executable tuyệt đối, cấm shell/arguments tuỳ ý, không truyền
ambient secrets, audit trước chạy, giới hạn 7 giây và 4KiB output đầy
đủ. PHP không thể phê duyệt task. Không có generic shell/PTY hoặc chuyển
mã lệnh người dùng đến subprocess.

`remote_control_available=false`; chưa được phát hành LIVE.

## Read-only paged source lines (experimental)

The trusted native E2EE console supports `view-lines <file> <start-line>`
for an existing **visible, regular UTF-8** workspace file up to 2 MiB.
It returns complete numbered source lines with a total-line count and
next-page line, never a truncated source line. One request counts as
one of the six short-session E2EE actions; each page is <=4096 bytes.
The normal `view` and all `edit` commands retain their strict <=4KiB
complete-file safety limit, and there is **no download UI**, arbitrary
disk read, shell, executable or file-transfer-mode transition.

Every response carries the **SHA-256 of the entire pinned source file**,
not just the returned text. The client refuses to display subsequent
pages from a different source SHA in the same native session, instead
of misleading the operator with mixed revisions. The host uses the
same descriptor-pinned path gate and snapshot stability checks as
existing remote file windows; it rejects symlinks, traversal, invalid
UTF-8, NUL, oversized files, missing lines and any single line too long
to display fully within the page. Server always requires TaskDeck
FilesRead, live Portal grant, authenticated E2EE and audit. Returned
text is terminal-escaped against control, ANSI and bidi spoofing.

Remote remains `remote_control_available=false`, not LIVE.

## Continue reading the same verified source with view-next (stacked PR)

After `view-lines <file> <start-line>`, use the literal `view-next`
(with **no arguments**) to request the authenticated next line from the
same normalized workspace file. Only the previously validated page
metadata can advance this cursor; it never accepts arbitrary file paths,
shell syntax, download commands or task identifiers. `view-next` does
nothing before a verified page, after EOF or after a detected source-SHA
conflict. Changing the local `cd` path cannot redirect the cursor to
another file. One page uses one authenticated E2EE action and retains the
six-actions-per-session bound.

No change to the TaskDeck Agent's FilesRead authorization, native
password, client-key allowlist, audit, Portal live grant or file-transfer
policy. This feature must NOT be interpreted as LIVE permission:
`remote_control_available=false`.

## Read-only literal search in up-to-2MiB source files (experimental)

`find-lines <file> :: <literal>` is a **typed E2EE** read-only operation,
separate from the existing <=4KiB-file `find`. It searches the entire
visible UTF-8 source (<=2 MiB), not just a local preview, and returns
**all complete matched source lines** (numbered), the total line count,
full-file SHA-256 and SHA-256 of the exact 1–96 byte UTF-8 literal.
If all matching lines will not fit the 4KiB E2EE result, the host
**denies the whole search** rather than concealing omitted matches.
Use a more specific literal to narrow results. No regex interpretation,
Git argv, executable filter, shell process or remote file download.

The host uses the same pinned parent/leaf file-descriptor and
before/after stability guards as `view-lines`; it checks FilesRead,
the live Portal grant, native TaskDeck password/client-key principal
and audit before sealing. The native console checks the exact query hash,
source SHA, complete match count and monotonically increasing line
numbers before exposing source text; it refuses to mix versions from
earlier source pages. Escape codes and bidi controls cannot control
the native terminal. The usual six E2EE rounds per session apply.

`remote_control_available=false`; no LIVE or generic PTY.

## Git branch-create — fixed namespace, authenticated review (experimental)

The native E2EE console supports `git branch-review` (read-only) and
`git branch-create <slug>`. The latter always performs a fresh
`git.branch_review` first, displays the current HEAD and symbolic branch,
then demands an exact `CREATE BRANCH taskdeck/<slug> <first-12-HEAD-hex>`
confirmation. The client sends the SHA-256 of the entire reviewed report
to a distinct authenticated `git.branch_create` action. These use two
of the six E2EE rounds.

**Host OFF by default.** Creating a branch requires BOTH host-local
`TASKDECK_REMOTE_GIT_WRITE=1` and
`TASKDECK_REMOTE_GIT_BRANCH_CREATE=1`, an authenticated TaskDeck
password session and approved Ed25519 key, pinned Agent identity, live
Portal grant, GitWrite + FilesRead + FilesWrite and audit **before**
ref modification. Neither PHP nor the operator can provide arbitrary
Git argv, the target commit, or a different ref namespace.

The only branch target is `refs/heads/taskdeck/<slug>` where
`<slug>` is lowercase ASCII (2–32 characters; first character a–z,
remaining a–z, digits or hyphen, not ending in hyphen). Host rereads
the symbolic current branch and HEAD, rejects unsafe symlinks, alternate
object stores and executable filters, and executes fixed trusted Git
`update-ref <new-ref> <reviewed-HEAD> <zero-old-OID>`.
Git's atomic expected-zero CAS refuses any existing loose or packed
branch, so this action **cannot force/overwrite** refs. Hooks disabled.
No checkout, HEAD modification, index/worktree mutation, Git push or
shell/PTY is implemented by this command.

An independent local Git process may advance HEAD after final review;
the new branch still points to the explicitly reviewed immutable
commit if the ref creation races that process. This is **not** an
atomic transaction with arbitrary external local writers. Production
acceptance must verify local filesystem, symlink and Git-config policy.
`remote_control_available=false`, no LIVE enablement.

## Managed task output cursors (merged staging, not LIVE)

Merged on main via PR #53 after exact-head Go/PHP CI PASS.\nThe native trusted console can read an already host-approved managed task's
terminal transcript **without silently truncating it**:
`terminal task-output <32hex-handle> <byte-offset>`. Begin at 0 and use
the authenticated `OUTPUT_NEXT` value for the next call. Every E2EE round
returns <=2048 UTF-8 bytes and carries `OUTPUT_OFFSET`, `OUTPUT_NEXT`,
`OUTPUT_TOTAL`, `TASK_STATE`, and `OUTPUT_EOF`. EOF=yes only once the
process has finished and the full recorded prefix was read. Output may be
still growing while a task is running. This is manual polling, not a PTY
stream, stdin bridge or file download.

TaskDeck's local session service tracks lifetime bytes even if a bounded
scrollback evicts earlier data. With the compatibility broker, ONLY the
host-created remote managed task runs on the daemon-local session manager;
ordinary terminals remain on their existing broker and no new remote HTTP
process-control endpoint is exposed. A stale byte cursor, invalid UTF-8 byte
boundary or older broker without verified offset support fails closed; the
host MUST NOT silently skip forward or rewrite the requested cursor.
Only this exact in-memory TaskDeck login, native E2EE channel, approved key,
TasksRun permission, live Portal grant and both default-OFF flags
`TASKDECK_REMOTE_TASK_RUN=1` and `TASKDECK_REMOTE_TASK_MANAGE=1` can read.
A finished legacy `task-status` also refuses a supposed complete result
if its source bytes were evicted or total output exceeds 3200 bytes; use
cursor windows instead. The existing 6-round E2EE application limit still applies, and the managed
task hard timeout remains 30 seconds. Read windows **before** claiming a
final `terminal task-status` result because the final status intentionally
retires its handle. Output > retained local scrollback cannot be recovered
in this mode. `remote_control_available=false` remains unchanged.

## Managed terminal tasks (merged staging, not LIVE)

Merged on main via PR #52 after exact-head Go/PHP CI PASS.\nThe native E2EE console adds `terminal task-start <ID>` after a host-approved
task catalogue and exact `START TASK <ID>` confirmation. The host responds
with a **random 128-bit task handle**, never a TaskDeck session ID or PID.
Use `terminal task-status <handle>` to query completion, or
`terminal task-cancel <handle>` followed by
`CANCEL TASK <first-12-hex-handle>` confirmation to request cancellation.

Host opt-in additionally requires `TASKDECK_REMOTE_TASK_MANAGE=1` alongside
`TASKDECK_REMOTE_TASK_RUN=1` (both OFF by default). At most ONE managed task
exists per authenticated in-memory TaskDeck principal; no persistence or
cross-login access. Hard 30-second process lifetime, and a host watchdog
attempts Stop/Kill on Portal grant revocation, E2EE channel closure, TaskDeck
logout, host flag removal or timeout. A cancel acknowledgement is deliberately
`cancel_requested`; use status to verify the process actually stopped.
Status remains bounded to complete <=3200-byte UTF-8 output or fails closed.
This is NOT a PTY, arbitrary command runner, stdin/resize, file download or
live stream. Relay sees only E2EE envelopes. `remote_control_available=false`.

## Terminal task exit reporting (merged staging, not LIVE)

Merged on main via PR #51 after exact-head Go/PHP CI PASS.\nThe trusted native console adds `terminal task-report <ID>`, using the
same host SHA-256 approved task catalogue and exact `RUN TASK <ID>`
confirmation as `terminal task <ID>`. The distinct typed E2EE result
contains `status=exited`, the TaskDeck process `exit_code` (0..255),
and complete output up to 3500 UTF-8 bytes. It never infers success from
program-controlled stdout. A nonzero exit is presented as a failed task.

The host still requires a TaskDeck-local password principal, an approved
native client key, TasksRun RBAC, active Portal grant, durable local audit,
`TASKDECK_REMOTE_TASK_RUN=1`, and the complete task approval digest.
Each run is limited to 7 seconds and is stopped on cancellation or
revocation. No arbitrary shell, PTY, stdin or file upload is exposed.
`remote_control_available=false` remains unchanged; LIVE is NO-GO.

## Large-file one-line editing (experimental staging, not LIVE)

The native `edit-large-line <file> <line>` (merged PR #50),
`delete-large-line <file> <line>` (merged PR #56), and new
`insert-large-line <file> <line>` (pending CI/merge in PR #57)
each read an authenticated `view-lines` page from an existing UTF-8
source up to 2 MiB. Every operation checks the complete SHA-256
of the source and uses two E2EE actions.

The operator must type the matching exact confirmation:
`EDIT LINE <line> <first-12-hex-SHA>`,
`DELETE LINE <line> <first-12-hex-SHA>`, or
`INSERT LINE <line> <first-12-hex-SHA>`.
Insertion is only possible **before an existing previewable line**,
not after EOF or into an empty file.

The host receives strictly typed `project.file.edit_line` with
up-to-512-byte new text, `project.file.delete_line` with an empty
content field, or `project.file.insert_line` with a bounded literal
line. Each uses its own authenticated E2EE receipt, distinct from
full-file replacement; no arbitrary shell, HTTP proxy, PHP code
or file-transfer capability is added.

The Agent requires a real TaskDeck password principal, FilesRead +
FilesWrite RBAC, approved client Ed25519 key, pinned Agent and live
Portal grant. It verifies the complete source snapshot and SHA-256
under the local mutation lease, durable local audit and session
revocation lock, then commits an atomic pinned-file rewrite.
Deletion preserves surviving LF/CRLF bytes; for an unterminated final
line it removes the preceding newline delimiter. Insertion selects
the target line's LF/CRLF convention (or the preceding delimiter
for an unterminated final line), preserving all other bytes.
Symlinks, invalid UTF-8, stale SHA, binary data, overlong lines,
over-limit file writes and unexpected concurrent changes fail closed.

Three host-only flags remain OFF by default; all are required:
`TASKDECK_REMOTE_FILE_REPLACE=1`,
`TASKDECK_REMOTE_TEXT_EDIT=1`, and
`TASKDECK_REMOTE_LARGE_LINE_EDIT=1`.
PR #56 merged after exact-head Go/PHP/native GUI CI PASS.
New insertion is not merged until PR #57 passes the same CI.
Remote control remains `remote_control_available=false`;
LIVE is NO-GO.

## Git redundant branch deletion (merged staging, not LIVE)

The native command `git branch-delete taskdeck/<slug>` uses an E2EE
`git.branch_delete_review` showing current HEAD, branch, exact target
ref/OID, index SHA-256 and verified clean status. It requires the exact
operator reply `DELETE BRANCH taskdeck/<slug> <first-12-hex-HEAD>`.
A second authenticated E2EE action sends a SHA-256 of the complete review.
Only refs under `taskdeck/` with the **same** commit as HEAD are eligible;
the current branch, dirty repositories, linked worktrees, symlinks,
packed-only refs, stale reviews or a different target OID are rejected.

Host local environment must set **both**
`TASKDECK_REMOTE_GIT_WRITE=1` and
`TASKDECK_REMOTE_GIT_BRANCH_DELETE=1`; both are OFF by default.
TaskDeck password principal, host-approved client key, pinned Agent,
Portal grant, GitWrite + FilesRead + FilesWrite, pre-write audit and
session revocation checks are enforced independently of PHP/Relay.
The host runs only a fixed Git `update-ref --no-deref -d` with old-OID
CAS; no arbitrary shell, branch command, checkout or file mutation.
**Merged on main in PR #49 after exact-head Go/PHP CI passed.
Remote control remains NO-GO.**

## Git same-HEAD switch (experimental staging only)

`git switch taskdeck/<slug>` is **not** unrestricted checkout. Native
client first requests an authenticated E2EE `git.switch_review` of
current HEAD, current branch, target ref/commit, whole-index SHA-256
and clean Git status, then requires exact confirmation
`SWITCH taskdeck/<slug> <first-12-hex-HEAD>`. The write action sends
the SHA-256 of this entire review; 2 E2EE rounds are required.

Host refuses if target is missing, symbolic/symlink/packed, outside
`refs/heads/taskdeck/`, points to a different commit, or if any
tracked/staged/untracked change exists. Host has a fixed trusted Git
command, hooks and filters disabled, and holds `.git/HEAD.lock` and
the target ref lock during full revalidation. Only the `.git/HEAD`
symbolic ref is atomically changed; no checkout, reset, file mutation,
Git index write, shell, fetch or remote Git push. A stale review or
external standard Git lock writer is denied, and only our own lockfiles
can be cleaned up.

**Host OFF by default.** Requires BOTH
`TASKDECK_REMOTE_GIT_WRITE=1` and
`TASKDECK_REMOTE_GIT_SWITCH_SAME_HEAD=1` on host, plus TaskDeck
password+approved Ed25519 client key, pinned Agent identity, live
Portal grant, GitWrite/FilesRead/FilesWrite and audit before side effect.
A non-cooperating local process that writes .git internals directly
outside Git locking is outside these atomicity guarantees; no LIVE
until independent field security testing. `remote_control_available=false`.

## Trusted native GUI — reviewed Git actions (PR #59, experimental staging)

Start the independently verified native client with `--gui` after the
pinned-Agent client-key preflight, local TaskDeck password and grant check.
The browser page is served **only** by the native client on 127.0.0.1; the
Portal cannot provide HTML/JS, Git argv or an executable shell. Under Git:

1. Review `Status`, `Log`, `Branches`, per-file `Diff`, `Staged` and `HEAD diff` as read-only actions.
2. Choose **Review Stage file**, **Review Unstage file**, **Review Commit**,
   **Review create branch**, **Review switch branch** or **Review delete branch**.
3. Read the full authenticated preview output in the result pane. Enter the
   exact one-time confirmation printed by the native client (45s expiry).
   For commit, enter a printable ASCII message of 3–96 characters.
4. Confirm. The host independently rechecks TaskDeck identity, RBAC, approved
   client key, live grant, local opt-ins, audit and SHA-256 snapshot CAS;
   a rejected or stale snapshot is not retried automatically.

Each mutation uses two of the six E2EE rounds available in the five-minute
session. Re-login through the trusted client for additional actions. The
host-side Git write environment flags documented above remain OFF by
default, and the GUI never bypasses them. Branch operations are restricted to
`taskdeck/<slug>`: create without checkout, same-HEAD clean switch, and
redundant same-HEAD deletion of a non-current branch. Merge, rebase,
push/pull/fetch and checkout across commits are **not** available through
this GUI. Releasing LIVE requires the independent deployment and hostile
PHP/Relay acceptance gates; a green CI result alone is insufficient.

## Native GUI small UTF-8 file editor (PR #60; experimental, no LIVE)

In the trusted, independently distributed native GUI, open **Files**,
select a non-hidden workspace-relative file (at most 4096 UTF-8 bytes)
and choose **Mở file để chỉnh sửa**. The host sends the complete
authenticated file preview over a typed E2EE round; the local GUI
fills a plain-text editor with that exact snapshot.

Change the text, read the current snapshot and enter the exact
`EDIT <file> <12-hex-SHA>` confirmation displayed by the native
client. The review is one-use and expires after 45 seconds. The native
client binds the file content to its full SHA-256 and sends the existing
`project.file.replace_text` action over a second authenticated E2EE
round. The host rechecks current TaskDeck identity/RBAC, live Portal
grant, owner-approved Ed25519 client key, default-off file write flags,
audit, workspace path, locks and whole-file SHA-CAS. A concurrently
changed file is **never overwritten** by this path.

UTF-8 input is limited to 4096 bytes and cannot contain NUL. Editing
the file to identical contents does not issue a write. This is NOT a
file upload/download feature, large-file editor, general shell, PHP
remote command interface, or live release authorization. The six-action
session budget and native browser local Origin/CSRF protections remain.

## Native Git GUI two-direction review (PR #61)

Use **Git → Review 2 chiều** for a source file to see both worktree
unstaged and index staged diffs in one clearly labeled result.
This requires two available authenticated E2EE rounds: `git.diff`,
then `git.diff_staged`. Each round independently checks TaskDeck
GitStatus/FilesRead, Agent identity, Portal grant and audit; a failure
in either round returns only an error and NEVER exposes a partial
review. The local GUI rejects metadata paths and command-like arguments.
No new Git write API, shell, push/pull or upload is enabled.

## Remote native GUI: one-line edits in large UTF-8 files (staging)

The **Files** panel now offers `Review sửa dòng`, `Review chèn dòng`
and `Review xóa dòng` for an existing, non-hidden, workspace-relative
UTF-8 file no larger than 2 MiB. Select a line number starting at 1.
Insertion is **before** the selected existing line. Each individual
operation reads one complete host-authenticated source line page with
whole-file SHA-256, reserves a second E2EE action, and requires the
exact `EDIT LINE`, `INSERT LINE` or `DELETE LINE` confirmation.
The original line, action type, exact path and line number are pinned
for 45 seconds and consumed after one attempted write. Editing a line
to unchanged content does not issue a host mutation.

The host checks the FULL-file SHA-256 again while holding its existing
file mutation lock, and independently enforces local TaskDeck login,
approved client key, Portal grant, FilesWrite permission and default-off
flags. This is *not* a general file transfer, large-file replacement,
shell/PTY or permission bypass. Only one 512-byte UTF-8 line is written
per confirmed action. The existing per-session six-action cap applies.

## Authenticated click-to-select Files explorer (PR #63)

In **Files → Duyệt thư mục**, the trusted native UI renders the
Agent's bounded E2EE directory listing as clickable folder/file
entries with separate styling. Selecting a folder populates the path;
choose **Duyệt thư mục** again to fetch that directory. Selecting a
file populates the read-only preview, large-line editor and Git path
inputs; it does *not* execute a request. **Thư mục cha** changes
only the local path field until the operator requests a new listing.

This feature uses the existing `project.dir.list` typed FilesRead
operation. On the client, the text listing is checked against the
192-byte response limit, 64-entry count, `D/F TAB name` syntax,
per-name UTF-8 length and visible relative path restrictions. Entries
are created using text-only DOM APIs and never interpolate HTML from
the host or PHP. No automatic navigation command, upload, shell,
file download or arbitrary Git invocation is enabled.

## Approved task cards in trusted native GUI (PR #64)

The **Tasks → Danh sách task** action retrieves the host-approved
TaskDeck task catalog over E2EE, verifies its complete bounded
`numeric ID<TAB>visible label` text format, then renders a clickable
task card for each listed task. Clicking a task selects only its numeric
ID. A separate reviewed `task_review` action and exact confirmation
remain mandatory before `task_execute`; there is no automatic
execution, shell, custom argv, env var, or PHP-served code path.

Malformed, duplicate, noncanonical, oversized or invisible task names
result in an empty, nonclickable catalog. Cards are created using
`document.createElement` and `textContent` only. Read-only catalog
UI does not weaken the Agent's existing independent TaskDeck login,
approved client key, per-session six-action limit, default-off task
execution flags, live Portal grants and audit.

## Native Git status changed-file picker (PR #65)

Click **Git → Status** to show the Agent-authenticated full tracked
Git status and a bounded clickable list of eligible file paths. Paths
that are hidden, escaped, quoted, renamed, command-option-like or
noncanonical are omitted rather than parsed as dangerous Git
arguments. Clicking a file merely selects that path for the existing
Files and Git preview/review controls; it never stages, commits or
runs a command automatically. Review and exact confirmation for any
Git write remain mandatory, subject to independent host SHA-CAS,
permissions, local TaskDeck authentication, live Portal grant and audit.

The catalog is a convenience view of the existing E2EE
`git.status_full` text, not a new Git backend or remote command API.

## Native Git taskdeck/ branch picker and UI behavioral tests

After **Git → Branches**, the local GUI shows selectable
`taskdeck/<slug>` branches from the authenticated `git.branches`
output. Only strict lowercase ASCII slugs and 40/64-character Git
OIDs are eligible for the selection list. Clicking a branch fills the
slug field for existing reviewed actions; it never switches, creates
or deletes a branch. Any prior visible Git write approval is cleared.

The Linux/macOS native GUI workflow also executes
`native_gui_dom_test.mjs` with Node's test runner, using the actual
bundled JavaScript and isolated synthetic E2EE host responses to
assert control behavior, limited data handling, and no implicit
write calls. This is additional regression coverage, not permission
to ship Remote LIVE.

## Managed task output pager — trusted native GUI

The Tasks panel can show a bounded, readable transcript for
a managed approved task. Enter the TaskDeck-owned 32-hex handle
returned by reviewed `task-start` and choose **Đọc output**.
The GUI reads one authenticated 2KiB task output window and
validates the exact handle, UTF-8 byte offsets, total, EOF,
state and expected E2EE response envelope. Choose **Đọc tiếp
trang đã xác thực** to request the next cursor without copying
`OUTPUT_NEXT`. No automatic polling/retry occurs.

Only contiguous windows for the same handle are concatenated.
At most four windows (8KiB unescaped source) are retained.
Output is displayed as text rather than HTML and terminal
control/bidi format symbols are escaped. Invalid windows disable
paging and cannot trigger a mutation or a new command.
The limit of six E2EE actions per login still applies.
This is a read-only managed-task transcript, **not a PTY,
arbitrary shell, file transfer or raw terminal control**.

## Safe authenticated source-page navigation in native Files GUI

Use **Xem theo dòng** to fetch one E2EE-authenticated line page
for a visible workspace-relative source path. **Xem trang tiếp theo**
is available only when the current complete source-page envelope has
a validated `NEXT_LINE`. The native Go GUI rejects forged or
truncated source pages, and the browser checks the whole-file
SHA-256 plus sequential line cursor again before concatenating.

If the file changes after the previous page, the cursor, source
SHA, or file selection changes unexpectedly, the GUI fails closed,
discards the old transcript and requires a fresh browse. It keeps
at most four pages in DOM memory, escapes control/bidi characters
and renders source text only. Every page uses one of six allowed
E2EE actions. This is manual read-only paging, not complete file
transfer or remote code execution.

## Click-to-select authenticated literal file search (PR #68)

In the Files panel, **Tìm trong file** uses the existing E2EE
literal `project.file.find_lines` operation. The trusted Go UI
validates the host's complete SHA/term-bound matching-lines
envelope before forwarding it. The browser independently parses
the bounded result and renders clickable matches in the form
`line: source text`, never as HTML.

Clicking a match only selects its one-based source line. Use
**Xem theo dòng** to request a new host-authenticated line page.
No write, shell command or network call is generated by selection.
A changed file path or search term invalidates previous matches;
starting another search clears the prior catalog immediately.
Invalid or incomplete host responses create no selectable entries.

## Adversarial localhost GUI ingress matrix (PR #69)

The native GUI Go race suite includes a compromised-PHP/Relay
regression matrix covering all typed Files, Git and Tasks
actions, including mutation review/execute, using hostile
Origin or missing/forged per-session local key. Even when a
test presents the correct ephemeral key with a hostile Origin,
no request is forwarded, no E2EE quota consumed and no
plaintext/review data is returned.

It also rejects forged Host headers, GET-to-mutation attempts,
wrong content types, unknown or concatenated JSON objects,
malformed JSON and oversized bodies before dispatch.
This tests the local GUI ingress boundary only. It does not
replace real external TLS/WSS deployment and independent
compromised-PHP acceptance; Remote LIVE remains NO-GO.

## Read-only console (strictly typed actions; no shell/PTY) — PR #70

The trusted native GUI includes a **Read-only Console** tab.
Supported commands are exactly: `help`, `git status`,
`git log`, `git branches`, `tasks list`,
`files list <relative-directory>`,
`files view <relative-file>`,
`tasks status <32-lowercase-hex-handle>` and
`tasks output <32-lowercase-hex-handle> <byte-offset>`.

This is a convenience text input for existing authenticated
read-only host operations; there is **no arbitrary executable,
shell expansion, PTY, Git write, file write or PHP command
handler**. Paths are canonical, workspace relative and checked
again by the native Go GUI and Agent. Unknown commands, shell
metacharacters, absolute paths, hidden Git paths and invalid
handles fail locally without consuming an E2EE round.

The console shows at most four escaped text-only results.
The existing six-round limit and host RBAC, key allowlist,
password check, live grant, E2EE and audit remain mandatory.
A console read cancels any open write confirmation.
This is an incremental interactive interface, not a
replacement for the pending authorized terminal PTY feature.

## Bounded source/Git reads and in-memory Console history (PR #71)

The trusted read-only Console now also supports
`git diff <relative-file>`, `git staged <relative-file>`,
`git head <relative-file>`, `files lines <relative-file> <line>`
and `files find <relative-file> <literal>`. Each maps directly
to an existing fixed read-only Agent E2EE operation. No arbitrary
Git revision, CLI arguments, shell execution, PTY or Git mutation.

Workspace paths are single-token Unicode-safe, canonical, relative
and reject hidden segments, option prefixes and shell control
characters. `files lines` accepts canonical 1..2097152.
Literal search accepts 1–96 UTF-8 bytes with no command metacharacters
or control/bidi characters. Invalid commands send **zero** E2EE
messages. The Agent still verifies permission, login, key approval,
grant, session limits and audit on each action.

Enter sends exactly one explicit read. Arrow Up/Down navigates a
maximum of 16 accepted commands stored **only in page memory**.
The `clear` command deletes local display output, does not reset
the E2EE quota and does not access the host.

## Read-only authenticated Git commit cards (PR #72)

The native Git panel now displays at most 30 recent commits
from the host's existing fixed `git.log` E2EE operation.
Every displayed card contains a parsed lowercase hexadecimal
short SHA plus bounded UTF-8 subject. A malformed, duplicated,
oversized or control/bidi-injected host response renders no cards.
The exact source is E2EE-authenticated and checked again in the
native browser code. Selecting a card updates **local text only**
and does not checkout, fetch, merge, push, execute Git argv or
send a network request. Local SHA/subject filtering costs no
Agent round and stores no data in PHP or browser persistence.
