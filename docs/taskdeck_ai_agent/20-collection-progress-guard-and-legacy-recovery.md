# HANDOFF 20 — Collection Progress Guard, Context Budget & Legacy Conversation Recovery

> **Implementation master plan / living checklist** — tài liệu giao trực tiếp cho AI Coder thực hiện, không phải chỉ phân tích hoặc đưa đề xuất. Phải sửa source, bổ sung regression test, cập nhật checklist và commit/push trực tiếp lên `main` theo từng task hoàn tất.

## 0. Thông tin và trạng thái

| Thuộc tính | Giá trị |
|---|---|
| Repository | `hacrot3000/PatchAndCollectionToolForAI` |
| Nhánh triển khai | **`main` — không tạo feature branch/PR thay cho yêu cầu này** |
| Baseline audit | `1c1e2ba0d5004bf5cee75b3fd8a2baa6be9c5422` (25/11/2026, 15:40:33 ICT) |
| Vị trí tài liệu trong repo (đề xuất) | `docs/taskdeck_ai_agent/20-collection-progress-guard-and-legacy-recovery.md` |
| Trạng thái hiện tại | `IN_PROGRESS` — Legacy runtime guard integrated; Go regression PASS at 92656e4e; latest UI/PATCH-invalidation CI pending; local checkout unavailable |
| Tiến độ | **0/10 phase · 3/36 task · 31/144 subtask DONE** |
| Phạm vi | Legacy Agent Chat, unified Agent Run, native COLLECT/PATCH, provider retry, compact context, journal, UI và migration |
| Ưu tiên | P1: tránh lặp/gián đoạn và bảo toàn dữ liệu; P2: quan sát, tuning và vận hành |

Baseline là mốc đã đọc source, không phải yêu cầu checkout/reset về commit cũ. Luôn đọc HEAD mới nhất trước khi làm và reconcile với các thay đổi mới. Không sửa code đã được khắc phục đúng; vẫn phải có bằng chứng kiểm tra và regression test trước khi đánh dấu task hoàn tất.

## 1. Chỉ thị bắt buộc cho AI Coder

- [ ] Đọc toàn bộ tài liệu này, kiểm tra `git status --short`, branch, HEAD, `origin/main`, các commit mới và các handoff 13–19 liên quan. Ghi baseline thực tế vào nhật ký tiến độ.
- [ ] Đặt bản tài liệu này đúng vị trí trong repository; không tạo thêm một master plan trùng lặp. Cập nhật mục lục tài liệu nếu repo có quy định.
- [ ] **Thực hiện code trực tiếp trên `main`.** Mỗi task/nhóm nhỏ có phụ thuộc chặt chẽ: đọc source cần thiết → sửa source → chạy test phù hợp → cập nhật checkbox/trạng thái/bằng chứng ngay trong file này → commit → push `origin main` → kiểm tra commit đã xuất hiện trên remote. Không chờ hoàn thành toàn bộ handoff mới commit.
- [ ] Nếu có commit đồng thời từ coder khác, fetch và reconcile an toàn; không force push, không reset hard, không ghi đè thay đổi ngoài phạm vi. Nếu không thể tích hợp an toàn, ghi blocker cụ thể.
- [ ] Nếu chạy thông qua TaskDeck local, sử dụng native COLLECT và native PATCH đang có; không tạo reader/runner thay thế, không bypass whitelist, không gọi sudo, không dùng PATCH để lách quyền command. Nếu môi trường agent không cấp Git hoặc test capability, vẫn triển khai phần được phép, ghi `BLOCKED` đối với bước chưa thực hiện được và giao đúng lệnh/bằng chứng cần người có quyền chạy. Không tuyên bố đã commit/push/test khi chưa có receipt.
- [ ] Giữ nguyên cơ chế approval, project identity, owner/workspace binding, run lease, capacity fence, durable journal và TaskPlan acceptance Handoff 16/17. Không đánh dấu hoàn tất task dựa vào lời AI tự báo hay provider `completed`.
- [ ] Sau mỗi task, tự lấy task khả thi tiếp theo; không yêu cầu người dùng nhập `continue` giữa các task. Nếu context gần hết, lưu checkpoint, next task và blockers ngay vào file; tiếp tục từ checkpoint qua cơ chế có sẵn. Không lặp COLLECT toàn bộ plan mỗi turn.

### 1.1. Quy tắc cập nhật tiến độ

Checklist có đúng ba cấp nghiệp vụ: cấp 1 = phase `CLP-xx` (10), cấp 2 = task `CLP-xx.yy` (36), cấp 3 = subtask `CLP-xx.yy.zz` (144). Checkbox Phase gate thuộc cấp 1. Mỗi subtask có checkbox độc lập và trạng thái riêng; khi bắt đầu đổi `TODO` thành `IN_PROGRESS`, sau từng thay đổi/test cập nhật ngay trạng thái và bằng chứng. Chỉ tick subtask khi phần việc đó được xác minh; task cấp 2 chỉ tick khi tất cả subtask và Acceptance đạt; phase chỉ tick khi các task con DONE và remote delivery được xác nhận. Báo riêng cả ba tổng, không cộng lẫn. Giữ ID ổn định. Các checkbox chỉ thị, regression matrix và Definition of Done không tính vào 144 subtask. Nếu phát hiện subtask mới, thêm ID kế tiếp trong task phù hợp và cập nhật tổng, không đổi ID cũ. Trạng thái: `TODO`, `IN_PROGRESS`, `VERIFY_PENDING`, `BLOCKED`, `DONE`. Checkbox `[x]` chỉ khi implementation và kiểm thử/đối chiếu bắt buộc đã có bằng chứng; nếu push chưa xác nhận thì không báo task đã được giao lên remote. Đánh dấu `IN_PROGRESS` ngay lúc nhận task; cập nhật sau từng thay đổi có ý nghĩa, kể cả khi chưa hoàn tất. Không dùng checkbox cho các kết quả chưa kiểm tra.

Ghi vào bảng nhật ký: thời gian, ID, thay đổi, file/test thực tế, kết quả, commit hoặc blocker, next task. Commit code và cập nhật checklist cùng nhau khi khả thi; nếu cần ghi SHA của chính commit đó, ghi trong commit kế tiếp (không tạo vòng lặp tự tham chiếu). `DONE` không thay thế việc xác minh remote; nếu push thất bại, ghi trạng thái delivery pending và xử lý trước khi báo hoàn tất.

Ví dụ lệnh (chỉ chạy bằng môi trường Git được cấp quyền):

```sh
git status --short
git fetch origin main
git branch --show-current
git rev-parse HEAD
git log -5 --oneline origin/main
# Sau khi sửa, test và cập nhật tài liệu:
git diff --check
git add -- <cac-file-thuoc-task> docs/taskdeck_ai_agent/20-collection-progress-guard-and-legacy-recovery.md
git commit -m "fix(agent): CLP-xx.yy <specific change>"
git push origin main
git fetch origin main
git merge-base --is-ancestor <commit-sha> origin/main
```

Không dùng `git add .` khi worktree có thay đổi khác. Không giả lập receipt test hoặc patch. PATCH wrapper PASS/rc=0 không chứng minh mọi lệnh test bên trong PASS.

## 2. Incident và bằng chứng đã xác nhận

Nguồn: `Pasted text(20261125-084934).txt`, báo cáo `taskdeck.agent.debug.report.v1`, tạo 25/11/2026 15:49:05 ICT. Conversation tạo 20/11/2026, chạy từ build cũ; provider `custom-r2d`, model `composer-2.5`, mode `stateless`.

| Dữ kiện | Quan sát |
|---|---|
| Session | `897b4014e45014d5ea61d0638a1f6fe9` |
| Event window hiển thị | seq 1572–1699, 27 `collect_started`, 2 `tool_final` của PATCH |
| PATCH lưu trong báo cáo | 5 bản: 3 PASS, 2 FAIL; PASS chỉ là trạng thái wrapper |
| Chuỗi COLLECT | 12 → PATCH → 7 → PATCH → 8 → failed; 1 schema error `grep` không hợp lệ |
| Hai PATCH cuối | call `chatcmpl-tool-87e2cbb50dd0b860`, `chatcmpl-tool-81c0b25efd642251`: feedback acknowledged |
| COLLECT cuối | `chatcmpl-tool-a076de338fe73566`, đã observed và egress; continuation bị lỗi |
| Payload | 304,153 B → 941,910 B (HTTP 400 context_length_exceeded) → 359,059 B |
| Provider traces | 6: 3 timeout ~600s, 1 HTTP 200, 1 HTTP 400, 1 cancellation |
| Thời gian timeout | Chủ yếu chờ response header; DNS/TCP/TLS/upload rất ngắn |
| Trạng thái cuối | `failed`, `provider_phase=unknown`, `auto_feedback=true`, `eligible=false`, reason `provider_delivery_unresolved` |

**Giới hạn bằng chứng:** log redacted không chứa arguments/output đầy đủ, không thể kết luận 27 COLLECT đều dư thừa, không thể xác định nguyên nhân server nội bộ của provider hoặc chính xác build SHA cũ. Không được biến giả thuyết thành sự thật. Fixture regression phải là synthetic, không đưa source/prompt/secret của workspace thật vào repository. Không cần truy cập provider thật để tái hiện các nhánh lỗi.

## 3. Code baseline và lỗ hổng cần xử lý

Đường dẫn dưới đây tương đối với `vscode_tasks_menu_go/`, trừ khi ghi khác. Tên module mới chỉ là đề xuất, ưu tiên tái sử dụng kiến trúc hiện có.

| Source hiện tại | Nhận định đã kiểm tra / hướng cần xác minh |
|---|---|
| `internal/server/agent_chat_collection_flow.go` | `for round := 0; ; round++`; fingerprint cache tối đa 64, đếm trùng trong lần gọi; không đo độ mới của bằng chứng hoặc tiến độ xuyên restart. |
| `internal/server/agent_collection_repair.go` | Có fingerprint chuẩn hóa và repair budget 4; `COLLECTION_NO_PROGRESS` thuộc nhóm repair; không dùng repair count như chỉ số tiến độ toàn task. |
| `internal/server/agent_chat_continuation.go` | No-progress check giữa completed turns; không bao phủ đầy đủ vòng COLLECT nội bộ. |
| `internal/server/agent_run_collection.go`, `agent_run_continuation.go` | Unified Run đã có repair/evidence/turn continuation riêng; cần đồng bộ guard, không giả định hai pipeline giống hệt nhau. |
| `internal/server/agent_chat_response_helpers.go` | Đã nhắc làm patch nhỏ, tránh đọc lại; chưa có điều phối dựa trên tiến độ trong mỗi chuỗi đọc. |
| `internal/agentchat/contextcompact/compact.go` | Giữ tool pair mới, catalog hóa PATCH/COLLECT cũ; mặc định collection budget 30%; current receipt không được âm thầm cắt bỏ. Cần đo request cuối sau serialize và cải thiện catalog. |
| `internal/server/agent_chat_retry.go` | 600s/attempt, retry 10s; tối đa 3 context recovery; unlimited retry có thể lặp cùng payload timeout. |
| `internal/server/agent_chat_adaptive_budget.go` | Cache input/output budget trong RAM, TTL 24h, chỉ học sau explicit rejection rồi accepted. Cần persistence có scope và giới hạn. |
| `internal/server/agent_chat_messages.go` | System instructions/contract build lại; AI Pack chỉ build khi history trống. Cần xử lý stale pack trong conversation cũ mà giữ canonical history. |
| `internal/server/agent_chat_collection_recovery.go`, `agent_chat_collection_auto_resume.go`, `agent_chat_provider_resume.go` | Recovery an toàn cho một số acknowledged/local interruption/prepared; không được tự replay unknown egress. Cần đường tiếp tục hợp lệ cho legacy. |
| `internal/server/agent_chat_tool_feedback.go`, `agent_chat_auto_feedback.go` | ACK của PATCH đã có thể tồn tại trước lỗi COLLECT; cần phân biệt failure domain và hiển thị đúng. |

Giữ regression `TestAgentCollectionContinuesWithoutRoundLimitToPatchProposal` (55 lượt đọc rồi PATCH) và các test duplicate/alternating reads, schema repair, full-file proof, retry unlimited, provider journal recovery. Không làm yếu test chỉ để đạt PASS.

## 4. Hợp đồng thiết kế và invariants

