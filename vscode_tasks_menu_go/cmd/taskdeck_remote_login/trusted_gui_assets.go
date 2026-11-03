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
    <div class="nav active"><span>01</span> Tổng quan</div>
    <div class="nav active"><span>02</span> Git · chỉ đọc</div>
    <div class="nav muted"><span>03</span> Git Pull / Commit / Push <em>Chưa bật</em></div>
    <div class="nav muted"><span>04</span> Terminal <em>Chưa bật</em></div>
    <div class="nav muted"><span>05</span> Editor / Files <em>Chưa bật</em></div>
   </nav>
   <div class="rail-note">Mật khẩu TaskDeck chỉ nhập trong client CLI đáng tin cậy, không nhập tại PHP Portal hoặc trang web khác.</div>
  </aside>
  <section class="content">
   <header>
    <div><span class="overline">TASKDECK REMOTE</span><h1>Không gian làm việc từ xa</h1><p>Giao diện cục bộ của client đã ghim danh tính Agent. Các thao tác được mã hóa đầu cuối.</p></div>
    <span class="state" id="state">Chờ xác minh phiên</span>
   </header>
   <div class="banner"><strong>Giới hạn bảo mật</strong><p>Đây là bản GUI thử nghiệm, không phải điều khiển toàn bộ TaskDeck. PHP/Relay chỉ trung chuyển ciphertext. Không có shell, Git ghi hoặc proxy TaskDeck HTTP.</p></div>
   <div class="tiles">
    <article class="tile">
     <div class="tile-heading"><span class="dot"></span><h2>Phiên E2EE</h2></div>
     <p>Kiểm tra Agent đang giữ phiên TaskDeck hợp lệ, khóa client được duyệt và quyền hiện hành.</p>
     <div class="actions"><button type="button" id="ping">Kiểm tra kết nối</button></div>
    </article>
    <article class="tile">
     <div class="tile-heading"><span class="dot"></span><h2>Git Status</h2></div>
     <p>Đọc trạng thái repository gốc trên máy TaskDeck. Chỉ hoạt động khi host bật riêng quyền đọc Git.</p>
     <div class="actions"><button type="button" id="git">Đọc trạng thái Git</button></div>
    </article>
   </div>
   <section class="result">
    <div class="result-head"><strong>Phản hồi được xác thực</strong><span id="remaining">Tối đa 4 yêu cầu mỗi phiên</span></div>
    <pre id="output" role="status" aria-live="polite">Chọn một thao tác ở trên. Không có dữ liệu nào được tải từ PHP Portal.</pre>
    <div class="git-summary" id="git-summary" hidden>
     <div class="git-branch" id="git-branch"></div>
     <table aria-label="Các tệp thay đổi trên Git"><thead><tr><th>Trạng thái</th><th>Đường dẫn tệp</th></tr></thead><tbody id="git-files"></tbody></table>
    </div>
   </section>
   <footer>
    <span id="security">Origin 127.0.0.1 • local capability • strict CSP</span>
    <button type="button" class="danger" id="disconnect">Ngắt phiên</button>
   </footer>
  </section>
 </main>
