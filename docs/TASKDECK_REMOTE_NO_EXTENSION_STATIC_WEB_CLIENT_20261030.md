> **KHÔNG CÒN LÀ PHƯƠNG ÁN TRIỂN KHAI (2026-10-30).** Người dùng xác định PHP tin cậy tại lần pair đầu, không muốn portal/domain thứ hai. Phương án cũ sử dụng origin web tĩnh độc lập và endpoint Relay `/v1/trusted-web` đã được hoàn tác. Thay bằng [mô hình client HTML offline xuất tại lần pair đáng tin cậy](TASKDECK_REMOTE_TRUSTED_FIRST_PAIR_OFFLINE_CLIENT_20261030.md). Không dựa vào Service Worker/PWA cache để giữ mã an toàn khi PHP bị hack về sau.

# TaskDeck Remote — Firefox web client KHÔNG extension, tách hoàn toàn PHP compromised origin

Ngày: 2026-10-30. Branch: feature/taskdeck-remote-access-architecture.
Ưu tiên UX: **mở URL bằng Firefox**, không yêu cầu cài Firefox extension,
desktop app hay mở inbound port tại máy TaskDeck.

## Quyết định kiến trúc

Dùng giao diện HTML/CSS/JS tĩnh tại một **origin HTTPS đáng tin cậy,
độc lập cả hạ tầng lẫn quyền quản trị/deploy** so với PHP Portal,
ví dụ https://remote-ui.example.com. Có thể dùng static hosting do
chính owner kiểm soát (GitHub Pages hoặc dịch vụ static hosting khác)
với tài khoản CI/CD/MFA và quyền deploy độc lập. Không đặt UI trên cùng
shared PHP hosting, không cùng document root/vhost, không dùng PHP
để phục vụ/rewrite/proxy HTML/JS của client, và không dùng iframe
từ PHP hay service worker do PHP quản lý.

**Trang điều khiển phải mở trực tiếp bằng bookmark URL đáng tin**.
Trang PHP có thể quảng bá liên kết nhưng **không là nguồn xác minh
URL của client**: PHP bị hack có thể trả liên kết phishing. Người dùng
xác minh địa chỉ thật một lần rồi lưu bookmark. Phần code static vẫn
có trust/update risk riêng: khóa chặt deploy, source/provenance, CSP,
không chèn runtime JS bên thứ ba hay tải JS động từ PHP. Nếu hacker
chiếm cả static origin thì mô hình này cũng không chống được.

## Đối chiếu quy trình ghép đôi của user

1. Firefox tại trang **trusted static origin** tạo Kc = client Ed25519
   signing keypair, private key non-extractable trong WebCrypto; lưu
   qua IndexedDB **của static origin** (không phải PHP); hiển thị
   Kc_public base64/fingerprint và TaskDeck UserID/username cho owner.
   Không bao giờ lưu private key dưới dạng string trong localStorage,
   không copy private key qua PHP.
2. Trên host TaskDeck: chạy
   taskdeck remote-key --workspace /duong/dan/project add username Kc_public
   rồi dùng
   taskdeck remote-key --workspace /duong/dan/project host-key
   để **in Agent Ed25519 public key và Device ID** của Agent đã tồn tại.
   Lệnh host-key **không sinh khóa khi bị mất key**, không gửi key cho PHP;
   private Agent Ed25519 seed không rời máy host.
3. Trở lại bookmark static UI và nhập/scan **Agent public key/Device ID**
   được nhìn trực tiếp trên máy host. Trang static UI lưu pin trong
   chính IndexedDB origin đáng tin, khi mismatch/rotation thì hard deny
   và yêu cầu xác nhận lại bằng host CLI.
4. Sau đó: Firefox kết nối tới Relay WSS với Origin của static UI; PHP
   chỉ cung cấp ticket/routing/grant phụ và Relay chỉ vận chuyển
   ciphertext. Browser kiểm Ka identity signature & ECDH transcript,
   dùng Kc private để ký một transcript hiện tại (KHÔNG ký payload tự
   do do PHP gửi), kiểm local TaskDeck approved key, TaskDeck password,
   per-action RBAC ngay trên host. Từ đó người dùng chỉ mở bookmark,
   không thao tác ghép đôi lại nếu key không đổi.

