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

### Safety model

PatchAndCollectionToolForAI does not treat manifests as shell scripts. Sensitive capabilities are constrained by schemas/allowlists, Git mutation is forbidden by the current policy, manual commands remain operator-executed, and missing/failed evidence must be reported explicitly rather than converted into a false PASS.
