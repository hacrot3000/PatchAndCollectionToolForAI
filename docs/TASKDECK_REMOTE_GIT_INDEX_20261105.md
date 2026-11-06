# TaskDeck Remote Git index — Stage tracked / Unstage all (2026-11-05)

The independently trusted native Remote GUI now has two deliberately
limited Git index operations. They execute only at the exact canonical
TaskDeck workspace Git repository root via authenticated E2EE to the Agent:

| Fixed E2EE operation | Host action |
| --- | --- |
| `git.stage_tracked` | `git add -u -- :/` (tracked modifications and deletions only) |
| `git.unstage_all` | `git restore --staged -- :/` (clear entire index to HEAD; keep working files) |

Neither operation accepts a client-selected path, revision, ref, command,
environment or custom Git argument. In particular, **stage tracked does
not add any untracked/new file**. Unstage all may remove newly added
files from the index, but does not remove them from the working tree.

## Permission and security model

- Two **separate host opt-ins**, both default OFF:
  `TASKDECK_REMOTE_GIT_STAGE_TRACKED=1` and
  `TASKDECK_REMOTE_GIT_UNSTAGE_ALL=1`.
- Both require the real authenticated TaskDeck principal,
  `git.write` per-project permission, approved native client software
  key and active Portal grant. A separate explicit GUI confirmation is
  required for each broad operation.
- Linux/macOS only. Detached/unborn HEAD, parent/nested repository,
  expired/closed session, a session with an active long task/PTY, an
  already-clean target or malformed E2EE action is refused.
- Fixed Git binary/arguments without a shell; minimal environment,
  disabled fsmonitor, disabled hooks, protected process groups,
  eight-second timeout and session-revocation cancellation.
- **Stage tracked fails closed if the effective local Git configuration
  includes an executable `filter.*.clean` or `filter.*.process`
  driver**, since Git clean/process filters can execute arbitrary code
  during `git add`. This intentionally restricts some Git LFS or custom
  filter repositories; use a separately reviewed local workflow there.
- Durable TaskDeck-local attempted audit is written **before mutation**.
  Audit contains no commit message, file paths, output, secrets or
  credentials. Local HEAD is rechecked after mutation and must not
  change. There are no automatic retries.
- E2EE success returns the exact operation and 40-hex HEAD, without
  any path or file content. GUI rejects mismatched responses and
  invalidates its previous Git Status snapshot.

## Verification

Go E2EE tests cover typed operations, forged/duplicate responses,
cross-operation substitution and rejection of client options.
Real temporary-repository tests verify tracked-only staging, preservation
of untracked files, index-only unstage, HEAD preservation, default-off
flags, no-op denial, host audit and refusal of executable Git filters.
The native localhost API rejects query paths, JSON bodies and PHP origins.
A browser test verifies separate feature gates, confirmations and
post-logout state. Agent CI runs the Go tests with the race detector.

## Code still missing

Per-file interactive stage/unstage is **not** implemented. Full diff
pagination, additional branch/revision actions, richer Terminal
streaming/reconnect, advanced task dependencies/secret input/output
replay, editor/files GUI and other remote capabilities remain. The
bounded Git task/PTY/fetch/pull/commit/push/log/diff operations are
separate features. Their code completion is not conditioned on live
hardware acceptance, though production rollout requires a security gate.
