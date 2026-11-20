# TaskDeck Remote Git pull — bounded implementation (2026-11-05)

Branch: `feature/taskdeck-remote-access-architecture`.

## Implemented in code

The **trusted native** Remote GUI contains an explicit, separately confirmed
`Git pull (fast-forward)` button. This is a fixed `git.pull` application
operation over the same Agent-authenticated E2EE channel as task execution.
PHP Portal and Relay do not receive the repository path, command, Git output
or plaintext credentials. The client cannot select a remote, branch, URL,
working directory or arbitrary Git arguments.

Host admission requires all of:

- The Remote application session already has a verified approved client key,
  local TaskDeck password/principal and live Portal grant.
- Current `git.write` permission, Portal operator/admin grant and a separate
  host switch `TASKDECK_REMOTE_GIT_PULL=1`. It is **OFF by default**.
- Exact canonical TaskDeck workspace Git root, an attached branch and
  configured upstream, and a clean status including untracked files.
- Mandatory, host-local attempted audit linked to the TaskDeck user and
  project **before** the network request.
- A cancellable **9-second** deadline inside the existing 15-second action
  lifecycle. Only `git pull --ff-only --no-recurse-submodules --no-edit` is
  executed, with hooks and file/ext protocols disabled for the operation.
  The subprocess group is retired on completion/cancellation.

Successful response contains two lowercase 40-hex commit IDs (before and
after) and a matching `updated` boolean. The strict E2EE decoder rejects
duplicate/unknown fields, fabricated capabilities, wrong IDs or inconsistent
results, and closes the channel on invalid authenticated replies. The native
GUI clears cached Git Status after a successful pull. Git output,
network errors, private repository details and credential data are not
forwarded to the PHP Portal or native GUI.

The implementation is currently Linux/macOS only; it supports noninteractive
host-side Git authentication (such as an existing SSH agent) and SHA-1 format
repositories. Large fetches exceeding the nine-second bound must be handled
separately; the deadline is intentional, not an unlimited network job.
The operation refuses dirty worktrees, diverged branches and unsafe remotes;
it does not attempt force-pull, merge or rebase.

## Verification and remaining scope

New Go regression tests exercise authenticated E2EE request/result decoding,
client fixed-operation routing, local-only GUI endpoint, forbidden arguments,
host opt-in, missing upstream, foreign workspace roots and viewer denial.
Go Agent and PHP Portal CI status must be checked for the source commit.

**Not yet implemented in remote application transport:** Git commit and push,
other Git write operations, interactive Terminal/PTY, general file/edit GUI,
multi-step task dependencies, secrets and resumable output across E2EE
sessions. These remain source-code backlog. Hardware/live deployment,
independent security review and production go-live are separate release gates,
not source-code feature completion. Remote remains disabled by default.
