# AI / ChatGPT usage contract — Python Patch Tool v6.11.0

This document overrides obsolete v5 COLLECT examples and older chat instructions.

## Public workflow

The normal public command is always:

```bash
./tools/run_python_patches.sh
```

If AI needs more source/evidence, it must return **one ZIP request package**, never a loose JSON:

```text
CODE_COLLECTION_REQUEST_<purpose>_<timestamp>.zip
└── CODE_COLLECTION_REQUEST_<purpose>_<timestamp>.json
```

The ZIP contains exactly one request JSON and is copied to `<project>/patchs/`.
The user runs the zero-argument launcher and later uploads only the verified result ZIP shown under `!!! [PRIMARY - UPLOAD THIS FILE] !!!`.

## Guaranteed action: exact-file `pack`

Python Patch Tool v6.11.0 guarantees this action at the overlay layer:

```json
{
  "type": "pack",
  "paths": [
    "main-esp32c3/main/ota/app_ota_ble_transport.c",
    "main-esp32c3/main/ota/app_ota_ble_transport.h"
  ]
}
```

Use `pack` when the AI needs exact current source files by known relative path. `paths` must contain exact regular files inside the project root. Do not use absolute paths, `..`, globs, directories, or symlinks. Missing paths must remain an explicit COLLECT failure rather than being skipped.

A request consisting only of `pack` actions is handled directly by the v6.11.0 compatibility collector. The result ZIP preserves each source as `files/<relative-path>` and includes `COLLECTION_MANIFEST.json` with path, byte size and SHA-256.

## All other COLLECT actions — strict compatibility rule

**Do not use the historical v5 action list as an authoritative schema.** An observed installed collector used with Patch Tool v6.9.1 rejected `overview` at runtime with `Unknown action type: overview`. Except for the overlay-guaranteed `pack` action above, action names remain private-collector/revision specific.

For any non-`pack` action, AI must do one of the following before creating the inner JSON:

1. Use the exact action names and fields defined by the **current installed collector/schema** supplied for that project; or
2. Reuse/adapt an action object from a `CODE_COLLECTION_REQUEST` known to have **PASSed with the same installed collector revision**.

If neither is available, AI must request the current collector/schema or a known-good request template. **Never guess an action type.** In particular, do not emit `overview` merely because an older v5 document listed it.

## Selection isolation — mandatory

A `CODE_COLLECTION_REQUEST` is a standalone job within one invocation:

- select **at most one** `[COLLECT]` item per run;
- never select `[COLLECT]` together with any `[PATCH]`;
- selecting a COLLECT in the TTY selector clears every other selection;
- selecting a PATCH while a COLLECT is selected switches back to PATCH selection;
- `a` means **all PATCHes**, not all COLLECT requests;
- priority `0..9` applies to PATCH only; COLLECT has no priority.

This is **selection isolation only**. Patch Tool v6.11.0 deliberately does **not** hold a project/process lock. The operator may open other terminal windows and run other Patch Tool processes manually/concurrently. Concurrent execution is operator-controlled and does not create global/shared history.

## Result ZIP

A successful COLLECT prints one highlighted result path only:

```text
!!! [PRIMARY - UPLOAD THIS FILE] !!!
>>> ACTION REQUIRED: UPLOAD TO CHATGPT / AI SERVER <<<
<absolute-path-to-result.zip>
```

The archived request path is informational. A result archive containing `COLLECTION_MANIFEST.json` is evidence and must not be placed back into `patchs/`; queue discovery skips it fail-closed.

## PATCH rules retained

PATCH stays in-place; SANDBOX/worktree execution remains removed. Duplicate PATCH suppression is local-only against the current project `patchs/patched/` by exact package SHA-256; it is not server/global/cross-machine history.
