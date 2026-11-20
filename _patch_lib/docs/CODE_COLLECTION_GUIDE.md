# CODE COLLECTION GUIDE — v6.20.2 AUTHORITATIVE CONTRACT

## COLLECT read-only command action (local and SSH)

When COLLECT runs in TaskDeck, **Task Run** lists each planned `command` action as a nested **QUEUE** row, then **RUNNING**, followed by **DONE** or **FAIL**, including elapsed time. Read-only local commands and saved-profile SSH both flush their bounded stdout/stderr to a run-owned console log while executing. **View console / Hide console / Copy console** operate during the run and command evidence remains available in **History** after the collection has finished. The normal COLLECT result ZIP/TXT still includes its own command report;  A run-owned `command_states.json` journal lets the current Task Run recover command rows during execution if a transient protocol event is missed. this live presentation is additional evidence, not shell access.

COLLECT accepts `{"type":"command", "transport":"local"|"ssh", "argv":[...]}` as an **evidence-producing** action. AI must provide one literal argv array per action, not a shell script or command string. The collector executes a whitelisted read-only command, captures merged console stdout/stderr in a bounded report `reports/NNN_command.md`, records the exit status, and includes the report in the usual COLLECT ZIP and TXT. Multiple independent command actions may be included in one COLLECT request. No project source payload is run or modified.

**Local example:**

```json
{"actions":[
  {"type":"command","transport":"local","argv":["grep","-nriF","--","connection failed","src"],"cwd":".","timeout_seconds":30,"max_output_bytes":32768},
  {"type":"command","transport":"local","argv":["head","-n","100","logs/app.log"]}
]}
```

**SSH example** (AI must ASK for the exact saved SSH profile name if unknown; do not invent host/credentials):

```json
{"actions":[
  {"type":"command","transport":"ssh","profile_name":"<saved unique TaskDeck profile name>","argv":["rg","-n","--","crash","logs"],"timeout_seconds":30,"max_output_bytes":32768}
]}
```

In a real COLLECT request, replace placeholders and provide `project.key` and `project.id` from the current AI Pack. SSH uses TaskDeck's saved-profile bridge and the same strict Go SSH whitelist, not arbitrary OpenSSH arguments. In local mode, only the **read-only subset of local legacy_strict** is available; local script/interpreter execution is explicitly prohibited in COLLECT even though a standalone PATCH may run an already existing project script. Whitelisted commands: `pwd`, `ls`, `tree`, `find`, `grep`, `rg`, `cat`, `head`, `tail`, `wc`, `nl`, and numeric-print-only `sed -n`. Local COLLECT basic commands are deliberately narrower than the legacy PATCH compatibility lane: `pwd` accepts no arguments; `ls` only `-a/-l/-h/-1` and one relative path; `tree` only `-a/-d/-L 1..3` and one relative path; `find` requires `-maxdepth 1..3` before optional `-type f/d`, `-name/-iname GLOB`, `-mindepth` and `-print/-print0`. File-writing `tree -o` (including the joined `-oFILE` form), unbounded `find`, and recursive `ls -R` are rejected. `find -exec`/`-delete` and other execution/writing actions, `tail -f`, `sed -i`, `grep -f`, `rg --pre`, shell pipelines and redirects remain forbidden.

Timeout is 1–120 seconds (default 30); `max_output_bytes` is 1024–65536 (default 65536). Output beyond the bound is marked truncated/INCOMPLETE; deadlines and non-success codes are reported with evidence. The command path never consumes stdin and does not create a script. SSH command files/paths resolve relative to the remote login directory; local command paths resolve relative to `cwd` under the project. **COLLECT commands can reveal sensitive information in command output**: AI should request only necessary diagnostics and avoid reading credentials/configurations with secrets. This does not replace the existing first-class `search`, `file`, `git`, `database_select` or other COLLECT actions; prefer those when structured provenance/coverage is needed.

Python Patch Tool v6.19.3 ships a self-contained, read-only COLLECT runtime. The authoritative action/field list is `COLLECT_ACTION_SCHEMA.json`; this guide explains the intended semantics.

