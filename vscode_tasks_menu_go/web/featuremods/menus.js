const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for grouped menus');

const style=document.createElement('style');
style.textContent=`
.taskmenu-menu{position:relative;display:inline-flex;align-items:center}.taskmenu-menu-trigger{white-space:nowrap;padding:2px 4px;border:0!important;border-radius:4px;background:transparent!important;font-size:11px}.taskmenu-menu-trigger:hover,.taskmenu-menu.open>.taskmenu-menu-trigger{background:#242a34!important}.taskmenu-menu-popover{display:none;position:absolute;top:calc(100% + 4px);right:0;z-index:1200;min-width:220px;max-width:min(440px,90vw);padding:5px;border:1px solid #3b414d;border-radius:7px;background:#171a20;box-shadow:0 10px 28px rgba(0,0,0,.35);gap:2px}.taskmenu-menu.open>.taskmenu-menu-popover{display:flex;flex-direction:column}.taskmenu-menu-popover>button{width:100%;text-align:left;border:0;background:transparent;border-radius:4px;padding:5px 7px}.taskmenu-menu-popover>button:hover:not(:disabled){background:#2b3440}.taskmenu-menu-popover>select{width:100%;text-align:left;padding:5px 7px}.taskmenu-menu-popover>.copy-console{margin-left:0;max-width:100%}.taskmenu-menu-popover>.appearance-controls{display:grid;grid-template-columns:1fr 1fr auto auto auto;gap:5px;align-items:center}.taskmenu-menu-popover .appearance-controls select,.taskmenu-menu-popover .appearance-controls button{width:auto}.taskmenu-menu-label{font-size:9px;font-weight:700;letter-spacing:.06em;opacity:.5;padding:3px 4px 1px}.self-update-validation-settings{display:flex;align-items:center;gap:6px;padding:5px 7px;font-size:11px}.self-update-validation-settings input{margin:0}.patch-ui-mode-control{display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:center;gap:8px;font-size:11px}.patch-ui-mode-control select{min-width:145px}.header-action-menus{display:flex;align-items:center;gap:1px;white-space:nowrap;margin-left:auto}.header-left-git,.header-action-menus .git-status-pill{margin:0;border:0!important;background:transparent!important;border-radius:4px;padding:2px 4px;font-size:10px;flex:0 0 auto}.header-left-git:hover,.header-action-menus .git-status-pill:hover{background:#242a34!important}.header-left-git.dirty,.header-action-menus .git-status-pill.dirty{color:#f0c66b}.header-left-git.clean,.header-action-menus .git-status-pill.clean{color:#83d69a}.pane-action-menus{display:flex;align-items:center;gap:1px;white-space:nowrap;margin-left:auto}.pane-action-menus .taskmenu-menu-popover{top:calc(100% + 2px)}.pane-action-menus .taskmenu-menu-trigger{padding:1px 4px;font-size:10px}.pane-action-menus .taskmenu-menu-popover button{font-size:11px;padding:4px 7px;border:0;background:transparent}.pane-action-menus .taskmenu-menu-popover .stop{color:#ffb2b7}.pane-action-menus .taskmenu-menu-popover .copy-console{color:#b9dcff}.pane-action-menus .taskmenu-menu-popover .console-search-btn{color:#d9cbff}.pane-action-menus .taskmenu-menu-popover .console-save-btn{color:#bfead0}.pane-action-menus .taskmenu-menu-popover .session-clear-console{color:#ffb8bd}
html[data-taskmenu-theme="light"] .taskmenu-menu-trigger:hover,html[data-taskmenu-theme="light"] .taskmenu-menu.open>.taskmenu-menu-trigger{background:#e8edf3!important}html[data-taskmenu-theme="light"] .taskmenu-menu-popover{background:#fff;border-color:#b9c0c8;box-shadow:0 10px 28px rgba(0,0,0,.15)}html[data-taskmenu-theme="light"] .taskmenu-menu-popover>button:hover:not(:disabled){background:#edf2f7}html[data-taskmenu-theme="light"] .pane-action-menus .taskmenu-menu-popover .copy-console{color:#194b78}html[data-taskmenu-theme="light"] .pane-action-menus .taskmenu-menu-popover .console-search-btn{color:#493b78}html[data-taskmenu-theme="light"] .pane-action-menus .taskmenu-menu-popover .console-save-btn{color:#23583b}html[data-taskmenu-theme="light"] .pane-action-menus .taskmenu-menu-popover .session-clear-console{color:#7b3037}
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
  const projectWorkspace=String(app.taskData?.workspace||'').trim();
  if(!projectWorkspace)return false;
  const header=document.querySelector('header'),workspace=document.querySelector('#workspace');if(!header||!workspace||header.querySelector('.header-action-menus'))return false;
  const host=document.createElement('div');host.className='header-action-menus';header.append(host);

  const files=makeMenu('Files','Project files');
  const quickOpen=document.createElement('button');quickOpen.type='button';quickOpen.textContent='Quick Open…  Ctrl+P';quickOpen.onclick=()=>{closeAll();globalThis.TaskMenuQuickOpen?.open();};
  const searchFiles=document.createElement('button');searchFiles.type='button';searchFiles.textContent='Search in Files…  Ctrl+Shift+F';searchFiles.onclick=()=>{closeAll();globalThis.TaskMenuProjectSearch?.open();};
  const explorer=document.createElement('button');explorer.type='button';explorer.textContent='Explorer';explorer.onclick=()=>{closeAll();globalThis.TaskMenuExplorer?.open();};
  addSection(files.pop,'OPEN',[quickOpen,searchFiles,explorer]);
  addSection(files.pop,'FILES',[document.querySelector('#upload-workspace')]);
  host.append(files.menu);

  const terminal=makeMenu('Terminal','Terminal actions');
  addSection(terminal.pop,'NEW TERMINAL',[document.querySelector('#terminal-cwd'),document.querySelector('#open-terminal')]);
  host.append(terminal.menu);

  const settings=makeMenu('Settings','Tool settings');
  const patchUIKey='vscode-tasks-menu:patch-ui-mode:'+projectWorkspace;
  const patchUIControl=document.createElement('label');patchUIControl.className='patch-ui-mode-control';
  const patchUILabel=document.createElement('span');patchUILabel.textContent='Interface';
  const patchUISelect=document.createElement('select');patchUISelect.id='patch-ui-mode';patchUISelect.title='Choose Patch Tool interface';
  for(const [value,label] of [['native','Native UI'],['terminal','Terminal (legacy)']]){
    const option=document.createElement('option');option.value=value;option.textContent=label;patchUISelect.append(option);
  }
  let patchUIMode='native';
  try{const saved=localStorage.getItem(patchUIKey);if(saved==='terminal')patchUIMode='terminal';}catch{}
  patchUISelect.value=patchUIMode;
  patchUISelect.onchange=()=>{
    patchUIMode=patchUISelect.value==='terminal'?'terminal':'native';
    try{localStorage.setItem(patchUIKey,patchUIMode);}catch(error){console.warn('Cannot persist Patch Tool UI mode',error);}
    window.dispatchEvent(new CustomEvent('taskmenu:patch-ui-mode',{detail:{mode:patchUIMode}}));
    closeAll();
  };
  patchUIControl.append(patchUILabel,patchUISelect);
  globalThis.TaskMenuPatchUISettings={get mode(){return patchUIMode;}};

  addSection(settings.pop,'PATCH TOOL',[patchUIControl]);
  addSection(settings.pop,'ENVIRONMENT',[document.querySelector('#env-profile'),document.querySelector('#env-profile-manage')]);
  addSection(settings.pop,'NOTIFICATIONS',[document.querySelector('#notifications-toggle')]);
  addSection(settings.pop,'APPEARANCE',[document.querySelector('.appearance-controls'),document.querySelector('#running-indicator-settings')]);
  addSection(settings.pop,'UPDATE',[document.querySelector('#self-update-validation-settings'),document.querySelector('#self-update-check')]);
  addSection(settings.pop,'WORKSPACE',[document.querySelector('#tasks-json-editor'),document.querySelector('#reload'),document.querySelector('#edit-title')]);
  host.append(settings.menu);

  const git=document.querySelector('.git-status-pill');
  if(git){
    if(app.layoutProfile==='mobile'){
      git.classList.remove('header-left-git');
      host.append(git);
    }else{
      git.classList.add('header-left-git');
      workspace.after(git);
    }
  }
  return true;
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
    ['SPLIT',['.session-split-vertical','.session-split-horizontal','.session-merge-vertical','.session-merge-horizontal','.session-swap-split','.session-unsplit']],
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
if(!installHeaderMenus()){
  window.addEventListener('taskmenu:tasks',()=>installHeaderMenus(),{once:true});
}
