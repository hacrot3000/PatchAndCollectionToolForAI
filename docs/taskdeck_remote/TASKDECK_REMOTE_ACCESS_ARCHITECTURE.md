# TaskDeck Remote Access — Kiến trúc đề xuất

> Trạng thái: BẢN THIẾT KẾ, chưa triển khai. Ngày: 2026-10-28.
>
> Phạm vi: mở truy cập TaskDeck qua Internet cho các máy nằm sau NAT/CGNAT/firewall, không yêu cầu IP public hoặc NAT port forwarding. Tài liệu này mô tả đích kiến trúc và điều kiện nghiệm thu, KHÔNG xác nhận hệ thống hiện tại đã hỗ trợ.
>
> Tài liệu đánh giá bảo mật độc lập: [TASKDECK_REMOTE_SECURITY_RISK_ASSESSMENT.md](TASKDECK_REMOTE_SECURITY_RISK_ASSESSMENT.md).

## 1. Yêu cầu bất biến

1. Management Portal chạy bằng PHP trên shared hosting; tài khoản và quan hệ phân quyền được quản lý trong SQLite3.
2. Một tài khoản Portal có thể được cấp quyền đối với nhiều TaskDeck; một TaskDeck có thể có nhiều người dùng được cấp quyền.
3. Portal chỉ lưu lâu dài **định danh TaskDeck, tên máy/instance và quan hệ phân quyền**; không lưu địa chỉ IP, cổng, hostname, URL nội bộ, mật khẩu TaskDeck, access token, session cookie, private key hoặc thông tin định tuyến kết nối của từng máy.
4. Quy định "không lưu authentication" trong mục 3 áp dụng cho **thông tin xác thực TaskDeck**, không phải authentication của chính Portal. Portal vẫn phải bảo vệ password hash/passkey, session của tài khoản Portal và khóa ký riêng của Portal.
5. TaskDeck luôn phải bật authentication khi Remote được kích hoạt. Không có đường bypass; đăng nhập Portal không thay thế bước xác thực/ủy quyền của TaskDeck.
6. TaskDeck chủ động tạo kết nối ra ngoài qua TCP/443 và tự báo trạng thái online. **Không** mở inbound listener công khai hay thay đổi firewall/router ở máy client.
7. Người dùng dùng trình duyệt truy cập TaskDeck với giao diện và thao tác tương đương khi truy cập trực tiếp: dashboard, terminal/PTY, editor, file, Git, Patch, log, Settings, tải lên/xuống và WebSocket, theo quyền được phép.
8. Không phá vỡ TaskDeck local, các module Patch/Collection, log viewer, cơ chế Auth/RBAC, project isolation và hành vi đa người dùng hiện tại.
9. Remote mặc định **OFF**; chỉ kích hoạt khi quản trị viên sở hữu máy chủ động đồng ý tại TaskDeck. Mọi lỗi xác thực/kiểm chứng đều **fail closed**.
10. Không coi việc truy cập được trang chủ là hoàn thành: phải kiểm chứng hành vi của mọi API, WebSocket, upload/download, thao tác kéo dài và reconnect.

## 2. Đánh giá khả thi và lựa chọn phương án

| Phương án | Shared hosting PHP đủ dùng? | Realtime terminal/WebSocket | Ưu/nhược | Quyết định |
| --- | --- | --- | --- | --- |
| HTTP polling qua PHP | Về kỹ thuật có thể | Trễ, tốn request, upload/stream khó | Không có daemon riêng nhưng giới hạn timeout, concurrency, I/O của hosting | Chỉ dự phòng/POC |
| WebRTC P2P + STUN/TURN | Không; vẫn cần signaling và TURN để bảo đảm kết nối | Có | Ít relay khi đi P2P, triển khai ICE/TURN phức tạp, nhiều mạng cần TURN | Cân nhắc giai đoạn sau |
| **Go WebSocket Relay + PHP Portal** | **Portal chạy trên shared host; Relay cần nền tảng giữ kết nối dài hạn** | **Có** | Đơn giản hóa tương thích HTTP + WebSocket hiện có, luôn đi outbound 443 | **Phương án chính** |