1. **Không đặt hard cap trên tổng số COLLECT hợp lệ.** Task nghiên cứu/reverse engineering/audit có thể cần hàng chục/hàng trăm lượt đọc. Không ép tạo PATCH vô nghĩa. Chỉ checkpoint/đổi chiến lược khi có bằng chứng không tiến triển hoặc giới hạn kỹ thuật/safety thật sự.
2. Phân biệt intent `implementation`, `read_only`, `mixed`, `unknown` dựa vào yêu cầu hiện tại, task plan và quyền người dùng; không suy ra quyền sửa code từ model metadata, keyword đơn lẻ hoặc file không tin cậy. `unknown` dùng khuyến cáo trung tính.
3. Đo **verified evidence novelty** và **task progress** riêng: đọc mới hữu ích không đồng nghĩa source thay đổi/acceptance đạt. Hash của artifact path/call ID/timestamp khác không phải bằng chứng mới. PATCH proposal không phải PATCH executed; wrapper PASS không phải test PASS; PATCH FAIL có thể đã thay đổi source.
4. Mỗi scope có owner/project/workspace, conversation, objective/step/generation; run/parent-turn/call ID chỉ dùng truy vết và idempotency. Không chia sẻ evidence giữa workspace/worktree/owner không tương thích; không xóa lịch sử khi đổi scope. Sau mutation, invalidate coverage có liên quan; nếu không biết phạm vi thì conservatively invalidate.
5. Native COLLECT/PATCH vẫn là thực thi duy nhất; guard chỉ phân tích metadata, admission/feedback/checkpoint, không tự đọc source thay native tool, không tự tạo PATCH. Không thay đổi contract schema hoặc bypass identity/approval.
6. Journal phải giữ nguyên native receipt và lịch sử canonical. Metadata guard versioned, bounded, crash-safe, idempotent; không tăng event count hai lần vì retry/restart. Không ghi prompt/source/secrets thừa vào debug hoặc cache.
7. Retry request khác payload chỉ khi có cơ sở protocol/consent an toàn. Không replay unknown mutating run hoặc gửi lại ACKed PATCH như kết quả mới. Prepared/sending/acknowledged/unknown/completed phải giữ đúng nghĩa. Unknown không được đổi thành acknowledged chỉ để unblock.
8. Bảo toàn user Stop, quyền Auto Collection/Auto Patch/Auto Feedback/Auto Continue, các provider/model binding, capacity fence, TaskPlan acceptance và khả năng chạy đồng thời. Không tạo vòng auto resume vô hạn.

### 4.1. Soft thresholds ban đầu (đề xuất, phải test và tune)

| Tình huống | Hành vi |
|---|---|
| 5 COLLECT kể từ PATCH hoặc checkpoint có tiến độ | Nhắc nhẹ, chỉ nếu objective implementation/mixed: đánh giá đã đủ dữ liệu cho bước nhỏ chưa. |
| 10 COLLECT | Yêu cầu nêu next objective và exact missing scope; ưu tiên bounded collect/patch nếu phù hợp. |
| 3 observation liên tiếp không có novelty đã xác minh | Cảnh báo thay chiến lược; UNKNOWN novelty không được xem là chắc chắn trùng. |
| Sau warning vẫn không tiến triển, lặp nhiều lần | Checkpoint bền vững, rebase context, thử một continuation có chiến lược khác nếu được phép; nếu vẫn stall thì hiển thị blocker có thể resume. |
| Có novelty hoặc tiến độ milestone mới | Cho phép tiếp tục, reset streak không tiến triển tương ứng. |
| Read-only/audit | Không áp ngưỡng bắt buộc PATCH; chỉ cảnh báo redundancy hoặc kỹ thuật. |

Các số 5/10/3 là giả định thiết kế ban đầu, không phải giá trị đã xác nhận tối ưu. Cấu hình central, có hysteresis/cooldown để không nhắc mọi lượt; giới hạn recovery theo objective/generation, không reset bằng cách đổi call ID. Không tự coi lượt đọc thứ 11 là lỗi.

### 4.2. Luồng xử lý đề xuất

```text
Provider response → validate/bind native call → progress precheck
  → native COLLECT (hoặc truthful diagnostic nếu duplicate đã chứng minh)
  → durable observation + guard update (idempotent)
  → evidence novelty / objective progress / policy decision
  → continue | advisory | targeted recovery | resumable checkpoint
  → build compact provider context + exact current receipt
  → provider egress (existing journal and retry rules)
  → PATCH proposal goes through existing approval / execution / receipt path
```

Không bỏ qua function output của call đã nhận. Nếu checkpoint trước khi chạy tool, ghi rõ NOT_EXECUTED/REJECTED và đóng pending call đúng protocol. Nếu không chắc chắn tool đã chạy hay chưa, bảo toàn unknown và reconciliation, không tạo kết quả giả.

## 5. Implementation checklist — 3 cấp: 10 phase / 36 task / 144 subtask

### CLP-00 — Baseline, audit và nền kiểm thử (3 task, P1)

- [ ] **Phase gate** — `TODO`; chỉ đánh dấu khi tất cả task cấp 2 DONE và remote delivery đã xác minh.

- [ ] **CLP-00.01 — Reconcile HEAD và bản đồ pipeline** (`IN_PROGRESS`, P1)
  - Đọc diff từ baseline đến HEAD, các handoff 13–19 và entrypoints Legacy/Unified. Xác định store, snapshot, provider retry, native evidence, UI API thực tế. Ghi source paths/line refs và các thay đổi đã có vào nhật ký. Xác định exact command test/build của repo và quyền môi trường.
  - **Acceptance:** có sơ đồ entrypoint → collect → journal → provider → patch → continuation cho cả hai pipeline; các guard hiện có và gaps được phân biệt; không reset repo.
    - [ ] **CLP-00.01.01** — Fetch origin/main, xác định HEAD/dirty files và diff với baseline; ghi commit thực tế, không reset. (`BLOCKED` — remote HEAD/diff confirmed, local dirty files/fetch unavailable)
    - [ ] **CLP-00.01.02** — Trace Legacy từ message/feedback qua collection flow, provider journal, Patch gateway và Auto Continue; ghi file/function. (`IN_PROGRESS`)
    - [ ] **CLP-00.01.03** — Trace Unified từ RunSpec/snapshot đến waiting-tool resolver, scheduler, receipt và TaskPlan; lập bảng khác biệt. (`IN_PROGRESS`)
    - [ ] **CLP-00.01.04** — Đối chiếu handoff 13–19, CI/Makefile/go.mod, quyền Git/test và các fix đã merge; cập nhật dependency map. (`IN_PROGRESS`)
- [x] **CLP-00.02 — Synthetic incident replay fixture** (`DONE` — verified synthetic four-case incident CI 45583866679)
  - Dùng fake provider/native collector và journal fixture dạng legacy, chuỗi 12/7/8 COLLECT, 2 ACKed PATCH, schema invalid, timeout, HTTP 400, cancellation. Không ngủ 600s; dùng fake clock/injected delay. Test old behavior trước khi sửa, không chứa source thật.
  - **Acceptance:** tái hiện stall/unknown với ACK PATCH giữ nguyên; ghi rõ phần mô phỏng và các giả định; fixture deterministic, không gọi network.
    - [x] **CLP-00.02.01** — Tạo fixture journal/session cũ với 12/7/8 COLLECT, hai PATCH ACK và metadata redacted giả lập. (`DONE` — 12/7/8 seeded history, two acknowledged PATCH receipts; named CI PASS)
    - [x] **CLP-00.02.02** — Inject fake native collector + provider, tạo lỗi grep, 400 context, timeout, cancellation bằng fake clock. (`DONE` — mock provider/collector, injected 1ms delay, four variants PASS)
    - [x] **CLP-00.02.03** — Viết assertions cho sequence, ACK, unknown phase, payload size và native execution count; không dùng mạng. (`DONE` — exact receipt/ACK/sequence/read-count/payload assertions PASS)
    - [x] **CLP-00.02.04** — Chạy fixture baseline, lưu observed behavior và đánh dấu phần synthetic không phải bằng chứng production. (`DONE` — CI 45583866679, synthetic bytes 330572/946632/409907/409907; not production)
- [x] **CLP-00.03 — Baseline regression và invariants** (`DONE` — baseline regression CI 45583368471 and 45583866679 SUCCESS)
  - Chạy targeted tests hiện có: collection, repair, native proof, compact, retry, session/store, provider recovery, continuation, plan acceptance. Lưu kết quả thực tế và lỗi đã tồn tại.
  - **Acceptance:** có baseline test command/exit code; test chưa chạy phải `VERIFY_PENDING`/`BLOCKED`, không đánh dấu PASS giả. Không chỉnh test unrelated để che lỗi.
    - [x] **CLP-00.03.01** — Liệt kê test hiện có và giữ baseline 55 unique reads → PATCH; xác nhận test names thực sự match. (`DONE` — pinned source test inventory confirmed; unchanged 55-read regression)
    - [x] **CLP-00.03.02** — Chạy suite collection/schema/native proof/compact trên baseline và ghi exit code. (`DONE` — core CI including collection/schema/native proof/compact PASS)
    - [x] **CLP-00.03.03** — Chạy retry/recovery/session/store/continuation/TaskPlan acceptance; ghi failures sẵn có. (`DONE` — core CI including retry/recovery/store/continuation/acceptance PASS)
    - [x] **CLP-00.03.04** — Ghi môi trường, lệnh, kết quả và blockers; không sửa test để che regression. (`DONE` — CI receipts, commands, original failures and local limitations recorded)

### CLP-01 — Shared Collection Progress Guard (5 task, P1)

- [ ] **Phase gate** — `TODO`; chỉ đánh dấu khi tất cả task cấp 2 DONE và remote delivery đã xác minh.

- [x] **CLP-01.01 — Shared policy/module và typed decision** (`DONE`, P1)
  - Tạo module dùng chung (ví dụ `internal/agentchat/collectionprogress` nếu phù hợp dependency graph). Định nghĩa `State`, `Observation`, `Decision`, policy version, threshold, reason codes và intent. Pure decision logic, không phụ thuộc HTTP/UI/collector. Tái sử dụng fingerprint hiện có.
  - **Acceptance:** unit test state transitions, 5/10 advisory, cooldown, read-only/unknown; không đặt hard cap số lượt đọc.
    - [x] **CLP-01.01.01** — Thiết kế package pure logic với State/Observation/Decision, reason codes, policy version; tránh dependency cycle. (`DONE` — isolated unit/race PASS; exact remote blobs verified)
    - [x] **CLP-01.01.02** — Cài decision function tách verified novelty, objective progress và unknown; không cấp quyền tool. (`DONE` — isolated unit/race PASS; exact remote blobs verified)
    - [x] **CLP-01.01.03** — Cài soft thresholds 5/10/3 và cooldown; không hard cap reads. (`DONE` — isolated unit/race PASS; exact remote blobs verified)
    - [x] **CLP-01.01.04** — Viết table-driven unit tests cho transitions, read-only/unknown, threshold boundary và long reads. (`DONE` — isolated unit/race PASS; exact remote blobs verified)
- [ ] **CLP-01.02 — Objective binding và intent** (`IN_PROGRESS`, P1)
  - Bind scope vào current task/user intent/TaskPlan step/generation; đổi task, user intervention, plan revision, worktree change phải có transition rõ ràng. Model hints chỉ là advisory. Không reset toàn bộ counters chỉ vì model/provider đổi.
  - **Acceptance:** coding/read-only/mixed/unknown, task switch, user Stop, no plan, task plan pending/accepted được test; không nâng quyền mutating.
    - [x] **CLP-01.02.01** — Xác định objective key từ scope owner/workspace/conversation + user goal/plan step/generation. (`DONE` — scope helper and unit tests pass in 45583866679; runtime binding pending)
    - [x] **CLP-01.02.02** — Xây intent classification bảo thủ, dựa trên user-owned goal và permissions, không dựa duy nhất vào model. (`DONE` — conservative advisory-only classifier and tests PASS in 45583866679; runtime input provenance pending)
    - [ ] **CLP-01.02.03** — Cài transitions khi đổi goal/step, user intervention, worktree, provider; không reset tùy tiện. (`TODO`)
    - [ ] **CLP-01.02.04** — Test no-plan, read-only/mixed/unknown, provider switch, Stop và permission không tăng. (`TODO`)
- [ ] **CLP-01.03 — Guard counters và hysteresis** (`IN_PROGRESS`, P1)
  - Đếm requested/observed/verified-new/duplicate/unknown/error, streak since progress, since last PATCH proposal/execution/confirmed progress riêng, recovery count, warning watermark. Dedupe logical observation; retry HTTP không phải COLLECT mới. Không reset no-progress vì proposal fail/no-op.
  - **Acceptance:** test duplicates, retry, rejected/no-op PATCH, actual change, read-only progress, threshold không spam.
    - [x] **CLP-01.03.01** — Định nghĩa counters riêng requested, observed, verified-new, duplicate, unknown, error, HTTP attempts. (`DONE` — pure counters; full core CI 45584061420 PASS; runtime adapter pending)
    - [x] **CLP-01.03.02** — Cài streak/watermark/cooldown, phân biệt last PATCH proposal, executed, meaningful progress. (`DONE` — independent counters/hysteresis verified in CI 45584061420; runtime receipt adapter pending)
    - [ ] **CLP-01.03.03** — Đảm bảo rejected/no-op/partial PATCH và retry không reset sai; dùng receipt thật. (`VERIFY_PENDING` — real native PATCH ledger adapter; HTTP attempt integration pending)
    - [ ] **CLP-01.03.04** — Test idempotence, boundaries, repeated warnings và progress reset hợp lệ. (`TODO`)
- [ ] **CLP-01.04 — Checkpoint và bounded recovery policy** (`IN_PROGRESS`, P1)
  - Tạo decisions `CONTINUE`, `ADVISE`, `REPLAN`, `CHECKPOINT` (tên có thể điều chỉnh). Recovery dùng tiếp continuation framework đã có; phân biệt stalled với user stop/unknown delivery. Ghi next action và reason; không tự báo completed.
  - **Acceptance:** sau nhiều lượt không novelty, tạo checkpoint và tối đa số recovery cho phép theo generation; khi có bằng chứng mới thì tiếp tục; không mất tool pair.
    - [x] **CLP-01.04.01** — Cài decisions continue/advisory/replan/checkpoint với reason và next action có schema bounded. (`DONE` — pure bounded decisions/reasons/actions verified in CI 45584362259)
    - [ ] **CLP-01.04.02** — Nối recovery generation/budget vào framework continuation, giữ stop/unknown là trạng thái riêng. (`VERIFY_PENDING` — Legacy verified receipt checkpoint/replan; CI and Unified pending)
    - [x] **CLP-01.04.03** — Định nghĩa cách đóng pending call thật khi checkpoint, không invent output hoặc mark task done. (`DONE` — exact native output committed before recovery and preserved at checkpoint; named test PASS 45587765715)
    - [ ] **CLP-01.04.04** — Test stall lặp, recovery tối đa, novelty xuất hiện lại, pending pair và no continuation storm. (`TODO`)
