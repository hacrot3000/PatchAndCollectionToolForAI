# Danh sách tính năng Python Patch Tool — v6.13.0

> Quy tắc duy trì: cập nhật file này ở **mỗi release** khi trạng thái capability thay đổi. Chỉ ghi COMPLETE khi có acceptance/regression tương ứng.

## Workflow / queue / selector

| Tính năng | Trạng thái |
|---|---|
| Entry point duy nhất `./tools/run_python_patches.sh` | COMPLETE |
| Tự phân loại PATCH / COLLECT / non-runnable evidence | COMPLETE |
| COLLECT request ZIP-only; raw JSON reject | COMPLETE |
| PATCH priority 0–9, stable order | COMPLETE |
| TTY filename width/height/Unicode viewport safety | COMPLETE |
| `i` inspect/dry-run PATCH không execution | **COMPLETE v6.13.0** |
| Exactly one COLLECT / không trộn PATCH | COMPLETE |
| Không process/project lock | COMPLETE |

## Duplicate / execution safety

| Tính năng | Trạng thái |
|---|---|
| Duplicate current-session exact size+SHA-256; giữ canonical và remove redundant queue copy | COMPLETE |
| Duplicate local-history SHA-256, current project only | COMPLETE |
| PATCH in-place; SANDBOX/worktree removed | COMPLETE |
| Fail-fast selected batch | COMPLETE |
| Exact machine-readable `PATCH_PACKAGE_SCHEMA.json` | **COMPLETE v6.13.0** |
| PATCH schema preflight trước payload | **COMPLETE v6.13.0** |
| Required package resources preflight | **COMPLETE v6.13.0** |
| Declared source SHA-256 / anchor preflight | **COMPLETE v6.13.0** |
| Post command cwd/executable preflight | **COMPLETE v6.13.0** |
| Tool version min/max/max-tested negotiation | **COMPLETE v6.13.0** |
| Partial-modification detection khi PATCH fail | **COMPLETE v6.13.0** |
| Automatic rollback không đủ metadata | **KHÔNG TỰ LÀM / FAIL-CLOSED** |

## FAIL → AI recovery / audit

| Tính năng | Trạng thái |
|---|---|
| Structured failure diagnosis | **COMPLETE v6.13.0** |
| `[PRIMARY] PATCH FAIL HANDOFF` ZIP | **COMPLETE v6.13.0** |
| Include bounded safe relevant current source trong handoff | **COMPLETE v6.13.0** |
| Source-drift/anchor → auto-prepare `pack` COLLECT request cho next run | **COMPLETE v6.13.0** |
| `artifacts/patch_tool/LAST_RUN.json` | **COMPLETE v6.13.0** |
| Bounded local run history (30) | **COMPLETE v6.13.0** |
| Resume hint cho selected item chưa chạy | **COMPLETE v6.13.0** |

## COLLECT

| Tính năng | Trạng thái |
|---|---|
| Exact `COLLECT_ACTION_SCHEMA.json` | COMPLETE |
| Không tự đoán action ngoài schema | COMPLETE |
| `pack`, `overview`, `find`, `search`, `git` readonly | COMPLETE |
| Verified result ZIP / request archive lifecycle | COMPLETE |
| Highlight một `[PRIMARY - UPLOAD THIS FILE]` | COMPLETE |
| COLLECT quality summary `files/source/reports/zip/truncated/missing` | **COMPLETE v6.13.0** |
| Truncation marker trong report manifest | **COMPLETE v6.13.0** |

## Packaging / docs

| Tính năng | Trạng thái |
|---|---|
| Full self-contained runtime cho documented contract | COMPLETE |
| `SHA256SUMS`, no pycache, launcher 0755 | COMPLETE |
| `implementing.md` live tracker | COMPLETE |
| AI docs: PATCH schema + COLLECT schema + recovery contract | **COMPLETE v6.13.0** |
| HTML user guide tối giản | COMPLETE; chỉ cập nhật khi workflow người dùng thật sự thay đổi |

## Bị giới hạn có chủ đích

- Historical/private-core format ngoài documented current contract: **FAIL-CLOSED / NOT GUARANTEED**.
- Automatic rollback: không thực hiện nếu package không cung cấp một recovery contract đủ an toàn; v6.13.0 chỉ **detect + diagnose + handoff**, không đoán rollback.
- Advanced phase inference và private historical LAST_RUN parity cũ không còn là blocker của structured current `LAST_RUN.json`; chỉ tinh chỉnh nếu runtime evidence mới cho thấy thiếu.

## v6.13.0 stop condition

Phase A, B, C đã hoàn tất. **Dừng và hỏi người dùng hướng tiếp theo.**
