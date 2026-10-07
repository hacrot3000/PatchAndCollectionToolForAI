# Connections: Local, SSH, SFTP, FTP & Database

TaskDeck có panel **Connections** cho terminal local, SSH, SFTP, FTP và database. Phần này mô tả cách dùng, giới hạn bảo mật và cách phục hồi credential.

## SSH profiles

Trong **Connections → SSH**, tạo profile gồm:

- tên profile;
- host/port/user;
- authentication bằng SSH agent, private key hoặc password;
- optional ProxyJump;
- optional remote home directory;
- optional preset commands;
- connect timeout;
- `ServerAliveInterval` và `ServerAliveCountMax`;
- optional interactive-terminal forwarding rules:
  - Local `-L`;
  - Remote `-R`;
  - Dynamic SOCKS `-D`.

TaskDeck dùng system OpenSSH client. Không có SSH library riêng và không tắt host-key verification.

Generic forwarding trong SSH profile chỉ được inject vào **interactive SSH terminal**. SSH Test chỉ kiểm tra login/host identity; SFTP và database tunnel không kế thừa các rule `-L/-R/-D` này để tránh vô tình mở port khi chỉ transfer file hoặc mở DB tunnel. TaskDeck thêm `ExitOnForwardFailure=yes` khi interactive profile có forwarding.

Host-key policy:

- agent/private key không có stored secret: `StrictHostKeyChecking=ask`;
- password hoặc encrypted private-key passphrase dùng one-time askpass: `StrictHostKeyChecking=accept-new`;
- changed host key vẫn bị OpenSSH từ chối.

Nếu OpenSSH báo changed/verification failure, TaskDeck có recovery dialog an toàn:

1. lấy effective `UserKnownHostsFile` bằng `ssh -G`;
2. inspect đúng host/port entry bằng `ssh-keygen -F`;
3. hiển thị lookup/fingerprint mà OpenSSH báo và matching file;
4. chỉ cho phép **Remove old known_hosts entry** sau explicit confirmation;
5. không reconnect tự động và không tự trust key mới.

TaskDeck không dùng `StrictHostKeyChecking=no`. Sau khi xóa old entry, người dùng phải reconnect và tự xác minh replacement host key qua trusted channel.

Password/passphrase không được đưa vào argv. Stored secret được lấy từ encrypted secret store và cấp cho OpenSSH qua one-time Unix-socket askpass ticket.

## Remote Workspace

**Connections → Remote** ghép các primitive hiện có thành một remote project workspace:

- một SSH profile;
- một SFTP profile bắt buộc tham chiếu đúng SSH profile đó;
- một `RemoteRoot` tuyệt đối trên remote host;
- zero hoặc nhiều database profile liên quan; DB dùng SSH tunnel phải tham chiếu cùng SSH profile.

Explorer chỉ gửi path **relative** so với `RemoteRoot`. Server normalize path và canonicalize `RemoteRoot` + target bằng `realpath` trên remote host trước list/read/write, nên `..` và symlink trỏ ra ngoài root bị từ chối. Text editor dùng cùng Editor của project local, nhưng IO đi qua SFTP và Save bắt `expected_sha256`; remote file đổi sau khi load sẽ vào conflict flow thay vì silently overwrite.

Từ cùng panel có thể:

- mở terminal SSH tại đúng folder đang duyệt bằng `remote_cwd`;
- chạy Git status/branches/log/diff/fetch/pull/push tại `RemoteRoot`;
- mở database profile/tunnel đã liên kết;
- duyệt file/folder và mở text file trong Editor.

Remote editor được tách khỏi local File Watcher, host LSP, Local History và workspace snapshot để `remote://...` không bị xử lý như project-local path. Shared-server runtime cần `settings.read + ssh.use`, sau đó từng operation vẫn cần permission riêng như `files.read/write`, `transfer.read/upload`, `terminal.create`, `git.*`. Profile create/edit/delete dùng `settings.write`.


