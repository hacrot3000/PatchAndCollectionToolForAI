package main

// All UI assets are compiled into the independently built native executable.
// PHP/Relay never serve or select HTML/JS and never see credentials or the
// localhost-only capability. Dynamic Agent output is rendered with textContent.
const trustedGUIHTML = `<!doctype html>
<html lang="vi">
<head>
 <meta charset="utf-8">
 <meta name="viewport" content="width=device-width, initial-scale=1">
 <title>TaskDeck · Remote Workspace</title>
 <link rel="stylesheet" href="/style.css">
 <script src="/app.js" defer></script>
</head>
<body>
 <main class="app">
  <aside class="rail" aria-label="Workspace navigation">
   <div class="logo">T<span>D</span></div>
   <div class="workspace"><span class="overline">TRUSTED CLIENT</span><strong>Remote Workspace</strong><small>Native E2EE • localhost only</small></div>
   <nav aria-label="Remote features">
    <button type="button" class="nav active" id="nav-overview" aria-current="page"><span>01</span> Tổng quan</button>
    <button type="button" class="nav" id="nav-project" aria-current="false"><span>02</span> Dự án · chỉ đọc</button>
    <button type="button" class="nav" id="nav-tasks" aria-current="false"><span>03</span> Task · chạy có điều kiện</button>
    <button type="button" class="nav" id="nav-git" aria-current="false"><span>04</span> Git · quyền riêng</button>
    <button type="button" class="nav" id="nav-terminal" aria-current="false"><span>05</span> Terminal · PTY</button>
    <div class="nav muted"><span>06</span> Các lệnh Git khác <em>Chưa bật</em></div>
    <button type="button" class="nav" id="nav-files" aria-current="false"><span>07</span> Files · chỉ đọc</button>
   </nav>
   <div class="rail-note">Mật khẩu TaskDeck chỉ nhập trong client CLI đáng tin cậy, không nhập tại PHP Portal hoặc trang web khác.</div>
  </aside>
  <section class="content">
   <header>
    <div><span class="overline">TASKDECK REMOTE</span><h1>Không gian làm việc từ xa</h1><p>Giao diện cục bộ của client đã ghim danh tính Agent. Các thao tác được mã hóa đầu cuối.</p></div>
    <span class="state" id="state">Chờ xác minh phiên</span>
   </header>
   <div class="activation" id="activation" hidden>
    <label for="local-token">Nhập mã kích hoạt từ terminal client TaskDeck tin cậy</label>
    <div class="activation-form">
     <input id="local-token" type="password" inputmode="text" autocomplete="off" autocapitalize="off" spellcheck="false" placeholder="64 ký tự hex; tuyệt đối không lấy từ PHP Portal">
     <button type="button" id="activate">Kích hoạt GUI</button>
    </div>
    <p>Trang này chạy tại 127.0.0.1. Mã chỉ được gửi về client native trên máy của bạn, không gửi tới PHP/Relay.</p>
   </div>
   <div class="view-head" aria-live="polite"><h2 id="view-title">Tổng quan</h2><p id="view-hint">Xem task và Git Status chỉ đọc; Git pull/commit/push cần opt-in riêng. Đổi tab không gửi thêm yêu cầu E2EE.</p></div>
   <section class="quick-actions" aria-label="Thao tác nhanh đã giới hạn">
    <strong>Thao tác nhanh · Không phải Terminal</strong>
    <p>Chỉ những yêu cầu đọc cố định đã được host phê duyệt. Không chấp nhận đường dẫn, đối số, lệnh shell hoặc Git ghi.</p>
    <div class="quick-row">
     <label for="quick-command">Lệnh (gõ help để xem hỗ trợ)</label>
     <div class="quick-form"><input id="quick-command" type="text" maxlength="64" autocomplete="off" autocapitalize="off" spellcheck="false" placeholder="help | features | ping | project | tasks | tasks next | git status" disabled><button type="button" id="quick-run" disabled>Thực hiện</button></div>
    </div>
    <p class="quick-hint" id="quick-hint" role="status" aria-live="polite"></p>
   </section>
   <section class="gui-panel" id="panel-overview" >
   <div class="target-summary" aria-label="Danh tính máy chủ và tài khoản đã xác thực"><div><span class="overline">THIẾT BỊ ĐÃ GHIM</span><strong id="target-device">Đang xác thực...</strong></div><div><span class="overline">TÀI KHOẢN TASKDECK</span><strong id="target-user">Đang xác thực...</strong></div></div>
   <div class="banner"><strong>Giới hạn bảo mật</strong><p>Đây là bản GUI thử nghiệm, không phải điều khiển toàn bộ TaskDeck. PHP/Relay chỉ trung chuyển ciphertext. Terminal PTY chỉ có trên Linux/macOS nếu bật quyền riêng; không có proxy TaskDeck HTTP. Chạy task và các thao tác Git pull, commit, push chỉ được phép sau khi host bật riêng từng chức năng, kiểm tra quyền và ghi audit. Không hỗ trợ force-push.</p></div>
   <div class="feature-check">
    <div><strong>Quyền đọc Remote GUI</strong><p id="feature-summary">Chưa kiểm tra. Hãy xác minh các chức năng host đã bật trước khi truy cập dự án, task hoặc Git.</p></div>
    <button type="button" id="features">Kiểm tra chức năng</button>
   </div>

   <div class="tiles">
    <article class="tile">
     <div class="tile-heading"><span class="dot"></span><h2>Phiên E2EE</h2></div>
     <p>Kiểm tra Agent đang giữ phiên TaskDeck hợp lệ, khóa client được duyệt và quyền hiện hành.</p>
     <div class="actions"><button type="button" id="ping">Kiểm tra kết nối</button></div>
    </article>
   </div>
   </section>
   <section class="gui-panel" id="panel-project" hidden>
   <div class="tiles">
    <article class="tile">
     <div class="tile-heading"><span class="dot"></span><h2>Dự án TaskDeck</h2></div>
     <p>Đọc tên dự án, workspace và nền tảng host thực tế. Không truy xuất đường dẫn tuyệt đối hay danh sách dự án khác.</p>
     <div class="actions"><button type="button" id="project">Thông tin dự án</button></div>
    </article>
   </div>
   <section class="project-overview" id="project-overview" hidden aria-label="Thông tin dự án được TaskDeck xác minh">
    <div><span class="overline">DỰ ÁN</span><strong id="project-name"></strong></div>
    <div><span class="overline">WORKSPACE</span><strong id="workspace-name"></strong></div>
    <div><span class="overline">HOST OS</span><strong id="host-os"></strong></div>
   </section>
   </section>
   <section class="gui-panel" id="panel-files" hidden>
    <article class="tile">
     <div class="tile-heading"><span class="dot"></span><h2>Files · Thư mục gốc</h2></div>
     <p>Đọc danh sách tên file/thư mục an toàn thuộc workspace đã được host cấp quyền. Mỗi lần nhận tối đa 8 mục qua E2EE, giới hạn 128 mục nguồn. Nếu danh sách thay đổi giữa các trang, server sẽ từ chối tiếp.</p>
     <div class="actions">
      <button type="button" id="files-root" disabled>Đọc danh sách</button>
      <button type="button" id="files-root-next" disabled>Trang tiếp</button>
     </div>
     <pre class="git-patch-output" id="files-root-output" hidden aria-label="Danh sách Files chỉ đọc"></pre>
     <div class="tile-heading"><span class="dot"></span><h2>Duyệt thư mục con · chỉ đọc</h2></div>
     <p>Chọn thư mục xuất hiện trong danh sách gốc đã xác thực. Agent chỉ liệt kê mục con trực tiếp của thư mục được host cho phép.</p>
     <label for="files-dir-select">Thư mục gốc đã nhận qua E2EE</label>
     <select id="files-dir-select" disabled><option value="">Chọn thư mục</option></select>
     <div class="actions">
      <button type="button" id="files-dir-open" disabled>Xem thư mục</button>
      <button type="button" id="files-dir-next" disabled>Trang thư mục tiếp</button>
     </div>
     <pre class="git-patch-output" id="files-dir-output" hidden aria-label="Mục con một cấp"></pre>
     <label for="files-select">Chọn file đã được liệt kê</label>
     <select id="files-select" disabled aria-label="File văn bản gốc">
      <option value="">Chọn file từ danh sách đã nhận</option>
     </select>
     <div class="actions"><button type="button" id="files-read-selected" disabled>Đọc file đã chọn</button></div>
     <p>Agent chỉ cho đọc file trong danh sách cho phép do host cấu hình; không hỗ trợ đường dẫn tùy ý.</p>
     <div class="tile-heading"><span class="dot"></span><h2>Đọc file văn bản host phê duyệt</h2></div>
     <p>Chỉ đọc một file gốc được cấu hình trên Agent. Không có ô nhập đường dẫn và không cung cấp quyền tải xuống hoặc sửa.</p>
     <div class="actions">
      <button type="button" id="files-read-start" disabled>Đọc nội dung</button>
      <button type="button" id="files-read-next" disabled>Đọc phần tiếp</button>
     </div>
     <pre class="git-patch-output" id="files-read-output" hidden aria-label="Nội dung file host chỉ định"></pre>
     <p>Chưa hỗ trợ chọn file tùy ý, sửa file, upload, download hoặc duyệt thư mục con. Tất cả đường dẫn do host quyết định, PHP Portal không nhận dữ liệu file ở dạng rõ.</p>
    </article>
   </section>
   <section class="gui-panel" id="panel-tasks" hidden>
   <div class="tiles">
    <article class="tile">
     <div class="tile-heading"><span class="dot"></span><h2>Task của workspace</h2></div>
     <p>Xem task qua E2EE. Khi host bật quyền, chạy task ngắn hoặc job dài có giới hạn; hỗ trợ các input process token/pickString đã xác minh. Không nhập lệnh shell tùy ý từ client.</p>
     <div class="actions">
      <button type="button" id="tasks-preview">Xem trước task</button><button type="button" id="tasks-report" disabled>Xem báo cáo task gần đây</button><button type="button" id="tasks-workflow-audit" disabled>Xem audit bước workflow</button><button type="button" id="tasks-workflow-audit-next" disabled>Xem tiếp audit</button>
      <button type="button" id="task-run-stop" hidden>Dừng task và ngắt phiên</button>
     </div>
     <div class="actions" id="long-task-controls" hidden>
      <button type="button" id="long-task-poll">Nhận output tiếp</button>
      <button type="button" id="long-task-copy" disabled>Sao chép output đã nhận</button>
      <button type="button" id="long-task-auto">Tạm dừng tự nhận</button>
      <button type="button" id="long-task-cancel">Hủy job (giữ phiên)</button>
      <button type="button" id="long-task-clear">Dọn job đã kết thúc</button>
     </div>
     <pre id="long-task-output" hidden aria-live="polite"></pre><pre id="tasks-report-output" hidden aria-label="Báo cáo kết quả task E2EE"></pre>
     <section class="task-run-history" id="task-run-history" hidden aria-label="Kết quả chạy task trong phiên hiện tại">
      <div class="task-run-history-head"><strong>Kết quả chạy task gần đây · chỉ lưu trong phiên</strong><button type="button" id="task-run-history-clear">Xóa kết quả</button></div>
      <ol id="task-run-history-list"></ol>
     </section>
    </article>
   </div>
   <section class="task-preview" id="task-preview" hidden aria-label="Task trên workspace đã xác minh">
    <div class="task-preview-heading"><strong>Danh sách task · chỉ xem trước</strong><span id="task-total"></span></div>
    <div class="task-filter-row">
     <div><label for="task-deps-select">Xem phụ thuộc của task đã được cấp qua E2EE</label>
      <select id="task-deps-select" disabled><option value="">Chọn Task</option></select></div>
     <button type="button" id="task-deps-view" disabled>Xem thứ tự và phụ thuộc · chỉ đọc</button>
     <button type="button" id="task-workflow-start" disabled>Chạy workflow đã kiểm tra</button>
    </div>
    <pre id="task-deps-output" hidden aria-label="Đồ thị phụ thuộc task chỉ đọc"></pre>
    <div class="task-coverage" aria-live="polite">
     <strong id="task-coverage">Chưa tải danh sách task</strong>
     <progress id="task-progress" value="0" max="1" aria-label="Tỷ lệ nhãn task đã tải"></progress>
     <span id="task-next-help"></span>
    </div>
    <div class="task-filter-row">
     <div>
      <label class="task-search-label" for="task-filter">Tìm trong nhãn task đã tải về</label>
      <input id="task-filter" type="search" maxlength="64" autocomplete="off" spellcheck="false" placeholder="Lọc cục bộ, không gửi yêu cầu Relay">
     </div>
     <div>
      <label class="task-search-label" for="task-sort">Sắp xếp các nhãn đã tải</label>
      <select id="task-sort" aria-label="Sắp xếp task đã tải">
       <option value="host">Thứ tự trên host</option>
       <option value="az">Tên A–Z</option>
       <option value="za">Tên Z–A</option>
      </select>
     </div>
    </div>
    <section class="task-comparison" aria-label="So sánh danh sách nhãn task đã xác thực">
     <strong>Thay đổi nhãn task giữa hai lần tải đầy đủ</strong>
     <p id="task-compare-summary" role="status" aria-live="polite">Tải đầy đủ nhãn task để tạo mốc so sánh.</p>
     <ul id="task-compare-list" aria-label="Nhãn mới xuất hiện hoặc không còn xuất hiện"></ul>
    </section>
    <p id="task-filter-empty" hidden>Không tìm thấy nhãn phù hợp trong các task đã tải.</p>
    <ul id="task-preview-labels"></ul>
    <div class="task-fetch-actions">
     <button type="button" id="task-fetch-next" hidden>Tải thêm task từ host (1 lượt E2EE)</button>
     <button type="button" id="task-fetch-all" hidden>Tải các trang còn lại (có thể dùng hết lượt E2EE)</button>
     <button type="button" id="task-stop-all" hidden>Dừng tải sau lượt đang xử lý</button>
    </div>
    <p id="task-load-status" role="status" aria-live="polite"></p>
    <div class="task-paging" id="task-paging" hidden>
     <button type="button" id="task-prev">Trang trước</button>
     <span id="task-page">Trang 1 / 1</span>
     <button type="button" id="task-next">Trang tiếp</button>
    </div>
    <p>Task ngắn tối đa 6 giây; job dài tối đa 90 giây trên Linux/macOS khi bật thêm TASKDECK_REMOTE_TASKS_LONG=1. Output, số lượt E2EE và input luôn có giới hạn.</p>
   </section>
   </section>
   <section class="gui-panel" id="panel-git" hidden>
   <div class="tiles">
    <article class="tile">
     <div class="tile-heading"><span class="dot"></span><h2>Git Status</h2></div>
     <p>Đọc trạng thái repository gốc trên máy TaskDeck. Chỉ hoạt động khi host bật riêng quyền đọc Git.</p>
     <div class="actions"><button type="button" id="git">Đọc trạng thái Git</button><button type="button" id="git-log" disabled>Git log (3 commit)</button><button type="button" id="git-branches" disabled>Nhánh Git cục bộ</button><button type="button" id="git-remotes" disabled>Git remote (chỉ tên)</button><button type="button" id="git-tracking-start" disabled>Tracking refs cục bộ</button><button type="button" id="git-tracking-next" disabled>Trang tracking tiếp</button><button type="button" id="git-branch-create" disabled>Tạo nhánh đã cấu hình trên host</button><button type="button" id="git-switch-review" disabled>Xem trước chuyển nhánh</button><button type="button" id="git-switch-same-head" disabled>Chuyển nhánh cùng HEAD</button><button type="button" id="git-branch-delete-review" disabled>Xem trước xoá nhánh TaskDeck</button><button type="button" id="git-branch-delete" disabled>Xoá nhánh đã xem trước</button><button type="button" id="git-diff" disabled>Git diff --stat</button><button type="button" id="git-patch-start" disabled>Xem diff chi tiết</button><button type="button" id="git-patch-cached-start" disabled>Review staged diff</button><button type="button" id="git-patch-next" disabled>Trang diff tiếp</button><button type="button" id="git-fetch" disabled>Git fetch (upstream)</button><button type="button" id="git-pull-review" disabled>Xem trước Pull (offline)</button><button type="button" id="git-pull-changes" disabled>Xem file sẽ thay đổi</button><button type="button" id="git-pull" disabled>Git pull (fast-forward)</button><button type="button" id="git-push" disabled>Git push (không force)</button></div>
      <p>Diff chi tiết chỉ đọc tối đa 4 KiB thay đổi tracked, chia trang E2EE; Review staged diff chỉ xem thay đổi đã stage. Snapshot thay đổi giữa các trang sẽ bị từ chối.</p><pre id="git-patch-output" class="git-patch-output" aria-label="Git diff chi tiết" hidden></pre>
      <p>Git fetch chỉ cập nhật upstream tracking, không đổi HEAD; Git pull yêu cầu worktree sạch. Git push chỉ đẩy commit HEAD lên upstream cùng tên nhánh, không force, không đẩy tags; mọi thao tác yêu cầu quyền riêng trên host và xác nhận.</p>
      <div class="actions git-index-actions"><button type="button" id="git-stage-tracked" disabled>Stage file đã theo dõi</button><button type="button" id="git-unstage-all" disabled>Unstage toàn bộ</button></div>
      <p>Stage chỉ lấy thay đổi của file đã theo dõi (kể cả file bị xoá), không thêm file mới. Unstage xóa toàn bộ staging nhưng không thay đổi working tree.</p>
      <div class="git-commit-form"><label for="git-commit-message">Commit message (tối đa 120 byte UTF-8)</label>
       <input id="git-commit-message" type="text" maxlength="120" spellcheck="false" autocomplete="off" placeholder="Nội dung commit, một dòng" disabled>
       <button type="button" id="git-commit" disabled>Commit các thay đổi đã stage</button>
      </div>
      <p>Commit không tự stage thêm file, không chạy hook hoặc ký commit. Cần quyền git.write và opt-in riêng tại host.</p>
    </article>
   </div>
   <section class="git-panel">    <div class="git-summary" id="git-summary" hidden>
     <div class="git-branch" id="git-branch"></div>
     <p class="git-snapshot-note">Ảnh chụp từ lần đọc E2EE gần nhất; không tự cập nhật. Host hiện bỏ qua file chưa được theo dõi (untracked) và từ chối kết quả vượt giới hạn 256 byte. Không dùng bảng này để kết luận repository hoàn toàn sạch.</p>
     <div class="git-inspector">
      <div><label for="git-filter">Tìm đường dẫn đã đọc (cục bộ)</label><input id="git-filter" type="search" maxlength="64" autocomplete="off" spellcheck="false" placeholder="Không gửi thêm yêu cầu E2EE" disabled></div>
      <div><label for="git-kind">Lọc thay đổi</label><select id="git-kind" disabled>
       <option value="all">Tất cả</option>
       <option value="staged">Đã stage</option>
       <option value="unstaged">Chưa stage</option>
       <option value="untracked">Chưa theo dõi (nếu host cung cấp)</option>
       <option value="conflict">Xung đột</option>
      </select></div>
     </div>
     <p class="git-count" id="git-count" aria-live="polite"></p>
     <p id="git-empty" hidden>Không có thay đổi phù hợp với bộ lọc.</p>
     <table aria-label="Các tệp thay đổi trên Git"><thead><tr><th>Trạng thái</th><th>Đường dẫn tệp</th></tr></thead><tbody id="git-files"></tbody></table>
     <div class="git-file-actions"><label for="git-file-select">Một file đã theo dõi từ Git Status E2EE</label><select id="git-file-select" disabled><option value="">Chọn file</option></select><button type="button" id="git-stage-file" disabled>Stage file này</button><button type="button" id="git-unstage-file" disabled>Unstage file này</button></div><p>Chỉ file có tên an toàn trong ảnh chụp Git Status mới được chọn; thay đổi status sẽ yêu cầu đọc lại.</p>
     <section class="git-delta" aria-label="So sánh hai ảnh chụp Git Status đã xác thực">
      <strong>Thay đổi so với lần đọc trước</strong>
      <p id="git-delta-summary" role="status" aria-live="polite">Đọc Git Status lần nữa để xem khác biệt.</p>
      <ul id="git-delta-list" aria-label="Các thay đổi giữa hai lần đọc Git Status"></ul>
      <p>Chỉ so sánh trạng thái file trên cùng nhánh/tracking; không phải Git diff nội dung và không đồng bộ upstream. Mỗi lần đọc lại dùng 1 lượt E2EE; xem kết quả không gửi thêm yêu cầu.</p>
     </section>
    </div>
   </section>
   </section>
   <section class="gui-panel" id="panel-terminal" hidden>
    <article class="tile terminal-panel">
     <div class="tile-heading"><span class="dot"></span><h2>Terminal PTY · Phiên tạm</h2></div>
     <p>Chỉ có trên Linux/macOS khi host bật TASKDECK_REMOTE_TERMINAL=1 và cấp đủ terminal.create/view_own/control_own. Shell cố định /bin/sh; tối đa 90 giây, 16 lượt E2EE và output có giới hạn. Không chuyển qua PHP.</p>
     <div class="actions terminal-actions">
      <button type="button" id="terminal-open" disabled>Mở Terminal</button>
      <button type="button" id="terminal-poll" disabled>Đọc output</button>
      <button type="button" id="terminal-auto" disabled>Bật tự nhận output</button>
      <button type="button" id="terminal-interrupt" disabled>Ctrl+C</button>
      <button type="button" id="terminal-eof" disabled>Ctrl+D</button>
      <button type="button" id="terminal-close" disabled>Đóng Terminal</button>
     </div>
     <p id="terminal-state" role="status">Chưa mở Terminal.</p>
     <pre id="terminal-output" class="terminal-output" aria-label="PTY output (text only)"></pre>
     <div class="terminal-input-group">
      <label for="terminal-input">Gửi một dòng shell (tối đa 95 byte UTF-8)</label>
      <div class="quick-form">
       <input type="text" id="terminal-input" maxlength="95" autocomplete="off" spellcheck="false" placeholder="Nhập lệnh rồi nhấn Enter" disabled>
       <button type="button" id="terminal-send" disabled>Gửi</button>
      </div>
     </div>
     <div class="terminal-resize">
      <label for="terminal-cols">Cột</label><select id="terminal-cols"><option value="80">80</option><option value="120">120</option><option value="160">160</option></select>
      <label for="terminal-rows">Dòng</label><select id="terminal-rows"><option value="24">24</option><option value="32">32</option><option value="48">48</option></select>
      <button type="button" id="terminal-resize" disabled>Đổi kích thước</button>
     </div>
     <p>Output là văn bản thuần, không chạy escape sequence từ host dưới dạng HTML. “Tự nhận output” chỉ đọc sau khi mở/gửi lệnh/đổi kích thước hoặc khi Agent báo còn output; không Poll vô hạn lúc rảnh. Mỗi lần đọc sử dụng một lượt E2EE, luôn giữ lượt cuối để đóng Terminal.</p>
    </article>
   </section>
   <section class="result">
    <div class="result-head"><strong>Phản hồi được xác thực</strong><span id="remaining">Tối đa 16 yêu cầu mỗi phiên</span></div>
    <div class="session-metadata" aria-live="polite"><span>Thời gian còn lại: <strong id="session-clock">Đang kiểm tra</strong></span><span id="session-activity">Không có tác vụ đang chạy</span><span>Độ trễ E2EE: <strong id="ping-latency">Chưa đo</strong></span></div>
    <pre id="output" role="status" aria-live="polite">Chọn một thao tác ở trên. Không có dữ liệu nào được tải từ PHP Portal.</pre>

   </section>
   <footer>
    <span id="security">Origin 127.0.0.1 • local capability • strict CSP</span>
    <button type="button" class="danger" id="disconnect">Ngắt phiên</button>
   </footer>
  </section>
 </main>
</body>
</html>`

