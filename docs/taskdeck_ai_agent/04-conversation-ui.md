# F04 — Điều phối hội thoại, streaming và giao diện

**Bố cục Agent Chat dock:** [10 — Agent Chat Dockable Panel, Right/Bottom & Auto Hide](10-agent-chat-dock-panel-ui.md). Source hiện đã chuyển UI chính sang dock Right/Bottom, có dải thu gọn, auto-hide riêng, splitter và More actions; browser smoke/a11y, mobile và kiểm thử tương tác layout vẫn chưa được xác nhận. [AI Debug](11-agent-chat-debug-observability.md) bổ sung structured provider errors, server-side bounded trace và capture safe/raw opt-in; acceptance live/browser vẫn pending.

**Đặc tả AI Debug riêng (chưa triển khai):** [11 — Agent Chat Debug / request-response observability](11-agent-chat-debug-observability.md). Yêu cầu nút Debug trên title dock, backend trace Test/Probe/Send/Tools, lỗi FAILED có nguyên nhân, console và safe/raw payload inspector.


Điều hướng: [kế hoạch chung](README.md). Áp dụng F04, F04.1, F04.1.1–F04.1.2, F04.2, F04.2.1–F04.2.3.

## Hiện trạng

`agent_chat_messages.go` và `agent_chat_tool_feedback.go` xử lý đồng bộ; `lockAgentTurn` là mutex process-local. Ghi user trước gọi model, sau đó ghi response bằng CAS; request lỗi thành `failed`, không tự retry. UI `web/featuremods/agentchat.js` đã có danh sách/session switching, chọn provider/model/mode, hiển thị nội dung patch, approve/reject, polling khi panel mở và gửi kết quả có consent. `AbortController` hiện chỉ hủy HTTP browser, không phải server-side cancellation có bằng chứng; `agentBusy` là global nên không cho nhiều lượt UI độc lập.

## F04.1/F04.1.1 — coordinator và events

1. Thiết kế run/turn ID ngẫu nhiên, request idempotency key do client tạo, digest payload và revision; lưu intent trước gọi provider. Duplicate key cùng payload trả trạng thái/run hiện có; khác payload trả conflict. Không đặt idempotency key làm bằng chứng provider đã nhận.
2. Định nghĩa bảng transition session/run/turn/tool cùng điều kiện hợp lệ và CAS. Lưu đủ phase, timestamps và error classification; failed update phải hiển thị lỗi, không nuốt lỗi lưu. Ranh giới crash sau khi provider nhận request nhưng trước khi lưu response là `unknown`, không tự replay. Thiết kế cùng transaction ledger tại [06](06-parallel-remote.md).
3. Thêm event journal có sequence monotonic theo session, giới hạn dung lượng/retention, owner-scoped `GET .../events` SSE. Event chỉ là thông báo, state bền vững là nguồn chân lý; cursor hết hạn trả lỗi/resync rõ ràng. Ghi trước khi publish, không phát token/response chưa được lưu như đã hoàn tất.
4. Reuse transport bảo vệ egress theo [02](02-providers.md), parse streaming incremental có backpressure và bound; tool calls chỉ dispatch sau response hoàn chỉnh, validate và human approval. Browser reconnect dùng cursor + snapshot revision, deduplicate event; không retry POST gây side effect.

## Trạng thái sau đợt hardening

Đã có: provider turn context thuộc daemon lifecycle (browser disconnect không cancel; explicit Cancel và daemon shutdown vẫn cancel), `request_id` idempotency digest bao gồm attachments, durable transition journal + cursor API, per-session tab draft/attachment và generation guard khi GET trả đảo thứ tự, panel execution evidence/copy, bounded cancellation và rate/concurrency admission. Chat Completions SSE có adapter riêng; UI chọn API type và chỉ cho stateful mode khi Responses endpoint được probe phù hợp. Chưa có: pause/resume API, durable token/cost quota, interactive browser acceptance thực.

## F04.1.2 — cancel/pause/resume và quota