## SFTP profiles

Trong **Connections → SFTP**, profile SFTP không lưu lại host/user/key/password riêng. Nó chỉ tham chiếu một SSH profile hiện có và thêm **Initial remote path**.

Nhờ vậy SFTP tự kế thừa từ SSH profile:

- host/port/user;
- SSH agent, private key hoặc password auth;
- private-key passphrase/password đã lưu trong encrypted secret store;
- ProxyJump;
- host-key verification policy;
- connect timeout.

TaskDeck dùng **system OpenSSH `sftp`**; không có SFTP Go package và không có npm package. Nếu máy chưa có executable `sftp`, Test/Open sẽ báo lỗi runtime rõ ràng.

SFTP không chạy preset command hoặc custom terminal bootstrap của SSH terminal. Các file operation được gửi dưới dạng command SFTP có quote/validation riêng, không qua remote shell.

## FTP profiles

Trong **Connections → FTP**, profile gồm:

- Name;
- Host;
- Port, mặc định `21`;
- Username;
- Password;
- Initial remote path;
- Connect timeout;
- transport security:
  - Plain FTP;
  - Explicit TLS / FTPES;
  - Implicit FTPS;
- TLS certificate verification policy.

FTP/FTPS client được viết bằng **Go standard library**, không dùng third-party FTP package. Nó dùng binary mode `TYPE I`, ưu tiên `EPSV`, fallback `PASV` trên IPv4 và ưu tiên `MLSD` trước khi fallback `LIST`.

FTPS mặc định xác minh TLS certificate/hostname. TaskDeck không tự tắt certificate verification. Plain FTP vẫn được hỗ trợ cho hệ thống cũ nhưng username/password và file content không được mã hóa trên đường truyền; nếu server hỗ trợ SFTP hoặc verified FTPS thì nên dùng transport mã hóa.

FTP password được lưu qua cùng encrypted secret store của SSH/database. Browser chỉ nhận `has_secret`, không nhận plaintext hoặc `secret_ref`.

## Dual-pane file workspace

Click một SFTP/FTP profile sẽ mở workspace hai pane kiểu FileZilla:

- **pane trái**: chọn nhanh giữa **Host** và **Local browser**;
- **pane phải**: filesystem của FTP/SFTP profile đang mở;
- divider ở giữa có thể kéo để đổi tỷ lệ trái/phải, double-click để reset 50/50;
- nút **→ / ←** truyền item đang chọn sang pane đối diện;
- folder upload/download được duyệt đệ quy; empty folder vẫn được tạo;
- double-click folder để đi vào; double-click file để truyền sang bên kia.

**Host** là filesystem của máy chạy TaskDeck nhưng vẫn bị giới hạn trong workspace hiện tại. Transfer Host ↔ Remote chạy trực tiếp server-side, không phải tải file vòng qua browser.

**Local browser** dùng File System Access API của browser. Người dùng phải chọn/grant một folder local; TaskDeck lưu directory handle bằng IndexedDB của browser để chuyển nhanh giữa các local root đã cấp quyền. Browser không cho web app tự ý truy cập đường dẫn tuyệt đối ngoài các folder đã được người dùng cấp quyền.

Mỗi pane có path input cùng danh sách xổ xuống **Favorites / Recent**:

- Host: nhớ riêng theo workspace;
- Local: nhớ riêng theo từng local root đã cấp quyền;
- Remote: nhớ riêng theo từng FTP/SFTP profile.

Nút **☆ / ★** pin hoặc bỏ pin path hiện tại; Recent được cập nhật tự động sau khi browse thành công.

Danh sách file có các header **Name / Type / Size / Modified** có thể click để đổi sort. Mặc định sort theo **Type tăng dần**, nên folder nằm trước file; khi sort theo Name/Size/Modified, folder vẫn được giữ thành nhóm trước file.

### Multi-select và context menu

