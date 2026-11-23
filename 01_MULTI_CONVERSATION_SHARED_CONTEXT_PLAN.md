# TASKDECK — KẾ HOẠCH 01: MULTI-CONVERSATION, SHARED CONTEXT & AGENT COORDINATION

> **Loại:** Implementation plan + feature checklist (3 cấp)  
> **Ngày lập:** 2026-11-20  
> **Cập nhật:** 2026-11-20 — Run lifecycle, DAG core, immutable snapshots, detached worktree binding và owner-bound Provider Router invocation đã có foundation; Agent Patch continuation, manifest orchestration, UI và coordination vẫn pending.  
> **Trạng thái:** IN PROGRESS (thay đổi cục bộ chưa commit; theo yêu cầu không stage/commit/push).  
> **Phạm vi:** Nhiều conversation chạy song song; context độc lập/chung; coordination giữa agent; workspace an toàn.  
> **Không bao gồm:** UI/logic chuyển provider trong một conversation (xem kế hoạch 02).  
> **Đề xuất vị trí trong repo:** `docs/ai-agent/plans/01_MULTI_CONVERSATION_SHARED_CONTEXT_PLAN.md` (chưa commit vào Git).  

**Tiến độ lúc lập kế hoạch:** 0/139. Hiện có implementation foundation chưa commit; các checkbox vẫn **[ ]** cho tới khi từng subtask đủ test, tích hợp end-to-end và evidence theo tiêu chí dưới đây.

## 1. Kỳ vọng và tình huống bắt buộc

1. Task lớn A có 3 conversations C1/C2/C3 chạy đồng thời trên các agent có thể giống hoặc khác nhau.
2. Cả ba chia sẻ instructions/context cấp nhóm nhưng giữ prompt, history, attachments, stream, stop và retry riêng.
3. C2 biết C1 đang thiết kế method A2; dùng được chữ ký dự kiến như một **contract**, nhưng **không** coi implementation đã có trong workspace đến khi verified/integrated.
4. Chuyển tab, reload UI, lỗi một conversation không phá run khác; kết quả out-of-order vẫn gắn đúng request.
5. Có hai chế độ workspace tùy chọn: **Shared Working Directory** (phát hiện conflict) và **Isolated Git Worktree** (mặc định nên chọn khi nhiều agent cùng chỉnh code).

## 2. Phạm vi sở hữu và hợp đồng với kế hoạch 02

| Thành phần | Kế hoạch 01 sở hữu | Kế hoạch 02 sở hữu |
|---|---|---|
| Conversation/Context Group/Peer coordination | Toàn bộ | Chỉ tích hợp |
| Run scheduling, persistence, stream routing, stop/retry lifecycle | API và orchestration | Provider-specific execution/cancel/retry binding |
| `provider_id`, `model`, `provider_config_snapshot` trong run | Persist, truyền nguyên trạng | Chọn, xác thực và snapshot lúc Send |
| Shared Context Snapshot | Version, scope, manifest | Serialize sang format provider |
| UI conversation list, tab, group | Toàn bộ | Provider selector và provider badges |
| Workspace/Git/Patch binding | Chủ trì | Tích hợp đúng run/provider |

**Hợp đồng tối thiểu cần khóa trước khi hai coder cùng triển khai:**

```json
{
  "conversation_id": "conv-123",
  "run_id": "run-102",
  "parent_turn_id": "turn-101",
  "context_snapshot_id": "ctx-102",
  "provider_id": "gpt",
  "provider_config_version": "v1",
  "attempt": 1,
  "idempotency_key": "request-uuid"
}
```

Quy tắc: mọi streaming/tool/patch event phải mang `conversation_id` và `run_id`. Dữ liệu provider trên run là bất biến sau khi Send. Việc lấy context mới từ peer chỉ làm thay đổi **snapshot mới**, không tự sửa snapshot của run cũ.

## 2A. ĐẶC TẢ TRIỂN KHAI BẮT BUỘC (áp dụng cho toàn bộ checklist MC)

> **Lưu ý về code thực tế:** Đây là thiết kế mục tiêu, **không khẳng định** đã tồn tại các package, bảng DB hay endpoint tương ứng trong TaskDeck. Coder bắt buộc đọc HEAD mới nhất, dùng lại mô-đun hiện có, lập mapping `khái niệm → file/hàm thực tế` trước khi bắt đầu. Ưu tiên thay đổi tăng dần, không thay toàn bộ AI Agent đang chạy.

### A. Hướng dẫn giao việc trực tiếp cho coder AI

```text
Bạn là coder AI phụ trách FEATURE MC — Multi-Conversation, Shared Context & Coordination của TaskDeck.
- Đọc hết file plan này và đối chiếu source/HEAD hiện tại trước khi sửa bất kỳ file nào.
- THỰC THI code + test, không chỉ soạn prompt hoặc phân tích. Làm theo thứ tự phụ thuộc MC-00 → MC-10.
- Giữ nguyên AI Agent, Patch Tool, AI Retry, History, Debug Console, Attach to Chat và chế độ một conversation.
- Chịu trách nhiệm Conversation/Run Manager, persistence, shared group/snapshot, coordination/workspace.
- KHÔNG tự tạo ProviderRouter/Hot-Switch thứ hai: dùng interface chung với coder HS.
- Cập nhật ngay checkbox từng MC-* khi hoàn thành có bằng chứng; không tự tick khi chưa chạy test.
- Chạy build/test phù hợp với repo; mỗi batch có commit rõ ràng, đồng bộ remote theo quyền và quy trình project.
- Cuối mỗi batch báo: HEAD trước/sau, files sửa, tasks done, test PASS/FAIL/BLOCKED, lỗi còn lại, dependency với HS.
- Nếu chưa có adapter/provider để integration: dùng fake provider, test contract, giữ task tích hợp là [ ].
```

### B. Dữ liệu tối thiểu và migration (đặt tên theo convention sẵn có)

| Đối tượng | Trường tối thiểu | Quy tắc |
|---|---|---|
| `context_groups` | `id, project_id, title, mode, shared_revision, created_at` | Một nhóm có nhiều conversation; revision tăng khi update |
| `conversations` | `id, project_id, group_id?, fork_source_id?, fork_turn_id?, title, archived_at, updated_at` | Không trộn local history giữa các conversation |
| `conversation_messages` | `id, conversation_id, parent_turn_id?, origin_run_id?, role, body_ref, sequence, created_at` | Transcript xác định bằng quan hệ, không chỉ timestamp |
| `agent_runs` | `id, conversation_id, parent_turn_id?, context_snapshot_id, provider_id, model, provider_config_version, state, idempotency_key, created_at` | Provider binding bất biến, HS được quyền chọn |
| `run_attempts` | `id, run_id, attempt_no, operation_id?, status, started_at, finished_at?` | Retry là attempt mới; side-effect cần ledger |
| `context_snapshots` | `id, group_revision?, manifest_json, payload_hash, created_at` | Immutable, có provenance và source versions |
| `run_events` | `event_id, run_id, conversation_id, attempt_id?, seq, type, payload_json, occurred_at` | `(run_id, seq)` unique; có cursor replay |
| `coordination_events` | `id, group_id, source_conversation_id, target_conversation_id?, kind, correlation_id, payload, created_at` | Append-only, giới hạn peer request |
| `contracts/dependencies` | `owner, consumer, contract_key, revision, status, artifact_ref?, integrated_ref?, test_evidence?` | Không coi PLANNED là code đã tồn tại |
| `workspace_bindings` | `conversation_id, workspace_mode, worktree_path?, branch_ref?, base_sha, head_sha?` | Đường dẫn thật chỉ được quản lý server-side |

Migration cần chạy lặp không lỗi, backup dữ liệu History trước, có đường rollback giao diện không phá schema. Chỉ lưu metadata/file references đối với attachments lớn; không đính nguyên secrets/token vào event log. Dùng FK, index và transaction phù hợp với engine storage thực tế; tên bảng trên chỉ là đặc tả logic.

### C. Hợp đồng Run/Event dùng chung với coder HS

Mỗi `POST Send` tạo request có ID bất biến. Mẫu minh họa (tên trường chính xác chốt ở MC-01.02 + HS-00.02):

```json
{
  "project_id": "project-1",
  "conversation_id": "conv-2",
  "parent_turn_id": "turn-9",
  "group_context_revision": 4,
  "provider_id": "gpt",
  "model": "configured-model",
  "provider_config_version": "cfg-8",
  "context_snapshot_id": "ctx-26",
  "workspace_binding_id": "ws-2",
  "idempotency_key": "client-request-uuid",
  "message": {"role": "user", "content": "Hãy triển khai task A2"}
}
```

Response tạo mới có `run_id`, trạng thái `queued|running`, `context_snapshot_id` và `event_cursor`. `run_id` được ghi trước khi bắt đầu provider invocation. Event envelope:

```json
{
  "event_id": "evt-uuid",
  "project_id": "project-1",
  "conversation_id": "conv-2",
  "run_id": "run-26",
  "attempt_id": "run-26-a1",
  "seq": 37,
  "type": "assistant.delta",
  "operation_id": null,
  "payload": {"text": "..."},
  "occurred_at": "2026-11-20T08:00:00Z"
}
```

