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


## Rà soát mã nguồn bổ sung — 2026-10-28

Phạm vi kiểm tra lần này: `remote_portal/src/Store.php`, `remote_portal/public/index.php`, `remote_portal/src/TicketSigner.php`, luồng Agent/Relay xác thực grant và login-only E2EE, cùng kiểm thử CI PHP/Go. Đây là **code review có kiểm thử hồi quy**, không phải pentest độc lập hoặc bằng chứng an toàn production.

| Phát hiện / rủi ro | Mức | Kết quả và giới hạn |
| --- | --- | --- |
| Đăng nhập ẩn danh bằng hàng loạt username giả làm phình `login_attempts` trong SQLite | P1 — **đã sửa** | Chỉ lưu bộ đếm cho user có thật, `init` dọn row legacy. Kiểm thử 60 username không tạo row mới và bảo toàn bộ đếm tài khoản thật |
| SQLite trên shared hosting có thể được tạo với mode mặc định đọc được bởi Unix user khác | P1 — **đã siết** | Bắt buộc thư mục 0700, file DB 0600, không nhận symlink và tạo file dưới umask 0077. PHP smoke và CI kiểm chứng. Site cũ phải `chmod` trước khi nâng cấp |
| Grant đã ký trước khi đổi mật khẩu Portal vẫn có thể tồn tại trong cửa sổ 30 giây | P1 — **cải thiện login-only** | Ticket ký `auth_epoch`, Agent xác minh trực tiếp quyền và thế hệ phiên với Portal trước khi gọi scrypt. Test old epoch bị từ chối. Điều này **không** thay thế thu hồi session ứng dụng khi remote execution được triển khai |
| Flood anonymous enrollment (mỗi key Ed25519 tự ký), dò tên account và quá tải PHP/SQLite | P1 — **còn mở** | Có TTL/cap pending enrollment, nhưng thiếu ingress throttling tại CDN/reverse proxy dựa trên IP nguồn tin cậy. Không lấy `X-Forwarded-For` do client tự khai; không lưu IP gốc vào bảng Portal nếu chưa có chính sách riêng tư rõ ràng |
| XSS/JavaScript cấp từ Portal bị xâm nhập đánh cắp mật khẩu TaskDeck trước khi WebCrypto mã hóa | P0 khi có thao tác thật — **còn mở** | CSP và E2EE không giải quyết frontend supply-chain trust. Cần chiến lược phân phối browser/client được xác thực độc lập trước khi bật credential/command production |
| Chưa có MFA/passkey, thu hồi phiên ứng dụng trực tiếp, key rollover và allowlisted TaskDeck application dispatcher | P0/P1 — **còn mở** | Vẫn chỉ chẩn đoán và login-only experimental; không mở Terminal/Patch/Git/File/HTTP API. Cần kiểm thử RBAC, revocation, chống replay và flow control trên từng lệnh |

**Điều kiện an toàn khi triển khai thử nghiệm:** không bật `REMOTE_PORTAL_EXPERIMENTAL_LOGIN` hoặc `TASKDECK_REMOTE_LOGIN_PROBE` trên hosting public; bảo vệ thư mục SQLite và seed ký; giữ Remote Agent mặc định tắt; tránh công bố link/credential qua log; kiểm tra lại đường truy cập trực tiếp đến các thư mục PHP không thuộc `public`; đặt rate-limit ngoài PHP cho `/login` và `/api/agent/enroll` khi phải expose control plane. Kiểm tra các quyền OS của tài khoản chạy PHP và backup SQLite, không chỉ permission của file gốc.

**Nguyên tắc phát hành:** chỉ coi Remote Access production-ready khi application transport E2EE đầy đủ, Portal MFA, TaskDeck authentication/RBAC mọi action, thu hồi live session, rate-limit ingress, key rotation và bài kiểm tra NAT/CGNAT end-to-end đều vượt qua kiểm thử và review độc lập.


### Kiểm tra bổ sung: chống brute-force đa tiến trình PHP

**Rủi ro đã được khắc phục trong phạm vi Portal:** các PHP worker trước đây có thể cùng đọc số lần đăng nhập sai, rồi xác minh mật khẩu tốn CPU trước khi cập nhật SQLite. Hàng chục request song song có thể vượt ngưỡng 5 lần trong một cửa sổ. Code mới đặt chỗ lượt kiểm tra trước khi chạy Argon2/bcrypt bằng giao dịch `BEGIN IMMEDIATE`, khóa tạm tài khoản và kiểm tra trạng thái password/auth epoch sau khi xác thực. Bài test `bruteforce.php` tạo 24 worker riêng biệt.

**Kiểm soát ingress:** đăng nhập giới hạn 10/phút/IP và 180/phút toàn site; enrollment 12/phút/IP và 120/phút toàn site, lưu trên SQLite với HMAC-SHA256(IP) và key riêng 0600 ngoài webroot. Không lưu IP thô, không dùng header X-Forwarded-For do Internet tự gửi; các trường hợp thiếu key, lỗi SQLite và đồng hồ bị lùi đều chặn. Khi reverse proxy làm mọi người dùng có chung REMOTE_ADDR, phải cấu hình trusted proxy/edge đúng; không được tự tin rằng ứng dụng đã loại bỏ hoàn toàn brute-force phân tán hay DDoS.

**Rủi ro còn lại:** khóa account có thể bị lạm dụng để gây từ chối dịch vụ với tài khoản thực; cần MFA/passkey, adaptive step-up và quy trình khôi phục an toàn. Giới hạn toàn cục có thể ảnh hưởng người dùng hợp pháp khi site bị flood. Kiểm soát trên CDN, firewall/WAF và chính sách privacy/logging là điều kiện vận hành bắt buộc trước production.


**Phát hiện P1 bổ sung đã khắc phục — nonce flood qua `/api/agent/grant-check`:** trước đây yêu cầu Ed25519 tự ký của Device ID chưa ghép nối vẫn ghi `device_nonces` rồi mới phát hiện không có quyền. Kẻ tấn công có thể tạo nhiều khóa công khai và tăng bảng SQLite mà không cần chiếm tài khoản. Đã sửa điều kiện kiểm tra thiết bị hợp lệ, đang được ghép nối và đã bật **trước khi ghi nonce**, trong cùng transaction. Kiểm thử 24 danh tính tự tạo bảo đảm không tăng bảng nonce. Cần tiếp tục có rate limit ở reverse proxy/CDN để giảm CPU khi flood chữ ký.