Cả hai pane hỗ trợ chọn nhiều item:

- click thường: chọn một item;
- **Ctrl/Cmd-click**: thêm/bỏ từng item;
- **Shift-click**: chọn một dải theo thứ tự đang hiển thị;
- checkbox từng dòng: thêm/bỏ item;
- checkbox ở header: chọn/bỏ toàn bộ danh sách hiện tại;
- **Ctrl/Cmd+A** khi focus file list: chọn toàn bộ;
- **Esc**: bỏ selection.

Right-click một item chưa chọn sẽ chọn item đó rồi mở menu. Right-click một item đã nằm trong selection nhiều item sẽ giữ nguyên selection và menu sẽ thao tác trên cả nhóm.

Menu được tạo theo pane, loại item và số lượng selection:

- **Host / Local**: Open Folder, **Upload to remote FTP/SFTP** cho cả file/folder/mixed selection, Rename (single item), Delete selected items, New Folder, Copy Name/Path, Select All/Clear Selection, Refresh; Host file còn có Download to browser.
- **Remote FTP/SFTP**: Open Folder, **Download to left** cho cả file/folder/mixed selection, Download to browser (single file), Rename (single item), Delete selected items, New Remote Folder, Copy Name/Path, Select All/Clear Selection, Refresh.
- Folder delete ở **Host/Local** vẫn non-recursive; **Remote FTP/SFTP** dùng background scanner + Transfer Queue để delete đệ quy theo post-order.
- Local rename dùng native `FileSystemHandle.move()`; nếu browser không hỗ trợ API này TaskDeck sẽ báo rõ thay vì mô phỏng bằng copy/delete.

Các thao tác remote hiện có:

- browse path, Up, Refresh;
- upload/download;
- New Folder;
- rename;
- delete file;
- delete remote folder **recursive qua background scan + Transfer Queue**; child file/folder được enqueue trước, parent folder enqueue sau.

Double-click folder để đi vào. Double-click file để transfer qua pane đối diện.

Download tới browser vẫn dùng one-time ticket 90 giây để tương thích browser control lease mà không đưa remote path vào URL. FTP download stream trực tiếp; SFTP dùng private temporary file rồi cleanup. Upload tối đa 1 GiB theo safety boundary hiện tại.

Directory listing, FTP control reply và SFTP stdout/stderr đều bị bound để tránh output không giới hạn.

### Remote folder session cache

Remote listing được cache theo **FTP/SFTP profile + remote path** trong session của tab browser:

- đi lại folder đã mở dùng cache ngay, không gọi lại FTP/SFTP;
- nút **Refresh** luôn ép tải lại folder hiện tại;
- rename/delete/new-folder sẽ invalidate đúng listing bị thay đổi;
- upload chỉ invalidate các destination folder thực sự có thay đổi;
- khi queue kết thúc, chỉ destination folder đang hiển thị mới được reload ngay; các folder đã invalidate khác sẽ reload khi người dùng mở lại;
- cache là session-only, không ghi vào cấu hình lâu dài.

### Reload/session recovery

FTP/SFTP workspace state được lưu theo workspace để browser reload không làm mất tab đang mở:

