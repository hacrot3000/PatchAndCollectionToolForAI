const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for restart/clear controls');

const style=document.createElement('style');
style.textContent=`
.session-rerun{display:none!important}
.session-force-restart,.session-clear-console,.session-terminate,.session-force-kill,.session-process-tree{white-space:nowrap}
.session-force-restart{background:#49351f;border-color:#795b34}
.session-force-kill{color:#ff9fa7}
.session-process-backdrop{display:none;position:fixed;inset:0;z-index:2650;background:rgba(0,0,0,.7);align-items:center;justify-content:center;padding:24px}
.session-process-backdrop.visible{display:flex}
.session-process-dialog{width:min(900px,96vw);max-height:82vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #4a5362;border-radius:9px;box-shadow:0 20px 60px rgba(0,0,0,.55)}
.session-process-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #30343b}
.session-process-title{flex:1;font-weight:700}
.session-process-body{overflow:auto;padding:8px 10px}
.session-process-table{width:100%;border-collapse:collapse;font:10px/1.4 ui-monospace,monospace}
.session-process-table th,.session-process-table td{text-align:left;vertical-align:top;padding:4px 6px;border-bottom:1px solid #292f38}
.session-process-command{word-break:break-all}
html[data-taskmenu-theme="light"] .session-process-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .session-process-table th,html[data-taskmenu-theme="light"] .session-process-table td{border-color:#dde2e8}
`;
document.head.append(style);

function taskByID(id){return app.taskData?.tasks.find(t=>t.id===id)||null;}

const processBackdrop=document.createElement('div');processBackdrop.className='session-process-backdrop';
const processDialog=document.createElement('div');processDialog.className='session-process-dialog';processDialog.setAttribute('role','dialog');processDialog.setAttribute('aria-modal','true');
const processHead=document.createElement('div');processHead.className='session-process-head';
const processTitle=document.createElement('div');processTitle.className='session-process-title';processTitle.textContent='Session process tree';
const processRefresh=document.createElement('button');processRefresh.textContent='Refresh';
const processClose=document.createElement('button');processClose.textContent='Close';
const processBody=document.createElement('div');processBody.className='session-process-body';
processHead.append(processTitle,processRefresh,processClose);processDialog.append(processHead,processBody);processBackdrop.append(processDialog);document.body.append(processBackdrop);
let processSessionID='';
function closeProcessTree(){processBackdrop.classList.remove('visible');processSessionID='';}
processClose.onclick=closeProcessTree;processBackdrop.addEventListener('mousedown',event=>{if(event.target===processBackdrop)closeProcessTree();});

function elapsedLabel(seconds){seconds=Number(seconds)||0;if(seconds<60)return seconds+'s';if(seconds<3600)return Math.floor(seconds/60)+'m '+(seconds%60)+'s';return Math.floor(seconds/3600)+'h '+Math.floor((seconds%3600)/60)+'m';}
function renderProcessTree(rows){
  processBody.replaceChildren();
  if(!rows.length){const empty=document.createElement('div');empty.textContent='No running process is attached to this session.';processBody.append(empty);return;}
  const table=document.createElement('table');table.className='session-process-table';
  const head=document.createElement('thead'),hr=document.createElement('tr');
  for(const label of ['PID','PPID','State','Elapsed','Command']){const th=document.createElement('th');th.textContent=label;hr.append(th);}head.append(hr);table.append(head);
  const body=document.createElement('tbody');
  for(const row of rows){
    const tr=document.createElement('tr');
    const values=[row.pid,row.ppid,row.state||'',elapsedLabel(row.elapsed_seconds),row.args||row.command||''];
    values.forEach((value,index)=>{const td=document.createElement('td');if(index===4){td.className='session-process-command';td.style.paddingLeft=(6+Math.max(0,Number(row.depth)||0)*18)+'px';}td.textContent=String(value??'');tr.append(td);});
    body.append(tr);
  }
  table.append(body);processBody.append(table);
}
async function loadProcessTree(){
  if(!processSessionID)return;
  processRefresh.disabled=true;
  try{
    const data=await app.jsonFetch('/api/sessions/process-tree?id='+encodeURIComponent(processSessionID));
    renderProcessTree(Array.isArray(data?.processes)?data.processes:[]);
  }finally{processRefresh.disabled=false;}
}
processRefresh.onclick=()=>loadProcessTree().catch(app.showError);
async function openProcessTree(view){processSessionID=view.meta.id;processTitle.textContent='Process tree · '+(view.meta.title||view.meta.label||view.meta.id);processBackdrop.classList.add('visible');await loadProcessTree();}
async function terminateSession(view){
  if(!window.confirm('Send SIGTERM to this session process group? Applications may perform normal shutdown cleanup.'))return;
  await app.jsonFetch('/api/sessions/terminate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:view.meta.id})});
}
async function forceKillSession(view){
  if(!window.confirm('Force kill this session process group with SIGKILL? Unsaved process state may be lost.'))return;
  await app.jsonFetch('/api/sessions/force-kill',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:view.meta.id})});
}

