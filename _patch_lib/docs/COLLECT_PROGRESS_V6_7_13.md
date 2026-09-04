# COLLECT progress v6.7.13

Retains the v6.7.x one-line TTY progress supervisor with live terminal-width
recalculation, bounded status text, replacement decoding for invalid UTF-8,
and line-oriented non-TTY behavior. Readonly COLLECT never uses transaction
worktrees.

## Completion artifact contract

A collector exit code of zero is not sufficient by itself. Before the final
status is rendered, the supervisor must detect the result collection ZIP,
resolve a local absolute/project-relative path, confirm that the file exists, confirm that it is a ZIP archive, and verify member CRC integrity before exposing it as the primary upload artifact. If that contract fails, the final status
is `COLLECT rc=2`; the console must never print a green/checked rc=0 first and
then contradict it below.

Quoted result/request paths and paths containing spaces are accepted. Legacy
collectors that print the same result archive as both a labelled `ZIP:` line
and a bare path are canonicalized to one highlighted
`[PRIMARY - UPLOAD THIS FILE]` path. The archived request is shown separately.
Completion paths are suppressed from live heartbeat detail so captured output
does not duplicate the upload target. ZIP-specific completion labels (`ZIP:`,
`RESULT ZIP:`, `OUTPUT ZIP:` and `ARTIFACT ZIP:`) outrank generic fallback
labels such as `FILE:` so a later unrelated debug/archive path cannot replace
the real collection result. The latest result path and latest archived-request path are retained in separate
completion slots rather than a shared FIFO or the bounded failure tail. Repeated
request metadata therefore cannot evict an earlier valid result path, and a
collector may print hundreds of later lines without turning a valid rc=0 into a
false missing-artifact failure.

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

Manual COLLECT subcommands are rejected by the public launcher. The
zero-argument dispatcher invokes the readonly supervisor internally; AI-generated
instructions must expose only the zero-argument workflow.
