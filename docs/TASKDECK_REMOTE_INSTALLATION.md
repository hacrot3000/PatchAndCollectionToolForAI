# Cài đặt TaskDeck Remote (PHP Portal + Go Relay + TaskDeck Agent)

> **Trạng thái 30/10/2026: EXPERIMENTAL / KHÔNG PHẢI RELEASE REMOTE CONTROL.**
> Merge source vào `main` **không bật điều khiển từ xa**. Các luồng Terminal,
> File, Editor, Git, Patch/Collection và proxy HTTP/WS TaskDeck **chưa có tuyến
> remote được phát hành**. `remote_control_available=false` ở mọi chế độ.
> Hiện có pairing/presence, E2EE diagnostic/login và các thao tác native
> giới hạn: `session.ping`, đọc tệp 192 byte, `git.status` 192 byte và
> thay tệp 96 byte có điều kiện (mặc định tắt). **Không có shell, Patch,
> Git write/push hay Remote Control UI hoàn chỉnh.** Không triển khai như
> một dịch vụ Remote Control công khai.

## 1. Kiến trúc và yêu cầu

| Thành phần | Cài ở đâu | Vai trò |
| --- | --- | --- |
| **PHP Portal** | Shared host có HTTPS/PHP 8.1+/PDO SQLite/Sodium | Tài khoản Portal + MFA, pairing, cấp ticket/lease, grant và heartbeat; **không có mật khẩu TaskDeck** |
| **Go Relay** | Máy riêng chạy Go WSS lâu dài (VPS hoặc service phù hợp), TLS hợp lệ | Trung chuyển các bản tin mã hóa, xác minh ticket/Agent; **không giải mã** hay thực thi |
| **TaskDeck Agent** | Chính máy Linux/macOS chạy TaskDeck + project | **Chủ động kết nối ra ngoài** Portal HTTPS và Relay WSS, không cần public IP, port forwarding hoặc mở inbound TaskDeck |
| **Trusted client** | Linux/macOS do người dùng kiểm soát, build hoặc verify độc lập từ GitHub | Giữ client private key, xác minh Agent pin và nhập mật khẩu TaskDeck **cục bộ**, không tải JavaScript/nhập credential từ PHP |

**Điều kiện an toàn:** Ngay cả sau khi PHP bị hack (sau lần pairing đáng tin cậy), PHP/Relay không được tự đăng nhập TaskDeck; không đọc được mật khẩu, request/response E2EE; không bỏ qua host-key approval, TaskDeck local password/RBAC để chạy lệnh. Mọi tính năng mới chỉ được release sau khi ba điều kiện trên PASS trên chính đường truyền chức năng đó, không dựa vào PASS của `session.ping`.

PHP có thể đọc dữ liệu Portal và metadata (Device ID, IP truy cập, thời gian/kích cỡ bản tin) hoặc gây DoS; E2EE không bảo vệ người dùng nhập mật khẩu vào **trang PHP đã bị thay mã**. Chỉ dùng client đã tải/ghim nguồn từ trước hoặc từ kênh độc lập đã kiểm chứng. Không cần browser extension và không dựng thêm một Portal; Go Relay **không** phải Portal thứ hai, mà là dịch vụ WSS trung chuyển riêng. **Shared host chỉ chạy PHP, không chạy được long-lived Go/WSS, thì chưa đủ hạ tầng cho kiến trúc này.**

## 2. Cài PHP Portal trên shared hosting (chỉ pairing/diagnostics)

Yêu cầu: PHP >= 8.1, `pdo_sqlite`, `sodium`, HTTPS, thư mục session riêng; Apache dùng `mod_rewrite` và cho phép `.htaccess`, nginx/LiteSpeed cấu hình rewrite tương đương.

