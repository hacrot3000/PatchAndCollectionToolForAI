# TaskDeck trên macOS / TaskDeck on macOS

> **Cập nhật 2026-10-27:** mã hỗ trợ macOS native đã được hợp nhất vào `main` qua PR #12 ngày 18/10/2026 cho **Intel (amd64)** và **Apple Silicon (arm64)**, nhưng chưa có xác nhận native Go tests, tương tác PTY và smoke trên máy thật PASS. Docker Desktop là phương án chạy backend Linux trên Mac. Đây không phải gói `.app` ký/notarize của macOS.

[README dự án](../README.md) · [Các tính năng mới](FEATURES.md) · [Bảng hỗ trợ nền tảng](PLATFORM_SUPPORT.md) · [Handoff macOS](../MACOS_PORTABILITY_HANDOFF.md)

## 1. Chọn cách chạy (Tiếng Việt)

| | Native Go trên Mac | Docker Desktop trên Mac |
| --- | --- | --- |
| Công cụ thực thi task/terminal | Command và executable **trên host macOS** | Command và executable **Linux trong container** |
| Mac Intel, Apple Silicon | Có source; chờ CI/hardware acceptance | Có phương án Docker Compose multi-arch |
| Yêu cầu trên host | Go **1.25+**, Bash, Git, tar, curl/wget; Python 3.10+ cho full Patch Tool | Docker Desktop và Docker Compose v2; không cần Go |
| Thư mục project | Đọc/ghi trực tiếp theo quyền của người chạy TaskDeck | Bind mount đúng một thư mục project vào `/workspace` |
| SSH / credentials | Dùng runtime/CLI được cài trên host | Phải cấp quyền, cấu hình secrets/SSH riêng trong container |
| Update | Global installer hoặc self-update sau khi đã xác minh | Rebuild/pull image, **không** chạy native self-update trong container |
| Khuyến nghị hiện tại | Đã có trên `main`; vẫn cần kiểm thử trên Mac thật | Phương án thay thế khi muốn môi trường Linux nhất quán |

## 2. Cài TaskDeck native

### 2.1 Chuẩn bị

- Không cần tự cài Go trước: `install.sh` tự tải Go phù hợp nếu chưa có hoặc bản đang dùng dưới 1.25 (native editor sử dụng `os.Root`);
- Git, Bash (macOS thường có Bash 3.2), `tar`, `curl` hoặc `wget`;
- Python **3.10+** để sử dụng đầy đủ Python Patch Tool; nếu thiếu Python, installer vẫn cài core với cảnh báo nhưng Patch add-on có thể không hoạt động đầy đủ;
- Firefox, Safari hoặc Chrome để sử dụng web UI. Không cần cài Electron hay `.app`.

Ví dụ kiểm tra trên Terminal:

```bash
uname -sm
go version
git --version
python3 --version
```

Nếu thiếu Git/CLT, có thể cần cài Xcode Command Line Tools. Không cần cài Go bằng Homebrew: `install.sh` tự tải Go từ go.dev, kiểm tra SHA-256 theo bản phát hành chính thức và lưu ở `~/.local/lib/taskdeck/toolchains/` mà không dùng `sudo`.

### 2.2 Cài từ `main` hoặc một file `install.sh` tải riêng

```bash
git clone https://github.com/hacrot3000/PatchAndCollectionToolForAI.git
cd PatchAndCollectionToolForAI
git switch main
./install.sh
```

PR #12 đã được hợp nhất vào `main`; việc hợp nhất mã nguồn không thay thế kiểm thử nghiệm thu macOS. Lưu ý **`install.sh` ở root** là trình cài global; `self-install-and-update.sh` là Python Patch standalone legacy có giả định GNU/Linux và chưa được khẳng định tương thích Mac.

Installer:
- Tự cài Go chính thức nếu cần (Intel/ARM, checksum SHA-256 ghim), rồi kiểm tra source offline-vendored, compile/validate candidate trước khi activate;
- Cài binary vào `~/.local/bin/taskdeck`;
- Lưu release có revision dưới `~/.local/lib/taskdeck/releases/<revision>/`, chuyển symlink `current`;
- Đóng gói Python Patch runtime **cùng revision** với TaskDeck để tránh lệch phiên bản.

### 2.3 Mở một project

```bash
cd /Users/you/projects/my-project
"$HOME/.local/bin/taskdeck" --workspace "$PWD"
```

Nếu `~/.local/bin` nằm trong `PATH`, dùng `taskdeck --workspace "$PWD"`. Trình duyệt sẽ được mở qua lệnh macOS `open` nếu cấu hình cho phép. Nếu không muốn tự mở browser, thêm `--no-browser`. TaskDeck sử dụng thư mục workspace bạn chỉ định; không phải thư mục repository TaskDeck.

Khi thiếu `.vscode/tasks.json`, bản interactive hỏi có muốn quét project **tối đa ba cấp** và tạo template theo toolchain hay không. Không tự động chạy lệnh mẫu. Khi dùng CLI không tương tác và bạn chủ động cho phép tạo file:

