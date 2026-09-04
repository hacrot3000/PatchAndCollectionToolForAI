# PATCH PACKAGE GUIDE — v6.18.3 authoritative AI/tool contract

Machine-readable source of truth:

```text
tools/_patch_lib/docs/PATCH_PACKAGE_SCHEMA.json
```

AI must read that schema before generating a PATCH package. Do not invent manifest fields.

## Package shape

Preferred archive PATCH:

```text
patch_x.zip
├── PATCH_TOOL_MANIFEST.json
├── exactly-one Python entrypoint.py
└── resources/...                    # optional
```

or data-only OPS:

```text
patch_x.zip
├── PATCH_TOOL_MANIFEST.json
└── PATCH_TOOL_OPS.json
```

A package with both OPS and Python entrypoint, multiple Python entrypoints, unsafe archive members, or missing root manifest is rejected **before project modification**.

## Compatibility

Optional manifest block:

```json
{
  "compatibility": {
    "min_tool_version": "6.17.12",
    "max_tool_version": "7.0.0",
    "max_tested_version": "6.17.13"
  }
}
```

- `min_tool_version` / `max_tool_version` are hard execution gates.
- `max_tested_version` is a warning boundary, not a hard block.
- Versions are exact numeric semantic versions `X.Y.Z`.

## Targets and preflight

When AI knows the files a patch can modify, declare them:

```json
{
  "targets": ["src/a.c", "include/a.h"]
}
```

This improves failure/partial-modification diagnostics even for Python patches.

For exact source assumptions, declare preflight evidence:

```json
{
  "preflight": {
    "files": [
      {
        "path": "src/a.c",
        "sha256": "<64 hex>",
        "anchors": ["exact source anchor"]
      }
    ]
  }
}
```

If SHA/anchor/existence does not match, Patch Tool prints:

```text
PREFLIGHT FAIL — project unchanged
```

and may prepare a `CODE_COLLECTION_REQUEST_patch_recovery_*.zip` for the next invocation.

`preflight.require_clean_worktree=true` may be used only when a clean Git worktree is genuinely required by the patch.

## Required package resources

If a Python entrypoint requires packaged data, list it:

```json
{"resources":["resources/final/foo.c"]}
```

Missing/non-regular required resources are rejected before the payload runs.

## Recovery policy

Every PATCH failure generates a structured FAIL handoff and automatically discovers/bundles related current source. This is mandatory since v6.17.5. The legacy `recovery.fail_handoff` boolean remains schema-accepted for package compatibility, but `false` is deprecated and ignored with a warning.

Source drift/anchor mismatch may additionally prepare a read-only `pack` COLLECT request. That secondary next-run request may still be disabled independently:

```json
{
  "recovery": {
    "collect_on_source_drift": false
  }
}
```

FAIL_HANDOFF source discovery uses structured target/preflight/rollback evidence, source paths from failure output, bounded basename lookup, and one-hop local references/same-stem companions. The ZIP includes `SOURCE_DISCOVERY.json` so AI can distinguish attached source from files omitted by safety quotas.

## Metadata-driven safe rollback

Automatic rollback is **opt-in and fail-closed**. It is not a replacement for SANDBOX/worktree transactions. AI may request rollback only when every mutable target is known and has an exact preflight baseline.

Example:

```json
{
  "targets": ["src/a.c", "generated/new.h"],
  "preflight": {
    "files": [
      {"path": "src/a.c", "exists": true, "sha256": "<64 hex>"},
      {"path": "generated/new.h", "exists": false}
    ]
  },
  "recovery": {
    "rollback": {
      "targets": ["src/a.c", "generated/new.h"],
      "on": ["payload_failure", "post_patch_failure"],
      "max_total_bytes": 268435456
    }
  }
}
```

Rules:

