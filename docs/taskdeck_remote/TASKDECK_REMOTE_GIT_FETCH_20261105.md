# TaskDeck Remote Git fetch — fixed upstream-only E2EE (2026-11-05)

This milestone adds **Git fetch (upstream)** to the independently pinned
native TaskDeck Remote GUI. The PHP Portal and Relay forward only opaque
encrypted records. No browser-supplied Git URL, remote, branch, revision,
shell expression, environment, credentials or refspec is ever executed.

## Host action and safety boundaries

- Host must explicitly set `TASKDECK_REMOTE_GIT_FETCH=1` (default OFF).
- The user requires a real TaskDeck password principal, approved client
  key, fresh local `git.write` permission and live Portal grant for each
  action. The application dispatcher performs authorization before/after.
- Requires the exact, canonical host-selected workspace Git root and an
  attached branch with an existing configured same-name upstream.
- Resolves and validates a **single** effective SSH or HTTPS fetch URL.
  Local/file/ext/git/unknown protocols and multiple destinations are denied.
- A fixed bounded Git command fetches only
  `refs/heads/<current_branch>:refs/remotes/<configured_remote>/<current_branch>`.
  The refspec contains no leading `+`, preventing non-fast-forward
  tracking-branch updates. It neither downloads tags/submodules nor
  changes HEAD, working files or index. A rewritten remote branch is denied.
- No shell, hooks, Git environment overrides, credential output, arbitrary
  subprocess nor browser-origin SSH options are supported. The Git command
  is cancelled on session revocation, with a nine-second timeout.
- Mandatory TaskDeck-local `remote.action.git_fetch` **attempted** audit is
  durable before the network mutation. The audit excludes URL, branch,
  credentials, file paths and command output.

After success the authenticated E2EE response reports only current local
HEAD and upstream-tracking commit IDs before/after, plus a consistent
`updated` boolean. The GUI invalidates cached Git Status and requires a
new read to show current branch/tracking divergence.

## Tests

Go tests cover strict E2EE request/reply integrity, replay/duplicate and
unknown-key rejection, default-off and viewer denial, invalid remote
schemes and multiple URLs, wrong upstream, canonical workspace root,
an actual fast-forward fetch into a temporary tracking branch using a
**test-only fake SSH transport**, idempotent repeat, unchanged HEAD and
refusal of a rewritten upstream (no force).

Native loopback route tests deny caller-selected URLs, branches, query
arguments and cross-origin requests. JavaScript tests exercise feature
flag isolation, user confirmation, no request body, result validation and
logout disabling. The Go Agent CI runs all scoped tests with the race
detector; the PHP Portal CI is independent.

## Incomplete code backlog

- Git stage/unstage and further branch/revision controls, full diff paging.
- Longer-lived Terminal streaming/reconnect enhancements beyond bounded PTY.
- Advanced task dependencies, secret input and cross-session output replay.
- Remaining editor/files GUI and other remote workflows.
- No Windows-specific support yet by earlier project decision.

Completing source code does not require live-machine acceptance to count as
implemented, but it does not waive production security/release review.
