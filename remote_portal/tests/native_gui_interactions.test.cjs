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
      labels:taskReportedTotal===0?[]:['Build','<img onerror=alert(1)>','Deploy','QA','Lint','Package','Smoke','Docs'],
      remaining:--mockRemaining,
    })};
  }
  if(path==='/api/tasks-preview-next'){
    return {ok:true,json:async()=>({
      ok:true,total:taskReportedTotal,labels:emptyNextTaskPage?[]:['Integration','Release'],remaining:--mockRemaining,
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
  get('disconnect').click();
  await flush();
  assert.ok(closeStarted,'pending E2EE blocked logout request');
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
  const beforeRemoteNext=fetchCount;
  get('task-fetch-next').click();
  await flush();
  assert.equal(fetchCount,beforeRemoteNext+1,'host-owned next task page must cost exactly one E2EE round');
  assert.equal(get('task-total').textContent,'10 / 10 task');
  assert.equal(get('task-coverage').textContent,'Đã tải 10 / 10 nhãn task');
  assert.match(get('task-next-help').textContent,/Đã tải hết các nhãn task hợp lệ/);
  assert.equal(get('task-progress').value,10);
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
  pageEvents.get('pagehide')();
  await flush();
  assert.equal(closeStarted,true,'closing a fresh trusted GUI tab did not revoke local session');
  assert.equal(get('task-preview').hidden,true,'pagehide left task labels accessible in browser history cache');
  assert.equal(get('task-preview-labels').children.length,0,'pagehide leaked displayed task labels');
  assert.equal(get('task-coverage').textContent,'','pagehide retained task coverage');
  assert.equal(get('task-sort').disabled,true,'pagehide left task browsing enabled');
  assert.equal(get('target-device').textContent,'','pagehide retained verified agent identity');
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
  assert.equal(get('git-kind').disabled,false);
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
  get('disconnect').click();
  await flush();
  assert.equal(closeStarted,true,'Git-only user could not revoke session');
  assert.equal(get('git-summary').hidden,true,'Git status remained visible after logout');
  assert.equal(get('git-files').children.length,0,'authenticated Git file paths leaked after logout');
  assert.equal(get('git-filter').value,'','search term survived logout');
  assert.equal(get('git-filter').disabled,true,'search remained active after logout');
  assert.equal(get('git-kind').disabled,true,'status filter remained active after logout');
  assert.equal(get('git-count').textContent,'','Git result count leaked after logout');
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
  console.log('PASS: GUI sorting, coverage, empty workspace, malformed page, quota and pagehide wipe');
})().catch(error=>{console.error(error);process.exitCode=1;});
