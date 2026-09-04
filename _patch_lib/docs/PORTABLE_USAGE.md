# Python Patch Tool v6.19.3 portable usage

The release is self-contained for its v6.19.2 documented PATCH/COLLECT contract. Put PATCH or `CODE_COLLECTION_REQUEST_*.zip` directly under `<project>/patchs/`; all platforms use the same queue and Python core.

## v6.18.8 HISTORY/report visual priority

When a report is shown directly or reopened from HISTORY, AI-facing artifacts are visually prioritized on ANSI-capable terminals: `COLLECT result`, `FAIL handoff`, and `Recovery COLLECT` use the bright-yellow upload style and existing paths are underlined. Missing AI-facing artifacts use the failure warning palette. `INCOMPLETE`/`PREFLIGHT_FAIL` status is also emphasized. `NO_COLOR` and non-TTY output remain plain and exact artifact paths are never clipped.


## Linux / POSIX

Install/update at the project root:

```bash
unzip -o python_patch_tool_v6.19.2.zip -d "$PWD"
./tools/run_python_patches.sh
```

> **Zero-work HISTORY safety:** HISTORY landing is optional/read-only. If an existing artifact/history path is unsafe (for example symlink/reparse), zero-work must warn and skip HISTORY rather than fail the no-work invocation; operations that actually consume or mutate recovery artifacts remain fail-closed.


## Windows

Requirement: **Python 3.10+**. The launcher accepts Python Launcher (`py -3`) or a `python` / `python3` command on PATH.

PowerShell install/update at the project root:

```powershell
Expand-Archive -Force .\python_patch_tool_v6.19.2.zip .
tools\run_python_patches.bat
```

Recommended normal command from either CMD or PowerShell:

```bat
tools\run_python_patches.bat
```

Direct PowerShell alternative:

```powershell
.\tools\run_python_patches.ps1
```

The BAT wrapper starts the packaged PowerShell launcher with process-local `-ExecutionPolicy Bypass`; it does **not** change the machine/user ExecutionPolicy setting.

On a native Windows console, v6.17.5 uses the fullscreen selector when `msvcrt` input and VT output are available: ↑/↓, Space, priorities 0–9, `a`, `n`, `d`, `i`, `v`, `h`, Enter, q/Esc. Unsupported/non-TTY consoles automatically fall back to the stable line selector (`1`, `1,3-5`, `a`, `d 2`, `i 1`, `v 1`, `h`, `q`). PATCH/COLLECT rules are otherwise unchanged.

## AI workflow

Before working with AI, send all current `tools/_patch_lib/docs/`. For Patch Tool development also send `tools/implementing.md` and `tools/PYTHON_PATCH_TOOL_FEATURES_VI.md`.

COLLECT actions are defined exclusively by `docs/COLLECT_ACTION_SCHEMA.json`: the authoritative current/restored set in the schema, including `pack`, `overview`, `find`, `search`, `git`, historical read-only actions, and the compatibility aliases `search_files`, `content`, `symbol_graph`.

PATCH package construction must follow `PATCH_PACKAGE_SCHEMA.json` and `PATCH_PACKAGE_GUIDE.md`. External `post_patch.commands[].argv` executables must exist on the current OS; a Linux-only `bash`/`sh` command is not automatically translated on Windows.

## v6.18.4 zero-work HISTORY and validation compatibility

Running the zero-argument launcher with no runnable PATCH/COLLECT lands on HISTORY. Native interactive terminals get the browser; captured IDE/task output prints the bounded HISTORY list and returns immediately instead of blocking on stdin. This is not a run and does not create/overwrite LAST_RUN/history/ledger state.

Trusted local validation may auto-select profiles from actual changed paths via `.python_patch_tool.json` `validation.selection`. Per-profile `diagnostic_rerun` is bounded diagnostic evidence only. Use `run --no-validation` or direct `--patch ... --no-validation` only when explicitly disabling both requested and auto-selected validation for that invocation.

## Recovery / audit

Failed PATCH runs write `artifacts/patch_tool/LAST_RUN.json` and always attempt a `fail_handoffs/FAIL_HANDOFF_*.zip`; every FAIL handoff automatically bundles bounded related source plus `SOURCE_DISCOVERY.json`. Source drift can additionally prepare a next-run COLLECT request. Interactive PATCH inspect is `i` (line selector: `i <index>`), read-only validate is `v` (line selector: `v <index>`), and Tool Health is `h`. Direct validation is `tools\run_python_patches.bat validate --patch patchs\example.zip` on Windows or `./tools/run_python_patches.sh validate --patch patchs/example.zip` on POSIX.

When multiple PATCHes are selected, read-only batch preflight validates the selected set before the first source write. Under the default `continue_independent` + `transaction=patch`, an item-local `PREFLIGHT_FAIL` rejects only that PATCH: dependency/effective-target-related successors become `BLOCKED`, while unrelated PATCHes continue. Global planning/resource/transaction failures, explicit `fail_fast`, and atomic `transaction=batch` remain whole-batch fail-closed. Ctrl+C, rollback failure, or partial/unknown project state still safety-stop. `NOT_EXECUTED` is reserved for work genuinely not attempted because a global/explicit stop occurred.

