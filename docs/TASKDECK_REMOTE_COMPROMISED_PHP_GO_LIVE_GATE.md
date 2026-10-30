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

## Xác nhận CI sau login-only và ticket UX (2026-10-30)

**CI PASS đã xác minh (tránh trộn SHA):**

- Go Agent/Relay, source `1352258b7200dbd196a536d67cbbca5593fd24ca`:
  [workflow #41006883306 — completed SUCCESS](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41006883306).
  Cùng SHA đó, [PHP #41006883310 — SUCCESS](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41006883310).
- Firefox offline HTML WebCrypto login-v2 regression, source
  `a8326a8c31986718a7327e4b7afffa9a732875a9`:
  [PHP/Node #41007290941 — SUCCESS](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41007290941).
  Chống gọi mật khẩu trước Agent/client key approval, phủ nhận host key,
  đáp ứng login-result AEAD/sequence, sửa lỗi test lấy transcript của
  phiên E2EE khác.
- UI nhập ticket JSON thuần (hai trường `ticket`,
  `expires_in_seconds:30`), chống field lạ, và xử lý Relay đóng
  WSS sau phản hồi đã chuyển cho WebCrypto kiểm tra:
  source `1e2821d82372d22090f6464f6372a7fc36537492`,
  [PHP/Node #41007686079 — completed SUCCESS](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41007686079).

**Cảnh báo quan trọng:** các lần CI phía PHP chỉ kiểm tra Node WebCrypto
và PHP HTTP tests, **không thay thế** việc chạy file `file://` thật
bằng Firefox và tác vụ NAT/CGNAT. Go CI được xác minh ở source code
có thay đổi Go gần nhất; sau đó chỉ chỉnh HTML/tests/docs.
Nguồn PHP bị hack về sau vẫn có thể DoS/phishing, nhưng không được
chạy JS trên local offline client.

Cờ mới `-allow-offline-login-probe` phải do operator bật trên Go
Relay và chỉ hợp lệ khi đồng thời `-allow-offline-file-client`.
Mặc định cả hai false. Mở đường này chỉ phục vụ **boolean login-only**;
server Agent vẫn xác thực `User.ID`, approved Ed25519 client key,
TaskDeck password và local project membership/RBAC. Không mount remote
terminal, files, Git/Patch hoặc ứng dụng bất kỳ.

**Còn thiếu bắt buộc trước LIVE:** Firefox thật qua TLS WSS với PHP
bị sửa sau pairing, test host Linux/macOS sau NAT/CGNAT, full
application dispatcher với per-action authorization, revoke, stream,
trusted client distribution/update và independent red team.

`remote_control_available=false` không được đổi. **NO-GO Remote
Control**. `approved:true` hoặc `authenticated:true` trong
kết quả đã mã hóa vẫn **không phải** capability hoặc session token.

## Fifth security audit — stale vault identity / asynchronous unlock / active WSS cancellation (2026-10-30)

**Audit baseline:** `46968e4aaaa3a83b8714ed746528dd290a7d56d5`.
**Latest source under test:** `30ea6dd8f8558405606c97f91360414bacff19e7`.
No application-remote route or TaskDeck action dispatcher is enabled.

### Findings and fixes

1. **P1 — stale authenticated client vault after selecting another file**.
   `vault-file` selection previously left `trustedVault` and the
   existing ECDH `offlineProbe` intact until the user clicked Unlock
   and finished AES-GCM/PBKDF2. Clicking Prepare/Connect in between
   could use a *different previously unlocked client's* Agent pin and
   signing key rather than the newly selected vault. **Fixed**:
   changing the selected file immediately invalidates cached trust,
   clears nonce and public ECDH display, and requires a fresh
   successful unlock before any further outbound attempt.

2. **P1 — asynchronous file read/decrypt can authorize an obsolete file**.
   The old Unlock handler allowed multiple overlapping clicks and
   committed whichever `openVault()` resolved last. **Fixed**:
   `unlockBusy` serializes unlock; the previous trusted key is
   revoked synchronously before reading the file or deriving the
   PBKDF2 key. A monotonically increasing file-selection generation
   plus exact file-object check is verified both after `file.text()`
   and after the actual crypto verification. A changed file's
   in-flight result is discarded; the password field is cleared
   and the button restored in `finally`.

3. **P2 — concurrent ECDH Prepare could overwrite copied nonce/key**.
   Previously duplicate clicks on `makeE2eeProbe()` could race
   over `offlineProbe` and the displayed values, causing a Portal
   ticket to be issued for a different key from the one used during
   WSS handshake. **Fixed**: serializing with `prepareBusy`,
   blocking Prepare/Connect while unlocking or active WSS, and
   validating that the trusted vault and Relay origin are unchanged
   before publishing a newly generated ECDH key. Changing Relay
   origin clears prepared nonce and key.

4. **P1 — changing vault mid-WSS could finish a login with old identity**.
   The E2EE socket captured `activeVault` correctly but a subsequent
   user-selected vault change left that socket running, possibly
   displaying a success against the old host after the user already
   switched identities. **Fixed** by revoking the active WSS on
   vault-selection change. The one-shot `activeOfflineAbort`
   invokes the same fail-closed `finish` path, closes WSS,
   aborts any active local TaskDeck password dialog and rejects
   the pending operation. A late ciphertext cannot turn the
   aborted state into success.

**Code commits:**
- `c76fb955e970ca5fcd5ae1cc3e68bebdc69c216c` —
  invalidate old vault, serialize unlock and ECDH.
- `94bcf998722e70e6b40ea8722bfd2ddbdb535998` —
  revoke live WSS on vault switch.
- `bd0c6ee1396476067b8841a6188b7a76f3a56d5f` —
  introduce true HTML listener regression and hook into CI.
- `30ea6dd8f8558405606c97f91360414bacff19e7` —
  test live WSS cancellation on vault change.

### Verified CI

- [PHP Portal / Node WebCrypto workflow #41020570369](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41020570369)
  **completed SUCCESS** at source `30ea6dd8f8558405606c97f91360414bacff19e7`.
  Includes actual HTML listener tests with a pending file-read,
  changing selected vault during unlock, stale-session prevention,
  concurrent ECDH preparation and a fake WebSocket that must
  close immediately when the user switches vault.
- [Go Agent / Relay workflow #41020233978](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41020233978)
  **completed SUCCESS** at source `bd0c6ee1396476067b8841a6188b7a76f3a56d5f`.
  The two subsequent commits only changed the offline HTML and
  Node test. Do not claim Go CI was run on the final HTML source.

### Remaining limits and NO-GO

These are source-level and Node WebCrypto tests, not Firefox's native
`file://` browser UX, hardware download persistence, live WSS
TLS/certificate checks, real Linux/macOS NAT/CGNAT or compromised
PHP red-team field tests. Offline HTML updates are not securely
delivered to existing downloaded local copies; upgrading must not
run untrusted JS from a newly compromised PHP Portal.

`remote_control_available=false` remains mandatory.
An approved Ed25519 client key, successful E2EE handshake or valid
TaskDeck password probe yields only a one-shot boolean: **no**
remote session token, Terminal/Files/Git/Patch, arbitrary command
execution or app dispatcher. **Remote Control remains NO-GO.**

## Sixth security re-audit — Relay replay-ledger OS owner enforcement (2026-10-30)

**Baseline:** `3d094624857e71eea4f03b3944cebe31a9de490b`.
**Current audited source:** `f6f713abdabbdb53ff0aea6070cdd44a284e3d97`.

### Confirmed finding

**P1, host security boundary** — `internal/remoterelay/replay_ledger.go`
previously checked the replay directory for `0700`-equivalent permissions
and regular JTI marker files for `0600`-equivalent permissions.
It DID NOT verify that either filesystem inode was owned by the
Relay's **effective OS UID**. A privileged Relay misconfigured to use
a private but *foreign-owned* directory could trust JTI replay markers
under another account's control. The other owner might replace/remove
consumed JTI records to force unwanted replay attempts after restart.
This is a deployment/trust-path risk requiring control of the chosen
local replay path; it is not a claim that PHP alone remotely owns Relay's
local files.

### Code and tests

- `a3d65ab79d0b5ed88c5c5d87477f182f3accf2d9`:
  introduce `ownedReplayEntry`, requiring effective UID owner match
  in addition to private POSIX modes, directory/regular-file type and
  non-symlink constraints. Applied on first replay directory open
  and on **every JTI record during startup/prune**. Invalid metadata
  is rejected, not deleted: this remains fail-closed.
- `bad74302ea0a06406277f447f00dd0cbc49e6316`:
  add regression tests using a file-info wrapper with a foreign
  `syscall.Stat_t.Uid` (no root privileges needed). Tests also
  reject treating directory as JTI file or vice versa.
- Initial Go CI on `bad74302` **FAILED** because the test fixture's
  `t.TempDir()` mode was not always `0700`; it failed the
  *positive, owner-matched* prerequisite before checking the
  mocked foreign UID. There is no evidence that production
  `ownedReplayEntry` accepted an untrusted inode.
- `f6f713abdabbdb53ff0aea6070cdd44a284e3d97`:
  normalize fixture directory to mode `0700` before testing
  ownership. Production owner check and timeouts remain unchanged.

### Test evidence

- [PHP/Node CI #41024303056](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41024303056)
  **SUCCESS** at exact source `f6f713ab`.
- [Go Agent/Relay CI #41024302778](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41024302778)
  **RUNNING at the point this note was written**; final
  PASS/FAIL must be checked and appended before claiming full
  Go verification.

**Remaining deployment constraints:** even with UID checks, run
Relay as a non-root dedicated service user when possible, store
the replay ledger in a controlled private directory, and ensure
parent paths cannot be replaced by another account. This round
did not demonstrate real Firefox `file://`, physical NAT/CGNAT,
shared-hosting compromise, multi-replica replay storage or
independent penetration testing. Multiple Relay replicas require a
SHARED transactional replay store, not the local directory ledger.

**Remote Control remains NO-GO**:
`remote_control_available=false`, the authenticated browser
flow only reports a one-shot login boolean and opens no TaskDeck
Terminal/File/Git/Patch execution route.

## Sixth audit — final CI result (2026-10-30)

The corrected replay-ledger ownership source and regression fixture at
`f6f713abdabbdb53ff0aea6070cdd44a284e3d97` have now passed
BOTH corresponding GitHub Actions workflows on the **exact same SHA**:

- [Go Agent/Relay #41024302778](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41024302778):
  **completed SUCCESS**. Includes `go test -race` for
  `internal/remoterelay` with the new UID-negative tests,
  compromised-PHP/Relay authorization tests, TaskDeck login/RBAC
  tests and macOS/Linux ARM cross-compilation.
- [PHP Portal/Node #41024303056](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41024303056):
  **completed SUCCESS**. PHP/Node checks unchanged by the Go-only
  ownership guard.

This supersedes the earlier 'RUNNING' status in this audit's first
progress note. The previous failed test fixture at `bad74302`
was corrected without relaxing the production owner check.

**Security conclusion:** Relay now fails closed on foreign-owner replay
directories and JTI markers, including cases where they have private
permission bits. This does NOT prove safe mounting of arbitrary
ancestor directories, multi-replica replay guarantees or resistance
to a fully compromised Relay OS account.

**Remote Control still NO-GO.** No remote execution capability has
been enabled, `remote_control_available=false`.

## Seventh relay security review — Agent-to-Relay JSON ambiguity (2026-10-30)

**Source SHA:** `e14622bb28553c674e63ee59f3d4aa2792df4453`. Branch: `feature/taskdeck-remote-access-architecture`.
**Scope:** exact JSON framing for the existing authenticated device WebSocket;
no new Relay routes, PHP endpoints, passwords, TaskDeck principal, app
dispatcher, file or terminal traffic were enabled.

### Finding and correction

The Agent had a strict Relay-command decoder, but the Relay still used
`json.Unmarshal` directly for Agent registration and subsequent device
frames. Go accepts duplicated object keys, case-insensitive struct-field
aliases, and unknown keys in that mode. This could cause protocol-confusion
regressions as handlers evolve. No local TaskDeck login/RBAC bypass was
demonstrated: successful registration still required the Agent Ed25519
private key and the signed Portal presence lease.

Commit [`e14622bb`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/e14622bb28553c674e63ee59f3d4aa2792df4453)
replaces the two lenient ingress decoders with bounded, strict,
case-sensitive, duplicate-rejecting parsers in
`internal/remoterelay/device_frames.go`. The registration envelope
requires exactly the established identity/challenge/lease/signature fields
and a JSON integer timestamp; the authenticated socket accepts only
`ping`, `probe_ack`, `probe_rejected`, `login_ack`, `login_rejected`,
`echo_ack`, `echo_rejected`, each with its exact fields.
The nested `probe_ack.crypto` object is checked recursively. Escaped
aliases, unknown fields, missing fields, invalid data types, trailing JSON,
and excess encoded lengths are rejected before downstream lease/ACK checks.
No new action type is added.

Regression cases in `device_frames_test.go` cover all legitimate
frame types, signed-proof decode, duplicate/case/escaped key aliases,
unknown authority fields, nested crypto duplicates, ciphertext omission,
wrong JSON types and trailing objects.

### CI for this exact source

- [Go Agent/Relay workflow #41028995651](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41028995651) — **SUCCESS** when checked for this handoff.
- [PHP Portal/Node workflow #41028995612](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41028995612) — **SUCCESS** when checked for this handoff.

Only call this source *fully verified* after Go and PHP both report
COMPLETED/SUCCESS on the same source SHA. This code review and automated
regression do **not** substitute for an independent penetration test.

### Open functionality and safety gates

The PHP shared-hosting Portal is an **authentication/grant/control-plane
component**, not the actual long-lived WebSocket relay. The Go Relay must
be deployed separately (or the transport must be intentionally redesigned
for bounded HTTPS polling); do not advertise PHP-only hosting as supporting
Internet remote control. Current Firefox client uses a first-pair trusted
downloaded `file://` HTML + encrypted local vault; it must never load
replacement JS from PHP after pairing.

**NO-GO for remote application control.** The production Relay remains
limited to reachability/E2EE diagnostic and login-only probes.
`remote_control_available=false` remains mandatory. The internal
application mailbox is unmounted. No Terminal/Files/Git/Patch/Collection
transport is exposed; no authenticated application session or per-action
RBAC gateway exists on the public WSS route. Next implementation must
start with a fully bounded, verified application `session.ping` from the
trusted client through real WSS to host-local TaskDeck authorization,
then prove replay/revoke/timeout semantics and only then add typed
work operations. Real Firefox `file://`, NAT/CGNAT, shared-host deployment,
trusted file provenance and independent security testing remain required.

## Scope lock — three security invariants (2026-10-30)

**Source commit:** [`0404e79adbce59b82dcb6e12ca146a4815440351`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/0404e79adbce59b82dcb6e12ca146a4815440351).
**Security objective:** protect TaskDeck if PHP shared hosting/Portal is fully compromised AFTER the initial owner-verified pairing; do not accidentally enable remote execution during hardening.

1. **PHP control-plane compromise must not authenticate TaskDeck.** An attacker may read/write PHP, SQLite, MFA/session data, Portal ticket/lease signing seed, and falsely report admin/allowed=true. None is a TaskDeck login. The host validates Agent Ed25519 identity bound to its workspace, owner-approved client public key and proof of matching client **private** Ed25519 key on the exact E2EE transcript, the real TaskDeck-local password hash, account state, immutable user ID, project membership and per-action permission. The PHP grant is an additional deny/ceiling, not a replacement for these requirements. Compromised PHP may still disrupt availability, burn tickets, or phish if the person trusts replacement PHP-served code; preventing the latter requires the offline HTML file verified at first pairing and no automatic code refresh from the compromised PHP origin.

2. **PHP/Relay never receive plaintext TaskDeck application data.** Browser offline client and authenticated host Agent derive ephemeral ECDH P-256/AES-256-GCM keys with signed transcript, directional sequence counters and replay-fail-close. WSS transports a **base64 representation of ciphertext**. The Go Relay currently handles only signed presence, one-shot client-key check, diagnostic echo and encrypted login boolean. The PHP Portal does not currently carry TaskDeck terminal/file/command bytes at all; it is NOT the WebSocket data relay. A compromised Portal/Relay CAN see Portal account data, Device ID, key fingerprint/public keys, traffic timing, sizes, routing IP, nonce/ticket, and can drop/delay/corrupt traffic. These are NOT encrypted by the application envelope and therefore cannot be promised secret. Future file/terminal/output streams may only be mounted as typed, AEAD-encrypted frames AFTER TaskDeck authorization. The CLI/Firefox password prompt must remain in a locally retained trusted client, NEVER in PHP-served post-pair JS.

3. **No remote login bypass / command execution.** Signed Portal admin claims, a successful WSS handshake, Agent proof, key preflight, or a password-only result never creates a TaskDeck principal or remote command. An application action must use the same in-process local session created by approved client key + password + local project RBAC and still live E2EE channel; authorization must be re-evaluated for each operation, with revocation, timeout, audit and bounded resource controls. At this checkpoint the real Internet application dispatcher and Terminal/Files/Git/Patch/Collection routes are UNMOUNTED and `remote_control_available=false`. **No remote command can be claimed operational/securely authorized end-to-end until such a route is created and tested.**

### This audit's concrete additions

- `internal/remoterelay/signed_claims.go` + `tickets.go` + `lease.go`: parse signed ticket/lease JSON as exact-key, exact-type, bounded, one-meaning objects. Invalid/duplicate/escaped-alias/case-aliased/extra claims are refused even when an attacker has the Portal signing seed. A refused token never consumes a JTI replay record.
- `tickets_test.go`, `lease_test.go`: negative forged-signer cases and positive canonical PHP-format claims.
- `remote_portal/tests/offline_e2ee_crypto.test.cjs`: captures actual encrypted login WSS wrapper from offline Firefox crypto, confirms no plaintext TaskDeck password/username is included and that the sequence-numbered ciphertext opens only at the authorized Agent.

CI checks for EXACT code source `0404e79adbce59b82dcb6e12ca146a4815440351`:
- [Go Agent + Relay #41030706501](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41030706501) — **PENDING (not yet a PASS)**.
- [PHP Portal + offline WebCrypto #41030706506](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41030706506) — **SUCCESS**.
These tests validate covered invariants only. If any is pending, do not mark full validation PASS.

**Release decision: NO-GO for remote control.** The three invariants are conditions that every future real application frame/operation must satisfy, not a declaration that the presently unmounted dispatcher has passed. Finish an authenticated remote `session.ping` across actual WSS → host-local TaskDeck permission gate, then negative tests for wrong password, unapproved/forged key, stale grants, replay, PHP authority spoof, local revoke, lost channel, ciphertext corruption; only then consider typed read/write operations. Maintain hardware Firefox `file://`, NAT/CGNAT, shared-host tests, authentic binary/HTML provenance and independent pentest as release gates.

### Confirmed same-source CI result — 2026-10-30

Source SHA [`0404e79adbce59b82dcb6e12ca146a4815440351`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/0404e79adbce59b82dcb6e12ca146a4815440351):
- [Go Agent + Relay #41030706501](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41030706501) — **COMPLETED / SUCCESS**. Includes Agent/Relay cryptographic verification, signed-grant ambiguity regression, host-local login/security integration and cross-platform compile gates.
- [PHP Portal + offline WebCrypto #41030706506](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41030706506) — **COMPLETED / SUCCESS**. Includes actual offline-client captured-wire assertions for encrypted TaskDeck login, plus PHP Portal regressions.

This supersedes the PENDING result captured earlier in the same-source handoff.
Only the **implemented login-only and diagnostic boundaries** are verified:
a valid Portal admin ticket cannot itself authorize TaskDeck, the existing
login request is opaque AES-GCM ciphertext to PHP/Relay, and the current
remote application command dispatcher remains **UNMOUNTED**. Do not infer
that future Terminal/Files/Git/Patch transfer or browser-level phishing
resistance is tested or released. `remote_control_available=false`,
Remote Control **NO-GO**.

### Security audit — final TaskDeck authorization boundary (2026-10-30)

**Audited code HEAD:** [`a3531ff3a6837a74155bf52b8cca49b97c69c769`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/a3531ff3a6837a74155bf52b8cca49b97c69c769).

Scope remains the THREE stated conditions; no generic UI redesign or unrelated
app features were changed.

| Required condition | Evidence at this HEAD | Release assessment |
| --- | --- | --- |
| PHP fully compromised AFTER initial trusted pairing cannot authorize TaskDeck | Stolen Portal signer, forged `admin`, positive live grant and even a known local password must not skip owner-approved client Ed25519 possession, immutable host User.ID and local RBAC. Added mandatory `clientProofVerified`/host key identity precondition INSIDE `authorizeRemoteAuthenticatedSession`, not just in `authorizeRemoteTransportAction`. Negative test calls lower-level helper directly and demands session teardown. | PASS for current staged/in-process and login-only test routes, NOT verified end-to-end for a production application command transport |
| TaskDeck data crossing PHP remains secret | Current TaskDeck login payload is AES-256-GCM E2EE with signed/pinned Agent ECDH transcript. PHP is the Portal control plane; actual E2EE frames presently traverse independent Go Relay, not PHP. Password and diagnostic ciphertext have verified wire-only handling. | PASS for implemented login-only frames. Future commands, files, terminal output not yet transported, therefore NOT TESTED. PHP Portal credentials, metadata and traffic timing are not E2EE and cannot be claimed hidden from compromised PHP |
| Cannot bypass TaskDeck login and run commands | Per-action TaskDeck local permission gate now also checks approved-key proof at FINAL authority boundary. A local principal/key/password revocation shuts down the entire E2EE session, not merely the current action. No production generic command runner has been mounted. | PASS for bounded internal session.ping mock and selected real-HTTP denial tests; NO evidence of fully authenticated remote Terminal/Files/Git/Patch operations over a real WSS session |

**Code changes:** [`ffbac25d`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/ffbac25d80e9c4e34e6b51298ec36bc383aa5e78)
(final local authorization key proof), [`fb9e7cc`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/fb9e7cc58ef8b4f1d14c84b0e7c781da7e33660d)
(closes E2EE after failed local identity/key recheck),
[`a3531ff`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/a3531ff3a6837a74155bf52b8cca49b97c69c769)
(test fixture compile fix). Prior `ffbac25d` workflow failed due to one
incorrect test-fixture variable, not due to an assertion bypass. Fixed and
verified at exact code SHA above.

**CI on exact source SHA:**
- [Go Agent / Relay #41032333032](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41032333032): **COMPLETED / SUCCESS**, including the new direct low-level authorization attack regression, the existing PHP-compromise red-team and cross-platform builds.
- [PHP Portal / offline E2EE #41032332923](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41032332923): **COMPLETED / SUCCESS**.

**RELEASE DECISION: REMOTE CONTROL NO-GO.** Three full production
conditions CANNOT all be marked PASS while application command transport
is unmounted. No browser→Relay→Agent→TaskDeck command authorization,
decryption and output encryption live path exists; there is nothing safe
and functional to release as remote Terminal/File/Git/Patch. Do not
enable `remote_control_available`, merge into main, tag release or
deploy Remote Control on the basis of existing login-only and internal
mock PASS. Next step MUST implement the real E2EE application session
via production WSS (and PHP opaque forwarding only if the architecture
actually sends application frames through PHP), then use a controlled
post-pair PHP+Relay compromise simulation to test each permitted command
and file/output frame under incorrect password, unknown/revoked client
private key, replay, altered ciphertext, stale grant and lost network.
After security CI on EXACT release SHA, verify on real Linux/macOS and
public Relay/PHP hosting before a live enablement.

## Three-condition release gate — Agent-side real TLS/WSS app mailbox (2026-10-30)

**Production source under test:** [`1c342061460eeb27dc147256af6d2b6e9684e2bb`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/1c342061460eeb27dc147256af6d2b6e9684e2bb).

**Changes:** [`ad785811`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/ad785811182d67bac26418739d1f8daee3fb60b5) connects the existing opaque `Registry.AppPing` mailbox to the *real Agent-side* `/v1/device` WSS handler. Strict JSON allowlists add only `app_ready`, `app_ping_ack`, `app_ping_close` and the server's outgoing `app_ping` ciphertext envelope; PHP/Relay cannot supply arbitrary command, HTTP path, TaskDeck principal, cookie or plaintext. `Registry.ActivateAppPing` validates the authentic Agent's Ed25519 signature, actual registered connection nonce and last login probe; one pending action, deadline and quota are retained. [`1c342061`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/1c342061460eeb27dc147256af6d2b6e9684e2bb) tests actual TLS/WSS Agent registration and app ciphertext delivery and shows a hacked PHP with its own signing seed cannot forge the separate Agent-ready signature. The test **injects a staged last-login marker internally**, so it is NOT proof of real TaskDeck password login over app WSS.

Three release questions:
1. **Hacked PHP access to TaskDeck:** independently authenticated Agent private key is required for application mailbox readiness. PHP's signer cannot create it. **Partial PASS** (network Agent-side, not yet full app login/session).
2. **Confidentiality:** real Agent↔Relay WSS carries only bounded opaque base64 E2EE-shaped frames; the Relay neither decrypts nor embeds TaskDeck plaintext. Existing Go/WebCrypto tests prove AES-GCM of login. **Partial PASS** (new app WSS test uses an opaque ciphertext-shaped fixture; browser↔Agent real application frame/end-to-end data still unimplemented). Portal account and routing metadata remain visible to hacked PHP.
3. **TaskDeck login bypass:** the per-action TaskDeck local verifier and host-only client-key checks stay mandatory; the new Relay route does not expose a TaskDeck command dispatcher. **Pending full real-session end-to-end test** — the in-memory test's login slot is not a production login.

**Deliberately NOT exposed:** The Browser side has NO public application endpoint or `session.ping` producer for this flow, Agent does not send `app_ready` from a real `StartEncryptedRemoteSession` yet, no response is decrypted by any browser application route and no Terminal/Files/Git/Patch command is enabled. Keep `remote_control_available=false`; do not release as remote control, change flags, merge to main or claim three full PASS results based solely on these tests.

**Same-source CI:** [Go Agent/Relay #41033901092](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41033901092) = **PENDING**; [PHP Portal/Node #41033901025](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41033901025) = **SUCCESS**. This is the audit state at documentation commit time; update separately if pending.

**Next mandatory implementation:** build an opt-in trusted-client application-login WSS path on the *existing pinned Agent E2EE channel*, keep the non-serializable TaskDeck-authenticated principal in the host process, deliver only typed `session.ping` ciphertext to `executeRemoteE2EEPing`, recheck host-approved private-key proof, TaskDeck password/RBAC and revocation on EVERY operation, return only AEAD ciphertext. Test live Browser WSS→Relay WSS→Agent→local TaskDeck→encrypted response with a compromised PHP signer plus real password but absent client private key; add negative tests for unknown/revoked key, replay/tampered ciphertext, stale Portal roles and cancellation, then safely extend to actual feature operation(s). A working remote function must exist to warrant a LIVE release.

### Same-source CI verified for Agent-side application WSS — 2026-10-30

**Audited source:** [`1c342061460eeb27dc147256af6d2b6e9684e2bb`](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/1c342061460eeb27dc147256af6d2b6e9684e2bb).
- [Go Agent/Relay #41033901092](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41033901092): **COMPLETED / SUCCESS**. Includes the newly added actual TLS/WSS encrypted application mailbox transfer and malicious PHP private-key impersonation denial, Go race/security tests and cross-build checks.
- [PHP Portal/Node #41033901025](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41033901025): **COMPLETED / SUCCESS**. No PHP application data-processing or TaskDeck authorization added.

This supersedes the PENDING CI notation in the Agent WSS section above.
This proves ONLY the secure opaque **Agent-side** transport stage; it does
not complete trusted Browser-side app login/command exchange or bind
the new application-ready message to a live TaskDeck password principal
at the host. Do not claim a fully functional or fully audited Remote
Control connection. `remote_control_available=false`; **LIVE NO-GO**.
