# TaskDeck (Go)

`vscode_tasks_menu_go` là backend Go cho executable ở root:

```bash
./vscode_tasks_menu
```

Công cụ luôn lấy workspace từ **thư mục hiện tại** khi gọi executable và đọc động:

```text
<workspace>/.vscode/tasks.json
```

Vì vậy thay đổi `tasks.json` sẽ đổi menu ngay, không cần build lại Go. Nút **Reload tasks.json** trên web và phím `r` trong terminal mode sẽ đọc lại file.

## TaskDeck global + Patch add-on

Đường cài đặt chính hiện tại là global user app. Từ root repository:

```bash
./install.sh
```

Installer tạo một release versioned chứa cả TaskDeck và Python Patch Tool:

```text
~/.local/bin/taskdeck
~/.local/lib/taskdeck/current -> releases/<revision>/
├── taskdeck
└── patchtool/
    ├── python_patch_entry.py
    ├── run_python_patches.sh
    └── _patch_lib/
```

Từ project cần làm việc:

```bash
cd PROJECT_ROOT
taskdeck
```

Web UI có panel **Patch** native cho Queue/Failed, Running/progress/artifacts, Resume, History, Plan và Health. Python vẫn sở hữu policy/business logic; TaskDeck chỉ dùng protocol có cấu trúc và giữ PTY làm evidence/fallback.

CLI tương đương được giữ như first-class subcommand:

```bash
cd PROJECT_ROOT
taskdeck patch
taskdeck patch plan
taskdeck patch report
```

Project-local Patch data vẫn ở project. Sau migration, runtime Python không cần nằm trong `tools/`; launcher legacy đã xác minh có thể được thay bằng shim chuyển tiếp sang `taskdeck patch`. Root launcher `./vscode_tasks_menu` vẫn được giữ làm bootstrap/compatibility entry và sẽ cài/chuyển tiếp sang global TaskDeck khi cần.

## Cài đặt một file / self-install

Có thể cài mới chỉ bằng **một file** root `vscode_tasks_menu`. Không cần tải sẵn thư mục `vscode_tasks_menu_go`.

Ví dụ:

```bash
chmod +x ./vscode_tasks_menu
./vscode_tasks_menu
```

Nếu launcher phát hiện source Go bắt buộc còn thiếu, nó tự:

1. lấy SHA mới nhất của nhánh `main` từ GitHub;
2. tải source archive đúng SHA đó;
3. chỉ cài/bổ sung `vscode_tasks_menu_go` cạnh launcher;
4. build `vscode_tasks_menu_go/.build/vscode_tasks_menu`;
5. nhúng đúng source revision vào binary và ghi revision marker;
6. tiếp tục chạy tool với workspace là thư mục hiện tại.

Yêu cầu bootstrap: `bash`, Go toolchain, `tar`, và một trong `curl` hoặc `wget`.

Source được tải qua HTTPS từ GitHub. Archive được staging trong thư mục tạm cạnh launcher, giới hạn 128 MiB, kiểm tra các file bắt buộc trước khi cài. Nếu `vscode_tasks_menu_go` đã tồn tại nhưng thiếu file, self-install chỉ bổ sung file còn thiếu và **không ghi đè source/local changes đang có**.

Sau khi đã cài xong, các lần chạy bình thường không truy cập mạng để self-install. Việc nâng cấp tiếp tục dùng:

```bash
./vscode_tasks_menu --self-update
```

## Chế độ web

```bash
./vscode_tasks_menu
```

Lần chạy đầu sẽ:

1. nếu thiếu source, tự self-install `vscode_tasks_menu_go` từ GitHub;
2. build Go binary vào `vscode_tasks_menu_go/.build/` nếu cần;
3. khởi động daemon nền;
4. bind port đã cấu hình (`0` = hệ điều hành chọn port trống);
5. in URL;
6. tự mở browser nếu `open_browser = true`.

Nếu daemon của cùng workspace đã chạy, launcher chỉ in URL và mở browser, không tạo daemon thứ hai.

Mỗi lần bấm một task sẽ tạo một **PTY session riêng** và một tab terminal riêng trên web. Task nhận TTY thật nên các workflow dùng `input()`, ANSI/curses, phím mũi tên, Ctrl+C và các prompt tương tác tiếp tục hoạt động như terminal.

### Mở terminal nhanh

Trên web UI, nút **Terminal** ở header là direct action: click một lần sẽ tạo terminal local mới ngay, không còn mở menu dropdown. CWD dùng cho terminal mới vẫn được cấu hình riêng tại **Settings → Terminal**.

Hotkey global:

```text
Ctrl+Shift+`
```

Hotkey gọi cùng `startTerminal()` như nút header, vì vậy vẫn đi qua permission `terminal.create`, browser lease và cơ chế CWD injection hiện có. Giữ phím không tạo lặp terminal vì keyboard repeat được bỏ qua. Trên macOS, listener cũng chấp nhận `Cmd+Shift+`` theo primary-modifier convention của các shortcut web khác trong TaskDeck.

### Git Quick Actions / multi-repository workspace

Git panel không còn giả định workspace root cũng là Git root. TaskDeck có thể quản lý nhiều repository độc lập nằm trong cùng workspace, kể cả trường hợp workspace root là một repo và các thư mục con lại là repo riêng.

