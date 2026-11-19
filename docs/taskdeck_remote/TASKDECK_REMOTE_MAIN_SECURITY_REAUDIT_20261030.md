# TaskDeck Remote — kiểm toán lại 3 điều kiện bảo mật trên main (30/10/2026)

**HEAD kiểm tra:** [`70a06b246eb87ba3b70994832a013bba9a9672f9`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/70a06b246eb87ba3b70994832a013bba9a9672f9).

**Phạm vi duy nhất:** (1) PHP bị chiếm quyền SAU lần ghép đôi đáng tin cậy,
(2) dữ liệu TaskDeck đi qua PHP/Relay có bị đọc trộm hay không,
(3) có thể bỏ qua TaskDeck local login để thực thi lệnh trái phép hay không.
Không đánh giá hay triển khai tính năng ngoài phạm vi này. Không suy luận
"đạt production" từ việc kiểm thử thành công một thao tác chẩn đoán.

## Kết quả mã nguồn và kiểm thử

| Điều kiện | Luồng đã kiểm tra và bằng chứng | Kết luận |
| --- | --- | --- |
| **1. PHP bị hack không truy cập TaskDeck** | PHP signing seed / giả mạo `admin` / `allowed=true`, Portal ticket và lease không tạo ra phiên TaskDeck; Agent pin Ed25519 riêng; client phải chứng minh possession Ed25519 private key trên **E2EE transcript cụ thể** đã được host-local CLI duyệt; TaskDeck tự kiểm tra SQLite account/password, project membership và RBAC. `TestRemoteCompromisedPHPPortalWithSigningKeyCannotAuthenticateOverRealRelay`, `TestCompromisedPHPAdminTicketNeverAuthenticatesTaskDeckHTTP`, `TestCompromisedPortalCannotExecuteWithPasswordOnlyOrCopiedPublicClientKey`. | **PASS trong phạm vi đăng nhập + `session.ping` có mã nguồn/test.** |
| **2. PHP không đọc được dữ liệu TaskDeck** | PHP Portal chỉ giữ account/MFA của **Portal**, signed ticket/lease, grant, presence; app frame chạy qua **Go Relay WSS riêng** dưới ciphertext AES-256-GCM Browser→Agent từ P-256 ECDH đã được Agent Ed25519 ký và client ghim. Mật khẩu TaskDeck và `session.ping` request/response không có dạng plaintext trong PHP hoặc Relay. Strict frame parser, anti-replay sequence, AEAD poison; `TestRemoteLoginEncryptedCodecRoundtripAndNoSessionToken`, `TestRealWSSAppPingWithCompromisedPHPStillRequiresLocalTaskDeckLogin`, `remote_portal/tests/offline_e2ee_crypto.test.cjs`. | **PASS với dữ liệu TaskDeck thực sự đã triển khai.** Không bao gồm dữ liệu tài khoản **Portal**, Device ID, public keys, IP, timing, kích cỡ traffic và tính sẵn sàng — PHP bị hack vẫn biết/chi phối các metadata này. |
| **3. Không bypass TaskDeck login để thực thi lệnh** | `StartEncryptedRemoteSession` yêu cầu login v1 ứng dụng **được gắn loại ngay bên trong AEAD**, không thể nâng từ diagnostic login bằng cách sửa outer JSON; trả `authenticated=true` mới được Agent phát chữ ký `app_ready`. TaskDeck giữ principal **trong tiến trình**, từng action tái kiểm tra host-approved client key, mật khẩu hiện tại, account, membership, Portal grant, RBAC, timeout/cancel/revocation. `OpenRemoteAction` chấp nhận **duy nhất `session.ping`**, từ chối shell/command/path/File/PTY/HTTP. `TestCompromisedRelayCannotPromoteDiagnosticE2EELoginToAppSession`, `TestRemoteActionExecutionRejectsPortalOnlyAndUnauthorizedActions`, `TestRealWSSAppPingWithCompromisedPHPStillRequiresLocalTaskDeckLogin`. | **PASS với ping và thử tấn công lệnh giả đã kiểm thử.** Không có production command dispatcher để chứng minh an toàn cho các chức năng thực tế. |

