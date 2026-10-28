# TaskDeck Remote Access — Đánh giá bảo mật, rủi ro và điều kiện phát hành

> Ngày: 2026-10-28. Trạng thái: ĐÁNH GIÁ TRƯỚC TRIỂN KHAI (threat model / security design review), **không phải** kết quả pentest hay kiểm thử mã nguồn Remote đã hoàn tất.
>
> Tài liệu liên quan: [TASKDECK_REMOTE_ACCESS_ARCHITECTURE.md](TASKDECK_REMOTE_ACCESS_ARCHITECTURE.md).
>
> Tài liệu này mô tả rủi ro cần ưu tiên và tiêu chí kiểm chứng. Chưa có tính năng Remote production nào được xác minh trong repo tại thời điểm lập thiết kế.

## 1. Phạm vi, tài sản cần bảo vệ và đánh giá tổng quan

Mô hình mục tiêu: Portal PHP/SQLite3 trên shared hosting quản lý tài khoản, Device ID/tên và quyền kết nối; một Go Relay xử lý kênh truyền thời gian thực; Remote Agent tích hợp TaskDeck kết nối outbound tới Relay. Trình duyệt dùng đầy đủ chức năng TaskDeck mà không cần public IP/forward port trên client.

**Mức độ hệ quả nếu bị xâm nhập: RẤT CAO.** TaskDeck có terminal, sửa file, Git, Patch và quyền đối với workspace/OS của daemon. Remote Access mở thêm bề mặt Internet cho hệ thống có năng lực thực thi lệnh. Vì vậy tiêu chuẩn phát hành phải cao hơn một dashboard chỉ đọc.

Tài sản mục tiêu:
- Private key định danh thiết bị, dữ liệu auth/session TaskDeck và project-level permissions.
- Nội dung terminal, lệnh đã gõ, file source, Git credentials, secrets trong workspace, Patch artifact, log.
- Tài khoản Portal, membership/grants, Portal signing keys, session ticket.
- Tính toàn vẹn của TaskDeck, workspace, Patch mutation lock, phiên PTY.
- Tính khả dụng: kết nối, công việc đang chạy, hoạt động local không bị gián đoạn.

Đối thủ/rủi ro giả định:
- Người dùng Portal hợp pháp nhưng cố truy cập máy/project ngoài quyền.
- Kẻ có tài khoản Portal bị đánh cắp hoặc quyền admin Portal bị chiếm.
- Kẻ điều khiển một TaskDeck giả, replay heartbeat/ticket, đánh cắp onboarding code.
- Relay hoặc shared hosting bị xâm nhập; nội gián có khả năng đọc log hoặc thay đổi JS.
- Website độc hại mở WebSocket, XSS/CSRF, browser extension độc hại, MITM khi quản lý TLS sai.
- Một TaskDeck bị chiếm quyền cố dùng tunnel tấn công instance khác.
- Tấn công từ chối dịch vụ, ngắt mạng giữa thao tác gây hậu quả khó khôi phục.

Không thể loại bỏ hoàn toàn rủi ro host TaskDeck đã bị malware/compromise hoặc máy người dùng bị chiếm; cần giới hạn blast radius và quy trình thu hồi.

## 2. Quy tắc đánh giá

- **P0 / Blocker:** có thể trực tiếp bypass auth, RCE, truy cập trái phép dữ liệu/project, lộ credentials hoặc thay đổi quyền vượt thẩm quyền; **không được phát hành** khi chưa xử lý.
- **P1 / High:** tác động mạnh tới bảo mật/khả dụng, có thể trở thành P0 khi kết hợp điểm yếu khác; yêu cầu xử lý trước production.
- **P2 / Medium:** cần kế hoạch xử lý với chủ sở hữu, metric và hạn khắc phục; chỉ chấp nhận khi đã ghi rõ residual risk.

Phân loại dựa trên threat model, không phải CVSS tính từ một lỗ hổng đã xác nhận. Không coi việc thiết kế mitigation là bằng chứng đã xử lý thành công.

## 3. Bảng rủi ro ưu tiên

