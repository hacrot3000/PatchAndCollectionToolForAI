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
- [x] Phase 1A: Dockerfile multi-stage + .dockerignore + hai profile `core`/`dev`
- [x] Phase 1B: entrypoint startup an toàn, compose mẫu, smoke tests
- [x] Phase 2A: Windows PowerShell launcher, mount validation, lifecycle
- [x] Phase 2B: WSL2 lane và hướng dẫn chạy; bổ sung Windows CI/smoke
- [~] Phase 3: đã có audit + Windows compile probe; **ConPTY, named pipe broker, Job Objects và Windows-native backend chưa hỗ trợ/kiểm thử đầy đủ**

## Nhật ký commit tiến độ (nhỏ, trực tiếp main)

| Giai đoạn | Commit | Nội dung |
|---|---|---|
| Init | `24e84586` | Tạo handoff và tiêu chí |
| 1A | `325a9b38` | Multi-stage Dockerfile Go-builder / Python runtime + .dockerignore |
| 1A | `b6acab63` | Runtime là final stage mặc định, dev là profile opt-in |
| 1B | `7dbfdbc9` | Entry point non-root, tasks.json opt-in, HTTPS+auth password random |
| 1B | `1beb27b2` | Compose mount Windows /workspace, loopback host, volume HOME |
| 2A | `09dff0ba` | PowerShell launcher quản lý per-project container/image |
| 2A | `7dd073b4` | CMD wrapper Windows |
| 2B | `8638a052` | WSL2 path conversion, start/stop/status/update |
| CI | `d13c2f11` | Smoke lệnh terminal/Git/SSH/Python và HTTPS/mount có dấu cách |
| CI | `3b0f99b0` | Docker + Windows workflow bản đầu |
| CI | `f6c19df3` | Sửa cú pháp YAML image-size |
| CI | `e0b976ba` | Fix cleanup UID của volume test (không lộ mật khẩu) |
| Docs | `040efcaa` | Hướng dẫn Windows, .gitignore tránh commit secret |
| 2A | `f387516a`, `b50984d6`, `8cee09b0` | WinForms project + GUI chọn thư mục / điều khiển Docker |
| CI | `054cc5cc` | Build WinForms self-contained x64 + ZIP artifact |
| Release | `936c1616` | Opt-in build/publish GHCR amd64/arm64 cho runtime/dev |
| 2A | `169f4d43`, `781f6595` | ZIP chứa đầy đủ source Docker build; không cần clone để build image |
| Native | `323c91e0` | Audit IPC Unix, POSIX signals, PTY/ConPTY |
| Native CI | `f64920b1` | Probe Go native trên runner Windows; test Python Patch native lane |

## Kết quả kiểm thử đến thời điểm cập nhật

- **PASS** GitHub Actions [38544507063](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/38544507063):
  Docker runtime/development profile xây dựng thành công; dùng `git`,
  `git lfs`, `ssh`, `scp`, `sftp`, `bash`, `curl`, `jq`,
  `rsync`, `python3`, `make`, `ip`, `ps`, `find`, `sed`;
  khởi chạy TaskDeck HTTPS với auth=true từ mount có dấu cách.
- **PASS** [38545735067](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/38545735067):
  hai job Docker Linux smoke và đóng gói WinForms Windows x64 đều thành công.
- Runtime core được đo bằng Docker CLI tại run 38545168292:
  **315,827,235 bytes (~301 MiB)**. Runtime image không chứa Go build toolchain.
  Profile `dev` lớn hơn và không dùng mặc định.
- File `TaskDeckLauncher-win-x64.zip` (bao gồm EXE self-contained, script và
  minimal Docker build context) được upload thành GitHub Actions artifact;
  dung lượng artifact khoảng 69–70 MB tùy revision.
- Windows WSL2 script và PowerShell GUI đã qua parser/build Windows runner;
  **chưa** có smoke trên workstation Windows thực tế của người dùng.
- Windows Go native/ConPTY vẫn **BLOCKED**; báo cáo kỹ thuật:
  [docs/WINDOWS_NATIVE_PORT.md](docs/WINDOWS_NATIVE_PORT.md).
- Workflow phát hành GHCR yêu cầu chạy `workflow_dispatch` có
  `publish=true` hoặc đẩy tag `taskdeck-docker-v*`. **Chưa coi image
  đã được phát hành lên registry cho tới khi có run publish thành công.**

## Các điểm cần kiểm tra tiếp trên Windows thật

1. Đường dẫn `D:\Projects\...` có dấu cách, Unicode, liên kết NTFS,
   Git worktree/submodule, rất nhiều file và Git status cache.
2. SSH key/agent forwarding riêng, không nhúng private key; cấu hình quyền
   trong HOME volume và kiểm tra ssh-agent.
3. Restart/recreate nhiều project độc lập, đồng thời dùng cổng host khác nhau;
   không chạy hai container bind cùng host port.
4. Vòng đời websocket/terminal/SSH dài hạn trên Docker Desktop, nhiều tab,
   tự reconnect và polling dữ liệu.
5. Hiệu năng NTFS ↔ WSL2; repo lớn nên chuyển vào Linux filesystem WSL.
6. Native Windows ConPTY/Job Objects cần phát triển tiếp, **không** thay thế
   backend Docker đã được test.

## 2026-10-14 · Tiếp tục kiểm thử Windows x64 (sau Phase 2)

**Kiểm chứng được:** Docker `runtime` ~315,827,235 bytes (301 MiB),
`dev` lớn hơn theo lựa chọn; hai image chứa đủ Git/LFS, Bash, OpenSSH
ssh/scp/sftp, Python Patch runtime, rsync/curl, command tiện ích.
CI build cả hai, smoke HTTPS + mount có dấu cách, Windows launcher WinForms
self-contained ZIP. Tham khảo workflow PASS
[38545735067](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/38545735067)
và commit gần đây đã bổ sung kiểm tra nội dung ZIP sau giải nén.

**Các commit xử lý phát hiện mới:**
- `36663f84`: chẩn đoán stdout/LAST_RUN và rc khi batch fail trên Windows;
- `b9c3f7ab`, `270cd41a`: thử thay exec Windows rồi hoàn tác khi phát hiện phá idle;
- `cee2f6a4`: CI giải nén ZIP để xác minh Dockerfile, .dockerignore,
  source Go/Python và entrypoint đầy đủ;
- `7e4be2c8`: Windows fallback chạy dispatcher từ basename kèm cwd
  runtime nhằm không truyền script path có dấu cách làm token lệnh;
- `6d601419`: thêm trace nếu cả PowerShell/BAT idle fail;
- `879c579b`: cấu hình UTF-8 Windows và tiến trình con; chặn
  UnicodeEncodeError khi in log tiếng Việt (Python 3.14 cp1252);
- `7ad4cfe1`: regression test stream cp1252 + Unicode bằng unit Python;
- `30956036`: phân biệt lỗi PowerShell wrapper với dispatcher thực;
- `33a1a196`: fixture test Windows truyền project path dưới dạng argv,
  không chèn đường dẫn có dấu `\\` vào source Python.

**Windows native thực:** probe vẫn báo `creack/pty` và `syscall.Flock`
không hỗ trợ Windows; đây là lỗi compile platform đúng theo audit, chưa có
ConPTY/IPC native. Các Windows Patch Tool tests là lane độc lập, không nên
coi kết quả của nó là lỗi chạy Docker Linux. CI đầy đủ chỉ được đánh dấu
PASS khi cả các job bắt buộc thật sự PASS.
