# TaskDeck Compressed Folder Upload Handoff

Last updated: 2026-10-07
Target branch: main

## Goal

Before uploading selected folders to FTP/SFTP, perform a short bounded scan. When the selection contains many files, suggest an optional archive workflow while always preserving normal per-file upload.

Required behavior:
- Scan for only a few seconds / until a high-file-count threshold is reached; never require a full pre-scan before the user can continue.
- If many files are detected, offer: Compress + upload, Upload normally, or Cancel.
- Upload normally must preserve the existing scanner/queue/conflict/recovery path unchanged.
- Archive mode must preserve top-level selected names and refuse automatic extraction when those destination roots already exist.
- SFTP + authorized SSH shell: TaskDeck may automatically extract the uploaded archive and remove it after successful extraction.
- FTP, or SFTP without authorized SSH shell: upload the archive and present copyable extraction commands for the user to run manually.
- Host and Local-browser folder uploads should both receive the suggestion where technically supported.
- No npm/CDN/runtime dependency download; use Go stdlib and browser-native APIs only.

## Plan
- [ ] Add bounded Host folder pre-scan API.
- [ ] Add archive-only Host upload API using temporary tar.gz.
- [ ] Add remote extract-only SFTP/SSH API with collision preflight and cleanup.
- [ ] Fix shared authorization so every SSH extraction path requires ssh.use.
- [ ] Add Local-browser bounded scan and streaming tar.gz creation using CompressionStream.
- [ ] Add recommendation dialog with explicit Upload normally path.
- [ ] Show manual POSIX/PowerShell extraction commands when auto-extract is unavailable.
- [ ] Add regression tests and documentation.

## Progress
- Existing normal Host upload is daemon-owned via host_upload scan/job.
- Existing Local-browser upload is browser-owned and resumable through persisted scan descriptors.
- Existing archive tool can create safe ZIP/tar.gz from workspace paths and existing SFTP archive upload+extract already has reusable SSH execution helpers.