**Quy tắc không được phá:** `seq` tăng theo mỗi run; stream reconnect chỉ nhận lại event (không gửi thêm provider request); idempotency `Send` trả về run cũ; worker restart không tự chạy lại tool/patch khi trạng thái side effect chưa xác định. Retry tách `run_id` gốc và `attempt_id`; đổi provider sang model khác phải tạo child run mới theo HS.

### D. API bề mặt đề xuất (route thực tế tùy convention repo)

| Operation | Giao diện đề xuất | Mục đích / kết quả |
|---|---|---|
| List/create | `GET/POST /api/ai/conversations` | Scope project, list có filter/group, create trả stable ID |
| Fork/update/archive | `POST /api/ai/conversations/{id}/fork`, `PATCH/DELETE .../{id}` | Fork tại turn cụ thể, giữ prefix bất biến |
| Context group | `GET/POST/PATCH /api/ai/context-groups` | Shared instructions có revision/CAS |
| Send | `POST /api/ai/runs` | Lưu run + snapshot, trả `run_id` ngay, chống double submit |
| State/retry/stop | `GET /api/ai/runs/{id}`, `POST .../{id}/retry`, `POST .../{id}/stop` | Scope run; không dừng conversation khác |
| Event stream | `GET /api/ai/runs/{id}/events?after={cursor}` | SSE/WS theo stack hiện có, replay có cursor |
| Peer coordination | `POST /api/ai/groups/{id}/peer-requests`, `GET .../events` | Ask/answer/progress/contracts, luôn kiểm tra group membership |
| Artifacts/dependencies | `POST/GET /api/ai/groups/{id}/contracts`, `.../dependencies` | Contract registry và integration evidence |

**Đây là route minh họa.** Coder được chọn route khác nếu repo đã có API tương đương và ghi mapping trong ADR; không được song song dựng hai API trùng công dụng.

### E. State machine và quyết định concurrency

```text
QUEUED --worker acquired--> RUNNING
RUNNING --> WAITING_TOOL --> RUNNING
RUNNING --> WAITING_PEER --> RUNNING
RUNNING/WAITING_* --> COMPLETED | FAILED | STOPPED | INTERRUPTED
INTERRUPTED --explicit retry--> new attempt (not a silent re-execute)
```

- Mặc định `send-now` cho các run độc lập nếu tài nguyên đủ; khi hết quota thì `QUEUED` và giữ nguyên provider/context snapshot lúc Send. `STOPPED/COMPLETED/FAILED` là trạng thái terminal, không tái RUNNING trên cùng attempt.
- Mỗi run dùng cancellation token, retry timer, stream, patch operation, resource accounting **riêng**; trạng thái UI chỉ là view từ backend. Race cần kiểm bằng fake clock/fake provider + race detector khi áp dụng được.
- Fork từ cùng task A phải pin **shared group revision** + ancestor prefix; các prompt cuối có thể giống nhau nhưng không bị đồng nhất thành một message.
- Khi C1 đang RUNNING và C2 cần method A2, C2 chỉ nhận contract được publish và status; peer communication đi qua queue/checkpoint (không cố sửa provider prompt đang chạy).

### F. Coordination protocol & contract A2

```json
{
  "kind": "CONTRACT_PUBLISHED",
  "group_id": "group-A",
  "source_conversation_id": "conv-1",
  "contract": {
    "key": "BikeService.authenticate",
    "revision": 1,
    "signature": "authenticate(mac: String): AuthResult",
    "owner_task": "A1",
    "status": "PLANNED",
    "file_paths": ["example/path/BikeService.kt"]
  }
}
```

1. C1 publish `PLANNED` (chữ ký/chức năng đã thống nhất), C2 có thể viết code gọi dựa trên contract nhưng hiện UI ghi `Dependency pending`.
2. C1 có commit/path và artifact checksum: chuyển `IMPLEMENTED` (có bằng chứng code ở worktree C1).
3. Artifact được đưa vào workspace C2 sau review/apply/merge và xác định đúng ref: chuyển `INTEGRATED`.
4. Build/test liên quan chạy trên workspace C2, lưu command/result/ref: chuyển `VERIFIED`. Nếu contract thay đổi breaking, hạ trạng thái phụ thuộc về `NEEDS_REVIEW` (hoặc trạng thái tương đương theo thiết kế đã duyệt).
5. `ask_peer` có correlation ID, TTL/hop count, timeout và Inbox; peer chưa trả lời không được giả định đã đồng ý.

### G. Chính sách Git/workspace và integration

- **Isolated Worktree (mặc định khi nhiều agent sửa code):** mỗi conversation có worktree/branch riêng, không đổi branch ở working directory gốc; record base SHA và head SHA. Thay đổi chỉ sang C2 qua review/apply/cherry-pick/merge có evidence.
- **Shared Directory (chọn thủ công):** trước write/patch phải kiểm tra file hash/lease, serialize operation chạm cùng file; khi conflict báo rõ đường dẫn và diff, không `force overwrite`.
- Cả hai mode phải hỗ trợ agent đọc artifact/contract của nhau qua registry; chưa integrate thì không được báo compile đã thành công.
- Dọn worktree chỉ khi chắc chắn không còn thay đổi chưa commit; luôn giữ khả năng phục hồi. Không cho peer viết nhầm sang project/workspace khác.

### H. Quy tắc nghiệm thu và báo cáo của coder

- **Một subtask chỉ được tick `[x]` khi có:** code/ADR cụ thể, automated test hoặc manual test xác minh đúng yêu cầu, và commit/PR ref. Build PASS đơn lẻ không thay cho test logic.
- **Sau mỗi nhóm MC-XX:** chạy regression theo baseline; xem lại code cũ, không tạo server worker hoặc table thứ hai cùng chức năng; cập nhật bảng `Nhật ký tiến độ` cuối file.
- **Nếu `BLOCKED` bởi HS:** giữ `[ ]`, ghi `BLOCKED: HS-..`, cung cấp fake adapter + contract test; không bỏ qua tích hợp rồi tự đánh dấu DONE.
- **Merge/push:** `fetch` HEAD mới, kiểm tra dirty state, không reset/force-push/xóa dữ liệu người khác; chia commit nhỏ và push trên branch được giao theo chính sách hiện hành.

## 3. Feature checklist — ba cấp task/subtask

- **Cấp 1:** Milestone/nhóm chức năng (`MC-00`).
- **Cấp 2:** Task có deliverable cụ thể (`MC-00.01`).
- **Cấp 3:** Subtask kiểm chứng được (`MC-00.01.01`).
- **[ ]** chưa hoàn tất; **[x]** đã thực hiện và có bằng chứng; không đánh dấu hoàn tất chỉ vì đã viết code mà chưa đạt tiêu chí tương ứng. Nếu blocker, thêm ghi chú `BLOCKED: ...` tại đúng task, giữ checkbox chưa đánh dấu.

- [ ] **MC-00 — Khảo sát hiện trạng và chốt ranh giới triển khai**
  - [ ] **MC-00.01 — Khảo sát vòng đời AI Agent đang có**
    - [ ] **MC-00.01.01** Xác định nơi lưu conversations/messages/History, cách mở và khôi phục chat.  
      **Cách triển khai:** Trace toàn bộ read/write path cho conversation, message, History trong backend, frontend, file/DB. Tạo bảng `thành phần → hàm → store → migration` với đường dẫn và dòng mã thật.  
      **Kiểm chứng / đầu ra:** Đính kèm bảng inventory; tạo/mở lại chat đơn vẫn giữ đúng nội dung và thứ tự.
    - [ ] **MC-00.01.02** Vẽ luồng gửi request → provider → stream → tool/patch → response → retry/stop.  
      **Cách triển khai:** Dựng sequence diagram từ click Send, API HTTP/WS/SSE, worker, adapter, tool/patch tới streaming cuối cùng; ghi đủ retry, stop, timeout và trường hợp frontend disconnect.  
      **Kiểm chứng / đầu ra:** Diagram chứa tên hàm thực tế và xác nhận với tối thiểu 1 trace chạy thật.
    - [ ] **MC-00.01.03** Liệt kê state toàn cục dễ gây ghi đè giữa các conversation: current chat, loading, provider, abort, patch session.  
      **Cách triển khai:** Tìm biến singleton/global ở server lẫn JS (activeConversation, running, abort, patchSession, retry timer...). Đánh dấu từng biến phải chuyển thành map keyed bởi conversation_id/run_id.  
      **Kiểm chứng / đầu ra:** Có checklist từng biến, vị trí call-site và test failure/race dự kiến.
  - [ ] **MC-00.02 — Khảo sát các tích hợp và hợp đồng dùng chung**
    - [ ] **MC-00.02.01** Kiểm kê AI Pack, file/selection/terminal Attach to Chat, Debug Console, Saved History, Patch Tool, Automation AI Retry.  
      **Cách triển khai:** Lập ma trận điểm chạm cho AI Pack, Attach to Chat từ file/tab/selection/terminal, Debug panel, History, Patch Tool, AI Retry/patch retry; xác định owner của state mỗi tính năng.  
      **Kiểm chứng / đầu ra:** Ma trận có đường dẫn code, dữ liệu đầu vào/ra và regression case cho từng tích hợp.
    - [ ] **MC-00.02.02** Chốt interface RunManager ↔ Provider Router với nhóm triển khai Hot-Switch trước khi sửa shared code.  
      **Cách triển khai:** Tạo bản hợp đồng DTO RunRequest/RunRecord/RunEvent, lifecycle callbacks và ProviderExecutor mà hai coder thống nhất; public interface phải nằm ở module chung, không tạo fork song song.  
      **Kiểm chứng / đầu ra:** Hai phía chạy được contract test đơn giản với fake provider, schema version được ghi trong ADR.
    - [ ] **MC-00.02.03** Ghi quyết định migration, feature flag, backward compatibility và phân quyền sửa file giữa hai nhóm.  
      **Cách triển khai:** Chốt feature flags độc lập, đường lùi migration, nhánh/worktree coder 01 và vùng file ownership; quy định cách resolve conflict và thời điểm tích hợp.  
      **Kiểm chứng / đầu ra:** Có file ownership/merge protocol trong repo, không có hai implementation cạnh tranh cùng một RunManager.
  - [ ] **MC-00.03 — Ghi nhận baseline**
    - [ ] **MC-00.03.01** Chạy và lưu kết quả unit/build/lint/test hiện tại; đánh dấu lỗi tồn tại trước thay đổi.  
      **Cách triển khai:** Ghi HEAD SHA, phiên bản Go/frontend, lệnh build/test lint đúng dự án sau khi đọc README; lưu exit code và lỗi baseline trước thay đổi.  
      **Kiểm chứng / đầu ra:** Evidence có command, thời điểm, exit code và phân biệt baseline fail với regression mới.
    - [ ] **MC-00.03.02** Chụp/ghi các tình huống GUI: conversation đơn, refresh, retry, patch completed/failed/running.  
      **Cách triển khai:** Chạy manual baseline 1 chat: send, stop, History restore, reload, patch complete/fail/running; ghi video/screenshot/log khi thực tế khả dụng.  
      **Kiểm chứng / đầu ra:** Checklist baseline tái chạy được, có expected vs actual.
    - [ ] **MC-00.03.03** Lập bảng mã file/function thật sau khi đọc HEAD; không dùng tên module trong plan như bằng chứng code đang tồn tại.  
      **Cách triển khai:** Tổng hợp file map (router, handler, storage, frontend store/view, patch worker, provider), commit ref; không đoán tên file nếu chưa kiểm tra HEAD.  
      **Kiểm chứng / đầu ra:** Map dẫn tới file tồn tại và đã được người thực hiện kiểm tra.