```bash
"$HOME/.local/bin/taskdeck" --workspace "$PWD" --init-tasks --no-browser
```

## 3. Chạy bằng Docker Desktop

### 3.1 Launcher macOS (khuyến nghị cho Docker)

Cài/chạy Docker Desktop và cho phép File Sharing tới thư mục project. Từ **root repository TaskDeck**:

```bash
./deploy/macos/taskdeck.sh doctor /Users/you/projects/my-project
./deploy/macos/taskdeck.sh start  /Users/you/projects/my-project
./deploy/macos/taskdeck.sh status /Users/you/projects/my-project
./deploy/macos/taskdeck.sh logs   /Users/you/projects/my-project
./deploy/macos/taskdeck.sh update /Users/you/projects/my-project
./deploy/macos/taskdeck.sh stop   /Users/you/projects/my-project
```

- `start` / `update` build Docker image local nếu cần và tạo/recreate container; `logs` theo dõi log, `stop` gọi `compose down` **không xóa named volume**.
- Launcher đặt Compose project riêng bằng hash đường dẫn canonical workspace, dùng `TASKDECK_WORKSPACE` để bind mount vào `/workspace`, và chỉ publish cổng loopback theo Compose.
- `TASKDECK_PORT` mặc định `42882`. Nếu hai project chạy đồng thời, chọn port khác nhau:

```bash
TASKDECK_PORT=42883 ./deploy/macos/taskdeck.sh start /Users/you/projects/second-project
```

- `TASKDECK_PROFILE=runtime` (mặc định) hoặc `dev` (có thêm compiler/Node toolchain trong image); có thể dùng `TASKDECK_NO_BROWSER=1` để không gọi `open`.

Mở `https://localhost:42882` (hoặc cổng đã chọn). Khi chưa có cấu hình, entrypoint tạo HTTPS/auth và password ngẫu nhiên trong `<project>/.vscode/vscode_tasks_menu.ini`. **Không commit file mật khẩu**. Chứng chỉ tự ký gây cảnh báo trên browser; đối chiếu fingerprint trước khi trust.

### 3.2 Docker Compose thủ công (không dùng launcher)

Từ root TaskDeck:

```bash
cp deploy/docker/.env.example deploy/docker/.env
```

Sửa file `deploy/docker/.env`:

```dotenv
TASKDECK_WORKSPACE=/Users/you/projects/my-project
TASKDECK_PORT=42882
TASKDECK_PROFILE=runtime
TASKDECK_IMAGE=taskdeck:local
```

Sau đó chạy:

```bash
docker compose --env-file deploy/docker/.env -f deploy/docker/compose.yaml up -d --build
docker compose --env-file deploy/docker/.env -f deploy/docker/compose.yaml logs -f
docker compose --env-file deploy/docker/.env -f deploy/docker/compose.yaml down
```

Cấu hình Compose thủ công dùng Compose project mặc định của repo, **khác** cơ chế tách project theo hash của launcher; chọn một cách nhất quán trên mỗi workspace. Không dùng `down --volumes` nếu muốn giữ home volume. Không mount Docker socket, thư mục `~/.ssh` có quyền ghi hoặc ổ đĩa khác trừ khi bạn hiểu rõ rủi ro. Docker container không thể tự dùng `xcodebuild`/Keychain/USB và không chạy lệnh macOS host.

## 4. Tính năng và khác biệt macOS

| Tính năng | macOS native (`main`) | Docker Desktop |
| --- | --- | --- |
| Local terminal / tasks | `creack/pty` Darwin, lệnh host | PTY và lệnh Linux |
| Theo dõi CWD của shell | macOS `lsof`; fallback CWD ban đầu | Linux `/proc` |
| Process Tree | BSD `ps`, parse `etime` | Linux `ps` |
| Editor Save | Descriptor-relative `os.Root` + atomic file rename | Linux pinned file IO |
| Git/Smart Terminal/Schedules/Connections | Chung logic backend/web; cần smoke trên Mac | Chung logic backend/web; công cụ phải có trong container |
| Authentication/RBAC | Cùng kiểm tra backend; cần kiểm thử nhiều user | HTTPS/auth hiện có; cần cấu hình an toàn |
| Cập nhật | Installer native / self-update (cần smoke) | Rebuild/pull image |

**Không khẳng định parity trên Mac chỉ nhờ source hoặc cross-compile.** Cụ thể:

- **Editor metadata:** đường save native bảo toàn mode/ownership thông thường nhưng hiện **chưa chứng minh bảo toàn toàn bộ ACL, extended attributes, resource forks**. Sao lưu và không sửa file nhạy cảm metadata cho đến khi đã kiểm thử đầy đủ.
- **Shell & tools:** macOS thường mặc định `zsh`, nhưng task shell TaskDeck có thể dùng `/bin/bash`; lệnh GNU-only như `readlink -f` có thể không tồn tại trên BSD. Cài tool thích hợp hoặc sửa task. Các language server / CLI database / SSH client tùy hệ thống.
- **Daemon/config:** không coi broker-preserving `--reload-config`, listener handoff hay self-update native là đã được kiểm thử an toàn trên Mac; test update và rollback thực tế.
- **Schedule:** Cron timezone / file watcher / URL/Git triggers và workflow dựa vào process, clock, filesystem, permission của host. Thử cả máy sleep/wake và symlink; đừng bật tự chạy tác vụ nguy hiểm khi chưa kiểm tra. Chi tiết tính năng tại [FEATURES.md](FEATURES.md).

