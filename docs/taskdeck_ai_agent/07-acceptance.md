# F07 — Bảo mật, kiểm thử và vận hành

Điều hướng: [kế hoạch chung](README.md). Áp dụng F07, F07.1, F07.1.1–F07.1.2, F07.2, F07.2.1–F07.2.3.

## F07.1 — kiểm soát theo ranh giới

1. Lập ma trận route × method × quyền trong `internal/server/shared_authorization.go`, `server.go` và handler: auth, owner/project/workspace, same-origin/browser lease; kiểm tra lại quyền trước mỗi queue/select/cancel/egress và sau reconnect. Route không map phải fail closed. Test shared server với hai owner, quyền thiếu/bị thu hồi và ID đoán được.
2. Unknown model capability, malformed tool, identity drift, approval không hợp lệ, duplicate call và prompt mismatch đều dừng; không tin `approved` trong output. Viết audit có correlation ID/status, không ghi API key, prompt thô, source, log nhạy cảm. Kiểm tra không lộ secrets qua lỗi, response, config JSON, export, CI logs hay artifact.
3. Egress provider: duy trì HTTPS/public-IP mỗi kết nối, DNS rebinding, TLS, không redirect/proxy, timeout và size bound cho mọi adapter/stream/probe. Probe chỉ gửi dữ liệu tổng hợp giả, người dùng consent; preview context trước gửi và cảnh báo redaction không đảm bảo. Thực hiện independent security review trước release; không suy ra an toàn chỉ từ test SSRF.
4. Remote dùng host authorization/opt-in và E2EE; token/key không đi remote. Cô lập state private, kiểm tra mode/path/symlink, retention và xóa theo chính sách đã chốt; ghi nhận backup, artifact và log có thể chứa dữ liệu nhạy cảm.

## Trạng thái sau đợt hardening

Đã có: capability chưa verify → không quảng cáo tool thực thi (`agentToolCapabilityVerified`), Conversations chưa verify → stateless, approval không bao giờ đọc cờ từ model, route mới (`DELETE /providers`, `/providers/probe`, `PATCH /metadata`, `/cancel`, `GET .../execution`) đều map quyền trong `shared_authorization.go`, audit không chứa key, và lỗi không lộ secret. Còn thiếu: re-authorize tường minh/typed service thay cho `httptest` handler reuse, independent security review, và toàn bộ live acceptance.

## F07.2 — test và CI

| Mục | Test tối thiểu | Bằng chứng |
| --- | --- | --- |
| A1/A2 | AI Pack bound + mandatory instructions mỗi turn; stateful verified và stateless complete replay | mock request chính xác, oversized và untrusted context |
| A3 | Hai session/owner/model/target; history/keys không lẫn | unit + integration concurrent, remote chỉ sau F06 |
| A4 | Crash/timeout sau queue, start, select, tool output | side effect count = 1; UNKNOWN không replay |
| A5 | QUEUE/RUNNING/final, subitems console/copy | native protocol fixture + browser test, live riêng |
| A6 | project/key/HEAD/branch/dirty drift | reject trước side effect, có approval lại |
| A7 | remote disconnect/restart/permission revoke | UNKNOWN/FAILED có thể phục hồi, không false DONE |
| A8 | consent và secret sentinel | inspect request/log/export/artifact; không coi scrub là hoàn hảo |
| A9 | advisory AI và manual Patch không đổi | test regression Smart Terminal và Patch native |
| A10 | refresh/backend restart | transcript, ID, cursor và ledger nhất quán |

Trạng thái bằng chứng hiện tại trên merged HEAD: A1–A6/A10 có unit/race/fake-native tests; A7 remote Patch **BLOCKED** (không có typed operation và remote nằm ngoài scope); A8 có unit/static secret guards nhưng chưa có independent security audit; A9 có dock/Agent bootstrap test + `node test_ai_smart_terminal.cjs`; F07.2.3 interactive browser smoke remains **NOT VERIFIED**. Browser interaction, real provider, real Python Patch đều **NOT VERIFIED**.

Chạy test tập trung tại `vscode_tasks_menu_go`: `go test -race ./internal/agentchat`; `go test -race ./internal/taskmanifest`; `go test -race ./internal/server -run '^TestAgent'`; `go test ./internal/patchtool`; `go vet ./internal/agentchat ./internal/server ./internal/taskmanifest`; `node --check web/featuremods/agentchat.js`; `node web/agentchat_bootstrap_test.cjs`; `node test_ai_smart_terminal.cjs` nếu file có trong checkout. Thêm `go test -race` cho concurrency và `go test ./...` ở release gate khi môi trường cho phép. Kiểm tra tên test thực tế trước khi dùng selector; báo test không chạy/không có file, không coi regex không khớp là PASS.

Full-package server result on merged baseline `bba5eea6`: `go test ./internal/server -count=1` reported 22 failures in auth/editor/English UI/foreground tab/grouped menus/patch-panel/terminal/restart/restricted-role tests. The same exact 22 named tests failed in a clean detached worktree at `bba5eea6`; current follow-up commit `8d75b299` is Agent Chat debug/diagnostic-only, and all targeted `^TestAgent` tests pass afterward. Treat these 22 as proven baseline failures for this reviewed HEAD, not failures caused by the audit fixes.

`.github/workflows/taskdeck_agentchat_core.yml` hiện chạy race agentchat, vet (agentchat + server), syntax và toàn bộ server `^TestAgent` trên Go 1.25; nhánh `pull_request.paths` trong commit tài liệu gốc thiếu `internal/server/agent_chat_*`/authorization; đã sửa ở commit `7d6c6bed`. Khi sửa CI, bổ sung path trigger tương ứng, test mới và kiểm tra Go/platform matrix thực tế, không tuyên bố CI là live acceptance.

## Gate phát hành và vận hành

- Ghi commit, lệnh, môi trường, trạng thái, artifact đã scrub; tách unit/mock, native live, browser live, remote E2EE live, provider live và independent security review. Thiếu bất kỳ live/permission nào phải để NOT VERIFIED/BLOCKED.
- Test provider thật có thể tốn tiền và đưa data ra ngoài: chỉ chạy khi được phép, dùng project giả và dữ liệu không nhạy cảm. Live Patch/remote chỉ trên workspace test cô lập có quyền; cleanup có xác nhận mục tiêu.
- Cung cấp runbook xem session/run/queue, điều tra UNKNOWN, thu hồi local key, nâng schema/backup/rollback, quota và lỗi authorization. Không hứa rollback side effect patch chỉ bằng restore journal. Chỉ cập nhật checkbox với bằng chứng phù hợp mức yêu cầu.

## CI selector — finding and fix đã có bằng chứng

Commit `b2f2234a` có **25** server tests `TestAgent*` trong `agent_chat_*_test.go`, trong khi selector CI `-run '^TestAgentChat'` chỉ match **15**, **bỏ qua 10** `TestAgentTool*`, `TestAgentNative*`, `TestAgentApproved*`. Commit [7d6c6bed](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/7d6c6bedaccb7436ce3fa730551f7a30368e6fa9) sửa selector thành `'^TestAgent'` và thêm PR paths thiếu; [CI 44479964926](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/44479964926) **PASS**. Kết quả CI là **unit/mock Go + JS syntax**, không phải browser/native Python live/real provider/remote.

Không đóng M4/M6 khi còn thiếu middleware/auth tại internal handler, native result proof theo `run_id`+SHA, tool capability probe, HEAD/dirty/write lease và cross-process transaction. [Source audit](09-source-audit.md).
