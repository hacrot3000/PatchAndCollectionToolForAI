# TaskDeck Remote — Git branches (read-only)

A native-only, authenticated TaskDeck GUI can now inspect a bounded list of
**local Git branches** at the exact configured host workspace root. The
untrusted PHP Portal / Relay can only forward opaque E2EE frames. Neither
party chooses a Git path, revision, remote, ref, URL or command.

## Contract

- Fixed no-argument E2EE action `git.branches`.
- Host must enable `TASKDECK_REMOTE_GIT_BRANCHES=1` (default OFF).
- TaskDeck project permission `git.status`, independently approved native
  client key, authenticated local password principal, and live Portal grant
  are rechecked per action. Local read audit is required.
- Runs only on Linux/macOS. A detached HEAD, uncommitted/unborn repository,
  workspace nested under another repo, invalid Git branch label, oversized
  output, revoked session or command failure all deny the request.
- Uses `git symbolic-ref --quiet --short HEAD` and fixed
  `git for-each-ref --count=5 --sort=refname
  --format=%(refname:short) refs/heads/`; no shell, hooks, external
  helpers or networking. The response reports the current branch and
  at most four other local branch labels (with `... more` when needed).
- Validates each branch label as simple ASCII Git text. E2EE response is
  at most 240 bytes; larger values are refused rather than truncated or
  leaked through error output.
- The browser displays a plaintext result, not HTML, and does not offer
  checkout, merge, ref deletion, force push or other write operations.
  A branch-read feature flag never implies Git write privileges.

## Automated verification

Go tests exercise the fixed E2EE request, forged response/extra fields,
default-off flag, root isolation, detached HEAD and a real repository with
more than four local branches. Native localhost API tests reject URL
parameters, body smuggling and untrusted PHP origins. JavaScript tests
check separate read permission, no request body and post-logout disabling.
The remote-agent CI runs the Git tests under the race detector.

## Remaining remote code work

Git: branch checkout/create/delete after separate scope and safety review,
diff patch pagination and hunk selection, merge workflows and other actions.
Terminal: sustained streaming/reconnect/session restoration.
Tasks: dependency execution, secure input of secrets, and output replay
across sessions.
Other GUI: editor/files and remaining task/remote screens.
Windows-specific support is deliberately deferred.

Already implemented code counts as complete without requiring device-level
acceptance, but production deployment still requires a security review.
