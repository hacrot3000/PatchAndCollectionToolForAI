# TaskDeck Remote — mainline development

## Merge record (2026-11-06)

- Source: `feature/taskdeck-remote-access-architecture` at
  `ff9b31fa486f3aac31d7bd04fb0a6124aa063c8c`.
- Target before merge: `main` at
  `2de1825038ffa4a2c7f9c2c385006f9409f57d6d`.
- GitHub PR #146 was merged with method `merge`, preserving full commit
  history, at merge commit
  `5569c99e4c0a6e379996968ba084a2fc8004addf`.
- Source branch was 759 commits ahead / 0 behind, and its last code
  revision `6769022abc26efa125a36fa2ede028b46a92eb85`
  passed both TaskDeck Remote Agent (Go) and Remote Portal (PHP) CI.
  The source HEAD after that code commit only added documentation.

## Authoritative working branch

**From this merge onward, development of TaskDeck Remote is done directly
on `main`.** Do not open new feature branches for this workflow unless
the repository owner explicitly changes the decision. Do not continue
committing to `feature/taskdeck-remote-access-architecture`.

Remote Agent and Remote Portal push CI now target `main`; normal
`pull_request` triggers remain for externally submitted changes.
No force-push or history rewriting is authorized by this policy.

Before a direct `main` commit: read current HEAD, use a compare-and-swap
expected SHA on ref updates, keep E2EE/RBAC/audit boundaries, add bounded
regression tests, then inspect the CI for the committed revision. Never
silently mix incompatible pre-E2EE protocols from the historical `main`
implementation.

## Remaining code work (priority order)

1. Tasks — investigate and restore genuinely missing dependent-workflow
   reports and managed-task behavior; no blind protocol copying.
2. Git — larger paged reviews, remaining read-only interactive reviews and
   narrowly authorized mutations.
3. Files — safe deeper navigation, file-content reads within explicitly
   allowed subfolders and reviewed E2EE binary transfer contracts.
4. Editor/GUI — compare-and-swap guarded edits, conflict handling, and
   comprehensive file browser/editor interface.
5. Security — filesystem races, session revocation, quotas, concurrency,
   comprehensive permission and release-gate regression coverage.

Features whose code and automated checks are complete but only need
real-host testing are considered **code complete**.

## 2026-11-06 — Read-only, principal-scoped task outcome report

- Added `tasks.report` as an argument-free, explicitly whitelisted E2EE
  action, separately gated by the current TaskDeck user's project-scoped
  `tasks.view` permission and the LIVE authenticated Portal/client-key
  grant. The trusted host must enable BOTH
  `TASKDECK_REMOTE_TASKS_PREVIEW=1` and
  `TASKDECK_REMOTE_TASKS_REPORT=1`; report reads fail closed by default.
- The Agent reads only the local TaskDeck `remote.action.tasks_run_finished`
  audit rows filtered by the exact authenticated `user_id` AND
  `project_id`. It enforces known schema and `transport=e2ee` and returns
  at most three recent entries (task ID, completed/failed/cancelled/timed_out,
  exit code, duration and UTC timestamp), never raw audit text, command,
  argv, cwd, task input, output, environment, credentials or other users'
  records. Invalid/malicious legacy audit entries are skipped.
- A fully typed strict JSON parser and the existing 512-byte AEAD plaintext
  ceiling bind every E2EE response. The browser cannot supply task ID,
  username, project, filters or pagination offsets. The native GUI shows a
  names-free outcome summary via `textContent` and scrubs it on logout.
- The server appends a TaskDeck-local audit event for each authorized
  `tasks.report` action. Focused Go race/Node tests cover principal
  isolation, host opt-in, malformed/forged JSON, quotas, native no-argument
  requests and display cleanup.
- This adds **historical run results**, not dependency execution, queued
  workflow orchestration, persisted output or arbitrary report export.
  These are separate unfinished functionality and require new explicit
  E2EE contracts; do not use a generic remote audit/file API.

## 2026-11-06 — Read-only dependency graph for issued Task IDs

- Added the **read-only** `tasks.dependencies` E2EE action. The request
  carries exactly one stable numeric Task.ID **already issued by the same
  authenticated session's `tasks.preview` pages**. Client/PHP/Relay cannot
  select a tasks.json path, workspace, command, arguments, edges or executor.
- This check requires the existing `tasks.view` RBAC and
  `TASKDECK_REMOTE_TASKS_PREVIEW=1`. Enable dependency graph content with
  `TASKDECK_REMOTE_TASKS_DEPS=1` on the host; absent this opt-in the
  authenticated response is **content-free** `available=false`, not a
  failed login or a session-killing permission downgrade.
- Agent reads only the canonical host workspace's
  `.vscode/tasks.json`, verifies the task-source SHA-256 bound to the
  session's issued preview, rejects changed source/inode, and delegates
  graph correctness (missing dependencies, duplicate labels, cycles and
  topological ordering) to the existing TaskDeck
  `tasks.BuildWorkflowGraph`.
- The response returns only a bounded topological array of Task IDs and
  prerequisite edges (at most 8 nodes and 8 edges, inside the existing
  **512-byte AEAD plaintext**). No labels, commands, secrets, input values,
  workflow scripts or raw task configurations are sent to the Relay.
  The local GUI displays known labels from the previously authenticated
  preview using text-only rendering, otherwise displays just `Task #ID`.
- Every action is checked again for local identity, project permission,
  live Portal grant and E2EE; TaskDeck audits authorized reads. Graph
  inspection **never** starts a process, never schedules dependencies and
  does not grant `tasks.run`. The old task execution preflight still
  rejects `dependsOn` and other unapproved workflows.
- Tests cover malformed/duplicate/wrong-task E2EE replies, topological
  validation, unissued ID attempts, source changes, missing/cyclic
  dependencies, host opt-out, unchanged `tasks.run` block, and native GUI
  logout cleanup. The next workflow milestone is a separately reviewed
  host-owned, cancelable DAG executor with per-step rights and audit; it
  must not reuse this preview action as an execution capability.

## 2026-11-06 — Opt-in linear Task workflow execution

- `tasks.workflow.start` is a **separate**, authenticated E2EE mutation,
  not an extension of the read-only `tasks.dependencies` privilege.
  Native client sends only an already-issued root Task.ID. Every start
  requires a live TaskDeck `tasks.run` permission, current approved
  client-key proof, password, project grant and uncached Portal
  authorization, plus explicit host opt-ins:

  ```shell
  TASKDECK_REMOTE_TASKS_PREVIEW=1
  TASKDECK_REMOTE_TASKS_DEPS=1
  TASKDECK_REMOTE_TASKS_RUN=1
  TASKDECK_REMOTE_TASKS_LONG=1
  TASKDECK_REMOTE_TASKS_WORKFLOW_RUN=1
  ```

- This first executor is intentionally conservative: exactly **2–4**
  sequential `process` tasks in one linear dependency chain. It rejects
  branching/parallel graphs, scripts/shell interpreters, hidden inputs,
  secret environment values, arbitrary command templates, host workflow
  retry/output/policy metadata and workflows containing extra nodes.
  All `tasks.run` and `tasks.start` single-task paths **still reject
  dependsOn**. A DAG preview alone never authorizes mutation.
- The host builds and validates the exact pinned `.vscode/tasks.json`
  graph and each Task.ID before accepting the start. Every step
  independently re-checks the approved client credentials/RBAC, live
  Portal grant and the original source hash; changes or revocation
  fail closed before launching the next process.
- The Agent retains the exclusive long-task/terminal execution slot for
  the whole workflow; a single cancelable 90-second host job executes
  the steps in order. The **existing** `tasks.poll`, `tasks.cancel`
  and `tasks.clear` actions work with the root Task.ID. Standard
  process-group cleanup terminates children when canceled, disconnected
  or expired. Execution stops on the first nonzero exit code.
- Durable TaskDeck-local audit records a root workflow attempt before
  any execution plus **attempted and completed/failed/cancelled/timed_out**
  outcome events for each step, followed by the aggregate root outcome.
  Audits carry only numeric IDs, step count, exit code and status;
  never commands, paths, inputs, credentials or output.
- Native trusted GUI activates **Chạy workflow đã kiểm tra** only after
  authenticated DAG review and task-long/run capabilities, and always
  requires a confirmation. No user-entered shell command or arguments
  are accepted; ordinary long-task progress and cancellation are reused.
  Tests exercise E2EE-only start, topology/bounds, source drift, opt-out,
  execution ordering, fail-fast, process cancellation, audits and UI
  confirmation and logout.
- Long task GUI polling now consumes each bounded output window under a
  single per-job cursor lock. Simultaneous polls cannot replay a chunk or
  roll back a later cursor; a 64-request concurrency regression covers this.
- Security hardening: workflow process steps now require native ELF
  (Linux) or Mach-O/universal (macOS) executable headers. A file with
  executable mode but a script shebang, an executable text wrapper, or
  a symlink to either is denied in both the GUI RunAvailable preflight and
  actual execution. Regression tests cover these cases.
- Not yet supported: branching DAG, parallelism, workflow secret inputs,
  retry/condition policies, per-node output exports, workflow resume,
  persistent step-by-step result pagination and Windows execution.
  Those require separate reviewed contracts rather than silently relaxing
  this conservative host gate.

## 2026-11-09 — Giới hạn retry cho Remote workflow (mã nguồn)

- Remote chỉ chấp nhận chính sách `taskdeck.workflow.retry: 1` trên một task
  `process` đã vượt qua mọi kiểm tra DAG và mã thực thi native. Mọi
  metadata `taskdeck` khác (condition, outputs, timeout, retry cao hơn,
  thuộc tính lạ) tiếp tục bị từ chối; shell/script và implicit parallel
  vẫn không được phép.
- Đây là cơ chế **opt-in bổ sung, mặc định tắt**:
  `TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY=1`. Các host chỉ bật workflow
  thông thường không tự động cho phép chạy lặp. `retry:1` có thể gây
  lặp tác dụng phụ nếu bước đó không idempotent; chỉ cấu hình cho task
  đã được chủ host chấp thuận.
- Mỗi lần thực thi phải xác minh lại quyền `tasks.run`, live Portal
  grant, nguồn `.vscode/tasks.json`, cwd và binary. Mỗi attempt có
  audit `attempted` và `completed/failed/cancelled/timed_out` với
  số lần thử (1 hoặc 2); không ghi argv, output hay secret.
- Chỉ retry khi native child trả exit code 1..255; không retry nếu
  auth/preflight/audit fail, process không khởi động được, hủy hoặc
  deadline. Workflow vẫn dừng khi hết số lần thử hoặc có lỗi bảo mật.
- Trusted GUI xác nhận nguy cơ tác dụng phụ trước khi chạy workflow.
  E2EE client vẫn chỉ gửi root Task ID, không nhận lệnh/argv từ browser.
- Regression test bao gồm opt-in tắt/mở, giới hạn chính sách, hai lần
  thử thật, audit riêng mỗi attempt và fail-fast chặn dependency tiếp.
  CI cần được xác nhận PASS trên commit cuối cùng trước khi coi là
  kiểm thử hoàn tất; không tự bật tính năng trong môi trường triển khai.


## 2026-11-09 — Giới hạn Git patch lớn và quota phiên

- Mặc định, Git patch qua E2EE chỉ cho phép snapshot tối đa 4 KiB.
  Đặt TASKDECK_REMOTE_GIT_PATCH_LARGE=1 (và vẫn yêu cầu
  TASKDECK_REMOTE_GIT_PATCH=1) để chủ host cho phép snapshot đến 16 KiB.
  Tính năng mở rộng này mặc định tắt, không mở quyền Git mới.
- Trang E2EE tiếp tục giới hạn 320 byte, tối đa 256 trang; host đọc
  toàn bộ snapshot hợp lệ trước khi gửi trang đầu, và đối chiếu digest
  SHA-256 giữa các trang. Snapshot vượt 16 KiB bị từ chối hoàn toàn.
- QUAN TRỌNG: quota phiên Remote hiện giới hạn 16 lượt E2EE. Vì vậy,
  một diff 16 KiB có thể KHÔNG được đọc hết trong một phiên. Giao diện
  cảnh báo khi còn trang nhưng phiên gần hết lượt. Tạo phiên mới hiện
  chỉ bắt đầu lại từ trang đầu, không có resume cross-session.
- Vẫn cần nghiên cứu/chuyển thành code cơ chế review theo đoạn hoặc
  file được host phát hành để xem đầy đủ diff vượt ngân sách E2EE.


## 2026-11-09 — Timeout theo từng bước Remote workflow

- Chỉ hỗ trợ `taskdeck.workflow.timeoutSeconds` dạng **số nguyên JSON**
  từ **1 đến 30**, chỉ khi host bật riêng
  `TASKDECK_REMOTE_TASKS_WORKFLOW_TIMEOUT=1` (mặc định tắt).
  Vẫn yêu cầu các opt-in workflow/E2EE hiện có; tổng thời lượng
  của job vẫn không quá **90 giây**.