- backend quét bounded từ workspace root, mặc định sâu 4 cấp và bỏ qua các cây nặng như `.git`, `node_modules`, `vendor`, `build`, `dist`, `target`, `Library`, `Temp`;
- repo được xác minh bằng `git rev-parse --show-toplevel`, hỗ trợ `.git` dạng directory hoặc file (worktree/submodule);
- browser chỉ gửi `repo_id` tương đối đã có trong registry; backend không nhận arbitrary filesystem path cho Git action;
- Git panel có selector **Repository**, view **Repositories**, trạng thái tổng hợp branch/HEAD/changed/ahead/behind, và nút **↻ Scan**;
- toàn bộ Changes/Branches/Log/Stash/Compare/Fetch/Pull/Push/Commit/Merge chạy trong repo active;
- lựa chọn repo được lưu theo workspace;
- có thể bật auto-select repo theo CWD của terminal bằng cấu hình, mặc định tắt để tránh tự đổi repo ngoài ý muốn.

Cấu hình optional trong `.vscode/vscode_tasks_menu.ini`:

```ini
[git]
scan_enabled = true
scan_depth = 4
default_repository = projects/m3-client
auto_select_from_terminal_cwd = false

[git.repositories]
M3 Client = projects/m3-client
M3 Server = projects/m3-server
```

Repo khai báo trong `[git.repositories]` được ưu tiên đặt tên; auto-scan vẫn bổ sung các repo khác. `default_repository = .` chọn repo ở workspace root.

Git workflow nâng cao hiện có:

- các operation dài `fetch / pull / push / merge` và xóa branch thật trên remote chạy dưới dạng cancellable job, hiển thị stdout/stderr/progress trực tiếp trong Git panel và có **Cancel**;
- **Branches** phân biệt rõ phạm vi xóa:
  - **Delete local** → `git branch -d <branch>`: chỉ xóa local branch và Git từ chối nếu chưa merge an toàn;
  - **Force delete local** → `git branch -D <branch>`: chỉ xóa local branch nhưng cho phép bỏ branch chưa merge, vì vậy UI cảnh báo nguy cơ mất commit;
  - **Forget local ref** → `git branch -dr <remote>/<branch>`: chỉ xóa remote-tracking ref trong máy hiện tại; branch trên server không đổi và lần fetch sau có thể tạo ref lại;
  - **Delete on remote / Delete upstream remote** → `git push <remote> --delete <branch>`: xóa branch thật trên remote server nhưng không xóa local branch cùng tên;
- Diff dùng **visual side-by-side** theo hunk, có line number và highlight added/removed; raw unified patch vẫn có thể mở bên dưới để đối chiếu chính xác output Git;
- Diff biểu diễn đúng ba lớp state của Git theo chuỗi **HEAD (committed) → Index (staged) → Working tree (not staged)**:
  - **HEAD ↔ Staged** chỉ xem phần đã stage, đồng thời hỗ trợ **Unstage hunk**;
  - **Staged ↔ Working** chỉ xem phần sửa tiếp nhưng chưa stage, đồng thời hỗ trợ **Stage hunk / Discard hunk**;
  - **HEAD ↔ Working** xem tổng thay đổi tracked từ commit hiện tại đến file đang làm việc, là view read-only để tránh áp dụng hunk sai layer;
- khi một file đồng thời có staged và unstaged changes, hàng **Changes** hiện cả ba nút diff trên để có thể kiểm tra riêng từng lớp thay vì trộn chúng vào một patch;
- merge/rebase/cherry-pick/revert conflict mở recovery wizard theo file, hỗ trợ Current/Incoming/Mark resolved/Continue/Abort;
- large-file preflight phát hiện blob vượt giới hạn GitHub trước khi upload; nếu `git lfs` có sẵn, wizard có thể migrate exact file hoặc pattern vào Git LFS trên phần history chưa push;
- có file history + blame, tag management, cherry-pick và revert;
- recovery wizard giữ classification riêng cho network/auth/timeout/conflict/large-file/host-side rejection để không chỉ hiện raw Git error.

### Lifecycle của tab/session

- Reload browser hoặc đóng/mở lại browser: task đang chạy **không bị kill**; tab được phục hồi từ daemon và log PTY được replay.
- Task/terminal được sở hữu bởi **session broker process độc lập** với web daemon. Daemon chỉ proxy API/WebSocket tới broker qua Unix socket local.
- Khi self-update thay web daemon, broker tiếp tục giữ process group, PTY, session ID và scrollback; daemon mới reconnect lại các session hiện hữu thay vì chạy lại task.
- **Close** một tab đang chạy: chỉ đóng UI, process vẫn chạy và sẽ xuất hiện lại sau reload/mở lại web.
- **Stop**: gửi `SIGINT` cho process group của task.
- menu process control còn có **Terminate** (`SIGTERM`) và **Kill** (`SIGKILL`) cho cả process group, cùng **Process tree** và resource monitor để xem PID/CPU/RSS của process con.
- local Bash terminal dùng TaskDeck-generated rc wrapper để phát **OSC 7 / OSC 133** mà không sửa `~/.bashrc`; browser nhờ đó biết semantic CWD, command boundary và exit status.
- terminal header có **Commands** history với command/output/exit code/duration/CWD, hỗ trợ Copy command, Copy output và Rerun; có thể thêm **Session note**, bookmark một dòng trong output và export toàn session thành Markdown hoặc text.
- command history được lưu bounded/atomic tại `.vscode/vscode_tasks_menu.terminal_history.json` với quyền file `0600`; tối đa 64 session, 200 command/session, command/output/note đều có size cap. Khi terminal được recreate sau daemon/broker restart, history được remap từ session ID cũ sang ID mới. Trong shared-server, GET history dùng quyền view của đúng terminal và PUT dùng quyền control của đúng terminal, không dùng quyền settings project-wide.
- Close tab đã `exited/stopped`: session đã hoàn tất được xóa khỏi daemon; history của session đó cũng được cleanup.

- Daemon giữ tối đa khoảng 4 MiB scrollback cho mỗi session để reconnect/replay.

### Một browser điều khiển tại một thời điểm

