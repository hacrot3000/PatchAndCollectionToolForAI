const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for self update');

const endpoint='/api/state/tasks?scope=self-update';
let currentID='';
let currentRequest={status:'idle'};
let applyingRedirect=false;
let startingFromSettings=false;
let settingsStartDeadline=0;
// Terminal failures already dismissed locally must not reappear from a stale poll.
const dismissedFailureIDs=new Set();

const style=document.createElement('style');
style.textContent=`
.self-update-overlay{position:fixed;inset:0;z-index:4000;display:none;align-items:center;justify-content:center;background:rgba(0,0,0,.58);padding:20px}
.self-update-overlay.visible{display:flex}
.self-update-overlay.nonblocking{inset:auto 16px 16px auto;display:block;background:transparent;padding:0;pointer-events:none}
.self-update-overlay.nonblocking .self-update-dialog{width:min(460px,calc(100vw - 32px));pointer-events:auto}
.self-update-dialog{width:min(560px,96vw);max-height:85vh;overflow-y:auto;background:#171a20;border:1px solid #48515f;border-radius:10px;box-shadow:0 18px 55px rgba(0,0,0,.5);padding:14px}
.self-update-dialog h3{margin:0 0 6px;font-size:14px}.self-update-dialog p{margin:5px 0;line-height:1.4;font-size:12px}.self-update-revision{font-family:ui-monospace,monospace;font-size:11px;opacity:.65;word-break:break-all}.self-update-status{margin-top:9px;padding:8px 9px;border-radius:6px;background:#0d1117;border:1px solid #30343b;font-size:12px;white-space:pre-wrap}.self-update-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:10px}.self-update-error{color:#ff8994}.self-update-running{border-color:#365f84}
.self-update-go-guide{margin-top:10px;padding:10px;border:1px solid #48515f;border-radius:7px;font-size:12px;line-height:1.45}
.self-update-go-guide strong{display:block;margin-bottom:5px}
.self-update-go-guide details{margin-top:7px}
.self-update-go-guide summary{cursor:pointer;font-weight:600}
.self-update-go-guide pre{margin:6px 0;padding:8px;border-radius:5px;background:#0d1117;white-space:pre-wrap;overflow-wrap:anywhere;user-select:text}
.self-update-go-guide a{color:#80bbff;text-decoration:underline}
html[data-taskmenu-theme="light"] .self-update-go-guide pre{background:#f5f6f8}
html[data-taskmenu-theme="light"] .self-update-go-guide a{color:#145baa}
html[data-taskmenu-theme="light"] .self-update-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .self-update-status{background:#f5f6f8;border-color:#c8ced6}
`;
document.head.append(style);

const overlay=document.createElement('div');overlay.className='self-update-overlay';
const dialog=document.createElement('div');dialog.className='self-update-dialog';
const title=document.createElement('h3');title.textContent='TaskDeck update';
const intro=document.createElement('p');
const revision=document.createElement('div');revision.className='self-update-revision';
const status=document.createElement('div');status.className='self-update-status';
const goGuide=document.createElement('section');goGuide.className='self-update-go-guide';goGuide.hidden=true;
const goHeading=document.createElement('strong');goHeading.textContent='Thiếu Go trên hệ điều hành của máy chủ TaskDeck';
const goExplanation=document.createElement('p');goExplanation.textContent='Cài Go dùng chung trên máy đang chạy TaskDeck (host), không cài riêng vào project. Sau khi cài, đảm bảo daemon có thể tìm thấy Go trong PATH rồi thử Update lại.';
const goLinux=document.createElement('details');
const goLinuxTitle=document.createElement('summary');goLinuxTitle.textContent='Linux / Ubuntu / Debian';
const goLinuxSteps=document.createElement('pre');goLinuxSteps.textContent='sudo apt update\nsudo apt install -y golang-go\n/usr/bin/go version\n# Kiểm tra đường dẫn được ưu tiên:\ntype -a go\ngo version\ngo env GOROOT';
goLinux.append(goLinuxTitle,goLinuxSteps);
const goMac=document.createElement('details');
const goMacTitle=document.createElement('summary');goMacTitle.textContent='macOS (Homebrew)';
const goMacSteps=document.createElement('pre');goMacSteps.textContent='brew install go\n"$(brew --prefix go)/bin/go" version\ncommand -v go\ngo version\ngo env GOROOT';
goMac.append(goMacTitle,goMacSteps);
const goNote=document.createElement('p');goNote.textContent='Nếu Go đã được cài nhưng TaskDeck vẫn không tìm thấy, kiểm tra PATH/GOROOT của tiến trình daemon. PATH của terminal có thể khác PATH của TaskDeck. Cài system-wide rồi khởi động lại daemon nếu cần.';
const goOfficial=document.createElement('a');goOfficial.href='https://go.dev/doc/install';goOfficial.target='_blank';goOfficial.rel='noopener noreferrer';goOfficial.textContent='Hướng dẫn cài Go chính thức';
goGuide.append(goHeading,goExplanation,goLinux,goMac,goNote,goOfficial);
const actions=document.createElement('div');actions.className='self-update-actions';
const copyError=document.createElement('button');copyError.textContent='Copy error details';copyError.style.display='none';copyError.title='Copy revision and full self-update error details';
const dismiss=document.createElement('button');dismiss.textContent='Close';dismiss.style.display='none';
actions.append(copyError,dismiss);dialog.append(title,intro,revision,status,goGuide,actions);overlay.append(dialog);document.body.append(overlay);

