# Python Patch Tool v6.17.13 portable usage

The release is self-contained for its v6.17.13 documented PATCH/COLLECT contract. Put PATCH or `CODE_COLLECTION_REQUEST_*.zip` directly under `<project>/patchs/`; all platforms use the same queue and Python core.

## Linux / POSIX

Install/update at the project root:

```bash
unzip -o python_patch_tool_v6.17.13.zip -d "$PWD"
./tools/run_python_patches.sh
```

## Windows

Requirement: **Python 3.10+**. The launcher accepts Python Launcher (`py -3`) or a `python` / `python3` command on PATH.

PowerShell install/update at the project root:

```powershell
Expand-Archive -Force .\python_patch_tool_v6.17.13.zip .
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

COLLECT actions are defined exclusively by `docs/COLLECT_ACTION_SCHEMA.json`: `pack`, `overview`, `find`, `search`, `git`.

PATCH package construction must follow `PATCH_PACKAGE_SCHEMA.json` and `PATCH_PACKAGE_GUIDE.md`. External `post_patch.commands[].argv` executables must exist on the current OS; a Linux-only `bash`/`sh` command is not automatically translated on Windows.

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
- Automatic SMART RESUME describes the latest meaningful PATCH/COLLECT failure only. Older unresolved failures remain persistent planner constraints for related successors, but do not globally claim that the immediately previous invocation failed.
- Report item detail is selected by a numeric index (`1..N`), not the literal letter `N`.

- Old unpinned IDLE probes are removed first during history cleanup, so they do not consume the 30 meaningful-run retention budget.

- History timestamps are displayed in the machine local timezone; persisted JSON timestamps remain UTC.