Web UI dùng **exclusive browser lease**. Mỗi lần một browser/tab mới tải trang hoặc reload:

1. browser mới nhận lease điều khiển mới;
2. lease cũ bị revoke ngay;
3. WebSocket PTY của browser cũ bị đóng và mọi request thay đổi trạng thái bằng lease cũ bị từ chối;
4. browser cũ hiện overlay **Control moved to another browser** với nút **Reload and take control**.

Browser reload sau cùng luôn giành quyền điều khiển. Việc revoke browser **không dừng task/terminal** trong session broker; browser mới reconnect vào các session vẫn đang chạy.

Các control action nội bộ của self-update (`handoff`/`detach`) từ loopback được phép đi qua lease để daemon vẫn có thể tự thay thế/restart. Các action browser như confirm/cancel vẫn yêu cầu lease hiện tại.

### Mobile và desktop

Layout được xác định một lần khi trang tải:

- `mobile`: viewport hẹp hoặc coarse pointer;
- `desktop`: các trường hợp còn lại.

Mobile dùng giao diện riêng:

- task menu dạng drawer với nút `☰`;
- header action dạng menu `⋮`;
- tab cuộn ngang bằng touch;
- touch target lớn hơn;
- hỗ trợ `VisualViewport` và safe-area để giảm lỗi khi bàn phím ảo mở;
- mỗi terminal có thanh phím nhanh: `Esc`, `Tab`, `Ctrl+C`, `←`, `↑`, `↓`, `→`, `Enter`;
- **không bật terminal split trên mobile**.

State trình bày được tách theo profile. Desktop giữ file terminal layout cũ:

```text
vscode_tasks_menu.terminals.json
```

Mobile dùng:

```text
vscode_tasks_menu.terminals.mobile.json
```

Backend không nhận/lưu split trong profile mobile. Vì vậy mở mobile, thao tác tab rồi quay lại desktop không được xóa split desktop. Tab order và appearance (theme/font/font-size) cũng được namespace theo profile. Preference appearance desktop từ key cũ được migrate một chiều sang key desktop khi nâng cấp, nên không mất cấu hình cũ. Sidebar resize chỉ chạy ở desktop.

Nếu mobile tạo thêm terminal trong lúc takeover, desktop sẽ giữ lại order/split của các terminal cũ còn sống và append terminal mới. Split chỉ mất khi một terminal thuộc split thực sự không còn tồn tại.

### Broadcast Groups

Web UI có menu **Broadcast** ở bên trái **Terminal** với ba mode:

- **None**: chỉ tab nguồn nhận input như bình thường;
- **All**: mỗi raw key/input từ tab nguồn được gửi thêm tới mọi session đang `running`;
- **Group**: raw key/input chỉ được gửi thêm tới các session cùng broadcast group với tab nguồn; nếu tab nguồn không thuộc group nào thì không fan-out.

Broadcast giữ nguyên dữ liệu từ `xterm.onData()`, vì vậy ký tự thường, Enter, Ctrl+C, phím mũi tên và paste đều đi qua cùng cơ chế. Session nguồn vẫn dùng WebSocket input cũ; fan-out luôn loại source ID để tránh nhận một phím hai lần. Request broadcast của từng source được queue theo thứ tự để giảm nguy cơ reorder khi gõ nhanh.

Khi Broadcast đang bật, paste nhiều dòng hoặc payload lớn sẽ yêu cầu xác nhận trước khi fan-out để giảm rủi ro chạy nhầm lệnh đồng thời trên nhiều terminal.

Click phải một tab mở thêm phần **Broadcast group**:

- chọn một group đã có để assign tab;
- **Create new group…** để tạo group và assign ngay tab hiện tại;
- **Remove from group** để bỏ tab khỏi group.

Mỗi group chọn một trong các preset màu nền/chữ có độ tương phản cao: Slate, Ocean, Forest, Amber, Violet, Rose, Cyan và Lime. Tab thuộc group được tô theo preset đó để dễ nhận biết. Có thể edit/delete group từ menu Broadcast; xóa group sẽ tự bỏ assignment của các tab thuộc group.

Mỗi Broadcast group còn có thể lưu một environment dạng `NAME=VALUE`. Menu **NEW TERMINAL IN GROUP** tạo local terminal mới với environment của group rồi assign terminal đó vào group ngay sau khi session được tạo. Environment explicit của group được merge sau environment profile đang chọn nên giá trị group có precedence; terminal đang chạy không bị mutate environment giữa chừng.

Mode, group definitions, preset màu, environment và mapping `session_id -> group_id` được lưu atomic tại `.vscode/vscode_tasks_menu.broadcast.json`; runtime state legacy được tự migrate nhưng vẫn giữ bản cũ để rollback tức thời. Vì vậy cấu hình Broadcast sống qua browser reload, daemon restart, self-update và reboot, đồng thời assignment được remap khi terminal session ID cần recreate.

### Authentication mode migration wizard

Trong **Settings → Security → Authentication mode…**, TaskDeck có wizard 3 bước `Mode → Setup → Review` để chuyển hai chiều giữa single authentication và multi-user shared-server mà không cần sửa INI thủ công.

- **Single → Shared:** wizard chọn project ID, identity DB và admin. Nếu Basic Auth hiện tại đạt shared password policy, có thể reuse credential mà không gửi ngược plaintext hiện tại ra UI; backend hash vào identity DB, grant `system:admin`, bật HTTPS, tắt Basic Auth và scrub password legacy khỏi config. Username global đã tồn tại phải verify đúng password trước khi được thêm vào project.
- **Shared → Single:** project admin phải đặt credential Basic Auth mới; shared hash không bao giờ được giải mã. Trước khi chuyển, TaskDeck revoke toàn bộ browser session của project để cookie cũ không thể sống lại nếu sau này bật Shared trở lại.
- Identity DB, project, users và roles không bị xóa khi chuyển về Single. Các giá trị `project_id` / `identity_db` cũng được giữ trong config để lần chuyển lại Shared có thể reuse.
- Config được ghi atomic. Trên Linux/Unix hiện hỗ trợ broker-preserving `--reload-config`, daemon web tự reload sau wizard trong khi PTY/session broker tiếp tục chạy. Nền tảng chưa hỗ trợ reload an toàn sẽ báo cần restart thủ công thay vì âm thầm kill session.

