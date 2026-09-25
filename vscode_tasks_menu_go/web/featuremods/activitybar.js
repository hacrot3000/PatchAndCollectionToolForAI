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
  .task-history-panel{display:none;position:fixed;top:52px;bottom:0;left:48px;z-index:1800;width:min(var(--taskmenu-sidebar-panel-width,310px),calc(100vw - 48px));background:#11151b;border-right:1px solid #3b414d;box-shadow:10px 0 28px rgba(0,0,0,.28);flex-direction:column}
  .task-history-panel.visible{display:flex}
  .task-history-panel-head{height:42px;display:flex;align-items:center;gap:6px;padding:6px 8px;border-bottom:1px solid #30343b}
  .task-history-panel-title{font-size:12px;font-weight:700;letter-spacing:.04em;flex:1}
  .task-history-panel-close{padding:4px 7px;font-size:11px}
  .task-history-panel-content{flex:1;min-height:0;overflow:auto;padding:6px}
  .task-history-panel .history-section{display:flex;flex-direction:column;height:100%;margin:0;border:0;border-radius:0;background:transparent}
  .task-history-panel .history-head{justify-content:flex-end}
  .task-history-panel .history-head strong{display:none}
  .task-history-panel .history-list{max-height:none;flex:1}
  .task-history-empty{padding:12px 8px;opacity:.6;font-size:12px}
  #sidebar-auto-hide-toggle{grid-column:1/-1;width:100%!important;text-align:left}
  html[data-taskmenu-theme="light"] .task-activity-bar{background:#f0f3f6;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-activity-button{color:#586069}
  html[data-taskmenu-theme="light"] .task-activity-button:hover,html[data-taskmenu-theme="light"] .task-activity-button.active{background:#dfe5eb;color:#202124}
  html[data-taskmenu-theme="light"] body.task-sidebar-auto-hide #menu{background:#f5f7fa;border-color:#d0d7de;box-shadow:10px 0 28px rgba(0,0,0,.12)}
  html[data-taskmenu-theme="light"] .task-history-panel{background:#fff;border-color:#b9c0c8;box-shadow:10px 0 28px rgba(0,0,0,.12)}
  html[data-taskmenu-theme="light"] .task-history-panel-head{border-color:#d0d7de}
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
  const patchButton=makeButton('patch','Patch Tool',`<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M7 5h10v14H7z"/><path d="M9.5 8.5h5M9.5 12h5M9.5 15.5h3"/></svg>`);
  const historyButton=makeButton('history','History',`<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8.5"/><path d="M12 7.5v5l3 2"/></svg>`);
  main.prepend(rail);

  const historyPanel=document.createElement('div');
  historyPanel.className='task-history-panel';
  const historyPanelHead=document.createElement('div');historyPanelHead.className='task-history-panel-head';
  const historyPanelTitle=document.createElement('div');historyPanelTitle.className='task-history-panel-title';historyPanelTitle.textContent='HISTORY';
  const historyPanelClose=document.createElement('button');historyPanelClose.type='button';historyPanelClose.className='task-history-panel-close';historyPanelClose.textContent='×';historyPanelClose.title='Close History';
  const historyContent=document.createElement('div');historyContent.className='task-history-panel-content';
  historyPanelHead.append(historyPanelTitle,historyPanelClose);historyPanel.append(historyPanelHead,historyContent);document.body.append(historyPanel);

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
    for(const button of [tasksButton,explorerButton,patchButton,historyButton]){
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
  function historyAnchor(){
    const quick=[...menu.querySelectorAll('.quick-task-section')];
    return quick.length?quick[quick.length-1].nextSibling:menu.querySelector('.task-search-panel')?.nextSibling;
  }
  function showHistoryEmpty(){
    if(historyContent.querySelector('.history-section'))return;
    historyContent.replaceChildren();
    const empty=document.createElement('div');empty.className='task-history-empty';empty.textContent='No task history yet';
    historyContent.append(empty);
  }
  function adoptHistoryFromTasks(){
    if(!enabled())return;
    const section=menu.querySelector('.history-section');
    if(section){
      historyContent.querySelector('.history-section')?.remove();
      historyContent.replaceChildren(section);
    }
    showHistoryEmpty();
  }
  function restoreHistoryToTasks(){
    const section=historyContent.querySelector('.history-section');
    if(!section)return;
    const anchor=historyAnchor();
    if(anchor)menu.insertBefore(section,anchor);else menu.append(section);
    historyContent.replaceChildren();
  }
  function closeHistoryPanel(){
    historyPanel.classList.remove('visible');
  }
  function closeExplorer(){
    try{globalThis.TaskMenuExplorer?.close();}catch(error){console.warn('Auto sidebar could not close Explorer',error);}
  }
  function deactivatePatchPanel(){
    try{globalThis.TaskMenuPatchPanel?.deactivate();}catch(error){console.warn('Auto sidebar could not deactivate Patch Tool',error);}
  }
  function closeActive(){
    const previous=activeView;
    activeView='';
    hideTasksPanel();
    if(previous==='explorer')closeExplorer();
    if(previous==='patch')deactivatePatchPanel();
    if(previous==='history')closeHistoryPanel();
    updateButtons();
    fitTerminals();
  }
  function showTasks(){
    activeView='tasks';
    closeExplorer();
    deactivatePatchPanel();
    closeHistoryPanel();
    document.body.classList.add('task-sidebar-panel-open');
    updateButtons();
    fitTerminals();
  }
  function showExplorer(){
    activeView='explorer';
    hideTasksPanel();
    deactivatePatchPanel();
    closeHistoryPanel();
    try{globalThis.TaskMenuExplorer?.open();}catch(error){console.warn('Auto sidebar could not open Explorer',error);}
    updateButtons();
    fitTerminals();
  }
  function showPatch(){
    activeView='patch';
    hideTasksPanel();
    closeExplorer();
    closeHistoryPanel();
    try{globalThis.TaskMenuPatchPanel?.open();}catch(error){console.warn('Auto sidebar could not open Patch Tool',error);}
    updateButtons();
    fitTerminals();
  }
  function showHistory(){
    activeView='history';
    hideTasksPanel();
    closeExplorer();
    deactivatePatchPanel();
    adoptHistoryFromTasks();
    historyPanel.classList.add('visible');
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
    if(view==='patch')showPatch();
    if(view==='history')showHistory();
  }

  tasksButton.onclick=()=>activate('tasks');
  explorerButton.onclick=()=>activate('explorer');
  patchButton.onclick=()=>activate('patch');
  historyButton.onclick=()=>activate('history');
  historyPanelClose.onclick=()=>{if(activeView==='history')closeActive();else closeHistoryPanel();};

  const historyObserver=new MutationObserver(()=>{if(enabled())adoptHistoryFromTasks();});
  historyObserver.observe(menu,{childList:true,subtree:true});
  historyContent.addEventListener('click',event=>{
    const target=event.target instanceof Element?event.target:null;
    if(!target?.closest('.history-clear'))return;
    setTimeout(()=>{
      historyContent.querySelector('.history-section')?.remove();
      showHistoryEmpty();
    },0);
  });

  window.addEventListener('taskmenu:patch-panel-visible',event=>{
    if(!enabled())return;
    const visible=Boolean(event.detail?.visible);
    if(visible){
      activeView='patch';
      hideTasksPanel();
      closeExplorer();
      closeHistoryPanel();
    }else if(activeView==='patch'){
      activeView='';
    }
    updateButtons();
  });

  const explorerPanel=document.querySelector('.project-explorer');
  if(explorerPanel){
    const observer=new MutationObserver(()=>{
      if(!enabled())return;
      const visible=explorerPanel.classList.contains('visible');
      if(visible){
        activeView='explorer';
        hideTasksPanel();
        closeHistoryPanel();
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
    deactivatePatchPanel();
    closeHistoryPanel();
    refreshPanelWidth();
    document.body.classList.toggle('task-sidebar-auto-hide',on);
    if(on)adoptHistoryFromTasks();else restoreHistoryToTasks();
    if(persist){
      try{localStorage.setItem(modeKey,on?'1':'0');}catch(error){console.warn('Cannot persist auto sidebar mode',error);}
    }
    updateButtons();
    updateToggle();
    fitTerminals();
  }

  toggle.onclick=()=>setAutoHide(!enabled());

  document.addEventListener('pointerdown',event=>{
    if(!enabled()||!activeView)return;
    const target=event.target;
    if(!(target instanceof Node))return;
    if(rail.contains(target))return;
    const headerMenus=document.querySelector('.header-action-menus');
    if(headerMenus?.contains(target))return;
    if(activeView==='tasks'&&menu.contains(target))return;
    if(activeView==='explorer'&&explorerPanel?.contains(target))return;
    if(activeView==='patch'&&globalThis.TaskMenuPatchPanel?.panel?.contains(target))return;
    if(activeView==='history'&&historyPanel.contains(target))return;
    closeActive();
  },true);

  document.addEventListener('keydown',event=>{
    if(event.key==='Escape'&&enabled()&&(activeView==='tasks'||activeView==='patch'||activeView==='history'))closeActive();
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