async function waitUntilStopped(id,timeoutMs=6000){
  const deadline=Date.now()+timeoutMs;
  let meta=null;
  while(Date.now()<deadline){
    meta=await app.jsonFetch('/api/sessions/'+id);
    if(meta.status!=='running')return meta;
    await new Promise(resolve=>setTimeout(resolve,100));
  }
  meta=await app.jsonFetch('/api/sessions/'+id);
  if(meta.status==='running')throw new Error('The process is still running after force-kill; it will not be restarted to avoid overlapping processes.');
  return meta;
}

async function restartTask(view,button){
  const task=taskByID(view.meta.task_id);
  if(!task)throw new Error('Task no longer exists in tasks.json');
  button.disabled=true;
  try{
    if(view.meta.status==='running'){
      await app.jsonFetch('/api/sessions/force-kill',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:view.meta.id})});
      await waitUntilStopped(view.meta.id);
    }
    await app.startTask(task);
  }finally{
    if(button.isConnected)button.disabled=false;
  }
}

async function clearConsole(view,button){
  if(!window.confirm('Clear this console and server-side scrollback? This cannot be undone. The running process will not be stopped.'))return;
  button.disabled=true;
  try{
    await app.jsonFetch('/api/sessions/clear-console',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:view.meta.id})});
    view.term.clearSelection();
    view.term.clear();
    view.term.write('\x1b[2J\x1b[H');
    view.term.focus();
  }finally{
    if(button.isConnected)button.disabled=false;
  }
}

function decorate(view){
  const head=view.pane.querySelector('.pane-head');if(!head)return;
  if(view.meta.task_id>0&&!head.querySelector('.session-force-restart')){
    const restart=document.createElement('button');restart.className='session-force-restart';restart.textContent='Restart';restart.title='Force-kill the current process if needed, then run the task again from the beginning';restart.onclick=()=>restartTask(view,restart).catch(app.showError);view.stop.before(restart);
  }
  if(!head.querySelector('.session-process-tree')){
    const tree=document.createElement('button');tree.className='session-process-tree';tree.textContent='Process tree…';tree.title='Inspect the process tree owned by this session';tree.onclick=()=>openProcessTree(view).catch(app.showError);view.stop.before(tree);
  }
  if(view.canControl&&!head.querySelector('.session-terminate')){
    const terminate=document.createElement('button');terminate.className='session-terminate';terminate.textContent='Terminate';terminate.title='Send SIGTERM to the whole session process group';terminate.onclick=()=>terminateSession(view).catch(app.showError);view.stop.after(terminate);
    const kill=document.createElement('button');kill.className='session-force-kill';kill.textContent='Force kill';kill.title='Send SIGKILL to the whole session process group';kill.onclick=()=>forceKillSession(view).catch(app.showError);terminate.after(kill);
  }
  if(!head.querySelector('.session-clear-console')){
    const clear=document.createElement('button');clear.className='session-clear-console';clear.textContent='🧹 Clear console';clear.title='Clear the console and server-side scrollback without stopping the running process';clear.onclick=()=>clearConsole(view,clear).catch(app.showError);view.copy.before(clear);
  }
}

window.addEventListener('taskmenu:session',event=>{const view=event.detail?.view;if(view)decorate(view);});
for(const view of app.views.values())decorate(view);