### Preset command cho terminal

Preset command chỉ xuất hiện trên **terminal tab** (`task_id == 0`). Click phải tab terminal sẽ có:

```text
Preset command ▶
```

Submenu hiển thị từng preset theo dạng:

```text
Tên preset — đoạn đầu của command thứ nhất
```

Ví dụ:

```text
Auto commit — git add .
Deploy — ./scripts/deploy.sh
```

Chọn preset sẽ chạy các command **tuần tự trong chính shell terminal hiện tại**. Runner không spawn một shell riêng, vì vậy state như `cd` hoặc `export` của command có thể tiếp tục ảnh hưởng tới các command sau.

Sau mỗi command, runner lấy exit code của chính shell và chỉ gửi command kế tiếp khi exit code bằng `0`. Nếu command trả khác `0`, session đóng, hoặc kết nối terminal bị mất thì preset dừng ngay và không gửi các command còn lại.

Ví dụ:

```text
git add .
git commit "Auto commit all"
git push
git status
```

Nếu `git push` trả exit code khác `0`, `git status` sẽ không được gửi.

Runner hiện hỗ trợ terminal shell `bash/sh/zsh/dash/ksh/ash/fish`. Command được gửi trực tiếp qua WebSocket của terminal nguồn, không đi qua Broadcast All/Group, vì vậy chọn một preset không tự fan-out sang các tab khác.

Submenu có **Manage presets…** để mở dialog riêng. Dialog hỗ trợ:

- Add preset;
- đổi tên preset;
- Add/Remove từng command;
- mỗi command là một textarea riêng nên có thể chứa nhiều dòng;
- Save;
- Delete preset;
- Close;
- cảnh báo trước khi bỏ các thay đổi chưa Save.

Preset được lưu project-local tại:

```text
<workspace>/vscode_tasks_menu.presets.json
```

File được ghi atomic với quyền `0600`, nên preset vẫn còn sau browser reload, daemon restart, self-update và reboot miễn project/file còn tồn tại.

### Project profiles

TaskDeck hỗ trợ nhiều **Project profile** cho cùng một workspace, ví dụ `Development`, `Debug`, `Production` hoặc profile riêng theo khách hàng. Nút **Profile…** mở manager để tạo profile từ context đang dùng, Save/Apply/Delete profile và chuyển nhanh giữa các context.

Mỗi profile có thể lưu:

- snapshot biến môi trường đang chọn;
- tập command preset và task dùng cho project;
- các local terminal startup với CWD/title;
- reference tới SSH, database và FTP/SFTP profile;
- default Git repository đang chọn.

Apply profile **không tự chạy task hoặc command preset**, không tự switch Git branch và không copy password/private-key passphrase/database/FTP secret vào file profile. Connection profile chỉ được lưu bằng ID; secret tiếp tục nằm trong secret store hiện có.

Environment được snapshot vào project profile để profile vẫn dùng được nếu browser-local environment profile cũ bị xóa. Snapshot env chỉ được áp dụng cho **local task/terminal**; TaskDeck không inject local environment overrides vào SSH terminal remote.

Project profiles được lưu atomic với quyền `0600` tại:

```text
<workspace>/vscode_tasks_menu.project_profiles.json
```

### Download file xuất hiện trong output

Khi output của task in ra đường dẫn file, có thể dùng chuột **bôi chọn vùng text chứa path**. Web UI sẽ kiểm tra các path thật nằm trong vùng chọn:

- nếu có 1 file hợp lệ, nút **Download** xuất hiện trên thanh đầu của terminal;
- nếu có nhiều file, UI hiện thêm danh sách file và nút **Download (N)** để chọn file cần tải;
- thứ tự file giữ theo thứ tự xuất hiện trong output, vì vậy các block kiểu `PRIMARY / ZIP preferred` vẫn ưu tiên file được in trước;
- hỗ trợ cả path tuyệt đối dưới workspace và path tương đối tính từ workspace.

Backend chỉ cho tải **regular file nằm trong workspace hiện tại**. `..`, path tuyệt đối ra ngoài workspace và symlink trỏ ra ngoài workspace đều bị từ chối. File được trả về với `Content-Disposition: attachment` để browser tải xuống máy thay vì mở như trang web.

Ví dụ block sau có thể bôi chọn nguyên khối; UI sẽ nhận ra cả ZIP và TXT:

```text
ZIP (preferred) — copy path below:
/home/user/project/artifacts/ptv_to_ai/CR_57a48bda.zip
Clear-text TXT — copy path below:
/home/user/project/artifacts/ptv_to_ai/CR_57a48bda.txt
```

### Reload tasks.json an toàn

ID của task được tạo ổn định từ chính định nghĩa task, không còn phụ thuộc vị trí trong mảng `tasks`. Vì vậy nếu browser đang giữ menu cũ trong lúc `tasks.json` được chèn, xóa hoặc reorder, click cũ không thể trỏ nhầm sang task khác.

Nếu định nghĩa task đã thay đổi, ID cũng thay đổi; request từ menu cũ sẽ bị server từ chối với `task not found` và người dùng chỉ cần reload menu.

## Chế độ terminal native Go