1. Deploy thư mục `remote_portal/` vào vị trí ứng dụng. **Document root chỉ trỏ đến `remote_portal/public/`**; các thư mục `src/`, `bin/`, `offline/`, `tests/`, database, PHP sessions và private key nằm **ngoài** webroot, không truy cập URL được.
2. Trên account PHP CLI/hosting, tạo thư mục private (owner-only `0700`), DB `0600`, giữ nguyên owner (không đặt world-readable). Cấu hình biến môi trường cho **cả PHP-FPM và PHP CLI** bằng cơ chế riêng của host, không ghi seed/secret vào Git hay file trong public.
3. Ví dụ cấu hình (thay bằng domain/path thực tế; đây là **biến môi trường**, không phải lệnh tiết lộ secret):

```sh
export REMOTE_PORTAL_DB_PATH=/home/ACCOUNT/private/taskdeck-remote/portal.sqlite3
export REMOTE_PORTAL_SIGNING_SEED_PATH=/home/ACCOUNT/private/taskdeck-remote/ticket.seed
export REMOTE_PORTAL_ISSUER=https://portal.example.com
export REMOTE_PORTAL_RELAY_ORIGIN=https://relay.example.com
# Chỉ bật cho thử nghiệm pairing và session.ping có kiểm soát:
export REMOTE_PORTAL_EXPERIMENTAL_TICKETS=1
# KHÔNG đặt REMOTE_PORTAL_DEV_HTTP=1 trên Internet.
```

4. Chạy các lệnh từ **root repository** (không expose file CLI qua web):

```sh
php remote_portal/bin/admin.php init
php remote_portal/bin/admin.php init-ticket-key   # lần đầu; không xoay key tùy tiện
php remote_portal/bin/admin.php relay-public-key   # CHỈ xuất public key base64
php remote_portal/bin/admin.php create-admin sysadmin
```

Mật khẩu admin Portal nhập qua **STDIN**, không qua argv. Đăng nhập Portal trên HTTPS và thiết lập **TOTP/MFA bắt buộc** trước khi tạo remote ticket. Khi cập nhật schema, chạy lại `init` với **cùng DB** (không xóa DB hay đổi signing key); backup DB, private seed và PHP session config bằng quy trình riêng tư có quyền hạn phù hợp.

## 3. Cài Go Relay ở máy chạy WSS riêng

```sh
cd vscode_tasks_menu_go
go build -o ./taskdeck_remote_relay ./cmd/taskdeck_remote_relay
```

Trên máy Relay, copy **PUBLIC** Portal key từ lệnh `relay-public-key` vào file riêng tư, ví dụ `/srv/taskdeck-relay/portal-pub.b64` (owner-only); tạo `/srv/taskdeck-relay/replay` là directory owner-only `0700`. Dùng TLS cert/key khớp `relay.example.com`. Một ví dụ chạy thử trong môi trường được kiểm soát:

```sh
./taskdeck_remote_relay \
  -listen=127.0.0.1:8443 \
  -portal-issuer=https://portal.example.com \
  -relay-origin=https://relay.example.com \
  -portal-public-key-file=/srv/taskdeck-relay/portal-pub.b64 \
  -ticket-replay-dir=/srv/taskdeck-relay/replay \
  -tls-cert=/srv/taskdeck-relay/fullchain.pem \
  -tls-key=/srv/taskdeck-relay/privkey.pem
```

Đưa `https://relay.example.com` ra Internet bằng TLS reverse proxy hỗ trợ **WSS upgrade tới TLS backend** hoặc cấu hình listener TLS trực tiếp thích hợp. Các đường Relay cần: `/healthz`, `/v1/device`, `/v1/browser`. Không proxy các request vào TaskDeck host; không expose TaskDeck port. **Không chạy Relay bằng root nếu không cần** và không dùng nhiều replica với replay ledger cục bộ độc lập (cần ledger giao dịch dùng chung).

**Cờ thử nghiệm mặc định OFF:** `-allow-offline-file-client`, `-allow-offline-login-probe` (đòi cờ offline) và `-allow-app-ping` (chỉ `session.ping`, **không** mở lệnh). Không bật các cờ này khi chỉ cần presence/pairing.

## 4. Kích hoạt Agent trên máy TaskDeck (Linux/macOS)

