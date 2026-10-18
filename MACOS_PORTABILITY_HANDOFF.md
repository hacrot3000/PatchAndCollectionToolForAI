# TaskDeck macOS portability handoff — 2026-10-18

## Scope and branch

- Branch: `feature/macos-native-support`, branched from main `8408b9b0583f3d724c9845b2153b4039b46d03cc`.
- Purpose: make TaskDeck usable on macOS Intel and Apple Silicon, without changing Linux business logic or weakening path checks.
- No Windows native/ConPTY claims; Windows continues through Docker/WSL2.

## Delivered source

1. Vendored unchanged upstream github.com/creack/pty v1.1.24 Darwin support (`pty_darwin.go`, `ioctl_bsd.go`) and arm64 types. Preserves `GOPROXY=off`/offline build.
2. Added macOS safe editor backend using Go 1.25+ `os.Root` for pinned, descriptor-relative reads/writes/rename, symlink-outside protection, atomic temporary file replacement, permissions and owner restoration. Kept non-Linux generic fallback only for non-Darwin targets; old Go on Mac intentionally fails to compile instead of using the race-prone fallback.
3. Added BSD `ps` process-tree parsing with `etime`; terminal live CWD uses `lsof` instead of Linux `/proc` and falls back to launch CWD.
4. macOS-portable global installer: BSD `tar`, BSD `mv -h` atomic symlink replacement and minimum Go 1.25 gate.
5. Docker Desktop launcher isolates Compose project/home volume by canonical workspace hash; uses existing loopback-only compose config; stop retains volume. Mock regression tests.
6. Documentation in Vietnamese and English: README and `docs/MACOS.md`.

## Documentation pass — 2026-10-18

The follow-up documentation pass (same feature branch / PR #12) includes:

- `README.md` **Vietnamese + English**: recent October capabilities; Schedule/Workflow, granular role permissions, terminal/editor/Git improvements; quick native and Docker usage; platform comparison and clear unverified status.
- `vscode_tasks_menu_go/README.md`: backend overview for new features and native/Docker Mac; links to CI and acceptance.
- `docs/README.md`: bilingual documentation index.
- `docs/FEATURES.md`: feature catalog with supported vs incomplete Schedule capabilities and shared-role security notes.
- `docs/PLATFORM_SUPPORT.md`: Linux/macOS Intel/macOS ARM/Docker/Windows capability matrix and runtime constraints.
- `docs/MACOS.md`: detailed bilingual installation and troubleshooting for native/Docker; port, credentials, security, updating, differences and acceptance steps.
- `vscode_tasks_menu_go/SELF_UPDATE.md`: avoid implying untested Mac listener/session handoff is guaranteed, distinguish Docker image updates.
- `vscode_tasks_menu_go/SHARED_SERVER_DEPLOYMENT.md`: warn that Mac native shared-server and multi-user readiness still need tests, and default Docker loopback is not a public deployment design.
- `docs/WINDOWS.md`: cross-reference macOS and the platform matrix.

**Validation of documentation**: inspect relative links and commands against this feature branch. Docs commits do **not** change status of native CI, Mac filesystem metadata parity or hardware smoke. Keep PR in Draft until acceptance gates are satisfied.

## Validation status and controls

- **Source changes committed**, not yet marked verified native release.
- `.github/workflows/vscode_tasks_menu_go.yml` has a macOS Intel + Apple Silicon job, selectable with `workflow_dispatch` on this branch before merge (the workflow exists on main already). Separate `taskdeck_macos_native.yml` is available after merge.
- Mac jobs compile ALL Go packages/test sources with Go 1.25, run Darwin pinned editor security and `ps` tests, run Docker launcher mock tests, build native CLI and run isolated `install.sh`.
- CI cannot be asserted PASS unless actual GitHub Actions runs complete.
- Linux original manual CI must also be rerun before main merge.
- On an actual Mac, validate web startup, auth, multi-user restrictions, PTY read/write/resize/close, terminal cwd, Git, editor, task execution, SSH/SFTP, Patch, self-update and rollback.

## Known non-parity / risk to close

- macOS Finder metadata: the Darwin atomic editor currently restores ordinary mode and ownership but not all xattrs, ACLs and resource forks. Need native Mac tests and a descriptor-based metadata-preservation implementation before claiming complete metadata parity. Do not silently claim parity.
- Linux's broker-preserving daemon reload is Linux-only. macOS restart must be tested; no equivalent guarantee.
- `self-install-and-update.sh` (separate legacy Python Patch standalone path) still has GNU utility assumptions. Use global `install.sh` for TaskDeck on macOS.
- Docker Desktop runs Linux commands inside the container; there is no implicit access to host macOS CLI, hardware or keychain.
- Optional host tools (database CLIs, language servers, GNU utilities) remain dependencies to install separately for their respective features.

## Repeatable acceptance

1. Select `feature/macos-native-support` under Actions → **VSCode Tasks Menu Go** → Run workflow; verify macos-15-intel and macos-15 jobs PASS.
2. Run `bash deploy/macos/test_launcher.sh` locally (mock Docker only).
3. On native Mac: Go 1.25+; `./install.sh`, `~/.local/bin/taskdeck --workspace "$PWD"`, perform UI/hardware smoke listed above.
4. On Mac Docker Desktop: `./deploy/macos/taskdeck.sh doctor <project>` then `start <project>`; confirm port 42882 loopback, HTTPS/auth and project files mounted.
5. Compare branch to main; ensure PR reviewed, tests run and safety issues triaged before merge.

Do not infer tests passed from static review alone.