- khôi phục các FTP/SFTP profile tab đang mở, tab đang active và left/remote current path;
- restore được phối hợp sau terminal restore để không bị terminal/task khác ghi đè snapshot trước khi file-transfer đọc lại;
- **Host workspace ↔ FTP/SFTP** và **remote delete** dùng queue/scanner/worker thuộc TaskDeck daemon. Reload browser không dừng job: browser mới chỉ reconnect vào `/api/file-transfer/jobs` và render lại snapshot `Queued / Running / Done / Failed`;
- trạng thái **Pause queue**, active scans, queue items và history Host/Remote do daemon giữ trong suốt vòng đời daemon; Pause/Resume/Resume selected/Remove selected được gửi qua server queue control API;
- remote delete chạy post-order ở daemon và idempotent: child được xóa trước parent; target đã bị xóa được coi là hoàn tất. Journal delete browser của các release cũ được migrate một lần sang daemon queue;
- **Local browser ↔ FTP/SFTP** là ngoại lệ do browser sandbox: daemon không thể đọc `FileSystemHandle`. TaskDeck lưu queue item + scan descriptor ở browser storage, khôi phục handle từ IndexedDB sau reload, đổi item đang `Running` thành `Queued`, scan lại phần cây còn dang dở và dedup item đã discover;
- nếu browser yêu cầu cấp lại quyền Local folder sau reload, queue/scanner vẫn được giữ. Bấm **Grant** rồi Retry/Resume để tiếp tục; item không bị xóa chỉ vì permission tạm thời chưa granted;
- server-side transfer queue có **durable journal**: TaskDeck ghi state atomic để daemon restart/self-update có thể khôi phục queue/scanner/history. Item đang Running được reconcile về trạng thái có thể resume/retry thay vì bị coi là hoàn tất;
- trước khi tiếp tục sau daemon restart, TaskDeck validate lại source/destination/profile context; không tự chạy mù job stale;
- upload/download dùng partial-transfer state và resumable primitive khi FTP/SFTP transport hỗ trợ; final destination chỉ được coi hoàn tất sau khi transfer hoàn thành.

### Transfer Queue

Bên dưới hai file panes có **Transfer Queue** dùng chung cho upload/download/delete.

Mỗi FTP/SFTP profile có **Max concurrent connections** riêng, mặc định **3** và có thể chỉnh từ **Connections → FTP/SFTP profile** hoặc trực tiếp ở header Transfer Queue. Đây là một budget chung của profile cho scanner, transfer và các remote operation của file workspace:

- mỗi scanner đang chạy giữ 1 slot;
- mỗi upload/download/delete worker đang chạy giữ 1 slot;
- browse/list/change directory, remote mutation, remote SHA-256/text compare và SFTP archive upload/extract cũng phải lấy slot trước khi mở remote connection;
- ví dụ `max=3`: 1 scan đang chạy chỉ còn tối đa 2 transfer; 2 scan chỉ còn 1 transfer; 3 scan thì transfer mới chờ;
- scan vượt budget giữ trạng thái **Scan queued** và chỉ được scheduler cấp slot khi có connection trống;
- khi usage đạt `max`, pane **Remote · FTP/SFTP** khóa path input, history, Go, Up, Refresh, double-click/Open folder; backend vẫn kiểm tra lại và trả `429` nếu có race;
- queue Local-browser có nhiều worker song song thay vì chạy tuần tự; nếu backend báo pool đầy, item quay lại **Queued · Waiting for an FTP/SFTP connection slot** thay vì bị đánh Failed;
- đổi limit có hiệu lực ngay và được lưu vào profile. Nếu giảm limit xuống thấp hơn số connection đang chạy, TaskDeck không cắt request đang chạy; scheduler chỉ ngừng cấp slot mới cho tới khi usage trở lại dưới limit.