- `recovery.rollback.targets` must exactly cover the PATCH target set resolved by preflight.
- Every rollback target needs an explicit `preflight.files` baseline. Existing files require `exists:true` plus exact SHA-256; initially absent files require `exists:false`.
- The runner snapshots only those declared regular-file targets, with a bounded total byte limit, after preflight PASS and before payload execution.
- Rollback may run only for `payload_failure` and/or `post_patch_failure`, before Git policy. It never attempts to undo a Git commit/push failure.
- Existing files are restored from exact snapshot bytes; files that were initially absent are removed only if they became a regular file/symlink at the exact declared target path. Directories/non-file objects are never recursively removed.
- On a Git project the pre/post worktree fingerprint verifies whether the whole project returned to baseline. If an undeclared file changed, status becomes `PARTIAL` even if declared targets were restored. Outside Git, verification is explicitly limited to declared targets.
- Missing/ambiguous recovery metadata becomes `PREFLIGHT FAIL — project unchanged`; AI must not guess a rollback contract.

`ROLLBACK: PASS` means the configured recovery scope was restored and verified according to the evidence available. `PARTIAL`/`FAIL` remains a normal PATCH failure and the generated FAIL_HANDOFF should be uploaded to AI.

## Rollback path/runtime safety — v6.14.1

For a rollback target declared with `exists:false`, the target's parent directory must already exist before the PATCH starts and every ancestor must be a real non-symlink directory inside the project. The tool does not invent or remove directories as part of rollback.

Rollback baselines are re-checked when the pre-payload snapshot is created. If a target changes between preflight and snapshot, execution fails closed before the payload runs.

On POSIX, restore pins the validated parent directory with a directory file descriptor and `O_NOFOLLOW`-style checks so an ancestor symlink swap cannot redirect the restore outside the project.

On POSIX, Python PATCH payloads and post-patch commands run in isolated process groups. Timeout, SIGINT or SIGTERM terminates the managed group before rollback/return. Windows uses the platform-specific subprocess branch; PATCH authors must not assume POSIX signal/process-group semantics there.

## Exact input lifecycle — v6.14.1

Patch Tool snapshots the exact PATCH package selected from `patchs/` before preflight/execution and archives those exact executed bytes after PASS. If another process or the PATCH itself replaces the same queue filename while execution is in progress, the replacement is preserved in `patchs/` for a later run instead of being archived as though it had already executed.

A FAIL_HANDOFF only embeds the current queue PATCH when its SHA-256 still equals the structured executed package SHA. A changed/replaced queue package is omitted from the handoff rather than misidentified.

## Inspect / dry-run

In the interactive selector the user can press `i` on a PATCH (line selector: `i <index>`). This runs the same package/schema/preflight checks, shows compatibility/targets/post commands/Git policy, and ends with:

```text
INSPECT RESULT: PASS — project unchanged
```

No PATCH payload is executed and the package is not archived.

## v6.17.6 recovery integrity

Cross-run recovery binds filesystem actions to the exact replay package recorded by the failed run (`requeued_as` plus recorded patch SHA-256). Historical `patch_file` / `patch_id` remain logical predecessor declarations only; a different package later occupying the old filename is not eligible for Retry/Delete/Run-after. Exact rollback replay may bypass duplicate-history suppression only when the failed-run report proves the same queue filename and SHA-256.

For a failed PATCH with no declared/effective target paths, an unchanged Git worktree fingerprint is insufficient to prove no modification because ignored files are outside that observation boundary. The partial state is therefore `unknown` and continuation safety-stops.

## v6.17.5 package lint / validate

Before delivering a PATCH, validate the manifest against `PATCH_PACKAGE_SCHEMA.json` and `PATCH_PACKAGE_CHECKLIST.json`. **Never invent `source_baseline`** or other legacy/custom fields. Source assumptions belong only in `preflight.files` using `path`, optional `exists`, `sha256`, and/or `anchors`.

Known migration:

```text
source_baseline.files[].file   -> preflight.files[].path
source_baseline.files[].sha256 -> preflight.files[].sha256
```

Timeout values, when present, must be integers `1..1800`. Omit `execution.timeout_seconds` to use the default; do not use `0` as an unlimited sentinel.

