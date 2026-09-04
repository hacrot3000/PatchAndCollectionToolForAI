# AI / ChatGPT usage contract — Python Patch Tool v6.17.10

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

Patch Tool v6.17.6 validates/preflights before payload execution:

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
- automatically discovered related current source files for every PATCH failure (`current_source/**`);
- `SOURCE_DISCOVERY.json` describing evidence, bounded scan, included/skipped files and limits;
- generated recovery COLLECT request when applicable.

If the tool prints:

```text
!!! [PRIMARY - UPLOAD THIS FILE] PATCH FAIL HANDOFF !!!
```

AI should ask the user to upload that ZIP rather than reconstructing failure state from copied console fragments.

From v6.17.5, FAIL_HANDOFF source collection is mandatory on every PATCH failure. Discovery starts from structured target/preflight/rollback evidence, then paths printed in traceback/compiler logs; a bare source basename may trigger a bounded project scan, followed by one-hop local reference/same-stem expansion. `recovery.fail_handoff=false` is backward-compatible syntax only and is ignored with a warning. The tool never performs an unbounded whole-repository dependency crawl; `SOURCE_DISCOVERY.json` records limits and omissions.

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

Select at most one `[COLLECT]` per invocation. **Never mix COLLECT and PATCH.** This is selection isolation: there is no global queue/selector lock, so separate terminals and COLLECT remain usable independently. v6.17.5 serializes only the PATCH source-mutation lane per project to prevent two PATCH processes from losing each other's read-modify-write changes. Unsupported actions/fields become `COLLECT INVALID` before collector execution.

On COLLECT PASS, the tool prints one result ZIP and a quality summary:

```text
COLLECT QUALITY: files=... | source=... | reports=... | zip=... | truncated=... | missing=...
```

If `truncated>0`, AI must treat evidence as bounded/incomplete and should request a narrower follow-up collection if the missing context matters.


## Self-contained runtime

v6.17.6 ships the documented PATCH runner, utilities, readonly collector, schemas, dispatcher, progress supervisor and Windows launchers. The documented current contract does not require an older **private core**. Historical formats outside the current schemas fail closed rather than being guessed.

## Duplicate rules

- Current queue: same size + exact SHA-256 → keep first natural-order PATCH and remove redundant queue copies before selector.
- Local successful history: exact SHA-256 against direct `patchs/patched/` in this project → skip locally.
- No global/server/cross-machine duplicate database. `project.key` is a local project identity guard, not a shared duplicate database.

## Tool Health / install self-audit

The zero-argument selector exposes read-only Tool Health with key `h` (line selector: `h`). When the queue is empty, a compact health line is printed automatically. Health verifies the installed VERSION, `SHA256SUMS`, required self-contained runtime files, executable launcher and authoritative PATCH/COLLECT schemas. It never executes PATCH or downloads updates.

## Windows portability boundary

Windows uses the line selector because the fullscreen selector relies on POSIX `termios`. AI must not assume arrow/Space/priority-key UI is available on Windows; line selection still supports indexes/ranges, `a`, delete, inspect and health. Any `post_patch.commands[].argv` executable must exist on the target OS. Do not hard-code `bash`/`sh` for a Windows-targeted PATCH unless the project explicitly provides it.

## User-guide boundary

`tools/HUONG_DAN_PYTHON_PATCH_TOOL.html` stays intentionally minimal and user-oriented. Internal schema/action/preflight details belong in `tools/_patch_lib/docs/`, not in the user guide.


## v6.14.1 runtime robustness invariants (retained by v6.17.5)

- The PATCH queue root `patchs/` must be a real project-local directory; a symlinked/unsafe queue root fails closed.
- The exact PATCH package selected is snapshotted before preflight and the exact executed bytes are what PASS archival records. A same-name replacement with different bytes remains queued.
- COLLECT uses the same exact-request identity rule: the executed request snapshot is archived; a same-name replacement remains queued for a later invocation.
- Python PATCH payloads and post-patch commands are process-group managed. Timeout/SIGINT/SIGTERM must terminate descendants before rollback/result publication.
- FAIL_HANDOFF must never attach a current queue package whose SHA differs from the executed package SHA.
- Tool Health requires checksum coverage for all required runtime files and rejects unsafe symlink ancestors.

