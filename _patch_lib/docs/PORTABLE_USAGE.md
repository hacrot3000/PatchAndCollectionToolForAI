# Portable use — Python Patch Tool v6.11.0

Normal entry point:

```bash
./tools/run_python_patches.sh
```

## COLLECT request delivery

AI must provide `CODE_COLLECTION_REQUEST_<purpose>_<timestamp>.zip` containing exactly one `CODE_COLLECTION_REQUEST*.json`. Loose request JSON is rejected. The user copies only the ZIP to `patchs/` and runs the zero-argument launcher.

## Guaranteed `pack` compatibility

v6.11.0 natively supports `{"type":"pack","paths":[...]}` for exact current project files. Paths are project-relative regular files only; absolute/traversal/glob/directory/symlink entries fail closed. Pack-only requests do not depend on private collector support.

Other COLLECT action names remain revision-specific to the installed private collector. The historical v5 action table is not a universal schema; `overview` has been observed rejected by a v6.9.1-installed collector.

## One COLLECT per invocation

Each invocation may select either:

- one or more PATCHes; **or**
- exactly one COLLECT request.

Never both. Never two COLLECT requests. `a` selects PATCHes only and priority `0..9` is PATCH-only. This rule is enforced by TTY selection, line selection, initial/config selection normalization, and a final execution guard.

Line-mode `a/all` has the same PATCH-only meaning as the TTY `a` key. On a COLLECT-only queue it does not start the request; select the specific COLLECT index explicitly.

There is **no project/process queue lock** in v6.11.0. Other terminals may run other Patch Tool processes manually if the operator wants concurrent work.

## Permanent PATCH in-place contract

SANDBOX / detached-worktree execution is removed. PATCH routes through the public launcher are forced to `--transaction off`; obsolete transaction-only invocations fail closed.

## Result collection ZIP

The verified result is highlighted once under `[PRIMARY - UPLOAD THIS FILE]`. Upload that ZIP to ChatGPT/AI. Do not return it to `patchs/`; canonical collection-result archives are skipped as non-runnable evidence.
