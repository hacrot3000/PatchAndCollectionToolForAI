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
.self-update-dialog{width:min(560px,96vw);background:#171a20;border:1px solid #48515f;border-radius:10px;box-shadow:0 18px 55px rgba(0,0,0,.5);padding:18px}
.self-update-dialog h3{margin:0 0 8px;font-size:16px}.self-update-dialog p{margin:7px 0;line-height:1.45}.self-update-revision{font-family:ui-monospace,monospace;font-size:12px;opacity:.75;word-break:break-all}.self-update-status{margin-top:12px;padding:9px 10px;border-radius:6px;background:#0d1117;border:1px solid #30343b;font-size:12px;white-space:pre-wrap}.self-update-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:14px}.self-update-confirm{background:#24472f;border-color:#3b7850}.self-update-cancel{background:#3b2528;border-color:#684047}.self-update-error{color:#ff8994}
html[data-taskmenu-theme="light"] .self-update-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .self-update-status{background:#f5f6f8;border-color:#c8ced6}
`;
document.head.append(style);

const overlay=document.createElement('div');overlay.className='self-update-overlay';
const dialog=document.createElement('div');dialog.className='self-update-dialog';
const title=document.createElement('h3');title.textContent='VS Code Tasks Menu update';
const intro=document.createElement('p');
const revision=document.createElement('div');revision.className='self-update-revision';
const status=document.createElement('div');status.className='self-update-status';
const actions=document.createElement('div');actions.className='self-update-actions';
const cancel=document.createElement('button');cancel.className='self-update-cancel';cancel.textContent='Cancel';
const confirm=document.createElement('button');confirm.className='self-update-confirm';confirm.textContent='Update now';
actions.append(cancel,confirm);dialog.append(title,intro,revision,status,actions);overlay.append(dialog);document.body.append(overlay);

const checkUpdate=document.createElement('button');
checkUpdate.id='self-update-check';
checkUpdate.type='button';
checkUpdate.textContent='Check update';
checkUpdate.title='Check GitHub for a newer vscode_tasks_menu revision';
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
function statusText(req){
  if(req?.error)return 'Update failed: '+req.error;
  const labels={
    awaiting_confirmation:'Waiting for confirmation.',
    confirmed:'Update confirmed. Preparing the update…',
    downloading:'Downloading the latest source…',
    testing:'Running validation tests…',
    building:'Building the new binary…',
    installing:'Installing the validated binary…',
    ready_restart:'New binary is ready. Preparing daemon handoff…',
    restarting:'Restarting the daemon…',
    completed:'Update completed. The new daemon is ready.',
    failed:'Update failed.',
    cancelled:'Update cancelled.'
  };
  return labels[req?.status]||String(req?.status||'');
}

function show(req){
  currentID=req.id||'';overlay.classList.add('visible');
  revision.textContent=req.revision?'Revision: '+req.revision:'';
  status.classList.toggle('self-update-error',req.status==='failed');
  status.textContent=statusText(req);
  const waiting=req.status==='awaiting_confirmation';
  actions.style.display=waiting?'flex':'none';
  intro.textContent=waiting
    ?'A new VS Code Tasks Menu version is available. Terminal tabs, order, CWD, and split layout will be saved before updating. The daemon will try to keep the same URL and port; running task processes cannot be reattached after the daemon is replaced.'
    :'Updating. This page will reconnect automatically when the new daemon is ready.';
}
function hide(){overlay.classList.remove('visible');currentID='';}

async function checkAndStartUpdate(){
  if(startingFromSettings)return;
  checkUpdate.disabled=true;
  checkUpdate.textContent='Checking…';
  let frozen=false;
  try{
    const result=await app.jsonFetch(endpoint+'&action=check');
    if(!result?.available){
      const short=String(result?.installed_revision||'').slice(0,12);
      showCheckUpdateFeedback(short?'Up to date · '+short:'Up to date');
      return;
    }

    const restore=terminalRestore();
    if(restore?.freezeForSelfUpdate){await restore.freezeForSelfUpdate();frozen=true;}
    else await restore?.persistSnapshot?.();

    startingFromSettings=true;
    settingsStartDeadline=Date.now()+10000;
    checkUpdate.textContent='Updating…';
    overlay.classList.add('visible');
    revision.textContent=result.remote_revision?'Revision: '+result.remote_revision:'';
    intro.textContent='A newer version is available. Starting the existing self-update pipeline automatically…';
    status.classList.remove('self-update-error');
    status.textContent='Starting automatic self-update…';
    actions.style.display='none';
    await app.jsonFetch(endpoint+'&action=start',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});
  }catch(e){
    startingFromSettings=false;
    settingsStartDeadline=0;
    if(frozen)resumeTerminalPersistence();
    resetCheckUpdateButton();
    hide();
    app.showError(e);
  }
}
checkUpdate.onclick=()=>checkAndStartUpdate();


async function postAction(action,id){
  return app.jsonFetch(endpoint+'&action='+encodeURIComponent(action),{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id})});
}

confirm.onclick=async()=>{
  if(!currentID)return;
  confirm.disabled=true;cancel.disabled=true;
  let frozen=false;
  try{
    const restore=terminalRestore();
    if(restore?.freezeForSelfUpdate){await restore.freezeForSelfUpdate();frozen=true;}
    else await restore?.persistSnapshot?.();
    const req=await postAction('confirm',currentID);show(req);
  }catch(e){
    if(frozen)resumeTerminalPersistence();
    app.showError(e);confirm.disabled=false;cancel.disabled=false;
  }
};
cancel.onclick=async()=>{
  if(!currentID)return;
  cancel.disabled=true;confirm.disabled=true;
  try{await postAction('cancel',currentID);resumeTerminalPersistence();hide();}catch(e){app.showError(e);cancel.disabled=false;confirm.disabled=false;}
};

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
  const target=String(req.target_url||location.origin).replace(/\/$/,'');
  const here=location.origin.replace(/\/$/,'');
  const url=new URL(target===here?location.href:target+'/');
  url.searchParams.set('_self_updated',req.id);
  setTimeout(()=>location.replace(url.toString()),450);
}

async function poll(){
  try{
    const req=await app.jsonFetch(endpoint);
    if(!req||req.status==='idle'||!req.id){
      if(startingFromSettings){
        if(Date.now()<settingsStartDeadline){
          overlay.classList.add('visible');
          status.textContent='Waiting for the updater process to start…';
          actions.style.display='none';
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
    if(req.status==='cancelled'){startingFromSettings=false;settingsStartDeadline=0;resumeTerminalPersistence();resetCheckUpdateButton();if(req.id===currentID)hide();return;}
    if(req.status==='completed'){show(req);redirectAfterUpdate(req);return;}
    if(req.status==='failed'){startingFromSettings=false;settingsStartDeadline=0;resumeTerminalPersistence();resetCheckUpdateButton();show(req);actions.style.display='none';return;}
    show(req);
  }catch(e){
    if(currentID){overlay.classList.add('visible');status.textContent='Waiting for the new daemon to start…';actions.style.display='none';}
  }
}

setInterval(poll,900);setTimeout(poll,150);
