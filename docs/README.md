# TaskDeck documentation / Tài liệu TaskDeck

## Tiếng Việt

**Bắt đầu:** [README dự án](../README.md) · [README backend Go](../vscode_tasks_menu_go/README.md).

| Bạn cần | Tài liệu |
| --- | --- |
| Tìm tính năng mới (Schedule, workflow AND/OR, role, Smart Terminal, Git/editor) | [FEATURES.md](FEATURES.md) |
| Biết tính năng nào chạy Linux, Mac native, Docker, Windows | [PLATFORM_SUPPORT.md](PLATFORM_SUPPORT.md) |
| Cài/chạy TaskDeck trên macOS Intel/Apple Silicon, native hoặc Docker Desktop | [MACOS.md](MACOS.md) |
| Cài/chạy trên Windows qua Docker/WSL2 | [WINDOWS.md](WINDOWS.md) |
| Trạng thái mã nguồn backend Go native Windows | [WINDOWS_NATIVE_PORT.md](WINDOWS_NATIVE_PORT.md) |
| Theo dõi công việc macOS, acceptance còn thiếu, CI | [MACOS_PORTABILITY_HANDOFF.md](../MACOS_PORTABILITY_HANDOFF.md) |
| Cấu hình kết nối SSH/SFTP/FTP, database và secrets | [CONNECTIONS.md](../vscode_tasks_menu_go/CONNECTIONS.md) |
| Self-update/broker/recovery, lưu ý macOS vs container | [SELF_UPDATE.md](../vscode_tasks_menu_go/SELF_UPDATE.md) |
| Quản lý shared server, TLS, user và role | [SHARED_SERVER_DEPLOYMENT.md](../vscode_tasks_menu_go/SHARED_SERVER_DEPLOYMENT.md) |
| Kiến trúc/giới hạn Schedule nâng cao | [Schedule handoff](../vscode_tasks_menu_go/SCHEDULE_FEATURE_EXPANSION_HANDOFF_20261018.md) |
| Kiểm toán phân quyền Settings và Schedule | [Role audit](../vscode_tasks_menu_go/MULTI_USER_SETTINGS_ROLE_AUDIT_20261018.md) |

**Trạng thái macOS (2026-10-18):** mã native cho Intel/ARM đã triển khai trên `feature/macos-native-support`, **chưa được chứng nhận test native trên runner/hardware PASS**. Docker Desktop là backend Linux, không phải native macOS. Thông tin an toàn và giới hạn được ghi rõ trong [macOS guide](MACOS.md).

## English

Start with the [root README](../README.md) or the [Go backend README](../vscode_tasks_menu_go/README.md). Read [Features](FEATURES.md) for recent Schedule/role/Git/editor/terminal changes, [Platform support](PLATFORM_SUPPORT.md) for operating system capabilities, [macOS](MACOS.md) for Intel/Apple Silicon native and Docker Desktop setup, or [Windows](WINDOWS.md) for Docker/WSL2.

For deeper operations: [Self-update](../vscode_tasks_menu_go/SELF_UPDATE.md), [Connections](../vscode_tasks_menu_go/CONNECTIONS.md), [Shared server](../vscode_tasks_menu_go/SHARED_SERVER_DEPLOYMENT.md), [Schedule handoff](../vscode_tasks_menu_go/SCHEDULE_FEATURE_EXPANSION_HANDOFF_20261018.md), and [Role audit](../vscode_tasks_menu_go/MULTI_USER_SETTINGS_ROLE_AUDIT_20261018.md).

**macOS native implementation is not yet fully verified by completed Intel/ARM CI and hardware acceptance.** Docker Desktop runs a Linux container. Check the platform matrix and known limitations before relying on production parity.
