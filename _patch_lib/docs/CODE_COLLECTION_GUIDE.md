# CODE COLLECTION GUIDE — v6.12.1 AUTHORITATIVE CONTRACT

Python Patch Tool v6.12.1 is self-contained for its documented COLLECT schema. The authoritative action list is not inferred from old guides; it is defined by `COLLECT_ACTION_SCHEMA.json` and enforced before execution.

This is an **AI/tool-facing technical document**. Do not copy the action table into the end-user HTML guide; the user should not need to choose or understand action types.

## Supported actions

| Action | Purpose |
|---|---|
| `pack` | Exact current bytes for known project-relative files. |
| `overview` | Bounded project/tree/file-type overview. |
| `find` | Find path/name globs, optionally collect matched files. |
| `search` | Bounded literal/regex search with line context. |
| `git` | Fixed read-only Git status/log/diff sections. |

Any other action type or unsupported field is rejected as `COLLECT INVALID` before the collector runs.

## Examples

### Exact files

```json
{
  "id": "ota-transport-source",
  "actions": [
    {"type": "pack", "paths": ["main/app.c", "main/app.h"]}
  ]
}
```

### Overview

```json
{
  "id": "project-overview",
  "actions": [
    {"type": "overview", "path": ".", "tree_depth": 3}
  ]
}
```

### Find + search + Git

```json
{
  "id": "root-cause-source",
  "actions": [
    {"type": "find", "paths": ["."], "patterns": ["*.java", "*.properties"], "collect": true},
    {"type": "search", "paths": ["."], "query": "getPassword\\(|saveBatch", "regex": true, "context_lines": 8},
    {"type": "git", "sections": ["status", "log", "diff_stat", "diff"]}
  ]
}
```

## Request delivery

AI returns **one ZIP containing exactly one `CODE_COLLECTION_REQUEST*.json`**. The user copies the ZIP to `patchs/` and runs only:

```bash
./tools/run_python_patches.sh
```

Raw JSON is rejected.

## Selection

One invocation can run exactly one COLLECT request and cannot mix it with PATCH. This is not a global lock; separate terminals may run independently.

## Result

Upload only the result ZIP highlighted as `[PRIMARY - UPLOAD THIS FILE]`.
