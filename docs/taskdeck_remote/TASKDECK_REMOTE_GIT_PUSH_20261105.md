# TaskDeck Remote Git push — bounded E2EE operation (2026-11-05)

Branch: `feature/taskdeck-remote-access-architecture`.

## Implemented

The native trusted GUI now exposes a separate **Git push (không force)**
button. It requires explicit confirmation, the **host-only** opt-in
`TASKDECK_REMOTE_GIT_PUSH=1` (default OFF), and a freshly verified
`git.write` permission on the existing TaskDeck-local authenticated
password+approved-client-key session. The live Portal grant and membership
are rechecked around the operation. PHP Portal and Relay only forward opaque
E2EE records and cannot select a repository, branch, refspec or command.

The host checks the exact canonical TaskDeck workspace Git root, attached
branch, a single Git upstream tracking the **same branch name**, a permitted
SSH/HTTPS effective push URL, one remote destination, and local ancestry of
upstream → HEAD. The actual push is a fixed, no-force/no-tags/no-verify
Git invocation with one refspec:

```
<verified-local-HEAD-sha>:refs/heads/<verified-current-branch>
```

The client never supplies these identifiers; Git resolves the host's
configured remote. The server independently rejects non-fast-forward pushes.
A local signed attempted-audit tied to the real TaskDeck user/project is
required **before** network execution. Audit logs do not include the remote
URL, credentials, branch name or command output. The command has a
nine-second timeout inside the existing authenticated-action deadline,
process-group cancellation, disabled pre-push hooks, restricted network
protocols and noninteractive SSH/HTTPS authentication. Successful E2EE
responses contain only the exact pushed commit object ID. GUI Git Status
cache is invalidated on success.

This code is currently Linux/macOS only and supports 40-hex SHA-1 Git object
IDs. It deliberately refuses remote branches whose upstream name differs
from the local branch, multiple push URLs, local/file/ext/git protocol URLs,
client-selected force/tags, and pushes from a detached HEAD. Authentication
must already be configured locally on the host (for example, an approved SSH
agent). Larger transfers that exceed the bounded deadline are **not**
supported by this task-specific operation; no unbounded session or retry is
silently started.

## Code verification

Go tests cover strict request/result E2EE codecs; empty/extra/forged
arguments and records; advisory capability flags; viewer denial; host opt-in;
workspace root isolation; untrusted URL schemes; branch/upstream mismatch;
multiple destinations; and a positive fast-forward push to an isolated bare
Git repository through a **test-only local SSH stub**, followed by a remote
advance to verify non-fast-forward refusal. Native GUI tests exercise
confirmation, feature gating, localhost capability/Origin/body constraints,
stale-status invalidation and logout. The Go Agent CI includes the new
browser test and existing protocol/security suites.

## Remaining code backlog

1. Interactive Terminal (streaming PTY, lifecycle, audit, resize).
2. Other Git operations (fetch, log, diff, stage, unstage, branch management).
3. Remaining GUI screens (files/editor and other host operations).
4. Advanced task dependencies, secret-bearing inputs, output replay across
   separate authenticated sessions; and additional remote capabilities.

All Git operations remain individually host opted-in. Passing source tests
does not automatically authorize production enablement or replace security
review. Running on a real client device is a separate release check, **not**
an unfinished source-code feature.
