# COLLECT progress v6.8.1

Retains the v6.7.x one-line TTY progress supervisor with live terminal-width
recalculation, bounded status text, replacement decoding for invalid UTF-8,
and line-oriented non-TTY behavior. Readonly COLLECT never uses transaction
worktrees.

v6.8.1 retains the dedicated collector process group and
SIGINT/SIGTERM forwarding from the supervisor. If an IDE/task runner terminates
only the supervisor PID, the collector tree is terminated as well instead of
being left behind. A non-exiting child is escalated after a short grace period.

The normal user command remains `./tools/run_python_patches.sh` with no
arguments. Manual COLLECT subcommands are internal dispatcher details and must not be
presented by AI-generated instructions as the normal workflow.

v6.8.1 retains canonical successful completion output. A collector that reports the
same result archive more than once is reduced to one highlighted
`[PRIMARY - UPLOAD THIS FILE]` path. The request archive is shown separately as
informational metadata. Completion paths are also suppressed from the live
heartbeat detail so captured terminal output does not accidentally duplicate
the upload target.
