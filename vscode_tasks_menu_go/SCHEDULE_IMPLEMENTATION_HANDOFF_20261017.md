# TaskDeck Schedule — Implementation & handoff — 2026-10-17

## Summary / Tóm tắt

TaskDeck now has a server-side Schedule engine, a left Activity Bar Schedules
icon, categorized sidebar, workspace tab for editing schedules, and a separate
running/history console view.

The engine **runs on the TaskDeck server independently of the browser tab**.
Jobs are configured under the user's OS configuration directory, in
`taskdeck/schedules/<SHA256 of canonical workspace path prefix>/` as
`state.json` and `logs/<run-id>.log`. These files are not written inside
the source workspace. Config writes are atomic and mode `0600`; folders
are created mode `0700`.

### Quick start / Cách sử dụng

1. Open **Schedules** on the left Activity Bar (auto-hide and narrow panel supported).
2. Click **+ New**; create a schedule. Schedules default to **disabled**.
3. Choose trigger:
   - **Cron**: five-field cron expression `minute hour day-of-month month weekday`,
     with optional IANA timezone (e.g. `Asia/Ho_Chi_Minh`) or `Local`.
   - **URL**: poll every 30–86400 seconds; response can match exact text,
     a keyword, a JSON dot path/value, or text under an XML path.
   - **Git**: choose a **specific repository root** using the editable
     repository input plus the discovered-repo dropdown (same discovery engine
     as the Git Quick Actions `git-repo-select`). The input remains fully
     editable; native text suggestions and a **↻ Rescan** button are available.
     For a new Git schedule, it automatically fills the Git bar's currently
     selected repository, otherwise the workspace's default or first discovered
     repository. Existing saved paths and user-typed paths are never overwritten
     by asynchronous discovery/rescan. Use `HEAD` / branch ref to trigger on
     changed local commit, dirty tracked worktree, or clean tracked worktree.
   - **Dependency**: choose parent schedule, status (success/failure/any),
     optional specific exit code, required substring and forbidden substring
     in its console output. Cycles and missing parents are rejected.
4. Choose **Custom shell command** or a task from `tasks.json`.
5. Choose **Allow overlapping runs** (default: disabled), execution timeout,
   minimum **Retain N days** and **Retain N runs**, and **Enabled**.
6. Save. **Run now** works manually even if a schedule is disabled; use **Stop**
   on a selected running instance to cancel it.
7. Select a run in the sidebar or run history to read its console log.

**Retention uses the later of the two constraints**. An entry is deleted only
after it is **both older than N days and outside the newest N runs**. Example:
2 days / 10 runs with hourly runs retains ~48–49 records (not just 10);
1 day / 24 runs with hourly runs retains ~24–25 depending on current time.
The most recent threshold is based on the run timestamp.

### Detailed behavior / Chi tiết

- URL polling uses a bounded 8-second GET, 256 KiB maximum body, no redirects,
  proxy disabled, DNS validation at dial time, and blocks local/private
  address ranges and credential-bearing URLs. It intentionally refuses internal
  development endpoints by default. Both http and https public endpoints are
  supported. Response triggers fire on **false → true**, not on every poll
  while a condition remains true. Transient network errors do not reset edges.
- Git monitors the chosen local checkout **without automatically fetching**
  remotes. A commit SHA change must be observed *after an initial baseline*.
  Dirty/clean examines tracked worktree changes, not untracked files. Polling
  is bounded and supports legacy Git by using process `Dir` rather than
  `git -C`.
- Dependency triggers fire once per completed parent run, subject to filters.
  Source console is bounded to 2 MiB per run. A canceled or timeout run is
  treated as a failed run with exit code `-1`.
- Cron uses five fields with `*`, `*/n`, lists and ranges.
  It checks at five-second intervals, once per matching local minute.
  It does not currently implement extended `@daily` or second-resolution cron,
  catch-up of schedules missed during server downtime, or a GUI cron builder.
