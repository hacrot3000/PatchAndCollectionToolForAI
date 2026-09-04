# AI / ChatGPT usage contract — Python Patch Tool v6.12.1

This document overrides obsolete v5/v6.11 COLLECT examples and older chat instructions.

## Public workflow

The normal public command is always:

```bash
./tools/run_python_patches.sh
```

PATCH and COLLECT packages go into `<project>/patchs/`. AI must return **one ZIP artifact**, never loose patch/collect JSON.

## Before an AI work session

The user should send AI **all files under `tools/_patch_lib/docs/`**. When Patch Tool itself is being developed, also send:

- `tools/implementing.md`
- `tools/PYTHON_PATCH_TOOL_FEATURES_VI.md`

AI must read the current docs before creating PATCH/COLLECT artifacts.

### User-guide boundary

`tools/HUONG_DAN_PYTHON_PATCH_TOOL.html` is intentionally user-oriented and must **not** expose or require the user to understand the internal COLLECT action list/schema. Keep action names, validation fields, and request construction rules in the AI-facing docs under `tools/_patch_lib/docs/`. The AI chooses/constructs the request; the user only places the returned request ZIP in `patchs/`, runs the normal launcher, and uploads the highlighted result ZIP.

## Authoritative COLLECT action schema

The machine-readable source of truth is:

```text
tools/_patch_lib/docs/COLLECT_ACTION_SCHEMA.json
```

v6.12.1 is self-contained and guarantees exactly these readonly actions:

- `pack`
- `overview`
- `find`
- `search`
- `git`

Do not invent or alias an unsupported action. Historical names such as `research`, `content`, `search_files`, `symbol_graph`, `references`, `callgraph`, `dependencies`, or `decompile` are **not part of the v6.12.1 contract** unless a future schema explicitly adds them.

### `pack`

```json
{"type":"pack","paths":["relative/file.c","relative/file.h"]}
```

Exact regular files only; no absolute paths, `..`, globs, directories or symlinks.

### `overview`

```json
{"type":"overview","path":".","tree_depth":3}
```

Produces a bounded project/file-type/tree overview. This is a real supported action in v6.12.1; it is no longer guessed/delegated to an unknown private collector.

### `find`

```json
{"type":"find","paths":["."],"patterns":["*.c","CMakeLists.txt"],"collect":true}
```

Finds by path/name glob. With `collect:true`, matched regular files are copied into the result subject to limits.

### `search`

```json
{"type":"search","paths":["src"],"query":"foo|bar","regex":true,"context_lines":8}
```

Bounded UTF-8 text search with context.

### `git`

```json
{"type":"git","sections":["status","log","diff_stat","diff"]}
```

Only fixed read-only Git sections are accepted. Arbitrary Git/shell commands are not accepted.

## COLLECT request delivery

AI returns exactly one ZIP:

```text
CODE_COLLECTION_REQUEST_<purpose>_<timestamp>.zip
└── CODE_COLLECTION_REQUEST_<purpose>_<timestamp>.json
```

The inner JSON must pass `COLLECT_ACTION_SCHEMA.json` preflight. Unsupported actions/fields become `COLLECT INVALID` before execution.

## Selection isolation

- at most one `[COLLECT]` per invocation;
- never mix COLLECT and PATCH;
- `a` means all PATCHes only;
- priority `0..9` applies to PATCH only;
- no project/process lock: other terminals may run independently by operator choice.

## PATCH duplicate rules

Two duplicate scopes exist:

1. **Current queue/session**: exact same size + SHA-256 among queued PATCHes. The first natural-order file is canonical; later byte-identical files are removed from `patchs/` immediately and never shown as separate runnable items.
2. **Local successful history**: exact SHA-256 against current project's direct `patchs/patched/`; those queue files are skipped, not globally/server suppressed.

No cross-machine/server/project-key duplicate database is used.

## Self-contained package contract

v6.12.1 ships its own:

- `python_patch_runner.py`
- `python_patch_utils.py`
- `python_patch_readonly_collector.py`
- `python_patch_collect_compat.py`
- dispatcher/progress/schema/docs

The package therefore does not require an older private core for the documented v6.12.1 contract. Historical/ambiguous formats outside this contract fail closed instead of being guessed.

## Result ZIP

A successful COLLECT prints exactly one highlighted upload path:

```text
!!! [PRIMARY - UPLOAD THIS FILE] !!!
>>> ACTION REQUIRED: UPLOAD TO CHATGPT / AI SERVER <<<
<absolute-path-to-result.zip>
```

Upload that ZIP to AI. Do not put collection-result ZIPs back into `patchs/`.

## PATCH rules retained

PATCH remains in-place. SANDBOX/detached Git worktree transaction execution is permanently removed. Patch archives are safely extracted outside the project and the patch script/OPS runs with project root as working directory.