Cài TaskDeck theo [README chính](../README.md). Workspace phải có **shared-server identity**, HTTPS và authentication, TaskDeck user/project membership và đúng RBAC; xem [Shared Server Deployment](../vscode_tasks_menu_go/SHARED_SERVER_DEPLOYMENT.md). Không thể thay thế bằng quyền admin Portal.

Tại TaskDeck → **Settings → Security → Remote Access**: bật outbound presence, nhập **Portal HTTPS origin**, **Relay HTTPS origin** và **Portal signing public key** ở bước 2. Public key này phải kiểm tra độc lập với người quản trị Portal lúc pairing lần đầu; không lấy bản thay thế tự động từ PHP sau pairing. Lưu cấu hình, reload/restart daemon theo UI. TaskDeck sẽ kết nối **đi ra** đến Portal/Relay; không mở inbound trên mạng/router.

Trên máy TaskDeck, dùng cùng OS account chạy daemon và đúng workspace:

```sh
taskdeck remote-key --workspace /ABSOLUTE/PROJECT host-key
taskdeck remote-key --workspace /ABSOLUTE/PROJECT list
```

Dùng `host-key` để lấy Device ID 64 hex và Agent public key **trực tiếp trên host**. Trong Portal, admin đăng nhập MFA, tạo/duyệt pairing theo Device ID và mã pairing **hiển thị trong Settings của chính TaskDeck**; cấp quyền Portal account cho thiết bị. Không đăng khóa riêng lên PHP.

**Chỉ khi thử nghiệm E2EE login/ping** và operator chủ động chấp nhận mới bật biến môi trường **trước khi daemon bắt đầu**:

```sh
TASKDECK_REMOTE_LOGIN_PROBE=1 TASKDECK_REMOTE_APP_PING=1 \
  taskdeck --workspace /ABSOLUTE/PROJECT
```

Lưu ý: nếu daemon đã khởi chạy sẵn, thiết lập env trong terminal khác **không** thay đổi daemon đó; phải cấu hình trong service/process quản lý daemon và restart đúng cách. Để thử nghiệm `session.ping` đồng thời cần Relay `-allow-app-ping`. Mặc định các biến đều OFF.

## 5. Cài trusted client và duyệt khóa lần đầu

Chỉ dùng client build từ **GitHub commit đã được người sở hữu xác minh** độc lập hoặc artifact GitHub có attestation; không tải binary/HTML từ PHP nếu PHP có thể bị thay mã. Quy trình ký và xác minh archive nằm ở [Native client provenance](TASKDECK_REMOTE_NATIVE_CLIENT_PROVENANCE.md). Sau merge lên `main`, workflow thủ công và script verify ghim `refs/heads/main`.

Trên client Linux/macOS, build bằng Go từ checkout đáng tin (nếu tự build):

```sh
cd vscode_tasks_menu_go
go build -o ./taskdeck_remote_login ./cmd/taskdeck_remote_login
```

Tạo **file ghim Device ID nhận trực tiếp từ TaskDeck host** (không copy từ trang Portal bị nghi ngờ); thay placeholder bằng 64 hex thực:

```sh
mkdir -p "$HOME/.config/taskdeck/trusted-remote"
chmod 700 "$HOME/.config/taskdeck/trusted-remote"
printf '%s\n' 'OWNER_VERIFIED_64_HEX_DEVICE_ID' > "$HOME/.config/taskdeck/trusted-remote/device.id"
chmod 600 "$HOME/.config/taskdeck/trusted-remote/device.id"
```

Chạy client lần đầu (chưa yêu cầu mật khẩu):

```sh
./taskdeck_remote_login \
  -relay=https://relay.example.com \
  -portal=https://portal.example.com \
  -device=OWNER_VERIFIED_64_HEX_DEVICE_ID \
  -pin-file="$HOME/.config/taskdeck/trusted-remote/device.id"
```

Client sinh **client private key tại máy người dùng** và hiển thị **client public Ed25519 key** để copy. Trên chính TaskDeck host, quản trị viên đối chiếu public key qua kênh tin cậy rồi duyệt:

