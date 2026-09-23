const app=globalThis.TaskMenuApp;

function installPatchPanel(){
  if(!app||app.layoutProfile==='mobile'||!app.taskData?.workspace)return false;
  if(globalThis.TaskMenuPatchPanel)return true;

  const style=document.createElement('style');
  style.textContent=`
  .task-patch-panel{display:none;position:fixed;top:52px;bottom:0;left:48px;z-index:1800;width:min(var(--taskmenu-sidebar-panel-width,310px),calc(100vw - 48px));background:#11151b;border-right:1px solid #3b414d;box-shadow:10px 0 28px rgba(0,0,0,.28);flex-direction:column}
  .task-patch-panel.visible{display:flex}
  .task-patch-panel-head{height:42px;display:flex;align-items:center;gap:6px;padding:6px 8px;border-bottom:1px solid #30343b}
  .task-patch-panel-title{font-size:12px;font-weight:700;letter-spacing:.04em;flex:1}
  .task-patch-panel-close{padding:4px 7px;font-size:11px}
  .task-patch-panel-body{padding:10px;overflow:auto}
  .task-patch-panel-note{font-size:12px;line-height:1.45;opacity:.72;margin:0 0 10px}
  .task-patch-actions{display:grid;gap:7px}
  .task-patch-action{display:flex;flex-direction:column;align-items:flex-start;gap:2px;width:100%;padding:9px 10px;text-align:left}
  .task-patch-action strong{font-size:12px}
  .task-patch-action span{font-size:11px;opacity:.65}
  html[data-taskmenu-theme="light"] .task-patch-panel{background:#fff;border-color:#b9c0c8;box-shadow:10px 0 28px rgba(0,0,0,.12)}
  html[data-taskmenu-theme="light"] .task-patch-panel-head{border-color:#d0d7de}
  `;
  document.head.append(style);

  const panel=document.createElement('aside');
  panel.className='task-patch-panel';
  panel.setAttribute('aria-label','Patch Tool');

  const head=document.createElement('div');head.className='task-patch-panel-head';
  const title=document.createElement('div');title.className='task-patch-panel-title';title.textContent='PATCH TOOL';
  const closeButton=document.createElement('button');closeButton.type='button';closeButton.className='task-patch-panel-close';closeButton.textContent='×';closeButton.title='Close Patch Tool';
  head.append(title,closeButton);

  const body=document.createElement('div');body.className='task-patch-panel-body';
  const note=document.createElement('p');note.className='task-patch-panel-note';
  note.textContent='Built-in Python Patch Tool. Actions open in the existing TaskDeck terminal so the current interactive workflow is preserved.';
  const actions=document.createElement('div');actions.className='task-patch-actions';
  body.append(note,actions);
  panel.append(head,body);
  document.body.append(panel);

  const actionDefs=[
    ['queue','Queue','Open the normal PATCH/COLLECT queue'],
    ['resume','Resume','Continue an interrupted or failed run'],
    ['history','History','Open Patch Tool reports/history'],
    ['plan','Plan','Inspect the execution plan without replacing the Python engine'],
  ];
  const buttons=[];
  for(const [mode,label,detail] of actionDefs){
    const button=document.createElement('button');
    button.type='button';
    button.className='task-patch-action';
    button.dataset.patchMode=mode;
    const strong=document.createElement('strong');strong.textContent=label;
    const span=document.createElement('span');span.textContent=detail;
    button.append(strong,span);
    button.onclick=()=>start(mode,button).catch(app.showError);
    actions.append(button);buttons.push(button);
  }

  function setVisible(value){
    const visible=Boolean(value);
    panel.classList.toggle('visible',visible);
    window.dispatchEvent(new CustomEvent('taskmenu:patch-panel-visible',{detail:{visible}}));
  }
  function open(){setVisible(true);}
  function close(){setVisible(false);}
  function toggle(){setVisible(!panel.classList.contains('visible'));}

  async function start(mode,sourceButton=null){
    if(sourceButton)sourceButton.disabled=true;
    try{
      const meta=await app.jsonFetch('/api/sessions',{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({kind:'patch',patch_mode:mode}),
      });
      app.attachSession(meta,true);
      window.dispatchEvent(new CustomEvent('taskmenu:patch-session-started',{detail:{mode,meta}}));
      return meta;
    }finally{
      if(sourceButton?.isConnected)sourceButton.disabled=false;
    }
  }

  closeButton.onclick=close;
  globalThis.TaskMenuPatchPanel={open,close,toggle,start,get panel(){return panel;},get visible(){return panel.classList.contains('visible');}};
  return true;
}

function safeInstallPatchPanel(){
  try{installPatchPanel();}
  catch(error){console.warn('Patch panel enhancement disabled:',error);}
}

if(app?.taskData?.workspace)safeInstallPatchPanel();
else window.addEventListener('taskmenu:tasks',safeInstallPatchPanel,{once:true});