This is an **AI/tool-facing technical document**. The public user workflow remains intentionally simple: AI provides one request ZIP (or one full plain-text JSON request for TaskDeck's Paste text mode); TaskDeck queues a canonical ZIP in `patchs/`, then the normal run workflow executes it.

## Public workflow — historical direct COLLECT CLI remains superseded

Do **not** tell the user to run historical commands such as:

```text
./tools/run_python_patches.sh collect search ...
```

That direct subcommand workflow was intentionally superseded by the queue workflow. Current delivery is:

```text
CODE_COLLECTION_REQUEST_<purpose>_<timestamp>.zip
└── CODE_COLLECTION_REQUEST_<purpose>_<timestamp>.json
```

Then the user runs only:

```bash
./tools/run_python_patches.sh
```

Raw request JSON is not a directly runnable file in `patchs/`. TaskDeck's **Paste text → COLLECT** is the supported exception: paste exactly one complete request JSON with `project.key`, `project.id` and nonempty `actions` according to `COLLECT_ACTION_SCHEMA.json`. The backend checks identity and puts that JSON in a canonical request ZIP; it never runs pasted Python. An optional Python response form is a single `COLLECT_REQUEST_JSON = r'''{...}'''` literal; only the JSON string is extracted. ZIP uploads continue to work unchanged.

## v6.18.5 `find` / `directory` glob semantics

`find.patterns[]` are matched additively against:

1. the filename (`MineInfoCSHandler.java`);
2. the path relative to each requested `paths[]` scope (`src/main/java/.../MineInfoCSHandler.java`);
3. the full project-relative path.

This means a request rooted at `projects/m3-server/trunk/jdqs_server` may correctly use `src/main/java/.../*.java` patterns. `find` scans with `max_search_files`; `max_files` only limits packaged output. The report includes `=== FIND COVERAGE ===` and a zero result is reliable only when coverage is `VERIFIED`.

For both `find` and `directory`, globstar `**/` means **zero or more** directory levels. Example: `**/*.java` matches both `Foo.java` and `nested/Foo.java`.

These semantics are additive: historical basename and project-relative matching remain supported.

## Current actions

### Core/current actions

| Action | Purpose |
|---|---|
| `pack` | Exact current bytes for known project-relative regular files. |
| `overview` | Bounded project/tree/file-type overview. |
| `find` | Find path/name globs, optionally collect matched files. |
| `search` | Coverage-aware literal/regex source search. |
| `git` | Fixed read-only Git status/log/diff sections. |

### Restored historical COLLECT capabilities

| Action | Purpose / compatibility semantics |
|---|---|
| `ls` | Immediate bounded directory listing. |
| `tree` | Bounded Python directory tree; symlinks are not followed by default. |
| `research` | `overview` + the current coverage-aware `search` engine. |
| `file` / `range` | Whole text file or selected line interval. |
| `head` / `tail` | First/last bounded line count. |
| `symbol` | Function/class/struct-like symbol block extraction from an exact file. |
| `references` | Symbol reference search through the current verified filesystem search backend. |
| `callgraph` | Bounded caller/reference discovery plus heuristic source-level callees. |
| `dependencies` | Bounded include/import/use/require inventory. |
| `directory` | Recursive source collection with include/exclude globs. Obvious credential/private-key files are not auto-collected. |
| `zip` | Historical alias for current exact-file `pack`. |
| `decompile` / `ida` / `ghidra` | Large IDA/Ghidra-like text dump extraction by address/name/regex using a temporary SQLite index, with neighbors/references. |

### Historical/private request aliases restored for real existing workflows

| Alias | Current behavior |
|---|---|
| `search_files` | Alias to coverage-aware `search`. |
| `content` | Alias to coverage-aware `search`. |
| `symbol_graph` | Multi-symbol references/callers/callees/dependency investigation. |

These aliases exist because historical request ZIPs used them. They are protected compatibility surface and must not be silently removed.

## Important compatibility boundary: `pack`

Old v5 documentation allowed directory-style packing. v6.11 intentionally redefined guaranteed `pack` as **exact regular-file evidence**, and v6.18.3 keeps that safer/current semantics rather than silently changing it again.

Use:

- `pack` / `zip` for exact known files;
- `directory` for a recursive subtree selected by include/exclude patterns.

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

### Historical source inspection restored

```json
{
  "id": "runtime-investigation",
  "actions": [
    {"type": "tree", "path": "src", "max_depth": 4},
    {"type": "symbol", "path": "src/runtime.c", "symbol": "runtime_start"},
    {"type": "references", "symbol": "runtime_start", "paths": ["src", "include"]},
    {"type": "dependencies", "paths": ["src"]}
  ]
}
```

### M3-compatible aliases

```json
{
  "id": "m3-symbol-graph",
  "actions": [
    {
      "type": "symbol_graph",
      "paths": ["projects/m3-server"],
      "symbols": ["CmdMineInfoCSReqMsg"],
      "context_lines": 8,
      "include_references": true,
      "include_callers": true,
      "include_callees": true,
      "include_dependencies": true,
      "max_occurrences": 1200,
      "max_callers": 300,
      "max_callees": 50,
      "max_dependency_files": 400
    },
    {
      "type": "content",
      "paths": ["projects/m3-server"],
      "query": "CmdMineInfoCSReqMsg",
      "regex": false,
      "context_lines": 8
    }
  ]
}
```

### Directory collector

```json
{
  "type": "directory",
  "path": "gate-rp2040",
  "include": ["**/*.c", "**/*.h"],
  "exclude": ["build/**"]
}
```

### Large decompile dump

```json
{
  "type": "decompile",
  "source": "docs/GM_52_76.c",
  "name": "sub_140123456",
  "match": "exact",
  "neighbors_before": 2,
  "neighbors_after": 2,
  "include_references": true
}
```

Address lookup is also supported:

```json
{
  "type": "ida",
  "source": "docs/GM_52_76.c",
  "address": "0x21E551A",
  "include_references": true
}
```

The temporary SQLite index is created outside the project source tree and deleted when the action finishes.

## Search/discovery contract — v6.18.0+ preserved

> **Zero matches is a search result, not proof of absence.**

A zero result may be interpreted as absence only when the report says:

```text
Coverage status: VERIFIED
```

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

Defaults are investigation-safe:

- `source_scope=filesystem`;
- untracked/gitignored files are visible unless `respect_gitignore=true` is explicitly requested;
- `backend=auto` uses a primary backend plus an independent Python traversal;
- symlink following is disabled by default;
- search budgets are separate from collection budgets.

Primary/fallback disagreement emits `SEARCH_INCONSISTENCY` and makes the result `INCOMPLETE`. `must_find=true` with zero matches also creates an `INCOMPLETE` diagnostic ZIP instead of a false PASS.

Run the disposable discovery fixture with:

```bash
./tools/run_python_patches.sh health-search
```

## AI context synchronization (v6.19.2)

Top-level COLLECT request may include:

```json
"ai_context": {
  "known_tool_version": "6.19.2",
  "sync_token": "ptv-ai-sync-v1:<token>",
  "agent_id": "default"
}
```

When the AI context is stale/unknown and the current fingerprint has not yet been delivered to that agent, the result ZIP includes `AI_TOOL_SYNC/` with the current authoritative docs. The same material automatically appears in the clear-text companion. After one successful delivery the full docs are suppressed until the tool/document fingerprint changes.

## Database SELECT evidence (v6.19.0)

COLLECT supports `database_select` as a SELECT-only active builder. The request names a local profile and supplies structured arrays/objects for SELECT expressions, sources, joins, nested conditions, subqueries, GROUP BY/HAVING, ORDER BY and limits. Raw SQL is not an accepted field.

Profiles live outside request ZIPs in `tools/db_profiles.local.json` (or `.python_patch_tool/db_profiles.local.json`) and may describe SQLite, loopback MySQL, or MySQL through an SSH tunnel. Start from `tools/db_profiles.example.json`. MySQL passwords are not stored in these JSON files; use `mysql_config_editor` login paths.

Database result chunks are stored inside the same `CODE_COLLECTION_RESULT_*.zip` under `database_queries/`. A fully exhausted SELECT is `COMPLETED/VERIFIED`; row/byte/timeout/package truncation keeps available chunks and returns `INCOMPLETE`.

See `DATABASE_SELECT_ACTIVE_BUILDER.md` for the exact AST grammar and examples.

## Limits

Current request limits include:

- `max_file_bytes` — exact files added to the result ZIP;
- `max_total_bytes` — aggregate collected file bytes;
- `max_files` — collected-file quota;
- `max_report_bytes` — aggregate report quota;
- `max_search_files` — source discovery/search budget;
- `max_search_file_bytes` — per-file content-search/read budget;
- `max_decompile_file_bytes` — large decompile source budget.

Request-provided limits may not exceed local hard ceilings.

## Safety invariants

- Only project-contained relative paths are accepted for file/subtree actions.
- `..` traversal, unsafe absolute file collection paths, and symlink file targets fail closed.
- Request data cannot provide arbitrary shell commands.
- Internal Patch Tool output folders are excluded from source discovery/automatic collection.
- Discovery-driven `directory` collection skips obvious sensitive filename/content candidates rather than auto-ingesting them.
- Explicit exact-file `pack` retains current v6 semantics: if the operator/AI explicitly names a sensitive file, it is preserved exactly but clearly warned in console/manifest.
- Search-like compatibility actions (`research`, `references`, `search_files`, `content`, symbol-graph reference discovery) use the same filesystem coverage/fallback protections as `search`.
- Regex search remains isolated in a worker with a hard timeout. v6.18.7 adds a soft deadline/checkpoint: timeout preserves the newest partial result as COLLECT `INCOMPLETE` instead of discarding evidence.
- `fallback_search=true` means independent fallback is mandatory for zero/error verification. Positive primary matches skip a second full content scan by default; set `verify_nonzero_with_fallback=true` to request the more expensive positive-result consistency pass.
- `Search execution status: COMPLETED` is reserved for fully finished/untruncated search actions; timeout, search-budget exhaustion, and `max_matches` truncation report `PARTIAL`.
- Discovery-driven file actions (`find` with `collect=true`, `directory`) keep files already copied when collection quotas are reached and mark the result `INCOMPLETE`; exact `pack` still fails closed because its named files are mandatory evidence. Any action/report truncation also prevents a false `COMPLETED` result.
- COLLECT snapshots the exact request ZIP before execution and archives the exact executed bytes after success.

## Selection/result

Exactly one COLLECT may run in an invocation and it cannot be mixed with PATCH. This is selection isolation, not a global process lock.

Upload only the result ZIP highlighted as:

```text
[PRIMARY - UPLOAD THIS FILE]
```

When `collection_status=INCOMPLETE`, upload the result for diagnostics but do not treat missing evidence as proof of absence.

### v6.18.7 bounded COLLECT final status

A COLLECT that preserves usable evidence but cannot prove full coverage (timeout, result/report truncation, or discovery output quota) exits with `rc=3`, writes the result ZIP, and reports `SUMMARY: INCOMPLETE` rather than `SUMMARY: FAIL`. `FAIL` remains reserved for execution/schema/integrity failures.



## Mandatory project identity — TaskDeck workspaces and AI handoffs

TaskDeck now initializes `.python_patch_tool.json` on new workspaces **and migrates existing workspaces** on startup/first AI Pack/PATCH/COLLECT use without overwriting unrelated settings:

```json
{"project":{"key":"admin-go-tool","id":"<persistent UUID v4>"}}
```

`project.key` defaults to the workspace directory name; `project.id` is randomly generated **once** and retained across upgrades/restarts. Two workspaces named alike remain different because their IDs differ. A preexisting valid project key/ID is preserved; invalid config is never silently overwritten.

**AI guard (mandatory):** Treat `project.key` **and** `project.id` as one inseparable target identity, using the authoritative identity in the current AI Pack prompt or new TaskDeck handoff. Every newly generated `PATCH_TOOL_MANIFEST.json` and `CODE_COLLECTION_REQUEST_*.json` must include:

```json
"project": {"key": "admin-go-tool", "id": "<UUID shown by TaskDeck>"}
```

Compare both fields **before** reading, uploading, mixing, acting on, or generating a successor from an incoming COLLECT, FAIL_HANDOFF, patch, or failure evidence. If missing, malformed, inconsistent, or different from the target, **stop and refuse**; explicitly ask the user to confirm the source key/UUID, target key/UUID, and permission to proceed. Never assume matching paths, names, or filenames imply the same project. Never silently rewrite a foreign UUID to bypass a mismatch. Existing legacy handoffs lacking an ID are unverified, not automatically trusted because the workspace now has an ID; confirm with the user and collect a fresh identity-bound artifact before continuing.

Current output: `FAIL_SUMMARY.json`, `COLLECTION_MANIFEST.json` and incomplete COLLECT handoffs include `project_identity`; the ZIP also carries `PROJECT_IDENTITY.json` plus `PROJECT_IDENTITY_INSTRUCTIONS.txt`. The clear-text TXT companion includes these ZIP members. The browser Copy error log and Copy failure + handoff actions resolve the *current* workspace identity at copy time and refuse to copy if it cannot be verified. Incoming PATCH/COLLECT requests that declare key+id are checked before source modification/collection; legacy requests require AI/user confirmation.
