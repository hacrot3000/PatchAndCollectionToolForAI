# Virtual large-file editor (host project files)

This feature is an opt-in transport used by TaskDeck's Project Editor when the host file exceeds the original 2 MiB editor limit. It does not alter the small-file CodeMirror path or FTP/SFTP remote-editor transport.

## Architecture and memory budget

- `GET /api/project/file?windowed=1&path=...` returns metadata without returning the whole large file.
- `GET /api/project/file-window?path=...&start=...&count=...` delivers absolute, contiguous source lines. A typical window is 180 lines, including approximately 60 buffered lines before the requested top line. The server enforces at most 1,024 lines and 2 MiB per response.
- On first access, Go scans the source stream to validate UTF-8, count newlines, calculate the SHA-256 revision, and build a sparse index of offsets every 256 lines. Only the offsets and metadata are cached (up to 12 files). Subsequent jumps seek to the nearest index checkpoint.
- The browser creates a single CodeMirror instance for the active window. The line navigator, Page buttons and global slider use **absolute file line numbers**. Reaching the top/bottom of the current buffer fetches adjacent ranges; jumping a long distance makes a new bounded request.
- `GET /api/project/file-window/search` scans server-side, with forward or backward/wrap search; no 100 MB text is transferred for searches.
- `PUT /api/project/file-window/save` sends the edited window and its original byte offsets, the full-file SHA-256, the original window SHA-256, file size and nanosecond modification time. The backend streams unmodified prefix and suffix around the change into a pinned temporary file, preserves permissions/metadata using existing safe I/O, checks revision again and atomically replaces the source. No whole-file upload or 100 MB in-memory editor buffer is required.
- An edited window must be saved, discarded, or kept open before navigating away. This avoids silently saving changes when Editor Auto Save is OFF. A bounded unsaved window draft is kept in sessionStorage for reload recovery. Failed saves and conflicts keep the user's original buffer editable.

## Limits and deliberate safeguards

- Only UTF-8 text without NUL is supported. Single physical lines longer than 2 MiB are rejected instead of expanding client memory unexpectedly.
- Large-file editing does not support full-document Replace All, outline, Markdown preview, or structured visualization. These controls are disabled to prevent treating a partial window as the whole document.
- Changes to line-ending or BOM format are disabled in the windowed UI; each window retains its existing CRLF/LF representation.
- Unlike small file saves, **local-history snapshots are not generated** for large files. Commit/backup important files in Git. The server invalidates the project symbol index after a large-file edit.
- This virtual mode currently targets **host project files**; it does not yet provide range edits for browser-local, FTP/SFTP, or remote-workspace files.
- The physical browser scrollbar belongs to the current window; the adjacent global slider represents the full-file range. End-to-end browser interaction and Go validation still require execution before merging to main.

## Verification

From `vscode_tasks_menu_go`:

```sh
gofmt -e internal/server/project_large_editor.go >/dev/null
go test ./internal/server -run 'TestWindowedEditor' -count=1
node --test tests/large_file_editor.test.mjs
go test ./...
go build ./cmd/vscode_tasks_menu
```

Manual smoke: open a 100 MB multi-line UTF-8 project file, drag the global slider far into the file, search and jump to a match, edit and save a region, verify untouched prefix/suffix checksums, and repeat after a concurrent external file edit (save must return conflict). Repeat with CRLF, non-ASCII text and tab restore. Validate on Linux and macOS; also confirm the existing Editor works unchanged for files under 2 MiB.