const checkUpdate=document.createElement('button');
checkUpdate.id='self-update-check';
checkUpdate.type='button';
checkUpdate.textContent='Check update';
checkUpdate.title='Check GitHub for a newer TaskDeck revision';
document.querySelector('header')?.append(checkUpdate);

function canCheckSelfUpdate(){
  return !app.sharedMode||Boolean(app.hasPermission?.('selfupdate.check')||app.hasPermission?.('project.admin'));
}
function canRunSelfUpdate(){
  return !app.sharedMode||Boolean(app.hasPermission?.('selfupdate.run')||app.hasPermission?.('project.admin'));
}
function syncSelfUpdatePermissionUI(){
  const canCheck=canCheckSelfUpdate(),canRun=canRunSelfUpdate();
  checkUpdate.hidden=!canCheck;
  checkUpdate.dataset.canRun=canRun?'1':'0';
  if(canCheck&&!canRun)checkUpdate.title='Check for TaskDeck updates (starting an update requires selfupdate.run)';
}
syncSelfUpdatePermissionUI();

function resetCheckUpdateButton(){
  if(!checkUpdate.isConnected)return;
  checkUpdate.disabled=false;
  checkUpdate.textContent='Check update';
}
function showCheckUpdateFeedback(label,delay=1600){
  checkUpdate.disabled=true;
  checkUpdate.textContent=label;
  setTimeout(()=>{if(!startingFromSettings)resetCheckUpdateButton();},delay);
}

function completedMarker(id){return 'vscode-tasks-menu:self-update-completed:'+id;}
function updatedQueryID(){return new URL(location.href).searchParams.get('_self_updated')||'';}
function clearUpdatedQuery(){
  const url=new URL(location.href);if(!url.searchParams.has('_self_updated'))return;
  url.searchParams.delete('_self_updated');history.replaceState(null,'',url);
}
const arrivingID=updatedQueryID();if(arrivingID){try{sessionStorage.setItem(completedMarker(arrivingID),'1');}catch{}clearUpdatedQuery();}

function terminalRestore(){return globalThis.TaskMenuTerminalRestore;}
function resumeTerminalPersistence(){terminalRestore()?.resumeAfterSelfUpdate?.();}
// Match only toolchain-discovery errors, not arbitrary Go build/test failures.
 // Older TaskDeck versions do not send a typed error_code.