- [ ] **MC-01 — Thiết kế mô hình dữ liệu và hợp đồng API**
  - [ ] **MC-01.01 — Định nghĩa định danh và quan hệ**
    - [ ] **MC-01.01.01** Thiết kế Project → ContextGroup → Conversation → Message/Turn → Run, dùng ID ổn định.  
      **Cách triển khai:** Thiết kế quan hệ project_id → group_id → conversation_id → messages/turns → runs; UUID/ULID hoặc ID engine hiện tại, FK/index và isolation theo project.  
      **Kiểm chứng / đầu ra:** ERD + migration dự kiến; hai project có ID khác nhau không nhìn thấy nhau.
    - [ ] **MC-01.01.02** Định nghĩa parent_turn_id, origin_run_id, sequence/revision, run attempt và idempotency_key.  
      **Cách triển khai:** Định nghĩa created_at, ordinal, parent_turn_id, fork_from_turn_id, originating_run_id, attempt_id, idempotency_key và optimistic revision; thứ tự logic không lấy theo lúc token kết thúc.  
      **Kiểm chứng / đầu ra:** Test hai response trả về đảo thứ tự vẫn render dưới prompt gốc.
    - [ ] **MC-01.01.03** Phân biệt context shared, local, coordination và workspace binding trong schema.  
      **Cách triển khai:** Phân lớp global instructions, group manifest, conversation local, coordination digest, workspace binding; mỗi lớp có owner, revision, visibility và policy tải vào snapshot.  
      **Kiểm chứng / đầu ra:** Có bảng field source và negative test chống rò context giữa independent/group.
  - [ ] **MC-01.02 — Định nghĩa trạng thái và sự kiện**
    - [ ] **MC-01.02.01** Thiết kế state machine run: queued/running/waiting_tool/waiting_peer/completed/failed/stopped/interrupted.  
      **Cách triển khai:** Vẽ state transition có điều kiện: QUEUED→RUNNING→WAITING_TOOL/WAITING_PEER→COMPLETED/FAILED/STOPPED/INTERRUPTED; CAS terminal states bất biến.  
      **Kiểm chứng / đầu ra:** Table-driven tests từ chối transition sai như STOPPED→RUNNING hay COMPLETED→FAILED.
    - [ ] **MC-01.02.02** Thiết kế event envelope {event_id, project_id, conversation_id, run_id, sequence, type, payload, timestamp}.  
      **Cách triển khai:** Chuẩn hoá event_id, project/conversation/run/attempt IDs, monotonic per-run sequence, event_type, payload schema_version, occurred_at; lưu event trước khi publish nếu cần replay.  
      **Kiểm chứng / đầu ra:** Replay từ cursor không nhân đôi chunk, mất/chệch thứ tự xử lý theo run.
    - [ ] **MC-01.02.03 — PARTIAL** Chốt contract ProviderExecutionSpec và immutable provider_id/config_snapshot do Hot-Switch sở hữu.  
      **Cách triển khai:** Đồng bộ với kế hoạch HS: ProviderExecutionSpec chứa provider_id/model/config_version và snapshot reference bất biến; RunManager lưu spec, ProviderRouter chỉ thực thi.  
      **Kiểm chứng / đầu ra:** Contract fixture có pass/fail cho invalid provider, retry không đổi provider.  
      **Implementation note (partial, uncommitted):** Dùng `agentchat.RunSpec` do PLAN 02 cung cấp; `agentrun.Manager` lưu binding immutable; `Server.AgentRunExecutor` là injection point. Provider adapter nhận principal owner qua context và có thể lấy workspace/snapshot qua `ResolveAgentRunInputs`/`AgentRunWorkspaceResolver`. Run API now lazily wires `NewAgentRunProviderExecutor(owner)` when no test/custom executor is injected; PLAN 02 should use the supplied workspace resolver and `BuildAgentRunWorkspacePack` in payload building. Agent Patch continuation/UI remain open, so checkbox chưa hoàn tất.
  - [ ] **MC-01.03 — Thiết kế API quản lý**
    - [ ] **MC-01.03.01** Đặc tả CRUD conversation/context group, fork, attach/detach group, rename, archive/delete và list/filter.  
      **Cách triển khai:** Định nghĩa REST/WS handlers create/list/update/archive/delete/fork/move cho group và conversation; filter by project/status, phân trang và idempotency cho create.  
      **Kiểm chứng / đầu ra:** OpenAPI/route table và tests 201/400/403/404/409; legacy chat vẫn liệt kê.
    - [ ] **MC-01.03.02** Đặc tả send/stop/retry/status/stream theo run_id; xử lý request trùng bằng idempotency key.  
      **Cách triển khai:** Send trả run_id ngay, status/read/stop/retry/subscribe theo run_id; retry tạo attempt mới, Send trùng key trả cùng run; API có explicit cursor replay.  
      **Kiểm chứng / đầu ra:** Test double-click và HTTP retry không chạy provider lần hai.
    - [ ] **MC-01.03.03** Đặc tả ACL/boundary theo project và phạm vi chia sẻ; không truyền nhầm dữ liệu nhóm khác.  
      **Cách triển khai:** Enforce project scope tại service/storage, kiểm tra membership khi truy xuất group/peer/artifact; phân biệt read shared vs write; tránh dùng project id từ client làm sự thật duy nhất.  
      **Kiểm chứng / đầu ra:** Cross-project read/ask_peer bị 403; audit event không lộ secret.

