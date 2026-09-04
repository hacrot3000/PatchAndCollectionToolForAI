# AI / ChatGPT usage contract — Python Patch Tool v6.15.1

This document overrides older Patch Tool instructions when they conflict with the current package.

## Public workflow

The user normally runs one zero-argument public launcher:

```text
Linux/POSIX: ./tools/run_python_patches.sh
Windows:     tools\run_python_patches.bat
PowerShell:   .\tools\run_python_patches.ps1
```

All launchers resolve the same project root and call the same dispatcher/runner/collector. PATCH and COLLECT ZIPs are placed directly under `<project>/patchs/`. Windows requires Python 3.10+; the packaged PowerShell launcher probes `py -3`, `python`, then `python3`.

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

Do not invent PATCH manifest fields or COLLECT action names/fields. In particular, never create `source_baseline`; exact source assumptions belong in `preflight.files`. `overview` is valid because it has an exact current schema and implementation, not because an old document once mentioned it.

## PATCH correctness contract

AI-generated archive PATCHes must follow `PATCH_PACKAGE_GUIDE.md` and `PATCH_PACKAGE_SCHEMA.json`.

Patch Tool v6.15.1 validates/preflights before payload execution:

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

### Optional safe rollback

Rollback is never inferred. AI may add `recovery.rollback` only when it can declare the complete target set and exact baseline metadata required by `PATCH_PACKAGE_SCHEMA.json` / `PATCH_PACKAGE_GUIDE.md`. It is limited to payload/post-patch failures before Git policy. A rollback result is recorded as `PASS`, `PARTIAL`, `FAIL`, or `SKIPPED` in structured PATCH evidence. If the contract is incomplete, preflight rejects the package before project modification.

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

v6.15.1 ships the documented PATCH runner, utilities, readonly collector, schemas, dispatcher, progress supervisor and Windows launchers. The documented current contract does not require an older **private core**. Historical formats outside the current schemas fail closed rather than being guessed.

## Duplicate rules

- Current queue: same size + exact SHA-256 → keep first natural-order PATCH and remove redundant queue copies before selector.
- Local successful history: exact SHA-256 against direct `patchs/patched/` in this project → skip locally.
- No global/server/project-key/cross-machine duplicate database.

## Tool Health / install self-audit

The zero-argument selector exposes read-only Tool Health with key `h` (line selector: `h`). When the queue is empty, a compact health line is printed automatically. Health verifies the installed VERSION, `SHA256SUMS`, required self-contained runtime files, executable launcher and authoritative PATCH/COLLECT schemas. It never executes PATCH or downloads updates.

## Windows portability boundary

Windows uses the line selector because the fullscreen selector relies on POSIX `termios`. AI must not assume arrow/Space/priority-key UI is available on Windows; line selection still supports indexes/ranges, `a`, delete, inspect and health. Any `post_patch.commands[].argv` executable must exist on the target OS. Do not hard-code `bash`/`sh` for a Windows-targeted PATCH unless the project explicitly provides it.

## User-guide boundary

`tools/HUONG_DAN_PYTHON_PATCH_TOOL.html` stays intentionally minimal and user-oriented. Internal schema/action/preflight details belong in `tools/_patch_lib/docs/`, not in the user guide.


## v6.14.1 runtime robustness invariants (retained by v6.15.1)

- The PATCH queue root `patchs/` must be a real project-local directory; a symlinked/unsafe queue root fails closed.
- The exact PATCH package selected is snapshotted before preflight and the exact executed bytes are what PASS archival records. A same-name replacement with different bytes remains queued.
- COLLECT uses the same exact-request identity rule: the executed request snapshot is archived; a same-name replacement remains queued for a later invocation.
- Python PATCH payloads and post-patch commands are process-group managed. Timeout/SIGINT/SIGTERM must terminate descendants before rollback/result publication.
- FAIL_HANDOFF must never attach a current queue package whose SHA differs from the executed package SHA.
- Tool Health requires checksum coverage for all required runtime files and rejects unsafe symlink ancestors.

## v6.15.1 diagnostics contract

AI/package generators should also read `PATCH_PACKAGE_CHECKLIST.json`. When project source is available, use the read-only `validate --patch` route before handing a package to the user. A validate PASS is not an execution bypass: the runner repeats preflight immediately before payload.

Manifest lint reports all discoverable schema issues in the same pass, including unsupported fields and invalid timeout bounds. Source preflight reports all declared affected files, expected/actual SHA-256 and missing anchors. Recovery COLLECT requests are narrowed to those safe affected source paths.

Data-only OPS is sequentially dry-run against a temporary mirror before execution. This makes an OPS source/anchor mismatch a project-unchanged preflight failure. Arbitrary Python payload code is never executed during inspect/validate.

Windows zero-argument dispatch does not use the POSIX `.sh` internally. Native console fullscreen selection uses `msvcrt` + VT when available and falls back to line selection when safe fullscreen operation is unavailable.
