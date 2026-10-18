# TaskDeck — tính năng hiện tại và bổ sung gần đây / Feature guide

> Đối chiếu theo source và handoff tại ngày **2026-10-18**. Các mục ghi **đã triển khai trong source** không mặc nhiên chứng minh full Go tests, browser smoke, CI/hardware trên mọi hệ điều hành đã PASS. Tài liệu cho người dùng; liên kết handoff là nguồn về chi tiết kỹ thuật và điểm chưa đạt.

## Tiếng Việt

### 1. Project & Tasks

- **Global TaskDeck + Python Patch add-on:** một bản phát hành versioned chứa cả Go server và Python Patch runtime; gọi `taskdeck`, `taskdeck patch`, `taskdeck patch plan`, `taskdeck patch report`. Luồng không được phép PATCH/COLLECT vẫn giữ policy của Python.
- **Tạo Tasks tự động:** khi thiếu `.vscode/tasks.json`, hỏi trước khi quét tối đa ba cấp thư mục để đề xuất task mẫu cho Go, Node, Python, Rust, Java, Android/Gradle, Flutter, PHP, Nim…; không tự chạy command hoặc ghi đè task hiện có. CLI có `--init-tasks` cho bước chủ động không tương tác.
- **Workspace đa repo:** Git Quick Actions chọn được repo chính hoặc repo con; điều hướng trạng thái/diff, Git Log tìm kiếm và Show Diff, branch compare, conflict recovery, Git LFS preflight.
- **Editor / compare:** syntax, autocomplete, LSP tùy host, File/Folder/Git Diff, copy hunk hoặc dòng sang đối diện, Save riêng từng bên, SHA guard chống ghi đè file thay đổi ở ngoài. Structured view read-only: JSON/XML Pretty/Tree, CSV/TSV Pretty/Table; nhận diện Cocos/FairyGUI/Gradle/ProGuard.
- **SSH/SFTP/FTP/Database:** remote shell, reconnect SSH khi rớt ngoài ý muốn (không tự reconnect sau exit chủ động), transfer queue/dual-pane, các database workbench theo adapter; có thể cần cài toolchain/credential riêng.

### 2. Terminal & trải nghiệm làm việc

- PTY task/terminal riêng; split, tab restore, broadcast group có xác nhận với input lớn, preset command, OSC 133 history/recording, clipboard/copy/clear theo menu nhóm, alias/path-aware Smart Terminal completion.
- Hỗ trợ quyền sở hữu và session theo user trong shared-server, tránh user khác tự attach hoặc nhận nhầm trạng thái terminal của người khác; UI và API luôn phải kiểm tra quyền server-side.
- Browser/terminal keyboard shortcuts có nhánh Cmd trên macOS; shortcut có thể bị Firefox/Safari/macOS chặn trước khi đến TaskDeck.

### 3. Schedule/Workflow (bổ sung 17–18/10/2026)

- Mỗi lịch **mặc định single condition**, có thể thêm/xóa điều kiện để chuyển linh hoạt sang **multi AND/OR**, hỗ trợ nhóm lồng nhau. Condition gồm Cron (timezone), URL, Git, dependency, file/directory và authenticated webhook signal.
- Condition Inspector, Simulate không thực thi task, TTL evidence, retry/backoff (có exit-code allowlist), concurrency Skip/Parallel/Queue/Replace và exclusive resource group; giới hạn tài nguyên.
- Workflow Run ID, dependency fan-out/fan-in, workflow graph các lần chạy thực tế, lịch sử/console, Analytics, Alerts; Cron backfill thủ công **chỉ cho lịch Cron đơn** (preview trước).
- Pause/Resume, calendar exception, clone, JSON export/import (import mặc định disabled), basic templates, `TASKDECK_OUTPUT NAME=value` để chuyển biến kết quả giới hạn cho lịch phụ thuộc.
- **Chưa đầy đủ:** endpoint webhook public có HMAC/replay protection, email delivery, notification queue bền vững, gallery template tự định nghĩa, gửi file artifact giữa workflows, timeline lịch đa ngày. Tham khảo [handoff Schedule](../vscode_tasks_menu_go/SCHEDULE_FEATURE_EXPANSION_HANDOFF_20261018.md).

### 4. Shared server / Role-based access