Không thể cam kết giải pháp chỉ cần PHP shared host cho mọi tính năng realtime: shared host thông thường không bảo đảm daemon/WebSocket sống lâu. Cần một relay Go nhỏ trên VPS/container managed hoặc dịch vụ tương đương. Mọi TaskDeck cùng dùng relay, **không cấp VPS/tunnel riêng cho từng máy**.

## 3. Sơ đồ tin cậy và luồng dữ liệu

~~~text
 [Browser / Người dùng] --HTTPS--> [Management Portal PHP + SQLite3]
           |                                   |
           | WSS/HTTPS + ticket một lần         | phát hành quyền phiên
           v                                   v
     [Relay Gateway Go] <--xác minh assertion-- [Portal]
           ^
           | WSS outbound, tạo từ phía client
           |
     [TaskDeck Remote Agent / Go]
           |
           +-- Auth + RBAC + project/session isolation hiện có
           +-- HTTP APIs, WebSocket PTY/log, Patch, editor, files, Git
           +-- Credential và khóa riêng vẫn trên máy cục bộ
~~~

Ba miền tin cậy:
- **Portal (control plane):** danh tính người dùng Portal, sở hữu/ghép thiết bị, danh sách thiết bị, quyền mở kết nối, audit tối thiểu. Không vận chuyển dữ liệu terminal/file và không biết địa chỉ kết nối thật của từng TaskDeck.
- **Relay (data plane):** ghép và chuyển tiếp các kết nối đang hoạt động theo Device ID/session ID; lưu mapping động trong RAM có TTL. Relay có thể thấy IP tầng mạng trong lúc phục vụ kết nối, nhưng không được ghi IP vào hồ sơ dài hạn. Relay **không** được tự quyết định quyền thao tác TaskDeck.
- **TaskDeck (execution/auth authority):** định danh thiết bị bằng private key tại máy, xác thực tài khoản TaskDeck, kiểm tra quyền trên từng hành động, thực hiện lệnh. Không tin tuyệt đối vào dữ liệu do Portal/Relay/browser gửi.

Lưu ý: một host có thể chạy nhiều TaskDeck cho nhiều project. **Device ID phải đại diện cho một TaskDeck instance/project cụ thể**, không chỉ một hostname. Nếu muốn hiển thị nhóm máy vật lý, đó chỉ là metadata UI, không phải boundary bảo mật.

## 4. Device identity, onboarding, ghép nối

1. Khi bật Remote trong Settings, một TaskDeck instance tạo cặp khóa **Ed25519** bằng CSPRNG; private key lưu trong kho cục bộ, giới hạn quyền file/OS keychain nếu có, **không xuất lên Portal/Relay**.
2. Device ID có thể là chuỗi fingerprint theo dạng versioned: **SHA-256 của public key được mã hóa canonical**, bảo đảm phía khác có thể tự kiểm chứng. Kết hợp project/instance identity đã gắn với khóa, không dùng hostname làm định danh xác thực.
3. Agent cấu hình URL Portal và URL Relay **chung** (không phải endpoint riêng của mỗi thiết bị), kiểm tra TLS và đăng ký/claim Device ID.
4. Ghép nối phải có **mã ngẫu nhiên một lần, thời hạn ngắn**, hiển thị cục bộ và xác nhận từ cả Portal lẫn TaskDeck. Không cho một tài khoản bất kỳ chiếm tên/Device ID chưa ghép. Quyền sở hữu lần đầu và chuyển quyền sở hữu cần xác nhận trực tiếp tại TaskDeck hoặc quy trình recovery mạnh tương đương.
5. Khi báo online, TaskDeck chứng minh đang nắm private key bằng việc ký challenge nonce riêng, có timestamp, audience, Device ID và hạn dùng. Portal/Relay tính lại fingerprint từ public key và kiểm tra chữ ký; chống replay bằng nonce đã dùng/TTL.
6. Rotation key yêu cầu ghép lại hoặc xác nhận bằng khóa cũ; mất khóa cần recovery cục bộ, tuyệt đối không cho Portal tự gán khóa mới vào thiết bị cũ.
7. Không lưu bản ghi riêng chứa public key trong SQLite nếu giữ nguyên quy định chỉ lưu ID/tên. Public key được gửi kèm trong lần chứng minh quyền sở hữu và kiểm bằng fingerprint Device ID; cách này **không thay thế** quy trình ghép nối có xác nhận cục bộ.

