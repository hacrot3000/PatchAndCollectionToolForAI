# F08 — Task planning và execution scheduler

Điều hướng: [kế hoạch chung](README.md). F08.1 đã được triển khai ở mức parse/validate/preview; F08.2, F08.2.1–F08.2.2 vẫn là tương lai/không được triển khai theo yêu cầu hiện tại.

## F08.1 — Markdown/canonical JSON manifest: IMPLEMENTED (planning-only)

Package `internal/taskmanifest` parses bounded Markdown and strict versioned canonical JSON into a deterministic manifest. It validates project binding, IDs, duplicate IDs, unknown/self/duplicate dependencies, cycles, acceptance warnings, count/input limits, and reports roots/blocked/topological preview. `POST /api/agent/task-manifests/validate` derives the active project ID server-side, checks permission, performs no provider call, writes no file/task and invokes no executor. The Agent Chat UI accepts Markdown or canonical JSON, renders issues/graph preview, and permits download of valid canonical JSON. `planning_only:true` and `execution_enabled:false` are part of the response contract. No user Markdown is treated as shell/Patch instructions.

## F08.2 — scheduler sau khi được duyệt

Dự kiến ready queue theo dependency terminal-success, bounded concurrency, durable transition/claim/fencing và backpressure; task failed/unknown không mở khóa dependent. Mỗi task có session/worktree cô lập và quota/consent/approval riêng. Controlled integration cần xác thực base HEAD, diff review, conflict, test và explicit approval trước branch mutation; không tự merge. Persist report provenance, resume/retry chỉ với hành động an toàn, test crash/concurrent scheduler và xung đột. Chọn strategy worktree/merge theo platform và chính sách người dùng vào thời điểm triển khai, không coi mô tả này là thiết kế được phê duyệt.

**Phân biệt scope:** server continuation của một Patch Tool đã được người dùng phê duyệt để gửi kết quả về AI thuộc **F05.3 hiện hành**; nó không đồng nghĩa AI tự chia task. DAG scheduler/worktree/merge và tự hoạch định task thuộc **F08 deferred**, chỉ triển khai nếu người dùng duyệt riêng. [Source audit](09-source-audit.md).
