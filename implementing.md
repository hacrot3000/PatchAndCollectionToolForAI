Tôi đã rà lại `main` hiện tại tại HEAD `17f9e1af`, gồm README, `CONNECTIONS.md`, Database Workbench handoff, Git panel/recovery backend, terminal/session broker, database UI/adapters, file-transfer queue và SSH profile/runtime. Nhìn tổng thể, **không còn thiếu chức năng nền tảng lớn**; TaskDeck hiện đã khá đầy đủ. Phần đáng đầu tư tiếp theo là độ sâu thao tác, khả năng phục hồi và UX cho các operation dài.

| Nhóm | Hiện tại đã khá đầy đủ | Phần tôi đánh giá còn nên bổ sung | Ưu tiên |
|---|---|---|---|
| **Git** | Multi-repo, Changes, stage/unstage, smart ignore, commit, branch, Merge From/To, conflict wizard, stash, log, compare, ahead/behind, push/pull/fetch, recovery wizard, large-file preflight | **Partial/hunk staging**, Git LFS workflow, streaming progress + Cancel cho push/fetch/merge dài, cherry-pick/revert/tag, branch delete/cleanup, file history/blame | **Rất cao** |
| **Terminal** | PTY thật, reconnect/replay, split, drag split, restore layout, rename/color/group, Broadcast, preset commands, mobile UI, CWD, path detect/View/Preview | **Terminate/Kill ngoài SIGINT**, shell integration OSC 7/133, detached-session manager, process/resource inspector, paste safety cho Broadcast, configurable scrollback | **Cao** |
| **Database** | MySQL/SQLite/Mongo/Redis, SSH tunnel, paging, edit grid, filter/sort, autocomplete, multi-query tabs, multi-result, import SQL lớn, export/copy, schema/structure | **Transaction session**, EXPLAIN/ANALYZE, query history/snippets, query cancel, FK navigation/schema relationship, bulk CSV/JSON import, streaming full-table export | **Rất cao** |
| **FTP/SFTP** | Dual pane, multi-select, recursive transfer/delete, daemon queue, retry/pause, conflict policy, checksum comparison, Favorites/Recent, browser-local support | **Durable queue qua daemon restart**, resumable partial transfer, FTPS, sync/mirror with dry-run, concurrency/bandwidth limits, post-transfer verification | **Rất cao** |
| **SSH** | Agent/key/password, ProxyJump, secret store, host-key verification, custom home, preset command, terminal integration, SFTP reuse | **Keepalive**, generic Local/Remote/Dynamic forwarding, reconnect workflow, known-host management, optional connection multiplexing, stronger connection diagnostics | **Cao** |

### Git

Git panel vừa được cải tiến khá mạnh nên phần còn thiếu rõ nhất bây giờ là **partial staging**. Hiện Stage hoạt động theo cả path. Với source file lớn, người dùng thường chỉ muốn commit vài hunk. Tôi đề xuất khi mở Diff có checkbox từng hunk và từng line, sau đó backend tạo patch có kiểm tra context rồi dùng `git apply --cached --check` trước khi stage. Cần có `Stage hunk`, `Unstage hunk`, `Discard hunk`; riêng Discard phải confirmation.

Phần thứ hai là **Git operation job model**. Hiện các lệnh Git chạy synchronous qua HTTP và chỉ trả output khi process kết thúc. Sau khi push timeout được nâng lên 10 phút, một push 2–3 GB có thể trông giống UI bị treo. Nên chuyển push/fetch/pull/merge dài sang job có `Running`, live stdout/stderr, elapsed time và **Cancel**. Backend phải cancel process group, không chỉ HTTP request.

Git LFS cũng nên làm tiếp từ large-file wizard vừa có. Hiện TaskDeck hỗ trợ bỏ file lớn khỏi commit nhưng chưa hỗ trợ migration sang LFS. Có thể thêm:

```text
Large file detected
logic/capture.csv — 637.58 MiB

[Remove + ignore]
[Migrate this file to Git LFS]
[Track *.csv with Git LFS]
```

