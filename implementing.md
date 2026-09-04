# Python Patch Tool — implementing.md

Phiên bản mục tiêu: **v6.17.3**  
Trạng thái: **CONTROLLED BATCH ENGINE + SMART RESUME + ADVANCED REPORTING — COMPLETE / STOP**

## Baseline

Baseline kỹ thuật: **v6.16.0**. Release này mở rộng batch report thành batch execution engine có policy rõ ràng nhưng vẫn giữ các invariant cũ: PATCH chạy in-place, SANDBOX/worktree không quay lại, schema/preflight fail-closed, COLLECT chạy độc lập, duplicate-local skip-once → `patchs/ignore/`.

## Scope được chấp thuận

| Hạng mục | Trạng thái |
|---|---|
| Continue-on-failure có kiểm soát | COMPLETE |
| Dependency giữa các PATCH | COMPLETE |
| Bắt buộc successor xử lý predecessor FAIL | COMPLETE |
| Whole-batch preflight trước source write đầu tiên | COMPLETE |
| Batch transaction / rollback policy | COMPLETE |
| Smart resume | COMPLETE |
| Report browser nâng cao | COMPLETE |
| Source before/after trong report | COMPLETE |
| Run history management | COMPLETE |
| Support bundle từ report | COMPLETE |
| Native Windows runtime test lane | COMPLETE — packaged; native execution cần host Windows |
| Target-overlap/conflict analyzer | **NOT IMPLEMENTED — BY REQUIREMENT** |
| Patch provenance / identity | **NOT IMPLEMENTED — BY REQUIREMENT** |

## 1. Controlled continue-on-failure

Batch có hai policy:

- `fail_fast` — mặc định.
- `continue_independent` — PATCH độc lập sau một FAIL chỉ được chạy tiếp khi runner chứng minh project không bị partial modification, hoặc per-PATCH rollback đã PASS.

Các lỗi integrity/tool/rollback, Ctrl+C, hoặc partial/unknown project state tạo **SAFETY STOP** dù policy đang là continue. Tool không đánh đổi source integrity để cố chạy hết batch.

Config:

```json
{
  "batch": {
    "failure_policy": "continue_independent",
    "transaction_policy": "patch"
  }
}
```

Per-run override:

```bash
./tools/run_python_patches.sh run --failure-policy continue_independent
```

## 2. PATCH dependency

Manifest mới cho phép:

```json
"batch": {
  "depends_on": ["phase-1-patch-id"],
  "on_dependency_failure": "block"
}
```

- Dependency dùng `manifest.patch.id` hiện hữu; không thêm hệ provenance/identity mới.
- Tool stable-topological-sort batch theo dependency.
- Missing dependency hoặc cycle FAIL trước payload đầu tiên.
- Nếu dependency runtime FAIL:
  - mặc định successor = `BLOCKED`;
  - `run_anyway` chỉ chạy khi manifest chủ động cho phép.

## 3. Không để PATCH predecessor FAIL bị mồ côi

Nếu `LAST_RUN` còn predecessor FAIL trong `patchs/` và người dùng chọn successor thay vì retry predecessor, successor bắt buộc có:

```json
"batch": {
  "previous_failure": {
    "patch_id": "failed-id",
    "patch_file": "failed_patch.zip",
    "action": "delete",
    "reason": "successor supersedes predecessor after rebase"
  }
}
```

Action hợp lệ:

- `delete`: move package predecessor sang `patchs/ignore/YYYY-MM-DD-*` sau whole-batch preflight PASS.
- `retry_before`: tự đưa predecessor chạy trước successor.
- `run_after`: tự đưa predecessor chạy sau successor.
- `block`: chặn batch trước source write.

`reason` bắt buộc. `AI_USAGE_CONTRACT.md` ghi rõ đây là yêu cầu bắt buộc khi AI tạo successor từ FAIL_HANDOFF.

## 4. Whole-batch preflight

Trước payload đầu tiên:

1. validate schema/package/compatibility cho toàn bộ selected PATCH;
2. resolve dependency order/cycle/missing dependency;
3. enforce `previous_failure` action;
4. validate transaction compatibility;
5. chạy `validate` read-only cho từng PATCH.

PATCH phụ thuộc có thể được ghi `DEFERRED_AFTER_DEPENDENCY` nếu source mismatch hiện tại là do nó mô tả post-dependency state. Runner vẫn bắt buộc full preflight ngay trước PATCH đó. Schema/package/tool error không bao giờ được defer.

Nếu whole-batch preflight FAIL: **không PATCH payload nào được chạy**.

