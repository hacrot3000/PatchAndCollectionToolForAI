# TaskDeck on Windows · triển khai Docker / WSL2 / launcher / native

Ngày bắt đầu: 2026-10-14 · nhánh đích: `main`

## Mục tiêu
1. Docker Linux image nhỏ, vẫn có terminal PTY, Bash, Git/Git LFS, SSH/SCP/SFTP, curl, rsync, Python Patch Tool và công cụ shell.
2. Bind-mount project Windows (hoặc WSL) vào `/workspace`, cổng HTTPS chỉ publish trên Windows loopback, persistent user home.
3. Launcher PowerShell chọn project, build/update/start/stop/logs/doctor; không mount toàn bộ drive và không tự xóa volume dữ liệu.
4. CI kiểm thử image, entrypoint, môi trường, đường dẫn có khoảng trắng; handoff commit nhỏ.
5. Native Windows ConPTY: khảo sát chỗ đang phụ thuộc Unix; tách lane tương thích, không tuyên bố đã hỗ trợ đầy đủ nếu chưa kiểm thử thực tế.

## Phát hiện kiến trúc
- Go web frontend embedded; `internal/session/session.go` dùng `github.com/creack/pty` và POSIX `syscall.Kill`. Container Linux giữ nguyên tính năng; Windows native cần ConPTY và process/PTY abstraction.
- `resolveWorkspace` yêu cầu `<workspace>/.vscode/tasks.json`. Entry point Docker sẽ chỉ khởi tạo file mẫu nếu `TASKDECK_INIT_WORKSPACE=1`.
- `config.Load` tạo `.vscode/vscode_tasks_menu.ini` trong workspace; bind `0.0.0.0` chỉ hợp lệ khi auth được bật. Docker init opt-in tạo password ngẫu nhiên, config HTTPS và chmod 0600; tuyệt đối không mặc định mở HTTP 0.0.0.0 không bảo vệ.
- `patchtool.Resolve` hỗ trợ biến `TASKDECK_PATCH_RUNTIME`, cho phép bundling patch runtime tại `/opt/taskdeck/patchtool`.
- `run_windows_native_tests.ps1` hiện xác minh Python Patch Tool, **không** xác minh toàn bộ Go TaskDeck Windows.

## Tiến độ
- [ ] Phase 1A: Dockerfile multi-stage + .dockerignore + hai profile `core`/`dev`
- [ ] Phase 1B: entrypoint startup an toàn, compose mẫu, smoke tests
- [ ] Phase 2A: Windows PowerShell launcher, mount validation, lifecycle
- [ ] Phase 2B: WSL2 lane và hướng dẫn chạy; bổ sung Windows CI/smoke
- [ ] Phase 3: audit native Windows, ConPTY adapter + process semantics (chỉ đánh dấu PASS sau Windows CI thực)
