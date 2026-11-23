# TASKDECK — KẾ HOẠCH 02: HOT-SWITCH AI AGENT / PROVIDER THEO TỪNG REQUEST

> **Loại:** Implementation plan + feature checklist (3 cấp)  
> **Ngày lập:** 2026-11-20  
> **Cập nhật:** 2026-11-20 — Đã nối selector/provider theo conversation vào Agent Chat composer, gửi run song song qua API PLAN 01, lưu run cards và ghép run history vào snapshot mới.
> **Trạng thái:** P0 path đã được kiểm chứng bằng mock UI + handler integration tests; chưa nghiệm thu bằng browser thật/provider credential và vẫn còn dependency Patch/tool routing.  
> **Phạm vi:** Đang chạy Composer vẫn có thể chọn GPT cho request mới trong cùng conversation; mỗi request cố định provider của chính nó.  
> **Không bao gồm:** Xây dựng quản lý nhiều conversation, shared context và peer communication (xem kế hoạch 01).  
> **Đề xuất vị trí trong repo:** `docs/ai-agent/plans/02_AGENT_PROVIDER_HOT_SWITCH_PLAN.md` (chưa commit vào Git).  

**Tiến độ ban đầu:** 0/126 mục hoàn tất (Cấp 1: 0/10, cấp 2: 0/29, cấp 3: 0/87). Tất cả mục được để **[ ]** vì chưa triển khai/kiểm chứng mã nguồn trong tác vụ lập plan này.

**Tiến độ triển khai hiện tại:** Selector/model đã mount trong composer; preference localStorage theo conversation được phục hồi khi chuyển tab và remount UI. Khi legacy Agent Chat busy hoặc provider/model khác binding session, Send tạo snapshot + RunSpec và submit vào Manager chung; run cards poll trạng thái/output riêng, Stop/Retry dùng run ID. Snapshot pin system instructions, automation/retry/reasoning settings, owner/execution/provider/model/config/credential RunBinding, ParentTurnID và excluded pending IDs; snapshot mới ghép prompt của mọi run, chỉ output Completed và bỏ assistant output partial/pending. Public provider config revisions được archive; credential fingerprint đổi thì retry fail-closed, không dùng key mới. Run executor cố ý text-only vì Patch/tool handoff chưa có. Backend handler integration và UI bootstrap/mock #101/#102 pass; chưa smoke test trình duyệt thật hoặc provider endpoint thật. End-to-end checkboxes vẫn **[ ]**.

**Interface phối hợp hiện thấy trong working tree:** `agentrun.Executor.Execute/Cancel(context.Context, agentchat.RunSpec, []agentchat.HistoryItem, agentchat.EventSink)` tương thích với `agentchat.ProviderRouter`; `agent_run_api.go` dùng `NewAgentRunProviderExecutor(owner)` theo owner và `ResolveAgentRunInputs`/`BuildAgentRunWorkspacePack`. Provider config version và credential fingerprint được pin trong RunSpec; provider store lưu public config revisions để retry cùng endpoint/model, còn resolver đối chiếu fingerprint của credential hiện tại và fail-closed khi key bị đổi/thu hồi. Snapshot kế tiếp ghép prompt của run đã gửi và chỉ assistant output của run Completed, giữ `OriginRunID`/provider/model/parent. Manager vẫn là scheduler duy nhất, global cap 5. Stop hủy request context; không có xác nhận hủy remote. Còn P1: BuildAgentRunWorkspacePack lấy đúng execution workspace nhưng pack chưa được materialize/hash vào snapshot tại Send; MaxInputTokens chỉ có byte-bound bảo thủ, chưa có tokenizer. Run executor cố ý text-only; Native Patch/tool handoff chưa tích hợp. Full server suite 300s chưa hoàn tất; lần chạy mới nhất PASS `TestRunOwnedGlobalSlotIsNotAcquiredTwiceByAgentChat` rồi vẫn đang chạy nhóm Remote Workflow khi timeout. Có nhiều assertion fail trong auth/editor/terminal/Patch ngoài phạm vi HS; focused server tests đang PASS, full suite chưa được xác nhận.

## 1. Hành vi bắt buộc và giới hạn

- Request **#101 → Composer** đang chạy; người dùng đổi dropdown thành **GPT**; request **#101 không đổi provider và không bị stop**.
- Request **#102 → GPT** gửi ngay khi #101 còn active (nếu giới hạn tài nguyên cho phép); #102 lưu snapshot tại thời điểm gửi.
- GPT hoàn thành trước Composer vẫn gắn đúng prompt, parent turn và provider. #102 **không mặc nhiên có output của #101** nếu #101 chưa hoàn tất lúc tạo snapshot.
- Khi retry #101, dùng **Composer**, không dùng provider đang chọn trong UI; nếu muốn thử provider khác phải là thao tác **Retry with another provider** riêng có audit trail.
- Không hứa chuyển internal session/thoughts của Composer sang GPT. Chỉ chuyển được canonical visible history, attachments, tool results và các artifact hợp lệ/được hỗ trợ.
- Hot-switch nghĩa là **đổi provider cho request tiếp theo**, không phải di chuyển request đang chạy sang model khác giữa chừng.

## 2. Ranh giới với Multi-Conversation

| Hạng mục | Kế hoạch 02 thực hiện | Phụ thuộc kế hoạch 01 |
|---|---|---|
| Provider selector (next request) | Có | UI conversation identity |
| ProviderRegistry, adapter, capability check | Có | RunManager gọi adapter bằng immutable spec |
| Canonical history → provider payload | Có | Context Snapshot và Message schema |
| Per-run provider binding, retry/stop | Provider-specific | Run lifecycle, persistence, concurrency controls |
| Stream, patch, debug | Mapping theo provider | Event routing theo `conversation_id`/`run_id` |
| 2 request song song trong một conversation | Chọn provider đúng mỗi run | Scheduler, parent turn + idempotency |

**Nguyên tắc API:** Request lúc Send phải chốt `provider_id`, `model`, `provider_config_version`, `parent_turn_id`, `context_snapshot_id`, `idempotency_key`; mọi callback sau đó xác định provider dựa vào **run_id**, không đọc selected provider hiện tại.

## 2A. ĐẶC TẢ TRIỂN KHAI BẮT BUỘC (áp dụng cho toàn bộ checklist HS)

> **Lưu ý về code thực tế:** Đây là thiết kế mục tiêu, **không khẳng định** đã tồn tại `ProviderRouter`, `RunManager` hay các endpoint dưới đây. Coder phải đọc HEAD TaskDeck, xác minh Composer/GPT là loại provider/agent đang cấu hình như thế nào và dùng lại executor hiện có. Không tự tạo một mô hình concurrent run thay cho nhóm MC.

### A. Hướng dẫn giao việc trực tiếp cho coder AI

```text
Bạn là coder AI phụ trách FEATURE HS — Hot-Switch Agent/Provider của TaskDeck.
- Đọc hết file plan 02 và contract chung trong plan 01, fetch HEAD mới nhất, khảo sát code thật.
- THỰC HIỆN code + test + commit theo thứ tự HS-00→HS-09; không chỉ trả bản phân tích hoặc prompt.
- Chỉ đổi provider cho REQUEST MỚI; mọi request đang chạy phải giữ original provider, stream, patch và retry.
- Bạn phụ trách ProviderRegistry/Adapter, next-provider selector, canonical history serializer và provider-bound execution.
- Không xây RunManager, scheduler, conversation storage riêng; phối hợp coder MC qua DTO/event contract chung.
- Với nơi MC chưa xong: phát triển qua mock/fake RunManager và contract tests, để checkbox integration chưa xong.
- Bảo toàn Composer, GPT, AI Retry (Off / 5 lần / Unlimited), Patch Retry, Debug, Attach to Chat và History cũ.
- Chỉ tick HS-* khi đã thực hiện và có test/bằng chứng; cập nhật plan sau từng commit.
- Cuối batch báo: HEAD trước/sau, files, tasks done, tests, limitations, dependency và Git commit/PR.
```

### B. Ý nghĩa chính xác của việc đổi agent