## 5. Database và dữ liệu được phép lưu

Dự thảo schema (không phải migration):

| Bảng | Trường chính | Lưu ý |
| --- | --- | --- |
| users | id, username, password_hash/passkey metadata, status, MFA state | Chỉ thông tin auth của **Portal** |
| devices | device_id, display_name, status_enabled | Không có ip/port/hostname/endpoint/TaskDeck credentials |
| user_devices | user_id, device_id, allowed_role, granted_by, revoked_at | Grant phải được ràng buộc theo chính instance/project |
| audit_events | actor_id, event_type, target_device_id, time, outcome | Không log secret, nội dung terminal, IP, payload, đường dẫn nhạy cảm |
| web_sessions | id_hash, user_id, expires_at, csrf metadata | Session **Portal**, có TTL và revoke |

Challenge, ticket ID đã dùng, bảng mapping relay/agent và heartbeat state là **dữ liệu tạm thời** có TTL. Có thể dùng RAM hoặc ephemeral store; nếu cần chống replay trên nhiều replica phải có kho đồng bộ hỗ trợ consume-once nguyên tử. Không đặt chi tiết kết nối dài hạn vào bảng devices.

SQLite file phải ngoài public webroot; chuẩn bị backup, kiểm tra file locking trên shared host. **Không bật WAL trên network filesystem** khi không bảo đảm locking/memory-sharing theo tài liệu SQLite. Ngăn đọc trực tiếp các file .db, .db-wal, .db-shm, backup, debug và config qua web server.

Portal chỉ lưu tên/ID định danh của máy, nhưng **HTTP access log của shared hosting/Relay/CDN có thể vẫn ghi IP**. Chính sách privacy phải bao quát cả hệ thống hạ tầng, không được tuyên bố zero-IP-log khi chưa kiểm soát được.

## 6. Quy trình mở phiên end-to-end

1. User đăng nhập Portal qua HTTPS, có MFA hoặc passkey cho tài khoản có quyền remote thao tác, kiểm tra account/session/rate limit.
2. User chọn Device ID trong danh sách máy được cấp quyền; Portal kiểm quyền theo user-device-role và tính online từ tín hiệu có TTL.
3. Portal phát hành **signed short-lived assertion** cho một session, chứa tối thiểu: issuer, audience, user ID, Device ID, project/instance ID, max scope, issued-at, expiry (ví dụ 30 giây), random jti, purpose = remote-connect. Không bao gồm password/TaskDeck session token.
4. Browser thiết lập WSS tới Relay. Không truyền ticket trong query string hoặc URL có thể bị log/referrer; dùng kênh handshake bảo mật và consume-once. Relay kiểm chữ ký của Portal, audience, hạn dùng, jti, mapping Device ID và quyền hiện hành; TTL không thay thế kiểm tra thu hồi quyền.
5. Agent được kiểm tra độc lập bằng Device ID/challenge ký; nhận assertion đã xác minh và chỉ nhận kết nối cho chính instance/project được cấp. Không tin request từ browser dựa vào ID do người dùng tự nhập.
6. Kết nối browser↔TaskDeck được ràng buộc với session cụ thể; **TaskDeck vẫn yêu cầu xác thực của chính nó**, rồi kiểm quyền cho mỗi API/command/WS theo scope là giao của Portal grant và TaskDeck RBAC.
7. Khi logout, revoke grant, tắt Remote, đổi quyền, khóa thiết bị hoặc timeout: đóng các stream và ngăn request mới. Với phiên dài hạn, dùng push revocation và kiểm quyền định kỳ; outage control plane phải fail closed sau TTL lease hữu hạn.
8. Mất kết nối: reconnect dùng phiên mới/được ràng buộc lại với kiểm quyền; **không tự phát lại POST, terminal input, patch, file delete hoặc lệnh không idempotent** nếu chưa xác định trạng thái.

Đề nghị session ticket sống 30 giây, một lần; quyền điều khiển cần lease ngắn có tái xác minh. Các giá trị là cấu hình khởi tạo để kiểm thử, không phải hằng số giao thức bất biến.

### 6.1. Điều kiện "truy cập giống trực tiếp"

