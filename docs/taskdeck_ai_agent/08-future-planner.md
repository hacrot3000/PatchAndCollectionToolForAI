# F08 — Planner tương lai, chưa triển khai

Điều hướng: [kế hoạch chung](README.md). Áp dụng F08, F08.1, F08.1.1, F08.2, F08.2.1–F08.2.2. Đây là ghi chú kiến trúc, không phải quyền triển khai planner/scheduler trong phạm vi hiện tại.

## F08.1 — manifest sau khi được duyệt

Chỉ bắt đầu sau khi có yêu cầu riêng và F03/F05/F06/F07 đạt gate tương ứng. Dự kiến parse Markdown thành `TaskManifest` versioned, mỗi task có ID ổn định, mục tiêu, source refs, dependency IDs, quyền/tài nguyên, acceptance và giới hạn; coi Markdown/LLM output là không tin cậy. Validate schema/size/count, duplicate, ID lạ, cycle, dependency không tồn tại; reject thay vì tự sửa đồ thị. Cần test đồ thị vòng, disconnected, input quá lớn và content độc hại. Chốt schema/hành vi trước code.

## F08.2 — scheduler sau khi được duyệt

Dự kiến ready queue theo dependency terminal-success, bounded concurrency, durable transition/claim/fencing và backpressure; task failed/unknown không mở khóa dependent. Mỗi task có session/worktree cô lập và quota/consent/approval riêng. Controlled integration cần xác thực base HEAD, diff review, conflict, test và explicit approval trước branch mutation; không tự merge. Persist report provenance, resume/retry chỉ với hành động an toàn, test crash/concurrent scheduler và xung đột. Chọn strategy worktree/merge theo platform và chính sách người dùng vào thời điểm triển khai, không coi mô tả này là thiết kế được phê duyệt.