const trustedGUICSS = `:root{color-scheme:dark;font-family:ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#0b1018;color:#e9f0f9}*{box-sizing:border-box}body{margin:0}.app{min-height:100vh;display:grid;grid-template-columns:268px minmax(0,1fr)}.rail{background:#101824;border-right:1px solid #273344;padding:22px 15px;display:flex;flex-direction:column;gap:24px}.logo{font-size:24px;font-weight:900;letter-spacing:-2px;color:#8cc1ff}.logo span{color:#c2f0dc}.workspace{display:flex;flex-direction:column;gap:8px}.workspace strong{font-size:19px}.workspace small{color:#a4b1c4}.overline{color:#78b6f5;font-size:10px;font-weight:800;letter-spacing:1.8px}.rail nav{display:flex;flex-direction:column;gap:6px}.nav{border-radius:7px;padding:12px 10px;font-size:12px;display:flex;align-items:center;gap:9px}.nav span{font-size:10px;opacity:.7}.nav.active{background:#1d3147;border:1px solid #365775;color:#e7f5ff}.rail button.nav{background:transparent;color:#c3d0e1;border:1px solid transparent;width:100%;text-align:left;font-weight:600;white-space:nowrap}.rail button.nav.active{background:#1d3147;border-color:#365775;color:#f4f9ff}.rail button.nav:focus-visible{outline:2px solid #9fd7ff;outline-offset:1px}.view-head{margin:20px 0 12px}.view-head h2{font-size:18px;margin:0 0 6px}.view-head p{font-size:12px;line-height:1.6;color:#adc3d8;margin:0}.gui-panel[hidden]{display:none!important}.git-panel{margin-top:16px;background:#142331;border:1px solid #315367;border-radius:9px}.terminal-panel{margin-top:14px}.terminal-actions{flex-wrap:wrap}.terminal-output{background:#07101a;color:#d2f4e5;border:1px solid #2d5460;border-radius:7px;min-height:180px;max-height:380px;white-space:pre-wrap;overflow:auto;font:12px ui-monospace,monospace;padding:14px}.terminal-input-group label,.terminal-resize label{font-size:11px;color:#c4d9ed}.terminal-input-group input{flex:1;min-width:120px;background:#0c1826;color:#eff7ff;border:1px solid #527594;border-radius:6px;padding:10px;font:12px ui-monospace,monospace}.terminal-resize{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-top:12px}.terminal-resize select{border:1px solid #527594;background:#0c1826;color:#eff7ff;padding:8px;border-radius:6px}.git-panel .git-summary{border-top:0}.git-file-actions{display:flex;flex-wrap:wrap;gap:9px;margin-top:12px;align-items:center}.git-file-actions label{font-size:11px;color:#c3dbef}.git-file-actions select{max-width:240px;padding:8px;background:#0c1826;color:#eff7ff;border:1px solid #527594;border-radius:6px}.git-patch-output{max-height:340px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;background:#0b1521;color:#d3e7f4;padding:12px;border:1px solid #375873;border-radius:8px;font-size:11px}.git-index-actions{flex-wrap:wrap;margin-top:10px}.git-commit-form{display:flex;gap:9px;align-items:flex-end;flex-wrap:wrap;margin-top:12px}.git-commit-form label{font-size:12px;flex-basis:100%;color:#c5d9ec}.git-commit-form input{flex:1;min-width:195px;padding:10px 12px;border:1px solid #527594;border-radius:6px;background:#0c1826;color:#eff7ff;font-size:12px}.git-commit-form input:focus-visible{outline:2px solid #9fd7ff;outline-offset:1px}.git-panel .git-summary[hidden]{display:none}.nav.muted{opacity:.53;flex-wrap:wrap}.nav em{font-size:10px;margin-left:auto}.rail-note{font-size:11px;color:#aab8cb;line-height:1.6;margin-top:auto;border-top:1px solid #293445;padding-top:14px}.content{max-width:1200px;width:100%;padding:36px 42px;margin:auto}header{display:flex;align-items:flex-start;justify-content:space-between;gap:20px}h1{font-size:28px;letter-spacing:-.8px;margin:10px 0}header p{font-size:13px;color:#aab8cb;line-height:1.5;max-width:600px}.state{border:1px solid #517a78;color:#afeee0;background:#123834;padding:8px 11px;border-radius:50px;font-size:11px;white-space:nowrap}.state.bad{border-color:#8d5354;color:#ffd3d3;background:#432326}  .activation{padding:16px;margin:16px 0;border:1px solid #6084a8;background:#162b40;border-radius:9px}.activation label{display:block;font-size:12px;font-weight:700;color:#cde9fe;margin-bottom:10px}.activation-form{display:flex;gap:10px}.activation-form input{flex:1;min-width:0;background:#0b1725;border:1px solid #6485a6;color:#fff;border-radius:6px;padding:10px;font:12px ui-monospace,monospace}.activation p{font-size:11px;color:#b9cde1;line-height:1.5}.target-summary{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:22px}.target-summary>div{border:1px solid #303e50;background:#152334;border-radius:9px;padding:12px 14px;display:flex;flex-direction:column;gap:6px;min-width:0}.target-summary strong{font-size:12px;overflow-wrap:anywhere;color:#deedf9}.banner{margin:28px 0;border-left:3px solid #d3aa68;background:#242333;padding:15px 18px;border-radius:6px}.banner strong{font-size:12px;color:#ffe0a7}.banner p{font-size:12px;color:#c6cfdf;line-height:1.5;margin-bottom:0} .feature-check{display:flex;align-items:center;justify-content:space-between;gap:14px;margin:16px 0 22px;padding:14px 18px;border:1px solid #456181;background:#172b40;border-radius:9px}.feature-check strong{font-size:13px}.feature-check p{font-size:11px;color:#b9cde1;line-height:1.5;margin:6px 0 0}.feature-check button{flex-shrink:0}.tiles{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.tile{border:1px solid #303e50;background:#172130;border-radius:10px;padding:20px}.tile-heading{display:flex;gap:9px;align-items:center}.tile-heading h2{margin:0;font-size:16px}.dot{height:8px;width:8px;border-radius:50%;background:#83bdff} .project-overview{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin-top:16px}.project-overview[hidden]{display:none}.project-overview>div{background:#142b37;border:1px solid #315967;border-radius:8px;padding:14px;display:flex;flex-direction:column;gap:8px;min-width:0}.project-overview strong{font-size:13px;overflow-wrap:anywhere;color:#d9edf9} .task-preview{margin-top:16px;padding:16px;background:#142632;border:1px solid #305166;border-radius:9px}.task-preview[hidden]{display:none}.task-preview-heading{display:flex;justify-content:space-between;gap:12px;font-size:13px}.task-preview-heading span,.task-preview p{color:#aab8cb;font-size:11px}.task-coverage{display:flex;flex-direction:column;gap:8px;margin:12px 0;padding:11px 13px;border:1px solid #305269;background:#0e202e;border-radius:7px}.task-coverage strong{font-size:12px}.task-coverage span{font-size:11px;color:#c2d3e4;line-height:1.55}.task-coverage progress{width:100%;height:8px;accent-color:#72baed}.task-filter-row{display:grid;grid-template-columns:minmax(0,2fr) minmax(0,1fr);gap:12px}.task-filter-row>div{min-width:0}.task-preview select{width:100%;border:1px solid #527594;background:#0c1826;color:#eff7ff;border-radius:6px;font-size:12px;padding:10px}.task-preview #task-filter-empty[hidden]{display:none}@media(max-width:620px){.task-filter-row{grid-template-columns:1fr}}.task-search-label{display:block;font-size:11px;margin:12px 0 6px;color:#c3dbef}.task-preview input[type="search"]{width:100%;border:1px solid #527594;background:#0c1826;color:#eff7ff;border-radius:6px;font-size:12px;padding:10px}.task-preview ul{margin:12px 0;padding-left:18px;font-size:12px;line-height:1.7;overflow-wrap:anywhere}
.task-comparison{margin-top:14px;padding:12px 14px;background:#112634;border:1px solid #31586a;border-radius:7px}
.task-comparison strong{font-size:12px;color:#d5eafb}
.task-comparison p{font-size:11px;line-height:1.6;color:#bdd5e6}
.task-comparison ul{margin:8px 0 0;padding-left:20px}
.task-comparison ul:empty{display:none} .task-fetch-actions{display:flex;flex-wrap:wrap;align-items:center;gap:8px;margin:12px 0}.task-fetch-actions button{max-width:100%;white-space:normal}.task-paging{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin:12px 0}.task-paging[hidden]{display:none}.task-paging span{font-size:11px;color:#b9cde1} .gui-panel>.tiles{margin-top:14px}.gui-panel .tiles{grid-template-columns:1fr}.tile p{min-height:56px;font-size:12px;color:#acbbcf;line-height:1.6}.actions{display:flex;gap:9px}button{cursor:pointer;border:1px solid #406eab;background:#2b5d96;color:#fff;padding:10px 13px;border-radius:6px;font:inherit;font-size:12px;font-weight:700}button:hover{filter:brightness(1.14)}button:disabled{opacity:.42;cursor:not-allowed}.result{background:#101824;border:1px solid #303e50;border-radius:10px;margin-top:18px;overflow:hidden}.result-head{display:flex;justify-content:space-between;gap:8px;background:#192536;padding:13px 16px;font-size:12px} .result-head span{color:#aab8cb}.session-metadata{display:flex;flex-wrap:wrap;gap:10px;justify-content:space-between;background:#101b29;color:#aab8cb;padding:9px 16px;font-size:11px;border-bottom:1px solid #283649}.session-metadata strong{color:#b9defe}pre{font-size:12px;white-space:pre-wrap;overflow-wrap:anywhere;min-height:146px;max-height:350px;overflow:auto;padding:16px;margin:0;line-height:1.6;color:#e2f1fc}footer{display:flex;justify-content:space-between;align-items:center;gap:14px;margin-top:24px;color:#aab8cb;font-size:11px}.git-summary{border-top:1px solid #303e50;padding:15px}.git-branch{font-size:12px;font-weight:700;color:#a5caff;margin-bottom:9px}.git-summary table{border-collapse:collapse;width:100%;font-size:11px}.git-summary th{text-align:left;color:#aab8cb}.git-summary th,.git-summary td{padding:7px;border-bottom:1px solid #2a3647}.git-summary td:first-child{width:110px;font-family:ui-monospace,monospace;color:#f0bd75}.git-summary td:last-child{font-family:ui-monospace,monospace;overflow-wrap:anywhere}.danger{background:transparent;border:1px solid #87515f;color:#ffc7d0}@media(max-width:880px){.app{grid-template-columns:1fr}.rail{padding:12px 16px;gap:12px}.rail nav{flex-direction:row;overflow:auto}.rail-note{display:none}.workspace{gap:3px}.content{padding:20px}header{flex-wrap:wrap}  .feature-check{flex-wrap:wrap}.activation-form{flex-wrap:wrap}.target-summary,.project-overview{grid-template-columns:1fr}.tiles{grid-template-columns:1fr}.tile p{min-height:0}footer{flex-wrap:wrap}}
.git-snapshot-note{font-size:11px;color:#aab8cb;line-height:1.55;margin:6px 0 14px}
.git-inspector{display:grid;grid-template-columns:minmax(0,2fr) minmax(0,1fr);gap:12px}
.git-inspector label{display:block;font-size:11px;color:#c3dbef;margin-bottom:6px}
.git-inspector input,.git-inspector select{width:100%;min-width:0;padding:9px;background:#0c1826;color:#eff7ff;border:1px solid #527594;border-radius:6px;font:12px ui-sans-serif,sans-serif}
.git-inspector :disabled{opacity:.55}
.git-count,#git-empty{font-size:11px;color:#b9cde1;margin:12px 0}
#git-empty[hidden]{display:none}
.quick-actions{border:1px solid #335272;background:#142538;border-radius:9px;padding:14px 16px;margin-top:18px}
.quick-actions strong{color:#cfebff;font-size:13px}
.quick-actions p{color:#b9cde1;font-size:11px;line-height:1.6}
.quick-row label{font-size:11px;color:#c3dbef}
.quick-form{display:flex;gap:8px;margin:7px 0}
.quick-form input{flex:1;min-width:0;background:#0c1826;border:1px solid #527594;color:#eff7ff;border-radius:6px;padding:10px;font:12px ui-monospace,monospace}
.quick-form input:disabled{opacity:.5}
.quick-hint{min-height:1.2em;margin-bottom:0}
@media(max-width:620px){.quick-form{flex-wrap:wrap}.quick-form button{width:100%}}
.git-delta{border-top:1px solid #315367;margin-top:14px;padding-top:13px}
.git-delta strong{font-size:12px;color:#cce7ff}
.git-delta p,.git-delta ul{font-size:11px;color:#c5d5e6;line-height:1.65}
.git-delta ul{padding-left:19px;overflow-wrap:anywhere}
.git-delta ul:empty{display:none}
@media(max-width:620px){.git-inspector{grid-template-columns:1fr}}
.task-run-history{margin-top:14px;padding:12px;background:#112634;border:1px solid #31586a;border-radius:7px}
.task-run-history[hidden]{display:none}
.task-run-history-head{display:flex;justify-content:space-between;align-items:center;gap:12px;flex-wrap:wrap}
.task-run-history-head strong{font-size:12px;color:#d5eafb}
.task-run-history ol{margin:10px 0 0;padding-left:22px;font-size:12px;color:#c8dded;line-height:1.65;overflow-wrap:anywhere}
.task-run-history li{padding:6px 0;border-bottom:1px solid #304c5b}
.task-run-history li:last-child{border-bottom:0}
.task-run-history pre{min-height:0;max-height:100px;padding:6px 0;font-size:11px}
`

