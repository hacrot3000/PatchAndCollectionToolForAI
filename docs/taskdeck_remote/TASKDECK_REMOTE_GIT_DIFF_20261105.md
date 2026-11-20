# TaskDeck Remote Git diff — bounded read-only stat (2026-11-05)

This adds a fixed `git.diff` E2EE operation to the independently trusted
native TaskDeck Remote GUI (Linux/macOS). The Agent, not PHP Portal or Relay,
chooses the exact canonical workspace Git root. Client messages cannot carry
a path, revision, command, diff options, environment or repository URL.

On the host, `TASKDECK_REMOTE_GIT_DIFF=1` must be explicitly enabled
(default OFF), and the authenticated TaskDeck user must have `git.diff`
RBAC. The live Portal grant, approved client key and local TaskDeck
password principal are checked per action. Read-only audit is recorded.

The command executes a fixed `git diff --stat HEAD --` for the index and
working tree combined, with `--no-ext-diff`, `--no-textconv` and
`--no-color` and disabled optional fsmonitor. It excludes untracked
files. Neither a shell nor an external diff program is invoked. The
returned summary is capped at 240 UTF-8 bytes; oversized or malformed
results are denied rather than truncated. An unchanged working tree is
reported as `clean`. The GUI only renders the authenticated result as
text, with no HTML/ANSI interpretation.

The Go regression tests cover strict E2EE framing, forged path/revision
arguments, the actual Git stat in an isolated repository, disabled host
opt-in and prevention of parent-repository traversal. GUI/native-route
tests and the remote-agent CI also exercise the command.

Further Git work: fetch, stage/unstage, full diff pagination, branch and
history operations. Remote tasks advanced dependency/secret features,
other GUI screens and remaining remote workflows are separate code
backlog. Completed code does not await device acceptance for feature
status, but production enablement still requires security review.
