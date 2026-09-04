# COLLECT progress v6.10.0

Retains the v6.7.x one-line TTY progress supervisor with live terminal-width
recalculation, bounded status text, replacement decoding for invalid UTF-8,
and line-oriented non-TTY behavior. Readonly COLLECT never uses transaction
worktrees.

v6.10.0 retains the dedicated collector process group and
SIGINT/SIGTERM forwarding from the supervisor. If an IDE/task runner terminates
only the supervisor PID, the collector tree is terminated as well instead of
being left behind. A non-exiting child is escalated after a short grace period.

The normal user command remains `./tools/run_python_patches.sh` with no
arguments. Manual COLLECT subcommands are internal dispatcher details and must not be
presented by AI-generated instructions as the normal workflow.

v6.10.0 retains canonical successful completion output. A collector that reports the
same result archive more than once is reduced to one highlighted
`[PRIMARY - UPLOAD THIS FILE]` path. The request archive is shown separately as
informational metadata. Completion paths are also suppressed from the live
heartbeat detail so captured terminal output does not accidentally duplicate
the upload target.

## v6.10.0 post-exit drain repair

The supervisor does not treat collector-process exit as equivalent to reader
EOF. It continues draining buffered stdout for a bounded grace period, which
prevents noisy/large collectors from losing their final `ZIP : ...` line and
being converted from a real PASS into a false `rc=3`. If a descendant keeps
the stdout pipe open after the parent collector has exited, the supervisor
cleans the lingering process group after the grace period and emits one warning.

The result/request completion metadata is tracked independently from the bounded
120-line diagnostic tail. Therefore a valid result ZIP remains available even
if the collector prints hundreds of ordinary lines after announcing it.

## v6.10.0 drain-window signal repair

Signal forwarding remains active until the post-exit stdout drain and descendant
cleanup are complete. If the collector parent has already exited but a child
still owns stdout, a supervisor-only SIGINT/SIGTERM is forwarded to the original
collector process group instead of terminating only the supervisor and leaving
an orphan. Final shell status remains normalized to 130/143.


## v6.10.0 UI regression repair
- Fullscreen selector rows are clipped to terminal cell width before ANSI styling; the current row is bold/reverse highlighted and the header always shows `CON TRỎ i/N`, preventing loss of position with very long filenames.
- Successful COLLECT uses a high-contrast `ACTION REQUIRED` upload banner. The verified result ZIP path is printed exactly once; the archived request remains informational.

## v6.10.0 narrow-terminal completion banner

The completion banner uses the live terminal width without a hard minimum that
can exceed the screen. Decorative rule/action rows are clipped to the available
cells. The verified artifact path itself remains complete for copy/upload and
may naturally wrap only after progress rendering has finished.