function selfUpdateMissingGo(req){
  if(req?.status!=='failed')return false;
  const code=String(req.error_code||'').toUpperCase();
  if(code==='GO_TOOLCHAIN_MISSING')return true;
  const error=String(req.error||'');
  return /Go toolchain not found for self-update/i.test(error)||
    /exec:\s*["']go["']:\s*executable file not found in \$PATH/i.test(error)||
    /\bgo:\s*(?:command not found|executable file not found)/i.test(error);
}
function selfUpdateErrorDetails(req){
  const lines=['TaskDeck update'];
  if(req?.revision)lines.push('Revision: '+String(req.revision));
  lines.push('Candidate validation or activation failed. The current TaskDeck remains usable.');
  lines.push(statusText(req));
  if(selfUpdateMissingGo(req)){
    lines.push('Lỗi thiếu Go trên máy chủ TaskDeck. Cài Go dùng chung cho hệ điều hành, không phải trong một project.\n'+
      'Linux/Ubuntu/Debian: sudo apt update && sudo apt install -y golang-go\n'+
      'macOS/Homebrew: brew install go\n'+
      'Kiểm tra trên host: command -v go; go version; go env GOROOT\n'+
      'Đảm bảo PATH/GOROOT của daemon đúng (có thể cần restart daemon).\n'+
      'Hướng dẫn chính thức: https://go.dev/doc/install');
  }
  return lines.join('\n\n');
}
async function copyText(text){
  const value=String(text||'');
  if(!value)return false;
  if(navigator.clipboard&&window.isSecureContext){
    try{await navigator.clipboard.writeText(value);return true;}catch{}
  }
  const area=document.createElement('textarea');area.value=value;area.setAttribute('readonly','');area.style.position='fixed';area.style.left='-9999px';area.style.top='0';
  document.body.append(area);area.select();
  try{return document.execCommand('copy');}finally{area.remove();}
}
function statusText(req){
  if(req?.error)return 'Update failed: '+req.error;
  const labels={
    awaiting_confirmation:'Waiting for confirmation.',
    confirmed:'Update confirmed. Preparing the update…',
    downloading:'Dry-run: downloading candidate source…',
    testing:'Dry-run: running full validation tests…',
    building:'Dry-run: building and validating candidate binary…',
    installing:'Candidate passed dry-run. Activating validated release…',
    ready_restart:'New binary is ready. Preparing daemon handoff…',
    restarting:'Restarting the daemon…',
    completed:'Update completed. The new daemon is ready.',
    failed:'Update failed.',
    cancelled:'Update cancelled.'
  };
  return labels[req?.status]||String(req?.status||'');
}

function show(req,{reconnecting=false,starting=false}={}){
  currentRequest={...(req&&typeof req==='object'?req:{}),status:String(req?.status||'idle')};
  const failed=req?.status==='failed';
  const completed=req?.status==='completed';
  const missingGo=selfUpdateMissingGo(req);
  title.textContent=missingGo?'TaskDeck update · Thiếu Go':'TaskDeck update';
  goGuide.hidden=!missingGo;
  currentID=req?.id||currentID||'';
  overlay.classList.add('visible','nonblocking');
  dialog.classList.toggle('self-update-running',!failed);
  revision.textContent=req?.revision?'Revision: '+req.revision:'';
  status.classList.toggle('self-update-error',failed);
  status.textContent=reconnecting
    ?'Waiting for the TaskDeck daemon to reconnect…'
    :(starting?'Starting TaskDeck self-update…':statusText(req));
  copyError.style.display=failed?'':'none';
  dismiss.style.display=failed?'':'none';
  // The dialog is reused across update attempts. Never inherit a disabled Close
  // button from a previously acknowledged failure.
  if(failed)dismiss.disabled=false;
  copyError.dataset.details=failed?selfUpdateErrorDetails(req):'';
  actions.style.display=failed?'flex':'none';
  if(failed){
    intro.textContent=missingGo
      ?'Không tìm thấy Go để build bản cập nhật. TaskDeck hiện tại vẫn hoạt động; hãy cài Go dùng chung trên máy chủ theo hướng dẫn bên dưới.'
      :'Candidate validation or activation failed. The current TaskDeck remains usable; close this notice and continue working.';
  }else if(completed){
    intro.textContent='Update completed. Reloading TaskDeck…';
  }else if(reconnecting){
    intro.textContent='The updater is handing off to the new daemon. Open sessions can continue through the session broker.';
  }else{
    intro.textContent=String(req?.message||'Update is running in the background. You can keep using TaskDeck while validation is in progress.');
  }
}
function showStartingProgress(){
  show({status:'confirmed',message:'Starting the updater process…'},{starting:true});
}
function showReconnectProgress(){
  show({id:currentID,status:'restarting',message:'Waiting for the TaskDeck daemon to reconnect…'},{reconnecting:true});
}
function hide(){
  overlay.classList.remove('visible','nonblocking');dialog.classList.remove('self-update-running');
  currentID='';currentRequest={status:'idle'};copyError.dataset.details='';
  dismiss.disabled=false;
}

async function checkAndStartUpdate(){
  if(startingFromSettings)return;
  if(!canCheckSelfUpdate())throw new Error('Self-update check permission is required');
  checkUpdate.disabled=true;
  checkUpdate.textContent='Checking…';
  try{
    const result=await app.jsonFetch(endpoint+'&action=check');
    if(!result?.available){
      const short=String(result?.installed_revision||'').slice(0,12);
      showCheckUpdateFeedback(short?'Up to date · '+short:'Up to date');
      return;
    }
    if(!canRunSelfUpdate()){
      const revision=String(result?.revision||result?.remote_revision||'').slice(0,12);
      showCheckUpdateFeedback(revision?'Update available · '+revision:'Update available',2600);
      checkUpdate.title='Update available; selfupdate.run permission is required to install it';
      return;
    }

    globalThis.TaskMenuWorkspaceTabs?.saveNow?.();
    await globalThis.TaskMenuEditor?.persistSession?.();
    await terminalRestore()?.persistSnapshot?.();

    startingFromSettings=true;
    settingsStartDeadline=Date.now()+10000;
    checkUpdate.textContent='Updating…';
    checkUpdate.title='Starting TaskDeck self-update…';
    showStartingProgress();
    await app.jsonFetch(endpoint+'&action=start',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});
  }catch(e){
    startingFromSettings=false;
    settingsStartDeadline=0;
    resetCheckUpdateButton();
    hide();
    app.showError(e);
  }
}
checkUpdate.onclick=()=>checkAndStartUpdate();


