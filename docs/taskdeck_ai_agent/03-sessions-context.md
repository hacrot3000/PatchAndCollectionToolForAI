# F03 — Session bền vững và context

Điều hướng: [kế hoạch chung](README.md). Áp dụng F03, F03.1, F03.1.1–F03.1.2, F03.2, F03.2.1–F03.2.2, F03.3, F03.3.1–F03.3.3.

Agent Chat session journals are private per workspace, and the server additionally checks both the workspace path and the current project ID for every scoped store read, list and write. Tabs within the same project share its conversations; another project cannot list or open them even if a journal is misplaced. Existing journals are not deleted. If a project identity changes, sessions bound to the old identity are hidden rather than silently rebound.

## Hiện trạng

`internal/agentchat/store.go` lưu một JSON/session private 0600, atomic rename, revision CAS và mutex theo process; `Session` ràng buộc owner/workspace/project/provider/model/mode, chưa có target, title, archive, version migration hay cross-process CAS. Limit 4 MiB/session, 256 history items, 32 tool records. `agent_chat_sessions.go` chỉ list/create/get, giới hạn 32 sessions/owner và kể cả session cũ; fallback legacy có thể tạo session dùng provider chưa đăng ký Agent. `agent_chat_messages.go` lưu user trước egress, first turn lấy `BuildAIPack` text ≤400 KiB và fingerprint; `scrubAIContext` là best-effort, không bảo đảm loại hết secret. Context đầy trả lỗi, chưa compaction.

## F03.1/F03.1.2 — lifecycle và schema

1. Thiết kế schema version + migration từ JSON hiện tại: giữ nguyên ID, revision, history/tool-call, timestamps; target mặc định `local` chỉ cho bản ghi cũ được chứng minh local. Giữ owner và workspace identity immutable; không tự suy session từ tab. Target remote chưa được sử dụng cho đến F06.
2. Bổ sung rename/archive/list và cancel đúng scope; quyết định delete vật lý/retention trước khi thêm API. Ràng buộc không archive/delete run đang hoạt động hoặc UNKNOWN nếu chưa có chính sách; danh sách trả summary có pagination/bounds và phân biệt active/archive. Kiểm tra route/action quyền và audit, revision CAS; limit 32 phải định nghĩa active khác archive.
3. Đưa transaction và cross-process lock/store theo [06](06-parallel-remote.md), không dùng mutex Go làm bằng chứng liên process; migration crash-safe, file cũ hợp lệ hoặc rollback được. Test 2 owner/workspace, version cũ/mới, concurrent update, crash và restart; không làm mất dữ liệu nhạy cảm.

## Trạng thái sau đợt hardening

Đã có: `schema_version`, `title`, `archived`, `target` (chỉ `local`), migration in-memory cho bản ghi cũ, giới hạn 32 chỉ tính session active, `PATCH /metadata` chặn archive khi còn `queuing/queued/dispatching/running/unknown`, store reject orphan `function_call_output`/duplicate call ID, journal có cross-process lock. Đã có thêm compaction an toàn (`internal/agentchat/compact.go`) chỉ bỏ lượt text hoàn tất trước tool exchange và ngoài tail bảo vệ, fail closed khi không còn gì an toàn. Đã có: Conversations ID chỉ dùng khi probe verified, nếu không thì replay stateless canonical history (mode lưu trữ giữ nguyên). Chưa có: budget theo token/tokenizer tương thích, delete vật lý/retention, pagination, migration crash-safe có test.

## F03.2 — AI Pack

Giữ first-turn snapshot text bound, tách system instruction ra khỏi dữ liệu không tin cậy và xác minh project ID; kiểm tra cả key khi chuẩn bị Patch. Thêm preview/redaction cảnh báo và consent trước egress theo [07](07-acceptance.md); không gửi ZIP bytes, không cắt âm thầm schema bắt buộc. Lưu fingerprint, provenance và các version cần thiết để phát hiện stale; không lưu key trong snapshot. Test pack thiếu docs/oversize/identity drift và prompt injection. Việc đánh dấu F03.2.1/2 đã làm chỉ phản ánh mock, không phải kiểm định redaction tuyệt đối.

## F03.3.2/F03.3.3 — replay và compaction

1. Conversation API: probe/create/reuse chỉ sau F02.2.2; lưu ID gắn endpoint/model/owner, không mix `conversation` với `previous_response_id`. Nếu capability chưa verified, dùng stateless trước request; lỗi sau khi gửi là ambiguous, không tự gửi lại.
2. Canonical history phải bảo toàn thứ tự, ID, arguments và output chính xác; reject orphan output, duplicate/collision theo `(session, model_turn, call_id)`, call đang pending không bị bỏ. Nâng schema có migration vì model `ToolCallRecord` hiện chỉ lưu `call_id` trong session.
3. Budget tính theo giới hạn byte hiện có và budget token theo provider (token count là ước lượng nếu không có tokenizer tương thích). Compaction chỉ loại/tóm tắt đoạn đã hoàn tất, giữ immutable instructions/project identity, input gần nhất, tool pair và unresolved commitments; không để model-generated summary trở thành system policy. Lưu provenance và snapshot hash, cung cấp review và fail closed nếu không đủ chỗ. Giữ bản canonical gốc theo retention, không ghi đè mất kiểm toán.
4. Test multi-turn stateless/stateful, orphan/collision, cắt ở ranh giới tool pair, response ID cũ, hết budget, provider switching và restart; `go test ./internal/agentchat ./internal/server -run 'TestAgentChat|Test.*Responses|Test.*Store' -count=1`.

## P1 — Tách intent khỏi delivery ACK

`ToolCallRecord.FeedbackSent` hiện được ghi **trước** HTTP call đến provider. Đây là **at-most-once outgoing attempt marker**, không chứng minh provider đã nhận hoặc đã lưu `function_call_output`; không quảng bá exactly-once delivery. Timeout/transport error cần `unknown`/manual reconciliation, không tự resend. Tương tự, mutex/CAS trong JSON session bảo vệ **một daemon**, không tạo transaction xuyên process. Chi tiết side effects và trạng thái `[pending, queuing, queued, dispatching, running, done/error, unknown]`: [Source audit](09-source-audit.md).