const trustedGUIJavaScript = `'use strict';
(function(){
  let capability=location.hash.slice(1);
  // Keep the one-use localhost capability in JS memory, never in the
  // referrer, browser navigation, Portal URLs, cookies or persistent storage.
  history.replaceState(null,'',location.pathname);
  const activation=document.getElementById('activation');
  const localToken=document.getElementById('local-token');
  const activate=document.getElementById('activate');
  const features=document.getElementById('features');
  const featureSummary=document.getElementById('feature-summary');
  const ping=document.getElementById('ping');
  const git=document.getElementById('git');
  const gitLog=document.getElementById('git-log');
  const gitBranches=document.getElementById('git-branches');
  const gitRemotes=document.getElementById('git-remotes');
  const gitTrackingStart=document.getElementById('git-tracking-start');
  const gitTrackingNext=document.getElementById('git-tracking-next');
  const gitBranchCreate=document.getElementById('git-branch-create');
   const gitSwitchReview=document.getElementById('git-switch-review');
   const gitSwitchSameHead=document.getElementById('git-switch-same-head');
  const gitBranchDeleteReview=document.getElementById('git-branch-delete-review');
  const gitBranchDelete=document.getElementById('git-branch-delete');
  const gitDiff=document.getElementById('git-diff');
  const gitPatchStart=document.getElementById('git-patch-start');
   const gitPatchCachedStart=document.getElementById('git-patch-cached-start');
  const gitPatchNext=document.getElementById('git-patch-next');
  const gitPatchOutput=document.getElementById('git-patch-output');
  const gitFetch=document.getElementById('git-fetch');
  const gitStageTracked=document.getElementById('git-stage-tracked');
  const gitUnstageAll=document.getElementById('git-unstage-all');
  const gitPullReview=document.getElementById('git-pull-review');
   const gitPullChanges=document.getElementById('git-pull-changes');
  const gitPull=document.getElementById('git-pull');
  const gitCommit=document.getElementById('git-commit');
  const gitPush=document.getElementById('git-push');
  const gitCommitMessage=document.getElementById('git-commit-message');
  const terminalOpen=document.getElementById('terminal-open');
  const terminalPoll=document.getElementById('terminal-poll');
  const terminalAutoButton=document.getElementById('terminal-auto');
  const terminalInterrupt=document.getElementById('terminal-interrupt');
  const terminalEOF=document.getElementById('terminal-eof');
  const terminalClose=document.getElementById('terminal-close');
  const terminalSend=document.getElementById('terminal-send');
  const terminalInput=document.getElementById('terminal-input');
  const terminalOutput=document.getElementById('terminal-output');
  const terminalStateText=document.getElementById('terminal-state');
  const terminalResize=document.getElementById('terminal-resize');
  const terminalCols=document.getElementById('terminal-cols');
  const terminalRows=document.getElementById('terminal-rows');
  const project=document.getElementById('project');
  const tasksPreview=document.getElementById('tasks-preview');
  const tasksReport=document.getElementById('tasks-report');
  const workflowAuditButton=document.getElementById('tasks-workflow-audit');
  const workflowAuditNext=document.getElementById('tasks-workflow-audit-next');
  const tasksReportOutput=document.getElementById('tasks-report-output');
  const taskRunStop=document.getElementById('task-run-stop');
  const longTaskControls=document.getElementById('long-task-controls');
  const longTaskPoll=document.getElementById('long-task-poll');
  const longTaskCopy=document.getElementById('long-task-copy');
  const longTaskAutoButton=document.getElementById('long-task-auto');
  const longTaskCancel=document.getElementById('long-task-cancel');
  const longTaskClear=document.getElementById('long-task-clear');
  const longTaskOutput=document.getElementById('long-task-output');
  const taskRunHistoryView=document.getElementById('task-run-history');
  const taskRunHistoryList=document.getElementById('task-run-history-list');
  const taskRunHistoryClear=document.getElementById('task-run-history-clear');
  const taskPreviewSection=document.getElementById('task-preview');
  const taskTotal=document.getElementById('task-total');
  const taskLabels=document.getElementById('task-preview-labels');
  const taskDepsSelect=document.getElementById('task-deps-select');
  const taskDepsView=document.getElementById('task-deps-view');
  const taskWorkflowStart=document.getElementById('task-workflow-start');
  const taskDepsOutput=document.getElementById('task-deps-output');
  const taskFetchNext=document.getElementById('task-fetch-next');
  const taskFetchAll=document.getElementById('task-fetch-all');
  const taskStopAll=document.getElementById('task-stop-all');
  const taskLoadStatus=document.getElementById('task-load-status');
  const taskFilter=document.getElementById('task-filter');
  const taskSort=document.getElementById('task-sort');
  const taskCoverage=document.getElementById('task-coverage');
  const taskProgress=document.getElementById('task-progress');
  const taskNextHelp=document.getElementById('task-next-help');
  const taskFilterEmpty=document.getElementById('task-filter-empty');
  const taskCompareSummary=document.getElementById('task-compare-summary');
  const taskCompareList=document.getElementById('task-compare-list');
  taskSort.value='host';
  const taskPaging=document.getElementById('task-paging');
  const taskPrev=document.getElementById('task-prev');
  const taskNext=document.getElementById('task-next');
  const taskPageText=document.getElementById('task-page');
  const taskPageSize=4;
  let cachedTaskLabels=[];
  let cachedTaskIDs=[];
  // A Task.ID alone is not a valid event-handler lease. Rendering a new
  // row (including a refreshed host snapshot with identical IDs) retires
  // its previous callbacks. Appending an unchanged page preserves its DOM
  // and epoch to avoid breaking focused input fields.
  let taskPageRenderEpoch=0;
  let workflowAuditRows=[],workflowAuditMore=false;
  let taskDepsPendingID=0,taskWorkflowReadyID=0;
  // A verified host dependency snapshot supplies display labels only.
  // Once a workflow starts, freeze the <=4 step-to-Task.ID mapping:
  // refreshing/reordering the task list must not rename a running step.
  let preparedWorkflowSteps=null,activeWorkflowSteps=null;
  let inspectedTasks=new Map();
  // Transient, local-only drafts survive pagination, sorting and job polls.
  // Never retain them across a new host snapshot, re-inspection or logout.
  let taskInputDrafts=new Map();
  // Only locally decrypted task outcomes, never written to browser storage
  // or Relay. Bounded so repeated actions cannot inflate retained output.
  let taskRunHistory=[],taskHistorySerial=0,longTaskHistorySerial=0;
  // Reconcile by host-result serial so streaming Poll never detaches unchanged
  // history rows, text selections, clipboard buttons or their scroll state.
  let taskRunHistoryDOM=new Map(),taskRunHistoryOrder='';
  // Clipboard is a deliberately user-triggered local browser operation.
  // Never persist task output in Web Storage or send it to PHP/Relay.
  // If Firefox disallows clipboard writes, fail visibly and do not
  // fall back to a hidden textarea or an unsafe auto-copy mechanism.
  function copyVerifiedTaskOutput(value,button,stillPresent){
    if(closed||terminated||!verified||!value||!stillPresent())return;
    if(typeof navigator==='undefined'||!navigator.clipboard||
       typeof navigator.clipboard.writeText!=='function'){
      button.textContent='Clipboard không khả dụng';
      return;
    }
    try{
      Promise.resolve(navigator.clipboard.writeText(value)).then(()=>{
        if(!closed&&!terminated&&stillPresent()){
          button.textContent='Đã sao chép';
        }
      },()=>{
        if(!closed&&!terminated&&stillPresent()){
          button.textContent='Không thể sao chép';
        }
      });
    }catch(error){
      if(!closed&&!terminated&&stillPresent()){
        button.textContent='Không thể sao chép';
      }
    }
  }
  // An uncleared host job owns its live result row. The operator can
  // clear older results, but must not disconnect the row from the Poll
  // updater before the final authenticated status/output is received.
  function activeTaskHistorySerial(){
    return longTaskID>0&&longTaskHistorySerial>0?longTaskHistorySerial:0;
  }
  function renderTaskRunHistory(){
    taskRunHistoryView.hidden=taskRunHistory.length===0;
    const activeSerial=activeTaskHistorySerial();
    taskRunHistoryClear.disabled=closed||!taskRunHistory.some(
      item=>item.serial!==activeSerial);
    taskRunHistoryClear.textContent=activeSerial?
      'Xóa kết quả cũ':'Xóa kết quả';
    const serials=new Set();
    for(const item of taskRunHistory){
      serials.add(item.serial);
      let view=taskRunHistoryDOM.get(item.serial);
      if(!view){
        const li=document.createElement('li');
        const header=document.createElement('strong');
        const caption=document.createElement('span');
        header.append(caption);
        li.append(header);
        view={li,header,caption,copy:null,body:null,copyOutput:''};
        taskRunHistoryDOM.set(item.serial,view);
      }
      const heading='Task '+String(item.taskID)+
        (item.label?' · '+item.label:'')+' · '+
        item.kind+' · '+item.status+
        (item.exitCode>=0?' · exit='+String(item.exitCode):'')+
        (item.truncated?' · output đã cắt':'');
      if(view.caption.textContent!==heading)view.caption.textContent=heading;
      if(item.output&&!view.copy){
        const copy=document.createElement('button');
        copy.type='button';
        copy.textContent='Sao chép output';
        copy.setAttribute('aria-label','Sao chép output Task '+String(item.taskID));
        const serial=item.serial;
        copy.addEventListener('click',()=>{
          const current=taskRunHistory.find(x=>x.serial===serial);
          if(!current||!current.output)return;
          const snapshot=current.output;
          copyVerifiedTaskOutput(snapshot,copy,()=>
            taskRunHistory.some(x=>x.serial===serial&&x.output===snapshot));
        });
        view.header.append(copy);
        view.copy=copy;
      }
      if(view.copy){
        view.copy.disabled=closed||terminated||!verified||!item.output;
        view.copy.hidden=!item.output;
        if(view.copyOutput!==item.output){
          view.copy.textContent='Sao chép output';
          view.copyOutput=item.output;
        }
      }
      if(item.output&&!view.body){
        view.body=document.createElement('pre');
        view.li.append(view.body);
      }
      if(view.body){
        view.body.hidden=!item.output;
        if(view.body.textContent!==item.output)
          view.body.textContent=item.output;
      }
    }
    // Rearrange only when a result was added, deleted or actually moved.
    // Ordinary streaming chunks update existing text nodes in place.
    const order=taskRunHistory.map(item=>item.serial).join(',');
    if(order!==taskRunHistoryOrder){
      taskRunHistoryList.replaceChildren(...taskRunHistory.map(
        item=>taskRunHistoryDOM.get(item.serial).li));
      taskRunHistoryOrder=order;
    }
    for(const serial of taskRunHistoryDOM.keys()){
      if(!serials.has(serial))taskRunHistoryDOM.delete(serial);
    }
  }
  function rememberTaskRun(taskID,kind,status,exitCode,outputText,truncated){
    const serial=++taskHistorySerial;
    // Capture the already verified host label now; a future snapshot can
    // change or remove the task without changing this historical result.
    const labelIndex=cachedTaskIDs.indexOf(taskID);
    const label=labelIndex<0?'':cachedTaskLabels[labelIndex];
    taskRunHistory.unshift({serial,taskID,label,kind,status,exitCode,
      output:outputText,truncated});
    if(taskRunHistory.length>6)taskRunHistory.length=6;
    renderTaskRunHistory();
    return serial;
  }
  // Keep at most maxBytes of the *decrypted* UTF-8 transcript in the
  // history preview. JS .slice(-N) counts UTF-16 code units and can
  // bisect a surrogate pair (emoji), producing a value that cannot be
  // encoded back to UTF-8 or copied faithfully. Walk whole code points
  // backwards; never synthesize replacement characters or split a rune.

  // Follow the last output chunk only if the reader is already near the
  // bottom; never steal the scroll position from someone reading older log.
  function renderLongTaskTranscript(value,reset=false){
    const node=longTaskOutput;
    const top=node.scrollTop,height=node.scrollHeight,viewport=node.clientHeight;
    const measurable=Number.isFinite(top)&&Number.isFinite(height)&&
      Number.isFinite(viewport)&&height>=0&&viewport>=0;
    const follow=reset||(measurable&&
      (height<=viewport||height-top-viewport<=28));
    node.textContent=value;
    if(reset)node.scrollTop=0;
    else if(follow)node.scrollTop=node.scrollHeight;
  }
  function boundedUTF8HistoryTail(value,maxBytes){
    let start=value.length,bytes=0;
    while(start>0){
      const last=value.charCodeAt(start-1);
      let cp=last,units=1;
      if(last>=0xdc00&&last<=0xdfff&&start>1){
        const high=value.charCodeAt(start-2);
        if(high>=0xd800&&high<=0xdbff){
          cp=0x10000+((high-0xd800)<<10)+(last-0xdc00);
          units=2;
        }
      }
      // Every caller provides E2EE-validated UTF-8. A lone surrogate
      // cannot be re-encoded safely: discard it instead of returning a
      // broken clipboard string if an internal caller violates that gate.
      if(cp>=0xd800&&cp<=0xdfff){
        start-=units;
        break;
      }
      const width=cp<=0x7f?1:cp<=0x7ff?2:cp<=0xffff?3:4;
      if(bytes+width>maxBytes)break;
      bytes+=width;
      start-=units;
    }
    return {text:value.slice(start),clipped:start>0};
  }
  function updateLongTaskHistory(taskID,status,exitCode,truncated,receivedOutput){
    const item=taskRunHistory.find(entry=>entry.serial===longTaskHistorySerial&&
      entry.taskID===taskID&&entry.kind==='dài');
    if(!item)return;
    item.status=status;
    item.exitCode=exitCode;
    // Keep a bounded, local-only tail of output for the history view. The
    // full 16 KiB transcript remains in the active job panel until cleared.
    // No task output is persisted or returned to PHP/Relay as plaintext.
    const tail=boundedUTF8HistoryTail(receivedOutput,2048);
    item.output=tail.text;
    item.truncated=item.truncated||truncated||tail.clipped;
    renderTaskRunHistory();
  }
  let inspectButtons=[];
  // Never report removals from a partly downloaded authenticated snapshot.
  let lastCompleteTaskLabels=null;
  // The bulk loader is explicitly user-started, bounded by the existing
  // 16-round native quota and sequentially awaits each sealed E2EE reply.
  let bulkLoading=false,bulkStopRequested=false;
  let remoteTaskTotal=0;
  let canFetchTaskNext=false;
  let taskPage=0;
  function compareCompletedTasks(){
    taskCompareList.replaceChildren();
    if(cachedTaskLabels.length!==remoteTaskTotal){
      taskCompareSummary.textContent=lastCompleteTaskLabels===null?
        'Chưa tải đủ danh sách task để tạo mốc so sánh.':
        'Bản đọc mới chưa đủ '+String(remoteTaskTotal)+' nhãn. Không suy đoán task bị xóa khi còn trang chưa tải.';
      return;
    }
    if(lastCompleteTaskLabels===null){
      taskCompareSummary.textContent='Đã tải đủ '+String(remoteTaskTotal)+
        ' nhãn lần đầu. Đọc lại toàn bộ để kiểm tra khác biệt.';
    }else{
      const tally=labels=>{
        const result=new Map();
        for(const label of labels)result.set(label,(result.get(label)||0)+1);
        return result;
      };
      const old=tally(lastCompleteTaskLabels);
      const current=tally(cachedTaskLabels);
      let added=0,removed=0,shown=0;
      function record(message){
        if(shown>=20)return;
        const item=document.createElement('li');
        item.textContent=message;
        taskCompareList.append(item);
        shown++;
      }
      for(const [label,count] of current){
        const extra=count-(old.get(label)||0);
        if(extra>0){added+=extra;record('Mới xuất hiện: '+label+' ('+String(extra)+')');}
      }
      for(const [label,count] of old){
        const missing=count-(current.get(label)||0);
        if(missing>0){removed+=missing;record('Không còn xuất hiện: '+label+' ('+String(missing)+')');}
      }
      taskCompareSummary.textContent='Giữa hai danh sách đầy đủ: '+
        String(added)+' nhãn mới, '+String(removed)+
        ' nhãn không còn. Chỉ so sánh tên hiển thị, không phải ID hay nội dung task.'+
        (shown>=20?' Hiển thị tối đa 20 dòng.':'');
    }
    lastCompleteTaskLabels=cachedTaskLabels.slice();
  }
  function filteredTaskLabels(){
    const needle=String(taskFilter.value||'').trim().toLocaleLowerCase();
    const entries=cachedTaskLabels.map((label,i)=>({label,id:cachedTaskIDs[i]||0}));
    const result=needle?entries.filter(item=>item.label.toLocaleLowerCase().includes(needle)):entries;
    if(taskSort.value==='az'){
      result.sort((a,b)=>a.label.localeCompare(b.label,'vi'));
    }else if(taskSort.value==='za'){
      result.sort((a,b)=>b.label.localeCompare(a.label,'vi'));
    }
    return result;
  }
  function renderTaskCoverage(){
    const loaded=cachedTaskLabels.length;
    taskCoverage.textContent='Đã tải '+String(loaded)+' / '+
      String(remoteTaskTotal)+' nhãn task';
    taskProgress.max=Math.max(1,remoteTaskTotal);
    taskProgress.value=loaded;
    if(!canFetchTaskNext){
      taskNextHelp.textContent='Đã tải hết các nhãn task hợp lệ của workspace.';
    }else if(budget<=0){
      taskNextHelp.textContent='Đã hết 16 lượt E2EE. Mở phiên mới và xác thực lại trong terminal native TaskDeck để tiếp tục.';
    }else if(longTaskID!==0&&budget<=3){
      taskNextHelp.textContent='Giữ lượt E2EE còn lại để hủy, nhận trạng thái và dọn job hiện tại.';
    }else{
      taskNextHelp.textContent='Còn '+String(remoteTaskTotal-loaded)+
        ' nhãn chưa tải. Tải thêm từ host sẽ dùng 1 trong '+String(budget)+' lượt E2EE còn lại.';
    }
  }
  // A successful fast-forward may change .vscode/tasks.json and every
  // Task.ID authorization. Discard the old browser snapshot even if the
  // host task list was previously inspected. The next Tasks Preview
  // obtains a new host-issued page digest. Historical output is separate.
  function invalidateTaskPreviewAfterPull(){
    taskWorkflowReadyID=0;taskDepsPendingID=0;
    preparedWorkflowSteps=null;
    taskDepsSelect.replaceChildren();taskDepsSelect.value='';
    taskDepsOutput.textContent='';taskDepsOutput.hidden=true;
    taskPreviewSection.hidden=true;
    taskLabels.replaceChildren();inspectButtons=[];
    taskPageRenderEpoch++;
    taskPaging.hidden=true;
    cachedTaskIDs=[];cachedTaskLabels=[];
    inspectedTasks.clear();taskInputDrafts.clear();
    remoteTaskTotal=0;canFetchTaskNext=false;taskPage=0;
    lastCompleteTaskLabels=null;
    taskFilter.value='';taskSort.value='host';
    taskTotal.textContent='';
    taskCompareSummary.textContent='';
    taskCompareList.replaceChildren();
    taskLoadStatus.textContent='Git pull đã thay đổi HEAD. Hãy xem trước task để xác minh lại.';
  }
  function renderTaskPage(){
    const rowEpoch=++taskPageRenderEpoch;
    const labels=filteredTaskLabels();
    const count=Math.max(1,Math.ceil(labels.length/taskPageSize));
    taskPage=Math.max(0,Math.min(taskPage,count-1));
    taskLabels.replaceChildren();
    inspectButtons=[];
    for(const item of labels.slice(taskPage*taskPageSize,(taskPage+1)*taskPageSize)){
      const li=document.createElement('li');
      li.textContent=item.label;
      if(item.id>0){
        const note=document.createElement('small');
        note.textContent=' · ID ổn định: '+String(item.id)+' ';
        li.append(note);
        const verify=document.createElement('button');
        verify.type='button';
        verify.textContent='Kiểm tra ID';
        verify.disabled=closed||busy||!verified||budget<=0||
          !featuresKnown||!available.tasks_preview;
        verify.addEventListener('click',()=>{
          // A detached button from an obsolete host-issued task snapshot
          // is never permission to inspect that ID after Git Pull, a new
          // Tasks Preview, or logout.
          if(closed||busy||bulkLoading||budget<=0||
            rowEpoch!==taskPageRenderEpoch||
            taskPreviewSection.hidden||!cachedTaskIDs.includes(item.id)||
            !featuresKnown||!available.tasks_preview)return;
          invoke('/api/tasks-inspect/'+String(item.id));
        });
        li.append(verify);
        inspectButtons.push(verify);
        if(featuresKnown&&available.task_run){
          const selection=inspectedTasks.get(item.id);
          const draft=taskInputDrafts.get(item.id)||{};
          const saveDraft=(key,value)=>{
            const next=taskInputDrafts.get(item.id)||{};
            next[key]=value;
            taskInputDrafts.set(item.id,next);
          };
          let choiceSelect=null;
          let choiceSelect2=null;
          let textInput=null;
          if(selection&&selection.inputType==='promptString'){
            const caption=document.createElement('small');
            caption.textContent=' · Tham số '+selection.inputID+' (mã ngắn): ';
            li.append(caption);
            textInput=document.createElement('input');
            textInput.type='text';
            textInput.maxLength=64;
            textInput.autocomplete='off';
            textInput.spellcheck=false;
            textInput.value=typeof draft.text==='string'?draft.text:'';
            textInput.addEventListener('input',()=>saveDraft('text',textInput.value));
            textInput.setAttribute('aria-label','Nhập tham số '+selection.inputID);
            textInput.disabled=busy||closed||budget<=0;
            li.append(textInput);
            inspectButtons.push(textInput);
          }
          if(selection&&selection.choices.length>0){
            const caption=document.createElement('small');
            caption.textContent=' · Chọn '+selection.inputID+': ';
            li.append(caption);
            choiceSelect=document.createElement('select');
            choiceSelect.setAttribute('aria-label','Chọn '+selection.inputID);
            for(let index=0;index<selection.choices.length;index++){
              const choiceOption=document.createElement('option');
              choiceOption.value=String(index+1);
              choiceOption.textContent=selection.choices[index];
              choiceSelect.append(choiceOption);
            }
            choiceSelect.value=Number.isInteger(draft.choice)&&draft.choice>=1&&
              draft.choice<=selection.choices.length?String(draft.choice):'1';
            choiceSelect.addEventListener('change',()=>saveDraft('choice',Number(choiceSelect.value)));
            choiceSelect.disabled=busy||closed||budget<=0;
            li.append(choiceSelect);
            inspectButtons.push(choiceSelect);
          }
          if(selection&&selection.choices2.length>0){
            const caption2=document.createElement('small');
            caption2.textContent=' · Chọn '+selection.inputID2+': ';
            li.append(caption2);
            choiceSelect2=document.createElement('select');
            choiceSelect2.setAttribute('aria-label','Chọn '+selection.inputID2);
            for(let index=0;index<selection.choices2.length;index++){
              const option2=document.createElement('option');
              option2.value=String(index+1);
              option2.textContent=selection.choices2[index];
              choiceSelect2.append(option2);
            }
            choiceSelect2.value=Number.isInteger(draft.choice2)&&draft.choice2>=1&&
              draft.choice2<=selection.choices2.length?String(draft.choice2):'1';
            choiceSelect2.addEventListener('change',()=>saveDraft('choice2',Number(choiceSelect2.value)));
            choiceSelect2.disabled=busy||closed||budget<=0;
            li.append(choiceSelect2);
            inspectButtons.push(choiceSelect2);
          }
          const run=document.createElement('button');
          run.type='button';
          run.textContent=selection?'Chạy task':'Kiểm tra ID trước khi chạy';
          run.taskRun=true;
          run.taskID=item.id;
          run.disabled=!selection||closed||busy||bulkLoading||budget<=0||
            longTaskID!==0||terminalState!=='closed';
          const launch=(isLong)=>{
            // A native row's old event handler can outlive its DOM node.
            // Inspect permission is snapshot-scoped: the SAME task ID in
            // another host preview still requires a fresh inspect.
            if(!selection||closed||busy||bulkLoading||budget<=0||
              rowEpoch!==taskPageRenderEpoch||
              taskPreviewSection.hidden||!cachedTaskIDs.includes(item.id)||
              inspectedTasks.get(item.id)!==selection||
              !featuresKnown||!available.task_run||terminalState!=='closed'||
              longTaskID!==0||(isLong&&!available.task_long))return;
            const choice=choiceSelect?Number(choiceSelect.value):0;
            if(choiceSelect&&(!Number.isInteger(choice)||choice<1||
              choice>selection.choices.length))return;
            let selectedChoice=choice;
            if(choiceSelect2){
              const choice2=Number(choiceSelect2.value);
              if(!Number.isInteger(choice2)||choice2<1||
                choice2>selection.choices2.length)return;
              selectedChoice=(choice-1)*selection.choices2.length+choice2;
            }
            const prefix=isLong?'/api/tasks-start/':'/api/tasks-run/';
            let endpoint=prefix+String(item.id)+
              (selectedChoice?'/'+String(selectedChoice):'');
            let payload;
            if(textInput){
              const value=String(textInput.value||'');
              if(!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$/.test(value)){
                output.textContent='Tham số phải là mã 1–64 ký tự: chữ, số, _, - hoặc .; bắt đầu bằng chữ hoặc số.';
                return;
              }
              endpoint=isLong?'/api/tasks-start-input/'+String(item.id):
                '/api/tasks-run-input/'+String(item.id);
              payload={value};
            }
            const chosen=choiceSelect?
              '\nLựa chọn '+selection.inputID+': '+selection.choices[choice-1]:'';
            const chosen2=choiceSelect2?
              '\nLựa chọn '+selection.inputID2+': '+
                selection.choices2[Number(choiceSelect2.value)-1]:'';
            // Task execution can have side effects on the host even if its
            // definition is approved. Never execute on a single stray click.
            // Do not echo a prompt token in the confirmation or audit.
            if(!window.confirm('Xác nhận chạy '+
              (isLong?'task dài (tối đa 90 giây)':'task ngắn (tối đa 6 giây)')+
              ': '+item.label+' (ID '+String(item.id)+')?'+chosen+chosen2+
              (textInput?'\nĐã nhập mã tham số tại client.':'')+
              '\nTask này có thể thay đổi dữ liệu trên máy host.'))return;
            if(textInput){
              // Only the already-confirmed POST body retains the bounded
              // token until dispatch. Never keep a second copy in a focused
              // input or a draft after it has been submitted. A canceled
              // confirmation and a locally invalid token remain editable.
              textInput.value='';
              const draft=taskInputDrafts.get(item.id);
              if(draft){
                delete draft.text;
                if(Object.keys(draft).length===0)taskInputDrafts.delete(item.id);
              }
            }
            invoke(endpoint,payload);
          };
          run.addEventListener('click',()=>launch(false));
          li.append(run);
          inspectButtons.push(run);
          if(available.task_long){
            const runLong=document.createElement('button');
            runLong.type='button';
            runLong.textContent='Chạy task dài';
            runLong.taskRun=true;
            runLong.taskLong=true;
            runLong.taskID=item.id;
            runLong.disabled=!selection||closed||busy||budget<=0||
              longTaskID!==0||terminalState!=='closed';
            runLong.addEventListener('click',()=>launch(true));
            li.append(runLong);
            inspectButtons.push(runLong);
          }
        }
      }
      taskLabels.append(li);
    }
    updateTaskPageChrome(labels);
  }
  // A newly fetched host page can extend the available page count without
  // changing the visible entries. Update pagination/coverage independently
  // so focused task inputs and buttons keep their DOM identity.
  function updateTaskPageChrome(labels){
    const count=Math.max(1,Math.ceil(labels.length/taskPageSize));
    taskPaging.hidden=labels.length<=taskPageSize;
    taskFilterEmpty.hidden=labels.length!==0;
    taskFilterEmpty.textContent=remoteTaskTotal===0?
      'Workspace hiện chưa có task có nhãn hợp lệ.':
      'Không tìm thấy nhãn phù hợp trong các task đã tải.';
    renderTaskCoverage();
    taskPageText.textContent='Trang '+String(taskPage+1)+' / '+String(count)+
      (taskFilter.value?' · '+String(labels.length)+' kết quả lọc':'');
    taskPrev.disabled=taskPage===0;
    taskNext.disabled=taskPage+1>=count;
    taskFetchNext.hidden=!canFetchTaskNext;
    taskFetchNext.disabled=busy||bulkLoading||closed||!verified||budget<=0||
      (longTaskID!==0&&budget<=3)||!featuresKnown||!available.tasks_preview;
    taskFetchAll.hidden=!canFetchTaskNext||bulkLoading;
    taskFetchAll.disabled=busy||closed||!verified||budget<=0||
      (longTaskID!==0&&budget<=3)||
      !featuresKnown||!available.tasks_preview;
    taskStopAll.hidden=!bulkLoading;
    taskStopAll.disabled=closed;
  }
  taskFilter.addEventListener('input',()=>{
    if(closed)return;
    taskPage=0;
    renderTaskPage();
  });
  taskSort.addEventListener('change',()=>{
    if(closed)return;
    taskPage=0;
    renderTaskPage();
  });
  taskPrev.addEventListener('click',()=>{
    if(closed||taskPage===0)return;
    taskPage--;renderTaskPage();
  });
  taskNext.addEventListener('click',()=>{
    if(closed||(taskPage+1)*taskPageSize>=filteredTaskLabels().length)return;
    taskPage++;renderTaskPage();
  });
  const filesRootButton=document.getElementById('files-root');
  const filesRootNext=document.getElementById('files-root-next');
  const filesRootOutput=document.getElementById('files-root-output');
  const filesSelect=document.getElementById('files-select');
  const filesDirSelect=document.getElementById('files-dir-select');
  const filesDirOpen=document.getElementById('files-dir-open');
  const filesDirNext=document.getElementById('files-dir-next');
  const filesDirOutput=document.getElementById('files-dir-output');
  const filesReadSelected=document.getElementById('files-read-selected');
  const filesReadStart=document.getElementById('files-read-start');
  const filesReadNext=document.getElementById('files-read-next');
  const filesReadOutput=document.getElementById('files-read-output');
  const projectOverview=document.getElementById('project-overview');
  const projectName=document.getElementById('project-name');
  const workspaceName=document.getElementById('workspace-name');
  const hostOS=document.getElementById('host-os');
  const disconnect=document.getElementById('disconnect');
  const state=document.getElementById('state');
  const output=document.getElementById('output');
  const quickCommand=document.getElementById('quick-command');
  const quickRun=document.getElementById('quick-run');
  const quickHint=document.getElementById('quick-hint');
  const remaining=document.getElementById('remaining');
  const targetDevice=document.getElementById('target-device');
  const targetUser=document.getElementById('target-user');
  const sessionClock=document.getElementById('session-clock');
  const pingLatency=document.getElementById('ping-latency');
  const sessionActivity=document.getElementById('session-activity');
  const gitSummary=document.getElementById('git-summary');
  const gitBranch=document.getElementById('git-branch');
  const gitFiles=document.getElementById('git-files');
  const gitFileSelect=document.getElementById('git-file-select');
  const gitStageFile=document.getElementById('git-stage-file');
  const gitUnstageFile=document.getElementById('git-unstage-file');
  const gitFilter=document.getElementById('git-filter');
  const gitKind=document.getElementById('git-kind');
  const gitCount=document.getElementById('git-count');
  const gitEmpty=document.getElementById('git-empty');
  const gitDeltaSummary=document.getElementById('git-delta-summary');
  const gitDeltaList=document.getElementById('git-delta-list');
  const viewTitle=document.getElementById('view-title');
  const viewHint=document.getElementById('view-hint');
  const navButtons={
    overview:document.getElementById('nav-overview'),
    project:document.getElementById('nav-project'),
    tasks:document.getElementById('nav-tasks'),
    git:document.getElementById('nav-git'),
    terminal:document.getElementById('nav-terminal'),
    files:document.getElementById('nav-files')
  };
  const panels={
    overview:document.getElementById('panel-overview'),
    project:document.getElementById('panel-project'),
    tasks:document.getElementById('panel-tasks'),
    git:document.getElementById('panel-git'),
    terminal:document.getElementById('panel-terminal'),
    files:document.getElementById('panel-files')
  };
  const viewDescriptions={
    overview:['Tổng quan','Quản lý phiên và kiểm tra chức năng host. Chuyển tab không tốn lượt E2EE.'],
    project:['Dự án TaskDeck','Dữ liệu hiển thị được xác thực, chỉ thuộc một dự án TaskDeck đã cấp quyền.'],
    tasks:['Task của workspace','Chỉ xem nhãn task, không gửi ID, đường dẫn hoặc lệnh thực thi.'],
    git:['Git Status','Dữ liệu Git và thao tác pull, commit, push đều phải được host cho phép.'],
    terminal:['Terminal PTY','Shell /bin/sh giới hạn 90 giây, một phiên trên mỗi kết nối E2EE, Linux/macOS.'],
    files:['Files · chỉ đọc','Danh sách tên mục gốc do Agent cấp quyền; chưa có thao tác đọc nội dung hoặc sửa file.']
  };
  function showView(next){
    if(!Object.prototype.hasOwnProperty.call(panels,next))return;
    for(const key of Object.keys(panels)){
      const selected=key===next;
      panels[key].hidden=!selected;
      navButtons[key].classList.toggle('active',selected);
      navButtons[key].setAttribute('aria-current',selected?'page':'false');
    }
    viewTitle.textContent=viewDescriptions[next][0];
    viewHint.textContent=viewDescriptions[next][1];
  }
  for(const key of Object.keys(navButtons)){
    navButtons[key].addEventListener('click',()=>showView(key));
  }
  // Snapshot-only local inspector. Search and classification never use E2EE
  // budget, connect to PHP, or grant new host/Git privileges.
  let gitRows=[];
  let gitRawSnapshot='';
  let gitSnapshotCount=0,gitSnapshotBranch='';
  function renderGitRows(){
    const needle=String(gitFilter.value||'').trim().toLocaleLowerCase();
    const selected=gitKind.value;
    const visible=gitRows.filter(item=>{
      if(needle&&!item.path.toLocaleLowerCase().includes(needle))return false;
      if(selected==='all')return true;
      if(item.conflict)return selected==='conflict';
      if(selected==='untracked')return item.code==='??';
      if(selected==='staged')return item.code[0]!==' '&&item.code[0]!=='?';
      if(selected==='unstaged')return item.code[1]!==' '&&item.code[1]!=='?';
      return false;
    });
    gitFiles.replaceChildren();
    for(const item of visible){
      const row=document.createElement('tr');
      const kind=document.createElement('td');
      const file=document.createElement('td');
      kind.textContent=item.code;
      file.textContent=item.path;
      row.append(kind,file);
      gitFiles.append(row);
    }
    gitCount.textContent='Hiển thị '+String(visible.length)+' / '+
      String(gitRows.length)+' thay đổi từ ảnh chụp E2EE.';
    gitEmpty.hidden=visible.length!==0;
    gitEmpty.textContent=gitRows.length===0?
      'Không có thay đổi ở các file đã theo dõi trong ảnh chụp này.':
      'Không có thay đổi phù hợp với bộ lọc cục bộ.';
  }
  function renderGit(raw){
    // All status reads are host-authorized AEAD results. Delta is local only.
    if(typeof raw!=='string'||raw.length===0||raw.length>256||
      /[\x00-\x08\x0b-\x1f\x7f]/.test(raw))throw new Error('Git Status E2EE không hợp lệ');
    const lines=raw.split('\n').filter(Boolean);
    let rows=[],branch='',heading='';
    if(raw==='clean'){
      branch='clean';
      heading='Không có thay đổi ở các file đã theo dõi';
    }else{
      if(lines.length===0||!lines[0].startsWith('## ')){
        throw new Error('Git Status E2EE thiếu nhánh đã xác thực');
      }
      branch=lines[0];heading=branch;
      const seen=new Set();
      for(const line of lines.slice(1)){
        const code=line.slice(0,2),path=line.slice(3);
        if(line.length<4||line[2]!==' '||
          !/^[ MARCTUD?!]{2}$/.test(code)||code==='  '||
          !path.trim()||seen.has(path))throw new Error('Dòng Git Status E2EE không hợp lệ');
        seen.add(path);
        rows.push({code,path,
          conflict:['DD','AU','UD','UA','DU','AA','UU'].includes(code)});
      }
    }
    const oldRows=gitRows,priorBranch=gitSnapshotBranch;
    gitDeltaList.replaceChildren();
    if(gitSnapshotCount===0){
      gitDeltaSummary.textContent='Ảnh chụp đầu tiên. Đọc lại Git Status để so sánh.';
    }else if(priorBranch!==branch){
      gitDeltaSummary.textContent='Nhánh hoặc tracking đã thay đổi. Ảnh chụp mới trở thành mốc so sánh.';
    }else{
      const old=new Map(oldRows.map(item=>[item.path,item.code]));
      const current=new Map(rows.map(item=>[item.path,item.code]));
      let added=0,changed=0,missing=0;
      function record(message){
        const node=document.createElement('li');
        node.textContent=message;
        gitDeltaList.append(node);
      }
      for(const item of rows){
        if(!old.has(item.path)){
          added++;
          record('Mới xuất hiện: '+item.path+' ['+item.code+']');
        }else if(old.get(item.path)!==item.code){
          changed++;
          record('Đổi trạng thái: '+item.path+' ['+old.get(item.path)+' → '+item.code+']');
        }
      }
      for(const item of oldRows){
        if(!current.has(item.path)){
          missing++;
          record('Không còn trong ảnh chụp: '+item.path);
        }
      }
      gitDeltaSummary.textContent='So với lần đọc trước: '+
        String(added)+' mục mới, '+String(changed)+' đổi trạng thái, '+
        String(missing)+' không còn xuất hiện.';
    }
    gitSnapshotCount++;
    gitSnapshotBranch=branch;
    gitBranch.textContent=heading;
    gitRows=rows;
    gitRawSnapshot=raw;
    gitFileSelect.replaceChildren();
    const placeholder=document.createElement('option');
    placeholder.value='';placeholder.textContent='Chọn file đã theo dõi';
    gitFileSelect.append(placeholder);
    for(const [i,item] of rows.entries()){
      if(i>=12)break;
      const path=item.path;
      if(!/^[A-Za-z0-9_.-]+(\/[A-Za-z0-9_.-]+)*$/.test(path)||
        path.split('/').some(p=>p==='.'||p==='..')||path.startsWith('.git/'))continue;
      if(!['M','D'].includes(item.code[1])&&
        !['M','A','D'].includes(item.code[0]))continue;
      const option=document.createElement('option');
      option.value=String(i+1);
      option.textContent=path+' ['+item.code+']';
      gitFileSelect.append(option);
    }
    gitFileSelect.value='';
    gitFileSelect.disabled=rows.length===0;
    gitFilter.value='';
    gitKind.value='all';
    gitFilter.disabled=false;
    gitKind.disabled=false;
    renderGitRows();
    gitSummary.hidden=false;
  }
  gitFilter.addEventListener('input',()=>{if(!closed)renderGitRows();});
  gitKind.addEventListener('change',()=>{if(!closed)renderGitRows();});
  gitFileSelect.addEventListener('change',()=>{if(!closed)render();});
  let busy=false,closed=false,terminated=false,closing=false,budget=16;
  let taskRunning=false; // Short tasks: stop revokes E2EE.
  let longTaskID=0,longTaskState='',longTaskText='',longTaskMore=false;
  // Cancel acknowledgement is distinct from process exit. Poll may still
  // report "running" while the host is terminating the process group.
  let longTaskCancelPending=false;
  let longTaskAuto=false,longTaskPollScheduled=false,longTaskPollEpoch=0;
  // A task ID identifies a configured task, NOT one process execution.
  // Timers belonging to a cleared execution may never poll its successor
  // when the operator restarts that same task ID in this native session.
  let terminalState='closed',terminalTranscript='';
  // Automatic Terminal output is opt-in and event-driven, not a
  // quota-burning idle poll loop. A queued timer belongs to one epoch;
  // close, manual poll and pause invalidate all older callbacks.
  let terminalAutoEnabled=false,terminalAutoPending=false;
  let terminalAutoScheduled=false,terminalAutoEpoch=0;
  let gitPatchText='',gitPatchPage=0,gitPatchMore=false,gitPatchMode='head';
  let gitSwitchPreview=null,gitSwitchPending=null;
  let gitDeletePreview=null,gitDeletePending=null;
  let filesRootNames=[],filesRootMore=false;
  let filesDirNames=[],filesDirMore=false,filesDirChosen='';
  let filesReadName='',filesReadText='',filesReadPage=0,filesReadMore=false;
  let filesPendingSelectedName='';
  let gitTrackingText='',gitTrackingPage=0,gitTrackingMore=false,gitTrackingLast='';
  let actionEpoch=0;
  // Only the newest initiated localhost status poll may alter the GUI.
  let sessionStatusEpoch=0;
  let featuresKnown=false;
  let available={session_ping:false,project_info:false,files_root:false,tasks_preview:false,
    task_run:false,task_long:false,git_status:false,git_log:false,git_branches:false,git_branch_create:false,git_diff:false,git_patch:false,git_fetch:false,git_stage_tracked:false,git_unstage_all:false,git_stage_file:false,git_unstage_file:false,git_pull:false,git_commit:false,git_push:false,terminal:false};
  let expiresAt=0,expiredByClock=false;
  // Native quota guard: retain Cancel -> Poll -> Clear when a host job
  // owns the session. The Agent also independently authorizes every action.
  function terminalMaintenanceAllowed(endpoint){
    // The host has ONE PTY per authenticated session. The GUI cannot
    // dispatch a duplicate Open, a Poll/Close before Open, or a Write/
    // Resize after exit even through a synthetic click on a disabled
    // button. Preserve the last E2EE round for explicit PTY Close.
    if(endpoint==='/api/terminal-open')
      return terminalState==='closed'&&longTaskID===0&&budget>=2;
    if(endpoint==='/api/terminal-close')
      return terminalState!=='closed';
    if(endpoint==='/api/terminal-poll')
      return terminalState!=='closed'&&budget>1;
    if(endpoint==='/api/terminal-write'||endpoint==='/api/terminal-resize')
      return terminalState==='running'&&budget>1;
    return terminalState==='closed'||budget>1||endpoint==='/api/close';
  }
  // Git alone may resume after the host has confirmed a terminal task
  // status and all output has been received. Keep the LAST authenticated
  // round for explicit Clear. Running jobs still reserve Cancel/Poll/Clear.
  // The exact same immutable endpoint list is used by invoke's Git gate.
  const remoteGitMutations=Object.freeze([
    '/api/git-branch-create','/api/git-switch-same-head','/api/git-branch-delete',
    '/api/git-stage-tracked','/api/git-unstage-all',
    '/api/git-stage-file','/api/git-unstage-file','/api/git-fetch',
    '/api/git-pull','/api/git-commit','/api/git-push'
  ]);
  function taskMaintenanceAllowed(endpoint){
    if(longTaskID===0||budget>3||endpoint==='/api/close')return true;
    if(remoteGitMutations.includes(endpoint)&&budget>1&&!longTaskMore&&
       ['completed','failed','cancelled','timed_out'].includes(longTaskState)){
      return true;
    }
    const id=String(longTaskID);
    if(endpoint==='/api/tasks-cancel/'+id)
      // At least three rounds: Cancel ACK, final status Poll, and Clear.
      return budget>=3&&!longTaskCancelPending&&
        ['started','running'].includes(longTaskState);
    if(endpoint==='/api/tasks-clear/'+id)
      return ['completed','failed','cancelled','timed_out'].includes(longTaskState);
    if(endpoint==='/api/tasks-poll/'+id)
      // Once Cancel was acknowledged, an interim "running" response
      // does not mean a NEW cancel round is needed. Keep one round for
      // the final Clear and allow the remaining round to observe exit.
      return (['started','running'].includes(longTaskState)&&
        !longTaskCancelPending?budget>3:budget>1)&&
        (['started','running','cancel_requested'].includes(longTaskState)||longTaskMore);
    return false;
  }
  function clockText(ms){
    if(ms<=0)return 'Hết hạn';
    const seconds=Math.ceil(ms/1000);
    return String(Math.floor(seconds/60)).padStart(2,'0')+':'+String(seconds%60).padStart(2,'0');
  }
  function updateSessionClock(){
    if(!verified||terminated||closed){sessionClock.textContent='Phiên đã đóng';return;}
    if(expiresAt===0){sessionClock.textContent='Chưa xác định';return;}
    const left=expiresAt-Date.now();
    sessionClock.textContent=clockText(left);
    if(left<=0){
      // The browser's countdown is only a best-effort early revoke.
      // The native Go listener independently closes E2EE at its
      // absolute deadline even when Firefox is asleep or suspended.
      expiredByClock=true;
      output.textContent='Phiên hết hạn. Đang thu hồi kết nối E2EE tại native client.';
      invoke('/api/close');
    }
  }
  let verified=/^[a-f0-9]{64}$/.test(capability);
  // One trusted native session per page. A previously used local
  // activation code must never re-arm a closed/revoked native GUI.
  let activationConsumed=verified;
  // Authenticated data must not remain displayed after logout, expiration
  // or revocation. Do not persist previous remote metadata in the browser.
  function clearPrivateViews(){
    taskWorkflowReadyID=0;taskDepsPendingID=0;
    preparedWorkflowSteps=null;activeWorkflowSteps=null;
    taskDepsSelect.replaceChildren();taskDepsSelect.value='';
    taskDepsOutput.textContent='';taskDepsOutput.hidden=true;
    tasksReportOutput.textContent='';tasksReportOutput.hidden=true;
    workflowAuditRows=[];workflowAuditMore=false;
    gitSwitchPreview=null;gitSwitchPending=null;
    gitDeletePreview=null;gitDeletePending=null;
    longTaskID=0;longTaskState='';longTaskText='';
    longTaskCancelPending=false;
    longTaskHistorySerial=0;
    taskRunHistory=[];taskRunHistoryList.replaceChildren();
    taskRunHistoryDOM.clear();taskRunHistoryOrder='';
    taskRunHistoryView.hidden=true;taskRunHistoryClear.disabled=true;
    longTaskMore=false;longTaskAuto=false;
    longTaskPollEpoch++;longTaskPollScheduled=false;
    terminalState='closed';terminalTranscript='';
    terminalAutoEnabled=false;terminalAutoPending=false;
    terminalAutoEpoch++;terminalAutoScheduled=false;
    gitPatchText='';gitPatchPage=0;gitPatchMore=false;
    gitPatchOutput.textContent='';gitPatchOutput.hidden=true;
    terminalInput.value='';terminalOutput.textContent='';
    terminalStateText.textContent='Phiên đã đóng.';
    longTaskControls.hidden=true;
    longTaskOutput.hidden=true;longTaskOutput.textContent='';
    taskRunning=false;
    taskRunStop.hidden=true;
    taskRunStop.disabled=true;
    quickCommand.value='';
    quickCommand.disabled=true;
    quickRun.disabled=true;
    quickHint.textContent='';
    gitSummary.hidden=true;
    gitFiles.replaceChildren();
    gitBranch.textContent='';
    gitRows=[];
    gitRawSnapshot='';gitFileSelect.replaceChildren();gitFileSelect.value='';
    gitFileSelect.disabled=true;
    gitCommitMessage.value='';
    gitSnapshotCount=0;
    gitSnapshotBranch='';
    gitDeltaSummary.textContent='';
    gitDeltaList.replaceChildren();
    gitFilter.value='';
    gitFilter.disabled=true;
    gitKind.value='all';
    gitKind.disabled=true;
    gitCount.textContent='';
    gitEmpty.hidden=true;
    gitEmpty.textContent='';
    projectOverview.hidden=true;
    filesRootOutput.hidden=true;filesRootOutput.textContent='';
    filesRootNames=[];filesRootMore=false;
    filesReadName='';filesReadText='';filesReadPage=0;filesReadMore=false;
    filesPendingSelectedName='';
    filesSelect.replaceChildren();
    filesSelect.value='';
    filesDirNames=[];filesDirMore=false;filesDirChosen='';
    filesDirSelect.replaceChildren();filesDirSelect.value='';
    filesDirOutput.textContent='';filesDirOutput.hidden=true;
    filesReadOutput.textContent='';filesReadOutput.hidden=true;
    projectName.textContent='';
    workspaceName.textContent='';
    hostOS.textContent='';
    taskPreviewSection.hidden=true;
    taskLabels.replaceChildren();
    taskPaging.hidden=true;
    cachedTaskLabels=[];
    cachedTaskIDs=[];
    inspectedTasks.clear();
    taskInputDrafts.clear();
    inspectButtons=[];
    lastCompleteTaskLabels=null;
    taskCompareSummary.textContent='';
    taskCompareList.replaceChildren();
    remoteTaskTotal=0;
    canFetchTaskNext=false;
    taskFetchNext.hidden=true;
    taskFetchNext.disabled=true;
    bulkStopRequested=true;
    bulkLoading=false;
    taskFetchAll.hidden=true;
    taskFetchAll.disabled=true;
    taskStopAll.hidden=true;
    taskStopAll.disabled=true;
    taskLoadStatus.textContent='';
    taskFilter.value='';
    taskFilter.disabled=true;
    taskSort.value='host';
    taskSort.disabled=true;
    taskCoverage.textContent='';
    taskNextHelp.textContent='';
    taskProgress.value=0;
    taskProgress.max=1;
    taskFilterEmpty.hidden=true;
    taskPage=0;
    taskTotal.textContent='';
    targetDevice.textContent='';
    targetUser.textContent='';
    pingLatency.textContent='Chưa đo';
    featuresKnown=false;
    available={session_ping:false,project_info:false,files_root:false,tasks_preview:false,task_run:false,task_long:false,git_status:false,git_log:false,git_branches:false,git_branch_create:false,git_diff:false,git_patch:false,git_fetch:false,git_stage_tracked:false,git_unstage_all:false,git_stage_file:false,git_unstage_file:false,git_pull:false,git_commit:false,git_push:false,terminal:false};
    featureSummary.textContent='Phiên đã đóng. Đăng nhập lại bằng client TaskDeck tin cậy.';
  }
  // Host guarantees terminal task status is published only after the
  // process group has exited and its workspace lease was released.
  // Preserve the completed job's output while allowing the next Git
  // operation; keep three E2EE rounds reserved for explicit Clear.
  // Git index/HEAD changes invalidate previously decrypted diff pages.
  // The host will enforce its own source snapshot; clearing the client
  // cursor first prevents stale output and synthetic Next clicks.
  function invalidateGitPatchReview(){
    gitPatchText='';gitPatchPage=0;gitPatchMore=false;
    gitPatchMode='head';
    gitPatchOutput.textContent='';gitPatchOutput.hidden=true;
  }
  function invalidateGitBranchReview(){
    gitSwitchPreview=null;gitSwitchPending=null;
    gitDeletePreview=null;gitDeletePending=null;
  }
  function gitJobBlocksMutation(){
    const finished=['completed','failed','cancelled','timed_out'].includes(longTaskState);
    return terminalState!=='closed'||
      (longTaskID!==0&&
        (!finished||(budget<=3&&(longTaskMore||budget<=1))));
  }
  function render(){
    if(closed)clearPrivateViews();
    const terminalCloseReserved=terminalState!=='closed'&&budget<=1;
    const maintenanceOnly=(longTaskID!==0&&budget<=3)||terminalCloseReserved;
    const gitWorkspaceBusy=gitJobBlocksMutation();
    // Only definitive native close or browser pagehide can destroy the
    // local capability. A denied E2EE action still permits authenticated
    // /api/close to revoke the native server before expiry.
    if(terminated){
      capability='';
      localToken.value='';
    }
    activate.disabled=activationConsumed||verified||closed||closing||terminated;
    quickCommand.disabled=busy||closed||!verified||maintenanceOnly;
    quickRun.disabled=busy||closed||!verified||maintenanceOnly;
    ping.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||
      (featuresKnown&&!available.session_ping);
    features.disabled=busy||closed||!verified||budget<=0||maintenanceOnly;
    git.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.git_status;
    gitLog.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.git_log;
    gitBranches.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.git_branches;
    gitRemotes.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.git_status;
    gitTrackingStart.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.git_status;
    gitTrackingNext.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.git_status||!gitTrackingMore;
    gitBranchCreate.disabled=busy||closed||!verified||budget<=0||gitWorkspaceBusy||!featuresKnown||!available.git_branch_create;
     // Branch read availability is a UX hint only. Host independently
     // requires git.write and TASKDECK_REMOTE_GIT_SWITCH=1.
     gitSwitchReview.disabled=busy||closed||!verified||budget<=0||gitWorkspaceBusy||!featuresKnown||!available.git_branches;
     gitSwitchSameHead.disabled=gitSwitchReview.disabled||gitSwitchPreview===null;
    gitBranchDeleteReview.disabled=gitSwitchReview.disabled;
    gitBranchDelete.disabled=gitBranchDeleteReview.disabled||gitDeletePreview===null;
    gitDiff.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.git_diff;
    gitPatchStart.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.git_patch;
     gitPatchCachedStart.disabled=gitPatchStart.disabled;
    gitPatchNext.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.git_patch||!gitPatchMore;
    gitFetch.disabled=busy||closed||!verified||budget<=0||gitWorkspaceBusy||!featuresKnown||!available.git_fetch;
    gitStageTracked.disabled=busy||closed||!verified||budget<=0||gitWorkspaceBusy||!featuresKnown||!available.git_stage_tracked;
    const selectedFile=gitRows[Number(gitFileSelect.value)-1];
    const fileReady=!busy&&!closed&&verified&&budget>0&&!gitWorkspaceBusy&&
      featuresKnown&&gitRawSnapshot!==''&&selectedFile!==undefined&&
      Number.isInteger(Number(gitFileSelect.value));
    gitStageFile.disabled=!fileReady||!available.git_stage_file||
      !['M','D'].includes(selectedFile?.code?.[1]);
    gitUnstageFile.disabled=!fileReady||!available.git_unstage_file||
      !['M','A','D'].includes(selectedFile?.code?.[0]);
    gitUnstageAll.disabled=busy||closed||!verified||budget<=0||gitWorkspaceBusy||!featuresKnown||!available.git_unstage_all;
    gitPullReview.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.git_status;
     gitPullChanges.disabled=gitPullReview.disabled;
    gitPull.disabled=busy||closed||!verified||budget<=0||gitWorkspaceBusy||!featuresKnown||!available.git_pull;
    gitCommit.disabled=busy||closed||!verified||budget<=0||gitWorkspaceBusy||!featuresKnown||!available.git_commit;
    gitPush.disabled=busy||closed||!verified||budget<=0||gitWorkspaceBusy||!featuresKnown||!available.git_push;
    gitCommitMessage.disabled=gitCommit.disabled;
    const canTerminal=featuresKnown&&available.terminal&&verified&&!closed&&budget>0;
    terminalOpen.disabled=busy||!canTerminal||budget<2||
      terminalState!=='closed'||longTaskID!==0;
    const terminalLive=canTerminal&&terminalState==='running'&&
      !terminalCloseReserved;
    terminalPoll.disabled=busy||!canTerminal||terminalCloseReserved||
      terminalState==='closed';
    terminalAutoButton.disabled=busy||!canTerminal||
      (terminalState==='closed'&&budget<2);
    terminalAutoButton.textContent=terminalAutoEnabled?
      'Tắt tự nhận output':'Bật tự nhận output';
    terminalSend.disabled=busy||!terminalLive;
    terminalInput.disabled=terminalSend.disabled;
    terminalInterrupt.disabled=busy||!terminalLive;
    terminalEOF.disabled=busy||!terminalLive;
    terminalResize.disabled=busy||!terminalLive;
    terminalClose.disabled=busy||!canTerminal||terminalState==='closed';
    project.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.project_info;
    filesRootButton.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.files_root;
    filesRootNext.disabled=filesRootButton.disabled||!filesRootMore;
    filesSelect.disabled=filesRootButton.disabled||filesRootNames.every(x=>!x.startsWith('F:'));
    filesDirSelect.disabled=filesRootButton.disabled||filesRootNames.every(x=>!x.startsWith('D:'));
    const dirIndex=Number(filesDirSelect.value);
    filesDirOpen.disabled=filesDirSelect.disabled||
      !Number.isInteger(dirIndex)||dirIndex<1||
      dirIndex>filesRootNames.length||!filesRootNames[dirIndex-1].startsWith('D:');
    filesDirNext.disabled=filesRootButton.disabled||!filesDirMore;
    const selectedIndex=Number(filesSelect.value);
    filesReadSelected.disabled=filesSelect.disabled||
      !Number.isInteger(selectedIndex)||selectedIndex<1||
      selectedIndex>filesRootNames.length||
      !filesRootNames[selectedIndex-1].startsWith('F:');
    // This is only an affordance; the Agent independently checks
    // files.read and both opt-ins before opening a host-chosen file.
    filesReadStart.disabled=filesRootButton.disabled;
    filesReadNext.disabled=filesReadStart.disabled||!filesReadMore;
    tasksPreview.disabled=busy||closed||!verified||budget<=0||maintenanceOnly||!featuresKnown||!available.tasks_preview;
    tasksReport.disabled=tasksPreview.disabled;
    workflowAuditButton.disabled=tasksReport.disabled;
    workflowAuditNext.disabled=tasksReport.disabled||!workflowAuditMore||
      workflowAuditRows.length>=32;
    const depsID=Number(taskDepsSelect.value);
    taskDepsSelect.disabled=tasksPreview.disabled||
      !cachedTaskIDs.some(id=>Number.isInteger(id)&&id>0);
    taskDepsView.disabled=taskDepsSelect.disabled||
      !Number.isInteger(depsID)||depsID<1||depsID>2147483647||
      !cachedTaskIDs.includes(depsID);
    taskWorkflowStart.disabled=taskDepsView.disabled||
      taskWorkflowReadyID!==depsID||!available.task_long||
      !available.task_run||longTaskID!==0||budget<=3||bulkLoading;
    taskRunStop.hidden=!taskRunning||closed;
    taskRunStop.disabled=!taskRunning||!verified||closed||closing||terminated;
    longTaskControls.hidden=longTaskID===0||closed;
    longTaskOutput.hidden=longTaskID===0||closed;
    longTaskCopy.disabled=closed||terminated||!verified||
      longTaskID===0||longTaskText.length===0;
    longTaskPoll.disabled=busy||closed||!verified||budget<=0||longTaskID===0||
      !taskMaintenanceAllowed('/api/tasks-poll/'+String(longTaskID))||
      (!['started','running','cancel_requested'].includes(longTaskState)&&!longTaskMore);
    longTaskAutoButton.disabled=closed||!verified||longTaskID===0;
    longTaskAutoButton.textContent=longTaskAuto?'Tạm dừng tự nhận':'Tiếp tục tự nhận';
    longTaskCancel.disabled=busy||closed||!verified||budget<=0||
      longTaskID===0||longTaskCancelPending||
      !taskMaintenanceAllowed('/api/tasks-cancel/'+String(longTaskID))||
      !['started','running'].includes(longTaskState);
    longTaskClear.textContent=longTaskMore?
      'Bỏ output còn lại và dọn job':'Dọn job đã kết thúc';
    longTaskClear.disabled=busy||closed||!verified||budget<=0||
      longTaskID===0||!['completed','failed','cancelled','timed_out'].includes(longTaskState);
    taskFetchNext.disabled=busy||bulkLoading||closed||!verified||budget<=0||
      maintenanceOnly||!featuresKnown||!available.tasks_preview;
    taskFetchAll.disabled=busy||bulkLoading||closed||!verified||budget<=0||
      maintenanceOnly||
      !featuresKnown||!available.tasks_preview;
    // Ending a bulk run does not re-render the task page. Keep visibility
    // derived from state so Stop/Resume cannot leave stale controls.
    taskFetchAll.hidden=!canFetchTaskNext||bulkLoading;
    taskStopAll.hidden=!bulkLoading;
    taskStopAll.disabled=closed||!bulkLoading;
    for(const button of inspectButtons){
      button.disabled=busy||closed||bulkLoading||!verified||budget<=0||
        maintenanceOnly||!featuresKnown||(button.taskRun?
          !available.task_run:!available.tasks_preview)||
        (button.taskRun&&!inspectedTasks.has(
          Number(button.taskID||0)))||
        (button.taskRun&&(terminalState!=='closed'||longTaskID!==0))||
        (button.taskLong===true&&(!available.task_long||longTaskID!==0));
    }
    if(taskPreviewSection.hidden===false){renderTaskCoverage();}
    // Keep Disconnect available even after a revoked/failed operation.
    disconnect.disabled=terminated||closing||!verified;
    state.textContent=closed?'Phiên đã đóng':(!verified?'Không có mã phiên cục bộ':busy?'Đang xử lý':'Phiên cục bộ sẵn sàng');
    state.classList.toggle('bad',closed||!verified);
    remaining.textContent='Lượt thao tác còn lại: '+budget;
    // The status line is derived exclusively from the last authenticated
    // host job/PTY response, never from task names or unauthenticated DOM
    // text. A completed job remains here until explicit Clear, and an
    // acknowledged Cancel may still be terminating the host process.
    let activity='Không có tác vụ đang chạy';
    if(terminalState==='running')activity='Terminal PTY đang chạy';
    else if(terminalState!=='closed')activity='Terminal PTY: '+terminalState;
    if(longTaskID!==0){
      const jobLabel='Task '+String(longTaskID);
      if(['completed','failed','cancelled','timed_out'].includes(longTaskState)){
        activity=jobLabel+' đã kết thúc ('+longTaskState+') — chờ dọn job'+
          (longTaskMore?' · còn output chưa nhận':'');
      }else if(longTaskCancelPending||longTaskState==='cancel_requested'){
        activity=jobLabel+' đã yêu cầu hủy — đang chờ kết thúc';
      }else{
        activity=jobLabel+' đang chạy ('+longTaskState+')';
      }
    }
    sessionActivity.textContent=closed?'Phiên không còn hợp lệ':
      busy?'Đang chờ phản hồi E2EE':
      terminalCloseReserved?activity+' · Giữ lượt cuối để đóng Terminal':activity;
  }
  // Running jobs reserve Cancel + Poll + Clear. After a Cancel ACK or
  // any terminal state, Cancel is no longer legal: automatic draining
  // may spend remaining rounds down to ONE final Clear. Recheck both
  // the session quota and ownership when each timer actually fires.
  function mayAutoPollLongTask(){
    const requiresCancelReserve=
      ['started','running'].includes(longTaskState)&&!longTaskCancelPending;
    return longTaskID!==0&&
      (requiresCancelReserve?budget>3:budget>1)&&
      (['started','running','cancel_requested'].includes(longTaskState)||
        longTaskMore)&&
      taskMaintenanceAllowed('/api/tasks-poll/'+String(longTaskID));
  }
  function scheduleTerminalAutoPoll(){
    if(!terminalAutoEnabled||!terminalAutoPending||terminalAutoScheduled||
      !verified||busy||closed||closing||terminated||
      !featuresKnown||!available.terminal||terminalState==='closed'||
      !terminalMaintenanceAllowed('/api/terminal-poll'))return;
    const epoch=terminalAutoEpoch;
    terminalAutoScheduled=true;
    setTimeout(()=>{
      if(epoch!==terminalAutoEpoch)return;
      terminalAutoScheduled=false;
      if(terminalAutoEnabled&&terminalAutoPending&&!busy&&!closed&&
        !closing&&!terminated&&
        terminalMaintenanceAllowed('/api/terminal-poll')){
        // The reply decides whether another bounded chunk remains.
        terminalAutoPending=false;
        invoke('/api/terminal-poll');
      }
    },350);
  }
  function scheduleLongTaskPoll(){
    if(!longTaskAuto||longTaskPollScheduled||
      !verified||busy||closed||closing||terminated||bulkLoading||
      !mayAutoPollLongTask())return;
    const selectedID=longTaskID,selectedEpoch=longTaskPollEpoch;
    longTaskPollScheduled=true;
    setTimeout(()=>{
      // Do not disturb a newer execution's scheduled timer if this
      // callback belongs to a previously cleared job or paused run.
      if(selectedEpoch!==longTaskPollEpoch)return;
      longTaskPollScheduled=false;
      if(selectedID===longTaskID&&longTaskAuto&&!busy&&!closed&&
        !closing&&!terminated&&!bulkLoading&&mayAutoPollLongTask()){
        invoke('/api/tasks-poll/'+String(selectedID));
      }
    },450);
  }
  function validCommitSubject(message){
    if(typeof message!=='string'||message.length<1||message!==message.trim()||
      /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u.test(message))return false;
    // URI percent escapes count UTF-8 bytes; no browser storage or network
    // call is required to validate a potentially Vietnamese commit subject.
    try{
      return encodeURIComponent(message).replace(/%[0-9a-f]{2}/gi,'x').length<=120;
    }catch(error){return false;}
  }
  function validTerminalInputSegment(input){
    if(typeof input!=='string'||input.length<1)return false;
    for(const ch of input){
      const code=ch.codePointAt(0);
      if([3,4,8,9,10,13,127].includes(code))continue;
      if(/[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u.test(ch))return false;
    }
    try{
      return encodeURIComponent(input).replace(/%[0-9a-f]{2}/gi,'x').length<=96;
    }catch(error){return false;}
  }
  function validTerminalOutput(value,maxBytes=128){
    if(typeof value!=='string')return false;
    try{
      if(encodeURIComponent(value).replace(/%[0-9a-f]{2}/gi,'x').length>maxBytes)return false;
    }catch(error){return false;}
    for(const ch of value){
      if(ch==='\n'||ch==='\t')continue;
      if(/[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u.test(ch))return false;
    }
    return true;
  }
  function validGitRemotesReport(value){
    const prefix='GIT_REMOTES_V1\n';
    if(typeof value!=='string'||value.length>320||!value.startsWith(prefix))return false;
    const rest=value.slice(prefix.length);
    if(rest==='')return true;
    if(!rest.endsWith('\n'))return false;
    const names=rest.slice(0,-1).split('\n');
    if(names.length>16)return false;
    let prior='';
    for(const name of names){
      if(!/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(name)||
         /[._-]$/.test(name)||name.includes('..')||
         (prior!==''&&prior>=name))return false;
      prior=name;
    }
    return true;
  }
  async function invoke(endpoint,payload){
    const isClose=endpoint==='/api/close';
    const isTextRun=endpoint.startsWith('/api/tasks-run-input/')||
      endpoint.startsWith('/api/tasks-start-input/');
    const isRun=endpoint.startsWith('/api/tasks-run/')||
      endpoint.startsWith('/api/tasks-run-input/');
    const isLongAction=endpoint.startsWith('/api/tasks-workflow-start/')||
      endpoint.startsWith('/api/tasks-start/')||
      endpoint.startsWith('/api/tasks-start-input/')||
      endpoint.startsWith('/api/tasks-poll/')||
      endpoint.startsWith('/api/tasks-cancel/')||
      endpoint.startsWith('/api/tasks-clear/');
    const isCommit=endpoint==='/api/git-commit';
    const isGitFile=endpoint==='/api/git-stage-file'||endpoint==='/api/git-unstage-file';
    const isFilesSelect=endpoint==='/api/files-read-select'||endpoint==='/api/files-dir-select';
    const isTerminalWrite=endpoint==='/api/terminal-write';
    const isTerminalResize=endpoint==='/api/terminal-resize';
    if(payload!==undefined){
      if(!payload)return;
      if(isTextRun){
        if(Object.keys(payload).length!==1||typeof payload.value!=='string'||
          !/^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$/.test(payload.value))return;
      }else if(isCommit){
        if(Object.keys(payload).length!==1||!validCommitSubject(payload.message))return;
       }else if(isFilesSelect){
        if(Object.keys(payload).length!==1||!Number.isInteger(payload.index)||
          payload.index<1||payload.index>128||
          payload.index>filesRootNames.length||
          !(endpoint==='/api/files-dir-select'?
            filesRootNames[payload.index-1].startsWith('D:'):
            filesRootNames[payload.index-1].startsWith('F:')))return;
      }else if(isGitFile){
        if(Object.keys(payload).length!==2||!Number.isInteger(payload.index)||
          payload.index<1||payload.index>12||typeof payload.snapshot!=='string'||
          payload.snapshot!==gitRawSnapshot||payload.snapshot.length>256)return;
      }else if(isTerminalWrite){
        if(Object.keys(payload).length!==1||
          !validTerminalInputSegment(payload.input))return;
      }else if(isTerminalResize){
        if(Object.keys(payload).length!==2||
          !Number.isInteger(payload.cols)||payload.cols<20||payload.cols>180||
          !Number.isInteger(payload.rows)||payload.rows<8||payload.rows>60)return;
      }else return;
    }
    if((isTextRun||isCommit||isGitFile||isFilesSelect||isTerminalWrite||isTerminalResize)&&payload===undefined)return;
    // A disabled button is UX; the endpoint also needs local admission.
    // Share the exact endpoint set with the reserved-E2EE-round gate.
    if(remoteGitMutations.includes(endpoint)&&gitJobBlocksMutation())return;
    // An acknowledged cancel is irreversible for this job. The next
    // status read can legitimately be "running" while SIGTERM propagates;
    // it must never authorize a second cancel or consume cleanup quota.
    if(longTaskCancelPending&&longTaskID>0&&
      endpoint==='/api/tasks-cancel/'+String(longTaskID))return;
    if(!verified||(busy&&!isClose)||(closed&&!isClose)||
      (!isClose&&budget<=0)||
      (!isClose&&!taskMaintenanceAllowed(endpoint))||
      (!isClose&&!terminalMaintenanceAllowed(endpoint))||
      (!isClose&&bulkLoading&&endpoint!='/api/tasks-preview-next')||
      (isClose&&(closing||terminated)))return;
    // Closing the UI scrubs its token synchronously. Capture the
    // capability for THIS already-admitted local request, never a
    // navigable URL, clipboard, browser storage, or arbitrary endpoint.
    const requestCapability=capability;
    if(isClose){closing=true;closed=true;}
    if(isRun)taskRunning=true;
    actionEpoch++;
    busy=true;render();
    try{
      const headers={'X-TaskDeck-Local-Capability':requestCapability};
      if(isTextRun||isCommit||isGitFile||isFilesSelect||isTerminalWrite||isTerminalResize)headers['Content-Type']='application/json';
      const options={
        method:'POST',mode:'same-origin',credentials:'omit',cache:'no-store',
        headers
      };
      if(isTextRun||isCommit||isGitFile||isFilesSelect||isTerminalWrite||isTerminalResize)options.body=JSON.stringify(payload);
      const response=await fetch(endpoint,options);
      if(!response.ok)throw new Error('TaskDeck từ chối thao tác hoặc phiên đã hết hạn (HTTP '+response.status+')');
      const data=await response.json();
      // A late read result must NEVER overwrite a user-requested logout.
      if(!isClose&&closed)return;
      if(!data.ok)throw new Error('Phản hồi xác thực không hợp lệ');
      if(endpoint==='/api/close'){
        closed=true;terminated=true;
        output.textContent=expiredByClock?
          'Phiên đã hết hạn và đã ngắt kết nối E2EE.':
          'Đã ngắt phiên.';
      }else{
        budget=Number(data.remaining);
        if(!Number.isInteger(budget)||budget<0||budget>16)throw new Error('Số lượt không hợp lệ');
        output.textContent=endpoint==='/api/git-status'?String(data.status):
          endpoint==='/api/project-info'?'Đã đọc thông tin dự án qua E2EE.':
          (endpoint==='/api/tasks-preview'||endpoint==='/api/tasks-preview-next')?
           'Đã đọc nhãn task qua E2EE.':
          endpoint==='/api/features'?'Đã đọc các quyền GUI hiện tại qua E2EE.':
          endpoint.startsWith('/api/tasks-inspect/')?
            'Đang xác minh định danh task qua E2EE.':
          (endpoint.startsWith('/api/tasks-run/')||isTextRun)?
            'Đang chạy task đã chọn trên host.':String(data.message);
        if(endpoint==='/api/features'){
          const priorTaskRun=available.task_run,priorTaskLong=available.task_long;
          const keys=['session_ping','project_info','tasks_preview','git_status'];
          if(data.remote_control_available!==false||
            keys.some(key=>typeof data[key]!=='boolean')||
            (data.task_run!==undefined&&typeof data.task_run!=='boolean')||
            (data.task_long!==undefined&&typeof data.task_long!=='boolean')||
            (data.files_root!==undefined&&typeof data.files_root!=='boolean')||
            (data.git_log!==undefined&&typeof data.git_log!=='boolean')||
            (data.git_branches!==undefined&&typeof data.git_branches!=='boolean')||
            (data.git_branch_create!==undefined&&typeof data.git_branch_create!=='boolean')||
            (data.git_diff!==undefined&&typeof data.git_diff!=='boolean')||
            (data.git_patch!==undefined&&typeof data.git_patch!=='boolean')||
            (data.git_stage_tracked!==undefined&&typeof data.git_stage_tracked!=='boolean')||
            (data.git_unstage_all!==undefined&&typeof data.git_unstage_all!=='boolean')||
            (data.git_stage_file!==undefined&&typeof data.git_stage_file!=='boolean')||
            (data.git_unstage_file!==undefined&&typeof data.git_unstage_file!=='boolean')||
            (data.git_fetch!==undefined&&typeof data.git_fetch!=='boolean')||
            (data.git_pull!==undefined&&typeof data.git_pull!=='boolean')||
            (data.git_commit!==undefined&&typeof data.git_commit!=='boolean')||
            (data.git_push!==undefined&&typeof data.git_push!=='boolean')||
            (data.terminal!==undefined&&typeof data.terminal!=='boolean')){
            throw new Error('Phản hồi khả năng GUI không hợp lệ');
          }
          available={session_ping:data.session_ping,
            project_info:data.project_info,files_root:data.files_root===true,
            tasks_preview:data.tasks_preview,task_run:data.task_run===true,
            task_long:data.task_long===true,
            git_status:data.git_status,git_log:data.git_log===true,git_branches:data.git_branches===true,git_branch_create:data.git_branch_create===true,git_diff:data.git_diff===true,git_patch:data.git_patch===true,git_fetch:data.git_fetch===true,
            git_stage_tracked:data.git_stage_tracked===true,
            git_unstage_all:data.git_unstage_all===true,
            git_stage_file:data.git_stage_file===true,
            git_unstage_file:data.git_unstage_file===true,
            git_pull:data.git_pull===true,
            git_commit:data.git_commit===true,git_push:data.git_push===true,
            terminal:data.terminal===true};
          featuresKnown=true;
          featureSummary.textContent=
            'Ping: '+(available.session_ping?'Được phép':'Thiếu quyền')+
            ' · Dự án: '+(available.project_info?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Files gốc: '+(available.files_root?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Task: '+(available.tasks_preview?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Git: '+(available.git_status?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Git log: '+(available.git_log?'Được phép (3 commit)':'Chưa bật hoặc thiếu quyền')+
            ' · Nhánh cục bộ: '+(available.git_branches?'Được phép (tối đa 4)':'Chưa bật hoặc thiếu quyền')+
             ' · Git remotes: '+(available.git_status?'Chỉ tên · cần opt-in riêng tại host':'Thiếu quyền đọc Git')+
            ' · Tracking refs: '+(available.git_status?'Chỉ đọc · cần opt-in tại host':'Thiếu quyền đọc Git')+
            ' · Tạo nhánh host: '+(available.git_branch_create?'Được phép (từ HEAD)':'Chưa bật hoặc thiếu quyền')+
            ' · Git diff: '+(available.git_diff?'Được phép (--stat)':'Chưa bật hoặc thiếu quyền')+
            ' · Diff chi tiết: '+(available.git_patch?'Được phép (4 KiB)':'Chưa bật hoặc thiếu quyền')+
            ' · Stage tracked: '+(available.git_stage_tracked?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Unstage all: '+(available.git_unstage_all?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Stage một file: '+(available.git_stage_file?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Unstage một file: '+(available.git_unstage_file?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Git fetch: '+(available.git_fetch?'Được phép (upstream)':'Chưa bật hoặc thiếu quyền')+
            ' · Xem trước Pull: '+(available.git_status?'Chỉ đọc offline · cần opt-in host':'Thiếu quyền Git')+
            ' · Git pull: '+(available.git_pull?'Được phép (ff-only)':'Chưa bật hoặc thiếu quyền')+
            ' · Git commit: '+(available.git_commit?'Được phép (staged)':'Chưa bật hoặc thiếu quyền')+
            ' · Git push: '+(available.git_push?'Được phép (upstream)':'Chưa bật hoặc thiếu quyền')+
            ' · PTY: '+(available.terminal?'Được phép (Unix)':'Chưa bật hoặc thiếu quyền')+
            ' · Chạy task: '+(available.task_run?'Được phép (task ngắn)':'Chưa bật hoặc thiếu quyền')+
            ' · Job dài: '+(available.task_long?'Được phép (Unix, giới hạn)':'Chưa bật hoặc thiếu quyền');
          // A feature refresh often changes no task capabilities. Avoid
          // recreating focused task inputs unless the buttons to display
          // actually changed; render() handles ordinary disabled states.
          if(!taskPreviewSection.hidden&&
             (priorTaskRun!==available.task_run||
              priorTaskLong!==available.task_long))renderTaskPage();
        }
        if(endpoint==='/api/files-read-start'||endpoint==='/api/files-read-next'||
           endpoint==='/api/files-read-select'){
          const first=endpoint==='/api/files-read-start'||endpoint==='/api/files-read-select';
          const safeName=typeof data.name==='string'&&
            /^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$/.test(data.name);
          if(data.remote_control_available!==false||
            !safeName||!Number.isInteger(data.page)||data.page<1||data.page>16||
            typeof data.more!=='boolean'||typeof data.text!=='string'||
            data.text.length>220||(data.more&&data.text.length===0)||
            data.text.length===0&&!first||
            !first&&(!filesReadMore||filesReadName!==data.name||
              data.page!==filesReadPage+1)||
            first&&data.page!==1||
            (endpoint==='/api/files-read-select'&&
              (!filesPendingSelectedName||data.name!==filesPendingSelectedName))||
            (!first&&filesReadText.length+data.text.length>2048)||
            Array.from(data.text).some(ch=>{
              const cp=ch.codePointAt(0);
              return cp!==9&&cp!==10&&(cp<32||
                (cp>=127&&cp<=159)||(cp>=0x200b&&cp<=0x200f)||
                (cp>=0x202a&&cp<=0x202e)||
                (cp>=0x2060&&cp<=0x206f));
            })){
            throw new Error('Trang nội dung file E2EE không hợp lệ');
          }
          filesReadName=data.name;
          filesReadPage=data.page;
          filesReadText=first?data.text:filesReadText+data.text;
          filesReadMore=data.more;
          if(first)filesPendingSelectedName='';
          filesReadOutput.textContent='File: '+filesReadName+' (chỉ đọc)\n'+
            filesReadText+(filesReadMore?'\n[Còn nội dung chưa nhận]':'');
          filesReadOutput.hidden=false;
         }
         if(endpoint==='/api/files-dir-select'||endpoint==='/api/files-dir-next'){
          const first=endpoint==='/api/files-dir-select';
          if(data.remote_control_available!==false||!Array.isArray(data.items)||
            data.items.length>8||typeof data.more!=='boolean'||
            (data.more&&data.items.length===0)||
            (!first&&!filesDirMore)||
            (!first&&filesDirNames.length+data.items.length>128)||
            (first&&!filesDirChosen)||
            data.items.some((v,i)=>typeof v!=='string'||
              !/^[FD]:[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$/.test(v)||
              (i>0&&data.items[i-1].slice(2)>=v.slice(2)))||
            (!first&&data.items.length>0&&filesDirNames.length>0&&
              filesDirNames[filesDirNames.length-1].slice(2)>=data.items[0].slice(2))){
            throw new Error('Trang thư mục con E2EE không hợp lệ');
          }
          filesDirNames=first?data.items.slice():filesDirNames.concat(data.items);
          filesDirMore=data.more;
          filesDirOutput.textContent='Thư mục: '+filesDirChosen+' (chỉ liệt kê tên)\n'+
            filesDirNames.map(x=>(x.startsWith('D:')?'[DIR] ':'[FILE] ')+x.slice(2)).join('\n')+
            (filesDirMore?'\n[Còn mục tiếp]':'');
          filesDirOutput.hidden=false;
         }
         if(endpoint==='/api/files-root'||endpoint==='/api/files-root-next'){
          const first=endpoint==='/api/files-root';
          if(data.remote_control_available!==false||!Array.isArray(data.items)||data.items.length>8||
            typeof data.more!=='boolean'||(data.more&&data.items.length===0)||
            (!first&&!filesRootMore)||
            (!first&&filesRootNames.length+data.items.length>128)||
            data.items.some((v,i)=>typeof v!=='string'||
              !/^[FD]:[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$/.test(v)||
              (i>0&&data.items[i-1].slice(2)>=v.slice(2)))||
            (!first&&data.items.length>0&&filesRootNames.length>0&&
              filesRootNames[filesRootNames.length-1].slice(2)>=data.items[0].slice(2))){
            throw new Error('Trang Files E2EE không hợp lệ');
          }
          filesRootNames=first?data.items.slice():filesRootNames.concat(data.items);
          filesRootMore=data.more;
          const previousDirChoice=first?'':filesDirSelect.value;
          filesDirSelect.replaceChildren();
          const firstDir=document.createElement('option');
          firstDir.value='';firstDir.textContent='Chọn thư mục đã nhận qua E2EE';
          filesDirSelect.append(firstDir);
          filesRootNames.forEach((v,i)=>{
            if(!v.startsWith('D:'))return;
            const option=document.createElement('option');
            option.value=String(i+1);
            option.textContent=v.slice(2);
            filesDirSelect.append(option);
          });
          filesDirSelect.value=(previousDirChoice&&
            filesRootNames[Number(previousDirChoice)-1]?.startsWith('D:'))?
            previousDirChoice:'';
          if(first){
            filesDirNames=[];filesDirMore=false;filesDirChosen='';
            filesDirOutput.textContent='';filesDirOutput.hidden=true;
          }
          const previousChoice=first?'':filesSelect.value;
          filesSelect.replaceChildren();
          const initial=document.createElement('option');
          initial.value='';initial.textContent='Chọn file đã nhận qua E2EE';
          filesSelect.append(initial);
          filesRootNames.forEach((v,i)=>{
            if(!v.startsWith('F:'))return;
            const option=document.createElement('option');
            option.value=String(i+1);
            option.textContent=v.slice(2);
            filesSelect.append(option);
          });
          filesSelect.value=(previousChoice&&filesRootNames[Number(previousChoice)-1]?.startsWith('F:'))?
            previousChoice:'';
          if(first){
            filesReadName='';filesReadText='';filesReadPage=0;filesReadMore=false;
            filesPendingSelectedName='';
            filesReadOutput.textContent='';filesReadOutput.hidden=true;
          }
          filesRootOutput.textContent=filesRootNames.map(v=>
            (v[0]==='D'?'Thư mục: ':'File: ')+v.slice(2)).join('\n')+
            (filesRootMore?'\nCòn mục chưa nhận.':'');
          if(filesRootNames.length===0)filesRootOutput.textContent='Không có mục gốc phù hợp.';
          filesRootOutput.hidden=false;
         }
         if(endpoint==='/api/project-info'){
          for(const key of ['project_name','workspace_name']){
            if(typeof data[key]!=='string'||data[key].length===0||
              data[key].length>96)throw new Error('Thông tin dự án không hợp lệ');
          }
          if(!['linux','darwin','windows'].includes(data.host_os))throw new Error('Nền tảng host không hợp lệ');
          projectName.textContent=data.project_name;
          workspaceName.textContent=data.workspace_name;
          hostOS.textContent=data.host_os;
          projectOverview.hidden=false;
        }
        if(endpoint.startsWith('/api/tasks-dependencies/')){
          const id=Number(endpoint.slice('/api/tasks-dependencies/'.length));
          const order=data.order,edges=data.edges;
          const validID=x=>Number.isInteger(x)&&x>0&&x<=2147483647;
          const positions=new Map();
          const seenEdges=new Set();
          const allNodesReachRoot=()=>{
            // A runnable reply must contain exactly one rooted DAG. The
            // client does not infer parallelism or accept disconnected IDs.
            const reached=new Set([id]);
            for(let i=order.length-1;i>=0;i--){
              if(!reached.has(order[i]))continue;
              for(const edge of edges){
                if(edge[1]===order[i])reached.add(edge[0]);
              }
            }
            return reached.size===order.length;
          };
          // A local task selection can change while an authenticated
          // dependency read is still awaiting its E2EE reply. This is a
          // discarded UI selection, not an invalid remote message.
          // Fully validate the host's graph before considering it stale.
          const stillSelected=id===taskDepsPendingID&&
            Number(taskDepsSelect.value)===id;
          if(!validID(id)||
            !cachedTaskIDs.includes(id)||data.task_id!==id||
            data.remote_control_available!==false||
            typeof data.available!=='boolean'||typeof data.run_available!=='boolean'||
            !Array.isArray(order)||
            !Array.isArray(edges)||order.length>8||edges.length>8||
            order.some((n,i)=>{
              if(!validID(n)||positions.has(n))return true;
              positions.set(n,i);return false;
            })||
            (!data.available&&(data.run_available||order.length!==0||edges.length!==0))||
            (data.available&&(order.length===0||order[order.length-1]!==id))||
            (data.run_available&&(!data.available||order.length<2||
              order.length>4||edges.length<order.length-1))||
            edges.some(e=>{
              if(!Array.isArray(e)||e.length!==2||
                !positions.has(e[0])||!positions.has(e[1])||
                positions.get(e[0])>=positions.get(e[1]))return true;
              const key=String(e[0])+':'+String(e[1]);
              if(seenEdges.has(key))return true;
              seenEdges.add(key);
              return false;
            })||
            (data.run_available&&!allNodesReachRoot())){
            throw new Error('Đồ thị phụ thuộc E2EE không hợp lệ');
          }
          const displayID=x=>{
            const i=cachedTaskIDs.indexOf(x);
            return i>=0?cachedTaskLabels[i]+' (#'+x+')':'Task #'+x;
          };
          if(stillSelected){
          taskDepsOutput.textContent=!data.available?
            'Host chưa bật xem phụ thuộc cho TaskDeck Remote.':
            'Thứ tự topo (chỉ xem, không tự chạy):\n'+
             order.map((id,i)=>String(i+1)+'. '+displayID(id)).join('\n')+
            '\nPhụ thuộc:\n'+
             (edges.length?edges.map(e=>displayID(e[1])+' ← '+displayID(e[0])).join('\n'):
              '(Không có phụ thuộc)');
          taskDepsOutput.hidden=false;
          // Server has already checked explicit dependsOrder=sequence,
          // process-only steps and fresh permission. The browser checks
          // structural consistency above and never invents run authority.
          taskWorkflowReadyID=data.available&&data.run_available?id:0;
          // Capture immutable display metadata only AFTER strict ID/DAG
          // validation. These labels never authorize execution or become
          // browser-controlled task arguments in the E2EE start request.
          preparedWorkflowSteps=taskWorkflowReadyID?{
            root:id,
            steps:order.map(stepID=>{
              const known=cachedTaskIDs.indexOf(stepID);
              return {id:stepID,
                label:known<0?'':String(cachedTaskLabels[known])
                  .replace(/[\r\n\t]/g,' ').slice(0,72)};
            })
          }:null;
          taskDepsPendingID=0;
          }
          // Obsolete but correctly authenticated responses only consume
          // their own E2EE round; never re-arm the old workflow button or
          // close the native session for a harmless local selection change.
         }
         if(endpoint==='/api/tasks-report'){
          workflowAuditRows=[];workflowAuditMore=false;
          if(data.remote_control_available!==false||!Array.isArray(data.items)||
            data.items.length>3||data.items.some((item,i)=>
              !item||typeof item!=='object'||
              !Number.isInteger(item.task_id)||item.task_id<1||item.task_id>2147483647||
              !['completed','failed','cancelled','timed_out'].includes(item.status)||
              !Number.isInteger(item.exit_code)||item.exit_code<(-1)||item.exit_code>255||
              !Number.isInteger(item.duration_ms)||item.duration_ms<0||item.duration_ms>86400000||
              !Number.isInteger(item.timestamp)||item.timestamp<1577836800||
              item.timestamp>Math.floor(Date.now()/1000)+300||
              (i>0&&item.timestamp>data.items[i-1].timestamp)||
              Object.keys(item).sort().join(',')!=='duration_ms,exit_code,status,task_id,timestamp')){
            throw new Error('Báo cáo task E2EE không hợp lệ');
          }
          tasksReportOutput.textContent=data.items.length===0?
            'Chưa có kết quả task được lưu cho tài khoản này.':
            data.items.map(item=>{
              const when=new Date(item.timestamp*1000).toLocaleString('vi-VN');
              return when+' · Task #'+item.task_id+' · '+item.status+
                ' · exit '+item.exit_code+' · '+item.duration_ms+' ms';
            }).join('\n');
          tasksReportOutput.hidden=false;
         }
         if(endpoint==='/api/tasks-workflow-audit'||
            endpoint==='/api/tasks-workflow-audit-next'){
          const next=endpoint==='/api/tasks-workflow-audit-next';
          if(next&&!workflowAuditMore){
            throw new Error('Không có trang báo cáo bước workflow tiếp theo');
          }
          if(data.remote_control_available!==false||!Array.isArray(data.items)||
            data.items.length>2||typeof data.more!=='boolean'||
            (data.more&&data.items.length!==2)||
            (next&&data.items.length===0)||
            workflowAuditRows.length+data.items.length>32&&next||
            data.items.some((item,i)=>
              !item||typeof item!=='object'||
              !['attempt,exit_code,root_task_id,status,step,task_id,timestamp,total',
                'attempt,exit_code,root_task_id,run_id,status,step,task_id,timestamp,total']
                .includes(Object.keys(item).sort().join(','))||
              (Object.prototype.hasOwnProperty.call(item,'run_id')&&
                (typeof item.run_id!=='string'||!(/^[a-f0-9]{16}$/).test(item.run_id)))||
              !Number.isInteger(item.root_task_id)||item.root_task_id<1||
              item.root_task_id>2147483647||
              !Number.isInteger(item.task_id)||item.task_id<1||
              item.task_id>2147483647||
              !Number.isInteger(item.step)||item.step<1||
              !Number.isInteger(item.total)||item.total<2||item.total>4||
              item.step>item.total||
              !Number.isInteger(item.attempt)||item.attempt<1||item.attempt>2||
              !['completed','failed','cancelled','timed_out','skipped'].includes(item.status)||
              !Number.isInteger(item.exit_code)||item.exit_code< -1||
              item.exit_code>255||(item.status==='completed'&&item.exit_code!==0)||
              (item.status==='skipped'&&
                (item.step!==item.total||item.task_id!==item.root_task_id||
                 item.attempt!==1||item.exit_code!== -1))||
              !Number.isInteger(item.timestamp)||item.timestamp<1577836800||
              item.timestamp>Math.floor(Date.now()/1000)+300||
              (i>0&&item.timestamp>data.items[i-1].timestamp)||
              (next&&i===0&&workflowAuditRows.length>0&&
               item.timestamp>workflowAuditRows[workflowAuditRows.length-1].timestamp))){
            throw new Error('Báo cáo bước workflow E2EE không hợp lệ');
          }
          if(!next){workflowAuditRows=[];}
          workflowAuditRows=workflowAuditRows.concat(data.items);
          workflowAuditMore=data.more;
          // Group only records bearing the same authenticated per-run ID.
          // Older records have no ID, so never guess that two happened in
          // one execution merely because their task IDs look alike.
          const groups=[];
          const groupByRun=new Map();
          for(let i=0;i<workflowAuditRows.length;i++){
            const item=workflowAuditRows[i];
            let group=item.run_id?groupByRun.get(item.run_id):null;
            if(group){
              if(group.root!==item.root_task_id||group.total!==item.total){
                throw new Error('Audit lượt workflow có metadata không thống nhất');
              }
            }else{
              group={run:item.run_id||'',root:item.root_task_id,
                total:item.total,rows:[]};
              groups.push(group);
              if(item.run_id)groupByRun.set(item.run_id,group);
            }
            group.rows.push(item);
          }
          const groupText=groups.map(group=>{
            const covered=new Set(group.rows.map(item=>item.step)).size;
            const header=(group.run?'Lượt '+group.run:'Lượt cũ')+
              ' · Workflow #'+group.root+' · '+covered+'/'+group.total+
              ' bước có kết quả trong snapshot';
            return header+'\n'+group.rows.map(item=>{
              const when=new Date(item.timestamp*1000).toLocaleString('vi-VN');
              return when+' · Bước '+item.step+'/'+item.total+
                ' (Task #'+item.task_id+') · Lần '+item.attempt+
                ' · '+(item.status==='skipped'?
                  'bỏ qua (không chạy process)':item.status)+
                (item.status==='skipped'?'':' · exit '+item.exit_code);
            }).join('\n');
          }).join('\n\n');
          tasksReportOutput.textContent=workflowAuditRows.length===0?
            'Không có kết quả bước workflow hợp lệ của tài khoản này.':
            'Audit workflow của tài khoản này (tối đa 32 sự kiện từ 128 audit gần nhất):\n'+
            groupText+(workflowAuditMore?'\nCòn trang; bấm Xem tiếp audit.':'');
          tasksReportOutput.hidden=false;
         }
         if(endpoint==='/api/tasks-preview'||endpoint==='/api/tasks-preview-next'){
          const appendPage=endpoint==='/api/tasks-preview-next';
          const previousVisible=appendPage?
            filteredTaskLabels().slice(taskPage*taskPageSize,(taskPage+1)*taskPageSize):null;
          if(!Number.isInteger(data.total)||data.total<0||data.total>100000||
            !Array.isArray(data.labels)||data.labels.length>24||
            data.labels.length>data.total||
            (data.total>0&&data.labels.length===0)||
            data.labels.some(x=>typeof x!=='string'||
              !x.trim()||x.length>64||/[\x00-\x1f\x7f]/.test(x))||
            (data.ids!=null&&(!Array.isArray(data.ids)||
              data.ids.length!==data.labels.length||
              data.ids.some(x=>!Number.isInteger(x)||x<0||x>2147483647)))||
            (appendPage&&(!canFetchTaskNext||data.total!==remoteTaskTotal||
              cachedTaskLabels.length+data.labels.length>data.total))){
            throw new Error('Trang task E2EE không hợp lệ');
          }
          if(!appendPage){
            taskWorkflowReadyID=0;
            preparedWorkflowSteps=null;
            taskDepsPendingID=0;
            taskDepsOutput.textContent='';taskDepsOutput.hidden=true;
            cachedTaskLabels=data.labels.slice();
            cachedTaskIDs=data.ids==null?data.labels.map(()=>0):data.ids.slice();
            inspectedTasks.clear(); // a new preview invalidates prior selections
            taskInputDrafts.clear(); // never reuse draft values against a new host task snapshot
            taskFilter.value='';
            taskFilter.disabled=false;
            taskSort.disabled=false;
            taskSort.value='host';
            taskPage=0;
          }else{
            cachedTaskLabels.push(...data.labels);
            cachedTaskIDs.push(...(data.ids==null?data.labels.map(()=>0):data.ids));
          }
          const previousDeps=appendPage?taskDepsSelect.value:'';
          taskDepsSelect.replaceChildren();
          const placeholder=document.createElement('option');
          placeholder.value='';placeholder.textContent='Chọn Task đã nhận';
          taskDepsSelect.append(placeholder);
          cachedTaskIDs.forEach((id,i)=>{
            if(!Number.isInteger(id)||id<=0)return;
            const opt=document.createElement('option');
            opt.value=String(id);opt.textContent=cachedTaskLabels[i]+' (#'+id+')';
            taskDepsSelect.append(opt);
          });
          taskDepsSelect.value=previousDeps&&
            cachedTaskIDs.includes(Number(previousDeps))?previousDeps:'';
          remoteTaskTotal=data.total;
          canFetchTaskNext=data.labels.length>0 &&
            cachedTaskLabels.length<remoteTaskTotal;
          compareCompletedTasks();
          const currentLabels=filteredTaskLabels();
          const currentVisible=currentLabels.slice(taskPage*taskPageSize,
            (taskPage+1)*taskPageSize);
          if(appendPage&&previousVisible.length===currentVisible.length&&
             previousVisible.every((entry,index)=>
               entry.id===currentVisible[index].id&&
               entry.label===currentVisible[index].label)){
            // No visible task changed: keep the existing inputs, focus and
            // click handlers. The authenticated E2EE reply updates only
            // the page controls and loaded-count indicators.
            updateTaskPageChrome(currentLabels);
          }else{
            renderTaskPage();
          }
          taskTotal.textContent=String(cachedTaskLabels.length)+' / '+
            String(remoteTaskTotal)+' task';
          taskPreviewSection.hidden=false;
        }
        if(isRun){
          const segments=isTextRun?
            [endpoint.slice('/api/tasks-run-input/'.length)]:
            endpoint.slice('/api/tasks-run/'.length).split('/');
          const id=Number(segments[0]);
          const choice=segments.length===2?Number(segments[1]):0;
          if(!Number.isInteger(id)||id<1||id>2147483647||
            (segments.length!==1&&segments.length!==2)||
            !Number.isInteger(choice)||choice<0||choice>36||
            data.task_id!==id||!Number.isInteger(data.exit_code)||
            data.exit_code<0||data.exit_code>255||
            typeof data.output!=='string'||data.output.length>192||
            typeof data.truncated!=='boolean'||
            data.remote_control_available!==false){
            throw new Error('Phản hồi chạy task E2EE không hợp lệ');
          }
          output.textContent='Task ID '+String(id)+' đã kết thúc, mã thoát: '+
            String(data.exit_code)+(data.truncated?' (output bị cắt)':'')+
            '\n'+data.output;
          rememberTaskRun(id,'ngắn',data.exit_code===0?'completed':'failed',
            data.exit_code,data.output,data.truncated);
        }
        if(isLongAction){
          let prefix='';
          for(const candidate of ['/api/tasks-workflow-start/','/api/tasks-start/','/api/tasks-start-input/',
            '/api/tasks-poll/','/api/tasks-cancel/','/api/tasks-clear/']){
            if(endpoint.startsWith(candidate)){prefix=candidate;break;}
          }
          const segment=endpoint.slice(prefix.length).split('/');
          const id=Number(segment[0]);
          if(!Number.isInteger(id)||id<1||id>2147483647||
            segment.length>2||data.task_id!==id||
            !['started','running','cancel_requested','completed','failed',
              'cancelled','timed_out','cleared'].includes(data.status)||
            !Number.isInteger(data.exit_code)||data.exit_code< -1||
            data.exit_code>255||typeof data.more!=='boolean'||
            typeof data.output!=='string'||
            !validTerminalOutput(data.output)||typeof data.truncated!=='boolean'||
            data.remote_control_available!==false){
            throw new Error('Phản hồi job dài E2EE không hợp lệ');
          }
          const fields=['step','step_total','attempt','attempt_total','step_status'];
          const present=fields.filter(key=>Object.prototype.hasOwnProperty.call(data,key));
          if(present.length!==0&&present.length!==fields.length){
            throw new Error('Tiến độ bước workflow thiếu trường E2EE');
          }
          const step=present.length?data.step:0;
          const stepTotal=present.length?data.step_total:0;
          const attempt=present.length?data.attempt:0;
          const attemptTotal=present.length?data.attempt_total:0;
          const stepStatus=present.length?data.step_status:'';
          const workflowProgress=step!==0||stepTotal!==0||
            attempt!==0||attemptTotal!==0||stepStatus!=='';
          const hasStepHistory=Object.prototype.hasOwnProperty.call(data,'step_history');
          const stepHistory=hasStepHistory?data.step_history:'';
          if(typeof stepHistory!=='string'||stepHistory.length>4||
            (stepHistory!==''&&(!workflowProgress||
              stepHistory.length!==stepTotal))){
            throw new Error('Báo cáo từng bước workflow không hợp lệ');
          }
          if(stepHistory!==''){
            const codes={running:'R',completed:'C',skipped:'S',
              failed:'F',cancelled:'X',timed_out:'T'};
            if(stepStatus==='skipped'&&
              (step!==stepTotal||
               !['running','completed'].includes(data.status)||!stepHistory)){
              throw new Error('Bước bỏ qua chỉ hợp lệ ở cuối workflow thành công');
            }
            for(let i=0;i<stepHistory.length;i++){
              const expected=i<step-1?'C':i===step-1?codes[stepStatus]:'-';
              const finalCleanupFailure=stepStatus!=='skipped'&&
                i===step-2&&step===stepTotal&&stepHistory[i]==='F';
              if(stepHistory[i]!==expected&&!finalCleanupFailure){
                throw new Error('Báo cáo từng bước workflow sai trạng thái');
              }
            }
          }
          if(workflowProgress&&
            (!Number.isInteger(step)||!Number.isInteger(stepTotal)||
             !Number.isInteger(attempt)||!Number.isInteger(attemptTotal)||
             step<1||stepTotal<2||stepTotal>4||step>stepTotal||
             attempt<1||attemptTotal<1||attemptTotal>2||
             attempt>attemptTotal||
             !['running','completed','skipped','failed','cancelled','timed_out'].includes(stepStatus)||
             ['started','cleared','cancel_requested'].includes(data.status)||
             !validTerminalOutput(data.output,64))){
            throw new Error('Tiến độ bước workflow E2EE không hợp lệ');
          }
          if(!workflowProgress&&present.length&&
            (step!==0||stepTotal!==0||attempt!==0||attemptTotal!==0||
             stepStatus!=='')){
            throw new Error('Tiến độ workflow không hợp lệ');
          }
          const detailsRaw=Object.prototype.hasOwnProperty.call(data,'step_details')?
            data.step_details:'';
          if(typeof detailsRaw!=='string'||detailsRaw.length>39||
            (detailsRaw!==''&&(stepHistory===''||!workflowProgress))){
            throw new Error('Mã thoát/thời gian workflow không hợp lệ');
          }
          let details=[];
          if(detailsRaw!==''){
            details=detailsRaw.split('|');
            if(details.length!==stepTotal){
              throw new Error('Số bước kết quả E2EE không khớp');
            }
            for(let i=0;i<details.length;i++){
              const code=stepHistory[i];
              const value=details[i];
              if(code==='R'||code==='-'||code==='S'){
                if(value!=='-')throw new Error('Bước đang chờ không thể có kết quả');
              }else{
                if(!/^-?(0|[1-9][0-9]{0,2}):(0|[1-9][0-9]{0,4})$/.test(value)){
                  throw new Error('Mã thoát/thời gian bước sai định dạng');
                }
                const parts=value.split(':');
                const exit=Number(parts[0]);
                const ms=Number(parts[1]);
                if(exit< -1||exit>255||ms>90000||
                  String(exit)!==parts[0]||String(ms)!==parts[1]||
                  (code==='C'&&exit!==0)){
                  throw new Error('Mã thoát/thời gian bước vượt giới hạn');
                }
              }
            }
          }
          // Only the explicit start endpoint can create a local job. A
          // forged/stale "started" response from Poll must not reset the
          // already-authenticated output, history, or selected Task ID.
          const launching=prefix==='/api/tasks-workflow-start/'||
            prefix==='/api/tasks-start/'||prefix==='/api/tasks-start-input/';
          const clearing=prefix==='/api/tasks-clear/';
          const cancelling=prefix==='/api/tasks-cancel/';
          const polling=prefix==='/api/tasks-poll/';
          const finalStates=['completed','failed','cancelled','timed_out'];
          // The Agent codec requires in-flight replies to carry exit=-1.
          // A cancellation ACK is not a poll result and cannot claim
          // buffered output; browser-side validation must match the
          // typed AEAD protocol even if the localhost bridge is corrupted.
          if((['started','running','cancel_requested','cleared'].includes(data.status)&&
              data.exit_code!==-1)||
             (['started','cancel_requested','cleared'].includes(data.status)&&
              (data.output!==''||data.more||data.truncated))||
             (cancelling&&data.status!=='cancel_requested')||
             (polling&&['cancel_requested','started','cleared'].includes(data.status))){
            throw new Error('Trạng thái/exit code job dài E2EE không hợp lệ');
          }
          if(launching){
            if(longTaskID!==0||data.status==='cleared'||
              data.status==='cancel_requested'){
              throw new Error('Phản hồi khởi chạy job không hợp lệ');
            }
          }else{
            if(longTaskID!==id||data.status==='started'||
              (data.status==='cleared')!==clearing||
              (finalStates.includes(longTaskState)&&
               !finalStates.includes(data.status)&&!clearing)||
              (finalStates.includes(longTaskState)&&
               finalStates.includes(data.status)&&data.status!==longTaskState)){
              throw new Error('Phản hồi job không khớp vòng đời đã xác minh');
            }
          }
          if(launching){
            activeWorkflowSteps=prefix==='/api/tasks-workflow-start/'&&
              preparedWorkflowSteps&&preparedWorkflowSteps.root===id?
              preparedWorkflowSteps.steps.map(node=>({id:node.id,label:node.label})):null;
            preparedWorkflowSteps=null;
            longTaskPollEpoch++;longTaskPollScheduled=false;
            longTaskID=id;longTaskText='';
            longTaskCancelPending=false;
            longTaskAuto=!finalStates.includes(data.status);
            longTaskHistorySerial=rememberTaskRun(
              id,'dài',data.status,data.exit_code,'',data.truncated);
          }
          if(data.output){
            longTaskText=(longTaskText+data.output).slice(0,16384);
          }
          if(prefix==='/api/tasks-cancel/'&&data.status==='cancel_requested'){
            longTaskCancelPending=true;
          }
          longTaskState=data.status;
          longTaskMore=data.more;
          if(data.status!=='cleared'){
            updateLongTaskHistory(id,data.status,data.exit_code,data.truncated,longTaskText);
          }
          if(data.status==='cleared'){
            longTaskPollEpoch++;longTaskPollScheduled=false;
            longTaskHistorySerial=0;
            longTaskID=0;
            activeWorkflowSteps=null;
            longTaskText='';longTaskMore=false;
            longTaskCancelPending=false;
            longTaskAuto=false;
            // The host slot is now gone: the final retained outcome is
            // ordinary history and may be explicitly deleted by its owner.
            renderTaskRunHistory();
          }
          const historyLabels={'-':'Chưa chạy',R:'Đang chạy',
            C:'Hoàn tất',S:'Bỏ qua',F:'Lỗi',X:'Đã hủy',T:'Hết thời gian'};
          const historyLines=stepHistory?
            Array.from(stepHistory).map((code,i)=>
              'Bước '+String(i+1)+': '+historyLabels[code]+
              (details[i]&&details[i]!=='-'?
                ' (exit='+details[i].split(':')[0]+
                ', '+details[i].split(':')[1]+' ms)':'')).join(' · '):'';
          const verifiedStepMap=workflowProgress&&activeWorkflowSteps&&
            activeWorkflowSteps.length===stepTotal?activeWorkflowSteps:null;
          // Show real host-issued Task IDs and names alongside typed
          // workflow progress, never guess names from a refreshed list.
          const stepMapLine=verifiedStepMap?verifiedStepMap.map((node,i)=>
            'Bước '+String(i+1)+' → Task #'+String(node.id)+
            (node.label?' · '+node.label:'')).join(' | '):'';
          const currentStepNode=verifiedStepMap?verifiedStepMap[step-1]:null;
          renderLongTaskTranscript('Task '+String(id)+' · '+data.status+
            ' · exit='+String(data.exit_code)+
            (workflowProgress?' · Bước '+String(step)+'/'+String(stepTotal)+
              ' · Lần '+String(attempt)+'/'+String(attemptTotal)+
              ' · '+stepStatus:'')+
            (currentStepNode?' · Task #'+String(currentStepNode.id)+
              (currentStepNode.label?' · '+currentStepNode.label:''):'')+
            (data.truncated?' · output vượt giới hạn':'')+
            (longTaskMore?' · còn output chưa nhận':'')+
            (historyLines?'\n'+historyLines:'')+
            (stepMapLine?'\n'+stepMapLine:'')+
            '\n'+longTaskText,launching||clearing);
          output.textContent='Trạng thái job E2EE: '+data.status+
            ' (Task ID '+String(id)+').'+
            (longTaskCancelPending&&
             ['running','cancel_requested'].includes(longTaskState)?
              ' Đã yêu cầu hủy; đang chờ tiến trình kết thúc.':'')+
            (longTaskID>0&&longTaskAuto&&
              (longTaskMore||['started','running','cancel_requested'].includes(longTaskState))&&
              !mayAutoPollLongTask()?
              ' Đã tạm dừng tự nhận để giữ lượt cho hủy/dọn job.':'');
          // Job output/status does not change the task list. Rebuilding the
          // controls here would erase the operator's in-progress input/focus.
          // The finally render() updates run/cancel availability in place.
        }
        if(endpoint.startsWith('/api/tasks-inspect/')){
          const id=Number(endpoint.slice('/api/tasks-inspect/'.length));
          if(!Number.isInteger(id)||id<1||id>2147483647||
            data.task_id!==id||data.eligible!==true||
            data.remote_control_available!==false){
            throw new Error('Kết quả xác minh task không đúng ID đã chọn');
          }
          const inputID=data.input_id==null?'':data.input_id;
          const choices=data.choices==null?[]:data.choices;
          const inputType=data.input_type==null?'':data.input_type;
          const inputID2=data.input_id_2==null?'':data.input_id_2;
          const choices2=data.choices_2==null?[]:data.choices_2;
          if(typeof inputID2!=='string'||
            !Array.isArray(choices2)||choices2.length>6||
            (inputID2===''&&choices2.length!==0)||
            (inputID2!==''&&(!/^[A-Za-z0-9_.-]{1,32}$/.test(inputID2)||
              inputID2===inputID||choices2.length===0))||
            (inputID2!==''&&(inputID===''||choices.length===0||
              inputType!==''))||
            choices2.some(value=>typeof value!=='string'||
              !/^[A-Za-z0-9][A-Za-z0-9 _./:@+=-]{0,31}$/.test(value))||
            new Set(choices2).size!==choices2.length||
            typeof inputType!=='string'||
            (inputType!==''&&inputType!=='promptString')||
            (inputType==='promptString'&&
              (choices.length!==0||inputID===''))||
            (inputType===''&&inputID!==''&&choices.length===0)||
            typeof inputID!=='string'||
            !Array.isArray(choices)||choices.length>6||
            (inputID===''&&choices.length!==0)||
            (inputID!==''&&!/^[A-Za-z0-9_.-]{1,32}$/.test(inputID))||
            choices.some(value=>typeof value!=='string'||
              !/^[A-Za-z0-9][A-Za-z0-9 _./:@+=-]{0,31}$/.test(value))||
            new Set(choices).size!==choices.length){
            throw new Error('Task có tham số không thuộc danh sách được host cho phép');
          }
          inspectedTasks.set(id,{inputID,choices,inputType,inputID2,choices2});
          taskInputDrafts.delete(id); // metadata may differ from the prior inspection
          if(!taskPreviewSection.hidden)renderTaskPage();
          output.textContent='Task ID '+String(id)+
            ' đã được xác minh; '+(inputType==='promptString'?
             'nhập mã tham số ngắn rồi bấm Chạy task.':
             choices2.length?'chọn cả hai danh sách rồi bấm Chạy task.':
             choices.length?'chọn một giá trị có sẵn rồi bấm Chạy task.':
             'có thể chạy task ngắn qua E2EE.');
        }
        if(endpoint==='/api/ping'){
          if(!Number.isInteger(data.round_trip_ms)||data.round_trip_ms<0||
            data.round_trip_ms>16000)throw new Error('Kết quả đo độ trễ không hợp lệ');
          pingLatency.textContent=String(data.round_trip_ms)+' ms';
        }
        if(['/api/git-stage-tracked','/api/git-unstage-all','/api/git-stage-file',
          '/api/git-unstage-file','/api/git-pull','/api/git-fetch',
          '/api/git-commit','/api/git-push'].includes(endpoint)){
          gitPatchText='';gitPatchPage=0;gitPatchMore=false;
          gitPatchOutput.textContent='';gitPatchOutput.hidden=true;
        }
        if(endpoint==='/api/git-status'){renderGit(data.status);}
        if(['/api/git-patch-start','/api/git-patch-next',
           '/api/git-patch-cached-start','/api/git-patch-cached-next'].includes(endpoint)){
          const fresh=endpoint.endsWith('-start');
          const mode=endpoint.includes('-cached-')?'cached':'head';
          if(!fresh&&mode!==gitPatchMode)throw new Error('Trang diff sai loại');
          if(data.remote_control_available!==false||
            !Number.isInteger(data.page)||data.page<1||data.page>256||
            data.page!==(fresh?1:gitPatchPage+1)||
            typeof data.patch!=='string'||data.patch.length<1||
            data.patch.length>320||typeof data.more!=='boolean'||
            Array.from(data.patch).some(ch=>{
              if(ch==='\n'||ch==='\t')return false;
              return /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u.test(ch);
            })||
            (fresh?data.patch.length>16384:
              gitPatchText.length+data.patch.length>16384)){
            throw new Error('Trang Git diff E2EE không hợp lệ');
          }
          gitPatchText=fresh?data.patch:gitPatchText+data.patch;
          gitPatchMode=mode;
          gitPatchPage=data.page;gitPatchMore=data.more;
          gitPatchOutput.hidden=false;
          gitPatchOutput.textContent=gitPatchText;
          output.textContent=(mode==='cached'?'Staged Git diff':'Git diff')+' trang '+String(data.page)+
            (data.more?' · Còn trang tiếp theo.':' · Đã đọc hết diff.')+
            (data.more&&budget<=2?
              ' Lưu ý: Phiên gần hết lượt E2EE; diff CHƯA đầy đủ. '+
              'Phiên mới phải bắt đầu lại từ trang đầu.':'');
        }
        if(endpoint==='/api/git-diff'){
          if(data.remote_control_available!==false||typeof data.summary!=='string'||
            data.summary.length===0||data.summary.length>240||
            Array.from(data.summary).some(ch=>{const c=ch.codePointAt(0);return (c<32&&c!==10)||c===127;})){
            throw new Error('Phản hồi Git diff E2EE không hợp lệ');
          }
          output.textContent=data.summary;
        }
        if(endpoint==='/api/git-branch-delete-review'||endpoint==='/api/git-branch-delete'){
          const safeCurrent=typeof data.current==='string'&&
            /^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$/.test(data.current)&&
            data.current!=='HEAD'&&!data.current.includes('..')&&
            !data.current.includes('//')&&!data.current.includes('/.')&&
            !data.current.endsWith('.')&&!data.current.endsWith('/')&&
            !data.current.split('/').some(p=>p==='HEAD'||p.endsWith('.lock'));
          const safeName=typeof data.branch==='string'&&
            /^taskdeck\/[A-Za-z0-9][A-Za-z0-9._-]{0,39}$/.test(data.branch)&&
            !['HEAD','main','master','develop','trunk','release'].includes(data.branch.slice(9))&&
            !data.branch.endsWith('.')&&!data.branch.endsWith('.lock')&&
            !data.branch.includes('..');
          if(data.remote_control_available!==false||!safeCurrent||!safeName||
            data.current===data.branch||
            typeof data.head!=='string'||!/^[0-9a-f]{40}$/.test(data.head)){
            throw new Error('Kết quả xoá nhánh TaskDeck không hợp lệ');
          }
          const plan={current:data.current,branch:data.branch,head:data.head};
          if(endpoint==='/api/git-branch-delete-review'){
            gitDeletePreview=plan;
            gitDeletePending=null;
            output.textContent='Xem trước chỉ đọc: nhánh '+plan.branch+
              ' sẽ bị XOÁ (cục bộ), đang đứng tại '+plan.current+
              ', cùng HEAD '+plan.head.slice(0,12)+
              '. Không xóa commit, không force, không checkout. Yêu cầu xác nhận riêng.';
          }else{
            if(!gitDeletePending||gitDeletePending.current!==plan.current||
              gitDeletePending.branch!==plan.branch||
              gitDeletePending.head!==plan.head){
              gitDeletePending=null;
              throw new Error('Kết quả xoá nhánh khác bản đã xác nhận');
            }
            invalidateGitBranchReview();
            output.textContent='Đã xoá nhánh dư thừa cục bộ '+plan.branch+
              '. Nhánh hiện tại '+plan.current+' và HEAD '+
              plan.head.slice(0,12)+' vẫn được giữ nguyên.';
          }
         }
         if(endpoint==='/api/git-switch-review'||endpoint==='/api/git-switch-same-head'){
          const safeFrom=typeof data.from==='string'&&
            /^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$/.test(data.from)&&
            !data.from.includes('..')&&!data.from.includes('//')&&
            !data.from.includes('/.')&&!data.from.endsWith('.')&&
            !data.from.endsWith('/')&&data.from!=='HEAD'&&
            !data.from.endsWith('.lock')&&!data.from.split('/').includes('HEAD')&&
            !data.from.split('/').some(p=>p.endsWith('.lock'));
          const safeTarget=typeof data.branch==='string'&&
            /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(data.branch)&&
            data.branch!=='HEAD'&&!data.branch.endsWith('.')&&
            !data.branch.endsWith('.lock')&&!data.branch.includes('..');
          if(data.remote_control_available!==false||!safeFrom||!safeTarget||
            data.from===data.branch||
            typeof data.head!=='string'||!/^[0-9a-f]{40}$/.test(data.head)){
            throw new Error('Kết quả đổi nhánh cùng HEAD không hợp lệ');
          }
          const plan={from:data.from,branch:data.branch,head:data.head};
          if(endpoint==='/api/git-switch-review'){
            gitSwitchPreview=plan;
            gitSwitchPending=null;
            output.textContent='Đã xem trước trên host: '+plan.from+' → '+plan.branch+
              ', cùng commit '+plan.head.slice(0,12)+
              '. Nếu đồng ý, bấm Chuyển nhánh cùng HEAD. Host sẽ kiểm tra lại trước khi thực hiện.';
          }else{
            if(!gitSwitchPending||
              gitSwitchPending.from!==plan.from||
              gitSwitchPending.branch!==plan.branch||
              gitSwitchPending.head!==plan.head){
              gitSwitchPending=null;
              throw new Error('Git switch khác với bản xem trước');
            }
            invalidateGitBranchReview();invalidateGitPatchReview();
            output.textContent='Đã chuyển nhánh cục bộ từ '+plan.from+
              ' sang '+plan.branch+' tại HEAD '+plan.head.slice(0,12)+
              '. Không fetch, reset hoặc thay đổi commit.';
          }
         }
         if(endpoint==='/api/git-branch-create'){
          if(data.remote_control_available!==false||
            typeof data.branch!=='string'||
            !/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(data.branch)||
            data.branch==='HEAD'||data.branch.endsWith('.')||
            data.branch.endsWith('.lock')||data.branch.includes('..')||
            typeof data.head!=='string'||!/^[0-9a-f]{40}$/.test(data.head)){
            throw new Error('Kết quả tạo nhánh host không hợp lệ');
          }
          invalidateGitBranchReview();invalidateGitPatchReview();
          output.textContent='Đã tạo nhánh cục bộ '+data.branch+
            ' tại '+data.head.slice(0,12)+'. HEAD hiện tại không đổi.';
        }
        if(endpoint==='/api/git-tracking-start'||endpoint==='/api/git-tracking-next'){
          const fresh=endpoint==='/api/git-tracking-start';
          const header='GIT_TRACKING_V1\n';
          if(data.remote_control_available!==false||
            !Number.isInteger(data.page)||data.page!==(fresh?1:gitTrackingPage+1)||
            data.page>32||typeof data.refs!=='string'||data.refs.length>360||
            typeof data.more!=='boolean'||
            (fresh?!data.refs.startsWith(header):!gitTrackingMore||data.refs.startsWith(header))||
            !data.refs.endsWith('\n')||gitTrackingText.length+data.refs.length>4096){
            throw new Error('Trang Git tracking không hợp lệ');
          }
          const body=fresh?data.refs.slice(header.length):data.refs;
          const lines=body===''?[]:body.slice(0,-1).split('\n');
          if((body===''&&!fresh)||(!lines.length&&data.more)){
            throw new Error('Trang Git tracking rỗng hoặc không hoàn chỉnh');
          }
          let prev=fresh?'':gitTrackingLast;
          for(const line of lines){
            const at=line.lastIndexOf(' '),name=line.slice(0,at),oid=line.slice(at+1);
            const parts=name.split('/');
            if(at<=0||parts.length<2||parts.length>7||name.length>160||
              name.endsWith('/HEAD')||!/^([0-9a-f]{40}|[0-9a-f]{64})$/.test(oid)||
              parts.some(p=>!(/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(p))||
                /[._-]$/.test(p)||p.includes('..')||p.endsWith('.lock'))||
              (prev!==''&&prev>=name)){
              throw new Error('Tên Git tracking hoặc commit OID không hợp lệ');
            }
            prev=name;
          }
          gitTrackingLast=prev;
          gitTrackingText=fresh?data.refs:gitTrackingText+data.refs;
          gitTrackingPage=data.page;gitTrackingMore=data.more;
          output.textContent=gitTrackingText+(data.more?'(còn trang tiếp theo)':'');
        }
        if(endpoint==='/api/git-remotes'){
          if(data.remote_control_available!==false||!validGitRemotesReport(data.remotes)){
            throw new Error('Phản hồi tên Git remote E2EE không hợp lệ');
          }
          output.textContent=data.remotes==='GIT_REMOTES_V1\n'?
            'Không có Git remote đã cấu hình.':data.remotes;
        }
        if(endpoint==='/api/git-branches'){
          if(data.remote_control_available!==false||typeof data.branches!=='string'||
            data.branches.length===0||data.branches.length>240||
            Array.from(data.branches).some(ch=>{
              const c=ch.codePointAt(0);
              return (c<32&&c!==10)||c===127;
            })){
            throw new Error('Phản hồi Git branches E2EE không hợp lệ');
          }
          output.textContent=data.branches;
        }
        if(endpoint==='/api/git-log'){
          if(data.remote_control_available!==false||typeof data.log!=='string'||
            data.log.length===0||data.log.length>240||
            Array.from(data.log).some(ch=>{const c=ch.codePointAt(0);return (c<32&&c!==10)||c===127;})){
            throw new Error('Phản hồi Git log E2EE không hợp lệ');
          }
          output.textContent=data.log;
        }
        if(isGitFile){
          const want=endpoint==='/api/git-stage-file'?'git.stage_file':'git.unstage_file';
          if(data.remote_control_available!==false||data.operation!==want||
            typeof data.head!=='string'||!/^[0-9a-f]{40}$/.test(data.head)){
            throw new Error('Phản hồi Git staging theo file không hợp lệ');
          }
          gitRawSnapshot='';gitFileSelect.replaceChildren();gitFileSelect.value='';
          gitFileSelect.disabled=true;
          gitSummary.hidden=true;
          gitRawSnapshot='';gitFileSelect.replaceChildren();gitFileSelect.value='';
          gitFileSelect.disabled=true;
          gitRows=[];gitFiles.replaceChildren();gitBranch.textContent='';
          gitSnapshotCount=0;gitSnapshotBranch='';
          gitDeltaSummary.textContent='';gitDeltaList.replaceChildren();
          invalidateGitPatchReview();invalidateGitBranchReview();
          output.textContent=want==='git.stage_file'?
            'Đã stage đúng file đã chọn. Đọc lại Git Status để tiếp tục.':
            'Đã unstage đúng file đã chọn. Working tree không thay đổi.';
        }
        if(endpoint==='/api/git-stage-tracked'||endpoint==='/api/git-unstage-all'){
          const want=endpoint==='/api/git-stage-tracked'?'git.stage_tracked':'git.unstage_all';
          if(data.remote_control_available!==false||data.operation!==want||
            typeof data.head!=='string'||!/^[0-9a-f]{40}$/.test(data.head)){
            throw new Error('Kết quả Git staging E2EE không hợp lệ');
          }
          gitSummary.hidden=true;
          gitRawSnapshot='';gitFileSelect.replaceChildren();gitFileSelect.value='';
          gitFileSelect.disabled=true;
          gitRows=[];gitFiles.replaceChildren();gitBranch.textContent='';
          gitSnapshotCount=0;gitSnapshotBranch='';
          gitDeltaSummary.textContent='';gitDeltaList.replaceChildren();
          invalidateGitPatchReview();invalidateGitBranchReview();
          output.textContent=want==='git.stage_tracked'?
            'Đã stage thay đổi của các file được Git theo dõi. Không tự thêm untracked.':
            'Đã unstage toàn bộ index. Working tree không thay đổi.';
        }
        if(endpoint==='/api/git-fetch'){
          const sha=/^[0-9a-f]{40}$/;
          if(data.remote_control_available!==false||
            typeof data.head!=='string'||!sha.test(data.head)||
            typeof data.before!=='string'||!sha.test(data.before)||
            typeof data.after!=='string'||!sha.test(data.after)||
            typeof data.updated!=='boolean'||
            data.updated!==(data.before!==data.after)){
            throw new Error('Kết quả Git fetch E2EE không hợp lệ');
          }
          gitSummary.hidden=true;
          gitRawSnapshot='';gitFileSelect.replaceChildren();gitFileSelect.value='';
          gitFileSelect.disabled=true;
          gitRows=[];gitFiles.replaceChildren();gitBranch.textContent='';
          gitSnapshotCount=0;gitSnapshotBranch='';
          gitDeltaSummary.textContent='';gitDeltaList.replaceChildren();
          output.textContent=data.updated?
            'Đã fetch upstream: '+data.after.slice(0,12)+
              '. HEAD cục bộ vẫn là '+data.head.slice(0,12)+'.':
            'Upstream không đổi ('+data.after.slice(0,12)+
              '). HEAD cục bộ vẫn là '+data.head.slice(0,12)+'.';
        }
        if(endpoint==='/api/git-pull-changes'){
          const oid=/^[0-9a-f]{40}$/;
          if(data.remote_control_available!==false||
            typeof data.head!=='string'||!oid.test(data.head)||
            typeof data.upstream_head!=='string'||!oid.test(data.upstream_head)||
            !Array.isArray(data.files)||data.files.length>6||
            (data.head===data.upstream_head&&data.files.length!==0)||
            data.files.some((v,i)=>{
              if(typeof v!=='string'||v.length<4||v.length>50||
                !['A:','M:','D:'].includes(v.slice(0,2)))return true;
              const segments=v.slice(2).split('/');
              if(segments.length>4||segments.some(seg=>
                !/^[A-Za-z0-9][A-Za-z0-9_.-]*$/.test(seg)))return true;
              return i>0&&data.files[i-1].slice(2)>=v.slice(2);
            })){
            throw new Error('Danh sách file Git Pull E2EE không hợp lệ');
          }
          output.textContent='Review file fast-forward cục bộ (chỉ đọc): '+
            data.head.slice(0,12)+' → '+data.upstream_head.slice(0,12)+'\n'+
            (data.files.length?data.files.map(v=>
              (v[0]==='A'?'Thêm: ':v[0]==='M'?'Sửa: ':'Xóa: ')+v.slice(2)).join('\n'):
              'Không có file thay đổi trong tree.')+
            '\nChỉ xem trước, chưa fetch/pull hay thay đổi Git.';
         }
         if(endpoint==='/api/git-pull-review'){
          const sha=/^[0-9a-f]{40}$/;
          if(data.remote_control_available!==false||
            typeof data.head!=='string'||!sha.test(data.head)||
            typeof data.upstream_head!=='string'||!sha.test(data.upstream_head)||
            !['up-to-date','fast-forward'].includes(data.status)||
            (data.status==='up-to-date')!==(data.head===data.upstream_head)){
            throw new Error('Phản hồi xem trước Git pull E2EE không hợp lệ');
          }
          output.textContent=data.status==='fast-forward'?
            'Upstream đã fetch sẵn có thể fast-forward: '+data.head.slice(0,12)+
              ' → '+data.upstream_head.slice(0,12)+
              '. Chỉ xem trước; chưa fetch/pull hay ghi bất kỳ ref nào.':
            'HEAD đã đồng bộ với upstream cục bộ ('+data.head.slice(0,12)+
              '). Chỉ xem trước; chưa fetch/pull.';
        }
        if(endpoint==='/api/git-pull'){
          const sha=/^[0-9a-f]{40}$/;
          if(data.remote_control_available!==false||
            typeof data.before!=='string'||!sha.test(data.before)||
            typeof data.after!=='string'||!sha.test(data.after)||
            typeof data.updated!=='boolean'||
            data.updated!==(data.before!==data.after)){
            throw new Error('Kết quả Git pull E2EE không hợp lệ');
          }
          // A successful Git write invalidates any locally cached status.
          gitSummary.hidden=true;
          gitRawSnapshot='';gitFileSelect.replaceChildren();gitFileSelect.value='';
          gitFileSelect.disabled=true;
          gitRows=[];gitFiles.replaceChildren();gitBranch.textContent='';
          gitSnapshotCount=0;gitSnapshotBranch='';
          gitDeltaSummary.textContent='';gitDeltaList.replaceChildren();
          if(data.updated){
            invalidateGitPatchReview();invalidateGitBranchReview();
            invalidateTaskPreviewAfterPull();
          }
          output.textContent=data.updated?
            'Git pull fast-forward thành công: '+data.before.slice(0,12)+
            ' → '+data.after.slice(0,12)+'. Đọc Git status để lấy trạng thái mới.':
            'Git pull thành công: HEAD không thay đổi ('+data.after.slice(0,12)+').';
        }
        if(endpoint==='/api/git-commit'){
          const sha=/^[0-9a-f]{40}$/;
          if(data.remote_control_available!==false||
            typeof data.before!=='string'||!sha.test(data.before)||
            typeof data.after!=='string'||!sha.test(data.after)||
            data.before===data.after){
            throw new Error('Kết quả Git commit E2EE không hợp lệ');
          }
          gitCommitMessage.value='';
          gitSummary.hidden=true;
          gitRawSnapshot='';gitFileSelect.replaceChildren();gitFileSelect.value='';
          gitFileSelect.disabled=true;
          gitRows=[];gitFiles.replaceChildren();gitBranch.textContent='';
          gitSnapshotCount=0;gitSnapshotBranch='';
          gitDeltaSummary.textContent='';gitDeltaList.replaceChildren();
          invalidateGitPatchReview();invalidateGitBranchReview();
          output.textContent='Đã commit nội dung đã stage trên host: '+
            data.after.slice(0,12)+'. Đọc lại Git status để cập nhật.';
        }
        if(endpoint==='/api/git-push'){
          const sha=/^[0-9a-f]{40}$/;
          if(data.remote_control_available!==false||
            typeof data.head!=='string'||!sha.test(data.head)){
            throw new Error('Kết quả Git push E2EE không hợp lệ');
          }
          gitSummary.hidden=true;
          gitRawSnapshot='';gitFileSelect.replaceChildren();gitFileSelect.value='';
          gitFileSelect.disabled=true;
          gitRows=[];gitFiles.replaceChildren();gitBranch.textContent='';
          gitSnapshotCount=0;gitSnapshotBranch='';
          gitDeltaSummary.textContent='';gitDeltaList.replaceChildren();
          output.textContent='Git push đã thành công trên upstream được cấu hình sẵn: '+
            data.head.slice(0,12)+'. Đọc lại Git status để cập nhật trạng thái.';
        }
        if(endpoint.startsWith('/api/terminal-')){
          const isPTYOpen=endpoint==='/api/terminal-open';
          const isPTYPoll=endpoint==='/api/terminal-poll';
          const isPTYClose=endpoint==='/api/terminal-close';
          const isPTYWrite=endpoint==='/api/terminal-write';
          const isPTYResize=endpoint==='/api/terminal-resize';
          // Only Poll can return captured output or report "exited".
          // Open/Write/Resize acknowledge running; Close acknowledges closed.
          // Poll never closes/clears a host PTY lease and an exited shell
          // cannot spontaneously become running again.
          const expectsRunning=isPTYOpen||isPTYWrite||isPTYResize;
          if(data.remote_control_available!==false||
            typeof data.state!=='string'||
            !['running','exited','closed'].includes(data.state)||
            typeof data.output!=='string'||
            typeof data.truncated!=='boolean'||typeof data.more!=='boolean'||
            !validTerminalOutput(data.output)||
            (expectsRunning&&data.state!=='running')||
            (isPTYClose&&data.state!=='closed')||
            (isPTYPoll&&(terminalState==='closed'||
              data.state==='closed'||
              (terminalState==='exited'&&data.state!=='exited')))||
            (!isPTYPoll&&(data.output!==''||data.more||data.truncated))){
            throw new Error('Kết quả Terminal E2EE không hợp lệ');
          }
          terminalState=data.state;
          if(isPTYClose){
            terminalAutoEpoch++;terminalAutoScheduled=false;
            terminalAutoPending=false;
          }else if(isPTYOpen||isPTYWrite||isPTYResize){
            terminalAutoPending=true;
          }else if(isPTYPoll){
            terminalAutoPending=data.more;
          }
          if(endpoint==='/api/terminal-open'){
            terminalTranscript='';
            terminalOutput.textContent='';
          }
          if(data.output){
            terminalTranscript=(terminalTranscript+data.output).slice(-8192);
            terminalOutput.textContent=terminalTranscript;
          }
          if(endpoint==='/api/terminal-close')terminalInput.value='';
          terminalStateText.textContent=
            terminalState==='running'?'Terminal đang chạy (giới hạn 90 giây).':
            terminalState==='exited'?'Shell đã thoát. Có thể đọc output còn lại hoặc đóng phiên.':
            'Terminal đã đóng.';
          if(data.truncated)terminalStateText.textContent+=' Output đã đạt giới hạn capture 16 KiB.';
          if(data.more)terminalStateText.textContent+=' Còn output để đọc.';
          output.textContent='Terminal '+terminalState+' · PTY host đã xác thực qua E2EE.';
        }
        // Other reads must not erase locally verified Git data when navigating.
        // The cached result is a non-authoritative display, never a permission.
        if(budget===0)output.textContent+='\n\nĐã sử dụng hết 16 yêu cầu. Hãy tạo phiên được xác thực mới.';
      }
    }catch(error){
      if(isClose||!closed)output.textContent=String(error.message||error);
      closed=true; // fail closed, never switch to a PHP page or direct HTTP.
    }finally{
      if(isClose)closing=false;
      if(isRun)taskRunning=false;
      busy=false;render();updateSessionClock();
      if(!closed){
        scheduleLongTaskPoll();
        scheduleTerminalAutoPoll();
      }
    }
  }
  async function loadLocalSession(){
    if(!verified||busy||closed||closing||terminated)return;
    const observedEpoch=actionEpoch;
    const observedStatus=++sessionStatusEpoch;
    try{
      const response=await fetch('/api/session',{
        method:'POST',mode:'same-origin',credentials:'omit',cache:'no-store',
        headers:{'X-TaskDeck-Local-Capability':capability}
      });
      if(!response.ok)throw new Error('Phiên E2EE đã đóng (HTTP '+response.status+')');
      const data=await response.json();
      // Local status may race with an E2EE action or Disconnect.
      // Do not overwrite a newer action's budget or re-open a closed GUI.
      if(observedStatus!==sessionStatusEpoch||observedEpoch!==actionEpoch||busy||closed||closing||terminated)return;
      if(data.ok!==true||typeof data.closed!=='boolean'||data.closed||
        !Number.isInteger(data.remaining)||data.remaining<0||data.remaining>16||
        !Number.isInteger(data.expires_in_seconds)||data.expires_in_seconds<0||
        data.expires_in_seconds>300)throw new Error('Metadata phiên không hợp lệ');
      if(typeof data.device_id!=='string'||typeof data.username!=='string'||
        (data.device_id!==''&&!/^[a-f0-9]{64}$/.test(data.device_id))||
        data.username.length>128)throw new Error('Danh tính Agent không hợp lệ');
      targetDevice.textContent=data.device_id||'Chưa có thông tin';
      targetUser.textContent=data.username||'Chưa có thông tin';
      budget=data.remaining;
      expiresAt=Date.now()+data.expires_in_seconds*1000;
      updateSessionClock();
    }catch(error){
      if(observedStatus===sessionStatusEpoch&&observedEpoch===actionEpoch&&!busy&&!closed&&!closing&&!terminated){
        closed=true;
        output.textContent=String(error.message||error);
        updateSessionClock();
      }
    }finally{render();}
  }
  activate.addEventListener('click',()=>{
    if(activationConsumed||verified||closed||closing||terminated){
      localToken.value='';
      return; // do not re-enable a revoked one-use native session
    }
    const candidate=String(localToken.value||'').trim();
    localToken.value='';
    if(!/^[a-f0-9]{64}$/.test(candidate)){
      output.textContent='Mã không đúng. Hãy lấy mã 64 ký tự trực tiếp từ terminal client TaskDeck tin cậy.';
      return;
    }
    activationConsumed=true;
    capability=candidate;
    verified=true;
    featuresKnown=false;
    available={session_ping:false,project_info:false,files_root:false,tasks_preview:false,
      task_run:false,task_long:false,git_status:false,git_log:false,git_branches:false,git_branch_create:false,git_diff:false,git_patch:false,git_fetch:false,git_stage_tracked:false,git_unstage_all:false,git_stage_file:false,git_unstage_file:false,git_pull:false,git_commit:false,git_push:false,terminal:false};
    featureSummary.textContent='Chưa kiểm tra. Bấm Kiểm tra chức năng để biết những chức năng được cấp phép.';
    activation.hidden=true;
    closed=false;terminated=false;closing=false;
    render();
    loadLocalSession();
  });
  // This exact in-binary vocabulary only delegates to the SAME fixed
  // endpoints as the reviewed GUI buttons. Never turn input into a URL,
  // Git argument, script, task ID, or shell command. No command history.
  function runQuickAction(){
    const raw=String(quickCommand.value||'');
    quickCommand.value='';
    if(!verified||closed||terminated||closing||busy)return;
    if(raw.length>64){
      quickHint.textContent='Lệnh quá dài. Không gửi yêu cầu.';
      return;
    }
    const command=raw.trim().toLowerCase();
    if(command==='help'){
      output.textContent='Lệnh: help | clear | features | ping | project | tasks | tasks next | git status | disconnect.\nĐây không phải terminal; không có shell, tham số, Git ghi hoặc thực thi task.';
      quickHint.textContent='Đã hiển thị trợ giúp cục bộ, không dùng lượt E2EE.';
      return;
    }
    if(command==='clear'){
      output.textContent='';
      quickHint.textContent='Đã xóa khu vực phản hồi cục bộ.';
      return;
    }
    if(command==='disconnect'){
      quickHint.textContent='';
      invoke('/api/close');
      return;
    }
    const routes=Object.freeze({
      features:'/api/features',
      ping:'/api/ping',
      project:'/api/project-info',
      tasks:'/api/tasks-preview',
      'tasks next':'/api/tasks-preview-next',
      'git status':'/api/git-status'
    });
    if(!Object.prototype.hasOwnProperty.call(routes,command)){
      quickHint.textContent='Lệnh không được hỗ trợ; không gửi yêu cầu. Gõ help để xem các lệnh đọc được phép.';
      return;
    }
    const permission={
      ping:'session_ping',project:'project_info',tasks:'tasks_preview',
      'tasks next':'tasks_preview','git status':'git_status'
    }[command];
    if(command!=='features'&&
      (command!=='ping'||featuresKnown)&&
      (!featuresKnown||!available[permission])){
      quickHint.textContent='Host chưa cấp quyền đọc này. Kiểm tra chức năng trước; không gửi yêu cầu.';
      return;
    }
    if(command==='tasks next'&&(!canFetchTaskNext||bulkLoading)){
      quickHint.textContent='Chưa có trang task tiếp theo đã xác minh; không gửi yêu cầu.';
      return;
    }
    if(budget<=0||bulkLoading){
      quickHint.textContent='Phiên đã hết lượt hoặc đang tải task; không gửi yêu cầu.';
      return;
    }
    if(!taskMaintenanceAllowed(routes[command])){
      quickHint.textContent='Đang giữ lượt E2EE cho hủy/nhận trạng thái/dọn job dài; không gửi lệnh đọc.';
      return;
    }
    quickHint.textContent='Đang thực hiện thao tác đọc cố định qua E2EE.';
    // Move to the relevant result view at the instant of the explicit
    // request, not on a late reply (which might arrive after a user
    // navigates elsewhere). Palette is always present above all tabs.
    const targetViews=Object.freeze({
      project:'project',tasks:'tasks','tasks next':'tasks','git status':'git'
    });
    if(Object.prototype.hasOwnProperty.call(targetViews,command)){
      showView(targetViews[command]);
    }
    invoke(routes[command]);
  }
  quickRun.addEventListener('click',runQuickAction);
  quickCommand.addEventListener('keydown',event=>{
    if(event.key==='Enter'){
      event.preventDefault();
      runQuickAction();
    }
  });
  features.addEventListener('click',()=>invoke('/api/features'));
  ping.addEventListener('click',()=>invoke('/api/ping'));
  git.addEventListener('click',()=>invoke('/api/git-status'));
  gitLog.addEventListener('click',()=>invoke('/api/git-log'));
  gitBranches.addEventListener('click',()=>invoke('/api/git-branches'));
  gitRemotes.addEventListener('click',()=>invoke('/api/git-remotes'));
  gitTrackingStart.addEventListener('click',()=>invoke('/api/git-tracking-start'));
  gitTrackingNext.addEventListener('click',()=>{
    if(gitTrackingMore&&gitTrackingPage>0)invoke('/api/git-tracking-next');
  });
  gitBranchDeleteReview.addEventListener('click',()=>{
    if(busy||closed||!verified||budget<=0||gitJobBlocksMutation()||
      terminalState!=='closed'||!featuresKnown||!available.git_branches)return;
    gitDeletePreview=null;gitDeletePending=null;
    invoke('/api/git-branch-delete-review');
  });
  gitBranchDelete.addEventListener('click',()=>{
    if(busy||closed||!verified||budget<=0||gitJobBlocksMutation()||
      terminalState!=='closed'||!featuresKnown||!available.git_branches||
      !gitDeletePreview)return;
    const plan=gitDeletePreview;
    if(!window.confirm('XÁC NHẬN XOÁ nhánh cục bộ '+plan.branch+
      ', trong khi giữ nhánh '+plan.current+' và commit '+
      plan.head.slice(0,12)+'? Không thể khôi phục tên ref tự động. Host chỉ xoá nếu hai nhánh vẫn cùng HEAD, worktree/index sạch và cho phép git.write.'))return;
    gitDeletePending=plan;
    gitDeletePreview=null;
    invoke('/api/git-branch-delete');
  });
  gitSwitchReview.addEventListener('click',()=>{
    if(busy||closed||!verified||budget<=0||gitJobBlocksMutation()||
      terminalState!=='closed'||!featuresKnown||!available.git_branches)return;
    gitSwitchPreview=null;gitSwitchPending=null;
    invoke('/api/git-switch-review');
  });
  gitSwitchSameHead.addEventListener('click',()=>{
    if(busy||closed||!verified||budget<=0||gitJobBlocksMutation()||
      terminalState!=='closed'||!featuresKnown||!available.git_branches||
      !gitSwitchPreview)return;
    const plan=gitSwitchPreview;
    if(!window.confirm('Xác nhận CHUYỂN nhánh trên host từ '+plan.from+
      ' sang '+plan.branch+' cùng HEAD '+plan.head.slice(0,12)+
      '? Host kiểm tra lại index/worktree sạch, quyền git.write và opt-in riêng.'))return;
    gitSwitchPending=plan;
    gitSwitchPreview=null;
    invoke('/api/git-switch-same-head');
  });
  gitBranchCreate.addEventListener('click',()=>{
    if(busy||closed||!verified||gitJobBlocksMutation()||!featuresKnown||
      !available.git_branch_create||budget<=0)return;
    if(!window.confirm('Tạo một nhánh Git MỚI tại HEAD, với tên được quản trị viên cấu hình sẵn trên host? Không checkout, không push và không thay đổi working tree.'))return;
    invoke('/api/git-branch-create');
  });
  gitDiff.addEventListener('click',()=>invoke('/api/git-diff'));
  gitPatchStart.addEventListener('click',()=>invoke('/api/git-patch-start'));
   gitPatchCachedStart.addEventListener('click',()=>invoke('/api/git-patch-cached-start'));
  gitPatchNext.addEventListener('click',()=>{
    if(gitPatchMore&&gitPatchPage>0)invoke(gitPatchMode==='cached'?
      '/api/git-patch-cached-next':'/api/git-patch-next');
  });
  function chosenGitFile(){
    const index=Number(gitFileSelect.value);
    if(!Number.isInteger(index)||index<1||index>12||
      !gitRows[index-1]||!gitRawSnapshot||gitRawSnapshot.length>256)return null;
    return {index,snapshot:gitRawSnapshot};
  }
  gitStageFile.addEventListener('click',()=>{
    if(busy||closed||!verified||gitJobBlocksMutation()||!featuresKnown||
      !available.git_stage_file||budget<=0)return;
    const selection=chosenGitFile();
    if(!selection||!['M','D'].includes(gitRows[selection.index-1].code[1]))return;
    if(!window.confirm('Stage CHỈ file đã chọn từ Git Status E2EE? Nếu status trên host thay đổi, yêu cầu sẽ bị từ chối.'))return;
    invoke('/api/git-stage-file',selection);
  });
  gitUnstageFile.addEventListener('click',()=>{
    if(busy||closed||!verified||gitJobBlocksMutation()||!featuresKnown||
      !available.git_unstage_file||budget<=0)return;
    const selection=chosenGitFile();
    if(!selection||!['M','A','D'].includes(gitRows[selection.index-1].code[0]))return;
    if(!window.confirm('Unstage CHỈ file đã chọn từ Git Status E2EE? Working tree không bị sửa.'))return;
    invoke('/api/git-unstage-file',selection);
  });
  gitStageTracked.addEventListener('click',()=>{
    if(busy||closed||!verified||gitJobBlocksMutation()||!featuresKnown||
      !available.git_stage_tracked||budget<=0)return;
    if(!window.confirm('STAGE toàn bộ thay đổi của file đã được theo dõi trên host (bao gồm xóa file)? Không thêm untracked.'))return;
    invoke('/api/git-stage-tracked');
  });
  gitUnstageAll.addEventListener('click',()=>{
    if(busy||closed||!verified||gitJobBlocksMutation()||!featuresKnown||
      !available.git_unstage_all||budget<=0)return;
    if(!window.confirm('UNSTAGE toàn bộ index trên host? Nội dung working tree không thay đổi.'))return;
    invoke('/api/git-unstage-all');
  });
  gitFetch.addEventListener('click',()=>{
    if(busy||closed||!verified||gitJobBlocksMutation()||!featuresKnown||
      !available.git_fetch||budget<=0)return;
    if(!window.confirm('Git fetch chỉ từ upstream cùng nhánh đã cấu hình trên host? Không cập nhật HEAD, không tải tags/submodules.'))return;
    invoke('/api/git-fetch');
  });
  gitPullReview.addEventListener('click',()=>invoke('/api/git-pull-review'));
   gitPullChanges.addEventListener('click',()=>invoke('/api/git-pull-changes'));
  gitPull.addEventListener('click',()=>{
    if(busy||closed||!verified||gitJobBlocksMutation()||!featuresKnown||!available.git_pull||budget<=0)return;
    if(!window.confirm('Thực hiện Git pull --ff-only trên repository gốc của host? Worktree phải sạch và đã cấu hình upstream.'))return;
    invoke('/api/git-pull');
  });
  gitPush.addEventListener('click',()=>{
    if(busy||closed||!verified||gitJobBlocksMutation()||!featuresKnown||
      !available.git_push||budget<=0)return;
    if(!window.confirm('Đẩy đúng commit HEAD hiện tại lên upstream cùng tên nhánh? Không force, không gửi tags, không chọn repository khác.'))return;
    invoke('/api/git-push');
  });
  gitCommit.addEventListener('click',()=>{
    if(busy||closed||!verified||gitJobBlocksMutation()||!featuresKnown||!available.git_commit||budget<=0)return;
    const message=gitCommitMessage.value;
    if(!validCommitSubject(message)){
      output.textContent='Commit message cần một dòng, không có ký tự điều khiển và tối đa 120 byte UTF-8.';
      return;
    }
    if(!window.confirm('Commit CHỈ các thay đổi ĐÃ STAGE trên host? Không tự động thêm file và không chạy hooks.'))return;
    invoke('/api/git-commit',{message});
  });
  // These commands reach only the independently trusted native loopback
  // server; they are never dispatched via an arbitrary URL or PHP page.
  terminalOpen.addEventListener('click',()=>{
    if(busy||closed||!verified||budget<=0||terminalState!=='closed'||
      longTaskID!==0||!featuresKnown||!available.terminal)return;
    if(!window.confirm('Mở một shell /bin/sh trên TaskDeck host? Chỉ dùng với thiết bị và tài khoản bạn kiểm soát.'))return;
    invoke('/api/terminal-open');
  });
  terminalPoll.addEventListener('click',()=>{
    if(busy||closed||!verified||budget<=0||terminalState==='closed'||
      !featuresKnown||!available.terminal)return;
    // Manual Poll takes precedence over any queued automatic callback.
    terminalAutoEpoch++;terminalAutoScheduled=false;
    terminalAutoPending=false;
    invoke('/api/terminal-poll');
  });
  terminalAutoButton.addEventListener('click',()=>{
    if(busy||closed||closing||terminated||!verified||budget<=0||
      !featuresKnown||!available.terminal)return;
    terminalAutoEpoch++;terminalAutoScheduled=false;
    terminalAutoEnabled=!terminalAutoEnabled;
    render();
    if(terminalAutoEnabled)scheduleTerminalAutoPoll();
  });
  function sendTerminalText(input){
    if(busy||closed||!verified||budget<=0||terminalState!=='running'||
      !featuresKnown||!available.terminal||!validTerminalInputSegment(input))return;
    invoke('/api/terminal-write',{input});
  }
  terminalSend.addEventListener('click',()=>{
    const value=terminalInput.value;
    terminalInput.value='';
    if(value.includes(String.fromCharCode(10))||value.includes(String.fromCharCode(13))||
      !validTerminalInputSegment(value+String.fromCharCode(10))){
      terminalStateText.textContent='Lệnh phải là một dòng UTF-8 tối đa 95 byte.';
      return;
    }
    sendTerminalText(value+String.fromCharCode(10));
  });
  terminalInput.addEventListener('keydown',event=>{
    if(event.key==='Enter'){
      event.preventDefault();
      terminalSend.click();
    }
  });
  terminalInterrupt.addEventListener('click',()=>sendTerminalText(String.fromCharCode(3)));
  terminalEOF.addEventListener('click',()=>sendTerminalText(String.fromCharCode(4)));
  terminalResize.addEventListener('click',()=>{
    if(busy||closed||!verified||budget<=0||terminalState!=='running'||
      !featuresKnown||!available.terminal)return;
    const cols=Number(terminalCols.value),rows=Number(terminalRows.value);
    if(!Number.isInteger(cols)||cols<20||cols>180||
      !Number.isInteger(rows)||rows<8||rows>60)return;
    invoke('/api/terminal-resize',{cols,rows});
  });
  terminalClose.addEventListener('click',()=>{
    if(busy||closed||!verified||budget<=0||terminalState==='closed'||
      !featuresKnown||!available.terminal)return;
    if(!window.confirm('Đóng Terminal PTY và dừng mọi tiến trình shell đang chạy trong phiên này?'))return;
    invoke('/api/terminal-close');
  });
  project.addEventListener('click',()=>invoke('/api/project-info'));
  filesDirSelect.addEventListener('change',()=>{if(!closed)render();});
  filesDirOpen.addEventListener('click',()=>{
    const index=Number(filesDirSelect.value);
    if(busy||closed||!verified||!Number.isInteger(index)||index<1||
      index>filesRootNames.length||!filesRootNames[index-1].startsWith('D:'))return;
    filesDirNames=[];filesDirMore=false;
    filesDirChosen=filesRootNames[index-1].slice(2);
    filesDirOutput.textContent='';filesDirOutput.hidden=true;
    invoke('/api/files-dir-select',{index});
  });
  filesDirNext.addEventListener('click',()=>{
    if(filesDirMore)invoke('/api/files-dir-next');
  });
  filesSelect.addEventListener('change',()=>{if(!closed)render();});
  filesReadSelected.addEventListener('click',()=>{
    const index=Number(filesSelect.value);
    if(busy||closed||!verified||!Number.isInteger(index)||index<1||
      index>filesRootNames.length||!filesRootNames[index-1].startsWith('F:'))return;
    filesPendingSelectedName=filesRootNames[index-1].slice(2);
    filesReadName='';filesReadText='';filesReadPage=0;filesReadMore=false;
    filesReadOutput.textContent='';filesReadOutput.hidden=true;
    invoke('/api/files-read-select',{index});
  });
  filesReadStart.addEventListener('click',()=>{
    filesReadName='';filesReadText='';filesReadPage=0;filesReadMore=false;
    filesReadOutput.textContent='';filesReadOutput.hidden=true;
    invoke('/api/files-read-start');
  });
  filesReadNext.addEventListener('click',()=>{
    if(filesReadMore)invoke('/api/files-read-next');
  });
  filesRootButton.addEventListener('click',()=>invoke('/api/files-root'));
  filesRootNext.addEventListener('click',()=>{
    if(filesRootMore)invoke('/api/files-root-next');
  });
  tasksPreview.addEventListener('click',()=>invoke('/api/tasks-preview'));
  taskDepsSelect.addEventListener('change',()=>{
    taskWorkflowReadyID=0;taskDepsPendingID=0;
    preparedWorkflowSteps=null;
    taskDepsOutput.textContent='';taskDepsOutput.hidden=true;
    render();
  });
  taskDepsView.addEventListener('click',()=>{
    const id=Number(taskDepsSelect.value);
    if(busy||closed||bulkLoading||!verified||!featuresKnown||
      !available.tasks_preview||budget<=0||!Number.isInteger(id)||
      id<=0||id>2147483647||!cachedTaskIDs.includes(id))return;
    taskWorkflowReadyID=0;
    preparedWorkflowSteps=null;
    taskDepsPendingID=id;
    taskDepsOutput.textContent='';taskDepsOutput.hidden=true;
    invoke('/api/tasks-dependencies/'+String(id));
  });
  taskWorkflowStart.addEventListener('click',()=>{
    const id=Number(taskDepsSelect.value);
    if(busy||closed||bulkLoading||!verified||!featuresKnown||
      !available.task_run||!available.task_long||budget<=3||
      longTaskID!==0||!Number.isInteger(id)||id<1||
      id!==taskWorkflowReadyID||!cachedTaskIDs.includes(id))return;
    if(!window.confirm('Chạy tối đa 4 bước process theo thứ tự phụ thuộc trên host? '+
      'Các nhánh chỉ được chạy tuần tự khi host khai báo dependsOrder: sequence. '+
      'Một bước thất bại có thể được chạy lại đúng một lần khi task có retry: 1 '+
      'và host đã bật quyền retry riêng; việc chạy lại có thể lặp tác dụng phụ. '+
      'Host có thể giới hạn 1–30 giây/bước với timeoutSeconds khi bật riêng; '+
      'bước hết giờ sẽ bị dừng, không chạy lại hoặc chạy bước sau. '+
      'Khi host bật quyền condition riêng, bước cuối có condition: always '+
      'có thể chạy dọn dẹp sau lỗi ở bước ngay trước; workflow vẫn thất bại. '+
      'condition: failure chỉ chạy bước cuối sau lỗi ở bước ngay trước, '+
      'nếu các bước trước thành công thì đánh dấu bỏ qua, không chạy process. '+
      'Không áp dụng sau hủy hoặc hết giờ. '+
      'Có thể hủy, dừng khi lỗi; không gửi lệnh hoặc tham số từ browser.'))return;
    taskWorkflowReadyID=0;
    invoke('/api/tasks-workflow-start/'+String(id));
  });
  tasksReport.addEventListener('click',()=>invoke('/api/tasks-report'));
  workflowAuditButton.addEventListener('click',()=>invoke('/api/tasks-workflow-audit'));
  workflowAuditNext.addEventListener('click',()=>invoke('/api/tasks-workflow-audit-next'));
  taskFetchNext.addEventListener('click',()=>{
    if(!canFetchTaskNext||bulkLoading||closed||busy||budget<=0)return;
    invoke('/api/tasks-preview-next');
  });
  async function loadRemainingTaskPages(){
    if(bulkLoading||busy||closed||terminated||budget<=0||
      (longTaskID!==0&&budget<=3)||
      !canFetchTaskNext||!featuresKnown||!available.tasks_preview)return;
    bulkLoading=true;
    bulkStopRequested=false;
    let pages=0;
    taskLoadStatus.textContent='Đang tải tuần tự qua E2EE. Có thể dừng sau yêu cầu hiện tại hoặc ngắt phiên ngay.';
    render();
    try{
      // Local and Relay both enforce the same absolute 16-round limit.
      // Never retry a failed exchange or silently create a new session.
      // Bulk reads must not consume the last three authenticated actions
      // needed to cancel/inspect/clear an active long-running host job.
      for(let i=0;i<16&&!bulkStopRequested&&!closed&&!terminated&&
          canFetchTaskNext&&budget>(longTaskID!==0?3:0);i++){
        const countBefore=cachedTaskLabels.length;
        await invoke('/api/tasks-preview-next');
        if(closed||terminated||cachedTaskLabels.length<=countBefore)break;
        pages++;
        if(!bulkStopRequested){
          taskLoadStatus.textContent='Đã tải thêm '+String(pages)+
            ' trang; đang có '+String(cachedTaskLabels.length)+' / '+
            String(remoteTaskTotal)+' nhãn task.';
        }
      }
    }finally{
      const stopped=bulkStopRequested;
      bulkLoading=false;
      bulkStopRequested=false;
      if(!closed&&!terminated){
        taskLoadStatus.textContent=stopped?
          'Đã dừng. Dữ liệu đã tải vẫn có thể tìm kiếm và phân trang cục bộ.':
          !canFetchTaskNext?'Đã tải đủ các nhãn task hợp lệ.':
          budget===0?'Đã hết lượt E2EE. Cần phiên native mới để tải tiếp.':
          longTaskID!==0&&budget<=3?
            'Đã dừng tải tự động để giữ ba lượt E2EE cho job đang chạy.':
            'Đã dừng tải tự động; có thể chọn tải thêm.';
      }else{
        taskLoadStatus.textContent='';
      }
      render();
      // The pending poll timer may have fired while bulkLoading was true.
      // Reschedule once the bulk operation releases the GUI, without
      // spending any of the three reserved action rounds.
      if(!closed&&!terminated)scheduleLongTaskPoll();
    }
  }
  taskFetchAll.addEventListener('click',()=>{loadRemainingTaskPages();});
  taskStopAll.addEventListener('click',()=>{
    if(!bulkLoading||closed)return;
    bulkStopRequested=true;
    taskLoadStatus.textContent='Đã yêu cầu dừng; không gửi thêm trang sau lượt E2EE hiện tại.';
  });
  longTaskAutoButton.addEventListener('click',()=>{
    if(longTaskID===0||closed)return;
    longTaskPollEpoch++;longTaskPollScheduled=false;
    longTaskAuto=!longTaskAuto;
    render();
    if(longTaskAuto)scheduleLongTaskPoll();
  });
  longTaskPoll.addEventListener('click',()=>{
    if(longTaskID>0&&budget>0&&!busy)invoke('/api/tasks-poll/'+String(longTaskID));
  });
  longTaskCopy.addEventListener('click',()=>{
    if(longTaskID===0||closed||terminated||!verified||!longTaskText)return;
    const id=longTaskID,outputSnapshot=longTaskText;
    copyVerifiedTaskOutput(outputSnapshot,longTaskCopy,()=>
      longTaskID===id&&longTaskText===outputSnapshot);
  });
  longTaskCancel.addEventListener('click',()=>{
    if(longTaskID>0&&budget>0&&!busy&&!longTaskCancelPending&&
      taskMaintenanceAllowed('/api/tasks-cancel/'+String(longTaskID))&&
      ['started','running'].includes(longTaskState)){
      invoke('/api/tasks-cancel/'+String(longTaskID));
    }
  });
  longTaskClear.addEventListener('click',()=>{
    if(longTaskID>0&&budget>0&&!busy&&
      ['completed','failed','cancelled','timed_out'].includes(longTaskState)){
      // Clear discards unread host-captured output. The button label alone
      // is not sufficient protection against accidental data loss.
      if(longTaskMore&&!window.confirm(
        'Task còn output chưa nhận. Bỏ vĩnh viễn phần output còn lại và dọn job này?'))return;
      invoke('/api/tasks-clear/'+String(longTaskID));
    }
  });
  taskRunHistoryClear.addEventListener('click',()=>{
    if(closed)return;
    const activeSerial=activeTaskHistorySerial();
    // Only an explicit job Clear releases the active history record.
    // A user clearing old results must not sever live Poll updates.
    taskRunHistory=activeSerial?
      taskRunHistory.filter(item=>item.serial===activeSerial):[];
    if(!activeSerial)longTaskHistorySerial=0;
    renderTaskRunHistory();
  });
  taskRunStop.addEventListener('click',()=>{
    if(taskRunning&&!closing&&!closed)invoke('/api/close');
  });
  disconnect.addEventListener('click',()=>invoke('/api/close'));
  // Best effort only: browser/OS crash cannot guarantee pagehide delivery.
  // Native E2EE is also bounded by the host session deadline.
  window.addEventListener('pagehide',()=>{
    if(terminated||!verified)return;
    const revokeCapability=capability;
    closed=true;terminated=true;
    // A page can enter the back-forward cache rather than being destroyed.
    // Clear all previous E2EE data SYNCHRONOUSLY before the page is hidden.
    output.textContent='Phiên đã đóng khi rời trang.';
    render();
    fetch('/api/close',{
      method:'POST',mode:'same-origin',credentials:'omit',
      cache:'no-store',keepalive:true,
      headers:{'X-TaskDeck-Local-Capability':revokeCapability}
    }).catch(()=>{});
  });
  if(!verified){
    activation.hidden=false;
    output.textContent='Nhập mã kích hoạt từ terminal client TaskDeck tin cậy để sử dụng GUI. Không nhập mật khẩu TaskDeck tại đây.';
  }
  showView('overview');
  render();
  loadLocalSession();
  setInterval(updateSessionClock,1000);
  // Observe localhost-only native session state while the tab is idle.
  // No E2EE round, no automatic password login or PHP access.
  setInterval(()=>{
    if(verified&&!closed&&!busy&&!closing&&!terminated)loadLocalSession();
  },15000);
})();`