**P1 đóng thêm — DoS nonce từ Device ID đã ghép nối bị lộ private key:** các lời gọi `heartbeat` / `grant-check` trước đây có chữ ký hợp lệ nhưng không có hạn mức riêng, có thể làm tăng `device_nonces` không giới hạn trong TTL. Đã thêm quota bền vững qua nhiều PHP worker: tối đa 90 signed proofs/Device ID/60 giây, đặt quota trước khi ghi nonce trong một giao dịch SQLite. Grant bị thu hồi/không hợp lệ không được lưu nonce. Xóa thiết bị xóa cả quota tương ứng, kiểm thử trường hợp vượt ngưỡng và cập nhật sau hết thời gian. Vẫn cần chặn flood trước PHP bằng hạ tầng reverse proxy/WAF; đây không phải giải pháp DDoS toàn diện.


### Rà soát tiếp: tiền xác thực Agent, quyền Portal, timing oracle

- **P1 giảm thiểu — flood trước Ed25519:** quota Device ID chỉ hoạt động *sau* khi xác thực, không ngăn request JSON/chữ ký giả chiếm PHP worker. Hai API Agent `heartbeat` và `grant-check` nay dùng SQLite admission **trước** kiểm tra chữ ký: 900/phút/IP (HMAC, không lưu IP thô) và 9000/phút/site. DB bị lỗi hoặc chưa migrate thì fail closed. Giới hạn cao có chủ đích để không gây khóa nhầm khi nhiều TaskDeck cùng NAT. Cần CDN/WAF vì flood lớn vẫn có thể làm đầy hàng đợi trước PHP.
- **P1 giảm thiểu — chiếm phiên admin:** các endpoint tạo tài khoản, ghép/xóa thiết bị, cấp/thu hồi quyền trước đây không có ngân sách thao tác riêng cho phiên đã đăng nhập. Nay có `portal_admin_limits`: tối đa 60 thao tác ghi/phút/tài khoản admin, kiểm tra account đang bật và còn admin ở lúc đặt quota, chống nhiều PHP worker cùng vượt giới hạn. Quota không thay thế CSRF, MFA hoặc xác thực lại cho hành động nhạy cảm.
- **P2 giảm thiểu — username enumeration:** tài khoản có thật bị khóa từng trả về nhanh hơn tài khoản không tồn tại vì không chạy dummy verify. Các đường này giờ đều chạy Argon2 giả khi từ chối, trong giới hạn ingress đã kiểm soát.
- **Tương thích upgrade:** tạo bảng `portal_agent_ingress_limits` và `portal_admin_limits` bằng `admin.php init` (additive), không sửa ràng buộc cũ của bảng login/enrollment. CI cần chạy `bruteforce.php` và `admin_limits.php`.

**Vẫn còn rủi ro:** DOS ở hạ tầng/proxy trước PHP, khả năng khóa hợp lệ khi quá tải toàn cục, đánh cắp PHP session, thiếu passkey và xác thực lại riêng từng hành động đặc biệt quan trọng, chính sách phục hồi MFA, phân quyền theo lệnh khi Remote Control thực sự được mở.


### Cập nhật: MFA Portal, khóa bí mật và session DoS

**Đã xử lý:** Portal có TOTP MFA tiêu chuẩn RFC6238 (mặc định bắt buộc trước khi sử dụng thiết bị/quyền), bảo vệ trước replay bằng cập nhật bước thời gian trong SQLite; bí mật TOTP chỉ lưu được mã hóa bằng khóa 0600 ngoài webroot; bật/reset MFA tăng `auth_epoch`. Phương thức reset qua web không tồn tại; reset chỉ thực hiện trong private CLI có `--confirm`, ghi audit và buộc thiết lập lại MFA. Các yêu cầu xác minh mật khẩu trong phiên đã đăng nhập có quota 5/10 phút/tài khoản và không thể bỏ qua bằng cách mở thêm phiên. Khóa mã hóa MFA và HMAC chống brute-force không thể bị lệnh init âm thầm thay mới nếu đã có dữ liệu phụ thuộc.

**Đã xử lý thêm P1 — DoS session PHP trước đăng nhập:** trước đây GET/POST sai URL hoặc CSRF không hợp lệ vẫn có thể tạo PHP session file không giới hạn. Đã đưa whitelist đường dẫn/phương thức, giới hạn kích thước form và hạn mức HMAC/SQLite (120/phút/IP nguồn, 2400/phút toàn site) đến trước `session_start()`; request không hợp lệ bị từ chối mà không phân bổ session. Có kiểm thử đa tiến trình và HTTP đếm file session.

**Rủi ro còn lại:** TOTP không chống phishing/real-time relay như passkey; đăng ký MFA lần đầu phải thực hiện trên HTTPS và thiết bị tin cậy. Người dùng hợp lệ sau khóa TOTP có thể cần chờ mã 30 giây mới trước hành động nhạy cảm khác. Dịch vụ shared hosting vẫn phải có WAF/CDN chặn flood trước PHP; hạn mức toàn cục có thể bị đối phương dùng để gây mất dịch vụ. Khóa 0600 phải được sao lưu và kiểm tra phục hồi định kỳ; mất khóa MFA khiến không thể giải mã secret đã đăng ký. Remote Control/terminal/file/Patch chưa được mở cho đến khi hoàn tất E2EE ứng dụng và phân quyền từng hành động.


### Khóa ổn định định danh HMAC qua restart và khôi phục

