# Danh sách tính năng Python Patch Tool — v6.14.1

> Quy tắc duy trì: cập nhật file này ở **mỗi release** khi trạng thái capability thay đổi. Chỉ ghi COMPLETE khi có acceptance/regression tương ứng.

## Workflow / queue / selector

| Tính năng | Trạng thái |
|---|---|
| Entry point duy nhất `./tools/run_python_patches.sh` | COMPLETE |
| Tự phân loại PATCH / COLLECT / non-runnable evidence | COMPLETE |
| Queue root `patchs/` phải là directory thật trong project; symlink/unsafe path fail-closed | **COMPLETE v6.14.1** |
| COLLECT request ZIP-only; raw JSON reject | COMPLETE |
| PATCH priority 0–9, stable order | COMPLETE |
| TTY filename width/height/Unicode viewport safety | COMPLETE |
| `i` inspect/dry-run PATCH không execution | COMPLETE |
| `h` Tool Health/self-audit read-only | COMPLETE |
| Exactly one COLLECT / không trộn PATCH | COMPLETE |
| Không process/project lock | COMPLETE / BY REQUIREMENT |

## Duplicate / execution / process safety

| Tính năng | Trạng thái |
|---|---|
| Duplicate current-session exact size+SHA-256; giữ canonical và remove redundant queue copy | COMPLETE |
| Duplicate current-session safe-removal chống hash→unlink race khi nhiều terminal | **COMPLETE v6.14.1** |
| Duplicate local-history SHA-256, current project only | COMPLETE |
| PATCH in-place; SANDBOX/worktree removed | COMPLETE |
| Fail-fast selected batch | COMPLETE |
| PATCH runner child process group + SIGINT/SIGTERM forwarding | **COMPLETE v6.14.1** |
| SIGINT/SIGTERM controlled cleanup + rollback + rc130/143 | **COMPLETE v6.14.1** |
| Payload/post-command timeout terminate toàn descendant process group trước rollback | **COMPLETE v6.14.1** |
| Exact PATCH input snapshot: execute/archive đúng bytes đã chọn | **COMPLETE v6.14.1** |
| Same-name PATCH replacement trong lúc chạy được giữ lại trong queue | **COMPLETE v6.14.1** |
| Exact COLLECT request snapshot/archive; replacement khác bytes được giữ lại | **COMPLETE v6.14.1** |

## PATCH schema / rollback / recovery

| Tính năng | Trạng thái |
|---|---|
| Exact machine-readable `PATCH_PACKAGE_SCHEMA.json` | COMPLETE |
| PATCH schema/resource/source/post-command preflight | COMPLETE |
| Tool version min/max/max-tested negotiation | COMPLETE |
| Partial-modification detection khi PATCH fail | COMPLETE |
| Metadata-driven rollback opt-in cho exact declared targets | COMPLETE |
| Rollback reject symlink/non-directory ancestor | **COMPLETE v6.14.1** |
| Rollback `exists:false` yêu cầu parent directory đã tồn tại an toàn trước payload | **COMPLETE v6.14.1** |
| Rollback snapshot re-check exact baseline để chặn TOCTOU | **COMPLETE v6.14.1** |
| Rollback restore POSIX dùng pinned dir-FD/`O_NOFOLLOW` | **COMPLETE v6.14.1** |
| Generic rollback khi thiếu metadata / Git-policy rollback | **KHÔNG TỰ LÀM / FAIL-CLOSED** |
| Structured failure diagnosis | COMPLETE |
| `[PRIMARY] PATCH FAIL HANDOFF` ZIP | COMPLETE |
| FAIL_HANDOFF chỉ attach PATCH khi SHA khớp exact executed package | **COMPLETE v6.14.1** |
| Source-drift/anchor → auto-prepare `pack` COLLECT request cho next run | COMPLETE |
| `artifacts/patch_tool/LAST_RUN.json` + bounded history + resume hint | COMPLETE |

## COLLECT

| Tính năng | Trạng thái |
|---|---|
| Exact `COLLECT_ACTION_SCHEMA.json` | COMPLETE |
| Không tự đoán action ngoài schema | COMPLETE |
| `pack`, `overview`, `find`, `search`, `git` readonly | COMPLETE |
| Exact request snapshot trước execution/archive | **COMPLETE v6.14.1** |
| Verified result ZIP / request archive lifecycle | COMPLETE |
| Highlight một `[PRIMARY - UPLOAD THIS FILE]` | COMPLETE |
| COLLECT quality summary `files/source/reports/zip/truncated/missing` | COMPLETE |

## Packaging / docs / health

| Tính năng | Trạng thái |
|---|---|
| Full self-contained runtime cho documented contract | COMPLETE |
| `SHA256SUMS`, no pycache, launcher 0755 | COMPLETE |
| Tool Health kiểm tra VERSION / checksum / runtime / launcher / schemas | COMPLETE |
| Tool Health bắt buộc checksum coverage của required runtime | **COMPLETE v6.14.1** |
| Tool Health reject required/checksummed path có symlink ancestor | **COMPLETE v6.14.1** |
| Launcher không tự sinh `__pycache__`/`.pyc`; Tool Health không self-WARN trên install sạch | **COMPLETE v6.14.1** |
| `implementing.md` live tracker | COMPLETE |
| AI docs: PATCH/COLLECT schema + recovery/rollback/runtime robustness contract | **COMPLETE v6.14.1** |
| HTML user guide tối giản | COMPLETE; **không đổi ở v6.14.1 vì workflow người dùng không thay đổi** |

## Bị giới hạn có chủ đích

- Historical/private-core format ngoài documented current contract: **FAIL-CLOSED / NOT GUARANTEED**.
- Không process/project lock: concurrency vẫn là lựa chọn của người dùng; v6.14.1 harden identity/lifecycle thay vì khóa toàn project.
- Auto rollback chỉ được phép khi manifest có exact recovery metadata. Thiếu metadata → tool không đoán.
- Rollback không tự xóa directory ban đầu chưa tồn tại; `exists:false` chỉ hợp lệ khi parent đã tồn tại an toàn.
- Git commit/push failure không kích hoạt generic rollback.
- Advanced phase inference chỉ tinh chỉnh khi có runtime evidence mới.

## v6.14.1 stop condition

Robustness audit của v6.14.0 đã hoàn tất và có regression riêng. **Dừng và hỏi người dùng hướng tiếp theo.**
