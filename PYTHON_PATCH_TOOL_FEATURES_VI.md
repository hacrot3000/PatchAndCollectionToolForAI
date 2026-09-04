# Danh sách tính năng Python Patch Tool — v6.14.2

> Quy tắc duy trì: cập nhật file này ở **mỗi release** khi trạng thái capability thay đổi. Chỉ ghi COMPLETE khi có acceptance/regression tương ứng.

## Workflow / queue / selector

| Tính năng | Trạng thái |
|---|---|
| Linux/POSIX entry point `./tools/run_python_patches.sh` | COMPLETE |
| Windows CMD entry point `tools\run_python_patches.bat` | **COMPLETE v6.14.2** |
| Windows PowerShell entry point `.\tools\run_python_patches.ps1` | **COMPLETE v6.14.2** |
| Cùng zero-argument dispatcher/project root/queue trên Linux và Windows | **COMPLETE v6.14.2** |
| Windows tự tìm Python 3.10+ qua `py -3` / `python` / `python3` | **COMPLETE v6.14.2** |
| Tự phân loại PATCH / COLLECT / non-runnable evidence | COMPLETE |
| Queue root `patchs/` phải là directory thật trong project; symlink/unsafe path fail-closed | COMPLETE v6.14.1 |
| COLLECT request ZIP-only; raw JSON reject | COMPLETE |
| PATCH priority 0–9, stable order | COMPLETE trên fullscreen POSIX selector |
| Windows/non-TTY line selector: index/range/all/delete/inspect/health/quit | **COMPLETE v6.14.2** |
| TTY filename width/height/Unicode viewport safety | COMPLETE |
| `i` inspect/dry-run PATCH không execution | COMPLETE |
| `h` Tool Health/self-audit read-only | COMPLETE |
| Exactly one COLLECT / không trộn PATCH | COMPLETE |
| Không process/project lock | COMPLETE / BY REQUIREMENT |

## Duplicate / execution / process safety

| Tính năng | Trạng thái |
|---|---|
| Duplicate current-session exact size+SHA-256; giữ canonical và remove redundant queue copy | COMPLETE |
| Duplicate current-session safe-removal chống hash→unlink race khi nhiều terminal | COMPLETE v6.14.1 |
| Duplicate local-history SHA-256, current project only | COMPLETE |
| PATCH in-place; SANDBOX/worktree removed | COMPLETE |
| Fail-fast selected batch | COMPLETE |
| PATCH runner child process group + SIGINT/SIGTERM forwarding | COMPLETE v6.14.1 |
| SIGINT/SIGTERM controlled cleanup + rollback + rc130/143 | COMPLETE v6.14.1 |
| Payload/post-command timeout descendant containment | COMPLETE v6.14.1 trên POSIX; Windows dùng nhánh process-control riêng của Python |
| Exact PATCH input snapshot: execute/archive đúng bytes đã chọn | COMPLETE v6.14.1 |
| Same-name PATCH replacement trong lúc chạy được giữ lại trong queue | COMPLETE v6.14.1 |
| Exact COLLECT request snapshot/archive; replacement khác bytes được giữ lại | COMPLETE v6.14.1 |

## PATCH schema / rollback / recovery

| Tính năng | Trạng thái |
|---|---|
| Exact machine-readable `PATCH_PACKAGE_SCHEMA.json` | COMPLETE |
| PATCH schema/resource/source/post-command preflight | COMPLETE |
| Tool version min/max/max-tested negotiation | COMPLETE |
| Partial-modification detection khi PATCH fail | COMPLETE |
| Metadata-driven rollback opt-in cho exact declared targets | COMPLETE |
| Rollback reject symlink/non-directory ancestor | COMPLETE v6.14.1 |
| Rollback `exists:false` yêu cầu parent directory đã tồn tại an toàn trước payload | COMPLETE v6.14.1 |
| Rollback snapshot re-check exact baseline để chặn TOCTOU | COMPLETE v6.14.1 |
| Rollback restore POSIX dùng pinned dir-FD/`O_NOFOLLOW` | COMPLETE v6.14.1 |
| Generic rollback khi thiếu metadata / Git-policy rollback | **KHÔNG TỰ LÀM / FAIL-CLOSED** |
| Structured failure diagnosis | COMPLETE |
| `[PRIMARY] PATCH FAIL HANDOFF` ZIP | COMPLETE |
| FAIL_HANDOFF chỉ attach PATCH khi SHA khớp exact executed package | COMPLETE v6.14.1 |
| Source-drift/anchor → auto-prepare `pack` COLLECT request cho next run | COMPLETE |
| `artifacts/patch_tool/LAST_RUN.json` + bounded history + resume hint | COMPLETE |

## COLLECT

| Tính năng | Trạng thái |
|---|---|
| Exact `COLLECT_ACTION_SCHEMA.json` | COMPLETE |
| Không tự đoán action ngoài schema | COMPLETE |
| `pack`, `overview`, `find`, `search`, `git` readonly | COMPLETE |
| Exact request snapshot trước execution/archive | COMPLETE v6.14.1 |
| Verified result ZIP / request archive lifecycle | COMPLETE |
| Highlight một `[PRIMARY - UPLOAD THIS FILE]` | COMPLETE |
| COLLECT quality summary `files/source/reports/zip/truncated/missing` | COMPLETE |

## Packaging / docs / health

| Tính năng | Trạng thái |
|---|---|
| Full self-contained runtime cho documented contract | COMPLETE |
| `SHA256SUMS`, no pycache, POSIX launcher 0755 | COMPLETE |
| Windows `.bat` + `.ps1` nằm trong exact checksum coverage | **COMPLETE v6.14.2** |
| Tool Health kiểm tra VERSION / checksum / runtime / launcher / schemas | COMPLETE |
| Tool Health bắt buộc checksum coverage của required runtime | COMPLETE v6.14.1 |
| Tool Health reject required/checksummed path có symlink ancestor | COMPLETE v6.14.1 |
| Launcher không tự sinh `__pycache__`/`.pyc` | COMPLETE v6.14.1; áp dụng cả PowerShell launcher v6.14.2 |
| `implementing.md` live tracker | COMPLETE |
| AI docs: PATCH/COLLECT + portable Windows workflow | **COMPLETE v6.14.2** |
| HTML user guide tối giản | **UPDATED v6.14.2 chỉ để thêm cài/chạy Windows** |

## Bị giới hạn có chủ đích

- Historical/private-core format ngoài documented current contract: **FAIL-CLOSED / NOT GUARANTEED**.
- Không process/project lock: concurrency vẫn là lựa chọn của người dùng.
- Auto rollback chỉ được phép khi manifest có exact recovery metadata. Thiếu metadata → tool không đoán.
- Git commit/push failure không kích hoạt generic rollback.
- Fullscreen arrow/Space/priority UI hiện là POSIX TTY capability; Windows dùng line selector ổn định thay vì giả lập `termios`.
- External `post_patch.commands[].argv` phải portable theo OS hoặc dùng executable có thật trên máy; launcher không tự chuyển lệnh Bash thành PowerShell/CMD.

## v6.14.2 stop condition

Windows launcher support đã hoàn tất. **Dừng và hỏi người dùng hướng tiếp theo.**