- Timeout có thể đi cùng `retry: 1` nếu và chỉ nếu host cũng bật
  `TASKDECK_REMOTE_TASKS_WORKFLOW_RETRY=1`. Timeout **không bao giờ**
  kích hoạt retry. Nếu một bước bị timeout thì kill process group,
  audit `timed_out`, dừng workflow, không bắt đầu dependency sau.
- Cả RunAvailable của GUI và executor kiểm tra cùng một policy host-only.
  Trước mỗi bước, host xác minh lại opt-in, quyền, source digest,
  executable và audit. Metadata bổ sung như `condition`,
  `continueOnError`, `outputs` vẫn bị từ chối.
- Quyền timeout bị rút trong lúc workflow hoạt động sẽ ngăn bước kế
  tiếp bắt đầu; không thay đổi quyền shell hay gửi tham số từ browser.
- Test đã bổ sung: opt-in bật/tắt, giới hạn 1–30, chính sách kết hợp,
  từ chối số không hợp lệ, timeout process thực tế và audit fail-fast.

## 2026-11-09 — Tiến độ workflow theo từng bước (typed E2EE)

- Agent và native client hiện hỗ trợ các trường **tùy chọn** trong phản
  hồi `tasks.poll`: `step`, `step_total`, `attempt`,
  `attempt_total`, `step_status`. Đây là metadata do Agent xác
  lập khi thực thi native process, KHÔNG suy đoán từ stdout của task.
- Bước 1–4, tổng 2–4, lần thử 1–2 và trạng thái từng bước chỉ cho
  phép `running/completed/failed/cancelled/timed_out`. Một phản hồi
  thiếu field, trùng key, sai kiểu hoặc nằm ngoài giới hạn bị từ chối
  và làm đóng kênh E2EE.
- Task chạy đơn vẫn sử dụng giao thức v1 với metadata bước bị bỏ
  trống; native client mới đọc được phản hồi Agent cũ và không hiển
  thị “Bước 0/0”. Agent mới khi gửi metadata bước workflow yêu cầu
  native client đã cập nhật để hiểu các trường tùy chọn.
- Poll workflow giới hạn output **64 byte/trang** (task đơn tiếp tục
  128 byte/trang), tổng capture 16 KiB; metadata và output đều nằm
  trong mailbox E2EE bị giới hạn 512 byte.
- Native GUI hiển thị “Bước X/Y · Lần A/B · Trạng thái” bên cạnh
  output. Chỉ số bước được bảo vệ bởi phiên và root Task ID đã xác
  thực, không do browser chỉ định.
- Đây là tiến độ *live/last-step*, chưa thay thế báo cáo lịch sử toàn
  bộ các bước hoặc phục hồi job giữa phiên. Cả hai vẫn thuộc backlog.

 
## 2026-11-09 — Bảng lịch sử trạng thái từng bước trong một phiên E2EE

- Bổ sung metadata tùy chọn `step_history` trong phản hồi
  `tasks.poll`. Đây là chuỗi **2–4 ký tự** do Agent phát hành:
  `-` chưa chạy, `R` đang chạy, `C` hoàn tất, `F` thất bại,
  `X` đã hủy, `T` hết thời gian. Không bao gồm stdout, tham số,
  đường dẫn, token hay secrets.
- Chỉ hợp lệ khi đi kèm đầy đủ 5 trường tiến độ host hiện có;
  chiều dài bằng `step_total`, toàn bộ bước trước bước hiện tại
  phải hoàn tất (`C`), bước hiện tại khớp `step_status`,
  các bước sau chưa chạy (`-`). Metadata sai/hư/thiếu/trùng
  làm đóng phiên E2EE; Agent cũ không gửi `step_history` vẫn tương
  thích với native client mới.
- Cập nhật trạng thái được đồng bộ bởi mutex ngay trong Agent, tại
  lúc bắt đầu/chạy retry/hoàn tất bước, không parse dòng log của
  chương trình con. Native GUI trình bày bằng text, không HTML;
  output vẫn bị giới hạn 64 byte/trang workflow và 512 byte/frame.
- Chỉ là **lịch sử trạng thái tóm tắt của job hiện tại**, giữ tới khi
  dọn job hoặc đóng E2EE session; báo cáo bền vững của nhiều phiên,
  thời lượng và mã lỗi riêng mỗi bước vẫn chưa hoàn tất.

 
## 2026-11-09 — Mã thoát và thời lượng từng bước trong Remote workflow

- Bổ sung `step_details` (metadata host-owned, tùy chọn) bên cạnh
  `step_history` trong `tasks.poll`. Cấu trúc là 2–4 phần phân
  tách bởi `|`, mỗi phần là `-` nếu đang chờ/chạy, hoặc
  `<exit_code>:<milliseconds>` khi đã kết thúc.
- Exit code nằm trong `[-1,255]` (-1 = lỗi khởi chạy/hủy hoặc
  không có mã thoát hợp lệ), thời lượng từ **0 đến 90.000 ms**.
  Bước hoàn tất thành công yêu cầu exit=0. Thời lượng tính tại
  thời điểm Agent gọi process cho **lần thử cuối** của từng bước;
  chưa phải tổng thời gian gồm preflight, audit hay các lần retry.
- Toàn bộ chuỗi được cập nhật dưới job mutex, có kiểm tra trạng thái
  từng bước, số nguyên canonical, giới hạn byte và envelope E2EE
  **512 byte**; không truyền stdout, command, argv, filepath, secret.
  Bước chưa chạy không được giả tạo exit code.
- Native GUI hiển thị ví dụ `Bước 1: Hoàn tất (exit=0, 10 ms)`.
  Các client mới vẫn chấp nhận phản hồi Agent cũ thiếu trường này.
- Đây là báo cáo chi tiết **trong phiên đang chạy**; chưa có truy vấn
  kết quả từng bước bền vững hoặc khôi phục workflow qua E2EE session.

 
## 2026-11-09 — Audit bước workflow có thể đọc lại qua phiên E2EE mới

- Đã bổ sung hành động chỉ đọc `tasks.workflow.audit`, xuất tối đa **hai**
  sự kiện **kết thúc bước** gần nhất của đúng TaskDeck user và project.
  Không phụ thuộc vào phiên cũ, job slot, mã nguồn task hiện tại hoặc
  workspace; dữ liệu đến từ audit SQLite cục bộ trên host.
- Quyền: `tasks.view` và Portal Grant đang hoạt động, kết hợp ba opt-in
  độc lập: `TASKDECK_REMOTE_TASKS_PREVIEW=1`,
  `TASKDECK_REMOTE_TASKS_REPORT=1`,
  `TASKDECK_REMOTE_TASKS_WORKFLOW_REPORT=1` (mặc định tắt).
  Không cần `tasks.run`; không có API resume hoặc lệnh mới.
- Kết quả được đưa qua E2EE: `root_task_id`, `task_id`, vị trí/tổng
  bước, lần thử, trạng thái, exit code, thời điểm UTC. Không truyền
  tên task, đường dẫn, argv, stdout, secrets, nội dung audit thô.
- Decoder áp dụng schema JSON đúng khóa, từ chối duplicate/null/số
  không canonical, không cho phép yêu cầu user/project/task ID tùy ý.
  Agent bỏ qua audit `attempted`, audit chứa thuộc tính lạ hoặc
  audit khác principal; không ghi audit mới lên PHP.
- Giao diện native có nút “Xem audit bước workflow”; nội dung hiển thị
  bằng textContent và bị xóa khi logout. Kết quả chỉ gồm **hai sự kiện
  gần nhất**, KHÔNG đồng nghĩa bản báo cáo đầy đủ một workflow hay
  khôi phục công việc; truy vấn phân trang/báo cáo bền vững toàn bộ
  chuỗi vẫn chưa hoàn tất về mã nguồn.

 
## 2026-11-09 — Phân trang audit workflow bằng cursor của Agent

- Hành động mới **`tasks.workflow.audit.next`** chỉ đọc, không nhận
  user/project/task ID, thời điểm, offset, token hay body. Cùng quyền
  `tasks.view` và các opt-in của `tasks.workflow.audit`.
- Khi bắt đầu, host quét tối đa **128 audit gần nhất** của đúng principal,
  lọc kết quả bước cuối hợp lệ, giữ snapshot tối đa **32 sự kiện** trong
  E2EE session. Mỗi trang chỉ tối đa **2 sự kiện**; trường `more`
  có xác thực cho biết còn trang hay không. Cursor chỉ tăng sau khi
  phản hồi E2EE được đóng gói thành công.
- Snapshot không bị xô lệch khi audit mới xuất hiện. Không thể yêu
  cầu Next trước Start, tua lại cursor, đọc trang quá cuối, hay sử
  dụng cursor của session khác. Bắt đầu lại sẽ tạo snapshot mới.
- Native GUI có nút “Xem tiếp audit”, ghép tối đa 32 sự kiện vào
  vùng textContent và xóa khi logout; không đưa dữ liệu sang PHP.
  Phiên vẫn chịu hạn mức 16 lượt E2EE, nên chưa chắc hiển thị hết
  32 bản ghi trong một phiên. Không tự động nối/resume cursor giữa
  các phiên; một phiên mới bắt đầu lại từ sự kiện mới nhất.

 
## 2026-11-09 — Giữ tương thích ngược cho client audit v1

- Hành động cũ `tasks.workflow.audit` luôn trả tối đa hai mục
  và **không gửi khóa `more`**; client cũ dùng decoder exact-key
  tiếp tục hoạt động với host/Agent mới.
- Native client mới dùng `tasks.workflow.audit.start` để tạo cursor
  và đọc khóa `more`, sau đó gọi `tasks.workflow.audit.next`.
  Toàn bộ hành động đều read-only và giữ nguyên RBAC + host opt-in.
- Client mới vẫn đọc được phản hồi v1 thiếu `more`. Không tự động
  tiếp tục từ client cũ hoặc vượt giới hạn 16 lượt E2EE.

 
## 2026-11-09 — Mã tương quan riêng cho từng lần chạy Remote workflow

- Mỗi lần bắt đầu workflow, Agent tạo mã ngẫu nhiên **16 ký tự hex**
  bằng hệ thống ngẫu nhiên mật mã. Mã này chỉ để **nhóm các bước
  thuộc cùng lần thực thi**, không phải session, bearer token hay
  quyền điều khiển.
- Mỗi audit `attempted/completed/failed/cancelled/timed_out` của
  workflow mới chứa `run_id`; chỉ khi cả cấu trúc và mã hợp lệ mới
  được đưa vào báo cáo bền vững. Các audit cũ thiếu mã vẫn đọc được,
  hiển thị rõ “Lượt cũ”.
- Chỉ phản hồi phân trang mới `tasks.workflow.audit.start/next`
  có trường `run_id` tùy chọn. Hành động cũ `tasks.workflow.audit`
  không phát hành `run_id` hoặc `more` để giữ tương thích với
  client v1 kiểm tra đúng số trường.
- Báo cáo có thể nhận biết hai lần chạy cùng `root_task_id`; chưa có
  lựa chọn ID chạy từ browser, API resume hoặc toàn bộ lịch sử không
  bị giới hạn. Các tính năng đó vẫn thuộc backlog.

 
## 2026-11-09 — Nhóm báo cáo các bước theo lần chạy trên native GUI

- Giao diện gom các audit bước có cùng mã `run_id` xác thực thành
  một nhóm tương ứng **một lần chạy**, dù các bước đến từ các trang
  E2EE khác nhau. Kiểm tra root task ID và tổng số bước phải thống nhất.
- Hiển thị số bước **có bản ghi kết quả trong cửa sổ snapshot**,
  không đánh đồng với số bước chắc chắn đã chạy/hoàn tất trên host.
  Retry của cùng bước vẫn là các bản ghi sự kiện riêng.
- Audit cũ không chứa mã lượt chạy chỉ hiển thị “Lượt cũ” cho từng
  bản ghi, không suy đoán ghép các lần chạy bằng ID task.
- Chỉ xử lý textContent của tối đa 32 sự kiện trong bộ nhớ GUI;
  không lưu history trong browser storage hoặc PHP Relay.

 
## 2026-11-09 — Conditional cleanup an toàn cho Remote Task workflow

- Opt-in host: `TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION=1` (mặc
  định tắt). Chỉ hỗ trợ `taskdeck.workflow.condition:"success"` và
  `"always"`, đúng chuỗi lowercase; mọi `failure`, cấu hình
  `continueOnError`, output export và policy khác vẫn bị từ chối.
- `condition:"success"` hoạt động theo fail-fast mặc định.
  `"always"` chỉ được gắn với **bước cuối/root** của workflow
  tuần tự tối đa 4 bước; không được dùng ở bước trung gian.
- Khi **bước ngay trước cuối** thất bại sau các retry đã cho phép,
  chỉ bước cuối `"always"` mới có thể chạy dọn dẹp. Nếu một bước
  sớm hơn lỗi thì dừng; không bỏ qua phụ thuộc trung gian.
- Không chạy dọn dẹp sau **timeout**, **hủy**, mất phân quyền hoặc
  host thu hồi gate; kiểm tra lại quyền chạy, hash source, cwd,
  executable và audit ngay trước process. Tác dụng phụ của bước
  dọn dẹp vẫn là hành động thực, do đó GUI cảnh báo rõ trước khi chạy.
