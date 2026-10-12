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

## 2026-10-12 — SFTP intermittent download visibility and retries

Confirmed problems from the code: **Download to browser** previously navigated a hidden iframe immediately after ticket issuance. The transfer never appeared in Transfer Queue and HTTP 429/SFTP errors could be invisible. **Download to left (Host)** creates a daemon scanner job, but the browser previously rendered only `snapshot.items`; before the first item was found (or if an SFTP scan failed early), the queue appeared empty. Background queue polling also logged refresh errors only to the JS console, and overlapping polls could apply an older snapshot after a newer one.

Changes: browser downloads up to 128 MiB use a visible queued fetch with streamed byte counts, clear HTTP errors, 429 pool retry and remote size verification. Larger known files retain browser-native iframe streaming without buffering but explicitly show a **handoff, not verified completion** in queue; use **Download to left → Host** to track daemon-side file completion for very large downloads. Daemon job submission immediately displays a temporary row and does not report an accepted job as failed just because the subsequent GET queue refresh errors. Queue now renders synthetic **Scan** rows for queued/running/failed/empty-result daemon scan jobs, displays poll failures, and rejects obsolete overlapping poll results. **Retry failed** on backend requeues failed directory scans with the original request, not just per-file failures, without restarting successfully completed or intentionally stopped scans. Backend scanner/transfer worker limits remain unchanged (per-profile Max Connections default 3). No server changes required for API response fields because job state and errors were already present in `snapshot.jobs`.

Follow-up: A daemon submission placeholder remains in the Transfer Queue until a GET snapshot actually acknowledges its job ID, even if the immediate post-submission refresh fails. After two minutes with no matching daemon job the placeholder becomes a visible diagnostic failure (the client does **not** automatically duplicate the POST). Browser-local downloads show an explicit local-folder permission step before entering the scan queue; permission rejection is retained as a failed row. The global Retry failed command explains when automatic retry would risk duplicating an unconfirmed submission and requires a manual retry. 

## 2026-10-12 — Multi-GB upload/download and live progress

Previously the daemon rejected all files over 1 GiB (`maxFileTransferBytes=1<<30`); SFTP transfers inherited connect timeout + 30s as a hard whole-command timeout; FTP data sockets used an absolute one-shot timeout rather than an idle timeout. Now the safety cap is **64 GiB per file**, FTP data socket deadlines renew on I/O and reset before the final 226 reply, while OpenSSH SFTP file-copy commands can run up to 24 hours with SSH keepalives and cancellation. This does not loosen short timeouts on metadata/list/mutate requests.

Server Host upload/download jobs report `bytes_done`, `bytes_total` and `speed_bps` via queue snapshots. FTP counts streaming reader/writer bytes, SFTP Host downloads observe the growing partial file, and SFTP Host uploads periodically measure destination size (best effort, requires remote stat and may lag). Browser-local uploads use XMLHttpRequest upload progress and browser-local downloads stream fetch into File System Access writable without buffering all bytes. Browser-native large downloads retain native streaming and poll the one-use progress-ticket endpoint for bytes/errors. SFTP browser downloads stream from a growing temporary file as SFTP receives it rather than waiting for the complete file in an HTTP request; the browser download manager remains responsible for final disk-save completion. Resume of partial Host downloads was already supported; a failed upload may need manual Retry/verification.

Regression: verify a sparse multi-GiB payload over real FTP/SFTP profiles, including network throttling, reconnects, cancel/retry, disk-space constraints and reverse-proxy upload request-timeout settings. Do not report a successful browser disk save based only on the HTTP server finishing its write.

## 2026-10-12 — Log tăng trưởng khi đang download / nguồn upload thay đổi

**Remote SFTP/FTP → Host** now handles an actively appended game-server log. The size found during initial scan is a hint, not a strict final-length requirement. It downloads streaming to the existing private partial file, then checks a fresh remote stat. If new bytes have appeared, it resumes from the previous local end with a **64 KiB overlap**, compares all overlap bytes before appending any new data, and repeats up to five catch-up rounds. The first completed transfer also performs a tail recheck. FTP uses REST to seek efficiently where supported; otherwise it streams from the beginning and skips old bytes before verifying overlap, at the cost of additional bandwidth. SFTP rewinds a small local suffix before OpenSSH `reget`, compares the suffix, then keeps only the verified append. Progress totals grow with the file instead of staying pinned to scan-time size.

For remote rotation, truncation, replacement with a larger/different log, or modification without appending, the download is **restarted cleanly once**, never splicing incompatible bytes into the result. On a second inconsistent change it fails with a descriptive message; it does not repeatedly restart forever. If the log keeps growing beyond the five catch-up rounds, the tool saves the verified prefix as a **snapshot** and labels it as such; this is a bounded download, not a permanent `tail -f`. A suffix overlap and remote size/mtime checks substantially reduce risk but are **not a full-file integrity proof** against adversarial concurrent rewrites. A true immutable snapshot requires filesystem-level snapshotting, locking or a complete stable remote checksum.

**Host/staged browser upload retries** now verify the last 64 KiB of the remote partial against the current source before resuming; a mismatched, truncated or unverifiable remote partial forces a full upload from byte zero rather than blind concatenation. SFTP uses a sparse local resume probe so only the remote suffix is transferred; FTP uses REST or an explicit streamed fallback. If the original local source is changed during upload, the job reports an explicit error rather than claiming an internally consistent file was uploaded. This is intentionally not automatic append-follow for a changing **upload source**.

**Scope:** the append/rotate-safe continuation is implemented for the daemon **Download to Host** operation; Browser-native and **Download to Local browser** still use their own streaming paths and do not have the same verified append-loop guarantee. For actively rotating game logs use **Remote → Host** until verified-browser snapshot staging is implemented. Integration tests simulate a log growing from 100 to 150 lines, a larger replacement log, upload-tail mismatch and a sparse 3 GiB partial. Real-network verification with a live game server is still required before promising continuous log consistency.

Additional browser SFTP fix: a live file can outgrow its initial stat during the HTTP transfer. The SFTP browser response now uses chunked streaming rather than freezing `Content-Length`, and its ticket progress total follows the number of bytes actually delivered. This eliminates the stale-length HTTP failure, but native browser downloads remain a stream snapshot without the Host job's verified multi-round append/rotation recovery. Server-side "success" only means the HTTP body was written, not that Firefox saved it.