| ID | Mức | Kịch bản rủi ro | Kiểm soát bắt buộc / tiêu chí xác minh |
| --- | --- | --- | --- |
| R-01 | **P0** | Tài khoản Portal bị chiếm và tự động mở terminal trên mọi máy | Portal chỉ cấp quyền mở đường truyền; TaskDeck **vẫn yêu cầu auth riêng** + RBAC từng hành động; MFA/passkey cho tài khoản điều khiển; test Portal admin không vượt được TaskDeck auth |
| R-02 | **P0** | Giả mạo Device ID/heartbeat, chiếm máy bằng thiết bị giả | Device ID fingerprint public key; Ed25519 challenge-response, binding instance/project; pairing 2 chiều có xác nhận cục bộ; test replay/takeover |
| R-03 | **P0** | Replay/đánh cắp session ticket, ticket A mở máy B | Signed ticket ngắn hạn, aud/purpose/Device ID/user/project/roles/jti; consume-once nguyên tử, reject expired/reused/mismatched; test hoán đổi tham số |
| R-04 | **P0** | Portal grant viewer nhưng browser dùng quyền admin TaskDeck thực thi lệnh | Permission hiệu lực = giao Portal scope ∩ TaskDeck RBAC; enforced trong API **và** mọi WS action, không chỉ UI; test viewer→terminal/Patch/file-write denial |
| R-05 | **P0** | Session project/instance A đọc hoặc thao tác project B | Ràng buộc project/instance trong ticket, Agent, origin, cookie, WS, file/PTY/session; negative test cross-project và direct-ID |
| R-06 | **P0** | Public remote proxy mở đường bypass sharedAuth/same-origin/CSRF | Không xóa auth middleware; origin adapter rõ ràng; allowlist Host/Origin, CSRF; không mở proxy arbitrary URL/internal network; test mọi endpoint |
| R-07 | **P0** | Relay/plaintext ghi lại password, terminal, file và secret | Quy định rõ trust model. Nếu claim E2EE: E2EE browser↔TaskDeck có xác thực key, no plaintext/log, kiểm thử relay bị quan sát; chưa E2EE thì **không** phát hành dưới tuyên bố relay không đọc được |
| R-08 | **P0** | JS frontend Portal/Relay bị thay thế để đánh cắp TaskDeck credentials | XSS/deploy/supply-chain controls, strict CSP, tách origin, MFA/passkey, hạn chế third-party JS; đánh giá giải pháp frontend code pinning nếu mô hình đe dọa yêu cầu chống server độc hại |
| R-09 | **P0** | Reverse proxy cho phép SSRF vào loopback/metadata/host LAN | Agent chỉ route về đúng handler TaskDeck đã cấu hình, không nhận tùy ý host:port/scheme; chống path traversal, URL rewriting ngoài allowlist; fuzz/test SSRF |
| R-10 | **P0** | Remote bật khi TaskDeck không có authentication hoặc legacy bypass | Default OFF; preflight bắt buộc auth; deny remote nếu auth off/misconfigured; test cả HTTP và WS, restart và self-update |
| R-11 | **P1** | Quyền bị thu hồi nhưng phiên đã mở vẫn chạy không giới hạn | Session-bound lease/expiry, revoke push + forced disconnect, recheck; fail closed sau TTL hữu hạn khi Portal không truy cập được; test revocation giữa PTY |
| R-12 | **P1** | Cross-Site WebSocket Hijacking / CSRF / session fixation | Explicit Origin allowlist, SameSite/Secure/HttpOnly cookie, CSRF checks, rotate session ID, reject unauthorized WS handshake/message; automated browser tests |
| R-13 | **P1** | Mất mạng rồi tự retry POST/terminal input/patch/delete gây thực thi hai lần | Dedupe idempotency key cho API hỗ trợ; không tự phát lại non-idempotent actions; trạng thái task trên server, explicit user confirmation để retry |
| R-14 | **P1** | Relay DoS: hàng nghìn kết nối/stream, buffer upload không giới hạn | Connection/user/device quotas, message size, flow control, memory/file limits, deadlines, idle timeout, rate limit, circuit breaker |
| R-15 | **P1** | SQLite3 hoặc file key trên shared host bị tải xuống/đọc trái phép | DB/key ngoài webroot, permissions, no backups public, prepared SQL, CSP, hardened PHP, backup encryption, storage filesystem locking tests |
| R-16 | **P1** | Private key agent bị lộ, clone danh tính TaskDeck | OS file ACL / keychain nếu có, rotation, local revoke, giới hạn quyền process; không đưa vào log/backup/repo; test clone/kid rotation |
| R-17 | **P1** | Portal signing key bị đánh cắp hoặc lạm dụng | Tách khỏi webroot, rotate key ID, dual-key rollover, audit use, disaster revoke; chữ ký Portal **không** thay thế auth TaskDeck |
| R-18 | **P1** | Cookie/localStorage phiên của các máy trộn lẫn dưới cùng origin | Isolated public origin per session/instance hoặc kiến trúc lưu trữ chứng minh cách ly; test cross-tab/tenant/cookie domain/path |
| R-19 | **P1** | Nhầm phạm vi ủy quyền khi một host có nhiều project | Fingerprint per instance/project, explicit project binding mọi request; không mặc định cấp quyền toàn bộ host |
| R-20 | **P1** | Terminal/Patch tiếp tục thực thi sau khi phiên remote disconnect mà UI báo đã dừng | Định nghĩa lifecycle/ownership rõ; terminate theo policy hoặc hiển thị continuing task; không tự kill Patch gây corrupt; test abrupt disconnect |
| R-21 | **P1** | Update/migration bật Remote hoặc suy yếu firewall/auth mà user không biết | Remote state bảo toàn nhưng default OFF nếu chưa được chủ sở hữu phê duyệt; kiểm tra config mỗi lần restart/update, tests rollback |
| R-22 | **P2** | Presence online báo sai khi Agent chết hoặc Relay không nhận heartbeat | Heartbeat có ký hoặc gắn auth channel, TTL, phân biệt Connected/Last seen/Unknown; online không đồng nghĩa sẵn sàng nhận lệnh |
| R-23 | **P2** | Nhân sự hosting/CDN có thể đọc log IP/URI dù SQLite không lưu IP | Thiết kế data-minimization, retention ngắn, không đưa token vào URL; rà soát third-party logs, quy định retention/exposure rõ |
| R-24 | **P2** | Shared hosting quá tải / SQLite locked gây mất availability | Giới hạn polling, giao dịch ngắn, busy timeout, backup và restore drill, load test trên đúng loại filesystem/hosting |
| R-25 | **P2** | Nhầm trạng thái máy offline làm người dùng gửi thao tác nhiều lần | UI phân biệt Offline/Connecting/Auth required/Permission denied/Task running; state machine và confirmation rõ ràng |
| R-26 | **P2** | Lạm dụng tính năng đổi tên/claim máy hoặc quản trị membership | CSRF protection, role guard, local confirmation ownership change, audit immutable/append-only tamper-evidence nếu cần |

