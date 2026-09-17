const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for restart/clear controls');

const style=document.createElement('style');
style.textContent=`
.session-rerun{display:none!important}
.session-force-restart,.session-clear-console{white-space:nowrap}
.session-force-restart{background:#49351f;border-color:#795b34}
`;
document.head.append(style);

function taskByID(id){return app.taskData?.tasks.find(t=>t.id===id)||null;}

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
  if(!head.querySelector('.session-clear-console')){
    const clear=document.createElement('button');clear.className='session-clear-console';clear.textContent='🧹 Clear console';clear.title='Clear the console and server-side scrollback without stopping the running process';clear.onclick=()=>clearConsole(view,clear).catch(app.showError);view.copy.before(clear);
  }
}

window.addEventListener('taskmenu:session',event=>{const view=event.detail?.view;if(view)decorate(view);});
for(const view of app.views.values())decorate(view);