Read-only project-aware validation:

```text
Linux:   ./tools/run_python_patches.sh validate --patch patchs/example.zip
Windows: tools\run_python_patches.bat validate --patch patchs/example.zip
```

Validation/inspect results use one of four classes:

- `READY_TO_APPLY` — package and current source preflight are ready; project unchanged.
- `PATCH_INVALID` — package/schema/compatibility/environment contract is invalid; project unchanged.
- `SOURCE_DRIFT` — current source SHA/existence/anchor or OPS match assumptions do not hold; project unchanged.
- `TOOL_ERROR` — unexpected Patch Tool internal failure; project unchanged.

Schema lint reports multiple independent manifest issues in one pass. Source preflight likewise aggregates declared file mismatches and includes expected/actual SHA where applicable. For data-only OPS packages, v6.17.5 simulates the sequential operation list on a private temporary mirror before execution; a missing/ambiguous source match fails before the real payload is allowed to write.

## Batch metadata (v6.17.5)

PATCH có thể khai báo:

```json
"batch": {
  "depends_on": ["patch-id-before"],
  "on_dependency_failure": "block",
  "previous_failure": {
    "patch_id": "failed-patch-id",
    "patch_file": "failed_patch.zip",
    "action": "retry_before",
    "reason": "successor requires predecessor state"
  }
}
```

`depends_on` sử dụng `patch.id` hiện hữu. Tool stable-topological-sort batch theo dependency, fail closed nếu thiếu dependency hoặc cycle. `on_dependency_failure` mặc định là `block`; giá trị legacy `run_anyway` vẫn được schema đọc để tương thích nhưng runtime **ignore và vẫn BLOCKED** khi predecessor liên quan FAIL.

`previous_failure` là contract phục hồi queue giữa các lần chạy. Chỉ successor **có quan hệ** với unresolved predecessor (dependency, effective-target overlap, hoặc explicit declaration) phải chỉ rõ một trong `delete`, `retry_before`, `run_after`, `block`; PATCH độc lập không bị ràng buộc. Xem `AI_USAGE_CONTRACT.md` để biết quy tắc bắt buộc dành cho AI tạo PATCH.

### Batch policies

`.python_patch_tool.json`:

```json
{
  "batch": {
    "failure_policy": "continue_independent",
    "transaction_policy": "patch"
  }
}
```

`failure_policy`:
- `continue_independent`: **mặc định từ v6.17.5**. Sau failure đã được containment chứng minh an toàn, PATCH không phụ thuộc và không overlap effective target sẽ tự chạy tiếp.
- `fail_fast`: explicit opt-in để dừng ngay tại lỗi đầu tiên.
- Dependency FAIL hoặc runtime effective-target overlap với PATCH FAIL/BLOCKED làm successor `BLOCKED` mặc định; Ctrl+C/rollback failure/partial-unknown vẫn safety-stop.

`transaction_policy`:
- `patch`: rollback theo contract từng PATCH như trước.
- `batch`: atomic rollback theo effective target set của toàn batch. `manifest.targets` vẫn bắt buộc; tool mở rộng set bằng preflight/recovery/OPS targets resolve được trước execution. Dispatcher giữ project mutation lock xuyên snapshot→execution→rollback/commit; target/package snapshot có generation checks và POSIX restore dùng pinned dir-fd. Chế độ này từ chối side effects không target-bounded (`post_patch.commands`, Git add/commit/push) và verify state/SHA sau rollback.

Có thể override cho một lượt chạy:

```bash
./tools/run_python_patches.sh run --failure-policy continue_independent --transaction-policy batch
```

Windows dùng cùng options qua `.bat`/`.ps1`.

### v6.17.5 integrity notes