- [ ] **CLP-01.05 — Policy configuration và compatibility** (`IN_PROGRESS`, P1)
  - Central defaults + validate range; không rải magic numbers. Dùng internal config trước nếu UI không cần chỉnh. Policy version/snapshot phù hợp unified run; tránh thay policy giữa active immutable run.
  - **Acceptance:** legacy session thiếu config dùng defaults an toàn; invalid values không phá workflow; 55+ unique reads hợp lệ vẫn chạy.
    - [x] **CLP-01.05.01** — Đặt defaults tập trung, validate ranges/cooldown và version; ghi policy semantics. (`DONE` — central threshold validation and version; CI 45584362259 PASS; snapshot integration pending)
    - [ ] **CLP-01.05.02** — Áp dụng defaults an toàn cho session cũ; cấu hình immutable snapshot cho Run. (`TODO`)
    - [ ] **CLP-01.05.03** — Test invalid config, restart, policy change giữa active run và 55+ unique reads. (`TODO`)
    - [ ] **CLP-01.05.04** — Viết tài liệu tuning; không biến thresholds thành quota hoặc giới hạn tổng. (`TODO`)

### CLP-02 — Evidence novelty, overlap và invalidation (4 task, P1)

- [ ] **Phase gate** — `TODO`; chỉ đánh dấu khi tất cả task cấp 2 DONE và remote delivery đã xác minh.

- [ ] **CLP-02.01 — Canonical scope descriptor** (`IN_PROGRESS`, P1)
  - Tái sử dụng canonical request fingerprint; chuẩn hóa project-relative path, action semantics, source range/symbol/query, relevant limits/coverage, source revision khi có proof. Không bỏ các tham số ảnh hưởng thực thi hoặc authorization. Không tự mở file ngoài native collector.
  - **Acceptance:** title/call ID khác không né duplicate; action/limits có ý nghĩa khác không bị gộp sai; paths/worktree/identity boundary được test.
    - [x] **CLP-02.01.01** — Audit fingerprint hiện tại, giữ tham số action/limits/project/authorization ảnh hưởng thực thi. (`DONE` — shared existing fingerprint audited and tested, CI 45584606033 PASS)
    - [ ] **CLP-02.01.02** — Định nghĩa scope descriptor cho path/range/symbol/query/worktree/source version theo metadata đáng tin. (`VERIFY_PENDING` — shared fingerprint/scope descriptor tests PASS, CI 45584606033; runtime scope binding still pending)
    - [x] **CLP-02.01.03** — Canonicalize an toàn, loại title/call ID volatile; không tự truy cập filesystem ngoài native COLLECT. (`DONE` — canonical request fingerprint and malformed/duplicate key tests, CI 45584606033 PASS)
    - [x] **CLP-02.01.04** — Test equivalence, distinct limits, path/identity isolation và invalid inputs. (`DONE` — scope equivalence, distinct limits, identity and invalid input tests, CI 45584606033 PASS)
- [ ] **CLP-02.02 — Evidence index từ native receipt** (`IN_PROGRESS`, P1)
  - Dùng manifest/proof/native observation metadata đã xác minh. Lưu bounded index theo logical scope, content digest/coverage/partial status; loại volatile artifact paths/timestamps khỏi novelty. Không hash toàn bộ source lặp lại chỉ để guard.
  - **Acceptance:** exact duplicate, same content/different artifact, new range/symbol, incomplete/truncated, missing metadata đều phân biệt; unknown không thành verified-no-progress.
    - [x] **CLP-02.02.01** — Đọc cấu trúc native receipt/manifest/proof; xác định fields thực sự available ở cả hai pipeline. (`DONE` — native common receipt/manifest/full-file proof fields traced; native adapter tested)
    - [ ] **CLP-02.02.02** — Tạo adapter phân loại complete/partial/unknown/error và lấy digest/coverage chỉ khi proof hợp lệ. (`VERIFY_PENDING` — full-file proof tests PASS, CI 45584790677; partial/unsupported unknown; runtime adapter pending)
    - [x] **CLP-02.02.03** — Cài bounded index theo logical scope, loại timestamp/artifact run path khỏi novelty. (`DONE` — bounded evidence index with volatile artifact IDs excluded; package CI PASS and Legacy integration fixture PASS)
    - [ ] **CLP-02.02.04** — Test same content khác artifact, new range/symbol, missing/truncated metadata; unknown không thành duplicate. (`VERIFY_PENDING` — pure evidence tests PASS, CI 45584790677; native range/symbol tests pending)
- [ ] **CLP-02.03 — Overlap/subset/alternating read detection** (`IN_PROGRESS`, P1)
  - Nhận diện range overlap/subset và chuỗi A/B/A/B, query đổi nhẹ nếu chứng minh output/coverage tương đương. Chỉ reject/skip nếu có proof tương thích và source còn hợp lệ; nếu heuristic thì advisory, không ngăn đọc.
  - **Acceptance:** A[1..100]→A[1..110] tính phần mới đúng; 55+ trang khác nhau không bị chặn; metadata thiếu không sinh false positive; test scope eviction.
    - [x] **CLP-02.03.01** — Tính interval union/subset/overlap và phần coverage mới trên cùng source revision. (`DONE` — verified same-revision union/subset/overlap pure tests passed in core Go suite 45587765715; native range adapter pending)
    - [ ] **CLP-02.03.02** — Nhận diện A/B/A/B, query biến thể với proof tương đương; heuristic chỉ advisory. (`TODO`)
    - [ ] **CLP-02.03.03** — Tách skip đã chứng minh khỏi warning; khi proof stale/thiếu phải cho đọc hợp lệ. (`VERIFY_PENDING` — Legacy proofless skip removed; Unified pending)
    - [ ] **CLP-02.03.04** — Test A1–100→A1–110, 55+ trang, eviction, changed content và false-positive cases. (`TODO`)
- [ ] **CLP-02.04 — Source mutation invalidation & resource bounds** (`IN_PROGRESS`, P1)
  - Invalidate coverage sau PATCH hoặc external change/worktree drift; dùng receipt changed-files nếu đáng tin, fallback invalidate rộng khi mutation unknown/partial. Bounded memory/index size, TTL hoặc eviction; không làm mất lịch sử gốc.
  - **Acceptance:** đọc lại sau sửa file không bị chặn sai; failed partial patch, external edit, rename/delete, source lớn ~150 MB; benchmark metadata bounded, không thêm full-source scan.
    - [ ] **CLP-02.04.01** — Nối invalidation với native PATCH receipt, changed files và external/worktree drift signal. (`VERIFY_PENDING` — session CAS native ledger transitions; external drift/path adapter pending)
    - [ ] **CLP-02.04.02** — Invalidate theo path khi proof đầy đủ, scope rộng khi partial/unknown mutation; xử lý rename/delete. (`VERIFY_PENDING` — pure invalidation; authoritative mutation adapter pending)
    - [ ] **CLP-02.04.03** — Cài TTL/eviction/size cap và benchmark metadata; không scan/hash lại toàn source. (`VERIFY_PENDING` — bounded metadata index; benchmark pending)
    - [ ] **CLP-02.04.04** — Test read-after-patch, failed partial patch, external edit, 150MB source và bounded overhead. (`TODO`)

### CLP-03 — Durable state, restart và cross-turn (3 task, P1)

- [ ] **Phase gate** — `TODO`; chỉ đánh dấu khi tất cả task cấp 2 DONE và remote delivery đã xác minh.

- [ ] **CLP-03.01 — Versioned durable guard state** (`IN_PROGRESS`, P1)
  - Chọn authoritative storage phù hợp session/run/snapshot; thêm fields additive, migration backward-compatible, revision/CAS + existing locks. Scope và event keys rõ ràng. Không tạo database/journal cạnh tranh với store hiện tại.
  - **Acceptance:** save/load/round-trip, legacy missing fields, unknown future version fail safe, concurrent update và crash consistency tests.
    - [ ] **CLP-03.01.01** — Chọn authoritative session/run store; thiết kế additive versioned state và migration defaults. (`VERIFY_PENDING` — session CAS metadata implementation; Unified snapshot pending)
    - [ ] **CLP-03.01.02** — Tích hợp revision/CAS/locks; persist guard state và scope, tránh competing journal. (`VERIFY_PENDING` — Session.Save uses existing CAS; CI pending)
    - [x] **CLP-03.01.03** — Xử lý missing fields/corrupt state/future version fail-safe, không mất canonical history. (`DONE` — opaque unknown/corrupt guard preserved and canonical session remains readable; agentchat race suite PASS 45587765715)
    - [ ] **CLP-03.01.04** — Test round-trip, legacy load, concurrent save, failure injection và atomicity. (`VERIFY_PENDING` — round-trip/stale CAS/opaque tests added; crash injection pending)
- [ ] **CLP-03.02 — Exactly-once logical observation** (`IN_PROGRESS`, P1)
  - Persist observation và guard transition tại checkpoint hiện có; nếu phải hai bước thì reconcile idempotently sau crash. Dedupe bằng scoped logical call/receipt identity; không double-count retry, reconcile hoặc reload.
  - **Acceptance:** crash trước/sau native run, receipt save, provider egress; restart và auto continuation giữ streak; không replay native call do guard.
    - [ ] **CLP-03.02.01** — Xác định logical observation key từ scoped call/receipt identity, không dùng HTTP attempt. (`VERIFY_PENDING` — Legacy stable event sequence plus run/call binding; full recovery adapter pending)
    - [ ] **CLP-03.02.02** — Gắn guard transition vào durable observation checkpoint; reconcile hai bước nếu transaction không chung. (`VERIFY_PENDING` — Legacy request and observation same CAS checkpoints; Unified pending)
    - [ ] **CLP-03.02.03** — Cài idempotent replay metadata sau crash; không rerun native action hoặc resend ACK. (`TODO`)
    - [ ] **CLP-03.02.04** — Test crash trước/sau tool, receipt, egress; retry/reconcile/reload chỉ count một lần. (`TODO`)
- [ ] **CLP-03.03 — Legacy hydration và bounded history** (`IN_PROGRESS`, P1)
  - Khởi tạo lazy từ journal/canonical history/archived metadata sẵn có; phân biệt historical known vs unknown, không fabricate novelty. History compacted không làm mất guard. Bound scan, không đọc toàn bộ archive mỗi request.
  - **Acceptance:** fixture session 20/11, partial journal/archived pairs, migration idempotent, restart/self-update giữ counters và original history; nhiều conversations không lẫn state.
    - [x] **CLP-03.03.01** — Thiết kế bounded hydration từ journal/history/archive metadata hiện có. (`DONE` — bounded retained-history migration design and round-trip test PASS; archived detailed hydration still pending)
    - [ ] **CLP-03.03.02** — Khôi phục known counters/ACKs; đánh dấu unknown history không đủ proof; không đoán novelty. (`VERIFY_PENDING` — historical counts separate from current objective)
    - [ ] **CLP-03.03.03** — Persist migration marker và cursor; không rescan archive mỗi request, không sửa history. (`TODO`)
    - [ ] **CLP-03.03.04** — Test legacy fixture, partial archive, repeated migration, restart/self-update và isolation. (`TODO`)

### CLP-04 — Tích hợp Agent Chat và Agent Run (4 task, P1)

- [ ] **Phase gate** — `TODO`; chỉ đánh dấu khi tất cả task cấp 2 DONE và remote delivery đã xác minh.

- [ ] **CLP-04.01 — Legacy Agent Chat inner loop** (`IN_PROGRESS`, P1)
  - Tích hợp guard trong `agent_chat_collection_flow.go` trước native execution và sau observation/receipt; refresh settings theo logic hiện có. Duy trì canonical tool call/output, journal phases và repair budget.
  - **Acceptance:** 27-round fixture nhận advisory/checkpoint đúng policy, duplicate guard cũ vẫn đúng, cancellation và disabled COLLECT không bị bypass; 55 rounds unique → PATCH PASS test logic giữ nguyên.
    - [x] **CLP-04.01.01** — Gắn guard precheck sau validation/permission trong legacy collection loop. (`DONE` — Legacy guard entry after existing permission check; server suite PASS 45587765715)
    - [x] **CLP-04.01.02** — Gắn post-observation decision vào checkpoint trước provider egress, giữ original output và repair. (`DONE` — Legacy counters and advisory at existing durable receipt checkpoint, exact output test PASS 45587765715)
    - [x] **CLP-04.01.03** — Nối warning/checkpoint/cancel vào existing state transitions; không nuốt pending tool pair. (`DONE` — Legacy checkpoint/replan preserves ACK and exact call/output; named test PASS 45587765715)
    - [ ] **CLP-04.01.04** — Test incident 27 reads, duplicate, disabled collection, cancellation và 55 reads → PATCH. (`TODO`)
