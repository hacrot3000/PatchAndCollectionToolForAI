# HANDOFF — Các gate bắt buộc trước khi mở TaskDeck Remote Access
Ngày chốt: 2026-10-29 · Nhánh: `feature/taskdeck-remote-access-architecture`
Mốc đọc: `7587c7e1b7df8457984261aacf3027399d797fc3`
Mã nguồn đã được hai CI kiểm tra: `93536acabd61aa37a0f46cdc3d47f2d5d108ecc8`.

> **Quyết định: LOGIN-ONLY được phép thử nghiệm có kiểm soát; Remote CONTROL = NO-GO.** Hai workflow Go/PHP đã PASS trên cùng source commit, nhưng chưa chứng minh tính năng điều khiển ứng dụng từ xa an toàn. Tài liệu này là handoff để hoàn tất rồi **dừng**, không tự bật Remote Control khi chỉ PASS một phần.

## 1. Yêu cầu bất biến
1. PHP Portal chỉ quản lý tài khoản Portal/ticket/rendezvous, không có mật khẩu TaskDeck, private key Agent/client, cookie/TaskDeck principal hoặc API add/revoke key.
2. Mọi mật khẩu TaskDeck nhập **chỉ** vào native client đáng tin cậy, không vào HTML/JS PHP. Client ghim Agent Device ID độc lập ngoài PHP; chỉ hỏi mật khẩu sau khi nhận chứng thực Agent E2EE và host-approved-key preflight.
3. Client tự tạo Ed25519 seed riêng 0600, hiển thị public key để copy ở popup native (TTY fallback); không gửi private seed. TaskDeck host **chỉ qua CLI cùng OS user với daemon** thực hiện `taskdeck remote-key --workspace /absolute/project list|add|revoke`, khóa gắn immutable local SQLite User.ID và đúng project.
4. Chỉ public key đã duyệt + chữ ký private key cho đúng transcript E2EE + mật khẩu TaskDeck hợp lệ + RBAC/membership TaskDeck + grant Portal hiện hành mới được tạo session. Password-only V1, Portal admin ticket, forged `allowed=true`, key đã revoke/không duyệt, username tái tạo đều bị từ chối.
5. Agent nối **outbound 443**, không mở inbound TaskDeck; Remote OFF mặc định; local access không phụ thuộc PHP/Relay.

## 2. Evidence hiện có: KHÔNG phải PASS Remote Control
- **PASS CI source**: Go [40859169884](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40859169884) và PHP [40859169940](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40859169940), cùng commit `93536acabd61aa37a0f46cdc3d47f2d5d108ecc8`.
- So sánh mốc code tới HEAD `7587c7e1...`: chỉ `docs/TASKDECK_REMOTE_IMPLEMENTATION_STATUS.md` và `docs/TASKDECK_REMOTE_RELEASE_READINESS_20261029.md`; không có source code thay đổi.
- Đã có: native CLI login-only, Agent proof/E2EE, ký preflight khóa, host CLI key add/list/revoke, SQLite immutable user binding, login rate budget, kiểm thử chống Portal bị chiếm, TLS/WSS integration, cross-build.
- **Chưa có**: tuyến mạng cho `StartEncryptedRemoteSession`, callback/dispatcher application từ client↔Relay↔Agent; `executeRemoteE2EEPing` vẫn unmounted; HTTP/PTY/File/Git/Patch Remote chưa được kiểm chứng; chưa có binary client phát hành được xác thực ngoài Portal; chưa kiểm thử GUI/TTY trên máy thật và chưa có pentest độc lập.
- Căn cứ: [release readiness](TASKDECK_REMOTE_RELEASE_READINESS_20261029.md), [implementation status](TASKDECK_REMOTE_IMPLEMENTATION_STATUS.md), [security gate](TASKDECK_REMOTE_COMPROMISED_PHP_GO_LIVE_GATE.md).

## 3. Danh sách gate mở Remote CONTROL (tất cả đều phải PASS)
Mỗi gate yêu cầu PR/commit code, test tự động âm/dương, log chứng cứ, CI xanh đúng commit, người duyệt độc lập. Không hạ tiêu chuẩn hoặc bỏ test để làm xanh CI.

