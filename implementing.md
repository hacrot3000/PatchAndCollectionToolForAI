# v6.18.3 mandatory continuity gate — NO SILENT REMOVAL

Before modifying Patch Tool, read `tools/_patch_lib/docs/NO_SILENT_REMOVAL_POLICY.md`, `CAPABILITY_LEDGER.md`, and `HISTORICAL_FEATURE_BASELINE_V5_15.md`. Do not delete or narrow any capability previously marked COMPLETE/PRESERVED/COMPATIBILITY_RESTORED unless the user explicitly requests it or a later documented contract supersedes it. Every intentional transition must be recorded in the ledger and protected by a behavioral test. Surface/string-only compatibility tests are not sufficient.

# Python Patch Tool — implementing.md

Phiên bản mục tiêu: **v6.18.3**  
Trạng thái: **UPGRADE CONTINUITY + RESTORED EMPTY-QUEUE HISTORY — COMPLETE**

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
| Runtime failed-target overlap guard | **COMPLETE v6.17.5** |
| Static target-overlap/conflict analyzer trước execution | **COMPLETE v6.17.7** |
| Local patch ledger / ID reuse detection | **COMPLETE v6.17.7** |
| Cryptographic provenance / signature trust | **NOT IMPLEMENTED** |

## 1. Controlled continue-on-failure

Batch có hai policy:

- `continue_independent` — **mặc định từ v6.17.5**. PATCH độc lập sau một FAIL tiếp tục tự động khi failure đã được containment chứng minh an toàn.
- `fail_fast` — vẫn hỗ trợ như explicit opt-in khi muốn dừng ngay ở lỗi đầu tiên.

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

Ngoài dependency khai báo, v6.17.5 dùng **runtime failed-target overlap guard**: PATCH sau có effective target trùng với PATCH đã FAIL/BLOCKED sẽ bị `BLOCKED` mặc định; PATCH target độc lập vẫn chạy. Đây không phải static target/conflict analyzer giữa mọi PATCH.

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
  - `run_anyway` chỉ còn được schema chấp nhận để tương thích package cũ; runtime **luôn BLOCKED** khi dependency/related-target predecessor đã FAIL.

## 3. Không để PATCH predecessor FAIL bị mồ côi

Nếu registry/LAST_RUN còn predecessor FAIL chưa resolve và PATCH được chọn **có quan hệ** (dependency, effective-target overlap, hoặc chủ động khai báo `previous_failure`) thì successor liên quan bắt buộc có:

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

- `delete`: move package predecessor sang `patchs/ignore/YYYY-MM-DD-*` sau khi các global preflight gate an toàn đã PASS; một PREFLIGHT_FAIL không liên quan ở item khác không tự chặn action này.
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

v6.17.9 phân loại preflight failure theo scope:

- **Global preflight failure** (transaction compatibility/resource/planning integrity, hoặc `transaction_policy=batch`, hoặc explicit `fail_fast`) vẫn dừng toàn batch trước source write.
- Với mặc định `continue_independent` + `transaction_policy=patch`, **item-local read-only preflight failure** (`PREFLIGHT_FAIL`, ví dụ `project_identity_unconfigured`/source drift độc lập) chỉ đánh fail PATCH đó. PATCH phụ thuộc hoặc overlap effective target bị `BLOCKED`; PATCH độc lập vẫn tiếp tục.
- `PREFLIGHT_FAIL` không chạy payload và không chạy `on_failure.commands`, vì execution chưa bắt đầu.

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

## 5.1. Integrity hardening v6.17.5

