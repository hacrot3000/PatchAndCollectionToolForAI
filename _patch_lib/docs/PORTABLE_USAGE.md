# Portable use — Python Patch Tool v6.9.3

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

TTY selector keeps Space, arrows, `a`, `n`, `d`, Enter and q/Esc, and adds
explicit execution priority digits `0` through `9` for the current row. Pressing
a digit selects that row and displays `[0]`..`[9]`. Lower priorities execute
first; equal priorities preserve the queue order already shown by the tool.
Rows selected normally with Space display `[x]` and, when mixed with numbered
rows, execute after all explicit priorities while preserving their existing
queue order. Space on a numbered row first returns it to plain `[x]`; another
Space deselects it. `a` selects all rows as `[x]`; `n` clears both selection
and priority. Deleting a row reindexes its selection/priority metadata with the
remaining rows.

The line fallback is used whenever either stdin or stdout is not a TTY, or when
`automation.zero_argument.selector_ui` is `line`. It keeps the historical
number/list/range item-selection grammar, plus all/none/quit and confirmed
deletion. A single remaining item is selected by default so Enter is enough to
run it.

Previously documented zero-argument `selection=prompt|all|first|newest`,
`non_interactive_confirmed`, `initial_selection=none|all`, and
`selector_ui=auto|line` remain supported. A mixed PATCH/COLLECT queue always
falls back to user confirmation for automatic selection.

Selected work is executed in natural order and stops on the first failure.
Remaining selected items are reported as `SKIPPED / NOT EXECUTED` and remain in
the queue.


## Local-only duplicate PATCH handling

Before the normal zero-argument selector is shown, PATCH candidates are compared
only with direct regular files already present in this project's
`patchs/patched/` directory. Exact SHA-256 package content is authoritative.

- Identical bytes under another filename are still a local duplicate.
- The same filename with different bytes is **not** a duplicate and remains runnable.
- No PROJECT KEY, network service, shared database, Git remote, or history from
  another machine participates in the decision. The same PATCH can therefore be
  run independently on multiple machines/projects.
- A duplicate is **skip-only**: its queue file remains untouched in `patchs/`.
- The console records the reason as:

  ```text
  PATCHES SKIPPED / NOT EXECUTED:
  1. [SKIPPED:DUPLICATE_LOCAL] <patch-name.zip>
     Local match: patchs/patched/<historical-name.zip>
  ```

Duplicate PATCHes are removed from the runnable selector set before selection,
so they cannot be executed accidentally by the normal zero-argument workflow.

## v6.9.3 queue/result correctness

PATCH child termination by signal is reported using normal shell codes (for
example Ctrl+C = 130 and SIGTERM = 143), not negative Python subprocess codes.
Line-selector Ctrl+C cancels without a traceback. A normal zero-argument COLLECT
PASS is accepted only if the request ZIP has actually moved from `patchs/` to
`patchs/patched/`. If a legacy collector prints more than one candidate result
ZIP, the supervisor validates candidates newest-first and highlights exactly one
valid upload ZIP.

### v6.9.3 local-history boundary

Duplicate suppression never follows a symlinked `patchs/` or
`patchs/patched/`. Shared/external history is ignored with a warning, because
local-only means the physical history directory belongs to this project root.
The dispatcher also rechecks duplicate status immediately before each selected
PATCH launch so a just-completed identical local PATCH can suppress a later
copy in the same run.


## v6.9.3 regression notes

- A support HANDOFF may contain the original `CODE_COLLECTION_REQUEST*.json` as
  evidence. Structural HANDOFF identity wins over COLLECT discovery, so such a
  bundle is skipped instead of rerunning the preserved request.
- PASS summaries distinguish completed items from local duplicate skips.
- COLLECT waits for bounded post-exit stdout drain before deciding whether a
  valid result ZIP was reported. Result/request metadata is retained separately
  from the bounded diagnostic tail, so long trailing logs do not hide it.


V6.9.3 in-place boundary hardening:
  Historical/short PATCH execution flags such as `-a -y --move` are treated as
  execution-capable and receive `--transaction off`. Only documented read-only
  utility routes (`paths`, help, version) bypass the execution-only argument.

## v6.9.3 selector width and concurrent-run safety

The fullscreen selector clips every rendered row to the current terminal cell
width (including double-width CJK glyphs). This prevents long package names from
wrapping and shifting the cursor during redraw. Clipping is display-only; the
full filename remains the execution identity.

Only one zero-argument queue session may own a project at a time. If the same
project is started twice concurrently, the later session reports `BUSY` and
executes nothing. The lock is local to that project and does not participate in
cross-machine or cross-project duplicate history.

- Release packaging preserves executable mode on `tools/run_python_patches.sh`; clean extraction is tested before release.

## v6.9.3 selector viewport and lock-path repair

Fullscreen selection is bounded in both terminal dimensions. Long names are
clipped by live cell width, and long queues are rendered through a cursor-centered
viewport that never exceeds `terminal_height - 1` physical rows. The viewport is
display-only: selection indexes, priorities, natural order, deletion mapping and
execution identities remain unchanged. This prevents scroll-induced duplicated
or overwritten selector rows on short terminals.

The project-local zero-argument queue lock is opened without following symlinks
and without writing/truncating the lock inode. A symlinked/hardlinked unsafe lock
path fails closed as `QUEUE LOCK` error; genuine lock contention remains `BUSY`.
This hardens the existing local concurrency guard and does not create shared,
PROJECT KEY, network or cross-machine state.

COLLECT signal forwarding stays installed through the bounded post-exit drain.
A signal delivered after the collector parent exits is still forwarded to any
stdout-holding descendant process group, preventing orphan collectors.
