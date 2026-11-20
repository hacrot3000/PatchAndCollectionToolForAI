# TaskDeck native project launchers (Linux / macOS)

## English

TaskDeck can create a native OS application launcher for the **currently open
project**. Open **Settings → Workspace → Create project launcher icon…**
in a locally running, native TaskDeck instance.

- **Linux**: installs a Desktop Entry at
  `$XDG_DATA_HOME/applications/taskdeck-project-<workspace-id>.desktop`,
  falling back to `~/.local/share/applications`. It appears under the host
  desktop's application menu/search (e.g., GNOME, KDE). The icon uses the
  project-specific copy of the bundled **Glossy Terminal Task Stack** image.
  The PNG is stored under `$XDG_DATA_HOME/taskdeck/launcher-icons/`
  (or `~/.local/share/taskdeck/launcher-icons/`). Each Desktop Entry's
  `Icon=` points to its own stable absolute image path.
- **macOS**: installs an application bundle under
  `~/Applications/TaskDeck Projects/TaskDeck - <project> - <workspace-id>.app`.
  The bundle is a lightweight shell launcher with an Info.plist; the project
  opens in the default browser via TaskDeck's normal startup. Its
  `Contents/Resources/taskdeck.icns` icon is generated from the same PNG at
  16, 32, 64, 128, 256, 512 and 1024 pixels; `CFBundleIconFile` references
  that file for native Finder/Dock/Launchpad presentation.
- Both launchers execute the **native host's TaskDeck** with explicit
  absolute `--workspace` and `--open-browser` arguments. Clicking a
  shortcut opens the project UI even when automatic browser launch is off in
  the project's normal CLI configuration. They work irrespective of the
  launching directory. An installed `taskdeck` executable in `~/.local/bin` (or
  PATH) is preferred if it matches the running binary, so ordinary
  self-updates keep the launcher valid.
- Distinct workspaces receive separate, deterministic IDs even when
  directory names are identical. Repeated clicks **update** the existing
  shortcut **and its image**, with the same launcher filename, and never
  create duplicates. Other projects and their icons are not changed.
  The backend refuses to overwrite an unrelated file or bundle. Concurrent
  installation requests are serialized in the local TaskDeck process.
- The feature is intentionally **not available in Docker/containers,
  Windows, shared-server mode, or from non-loopback HTTP clients**. It cannot
  install an icon on another computer merely because that computer has opened
  the TaskDeck web UI. Use native TaskDeck on the actual host.
- The API accepts no user-supplied filesystem paths or shell commands.
  Existing launchers are updated only after identifying their TaskDeck
  workspace marker. Every path used in an Exec command or shell script is
  quoted; the creation operation uses temporary files and atomic renames.

The Settings button only appears when `GET /api/system/project-launcher`
indicates native support. Clicking it sends
`POST /api/system/project-launcher`; a successful result includes the path.
The running server must have permission to write its user's Applications /
application menu directory. On Linux the desktop shell may need a moment
to refresh its application index. On macOS, if LaunchServices/Spotlight has
not yet indexed the bundle, locate it in `~/Applications/TaskDeck Projects`
and open it once from Finder. Apple security protections may require a
manual first-launch confirmation for locally generated, unsigned apps.

## Tiếng Việt

Trong TaskDeck chạy trực tiếp trên máy Linux hoặc macOS, mở
**Settings → Workspace → Create project launcher icon…**.

- **Linux**: thêm icon TaskDeck cho project vào menu ứng dụng/launcher
  của desktop, trong `~/.local/share/applications` (hoặc `$XDG_DATA_HOME`).
  Dùng hình **Glossy Terminal Task Stack.png** đã đóng gói trong TaskDeck,
  với một đường dẫn PNG ổn định riêng cho từng project.
- **macOS**: tạo `.app` riêng cho project trong
  `~/Applications/TaskDeck Projects`, có icon `.icns` đa kích thước
  chuyển từ cùng hình gốc. Có thể mở bằng Finder/Spotlight
  sau khi hệ thống cập nhật chỉ mục.
- Bấm icon sẽ khởi chạy TaskDeck đúng thư mục project và mở trình duyệt,
  kể cả khi cấu hình CLI thông thường tắt tự mở browser.
  Nhiều project được tạo nhiều icon độc lập; bấm nút lần nữa để cập nhật
  launcher **và file icon hiện có**, không tạo thêm launcher mới.
- Nếu TaskDeck chạy trong Docker, trên remote/shared server hoặc không
  phải app native của người dùng desktop, nút sẽ không hiện. Các trường hợp
  này không thể cài icon lên host từ bên trong container.
- Nếu ứng dụng macOS chưa được ký, Finder có thể cần người dùng xác nhận
  ở lần mở đầu tiên.

## Code / Test

- `internal/server/project_launcher.go`: status/create API, native Linux
  desktop entries and macOS bundles, workspace IDs, ownership protection
  and runtime eligibility checks.
- `internal/server/project_launcher_icon.go` and `taskdeck_icon.png`:
  offline image embedding from the original repository-root file
  `Glossy Terminal Task Stack.png`, and native multi-resolution ICNS creation.
- `internal/server/project_launcher_test.go`: both output formats, macOS
  plist, shell escaping, install/update, collision avoidance, symlinks,
  shared-server and non-local request rejection.
- `web/featuremods/projectlauncher.js`: Settings button lifecycle.
- `tests/project_launcher.test.mjs`: frontend visibility and POST flow.
- `internal/server/server.go`: API route.
- `web/featuremods/menus.js`, `next.js`: Settings integration.

Validation:

```bash
cd vscode_tasks_menu_go
go test ./internal/server -run '^TestProjectLauncher' -count=1
go test ./... -count=1
go build ./...
node --test tests/project_launcher.test.mjs
```

The asset is compiled directly into the binary via Go embed. Installing
or refreshing a launcher does not fetch a remote image and does not depend
on Internet availability. The embedded asset has exactly the same Git blob
SHA as the repository-root `Glossy Terminal Task Stack.png` source. Tests
exercise icon decoding, ICNS entries, updating stale icons, and independent
launchers for two projects with identical folder names.

Manual smoke tests must run on an actual Linux desktop and macOS host:
create shortcut, launch from the application menu/Finder, verify the chosen
workspace opens in the browser, then repeat after a TaskDeck self-update.

GitHub connector commits do not imply that local Go builds, UI smoke tests
or macOS LaunchServices integration have been executed.
