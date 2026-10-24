const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for mobile UI');

if(app.layoutProfile==='mobile'){
  const style=document.createElement('style');
  style.dataset.taskmenuMobile='1';
  style.textContent=`
  html[data-taskmenu-layout="mobile"]{--taskmenu-mobile-height:100dvh;--taskmenu-header-height:50px}
  html[data-taskmenu-layout="mobile"] body{height:var(--taskmenu-mobile-height,100dvh)!important;max-height:var(--taskmenu-mobile-height,100dvh)!important;padding-top:env(safe-area-inset-top);padding-bottom:env(safe-area-inset-bottom)}
  html[data-taskmenu-layout="mobile"] body>header{height:50px!important;min-height:50px!important;max-height:50px!important;padding:0 8px!important;gap:8px!important;z-index:1300;background:#101216}
  html[data-taskmenu-layout="mobile"] body.mobile-actions-open>header{z-index:1600}
  html[data-taskmenu-layout="mobile"] body>header>strong{font-size:13px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:42vw}
  html[data-taskmenu-layout="mobile"] body>header>#workspace{display:none}
  html[data-taskmenu-layout="mobile"] body>main{display:grid!important;grid-template-columns:minmax(0,1fr)!important;max-height:calc(100% - 50px)!important}
  html[data-taskmenu-layout="mobile"] body>main>aside{position:fixed;z-index:1500;top:calc(50px + env(safe-area-inset-top));bottom:env(safe-area-inset-bottom);left:0;width:min(88vw,360px);max-width:360px;background:#101216;border-right:1px solid #30343b;box-shadow:8px 0 28px rgba(0,0,0,.45);transform:translateX(-105%);transition:transform .16s ease;padding:10px 10px 18px;box-sizing:border-box;overflow-y:auto;overscroll-behavior:contain;-webkit-overflow-scrolling:touch}
  html[data-taskmenu-layout="mobile"] body.mobile-drawer-open>main>aside{transform:translateX(0)}
  html[data-taskmenu-layout="mobile"] .sidebar-resizer{display:none!important}
  html[data-taskmenu-layout="mobile"] body>main>section{grid-column:1!important;width:100%;min-width:0}
  html[data-taskmenu-layout="mobile"] #tabs{height:48px!important;flex:0 0 48px!important;align-items:stretch!important;scroll-snap-type:x proximity;-webkit-overflow-scrolling:touch;padding-right:6px}
  html[data-taskmenu-layout="mobile"] #tabs .tab{min-height:44px;max-width:75vw;scroll-snap-align:start;padding:8px 10px;margin-left:3px}
  html[data-taskmenu-layout="mobile"] #tabs .tab .close{display:inline-flex;align-items:center;justify-content:center;min-width:30px;min-height:30px;margin-left:5px}
  html[data-taskmenu-layout="mobile"] .pane-head{min-height:44px;padding:4px 6px;gap:5px;overflow:visible}
  html[data-taskmenu-layout="mobile"] .pane-head>.command{display:none}
  html[data-taskmenu-layout="mobile"] .pane-action-menus{margin-left:0;flex:1;justify-content:flex-end;gap:4px}
  html[data-taskmenu-layout="mobile"] .pane-action-menus .taskmenu-menu-trigger{min-height:38px;padding:7px 10px}
  html[data-taskmenu-layout="mobile"] .taskmenu-menu-popover{position:fixed;left:8px!important;right:8px!important;top:auto!important;bottom:calc(8px + env(safe-area-inset-bottom));max-width:none!important;max-height:72vh;overflow:auto}
  html[data-taskmenu-layout="mobile"] button,html[data-taskmenu-layout="mobile"] select,html[data-taskmenu-layout="mobile"] input{min-height:42px}
  html[data-taskmenu-layout="mobile"] .task,html[data-taskmenu-layout="mobile"] .group>button,html[data-taskmenu-layout="mobile"] .quick-task-run{min-height:44px;padding-top:9px;padding-bottom:9px}
  html[data-taskmenu-layout="mobile"] .terminal{padding:4px}
  html[data-taskmenu-layout="mobile"] .mobile-header-button{display:inline-flex;align-items:center;justify-content:center;min-width:42px;min-height:42px;padding:6px 9px;font-size:18px}
  html[data-taskmenu-layout="mobile"] .mobile-tools-button{min-width:76px;font-size:12px;font-weight:750;white-space:nowrap;background:#223950;border:1px solid #446b93;border-radius:7px;padding:7px 9px}
  html[data-taskmenu-layout="mobile"] .mobile-tools-button[aria-expanded="true"]{background:#315982;color:#fff}
  html[data-taskmenu-layout="mobile"] .mobile-menu-button{flex:0 0 42px}
  html[data-taskmenu-layout="mobile"] .mobile-workspace-tools{scroll-margin-top:12px}
  html[data-taskmenu-layout="mobile"] body>header>strong{max-width:25vw}
  html[data-taskmenu-layout="mobile"] .mobile-actions-button{margin-left:auto}
  html[data-taskmenu-layout="mobile"] .header-action-menus{display:none;position:fixed;z-index:1550;top:calc(54px + env(safe-area-inset-top));right:8px;width:min(92vw,360px);max-height:calc(100dvh - 78px);overflow:auto;padding:8px;border:1px solid #3b414d;border-radius:10px;background:#171a20;box-shadow:0 12px 34px rgba(0,0,0,.5);flex-direction:column;align-items:stretch}
  html[data-taskmenu-layout="mobile"] body.mobile-actions-open .header-action-menus{display:flex}
  html[data-taskmenu-layout="mobile"] .header-action-menus>.taskmenu-menu,html[data-taskmenu-layout="mobile"] .header-action-menus>.taskmenu-direct-action,html[data-taskmenu-layout="mobile"] .header-action-menus>.git-status-pill{width:100%}
  html[data-taskmenu-layout="mobile"] .header-action-menus>.taskmenu-menu>.taskmenu-menu-trigger,html[data-taskmenu-layout="mobile"] .header-action-menus>.taskmenu-direct-action{width:100%;text-align:left;min-height:44px}
  html[data-taskmenu-layout="mobile"] .header-action-menus .taskmenu-menu-popover{position:static!important;display:none;width:100%;max-height:none;box-shadow:none;margin-top:4px}
  html[data-taskmenu-layout="mobile"] .header-action-menus .taskmenu-menu.open>.taskmenu-menu-popover{display:flex}
  html[data-taskmenu-layout="mobile"] .mobile-backdrop{display:none;position:fixed;inset:0;z-index:1400;background:rgba(0,0,0,.5)}
  html[data-taskmenu-layout="mobile"] body.mobile-drawer-open .mobile-backdrop,html[data-taskmenu-layout="mobile"] body.mobile-actions-open .mobile-backdrop{display:block}
  html[data-taskmenu-layout="mobile"] .mobile-terminal-keys{display:flex;flex:0 0 auto;gap:4px;overflow-x:auto;padding:4px 5px;border-bottom:1px solid #30343b;background:#0d1117;-webkit-overflow-scrolling:touch}
  html[data-taskmenu-layout="mobile"] .mobile-terminal-keys button{flex:0 0 auto;min-width:46px;min-height:38px;padding:5px 8px;font:12px ui-monospace,monospace}
  html[data-taskmenu-layout="mobile"][data-taskmenu-theme="light"] body>header,html[data-taskmenu-layout="mobile"][data-taskmenu-theme="light"] body>main>aside{background:#f5f7fa}
  html[data-taskmenu-layout="mobile"][data-taskmenu-theme="light"] .header-action-menus{background:#fff;border-color:#b9c0c8}
  html[data-taskmenu-layout="mobile"][data-taskmenu-theme="light"] .mobile-terminal-keys{background:#f6f8fa;border-color:#d0d7de}
  /* The mobile drawer replaces the desktop-only Activity Bar; all views stay reachable. */
  html[data-taskmenu-layout="mobile"] body.mobile-drawer-open>main>aside{z-index:2150}
  html[data-taskmenu-layout="mobile"] .mobile-workspace-tools{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:6px;padding:7px 3px 11px;margin-bottom:8px;border-bottom:1px solid #384452}
  html[data-taskmenu-layout="mobile"] .mobile-workspace-tools-title{grid-column:1/-1;font-size:11px;font-weight:700;letter-spacing:.05em;opacity:.78}
  html[data-taskmenu-layout="mobile"] .mobile-workspace-tool{display:flex;align-items:center;min-width:0;gap:7px;text-align:left;white-space:normal;overflow-wrap:anywhere;padding:9px 7px;min-height:44px;border-radius:6px;font-size:12px}
  html[data-taskmenu-layout="mobile"] .mobile-workspace-tool-icon{flex:0 0 21px;text-align:center;font-size:15px}
  html[data-taskmenu-layout="mobile"] .mobile-workspace-tool[hidden]{display:none!important}
  html[data-taskmenu-layout="mobile"] .task-patch-panel{inset:0!important;width:100%!important;max-width:100%!important;min-width:0!important;overflow:hidden}
  html[data-taskmenu-layout="mobile"] .task-patch-panel-body{box-sizing:border-box;max-width:100%;padding:9px 9px 45px!important;overscroll-behavior:contain;-webkit-overflow-scrolling:touch}
  html[data-taskmenu-layout="mobile"] .task-patch-panel button,html[data-taskmenu-layout="mobile"] .task-patch-panel select{min-height:40px;white-space:normal}
  html[data-taskmenu-layout="mobile"] .task-patch-actions{display:grid!important;grid-template-columns:repeat(2,minmax(0,1fr));gap:6px!important}
  html[data-taskmenu-layout="mobile"] .task-patch-actions>*{min-width:0}
  html[data-taskmenu-layout="mobile"] .task-patch-summary-list,html[data-taskmenu-layout="mobile"] .task-patch-prompt-items{grid-template-columns:minmax(0,1fr)!important}
  html[data-taskmenu-layout="mobile"] .task-patch-prompt-tools,html[data-taskmenu-layout="mobile"] .task-patch-prompt-buttons,html[data-taskmenu-layout="mobile"] .task-patch-ai-pack-actions,html[data-taskmenu-layout="mobile"] .task-patch-history-tools,html[data-taskmenu-layout="mobile"] .task-patch-history-head{flex-wrap:wrap}
  html[data-taskmenu-layout="mobile"] .task-patch-history-run-row{grid-template-columns:minmax(0,1fr)}
  html[data-taskmenu-layout="mobile"] .task-patch-history-run-actions{justify-content:flex-start}
  html[data-taskmenu-layout="mobile"] .task-patch-history-variant{grid-template-columns:minmax(0,1fr);gap:4px}
  html[data-taskmenu-layout="mobile"] .task-patch-history-file-actions{justify-content:flex-start}
  html[data-taskmenu-layout="mobile"] .task-patch-ai-pack-rules textarea{max-width:100%;box-sizing:border-box}
  html[data-taskmenu-layout="mobile"] .task-patch-paste-dialog{max-width:calc(100vw - 16px);box-sizing:border-box}
  html[data-taskmenu-layout="mobile"] .task-connections-panel,
  html[data-taskmenu-layout="mobile"] body:not(.task-sidebar-auto-hide) .task-connections-panel{top:calc(50px + env(safe-area-inset-top))!important;bottom:env(safe-area-inset-bottom)!important;left:0!important;right:0!important;width:100vw!important;max-width:100%!important;box-sizing:border-box;z-index:1850}
  html[data-taskmenu-layout="mobile"] .task-connections-head{height:auto;min-height:46px;flex-wrap:wrap}
  html[data-taskmenu-layout="mobile"] .task-connection-form{grid-template-columns:minmax(0,1fr)}
  html[data-taskmenu-layout="mobile"] .task-connection-dialog{padding:8px;box-sizing:border-box}
  html[data-taskmenu-layout="mobile"] .task-connection-dialog-card{max-width:100%;max-height:calc(100dvh - 35px);box-sizing:border-box}
  html[data-taskmenu-layout="mobile"] .project-explorer{top:calc(50px + env(safe-area-inset-top))!important;bottom:env(safe-area-inset-bottom)!important;left:0!important;width:min(100vw,420px)!important;max-width:100%!important;z-index:1850}
  html[data-taskmenu-layout="mobile"] .schedule-nav{max-width:100vw;box-sizing:border-box}
  html[data-taskmenu-layout="mobile"] .schedule-toolbar{flex-wrap:wrap}
  html[data-taskmenu-layout="mobile"] .schedule-layout{min-width:0}
  html[data-taskmenu-layout="mobile"] .schedule-form{padding:8px;max-height:none}
  html[data-taskmenu-layout="mobile"] .schedule-repo-combo{grid-template-columns:minmax(0,1fr) minmax(0,1fr) 36px}
  html[data-taskmenu-layout="mobile"] .schedule-console{overflow:auto;max-width:100%;box-sizing:border-box}
  html[data-taskmenu-layout="mobile"][data-taskmenu-theme="light"] .mobile-workspace-tools{border-color:#c6d0db}
  html[data-taskmenu-layout="mobile"] .git-panel,html[data-taskmenu-layout="mobile"] .git-panel.git-panel-wide,html[data-taskmenu-layout="mobile"] body.task-sidebar-auto-hide .git-panel{left:4px!important;right:4px!important;width:calc(100vw - 8px)!important;max-width:calc(100vw - 8px)!important;top:calc(50px + env(safe-area-inset-top))!important;bottom:env(safe-area-inset-bottom)!important;box-sizing:border-box}
  html[data-taskmenu-layout="mobile"] .git-panel-head,html[data-taskmenu-layout="mobile"] .git-repo-bar{flex-wrap:wrap}
  html[data-taskmenu-layout="mobile"] .git-repo-select{min-width:0;max-width:100%;flex:1}
  html[data-taskmenu-layout="mobile"] .git-row{grid-template-columns:auto minmax(0,1fr)}
  html[data-taskmenu-layout="mobile"] .git-row-actions{grid-column:1/-1;justify-content:flex-start}
  html[data-taskmenu-layout="mobile"] .operation-center-backdrop,html[data-taskmenu-layout="mobile"] .project-health-backdrop{padding:calc(5px + env(safe-area-inset-top)) 5px calc(5px + env(safe-area-inset-bottom))}
  html[data-taskmenu-layout="mobile"] .operation-center-dialog,html[data-taskmenu-layout="mobile"] .project-health-dialog{width:100%;max-height:98dvh;min-width:0}
  html[data-taskmenu-layout="mobile"] .operation-center-head{flex-wrap:wrap}
  html[data-taskmenu-layout="mobile"] .operation-center-row{grid-template-columns:minmax(0,1fr) auto!important;gap:6px}
  html[data-taskmenu-layout="mobile"] .operation-center-main,html[data-taskmenu-layout="mobile"] .operation-center-error{grid-column:1/-1;white-space:normal;overflow-wrap:anywhere}
  html[data-taskmenu-layout="mobile"] .operation-center-actions{grid-column:1/-1;justify-content:flex-start}
  html[data-taskmenu-layout="mobile"] .workspace-snapshot-backdrop,html[data-taskmenu-layout="mobile"] .project-profile-backdrop{padding:5px}
  html[data-taskmenu-layout="mobile"] .workspace-snapshot-dialog,html[data-taskmenu-layout="mobile"] .project-profile-dialog{width:100%;max-height:calc(100dvh - 10px);box-sizing:border-box}
  html[data-taskmenu-layout="mobile"] .workspace-snapshot-row{grid-template-columns:minmax(0,1fr)}
  html[data-taskmenu-layout="mobile"] .workspace-snapshot-row-actions{justify-content:flex-start}
  html[data-taskmenu-layout="mobile"] .workspace-snapshot-actions,html[data-taskmenu-layout="mobile"] .project-profile-actions{flex-wrap:wrap}
  html[data-taskmenu-layout="mobile"] .schedule-nav,html[data-taskmenu-layout="mobile"] body.task-sidebar-auto-hide .schedule-nav{top:calc(50px + env(safe-area-inset-top))!important;left:0!important;bottom:env(safe-area-inset-bottom)!important;width:min(100vw,420px)!important;max-width:100%!important}
  html[data-taskmenu-layout="mobile"] .tab-context-menu,html[data-taskmenu-layout="mobile"] .tab-context-submenu{max-width:calc(100vw - 12px);max-height:calc(100dvh - 64px);box-sizing:border-box}


  `;
  document.head.append(style);

  const header=document.querySelector('header');
  const menu=document.querySelector('#menu');
  const backdrop=document.createElement('div');
  backdrop.className='mobile-backdrop';
  document.body.append(backdrop);

  const menuButton=document.createElement('button');
  menuButton.type='button';
  menuButton.className='mobile-header-button mobile-menu-button';
  menuButton.textContent='☰';
  menuButton.title='Open task menu';
  menuButton.setAttribute('aria-label','Open task menu');

  const toolsButton=document.createElement('button');
  toolsButton.type='button';
  toolsButton.className='mobile-header-button mobile-tools-button';
  toolsButton.textContent='Công cụ';
  toolsButton.title='Mở các chức năng TaskDeck trên mobile';
  toolsButton.setAttribute('aria-label','Công cụ TaskDeck');
  toolsButton.setAttribute('aria-controls','mobile-workspace-tools');
  toolsButton.setAttribute('aria-expanded','false');

  const actionsButton=document.createElement('button');
  actionsButton.type='button';
  actionsButton.className='mobile-header-button mobile-actions-button';
  actionsButton.textContent='⋮';
  actionsButton.title='Open actions';
  actionsButton.setAttribute('aria-label','Open actions');

  header?.prepend(menuButton,toolsButton);
  header?.append(actionsButton);

  function closePanels(){
    document.body.classList.remove('mobile-drawer-open','mobile-actions-open');
    toolsButton.setAttribute('aria-expanded','false');
    menuButton.setAttribute('aria-expanded','false');
    actionsButton.setAttribute('aria-expanded','false');
  }
  function toggleDrawer(){
    const open=!document.body.classList.contains('mobile-drawer-open');
    closePanels();
    if(open)document.body.classList.add('mobile-drawer-open');
    menuButton.setAttribute('aria-expanded',String(open));
  }
  function toggleTools(){
    const open=!document.body.classList.contains('mobile-drawer-open')||toolsButton.getAttribute('aria-expanded')!=='true';
    closePanels();
    if(open){
      restoreMobileNavigation();
      document.body.classList.add('mobile-drawer-open');
      toolsButton.setAttribute('aria-expanded','true');
      menu?.scrollTo?.({top:0,behavior:'instant'});
    }
  }
  function toggleActions(){
    const open=!document.body.classList.contains('mobile-actions-open');
    closePanels();
    if(open)document.body.classList.add('mobile-actions-open');
    actionsButton.setAttribute('aria-expanded',String(open));
  }
  // Feature modules load after mobile.js. Resolve them lazily at click time
  // instead of duplicating desktop logic or silently dropping mobile access.
  const toolsNav=document.createElement('nav');
  toolsNav.className='mobile-workspace-tools';
  toolsNav.id='mobile-workspace-tools';
  toolsNav.setAttribute('data-taskmenu-persistent','1');
  toolsNav.setAttribute('aria-label','Workspace views and tools');
  const toolsTitle=document.createElement('div');toolsTitle.className='mobile-workspace-tools-title';toolsTitle.textContent='WORKSPACE · TOOLS';
  toolsNav.append(toolsTitle);
  const can=permission=>!app.sharedMode||Boolean(app.hasPermission?.('project.admin')||app.hasPermission?.(permission));
  const canAny=(...permissions)=>permissions.some(can);
  function openMobileTool(name){
    const api={
      tasks:()=>{document.body.classList.add('mobile-drawer-open');return true;},
      explorer:()=>globalThis.TaskMenuExplorer?.open?.(),
      connections:()=>globalThis.TaskMenuConnections?.open?.(),
      patch:()=>globalThis.TaskMenuPatchPanel?.open?.(),
      schedules:()=>globalThis.TaskMenuSchedules?.open?.('','overview'),
      'schedule-list':()=>globalThis.TaskMenuSchedules?.openPanel?.(),
      history:()=>{const section=menu?.querySelector('.history-section');if(!section)throw new Error('Task history is not available');section.scrollIntoView({block:'start',behavior:'smooth'});document.body.classList.add('mobile-drawer-open');return true;},
      git:()=>globalThis.TaskMenuGitFiles?.open?.(),
      search:()=>globalThis.TaskMenuProjectSearch?.open?.(),
      remote:()=>globalThis.TaskMenuRemoteWorkspace?.open?.(),
      operations:()=>globalThis.TaskMenuOperationCenter?.open?.(),
      snapshots:()=>globalThis.TaskMenuWorkspaceSnapshots?.open?.(),
      health:()=>globalThis.TaskMenuProjectHealth?.open?.(),
      profiles:()=>globalThis.TaskMenuProjectProfiles?.open?.()
    };
    const entry=mobileViews.find(item=>item[0]===name);
    if(entry&&!entry[3]())throw new Error("You do not have permission to open "+name);
    const handler=api[name];
    if(!handler)throw new Error('Unknown mobile workspace tool: '+name);
    // Fixed desktop overlays have higher stacking contexts than the workspace
    // panes. Dismiss the previous overlay before navigating to another tool.
    // Otherwise a successful Patch/Schedule open may look like a no-op.
    if(name!=='connections')globalThis.TaskMenuConnections?.close?.();
    if(name!=='explorer')globalThis.TaskMenuExplorer?.close?.();
    if(name!=='schedule-list')globalThis.TaskMenuSchedules?.closePanel?.();
    if(name!=='git')globalThis.TaskMenuGitFiles?.close?.();
    if(name!=='tasks'&&name!=='history')closePanels();
    const result=handler();
    if(result===undefined&&['patch','connections','schedules','schedule-list','git','explorer'].includes(name)){
      const available={patch:globalThis.TaskMenuPatchPanel,connections:globalThis.TaskMenuConnections,
        schedules:globalThis.TaskMenuSchedules,'schedule-list':globalThis.TaskMenuSchedules,
        git:globalThis.TaskMenuGitFiles,explorer:globalThis.TaskMenuExplorer};
      if(!available[name])throw new Error(name+' is not initialized for this workspace');
    }
    return result;
  }
  const mobileViews=[
    ['tasks','☷','Tasks',()=>true],
    ['explorer','▤','Explorer',()=>can('files.read')],
    ['connections','⌁','Connections',()=>canAny('settings.read','ssh.use','db.read','transfer.read')],
    ['patch','▣','Patch Tool',()=>canAny('patch.view','patch.run')],
    ['schedules','▦','Schedules',()=>canAny('schedules.view','schedules.manage','schedules.run')],
    ['schedule-list','☷','Schedule list',()=>canAny('schedules.view','schedules.manage','schedules.run')],
    ['history','◷','Task history',()=>true],
    ['git','⑂','Git repositories',()=>can('git.status')],
    ['search','⌕','Search files',()=>can('files.read')],
    ['remote','⇄','Remote workspace',()=>can('ssh.use')&&can('settings.read')],
    ['operations','⇅','Operations',()=>true],
    ['snapshots','◫','Snapshots',()=>true],
    ['health','♡','Project health',()=>true],
    ['profiles','▥','Project profiles',()=>can('settings.read')]
  ];
  const mobileButtons=[];
  for(const [name,symbol,title,allowed] of mobileViews){
    const button=document.createElement('button');button.type='button';
    button.className='mobile-workspace-tool';button.dataset.mobileTool=name;
    const icon=document.createElement('span');icon.className='mobile-workspace-tool-icon';icon.textContent=symbol;
    const label=document.createElement('span');label.textContent=title;
    button.append(icon,label);
    button.onclick=()=>{try{Promise.resolve(openMobileTool(name)).catch(app.showError);}catch(error){app.showError(error);}};
    button.hidden=!allowed();mobileButtons.push([button,allowed]);toolsNav.append(button);
  }
  function restoreMobileNavigation(){
    if(menu&&menu.firstElementChild!==toolsNav)menu.prepend(toolsNav);
  }
  restoreMobileNavigation();
  // The task tree may be rebuilt from a server refresh, or replaced with a
  // permissions notice. Reattach the same nav node without duplicating events.
  const navigationObserver=new MutationObserver(()=>{
    if(menu&&!menu.contains(toolsNav))restoreMobileNavigation();
  });
  if(menu)navigationObserver.observe(menu,{childList:true});
  window.addEventListener('taskmenu:tasks',()=>{
    restoreMobileNavigation();
    for(const [button,allowed] of mobileButtons)button.hidden=!allowed();
  });
  // Available to mobile-only deep links and for UI regression tests.
  globalThis.TaskMenuMobileTools={open:openMobileTool,get menu(){return toolsNav;}};

  menuButton.onclick=toggleDrawer;
  toolsButton.onclick=toggleTools;
  actionsButton.onclick=toggleActions;
  backdrop.onclick=closePanels;

  menu?.addEventListener('click',event=>{
    const target=event.target instanceof Element?event.target:null;
    if(target?.closest('.task,.quick-task-run,.task-search-result'))setTimeout(closePanels,0);
  });
  document.querySelector('.header-action-menus')?.addEventListener('click',event=>{
    const target=event.target instanceof Element?event.target:null;
    if(target?.closest('.taskmenu-direct-action'))setTimeout(closePanels,0);
  });
  document.addEventListener('keydown',event=>{if(event.key==='Escape')closePanels();});

  function updateVisualViewport(){
    const height=window.visualViewport?.height||window.innerHeight;
    if(Number.isFinite(height)&&height>0)document.documentElement.style.setProperty('--taskmenu-mobile-height',height+'px');
    for(const view of app.views.values())setTimeout(()=>{try{view.fit.fit();}catch{}},0);
  }
  window.visualViewport?.addEventListener('resize',updateVisualViewport);
  window.visualViewport?.addEventListener('scroll',updateVisualViewport);
  window.addEventListener('orientationchange',()=>setTimeout(updateVisualViewport,80));
  updateVisualViewport();

  const keys=[
    ['Esc','\x1b'],['Tab','\t'],['Ctrl+C','\x03'],
    ['←','\x1b[D'],['↑','\x1b[A'],['↓','\x1b[B'],['→','\x1b[C'],['Enter','\r']
  ];
  const installed=new WeakSet();
  function installTerminalKeys(view){
    if(!view?.pane||installed.has(view))return;
    installed.add(view);
    const bar=document.createElement('div');bar.className='mobile-terminal-keys';
    for(const [label,data] of keys){
      const button=document.createElement('button');button.type='button';button.textContent=label;
      button.onclick=()=>{
        if(app.browserLeaseLost)return;
        if(view.ws&&view.ws.readyState===WebSocket.OPEN)view.ws.send(data);
        try{view.term.focus();}catch{}
      };
      bar.append(button);
    }
    const terminal=view.pane.querySelector('.terminal');
    if(terminal)terminal.before(bar);else view.pane.append(bar);
    setTimeout(()=>{try{view.fit.fit();}catch{}},0);
  }
  window.addEventListener('taskmenu:session',event=>installTerminalKeys(event.detail?.view));
  for(const view of app.views.values())installTerminalKeys(view);

  globalThis.TaskMenuMobile={closePanels,updateVisualViewport};
}
