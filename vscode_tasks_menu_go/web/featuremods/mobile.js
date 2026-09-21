const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for mobile UI');

if(app.layoutProfile==='mobile'){
  const style=document.createElement('style');
  style.dataset.taskmenuMobile='1';
  style.textContent=`
  html[data-taskmenu-layout="mobile"]{--taskmenu-mobile-height:100dvh}
  html[data-taskmenu-layout="mobile"] body{height:var(--taskmenu-mobile-height,100dvh)!important;max-height:var(--taskmenu-mobile-height,100dvh)!important;padding-top:env(safe-area-inset-top);padding-bottom:env(safe-area-inset-bottom)}
  html[data-taskmenu-layout="mobile"] body>header{height:50px!important;min-height:50px!important;max-height:50px!important;padding:0 8px!important;gap:8px!important;z-index:1300;background:#101216}
  html[data-taskmenu-layout="mobile"] body.mobile-actions-open>header{z-index:1600}
  html[data-taskmenu-layout="mobile"] body>header>strong{font-size:13px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:42vw}
  html[data-taskmenu-layout="mobile"] body>header>#workspace{display:none}
  html[data-taskmenu-layout="mobile"] body>main{display:grid!important;grid-template-columns:minmax(0,1fr)!important;max-height:calc(100% - 50px)!important}
  html[data-taskmenu-layout="mobile"] body>main>aside{position:fixed;z-index:1500;top:calc(50px + env(safe-area-inset-top));bottom:env(safe-area-inset-bottom);left:0;width:min(88vw,360px);max-width:360px;background:#101216;border-right:1px solid #30343b;box-shadow:8px 0 28px rgba(0,0,0,.45);transform:translateX(-105%);transition:transform .16s ease;padding:10px 10px 18px}
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
  html[data-taskmenu-layout="mobile"] .mobile-actions-button{margin-left:auto}
  html[data-taskmenu-layout="mobile"] .header-action-menus{display:none;position:fixed;z-index:1550;top:calc(54px + env(safe-area-inset-top));right:8px;width:min(92vw,360px);max-height:calc(100dvh - 78px);overflow:auto;padding:8px;border:1px solid #3b414d;border-radius:10px;background:#171a20;box-shadow:0 12px 34px rgba(0,0,0,.5);flex-direction:column;align-items:stretch}
  html[data-taskmenu-layout="mobile"] body.mobile-actions-open .header-action-menus{display:flex}
  html[data-taskmenu-layout="mobile"] .header-action-menus>.taskmenu-menu,html[data-taskmenu-layout="mobile"] .header-action-menus>.git-status-pill{width:100%}
  html[data-taskmenu-layout="mobile"] .header-action-menus>.taskmenu-menu>.taskmenu-menu-trigger{width:100%;text-align:left;min-height:44px}
  html[data-taskmenu-layout="mobile"] .header-action-menus .taskmenu-menu-popover{position:static!important;display:none;width:100%;max-height:none;box-shadow:none;margin-top:4px}
  html[data-taskmenu-layout="mobile"] .header-action-menus .taskmenu-menu.open>.taskmenu-menu-popover{display:flex}
  html[data-taskmenu-layout="mobile"] .mobile-backdrop{display:none;position:fixed;inset:0;z-index:1400;background:rgba(0,0,0,.5)}
  html[data-taskmenu-layout="mobile"] body.mobile-drawer-open .mobile-backdrop,html[data-taskmenu-layout="mobile"] body.mobile-actions-open .mobile-backdrop{display:block}
  html[data-taskmenu-layout="mobile"] .mobile-terminal-keys{display:flex;flex:0 0 auto;gap:4px;overflow-x:auto;padding:4px 5px;border-bottom:1px solid #30343b;background:#0d1117;-webkit-overflow-scrolling:touch}
  html[data-taskmenu-layout="mobile"] .mobile-terminal-keys button{flex:0 0 auto;min-width:46px;min-height:38px;padding:5px 8px;font:12px ui-monospace,monospace}
  html[data-taskmenu-layout="mobile"][data-taskmenu-theme="light"] body>header,html[data-taskmenu-layout="mobile"][data-taskmenu-theme="light"] body>main>aside{background:#f5f7fa}
  html[data-taskmenu-layout="mobile"][data-taskmenu-theme="light"] .header-action-menus{background:#fff;border-color:#b9c0c8}
  html[data-taskmenu-layout="mobile"][data-taskmenu-theme="light"] .mobile-terminal-keys{background:#f6f8fa;border-color:#d0d7de}
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

  const actionsButton=document.createElement('button');
  actionsButton.type='button';
  actionsButton.className='mobile-header-button mobile-actions-button';
  actionsButton.textContent='⋮';
  actionsButton.title='Open actions';
  actionsButton.setAttribute('aria-label','Open actions');

  header?.prepend(menuButton);
  header?.append(actionsButton);

  function closePanels(){
    document.body.classList.remove('mobile-drawer-open','mobile-actions-open');
  }
  function toggleDrawer(){
    const open=!document.body.classList.contains('mobile-drawer-open');
    closePanels();
    if(open)document.body.classList.add('mobile-drawer-open');
  }
  function toggleActions(){
    const open=!document.body.classList.contains('mobile-actions-open');
    closePanels();
    if(open)document.body.classList.add('mobile-actions-open');
  }
  menuButton.onclick=toggleDrawer;
  actionsButton.onclick=toggleActions;
  backdrop.onclick=closePanels;

  menu?.addEventListener('click',event=>{
    const target=event.target instanceof Element?event.target:null;
    if(target?.closest('.task,.quick-task-run,.task-search-result'))setTimeout(closePanels,0);
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
