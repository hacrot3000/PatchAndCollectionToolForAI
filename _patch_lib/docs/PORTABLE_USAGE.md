# Python Patch Tool v6.12.0 portable usage

Install/update directly at project root:

```bash
unzip -o python_patch_tool_v6.12.0.zip -d "$PWD"
./tools/run_python_patches.sh
```

The release is self-contained for its v6.12.0 documented PATCH/COLLECT contract. It ships the PATCH runner, patch utilities and readonly collector; no older private core is required.

Public workflow remains zero-argument. Put PATCH or `CODE_COLLECTION_REQUEST_*.zip` under `patchs/`, then run the command above.

Before working with AI, send all current `tools/_patch_lib/docs/`. For Patch Tool development also send `tools/implementing.md` and `tools/PYTHON_PATCH_TOOL_FEATURES_VI.md`.

COLLECT actions are defined exclusively by `docs/COLLECT_ACTION_SCHEMA.json`: `pack`, `overview`, `find`, `search`, `git`.
