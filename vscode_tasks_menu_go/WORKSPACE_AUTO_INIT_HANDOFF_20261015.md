# TaskDeck Workspace Auto-Init — Handoff (2026-10-15)

Branch: feature/workspace-auto-init-20261015
Status: Code implemented. Native Go/CI and real terminal smoke testing still needed.
Policy: Do not commit this feature to main or merge without explicit instruction.

## Goal

When TaskDeck starts in a new project missing .vscode/tasks.json, ask whether to create a project scaffold. After the user accepts, scan a few directory levels, recognize language/build markers and generate editable sample tasks.

## Implemented

- cmd/vscode_tasks_menu/main.go: startup preflight, new --init-tasks flag for explicitly requested noninteractive creation, no prompt from daemon/maintenance/self-update.
- cmd/vscode_tasks_menu/workspace_init.go: [y/N] confirmation, language summary, create tasks, continue normal startup and automatic INI creation.
- internal/workspaceinit/init.go: bounded breadth-first scan (3 folder levels, at most 4000 directory entries), excludes node_modules, .git, vendor and generated caches; does not follow source-directory symlinks or run user code.
- Languages: Go, Node.js/JS/TS, Python, Rust, Java Maven, Java/Android Gradle, Nim, PHP Composer, CMake/C++, Make, Dart Flutter. Handles mixed and nested projects with individual working directories.
- Task examples use type=process with separate command/args rather than interpolated Bash. npm scripts are included only when declared in package.json.
- Parent manifest suppresses redundant nested source-only tasks; unrecognized project receives a simple pwd task.
- Creates .vscode/tasks.json with exclusive file-create semantics; never overwrites an existing file or follows a .vscode symlink.

## How to exercise

- From a Go project with go.mod but no tasks.json: run taskdeck, answer y. Go Test/Build/Vet should be proposed and TaskDeck starts.
- Answer n or Enter: no writes.
- Run taskdeck --workspace /absolute/project --init-tasks --no-browser for explicit noninteractive creation.
- Existing tasks.json remains unchanged, including malformed or symlinked files.
- Mixed project with root go.mod plus frontend/package.json: creates separate task groups, frontend tasks have frontend cwd.
- --serve, --status, taskdeck patch, session broker and noninteractive invocations do not request project creation without the explicit startup flag.
- Project scanning is only an inspection; generated tasks do NOT run until selected by the user.

## Tests added

- internal/workspaceinit/init_test.go: multilingual scan, safe depth/exclusions, symlink protection, duplicate suppression, fallback, tasks.Load and ResolveExecution compatibility.
- cmd/vscode_tasks_menu/workspace_init_test.go: yes/no/default, explicit opt-in without TTY, existing tasks unchanged, invalid workspace rejection.
- Feature branch is included in the workflow push trigger at .github/workflows/vscode_tasks_menu_go.yml.

Native go test/go build and an actual taskdeck launch have NOT been verified during this session. Do not claim they passed before checking.

## Next checks

- Run go test ./internal/workspaceinit ./cmd/vscode_tasks_menu and ideally go test ./... from vscode_tasks_menu_go.
- Smoke-test new workspace creation and generated tasks from a build of this feature branch.
- Update this handoff and commit fixes in small increments, without modifying main.