## v6.17.6 recovery integrity contract

Recovery filesystem actions MUST bind to the exact package identity recorded by the failed run (`requeued_as` plus patch SHA-256 where present); historical predecessor filenames/IDs are declarations, not authority to operate on a newly replaced file. Exact rollback replay may bypass duplicate suppression only for that report-proven identity. A failed PATCH without declared/effective targets is not allowed to infer “unchanged” merely from a Git-clean fingerprint; the state is unknown and safety-stops. Unsafe queue/artifact/lock paths fail closed with a concise tool error.

## v6.17.5 diagnostics contract

AI/package generators should also read `PATCH_PACKAGE_CHECKLIST.json`. When project source is available, use the read-only `validate --patch` route before handing a package to the user. A validate PASS is not an execution bypass: the runner repeats preflight immediately before payload.

Manifest lint reports all discoverable schema issues in the same pass, including unsupported fields and invalid timeout bounds. Source preflight reports all declared affected files, expected/actual SHA-256 and missing anchors. Recovery COLLECT requests are narrowed to those safe affected source paths.

Data-only OPS is sequentially dry-run against a temporary mirror before execution. This makes an OPS source/anchor mismatch a project-unchanged preflight failure. Arbitrary Python payload code is never executed during inspect/validate.

v6.17.5 integrity rules: OPS `already` is explicit-only (never inferred merely because `new` occurs elsewhere); OPS diagnose/execution is a managed subprocess bounded by `execution.timeout_seconds`; source replacement is atomic. `git.commit=auto` refuses target files already dirty before PATCH, and commit return code must be exactly zero. `internal_error` is always a safety stop.

The dispatcher also binds planned dependency/effective-target metadata to the queue package SHA-256. The selected package must still match that SHA when a batch snapshot is taken and immediately before child execution; otherwise execution stops as `package_input_changed` before payload. Batch replay snapshots carry their own SHA-256 and size and are verified again before requeue. This is a transient execution-integrity check only; it does not introduce a provenance/trust identity system. Mutation lock files and Patch Tool artifact subdirectories reject symlink/reparse redirection.

Regex COLLECT search is isolated in a managed worker with a 60-second hard timeout per search action, so pathological Python `re` patterns fail rather than hang indefinitely.

FAIL_HANDOFF auto-discovered source and exact COLLECT source attachments intentionally preserve diagnostic bytes. If likely credentials/private keys are detected, v6.17.5 emits a sensitive-content warning so the operator can review the bundle before upload; it does not silently redact source needed for diagnosis.

Windows zero-argument dispatch does not use the POSIX `.sh` internally. Native console fullscreen selection uses `msvcrt` + VT when available and falls back to line selection when safe fullscreen operation is unavailable.

## v6.17.5 — Bắt buộc xử lý PATCH liền trước đã FAIL

Khi AI nhận một `FAIL_HANDOFF` và tạo PATCH kế tiếp để tiếp tục cùng luồng công việc, **không được bỏ mặc PATCH đã FAIL trong queue**. Nếu lần chạy gần nhất còn một PATCH lỗi chưa được giải quyết và người dùng chuẩn bị chạy một PATCH kế tiếp thay vì retry chính PATCH lỗi đó, PATCH kế tiếp **bắt buộc** khai báo `manifest.batch.previous_failure`.

Ví dụ:

```json
{
  "batch": {
    "previous_failure": {
      "patch_id": "m3-windows-preview-ports-r3",
      "patch_file": "patch_m3_windows_preview_ports_r3.zip",
      "action": "delete",
      "reason": "PATCH mới đã rebase toàn bộ thay đổi của predecessor lên source hiện tại"
    }
  }
}
```

`action` chỉ được chọn một trong bốn hành động có nghĩa rõ ràng:

