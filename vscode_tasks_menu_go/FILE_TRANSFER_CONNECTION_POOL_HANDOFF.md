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

- [ ] Add profile-level max_connections with validation/default.
- [ ] Add daemon per-profile connection scheduler shared by scans and transfers.
- [ ] Make server transfer workers concurrent up to available budget instead of serial.
- [ ] Queue scans when the shared budget is exhausted.
- [ ] Expose connection usage/limit in transfer queue snapshot.
- [ ] Gate remote directory navigation/list requests using the same budget.
- [ ] Add profile UI control, default 3, and persist it.
- [ ] Reflect active/queued scan and connection usage in UI.
- [ ] Add backend/frontend/profile persistence tests.
- [ ] Document and complete final build-oriented review.

## Progress

### 2026-10-07 — start
- Existing daemon transfer queue is serial (one worker loop), while job scans start immediately in goroutines and only track ActiveScans; there is no shared connection cap.
- Profile schema currently has no concurrency/connection field.
- Implementation will make the daemon authoritative for remote connection budgeting.