```sh
taskdeck remote-key --workspace /ABSOLUTE/PROJECT add alice 'CLIENT_ED25519_PUBLIC_KEY_BASE64' 'Laptop'
taskdeck remote-key --workspace /ABSOLUTE/PROJECT list
# Thu hồi tức thì khi mất thiết bị:
taskdeck remote-key --workspace /ABSOLUTE/PROJECT revoke CLIENT_KEY_FINGERPRINT_64_HEX
```

Lần chạy client kế tiếp: nhập username TaskDeck tại **client**, copy chỉ public nonce và public P-256 key vào mục Portal **“Cấp ticket cho client TaskDeck cài riêng”**, lấy ticket **30 giây** rồi paste lại vào client. Chỉ sau khi client kiểm tra Agent đã ghim và host xác nhận khóa client mới có prompt mật khẩu TaskDeck **ở terminal cục bộ**. Không nhập mật khẩu TaskDeck vào Portal.

**Thử nghiệm `session.ping`**: trên Relay đã bật `-allow-app-ping`, TaskDeck daemon bật hai env ở bước 4, có thể thêm `-test-app-ping` vào lệnh client phía trên. Đây là yêu cầu cố định read-only E2EE được host RBAC kiểm tra. Client vẫn không cấp HTTP cookie/token TaskDeck.

**Thử nghiệm đọc tệp E2EE (không phải Remote Control production):** thay `-test-app-ping` bằng `-read-file=docs/example.txt` với một tệp văn bản UTF-8 nhỏ trong workspace đã được host cấp quyền `FilesRead`. Chỉ hỗ trợ tối đa 192 byte UTF-8, đường dẫn tương đối, từ chối ký tự/đường dẫn nguy hiểm, tệp không tồn tại hoặc tệp lớn hơn giới hạn; nội dung được trả qua AES-GCM và hiển thị dưới dạng escaped text trên client native. Không chấp nhận ghi tệp qua lệnh `-read-file`, Terminal, Git write, Patch, HTTP, đường dẫn tuyệt đối hay cookie. Giữ hai cờ thử nghiệm Relay/daemon đã nêu phía trên; mặc định OFF. **Chưa live**.

**Git status giới hạn:** `-git-status` dùng cùng client và cờ thử nghiệm; TaskDeck tự kiểm tra `GitStatus`, chạy `git status --short --branch --untracked-files=no` tối đa 192 byte. Đây không phải quyền gọi lệnh Git tùy ý.

**Thử nghiệm thay tệp cực nhỏ — NGUY HIỂM HƠN, MẶC ĐỊNH OFF:** `-replace-file=relative/file.txt` yêu cầu người dùng nhập nội dung thay thế tại CLI tin cậy, tối đa **96 byte UTF-8**. Client đọc và lấy SHA-256 phiên bản hiện có; host phải kiểm tra `FilesWrite`, SHA-256, shared workspace mutation lock, audit cục bộ và trạng thái phiên trước khi commit thay đổi. Ngoài các cờ thử nghiệm đã nêu, chỉ bật `TASKDECK_REMOTE_FILE_REPLACE=1` **trên chính tiến trình daemon TaskDeck** khi chủ động thử chức năng này; giá trị khác `1` sẽ từ chối hoàn toàn. PHP không có khả năng bật cờ host này. Không bật cho môi trường production hoặc host có dữ liệu quan trọng khi chưa thử và backup. Ví dụ cấu hình cho một phiên kiểm thử do chủ host thực hiện:

```sh
TASKDECK_REMOTE_LOGIN_PROBE=1 TASKDECK_REMOTE_APP_PING=1 \
  TASKDECK_REMOTE_FILE_REPLACE=1 taskdeck --workspace /ABSOLUTE/PROJECT
```

Các lệnh `-read-file`, `-git-status`, `-replace-file` hiện chỉ là thao tác native giới hạn; **không cấp session cookie, SSH/PTY, Terminal, HTTP proxy hay Patch/Git push**. `remote_control_available=false` vẫn giữ nguyên.