- `delete`: PATCH kế tiếp thay thế predecessor; tool chỉ move **package PATCH lỗi** sang `patchs/ignore/YYYY-MM-DD-<tên gốc>`, không xóa source. Hành động này chỉ được áp dụng sau khi các global preflight gate cần thiết PASS; PREFLIGHT_FAIL item-local không liên quan không tự chặn action.
- `retry_before`: predecessor vẫn cần thiết; tool đưa PATCH lỗi chạy trước PATCH mới. Nếu predecessor tiếp tục lỗi, dependency/failure policy quyết định PATCH sau có bị block hay không.
- `run_after`: PATCH mới phải chạy trước để chuẩn bị source rồi predecessor được retry sau. Không dùng nếu `depends_on` tạo thứ tự ngược lại.
- `block`: AI xác định chưa an toàn để tiếp tục; tool chặn batch trước source write.

`reason` là bắt buộc. AI phải mô tả ngắn vì sao hành động đó đúng. Khuyến nghị khai báo cả `patch_id` và `patch_file` để tool đối chiếu chính xác với predecessor trong `LAST_RUN`/`UNRESOLVED_FAILURES.json`; filesystem action luôn bind thêm exact package SHA khi có.

**Quy tắc chống PATCH mồ côi:** sau khi nhận FAIL_HANDOFF, mọi PATCH successor phải chủ động quyết định số phận predecessor. Không được tạo PATCH mới rồi mặc định để người dùng tự đoán nên xóa PATCH cũ, retry trước hay chạy lại sau.

## v6.17.5 — Dependency và failure policy

Dependency dùng `manifest.batch.depends_on` với `manifest.patch.id` đã tồn tại trong schema; đây không phải cơ chế provenance mới.

```json
{
  "patch": {"id": "phase-2"},
  "batch": {
    "depends_on": ["phase-1"],
    "on_dependency_failure": "block"
  }
}
```

- Dependency bắt buộc phải có mặt trong batch được chọn; thiếu dependency hoặc cycle làm whole-batch preflight FAIL trước source write.
- `on_dependency_failure` mặc định là `block`.
- `run_anyway` là giá trị legacy chỉ còn được schema chấp nhận để tương thích; **không dùng trong PATCH mới**. Runtime luôn `BLOCKED` khi dependency hoặc related-target predecessor đã FAIL.
- Không dùng dependency để thay thế source preflight. Runner vẫn preflight lại ngay trước từng PATCH.

## v6.17.5 — Whole-batch preflight và transaction

Trước PATCH đầu tiên, dispatcher kiểm tra schema/package/compatibility/dependency/predecessor-action cho cả batch và chạy validate read-only cho từng PATCH. PATCH phụ thuộc có thể được ghi `DEFERRED_AFTER_DEPENDENCY` nếu source hiện tại chỉ mismatch vì nó mô tả trạng thái sau dependency; runner vẫn bắt buộc preflight đầy đủ ngay trước execution.

**v6.17.9:** với `continue_independent` + `transaction=patch`, một validate read-only FAIL của riêng một PATCH trở thành `PREFLIGHT_FAIL` item-local. PATCH độc lập tiếp tục; PATCH phụ thuộc/overlap target bị `BLOCKED`. Global preflight gate, `transaction=batch`, hoặc explicit `fail_fast` vẫn dừng trước source write.

`batch.transaction_policy = batch` là atomic rollback theo **effective target set resolve trước**, không phải sandbox/worktree. Khi dùng policy này:

- mọi PATCH vẫn phải khai báo `manifest.targets`;
- tool mở rộng target set bằng `preflight.files[].path`, `recovery.rollback.targets` và target suy ra trực tiếp từ OPS;
- `post_patch.commands` bị từ chối;
- Git add/commit/push bị từ chối;
- dispatcher giữ project mutation lock xuyên suốt snapshot → child PATCHes → rollback/commit; runner con chỉ bypass acquire khi nhận đúng lock-owner token;
- tool snapshot effective targets và exact package bytes bằng generation-checked file descriptors trước execution;
- rollback phải verify final state/SHA trước khi được báo PASS;
- nếu batch FAIL, effective targets được restore và package đã PASS nhưng bị rollback được requeue để smart resume có thể replay.

