# TaskDeck Remote — Tiêu chí lên live khi PHP Portal bị chiếm quyền

Cập nhật: 2026-10-29. Branch: `feature/taskdeck-remote-access-architecture`.

## Định nghĩa phạm vi

Giả định đối thủ đã chiếm quyền **toàn bộ** shared hosting PHP:
- Đọc/thay PHP, HTML, JS, SQLite, password hash và khóa ký ticket/lease của **Portal**.
- Tự cấp Portal role `admin`, ticket, grant, lease, bỏ qua MFA, và giả lập mọi phản hồi `{"ok":true,"allowed":true}`.
- Sử dụng Relay từ Internet, tạo kết nối WSS và liên tục thử mật khẩu TaskDeck.
- **Không** sở hữu private key Agent Ed25519 trên máy TaskDeck, không điều khiển host TaskDeck, không biết mật khẩu TaskDeck và không điều khiển nguồn phân phối client đáng tin cậy.

**Phân biệt bắt buộc:** Đối thủ có khóa ký Portal **có thể** thiết lập một kết nối truyền tải/diagnostic đến Agent nếu biết Device ID và ticket hợp lệ. Họ **không** được tạo phiên **đã xác thực TaskDeck** hoặc gọi bất cứ chức năng TaskDeck nào chỉ nhờ PHP, không có mật khẩu hợp lệ và local RBAC. Vì vậy không được hứa “không có TCP/WebSocket/E2EE handshake nào”; lời hứa bảo mật ở đây là **không có phiên sử dụng TaskDeck**.

## Bảng quyết định

| Điều kiện | Kiểm chứng hiện tại | Kết luận |
|---|---|---|
| PHP không có mật khẩu TaskDeck, hash/salt, cookie hoặc private key Agent | PHP lưu tài khoản **Portal**, SQLite grants và ký ticket/lease; password TaskDeck chỉ giải mã phía TaskDeck qua E2EE | PASS về ranh giới mã nguồn hiện tại, không bảo đảm khi người dùng bị phishing |
| Ticket admin, khóa ký Portal, phản hồi `allowed=true` không vượt scrypt TaskDeck | Unit + Go TLS/WSS E2E với Portal ký giả, mật khẩu sai và Agent gọi verifier TaskDeck thật | PASS về khả năng vượt kiểm tra mật khẩu |
| PHP không thể giả mạo kết nối Agent thật | Relay challenge-bound Ed25519 proof cần **private key trên TaskDeck**; real WSS test với lease Portal ký đúng nhưng key Agent của hacker | PASS |
| Client không nhận mã JS hoặc Device ID giả từ PHP để nhập mật khẩu | CLI độc lập, password sau khi xác thực AEAD/Agent, yêu cầu 0600 out-of-band Device ID pin | PASS cho client mẫu; **chưa có phân phối binary đã ký/verified** |
| Chống đoán mật khẩu không giới hạn khi PHP đã bị hack | Giới hạn bền vững per-Device ID trước scrypt + giới hạn đăng nhập TaskDeck và ticket/Relay quotas | PASS trong phạm vi giới hạn code hiện tại; vẫn có thể DoS hoặc lừa nhập password |
| Thu hồi quyền và RBAC trong một phiên hành động | Backend nội bộ có local checks và live introspection; dispatcher chưa được nối vào đường thực thi | CHƯA XÁC NHẬN cho Remote Control |
| Remote Control thật qua Internet, file, terminal, Git, Patch, HTTP | `remote_control_available=false`, không có app dispatcher | **CHƯA TRIỂN KHAI** |

## GO/NO-GO minh bạch

- **Có thể chuẩn bị pilot Portal quản trị/presence + thử nghiệm chẩn đoán E2EE/login-only**, nếu owner bảo vệ config/Agent seed, xác minh nguồn client, pin Device ID ngoài PHP và triển khai TLS/Relay quotas. Phải thử trên môi trường triển khai thực, backup và rollback. Đây chưa phải chấp thuận vận hành public production.
- **NO-GO cho phiên điều khiển TaskDeck từ xa** ở trạng thái mã nguồn hiện tại. Các kiểm thử thông qua không biến module thực thi còn thiếu thành tính năng. Chỉ khi dispatcher với local TaskDeck login, account/project RBAC cho từng hành động, revocation/cancellation và bounded stream + trusted client UX hoàn tất mới đổi trạng thái.
- **Giả định không thể bảo vệ:** PHP bị hacker chỉnh sửa có thể hiển thị form giả, link tải client giả, chỉ người dùng pin nhầm Device ID hoặc tự nhập mật khẩu vào trang PHP độc hại mới làm lộ mật khẩu. Không thể khẳng định tuyệt đối hacker *không bao giờ* đăng nhập nếu hacker đã đánh cắp được mật khẩu; khi đó cần đổi mật khẩu/revoke từ máy TaskDeck độc lập, không qua PHP.

## Kiểm thử đã bổ sung

- `internal/server/remote_compromised_portal_relay_test.go` — một TLS host cung cấp cả PHP API bị chiếm và Relay WSS thật, Portal liên tục phản hồi `allowed=true`, cấp lease, ký ticket admin bất kỳ. Agent dùng `VerifyEncryptedRemoteLogin` thật trên TaskDeck Shared Identity, mật khẩu sai không thể trả `authenticated=true`, browser không được phép tái sử dụng channel.
- `internal/remoterelay/compromised_portal_device_test.go` — kẻ có private key Portal ký lease hợp lệ nhưng ký proof bằng private key Agent giả; Relay không đăng ký được dưới Device ID của máy thật.

**Bằng chứng CI:** [Go Remote Agent run 40806204975](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40806204975) — **PASS** bài E2E TLS/WSS có Portal bị chiếm khóa ký, kiểm tra mật khẩu TaskDeck bằng identity database thật. [Go Remote Agent run 40806726641](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40806726641) — **PASS** kiểm thử Agent giả với lease do Portal ký, Go race tests và build đa nền tảng. [PHP Portal run 40805004482](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40805004482) — **PASS** suite Portal hiện có. Chưa chạy penetration test trên máy chủ live cụ thể.

**Tài liệu này phân biệt kết quả đã được test với điều kiện lên live chưa đủ; không được tự động bật Remote Control chỉ vì các bài kiểm thử mật khẩu đều PASS.**