- [ ] **CLP-04.02 — Unified Agent Run waiting-tool path** (`TODO`, P1)
  - Tích hợp cùng guard trong `agent_run_collection.go`, authoritative run snapshot/parent turn và scheduler. Không lấy state mutable của session làm thay đổi immutable run binding. Ensure parity với legacy theo cùng input semantics.
  - **Acceptance:** typed/text fallback, run waiting tool, denied tools, partial receipt, restart, concurrency; cùng scenario cho ra decision tương đương khi policy/permission giống nhau.
    - [ ] **CLP-04.02.01** — Gắn shared guard vào Unified waiting-tool resolver sau authorization/identity checks. (`TODO`)
    - [ ] **CLP-04.02.02** — Bind run snapshot/parent turn, persist evidence transition với scheduler checkpoint. (`TODO`)
    - [ ] **CLP-04.02.03** — Xử lý typed/text fallback, denied calls, partial result và restart không đổi immutable binding. (`TODO`)
    - [ ] **CLP-04.02.04** — Test parity với legacy, concurrent runs/worktrees, lease/capacity và native execution count. (`TODO`)
- [ ] **CLP-04.03 — Coordination với Auto Continue/TaskPlan** (`TODO`, P1)
  - Nối checkpoint guard vào `agent_chat_continuation.go`, `agent_run_continuation.go`, task-plan coordinator và disposition. Không tăng gấp đôi no-progress giữa inner loop và outer turn; dùng objective/step acceptance thật.
  - **Acceptance:** stall không tạo continuation storm, checkpoint có next action; chưa đạt evidence/acceptance không mark done; read-only progress không bị nhầm là coding stall.
    - [ ] **CLP-04.03.01** — Trace no-progress signatures và TaskPlan acceptance ở cả hai continuation paths. (`TODO`)
    - [ ] **CLP-04.03.02** — Thống nhất ownership counters inner/outer, không double count hoặc reset do provider completed. (`TODO`)
    - [ ] **CLP-04.03.03** — Nối checkpoint, reason, next action và auto-continue policy; bounded replan theo generation. (`TODO`)
    - [ ] **CLP-04.03.04** — Test stall storm, read-only progress, pending plan step và verified/attested acceptance. (`TODO`)
- [ ] **CLP-04.04 — Model-visible bounded feedback** (`IN_PROGRESS`, P1)
  - Tạo advisory ngắn, có structured reason, counts, missing evidence và next strategy; giữ exact receipt gốc riêng, không sửa nội dung bằng chứng để chèn cảnh báo. Tôn trọng output budget, prompt untrusted tool data boundary và fallback protocol.
  - **Acceptance:** model mock nhận warning sau threshold, có thể chọn targeted COLLECT hoặc submit PATCH; read-only không bị ép patch; no fabricated tool output; metadata không cấp quyền.
    - [ ] **CLP-04.04.01** — Thiết kế advisory schema ngắn với reason, counters, verified/missing scope và next strategy. (`TODO`)
    - [ ] **CLP-04.04.02** — Tách advisory khỏi original receipt; giữ exact call-output pair và trusted instruction boundary. (`TODO`)
    - [ ] **CLP-04.04.03** — Tạo variant coding/read-only/unknown, output bounded, typed/text protocol tương thích. (`TODO`)
    - [ ] **CLP-04.04.04** — Mock provider nhận warning rồi targeted read hoặc PATCH; test no forced patch/no fabricated output. (`TODO`)

### CLP-05 — Context compact và stale AI Pack (4 task, P1)

- [ ] **Phase gate** — `TODO`; chỉ đánh dấu khi tất cả task cấp 2 DONE và remote delivery đã xác minh.

- [ ] **CLP-05.01 — Audit final payload budget** (`TODO`, P1)
  - Đo budget sau serialize thực tế: instructions, schema/tools, current receipt, mandatory user goal, history, reserved output/reasoning; phân biệt bytes và estimated tokens. Kiểm tra duplicate item giữa History/Next và legacy fallback prefix; dùng estimator/tokenizer phù hợp nếu có.
  - **Acceptance:** regression 304 KB→942 KB synthetic; tránh gửi duplicate tool pair; nếu mandatory input vượt giới hạn thì báo cụ thể, không âm thầm bỏ receipt; test cả Responses/Chat Completions, stateful rebase.
    - [ ] **CLP-05.01.01** — Instrument final serialized payload bytes/estimated tokens và reserved output; breakdown theo input component. (`TODO`)
    - [ ] **CLP-05.01.02** — Audit History/Next duplicate, schema overhead, initial pack và legacy fallback grouping. (`TODO`)
    - [ ] **CLP-05.01.03** — Thêm final budget validation cho Responses/Chat Completions/rebase, explicit mandatory-overflow reason. (`TODO`)
    - [ ] **CLP-05.01.04** — Test synthetic 304→942 KB, no duplicate pair, mandatory receipt intact và estimator error bounds. (`TODO`)
- [ ] **CLP-05.02 — Evidence catalog có ích cho bước tiếp theo** (`IN_PROGRESS`, P1)
  - Mở rộng compact catalog bounded: objective, canonical scope, coverage/status, hash/proof ref, exact missing scope, invalidation marker. Giữ current receipt byte-exact; prior PATCH metadata only; recent COLLECT theo budget. Đảm bảo legacy text fallback được phân loại đúng.
  - **Acceptance:** prior evidence có thể reuse không đọc lại toàn file, incomplete không thành success, old patch không resend source, no tool orphan; test compact/rebase nhiều ngày.
    - [ ] **CLP-05.02.01** — Mở rộng catalog metadata objective/scope/coverage/status/proof/hash/invalidation; bound size. (`TODO`)
    - [x] **CLP-05.02.02** — Giữ current receipt byte-exact, prior PATCH metadata-only, recent COLLECT theo budget. (`DONE` — existing receipt/provenance regression and new compact tests PASS in 45583866679)
    - [ ] **CLP-05.02.03** — Nhận diện đúng legacy fallback prefix, orphan pairs và stale evidence; không mark incomplete success. (`TODO`)
    - [ ] **CLP-05.02.04** — Test long/multiday compact, rebase, catalog reuse và no lost user goal. (`TODO`)
- [ ] **CLP-05.03 — AI Pack/contract refresh cho conversation cũ** (`TODO`, P1)
  - Version/fingerprint pack/schema; current trusted system/tool contract authoritative. Tách historical pack khỏi user intent, render compatibility summary/rebase tại request mới; không rewrite canonical history hoặc refresh nguyên pack mỗi lượt. Không silently regrant permission.
  - **Acceptance:** old AI Pack chứa schema cũ (`grep`/legacy forms) không lấn current contract; current user goal và attachments/evidence không mất; legacy conversation tiếp tục không cần tạo mới.
    - [ ] **CLP-05.03.01** — Version/fingerprint current contract và initial AI Pack; detect stale pack trong request projection. (`TODO`)
    - [ ] **CLP-05.03.02** — Tách historical instructions khỏi user goal/attachments; current trusted schema authoritative. (`TODO`)
    - [ ] **CLP-05.03.03** — Render compatibility summary/rebase không rewrite journal, không resend full pack mỗi lượt hay regrant permission. (`TODO`)
    - [ ] **CLP-05.03.04** — Test old grep schema, legacy forms, stale stateful IDs và same conversation continuation. (`TODO`)
- [ ] **CLP-05.04 — Budget-aware next COLLECT và preflight** (`TODO`, P1)
  - Nếu budget pressure cao, hướng model chọn scoped actions/limits; tận dụng native excerpt/full-artifact refs. Không cắt native result đã tạo hoặc thay collector. Khi không thể gửi exact current receipt trong limit, checkpoint và báo giới hạn/đường tiếp tục hợp lệ.
  - **Acceptance:** request hợp lệ trong configured budget hoặc bounded explicit error; large source 150 MB targeted read vẫn hỗ trợ; no silent truncation of required current tool receipt.
    - [ ] **CLP-05.04.01** — Tính pressure trước native request và đưa scoped action/limit guidance cho model. (`TODO`)
    - [ ] **CLP-05.04.02** — Tái sử dụng native excerpt/full artifact references; không thêm collector hoặc cắt receipt đã tạo. (`TODO`)
    - [ ] **CLP-05.04.03** — Nếu mandatory input không fit, đóng checkpoint đúng protocol với diagnostic rõ ràng. (`TODO`)
    - [ ] **CLP-05.04.04** — Test 150MB targeted read, hard technical overflow, no silent receipt loss và resume. (`TODO`)

### CLP-06 — Durable adaptive context budget và retry strategy (4 task, P1/P2)

- [ ] **Phase gate** — `TODO`; chỉ đánh dấu khi tất cả task cấp 2 DONE và remote delivery đã xác minh.

- [ ] **CLP-06.01 — Persistent adaptive budget** (`TODO`, P2)
  - Migrate RAM cache sang bounded durable hints hoặc durable backing của cache; key owner/workspace/provider endpoint+API/model/config version/credential fingerprint (không lưu secret), TTL/eviction, atomic write, schema migration. Learn từ explicit rejection và accepted smaller request; không suy ra context limit từ timeout.
  - **Acceptance:** restart giữ hint trong TTL; change model/endpoint/credential/config không reuse nhầm; invalid/corrupt entry fail safe; concurrency/privacy tests.
    - [ ] **CLP-06.01.01** — Thiết kế durable bounded hint store với scoped endpoint/API/model/config/credential fingerprint, TTL/version. (`TODO`)
    - [ ] **CLP-06.01.02** — Cài atomic persistence, eviction/corrupt-state handling và concurrency; không lưu key/prompt. (`TODO`)
    - [ ] **CLP-06.01.03** — Learn chỉ từ structured rejection rồi accepted smaller request; invalidate khi config thay đổi. (`TODO`)
    - [ ] **CLP-06.01.04** — Test restart, TTL, credential rotation, owner/workspace isolation và privacy. (`TODO`)
- [ ] **CLP-06.02 — Retry payload stagnation telemetry** (`TODO`, P1)
  - Track attempt count, effective payload digest, input/output budget, error class, header-wait vs connect vs read, recovery generation. Retry không tính thành COLLECT mới. Phân loại HTTP 400 structured, 499, timeout, incomplete, cancellation, unknown delivery riêng.
  - **Acceptance:** 3 timeout cùng digest có thể phát hiện; explicit Stop không retry; no secrets/prompt in trace; Retry Off/Limited/Unlimited semantics được giữ.
    - [ ] **CLP-06.02.01** — Track effective digest, attempt/recovery generation, input/output budget và retry mode. (`TODO`)
    - [ ] **CLP-06.02.02** — Phân loại connect/header/read timeout, 499, structured 400, incomplete, cancel, unknown. (`TODO`)
    - [ ] **CLP-06.02.03** — Expose redacted stagnation diagnostic; không count HTTP retry thành native observation. (`TODO`)
    - [ ] **CLP-06.02.04** — Test same digest 3 timeouts, Stop, Off/Limited/Unlimited và no secret leak. (`TODO`)
- [ ] **CLP-06.03 — Bounded strategy adaptation** (`TODO`, P1)
  - Với explicit context rejection, giảm progressive theo cơ chế hiện tại và exact receipt. Với timeout, không tự khẳng định context overflow; khi retry được consent và an toàn, dùng existing retry/backoff; đề xuất request nhỏ hơn/alternate model hoặc checkpoint/new continuation được authorize nếu stagnation. Không biến unknown egress thành safe retry; không lén đổi provider/model. Giữ 600s mỗi attempt, không thêm cumulative time cap cho task hợp lệ.
  - **Acceptance:** timeout lặp không kích hoạt native tool lần hai; context rejection giảm budget, 499/incomplete/cancel đúng policy; unlimited không bị đổi thành limited một cách âm thầm; nếu cần tạm dừng vì safety thì reason rõ và resume hợp lệ.
    - [ ] **CLP-06.03.01** — Giữ 600s/attempt và progressive recovery cho structured context rejection; bảo toàn exact receipt. (`TODO`)
    - [ ] **CLP-06.03.02** — Tách timeout khỏi context overflow; xác định consent/safety trước mọi đổi payload hoặc continuation. (`TODO`)
    - [ ] **CLP-06.03.03** — Khi stagnation, đưa bounded strategy/replan/checkpoint hoặc đề xuất đổi model; không tự đổi provider hay silently cap unlimited. (`TODO`)
    - [ ] **CLP-06.03.04** — Test timeout/499/incomplete/cancel, exact receipt, no duplicate tool, no unsafe replay. (`TODO`)
- [ ] **CLP-06.04 — Legacy/Unified retry parity** (`TODO`, P1)
  - Audit provider router/executor của Unified Run; dùng shared classification/budget hints nơi phù hợp, bảo toàn immutable binding và request journal. Ghi rõ các khác biệt protocol cần giữ.
  - **Acceptance:** matrix hai pipeline × retry modes × errors, no duplicate execution, no unsafe replay, fake clock/network deterministic; current receipt preserved.
    - [ ] **CLP-06.04.01** — Audit Unified provider router/executor retry journal và pinned policy; map khác biệt với Legacy. (`TODO`)
    - [ ] **CLP-06.04.02** — Tái sử dụng shared classification/hints khi tương thích, không mutate immutable RunSpec. (`TODO`)
    - [ ] **CLP-06.04.03** — Đồng bộ telemetry/budget behavior và ghi protocol-specific exceptions. (`TODO`)
    - [ ] **CLP-06.04.04** — Chạy fake-clock matrix hai pipelines × retry modes × errors, assert no duplicate native effect. (`TODO`)