Chỉ hiện LFS option nếu `git lfs version` thành công. `git lfs migrate import` rewrite history nên phải có preview commit range + explicit confirmation.

Các thao tác nâng cao tiếp theo đáng có là cherry-pick, revert, local/remote branch delete, tags và file history/blame. Recovery backend thực tế đã biết trạng thái cherry-pick/revert, nhưng chưa có workflow chủ động bắt đầu chúng trong Git panel.

### Terminal

Terminal hiện rất mạnh về layout/session persistence. Khoảng trống quan trọng nhất là **process lifecycle**. Hiện `Stop` chủ yếu gửi `SIGINT`; có process không phản ứng với Ctrl+C. Tôi đề xuất menu:

```text
Interrupt     Ctrl+C / SIGINT
Terminate     SIGTERM
Kill          SIGKILL
Process tree
```

Terminate/Kill phải tác động đúng process group của session và có confirmation cho Kill.

Nâng cấp lớn tiếp theo là **shell integration**. TaskDeck hiện lấy CWD và parse terminal theo cách riêng; nếu hỗ trợ OSC 7 + OSC 133 có thể biết chính xác command boundary, current directory, command exit status và prompt. Khi đó terminal có thể hiển thị từng command block kiểu:

```text
$ idf.py build                         ✓ 0  18.4s
$ git push                             ✗ 1  4.2s
```

Từ đó có thể click `Copy command`, `Rerun`, `Open CWD`, `Copy output of this command`, và path detection cũng chính xác hơn rất nhiều.

Một cải tiến UX nữa là **Detached sessions**. Hiện Close tab đang chạy không kill process và session có thể xuất hiện lại sau reload — behavior này đúng về an toàn nhưng dễ làm người dùng nhầm. Nên có panel riêng:

```text
Running
Detached
Exited
```

và đổi menu thành `Detach tab`, `Terminate session`, tránh từ “Close” quá mơ hồ.

Broadcast nên có thêm cảnh báo khi paste nhiều dòng hoặc lượng text lớn vào `All/Group`, vì một paste nhầm có thể chạy lệnh đồng thời trên nhiều server.

### Database

Database Workbench đã ở mức khá gần một DB client độc lập. Điểm kiến trúc còn thiếu đáng kể nhất là **transaction thực sự**. Tài liệu hiện tại ghi rõ mutation batch là per-item và có thể partial success, không phải một transaction.

Nên bổ sung một chế độ explicit:

```text
Auto commit: ON

hoặc

Transaction
[Begin]
[Commit]
[Rollback]
```

Điều này đòi hỏi một DB connection/session sống xuyên nhiều request; không thể chỉ spawn CLI độc lập cho mỗi statement. Vì vậy đây là feature lớn nhưng rất đáng làm cho MySQL/SQLite trước.

Tiếp theo là **EXPLAIN / EXPLAIN ANALYZE**. Trong Query tab nên có:

```text
Run
Explain
Explain Analyze
```

và render plan tree/table thay vì raw text. Với MySQL đây là công cụ debug query rất hữu ích.

Tôi cũng khuyên thêm **Query History + Saved Snippets**. Hiện nhiều Query tab/persistence giải quyết workspace, nhưng chưa thấy một history dạng:

```text
Today
08:12 SELECT ...
08:10 UPDATE ...
Yesterday
...
```

có duration, affected rows, success/failure và `Open in new query`.

Workbench cũng nên hiểu foreign key để từ một cell `user_id=123` có thể `Go to referenced row`, và từ Structure có tab `Foreign Keys / References`.

Bulk import CSV/JSON với mapping column + preview + validation cũng là phần còn thiếu hợp lý. Table export hiện đã có, nhưng với table rất lớn nên chuyển sang streaming server-side thay vì materialize toàn dataset trong browser.

### FTP / SFTP

Đây là nhóm tôi thấy có **một gap phục hồi rất rõ trong chính tài liệu hiện tại**: server-side queue sống qua browser reload nhưng **không durable qua TaskDeck daemon restart**.

