# Danh sách tính năng Python Patch Tool — v6.14.0

> Quy tắc duy trì: cập nhật file này ở **mỗi release** khi trạng thái capability thay đổi. Chỉ ghi COMPLETE khi có acceptance/regression tương ứng.

## Workflow / queue / selector

| Tính năng | Trạng thái |
|---|---|
| Entry point duy nhất `./tools/run_python_patches.sh` | COMPLETE |
| Tự phân loại PATCH / COLLECT / non-runnable evidence | COMPLETE |
| COLLECT request ZIP-only; raw JSON reject | COMPLETE |
| PATCH priority 0–9, stable order | COMPLETE |
| TTY filename width/height/Unicode viewport safety | COMPLETE |
| `i` inspect/dry-run PATCH không execution | COMPLETE |
| `h` Tool Health/self-audit read-only; line mode hỗ trợ `h`; IDLE in compact health | **COMPLETE v6.14.0** |
| Exactly one COLLECT / không trộn PATCH | COMPLETE |
| Không process/project lock | COMPLETE |

## Duplicate / execution safety

| Tính năng | Trạng thái |
|---|---|
| Duplicate current-session exact size+SHA-256; giữ canonical và remove redundant queue copy | COMPLETE |
| Duplicate local-history SHA-256, current project only | COMPLETE |
| PATCH in-place; SANDBOX/worktree removed | COMPLETE |
| Fail-fast selected batch | COMPLETE |
| Exact machine-readable `PATCH_PACKAGE_SCHEMA.json` | COMPLETE |
| PATCH schema/resource/source/post-command preflight | COMPLETE |
| Tool version min/max/max-tested negotiation | COMPLETE |
| Partial-modification detection khi PATCH fail | COMPLETE |
| Metadata-driven rollback opt-in cho exact declared targets | **COMPLETE v6.14.0** |
| Rollback payload/post-patch failure bằng bounded exact-byte snapshot | **COMPLETE v6.14.0** |
| Rollback phát hiện thay đổi ngoài declared scope và không false-PASS | **COMPLETE v6.14.0** |
| Generic rollback khi thiếu metadata / Git-policy rollback | **KHÔNG TỰ LÀM / FAIL-CLOSED** |

## FAIL → AI recovery / audit

| Tính năng | Trạng thái |
|---|---|
| Structured failure diagnosis | COMPLETE |
| `[PRIMARY] PATCH FAIL HANDOFF` ZIP | COMPLETE |
| Include bounded safe relevant current source trong handoff | COMPLETE |
| Source-drift/anchor → auto-prepare `pack` COLLECT request cho next run | COMPLETE |
| `artifacts/patch_tool/LAST_RUN.json` | COMPLETE |
| Bounded local run history (30) | COMPLETE |
| Resume hint cho selected item chưa chạy | COMPLETE |
| Rollback result (`PASS/PARTIAL/FAIL/SKIPPED`) nằm trong structured PATCH result/handoff | **COMPLETE v6.14.0** |

## COLLECT

| Tính năng | Trạng thái |
|---|---|
| Exact `COLLECT_ACTION_SCHEMA.json` | COMPLETE |
| Không tự đoán action ngoài schema | COMPLETE |
| `pack`, `overview`, `find`, `search`, `git` readonly | COMPLETE |
| Verified result ZIP / request archive lifecycle | COMPLETE |
| Highlight một `[PRIMARY - UPLOAD THIS FILE]` | COMPLETE |
| COLLECT quality summary `files/source/reports/zip/truncated/missing` | COMPLETE |
| Truncation marker trong report manifest | COMPLETE |

## Packaging / docs / health

| Tính năng | Trạng thái |
|---|---|
| Full self-contained runtime cho documented contract | COMPLETE |
| `SHA256SUMS`, no pycache, launcher 0755 | COMPLETE |
| Tool Health kiểm tra VERSION / checksum / runtime / launcher / schemas | **COMPLETE v6.14.0** |
| `implementing.md` live tracker | COMPLETE |
| AI docs: PATCH schema + COLLECT schema + recovery/rollback contract | **COMPLETE v6.14.0** |
| HTML user guide tối giản | COMPLETE; v6.14.0 chỉ thêm một dòng về phím `h` vì đây là thao tác trực tiếp của người dùng |

## Bị giới hạn có chủ đích

- Historical/private-core format ngoài documented current contract: **FAIL-CLOSED / NOT GUARANTEED**.
- Auto rollback chỉ được phép khi manifest có exact recovery metadata. Thiếu metadata → preflight reject; tool không đoán.
- Rollback chỉ dành cho `payload_failure` / `post_patch_failure`, trước Git policy. Git commit/push failure không được tự rollback bằng cách giả lập transaction.
- Trên Git project, fingerprint toàn worktree được dùng để phát hiện rollback chưa hoàn chỉnh do thay đổi ngoài scope. Ngoài Git, verification chỉ cam kết các declared rollback targets.
- Advanced phase inference chỉ tinh chỉnh nếu runtime evidence mới cho thấy thiếu.

## v6.14.0 stop condition

Hai phần còn thiếu theo danh sách đã phê duyệt (#1 rollback an toàn có metadata và #12 Tool Health) đã hoàn tất. **Dừng và hỏi người dùng hướng tiếp theo.**