**P1 đã khắc phục:** nếu mất/thay file `portal-abuse-hmac.key` rồi chạy lại `admin.php init`, trước đây hệ thống có thể tạo khóa ngẫu nhiên mới, khiến toàn bộ định danh hạn mức theo IP thay đổi và coi các nguồn đã bị khóa như nguồn mới. Đã bổ sung `portal_key_checks` với SHA-256 commitment của khóa 256-bit (không lưu khóa thô hoặc IP). Trong vận hành, mọi yêu cầu đều kiểm tra commitment; nếu file khóa mất hoặc bị thay, trả về lỗi và không tiếp tục thử đăng nhập. CLI migration không được tái tạo khóa đã đăng ký. Khôi phục phải dùng bản sao khóa gốc được bảo vệ ở ngoài webroot. Không tự động xóa commitment. Kiểm thử chứng minh giữ nguyên khóa sau migration, từ chối key đã thay và phục hồi sau khi trả lại key chuẩn.


**Đã khắc phục P1 — các cách viết IP có thể lách quota:** chuẩn hóa địa chỉ bằng `inet_pton`, hợp nhất IPv4-mapped IPv6 trước khi tính HMAC. Dạng IPv6 rút gọn và đầy đủ nay dùng chung lượt thử; vẫn không lưu IP thô. Đổi định dạng HMAC trong bản nâng cấp có thể làm mới quota *theo nguồn* trong một cửa sổ, nhưng quota toàn cục vẫn áp dụng; cần triển khai trong thời điểm bảo trì, WAF luôn bật.

**Đã khắc phục P1 — enrollment JSON flood:** cả `/api/agent/enroll`, `heartbeat` và `grant-check` áp dụng quota nguồn/toàn site trước khi giải mã JSON, kiểm tra chữ ký. Kiểm thử HTTP gửi 12 enrollment sai content type rồi yêu cầu thứ 13 phải trả 429 trước khi làm việc tốn tài nguyên.


### Kiểm tra phân quyền ngay trong thao tác ghi

**P1 đã giảm thiểu — thiếu defense-in-depth trong Store:** những hàm `grant`, `revoke`, `claim`, `removeDevice` trước đây dựa vào việc lớp HTTP đã xác minh quyền admin. Nếu dùng lại hàm từ một code path mới hoặc lỗi kiểm tra quyền tại router, tài khoản không phải admin có thể thay đổi quyền ghép máy. Đã bổ sung kiểm tra admin còn hiệu lực và tài khoản đang bật **ngay trong cùng giao dịch SQLite** thực hiện thao tác, audit cùng commit/rollback. Bài smoke gọi trực tiếp cả bốn hàm bằng tài khoản không phải admin và kiểm tra grant không thay đổi. Giới hạn 60 thao tác/phút là lớp khác, không phải yếu tố xác thực.


### Lỗ hổng lưu secret MFA trong PHP session dùng chung

**P1 đã giảm thiểu:** Portal có thể đặt session theo cấu hình mặc định của shared hosting dù SQLite và khóa MFA đã nằm ngoài webroot, dưới quyền 0700. Session bao gồm CSRF/token đăng nhập và secret TOTP **chưa xác nhận**; hệ thống mặc định hoặc custom handler kém an toàn có thể làm lộ chúng qua các tài khoản cùng máy chủ. Đã buộc browser session dùng handler `files` và thư mục riêng `browser-sessions-<sha256-prefix>` cạnh file SQLite, tách biệt theo đường dẫn DB đã chuẩn hóa với quyền 0700, kiểm tra symlink/quyền/mục tiêu hợp lệ trước `session_start`; hai cơ sở dữ liệu cùng thư mục cha không dùng chung session. Không thể khởi tạo storage an toàn thì HTTP 503; Agent API vẫn không tạo session. Test chạy HTTP thật từ PHP cấu hình session mặc định là `/tmp` và kiểm tra file session tạo dưới vùng riêng có quyền chủ sở hữu.

**Rủi ro vẫn còn:** compromise PHP worker hoặc cùng Unix UID vẫn đọc được session; cần cách ly người dùng/ứng dụng tại hosting, session TTL ngắn, HTTPS và CSP đúng. Nâng cấp sẽ khiến người dùng đăng nhập lại và phải lưu trữ/backup SQLite, khóa HMAC/MFA riêng tư đúng quyền.


### MFA ticket bypass và browser isolation

**P1 giảm thiểu — cấu hình bootstrap bỏ qua MFA:** khi `REMOTE_PORTAL_REQUIRE_MFA=0`, phiên Portal được xác thực chỉ bằng mật khẩu trước đây có thể thử tạo Relay ticket nếu bật cờ thử nghiệm. Đã bổ sung chốt riêng tại `/api/remote/ticket`: tài khoản chưa có TOTP bị từ chối ngay trước ký, không phụ thuộc biến cấu hình bootstrap. Kiểm thử HTTP thật khẳng định trang Portal có thể vào bằng mật khẩu trong chế độ khôi phục nhưng endpoint ticket trả HTTP 403.

**Bảo vệ trình duyệt:** thêm COOP/CORP same-origin, Permissions-Policy vô hiệu camera/microphone/geolocation/payment, kiểm tra session ID đổi sau login/MFA/logout. Dev-router chỉ phục vụ danh sách JS E2EE đã biết và từ chối các URL khác, tránh sửa lỗi tải JS bằng cách mở rộng phục vụ file tùy ý. Rủi ro còn tồn tại: dùng chung hostname với ứng dụng khác có thể gây rò rỉ same-origin dù có các header này; nên dành một hostname HTTPS riêng cho Portal, bật MFA bắt buộc và bảo đảm ký tự nguồn JavaScript tin cậy.


### Điều kiện thế hệ xác thực và MFA quản trị

**P1 giảm thiểu — ticket có chữ ký nhưng thiếu thế hệ xác thực:** Go Relay/Agent trước đây kiểm tra `AuthEpoch < 0`, nghĩa là ticket bị thiếu trường hoặc bằng 0 vẫn qua bộ giải mã chữ ký, dù lớp live-grant sau đó thường chặn. Đã yêu cầu `AuthEpoch >= 1` ngay tại ranh giới xác minh Ed25519. Kiểm thử ký ticket hợp lệ bằng khóa test, thử giá trị 0, -1 và giá trị hợp lệ, đồng thời xác nhận toàn bộ kiểm thử Go/TaskDeck với fixture có epoch dương.