```bash
./vscode_tasks_menu --terminal
```

Menu terminal được render trực tiếp bằng Go và cũng đọc `.vscode/tasks.json` động.

Phím chính:

```text
↑/↓ hoặc j/k   chọn
Enter           mở group / chạy task
r               reload tasks.json
Home/End        đầu / cuối danh sách
q hoặc Esc      quay lại / thoát
```

Task vẫn chạy nguyên command khai báo trong `tasks.json`; các task vốn gọi Python hoặc shell vẫn tiếp tục gọi chính script đó.

## Quản lý daemon

```bash
./vscode_tasks_menu --status
./vscode_tasks_menu --reload-config
./vscode_tasks_menu --stop-daemon
./vscode_tasks_menu --restart-daemon
```

`--reload-config` đọc và validate lại `<workspace>/vscode_tasks_menu.ini` trước khi tác động tới daemon. Nếu config hợp lệ và daemon đang chạy, lệnh chỉ restart web daemon bằng đường broker-preserving nên task/terminal hiện có tiếp tục chạy; thay đổi bind/port/TLS/auth được áp dụng đầy đủ. Nếu config không hợp lệ, daemon hiện tại không bị dừng. Nếu daemon chưa chạy, lệnh chỉ xác nhận config hợp lệ và không tự start daemon.

`--stop-daemon` và `--restart-daemon` là thao tác chủ động: daemon yêu cầu session broker shutdown, broker gửi interrupt/hangup tới các session đang chạy rồi force-kill process còn sót sau grace period. Vì vậy hai lệnh này vẫn giữ semantics cũ là dừng session.

Self-update dùng đường khác: daemon **detach nhưng không shutdown broker**, nên task/terminal có thể tiếp tục chạy xuyên qua daemon replacement.

### Session broker và self-update

Mỗi workspace có broker state/socket riêng trong runtime directory. Broker được daemon tự start nếu chưa có, hoặc reuse nếu đã sống. Browser không kết nối trực tiếp broker; toàn bộ API công khai vẫn đi qua web daemon như trước.

Trong self-update bình thường:

```text
task/terminal -> broker
                  |
old daemon --------+
   detach
new daemon --------+
                  |
browser reconnect
```

Daemon mới query broker sessions trước. Nếu broker còn session, nó giữ nguyên session ID và không recreate terminal. Core UI `syncSessions()` attach lại các tab; scrollback gồm cả output phát sinh trong khoảng daemon bị thay.

Nếu listener-FD handoff không dùng được, updater ưu tiên control action `detach` để daemon cũ thoát mà broker vẫn sống, rồi start daemon mới trên address cũ. Legacy SIGTERM chỉ là last resort.

**Giới hạn chuyển tiếp:** lần đầu update từ build pre-broker sang build broker không thể chuyển ownership của các PTY đã được daemon cũ tạo trước đó. Sau khi đã chạy build broker, các session tạo mới có thể được giữ qua các self-update tiếp theo.

Chi tiết xem `SELF_UPDATE.md`.

## Cấu hình local và remote

File runtime:

```text
<workspace>/vscode_tasks_menu.ini
```

Nếu chưa có, app tự tạo với quyền `0600`. File này có thể chứa credential nên không nên commit vào Git. File mẫu được commit tại root là `vscode_tasks_menu.ini.example`.

### Protocol

`server.protocol` nhận hai giá trị:

- `https` — **mặc định**.
- `http` — chỉ dùng khi chủ động muốn tắt TLS.

Port không thay đổi theo protocol. Ví dụ `port = 42882` có thể phục vụ `https://...:42882` hoặc `http://...:42882` tùy `protocol`.

Config cũ chưa có `protocol` được hiểu là `https`.

### HTTPS mặc định và certificate tự ký

Config local tối thiểu:

```ini
[server]
protocol = https
bind = 127.0.0.1
port = 0
open_browser = true

[auth]
enabled = false
username = admin
password = change-me
```

Với `protocol=https`:

- Nếu cấu hình cả `tls_cert` và `tls_key`, tool dùng đúng certificate/key đó và kiểm tra key pair trước khi mở server.
- Nếu bỏ trống cả hai, tool tự tạo **self-signed certificate** và ECDSA P-256 private key, sau đó tái sử dụng qua các lần chạy.
- Certificate tự sinh có SAN cho `localhost`, loopback, hostname máy, `advertise_host`, và các IP interface hiện tại khi bind wildcard.
- Certificate tự sinh được rotate khi sắp hết hạn hoặc SAN hiện tại không còn đủ.
- Private key và certificate tự sinh được lưu ngoài repo trong user config directory theo hash workspace; thư mục private `0700`, file `0600`.
- `--status`/startup in đường dẫn certificate và SHA-256 fingerprint để người dùng đối chiếu trước khi trust.

Self-signed HTTPS **mã hóa traffic nhưng không tự tạo trust**. Browser sẽ cảnh báo cho tới khi certificate được trust. Hãy kiểm tra fingerprint mà tool in ra trước khi thêm certificate vào trust store. Tool không tự động sửa trust store của hệ điều hành/browser.

Nếu client truy cập bằng IP/hostname cụ thể, nên đặt đúng `advertise_host` để giá trị đó được đưa vào SAN:

```ini
[server]
protocol = https
bind = 0.0.0.0
port = 42882
advertise_host = 192.168.1.20
open_browser = true

[auth]
enabled = true
username = admin
password = thay-bang-mat-khau-rieng
```

Có thể thay self-signed bằng certificate riêng:

```ini
[server]
protocol = https
bind = 0.0.0.0
port = 42882
advertise_host = devbox.example.lan
tls_cert = /absolute/path/to/server.crt
tls_key = /absolute/path/to/server.key
```

