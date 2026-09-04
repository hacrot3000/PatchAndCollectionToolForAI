# Portable use — Python Patch Tool v6.7.10

Normal user entry point is always:

```bash
./tools/run_python_patches.sh
```

## MANDATORY AI contract for COLLECT requests

When an AI/ChatGPT needs more project source before it can safely create a
PATCH, the **deliverable to the user MUST be one ZIP request package**.

The rules are mandatory:

1. **NEVER give the user a loose `.json` COLLECT request.**
2. The AI must create a `.zip` file that contains **exactly one**
   `CODE_COLLECTION_REQUEST*.json` request file.
3. The ZIP itself is what the user downloads/copies into `patchs/`.
4. For normal operation, **NEVER instruct the user to invoke a manual
   COLLECT subcommand or pass a request path on the command line**. Those are
   internal routing details, not the public workflow.
5. The user runs only:

   ```bash
   ./tools/run_python_patches.sh
   ```

   The zero-argument queue discovers the ZIP and labels/routes it as
   `[COLLECT]` automatically.
6. If an AI cannot produce the ZIP artifact, it must not silently fall back to
   returning JSON as the primary file.

Correct AI output pattern:

```text
Download: CODE_COLLECTION_REQUEST_<purpose>_<timestamp>.zip

Then copy that ZIP to <project>/patchs/ and run:
./tools/run_python_patches.sh
```

Forbidden behavior is described only in prose to avoid copy/paste ambiguity:
returning a standalone request JSON, telling the user to place that JSON in
`patchs/`, or telling the user to run the launcher with COLLECT-specific
arguments are all obsolete.

The **request ZIP** above is different from the **result/source collection ZIP**
created by the readonly collector. The request ZIP tells the tool what to
collect; the result ZIP contains the collected evidence/source for the AI.

Loose `CODE_COLLECTION_REQUEST*.json` files placed in `patchs/` are rejected by
the queue on purpose.

## Permanent in-place PATCH contract

SANDBOX / detached-worktree execution is removed from the supported workflow.
Every documented PATCH execution route through the public launcher (`--patch`,
`--all`, `--select`, or a direct patch package path) strips historical
transaction flags and forces the installed compatible core to
`--transaction off`. Transaction/SANDBOX-only invocations fail closed instead
of falling through to a legacy zero-argument core.

Historical documents from v5 that recommend `transaction.mode=auto` or
`required` are obsolete for the current workflow and must not be used to
re-enable worktrees. Utility-only commands such as `paths` and `--help` are
passed through without execution-only arguments. COLLECT remains readonly.

## Queue and selector compatibility

Zero-argument discovery recognizes v5+ PATCH ZIPs by root
`PATCH_TOOL_MANIFEST.json`, documented legacy-v4 signatures, and COLLECT ZIPs.
HANDOFF/report/tool-distribution archives and symlink queue entries are skipped.
A root `PATCH_TOOL_MANIFEST.json` has precedence over any nested
`CODE_COLLECTION_REQUEST_*.json` resource so a valid PATCH cannot be routed as
COLLECT. ZIP/TAR package extension matching at the launcher is case-insensitive.

TTY selector keeps Space, arrows, `a`, `n`, `d`, Enter and q/Esc. The line
fallback is used whenever either stdin or stdout is not a TTY, or when
`automation.zero_argument.selector_ui` is `line`. It accepts numbers, lists,
ranges, all/none/quit and confirmed deletion. A single remaining item is
selected by default so Enter is enough to run it.

Previously documented zero-argument `selection=prompt|all|first|newest`,
`non_interactive_confirmed`, `initial_selection=none|all`, and
`selector_ui=auto|line` remain supported. A mixed PATCH/COLLECT queue always
falls back to user confirmation for automatic selection.

Selected work is executed in natural order and stops on the first failure.
Remaining selected items are reported as `SKIPPED / NOT EXECUTED` and remain in
the queue.

## v6.7.10 completion hardening

A zero collector exit code becomes a public PASS only when the result collection
ZIP is detected and usable. Missing/non-ZIP result artifacts fail closed before
the final status row is printed. Quoted paths and paths containing spaces are
accepted. Stop handlers are installed before collector spawn and cover common
task/terminal-close signals so the readonly child tree is not left orphaned.

Queue entries are revalidated immediately before execution. A selected entry
that has disappeared, become a symlink, or changed from PATCH/COLLECT into a
different artifact class fails closed rather than being executed from stale
selector state.