**P1 giảm thiểu — MFA override cho admin:** cờ `REMOTE_PORTAL_REQUIRE_MFA=0` có thể hỗ trợ khởi tạo tài khoản chưa đăng ký TOTP, nhưng không còn cho tài khoản admin truy cập phần cấp quyền, tạo tài khoản hoặc ghép máy chỉ với mật khẩu. Admin bắt buộc bật TOTP kể cả khi cờ override; tài khoản thường trong chế độ bootstrap chỉ có thể sử dụng các tính năng Portal không phải Remote. Endpoint cấp Relay ticket tiếp tục bắt buộc TOTP độc lập với cấu hình. Bài kiểm thử HTTP xác nhận các request quản trị trực tiếp bị chặn.

**Rủi ro chưa giải quyết:** thu hồi/cắt phiên đang hoạt động trên Relay khi grant thay đổi, bảo vệ khóa Ed25519 bằng backup/rotation có kiểm soát, bảo đảm authorized TaskDeck application dispatcher và per-command RBAC, chống flood ở CDN/reverse proxy, kiểm thử end-to-end ngoài mạng LAN.


### Cách ly hạn mức giữa nhiều Agent và chống chiếm slot Relay

**P1 đã giảm thiểu — nhầm lẫn Device ID trong giới hạn scrypt:** nhiều project trên cùng host sử dụng chung thư mục cấu hình nhưng Agent trước đây ghi tất cả vào `remote-login-rate.json`. Khi project B đọc file có `device_id` của project A, tài khoản bị từ chối dù hợp lệ, đồng thời các lần migration có thể vô tình khôi phục quota. Đã chuyển sang file và file-lock theo Device ID; bản ghi của Agent cũ được kiểm tra và **di chuyển** atomically dưới khóa cũ/mới, giữ các lượt thử đã sử dụng. Project khác không được phép chiếm hoặc kế thừa budget của project A. Kiểm thử nhiều Agent, restart, migration đang hết quota và file hỏng/symlink.

**P1 đã giảm thiểu — một nguồn chiếm toàn bộ Relay:** mức giới hạn toàn cục hiện có (256 Agent / 64 Browser) không ngăn một IP mở nhiều socket và chiếm hết tài nguyên. Thêm ngân sách 64 Agent / 8 Browser mỗi địa chỉ nguồn TCP trực tiếp, lưu tạm trong RAM rồi xóa khi socket đóng. Không dùng `X-Forwarded-For` do client tự khai. IPv4 mapped-IPv6 được chuẩn hóa để không cấp thêm quota. Cần giữ reverse proxy/WAF và chính sách trusted proxy vì nhiều Agent hợp lệ đi chung NAT có thể bị giới hạn theo địa chỉ chia sẻ; tấn công từ nhiều nguồn phân tán vẫn có thể làm cạn global slots.

**Phát hành:** hiện vẫn chưa có dispatcher an toàn cho lệnh TaskDeck, lập luận E2EE và phân quyền từng lệnh phải được kiểm chứng end-to-end trước khi mở Remote Control.


### Không thể bỏ qua MFA khi gọi thẳng Store cấp ticket

**P1 giảm thiểu — kiểm tra MFA chỉ ở HTTP:** trước đây `/api/remote/ticket` từ chối người dùng chưa bật TOTP, nhưng `Store::reserveRemoteTicket` lại chỉ kiểm tra quyền truy cập thiết bị, online và hạn mức. Một lối gọi khác trong tương lai có thể đặt chỗ ticket cho người dùng chưa bật MFA, ngay cả sau khi quản trị viên reset TOTP. Đã thêm điều kiện tài khoản có secret TOTP vào **cùng giao dịch SQLite** với kiểm tra grant/enablement và ngân sách cấp ticket; reset MFA offline sẽ làm các đặt chỗ kế tiếp bị từ chối, không tốn quota. Test dùng TOTP đã kích hoạt thật và thử trước/sau reset, song song vẫn giữ test limit nhiều worker.

**Vẫn cần:** trong mọi trường hợp trước khi sử dụng ticket thực tế, Agent phải đối chiếu `auth_epoch`/grant trực tiếp với Portal và tái kiểm tra quyền truy cập ở mọi hành động ứng dụng. Cơ chế này không thay thế thu hồi phiên đang sống hoặc xác thực riêng TaskDeck.


### MFA bắt buộc tại mọi thao tác quản trị ở tầng Store

**P1 giảm thiểu — đường gọi bỏ qua MFA của quản trị viên:** các hàm `Store::grant`, `revoke`, `claim`, `removeDevice` và đặt quota quản trị trước đây chỉ hỏi vai trò admin và trạng thái enabled, dựa vào router để xác nhận tài khoản đã bật TOTP. Lớp dữ liệu nay từ chối admin chưa có secret TOTP ngay **trong cùng SQLite write transaction** với kiểm tra quyền và thao tác ghi; cờ bootstrap `REMOTE_PORTAL_REQUIRE_MFA=0` không thể mở lại quyền này. Reset MFA bằng CLI sẽ chặn các hành động quản trị mới cho đến khi người dùng đăng ký lại.

Kiểm thử hồi quy dùng TOTP thực cho các bước trước/sau enrollment và sau reset, đồng thời giữ bài kiểm thử nhiều worker, revoke/grant/claim và hạn mức riêng. MFA là điều kiện cần nhưng không thay thế CSRF, phân quyền từng lệnh, xác thực lại thao tác nhạy cảm và chống lấy cắp phiên.


### Đường tạo tài khoản web không được dùng bootstrap CLI

**P1 đã giảm thiểu — dùng chung phương thức tạo tài khoản:** endpoint `/users/create` trước đây dựa vào router kiểm tra admin và MFA rồi gọi `Store::createUser` dành cho bootstrap, vốn không có người thực hiện/kiểm tra quyền ở giao dịch SQLite. Nay endpoint gọi `createUserByAdmin`: xác nhận admin còn enabled và có TOTP tại lúc ghi, chỉ cho phép tạo user thường, ghi audit actor cùng transaction. Hash mật khẩu được tính trước khi giữ SQLite write lock để tránh trì hoãn các worker. Phương thức bootstrap `createUser` chỉ chạy trong PHP CLI, không chạy dưới PHP-FPM/Apache hoặc PHP dev HTTP. Lệnh `admin.php create-admin` trên host được giữ nguyên.

