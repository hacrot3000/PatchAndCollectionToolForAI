# TaskDeck Remote — E2EE GitStatus subprocess isolation

Date: 2026-10-31. Branch: `security/remote-git-status-process-isolation-20261031`.
Base: `main` at `1cfd12f855c79c903206f8f30388bf4d58bed585`.

## Security issue closed

The experimental authenticated `git.status` route previously invoked `exec.CommandContext(ctx, "git", ...)`. Go resolves this executable using the **TaskDeck daemon's own PATH**, not the sanitized `cmd.Env`. The route also explicitly used `append(os.Environ(), ...)`: this preserved daemon/private `HOME`, `GIT_EXEC_PATH`, `GIT_CONFIG_COUNT`, `LD_PRELOAD`, `DYLD_INSERT_LIBRARIES`, etc. A workspace-controlled executable could therefore run under TaskDeck's OS identity, despite the narrow intended read-only `GitStatus` capability. Disabling fsmonitor by `-c` was insufficient against ambient-process injection.

## Changes

- Select Git only from fixed system locations (`/usr/bin/git`, `/bin/git`, `/opt/homebrew/bin/git`, `/usr/local/bin/git`), in that order. Validate the binary using the existing host-only executable policy: absolute executable outside the editable workspace, real regular executable, and not group/world-writable. If none is safe, return an E2EE action denial; never fall back to daemon PATH.
- Run Git with a **complete, minimal environment**, not `os.Environ`. Explicitly disable the global/system Git config, optional locks and terminal prompts. `HOME=/nonexistent` and fixed `PATH=/usr/bin:/bin`. No `GIT_CONFIG_COUNT`, trace hooks, dynamic loader injections or private TaskDeck environment survive.
- Maintain the existing fixed `git status --short --branch --untracked-files=no` arguments and local TaskDeck `GitStatus` RBAC, TaskDeck password + owner-approved client key, live Portal grant, authenticated E2EE and local audit. Do not add generic Git, arbitrary commands or new PHP authority.
- Regression test with a fake `git` in the workspace prepended to daemon PATH, hostile `GIT_CONFIG_COUNT`, `GIT_EXEC_PATH`, and an ordinary E2EE authenticated Git status. Assert the forged Git is not executed and no marker is written. Test the exact allowed Git subprocess environment separately.

## Residual risk and operational constraints

An operator-controlled Git installation at an otherwise trusted path and repository metadata remain outside this check's byte-level trust. On macOS, the system `/usr/bin/git` toolchain shim requires working command-line tools; installation/configuration must be validated on an actual device. The bounded `192`-byte status output limit remains, and oversized output continues to fail closed.

This patch is **not** remote browser functionality or a public Remote Control release. No Terminal/PTy, HTTP proxy, generic Git, File streaming, Patch/Collection or live control is enabled. All three security invariants still apply to each future action. Keep `remote_control_available=false` and Remote Control `NO-GO` until real deployment and independent security review of the complete action surface.

## Validation required

- GitHub Actions: Go Agent/Relay and PHP Portal must pass on the **exact same commit**.
- Native Linux and macOS host checks with system Git and external Relay/PHP after approval.
- Compromised Portal/Relay simulation must not obtain a TaskDeck principal or plaintext.
- No release or deployment before production acceptance.

CI SHA and run links to be added after completion; this document intentionally does not claim tests have run before they have.
