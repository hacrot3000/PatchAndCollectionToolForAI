# Main → Remote integration resolution (2026-11-05)

Source main: `2ded4207222acaed4f07f50fc9fd3bdfea5234d5`.
Target remote: `12af56d278f289b7dc8fac47dc49ce4adc6cca69`.
Shared ancestor: `5bf68380dac8b1719d8a493cc9ceb7926da237f1`.

The history-preserving merge commit `7971763d2ba8cd9ad9e9bb88e4c8f7cb256b1db1`
has exactly two parents (remote, main) and the byte-for-byte Git
`merge --no-commit --no-ff -X ours` tree.

## Post-merge compatibility selection

CI on that raw merged tree demonstrated duplicate Go E2EE result functions and
different, incompatible `PingClient` method signatures and request schemas.
In particular, main and remote independently implemented `git.diff`,
`git.log`, `git.commit` and task execution with different typed protocols.
Blindly taking a union would not be a safe authenticated remote API.

This follow-up keeps the **remote architecture implementation** as the
authoritative default under `internal/remotetransport`,
`internal/remoteportal`, `internal/remoterelay`,
`cmd/taskdeck_remote_login` and related `internal/server/remote_*` handlers.
Main's separate native interactive GUI, file-transfer implementation,
review/digest Git API and its dependent server helper files are *not*
enabled in this merged build. Corresponding one-off workflows were
omitted in favor of the existing remote-agent and portal suites.

Main's unrelated improvements (PATCH/COLLECT, database, broker,
self-update, task variables and ordinary application UI) remain in the
merged tree wherever they do not depend on the incompatible alternative
remote transport. Existing remote task GUI, Git pull/commit/push and PTY
remain protected by their original independent E2EE permissions.

## Integration gate

Before promotion to the named remote branch, run Go Agent and PHP Portal
CI plus relevant PATCH/COLLECT, MySQL and self-update checks. No forced
branch update, production remote-control enablement or weakening of
existing authentication is authorized by this merge.

**Deferred code integration**, not a device-test-only gap: porting main's
independently implemented remote file/interactive operations onto the
remote architecture with explicit typed-operation design review and
focused regression tests.