Test real HTTP xác nhận tạo user thường và audit, còn test trực tiếp xác nhận user thường hoặc admin bị reset TOTP không thể tạo tài khoản. Đây là phân tách bootstrap/offline với thao tác web, không phải giấy phép cho remote TaskDeck.


### Ràng buộc E2EE trước khi đốt ticket và xác minh quyền theo từng hành động

**P1 đã giảm thiểu — ticket bị đốt bởi yêu cầu sai E2EE:** Relay/Agent trước đây ghi ticket/JTI vào replay ledger rồi mới đối chiếu nonce + P-256 key hoặc binding của E2EE channel; request sai có thể làm ticket hợp lệ bị mất hiệu lực. Nay kiểm tra chữ ký, claim và binding **trước** khi ghi JTI. Ticket với nonce/khóa sai, điểm P-256 không hợp lệ hoặc Device ID không khớp đều bị từ chối mà không sử dụng hạn mức replay. Ticket đúng vẫn một lần duy nhất và được ghi bền vững; xác nhận sở hữu khóa riêng chỉ đến khi bản tin AES-GCM hợp lệ.

**P1 đã giảm thiểu trong adapter chưa gắn dispatcher — thu hồi quyền phiên:** backend `authorizeRemoteAuthenticatedSession` trước đây chỉ kiểm tra quyền local TaskDeck, không đối chiếu grant đang tồn tại tại Portal. Đã buộc phiên chứa signed AgentGrant và chỉ bind một Agent có Device ID, URL Portal issuer và verifier khóa ký đúng đã ghim; mặc định không có bind => cấm toàn bộ hành động. Trước **mỗi** hành động được dự kiến, Agent gửi chứng minh Ed25519 có nonce mới qua HTTPS tới Portal, thất bại hoặc bị thu hồi/tắt tài khoản => từ chối. Không cache kết quả giữa các hành động. Kiểm thử tích hợp thay đổi quyền ở giữa hai lần cấp phép, mất kết nối Portal, sai Agent/issuer/signer và hết hạn/đóng channel.

**Vẫn còn rủi ro:** Chưa có dispatcher remote thực sự; khi mở phải kiểm tra từng lệnh, giới hạn/quy định timeout, kill/cancel hành động đang chạy khi quyền bị thu hồi, chống TOCTOU giữa check và thực thi dài, audit bắt buộc, giảm rủi ro DoS khi nhiều action cần live-check. Đây không phải chứng nhận bảo mật hệ thống đầu-cuối.


### Chống hoán đổi grant giữa các Device ID qua kênh E2EE

**P1 đã giảm thiểu — cùng Browser binding nhưng khác danh tính Agent:** khóa P-256 và nonce của Browser chỉ xác định một phía của phiên, nên hai Agent có Ed25519 Device ID khác nhau có thể tạo ra kênh E2EE khác nhau nhưng cùng `BrowserTicketBinding`. Trước đây `AgentGrantVerifier` chỉ kiểm tra Device ID trong ticket và Browser binding, không đối chiếu Device ID của handshake được xác thực bởi kênh. Nay `Channel` giữ Device ID đã xác minh từ chữ ký Agent và `AgentGrantVerifier.Consume` yêu cầu trùng Device ID trước khi đốt JTI. `Close` xóa binding; test chủ động giả lập hai kênh khác Device ID dùng chung khóa Browser/nonce và chứng minh token không thể chuyển kênh hoặc bị mất hiệu lực do thử sai.

Chưa bật dispatcher TaskDeck thực tế; cần tiếp tục xem xét hủy hành động dài đang chạy ngay khi Portal thu hồi quyền, giới hạn lưu lượng/chi phí kiểm tra grant theo từng hành động và stress-test qua proxy mạng thật.


### Thu hồi MFA sống, TOCTOU đổi mật khẩu và reset MFA qua HTTP

**P1 đã giảm thiểu — Agent live grant chưa xét TOTP:** `Store::eligibleRemoteRole` trước đây vẫn có thể báo grant hợp lệ cho tài khoản chưa có TOTP nếu một đường nội bộ gọi trực tiếp, dù HTTP ticket issuance đã chặn. Truy vấn quyền dùng chung nay yêu cầu MFA còn hoạt động; kiểm thử ký proof Ed25519 chứng minh không có TOTP thì Agent grant check trả false, có MFA mới được phép, reset MFA offline lập tức làm mất điều kiện nhận quyền.

**P1 đã giảm thiểu — race đổi mật khẩu với reset MFA:** mật khẩu cũ được xác minh trước bước tính hash tốn CPU, còn lần UPDATE trước đây chỉ so sánh password_hash, không so sánh thế hệ xác thực và trạng thái MFA đã xác minh. Đặc biệt đọc `userById()` sau UPDATE có thể lấy nhầm `auth_epoch` do worker reset MFA vừa tăng, khiến browser session sai nhận generation mới. Nay UPDATE compare-and-swap ba thành phần password_hash/auth_epoch/mfa_secret_enc dưới `BEGIN IMMEDIATE`, audit commit cùng transaction và trả generation chính giao dịch sinh ra. Test mô phỏng snapshot cũ, tăng auth_epoch/đổi MFA ở worker khác trước commit và xác nhận lệnh cũ bị từ chối, không thay mật khẩu hoặc ghi audit sai.

**P1 đã giảm thiểu — quên khóa CLI-only reset MFA ở tầng Store:** `resetTotpForRecovery` trước đây chỉ phụ thuộc vào việc router không đăng ký endpoint. Bất kỳ route PHP mới nào gọi thẳng Store có thể làm lộ chức năng reset khi tài khoản chưa được xác minh với operator. Nay method tự kiểm tra `PHP_SAPI==='cli'`, giữ thao tác chỉ cho operator host và kiểm thử qua PHP HTTP thật. Giới hạn này không phòng chống PHP worker bị xâm nhập có quyền hệ điều hành tương đương operator; cần cách ly user/host trên shared hosting.

**Rủi ro còn mở:** chưa có dispatcher Remote Control thực tế hay hủy ngay các tác vụ dài khi quyền bị thu hồi, chưa stress-test mạng thật/NAT/reverse-proxy, chi phí live grant introspection cần hạn mức hợp lý khi tích hợp.


