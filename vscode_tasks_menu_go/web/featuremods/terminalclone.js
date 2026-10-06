const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal clone');

function localTerminal(view){
  const meta=view?.meta;if(!meta)return false;
  const kind=String(meta.kind||'').trim();
  const terminalKind=kind==='terminal'||(!kind&&Number(meta.task_id||0)===0&&String(meta.label||'').toLowerCase().includes('terminal'));
  if(!terminalKind)return false;
  if(String(meta.target_type||'').toLowerCase()==='ssh'||String(meta.target_profile_id||'').trim())return false;
  return true;
}

function cloneAllowed(view){
  if(!localTerminal(view))return false;
  if(!app.sharedMode)return true;
  return Boolean(app.hasPermission?.('terminal.create')&&app.canControlSession?.(view.meta));
}

async function cloneTerminal(view){
  if(!view||view.closed||!localTerminal(view))throw new Error('Only local terminals can be duplicated');
  if(!cloneAllowed(view))throw new Error('Terminal create and control permissions are required to duplicate this terminal');
  const meta=await app.jsonFetch('/api/sessions/'+encodeURIComponent(view.meta.id)+'/clone',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:'{}'
  });
  return app.attachSession(meta,true);
}

function decorate(view){
  if(!localTerminal(view))return null;
  const head=view?.pane?.querySelector('.pane-head');if(!head)return null;
  let button=head.querySelector('.session-clone')||view.pane.querySelector('.session-clone');
  if(button){button.disabled=!cloneAllowed(view);return button;}
  button=document.createElement('button');
  button.type='button';
  button.className='session-clone';
  button.textContent='Duplicate terminal here';
  button.title='Duplicate this local terminal with its live CWD and launch environment';
  button.disabled=!cloneAllowed(view);
  button.onclick=()=>cloneTerminal(view).catch(app.showError);
  head.append(button);
  return button;
}

window.addEventListener('taskmenu:session',event=>{
  const view=event.detail?.view;if(view)setTimeout(()=>decorate(view),0);
});
window.addEventListener('taskmenu:permissions-changed',()=>{
  for(const view of app.views.values())decorate(view);
});
for(const view of app.views.values())decorate(view);

globalThis.TaskMenuTerminalClone={cloneTerminal,decorate,localTerminal,cloneAllowed};
