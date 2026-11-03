# Virtual large-file editor (host project files)

This feature is an opt-in transport used by TaskDeck's Project Editor when the host file exceeds the original 2 MiB editor limit. It does not alter the small-file CodeMirror path or FTP/SFTP remote-editor transport.

## Architecture and memory budget

- `GET /api/project/file?windowed=1&path=...` returns metadata without returning the whole large file.
- `GET /api/project/file-window?path=...&start=...&count=...` delivers absolute, contiguous source lines. A typical initial window is 440 lines with approximately 110 buffered lines before the target. Scrolling prefetches adjacent chunks of up to 100 lines, retaining at most roughly 720 lines subject to a 2 MiB browser buffer limit. The server enforces at most 1,024 lines and 2 MiB per response.
- On first access, Go scans the source stream to validate UTF-8, count newlines, calculate the SHA-256 revision, and build a sparse index of offsets every 256 lines. Only the offsets and metadata are cached (up to 12 files). Subsequent jumps seek to the nearest index checkpoint.
- The browser creates a single CodeMirror instance for the active window. The line navigator, Page buttons and global slider use **absolute file line numbers**. Approaching the edge of the current buffer prefetches contiguous ranges. Old offscreen lines are evicted while preserving the visual top-line anchor (no forced re-centering); jumping a long distance makes a new bounded request. Gutter numbers are absolute file line numbers.
- `GET /api/project/file-window/search` scans server-side, with forward or backward/wrap search; no 100 MB text is transferred for searches.
- `POST /api/project/file-window/replace` supports literal **Replace** (one match at a verified absolute line and UTF-8 byte column) and **Replace All** (whole large file, regardless of the visible window). Both run in a streaming Go worker, using the same pinned, atomic temporary-file write path; they never invoke a shell or need the whole source in browser memory. The request is guarded by the original full-file SHA-256, size and modification time, and the source is rechecked before commit. All matches preserve unrelated bytes, including CRLF/LF and Unicode text. Find/Replace share the same literal matching semantics. A dirty editor window must be saved/discarded/canceled first. This API requires the normal files.write permission in shared mode.
- File revisions carry `mtime_ns_exact` as a decimal string in window responses. Replace and Save send `expected_mtime_ns_exact` so JavaScript cannot round a 64-bit nanosecond timestamp. The legacy numeric fields remain for older clients, but the server prioritizes the exact string. Real external writes are still rejected by matching file version and SHA-256 before the atomic commit.
- `PUT /api/project/file-window/save` sends the edited window and its original byte offsets, the full-file SHA-256, the original window SHA-256, file size and nanosecond modification time. The backend streams unmodified prefix and suffix around the change into a pinned temporary file, preserves permissions/metadata using existing safe I/O, checks revision again and atomically replaces the source. No whole-file upload or 100 MB in-memory editor buffer is required.
- An edited window must be saved, discarded, or kept open before navigating away. This avoids silently saving changes when Editor Auto Save is OFF. A bounded unsaved window draft is kept in sessionStorage for reload recovery. Failed saves and conflicts keep the user's original buffer editable.

## Limits and deliberate safeguards

- Only UTF-8 text without NUL is supported. Single physical lines longer than 2 MiB are rejected instead of expanding client memory unexpectedly.
- While a large-file Replace or Replace All is pending, the editor is temporarily read-only: CodeMirror transactions, Save, Auto Save, Reload, navigation and tab closing are blocked until the server request and subsequent buffer refresh complete. If the current window has unsaved changes, the operator explicitly chooses Save / Discard / Cancel before the replacement is sent; Save failure or Cancel stops the replacement without dropping the draft. Discard first downloads the replacement window successfully and only then removes the old draft; a failed reload preserves unsaved text. External changes in other processes remain guarded by the server's SHA-256 and version checks.
- Large-file editing supports single-match Replace and full-document Replace All via a streaming server operation (literal text, not a regular expression). Replacement text is capped at 64 KiB, source lines at 2 MiB, and a full-document operation at 2,000,000 matches; files with invalid UTF-8/NUL are rejected. Outline, Markdown preview, and structured visualization remain unavailable in windowed mode.
- Changes to line-ending or BOM format are disabled in the windowed UI; each window retains its existing CRLF/LF representation.
- Unlike small file saves, **local-history snapshots are not generated** for large files. Commit/backup important files in Git. The server invalidates the project symbol index after a large-file edit.
- This virtual mode currently targets **host project files**; it does not yet provide range edits for browser-local, FTP/SFTP, or remote-workspace files.
- The physical browser scrollbar belongs to the current window; the adjacent global slider represents the full-file range. End-to-end browser interaction and Go validation still require execution before merging to main.

