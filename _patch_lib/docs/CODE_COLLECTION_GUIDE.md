# CODE COLLECTION GUIDE — v6.18.0 AUTHORITATIVE CONTRACT

Python Patch Tool v6.18.0 is self-contained for its documented COLLECT schema. The authoritative action list is not inferred from old guides; it is defined by `COLLECT_ACTION_SCHEMA.json` and enforced before execution.

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

One invocation can run exactly one COLLECT request and cannot mix it with PATCH. This is not a global queue lock; separate COLLECT/selector terminals may run independently. PATCH source mutation is serialized per project only while mutating source, to prevent lost updates.

## v6.17.6 safety bounds

Request limits cannot exceed the tool's local hard ceilings. Internal result/output folders are excluded from discovery so a COLLECT can never recursively collect the ZIP it is currently writing. Exact-file packing checks byte quotas during copying, not only before copying. Regex search executes in an isolated worker with a 60-second hard timeout per regex action. Likely credential/private-key files are still preserved exactly when explicitly requested, but the result manifest/console warns the operator before upload.

## Result

Upload only the result ZIP highlighted as `[PRIMARY - UPLOAD THIS FILE]`.


## Exact request lifecycle — v6.14.1

The request ZIP is snapshotted before execution. A successful COLLECT archives the exact request bytes that were executed. If another process replaces the same queue filename with different bytes while collection is running, that replacement remains in `patchs/` for a later run and is not silently archived/deleted as though it had executed.


## v6.18.0 search/discovery contract

**Zero matches is a search result, not proof of absence.** A zero result may be interpreted as absence only when the search report says `Coverage status: VERIFIED`.

Recommended investigation request:

```json
{
  "type": "search",
  "paths": ["projects/m3-server"],
  "query": "CmdMineInfoCSReqMsg",
  "regex": false,
  "backend": "auto",
  "source_scope": "filesystem",
  "filesystem": true,
  "respect_gitignore": false,
  "follow_symlinks": false,
  "must_find": true,
  "diagnose_on_zero": true,
  "fallback_search": true,
  "report_coverage": true,
  "report_skipped_dirs": true,
  "module_discovery": true,
  "anchor_paths": ["projects/m3-server/trunk/jdqs_server"],
  "expected_files": [
    "projects/m3-server/trunk/jdqs_server/src/main/java/com/xkhy/jdqs/handler/mine/MineInfoCSHandler.java"
  ]
}
```

Defaults are deliberately investigation-safe: `source_scope=filesystem`, `backend=auto`, `respect_gitignore=false`, `fallback_search=true`, `diagnose_on_zero=true`, coverage/skipped-dir reporting enabled, and symlink following disabled. `source_scope=git_tracked` is available only when the caller explicitly wants Git-index scope.

Search budgets are separate from collection-pack budgets. `limits.max_files` remains the bound for files collected/iterated by legacy pack/find/overview behavior; search uses `limits.max_search_files` (default 250,000; hard ceiling 1,000,000) and `max_search_file_bytes` (default 64 MiB). Hitting a search bound is surfaced as partial/untrusted coverage, never a silent zero.

`backend=auto` prefers ripgrep discovery when available and verifies it with a Python filesystem traversal. A primary/fallback disagreement emits:

```text
SEARCH_INCONSISTENCY
primary_matches=0
fallback_matches=17
```

and the collection result is `INCOMPLETE`. If ripgrep is unavailable, the tool uses two independent Python traversal strategies.

`must_find=true` is an assertion. Zero matches creates a valid diagnostic result ZIP with `collection_status=INCOMPLETE`, prints it as the PRIMARY upload artifact, and returns rc=3 instead of PASS. This intentionally gives AI the diagnostics without allowing a false absence conclusion.

Every search report includes `=== SEARCH COVERAGE ===` with requested/resolved scopes, directories visited, files considered/searched, extension counts, candidate modules/directories, skipped directories/files, backend results and final coverage status. With `diagnose_on_zero=true`, a `ZERO MATCH DIAGNOSTIC` section also reports candidate filename evidence, symlink/gitignore policy and whether a search limit was reached.

Run the disposable discovery fixture with:

```bash
./tools/run_python_patches.sh health-search
```

It covers literal/regex search, filename find, nested directories, untracked and gitignored files, Unicode names/content, symlink policy, source trees larger than the old 5,000-file boundary, relative/absolute in-project paths, `must_find`, `anchor_paths` and `expected_files`.
