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
    <button type="button" class="nav" id="nav-tasks" aria-current="false"><span>03</span> Task · xem trước</button>
    <button type="button" class="nav" id="nav-git" aria-current="false"><span>04</span> Git · chỉ đọc</button>
    <div class="nav muted"><span>05</span> Git Pull / Commit / Push <em>Chưa bật</em></div>
    <div class="nav muted"><span>06</span> Terminal <em>Chưa bật</em></div>
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
   <div class="view-head" aria-live="polite"><h2 id="view-title">Tổng quan</h2><p id="view-hint">Các tác vụ xem trước và trạng thái Git luôn chỉ đọc. Đổi tab không gửi thêm yêu cầu E2EE.</p></div>
   <section class="gui-panel" id="panel-overview" >
   <div class="target-summary" aria-label="Danh tính máy chủ và tài khoản đã xác thực"><div><span class="overline">THIẾT BỊ ĐÃ GHIM</span><strong id="target-device">Đang xác thực...</strong></div><div><span class="overline">TÀI KHOẢN TASKDECK</span><strong id="target-user">Đang xác thực...</strong></div></div>
   <div class="banner"><strong>Giới hạn bảo mật</strong><p>Đây là bản GUI thử nghiệm, không phải điều khiển toàn bộ TaskDeck. PHP/Relay chỉ trung chuyển ciphertext. Không có shell, Git ghi hoặc proxy TaskDeck HTTP.</p></div>
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
     <p>Xem tổng số và tối đa 24 nhãn task (tùy dung lượng bản tin E2EE), tìm kiếm và phân trang tại client. Không tải hoặc thực thi command, tham số, secrets hay cấu hình task.</p>
     <div class="actions"><button type="button" id="tasks-preview">Xem trước task</button></div>
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
    <p>Không có nút chạy task. Không thể chọn task hoặc gửi command qua GUI này.</p>
   </section>
   </section>
   <section class="gui-panel" id="panel-git" hidden>
   <div class="tiles">
    <article class="tile">
     <div class="tile-heading"><span class="dot"></span><h2>Git Status</h2></div>
     <p>Đọc trạng thái repository gốc trên máy TaskDeck. Chỉ hoạt động khi host bật riêng quyền đọc Git.</p>
     <div class="actions"><button type="button" id="git">Đọc trạng thái Git</button></div>
    </article>
   </div>
   <section class="git-panel">    <div class="git-summary" id="git-summary" hidden>
     <div class="git-branch" id="git-branch"></div>
     <table aria-label="Các tệp thay đổi trên Git"><thead><tr><th>Trạng thái</th><th>Đường dẫn tệp</th></tr></thead><tbody id="git-files"></tbody></table>
    </div>
   </section>
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

