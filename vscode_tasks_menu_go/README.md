# VS Code Tasks Menu (Go)

`vscode_tasks_menu_go` là backend Go cho executable ở root:

```bash
./vscode_tasks_menu
```

Công cụ luôn lấy workspace từ **thư mục hiện tại** khi gọi executable và đọc động:

```text
<workspace>/.vscode/tasks.json
```

Vì vậy thay đổi `tasks.json` sẽ đổi menu ngay, không cần build lại Go. Nút **Reload tasks.json** trên web và phím `r` trong terminal mode sẽ đọc lại file.

## Chế độ web

```bash
./vscode_tasks_menu
```

Lần chạy đầu sẽ:

1. build Go binary vào `vscode_tasks_menu_go/.build/` nếu cần;
2. khởi động daemon nền;
3. bind port đã cấu hình (`0` = hệ điều hành chọn port trống);
4. in URL;
5. tự mở browser nếu `open_browser = true`.

Nếu daemon của cùng workspace đã chạy, launcher chỉ in URL và mở browser, không tạo daemon thứ hai.

Mỗi lần bấm một task sẽ tạo một **PTY session riêng** và một tab terminal riêng trên web. Task nhận TTY thật nên các workflow dùng `input()`, ANSI/curses, phím mũi tên, Ctrl+C và các prompt tương tác tiếp tục hoạt động như terminal.

### Lifecycle của tab/session

- Reload browser hoặc đóng/mở lại browser: task đang chạy **không bị kill**; tab được phục hồi từ daemon và log PTY được replay.
- Task/terminal được sở hữu bởi **session broker process độc lập** với web daemon. Daemon chỉ proxy API/WebSocket tới broker qua Unix socket local.
- Khi self-update thay web daemon, broker tiếp tục giữ process group, PTY, session ID và scrollback; daemon mới reconnect lại các session hiện hữu thay vì chạy lại task.
- **Close** một tab đang chạy: chỉ đóng UI, process vẫn chạy và sẽ xuất hiện lại sau reload/mở lại web.
- **Stop**: gửi `SIGINT` cho process group của task.
- Close tab đã `exited/stopped`: session đã hoàn tất được xóa khỏi daemon.
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

Click phải một tab mở thêm phần **Broadcast group**:

- chọn một group đã có để assign tab;
- **Create new group…** để tạo group và assign ngay tab hiện tại;
- **Remove from group** để bỏ tab khỏi group.

Mỗi group chọn một trong các preset màu nền/chữ có độ tương phản cao: Slate, Ocean, Forest, Amber, Violet, Rose, Cyan và Lime. Tab thuộc group được tô theo preset đó để dễ nhận biết. Có thể edit/delete group từ menu Broadcast; xóa group sẽ tự bỏ assignment của các tab thuộc group.

Mode, group definitions, preset màu và mapping `session_id -> group_id` được lưu atomic trong runtime state của workspace, nên reload browser và self-update giữ nguyên khi session ID được broker bảo toàn.

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

Nếu dùng HTTPS reverse proxy thay vì TLS trực tiếp của tool, nên bind tool vào loopback và vẫn giữ `auth.enabled=true`, hoặc để reverse proxy tự enforce authentication.

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