- Cancel server-side đăng ký context theo run ID; yêu cầu quyền/owner, ghi ý định cancel, hủy provider request và gọi API native cancel có sẵn nếu chính xác run và được ủy quyền. Nếu native không xác nhận kết thúc, hiển thị `cancellation_requested` hoặc `unknown`, không `cancelled` giả. Không gửi tool output mới sau cancel.
- Pause chỉ dừng bước tiếp theo, không giả định dừng process hiện chạy. Resume kiểm tra lại owner/quyền, project, HEAD, approval, consent, deadline, ledger và capability; nếu ambiguous yêu cầu xử lý thủ công theo [05](05-patch-feedback.md). Không giữ request HTTP mở vô hạn.
- Quota per session/global: số calls/iterations, wall time, token input/output, chi phí, bytes context/output và write budget. Lưu counters bền vững và reserve atomic trước request; usage thiếu = unknown, chi phí không tự tính bằng 0. Chốt giá trị/default và loại provider trước khi bật; test exhaustion, đồng thời, crash sau reserve và partial result.

## F04.2 — UI

The session view reports whether the Patch tool is available for its provider and model. If tool calling is disabled or has no verified probe evidence, model text (including JSON that resembles commands) cannot create files. Select the session's provider and model in provider settings and explicitly run the capability probe; a successful schema probe enables Patch proposals, which still require review and approval. A schema-acceptance probe does not guarantee that a model will actually generate a function call.

File/viewer integrations use the public `globalThis.TaskMenuAgentChat.stageContext(context, options)` API to stage untrusted text with project/selection metadata. It returns `{ok, session_id, pending, attachment_count}` and never sends a message. Attachments are memory-only drafts, session-bound, and retained after failed sends; the current backend text-attachment limits remain four items, 32 KiB per item and 64 KiB total. Without an active session, staged context remains pending until the user chooses or creates a conversation. `globalThis.TaskMenuAgentChat.activeSessionID` exposes the active session for coordinated UI actions.

The API accepts `{kind, name, mime, text, source}`; `kind` is `file`, `file_selection`, `terminal_console` or `terminal_selection`. Source metadata can contain project path, 1-based selection range, terminal identity/CWD, dirty state and capture timestamp. `options` supports `open`, `focusComposer` and an explicit `session_id`. Staged text is labelled as untrusted data and grants no file-read or execution capability.

1. Chia state theo session/run, không dùng một `agentBusy` global; mỗi tab giữ model/target server-authoritative, revision, draft riêng. Add rename/archive, reasoning selector theo capability verified, attachments chỉ khi có schema, size, MIME, consent và backend thực sự hỗ trợ; không gửi bí mật hay tin metadata client.
2. Render event/status `queued/running/done/error/cancelled/unknown` theo authoritative execution; hiển thị subitems, thời gian, RC, console/copy và artifact theo quyền, không dựng thành công từ text model. Dùng `textContent`, giới hạn nội dung; không nhúng HTML model. Preview context và cảnh báo gửi ra ngoài riêng approval patch.
3. Refresh/reconnect tải snapshot, khôi phục cursor rồi nhận event, xử lý duplicate/gap và stale revision, clear busy khi lỗi. UI cho UNKNOWN và manual recovery, phân biệt browser disconnect với cancel. Không dùng localStorage chứa transcript/secrets.
4. Test browser với fake provider và native service: hai tab, duplicate click, reconnect, stale response, cancel race, refresh khi running, accessibility và mobile. Test JS `node --check web/featuremods/agentchat.js`, `go test ./internal/server -run '^TestAgentChat' -count=1`; thêm browser test tương tác thực, không xem syntax check là GUI acceptance.

## Luồng màn hình và cancel được phép hiểu

`created → requesting_ai → waiting_tool` (mới có proposal) → **review Python / approve** → `queuing/queued` (ZIP được nhận) → `dispatching/running` (native session/prompt) → `done/error` (final Protocol item) → user consent để gửi `function_call_output`. `unknown` không được hiển thị là success. Browser `AbortController` có thể cắt HTTP/model context nhưng **không chứng minh native process đã dừng**. `agentBusy` hiện chung cho panel và polling chỉ chạy khi panel mở; chưa có unattended server scheduler, per-session UI coordinator, hay cancel native verified. [Audit P1](09-source-audit.md).
