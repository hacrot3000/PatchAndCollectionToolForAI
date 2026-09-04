# Python Patch Tool v6.18.4 — portable layout and migration

## Primary installation: extract and run

From the project root:

```bash
unzip -o python_patch_tool_v6.18.4.zip -d "$PWD"
./tools/run_python_patches.sh
```

The release already contains final project-relative paths under `tools/`. No installer is required for normal installation or upgrade. The release does not contain `.python_patch_tool.json`, so project-local configuration is not overwritten by extraction.

## Public/private layout

```text
project/
└── tools/
    ├── run_python_patches.sh
    ├── run_python_patches.bat
    ├── run_python_patches.ps1
    ├── implementing.md
    ├── PYTHON_PATCH_TOOL_FEATURES_VI.md
    ├── HUONG_DAN_PYTHON_PATCH_TOOL.html
    └── _patch_lib/
        ├── python_patch_queue_dispatcher.py
        ├── python_patch_runner.py
        ├── python_patch_utils.py
        ├── install_python_patch_tool_v6.py      # optional controlled helper
        ├── install_python_patch_tool_v5.py      # historical filename wrapper
        ├── docs/
        └── SHA256SUMS
```

`tools/run_python_patches.sh` (or the Windows wrapper) is the normal public runtime entry point. Modules under `_patch_lib/` are internal/maintenance utilities.

## Optional controlled migration

Use only when an older project still has obsolete Patch-Tool-managed loose files or when you explicitly want a safe default config created:

```bash
python3 tools/_patch_lib/install_python_patch_tool_v6.py --project-root "$PWD" --dry-run
python3 tools/_patch_lib/install_python_patch_tool_v6.py --project-root "$PWD"
python3 tools/_patch_lib/install_python_patch_tool_v6.py --project-root "$PWD" --create-config
```

Safety contract:

- only an exact fixed list of historical Patch-Tool-managed loose files may be migrated;
- every migrated file is copied to `artifacts/patch_tool/installer_backups/...` before deletion;
- symlinks/non-regular files fail closed;
- unrelated `tools/*` files are untouched;
- an existing `.python_patch_tool.json` is never overwritten;
- `--create-config` only creates a missing config with prompt/non-interactive-safe defaults;
- no network operation, Git mutation, project-source mutation or arbitrary cleanup occurs.

The historical `install_python_patch_tool_v5.py` filename remains a thin wrapper around the current helper so old maintenance instructions do not fail solely because of a filename change.

## Compatibility governance

Before changing package layout or removing migration helpers, read `NO_SILENT_REMOVAL_POLICY.md`, `CAPABILITY_LEDGER.md`, `HISTORICAL_FEATURE_BASELINE_V5_15.md`, and `HISTORICAL_FEATURE_STATUS_V5_15.json`. Historical capability #81 was COMPLETE in v5.15 and is restored in v6.18.3; it must not silently disappear again.