**Chạy task TaskDeck được host cho phép trước — thử nghiệm, mặc định OFF:** Chỉ chạy task đã có sẵn trong `.vscode/tasks.json`, `taskdeckRemoteAllowed: true` (Boolean), không chứa dependency, inputs, môi trường tùy biến, secret, `taskdeckApproval` hoặc cwd ngoài root. TaskDeck phải bật riêng `TASKDECK_REMOTE_TASK_RUN=1` trong daemon ngoài hai cờ E2EE đang sử dụng. Client native dùng `-run-task=TASK_ID`, chọn đúng **ID task hiện trên TaskDeck local**. Host kiểm tra `TasksRun`, mật khẩu/khóa client/RBAC, grant hiện hành, ghi audit trước khi tạo tiến trình, giới hạn khoảng 7 giây, dừng tiến trình khi hủy và chỉ trả tối đa 192 byte kết quả dưới dạng ciphertext. Không cho PHP/client gửi command, args, shell, env hoặc nội dung task.

Ví dụ định nghĩa một task vô hại do chủ máy kiểm tra:

```json
{
  "label": "Remote safe health check",
  "type": "process",
  "command": "/bin/echo",
  "args": ["TASKDECK_HEALTH_OK"],
  "taskdeckRemoteAllowed": true
}
```

Lệnh chạy thử trên host (chỉ ở môi trường kiểm thử):

```sh
TASKDECK_REMOTE_LOGIN_PROBE=1 TASKDECK_REMOTE_APP_PING=1 \
  TASKDECK_REMOTE_TASK_RUN=1 taskdeck --workspace /ABSOLUTE/PROJECT
```

`-run-task` không làm việc với SSH, Terminal tương tác, Patch, task đòi hỏi xác nhận deploy hoặc sử dụng secrets. **Không phải tính năng Remote Control production**.

## 6. Kiểm tra, sự cố và chốt release

```sh
find remote_portal -name '*.php' -print0 | xargs -0 -n1 php -l
php remote_portal/tests/smoke.php
bash tests/test_remote_client_provenance.sh
cd vscode_tasks_menu_go
go test ./internal/remoterelay ./internal/remoteportal ./internal/remotetransport ./internal/server
```

- **Portal không truy cập được**: kiểm tra HTTPS, rewrite, document root, DB/session quyền 0700/0600, `REMOTE_PORTAL_ISSUER`; không hạ quyền file hay tắt TLS/MFA để đi đường vòng.
- **Agent offline**: kiểm tra TaskDeck daemon, đúng Portal/Relay origin, outbound TCP/443 và lease heartbeat; không mở port inbound TaskDeck.
- **Không nhận key/login**: xác minh `host-key`, ghim client Device ID 0600, `remote-key list`, cùng username/user ID và đúng quyền project; đổi mật khẩu/thu hồi key sẽ làm phiên cũ mất hiệu lực. Ticket hết hạn thì lấy ticket mới; không tái dùng.
- **Không chạy được lệnh trên Remote**: **đúng thiết kế hiện tại**. Có các thao tác E2EE đã liệt kê ở trên, nhưng **không tồn tại** dispatcher Terminal/PTY, Patch, SSH hoặc command tùy ý. `-replace-file` bị từ chối nếu host không bật riêng `TASKDECK_REMOTE_FILE_REPLACE=1`.
- **CI PASS không tự cấp phép release**. Chỉ phát hành Remote Control khi (1) PHP bị hack không thể xác thực/chiếm quyền TaskDeck, (2) mọi dữ liệu TaskDeck truyền qua Portal/Relay đều E2EE, (3) mọi hành động thực tế bị ràng buộc login, khóa client và RBAC cục bộ, có test red-team trên đúng release SHA và triển khai thật. Xem [3 bất biến bảo mật](TASKDECK_REMOTE_THREE_SECURITY_INVARIANTS_20261030.md) và [release gate](TASKDECK_REMOTE_COMPROMISED_PHP_GO_LIVE_GATE.md).

**Merge repository vào `main` chỉ tích hợp source và hướng dẫn**; không thay thế phê duyệt deployment, không tự bật cờ Remote Control.
