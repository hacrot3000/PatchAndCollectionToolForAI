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


### Guard mới: PHP/Relay bị chiếm không được đưa lệnh JSON nhập nhằng vào Agent (2026-10-29)

Agent xác thực danh tính Ed25519 và kiểm tra mật khẩu TaskDeck cục bộ là hai biên tin cậy **độc lập với khóa ký Portal**. Tuy vậy, tệp remoteportal/relay.go trước đây dùng json.Unmarshal trực tiếp cho lệnh do Relay gửi tới Agent, cho phép JSON có trường trùng, tên trường chữ hoa/thường nhập nhằng và trường không nhận biết. Đây là bề mặt **protocol confusion / future regression** khi PHP hoặc Relay bị chiếm quyền; không có bằng chứng kiểu JSON này đã vượt qua mật khẩu hoặc kích hoạt Remote Control.

Đã thay bằng decodeRelayCommandFrame: chấp nhận đúng ba loại tin nhắn reachability_probe, e2ee_login, e2ee_echo, mỗi loại chỉ có đúng những khóa bắt buộc, dạng chuỗi không rỗng; từ chối trùng khóa kể cả Unicode-escaped alias, sai chữ hoa/thường, JSON lồng nhau, lệnh thêm, trường password/bearer, phần nối dư/trailing JSON và thông điệp vượt giới hạn. Không thay đổi cách xác thực mật khẩu scrypt hoặc cấp quyền TaskDeck; không mở API mới. Hồi quy TestRelayCommandEnvelopeRejectsCompromisedPHPJSON gồm 21 tình huống độc hại và hợp lệ. Nguồn cập nhật: internal/remoteportal/relay.go và relay_test.go; bản sửa cuối import trùng ở commit 3ad0b73595feddf71fd5836379593cb8242be197.

**Tiêu chí kiểm chứng:** [CI Go Remote Agent 40826224243](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40826224243) **SUCCESS** trên đúng source commit 3ad0b73595feddf71fd5836379593cb8242be197, gồm 21 ca strict JSON Relay-to-Agent, kiểm thử Go -race Agent/Relay/E2EE, Go server security, build trusted client và cross-compile macOS/Linux ARM. Các run trung gian 40826118344 và 40826171165 thất bại do import io khai báo trùng, đã sửa ở commit trên.

**Rủi ro không thể loại bỏ chỉ bằng mã phía PHP:** nếu người dùng đặt **cùng mật khẩu Portal và TaskDeck**, kẻ chiếm PHP/SQLite có thể thử bẻ hash của Portal để suy ra mật khẩu TaskDeck; nếu người dùng làm theo chỉ dẫn giả của trang PHP và nhập mật khẩu TaskDeck vào đó thì mật khẩu bị lộ. Cần mật khẩu TaskDeck **riêng biệt, đủ mạnh**, chỉ nhập qua native client tin cậy sau khi kiểm chứng Device ID Agent ngoài PHP. Không quảng bá rằng hệ thống không thể có bất kỳ TCP/WSS handshake nào: Portal bị chiếm có thể khởi tạo handshake; điều bị TaskDeck từ chối là **xác thực/điều khiển ứng dụng không có mật khẩu TaskDeck hợp lệ**. Remote Control public vẫn **NO-GO**.


### Rà soát xác thực độc lập PHP lần nữa — 2026-10-29

**Đường thực tế đã kiểm kê:** Portal cấp ticket/lease và kiểm tra grant qua HTTPS; Relay chỉ chuyển thông điệp E2EE và không có endpoint HTTP/terminal/file của TaskDeck; Agent xác minh ticket đã ghim, thiết lập Ed25519/P-256 E2EE, dùng quota login bền vững; TaskDeck Shared Identity xác minh mật khẩu scrypt cục bộ, tài khoản/membership, cấp duy nhất boolean E2EE cho một lần thử. Phiên ứng dụng có thời hạn chỉ tồn tại trong backend staging **không có public route**. Đường HTTP TaskDeck riêng biệt vẫn phải có session cookie thật của TaskDeck; không chấp nhận bearer hoặc ticket do PHP tự ký.

**Phát hiện và sửa:** Bộ kiểm tra E2EE một lần VerifyEncryptedRemoteLogin có thể hoàn tất scrypt và kiểm tra membership, rồi bị trì hoãn tại lần ghi audit cục bộ. Nếu chủ máy TaskDeck đổi mật khẩu, vô hiệu hóa người dùng hoặc thu hồi membership trong lúc audit, mã trước đó vẫn có thể niêm phong boolean authenticated=true từ snapshot cũ. Đây là **sai lệch xác thực trong thời gian ngắn**, chưa phải khả năng tạo phiên ứng dụng hoặc thực hiện lệnh Remote Control. Đã bổ sung verifyRemoteSessionLocalPrincipal tại bước cuối trước closeWithoutChannel và niêm phong kết quả; thay đổi hoặc lỗi đọc identity => authenticated=false. Commit nguồn f6534445bfeadef564f358dc2b810ed4b8666aba.

