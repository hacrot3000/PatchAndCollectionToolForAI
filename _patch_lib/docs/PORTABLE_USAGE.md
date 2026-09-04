# Python Patch Tool v6.14.2 portable usage

The release is self-contained for its v6.14.2 documented PATCH/COLLECT contract. Put PATCH or `CODE_COLLECTION_REQUEST_*.zip` directly under `<project>/patchs/`; all platforms use the same queue and Python core.

## Linux / POSIX

Install/update at the project root:

```bash
unzip -o python_patch_tool_v6.14.2.zip -d "$PWD"
./tools/run_python_patches.sh
```

## Windows

Requirement: **Python 3.10+**. The launcher accepts Python Launcher (`py -3`) or a `python` / `python3` command on PATH.

PowerShell install/update at the project root:

```powershell
Expand-Archive -Force .\python_patch_tool_v6.14.2.zip .
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

Windows uses the line selector because the fullscreen selector is POSIX/`termios` based. Typical inputs are `1`, `1,3-5`, `a`, `d 2`, `i 1`, `h`, `q`, and Enter to confirm an existing selection. PATCH/COLLECT rules are otherwise unchanged.

## AI workflow

Before working with AI, send all current `tools/_patch_lib/docs/`. For Patch Tool development also send `tools/implementing.md` and `tools/PYTHON_PATCH_TOOL_FEATURES_VI.md`.

COLLECT actions are defined exclusively by `docs/COLLECT_ACTION_SCHEMA.json`: `pack`, `overview`, `find`, `search`, `git`.

PATCH package construction must follow `PATCH_PACKAGE_SCHEMA.json` and `PATCH_PACKAGE_GUIDE.md`. External `post_patch.commands[].argv` executables must exist on the current OS; a Linux-only `bash`/`sh` command is not automatically translated on Windows.

## Recovery / audit

Failed PATCH runs write `artifacts/patch_tool/LAST_RUN.json` and normally a `fail_handoffs/FAIL_HANDOFF_*.zip`; source drift can prepare a next-run COLLECT request. Interactive PATCH inspect is `i` (line selector: `i <index>`), and Tool Health is `h`.