- `internal_error`/tool failure chỉ cho phép continuation khi post-failure evidence chứng minh project không đổi; partial/unknown state vẫn safety-stop. Exception sau payload phải recompute partial state, không mặc định `detected=false`.
- OPS idempotency chỉ dùng `already` được khai báo explicit; sự xuất hiện của `new` ở nơi khác trong file không còn được coi là bằng chứng đã patch.
- OPS write dùng same-directory temporary + `fsync` + `os.replace`; OPS dry-run/execution chạy trong managed subprocess và chịu `execution.timeout_seconds`.
- Git auto-commit fail nếu target đã dirty trước PATCH; guard chạy **trước `git add`**. Nếu staging do tool đã xảy ra nhưng commit không hoàn tất, tool reset chính touched paths để không làm bẩn Git index; `git commit` chỉ PASS khi return code bằng `0`.
- Archive extraction có giới hạn entry/member/expanded bytes/compression ratio, reject symlink/non-regular/collision và Windows drive/ADS path.
- COLLECT có hard ceiling cục bộ, bỏ qua output/artifact nội bộ, kiểm quota theo chunk và cảnh báo source/log có dấu hiệu secret trước upload.
- Regex COLLECT được cô lập trong worker subprocess và có hard timeout 60s cho mỗi search action.
- **Execution-byte binding v6.17.5:** metadata/dependency/effective-targets được bind với SHA-256 của đúng queue package khi plan. Batch transaction bắt buộc snapshot đủ mọi selected PATCH và SHA phải còn khớp planned bytes; ngay trước child spawn dispatcher verify lại SHA. Package biến mất/thay byte sau plan = `package_input_changed`, payload không được chạy. Đây là execution-integrity guard, **không phải** provenance/identity subsystem.
- Batch replay snapshot lưu `stored + sha256 + size`; nếu exact replay snapshot bị sửa/hỏng sau transaction snapshot, requeue fail-closed với `batch_requeue_failed`, không requeue byte đã bị thay.
- Mutation lock directory/file reject symlink/reparse; POSIX mở lock leaf bằng `O_NOFOLLOW`. Lock path bất thường không được phép truncate/write xuyên symlink.
- Artifact subdirectories quan trọng (`runs`, `history`, `runtime`, `fail_handoffs`, `support`, `exports`) phải là real directories. FAIL_HANDOFF có fallback về hardened `artifacts/patch_tool/` nếu riêng `fail_handoffs/` bị hỏng/không an toàn.
- FAIL_HANDOFF source snapshot dùng per-file independent state + no-follow descriptor; lỗi/disappearance của source N không được xóa snapshot source N-1 hoặc làm mất toàn bộ handoff.

## 5.2. Recovery integrity hardening v6.17.6

- SMART RESUME và `batch.previous_failure` tách **logical predecessor identity** khỏi **filesystem package identity**: tên/id lịch sử chỉ dùng để validate manifest; thao tác Retry/Delete/Run bind tới exact `requeued_as` và SHA-256 đã ghi trong failure report. File mới chiếm lại tên cũ không được phép bị coi là predecessor lỗi.
- Exact replay package do batch rollback tạo được phép vượt session/history duplicate filtering **chỉ** khi `LAST_RUN` failure report xác nhận chính xác `requeued_as + patch_sha256`. Duplicate thông thường vẫn giữ hành vi skip/move-to-ignore cũ.
- Với PATCH không có declared/effective target, Git fingerprint không thấy delta **không còn là bằng chứng project sạch** vì `.gitignore` có thể che thay đổi. Trạng thái này là `partial_modification.detected=null/unknown` và gây safety-stop sau failure.
- Queue/artifact/lock filesystem safety violation được trả về lỗi gọn `rc=2`; riêng artifact root không an toàn thì tool không cố ghi `LAST_RUN` qua chính path không an toàn và không văng traceback ngoài ý muốn.

## 6. Smart resume

Sau batch/PATCH FAIL, interactive run dùng **menu mũi tên** thay cho nhập số:

- `↑/↓`: di chuyển; `Enter`: chọn; bên dưới luôn có **MÔ TẢ MỤC ĐANG CHỌN**.
- Retry/replay toàn bộ unresolved.
- Retry PATCH lỗi.
- Chạy remaining/blocked.
- **COLLECT source của PATCH lỗi**: tự dựng CODE_COLLECTION_REQUEST từ source/evidence của failure; nhiều PATCH lỗi được chọn bằng checkbox và chạy COLLECT tuần tự từng request.
- **Xóa PATCH lỗi khỏi hàng đợi**: chuyển an toàn sang `patchs/ignore/` (không unlink vĩnh viễn).
- Bỏ Smart Resume và mở selector queue bình thường.

Nếu có nhiều PATCH lỗi, các thao tác Retry/COLLECT/Delete mở selector giống queue: `↑/↓`, `Space`, `a`, `n`, `Enter`. Nếu atomic batch rollback, PATCH từng PASS trước đó được đánh dấu `batch_rolled_back` và nằm trong nhóm cần replay.