</body>
</html>`

const trustedGUICSS = `:root{color-scheme:dark;font-family:ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#0b1018;color:#e9f0f9}*{box-sizing:border-box}body{margin:0}.app{min-height:100vh;display:grid;grid-template-columns:268px minmax(0,1fr)}.rail{background:#101824;border-right:1px solid #273344;padding:22px 15px;display:flex;flex-direction:column;gap:24px}.logo{font-size:24px;font-weight:900;letter-spacing:-2px;color:#8cc1ff}.logo span{color:#c2f0dc}.workspace{display:flex;flex-direction:column;gap:8px}.workspace strong{font-size:19px}.workspace small{color:#a4b1c4}.overline{color:#78b6f5;font-size:10px;font-weight:800;letter-spacing:1.8px}.rail nav{display:flex;flex-direction:column;gap:6px}.nav{border-radius:7px;padding:12px 10px;font-size:12px;display:flex;align-items:center;gap:9px}.nav span{font-size:10px;opacity:.7}.nav.active{background:#1d3147}.nav.muted{opacity:.53;flex-wrap:wrap}.nav em{font-size:10px;margin-left:auto}.rail-note{font-size:11px;color:#aab8cb;line-height:1.6;margin-top:auto;border-top:1px solid #293445;padding-top:14px}.content{max-width:1200px;width:100%;padding:36px 42px;margin:auto}header{display:flex;align-items:flex-start;justify-content:space-between;gap:20px}h1{font-size:28px;letter-spacing:-.8px;margin:10px 0}header p{font-size:13px;color:#aab8cb;line-height:1.5;max-width:600px}.state{border:1px solid #517a78;color:#afeee0;background:#123834;padding:8px 11px;border-radius:50px;font-size:11px;white-space:nowrap}.state.bad{border-color:#8d5354;color:#ffd3d3;background:#432326}.banner{margin:28px 0;border-left:3px solid #d3aa68;background:#242333;padding:15px 18px;border-radius:6px}.banner strong{font-size:12px;color:#ffe0a7}.banner p{font-size:12px;color:#c6cfdf;line-height:1.5;margin-bottom:0}.tiles{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.tile{border:1px solid #303e50;background:#172130;border-radius:10px;padding:20px}.tile-heading{display:flex;gap:9px;align-items:center}.tile-heading h2{margin:0;font-size:16px}.dot{height:8px;width:8px;border-radius:50%;background:#83bdff}.tile p{min-height:56px;font-size:12px;color:#acbbcf;line-height:1.6}.actions{display:flex;gap:9px}button{cursor:pointer;border:1px solid #406eab;background:#2b5d96;color:#fff;padding:10px 13px;border-radius:6px;font:inherit;font-size:12px;font-weight:700}button:hover{filter:brightness(1.14)}button:disabled{opacity:.42;cursor:not-allowed}.result{background:#101824;border:1px solid #303e50;border-radius:10px;margin-top:18px;overflow:hidden}.result-head{display:flex;justify-content:space-between;gap:8px;background:#192536;padding:13px 16px;font-size:12px}.result-head span{color:#aab8cb}pre{font-size:12px;white-space:pre-wrap;overflow-wrap:anywhere;min-height:146px;max-height:350px;overflow:auto;padding:16px;margin:0;line-height:1.6;color:#e2f1fc}footer{display:flex;justify-content:space-between;align-items:center;gap:14px;margin-top:24px;color:#aab8cb;font-size:11px}.git-summary{border-top:1px solid #303e50;padding:15px}.git-branch{font-size:12px;font-weight:700;color:#a5caff;margin-bottom:9px}.git-summary table{border-collapse:collapse;width:100%;font-size:11px}.git-summary th{text-align:left;color:#aab8cb}.git-summary th,.git-summary td{padding:7px;border-bottom:1px solid #2a3647}.git-summary td:first-child{width:110px;font-family:ui-monospace,monospace;color:#f0bd75}.git-summary td:last-child{font-family:ui-monospace,monospace;overflow-wrap:anywhere}.danger{background:transparent;border:1px solid #87515f;color:#ffc7d0}@media(max-width:880px){.app{grid-template-columns:1fr}.rail{padding:12px 16px;gap:12px}.rail nav{flex-direction:row;overflow:auto}.rail-note{display:none}.workspace{gap:3px}.content{padding:20px}header{flex-wrap:wrap}.tiles{grid-template-columns:1fr}.tile p{min-height:0}footer{flex-wrap:wrap}}`

const trustedGUIJavaScript = `'use strict';
(function(){
  const capability=location.hash.slice(1);
  // Keep the one-use localhost capability in JS memory, never in the
  // referrer, browser navigation, Portal URLs, cookies or persistent storage.
  history.replaceState(null,'',location.pathname);
  const ping=document.getElementById('ping');
  const git=document.getElementById('git');
  const disconnect=document.getElementById('disconnect');
  const state=document.getElementById('state');
  const output=document.getElementById('output');
  const remaining=document.getElementById('remaining');
  const gitSummary=document.getElementById('git-summary');
  const gitBranch=document.getElementById('git-branch');
  const gitFiles=document.getElementById('git-files');
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
  let busy=false,closed=false,budget=4;
  const verified=/^[a-f0-9]{64}$/.test(capability);
  function render(){
    ping.disabled=busy||closed||!verified||budget<=0;
    git.disabled=busy||closed||!verified||budget<=0;
    disconnect.disabled=closed||!verified;
    state.textContent=closed?'Phiên đã đóng':(!verified?'Không có mã phiên cục bộ':busy?'Đang xử lý':'Phiên cục bộ sẵn sàng');
    state.classList.toggle('bad',closed||!verified);
    remaining.textContent='Lượt thao tác còn lại: '+budget;
  }
  async function invoke(endpoint){
    if(busy||closed||!verified)return;
    busy=true;render();
    try{
      const response=await fetch(endpoint,{
        method:'POST',mode:'same-origin',credentials:'omit',cache:'no-store',
        headers:{'X-TaskDeck-Local-Capability':capability}
      });
      if(!response.ok)throw new Error('TaskDeck từ chối thao tác hoặc phiên đã hết hạn (HTTP '+response.status+')');
      const data=await response.json();
      if(!data.ok)throw new Error('Phản hồi xác thực không hợp lệ');
      if(endpoint==='/api/close'){
        closed=true;output.textContent='Đã ngắt phiên.';
      }else{
        budget=Number(data.remaining);
        if(!Number.isInteger(budget)||budget<0||budget>4)throw new Error('Số lượt không hợp lệ');
        output.textContent=endpoint==='/api/git-status'?String(data.status):String(data.message);
        if(endpoint==='/api/git-status'){renderGit(String(data.status));}
        else{gitSummary.hidden=true;gitFiles.replaceChildren();}
        if(budget===0)output.textContent+='\n\nĐã sử dụng hết 4 yêu cầu. Hãy tạo phiên được xác thực mới.';
      }
    }catch(error){
      output.textContent=String(error.message||error);
      closed=true; // fail closed, never switch to a PHP page or direct HTTP.
    }finally{busy=false;render();}
  }
  ping.addEventListener('click',()=>invoke('/api/ping'));
  git.addEventListener('click',()=>invoke('/api/git-status'));
  disconnect.addEventListener('click',()=>invoke('/api/close'));
  if(!verified)output.textContent='Thiếu mã capability cục bộ. Mở lại đúng URL được client TaskDeck tin cậy in ra.';
  render();
})();`
