# TaskDeck FTP & SFTP Connections Plan

Status: implementation complete in feature branch; automated CI green at checkpoint before final docs; real-server FTP/SFTP smoke still pending

Branch: `feat/ftp-sftp-connections`

Base: `main` at `91bd38af04381eb6566d28d3c203b8cca857a9db`

## 1. Goal

Extend the existing **CONNECTIONS** area with reusable FTP and SFTP file-transfer connections while preserving the current LOCAL, SSH and DATABASES behavior.

The feature is intended to provide a practical remote-file workflow directly in TaskDeck:

- save/edit/clone/delete connection profiles;
- test a connection before or after saving;
- browse remote directories;
- download files;
- upload files;
- create directories;
- rename files/directories;
- delete files/directories with confirmation;
- refresh the current directory;
- preserve credentials in the existing encrypted secret store;
- keep operations bounded and auditable;
- remain offline-buildable.

## 2. Mandatory dependency constraints

These are hard requirements:

- Do not add npm packages.
- Minimize third-party libraries.
- Do not introduce build-time downloads.
- If a third-party source becomes unavoidable, vendor its source into this repository and compile from the vendored copy.
- Prefer Go standard library and existing TaskDeck infrastructure.
- Preserve `GOPROXY=off GOSUMDB=off` tests/builds.
- Keep checkpoint commits small and independently reviewable.
- Secrets must never appear in browser-visible profile JSON, URLs, argv, logs, audit payloads, terminal titles or diagnostic text.
- Existing SSH/database behavior must remain unchanged unless the user explicitly uses FTP/SFTP.

## 3. Dependency decision

### 3.1 SFTP: use system OpenSSH `sftp`

TaskDeck already uses the system OpenSSH `ssh` client for SSH terminals and SSH tunnels. SFTP will follow the same architecture and use the system `sftp` executable instead of importing an SSH/SFTP Go library.

Reasons:

- zero new Go module dependency;
- reuses the user's OpenSSH installation, keys, agent, known_hosts and ProxyJump behavior;
- reuses TaskDeck's existing encrypted secret store + one-time `SSH_ASKPASS` bridge;
- avoids vendoring `golang.org/x/crypto/ssh` and an SFTP library;
- the OpenSSH `sftp` program is the native SFTP client and performs operations over SSH transport.

TaskDeck must probe `sftp` availability and report an actionable runtime error if it is missing.

### 3.2 FTP: built-in Go stdlib client

TaskDeck will implement the required FTP client subset using only Go standard library packages such as:

- `net`;
- `bufio`;
- `io`;
- `context`;
- `time`;
- `strconv`;
- `strings`.

Do not add an FTP package merely for convenience.

Protocol choices:

- control connection: RFC 959 semantics;
- binary transfers: `TYPE I`;
- passive data connections only;
- prefer `EPSV` (RFC 2428);
- IPv4 fallback to `PASV` only when EPSV is unsupported;
- prefer `MLSD` machine-readable directory listings (RFC 3659);
- bounded fallback parsing of `LIST` only when MLSD is unavailable.

Plain FTP sends credentials/data without transport encryption. The UI must label this clearly.

Explicit FTPS can be added later using Go `crypto/tls` without third-party dependencies, but it is not required for the first FTP checkpoint and must not be silently conflated with SFTP.

## 4. Profile model

Use a dedicated file-transfer profile store instead of overloading database or SSH profile JSON.

Conceptual model:

```text
FileTransferProfile
    id
    name
    protocol = ftp | sftp

    # FTP fields
    host
    port
    username
    secret_ref
    initial_path

    # SFTP fields
    ssh_profile_id
    initial_path

    connect_timeout_seconds
```

Rules:

- FTP default port: 21.
- SFTP uses an existing SSH profile instead of duplicating SSH host/authentication fields.
- SFTP therefore inherits:
  - host/port/user;
  - agent/private-key/password authentication;
  - identity file;
  - encrypted password/passphrase;
  - ProxyJump;
  - host-key verification policy;
  - connection timeout.
- SFTP does not run SSH terminal preset commands or custom terminal bootstrap.
- `initial_path` is a file-browser starting directory only.
- The file-transfer profile API exposes `has_secret` for FTP but never `secret_ref`.
- SFTP profiles contain only `ssh_profile_id`; they never copy an SSH secret.

Default config file:

```text
~/.config/taskdeck/file_transfer_profiles.json
```

Credential backup documentation must be updated to include this file.

## 5. File browser API

Use resource-oriented HTTP APIs with JSON metadata and bounded streaming endpoints.

Proposed endpoints:

```text
GET    /api/file-transfer/profiles
POST   /api/file-transfer/profiles
PUT    /api/file-transfer/profiles/{id}
DELETE /api/file-transfer/profiles/{id}

POST   /api/file-transfer/test
POST   /api/file-transfer/list
POST   /api/file-transfer/mkdir
POST   /api/file-transfer/rename
POST   /api/file-transfer/delete

POST   /api/file-transfer/upload
GET/POST download endpoint chosen to avoid secrets/unsafe remote paths in URL
```

Every operation accepts a profile ID and normalized remote path. Stored passwords are resolved server-side.

