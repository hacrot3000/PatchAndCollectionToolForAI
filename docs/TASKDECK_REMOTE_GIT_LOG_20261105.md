# TaskDeck Remote Git log — fixed E2EE read (2026-11-05)

The independently trusted native Remote GUI now has a **Git log (3 commit)**
button. It calls the exact `git.log` E2EE operation. The PHP Portal and
Relay only forward opaque encrypted records; they cannot supply a repository,
branch, arbitrary argv, command, rev-list expression or output path.

The host must separately enable `TASKDECK_REMOTE_GIT_LOG=1` (default OFF).
Every request requires the current TaskDeck principal's `git.log`
permission, approved software key, live Portal grant, project ownership
and existing audited E2EE session. The exact canonical workspace Git root
is checked so a project subfolder cannot discover a parent repository.

The host runs one fixed Git read, equivalent to:
`git --no-pager log --max-count=3 --no-show-signature --no-decorate
--pretty=format:%h\ %s`.
It uses no shell; Git read-only optional locks are disabled, optional
fsmonitor and pager execution are prohibited, and inherited `GIT_*`
overrides are discarded. Output is capped at **240 UTF-8 bytes** and must
contain no terminal escape/control codes. An oversized or malformed log is
denied, not truncated into a misleading partial history. The E2EE decoder
refuses unknown/duplicate keys, forged capabilities and response IDs.

The native GUI renders output with `textContent`, not HTML. Its feature
flag is optional for compatibility with older Agents, but is never used
as authorization on the host; every action is rechecked.

Go tests cover a fixed native E2EE exchange, malformed schema and attempts
to inject arguments, three real local commits, disabled host opt-in and
parent-repository root confinement. Native local-route tests and JavaScript
asset checks verify the bounded button and API.

**Next Git work:** fixed git.diff, fetch, stage/unstage and other Git
operations. Terminal PTY and Git pull/commit/push already have bounded
source-code paths. They remain disabled by default and require independent
production security review. Unfinished code also includes advanced task
dependencies/secrets/cross-session output, further GUI screens and remaining
remote functionality. Live-device use alone is not counted as missing code.