If the previous run left a failed PATCH in `patchs/`, a successor generated from its FAIL_HANDOFF must declare `batch.previous_failure` with an explicit `delete`, `retry_before`, `run_after`, or `block` action and a reason. This prevents orphan failed PATCHes. Smart Resume is available with `resume --resume-mode all|failed|remaining`.

Per-PATCH detail logs plus `SUMMARY.txt`, aggregate `batch.log`, and declared-target source diffs are retained under `artifacts/patch_tool/runs/<run_id>/`. Reopen a run with `report`; use `report --list`, `--pin`, `--unpin`, `--export`, `--delete`, or `--cleanup` to manage history. `report --run-id <id> --support-item <N>` exports a focused support ZIP for one report item.

For native Windows verification of the actual BAT/PowerShell runtime, run `.\tools\run_windows_native_tests.ps1` on a Windows host. The release host must not claim this lane passed natively unless it was actually executed on Windows.

Interactive Smart Resume in v6.17.5 uses Up/Down + Enter with a live description. Retry, failed-source COLLECT, and safe Delete-to-`patchs/ignore` support failed-PATCH multi-select with Space/a/n; multiple recovery COLLECT requests execute sequentially.

## Read-only planning and reproducible recipes (v6.17.7)

```bash
./tools/run_python_patches.sh plan
./tools/run_python_patches.sh plan --export-recipe BATCH_RECIPE.json
./tools/run_python_patches.sh run --recipe BATCH_RECIPE.json
```

Windows:

```powershell
.\tools\run_python_patches.ps1 plan --export-recipe BATCH_RECIPE.json
.\tools\run_python_patches.ps1 run --recipe BATCH_RECIPE.json
```

`plan` does not mutate project source. OPS previews run on a private mirror; arbitrary Python payloads are never executed for preview. Recipe execution requires exact queued package filename + SHA-256 + patch id and, when recorded, the same local project key.

## v6.17.8 managed command note

PATCH payload, post-patch, trusted validation and failure-only commands are automation-only and receive closed stdin. Do not write PATCH packages that wait for interactive confirmation. Timeout/interruption is process-tree contained before control returns to rollback/reporting. On Windows, the packaged implementation uses new process groups + CTRL_BREAK/taskkill fallback; run `tools\\run_windows_native_tests.ps1` on a real Windows host before claiming native verification for a release.

POSIX `run_python_patches.sh` now performs the same Python 3.10+ gate as the Windows launcher and fails with a clear rc=2 message before importing the tool on an older interpreter.


## v6.17.10 policy/config notes

- `.python_patch_tool.json` is parsed with one bounded non-symlink duplicate-key-safe contract across selector, batch policy, project identity and validation profiles. If the file exists but is malformed/unsafe, `run`/`resume`/`plan` fail closed instead of silently falling back to default policies.
- `plan --failure-policy ... --transaction-policy ...` applies those overrides; exported recipes preserve the effective policies exactly.
- A plan using `transaction=batch` performs the same transaction-compatibility gate as execution before preview/export.
- Cross-run unresolved failures constrain only related successors; unrelated queued PATCHes remain eligible. Multiple related unresolved predecessors must be resolved/retried via Smart Resume before a singular `batch.previous_failure` successor batch can proceed.


Recipe policy override rule: `run --recipe` uses the policies stored in the recipe; `--failure-policy`/`--transaction-policy` must not be combined with `--recipe`. Create a new recipe with `plan` overrides when different policies are intended.

## v6.17.12 zero-argument history and live status

- Interactive zero-argument selection always exposes a `HISTORY` row. Use Up/Down + Enter; returning to the queue preserves the current PATCH selection.
- If no runnable PATCH/COLLECT remains, warnings, `AUTO STATUS: IDLE` and Tool Health are printed first, then the interactive zero-argument launcher opens history automatically. The default row is the newest meaningful PASS run.
- Enter on a history run opens the existing report browser and immediately prints an `Important files` block with **absolute paths** for COLLECT result/request ZIPs, FAIL_HANDOFF, recovery/replay/archive packages and important diagnosis logs. Historical artifacts that were cleaned remain visible as paths marked `[missing]`; item detail, aggregate log, source diff and support bundle actions remain available from the same report menu.
- PATCH execution uses a best-effort fixed status header only on a suitable interactive terminal. Redirected output, dumb/tiny terminals, unsupported Windows VT or resize/render failures fall back to normal scrolling output. Set `PTV_DISABLE_LIVE_STATUS=1` to opt out. Stored logs remain raw/authoritative.



## v6.17.13 history and resume clarification