CLI non-interactive vẫn hỗ trợ:

```bash
./tools/run_python_patches.sh resume --resume-mode all
./tools/run_python_patches.sh resume --resume-mode failed
./tools/run_python_patches.sh resume --resume-mode remaining
```

Dependency, runtime failed-target overlap và predecessor-action rules vẫn áp dụng khi resume.

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

## 12. Mandatory FAIL_HANDOFF source collection — v6.17.5

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

## 13. v6.17.7 — project policy, planning và persistent state

### Project Identity Guard

`manifest.project.key` nay là runtime contract thật. Nếu PATCH khai báo project key, project phải cấu hình cùng key trong `.python_patch_tool.json`:

```json
{
  "project": {"key": "bletonfc"}
}
```

Thiếu key local => `project_identity_unconfigured`; key khác => `project_mismatch`. Cả hai fail trước payload và project unchanged.

### Trusted Validation Profiles

`manifest.validation.profiles` chỉ chứa tên profile. Command được định nghĩa local trong project, PATCH/AI không được nhét command vào field này:

```json
{
  "validation_profiles": {
    "unit": {
      "argv": ["./tools/test.sh"],
      "cwd": ".",
      "timeout_seconds": 900,
      "description": "Focused regression"
    }
  }
}
```

Profile thiếu/invalid/executable thiếu fail ở preflight. Profile được chạy bằng managed subprocess sau payload/post-patch và trước Git/archive. Chỉ tên profile được persist vào result; resolved argv local không được ghi vào FAIL/report metadata. `transaction_policy=batch` reject validation profiles vì command có thể tạo side-effect ngoài effective targets.

### Persistent Unresolved-Failure Registry

`artifacts/patch_tool/UNRESOLVED_FAILURES.json` giữ PATCH FAIL/PREFLIGHT_FAIL qua nhiều phiên, không phụ thuộc `LAST_RUN.json`. Failure chỉ tự resolve khi **cùng logical patch và cùng exact package SHA-256** PASS; reuse `patch.id` với bytes khác không resolve failure cũ. User Delete trong Smart Resume hoặc `previous_failure.action=delete` thành công vẫn resolve explicit predecessor. Registry không chặn PATCH độc lập: sau khi `LAST_RUN` đã đổi, planner chỉ tái áp `batch.previous_failure` khi PATCH mới phụ thuộc vào failed patch id hoặc overlap effective target của failure; như vậy continuation độc lập vẫn giữ nguyên nhưng successor liên quan không thể lách recovery contract.

### Static Batch Conflict Analyzer

Trước execution, selected PATCHes được phân tích theo effective target + dependency closure:

- `ORDER-DEPENDENT`: overlap nhưng không có dependency ordering;
- `DEPENDENCY-ORDERED`: overlap đã được dependency chain sắp thứ tự.

Analyzer chỉ cảnh báo/report; không tự đổi thứ tự và không suy đoán arbitrary Python side effects ngoài declared/effective targets.

### `plan` + OPS diff preview

```bash
./tools/run_python_patches.sh plan
./tools/run_python_patches.sh plan --export-recipe BATCH_RECIPE.json
```

`plan` là read-only: resolve order/dependency, project/profile preflight, static overlap, package SHA, ledger warning, disk/resource estimate và preview từng PATCH. OPS được chạy trên private mirror để tạo unified diff; arbitrary Python payload chỉ báo deterministic diff unavailable và không được execute để “đoán”. Selector thường có phím `p` để preview PATCH hiện tại.

### Patch Ledger / ID reuse detection

`artifacts/patch_tool/PATCH_LEDGER.json` lưu `patch.id + package SHA-256 + status/run metadata`. Nếu cùng `patch.id` xuất hiện với SHA khác, run/plan cảnh báo `PATCH ID REUSE`. Đây là provenance-light cục bộ, không phải chữ ký/PKI/trust subsystem.

### Reproducible Batch Recipe

`plan --export-recipe` ghi exact filename + SHA + patch id + batch policies + project key. Chạy lại:

```bash
./tools/run_python_patches.sh run --recipe BATCH_RECIPE.json
```

Thiếu package, SHA khác, patch id khác hoặc project key khác => fail trước payload. Exact recipe package được bảo vệ khỏi duplicate-history suppression trong invocation đó, nhưng byte vẫn phải đúng SHA recipe.

