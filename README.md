# PatchAndCollectionToolForAI

**Tiếng Việt** · [English](#english)

PatchAndCollectionToolForAI là bộ công cụ chạy cục bộ để **áp dụng PATCH do AI chuẩn bị** và **thu thập bằng chứng/source/log có kiểm soát để gửi lại cho AI**. Thiết kế ưu tiên khả năng phục hồi, chẩn đoán rõ ràng và không âm thầm loại bỏ các capability đã có.

## Chức năng chính

- Hàng đợi tương tác cho **PATCH** và **COLLECT**, có validate/preflight, preview/inspect, batch policy, HISTORY và failed queue.
- PATCH có kiểm tra source drift, rollback/recovery, FAIL_HANDOFF và các artifact chẩn đoán để gửi lại cho AI khi không thể áp dụng an toàn.
- COLLECT chỉ đọc để tìm kiếm/đóng gói source, log và evidence theo manifest; có giới hạn tài nguyên và báo `INCOMPLETE` khi coverage không đầy đủ.
- Truy vấn database theo profile ở chế độ **SELECT-only**; không cung cấp đường raw SQL mutation.
- Git cơ bản qua **allowlist cố định** để xem status/branch/log/show/diff; `switch` chỉ cho local branch đã tồn tại và worktree sạch. Tool không cung cấp raw Git command và cấm các mutation như add/commit/merge/rebase/reset/push/pull/cherry-pick/checkout.
- `manual_execution` chỉ **hướng dẫn người dùng chạy command ở terminal khác** rồi thu evidence/log theo từng bước. Patch Tool không tự chạy command đó.
- Result/FAIL_HANDOFF/COLLECT có ZIP và clear-text TXT companion để dễ upload cho AI.
- Tool Health, checksum/package integrity, capability ledger và regression suite giúp phát hiện cài đặt lỗi hoặc regression trước khi phát hành.
- Launcher Linux/WSL và Windows (`.sh`, `.ps1`, `.bat`).

Tài liệu chi tiết nằm trong `_patch_lib/docs/`. Hướng dẫn tiếng Việt đầy đủ hơn: `HUONG_DAN_PYTHON_PATCH_TOOL.html` và `PYTHON_PATCH_TOOL_FEATURES_VI.md`.

## Cài đặt / cập nhật

### Khuyến nghị — TaskDeck global + Patch add-on

Đường dùng chính hiện tại là **TaskDeck global**. Installer cài binary TaskDeck và đúng revision Python Patch Tool cùng một release versioned, nên web UI, `taskdeck patch` và runtime Python không bị lệch phiên bản.

Từ root của repository này:

```bash
./install.sh
```

Mặc định cài vào:

```text
~/.local/bin/taskdeck
~/.local/lib/taskdeck/current -> releases/<revision>/
~/.local/lib/taskdeck/current/patchtool/
```

Sau đó, từ **project cần thao tác**:

```bash
cd PROJECT_ROOT
taskdeck              # mở TaskDeck web UI; chọn panel Patch
taskdeck patch        # chạy hàng đợi Patch Tool trong terminal
taskdeck patch plan   # xem plan
taskdeck patch report # xem history/report
```

Panel **Patch** là đường web chính cho Queue/Failed, Running/progress/artifacts, Resume, History, Plan và Health. PTY/terminal vẫn được giữ làm evidence/fallback; `taskdeck patch ...` vẫn là giao diện CLI được hỗ trợ chính thức.

Dữ liệu project vẫn nằm trong project (`patchs/`, `artifacts/patch_tool/`, `artifacts/ptv_to_ai/`, `.python_patch_tool.json`). Runtime Patch Tool không cần nằm trong `PROJECT_ROOT/tools/` sau khi migrate. Launcher legacy đã xác minh có thể được TaskDeck thay bằng shim tương thích chuyển tiếp sang `taskdeck patch`.

Các cách portable bên dưới vẫn được giữ để tương thích hoặc dùng độc lập khi chưa muốn cài TaskDeck global.

### Cách 1 — Clone repository

Yêu cầu: Bash, Git và các tiện ích chuẩn `readlink`, `mktemp`, `cp`, `cmp`.

Nên clone repository trực tiếp thành thư mục `tools/` của project, vì launcher xác định **project root là thư mục cha của thư mục chứa launcher**:

```bash
cd PROJECT_ROOT
git clone https://github.com/hacrot3000/PatchAndCollectionToolForAI.git tools
cd tools
chmod +x self-install-and-update.sh run_python_patches.sh
./self-install-and-update.sh
```

Khi `self-install-and-update.sh` nằm ngay tại root của đúng repository này, remote `origin` đúng và đang ở branch `main`, script dùng:

```bash
git pull --ff-only origin main
```

Script không tự chuyển branch. Nếu repo đang ở branch khác hoặc detached HEAD, script sẽ dừng và yêu cầu bạn chuyển về `main` trước.

Các lần cập nhật sau chỉ cần:

```bash
./self-install-and-update.sh
```

### Cách 2 — Cài Patch Tool vào thư mục `tools/` của project khác

Đặt **chính file** `self-install-and-update.sh` vào thư mục muốn cài Patch Tool, ví dụ:

```text
/my-project/
├── tools/
│   └── self-install-and-update.sh
└── ...
```

Có thể tải file mà không pipe trực tiếp vào shell:

```bash
mkdir -p /my-project/tools
curl -fL \
  -o /my-project/tools/self-install-and-update.sh \
  https://raw.githubusercontent.com/hacrot3000/PatchAndCollectionToolForAI/main/self-install-and-update.sh
chmod +x /my-project/tools/self-install-and-update.sh
/my-project/tools/self-install-and-update.sh
```

Bạn có thể gọi script từ bất kỳ working directory nào. **Target luôn là thư mục chứa script, không phải `pwd`.**

Ở chế độ portable, script clone branch `main` vào thư mục tạm, cập nhật `_patch_lib` và các tracked public files vào target rồi verify nội dung. Nếu target đang nằm trong một Git repository khác, script **không** chạy `pull`, `reset`, `checkout` hay thay đổi branch của repository project đó.

> `self-install-and-update.sh` dùng Bash và `readlink -f`; Linux/WSL là môi trường khuyến nghị. Trên Windows có thể dùng WSL/Git Bash tương thích để cài/update, sau đó chạy launcher Windows nếu cần.

## Chạy tool

Cách khuyến nghị sau khi cài TaskDeck global:

```bash
cd PROJECT_ROOT
taskdeck patch
```

Hoặc mở `taskdeck` và dùng panel **Patch** để thao tác native trên web. Các lệnh/flag nâng cao chưa có UI vẫn chạy qua `taskdeck patch ...`.

Đường launcher project-local bên dưới là compatibility path và vẫn được giữ:

Nếu Patch Tool được cài vào `PROJECT_ROOT/tools/`:

```bash
cd PROJECT_ROOT
./tools/run_python_patches.sh
```

Windows:

```text
tools\run_python_patches.bat
```

hoặc PowerShell:

```powershell
.\tools\run_python_patches.ps1
```

PATCH/COLLECT package được đặt trong `patchs/` của project và runner sẽ tự phân loại theo manifest/contract hiện có.

## VS Code Tasks Menu (`vscode_tasks_menu`)

Repository cũng cung cấp `vscode_tasks_menu`: giao diện web/terminal cho `<workspace>/.vscode/tasks.json`, dùng PTY thật để chạy các task tương tác mà không cần mở từng terminal thủ công.

Nếu repository được cài thành `PROJECT_ROOT/tools/`, chạy từ **project root**:

```bash
cd PROJECT_ROOT
./tools/vscode_tasks_menu
```

Launcher luôn dùng **working directory hiện tại làm workspace**, tự build backend Go khi cần, khởi động daemon và mở web UI. Có thể dùng terminal UI native Go bằng:

```bash
./tools/vscode_tasks_menu --terminal
```

Các chức năng chính gồm:

- mỗi task chạy trong PTY/session/tab riêng, hỗ trợ ANSI, prompt tương tác, Ctrl+C và reconnect/replay scrollback;
- terminal tab, đổi tên tab, sắp xếp/khôi phục tab, split dọc/ngang, nhiều split group, active tab và CWD được lưu theo project;
- Broadcast Groups: click phải tab để gán/tạo/xóa khỏi group; menu `Broadcast` hỗ trợ `None / All / Group`, gửi raw key/Enter/Ctrl+C/arrow tới các session đích và tô màu tab theo preset group;
- Preset command cho terminal tab: click phải `Preset command ▶` để chọn chuỗi lệnh project-local; lệnh chạy tuần tự trong chính shell terminal, dừng ngay khi command trả exit code khác 0; dialog riêng hỗ trợ Add/Edit/Delete/Close và lưu tại `vscode_tasks_menu.presets.json`;
- reload browser vẫn khôi phục terminal layout; self-update giữ session broker sống qua daemon replacement nên process/PTY/session ID/scrollback tiếp tục tồn tại, đồng thời tabs, CWD, order và split layout được attach lại;
- Console menu có copy/search/save/clear log; clear yêu cầu xác nhận;
- phát hiện file path trong output để download, có thể bật/tắt theo từng tab và clear/ignore danh sách detect;
- upload file vào workspace với kiểm tra traversal/symlink và xác nhận overwrite;
- Git Quick Actions có kiểm soát, Favorites/Recent/History, task search, notification, theme/font và các tiện ích UI khác;
- hỗ trợ local/remote access qua cấu hình `vscode_tasks_menu.ini`, với auth/TLS và các kiểm tra an toàn khi bind ra ngoài loopback.

Quản lý daemon:

```bash
./tools/vscode_tasks_menu --status
./tools/vscode_tasks_menu --stop-daemon
./tools/vscode_tasks_menu --restart-daemon
```

### Self-update

Sau khi đã bootstrap phiên bản có hỗ trợ self-update, có thể tự kiểm tra branch `main` của public repository, tải source mới, test/build/validate và thay binary hiện tại bằng:

```bash
./tools/vscode_tasks_menu --self-update
```

Nếu daemon đang chạy, web UI sẽ yêu cầu xác nhận trước khi update. Các task/terminal mới được tạo bởi phiên bản có session broker thuộc một broker process độc lập với web daemon; updater ưu tiên giữ broker sống, thay daemon rồi reconnect lại **cùng session ID**, vì vậy process/PTY và scrollback tiếp tục chạy trong lúc update. Listener/port cũng được giữ khi handoff cho phép; nếu URL thay đổi, browser sẽ redirect sang URL mới. Riêng lần nâng chuyển tiếp đầu tiên từ một bản **pre-broker**, các process đã được daemon cũ sở hữu không thể chuyển ownership giữa chừng và có thể mất trong lần update đó. Xem `vscode_tasks_menu_go/SELF_UPDATE.md` để biết fallback/last-resort behavior.

Tài liệu chi tiết:

- `vscode_tasks_menu_go/README.md` — kiến trúc, web/terminal mode, session, download/upload, remote access và CI.
- `vscode_tasks_menu_go/SELF_UPDATE.md` — quy trình self-update, listener handoff, restore session và fallback.

## Nguyên tắc an toàn quan trọng

PatchAndCollectionToolForAI không coi manifest là shell script. Các capability nguy hiểm được giới hạn theo schema/allowlist, Git mutation bị cấm theo policy hiện tại, manual command luôn do người vận hành tự chạy, và lỗi/thiếu evidence phải được thể hiện rõ thay vì báo PASS giả.

---

<a id="english"></a>

## English

PatchAndCollectionToolForAI is a local toolset for **applying AI-prepared PATCH packages** and **collecting controlled source/log/evidence packages to send back to AI**. Its design prioritizes recovery, explicit diagnostics, and preservation of existing capabilities without silent removal.

### Main capabilities

- Interactive **PATCH/COLLECT** queue with validation/preflight, preview/inspect, batch policy, HISTORY, and a persistent failed queue.
- PATCH source-drift checks, rollback/recovery, FAIL_HANDOFF, and diagnostic artifacts when a patch cannot be applied safely.
- Read-only COLLECT actions for searching and packaging source/log/evidence with resource bounds and explicit `INCOMPLETE` coverage reporting.
- Profile-based **SELECT-only** database collection; no raw SQL mutation path.
- Basic Git inspection through a **fixed allowlist** for status/branch/log/show/diff; `switch` is restricted to an existing local branch with a clean worktree. Raw Git commands are not exposed, and mutations such as add/commit/merge/rebase/reset/push/pull/cherry-pick/checkout are forbidden.
- `manual_execution` only **instructs the operator to run commands in another terminal** and then verifies step-by-step evidence/logs. Patch Tool does not execute those commands itself.
- ZIP plus clear-text TXT companions for COLLECT/result/FAIL_HANDOFF artifacts so they are easy to upload to AI.
- Tool Health, package/checksum integrity, capability ledger, and regression gates to detect broken installs or behavioral regressions.
- Linux/WSL and Windows launchers (`.sh`, `.ps1`, `.bat`).

Detailed contracts are under `_patch_lib/docs/`.

### Install / update

#### Recommended — global TaskDeck + Patch add-on

The primary path is now the **global TaskDeck** installation. The installer ships the TaskDeck binary and the matching Python Patch Tool runtime in the same versioned release, preventing web/CLI/runtime revision skew.

From this repository root:

```bash
./install.sh
```

Default layout:

```text
~/.local/bin/taskdeck
~/.local/lib/taskdeck/current -> releases/<revision>/
~/.local/lib/taskdeck/current/patchtool/
```

Then, from the project you want to operate on:

```bash
cd PROJECT_ROOT
taskdeck              # open TaskDeck web UI; select the Patch panel
taskdeck patch        # run the Patch Tool queue in a terminal
taskdeck patch plan   # inspect the plan
taskdeck patch report # inspect history/report
```

The **Patch** panel is the primary web path for Queue/Failed, Running/progress/artifacts, Resume, History, Plan, and Health. PTY/terminal remains an explicit evidence/fallback surface, and `taskdeck patch ...` remains a supported first-class CLI.

Project data stays project-local (`patchs/`, `artifacts/patch_tool/`, `artifacts/ptv_to_ai/`, and `.python_patch_tool.json`). The Patch Tool runtime no longer needs to live under `PROJECT_ROOT/tools/` after migration. A verified legacy launcher may be replaced by a compatibility shim that forwards to `taskdeck patch`.

The portable installation methods below remain supported for compatibility or standalone use.

#### Option 1 — Clone the repository

Requirements: Bash, Git, and standard `readlink`, `mktemp`, `cp`, and `cmp` utilities.

Clone the repository directly as the project's `tools/` directory. The launcher treats **the parent directory of its own directory as the project root**:

```bash
cd PROJECT_ROOT
git clone https://github.com/hacrot3000/PatchAndCollectionToolForAI.git tools
cd tools
chmod +x self-install-and-update.sh run_python_patches.sh
./self-install-and-update.sh
```

When `self-install-and-update.sh` is located at the root of this exact repository, `origin` matches this project, and the current branch is `main`, it updates with:

```bash
git pull --ff-only origin main
```

The script never switches branches automatically. A different branch or detached HEAD is rejected until you switch back to `main`.

For later updates:

```bash
./self-install-and-update.sh
```

#### Option 2 — Install into another project's `tools/` directory

Place **the `self-install-and-update.sh` file itself** in the directory where Patch Tool should be installed, for example:

```text
/my-project/
├── tools/
│   └── self-install-and-update.sh
└── ...
```

You can download it without piping remote content directly into a shell:

```bash
mkdir -p /my-project/tools
curl -fL \
  -o /my-project/tools/self-install-and-update.sh \
  https://raw.githubusercontent.com/hacrot3000/PatchAndCollectionToolForAI/main/self-install-and-update.sh
chmod +x /my-project/tools/self-install-and-update.sh
/my-project/tools/self-install-and-update.sh
```

The script may be invoked from any working directory. **Its target is always the directory containing the script, never `pwd`.**

In portable mode it clones `main` into a temporary directory, replaces `_patch_lib`, updates tracked public files in the target, and verifies the result. If the target lives inside a different Git repository, the installer does **not** pull, reset, checkout, or switch branches in that project repository.

> `self-install-and-update.sh` uses Bash and `readlink -f`; Linux/WSL is the recommended environment. On Windows, use a compatible WSL/Git Bash environment for installation/update, then use the Windows launcher if desired.

### Run

Recommended after installing global TaskDeck:

```bash
cd PROJECT_ROOT
taskdeck patch
```

Or launch `taskdeck` and use the native **Patch** panel. Advanced commands/flags that intentionally remain CLI-only can still be invoked through `taskdeck patch ...`.

The project-local launcher below remains a compatibility path:

If installed under `PROJECT_ROOT/tools/`:

```bash
cd PROJECT_ROOT
./tools/run_python_patches.sh
```

Windows:

```text
tools\run_python_patches.bat
```

or PowerShell:

```powershell
.\tools\run_python_patches.ps1
```

Place PATCH/COLLECT packages in the project's `patchs/` directory; the runner classifies them according to the existing manifest/contracts.

### VS Code Tasks Menu (`vscode_tasks_menu`)

The repository also provides `vscode_tasks_menu`, a web/terminal interface for `<workspace>/.vscode/tasks.json`. It runs interactive tasks in real PTYs so users do not need to open and manage separate terminals manually.

If the repository is installed as `PROJECT_ROOT/tools/`, run it from the **project root**:

```bash
cd PROJECT_ROOT
./tools/vscode_tasks_menu
```

The launcher always uses the **current working directory as the workspace**, builds the Go backend when necessary, starts the daemon, and opens the web UI. A native Go terminal UI is also available:

```bash
./tools/vscode_tasks_menu --terminal
```

Main capabilities include:

- one PTY/session/tab per task, with ANSI output, interactive prompts, Ctrl+C, reconnect, and scrollback replay;
- terminal tabs, tab renaming, tab ordering/restoration, vertical and horizontal splits, multiple split groups, active-tab state, and per-terminal CWD persistence;
- Broadcast Groups: right-click a tab to assign/create/remove group membership; the `Broadcast` menu provides `None / All / Group`, fans out raw key/Enter/Ctrl+C/arrow input to matching sessions, and colors grouped tabs using readable presets;
- Terminal-only Preset command: right-click `Preset command ▶` to run a project-local command sequence in the current shell; commands execute sequentially and stop immediately on a non-zero exit code, with Add/Edit/Delete/Close management persisted in `vscode_tasks_menu.presets.json`;
- browser reload restores terminal layout; self-update keeps the independent session broker alive across web-daemon replacement so process/PTY/session IDs and scrollback survive while tabs, CWDs, ordering, and split layout are reattached;
- Console actions for copy/search/save/clear log, with confirmation before clearing;
- file-path detection in terminal output for downloads, with per-tab enable/disable and clear/ignore controls;
- workspace uploads with traversal/symlink protection and overwrite confirmation;
- controlled Git Quick Actions, Favorites/Recent/History, task search, notifications, theme/font controls, and other browser UI helpers;
- local or remote access through `vscode_tasks_menu.ini`, including auth/TLS and safety checks before binding outside loopback.

Daemon management:

```bash
./tools/vscode_tasks_menu --status
./tools/vscode_tasks_menu --stop-daemon
./tools/vscode_tasks_menu --restart-daemon
```

#### Self-update

After bootstrapping a version that supports self-update, the tool can check the public repository's `main` branch, download the new source, test/build/validate it, and atomically replace the current binary with:

```bash
./tools/vscode_tasks_menu --self-update
```

If a daemon is running, the web UI asks for confirmation before updating. Tasks/terminals created by a broker-enabled build are owned by a session-broker process that is independent of the web daemon. The updater prefers to keep that broker alive, replace the daemon, and reconnect to the **same session IDs**, so running processes, PTYs, and scrollback continue through the update. The listener/port is also preserved when handoff is available; if the URL changes, the browser redirects to the new URL. The first transition from a **pre-broker** build is the exception: processes already owned by the old daemon cannot have their PTY ownership migrated mid-run and may be lost during that one upgrade. See `vscode_tasks_menu_go/SELF_UPDATE.md` for fallback/last-resort behavior.

Detailed documentation:

- `vscode_tasks_menu_go/README.md` — architecture, web/terminal modes, sessions, download/upload, remote access, and CI.
- `vscode_tasks_menu_go/SELF_UPDATE.md` — self-update flow, listener handoff, session restoration, and fallback behavior.

### Safety model

PatchAndCollectionToolForAI does not treat manifests as shell scripts. Sensitive capabilities are constrained by schemas/allowlists, Git mutation is forbidden by the current policy, manual commands remain operator-executed, and missing/failed evidence must be reported explicitly rather than converted into a false PASS.