- `selected_next_provider` thuộc **composer preference của conversation**, không phải thuộc run; thay đổi nó không gửi API tới provider đang chạy và không chạm `AbortController` của run trước.
- Ngay lúc **Send**, backend validate provider/model/capabilities, lưu `run.provider_id`, `run.model`, `provider_config_version`, `context_snapshot_id`, `parent_turn_id`, `idempotency_key` cùng transaction. Các trường đó immutable trong suốt lifetime của run.
- Request #101 gửi Composer khi đang running; người dùng chọn GPT rồi gửi #102 → hai run tồn tại độc lập, có thể complete đảo thứ tự. Context #102 chỉ có những message/outputs đã hoàn thành hoặc đã pin tại thời điểm snapshot, **không bao gồm output chưa xong của #101**.
- Retry #101 phải gọi Composer với context/snapshot cũ theo retry policy. Chỉ thao tác explicit `Retry with another provider` mới tạo child run mới và ghi lineage/audit; **không** sửa provider của #101.
- Native session/thread mỗi provider là riêng biệt. Chỉ trao đổi content có thể truy cập (visible messages, tool outputs, attachments) thông qua canonical transcript; không chuyển hidden/internal reasoning hoặc opaque state từ Composer sang GPT.

### C. Provider Registry + adapter contract (Go pseudocode tham khảo)

```go
// Thay tên và type cho phù hợp kiến trúc thật; đây không phải code có sẵn.
type ProviderKey string

type ProviderCapabilities struct {
    Streaming      bool
    ToolCalls      bool
    FileInput      bool
    ImageInput     bool
    Cancellation  bool
    NativeSessions bool
    MaxContext     int
}

type ExecutionSpec struct {
    RunID             string
    ConversationID    string
    ParentTurnID      string
    ProviderID        ProviderKey
    Model             string
    ConfigVersion     string
    ContextSnapshotID string
    AttemptID         string
}

type ProviderAdapter interface {
    Capabilities() ProviderCapabilities
    Execute(ctx context.Context, spec ExecutionSpec, prompt CanonicalPrompt, sink EventSink) error
    // Cancel có thể trả về best-effort nếu provider không hỗ trợ hủy thực sự.
    Cancel(ctx context.Context, runID, attemptID string) error
}
```

- Registry ánh xạ `provider_id → adapter + config + capabilities`, refresh khi cấu hình thay đổi, không đưa API key thô xuống frontend.
- `EventSink` chuẩn hóa token/text, tool call/result, patch status, usage, error và terminal event; gắn `run_id`, `attempt_id`, `provider_id`, `seq` từ run spec thay vì active tab.
- Kiểm tra capability trước Send: nếu provider không nhận type attachment hay tool bắt buộc thì thông báo cụ thể; không âm thầm lược bỏ ngữ cảnh hoặc fallback.
- Native provider session có thể reuse **chỉ khi** cùng provider/model/conversation lineage + history revision phù hợp; khi đổi provider phải serialize canonical history và tạo session độc lập.

### D. API/DB contract với nhóm MC

| Hạng mục | MC nắm quyền | HS thực hiện |
|---|---|---|
| `POST /api/ai/runs` (route minh họa) | Tạo record/run_id, snapshot, scheduler/queue | Validate selected provider/model và cung cấp immutable `ExecutionSpec` |
| `GET/POST /api/ai/runs/{id}/stop,retry` | Run lifecycle, attempt id, cancellation ownership | Gọi đúng adapter gốc từ run spec; canonicalize response/error |
| `run_events` | Persist/stream/replay theo run_id/seq | Sinh normalized provider/tool/patch events vào EventSink |
| `context_snapshots` | Lưu manifest, shared/local revision | Convert snapshot→provider-specific payload có token budget |
| `conversation preferences` | Persist theo conversation | Cung cấp `selected_next_provider` và UX chọn model cho request kế tiếp |

**Atomic send:** lấy preference hiện tại đúng một lần khi Send (frontend truyền lựa chọn rõ + backend validate), persist request và provider binding trước khi launch; chuyển dropdown sau Send không có quyền làm thay đổi run cũ. Những callback phục vụ stop/retry/patch luôn lookup run spec bằng `run_id`, tuyệt đối không đọc biến `currentProvider` đang thay đổi.

### E. Canonical prompt & context portability

```json
{
  "conversation_id": "conv-123",
  "snapshot_id": "ctx-102",
  "history_revision": "v42",
  "messages": [
    {"role": "user", "kind": "text", "text": "Kiểm tra SmartKey", "turn_id": "t100"},
    {"role": "assistant", "kind": "text", "text": "Đã rà soát phần A", "turn_id": "t101", "origin_provider": "composer"},
    {"role": "user", "kind": "text", "text": "Tiếp tục bằng GPT", "turn_id": "t102"}
  ],
  "excluded_pending_runs": ["run-101"],
  "attachments": [],
  "source_manifest": ["global:v1", "group:v4", "conversation:v42"]
}
```

- Định nghĩa mapper cho user/assistant/tool/system/content parts/file ranges/terminal data; dùng canonical **visible** data thay vì native session state không chuyển được.
- Khi provider cũ được chọn lại, append các lượt thiếu vào session cũ **nếu an toàn**; nếu không, khởi tạo session mới từ canonical snapshot; không được lấy session ID Composer gửi sang GPT.
- Token budget ưu tiên requirements/safety/system, latest task, dependencies, source citations; older history có thể tóm tắt với provenance; cảnh báo loại attachment unsupported, không drop lặng.
- Show rõ `#102 được gửi trước khi #101 hoàn tất; kết quả #101 chưa được đưa vào context #102` nếu liên quan.

### F. Sequence demo và concurrency/cancel

```text
Time   Conversation chat             Composer adapter           GPT adapter
T1     Send #101 (provider=Composer)  run101 RUNNING            -
T2     Select GPT (for next request)  run101 RUNNING            -
T3     Send #102 (provider=GPT)       run101 RUNNING            run102 RUNNING
T4     GPT #102 completes             run101 RUNNING            run102 COMPLETED
T5     Stop/complete #101             run101 STOPPED/COMPLETED   run102 unchanged
```

Các quy tắc bắt buộc: streaming/out-of-order theo run ID; UI badge message dựa trên `run.provider_id`, không dùng dropdown; Stop #101 không cancel #102; Retry #101 dùng original binding. Nếu scheduler queue #102, provider vẫn pin GPT và không đổi dù dropdown chuyển lại Composer.

### G. Retry & Patch Tool compatibility

| Trường hợp | Thực thi bắt buộc |
|---|---|
| Retry Off | Không auto retry; có manual retry rõ ràng |
| Retry 5 times | Mỗi lần lỗi transient retry tối đa 5 lần theo policy đã cấu hình (mặc định delay 10 giây), bộ đếm theo run |
| Retry Unlimited | Tiếp tục tới khi thành công/Stop cho lỗi retryable; dừng/requires action khi lỗi cố định như auth/unsupported input để tránh vòng lặp vô ích |
| Patch failed/hung | Xem status/evidence cũ trước, cho phép **rerun patch** hoặc **resend existing result**; không lặp side effect khi resend |
| Switch khi patch #101 đang chạy | Patch tiếp tục ở run #101/Composer, UI của #102/GPT độc lập |
| Provider #101 unavailable sau restart | Mark INTERRUPTED/FAILED có căn cứ; không dùng provider hiện đang chọn để chạy hộ |

### H. Điều kiện nghiệm thu và báo cáo của coder

- Checklist `[x]` chỉ khi code + test được kiểm chứng + commit/ref, ghi ngay dưới subtask hoặc trong bảng nhật ký cuối file; state cha chỉ tick khi tất cả con đạt và milestone gate pass.
- Test bắt buộc: **Composer #101 RUNNING → chọn GPT → send #102; #102 xong trước; #101 tiếp tục; stop riêng; retry vẫn Composer; reload vẫn đúng binding**.
- Chạy fake provider tests kể cả khi credentials GPT/Composer không sẵn; ghi `BLOCKED` cho smoke test thật nếu chưa cấu hình (không ghi PASS giả).
- Không ghi key/token trong Debug Console; patch/native tool actions phải idempotent hoặc báo unknown-outcome để tránh chạy hai lần.
- Mỗi batch `fetch` HEAD, kiểm tra dirty/uncommitted files, commit theo task, push đúng branch theo quyền; không force-push/reset các thay đổi của coder MC.

## 3. Feature checklist — ba cấp task/subtask

- **Cấp 1:** Milestone/nhóm chức năng (`HS-00`).
- **Cấp 2:** Task có deliverable (`HS-00.01`).
- **Cấp 3:** Subtask có thể chứng minh (`HS-00.01.01`).
- **[ ]** chưa hoàn tất; **[x]** hoàn tất có bằng chứng; blocker được ghi cạnh mục tương ứng nhưng giữ **[ ]**.