### HTTP tùy chọn

Khi thật sự cần plaintext HTTP:

```ini
[server]
protocol = http
bind = 127.0.0.1
port = 42882
open_browser = true
```

Với `protocol=http`, không được cấu hình `tls_cert`/`tls_key`. Nếu HTTP được bind ra ngoài loopback, tool vẫn yêu cầu authentication và in cảnh báo rằng Basic Auth không mã hóa credential trên đường truyền.

### Remote access và security boundary

Khi bind ra ngoài loopback, app **không khởi động** nếu auth chưa bật, username/password rỗng hoặc password vẫn là `change-me`. Daemon còn kiểm tra **listener thực tế sau khi bind**; vì vậy các đường nội bộ như `--listen-addr` hoặc listener được handoff cũng không thể vô tình mở non-loopback khi auth không hợp lệ.

Remote client không có credential chỉ nhận `401`; `/api/health` chỉ anonymous từ loopback. Basic Auth có giới hạn brute-force theo IP, WebSocket giữ same-origin check mặc định và message read-limit, HTTP server giới hạn header/body và header timeout. Session broker không mở TCP; Unix socket được bảo vệ bằng thư mục private `0700` và socket `0600`.

Backend chặn cross-origin request làm thay đổi trạng thái và gửi security header. Khi HTTPS bật, server yêu cầu TLS 1.2+ và gửi HSTS. CSP không cho phép tải script từ CDN ngoài; browser chỉ dùng asset do chính daemon phục vụ.

Nếu dùng HTTPS reverse proxy với **legacy mode**, nên bind tool vào loopback và vẫn giữ `auth.enabled=true`; proxy auth chỉ nên là lớp bổ sung.

Với **shared-server mode**, TaskDeck identity/permission vẫn là authoritative và upstream proxy → TaskDeck cũng phải dùng HTTPS. Không dùng `X-Forwarded-Proto` để thay thế TLS backend. Xem cấu hình, trust model, backup/restore và nginx mẫu tại [SHARED_SERVER_DEPLOYMENT.md](SHARED_SERVER_DEPLOYMENT.md).

## SSH và database connections

TaskDeck có panel **Connections** cho terminal local, SSH và database profiles. Database hỗ trợ MySQL, Redis, MongoDB và SQLite; MySQL/Redis/MongoDB có thể đi qua generic SSH tunnel, còn SQLite là local-file only.

**Connection / Tunnel Graph** hiển thị route thực tế của các connection đang lưu/đang chạy, ví dụ `Browser → TaskDeck → SSH host → Tunnel → Database`. Với DB tunnel, session metadata mang tunnel ID, PID, local loopback port, remote host/port, SSH profile và thời điểm start; Graph có Check và Reconnect/Open nhưng không expose password, private key hay secret reference. DB Reconnect dùng đúng profile/session workflow hiện hữu và cảnh báo nếu còn pending grid/query edit hoặc transaction chưa commit.

Database session có **Workbench UI** gồm navigator + context menu, Data grid phân trang/editable, Structure/Inspector và Query editor. MySQL/SQLite hỗ trợ sort/filter và row editing theo stable key; MongoDB dùng document/`_id`; Redis dùng type-aware key viewer/editor cho các type có semantics an toàn. Query editor của MySQL/SQLite dùng CodeMirror 6 vendored với SQL syntax highlighting + autocomplete cho keyword, table/view và column (kể cả alias/qualified name); metadata cột được lazy-load từ schema hiện tại và có thể gọi completion bằng `Ctrl+Space`. Workbench hỗ trợ nhiều Query tab độc lập (`Query 1`, `Query 2`, …). Một Query tab có thể chạy nhiều SQL statement phân cách bởi `;`; TaskDeck tách an toàn ngoài string/comment, thực thi tuần tự qua API single-statement hiện có và hiển thị từng kết quả trong các tab `Result 1`, `Result 2`, …; khi một statement lỗi thì dừng các statement phía sau. Query result hỗ trợ chọn dòng, copy/export TXT/CSV/JSON, refresh, client-side filter/order, click header để đổi ASC/DESC, và Add row khi SELECT được xác nhận editable. SQL script `.sql`/`.txt` có thể Open/Save trên host workspace hoặc client; file trên 2 MiB không được nạp vào editor mà chuyển sang streamed import wizard (tối đa 2 GiB), với host path luôn bị giới hạn trong workspace. Direct SQL/query editor vẫn là first-class workflow.

**Database Schema Diff** so sánh hai database session đang mở theo hướng **Source → Target** chỉ bằng metadata (`list_objects` + `describe_object`), không đọc table data. Snapshot chuẩn hóa columns/indexes/foreign keys/SQL definition, có export structure JSON và preview migration SQL. MySQL hỗ trợ sinh `ADD/DROP/MODIFY COLUMN` cùng create/drop object khi metadata đủ; SQLite chỉ tự sinh DDL tương thích rõ ràng và để warning cho thay đổi cần table rebuild. Apply luôn chạy sau preview; destructive migration yêu cầu nhập `APPLY <catalog>`, target read-only bị chặn, và tiến độ được đưa vào Operation Center.

**Transaction workspace** dùng chính DB session hiện hữu. MySQL/SQLite có Begin/Commit/Rollback, badge `TX ACTIVE` và toggle **Auto-commit** (mặc định bật). Tắt Auto-commit sẽ mở transaction thật; bật lại bị chặn cho đến khi Commit/Rollback để không commit ngầm. Data Grid và editable Query result có thể Apply nhiều nhóm thay đổi liên tiếp trong cùng transaction; Commit/Rollback bị chặn nếu còn edit local chưa Apply/Revert, và các grid/SELECT result được reload sau khi transaction kết thúc để phản ánh dữ liệu thật.