- Global identity DB có thể chia sẻ giữa nhiều project daemon; project ID, membership, custom role, permission override, audit và cookie/project boundary tách riêng.
- Có ba quyền Schedule: `schedules.view`, `schedules.run`, `schedules.manage`; admin có đầy đủ. **`schedules.manage` chạy task nền dưới account TaskDeck**, chỉ cấp cho role tin cậy. Grant không mặc định cho developer/operator/viewer.
- **Settings theo role:** `settings.read` không cho xem/sửa cấu hình chung; update source, backup, terminal CWD, task config và AI provider cần `settings.write`; auth migration cần `project.admin`; update check/run còn có quyền riêng.
- Settings/terminal UI lọc nút theo role nhưng authorization **phải được thực thi phía server**, không dựa vào việc ẩn nút. [Audit Role & Settings](../vscode_tasks_menu_go/MULTI_USER_SETTINGS_ROLE_AUDIT_20261018.md) và [Terminal isolation](../vscode_tasks_menu_go/SHARED_TERMINAL_ISOLATION_HANDOFF_20261016.md).

### 5. macOS (mới trên branch `feature/macos-native-support`)

- Thêm PTY Darwin cho Intel/ARM, đọc cây tiến trình qua BSD `ps`, CWD terminal qua `lsof`, editor pinned path bằng `os.Root` (yêu cầu Go 1.25+) và thay thế symlink cài đặt bằng BSD `mv` phù hợp.
- Thêm `deploy/macos/taskdeck.sh` chạy backend Linux bằng Docker Desktop; tách Compose project/volume theo thư mục dự án, mặc định publish loopback.
- Thêm CI manual cho macOS Intel/Apple Silicon và test mock Docker launcher. **Native Mac chưa được xác nhận PASS CI và chạy trên phần cứng thực tế; metadata macOS (xattr/ACL/resource fork), reload daemon và update cần kiểm thử tiếp.**
- Hướng dẫn: [macOS](MACOS.md) · [Bảng nền tảng](PLATFORM_SUPPORT.md) · [handoff](../MACOS_PORTABILITY_HANDOFF.md).

## Quick start

```bash
# Linux or macOS native with required Go installed:
./install.sh
cd /path/to/workspace
"$HOME/.local/bin/taskdeck" --workspace "$PWD"

# macOS Docker Desktop, from TaskDeck repository root:
./deploy/macos/taskdeck.sh doctor /Users/you/projects/demo
./deploy/macos/taskdeck.sh start  /Users/you/projects/demo
```

Xem thêm: [Go backend README](../vscode_tasks_menu_go/README.md), [Connections](../vscode_tasks_menu_go/CONNECTIONS.md), [Self-update](../vscode_tasks_menu_go/SELF_UPDATE.md), [Windows](WINDOWS.md), [Shared server deployment](../vscode_tasks_menu_go/SHARED_SERVER_DEPLOYMENT.md).

---

## English — feature summary

TaskDeck combines a Go browser workspace with the separately policy-governed Python Patch Tool. Core features include per-task interactive PTYs, terminals/tabs/splits, project-aware Git actions and comparisons, local/remote files, SSH/SFTP/FTP, database workbench, editor completion and structured read-only previews. Missing `tasks.json` can be generated after confirmation using bounded project scanning; no arbitrary commands run during discovery.

Recent Schedule work supports a single default condition convertible to nested AND/OR groups, Cron/URL/Git/dependency/file/webhook triggers, condition inspection/simulation, evidence TTL, retries, concurrency/resource groups, correlated workflow execution graphs, export/import (disabled by default), alerts, analytics and bounded output variables. Public HMAC webhook intake, email notifications, durable notification queues, cross-run file artifacts, custom templates and full calendar timelines are **not complete**.

Shared-server mode adds project-scoped identity/roles, separate terminal ownership, Schedule permissions (`view`/`run`/`manage`) and `settings.write`-guarded global controls. The backend enforces these permissions independently of which UI controls are visible.

On macOS, the feature branch adds native Darwin PTY/process/file support and an alternative Docker Desktop launcher; **native Intel/ARM runtime acceptance is pending**. Native executes macOS host commands; Docker executes Linux commands inside a mounted workspace. See [macOS setup](MACOS.md), [platform matrix](PLATFORM_SUPPORT.md), and [macOS handoff](../MACOS_PORTABILITY_HANDOFF.md). For detailed feature semantics and outstanding validation, follow the linked technical handoffs above.
