# TaskDeck — iOS / Swift syntax highlighting (2026-10-17)

## Phạm vi / Scope

- **Swift (`.swift`)**: auto-detect trong editor theo tên file, tô màu keyword, tên type,
  `@propertyWrapper` / attribute, `#directive`, số, chuỗi thường/raw/multiline,
  escaped identifiers và comment `//` / `/* ... */` lồng nhau.
- Dùng **cùng lexer Swift** trong editor, File Compare, Git diff preview và những
  nơi File Compare dùng tô màu syntax; không ghi/chỉnh sửa source khi tô màu.
- Autocomplete nội bộ Tier 1 cho Swift keyword, SwiftUI và Swift Concurrency snippets.
  Swift Tier 2 project-symbol completion chưa được hỗ trợ: chủ động không gửi
  request backend cho Swift để tránh các request không có kết quả.
- Nhận diện bổ sung file dự án iOS: `.m` Objective-C, `.mm` Objective-C++,
  `.metal` / `.modulemap` dùng highlighting C/C++; `.plist`, `.storyboard`,
  `.xib`, `.entitlements`, `.stringsdict`, `.xcworkspacedata`, `.xcsettings`
  dùng XML; `.xcconfig` dùng config; `.strings` dùng string/comment highlighting.
- Không thay vendor CodeMirror, không CDN, không npm install khi chạy.

## Thử nghiệm / Verification

Từ `vscode_tasks_menu_go`:

```bash
node --test tests/swiftsyntax.test.mjs
go test ./internal/server -run 'Test(SwiftSyntax|EditorInitialLanguageCoverageMapping|EditorCompletionTierOneProfilesCoverRegisteredLanguages)' -count=1
```

**Kết quả kiểm tra trong phiên:** parse JavaScript cho lexer, editor, completion và
File Compare: **PASS**; thực thi 6 fixture lexer Swift bằng V8: **6/6 PASS**.
Go test và smoke test thực tế trên browser chưa được chạy trực tiếp từ môi trường này.

Thử trên trình duyệt: mở `ContentView.swift`, chỉnh sửa keyword/`@State`/
Swift multiline strings, mở File Compare của cùng file, đổi light/dark theme,
đảm bảo highlight không thay đổi buffer, Save vẫn bảo toàn source.

## Giới hạn / Limitations

- Lexer là bộ tô màu lightweight, không phải Swift parser/SourceKit-LSP và không
  cung cấp semantic diagnostics hoặc semantic completion.
- Objective-C / Metal đang dùng highlighter C/C++; các cú pháp đặc thù như
  Objective-C message send chưa được semantic-highlight riêng.
- XML files được nhận diện theo extension; binary `.plist` không thể đọc
  như văn bản và không được chuyển đổi tự động.
- Syntax được tính theo document và cache theo immutable CodeMirror document;
  một chỉnh sửa tạo document mới, tự loại cache cũ bằng WeakMap.
