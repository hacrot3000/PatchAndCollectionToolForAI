# CODE COLLECTION GUIDE — CURRENT COMPATIBILITY CONTRACT (v6.11.0)

This file intentionally replaces obsolete v5 guides at the same path.

## Guaranteed overlay action: `pack`

Python Patch Tool v6.11.0 natively supports one readonly action without relying on the private collector core:

```json
{
  "type": "pack",
  "paths": [
    "relative/path/to/source.c",
    "relative/path/to/source.h"
  ]
}
```

`pack` means: copy the exact current bytes of the listed project files into one verified collection result ZIP. The result contains `COLLECTION_MANIFEST.json` plus each source under `files/<project-relative-path>` with size and SHA-256 metadata.

Rules for `pack.paths`:

- each entry is an exact project-relative file path;
- `/` separators only;
- no absolute paths;
- no `..` traversal;
- no glob patterns;
- no directories;
- no symlinks;
- missing files make the COLLECT fail instead of silently omitting evidence.

A request containing only `pack` actions is handled by the v6.11.0 overlay itself and therefore does not require `python_patch_readonly_collector.py` to understand `pack`.

## Other action names remain collector-specific

Older versions of this guide advertised a fixed list including `overview`, `research`, `ls`, `tree`, `find`, `file`, `search`, `references`, `git`, and decompile aliases. That historical list is **not authoritative for the currently installed private collector**. Runtime evidence from an installed v6.9.1 environment rejected `overview` with `Unknown action type: overview`.

Therefore, except for the overlay-guaranteed `pack` action above:

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
