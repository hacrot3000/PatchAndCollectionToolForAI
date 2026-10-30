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


## Implementation checkpoint — Firefox offline first-pair client (2026-10-30)

**Implemented, NOT full Remote Control:** `remote_portal/offline/taskdeck_offline_client.html`
is a standalone, self-contained HTML/CSS/JS file intended to be downloaded
**once** while PHP is still trusted, then opened from local disk in Firefox.
Portal's authenticated `GET /offline-client` sends it as
`Content-Disposition: attachment` and `application/octet-stream`;
anonymous and incomplete-MFA sessions cannot download it. The HTML is
stored outside the PHP document root and the PHP website does NOT execute
its private-key code. No application WebSocket or TaskDeck action is enabled.

First-time owner procedure:

1. Sign in to the clean PHP Portal with MFA, click
   **Tải client Firefox OFFLINE để ghép đôi**; save the HTML in a trusted
   local directory. Bookmark its local `file://` URL. Do not update this
   code by downloading another copy from PHP once paired.
2. Open that local HTML in Firefox; enter the **TaskDeck local username**
   and click create Ed25519 key. Copy only the public key.
3. On the REAL TaskDeck host, run:
   `taskdeck remote-key --workspace /absolute/project add USER PUBLIC_KEY`
   (host local CLI checks SQLite user ID + enabled project membership).
   Then run `taskdeck remote-key --workspace /absolute/project host-key`.
   This **only reads the existing Agent identity**, never creates it.
4. Copy the displayed Agent Ed25519 public key into the local HTML,
   calculate SHA-256 Device ID there and check it against the host's
   displayed Device ID directly. Never accept a replacement key from PHP.
5. Use a unique strong local vault password (minimum 16 chars; not Portal
   or TaskDeck password) and download `taskdeck-offline-vault.json`.
   The vault stores public keys / Agent Device ID + PBKDF2-SHA256 salt
   (600,000 iterations) + AES-256-GCM IV and encrypted client private
   PKCS8. Neither private key nor plaintext TaskDeck credentials are sent
   to PHP. Keep a backup of HTML/vault; a lost vault requires revoke
   and re-pair on the host.
6. Re-open the *same* offline HTML, import the encrypted JSON vault
   and verify unlock locally. It validates Ed25519 client public/private
   consistency and pinned Agent Device ID without any network traffic.

Automated tests:
- `remote_portal/tests/offline_client.test.cjs`: evaluates the **actual
  embedded JS** through Node WebCrypto, checks cryptographic roundtrip and
  rejects wrong password, swapped pubkey, tampered user/pin/AEAD ciphertext.
- `remote_portal/tests/credential_boundary.test.cjs`: forbids long-lived
  client crypto/private-key storage in PHP-served online page and checks
  the attachment-only authenticated delivery.
- `remote_portal/tests/mfa_http.php`: real local PHP HTTP checks
  unauthorized download denial, MFA gate, attachment header, and success
  after MFA enrollment.
- Both Go and PHP workflows were green on source `efa43546` before
  this feature; for the new HTML, the PHP workflow on source
  `84fbee28f92aba738c299e809cd8524b39312b81` is
  [SUCCESS](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41003844995).

**Security limit / remaining work:** this milestone is **first-pair
encryption only**. It does NOT connect to TaskDeck over the Internet,
does NOT currently use client signing key for login/application traffic
from Firefox and does NOT make a remote command available. The future
client-on-wire MUST verify Agent-signed E2EE transcripts, sign a
one-use client challenge with the approved private key, and be checked
against the host's local credentials and RBAC before each action; a
malicious PHP must never be able to provide executable JS updates.
Firefox `file://` + actual Relay interoperability and physical NAT
tests remain necessary. Keep `remote_control_available=false`.

## Đợt tiếp 2026-10-30: Firefox offline E2EE diagnostics over Relay (KHÔNG CẤP REMOTE)

