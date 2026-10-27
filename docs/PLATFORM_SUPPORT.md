# TaskDeck — hỗ trợ nền tảng / Platform support

> Cập nhật: 2026-10-27. **Có mã nguồn** không đồng nghĩa với **đã kiểm thử trên thiết bị**. Xem [macOS port handoff](../MACOS_PORTABILITY_HANDOFF.md) và [Windows port handoff](../WINDOWS_PORTABILITY_HANDOFF.md). Mã macOS đã được hợp nhất vào `main` qua PR #12; đây không phải chứng nhận bản phát hành macOS native.

## Tiếng Việt

| Nền tảng | Cách chạy | Trạng thái triển khai | Lưu ý |
| --- | --- | --- | --- |
| Linux x86_64 | Native Go CLI/web, `./install.sh` | Đường sử dụng chính | PTY Linux, công cụ và filesystem trên host |
| Linux arm64 | Docker image / môi trường Linux arm64 phù hợp | Có Docker multi-arch | Kiểm tra công cụ và bản build tương ứng |
| macOS Intel (amd64) | Native Go 1.25+ | **Mã nguồn đã bổ sung, chưa xác nhận PASS CI/hardware** | Darwin PTY; BSD `ps`, `lsof`, BSD installer |
| macOS Apple Silicon (arm64) | Native Go 1.25+ | **Mã nguồn đã bổ sung, chưa xác nhận PASS CI/hardware** | Giống Intel, với PTY arm64 vendored |
| macOS Intel/Apple Silicon | Docker Desktop (Linux container) | Phương án thay thế bằng Docker Compose/launcher | Task/terminal chạy **Linux**, không chạy macOS host |
| Windows 10/11 | Docker Desktop WSL2 hoặc WSL2 | Đường được tài liệu hóa | Windows launcher `.exe` điều khiển container; không phải backend native |
| Windows native | Go/ConPTY | Chưa công bố hỗ trợ | Python Patch Tool `.ps1`/`.bat` là dự án con riêng |

### Chọn native hay Docker trên Mac?

**Native** phù hợp khi cần chạy lệnh, truy cập thư mục, SSH, toolchain Go/Python/Node và Git **trên chính host macOS**. Cần Go 1.25+ để build; Python 3.10+ được khuyến nghị cho Patch add-on. Hiện vẫn cần chạy workflow Intel/ARM, smoke trên máy thật và xem xét việc bảo toàn macOS xattr/ACL/resource fork khi editor Save.

**Docker Desktop** phù hợp khi cần môi trường Linux nhất quán, không muốn cài Go trên Mac. Không tự có quyền truy cập macOS Keychain, USB, file ngoài mount, executable host hoặc SSH key host. Mount đúng thư mục dự án; cấp quyền File Sharing trong Docker Desktop; không cấp SSH private key vào image. Container mới sử dụng HTTPS + auth do entrypoint cấu hình, nhưng cần kiểm tra chứng chỉ và password. Khi dùng nhiều workspace song song, chỉ định `TASKDECK_PORT` khác nhau.

### Các thao tác chính

```bash
# macOS native, từ checkout nhánh main
git switch main
./install.sh
cd /Users/you/projects/example
"$HOME/.local/bin/taskdeck" --workspace "$PWD"

# macOS Docker, từ root repository TaskDeck
./deploy/macos/taskdeck.sh doctor /Users/you/projects/example
./deploy/macos/taskdeck.sh start /Users/you/projects/example
./deploy/macos/taskdeck.sh status /Users/you/projects/example
./deploy/macos/taskdeck.sh stop /Users/you/projects/example
```

**Không nhầm lẫn:** installer global `install.sh` khác với script standalone Python Patch `self-install-and-update.sh`; script standalone vẫn có giả định GNU/Linux. Self-update bên trong container không thay thế được thao tác rebuild/pull image.

### Khác biệt tính năng

| Khả năng | Native Linux | Native macOS (đang kiểm thử) | Docker trên macOS/Windows |
| --- | --- | --- | --- |
| PTY / Tasks / local terminal | Linux PTY | Darwin PTY vendored; cần smoke tương tác | Linux PTY trong container |
| Live terminal CWD | Linux `/proc/<pid>/cwd` | macOS `lsof` + fallback CWD ban đầu | Linux `/proc` trong container |
| Process Tree | GNU/Linux `ps` | BSD `ps`/elapsed parser | Linux `ps` |
| Editor save an toàn | Pinned Linux file IO | Go 1.25 `os.Root` chống symlink escape; chưa xác nhận đầy đủ xattr/ACL | Linux file IO trên volume bind-mount |
| Terminal task command | Shell và executable Linux | Shell và executable trên Mac; GNU-only task cần chỉnh | Shell và executable Linux, không phải host |
| HTTPS/auth/role permission | Chính sách chung TaskDeck | Chung mã nguồn; cần kiểm thử native | Chính sách chung, kiểm tra config/port |
| Self-update/reload | Có đường broker/listener-preserving | Cần smoke thực tế; không bảo đảm reload-preserving trên mọi Mac | **Rebuild/pull image**, không self-update trong container |

Mọi tính năng Git, Schedule, SSH/SFTP/FTP, Database, Smart Terminal, Patch và RBAC đều dùng chung phần lớn backend/web code. **Điều đó không xác nhận native Mac đã đạt feature parity**: lệnh ngoài, OS process/FS semantics, TLS, browser, permission và update cần acceptance theo checklist.

### Acceptance tối thiểu để công bố macOS native

1. Tại GitHub Actions, chọn workflow `VSCode Tasks Menu Go` (trên branch hiện tại), chạy `macos-15-intel` và `macos-15`; kiểm tra bước compile, unit, security regressions, binary và installer.
2. Smoke trên cả máy Intel và Apple Silicon, gồm đăng nhập HTTPS, PTY resize/close, terminal CWD, Tasks, Git, editor Save, Schedule, SSH/SFTP, Database, Python Patch và self-update/rollback.
3. Kiểm tra symlink swap, metadata đặc biệt (ACL/xattr/resource fork) và các file dùng chung với host; ghi nhận hạn chế còn tồn tại.
4. Chạy regression Linux cùng matrix macOS trước khi tuyên bố bản native đạt nghiệm thu; việc đã merge không đồng nghĩa các phép kiểm thử đã PASS.

Liên kết: [Hướng dẫn macOS](MACOS.md) · [Hướng dẫn Windows](WINDOWS.md) · [Danh mục tính năng mới](FEATURES.md).

---

## English

**Status on 2026-10-18:** Linux-native is the primary deployment path. macOS Intel and Apple Silicon **native source is implemented but not yet verified by completed runner and real-hardware acceptance tests**. Go 1.25+ is required for the macOS pinned editor. The Docker Desktop alternative runs a **Linux backend**; it cannot run host macOS commands or use host keychain/USB without explicit integration and permissions. Windows is supported through Docker Desktop/WSL2; a native Windows Go/ConPTY backend is not claimed.

Choose native macOS if terminal tasks must execute on the host, or Docker Desktop if a self-contained Linux environment is preferred. Use `./install.sh` for native installation, `./deploy/macos/taskdeck.sh start <project>` for Docker and a distinct `TASKDECK_PORT` per concurrently running project. Do not treat a passing cross-compile as proof of runtime PTY, file security or session-broker behavior. The detailed platform matrix, known limitations, and acceptance steps above apply to both language audiences.