### CLP-07 — Recovery conversation từ build cũ (4 task, P1)

- [ ] **Phase gate** — `TODO`; chỉ đánh dấu khi tất cả task cấp 2 DONE và remote delivery đã xác minh.

- [ ] **CLP-07.01 — Tách PATCH ACK khỏi COLLECT failure** (`TODO`, P1)
  - Sửa diagnostic/admission state để lỗi continuation sau PATCH không bị mô tả là PATCH result delivery unconfirmed nếu PATCH receipt đã ACK. Truy vết actual provider run kind/call ID/phase và ACK receipts.
  - **Acceptance:** fixture hai ACKed PATCH rồi COLLECT unknown giữ nguyên ACKs; auto feedback không resend; UI/diagnostic chỉ đúng failure domain.
    - [ ] **CLP-07.01.01** — Trace provider run kind/call/phase và actual PATCH receipt ACK; tách failure domain. (`TODO`)
    - [ ] **CLP-07.01.02** — Sửa admission/diagnostic không downgrade ACK khi collection continuation fail. (`TODO`)
    - [ ] **CLP-07.01.03** — Cập nhật UI reason/action để không resend acknowledged result; giữ unknown collection. (`TODO`)
    - [ ] **CLP-07.01.04** — Test 2 ACKed PATCH → 8 COLLECT → unknown, including last_request_status=200 không ACK nhầm. (`TODO`)
- [ ] **CLP-07.02 — Safe legacy reconciliation** (`TODO`, P1)
  - Audit prepared/sending/acknowledged/unknown và historical unconfirmed, orphan call/output, pending Patch. Dùng authoritative journal proof để repair chỉ khi đủ bằng chứng; không dựa vào `last_request_status=200` để ACK nhầm run. Không chạy lại executed PATCH.
  - **Acceptance:** restart old session với mixed states, unknown vẫn unknown; prepared-only resume theo existing rules; no fabricated ACK/result; historical unresolved side effect vẫn bị chặn/reconcile.
    - [ ] **CLP-07.02.01** — Liệt kê legacy states prepared/sending/acknowledged/unknown và orphan/pending receipts. (`TODO`)
    - [ ] **CLP-07.02.02** — Cài reconciliation chỉ khi authoritative proof đủ, preserve old unknown và side-effect fences. (`TODO`)
    - [ ] **CLP-07.02.03** — Dùng existing prepared-only resume; không execute lại PATCH, không invent ACK/output. (`TODO`)
    - [ ] **CLP-07.02.04** — Test mixed states qua restart, historical unconfirmed, pending mutation và orphan tool pairs. (`TODO`)
- [ ] **CLP-07.03 — Explicit safe resume/new continuation** (`TODO`, P1)
  - Với collection-only unknown egress: cung cấp đường tiếp tục từ durable evidence như **new** continuation sau consent/authorization thích hợp, nếu không còn unresolved side effect. Giữ old run unknown/superseded metadata có audit trail, new request ID/digest, current contract, stateless rebase nếu phù hợp; không replay payload cũ hoặc resend ACKed receipt như mới. Nếu không thể chứng minh read-only/no outstanding patch thì hiển thị blocker/reconcile. Không tự resume user cancellation trái ý.
  - **Acceptance:** cùng conversation ID, giữ task plan và progress, không new workspace/conversation bắt buộc; next task có thể ra PATCH mới; restart/repeated resume idempotent; old unknown không bị xóa. Existing pending user turns/lease giữ đúng.
    - [ ] **CLP-07.03.01** — Thiết kế explicit resume action/authorization cho collection-only unknown, kiểm tra unresolved mutation và lease. (`TODO`)
    - [ ] **CLP-07.03.02** — Tạo new request ID/digest từ durable evidence/current contract, stateless rebase nếu phù hợp; giữ audit old unknown. (`TODO`)
    - [ ] **CLP-07.03.03** — Bind same conversation/plan/pending user turn; idempotency và user Stop được giữ, không replay old payload. (`TODO`)
    - [ ] **CLP-07.03.04** — Test resume thành PATCH mới, repeated click/restart, denied consent/pending mutation/cancelled run. (`TODO`)
- [ ] **CLP-07.04 — Migration và recovery test suite** (`TODO`, P1)
  - Tạo fixtures old→current→restart, session có archive, stale pack, previous response ID, feedback acknowledged/unknown, pending tool, cancelled, multiple sessions. Test native execution count, receipt history, selection/UI mapping, retry permissions.
  - **Acceptance:** migration additive/idempotent, old conversation tiếp tục khi được phép, no lost source/history/plan/settings; failure mode bảo toàn journal. Không thử trên dữ liệu production chưa backup/consent.
    - [ ] **CLP-07.04.01** — Tạo old→current→restart fixtures với archive/stale pack, previous response, ACK/unknown/pending/cancelled. (`TODO`)
    - [ ] **CLP-07.04.02** — Assert original history/receipt/goal/settings/plan, migration marker và execution count không đổi ngoài transition hợp lệ. (`TODO`)
    - [ ] **CLP-07.04.03** — Chạy legacy + unified recovery, permissions, concurrency và UI mapping. (`TODO`)
    - [ ] **CLP-07.04.04** — Ghi kết quả, blocker/môi trường; chỉ dùng production copy nếu có backup và consent. (`TODO`)

### CLP-08 — UI và observability (2 task, P2)

- [ ] **Phase gate** — `TODO`; chỉ đánh dấu khi tất cả task cấp 2 DONE và remote delivery đã xác minh.

- [ ] **CLP-08.01 — Conversation UI progress & actions** (`TODO`, P2)
  - Hiển thị compact: COLLECT since last executed PATCH (và since meaningful progress khi hữu ích), novelty/streak, current objective/checkpoint, reason, actual provider retry. Phân biệt `Reading`, `Replanning`, `Waiting provider`, `Stalled`, `Recovery required`; dùng UI hiện có, không tăng rối. Action resume/reconcile đúng state; disabled rõ lý do. Hỗ trợ light/dark, long session, tab hot-switch.
  - **Acceptance:** counters từ authoritative state, refresh/restart đúng; không lẫn conversations; read-only không hiện yêu cầu patch vô nghĩa; UI English labels/hướng dẫn theo quy ước dự án.
    - [ ] **CLP-08.01.01** — Thêm authoritative API projection cho counters, objective, checkpoint, provider phase/retry và reason. (`TODO`)
    - [ ] **CLP-08.01.02** — Thiết kế compact English labels Reading/Replanning/Waiting provider/Stalled/Recovery required; read-only phù hợp. (`TODO`)
    - [ ] **CLP-08.01.03** — Nối resume/reconcile action với backend guard, disabled reasons; không bypass permissions. (`TODO`)
    - [ ] **CLP-08.01.04** — Test polling/restart, tab hot-switch, light/dark, long session và no cross-session leakage. (`TODO`)
- [ ] **CLP-08.02 — Safe debug report và telemetry** (`TODO`, P2)
  - Thêm bounded guard events/counters, objective scope hash, evidence novelty class, overlap warning, payload size/token estimate, recovery type; correlation run/call ID; phân biệt clear debug history với canonical journal. Không log source/patch Python/prompt/API key.
  - **Acceptance:** export tái hiện nguyên nhân stall/timeout/ACK domain; clear debug không reset durable guard; event retention/size bounded; redaction tests.
    - [ ] **CLP-08.02.01** — Thêm structured bounded events guard decision/novelty/overlap/attempt/budget/recovery, run/call correlation. (`TODO`)
    - [ ] **CLP-08.02.02** — Redact source/prompt/Patch Python/credentials; enforce size/retention limits. (`TODO`)
    - [ ] **CLP-08.02.03** — Giữ clear debug marker tách canonical guard/journal; export đúng failure domain. (`TODO`)
    - [ ] **CLP-08.02.04** — Test redaction, clear history, retention và synthetic incident reconstruction. (`TODO`)

### CLP-09 — End-to-end regression, rollout và nghiệm thu (3 task, P1)

- [ ] **Phase gate** — `TODO`; chỉ đánh dấu khi tất cả task cấp 2 DONE và remote delivery đã xác minh.

- [ ] **CLP-09.01 — Full cross-pipeline regression matrix** (`TODO`, P1)
  - Chạy toàn bộ scenarios mục 6 và relevant existing tests; fake provider/no real secrets. Chạy race detector phù hợp concurrency, format/lint, test toàn bộ package liên quan và CI theo repo. Ghi command, exit code, logs/CI link.
  - **Acceptance:** không regression duplicate, schema, auth, capacity, approval, TaskPlan evidence; không test nào bị skip mà không giải trình. 55+ useful reads chạy; no unsafe replay.
    - [ ] **CLP-09.01.01** — Lập mapping R01–R28 tới test names/packages, thêm test còn thiếu; không bỏ suite baseline. (`TODO`)
    - [ ] **CLP-09.01.02** — Chạy targeted + package/full CI suite, format/lint/diff check, ghi exact commands/exit codes. (`TODO`)
    - [ ] **CLP-09.01.03** — Chạy race tests cho save/reconcile/lease/counters; kiểm tra native Python integration nếu chạm contract. (`TODO`)
    - [ ] **CLP-09.01.04** — Đối chiếu từng scenario, ghi failures/skip blockers, sửa regression trước DONE. (`TODO`)
- [ ] **CLP-09.02 — Upgrade smoke test & rollback guidance** (`TODO`, P1)
  - Build bằng quy trình repo; dùng synthetic copy của legacy state để self-update/reload/restart. Smoke test auto apply off/on, auto continue, provider switch, Stop, concurrent conversations, old session resume. Nếu có user environment được cấp phép, kiểm tra riêng với backup; không thực thi lại patch production.
  - **Acceptance:** xác nhận runtime build SHA, migration và resume thực tế hoặc ghi rõ phần chưa có môi trường; rollback không phá canonical journal; hướng dẫn vận hành/diagnostic.
    - [ ] **CLP-09.02.01** — Build theo repo, ghi build SHA và tạo synthetic backup legacy state; không dùng production patch. (`TODO`)
    - [ ] **CLP-09.02.02** — Thực hiện self-update/reload/restart smoke, kiểm tra migration/recovery/counters/ACK. (`TODO`)
    - [ ] **CLP-09.02.03** — Test auto flags, provider switch, Stop, concurrency và rollback compatibility; lưu evidence. (`TODO`)
    - [ ] **CLP-09.02.04** — Viết hướng dẫn upgrade/rollback/diagnostic, đánh dấu runtime verification chưa có nếu thiếu môi trường. (`TODO`)
- [ ] **CLP-09.03 — Final integration, docs và remote verification** (`TODO`, P1)
  - Rà soát main mới nhất, cập nhật handoff index/config docs, đóng các task có bằng chứng, thống kê checklist, liệt kê blockers còn lại. Commit/push trực tiếp main, verify remote ancestry và CI trạng thái nếu có. Không bỏ qua unresolved mandatory test.
  - **Acceptance:** toàn bộ 36 task được đối chiếu, chỉ DONE khi đủ chứng cứ; report cuối gồm baseline/final SHA, tests, migration proof, remaining risks. Không tuyên bố release đã deploy khi chỉ push code.
    - [ ] **CLP-09.03.01** — Reconcile HEAD mới nhất, audit toàn bộ checkbox/evidence và unresolved failures, cập nhật docs/index. (`TODO`)
    - [ ] **CLP-09.03.02** — Xác minh invariants, regression matrix, migration và acceptance; không mark done nếu thiếu proof. (`TODO`)
    - [ ] **CLP-09.03.03** — Commit code+tracker lên main, push và verify ancestry/CI; ghi actual SHA và delivery status. (`TODO`)
    - [ ] **CLP-09.03.04** — Tổng kết final report: baseline/final SHA, test outcomes, limitations, blockers, rollout status; không claim deploy. (`TODO`)

## 6. Regression matrix bắt buộc

Mỗi scenario cần test name, actual result và file/receipt/CI ref trong nhật ký. Đánh dấu checkbox trong ID chỉ khi scenario thực sự PASS; ghi VERIFY_PENDING/BLOCKED nếu chưa thể chạy. Expected dưới đây là tiêu chí, chưa phải kết quả test.

