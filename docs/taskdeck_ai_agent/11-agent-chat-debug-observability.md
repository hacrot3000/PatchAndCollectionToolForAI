# F04.UI-DEBUG — AI Agent Debug: cấu hình, request/response, streaming, tool execution và console

**Trạng thái:** ĐÃ TRIỂN KHAI MỘT PHẦN — server trace/provider transport capture và panel cơ bản; browser/live acceptance, một số preflight spans và timings chi tiết còn thiếu. Xem evidence handoff bên dưới.
**Ngày:** 2026-11-19. **Branch mục tiêu:** main. **HEAD khảo sát trước triển khai:** bba5eea603f768f7172ecf758d29f01a5a48bc7e.
**Phụ thuộc bố cục:** [10 — Dock panel Right/Bottom và Auto Hide](10-agent-chat-dock-panel-ui.md).  
**Liên quan:** [F02 Providers](02-providers.md), [F03 Sessions](03-sessions-context.md), [F04 UI](04-conversation-ui.md), [F05 Patch feedback](05-patch-feedback.md), [F07 Acceptance](07-acceptance.md).

## 1. Mục tiêu

Người dùng cấu hình model/provider và gọi Agent Chat/Test connection chỉ thấy **FAILED**, không rõ lỗi ở bước nào và không có log. Hãy bổ sung **AI Debug thực sự từ backend tới frontend**, để người dùng tự quan sát:

- Giá trị cấu hình hiệu lực (provider, API type, sanitized endpoint, model, reasoning, stream, capabilities, context mode), kết quả validate, probe và điều kiện bị từ chối.
- Dữ liệu được **đóng gói gửi provider**: instructions, AI Pack, context/history, messages, attachments, function tool schemas, tool outputs, reasoning, max tokens và serialized HTTP request body.
- Dữ liệu **nhận provider**: HTTP status, response headers an toàn, error body (kể cả non-200), JSON/SSE raw frames, parsed response, tool calls, token usage.
- Vòng đời AI local: session/turn/revision, approval, queued native ZIP, Patch Session, selected item/run, command RC/output tail, verified final, function_call_output feedback, retry/timeout/cancel/UNKNOWN.
- **Nguyên nhân thất bại cụ thể** kèm stage, error code, safe provider error message, trace ID, thời gian và gợi ý kiểm tra. Ngay cả khi Debug OFF cũng không được chỉ ghi FAILED.

Đây là observability/diagnostics, **không thêm quyền chạy lệnh, không tự gửi request/retry, không bypass approval, không sửa remote**.

## 2. Hiện trạng mã nguồn cần rà lại trước khi sửa

Tại HEAD khảo sát:

| File | Điểm cần instrument |
| --- | --- |
| web/featuremods/agentchat.js | Header, showAgentError, settings/test/probe/send/continue, console mới |
| docs/taskdeck_ai_agent/10-agent-chat-dock-panel-ui.md | Agent Chat sắp đổi modal thành dock Right/Bottom, Auto Hide riêng, header responsive/More |
| internal/server/agent_chat_providers.go | Save/Delete/Test/Probe, kết quả verified/unsupported/inconclusive; test có thể chỉ trả lỗi tổng quát |
| internal/server/agent_chat_messages.go | Build AI Pack → BuildResponsesRequest → provider call → result; nhiều lỗi bị gộp thành failed/502 |
| internal/agentchat/providers.go | ValidateProvider, validateProviderCall, DoResponses; HTTP non-200 thường chỉ trả status, request transport lỗi chỉ báo generic |
| internal/agentchat/stream.go | DoResponsesStream và SSE accumulator/parser, lỗi thiếu context/raw frames |
| internal/agentchat/responses.go | Serialized request và parsed output |
| internal/server/agent_chat_tools.go, agent_chat_native.go, agent_chat_sync.go | Approval, queue, native PATCH, reconciliation |
| internal/server/agent_chat_tool_feedback.go, agent_chat_auto_feedback.go | Tool output continuation và auto feedback |
| internal/agentchat/store.go | Durable session journal — **không dùng để lưu raw debug body** |
| web/agentchat_bootstrap_test.cjs, internal/server/agent_chat_ui_test.go | Runtime bootstrap và contract tests để giữ tránh TDZ/ReferenceError |