### Disk/resource preflight

Trước PATCH batch, dispatcher ước lượng package snapshot + target snapshot/rollback overhead và kiểm free space trên project volume + temp volume. Không đủ dung lượng => `insufficient_disk_space` trước source write. COLLECT/FAIL_HANDOFF vẫn giữ quota riêng đã có.

### Queue search/filter

Fullscreen selector: `/` nhập filter; line selector: `/text`. Filter tìm trong filename, kind/detail, `patch.id`, summary và effective targets. Enter trống sau `/` bỏ filter. Search không thay execution semantics.

### Phạm vi identity

v6.17.7 đã có project identity + local ledger + recipe SHA binding. **Cryptographic provenance, signature/PKI, remote trust registry vẫn không được triển khai**.

## 14. v6.17.8 — failure-only commands + script execution correctness

### `manifest.on_failure.commands`

PATCH package có thể khai báo command chỉ chạy khi PATCH đã qua preflight, thực sự bước vào execution và sau đó FAIL. Command object dùng cùng contract với `post_patch.commands`: `name`, `argv[]`, `cwd`, `timeout_seconds` (`1..1800`).

Thứ tự failure path: classify lỗi → rollback attempt nếu trigger được bật → chạy `on_failure.commands` → snapshot lại final project delta → ghi structured result / FAIL_HANDOFF. RC/lỗi gốc của PATCH luôn là primary; lỗi/timeout của `on_failure` được ghi riêng và không che nguyên nhân gốc. Nếu failure-command không PASS thì continuation safety-stop; chỉ khi toàn sequence PASS và final project delta proven unchanged mới cho PATCH độc lập tiếp tục. Ctrl+C/SIGTERM propagate để dừng batch, không bị biến thành rc command bình thường. Batch transaction atomic reject `on_failure.commands` vì side effect không target-bounded.

### Script/process execution hardening

- managed commands đóng stdin (`DEVNULL`), không được chiếm input của selector;
- timeout có cờ riêng, không suy timeout từ `rc == 124`;
- POSIX: leader exit nhưng process-group descendant còn sống sau drain grace => terminate tree + execution failure (`effective rc=125` khi leader rc=0); Windows timeout/interrupt tree termination có contract riêng, còn normal-exit descendant detection chưa được tuyên bố native PASS;
- Git add/commit/push/reset dùng managed process tree, `git.timeout_seconds` mặc định 300, `GIT_TERMINAL_PROMPT=0`; commit hook timeout không được để child orphan/staging leak;
- Git fingerprint/status observation tắt fsmonitor/external diff/textconv helper và Git command failure trở thành unknown/error, không được coi là clean;
- batch read-only validate timeout gửi signal cho runner để runner cleanup nested OPS worker trước force-kill;
- payload/post/validation/on-failure/Git command không được inherit `PTV_PATCH_RESULT_FILE`, COLLECT result channel hoặc parent mutation-lock token/key;
- COLLECT Git context đánh dấu section Git command FAIL rõ ràng thay vì đóng stderr như dữ liệu thành công;
- collector parent exit nhưng descendant vẫn giữ stdout/process tree => cleanup và không báo false PASS.

Windows vẫn có packaged process-group/taskkill/CTRL_BREAK contract; native PASS chỉ được tuyên bố sau khi chạy lane trên Windows thật.

### 14.1 Dispatcher foreground lifecycle + internal-error recovery

- `inspect` / `preview` / `validate` no longer use bare `subprocess.run`; dispatcher owns a new process group, applies a bounded outer guard, and forwards Ctrl+C/SIGTERM before force-kill fallback.
- COLLECT remains intentionally without a global wall-clock timeout because its request already has bounded file/byte/action limits and may be long-running, but dispatcher now owns/forwards its foreground process-tree lifecycle.
- `internal_error` after execution begins now follows the same recovery boundary as an ordinary execution failure: rollback when the configured stage supports it, then `on_failure`; preflight/internal package failures still never run failure commands.


## 15. v6.17.9 — item-local batch preflight continuation