- Sau khi dọn dẹp thành công, **kết quả tổng thể vẫn là failed** với
  mã lỗi của bước gốc. Báo cáo tiến độ E2EE có thể chứa `FC`
  hoặc `CFC` ở đúng vị trí cuối, nhưng không chấp nhận tiền tố
  lỗi ở giữa hoặc nhảy qua các bước phụ thuộc.
- Bảo vệ fail-closed và giới hạn 90 giây toàn job vẫn giữ nguyên;
  không nhận condition, lệnh, đối số từ browser. Các chế độ
  `condition:"failure"`, nhánh rẽ, song song, truyền output và
  `continueOnError` vẫn là những phần chưa hoàn tất.

 
## 2026-11-09 — Final recovery condition:failure

- Opt-in riêng `TASKDECK_REMOTE_TASKS_WORKFLOW_CONDITION=1` tiếp tục
  bắt buộc. `taskdeck.workflow.condition:"failure"` giờ được cho
  phép **chỉ trên bước cuối/root** trong DAG tuần tự an toàn.
- Nếu bước ngay trước thất bại (mã exit thực), bước cuối được phép
  chạy recovery; kết quả tổng thể vẫn **failed** và giữ mã lỗi gốc
  kể cả khi recovery cũng thất bại. Các bước phụ thuộc trung gian
  không bị bỏ qua; không chạy sau cancel, timeout, source drift,
  revoked role/grant hoặc opt-in bị tắt.
- Nếu toàn bộ bước trước thành công, final recovery không chạy:
  trạng thái workflow **completed**, E2EE tiến độ ghi `S` với
  `step_status:"skipped"`, `step_details:"-"` cho final step.
  Đây không phải exit code thành công giả và không có audit
  `attempted` cho process không được thực thi.
- Bộ giải mã E2EE và native GUI chỉ chấp nhận `S` ở **cuối cùng**,
  sau các bước hoàn tất `C` và với kết quả tổng thể completed.
  Các lịch sử `FS`, `S-`, `CSS`, skipped có exit giả và skipped
  khi aggregate failed đều bị từ chối.
- Phản hồi giao thức từ Agent cũ không phát hành trạng thái skipped
  vẫn tương thích với client mới. Client cũ không hiểu skipped
  có thể từ chối phản hồi nếu bật opt-in mới; vì vậy khi phát hành
  Agent có điều kiện failure, phải cập nhật native client tương ứng.
- Báo cáo audit bền vững hiện lưu kết quả các process đã chạy, không
  tạo bản ghi process giả cho bước bỏ qua; báo cáo workflow trọn
  vẹn gồm các bước skipped vẫn thuộc backlog.

## 2026-11-10 — Workflow completion barrier dùng chung với task đơn

- `runRemoteLinearWorkflow` trước đây công bố `job.status` trước
  `workspaceRelease`, và không đánh dấu `job.finalizing`. Vì thế
  `tasks.cancel` có thể ACK một workflow đã chạy xong nhưng còn
  đang giải phóng workspace; Poll cũng có thể thấy kết quả cuối quá sớm.
- Sử dụng `job.publishCompletion(status,exitCode)` đã kiểm thử của task
  đơn: đánh dấu `finalizing` trước release, công bố trạng thái và đóng
  `done` sau release. Cancel muộn bị từ chối; không đổi E2EE, RBAC,
  quota, lệnh thực thi, retry hoặc workflow conditions.
- Hồi quy mới kiểm tra workflow `completed` và `failed`: đã qua
  finalization barrier, Cancel muộn bị từ chối, kết quả giữ nguyên,
  và Clear vẫn thành công. Test helper hiện có đã kiểm tra barrier
  chặn release với bốn trạng thái task đơn.
- Xác nhận Go CI trên commit mới trước khi khẳng định PASS.
  Remote live gate vẫn mặc định OFF/NO-GO; không mở rộng Windows.

## 2026-11-10 — Khép cửa sổ Cancel trước audit cuối

- Khóa `finalizing` ngay sau khi task đơn đã reaped process, hoặc sau
  vòng lặp chạy workflow, **trước** khi phân loại kết quả và ghi audit
  kết thúc. Trước đây Cancel có thể đến trong lúc audit và nhận ACK
  dù không còn process để hủy.
- `publishCompletion` tiếp tục nhắc lại trạng thái `finalizing`
  (idempotent), chờ release workspace rồi mới công bố trạng thái cuối,
  đóng `done` và đánh thức Poll. Cancel trước `beginFinalizing`
  vẫn được xử lý theo đường hủy hợp lệ; Cancel sau đó bị từ chối.
- Thêm test chặn Cancel giữa lúc đã bắt đầu finalization và trước khi
  bắt đầu release, giữ nguyên trạng thái đang chờ audit; kiểm tra
  Clear sau khi publish. Workflow success/failure integration ở commit
  trước vẫn kiểm tra cùng helper. CI phải được kiểm tra trên SHA mới.

## 2026-11-10 — Atomic snapshot của Cancel trước phân loại trạng thái

- `beginFinalizing()` đánh dấu rào chắn và chụp `cancelRequested`
  trong cùng critical section của `job.mu`, cũng là lock mà
  `cancelRemoteLongTask` sử dụng. Cancel đã nhận ACK trước rào chắn
  không thể bị bỏ sót dù lời gọi `job.cancel()` chưa kịp truyền
  vào `context`. Cancel tới sau rào chắn bị từ chối.
- Task đơn và workflow đều tính `cancelled` từ snapshot này hoặc
  context đã hủy, trừ trường hợp deadline đã hết (giữ ưu tiên timeout).
  Hồi quy bổ sung kiểm tra snapshot cả khi có và không có Cancel trước
  boundary. Không thay đổi wire protocol, tối đa E2EE hay chính sách
  quyền. Kiểm tra workflow CI trên commit mới.

## 2026-11-10 — Lịch sử bền vững cho recovery cuối không chạy

- Khi task `condition:"failure"` ở bước cuối bị bỏ qua vì mọi
  dependency đều thành công, host giờ ghi **một kết quả audit
  `skipped`**, không ghi `attempted` và không tạo process.
  Đường audit lỗi sẽ fail-closed thay vì báo workflow thành công
  mà không có lịch sử. Audit `skipped` lưu `exit_code:-1` rõ
  ràng, không giả kết quả chạy process thành công.
- Bộ giải mã E2EE chỉ chấp nhận `skipped` ở **bước cuối/root**,
  `attempt:1`, `exit_code:-1`; cùng phép kiểm tra độc lập
  trong native GUI. Audit phiên cũ thiếu trạng thái skipped vẫn
  hợp lệ. Trình xem hiển thị “bỏ qua (không chạy process)” và
  có thể nhóm với `run_id` của execution sau khi đăng nhập lại.
- Test Go xác minh audit bền vững, số bản ghi, không có attempted
  cho recovery, truy xuất audit qua E2EE; test codec từ chối
  skipped trung gian/sai task/sai exit/retry. Test DOM native
  kiểm tra hiển thị bản ghi skipped và các run cũ.
- Giữ giới hạn cửa sổ audit 32, phân trang 2, per-principal RBAC,
  payload E2EE không chứa lệnh/output/password và default host opt-in
  OFF. LIVE security gate vẫn NO-GO.

## 2026-11-10 — Tương thích client cũ sau khi thêm audit skipped

- `tasks.workflow.audit` v1 giữ nguyên đúng hai slot tối đa,
  không `run_id`, không `more`, và chỉ phát hành bốn trạng thái
  cũ `completed/failed/cancelled/timed_out`.
  Lọc các bản ghi `skipped` từ snapshot host **trước khi chọn
  tối đa hai kết quả**, để không làm mất các bản ghi cũ còn hợp lệ.
- V1 không tạo/cấp quyền cursor tiếp theo; giải phóng snapshot ngay
  sau khi seal. Chỉ `tasks.workflow.audit.start/next` được phát hành
  skipped, run ID và cursor snapshot hiện đại.
- Kiểm thử integration tạo workflow recovery bị bỏ qua: giao thức mới
  thấy bản ghi skipped + run_id, v1 chỉ thấy kết quả bước đã chạy,
  không có metadata mới, không có cursor. Không thay đổi quyền,
  giới hạn audit, transport hoặc khả năng thực thi.

## 2026-11-10 — Git Pull/Fetch/Push: ngăn proxy và bí mật HTTP trong repo

- Git config của repository, kể cả `includeIf` và các khóa scoped theo URL,
  không còn được phép điều hướng request qua `http.proxy` hoặc
  `http.proxyAuthMethod`; không được tự thêm `http.extraHeader`,
  `http.cookieFile`, `http.saveCookies`, `http.emptyAuth`,
  `http.sslCert`, `http.sslKey` hay `http.curloptResolve`.
  Đây là các kênh có thể đổi endpoint hoặc chuyển dữ liệu xác thực/
  private-key ngoài chính sách remote được phê duyệt.
- Các lệnh Pull/Fetch/Push dùng chung preflight fail-closed cũ,
  giữ nguyên chính sách chặn `sslVerify` và `followRedirects`.
  `http.sslCAInfo` của host vẫn được chấp nhận để hỗ trợ CA doanh
  nghiệp hoặc chứng chỉ test HTTPS, nhưng TLS verification bắt buộc.
- Test tích hợp thêm các cấu hình local/URL-specific, kiểm tra
  policy từ chối và **không có request tới server** cho cả ba
  thao tác. Không cho browser truyền config/path/remote/credential.
  Cần xác nhận PASS của CI Go/PHP trên SHA cuối trước release.

## 2026-11-10 — Remote GUI: gắn Task.ID và nhãn đã xác thực vào tiến độ workflow

- Sau khi người dùng xem dependency graph đã xác thực qua E2EE,
  native GUI ghi tạm thứ tự bước và nhãn task (tối đa bốn bước,
  nhãn tối đa 72 ký tự). Khi phản hồi workflow-start hợp lệ, GUI
  **đóng băng bản đồ bước cho đúng lượt chạy**. Không dùng nhãn
  này để cấp phép hoặc gửi đối số mới tới host.
- Màn hình Poll trình bày cả `step/step_total`, attempt, trạng thái,
  mã thoát/thời gian và `Task #ID · tên task` cho từng bước. Nếu
  dependency chưa được tải vào danh sách preview, chỉ hiển thị ID
  đã được host gửi trong graph; không suy đoán nhãn.
- Làm mới/chỉnh sắp xếp danh sách task không sửa bản đồ của job
  đang chạy. Clear và logout xóa bản đồ riêng tư, trong khi start
  một task đơn không được thừa hưởng nhãn của workflow trước.
- Test native DOM kiểm tra DAG ID, tên có HTML đặc biệt vẫn chỉ là
  `textContent`, snapshot được đổi tên sau khi job đã chạy không
  làm lệch nhãn trong tiến độ. Không thay đổi giao thức E2EE hay
  giới hạn 16 lượt hành động và 16 KiB output.

### 2026-11-10 — Kiểm thử giữa hai lần Poll

- Tăng cường test native: trong lúc workflow vẫn `running`,
  đọc lại dependency (host trả unavailable), đổi nhãn task trên
  trang preview, rồi nhận Poll kết thúc. Xác minh đường render
  **sau** refresh vẫn giữ Task.ID/nhãn đã chốt khi start.
  Tránh kiểm thử giả chỉ xem nội dung DOM cũ chưa được render lại.

## 2026-11-10 — Poll output chỉ commit sau E2EE Seal

- E2EE Poll nay trả snapshot và commit closure, không tăng con trỏ
  output ngay lúc đọc. Chỉ sau khi SealRemoteLongTaskProgress mã hóa
  thành công mới tăng cursor dưới job.mu. Seal lỗi giữ nguyên output
  để request sau đọc lại, không làm mất dữ liệu âm thầm.
- Poll trực tiếp nội bộ giữ nguyên cơ chế tiêu thụ ngay sau đọc;
  commit lỗi thời không được ghi đè cursor của reader khác.
- Kiểm thử thêm trường hợp không seal rồi Poll lại, seal thành công
  tiếp tục đọc phần đuôi, callback lỗi thời và Poll timeout recheck.
  Giữ nguyên timeout, giới hạn chunk và phân quyền hiện hữu.

### 2026-11-10 — Đưa deferred task Poll vào CI race preflight

- Workflow `TaskDeck Remote Agent (Go)` kiểm tra ngay đầu pipeline
  bằng `go test -race` các hồi quy long task: finalization,
  timeout recheck, cursor chưa Seal không được tăng, và timeout
  có output chưa Seal. Không chờ chạy toàn bộ test Go sau cùng
  mới phát hiện mất output trong giao thức task.

### 2026-11-10 — Hồi quy dispatcher E2EE Seal lỗi

- Bổ sung test thực tế đi xuyên `executeRemoteE2EEPing` với
  client/agent E2EE đã xác thực, job hoàn tất và một đoạn output đang chờ.
  Cố tình tạo tiến độ workflow có `step_status` không hợp lệ để bộ
  mã hóa phản hồi từ chối. Xác nhận **không trả frame** và
  `outputCursor` vẫn bằng 0; sửa metadata rồi Poll lần hai phải
  trả đúng nguyên đoạn output, Poll lần ba không trả trùng.