### Agent giới hạn một lượt đăng nhập dù callback TaskDeck sai

**P1 giảm thiểu — callback là điểm duy nhất tiêu thụ grant:** trước đây `verifyEncryptedLogin` dựa vào `TakeForLogin` do TaskDeck callback gọi; nếu một callback khác bị cấu hình hoặc refactor nhầm, nó có thể không tiêu thụ grant, khiến cùng một ticket gây nhiều lần xử lý mật khẩu. Agent nay kiểm tra signer/verifier đã ghim, Device ID và cùng đối tượng kênh E2EE, khóa `loginStarted` bằng atomic compare-and-swap **trước** khi gọi Portal hay dùng quota scrypt, và không giải phóng khóa khi thất bại. Sau callback, Agent bắt buộc xác nhận Browser đã gửi bản tin AES-GCM xác thực đầu tiên và grant đã được chính callback tiêu thụ; nếu không, đóng grant và từ chối kết quả, kể cả callback trả thành công. Grant đã tiêu thụ từ trước không thể tái đưa vào Agent để lách chốt.

Kiểm thử gồm 40 goroutine thử dùng chung ticket, các grant ký riêng vượt ngưỡng 5 lượt scrypt, callback giả thành công, phiên khác verifier/đã dùng, thu hồi Portal trước khi gọi TaskDeck và yêu cầu giải mã ciphertext đúng trình tự. Cơ chế này **không** cho phép Terminal/File/Git/Patch từ Internet. Rủi ro còn mở gồm khống chế flood phân tán ở edge, hủy công việc dài khi revoke và dispatcher theo từng quyền ở từng hành động.


### Chặn callback trả phản hồi đăng nhập plaintext hoặc ciphertext không hợp lệ

**P1 giảm thiểu — rò rỉ do callback tự trả dữ liệu ra Relay:** Agent trước đây gửi trực tiếp các byte do callback đăng nhập TaskDeck trả về, chỉ trông chờ callback mã hóa AES-GCM và không bao gồm dữ liệu khác ngoài kết quả boolean. Một callback sai/được refactor có thể trả plaintext chứa tên tài khoản, mật khẩu, dữ liệu riêng hoặc một ciphertext có token/metadata không được phép; Relay sẽ chuyển tiếp byte đó. Nay Agent tự mở lại frame cuối đã `Seal` bằng **send AES-GCM key** để xác nhận MAC, đúng sequence/direction/transcript mà không thay đổi nonce/counter; sau đó chỉ chấp nhận đúng JSON boolean `taskdeck-e2ee-login-result-v1`, không thêm key, không duplicate, không cấp Remote Control. Khi sai, Agent không chuyển tiếp gì và đóng cả grant lẫn Channel. Kiểm thử ở crypto và Agent xác nhận các kiểu sai, kể cả callback đã gọi `TakeForLogin` nhưng trả token hoặc plaintext.

Rủi ro còn mở: helper kiểm tra này chưa phải bảo vệ cho dispatcher Remote Control chưa được triển khai; vẫn cần cơ chế hủy tác vụ dài, giới hạn luồng, audit và RBAC từng lệnh trước khi mở. Một callback nội bộ bị xâm nhập vẫn có thể lạm dụng logic xác minh mật khẩu trong tiến trình TaskDeck; kiểm soát này chặn dữ liệu trả sai ra Relay, không thay thế sandbox/OS isolation.


### Chống DoS bằng proof từ thiết bị đã ghép cặp nhưng bị thu hồi quyền

**P1 giảm thiểu — yêu cầu bị từ chối không bị tính quota:** `checkCurrentAgentGrant` trước đây chỉ dành một lượt trong ngân sách 90 proof/phút nếu grant còn hợp lệ. Một thiết bị đã ghép cặp nhưng người dùng bị thu hồi quyền vẫn có thể gửi liên tục proof Ed25519 hợp lệ để gây tải CPU/SQLite mà không mất quota. Nay sau xác thực chữ ký và xác nhận Device ID còn enabled, quota chung được tính **trước** khi xét quyền người dùng, MFA và `auth_epoch`; grant bị từ chối vẫn mất một lượt, không ghi nonce replay. Thiết bị chưa được ghép cặp không tạo bản ghi hạn mức.

**P1 giảm thiểu — rollback quota khi replay đúng chữ ký:** với cùng nonce/chữ ký đã dùng, trước đây heartbeat/grant-check ném lỗi replay và rollback `BEGIN IMMEDIATE`, vô tình hoàn lại hạn mức vừa dành. Nay lưu quota thành công trước khi trả lỗi replay; không gia hạn presence, không ghi thêm nonce, không cấp phép. Test gồm replay heartbeat và grant thật, 90 lần gọi grant sau revoke, yêu cầu thứ 91 bị chặn, bảng nonce không tăng.

**Rủi ro còn lại:** WAF/reverse proxy vẫn phải hạn chế invalid/unauthenticated requests từ nguồn phân tán vì chúng có thể tiêu tốn chi phí xác minh chữ ký trước khi xác định paired identity. Cơ chế này không giải quyết việc kết thúc tác vụ Remote Control dài đang chạy (dispatcher chưa bật).


### Relay giới hạn tốc độ mở WebSocket, không chỉ số socket đồng thời

**P1 giảm thiểu — handshake churn DoS:** Relay trước đây giới hạn 256 Agent/64 Browser socket tổng và 64 Agent/8 Browser socket trên mỗi nguồn IP, nhưng giải phóng slot ngay khi socket đóng. Một nguồn có thể mở/đóng liên tục, bắt server lặp WebSocket handshake/challenge/crypto mà không bao giờ chạm trần socket đang mở. Đã bổ sung ngân sách theo khoảng 60 giây: **Browser 40/IP và 300/Relay; Agent 180/IP và 2400/Relay**, độc lập với số kết nối hiện hành. HTTP 429 được trả về **trước** khi nâng cấp WebSocket. Dùng IP TCP thực, không tin `X-Forwarded-For`; IPv4 mapped IPv6 chuẩn hóa chung. Map IP tối đa 4096 mục cho mỗi loại, dọn bản ghi cũ và từ chối khi tràn; lùi thời gian không phục hồi quota.