## 5. Batch transaction

`transaction_policy`:

- `patch`: rollback riêng từng PATCH như trước.
- `batch`: atomic rollback theo union của **effective targets** đã resolve trước batch.

Batch atomic mode fail-closed:

- mọi PATCH vẫn phải khai báo `manifest.targets` làm contract tối thiểu;
- dispatcher mở rộng effective target set bằng `manifest.targets` + `preflight.files[].path` + `recovery.rollback.targets` + target suy ra trực tiếp từ OPS;
- không cho `post_patch.commands`;
- không cho Git add/commit/push;
- dispatcher giữ mutation lock xuyên suốt `snapshot -> execute children -> rollback/commit`; child runner nhận lock-owner token để không deadlock;
- snapshot exact effective targets/package bytes bằng open-fd + generation checks (inode/size/mtime), rồi verify SHA/state sau rollback;
- snapshot exact package bytes của selected PATCH;
- nếu bất kỳ PATCH FAIL, toàn bộ effective targets đã snapshot được restore về pre-batch state;
- rollback FAIL có exit code riêng `70`; report phân biệt `batch_rollback_attempted`, `batch_rollback_status` và `batch_rolled_back`;
- PATCH đã PASS nhưng thay đổi bị batch rollback được requeue để replay/resume bằng publish **atomic no-overwrite**; concurrent queue replacement không bị ghi đè. Nếu source rollback PASS nhưng requeue replay package FAIL, transaction báo `REQUEUE_FAILED` và exit code `71` thay vì thoát exception/mất report.

Không tuyên bố atomicity cho path mà Python payload tự ghi nhưng không khai báo/không thể resolve trước. PATCH mutation được serialize theo project để tránh lost-update giữa hai terminal; selector/COLLECT vẫn độc lập.

## 5.1. Integrity hardening v6.17.3

- `internal_error` luôn safety-stop; nếu exception xảy ra sau payload thì partial state được tính lại, không mặc định `detected=false`.
- OPS idempotency chỉ dùng `already` được khai báo explicit; sự xuất hiện của `new` ở nơi khác trong file không còn được coi là bằng chứng đã patch.
- OPS write dùng same-directory temporary + `fsync` + `os.replace`; OPS dry-run/execution chạy trong managed subprocess và chịu `execution.timeout_seconds`.
- Git auto-commit fail nếu target đã dirty trước PATCH; guard chạy **trước `git add`**. Nếu staging do tool đã xảy ra nhưng commit không hoàn tất, tool reset chính touched paths để không làm bẩn Git index; `git commit` chỉ PASS khi return code bằng `0`.
- Archive extraction có giới hạn entry/member/expanded bytes/compression ratio, reject symlink/non-regular/collision và Windows drive/ADS path.
- COLLECT có hard ceiling cục bộ, bỏ qua output/artifact nội bộ, kiểm quota theo chunk và cảnh báo source/log có dấu hiệu secret trước upload.
- Regex COLLECT được cô lập trong worker subprocess và có hard timeout 60s cho mỗi search action.

## 6. Smart resume

Sau batch FAIL, interactive run hiển thị Smart Resume:

1. replay/retry toàn bộ unresolved items theo order cũ;
2. retry failed PATCHes;
3. chạy remaining/blocked items;
4. bỏ resume suggestion và dùng selector bình thường.

Nếu atomic batch rollback, PATCH từng PASS trước đó được đánh dấu `batch_rolled_back` và nằm trong nhóm cần replay.

CLI:

```bash
./tools/run_python_patches.sh resume --resume-mode all
./tools/run_python_patches.sh resume --resume-mode failed
./tools/run_python_patches.sh resume --resume-mode remaining
```

Dependency và predecessor-action rules vẫn áp dụng khi resume.

## 7. Advanced report browser

Report table hỗ trợ:

- `PASS`
- `FAIL`
- `BLOCKED`
- `PREFLIGHT_FAIL`
- `NOT_EXECUTED`
- `SKIPPED_*`

Menu:

- `N`: detail + full PATCH log;
- `a`: aggregate batch log;
- `p`: chỉ PASS;
- `x`: chỉ problem items;
- `c`: item có source changes;
- `d N`: source before/after diff;
- `s N`: tạo support bundle ZIP;
- `h`: history;
- `q`: exit.

## 8. Source before/after

Với PATCH có declared `targets`, dispatcher snapshot metadata/source text nhỏ trước và sau execution. Report ghi changed declared targets và tạo `source.diff` dạng unified diff khi file là UTF-8/text đủ nhỏ; binary/large file vẫn có size/SHA metadata.