- [ ] **MC-02 — Persistence, migration và recovery**
  - [ ] **MC-02.01 — Tạo storage bền vững**
    - [ ] **MC-02.01.01** Lưu conversation, message, run, run_attempt, context_group, membership và event checkpoint.  
      **Cách triển khai:** Thêm migration idempotent cho tables (groups, membership, conversations, messages, runs, run_attempts, snapshots, coordination_events); foreign key/index và versioning tương thích DB thật.  
      **Kiểm chứng / đầu ra:** Run migration 2 lần không lỗi; transaction rollback không để partial schema.
    - [ ] **MC-02.01.02** Lưu stream chunks/artifact references theo chính sách retention; đặt index theo group, conversation, run, time.  
      **Cách triển khai:** Quy định lưu chunk raw hoặc aggregate, attachments bằng reference/checksum thay vì nhân bản blob; tạo compound index (conversation_id,created_at), (run_id,sequence), (group_id,revision).  
      **Kiểm chứng / đầu ra:** Benchmark list/history/replay với dataset nhiều run; retention không xóa dữ liệu cần recovery.
    - [ ] **MC-02.01.03** Đảm bảo update nguyên tử/optimistic version khi nhiều worker ghi vào cùng conversation.  
      **Cách triển khai:** Sử dụng transaction/CAS trên message version, run terminal state, sequence allocator và idempotency key; tránh giữ global mutex khi gọi provider/tool lâu.  
      **Kiểm chứng / đầu ra:** Race test -race hoặc tương đương: hai writer không mất event, không duplicate run.
  - [ ] **MC-02.02 — Migration History cũ**
    - [ ] **MC-02.02.01** Chuyển conversation đơn hiện có thành conversation độc lập mặc định, giữ nguyên nội dung và thứ tự.  
      **Cách triển khai:** Viết migration đọc History legacy, map thành independent conversation; giữ message IDs/order nếu được, lưu legacy_id và migration version; tính năng dùng storage mới khi flag on.  
      **Kiểm chứng / đầu ra:** So sánh hash nội dung và số lượng trước/sau cho fixture lịch sử cũ.
    - [ ] **MC-02.02.02** Không sửa/xóa file History cũ trước khi backup và test migration/rollback.  
      **Cách triển khai:** Backup trên đĩa có thể phục hồi; migration có dry-run, resume marker, atomic swap nếu phù hợp; document downgrade khi schema mới vẫn tồn tại.  
      **Kiểm chứng / đầu ra:** Chạy rollback rehearsal trên bản sao dữ liệu, không mất history gốc.
    - [ ] **MC-02.02.03** Bảo đảm opening old chat, search, copy/export vẫn hoạt động sau nâng cấp.  
      **Cách triển khai:** Kiểm tra UI truy vấn legacy và migrated history, search, copy, export; đảm bảo không sinh bản ghi kép khi migrate lại.  
      **Kiểm chứng / đầu ra:** Golden test và manual test trên dữ liệu nhiều vòng chat dài.
  - [ ] **MC-02.03 — Khôi phục khi mất kết nối hoặc restart**
    - [ ] **MC-02.03.01** Từ DB khôi phục danh sách conversation và trạng thái cuối cùng mà không chạy lặp tool/patch.  
      **Cách triển khai:** Khi server restart, rebuild run view từ DB/event log, truy vấn native patch status dựa operation_id trước khi báo interrupted; tuyệt đối không auto re-execute side effect không rõ trạng thái.  
      **Kiểm chứng / đầu ra:** Chaos test restart giữa patch: không tạo bản patch thứ hai.
    - [ ] **MC-02.03.02** Đánh dấu run mất worker là interrupted; xác định rõ được resume/retry hay chỉ báo lỗi.  
      **Cách triển khai:** Phân loại worker_lost, provider session resumable, tool outcome unknown; trạng thái INTERRUPTED với action Resume Stream/Retry New Attempt/Inspect patch evidence.  
      **Kiểm chứng / đầu ra:** UI không báo Completed giả; operator có được last known state + cause.
    - [ ] **MC-02.03.03** Áp dụng event replay/cursor từ last_event_id để client reload không thiếu hay nhân đôi message.  
      **Cách triển khai:** SSE/WS dùng last_event_id hoặc per-run cursor, dedup theo event_id, phân biệt persisted snapshot/ephemeral delta; xử lý gap bằng refetch.  
      **Kiểm chứng / đầu ra:** Test disconnect 30s rồi reconnect: không thiếu/mất/lặp messages.

- [ ] **MC-03 — Concurrent Run Manager và stream isolation**
  - [ ] **MC-03.01 — Chạy song song độc lập**
    - [ ] **MC-03.01.01** Mỗi run có cancellation context, worker/queue, timeout, retry budget và resource tracking riêng.  
      **Cách triển khai:** Mỗi run có context.WithCancel riêng, cancellation registry keyed run_id, timeout cấp provider/tool, timer retry per run; worker persist state trước async launch.  
      **Kiểm chứng / đầu ra:** Stop run A không cancel run B; memory leak test sau nhiều run.
    - [ ] **MC-03.01.02 — PARTIAL** Cho phép nhiều conversation chạy cùng lúc; đặt concurrency limit theo project/provider và backpressure.  
      **Cách triển khai:** Scheduler token bucket/semaphore per provider/project với configurable caps, priority FIFO và queue visibility; tách waiting_peer khỏi worker slot nếu thích hợp.  
      **Kiểm chứng / đầu ra:** Chạy 3 run và cap=2: 2 running + 1 queued, không starvation.  
      **Implementation note (partial, uncommitted):** `agentrun.Manager` dùng semaphore process-global cap=5; Agent Chat per-owner cap tăng từ 2 lên 5; `AcquireGlobalSlotContext` gắn lease không forge được và `GlobalSlotHeld` ngăn nested Agent Chat request chiếm slot lần hai; lease được giữ qua `waiting_tool` tới continuation/Stop. Tests `TestThreeConversationsInOneProjectCanRunTogether`, `TestRunOwnedGlobalSlotIsNotAcquiredTwiceByAgentChat`, `TestWaitingToolRetainsGlobalSlotUntilToolResultContinuation` và `TestGlobalConcurrencyNeverExceedsFiveAcrossManagers` PASS. Found/fixed test fixture leak: `TestAgentAdmissionAdmitsWaiterAfterRelease` previously left its second held token unreleased. Chưa có project/provider sub-caps, configurable queue policy hay UI queued state.
    - [ ] **MC-03.01.03** Hỗ trợ nhiều run cùng conversation khi người dùng gửi request mới; pin parent_turn và snapshot để tránh race.  
      **Cách triển khai:** Khi same-conversation send-now, chốt parent_turn_id và snapshot_at_send, tạo branch trong logical DAG; UI cho biết run nào chưa có trong snapshot.  
      **Kiểm chứng / đầu ra:** Hai send liên tiếp trong 100 ms không overwrite parent/context; kết quả về ngược vẫn đúng.
  - [ ] **MC-03.02 — Định tuyến stream và tool**
    - [ ] **MC-03.02.01** Mọi delta/token/tool-call/patch event gắn conversation_id + run_id + event sequence.  
      **Cách triển khai:** Router kiểm tra event envelope từ provider/tool/patch, resolve run context và broadcast cho đúng listeners; không dựa vào active conversation DOM.  
      **Kiểm chứng / đầu ra:** Synthetic streams A/B xen kẽ không có chunk của A trong B.
    - [ ] **MC-03.02.02** Chuyển tab/ngắt frontend không hủy worker đang chạy; chỉ nút Stop đúng run mới cancel.  
      **Cách triển khai:** Tách lifecycle worker khỏi WebSocket/panel; frontend unsubscribe khi đổi tab chỉ ngắt display; stop endpoint có quyền và run_id.  
      **Kiểm chứng / đầu ra:** Close panel/reload trong lúc stream: worker tiếp tục và resubscribe được.
    - [ ] **MC-03.02.03** Output đến lệch thứ tự vẫn gắn đúng message/run và có chỉ báo nhánh lịch sử nếu phát sinh.  
      **Cách triển khai:** Render message theo parent_turn, run_id, sequence, time; khi out-of-order show pending/finished child đúng vị trí; tránh sort solely timestamp.  
      **Kiểm chứng / đầu ra:** Fixture #102 complete trước #101 vẫn liên kết đúng prompt.
  - [ ] **MC-03.03 — Bảo đảm thực thi không lặp**
    - [ ] **MC-03.03.01** Deduplicate gửi request khi click đôi, reconnect stream, timeout request hoặc server retry.  
      **Cách triển khai:** Server enforce unique (project, idempotency_key), store result/run_id; dedup reconnect SSE, retry request POST không khởi tạo run trùng.  
      **Kiểm chứng / đầu ra:** Simulate network timeout sau server accept: 2 POST = 1 provider invocation.
    - [ ] **MC-03.03.02** Tool/patch side effect có operation_id; tránh thực thi lại nếu kết quả cũ còn có thể lấy.  
      **Cách triển khai:** Operation ledger theo operation_id, patch_id, run_id, input_hash, output/result; khi outcome unknown chỉ inspect evidence hoặc chờ user, không tự chạy lại non-idempotent tools.  
      **Kiểm chứng / đầu ra:** Injected crash tại commit/patch boundary không tạo double side effects.
    - [ ] **MC-03.03.03** Stop run A không tác động run B, patch B, hoặc AI retry đang hoạt động của conversation khác.  
      **Cách triển khai:** Cancel scope theo run/attempt; patch worker có ownership, avoid passing parent cancel to unrelated runs; timer retry cancel riêng.  
      **Kiểm chứng / đầu ra:** E2E Stop A khi patch B chạy/AI retry C chờ không ảnh hưởng B/C.

