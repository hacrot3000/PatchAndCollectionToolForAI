'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');

const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const match=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(match,'trusted GUI JavaScript must be compiled into native client');

class MockElement{
  constructor(id){
    this.id=id;
    this.textContent='';
    this.hidden=false;
    this.disabled=false;
    this.listeners={};
    this.attributes={};
    this.classList={toggle(){}};
    this.children=[];
  }
  addEventListener(event,cb){this.listeners[event]=cb;}
  setAttribute(key,value){this.attributes[key]=value;}
  getAttribute(key){return this.attributes[key]||null;}
  click(){const cb=this.listeners.click;if(!cb)throw Error('click missing '+this.id);cb();}
  replaceChildren(){this.children=[];}
  append(...nodes){this.children.push(...nodes);}
}
const nodes=new Map();
function get(id){
  if(!nodes.has(id))nodes.set(id,new MockElement(id));
  return nodes.get(id);
}
const token='0123456789abcdef'.repeat(4);
let resolvePing;
let pingStarted=false,closeStarted=false,sessionStarted=false;
let featureMode='workspace';
let gitStatusResponse='## develop\n M docs/<script>bad.js\n?? new.txt\n';
let taskReportedTotal=10;
const originalTaskLabels=['Build','<img onerror=alert(1)>','Deploy','QA','Lint','Package','Smoke','Docs'];
let taskFirstPageLabels=originalTaskLabels.slice();
let taskNextPageLabels=['Integration','Release'];
let emptyNextTaskPage=false;
let fetchCount=0;
let mockRemaining=16;
let localSessionClosed=false;
let delayLocalSession=false;
let resolveLocalSession;
const healthChecks=[];
const fetch=async(path,options)=>{
  fetchCount++;
  assert.equal(options.method,'POST');
  assert.equal(options.mode,'same-origin');
  assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
  if(path==='/api/session'){
    sessionStarted=true;
    if(delayLocalSession){
      return await new Promise(resolve=>{resolveLocalSession=resolve;});
    }
    if(localSessionClosed)return {ok:false,status:410};
    return {ok:true,json:async()=>({
      ok:true,closed:false,active:false,remaining:16,expires_in_seconds:300,
      device_id:'a'.repeat(64),username:'alice',
    })};
  }
  if(path==='/api/ping'){
    pingStarted=true;
    mockRemaining--;
    return await new Promise(resolve=>{resolvePing=resolve;});
  }
  if(path==='/api/features'){
    return {ok:true,json:async()=>featureMode==='git-only'?{
      ok:true,session_ping:false,project_info:false,tasks_preview:false,
      git_status:true,remote_control_available:false,remaining:--mockRemaining,
    }:{
      ok:true,session_ping:true,project_info:true,tasks_preview:true,
      git_status:false,remote_control_available:false,remaining:--mockRemaining,
    }};
  }
  if(path==='/api/git-status'){
    return {ok:true,json:async()=>({
      ok:true,status:gitStatusResponse,remaining:--mockRemaining,
    })};
  }
  if(path==='/api/tasks-preview'){
    return {ok:true,json:async()=>({
      ok:true,total:taskReportedTotal,
      labels:taskReportedTotal===0?[]:taskFirstPageLabels.slice(),
      remaining:--mockRemaining,
    })};
  }
  if(path==='/api/tasks-preview-next'){
    return {ok:true,json:async()=>({
      ok:true,total:taskReportedTotal,labels:emptyNextTaskPage?[]:taskNextPageLabels.slice(),remaining:--mockRemaining,
    })};
  }
  if(path==='/api/project-info'){
    return {ok:true,json:async()=>({
      ok:true,project_name:'<img src=x onerror=alert(1)>',
      workspace_name:'remote-repo',host_os:'linux',remaining:--mockRemaining,
    })};
  }
  if(path==='/api/close'){
    closeStarted=true;
    return {ok:true,json:async()=>({ok:true,closed:true})};
  }
  throw Error('unexpected native UI endpoint '+path);
};
const history={replaceState(_state,_title,path){assert.equal(path,'/');}};
const location={hash:'#'+token,pathname:'/'};
const pageEvents=new Map();
const sandbox={
  document:{getElementById:get,createElement:(tag)=>new MockElement(tag)},
  window:{addEventListener(event,cb){pageEvents.set(event,cb);}},
  history,location,fetch,setInterval(cb,ms){if(ms===15000)healthChecks.push(cb);},Date,Number,String,Promise,
};
vm.createContext(sandbox);
new vm.Script(match[1],{filename:'trusted-remote-gui.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<15;i++)await Promise.resolve();};
(async()=>{
  await flush();
  assert.ok(sessionStarted,'GUI failed to load local-only session metadata');
  assert.equal(get('target-device').textContent,'a'.repeat(64));
  assert.equal(get('target-user').textContent,'alice');
  assert.equal(get('ping').disabled,false);
  get('ping').click();
  await flush();
  assert.ok(pingStarted,'E2EE ping not started');
  assert.equal(get('ping').disabled,true,'parallel read must be disabled');
  assert.equal(get('disconnect').disabled,false,'user must be able to disconnect while E2EE request is pending');
  assert.equal(get('quick-run').disabled,true,'quick actions must not race with an in-flight E2EE request');
  get('disconnect').click();
  await flush();
  assert.ok(closeStarted,'pending E2EE blocked logout request');
  const closedCalls=fetchCount;
  get('local-token').value=token;
  get('activate').click();await flush();
  assert.equal(fetchCount,closedCalls,'closed native GUI re-activated a bearer capability');
  assert.equal(get('local-token').value,'','rearm input retained a bearer capability');
  assert.equal(get('activate').disabled,true,'activation button became usable after logout');
  assert.equal(get('output').textContent,'Đã ngắt phiên.');
  // A slow/malicious Relay reply must never replace the local logout state.
  resolvePing({ok:true,json:async()=>({
    ok:true,message:'stale success must not show',remaining:2,round_trip_ms:100,
  })});
  await flush();
  assert.equal(get('output').textContent,'Đã ngắt phiên.');
  assert.equal(get('ping').disabled,true);
  assert.equal(get('disconnect').disabled,true);
  assert.ok(pageEvents.has('pagehide'),'browser page close handler missing');
  const countBefore=closeStarted;
  pageEvents.get('pagehide')();
  await flush();
  assert.equal(closeStarted,countBefore,'pagehide attempted to reopen an already closed session');
  // Reopen a FRESH independently verified native GUI page. Closing the tab
  // before any other action must still send a best-effort local session close.
  nodes.clear();
  pageEvents.clear();
  closeStarted=false;
  sessionStarted=false;
  mockRemaining=16;
  location.hash=''; // OS browser opened a SAFE origin-only URL.
  new vm.Script(match[1],{filename:'trusted-remote-gui-new-session.js'}).runInContext(sandbox);
  await flush();
  assert.equal(sessionStarted,false,'native UI contacted session endpoint without local user capability');
  assert.equal(get('ping').disabled,true,'actions exposed before trusted CLI activation');
  assert.equal(get('activation').hidden,false,'native activation form missing');
  get('local-token').value=token;
  get('activate').click();
  await flush();
  assert.equal(get('local-token').value,'','activation secret remained visible in the form');
  assert.equal(get('activation').hidden,true);
  assert.ok(sessionStarted,'fresh GUI did not authenticate locally entered capability');
  const afterFirstActivation=fetchCount;
  get('local-token').value=token;
  get('activate').click();await flush();
  assert.equal(fetchCount,afterFirstActivation,'second activation was allowed on same page');
  assert.equal(get('local-token').value,'','second activation leaked local secret');
  // Local tab navigation must never spend Relay E2EE app rounds.
  const callsBeforeNavigation=fetchCount;
  get('nav-project').click();
  assert.equal(get('panel-overview').hidden,true);
  assert.equal(get('panel-project').hidden,false);
  assert.equal(get('nav-project').getAttribute('aria-current'),'page');
  assert.equal(get('view-title').textContent,'Dự án TaskDeck');
  get('nav-tasks').click();
  assert.equal(get('panel-project').hidden,true);
  assert.equal(get('panel-tasks').hidden,false);
  get('nav-git').click();
  assert.equal(get('panel-git').hidden,false);
  get('nav-overview').click();
  assert.equal(get('panel-overview').hidden,false);
  assert.equal(get('panel-git').hidden,true);
  assert.equal(fetchCount,callsBeforeNavigation,'navigating GUI sent a request to PHP/Relay');
  assert.equal(get('project').disabled,true,'project view available before host authorizes flags');
  assert.equal(get('tasks-preview').disabled,true,'tasks available before host authorizes flags');
  get('features').click();
  await flush();
  assert.equal(get('project').disabled,false,'opted-in project view was not enabled');
  assert.equal(get('tasks-preview').disabled,false,'opted-in task preview was not enabled');
  assert.equal(get('git').disabled,true,'host-disabled Git status must not be accessible');
  assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 15');
  assert.match(get('feature-summary').textContent,/Git: Chưa bật hoặc thiếu quyền/);
  get('project').click();
  await flush();
  assert.equal(get('project-name').textContent,'<img src=x onerror=alert(1)>',
    'project name was not safely rendered as plain text');
  assert.equal(get('workspace-name').textContent,'remote-repo');
  assert.equal(get('host-os').textContent,'linux');
  assert.equal(get('project-overview').hidden,false);
  get('nav-tasks').click();
  get('nav-project').click();
  assert.equal(get('project-name').textContent,'<img src=x onerror=alert(1)>',
    'locally verified project data was lost on UI navigation');
  assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 14');
  get('tasks-preview').click();
  await flush();
  assert.equal(get('task-total').textContent,'8 / 10 task');
  assert.equal(get('task-coverage').textContent,'Đã tải 8 / 10 nhãn task');
  assert.match(get('task-next-help').textContent,/1 trong 13 lượt E2EE/);
  assert.equal(get('task-progress').value,8);
  assert.equal(get('task-progress').max,10);
  assert.equal(get('task-preview-labels').children.length,4);
  assert.equal(get('task-preview-labels').children[1].textContent,'<img onerror=alert(1)>');
  assert.equal(get('task-paging').hidden,false);
  assert.equal(get('task-page').textContent,'Trang 1 / 2');
  const beforePageChange=fetchCount;
  get('task-next').click();
  assert.equal(get('task-page').textContent,'Trang 2 / 2');
  assert.equal(get('task-preview-labels').children.length,4);
  assert.equal(get('task-preview-labels').children[0].textContent,'Lint');
  assert.equal(get('task-next').disabled,true);
  get('task-prev').click();
  assert.equal(get('task-page').textContent,'Trang 1 / 2');
  assert.equal(get('task-preview-labels').children[1].textContent,'<img onerror=alert(1)>');
  const beforeSort=fetchCount;
  get('task-sort').value='az';
  get('task-sort').listeners.change();
  assert.equal(get('task-preview-labels').children[0].textContent,'<img onerror=alert(1)>');
  get('task-sort').value='za';
  get('task-sort').listeners.change();
  assert.equal(get('task-preview-labels').children[0].textContent,'Smoke');
  get('task-sort').value='host';
  get('task-sort').listeners.change();
  assert.equal(get('task-preview-labels').children[0].textContent,'Build');
  assert.equal(fetchCount,beforeSort,'sorting local task labels contacted Portal or Relay');
  assert.equal(fetchCount,beforePageChange,'task page navigation spent a Relay operation');
  const beforeFilter=fetchCount;
  get('task-filter').value='lint';
  get('task-filter').listeners.input();
  assert.equal(get('task-preview-labels').children.length,1);
  assert.equal(get('task-preview-labels').children[0].textContent,'Lint');
  assert.equal(get('task-paging').hidden,true);
  get('task-filter').value='<IMG';
  get('task-filter').listeners.input();
  assert.equal(get('task-preview-labels').children.length,1);
  assert.equal(get('task-preview-labels').children[0].textContent,'<img onerror=alert(1)>');
  get('task-filter').value='___missing___';
  get('task-filter').listeners.input();
  assert.equal(get('task-preview-labels').children.length,0);
  assert.equal(get('task-filter-empty').hidden,false);
  assert.equal(fetchCount,beforeFilter,'client-side task search contacted Relay');
  get('task-filter').value='';
  get('task-filter').listeners.input();
  assert.equal(get('task-preview-labels').children.length,4);
  assert.equal(get('task-preview').hidden,false);
  const cachedCalls=fetchCount;
  get('nav-git').click();
  get('nav-tasks').click();
  assert.equal(get('task-preview-labels').children[1].textContent,'<img onerror=alert(1)>');
  assert.equal(fetchCount,cachedCalls,'tab navigation refreshed remote data without user action');
  assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 13');
  assert.equal(get('task-fetch-next').hidden,false,'GUI did not offer more tasks from verified host');
  assert.match(get('task-compare-summary').textContent,/Chưa tải đủ/);
  assert.equal(get('task-compare-list').children.length,0);
  const beforeRemoteNext=fetchCount;
  get('task-fetch-next').click();
  await flush();
  assert.equal(fetchCount,beforeRemoteNext+1,'host-owned next task page must cost exactly one E2EE round');
  assert.equal(get('task-total').textContent,'10 / 10 task');
  assert.equal(get('task-coverage').textContent,'Đã tải 10 / 10 nhãn task');
  assert.match(get('task-next-help').textContent,/Đã tải hết các nhãn task hợp lệ/);
  assert.equal(get('task-progress').value,10);
  assert.match(get('task-compare-summary').textContent,/Đã tải đủ 10 nhãn lần đầu/);
  assert.equal(get('task-compare-list').children.length,0);
  assert.equal(get('task-fetch-next').hidden,true,'GUI tried to fetch past final task');
  assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 12');
  get('task-next').click();
  get('task-next').click();
  assert.equal(get('task-preview-labels').children[0].textContent,'Integration');
  assert.equal(get('task-preview-labels').children[1].textContent,'Release');
  const allCalls=fetchCount;
  get('task-fetch-next').click();
  await flush();
  assert.equal(fetchCount,allCalls,'GUI made extra E2EE call after last page');
  // Compare only after every page of the second host-authenticated read.
  // Count duplicate labels as occurrences, not as unique set members.
  taskFirstPageLabels=['Build','Build','Deploy','QA','Lint','Package','Smoke','Docs'];
  const beforeTaskRefresh=fetchCount;
  get('tasks-preview').click();await flush();
  assert.equal(fetchCount,beforeTaskRefresh+1);
  assert.match(get('task-compare-summary').textContent,/Không suy đoán task bị xóa/);
  assert.equal(get('task-compare-list').children.length,0);
  const beforeLocalSort=fetchCount;
  get('task-sort').value='az';get('task-sort').listeners.change();
  assert.equal(fetchCount,beforeLocalSort,'local sort must not ask host for comparison');
  get('task-fetch-next').click();await flush();
  assert.equal(fetchCount,beforeTaskRefresh+2);
  assert.match(get('task-compare-summary').textContent,/1 nhãn mới, 1 nhãn không còn/);
  assert.equal(get('task-compare-list').children.length,2);
  assert.ok(get('task-compare-list').children.some(x=>x.textContent.includes('Mới xuất hiện: Build (1)')));
  assert.ok(get('task-compare-list').children.some(x=>x.textContent.includes('Không còn xuất hiện: <img onerror=alert(1)> (1)')));
  taskFirstPageLabels=['Build','Build','Deploy-NEW','QA','Lint','Package','Smoke','Docs'];
  get('tasks-preview').click();await flush();
  assert.match(get('task-compare-summary').textContent,/Không suy đoán task bị xóa/);
  get('task-fetch-next').click();await flush();
  assert.match(get('task-compare-summary').textContent,/1 nhãn mới, 1 nhãn không còn/);
  assert.ok(get('task-compare-list').children.some(x=>x.textContent.includes('Mới xuất hiện: Deploy-NEW')));
  assert.ok(get('task-compare-list').children.some(x=>x.textContent.includes('Không còn xuất hiện: Deploy')));
  assert.equal(get('task-total').textContent,'10 / 10 task');
  taskFirstPageLabels=originalTaskLabels.slice();
  pageEvents.get('pagehide')();
  await flush();
  assert.equal(closeStarted,true,'closing a fresh trusted GUI tab did not revoke local session');
  assert.equal(get('task-preview').hidden,true,'pagehide left task labels accessible in browser history cache');
  assert.equal(get('task-preview-labels').children.length,0,'pagehide leaked displayed task labels');
  assert.equal(get('task-coverage').textContent,'','pagehide retained task coverage');
  assert.equal(get('task-compare-summary').textContent,'','pagehide retained comparison');
  assert.equal(get('task-compare-list').children.length,0,'pagehide leaked old task labels');
  assert.equal(get('task-sort').disabled,true,'pagehide left task browsing enabled');
  assert.equal(get('target-device').textContent,'','pagehide retained verified agent identity');
  const afterPagehide=fetchCount;
  get('local-token').value=token;get('activate').click();await flush();
  assert.equal(fetchCount,afterPagehide,'pagehide or bfcache restored native GUI authority');
  assert.equal(get('local-token').value,'','pagehide activation attempt retained secret');
  // A Git-only TaskDeck user must retain Git status access without
  // accidentally receiving TaskDeck tasks.view/Ping authority.
  featureMode='git-only';
  mockRemaining=16;
  nodes.clear();pageEvents.clear();
  closeStarted=false;sessionStarted=false;location.hash='';
  new vm.Script(match[1],{filename:'trusted-remote-gui-git-only.js'}).runInContext(sandbox);
  await flush();
  get('local-token').value=token;get('activate').click();
  await flush();
  get('features').click();
  await flush();
  assert.equal(get('git').disabled,false,'Git-only user lost authorized read-only Git status');
  assert.equal(get('ping').disabled,true,'Git-only user bypassed missing tasks.view');
  assert.equal(get('project').disabled,true,'Git-only user accessed project overview');
  assert.equal(get('tasks-preview').disabled,true,'Git-only user accessed task labels');
  get('nav-git').click();
  get('git').click();
  await flush();
  assert.equal(get('git-summary').hidden,false,'authorized Git-only user did not see Git data');
  assert.equal(get('git-branch').textContent,'## develop');
  assert.equal(get('git-files').children.length,2);
  assert.equal(get('git-files').children[0].children[1].textContent,'docs/<script>bad.js');
  assert.equal(get('git-count').textContent,'Hiển thị 2 / 2 thay đổi từ ảnh chụp E2EE.');
  assert.match(get('git-delta-summary').textContent,/Ảnh chụp đầu tiên/);
  assert.equal(get('git-delta-list').children.length,0);
  assert.equal(get('git-kind').disabled,false);
  // Valid Git porcelain 'T' type changes (regular file to symlink etc.)
  // must not be mistaken for forged data and close the native session.
  gitStatusResponse='## develop\n T typechanged.file\n';
  get('git').click();await flush();
  assert.equal(get('git-files').children[0].children[0].textContent,' T');
  assert.notEqual(get('state').textContent,'Phiên đã đóng');
  gitStatusResponse='## develop\n M docs/<script>bad.js\n?? new.txt\n';
  get('git').click();await flush();
  const beforeGitFilter=fetchCount;
  get('git-filter').value='<SCRIPT>';
  get('git-filter').listeners.input();
  assert.equal(get('git-files').children.length,1);
  assert.equal(get('git-files').children[0].children[1].textContent,'docs/<script>bad.js');
  get('git-filter').value='';
  get('git-filter').listeners.input();
  get('git-kind').value='unstaged';
  get('git-kind').listeners.change();
  assert.equal(get('git-files').children.length,1);
  assert.equal(get('git-files').children[0].children[0].textContent,' M');
  get('git-kind').value='untracked';
  get('git-kind').listeners.change();
  assert.equal(get('git-files').children.length,1);
  assert.equal(get('git-files').children[0].children[1].textContent,'new.txt');
  get('git-kind').value='staged';
  get('git-kind').listeners.change();
  assert.equal(get('git-files').children.length,0);
  assert.equal(get('git-empty').hidden,false);
  get('git-kind').value='all';
  get('git-kind').listeners.change();
  assert.equal(get('git-files').children.length,2);
  assert.equal(fetchCount,beforeGitFilter,'local Git inspector sent an unreviewed host or Relay request');
  const callsBeforeGitTabs=fetchCount;
  get('nav-overview').click();
  get('nav-git').click();
  assert.equal(get('git-files').children.length,2,'local tab navigation erased Git snapshot');
  assert.equal(fetchCount,callsBeforeGitTabs,'Git view navigation unexpectedly reached Portal/Relay');
  // One additional explicit, authenticated E2EE read compares cached snapshots.
  const beforeSecondRead=fetchCount;
  gitStatusResponse='## develop\nM  docs/<script>bad.js\n M core/new.go\n';
  get('git').click();await flush();
  assert.equal(fetchCount,beforeSecondRead+1,'GUI comparison sent a hidden request');
  assert.equal(get('git-delta-list').children.length,3);
  assert.match(get('git-delta-summary').textContent,/1 mục mới, 1 đổi trạng thái, 1 không còn/);
  const delta=get('git-delta-list').children.map(item=>item.textContent);
  assert.ok(delta.some(s=>s.includes('Đổi trạng thái: docs/<script>bad.js')));
  assert.ok(delta.some(s=>s.includes('Mới xuất hiện: core/new.go')));
  assert.ok(delta.some(s=>s.includes('Không còn trong ảnh chụp: new.txt')));
  const beforeLocalDiff=fetchCount;
  get('git-filter').value='core/new.go';get('git-filter').listeners.input();
  get('nav-overview').click();get('nav-git').click();
  assert.equal(get('git-delta-list').children.length,3);
  assert.equal(fetchCount,beforeLocalDiff,'viewing cached Git delta contacted PHP/Relay');
  // Mismatched branch/tracking must discard comparison history.
  gitStatusResponse='## main\n M core/new.go\n';
  get('git').click();await flush();
  assert.match(get('git-delta-summary').textContent,/Nhánh hoặc tracking đã thay đổi/);
  assert.equal(get('git-delta-list').children.length,0);
  gitStatusResponse='## develop\n M docs/<script>bad.js\n?? new.txt\n';
  get('disconnect').click();
  await flush();
  assert.equal(closeStarted,true,'Git-only user could not revoke session');
  assert.equal(get('git-summary').hidden,true,'Git status remained visible after logout');
  assert.equal(get('git-files').children.length,0,'authenticated Git file paths leaked after logout');
  assert.equal(get('git-filter').value,'','search term survived logout');
  assert.equal(get('git-filter').disabled,true,'search remained active after logout');
  assert.equal(get('git-kind').disabled,true,'status filter remained active after logout');
  assert.equal(get('git-count').textContent,'','Git result count leaked after logout');
  assert.equal(get('git-delta-summary').textContent,'','Git comparison retained on logout');
  assert.equal(get('git-delta-list').children.length,0,'old Git path retained on logout');
  assert.equal(get('target-device').textContent,'','verified host identity was retained after logout');
  assert.equal(get('feature-summary').textContent,
    'Phiên đã đóng. Đăng nhập lại bằng client TaskDeck tin cậy.');
  pageEvents.get('pagehide')();
  await flush();
  // A syntactically valid but semantically invalid Git response must fail
  // closed; it must not masquerade as an empty/successful repository.
  nodes.clear();pageEvents.clear();location.hash='';sessionStarted=false;
  mockRemaining=16;gitStatusResponse='not Git porcelain output';
  new vm.Script(match[1],{filename:'trusted-gui-malformed-git.js'}).runInContext(sandbox);
  await flush();
  get('local-token').value=token;get('activate').click();await flush();
  get('features').click();await flush();
  get('git').click();await flush();
  assert.equal(get('state').textContent,'Phiên đã đóng');
  const beforeRejectedRearm=fetchCount;
  get('local-token').value=token;get('activate').click();await flush();
  assert.equal(fetchCount,beforeRejectedRearm,'malformed authenticated Git result re-armed session');
  assert.equal(get('git-summary').hidden,true);
  assert.equal(get('git-files').children.length,0);
  gitStatusResponse='## develop\n M docs/<script>bad.js\n?? new.txt\n';
  // A background check contacts only the native localhost listener.
  // It clears the UI after native session expiry, with no Relay calls.
  nodes.clear();pageEvents.clear();location.hash='';sessionStarted=false;
  mockRemaining=16;
  new vm.Script(match[1],{filename:'trusted-gui-idle-expiry.js'}).runInContext(sandbox);
  await flush();
  get('local-token').value=token;get('activate').click();
  await flush();
  assert.equal(get('target-device').textContent,'a'.repeat(64));
  assert.ok(healthChecks.length>0);
  const beforeHealth=fetchCount;
  localSessionClosed=true;
  healthChecks[healthChecks.length-1]();
  await flush();
  assert.equal(fetchCount,beforeHealth+1);
  assert.equal(get('target-device').textContent,'');
  assert.equal(get('state').textContent,'Phiên đã đóng');
  assert.equal(get('git').disabled,true);
  const afterHealth=fetchCount;
  healthChecks[healthChecks.length-1]();
  await flush();
  assert.equal(fetchCount,afterHealth);
  // The browser may have an older localhost status request outstanding
  // while the user uses a fresh encrypted action. The late status result
  // must not overwrite the E2EE action quota or feature permissions.
  nodes.clear();pageEvents.clear();location.hash='';sessionStarted=false;
  localSessionClosed=false;delayLocalSession=true;mockRemaining=16;
  new vm.Script(match[1],{filename:'trusted-gui-status-race.js'}).runInContext(sandbox);
  await flush();
  get('local-token').value=token;get('activate').click();
  await flush();
  assert.equal(typeof resolveLocalSession,'function','status request was not delayed');
  get('features').click();
  await flush();
  assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 15');
  const staleReply={ok:true,json:async()=>({
    ok:true,closed:false,active:false,remaining:16,expires_in_seconds:300,
    device_id:'a'.repeat(64),username:'alice'
  })};
  resolveLocalSession(staleReply);
  await flush();
  assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 15',
    'late native status incorrectly restored spent authenticated action quota');
  delayLocalSession=false;
  // Overlapping localhost metadata reads can resolve out of order.
  // The older success or failure must never override a newer observation.
  nodes.clear();pageEvents.clear();location.hash='';sessionStarted=false;
  delayLocalSession=true;mockRemaining=16;
  new vm.Script(match[1],{filename:'trusted-gui-status-poll-order.js'}).runInContext(sandbox);
  await flush();
  get('local-token').value=token;get('activate').click();await flush();
  const stalePoll=resolveLocalSession;
  healthChecks[healthChecks.length-1]();await flush();
  const latestPoll=resolveLocalSession;
  assert.notEqual(stalePoll,latestPoll);
  latestPoll({ok:true,json:async()=>({
    ok:true,closed:false,active:false,remaining:7,expires_in_seconds:120,
    device_id:'a'.repeat(64),username:'alice'
  })});
  await flush();
  assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 7');
  stalePoll({ok:true,json:async()=>({
    ok:true,closed:false,active:false,remaining:16,expires_in_seconds:300,
    device_id:'a'.repeat(64),username:'alice'
  })});
  await flush();
  assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 7',
    'old localhost poll restored previously consumed E2EE quota');
  healthChecks[healthChecks.length-1]();await flush();
  const staleDenied=resolveLocalSession;
  healthChecks[healthChecks.length-1]();await flush();
  const latestAllowed=resolveLocalSession;
  latestAllowed({ok:true,json:async()=>({
    ok:true,closed:false,active:false,remaining:5,expires_in_seconds:90,
    device_id:'a'.repeat(64),username:'alice'
  })});
  await flush();
  staleDenied({ok:false,status:410});
  await flush();
  assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 5');
  assert.notEqual(get('state').textContent,'Phiên đã đóng',
    'stale HTTP error closed a newer authenticated session');
  pageEvents.get('pagehide')();await flush();
  delayLocalSession=false;
  // After four E2EE rounds, partial task lists remain clearly marked as
  // partial and the GUI must never try another encrypted request.
  nodes.clear();pageEvents.clear();location.hash='';sessionStarted=false;
  taskReportedTotal=100;
  featureMode='workspace';
  mockRemaining=16;
  new vm.Script(match[1],{filename:'trusted-gui-exhausted-task-pages.js'}).runInContext(sandbox);
  await flush();
  get('local-token').value=token;get('activate').click();
  await flush();
  get('features').click();await flush();
  get('project').click();await flush();
  get('tasks-preview').click();await flush();
  get('task-fetch-next').click();await flush();
  assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 12');
  for(let n=0;n<12;n++){get('features').click();await flush();}
  assert.equal(get('remaining').textContent,'Lượt thao tác còn lại: 0');
  assert.equal(get('task-coverage').textContent,'Đã tải 10 / 100 nhãn task');
  assert.match(get('task-next-help').textContent,/Đã hết 16 lượt E2EE/);
  assert.equal(get('task-fetch-next').hidden,false);
  assert.equal(get('task-fetch-next').disabled,true);
  const beforeQuotaRetry=fetchCount;
  get('task-fetch-next').click();await flush();
  assert.equal(fetchCount,beforeQuotaRetry,'exhausted GUI attempted a 17th E2EE round');
  pageEvents.get('pagehide')();await flush();
  assert.equal(get('task-coverage').textContent,'','pagehide retained partial task list');
  assert.equal(get('task-preview-labels').children.length,0);
  // Empty workspace has a different UX from a filter with zero matches,
  // and must not activate a fetch-next action or invent a task count.
  nodes.clear();pageEvents.clear();location.hash='';sessionStarted=false;
  taskReportedTotal=0;mockRemaining=16;
  new vm.Script(match[1],{filename:'trusted-gui-empty-workspace.js'}).runInContext(sandbox);
  await flush();
  get('local-token').value=token;get('activate').click();await flush();
  get('features').click();await flush();
  get('tasks-preview').click();await flush();
  assert.equal(get('task-coverage').textContent,'Đã tải 0 / 0 nhãn task');
  assert.equal(get('task-filter-empty').hidden,false);
  assert.match(get('task-filter-empty').textContent,/chưa có task/);
  assert.equal(get('task-fetch-next').hidden,true);
  pageEvents.get('pagehide')();await flush();
  // A purported next page that contains zero labels while its announced
  // total still exceeds the accumulated count is invalid and revokes UI.
  nodes.clear();pageEvents.clear();location.hash='';sessionStarted=false;
  taskReportedTotal=100;emptyNextTaskPage=true;mockRemaining=16;
  new vm.Script(match[1],{filename:'trusted-gui-malformed-empty-page.js'}).runInContext(sandbox);
  await flush();
  get('local-token').value=token;get('activate').click();await flush();
  get('features').click();await flush();
  get('tasks-preview').click();await flush();
  get('task-fetch-next').click();await flush();
  assert.equal(get('state').textContent,'Phiên đã đóng');
  assert.equal(get('task-preview-labels').children.length,0);
  assert.equal(get('task-coverage').textContent,'');
  assert.equal(get('task-fetch-next').disabled,true);
  // The native quick-actions GUI is NOT a shell. Only exact, fixed
  // allowlisted verbs may call an already approved localhost endpoint.
  nodes.clear();pageEvents.clear();location.hash='';sessionStarted=false;
  taskReportedTotal=10;emptyNextTaskPage=false;mockRemaining=16;
  featureMode='workspace';
  new vm.Script(match[1],{filename:'trusted-gui-quick-actions.js'}).runInContext(sandbox);
  await flush();
  assert.equal(get('quick-run').disabled,true,'quick command enabled before local activation');
  get('local-token').value=token;get('activate').click();await flush();
  assert.equal(get('quick-run').disabled,false);
  const beforeDenied=fetchCount;
  for(const command of ['git push','terminal exec','tasks run',
    'git status --porcelain','/api/close','git status; echo leaked']){
    get('quick-command').value=command;
    get('quick-run').click();
  }
  await flush();
  assert.equal(fetchCount,beforeDenied,'untrusted command escaped local quick-action allowlist');
  assert.equal(get('quick-command').value,'','rejected input remained in the GUI');
  assert.match(get('quick-hint').textContent,/không được hỗ trợ/);
  get('quick-command').value='project';get('quick-run').click();await flush();
  assert.equal(fetchCount,beforeDenied,'unverified host feature was invoked by quick-action');
  get('quick-command').value='help';get('quick-run').click();await flush();
  assert.equal(fetchCount,beforeDenied,'local help spent an E2EE round');
  assert.match(get('output').textContent,/không có shell/);
  let enterWasHandled=false;
  get('quick-command').value='clear';
  get('quick-command').listeners.keydown({
    key:'Enter',preventDefault(){enterWasHandled=true;}
  });
  await flush();
  assert.equal(enterWasHandled,true,'keyboard Enter did not use local quick action');
  assert.equal(get('output').textContent,'');
  assert.equal(fetchCount,beforeDenied,'local clear touched Relay');
  get('quick-command').value='features';get('quick-run').click();await flush();
  assert.equal(fetchCount,beforeDenied+1,'fixed feature check was not delegated to native GUI');
  get('quick-command').value='project';get('quick-run').click();await flush();
  assert.equal(get('project-name').textContent,'<img src=x onerror=alert(1)>');
  assert.equal(get('panel-project').hidden,false,'quick project did not open its result view');
  assert.equal(get('quick-run').disabled,false,'quick palette disappeared after switching tab');
  get('quick-command').value='tasks next';get('quick-run').click();await flush();
  const beforeTask=fetchCount;
  assert.equal(beforeTask,beforeDenied+2,'task cursor was accepted without first page');
  get('quick-command').value='tasks';get('quick-run').click();await flush();
  assert.equal(fetchCount,beforeTask+1,'fixed task read was not dispatched');
  assert.equal(get('panel-tasks').hidden,false,'quick task did not open task panel');
  get('quick-command').value='tasks next';get('quick-run').click();await flush();
  assert.equal(get('task-total').textContent,'10 / 10 task');
  const beforeDeniedGit=fetchCount;
  get('quick-command').value='git status';get('quick-run').click();await flush();
  assert.equal(fetchCount,beforeDeniedGit,'quick-action ignored TaskDeck Git RBAC flag');
  get('quick-command').value='disconnect';get('quick-run').click();await flush();
  assert.equal(get('state').textContent,'Phiên đã đóng');
  assert.equal(get('quick-command').value,'');
  assert.equal(get('quick-command').disabled,true);
  assert.equal(get('quick-hint').textContent,'');
  const afterClose=fetchCount;
  get('quick-command').value='git status';get('quick-run').click();await flush();
  assert.equal(fetchCount,afterClose,'closed quick-action re-opened network authority');

  // Independently verify Git-only RBAC, without granting tasks.view.
  nodes.clear();pageEvents.clear();location.hash='';sessionStarted=false;
  mockRemaining=16;featureMode='git-only';
  new vm.Script(match[1],{filename:'trusted-gui-quick-actions-git-only.js'}).runInContext(sandbox);
  await flush();
  get('local-token').value=token;get('activate').click();await flush();
  get('quick-command').value='features';get('quick-run').click();await flush();
  const beforeGitOnly=fetchCount;
  get('quick-command').value='tasks';get('quick-run').click();await flush();
  assert.equal(fetchCount,beforeGitOnly,'Git-only user started TaskDeck task preview');
  get('quick-command').value='git status';get('quick-run').click();await flush();
  assert.equal(fetchCount,beforeGitOnly+1,'Git-only user lost permitted status read');
  assert.equal(get('git-summary').hidden,false);
  assert.equal(get('panel-git').hidden,false,'quick Git status did not open Git panel');
  assert.equal(get('quick-run').disabled,false,'global palette disappeared in Git panel');
  pageEvents.get('pagehide')();await flush();
  assert.equal(get('quick-command').value,'');
  assert.equal(get('quick-hint').textContent,'');
  console.log('PASS: GUI local allowlisted quick actions, immutable routing, RBAC and scrub');
})().catch(error=>{console.error(error);process.exitCode=1;});