const trustedGUICSS = `:root{color-scheme:dark;font-family:ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#0b1018;color:#e9f0f9}*{box-sizing:border-box}body{margin:0}.app{min-height:100vh;display:grid;grid-template-columns:268px minmax(0,1fr)}.rail{background:#101824;border-right:1px solid #273344;padding:22px 15px;display:flex;flex-direction:column;gap:24px}.logo{font-size:24px;font-weight:900;letter-spacing:-2px;color:#8cc1ff}.logo span{color:#c2f0dc}.workspace{display:flex;flex-direction:column;gap:8px}.workspace strong{font-size:19px}.workspace small{color:#a4b1c4}.overline{color:#78b6f5;font-size:10px;font-weight:800;letter-spacing:1.8px}.rail nav{display:flex;flex-direction:column;gap:6px}.nav{border-radius:7px;padding:12px 10px;font-size:12px;display:flex;align-items:center;gap:9px}.nav span{font-size:10px;opacity:.7}.nav.active{background:#1d3147;border:1px solid #365775;color:#e7f5ff}.rail button.nav{background:transparent;color:#c3d0e1;border:1px solid transparent;width:100%;text-align:left;font-weight:600;white-space:nowrap}.rail button.nav.active{background:#1d3147;border-color:#365775;color:#f4f9ff}.rail button.nav:focus-visible{outline:2px solid #9fd7ff;outline-offset:1px}.view-head{margin:20px 0 12px}.view-head h2{font-size:18px;margin:0 0 6px}.view-head p{font-size:12px;line-height:1.6;color:#adc3d8;margin:0}.gui-panel[hidden]{display:none!important}.git-panel{margin-top:16px;background:#142331;border:1px solid #315367;border-radius:9px}.git-panel .git-summary{border-top:0}.git-panel .git-summary[hidden]{display:none}.nav.muted{opacity:.53;flex-wrap:wrap}.nav em{font-size:10px;margin-left:auto}.rail-note{font-size:11px;color:#aab8cb;line-height:1.6;margin-top:auto;border-top:1px solid #293445;padding-top:14px}.content{max-width:1200px;width:100%;padding:36px 42px;margin:auto}header{display:flex;align-items:flex-start;justify-content:space-between;gap:20px}h1{font-size:28px;letter-spacing:-.8px;margin:10px 0}header p{font-size:13px;color:#aab8cb;line-height:1.5;max-width:600px}.state{border:1px solid #517a78;color:#afeee0;background:#123834;padding:8px 11px;border-radius:50px;font-size:11px;white-space:nowrap}.state.bad{border-color:#8d5354;color:#ffd3d3;background:#432326}  .activation{padding:16px;margin:16px 0;border:1px solid #6084a8;background:#162b40;border-radius:9px}.activation label{display:block;font-size:12px;font-weight:700;color:#cde9fe;margin-bottom:10px}.activation-form{display:flex;gap:10px}.activation-form input{flex:1;min-width:0;background:#0b1725;border:1px solid #6485a6;color:#fff;border-radius:6px;padding:10px;font:12px ui-monospace,monospace}.activation p{font-size:11px;color:#b9cde1;line-height:1.5}.target-summary{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:22px}.target-summary>div{border:1px solid #303e50;background:#152334;border-radius:9px;padding:12px 14px;display:flex;flex-direction:column;gap:6px;min-width:0}.target-summary strong{font-size:12px;overflow-wrap:anywhere;color:#deedf9}.banner{margin:28px 0;border-left:3px solid #d3aa68;background:#242333;padding:15px 18px;border-radius:6px}.banner strong{font-size:12px;color:#ffe0a7}.banner p{font-size:12px;color:#c6cfdf;line-height:1.5;margin-bottom:0} .feature-check{display:flex;align-items:center;justify-content:space-between;gap:14px;margin:16px 0 22px;padding:14px 18px;border:1px solid #456181;background:#172b40;border-radius:9px}.feature-check strong{font-size:13px}.feature-check p{font-size:11px;color:#b9cde1;line-height:1.5;margin:6px 0 0}.feature-check button{flex-shrink:0}.tiles{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.tile{border:1px solid #303e50;background:#172130;border-radius:10px;padding:20px}.tile-heading{display:flex;gap:9px;align-items:center}.tile-heading h2{margin:0;font-size:16px}.dot{height:8px;width:8px;border-radius:50%;background:#83bdff} .project-overview{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin-top:16px}.project-overview[hidden]{display:none}.project-overview>div{background:#142b37;border:1px solid #315967;border-radius:8px;padding:14px;display:flex;flex-direction:column;gap:8px;min-width:0}.project-overview strong{font-size:13px;overflow-wrap:anywhere;color:#d9edf9} .task-preview{margin-top:16px;padding:16px;background:#142632;border:1px solid #305166;border-radius:9px}.task-preview[hidden]{display:none}.task-preview-heading{display:flex;justify-content:space-between;gap:12px;font-size:13px}.task-preview-heading span,.task-preview p{color:#aab8cb;font-size:11px}.task-coverage{display:flex;flex-direction:column;gap:8px;margin:12px 0;padding:11px 13px;border:1px solid #305269;background:#0e202e;border-radius:7px}.task-coverage strong{font-size:12px}.task-coverage span{font-size:11px;color:#c2d3e4;line-height:1.55}.task-coverage progress{width:100%;height:8px;accent-color:#72baed}.task-filter-row{display:grid;grid-template-columns:minmax(0,2fr) minmax(0,1fr);gap:12px}.task-filter-row>div{min-width:0}.task-preview select{width:100%;border:1px solid #527594;background:#0c1826;color:#eff7ff;border-radius:6px;font-size:12px;padding:10px}.task-preview #task-filter-empty[hidden]{display:none}@media(max-width:620px){.task-filter-row{grid-template-columns:1fr}}.task-search-label{display:block;font-size:11px;margin:12px 0 6px;color:#c3dbef}.task-preview input[type="search"]{width:100%;border:1px solid #527594;background:#0c1826;color:#eff7ff;border-radius:6px;font-size:12px;padding:10px}.task-preview ul{margin:12px 0;padding-left:18px;font-size:12px;line-height:1.7;overflow-wrap:anywhere} .task-fetch-actions{display:flex;flex-wrap:wrap;align-items:center;gap:8px;margin:12px 0}.task-fetch-actions button{max-width:100%;white-space:normal}.task-paging{display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin:12px 0}.task-paging[hidden]{display:none}.task-paging span{font-size:11px;color:#b9cde1} .gui-panel>.tiles{margin-top:14px}.gui-panel .tiles{grid-template-columns:1fr}.tile p{min-height:56px;font-size:12px;color:#acbbcf;line-height:1.6}.actions{display:flex;gap:9px}button{cursor:pointer;border:1px solid #406eab;background:#2b5d96;color:#fff;padding:10px 13px;border-radius:6px;font:inherit;font-size:12px;font-weight:700}button:hover{filter:brightness(1.14)}button:disabled{opacity:.42;cursor:not-allowed}.result{background:#101824;border:1px solid #303e50;border-radius:10px;margin-top:18px;overflow:hidden}.result-head{display:flex;justify-content:space-between;gap:8px;background:#192536;padding:13px 16px;font-size:12px} .result-head span{color:#aab8cb}.session-metadata{display:flex;flex-wrap:wrap;gap:10px;justify-content:space-between;background:#101b29;color:#aab8cb;padding:9px 16px;font-size:11px;border-bottom:1px solid #283649}.session-metadata strong{color:#b9defe}pre{font-size:12px;white-space:pre-wrap;overflow-wrap:anywhere;min-height:146px;max-height:350px;overflow:auto;padding:16px;margin:0;line-height:1.6;color:#e2f1fc}footer{display:flex;justify-content:space-between;align-items:center;gap:14px;margin-top:24px;color:#aab8cb;font-size:11px}.git-summary{border-top:1px solid #303e50;padding:15px}.git-branch{font-size:12px;font-weight:700;color:#a5caff;margin-bottom:9px}.git-summary table{border-collapse:collapse;width:100%;font-size:11px}.git-summary th{text-align:left;color:#aab8cb}.git-summary th,.git-summary td{padding:7px;border-bottom:1px solid #2a3647}.git-summary td:first-child{width:110px;font-family:ui-monospace,monospace;color:#f0bd75}.git-summary td:last-child{font-family:ui-monospace,monospace;overflow-wrap:anywhere}.danger{background:transparent;border:1px solid #87515f;color:#ffc7d0}@media(max-width:880px){.app{grid-template-columns:1fr}.rail{padding:12px 16px;gap:12px}.rail nav{flex-direction:row;overflow:auto}.rail-note{display:none}.workspace{gap:3px}.content{padding:20px}header{flex-wrap:wrap}  .feature-check{flex-wrap:wrap}.activation-form{flex-wrap:wrap}.target-summary,.project-overview{grid-template-columns:1fr}.tiles{grid-template-columns:1fr}.tile p{min-height:0}footer{flex-wrap:wrap}}`

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
  const project=document.getElementById('project');
  const tasksPreview=document.getElementById('tasks-preview');
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
  taskSort.value='host';
  const taskPaging=document.getElementById('task-paging');
  const taskPrev=document.getElementById('task-prev');
  const taskNext=document.getElementById('task-next');
  const taskPageText=document.getElementById('task-page');
  const taskPageSize=4;
  let cachedTaskLabels=[];
  // The bulk loader is explicitly user-started, bounded by the existing
  // 16-round native quota and sequentially awaits each sealed E2EE reply.
  let bulkLoading=false,bulkStopRequested=false;
  let remoteTaskTotal=0;
  let canFetchTaskNext=false;
  let taskPage=0;
  function filteredTaskLabels(){
    const needle=String(taskFilter.value||'').trim().toLocaleLowerCase();
    const result=needle?cachedTaskLabels.filter(
      label=>label.toLocaleLowerCase().includes(needle)):
      cachedTaskLabels.slice();
    if(taskSort.value==='az'){
      result.sort((a,b)=>a.localeCompare(b,'vi'));
    }else if(taskSort.value==='za'){
      result.sort((a,b)=>b.localeCompare(a,'vi'));
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
    for(const label of labels.slice(taskPage*taskPageSize,(taskPage+1)*taskPageSize)){
      const li=document.createElement('li');
      li.textContent=label;
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
  const remaining=document.getElementById('remaining');
  const targetDevice=document.getElementById('target-device');
  const targetUser=document.getElementById('target-user');
  const sessionClock=document.getElementById('session-clock');
  const pingLatency=document.getElementById('ping-latency');
  const sessionActivity=document.getElementById('session-activity');
  const gitSummary=document.getElementById('git-summary');
  const gitBranch=document.getElementById('git-branch');
  const gitFiles=document.getElementById('git-files');
  const viewTitle=document.getElementById('view-title');
  const viewHint=document.getElementById('view-hint');
  const navButtons={
    overview:document.getElementById('nav-overview'),
    project:document.getElementById('nav-project'),
    tasks:document.getElementById('nav-tasks'),
    git:document.getElementById('nav-git')
  };
  const panels={
    overview:document.getElementById('panel-overview'),
    project:document.getElementById('panel-project'),
    tasks:document.getElementById('panel-tasks'),
    git:document.getElementById('panel-git')
  };
  const viewDescriptions={
    overview:['Tổng quan','Quản lý phiên và kiểm tra chức năng host. Chuyển tab không tốn lượt E2EE.'],
    project:['Dự án TaskDeck','Dữ liệu hiển thị được xác thực, chỉ thuộc một dự án TaskDeck đã cấp quyền.'],
    tasks:['Task của workspace','Chỉ xem nhãn task, không gửi ID, đường dẫn hoặc lệnh thực thi.'],
    git:['Git Status','Dữ liệu Git chỉ đọc từ lần yêu cầu gần nhất; có thể đã thay đổi trên host.']
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
  function renderGit(raw){
    // Git status is HOST-validated authenticated plaintext. Never use
    // interpreted HTML, terminal escapes or remote-supplied links.
    gitFiles.replaceChildren();
    const lines=raw.split('\n').filter(Boolean);
    gitBranch.textContent=lines.length?lines[0]:'Repository sạch';
    for(const line of lines.slice(1)){
      if(line.length<4)continue;
      const row=document.createElement('tr');
      const kind=document.createElement('td');
      const file=document.createElement('td');
      kind.textContent=line.slice(0,2);
      file.textContent=line.slice(3);
      row.append(kind,file);
      gitFiles.append(row);
    }
    gitSummary.hidden=false;
  }
  let busy=false,closed=false,terminated=false,closing=false,budget=16;
  let actionEpoch=0;
  let featuresKnown=false;
  let available={session_ping:false,project_info:false,tasks_preview:false,git_status:false};
  let expiresAt=0;
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
      closed=true;
      output.textContent='Phiên đã hết thời hạn. Tạo phiên mới bằng ứng dụng native TaskDeck.';
      render();
    }
  }
  let verified=/^[a-f0-9]{64}$/.test(capability);
  // Authenticated data must not remain displayed after logout, expiration
  // or revocation. Do not persist previous remote metadata in the browser.
  function clearPrivateViews(){
    gitSummary.hidden=true;
    gitFiles.replaceChildren();
    gitBranch.textContent='';
    projectOverview.hidden=true;
    projectName.textContent='';
    workspaceName.textContent='';
    hostOS.textContent='';
    taskPreviewSection.hidden=true;
    taskLabels.replaceChildren();
    taskPaging.hidden=true;
    cachedTaskLabels=[];
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
    available={session_ping:false,project_info:false,tasks_preview:false,git_status:false};
    featureSummary.textContent='Phiên đã đóng. Đăng nhập lại bằng client TaskDeck tin cậy.';
  }
  function render(){
    if(closed)clearPrivateViews();
    ping.disabled=busy||closed||!verified||budget<=0||
      (featuresKnown&&!available.session_ping);
    features.disabled=busy||closed||!verified||budget<=0;
    git.disabled=busy||closed||!verified||budget<=0||!featuresKnown||!available.git_status;
    project.disabled=busy||closed||!verified||budget<=0||!featuresKnown||!available.project_info;
    tasksPreview.disabled=busy||closed||!verified||budget<=0||!featuresKnown||!available.tasks_preview;
    taskFetchNext.disabled=busy||bulkLoading||closed||!verified||budget<=0||
      !featuresKnown||!available.tasks_preview;
    taskFetchAll.disabled=busy||bulkLoading||closed||!verified||budget<=0||
      !featuresKnown||!available.tasks_preview;
    // Ending a bulk run does not re-render the task page. Keep visibility
    // derived from state so Stop/Resume cannot leave stale controls.
    taskFetchAll.hidden=!canFetchTaskNext||bulkLoading;
    taskStopAll.hidden=!bulkLoading;
    taskStopAll.disabled=closed||!bulkLoading;
    if(taskPreviewSection.hidden===false){renderTaskCoverage();}
    // Keep Disconnect available even after a revoked/failed operation.
    disconnect.disabled=terminated||closing||!verified;
    state.textContent=closed?'Phiên đã đóng':(!verified?'Không có mã phiên cục bộ':busy?'Đang xử lý':'Phiên cục bộ sẵn sàng');
    state.classList.toggle('bad',closed||!verified);
    remaining.textContent='Lượt thao tác còn lại: '+budget;
    sessionActivity.textContent=busy?'Đang chờ phản hồi E2EE':(closed?'Phiên không còn hợp lệ':'Không có tác vụ đang chạy');
  }
  async function invoke(endpoint){
    const isClose=endpoint==='/api/close';
    if(!verified||(busy&&!isClose)||(closed&&!isClose)||
      (!isClose&&budget<=0)||
      (!isClose&&bulkLoading&&endpoint!='/api/tasks-preview-next')||
      (isClose&&(closing||terminated)))return;
    if(isClose){closing=true;closed=true;}
    actionEpoch++;
    busy=true;render();
    try{
      const response=await fetch(endpoint,{
        method:'POST',mode:'same-origin',credentials:'omit',cache:'no-store',
        headers:{'X-TaskDeck-Local-Capability':capability}
      });
      if(!response.ok)throw new Error('TaskDeck từ chối thao tác hoặc phiên đã hết hạn (HTTP '+response.status+')');
      const data=await response.json();
      // A late read result must NEVER overwrite a user-requested logout.
      if(!isClose&&closed)return;
      if(!data.ok)throw new Error('Phản hồi xác thực không hợp lệ');
      if(endpoint==='/api/close'){
        closed=true;terminated=true;output.textContent='Đã ngắt phiên.';
      }else{
        budget=Number(data.remaining);
        if(!Number.isInteger(budget)||budget<0||budget>16)throw new Error('Số lượt không hợp lệ');
        output.textContent=endpoint==='/api/git-status'?String(data.status):
          endpoint==='/api/project-info'?'Đã đọc thông tin dự án qua E2EE.':
          (endpoint==='/api/tasks-preview'||endpoint==='/api/tasks-preview-next')?
           'Đã đọc nhãn task qua E2EE.':
          endpoint==='/api/features'?'Đã đọc các quyền GUI hiện tại qua E2EE.':String(data.message);
        if(endpoint==='/api/features'){
          const keys=['session_ping','project_info','tasks_preview','git_status'];
          if(data.remote_control_available!==false||
            keys.some(key=>typeof data[key]!=='boolean')){
            throw new Error('Phản hồi khả năng GUI không hợp lệ');
          }
          available={session_ping:data.session_ping,
            project_info:data.project_info,
            tasks_preview:data.tasks_preview,git_status:data.git_status};
          featuresKnown=true;
          featureSummary.textContent=
            'Ping: '+(available.session_ping?'Được phép':'Thiếu quyền')+
            ' · Dự án: '+(available.project_info?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Task: '+(available.tasks_preview?'Được phép':'Chưa bật hoặc thiếu quyền')+
            ' · Git: '+(available.git_status?'Được phép':'Chưa bật hoặc thiếu quyền');
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
            (appendPage&&(!canFetchTaskNext||data.total!==remoteTaskTotal||
              cachedTaskLabels.length+data.labels.length>data.total))){
            throw new Error('Trang task E2EE không hợp lệ');
          }
          if(!appendPage){
            cachedTaskLabels=data.labels.slice();
            taskFilter.value='';
            taskFilter.disabled=false;
            taskSort.disabled=false;
            taskSort.value='host';
            taskPage=0;
          }else{
            cachedTaskLabels.push(...data.labels);
          }
          remoteTaskTotal=data.total;
          canFetchTaskNext=data.labels.length>0 &&
            cachedTaskLabels.length<remoteTaskTotal;
          renderTaskPage();
          taskTotal.textContent=String(cachedTaskLabels.length)+' / '+
            String(remoteTaskTotal)+' task';
          taskPreviewSection.hidden=false;
        }
        if(endpoint==='/api/ping'){
          if(!Number.isInteger(data.round_trip_ms)||data.round_trip_ms<0||
            data.round_trip_ms>16000)throw new Error('Kết quả đo độ trễ không hợp lệ');
          pingLatency.textContent=String(data.round_trip_ms)+' ms';
        }
        if(endpoint==='/api/git-status'){renderGit(String(data.status));}
        // Other reads must not erase locally verified Git data when navigating.
        // The cached result is a non-authoritative display, never a permission.
        if(budget===0)output.textContent+='\n\nĐã sử dụng hết 16 yêu cầu. Hãy tạo phiên được xác thực mới.';
      }
    }catch(error){
      if(isClose||!closed)output.textContent=String(error.message||error);
      closed=true; // fail closed, never switch to a PHP page or direct HTTP.
    }finally{
      if(isClose)closing=false;
      busy=false;render();updateSessionClock();
    }
  }
  async function loadLocalSession(){
    if(!verified||busy||closed||closing||terminated)return;
    const observedEpoch=actionEpoch;
    try{
      const response=await fetch('/api/session',{
        method:'POST',mode:'same-origin',credentials:'omit',cache:'no-store',
        headers:{'X-TaskDeck-Local-Capability':capability}
      });
      if(!response.ok)throw new Error('Phiên E2EE đã đóng (HTTP '+response.status+')');
      const data=await response.json();
      // Local status may race with an E2EE action or Disconnect.
      // Do not overwrite a newer action's budget or re-open a closed GUI.
      if(observedEpoch!==actionEpoch||busy||closed||closing||terminated)return;
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
      if(observedEpoch===actionEpoch&&!busy&&!closed&&!closing&&!terminated){
        closed=true;
        output.textContent=String(error.message||error);
        updateSessionClock();
      }
    }finally{render();}
  }
  activate.addEventListener('click',()=>{
    const candidate=String(localToken.value||'').trim();
    localToken.value='';
    if(!/^[a-f0-9]{64}$/.test(candidate)){
      output.textContent='Mã không đúng. Hãy lấy mã 64 ký tự trực tiếp từ terminal client TaskDeck tin cậy.';
      return;
    }
    capability=candidate;
    verified=true;
    featuresKnown=false;
    available={session_ping:false,project_info:false,tasks_preview:false,git_status:false};
    featureSummary.textContent='Chưa kiểm tra. Bấm Kiểm tra chức năng để biết những chức năng được cấp phép.';
    activation.hidden=true;
    closed=false;terminated=false;closing=false;
    render();
    loadLocalSession();
  });
  features.addEventListener('click',()=>invoke('/api/features'));
  ping.addEventListener('click',()=>invoke('/api/ping'));
  git.addEventListener('click',()=>invoke('/api/git-status'));
  project.addEventListener('click',()=>invoke('/api/project-info'));
  tasksPreview.addEventListener('click',()=>invoke('/api/tasks-preview'));
  taskFetchNext.addEventListener('click',()=>{
    if(!canFetchTaskNext||bulkLoading||closed||busy||budget<=0)return;
    invoke('/api/tasks-preview-next');
  });
  async function loadRemainingTaskPages(){
    if(bulkLoading||busy||closed||terminated||budget<=0||
      !canFetchTaskNext||!featuresKnown||!available.tasks_preview)return;
    bulkLoading=true;
    bulkStopRequested=false;
    let pages=0;
    taskLoadStatus.textContent='Đang tải tuần tự qua E2EE. Có thể dừng sau yêu cầu hiện tại hoặc ngắt phiên ngay.';
    render();
    try{
      // Local and Relay both enforce the same absolute 16-round limit.
      // Never retry a failed exchange or silently create a new session.
      for(let i=0;i<16&&!bulkStopRequested&&!closed&&!terminated&&
          canFetchTaskNext&&budget>0;i++){
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
          'Đã dừng tải tự động; có thể chọn tải thêm.';
      }else{
        taskLoadStatus.textContent='';
      }
      render();
    }
  }
  taskFetchAll.addEventListener('click',()=>{loadRemainingTaskPages();});
  taskStopAll.addEventListener('click',()=>{
    if(!bulkLoading||closed)return;
    bulkStopRequested=true;
    taskLoadStatus.textContent='Đã yêu cầu dừng; không gửi thêm trang sau lượt E2EE hiện tại.';
  });
  disconnect.addEventListener('click',()=>invoke('/api/close'));
  // Best effort only: browser/OS crash cannot guarantee pagehide delivery.
  // Native E2EE is also bounded by the host session deadline.
  window.addEventListener('pagehide',()=>{
    if(terminated||!verified)return;
    closed=true;terminated=true;
    // A page can enter the back-forward cache rather than being destroyed.
    // Clear all previous E2EE data SYNCHRONOUSLY before the page is hidden.
    output.textContent='Phiên đã đóng khi rời trang.';
    render();
    fetch('/api/close',{
      method:'POST',mode:'same-origin',credentials:'omit',
      cache:'no-store',keepalive:true,
      headers:{'X-TaskDeck-Local-Capability':capability}
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