Đây nên là ưu tiên cao nhất của file-transfer. Queue/scanner state nên có journal atomic trên disk:

```text
job
scan cursor
source
destination
conflict policy
completed entries
pending entries
```

Sau restart, TaskDeck phải validate source/destination trước khi Resume, không tự chạy mù.

Thứ hai là **resume file transfer**. Hiện retry một file lớn về cơ bản phải transfer lại. Nên dùng temporary `.part` và resume khi protocol hỗ trợ. Với SFTP có thể seek/resume; FTP có `REST` nếu server hỗ trợ. Chỉ rename `.part` thành final khi transfer hoàn tất.

FTP hiện được document rõ là plaintext. Tôi đề xuất thêm **FTPES/FTPS** thay vì chỉ cảnh báo. Go stdlib đã có `crypto/tls`, nên không nhất thiết phải thêm dependency ngoài. Profile có thể chọn:

```text
Plain FTP
Explicit TLS / FTPES
Implicit FTPS
```

TLS certificate verification mặc định phải bật.

Sau đó có thể thêm **Folder Sync/Mirror** rất hữu dụng:

```text
Compare Local ↔ Remote
Only local
Only remote
Different
Same

[Dry run]
[Sync →]
[← Sync]
```

Bắt buộc preview trước delete/mirror.

### SSH

SSH hiện dùng system OpenSSH là lựa chọn tốt; tôi không khuyên viết SSH client riêng.

Điểm nên thêm đầu tiên là profile-level:

```text
ServerAliveInterval
ServerAliveCountMax
TCPKeepAlive
ConnectTimeout
```

để các terminal chạy qua mạng/VPN ổn định hơn.

Hiện generic SSH profile có ProxyJump, nhưng tôi chưa thấy UI dành cho **general forwarding**. Nên bổ sung:

```text
Local forward    -L
Remote forward   -R
Dynamic SOCKS    -D
```

Database tunnel vẫn giữ subsystem riêng vì có lifecycle chặt chẽ, còn forwarding UI phục vụ developer use-case nói chung.

Host-key UX cũng có thể tốt hơn. Khi changed host key, thay vì chỉ hiện OpenSSH error, wizard nên hiển thị host/port, fingerprint cũ/mới nếu lấy được và các lựa chọn an toàn như `Open known_hosts location` hoặc `Remove matching old host-key entry` với confirmation. Không được tự trust key mới.

SSH reconnect cũng nên là **explicit reconnect**, không tự động chạy lại shell state. Khi SSH child disconnect, tab nên có `Reconnect profile` để mở SSH session mới nhưng giữ tab metadata/color/group; cần báo rõ remote CWD/process cũ không được khôi phục.

### Thứ tự tôi đề xuất triển khai

1. **Git operation jobs + live progress + Cancel**, đồng thời làm partial/hunk staging.
2. **FTP/SFTP durable queue qua daemon restart + resume `.part`**, vì đây là gap phục hồi rõ nhất hiện tại.
3. **Terminal Interrupt / Terminate / Kill + process tree**, sau đó OSC 7/133 shell integration.
4. **Database transaction session + query cancel + EXPLAIN**, vì đây là phần nâng Workbench từ “data editor tốt” lên “DB client đầy đủ”.
5. **Git LFS migration wizard**, tận dụng luôn large-file detector vừa có.
6. **SSH keepalive + forwarding + host-key recovery UX**.
7. **FTPS + folder sync/mirror dry-run**.
8. Sau đó mới đến các feature tiện ích như Git blame/tags, DB schema relationship, query history/snippets và terminal resource monitor.

Nếu chỉ chọn **3 việc có tỷ lệ lợi ích/công sức tốt nhất**, tôi sẽ chọn **Git live job/progress + cancel**, **Terminal terminate/kill/process tree**, và **FTP/SFTP durable queue + resume**. Ba phần này giải quyết các tình huống thực tế mà một IDE/web console thường dễ gây mất thời gian hoặc không rõ đang xảy ra chuyện gì, trong khi không đòi thay đổi toàn bộ kiến trúc như DB transaction session.