- Concurrency caps: 12 total running jobs, 4 per individual schedule, and
  at most one job per schedule unless overlap is explicitly enabled.
  External URL/Git checks use a pool of six slots, independently of cron ticks.
  Default run timeout: 1800 seconds; maximum 86400 seconds.
- Logs remain readable while jobs are active, and are persisted per run.
  On startup any previously persisted `running` run becomes `interrupted`.
  On graceful server stop, active process contexts are canceled and command
  process groups are terminated (Unix process groups; Windows taskkill /T).
- Scheduled `tasks.json` execution uses TaskDeck's workflow resolver;
  task ID is resolved at configuration-save and execution time. Tasks that
  require interactive input, deployment approval, or secret grants are not
  permitted to run unattended.
- Shared-server schedule APIs, repository enumeration, job control, and logs
  are gated by **`project.admin`**. This avoids a restricted developer
  creating an unattended privileged command or reading another schedule's log.
- No multi-host distributed leader election; run exactly one TaskDeck server
  per active workspace. The scheduler configuration is not a substitute for
  a system supervisor or HA scheduler.

### API

- `GET /api/schedules` — schedules, run metadata, trigger errors.
- `POST /api/schedules` — `action=upsert|delete|run|stop`.
- `GET /api/schedules/log?run_id=<id>` — text console.
- `GET /api/schedules/repositories` — repositories discovered by the same backend as Git Quick Actions; excludes non-primary attached-root paths that the schedule runner cannot resolve. Add `?refresh=1` to rescan; response includes `default_repository`.

### Files

- `internal/server/schedule_model.go` — schema, validation, cron and persistence.
- `internal/server/schedule_runtime.go` — polling, execution, dependency
  chaining, logs, retention, concurrency and stop.
- `internal/server/schedule_api.go` — API for management, log and repo selection.
- `internal/server/schedule_process_unix.go`, `schedule_process_windows.go`
  — process-tree termination.
- `internal/server/server.go`, `shared_authorization.go` — server lifecycle,
  authorization and route registration.
- `web/featuremods/schedulespanel.js` — categorized sidebar, editor tab,
  running/history list and console.
- `web/featuremods/activitybar.js`, `next.js` — activity icon and module import.
- `internal/server/schedule_test.go`, `schedule_ui_test.go` — regression tests.

### Verification

From `vscode_tasks_menu_go`:

```bash
go test ./internal/server -run 'TestSchedule' -count=1
go test ./internal/server -count=1
go test ./... -count=1
```

Browser smoke checklist:

- Open new Git schedule after selecting a non-default repo in Git Quick Actions;
  verify auto-fill uses that repo. Change repo with the adjacent dropdown,
  then type an arbitrary directory and verify it is never overwritten on rescan.
  Verify native suggestions work while typing, and existing schedule path
  survives reopening even if no longer discovered.

- At minimum 220px sidebar width, six Activity Bar icons must not overflow;
  verify auto-hide and non-auto-hide and panel resizing.
- Create disabled schedules for all four trigger types. Confirm nothing
  auto-runs until enabled, but manual Run works. Reload the browser/server
  and verify persisted definitions.
- Test `* * * * *`, `*/5 * * * *`, and an IANA timezone.
- URL text/JSON/XML + transient 500/timeout → recovery; no duplicate while
  condition stays true.
- Two nested Git repos: set separate roots, verify only selected repo
  changes fire its schedule.
- Schedule A `exit 0` → B; A `exit 42` → C filtered by 42; use output
  contains/absent predicates; reject A→B→A dependency cycle.
- Overlap disabled/enabled, manual start while another run is active, Stop,
  timeout, console history, retention with a simulated clock.
- Shared-server: verify non-admin/ restricted developers receive 403 for
  management, run control, log and repo discovery.
- Run under Go 1.19 and on an available Windows runner.