**Kiểm thử mới:** TestCompromisedPortalCannotReportStaleOneShotLoginAfterLocalRevocationDuringAudit dùng audit barrier đồng bộ và ba cách thay đổi độc lập tại TaskDeck: thay hash mật khẩu kể cả backdate timestamp, vô hiệu hóa tài khoản, và thu hồi membership; yêu cầu E2EE boolean từ chối. Commit 553c44357fe16136a9e643c2c5e9ca8c02b8573b. TestCompromisedPHPAdminTicketNeverAuthenticatesTaskDeckHTTP gửi ticket đã ký bởi khóa Portal mà hacker kiểm soát vào Authorization header, cookie giả và header admin trên **bảy đường HTTP thực** của TaskDeck, kiểm tra middleware từ chối và không cấp session cookie. Commit 877b5d07a1fe1a4e525a21c447c1d242c69f5865.

**CI:** [run 40832710932](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40832710932) SUCCESS cho bộ kiểm thử thu hồi giữa audit. [run 40832980357](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40832980357) **SUCCESS** trên đúng source commit 877b5d07a1fe1a4e525a21c447c1d242c69f5865: 7 tuyến HTTP thực từ chối Portal admin ticket giả làm bearer/cookie và không phát hành TaskDeck session; Go Agent/Relay/transport -race, HTTP/UI server security, Go↔WebCrypto interoperability, trusted CLI builds và cross-compile macOS/Linux ARM đều PASS.

**Phạm vi bảo đảm:** Nếu hacker chỉ có toàn quyền PHP/SQLite/Portal signer nhưng không có mật khẩu TaskDeck, private key Agent hoặc máy TaskDeck, các đầu vào Portal không thể trở thành session cookie TaskDeck, không thể vượt qua scrypt/local RBAC và không thể kích hoạt Remote Control qua Relay. Một handshake WSS không đồng nghĩa xác thực. Giới hạn ngoài threat model gồm phishing, người dùng dùng cùng mật khẩu Portal–TaskDeck, malware trên máy TaskDeck, và rủi ro phiên đăng nhập có mật khẩu đúng bị thu hồi sau thời điểm kết quả chẩn đoán đã được tạo. Vì public dispatcher vẫn chưa được triển khai và kiểm chứng, **Remote Control production tiếp tục NO-GO**.


## Bắt buộc duyệt khóa client bằng CLI tại TaskDeck host (2026-10-29)

**Yêu cầu được chốt:** Mọi Remote login phải dùng client native độc lập; PHP Portal chỉ cung cấp quyền rendezvous/ticket và không chứa biểu mẫu TaskDeck password. Mỗi client tạo Ed25519 key riêng theo TaskDeck Device ID đã ghim và username. Public key có thể sao chép, nhưng private seed 0600 phải ở trên máy client, trong thư mục riêng 0700. Client không tải JavaScript/binary từ PHP.

**Lần đầu:** Native client tự tạo key, hiển thị cảnh báo cùng PUBLIC key dạng Base64 và fingerprint, bật popup native macOS `osascript` hoặc Linux `zenity`/`kdialog` (có fallback hiển thị ở terminal). Không hỏi mật khẩu TaskDeck trong lần tạo key. Public key không tự được phê duyệt.

**Trên TaskDeck host:** Chỉ local command trên đúng tài khoản Unix chạy daemon có quyền chỉnh danh sách:
```bash
taskdeck remote-key --workspace /absolute/project list
taskdeck remote-key --workspace /absolute/project add alice 'BASE64_PUBLIC_KEY' 'My Linux laptop'
taskdeck remote-key --workspace /absolute/project revoke '64_HEX_FINGERPRINT'
```
Không có API PHP/Relay/HTTP để add/list/revoke. Dữ liệu trong private host config theo workspace: `remote-identities/<SHA256 workspace>.clients.json`, 0600, dir 0700; file lock riêng và atomic rename+fsync. 1 public key chỉ gắn 1 local username; một username có nhiều key. Danh sách rỗng => **từ chối** Remote login. Các kiểm tra signature bằng Go Ed25519 thực hiện trước scrypt, không tin bất kỳ `admin` Portal role nào để thêm key.

