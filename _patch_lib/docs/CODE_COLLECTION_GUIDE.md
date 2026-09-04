# CODE COLLECTION GUIDE — CURRENT COMPATIBILITY CONTRACT (v6.10.1)

This file intentionally replaces obsolete v5 guides at the same path.

## Do not use the old v5 action table

Older versions of this guide advertised a fixed list including `overview`, `research`, `ls`, `tree`, `find`, `file`, `search`, `references`, `pack`, `git`, and decompile aliases. That list is **not authoritative for the currently installed collector**. Runtime evidence from an installed v6.9.1 environment rejected `overview` with `Unknown action type: overview`.

Therefore:

- do not copy old action examples into a new request;
- do not guess aliases;
- use only the exact schema/actions from the current installed collector, or a request known to have PASSed with that same collector revision;
- if no authoritative schema/template is available, collect that compatibility evidence first instead of inventing JSON.

## Delivery format

AI returns one ZIP containing exactly one `CODE_COLLECTION_REQUEST*.json`. The user places the ZIP in `patchs/` and runs only:

```bash
./tools/run_python_patches.sh
```

## Selection rule

Exactly one COLLECT request may be selected in an invocation, and it cannot be mixed with PATCH. This does not serialize or lock the whole project: separate terminal windows/processes remain allowed by operator choice.

## Result

Upload the single verified result ZIP highlighted as `[PRIMARY - UPLOAD THIS FILE]`; do not put a collection-result ZIP back into the runnable queue.