- Với OPS replace/insert, `already` phải là marker/context idempotency **explicit** nếu cần. Tool không còn coi `new` xuất hiện ở nơi bất kỳ trong file là “already patched”.
- OPS dry-run và execution chịu `execution.timeout_seconds`; write source dùng atomic replace.
- `git.commit=auto` fail-closed nếu target đã dirty trước PATCH để tránh commit lẫn local edit của operator.
- ZIP/TAR package bị giới hạn số entry, kích thước giải nén, compression ratio; symlink/non-regular member, duplicate/case-fold/Unicode collision và Windows drive/ADS names bị reject.
- Planner records the stable SHA-256 of each selected queue package. Batch snapshot and immediate pre-spawn checks must match those planned bytes; replacement/disappearance fails before payload with `package_input_changed`.
- Batch replay snapshots record exact SHA-256 + size and are reverified before requeue; corrupted/missing replay bytes become `batch_requeue_failed`.
- Mutation lock files reject symlink/reparse paths and POSIX uses no-follow opens; artifact runtime/report/handoff subdirectories are real-directory checked.
- FAIL_HANDOFF source freezing is per-file isolated: a source that vanishes or changes cannot delete an already frozen sibling attachment.

## v6.17.7 project identity and trusted validation profiles

Optional project binding:

```json
"project": {"key": "my-project"}
```

If present, the target project must configure the same key in `.python_patch_tool.json`; otherwise preflight fails before payload.

Validation profiles are named local policies:

```json
"validation": {"profiles": ["unit", "build"]}
```

The package does not define the commands. The project operator defines `validation_profiles.<name>.argv/cwd/timeout_seconds` locally. Missing/invalid profiles fail preflight. Profiles execute after payload/post-patch and before Git/archive. Batch-transaction mode rejects validation profiles because their side effects are not necessarily target-bounded.

## v6.17.8 failure-only commands and managed script execution

A PATCH may declare commands that run **only after a valid PATCH entered execution and then failed**:

```json
"on_failure": {
  "commands": [
    {
      "name": "capture failing build state",
      "argv": ["python3", "tools/capture_failure.py"],
      "cwd": ".",
      "timeout_seconds": 300
    }
  ]
}
```

`on_failure.commands` uses the same command object contract as `post_patch.commands`: argv is an argument array (never a shell string), `cwd` is project-relative, and `timeout_seconds` is an integer `1..1800` when present. Commands are non-interactive: stdin is closed, so automation must pass inputs through argv/files/environment rather than `input()`/terminal prompts.

Execution order on a PATCH failure is:

1. detect/classify the PATCH/post-validation failure;
2. attempt metadata-driven rollback when that failure trigger is configured;
3. run `on_failure.commands` sequentially;
4. recompute final project-delta evidence;
5. publish structured failure result / FAIL_HANDOFF.

The **original PATCH failure remains authoritative**. A failure-only command that exits non-zero, times out, or leaves descendants is recorded under `result.on_failure`, but does not replace the original PATCH rc. Such a failed/incomplete failure-handler is a global continuation safety-stop; unrelated PATCHes resume automatically only when the complete `on_failure` sequence PASSes and the final project state is proven unchanged. User interruption is different: Ctrl+C/SIGTERM is control flow and propagates to stop the run/batch; it is not converted into an ordinary command failure. `on_failure` is not executed for invalid package/schema/preflight rejection, nor for a user interruption before/while execution.

`transaction_policy=batch` rejects `on_failure.commands`, just as it rejects target-unbounded post commands / validation profiles / Git side effects.

### `post_patch.run_when_no_changes`

`post_patch.commands` normally runs only when the PATCH produced a detected project delta. Set `post_patch.run_when_no_changes=true` only when the post command is intentionally required even for an idempotent/no-op PATCH. When omitted or `false`, a no-change PATCH skips `post_patch.commands`. This flag changes only whether post commands run; it does not turn patch metadata into a runtime gate and it does not override failure/rollback rules.

### Patch metadata fields are descriptive, not implicit runtime gates

`patch.version`, `patch.phase`, `patch.phase_under_test`, `patch.summary` and `patch.regression_scope` are package metadata used for identification, reporting, planning/search and human/AI context. They are **not** automatically enforced as source/version/regression gates. If a PATCH requires a specific source baseline or project, encode that requirement with `preflight.files`, `manifest.project.key`, dependencies, validation profiles or other documented runtime gates. AI must not assume a descriptive metadata value is validated merely because the schema accepts it.

