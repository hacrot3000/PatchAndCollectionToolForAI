# F05 — Tool Gateway, Patch execution và feedback

Điều hướng: [kế hoạch chung](README.md) · [source mapping](01-architecture.md). Áp dụng F05, F05.1, F05.1.1–F05.1.2, F05.2, F05.2.1–F05.2.2, F05.3, F05.3.1–F05.3.2.

## Hiện trạng và phạm vi

Gateway, approval, queue paste, native session selection, sync và explicit continuation đã có ở `internal/server/agent_chat_tools.go`, `agent_chat_native.go`, `agent_chat_sync.go`, `agent_chat_tool_feedback.go`; ledger ở `internal/agentchat/store.go`. Test bridge dùng fake native service, chưa chứng minh live Python runtime. Result collector lấy status/RC/elapsed, tối đa 24 command và output đã scrub; chưa phải artifact UI đầy đủ. Auto loop, crash reconciliation, expected HEAD và cross-process safety chưa hoàn thành.

## F05.1 — chính xác proposal/approval và idempotency

1. Proposal lưu digest của arguments và package, project ID/key-derived identity, canonical workspace, target, expected HEAD/branch và trạng thái dirty nếu chính sách yêu cầu. Xác minh dữ liệu lấy từ server, không tin Git metadata do model cung cấp. Recheck ngay trước queue, launch và selection; nếu thay đổi thì fail closed, yêu cầu proposal/approval mới. Trường hợp không Git và thay đổi ngoài Git cần chính sách riêng; HEAD đơn lẻ không phát hiện dirty/untracked.
2. Approval gắn owner, session, model turn/call, digest, workspace, target, revision, HEAD và thời hạn; chỉ user có quyền được cấp, tiêu thụ một lần bằng transaction với ledger trước side effect. Không chấp nhận approval từ model. Kiểm tra lại quyền và browser lease khi thực hiện; không tự gia hạn consent.
3. Dùng durable uniqueness `(session_id, model_turn_id, call_id)` và transaction/khóa liên process theo [06](06-parallel-remote.md). Chuyển dữ liệu cũ bằng migration tường minh; reject call ID trùng, khác arguments, hoặc không thuộc turn. Không đặt `FeedbackSent` đồng nghĩa provider đã nhận: phân biệt prepared/sending/acknowledged/unknown và correlation provider response.
4. Test đồng thời 2 process approve cùng proposal chỉ một dispatch; stale revision/owner/quyền/header/digest/HEAD, dirty change, approval hết hạn, lỗi persist trước và sau side effect. Không sửa/tái triển khai parser Python/Patch.

## F05.2 — runtime và bằng chứng

1. Dùng nguyên `patchPaste` → `sessionsRoot` → `sessionItem(prompt-response)` cùng native protocol, giữ đúng một package, digest và ID. Xem lại request context/audit/lease khi gọi handler nội bộ; mọi đường dẫn mới phải áp đúng kiểm tra của public route, không giả HTTP recorder là auth boundary.
2. Lưu mapping call → queue receipt SHA → native session/run → protocol item; chứng cứ PASS chỉ từ item đúng tên/kind/run ID với final state; output thiếu/đụng tên/ambiguous thành UNKNOWN. Lưu thông tin command stage/status/RC/time và artifact reference theo schema native, không lưu đường dẫn tùy ý để client đọc file.
3. Tạo read endpoint cho console và artifact dùng quyền `patch.view` + owner/project; kiểm tra reference native và giới hạn bytes, pagination, scrub trước egress model, không trả secret/path nội bộ. UI xem/copy tách biệt payload gửi AI, xin consent riêng. Không kết luận redaction là hoàn hảo.
4. Test fake protocol: nhiều item, duplicate tên, mismatched SHA, thiếu final, status fail, output quá lớn/secret, permission bị thu hồi. Nghiệm thu live cục bộ trong workspace test cô lập chỉ khi được cho phép, lưu log/digest/exit/result đã loại secret; chạy script patch vô hại có chủ đích và xác minh kết quả thực, không dùng workspace người dùng.

