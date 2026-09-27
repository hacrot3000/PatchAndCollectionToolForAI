# SSH & Database Connections

TaskDeck có panel **Connections** cho terminal local, SSH và database. Phần này mô tả cách dùng, giới hạn bảo mật và cách phục hồi credential.

## SSH profiles

Trong **Connections → SSH**, tạo profile gồm:

- tên profile;
- host/port/user;
- authentication bằng SSH agent, private key hoặc password;
- optional ProxyJump;
- optional remote home directory;
- optional preset commands.

TaskDeck dùng system OpenSSH client. Không có SSH library riêng và không tắt host-key verification.

Host-key policy:

- agent/private key không có stored secret: `StrictHostKeyChecking=ask`;
- password hoặc encrypted private-key passphrase dùng one-time askpass: `StrictHostKeyChecking=accept-new`;
- changed host key vẫn bị OpenSSH từ chối.

Password/passphrase không được đưa vào argv. Stored secret được lấy từ encrypted secret store và cấp cho OpenSSH qua one-time Unix-socket askpass ticket.

## Database profiles

Database profile chọn adapter và transport.

Supported adapters:

| Adapter | Runtime dependency | Direct | SSH tunnel | Initial execute behavior |
| --- | --- | --- | --- | --- |
| MySQL | system `mysql`/MariaDB-compatible CLI | yes | yes | SQL, bounded result |
| Redis | built-in pure-Go RESP2 client | yes | yes | read-oriented allowlist |
| MongoDB | system `mongosh` | yes | yes | bounded JSON `find` DSL |
| SQLite | Python 3 stdlib `sqlite3` | local file only | no | SQL, explicit read-only/read-write |

TaskDeck advertises MySQL/MongoDB/SQLite adapters only when their required local runtime can be probed successfully. Redis is always available because its client is built into TaskDeck.

### SSH tunnel transport

MySQL, Redis and MongoDB can select **SSH tunnel** and reference an SSH profile.

The database profile stores the **remote database host/port**. TaskDeck then:

1. reserves an ephemeral local IPv4 loopback port;
2. starts system OpenSSH with `-N -L 127.0.0.1:<local>:<remote-host>:<remote-port>`;
3. uses `ExitOnForwardFailure=yes`;
4. waits until the local listener is ready;
5. gives only `127.0.0.1:<local-port>` to the database adapter;
6. tears the tunnel down when the DB session ends, adapter crashes, connect fails or TaskDeck shuts down.

Tunnel listeners never bind wildcard addresses.

### Redis safety

The initial Redis adapter is deliberately read-oriented. Generic execute supports an explicit non-destructive allowlist such as:

`PING`, `GET`, `MGET`, `EXISTS`, `TYPE`, `TTL`, `HGET`, `HGETALL`, `LRANGE`, `SMEMBERS`, `ZRANGE`, `SCAN` and related read commands.

Commands including `SET`, `DEL`, `FLUSHALL`, `CONFIG`, `EVAL` and `MULTI` are rejected inside the adapter before being sent to Redis.

### MongoDB safety

MongoDB uses `mongosh --nodb --norc --quiet --file <temporary-script>`.

Credentials are not placed in argv. Each operation uses a short-lived script under a private temporary directory; the script file is mode `0600` and is removed after the invocation.

Generic execute does **not** accept JavaScript. It accepts JSON like:

```json
{
  "op": "find",
  "database": "main",
  "collection": "users",
  "filter": {"active": true},
  "sort": {"created_at": -1},
  "limit": 100
}
```

`$where`, `$function` and `$accumulator` are rejected because they can execute server-side JavaScript.

### SQLite safety

SQLite is local-file only.

A profile must point to an existing regular file. TaskDeck resolves symlinks before opening it and rejects host/port/user/password/SSH-tunnel fields for SQLite.

Read-only mode uses SQLite URI `mode=ro` plus `PRAGMA query_only=ON`. Read-write mode must be explicitly selected.

The helper is embedded into the TaskDeck binary and runs through Python 3 isolated mode (`-I`) using only stdlib `sqlite3`; no pip dependency is required.

## Result and resource limits

The database adapter protocol bounds untrusted/process output:

- protocol message: 1 MiB;
- result rows: 1000;
- result columns: 256;
- object browser entries: 5000;
- serialized cell: 256 KiB;
- adapter/tunnel diagnostics: 64 KiB.

Redis additionally bounds RESP line, bulk, array and nesting sizes. MongoDB and SQLite subprocess stdout/stderr are bounded and killed/rejected when output exceeds the configured adapter limit.

## Stored secrets

Default user config files live under the OS user-config directory in the `taskdeck` subdirectory. On Linux this is normally:

```text
~/.config/taskdeck/
├── master.key
├── secrets.json
├── ssh_profiles.json
└── db_profiles.json
```

When `XDG_CONFIG_HOME` is set, that location is used instead of `~/.config`.

Security properties:

- config directory: `0700`;
- key/profile/secret files and locks: `0600`;
- stored secrets: AES-GCM encrypted;
- each encrypted record is authenticated with its secret ID as associated data;
- browser APIs expose only `has_secret`, never the plaintext or internal `secret_ref`.

### Rotate a password/passphrase

Use **Connections → Edit** and enter the replacement password/passphrase.

TaskDeck writes the new encrypted secret first, updates the profile to the new reference, then deletes the old encrypted record. Leaving the field blank keeps the current secret. For database profiles, **Clear saved password** removes it. Changing an SSH profile to agent auth also removes the previous stored secret.

### Backup and restore

For a usable credential backup, keep these files together:

- `master.key`;
- `secrets.json`;
- `ssh_profiles.json`;
- `db_profiles.json`.

Preserve private file permissions when restoring.

`secrets.json` without the matching `master.key` cannot be decrypted. The master key is intentionally not derivable from a password or account.

### Recovery after losing/corrupting master.key

If `master.key` is lost or replaced, existing encrypted records are intentionally unrecoverable.

Recovery procedure:

1. stop TaskDeck processes that may be using Connections;
2. preserve the broken `master.key` and `secrets.json` elsewhere if forensic recovery is needed;
3. move/remove the unusable secret store files;
4. start TaskDeck so a fresh master key/store can be created;
5. edit affected SSH/database profiles and re-enter their passwords/passphrases.

Do not edit `secret_ref` values by hand. The UI/API rotates references atomically with secret storage.

## Offline install and self-update

TaskDeck's Go dependencies remain vendored through local `replace` directives. Installation runs:

```bash
GOPROXY=off GOSUMDB=off go test ./...
GOPROXY=off GOSUMDB=off go build ...
```

The installer also verifies the embedded SQLite helper source is present before build. Redis adds no external package; MongoDB and SQLite dependencies are runtime executables (`mongosh` and Python 3), not build-time downloads.

## Current shared-server boundary

This branch keeps the existing single-user/browser security model. Connections does not introduce a separate authorization or audit subsystem.

Shared-server RBAC/audit integration is intentionally deferred until the shared-server architecture is merged into the target branch; it should reuse that common authorization/audit layer rather than create a second incompatible system inside Connections.
