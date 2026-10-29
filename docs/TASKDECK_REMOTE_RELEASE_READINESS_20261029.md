# Kiểm định mở tính năng Remote Access — 29/10/2026

## Quyết định tách biệt hai phạm vi

- **Remote LOGIN-ONLY (đọc kết quả xác thực E2EE một lần):** Có thể kiểm thử có kiểm soát sau khi xác nhận Go + PHP CI cùng xanh trên commit nguồn gần nhất. Không cấp cookie, access token, TaskDeck principal cho browser, HTTP/PTY/File/Git/Patch hoặc Remote Control.
- **Remote CONTROL / truy cập TaskDeck ứng dụng qua Internet:** **NO-GO.** Chưa triển khai/lắp tuyến E2EE session + phiên lệnh và dispatcher trên Relay/Agent/Client. Không bật bằng cách chuyển môi trường `TASKDECK_REMOTE_LOGIN_PROBE=1`. Đường in-process `StartEncryptedRemoteSession` và ping đọc-only trong server vẫn **unmounted**.

## Threat model chủ đạo

Kẻ tấn công chiếm toàn quyền PHP Portal, SQLite, user Portal admin, signer, ticket và có thể cung cấp nội dung HTML/JavaScript độc hại. Hacker **không** có password TaskDeck cục bộ, seed Agent, private key client đã duyệt hoặc quyền OS host. Portal/Relay được phép gây DoS/nhận biết metadata kết nối nhưng không được đọc mật khẩu, cấp principal, phê duyệt key hoặc điều khiển TaskDeck.

- TaskDeck native client độc lập, có Device ID được xác minh và ghim qua kênh **ngoài Portal**.
- Client tự tạo Ed25519 key 0600; hiển thị public key copyable lần đầu, không hỏi mật khẩu trước khi host duyệt.
- Host CLI `taskdeck remote-key --workspace /absolute/project list|add|revoke` kiểm tra tài khoản SQLite thật, immutable User.ID và membership.
- Agent E2EE preflight phải trả **approved=true mã hóa và xác thực** từ khóa đã được host duyệt. Không có preflight hoặc ký sai chỉ hủy browser; không được làm Agent offline.
- Sau preflight, TaskDeck xác thực lại key, chữ ký ràng buộc phiên, password scrypt, user ID và RBAC; password-only v1 từ chối; grant Portal không thay thế yếu tố nào.
- Session staged kiểm tra lại key và quyền khi action; thu hồi khóa / reset password / hủy grant phải chặn.

## CI và kiểm thử bắt buộc cho LOGIN-ONLY

1. **PHP Portal** — PHP lint, SQLite grants, brute-force limits, MFA, browser ingress, ticket expiry/replay, signed Agent proofs, credential boundary, JS/WebCrypto interop. CI chạy lại khi mã Agent/Relay/E2EE đổi, không chỉ khi PHP đổi.
2. **Go -race** — remoteclientkey/remoteportal/remoterelay/remotetransport và native client; kiểm thử bền vững quota qua restart, race, file owner/symlink/JSON malformed.
3. **TaskDeck server -race** — các test `TestRemote*`, `TestEncryptedTaskDeckLogin*`, **`TestCompromised*`**, `TestTaskDeckSessionBindsExactPinnedAgent` và `TestDirectSharedRemoteHandler*`. Trước kiểm định này CI bỏ sót nhóm `TestCompromised*`; đã khắc phục.
4. **Real TLS/WSS integration** — đúng luồng Agent đã ghim → E2EE preflight → password → trả boolean; thiếu preflight, lệnh ciphertext sai kiểu hoặc key chưa duyệt không đăng nhập; Agent vẫn hoạt động cho người dùng hợp lệ. Giữ nguyên frame rate limiter (12 burst, 1/2s) và kiểm thử quota durable riêng; không nới giới hạn để hợp thức hóa test.
5. **Build** — native host CLI/client, Relay; cross-compile Linux ARM64, macOS ARM64/AMD64. Build thành công không thay thế chạy thử GUI thực tế trên máy người dùng.

## Những thử nghiệm bắt buộc trước khi công bố Remote CONTROL