Sửa regression orchestration: whole-batch read-only validation vẫn chạy trước source write, nhưng kết quả FAIL của từng PATCH không còn tự biến mọi PATCH khác thành `NOT_EXECUTED` dưới policy mặc định. Dispatcher materialize FAIL_HANDOFF/COLLECT recovery cho item lỗi trước khi source khác thay đổi, sau đó state machine dependency/effective-target quyết định `BLOCKED` hay tiếp tục. Report giữ exit code failure của batch nhưng phản ánh đúng `PASS/PREFLIGHT_FAIL/BLOCKED/NOT_EXECUTED`.


### Metadata vs runtime gates / `run_when_no_changes`

`patch.version`, `phase`, `phase_under_test`, `summary` và `regression_scope` chỉ là metadata phục vụ identity/report/search/context; runtime **không** tự suy ra source/version gate từ các field này. Ràng buộc có hiệu lực phải nằm ở `preflight.files`, `manifest.project.key`, dependency hoặc trusted validation profile.

`post_patch.run_when_no_changes` mặc định `false`: nếu payload là idempotent/no-op và không tạo project delta đã phát hiện thì `post_patch.commands` được skip. Chỉ khi manifest đặt `true` thì post commands mới chạy cả trong no-change path. Flag này không đổi success/failure/rollback semantics.

## v6.17.10 — Contract consistency audit

- Cross-run predecessor enforcement là **relation-based trên từng PATCH được chọn**, không còn gắn cứng vào item đầu batch. PATCH độc lập đứng trước không bị ép xử lý failure cũ; successor liên quan ở vị trí sau không thể lách `batch.previous_failure`.
- Nếu một batch mới đồng thời liên quan tới **nhiều unresolved predecessor**, schema `previous_failure` hiện chỉ biểu diễn một predecessor nên planner fail-closed với `multiple_previous_failures_action_required`; xử lý chúng qua Smart Resume trước.
- `batch.on_dependency_failure=run_anyway` là legacy-compatible input nhưng bị runtime ignore; dependency hoặc related-target failure luôn BLOCKED.
- `plan --failure-policy/--transaction-policy` dùng đúng effective policy; recipe export ghi đúng policy đó và plan kiểm transaction compatibility giống execution.
- `.python_patch_tool.json` được đọc qua cùng bounded/non-symlink/duplicate-key parser cho identity, validation, selector và batch policy; file malformed/unsafe làm `run`/`resume`/`plan` fail-closed rc=2 thay vì fallback policy.


Recipe policy override rule: `run --recipe` uses the policies stored in the recipe; `--failure-policy`/`--transaction-policy` must not be combined with `--recipe`. Create a new recipe with `plan` overrides when different policies are intended.

## v6.17.12 — Zero-argument HISTORY + live PATCH status

- Khi chạy public launcher **không tham số** ở terminal tương tác, selector luôn có thêm dòng `HISTORY`; dùng `↑/↓` rồi `Enter` để mở lịch sử mà không làm mất lựa chọn PATCH hiện tại. Smart Resume cũng có mục HISTORY tương tự.
- Nếu discovery ban đầu không có PATCH/COLLECT runnable, zero-argument mode chỉ in warning hiện có, `AUTO STATUS: IDLE` và Tool Health rồi return `0` **trước khi chạm LAST_RUN/history/registry/run artifacts**. Invocation rỗng không phải một run và không được tạo log/state chạy mới.
- History dùng trực tiếp `artifacts/patch_tool/history/*.json` và report browser hiện có. Con trỏ mặc định ưu tiên **lần PASS gần nhất có PATCH/COLLECT thực sự**, không ưu tiên một lần IDLE rỗng vừa tạo. `Enter` mở report và **in sẵn mục `Important files` với đường dẫn tuyệt đối** cho COLLECT result/request ZIP, FAIL_HANDOFF, recovery COLLECT, replay/archived package và detail/preflight log quan trọng; artifact đã bị cleanup vẫn giữ path lịch sử và được đánh dấu `[missing]`. Detail/aggregate log, source diff và support ZIP vẫn dùng cùng report browser.
- Khi chạy PATCH trên TTY phù hợp, tool dùng **best-effort fixed live status header**: mỗi PATCH hiển thị `WAITING`, `RUNNING`, `PASS`, `FAILED`, `PREFLIGHT FAILED`, `BLOCKED`, `NOT EXECUTED` hoặc `SKIPPED`; log child cuộn ở vùng dưới. Batch lớn dùng sliding status window để không chiếm hết màn hình.
- Live header chỉ là presentation. Redirect/non-TTY, `TERM=dumb`, terminal quá nhỏ, Windows console không bật được VT, hoặc resize/lỗi render sẽ tự fallback về console truyền thống. Có thể tắt chủ động bằng `PTV_DISABLE_LIVE_STATUS=1`. Raw detail logs trên disk **không bị sanitize**; chỉ live display loại escape sequence có thể xóa/di chuyển cursor để bảo vệ header.