Đã có test đồng thời nhiều goroutine, nhiều IP, giả XFF, tự ngắt socket liên tiếp và test WebSocket TLS thật với kết nối thứ 41 bị chặn dù slot đều đã trả. Giới hạn này là *process-local*, không đồng bộ nhiều replica hoặc bảo vệ chống DDoS phân tán; reverse proxy chia sẻ một IP cần điều chỉnh quota hoặc dùng edge limit riêng. Remote Control vẫn chưa bật.

### Bổ sung: flood bản tin qua WebSocket Agent đã xác thực (2026-10-28)

**P1 đã giảm thiểu — giới hạn handshake không giới hạn traffic trong socket:** giới hạn 180 handshake Agent/phút/nguồn và số socket đồng thời không ngăn một Agent đã ghép cặp giữ một WSS kết nối hợp lệ rồi gửi liên tục các `ping` có lease đã ký, gây tải xác minh Ed25519, giải mã JSON và thao tác Registry. Lỗ hổng này có thể làm mất tính sẵn sàng Relay mà không phát sinh thêm handshake hay slot.

Relay nay dùng token bucket **riêng cho từng kết nối Agent đã xác thực**, tối đa 12 bản tin bùng phát, nạp thêm một bản tin mỗi 2 giây, không tích lũy quá 12. Hạn mức được áp dụng trước cả giải mã JSON và xác minh lease trong vòng xử lý bản tin; quá hạn mức đóng socket, giải phóng Registry/slot. Lùi đồng hồ không nạp lại quota. Các Agent ở kết nối khác không chia sẻ quota. Kiểm thử đơn vị xác nhận từ chối ngay bản tin thứ 13 và tính toán thời gian; kiểm thử TLS/WSS với Ed25519 thực xác nhận phản hồi `pong` cho 12 bản tin hợp lệ rồi đóng khi flood. Kiểm thử Agent đăng nhập E2EE theo nhiều ticket được điều chỉnh tốc độ ping giả lập 2 giây/lần để không vô tình mô phỏng flood trong một bài kiểm tra hành vi hợp lệ.

**Không phải bảo vệ tuyệt đối:** một tác nhân kiểm soát hàng loạt định danh Agent hợp lệ vẫn có thể tạo tải tổng lớn; cần giới hạn CPU/memory ở hệ điều hành, ingress TLS/edge, per-identity circuit breaker và vận hành khóa an toàn. Phiên bị đóng có thể kết nối lại trong giới hạn handshake; không được coi là cơ chế thu hồi quyền tức thời. Portal chưa thay thế được mô hình xác thực TaskDeck riêng và không có Remote Control dispatcher. Không thay đổi yêu cầu **cấm mở luồng HTTP/WS/Terminal/File/Git/Patch** trước khi hoàn tất các release gate.

### P1 giảm thiểu: race thu hồi quyền trong khi HTTPS Portal đang kiểm tra

**Lỗi:** `VerifyCurrentPortalGrant` và adapter `authorizeRemoteAuthenticatedSession` ban đầu chỉ kiểm tra grant/session/channel lúc bắt đầu. Nếu người dùng đóng tab, Agent bị ngắt E2EE, hoặc quyền bị đóng **trong lúc chờ HTTPS Portal**, một phản hồi `allowed=true` đến muộn có thể khiến lớp kiểm tra nội bộ trả thành công cho phiên không còn sống. Đây là rủi ro TOCTOU tại biên bảo mật, mặc dù dispatcher Remote Control vẫn chưa được kích hoạt.

**Sửa:** Agent tái kiểm tra grant còn do chính verifier đã ghim sở hữu, chưa đóng và Channel còn xác thực **sau khi** Portal trả kết quả. TaskDeck session nay gọi `Channel.Close()` khi đóng, xóa binding E2EE và vô hiệu hóa khóa AEAD. Sau thao tác tra cứu quyền local, adapter cũng tái kiểm tra session chưa đóng, grant còn hoạt động, Channel đúng đối tượng và hạn phiên 5 phút. Không dùng lại hạn 30 giây của rendezvous ticket để xác định vòng đời session đã xác thực.

**Bằng chứng kiểm thử:** các test có barrier channel buộc Portal callback trả `allowed=true` sau khi grant/session/channel đã bị đóng, yêu cầu từ chối cuối cùng. Bài test kiểm tra channel cũng không còn `Established` sau `Close()`.

**Rủi ro còn mở:** tái kiểm tra không phải mutex giữ quyền suốt cả quá trình thực hiện lệnh. Một dispatcher trong tương lai phải phối hợp lock/lease theo mỗi lệnh, hủy PTY/HTTP/file stream đang chạy khi revoke và không có side effect mới sau khi session đóng. Đợt này không mở Terminal/File/Git/Patch/HTTP từ Internet.

### Hồi quy P1: đóng E2EE quá sớm trong luồng đăng nhập một lần và bỏ sót test

**Phát hiện:** `remoteTaskDeckSession.Close()` gần đây được tăng cường đóng channel E2EE khi thu hồi quyền. Tuy nhiên, luồng `VerifyEncryptedRemoteLogin` dùng chung hàm đóng session, khiến channel bị vô hiệu **trước khi mã hóa boolean đăng nhập**. Browser không nhận kết quả hợp lệ. Workflow Go trước đó chỉ lọc `TestRemote`, bỏ qua `TestEncryptedTaskDeckLogin*`, nên CI xanh dù nhánh mã này có lỗi.

**Sửa:** Phân biệt kết thúc one-shot login probe (chỉ hủy principal và grant đã dùng, để Agent gửi boolean E2EE rồi tự đóng kết nối) với kết thúc application session (đóng toàn bộ channel). Chặn replay grant đã đóng/hết hạn hoặc gắn sai channel bằng cách hủy channel ngay. Mở rộng test Go server chạy race detector và luôn bao gồm `TestEncryptedTaskDeckLogin`.

