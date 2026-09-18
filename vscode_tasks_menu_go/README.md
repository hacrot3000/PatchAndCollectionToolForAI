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
./vscode_tasks_menu --stop-daemon
./vscode_tasks_menu --restart-daemon
```

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

Nếu chưa có, app tự tạo với quyền `0600`. File này nằm trong `.gitignore` vì có thể chứa credential. File mẫu được commit tại root là `vscode_tasks_menu.ini.example`.

### Local mặc định

```ini
[server]
bind = 127.0.0.1
port = 0
open_browser = true

[auth]
enabled = false
username = admin
password = change-me
```

### Remote access

Ví dụ:

```ini
[server]
bind = 0.0.0.0
port = 0
advertise_host = 192.168.1.20
open_browser = true
# tls_cert = /absolute/path/to/server.crt
# tls_key = /absolute/path/to/server.key

[auth]
enabled = true
username = admin
password = thay-bang-mat-khau-rieng
```

Khi bind ra ngoài loopback, app **không khởi động** nếu auth chưa bật, username/password rỗng hoặc password vẫn là `change-me`.

Basic Auth trên HTTP không mã hóa credential trên đường truyền. Khi truy cập qua mạng không tin cậy nên cấu hình `tls_cert` và `tls_key` để dùng HTTPS/WSS.

Backend cũng chặn cross-origin request làm thay đổi trạng thái và gửi các security header cơ bản. CSP không cho phép tải script từ CDN ngoài; browser chỉ dùng asset do chính daemon phục vụ.

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
