# AI / ChatGPT usage contract — Python Patch Tool v6.13.0

This document overrides older Patch Tool instructions when they conflict with the current package.

## Public workflow

The user normally runs only:

```bash
./tools/run_python_patches.sh
```

PATCH and COLLECT ZIPs are placed directly under `<project>/patchs/`.

Before creating artifacts, AI should read **all files under `tools/_patch_lib/docs/`**. When Patch Tool itself is being developed, also read:

- `tools/implementing.md`
- `tools/PYTHON_PATCH_TOOL_FEATURES_VI.md`

## Exact schemas — never guess

PATCH source of truth:

```text
tools/_patch_lib/docs/PATCH_PACKAGE_SCHEMA.json
```

COLLECT source of truth:

```text
tools/_patch_lib/docs/COLLECT_ACTION_SCHEMA.json
```

Do not invent PATCH manifest fields or COLLECT action names/fields. `overview` is valid because it has an exact current schema and implementation, not because an old document once mentioned it.

## PATCH correctness contract

AI-generated archive PATCHes must follow `PATCH_PACKAGE_GUIDE.md` and `PATCH_PACKAGE_SCHEMA.json`.

Patch Tool v6.13.0 preflights before payload execution:

- manifest schema;
- payload ambiguity/entrypoint;
- tool min/max compatibility;
- declared package resources;
- declared current-file existence/SHA-256/exact anchors;
- post-patch command cwd + executable availability;
- required Git executable/worktree when Git policy is active.

A preflight failure is a **project-unchanged failure** and is reported as `PREFLIGHT FAIL — project unchanged`.

AI should declare `targets` for Python PATCHes whenever known. This makes partial-modification diagnosis reliable even outside Git.

PATCH remains **in-place**. SANDBOX/detached worktree transaction execution is permanently removed.

## PATCH failure recovery

PATCH failures create structured evidence under `artifacts/patch_tool/`:

- `LAST_RUN.json` — current machine-readable run result;
- `history/*.json` — bounded local run history;
- `fail_handoffs/FAIL_HANDOFF_*.zip` — primary artifact to upload to AI on PATCH failure.

A FAIL handoff can contain:

- original failed PATCH package;
- bounded console log;
- structured diagnosis/preflight/partial-modification metadata;
- Patch Tool version/schema/task context;
- safe relevant current source files when determinable;
- generated recovery COLLECT request when applicable.

If the tool prints:

```text
!!! [PRIMARY - UPLOAD THIS FILE] PATCH FAIL HANDOFF !!!
```

AI should ask the user to upload that ZIP rather than reconstructing failure state from copied console fragments.

For `source_drift` / `anchor_mismatch`, Patch Tool may create:

```text
patchs/CODE_COLLECTION_REQUEST_patch_recovery_<patch-sha>.zip
```

It is **prepared only**, never automatically executed. The user chooses it in the next invocation.

## COLLECT contract

AI returns exactly one request ZIP, never loose JSON:

```text
CODE_COLLECTION_REQUEST_<purpose>_<timestamp>.zip
└── CODE_COLLECTION_REQUEST_<purpose>_<timestamp>.json
```

The inner request must validate against `COLLECT_ACTION_SCHEMA.json`.

Select at most one `[COLLECT]` per invocation. **Never mix COLLECT and PATCH.** This is selection isolation only; there is **no project/process lock**, so the operator may intentionally run other terminals independently. Unsupported actions/fields become `COLLECT INVALID` before collector execution.

On COLLECT PASS, the tool prints one result ZIP and a quality summary:

```text
COLLECT QUALITY: files=... | source=... | reports=... | zip=... | truncated=... | missing=...
```

If `truncated>0`, AI must treat evidence as bounded/incomplete and should request a narrower follow-up collection if the missing context matters.


## Self-contained runtime

v6.13.0 ships the documented PATCH runner, utilities, readonly collector, schemas, dispatcher and progress supervisor. The documented current contract does not require an older **private core**. Historical formats outside the current schemas fail closed rather than being guessed.

## Duplicate rules

- Current queue: same size + exact SHA-256 → keep first natural-order PATCH and remove redundant queue copies before selector.
- Local successful history: exact SHA-256 against direct `patchs/patched/` in this project → skip locally.
- No global/server/project-key/cross-machine duplicate database.

## User-guide boundary

`tools/HUONG_DAN_PYTHON_PATCH_TOOL.html` stays intentionally minimal and user-oriented. Internal schema/action/preflight details belong in `tools/_patch_lib/docs/`, not in the user guide.