- Thiết kế và gắn đường session ứng dụng end-to-end. Không tái sử dụng boolean của login-only như bearer/cookie, không đưa credential vào PHP.
- Kiểm định timeout/close/cancel của Agent, Relay và client trong khi có action; không nhận callback sau khi revoke/timeout, không có stream/command sống sót ngoài phiên.
- Từng capability HTTP/PTY/file/Git/Patch phải có danh sách cho phép và quyền TaskDeck cục bộ, ràng buộc đúng project/owner, audit fail-closed và chống path traversal/command injection. Chạy test thực tế cho mỗi API; không lắp HTTP proxy chung để bỏ qua middleware.
- Phân phối binary client **độc lập với PHP** bằng nguồn phát hành xác thực/signature hoặc cơ chế checksum được ghim ngoài Portal. PHP bị chiếm không thể cung cấp bản cập nhật/client giả để lấy mật khẩu và private key.
- Kiểm thử thực tế Linux và macOS (hộp thoại copy public key, TTY không echo mật khẩu, CLI key add/revoke, mất kết nối, offline, restart, bảo vệ seed); giả lập PHP+Relay bị xâm nhập và kiểm thử xâm nhập độc lập.
- Hai workflow Go và PHP phải xanh trên **cùng code commit**; không có test bị bỏ qua hoặc fail do giới hạn thời gian/rate. Kiểm thử tải không được vô tình làm Agent offline.

**Không dùng `RemoteAccessEnabled` hoặc `TASKDECK_REMOTE_LOGIN_PROBE=1` làm tiêu chí kết luận Remote Control đã an toàn.** Đến khi tất cả mục trên đạt, tính năng Remote Control vẫn bắt buộc NO-GO, không mở cổng inbound TaskDeck, không cho Portal gửi lệnh vào máy host.

## Kế hoạch thử nghiệm LOGIN-ONLY có kiểm soát

Chỉ trên host được quản lý và đã có Shared Identity + TLS:
- Duyệt key bằng lệnh CLI trên host cùng OS user chạy TaskDeck daemon; client native độc lập không tải từ PHP.
- Khai báo `TASKDECK_REMOTE_LOGIN_PROBE=1` khi khởi động tiến trình TaskDeck **để bật kiểm tra mật khẩu E2EE một lần**, không bật ứng dụng remote. Kiểm tra lại không có `remote_control_available:true`.
- Thử sai mật khẩu, thiếu key, key bị thu hồi, account bị vô hiệu hóa, thành công; kiểm tra log không có password/private seed, không có cookie/token/task execution.
- Tắt opt-in bằng cách bỏ `TASKDECK_REMOTE_LOGIN_PROBE` và restart TaskDeck nếu không thử nghiệm.

**Kết quả cuối: LOGIN-ONLY đủ điều kiện kiểm thử có kiểm soát, Remote Control NO-GO.** Cả hai workflow sau **COMPLETED SUCCESS trên cùng mã nguồn `93536acabd61aa37a0f46cdc3d47f2d5d108ecc8`**:

- [TaskDeck Remote Agent (Go) — 40859169884](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40859169884): các test Go -race, כולל nhóm `TestCompromised*` trước đây bị bỏ sót; TLS/WSS preflight thiếu/sai, Agent không mất kết nối; CLI, interop, cross-compile.
- [TaskDeck Remote Portal (PHP) — 40859169940](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40859169940): PHP lint, password-free browser, tickets, brute-force, MFA, SQL, ràng buộc chữ ký và ứng xử Relay trên cùng commit.

**Phát hiện quan trọng trong đợt kiểm định:** (1) quy tắc CI chỉ chọn `TestRemote*` bỏ sót kiểm thử `TestCompromised*`; (2) Agent cho phép gửi vào callback nếu chưa có `approved=true` từ E2EE preflight (TaskDeck nội bộ vẫn tự chặn, nhưng là rủi ro hồi quy về sau); (3) việc từ chối yêu cầu login sai thứ tự có thể kéo sập authenticated Agent presence; (4) test TLS dồn nhiều thao tác trong một socket có thể chạm frame-budget bảo vệ, không phải lỗi cần nới hạn mức. Đã bổ sung/siết guard và kết quả phát lại của Agent, chạy đầy đủ bộ negative; giữ nguyên frame-budget 12 burst/1 mỗi 2 giây và kiểm thử quota CPU ở package riêng.