**Luồng sử dụng thực tế trong local HTML:**
1. Trên host đã có khóa Agent và host allowlist, client mở bản HTML file:// đã lưu từ lần PHP sạch.
2. Nhập kho khóa JSON + mật khẩu vault; HTML giải mã/kiểm local pin Device ID rồi xóa mật khẩu khỏi trường nhập.
3. Bước 5, nhập **Relay HTTPS origin** cố định từ cấu hình đã xác minh trước đó, nhấn "Tạo mã phiên và khóa ECDH dùng một lần".
4. Sao chép **chỉ** client nonce và P-256 public key công khai vào giao diện cấp ticket trên PHP. PHP chỉ phát ticket 30s; dù đã bị hacker chiếm quyền cũng không có private key client/Agent.
5. Dán ticket vào HTML offline; bấm "Kiểm tra chữ ký Agent và 3 vòng E2EE". Trình duyệt kết nối `wss://RELAY_HOST/v1/offline` và xác minh:
   - Portal ticket là dữ liệu rendezvous không tin cậy; Relay chỉ đưa gói đã có định dạng cố định.
   - Agent reachability Ed25519 signature với đúng raw public key được ghim trong kho khóa, 32-byte nonce và probe ID.
   - Agent ECDH ephemeral P-256 được ký bằng đúng khóa Agent Ed25519, binding toàn bộ E2EE transcript và SHA-256 Device ID đã ghim.
   - HKDF-SHA256(72 byte) => AES-256-GCM hai chiều, AAD chứa transcript hash/direction/sequence.
   - Authenticated ready message của Agent (seq=1), sau đó **tối đa 3 vòng** AES-GCM nonce echo qua Relay, đúng thứ tự, không pipeline/replay.
6. Firefox báo thành công chỉ khi đủ 3 reply AEAD đã xác minh. Nếu Agent key khác, chữ ký giả, nonce sai, gói AES-GCM sai hay mất mạng => từ chối; không cấp quyền TaskDeck.

**Relay:** opt-in mới `taskdeck_remote_relay -allow-offline-file-client`; mặc định **false**. Chỉ mở endpoint chẩn đoán WSS `/v1/offline`, yêu cầu `Origin: null` chính xác, không query, dùng rate-limit/replay/quota của browserSocket trước đây. **Origin:null không xác thực browser tin cậy**: HTML sandboxed trên PHP bị hack cũng có thể gửi Origin:null; host-public-key crypto và future approved-client proof/password mới là ranh giới bảo mật. Không cho JavaScript do PHP phát hành chạy trong file client offline và không nhập password TaskDeck vào PHP. PHP vẫn có thể DoS/giả tickets, nhưng không làm giả Agent transcript khi Agent seed còn an toàn.

