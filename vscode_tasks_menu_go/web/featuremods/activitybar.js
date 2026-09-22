const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for activity bar');

if(app.layoutProfile!=='mobile'){
  const main=document.querySelector('main');
  const menu=document.querySelector('#menu');
  const reload=document.querySelector('#reload');
  if(!main||!menu)throw new Error('Task menu layout unavailable for activity bar');

  const style=document.createElement('style');
  style.textContent=`
  .task-activity-bar{display:none;grid-column:1;grid-row:1;position:relative;z-index:1900;width:48px;min-width:48px;height:100%;border-right:1px solid #30343b;background:#0d1015;flex-direction:column;align-items:stretch;padding:4px 3px;gap:3px}
  .task-activity-button{position:relative;width:42px;height:42px;min-height:42px;padding:0;border:0;border-radius:5px;background:transparent;color:#aab1bc;display:grid;place-items:center}
  .task-activity-button:hover{background:#202630;color:#f0f3f7}
  .task-activity-button.active{color:#fff;background:#252c36}
  .task-activity-button.active::before{content:'';position:absolute;left:-3px;top:7px;bottom:7px;width:2px;background:#67a9e8;border-radius:0 2px 2px 0}
  .task-activity-button svg{width:22px;height:22px;fill:none;stroke:currentColor;stroke-width:1.7;stroke-linecap:round;stroke-linejoin:round}
  body.task-sidebar-auto-hide main{grid-template-columns:48px minmax(0,1fr)!important}
  body.task-sidebar-auto-hide .task-activity-bar{display:flex}
  body.task-sidebar-auto-hide main>section{grid-column:2;grid-row:1}
  body.task-sidebar-auto-hide #menu{display:none;position:fixed;top:52px;bottom:0;left:48px;z-index:1750;width:min(var(--taskmenu-sidebar-width,310px),calc(100vw - 48px));background:#101216;border-right:1px solid #30343b;box-shadow:10px 0 28px rgba(0,0,0,.28)}
  body.task-sidebar-auto-hide.task-sidebar-panel-open #menu{display:block}
  body.task-sidebar-auto-hide .sidebar-resizer{display:none!important}
  #sidebar-auto-hide-toggle{width:100%;text-align:left}
  html[data-taskmenu-theme="light"] .task-activity-bar{background:#f0f3f6;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-activity-button{color:#586069}
  html[data-taskmenu-theme="light"] .task-activity-button:hover,html[data-taskmenu-theme="light"] .task-activity-button.active{background:#dfe5eb;color:#202124}
  html[data-taskmenu-theme="light"] body.task-sidebar-auto-hide #menu{background:#f5f7fa;border-color:#d0d7de;box-shadow:10px 0 28px rgba(0,0,0,.12)}
  `;
  document.head.append(style);

  const rail=document.createElement('nav');
  rail.className='task-activity-bar';
  rail.setAttribute('aria-label','Sidebar views');

  function iconButton(view,title,svg){
    const button=document.createElement('button');
    button.type='button';
    button.className='task-activity-button';
    button.dataset.view=view;
    button.title=title;
    button.setAttribute('aria-label',title);
    button.setAttribute('aria-pressed','false');
    button.innerHTML=svg;
    rail.append(button);
    return button;
  }

  const tasksButton=iconButton('tasks','Tasks',`<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 6h2M10 6h9M5 12h2M10 12h9M5 18h2M10 18h9"/></svg>`);
  const explorerButton=iconButton('explorer','Explorer',`<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3.5 6.5h6l2 2h9v10h-17z"/><path d="M3.5 8.5h17"/></svg>`);
  main.prepend(rail);

  const storageKey='vscode-tasks-menu:sidebar-auto-hide:'+app.taskData.workspace;
  let activeView='';

  function readEnabled(){
    try{return localStorage.getItem(storageKey)==='1';}
    catch{return false;}
  }
  function persistEnabled(enabled){
    try{localStorage.setItem(storageKey,enabled?'1':'0');}
    catch(error){console.warn('Cannot persist sidebar auto-hide setting',error);}
  }
  function updateButtons(){
    for(const button of [tasksButton,explorerButton]){
      const active=button.dataset.view===activeView;
      button.classList.toggle('active',active);
      button.setAttribute('aria-pressed',active?'true':'false');
    }
  }
  function fitTerminals(){
    setTimeout(()=>{for(const view of app.views.values())try{view.fit.fit();}catch{}},0);
  }
  function closeActive(){
    const previous=activeView;
    activeView='';
    document.body.classList.remove('task-sidebar-panel-open');
    if(previous==='explorer')globalThis.TaskMenuExplorer?.close();
    updateButtons();
    fitTerminals();
  }
  function openTasks(){
    activeView='tasks';
    globalThis.TaskMenuExplorer?.close();
    document.body.classList.add('task-sidebar-panel-open');
    updateButtons();
    fitTerminals();
  }
  function openExplorer(){
    activeView='explorer';
    document.body.classList.remove('task-sidebar-panel-open');
    globalThis.TaskMenuExplorer?.open();
    updateButtons();
    fitTerminals();
  }
  function activate(view){
    if(!document.body.classList.contains('task-sidebar-auto-hide')){
      if(view==='explorer')globalThis.TaskMenuExplorer?.open();
      return;
    }
    if(activeView===view){closeActive();return;}
    if(view==='tasks')openTasks();
    else if(view==='explorer')openExplorer();
  }

  tasksButton.onclick=()=>activate('tasks');
  explorerButton.onclick=()=>activate('explorer');

  const toggle=document.createElement('button');
  toggle.type='button';
  toggle.id='sidebar-auto-hide-toggle';
  function updateToggle(){
    const enabled=document.body.classList.contains('task-sidebar-auto-hide');
    toggle.textContent=enabled?'Task sidebar: Auto-hide':'Task sidebar: Always visible';
    toggle.title=enabled?'Disable auto-hide and keep Tasks visible':'Collapse Tasks into a left activity bar';
    toggle.setAttribute('aria-pressed',enabled?'true':'false');
  }
  function setAutoHide(enabled,{persist=true}={}){
    const next=Boolean(enabled);
    activeView='';
    document.body.classList.remove('task-sidebar-panel-open');
    globalThis.TaskMenuExplorer?.close();
    document.body.classList.toggle('task-sidebar-auto-hide',next);
    if(persist)persistEnabled(next);
    updateButtons();
    updateToggle();
    window.dispatchEvent(new CustomEvent('taskmenu:sidebar-mode-changed',{detail:{autoHide:next}}));
    fitTerminals();
  }
  toggle.onclick=()=>setAutoHide(!document.body.classList.contains('task-sidebar-auto-hide'));
  reload?.before(toggle);

  window.addEventListener('taskmenu:explorer-opened',()=>{
    if(!document.body.classList.contains('task-sidebar-auto-hide'))return;
    activeView='explorer';
    document.body.classList.remove('task-sidebar-panel-open');
    updateButtons();
  });
  window.addEventListener('taskmenu:explorer-closed',()=>{
    if(activeView!=='explorer')return;
    activeView='';
    updateButtons();
  });
  document.addEventListener('keydown',event=>{
    if(event.key==='Escape'&&document.body.classList.contains('task-sidebar-auto-hide')&&activeView==='tasks')closeActive();
  });

  setAutoHide(readEnabled(),{persist:false});
  globalThis.TaskMenuActivityBar={setAutoHide,activate,get activeView(){return activeView;}};
}