Không thực hiện thao tác ghép đôi bằng form \`portal.example.com\`:
nếu PHP bị hack, script có thể lấy lại/đổi khóa, lừa người dùng,
dùng signing key thông qua WebCrypto mặc dù extractable=false.

## Trust boundaries và luồng dữ liệu

    FIREFOX tại https://remote-ui.example.com (trusted static origin)
       Kc private trong IndexedDB, Ka public đã ghim + password input
         |                                |
         | ticket request                | E2EE AES-GCM ciphertext
         v                                v
    PHP PORTAL (untrusted)          Go RELAY (untrusted)
      Portal account/MFA/                 |  /v1/trusted-web
      short ticket + grant               |  WSS
                                          v
                                   TASKDECK AGENT / HOST
                                   Ka private, approved Kc registry,
                                   TaskDeck SQLite local auth + RBAC

Agent->Relay outbound WSS. Cặp Ka private, Kc private **không trao đổi**.
Ký bằng Ed25519; sinh session key bằng ephemeral ECDH P-256
(hiện protocol TaskDeck), HKDF và AES-GCM sequence + replay checks.

**Không nên nói "PHP chỉ proxy" như hiện trạng:** PHP hiện đang làm
Portal login/MFA, cấp signed ticket 30s, grant introspection và
role ceiling. Quyền này bị xem là **không tin cậy để cấp quyền
TaskDeck** khi PHP bị hack; host tự kiểm key, password, local roles.

## Kết nối WebSocket khi client ở origin riêng

Mã Relay mới:
- Cờ **opt-in** \`taskdeck_remote_relay -trusted-web-origin
  https://remote-ui.example.com\`, chỉ nhận HTTPS origin canonical
  không wildcard, không path/query, khác Portal và Relay origin.
  Nếu không khai báo, endpoint được khóa (403).
- Endpoint WSS \`/v1/trusted-web\` chỉ chấp nhận Origin **khớp chính
  xác** cấu hình của Go Relay (không lấy từ PHP, Portal ticket, JS
  runtime hay header do PHP sinh ra).
- Legacy \`/v1/browser\` vẫn dành cho Portal diagnostics + native
  login-only, **không mount** remote application dispatcher.
- Origin chỉ chống JS do PHP trả về tự gọi **trusted WebSocket route**,
  **không phải** xác thực TaskDeck: native code/non-browser có thể
  tự đặt Origin. Vì vậy mọi login/action vẫn bắt buộc khóa client
  được host duyệt + chữ ký binding + TaskDeck password + per-action RBAC.
- Endpoint mới hiện **chỉ tái sử dụng diagnostic handshake**;
  **không** triển khai web control, không mở terminal/files/Git/Patch.

## Ticket và trải nghiệm không extension

Bản đầu có thể copy một ticket ngắn hạn từ Portal rồi paste vào
**trang remote-ui đã bookmark**, không copy private key hay password
vào Portal. Để giảm thao tác về sau, thiết kế flow redirect **từ
trusted UI sang Portal và trở về đúng trusted UI** với state + nonce +
PKCE-like binding, exact redirect origin/URI và chống CSRF. Kể cả
nếu PHP bị hack và phát ticket role admin, Agent vẫn kiểm Kc, Ka,
password, local RBAC và key revocation. PHP có thể từ chối phục vụ,
chặn/quấy nhiễu ticket hoặc phishing URL; không bao giờ bật kiểm
soát chỉ dựa vào claims của PHP.

Chú ý browser same-site/cross-site cookies/CORS, SameSite và
credentialed requests: không bật Access-Control-Allow-Origin:* kèm
credentials, không nới CORS theo Origin do PHP gửi. Nếu không
triển khai redirect/ticket flow an toàn, giữ quy trình paste ticket
thay vì mở credentialed CORS.

## Chặn mã độc trong static UI

- CSP \`default-src 'none'; script-src 'self'; connect-src
  https://portal.example.com wss://relay.example.com; style-src
  'self'; img-src 'self' data:; base-uri 'none'; frame-ancestors 'none';
  form-action 'none'\`. Chính sách triển khai cần kiểm thử có thực
  thi được trên static hosting; chọn self-hosted JS/CSS, không
  \`unsafe-inline\`, không \`eval\`, không JS import từ PHP.
- Strict-Transport-Security, X-Content-Type-Options, Referrer-Policy
  và cache/immutable build version, tùy hosting.
- Không gửi raw password/CryptoKey qua postMessage tới Portal,
  window.opener, iframe, extension/content script không đáng tin.
- UI phải render dữ liệu từ PHP/Relay bằng textContent và schema parse,
  không innerHTML để tránh DOM-XSS; không execute code từ Relay.
- Kc non-extractable làm giảm rò rỉ trực tiếp nhưng **không** ngăn mã
  chạy trên chính trusted static origin dùng key để ký. Phải bảo vệ
  chính origin, pipeline build và kênh cập nhật.
- Browser key storage có thể mất khi xóa site data, profile, private
  browsing, reinstall. Thiết kế revoke/re-enroll rõ, không âm thầm
  đổi public key hay tự approve từ PHP.

## Những gì đã được code và chưa có

Có thật:
- Host CLI \`remote-key host-key\`, đọc identity sẵn có fail-closed;
  \`remote-key add\` local approval.
- Relay cho phép cấu hình trusted static origin và WSS diagnostic
  endpoint độc lập, không đổi Portal behavior.
- Native client login-only E2EE và các host-local verification gates.

**Chưa làm:** Firefox static UI thật, ghép đôi WebCrypto/browser
IndexedDB, first-party deploy origin, flow ticket redirect, production
E2EE remote app dispatcher, RBAC + revoke end-to-end cho thao tác
Terminal/Files/Git/Patch/Collection, actual NAT/Linux/macOS device
test, independent PHP-compromise pentest. Hệ thống vẫn giữ
remote_control_available=false, Remote Control NO-GO.

## Xác nhận CI triển khai điểm vào cho origin độc lập

- Source SHA `ef536bfc18a1bbe4aa6aaba265f1b06a44701abc`.
- [Go Agent + Relay / race / red-team / cross-build #40999562716](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40999562716): **COMPLETED SUCCESS**.
- [PHP Portal security #40999562612](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40999562612): **COMPLETED SUCCESS**.
- Opt-in `-trusted-web-origin` phải được cấu hình tại tiến trình Relay,
  không lấy từ PHP. Test kiểm yêu cầu từ PHP origin, wildcard, origin
  giả và endpoint chưa cấu hình bị HTTP 403. Test cũng kiểm WebSocket
  handshake thực từ origin độc lập khớp chính xác.

Đây mới chỉ là test cho chính sách Origin và diagnostic WebSocket route,
**chưa có** static browser application UI hoặc production Remote
Control dispatcher. Origin chỉ bảo vệ nguồn JavaScript trình duyệt;
mọi hành động thực tế vẫn cần local Ed25519, TaskDeck password và RBAC.
