const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for grouped menus');

const style=document.createElement('style');
style.textContent=`
.taskmenu-menu{position:relative;display:inline-flex;align-items:center}.taskmenu-menu-trigger{white-space:nowrap;padding:6px 9px}.taskmenu-menu-popover{display:none;position:absolute;top:calc(100% + 6px);right:0;z-index:1200;min-width:220px;max-width:min(440px,90vw);padding:8px;border:1px solid #3b414d;border-radius:8px;background:#171a20;box-shadow:0 10px 28px rgba(0,0,0,.35);gap:6px}.taskmenu-menu.open>.taskmenu-menu-popover{display:flex;flex-direction:column}.taskmenu-menu-popover>button,.taskmenu-menu-popover>select{width:100%;text-align:left}.taskmenu-menu-popover>.copy-console{margin-left:0;max-width:100%}.taskmenu-menu-popover>.appearance-controls{display:grid;grid-template-columns:1fr 1fr auto auto auto;gap:5px;align-items:center}.taskmenu-menu-popover .appearance-controls select,.taskmenu-menu-popover .appearance-controls button{width:auto}.taskmenu-menu-label{font-size:10px;font-weight:700;letter-spacing:.06em;opacity:.55;padding:2px 3px 0}.header-action-menus{display:flex;align-items:center;gap:6px;white-space:nowrap}.header-action-menus .git-status-pill{margin:0}.pane-action-menus{display:flex;align-items:center;gap:5px;white-space:nowrap;margin-left:auto}.pane-action-menus .taskmenu-menu-popover{top:calc(100% + 4px)}.pane-action-menus .taskmenu-menu-trigger{padding:5px 8px;font-size:11px}.pane-action-menus .taskmenu-menu-popover button{font-size:11px;padding:5px 8px}.pane-action-menus .taskmenu-menu-popover .stop{background:#3b2528;border-color:#684047}
.pane-action-menus .taskmenu-menu-popover .copy-console{background:#203b58;border-color:#35648d;color:#d9ecff}.pane-action-menus .taskmenu-menu-popover .console-search-btn{background:#332a55;border-color:#594a8e;color:#eee7ff}.pane-action-menus .taskmenu-menu-popover .console-save-btn{background:#203f31;border-color:#3a7058;color:#dcf6e7}.pane-action-menus .taskmenu-menu-popover .session-clear-console{background:#4a252a;border-color:#7a4048;color:#ffe2e4}
html[data-taskmenu-theme="light"] .taskmenu-menu-popover{background:#fff;border-color:#b9c0c8;box-shadow:0 10px 28px rgba(0,0,0,.15)}
html[data-taskmenu-theme="light"] .pane-action-menus .taskmenu-menu-popover .copy-console{background:#e8f2ff;border-color:#8db7df;color:#194b78}html[data-taskmenu-theme="light"] .pane-action-menus .taskmenu-menu-popover .console-search-btn{background:#f0ebff;border-color:#afa1da;color:#493b78}html[data-taskmenu-theme="light"] .pane-action-menus .taskmenu-menu-popover .console-save-btn{background:#e9f7ef;border-color:#87bf9f;color:#23583b}html[data-taskmenu-theme="light"] .pane-action-menus .taskmenu-menu-popover .session-clear-console{background:#fff0f1;border-color:#d7989e;color:#7b3037}
`;
document.head.append(style);

const openMenus=new Set();
function closeMenu(menu){menu.classList.remove('open');openMenus.delete(menu);}
function closeAll(except=null){for(const menu of [...openMenus])if(menu!==except)closeMenu(menu);}
function makeMenu(label,title){
  const menu=document.createElement('div');menu.className='taskmenu-menu';
  const trigger=document.createElement('button');trigger.className='taskmenu-menu-trigger';trigger.textContent=label+' ▾';trigger.title=title||label;
  const pop=document.createElement('div');pop.className='taskmenu-menu-popover';
  trigger.onclick=event=>{event.stopPropagation();const opening=!menu.classList.contains('open');closeAll(menu);menu.classList.toggle('open',opening);if(opening)openMenus.add(menu);else openMenus.delete(menu);};
  pop.onclick=event=>event.stopPropagation();menu.append(trigger,pop);return {menu,trigger,pop};
}
function addSection(pop,label,nodes){
  const available=nodes.filter(Boolean);if(!available.length)return false;
  const head=document.createElement('div');head.className='taskmenu-menu-label';head.textContent=label;pop.append(head,...available);return true;
}

