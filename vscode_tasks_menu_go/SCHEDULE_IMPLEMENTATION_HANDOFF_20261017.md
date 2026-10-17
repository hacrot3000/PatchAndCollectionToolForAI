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