Trước khi thực hiện: fetch origin/main, đọc HEAD thực tế và điều chỉnh theo code mới. Nếu Chat Completions adapter hoặc dock đã code thêm thì debug phải bao phủ chúng; không hardcode chỉ Responses.

## 3. Header / Debug panel theo tài liệu UI số 10

### 3.1. Icon bật/tắt trực tiếp trên title

- Thêm icon/nút **AI Debug** hình bug/terminal trên toolbar/title; **luôn truy cập trực tiếp** dù Right dock hẹp hay Bottom dock; không giấu độc quyền trong menu ⋯.
- OFF/ON rõ qua icon, tooltip, aria-label/aria-pressed, badge “Capturing”. Bấm ON bắt đầu server-side capture và hiện tab **AI Debug**; bấm OFF ngừng capture mới. Không thay đổi trạng thái chạy AI/patch.
- Giữ các control bắt buộc khác từ tài liệu 10: Right↔Bottom, Auto Hide/Pin, More, Hide. Khi chật chỗ, New session/Preview/Rename/Archive/Providers/Refresh có thể ở More.
- **Debug Safe** mặc định khi ON. **Raw payload** phải có xác nhận riêng (mục 6), không bật kèm Debug.
- Đổi vị trí, kéo resize, Auto Hide hay collapse không làm mất/debug reset trace, cancel provider request hoặc tạo chạy lại.
- Khi UI dock chưa triển khai, triển khai backend trước; sau đó gắn icon/tab vào dock được giao ở tài liệu 10, **không coi popup modal cũ là đích cuối**.

### 3.2. Hình dung

~~~text
AI Agent Chat  [Right↔Bottom] [Pin] [🐞 Debug ON] [⋯] [Hide]
[Conversation] [AI Debug]
┌─────────────────────────────────────────────────────────────┐
│ AI Debug · Capturing (safe) [Session ▾] [Clear] [Export]     │
│ [Timeline] [Console] [HTTP] [Payload] [Tools] [Configuration]│
│ 18:20:12.101  config.validated        model=gpt..           │
│ 18:20:12.102  request.built           8,212 bytes SHA=...   │
│ 18:20:12.120  POST /v1/responses     started                │
│ 18:20:12.542  HTTP 400              PROVIDER_HTTP_400       │
│              unsupported reasoning parameter                 │
│            [View Request] [View Response] [Copy]              │
└─────────────────────────────────────────────────────────────┘
~~~

### 3.3. Các tab/chức năng

1. **Timeline:** từng event thời gian chính xác (UTC+local), stage, duration, severity, trace/span ID, trạng thái, outcome. Click mở chi tiết. Có lọc session/provider/trace/type/level, search, clear.
2. **Console:** log dạng terminal đọc được (không dùng terminal backend thật), severity/info/error, copy selected/all, limit entries/bytes, follow-tail **khi đang ở cuối**; user cuộn lên xem log cũ thì giữ vị trí.
3. **HTTP:** mỗi attempt một hàng: POST target endpoint đã sanitize, API type, HTTP status, DNS/TLS/TTFB/elapsed nếu đo được, Content-Type, Retry-After, request ID của provider nếu có, size/hash, safe headers, network error type.
4. **Payload:** Request Build Input, **Serialized Request Body**, **Raw/Redacted Response Body**, Parsed Result, SSE Frames, Tool Args/Tool Result. JSON pretty view và raw byte view phải phân biệt; có Copy, UTF-8/errors, truncation/redaction/sha256/bytes indicator.
5. **Tools:** call ID/name, args, approval/reject, queue package SHA, Patch Session/run/item, command statuses/RC/output tail, final evidence, feedback delivered/unknown; không tạo execute shortcut.
6. **Configuration:** provider name/ID, api_type, sanitized base URL, selected model/reasoning/context mode, capability declared vs verified, stream flag, key present yes/no, model limits/AI Pack size/fingerprint. Nút “Test Connection” và “Probe” sử dụng API thật hiện có và chỉ chạy khi user nhấn, với consent như hiện tại.
7. **Error details:** mã, stage, safe message, HTTP status, provider error code/type nếu có, trace ID để tìm, gợi ý kiểm tra dựa trên evidence; không đoán nếu thiếu log.