**Verification honesty:** Backend and frontend have been committed. Go tests
have been added but have **not been executed against this repository in the
current connector-only environment**; a production build and Firefox browser
smoke remain required.


## Schedule navigation fixes (2026-10-17)

Two sidebar/tab integration regressions were corrected:

1. **Active tab:** Schedule now uses `syncScheduleActiveTab()` on every core
   `taskmenu:view-activated` event, applying `active` and `aria-selected`
   to its workspace tab and toggling its content pane in sync with the
   authoritative `app.active === 'external:schedules'` value. Switching back
   to a Terminal clears the Schedule tab's active styling.
2. **Persistent sidebar:** `openTab()` no longer calls `closePanel()`.
   Selecting a schedule or an entry under **Recent runs** keeps the left
   Schedule panel and its Activity Bar selection in place, including the
   auto-hide layout. Sidebar closing remains explicit (another Activity Bar
   view, close button, outside click in auto-hide, Escape).
3. **Close behavior:** Closing an active Schedule workspace tab attempts to
   restore the previously active real workspace tab, not an artificial
   `external:empty` view. A close when Schedule is inactive does not steal
   focus from the current tab.

Regression coverage:
- `tests/schedule_tab_navigation.test.mjs` (seven Node cases)
- `internal/server/schedule_ui_test.go` asserts active-class integration and
  that openTab never closes the Schedule sidebar.
- Direct JavaScript checks: 11/11 PASS, including active tab and sidebar
  persistence. Full Node/Go and Firefox UI suite still require a repository
  runtime to execute.

Run:
```bash
node --test tests/schedule_tab_navigation.test.mjs
go test ./internal/server -run 'TestScheduleFrontendUsesWorkspaceTabAndResponsiveActivityBar' -count=1
```


## Schedule management / multi-trigger extension (2026-10-17)

### Workspace screens

- Clicking the **Schedule** Activity Bar icon opens **All schedules** in the
  workspace and retains the left Schedule navigation panel. All schedules
  appear in one table, with name/ID, enabled state, trigger summary, execution
  target, number of active runs, latest run, and direct Edit / Run / Enable /
  Disable / Delete actions.
- **All runs** lists records across *every* schedule (running, successful,
  failed and interrupted), including start/end time, duration, cause, exit code,
  and controls to open that run's console or stop an active run.
- **Dependency graph** draws a directed graph with branching, including
  multiple parents feeding one dependent and one parent with many children.
  Click a node to open the schedule's detail editor.
- **Details / Edit** includes immediate links to upstream dependencies and
  downstream schedules that depend on the selected schedule.

### Condition expressions

A schedule can now have a recursive **AND / OR** condition expression rather
than only one trigger. It supports Cron, URL, Git and "after schedule" leaves,
nested groups and unlimited sibling conditions within the existing request and
persistent-state size limits.

Example: **(URL status READY AND Git commit changed) OR (Build succeeded
AND Test console contains PASS)**.

Technical behavior:

- Each condition has an independent persisted ID and an in-memory polling
  baseline, with separate URL/Git intervals and cron-minute observations.
- A leaf signals when an eligible trigger event occurs. **AND** latches each
  leaf signal until *all* operands (including nested groups) have fired;
  **OR** is satisfied when *any* operand fires. When a run starts successfully,
  the collected signals are cleared for that schedule.
- If overlap is forbidden or concurrency is full, signals stay latched until
  another condition event arrives; the runner does not start a job in excess
  of the established concurrency/overlap limits.
- Dependency leaves match the parent run's result status, exit code, required
  console substring and excluded substring; one parent completion may satisfy
  multiple downstream branches. Dependency cycles are rejected across the
  whole graph, including legacy single-dependency schedules.
- Old schedules without `condition_group` continue working as a synthetic
  single-leaf expression. Editing/saving one in multi-condition mode migrates
  it to the expression schema. Newly created schedules default to a disabled **single Cron condition**.
  Adding another condition or group automatically promotes the configuration
  to multi; removing back to one condition automatically stores it as single.
