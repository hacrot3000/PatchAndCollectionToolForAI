# Source audit — “New deck AI document” (2026-11-19)

**Reviewed revision:** `b2f2234a` (“New deck AI document”), all 9 Markdown files in `docs/taskdeck_ai_agent/`, canonical handoff, current Agent Chat Go/JS and workflow. The 9 Markdown files have no broken relative links in repository HEAD. This is a **source/documentation review**, not a live provider, browser or remote acceptance test.

## Kết luận

**F01–F08 và phân cấp feature/subtask nhìn chung đúng, có thể dùng làm kế hoạch triển khai. Chưa được xem là release-ready hoặc autonomous end-to-end.** Các trạng thái `[x]` ở handoff chỉ phản ánh phần code/test được nêu, không mặc định chứng minh thực thi Python hoặc provider thật. Ưu tiên khắc phục được liệt kê dưới đây theo code hiện có.

| Priority | Findings grounded in actual source | Blocker / completion evidence |
| --- | --- | --- |
| **P0 — CI coverage** | Workflow ban đầu dùng `go test ./internal/server -run '^TestAgentChat'`: **15/25** test Agent Server match; **10** test `TestAgentTool*`, `TestAgentNative*`, `TestAgentApproved*` không chạy; `pull_request.paths` cũng thiếu Agent server paths. | **Đã sửa** tại `7d6c6bed`: `^TestAgent` và PR paths đúng; [CI 44479964926](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/44479964926) **PASS**. Vẫn chỉ là Go/mock CI, không phải live runtime. |
| **P0 — Tool capability** | `agent_chat_messages.go` dùng `model.ToolCalling` do user khai báo để bật `EnablePatchTool`; `/api/agent/providers/test` chỉ kiểm tra text Responses. | Probe tool calling vô hại, stateful/SSE riêng theo model/endpoint fingerprint và consent trước khi tự nhận tương thích; unsupported/timeout phải phân biệt. |
| **P0 — Internal handler boundary** | `agentQueueViaNativePaste`, `agentStartNativePatch`, `agentSelectNativePatch` dựng `httptest.NewRecorder` để gọi `patchPaste`, `sessionsRoot`, `sessionItem`. Router middleware **không tự chạy lại**. | Audit auth, owner, CSRF/same-origin, `PrincipalFromContext`, shared `patch.run`, project identity, mutation lease/audit **tại mỗi side effect**; ưu tiên typed service interface thay vì giả lập HTTP. |
| **P0 — Native result correlation** | `agentExactPatchSelection` chọn đúng một index bằng tên trong prompt; `agentPatchResultFromProtocol` kết luận final theo `item.Name + item.Kind` trong protocol state, chưa đối chiếu end-to-end `run_id` + ZIP SHA với approval. | Không nói PASS đáng tin cho tình huống stale/reused/multiple item; bind run ID, manifest/package SHA, expected prompt ID và native session owner trước khi DONE. |
| **P0 — Write drift / multi-process** | Project key/ID có kiểm tra nhưng **không** có expected HEAD/branch/dirty gate; `agentJournalMu`, `agentTurnLocks` chỉ process-local. JSON compare-then-rename không phải cross-process CAS. | Git/source drift check, write lease và atomic claim/fencing across daemon trước unattended execution, crash/restart fault injection. |
| **P1 — Feedback semantics** | `agentChatToolContinueAPI` đặt `FeedbackSent=true` **trước** khi gọi provider. | Đây là **at-most-one outgoing attempt**, không phải provider ACK/guaranteed exactly-once delivery. Timeout = ambiguous; không auto resend. |
| **P1 — UI/turn lifecycle** | `agentBusy` chung, `AbortController` ở browser và `POST .../sync` polling khi mở panel; không có server-side unattended run coordinator. | Per-session turn state, server cancellation evidence, event journal/SSE, recovery/retention. Browser abort không chứng minh native process đã dừng. |
| **P1 — Provider + data exposure** | Provider capability chưa probe, first-turn AI Pack text scrub best-effort, chưa có preview/redaction tuyệt đối, credential deletion/retention. | Consent rõ, preview, data minimization, key lifecycle, provider capability/price/usage bounds; không hứa bí mật sẽ không bao giờ lộ chỉ từ scrub. |
| **P2 — Handoff drift** | Mục 7 canonical handoff còn snapshot M1–M3 nói chưa có gateway, gọi CI cũ “Latest”; trong phần sau đã ghi gateway mới. | Đã chỉnh nhãn lịch sử và cập nhật mô tả F05/UI; không đồng thời tuyên bố “no execution” và “native run ready”. |

## Protocol chain và trạng thái được phép suy luận

| Stage | Evidence currently available | What is NOT proven |
| --- | --- | --- |
| `function_call` | Responses `call_id`, arguments JSON, model/session ID | Không chứng minh được user approval |
| `pending` / approve | Revision + owner và native patch validation, per-call user action | Chưa chạy Python |
| `queuing` | Durable intent marker trước side effect | Crash giữa marker và receipt = UNKNOWN |
| `queued` | Native `patchPaste` receipt, SHA256/ZIP | Chưa preflight hay execute |
| `dispatching` | Native session ID đã tạo | Prompt chưa chắc đã được chấp nhận |
| `running` | `prompt_response` được chấp nhận cho đúng index | Không phải PASS |
| `done` / `error` | Final per-item status từ native `ProtocolState.Items` | Cần bổ sung đối chiếu run ID/SHA |
| `feedback_sent` | Tool output ghi trong local history trước egress | **Không chứng minh provider đã nhận** |

**Source map:** `internal/agentchat/responses.go` → `internal/server/agent_chat_response_helpers.go` → `agent_chat_tools.go` → `patch_paste.go` → `agent_chat_native.go` → `agent_chat_sync.go` → `agent_chat_tool_feedback.go`. Code và tài liệu dùng đường dẫn tương đối gốc `vscode_tasks_menu_go/`. Fake-native integration test là `TestAgentApprovedPatchRunsExactlyOneNativeQueueEntry`, chỉ gọi handler với fake Session Service, **không chạy real Python patch**.

## Release gate / bằng chứng chấp nhận

- [x] Đọc 9 file F01–F08 và xác minh liên kết Markdown tương đối.
- [x] Đối chiếu chức năng với source, test names và checklist chính.
- [x] Sửa workflow selector/PR paths, xác minh CI mới chạy PASS (Go/mock).
- [ ] Kiểm thử native benign Python PATCH trong workspace được phép và cô lập, có SHA/run ID/RC thực tế.
- [ ] Kiểm thử authentication/middleware/owner/shared lease tại **nội bộ** dispatch/approval.
- [ ] Bổ sung exact run/SHA result proof, HEAD/dirty guard, multi-process claim, crash recovery.
- [ ] Probe provider/tool/stream/stateful với consent và dữ liệu test vô hại.
- [ ] Kiểm thử UI browser, console/artifact, cancel/reconnect và permissions.
- [ ] E2EE host opt-in / typed Remote Patch action verified + security review.

**Scope không đổi:** F05.3 (server tự chuyển kết quả của patch đã phê duyệt thành `function_call_output`) thuộc tính năng hiện tại nhưng chưa xong. F08 (AI tự chia task/DAG/worktree/merge) là tương lai, vẫn hoãn. Không suy từ source tests hoặc CI PASS thành `RELEASE READY`.

**Lệnh kiểm tra (từ `vscode_tasks_menu_go/`):**

```bash
go test -race -count=1 ./internal/agentchat
go vet ./internal/agentchat
go test -count=1 ./internal/server -run '^TestAgent'
node --check web/featuremods/agentchat.js
```