## v6.17.13 — History/IDLE/Smart Resume semantics

- User-facing HISTORY chỉ liệt kê run có PATCH/COLLECT thực sự. Các `IDLE` cũ vẫn còn trên disk cho tới cleanup nhưng bị ẩn; từ v6.17.13 run IDLE chỉ cập nhật `LAST_RUN.json` và không tạo thêm `history/*.json`.
- HISTORY row ưu tiên thông tin có giá trị vận hành: **tên package trước, thời gian sau, trạng thái cuối**. Run-id vẫn tồn tại trong JSON/CLI management nhưng không chiếm dòng browser chính.
- Nếu discovery ban đầu có package runnable nhưng toàn bộ bị session/local duplicate filtering loại bỏ, tool in `QUEUE CLEANUP SUMMARY` và chờ Enter trước khi mở HISTORY. Việc này bảo đảm người dùng đọc được package nào đã bị tự loại/chuyển `patchs/ignore`. Trường hợp ngay từ discovery đã không có runnable package thì không tự mở HISTORY và không ghi run/log.
- SMART RESUME tự động chỉ dựa trên **LAST_RUN thực sự FAIL và còn recovery item của run đó trong queue**. Không fallback sang history cũ và không dùng registry cũ để bật global startup prompt. Registry vẫn là planner safety state cho successor liên quan.
- `UNRESOLVED_FAILURES.json` vẫn là safety state: planner tiếp tục enforce failure cũ khi một successor có dependency/effective-target relation, và exact rollback replay identity của unresolved run vẫn bypass duplicate-history suppression. Chỉ bỏ global startup prompt sai ngữ nghĩa, không bỏ predecessor safety.
- Report menu hiển thị `1..N=detail` thay vì `N=detail`; run không có item chỉ hiện aggregate/history/quit và trả lời rõ nếu người dùng thử action item-level.

- History cleanup loại IDLE unpinned cũ trước khi áp RUN_HISTORY_LIMIT=30 cho meaningful runs; pinned run vẫn được giữ.

- History row chuyển `started_at` UTC sang timezone local chỉ ở presentation; persisted report timestamp không thay đổi.


## 18. v6.18.0 — Search discovery / false-zero hardening

### Nguyên nhân gốc được sửa

COLLECT `search` trước v6.18.0 dùng filesystem traversal nhưng tái sử dụng `limits.max_files` của collection package (mặc định 5.000). Scanner dừng traversal khi chạm ngưỡng mà report chỉ ghi `Matches: 0`, không ghi coverage/truncation. Với source tree lớn, symbol nằm sau 5.000 file đầu tiên có thể bị báo false-zero dù file tồn tại. Ngoài ra các directory/symlink/read-error bị skip không được surface.

### Contract mới