- [ ] **HS-00 — Khảo sát provider và chốt hợp đồng chung**
  - [ ] **HS-00.01 — Đối chiếu hiện trạng provider**
    - [ ] **HS-00.01.01** Tìm selector, adapter, credential/config, request executor và vị trí state currentProvider hiện tại.  
      **Cách triển khai:** Đi từ dropdown UI/setting đến handler Send→provider executor; inventory currentProvider, config/secret store, adapter và route.  
      **Kiểm chứng / đầu ra:** Có file/line thực tế; xác định nơi selected provider bị đọc muộn gây race.
    - [ ] **HS-00.01.02** Khảo sát Composer và GPT: native session, streaming, tool calls, patch, retry, stop và lỗi mạng.  
      **Cách triển khai:** Chạy test request trên Composer và GPT nếu cấu hình có sẵn: stream, tool, patch, cancel, timeout, failed; ghi provider native session ID/capabilities và request logs redacted.  
      **Kiểm chứng / đầu ra:** Có trace so sánh từ click Send đến completed/error cho từng provider thực sự dùng.
    - [ ] **HS-00.01.03** Ghi capability matrix thực tế; không giả định provider hỗ trợ chuyển nguyên internal state.  
      **Cách triển khai:** Lập capability matrix: text, streaming, context budget, files/images, tool/function calling, native session resumability, cancellation, retry semantics.  
      **Kiểm chứng / đầu ra:** Matrix chỉ đánh supported khi đã chứng thực qua code/docs/test, unsupported rõ ràng.
  - [ ] **HS-00.02 — Đồng bộ hợp đồng với nhóm Multi-Conversation**
    - [x] **HS-00.02.01** Thống nhất RunRequest, run_id, conversation_id, parent_turn_id, context_snapshot_id, provider_id, attempt và event envelope.  
      **Cách triển khai:** Dùng một schema RunRequest/RunEvent chung với MC (IDs, parent turn, snapshot, provider, attempt, idempotency); fixture JSON versioned.  
      **Kiểm chứng / đầu ra:** Contract test giữa two modules được CI chạy, chưa chốt thì không thay shared code.  
      **Bằng chứng (2026-11-23):** `agentchat.RunSpec`/`RunEvent` là envelope duy nhất; `agentrun.Manager` là nơi duy nhất giữ run state, `agentchat.ProviderRouter` chỉ quyết định/gọi provider. `go test ./internal/agentchat ./internal/agentrun` và `-race` PASS; server test `TestComposerRunCanOverlapGPTRunRetryAndStopIndependently` PASS xác nhận binding bất biến qua retry.
    - [x] **HS-00.02.02** Thống nhất ranh giới: RunManager điều phối/lưu/truyền run; Provider Router quyết định và gọi provider.  
      **Cách triển khai:** RunManager MC nắm scheduling/persistence/cancellation ownership; HS nắm registry/router/serialization; bỏ loop riêng và double retry scheduler trong adapters.  
      **Kiểm chứng / đầu ra:** Không có hai nơi tự quản run state hoặc global AbortController.  
      **Bằng chứng (2026-11-23):** Chỉ `agentrun.Manager` giữ scheduler/persistence/cancel (`internal/agentrun/manager.go`); `agentchat.ConfiguredProviderAdapter` chỉ phát `RunEvent` và dùng chính `spec.RetryMode`, không có retry loop thứ hai (`internal/agentchat/provider_adapter.go`). `TestGlobalConcurrencyNeverExceedsFiveAcrossManagers`, `TestRestartMarksInFlightInterruptedAndRetryKeepsBinding` và `TestAgentChatAndRunManagerShareFiveRunLimit` PASS.
    - [ ] **HS-00.02.03** Định nghĩa cách phát hành feature flags và merge mà không cùng nhóm sửa những file giao nhau không kiểm soát.  
      **Cách triển khai:** Định nghĩa module ownership, shared interface PR first, feature flags, merge sequencing và conflict resolution; nhánh làm việc riêng nếu concurrent coder.  
      **Kiểm chứng / đầu ra:** Hai coder không push cùng HEAD/line mà không sync, integration order ghi rõ.
  - [ ] **HS-00.03 — Baseline và bản thiết kế hành vi**
    - [ ] **HS-00.03.01** Ghi nhận hành vi conversation hiện tại, provider select, retry, patch and debug trước khi chỉnh sửa.  
      **Cách triển khai:** Snapshot baseline provider behavior và các đường phụ Attach to Chat, AI Pack preview, Debug, Patch failed/running; ghi commands/HEAD.  
      **Kiểm chứng / đầu ra:** Có test ledger pass/fail và đường quay lại khi regression.
    - [x] **HS-00.03.02** Chốt chính sách gửi mới khi run cũ chưa hoàn thành: send-now snapshot hoặc explicit queue; default send-now nếu được phép.  
      **Cách triển khai:** Chính sách send-now mặc định nếu scheduler cho phép; nếu queue, reserve provider at Send; ctx #102 chỉ gồm completed turns trước snapshot.  
      **Kiểm chứng / đầu ra:** UI không chờ #101 trừ khi user chọn queue; run #102 hiển thị stale-context warning.  
      **Bằng chứng (2026-11-23):** Send-now là hành vi mặc định: `POST /api/agent/context-snapshots` + `POST /api/agent/runs` không bị chặn bởi run đang chạy. Snapshot #102 chỉ chứa prompt của #101 (chưa có output) và ghim `excluded_pending_run_ids=[run-101]`; UI hiện cảnh báo "Context snapshot excluded unfinished run output". `TestComposerRunCanOverlapGPTRunRetryAndStopIndependently` và `TestContextSnapshotIncludesActiveRunPromptButExcludesItsPartialOutput` PASS.
    - [x] **HS-00.03.03** Phân biệt đổi provider cho NEXT REQUEST với chuyển một run đang chạy (không hỗ trợ chuyển mid-flight).  
      **Cách triển khai:** Chỉ đổi `selected_next_provider`, không hot-migrate live stream/native provider session; hiển thị explanatory tooltip nếu cần.  
      **Kiểm chứng / đầu ra:** Composer #101 còn running khi chọn GPT, không restart/abort #101.  
      **Bằng chứng (2026-11-23):** Selector chỉ ghi preference `taskdeck.agent.next-provider.v1.<conversation_id>`; không có đường code nào đổi binding của run đang chạy. `TestComposerRunCanOverlapGPTRunRetryAndStopIndependently` xác nhận #101 vẫn chạy và giữ provider sau khi #102 GPT hoàn tất trước.

