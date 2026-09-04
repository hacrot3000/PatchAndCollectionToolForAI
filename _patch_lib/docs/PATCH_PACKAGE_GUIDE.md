# PATCH PACKAGE GUIDE — v6.15.0 authoritative AI/tool contract

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
    "min_tool_version": "6.15.0",
    "max_tool_version": "7.0.0",
    "max_tested_version": "6.15.0"
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

## Rollback path/runtime safety — v6.14.1

For a rollback target declared with `exists:false`, the target's parent directory must already exist before the PATCH starts and every ancestor must be a real non-symlink directory inside the project. The tool does not invent or remove directories as part of rollback.

Rollback baselines are re-checked when the pre-payload snapshot is created. If a target changes between preflight and snapshot, execution fails closed before the payload runs.

On POSIX, restore pins the validated parent directory with a directory file descriptor and `O_NOFOLLOW`-style checks so an ancestor symlink swap cannot redirect the restore outside the project.

On POSIX, Python PATCH payloads and post-patch commands run in isolated process groups. Timeout, SIGINT or SIGTERM terminates the managed group before rollback/return. Windows uses the platform-specific subprocess branch; PATCH authors must not assume POSIX signal/process-group semantics there.

## Exact input lifecycle — v6.14.1

Patch Tool snapshots the exact PATCH package selected from `patchs/` before preflight/execution and archives those exact executed bytes after PASS. If another process or the PATCH itself replaces the same queue filename while execution is in progress, the replacement is preserved in `patchs/` for a later run instead of being archived as though it had already executed.

A FAIL_HANDOFF only embeds the current queue PATCH when its SHA-256 still equals the structured executed package SHA. A changed/replaced queue package is omitted from the handoff rather than misidentified.

## Inspect / dry-run

In the interactive selector the user can press `i` on a PATCH (line selector: `i <index>`). This runs the same package/schema/preflight checks, shows compatibility/targets/post commands/Git policy, and ends with:

```text
INSPECT RESULT: PASS — project unchanged
```

No PATCH payload is executed and the package is not archived.

## v6.15.0 package lint / validate

Before delivering a PATCH, validate the manifest against `PATCH_PACKAGE_SCHEMA.json` and `PATCH_PACKAGE_CHECKLIST.json`. **Never invent `source_baseline`** or other legacy/custom fields. Source assumptions belong only in `preflight.files` using `path`, optional `exists`, `sha256`, and/or `anchors`.

Known migration:

```text
source_baseline.files[].file   -> preflight.files[].path
source_baseline.files[].sha256 -> preflight.files[].sha256
```

Timeout values, when present, must be integers `1..1800`. Omit `execution.timeout_seconds` to use the default; do not use `0` as an unlimited sentinel.

Read-only project-aware validation:

```text
Linux:   ./tools/run_python_patches.sh validate --patch patchs/example.zip
Windows: tools\run_python_patches.bat validate --patch patchs/example.zip
```

Validation/inspect results use one of four classes:

- `READY_TO_APPLY` — package and current source preflight are ready; project unchanged.
- `PATCH_INVALID` — package/schema/compatibility/environment contract is invalid; project unchanged.
- `SOURCE_DRIFT` — current source SHA/existence/anchor or OPS match assumptions do not hold; project unchanged.
- `TOOL_ERROR` — unexpected Patch Tool internal failure; project unchanged.

Schema lint reports multiple independent manifest issues in one pass. Source preflight likewise aggregates declared file mismatches and includes expected/actual SHA where applicable. For data-only OPS packages, v6.15.0 simulates the sequential operation list on a private temporary mirror before execution; a missing/ambiguous source match fails before the real payload is allowed to write.