## 4. Rủi ro đặc biệt khi Portal bị xâm nhập

Đây là tình huống phải thiết kế trước, không được giả định Portal luôn an toàn.

**Một Portal attacker có thể** cấp grant tùy ý, giả trạng thái online, mint ticket nếu chiếm signing key, hiển thị link lừa đảo, sửa PHP/JS, can thiệp dữ liệu user_devices, gây DoS, cố social engineering tài khoản TaskDeck.

**Những gì hệ thống PHẢI chặn:** chỉ chiếm Portal không đủ để thực hiện hành động trong TaskDeck; TaskDeck bắt buộc xác thực riêng trước khi cho phép thao tác, chỉ sử dụng quyền TaskDeck hợp lệ, không chấp nhận "Portal admin" thành local superuser. E2EE nếu triển khai đúng cũng không cho Relay thụ động đọc plaintext.

**Giới hạn rất quan trọng:** browser E2EE thực hiện bằng JavaScript động từ Portal hoặc Relay có thể bị server bị chiếm thay mã, đánh cắp password/đầu vào **trước khi mã hóa**. Do vậy **không được nói rằng E2EE bảo vệ chắc chắn trước Portal đã bị chiếm**. Khi cần chống attacker có quyền sửa JS, phải sử dụng kênh phân phối client tin cậy hơn (signed/pinned client app/extension, code integrity độc lập hoặc xác minh ngoài băng), và xác định rõ threat model.

Dù có 2FA Portal, vẫn cần auth TaskDeck, thiết bị tin cậy của người điều khiển và quyền tối thiểu. Bật approval cục bộ đối với thao tác cấp quyền remote có đặc quyền cao là lớp bảo vệ bổ sung tùy chính sách vận hành.

## 5. Rủi ro riêng của Relay / tunnel

### 5.1. TLS không phải E2EE

TLS Browser→Relay và Relay→Agent bảo vệ đường mạng nhưng Relay có thể giải mã payload khi làm proxy. Nếu nhà vận hành không chấp nhận Relay đọc được terminal/file thì E2EE là **release blocker**. Không truyền Basic Auth, cookie TaskDeck hay access token qua Relay dạng plaintext HTTP body có thể bị ghi log ngoài mô hình đã chấp nhận.