- [ ] **MC-04 — Giao diện nhiều conversation**
  - [ ] **MC-04.01 — Conversation navigation**
    - [ ] **MC-04.01.01** Bổ sung danh sách/tree conversation theo context group và nhóm Independent.  
      **Cách triển khai:** UI cây Context Group A → C1/C2/C3 và Independent; lazy load, group collapse, sort by updated_at, distinguish archived.  
      **Kiểm chứng / đầu ra:** Có thể mở 3 conversation riêng; selection highlight đúng.
    - [ ] **MC-04.01.02** Hỗ trợ tạo mới, rename, fork từ message/snapshot, move vào/ra group, archive/delete có xác nhận.  
      **Cách triển khai:** Implement Create/Fork/Rename/Move/Archive/Delete với confirm cho dữ liệu mất, semantics xóa active runs (stop? archive?) rõ ràng; fork pinned snapshot.  
      **Kiểm chứng / đầu ra:** Các action persist sau refresh, không phá conversation khác.
    - [ ] **MC-04.01.03** Hiển thị compact status badge/running count và provider của từng run; giữ nhãn History ngắn.  
      **Cách triển khai:** Hiển thị badge tổng active/running/queued/failed theo conversation, provider theo mỗi run từ field binding; menu History gọn, overflow chỉ khi có options.  
      **Kiểm chứng / đầu ra:** Badge phản ánh server state, không đếm từ UI tab đang mở.
  - [ ] **MC-04.02 — Tab/panel hoạt động độc lập**
    - [ ] **MC-04.02.01** Mở nhanh nhiều chat/tabs mà không reset composer draft, scroll, attachments hoặc selection của tab khác.  
      **Cách triển khai:** State per-conversation cho composer draft, pasted code, attach file, scroll anchor, focused input; restore khi switch tab.  
      **Kiểm chứng / đầu ra:** Đổi C1→C2→C1 không mất draft/attachment hoặc cuộn tới đầu.
    - [ ] **MC-04.02.02** Tạo UI store keyed conversation_id/run_id; bỏ shared loading/abort/stream buffer.  
      **Cách triển khai:** Refactor frontend store keyed `conversation_id` và nested run_id (messages, loading, streaming, abort status); không tái dùng buffer/chunk singleton.  
      **Kiểm chứng / đầu ra:** Component tests simulate 3 streams interleaved không bị trộn nội dung.
    - [ ] **MC-04.02.03** Có khả năng chuyển tab, đóng panel, reopen khi các run khác vẫn tiếp tục.  
      **Cách triển khai:** Closing tab chỉ unmount view/unsubscribe; backend vẫn chạy; reopening hydrate stored history và subscribe last event cursor.  
      **Kiểm chứng / đầu ra:** Run completed khi tab đóng xuất hiện đúng sau reopen.
  - [ ] **MC-04.03 — Trạng thái & tương tác**
    - [ ] **MC-04.03.01 — PARTIAL** Hiển thị run in progress/queued/waiting/failed cùng stop/retry chi tiết đúng run.  
      **Cách triển khai:** Per-run actions Stop/Retry/Inspect Tool, show queued/waiting status and reason, disable invalid actions; prevent panel-wide stop ambiguity.  
      **Kiểm chứng / đầu ra:** Bấm Stop run cụ thể chỉ một run đổi STOPPED.  
      **Implementation note (partial, uncommitted):** Added standalone `taskworkflows.js` Run All UI through a separate menu entry (not the Hot-Switch selector file): manifest validation, provider/model+context-group pin, concurrency, per-task wait/status/report, approve/reject/retry/reviewed-resume and workflow stop. Static contract tests pass. Per-conversation tabs/stream/draft state, individual run controls, cross-tab browser E2E remain pending.
    - [ ] **MC-04.03.02** Hiển thị response out-of-order rõ ràng, không tự gắn kết quả vào prompt sau.  
      **Cách triển khai:** UI hiển thị nhánh reply theo prompt/parent_turn, label run # và timestamp; cảnh báo snapshot không chứa pending peer result.  
      **Kiểm chứng / đầu ra:** Test out-of-order và overlapping user prompts dễ theo dõi.
    - [ ] **MC-04.03.03** Thêm tìm kiếm/filter context group, status, provider và tên conversation khi phù hợp.  
      **Cách triển khai:** Filter names/group/provider/status với debounce, pagination, empty states; search context chỉ search metadata thuộc project được phép.  
      **Kiểm chứng / đầu ra:** Filter nhanh khi >=100 chats, không hiển thị group khác.

- [ ] **MC-05 — Shared Context Group và snapshot**
  - [ ] **MC-05.01 — Khởi tạo context chung**
    - [ ] **MC-05.01.01 — PARTIAL** Lưu mục tiêu task lớn, instructions, constraints, code conventions và references của context group.  
      **Cách triển khai:** Group profile gồm title/goal/requirements/instructions/style/code references + owner + revision; yêu cầu edit/update API có optimistic lock.  
      **Kiểm chứng / đầu ra:** Ba thành viên nhận chung instructions v1; chỉnh thành v2 không sửa run cũ.  
      **Implementation note (partial, uncommitted):** Owner/project-bound Context Group CRUD store and API persist title/instructions/hash/revision. Workflow can pin one group revision and inject its instructions into every task snapshot. Membership/fork/collaborator visibility and UI remain pending.
    - [x] **MC-05.01.02** Khi fork, sao chép hoặc tham chiếu lịch sử đến điểm fork; những message tiếp theo là local và không bị trộn.
      **Cách triển khai:** Fork conversation record có source_conversation_id + fork_turn_id; pin ancestor snapshot và chỉ lưu local events sau fork; tránh duplicate history bằng graph/source refs.
      **Kiểm chứng / đầu ra:** C1/C2 cùng prefix nhưng viết hai prompt khác nhau không cross-contaminate.
      **Bằng chứng (2026-11-23):** `go test ./internal/agentrun ./internal/server -run 'Test(SharedSnapshotForFork|ForkConversationWhileSourceIsActive)' -count=1` PASS; test fork đầu/giữa/cuối trên source đang active, hai fork độc lập và boundary ID không tồn tại fail-closed. Race run của cả hai package cũng PASS.
    - [ ] **MC-05.01.03 — PARTIAL** Cho phép chỉnh shared context có revision và xem được nguồn gốc/version của thay đổi.  
      **Cách triển khai:** Mỗi update tạo immutable group_revision và change log ai/khi nào/diff; snapshot reference revision; rollback revision bằng new revision.  
      **Kiểm chứng / đầu ra:** Xem được v1/v2 và run dùng v1 giữ nguyên khi group lên v2.  
      **Implementation note (partial, uncommitted):** Context Group update dùng `expected_revision` CAS; workflow lưu `context_group_id`, revision, SHA-256 và instruction text; mỗi run snapshot ghi source ref. Test xác nhận sửa group v2 không đổi snapshot v1. Change actor/history UI còn pending.
  - [ ] **MC-05.02 — Tạo prompt/context đúng phạm vi**
    - [ ] **MC-05.02.01** Lắp snapshot từ global + group + local history + coordination digest + attachments theo chính sách.
      **Cách triển khai:** Snapshot assembler thứ tự system/global → group → ancestor prefix → local turn chain → selected peer digest → attachments; mỗi nguồn có provenance/scope.
      **Kiểm chứng / đầu ra:** Inspect snapshot of C2 và assert không chứa private C3 local content ngoài digest đã share.
      **Implementation note (partial, 2026-11-23):** Standard Agent Run snapshots now inject revision/hash-pinned shared instructions only from groups containing the conversation. `TestContextSnapshotPinsGroupInstructionsOnlyForMembers` PASS covers member inclusion, independent-conversation isolation, and immutable old snapshots after a group revision update. Coordination digests, attachment provenance and a context-inspection UI remain pending.
    - [ ] **MC-05.02.02 — PARTIAL** Pin context_snapshot_id cho mỗi run/attempt, lưu hashes/manifest để debug và retry tái lập.  
      **Cách triển khai:** Persist context manifest (source refs, rev, hashes, truncation decisions, token estimates), immutable snapshot ID referenced by run/attempt.  
      **Kiểm chứng / đầu ra:** Retry same run reconstruct byte-equivalent normalized context nếu cấu hình/attachments còn sẵn.  
      **Implementation note (partial, uncommitted):** Snapshot content is hash-verified, owner/conversation/project bound and includes source refs for pinned group revision + manifest/task. Attachments, truncation/token manifest and provider-independent reserialization remain pending.
    - [ ] **MC-05.02.03** Đặt token budget, compaction/summarization có nguồn tham chiếu; không nhét toàn bộ peer history mỗi request.  
      **Cách triển khai:** Budget per provider capability; trim bằng priority và token count estimate; summary có nguồn và revision; không auto dump full peer histories.  
      **Kiểm chứng / đầu ra:** Context nhiều peers không vượt provider limit; vẫn giữ critical user instruction.
  - [ ] **MC-05.03 — Cập nhật peer information an toàn**
    - [ ] **MC-05.03.01** Publish summary/delta khi peer đổi trạng thái hoặc công bố artifact; đánh revision.  
      **Cách triển khai:** Cơ chế publish_progress/state_delta/artifact event với group_id, owner, revision, summary và visibility; debounce repeated notifications.  
      **Kiểm chứng / đầu ra:** C2 nhận một delta mới khi C1 publish contract v2, không duplicate event.
    - [ ] **MC-05.03.02** Đồng bộ ở điểm checkpoint/request/tool-turn tiếp theo; không giả định thay đổi giữa chừng đã đến model.  
      **Cách triển khai:** Chỉ refresh context tại turn boundary/tool checkpoint/resume; underway model call không bị mutate; show pending update badge nếu checkpoint chưa tới.  
      **Kiểm chứng / đầu ra:** Event đến giữa stream không thay nội dung snapshot của run active.
    - [ ] **MC-05.03.03** Có nút xem context được đính kèm và khả năng ngắt chia sẻ cho từng conversation.  
      **Cách triển khai:** Inspect Context UI hiển thị nguồn global/group/local/peer, revisions, truncated files; toggle share/unshare (tác động future snapshots, không thu hồi thông tin đã gửi).  
      **Kiểm chứng / đầu ra:** Tắt share C2 khiến request tiếp theo không mang peer digest.