async function postAction(action,id){
  return app.jsonFetch(endpoint+'&action='+encodeURIComponent(action),{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id})});
}

copyError.onclick=async()=>{
  const details=String(copyError.dataset.details||'');
  if(!details)return;
  const original=copyError.textContent;
  try{
    const copied=await copyText(details);
    if(!copied)throw new Error('Clipboard copy failed');
    copyError.textContent='✓ Copied';
    setTimeout(()=>{if(copyError.isConnected)copyError.textContent=original;},1200);
  }catch(e){
    app.showError(e);
  }
};
dismiss.onclick=()=>{
  const id=currentID;
  if(id&&currentRequest?.status==='failed'){
    dismissedFailureIDs.add(id);
    // Keep the set bounded across repeated updates in long-lived browser tabs.
    if(dismissedFailureIDs.size>32)dismissedFailureIDs.delete(dismissedFailureIDs.values().next().value);
  }
  // This is a nonblocking notice, not an update confirmation. Its Close button
  // must work immediately even if ACK fails or the daemon is reconnecting.
  startingFromSettings=false;settingsStartDeadline=0;
  resumeTerminalPersistence();resetCheckUpdateButton();hide();
  if(id)void postAction('ack',id).catch(error=>{
    console.warn('TaskDeck self-update notice ACK failed; suppressed locally',error);
  });
};

function isLoopbackHostname(hostname){
  const host=String(hostname||'').trim().toLowerCase().replace(/^\[|\]$/g,'');
  return host==='localhost'||host==='::1'||host.startsWith('127.');
}
function redirectTargetForUpdate(req){
  const here=new URL(location.href);
  const raw=String(req?.target_url||'').trim();
  if(!raw)return here;
  let target;
  try{target=new URL(raw,here);}catch{return here;}
  // target_url is derived from the daemon listener. A wildcard listener is
  // intentionally published as 127.0.0.1, which is only meaningful on the
  // server itself. Remote browsers must keep the origin they actually used.
  if(isLoopbackHostname(target.hostname)&&!isLoopbackHostname(here.hostname))return here;
  if(target.origin===here.origin)return here;
  return new URL(target.origin+'/');
}