Các capability mới trong Connections/Workbench:

- MySQL và SQLite có explicit transaction **Begin / Commit / Rollback** trên một connection thật; trong transaction, grid/object actions bị gate để không chạy trên connection khác;
- query đang chạy có thể **Cancel** mà không kill toàn adapter; MySQL transaction dùng `CONNECTION_ID()` + `KILL QUERY`;
- SQLite có **EXPLAIN QUERY PLAN**; MySQL/MariaDB có **EXPLAIN** và **EXPLAIN ANALYZE** cho SELECT/WITH; kết quả được chuẩn hóa thành plan tree (SQLite theo id/parent, MySQL theo SELECT/table, Analyze theo cây output `->`) và vẫn có Raw plan result để debug; nếu server MySQL/MariaDB không hỗ trợ Analyze, nút Analyze tự bị ẩn sau lỗi capability/runtime rõ ràng;
- Query History + Saved Snippets được persist, và relational Structure có foreign-key metadata/navigation;
- SSH profile expose keepalive/connect timeout, hỗ trợ Local/Remote/Dynamic forwarding (`-L/-R/-D`) chỉ cho interactive terminal;
- SSH test có host-key recovery an toàn: inspect matching `known_hosts` entry và chỉ xóa old entry sau explicit confirmation; TaskDeck không tự trust key mới;
- FTP profile hỗ trợ verified FTPS modes; file workspace có sync/mirror dry-run, Sync Profile, exclude glob, SHA-256 compare, bidirectional conflict detection và mirror delete mặc định tắt;
- Project/File context menu có integrity tools dùng chung: SHA-256, MD5 compatibility, compare checksum, tạo SHA256SUMS cho folder và verify SHA-256 manifest; manifest generation không tự ghi file vào project;
- Archive tools hỗ trợ preview ZIP/tar.gz không extract, create/extract an toàn trong workspace, multi-select create/download ZIP; Host archive có thể upload + extract qua SFTP profile liên kết SSH, còn FTP cố ý không giả lập remote shell;
- Project file context menu có **Open With…** thống nhất cho Text Editor, Hex, Image Preview, Markdown Preview, Diff, Download và terminal directory; local single-user mode còn có Open via host application, còn shared-server cố ý không expose host-app launch;
- **Ctrl/Cmd+Shift+P** mở Unified Command Palette: tìm/mở file tools, Explorer/Search/Symbols/Project Profiles/Snapshots, chạy trực tiếp task theo label, mở/refresh Git, mở SSH/Database/SFTP/FTP Connections, Settings và toàn bộ terminal actions; palette cũ của terminal được giữ alias tương thích;
- **Ctrl/Cmd+P** mở Global Quick Open: project files qua bounded backend search, recently opened, mọi workspace tab, terminal/task session, database tab, SSH/DB/SFTP/FTP saved profiles; prefix `>` command, `@` symbol, `#` text search, `git:` branch/commit graph search và `ssh:` profile; không fetch toàn project tree;
- **Operation Center** gom background work vào một panel `Queued / Running / Completed / Failed`: Git background jobs, file-transfer/file-copy queue, Patch runs, task sessions + durable task history, self-update và DB import/export; action Open/Cancel/Retry/Copy error/Clear completed được delegate về đúng subsystem, không tạo job engine thứ hai;
- server-side FTP/SFTP queue được journal qua daemon restart và upload/download có resumable partial-transfer support khi transport cho phép.

Hướng dẫn sử dụng, dependency, giới hạn bảo mật, policy host key, secret rotation/backup/recovery và offline/self-update build được tổng hợp tại:

- [CONNECTIONS.md](CONNECTIONS.md)


## Menu từ tasks.json

Các field riêng đang dùng:

```jsonc
{
  "label": "Build: Compile + Bluetooth OTA",
  "menuLabel": "Compile + Bluetooth OTA",
  "menuGroup": "ESP32-C3 Main/Build",
  "type": "process",
  "command": "python3",
  "args": ["-u", "${workspaceFolder}/ota.py"]
}
```

- `menuGroup`: đường dẫn menu đa cấp, phân cách bằng `/`.
- `menuLabel`: label ngắn trên menu; nếu thiếu dùng `label`.
- group đặc biệt `build` được đưa các shortcut build ra root như menu terminal cũ.
- `${workspaceFolder}`, `${workspaceFolderBasename}` và `${env:NAME}` được expand lúc task bắt đầu.

Task `type=process` chạy command/args trực tiếp. Task shell mặc định chạy qua `/bin/bash -c`, khớp hành vi menu Python trước đây.

Parser hỗ trợ JSONC của VS Code: `// comment`, `/* block comment */` và trailing comma; comment marker nằm bên trong chuỗi/URL vẫn được giữ nguyên.

### Visual editor cho tasks.json

Trong **Settings → Workspace → Edit tasks.json…**, TaskDeck có editor trực quan cho `.vscode/tasks.json`. Editor đọc/ghi qua cùng project-file API đã có optimistic SHA check, workspace path containment và mutation lock; không có đường ghi cấu hình task thứ hai.

Visual mode hỗ trợ add/clone/delete/reorder task, label/menu group/detail, shell/process, command, args, cwd, environment và các template phổ biến cho Bash, Python, Node.js, Go, Rust, Make, CMake, Gradle, Maven và Docker Compose. Các field VS Code nâng cao vẫn được giữ trong object và có thể chỉnh đầy đủ ở **Raw JSON**. Lưu từ Visual mode sẽ normalize JSON và bỏ JSONC comment; UI cảnh báo trước về đặc điểm này.

