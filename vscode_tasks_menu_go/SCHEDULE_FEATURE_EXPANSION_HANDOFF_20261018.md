# Schedule Expansion Handoff — 2026-10-18

Branch: `main`. This work implements the high-priority core and most management
features proposed in the Schedule roadmap in independently committed increments.
**Do not claim production readiness until a real Go build, Node tests and
Firefox integration checks have passed.** The GitHub connector can edit files
but the current environment does not have a full private repository checkout.

## Implemented / available for testing

| Feature | Status | Implementation |
| --- | --- | --- |
| Condition inspector | Implemented | GET `/api/schedules` adds `condition_inspector`; POST `inspect` returns per-leaf state, last observation/error, expiry and next Cron dates. |
| Retry with backoff | Implemented | `retry_max` 0–10, `retry_delay_seconds`, `retry_backoff`; successive attempts use the same run ID and append to its bounded log, with cancellation awareness. |
| Condition TTL | Implemented | `condition_ttl_seconds` 0–604800. AND/OR evidence older than TTL no longer counts. |
| Next Run / Timeline | Partial | `inspect` predicts up to ten next Cron dates. No dedicated multi-day calendar timeline yet. |
| Concurrency policy | Implemented | Skip, parallel, bounded queued, replace (cancel previous and queue) and exclusive `resource_group`; queued runs show `queued`, can be cancelled and become interrupted on restart. |
| Workflow Run ID | Implemented | `workflow_id` propagates through dependent triggers, `trigger_run_id` tracks direct parent; AND of OR branches resolves compatible workflow combinations rather than mixing unrelated chains. |
| Condition simulator | Implemented | POST `simulate` evaluates caller-supplied synthetic condition bits without launching any task or network probe. |
| Notifications | Partial | `Alerts` page for recent failures, unusually long execution or queued delay. External email/webhook delivery is **not implemented**. |
| Pause/Resume/Backfill | Implemented | Pause/Resume actions; deliberate Cron backfill preview/queue for **single Cron only**, limited to 7 days/10 occurrences with dedup. No automatic catch-up after restart. |
| Output variables | Partial | Explicit `TASKDECK_OUTPUT NAME=value` lines (20 keys, max 1024 bytes each) stored per run, passed to the directly triggered child as `TASKDECK_DEP_NAME` environment variables. Multi-parent aggregation/passing files is not implemented. |
| Webhook Trigger | Not implemented | Needs a security-reviewed, authenticated and replay-resistant external endpoint that safely interoperates with GitHub/other providers. Do not bypass TaskDeck same-origin/auth middleware casually. |
| File/directory trigger | Implemented | File condition change/exists/missing, polling >=30s, relative workspace paths only, symlink traversal prevention, max 512 immediate children per watched directory. |
| Calendar Exceptions | Implemented | Exclude dates, allowed weekdays, time windows including overnight; only automatic execution is gated, manual Run is an explicit override. |
| Templates / Import / Export | Partial | Duplicate a schedule; JSON Export all and atomic Import disabled copies, remap dependencies, validated by existing engine. No reusable template gallery or schedule version-history UI yet. |
| Analytics | Implemented | Server aggregate successful/failed count, average elapsed seconds and recent error; `Analytics` page. |
| Workflow Execution view | Implemented | Dedicated `Workflow runs` page groups actual run records by `workflow_id`, shows parent-run reference and opens console. This is a run list/grouped view, not yet a full per-execution SVG DAG. |

## File map

- `internal/server/schedule_model.go`: state fields and validation
- `internal/server/schedule_runtime.go`: polling, condition latches, dispatch,
  retry attempts and correlated dependency signaling
- `internal/server/schedule_inspection.go`: TTL, workflow-aware recursive AND/OR,
  next Cron estimate and run analytics
- `internal/server/schedule_queue.go`: bounded queues and resource exclusion
- `internal/server/schedule_file.go`: file/directory poll and path safety
- `internal/server/schedule_calendar.go`: execution calendar
- `internal/server/schedule_outputs.go`: bounded opt-in stdout variables
- `internal/server/schedule_import.go`: ID/parent remapping
- `internal/server/schedule_backfill.go`: controlled backfill
- `internal/server/schedule_api.go`: inspect/simulate/pause/resume/import/backfill APIs
- `web/featuremods/schedulespanel.js`: settings, controls, import/export, backfill
- `web/featuremods/schedulemonitor.js`: condition inspector and simulation
- `web/featuremods/scheduleviews.js`: analytics, alert list, workflow executions
- `web/featuremods/scheduleconditions.js`: file trigger and multi/single editor
- `web/featuremods/next.js`: monitor module import

