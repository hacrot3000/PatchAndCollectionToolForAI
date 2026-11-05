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
    <div class="nav muted"><span>07</span> Editor / Files <em>Chưa bật</em></div>
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
   <section class="gui-panel" id="panel-tasks" hidden>
   <div class="tiles">
    <article class="tile">
     <div class="tile-heading"><span class="dot"></span><h2>Task của workspace</h2></div>
     <p>Xem task qua E2EE. Khi host bật quyền, chạy task ngắn hoặc job dài có giới hạn; hỗ trợ các input process token/pickString đã xác minh. Không nhập lệnh shell tùy ý từ client.</p>
     <div class="actions">
      <button type="button" id="tasks-preview">Xem trước task</button>
      <button type="button" id="task-run-stop" hidden>Dừng task và ngắt phiên</button>
     </div>
     <div class="actions" id="long-task-controls" hidden>
      <button type="button" id="long-task-poll">Nhận output tiếp</button>
      <button type="button" id="long-task-auto">Tạm dừng tự nhận</button>
      <button type="button" id="long-task-cancel">Hủy job (giữ phiên)</button>
      <button type="button" id="long-task-clear">Dọn job đã kết thúc</button>
     </div>
     <pre id="long-task-output" hidden aria-live="polite"></pre>
     <section class="task-run-history" id="task-run-history" hidden aria-label="Kết quả chạy task trong phiên hiện tại">
      <div class="task-run-history-head"><strong>Kết quả chạy task gần đây · chỉ lưu trong phiên</strong><button type="button" id="task-run-history-clear">Xóa kết quả</button></div>
      <ol id="task-run-history-list"></ol>
     </section>
    </article>
   </div>
   <section class="task-preview" id="task-preview" hidden aria-label="Task trên workspace đã xác minh">
    <div class="task-preview-heading"><strong>Danh sách task · chỉ xem trước</strong><span id="task-total"></span></div>
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
     <div class="actions"><button type="button" id="git">Đọc trạng thái Git</button><button type="button" id="git-log" disabled>Git log (3 commit)</button><button type="button" id="git-branches" disabled>Nhánh Git cục bộ</button><button type="button" id="git-branch-create" disabled>Tạo nhánh đã cấu hình trên host</button><button type="button" id="git-diff" disabled>Git diff --stat</button><button type="button" id="git-patch-start" disabled>Xem diff chi tiết</button><button type="button" id="git-patch-next" disabled>Trang diff tiếp</button><button type="button" id="git-fetch" disabled>Git fetch (upstream)</button><button type="button" id="git-pull" disabled>Git pull (fast-forward)</button><button type="button" id="git-push" disabled>Git push (không force)</button></div>
      <p>Diff chi tiết chỉ đọc tối đa 4 KiB thay đổi tracked, chia trang E2EE; khi thay đổi nội dung giữa các trang sẽ từ chối đọc tiếp.</p><pre id="git-patch-output" class="git-patch-output" aria-label="Git diff chi tiết" hidden></pre>
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
     <p>Output là văn bản thuần, không chạy escape sequence từ host dưới dạng HTML. Nhấn “Đọc output” để lấy phần tiếp theo; mỗi lần sử dụng một lượt xác thực E2EE.</p>
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
  const gitBranchCreate=document.getElementById('git-branch-create');
  const gitDiff=document.getElementById('git-diff');
  const gitPatchStart=document.getElementById('git-patch-start');
  const gitPatchNext=document.getElementById('git-patch-next');
  const gitPatchOutput=document.getElementById('git-patch-output');
  const gitFetch=document.getElementById('git-fetch');
  const gitStageTracked=document.getElementById('git-stage-tracked');
  const gitUnstageAll=document.getElementById('git-unstage-all');
  const gitPull=document.getElementById('git-pull');
  const gitCommit=document.getElementById('git-commit');
  const gitPush=document.getElementById('git-push');
  const gitCommitMessage=document.getElementById('git-commit-message');
  const terminalOpen=document.getElementById('terminal-open');
  const terminalPoll=document.getElementById('terminal-poll');
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
  const taskRunStop=document.getElementById('task-run-stop');
  const longTaskControls=document.getElementById('long-task-controls');
  const longTaskPoll=document.getElementById('long-task-poll');
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
  let inspectedTasks=new Map();
  // Transient, local-only drafts survive pagination, sorting and job polls.
  // Never retain them across a new host snapshot, re-inspection or logout.
  let taskInputDrafts=new Map();
  // Only locally decrypted task outcomes, never written to browser storage
  // or Relay. Bounded so repeated actions cannot inflate retained output.
  let taskRunHistory=[],taskHistorySerial=0,longTaskHistorySerial=0;
  function renderTaskRunHistory(){
    taskRunHistoryList.replaceChildren();
    taskRunHistoryView.hidden=taskRunHistory.length===0;
    taskRunHistoryClear.disabled=taskRunHistory.length===0||closed;
    for(const item of taskRunHistory){
      const li=document.createElement('li');
      const header=document.createElement('strong');
      header.textContent='Task '+String(item.taskID)+
        (item.label?' · '+item.label:'')+' · '+
        item.kind+' · '+item.status+
        (item.exitCode>=0?' · exit='+String(item.exitCode):'')+
        (item.truncated?' · output đã cắt':'');
      li.append(header);
      if(item.output){
        const body=document.createElement('pre');
        body.textContent=item.output;
        li.append(body);
      }
      taskRunHistoryList.append(li);
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
  function updateLongTaskHistory(taskID,status,exitCode,truncated){
    const item=taskRunHistory.find(entry=>entry.serial===longTaskHistorySerial&&
      entry.taskID===taskID&&entry.kind==='dài');
    if(!item)return;
    item.status=status;
    item.exitCode=exitCode;
    item.truncated=item.truncated||truncated;
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
    }else{
      taskNextHelp.textContent='Còn '+String(remoteTaskTotal-loaded)+
        ' nhãn chưa tải. Tải thêm từ host sẽ dùng 1 trong '+String(budget)+' lượt E2EE còn lại.';
    }
  }
  function renderTaskPage(){
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
          if(closed||busy||bulkLoading||budget<=0||
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
            if(!selection||closed||busy||bulkLoading||budget<=0||
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
    taskFetchNext.disabled=busy||bulkLoading||closed||!verified||budget<=0||!featuresKnown||
      !available.tasks_preview;
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
    terminal:document.getElementById('nav-terminal')
  };
  const panels={
    overview:document.getElementById('panel-overview'),
    project:document.getElementById('panel-project'),
    tasks:document.getElementById('panel-tasks'),
    git:document.getElementById('panel-git'),
    terminal:document.getElementById('panel-terminal')
  };
  const viewDescriptions={
    overview:['Tổng quan','Quản lý phiên và kiểm tra chức năng host. Chuyển tab không tốn lượt E2EE.'],
    project:['Dự án TaskDeck','Dữ liệu hiển thị được xác thực, chỉ thuộc một dự án TaskDeck đã cấp quyền.'],
    tasks:['Task của workspace','Chỉ xem nhãn task, không gửi ID, đường dẫn hoặc lệnh thực thi.'],
    git:['Git Status','Dữ liệu Git và thao tác pull, commit, push đều phải được host cho phép.'],
    terminal:['Terminal PTY','Shell /bin/sh giới hạn 90 giây, một phiên trên mỗi kết nối E2EE, Linux/macOS.']
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
  let longTaskAuto=false,longTaskPollScheduled=false;
  let terminalState='closed',terminalTranscript='';
  let gitPatchText='',gitPatchPage=0,gitPatchMore=false;
  let actionEpoch=0;
  // Only the newest initiated localhost status poll may alter the GUI.
  let sessionStatusEpoch=0;
  let featuresKnown=false;
  let available={session_ping:false,project_info:false,tasks_preview:false,
    task_run:false,task_long:false,git_status:false,git_log:false,git_branches:false,git_branch_create:false,git_diff:false,git_patch:false,git_fetch:false,git_stage_tracked:false,git_unstage_all:false,git_stage_file:false,git_unstage_file:false,git_pull:false,git_commit:false,git_push:false,terminal:false};
  let expiresAt=0,expiredByClock=false;
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
    longTaskID=0;longTaskState='';longTaskText='';
    longTaskHistorySerial=0;
    taskRunHistory=[];taskRunHistoryList.replaceChildren();
    taskRunHistoryView.hidden=true;taskRunHistoryClear.disabled=true;
    longTaskMore=false;longTaskAuto=false;
    terminalState='closed';terminalTranscript='';
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
    available={session_ping:false,project_info:false,tasks_preview:false,task_run:false,task_long:false,git_status:false,git_log:false,git_branches:false,git_branch_create:false,git_diff:false,git_patch:false,git_fetch:false,git_stage_tracked:false,git_unstage_all:false,git_stage_file:false,git_unstage_file:false,git_pull:false,git_commit:false,git_push:false,terminal:false};
    featureSummary.textContent='Phiên đã đóng. Đăng nhập lại bằng client TaskDeck tin cậy.';
  }
  function render(){
    if(closed)clearPrivateViews();
    // Only definitive native close or browser pagehide can destroy the
    // local capability. A denied E2EE action still permits authenticated
    // /api/close to revoke the native server before expiry.
    if(terminated){
      capability='';
      localToken.value='';
    }
    activate.disabled=activationConsumed||verified||closed||closing||terminated;
    quickCommand.disabled=busy||closed||!verified;
    quickRun.disabled=busy||closed||!verified;
    ping.disabled=busy||closed||!verified||budget<=0||
      (featuresKnown&&!available.session_ping);
    features.disabled=busy||closed||!verified||budget<=0;
    git.disabled=busy||closed||!verified||budget<=0||!featuresKnown||!available.git_status;
    gitLog.disabled=busy||closed||!verified||budget<=0||!featuresKnown||!available.git_log;
    gitBranches.disabled=busy||closed||!verified||budget<=0||!featuresKnown||!available.git_branches;
    gitBranchCreate.disabled=busy||closed||!verified||budget<=0||longTaskID!==0||!featuresKnown||!available.git_branch_create;
    gitDiff.disabled=busy||closed||!verified||budget<=0||!featuresKnown||!available.git_diff;
    gitPatchStart.disabled=busy||closed||!verified||budget<=0||!featuresKnown||!available.git_patch;
    gitPatchNext.disabled=busy||closed||!verified||budget<=0||!featuresKnown||!available.git_patch||!gitPatchMore;
    gitFetch.disabled=busy||closed||!verified||budget<=0||longTaskID!==0||!featuresKnown||!available.git_fetch;
    gitStageTracked.disabled=busy||closed||!verified||budget<=0||longTaskID!==0||!featuresKnown||!available.git_stage_tracked;
    const selectedFile=gitRows[Number(gitFileSelect.value)-1];
    const fileReady=!busy&&!closed&&verified&&budget>0&&longTaskID===0&&
      featuresKnown&&gitRawSnapshot!==''&&selectedFile!==undefined&&
      Number.isInteger(Number(gitFileSelect.value));
    gitStageFile.disabled=!fileReady||!available.git_stage_file||
      !['M','D'].includes(selectedFile?.code?.[1]);
    gitUnstageFile.disabled=!fileReady||!available.git_unstage_file||
      !['M','A','D'].includes(selectedFile?.code?.[0]);
    gitUnstageAll.disabled=busy||closed||!verified||budget<=0||longTaskID!==0||!featuresKnown||!available.git_unstage_all;
    gitPull.disabled=busy||closed||!verified||budget<=0||longTaskID!==0||!featuresKnown||!available.git_pull;
    gitCommit.disabled=busy||closed||!verified||budget<=0||longTaskID!==0||!featuresKnown||!available.git_commit;
    gitPush.disabled=busy||closed||!verified||budget<=0||longTaskID!==0||!featuresKnown||!available.git_push;
    gitCommitMessage.disabled=gitCommit.disabled;
    const canTerminal=featuresKnown&&available.terminal&&verified&&!closed&&budget>0;
    terminalOpen.disabled=busy||!canTerminal||terminalState!=='closed'||longTaskID!==0;
    const terminalLive=canTerminal&&terminalState==='running';
    terminalPoll.disabled=busy||!canTerminal||terminalState==='closed';
    terminalSend.disabled=busy||!terminalLive;
    terminalInput.disabled=terminalSend.disabled;
    terminalInterrupt.disabled=busy||!terminalLive;
    terminalEOF.disabled=busy||!terminalLive;
    terminalResize.disabled=busy||!terminalLive;
    terminalClose.disabled=busy||!canTerminal||terminalState==='closed';
    project.disabled=busy||closed||!verified||budget<=0||!featuresKnown||!available.project_info;
    tasksPreview.disabled=busy||closed||!verified||budget<=0||!featuresKnown||!available.tasks_preview;
    taskRunStop.hidden=!taskRunning||closed;
    taskRunStop.disabled=!taskRunning||!verified||closed||closing||terminated;
    longTaskControls.hidden=longTaskID===0||closed;
    longTaskOutput.hidden=longTaskID===0||closed;
    longTaskPoll.disabled=busy||closed||!verified||budget<=0||longTaskID===0||
      (!['started','running','cancel_requested'].includes(longTaskState)&&!longTaskMore);
    longTaskAutoButton.disabled=closed||!verified||longTaskID===0;
    longTaskAutoButton.textContent=longTaskAuto?'Tạm dừng tự nhận':'Tiếp tục tự nhận';
    longTaskCancel.disabled=busy||closed||!verified||budget<=0||
      longTaskID===0||!['started','running'].includes(longTaskState);
    longTaskClear.textContent=longTaskMore?
      'Bỏ output còn lại và dọn job':'Dọn job đã kết thúc';
    longTaskClear.disabled=busy||closed||!verified||budget<=0||
      longTaskID===0||!['completed','failed','cancelled','timed_out'].includes(longTaskState);
    taskFetchNext.disabled=busy||bulkLoading||closed||!verified||budget<=0||
      !featuresKnown||!available.tasks_preview;
    taskFetchAll.disabled=busy||bulkLoading||closed||!verified||budget<=0||
      (longTaskID!==0&&budget<=3)||
      !featuresKnown||!available.tasks_preview;
    // Ending a bulk run does not re-render the task page. Keep visibility
    // derived from state so Stop/Resume cannot leave stale controls.
    taskFetchAll.hidden=!canFetchTaskNext||bulkLoading;
    taskStopAll.hidden=!bulkLoading;
    taskStopAll.disabled=closed||!bulkLoading;
    for(const button of inspectButtons){
      button.disabled=busy||closed||bulkLoading||!verified||budget<=0||
        !featuresKnown||(button.taskRun?
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
    sessionActivity.textContent=busy?'Đang chờ phản hồi E2EE':(closed?'Phiên không còn hợp lệ':'Không có tác vụ đang chạy');
  }
  // No background E2EE work without a user-started job. Reserve three
  // application roundtrips so Cancel and Clear remain possible. A poll is
  // always an ordinary authenticated native action with fresh RBAC checks.
  function scheduleLongTaskPoll(){
    if(!longTaskAuto||longTaskPollScheduled||longTaskID===0||
      !verified||busy||closed||closing||terminated||bulkLoading||
      budget<=3||(!['started','running','cancel_requested'].includes(longTaskState)&&
        !longTaskMore))return;
    const selectedID=longTaskID;
    longTaskPollScheduled=true;
    setTimeout(()=>{
      longTaskPollScheduled=false;
      if(selectedID===longTaskID&&longTaskAuto&&!busy&&!closed&&
        !closing&&!terminated&&!bulkLoading&&budget>3&&
        (['started','running','cancel_requested'].includes(longTaskState)||
         longTaskMore)){
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
  function validTerminalOutput(value){
    if(typeof value!=='string')return false;
    try{
      if(encodeURIComponent(value).replace(/%[0-9a-f]{2}/gi,'x').length>128)return false;
    }catch(error){return false;}
    for(const ch of value){
      if(ch==='\n'||ch==='\t')continue;
      if(/[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u.test(ch))return false;
    }
    return true;
  }
  async function invoke(endpoint,payload){
    const isClose=endpoint==='/api/close';
    const isTextRun=endpoint.startsWith('/api/tasks-run-input/')||
      endpoint.startsWith('/api/tasks-start-input/');
    const isRun=endpoint.startsWith('/api/tasks-run/')||
      endpoint.startsWith('/api/tasks-run-input/');
    const isLongAction=endpoint.startsWith('/api/tasks-start/')||
      endpoint.startsWith('/api/tasks-start-input/')||
      endpoint.startsWith('/api/tasks-poll/')||
      endpoint.startsWith('/api/tasks-cancel/')||
      endpoint.startsWith('/api/tasks-clear/');
    const isCommit=endpoint==='/api/git-commit';
    const isGitFile=endpoint==='/api/git-stage-file'||endpoint==='/api/git-unstage-file';
    const isTerminalWrite=endpoint==='/api/terminal-write';
    const isTerminalResize=endpoint==='/api/terminal-resize';
    if(payload!==undefined){
      if(!payload)return;
      if(isTextRun){
        if(Object.keys(payload).length!==1||typeof payload.value!=='string'||
          !/^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$/.test(payload.value))return;
      }else if(isCommit){
        if(Object.keys(payload).length!==1||!validCommitSubject(payload.message))return;
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
    if((isTextRun||isCommit||isGitFile||isTerminalWrite||isTerminalResize)&&payload===undefined)return;
    if(!verified||(busy&&!isClose)||(closed&&!isClose)||
      (!isClose&&budget<=0)||
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
      if(isTextRun||isCommit||isGitFile||isTerminalWrite||isTerminalResize)headers['Content-Type']='application/json';
      const options={
        method:'POST',mode:'same-origin',credentials:'omit',cache:'no-store',
        headers
      };
      if(isTextRun||isCommit||isGitFile||isTerminalWrite||isTerminalResize)options.body=JSON.stringify(payload);
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
          const keys=['session_ping','project_info','tasks_preview','git_status'];
          if(data.remote_control_available!==false||
            keys.some(key=>typeof data[key]!=='boolean')||
            (data.task_run!==undefined&&typeof data.task_run!=='boolean')||
            (data.task_long!==undefined&&typeof data.task_long!=='boolean')||
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
            project_info:data.project_info,
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
            ' · Task: '+(available.tasks_preview?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Git: '+(available.git_status?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Git log: '+(available.git_log?'Được phép (3 commit)':'Chưa bật hoặc thiếu quyền')+
            ' · Nhánh cục bộ: '+(available.git_branches?'Được phép (tối đa 4)':'Chưa bật hoặc thiếu quyền')+
            ' · Tạo nhánh host: '+(available.git_branch_create?'Được phép (từ HEAD)':'Chưa bật hoặc thiếu quyền')+
            ' · Git diff: '+(available.git_diff?'Được phép (--stat)':'Chưa bật hoặc thiếu quyền')+
            ' · Diff chi tiết: '+(available.git_patch?'Được phép (4 KiB)':'Chưa bật hoặc thiếu quyền')+
            ' · Stage tracked: '+(available.git_stage_tracked?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Unstage all: '+(available.git_unstage_all?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Stage một file: '+(available.git_stage_file?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Unstage một file: '+(available.git_unstage_file?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Git fetch: '+(available.git_fetch?'Được phép (upstream)':'Chưa bật hoặc thiếu quyền')+
            ' · Git pull: '+(available.git_pull?'Được phép (ff-only)':'Chưa bật hoặc thiếu quyền')+
            ' · Git commit: '+(available.git_commit?'Được phép (staged)':'Chưa bật hoặc thiếu quyền')+
            ' · Git push: '+(available.git_push?'Được phép (upstream)':'Chưa bật hoặc thiếu quyền')+
            ' · PTY: '+(available.terminal?'Được phép (Unix)':'Chưa bật hoặc thiếu quyền')+
            ' · Chạy task: '+(available.task_run?'Được phép (task ngắn)':'Chưa bật hoặc thiếu quyền')+
            ' · Job dài: '+(available.task_long?'Được phép (Unix, giới hạn)':'Chưa bật hoặc thiếu quyền');
          if(!taskPreviewSection.hidden)renderTaskPage();
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
        if(endpoint==='/api/tasks-preview'||endpoint==='/api/tasks-preview-next'){
          const appendPage=endpoint==='/api/tasks-preview-next';
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
          remoteTaskTotal=data.total;
          canFetchTaskNext=data.labels.length>0 &&
            cachedTaskLabels.length<remoteTaskTotal;
          compareCompletedTasks();
          renderTaskPage();
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
          for(const candidate of ['/api/tasks-start/','/api/tasks-start-input/',
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
            data.output.length>128||typeof data.truncated!=='boolean'||
            data.remote_control_available!==false){
            throw new Error('Phản hồi job dài E2EE không hợp lệ');
          }
          if(data.status==='started'){
            longTaskID=id;longTaskText='';
            longTaskAuto=true;
            longTaskHistorySerial=rememberTaskRun(id,'dài','started',-1,'',false);
          }else if(longTaskID!==id){
            throw new Error('Job trả về không thuộc phiên đang chọn');
          }
          if(data.output){
            longTaskText=(longTaskText+data.output).slice(0,16384);
          }
          longTaskState=data.status;
          longTaskMore=data.more;
          if(data.status!=='cleared'){
            updateLongTaskHistory(id,data.status,data.exit_code,data.truncated);
          }
          if(data.status==='cleared'){
            longTaskHistorySerial=0;
            longTaskID=0;
            longTaskText='';longTaskMore=false;
            longTaskAuto=false;
          }
          longTaskOutput.textContent='Task '+String(id)+' · '+data.status+
            ' · exit='+String(data.exit_code)+
            (data.truncated?' · output vượt giới hạn':'')+
            (longTaskMore?' · còn output chưa nhận':'')+
            '\n'+longTaskText;
          output.textContent='Trạng thái job E2EE: '+data.status+
            ' (Task ID '+String(id)+').'+
            (budget<=3&&longTaskID>0&&
              (longTaskMore||['started','running','cancel_requested'].includes(longTaskState))?
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
        if(endpoint==='/api/git-patch-start'||endpoint==='/api/git-patch-next'){
          const fresh=endpoint==='/api/git-patch-start';
          if(data.remote_control_available!==false||
            !Number.isInteger(data.page)||data.page<1||data.page>256||
            data.page!==(fresh?1:gitPatchPage+1)||
            typeof data.patch!=='string'||data.patch.length<1||
            data.patch.length>320||typeof data.more!=='boolean'||
            Array.from(data.patch).some(ch=>{
              if(ch==='\n'||ch==='\t')return false;
              return /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}]/u.test(ch);
            })||
            (fresh?data.patch.length>4096:
              gitPatchText.length+data.patch.length>4096)){
            throw new Error('Trang Git diff E2EE không hợp lệ');
          }
          gitPatchText=fresh?data.patch:gitPatchText+data.patch;
          gitPatchPage=data.page;gitPatchMore=data.more;
          gitPatchOutput.hidden=false;
          gitPatchOutput.textContent=gitPatchText;
          output.textContent='Git diff trang '+String(data.page)+
            (data.more?' · Còn trang tiếp theo.':' · Đã đọc hết diff.');
        }
        if(endpoint==='/api/git-diff'){
          if(data.remote_control_available!==false||typeof data.summary!=='string'||
            data.summary.length===0||data.summary.length>240||
            Array.from(data.summary).some(ch=>{const c=ch.codePointAt(0);return (c<32&&c!==10)||c===127;})){
            throw new Error('Phản hồi Git diff E2EE không hợp lệ');
          }
          output.textContent=data.summary;
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
          gitPatchText='';gitPatchPage=0;gitPatchMore=false;
          gitPatchOutput.textContent='';gitPatchOutput.hidden=true;
          output.textContent='Đã tạo nhánh cục bộ '+data.branch+
            ' tại '+data.head.slice(0,12)+'. HEAD hiện tại không đổi.';
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
          if(data.remote_control_available!==false||
            typeof data.state!=='string'||
            !['running','exited','closed'].includes(data.state)||
            typeof data.output!=='string'||
            typeof data.truncated!=='boolean'||typeof data.more!=='boolean'||
            !validTerminalOutput(data.output)||
            (data.state==='closed'&&(data.output!==''||data.more))||
            (endpoint==='/api/terminal-open'&&data.state!=='running')||
            (endpoint==='/api/terminal-close'&&data.state!=='closed')){
            throw new Error('Kết quả Terminal E2EE không hợp lệ');
          }
          terminalState=data.state;
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
      if(!closed)scheduleLongTaskPoll();
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
    available={session_ping:false,project_info:false,tasks_preview:false,
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
  gitBranchCreate.addEventListener('click',()=>{
    if(busy||closed||!verified||longTaskID!==0||!featuresKnown||
      !available.git_branch_create||budget<=0)return;
    if(!window.confirm('Tạo một nhánh Git MỚI tại HEAD, với tên được quản trị viên cấu hình sẵn trên host? Không checkout, không push và không thay đổi working tree.'))return;
    invoke('/api/git-branch-create');
  });
  gitDiff.addEventListener('click',()=>invoke('/api/git-diff'));
  gitPatchStart.addEventListener('click',()=>invoke('/api/git-patch-start'));
  gitPatchNext.addEventListener('click',()=>{
    if(gitPatchMore&&gitPatchPage>0)invoke('/api/git-patch-next');
  });
  function chosenGitFile(){
    const index=Number(gitFileSelect.value);
    if(!Number.isInteger(index)||index<1||index>12||
      !gitRows[index-1]||!gitRawSnapshot||gitRawSnapshot.length>256)return null;
    return {index,snapshot:gitRawSnapshot};
  }
  gitStageFile.addEventListener('click',()=>{
    if(busy||closed||!verified||longTaskID!==0||!featuresKnown||
      !available.git_stage_file||budget<=0)return;
    const selection=chosenGitFile();
    if(!selection||!['M','D'].includes(gitRows[selection.index-1].code[1]))return;
    if(!window.confirm('Stage CHỈ file đã chọn từ Git Status E2EE? Nếu status trên host thay đổi, yêu cầu sẽ bị từ chối.'))return;
    invoke('/api/git-stage-file',selection);
  });
  gitUnstageFile.addEventListener('click',()=>{
    if(busy||closed||!verified||longTaskID!==0||!featuresKnown||
      !available.git_unstage_file||budget<=0)return;
    const selection=chosenGitFile();
    if(!selection||!['M','A','D'].includes(gitRows[selection.index-1].code[0]))return;
    if(!window.confirm('Unstage CHỈ file đã chọn từ Git Status E2EE? Working tree không bị sửa.'))return;
    invoke('/api/git-unstage-file',selection);
  });
  gitStageTracked.addEventListener('click',()=>{
    if(busy||closed||!verified||longTaskID!==0||!featuresKnown||
      !available.git_stage_tracked||budget<=0)return;
    if(!window.confirm('STAGE toàn bộ thay đổi của file đã được theo dõi trên host (bao gồm xóa file)? Không thêm untracked.'))return;
    invoke('/api/git-stage-tracked');
  });
  gitUnstageAll.addEventListener('click',()=>{
    if(busy||closed||!verified||longTaskID!==0||!featuresKnown||
      !available.git_unstage_all||budget<=0)return;
    if(!window.confirm('UNSTAGE toàn bộ index trên host? Nội dung working tree không thay đổi.'))return;
    invoke('/api/git-unstage-all');
  });
  gitFetch.addEventListener('click',()=>{
    if(busy||closed||!verified||longTaskID!==0||!featuresKnown||
      !available.git_fetch||budget<=0)return;
    if(!window.confirm('Git fetch chỉ từ upstream cùng nhánh đã cấu hình trên host? Không cập nhật HEAD, không tải tags/submodules.'))return;
    invoke('/api/git-fetch');
  });
  gitPull.addEventListener('click',()=>{
    if(busy||closed||!verified||longTaskID!==0||!featuresKnown||!available.git_pull||budget<=0)return;
    if(!window.confirm('Thực hiện Git pull --ff-only trên repository gốc của host? Worktree phải sạch và đã cấu hình upstream.'))return;
    invoke('/api/git-pull');
  });
  gitPush.addEventListener('click',()=>{
    if(busy||closed||!verified||longTaskID!==0||!featuresKnown||
      !available.git_push||budget<=0)return;
    if(!window.confirm('Đẩy đúng commit HEAD hiện tại lên upstream cùng tên nhánh? Không force, không gửi tags, không chọn repository khác.'))return;
    invoke('/api/git-push');
  });
  gitCommit.addEventListener('click',()=>{
    if(busy||closed||!verified||longTaskID!==0||!featuresKnown||!available.git_commit||budget<=0)return;
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
    invoke('/api/terminal-poll');
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
  tasksPreview.addEventListener('click',()=>invoke('/api/tasks-preview'));
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
    longTaskAuto=!longTaskAuto;
    render();
    if(longTaskAuto)scheduleLongTaskPoll();
  });
  longTaskPoll.addEventListener('click',()=>{
    if(longTaskID>0&&budget>0&&!busy)invoke('/api/tasks-poll/'+String(longTaskID));
  });
  longTaskCancel.addEventListener('click',()=>{
    if(longTaskID>0&&budget>0&&!busy&&
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
    taskRunHistory=[];
    longTaskHistorySerial=0;
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