Snapshot này dùng cho reporting, không thay thế transaction snapshot.

## 9. Run history management

```bash
./tools/run_python_patches.sh report --list
./tools/run_python_patches.sh report --pin <run_id>
./tools/run_python_patches.sh report --unpin <run_id>
./tools/run_python_patches.sh report --export <run_id>
./tools/run_python_patches.sh report --delete <run_id>
./tools/run_python_patches.sh report --cleanup
```

Pinned run không bị automatic retention cleanup. Export tạo ZIP chứa run JSON + persistent logs/report artifacts.

## 10. Support bundle từ report

Trong menu dùng `s N`, hoặc:

```bash
./tools/run_python_patches.sh report --run-id <run_id> --support-item 2
```

Support ZIP chỉ gom evidence liên quan item: report row, batch summary, detail/preflight log, source diff, FAIL_HANDOFF và recovery COLLECT nếu có. Không tự gom toàn repo.

## 11. Native Windows runtime lane

Package có:

```powershell
.\tools\run_windows_native_tests.ps1
```

Lane chạy thực trên Windows và kiểm tra BAT + PowerShell launcher, project path có space/Unicode, controlled continue batch, persistent report và Windows runtime contracts. Host build Linux không được phép giả vờ đây là native PASS; release chỉ ghi native lane **packaged** nếu chưa chạy trên máy Windows thật.

## 12. Mandatory FAIL_HANDOFF source collection — v6.17.3

Mọi **PATCH FAIL** đều phải tạo `FAIL_HANDOFF_*.zip` và tự thu source liên quan; không phụ thuộc diagnosis có `affected_paths` hay không. `recovery.fail_handoff=false` được giữ parser-compatible cho package cũ nhưng dispatcher cảnh báo và **không cho tắt** handoff nữa.

Thứ tự discovery fail-closed/bounded:

1. structured evidence: `diagnosis.affected_paths`, partial/project delta, `preflight.target_paths`, preflight checks/issues, rollback paths;
2. effective targets của đúng queue package nếu SHA vẫn khớp bytes đã chạy;
3. source path xuất hiện trong traceback/compiler/tool log;
4. nếu log chỉ có basename (`Foo.cpp:123`), bounded project scan tối đa 25.000 file, bỏ `.git`, build/dist, dependency/cache, `artifacts` và `patchs`;
5. mở rộng **một hop** tới quoted local code/config reference và file cùng stem (`.c/.h`, `.cpp/.hpp`, `.ts/.tsx`, ...).

Bundle ghi:

- `current_source/<relative-path>` — source được **snapshot ổn định theo generation** trước khi tạo ZIP; SHA-256 của từng attachment được ghi trong `SOURCE_DISCOVERY.json`. File biến mất/đổi trong lúc snapshot chỉ bị skip, không làm hỏng toàn bộ FAIL_HANDOFF;
- `SOURCE_DISCOVERY.json` — evidence/reason cho từng file, SHA-256 snapshot, số file scan, truncation, included/skipped và limits;
- `DETAIL.log` — full per-item log khi <=64 MiB; log quá lớn giữ phần đầu + cuối trong giới hạn 64 MiB để lỗi xuất hiện muộn vẫn có evidence; source-path scan dùng tối đa 32 MiB evidence đầu/cuối thay vì chỉ console capture 8 MiB;
- `FAIL_SUMMARY.json.source_discovery` — summary ngắn;
- sensitive-content warning vẫn hoạt động, không âm thầm redact source.

Hard limits hiện tại: tối đa 256 source files, 32 MiB/file, 128 MiB tổng source, 1 MiB text/reference scan mỗi seed. Khi vượt quota, handoff vẫn được tạo và `SOURCE_DISCOVERY.json` ghi rõ file bị bỏ qua. Child PATCH chết trước khi ghi structured result sẽ được dispatcher tạo minimal failure result để vẫn có thể đối chiếu package SHA và thu target metadata khi queue bytes còn nguyên.

## Regression/stop condition

Release chỉ chuyển sang COMPLETE khi:

- new batch-engine E2E PASS;
- old v6.16 regression PASS hoặc được cập nhật đúng contract mới;
- Windows lane packaging contract PASS;
- Tool Health PASS;
- exact `SHA256SUMS` coverage PASS;
- clean-extract public launcher smoke PASS;
- docs/version/checksum đồng bộ.

Sau khi đạt các điều kiện trên: **STOP**, không tự mở Target-overlap analyzer hoặc provenance/identity.