- Đưa test này vào bước `go test -race` preflight đầu pipeline,
  cùng với test deferred cursor. Không thay đổi schema hay host opt-in.

## 2026-11-10 — Terminal Poll chỉ tiêu thụ output sau E2EE Seal

- Terminal Poll của native E2EE dispatcher nay đọc snapshot deferred.
  Cursor PTY chỉ tăng sau khi Seal AEAD hoàn tất thành công;
  nếu lỗi kiểm tra schema/kênh, một Poll hợp lệ sau đó nhận lại
  chính xác đoạn dữ liệu chưa gửi. Poll trực tiếp nội bộ vẫn tiêu
  thụ khi đọc như trước, không đổi API host.
- Tạo `terminalActionMu` scoped theo session để tuần tự hóa
  Open/Poll/Write/Resize/Close qua bước Seal và cập nhật cursor,
  không cho hai Poll cùng tiêu thụ một đoạn hay Close vượt Poll.
  Session.Close không đợi mutex này. Commit chỉ thuộc đúng job
  đã đọc và không ghi đè con trỏ do reader khác cập nhật.
- Test `go test -race`: dispatcher E2EE trả lỗi khi host PTY
  state không hợp lệ, không mất output; Poll hợp lệ kế tiếp đọc
  chính xác dữ liệu, Poll sau không trùng. Unit test khác kiểm tra
  callback lỗi thời không ghi đè cursor và vẫn đọc được phần đuôi.
  Windows vẫn từ chối Terminal, không mở rộng quyền/route PHP.

## 2026-11-10 — Native Remote Task GUI: bỏ qua dependency reply lỗi thời

- Người dùng có thể đổi task đang chọn trong khi lệnh xem dependency
  đang đợi phản hồi từ Agent. Việc đổi lựa chọn lập tức xóa đồ thị
  đang hiển thị, hủy trạng thái `run_available` và bản đồ workflow
  đã chuẩn bị; không gửi lệnh hoặc tự động chạy task.
- Nếu phản hồi cũ đến sau, GUI vẫn **kiểm tra đầy đủ** ID, schema,
  edges, topo, giới hạn DAG và cờ `remote_control_available` của
  dữ liệu E2EE; nhưng không dùng đồ thị đó để khôi phục nút chạy và
  không thu hồi phiên chỉ vì người dùng thay đổi lựa chọn.
  Chỉ kết quả đúng lựa chọn đang chờ mới được hiển thị.
- Kiểm thử DOM giữ một request đang chờ, đổi lựa chọn, giải phóng
  phản hồi, xác minh phiên không bị đóng và nút Run vẫn vô hiệu,
  rồi chọn lại task và kiểm tra workflow được duyệt bình thường.
  Không thay đổi giới hạn action, quyền hoặc giao thức.

### 2026-11-10 — Hồi quy bảo mật khi selection bị bỏ giữa request

- Kiểm thử thêm trường hợp người dùng đổi lựa chọn trong lúc phản hồi
  dependency **không hợp lệ** đang chờ: GUI vẫn phải kiểm tra graph
  được giải mã, từ chối cạnh ngược topo và đóng/xóa phiên theo
  fail-closed. Phân biệt rõ reply hợp lệ nhưng lỗi thời (bỏ qua UI)
  với reply sai cấu trúc (tiếp tục từ chối theo giao thức).

## 2026-11-10 — Copy output trong Remote GUI chạy task

- Bổ sung nút sao chép output **đã nhận** của job dài, cùng nút
  sao chép trong từng kết quả lịch sử (task ngắn và dài).
  Chỉ kích hoạt khi người dùng bấm và phiên browser E2EE vẫn
  hợp lệ. Output được copy là đúng chuỗi text đã giải mã trong
  bộ nhớ, không kèm nhãn/task ID và không đánh giá HTML.
- Dùng `navigator.clipboard.writeText` của trình duyệt an toàn;
  khi không khả dụng hoặc bị từ chối, hiển thị trạng thái lỗi
  tại nút và không tự chuyển sang đường copy kém an toàn.
  Không lưu output lên localStorage/sessionStorage, PHP/Relay
  hoặc audit. Job đã Clear không còn nút copy active, nhưng
  kết quả còn trong lịch sử phiên vẫn được sao chép.
- Callback và nút lịch sử cũ không được phép copy sau logout,
  kể cả khi DOM cũ còn bị bên thứ ba giữ tham chiếu; kết quả
  async về muộn không được cập nhật giao diện đã thu hồi.
- Test Node GUI kiểm tra output có chuỗi HTML đặc biệt được
  copy đúng nguyên văn, không lẫn header, không copy khi đóng
  phiên, và nút active vô hiệu sau Clear.

## 2026-11-10 — Xóa promptString khỏi GUI sau xác nhận chạy

- Remote GUI hiện xóa mã tham số `promptString` khỏi ô nhập
  và bộ nhớ nháp của task sau khi người dùng xác nhận thực thi
  một lệnh task ngắn/dài. Bản POST đã chốt vẫn được gửi qua
  E2EE như trước; task không thay đổi đầu vào.
- Nếu người dùng từ chối xác nhận hoặc nhập mã không hợp lệ,
  giữ nguyên dữ liệu trong ô để chỉnh sửa, không gửi lệnh.
  Các lựa chọn `pickString` từ host không bị xóa.
- Kiểm thử Node tái tạo việc người dùng điền và ghi nháp mã,
  từ chối rồi xác nhận, kiểm tra không còn mã trên DOM và
  không khôi phục sau filter/rerender. Đây không phải cơ chế
  nhập secret; giới hạn token và chính sách không hỗ trợ
  `secretEnv` từ xa vẫn giữ nguyên.

### 2026-11-10 — Long-task promptString scrub regression

- Test thêm nhánh `tasks-start-input/<Task.ID>`: xác minh
  cùng cơ chế xóa prompt sau xác nhận áp dụng cho job dài,
  bản E2EE POST vẫn nhận đúng token, sau filter/rerender không
  khôi phục token, job được giữ trong session. Không có quyền
  mới hay nguồn input ngoài `promptString` token hiện hữu.

### 2026-11-10 — Fix fixture test promptString cho task dài

- CI Go đã bắt đúng lỗi fixture: nhánh task dài lên lịch auto-Poll
  nhưng mock VM của test promptString chưa khai báo `setTimeout`,
  làm Node ném ReferenceError *sau khi* các assert đã PASS.
- Bổ sung timer no-op trong test fixture, giống các fixture native
  khác. Không chạy Poll nền trong test bảo mật input vì auto-Poll
  đã có bộ test riêng; production GUI không đổi.

## 2026-11-10 — Remote Git sau khi job dài hoàn tất (giữ output)

- Git workspace mutation của host nay cho phép một job dài
  **đã finalizing, có done channel đóng, trạng thái terminal
  completed/failed/cancelled/timed_out** vẫn được giữ ở phiên
  để xem output/lịch sử. Không yêu cầu Clear trước khi Fetch/
  Pull/Commit/Push hoặc thao tác Git được duyệt khác.
- Trạng thái đang chạy, đang nhả workspace, channel nil,
  kết quả bất nhất và PTY đang mở vẫn chặn Git. Lease canonical
  per-workspace và OS directory lock tiếp tục chống tranh chấp
  giữa nhiều E2EE session, kể cả job kết thúc bất thường.
- GUI chỉ bật Git khi đã nhận **phản hồi cuối từ Agent**, vẫn
  giữ ba E2EE round cho Cancel/Poll/Clear nếu quota thấp.
  Output và lịch sử task không bị xóa khi chạy Git; chỉ Clear
  hoặc logout mới ảnh hưởng trạng thái job.
- Test Go kiểm tra chạy/finalizing/lease còn giữ bị từ chối,
  cho Git khi done & lease đã nhả, Poll output vẫn nhận được,
  Clear vẫn hoạt động. Test native GUI dùng job có output
  chưa nhận hết, chặn Git khi running, cho Git sau completed
  và giữ nguyên transcript/history. Không thêm API, quyền
  hoặc token Git mới.

### 2026-11-10 — Sửa hồi quy giới hạn Git-only sau khi job kết thúc

- CI Go phát hiện các thay thế quá rộng: điều kiện giữ quota 3 lượt E2EE của bulk task paging dùng nhầm cờ Git-mutation, khiến regression autopolled fail. Khôi phục đúng `longTaskID!==0` cho tất cả nhánh bulk, không đổi cơ chế Poll/Cancel/Clear.
- Khôi phục cấm mở Terminal mới và khởi chạy workflow khi job hoàn tất vẫn đang giữ slot (chưa Clear). Chỉ **Git** được phép tiếp tục khi host đã xác nhận nhả khóa workspace; không mở rộng sang task hoặc PTY khác.
- Test native GUI kiểm tra rằng một job completed giữ output vẫn chặn TerminalOpen kể cả khi click giả; test autopolled tiếp tục kiểm tra điều kiện giữ quota 3 lượt.

## 2026-11-10 — Git Pull làm mới tính hợp lệ danh sách Remote Task

- Sau khi E2EE `Git pull --ff-only` được xác nhận đã thay đổi HEAD, native GUI ẩn toàn bộ danh sách task đã tải, Task.ID đã inspect, bản đồ dependency có thể chạy, các prompt draft và phân trang cũ. Người dùng phải chọn Xem trước task để lấy snapshot/digest mới từ Agent trước khi chạy task. Tránh gọi một Task.ID cũ và bị Agent từ chối theo hash của `.vscode/tasks.json` mới.
- Chỉ thay đổi khi kết quả Pull có `updated=true`; Pull không cập nhật HEAD không cần xóa danh sách. Kết quả thực thi đã giải mã trong lịch sử và output của job kết thúc được giữ độc lập. Không cấp thêm quyền hay tự khởi chạy task.
- Test native DOM tái hiện host trả ID 55 trước Pull và 66 sau Pull: nút của dòng cũ không thể gửi lệnh, danh sách cũ biến mất, prompt draft không hồi sinh, ID mới phải inspect lại. Đưa test vào CI Go.

### 2026-11-10 — Chặn callback cũ ngay cả khi Task.ID được dùng lại

- Mỗi lần native GUI dựng lại danh sách task sẽ tăng `taskPageRenderEpoch` của UI. Nút Kiểm tra ID và Chạy task chỉ gửi request nếu callback thuộc đúng bản render hiện hành, ngoài kiểm tra Task.ID đã phát hành và metadata inspect. Khi append trang mới không làm thay đổi các dòng đang hiển thị, giữ nguyên DOM và epoch như cơ chế tối ưu trước đó.
- Git pull làm thay đổi HEAD cũng làm mất hiệu lực các callback cũ ngay lập tức. Test bổ sung tình huống snapshot mới phát hành lại cùng ID 55: nút cũ đã bị tháo khỏi DOM không thể tiêu thụ lượt E2EE hoặc chạy task.

## 2026-11-10 — Terminal GUI: tự nhận output sau thao tác (opt-in)

- Bổ sung **Bật/Tắt tự nhận output** ở native Remote Terminal. Mặc định tắt để không tiêu hao lượt E2EE ngoài ý muốn. Khi bật, GUI đọc một lần sau Open/Write/Resize và chỉ đọc tiếp nếu Poll xác thực báo `more=true`. Không chạy Poll định kỳ khi shell nhàn rỗi: tiến trình nền sinh output muộn vẫn dùng nút Đọc output thủ công.
- Poll thủ công ưu tiên hơn callback đang lên lịch. Pause/Close/Logout tăng epoch để callback cũ không gửi request, và không Poll khi một action khác đang bận. Luồng vẫn dùng endpoint `/api/terminal-poll` E2EE cũ; không tạo route/socket/credential và không chuyển PTY qua PHP.
- Bộ lập lịch kiểm tra `terminalMaintenanceAllowed('/api/terminal-poll')` trước khi đặt timer **và trước khi gửi request**, giữ lượt E2EE cuối cùng cho Close; status và output của host tiếp tục được xác thực ở dispatcher và render `textContent`.
- Test native GUI dùng timer có kiểm soát để xác nhận bật/tắt, backlog nhiều chunk, Poll thủ công không bị nhân đôi, dừng khi idle và quota=1 vẫn Close thành công. Đã thêm vào Go CI.

## 2026-11-10 — Lịch sử output task đúng giới hạn UTF-8

- Preview lịch sử output của một job dài giờ giữ đúng tối đa **2.048 byte UTF-8 cuối** đã giải mã thay vì 2.048 UTF-16 code unit. Các chuỗi có emoji/ký tự nhiều byte không bị cắt đôi surrogate pair, không tạo chuỗi invalid khiến Clipboard copy lỗi.
- Dấu `output đã cắt` xuất hiện nếu preview buộc phải bỏ ký tự đầu, bất kể JS `.length` còn thấp. Toàn bộ transcript job trong panel đang chạy vẫn giữ nguyên (host giới hạn capture 16 KiB). Không thay đổi API, quyền E2EE, cơ chế Poll/cursor, PHP/Relay.
- Test native GUI kiểm tra 1.024 emoji chỉ giữ 512 emoji (2.048 byte), cắt chính xác ranh giới UTF-8, không sinh unpaired surrogate, sao chép output xác thực nguyên văn và nút copy cũ mất hiệu lực sau logout. Thêm vào Go CI.