The browser must never receive FTP/SFTP credentials.

## 6. Remote path safety

Remote paths are not local filesystem paths and must not be passed through local `filepath.Clean` semantics.

Introduce protocol-neutral POSIX-style remote-path normalization:

- reject NUL and CR/LF;
- enforce a maximum encoded length;
- normalize separators to `/` where appropriate;
- preserve absolute vs relative semantics;
- prevent empty file names for mutation operations;
- never concatenate unquoted user paths into a shell command.

For SFTP batch input, each path must be encoded using the OpenSSH sftp command language quoting rules. Newlines/control characters remain rejected.

For FTP, paths are sent only as FTP command arguments after control-character validation.

## 7. SFTP process model

Use short-lived `sftp` subprocesses for initial implementation.

Each request:

1. loads the referenced SSH profile;
2. resolves system `sftp`;
3. builds safe OpenSSH arguments;
4. creates a one-time askpass ticket if the SSH profile has a stored secret;
5. sends a bounded batch program;
6. captures bounded stdout/stderr;
7. parses the result;
8. kills the subprocess on timeout/cancellation.

Authentication options should mirror the existing SSH profile policy as closely as the `sftp` CLI permits.

Important separation:

- do not invoke a remote shell;
- do not use SSH terminal preset commands;
- do not disable host-key checking;
- do not place a secret in command arguments.

Initial SFTP operations:

- test: `pwd`;
- list: `ls -la` or a more parseable supported form, with a strict parser and tests;
- download: `get`;
- upload: `put`;
- mkdir: `mkdir`;
- rename: `rename`;
- delete file: `rm`;
- delete directory: `rmdir`.

Directory deletion is non-recursive initially. Recursive delete requires an explicit later design.

## 8. FTP client model

Implement a small stateful client per request/session.

Control sequence:

```text
TCP dial
220 greeting
USER
PASS if required
TYPE I
optional FEAT
operation
QUIT
```

Data operation sequence:

1. enter passive mode using EPSV;
2. if EPSV is unsupported and the control connection is IPv4, fall back to PASV;
3. open the data TCP connection;
4. send MLSD/LIST/RETR/STOR;
5. read data with operation-specific limits;
6. require final 2xx completion reply.

Supported initial commands:

- `NOOP` / `PWD` for test;
- `CWD`;
- `MLSD`, fallback `LIST`;
- `RETR`;
- `STOR`;
- `MKD`;
- `RNFR` + `RNTO`;
- `DELE`;
- `RMD`;
- `SIZE` where useful.

Security/resource rules:

- password only comes from secret store;
- no credential in argv because FTP is implemented in-process;
- apply connect/read/write deadlines;
- cap control reply length;
- cap directory listing bytes and entry count;
- stream file bodies rather than buffering entire files;
- passive data target defaults to the control peer address for EPSV;
- PASV address handling must avoid blindly trusting an unrelated server-supplied address.

## 9. UI design

Add two visible sections to CONNECTIONS:

```text
CONNECTIONS

LOCAL
SSH
SFTP
FTP
DATABASES
```

### SFTP profile form

Fields:

- Name
- SSH profile
- Initial remote path

Actions:

- Test
- Save
- Clone
- Delete
- Edit

### FTP profile form

Fields:

- Name
- Host
- Port (default 21)
- Username
- Password
- Initial remote path
- Connect timeout

Actions:

- Test
- Save
- Clone
- Delete
- Edit

The FTP form must display a plain-language warning that ordinary FTP is not encrypted.

### Remote file workspace

Opening an FTP/SFTP profile opens a file-transfer workspace, separate from the narrow Connections sidebar.

Minimum controls:

- profile/protocol title;
- current remote path;
- Back/Up;
- Refresh;
- Upload;
- New Folder;
- file table with Name / Size / Modified / Type;
- double-click directory to enter;
- double-click file or Download action to download;
- context menu: Download, Rename, Delete;
- clear busy/error status.

Do not use npm UI libraries.

## 10. Upload/download design

### Download

The server streams the remote file directly to the HTTP response.

Requirements:

- bounded headers;
- safe filename for `Content-Disposition`;
- cancellation closes FTP data connection / kills SFTP process;
- no temporary local copy unless a protocol limitation forces one;
- no remote path in logs.

For SFTP, because OpenSSH `sftp get` fundamentally writes a local file, the first implementation may use a private `0700` temp directory and `0600` file, then stream and delete it. This is acceptable only if cleanup is guaranteed on success/failure/cancellation.

### Upload

Browser sends multipart data to TaskDeck.

Requirements:

- server enforces configurable/max upload size;
- filename/path validation;
- temporary files, if needed for SFTP `put`, use private permissions and guaranteed cleanup;
- FTP STOR should stream request data directly where practical.

## 11. Audit and shared-server compatibility

Add connection audit kinds/actions without recording remote path or file names:

```text
file_transfer_profile create/update/delete
file_transfer_connection test
file_transfer list
file_transfer download
file_transfer upload
file_transfer mkdir
file_transfer rename
file_transfer delete
```

Audit metadata may contain:

- profile ID;
- protocol;
- success/failure.

