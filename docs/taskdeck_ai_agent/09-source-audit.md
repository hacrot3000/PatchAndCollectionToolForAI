# Source audit — “New deck AI document” (2026-11-19)

**Baseline revision:** `b2f2234a` (“New deck AI document”). **Current implementation reviewed:** merged `main` HEAD `8d75b299` (includes implementation commits, the `origin/main` merge, and Agent Chat diagnostics capture). This audit now covers the current Agent Chat Go/JS, Chat Completions adapter, F08.1 validator/API/UI, workflow and the F01–F08/10–11 docs. Validation below is source + automated unit/fixture tests; it is **not** a live provider, real Python Patch or interactive browser acceptance.

## Kết luận

**F01–F08 và phân cấp feature/subtask nhìn chung đúng, có thể dùng làm kế hoạch triển khai. Chưa được xem là release-ready hoặc autonomous end-to-end.** Các trạng thái `[x]` ở handoff chỉ phản ánh phần code/test được nêu, không mặc định chứng minh thực thi Python hoặc provider thật. Ưu tiên khắc phục được liệt kê dưới đây theo code hiện có.

| Priority | Findings grounded in actual source | Blocker / completion evidence |
| --- | --- | --- |
| **P0 — CI coverage** | Workflow ban đầu dùng `go test ./internal/server -run '^TestAgentChat'`: **15/25** test Agent Server match; **10** test `TestAgentTool*`, `TestAgentNative*`, `TestAgentApproved*` không chạy; `pull_request.paths` cũng thiếu Agent server paths. | **Đã sửa** tại `7d6c6bed`: `^TestAgent` và PR paths đúng; [CI 44479964926](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/44479964926) **PASS**. Vẫn chỉ là Go/mock CI, không phải live runtime. |
| **P0 — Tool capability** | Đã có provider/model fingerprinted probes cho text, tool schema, previous-response, Conversations và SSE; `taskdeck_submit_patch` chỉ bật khi capability tool-calling là verified. Chat Completions stateful modes/unsupported streaming được báo unsupported. | Automated probes/mock tests PASS; **provider live capability remains NOT VERIFIED**. Probe tool schema proves acceptance, not reliable generation quality. |
| **P0 — Internal handler boundary** | `agentQueueViaNativePaste`, `agentStartNativePatch`, `agentSelectNativePatch` dựng `httptest.NewRecorder` để gọi `patchPaste`, `sessionsRoot`, `sessionItem`. Router middleware **không tự chạy lại**. | Audit auth, owner, CSRF/same-origin, `PrincipalFromContext`, shared `patch.run`, project identity, mutation lease/audit **tại mỗi side effect**; ưu tiên typed service interface thay vì giả lập HTTP. |
| **P0 — Native result correlation** | Final reconciliation now requires linked native session, item name + approved index + non-empty expected `run_id`, and re-verifies the persisted queue ZIP's SHA256/size receipt before adopting PASS/FAIL. Missing session or digest mismatch becomes UNKNOWN. | Fake Session Service tests PASS; **real native Patch run and final runtime receipt still NOT VERIFIED**. Native session owner is not yet available as a protocol evidence field. |
| **P0 — Write drift / multi-process** | Snapshot HEAD/branch/porcelain status at proposal + recheck before queue/selection; 10-minute approval TTL and argument digest binding; canonical Agent workspace lease; journal uses `flock` on supported Unix. | Still cannot exclude native/manual Patch writers that do not share the Agent lease; non-Unix and NFS/SMB multi-daemon semantics are not guaranteed. Keep concurrent external-writer/live race **NOT VERIFIED**. |
| **P1 — Feedback semantics** | Bounded 30-minute local opt-in auto-feedback now reconciles linked native sessions without GUI polling, persists final evidence and sends once through the shared continuation path. `FeedbackStatus` distinguishes prepared/sending/acknowledged/unknown; ambiguous delivery is never retried. | Background scheduler tests PASS with fake native service. External provider acknowledgement remains unknowable after transport failure; UNKNOWN is manual-only. |
| **P1 — UI/turn lifecycle** | Provider turn is detached from HTTP lifetime and bound to daemon context; explicit cancel and daemon shutdown cancel it. Per-session drafts/attachments, generation guards, durable event cursor, text-only attachments, body/idempotency bounds, and admission queue are covered by tests. | Automated JS bootstrap/static + Go tests PASS; interactive browser GUI still **NOT VERIFIED**. Native cancellation remains separate and does not claim the patch process stopped. |
| **P1 — Provider + data exposure** | Provider delete-local-key, capability probes and first-turn AI Pack preview are present; scrub remains explicitly best-effort. Added Chat Completions stateless adapter + SSE parser. | Unit/fixture tests PASS; no live provider, pricing/token accounting, retention policy, or independent security review. Preview must not be called guaranteed redaction. |
| **P1 — Request lifecycle/attachments** | Fixed: provider turns use daemon context, body budget covers maximum escaped message+attachments, raw UTF-8 is checked, and idempotency digest includes attachment name/MIME/content. | Tests cover disconnect→explicit cancel, max escaped body, oversize/binary/invalid UTF-8 and same/different attachment retries. Browser-refresh interaction remains not browser-verified. |
| **P1 — Multi-tab** | Fixed: draft and attachments are keyed per session; outgoing draft is saved before await; generation token discards stale GET response; inactive-tab response cannot overwrite status/draft. | JS bootstrap + static contract tests PASS; browser automation/interactive acceptance remains **NOT VERIFIED**. |
| **P1 — Backpressure** | Fixed: one waiter position for full duration, context cancellation, bounded owner/global queue and notification channel (no sleep polling). | Race tests cover 8 waiters, 9th reject, cancellation, release and bounded refusal. |
| **P1 — Alternate provider / planning** | Added Chat Completions JSON + SSE adapter, Responses regression coverage, strict Task Manifest Markdown/canonical JSON validation, preview/DAG/diagnostics and canonical JSON download; no manifest persistence/execution. | Go unit/fixture tests PASS. Real compatible endpoint and interactive planning GUI remain not live-verified. |
| **P1 — Provider diagnostics** | Commit `8d75b299` adds bounded diagnostic event capture/trace IDs and structured provider errors, with safe/raw viewing controls. | Diagnostic tests are included in Agent suite; raw mode permission/UI acceptance and payload-retention review remain to verify before production. |
| **P2 — Full server package** | `go test ./internal/server` on merged baseline `bba5eea6` has 22 failures across auth/editor/English UI/foreground/grouped menus/Patch-panel/terminal/restart/restricted-role tests. The same 22 named tests fail on a clean detached worktree at `bba5eea6`. | These are proven baseline failures relative to the reviewed base, not introduced by the current Agent changes; targeted `^TestAgent` suite passes. |
| **P2 — Handoff drift** | Older M1–M3 narrative and F02/F08 labels were stale after subsequent implementation. | Current implementation continuation below distinguishes source/mock/CI from live/blockers; remote Patch and F08.2 remain deferred. |