## 2026-11-10 — Giữ đúng một lượt Clear sau job đã hoàn tất và hết output

- Giảm mức bảo lưu quota **chỉ cho mutation Git** khi Agent đã xác nhận trạng thái terminal của job và `more=false`. Với hai lượt E2EE còn lại, người dùng có thể Git một lần, vẫn giữ lượt cuối để Clear. Nếu còn output chưa đọc (`more=true`) ở mức quota thấp, hoặc job vẫn chạy/đang finalizing, Git vẫn bị khóa.
- Giữ ba lượt để Cancel/Poll/Clear trong vòng đời job đang hoạt động. Các thao tác task/PTY khác vẫn chặn đến khi Clear; không thay đổi host RBAC, E2EE, `session.longTask`, workflow hay nút Poll.
- Dùng chung danh sách endpoint Git chính xác cho khóa mutation và khóa quota để tránh nới lỏng nhầm bất kỳ route nào. Test native giả lập quota=2: Git hợp lệ khi job completed + hết output và tiếp đó Clear thành công, nhưng không được Git khi vẫn còn output cần Poll/Clear.

## 2026-11-10 — Xóa Git diff/review lỗi thời sau mutation thành công

- Bổ sung hai helper nội bộ native GUI để thu hồi cache xem diff (HEAD/staged, trang tiếp theo, mode và nội dung cũ) và hai bản xem trước branch switch/delete khi Git index hoặc HEAD vừa đổi.
- Sau Stage/Unstage (file hoặc toàn bộ): không tiếp tục đọc staged patch cũ và không cho xác nhận bản review nhánh đã lập trước khi index đổi.
- Sau Commit: xóa patch và review nhánh cũ; sau Pull `updated=true`: xóa patch/review nhánh, đồng thời giữ cơ chế làm mới Task.ID đã có. Pull không làm thay đổi HEAD vẫn giữ patch, tránh làm mất bản xem hợp lệ không cần thiết.
- Các nút `Git patch Next` đã bị vô hiệu sẽ không gửi lệnh kể cả khi bị gọi bằng callback cũ. Mọi thao tác đọc lại bắt đầu từ `/api/git-patch-start` đã xác thực; không thêm route/permission/đối số hoặc chạm lịch sử/output task. Test native DOM mô phỏng Stage→Commit→Pull và kiểm tra không còn cache cũ hay lệnh Next dư.

### 2026-11-10 — Đồng bộ bản xem trước nhánh sau tạo/chuyển/xóa

- Hoàn thiện cùng một quy tắc thu hồi preview cho `git-branch-create`, `git-switch-same-head`, `git-branch-delete`: sau mutation thành công, kế hoạch branch switch/delete được duyệt từ danh sách nhánh trước đó không còn bấm được. Tạo/chuyển nhánh cũng xóa diff đã giải mã vì nguồn review có thể cần được đọc lại.
- Test native GUI mô phỏng nhánh tạo mới ngay sau một switch review, và switch thực hiện sau khi đã duyệt delete: các nút xác nhận từ kế hoạch cũ tự vô hiệu. Không đổi API/RBAC/arg Git, mọi lệnh vẫn xác minh lại trên Agent.

## 2026-11-10 — Theo cuối output task dài, không làm mất vị trí đang đọc

- Khi nhận Poll mới, GUI dùng `textContent` như cũ nhưng chỉ tự cuộn vùng output về đáy khi người dùng đang ở gần đáy (khoảng 28px) hoặc toàn bộ nội dung trước đó vừa màn hình. Nếu đang cuộn lên đọc log cũ, giữ nguyên `scrollTop`; đến gần đáy thì tự theo log mới. Khi một job mới bắt đầu hoặc Clear, đặt scroll về đầu.
- Thao tác chỉ cập nhật DOM trên client, không tăng lượt E2EE, không gửi dữ liệu tới PHP/Relay, không đổi logic output cursor hay quyền thực thi task. Thêm test Node VM cho các trường hợp đang đọc cuối/đọc đầu/quay lại cuối/reset và chạy trong CI Go.

## 2026-11-10 — Xóa lịch sử không làm mất kết quả của job đang chạy

- Khi người dùng chọn Xóa kết quả, native GUI chỉ xóa các bản ghi đã hoàn tất trước đó nếu host vẫn đang giữ một job dài chưa Clear. Bản ghi job hiện tại được bảo lưu theo serial của **lượt chạy** (không chỉ theo Task.ID), nên các Poll sau đó còn cập nhật đúng trạng thái/output cuối. Khi chỉ còn một bản ghi active, nút xóa bị vô hiệu và hiển thị nhãn **Xóa kết quả cũ**; sau khi Clear host, bản ghi cuối có thể xóa bình thường.
- Test GUI tái hiện lịch sử có một task cũ và một long job đang chạy, xóa kết quả, Poll rồi Cancel/đọc final và chạy lại cùng Task.ID. Không thay đổi E2EE, hành vi Cancel/Clear, output limit hoặc lưu trữ lịch sử chỉ trong phiên.


## 2026-11-11 — Kiểm tra quyền lần cuối sau khi workflow process thoát

- Một process ngắn có thể thoát giữa hai lượt watchdog (500ms) trong khi host
  đã thu hồi `TasksRun`. Trước khi ghi audit thành công/thất bại cho từng
  bước, executor hiện xác minh lại **quyền RBAC trên host** và mọi cờ opt-in
  liên quan, với giới hạn đọc identity 2 giây. Không có round E2EE hay
  kết nối Portal mới chỉ để thực hiện xác minh này.
- Áp dụng cho cả workflow tuần tự và từng sibling/final root song song.
  Khi quyền không còn, kết quả bước thành `cancelled`, exit=-1 và
  không chạy retry/cleanup/root sau đó. Đây là fail-closed accounting,
  **không** thể hoàn tác side effect mà process đã thực hiện trước lúc bị
  thu hồi quyền.
- Bổ sung regression test cho TasksRun role downgrade, opt-out host và
  context hủy tại điểm post-exit. Không mở rộng quyền chạy task, input,
  session lifetime, giao thức PHP/Relay hay hỗ trợ Windows.
- Chỉ tính code hoàn tất sau khi CI Go và PHP đúng SHA PASS; chạy thực địa
  không phải điều kiện để đánh dấu hoàn tất về mặt mã nguồn.


## 2026-11-11 — Git Pull / Commit / Push: host opt-out tại biên audit

- Mỗi thao tác Git ghi có một bước audit `attempted` bền vững trước khi
  thay đổi HEAD/index/ref hoặc gọi network. Nếu chủ host chuyển cờ opt-in
  về `0` trong lúc ghi audit, quyền trước audit không được phép tiếp tục.
- Ba chức năng Pull / Commit / Push nay xác minh lại **đúng cờ host**
  đã cho phép, trạng thái E2EE, grant đã ký và context ngay sau audit,
  trước khi phát lệnh Git side-effect. Cờ không thuộc danh sách cố định
  bị từ chối. Không thay đổi args Git, URL/credentials, force/rebase,
  PHP Portal hay giao thức E2EE.
- Regression test gồm cờ của cả ba thao tác, giả lập close/context hủy,
  và SQLite audit callback đổi cờ Commit ngay sau `attempted`: HEAD
  phải giữ nguyên, staged index phải còn nguyên. Không khẳng định thao
  tác Git đã thực hiện là có thể rollback nếu quyền bị thu hồi muộn hơn.


## 2026-11-11 — Remote Git Pull/Commit/Push: bảo vệ GitWrite sau audit

- Git Pull, Commit, Push trước đây đã kiểm tra RBAC khi vào dispatcher
  và kiểm tra cờ host sau audit `attempted`. Tuy nhiên, một thay đổi
  vai trò TaskDeck trong lúc ghi audit có thể thu hồi `GitWrite`
  mà không đóng session E2EE hoặc tắt cờ host.
- Ba action giờ xác minh lại TaskDeck principal/membership/approved-key
  **trên host** (timeout giới hạn), đối chiếu `PermissionGitWrite`
  và vai trò Portal, sau đó kiểm tra lại cờ host/phiên/grant trước
  khi khởi chạy Git side-effect. Không sử dụng quyền cũ trong GUI.
- Regression gồm test cho cả ba action và SQLite audit wrapper thu hồi
  GitWrite ngay sau `AppendAudit` của Commit. Xác nhận HEAD bất biến,
  staged index không mất, `attempted` audit vẫn tồn tại. Go CI kiểm tra
  sớm nhóm regression này bằng `-race`.
- Không đổi URL, argv, credential policy, force/rebase, PHP/Relay hay
  bất kỳ giao thức mới nào. Không thể hoàn tác thay đổi mà Git đã hoàn
  thành nếu quyền bị thu hồi *sau khi* side effect bắt đầu.


## 2026-11-11 — Terminal PTY: chặn audit-time role drift và đang chạy

- Terminal OPEN đã có per-action E2EE+RBAC, nhưng một thay đổi vai trò
  `TerminalCreate`/`TerminalControlOwn` trong lúc lưu audit attempted có
  thể lọt qua trước khi `pty.StartWithSize` được gọi. OPEN nay xác minh
  lại quyền và host opt-in trên SQLite trước khi tạo bất cứ shell nào.
- WRITE và RESIZE cũng xác minh lại sau audit. Nếu đã bị thu hồi quyền,
  từ chối lệnh và gọi cancel PTY để ngăn process tiếp tục chạy.
- Một watchdog giới hạn 500 ms đọc quyền host-local trong lúc PTY còn
  sống, hủy toàn bộ phiên shell khi host tắt cờ Terminal hoặc vai trò
  mất quyền. Tận dụng `job.cancel` sẵn có (kill group/session, cleanup
  workspace); không làm chậm vòng Poll hay thêm E2EE request.
- Test hồi quy: audit wrapper đổi vai trò ngay sau OPEN attempted không
  được tạo PTY hay rò lease; PTY đang chạy `sleep` bị dừng khi tắt
  cờ hoặc đổi role, không thực thi lệnh phía sau. Test chạy trong Go
  preflight với race detector, không mở rộng windows/credential/PHP.
- Mức độ bảo đảm: đây là giới hạn quyền và cleanup của PTY đã tạo;
  không thể rollback lệnh đã thực thi trước thời điểm thu hồi quyền.

 
## 2026-11-11 — Remote Terminal: chờ drain output PTY trước trạng thái exited

- Một shell có thể thoát ngay sau khi in output. Trước đây `cmd.Wait`
  đánh dấu `exited`, đóng `job.done`, rồi đóng TTY ngay trong khi
  `io.Copy` còn đang đọc. GUI có thể nhìn thấy trạng thái cuối và
  bỏ lỡ đoạn output cuối cùng (đặc biệt với lệnh ngắn).
- Sau `cmd.Wait`, tạo sự kiện `reaped` riêng với `done` để tránh
  gửi SIGKILL tới PID đã được Wait, đồng thời đợi reader nhận EOF
  trong giới hạn thời gian. Nếu slave không đóng, cưỡng bức đóng
  master và đợi reader thêm một khoảng giới hạn; trường hợp hiếm
  quá hạn được đánh dấu `truncated` thay vì khai báo log đầy đủ.
- Chỉ công bố `exited`, nhả workspace và đóng `job.done` sau khi
  đã xử lý drain và cleanup. Không ghi output lên audit/PHP/Relay,
  không thêm E2EE round, không mở lại PTY sau mất kết nối, không
  thay đổi quota 16 round, giới hạn capture 16 KiB hoặc 90 giây.
- Regression dùng PTY thật in giá trị chỉ được tính tại host, sau đó
  thoát ngay; khi `job.done` đóng, Poll phải nhận giá trị đầy đủ
  đúng một lần và workspace phải đã nhả. Thêm preflight Go `-race`.


## 2026-11-11 — Terminal PTY: cửa sổ output cuộn trong giới hạn 16 KiB

- Trước thay đổi, `remoteLongTaskCapture.Write` chỉ giữ 16 KiB đầu và
  đánh dấu `truncated` vĩnh viễn, khiến Terminal không nhận được output
  mới khi shell tiếp tục chạy, dù client đã Poll hết nội dung cũ.
- Thêm constructor `newRemoteTerminalCapture` **riêng cho PTY**,
  dùng bộ đệm rolling UTF-8 tối đa 16 KiB, cursor byte tuyệt đối tăng
  đơn điệu và chỉ drop dữ liệu cũ ở ranh giới rune. Stream rất dài có
  thể liên tục được Poll nếu người dùng theo kịp; nếu consumer chậm
  đến mức bị ghi đè, response `truncated=true` và trả về tail mới,
  không trả lại dữ liệu cũ hoặc mất cursor E2EE.