**Đăng nhập các lần sau:** Agent xác thực bằng Device ID đã ghim + E2EE P-256, native client ký `taskdeck-remote-client-login-v1 || 0x00 || lower(username) || 0x00 || SHA256(transcript)` với private key cục bộ. Mật khẩu và chứng minh key được mã hóa trong **cùng record AES-GCM v2**. TaskDeck host tự kiểm tra chữ ký, đúng user, key tồn tại trong allowlist rồi mới thử mật khẩu TaskDeck cục bộ và membership/RBAC. Mọi v1 password-only login, dù mật khẩu đúng, đều bị từ chối. Giao diện nhận được denial phải hướng dẫn copy public key, chứ không cấp ứng dụng hoặc đăng nhập bằng fallback password-only.

**Thu hồi:** `remote-key revoke` chặn đăng nhập mới ngay. Các staged application session được tạo qua entrypoint có key đã xác minh cũng đọc lại allowlist ở lần kiểm tra local principal tiếp theo, khiến khóa đã thu hồi mất quyền; thời hạn session tối đa vẫn 5 phút. Không cho tự khôi phục từ PHP.

**Kiểm tra trạng thái khóa trước mật khẩu (đã bổ sung):** Native client chỉ hỏi mật khẩu sau khi Agent đã ký transcript/thiết lập E2EE và trả về một bản tin AES-GCM xác nhận public key + chữ ký Ed25519 thuộc allowlist tại host. Relay có một mailbox key-check riêng, chuyển opaque ciphertext và cho phép đúng một login sau preflight; diagnostic echo thông thường không được nâng thành login. Bản tin `approved=false`, lỗi hoặc bị PHP/Relay thay thế khiến client không hỏi mật khẩu, bật popup native hiển thị public key để copy. Server TaskDeck tiếp tục tự kiểm tra allowlist cùng chữ ký trong login v2, ngay cả khi Portal/Relay nói `allowed=true`. **Giới hạn:** Client độc lập hiện là CLI kết hợp popup của macOS/Linux, không phải trang HTML do PHP gửi. Remote Control và long-lived dispatcher vẫn unmounted / NO-GO. Chỉ xác nhận PASS khi CI kiểm thử mã cuối hoàn tất thành công.

 
**CI acceptance for host CLI/software-key integration (2026-10-29):** [TaskDeck Remote Agent run 40845687947](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40845687947) **SUCCESS**, tested exact code commit `f6c3ace2376100e9b8b4dbf67bbbb6c7cd7c6851`. Includes host-only CLI add/list/revoke, client-owner file permissions, real TLS/WSS encrypted preflight and login v2, forged Portal status and stolen-password denial, local key revocation during success-audit, `-race` package tests, native CLI builds and Linux/macOS cross-compilation. Production Remote Control remains **NO-GO**, unrelated application command routes were not enabled.


## Rà soát lại khóa client: username Unicode / SQLite NOCASE (2026-10-29)

**Phát hiện và khắc phục:** SQLite `users.username COLLATE NOCASE` chỉ gấp chữ hoa ASCII. Mã Go trước đây dùng `strings.EqualFold` / `strings.ToLower` Unicode khi so sánh `Entry.Username` của host allowlist và khi ký proof cho client. Ví dụ `Karl` (Kelvin sign U+212A) và `Karl`, hoặc `Émilie` và `émilie`, là **hai tài khoản SQLite khác nhau**, nhưng có thể bị bộ so sánh Go xem là cùng tên. Nếu PHP bị chiếm quyền và đánh cắp mật khẩu của tài khoản đích, một client có key hợp lệ của tài khoản Unicode khác có thể bị hiểu nhầm là thiết bị đã duyệt của tài khoản đích. Đây là lỗi ở ranh giới giữa xác thực khóa client và tài khoản cục bộ.

**Đã sửa:** `remoteclientkey.FoldASCIIUsername` và `SameTaskDeckUsername` chỉ gấp ASCII A–Z, được dùng nhất quán ở host allowlist (add, verify, kiểm tra thu hồi), thông điệp chữ ký Ed25519 và kiểm tra principal cuối của TaskDeck. Per-account login rate-limit dùng cùng dạng tên canonical để không vô tình gộp hai tài khoản SQLite độc lập. Tài khoản Unicode khác nhau phải có key được host phê duyệt riêng.

**Siết local policy JSON:** Registry trước đây dựa vào `encoding/json` cho object; mặc định Go cho phép các khóa JSON trùng hoặc tên trường khác hoa/thường. Giờ bộ nạp policy xác thực toàn bộ tên trường chuẩn, trường bắt buộc, duplicate, nested field và trailing payload; mọi nhập nhằng/corruption đều từ chối xác thực, không fallback PHP. Tệp registry vẫn chỉ do host CLI dưới đúng tài khoản Unix thao tác.