| ID | Scenario | Expected |
|---|---|---|
| [ ] R01 | 55 hoặc 100 unique useful reads rồi PATCH | Không hard cap, proposal đi qua approval chuẩn |
| [ ] R02 | Exact duplicate, đổi title/call ID | Phát hiện; không đọc lại vô ích khi proof còn hiệu lực |
| [ ] R03 | A/B/A/B, range subset/overlap | Phát hiện overlap đã xác minh; phần mới được tính đúng |
| [ ] R04 | Unknown novelty, incomplete/truncated output | Không fabricated success hoặc false duplicate rejection |
| [ ] R05 | Implementation 5/10 reads | Advisory phù hợp, không ép patch; no warning spam |
| [ ] R06 | Read-only audit 100 reads, no PATCH | Hợp lệ; chỉ checkpoint nếu stall thật |
| [ ] R07 | PATCH rejected/fail/no-op/partial mutation | Không reset meaningful progress sai; invalidate coverage khi cần |
| [ ] R08 | PATCH thực thi source change rồi đọc lại | Không stale duplicate suppression; receipt đúng |
| [ ] R09 | Schema invalid `grep`, project identity mismatch | Existing repair + auth preserved; không queue invalid mutation |
| [ ] R10 | Crash trước/sau native observation, provider prepared/sending | Guard count idempotent; journal phases đúng; no tool replay |
| [ ] R11 | Legacy conversation archive/stale pack, restart/self-update | Goal/history/settings/plan giữ nguyên; current contract dùng đúng |
| [ ] R12 | 2 ACKed PATCH → 8 COLLECT → provider unknown | ACK không đổi; no resend/replay; đúng recovery reason |
| [ ] R13 | Explicit consent resume old collection unknown | New run, same conversation, audit old unknown; no unresolved mutation bypass |
| [ ] R14 | Pending/unconfirmed PATCH side effect | Block hoặc reconcile có proof; không tự execute lại |
| [ ] R15 | 304→942 KB, HTTP400 context exceeded | Progressive compact, current receipt byte-exact; bounded recovery |
| [ ] R16 | Mandatory input quá lớn | Explicit budget blocker/checkpoint; không silent discard |
| [ ] R17 | Repeated timeout cùng payload, HTTP499, cancel | Correct retry policy, no duplicate native action, Stop hiệu lực |
| [ ] R18 | Response incomplete/output budget | Bounded guidance; không chạy partial patch/function JSON |
| [ ] R19 | Persistent adaptive hint restart, key rotation, TTL | Correct scope, invalidation, no credential leakage |
| [ ] R20 | Multiple conversations / worktrees / provider hot-switch | Isolation, immutable binding, capacity/lease preserved |
| [ ] R21 | Completion-aware TaskPlan acceptance | Provider completed != task done; only verified/attested acceptance |
| [ ] R22 | AutoCollection/Patch/Feedback/Continue opt-out | Respect permissions kể cả khi recovery |
| [ ] R23 | Large source ~150 MB targeted native collect | Không reader mới; bounded overhead, no arbitrary source-size regression |
| [ ] R24 | UI/debug export/light-dark/clear | True status, redacted, durable guard không mất |
| [ ] R25 | Legacy vs unified cùng scenario | Policy parity, protocol-specific differences documented |
| [ ] R26 | Stateless rebase và typed/text fallback | No orphan/fabricated tool pair, current contract and exact receipt |
| [ ] R27 | Restart recovery sau user Stop | Không tự resume trái ý; explicit continue có quyền mới |
| [ ] R28 | Concurrent save/reconcile/retry same logical call | Không double count, no stale overwrite, race tests |

## 7. Thứ tự triển khai và kiểm thử

Thứ tự khuyến nghị: `CLP-00 → CLP-01 → CLP-02 → CLP-03 → CLP-04 → CLP-05 → CLP-06 → CLP-07 → CLP-08 → CLP-09`. Có thể ưu tiên CLP-07.01 song song sau baseline nếu sửa độc lập và cần khôi phục UX sớm. Chỉ làm song song nếu ownership/file scope tách biệt; không có hai coder sửa cùng store/loop mà không reconcile. Nếu phát hiện regression nghiêm trọng, sửa theo task nhỏ và cập nhật kế hoạch ngay.

Lệnh test gợi ý (phải kiểm tra `go.mod`, Makefile, CI và runtime thực tế trước khi dùng; chưa chạy trong lúc lập plan):

```sh
cd vscode_tasks_menu_go
go test ./internal/agentchat/...
go test ./internal/server -run 'TestAgent(Collection|NativeCollection|.*Continuation|.*Retry|.*Recovery|.*Context|.*TaskPlan)' -count=1
go test ./internal/agentrun/...
# Khi môi trường hỗ trợ race detector:
go test -race ./internal/agentchat/... ./internal/agentrun/...
# Sau targeted tests, chạy suite/build/CI chuẩn repository (không bỏ qua failures).
```

Kiểm tra cả các Python/native Patch Tool test hiện có nếu sửa contract/preflight/collector integration; dùng lệnh được dự án cho phép. Không coi `go test -run` không match test nào là PASS; kiểm tra test thực sự được chạy. Dùng fake clock thay vì đợi 600s. Không chạy sudo hoặc Docker ngoài phạm vi.

## 8. Progress tracker (cập nhật sau từng task)

| Phase | Tổng | DONE | Trạng thái |
|---|---:|---:|---|
| CLP-00 Baseline | 3 | 2 | IN_PROGRESS |
| CLP-01 Guard | 5 | 1 | IN_PROGRESS |
| CLP-02 Evidence | 4 | 0 | IN_PROGRESS |
| CLP-03 Durability | 3 | 0 | IN_PROGRESS |
| CLP-04 Integration | 4 | 0 | IN_PROGRESS |
| CLP-05 Compact/Pack | 4 | 0 | IN_PROGRESS |
| CLP-06 Budget/Retry | 4 | 0 | TODO |
| CLP-07 Legacy recovery | 4 | 0 | TODO |
| CLP-08 UI/Debug | 2 | 0 | TODO |
| CLP-09 Validation | 3 | 0 | TODO |
| **Total** | **36** | **3** | **IN_PROGRESS** |

### 8.0. Roll-up ba cấp (cập nhật cùng checklist)

| Phase | Phase DONE | Task DONE / total | Subtask DONE / total |
|---|---:|---:|---:|
| CLP-00 | 0/1 | 2/3 | 8/12 |
| CLP-01 | 0/1 | 1/5 | 11/20 |
| CLP-02 | 0/1 | 0/4 | 6/16 |
| CLP-03 | 0/1 | 0/3 | 2/12 |
| CLP-04 | 0/1 | 0/4 | 3/16 |
| CLP-05 | 0/1 | 0/4 | 1/16 |
| CLP-06 | 0/1 | 0/4 | 0/16 |
| CLP-07 | 0/1 | 0/4 | 0/16 |
| CLP-08 | 0/1 | 0/2 | 0/8 |
| CLP-09 | 0/1 | 0/3 | 0/12 |
| **Total** | **0/10** | **3/36** | **31/144** |

Regression gates: **0/28 PASS**. Mỗi lần cập nhật, ghi ID subtask cụ thể, file/test/receipt, trạng thái; chỉ roll up sau khi xác minh. Commit từng subtask có thay đổi độc lập hoặc nhóm nhỏ coherent, không để toàn phase mới commit. Không tạo commit rỗng chỉ để tick.

### 8.1. Nhật ký triển khai (append, không xóa lịch sử)

| Thời gian ICT | Task | Trạng thái | Thay đổi / bằng chứng test | Commit / remote / blocker | Next |
|---|---|---|---|---|---|
| 25/11/2026 | PLAN | CREATED | Audit baseline `1c1e2ba0`; mở rộng handoff thành checklist ba cấp, chưa sửa source hoặc chạy test | Chưa commit tài liệu | CLP-00.01 |

| 25/11/2026 16:00 ICT | CLP-00.01 | IN_PROGRESS / BLOCKED local checkout | Remote main `a7cc2a1711bfc25efca51676150a4f5d300c7aa1`; compare baseline: ahead 2, behind 0, 14 changed paths. Native failed-queue retirement, identity, PASS protection, preflight, runtime checksums and CI changed; not proof of CLP guard. go.mod Go 1.20; CI Go 1.25.x. | GitHub connector remote reads verified; local git status exit 128 (not a repository), git ls-remote exit 128 (DNS github.com unavailable), local Go 1.23.2. No baseline tests run. | CLP-00.01.02–04 |

| 25/11/2026 | CLP-01.01.01–04 | VERIFY_PENDING | Added pure advisory-only collectionprogress policy and unit tests. Local isolated Go 1.23.2: gofmt; GO111MODULE=off go test -v -count=1 (exit 0); GO111MODULE=off go test -race -count=1 (exit 0). Three tests with four intent subtests; 100 useful reads per intent, advisory 5/10, duplicate streak 3, cooldown 5, unknown/error, meaningful progress reset. Tested blob SHA policy.go c6c2f15e4eaf38dd3a8a1860c6a05a759f7a10ca; policy_test.go edbb28d49a6be1e580a0a845d883d00920fa4ec7. | Full repository checkout unavailable; no pipeline integration, no fixture or CI PASS asserted. Work isolated per §1 while baseline tests blocked. | CLP-00.02/03 when runner available; then CLP-01.02 |