- `source_scope=filesystem` là mặc định; file untracked và gitignored vẫn được nhìn thấy khi `respect_gitignore=false` (mặc định). `git_tracked` chỉ là chế độ opt-in rõ ràng.
- `limits.max_search_files` (mặc định 250.000, hard ceiling 1.000.000) và `max_search_file_bytes` tách khỏi `max_files`/`max_file_bytes` dùng cho file được đóng gói. Search không còn bị âm thầm cắt ở 5.000 file.
- `backend=auto`: ưu tiên `rg` nếu có; fallback filesystem Python dùng traversal độc lập. Nếu `rg` không có/lỗi trong auto mode, Python stack scanner + Python `os.walk` vẫn cung cấp hai traversal độc lập.
- Khi primary/fallback bất đồng zero/non-zero hoặc count khác nhau khi không bị truncate, report ghi `SEARCH_INCONSISTENCY`, `primary_matches`, `fallback_matches` và collection thành `INCOMPLETE`.
- `must_find=true` biến zero result thành `INCOMPLETE`; result ZIP vẫn được xuất/đánh dấu PRIMARY để AI đọc diagnostics, nhưng collector trả rc=3 và progress UI ghi `COLLECT INCOMPLETE`, không PASS.
- `diagnose_on_zero=true` mặc định: xác minh root, candidate module/directories, filename evidence, symlink/gitignore policy, search limits và diễn giải zero là VERIFIED hay UNTRUSTED.
- `anchor_paths` được ưu tiên trước requested scopes; `expected_files` được kiểm tra trực tiếp và đưa vào search scope. Search path có thể là relative hoặc absolute nhưng absolute path bắt buộc nằm trong project root.
- Coverage report ghi requested/resolved scopes, directories visited, files considered/searched, extension counts, module inventory, skipped dirs/files, primary/fallback và `Coverage status`.
- Nguyên tắc contract: **“Zero matches is a search result, not proof of absence.”** Zero chỉ có giá trị bằng chứng vắng mặt trong declared searchable scope khi `Coverage status: VERIFIED`.

### Search health

`./tools/run_python_patches.sh health-search` tạo fixture tạm và kiểm literal/regex/find, nested tree, untracked/gitignored, Unicode, symlink safety, >5.000 files, relative/absolute in-project paths, `must_find`, anchors và expected files. Fixture không sửa source project.


## v6.18.3 — Historical COLLECT capability restoration

Mục tiêu release này là hoàn tất vòng bảo toàn tính năng sau v6.18.2 mà không rollback workflow v6.

- Giữ public workflow: `CODE_COLLECTION_REQUEST_*.zip` trong `patchs/` + `./tools/run_python_patches.sh` không tham số. Direct CLI `collect <command>` cũ vẫn SUPERSEDED.
- Khôi phục action request: `ls`, `tree`, `research`, `file/range`, `head/tail`, `symbol`, `references`, `callgraph`, `dependencies`, `directory`, `decompile/ida/ghidra`.
- Khôi phục alias từng dùng thực tế: `search_files`, `content`, `symbol_graph`.
- Search alias bắt buộc dùng chung filesystem-first + fallback + coverage verification của v6.18.0.
- `pack` giữ exact-file semantics của v6.11+; subtree dùng `directory`.
- Decompile dùng temporary SQLite index, bounded/read-only, không ghi vào source tree.
- Thêm semantic release gate `self_test_collect_historical_actions_v6_18_3.py`.

**Stop condition:** chỉ dừng preservation audit khi mọi capability lịch sử đã biết có một disposition rõ ràng (`PRESERVED`, `COMPATIBILITY_RESTORED`, `SUPERSEDED`, `REMOVED_BY_REQUIREMENT`, hoặc fail-closed có giải thích), và không còn regression vô tình có bằng chứng.

## v6.18.1 — Upgrade continuity + restored empty-queue HISTORY

- Khôi phục workflow đã được yêu cầu ở v6.17.12: chạy public launcher không tham số trong TTY, nếu discovery không có PATCH/COLLECT runnable thì sau warning, `AUTO STATUS: IDLE` và Tool Health sẽ mở HISTORY hiện có.
- Giữ nguyên hardening đúng của v6.17.14: invocation rỗng **không** tạo run mới, không ghi `LAST_RUN.json`, không thêm `history/*.json`, không tạo run log và không cập nhật unresolved/ledger. HISTORY chỉ là read-only navigation tới state đã tồn tại.
- Không đổi Smart Resume gating của v6.17.14: failure cũ không hijack queue mới độc lập; unresolved predecessor vẫn được planner enforce cho successor liên quan.
- Thêm semantic upgrade-continuity gate (hiện được carry-forward theo version release) để khóa các capability đã công bố: zero-argument queue/HISTORY, Smart Resume/recovery, duplicate handling, report/support bundle, batch plan/lock, PATCH schema fields, COLLECT legacy actions, launcher Linux/Windows và toàn bộ search additions v6.18.0.
- Audit v6.17.14 -> v6.18.0 xác nhận runtime function/class surface không bị xóa; PATCH schema không mất field; COLLECT schema chỉ mở rộng search. Regression HISTORY là thay đổi semantics từ v6.17.14, không phải code search v6.18.0 xóa chức năng.