## Operational semantics and cautions

1. **TTL:** no backdated matching after the window expires. Existing
   conditions remain a sequence of event edges; URL/Git `clean/dirty`
   condition checks are not synchronous snapshots of all conditions. An
   inspector `matched` status is informational, distinct from a latched event.
2. **Correlation:** independent Cron/URL/Git conditions can participate in an
   AND combination with a related workflow. Parent-dependent leaves from
   **different workflows are not combined**. All parent IDs share one workflow
   root across fan-out and fan-in.
3. **Retry:** failed attempts reuse one run ID and the downstream dependencies
   are signaled **only after the final successful or failed outcome**. Be
   careful retrying non-idempotent actions: deploy, database updates, payment
   actions, file overwrites. Set `retry_max=0` if unsure.
4. **Queue:** queued entries are persisted but explicitly marked interrupted
   after server restart rather than executed unexpectedly. Queue is capped at
   ten per schedule and 100 per workspace. Replace cancels active runs using
   the established process-group termination logic, then queues a new run.
5. **Resource group:** a nonempty label forms an exclusive mutex across
   schedules and within the same schedule. A queued run is dispatched only
   after resources and concurrency become available.
6. **Backfill:** never replays composite AND/OR expressions. It requires a
   preview and confirmation and queues up to ten unique Cron timestamps.
7. **Export/Import:** importing creates fresh IDs, remaps imported dependencies
   and disables all imported schedules. Review commands and parent mappings
   before enabling; exported JSON may contain commands and paths, so treat it
   as confidential project configuration.
8. **Output variables:** lines prefixed `TASKDECK_OUTPUT ` are parsed from
   console output, bounded, persisted, and available only through
   `TASKDECK_DEP_<NAME>` for direct descendant runs. Never emit passwords or
   secrets in console logs.
9. **Webhooks:** deliberately not exposed before a safe token/HMAC,
   anti-replay and authorization design is implemented and tested.

## Validation

Run in a full repository checkout:

```bash
cd vscode_tasks_menu_go
go build ./...
go test ./internal/server -run 'TestSchedule' -count=1
node --test tests/schedule_single_multi.test.mjs
node --test tests/schedule_conditions_and_views.test.mjs
node --test tests/schedule_history_isolation.test.mjs
node --test tests/schedule_operability_features.test.mjs
```

Manual Firefox integration smoke:

1. Create single Cron, select retry/TTL/resource group and save.
2. Add Git and file conditions in AND; use Simulate, Inspector and expiry
   status. Convert back to single and confirm legacy config survives.
3. Run A → B/C → D and confirm workflow IDs stay together and another
   concurrently started A chain cannot satisfy D using mixed parent runs.
4. Force a command failure, confirm one run with multiple attempts and a
   complete console, then backfill a short historical Cron window.
5. Set Queue and Replace policies, exhaust global/per-schedule/resource limits,
   cancel a queued run and restart the process while runs are queued.
6. Test same-resource exclusivity for concurrent runs and multiple schedules.
7. Configure file-change / missing-file / watched directory; attempt symlinks
   and `../` traversal and verify rejection.
8. Exclude one date and one weekday, verify overnight window, verify manual
   override, inspect alert and analytics views.
9. Export/import two dependent schedules, confirm new parent IDs, both disabled
   and no unintentional executions.
10. Verify in shared-server mode that only users with the existing Schedule
    management permissions can modify definitions and inspect execution logs.

## Remaining priorities

1. Build and integration-test on an actual TaskDeck checkout. Fix *all*
   compiler/test failures before recommending production update.
2. Implement provider-compatible signed webhook trigger safely.
3. Add opt-in external alert delivery (email/webhook) with retries and
   deduplication, avoiding secrets in notifications.
4. Add complete graphical per-execution DAG and multi-parent output aggregation.
5. Add a template gallery, schedule configuration version history and richer
   calendar timeline (including DST and skipped-run previews).