- [ ] **MC-06 — Coordination Hub, contracts và dependencies**
  - [ ] **MC-06.01 — Lưu sự kiện phối hợp**
    - [ ] **MC-06.01.01** Tạo event log append-only cho task assignment, progress, blockers, requests, contracts và artifacts.  
      **Cách triển khai:** Append-only coordination log với types TASK_ASSIGNED/PROGRESS/BLOCKED/ASK/ANSWER/CONTRACT/ARTIFACT, producer, consumer, contract ref.  
      **Kiểm chứng / đầu ra:** Can query events theo group và correlate về initiating run.
    - [ ] **MC-06.01.02** Chống vòng lặp hỏi đáp agent-agent, duplicate event, và lưu correlation_id/TTL/timeout.  
      **Cách triển khai:** Mỗi peer request có correlation_id, hop_count <= cấu hình, dedup key, TTL và max unanswered; không cho auto ping-pong vô hạn.  
      **Kiểm chứng / đầu ra:** Synthetic loop A→B→A bị chặn với lỗi rõ; hỏi trùng không tăng số event.
    - [ ] **MC-06.01.03** Tạo bảng dependency task ↔ API/file/commit và trạng thái PLANNED/IMPLEMENTED/INTEGRATED/VERIFIED.  
      **Cách triển khai:** Dependency record link consumer task/run→producer contract/artifact, status PLANNED→IMPLEMENTED→INTEGRATED→VERIFIED, workspace/ref/test-evidence.  
      **Kiểm chứng / đầu ra:** Không cho VERIFIED khi thiếu SHA + integration proof + passing test.
  - [ ] **MC-06.02 — Cung cấp giao thức peer communication**
    - [ ] **MC-06.02.01** Triển khai get_peer_status, ask_peer, publish_progress, publish_contract, get_contract, register_dependency.
      **Cách triển khai:** Publish service/typed tool API `get_peer_status`, `ask_peer`, `publish_progress`, `publish_contract`, `get_contract`, `register_dependency`; validate group membership.
      **Kiểm chứng / đầu ra:** Fake agent contract tests cho success/403/timeout và schema validation.
      **Implementation note (partial, 2026-11-23):** Shared HTTP routes now expose conversation/group/coordination APIs with separate read/write requirements. Mutation handlers require `settings.write`; peer and group-scoped contract/artifact/dependency writes verify conversation owner/project and membership. Focused permission, membership and ask/answer tests PASS. Typed agent tools, inbox/deadline behavior and timeout/evidence workflow remain pending.
      **Implementation note (partial, 2026-11-23):** Shared-server routing now maps conversation/group/coordination reads and writes separately; mutation handlers require `settings.write`, and peer progress/messages plus group contract/artifact/dependency mutations validate owner/project and group membership. Focused RBAC/membership/ask-answer tests PASS. Tool APIs, peer mailbox deadlines and timeout behavior remain pending.
    - [ ] **MC-06.02.02** Cho phép hỏi agent đang chạy theo hàng đợi; peer phản hồi ở checkpoint hoặc request được phép.  
      **Cách triển khai:** Mailbox per conversation; peer ở RUNNING nhận notification tại tool checkpoint; peer idle có Inbox và tùy chọn explicit agent response, không tự khởi động model tùy tiện.  
      **Kiểm chứng / đầu ra:** Một câu hỏi tới peer running không mất khi peer switch tab.
    - [ ] **MC-06.02.03** Nếu peer không phản hồi, trả trạng thái waiting/timeout; không tự coi câu hỏi đã được giải quyết.  
      **Cách triển khai:** WAITING_PEER lưu deadline và escalation; timeout trả unresolved + last known contract, có nút Manual Resolve/Continue With Assumption.  
      **Kiểm chứng / đầu ra:** Peer offline => consumer biết chưa xác nhận, không tự đánh dấu dependency done.
  - [ ] **MC-06.03 — Đảm bảo correctness của API contract**
    - [ ] **MC-06.03.01** Lưu owner, signature, input/output, exceptions, version và paths liên quan cho từng contract.  
      **Cách triển khai:** Contract registry lưu language, symbol, signature, params, return, errors, semver/revision, owner, file_paths, code_ref, workspace_id.  
      **Kiểm chứng / đầu ra:** C2 fetch được contract A2 đủ để viết caller; versioned updates query được.
    - [ ] **MC-06.03.02** Thông báo thay đổi contract/breaking change đến các conversation bị ảnh hưởng.  
      **Cách triển khai:** Diff contract schema/version, classify breaking (rename, args/return) và publish invalidation tới consumers, mark dependencies NEEDS_REVIEW.  
      **Kiểm chứng / đầu ra:** Thay return type của A2 khiến C2 thấy cảnh báo và pending verify.
    - [ ] **MC-06.03.03** Chỉ đánh dấu INTEGRATED/VERIFIED khi artifact đã có trong workspace dùng và test qua; không dựa vào chat nói đã hoàn tất.  
      **Cách triển khai:** IMPLEMENTED cần code ref, INTEGRATED cần ref trong workspace consumer, VERIFIED cần build/test evidence cho dependency; state transitions audit.  
      **Kiểm chứng / đầu ra:** Không thể click VERIFIED nếu chỉ có chat báo hoàn thành hoặc commit chưa áp dụng.
  - [ ] **MC-06.04 — Hiển thị Coordination UI**
    - [ ] **MC-06.04.01** Thêm tab/activity Coordination Events, peer status, dependency tree, blocked items và cross-link sang run gốc.
      **Cách triển khai:** Coordination tab gồm event timeline, peer status, contract list, dependency graph, blocked queue; click event mở originating conversation/run.
      **Kiểm chứng / đầu ra:** Người dùng tìm được C1 owner của A2 trong <=2 thao tác từ C2.
      **Implementation note (partial, 2026-11-23):** Thêm Groups & Coordination panel dùng API group/coordination hiện có: group create/membership/instructions, peer progress/ask-answer, contract publish/advance, artifact publish, dependency register và feed. Shared routes tách quyền đọc/ghi; mutation handlers kiểm tra conversation owner/project và group membership. Node UI, embedded-asset, permission và membership tests PASS. Dependency graph, blocked-item actions, event-to-run cross-link và browser E2E chưa có, nên checkbox còn mở.
    - [ ] **MC-06.04.02** Cho phép người dùng xác nhận/override dependency hoặc dừng luồng trao đổi lặp.  
      **Cách triển khai:** Controls answer peer request, approve contract override, cancel unresolved/loop; actions log actor + reason + revision.  
      **Kiểm chứng / đầu ra:** Override chỉ cập nhật status với audit và không giả mạo build proof.
    - [ ] **MC-06.04.03** Thông báo rõ khi context/contract của peer còn stale hoặc chỉ là kế hoạch.  
      **Cách triển khai:** Badges PLANNED/IMPLEMENTED/INTEGRATED/VERIFIED, stale revision timestamp, tooltip artifact provenance; never paint planned as ready.  
      **Kiểm chứng / đầu ra:** C2 vẫn thấy A2 PLANNED cho tới khi có evidence.

- [ ] **MC-07 — Workspace, Git isolation và Patch integration**
  - [ ] **MC-07.01 — Chế độ làm việc với source code**
    - [ ] **MC-07.01.01** Hỗ trợ Independent/Shared Context/Shared+Coordination tách biệt với Shared Directory/Isolated Worktree.  
      **Cách triển khai:** Tách ContextMode và WorkspaceMode thành hai enum, UI không ép shared context dùng shared files; default isolation khi concurrent write.  
      **Kiểm chứng / đầu ra:** Matrix 3 context modes × 2 workspace modes hoạt động hợp lệ.
    - [ ] **MC-07.01.02 — PARTIAL** Khi isolated, ánh xạ mỗi conversation tới worktree/branch riêng với base commit và tracking head.  
      **Cách triển khai:** Với isolated, tạo branch/worktree riêng từ selected base SHA; persist worktree path, HEAD, owner conversation và cleanup state.  
      **Kiểm chứng / đầu ra:** C1 sửa file không thay nội dung file trong C2 trước integration.  
      **Implementation note (partial, uncommitted):** `agentrun.WorkspaceRegistry.CreateDetached` tạo worktree detached từ HEAD ngoài working tree owner, bind conversation/run/provider-config reference/permission scope và project identity; registry persisted dưới private state. Tests validate root+child mapping, unrelated repo rejection and restart-readable binding. Có GET integration report on-demand (base/head, changed paths, diff/status hashes, no mutation) và workflow lưu report per-task trong DAG record; browser report/review UI, cleanup policy và actual Python Patch execution trong child vẫn pending; fake-native approval/queue-selection/result-continuation is now tested.
    - [ ] **MC-07.01.03** Định nghĩa policy checkout, cleanup, stale worktree và không tự ý xóa thay đổi chưa commit.  
      **Cách triển khai:** Quy định worktree creation preflight, safe cleanup chỉ khi clean/approved, stale detection và preserve uncommitted changes; handle orphan worktrees.  
      **Kiểm chứng / đầu ra:** Delete conversation không xóa file chưa commit, báo đường dẫn recover.
  - [ ] **MC-07.02 — Đồng bộ artefact/dependency**
    - [ ] **MC-07.02.01** Cho phép publish commit/ref/diff/patch của agent sang registry; ghi provenance và checksum.  
      **Cách triển khai:** Artifact Registry hỗ trợ commit SHA, branch/ref, file path, patch id, checksum, producer run, dependency ids; publish event khi artifact sẵn sàng.  
      **Kiểm chứng / đầu ra:** Consumer verify checksum/provenance thay vì tin vào message text.
    - [ ] **MC-07.02.02** Cung cấp integration workflow review → apply/cherry-pick/merge → build/test; thông báo conflict.  
      **Cách triển khai:** Workflow integrate: choose artifact → preview diff/affected files → conflict check → cherry-pick/merge/apply → build/test → register integrated_ref; rollback on failure.  
      **Kiểm chứng / đầu ra:** A2 integrated chỉ sau diff review và test success; conflict giữ nguyên code khi abort.
    - [ ] **MC-07.02.03** Ở shared directory, phát hiện cùng file đang bị ghi và cảnh báo/lock/cas hoặc serialize thao tác có rủi ro.  
      **Cách triển khai:** Shared directory sử dụng per-file lease/CAS mtime+hash hoặc serialize patch operations; diff preview before write, warn stale write.  
      **Kiểm chứng / đầu ra:** Hai workers sửa cùng file không mất update thầm lặng.
  - [ ] **MC-07.03 — Tương thích Patch Tool và terminal**
    - [ ] **MC-07.03.01 — PARTIAL** Gắn native patch session/queue/status với run_id và conversation_id; không mất mapping khi chuyển tab.  
      **Cách triển khai:** Native Patch Session/queue giữ mapping run_id, conversation_id, workspace_id, attempt_id, patch_id; migrate old patch records safely.  
      **Kiểm chứng / đầu ra:** Switch tabs/refresh vẫn đúng console/evidence của từng run.  
      **Implementation note:** Run Event journal persists `run_id`, immutable `attempt_id`, provider ID, monotonic sequence and tool-call payload; run stays `waiting_tool` after tool proposal. Run API proposals are materialized into the owner conversation and routed through explicit approval/native queue/reconcile handlers with a child execution Server; verified native result resumes the same RunID with a new snapshot/attempt. Fake-native reject coverage remains; additionally, `TestAgentRunNativePatchRoundTripExecutesOnceAcrossRestart` PASS runs the repository's real Python Patch entrypoint and native session manager, verifies approval, exact single queue package, Run stop without falsely stopping Patch, result reconciliation, restart recovery and continuation. Provider-generated production call and browser UI correlation/reconnect still need PLAN 02 + browser E2E verification; checkbox remains open.
    - [ ] **MC-07.03.02** Retry patch cũ hoặc gửi lại kết quả patch phải được scope đúng run; chống chạy lại patch đã thành công.  
      **Cách triển khai:** UI retry patch có hai action `rerun_patch` và `resend_existing_result`, idempotent operation ledger, inspect status trước retry hung patch.  
      **Kiểm chứng / đầu ra:** Resend không thực thi patch lần hai; rerun tạo operation mới được audit.
    - [ ] **MC-07.03.03** Ràng buộc terminal/log/file attachments và quyền đọc ghi với workspace tương ứng.  
      **Cách triển khai:** File attachment/terminal target phải resolve qua workspace binding, không dùng global active root; enforce allowed root/file path.  
      **Kiểm chứng / đầu ra:** C1 Attach từ worktree C1 không lẫn file C2; terminal command chạy đúng cwd.