| 25/11/2026 | CLP-01.01 | DONE | Local isolated policy tests and race tests exit 0; remote blobs exactly match locally tested files; main confirmed at 1cc1d402a0208962de6aaa1b6ebce5d2325f7773. No runtime integration claimed. | [1cc1d402](https://github.com/hacrot3000/PatchAndCollectionToolForAI/commit/1cc1d402a0208962de6aaa1b6ebce5d2325f7773); [CI 45581659480](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45581659480) observed in progress | CLP-00.03 baseline CI |
| 25/11/2026 | CLP-00.03 | IN_PROGRESS | Baseline CI [45580850787](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45580850787) failure, agentchat-core job 139090221857: TestAcquireRunFenceExcludesSecondOwner: first fence acquire failed: same-host run fence unavailable, exit 1; server tests not reached. agent-failed-queue job 139090222198 PASS. Isolated reproduction with exact run_fencing.go/unix.go/test blobs and Status definitions from manager.go: umask 022; GO111MODULE=off go test -v -count=1 exit 1. Corrected fixture directory chmod 0700, added explicit 0755 refusal/no lock file assertion; GO111MODULE=off go test -v -race -count=1 exit 0. Production fence unchanged. Tested fixture blob 6f78342313f1cd4c8f712c708f3a86f30d495017. | Full CI pending. No skipped baseline tests marked PASS. | Verify CI and finish baseline trace/incident fixture |

| 25/11/2026 | CLP-00.03 | IN_PROGRESS | CI [45581920052](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45581920052) on ab1f30d6: fence test now passes; agentchat/agentrun reached server suite; TestAgentRecoverLatestOfRepeatedPreflightFailuresWithoutRerun panic in fakeAgentPatchProtocol.List (embedded nil Service), agent_chat_failed_queue.go:42. Added faithful mock List returning its synthetic Metadata. No production retirement/fence changed. | Remote CI exit 1; subsequent checks skipped. New fixture verification pending. | CLP-00.02 / CLP-00.03 |
| 25/11/2026 | CLP-00.02 | VERIFY_PENDING | Added TestAgentCLP20LegacyCollectionFailurePreservesAcknowledgedPatches: seeded 12/7 historical COLLECT pairs with two ACKed synthetic PATCH receipts, 8 fresh valid mock reads plus grep schema rejection, actual Legacy provider/journal loop with immediate timeout/400/Stop variants, exact receipt/ACK/no resend assertions. No production data or real provider/native mutation. | New Go server test not run locally (no checkout); CI pending. Combined growth + progressive retry, fake-clock attempt matrix and runtime replay remain outstanding. | Verify test; continue baseline matrix |

#### CLP-00.01 — Partial pinned-source trace
All paths below are relative to `vscode_tasks_menu_go/internal/server/`, read at `a7cc2a1711bfc25efca51676150a4f5d300c7aa1`.
- Legacy: `agent_chat_messages.go:162,356–448,512–586` message admission, request projection, Save, collection and Patch proposal. `agent_chat_collection_flow.go:113–359` validates native request, saves provider response checkpoint (181–184), saves native start (218–223), executes native (225), handles repair (279–301), builds exact next output/request (285–349), saves before egress (355–359). Inner loop has no fixed round cap (139).
- Feedback: `agent_chat_tool_feedback.go:204–425` checks duplicate canonical outputs, builds sibling output pairs, saves and continues; 406–413 handles Collection continuation failure separately from Patch feedback delivery. `agent_chat_continuation.go:20` outer auto continuation. `agent_chat_provider_resume.go:15` prepared recovery. `agent_chat_retry.go:241` provider retry.
- Unified: `agent_run_collection.go:21,45,77–155` RunSpec permission check, pending call resolution, schema/fingerprint repair, native execution, function output, snapshot CreateWithPolicy. `agent_run_continuation.go:61,103` verified progress signature and completed-turn decision. Different ownership: immutable RunSpec/snapshot vs mutable Legacy session/CAS. Full scheduler/lease/TaskPlan tracing remains pending.
- Native: `agent_chat_native_collection.go:37,149,198,307`; canonical request, manifest, observation, native runner. `agent_collection_repair.go:78,96` fingerprint and matching reads. New `agent_chat_failed_queue.go:29,73` retirement admission must be preserved.
- Pinned handoffs 13–19 (including both 18 documents) retrieved; 13/14 canonical compact/checkpoints, 15 PREPARED-only replay, 16/17 owner acceptance and authoritative native receipt, 18 native-only and completion-aware continuation, 19 immutable run-bound continuation. Handoff 17 active single-engine owner directive must be respected: reusable engine-neutral guard, no independent execution path. Full source reconciliation pending.
- Baseline test inventory: `agent_chat_collection_continuation_test.go` includes TestAgentCollectionContinuesWithoutRoundLimitToPatchProposal, TestAgentCollectionAlternatingIdenticalReadsDoNotExecuteAgain, TestAgentCollectionUnlimitedRetriesTimeoutAndHTTP499WithoutDuplicateRead; `agent_collection_repair_test.go` includes TestAgentProviderAcknowledgementCannotRegressToUnknown and TestAgentRecoverLegacyCollectionAcknowledgementRequiresExactJournal; `agent_chat_native_collection_test.go` includes TestAgentNativeCollectionArtifactSecurityAnd150MBSparseTXT; `agent_chat_retry_test.go` includes TestAgentContextRecoveryPreservesFreshReceiptAndShrinksHistory; `agent_chat_provider_run_test.go` includes TestAgentSendingRunRestartBecomesAmbiguousWithoutProviderReplay; `agent_run_collection_test.go` includes TestAgentRunCollectionContinuesExactProviderWithNativeObservation and TestAgentRunContinuationConsentAndUnknownEgressFence. Existing test source preserved; exact names inspected at baseline remote.
- CI: `.github/workflows/taskdeck_agentchat_core.yml` runs `go test -race -count=1 ./internal/agentchat/...`, `go test -v -race -count=1 ./internal/agentrun/...`, `go test -count=1 ./internal/server -run '^TestAgent'`, focused race tests, vet, Node checks and Python preflight/failed queue. These were read, not executed.
- At this initial trace checkpoint no source had changed. No local checkout; cannot assert local cleanliness or full baseline test success. Continue source audit via authenticated connector without force push; integration/test status remains unverified.

#### CLP-00.01 — Additional Unified trace (pinned `fe4cc344f1b82bfbeb4cab82b580cb7127cbd165`)

| Stage | Legacy | Unified |
|---|---|---|
| Entry / ownership | agent_chat_messages.go, mutable Session with revision CAS | agent_run_api.go:49,84; agent_run_provider_executor.go:27,72,119,138; immutable RunSpec and owner-bound snapshot |
| Tool resolution | agent_chat_collection_flow.go:181–359; journal before native start and provider egress | agent_run_collection.go:77–155; exact run provenance, execution identity, turn lock, native-only collector, exact result, snapshot with original policy |
| Durable scheduling | Session.Save and provider journal; PREPARED-only recovery | internal/agentrun/manager.go:356–467,621–715; attempt ID, pending-call exact pairing, locked record persistence, new idempotency key and queue; store.go:129 saveLocked |
| PATCH / ACK | agent_chat_tool_feedback.go; canonical native receipt | agent_run_patch.go:144–255; sync native result, refuse existing feedback intent/unknown, persist Session prepared before manager continuation; agent_run_feedback_ack.go:13 |
| Auto Continue / plan | agent_chat_plan_coordinator.go:19,107; agent_chat_continuation.go | agent_run_continuation.go:103–187; owner-pinned mechanical proofs, CAS, verified work signature, consent and no-progress guard; manager.go:788–819 validates run-bound history before queue |
| Existing gaps | inner collection loop has no durable cross-round progress guard | collection matching counts consecutive prior requests only; guard must not treat attempt IDs as objective IDs. Existing 256-item run history capacity at manager.go:415/797 is a separate storage limit, not a legitimate-read quota. |

No scheduler, permission, journal, fence, or PATCH recovery semantics changed by this audit. Local Git checkout and full build unavailable. Source trace is partial until all call sites and current runtime fixtures can be validated.

#### Verification checkpoint — CI on `fe4cc344f1b82bfbeb4cab82b580cb7127cbd165`

- [Agent Chat core run 45582342217](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45582342217), job 139095247757: **FAIL**. Actual CI logs show PASS for `internal/agentchat` (14.584s), `internal/agentchat/collectionprogress` (1.013s), `internal/agentchat/contextcompact` (1.226s), `internal/agentrun` (1.304s). Server suite `go test -count=1 ./internal/server -run '^TestAgent'` FAIL (22.243s).
- Server failures: `TestAgentRefreshDuringRequestOffersCancelNotDuplicate` at agent_chat_sessions_test.go:348, expected HTTP 409, got HTTP 202 queued; `TestAgentVisibleHistoryCompactsHistoricalPatchAndCollectionWithoutMutatingCanonical` at agent_chat_text_patch_test.go:214, historical PATCH provenance omitted from bounded catalog. Neither production path nor these tests changed by CLP-01.01 or the incident fixture. Need reconcile expected pending-turn behavior and fix catalog retention without weakening assertions.
- `agent-failed-queue` job 139095247358: **PASS**. Prior run-fence and nil mock failures no longer appear in latest CI logs. No separate verbose named PASS receipt for new CLP-00.02 fixture; leave VERIFY_PENDING, as full fixture requirements also include payload growth, progressive retry and fake-clock matrix.
- Remote commit `fe4cc344f1b82bfbeb4cab82b580cb7127cbd165` confirmed as main before this checkpoint. No deployment or full regression gate completion. Next safe work: CLP-00.01 trace completion, CLP-00.02 targeted fixture verification and growth/retry, CLP-00.03 two server failures; then CLP-01.02. Do not widen permissions or alter unknown/ACK recovery to clear the gate.

#### CLP-00.03 — Baseline repair continuation
- Rechecked remote main `6321781a90efbb81c2c5f93640558bee1c357c82`; no concurrent change.
- Fixed stale busy-session admission: `agent_chat_messages.go` refuses new pending turns while `agentPendingDispatchHandoffReady=false`. Existing queued turns remain durable; do not enable non-atomic dispatch or discard owner data. Existing failing TestAgentRefreshDuringRequestOffersCancelNotDuplicate remains unchanged.
- Fixed historical catalog eviction in `contextcompact/compact.go`: reserve up to 12 of the existing 36 bounded entries for recent PATCH/unknown/archived safety metadata, then fill remaining slots with recent observations under existing token budget; chronological rendering, no canonical mutations or replay. These are presentation priorities only, not authoritative receipt verification. Recompaction preserves priority summaries. Existing failing server provenance regression remains unchanged; added TestPriorityCatalogPreservesPatchAndUnknownAfterLongCollectionAndRetry (70 reads, 3 budgets, recompaction).
- Source changes and tests submitted for CI; no local repository checkout; no PASS claimed before actual CI receipt. CLP-05.02.02 partial fix advanced specifically to clear baseline regression, not full catalog acceptance.

#### CLP-01.02 — Objective scope groundwork
Reconciled concurrent merge `4cf2e8d69941164cdedf75270843fb6f72688410` (two commits ahead of `c4a86276`, no divergence; changes in provider_run/store/UAG-02 tests and master progress; no CLP module overlap). Added objective.go and objective_test.go with SHA-256 binding of authenticated owner/project/owner-workspace/execution-workspace/conversation/exact owner goal/step/generation. Provider, model, attempt and call IDs are deliberately absent. Pure objective transitions preserve Stop, invalid and unchanged scopes; classify actual owner-authored goal only (conservative EN/VI explicit requests and restrictions), never model hints or synthetic user-role outputs. Existing proposal availability can neutralize coding advice, not grant permission. Tests cover no-plan, step/generation/scope changes, Stop and disabled proposal path. Source-only until CI receipt; no session/snapshot integration, generation lifecycle or owner-input provenance adapter yet. CLP-01.02 remains IN_PROGRESS.

#### CLP-00.02/00.03 — Combined incident and PREPARED fixture
- Reconciled concurrent UAG-02 merge `00690e3311168f87e6c974b587866507bedd3878` after `822fe775` (ahead 2, behind 0, store/prepared tests only). On earlier concurrent merge `4cf2e8d6`, core CI [45583413605](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45583413605) failed at TestAgentSessionGetShowsOwnerScopedProviderEgressReviewWithoutReplay: PREPARED fixture missing sidecar after newly tightened Store admission. Stage exact synthetic payload before Save; retain all existing assertions and production fence. CI at `c4a86276` showed core Go/Node/vet step and Python preflight PASS, macOS crosscompile still running when checked.
- Expanded TestAgentCLP20LegacyCollectionFailurePreservesAcknowledgedPatches with progressive_retry_stop: actual legacy loop, 12/7 historical reads then 8 valid and 1 invalid request, synthetic growing payload (bounded item sizes), explicit HTTP 400, reduced egress, ambiguous timeout with byte-identical retry, Stop. Inject 1ms delay; no actual network or 600s sleep. Assert exact current call/output (one wire and canonical pair), ACK preservation, payload size trend, and eight native reads. Added named verbose CI selector so logs retain actual payload and outcome. This is not the production payload reproduction byte-for-byte.
- Test changes and workflow submitted for CI; no local checkout; CLP-00.02 remains VERIFY_PENDING until named receipt. CLP-01.02 objective groundwork also pending CI/integration. Next: check CI and complete baseline evidence, then objective/counters integration.

#### CLP-01.03 — Ordered counter reducer
Added counter.go/counter_test.go: separate requested/observed/native-read/verified-new/duplicate/unknown/error/HTTP-attempt/PATCH proposal/execution/rejection/uncertain/verified-progress counters. Preflight rejection is not a native read; HTTP retry never increases COLLECT counters. Only verified objective progress clears the existing progress/warning state. Stable ordered source sequence + event digest gives idempotent reduction for the same durable event and rejects conflicting latest sequence. This requires future journal adapters to reuse the original event sequence; does not yet prove end-to-end logical dedupe or persistence. Added tests for boundaries, no-op/failed/uncertain PATCH, preflight, retry, stale replay and conflicting sequence. CI pending. On earlier `c4a86276`, [CI 45583368471](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45583368471) completed SUCCESS; both original failing tests passed with unchanged assertions. Concurrent UAG-02 merges subsequently added stricter PREPARED admission; fixture repair and combined incident test at `ae58b99c431fffe5092845d90fb9c3512df314b6` awaiting named CI receipts.

#### CLP-01.04/01.05 — Bounded recovery policy groundwork
Pure recovery.go + recovery_test.go: one reserved replan per objective generation after 3 verified duplicates + 5 further duplicate observations, then at least 5 actual reads before another stall checkpoint. Requires a committed canonical output, safe new egress and current owner continuation grant; Stop, unresolved provider egress, pending call and invalid state have separate checkpoint reasons. Repeated evaluation of the same reserved checkpoint cannot reserve another replan. New verified evidence breaks the duplicate streak without refilling recovery allowance. Adds central range bounds to advisory thresholds (not an execution quota); 200 distinct verified reads tested in source. Recovery decision is not a scheduler or authorization grant; integration, durable reservation and exact call closure still pending. CI verification pending.

#### CLP-00.03 — Baseline regression receipt
At `c4a8627620274d98421be62d72dd7978614064e6`, CI [45583368471](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45583368471) completed SUCCESS (jobs 139098695930/139098696278). Linux Go 1.25.x: agentchat race 15.256s, collectionprogress race 1.013s, contextcompact race 1.350s, agentrun race 1.346s, all TestAgent server 22.674s, focused server race 7.938s, taskmanifest race 1.018s; both go vet, Node checks/tests, Python preflight 3 tests, macOS arm64/amd64 crosscompile and failed-queue job success. Subsequent UAG-02 merges introduced PREPARED fixture failure; corrected fixture at ae58b99c, CI still pending. Not a claim that HEAD or 28 handoff gates pass.

#### CLP-00.02 / CLP-00.03 — Verified receipt
CI [45583866679](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45583866679) at ae58b99c completed SUCCESS, both jobs. Named TestAgentCLP20LegacyCollectionFailurePreservesAcknowledgedPatches: timeout, context_rejected, stop, progressive_retry_stop all PASS (1.16s total). Synthetic initial 330572 bytes, late 946632, reduced 409907, retry 409907. Exact current receipt, eight native reads + one schema rejection, two historical ACKs and no duplicate egress after Stop asserted. No production data or live provider. Core Go/race/vet/Node/Python/macOS/failed-queue steps green. CLP-00.02 and 00.03 DONE for baseline reproduction; 00.01 local checkout blocker remains. Objective helper/classifier and prior PATCH compact fix also covered by this CI; their runtime integrations not done.

#### CLP-02.01 — Shared canonical fingerprint
Moved the existing server request fingerprint to collectionprogress/scope.go: deterministic JSON key order/number preservation, only top-level and action id/title discarded, execution-affecting fields retained. Ambiguous duplicate keys or malformed input cannot produce a reusable scope. Legacy repair path uses exact-byte fallback, not proof of coverage. Scope descriptor binds trusted objective and exact project identity plus optional authoritative source revision digest; missing revision never authorizes reuse. No new reader, validation bypass or native-skip decision. Tests cover metadata equivalence, distinct fields/numbers, invalid JSON, revision and scope isolation. CI pending; coverage adapter and actual source revision binding not yet implemented.

#### CLP-02.02 — Verified native evidence groundwork
Audited agent_chat_native_collection.go:198–259 and agent_chat_native_full_file_evidence.go:23–109. Server observation contains native session/run identity, project, wrapper status/rc, manifest verification, excerpt truncation and native_full_file_reads (action index/path/line count/report SHA-256). New evidence.go compares only supported complete full-file proofs covering every exact request action; partial, unsupported range/search/decompile, missing/invalid metadata remain UNKNOWN; schema error is ERROR. Digest excludes volatile receipt and artifact names. This is post-read comparison only, not pre-read reuse or native acceptance; authoritative caller binding still required. Added 128-entry objective-isolated index, immutable reduction, conservative UNKNOWN on eviction, tests for content/ID changes, partial/mismatch and bounds. No new reader, source scan or skip decision. CI/persistence/runtime adapters pending. Counter reducer commit 7d1bb2c7 passed full core CI [45584061420](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45584061420); pure counters validated, logical checkpoint integration still pending.

#### CLP-02.03/02.04 — Coverage and invalidation groundwork
Reconciled concurrent HEAD f9bdbcd4b3f69076ccf1c9b0eda9304ed44edfef (four commits ahead of d041ec9c, no divergence; store/provider/run API/UI changes outside CLP files). Added pure coverage.go/coverage_test.go: verified same-revision range overlap/union, bounded 64 scopes × 128 intervals, conservative unknown on eviction/corrupt or unavailable coverage. Tests A1–100→A1–110, alternating paths, 55 pages, changed revision, selective/unknown invalidation and metadata cap. Exact-request proof index conservatively clears on any potential mutation. Native range/mutation adapters and runtime integration not yet implemented; no read skipping. CI pending. Counter 7d1bb2c7 and recovery 479ead67 core CI 45584061420/45584362259 complete SUCCESS; mark only pure subtask evidence complete, keep integrations pending. Scope/evidence CI 45584606033/45584790677 still in progress at checkpoint.

Mẫu bản ghi task (điền dữ liệu thật):

```text
Task: CLP-xx.yy
Subtask(s): CLP-xx.yy.zz
Status: IN_PROGRESS / VERIFY_PENDING / BLOCKED / DONE
HEAD before:
Files changed:
Implementation decision / reason:
Tests executed (exact command, exit code, actual outcome):
Evidence refs (test name, native receipt, CI URL, relevant source):
Known limitations / blockers:
Commit: pending or actual SHA
Remote delivery: pending / confirmed / failed (reason)
Next task:
```

#### CI verification checkpoint — 64ccca20

- Scope fingerprint commit `d3275af7`: [CI 45584606033](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45584606033) complete; core and failed-queue jobs SUCCESS.
- Native full-file evidence commit `d041ec9c`: [CI 45584790677](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45584790677) complete; core and failed-queue jobs SUCCESS.
- Coverage commit `64ccca20`: [CI 45585091261](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45585091261) in progress at observation. No PASS claimed for this commit. No checklist increment for incomplete runtime adapters. Next: objective/counter receipt adapters, durable guard state and legacy/unified wiring.

#### CLP-03.01/03.03 — Additive Legacy session guard
Reconciled HEAD caa93714e695668130127c9e7014644a13dcbd9e (2 commits ahead, no divergence). CI 45585091261 failed server tests: PREPARED corrupt-digest fixture now rejected at Save; public Unified creation deliberately disabled by concurrent UAG-02 work (three old API/E2E fixtures). Do not restore public creation or weaken PREPARED fence to clear tests. Agentchat/collectionprogress coverage tests passed; full server suite not green.
Added agentchat/collection_guard.go and tests, Session.collection_guard raw additive field in existing revision-CAS journal, versioned metadata (96KiB), hashed scope/goal, generation, pinned policy, counters/evidence/recovery; invalid/future guard remains opaque and does not block canonical history. Bounded legacy retained-history summary separate from new objective counters, never claims old novelty or verified progress. Tests reopen, stale CAS, owner/step scope, legacy and future/invalid metadata. No separate journal. Legacy admission/checkpoint integration and Unified policy snapshot pending; CI pending.

#### CLP-01.02/03.02/04.01 — Legacy runtime admission and observation
Added agent_chat_collection_guard.go, wired real owner admission before original turn PREPARED save; synthetic scheduler/repair continuation distinguished via private in-process context marker (not request_id or user-controlled JSON). Explicit Continue preserves current binding; server-owned plan continuation binds plan goal/step; unbound legacy continuation stays neutral and retains only historical summary. Legacy loop records requested after permission and response checkpoint, observed after existing native collector, using session EventSeq and existing CAS. Schema rejection is not a native read; native proof adapter runs only on server-produced observation. Fixed-size advisory goes into trusted instructions separately from raw output, with budget estimator using identical instructions. No additional collector or network call; no new mutation authority. Legacy pre-existing fingerprint skip/receipt mutation still to remove under CLP-02.03, recovery reservation not enabled, HTTP/PATCH event adapters and Unified still pending. CI pending.

#### CLP-02.03/04.01/04.04 — Remove Legacy proofless skip
Removed third-identical-fingerprint suppression and receipt rewriting. Request shape now produces only fixed bounded instruction advice on third occurrence (including alternating scopes); never asserts unchanged source, skips native execution or alters a valid receipt. Updated duplicate tests for new safety contract: four actual reads with changed content on third request, exact canonical receipts, unknown novelty, one warning; alternating A/B six actual reads and two warnings. Kept 55 unique reads → PATCH regression untouched. Existing schema repair budget remains. Verified novelty policy separate; Unified proofless skip and durable verified-stall recovery pending. CI pending.

#### CLP-00.03 — Reconcile concurrent CI failures without weakening production fences
Core CI 45586888166 and 45587045914 passed agentchat/race, collectionprogress/race and contextcompact/race, and named CLP20 fixture; full server suite still failed the same four pre-existing concurrent UAG-02 regressions. Updated PREPARED corruption regression to assert the tightened Store rejects mismatched digest and then corrupt the actual private sidecar to test restart fail-closed without network. Public Unified POST remains hard-disabled; extracted private creation-policy handler, public route always false. Underlying existing admission and native Patch E2E tests invoke the private handler directly with true to exercise original permission, identity and binding assertions; public disabled regression unchanged. No configuration, header or request bypass and no scheduler route enabled. CI pending.

#### CLP-01.04/04.01 — Legacy bounded verified-stall checkpoint
Only a verified duplicate streak after earlier advice can trigger stall recovery. Save the exact native receipt and counter state before evaluating recovery; use original ACK, absence of unresolved PATCH/pending turn and current owner automation grant. One replan within existing next continuation (stateless rebase and fixed advisory, no replay); reserve in guard before PREPARED egress. Exhaustion returns recoverable local checkpoint preserving exact output and ACK, not task completion. Reprojection removes just-saved output from History only for the current Next pair; no canonical truncation. Recovery pure logic also checks exact event digest. Added named synthetic full-file proof integration test (14 actual reads, one replan, no 15th egress before explicit Continue; novel result resumes same objective without resetting budget). CI pending. Unified and legacy recovery UI remain pending.

#### CLP-00.03 / drain-only UI regression
CI 45587460818 at 078801df passed Go server suites, race and vet, then failed old bootstrap Hot-Switch fixture: composer correctly used Legacy /messages after concurrent UAG-02 disabled Unified creation, but test expected new RunManager POST. Added early same-conversation busy fence (preserve draft/live turn/AbortController, no new send/cancel). Updated regression to assert public busy composer refuses without egress; retained independent RunManager module mocked submission/binding, card isolation, retry/stop and native approval/continuation assertions as module-level tests, not public creation. No Unified route reenabled. CI pending.

#### CLP-01.03 / 02.04 — Native PATCH transition invalidation
At existing Session.Save CAS, compare previous vs new server-owned ToolCalls before archival. Count newly registered Legacy proposal/rejection, exact native final run receipt execution, uncertain outcome separately; ACK or same terminal ledger transition cannot double count. Any potential source mutation invalidates exact-request evidence broadly (including unknown or Unified ledger transitions; do not attribute Unified execution count to Legacy). Explicit native preflight no-change does not claim execution. Wrapper PASS never resets meaningful progress or acceptance; clear only duplicate streak on potential mutation. Preserve opaque guard during normal saves. Added store CAS test for proposal → running → final → ACK with unchanged progress and evidence invalidation. External drift/path-specific proof and HTTP attempts pending. CI pending.

#### Verified checkpoint — Legacy runtime
- Remote commits 0a8eee6f (session guard), 0ab64432 (admission/observation wiring), d193972f (no proofless skip), 078801df (PREPARED and drain-only regression fixtures), 92656e4e (verified-stall checkpoint), 9942f922 (busy composer protection), 2b71548c (native PATCH transition invalidation) confirmed on main.
- CI [45587765715](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45587765715) at 92656e4e: named TestAgentCLP20VerifiedStallPreservesReceiptAndBoundsReplan PASS (0.16s), four synthetic incident variants PASS, all TestAgent server 22.180s, focused server race 7.797s, agentchat/contextcompact/collectionprogress race and vet PASS. Full job FAIL only in old Hot-Switch Node bootstrap fixture expecting new Unified Run, corrected separately at 9942f922; no full CI success asserted.
- [45588106909](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45588106909) busy composer and [45588285483](https://github.com/hacrot3000/PatchAndCollectionToolForAI/actions/runs/45588285483) PATCH transition invalidation CI pending at checkpoint. Subtask ticks updated only for verified portions; 28 full gates remain unclaimed.
- Next: verify both CI runs; reconcile any regressions; complete durable HTTP attempt reconciliation, source drift/path proof, Unified snapshot/guard integration for existing drain-only runs, and CLP-07 explicit legacy unknown-collection resume with preserved audit. Do not reenable public Unified Run creation, bypass provider journal fences or treat wrapper PASS as verified task acceptance.

#### CLP-03.02 / CLP-06.02 — Attempt reconciliation
CI 45588106909 and 45588285483 both jobs SUCCESS, including full core Go/race/vet/Node/Python/macOS crosscompile. At existing Session.Save CAS project the existing private provider-attempt sidecar Total delta into guard HTTPAttempts, retaining run ID/cursor; reconcile previous run before replacement, including restart UNKNOWN. The count represents durable pre-egress reservations, not verified transmissions. No extra journal, no native count or ACK changes; corrupt audit marks incomplete and cannot fabricate counts. Bounded sidecar truncation still supports exact Total; test 132 attempts, restart/repeated Save/new run, corrupt audit. CI pending.
 
#### CLP-05.02 — Current text fallback compaction/archive
Shared exact native text observation prefix between Legacy producer, archive and compactor. Recognize current full generated envelope with preceding assistant COLLECT JSON and object receipt; preserve compatibility with historical prefix. Old fallback receipts become bounded catalog hash summaries (not verified success or mandatory human instructions), and durable private archive can retain them. Current Next stays exact; no canonical receipt deleted. Tests 20 large historical fallback receipts within context budget; 90 text reads across archive retaining all receipts and real owner instruction. CI pending.

#### CLP-05.02 — Preserve text request/receipt boundary
Reconciled concurrent UAG-02 HEAD f4408666 (two commits, no overlap with text pair files). Archive text request and receipt together; if the next item crosses the live window boundary retain both. Compactor catalogs completed text pairs as one group, preventing the old standalone request from appearing as recent assistant narration. No synthesized tool IDs.

### 8.2. Blockers và quyết định (append)

| ID | Task | Vấn đề / quyết định | Ảnh hưởng | Hành động tiếp theo | Trạng thái |
|---|---|---|---|---|---|
| D-001 | PLAN | Soft threshold 5/10/3 là đề xuất, chưa tuning thực tế | Không dùng như hard cap | Unit tests và fixture validation | OPEN |
| D-002 | PLAN | Log redacted, không xác nhận số COLLECT thừa hoặc exact old binary SHA | Cần test dựa trên synthetic evidence | CLP-00.02, CLP-07.04 | OPEN |
| D-003 | PLAN | Chưa chạy local/CI hoặc deploy build | Chưa xác nhận runtime fix | CLP-00.03, CLP-09 | OPEN |
| D-004 | CLP-00.01 | Sandbox has no repository checkout; direct Git DNS unavailable; connector remote API available | Cannot run full baseline/test/build locally; remote Git writes via connector only | Provide a checked-out repository/test runner or obtain actual CI receipts; no false PASS | OPEN |
| D-005 | CLP-00.03 | Pre-existing CI failure: run fence fixture used t.TempDir without ensuring 0700 | Prevents subsequent server tests from executing | Fixed fixture precondition; local isolated race test and latest CI agentrun package PASS | RESOLVED |
| D-006 | CLP-00.03 | Earlier core CI fails on pending-turn HTTP 202 vs 409 and lost historical PATCH provenance | Full server/race/remaining CI acceptance not green | Fixed both regressions in c4a86276; core CI 45583368471 and subsequent 45583866679 PASS | RESOLVED |

## 9. Definition of Done toàn handoff

- [ ] 10/10 phase, 36/36 task chính và 144/144 subtask đã DONE với bằng chứng tương ứng, hoặc báo rõ task còn BLOCKED/VERIFY_PENDING và không tuyên bố hoàn tất toàn handoff.
- [ ] Cả Legacy Agent Chat và Unified Agent Run có guard dùng chung hoặc shared policy tương đương được kiểm thử.
- [ ] Không ngăn legitimate long research, không bắt tạo PATCH khi chỉ đọc; no-progress có checkpoint/next strategy rõ ràng.
- [ ] Conversation từ build cũ có đường tiếp tục an toàn không mất journal/plan/evidence, không replay PATCH hoặc ACKed feedback.
- [ ] Compact budget và retry an toàn qua restart; explicit provider rejection khác timeout/cancel/unknown.
- [ ] UI/debug phản ánh đúng failure domain và counters; không lộ secrets, không bypass approval/identity/whitelist.
- [ ] Regression matrix và relevant full suite có kết quả xác minh; ghi limitations của môi trường và CI nếu chưa thể chạy.
- [ ] Code + tài liệu tiến độ đã commit/push trực tiếp `main`, remote được xác minh; báo final SHA và các commit chính.

**Chỉ thị bắt đầu:** Bắt đầu từ `CLP-00.01`, đọc HEAD hiện tại, cập nhật trạng thái và thực hiện task đầu tiên. Sau khi hoàn tất từng task, commit/push và tiếp tục task kế tiếp. Không dừng ở việc trả lời bằng một plan khác; file này là source of truth tiến độ. Không đánh dấu toàn bộ plan hoàn thành khi chỉ thực hiện một phần.