Responsive: Right panel hẹp dùng stacked details và horizontal scrolling thay vì cắt nội dung. Bottom panel thấp giữ header/filter, nội dung cuộn riêng. Context menu/Settings không gây auto-collapse khi tương tác. Không render raw HTTP/HTML qua innerHTML.

## 4. Structured error: bắt buộc ngay cả Debug OFF

Hiện DoResponses thường trả “provider returned HTTP 400/401” mà không có provider error body; lỗi transport có thể bị che thành “provider request failed”; handler message trả generic failed/502. Chỉ gắn console frontend sẽ **không giải quyết**.

Bổ sung error taxonomy typed, log đúng điểm lỗi, response của API có cấu trúc:

~~~json
{
  "error": {
    "code": "PROVIDER_HTTP_400",
    "stage": "provider_http",
    "message": "Provider rejected the request",
    "provider_error_type": "invalid_request_error",
    "provider_error_code": "unsupported_parameter",
    "provider_message": "reasoning parameter is not supported",
    "trace_id": "opaque-uuid",
    "http_status": 400,
    "retryable": false,
    "ambiguous": false
  }
}
~~~

Không buộc mọi upstream error phải ánh xạ HTTP nguyên trạng; user-facing status + code phải chính xác, không làm lộ nhạy cảm. Lỗi không có provider body phải ghi “body unavailable”. Không ép retry false nếu transport ambiguous.

Phải phân biệt ít nhất:

| Stage | Mã gợi ý |
| --- | --- |
| Provider config/secret | CONFIG_INVALID, MODEL_NOT_CONFIGURED, MISSING_API_KEY, UNSUPPORTED_API_TYPE |
| Network | DNS_LOOKUP_FAILED, PRIVATE_IP_BLOCKED, TLS_HANDSHAKE_FAILED, CONNECT_TIMEOUT, REQUEST_TIMEOUT, REQUEST_CANCELLED |
| Provider response | PROVIDER_HTTP_400/401/403/404/429/5XX, PROVIDER_UNSUPPORTED_API; Retry-After nếu có |
| Parsing | JSON_PARSE_FAILED, SSE_INVALID_CONTENT_TYPE, SSE_INCOMPLETE, SSE_PARSE_FAILED, TOOL_ARGUMENTS_INVALID |
| Capability/context | CAPABILITY_UNVERIFIED, TOOL_SCHEMA_REJECTED, AI_PACK_BUILD_FAILED, PROJECT_DRIFT, CONTEXT_LIMIT, REQUEST_TOO_LARGE |
| Session/tool | SESSION_REVISION_CONFLICT, APPROVAL_REQUIRED/REJECTED, NATIVE_PATCH_FAILED/UNKNOWN, FEEDBACK_DELIVERY_UNKNOWN |

Taxonomy là **yêu cầu** chứ không phải khẳng định tất cả đã có. Chỉ báo nguyên nhân gắn với chứng cứ thực; không hạ UNKNOWN thành FAILED hoặc PASS.

## 5. Backend phải capture ở đúng layer (không chỉ UI)

### 5.1. Instrument pipeline