## F05.3 — recovery và vòng lặp

1. Khi startup/reconnect, scan record nonterminal trong giới hạn và dưới khóa; đối chiếu queue receipt/session ID/native protocol. Có evidence final duy nhất → cập nhật; đang chạy được xác nhận → tiếp tục theo dõi; thiếu ID, queue ambiguous, session mất sau restart, hoặc state mâu thuẫn → UNKNOWN. Tuyệt đối không queue lại tự động.
2. Tạo manual recovery workflow owner/permission: xem evidence, mở native session, đánh dấu đã kiểm tra với audit; quyết định `not run`/`completed` chỉ khi có bằng chứng phù hợp và có phê duyệt. Không cho user text trở thành giả PASS. Sau khi resolve, tạo proposal/approval mới nếu cần chạy lại.
3. Scheduler continuation server-side chỉ bật khi user consent rõ phạm vi và quota, có durable lease/claim; khi có final result, persist đúng `call_id` một lần trước egress. Nếu timeout sau khi gửi output, trạng thái gửi UNKNOWN; không resend, không rerun patch. Nếu model đưa patch tiếp theo, dừng chờ approval mới. Đóng browser không làm mất scheduler nhưng không tự kéo dài consent hay quyền.
4. Fault injection tại từng điểm persist/publish/start/select/result/send, restart cả server và native process, duplicate sync/continue, hủy đồng thời; xác nhận không có lần thực thi thứ hai. `go test ./internal/server -run '^TestAgentChat' -count=1`, `go test -race ./internal/agentchat ./internal/server -run 'TestAgentChat|Test.*Store' -count=1`. Live acceptance và automated fake evidence ghi riêng.

## Trạng thái sau đợt hardening (source + mock tests)

Đã có trong mã: provider turns dùng daemon lifecycle context thay HTTP request context; scheduler tự đọc native protocol state cho local opt-in auto-feedback khi GUI đóng; nếu native session mất hoặc package SHA/size không còn khớp thì ghi UNKNOWN; final result gắn queue SHA/size + native session + call + index + run ID; auto feedback dưới session lock và consent 30 phút. Có test giả native cho RUNNING→PASS/FAIL, missing session, restart/reconcile và đồng thời; chưa có real Python runtime.

Còn thiếu: dùng chung lease với Patch Panel thủ công và independent security audit. Các gate live vẫn NOT VERIFIED; không tuyên bố production run.

## Release blockers đã xác minh qua source (P0)

- **Internal handler auth:** `agentQueueViaNativePaste`, `agentStartNativePatch`, `agentSelectNativePatch` dùng `httptest.NewRecorder` để gọi `patchPaste`/`sessionsRoot`/`sessionItem`. Đây là code reuse, **không** tự chạy lại middleware. Phải audit owner, CSRF/origin, `PrincipalFromContext`, shared `patch.run`, audit và write lease tại mỗi side effect; hướng đến typed service layer.
- **Final PASS correlation:** `agentExactPatchSelection` chỉ chọn đúng một index theo tên; `agentPatchResultFromProtocol` chỉ match `name+kind` trong Patch Session. Cần native `run_id`, queue receipt SHA, prompt ID và owner mapping khép kín trước khi nói verified PASS. Package digest trước dispatch chưa thay thế correlation lúc kết thúc.
- **Source drift:** `TASKDECK_PROJECT_KEY/ID` xác nhận identity, **không** xác nhận HEAD/branch/dirty/untracked; thiếu server-derived drift guard và write lease trước queue/start/select.
- **Ambiguous side effects:** process-local mutex + JSON journal không cho exactly-once đa daemon; `FeedbackSent=true` trước provider request không chứng minh provider ACK. Không auto requeue/resend khi crash/timeout.

Những gate này phải giữ **BLOCKED** cho unattended execution dù mock-native integration test PASS. Chỉ test live benign Python tại workspace cô lập có quyền. [Source audit](09-source-audit.md).