- mỗi upload/download file và mỗi remote delete target là một queue item độc lập;
- trạng thái: **Queued / Running / Conflict / Done / Skipped / Failed**;
- có cột **Kind** để phân biệt **Upload / Download / Delete**, cùng direction, source, target, size và lỗi;
- lỗi một file không chặn các file độc lập phía sau;
- **Retry failed** đưa các item lỗi trở lại queue;
- **Clear done** xóa các item hoàn tất khỏi lịch sử session;
- right-click queue có **Pause queue / Resume queue**; pause chỉ dừng worker transfer/delete, scanner nền vẫn tiếp tục scan và enqueue;
- mép trên Transfer Queue có resize handle: kéo lên/xuống để đổi chiều cao; double-click reset về mặc định; chiều cao được lưu theo workspace + FTP/SFTP profile;
- queue rows hỗ trợ multi-select; context menu có **Resume selected** (có thể chạy selected item dù global queue vẫn paused) và **Remove selected**;
- operation đang Running không bị abort giữa request; nếu Remove selected trúng item đang chạy thì item được đánh dấu remove-after-run;
- Host upload/download/delete dùng hai pipeline bất đồng bộ trong daemon: **scanner producer** duyệt cây thư mục và **queue worker** thực thi item đã tìm thấy; browser chỉ hiển thị và điều khiển;
- scanner phát hiện item đến đâu thì enqueue đến đó, không cần giữ toàn bộ cây hoặc chờ quét xong mới xử lý; remote delete dùng post-order để parent `rmdir` luôn nằm sau child delete;
- mỗi FTP/SFTP profile có scheduler riêng dùng chung connection budget; scanner có thể chạy song song với nhiều transfer worker trong phần slot còn lại. Local-browser pipeline giữ cùng budget/parallel-worker semantics nhưng phần đọc/ghi FileSystemHandle phải chạy ở browser;
- queue lớn dùng pending cursor O(1), throttle render và giới hạn tối đa 2000 row trong DOM; logical queue vẫn giữ đầy đủ trạng thái của toàn bộ item;
- empty folder vẫn được tạo dù không có file queue item.

### Folder sync / mirror

File workspace có compare/sync workflow cho **Host hoặc Local browser ↔ Remote**:

- nút Folder Sync mở **Setup** trước khi scan; có thể dùng path hiện tại hoặc chọn một **Sync Profile** đã lưu theo workspace + FTP/SFTP profile;
- profile lưu left source/root/path, remote path, compare mode, default action, exclude globs và cờ cho phép delete; profile không chứa credential;
- exclude hỗ trợ glob kiểu `*`, `?`, `**` và được áp dụng ngay trong recursive scan, trước khi item đi vào dry-run plan;
- scan hai phía và phân loại **Left only / Remote only / Different / Same / Type mismatch**;
- compare mặc định dùng size + Modified time; chế độ **SHA-256** short-circuit khi size khác và hash cả hai file khi size bằng nhau;
- **dry-run plan luôn read-only**; file chỉ thay đổi sau khi người dùng bấm action trong preview;
- one-way Sync copy/update theo hướng đã chọn;
- **Bidirectional Sync** chỉ tự copy các path chỉ tồn tại ở một phía; file tồn tại ở cả hai phía nhưng khác nội dung/metadata được đánh dấu **Conflict** và không tự chọn bên thắng;
- Mirror chạy copy/update trước, rồi mới delete các item chỉ tồn tại ở destination; **delete mặc định OFF** và action Mirror bị khóa cho đến khi bật `Allow destination deletes`;
- trước khi Mirror delete vẫn có confirmation riêng; type mismatch và bidirectional conflict không bị delete tự động;
- execution tái sử dụng transfer queue/scanner/recovery hiện có cho upload/download/remote delete; Local browser vẫn giữ boundary của File System Access API và permission của browser.


### Duplicate file conflict policy

Khi upload/download gặp destination đã tồn tại:

- nếu **folder trùng folder**, TaskDeck dùng lại folder hiện có và tiếp tục recurse, không hiện confirmation;
- nếu path đích tồn tại nhưng type không tương thích (ví dụ source file nhưng destination là folder), operation fail rõ ràng thay vì ghi đè type khác;
- nếu **file trùng file**, item chuyển sang **Conflict** và dùng một conflict dialog chung cho Host ↔ Remote lẫn Local browser ↔ Remote;
- các lựa chọn gồm:
  - **Overwrite destination**;
  - **Skip source file**;
  - **Overwrite only if size differs**;
  - **Overwrite only if source Modified time is newer**;
  - **Overwrite only if SHA-256 differs**;