| Gate | Mức | Trạng thái tại mốc | Điều kiện PASS cụ thể |
| --- | --- | --- | --- |
| RC-01 Client identity và enrollment | P0 | CI PASS; cần kiểm thử thực tế | Linux/macOS: first-use popup copy public key, **không** hỏi password, host CLI add/list/revoke đúng User.ID, key không duyệt/revoke không được login; TTY fallback và seed owner-only. |
| RC-02 Native client provenance | P0 | **BLOCKED** | Release binary có provenance/chữ ký và quy trình xác minh độc lập PHP; update không tải/chạy từ PHP; không thể thế binary bằng Portal giả. |
| RC-03 Long-lived application session | P0 | **NOT IMPLEMENTED / BLOCKED** | Lắp phiên E2EE sau login với identity/allowlist/Portal grant/TTL đúng; không tái dùng boolean diagnostic, bearer/cookie Portal hoặc local principal serialize. |
| RC-04 Relay-Agent-client action transport | P0 | **NOT IMPLEMENTED / BLOCKED** | Transport typed E2EE hai chiều, framing versioned, bounded messages, backpressure, closed/error, replay protection; Relay opaque, không HTTP generic proxy. |
| RC-05 TaskDeck action authorization | P0 | **NOT IMPLEMENTED / BLOCKED** | Từng action có allowlist, authenticated in-process principal, project/owner, local RBAC + Portal scope, live grant recheck, audit fail-closed; không bypass sharedAuth. |
| RC-06 Session lifecycle/revoke/cancel | P0 | **BLOCKED** | Logout, revoke key, password rotation, disable user, membership removal, Portal revoke, Agent offline, timeout, relay reset đều chặn action và đóng stream; side effects/cancel/rollback xác định. |
| RC-07 HTTP/UI origin and cookie isolation | P0 | **BLOCKED** | Thiết kế origin per session, CSRF/SameSite/CSP/Origin, WS upgrade; không forward PHP cookie/TaskDeck credential; multi-project isolation và security tests. |
| RC-08 Terminal/PTY/WS and streams | P0 | **NOT IMPLEMENTED / BLOCKED** | Interactive shell create/read/write/resize/close và logs qua Remote, ownership/RBAC, chống replay và stream leak; high-risk command gated. |
| RC-09 Files/Editor/Git/Patch/Collection | P0 | **NOT IMPLEMENTED / BLOCKED** | Upload/download lớn, edit/diff/Git/Patch/Collection chính xác, path traversal/symlink/race protection, mutation lock/ACL, audit, reconnect không lặp thao tác. |
| RC-10 Compromised PHP+Relay abuse test | P0 | Partial LOGIN-ONLY CI PASS; Remote CONTROL **BLOCKED** | Toàn quyền PHP SQLite+signer+admin+Relay, nhưng thiếu password/key thì **không** đăng nhập, mở session, gửi action, thêm key hoặc lấy secrets; fake approved/token/role không có hiệu lực. |
| RC-11 Recovery/key rotation/account isolation | P0 | Partial LOGIN-ONLY CI PASS; Remote CONTROL cần xác nhận | Mất key/re-register chỉ tại host; đổi key Agent và verify pin ngoài PHP; immutable User.ID, Unicode/SQLite casefold, 2 projects/agents/tabs không chia phiên. |
| RC-12 Production deployment & independent pentest | P0 | **NOT DONE / BLOCKED** | Thử trên Linux/macOS thực, NAT/CGNAT outbound 443, PHP share host, Relay production-like, mTLS/TLS cert policy, destructive fault injection, external security review và sign-off. |
| RC-13 Full CI and release controls | P0 | LOGIN-ONLY **PASS**; Remote CONTROL **BLOCKED** | Cả Go và PHP workflows PASS cùng source SHA **sau** các thay đổi RC; `-race`, negative tests, end-to-end TLS/WSS, load/DoS, cross builds; no skips, feature flag default OFF, rollback verified. |

## 4. Checklist kiểm thử end-to-end trước GO
- [ ] Host CLI `list` rỗng; client lần đầu sinh key và popup copy; không hỏi password, PHP không có chức năng add.
- [ ] Key chưa được host duyệt: signed preflight `approved=false`; đúng password vẫn không có TaskDeck application session.
- [ ] Host CLI add key vào **đúng existing/active User.ID + project**; valid private signature/Agent pin/E2EE + đúng password mới thành công.
- [ ] Sai password, key thuộc user khác, key giả, revoked, copied public key không có private key, E2EE transcript replay => DENY.
- [ ] Host revoke giữa session hoặc trong action; disable local user/rotate password/remove membership/revoke Portal => dừng ngay/đúng SLA, không lệnh chạy sau revoke.
- [ ] PHP+Relay giả định compromised: forged admin/ticket/approved JS/binary/HTTP cookie/WS frames và brute-force không vượt TaskDeck local auth; rate-limit còn hiệu lực qua restart.
- [ ] File/PTY/Terminal/Git/Patch/upload/download/WS trên Remote origin dùng đúng local RBAC; không path traversal, symlink leak, cross-project/cross-user cookie/pty.
- [ ] Không log password, seed, TaskDeck cookie/token, HTTP body secrets; không lưu IP/endpoint của TaskDeck trong PHP SQLite; người dùng vẫn dùng local TaskDeck khi Portal/Relay sập.
- [ ] Browser disconnect/network partition/reconnect/replay, cancel/timeout/agent restart, backlog overload, TLS failure => fail closed; không duplicate POST/terminal input/Patch/file-delete.
- [ ] CI và manual matrix có URL run, exact SHA, logs/screenshot kết quả Linux+macOS, báo cáo pentest và owner approve.