- [ ] **HS-01 — Provider Registry và định danh provider cho mỗi run**
  - [ ] **HS-01.01 — Chuẩn hóa Provider Registry**
    - [ ] **HS-01.01.01** Đăng ký provider_id ổn định, display name, enabled/health, model/config và capabilities.  
      **Cách triển khai:** Registry theo stable provider_id + display_name, enable/config status, models, credentials reference (không raw secret), latency/health; giữ adapter list từ code thật.  
      **Kiểm chứng / đầu ra:** List API chỉ trả provider khả dụng; ID không thay nếu đổi display name.
    - [ ] **HS-01.01.02** Hỗ trợ các capability: streaming, tool calling, image/file inputs, context limits, cancellation, sessions.  
      **Cách triển khai:** Capabilities typed booleans/limits/toolset; gating theo payload trước Send; tách `canCancel` khỏi `supportsInterruptStreaming` và native session resume.  
      **Kiểm chứng / đầu ra:** Test fake provider thiếu files/tools chặn gửi với lỗi rõ.
    - [ ] **HS-01.01.03** Không hiển thị provider đã tắt/unavailable như lựa chọn hợp lệ; giữ thông báo lỗi có thể khắc phục.  
      **Cách triển khai:** Selector hiển thị unavailable disabled kèm lý do config missing/quota/permission; refresh capabilities khi settings đổi.  
      **Kiểm chứng / đầu ra:** Không silent fallback và không cho gửi tới provider bị disable.
  - [ ] **HS-01.02 — Adapter contract**
    - [ ] **HS-01.02.01** Định nghĩa execute(spec, context, callbacks), cancel(run_id), normalize events/errors, timeout semantics.  
      **Cách triển khai:** Định nghĩa `execute(ctx,spec,prompt,sink)` sinh normalized token/tool/usage/error events, `cancel(runID)`, `inspect(operationID)`; chọn cancellation semantics theo API thật.  
      **Kiểm chứng / đầu ra:** Fake adapter deterministic chạy same contract cho GPT/Composer.
    - [ ] **HS-01.02.02** Bọc Composer adapter/GPT adapter hiện tại theo contract mà không làm hỏng đường gọi cũ.  
      **Cách triển khai:** Wrap adapters đang có thay vì viết lại network stack; map legacy callbacks/error codes sang normalized event; giữ debug trace redacted.  
      **Kiểm chứng / đầu ra:** Single-provider regression test vẫn pass khi flag hot-switch off.
    - [ ] **HS-01.02.03** Tách provider session ID (nếu có) khỏi conversation ID của TaskDeck.  
      **Cách triển khai:** Persist provider_session mapping keyed conversation+provider+model/session lineage; không lấy native session id làm conversation id hoặc reuse across providers.  
      **Kiểm chứng / đầu ra:** Composer→GPT→Composer giữ internal identities riêng không cross-route.
  - [ ] **HS-01.03 — Immutable run binding**
    - [x] **HS-01.03.01** Khi bấm Send, chốt provider_id, model, config version và context snapshot vào run.  
      **Cách triển khai:** In one transaction at Send, resolve and validate selected provider/model/config_version, snapshot provider binding và context manifest trong run record.  
      **Kiểm chứng / đầu ra:** Đổi selector 100 lần sau Send không thay run.provider_id.  
      **Bằng chứng (2026-11-23):** RunSpec chốt `ProviderID`, `ModelID`, `ProviderConfigVersion`, `CredentialFingerprint`, `ContextSnapshotID` và `ParentTurnID` lúc Send; snapshot lưu `RunBindingSnapshot` đúng các giá trị đó và `verifyAgentRunSnapshotBinding` từ chối lệch. `TestAgentContextSnapshotReturnsSafePinnedRunBinding`, `TestRunSpecRequiresCompleteImmutableBinding` và `TestComposerRunCanOverlapGPTRunRetryAndStopIndependently` PASS.
    - [x] **HS-01.03.02** Không đọc biến currentProvider để route stream/tool response/stop/retry của run đang chạy.  
      **Cách triển khai:** Every callback resolves run_id→immutable spec, including stream/tools/patch/retry/stop; grep + refactor global currentProvider reads ngoài UI preference.  
      **Kiểm chứng / đầu ra:** Interleaved Composer/GPT events không swap destinations.  
      **Bằng chứng (2026-11-23):** Không tồn tại biến `currentProvider` trong `internal/agentchat`; mọi event đi qua `runEventSink{manager,runID}` và `Manager` ghi vào record của đúng `runID`. `TestComposerRunCanOverlapGPTRunRetryAndStopIndependently` xác nhận output #102 chỉ nằm trong record của #102; `TestStopOnlyCancelsSelectedRun` xác nhận Stop chỉ hủy run được chọn.
    - [ ] **HS-01.03.03** Thêm migration/backfill provider info cho run/history cũ nếu dữ liệu có thể xác định; nếu không, đánh unknown.  
      **Cách triển khai:** Backfill provenance only if known, e.g. stored provider tags; else `unknown` with fallback display; migration idempotent/non-destructive.  
      **Kiểm chứng / đầu ra:** Old History displayed accurately; không gán GPT giả cho old Composer.