- Interactive history hides IDLE probes and displays package/request name first, then local run timestamp, then final status. New IDLE invocations update `LAST_RUN.json` but are not appended to `history/*.json`.
- If duplicate filtering consumes every queued candidate, the tool prints a queue-cleanup summary and waits for Enter before opening history, so automatic removals are visible. A queue that was truly empty from the start still opens history automatically in zero-argument interactive mode.
- Automatic SMART RESUME requires the immediately recorded `LAST_RUN` to be FAIL **and** at least one replay/failed/remaining item from that run to still be runnable in the current queue. Older unresolved failures remain persistent planner constraints for related successors, but never globally hijack unrelated new queue work.
- Report item detail is selected by a numeric index (`1..N`), not the literal letter `N`.

- Old unpinned IDLE probes are removed first during history cleanup, so they do not consume the 30 meaningful-run retention budget.

- History timestamps are displayed in the machine local timezone; persisted JSON timestamps remain UTC.


## v6.17.14 zero-work and SMART RESUME correction

- **Historical v6.17.14 behavior — superseded by v6.18.1 and retained here only for chronology.** If discovery finds no runnable PATCH/COLLECT, warnings, `AUTO STATUS: IDLE` and Tool Health are printed and the zero-argument launcher exits `0` immediately. The invocation is not a run: it creates no run directory and does not write `LAST_RUN`, history, ledger or unresolved-failure state; it also does not auto-open HISTORY.
- If runnable candidates existed but duplicate/session filtering removes all of them, `QUEUE CLEANUP SUMMARY` is shown and Enter may open HISTORY so the operator can see what was removed. This cleanup-only invocation still does not create a run report.
- Automatic SMART RESUME requires `LAST_RUN` itself to be FAIL **and** at least one replay/failed/remaining item from that exact run to still exist in the current runnable queue. Older unresolved failures remain planner constraints only for related successors and never force the startup recovery menu in front of unrelated new PATCH/COLLECT work.


## v6.18.0 search health

Run `./tools/run_python_patches.sh health-search` (or the equivalent PowerShell/BAT launcher command) to validate discovery/search independently of project source. Search is filesystem-first by default and does not use Git tracking as an implicit scope.

## v6.18.1 empty-queue behavior

Running the public launcher with no arguments on an interactive terminal and no runnable PATCH/COLLECT prints discovery warnings, IDLE status and Tool Health, then opens existing HISTORY. This does not create an IDLE run or modify LAST_RUN/history state.

## v6.18.2 compatibility CLI

Non-interactive historical automation is supported again by the public launcher:

```text
./tools/run_python_patches.sh --all
./tools/run_python_patches.sh --patch patch_a.zip --patch patch_b.zip
./tools/run_python_patches.sh --select 1,3-5
./tools/run_python_patches.sh -a -y --zip-failed --keep-failed-zip --move
```

The last form preserves historical automation flags; `--move` is compatibility syntax because successful current queue runs already archive their inputs. Before modifying Patch Tool itself, read `NO_SILENT_REMOVAL_POLICY.md` and `CAPABILITY_LEDGER.md`.


## v6.18.3 COLLECT capability restoration

The public workflow is unchanged: put one `CODE_COLLECTION_REQUEST_*.zip` in `patchs/` and run the zero-argument launcher. The old direct `collect <command>` CLI remains superseded.

The request schema again supports the historical read-only action families (`ls`, `tree`, `research`, file/range/head/tail, symbol/references, callgraph/dependencies, directory and bounded decompile aliases) plus the M3 compatibility aliases `search_files`, `content`, and `symbol_graph`. `search_files` and `content` use the same coverage-aware search implementation as `search`; a zero match is not evidence of absence unless coverage is VERIFIED.

`pack` intentionally remains exact-file evidence; use `directory` when a bounded subtree is required. See `CODE_COLLECTION_GUIDE.md` and `COLLECT_ACTION_SCHEMA.json` for the exact current fields.

## Optional controlled migration helper

Normal use does **not** require an installer. If an old project still has obsolete loose Patch Tool files under `tools/`, use the bounded helper:

```bash
python3 tools/_patch_lib/install_python_patch_tool_v6.py --project-root "$PWD" --dry-run
python3 tools/_patch_lib/install_python_patch_tool_v6.py --project-root "$PWD"
```

To create a safe current `.python_patch_tool.json` only when none exists:

```bash
python3 tools/_patch_lib/install_python_patch_tool_v6.py --project-root "$PWD" --create-config
```

The historical path `install_python_patch_tool_v5.py` remains as a compatibility wrapper. The helper never overwrites an existing config and only backs up/removes a fixed allowlist of obsolete Patch-Tool-managed loose files; unrelated project files under `tools/` are left untouched.

## v6.19.0 database SELECT profiles

Database evidence uses the normal zero-argument COLLECT workflow. AI-generated request ZIPs reference only a `database_select.profile` name and structured active-builder fields. Configure the local machine separately using `tools/db_profiles.local.json` (template: `tools/db_profiles.example.json`). Raw SQL and password fields are not supported. See `_patch_lib/docs/DATABASE_SELECT_ACTIVE_BUILDER.md`.