**CI trên chính main source SHA:**
- [Go Agent/Relay #41044342534](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41044342534): **completed/success**.
- [PHP Portal/E2EE #41044342522](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41044342522): **completed/success**.

## Ranh giới trust bắt buộc

- Chỉ tin client đã nhận từ nguồn độc lập, đã ghim Agent Device ID/key sau lần đầu
  tin cậy; **không nhập mật khẩu TaskDeck trong trang HTML/JS do PHP hiện tại phục vụ**.
  PHP bị hack có thể thay mã HTML và lừa người dùng đưa mật khẩu **trước khi E2EE**;
  không có thuật toán mã hóa nào ngăn phishing trên UI do attacker kiểm soát.
- Không có inbound port TaskDeck cần public/forward. Agent kết nối **outbound**.
  Shared PHP hosting không thay thế dịch vụ Go Relay WSS dài hạn.
- `remote_control_available=false` phải giữ nguyên; `TASKDECK_REMOTE_LOGIN_PROBE`,
  `TASKDECK_REMOTE_APP_PING`, Relay `-allow-app-ping` **mặc định OFF**.
  Mở thử nghiệm `session.ping` **không** kích hoạt Terminal, file, Git, Patch,
  collection, arbitrary HTTP/WS hoặc chạy lệnh hệ điều hành.

## Quyết định phát hành

**REMOTE CONTROL LIVE: NO-GO — không tag release, không deploy, không bật điều khiển.**

Cả ba điều kiện **đạt đối với các loại bản tin thực sự được triển khai**
(login/readonly `session.ping`), nhưng **không thể kết luận cả ba đạt đối với
Remote Control dùng thật** khi ứng dụng **chưa có đường truyền, giải mã,
kiểm tra quyền và thực thi** cho Terminal/Files/Git/Patch.
Kiểm thử từ chối lệnh giả **không** chứng minh rằng một dispatcher lệnh
production (chưa tồn tại) an toàn. Không biến kết quả PASS về bảo mật chẩn đoán
thành giấy phép release. Không có bằng chứng kiểm thử triển khai Internet thật
trên shared PHP/VPS/host/client theo đúng cấu hình phát hành.

**Không mở rộng trong lần rà soát này.** Chỉ xác nhận ba bất biến,
quyết định chưa đủ điều kiện live và dừng.

## Remote read-only integration verification — 2026-10-30

**Exact tested source:** [`2b747a93c9b53988a0bb51c2e62bfad9538a8463`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/2b747a93c9b53988a0bb51c2e62bfad9538a8463).
- [Go Agent/Relay CI #41048405356](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41048405356): **COMPLETED / SUCCESS**, including real WSS/TLS encrypted workspace text read, compromised-PHP signed-ticket scenario, TaskDeck `FilesRead` authorization, local owner-approved client proof, duplicate/forged action JSON, replay rejection, symlink escape and wrong-response/capability-forgery denial, Go race/security tests and builds.
- [PHP Portal CI #41048405613](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41048405613): **COMPLETED / SUCCESS**, same code SHA.

**Function now demonstrably implemented:** opt-in native trusted client `-read-file=workspace-relative/path.txt` using signed Agent + AEAD, host's in-process TaskDeck principal, local `FilesRead` and audit, <=192 bytes UTF-8, no PHP plaintext. This is **not** a generic TaskDeck HTTP proxy, file browser, write/editor, shell or long-lived terminal.

**Three-invariant conclusion:** PHP/Relay cannot, by merely controlling PHP signing/account/grants or forged socket messages, obtain this file's content or execute a command in the covered integration tests. The additional read path is bounded, host RBAC-checked and encrypted. **This narrow PASS is NOT proof for the missing production command and data flows.**

**LIVE NO-GO:** Terminal/session control, file transfer/edit/write, Git/Patch/Collection, browser Remote UI/stream multiplexing, release-binary provenance validation and actual external deployment/penetration tests remain absent. No production Remote Control release/tag/server deployment has been performed. `remote_control_available=false`; opt-in dev flags default OFF. Do not claim completion merely from a green CI on a preview operation.
