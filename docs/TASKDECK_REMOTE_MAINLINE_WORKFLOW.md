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
