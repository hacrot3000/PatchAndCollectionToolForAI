# PatchAndCollectionToolForAI

**Tiếng Việt** · [English](#english)

PatchAndCollectionToolForAI gồm **Python Patch Tool** (áp dụng PATCH do AI chuẩn bị, COLLECT source/log/evidence có kiểm soát) và **TaskDeck** (workspace web cho terminal/PTY, Git, editor/diff, SSH/SFTP/FTP, database, task và quản lý project). Hai thành phần có chính sách quyền riêng; thiết kế ưu tiên khả năng phục hồi, chẩn đoán rõ ràng và không âm thầm loại bỏ các capability đã có.

## Chức năng chính

- Hàng đợi tương tác cho **PATCH** và **COLLECT**, có validate/preflight, preview/inspect, batch policy, HISTORY và failed queue.
- PATCH có kiểm tra source drift, rollback/recovery, FAIL_HANDOFF và các artifact chẩn đoán để gửi lại cho AI khi không thể áp dụng an toàn.
- COLLECT chỉ đọc để tìm kiếm/đóng gói source, log và evidence theo manifest; có giới hạn tài nguyên và báo `INCOMPLETE` khi coverage không đầy đủ.
- **Python Patch Tool** thu thập dữ liệu database theo profile với truy vấn **SELECT-only**; đây là giới hạn của capability COLLECT, không phải tuyên bố rằng toàn bộ panel Database trong TaskDeck chỉ đọc.
- **Python Patch Tool** có Git inspection theo allowlist, không tự thực hiện Git mutation qua manifest. **TaskDeck Git Quick Actions** là module riêng, có quyền/kiểm tra phù hợp và hỗ trợ stage/commit, fetch/pull/push, merge, rebase, branch, stash, log, compare và recovery. Terminal cho phép chạy lệnh Git do người dùng chủ động nhập.
- `manual_execution` chỉ **hướng dẫn người dùng chạy command ở terminal khác** rồi thu evidence/log theo từng bước. Patch Tool không tự chạy command đó.
- Result/FAIL_HANDOFF/COLLECT có ZIP và clear-text TXT companion để dễ upload cho AI.
- **Project identity tự động:** TaskDeck bổ sung `project.key` (tên thư mục) và `project.id` (UUID duy nhất, giữ ổn định) vào `.python_patch_tool.json` cho cả project mới lẫn project cũ. AI Pack và handoff/failed của PATCH/COLLECT bắt buộc cung cấp identity; AI phải dừng và hỏi xác nhận khi key/UUID thiếu hoặc không khớp, tránh lẫn source giữa nhiều task/project.
- Tool Health, checksum/package integrity, capability ledger và regression suite giúp phát hiện cài đặt lỗi hoặc regression trước khi phát hành.
- TaskDeck chạy Linux native, Docker Desktop trên Windows (Linux image) hoặc WSL2; có **Windows GUI Launcher `.exe`** và script PowerShell/CMD, cùng Windows-native `.ps1`/`.bat` riêng cho Python Patch Tool.

Tài liệu chi tiết nằm trong `_patch_lib/docs/`. Hướng dẫn tiếng Việt đầy đủ hơn: `HUONG_DAN_PYTHON_PATCH_TOOL.html` và `PYTHON_PATCH_TOOL_FEATURES_VI.md`.

## Chạy TaskDeck trên Windows

**Hướng được hỗ trợ:** chạy backend Linux của TaskDeck qua **Docker Desktop (WSL2 engine)** hoặc trực tiếp trong **WSL2**. Project trên Windows được bind-mount vào `/workspace`, không cần copy source vào container. Bản **Windows Launcher `.exe` là ứng dụng GUI quản lý Docker**, không phải backend Go native; ConPTY/Windows IPC vẫn đang trong kế hoạch phát triển.

### Cách nhanh — Windows GUI Launcher

1. Cài và khởi động **Docker Desktop** với Linux/WSL2 engine.
2. Mở [workflow TaskDeck Docker and Windows launchers](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/workflows/taskdeck_windows_docker.yml), chọn **run thành công**, tải artifact `TaskDeckLauncher-win-x64` và giải nén.
3. Chạy `TaskDeckLauncher.exe`, chọn thư mục project (ví dụ `D:\Projects\M3`), chọn profile **runtime** (mặc định) hoặc **dev**, rồi bấm **Start**. Launcher xây dựng image từ Docker build context đi kèm ZIP nếu chưa có image, nên không cần cài Go/.NET SDK hay clone repo chỉ để build.
4. Mở `https://localhost:42882`; lấy thông tin đăng nhập từ file riêng tư `<project>\.vscode\vscode_tasks_menu.ini` được tạo lần đầu. **Không commit file chứa password.**

Nếu đã clone repository và muốn dùng PowerShell/CMD thay GUI:

```powershell
.\deploy\windows\taskdeck.ps1 -Action Doctor -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck.ps1 -Action Start  -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck.ps1 -Action Logs  -ProjectPath 'D:\Projects\M3' -Follow
.\deploy\windows\taskdeck.ps1 -Action Stop  -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck.ps1 -Action Update -ProjectPath 'D:\Projects\M3'
```

`deploy/windows/taskdeck.cmd` cũng chuyển tiếp các tham số trên. Launcher hỗ trợ `Start / Stop / Status / Open / Logs / Build / Update / Doctor / Credentials`, tạo container và home volume **riêng theo project**; lệnh `Update` build lại hoặc pull image theo lựa chọn, giữ dữ liệu project/volume. Không dùng self-update Linux bên trong container để cập nhật image.

### Docker image, công cụ và giới hạn

- **`runtime` (mặc định, khoảng 301 MiB trong CI):** Go backend đã biên dịch, Bash, Git/Git LFS, OpenSSH client (`ssh`, `scp`, `sftp`), Python Patch Tool, `curl`, `wget`, `rsync`, `jq`, `grep`, `sed`, `awk`, `find`, `tar`, `zip`, `unzip` và tiện ích terminal.
- **`dev` (tùy chọn):** thêm compiler GCC/G++, Node.js/npm, gdb và các công cụ build. Dockerfile multi-stage: Go compiler **không nằm trong runtime image**.
- **An toàn:** publish cổng trên **`127.0.0.1` của host**, listener trong container dùng HTTPS + xác thực; khởi tạo mật khẩu mạnh ngẫu nhiên khi chưa có config, mount duy nhất thư mục project đã chọn và giữ home bằng Docker volume. Không nhúng SSH private key vào image.
- **WSL2 không dùng Docker:** cài TaskDeck Linux theo `./install.sh`, hoặc dùng `deploy/windows/taskdeck-wsl.ps1` để chuyển đường dẫn Windows và chạy Linux backend. Với project rất lớn, nên lưu repo bên trong filesystem WSL2 để giảm chi phí I/O.
- **Chưa hỗ trợ backend Go Windows native:** Windows `.exe` hiện là **launcher**. Python Patch Tool riêng có thể chạy native với `.ps1`/`.bat` và đã có kiểm thử Windows; không đồng nghĩa PTY, daemon, broker của TaskDeck Go đã chạy native.

Xem [hướng dẫn Windows đầy đủ](docs/WINDOWS.md), [Docker Compose mẫu](deploy/docker/compose.yaml), [CI Docker/launcher](.github/workflows/taskdeck_windows_docker.yml), [CI kiểm thử Windows native riêng](.github/workflows/taskdeck_windows_native_audit.yml) và [handoff triển khai](WINDOWS_PORTABILITY_HANDOFF.md). Hai workflow này được tách riêng để kiểm thử backend Go native đang thiếu ConPTY không làm sai trạng thái phát hành Docker/WSL2.

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

Panel **Patch** đang được chuyển sang native workspace cho Queue/Failed, Running/progress/artifacts, Resume, History, Plan và Health. Phần code cutover Phase 2 đã được triển khai và CI kiểm tra, nhưng **browser smoke trên bản TaskDeck đã self-update vẫn còn pending**; vì vậy chưa được phép coi tích hợp native UI là hoàn tất. PTY/terminal vẫn được giữ làm evidence/fallback; `taskdeck patch ...` vẫn là giao diện CLI được hỗ trợ chính thức. Trạng thái recovery chính xác nằm trong `TASKDECK_PATCH_HANDOFF.md`.

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

Hoặc mở `taskdeck` và dùng panel **Patch** để kiểm tra luồng native đang được cut over. Cho đến khi Phase 2E.2 browser smoke PASS, không được suy ra rằng toàn bộ native UI đã hoàn tất chỉ vì các renderer/protocol tồn tại. Các lệnh/flag nâng cao chưa có UI vẫn chạy qua `taskdeck patch ...`.

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

PATCH/COLLECT package có thể được đặt trực tiếp trong `patchs/` của project **hoặc upload từ browser bằng nút Upload trong panel Patch**. Browser upload hỗ trợ chọn nhiều package ZIP/TAR/TGZ, publish atomically vào `patchs/`, hỏi xác nhận trước khi overwrite tên trùng và tự refresh Queue; Python Patch runtime vẫn là nguồn quyết định việc nhận diện/validate package trước khi người dùng chọn chạy.

## TaskDeck (`vscode_tasks_menu`)

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
- terminal tab, đổi tên tab, sắp xếp/khôi phục tab, split dọc/ngang và nhiều split group; nút **Terminal** trên header mở terminal mới trực tiếp (không qua dropdown), hotkey **Ctrl+Shift+`** cũng mở terminal mới, còn CWD mặc định cho terminal được chọn tại **Settings → Terminal**; trong layout split có thể kéo title của pane rồi thả vào vùng trái/phải/trên/dưới của pane khác để sắp xếp lại trực quan theo kiểu Terminator; active tab, CWD và split layout được lưu theo project;
- Broadcast Groups: click phải tab để gán/tạo/xóa khỏi group; menu `Broadcast` hỗ trợ `None / All / Group`, gửi raw key/Enter/Ctrl+C/arrow tới các session đích và tô màu tab theo preset group; mỗi group có thể lưu environment riêng và mở terminal mới trực tiếp trong group với environment đó; paste/input nhiều dòng hoặc payload lớn có preview + confirm trước khi fan-out;
- Preset command cho terminal tab: click phải `Preset command ▶` để chọn chuỗi lệnh project-local; lệnh chạy tuần tự trong chính shell terminal, dừng ngay khi command trả exit code khác 0; dialog riêng hỗ trợ Add/Edit/Delete/Close và lưu tại `vscode_tasks_menu.presets.json`;
- Terminal session recording dựa trên OSC 133 lưu command/output/exit code/duration/CWD, hỗ trợ session note, bookmark một dòng output, Rerun và export Markdown/text; history được persist server-side và remap sang session ID mới khi terminal được khôi phục sau restart;
- reload browser vẫn khôi phục terminal layout; self-update giữ session broker sống qua daemon replacement nên process/PTY/session ID/scrollback tiếp tục tồn tại, đồng thời tabs, CWD, order và split layout được attach lại;
- Console menu có copy/search/save/clear log; clear yêu cầu xác nhận;
- phát hiện file path trong output để download, có thể bật/tắt theo từng tab và clear/ignore danh sách detect;
- upload file vào workspace với kiểm tra traversal/symlink và xác nhận overwrite;
- **Git Quick Actions** cho workspace **nhiều repository**: phát hiện repo lồng nhau, chọn repo active hoặc theo terminal CWD; stage/commit, fetch/pull/push, branch, merge/rebase, stash, recovery và Git Graph. Trong **Branches**, các lệnh xóa gom vào một wizard chọn kiểu xóa; **Merge From / Merge To** gom vào wizard chọn hướng merge; bấm vào tên branch để copy. **Compare Branch** dùng giao diện tương tự Folder Compare, drill-down đến từng file.
- **Git Log** có ô tìm kiếm nhanh theo SHA/nội dung/tác giả/ngày ngay trên lịch sử đã tải (không gọi Git mỗi lần nhập), `Load older commits` phân trang theo nhóm 100 commit và **Show Diff**: một file mở File Compare, nhiều file mở Directory Compare chỉ các path thay đổi; merge commit cho chọn parent. Các cách xem diff này chỉ đọc. **Working Tree Diff** đối chiếu `HEAD ↔ Staged`, `Staged ↔ Working`, `HEAD ↔ Working` với syntax highlighting, tô thay đổi theo token, context và phân loại thay đổi comment/code.
- **Smart Terminal** gợi ý lệnh/alias theo shell và executable thực tế trong `PATH`, có hỗ trợ prompt Bash không phát OSC 133; hỗ trợ `\help`, `\ai` và nhóm `\do` khi nhập trực tiếp ở local terminal. Các tính năng như Favorites/Recent, task search, notification, theme/font tiếp tục dùng chung trong workspace;
- panel **Connections** hỗ trợ local terminal, SSH, **SFTP**, **FTP** và database; file-transfer workspace dùng dual-pane kiểu FileZilla với Host/Local ↔ Remote, upload/download folder đệ quy, remote folder cache theo session, Transfer Queue theo từng file (Queued/Running/Done/Failed), path Favorites/Recent và sortable headers; SFTP tái sử dụng system OpenSSH, FTP dùng client Go stdlib, không thêm npm/package ngoài;
- hỗ trợ local/remote access qua cấu hình `vscode_tasks_menu.ini`, với auth/TLS và các kiểm tra an toàn khi bind ra ngoài loopback.

Quản lý daemon:

```bash
./tools/vscode_tasks_menu --status
./tools/vscode_tasks_menu --stop-daemon
./tools/vscode_tasks_menu --restart-daemon
```

### Shared Server / Multi-user

TaskDeck hỗ trợ chế độ **shared server nhiều người dùng** theo mô hình **1 daemon = 1 workspace/project**. Nhiều daemon trên cùng host có thể dùng chung một identity DB để một user global tham gia nhiều project nhưng vẫn giữ quyền, session và dữ liệu vận hành tách biệt theo project.

Có thể bật/tắt chế độ này trực tiếp trong **Settings → Security → Authentication mode…** bằng wizard 3 bước `Mode → Setup → Review`, hoặc cấu hình thủ công `[shared_server] enabled=true` trong `vscode_tasks_menu.ini`. Wizard hỗ trợ migrate hai chiều:

- **Single → Shared:** tạo/reuse identity DB ngoài workspace, provision hoặc xác minh admin, gán `system:admin` cho project, bật HTTPS, tắt Basic Auth và scrub plaintext password legacy khỏi config. Nếu username đã tồn tại trong identity DB dùng chung, password phải xác minh đúng; TaskDeck không tự reset credential global.
- **Shared → Single:** yêu cầu đặt username/password Basic Auth mới vì shared password hash không thể/không được giải mã ngược; toàn bộ browser session của project bị revoke, nhưng identity DB/users/roles vẫn được giữ để có thể chuyển lại Shared sau này.
- Trên nền tảng hỗ trợ `--reload-config`, web daemon được reload sau migration trong khi independent session broker tiếp tục giữ task/terminal đang chạy.

Các capability chính đã triển khai:

- **Shared identity + project membership:** user/password là global identity; role, membership và permission override được quản lý theo project.
- **RBAC fail-closed:** system role `admin / developer / operator / viewer`, custom role theo project, permission riêng cho Tasks, Terminal, Patch, Files, Git, Settings, Self-update, Users, Roles, Sessions và Audit. Backend là nguồn quyết định cuối; UI hide/show chỉ là UX.
- **Project-bound browser sessions:** session lưu `project_id`; token đăng nhập ở project A không dùng được ở project B dù cùng user có membership ở cả hai. Idle timeout 30 phút, absolute lifetime 12 giờ.
- **Terminal privacy/ownership:** tách quyền view/control, hỗ trợ own/all scope và kiểm quyền xuyên suốt WebSocket. Origin lạ bị từ chối trước khi attach.
- **Project-scoped administration:** project admin chỉ xem/sửa users, custom roles, sessions và audit thuộc project của mình; direct-ID cross-project access bị từ chối.
- **Password lifecycle:** đổi password self-service và operator reset dùng chung policy; đổi/reset password revoke toàn bộ login session của global user trên các project.
- **Audit:** ghi authentication, authorization denial, task/terminal/Patch/file/settings/self-update/admin actions và mutation-lock conflict; không ghi password/raw token/keystroke.
- **Shared workspace mutation lock:** chỉ giữ lock khi có mutation thật. Native Patch Queue/Resume ở trạng thái chờ không còn khóa Git/file/self-update; lock `patch.run` chỉ được acquire ngay trước khi chạy PATCH (COLLECT-only không khóa source workspace). Terminal Patch vẫn giữ policy bảo thủ vì input PTY không đi qua structured action API. Admin `project.admin` thấy nút **Unlock** trên badge lock; thao tác này Stop Patch session đang giữ lock rồi mới release, không bypass một PATCH còn chạy.
- **Identity DB backup/restore:** SQLite online backup, integrity check, private backup file, pre-restore safety snapshot và rollback hữu hạn.
- **HTTPS/reverse-proxy hardening:** shared mode yêu cầu TLS thật ở TaskDeck listener; internal self-update handoff/detach cần cả loopback và private daemon control token.
- **Schema migration an toàn:** auth session được project-scope; session legacy không xác định project bị revoke khi migrate thay vì được gán project bằng suy đoán.
- **Broadcast keyboard fan-out** hiện chủ động **tắt trong shared mode** cho đến khi có mô hình ownership/permission riêng phù hợp.

Automated first-release acceptance đã cover hai daemon/Store độc lập dùng chung identity DB, HTTPS login + Secure cookie + WSS authorization, cross-project isolation, migration và security regression. CI chạy Go 1.19.x và Go 1.23.x với staged self-update tests, `go test ./...`, `go vet ./...` và production build. Trước khi ký first shared-server release vẫn cần smoke trên môi trường triển khai thật: browser profiles, reverse proxy HTTPS→HTTPS/WSS, password lifecycle, backup/restore drill và upgrade drill.

Tài liệu shared-server:

- `vscode_tasks_menu_go/MULTI_USER_SHARED_SERVER_PLAN.md` — kiến trúc, quyền, roadmap, schema và trạng thái triển khai.
- `vscode_tasks_menu_go/SHARED_SERVER_DEPLOYMENT.md` — TLS/reverse proxy, identity DB, backup/restore, multi-project deployment.
- `vscode_tasks_menu_go/SHARED_SERVER_ACCEPTANCE.md` — checklist acceptance tự động và smoke test cần chạy trên môi trường thật.

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

**Python Patch Tool** không coi manifest là shell script: các capability nhạy cảm bị giới hạn bởi schema/allowlist, không thực hiện Git mutation qua PATCH/COLLECT manifest, và `manual_execution` chỉ hướng dẫn người vận hành tự chạy command. **TaskDeck Git Quick Actions** có luồng mutation riêng với xác nhận và kiểm tra an toàn; terminal chạy lệnh người dùng nhập. Mọi lỗi/thiếu evidence phải được báo rõ, không biến thành PASS giả.

---

<a id="english"></a>

## English

PatchAndCollectionToolForAI combines **Python Patch Tool** (AI-prepared PATCH packages and controlled source/log/evidence COLLECT operations) with **TaskDeck** (a browser workspace for PTY terminals, Git, editor/diff, SSH/SFTP/FTP, databases, tasks, and project operations). These components enforce separate capability boundaries. The design prioritizes recovery, explicit diagnostics, and preservation of existing functionality.

### Main capabilities

- Interactive **PATCH/COLLECT** queue with validation/preflight, preview/inspect, batch policy, HISTORY, and a persistent failed queue.
- PATCH source-drift checks, rollback/recovery, FAIL_HANDOFF, and diagnostic artifacts when a patch cannot be applied safely.
- Read-only COLLECT actions for searching and packaging source/log/evidence with resource bounds and explicit `INCOMPLETE` coverage reporting.
- **Python Patch Tool** performs profile-based **SELECT-only** database collection; that restriction applies to the COLLECT capability, not necessarily to TaskDeck's separate Database panel.
- **Python Patch Tool** restricts Git inspection to an allowlist and does not execute Git mutations through PATCH/COLLECT manifests. **TaskDeck Git Quick Actions** is a separate permission-checked interface supporting staging/commits, fetch/pull/push, merge, rebase, branches, stash, log, comparison, and recovery. Users may also run explicit Git commands in a terminal.
- `manual_execution` only **instructs the operator to run commands in another terminal** and then verifies step-by-step evidence/logs. Patch Tool does not execute those commands itself.
- ZIP plus clear-text TXT companions for COLLECT/result/FAIL_HANDOFF artifacts so they are easy to upload to AI.
- **Automatic project identity:** TaskDeck migrates new and existing workspaces to persistent `project.key` and random `project.id` (UUID). AI Pack and PATCH/COLLECT handoffs include both. AI must stop and request user confirmation for missing/mismatched identities; a legacy handoff without UUID is unverified.
- Tool Health, package/checksum integrity, capability ledger, and regression gates to detect broken installs or behavioral regressions.
- Linux-native TaskDeck, Docker Desktop on Windows (Linux image), and WSL2, with a **Windows GUI Launcher `.exe`** plus PowerShell/CMD scripts; Python Patch Tool also has separately tested native Windows `.ps1`/`.bat` entry points.

Detailed contracts are under `_patch_lib/docs/`.

### Windows — Docker Desktop, WSL2, and the GUI launcher

**Supported approach:** run the Linux TaskDeck backend in a **Docker Desktop container (WSL2 engine)** or directly under **WSL2**. Bind-mount a Windows project directory to `/workspace`; the project is not copied into the image. **`TaskDeckLauncher.exe` is a Windows GUI that manages a Linux Docker backend, not a native Windows Go/ConPTY build.**

**Quick start:** install and start Docker Desktop, open the [TaskDeck Docker and Windows launchers CI workflow](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/workflows/taskdeck_windows_docker.yml), select a **successful run**, download the `TaskDeckLauncher-win-x64` artifact, unzip it, and launch `TaskDeckLauncher.exe`. Browse to a project such as `D:\Projects\M3`, select `runtime` (default) or `dev`, and click **Start**. The ZIP includes the Docker build context, so cloning the repository or installing Go/.NET SDKs just to run the launcher is unnecessary.

For a repository checkout, the PowerShell/CMD launcher offers the same operations:

```powershell
.\deploy\windows\taskdeck.ps1 -Action Doctor -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck.ps1 -Action Start  -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck.ps1 -Action Logs   -ProjectPath 'D:\Projects\M3' -Follow
.\deploy\windows\taskdeck.ps1 -Action Stop   -ProjectPath 'D:\Projects\M3'
.\deploy\windows\taskdeck.ps1 -Action Update -ProjectPath 'D:\Projects\M3'
```

`deploy/windows/taskdeck.cmd` forwards to the PowerShell launcher. Supported launcher actions include `Start / Stop / Status / Open / Logs / Build / Update / Doctor / Credentials`. Each project has its own named container/home volume. Updating rebuilds or explicitly pulls the image without deleting project data or home volumes; do **not** use the Linux in-container self-updater for Docker images.

- **`runtime` (default, approximately 301 MiB in CI):** precompiled Go backend, Bash, Git/Git LFS, OpenSSH (`ssh`, `scp`, `sftp`), Python Patch Tool, `curl`, `wget`, `rsync`, `jq`, `grep`, `sed`, `awk`, `find`, archive utilities, and common terminal commands. The Go compiler is excluded from the final runtime image.
- **`dev` (opt-in):** adds GCC/G++, Node.js/npm, gdb, and development/build tools.
- **Security:** host binding is restricted to `127.0.0.1`; HTTPS/authentication are enabled for new container workspaces, with a generated strong password stored privately at `<project>\.vscode\vscode_tasks_menu.ini`. **Never commit this credential file.** Mount only the selected project; SSH private keys are not bundled.
- **Without Docker:** install Linux TaskDeck within WSL2 using `./install.sh` or use `deploy/windows/taskdeck-wsl.ps1` to translate a Windows project path. For very large repositories, keeping files on the WSL2 filesystem improves I/O performance.
- **Native Windows status:** the Python Patch Tool has separate tested Windows PowerShell/BAT entry points, but the Go TaskDeck backend still needs ConPTY, Windows process controls, and broker/IPC changes. The launcher EXE does not imply that native Go is supported.

See the [Windows setup guide](docs/WINDOWS.md) (currently Vietnamese), [Docker Compose example](deploy/docker/compose.yaml), [Docker/launcher CI](.github/workflows/taskdeck_windows_docker.yml), [separate native Windows audit](.github/workflows/taskdeck_windows_native_audit.yml), and [implementation handoff](WINDOWS_PORTABILITY_HANDOFF.md). The native audit is intentionally separate from the supported Docker/WSL release checks.

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

The **Patch** panel is being cut over to a native workspace for Queue/Failed, Running/progress/artifacts, Resume, History, Plan, and Health. The corrected Phase 2 code path is implemented and covered by CI, but **installed-runtime browser smoke after self-update is still pending**, so the native UI integration must not yet be treated as complete. PTY/terminal remains an explicit evidence/fallback surface, and `taskdeck patch ...` remains a supported first-class CLI. See `TASKDECK_PATCH_HANDOFF.md` for the current recovery state.

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

Or launch `taskdeck` and exercise the **Patch** panel's native cutover path. Until Phase 2E.2 browser smoke passes, do not infer full native-UI completion merely from the presence of renderers/protocol endpoints. Advanced commands/flags that intentionally remain CLI-only can still be invoked through `taskdeck patch ...`.

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

PATCH/COLLECT packages can be placed in the project's `patchs/` directory **or uploaded from the browser with the Patch panel's Upload action**. Browser upload accepts multiple ZIP/TAR/TGZ packages, publishes them atomically into `patchs/`, confirms before overwriting a duplicate name, and refreshes Queue automatically; the Python Patch runtime remains authoritative for package classification/validation before the user explicitly selects execution.

### TaskDeck (`vscode_tasks_menu`)

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
- terminal tabs, tab renaming, tab ordering/restoration, vertical and horizontal splits, and multiple split groups; the header **Terminal** action opens a new terminal immediately (no dropdown), **Ctrl+Shift+`** opens a new terminal from anywhere in the UI, and the default terminal CWD is selected under **Settings → Terminal**; within a split layout, drag a pane title and drop it on the left/right/top/bottom region of another pane for Terminator-style visual rearrangement; active-tab state, per-terminal CWD, and split layout persist per project;
- Broadcast Groups: right-click a tab to assign/create/remove group membership; the `Broadcast` menu provides `None / All / Group`, fans out raw key/Enter/Ctrl+C/arrow input to matching sessions, and colors grouped tabs using readable presets;
- Terminal-only Preset command: right-click `Preset command ▶` to run a project-local command sequence in the current shell; commands execute sequentially and stop immediately on a non-zero exit code, with Add/Edit/Delete/Close management persisted in `vscode_tasks_menu.presets.json`;
- browser reload restores terminal layout; self-update keeps the independent session broker alive across web-daemon replacement so process/PTY/session IDs and scrollback survive while tabs, CWDs, ordering, and split layout are reattached;
- Console actions for copy/search/save/clear log, with confirmation before clearing;
- file-path detection in terminal output for downloads, with per-tab enable/disable and clear/ignore controls;
- workspace uploads with traversal/symlink protection and overwrite confirmation;
- **Git Quick Actions** for a **multi-repository workspace**: discover nested repos, select the active repository or follow the terminal CWD, and perform authorized stage/commit, fetch/pull/push, branch, merge/rebase, stash, recovery, and Git Graph operations. **Branches** consolidates deletion choices in a wizard and **Merge From / Merge To** in a direction-selection wizard; click a branch name to copy it. **Compare Branch** provides a Folder Compare-style view with per-file drill-down.
- **Git Log** has instant in-memory filtering of loaded commits by SHA/message/author/date (no per-keystroke Git process), `Load older commits` in 100-commit pages, and **Show Diff**: File Compare for a single changed file, changed-only Directory Compare for multi-file commits, and parent selection for merge commits. These diff views are read-only. **Working Tree Diff** separates `HEAD ↔ Staged`, `Staged ↔ Working`, and `HEAD ↔ Working` with syntax highlighting, inline token changes, context, and code/comment change classification.
- **Smart Terminal** suggests commands/aliases from the shell and verified `PATH` executables, including Bash prompts without OSC 133; local terminal input also supports `\help`, `\ai`, and `\do` commands. Workspace Favorites/Recent, task search, notifications, and theme/font controls remain available;
- the **Connections** panel supports local terminals, SSH, **SFTP**, **FTP**, and databases; its FileZilla-style workspace supports recursive folder upload/download, session-scoped remote folder caching, a per-file Transfer Queue (Queued/Running/Done/Failed), per-source/profile Favorites/Recent paths, and sortable headers; SFTP reuses system OpenSSH and FTP remains Go-stdlib only, with no npm or new package dependency;
- local or remote access through `vscode_tasks_menu.ini`, including auth/TLS and safety checks before binding outside loopback.

Daemon management:

```bash
./tools/vscode_tasks_menu --status
./tools/vscode_tasks_menu --stop-daemon
./tools/vscode_tasks_menu --restart-daemon
```

#### Shared Server / Multi-user

TaskDeck supports a **multi-user shared-server mode** built around the invariant **one daemon = one workspace/project**. Multiple daemons on the same host may share one identity database, allowing a global user to participate in multiple projects while project permissions, sessions, and operational data remain isolated.

The mode can be switched directly from **Settings → Security → Authentication mode…** through a three-step `Mode → Setup → Review` wizard, or configured manually with `[shared_server] enabled=true` in `vscode_tasks_menu.ini`. The wizard supports both migration directions:

- **Single → Shared:** creates/reuses an identity DB outside the workspace, provisions or verifies the initial administrator, grants `system:admin` for the project, enables HTTPS, disables Basic Auth, and scrubs the legacy plaintext Basic Auth password from the active config. If the username already exists in a shared identity DB, its password must verify; TaskDeck never silently resets a global identity.
- **Shared → Single:** requires a new Basic Auth username/password because a shared password hash cannot and must not be reversed; all browser sessions for the project are revoked while the identity DB/users/roles are retained for a later switch back to Shared.
- On platforms supporting `--reload-config`, the web daemon reloads after migration while the independent session broker keeps running tasks/terminals alive.

Implemented capabilities include:

- **Shared identity + project membership:** user/password credentials are global identities; roles, memberships, and permission overrides are project-scoped.
- **Fail-closed RBAC:** system roles `admin / developer / operator / viewer`, project custom roles, and explicit permissions for Tasks, Terminal, Patch, Files, Git, Settings, Self-update, Users, Roles, Sessions, and Audit. Backend enforcement is authoritative; frontend visibility is UX only.
- **Project-bound browser sessions:** sessions persist `project_id`; a token minted in project A is not accepted by project B even when the same user belongs to both. Sessions use a 30-minute idle timeout and 12-hour absolute lifetime.
- **Terminal privacy/ownership:** view and control permissions are separate, own/all scopes are supported, and WebSocket authorization is enforced end-to-end. Foreign Origins are rejected before attach.
- **Project-scoped administration:** project administrators can only view or mutate users, custom roles, sessions, and audit data belonging to their project; direct-ID cross-project access is denied.
- **Password lifecycle:** self-service password change and operator reset share one policy; password change/reset revokes all login sessions for that global identity across projects.
- **Audit:** records authentication, authorization denials, task/terminal/Patch/file/settings/self-update/admin actions, and mutation-lock conflicts without logging passwords, raw tokens, or terminal keystrokes.
- **Shared-workspace mutation lock:** coordinates mutating Patch runs, file writes, and self-update so conflicting workspace mutations do not run concurrently.
- **Identity DB backup/restore:** SQLite online backup, integrity checks, private backup files, pre-restore safety snapshots, and bounded rollback.
- **HTTPS/reverse-proxy hardening:** shared mode requires real TLS on the TaskDeck listener; internal self-update handoff/detach requires both loopback transport and a private daemon control token.
- **Safe schema migration:** auth sessions are project-scoped; legacy sessions whose project cannot be determined are revoked during migration instead of being assigned heuristically.
- **Broadcast keyboard fan-out** is intentionally **disabled in shared mode** until it has a dedicated ownership/permission model.

Automated first-release acceptance covers two independent daemons/Stores sharing one identity DB, HTTPS login + Secure cookie + WSS authorization, cross-project isolation, migrations, and security regressions. CI runs on Go 1.19.x and Go 1.23.x with staged self-update tests, `go test ./...`, `go vet ./...`, and a production build. Final first-release sign-off still requires smoke testing in a real deployment: browser profiles, HTTPS→HTTPS reverse proxy/WSS, password lifecycle, backup/restore drill, and upgrade drill.

Shared-server documentation:

- `vscode_tasks_menu_go/MULTI_USER_SHARED_SERVER_PLAN.md` — architecture, permissions, roadmap, schema, and implementation status.
- `vscode_tasks_menu_go/SHARED_SERVER_DEPLOYMENT.md` — TLS/reverse proxy, identity DB, backup/restore, and multi-project deployment.
- `vscode_tasks_menu_go/SHARED_SERVER_ACCEPTANCE.md` — automated acceptance evidence and real-environment smoke checklist.

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

**Python Patch Tool** never treats a manifest as an arbitrary shell script: sensitive operations are restricted by schema/allowlists, PATCH/COLLECT manifests do not execute Git mutations, and `manual_execution` only guides operator-run commands. **TaskDeck Git Quick Actions** has its own confirmed/permission-checked mutation workflows, and terminal commands are user initiated. Missing or failed evidence must be reported explicitly, never misrepresented as PASS.
