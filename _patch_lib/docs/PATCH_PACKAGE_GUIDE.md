# PATCH PACKAGE GUIDE — v6.13.0 authoritative AI/tool contract

Machine-readable source of truth:

```text
tools/_patch_lib/docs/PATCH_PACKAGE_SCHEMA.json
```

AI must read that schema before generating a PATCH package. Do not invent manifest fields.

## Package shape

Preferred archive PATCH:

```text
patch_x.zip
├── PATCH_TOOL_MANIFEST.json
├── exactly-one Python entrypoint.py
└── resources/...                    # optional
```

or data-only OPS:

```text
patch_x.zip
├── PATCH_TOOL_MANIFEST.json
└── PATCH_TOOL_OPS.json
```

A package with both OPS and Python entrypoint, multiple Python entrypoints, unsafe archive members, or missing root manifest is rejected **before project modification**.

## Compatibility

Optional manifest block:

```json
{
  "compatibility": {
    "min_tool_version": "6.13.0",
    "max_tool_version": "7.0.0",
    "max_tested_version": "6.13.0"
  }
}
```

- `min_tool_version` / `max_tool_version` are hard execution gates.
- `max_tested_version` is a warning boundary, not a hard block.
- Versions are exact numeric semantic versions `X.Y.Z`.

## Targets and preflight

When AI knows the files a patch can modify, declare them:

```json
{
  "targets": ["src/a.c", "include/a.h"]
}
```

This improves failure/partial-modification diagnostics even for Python patches.

For exact source assumptions, declare preflight evidence:

```json
{
  "preflight": {
    "files": [
      {
        "path": "src/a.c",
        "sha256": "<64 hex>",
        "anchors": ["exact source anchor"]
      }
    ]
  }
}
```

If SHA/anchor/existence does not match, Patch Tool prints:

```text
PREFLIGHT FAIL — project unchanged
```

and may prepare a `CODE_COLLECTION_REQUEST_patch_recovery_*.zip` for the next invocation.

`preflight.require_clean_worktree=true` may be used only when a clean Git worktree is genuinely required by the patch.

## Required package resources

If a Python entrypoint requires packaged data, list it:

```json
{"resources":["resources/final/foo.c"]}
```

Missing/non-regular required resources are rejected before the payload runs.

## Recovery policy

Default behavior on PATCH failure is to generate a structured FAIL handoff. Source drift/anchor mismatch also prepares a read-only `pack` COLLECT request when safe source paths are known.

A package may explicitly disable either behavior:

```json
{
  "recovery": {
    "fail_handoff": false,
    "collect_on_source_drift": false
  }
}
```

Do not disable them without a concrete reason.

## No guessed rollback

v6.13.0 detects whether a failed PATCH already changed the project and records affected paths when evidence is available. It does **not** invent an automatic rollback strategy. Existing OPS helpers may create their documented backups, but a generic Python patch is not silently reversed.

## Inspect / dry-run

In the interactive selector the user can press `i` on a PATCH (line selector: `i <index>`). This runs the same package/schema/preflight checks, shows compatibility/targets/post commands/Git policy, and ends with:

```text
INSPECT RESULT: PASS — project unchanged
```

No PATCH payload is executed and the package is not archived.
