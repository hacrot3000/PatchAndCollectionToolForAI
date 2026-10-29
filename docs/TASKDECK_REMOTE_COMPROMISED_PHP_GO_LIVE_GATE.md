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