**Giới hạn chứng nhận:** CI là bằng chứng kiểm thử tự động, **không phải** kiểm định xâm nhập độc lập trên hạ tầng triển khai hoặc kiểm thử thực tế popup/TTY trên cả Linux và macOS. Chưa có binary client phát hành được xác thực ngoài Portal. Đừng bật Remote Control hoặc gọi login-only diagnostic là điều khiển TaskDeck từ xa.

## CI confirmation for 2026-10-29 continuation

**Source commit**: `1ae714b42391ceeef246104c35e93c980c1d6395`.
Both workflows completed **SUCCESS on that exact source SHA**:

- Go Agent/security: [40863306541](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40863306541)
- PHP Portal/security: [40863306569](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40863306569)

The Go run includes remote server `-race`, native client verifier negative
cases and cross-builds. These automated results do **not** attest that the
manual native artifact provenance workflow was executed, or certify Linux/macOS
physical deployment, independent penetration tests, HTTP/WS/PTTY/files/Git/Patch
Remote Control, or RC-01…RC-13. Release verdict remains **NO-GO**.

## Continuation 29/10 — strict Relay ingress + idle grant revocation

**Latest changed source commit:** `85e77e5dae52aa2ed3c7f192fd6460fe1edf21ed`.
Do not confuse this with the previous audited source SHA `1ae714b4`.

**RC-04 hardening (still NOT complete):**

- `internal/remoterelay/browser_frames.go`: Relay rejects duplicate/case-folded
  JSON keys, unknown fields, empty/wrong-type values, trailing JSON,
  noncanonical/oversize ciphertext, and invalid handshake bindings before
  any authenticated Browser session is routed.
- `internal/remoterelay/http.go`: all Browser handshake, preflight, login
  and diagnostic echo frames use this strict decoder.
- After the single sealed client-key preflight response, only a login
  frame can follow; password-only V1 is refused by Relay and independently
  by the pinned Agent. Relay learns no approval boolean; Agent and TaskDeck
  remain the only parties enforcing actual key/password authorization.
- `browser_frames_test.go`: hostile JSON shapes and state-transition
  cases, including direct password-only login and echo-after-preflight.
- **Still missing:** long-lived E2EE application transport, typed dispatcher,
  flow control/cancellation/backpressure, complete per-action RBAC,
  multiproject isolation, complete end-to-end test matrix.

**RC-06 hardening (still BLOCKED for Remote CONTROL):**

- `internal/server/remote_access_portal_revoke_watcher.go`: staged internal
  E2EE sessions now recheck uncached live Portal grant on a separate 10s
  timer. Denial, timeout or unavailable Portal closes the local session.
- Independent 500ms host-local revocation checks still run even if the PHP
  grant introspection is slow/malicious. The active cancellable callback is
  cancelled through session Close().
- `remote_access_portal_revoke_watcher_test.go`: idle revoke, running
  callback revoke, and valid-grant preservation.
- **Still missing:** immediate external revoke notification, transactional
  cancellation or rollback of side effects, network end-to-end application
  session, and threat-model penetration test.

**Source-commit workflow evidence:**

- [Go Remote Agent / tests / race](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40864872450)
- [PHP Remote Portal](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40864872478)

Check these workflows' **final completed conclusions** on the exact source
commit before claiming tests PASS. Both are necessary but not sufficient
for Remote Control release.

**Release verdict unchanged: LOGIN-ONLY controlled testing only.
Remote CONTROL NO-GO; `remote_control_available=false`, no public
TaskDeck dispatcher, no TaskDeck inbound port.**

### CI result confirmed after completion (same code SHA)

At source SHA `85e77e5dae52aa2ed3c7f192fd6460fe1edf21ed`,
both mandatory source workflows completed **SUCCESS**:

- Go Agent: [40864872450](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40864872450)
- PHP Portal: [40864872478](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40864872478)

This confirms the covered unit, `-race`, protocol/negative tests and build
checks on that exact source commit. **It is NOT RC-04/RC-06 release PASS**:
there is no Internet-mounted application session/dispatcher, no live terminal
and file operations or independent on-device/pentest evidence. RC-02 also
remains partial until the manual attested native release workflow runs and
the artifacts are independently verified on actual target systems.

Remote CONTROL still **NO-GO**, with no public TaskDeck command route.