**Kiểm thử:** Go unit bao gồm casefold Kelvin/É Unicode và casefold ASCII hợp lệ, kiểm thử *SQLite thật* với hai identity và membership độc lập, trong đó client cầm key phê duyệt cho `Karl` + mật khẩu đúng của `Karl` nhưng E2EE TaskDeck login vẫn trả denial; thêm 8 trường hợp JSON duplicate/case alias/trailing đảm bảo fail-closed. CI [run 40849703869](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40849703869) **SUCCESS** cho source commit `3c7be8add04563a921e43fa5c20af60677f9117c`.

**Đã xử lý rủi ro account-recreation:** Host CLI `remote-key add` bắt buộc tra cứu trực tiếp SQLite trên máy TaskDeck và xác nhận tài khoản `enabled`, project hoạt động, membership `enabled`, rồi gắn approval với **immutable local `users.id`**. TaskDeck server từ chối login v2 và hành động phiên tiếp theo nếu `User.ID` không còn khớp, dù attacker giữ private key đã duyệt cho tài khoản cũ và biết mật khẩu tài khoản mới trùng username. Khi tạo lại tài khoản, người dùng phải `remote-key revoke` khóa cũ và `remote-key add` để phê duyệt lại; CLI không tự chuyển approval sang User.ID mới. Các entry cũ chỉ gắn username được đọc để quản trị, nhưng **không được Agent cho qua pre-password key check hoặc TaskDeck cho đăng nhập** cho tới khi host chủ động re-approve. Thiết kế hiện chưa cấp public Remote Control dispatcher và **vẫn NO-GO**.


### Immutable User.ID host approval validation (2026-10-29)

Host-only registry now stores optional `user_id` on each key entry for backward-readable metadata; **nonempty user_id is mandatory for any positive encrypted Agent key preflight**, and the TaskDeck server checks `IsApprovedForUser(..., actual SQLite users.id, ...)` before releasing authentication success and on each subsequent staged session action. Username-only legacy approvals cannot authenticate; explicit host CLI re-approval upgrades them. The CLI refuses missing SQLite identity DB, disabled/missing user, disabled/missing project or membership, and approval of a key previously bound to a different immutable User.ID without first revoking it. It does not query PHP or use Portal claims.

Tests exercise host CLI with a REAL local SQLite account and verify the immutable user binding; approved key for old User.ID must fail when the same username belongs to a newly created account, and explicit revoke/re-approve works. The real TLS/WSS integration fixture also exercises an ID-bound preflight. [CI 40851369672](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40851369672) **COMPLETED SUCCESS** on the exact latest source commit `34223d1189073e43764b3da28b80d83b57955d9e`. Includes host CLI local SQLite identity lookup, immutable-user key binding and legacy migration tests, Go E2EE and real TLS/WSS Relay tests, TaskDeck HTTP security suite, race tests, trusted native CLI builds and Linux/macOS cross-compilation.

**Remaining UX distinction:** Agent key preflight can only compare registered username and public key because it has no direct SQLite identity read in that isolated transport component. For an old approved key after account recreation it may return initial `approved=true`, but the final TaskDeck authentication is refused by immutable User.ID check. A follow-up may wire an owner-local identity check into the Agent preflight to avoid asking a password in that exceptional case. This cannot grant a TaskDeck session or execute any application operation.


### Host-SQLite-bound approval before password prompt — follow-up (2026-10-29)

**Resolved the remaining key-preflight UX/security gap:** An Agent previously verified that the client public key had an owner-managed approval entry and a valid signature, but it did not ask the *current local TaskDeck SQLite database* for the corresponding immutable User.ID. After a TaskDeck account was deleted and recreated under the same username, an old approved key could receive an encrypted `approved=true` and cause a password prompt, although the final login still refused the old ID.

The owner-only daemon now configures `Agent.ConfigureClientKeyLocalUserLookup` with `Server.LookupRemoteClientLocalUserID` **only when the experimental login-only probe is explicitly enabled**. This callback looks up the TaskDeck shared-identity SQLite user, verifies the account, project, and project membership remain active, and returns the real immutable local User.ID. Agent preflight now checks *both* the Ed25519 private-key proof bound to the current authenticated E2EE transcript and the host-approved key's User.ID against the callback result. No callback, SQL error, disabled account, removed membership, stale key ID, revoked key, Portal response, or canceled context can return an encrypted positive approval. No PHP/Relay/browser-supplied User.ID is accepted.