**CI kiểm chứng:** 
- Go Agent/Relay source `eb34b39dd21250314f3ee9311af91ab1fdd08c90`: [CI PASS #41004510406](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41004510406), gồm WSS Origin:null opt-in test, PHP-origin denied, default disabled, cross-build/race red-team.
- Node WebCrypto `remote_portal/tests/offline_e2ee_crypto.test.cjs` dùng **JS thật nhúng trong HTML**, Agent signing Ed25519 thật, ECDH/P-256/HKDF và AES-GCM tương thích định dạng Go. Kiểm 3 lượt stop-and-wait, giả chữ ký/Device ID/ECDH/ready/nonce và poison sau gói giả. Test này chỉ chứng minh crypto implementation trong Node; **chưa phải test trên trình duyệt Firefox thật**.

**Giới hạn phát hành còn nguyên:**
- Chẩn đoán E2EE **không** gửi private Ed25519 client key, plaintext TaskDeck password, cookie/username TaskDeck, command. Chưa thực hiện trusted-host client-key preflight trên HTML và chưa thực hiện password-bound login.
- `/v1/offline` đang dùng fixed diagnostic envelope, **không** kết nối production application dispatcher, không có Terminal/Files/Git/Patch/Collection action.
- Chưa thử file:// qua Firefox thật, TLS/CGNAT/NAT/linux/macOS host thật; chưa đủ GO.
- `remote_control_available=false` phải giữ nguyên. Không dùng `Origin:null`, ticket PHP hoặc browser pin như capability thực thi.


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

## 2026-10-30 — Firefox offline authenticated TaskDeck password login-only (PHP-compromised threat model)

### Mục tiêu và phạm vi

- **Trusted bootstrap**: lần pair đầu tiên PHP chưa bị hack. Firefox
  lưu `remote_portal/offline/taskdeck_offline_client.html` thành tệp
  **local**, vault được mã hóa; các lần sau KHÔNG tải HTML/JS từ PHP.
- **Threat model**: sau khi pair, toàn bộ PHP HTML/JS/SQLite/ticket/role
  và Relay forwarding có thể bị hacker sửa. Khóa Ed25519 Agent ở host,
  private key client trên trình duyệt local, TaskDeck password/SQLite
  và host key registry vẫn phải độc lập không bị chiếm.
- Đợt này thêm **E2EE TaskDeck password login-only boolean** cho client
  Firefox offline sau khi Agent và private key client được xác thực;
  KHÔNG cấp remote session token, cookie, TaskDeck action hoặc quyền
  Terminal/Files/Git/Patch.

### Đã triển khai trong mã

1. `vscode_tasks_menu_go/cmd/taskdeck_remote_relay/main.go`:
   cờ `-allow-offline-login-probe` **mặc định false**, chỉ chấp nhận
   nếu đồng thời bật `-allow-offline-file-client`. Đây là opt-in
   chẩn đoán login-only, không thay đổi tính năng native login.
2. `vscode_tasks_menu_go/internal/remoterelay/http.go`: trên
   `/v1/offline`, `Origin:null` chỉ là header origin, **không**
   được coi là danh tính. Relay chỉ chấp nhận login có encrypted
   `e2ee_client_key_check` trước đó; host Agent độc lập xác minh
   approved key. Khi cờ login tắt, ngay cả login sau preflight
   cũng bị chặn. Relay không giải mã mật khẩu hay cấp quyền ứng dụng.
3. `remote_portal/offline/taskdeck_offline_client.html`:
   Firefox chỉ hiển thị **hộp nhập mật khẩu local** sau khi xác
   minh Agent Ed25519/ECDH và nhận `approved:true` *bên trong*
   Agent-authenticated AES-GCM, ký bằng host-approved client Ed25519
   private key trên chính transcript E2EE.
   Hộp mật khẩu có nút hủy/thời hạn 20 giây, xóa input khi đóng,
   bị hủy khi WSS hết hạn. Trình duyệt gửi encrypted
   `taskdeck-e2ee-login-v2` gồm TaskDeck username, password,
   client_public_key + signature mới trên channel binding.
   **Mỗi kênh một key preflight, một login và một result**. Mã
   kiểm `taskdeck-e2ee-login-result-v1` với boolean
   `authenticated`, bắt buộc `remote_control_available:false`.
   Không có bearer token/cookie hay hành động ứng dụng.
4. `remote_portal/tests/offline_e2ee_crypto.test.cjs`:
   chạy thực mã JS nhúng trong HTML, Agent giả lập Ed25519, ECDH,
   HKDF/AES-GCM bằng Node WebCrypto, kiểm login v2 trên đúng
   transcript hiện thời, chống gọi password trước preflight,
   host denied, forged login ciphertext, replay và trả
   `remote_control_available:true`.
5. `vscode_tasks_menu_go/internal/remoterelay/browser_frames_test.go`:
   kiểm các loại frame Terminal/File/Git/Patch không được nhận,
   login-only phải có preflight, login mặc định tắt.

### CI / evidence

- Chốt Go/Relay tại source `1352258b7200dbd196a536d67cbbca5593fd24ca`:
  [Go Agent/Relay #41006883306 — PASS](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41006883306),
  [PHP Portal #41006883310 — PASS](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41006883310)
  cùng code commit (bao gồm flag và fallback).
- Frontend `bd874842` Node/PHP **PASS** qua
  [#41007008017](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41007008017).
- Kiểm thử mới ban đầu FAILED vì lấy transcript của **phiên E2EE
  khác** trong assertion, không phải lỗi mật mã production;
  đã sửa ở `a8326a8c`. Kết luận kiểm thử hiện hành phải
  dựa trên workflow của đúng source mới, không gắn nhãn PASS
  trước khi CI hoàn tất.

### Phần vẫn chưa hoàn thành (NO-GO Remote Control)

- Chưa có thử nghiệm trên Firefox file:// thật / NAT/CGNAT /
  Linux/macOS Agent với Relay production và mật khẩu TaskDeck thực.
- Chưa có authenticated application WSS dispatcher/Terminal/Files/
  Git/Patch/Collection, per-action RBAC/revocation, bounded stream
  và độc lập red-team full compromise PHP.
- Chưa chứng minh signing provenance và update/backup client local
  trong môi trường production hoặc thiết bị thật.

**Release gate:** `remote_control_available=false` giữ nguyên;
login-only không chứng minh 100% an toàn hoặc đủ chuẩn live.

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

## Security re-audit 2026-10-30 — offline Firefox / hacked-PHP trust boundary

Audit baseline: `7c9a22c717425533f71edb9348ff4ef0010f2728`.
Target threat model: initial PHP pairing clean; afterwards attacker can replace
PHP HTML/JS, Portal SQLite, tickets, permissions, admin sessions and Relay
forwarded messages. TaskDeck host Agent private Ed25519, local user password
and key-allowlist, and Firefox's pinned offline HTML/vault stay trusted.

### Concrete findings and code fixes (not just documentation)

1. **P1 FAIL-CLOSED**: In `remote_portal/offline/taskdeck_offline_client.html`,
   `verifyE2eeClientKeyStatus` and `verifyE2eeLoginResult` previously
   returned an error on a malformed OUTER unencrypted Relay envelope
   (wrong `type`, boolean `remote_control_available` missing/true/string)
   without poisoning the E2EE state first. The current UI would close WSS
   on error, but the cryptographic APIs separately allowed re-verifying a
   legitimate later ciphertext on the same state. Fixed: both validators
   now revoke the one-shot session BEFORE rejecting malformed outer
   metadata. Commit `bf910599eb4605a0d42a60468bd15836bbf45044`.
   New negative tests verify malformed wrappers followed by valid Agent
   ciphertext cannot revive a key-check or login result, including attempts
   to mark Remote Control available. Commit
   `9d1558283b021c0b99216a1d4830a9e73fed1278`.

2. **P1 FIRST-PAIR PIN CONFIRMATION**: The offline pairing UI used to
   enable exporting the encrypted client key merely after calculating
   `SHA256(agent_public_key)`, without the user actively acknowledging
   comparison to `taskdeck remote-key ... host-key` on the actual host.
   This could pin a mistyped/unverified Agent identity. Fixed: explicit
   out-of-band Device ID comparison checkbox is mandatory; editing the
   Agent public key resets confirmation, and changing the TaskDeck username
   disables export until it matches the generated client key owner.
   Previously generated raw PKCS8 bytes are zeroed on key regeneration.
   Commit `9d1ba5d9ed023daceed283466356210a7a33cb70`.
   Actual HTML click-handler regression tests in
   `remote_portal/tests/offline_pairing_gate.test.cjs` (CI-enforced)
   check export disabled before confirmation, username edits, Agent key
   edits, and re-confirmation. Commit
   `d3a114ff274d88c7509d8b853c37a2ddf310debc`.

### Verified security boundaries in source (not a live system guarantee)

- The trusted local HTML uses no PHP-hosted external JS, no PHP same-origin
  persisted private key and no HTTP fetch of application code. The
  intentionally opt-in WSS `/v1/offline` is rendezvous only.
- Ed25519 Agent identity and Device ID are pinned from the host; ECDH
  transcript signature + AES-GCM authenticated ready establish channel
  identity before any client-key or password input.
- Host-approved client Ed25519 signature is tied to fresh E2EE transcript;
  Agent independently checks TaskDeck-local immutable User.ID and approved
  allowlist. Login-only sends username/password inside authenticated E2EE.
- The PHP/Relay `Origin:null` is NOT proof of trusted client identity:
  sandboxed hostile PHP content can also produce this header.
- Relay login is separately opt-in and returns only authenticated boolean.
  `remote_control_available=false`, application dispatch not mounted.
  PHP/Relay compromise can deny service, misroute, cause ticket expiry,
  or attempt user phishing, but the source model does not grant TaskDeck
  access from a forged PHP admin ticket alone.

### CI evidence and incomplete gates

- PHP Portal / Node WebCrypto CI for source
  `d3a114ff274d88c7509d8b853c37a2ddf310debc`:
  [workflow #41015315711](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41015315711)
  — **COMPLETED SUCCESS**. Covers new negative crypto states and real
  offline pairing UI checkbox clicks, plus existing PHP/MFA security tests.
- Go Agent/Relay workflow for same source:
  [workflow #41015315710](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41015315710)
  — **PENDING at this note's writing**, must check final result before
  claiming complete source CI PASS.
- Not tested in this round: real Firefox opening `file://` with
  TLS/WSS and encrypted vault on Linux/macOS, NAT/CGNAT, physical host
  deployment, fully compromised PHP dynamic exploit test, and independent
  penetration test. This CI proves tested code paths, NOT GO-LIVE
  security under every possible attacker behavior.
- Remote Control is **NO-GO**. Do NOT enable Terminal/Files/Git/Patch,
  arbitrary HTTP/WebSocket proxy, or session tokens before authenticated
  app dispatcher and per-action TaskDeck RBAC/revocation are implemented.

## Final second-audit CI closure — 2026-10-30 (supersedes pending Go CI status above)

**Verified code commit:** `8d058701fc5b407982b05a5af12489b7ba5a363b`
on `feature/taskdeck-remote-access-architecture`.

- [Go Agent/Relay workflow #41015956184](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41015956184):
  **completed SUCCESS**, including `go test -race`, full server
  remote security suite, compromised-PHP/Relay tests, CLI key checks
  and cross-compilation for Linux ARM + macOS.
- [PHP Portal/Node WebCrypto workflow #41015956185](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41015956185):
  **completed SUCCESS** on the SAME source SHA, including owner-confirmed
  Agent pin UI, malformed Relay-envelope poison-on-error tests, MFA
  protected offline-client delivery, cryptographic interoperability.

**Additional issue found when re-running Go CI**: the preceding code
`d3a114ff` had one FAIL in
`TestRemoteStagedSessionAutoClosesOnGrantRevocationAndTransportLoss`,
subcase `Portal revoked`. Audit confirmed this was **a race inside
the test itself**, not a successful stale authorization: production
`remoteTaskDeckSession.Close()` deliberately sets `closed=true`
immediately (stopping all future actions) *before* closing the Portal
grant and E2EE Channel while holding `executionMu`. The test checked
only `closed=true`, then immediately asserted both cleanup resources
were already gone. The test is now fixed to await **all three**
security conditions simultaneously, inside the **existing unchanged
2-second bound**. Neither the production Close ordering nor the
security timeout was weakened. Fix:
`8d058701fc5b407982b05a5af12489b7ba5a363b`.
The complete Go CI and PHP CI then both passed.

This source audit does **not** prove browser `file://`/WSS
interoperability in physical Firefox, production NAT/CGNAT, code-update
integrity after first pairing, or absence of every remaining
hacked-PHP-origin UX/phishing risk. No HTTP/WebSocket application
dispatcher has been mounted; password result remains a single
encrypted boolean; `remote_control_available=false` is unchanged.
**Remote Control remains NO-GO** pending physical field tests,
independent penetration testing, and per-action host RBAC/revocation.

## Third security audit — offline Firefox concurrency / identity pin / WSS lifecycle (2026-10-30)

**Baseline:** `2bf51aa8ad1e506ea79201d28332445fabe5b166`.
**Change source:** `b1014b562861e6a2a3b7920e1b07fbc2f4fa5340`.
Threat model remains: PHP/Relay may be completely compromised after the
initial trusted offline HTML download/host-local key pairing. Agent private
Ed25519 key, host registry and genuine Firefox local HTML/vault are trusted.
PHP tickets, admin/role claims, and WSS plaintext envelopes are untrusted.

### Concrete findings and corrections

1. **P1 — concurrent Ed25519 signing could reuse AES-GCM nonce** in
   `sealE2eeClientKeyCheck`. Before the fix, two concurrent calls passed
   `state.sent===0` and both awaited `crypto.subtle.sign` before
   reserving sequence 1. They could each encrypt different payloads using
   the same direction/key/IV. **Fixed** by reserving sequence 1 *before*
   all asynchronous work, poisoning on duplicate/reentrant attempts,
   never transmitting a ciphertext after the channel was poisoned, and
   zeroing the short-lived plaintext Uint8Array. Commit `db9d6079`.
2. **P1 — duplicate incoming Agent verdicts could revive a poisoned
   session**. Key-status verifier and echo verifier previously marked
   state closed while doing async AES-GCM decrypt, but a concurrent
   validation failure could be overwritten by the first successful
   asynchronous callback reopening the state. Login verifier could
   similarly return `authenticated=true` after another concurrent
   call had flagged the session invalid. **Fixed** with monotonic
   `state.poisoned`, checked again after awaited WebCrypto decrypt;
   no key-approved state or login boolean can be resurrected after a
   duplicate/error. Commit `db9d6079`.
3. **P2 — unlocked vault could change during active browser proof**.
   E2EE handshake was comparing Agent proof against the mutable global
   `trustedVault` while client-key check uses an earlier snapshot.
   **Fixed**: immutable `activeVault` captured on WSS start is used
   throughout both checks; importing a second vault while E2EE runs is
   refused, without clearing the active state. Commit `8b5d8090`
   and `22770aa5`.
4. **P2 — Relay disconnect could leave password dialog active**.
   The socket close handler previously ignored close whenever the
   asynchronous `onmessage` handler was running, including the
   interactive 20-second local password prompt. **Fixed**: immediately
   abort/close the dialog on any close during the password prompt, and
   suppress close only while actively verifying the final Agent AEAD
   response (which may race normal WebSocket shutdown). Commit
   `8b5d8090`.
5. **P3 — JS/Go username length mismatch**: Firefox used UTF-16 code
   units while host Go limits usernames by 128 UTF-8 **bytes**.
   **Fixed** client validation to use `TextEncoder` byte count, with
   negative regression for 50 multi-byte characters. Commit
   `22770aa5` and `b1014b56`.

### Regression tests and CI

- Node WebCrypto test `remote_portal/tests/offline_e2ee_crypto.test.cjs`
  now executes the real inline HTML crypto with **simultaneous** key
  sealing, parallel valid key verdicts, parallel echo replies, and
  parallel valid login results. It verifies every duplicated/poisoned
  state rejects, and no valid result opens a closed session. Commit
  `ee8ed823`.
- PHP/Node workflow [#41017666494](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41017666494)
  **completed SUCCESS** on exact source SHA
  `b1014b562861e6a2a3b7920e1b07fbc2f4fa5340`, including the new
  concurrency and UTF-8 tests.
- The latest Go Agent/Relay implementation was **not changed** in this
  audit. Its previous full Go CI at source `8d058701`
  [#41015956184](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41015956184)
  passed. Do **not** imply this is a Go CI run on `b1014b56`.

### Remaining release blockers (unchanged)

- Full trust boundary has NOT been validated with real Firefox `file://`,
  real certificate/TLS/WSS, real TaskDeck host and NAT/CGNAT links.
  A compromised PHP can still DoS, phish users outside the local
  trusted HTML, and cause ticket issuance/revocation errors.
- No trusted offline client auto-updater or authenticated code-distribution
  mechanism exists. An existing local HTML copy is deliberately not
  refreshed from potentially compromised PHP; source-only security fixes
  do NOT automatically propagate to users' previously downloaded copies.
  Any future update needs an independently pinned signing key or direct
  trusted distribution; never automatically execute JS supplied by PHP.
- Login-only result is just an E2EE-authenticated boolean; there is no
  application dispatcher, remote TaskDeck cookie, terminal/files/Git/Patch
  capability, or transferable token on the Firefox offline route.
- Per-action host TaskDeck RBAC/revocation, bounded application stream,
  production deployment and independent compromised-PHP penetration test
  remain mandatory before enabling remote application access.

**GO/NO-GO:** `remote_control_available=false` remains in force.
**NO-GO for Remote Control.** CI success covers audited code paths only,
not Firefox field operation or an absolute security guarantee.

## Fourth security audit — offline vault export integrity under concurrent re-key (2026-10-30)

**Starting HEAD:** `c7cbc620066f7eaa80678b7ba89871c21b699d28`.
**Verified code SHA:** `021a5cd274970ec9a3161d6fa72c156ca5d75788`.

### New confirmed issue — P1 private-key backup corruption

The trusted Firefox single-file UI had an asynchronous vault export:
`makeVault` imported a reference to `pending.pkcs8` and awaited WebCrypto
(Agent key import, Ed25519 test, PBKDF2 600,000 iterations), then finally
encrypted the original `pending.pkcs8` view. Concurrent click on
"Generate client Ed25519 key" called `discardPending()`, which zeroed
the SAME mutable PKCS8 buffer before AES-GCM encrypt. Result: the HTML
could download a cryptographically intact **but undecryptable/invalid
private-key backup**; the matching public key might already have been
approved by the host, stranding the authorized identity. Double export
could also produce confusing duplicate backups.

**Fix (source `0048d85491ae81406cc2a6f283a19ee356152f32`):**
- `makeVault` validates then copies the PKCS8 into a private snapshot
  **synchronously before its first await** and uses only that snapshot
  for Ed25519 private-key import and AES-GCM vault encryption.
- Wipes the temporary snapshot using `finally` after success or error.
- The pairing UI serializes key generation, Agent verification and
  vault export using in-flight flags. It disables the export button
  during work; rejects concurrent re-key/double export; rechecks
  username, Agent public key and the out-of-band Device ID confirmation
  before actually downloading; clears vault passphrase fields on
  completion/error.
- Never claims a saved/downloaded private-key backup is durable merely
  because a Blob URL was generated. The user must keep/verify the file.

**Regression tests (source `021a5cd274970ec9a3161d6fa72c156ca5d75788`):**
- `remote_portal/tests/offline_client.test.cjs`: begins vault
  encryption, **immediately wipes the input PKCS8 before the first
  WebCrypto await completes**, then opens the encrypted vault
  and verifies the original client key is still recoverable.
  This previously would have exported a damaged private key.
- `remote_portal/tests/offline_pairing_gate.test.cjs`: runs the
  **actual HTML click handlers** while PBKDF2 export is active;
  attempts re-key and duplicate export and confirms only one
  download, the original public key remains intact, the export
  button stays disabled until completion and the passphrase
  field is erased.

### Evidence

[PHP Portal/Node WebCrypto workflow #41018968495](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41018968495)
**completed SUCCESS**, matching exact source
`021a5cd274970ec9a3161d6fa72c156ca5d75788`.
Go Agent/Relay source was NOT changed in this audit;
its last full run [#41015956184](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/41015956184)
completed SUCCESS on its own previous source SHA. **Do not**
claim same-SHA Go CI verification for this fourth audit.

### Unresolved gates and restrictions

- No real Firefox `file://` test of OS download permissions, browser
  user-activation policies or WSS over a deployed HTTPS Relay.
  Even a browser-initiated Blob download is not proof it was
  written durably; require user-side backup/re-import before
  relying solely on the exported encrypted vault.
- No NAT/CGNAT/physical Linux/macOS validation, trusted client
  signed-update workflow, complete TaskDeck application action
  dispatcher, independent compromised-PHP penetration test or
  final per-action host RBAC/revocation verification.
- Continue to treat PHP/Relay as untrusted after pairing and
  `Origin:null` as non-identifying. Do not execute refreshed
  PHP JavaScript inside the already-paired offline client.

**Release status: `remote_control_available=false`, NO-GO Remote
Control.** Login-only returns an Agent-authenticated boolean, not
a TaskDeck capability or HTTP/WebSocket application session.

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
