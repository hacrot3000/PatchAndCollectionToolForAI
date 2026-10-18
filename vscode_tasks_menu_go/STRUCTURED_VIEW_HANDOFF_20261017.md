# TaskDeck — Build/config syntax & Structured View (2026-10-17)

## Tóm tắt / Quick start

TaskDeck recognizes Gradle Groovy (`*.gradle`, `*.groovy`), Gradle Kotlin DSL
(`*.gradle.kts`, Kotlin highlighting), ProGuard/R8 (`*.pro`), common
`*.properties`/INI/TOML/YAML/JSON/XML configuration, CSV, and TSV files.

For structured text files, open the **Editor** and choose the new selector in its
toolbar:

| File | Available modes |
| --- | --- |
| Strict JSON (`.json`, Cocos `.prefab/.scene/.fire/.anim/.meta`, `.ipynb`) | Raw, Pretty, Tree |
| XML (`.xml`, `.plist`, FairyGUI `.fairy`, and existing XML family) | Raw, Pretty, Tree |
| CSV (`.csv`) and TSV (`.tsv`) | Raw, Pretty, Table |
| Gradle/Groovy, YAML, TOML, INI, properties, JSON5/JSONC | Syntax highlighting / Raw (no potentially lossy formatter) |

**VI:** `Raw` là nội dung ban đầu trong CodeMirror. `Pretty` chỉ biểu diễn với
xuống dòng/thụt lề; `Tree` cho phép đóng/mở cây phân cấp; `Table` cho phép
đọc CSV/TSV theo hàng/cột. **Chuyển chế độ không thay đổi nội dung file, không
gọi Save, không đánh dấu dirty**. Giao diện dùng buffer hiện tại (bao gồm thay
đổi chưa lưu). Khi buffer đổi, nhấn **Refresh** để cập nhật preview.

**EN:** The modes are presentation-only. Raw CodeMirror remains the source of
truth; Pretty/Tree/Table never dispatch editor transactions or write to disk.
A source change marks a preview stale, and Refresh recomputes it.

### Technical characteristics

- **JSON Pretty:** checks syntax with `JSON.parse` but formats original JSON
  characters rather than `JSON.stringify(JSON.parse(source))`. It therefore
  preserves original large integer spellings, string escaping and duplicated
  keys in the Pretty text. **JSON Tree** uses parsed objects, so duplicate keys
  collapse there. Never round-trip Tree text into source.
- **XML Pretty:** parsed with `DOMParser(application/xml)`; nesting is indented
  only where child elements have no meaningful mixed text. `xml:space=preserve`
  and mixed-content subtrees are serialized together. **XML Tree** presents
  element names, attributes and text in a collapsible hierarchy.
- **CSV/TSV:** quoted delimiters, doubled escaped quotes, CRLF and embedded
  newlines in quoted fields are handled. Pretty produces a human-readable
  aligned preview; Table renders an accessible HTML table (using textContent,
  not untrusted HTML). The first CSV row is displayed as its column header.
- **Limits:** maximum preview input 1 MiB UTF-8, first 500 tabular rows,
  bounded tree rendering and XML conversion. No network/runtime packages.
  XML with DTD/ENTITY is deliberately excluded from this browser viewer; use
  Raw for these documents. Syntax-invalid content is never silently repaired.
- **Safety:** Use only `textContent` for user data; no `innerHTML`, XSS markup
  injection, eval or API mutation. CodeMirror Save, autosave, find/replace,
  session swap, conflict checks, encoding and line-ending controls remain
  attached to the original buffer.

### Test / Verification

From `vscode_tasks_menu_go`:

```bash
node --test tests/structuredview.test.mjs
node --test tests/build_config_syntax.test.mjs
go test ./internal/server -run 'Test(StructuredFormatterLoadedBeforeEditor|StructuredEditorSyntaxAndReadOnlyView|EditorInitialLanguageCoverageMapping|EditorCompletionTierOneProfilesCoverRegisteredLanguages)' -count=1
```

Manual smoke in browser:
1. Open a one-line JSON file with large integer; switch Raw → Pretty → Tree → Raw.
   Observe no dirty indicator and identical original file content.
2. Repeat for a nested XML file, with attributes and mixed content.
3. Open a CSV file containing quotes, commas and embedded line breaks, and use
   Raw → Pretty → Table. Repeat with TSV.
4. Make an unsaved edit in Raw, open Pretty, return Raw, and verify original
   unsaved text remains unchanged. Reload or edit externally and verify
   Refresh updates stale preview.
5. Test malformed JSON, DTD-containing XML, and oversized files. They must
   retain usable Raw mode with an explicit error.
6. Verify Gradle Groovy (`build.gradle`), `build.gradle.kts`, `gradle.properties`,
   `proguard-rules.pro`, `.editorconfig`, `.csv`, `.tsv` syntax highlighting
   and comment/string distinction.

### Current validation status

- PASS: four-module JavaScript syntax checks and 15/15 V8-focused formatter,
  parser, mode switching, input recognition and buffer-preservation checks.
- PASS: 17/17 additional language-registry checks including Gradle Groovy,
  Gradle Kotlin DSL, ProGuard/R8, XML/JSON/CSV/TSV and Swift/TypeScript regressions.
- The dedicated Node tests and Go embedded integration test files have been
  committed; running them in the repository environment and browser visual
  smoke remains to be done.