1. **UI/API action:** Save provider, Test connection, Probe, Create Session, Send, Continue, Approve/Reject, Sync, Auto-feedback; event bắt đầu/kết thúc có trace ID, không log key.
2. **Config validation:** trường sai, effective config, endpoint sanitization, model/capability, key existence bool.
3. **AI Pack/context:** build begin/end/error, bytes/fingerprint; xác minh context mode/history/compaction, reasoning, tools. Chỉ raw mode cho phép xem free-text.
4. **Payload build:** log logical input và **body bytes thực tế sau serialize**, sha256, length, tool schema, max tokens, history count, whether streaming enabled. Bắt lỗi trước egress.
5. **Transport:** method, sanitized endpoint, safe headers, DNS/TLS/connect errors, HTTP status, safe response headers, error body cho non-2xx trong bounded reader, timings, provider request ID.
6. **JSON / SSE:** raw response bytes/frames trong giới hạn khi capture, parse event/accumulator, completion marker, assembled tool args, parsed result, usage và errors.
7. **Session state:** transition, request ID, turn ID, revision, cancellation source, ambiguous state/feedback status. Không tạo provider retry từ debug.
8. **Native tool:** proposal→approval→ZIP queue→selection→run→commands/results/evidence→function_call_output→provider ACK/unknown; trace liên quan bằng call/session IDs.
9. **Capability probes:** tất cả từng probe text/tool/stream/previous_response/conversation có request/response/error, declared vs verified vs unsupported/inconclusive.

**Bắt buộc Test Connection ghi trace kể cả chưa có Agent Session**; không bắt user gửi tin nhắn thật mới lấy log. Nếu error xảy ra trước HTTP request, timeline phải thể hiện không hề có egress.

### 5.2. Correlation record

~~~json
{
  "seq": 10,
  "trace_id": "random-opaque-id",
  "span_id": "random-opaque-span",
  "parent_span_id": "optional",
  "at": "2026-11-19T11:20:00.123Z",
  "duration_ms": 302,
  "level": "error",
  "stage": "provider_http",
  "event": "http.response",
  "code": "PROVIDER_HTTP_400",
  "provider_id": "custom",
  "model_id": "model-id",
  "session_id": "opaque-or-null",
  "client_request_id": "opaque",
  "tool_call_id": "optional",
  "safe_message": "Provider rejected request",
  "http_status": 400,
  "request_bytes": 9214,
  "response_bytes": 163,
  "request_sha256": "hex",
  "response_sha256": "hex",
  "redacted": true,
  "truncated": false
}
~~~

Timestamps/server sequence phải đơn điệu trong trace; mỗi logical turn có thể có nhiều HTTP attempts (probe/chaining) nhưng không được trộn. Với streaming giữ các frame theo đúng thứ tự. Correlation phải survive GUI hide/reconnect; trace link vào session ID/turn ID hoặc probe/test request ID.

### 5.3. Phân biệt request thực tế với bản nhìn

Giao diện phải hiển thị ba lớp:

- **Build Input** — canonical request config trước serialize.
- **Transport Request** — **JSON bytes sau serialize** mà HTTP client gửi (không phải TLS/TCP packets), method, sanitized endpoint, safe header list.
- **Transport Response** — JSON/error bytes hoặc SSE frames nhận được, và parsed result độc lập.

Khi user xem pretty JSON, bản đã redacted hoặc bị truncate, phải ghi rõ **REDACTED / REFORMATTED / TRUNCATED**; không gọi đó là “exact original”. Có digest/bytes để kiểm chứng, và lý do nếu không capture đủ. Với SSE không giả lập event đã bị mất.

## 6. Raw payload và bảo mật dữ liệu

Người dùng cần xem **mọi dữ liệu đóng gói gửi đi, nhận được**, nhưng nguyên bản có thể chứa AI Pack, prompt, attachment, secret tự do. Thiết kế **3 chế độ**:

1. **Debug OFF:** error_code/stage/safe message/trace ID vẫn có khi FAIL; không capture body hoặc raw events.
2. **Debug ON — Safe mặc định:** bounded event timeline + metadata/hash/timing; request/response view có structured redaction, **ẩn free-text prompt/AI Pack/attachments**, vì regex không đảm bảo lọc hết key trong nội dung tự do. Chỉ hiển thị nội dung safe đã scrub.
3. **Debug ON — RAW BODY (explicit opt-in):** sau xác nhận riêng, cấp owner-scoped grant có TTL ngắn, hiển thị bytes thực sự gửi/nhận (kể cả instructions, AI Pack, messages, tool result) trong **volatile bounded server buffer**. Bản raw có thể chứa thông tin nhạy cảm: có cảnh báo/badge RAW ACTIVE, nút tắt và xóa ngay. **Tuyệt đối không capture Authorization/Cookie/X-API-Key/secret-store values hoặc raw credential headers**, ngay cả khi RAW ON.