document.addEventListener('click',()=>closeAll());
document.addEventListener('keydown',event=>{if(event.key==='Escape')closeAll();});

function installHeaderMenus(){
  const header=document.querySelector('header'),workspace=document.querySelector('#workspace');if(!header||!workspace||header.querySelector('.header-action-menus'))return;
  const host=document.createElement('div');host.className='header-action-menus';header.append(host);

  const files=makeMenu('Files','Project files');
  const quickOpen=document.createElement('button');quickOpen.type='button';quickOpen.textContent='Quick Open…  Ctrl+P';quickOpen.onclick=()=>{closeAll();globalThis.TaskMenuQuickOpen?.open();};
  const searchFiles=document.createElement('button');searchFiles.type='button';searchFiles.textContent='Search in Files…  Ctrl+Shift+F';searchFiles.onclick=()=>{closeAll();globalThis.TaskMenuProjectSearch?.open();};
  const explorer=document.createElement('button');explorer.type='button';explorer.textContent='Explorer';explorer.onclick=()=>{closeAll();globalThis.TaskMenuExplorer?.open();};
  addSection(files.pop,'OPEN',[quickOpen,searchFiles,explorer]);
  host.append(files.menu);

  const terminal=makeMenu('Terminal','Terminal actions');
  addSection(terminal.pop,'NEW TERMINAL',[document.querySelector('#terminal-cwd'),document.querySelector('#open-terminal')]);
  host.append(terminal.menu);

  const workspaceMenu=makeMenu('Workspace','Workspace actions');
  addSection(workspaceMenu.pop,'FILES & TASKS',[document.querySelector('#upload-workspace'),document.querySelector('#reload')]);
  addSection(workspaceMenu.pop,'PROJECT',[document.querySelector('#edit-title')]);
  host.append(workspaceMenu.menu);

  const settings=makeMenu('Settings','Tool settings');
  addSection(settings.pop,'ENVIRONMENT',[document.querySelector('#env-profile'),document.querySelector('#env-profile-manage')]);
  addSection(settings.pop,'NOTIFICATIONS',[document.querySelector('#notifications-toggle')]);
  addSection(settings.pop,'APPEARANCE',[document.querySelector('.appearance-controls'),document.querySelector('#self-update-check')]);
  host.append(settings.menu);

  const git=document.querySelector('.git-status-pill');if(git)host.append(git);
}

function paneMenu(view,label,selectorPairs){
  const made=makeMenu(label,label+' actions');let count=0;
  for(const [section,selectors] of selectorPairs){const nodes=selectors.map(selector=>view.pane.querySelector(selector)).filter(Boolean);if(addSection(made.pop,section,nodes))count+=nodes.length;}
  if(!count)return null;return made.menu;
}

function decoratePane(view){
  const head=view?.pane?.querySelector('.pane-head');if(!head||head.querySelector('.pane-action-menus'))return;
  const host=document.createElement('div');host.className='pane-action-menus';
  const session=paneMenu(view,'Session',[
    ['PROCESS',['.session-force-restart','.terminal-rename']],
    ['SPLIT',['.session-split-vertical','.session-split-horizontal','.session-merge-vertical','.session-merge-horizontal','.session-unsplit']],
    ['LIFECYCLE',['.stop']]
  ]);
  const consoleMenu=paneMenu(view,'Console',[
    ['CONSOLE',['.copy-console','.console-search-btn','.console-save-btn','.session-clear-console']]
  ]);
  if(session)host.append(session);if(consoleMenu)host.append(consoleMenu);
  if(host.childElementCount)head.append(host);
}

window.addEventListener('taskmenu:session',event=>{const view=event.detail?.view;if(view)setTimeout(()=>decoratePane(view),0);});
for(const view of app.views.values())decoratePane(view);
installHeaderMenus();