- Event latches are in-memory, not durable across a TaskDeck server restart,
  just like previous trigger edge states. The system does not execute missed
  events after downtime. Condition group nesting has a safety maximum of
  12 levels; API/request and state size limits still apply.
- Updating a schedule clears that schedule's previous trigger baselines and
  pending latches. In-flight HTTP/Git checks from an obsolete definition
  cannot activate an edited schedule.

### Validation / release

- The previously reported Go build error in `schedule_conditions.go`
  (`strings` imported but unused) was removed in commit `c53503e54038`.
  The temporary unused `os` placeholder was also removed.
- Static module JS syntax checks: PASS. Focused live V8 rendering checks:
  **14/14 PASS** (multi-parent graph, all-runs action, legacy conversion,
  overview row actions and nested condition normalization).
- Added Go tests in `internal/server/schedule_conditions_test.go` for
  AND/OR, legacy compatibility, cycle validation and duplicate IDs.
- Added Node tests in `tests/schedule_conditions_and_views.test.mjs`.
  Run the complete Go/Node suite and Firefox browser smoke on a real TaskDeck
  checkout before activating in production.

```bash
cd vscode_tasks_menu_go
go build ./...
go test ./internal/server -run 'TestSchedule' -count=1
node --test tests/schedule_conditions_and_views.test.mjs
node --test tests/schedule_tab_navigation.test.mjs
```

Manual validation: create A with URL and Git conditions using AND, B using
OR with a dependency on A, C depending on both A and B, check that the DAG
shows branching correctly and each leaf event is counted only as intended.
Verify action buttons, multi-schedule run history, log console access, narrow
window sizing, sidebar persistence and cross-user administrator permissions.


## UI correction — Dependencies, trigger labels, console copying and history scope (2026-10-18)

- **Dependency details typography** now shares the form's 11px size and
  line-height. Upstream/downstream relation links inherit this typography.
- **All schedules / Trigger conditions** shows each referenced schedule's
  readable **name (ID)** in compound expressions. Legacy "after schedule"
  expressions also render parent name together with its ID.
- **Copy console** appears next to each run in the detail **Run history**,
  beside the selected console header, and on the **All runs** page. Copy is
  sourced by the exact run ID (not from a potentially stale visible console),
  with Clipboard API and browser copy fallback.
- **Run history isolation**: the Details view filters runs by the *selected
  schedule ID*. Creating an unsaved schedule shows no prior run history;
  changing schedules resets the selected run, console, copy control and
  history scroll position. Periodic refresh validates run/schedule membership,
  and an incrementing console-request sequence prevents out-of-order fetches
  from overwriting a newly selected schedule's console.
- Added regression tests in `tests/schedule_history_isolation.test.mjs`
  (7 cases), updated `tests/schedule_conditions_and_views.test.mjs`
  (parent name plus ID, Copy console), and Go embedded UI checks.
  Focused JavaScript behavior/static checks: **13/13 PASS**. Complete
  `go build`, Node test suite and Firefox browser validation remain to run
  on a repository checkout.

```bash
cd vscode_tasks_menu_go
go build ./...
go test ./internal/server -run TestSchedule -count=1
node --test tests/schedule_history_isolation.test.mjs
node --test tests/schedule_conditions_and_views.test.mjs
```

## Single/multi condition editing parity (2026-10-18)

All schedules now use the *same* expression editor, including legacy Cron,
URL, Git and dependency schedules:

- **New** opens with exactly one Cron leaf (single condition) and schedule
  disabled. There is no separate mode switch required.
- A leaf's trigger type can be changed directly (Cron, URL, Git, dependent
  schedule). Adding another leaf with **+ Condition**, or using **+ Group**,
  automatically becomes **Multiple conditions**, with explicit AND/OR controls.