- FTP/SFTP không có creation-time metadata thống nhất và đáng tin cậy, vì vậy TaskDeck dùng **Modified time**, không gọi trường này là Created time;
- SHA-256 là checksum nội dung thật. Với remote/Local browser, kiểm tra này phải đọc toàn bộ source và destination nên chậm hơn so sánh size/Modified;
- scope của một quyết định:
  - **This file only** — chỉ item đang hỏi;
  - **This transfer only** — áp dụng cho toàn bộ conflict còn lại của job/scan hiện tại;
  - **All uploads/downloads in this TaskDeck session** — áp dụng theo chiều Upload hoặc Download cho session hiện tại;
  - **Always for uploads/downloads (remember)** — lưu default theo workspace + FTP/SFTP profile + direction để các lần sau dùng lại;
- Host ↔ Remote conflict thuộc daemon queue, nên trạng thái Conflict và quyết định job/session tiếp tục tồn tại qua browser reload trong vòng đời daemon;
- Local browser ↔ Remote dùng cùng policy names/semantics; conflict policy của scan/queue được journal cùng Local transfer state để reload không quay lại confirm kiểu cũ.

Không còn dùng confirmation rời rạc kiểu “file already exists, overwrite?” cho Local transfer; mọi file collision đi qua contract conflict ở trên.


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

### Query execution, cancel, transactions và explain

Query execution dùng adapter capability thay vì giả định mọi backend giống nhau.

- adapter có capability `cancel`: nút **Run** đổi thành **Cancel** trong lúc query chạy; cancel request không kill toàn adapter process;
- multi-statement dừng tại statement hiện tại khi Cancel;
- SQLite dùng `EXPLAIN QUERY PLAN`;
- MySQL/MariaDB dùng `EXPLAIN` và có `EXPLAIN ANALYZE` cho `SELECT/WITH` vì ANALYZE thực sự thực thi query;
- SQLite và MySQL/MariaDB hỗ trợ explicit **Begin / Commit / Rollback**.

Transaction là connection thật, không chỉ là label UI:

- SQLite giữ một Python `sqlite3.Connection` xuyên nhiều request;
- MySQL giữ một `mysql`/MariaDB CLI process và server connection xuyên transaction;
- MySQL query cancel trong transaction dùng `CONNECTION_ID()` + `KILL QUERY` từ control connection để transaction connection không bị kill;
- Data-grid/object actions bị gate khi explicit transaction đang active vì chúng có pipeline/connection riêng; trong transaction dùng Query editor cho SQL rồi Commit/Rollback;
- disconnect với transaction chưa hoàn tất cố gắng rollback.

Query workspace còn có persisted **Query History** và **Saved Snippets**. Relational structure metadata có foreign-key/reference information và UI navigation tới referenced relation khi adapter cung cấp metadata phù hợp.


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
├── file_transfer_profiles.json
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
- `file_transfer_profiles.json`;
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
5. edit affected SSH/FTP/database profiles and re-enter their passwords/passphrases.

Do not edit `secret_ref` values by hand. The UI/API rotates references atomically with secret storage.

## Offline install and self-update

TaskDeck's Go dependencies remain vendored through local `replace` directives. Installation runs:

```bash
GOPROXY=off GOSUMDB=off go test ./...
GOPROXY=off GOSUMDB=off go build ...
```

The installer also verifies the embedded SQLite helper source is present before build. Redis and FTP add no external package. SFTP uses the system OpenSSH `sftp` executable. MongoDB and SQLite dependencies are runtime executables (`mongosh` and Python 3), not build-time downloads.

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


## FTP/SFTP shared-server boundary

FTP/SFTP was implemented first against the existing legacy/single-user Connections model. Shared-server authorization remains fail-closed: new or unmapped API routes are denied until a deliberate RBAC mapping is added.

Do not weaken shared-server authorization merely to make the UI visible. A later shared-mode integration should explicitly map profile management, remote read/download, upload/write and connection-test actions to documented permissions and bind any one-time download token to the authenticated project/user context.
