# TaskDeck Remote — task.run executable and environment boundary (2026-10-31)

Source branch: `security/remote-task-executable-boundary-20261031`, based on `main` commit `9a44375085e2ac3dd1e18b255d03e5e40c1bc4ab`.

## What this closes

The previous independent host approval covered the complete `tasks.json` definition (`ID@SHA256`), but not the executable selected by `exec.Command`. A relative command is resolved using the **daemon's** `PATH` before the child receives its sanitized environment. A command or symlink inside the workspace can also be replaced independently of the approved definition. The child inherited the daemon's `HOME`, exposing the private TaskDeck config directory location.

For the opt-in encrypted `task.run` ONLY:

- Require an **absolute** command executable: no daemon `PATH` search.
- Reject the executable if its path is in the workspace **or** if the resolved target (through symlinks) is there. Reject non-regular, missing, non-executable, group/world-writable binaries. Host CLI `taskdeck remote-task --workspace ... list` uses the same runtime predicate.
- Override child environment with only `PATH=/usr/bin:/bin`, `HOME=/nonexistent`, `LANG=C`, `LC_ALL=C`, `TERM=dumb`. Do not forward daemon `HOME`, `PATH`, private signing keys, Portal credentials or ambient variables.
- Keep `TASKDECK_REMOTE_TASK_RUN=1` and exact host-pinned `TASKDECK_REMOTE_APPROVED_TASKS=ID@SHA256` mandatory, together with client Ed25519 possession, real TaskDeck password, live grant, per-action RBAC, local audit, cancellation and short deadline.
- No arbitrary command line or input is accepted from PHP or Relay. The selected task ID is bound to the exact reviewed host definition.

**Host deployment change:** existing remotely approved tasks with relative commands (`go`, `python3`, etc.), executable files in the project, or mutable symlinks must be rewritten to use reviewed, absolute executables **outside** the workspace and approved again using the host CLI. A new SHA is required. Interpreter invocations can still execute mutable scripts/configuration passed as arguments; the host operator must independently trust and protect such dependencies. This guard is not OS sandboxing, and does not protect against compromise of the host's own OS account.

## Acceptance and release status

Tests cover the trusted `/bin/echo` path, relative lookup denial, workspace-contained command denial, symlink-in/out denial, group/world-writable denial, exact isolated child environment and a real encrypted `task.run` dispatch that refuses an otherwise host-approved workspace executable before any execution.

CI results and independent security review must be recorded against the exact PR head before merge. **No release approval is implied by this change.**

Remote Control remains `NO-GO`, `remote_control_available=false`, no production UI/PTY/HTTP/File/Git/Patch control enabled by this work. The three release invariants remain: (1) hacked PHP cannot authorize TaskDeck, (2) E2EE TaskDeck application bytes cannot be decrypted by PHP/Relay, and (3) no commands can be run before native-client-key + TaskDeck login + live per-action RBAC. External shared-host/Relay and production penetration tests remain mandatory.