- Deleting conditions until one remains automatically collapses even deeply
  nested groups to one leaf. The final condition cannot be removed, avoiding
  empty or invalid expressions.
- On **Save**, exactly one leaf serializes in the existing legacy single-trigger
  schema (`type=cron/url/git/dependency`, `condition_group=null`). Two or more
  serialize to `type=multi` with a recursive `condition_group`. The schedule
  ID remains intact and leaf properties (Git ref/path, URL match, Cron timezone,
  parent result/exit/output filters) are preserved during conversion.
- Existing multi schedules with one surviving nested leaf open as a single
  condition. Legacy single schedules can be promoted to AND/OR and reduced
  again without requiring a new schedule.
- Backend cycle validation now traverses *all* edges, including mixes of
  legacy single and nested multi dependencies. Converting a multi schedule to
  a single dependency cannot bypass cycle checks. Covered by
  `TestScheduleSingleMultiTransitionRejectsMixedDependencyCycle`.
- The Git condition editor retains editable path, detected repository dropdown,
  and manual rescan. When changing a leaf into Git, its empty path prefills the
  currently selected Git-bar repository if discoverable.
- The old top-level "Trigger type / Multiple conditions" selector and separate
  single-condition inputs are hidden; the common expression editor is the only
  authoritative place to change trigger conditions.

Regression tests:
```bash
cd vscode_tasks_menu_go
node --test tests/schedule_single_multi.test.mjs
node --test tests/schedule_conditions_and_views.test.mjs
go test ./internal/server -run 'TestSchedule' -count=1
go build ./...
```

Focused direct JavaScript interaction/serialization checks: **13/13 PASS**.
A complete Node/Go test suite and live Firefox UI test have not run in the
current environment. Verify that the server accepts an existing single
schedule converted to multi and reduced back to single after updating
TaskDeck from main.


## Retry exit-code policy (2026-10-18)

An additional optional `retry_exit_codes` list is now available in Schedule
Details. With this list empty, retries preserve the existing behavior (any
non-zero exit, up to `retry_max`). With it set to comma-separated exit codes
such as `1, 42`, retries occur only for those failure codes. Successful
exits, canceled/timed-out work, and exhausted retry budgets never retry.
The accepted values are 1–64 nonzero integers between -255 and 255; -1 can be
used to retry process failures when they are *not* cancellations.

Implementation:
- `schedule_model.go`: JSON property and validation
- `schedule_retry_policy.go`: pure retry predicate
- `schedule_runtime.go`: integrates the predicate with the existing retry loop
- `schedulespanel.js`: editable comma-separated field
- `schedule_retry_policy_test.go`: regression cases for success, cancellation,
  allowlisted/unlisted failures, and max attempt count

```bash
cd vscode_tasks_menu_go
go test ./internal/server -run TestScheduleRetryExitCodeAllowlist -count=1
go test ./internal/server -run TestSchedule -count=1
go build ./...
```

These test commands require a real TaskDeck checkout; the connector-only
editing environment has not executed them. Existing code already contains
condition TTL/inspector, retry/backoff, queue/replace/resource-group policy,
workflow-run linkage, simulation, file trigger, calendar exclusion, backfill,
import/export, run analytics and local alert views. Further audit and runtime
verification are required before claiming the entire 15-feature program is
complete. In particular, external webhook triggers and out-of-app alert
delivery still need implementation or validation.


## Authenticated webhook event triggers (2026-10-18)

Added `webhook` as a push-only Schedule trigger type. It may be the sole
condition or a leaf in a nested AND/OR expression. The Schedule polling
loop excludes webhook leaves.

An authorized TaskDeck project administrator can submit an event to the
existing management endpoint:

```http
POST /api/schedules
Content-Type: application/json

{"action":"signal_webhook","id":"SCHEDULE_ID","condition_id":"CONDITION_ID"}
```

- A single-trigger schedule uses `condition_id="legacy"`; multi-trigger
  schedules use the server-issued IDs displayed in the condition editor.
