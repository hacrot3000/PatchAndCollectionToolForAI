# Contextual command guides / Hướng dẫn lệnh theo ngữ cảnh

## Tiếng Việt

Smart Terminal hiển thị **các thao tác thường gặp, tham số cần điền và mô tả ngắn** khi bạn nhập tên lệnh đã biết, hiện hỗ trợ **Git**. Ví dụ, nhập `git remote set-url` để xem thao tác đổi URL, đặt URL riêng cho push, thêm URL push, xóa một URL push, thêm remote hoặc xóa remote. Các lựa chọn được sắp xếp sao cho thao tác khớp chính xác với chuỗi đang gõ đứng trước các lựa chọn liên quan.

Mỗi mục chứa:

- **Syntax**: ví dụ `git remote set-url <remote> <new-url>`, dấu `<...>` chỉ để hướng dẫn.
- **Mục đích** và **tham số**: mô tả ngắn bằng tiếng Việt để lựa chọn nhanh.
- **Risk**: `normal`, `caution` hoặc `danger`; các thao tác có khả năng mất dữ liệu như `git reset --hard HEAD`, `git clean -fd` được cảnh báo.
- **Command insert**: chỉ chèn tiền tố lệnh hợp lệ, không điền `<url>`, không gửi Enter, không tự thực thi.

Bấm **Tab** hoặc chọn mục bằng chuột để chèn tiền tố. Sau đó nhập những tham số còn lại và nhấn **Enter** khi muốn thực hiện. Phần hướng dẫn **không** gửi lệnh vào terminal trong quá trình hiển thị.

### Cấu trúc mở rộng

Bộ dữ liệu nằm ở `web/featuremods/terminal_command_guides.js` và hoàn toàn tách biệt với giao diện trong `smartterminal.js`. API:

```js
TaskDeckCommandGuides.register({
  id: 'mytool',
  sections: {
    base: [
      // [textToInsert, syntaxForDisplay, title, quickHelp, risk?]
      ['mytool list', 'mytool list', 'Liệt kê', 'Chỉ đọc']
    ],
    config: [
      ['mytool config ', 'mytool config <key>', 'Cấu hình', 'Nhập key']
    ]
  }
});
```

Một nhóm mới nên được đặt trong module độc lập và import trong `web/featuremods/next.js` **sau** `terminal_command_guides.js` nhưng **trước** `smartterminal.js`. Không cần thêm điều kiện đặc biệt trong renderer. Mỗi mục được đánh giá từ nội dung dòng lệnh hiện tại; không có yêu cầu mạng hoặc chạy shell ở lớp registry. Giới hạn các công thức cho những lệnh hữu ích; lệnh nguy hiểm phải có mô tả cảnh báo và không bao giờ kèm ký tự xuống dòng trong `textToInsert`.

Các bài test: `node --test test_terminal_command_guides.cjs` (registry) và `node --test test_smartterminal_path_ui.cjs` (popup + Tab/click).

## English

TaskDeck Smart Terminal supports declarative **command guides**, initially for common Git operations. Typing `git remote set-url` displays relevant alternatives with example syntax, short explanations and risk warnings. Placeholders such as `<remote>` and `<new-url>` are **display-only**, never inserted as executable shell text. Selecting a guide using Tab or click inserts only a partial safe command, without Enter.

The guide provider registry is implemented in `web/featuremods/terminal_command_guides.js`, separate from the renderer. A new command family can be registered by a separate imported provider using `TaskDeckCommandGuides.register({id,sections})`; each recipe is `[insert,syntax,title,help,risk?]`. The registry performs no shell execution or network requests. Review risky operations, add regression tests and import new providers before `smartterminal.js`.
