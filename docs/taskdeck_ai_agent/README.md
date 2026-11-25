# Kế hoạch triển khai TaskDeck AI Agent Chat

Ngày lập: 2026-11-19. Tài liệu nguồn và checklist chính: [handoff](../TASKDECK_AI_AGENT_CHAT_HANDOFF_20261119.md). Các tài liệu bên dưới là phương án triển khai, không phải bằng chứng tính năng đã hoàn thành. Đợt rà soát theo commit `b2f2234a` đã đối chiếu 9 file/đường link và code hiện tại; **chưa đạt release gate**. Chi tiết tại [Source Audit](09-source-audit.md). Không tự đổi trạng thái checkbox khi chưa có mã và bằng chứng tương ứng.

## Danh mục theo nhóm

| Nhóm | Phạm vi | Tài liệu |
| --- | --- | --- |
| F01 | Đối chiếu giao thức và kiến trúc | [01-architecture.md](01-architecture.md) |
| F02 | Provider, secret, capability, adapter | [02-providers.md](02-providers.md) |
| F03 | Session, AI Pack, context | [03-sessions-context.md](03-sessions-context.md) |
| F04 | Coordinator, streaming và UI | [04-conversation-ui.md](04-conversation-ui.md) |
| F05 | Gateway, Patch, thu kết quả và recovery | [05-patch-feedback.md](05-patch-feedback.md) |
| F06 | Đồng thời, writer lease và remote | [06-parallel-remote.md](06-parallel-remote.md) |
| F07 | Bảo mật, kiểm thử, vận hành và nghiệm thu | [07-acceptance.md](07-acceptance.md) |
| F08 | Planner tương lai, chỉ thiết kế | [08-future-planner.md](08-future-planner.md) |
| F04.UI-D | Agent Chat dock phải/dưới, auto-hide riêng và header/overflow menu (**source + DOM bootstrap; browser acceptance pending**) | [10-agent-chat-dock-panel-ui.md](10-agent-chat-dock-panel-ui.md) |
| F04.UI-DEBUG | AI Agent Debug: structured provider errors, server trace, HTTP/SSE capture và safe/raw payload (**backend capture + basic UI; acceptance còn pending**) | [11-agent-chat-debug-observability.md](11-agent-chat-debug-observability.md) |

**Rà soát liên nhóm:** [09-source-audit.md](09-source-audit.md) — báo cáo P0/P1, bằng chứng source/CI, không tạo thêm feature F09.

Mỗi dòng Fxx/Fxx.y/Fxx.y.z trong checklist nguồn liên kết trực tiếp đến tài liệu nhóm tương ứng. Khi thực hiện một mục, đọc cả phần nguyên tắc chung và phần ID cụ thể; không diễn giải mục `[x]` là được kiểm thử live.

## HANDOFF 20

- [Collection Progress Guard, Context Budget & Legacy Conversation Recovery](20-collection-progress-guard-and-legacy-recovery.md) — living checklist 10 phase / 36 task / 144 subtask; consult recorded test and remote receipts before claiming completion.

## Thứ tự và phụ thuộc thực tế

1. F01.3 chốt ranh giới parser/queue/runtime và xác minh test nền. F07 theo xuyên suốt, không chờ cuối.
2. F05.1/F06.1.2: chốt atomic ledger liên tiến trình, kiểm tra HEAD và lease trước side effect; F05.2 thu chứng cứ runtime. Có thể làm song song F02.1, F03.1.2 nếu không sửa cùng contract.
3. F05.3 recovery/reconciliation trước scheduler tự động; F03.3 đảm bảo replay nguyên vẹn; F02.2 capability probe trước khi bật stateful/stream.
4. F04 streaming/cancel/UI dựa trên state machine và durable events; sau đó F06 remote dựa trên action được thiết kế, ủy quyền và test riêng. F07 thực hiện nghiệm thu release sau các gate tương ứng.
5. F08 chỉ ghi nhận hướng tương lai, không đưa scheduler/DAG/worktree vào phạm vi hiện tại.

## Bất biến chung và cách bàn giao

- Giữ Smart Terminal AI ở chế độ tư vấn; không cấp shell hoặc đường chạy patch thứ hai. Mỗi patch cần user phê duyệt chính xác, không tin cờ approval từ model. Không coi queue receipt hay exit code là PASS.
- Ràng buộc owner, workspace thực, project ID/key, provider/model và target; kiểm tra quyền, consent và trạng thái tại từng side effect. Dữ liệu repo/log/model là không tin cậy. Không đưa key vào transcript/log/artifact/export.
- Mọi giới hạn và số liệu trong các tài liệu là đề xuất nếu không ghi rõ là hiện trạng. Giá trị production, hành vi lưu trữ, thao tác xóa và chính sách consent phải chốt trước khi triển khai; không tự mở rộng quyền.
- Với hành động có tác dụng phụ mà không chắc hoàn tất, ghi UNKNOWN và hướng dẫn điều tra, không tự replay. Chỉ thông báo success khi có chứng cứ authoritative. Với side effect đã chạy, hủy yêu cầu trình duyệt không đồng nghĩa hủy tác vụ.
- Bổ sung test tập trung, chạy test liên quan trước, cập nhật handoff theo từng checkbox với commit, lệnh, kết quả và phân biệt mock/live. Không gọi provider mất phí hoặc chạy lệnh remote thật chỉ để test nếu chưa được phép.
- Các lệnh `go test` trong bộ kế hoạch chạy từ `vscode_tasks_menu_go/`; môi trường production, Windows/macOS và shared server cần nghiệm thu riêng. Giữ tương thích Go theo `go.mod`; không suy ra hỗ trợ platform chỉ từ CI Linux.

## Những điểm cần xác nhận trước khi bật sản phẩm

- Phạm vi consent gửi dữ liệu (từng lượt hay theo run; thời hạn, thu hồi, preview) và chính sách retention/xóa transcript, key; không tự giả định consent vĩnh viễn.
- Các ngưỡng quota/chi phí và cách hành xử khi provider không báo usage; không diễn giải 0 hoặc missing là miễn phí.
- Hành vi với workspace không phải Git, branch detached, thay đổi chưa commit, symlink/mount và các process writer ngoài TaskDeck; nếu chưa có bảo đảm thì từ chối execution tự động.
- Chính sách khôi phục UNKNOWN và xung đột cross-process sau crash; không bao giờ tự coi timeout là chưa chạy.
- Remote Patch hiện chưa được chứng minh có operation tương ứng: phải có thiết kế, host opt-in và security gate riêng, không biến dispatcher hiện tại thành arbitrary HTTP/shell.

**Lưu ý trạng thái:** `queue receipt` ≠ đã chạy, `prompt_response accepted` ≠ PASS, và `FeedbackSent` ≠ provider ACK. Tại thời điểm kiểm tra, CI selector cũ bỏ qua 10/25 Agent server tests; selector mới và PR triggers đã được sửa trong commit `7d6c6bed`, CI [44479964926](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/44479964926) PASS cho tập test được chọn, không phải live acceptance.