Quy tắc bắt buộc:

- Không ghi raw prompt/payload vào session journal, app logs, browser localStorage, analytics, console.log hoặc export mặc định.
- Chỉ cấp quyền xem đúng owner/workspace/project; shared server phải có explicit permission cho debug capture và raw. Backend recheck từng GET body, không tin frontend toggle.
- Không để Safe view rò prompt/AI Pack free-text; raw consent khác external-provider consent.
- Raw chỉ thu từ thời điểm user bật, **không phục hồi dữ liệu quá khứ**; tắt RAW xóa raw buffer best effort. Sau server restart RAW luôn OFF.
- Giới hạn per body, SSE frames, events/trace, total memory/owner, duration và retention; gợi ý: 256 events/trace, 64 trace/owner, 256 KiB mỗi body, max 2 MiB raw/owner, TTL <=30 phút. Nếu giới hạn thấp hơn request thực thì hiển thị **bytes_total, bytes_captured, sha256, TRUNCATED**, không tuyên bố full raw.
- Export mặc định **sanitized**; raw export nếu thật sự cần phải là action riêng với xác nhận quyền và cảnh báo. Không tự gửi bundle debug tới provider.
- Debug ON không tăng số provider calls/chi phí và không gây blocking/uncapped memory growth; khi overload drop debug events với counter, không làm hỏng AI turn.

## 7. API và lưu trữ gợi ý

Có thể đổi tên theo conventions thực tế, nhưng phải giữ hợp đồng:

| API mẫu | Ý nghĩa |
| --- | --- |
| GET /api/agent/debug/state | Owner/workspace-scoped: enabled, safe/raw mode, TTL, limits/stats |
| POST /api/agent/debug/state | Enable/disable safe; explicit RAW grant riêng; OFF dừng capture, clear raw |
| GET /api/agent/debug/traces?since=cursor&limit=N | List summary theo owner, bounded/paginated; không trả body |
| GET /api/agent/debug/traces/{trace_id} | Event timeline + diagnostics meta; sanitized |
| GET /api/agent/debug/traces/{trace_id}/payload?part=request/response/sse&view=safe/raw | Payload scoped + ACL/TTL, no-store; giới hạn bytes |
| POST /api/agent/debug/clear | Xóa scoped traces; không xóa session/native Patch |

- Trace tạo trước provider call và finalize kể cả lỗi/timeout/cancel.
- Backend chủ động capture; **không phụ thuộc browser polling**.
- GET debug là read-only, không chạy native sync/tool side effects.
- Có cursor/reconnect/dedupe, owner/user scope, no-store, max bytes, permission deny test.
- Dùng ring buffer bounded + no-op interface khi OFF. Không nhét raw vào Session.History hay để provider nhận debug payload.
- Nếu giữ safe trace giữa refresh/restart phải có retention rõ; raw memory chỉ sống theo grant/daemon, tuyệt đối không persist ngầm.

## 8. Hướng dẫn implement và phạm vi file

**Không viết lại Agent Chat hay Patch Tool.** Gợi ý:

- internal/agentchat/diagnostics.go: typed errors/events, no-op recorder, redaction, bounded raw payload evidence.
- internal/server/agent_chat_debug.go: capture state/grant, per-owner ring, access/auth, read APIs, disposal.
- internal/agentchat/providers.go + stream.go + responses.go: hooks vào **transport thực** và parser; vẫn giữ DNS pin/TLS/no redirect/size limits.
- internal/server/agent_chat_providers.go: Test Connection/Probe ghi trace chi tiết, không tạo request bí mật.
- internal/server/agent_chat_messages.go: context/payload/provider stages + structured errors, preserving retry/cancel semantics.
- internal/server/agent_chat_tools.go/native.go/sync.go/tool_feedback.go/auto_feedback.go: tool lifecycle, native result, feedback.
- web/featuremods/agentchat.js + dock module theo tài liệu 10: debug header icon, tabs, console, HTTP inspector, payload view, filters, copy/export.
- shared auth/permissions: debug.read/capture/raw hoặc tối thiểu quyền tương đương có chứng minh; các GET payload nhạy cảm phải kiểm tra owner và grant.
- CI/test module: bổ sung tests cho tracing, redaction, request/response errors, DOM bootstrap, dock UI.

