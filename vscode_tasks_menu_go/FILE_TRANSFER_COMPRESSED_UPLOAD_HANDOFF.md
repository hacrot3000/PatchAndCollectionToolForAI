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
- [x] Add bounded Host folder pre-scan API.
- [x] Add archive-only Host upload API using temporary tar.gz.
- [x] Add remote extract-only SFTP/SSH API with collision preflight and cleanup.
- [x] Fix shared authorization so every SSH extraction path requires ssh.use.
- [x] Add Local-browser bounded scan and streaming tar.gz creation using CompressionStream.
- [x] Add recommendation dialog with explicit Upload normally path.
- [x] Show manual POSIX/PowerShell extraction commands when auto-extract is unavailable.
- [x] Add regression tests and documentation.

## Progress
- Existing normal Host upload is daemon-owned via host_upload scan/job.
- Existing Local-browser upload is browser-owned and resumable through persisted scan descriptors.
- Existing archive tool can create safe ZIP/tar.gz from workspace paths and existing SFTP archive upload+extract already has reusable SSH execution helpers.

## Completed behavior

- Quick scan is bounded to 2.5 seconds and stops early at 200 files; timeout with at least 100 files also triggers the suggestion.
- The recommendation dialog always keeps **Upload normally** available; normal Host/Local upload paths continue through the existing scanner/queue/conflict/recovery implementations.
- Host archive mode creates a temporary tar.gz with Go stdlib, uploads it once, then removes the local temp file.
- Local-browser archive mode streams the selected FileSystemHandle tree through a built-in TAR writer and browser-native `CompressionStream('gzip')`; no npm/CDN/runtime dependency was added.
- Archive mode preserves selected top-level names and is disabled by the UI when a top-level destination already exists.
- Automatic SFTP extraction performs the same top-level collision preflight again immediately before extraction, so stale browser cache/races cannot silently overwrite those roots.
- Automatic extraction requires SFTP plus authorized `ssh.use`; existing manual Host archive extraction was also tightened to require `ssh.use` in shared mode.
- FTP or SFTP without authorized SSH uploads the archive and shows copyable POSIX and PowerShell commands. FTP UI warns that FTP-visible paths may be chroot/virtual paths and may need shell-path adjustment.
- If automatic extraction fails, the uploaded archive is intentionally kept and manual commands are shown. If archive creation/upload fails before extraction, the user can choose normal upload as fallback.
- Archive upload/extract operations participate in the existing per-profile FTP/SFTP connection budget.

## Regression / static validation

- Added backend tests for the 200-file quick-scan cutoff, archive root validation, manual command preflight/cleanup, collision-safe extract command and unsupported archive formats.
- Added shared-route permission coverage for quick scan, archive upload and archive extract; SSH extraction routes explicitly require `ssh.use`.
- Added dual-pane UI contract coverage for quick scan, normal-upload choice, local native gzip creation, archive upload/extract APIs and manual command UI.
- Latest static contract review: all 360 string contracts in `file_transfer_dual_pane_ui_test.go` are present in current `filetransfer.js`.
- Latest modified Go implementation/test import review found no unused imports.
- `filetransfer.js` was syntax-parsed with `new Function(...)` on each direct edit performed in this batch.

## Documentation

- `CONNECTIONS.md` documents the 2.5-second/200-file heuristic, normal-upload fallback, collision rule, Host/Local implementation, SSH permission behavior and FTP chroot caveat.
- `README.md` summarizes compressed folder upload support.

## Status

This compressed-folder upload batch is complete on `main`. GitHub CI/status availability is checked separately at the final HEAD; do not infer a CI pass from static review alone.