Frontend TaskDeck có các API HTTP và WebSocket, nên phải giải quyết:
- Public origin riêng, an toàn cho mỗi session, ví dụ subdomain ngẫu nhiên dưới wildcard TLS; không dùng Device ID công khai làm host, không cho cookie/localStorage giữa các máy chia sẻ cùng origin.
- Reverse proxy đúng method/status/header và stream body (upload/download lớn); hỗ trợ WebSocket upgrade, ping/pong, close, backpressure, binary data, resize PTY, log streaming.
- Chuyển đổi Host/Origin/Cookie/Location/CSP/CSRF đúng mô hình bảo mật; **không disable same-origin, CSRF, HttpOnly, Secure, SameSite hay sharedAuth** để cho proxy hoạt động.
- Nếu URL, static asset, absolute path hoặc service worker không hoạt động qua origin mới, xây một lớp proxy adapter phù hợp, không mở proxy tùy ý tới localhost/private networks.
- Tách Portal session khỏi TaskDeck session. Không forward cookie đăng nhập Portal cho backend TaskDeck; không lưu TaskDeck credential ở Relay.
- Tương tác cùng tồn tại nhiều tab/nhiều project: ownership của terminal và Patch lock phải nhất quán với local UI.

Điều này phải được xác thực bằng test tích hợp, không chỉ nhìn thấy dashboard.

## 7. Mã hóa, xác thực và giới hạn trust

- Bắt buộc HTTPS/WSS TLS 1.2+ (ưu tiên TLS 1.3) giữa mọi hop; TLS verification nghiêm ngặt, không chấp nhận insecure-skip-verify như giải pháp sản xuất.
- **Mục tiêu production:** E2EE giữa browser và TaskDeck bằng khóa phiên được xác thực với device identity đã ghép; Relay chỉ vận chuyển ciphertext, không có plaintext password/terminal/file. Dùng giao thức chuẩn đã được review (authenticated ECDH + AEAD và chống replay/đổi khóa); tránh tự phát minh crypto.
- **Ngoại lệ cần ghi rõ:** nếu chỉ dùng reverse proxy TLS thông thường, Relay giải mã được dữ liệu. Không gọi đó là E2EE và không được tuyên bố Relay không thể đọc dữ liệu.
- **Giới hạn E2EE browser:** JavaScript do Portal/Relay phục vụ có thể bị thay đổi nếu origin bị xâm nhập; E2EE không ngăn JS độc hại đánh cắp input. Nên giảm trust frontend bằng CSP mạnh, phân tách origin, integrity/deploy controls, MFA/passkey, và cân nhắc client được đóng gói/pinned code khi threat model cao.
- Khoá ký Portal là bí mật **của Portal**, khác hoàn toàn private key và thông tin xác thực TaskDeck; phải lưu ngoài webroot và rotate được.
- Việc cấp quyền Portal không bao giờ biến thành đặc quyền thực thi lệnh TaskDeck.

## 8. Tích hợp với TaskDeck hiện tại

- Tích hợp Remote Agent thành module Go cạnh tầng server/session; không fork một bản frontend thứ hai.
- Tái sử dụng Auth/RBAC, project-bound browser session, Origin validation, PTY ownership, patch mutation locks, audit, self-update handling hiện có.
- Settings → Remote Access: bật/tắt (OFF mặc định), URL Portal/Relay chung, Device ID, tên instance, trạng thái online, ghép/hủy ghép, thu hồi mọi remote session, kiểm tra TLS/diagnostic.
- Không thay đổi cơ chế auth hiện có để thích ứng với một public proxy. Nếu mode chưa bắt buộc auth, chặn Remote và hướng dẫn bật auth trước.
- Tách adapter HTTP/WebSocket tunnel khỏi business logic, để cùng handler/API được dùng khi local hoặc remote.
- Tránh làm mất chức năng Linux/macOS, desktop/mobile browser, Patch Tool/Collection, log viewer.
- Không sửa behavior mặc định của local access hoặc âm thầm bật Remote sau self-update, migration, restore/backup.

## 9. Tính ổn định/vận hành

