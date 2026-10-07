# TaskDeck FTP/SFTP Connection Pool Handoff

Last updated: 2026-10-07
Target branch: main

## Goal

Add a configurable FTP/SFTP per-profile connection budget for scans, remote browsing and uploads/downloads.

Required behavior:
- Default max concurrent remote connections: 3.
- Scan operations consume the same connection budget as transfers.
- With max=3 and 1 scan running, at most 2 uploads/downloads run concurrently.
- With max=3 and 2 scans running, at most 1 upload/download runs concurrently.
- If the budget is fully occupied, additional scans wait in the scan queue instead of exceeding the limit.
- When no connection slot is available, Remote · FTP/SFTP directory navigation must be disabled/blocked until a slot is available.
- Setting must persist with the FTP/SFTP profile and survive restart/update.
- Existing profiles without the field normalize to default 3.

## Implementation plan

- [x] Add profile-level max_connections with validation/default.
- [x] Add daemon per-profile connection scheduler shared by scans and transfers.
- [x] Make server transfer workers concurrent up to available budget instead of serial.
- [x] Queue scans when the shared budget is exhausted.
- [x] Expose connection usage/limit in transfer queue snapshot.
- [x] Gate remote directory navigation/list requests using the same budget.
- [x] Add profile UI control, default 3, and persist it.
- [x] Reflect active/queued scan and connection usage in UI.
- [x] Add backend/frontend/profile persistence tests.
- [x] Document and complete final build-oriented review.

## Progress

### 2026-10-07 — start
- Existing daemon transfer queue is serial (one worker loop), while job scans start immediately in goroutines and only track ActiveScans; there is no shared connection cap.
- Profile schema currently has no concurrency/connection field.
- Implementation will make the daemon authoritative for remote connection budgeting.

### 2026-10-07 — implementation checkpoint

- Profile schema now persists `max_connections`; missing values normalize to 3 and accepted range is 1–16.
- Daemon queue now uses a per-profile shared connection scheduler. `ActiveScans + ActiveTransfers + interactive remote connections` cannot exceed the profile limit.
- Scans are queued as `scan_queued`; queued scans are considered busy work and receive a freed slot before new background transfer items.
- Daemon transfer items now run concurrently up to remaining capacity instead of one-at-a-time.
- Recovered scans after daemon restart return through the same scheduler rather than launching unbounded goroutines.
- Direct list/mutate/upload/download/host transfer, remote hash/text and SFTP archive operations all acquire the same connection budget.
- The Remote pane disables path/history/Go/Up/Refresh and folder navigation when the pool is full; backend 429 remains authoritative against races.
- Browser/Local transfer queue now runs parallel workers. Pool-full races requeue the item with `Waiting for an FTP/SFTP connection slot` instead of marking it Failed.
- Transfer Queue shows `Connections active/max`, running scans and queued scans, and includes a live `Max connections` control that persists back into the saved profile.
- Shared-server users need `settings.write` to change the live limit; read/transfer users still receive the current limit from snapshots.
- Regression coverage added for profile defaults/validation/persistence, 3-slot scan/transfer examples, queued fourth scan, connection snapshot and UI contracts.
- Static review found no unused imports in modified Go files; all 326 existing dual-pane UI string contracts currently match `filetransfer.js`.

## Final validation

- Static import review: no unused Go imports detected in all modified profile/server/test files.
- Existing dual-pane UI contract review: all 326 required strings in `file_transfer_dual_pane_ui_test.go` are present in the current `filetransfer.js`.
- JavaScript syntax was re-parsed after each major `filetransfer.js` edit before commit.
- GitHub combined status for the checkpoint HEAD contains no status checks and no workflow runs; therefore this batch does **not** claim a GitHub CI `go test`/build result.
- Documentation updated in `CONNECTIONS.md` and `README.md`.

## Key commits

- `f6e9ff8` / `7313917` / `16418b0`: profile schema/API/UI for persisted `max_connections`, default 3.
- `0bfff7b` / `5df490e` / `6204171`: shared daemon budget, concurrent scheduler, queued/recovered scans.
- `4959eae` / `a52d8a3` / `2ebc613` / `5e8cef9`: gate browse/navigation and direct browser/host transfers.
- `08b0528` / `6f73f8f` / `4124d63`: parallel browser workers, pool-full requeue and immediate navigation lock.
- `35c3495` / `dd0e42c`: live Transfer Queue Max connections control with shared-server permission enforcement.
- `d360675` / `0c5dd4a` / `c5fe34c`: include remote hash/text/archive operations in the same pool.
- `0fd915f` / `9fdc647` / `3bd90a9` / `022e706` / `fa230b4` / `0459f2d`: profile, scheduler, persistence and UI regression coverage.
- `83b91e5` / `83dd5f1`: user-facing connection-pool documentation.

## Status

This FTP/SFTP connection-pool batch is complete on `main`.