Với E2EE, dùng giao thức đã được đánh giá, xác thực public key theo identity đã ghép; key agreement phải chống active MITM, replay, key substitution, downgrade, nonce reuse. Không tự nghĩ ra custom crypto. Encryption không thay thế authorization và audit.

### 5.2. Relay không phải open proxy

- Không chấp nhận client gửi arbitrary host, port, local IP, URL hoặc taskdeck base URL để relay chuyển tới.
- Routing dựa **duy nhất** vào Device ID đã xác thực, session ticket và kênh Agent đang được thiết lập.
- Gắn ACL vào cả HTTP methods, WS events, download/upload, redirects và error handling; phân biệt user-controlled data với authority.
- Chặn request smuggling, header confusion, forwarded identity spoofing, path traversal và tunnel reuse giữa session.
- Giới hạn session và byte transferred, concurrency, buffer memory, QoS/bandwidth; không lưu nội dung lệnh/file nếu chưa có yêu cầu rõ.

### 5.3. Split control/data plane và sự cố thu hồi

Không giả định Relay biết trạng thái grant mới nhất chỉ vì đã nhận ticket 30 giây trước. Khi quyền bị thu hồi, Relay và Agent phải đóng phiên hoặc hết lease trong thời gian xác định. Nếu Portal/Relay mất liên lạc, không cho phép session bị bỏ mặc vô thời hạn.

Thực hiện định nghĩa về long-running work: một Patch đang chạy có thể tiếp tục trên TaskDeck sau khi tab mất kết nối **nếu** policy hiện tại cho phép; nhưng lệnh input mới phải bị từ chối khi session hết quyền. Báo rõ không chạy lại lệnh khi reconnect.

## 6. Risks của management PHP/SQLite3 trên shared host

- Database backup, SQLite -wal/-shm, PHP config, session file và secret signing key có thể bị lộ do đường dẫn hoặc file permissions. Chọn đường dẫn ngoài public root; diễn tập thử truy cập trực tiếp URL vào file.
- SQLite chấp nhận giao dịch ngắn, prepared statements, user_device uniqueness, FK constraints, transaction atomic và indices. Test race condition grant/revoke/consume-once.
- Không bật SQLite WAL khi DB đặt trên network filesystem không đáp ứng điều kiện; dùng journal mode phù hợp tài liệu SQLite và kiểm thử host thực tế.
- Password hash Portal: dùng Argon2id khi PHP hỗ trợ, nếu không dùng thuật toán password_hash mạnh được hỗ trợ; tuyệt đối không tự SHA256 password. Rotate session khi login/đổi quyền; bảo vệ cookie.
- Rate-limit đăng nhập, signup, pairing, reset, token issuance theo user/device và IP ở tầng edge nếu pháp lý/chính sách cho phép. Nếu cấm lưu IP, dùng ephemeral counters và có thể cần đánh đổi khả năng phát hiện abuse.
- MFA/passkey cho admin và remote operator; tránh thiết kế SMS là yếu tố duy nhất khi threat model yêu cầu chống phishing.
- Shared host bị khai thác filesystem hoặc có quyền sửa JS làm thay đổi mức rủi ro; không coi nó là trusted execution environment.

## 7. Riêng về privacy và "không lưu thông tin kết nối"

### Phân loại dữ liệu

| Nơi lưu | Được phép | Không được phép |
| --- | --- | --- |
| Portal SQLite (lâu dài) | user Portal/password hash/passkey, Device ID, tên, grants, audit tối thiểu | TaskDeck password, key, cookie, token, host/IP/port riêng từng thiết bị, URL kết nối riêng |
| Portal ephemeral | nonce/challenge, jti đã dùng, timestamp/TTL, trạng thái presence | Nội dung session/terminal/file |
| Relay RAM | mapping session↔agent WSS socket, transport address tạm thời, quota | TaskDeck private key/password, profile máy lâu dài |
| Relay/hosting logs | Sự kiện tối thiểu, mã lỗi, counters không nhạy cảm | Ticket, bearer token, cookie, upload body, terminal keystrokes, secrets |
| TaskDeck local | Private key agent, TaskDeck auth store, local sessions/projects | Secret của user khác hoặc quyền vượt cấp từ Portal |

**Không thể kết nối Internet mà Relay không thấy địa chỉ mạng tạm thời của socket**. Việc không lưu vào SQLite **không có nghĩa** không tồn tại IP trong load balancer, CDN, PHP-FPM access log, metrics hoặc công cụ quan sát. Cần kiểm tra thực tế mức lưu trữ, retention, quyền nhân viên và yêu cầu pháp lý liên quan.