- Agent chủ động nối outbound TCP/443; heartbeat gợi ý 20 giây, online TTL gợi ý 60 giây; reconnect exponential backoff + jitter, giới hạn kết nối đồng thời.
- Relay hỗ trợ proxy streaming/flow-control, quotas bandwidth, size/rate per device/user, giới hạn buffer; không lưu payload/file trong RAM vô hạn.
- Giới hạn tài nguyên shared host PHP: request ngắn, SQLite transaction ngắn, tránh chạy WebSocket daemon/long polling diện rộng trên shared hosting.
- Relay restart: session có thể mất; TaskDeck tác vụ đang chạy không được tự động chạy lại. Phải định nghĩa resume/re-attach an toàn cho PTY, upload, Patch.
- Availability phụ thuộc Portal (quyền), Relay (đường truyền), outbound Internet (client). Local TaskDeck **vẫn hoạt động** khi Portal/Relay lỗi.
- Dù DB không lưu IP, bất kỳ relay nào vẫn bắt buộc biết địa chỉ transport trong RAM để chuyển packet. Đây không phải "kết nối ẩn hoàn toàn" ở tầng mạng.
- Nếu muốn dùng nhiều Relay, cần cơ chế rendezvous/registry đồng bộ tạm thời, token consumption nguyên tử, revocation propagation và tránh sticky-session mơ hồ.

## 10. Giai đoạn thực hiện

1. **Portal/enrollment:** SQLite3, auth/MFA, RBAC, local approval pairing, device attestation, heartbeat, audit.
2. **Relay/Agent:** outbound WSS, session ticket, identity verification, revocation, diagnostics, security tests; chưa công bố public remote.
3. **Transparent remote UI:** HTTP + WebSocket proxy adapter theo cùng auth/RBAC, streaming file/PTY/Patch, origin & cookie isolation, multi-project.
4. **Security hardening:** E2EE/JS trust model, pentest, misuse/DoS tests, key rotation, backup/recovery, deploy/cutover.
5. **Optional:** WebRTC P2P + TURN fallback khi đã có baseline ổn định.

## 11. Acceptance checklist bắt buộc

- [ ] Portal chạy trên shared hosting PHP + SQLite3 (không yêu cầu WebSocket daemon trên host PHP).
- [ ] Client sau NAT/CGNAT/firewall outbound-only có thể kết nối và làm việc đầy đủ.
- [ ] SQLite không có TaskDeck secret/IP/endpoint; không có token TaskDeck trong PHP logs, Relay logs hay browser storage Portal.
- [ ] Không thể remote khi TaskDeck không bật authentication; không thể bypass bằng API/WS.
- [ ] Device ID được xác nhận bằng private-key proof + local enrollment, chống replay và takeover.
- [ ] Portal viewer không thể tăng quyền dù TaskDeck account cấp cao; kiểm quyền ở API và WS từng message.
- [ ] Revoke user/device có hiệu lực lên phiên đang mở trong lease tối đa xác định.
- [ ] Hai project/TaskDeck instance không thể dùng chung cookie, ticket, PTY hay file access.
- [ ] Terminal interactive, log WebSocket, Patch, editor, upload/download lớn, Git, self-update được smoke trên remote origin.
- [ ] Reconnect không lặp lại thao tác không idempotent, không làm sai mutation lock.
- [ ] Relay bị quan sát không thấy plaintext khi phát hành tính năng **E2EE**; nếu chưa E2EE phải đánh dấu rõ không đáp ứng yêu cầu này.
- [ ] Local TaskDeck vẫn hoạt động bình thường khi Portal/Relay lỗi; remote mặc định tắt.
- [ ] Đã hoàn thành tất cả P0/P1 security gates của tài liệu đánh giá bảo mật.

## 12. Tham chiếu

- TaskDeck current security baseline: [README.md](../README.md), [shared_auth.go](../vscode_tasks_menu_go/internal/server/shared_auth.go), [SHARED_SERVER_ACCEPTANCE.md](../vscode_tasks_menu_go/SHARED_SERVER_ACCEPTANCE.md).
- OWASP WebSocket Security Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html
- OWASP Session Management Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html
- NIST SP 800-63B-4 (July 2025): https://csrc.nist.gov/pubs/sp/800/63/b/4/final
- SQLite WAL limitations: https://sqlite.org/wal.html

**Nguyên tắc cuối:** Portal cho phép kết nối; chỉ TaskDeck mới quyết định người đó có quyền thực hiện hành động gì. Mọi ngoại lệ làm mất bảo vệ này phải bị từ chối.
