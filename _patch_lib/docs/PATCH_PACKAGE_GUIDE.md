# PATCH PACKAGE GUIDE — v6.14.0 authoritative AI/tool contract

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
    "min_tool_version": "6.14.0",
    "max_tool_version": "7.0.0",
    "max_tested_version": "6.14.0"
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

## Metadata-driven safe rollback

Automatic rollback is **opt-in and fail-closed**. It is not a replacement for SANDBOX/worktree transactions. AI may request rollback only when every mutable target is known and has an exact preflight baseline.

Example:

```json
{
  "targets": ["src/a.c", "generated/new.h"],
  "preflight": {
    "files": [
      {"path": "src/a.c", "exists": true, "sha256": "<64 hex>"},
      {"path": "generated/new.h", "exists": false}
    ]
  },
  "recovery": {
    "rollback": {
      "targets": ["src/a.c", "generated/new.h"],
      "on": ["payload_failure", "post_patch_failure"],
      "max_total_bytes": 268435456
    }
  }
}
```

Rules:

- `recovery.rollback.targets` must exactly cover the PATCH target set resolved by preflight.
- Every rollback target needs an explicit `preflight.files` baseline. Existing files require `exists:true` plus exact SHA-256; initially absent files require `exists:false`.
- The runner snapshots only those declared regular-file targets, with a bounded total byte limit, after preflight PASS and before payload execution.
- Rollback may run only for `payload_failure` and/or `post_patch_failure`, before Git policy. It never attempts to undo a Git commit/push failure.
- Existing files are restored from exact snapshot bytes; files that were initially absent are removed only if they became a regular file/symlink at the exact declared target path. Directories/non-file objects are never recursively removed.
- On a Git project the pre/post worktree fingerprint verifies whether the whole project returned to baseline. If an undeclared file changed, status becomes `PARTIAL` even if declared targets were restored. Outside Git, verification is explicitly limited to declared targets.
- Missing/ambiguous recovery metadata becomes `PREFLIGHT FAIL — project unchanged`; AI must not guess a rollback contract.

`ROLLBACK: PASS` means the configured recovery scope was restored and verified according to the evidence available. `PARTIAL`/`FAIL` remains a normal PATCH failure and the generated FAIL_HANDOFF should be uploaded to AI.

## Inspect / dry-run

In the interactive selector the user can press `i` on a PATCH (line selector: `i <index>`). This runs the same package/schema/preflight checks, shows compatibility/targets/post commands/Git policy, and ends with:

```text
INSPECT RESULT: PASS — project unchanged
```

No PATCH payload is executed and the package is not archived.
