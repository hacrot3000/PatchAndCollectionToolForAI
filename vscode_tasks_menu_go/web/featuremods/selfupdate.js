const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for self update');

const endpoint='/api/state/tasks?scope=self-update';
let currentID='';
let applyingRedirect=false;
let startingFromSettings=false;
let settingsStartDeadline=0;

const style=document.createElement('style');
style.textContent=`
.self-update-overlay{position:fixed;inset:0;z-index:4000;display:none;align-items:center;justify-content:center;background:rgba(0,0,0,.58);padding:20px}
.self-update-overlay.visible{display:flex}
.self-update-overlay.nonblocking{inset:auto 16px 16px auto;display:block;background:transparent;padding:0;pointer-events:none}
.self-update-overlay.nonblocking .self-update-dialog{width:min(560px,calc(100vw - 32px));pointer-events:auto}
.self-update-dialog{width:min(560px,96vw);background:#171a20;border:1px solid #48515f;border-radius:10px;box-shadow:0 18px 55px rgba(0,0,0,.5);padding:18px}
.self-update-dialog h3{margin:0 0 8px;font-size:16px}.self-update-dialog p{margin:7px 0;line-height:1.45}.self-update-revision{font-family:ui-monospace,monospace;font-size:12px;opacity:.75;word-break:break-all}.self-update-status{margin-top:12px;padding:9px 10px;border-radius:6px;background:#0d1117;border:1px solid #30343b;font-size:12px;white-space:pre-wrap}.self-update-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:14px}.self-update-error{color:#ff8994}
html[data-taskmenu-theme="light"] .self-update-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .self-update-status{background:#f5f6f8;border-color:#c8ced6}
`;
document.head.append(style);

const overlay=document.createElement('div');overlay.className='self-update-overlay';
const dialog=document.createElement('div');dialog.className='self-update-dialog';
const title=document.createElement('h3');title.textContent='TaskDeck update';
const intro=document.createElement('p');
const revision=document.createElement('div');revision.className='self-update-revision';
const status=document.createElement('div');status.className='self-update-status';
const actions=document.createElement('div');actions.className='self-update-actions';
const copyError=document.createElement('button');copyError.textContent='Copy error details';copyError.style.display='none';copyError.title='Copy revision and full self-update error details';
const dismiss=document.createElement('button');dismiss.textContent='Close';dismiss.style.display='none';
actions.append(copyError,dismiss);dialog.append(title,intro,revision,status,actions);overlay.append(dialog);document.body.append(overlay);

const checkUpdate=document.createElement('button');
checkUpdate.id='self-update-check';
checkUpdate.type='button';
checkUpdate.textContent='Check update';
checkUpdate.title='Check GitHub for a newer TaskDeck revision';
document.querySelector('header')?.append(checkUpdate);

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
function selfUpdateErrorDetails(req){
  const lines=['TaskDeck update'];
  if(req?.revision)lines.push('Revision: '+String(req.revision));
  lines.push('Candidate validation or activation failed. The current TaskDeck remains usable.');
  lines.push(statusText(req));
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
    testing:'Dry-run: running validation tests…',
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

function show(req){
  const failed=req?.status==='failed';
  if(!failed){hide();return;}
  currentID=req.id||'';overlay.classList.add('visible','nonblocking');
  revision.textContent=req.revision?'Revision: '+req.revision:'';
  status.classList.add('self-update-error');
  status.textContent=statusText(req);
  copyError.style.display='';
  dismiss.style.display='';
  copyError.dataset.details=selfUpdateErrorDetails(req);
  actions.style.display='flex';
  intro.textContent='Candidate validation or activation failed. The current TaskDeck remains usable; close this notice and continue working.';
}
function hide(){overlay.classList.remove('visible','nonblocking');currentID='';copyError.dataset.details='';}

async function checkAndStartUpdate(){
  if(startingFromSettings)return;
  checkUpdate.disabled=true;
  checkUpdate.textContent='Checking…';
  try{
    const result=await app.jsonFetch(endpoint+'&action=check');
    if(!result?.available){
      const short=String(result?.installed_revision||'').slice(0,12);
      showCheckUpdateFeedback(short?'Up to date · '+short:'Up to date');
      return;
    }

    await terminalRestore()?.persistSnapshot?.();

    startingFromSettings=true;
    settingsStartDeadline=Date.now()+10000;
    checkUpdate.textContent='Updating…';
    checkUpdate.title='Starting TaskDeck self-update…';
    hide();
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
dismiss.onclick=async()=>{
  const id=currentID;
  if(!id){hide();return;}
  dismiss.disabled=true;
  try{
    await postAction('ack',id);
    startingFromSettings=false;settingsStartDeadline=0;
    resumeTerminalPersistence();resetCheckUpdateButton();hide();
  }catch(e){
    app.showError(e);dismiss.disabled=false;
  }
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
  try{
    const req=await app.jsonFetch(endpoint);
    if(!req||req.status==='idle'||!req.id){
      if(startingFromSettings){
        if(Date.now()<settingsStartDeadline){
          checkUpdate.disabled=true;
          checkUpdate.textContent='Updating…';
          checkUpdate.title='Waiting for the updater process to start…';
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
    if(startingFromSettings){
      startingFromSettings=false;
      settingsStartDeadline=0;
    }
    if(req.status==='cancelled'){startingFromSettings=false;settingsStartDeadline=0;resumeTerminalPersistence();resetCheckUpdateButton();hide();return;}
    if(req.status==='completed'){hide();redirectAfterUpdate(req);return;}
    if(req.status==='failed'){startingFromSettings=false;settingsStartDeadline=0;resumeTerminalPersistence();resetCheckUpdateButton();show(req);return;}
    hide();
    checkUpdate.disabled=true;
    checkUpdate.textContent='Updating…';
    checkUpdate.title=statusText(req)||'TaskDeck self-update is running';
  }catch(e){
    if(startingFromSettings){
      checkUpdate.disabled=true;
      checkUpdate.textContent='Updating…';
      checkUpdate.title='Waiting for the TaskDeck daemon to reconnect…';
    }
  }
}

setInterval(poll,900);setTimeout(poll,150);
