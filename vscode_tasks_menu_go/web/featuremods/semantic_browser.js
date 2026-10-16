// Semantic Browser frontend. Remote page scripts and HTML MUST NOT execute in
// TaskDeck: only bounded, typed JSON becomes local textContent/native controls.
const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Semantic Browser');
const views=new Map();
const tabsHost=document.querySelector('#tabs'),panesHost=document.querySelector('#panes');
let browserEnabled=false;
const style=document.createElement('style');
style.textContent=`
.semantic-browser-tab{border-radius:6px 6px 0 0;border-bottom:0;margin-left:4px}
.semantic-browser-tab.active{background:#343b48}
.semantic-browser-tab .close{margin-left:8px;opacity:.75}
.semantic-browser-pane{background:#10151c;color:inherit}
.semantic-browser-toolbar{display:flex;align-items:center;gap:5px;padding:7px;border-bottom:1px solid #30343b}
.semantic-browser-toolbar button{padding:5px 8px}
.semantic-browser-address{flex:1;min-width:110px;padding:7px;border-radius:6px;background:#090e15;color:inherit;border:1px solid #465064;font:12px ui-monospace,monospace}
.semantic-browser-content{flex:1;overflow:auto;min-height:0;padding:16px 20px;font:13px/1.6 system-ui,sans-serif}
.semantic-browser-entry{margin:6px 0;max-width:100%;overflow-wrap:anywhere}
.semantic-browser-action{display:inline-flex;max-width:100%;padding:5px 9px;text-align:left;white-space:normal;align-items:center;gap:6px}
.semantic-browser-link{color:#84bfff;background:transparent;border:0;text-decoration:underline;padding:1px 2px}
.semantic-browser-field{display:flex;flex-direction:column;max-width:720px;gap:4px;margin:8px 0}
.semantic-browser-field input,.semantic-browser-field textarea,.semantic-browser-field select{background:#0b111a;color:inherit;border:1px solid #465064;border-radius:5px;padding:7px;min-width:120px}
.semantic-browser-field textarea{min-height:75px;resize:vertical;font:12px/1.45 ui-monospace,monospace}
.semantic-browser-field label,.semantic-browser-field>span{font-size:11px;opacity:.8}
.semantic-browser-heading{font-size:17px;font-weight:700;margin:15px 0 6px}
.semantic-browser-muted{opacity:.64;font-size:11px}
.semantic-browser-error{color:#ff9a9a;white-space:pre-wrap}
.semantic-browser-busy{opacity:.75}
.semantic-browser-preview{max-width:100%;max-height:660px;object-fit:contain;border:1px solid #465064;border-radius:6px;margin-top:10px}
.semantic-browser-preview-note{font-size:11px;opacity:.8;margin-top:7px}
.semantic-browser-windows{display:flex;gap:5px;align-items:center;flex-wrap:wrap;padding:5px 8px;border-bottom:1px solid #30343b}
.semantic-browser-window{font-size:11px;padding:5px 8px;max-width:230px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.semantic-browser-window.active{border-color:#659fda;background:#254363}
.semantic-browser-window-close{font-size:12px;padding:4px 8px}
.semantic-browser-dialog{padding:12px;border:1px solid #65778c;border-radius:7px;background:#17212d;margin:10px 0}
.semantic-browser-dialog strong{display:block;margin-bottom:6px}
.semantic-browser-dialog-text{white-space:pre-wrap;word-break:break-word}
.semantic-browser-dialog-actions{display:flex;gap:7px;margin-top:8px}
.semantic-browser-dialog input{padding:7px;background:#0b111a;color:inherit;border:1px solid #465064;max-width:100%;width:100%;margin-top:8px}
.semantic-browser-popup-status{font-size:11px;opacity:.78;padding:6px 8px}
.semantic-browser-auth-help{padding:8px 10px;border-left:3px solid #b49a67;font-size:11px;margin:8px 0;background:rgba(180,154,103,.1)}
.semantic-browser-info-dialog{background:#171e29;color:#ebeff5;border:1px solid #536078;border-radius:10px;padding:18px 20px;width:min(580px,calc(100vw - 28px));max-height:80vh;overflow:auto;font:12px/1.55 system-ui,sans-serif}
.semantic-browser-info-dialog::backdrop{background:rgba(0,0,0,.7)}
.semantic-browser-setup-wizard{width:min(820px,calc(100vw - 28px))}
.semantic-browser-wizard-nav{display:flex;gap:5px;flex-wrap:wrap;margin:12px 0}
.semantic-browser-wizard-nav button.active{border-color:#8dbcf3;background:#284e7d}
.semantic-browser-wizard-body{min-height:180px;max-height:53vh;overflow-y:auto}
.semantic-browser-wizard-command{border:1px solid #48596c;border-radius:7px;padding:10px;margin:10px 0}
.semantic-browser-wizard-command strong{display:block;margin-bottom:7px}
.semantic-browser-wizard-command pre{white-space:pre-wrap;overflow-wrap:anywhere;user-select:text;background:#101721;border-radius:5px;padding:10px}
.semantic-browser-wizard-footer{display:flex;gap:8px;flex-wrap:wrap;margin:10px 0}
.semantic-browser-wizard-check{margin:5px 0}
.semantic-browser-info-status{font-size:12px;color:#9dbff0}
.semantic-browser-auto-installer{border:1px solid #455b76;border-radius:8px;padding:12px;margin-top:12px}
.semantic-browser-installer-logs{max-height:180px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;font-size:11px;background:#101721;padding:10px}

.semantic-browser-info-dialog h3{margin:0 0 12px;font-size:17px}
.semantic-browser-info-dialog p{margin:8px 0}
.semantic-browser-info-dialog a{color:#91c6ff}
.semantic-browser-info-actions{display:flex;justify-content:flex-end;margin-top:14px}
.semantic-browser-info-field{display:flex;flex-direction:column;gap:6px;margin:14px 0}
.semantic-browser-info-field input{width:100%;padding:8px;border:1px solid #536078;border-radius:5px;background:#0b111a;color:inherit}
.semantic-browser-info-checkbox{display:flex;align-items:center;gap:7px;margin:12px 0}
.semantic-browser-info-error{color:#ff9a9a}
html[data-taskmenu-theme="light"] .semantic-browser-info-dialog{background:#fff;color:#202124}
html[data-taskmenu-theme="light"] .semantic-browser-pane{background:#fff;color:#202124}
html[data-taskmenu-theme="light"] .semantic-browser-address,html[data-taskmenu-theme="light"] .semantic-browser-field input,html[data-taskmenu-theme="light"] .semantic-browser-field textarea,html[data-taskmenu-theme="light"] .semantic-browser-field select{background:#f8fafc;color:#202124;border-color:#bbc4cc}
`;
document.head.append(style);
function el(tag,cls='',text=''){const node=document.createElement(tag);if(cls)node.className=cls;if(text)node.textContent=text;return node;}
function button(text,fn,cls=''){const node=el('button',cls,text);node.type='button';node.onclick=fn;return node;}
// The Browser entry point is installed by header menus between Files and Terminal.
function storageKey(){
  return ['taskdeck:semantic-browser:v1',app.taskData?.workspace||'',app.currentUser?.user_id||'local'].map(encodeURIComponent).join(':');
}