## 5. Kiểm thử trước khi phát hành native

Trên GitHub Actions, chọn workflow **TaskDeck macOS native (Intel + Apple Silicon)** → **Run workflow** → branch `main`. Workflow đã khai báo cả `macos-15-intel` và `macos-15`. Sau khi merge có thêm workflow độc lập [TaskDeck macOS native (Intel + Apple Silicon)](../.github/workflows/taskdeck_macos_native.yml).

Kiểm tra build và test package Go, pinned editor/symlink tests, BSD process-tree parser, binary `--version`, global installer isolated, Docker mock launcher. Test trên Mac thật thêm mở browser/login, PTY tương tác/resize/close, task run, active CWD, Git, Editor Save, file copy/move, SSH/SFTP, Database, Schedule, RBAC của nhiều user, Patch Tool, update/rollback và các file có ACL/xattr. Chạy lại regression Linux trước merge.

**Chưa thể gọi PASS** chỉ vì workflow đã tồn tại; cần kiểm tra log của run và kết quả thử máy thực tế.

## 6. Xử lý lỗi thường gặp

- **`go: command not found` hoặc Go cũ:** cài Go 1.25+, kiểm tra `go version` rồi chạy lại `./install.sh`.
- **`taskdeck: command not found`:** dùng `"$HOME/.local/bin/taskdeck"` hoặc thêm `~/.local/bin` vào `PATH` của shell.
- **`tasks.json` không tồn tại:** chấp nhận gợi ý tạo task trong CLI interactive, hoặc dùng `--init-tasks` khi thực sự muốn tạo.
- **Không mở trang HTTPS:** kiểm tra daemon/port và `--no-browser`; với Docker xem `... status` và `... logs`, tránh trùng port khi có nhiều project.
- **Docker báo không mount được project:** bật quyền File Sharing cho thư mục dự án trong Docker Desktop.
- **Không thấy công cụ macOS (`xcodebuild`, `open`, SDK):** bạn đang dùng Docker Linux; chọn native hoặc cấu hình CI/workflow chạy trên host Mac riêng.
- **Không đọc đúng CWD sau khi `cd`:** trên native Mac, kiểm tra `lsof`; nếu không lấy được sẽ fallback về thư mục ban đầu.
- **Quyền/connection:** kiểm tra `settings.write`, quyền Schedule, SSH key/credential và cấu hình HTTPS; không bypass backend RBAC chỉ để sửa UI.

---

## English — native and Docker Desktop quick guide

**Native status:** Intel (`amd64`) and Apple Silicon (`arm64`) source is integrated into `main`, but **native CI and real-Mac acceptance are still pending**. It is a Go CLI and browser workspace, not a signed/notarized macOS `.app`.

**Requirements:** Git, Bash, curl/wget and tar on the Mac; `install.sh` automatically provisions Go 1.27.3 when needed, verifying the official SHA256 without sudo. Python 3.10+ is required for full Python Patch Tool features. Native tasks execute host macOS commands. From a checkout of `main`:

```bash
git clone https://github.com/hacrot3000/PatchAndCollectionToolForAI.git
cd PatchAndCollectionToolForAI
git switch main
./install.sh

cd /Users/you/projects/my-project
"$HOME/.local/bin/taskdeck" --workspace "$PWD"
```

For Docker Desktop, install and start Docker, share your project folder, and run from the TaskDeck repository root:

```bash
./deploy/macos/taskdeck.sh doctor /Users/you/projects/my-project
./deploy/macos/taskdeck.sh start  /Users/you/projects/my-project
./deploy/macos/taskdeck.sh status /Users/you/projects/my-project
./deploy/macos/taskdeck.sh stop   /Users/you/projects/my-project
```

Open `https://localhost:42882`, check the generated workspace credential file, verify the self-signed certificate, and keep host exposure to loopback. The Docker launcher isolates Compose projects by workspace and preserves home volumes on stop; use a separate `TASKDECK_PORT` for concurrent projects. Docker executes **Linux**, not host macOS commands, and has no automatic Keychain/USB access.

**Limitations:** macOS atomic editor preservation of all xattrs/ACLs/resource forks, PTY/browser interaction, and broker-preserving update/reload **have not been fully verified**. Test before editing metadata-sensitive files or scheduling unattended actions. The Vietnamese matrix, acceptance checklist and troubleshooting sections above provide the full technical constraints. See [platform support](PLATFORM_SUPPORT.md), [feature guide](FEATURES.md) and [handoff](../MACOS_PORTABILITY_HANDOFF.md).