- Sanitize control/format characters ngay khi nhận từ PTY; giữ tối đa
  3 byte UTF-8 còn dở giữa các `io.Copy.Write`. Xử lý theo từng khối
  4096 byte để giới hạn cấp phát tức thời. Sau khi reader kết thúc
  hoặc hết drain deadline, đóng băng capture; output đến muộn không
  thay đổi transcript đã đánh dấu `exited`.
- Không thay đổi cơ chế `newRemoteLongTaskCapture` của Task, không
  đổi kích thước E2EE response 128 byte, quota 16 action, lifetime
  90 giây, grant hoặc giới hạn bộ nhớ 16 KiB. PHP/Relay không lưu
  plaintext. Native GUI hiển thị đúng ý nghĩa `truncated`: dữ liệu
  quá cũ đã bị ghi đè **hoặc** không thể thu đủ sau khi đóng PTY.
- Kiểm thử `-race`: nhiều cửa sổ đã Poll vượt 16 KiB không mất dữ
  liệu, client chậm nhận tail cùng dấu truncated, UTF-8 bị chia giữa
  Write, freeze sau finalize và deferred E2EE cursor vẫn đúng khi
  rolling window đã bị ghi đè.


## 2026-11-11 — Native Terminal GUI: giữ UTF-8 đúng khi transcript vượt 8 KiB

- Host Terminal đã có rolling capture 16 KiB nhưng GUI native vẫn
  giữ transcript bằng JavaScript `slice(-8192)`: sai đơn vị (UTF-16
  code units, không phải UTF-8 bytes) và có thể chặt đôi một
  surrogate pair khi log chứa emoji/Unicode.
- Chuyển GUI sang helper byte-bounded `boundedUTF8HistoryTail` hiện
  dùng cho Task; giữ tối đa 8192 **UTF-8 bytes**, bỏ phần cũ chỉ ở
  ranh giới code point. Ghi nhận trạng thái `terminalLocalClipped`
  riêng với host `data.truncated`; thông báo cho người dùng khi
  màn hình chỉ hiển thị tail 8 KiB. Không ảnh hưởng dữ liệu ở host.
- Mở PTY mới và logout xóa cả transcript, DOM và cờ clipping.
  Không lưu file, không đổi giao thức/giới hạn lượt gọi; chỉ hiển
  thị text đã xác thực bằng E2EE.
- Mở rộng Node regression vốn có trong CI để bao phủ chuỗi emoji
  vừa đúng 8 KiB, tràn ranh giới surrogate, tiếng Việt đa byte và
  việc xóa dữ liệu/cờ clipping khi logout. Bài test Task UTF-8 lịch
  sử vẫn giữ nguyên. Chưa mở rộng remote session reconnect.


## 2026-11-11 — Native Terminal Auto Read: một lượt đọc trễ có giới hạn

- Khi bật `Tự nhận output`, GUI trước đây chỉ Poll tiếp khi
  `more=true`; nếu shell đang thực hiện `sleep 1; echo DONE` và
  lần Poll đầu trả `more=false`, GUI ngừng đọc dù output xuất hiện
  ngay sau đó. Cơ chế đã có quota giới hạn và chỉ cho phép host
  PTY mở tối đa 90 giây, nên không được thêm Poll vô hạn.
- Sau Open/Write/Resize, **chỉ khi người dùng bật Auto Read**, cho
  phép đúng *một* Poll trễ bổ sung (~1,3 giây) nếu lần Poll tự động
  trước đó đang chạy nhưng `more=false`. Nếu quá lượt đó vẫn
  không có `more`, dừng, chờ thao tác thực sự tiếp theo; nếu `more`
  vẫn còn, tiếp tục drain đã có nhưng không tạo các vòng idle mới.
- Manual Poll có ưu tiên và invalidates callback cũ qua epoch,
  không tự sinh quiet check. Scheduler kiểm tra lại trạng thái phiên,
  quyền feature, độ bận và `budget>1` ngay lúc timer thực thi:
  luôn giữ lượt E2EE cuối cho Close; không tự mở lại Terminal.
- Node mock regression bổ sung lệnh output trễ sau Poll rỗng,
  xác nhận thời gian retry, không lặp vô hạn, giữ cơ chế pause/manual,
  không tiêu lượt Close. Không thay đổi Wire codec, relay, agent hay
  giới hạn 16 lượt. Hành vi auto vẫn chỉ là best-effort trong quota,
  không phải streaming push liên tục.


## 2026-11-11 — Git Pull / Commit / Push: revoke GitWrite during execution

- Previously all three actions checked live TaskDeck GitWrite immediately
  *after* their attempted audit, but a locally revoked role/host opt-in
  while `git pull`, `git commit` or `git push` was ALREADY running
  would not cancel the subprocess until caller cancellation/9-second timeout.
- New host-owned `runRemoteGitMutationWithLiveRBAC` wraps ONLY the
  irreversible typed Git command, retains the action context, and checks
  the uncached local TaskDeck principal, Portal role ceiling, channel/grant
  and exact per-command opt-in every `remoteLocalRevokePoll` interval.
  Revocation immediately cancels the command context; existing
  `exec.CommandContext` plus process-group Cancel kills subprocesses.
  Watcher is joined before returning. Fast successful operations are
  reauthorized after process completion; no credentials/argv/URL are
  derived from browser data. No extra global lock or PHP round trip.
- Test cases exercise opt-out during an outstanding command for each
  of Pull, Commit and Push; a genuine SQLite role downgrade during
  active Push; normal fast command completion; and fail-closed denial
  before an unauthorised command starts. Added targeted `-race`
  preflight CI to validate watcher lifecycle and cancellation.
- Caveat: cancellation is best-effort; an operation already finalized
  by the filesystem or remote server cannot be rolled back. E2EE,
  host opt-ins and LIVE NO-GO remain unchanged. No Windows-specific code.


## 2026-11-11 — Remote GUI Task: host-owned 5-step sequential DAG

- Expanded the *explicitly sequential* workflow execution boundary from
  4 to **5 host-defined, non-interactive process tasks**. The selected
  client still supplies ONLY an E2EE-issued root Task.ID, not a process,
  dependency graph, argument, environment variable or secret.
- A fifth step is usable in linear chains or in a multi-parent join
  whose author explicitly writes `dependsOrder: "sequence"`.
  Rooted reachability, stable source hash, local TasksRun RBAC, live
  Portal grant, per-step opt-ins, attempted/finished audits, strict cwd
  and native executable checks, process group cancellation and the
  existing 90-second overall deadline remain mandatory.
- Fork/join execution continues to support **only 2–3 independent
  leaves plus their final root**, never a 4-leaf/5-step parallel DAG.
  Host `TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL` is still
  independently required. Implicit parallel diamonds remain rejected.
- E2EE codec, native GUI, host execution and graph codec all accept
  a 5-step **sequential** status. The extra signed step history/outcome
  consumes more of the fixed plaintext envelope, so 5-step Poll output
  is reduced to **32 UTF-8 bytes** per response. The previous
  4-step parallel limit remains 48 bytes, and 2–4-step sequential
  limit remains 64 bytes. Both normal and timeout Poll branches use
  one shared limit helper (the timeout branch previously overlooked
  the 4-step parallel reduction).
- Regression: E2EE-initiated native 5-step process DAG with explicit
  sequence and multi-parent final step, checks exact order and
  authenticated five-element history; codec tests for a runnable
  five-node graph, six-node denial, escaped 5-step E2EE output bounds,
  malformed parallel five-node progress; Node native GUI tests for
  five-step DAG and progress acceptance plus >32-byte denial.
- This is one more supported *bounded sequential shape*, not a general
  workflow engine, shell-script executor, secret entry mechanism,
  background session reconnect or authorization to deploy LIVE.


## 2026-11-11 — Short Task: thu hồi quyền sau attempted audit và khi đang chạy

- Trước đây Task chạy ngắn đã kiểm tra quyền ở E2EE dispatcher và
  ghi audit trước khi thực thi, nhưng thao tác SQLite `attempted`
  có thể mất thời gian. Nếu host tắt `TASKDECK_REMOTE_TASKS_RUN` hoặc
  thu hồi quyền local `TasksRun` trong khoảng thời gian đó, thao tác
  `cmd.Run` vẫn có thể chạy với quyền cũ.
- Thêm `remoteShortTaskHostConsent` tái kiểm tra opt-in
  `TASKDECK_REMOTE_TASKS_PREVIEW/RUN`, TaskDeck-local RBAC, grant và
  channel; với `remoteShortTaskRunWithLiveRBAC` bao quanh riêng tiến
  trình thực thi (tên hàm thực tế: `runRemoteShortTaskWithLiveRBAC`).
  Callback bắt buộc dùng `exec.CommandContext` của child context và
  lệnh hủy process-group hiện hành.
- Khi lệnh đang chạy, watchdog kiểm tra theo
  `remoteLocalRevokePoll`, fail-closed khi mất opt-in, role hoặc
  session. Sau khi tiến trình kết thúc, kiểm tra lại để ngăn gửi false
  success khi quyền bị thu hồi giữa hai tick. Watchdog được join để
  tránh giữ goroutine sau khi Task kết thúc.
- Giữ phân biệt giữa exit code không bằng 0 (Task hợp lệ đã thất bại)
  và mất quyền thực thi (không được trả stdout/exit như một lệnh được
  phép). Audit chỉ chứa ID/status/duration/exit code; không chứa command,
  args, stdout hoặc secret. Không đổi 6s process timeout, E2EE quota,
  phần mềm native, hoặc giới hạn output.
- Regression `-race`: tắt preview/run hay hạ vai trò trong lúc
  attempted-audit để xác nhận chưa start process; tắt run khi
  shell thực sự đang chạy để dừng toàn bộ process-group; hạ vai trò
  trong lúc callback đang chạy; bảo toàn exit code khác 0 của task
  được phép. Cấu hình CI preflight mới.
- Phạm vi chỉ là Task chạy ngắn; workflow dài đã có cơ chế giám sát
  quyền riêng. Production tiếp tục LIVE NO-GO.


## 2026-11-11 — Short Task: kiểm tra lại source/cwd sau audit

- Bổ sung một ranh giới nữa trước lệnh side-effecting: sau khi
  `remote.action.tasks_run:attempted` được ghi bền vững, host
  tái chạy toàn bộ `prepareRemoteTaskExecutionInput` với cùng
  Task.ID, choice/text, chính xác snapshot hash đã phát cho client.
- So sánh lại command, argv, env, cwd và Task ID với bản pre-audit
  bằng helper kiểm tra tương đương execution hiện dùng cho workflow
  song song. Đồng thời resolve lại workspace và đường dẫn cwd để
  phát hiện symlink thay đổi trong lúc audit I/O.
- Nếu source `.vscode/tasks.json` bị thay đổi dù stable ID giữ
  nguyên, hay cwd symlink bị đổi sang thư mục khác giữa preflight
  và exec, từ chối thực thi; không có bất kỳ process side effect
  nào từ spec cũ. Các thay đổi sau ranh giới cuối vẫn chịu kiểm
  soát bởi hệ điều hành/workspace lease; không tuyên bố đây là
  transaction filesystem nguyên tử.
- Regression `-race`: identity audit store giả lập cập nhật một byte
  của tasks.json hoặc thay mục tiêu cwd symlink sau khi ghi attempted
  audit; xác minh Task chạy ngắn không được phép tạo file. Không
  thêm round E2EE, không cho client chọn command hoặc đường dẫn.


## 2026-11-12 — Task Remote GUI: 4 nhánh song song + 1 bước tổng hợp

- Mở rộng duy nhất **fork/join nông do host định nghĩa** từ tối đa ba
  lên **bốn Task process độc lập chạy song song**, sau đó đúng một
  Task process tổng hợp. Các leaf không được có phụ thuộc, retry,
  timeout hoặc condition; các shape DAG nhiều tầng, shell/script,
  secret và client-supplied graph vẫn bị từ chối.
- Cùng Task.ID đã được host phát qua E2EE, không chuyển danh sách
  subprocess, cwd, command, argv hay env qua PHP/Relay. Giữ nguyên
  chính sách host opt-in `TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL=1`,
  kiểm tra quyền TasksRun, graph và snapshot ở host, durable audit
  barrier cho **toàn bộ bốn worker**, xác minh lại trước khi exec,
  watcher thu hồi quyền, hủy process group, fail-fast và workspace
  execution lease; root chỉ chạy sau khi cả bốn leaf thành công.
- Mở rộng validation trạng thái workflow song song 5 bước trong E2EE
  codec và native GUI; tiến độ song song bốn leaf có thể ở trạng thái
  `RRRR-`, root kết thúc `CCCCC`. Tất cả bước vẫn có signed outcome.
  Để bảo toàn phong bì E2EE 512 byte, mỗi Poll năm bước song song
  chỉ tối đa **16 byte UTF-8**; năm bước tuần tự giữ 32 byte, song
  song bốn bước giữ 48 byte.
- Native GUI xác thực số bước, mode, kết quả, byte cap và thông báo
  đúng hỗ trợ 2–4 leaf. Không tự mở quyền `LIVE NO-GO`, không đổi
  các giới hạn session hay bổ sung tự động nối lại.