Command, file argument và danh sách execution file có thể chọn bằng workspace file browser. Multi-file/script execution được materialize thành một shell command tuần tự dùng `&&` (dừng ở lỗi đầu tiên) hoặc `;` (tiếp tục độc lập), nên task đã lưu vẫn đi qua runner hiện hữu. Metadata `taskdeck.template`, `taskdeck.runner`, `taskdeck.executionFiles` và `taskdeck.executionMode` chỉ giúp visual editor dựng lại cấu hình.

### Task workflow / dependency DAG

TaskDeck hiểu `dependsOn` và `dependsOrder` của VS Code và bổ sung policy runtime trong `taskdeck.workflow`:

```jsonc
{
  "label": "Deploy",
  "type": "shell",
  "command": "./deploy.sh",
  "args": ["--version", "${output:Package.version}"],
  "dependsOn": ["Test", "Package"],
  "dependsOrder": "parallel",
  "taskdeck": {
    "workflow": {
      "retry": 2,
      "timeoutSeconds": 300,
      "continueOnError": false,
      "condition": "success",
      "outputs": ["deployment"]
    }
  }
}
```

- dependency mặc định chạy **parallel**; đặt `dependsOrder: "sequence"` để chạy tuần tự theo thứ tự `dependsOn`;
- shared dependency được lock/idempotent trong một workflow nên chỉ chạy một lần dù nhiều nhánh cùng cần;
- `retry` từ 0–10; `timeoutSeconds` từ 0–86400 và dùng GNU/coreutils `timeout` khi timeout được bật;
- `continueOnError` biến failure của node thành effective success để downstream tiếp tục;
- `condition`: `success` (mặc định), `failure`, hoặc `always`;
- task khai báo output bằng `outputs`, rồi emit dòng `::taskdeck-output name=value`; downstream dùng `${output:Task label.name}`;
- output name phải là identifier an toàn, workflow tối đa 256 task và config sai/cycle/missing dependency bị từ chối trước khi launch;
- input `${input:...}` của toàn dependency chain được gom lên root task để dialog input hỏi đủ trước khi chạy;
- **Graph** trong visual tasks editor hiển thị roots, dependency edges và cảnh báo missing/cycle trước khi lưu.

Workflow được compile thành **một task execution** rồi chạy qua session/broker hiện hữu, vì vậy ownership, stop/restart, terminal tab và shared-server `tasks.run` không có pipeline thứ hai.

### Task run history

Task hoàn tất được ghi vào durable history server-side (tối đa 50 run gần nhất):

- trạng thái PASS/FAIL/STOPPED, exit code, thời gian bắt đầu/kết thúc và duration;
- Git commit tại task CWD khi CWD nằm trong Git repository;
- CWD, command preview, target type/profile và Project Profile đang active;
- console log artifact riêng, bounded 1 MiB/run và có nút **Details / Download log / Run again**;
- chọn đúng hai run rồi **Compare** để đối chiếu metadata + log side-by-side.

TaskDeck **không persist toàn bộ environment variables** vào history để tránh lưu credential/secret ngoài ý muốn; phần “environment/profile” được biểu diễn bằng target/profile context và Project Profile đã chọn. History metadata/log dùng project-local state directory, file permission private và shared-server vẫn áp ownership/visibility của task session.

## Browser assets chạy offline và được vendor thủ công

Web UI dùng các bản đã pin:

```text
@xterm/xterm 6.0.0
@xterm/addon-fit 0.11.0
```

Các file JavaScript/CSS và license đã được đưa trực tiếp vào source tree:

```text
vscode_tasks_menu_go/web/vendor/
```

Chúng được review như source của project và `go:embed` vào binary. **Build/runtime không dùng npm, node, CDN hoặc package manager để tải các asset này.** Project cũng không còn script tự động tải/cập nhật npm package.

Khi cần nâng xterm/addon-fit, thực hiện thủ công: lấy đúng upstream source/release mong muốn, review nội dung, thay các file đã vendor trong `web/vendor/`, cập nhật `web/vendor/VERSION.txt`, sau đó commit toàn bộ source đã review. Không dùng tool tự động update dependency.

Version và provenance hiện tại được ghi tại:

```text
vscode_tasks_menu_go/web/vendor/VERSION.txt
```

## Source layout

```text
vscode_tasks_menu_go/
├── cmd/vscode_tasks_menu/      entry point
├── internal/config/            INI config
├── internal/server/            HTTP/WebSocket + web UI
├── internal/broker/            persistent session broker + Unix-socket client/server
├── internal/session/           PTY session engine used by the broker
├── internal/state/             daemon state + file lock
├── internal/tasks/             JSONC parser + command resolver
├── internal/terminal/          terminal UI native Go
├── web/                        embedded browser assets
│   └── vendor/                 reviewed third-party source snapshots
├── go.mod
└── go.sum
```

Root `./vscode_tasks_menu` là launcher mỏng: tự build binary khi source mới hơn `.build/vscode_tasks_menu`, sau đó chạy binary với `--workspace "$PWD"`.

## Đóng gói source

`zip_modules.sh` có module:

```text
E. vscode-tasks-menu-go
```

Module này lấy toàn bộ source Go và browser asset dạng text (`.go`, `.js`, `.css`, `.txt`, `go.mod`, `go.sum`) nhưng loại `.build/`. Root launcher/config example nằm trong module `8` (root helper files). File thật `vscode_tasks_menu.ini` không được collect vì bị Git ignore.

## Kiểm tra CI

Workflow `.github/workflows/vscode_tasks_menu_go.yml` chạy với dependency Go thật và kiểm tra:

```bash
go test ./...
go vet ./...
go build -trimpath ./cmd/vscode_tasks_menu
```

Mục đích là bắt lỗi compile/test bằng `github.com/coder/websocket` và `github.com/creack/pty` thật, không dựa vào stub compile.