Nếu bắt buộc "tuyệt đối không lưu IP ở bất cứ đâu", phải thẩm định tính khả thi với nhà cung cấp hosting/relay, không được tự cam kết.

## 8. Rủi ro bảo mật đặc trưng TaskDeck

TaskDeck hiện có các chức năng mạnh hơn một admin portal thông thường:
- **Terminal/SSH/PTY:** có thể chạy lệnh shell, đọc secrets, thao tác repo. Kiểm tra user vs terminal ownership và attach permission trên WebSocket; không được đơn giản hóa thành "đã có ticket".
- **Editor/File Manager:** xử lý symlink, path traversal, archive extraction, upload overwrite, unsaved edits, race condition với Patch/Replace. Remote không được bypass workspace root hay mutation lock.
- **Patch/Collection:** file patch/collection có thể chứa lệnh, paths hoặc data nhạy cảm; bảo vệ upload type, validation, run authorization và artifact download.
- **Git:** credential helper/SSH keys ở máy TaskDeck; không đẩy key/token về Portal, không log URL có credentials. Pull/update cần kiểm soát permission.
- **Settings/Self-update/Launcher:** chức năng đổi auth, mở port, chạy task, tự cập nhật, tạo launcher là mục tiêu privilege escalation; áp dụng step-up auth/role guard ở backend và test user denied.
- **Multi-project:** tuyệt đối không dùng chung session ID, cookie, authenticated principal hoặc broker lease giữa các project khác nhau.
- **Browser remote:** CSP, WebSocket Origin, service worker, cache, cookies, redirect; malicious HTML/file preview không được chiếm quyền parent TaskDeck origin.
- **Local-first availability:** hỏng Portal/Relay không gây hỏng hoặc giảm khả năng dùng local; phiên Remote không tắt/tự hạ cấp bảo vệ local.

Các check của shared_auth/RBAC hiện có phải được duy trì; adapter remote nếu cần khác biệt origin phải có test negative trước khi merge.

## 9. Bộ test bắt buộc trước khi phát hành

### 9.1. Identity, pairing, authentication

- [ ] Thiết bị A và B có ID khác nhau; attacker dùng public key khác nhưng mạo danh ID A bị từ chối.
- [ ] Challenge bị phát lại, hết hạn, sai audience/sai project, signature sai, duplicate Device ID: đều bị từ chối.
- [ ] Claim/re-pair/rotate key không thể thực hiện chỉ bằng thay đổi SQLite hoặc thao tác Portal admin.
- [ ] User chưa đăng nhập Portal, user thiếu grant, grant expired/revoked không có tunnel.
- [ ] Signed ticket cũ, consumed ticket, ticket gắn user A mở máy B, ticket sửa scope, ticket đổi origin bị từ chối.
- [ ] Remote ON nhưng TaskDeck auth OFF/misconfigured: kết nối remote không được mở.
- [ ] Login Portal thành công **không** tự biến thành đăng nhập TaskDeck.

### 9.2. API, WebSocket, capability escalation

- [ ] Gọi API/WS trực tiếp (không qua UI) không vượt Auth/RBAC/CSRF/scope.
- [ ] Portal viewer không tạo PTY, gửi terminal input, run Patch, ghi file, đổi Settings hoặc self-update được.
- [ ] Cross-project/session/user direct-ID access bị chặn; session browser và storage không rò giữa remote tab.
- [ ] Malicious Origin, cross-site WebSocket, forged Host/X-Forwarded-*, path traversal, SSRF/URL proxy đều bị chặn.
- [ ] Remote upload/download binary, file lớn, slow client/slow agent, reconnect, timeout không phá lock/state và không tràn memory.
- [ ] Thử mọi route / WebSocket handler và chức năng đang có trong TaskDeck, không chỉ dashboard.

### 9.3. Failures, operational controls

- [ ] Thu hồi grant/khoá user khi terminal còn mở buộc đóng quyền input trong TTL đã quy định.
- [ ] Mất mạng khi đang POST/patch/delete/restart không gây tự chạy lại thao tác.
- [ ] Relay/Portal crash, restart, DNS/TLS failure, cert rotate: remote fail-closed, local vẫn dùng tốt.
- [ ] Rate limit, connection quota, message max, memory upper bound, timeout hoạt động dưới stress load.
- [ ] Secrets không xuất hiện trong PHP log, Relay log, browser URL/history, referrer, metrics hay debug log.
- [ ] SQLite và bản backup không truy cập được qua URL public, permission trên shared hosting được xác minh.
- [ ] Tính năng E2EE (nếu được claim) vượt qua test Relay MITM thụ động và active; không có downgrade; kiểm thử JS injection model.
- [ ] Kiểm thử Linux và macOS, Firefox/Chromium/Safari, nhiều TaskDeck trên cùng host, IPv4/IPv6, NAT/CGNAT, proxy chặn WS.

