# F01 — Kiến trúc và đối chiếu Patch native

Điều hướng: [kế hoạch chung](README.md) · [checklist](../TASKDECK_AI_AGENT_CHAT_HANDOFF_20261119.md). Áp dụng cho F01, F01.1, F01.1.1, F01.2, F01.2.1, F01.3, F01.3.1.

## Hiện trạng đã đối chiếu

- `vscode_tasks_menu_go/internal/agentchat/responses.go`: schema function `taskdeck_submit_patch` gồm `name`, `patch_python`, `additionalProperties=false`; `BuildResponsesRequest` luôn gửi instructions. Không có công cụ shell trong schema này.
- `internal/server/agent_chat_tools.go`: `parseAgentPatchArguments` giới hạn JSON/body; `agentChatToolDecisionAPI` kiểm tra owner, revision, project ID, `patch.run`, approval tường minh, lưu `queuing` trước khi gọi native paste. `pastedPythonPackage` trong `patch_paste.go` kiểm tra cả hai header `TASKDECK_PROJECT_KEY:` và `TASKDECK_PROJECT_ID:` và tạo ZIP gồm `PATCH_TOOL_MANIFEST.json` + `entrypoint.py`. Paste chỉ tạo queue, không chạy Python.
- `agent_chat_native.go`: `agentStartNativePatch` dùng `sessionsRoot` với `kind=patch`, `patch_mode=queue`, `patch_ui=native`; `agentSelectNativePatch` dùng `sessionItem` và prompt `taskdeck.patch` v1, `queue_selection`, chọn đúng một index theo tên, kiểm tra digest trước selection. Sau đó `agentPatchResultFromProtocol` đối chiếu item `PATCH`, nhận PASS/FAIL/PREFLIGHT_FAIL/INCOMPLETE và output bounded. `agent_chat_sync.go` chỉ reconcile bản ghi running khi được POST.
- `agent_chat_tool_feedback.go` lưu `function_call_output` với đúng call ID và `FeedbackSent` trước request; marker hiện là intent đã ghi, không chứng minh provider đã nhận. `store.go` chỉ khóa trong một daemon; khóa liên tiến trình chưa có. Remote dispatcher có allowlist và kiểm tra host-local; chưa chứng minh có Patch action từ xa.
- Handoff mục 7 còn snapshot M1–M3 cũ nói gateway chưa có; phần native Tool Gateway mới phản ánh code F05. Snapshot lịch sử cần được gắn nhãn và không dùng làm trạng thái triển khai hiện tại.

## F01.1/F01.2 — baseline đã có, duy trì

Khi thay đổi route hay protocol, cập nhật bảng bản đồ trong handoff, nêu rõ trạng thái source/mock/live; giữ AI advisory tách khỏi Agent. Contract context: chỉ dùng ID stateful nếu endpoint đã được xác minh; stateless replay canonical history, luôn gửi instructions, không coi AI Pack/log là chỉ thị ưu tiên cao.

## F01.3/F01.3.1 — chốt hợp đồng

1. Lập bảng field-to-field từ Responses `function_call` → `HistoryItem` → `ToolCallRecord` → `patchPasteRequest`/`patchUploadResult` → native `ProtocolState` → `function_call_output`; ghi version, ID, giới hạn và lỗi tại mỗi bước. Tra cứu source hiện tại và test trước khi chốt, không tự viết parser/schema patch thứ hai.
2. Ghi rõ khác biệt payload model là Python text, native paste đóng ZIP/manifest; đối chiếu `PATCH_PACKAGE_SCHEMA.json` trong AI Pack runtime thực tế, runtime preflight và hành vi trùng header. Phân biệt kiểm tra header khi paste với preflight native thực sự.
3. Vẽ state transition có điểm crash trước/sau ghi journal, publish ZIP, start session, select prompt và nhận final item. Ghi bằng chứng có thể truy vấn, quyền, owner, ID, digest và điểm UNKNOWN; `queuing` hiện không có receipt để tự chứng minh queue tương ứng.
4. Lập bảng operation E2EE hiện được allowlist, scope/permission/cancel. Nếu không có operation Patch tương đương, đánh dấu thiết kế F06 bị chặn cho tới khi định nghĩa và audit; không tái sử dụng task-run như thể là Patch.
5. Thêm test contract cho sai project/key, duplicate prompt item, SHA sai, protocol version sai, item khác, process exit thiếu item final. Chạy `go test ./internal/server -run '^TestAgentChat' -count=1` và `go test ./internal/agentchat -count=1`; bổ sung selector test native cụ thể sau khi kiểm tra tên thực tế.

Hoàn tất khi có source references chính xác, bảng state/side effect và test; không cần gọi provider hay thực thi remote thật. Những thay đổi execution triển khai theo [05](05-patch-feedback.md) và [06](06-parallel-remote.md).

## Contract và crash boundaries cần khóa trước live

| Bước | Source/hàm | ID/evidence | Không được tự suy |
| --- | --- | --- | --- |
| Responses | `BuildResponsesRequest` / `ParseResponsesResult` | `response.id`, `call_id`, JSON args | Provider tool capability verified |
| Proposal | `appendAgentProviderResult` / `ToolCallRecord` | owner, project, revision, call ID | User đã approve |
| Queue | `agentChatToolDecisionAPI` → `patchPaste` | project key/ID, ZIP path, SHA256 | Native preflight/PASS |
| Native | `agentStartNativePatch` → `agentSelectNativePatch` | native session ID, prompt ID, one exact name/index | Python thực thi thành công |
| Result | `agentPatchResultFromProtocol` | final per-item status, name/kind | Run/SHA correlation hoàn chỉnh |
| Feedback | `agentChatToolContinueAPI` | same call ID, `FeedbackSent` | Provider ACK |

**Mốc crash:** trước queue marker = chưa side effect; marker `queuing` nhưng thiếu receipt = UNKNOWN; có `queued` ≠ đã chạy; `dispatching` nhưng chưa xác nhận prompt = UNKNOWN; `running` chỉ xác nhận chọn item; `done/error` yêu cầu evidence native; `feedback_sent` trước egress = delivery ambiguous nếu timeout. `httptest.NewRecorder` gọi handler nội bộ **không tái áp middleware của router**. [Bảng P0/source audit](09-source-audit.md).
