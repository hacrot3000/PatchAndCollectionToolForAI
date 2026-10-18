# TaskDeck trên macOS / TaskDeck on macOS

**Trạng thái:** triển khai native CLI trên macOS Intel (`amd64`) và Apple Silicon (`arm64`) ở nhánh macOS. Chỉ công bố native verified sau khi workflow [TaskDeck macOS native](../.github/workflows/taskdeck_macos_native.yml) PASS trên cả hai runner và smoke trên máy thật.

## 1. Cài native (khuyến nghị)

Yêu cầu: macOS hỗ trợ Go **1.25+**, Bash 3.2+ (macOS có sẵn), `git`, `curl` hoặc `wget`; Python **3.10+** để dùng đầy đủ Python Patch add-on. Phần web của TaskDeck mở trong Firefox/Safari/Chrome, không cần build ứng dụng `.app`.

Cài Go từ [go.dev/dl](https://go.dev/dl/) hoặc Homebrew, và Command Line Tools nếu hệ thống thiếu Git. Kiểm tra:

```bash
go version
git --version
python3 --version
```

Cài từ checkout repository (không chép source vào workspace đích):

```bash
git clone https://github.com/hacrot3000/PatchAndCollectionToolForAI.git
cd PatchAndCollectionToolForAI
./install.sh
```

Sau khi nhánh đã được merge vào main, lệnh trên cài bản `main` mới nhất. Khi kiểm thử nhánh trước merge, dùng `git switch feature/macos-native-support` trước khi chạy installer.

Mở terminal trong **thư mục project cần thao tác** rồi chạy:

```bash
cd /Users/you/projects/your-project
"$HOME/.local/bin/taskdeck" --workspace "$PWD"
```

Nếu `~/.local/bin` đã nằm trong PATH, dùng `taskdeck --workspace "$PWD"`. TaskDeck sẽ mở trình duyệt theo cơ chế `open` chuẩn macOS, hoặc dùng `--no-browser` nếu chạy không có UI. Cài đặt lưu release riêng theo revision dưới `~/.local/lib/taskdeck/releases`, symlink `current`, và Python Patch runtime cùng version. Không cần `sudo`.

Để update sau khi có release mới, chạy `./install.sh` từ checkout mới (hoặc bootstrap bằng root launcher `./vscode_tasks_menu`). Trên native host dùng self-update của TaskDeck khi phù hợp; không dùng self-update bên trong container.

## 2. Docker Desktop thay thế

Nếu không muốn cài Go hoặc máy không đáp ứng yêu cầu native, TaskDeck chạy backend Linux qua Docker Desktop trên cả Intel và Apple Silicon (container dùng kiến trúc phù hợp). Từ root repository:

```bash
cp deploy/docker/.env.example deploy/docker/.env
```

Sửa `deploy/docker/.env` thành, ví dụ:

```dotenv
TASKDECK_WORKSPACE=/Users/you/projects/your-project
TASKDECK_PORT=42882
TASKDECK_PROFILE=runtime
TASKDECK_IMAGE=taskdeck:local
```

Sau đó:

```bash
docker compose --env-file deploy/docker/.env -f deploy/docker/compose.yaml up -d --build
docker compose --env-file deploy/docker/.env -f deploy/docker/compose.yaml logs -f
```

Mở `https://localhost:42882`. Container tự khởi tạo `.vscode/tasks.json` và cấu hình HTTPS + authentication khi chưa có, không ghi đè cấu hình tồn tại. Chứng chỉ tự ký cần được kiểm tra fingerprint. Khi dừng, dùng `docker compose ... down`, **không dùng `down -v`** nếu muốn giữ named volume. Container dùng lệnh Linux, **không** chạy binary/lệnh macOS native hoặc truy cập macOS Keychain/USB mặc định. Đảm bảo Docker Desktop đã cho phép chia sẻ thư mục project; không expose cổng ra mạng công cộng.

## 3. Khác biệt macOS cần lưu ý

- Terminal/Tasks native dùng PTY Darwin; `/bin/bash` có sẵn, shell mặc định của người dùng có thể là `zsh`. Những task viết lệnh GNU-only cần cài GNU tools riêng, hoặc chỉnh cho BSD tools.
- Process Tree dùng BSD `ps`; tìm CWD shell qua `lsof` thay cho Linux `/proc`. Không dựa vào `/proc` trên macOS.
- Editor native yêu cầu Go 1.25+ để dùng `os.Root` khi ghi file an toàn trước các đổi symlink. Đường ghi atomic bảo toàn mode/ownership thông thường. **Chưa có bằng chứng parity việc giữ mọi ACL, extended attributes và resource forks như bản Linux; cần sao lưu và kiểm tra các file có metadata macOS đặc biệt trước khi sửa.** Không dùng editor trên file nhạy cảm về metadata trước khi kiểm thử.
- Các tính năng cần lệnh tùy chọn như `gopls`, `clangd`, `python`, Node.js hay các database CLI chỉ có khi người dùng đã cài các executable tương ứng. SSH và Git dùng toolchain host.
- Daemon reload-preserving hiện Linux-specific; không khẳng định tính năng này tương đương trên macOS. Sau thay đổi cấu hình có thể cần restart.
- Việc dùng `self-install-and-update.sh` cho Python Patch standalone trên macOS cần kiểm tra riêng vì script legacy còn có tùy chọn GNU; đường cài chính của TaskDeck là `install.sh`.

## 4. Kiểm thử / Acceptance

Trước khi merge: GitHub Actions → workflow **VSCode Tasks Menu Go** (đã có trên main) → Run workflow → chọn `feature/macos-native-support`; job `macos-native` sẽ chạy trên cả Intel và Apple Silicon. Sau khi merge: cũng có thể chạy workflow chuyên dụng **TaskDeck macOS native (Intel + Apple Silicon)**. Hai runner phải PASS các bước: Go compile từng package/test, pinned-file security, BSD process tree, native binary và global installer.

Trên macOS thật cần thử thêm: mở dự án; HTTPS/login; local PTY gõ lệnh và thay đổi CWD; resize/close terminal; tasks; Git Quick Actions; editor create/save/rename; SSH/SFTP; Patch Tool (có Python 3.10+); logout/login đa user; update/rollback. CI compilation không thể thay thế kiểm thử tương tác với trình duyệt.

---

## English quick start

TaskDeck has a native macOS CLI/web backend for Intel and Apple Silicon in the macOS feature branch. Install **Go 1.25+**; Python 3.10+ is recommended for the Python Patch add-on. From a checked-out repository run `./install.sh`; then open a project folder and run `~/.local/bin/taskdeck --workspace "$PWD"`. Installation is per-user and requires no `sudo`. Native status remains **pending macOS runner and hardware acceptance tests**.

Alternatively, run the existing Linux container under Docker Desktop on macOS. Set `TASKDECK_WORKSPACE` to an absolute `/Users/...` directory in `deploy/docker/.env`, then use `docker compose --env-file deploy/docker/.env -f deploy/docker/compose.yaml up -d --build` and open `https://localhost:42882`. Docker uses Linux commands, not host-native macOS commands.

The native editor guards against symlink traversal using Go's `os.Root`; preserving every macOS-specific extended attribute / ACL / resource fork is **not yet verified**. Check these files before editing. See the Vietnamese sections above for runtime differences, security, and the full acceptance checklist.