- [ ] **MC-08 — Tương thích các tính năng AI Agent hiện có**
  - [ ] **MC-08.01 — History, Attach to Chat và debug**
    - [ ] **MC-08.01.01** Giữ nguyên History/rename/copy/export với nhiều conversation và group.  
      **Cách triển khai:** History list/search/rename/copy/export thêm group filter nhưng giữ format legacy; action đúng conversation even when tab inactive.  
      **Kiểm chứng / đầu ra:** Regression fixture: old chat và new chat đều export nội dung chuẩn.
    - [ ] **MC-08.01.02** Attach file, selected text + line ranges, terminal console đúng conversation đang soạn; không trôi attachment khi đổi tab.  
      **Cách triển khai:** Draft attachment keyed conversation; metadata file path, selected lines, terminal session, file revision, content hash; attach-to-chat chọn target rõ.  
      **Kiểm chứng / đầu ra:** Đổi tab nhanh trong lúc Attach không chuyển nhầm file sang chat khác.
    - [ ] **MC-08.01.03** Debug Console có filter conversation/run/provider và export event/correlation IDs; không lộ credentials.  
      **Cách triển khai:** AI Debug view filter group/conversation/run/provider/attempt, export redacted traces, redact credentials và raw auth headers.  
      **Kiểm chứng / đầu ra:** Trace đủ đối chiếu request→patch; secret scanner không phát hiện token.
  - [ ] **MC-08.02 — Automation và recovery**
    - [ ] **MC-08.02.01** AI retry Off/5 times/unlimited hoạt động theo từng run; stop chính xác và delay 10s đối với chế độ 5 lần theo thiết lập hiện có.  
      **Cách triển khai:** AI Retry Off/5/unlimited mỗi run có policy snapshot, counter và 10s wait theo config; stop cancels retry timer; non-retriable fast-fail.  
      **Kiểm chứng / đầu ra:** Hai run có policy khác nhau chạy đồng thời không chung counter.
    - [ ] **MC-08.02.02** Không gán retry của run này vào provider hoặc conversation đang active khác.  
      **Cách triển khai:** Retry engine lookup original run spec và provider identity; mỗi attempt log parent run, provider config snapshot; không dựa vào active tab.  
      **Kiểm chứng / đầu ra:** Switch provider/selection rồi retry C1 vẫn đúng C1/provider gốc.
    - [ ] **MC-08.02.03** Đồng bộ notification và state AI Pack preview/close box, patch evidence/terminal links theo đúng run.  
      **Cách triển khai:** AI Pack preview close state per run, patch evidence links per patch operation, notifications route by originating run.  
      **Kiểm chứng / đầu ra:** Preview C1 bị đóng không làm C2 biến mất; links mở đúng console.

- [ ] **MC-09 — Kiểm thử nhiều cấp và tính ổn định**
  - [ ] **MC-09.01 — Unit tests**
    - [ ] **MC-09.01.01** Test state machine, event ordering, idempotency, optimistic version, ACL/context scope và compaction.  
      **Cách triển khai:** Table driven tests transition rules, event sequence ordering, duplicate idempotency keys, optimistic locks, context scope, summary provenance.  
      **Kiểm chứng / đầu ra:** Run targeted tests/race detector, báo line coverage phần core.
    - [ ] **MC-09.01.02** Test parent_turn/thread fork, dependency transitions và contract invalidation.  
      **Cách triển khai:** Fixtures fork at middle turn, parallel descendants, contract v1→v2 invalidation và dependency state guards.  
      **Kiểm chứng / đầu ra:** Không auto VERIFY khi artifact ở workspace khác.
    - [ ] **MC-09.01.03 — PARTIAL** Test scheduler concurrency caps, cancellation isolation và retry budget độc lập.  
      **Cách triển khai:** Fake providers emit tokens asynchronously với errors; cap scheduler, cancel run A, retry budgets B/C.  
      **Kiểm chứng / đầu ra:** Test deterministic với fake clock, không flake do sleep thực.  
      **Implementation note (partial, 2026-11-23):** `go test -race ./internal/agentrun -count=1` PASS, including global cap, Stop, restart, retry and DAG cases. Focused `internal/server` race tests PASS. `TestAgentRunNativePatchRoundTripExecutesOnceAcrossRestart` additionally runs the real Python Patch entrypoint/native session manager, then validates one queue package, Run cancellation semantics, restart and same-run continuation. Concurrent provider streaming and independent AI retry budgets remain pending.
  - [ ] **MC-09.02 — Integration tests**
    - [ ] **MC-09.02.01** Chạy 3 conversation đồng thời, một fail/stop/retry, hai còn lại không bị ảnh hưởng.  
      **Cách triển khai:** Integration harness tạo C1/C2/C3, send đồng thời, stop C1, retry C2 và run C3 vẫn complete; trace corr IDs.  
      **Kiểm chứng / đầu ra:** Không có cross-cancel/cross-stream trong DB và UI.
    - [ ] **MC-09.02.02** Test shared context nhóm A, independent context nhóm B, peer ask/answer và A2 dependency chỉ verified sau build.  
      **Cách triển khai:** Fixture group A shared v1, independent B isolated; publish A2 PLANNED, ask/answer, merge artifact and build.  
      **Kiểm chứng / đầu ra:** Chỉ sau artifact integrated + test mới VERIFIED; B không đọc A.
    - [ ] **MC-09.02.03** Test patch đang RUNNING/FAILED, restart backend/UI, reconnect SSE/websocket và replay events.  
      **Cách triển khai:** Inject patch failure/hang, kill UI/backend, replay events; probe native patch status before restart action.  
      **Kiểm chứng / đầu ra:** Không mất patch evidence hoặc rerun duplicate khi reconnect.
  - [ ] **MC-09.03 — UI/E2E & performance**
    - [ ] **MC-09.03.01** Test chuyển tab/draft/attachments/scroll và response về sai thứ tự.  
      **Cách triển khai:** E2E browser three tabs chats: drafts, attachments, scroll, compose; reverse provider response order, ensure branch display.  
      **Kiểm chứng / đầu ra:** Ảnh/video và automated assertion trên message order/ownership.
    - [ ] **MC-09.03.02** Stress test số conversation active theo giới hạn cấu hình; đo latency streaming, memory, queue backlog.  
      **Cách triển khai:** Measure with configurable N concurrent runs, avg/p95 streaming latency, RSS, queue depth, event replay time; compare baseline.  
      **Kiểm chứng / đầu ra:** Chạy đến cap dự kiến không leak, không starvation; ngưỡng ghi rõ theo môi trường test.
    - [ ] **MC-09.03.03** Chạy regression cho single conversation, History, Debug, Attach to Chat, Patch Tool và AI Retry.  
      **Cách triển khai:** Regression suite toàn bộ single-conversation flows (History, preview, Attach to Chat, Patch, AI Retry, Debug).  
      **Kiểm chứng / đầu ra:** Các test baseline pass hoặc documented known pre-existing fail.

