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