- The API verifies that the schedule exists, the specified leaf is a
  webhook, that the schedule is enabled, not paused and within its calendar.
  The event is accepted with HTTP 202 and enters the normal AND/OR latch.
- **Security limitation**: this is an **administrator-authenticated inbound
  event API**, NOT an anonymous public webhook URL. The existing
  `project.admin` permission gate remains active. Do not embed an admin
  session token in third-party webhook configuration. A separately scoped
  and cryptographically verified public webhook endpoint has not yet been
  implemented.
- The API accepts an event; HTTP 202 is not proof that a job actually
  started, because AND conditions may still be waiting or concurrency
  may block execution. Check the condition inspector/run history.
- Added `TestScheduleWebhookConditionValidation` to ensure webhook works
  both as a single leaf and inside AND expressions.

```bash
cd vscode_tasks_menu_go
go test ./internal/server -run 'TestSchedule(Webhook|RetryExitCode)' -count=1
go build ./...
```

Verification still needs to run against a checkout; the GitHub connector
editing environment has no built Go binary or full browser runtime.


## External event and notification work (2026-10-18)

### Outbound completion notifications

Each schedule can configure `notify_webhook_url`, `notify_on_success`,
and `notify_on_failure`. Notifications are JSON HTTP POST requests emitted
after the final retry attempt, asynchronously and independently of the
schedule's result and downstream dependency signals.

Example sanitized body:
```json
{
  "event": "schedule.run.finished",
  "schedule_id": "SCHEDULE_ID",
  "schedule_name": "Deploy",
  "run_id": "RUN_ID",
  "workflow_id": "WORKFLOW_ID",
  "status": "failure",
  "exit_code": 42,
  "attempts": 3,
  "started_at": "2026-10-18T10:30:00Z",
  "finished_at": "2026-10-18T10:31:00Z"
}
```

The outbound transport rejects local/private destination IPs, credentials in
URLs, unsupported schemes and redirects; uses a DNS-validated direct dial,
does not inherit a proxy, bounds connection/request timeouts, and never sends
command lines, logs, captured environment variables or secrets. Errors
are recorded under the `schedule_id:notification` diagnostic key rather
than changing the run status. The URL is a privileged schedule setting; do
not expose it to non-admin users.

### Incoming webhook status

The new `webhook` condition is push-only; no background polling occurs. An
admin-authorized `signal_webhook` action accepts a specific condition ID
and adds a signal to the same existing AND/OR event evaluator. See the
authenticated webhook section above. A standalone publicly accessible
signed-provider callback, with replay prevention and scoped secrets, remains
a separate security-sensitive integration and has **not** been implemented.

### Tests and remaining work

- `internal/server/schedule_retry_policy_test.go` — retry allowlist and
  canceled/exhausted attempt handling.
- `internal/server/schedule_webhook_test.go` — single and nested webhook
  validation.
- `internal/server/schedule_notifications_test.go` — notification event
  settings and URL validation.
- Focused static checks verified JavaScript parses and checked Go imports
  across 11 changed files. They do not replace real compiler/test execution.

```bash
cd vscode_tasks_menu_go
go build ./...
go test ./internal/server -run 'TestSchedule' -count=1
node --test tests/schedule_single_multi.test.mjs
node --test tests/schedule_conditions_and_views.test.mjs
```

The current repository already contains substantial implementations for
Condition Inspector and simulation, TTL, retry/backoff, concurrency
queue/replace/resource locks, workflow ID tracking and a workflow execution
view, next Cron, local alerts, pause/resume/backfill, output variables,
file triggers, calendar exceptions, import/export/clone, and analytics.
These need whole-system integration testing, especially workflow crossover,
restart/interruption, retention, overlap and shared multi-user permissions.
The new outbound notifications support generic webhook delivery; email and
chat-specific adapters have not been implemented.
