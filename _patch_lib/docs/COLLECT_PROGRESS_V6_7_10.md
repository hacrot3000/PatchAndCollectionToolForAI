# COLLECT progress v6.7.10

Retains the v6.7.x one-line TTY progress supervisor with live terminal-width
recalculation, bounded status text, replacement decoding for invalid UTF-8,
and line-oriented non-TTY behavior. Readonly COLLECT never uses transaction
worktrees.

## Completion artifact contract

A collector exit code of zero is not sufficient by itself. Before the final
status is rendered, the supervisor must detect the result collection ZIP,
resolve a local absolute/project-relative path, confirm that the file exists,
and confirm that it is a ZIP archive. If that contract fails, the final status
is `COLLECT rc=2`; the console must never print a green/checked rc=0 first and
then contradict it below.

Quoted result/request paths and paths containing spaces are accepted. Legacy
collectors that print the same result archive as both a labelled `ZIP:` line
and a bare path are canonicalized to one highlighted
`[PRIMARY - UPLOAD THIS FILE]` path. The archived request is shown separately.
Completion paths are suppressed from live heartbeat detail so captured output
does not duplicate the upload target.

## Stop/terminal-close cleanup

The collector runs in a dedicated process group. The supervisor installs stop
handlers before spawning the collector, closing the previous spawn/handler
race. SIGINT, SIGTERM and SIGHUP are forwarded to the collector tree. SIGQUIT
is recorded as the original supervisor stop reason but the collector tree is
terminated with SIGTERM to avoid slow/native core-dump behavior. A child that
does not exit within the grace period is escalated to SIGKILL.

The normal user command remains:

```bash
./tools/run_python_patches.sh
```

Manual COLLECT subcommands are internal dispatcher details and must not be
presented by AI-generated instructions as the normal workflow.
