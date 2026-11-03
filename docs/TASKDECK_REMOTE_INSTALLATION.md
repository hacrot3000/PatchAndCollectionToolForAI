# Cài đặt TaskDeck Remote (PHP Portal + Go Relay + TaskDeck Agent)

> **Trạng thái 30/10/2026: EXPERIMENTAL / KHÔNG PHẢI RELEASE REMOTE CONTROL.**
> Merge source vào `main` **không bật điều khiển từ xa**. Các luồng Terminal,
> File, Editor, Git, Patch/Collection và proxy HTTP/WS TaskDeck **chưa có tuyến
> remote được phát hành**. `remote_control_available=false` ở mọi chế độ.
> Hiện có pairing/presence, E2EE diagnostic/login và tùy chọn thử nghiệm
> `session.ping` và `git.status` chỉ-đọc, cùng GUI thử nghiệm chạy trên máy client. Không triển khai như một dịch vụ Remote Control công khai.

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

**Cờ thử nghiệm mặc định OFF:** `-allow-offline-file-client`, `-allow-offline-login-probe` (đòi cờ offline) và `-allow-app-ping` (chỉ `session.ping` và, nếu host cho phép riêng, `git.status` chỉ đọc; **không** mở shell/Terminal hoặc Git ghi). Không bật các cờ này khi chỉ cần presence/pairing.

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

**Thử nghiệm Git chỉ đọc (nhánh tính năng, không LIVE):** Nếu TaskDeck workspace trỏ đúng root của Git repository, bật thêm `TASKDECK_REMOTE_GIT_STATUS=1` **trên daemon host**. Trên trusted native client chọn `-test-git-status` thay vì `-test-app-ping`. Không bật cả hai đồng thời. Lệnh này chỉ lấy trạng thái Git qua phiên E2EE; mọi `git add/commit/push`, terminal, proxy HTTP vẫn bị từ chối. Git có quá nhiều thay đổi vượt giới hạn 256 bytes sẽ bị từ chối thay vì in thiếu dữ liệu. Không bật trong production trước khi kiểm thử CI và môi trường thật.

### GUI thử nghiệm trên client đã xác minh (read-only, chưa LIVE)

Sau khi cấu hình Agent, Relay, ghim Device ID, duyệt client key và
đăng nhập thành công từ native client, có thể thay cờ `-test-app-ping`
bằng `-gui` khi chạy `taskdeck_remote_login`. Chương trình mở
GUI qua URL `http://127.0.0.1:<random_port>/` **chỉ trên
máy client**. CLI in riêng **mã kích hoạt 64 ký tự hex** vào terminal
của client; hãy dán mã này vào ô kích hoạt trong GUI localhost.
Không đưa mã vào trang PHP hay chia sẻ cho người khác. Đường dẫn mở
browser KHÔNG chứa mã trong tham số tiến trình `xdg-open/open`
(ngăn lộ qua danh sách tiến trình). HTML/JS/CSS được biên dịch
sẵn vào binary độc lập, không được tải từ PHP hay Relay; giao diện
dùng strict Origin/Host, CSP, chống CSRF và không ghi mã vào
storage/cookie. Sau khi nhập, mã chỉ gửi qua request local tới
native client, không đi vào PHP Portal.

GUI có nút **Kiểm tra chức năng** (`session.features` qua E2EE) để hỏi
host những chức năng đọc nào đã bật và được tài khoản hiện tại cấp quyền.
Các nút chức năng đọc sẽ khóa cho đến khi kiểm tra và tiếp tục khóa nếu
chưa được bật/quyền không đủ. Bước kiểm tra tốn một trong bốn lượt E2EE;
đây chỉ là gợi ý giao diện, host vẫn xác thực và kiểm tra RBAC lại ở
**mỗi thao tác**. Native client và Agent phải dùng build tương thích;
host cũ không hỗ trợ thao tác này sẽ từ chối an toàn.

Giao diện hiện có `session.ping`, `project.info` (host phải
bật riêng `TASKDECK_REMOTE_PROJECT_INFO=1`), `tasks.preview` chỉ đọc
(host phải bật riêng `TASKDECK_REMOTE_TASKS_PREVIEW=1`) và `git.status` chỉ đọc
(host phải bật riêng `TASKDECK_REMOTE_GIT_STATUS=1`), cùng nút đóng
phiên. `project.info` chỉ trả tên dự án được TaskDeck xác thực,
tên thư mục workspace (không có đường dẫn tuyệt đối) và host OS;
không liệt kê các dự án khác. Kết
quả Git chỉ hiển thị trong vùng text/bảng và không được chuyển thành
HTML hay command. Chỉ có tối đa bốn thao tác qua Relay trong phiên
kéo dài tối đa năm phút. **Chưa thể thực hiện Git Pull, Commit, Push,
Terminal hoặc Editor**. Không xuất bản listener localhost ra Internet
và không dùng GUI này như một bản Remote Control LIVE.

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

**Thử nghiệm `session.ping`**: trên Relay đã bật `-allow-app-ping`, TaskDeck daemon bật hai env ở bước 4, có thể thêm `-test-app-ping` vào lệnh client phía trên. Đây là yêu cầu cố định read-only E2EE được host RBAC kiểm tra, **không** phải phiên dùng Terminal/File/Git/Patch. Client vẫn không cấp HTTP cookie/token TaskDeck.

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
- **Không chạy được lệnh trên Remote**: **đúng thiết kế hiện tại**. Không tồn tại remote command/file/PTY dispatcher production; mọi hành động không thuộc `session.ping` bị chặn.
- **CI PASS không tự cấp phép release**. Chỉ phát hành Remote Control khi (1) PHP bị hack không thể xác thực/chiếm quyền TaskDeck, (2) mọi dữ liệu TaskDeck truyền qua Portal/Relay đều E2EE, (3) mọi hành động thực tế bị ràng buộc login, khóa client và RBAC cục bộ, có test red-team trên đúng release SHA và triển khai thật. Xem [3 bất biến bảo mật](TASKDECK_REMOTE_THREE_SECURITY_INVARIANTS_20261030.md) và [release gate](TASKDECK_REMOTE_COMPROMISED_PHP_GO_LIVE_GATE.md).

**Merge repository vào `main` chỉ tích hợp source và hướng dẫn**; không thay thế phê duyệt deployment, không tự bật cờ Remote Control.