## 5. Thứ tự xử lý đề xuất và điều kiện dừng
1. **Giữ đóng Remote CONTROL** và tập trung RC-02 (trusted client release), RC-03/04 (session+transport), rồi RC-05/06 (auth/action/revoke).
2. Sau đó RC-07/08/09 hoàn thiện Remote UI/API thực; RC-10/11 chạy hostile-PHP và isolation regression; RC-12/13 xác thực deploy, pentest, CI.
3. Mỗi gate cập nhật STATUS/PASS-FAIL, link exact PR/commit, automated/manual evidence. Không đánh dấu gate PASS nếu chỉ có unit tests nhưng yêu cầu cần E2E.
4. Chỉ mở tính năng khi **RC-01 đến RC-13 đều PASS**, người duyệt bảo mật chấp thuận và có rollback; trường hợp khác giữ `remote_control_available=false`, public dispatcher unmounted.

**Lưu ý phạm vi**: yêu cầu client key + CLI enrollment ở giai đoạn login-only đã hoàn thành ở mức tự động; không mở rộng/ngầm mở điều khiển chỉ vì mốc đó đã PASS.

## Bổ sung 29/10 — quy trình provenance cho native client (RC-02 PARTIAL)

Mã nguồn tiếp tục: `c0df350a0e9daed75c1750cc15acc287d5b34476`
(trên cùng nhánh; không merge `main`). Phần mới:

- `.github/workflows/taskdeck_remote_client_provenance.yml`: build thủ công
  Linux amd64/arm64 và macOS amd64/arm64, chạy test Go quan trọng, ký
  artifact provenance Sigstore/GitHub, upload **chỉ GitHub Actions**, không PHP.
- `deploy/remote/verify_client.sh`: xác minh chữ ký / workflow / GitHub repo /
  source SHA / branch và hosted runner bằng `gh attestation verify` trước khi
  giải nén, chặn symlink, duplicate hoặc file archive sai cấu trúc.
- `tests/test_remote_client_provenance.sh`: offline negative tests đối với
  thiếu provenance, source SHA không hợp lệ, symlink, file trùng, archive hỏng.
  Test giả lập `gh` chỉ chứng minh kiểm soát fail-closed; **không** thay thế
  xác minh chữ ký Sigstore thực.
- `docs/TASKDECK_REMOTE_NATIVE_CLIENT_PROVENANCE.md` và native client README:
  hướng dẫn lựa chọn SHA ngoài PHP, build/download/verify trước khi chạy.

Workflow Go/PHP cho nhánh đã được cập nhật chạy lại khi mã verifier thay đổi.
Tham chiếu CI trên source commit trên (phải đọc **kết quả cuối cùng**, không
suy luận PASS từ việc workflow đang chạy):

- Go: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40862729117
- PHP: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40862729218

**RC-02 từ BLOCKED chuyển thành PARTIAL về mã triển khai; chưa PASS.**
Cần chạy workflow phát hành thật, xác minh attestation từ máy độc lập, kiểm thử
Linux/macOS thực, review artifact, quyết định kênh phát hành/roll-back. Manual
workflow trên nhánh feature có thể chưa hiện trong GitHub Actions cho tới khi
workflow được đăng ký ở nhánh mặc định; không merge toàn bộ Remote Control chưa
kiểm định chỉ để kích hoạt workflow.

**RC-01 vẫn PARTIAL; RC-03…RC-13 không thay đổi trạng thái phát hành.**
Chỉ có thể thử **LOGIN-ONLY** trong môi trường kiểm soát. Remote CONTROL vẫn
**NO-GO**; giữ `remote_control_available=false`, không mount dispatcher.

## Bổ sung 29/10 — thu hồi quyền phiên nội bộ (RC-06 tiếp tục BLOCKED)

Source code commit: `1ae714b42391ceeef246104c35e93c980c1d6395`.

- `internal/server/remote_access_revoke_watcher.go`: sau khi
  `StartEncryptedRemoteSession` thành công, phiên E2EE nội bộ chạy giám sát
  định kỳ 500ms đối với **host-local** account, password commitment,
  immutable User.ID, project membership và approved client key. Tình trạng
  không thể xác minh trong deadline 2s cũng đóng phiên (fail closed).
- Mất E2EE channel hoặc Agent grant ngay lập tức kích hoạt Close qua
  `Done()`; Close hủy context của action đang hoạt động. Không có HTTP/PTY
  dispatcher hoặc API lệnh mới được mount.
- `remote_access_revoke_watcher_test.go`: negative test thu hồi user khi
  callback đang chạy phải trả DENY/đóng E2EE; valid session không tự bị
  đóng chỉ vì ticker.
- **Giới hạn quan trọng:** watcher chỉ cải thiện thời gian phản ứng khi host
  đổi thông tin. Đây không phải bảo đảm hủy ngay mọi side effect, không
  thay thế transaction/rollback, và chưa tự giám sát live PHP grant revoke
  giữa hai action. RC-06 vẫn **BLOCKED** cho Remote CONTROL.

Workflow tương ứng (kiểm tra kết quả cuối trên đúng code commit):

- Go: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40863306541
- PHP: https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/40863306569

**Không tự bật Remote Control; vẫn cần RC-03/04/05/07/08/09/10/11/12/13,
đồng thời bằng chứng còn thiếu của RC-01/02/06.**

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
