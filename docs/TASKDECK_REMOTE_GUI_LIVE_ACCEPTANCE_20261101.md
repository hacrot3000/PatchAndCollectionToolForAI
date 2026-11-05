# TaskDeck Remote GUI — Nghiệm thu bảo mật LIVE trên Linux/macOS

**Ngày lập:** 2026-11-01 · **Trạng thái:** NO-GO / CHƯA NGHIỆM THU THỰC ĐỊA

## Mục tiêu và nguyên tắc bất biến

1. **PHP/Relay bị chiếm quyền sau lần ghim danh tính Agent tin cậy** không được tự đăng nhập TaskDeck, thêm key, vượt quyền, thực thi task/Git hay lấy token/credential.
2. PHP/Relay không được giải mã hoặc thay đổi có hiệu lực nội dung E2EE. Mã GUI, JavaScript và hộp nhập mật khẩu đến từ **native client được xác minh**, tuyệt đối không phục vụ qua PHP.
3. Đăng nhập TaskDeck, Ed25519 key được host duyệt, RBAC + Portal grant + host opt-in, audit và hạn mức phiên phải được thực thi lại tại **Agent** cho từng thao tác.
4. `remote_control_available=false` duy trì đến khi toàn bộ mục P0 PASS, có evidence thực địa và phê duyệt bảo mật độc lập.

## Trạng thái CI có thể kiểm chứng

- Trusted GUI baseline: PR #54, Go/PHP CI và kiểm thử GUI trên runner Linux/macOS PASS trước khi merge. Chỉ chứng minh mã nguồn và test tự động, **không chứng minh thao tác ngoài thực địa**.
- Git stage/unstage GUI: PR #55 đang có CI riêng; chỉ cập nhật PASS khi commit cuối đã được kiểm chứng.
- Không dùng kết quả mô phỏng CLI/runner thay cho bài kiểm thử giữa **hai thiết bị thực và mạng Internet không công bố inbound port**.

## Ma trận thử nghiệm thực địa bắt buộc (chưa PASS)

| ID | Nơi thực hiện | Kịch bản | Kết quả PASS bắt buộc | Hiện trạng |
| --- | --- | --- | --- | --- |
| L01 | Linux Fedora + browser | Native `--gui`, ghim Agent Device ID đúng, client key chưa duyệt | Không hiện mật khẩu trước proof, chỉ hiện public key; Portal không thể tự thêm key | PENDING |
| L02 | Linux Fedora + browser | Duyệt Files, Git status/diff, task host-approved | Mỗi action có E2EE, RBAC, audit, hạn mức; kết quả text-only, không thể inject HTML | PENDING |
| M01 | macOS + browser | Native `--gui` từ binary xác minh độc lập PHP | GUI chỉ từ loopback, JS embedded, Origin/CSRF/CSP không bị bypass | PENDING |
| M02 | macOS + browser | Login sai/revoked key, người dùng TaskDeck bị disable | Không truy cập GUI có quyền; không nhận principal từ PHP/ticket | PENDING |
| N01 | Linux + macOS | Ngắt Wi-Fi hoặc kill Relay khi đang chờ E2EE Git review | Thao tác báo thất bại; không retry thực thi ghi ngầm, cần phiên xác thực mới | PENDING |
| N02 | Linux + macOS | Ngắt mạng ngay giữa task managed/đọc output | Watchdog dừng task theo chính sách; output cursor cũ không được dùng qua phiên mới | PENDING |
| N03 | Linux + macOS | Tắt browser/ấn Đóng phiên, thử gửi POST cùng token cũ | Phiên native/Agent bị đóng; không thể tái sử dụng token/cookie cho TaskDeck | PENDING |
| R01 | Host Linux/macOS | Thu hồi client key, đổi mật khẩu, thu hồi Portal grant trong phiên | Hành động kế tiếp bị từ chối; task đang chạy bị kết thúc theo policy | PENDING |
| G01 | Host + GUI | Stage tệp đã được preview, kiểm tra SHA/index thay đổi | Chỉ ghi đúng file đã duyệt, SHA-CAS; khi sửa ngoài trước xác nhận phải từ chối | PENDING |
| G02 | Host + GUI | Unstage với staged diff trống/bị đổi/sai quyền | Không ghi index; lượt E2EE đã phát phải bị trừ; fail closed | PENDING |
| G03 | Host + GUI | GitWrite/FilesRead/FilesWrite chưa bật hoặc RBAC viewer | Không có stage/unstage, không được chạy Git argv hoặc hook bất kỳ | PENDING |
| T01 | Host + GUI | Chạy task có full approval token, sai task ID hoặc sửa tasks.json | Chỉ task exact digest được chạy; không có arbitrary shell/PTY | PENDING |
| T02 | Host + GUI | Hủy task, revoke khi chạy, kiểm tra tiến trình con | Không trả trạng thái "đã hủy" giả; phải xác nhận STOPPED trên host | PENDING |
| P01 | PHP/Relay giả lập bị hack sau khi ghim | Thay HTML/JS Portal, fake response, CSRF/DNS rebinding, fake grant | Không lấy được password/E2EE plaintext, không mở lệnh TaskDeck, browser không dùng mã PHP | PENDING |
| P02 | PHP/Relay giả lập bị hack sau khi ghim | Replay/đảo thứ tự ciphertext, ngắt nối lại và dùng token cũ | AEAD/replay/session principal bị từ chối, không có side effect trùng | PENDING |
| X01 | Linux + macOS | Binary native download/update bị PHP thay thế | Người dùng chỉ dùng binary chữ ký/provenance độc lập; binary giả không được chấp nhận | PENDING |

## Thu thập evidence và fail criteria

Ghi lại OS, kiến trúc, build SHA, GitHub Actions URL, thiết bị Agent/Client (dùng alias), loại mạng (CGNAT/NAT), thời gian UTC, cờ host, thao tác, trạng thái process/task thực, kết quả Agent audit và expected vs actual. Tránh đưa mật khẩu, private seed, client token, PIN và ciphertext có thể định danh vào bản chia sẻ công khai.

**FAIL/NO-GO tức thời** nếu có bất kỳ trường hợp PHP tự thêm client key, HTML Portal yêu cầu mật khẩu TaskDeck, lệnh Git/task chạy không có RBAC, mất kết nối nhưng side effect bị replay, session/token cũ được dùng sau revoke, hay dữ liệu E2EE bị lộ. Khi lỗi xảy ra, giữ Remote OFF và tạo issue kèm evidence đã làm sạch.

## Quyết định release

- **CI tự động:** Có thể PASS riêng từng PR.
- **Thực địa Linux:** PENDING.
- **Thực địa macOS:** PENDING.
- **Compromised PHP/Relay với hạ tầng thật:** PENDING.
- **Người phê duyệt bảo mật độc lập:** PENDING.
- **Remote LIVE:** **NO-GO**; không bật `remote_control_available`, không ngầm tăng quyền Git/task.