It must not contain:

- host;
- username;
- password/passphrase;
- remote path;
- local temp path;
- file content.

Shared-server permissions can later map to:

```text
file_transfer.profiles.view
file_transfer.profiles.manage
file_transfer.connect
file_transfer.read
file_transfer.write
```

Server-side authorization remains mandatory; hiding buttons is not a security boundary.

## 12. Implementation checkpoints

### Phase 0 — design

- [x] Create `feat/ftp-sftp-connections` from current `main`.
- [x] Record dependency/security/API/UI decisions in this plan.
- [x] Add CI coverage for the feature branch while implementation is active.

### Phase 1 — file-transfer profile foundation

- [x] Add `internal/filetransferprofile` model and validation.
- [x] Add atomic/locked `file_transfer_profiles.json` store.
- [x] Add tests for normalization, limits, duplicate IDs and persistence.
- [x] Add server CRUD API.
- [x] Reuse encrypted secret store for FTP passwords.
- [x] Ensure API projection never exposes `secret_ref`.

### Phase 2 — SFTP transport

- [x] Add `FindOpenSFTP`.
- [x] Build safe sftp arguments from referenced SSH profile.
- [x] Reuse one-time askpass.
- [x] Add bounded batch/interactive stdin runner.
- [x] Add test/list/get/put/mkdir/rename/rm/rmdir operations.
- [x] Add parser fixtures for directory listing output.

### Phase 3 — native FTP transport

- [x] Implement bounded FTP reply parser.
- [x] Implement login + TYPE I.
- [x] Implement EPSV and IPv4 PASV fallback.
- [x] Implement MLSD parser; FEAT probing was not required for the first implementation.
- [x] Add bounded LIST fallback.
- [x] Implement RETR/STOR/MKD/RNFR+RNTO/DELE/RMD.
- [x] Add fake-server tests; no real network dependency in unit tests.

### Phase 4 — operation API

- [x] Add test/list/mkdir/rename/delete endpoints.
- [x] Add upload/download endpoints; FTP streams directly and SFTP uses private temporary files.
- [x] Add timeouts, cancellation and resource limits.
- [x] Add audit events with no sensitive path/credential data.

### Phase 5 — CONNECTIONS UI

- [x] Load FTP/SFTP profiles in `connections.js`.
- [x] Add FTP and SFTP sections.
- [x] Add profile forms and draft connection tests.
- [x] Add clone/edit/delete context actions.
- [x] Add plain FTP security warning.

### Phase 6 — remote file workspace

- [x] Add vanilla-JS remote file workspace.
- [x] Browse directories.
- [x] Upload/download.
- [x] New folder.
- [x] Rename/delete.
- [x] Refresh and current-path navigation.
- [x] Add UI contract tests.

### Phase 7 — docs and release validation

- [x] Update `CONNECTIONS.md`.
- [x] Update README feature list in both languages where appropriate.
- [x] Update credential backup/restore docs.
- [x] Run `go test ./...` in CI on Go 1.19.x and 1.23.x.
- [x] Run `go vet ./...` in CI.
- [x] Run offline build with `GOPROXY=off GOSUMDB=off` in CI.
- [ ] Remove temporary feature-branch CI trigger before merge.

## 13. Explicit non-goals for the first implementation

To keep the first release safe and dependency-free:

- no npm packages;
- no third-party FTP/SFTP package;
- no recursive remote delete;
- no remote chmod/chown;
- no synchronization engine;
- no automatic conflict resolution;
- no background mirroring;
- no remote file editing before download/upload semantics are stable;
- no implicit acceptance of changed SSH host keys;
- no embedding FTP passwords into URLs.

## 14. Acceptance criteria

The feature is ready to merge when:

1. existing LOCAL/SSH/DATABASES workflows remain green;
2. FTP and SFTP profiles can be created, edited, cloned, tested and deleted;
3. stored credentials never appear in browser API responses;
4. SFTP works using the system OpenSSH client and existing SSH profile authentication;
5. FTP works without third-party libraries;
6. remote listing, download, upload, mkdir, rename and non-recursive delete work for both protocols;
7. all operation paths are validated and bounded;
8. temporary files are private and reliably removed;
9. unit tests do not require internet access;
10. offline Go test/build succeeds with `GOPROXY=off GOSUMDB=off`;
11. real-server smoke is performed before merge/release for at least one FTP server and one OpenSSH SFTP server.


## 15. Implemented checkpoint summary

Implemented on `feat/ftp-sftp-connections`:

- profile model/store + encrypted FTP password references;
- SFTP via system OpenSSH only;
- native Go-stdlib FTP transport;
- Test/List/Mutation/Upload/Download APIs;
- one-time 90-second download tickets compatible with TaskDeck browser lease;
- vanilla-JS remote file workspace;
- FTP/SFTP sections in Connections;
- fake FTP and fake SFTP fixtures plus UI contract tests;
- no npm package and no new third-party Go module.

Before merge/release:

- run real-server smoke against FTP and SFTP;
- decide and implement explicit shared-server RBAC integration if FTP/SFTP is required in shared mode;
- remove the temporary feature-branch CI trigger when merging.