- [ ] **MC-10 — Rollout, tài liệu và bàn giao**
  - [ ] **MC-10.01 — Triển khai có kiểm soát**
    - [ ] **MC-10.01.01** Đặt feature flags multi_conversation/shared_context/coordination/worktree, mặc định an toàn theo giai đoạn.  
      **Cách triển khai:** Flags multi_conversation, shared_context, coordination, isolated_worktree tách độc lập; hide UI khi disabled nhưng DB/history vẫn đọc được.  
      **Kiểm chứng / đầu ra:** Toggle flag off không làm mất dữ liệu mới; một chat cũ dùng được.
    - [ ] **MC-10.01.02** Có migration/rollback procedure, cảnh báo dữ liệu có thể bị ảnh hưởng và cách khôi phục.  
      **Cách triển khai:** Viết migration dry-run, backup/restore, rollback code vs rollback data, forward-only fields; drill trên dữ liệu giả lập.  
      **Kiểm chứng / đầu ra:** Có runbook và logs thành công; không chạy destructive migration không backup.
    - [ ] **MC-10.01.03** Ghi metrics run failures, orphan events, mismatched run IDs, context size, peer wait và Git conflicts.  
      **Cách triển khai:** Metrics counters + structured logs: orphan_event, run_mismatch, peer_wait, contract_invalidated, patch_conflict, queue_latency, snapshot_size.  
      **Kiểm chứng / đầu ra:** Dashboard/log query phân biệt project/run, alert trên mismatch >0.
  - [ ] **MC-10.02 — Tài liệu & release gates**
    - [ ] **MC-10.02.01** Viết architecture decision record, API/schema docs, sequence diagrams và user guide tiếng Việt.  
      **Cách triển khai:** ADR state/schema, sequence diagram A2 producer/consumer, REST/WS contract examples, user guide tính năng nhóm/coordination.  
      **Kiểm chứng / đầu ra:** Tài liệu khớp code với links tới tests và code owners.
    - [ ] **MC-10.02.02** Cập nhật checklist vận hành, debug/recovery procedures và nhật ký commit/test bằng chứng.  
      **Cách triển khai:** Runbook recovery, patch reattach, inspect orphan operation; commit/test/evidence log sau mỗi batch; cập nhật task checklist cùng changes.  
      **Kiểm chứng / đầu ra:** Reviewer truy từ MC-* tới commit SHA, test command, result.
    - [ ] **MC-10.02.03** Đóng milestone chỉ khi toàn bộ child checklist và các acceptance scenario liên quan đạt.  
      **Cách triển khai:** Checklist gate tính toán từ tất cả descendants; require acceptance tests pass và không còn blocking bug; không auto tick nếu evidence thiếu.  
      **Kiểm chứng / đầu ra:** Milestone complete phản ánh actual evidence, không chỉ marker done.

## 4. Thứ tự thực hiện và cổng nghiệm thu

| Giai đoạn | Phụ thuộc | Điều kiện qua giai đoạn |
|---|---|---|
| Nền móng | MC-00 → MC-01 → MC-02 → MC-03 | 2+ run hoạt động độc lập, persistent, event routing không chéo |
| UI nhiều chat | MC-03 → MC-04 | Mở/đổi tab/refresh mà không hủy run khác |
| Chia sẻ context | MC-01/02 → MC-05 | 3 conversation kế thừa shared instruction, local history không bị trộn |
| Agent coordination | MC-05 → MC-06 | C2 biết A2 PLANNED, hỏi C1 và verified sau integration thật |
| Source integration | MC-03/06 → MC-07 → MC-08 | Conflict xử lý rõ; Patch Tool, History, Attach to Chat không regress |
| Release | MC-09 → MC-10 | Unit/integration/E2E pass, có migration/rollback/docs |

## 5. Acceptance scenarios (phải có automated test hoặc bằng chứng chạy thật)

| Mã | Tình huống | Kết quả cần đạt |
|---|---|---|
| MC-AC-01 | C1/C2/C3 gửi đồng thời | Ba run độc lập, streamed đúng conversation |
| MC-AC-02 | Stop C1 đang chạy | C2/C3 và patch của họ không dừng |
| MC-AC-03 | C2 retry 5 lần, C3 unlimited | Counter/timer/stop độc lập |
| MC-AC-04 | C2 nhận context từ group A, C4 independent | C4 không nhận nội dung group A |
| MC-AC-05 | C1 công bố A2 là PLANNED | C2 thấy contract, không báo code đã có |
| MC-AC-06 | C1 implementation xong, merge vào C2 + test | A2 đổi thành VERIFIED kèm evidence |
| MC-AC-07 | Response C3 về trước C1 | Thứ tự logic theo parent turn, không sai chat |
| MC-AC-08 | Reload frontend/server trong khi running | Không nhân đôi request/tool; trạng thái khôi phục hoặc interrupted minh bạch |
| MC-AC-09 | Hai agent chỉnh cùng file | Worktree cách ly hoặc shared mode cảnh báo/điều phối conflict |
| MC-AC-10 | Legacy History và Patch Tool | Dữ liệu cũ còn đọc được, patch session gắn đúng run |

## 6. Hướng dẫn coder AI cập nhật plan

1. Trước khi làm: fetch/pull HEAD mới nhất, kiểm tra branch hiện hành, đọc code thực tế và ghi baseline SHA.
2. Mỗi lần làm xong một subtask: sửa **[ ] → [x]** đúng mục, bổ sung link commit/PR, file/line hoặc kết quả test ngay bên dưới task liên quan nếu cần.
3. Chỉ đánh dấu task cấp 2 hoàn tất khi các subtask cấp 3 hoàn tất; tương tự cấp 1 chỉ hoàn tất khi mọi task con hoàn tất và acceptance gate đạt.
4. Nếu phải thay đổi kiến trúc/hợp đồng chung: ghi ADR, thông báo nhóm Hot-Switch và chạy contract tests trước khi tiếp tục.
5. Sau mỗi đợt triển khai, ghi: HEAD trước/sau, changed files, tests PASS/FAIL/SKIP, blockers, remaining tasks; không suy diễn việc đã test từ việc build thành công.

### Nhật ký tiến độ

| Ngày | Branch / commit | Milestone | Tests / evidence | Ghi chú |
|---|---|---|---|---|
| 2026-11-20 | ffa61d1b / main (uncommitted, no commit by instruction) | Run/workspace foundation | PASS: `go test -race -count=1 ./internal/agentrun ./internal/agentchat ./internal/taskmanifest`; targeted `go test -race ./internal/server` for `^TestAgent`, `^TestWorkflow`, `^TestManifestRunAll`, `^TestContextGroup`, plus `TestAgentRunPatchApprovalUsesExecutionWorktreeAndContinuesAfterNativeResult`; `go vet ./internal/agentrun ./internal/agentchat ./internal/taskmanifest ./internal/server`; `go test -run '^$' ./cmd/vscode_tasks_menu`; `node --check web/featuremods/taskworkflows.js`, `node --check web/featuremods/menus.js`, and `node web/taskworkflows.test.cjs` PASS. | Added manifest Run All API/PlanStore/DAG orchestration, task reports and human review gates; owner/project-scoped Context Groups pinned into task snapshots; detached worktree bindings; owner-bound Provider Router; fake-native Patch approval/reject/queue/result continuation in child worktrees; lease-backed global cap=5 held through `waiting_tool`. Fixed the suite semaphore leak in `TestAgentAdmissionAdmitsWaiterAfterRelease`; admission sequence + `TestRunOwnedGlobalSlotIsNotAcquiredTwiceByAgentChat` passes. Added separate menu-mounted Run All UI in `web/featuremods/taskworkflows.js` (Node static contract test PASS); Remaining: full Multi-Conversation tabs/cross-tab E2E, real Python/native/provider acceptance, process-kill recovery across separate OS process, Agent-to-Agent coordination/artifact registry, and owner-branch integration UX. Full `go test ./internal/server` still hits unrelated existing UI/assertion failures and a slow scrypt runtime probe timeout; full command package tests hit an unchanged `install.sh` string assertion. No stage/commit/push. |
| 2026-11-23 | `review/taskdeck-multiconversation-hotswitch-20261123` / `c10eb6e4` (working tree only) | Plan 01 review fixes | PASS: focused `go test ./internal/agentrun ./internal/server` regressions for fork start/middle/end, active-source independent forks, AddMember error handling, persisted history after restart, child-workspace snapshot response/run submit and rejection of unpinned child snapshots, peer ask/answer, member-only group-instruction snapshot pinning, shared RBAC/group-membership checks, Patch reject and actual native Patch continuation; `go test -race ./internal/agentrun`; targeted `go test -race ./internal/server`; `node --test web/agentchat_coordination_test.cjs`; `node --check web/featuremods/agentchat_coordination.js`. | Actual test launches repository `python_patch_entry.py` through a real native session manager: approval → one Patch queue package → stop Run while Patch continues → persisted result → restart → same-run continuation PASS. Fork boundary and group/coordination UI/API regressions PASS. Manual browser E2E and full server suite not run. PLAN 02 provider/router files were left untouched; concurrent PLAN 02 edits in the shared checkout were preserved. No stage/commit/push by this Plan 01 work. |