## Protocol chain và trạng thái được phép suy luận

| Stage | Evidence currently available | What is NOT proven |
| --- | --- | --- |
| `function_call` | Responses `call_id`, arguments JSON, model/session ID | Không chứng minh được user approval |
| `pending` / approve | Revision + owner và native patch validation, per-call user action | Chưa chạy Python |
| `queuing` | Durable intent marker trước side effect | Crash giữa marker và receipt = UNKNOWN |
| `queued` | Native `patchPaste` receipt, SHA256/ZIP | Chưa preflight hay execute |
| `dispatching` | Native session ID đã tạo | Prompt chưa chắc đã được chấp nhận |
| `running` | `prompt_response` được chấp nhận cho đúng index | Không phải PASS |
| `done` / `error` | Final `ProtocolState.Items` matching native session, approved item name/index, expected run ID; current queued ZIP reverified against persisted SHA256 + size | Test uses fake Session Service; no real Python execution evidence or native owner identity field |
| `feedback_sent` / `FeedbackStatus` | Local tool output persisted; `prepared`→`sending`→`acknowledged` or `unknown` | `acknowledged` means provider call returned a valid response; transport failure is ambiguous, never auto-resend |

**Source map:** `internal/agentchat/responses.go` → `internal/server/agent_chat_response_helpers.go` → `agent_chat_tools.go` → `patch_paste.go` → `agent_chat_native.go` → `agent_chat_sync.go` → `agent_chat_tool_feedback.go`. Code và tài liệu dùng đường dẫn tương đối gốc `vscode_tasks_menu_go/`. Fake-native integration test là `TestAgentApprovedPatchRunsExactlyOneNativeQueueEntry`, chỉ gọi handler với fake Session Service, **không chạy real Python patch**.

## Release gate / bằng chứng chấp nhận

- [x] Đọc 9 file F01–F08 và xác minh liên kết Markdown tương đối.
- [x] Đối chiếu chức năng với source, test names và checklist chính.
- [x] Sửa workflow selector/PR paths, xác minh CI mới chạy PASS (Go/mock).
- [ ] Kiểm thử native benign Python PATCH trong workspace được phép và cô lập, có SHA/run ID/RC thực tế.
- [ ] Kiểm thử authentication/middleware/owner/shared lease tại **nội bộ** dispatch/approval.
- [x] Partial: HEAD/dirty snapshot + verify, approval TTL, Agent workspace lease, flock journal, run-ID/index result binding, startup reconcile.
- [ ] Còn thiếu: lease/arbitration dùng chung với native/manual Patch writers; fault-injection đa process/restart và platform filesystem matrix. Final SHA/size is now checked at reconciliation.
- [x] Probe adapter/provider capability tests (mock/no project context) for Responses + Chat Completions; stateful is only advertised where the selected API type supports and verifies it.
- [ ] Live provider capability probe with explicit consent and benign external endpoint.
- [ ] Kiểm thử UI browser, console/artifact, cancel/reconnect và permissions.
- [ ] E2EE host opt-in / typed Remote Patch action verified + security review.

**Scope/current state:** F05.3 local opt-in automatic delivery after background native reconciliation is implemented and fake-tested; real native/provider run remains NOT VERIFIED. F08.1 is planning-only Markdown/canonical JSON validation + preview/download, no persistence or execution. F08.2 DAG execution/worktree/merge remains deferred. No source or CI test implies `RELEASE READY`.

**Lệnh kiểm tra (từ `vscode_tasks_menu_go/`):**

```bash
go test -race -count=1 ./internal/agentchat
go test -race -count=1 ./internal/taskmanifest
go test -race -count=1 ./internal/server -run '^TestAgent'
go test -count=1 ./internal/patchtool
go vet ./internal/agentchat ./internal/server ./internal/taskmanifest
node --check web/featuremods/agentchat.js
node web/agentchat_bootstrap_test.cjs
node test_ai_smart_terminal.cjs
```
