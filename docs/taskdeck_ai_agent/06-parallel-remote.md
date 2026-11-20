# F06 — Parallel sessions, write lease và remote

Điều hướng: [kế hoạch chung](README.md) · [Patch](05-patch-feedback.md). Áp dụng F06, F06.1, F06.1.1–F06.1.2, F06.2, F06.2.1–F06.2.2.

## Hiện trạng

`agentJournalMu` và `agentTurnLocks` chỉ đồng bộ trong một daemon. Lưu file revision không tạo compare-and-swap liên process; không thể hứa exactly-once khi hai daemon cùng dùng storage. Các lease native hiện có phải được khảo sát contract và phạm vi trước khi reuse; không mặc định bao phủ cả Agent lẫn các writer ngoài TaskDeck. Remote `remote_access_dispatch.go`, `remote_access_task_run.go`, `internal/remotetransport` là action có allowlist/E2EE và host-side authorization; không có bằng chứng hiện tại cho phép gửi Python Patch tùy ý.

## F06.1.1 — độc lập và backpressure

1. Xây run coordinator với giới hạn concurrent session/model/execution theo owner và global, queue hữu hạn; admission atomic, trả 429/queued minh bạch, công bằng giữa owner. Mỗi session có cancel token/history/quota riêng, scheduler không giữ secret lâu hơn cần thiết.
2. Chọn cơ chế transaction liên process được hỗ trợ thật trên các platform/deployment: có thể là database private hoặc OS locking + atomic journal; phải đánh giá tương thích Go/Windows/macOS, crash, shared filesystem và migration. Không chọn phụ thuộc mới chỉ vì tiện, không gọi mutex process-local là distributed lock. Nếu chưa chứng minh được, từ chối multi-daemon cùng workspace.
3. Test hai process và race: create/claim/approval/ledger chỉ một winner; owner isolation, quota, backpressure, process kill tại từng checkpoint, store corruption. Xử lý recovery không tạo duplicate side effect.

## F06.1.2 — workspace write lease / optimistic check

- Định danh workspace bằng đường dẫn canonical thực và metadata filesystem thích hợp, xử lý symlink/alias/worktree, tránh nhầm workspace khác hoặc cùng root. Chốt scope với hệ thống lease native; tất cả Agent writer và native Patch writer phải dùng cùng arbitration, lock ordering nhất quán, không deadlock. Reader đồng thời được phép nếu không vi phạm snapshot.
- Lấy lease trước recheck project identity, HEAD/branch và preflight, giữ trong toàn bộ execution thực sự; dùng fencing/owner epoch nếu có expiry. Không giải phóng chỉ vì HTTP disconnect. Crash/restart lease phải không cho writer mới trong lúc process cũ vẫn có thể ghi; nếu không xác minh được, khóa ở UNKNOWN/manual. Git HEAD không bảo vệ thay đổi dirty hoặc writer bên ngoài TaskDeck: ghi giới hạn và từ chối khi chính sách không bảo đảm.
- Test aliases, 2 session cùng workspace, workspace khác, native manual vs Agent, timeout, crash, lease hết hạn khi process còn sống, HEAD/dirty thay đổi sát selection, không Git. Không triển khai worktree planner F08 tại đây.

## F06.2 — remote

1. Đọc [hợp đồng remote](../taskdeck_remote/TASKDECK_REMOTE_APPLICATION_TRANSPORT_CONTRACT.md) và code dispatcher để xác định operation hỗ trợ. Đề xuất operation versioned, typed, size bounded, host opt-in cho Patch review/submit/status/cancel nếu hiện chưa có; không chuyển generic HTTP, shell hay arbitrary callback qua relay. Cần security review/approval trước mở scope mới.
2. Session bind target host/project/owner + channel identity; host xác thực quyền `patch.run`/`patch.view`, project key/ID, exact digest, approval, lease, HEAD và replay protection cho từng action; kiểm tra trước/sau theo hợp đồng remote. AI key chỉ ở nơi gọi provider, không chuyển sang host hoặc relay. Mã hóa E2EE không thay thế consent gửi context đến nhà cung cấp model.
3. Host trả execution ID bền vững, trạng thái/sequence và artifact opaque; reconnect truy vấn theo ID + authorized principal; disconnect/restart không tự chạy lại, evidence thiếu thành UNKNOWN. Không dùng portal identity làm thay thế host-local authorization.
4. Test E2EE mocked/host fixture: sai host/project, quyền thu hồi, portal compromise, replay frame, disconnect trước/sau dispatch, mất process và cancel race; live remote chỉ theo môi trường/ủy quyền riêng. Thêm test và release evidence theo [07](07-acceptance.md).

## Trạng thái sau đợt hardening

Đã có: lease canonical workspace cho Agent (`agent_chat_workspace_lease.go`, in-process + OS directory lock nơi hỗ trợ), journal `flock` liên process trên Linux/macOS, approval TTL và snapshot Git. Chưa có: dùng chung lease với Patch Panel/native writer, fencing/epoch cho lease hết hạn, policy từ chối multi-daemon trên non-Unix và NFS/SMB, fault-injection đa process, và bất kỳ remote Patch operation nào.

## Cảnh báo scope E2EE

Remote Task Run hiện hữu **không** đồng nghĩa đã có API Remote Agent Patch. Không gửi patch qua generic shell/HTTP proxy; phải thiết kế typed host operation, host opt-in và verify riêng `owner/project/target/call_id/digest/permissions` cùng native receipt trước khi mở F06. Không để local session mutex thay thế multi-process/host fencing. F08 planner vẫn deferred. [Source audit](09-source-audit.md).