const browserPreferencesDefault=Object.freeze({home:'',autoRefresh:true});
function browserPreferencesKey(){
  return ['taskdeck:semantic-browser:prefs:v1',app.taskData?.workspace||'',app.currentUser?.user_id||'local'].map(encodeURIComponent).join(':');
}
function browserPreferences(){
  try {
    const data=JSON.parse(localStorage.getItem(browserPreferencesKey())||'null');
    if(!data||typeof data!=='object'||Array.isArray(data))return {...browserPreferencesDefault};
    return {
      home:typeof data.home==='string'?data.home.slice(0,2048):'',
      autoRefresh:data.autoRefresh!==false
    };
  }catch{return {...browserPreferencesDefault};}
}
function validateBrowserHome(value){
  value=String(value||'').trim();
  if(!value)return '';
  let parsed;
  try{parsed=new URL(value);}catch{throw new Error('Browser homepage must be a valid HTTPS URL');}
  if(parsed.protocol!=='https:'||parsed.username||parsed.password||parsed.search||parsed.hash||value.length>2048){
    throw new Error('Use an HTTPS homepage without credentials, query parameters or fragments');
  }
  return parsed.href;
}
function saveBrowserPreferences(prefs){
  const next={home:validateBrowserHome(prefs.home),autoRefresh:prefs.autoRefresh!==false};
  localStorage.setItem(browserPreferencesKey(),JSON.stringify(next));
  return next;
}
function browserInfoDialog(title,build){
  document.querySelector('#semantic-browser-info-dialog')?.remove();
  const dialog=el('dialog','semantic-browser-info-dialog');dialog.id='semantic-browser-info-dialog';
  const heading=el('h3','',title);dialog.append(heading);
  build(dialog);
  const actions=el('div','semantic-browser-info-actions');
  actions.append(button('Close',()=>dialog.close()));
  dialog.append(actions);
  document.body.append(dialog);
  dialog.addEventListener('close',()=>dialog.remove(),{once:true});
  dialog.showModal();
  return {dialog,actions};
}
function addBrowserInfoLine(parent,text,tag='p'){
  const p=el(tag,'',text);parent.append(p);return p;
}
function openSetupGuide(){
  // The OS shown here belongs to the TaskDeck SERVER, not the remote viewer.
  // Secrets are generated and retained server-side; never put them in DOM.
  const current={step:0,diagnostics:null,notice:'',busy:false,hosts:'',installer:null,manualOnly:false,acknowledged:false,autoStart:false};
  return browserInfoDialog('Semantic Browser · Setup wizard',dialog=>{
    dialog.classList.add('semantic-browser-setup-wizard');
    const intro=addBrowserInfoLine(dialog,'Install the separate Chromium worker on the TaskDeck host. Socket/token are generated securely in TaskDeck; no manual environment variables are required.');
    intro.className='semantic-browser-muted';
    const nav=el('div','semantic-browser-wizard-nav');
    const headings=['1 · Kiểm tra máy','2 · Cấu hình','3 · Cài worker','4 · Xác minh'];
    const content=el('div','semantic-browser-wizard-body');
    const footer=el('div','semantic-browser-wizard-footer');
    const status=el('p','semantic-browser-info-status');
    const tabs=headings.map((name,index)=>button(name,()=>{current.step=index;paint();},'secondary'));
    nav.append(...tabs);dialog.append(nav,content,footer,status);
    function line(parent,value){return addBrowserInfoLine(parent,value);}
    function makeCommand(label,command){
      const group=el('div','semantic-browser-wizard-command');
      group.append(el('strong','',label));
      const pre=el('pre','',command);group.append(pre);
      group.append(button('Copy command',async()=>{
        try{await navigator.clipboard.writeText(command);status.textContent='Command copied';}
        catch{status.textContent='Clipboard blocked; select the command above and copy it manually';}
      },'secondary'));
      return group;
    }
    function nodeStatus(label,okay,details=''){
      const row=el('p','semantic-browser-wizard-check', (okay?'✓ ':'• ')+label+(details?' — '+details:''));
      return row;
    }
    const refresh=async()=>{
      if(current.busy)return;
      current.busy=true;status.textContent='Checking TaskDeck host…';
      try{
        current.diagnostics=await app.jsonFetch('/api/semantic-browser/setup',{cache:'no-store'});
        status.textContent='Host information updated';
      }catch(error){
        status.textContent='Cannot read server setup: '+errorOf(error);
        current.diagnostics=null;
      }finally{current.busy=false;paint();}
    };
    function paint(){
      content.replaceChildren();footer.replaceChildren();
      tabs.forEach((tab,i)=>{tab.disabled=current.busy;tab.classList.toggle('active',i===current.step);});
      const data=current.diagnostics;
      if(!data){
        line(content,'Cannot fetch host diagnostics. The wizard requires settings.write and browser.use on shared installations. Ask an administrator to configure the worker.');
        line(content,'Linux/macOS: install Node.js 20+ and Playwright Chromium, then launch the worker with the TaskDeck-managed private config file. Windows: install WSL2 and run TaskDeck plus the worker inside Ubuntu.');
        const docs=el('a','','Open complete setup and security guide');
        docs.href='https://github.com/hacrot3000/PatchAndCollectionToolForAI/blob/feature/semantic-browser-tab/vscode_tasks_menu_go/browser-worker/README.md';
        docs.target='_blank';docs.rel='noopener noreferrer';content.append(docs);
        footer.append(button('Retry detection',()=>{void refresh();}));
        return;
      }
      const osName={linux:'Linux',wsl:'Windows · WSL2',windows:'Windows host',darwin:'macOS',container:'Linux container'}[data.platform]||data.platform;
      if(current.step===0){
        content.append(el('h4','','Host diagnostic'));
        content.append(nodeStatus('TaskDeck host OS',true,osName));
        content.append(nodeStatus('Node.js 20+',data.node_supported,data.node_version||'not installed'));
        content.append(nodeStatus('npm',data.npm_installed,data.npm_version||'not installed'));
        content.append(nodeStatus('Docker (optional)',data.docker_installed,data.docker_version||'not installed'));
        content.append(nodeStatus('Worker package',Boolean(data.worker_dir),data.worker_dir||'not installed beside TaskDeck'));
        if(data.worker_dir)line(content,'Auto-detected worker: '+data.worker_dir);
        content.append(nodeStatus('Playwright npm package',data.playwright_installed,data.playwright_installed?'installed':'not detected'));
        content.append(nodeStatus('Socket reachable',data.worker_connected,data.worker_connected?'connected':'not running / inaccessible'));
        if(data.platform==='windows')line(content,'Windows native TaskDeck cannot use this Unix socket worker directly. Install WSL2 and run both TaskDeck and the worker in WSL; reopen this wizard from that instance.');
        if(data.platform==='container')line(content,'TaskDeck is running in a container. The isolated worker must share the private Unix-socket mount; protect its network with an independent firewall.');
        footer.append(button('Recheck',()=>{void refresh();}),button('Next: Configure',()=>{current.step=1;paint();}));
      }else if(current.step===1){
        content.append(el('h4','','Worker network and connection settings'));
        line(content,'Enter only HTTPS website domains you trust. Include required login/OAuth/CDN hosts. This allowlist does not replace OS/container egress firewall rules.');
        const field=el('label','semantic-browser-info-field');field.append(el('span','','Allowed hosts (comma-separated)'));
        const hosts=el('input');hosts.id='semantic-browser-allowed-hosts';hosts.placeholder='example.com,accounts.example.com';hosts.autocomplete='off';
        hosts.value=current.hosts||(data.allowed_hosts||[]).join(',');
        hosts.addEventListener('input',()=>{current.hosts=hosts.value;});
        field.append(hosts);content.append(field);
        line(content,'TaskDeck generates a strong random token, a private Unix socket path, and a mode-0600 settings file. The secret will never appear in this wizard.');
        content.append(nodeStatus('Configuration source',data.configured, data.source==='environment'?'Legacy environment variables (override managed file)':data.source));
        if(data.source==='environment')line(content,'Legacy environment variables are currently overriding managed configuration. Remove them and restart TaskDeck if you want the wizard settings to take effect.');
        if(data.source==='environment')line(content,'Saving managed settings is disabled until the legacy environment variables are removed.');
        if(data.go_os==='windows'){
          line(content,'Open TaskDeck from WSL2 to enable automatic worker configuration.');
        }else if(data.source!=='environment'){
          const save=button('Generate and save socket/token',async()=>{
            if(current.busy)return;
            current.busy=true;save.disabled=true;
            status.textContent='Saving private configuration…';
            try{
              const result=await app.jsonFetch('/api/semantic-browser/setup',{
                method:'POST',headers:{'Content-Type':'application/json'},
                body:JSON.stringify({allowed_hosts:hosts.value.trim()})
              });
              current.diagnostics=result;
              status.textContent='Saved securely. The token is not shown. Restart the worker after configuration changes.';
              current.step=2;
            }catch(error){status.textContent='Setup failed: '+errorOf(error);}
            finally{current.busy=false;paint();}
          });
          footer.append(save);
        }
        footer.append(button('Next: Install worker',()=>{current.step=2;paint();},'secondary'));
      }else if(current.step===2){
        content.append(el('h4','','Installation commands for '+osName));
        if(!data.node_supported)line(content,'Node.js 20 or newer is required. Install it on the TaskDeck host first (not on the computer only displaying the web UI).');
        if(data.platform==='windows')line(content,'Run the first command as Administrator in PowerShell, then open the installed Ubuntu terminal.');
        else line(content,'Execute these commands on the TaskDeck host, in a terminal. The worker must run under a restricted user, not root, with no project mounts and an egress firewall.');
        if(!data.worker_dir)line(content,'The running TaskDeck installation has no detectable browser-worker files. Use the repository checkout command below, or update to a release that bundles the optional worker.');
        else line(content,'Worker directory was found automatically: '+data.worker_dir);
        for(const item of data.commands||[])content.append(makeCommand(item.label,item.command));
        if(!data.configured)line(content,'Save the settings in step 2 first: the worker start command becomes available after secure configuration.');
        if(data.configured)line(content,'The launcher reads the same private configuration as TaskDeck; do not export TASKDECK_BROWSER_WORKER_TOKEN or print it in logs.');
        const automatic=el('div','semantic-browser-auto-installer');
        automatic.append(el('h4','','Cài đặt tự động có thể dừng / Auto installer'));
        line(automatic,'TaskDeck sẽ tạo worker.json nếu thiếu, tự tìm/tải worker, chạy npm và cài Chromium. Lệnh cài hệ thống cần sudo vẫn làm thủ công.');
        const job=current.installer;
        if(job&&job.state){
          line(automatic,'Trạng thái: '+job.state+' · '+(job.stage||'')+' — '+(job.message||''));
          if(Array.isArray(job.logs)&&job.logs.length){
            const logs=el('pre','semantic-browser-installer-logs',job.logs.join('\n'));automatic.append(logs);
          }
          if(job.state==='manual'&&job.manual_command)automatic.append(makeCommand('Tiếp tục thủ công từ bước dừng',job.manual_command));
          if(job.state==='ready')line(automatic,'Worker đã chạy và xác thực thành công.');
          if(job.state==='manual'&&job.active)line(automatic,'Đang chờ tiến trình cũ dừng hẳn; không thể khởi động đợt cài mới ngay lúc này.');
        }
        if(!current.manualOnly&&!job?.active&&job?.state!=='running'&&job?.state!=='ready'){
          const domain=el('label','semantic-browser-info-field');
          domain.append(el('span','','Trusted hosts (dùng khi chưa có worker.json)'));
          const domainInput=el('input');
          domainInput.placeholder='example.com,accounts.example.com';
          domainInput.value=current.hosts||(data.allowed_hosts||[]).join(',');
          domainInput.addEventListener('input',()=>{current.hosts=domainInput.value;});
          domain.append(domainInput);automatic.append(domain);
          const launch=el('label','semantic-browser-info-checkbox');
          const startInput=el('input');startInput.type='checkbox';startInput.checked=current.autoStart;
          startInput.addEventListener('change',()=>{current.autoStart=startInput.checked;});
          launch.append(startInput,el('span','','Tự khởi động worker sau khi cài (tùy chọn)'));automatic.append(launch);
          const isolation=el('label','semantic-browser-info-checkbox');
          const isolationInput=el('input');isolationInput.type='checkbox';isolationInput.checked=current.acknowledged;
          isolationInput.addEventListener('change',()=>{current.acknowledged=isolationInput.checked;});
          isolation.append(isolationInput,el('span','','Tôi xác nhận môi trường chạy Chromium đã được cô lập và có firewall chặn mạng nội bộ. Không dùng root.'));automatic.append(isolation);
          const start=button('Cài đặt tự động',async()=>{
            if(current.busy)return;
            if(current.autoStart&&!current.acknowledged){
              status.textContent='Muốn khởi động tự động phải xác nhận đã cấu hình sandbox/firewall.';return;
            }
            current.busy=true;start.disabled=true;
            try{
              current.installer=await app.jsonFetch('/api/semantic-browser/setup/run',{
                method:'POST',headers:{'Content-Type':'application/json'},
                body:JSON.stringify({action:'start',allowed_hosts:current.hosts||domainInput.value.trim(),
                  start_worker:current.autoStart,acknowledge_isolation:current.acknowledged})
              });
              status.textContent='Bắt đầu tự động. Có thể dừng bất cứ lúc nào.';
            }catch(error){status.textContent='Không bắt đầu được: '+errorOf(error);}
            finally{current.busy=false;paint();}
          });
          automatic.append(start);
        }
        if(job?.state==='running'||job?.state==='ready'){
          automatic.append(button(job.state==='ready'?'Dừng worker · Chuyển thủ công':'Dừng tự động · Chuyển thủ công',async()=>{
            try{
              current.installer=await app.jsonFetch('/api/semantic-browser/setup/run',{
                method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'cancel'})
              });
              current.manualOnly=true;status.textContent='Đã yêu cầu hủy. Bước đang chạy sẽ dừng.';
            }catch(error){status.textContent='Lỗi hủy: '+errorOf(error);}
            paint();
          }));
        }else{
          automatic.append(button(current.manualOnly?'Bật lại chế độ tự động':'Chuyển sang thủ công',()=>{
            current.manualOnly=!current.manualOnly;paint();
          },'secondary'));
        }
        content.append(automatic);
        footer.append(button('Next: Verify',()=>{current.step=3;void refresh();}));
      }else{
        content.append(el('h4','','Verification'));
        content.append(nodeStatus('Worker settings saved',data.configured,data.source||'none'));
        content.append(nodeStatus('Worker Unix socket reachable',data.worker_connected));
        content.append(nodeStatus('Current allowed hosts',(data.allowed_hosts||[]).length>0,(data.allowed_hosts||[]).join(', ')));
        line(content,data.worker_connected?
          'Worker socket responds. Use Browser → Check availability, then open an HTTPS site from your allowlist. This does not certify firewall/sandbox isolation.':
          'Worker is not yet reachable. Follow the installation commands, ensure both processes access the same private socket, then check again.');
        footer.append(button('Check again',()=>{void refresh();}),
          button('Show installation commands',()=>{current.step=2;paint();},'secondary'));
      }
    }
    async function pollInstaller(){
      try{
        const next=await app.jsonFetch('/api/semantic-browser/setup/run',{cache:'no-store'});
        if(!dialog.isConnected)return;
        const previous=current.installer?.state;
        const before=JSON.stringify(current.installer);
        current.installer=next;
        if(previous!=='manual'&&next.state==='manual')current.manualOnly=true;
        if(current.step===2&&before!==JSON.stringify(next))paint();
        if(previous!==next.state&&(next.state==='manual'||next.state==='ready'))void refresh();
      }catch(error){if(current.step===2)status.textContent='Cannot query installer: '+errorOf(error);}
    }
    const pollTimer=setInterval(()=>{void pollInstaller();},1250);
    dialog.addEventListener('close',()=>clearInterval(pollTimer),{once:true});
    void refresh().then(()=>pollInstaller());
  });
}
function openSettings(){
  const prefs=browserPreferences();
  return browserInfoDialog('Semantic Browser · Settings',dialog=>{
    addBrowserInfoLine(dialog,'Homepage and refresh preferences are local. Worker installation, socket/token and allowed domains are configured through the setup wizard.');
    dialog.append(button('Worker setup wizard',()=>{dialog.close();openSetupGuide();},'secondary'));
    const label=el('label','semantic-browser-info-field');
    label.append(el('span','','Default homepage (HTTPS, optional)'));
    const home=el('input');home.type='url';home.placeholder='https://example.com';home.value=prefs.home;home.autocomplete='off';
    label.append(home);dialog.append(label);
    const refreshLabel=el('label','semantic-browser-info-checkbox');
    const auto=el('input');auto.type='checkbox';auto.checked=prefs.autoRefresh;
    refreshLabel.append(auto,el('span','','Automatically refresh visible page controls (every 4 seconds)'));dialog.append(refreshLabel);
    addBrowserInfoLine(dialog,'Disabling refresh does not disable website JavaScript. You can still use Go/Reload manually. Browser contexts expire after inactivity; sessions and logins are not durable across worker or TaskDeck restarts.');
    const save=button('Save settings',()=>{
      try{
        saveBrowserPreferences({home:home.value,autoRefresh:auto.checked});
        dialog.close();
      }catch(error){const notice=dialog.querySelector('.semantic-browser-info-error')||el('p','semantic-browser-info-error');notice.textContent=error.message;dialog.append(notice);}
    });
    dialog.append(save);
  });
}
async function refreshAvailability(){
  const hasPermission=!app.sharedMode||Boolean(app.hasPermission?.('browser.use'));
  // Shared-server authorization protects even GET /api/semantic-browser.
  // Do not issue a denied request or claim the server worker is unconfigured.
  if(!hasPermission){
    browserEnabled=false;
    return {configured:null,permitted:false,enabled:false};
  }
  const status=await app.jsonFetch('/api/semantic-browser',{cache:'no-store'});
  browserEnabled=Boolean(status?.enabled);
  return {configured:Boolean(status?.enabled),permitted:true,enabled:browserEnabled};
}
async function checkAvailability(){
  let data=null,error='';
  try{data=await refreshAvailability();}catch(err){error=errorOf(err);}
  return browserInfoDialog('Semantic Browser · Availability',dialog=>{
    if(error){addBrowserInfoLine(dialog,'Cannot check browser service: '+error);return;}
    addBrowserInfoLine(dialog,data.configured===null?'Worker configuration is not disclosed without browser.use permission.':(data.configured?'Worker socket is configured in TaskDeck.':'Worker socket is NOT configured. Browser cannot open yet.'));
    addBrowserInfoLine(dialog,data.permitted?'Your account may use browser.use.':'Your account lacks browser.use. Request permission from the project admin.');
    addBrowserInfoLine(dialog,data.enabled?'Browser tabs can be opened. Actual Chromium connectivity is checked when browsing.':'Browser is unavailable until the configuration and permission requirements are met.');
    const guide=button('Installation / setup instructions',()=>{dialog.close();openSetupGuide();});dialog.append(guide);
  });
}
async function openBrowser(){
  await ready;
  const status=await refreshAvailability();
  if(!status.enabled){openSetupGuide();return null;}
  return createView();
}
function persist(){
  const state=[...views.values()].filter(v=>v.sessionID).map(v=>({id:v.id,session_id:v.sessionID}));
  try{sessionStorage.setItem(storageKey(),JSON.stringify(state));}catch{}
}
function errorOf(error){return String(error?.message||error||'Unknown browser error').slice(0,500);}
function showError(view,error){
  if(!view||!view.pane.isConnected){app.showError?.(error);return;}
  view.error.hidden=false;view.error.textContent=errorOf(error);
}
function showBusy(view,active){
  view.loading=Boolean(active);
  for(const button of [view.back,view.forward,view.reload,view.preview,view.goto])button.disabled=active||(!view.sessionID&&button!==view.goto);
  view.address.disabled=active;
  view.status.textContent=active?'Loading…':String(view.page?.title||'Semantic browser');
  view.pane.classList.toggle('semantic-browser-busy',Boolean(active));
}
function setCurrent(view){
  if(app.activateExternalView('semantic-browser:'+view.id,{force:true})===false)return;
  for(const item of views.values()){
    item.tab.classList.toggle('active',item===view);
    item.pane.classList.toggle('hidden',item!==view);
  }
}
window.addEventListener('taskmenu:view-activated',event=>{
  if(event.detail?.kind==='external'&&String(event.detail.id||'').startsWith('semantic-browser:'))return;
  for(const view of views.values()){
    view.tab.classList.remove('active');
    view.pane.classList.add('hidden');
  }
});
function clearAuto(view){clearTimeout(view.autoTimer);view.autoTimer=0;}
function controlKey(item){return String(item.frame_index||0)+'/'+String(item.id);}
function rememberInput(view,item,value){
  clearAuto(view); // A secret field must cancel an earlier username timer.
  view.dirty.set(controlKey(item),{element_id:item.id,frame_index:item.frame_index||0,kind:'fill',value:String(value)});
  if(!item.sensitive&&!view.composing)scheduleAuto(view);
}
function scheduleAuto(view){
  clearAuto(view);
  // Sync nonsecret inputs after a pause so autocomplete/AJAX sites can react.
  // Never auto-send passwords or one-time codes.
  if([...view.dirty.values()].some(item=>view.page?.elements?.find(e=>e.id===item.element_id&&(e.frame_index||0)===(item.frame_index||0))?.sensitive))return;
  view.autoTimer=setTimeout(()=>{
    if([...view.dirty.values()].some(field=>
      view.page?.elements?.find(e=>e.id===field.element_id&&(e.frame_index||0)===(field.frame_index||0))?.sensitive))return;
    enqueue(view,'interact',{target_action:'',auto:true});
  },650);
}
function ensureFresh(view){
  if(!view.tab.isConnected)throw new Error('Browser tab was closed');
  if(!view.sessionID)throw new Error('Navigate to a website first');
}
function pendingUpdates(view){
  return [...view.dirty.values()].map(item=>({...item}));
}
// Each user gesture and snapshot is serialized per tab. Unlike the earlier
// implementation, clicking Submit cannot be dropped while blur syncs fields.
function enqueue(view,action,params={}){
  clearAuto(view);
  const task=async()=>{
    if(!view.tab.isConnected)return null;
    const isNavigate=['navigate','back','forward','reload'].includes(action);
    if(action!=='navigate')ensureFresh(view);
    const maySubmit=action==='interact'&&['click','press'].includes(params.target_action);
    const updates=action==='interact'?pendingUpdates(view).filter(field=>
      maySubmit||!view.page?.elements?.find(e=>
        e.id===field.element_id&&(e.frame_index||0)===(field.frame_index||0))?.sensitive):[];
    showBusy(view,true);view.error.hidden=true;
    try{
      const {auto,...remoteParams}=params; // local-only marker: never forward to Go
      let payload={action,...remoteParams};
      if(view.sessionID){
        payload.session_id=view.sessionID;
        if(view.page?.revision)payload.revision=view.page.revision;
      }else if(action==='navigate')payload={action:'create',url:params.url};
      if(action==='interact')payload.field_updates=updates;
      const page=await app.jsonFetch('/api/semantic-browser',{
        method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)
      });
      if(!view.tab.isConnected)return null;
      if(page.session_id)view.sessionID=String(page.session_id);
      const previousWindow=view.page?.active_window_id;
      const previousURL=view.page?.url;
      if(action==='preview'&&typeof page.preview_image==='string')view.previewImage=page.preview_image;
      else if(action!=='snapshot'||previousWindow!==page.active_window_id||previousURL!==page.url)view.previewImage='';
      view.page=page;
      if(previousWindow&&page.active_window_id&&previousWindow!==page.active_window_id){
        // Changes are scoped to a real browser window. Never apply a draft
        // password or form update to a different OAuth/popup window.
        view.dirty.clear();
      }
      if(page.url)view.address.value=String(page.url);
      view.title.textContent='🌐 '+String(page.title||page.url||'Browser').slice(0,32);
      if(action==='interact')for(const field of updates){
        // Don't discard a newer edit typed while the request was in flight.
        const now=view.dirty.get(String(field.frame_index||0)+'/'+field.element_id);
        if(now?.value===field.value&&now?.kind===field.kind)view.dirty.delete(String(field.frame_index||0)+'/'+field.element_id);
      }
      if(isNavigate)view.dirty.clear();
      persist();
      render(view);
      return page;
    }catch(error){
      showError(view,error);
      return null;
    }finally{showBusy(view,false);}
  };
  const promise=view.queue.then(task,task);
  view.queue=promise.catch(()=>null);
  return promise;
}
function focusBrowserWindow(view,id){
  if(view.page?.active_window_id===id)return;
  if(view.dirty.size&&!window.confirm('Unsaved inputs in this window will be discarded. Switch windows?'))return;
  enqueue(view,'switch_window',{window_id:id});
}
function renderBrowserWindows(view,page){
  view.windows.replaceChildren();
  const windows=Array.isArray(page.windows)?page.windows:[];
  if(windows.length<=1){view.windows.hidden=true;return;}
  view.windows.hidden=false;
  for(const win of windows){
    if(!win?.id||!/^main$|^popup-[0-9]+$/.test(win.id))continue;
    const item=button((win.kind==='main'?'Main':'Popup')+' · '+String(win.title||'Website').slice(0,34),
      ()=>focusBrowserWindow(view,win.id),'semantic-browser-window');
    item.classList.toggle('active',Boolean(win.active));
    item.title=String(win.url||'').slice(0,280);
    view.windows.append(item);
    if(win.kind==='popup'){
      const close=button('×',()=>{
        if(view.dirty.size&&!window.confirm('Close popup and discard unsaved input?'))return;
        enqueue(view,'close_window',{window_id:win.id});
      },'semantic-browser-window-close');
      close.title='Close this popup window';view.windows.append(close);
    }
  }
}
function renderHTTPAuthentication(view,page){
  const challenge=page.auth_challenge;
  if(!challenge?.origin)return false;
  const panel=el('div','semantic-browser-dialog');
  panel.setAttribute('role','group');
  panel.setAttribute('aria-label','HTTP authentication challenge');
  panel.append(el('strong','','Website HTTP authentication'));
  panel.append(el('div','semantic-browser-dialog-text',
    'Sign in to '+String(challenge.origin).slice(0,180)+' (HTTP Basic). Credentials will be sent only to this website origin.'));
  if(challenge.window_id&&page.active_window_id!==challenge.window_id){
    panel.append(el('div','semantic-browser-muted','This challenge belongs to another browser window.'));
    const buttons=el('div','semantic-browser-dialog-actions');
    buttons.append(button('Open challenged window',()=>focusBrowserWindow(view,challenge.window_id)));
    buttons.append(button('Dismiss',()=>enqueue(view,'http_auth_cancel')));
    panel.append(buttons);view.content.append(panel);return true;
  }
  const user=el('input');user.type='text';user.autocomplete='username';
  user.placeholder='Username';user.setAttribute('aria-label','HTTP username');
  const pass=el('input');pass.type='password';pass.autocomplete='off';
  pass.placeholder='Password';pass.setAttribute('aria-label','HTTP password');
  panel.append(user,pass);
  const actions=el('div','semantic-browser-dialog-actions');
  const login=button('Sign in',()=>{
    const username=user.value,password=pass.value;
    // Do not cache the password in Tab state, localStorage or snapshot.
    pass.value='';
    enqueue(view,'http_auth',{http_username:username,http_password:password});
  });
  pass.onkeydown=event=>{if(event.key==='Enter'){event.preventDefault();login.click();}};
  const cancel=button('Cancel',()=>enqueue(view,'http_auth_cancel'));
  actions.append(login,cancel);panel.append(actions);
  view.content.append(panel);return true;
}
function renderBrowserDialog(view,page){
  const dialog=page.dialog;
  if(!dialog?.id)return false;
  const panel=el('div','semantic-browser-dialog');
  panel.setAttribute('role','alertdialog');
  panel.setAttribute('aria-modal','true');
  panel.append(el('strong','','Website '+String(dialog.type||'dialog')));
  panel.append(el('div','semantic-browser-dialog-text',String(dialog.message||'').slice(0,700)));
  let answer=null;
  if(dialog.type==='prompt'){
    answer=el('input');answer.type='text';answer.autocomplete='off';
    answer.setAttribute('aria-label','Website prompt answer');panel.append(answer);
  }
  const actions=el('div','semantic-browser-dialog-actions');
  const cancel=button(dialog.type==='alert'?'Dismiss':'Cancel',
    ()=>enqueue(view,'dialog_respond',{dialog_id:dialog.id,accept:false}));
  const ok=button('OK',()=>enqueue(view,'dialog_respond',{
    dialog_id:dialog.id,accept:true,prompt_text:answer?.value||''
  }));
  actions.append(ok,cancel);panel.append(actions);
  view.content.append(panel);
  ok.focus({preventScroll:true});
  return true;
}
function restoreFocus(view){
  const focused=document.activeElement;
  if(!focused||!view.content.contains(focused))return null;
  const id=focused.dataset.semanticId;
  if(!id)return null;
  let start=null,end=null;
  try{start=focused.selectionStart;end=focused.selectionEnd;}catch{}
  return {id,start,end};
}
function render(view){
  if(view.composing){view.renderDeferred=true;return;}
  const state=restoreFocus(view);
  const scroll=view.content.scrollTop;
  view.content.replaceChildren();
  const page=view.page;
  if(!page){
    view.content.append(el('p','semantic-browser-muted','Enter an HTTPS URL. All page scripts run in server-side Chromium.'));
    return;
  }
  renderBrowserWindows(view,page);
  if(page.window_event?.type==='closed')view.content.append(el('div','semantic-browser-popup-status','Popup closed. Returned to the website window.'));
  if(page.window_event?.type==='dialog_timeout')view.content.append(el('div','semantic-browser-popup-status','Website dialog timed out and was dismissed.'));
  view.content.append(el('div','semantic-browser-heading',String(page.title||page.url||'Website')));
  view.content.append(el('p','semantic-browser-muted',String(page.url||'')));
  if(/accounts\.google\.com|login\.microsoftonline\.com/i.test(page.url||'')){
    view.content.append(el('div','semantic-browser-auth-help',
      'This identity provider may block controlled browsers. Do not bypass provider security. An approved external-browser login requires support from the website; external cookies cannot be imported into this session.'));
  }
  if(renderBrowserDialog(view,page))return;
  if(renderHTTPAuthentication(view,page))return;
  if(view.previewImage&&typeof view.previewImage==='string'&&
      /^data:image\/jpeg;base64,[A-Za-z0-9+/=]+$/.test(view.previewImage)&&view.previewImage.length<=550000){
    const preview=el('img','semantic-browser-preview');
    preview.alt='Static screenshot of the remote website, for QR codes and controls not represented as text';
    preview.src=view.previewImage;
    view.content.append(el('div','semantic-browser-preview-note',
      'Static preview only: clicks must use the text/control list. No website JavaScript runs in this image.'));
    view.content.append(preview);
  }
  const entries=Array.isArray(page.elements)?page.elements:[];
  for(const item of entries){
    if(!item?.id||!/^td-sem-[0-9a-z]+$/.test(item.id))continue;
    const row=el('div','semantic-browser-entry');
    const name=String(item.text||'').slice(0,400);
    const key=controlKey(item);
    if(item.kind==='action'){
      const link=item.tag==='a'||item.role==='link';
      const control=button(name||'Interactive element',()=>{
        enqueue(view,'interact',{target_action:'click',element_id:item.id,frame_index:item.frame_index||0});
      },link?'semantic-browser-link':'semantic-browser-action');
      control.disabled=Boolean(item.disabled);
      row.append(control);
    }else if(item.kind==='input'){
      const wrap=el('div','semantic-browser-field');
      const label=el('label','',name||item.placeholder||'Input');
      // Native password inputs mask reliably in Firefox; textareas do not.
      const multiline=!item.sensitive&&Boolean(item.multiline||item.tag==='textarea');
      const input=el(multiline?'textarea':'input');
      if(!multiline)input.type=item.input_type==='password'||item.sensitive?'password':
        ['number','email','search','tel','url'].includes(item.input_type)?item.input_type:'text';
      input.dataset.semanticId=key;
      const dirty=view.dirty.get(key);
      input.value=dirty?.value??String(item.value||'');
      input.placeholder=String(item.placeholder||'');
      input.disabled=Boolean(item.disabled);
      input.setAttribute('aria-label',name||'Text field');
      input.addEventListener('compositionstart',()=>{view.composing=true;clearAuto(view);});
      input.addEventListener('compositionend',()=>{
        view.composing=false;rememberInput(view,item,input.value);
        if(view.renderDeferred){view.renderDeferred=false;render(view);}
      });
      input.addEventListener('input',()=>{rememberInput(view,item,input.value);});
      input.addEventListener('keydown',event=>{
        if(event.isComposing||view.composing)return;
        if(event.key==='Enter'&&(!multiline||event.ctrlKey||event.metaKey)){
          event.preventDefault();
          // Single-line Enter or Ctrl/Cmd+Enter in editors: flush all fields
          // before the authentic target receives the browser key event.
          enqueue(view,'interact',{target_action:'press',element_id:item.id,frame_index:item.frame_index||0,key:'Enter'});
        }
      });
      wrap.append(label,input);row.append(wrap);
    }else if(item.kind==='check'){
      const wrap=el('label','semantic-browser-entry');
      const input=el('input');input.type=item.input_type==='radio'?'radio':'checkbox';
      input.checked=Boolean(view.dirty.get(key)?.checked??item.checked);
      input.disabled=Boolean(item.disabled);
      input.onchange=()=>{
        view.dirty.set(key,{element_id:item.id,frame_index:item.frame_index||0,kind:'check',value:'',checked:input.checked});
        enqueue(view,'interact',{target_action:''});
      };
      wrap.append(input,document.createTextNode(' '+name));row.append(wrap);
    }else if(item.kind==='select'){
      const wrap=el('label','semantic-browser-field');
      wrap.append(el('span','',name||'Select'));
      const select=el('select');
      for(const option of item.options||[]){
        const entry=el('option','',String(option.text||''));entry.value=String(option.value||'');select.append(entry);
      }
      select.value=String(view.dirty.get(key)?.value??item.value??'');
      select.disabled=Boolean(item.disabled);
      select.onchange=()=>{
        view.dirty.set(key,{element_id:item.id,frame_index:item.frame_index||0,kind:'select',value:select.value});
        enqueue(view,'interact',{target_action:''});
      };
      wrap.append(select);row.append(wrap);
    }else {
      row.textContent=name;
      if(/^h[1-6]$/.test(item.tag||''))row.classList.add('semantic-browser-heading');
    }
    view.content.append(row);
  }
  if(page.truncated)view.content.append(el('p','semantic-browser-muted','Page element limit reached; complex visual content may be omitted.'));
  if(!entries.length)view.content.append(el('p','semantic-browser-muted','No textual controls detected yet. Try Refresh.'));
  view.content.scrollTop=scroll;
  if(state){
    const replacement=[...view.content.querySelectorAll('[data-semantic-id]')].find(node=>node.dataset.semanticId===state.id);
    if(replacement){
      replacement.focus({preventScroll:true});
      if(state.start!==null){try{replacement.setSelectionRange(state.start,state.end);}catch{}}
    }
  }
}
async function closeView(view){
  clearAuto(view);views.delete(view.id);view.tab.remove();view.pane.remove();persist();
  if(view.sessionID){
    try{await app.jsonFetch('/api/semantic-browser',{
      method:'DELETE',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_id:view.sessionID})
    });}catch(error){console.warn('Semantic Browser session close failed',errorOf(error));}
  }
  document.querySelector('#tabs > button:not(.hidden)')?.click();
}
function createView({id='',sessionID=''}={}){
  id=id||'semantic-browser:'+(globalThis.crypto?.randomUUID?.()||Date.now()+'-'+Math.random().toString(36).slice(2));
  const tab=el('button','tab semantic-browser-tab');tab.type='button';tab.dataset.id=id;
  const title=el('span','','🌐 Browser'),close=el('span','close','×');
  tab.append(title,close);tabsHost.append(tab);
  const pane=el('div','pane semantic-browser-pane hidden');pane.dataset.id=id;
  const toolbar=el('div','semantic-browser-toolbar');
  const back=button('←',()=>enqueue(view,'back'));
  const forward=button('→',()=>enqueue(view,'forward'));
  const reload=button('↻',()=>enqueue(view,'reload'));
  const preview=button('Image',()=>{
    if(view.previewImage){view.previewImage='';render(view);}
    else enqueue(view,'preview');
  });
  preview.title='Show one static screenshot for visual QR/2FA or complex widgets';
  const address=el('input','semantic-browser-address');address.type='url';address.placeholder='https://example.com';
  if(!sessionID)address.value=browserPreferences().home;
  const goto=button('Go',()=>enqueue(view,'navigate',{url:address.value.trim()}));
  address.onkeydown=event=>{if(event.key==='Enter'){event.preventDefault();goto.click();}};
  toolbar.append(back,forward,reload,preview,address,goto);
  const status=el('div','semantic-browser-muted','Enter a URL');status.style.padding='5px 12px';
  const error=el('div','semantic-browser-error');error.hidden=true;error.style.padding='6px 12px';
  const windows=el('div','semantic-browser-windows');windows.hidden=true;
  const content=el('div','semantic-browser-content');
  pane.append(toolbar,windows,status,error,content);panesHost.append(pane);
  const view={id,sessionID,page:null,tab,pane,title,back,forward,reload,preview,address,goto,windows,status,error,content,
    queue:Promise.resolve(),loading:false,dirty:new Map(),previewImage:'',autoTimer:0,composing:false,renderDeferred:false};
  views.set(id,view);
  tab.onclick=()=>setCurrent(view);
  close.onclick=event=>{event.stopPropagation();closeView(view).catch(err=>showError(view,err));};
  showBusy(view,false);render(view);setCurrent(view);persist();
  return view;
}
async function restore(){
  let data=[];
  try{const saved=JSON.parse(sessionStorage.getItem(storageKey())||'[]');if(Array.isArray(saved))data=saved.slice(0,4);}catch{}
  for(const item of data){
    if(!item?.id||!item?.session_id)continue;
    const view=createView({id:String(item.id),sessionID:String(item.session_id)});
    await enqueue(view,'snapshot');
    if(!view.page){views.delete(view.id);view.tab.remove();view.pane.remove();}
  }
  persist();
}
async function initialize(){
  try{await refreshAvailability();}catch{browserEnabled=false;}
  if(browserEnabled)await restore();
}
setInterval(()=>{
  if(!browserPreferences().autoRefresh)return;
  for(const view of views.values()){
    if(!view.sessionID||view.loading||!view.tab.classList.contains('active')||view.dirty.size||view.composing||view.page?.dialog||view.page?.auth_challenge)continue;
    const focus=document.activeElement;
    if(focus&&view.content.contains(focus)&&['INPUT','TEXTAREA','SELECT'].includes(focus.tagName))continue;
    enqueue(view,'snapshot');
  }
},4000);
const ready=new Promise(resolve=>{
  if(app.taskData?.workspace)initialize().finally(resolve);
  else window.addEventListener('taskmenu:tasks',()=>initialize().finally(resolve),{once:true});
});
globalThis.TaskMenuSemanticBrowser={views,open:openBrowser,openSetupGuide,openSettings,checkAvailability,refreshAvailability,ready};