Managed payload/post/validation/failure/Git-policy execution is process-tree contained for timeout/interruption on supported platforms. On POSIX, a leader that exits while descendants remain is also detected: descendants are terminated and the command is reported failed. Windows normal-exit descendant detection still requires native verification; timeout/interrupt tree termination is covered by the packaged Windows contract. A program that deliberately returns exit code `124` is distinct from a Patch Tool timeout (`timed_out=true`). Git auto-policy accepts `git.timeout_seconds` (`1..1800`, default 300) and disables terminal credential prompts. Patch Tool internal result/lock environment variables are removed before untrusted/project commands are spawned.

### v6.17.8 dispatcher / internal-error execution boundary

- `inspect`, `preview`, `validate` and the COLLECT progress supervisor are launched by the dispatcher through a process-group-aware foreground runner; Ctrl+C/SIGTERM is forwarded to the child tree instead of leaving a detached helper behind.
- If an unexpected Patch Tool exception occurs **after payload execution has begun**, it is still an execution failure: configured rollback is attempted for payload/post-validation stages, then `on_failure.commands` runs. The internal-error rc/diagnosis remains primary.
- Internal exceptions before execution (package/schema/preflight) remain project-unchanged failures and do not run `on_failure`.


## v6.17.10 local project config + planning consistency

`.python_patch_tool.json` dùng chung một parser bounded, reject symlink/duplicate JSON keys cho các đường identity, validation, selector và batch policy. Nếu file config tồn tại nhưng malformed/unsafe thì `run`/`resume`/`plan` fail-closed rc=2; không tự đổi sang policy mặc định. Các option runtime chính:

```json
{
  "project": {"key": "my-project"},
  "validation_profiles": {
    "unit": {"argv": ["./tools/test.sh"], "cwd": ".", "timeout_seconds": 900}
  },
  "automation": {
    "zero_argument": {
      "selection": "prompt",
      "non_interactive_confirmed": false,
      "initial_selection": "none",
      "selector_ui": "auto"
    }
  },
  "batch": {
    "failure_policy": "continue_independent",
    "transaction_policy": "patch"
  }
}
```

`plan` dùng đúng effective `failure_policy` / `transaction_policy` từ config hoặc CLI override. `--export-recipe` ghi đúng các policy đó; nếu `transaction=batch` không tương thích với post commands, validation profiles, on-failure commands hoặc Git side effects thì `plan` fail trước preview/export, giống execution.

Persistent failure registry giữ nhiều unresolved entries. Planner xét tất cả entry liên quan; một PASS chỉ auto-resolve entry cùng logical patch **và cùng exact SHA-256**. Nếu một selected batch liên quan tới nhiều unresolved predecessor, planner yêu cầu Smart Resume xử lý trước.

`git.fail_on_error` mặc định `true`. Nếu đặt `false`, lỗi Git automation được in ra nhưng không đổi PATCH result thành FAIL; chỉ dùng khi Git policy thật sự advisory. `git.commit=auto` vẫn giữ dirty-target guard/index cleanup như mô tả ở trên.


Recipe policy override rule: `run --recipe` uses the policies stored in the recipe; `--failure-policy`/`--transaction-policy` must not be combined with `--recipe`. Create a new recipe with `plan` overrides when different policies are intended.

## v6.18.2 command-only / strict compatibility lane

A manifest-only command package is accepted when `post_patch.run_when_no_changes=true`, `no_change_reason` is 20–500 non-whitespace characters, and exactly one command is declared. Command-only packages automatically use the `legacy_strict` safety profile. A source-changing PATCH keeps the current v6 post-patch command contract unless `post_patch.safety_profile=legacy_strict` is explicitly requested. This additive split restores historical compatibility without silently narrowing modern v6 build/test workflows.