Không suy diễn atomicity cho side effect mà Python payload tạo ra ngoài mọi target đã khai báo/resolve trước.

## v6.17.7 local project policy

When `manifest.project.key` is present, it is an enforced runtime identity guard. The PATCH must use the exact project key configured by the operator in `.python_patch_tool.json`; never invent a project key if it is unknown.

`manifest.validation.profiles` contains **profile names only**. Validation commands are trusted local project configuration under `validation_profiles`; AI-generated PATCH packages must not assume or redefine their argv. If a required profile is unknown, request/collect the current project config or omit the profile rather than inventing it.

The tool now maintains a local `PATCH_LEDGER.json` (`patch.id + SHA-256`) and warns when the same patch id is reused for different package bytes. This is provenance-light evidence only, not a signature or trust assertion.

For reproducible multi-PATCH replay, `plan --export-recipe` can export exact package filenames + SHA-256. A recipe mismatch must be fixed/rebased; do not bypass the SHA binding.

## v6.17.8 — failure-only command contract

AI may declare `manifest.on_failure.commands` when a valid PATCH needs a deterministic cleanup/diagnostic command **only if execution fails**. Use the exact same command-object rules as `post_patch.commands` (`name`, argv array, project-relative `cwd`, bounded `timeout_seconds`). Do not use a shell-string wrapper merely for sequencing; put each command in the ordered `commands` array.

Failure-only commands run after the configured rollback attempt. The PATCH's original failure/rc is primary; an `on_failure` error is secondary structured evidence. A failed/timeout/lingering `on_failure` command safety-stops automatic continuation; do not rely on later independent PATCHes running after cleanup failure. They are not a mechanism for handling an invalid manifest, preflight/source-drift rejection, or user Ctrl+C. Whole-batch atomic transaction mode rejects them because their side effects cannot be proven target-bounded.

All AI-declared commands are non-interactive. Do not depend on stdin/password/confirmation prompts. Managed command completion requires the full contained process tree to finish; detached/background work is invalid. `git.timeout_seconds` may be declared for automated Git policy and must remain in `1..1800`.

### v6.17.8 internal-error rule

Once payload execution has begun, an unexpected runner exception is treated as a PATCH execution failure for recovery purposes: applicable rollback first, then `on_failure.commands`. AI must not assume failure-only commands run for package/schema/preflight rejection or for user interruption. Dispatcher foreground routes (`inspect`/`preview`/`validate`/COLLECT supervisor) are signal/tree-contained as well. POSIX normal-exit orphan detection is runtime-tested; Windows native normal-exit orphan detection is not claimed by the Linux release lane.


## v6.17.10 — metadata and post-patch semantics

`patch.version`, `patch.phase`, `patch.phase_under_test`, `patch.summary` and `patch.regression_scope` are descriptive metadata. They do not create implicit runtime gates. Put enforceable source assumptions in `preflight.files`, project binding in `manifest.project.key`, ordering in batch dependencies, and executable checks in trusted validation profiles.

`post_patch.run_when_no_changes` defaults to `false`. Therefore `post_patch.commands` is skipped when the PATCH produced no detected project delta unless the manifest explicitly sets `run_when_no_changes=true`. This option does not change PATCH success/failure semantics; it only controls whether post commands also run for an idempotent/no-op PATCH.

## v6.17.10 — unresolved predecessor consistency

- Cross-run enforcement áp theo **PATCH successor thực sự liên quan**, không theo item đầu của batch. Quan hệ gồm `depends_on`, effective-target overlap, hoặc chính successor chủ động khai báo `batch.previous_failure`.
- PATCH độc lập không phải xử lý failure cũ.
- Nếu batch đang chọn liên quan đồng thời tới nhiều unresolved predecessor, planner fail-closed với `multiple_previous_failures_action_required`; dùng Smart Resume để Retry/Delete các predecessor trước vì manifest hiện chỉ có một `previous_failure` object.
- Một PATCH PASS chỉ tự resolve registry entry cũ khi logical identity **và exact SHA-256** khớp. Reuse `patch.id` với bytes khác không phải bằng chứng predecessor đã được sửa.