function redirectAfterUpdate(req){
  if(applyingRedirect||!req?.id)return;
  try{
    if(sessionStorage.getItem(completedMarker(req.id))==='1'){
      postAction('ack',req.id).catch(()=>{});
      hide();
      return;
    }
  }catch{}
  applyingRedirect=true;
  try{sessionStorage.setItem(completedMarker(req.id),'1');}catch{}
  const url=redirectTargetForUpdate(req);
  url.searchParams.set('_self_updated',req.id);
  setTimeout(()=>location.replace(url.toString()),450);
}

async function poll(){
  if(!canCheckSelfUpdate())return;
  try{
    const req=await app.jsonFetch(endpoint);
    if(!req||req.status==='idle'||!req.id){
      if(startingFromSettings){
        if(Date.now()<settingsStartDeadline){
          checkUpdate.disabled=true;
          checkUpdate.textContent='Updating…';
          checkUpdate.title='Waiting for the updater process to start…';
          showStartingProgress();
          return;
        }
        startingFromSettings=false;
        settingsStartDeadline=0;
        resumeTerminalPersistence();
        resetCheckUpdateButton();
        hide();
        app.showError(new Error('Automatic self-update did not start.'));
        return;
      }
      hide();return;
    }
    const wasStartingFromSettings=startingFromSettings;
    if(startingFromSettings){
      startingFromSettings=false;
      settingsStartDeadline=0;
    }
    if(req.status==='cancelled'){startingFromSettings=false;settingsStartDeadline=0;resumeTerminalPersistence();resetCheckUpdateButton();hide();return;}
    if(req.status==='completed'){show(req);redirectAfterUpdate(req);return;}
    if(req.status==='failed'){
      startingFromSettings=false;settingsStartDeadline=0;
      resumeTerminalPersistence();resetCheckUpdateButton();
      if(dismissedFailureIDs.has(req.id)){hide();return;}
      show(req);return;
    }
    show(req);
    checkUpdate.disabled=true;
    checkUpdate.textContent='Updating…';
    checkUpdate.title=statusText(req)||'TaskDeck self-update is running';
    if(wasStartingFromSettings&&req.id)currentID=req.id;
  }catch(e){
    if(startingFromSettings||currentID){
      checkUpdate.disabled=true;
      checkUpdate.textContent='Updating…';
      checkUpdate.title='Waiting for the TaskDeck daemon to reconnect…';
      showReconnectProgress();
    }
  }
}

function selfUpdateOperationSnapshot(){
  const req=currentRequest&&typeof currentRequest==='object'?currentRequest:{status:'idle'};
  const status=String(req.status||'idle').toLowerCase();
  if(status==='idle'||status==='cancelled')return [];
  const mapped=status==='failed'?'failed':(status==='completed'?'completed':'running');
  return [{
    source:'self-update',id:String(req.id||currentID||'self-update'),title:'TaskDeck self-update',
    detail:String(req.message||statusText(req)||status),status:mapped,error:String(req.error||''),
    created_at:String(req.created_at||''),updated_at:String(req.updated_at||''),
    can_cancel:false,can_retry:status==='failed',can_open:true
  }];
}
async function selfUpdateOperationControl(_operation,action){
  if(action==='open'){if(currentRequest?.status&&currentRequest.status!=='idle'&&canCheckSelfUpdate())show(currentRequest);return true;}
  if(action==='retry'){
    if(!canRunSelfUpdate())throw new Error('Self-update run permission is required');
    return checkAndStartUpdate();
  }
  throw new Error('Unsupported self-update operation action');
}
globalThis.TaskMenuSelfUpdate={operationSnapshot:selfUpdateOperationSnapshot,operationControl:selfUpdateOperationControl,checkAndStartUpdate,canCheck:canCheckSelfUpdate,canRun:canRunSelfUpdate,syncPermissionUI:syncSelfUpdatePermissionUI,get current(){return {...currentRequest};}};

setInterval(poll,900);setTimeout(poll,150);