- Regression `-race`: ngăn start khi host chưa bật opt-in;
  chạy thực tế bốn `/bin/sleep` đồng thời và kiểm tra root chỉ
  chạy sau bốn leaf; E2EE kiểm tra output escaped tối đa 16 byte,
  trạng thái đang chạy và giả mạo root/retry/mode; native GUI
  từ chối output 17 byte và trạng thái giả mạo.


## 2026-11-12 — Native GUI: khoá mode/tổng bước của workflow giữa các Poll

- CI Go cho commit `01eb11e8` đã PASS ở các test thực thi 4 worker
  và E2EE nhưng FAIL tại `native_gui_long_task_lifecycle.test.cjs`:
  test cũ vẫn coi trạng thái `RRR--` của 5 bước `parallel=true`
  là giả mạo. Sau mở rộng 4 leaf, đây là một trạng thái trung gian
  hợp lệ (ba worker hoạt động trước worker thứ tư). Sửa test để
  chấp nhận trạng thái này và dùng `RRRRR` (root chạy khi worker
  chưa xong) làm phản hồi sai.
- Gia cố **native GUI**: sau Poll đầu tiên có metadata workflow đã
  xác thực, ghi nhận cố định `step_total` và `parallel` theo
  từng active long job. Poll sau dù riêng lẻ trông hợp lệ vẫn phải
  khớp hai trường đó và vẫn chứa workflow metadata. Trạng thái
  bị đổi mode, đổi tổng bước hoặc mất metadata phải fail-closed
  và xoá transcript nhạy cảm khỏi giao diện.
- Không chặn ACK `tasks.cancel` / `tasks.clear` vì các ACK đó
  không mang tiến độ từng bước; trạng thái được xóa khi clear,
  logout hoặc bắt đầu long job mới. Không buộc workflow step index
  tăng đơn điệu: trong fork, tiến độ callback của worker có thể
  đến khác thứ tự. Không thay đổi Go E2EE codec hoặc quyền host.
- Bổ sung regression liên tiếp 2 Poll: parallel 5 bước ->
  sequential 5 bước hợp lệ riêng lẻ; parallel 5 -> parallel 4;
  parallel 5 -> mất metadata; và kiểm tra tất cả trường hợp sai
  đều xoá dữ liệu giao diện. Đưa Node test vào preflight CI sớm,
  không chỉ đợi nhóm GUI phía sau.


## 2026-11-12 — Native Remote GUI: không cho signed Task progress quay ngược

- Các trường `step_total` / `parallel` đã được cố định theo
  từng active long job nhưng các lần `tasks.poll` vẫn có thể có
  `step_history` hợp lệ riêng lẻ mà trái với lần Poll đã chấp nhận
  trước đó: ví dụ `CCRR-` → `RCRR-` sẽ biến dependency hoàn tất
  thành đang chạy. Không thể chỉ kiểm tra mã hóa AEAD từng frame;
  UI phải kiểm tra tiến độ theo thời gian của cùng một job.
- Native GUI tiếp tục đóng băng total/mode, và ghi nhớ `history`
  cùng `step_details` đã xác thực của Poll gần nhất. Từ chối
  việc bước đang chạy biến thành chưa bắt đầu, kết quả terminal
  `C/S/X/T` bị đổi, `F` bị đổi trái với retry tuần tự, hoặc exit
  code/thời gian của bước terminal đã xuất hiện bị sửa âm thầm.
  Khi bị từ chối, giao diện xóa output riêng tư và kết thúc local
  E2EE session như các lỗi trạng thái trước đó.
- Ngoại lệ có chủ đích: workflow tuần tự có `retry:1` được phép
  thay `F` thành `R/C` trong cùng bước. Với fork song song,
  **không** yêu cầu số thứ tự callback tăng đơn điệu: bốn worker
  có thể hoàn tất khác thứ tự, nhưng trạng thái terminal của mỗi
  worker không được quay ngược. Không tự thay đổi CLI process,
  quota E2EE, host authority, hay quy trình restart job.
- Test native JS dùng hai Poll liên tiếp, gồm rewind
  `CCRR-` → `RCRR-`, sửa duration đã chốt, bỏ metadata,
  tiến độ fork hợp lệ `RRR--` → `RRRR-`, và retry
  `F-` → `R-` của sequential Task. Preflight lifecycle
  JS đã nằm đầu workflow CI.
- Hạng mục kế tiếp: DAG nhiều tầng kết hợp parallel/sequential;
  nhập secret an toàn; những lệnh Git và GUI remote khác.


## 2026-11-12 — Long Task: post-audit snapshot, live TasksRun & host opt-out

- Tương tự Task chạy ngắn, quá trình `tasks.start` (long Task)
  trước đây chỉ kiểm tra thông số và RBAC trước khi ghi
  `remote.action.tasks_run:attempted`. Trong thời gian audit I/O
  diễn ra, TaskDeck local role, host opt-in, task source hoặc cwd
  symlink có thể bị thay đổi, và `cmd.Start` vẫn sử dụng thông số
  đã cache. Ngoài ra một long Task đang chạy vẫn tiếp tục nếu chỉ
  bị hạ quyền `TasksRun` (không logout hay tắt Portal grant).
- `remoteLongTaskHostConsent`: cùng kiểm tra role/key/grant
  uncached với Task ngắn, yêu cầu thêm host flag
  `TASKDECK_REMOTE_TASKS_LONG=1`. Sau durable attempted audit,
  host reload exact source hash/ID, command/args/env/cwd và
  resolve lại workspace/cwd symlink; nếu cấu hình thay đổi thì
  từ chối **trước** khi gọi `command.Start`. Quyền được kiểm
  tra trước và ngay tại điểm start; audit terminal `cancelled`
  hoặc `failed` được ghi khi attempted đã thành công nhưng
  không start được.
- Watchdog của long Task (không phụ thuộc vào việc client Poll)
  theo dõi host opt-in, live local RBAC, session software key,
  E2EE channel và Portal grant bằng
  `remoteLocalRevokePoll`. Khi không còn quyền, hủy context
  để `CommandContext.Cancel` diệt process group; watcher
  được join sau `Wait`, trước khi công bố kết quả.
  Khi nhận kết quả, host reauthorize lần cuối để không xác
  nhận thành công khi mất quyền ngay sau tick cuối.
- Ranh giới: không thể rollback tác dụng phụ đã hoàn tất trước
  khi hủy. Giữ nguyên quota 16 hành động E2EE, capture 16 KiB,
  timeout Task tối đa 90 giây, dọn workspace lease sau reaping,
  audit không ghi password, stdout, command hoặc arguments.
- Regression Go `-race`: thu hồi từng host opt-in và hạ role
  sau attempted audit; sửa byte của tasks.json và đổi cwd symlink
  trong lúc audit; thực thi shell thật, rồi hủy giữa chừng bằng
  opt-out hoặc role downgrade và xác nhận không tạo side effect
  cuối; phân biệt lỗi thực thi exit 7 với lỗi mất quyền.
  Test được đưa lên preflight Agent CI.
- Chức năng Long Task vì thế hoàn tất về code trong trường hợp
  host thu hồi quyền giữa chừng. DAG nhiều tầng/song song kết hợp
  và secret input vẫn chưa hoàn tất, LIVE production vẫn NO-GO.


## 2026-11-12 — Task Remote GUI: một bước tuần tự rồi fork/join (4–5 bước)

- Mở rộng **một shape DAG được giới hạn chặt**: một Task process
  chuẩn bị trước (`Prepare`), 2 hoặc 3 Task process con cùng phụ thuộc
  `Prepare`, và đúng một Task process tổng hợp có
  `dependsOrder: "parallel"` và phụ thuộc vào tất cả con.
  Thứ tự được thực thi: chuẩn bị **hoàn tất thành công** → 2–3
  con chạy đồng thời → toàn bộ con thành công → tổng hợp.
  Không có DAG nhiều tầng khác, nested fanout, child có retry,
  timeout, condition, secret hoặc shell interpreter.
- Bổ sung host flag **`TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX=1`**
  riêng biệt, mặc định tắt. Ngoài cờ này, host vẫn phải bật đầy đủ
  opt-in của workflow và `TASKDECK_REMOTE_TASKS_WORKFLOW_PARALLEL=1`.
  Các repo/host chỉ bật cờ song song cũ không tự nhận quyền chạy
  thêm shape mới.
- Xác minh đồ thị ở host bằng nhãn `dependsOn`, `dependsOrder`,
  toàn bộ Task.ID và exact edges đã phát cho phiên E2EE. Client
  tiếp tục chỉ gửi ID của root; không có command/argv/cwd/env/
  graph do trình duyệt chọn. Giữ tổng số bước 4–5, tối đa 3 worker
  đồng thời ở shape này, quota và 90-second job deadline.
- Prelude được preflight trước khi bắt đầu, có durable attempted audit
  và xác minh lại source/exec/env/cwd sau audit; watchdog giám sát
  TasksRun và cả hai cờ host khi đang chạy, hủy process-group nếu
  thu hồi quyền. Prelude failed/cancelled thì không ghi attempted
  cho child/root và không chạy những task đó.
- Các worker vẫn sử dụng audit barrier và prelaunch revalidation
  nguyên gốc, cùng một watcher quyền và fail-fast; root chỉ chạy
  sau khi tất cả child kết thúc thành công. Bản ghi E2EE tiến độ
  giữ nguyên `parallel=true`, lịch sử `CRR-` hoặc `CRRR-`,
  với 48-byte cap cho tổng 4 bước và 16-byte cap cho 5 bước.
  Native GUI lấy đúng **số nhánh song song** từ indegree root
  của graph đã được xác thực thay vì mặc định `stepTotal-1`.
- Regression `-race`: cả hai shape 2/3 worker chạy thực tế,
  E2EE final progress, thiếu opt-in prefix, bước chuẩn bị thất
  bại, thu hồi opt-in trong lúc bước chuẩn bị chạy, đổi tasks.json
  sau attempted audit; Node browser test graph 5-node 6-edge;
  workflow/Go CI vẫn tiếp tục kiểm tra flat fork.
- Chỉ tính là hoàn tất code sau khi CI pass. LIVE production vẫn
  `remote_control_available=false`, không tự kích hoạt.


### CI follow-up (2026-11-12)

- First CI Go run for the prefix commit stopped at the existing native GUI
  dependency confirmation assertion: the newly reworded consent prompt no
  longer contained the exact earlier bounded-parallel explanation, although
  the host and E2EE checks preceding that step succeeded. Restore the
  2–4 independent Task wording, preserve the final join success guarantee,
  and add an explicit positive assertion for the new prefix opt-in.
- This is a GUI confirmation/test compatibility correction, not a
  privilege relaxation. The targeted Go runtime tests will execute on
  the next CI run; do not mark the prefix feature fully validated before
  those results arrive.


### CI follow-up: TaskDeck default dependsOrder on one-parent leaves

- The first Go `-race` preflight exposed a host graph-validation mismatch:
  TaskDeck normalizes an omitted `dependsOrder` to `"parallel"`
  even for a Task with exactly one `dependsOn`. The new prefix shape
  mistakenly demanded `"sequence"` (by rejecting normalized
  `"parallel"`), so a correct `Prepare → Worker` edge was denied.
- Keep the strict requirement that each worker depends on **exactly**
  the single prelude Task label, but allow either normalized ordering:
  with one dependency there is no scheduling ambiguity. The join/root
  still must explicitly use `dependsOrder: "parallel"` and depend on
  all leaves. Extra dependencies, nested forks and implicit DAGs remain
  denied. The existing preflight Go tests intentionally omit
  `dependsOrder` from the workers to guard this compatibility.


## 2026-11-12 — Task Remote GUI: hai bước chuẩn bị tuần tự trước fork/join

- Giữ giới hạn tổng cộng **5 Task** nhưng bổ sung thêm đúng một
  shape DAG do **host định nghĩa**: `Prepare One → Prepare Two →
  (Leaf A || Leaf B) → Join`. Không hỗ trợ nested fork, ba bước
  chuẩn bị, retry/condition/timeout trên leaf hoặc do browser
  cấp command, argv, env, cwd hay graph.
- Yêu cầu tất cả opt-in hiện có, và **cả hai**
  `TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX=1` và
  `TASKDECK_REMOTE_TASKS_WORKFLOW_PREFIX2=1`. Cờ thứ hai
  mặc định không bật, không nâng quyền cho host chỉ chấp thuận
  một bước chuẩn bị.
- Xác minh host DAG: nút đầu không có dependency; nút thứ hai
  chỉ phụ thuộc nút đầu; hai leaf chỉ phụ thuộc nút thứ hai;
  root phụ thuộc đúng cả hai leaf và có `dependsOrder: parallel`.
  Graph bao gồm chính xác **5 nút, 5 cạnh**; mọi cạnh bất thường
  hoặc tên trùng không được phép.
- Thực thi hai bước chuẩn bị theo thứ tự: từng step có preflight,
  `attempted` audit bền vững, kiểm tra source/executable/cwd/env
  lại sau audit; watcher host kiểm tra cả hai cờ opt-in,
  `TasksRun`, Portal grant và session. Mất quyền hoặc failed
  ở bất kỳ bước nào dừng trước các leaf/root, hủy process group.