## 2026-10-29 continuation — internal RC-03/04 transport and RC-06 timeouts

The source-code checkpoint for this batch is
`9a670bc9075e3703a72cdd3a960157d4ca867b8a`
(see [application transport contract](TASKDECK_REMOTE_APPLICATION_TRANSPORT_CONTRACT.md)).
Two new internal code paths are **not mounted**:

- Native `remotetransport.PingClient`: stop-and-wait E2EE request ID,
  15-second deadline, same-Channel reply proof, one in-flight request,
  cancel/poison on replay or loss; in-process TaskDeck-authenticated
  integration and negative tests.
- Relay `app_mailbox.go`: Agent Ed25519 application-ready signature tied to
  device connection/probe, one-use activation, four one-slot opaque pings,
  20-second per-request deadline, five-minute expiry, cleanup on disconnect,
  stale ACK and signature-replay tests.
- Portal grant idle watchdog: when even a malicious callback ignores
  cancellation, deadline expiration denies and closes the local session.
  A separate test shows host-local password/key/account revocation remains
  responsive while Portal introspection is stalled.

**Security scope:** The application route/Agent session bridge is NOT connected
to the public Relay, native client's real Internet transport or TaskDeck
HTTP/terminal/files. No application command access has been enabled.
RC-03, RC-04 and RC-06 therefore remain **NOT RELEASE-READY**.

CI on this source SHA (use the final completed conclusions; do not claim PASS
while the runs are still in progress):

- Go Agent: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40867577262
- PHP Portal: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40867577279

Keep `remote_control_available=false`, zero public TaskDeck action routes,
no inbound TaskDeck port, and **13/13 RC gates required before GO**.

## Internal cross-layer test evidence — 2026-10-29

Source SHA: `a6e88c5c3c2b69c735e0fdb9798ab8045fe23664`.

`internal/server/remote_access_opaque_relay_integration_test.go` now
exercises native + Relay Registry + TaskDeck-local authorization using the
same Agent Ed25519 identity:

- Signed Relay device connect proof, signed Agent P-256 E2EE handshake and
  AES-GCM ready record, exact nonce/probe correlation.
- E2EE record counters continue across a simulated login mailbox and
  application `session.ping`, while Relay sees only encoded ciphertext.
- Server checks the real local TaskDeck SQLite password, account,
  project permission and live Portal authorization on the fixed ping.
- Additional negative tests deny a disabled local user, a revoked Portal
  grant and requests exceeding the four-round Relay mailbox quota.
- This is an **in-process fixture**. The login mailbox stage deliberately
  simulates its opaque records. The production Agent application callback
  and WSS routes remain unmounted; do not call it an Internet E2E proof.

Go CI has a focused `-race` gate on
`Test(AppMailbox|PingClient|RemoteOpaqueRelayMailbox)` before the larger
server security regression. Workflow-path changes now cause both Go and PHP
jobs to run on the **same source commit** when either remote CI definition
changes.

Workflow links for this source SHA — confirm COMPLETED SUCCESS before marking
CI green, and do not confuse green internal tests with release permission:

- Go: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40868867397
- PHP: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40868867379

Release state remains **REMOTE CONTROL NO-GO**; RC-03/04 are internal
PARTIAL only. Native signed distribution, production E2EE session routing,
per-action authorization against real network disconnects, terminal/files/
Git/Patch, compromised PHP+Relay red team, Linux/macOS real-device and
independent pentest evidence are still missing.

### Verified CI evidence — completed on the SAME source commit

`a6e88c5c3c2b69c735e0fdb9798ab8045fe23664` was tested by both
mandatory workflows with final `conclusion=success`:

- [Go Agent, -race, focused E2EE/Relay, full Remote security, cross-build — run 40868867397](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40868867397)
- [PHP Portal, SQLite/MFA/ticket/negative security tests — run 40868867379](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40868867379)

The subsequent documentation-only changes do not alter source.
This evidence is **CI PASS for the staged internal work**, NOT approval
to publish or activate Remote Control. There is no production Agent-to-Relay
application dispatcher, real network/GUI acceptance or independent pentest;
native release provenance on physical Linux/macOS remains unverified.
**Remote CONTROL remains NO-GO / `remote_control_available=false`.**