## 10. Security release gate / quy trình quyết định

**Không được đánh dấu READY/production** nếu còn P0, R-11/R-12/R-13/R-14/R-15/R-18 chưa được test hoặc có auth bypass trong một route.

Tài liệu nghiệm thu cần ghi:
1. Threat model được duyệt, trust assumptions và threat exclusions cụ thể.
2. Danh sách actual endpoints, HTTP methods, WS events, authorization map; ma trận local↔remote.
3. Kết quả test tự động + manual exploit/fuzz, bằng chứng fail closed.
4. Chính sách key storage/rotate/revoke/re-pair và kiểm thử khôi phục khi mất khóa.
5. TLS certificate/domain, CSP, cookie policies, origin isolation và browser security review.
6. Privacy data inventory: SQLite, file, backup, metrics, hosting/CDN logs và retention.
7. Penetration test độc lập hoặc ít nhất red-team review có sign-off trước rollout rộng.
8. Production rollback plan: tắt Remote tại TaskDeck, khóa token issuer, ngắt Relay session, bảo đảm local không bị ảnh hưởng.

Chỉ sau đó mới xem xét mở cho nhiều người dùng. Rollout theo allowlist thiết bị/người dùng và giới hạn tài nguyên là lựa chọn an toàn.

## 11. Residual risks sau khi có đầy đủ biện pháp

Không có kiến trúc nào loại bỏ hoàn toàn rủi ro:
- Máy client vốn đã bị compromise có thể làm lộ khóa/credentials cục bộ.
- Browser/extension bị compromise có thể thấy plaintext và nhận lệnh hợp lệ.
- Portal kiểm soát JS có thể xâm phạm an toàn frontend ngay cả khi áp dụng E2EE bằng JS; cần kênh client độc lập nếu điều này không chấp nhận được.
- Lưu lượng, thời gian phiên, địa chỉ transport có thể bị quan sát bởi nhà mạng, Relay hoặc hạ tầng hosting, dù dữ liệu ứng dụng đã mã hóa.
- Shared host và một Relay đơn là single points of availability failure; scale/failover có thêm phức tạp quản lý trạng thái và chống replay.
- Người dùng được cấp quyền shell hợp lệ vẫn có thể thực hiện tác vụ nguy hiểm. RBAC không phải sandbox của hệ điều hành. Nếu yêu cầu chặn thao tác OS, cần OS-level sandbox/isolation, không chỉ role UI.

Chủ sở hữu phải chấp nhận rõ các residual risks hoặc tăng mức kiểm soát; không được ghi "an toàn tuyệt đối".

## 12. Kết luận

**Hướng kiến trúc khả thi, nhưng là hạ tầng remote execution có tác động bảo mật cao.** Quyền kết nối trên Portal và quyền hành động trong TaskDeck phải luôn tách biệt. Identity thiết bị phải có bằng chứng sở hữu private key và ghép nối cục bộ; tunnel không bao giờ là đường đi tắt qua Auth/RBAC/Origin checks; ngắt quyền phải tác động được tới phiên đang chạy.

Đối với yêu cầu Relay không đọc dữ liệu và Portal không giữ bất kỳ bí mật TaskDeck nào, cần xác minh thực tế từ mã và network trace; không được đánh đồng "không lưu vào SQLite" với "không thể quan sát được".

### Nguồn/tham chiếu chuẩn

- OWASP WebSocket Security Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html
- OWASP Authentication Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html
- OWASP Session Management Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html
- OWASP Cross-Site Request Forgery Prevention Cheat Sheet: https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html
- NIST SP 800-63B-4: https://csrc.nist.gov/pubs/sp/800/63/b/4/final
- SQLite WAL limitations: https://sqlite.org/wal.html
- TaskDeck security baseline: [README](../README.md), [shared_auth.go](../vscode_tasks_menu_go/internal/server/shared_auth.go), [SHARED_SERVER_ACCEPTANCE.md](../vscode_tasks_menu_go/SHARED_SERVER_ACCEPTANCE.md).