**New tests:** `internal/remoteportal/client_approval_test.go` proves that missing local lookup denies a password prompt, a deleted/recreated account using the same username cannot inherit the old key, explicit host reapproval works, and a signature for another E2EE channel or another local account fails. `internal/server/remote_access_client_identity_test.go` checks *real SQLite* active/missing/disabled users, project and membership rejection, disabled Remote flag and canceled context. The existing real TLS/WSS Relay-Agent integration now configures a trusted immutable-ID lookup explicitly rather than silently relying on the mere presence of an allowlist record.

**Verified:** [Go Remote Agent CI #40855540805](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40855540805) **COMPLETED SUCCESS** on source commit `35e6e123620e71c4631a1d61c9eff652a127decd`. Covered `-race` Agent/Relay/transport tests, server Remote HTTP/login security, real SQLite client identity tests, host CLI, trusted native client build, WebCrypto interop and Linux/macOS cross-compile.

**Go-live policy unchanged:** The live TaskDeck login remains two-factor (host-approved software key **and** local TaskDeck password). PHP cannot add/revoke client keys, provide private keys or set the local User.ID. The public Remote Control application dispatcher remains unmounted; production remains **NO-GO** pending separate end-to-end application authorization/security testing.

## Tiếp tục 30/10/2026 — tấn công PHP chiếm toàn quyền, có cả mật khẩu TaskDeck

**Code checkpoint:** `b3b065dab7527213d5fedcf6c6a0628a5adcd286`.
**Threat model mạnh hơn:** đối thủ chiếm PHP/SQLite/HTML/JS/MFA,
khóa ký ticket/lease, có thể trả `allowed=true`, sao chép
**host-approved public client key** và thậm chí đã biết đúng **mật khẩu
TaskDeck**. Đối thủ **không** sở hữu native client private Ed25519 key,
không kiểm soát Agent seed/host TaskDeck và không sửa được host allowlist
qua CLI dưới đúng OS user. Trong trường hợp này, test bắt buộc chứng minh
không xuất hiện TaskDeck principal, cookie, session hay action.

**Mã và kiểm thử được thay đổi:**

- `remote_access_session_login_test.go`: hai test mới tách bạch
  (a) mật khẩu thật nhưng key chưa được host duyệt;
  (b) key public đã được host duyệt + mật khẩu thật nhưng chữ ký của
  private key giả. Cả hai phải DENY và đóng E2EE.
- `remote_compromised_portal_relay_test.go`: TLS/WSS Relay thật với
  Portal ký được lease và admin ticket, trả grant `allowed=true`.
  Agent dùng identity/approved-key registry của đúng TaskDeck workspace,
  thử **stolen public key + forged client signature** qua preflight E2EE;
  phải từ chối trước khi đụng mật khẩu. Dù PHP tiếp tục gửi mật khẩu thật,
  Agent không được cho phiên ứng dụng. HTTP forged Portal admin bearer
  cũng bị thử với các route terminal, file mutation, Patch, Git, DB, admin.
- `grant_verdict_strict.go`: chỉ chấp nhận hai boolean canonical
  `{"ok":true,"allowed":true}`. Duplicate JSON, case alias, extra authority
  fields (cookie, role), null/string/nested/trailing JSON phải fail closed.
  Bản thân `allowed=true` không phải quyền TaskDeck.
- `remote_access_session_login_test.go` cũng kiểm tra host revoke
  **client key** ngay giữa hai lần grant-check HTTPS khi PHP treo rồi trả
  `allowed=true`, và revoke khi app session nội bộ đang sống.
- `remoteportal/relay.go` sửa nhịp poll 20s -> 2s: ticket ký chỉ sống
  ~30s, ba lượt probe/preflight/login ở 20s không thể hoàn tất trước expiry.
  2s không vượt budget lâu dài một Agent frame / 2s của Relay.
  `relay_test.go` thêm regression giới hạn thời gian và frame rate.
- `remote_access_session.go` thu hồi grant và xóa AEAD channel dưới
  cùng execution lock **trước** khi báo hủy active callback; loại bỏ cửa
  sổ callback hoàn tất trong khi channel vẫn báo Established.
- `.github/workflows/taskdeck_remote_agent.yml` thêm test red-team
  độc lập chạy dưới `-race` trước full server regression.

**Bằng chứng CI trên cùng source SHA:** 
Go: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40990127102
PHP: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40990127100

Chỉ ghi **FULL CI PASS** khi cả hai run đã kết thúc `conclusion=success`.
Riêng bước red-team chuyên biệt đã được quan sát PASS khi full Go job
còn đang chạy tại thời điểm bổ sung tài liệu.

**Ranh giới và NO-GO:** PHP bị hack vẫn có thể DoS, chặn/revoke grant,
gửi link ứng dụng độc hại hoặc lừa người dùng nhập mật khẩu trên trang giả.
Không thể tuyên bố loại bỏ phishing hoặc availability loss nếu máy chủ
PHP nằm trong tay đối thủ. Bảo đảm ở đây là **không cấp quyền TaskDeck khi
thiếu private key client hợp lệ + password/local RBAC**. Không thể gọi
đạt 100% live Remote Control khi phiên E2EE ứng dụng thực vẫn chưa nối
Internet, terminal/files/Git/Patch chưa có dispatcher an toàn và RC-01..13
chưa PASS. Giữ `remote_control_available=false` và không merge main.

### Xác nhận CI cuối cùng — 30/10/2026

**Source commit đã kiểm thử:** `9dc45e543047af70f190529dcecb8bfb68aa8222`.

Cả hai workflow đều **COMPLETED / SUCCESS trên cùng source SHA**:

- [TaskDeck Remote Agent (Go) #40990613993](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40990613993): `-race` native/Agent/Relay/TaskDeck, red-team PHP compromised signer+real password+stolen approved public key, key revocation during hung PHP, app mailbox internal checks, HTTP authorization, CLI and cross-build macOS ARM/Intel + Linux ARM.
- [TaskDeck Remote Portal (PHP) #40990613987](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40990613987): Portal-side validation/security regression.

The earlier source SHA `b3b065da` passed red-team and full Go tests but its workflow job failed because of a stray shell command after successful cross-builds. The workflow was repaired by `9dc45e54`; only the latter run is used as full CI success evidence.

**Đánh giá riêng kịch bản PHP compromised:** login-only host authorization boundary **PASS ở mức kiểm thử tự động (unit, TLS/WSS real handler và Go race)**. Kẻ có quyền sửa toàn bộ PHP/SQLite/MFA/Portal signer và kể cả biết mật khẩu TaskDeck nhưng không có private Ed25519 key đã được host duyệt không vượt được Agent key preflight / local TaskDeck principal. PHP admin ticket không mở được các API Terminal/Patch/Git/Files/DB/Admin; khoá đã thu hồi không thể sống lại nhờ phản hồi `allowed=true` muộn.

**Không đồng nghĩa 100% production security hoặc GO Remote Control:** một PHP đã bị hack vẫn có thể DoS/phishing/phân phối file cài giả; con người phải pin Device ID và xác minh native release độc lập với PHP. Chưa có production native↔Relay↔Agent application route, Terminal/Files/Git/Patch bounded action dispatcher, kiểm thử end-to-end qua hạ tầng CGNAT/hosting thật, ký và xác minh binary release vật lý, pentest độc lập, và toàn bộ RC-01 đến RC-13 đều PASS. Không mở cổng TaskDeck, không merge `main`; giữ `remote_control_available=false` và **Remote Control NO-GO**.

## Kiểm định tiếp tục 2026-10-30: chống giả Agent theo workspace và bỏ qua private key

**Source checkpoint:** `1ca9326dc3b02ca9e555ab8e080c5fe915e575ae`.

### Lỗ hổng phòng ngừa / hardening đã commit

1. **Workspace Agent identity binding** (`remote_access_workspace_agent.go`, `remote_access_session_login.go`): bước tạo phiên ứng dụng `StartEncryptedRemoteSession` nay đối chiếu **ba thứ cùng một lúc**: Agent Device ID, `Channel.BoundDeviceID()` và fingerprint của Agent Ed25519 seed đặt trên **đúng host/workspace TaskDeck**. Portal origin của Agent phải trùng URL trong cấu hình host. Không nhận Device ID, Portal URL hoặc identity seed từ PHP/Relay; seed mất thì DENY, tuyệt đối không tự tạo key mới trong yêu cầu đăng nhập. Các test `TestRemoteStageRejectsCrossWorkspaceAgentAndPortalIdentity`, `TestRemoteStageRejectsMissingHostIdentityWithoutRegeneration` và `TestCompromisedPHPAgentSubstitutionCannotEnterTaskDeckSession` chứng minh từ chối khi giả Agent, thay origin hoặc seed biến mất dù client có key/password thật.
2. **Chặn password-only tại cửa application action** (`remote_access_authorization.go`, `remote_access_session.go`): phiên nội bộ chưa gắn `clientProofVerified=true`, `clientPublicKey` và tên tài khoản được xác minh sẽ bị **đóng** trước mọi callback. Chỉ `StartEncryptedRemoteSession` gắn bit này sau khi `remoteclientkey.Verify` kiểm tra Ed25519 signature trên chính transcript E2EE và local host allowlist; mỗi action sau đó vẫn đọc lại allowlist, TaskDeck password generation, membership/RBAC và Portal grant. Test `TestCompromisedPortalCannotExecuteWithPasswordOnlyOrCopiedPublicClientKey` chứng minh attacker có Portal admin ticket + TaskDeck password, hoặc chỉ sao chép public key, không thể gọi callback.
3. **Test fixture được nâng chuẩn**: các unit/Relay in-process fixtures mô phỏng một client key đã được host duyệt; không còn mặc định coi password-only là phiên điều khiển. Đây chỉ là mô phỏng nội bộ, **không phải** kiểm thử production WSS action route.

### CI cùng một source SHA — tất cả SUCCESS

- [Go Agent, -race red-team + local session/Agent binding + HTTP auth + cross-build, run 40992679028](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40992679028) — **COMPLETED SUCCESS**.
- [PHP Portal security regression, run 40992679113](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40992679113) — **COMPLETED SUCCESS**.

Đây là PASS của kiểm thử bảo mật **ranh giới đăng nhập và action nội bộ** khi PHP/Relay đã bị chiếm quyền, **không phải** chứng nhận toàn bộ hệ thống không thể bị hack.

### Điều kiện chưa đạt — tuyệt đối chưa chuyển Remote Control sang GO

`StartEncryptedRemoteSession` và mailbox ứng dụng vẫn **chưa được kết nối vào production Internet dispatcher**. Native CLI hiện là login-only. Chưa có terminal/PTY, Files, Git, Patch/Collection qua phiên được kiểm soát, kiểm tra permission từng hành động trên WSS thật, binary provenance ký và xác minh trên máy vật lý, pentest độc lập, thử shared-host PHP/Relay bị tấn công ở hạ tầng triển khai và full RC-01..RC-13. PHP bị hacker sửa vẫn có thể DoS/phishing/trả trang tải client giả; chỉ việc **không cho nó tự tạo principal/action khi thiếu client private key và mật khẩu TaskDeck** mới được kiểm chứng trong phạm vi tests đã chạy.

Giữ `remote_control_available=false`, không tạo HTTP/WS remote application routes, không mở inbound port của TaskDeck, không merge vào main khi chưa hoàn thiện release gates.

## Kiểm chứng cuối cùng ngày 2026-10-30 — PHP/Relay giả mạo và silent ACK

**Source SHA:** `fc4011e970597918c30135bb8f9246d32fec87bb`.
Cả hai workflow trên **cùng source SHA** đều **completed / success**:

- [Go Agent/race/red-team #40994280931](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40994280931)
- [PHP Portal security #40994280960](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40994280960)

Thay đổi `remoterelay/app_mailbox.go` bổ sung `time.AfterFunc`
để **đóng chủ động cả mailbox và kênh reply đang chờ** khi
(1) Relay im lặng quá hạn ACK 20 giây hoặc (2) mailbox hết TTL 5 phút,
không phụ thuộc thao tác tiếp theo. Hai lần hủy được tuần tự hóa dưới
`Registry.mu`, kiểm tra đúng device/Agent nonce, `sessionSeq`, pointer
mailbox và pointer request; một timer cũ không thể đóng session mới.
ACK hợp lệ, close, disconnect và quota exhaustion đều dừng timer cũ.
Các test `TestCompromisedRelaySilentAppACKCannotHoldPendingForever`,
`TestCompromisedRelayAppACKExpiryRaceIsSingleTerminalOutcome` kiểm tra
timeout và ACK race dưới `-race`. Cơ chế này còn **UNMOUNTED**; nó
không cấp quyền application/TaskDeck qua PHP.

**Kết luận:** với giả định host và native private keys an toàn,
PHP dù mất quyền kiểm soát hoàn toàn, mất signing seed và có khả năng
forge Portal admin tickets/roles/grants cũng **không thể tự mình**
tạo một local TaskDeck principal hoặc action được phép. Đây là kết luận
trong phạm vi mã nguồn, kiểm thử fake-malicious PHP/real TLS-WSS handler
và Go race suite đã chạy, KHÔNG phải chứng nhận an toàn tuyệt đối.
Nếu hacker chiếm máy người dùng/phishing native password và private
key, hoặc lấy được Agent seed trên host, giả định không còn đúng.
Host phải bảo đảm public-key approval thực hiện **chỉ qua CLI của
TaskDeck trên host**; client binary và Device ID pin lấy ngoài PHP.

**Remote Control vẫn NO-GO:** client native hiện chỉ có login-only;
application remote network route chưa kết nối và chưa có khả năng
terminal/files/Git/Patch/Collection với kiểm thử end-to-end. Không
mở thêm cổng, không đổi `remote_control_available=false`.


## Bổ sung — host-approved client private-key possession (offline Firefox)

Đã bổ sung bước thứ hai của native client E2EE, **không cần browser extension
hoặc portal thứ hai**:

1. Sau khi giải mã vault tại Firefox local file, private Ed25519 key được
   import thành WebCrypto `CryptoKey` non-extractable để **chỉ ký trong bộ
   nhớ**, không xuất private PKCS8 qua PHP/Relay. Vault JSON trên đĩa vẫn
   được bảo vệ bởi PBKDF2-SHA256(600k)+AES-256-GCM.
2. Dùng đúng handshake Agent Ed25519 và ephemeral P-256, rồi thực hiện
   `taskdeck-e2ee-client-key-check-v1` bên trong AES-256-GCM. Client ký
   **chính xác** `taskdeck-remote-client-login-v1 || 0x00 || ASCII-lower(username) ||
   0x00 || SHA256(E2EE transcript)`.
3. Agent kiểm chữ ký client Ed25519, host-owned registry approved key
   gắn với TaskDeck SQLite `User.ID`; chỉ Agent biết kết quả.
4. Firefox nhận một gói `e2ee_client_key_check_result` với ciphertext AES-GCM
   authenticated, kiểm `taskdeck-e2ee-client-key-status-v1` và
   `approved: true|false`. Bất kỳ status thuần từ PHP/Relay, ciphertext
   bị sửa, replay hoặc payload sai schema đều không được chấp nhận.
5. Trạng thái E2EE **one-shot** sau khi trả về kết quả. Client không
   tự động gửi mật khẩu hoặc biến kết quả approved thành quyền TaskDeck.
   Mỗi lần thử dùng khóa ECDH mới/nonce mới/ticket mới; PHP không thể
   copy public key hay phát lại chữ ký của lượt trước để mạo danh.
6. Trên Go Relay, `/v1/offline` **tuyệt đối không nhận e2ee_login**
   dù đã thực hiện key-check. Đã tách chính sách
   `allowedBrowserProtectedTransitionForRoute` và bổ sung unit test.
   Native `/v1/browser` login-only vẫn giữ nguyên.

**Chỉ dẫn UI:** Sau khi mở vault, nhập Relay HTTPS origin, bấm chuẩn bị
nonce/ephemeral public, copy hai thông tin công khai vào PHP xin ticket,
paste ticket, rồi bấm "Kiểm tra khóa client được host duyệt".
Kết quả thành công chứng minh khóa private thuộc registry host tại thời
điểm Agent kiểm — **không** đăng nhập TaskDeck, không cấp capability/token
hay quyền Terminal/Files/Git/Patch. Không bật `remote_control_available`.

Kiểm thử:
- Node WebCrypto `remote_portal/tests/offline_e2ee_crypto.test.cjs`
  kiểm chữ ký E2EE-transcript bằng client Ed25519, xác minh signature
  bằng public key thật, phản hồi approved/denied dưới AEAD và
  poison-on-forgery; kiểm cả PHP admin giả mạo status.
- Go `TestOfflineFileRouteNeverAcceptsTaskDeckPasswordOrApplicationFrame`
  xác nhận offline route deny login/terminal/file operations, bất kể
  preflight thành công.

**Chưa hoàn thành để lên live:** chưa có trình duyệt Firefox chạy test
trực tiếp trên file:// và mạng WSS thật, chưa có E2EE password authentication
flow cho Firefox offline (đang CỐ Ý khóa), chưa có ứng dụng remote
Terminal/Files/Git/Patch sau per-action RBAC; chưa có NAT/CGNAT Linux/macOS
field test và red-team độc lập. Client và Agent local-compromise nằm ngoài
phạm vi giả định PHP-only. `remote_control_available=false` vẫn bắt buộc.

## Kết quả CI cuối của đợt E2EE offline + client-key preflight

**Verified source:** `8577f5cc298b1dca970f99795275398c67fa2b07`.
Hai workflow trên **cùng source SHA** đã completed/success:
- [Go Agent / Relay CI #41005763889](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41005763889): PASS, gồm red-team PHP/Relay, race tests, kiểm thử /v1/offline no-password policy, cross-build macOS/Linux ARM.
- [PHP Portal / Node WebCrypto CI #41005763878](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41005763878): PASS, gồm mật mã E2EE của HTML offline thực, host-approved client key Ed25519 signature binding, forged/denied/replayed encrypted verdict, HTTP MFA-gated download.

Lưu ý: bộ CI trên không mô phỏng được toàn bộ Firefox file://
và mạng/host NAT thật; **đó là release gate còn thiếu**. 
`remote_control_available=false`, và đường offline từ chối password/login
frames dù key-preflight thành công. Không quảng bá GO Remote Control.
