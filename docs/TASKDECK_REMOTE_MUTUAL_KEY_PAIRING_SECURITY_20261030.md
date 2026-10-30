# TaskDeck Remote — Ghép đôi public key hai chiều khi PHP đã bị hack

Ngày: 2026-10-30. Branch: feature/taskdeck-remote-access-architecture.
Trạng thái: thiết kế và kiểm định ranh giới, KHÔNG bật Remote Control.

## Kết luận

Phương án hai public keys khả thi để chống hacker chỉ chiếm PHP hoặc
Relay **nếu client crypto chạy trong chương trình được cài độc lập**.
Nếu UI được tải từ chính PHP, dù public key TaskDeck đã nhập vào browser
và private key lưu local, hacker có thể sửa JavaScript để đọc mật khẩu,
lạm dụng một CryptoKey non-extractable bằng các lời gọi sign/decrypt,
thay nội dung lệnh hoặc thay trust anchor hiển thị. Không dùng origin
PHP làm nơi chứa và dùng khóa định danh client.

Giả định để phòng thủ: private key Agent vẫn trên host, private key
client vẫn trong client độc lập, host approved keys và TaskDeck SQLite
không bị sửa. Nếu cả host hoặc client tin cậy bị chiếm, mô hình này
không bảo đảm ngăn truy cập. Hacker PHP vẫn có thể DoS/phishing.

## Đối chiếu bốn bước đề xuất

1. Client sinh cặp Ed25519 (Kc_priv, Kc_pub), hiển thị public key và
   fingerprint cho người vận hành. KHÔNG gửi Kc_priv qua PHP. Native
   login-only client đã làm việc này trong
   vscode_tasks_menu_go/internal/remoteclientkey/client.go và
   cmd/taskdeck_remote_login/main.go.
2. Người dùng tới host, chạy taskdeck remote-key add cho Kc_pub. Host
   gắn khóa duyệt với immutable TaskDeck User.ID. Host đã có Agent
   identity Ed25519 (Ka_priv, Ka_pub) theo workspace: nên in ra Ka_pub
   hoặc Device ID SHA-256(Ka_pub) hiện tại, **không phát sinh lại một
   Agent identity sau mỗi lần add**. Rotation phải được xác nhận trực
   tiếp và có cơ chế đổi trust anchor; PHP không được tự đổi Ka_pub.
3. Nhập/scan Ka_pub hoặc Device ID qua **native app đã cài độc lập**
   hoặc Firefox/Chrome extension với popup/options thuộc extension
   origin. Lưu pin trong kho của chính client; native CLI hiện dùng
   file owner-only 0600 và thư mục 0700. Không nhập vào form do PHP
   phát hành. IndexedDB + WebCrypto extractable:false cũng KHÔNG
   cứu được khi JS cùng PHP origin là độc hại.
4. PHP chỉ làm trung chuyển ciphertext/rendezvous metadata. Ở HEAD
   hiện tại PHP còn có vai trò control-plane: MFA, role ceiling,
   signed ticket 30s, live grant-check. Tất cả các dữ kiện này được
   coi là KHÔNG ĐÁNG TIN để cấp quyền TaskDeck. Không loại bỏ
   các gate hiện có khi chưa sửa/kiểm chứng lại protocol.

**Không bao giờ trao đổi private key hai chiều. Chỉ trao đổi public.**
Public key không bí mật nhưng đường nhập/ghim phải bảo đảm tính xác thực.

## Handshake bảo mật yêu cầu

A. Client kiểm tra public Ka_pub đã ghim ngoài PHP. Agent kiểm Kc_pub
   đã duyệt và gắn User.ID trong registry host trước khi nhận quyền.
B. Hai phía sinh ECDH ephemeral cho **mỗi phiên**. Agent ký transcript
   bằng Ka_priv; client ký transcript bằng Kc_priv (domain-separated).
   Transcript phải bind protocol/version, cả hai public key fingerprint,
   Device ID, nonces, probe/session ID, ephemeral keys, expiry.
C. Xác minh chữ ký và các nonce trước khi dùng HKDF(ECDH secret,
   transcript hash) tạo send/receive AEAD keys. Các bản tin AES-GCM
   có sequence numbers, replay protection, length limits, expiry.
   Ed25519 dùng ký **không phải** giải thuật mã hóa payload.
D. Sau xác minh hai khóa, TaskDeck host còn phải xác minh password
   TaskDeck, account enabled, current local User.ID, membership,
   RBAC theo từng action và key revocation. PHP admin/grant/role không
   thể tự tạo principal hoặc nới quyền.
E. Với terminal/file/Git/Patch có tác dụng phụ phải có typed operations,
   authorization sát side effect, timeout/cancel, audit và chống retry
   gây thực thi lặp. Không tunnel thẳng HTTP TaskDeck qua PHP.

## Sơ đồ nguyên lý

    Trusted native client / browser extension origin:
      Kc_private + pinned Ka_public + local TaskDeck password UI
                | public Kc qua OOB ---------------+
                | encrypted AEAD over untrusted    |
                v                                    v
     COMPROMISED PHP / RELAY                 HOST TASKDECK CLI
     tickets, routing, ciphertext           owner approves Kc_public
     cannot authenticate TaskDeck           prints existing Ka_public
                |                                    |
                +---- opaque E2EE traffic ----------> TaskDeck Agent
                                                     Ka_private
                                                     local user DB + RBAC

Out-of-band (OOB) là sao chép/scan từ host thực, không qua form PHP.
Không cần mở inbound port của TaskDeck; kết nối Agent->Relay là outbound.

## Nguy cơ đặc biệt của Firefox/Chrome

- JS do PHP phát hành có cùng origin với localStorage/IndexedDB của
  trang PHP. Nó đọc được key exportable và có thể **dùng** CryptoKey
  non-extractable để ký hoặc giải mã qua WebCrypto.
- Firefox/Chrome extension riêng có thể là client an toàn hơn: giữ
  private key, password form và signing API tại extension origin;
  không đưa CryptoKey vào DOM/content script của trang PHP, không
  import JS từ PHP, không cấp quyền ký tự động theo yêu cầu PHP.
- Client native dùng HTML/CSS WebView đóng gói local là lựa chọn khác.
  Nếu dùng local HTTP 127.0.0.1, phải chặn cross-origin, kiểm Host,
  Origin, CSRF token và DNS rebinding; không tin JS của Portal.
- PHP/Relay vẫn thấy IP/routing/timing/traffic sizes và vẫn có thể
  chặn kết nối hoặc lừa tải client giả. Chống giả mạo quyền điều khiển
  không đồng nghĩa chống DoS, phishing, endpoint compromise.

## Hiện trạng và GO/NO-GO

Đã có: native client Ed25519, owner-approved host client registry,
Agent identity + out-of-band Device ID pin, authenticated E2EE,
TaskDeck-local password và role checks. Client hiện chỉ login-only.
Staged internal application session/action code chưa nối production WSS.
Giữ nguyên remote_control_available=false; KHÔNG mở route điều khiển.

Để đạt mục tiêu mới cần thêm UI cài độc lập (native hoặc extension),
luồng xuất Host public key thực tế cho operator, kiểm thử onboarding,
revoke/rotation và lệnh qua WSS thực trên Linux/macOS/NAT, kiểm định
binary/update độc lập và pentest PHP/JS thay thế. Không gọi đây là
100% live-ready trước khi hoàn thành.

Tài liệu tham khảo: W3C Web Cryptography API security considerations,
RFC 10017 section 5 (malicious JavaScript), OWASP Web Frontend
Security Cheat Sheet.