**Trình tự bắt buộc:**
- D0: Review HEAD; tái hiện FAILED bằng fake provider 400/401 và chỉ ra root cause trong code.
- D1: Chuyển lỗi provider thành structured error; cải thiện thông báo FAILED **kể cả Debug OFF**.
- D2: Server trace recorder, capture config/turn/test/probe, typed transport timings/errors.
- D3: Bounded serialized HTTP body/response/SSE capture safe/raw, consent/ACL/TTL/eviction.
- D4: Icon Debug và UI tabs tích hợp với dock Right/Bottom/Auto Hide; responsive overflow.
- D5: Tool → Native Patch → feedback trace, không tạo side effects.
- D6: Tests/CI, handoff và documentation.

Nếu dock chưa được code, có thể triển khai D0–D3 độc lập; sau đó phối hợp task giao diện số 10. Không tuyên bố D4 DONE nếu chưa có dock thật.

## 9. Acceptance tests / checklist

### Provider và dữ liệu gốc

- [ ] Invalid URL/model/key không được gửi provider; lỗi chứa field/code/trace.
- [ ] HTTP 400/401/403/404/429/5xx: hiển thị status, safe provider message/code, request-response evidence nếu captured.
- [ ] Phân biệt local quota 429 và upstream 429; Retry-After khi có.
- [ ] DNS failure/private IP reject/TLS error/connect timeout/cancel có cause/stage đúng và không leak credentials.
- [ ] Test Connection và capability probes có trace **không cần session**.
- [ ] AI Pack/context build/preflight failure có trace chứng minh **không gửi**.
- [ ] Serialized request body bằng bytes thực được fake client nhận, sha256/length đúng; raw mode explicit.
- [ ] Non-200 response body thật được capture bounded; large response truncate có thông báo.
- [ ] SSE frames đúng thứ tự, completion/error/last frame; parser malformed/EOF có trace.
- [ ] Tool calls, approval, native queue/run/RC, final evidence, function_call_output gắn cùng correlation IDs.

### UI và lifecycle

- [ ] Icon Debug ON/OFF nằm trên title Right dock và Bottom dock; không biến mất khi narrow.
- [ ] Debug view có Timeline / Console / HTTP / Payload / Tools / Configuration; filter/copy/clear/export.
- [ ] Console auto-follow chỉ khi đang ở cuối; scroll lên giữ vị trí khi log mới đến.
- [ ] Dock Right↔Bottom, Auto Hide, collapse/expand vẫn giữ logs, filters, draft và không cancel.
- [ ] Lỗi thường có stage/code/safe reason kể cả khi Debug OFF.
- [ ] Raw payload cần xác nhận quyền/TTL và bị xóa khi tắt; Safe export không chứa prompt/key.
- [ ] Settings, More menu, file picker, API request khi panel collapsed không gây lỗi race/TDZ.
- [ ] Firefox/Chromium test thực; không chỉ node --check hoặc grep.

### Bảo mật và regression

- [ ] Canary credentials không bao giờ xuất hiện trong UI, backend logs, trace headers, safe export.
- [ ] Safe view không đưa AI Pack/prompt free text; RAW chỉ khi consent riêng.
- [ ] Cross-owner/project isolation; deny unauthenticated or unauthorized raw GET.
- [ ] Bounded memory/TTL/concurrency; tests -race; Debug OFF gần như no-op.
- [ ] Bật/tắt debug không tự gọi AI, không đổi execution/approval/UNKNOWN semantics.
- [ ] Không regression Patch Tool, Smart Terminal, terminal panes và sidebar trái.

