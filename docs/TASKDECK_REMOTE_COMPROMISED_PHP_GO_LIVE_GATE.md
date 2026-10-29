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

**Tiến độ mới — lifecycle hành động:** code nội bộ đã có admission theo phiên E2EE/mật khẩu TaskDeck thật, giới hạn một tác vụ/phiên, timeout 15 giây, kiểm tra quyền Portal + RBAC/password local trước và sau hành động, hủy khi session/AgentGrant/E2EE bị đóng hoặc ciphertext bị sửa/replay. Các chức năng Terminal/File/Git/Patch/HTTP vẫn chưa được gắn vào gateway này. Không có khả năng hủy/rollback side effect không tuân theo `context`; chưa có dispatcher stream bảo mật hoặc phân phối client có chữ ký. **Kết luận Remote Control vẫn NO-GO** mặc dù lớp xác thực độc lập với PHP đã được kiểm thử.

**Tài liệu này phân biệt kết quả đã được test với điều kiện lên live chưa đủ; không được tự động bật Remote Control chỉ vì các bài kiểm thử mật khẩu đều PASS.**


**Milestone giao thức E2EE (2026-10-29):** Đã thêm codec JSON mã hóa với đúng **một** operation `session.ping`, từ chối/đóng channel khi nhận payload Terminal/File/Git/Patch/HTTP, khóa JSON trùng hoặc mã response chứa token. Bộ thực thi thử nghiệm `executeRemoteE2EEPing` trong TaskDeck đòi **phiên do mật khẩu TaskDeck thực tạo ra**, TaskDeck `tasks.view` và Portal live-check trước/sau tác vụ. Audit `remote.action.session_ping` ghi tại TaskDeck bằng user/project thật, không dùng log của PHP. **Chưa gắn WebSocket action route / chưa có long-lived client session**, nên milestone này KHÔNG đồng nghĩa với mở Remote Control. Remote Control tiếp tục NO-GO.


**CI:** [Go Remote Agent run 40816666503](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40816666503) — **PASS** với commit `afa47ff43dc1d52cb6662b065a41f9078183cefb`, bao gồm kiểm thử action E2EE chỉ cho phép `session.ping`, xác thực TaskDeck thực, RBAC hai lần và audit TaskDeck. Đây là kiểm thử nội bộ; trạng thái Remote Control vẫn **NO-GO**.

**Bằng chứng mới về phiên dài hạn:** [GitHub Actions 40818162152](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40818162152) **PASS** (source `87900fb69a662a025c780bec2f6aecbc43afa4f5`). Xác nhận: Portal bị chiếm khóa ký và tự cấp `admin` vẫn không tạo được phiên khi sai mật khẩu TaskDeck; chỉ đúng mật khẩu và đủ quyền cục bộ mới tạo ra phiên E2EE tại máy TaskDeck. Phiên tự hủy khi revoke/disconnect/expire và không giữ quyền sau đổi hash mật khẩu cục bộ (kể cả timestamp bị backdate). Chưa kết nối native client + Relay action loop, vẫn **NO-GO cho Remote Control public**.


**Bổ sung kiểm thử PHP trì hoãn phản hồi (2026-10-29):** Sau khi đã kiểm tra đúng mật khẩu TaskDeck cục bộ, Portal bị hack có thể cố tình giữ request `grant-check` thứ hai, đợi quản trị viên đổi mật khẩu hoặc gỡ quyền tại máy TaskDeck, rồi trả `allowed=true`. Đã thêm bước kiểm chứng lại mật khẩu/hash phiên bản, trạng thái người dùng và membership **tại TaskDeck sau HTTP** và trước khi phát phản hồi E2EE đăng nhập. Test `TestRemoteStagedLoginDeniesPasswordRotationDuringLastPortalGrantCheck` kiểm tra race này bằng barrier đồng bộ, không dựa vào sleep. Điều này không cho phép Portal tự tạo session/credential và **không bật Remote Control**. Cần xác nhận CI riêng cho source commit `7a789e881e16fc29182c12fdd16de7e709dc8fc2` trước khi đánh dấu PASS.


**Bổ sung hồi quy membership:** `TestRemoteStagedLoginDeniesTaskDeckMembershipRemovalDuringLastPortalCheck` dùng cùng barrier HTTPS, vô hiệu hóa membership dự án trong identity database của TaskDeck trước khi PHP trả `allowed=true`, yêu cầu không phát sinh phiên/boolean thành công và hủy grant/E2EE. Tình huống này xác nhận Portal admin bị hack không thể khôi phục quyền dự án đã thu hồi tại TaskDeck. Mã kiểm thử tại commit `c7365e13fef67964b9845242a37f578c5b8c5e3f`; vẫn cần CI run cho đúng mã trước khi kết luận PASS.

**Xác nhận CI cho hồi quy PHP bị hack:** [Remote Agent run 40819494945](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40819494945) đã **SUCCESS** cho source commit `c7365e13fef67964b9845242a37f578c5b8c5e3f`, bao gồm cả trường hợp password và membership thay đổi ngay trong lúc Portal trả `allowed=true`. Lượt kiểm thử cho bản ổn định thêm synchronization khi hết hạn session (`3db6fdbcc90c72b252160544676554a336d0a175`) có run độc lập [40819715312](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40819715312); không được coi PASS trước khi run này hoàn tất. **Remote Control vẫn NO-GO** vì các đường Internet dispatcher/stream chưa được kết nối và kiểm định.


**CI cuối cùng (29/10/2026):** [TaskDeck Remote Agent run 40819715312](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40819715312) **SUCCESS** cho source commit `3db6fdbcc90c72b252160544676554a336d0a175`, gồm kiểm thử race Agent/Relay/transport, kiểm thử TaskDeck server với local password và membership bị thay đổi khi PHP trả grant-check muộn, hồi quy cleanup khi session hết hạn, build CLI/Relay và cross-compile macOS/Linux ARM. Các commit tài liệu sau đó không làm thay đổi source đã kiểm chứng. **Kết luận chỉ cho ranh giới xác thực**: Portal bị chiếm quyền và có khóa ký/ticket admin nhưng không có mật khẩu TaskDeck thì không thể tạo phiên ứng dụng được TaskDeck chấp nhận. Trường hợp người dùng bị phishing lộ mật khẩu hoặc máy chạy TaskDeck bị chiếm quyền nằm ngoài bảo đảm này. Không có public Remote Control dispatcher, vì vậy vẫn **NO-GO** cho triển khai phiên điều khiển Internet.


### Trusted-client forged Relay response acceptance gate

Native login client must reject **all** duplicate, unknown, over-limit and trailing Relay JSON keys; The decoder must also reject non-canonical casing (for example `Ticket_Valid`) and JSON escape aliases for the same logical key. in Agent-proof/login-result envelopes. Before invoking the password TTY prompt, it must confirm both the out-of-band Device ID pin and separate signed Agent reachability+ECDH proofs, including the exact nonce, then authenticate the ready AES-GCM record. Password and application data must never be accepted from or posted to PHP. The regression suite in `cmd/taskdeck_remote_login/main_test.go` now covers these boundaries. The login-result envelope cannot smuggle a `bearer`, token, `remote_control_available=true` or conflicting duplicate field. This strengthens parsing but does **not** make a PHP-served password form trustworthy or enable Remote Control.