- Giữ nguyên barrier audit cho hai leaf, re-preflight trước khi
  khởi động song song, watcher, join và post-exit authorization
  trước khi chạy root. Kết quả tiến độ E2EE năm bước giữ
  `parallel=true` và giới hạn **16 bytes** output mỗi Poll,
  như dạng fork/join 5 bước trước đây; native GUI tiếp tục
  lấy số nhánh song song từ indegree của root được xác thực.
- Regression Go `-race`: thực thi đủ năm bước và kiểm tra
  `CCRR-` khi hai leaf đang hoạt động, `CCCCC` khi xong,
  transcript và E2EE final status, từ chối nếu không bật
  `PREFIX2`, ngăn chạy con/root khi bước thứ hai thất bại,
  hủy giữa chừng khi host tắt `PREFIX2`. Native GUI xác nhận
  hiển thị điều kiện opt-in thứ hai và giữ shape chỉ đọc.
- Chỉ tính hoàn tất sau CI; không bật Remote Control ngoài
  deployment được phê duyệt, production vẫn `LIVE NO-GO`.


### CI follow-up: Go test import cleanup

- CI của commit `cde94fb0` đã dừng ngay ở bước biên dịch
  `internal/server` vì file regression mới còn import `os`
  nhưng không sử dụng. Loại bỏ import thừa; test workflow thực
  thi thực tế **chưa chạy trong lần CI lỗi này**. Không coi đây
  là một lỗi runtime đã được chứng minh hoặc tính năng đã pass.
- Kiểm tra tiếp bằng CI mới gồm preflight `-race` cho
  `TestRemoteWorkflowDoublePrefix*`, full workflow và native GUI.


## 2026-11-13 — Remote Task GUI: final per-process launch guard for fork/prefix/root

- The attempted-audit barrier and shared sibling preflight previously ran
  before launching goroutines. In the intervening window a host could
  change tasks.json, inherited environment or an approved executable
  symlink, despite the earlier audit row remaining valid. Strengthen
  `executeRemoteParallelPrepared` with a **per-process** re-preflight
  immediately before publishing `running` and calling `exec.Cmd.Run`.
  All three contexts use this check: serial prelude(s), parallel leaves
  and final root. It verifies host consent/RBAC, snapshot digest,
  executable resolution, command, argv, env and cwd.
- Record a canonical `resolvedCwd` in each host-owned prepared process
  and include it in `remoteParallelSameExecution` comparisons. Comparing
  only the lexical cwd would not detect a symlink whose target changed
  between two valid directories under the same workspace. The short
  Task caller's separate post-audit cwd check remains unchanged.
- If the last-moment re-preflight fails, do not run the process; persist
  a terminal `failed` or `cancelled` step outcome, update the signed
  workflow history, and stop the DAG before starting a dependent root.
  The browser still provides only an E2EE-issued root Task ID, never a
  process spec. The limits (5 total steps, concurrency, 90s lease,
  16-byte output cap for parallel five-step Polls) stay unchanged.
- Regression Go `-race`: stable setup permits launch; a forged stale
  canonical cwd, retargeted native executable symlink outside workspace,
  changed inherited environment, changed tasks.json bytes, and revoked
  parallel host opt-in must all deny at the final launch boundary.
  CI runs these in an early preflight step.
- Filesystem mutations remain possible after any path-based check.
  This closes the *identified post-audit/pre-launch observation gap*;
  it is not equivalent to a kernel-atomic pinned-file descriptor exec,
  and must not be represented as such. Production remains LIVE NO-GO.


### Follow-up: execute from the canonical approved cwd

- The prelaunch guard already pins `resolvedCwd`. The process must
  actually use that approved **canonical** directory; passing the old
  lexical `execution.Cwd` to `exec.Cmd.Dir` leaves a final symlink
  retarget gap between validation and the kernel's chdir.
- Parallel prelude, workers and root now set `cmd.Dir` to the
  revalidated `prepared.resolvedCwd`. The path is guaranteed absolute,
  inside the approved workspace, and matched with the pre-audit
  resolved path. Tests assert the canonical directory is populated.
- This blocks re-following the original cwd symlink during launch;
  it does not pin filesystem directory inodes against arbitrary
  concurrent rename/replacement. A kernel-level dirfd/chdir design
  is out of scope. Production LIVE NO-GO remains unchanged.


## 2026-11-13 — Git Commit: pin repository, branch, HEAD and staged index after audit

- Git Commit previously checked branch, HEAD and whether the index
  contained staged differences before writing the durable
  `remote.action.git_commit:attempted` audit. An unrelated host
  process could change any of those values while the audit row was
  being persisted. The GitWrite grant alone cannot authorize
  committing a **different** branch, HEAD or staged snapshot.
- Before the audit, record the exact branch ref, HEAD object ID and
  inode of the canonical repository Git directory; hash the
  `git ls-files --stage -z` stream incrementally with SHA-256, never
  retaining staged file paths/content in memory, E2EE, PHP or audit.
  A 3-second child context caps the index-fingerprint command.
- **After the audit and again at the actual commit launch**, re-check
  exact top-level workspace, resolved Git directory identity, branch
  ref, HEAD and staged-index fingerprint. Also re-scan executable
  filter drivers after audit so hostile local config cannot introduce
  a command-bearing clean/smudge/process filter between initial
  authorization and `git commit`. Deny any mismatch; no Commit
  subprocess starts and GitWrite/host opt-in guards still apply.
- Tests use a host-local audit-store hook, not browser arguments,
  to mutate the index, switch HEAD to another local branch, move
  HEAD to its parent or add an executable Git filter after the
  mandatory attempted audit. Require a denied result, unchanged
  post-mutation HEAD, no unauthorized commit, and an audit row
  without commit text or filenames. Add the targeted suite to the
  early Go `-race` CI preflight.
- Git storage is not a kernel-atomic transaction against arbitrary
  unrelated local writers after the final check; this hardening
  addresses identified pre/post-audit races without claiming to
  eliminate every external filesystem race. Do not change
  `remote_control_available=false` (LIVE NO-GO).


## 2026-11-13 — Git Pull: snapshot HEAD/upstream/URL/worktree after attempted audit

- Pull previously selected a clean, single Git repository/upstream and
  SSH/HTTPS URL before its mandatory `remote.action.git_pull:attempted`
  audit. A local writer could modify HEAD, branch tracking config,
  effective remote URL, Git filters or untracked files during that audit,
  then cause the fixed `pull --ff-only` to act on a **different**
  upstream or dirty worktree despite retaining a valid GitWrite grant.
- Before audit, bind the canonical Git directory path and directory
  inode in addition to the existing canonical workspace, current branch,
  HEAD commit, branch remote + merge ref, symbolic upstream and unique
  effective fetch URL. Continue rejecting dirty/untracked files,
  modified submodules, executable filters and repository HTTP overrides.
- **After audit and again immediately before the network subprocess**,
  verify the identical repository top-level/Git directory identity,
  branch, HEAD, remote/merge/upstream config, URL and clean worktree.
  Re-scan dangerous filter and HTTP overrides so a config mutation
  during the audit cannot trigger a credential helper, insecure TLS or
  filter process. Do not accept alternate remote/refspec arguments
  from the E2EE client; keep only the already-fixed fast-forward pull
  with no tags, local/file/ext protocols, submodules or redirects.
- Regression `-race` mutates each element via a **host-local**
  attempted-audit hook: URL, tracking merge ref, HEAD, untracked
  worktree contents, executable filter and TLS override. Require the
  Git operation to be denied with no additional HEAD movement and
  with a content-free mandatory audit row. No real network server is
  contacted during the denial tests.
- This recheck guards the identifiable audit gap, not out-of-process
  mutation in the last microseconds before a kernel/Git ref update.
  Production Remote Control remains LIVE NO-GO.


### Pull post-audit regression fixture correction

- The first `-race` run rejected each synthetic scenario **before the
  attempted audit** because the test only staged `tracked.txt` in its
  baseline while the admin workspace may also contain host-owned
  untracked files. Pull correctly requires a fully clean worktree.
  This did not exercise the new post-audit checks (no audit record
  and no intended hook mutation), so it was an invalid regression
  fixture rather than a demonstrated Pull security failure.
- Stage the complete original fixture with `git add -A` for its first
  baseline commit, matching the existing working Pull test. Then
  inject one mutation only **after** a durable attempted audit and
  assert that Git Pull is denied with no additional HEAD movement.
  Keep the hard security preflight intact; do NOT weaken the rule
  denying untracked files in production.


## 2026-11-13 — Git Push: revalidate exact host-owned destination and refs

- The original E2EE Git Push already pinned a fixed, non-force
  `HEAD-SHA:refs/heads/current` refspec and required an approved
  single SSH/HTTPS upstream before durable
  `remote.action.git_push:attempted` audit. It did **not** revalidate
  the effective push URL, local branch or tracking configuration if a
  separate local Git writer changed them **during** the audit.
  In particular, pinning the commit SHA does not pin its destination.
- Before audit, capture canonical Git directory path and its directory
  inode as well as the already checked canonical workspace, current
  symbolic branch, branch remote/merge, symbolic upstream, exact HEAD
  commit, upstream object ID and effective `remote get-url --push --all`
  result. Continue rejecting multiple URLs, unsupported protocols,
  URL credentials, HTTP config overrides and non-FF local history.
- After the audit **and again within the live GitWrite-guarded
  execution callback immediately before launching the subprocess**,
  verify repository top-level and Git directory identity, symbolic
  branch, HEAD SHA, branch remote/merge, upstream mapping + object ID,
  effective push URL and safe HTTP configuration. Deny any drift
  without opening an SSH connection. Remote refspec stays fixed and
  explicitly non-force; there are no browser-provided Git args.
- Unix Go `-race` regression tests inject host-local changes after
  the attempted audit: push URL retarget, symbolic branch switch,
  HEAD reset, branch merge or remote reconfiguration, tracking-ref
  movement, and unsafe `http.sslVerify` change. A fake local
  `ssh` executable leaves a marker if any prohibited push is
  attempted; assert marker absent, HEAD unchanged after the
  intentional mutation, and content-free audit record.
- These repeated host-side checks reduce the identifiable audit
  time-of-check/time-of-use gap. A Git command is not a kernel-atomic
  transaction with unrelated local writers or a remote server.
  Keep production Remote Control LIVE NO-GO.

## 2026-11-13 — Remote GUI short task: last-moment process launch guard

- Added a final source, executable symlink, canonical workspace/cwd and
  inherited environment comparison inside the live-RBAC callback, not only
  following the mandatory attempted audit. A late host-side mutation now
  denies process spawn, without changing any E2EE wire operation.
- Execution uses the pinned canonical executable with the originally
  approved argv[0] to preserve native shell behavior. Includes Unix
  regression checks for changed task source, executable symlink, inherited
  environment and host opt-out. The same short-task limits and
  process-group cancellation still apply.
- **Code completed; physical LIVE verification is separate.** Remote
  production go-live remains NO-GO.

## 2026-11-13 — Remote GUI long task: pin executable and final launch validation

- The 90-second host-managed long task checked unchanged task source,
  inherited env and canonical cwd after the durable attempted audit,
  but it had not pinned the resolved executable symlink target. An
  executable retarget after audit could therefore launch a different
  host-local program even though the stable Task.ID did not change.
- Capture canonical executable, workspace and cwd BEFORE attempted audit;
  revalidate task source, argv, environment and paths AFTER audit and
  once more immediately before native `cmd.Start()`, after the
  potentially blocking local RBAC and host opt-in checks. Spawn only
  the pinned resolved executable while retaining the host-authored
  `argv[0]`. A failed check denies the launch without occupying the
  long-job slot or leaking the workspace lease.
- Added Unix regression with an audit-time executable symlink retarget
  to a marker-writing program; asserting no job/marker is created.
  The 90-second bound, process group cancellation, host-side audit,
  E2EE quota and session-owned status lifecycle are unchanged.
- Code is implemented on `main`; live production remains NO-GO.

## 2026-11-13 — Sequential Remote Task workflow: pin process before step audit

- Fixed a sequential workflow admission gap: the resolved executable
  target was first recorded **after** the mandatory per-step attempted
  audit. An unrelated local writer could retarget the host-authored
  executable symlink during audit; both later checks would see the new
  native binary and consider the changed program approved.
- Resolve and pin the allowed native executable and canonical cwd
  **before** recording the step attempt. After audit, independently
  re-read the immutable E2EE task source, process argv, inherited env,
  executable target, canonical cwd and live TaskDeck authorization.
  Revalidate the same approval once more under the child cancellation
  context immediately before `cmd.Run()`, including retries and
  final cleanup tasks. Only the pinned executable/cwd are passed to Go.
- A denied launch logs a content-free terminal step outcome, does not
  retry it, and does not start later workflow steps. Unix regression
  retargets an executable from echo to touch during attempted audit
  and verifies no marker file or root process. Additional tests cover
  canonical cwd and inherited environment drift.
- No new remote command or E2EE parameter, no permission relaxation.
  Production LIVE remains NO-GO separately from code completion.