- [ ] **HS-02 — UI Hot-Switch và trải nghiệm chuyển provider**
  - [ ] **HS-02.01 — Selector theo conversation cho request tiếp theo**
    - [ ] **HS-02.01.01** Bổ sung dropdown/provider selector vào composer; hiển thị NEXT: Composer/GPT rõ ràng.  
      **Cách triển khai:** Provider dropdown nhỏ trong footer composer, current `Next: GPT`, shortcut keyboard accessible; selected state scoped conversation.  
      **Kiểm chứng / đầu ra:** Changing C1 selection does not alter C2; Send binds visible choice.
    - [ ] **HS-02.01.02** Cho phép chuyển khi conversation đang có một hoặc nhiều run active; không reset draft/attachments.  
      **Cách triển khai:** Remove disabled-on-running global selector, keep draft/attachments on switch, update UI immediately while preserving active run streams.  
      **Kiểm chứng / đầu ra:** Start #101 Composer, switch GPT, request #101 remains streaming.
    - [x] **HS-02.01.03** Lưu selected_next_provider theo conversation; có default của project và tùy chọn dùng mặc định.  
      **Cách triển khai:** Persist `next_provider_id` per conversation with project default fallback; explicit reset uses project default, never overwrites active runs.  
      **Kiểm chứng / đầu ra:** Reload restores C1 GPT and C2 Composer choices.  
      **Bằng chứng (2026-11-23):** `web/featuremods/agentchat_runs_ui.js` lưu preference theo khoá `taskdeck.agent.next-provider.v1.<conversation_id>` và khôi phục khi remount; fallback về provider/model của session. Node test trong `web/agentchat_bootstrap_test.cjs` (Composer #101 → GPT #102, reload khôi phục GPT, quay lại Composer) PASS.
  - [ ] **HS-02.02 — Thông tin provider trên từng message**
    - [x] **HS-02.02.01** Hiển thị badge provider và trạng thái run trên response, request pending và History.  
      **Cách triển khai:** Badges use run.provider_id (not selected_next_provider) in transcript, pending cards and History; unknown legacy rendered neutral.  
      **Kiểm chứng / đầu ra:** Switch UI after Send does not repaint old response badge.  
      **Bằng chứng (2026-11-23):** `agentProviderBadge` trong `web/featuremods/agentchat.js` đọc `item.provider_id`/`item.model_id` của history item (mang provenance từ `OriginRunID`), không đọc selector hiện tại; run card đọc `run.spec.provider_id`. Node bootstrap test xác nhận badge và card giữ đúng provider sau khi đổi selector.
    - [x] **HS-02.02.02** Khi request #101 đang Composer và #102 là GPT, UI hiển thị song song, stop/retry tách biệt.  
      **Cách triển khai:** Show two running cards #101 Composer, #102 GPT with independent Stop/Retry/Inspect; fix loading state keyed run.  
      **Kiểm chứng / đầu ra:** Stop #101 keeps #102 progress and its actions accessible.  
      **Bằng chứng (2026-11-23):** `web/featuremods/agentchat_runs_ui.js` render card theo `run.spec.run_id`, action Stop/Retry gọi endpoint có runID cụ thể. Node bootstrap test xác nhận Stop #101 không đổi trạng thái #102 và card #102 vẫn hiển thị; thêm `web/agentchat_runs_isolation_test.cjs` xác nhận poll của conversation cũ không vẽ dữ liệu sang conversation mới.
    - [x] **HS-02.02.03** Hiển thị cảnh báo nếu #102 không bao gồm output #101 vì #101 chưa hoàn tất tại thời điểm snapshot.  
      **Cách triển khai:** Compute prior_active_run exclusion at snapshot_at_send; show informational warning `#101 chưa nằm trong context #102`, optional queue setting.  
      **Kiểm chứng / đầu ra:** Warning appears only when there is relevant unfinished prior run.  
      **Bằng chứng (2026-11-23):** Snapshot ghim `excluded_pending_run_ids` cho các run queued/running/waiting_tool; run card hiển thị "Context snapshot excluded unfinished run output: run-101" chỉ khi danh sách này khác rỗng. Node bootstrap test và `TestContextSnapshotIncludesActiveRunPromptButExcludesItsPartialOutput` PASS.
  - [ ] **HS-02.03 — Khả dụng và lỗi dễ hiểu**
    - [ ] **HS-02.03.01** Trình bày lý do provider không sẵn sàng hoặc không hỗ trợ tool/attachment hiện tại.  
      **Cách triển khai:** Capability preflight check attachment types, tool list, context window and provider available; display actionable validation error.  
      **Kiểm chứng / đầu ra:** No dropped files/tools/critical prompt in silent conversion.
    - [ ] **HS-02.03.02** Không âm thầm đổi provider nếu user đã chọn cụ thể mà provider lỗi; chỉ fallback khi user bật policy.  
      **Cách triển khai:** Default explicit provider choice must fail rather than reroute silently; optional opt-in fallback with audit new attempt/run in HS-05.  
      **Kiểm chứng / đầu ra:** GPT quota error never secretly invokes Composer.
    - [ ] **HS-02.03.03** Giữ tương thích shortcut Send, Enter, Attach to Chat, AI Pack preview và nút đóng panel.  
      **Cách triển khai:** Keep Enter/Shift+Enter, send shortcut, Attach UI, preview-close, focus/selection semantics; selector must not grab Enter unexpectedly.  
      **Kiểm chứng / đầu ra:** UI regression tests for keyboard and attachments on hot-switch.

- [ ] **HS-03 — Canonical conversation history và context portability**
  - [ ] **HS-03.01 — Mô hình trung gian độc lập provider**
    - [ ] **HS-03.01.01** Chuẩn hóa user/assistant/tool/system messages, attachments, tool results, timestamps và file/line provenance.  
      **Cách triển khai:** Canonical message shape role, kind/text, tool_call/result, attachment references, provenance, origin_run_id, timestamps, source provider tags.  
      **Kiểm chứng / đầu ra:** Serializer roundtrip preserves visible chat and line-numbered file excerpts.
    - [x] **HS-03.01.02** Gắn parent_turn/run reference để giữ đúng thứ tự logic khi hai response trả về đảo thứ tự.  
      **Cách triển khai:** DAG history parent_turn_id with snapshot anchor; never flatten by completion timestamp, keep partial run exclusion semantics.  
      **Kiểm chứng / đầu ra:** GPT #102 response before Composer #101 still attached to #102 prompt.  
      **Bằng chứng (2026-11-23):** `agentchat.HistoryItem` mang `ParentTurnID`, `OriginRunID`, `ProviderID`, `ModelID`; snapshot chọn parent turn bằng `parent_turn_id` chứ không theo timestamp, và chỉ nối output của run Completed. `TestHistoryItemCarriesRunProvenance`, `TestContextSnapshotIncludesActiveRunPromptButExcludesItsPartialOutput` và `TestComposerRunCanOverlapGPTRunRetryAndStopIndependently` PASS.
    - [ ] **HS-03.01.03** Phân biệt text hữu hình và dữ liệu nội bộ provider; không yêu cầu xuất private reasoning/opaque state.  
      **Cách triển khai:** Only user-visible content and permitted tool outputs in canonical data; native hidden thoughts/session secrets are opaque refs or omitted.  
      **Kiểm chứng / đầu ra:** Cross-provider payload contains no private reasoning or auth headers.
  - [ ] **HS-03.02 — Bộ chuyển đổi context theo provider**
    - [x] **HS-03.02.01** Mapper canonical → Composer/GPT request, giữ roles, instructions, source links và tool outputs hỗ trợ.  
      **Cách triển khai:** Provider-specific serializers build payload from canonical snapshots, map roles/tool calls/text, file refs and system instructions; log payload schema not secrets.  
      **Kiểm chứng / đầu ra:** Golden fixture Composer→GPT retains prompts, visible results and attachments when supported.  
      **Bằng chứng (2026-11-23):** `agentchat.CanonicalProviderPayloadBuilder` dựng canonical request rồi `BuildChatCompletionsRequest`/`BuildResponsesRequest` chiếu sang từng API; không mang `previous_response_id`, `conversation` hay metadata nội bộ. `TestCanonicalPayloadBuilderMapsHistoryAndDropsOpaqueSessionState` và `TestCanonicalPayloadBuilderReplaysToolContinuationForBothProviderAPIs` PASS.
    - [ ] **HS-03.02.02** Handle unsupported attachment/tool bằng thông báo rõ, không bỏ lặng dữ liệu quan trọng.  
      **Cách triển khai:** Preflight unsupported media/tools: convert only safe types with review or refuse; generate error listing exact affected items, never silently strip.  
      **Kiểm chứng / đầu ra:** Send unsupported binary to text-only provider yields explicit actionable refusal.
    - [ ] **HS-03.02.03** Áp dụng token budget, truncation/summarization có manifest và context snapshot version.  
      **Cách triển khai:** Token budget per provider model, priority system/task/user, summarize older completed history with provenance; persist exact snapshot revision/manifest.  
      **Kiểm chứng / đầu ra:** Test long history within model context and critical instruction retained.
  - [ ] **HS-03.03 — Session bridging và đồng bộ**
    - [ ] **HS-03.03.01** Nếu provider có thread/session gốc thì chỉ reuse cho các run tương thích; có mapping riêng theo provider+conversation.  
      **Cách triển khai:** Mapping native session keyed provider+conversation+model+history revision; verify lineages before reuse; start fresh if unsupported.  
      **Kiểm chứng / đầu ra:** Native session from Composer never passed to GPT.
    - [ ] **HS-03.03.02** Khi chuyển provider, dựng context từ canonical history/snapshot thay vì reuse opaque session của provider khác.  
      **Cách triển khai:** On switch, rebuild provider prompt from canonical transcript rather than sharing opaque session ID; include note of unsupported hidden state.  
      **Kiểm chứng / đầu ra:** GPT can answer user-visible previous Composer work without internal Composer session.
    - [ ] **HS-03.03.03** Khi quay lại provider cũ, nạp lại các lượt chat liên quan sau lần dùng trước để tránh mất bối cảnh.  
      **Cách triển khai:** Track last_included_turn per provider session; on return replay missing completed turns or rebuild session if it cannot append safely.  
      **Kiểm chứng / đầu ra:** Composer→GPT→Composer preserves relevant GPT visible output.

- [ ] **HS-04 — Dispatcher và semantics của request đồng thời**
  - [ ] **HS-04.01 — Gửi request đúng provider đã chọn**
    - [ ] **HS-04.01.01** Gửi request với selected provider được snapshot đồng bộ tại thời điểm Send; chống race đổi dropdown liên tục.  
      **Cách triển khai:** Send handler reads selected provider exactly once and transactionally saves RunRequest+snapshot, then dispatches on stored spec; protect against quick dropdown events.  
      **Kiểm chứng / đầu ra:** Rapid toggle and Send always binds provider displayed at click snapshot.
    - [ ] **HS-04.01.02** Hỗ trợ Composer #101 vẫn RUNNING khi GPT #102 bắt đầu; không chặn bởi global busy flag.  
      **Cách triển khai:** Remove per-conversation/global busy guards for concurrent run when scheduler policy allows; issue separate run_id and stream subscription.  
      **Kiểm chứng / đầu ra:** Fake provider C101 blocked 5s while GPT C102 finishes successfully.
    - [ ] **HS-04.01.03** Tính eligibility/concurrency theo RunManager; nếu queue, vẫn lưu provider đã chọn vào request queued.  
      **Cách triển khai:** RunManager checks cap/project quotas; queued RunRequest retains pinned provider/model; UI says queued with reason and cancel option.  
      **Kiểm chứng / đầu ra:** Queue GPT run while cap reached then process by GPT even if selector now Composer.
  - [ ] **HS-04.02 — Streaming, result và tool dispatch**
    - [ ] **HS-04.02.01** Route token delta, tool request/result, patch status và errors theo run_id bất kể tab/provider đang chọn.  
      **Cách triển khai:** Normalize events with run_id from originating executor callback, no activeProvider reads; map patch and tool callbacks to origin run.  
      **Kiểm chứng / đầu ra:** Interleaved token/patch events never bleed into other provider transcript.
    - [ ] **HS-04.02.02** Ghi parent_turn_id; response #102 về trước #101 vẫn gắn đúng prompt và snapshot.  
      **Cách triển khai:** Store immutable parent_turn_id, snapshot_rev and response message run_id; UI renders at correct prompt even if completion order reversed.  
      **Kiểm chứng / đầu ra:** Test #102 completes first with persistent/reloaded transcript stable.
    - [ ] **HS-04.02.03** Gắn operation IDs và tránh cùng lúc hai provider ghi đè chung vùng file/patch không an toàn.  
      **Cách triển khai:** Workspace operation lock/lease keyed workspace/path for patch writes, operation IDs for retry dedup; coordinate MC Workspace Manager.  
      **Kiểm chứng / đầu ra:** Concurrent file edits produce explicit conflict, not silent overwrite.
  - [ ] **HS-04.03 — Stop và retry độc lập**
    - [ ] **HS-04.03.01** Stop request #101 chỉ hủy adapter/cancel context #101, không ảnh hưởng GPT #102.  
      **Cách triển khai:** Stop endpoint calls adapter cancellation keyed run_id+attempt_id, detach only matching stream, maintain second run/patch contexts.  
      **Kiểm chứng / đầu ra:** Stop Composer #101, GPT #102 remains running and can complete.
    - [ ] **HS-04.03.02** Retry request #101 giữ nguyên provider_id/model/config đã pin; không dùng current selected provider.  
      **Cách triển khai:** Retry clones immutable provider spec/snapshot, increments attempt, respects Off/5/unlimited snapshot and original model; UI current choice irrelevant.  
      **Kiểm chứng / đầu ra:** Toggle Composer→GPT→retry #101 still invokes Composer.
    - [ ] **HS-04.03.03** Cho phép tùy chọn Explicit Retry with another provider tạo run mới có provenance; không biến đổi run cũ.  
      **Cách triển khai:** Explicit `Retry with another provider` opens selection and creates child run with lineage, warning about different context/tool capabilities; audit originals.  
      **Kiểm chứng / đầu ra:** Old run remains unchanged; new run displays switched provider and parent link.

- [ ] **HS-05 — Error, recovery và chính sách provider**
  - [ ] **HS-05.01 — Retry cùng provider gốc**
    - [ ] **HS-05.01.01** Tích hợp Off/5 times/unlimited với delay và stop policy hiện có; kiểm tra retry per run, không per conversation.  
      **Cách triển khai:** AI Retry policy stored per run: Off/5 attempts with 10s between retries/unlimited until stop, with independent timers and jitter if config supports.  
      **Kiểm chứng / đầu ra:** Two runs retry with separate counters, Stop ends one immediately.
    - [ ] **HS-05.01.02** Phân biệt transient network/provider error với invalid request/tool incompatibility để không retry vô hạn lỗi cố định.  
      **Cách triển khai:** Classify errors retryable network/429/5xx vs invalid auth/unsupported input; bounded exponential for rate limits if allowed; unlimited not loop permanent error.  
      **Kiểm chứng / đầu ra:** Fake 401/400 fails fast, 503 retries per selected policy.
    - [ ] **HS-05.01.03** Patch failed/hung có hai đường riêng: chạy lại patch hoặc chỉ gửi lại kết quả patch cũ; giữ provider tương ứng.  
      **Cách triển khai:** Separate rerun_patch from resend_patch_result actions, inspect original patch_id/status first; keep provider/run binding during feedback to AI.  
      **Kiểm chứng / đầu ra:** Resend old result never reapplies patch; hanging patch not treated as success.
  - [ ] **HS-05.02 — Recovery và idempotency**
    - [ ] **HS-05.02.01** Khi reload/restart, khôi phục selected_next_provider và run.provider_id chính xác.
      **Cách triển khai:** Persist next_provider and run binding across restart; reconciliation marks interrupted when actual native worker unreachable.  
      **Kiểm chứng / đầu ra:** After reload UI choice GPT but #101 still shows Composer.
    - [x] **HS-05.02.02** Stream reconnect không tạo thêm provider request; retry có attempt_id/idempotency key.
      **Bằng chứng (2026-11-23):** Recovery tests dùng subprocess bị kill thật (`TestAgentRunRecoversAfterProcessKillWithoutReplayingSideEffects`, `TestAgentRunUncertainProviderResultRequiresReconciliation`, `TestAgentRunReloadAndRetryKeepsSingleProviderAttempt`) PASS: daemon mới không replay, reviewed retry tạo đúng 1 attempt với attempt ID/idempotency key mới.
      **Cách triển khai:** Use operation_id/attempt_id/idempotency key for replay and retry; stream reconnect only resubscribes, no network call to provider.  
      **Kiểm chứng / đầu ra:** Simulate dropped network, refresh, resend: one request execution.
    - [x] **HS-05.02.03** Nếu provider run đã interrupted, hiển thị trạng thái và yêu cầu thao tác phù hợp thay vì tự báo completed.
      **Bằng chứng (2026-11-23):** Recovery tests xác nhận run bị kill được phục hồi là `interrupted`, `RetrySafe=false`, có `LastError`, từ chối auto retry/review thiếu xác nhận và chỉ reviewed resume tạo thêm provider call.
      **Cách triển khai:** Differentiate lost-connection UI vs failed worker; inspect provider/patch state when possible; show Retry/Inspect/Resume options accurately.  
      **Kiểm chứng / đầu ra:** Cannot claim completed without terminal record, and no blind tool redo.
  - [ ] **HS-05.03 — Fallback policy minh bạch**
    - [ ] **HS-05.03.01** Định nghĩa optional fallback manual/auto opt-in, điều kiện, provider đích, ghi audit và consent.  
      **Cách triển khai:** Define fallback opt-in stored as policy: eligible errors, target priority, max attempts; explicit status badge/audit change.  
      **Kiểm chứng / đầu ra:** Without opt-in, no cross-provider request; with it, origin and target linked.
    - [ ] **HS-05.03.02** Không tự chuyển khi đang chạy; fallback hợp lệ tạo attempt/run được đánh dấu rõ ràng.  
      **Cách triển khai:** Disallow mid-flight provider replacement; fallback starts new attempt/run with separate provider spec and context provenance.  
      **Kiểm chứng / đầu ra:** Active run A immutable; fallback B is visibly separate.
    - [ ] **HS-05.03.03** Xử lý timeout/rate limit/quota với thông điệp rõ và status code chuẩn hóa.  
      **Cách triển khai:** Normalize provider errors timeout, 401/403, quota, 429 and malformed tool; attach retry_after for rate limit, action text without leaked raw credentials.  
      **Kiểm chứng / đầu ra:** Error messages actionable in UI and debug export.

- [ ] **HS-06 — Tích hợp Patch Tool, attachments và Debug Console**
  - [ ] **HS-06.01 — Tool và patch đồng nhất**
    - [ ] **HS-06.01.01** Ánh xạ tool result, files affected, diff, native patch sessions vào originating run.  
      **Cách triển khai:** Native patch job maps to `origin_run_id` and provider_id, files affected/diff/output metadata; status query independent of active tab/provider.  
      **Kiểm chứng / đầu ra:** Composer patch links still point to Composer after switch to GPT.
    - [ ] **HS-06.01.02** Sau khi chuyển provider, patch old run vẫn tiếp tục và status không bị chuyển sang provider mới.  
      **Cách triển khai:** Patch progress/result callbacks resolve pinned run regardless of next-provider change; continue existing patch workflow until terminal state.  
      **Kiểm chứng / đầu ra:** Switch during Patch RUNNING does not reset spinner or misroute result.
    - [ ] **HS-06.01.03** Nếu hai provider cùng chạm files/terminal, dùng Workspace Manager để serialize/lock/isolated worktree.  
      **Cách triển khai:** Use MC Workspace Manager to isolate worktrees or serialize shared path writes; no provider-specific duplicate locking service.  
      **Kiểm chứng / đầu ra:** File contention scenario rejects stale patch cleanly.
  - [ ] **HS-06.02 — File/console context trên request mới**
    - [ ] **HS-06.02.01** Attach to Chat từ file explorer/tab/terminal/selection đi vào draft của conversation đúng và request mới.  
      **Cách triển khai:** Attach to Chat source actions target active draft conversation_id at click time and store immutable attachment refs in next run Send.  
      **Kiểm chứng / đầu ra:** Attach terminal from C1 then switch C2: C2 attachment list remains distinct.
    - [ ] **HS-06.02.02** Gắn file path, selected line range, content, source revision vào snapshot trước khi gửi.  
      **Cách triển khai:** Attachment manifest captures path, line start/end, content checksum, git commit/workspace revision, terminal session timestamp.  
      **Kiểm chứng / đầu ra:** Opening request Inspector shows exact file fragment sent, even if file later edited.
    - [ ] **HS-06.02.03** Kiểm tra attachment compatibility với provider mới và hiển thị warning nếu không thể chuyển đổi.  
      **Cách triển khai:** Provider preflight rejects unsupported attachment or converts to compatible canonical text only with explicit indication.  
      **Kiểm chứng / đầu ra:** No data silently dropped when switching to GPT text-only endpoint.
  - [ ] **HS-06.03 — Debug/observability**
    - [ ] **HS-06.03.01** Ghi selection-change, send, provider binding, stream, retry, stop, errors và patch operation theo correlation ID.  
      **Cách triển khai:** Emit structured events provider_selected/started/stream_chunk/retry/stop/patch/error with IDs; use trace spans per run, never rely on current UI.  
      **Kiểm chứng / đầu ra:** Filter trace for run #101 shows one coherent Composer execution.
    - [ ] **HS-06.03.02** AI Debug Console filter provider_id, conversation_id, run_id, attempt_id; có export redacted bundle.  
      **Cách triển khai:** Debug panel filters provider, convo, run, attempt, displays redacted normalized request/response and downloadable bundle.  
      **Kiểm chứng / đầu ra:** User can correlate #101/#102 without reading unrelated project secrets.
    - [ ] **HS-06.03.03** Không log API keys, cookies, bearer tokens, secret headers hoặc user credentials.  
      **Cách triển khai:** Redact API keys, cookies, passwords, auth headers and key-bearing query params in logs/export; integrate redaction test fixtures.  
      **Kiểm chứng / đầu ra:** Automated scanner fixture confirms no secret material exported.

- [ ] **HS-07 — Schema, API và backward compatibility**
  - [ ] **HS-07.01 — Data/API changes**
    - [ ] **HS-07.01.01** Bổ sung immutable provider fields trong agent_runs, last_selected_provider của conversation (hoặc bảng preference).  
      **Cách triển khai:** Extend run table immutable provider_id/model/config_ref; conversation preference table next_provider; migrations support old DB.  
      **Kiểm chứng / đầu ra:** Schema v2 loads legacy data; old runs not mutated on select.
    - [ ] **HS-07.01.02** API send yêu cầu/nhận provider_id được xác thực; không để caller giả mạo provider capabilities.  
      **Cách triển khai:** Send API validates provider against registry/config/capability, accepts explicit provider_id or default; server—not UI—enforces permissions.  
      **Kiểm chứng / đầu ra:** POST disabled/unknown provider returns 4xx without invoking adapter.
    - [ ] **HS-07.01.03** API list/status/history trả provider metadata theo run, không chỉ theo conversation.  
      **Cách triển khai:** Include binding and capability warning in run/history/status API; maintain backward-compatible fields for existing consumers.  
      **Kiểm chứng / đầu ra:** Single-chat UI and history API load when feature flag off.
  - [ ] **HS-07.02 — Migration**
    - [ ] **HS-07.02.01** Gán default provider cho history cũ khi có bằng chứng; không tự suy diễn cho record không rõ.  
      **Cách triển khai:** Only backfill known original provider from stored metadata; unknown remains unknown; migration migration_version prevents duplicate processing.  
      **Kiểm chứng / đầu ra:** No forged provider display for legacy records.
    - [ ] **HS-07.02.02** Không làm mất nội dung chat cũ và không buộc người dùng xóa/recreate conversation.  
      **Cách triển khai:** Keep canonical historical message text/attachments unchanged; supply mapper for legacy read and safe export.  
      **Kiểm chứng / đầu ra:** Legacy fixture hash and message count stable after upgrade.
    - [ ] **HS-07.02.03** Có rollback/feature flag sao cho giao diện đơn provider tiếp tục hoạt động khi flag off.  
      **Cách triển khai:** Flag hot_switch_provider gates new selector, not deletion of new schema; rollback to old UI uses saved default and existing history.  
      **Kiểm chứng / đầu ra:** Flag disable during active run does not break stop/recovery.
  - [ ] **HS-07.03 — Kiểm tra hợp đồng hai feature**
    - [ ] **HS-07.03.01** Đảm bảo MC RunManager chấp nhận provider binding snapshot và không ghi đè khi retry.  
      **Cách triển khai:** Integration MC RunManager persists chosen provider once and passes immutable spec into HS adapter across stop/retry.  
      **Kiểm chứng / đầu ra:** Contract test asserts retry #101 Composer after selector GPT.
    - [ ] **HS-07.03.02** Đảm bảo context group/snapshot của MC có thể build prompt cho nhiều provider.  
      **Cách triển khai:** HS serializer reads MC Context Snapshot manifest and source revisions, applies provider budget and capability conversion.  
      **Kiểm chứng / đầu ra:** Same group context reaches GPT/Composer with equivalent visible instructions.
    - [ ] **HS-07.03.03** Chạy contract tests để bắt mismatch khi hai nhóm thay đổi schema/typed DTO.  
      **Cách triển khai:** Version schema/DTO fixture for run/event/snapshot; CI cross-module contract test fails on breaking field changes.  
      **Kiểm chứng / đầu ra:** Both coder branches can integrate without runtime schema mismatch.

- [ ] **HS-08 — Tests theo ma trận chuyển provider**
  - [ ] **HS-08.01 — Unit tests**
    - [ ] **HS-08.01.01** Test snapshot provider tại Send và đổi selector khi run vẫn đang running.  
      **Cách triển khai:** Unit test selector change between SaveRun and StartExecutor and after first stream chunk, use fake model adapters.  
      **Kiểm chứng / đầu ra:** run.provider_id stable under rapid UI selection toggles.
    - [ ] **HS-08.01.02** Test provider-specific mapping/unsupported types, capability check và provider error normalization.  
      **Cách triển khai:** Golden tests canonical → Composer/GPT serialization, unsupported media, errors and no hidden state transfer.  
      **Kiểm chứng / đầu ra:** Stable deterministic payload, correct capability validation failures.
    - [ ] **HS-08.01.03** Test Retry/Stop/Queue không phụ thuộc selected_next_provider và chạy đúng provider cũ.  
      **Cách triển khai:** Fake clock tests queue/stop/retry across two providers; assert original binding and separate timers.  
      **Kiểm chứng / đầu ra:** No retry uses UI currently selected provider.
  - [ ] **HS-08.02 — Integration tests**
    - [ ] **HS-08.02.01** Composer #101 đang stream → đổi GPT → gửi #102; cả hai vẫn chạy và trả về đúng nơi.  
      **Cách triển khai:** Start slow Composer #101, switch provider, issue GPT #102; both stream independent and store distinct provider tags.  
      **Kiểm chứng / đầu ra:** Both complete with correct messages/run IDs.
    - [ ] **HS-08.02.02** GPT #102 hoàn tất trước Composer #101 và ngược lại; lịch sử vẫn liên kết đúng parent turn.  
      **Cách triển khai:** Inject reversed completion and reconcile render/persistence, including frontend reload and event replay.  
      **Kiểm chứng / đầu ra:** History stable and #101/#102 response ownership correct.
    - [ ] **HS-08.02.03** Composer lỗi → chọn GPT → retry Composer vẫn Composer; stopped #101 không hủy #102.  
      **Cách triển khai:** Simulate Composer failure+retry while selector GPT, stop Composer during GPT running; inspect request invocations.  
      **Kiểm chứng / đầu ra:** Composer retry targets Composer and no abort to GPT #102.
  - [ ] **HS-08.03 — E2E & reliability**
    - [ ] **HS-08.03.01** Reload app khi #101 active và selector GPT; trạng thái/selected provider/History được phục hồi.  
      **Cách triển khai:** E2E start #101, switch GPT, reload frontend while active and inspect post-reload run status plus preference persistence.  
      **Kiểm chứng / đầu ra:** Run badge Composer, next selector GPT, no duplicate invocations.
    - [ ] **HS-08.03.02** Test switching khi có AI Pack, file selection, terminal log, patch failed/hung, tool calls.  
      **Cách triển khai:** Run UI flow with AI Pack, line-range Attach, terminal log, failed/hung Patch Tool, debug console; switch at each checkpoint.  
      **Kiểm chứng / đầu ra:** All attachments and patch results assigned to correct run after switch.
    - [ ] **HS-08.03.03** Chạy stress chuyển selector liên tục, gửi nhanh, disconnect/reconnect, quota/rate limit, unsupported tool.  
      **Cách triển khai:** Stress rapid select+send, WS disconnect, 429/timeout and unsupported tools with fake provider latency; record flakiness.  
      **Kiểm chứng / đầu ra:** No race, lost output, or secret leak; failures visible with reason.

- [ ] **HS-09 — Rollout, tài liệu và tiêu chí đóng**
  - [ ] **HS-09.01 — Feature rollout**
    - [ ] **HS-09.01.01** Đặt feature flag hot_switch_provider và metrics switch_attempts, binding_mismatch, retries, provider_errors.  
      **Cách triển khai:** Flag `hot_switch_provider` and metrics selected/bound/mismatch/retry/failure, counters aggregate without secrets.  
      **Kiểm chứng / đầu ra:** Dashboard shows zero binding mismatch on E2E; flag off restores legacy selector.
    - [ ] **HS-09.01.02** Bật theo từng provider được xác nhận capability; có rollback không làm mất history.  
      **Cách triển khai:** Enable providers only after capability matrix validated; staged canary with manual rollback action preserving schema/history.  
      **Kiểm chứng / đầu ra:** Flag toggle on/off verified on existing conversations and patch runs.
    - [ ] **HS-09.01.03** Đối chiếu hành vi với các bản app/runtime supported và thử trên đường khởi động lại thực tế.  
      **Cách triển khai:** Run real environment build and smoke with available Composer/GPT configs; record unsupported environments rather than claim pass.  
      **Kiểm chứng / đầu ra:** Report exact OS/build/test command, PASS/FAIL/BLOCKED and logs.
  - [ ] **HS-09.02 — Bàn giao**
    - [ ] **HS-09.02.01** Viết spec chuyển provider, limitations, guide sử dụng và sơ đồ sequence request #101/#102.  
      **Cách triển khai:** Document user semantics `Next request`, out-of-order history, retry same provider, capability limits, sequence diagram #101/#102.  
      **Kiểm chứng / đầu ra:** Another coder can reproduce hot-switch without tribal knowledge.
    - [ ] **HS-09.02.02** Ghi commit SHA, build/test evidence, file changes và known limitations sau từng milestone.  
      **Cách triển khai:** Update plan checkbox as work completes, log HEAD/branch/commit/test matrix and paths; include code review notes.  
      **Kiểm chứng / đầu ra:** All completed subtask has commit+test evidence.
    - [ ] **HS-09.02.03** Đóng milestone khi cả checklist con và acceptance scenarios liên quan hoàn tất.  
      **Cách triển khai:** Close milestone after subtasks and acceptance gates; CI plus manual smoke verified, remaining issues recorded.  
      **Kiểm chứng / đầu ra:** No milestone green while subtask unchecked or test failed.

## 4. Thứ tự triển khai và cổng nghiệm thu

| Giai đoạn | Phụ thuộc | Điều kiện qua giai đoạn |
|---|---|---|
| Kiến trúc chung | HS-00 → HS-01 | Registry + adapter contract + immutable per-run binding |
| Chuyển provider | HS-01 → HS-02 → HS-03 | UI chọn NEXT provider; canonical history gửi được sang provider khác |
| Concurrent semantics | HS-03 + MC-03 → HS-04 | Composer #101 và GPT #102 cùng chạy, không nhầm output |
| Retry/recovery | HS-04 → HS-05 → HS-06 | Stop/retry/patch đúng run và provider dù đã switch |
| Compatibility | HS-01/04 + MC-01/02 → HS-07 | Migration và contract tests qua |
| Release | HS-08 → HS-09 | E2E matrix pass, rollback và tài liệu có sẵn |

## 5. Acceptance scenarios

| Mã | Tình huống | Kết quả cần đạt |
|---|---|---|
| HS-AC-01 | Composer #101 running → chọn GPT | #101 vẫn Composer và tiếp tục streaming |
| HS-AC-02 | Gửi #102 khi #101 chưa xong | #102 dùng GPT; không bị chặn bởi global busy flag |
| HS-AC-03 | #102 xong trước #101 | Cả hai output vào đúng run/thread, không overwrite |
| HS-AC-04 | Stop #101 | Không hủy #102 |
| HS-AC-05 | Chọn GPT rồi retry #101 | Retry vẫn Composer theo run gốc |
| HS-AC-06 | GPT thiếu tool/attachment được yêu cầu | Thông báo capability mismatch, không bỏ lặng nội dung |
| HS-AC-07 | Chuyển Composer → GPT → Composer | Context khả dụng được đồng bộ đúng qua canonical history |
| HS-AC-08 | Refresh/reconnect và retry | Đúng provider, run_id, attempt; không duplicate side effects |
| HS-AC-09 | Patch Composer chạy sau khi user chọn GPT | Patch status/results vẫn thuộc #101/Composer |
| HS-AC-10 | Provider timeout/quota/network | Error rõ ràng; fallback chỉ nếu có opt-in |

## 6. Điều kiện phối hợp cho hai coder AI chạy song song

1. Nhóm Hot-Switch chỉ sửa provider adapters/registry/selector/context serializers và các file được nhóm Multi-Conversation đồng ý; không tạo một RunManager thứ hai.
2. Nhóm Multi-Conversation định nghĩa schema/event API; nhóm Hot-Switch ký xác nhận contract trước khi bắt đầu sửa integration points.
3. Không dùng global mutable `currentProvider` cho active request. `selected_next_provider` chỉ là preference của composer cho **Send mới**.
4. Nếu task nhóm 02 cần API nhóm 01 chưa hoàn thành, tạo adapter/interface với mock test, ghi dependency; không đánh dấu integration hoàn tất.
5. Mỗi lần sửa cập nhật checkbox 3 cấp, link commit, test logs và baseline HEAD; không commit trực tiếp vào nhánh đang được nhóm còn lại sửa cùng file nếu chưa điều phối.

### Nhật ký tiến độ

| Ngày | Branch / commit | Milestone | Tests / evidence | Ghi chú |
|---|---|---|---|---|
| 2026-11-20 | Chưa có | Lập kế hoạch | Chưa chạy test | Checklist tạo mới; chưa chỉnh code |
| 2026-11-20 | `main` @ `07fd54a5` (HEAD tiến lên trong lúc hai coder dùng chung branch; HS không stage/commit/push) | HS UI + pinning cấu hình provider | `go test -count=1 ./internal/agentchat` PASS; `go test -race -count=1 ./internal/agentchat` PASS; `go vet ./internal/agentchat` PASS; `go test -timeout 60s ./internal/agentrun` PASS; `node web/agentchat_bootstrap_test.cjs` PASS; `node --test web/agentproviderselector_test.cjs` PASS | Selector/model đã gắn composer, preference local theo conversation; Run cards có Stop/Retry riêng; đường Agent Chat/Patch cũ vẫn dùng khi cùng provider và không active. Snapshot lưu safe prompt policy/retry/automation/binding; public config revisions archive không secret, run pin credential fingerprint. SSE live, timeout 60s/call, retry off/5/unlimited. Run đồng thời text-only. |
| 2026-11-20 | Cùng working tree | #101/#102 backend + UI mock acceptance | Focused `internal/server` tests `TestComposerRunCanOverlapGPTRunRetryAndStopIndependently`, `TestContextSnapshotIncludesActiveRunPromptButExcludesItsPartialOutput`, `TestOwnerRunRetryResolvesPinnedConfigAndRejectsRotatedCredential`, `TestRunPromptUsesPinnedSnapshotAndExecutionWorkspacePack` PASS; UI bootstrap PASS | #101 Composer active → #102 GPT finishes first → retry #101 remains Composer → Stop #101 leaves #102 complete. Snapshot pins parent/excluded run IDs and includes #101 prompt, not partial output. UI exercised with fake API/DOM, not a live browser/provider. Completed outputs feed later snapshots with provenance. |
| 2026-11-20 | Cùng working tree | Chẩn đoán full `internal/server` | `timeout 300s go test -v ./internal/server -count=1` kết thúc 124 khi còn chạy nhóm Remote Workflow; `TestRunOwnedGlobalSlotIsNotAcquiredTwiceByAgentChat` PASS trong log. Các focused #101/#102 và snapshot tests PASS. | Full suite có nhiều test assertion lỗi trong các vùng auth/editor/terminal/Patch ngoài phạm vi HS và vượt 300s; chưa có full-suite PASS. Stack global-slot của lần trước không tái hiện ở lần mới nhất; test riêng và nhóm Agent+slot PASS, nên chưa xác định được run nào từng giữ slot. Không sửa Manager/test của PLAN 01. |
| 2026-11-23 | `review/taskdeck-multiconversation-hotswitch-20261123` @ `c10eb6e4` | Tool continuation payload, Native Patch run cards, execution-pack pinning | `go test ./internal/agentchat` PASS; `go test -race ./internal/agentchat` PASS; `go vet ./internal/agentchat` PASS; `node web/agentchat_bootstrap_test.cjs` PASS (incl. Patch review/approve/continue/uncertain state); `node --test web/agentproviderselector_test.cjs` PASS; focused server tests `TestProviderToolCall*`, `TestRunPrompt*`, `TestAgentContextSnapshotReturnsSafePinnedRunBinding`, `TestComposerRunCanOverlapGPTRunRetryAndStopIndependently` PASS, including `go test -race` after PLAN 01's persisted-history test file landed. Actual second Responses JSON payload asserted for original user → function_call → matching Native Patch output; both Responses and Chat Completions mapping unit tests PASS. | Fixed continuation replay dropping items after the pinned user turn. Snapshots now include a SHA-256 reference to the execution workspace AI Pack at Send; provider execution rejects drift before egress. Provider-specific function-call fragments stream as non-executable delta events, while the canonical tool-call event is buffered until the response succeeds; a stream-error/retry regression confirms it is not duplicated. Run cards read the existing conversation tool ledger and use PLAN 01 decision/continue APIs; they do not execute Patch. No boxes were checked because parent HS acceptance criteria include broader unverified items and live browser/provider acceptance. The temporary server build failure from PLAN 01's newly added persisted-history test cleared after that test was updated; the focused server race suite then passed. No PLAN 01 Manager implementation was edited. |