### Lệnh test chuẩn

~~~bash
cd vscode_tasks_menu_go
go test -count=1 ./internal/agentchat
go test -race -count=1 ./internal/agentchat
go test -count=1 ./internal/server -run '^TestAgent'
go vet ./internal/agentchat ./internal/server
node --check web/featuremods/agentchat.js
node web/agentchat_bootstrap_test.cjs
node test_ai_smart_terminal.cjs
~~~

Bổ sung tests cho diagnostics/transport/error classification và browser smoke khi khả dụng; update CI workflow. Không dùng credential thật hoặc gọi provider mất phí khi không được phép.

## 10. Cách giao việc và bàn giao

- Fetch origin/main trước khi sửa, bảo toàn commit đồng thời; làm **trực tiếp main, commit nhỏ, push**, không force push.
- Báo lỗi gốc cụ thể của FAILED, file thay đổi và mã lỗi mới, bằng chứng request/response của fake provider, evidence trace xuyên pipeline.
- Báo HEAD trước/sau, commit links, test commands/result, CI; browser/live **NOT VERIFIED** nếu chưa chạy.
- Phân biệt code hoàn thành và chỉ chưa nghiệm thu live. Không đánh dấu DONE nếu chỉ mới có frontend mock.
- Update [README](README.md), [F04](04-conversation-ui.md), [tài liệu dock 10](10-agent-chat-dock-panel-ui.md) và main handoff khi code thực sự triển khai.

**Lệnh giao AI Coder:** “Thực hiện toàn bộ đặc tả này trên HEAD main mới nhất, cùng với UI dock theo tài liệu 10. Bắt buộc thu thông tin chẩn đoán từ backend (không phải console frontend giả), giải thích được FAILED và xem được dữ liệu request/response gốc khi chủ động bật RAW; viết test, commit và push main. Không dừng ở việc lập prompt hoặc kế hoạch.”


## 2026-11-23 — 600-second provider timeout and HTTP phase diagnostics

TaskDeck's Responses/Chat Completions request transport previously used a fixed 30-second `ResponseHeaderTimeout` even though the model could spend longer reasoning before sending a response. The retry coordinator and HTTP client also enforced 60 seconds per provider attempt. Those limits generated misleading `CONNECT_TIMEOUT` errors for providers that had already accepted a large request but had not returned headers.

- Provider request deadline, overall HTTP client timeout and response-header timeout are now **600 seconds per attempt**. Short DNS/TCP dial (8s per address) and TLS handshake (10s) protection remains; Stop/cancellation and retry policy remain authoritative. Unlimited retries are not made automatic merely by this change.
- HTTP trace diagnostics report `dns_ms`, `tcp_ms`, `tls_ms`, `upload_ms`, `header_wait_ms` and `ttfb_ms` where measurable. Incomplete phases can report elapsed so far; missing/zero phase metrics are not proof of an instantaneous phase. No IP addresses, raw headers, prompts or credentials are included in these new fields.
- Classify HTTP header delays as `RESPONSE_HEADER_TIMEOUT`, overall timeouts as `REQUEST_TIMEOUT`, dial timeouts as `DIAL_TIMEOUT`, and distinguish TLS/DNS timeouts from other failures. These are diagnostics, **not** proof that the remote provider did not process a request.
- The Responses SSE adapter now sets `stream:true` in its actual request body (in addition to the Accept header), without modifying the durable canonical request or the verified-streaming capability requirement. Never auto-enable an unverified SSE endpoint. Tests cover the streaming JSON flag, hardened network transport, 600-second limits, phase breakdown and error classification.
- **Security decision:** Leave outbound HTTP proxies disabled, with pinned public or explicitly trusted CGNAT IPs, certificate checks and redirect protection. Do not silently enable arbitrary environment proxies to imitate an editor's behavior. Keep-alives remain disabled in the per-call short-lived client; enabling them without an owner-/provider-scoped connection pool does not improve reuse and could weaken DNS revalidation.