**Thu hồi quyền trong khi xác thực:** Một Portal introspection trước khi chạy scrypt không đảm bảo quyền vẫn tồn tại khi kết quả xuất hiện. Agent nay kiểm tra grant live lần hai *sau* callback TaskDeck và *trước* chuyển ciphertext đăng nhập. Portal bị ngắt mạng/revoke/role downgrade ở bước cuối => đóng grant và channel, không trả result. Đây là phòng vệ tại thời điểm trả lời, không thay thế cơ chế hủy lệnh đang chạy cho dispatcher tương lai.

**Giới hạn:** Việc đăng nhập vẫn là probe boolean được mã hóa, không cấp token hoặc quyền điều khiển TaskDeck. Chưa có transport ứng dụng E2EE, kiểm tra RBAC mỗi hành động và cơ chế hủy tác vụ khi revoke.

### P1: race giữa hủy grant và xác nhận đăng nhập E2EE một lần

**Phát hiện:** Probe đăng nhập TaskDeck trả đúng một boolean đã mã hóa, nhưng cần hủy principal tạm trước khi hồi đáp. Trước đây cleanup đồng thời đóng `AgentGrant`; để đăng nhập bình thường thành công, Agent phải bỏ qua trạng thái grant đã đóng khi kiểm tra quyền lần hai. Như vậy, một lệnh thu hồi grant xảy ra độc lập trong lúc tính scrypt hoặc trong lúc chờ HTTPS Portal vẫn có nguy cơ bị nhầm với cleanup bình thường, dẫn đến phản hồi đăng nhập thành công đã cũ.

**Đã giảm thiểu:** tách đóng principal nội bộ khỏi việc đóng grant/Channel. Probe một lần chỉ hủy principal, còn Agent kiểm tra lại grant chưa bị hủy, Channel E2EE còn hợp lệ, request context chưa hủy sau bước xác thực và sau kiểm tra Portal mới nhất. Agent sử dụng CAS nguyên tử trên cờ đóng của grant để quyết định giữa kết thúc đăng nhập và thu hồi: revoke đến trước thì không phát ciphertext, đóng E2EE; thành công đến trước thì grant được đánh dấu đã dùng trước khi phát boolean. Session ứng dụng thông thường vẫn đóng toàn bộ grant và Channel.

**Kiểm thử hồi quy:** dùng barrier đồng bộ để chủ động hủy grant trong khi callback mật khẩu hoặc kiểm tra HTTPS Portal thứ hai đang chạy; cả hai trường hợp phải từ chối và vô hiệu hóa Channel. Kiểm thử đường thành công yêu cầu 2 lần Portal check và grant đã đóng. Các test này **không chứng minh** khả năng hủy một tác vụ đã bắt đầu: remote dispatcher vẫn chưa hoạt động.

**Còn mở:** thu hồi quyền Portal vẫn có độ trễ hữu hạn do mạng và thời điểm phản hồi; khi kích hoạt thực thi lệnh cần thêm cơ chế dừng side effect đang chạy, backpressure, giới hạn tài nguyên và một authorization lease có điểm linearization rõ ràng cho từng lệnh.

### P0: Máy chủ PHP bị chiếm quyền và ranh giới tin cậy của mật khẩu TaskDeck

**Kịch bản tối đa:** Hacker có toàn quyền hệ thống shared hosting PHP, sửa được HTML/JS/PHP, đọc SQLite và private Portal signing seed, giả mạo grant/ticket, đóng/mở presence, trả `allowed=true` cho mọi request. Không giả định Portal có thể giữ bí mật hay cung cấp mã JavaScript đáng tin.

**Điều kiện độc lập bắt buộc tại TaskDeck:** (1) ticket/role của Portal chỉ có quyền rendezvous, không phải bằng chứng xác thực TaskDeck; (2) TaskDeck tự kiểm chứng mật khẩu thật bằng local identity/password hash và kiểm tra project/membership/RBAC; (3) tất cả kết quả/phản hồi xác thực thuộc đúng channel E2EE đã xác nhận danh tính thiết bị, không phát cookie/bearer ra Portal; (4) từng lệnh sau này phải có RBAC local, kiểm tra thu hồi và hủy thao tác; (5) không có đường HTTP/terminal/files/git/patch từ Relay vào TaskDeck trước security gate. Các kiểm thử Go mô phỏng Portal làm chủ khóa ký và tự cấp role admin vẫn không thể biến mật khẩu sai/trống thành login thành công; mật khẩu đúng chỉ được trả boolean mã hóa một lần.

**Lỗ hổng P0 đã xác định trong giao diện trước đây:** Portal đã từng phục vụ form `taskdeck_password` cùng `remote_login.js` và codec WebCrypto xử lý mật khẩu. Khi bị hack, PHP chỉ cần thay JS trước khi người dùng nhập: E2EE không thể bảo vệ bí mật trước mã JavaScript độc hại chạy trong cùng trang. Vì vậy cam kết 'PHP bị chiếm quyền nhưng tuyệt đối không thể lấy mật khẩu TaskDeck' **không đạt được** với client trước đây, mặc dù PHP backend bình thường không lưu password.

**Giảm thiểu ngay:** Gỡ form/JS đăng nhập khỏi Portal, xóa script từ public webroot, chuyển codec xử lý credential sang fixture kiểm thử ngoài webroot, chỉ giữ chức năng chẩn đoán E2EE không mật khẩu; cập nhật development router và CI để chống hồi quy. Portal hiển thị cảnh báo không nhập mật khẩu TaskDeck. Đây là giảm bề mặt tấn công ở bản phân phối chuẩn, **không phải** cơ chế chống lại attacker đã có quyền tùy ý sửa PHP/HTML/JS.

**Yêu cầu trước khi phát hành:** Phải có client độc lập được ký và xác minh nguồn gốc (native/extension/origin khác được bảo vệ), không bao giờ tải mã nhập mật khẩu từ Portal, ghim Device ID/khóa Agent ngoài kênh Portal, giữ private E2EE key trên client, kiểm tra quyền tại TaskDeck per-action và kiểm thử giả lập PHP đã chiếm quyền trên môi trường thực. Chưa đáp ứng thì Remote Control bị khóa. Không thể khẳng định bảo vệ trước người dùng bị lừa nhập mật khẩu vào trang PHP độc hại, dù thuật toán mật mã không bị phá.
