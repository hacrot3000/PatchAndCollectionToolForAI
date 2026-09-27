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

### Database Workbench UI

Sau khi mở một database profile, workspace database có ba tab chính:

- **Data** — xem dữ liệu theo trang và edit trực tiếp khi adapter/object cho phép;
- **Structure** — xem columns, primary key/index metadata, SQL definition hoặc collection info;
- **Query** — giữ nguyên direct SQL/query/Redis command editor hiện có.

Navigator bên trái hỗ trợ:

- lọc nhanh object theo tên/type;
- click để inspect;
- double-click table/view/collection/key để mở Data;
- click phải để mở context menu theo object type;
- copy object name / qualified name;
- refresh schema/object list;
- sinh query template phù hợp adapter.

Context actions hiện gồm các thao tác phù hợp từng adapter:

| Adapter/object | Common context actions |
| --- | --- |
| MySQL/SQLite table | View Data, Inspect, Count Rows, Generate SELECT/INSERT/UPDATE/DELETE, Truncate, Drop |
| MySQL/SQLite view | View Data read-only, Inspect, Count Rows, Generate SELECT, Drop View |
| MongoDB collection | View Data, Inspect, Count Documents, Generate Find, Clear Collection, Drop Collection |
| Redis key | View Value, Inspect, Count Entries, Generate Read Command, Delete Key |

Destructive actions luôn bị gate bởi profile read-only và có confirmation riêng. Drop/Delete yêu cầu gõ lại object/key name trước khi gửi operation.

#### Paged Data grid

Grid dùng server-side paging, không tải toàn bộ table/collection vào browser.

Page size hỗ trợ:

`25 / 50 / 100 / 250 / 500 / 1000`

Page size được nhớ theo profile/catalog/object trong browser. Grid có First / Previous / Next, row range, optional total count và trạng thái editable/read-only.

MySQL/SQLite hỗ trợ server-side sort và filter descriptor. Filter UI hỗ trợ tối đa 16 điều kiện AND:

- `=`, `!=`, `<`, `<=`, `>`, `>=`;
- `contains`;
- `starts with`;
- `IS NULL`;
- `IS NOT NULL`.

Browser chỉ gửi descriptor có cấu trúc; browser không tự nối SQL filter. Adapter chịu trách nhiệm identifier quoting/value binding hoặc safe expression construction.

#### Inline editing

Khi adapter trả stable row identity và profile cho phép ghi:

- cell có thể edit trực tiếp;
- dirty rows được đánh dấu;
- **+ Row** thêm row/document/member mới;
- row context menu có Delete/Restore;
- cell context menu có Set NULL khi column cho phép;
- long text/JSON/document có editor riêng;
- **Apply changes** gửi batch mutation có identity;
- **Revert** bỏ thay đổi chưa apply.

Update/delete không dựa vào toàn bộ giá trị đang hiển thị. Adapter phải xác nhận stable identity:

- MySQL: primary key hoặc non-null unique key;
- SQLite: primary key, fallback `rowid` chỉ khi table thực sự có rowid;
- MongoDB: `_id`;
- Redis Hash: field;
- Redis Set/ZSet: member.

Nếu identity không an toàn, Data grid tự chuyển read-only và hiển thị lý do.

Keyboard shortcuts trong Data:

- `Ctrl/Cmd+S`: Apply changes;
- `Alt+N` hoặc `Alt+Insert`: thêm row khi editable;
- `Enter`: kết thúc edit cell và chuyển tiếp;
- `Esc`: bỏ text cell đang edit nhưng chưa commit vào dirty state.

#### Adapter-specific behavior

**MySQL / MariaDB**

- full paged grid;
- PK/non-null unique identity;
- insert/update/delete;
- sort/filter;
- count/truncate/drop;
- columns + indexes trong Structure;
- direct SQL vẫn nằm ở Query tab.

**SQLite**

- local-file only như trước;
- paging/sort/filter;
- insert/update/delete;
- PK/`rowid` identity;
- count/delete-all/drop;
- columns/indexes/DDL trong Structure;
- Python stdlib helper dùng parameter binding cho Workbench operations.

**MongoDB**

- collection paging theo `_id`;
- document hiển thị/edit dưới dạng JSON;
- insert/replace/delete dùng `_id` identity;
- Clear/Drop Collection;
- Generate Find tạo JSON DSL trong Query tab;
- browser không gửi JavaScript tùy ý cho `mongosh`.

**Redis**

- key navigator vẫn dùng SCAN bounded;
- double-click key mở type-aware viewer;
- String: single value, read-only trong grid;
- List: index/value, read-only trong grid;
- Hash: field/value, hỗ trợ add/edit/delete field;
- Set: member, hỗ trợ add/delete;
- ZSet: member/score, hỗ trợ add/edit score/delete;
- Count Entries và Delete Key qua context menu;
- generic Query tab vẫn chỉ chấp nhận read-oriented command allowlist.

String/List cố ý chưa bật generic grid mutation vì semantics Add/Delete của relational grid không ánh xạ an toàn, rõ ràng sang hai type này.

#### Grid safety / resource boundaries

Workbench vẫn đi qua cùng process-isolated adapter protocol. Các request/response bị giới hạn bởi protocol limits hiện có, gồm page size tối đa 1000, 256 columns và 256 KiB/cell.

Mutation browser API chỉ expose ba operation generic đã normalize:

- `browse_rows`;
- `mutate_rows`;
- `object_action`.

Stored credential, `secret_ref`, SQL generated internally và raw mutation values không được đưa vào connection audit log.

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

## Audit hook and current shared-server boundary

Connections emits a small server-side audit event for sensitive successful/failed operations such as profile mutation, SSH connection tests/terminal starts and DB session operations.

The event contains only:

- kind;
- action;
- profile ID;
- session ID;
- success/failure.

It intentionally does not contain password/passphrase, host, SQL, Redis command, Mongo filter, request body or query string. If no external audit sink is configured, TaskDeck writes the same minimal metadata to its existing logger.

This branch keeps the existing single-user/browser authorization model. Shared-server RBAC and user-aware audit attribution are intentionally deferred until the shared-server architecture is merged into the target branch; that architecture can attach its own sink through the connection audit hook rather than create a second incompatible auth/audit system inside Connections.
