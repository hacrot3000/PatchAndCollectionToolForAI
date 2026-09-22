const app=globalThis.TaskMenuApp;

function installActivityBar(){
  if(!app||app.layoutProfile==='mobile'||!app.taskData?.workspace)return false;
  if(document.querySelector('.task-activity-bar'))return true;

  const main=document.querySelector('main');
  const menu=document.querySelector('#menu');
  const appearance=document.querySelector('.appearance-controls');
  if(!main||!menu||!appearance)return false;

  const style=document.createElement('style');
  style.textContent=`
  .task-activity-bar{display:none;grid-column:1;grid-row:1;position:relative;z-index:1900;width:48px;min-width:48px;height:100%;border-right:1px solid #30343b;background:#0d1015;flex-direction:column;align-items:center;padding:4px 3px;gap:3px}
  .task-activity-button{position:relative;width:42px;height:42px;min-height:42px;padding:0;border:0;border-radius:5px;background:transparent;color:#aab1bc;display:grid;place-items:center}
  .task-activity-button:hover{background:#202630;color:#f0f3f7}
  .task-activity-button.active{color:#fff;background:#252c36}
  .task-activity-button.active::before{content:'';position:absolute;left:-3px;top:7px;bottom:7px;width:2px;background:#67a9e8;border-radius:0 2px 2px 0}
  .task-activity-button svg{width:22px;height:22px;fill:none;stroke:currentColor;stroke-width:1.7;stroke-linecap:round;stroke-linejoin:round}
  body.task-sidebar-auto-hide main{grid-template-columns:48px minmax(0,1fr)!important}
  body.task-sidebar-auto-hide .task-activity-bar{display:flex}
  body.task-sidebar-auto-hide main>section{grid-column:2!important;grid-row:1!important}
  body.task-sidebar-auto-hide #menu{display:none;position:fixed;top:52px;bottom:0;left:48px;z-index:1750;width:min(var(--taskmenu-sidebar-panel-width,310px),calc(100vw - 48px));background:#101216;border-right:1px solid #30343b;box-shadow:10px 0 28px rgba(0,0,0,.28)}
  body.task-sidebar-auto-hide.task-sidebar-panel-open #menu{display:block}
  body.task-sidebar-auto-hide .sidebar-resizer{display:none!important}
  body.task-sidebar-auto-hide .project-explorer{left:48px!important;width:min(var(--taskmenu-sidebar-panel-width,310px),calc(100vw - 48px))!important}
  #sidebar-auto-hide-toggle{grid-column:1/-1;width:100%!important;text-align:left}
  html[data-taskmenu-theme="light"] .task-activity-bar{background:#f0f3f6;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-activity-button{color:#586069}
  html[data-taskmenu-theme="light"] .task-activity-button:hover,html[data-taskmenu-theme="light"] .task-activity-button.active{background:#dfe5eb;color:#202124}
  html[data-taskmenu-theme="light"] body.task-sidebar-auto-hide #menu{background:#f5f7fa;border-color:#d0d7de;box-shadow:10px 0 28px rgba(0,0,0,.12)}
  `;
  document.head.append(style);

  const rail=document.createElement('nav');
  rail.className='task-activity-bar';
  rail.setAttribute('aria-label','Sidebar views');

  function makeButton(view,title,svg){
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

  const tasksButton=makeButton('tasks','Tasks',`<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M5 6h2M10 6h9M5 12h2M10 12h9M5 18h2M10 18h9"/></svg>`);
  const explorerButton=makeButton('explorer','Explorer',`<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3.5 6.5h6l2 2h9v10h-17z"/><path d="M3.5 8.5h17"/></svg>`);
  main.prepend(rail);

  const toggle=document.createElement('button');
  toggle.type='button';
  toggle.id='sidebar-auto-hide-toggle';
  appearance.append(toggle);

  const modeKey='vscode-tasks-menu:sidebar-auto-hide:'+app.taskData.workspace;
  const widthKey='vscode-tasks-menu:sidebar-width:'+app.taskData.workspace;
  let activeView='';

  function sidebarWidth(){
    let value=310;
    try{value=Number(localStorage.getItem(widthKey))||310;}catch{}
    return Math.max(220,Math.min(650,Math.round(value)));
  }
  function refreshPanelWidth(){
    main.style.setProperty('--taskmenu-sidebar-panel-width',sidebarWidth()+'px');
  }
  function enabled(){
    return document.body.classList.contains('task-sidebar-auto-hide');
  }
  function updateButtons(){
    for(const button of [tasksButton,explorerButton]){
      const active=enabled()&&button.dataset.view===activeView;
      button.classList.toggle('active',active);
      button.setAttribute('aria-pressed',active?'true':'false');
    }
  }
  function updateToggle(){
    const on=enabled();
    toggle.textContent=on?'Task sidebar: Auto-hide':'Task sidebar: Always visible';
    toggle.title=on?'Keep the Tasks sidebar always visible':'Collapse the Tasks sidebar into the left activity bar';
    toggle.setAttribute('aria-pressed',on?'true':'false');
  }
  function fitTerminals(){
    setTimeout(()=>{for(const view of app.views.values())try{view.fit.fit();}catch{}},0);
  }
  function hideTasksPanel(){
    document.body.classList.remove('task-sidebar-panel-open');
  }
  function closeExplorer(){
    try{globalThis.TaskMenuExplorer?.close();}catch(error){console.warn('Auto sidebar could not close Explorer',error);}
  }
  function closeActive(){
    const previous=activeView;
    activeView='';
    hideTasksPanel();
    if(previous==='explorer')closeExplorer();
    updateButtons();
    fitTerminals();
  }
  function showTasks(){
    activeView='tasks';
    closeExplorer();
    document.body.classList.add('task-sidebar-panel-open');
    updateButtons();
    fitTerminals();
  }
  function showExplorer(){
    activeView='explorer';
    hideTasksPanel();
    try{globalThis.TaskMenuExplorer?.open();}catch(error){console.warn('Auto sidebar could not open Explorer',error);}
    updateButtons();
    fitTerminals();
  }
  function activate(view){
    if(!enabled()){
      if(view==='explorer'){
        try{globalThis.TaskMenuExplorer?.open();}catch(error){console.warn('Explorer unavailable',error);}
      }
      return;
    }
    if(activeView===view){closeActive();return;}
    if(view==='tasks')showTasks();
    if(view==='explorer')showExplorer();
  }

  tasksButton.onclick=()=>activate('tasks');
  explorerButton.onclick=()=>activate('explorer');

  const explorerPanel=document.querySelector('.project-explorer');
  if(explorerPanel){
    const observer=new MutationObserver(()=>{
      if(!enabled())return;
      const visible=explorerPanel.classList.contains('visible');
      if(visible){
        activeView='explorer';
        hideTasksPanel();
      }else if(activeView==='explorer'){
        activeView='';
      }
      updateButtons();
    });
    observer.observe(explorerPanel,{attributes:true,attributeFilter:['class']});
  }

  function setAutoHide(value,{persist=true}={}){
    const on=Boolean(value);
    activeView='';
    hideTasksPanel();
    closeExplorer();
    refreshPanelWidth();
    document.body.classList.toggle('task-sidebar-auto-hide',on);
    if(persist){
      try{localStorage.setItem(modeKey,on?'1':'0');}catch(error){console.warn('Cannot persist auto sidebar mode',error);}
    }
    updateButtons();
    updateToggle();
    fitTerminals();
  }

  toggle.onclick=()=>setAutoHide(!enabled());
  document.addEventListener('keydown',event=>{
    if(event.key==='Escape'&&enabled()&&activeView==='tasks')closeActive();
  });
  window.addEventListener('resize',()=>{if(enabled())refreshPanelWidth();});

  let initial=false;
  try{initial=localStorage.getItem(modeKey)==='1';}catch{}
  setAutoHide(initial,{persist:false});

  globalThis.TaskMenuActivityBar={setAutoHide,activate,get activeView(){return activeView;}};
  return true;
}

function safeInstallActivityBar(){
  try{installActivityBar();}
  catch(error){console.warn('Auto sidebar enhancement disabled:',error);}
}

if(app?.taskData?.workspace)safeInstallActivityBar();
else window.addEventListener('taskmenu:tasks',safeInstallActivityBar,{once:true});