## Keyboard indentation (normal and windowed editors)

- Both **Edit File** and **Edit Big File** use the shared keyboard handler in `web/featuremods/editor.js`. **Tab** indents selected nonblank lines in one CodeMirror transaction rather than replacing the selection; **Shift+Tab** outdents them. The selection, including backward selection and end-at-next-line boundaries, is preserved.
- The indentation unit is inferred from the detected language, local leading whitespace, neighboring source blocks, and lightweight syntax hints (including Python and Nim block openers). Tabs stay tabs and spaces stay spaces; a tab elsewhere in the file does not override a local space-indented block. Python/Nim default to four/two spaces only when no local evidence exists. Other consistent widths, including three and six spaces, are recognized.
- Consecutive Tab/Shift+Tab on the same unchanged selection retains the exact inserted units so unusually indented blocks remain reversible.
- This is a context-based heuristic, **not a full language parser**. It does not normalize other lines or rewrite pre-existing mixed indentation. Large-file inference is limited to the currently buffered source window; it cannot inspect unseen portions of the file. Editing is blocked when the window is unloaded, saving/replacing, or read-only.
- An open CodeMirror completion popup continues to own Tab for accepting a suggestion. Smart Enter uses the inferred unit when indenting bracket blocks.

Regression command from the repository root:

```sh
node --test vscode_tasks_menu_go/test_editor_indentation.cjs
```

## Toggle comments

- **Ctrl+/** (macOS **Cmd+/**) comments/uncomments the selected nonblank lines or the current line, using the same CodeMirror handler as Edit File. All-commented selection: remove one level. Any uncommented line: comment the selected block, including already-commented rows for languages with nestable line prefixes.
- Comment syntax is language-aware (`#`, `//`, `--`, `;`, HTML/XML `<!-- -->` and CSS `/* */`); mixed or ambiguous HTML/Vue source region boundaries, strict JSON and formats without safe comment syntax are not auto-commented. Non-nestable block comments are handled conservatively. Selection, indentation, Undo grouping and immediate exact toggle reversal are preserved.
- The large-file editor only scans and edits the already-loaded bounded window; an unloaded, saving, replacing or read-only window cannot accept edits. Whole-file cross-window selections are not supported.
- Regression: `node --test vscode_tasks_menu_go/test_editor_comment_toggle.cjs`.

## Verification

From `vscode_tasks_menu_go`:

```sh
gofmt -e internal/server/project_large_editor.go >/dev/null
go test ./internal/server -run 'Test(WindowedEditor|LargeEditorReplace)' -count=1
node --test tests/large_file_editor.test.mjs tests/large_file_editor_scroll.test.mjs
go test ./...
go build ./cmd/vscode_tasks_menu
```

Manual smoke: open a 100 MB multi-line UTF-8 project file, drag the global slider far into the file, search and jump to a match, edit and save a region, verify untouched prefix/suffix checksums, and repeat after a concurrent external file edit (save must return conflict). Repeat with CRLF, non-ASCII text, Replace/Replace All, a stale file revision (409), and tab restore. Validate on Linux and macOS; also confirm the existing Editor works unchanged for files under 2 MiB.
