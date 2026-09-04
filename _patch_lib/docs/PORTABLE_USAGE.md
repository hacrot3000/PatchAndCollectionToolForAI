# Python Patch Tool v6.17.2 portable usage

The release is self-contained for its v6.17.2 documented PATCH/COLLECT contract. Put PATCH or `CODE_COLLECTION_REQUEST_*.zip` directly under `<project>/patchs/`; all platforms use the same queue and Python core.

## Linux / POSIX

Install/update at the project root:

```bash
unzip -o python_patch_tool_v6.17.2.zip -d "$PWD"
./tools/run_python_patches.sh
```

## Windows

Requirement: **Python 3.10+**. The launcher accepts Python Launcher (`py -3`) or a `python` / `python3` command on PATH.

PowerShell install/update at the project root:

```powershell
Expand-Archive -Force .\python_patch_tool_v6.17.2.zip .
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

On a native Windows console, v6.17.2 uses the fullscreen selector when `msvcrt` input and VT output are available: ↑/↓, Space, priorities 0–9, `a`, `n`, `d`, `i`, `v`, `h`, Enter, q/Esc. Unsupported/non-TTY consoles automatically fall back to the stable line selector (`1`, `1,3-5`, `a`, `d 2`, `i 1`, `v 1`, `h`, `q`). PATCH/COLLECT rules are otherwise unchanged.

## AI workflow

Before working with AI, send all current `tools/_patch_lib/docs/`. For Patch Tool development also send `tools/implementing.md` and `tools/PYTHON_PATCH_TOOL_FEATURES_VI.md`.

COLLECT actions are defined exclusively by `docs/COLLECT_ACTION_SCHEMA.json`: `pack`, `overview`, `find`, `search`, `git`.

PATCH package construction must follow `PATCH_PACKAGE_SCHEMA.json` and `PATCH_PACKAGE_GUIDE.md`. External `post_patch.commands[].argv` executables must exist on the current OS; a Linux-only `bash`/`sh` command is not automatically translated on Windows.

## Recovery / audit

Failed PATCH runs write `artifacts/patch_tool/LAST_RUN.json` and always attempt a `fail_handoffs/FAIL_HANDOFF_*.zip`; every FAIL handoff automatically bundles bounded related source plus `SOURCE_DISCOVERY.json`. Source drift can additionally prepare a next-run COLLECT request. Interactive PATCH inspect is `i` (line selector: `i <index>`), read-only validate is `v` (line selector: `v <index>`), and Tool Health is `h`. Direct validation is `tools\run_python_patches.bat validate --patch patchs\example.zip` on Windows or `./tools/run_python_patches.sh validate --patch patchs/example.zip` on POSIX.

When multiple PATCHes are selected, whole-batch preflight validates the selected set before the first source write. The default failure policy remains `fail_fast`; `run --failure-policy continue_independent` may continue independent PATCHes only after a failure proven safe (no partial/unknown source state, or successful per-PATCH rollback). Dependency failures are reported as `BLOCKED`; batch-preflight failures as `PREFLIGHT_FAIL`; untouched later work remains `NOT_EXECUTED`.

If the previous run left a failed PATCH in `patchs/`, a successor generated from its FAIL_HANDOFF must declare `batch.previous_failure` with an explicit `delete`, `retry_before`, `run_after`, or `block` action and a reason. This prevents orphan failed PATCHes. Smart Resume is available with `resume --resume-mode all|failed|remaining`.

Per-PATCH detail logs plus `SUMMARY.txt`, aggregate `batch.log`, and declared-target source diffs are retained under `artifacts/patch_tool/runs/<run_id>/`. Reopen a run with `report`; use `report --list`, `--pin`, `--unpin`, `--export`, `--delete`, or `--cleanup` to manage history. `report --run-id <id> --support-item <N>` exports a focused support ZIP for one report item.

For native Windows verification of the actual BAT/PowerShell runtime, run `.\tools\run_windows_native_tests.ps1` on a Windows host. The release host must not claim this lane passed natively unless it was actually executed on Windows.
