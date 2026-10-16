const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for tab context menu');

const tabsHost=document.querySelector('#tabs');
if(!tabsHost)throw new Error('Tab host unavailable for tab context menu');

const style=document.createElement('style');
style.textContent=`
.tab-context-menu{position:fixed;z-index:4000;display:none;min-width:230px;max-width:min(360px,92vw);max-height:min(620px,86vh);overflow:auto;overscroll-behavior:contain;scrollbar-gutter:stable;padding:7px;border:1px solid #3b414d;border-radius:8px;background:#171a20;box-shadow:0 12px 34px rgba(0,0,0,.42)}
.tab-context-menu.open{display:block}
.tab-context-submenu{position:fixed;z-index:4100;display:none;min-width:300px;max-width:min(520px,92vw);max-height:min(620px,86vh);overflow:auto;overscroll-behavior:contain;scrollbar-gutter:stable;padding:7px;border:1px solid #3b414d;border-radius:8px;background:#171a20;box-shadow:0 12px 34px rgba(0,0,0,.42)}
.tab-context-submenu.open{display:block}
.tab-context-menu button,.tab-context-submenu button{display:block;width:100%;text-align:left;margin:0;border:0;background:transparent;border-radius:4px;padding:5px 7px;font-size:12px;color:inherit}
.tab-context-menu button:hover:not(:disabled),.tab-context-submenu button:hover:not(:disabled){background:#2b3440}
.tab-context-menu button.context-danger,.tab-context-submenu button.context-danger{color:#ffb2b7}
.tab-context-submenu-parent{display:flex!important;align-items:center;gap:8px}
.tab-context-submenu-parent .context-submenu-arrow{margin-left:auto;opacity:.7}
.tab-context-heading{padding:5px 7px 3px;font-size:10px;font-weight:800;letter-spacing:.08em;opacity:.55}
.tab-context-separator{height:1px;background:#30343b;margin:6px 3px}
.tab-context-menu button.context-check{display:flex;align-items:center;gap:8px}
.tab-context-menu button.context-check input{margin:0;pointer-events:none}
.pane-head>.terminal-context-trigger{flex:0 0 18px;width:18px;height:18px;min-width:18px;padding:0!important;display:inline-flex;align-items:center;justify-content:center;border:1px solid #48515f!important;border-radius:3px!important;background:#171a20!important;font:700 13px/1 ui-monospace,monospace!important;opacity:.78}
.pane-head>.terminal-context-trigger:hover:not(:disabled),.pane-head>.terminal-context-trigger[aria-expanded="true"]{background:#2b3440!important;border-color:#6b7788!important;opacity:1}
html[data-taskmenu-theme="light"] .pane-head>.terminal-context-trigger{background:#fff!important;border-color:#b9c0c8!important;color:#202124}
html[data-taskmenu-theme="light"] .pane-head>.terminal-context-trigger:hover:not(:disabled),html[data-taskmenu-theme="light"] .pane-head>.terminal-context-trigger[aria-expanded="true"]{background:#edf2f7!important}
.taskdeck-tab-readonly .taskdeck-readonly-lock{display:inline-flex;align-items:center;flex:0 0 auto;font-size:12px;margin-right:5px;opacity:.95;color:inherit}.taskdeck-readonly-lock[hidden]{display:none!important}
[data-taskdeck-readonly="1"]:not([data-taskdeck-readonly-kind="terminal"]) button,[data-taskdeck-readonly="1"]:not([data-taskdeck-readonly-kind="terminal"]) input,[data-taskdeck-readonly="1"]:not([data-taskdeck-readonly-kind="terminal"]) textarea,[data-taskdeck-readonly="1"]:not([data-taskdeck-readonly-kind="terminal"]) select{opacity:.55}
[data-taskdeck-readonly="1"]:not([data-taskdeck-readonly-kind="terminal"]) .cm-content{caret-color:transparent}
html[data-taskmenu-theme="light"] .tab-context-menu,html[data-taskmenu-theme="light"] .tab-context-submenu{background:#fff;border-color:#b9c0c8;box-shadow:0 12px 34px rgba(0,0,0,.18)}
html[data-taskmenu-theme="light"] .tab-context-menu button:hover:not(:disabled),html[data-taskmenu-theme="light"] .tab-context-submenu button:hover:not(:disabled){background:#edf2f7}
html[data-taskmenu-theme="light"] .tab-context-menu button.context-danger,html[data-taskmenu-theme="light"] .tab-context-submenu button.context-danger{color:#7b3037}
`;
document.head.append(style);

const menu=document.createElement('div');
menu.className='tab-context-menu';
menu.setAttribute('role','menu');
document.body.append(menu);

let contextView=null;
let submenu=null;
let submenuOwner=null;
let readonlyStorageName='';
let readonlyKeys=new Set();

function readOnlyStorageKey(){
  const workspace=String(app.taskData?.workspace||'').trim()||'global';
  return 'taskdeck:tab-readonly:'+workspace;
}

function ensureReadOnlyState(){
  const name=readOnlyStorageKey();
  if(name===readonlyStorageName)return;
  readonlyStorageName=name;
  try{
    const raw=JSON.parse(localStorage.getItem(name)||'[]');
    readonlyKeys=new Set(Array.isArray(raw)?raw.filter(value=>typeof value==='string'&&value):[]);
  }catch{readonlyKeys=new Set();}
}

function persistReadOnlyState(){
  ensureReadOnlyState();
  try{localStorage.setItem(readonlyStorageName,JSON.stringify([...readonlyKeys]));}
  catch(error){console.warn('Cannot persist tab read-only state',error);}
}

function descriptorForTab(tab){
  if(!(tab instanceof Element))return null;
  if(tab.classList.contains('task-patch-tab')){
    const patch=globalThis.TaskMenuPatchPanel;
    return {kind:'patch',key:'patch',tab,pane:patch?.panel||null,view:patch||null};
  }
  if(tab.classList.contains('db-tab')){
    const id=String(tab.dataset.id||'');
    const view=globalThis.TaskMenuDatabase?.views?.get?.(id)||null;
    return view?{kind:'database',key:'database:'+id,tab,pane:view.pane,view}:null;
  }
  const id=String(tab.dataset.id||'');
  if(!id)return null;
  const editor=globalThis.TaskMenuEditor?.editors?.get?.(id)||null;
  if(editor)return {kind:'editor',key:'editor:'+(editor.file?.path||id),tab,pane:editor.pane,view:editor};
  const terminal=app.views.get(id)||null;
  if(terminal)return {kind:'terminal',key:'terminal:'+id,tab,pane:terminal.pane,view:terminal};
  return null;
}

function descriptorReadOnly(descriptor){
  ensureReadOnlyState();
  return Boolean(descriptor?.key&&readonlyKeys.has(descriptor.key));
}

function applyDescriptorReadOnly(descriptor,enabled){
  if(!descriptor)return false;
  // A real tab child avoids ::before conflicts with tab type / split accents.
  const tab=descriptor.tab;
  if(tab){
    let lock=tab.querySelector(':scope > .taskdeck-readonly-lock');
    if(!lock){
      lock=document.createElement('span');
      lock.className='taskdeck-readonly-lock';
      lock.textContent='🔒';
      lock.title='Read only';
      lock.setAttribute('aria-label','Read only');
      tab.prepend(lock);
    }
    lock.hidden=!enabled;
  }
  const next=Boolean(enabled);
  descriptor.tab?.classList.toggle('taskdeck-tab-readonly',next);
  if(descriptor.pane){
    if(next){
      descriptor.pane.dataset.taskdeckReadonly='1';
      descriptor.pane.dataset.taskdeckReadonlyKind=descriptor.kind;
    }else{
      delete descriptor.pane.dataset.taskdeckReadonly;
      delete descriptor.pane.dataset.taskdeckReadonlyKind;
    }
    descriptor.pane.setAttribute('aria-readonly',next?'true':'false');
  }
  if(descriptor.kind==='terminal')app.setViewReadOnly?.(descriptor.view,next);
  else if(descriptor.kind==='editor')globalThis.TaskMenuEditor?.setTabReadOnly?.(descriptor.view,next);
  else if(descriptor.kind==='database')globalThis.TaskMenuDatabase?.setTabReadOnly?.(descriptor.view,next);
  else if(descriptor.kind==='patch')globalThis.TaskMenuPatchPanel?.setReadOnly?.(next);
  return next;
}

function setDescriptorReadOnly(descriptor,enabled){
  if(!descriptor?.key)return false;
  ensureReadOnlyState();
  const next=Boolean(enabled);
  if(next)readonlyKeys.add(descriptor.key);else readonlyKeys.delete(descriptor.key);
  persistReadOnlyState();
  applyDescriptorReadOnly(descriptor,next);
  window.dispatchEvent(new CustomEvent('taskmenu:tab-readonly-changed',{detail:{kind:descriptor.kind,key:descriptor.key,enabled:next}}));
  return next;
}

function decorateTerminalPane(view){
  const head=view?.pane?.querySelector('.pane-head');
  if(!head)return null;
  let trigger=head.querySelector(':scope > .terminal-context-trigger');
  if(trigger)return trigger;
  trigger=document.createElement('button');
  trigger.type='button';
  trigger.className='terminal-context-trigger';
  trigger.textContent='⋮';
  trigger.title='Open the same context menu as right-clicking this terminal tab';
  trigger.setAttribute('aria-label','Open terminal context menu');
  trigger.setAttribute('aria-haspopup','menu');
  trigger.setAttribute('aria-expanded','false');
  trigger.onclick=event=>{
    event.preventDefault();event.stopPropagation();
    const rect=trigger.getBoundingClientRect();
    openContextMenu(view,rect.left,rect.bottom+3);
    trigger.setAttribute('aria-expanded','true');
  };
  head.prepend(trigger);
  return trigger;
}

function registerTab(tab){
  setTimeout(()=>{
    const descriptor=descriptorForTab(tab);
    if(!descriptor)return;
    applyDescriptorReadOnly(descriptor,descriptorReadOnly(descriptor));
    if(descriptor.kind==='terminal')decorateTerminalPane(descriptor.view);
  },0);
}

function closeSubmenu(){
  if(submenu){submenu.remove();submenu=null;}
  submenuOwner=null;
}

function closeContextMenu(){
  closeSubmenu();
  menu.classList.remove('open');
  menu.replaceChildren();
  for(const trigger of document.querySelectorAll('.terminal-context-trigger[aria-expanded="true"]'))trigger.setAttribute('aria-expanded','false');
  contextView=null;
}

function findPaneMenu(view,label){
  for(const group of view?.pane?.querySelectorAll('.pane-action-menus .taskmenu-menu')||[]){
    const trigger=group.querySelector(':scope > .taskmenu-menu-trigger');
    if((trigger?.textContent||'').trim().startsWith(label))return group;
  }
  return null;
}

function semanticClass(button,label){
  if(label==='Console')return '';
  if(button.classList.contains('stop'))return 'context-danger';
  if(button.matches('.session-terminate,.session-force-kill'))return 'context-danger';
  return '';
}

function sourceActions(view,label){
  const group=findPaneMenu(view,label);
  if(!group)return [];
  const pop=group.querySelector(':scope > .taskmenu-menu-popover');
  if(!pop)return [];
  return [...pop.children].filter(node=>node instanceof HTMLButtonElement&&!node.hidden&&node.style.display!=='none');
}

function sourceAction(view,source,label){
  // Reuse the original toolbar button, including its live disabled/hidden
  // state and its existing confirmation, permission and mutation checks.
  return {
    label:(source.textContent||'').trim()||source.title||'Action',
    title:source.title||'',
    disabled:source.disabled,
    danger:semanticClass(source,label)==='context-danger',
    run:()=>setTimeout(()=>{
      if(!view.closed&&source.isConnected&&!source.disabled)source.click();
    },0)
  };
}
function groupSessionSources(view,sources){
  const groups={session:[],split:[],lifecycle:[]};
  for(const source of sources){
    const kind=source.matches('.session-split-vertical,.session-split-horizontal,.session-merge-vertical,.session-merge-horizontal,.session-move-split-group,.session-swap-split,.session-unsplit')
      ?'split':source.matches('.stop,.session-terminate,.session-force-kill')?'lifecycle':'session';
    groups[kind].push(sourceAction(view,source,'Session'));
  }
  return groups;
}
function closeTabAction(view){
  const close=view.tab?.querySelector('.close');
  return {label:'Close tab',title:'Close this tab using the normal TaskDeck tab-close lifecycle',
    disabled:!close,run:()=>{if(close?.isConnected)close.click();}};
}
function addHeading(label){
  const heading=document.createElement('div');
  heading.className='tab-context-heading';
  heading.textContent=label.toUpperCase();
  menu.append(heading);
}

function addReadOnlyToggle(descriptor){
  if(!descriptor)return false;
  addHeading('Tab');
  const item=document.createElement('button');item.type='button';item.className='context-check';
  const checkbox=document.createElement('input');checkbox.type='checkbox';checkbox.tabIndex=-1;checkbox.checked=descriptorReadOnly(descriptor);
  const label=document.createElement('span');label.textContent='Read only';
  item.append(checkbox,label);
  item.title='When enabled, content input is read-only; toolbar and utility actions stay available';
  item.onclick=event=>{
    event.preventDefault();event.stopPropagation();
    checkbox.checked=setDescriptorReadOnly(descriptor,!checkbox.checked);
  };
  menu.append(item);
  return true;
}

function styleCustomItem(item,action){
  item.title=action.title||item.textContent;
  item.disabled=Boolean(action.disabled);
  if(action.danger)item.classList.add('context-danger');
  if(action.preset){
    item.style.background=action.preset.bg||'';
    item.style.color=action.preset.fg||'';
    item.style.borderColor=action.preset.fg||'';
  }
}

function runCustomAction(action,item){
  const targetView=contextView;
  closeContextMenu();
  if(!targetView||targetView.closed||item.disabled)return;
  if(targetView.tab?.dataset?.viewKind==='editor')globalThis.TaskMenuEditor?.activateEditor?.(targetView.id);
  else if(targetView.meta?.id)app.activateView(targetView.meta.id,{force:true});
  Promise.resolve(action.run?.(targetView)).catch(app.showError);
}

function positionSubmenu(owner){
  if(!submenu)return;
  const ownerRect=owner.getBoundingClientRect();
  globalThis.TaskMenuContextViewport.place(submenu,ownerRect.right,ownerRect.top,{anchor:ownerRect,submenu:true});
}

function openCustomSubmenu(owner,actions){
  if(submenuOwner===owner&&submenu)return;
  closeSubmenu();
  submenuOwner=owner;
  submenu=document.createElement('div');
  submenu.className='tab-context-submenu open';
  submenu.setAttribute('role','menu');
  for(const action of actions||[]){
    const item=document.createElement('button');
    item.type='button';
    item.textContent=action.label||'Action';
    styleCustomItem(item,action);
    item.onclick=event=>{
      event.preventDefault();event.stopPropagation();
      runCustomAction(action,item);
    };
    submenu.append(item);
  }
  document.body.append(submenu);
  positionSubmenu(owner);
}

function addCustomActions(label,actions){
  if(!Array.isArray(actions)||!actions.length)return false;
  if(label)addHeading(label);
  for(const action of actions){
    const item=document.createElement('button');
    item.type='button';
    item.textContent=action.label||'Action';
    styleCustomItem(item,action);
    if(Array.isArray(action.children)&&action.children.length){
      item.classList.add('tab-context-submenu-parent');
      const arrow=document.createElement('span');arrow.className='context-submenu-arrow';arrow.textContent='▶';item.append(arrow);
      item.onpointerenter=()=>openCustomSubmenu(item,action.children);
      item.onclick=event=>{
        event.preventDefault();event.stopPropagation();
        if(submenuOwner===item&&submenu)closeSubmenu();else openCustomSubmenu(item,action.children);
      };
    }else{
      item.onpointerenter=()=>{if(submenuOwner!==item)closeSubmenu();};
      item.onclick=event=>{
        event.preventDefault();event.stopPropagation();
        runCustomAction(action,item);
      };
    }
    menu.append(item);
  }
  return true;
}

function clampPosition(x,y){
  globalThis.TaskMenuContextViewport.place(menu,x,y);
}

function openEditorContextMenu(view,x,y){
  closeContextMenu();
  contextView=view;
  const descriptor=descriptorForTab(view.tab);
  addReadOnlyToggle(descriptor);
  const sep=document.createElement('div');sep.className='tab-context-separator';menu.append(sep);
  addHeading('Editor');
  const editor=globalThis.TaskMenuEditor;
  const projectActions=(globalThis.TaskMenuProjectFileActions?.standardActions?.(view.file?.path,'file')||[])
    .filter(action=>action.label!=='Open'&&!action.separator);
  const splitParent=editor?.getSplitParent?.(view);
  const compare=globalThis.TaskMenuFileCompare;
  const currentCompareSource=compare?.editorSource?.(view,{label:'Editor buffer'});
  const actions=[
    {label:'Save',title:'Save file',disabled:Boolean(view.file?.read_only)||Boolean(view.tabReadOnly)||!view.dirty,run:()=>editor?.saveEditor?.(view)},
    {label:'Reload',title:'Reload file from disk',run:()=>editor?.reloadEditor?.(view)},
    {label:'Select current editor for compare',title:'Use the current editor buffer as Compare A',disabled:!currentCompareSource,run:()=>compare?.selectForCompare?.(currentCompareSource)},
    {label:'Compare selected file ↔ current editor',title:'Open File Compare with the previously selected Compare A source and this editor buffer',disabled:!currentCompareSource||!compare?.selection||!compare?.canCompareWithSelected?.(currentCompareSource),run:()=>compare?.compareWithSelected?.(currentCompareSource,{title:'Selected file ↔ Editor'})},
    {label:'Compare current ↔ saved',title:'Compare the current editor buffer with the saved project file',run:()=>compare?.openEditorSaved?.(view)},
    {label:'Compare current ↔ clipboard',title:'Compare the current editor buffer with clipboard text',run:()=>compare?.openEditorClipboard?.(view)},
    {label:'Go to line…',title:'Go to a line in this file',run:()=>editor?.goToLine?.(view)},
    {label:'Split vertical…',title:'Show another file beside this editor',disabled:editor?.splitSupported===false,run:()=>editor?.splitEditor?.(view,'vertical')},
    {label:'Split horizontal…',title:'Show another file below this editor',disabled:editor?.splitSupported===false,run:()=>editor?.splitEditor?.(view,'horizontal')},
    {label:'Swap split panes',title:'Swap this editor with its split sibling',disabled:!splitParent,run:()=>editor?.swapEditorSplit?.(view)},
    {label:'Unsplit',title:'Remove this editor from its current split',disabled:!splitParent,run:()=>editor?.unsplitEditor?.(view)},
    {label:'Close',title:'Close editor tab',danger:true,run:()=>editor?.closeEditor?.(view.id)}
  ];
  // Frequent editor actions stay one click away. The long-tail operations
  // live behind consistent first-level submenus.
  addCustomActions('',[actions[0],actions[1],actions[6]]);
  addCustomActions('',[
    {label:'Compare',children:actions.slice(2,6)},
    {label:'Split',children:actions.slice(7,11)},
    ...(projectActions.length?[{label:'Project',children:projectActions}]:[]),
    {label:'Lifecycle',children:[actions[11]]}
  ]);
  menu.classList.add('open');
  clampPosition(x,y);
}

function openContextMenu(view,x,y){
  closeContextMenu();
  contextView=view;
  const descriptor=descriptorForTab(view.tab);
  addReadOnlyToggle(descriptor);
  const session=sourceActions(view,'Session');
  const consoleActions=sourceActions(view,'Console');
  const presetActions=globalThis.TaskMenuCommandPresets?.contextActions?.(view)||[];
  const broadcastActions=globalThis.TaskMenuBroadcast?.contextActions?.(view)||[];
  const grouped=groupSessionSources(view,session);
  const consoleItems=consoleActions.map(source=>sourceAction(view,source,'Console'));
  const lifecycle=[closeTabAction(view),...grouped.lifecycle];
  const groups=[
    ...(consoleItems.length?[{label:'Console',children:consoleItems}]:[]),
    ...(grouped.session.length?[{label:'Session',children:grouped.session}]:[]),
    ...(grouped.split.length?[{label:'Split',children:grouped.split}]:[]),
    ...(lifecycle.length?[{label:'Lifecycle',children:lifecycle}]:[]),
    ...presetActions,
    ...(broadcastActions.length?[{label:'Broadcast group',children:broadcastActions}]:[])
  ];
  if(groups.length){
    const sep=document.createElement('div');sep.className='tab-context-separator';menu.append(sep);
    addCustomActions('',groups);
  }else{
    const empty=document.createElement('div');empty.className='tab-context-heading';empty.textContent='NO ACTIONS AVAILABLE';menu.append(empty);
  }
  menu.classList.add('open');
  clampPosition(x,y);
}

function openFeatureContextMenu(descriptor,x,y){
  closeContextMenu();
  addReadOnlyToggle(descriptor);
  menu.classList.add('open');
  clampPosition(x,y);
}

// Capture close intent before feature-specific handlers remove their views.
// Only commit it to history after the tab actually disappears (or Patch Tool
// becomes hidden), so canceling an unsaved-edits prompt adds no false entry.
const tabClosePending=new WeakMap();
const recentlyClosedTabs=[];
function reopenDescriptor(tab){
  if(!tab||!tab.dataset?.id)return null;
  const id=String(tab.dataset.id);
  const editor=globalThis.TaskMenuEditor?.editors?.get?.(id);
  if(editor)return {kind:'editor',path:String(editor.file?.path||'')};
  if(tab.classList.contains('file-compare-tab')){
    const state=globalThis.TaskMenuFileCompare?.snapshotState?.();
    return state?{kind:'file-compare',state}:null;
  }
  if(tab.classList.contains('dircmp-tab')){
    const state=globalThis.TaskMenuDirectoryCompare?.snapshotState?.();
    return state?{kind:'directory-compare',state}:null;
  }
  if(tab.classList.contains('hex-tab')){
    const view=globalThis.TaskMenuHexViewer?.views?.get?.(id);
    return view?{kind:'hex',path:String(view.path||''),offset:view.currentOffset||0}:null;
  }
  if(tab.classList.contains('task-patch-tab'))return {kind:'patch'};
  if(tab.classList.contains('db-tab')){
    const view=globalThis.TaskMenuDatabase?.views?.get?.(id);
    const profile=String(view?.meta?.profile_id||view?.profile?.id||'');
    return profile?{kind:'database',profile}:null;
  }
  if(tab.classList.contains('ft-tab')){
    const view=[...(globalThis.TaskMenuFileTransfer?.views?.values?.()||[])].find(v=>v?.tab===tab);
    const profile=String(view?.profile?.id||'');
    return profile?{kind:'file-transfer',profile}:null;
  }
  if(app.views.has(id))return {kind:'session',id};
  return null;
}
function pushRecentlyClosed(item){
  if(!item)return;
  recentlyClosedTabs.unshift(item);
  if(recentlyClosedTabs.length>20)recentlyClosedTabs.length=20;
}
async function reopenClosedTab(){
  while(recentlyClosedTabs.length){
    const entry=recentlyClosedTabs.shift();
    try{
      switch(entry.kind){
        case 'editor':
          if(entry.path){await globalThis.TaskMenuEditor?.openFile?.(entry.path);return true;}
          break;
        case 'session':
          if(await app.reopenHiddenSession?.(entry.id))return true;
          break;
        case 'file-compare':
          if(await globalThis.TaskMenuFileCompare?.restoreState?.(entry.state))return true;
          break;
        case 'directory-compare':
          if(await globalThis.TaskMenuDirectoryCompare?.restoreState?.(entry.state))return true;
          break;
        case 'hex':
          if(entry.path){
            await globalThis.TaskMenuHexViewer?.restoreState?.([{path:entry.path,offset:entry.offset}]);
            return true;
          }
          break;
        case 'patch':
          globalThis.TaskMenuPatchPanel?.open?.();return true;
        case 'database':
          if(entry.profile){await globalThis.TaskMenuDatabase?.openProfile?.(entry.profile);return true;}
          break;
        case 'file-transfer':
          if(entry.profile){await globalThis.TaskMenuFileTransfer?.openProfile?.(entry.profile);return true;}
          break;
      }
    }catch(error){
      console.warn('Cannot reopen recently closed tab',entry,error);
      // Do not rerun old terminal commands or silently start new terminals.
    }
  }
  return false;
}
function openEmptyTabContextMenu(x,y){
  closeContextMenu();
  addHeading('Workspace tabs');
  const add=(label,run,{disabled=false,title=''}={})=>{
    const button=document.createElement('button');
    button.type='button';button.textContent=label;button.title=title;button.disabled=disabled;
    button.onclick=event=>{
      event.preventDefault();event.stopPropagation();closeContextMenu();
      Promise.resolve().then(run).catch(app.showError);
    };
    menu.append(button);
  };
  add('Re-open closed tab',async()=>{
    if(!await reopenClosedTab())throw new Error('Closed tab cannot be restored: the session, file or connection no longer exists.');
  },{
    disabled:recentlyClosedTabs.length===0,
    title:'Restore the most recently closed editor, terminal, comparison, viewer, connection or Patch Tool'
  });
  add('New terminal',()=>app.startTerminal(),{
    disabled:Boolean(app.sharedMode&&!app.hasPermission?.('terminal.create')),
    title:'Open a new local terminal'
  });
  const separator=document.createElement('div');separator.className='tab-context-separator';menu.append(separator);
  add('Quick Open…  Ctrl+P',()=>globalThis.TaskMenuQuickOpen?.open?.());
  add('Search in Files…  Ctrl+Shift+F',()=>globalThis.TaskMenuProjectSearch?.open?.());
  menu.classList.add('open');clampPosition(x,y);
}

function readonlyPaneFromTarget(target){
  return target instanceof Element?target.closest('[data-taskdeck-readonly="1"]'):null;
}

function readonlyMutationPaneFromTarget(target){
  const pane=readonlyPaneFromTarget(target);
  if(!pane||pane.dataset.taskdeckReadonlyKind==='terminal')return null;
  return pane;
}

function stopReadonlyMutation(event){
  const pane=readonlyMutationPaneFromTarget(event.target);
  if(!pane)return;
  event.preventDefault();
  event.stopImmediatePropagation();
}

function readonlyKeyAllowed(event){
  if(event.ctrlKey||event.metaKey)return ['a','c','f'].includes(String(event.key||'').toLowerCase());
  return ['ArrowUp','ArrowDown','ArrowLeft','ArrowRight','PageUp','PageDown','Home','End','Escape','Tab'].includes(event.key);
}

document.addEventListener('pointerdown',event=>{
  const pane=readonlyMutationPaneFromTarget(event.target);
  if(!pane)return;
  const target=event.target instanceof Element?event.target:null;
  const interactive=target?.closest('button,input,select,textarea,[role="button"],[role="tab"]');
  if(!interactive)return;
  event.preventDefault();event.stopImmediatePropagation();
},true);
document.addEventListener('beforeinput',stopReadonlyMutation,true);
document.addEventListener('paste',stopReadonlyMutation,true);
document.addEventListener('cut',stopReadonlyMutation,true);
document.addEventListener('drop',stopReadonlyMutation,true);
document.addEventListener('change',stopReadonlyMutation,true);
document.addEventListener('submit',stopReadonlyMutation,true);
document.addEventListener('keydown',event=>{
  if(!readonlyMutationPaneFromTarget(event.target)||readonlyKeyAllowed(event))return;
  event.preventDefault();event.stopImmediatePropagation();
},true);
document.addEventListener('click',event=>{
  if(!readonlyMutationPaneFromTarget(event.target))return;
  event.preventDefault();event.stopImmediatePropagation();
},true);
document.addEventListener('dblclick',event=>{
  if(!readonlyMutationPaneFromTarget(event.target))return;
  event.preventDefault();event.stopImmediatePropagation();
},true);
document.addEventListener('contextmenu',event=>{
  if(!readonlyMutationPaneFromTarget(event.target))return;
  event.stopImmediatePropagation();
},true);

tabsHost.addEventListener('click',event=>{
  const target=event.target instanceof Element?event.target:null;
  const close=target?.closest('.close');
  const tab=close?.closest('[data-id]');
  if(tab?.parentElement===tabsHost)tabClosePending.set(tab,reopenDescriptor(tab));
},true);

tabsHost.addEventListener('contextmenu',event=>{
  const target=event.target instanceof Element?event.target:null;
  const tab=target?.closest('.tab[data-id],.db-tab[data-id],.task-patch-tab');
  if(!tab||!tabsHost.contains(tab)){
    // Includes the 4px gutters between full-width tabs.
    event.preventDefault();event.stopPropagation();
    openEmptyTabContextMenu(event.clientX,event.clientY);
    return;
  }
  const descriptor=descriptorForTab(tab);
  if(!descriptor)return;
  event.preventDefault();event.stopPropagation();
  if(descriptor.kind==='editor')openEditorContextMenu(descriptor.view,event.clientX,event.clientY);
  else if(descriptor.kind==='terminal')openContextMenu(descriptor.view,event.clientX,event.clientY);
  else openFeatureContextMenu(descriptor,event.clientX,event.clientY);
},true);

document.addEventListener('pointerdown',event=>{
  if(!menu.classList.contains('open'))return;
  const target=event.target instanceof Node?event.target:null;
  if(target&&(menu.contains(target)||submenu?.contains(target)))return;
  closeContextMenu();
},true);
document.addEventListener('keydown',event=>{if(event.key==='Escape')closeContextMenu();});
window.addEventListener('blur',closeContextMenu);
window.addEventListener('resize',closeContextMenu);
menu.addEventListener('scroll',()=>closeSubmenu(),{passive:true});
menu.addEventListener('wheel',event=>event.stopPropagation(),{passive:true});

const tabObserver=new MutationObserver(records=>{
  for(const record of records){
    for(const node of record.removedNodes){
      if(!(node instanceof HTMLElement)||node.isConnected)continue;
      if(!node.matches?.('.tab[data-id],.db-tab[data-id],.task-patch-tab'))continue;
      const pending=tabClosePending.get(node);
      if(pending){pushRecentlyClosed(pending);tabClosePending.delete(node);}
    }
    if(record.type==='attributes'&&record.attributeName==='hidden'&&record.target.hidden){
      const pending=tabClosePending.get(record.target);
      if(pending){pushRecentlyClosed(pending);tabClosePending.delete(record.target);}
    }
    for(const node of record.addedNodes){
      if(!(node instanceof Element))continue;
      if(node.matches?.('.tab[data-id],.db-tab[data-id],.task-patch-tab'))registerTab(node);
      for(const tab of node.querySelectorAll?.('.tab[data-id],.db-tab[data-id],.task-patch-tab')||[])registerTab(tab);
    }
  }
});
tabObserver.observe(tabsHost,{childList:true,subtree:true,attributes:true,attributeFilter:['hidden']});
for(const tab of tabsHost.querySelectorAll('.tab[data-id],.db-tab[data-id],.task-patch-tab'))registerTab(tab);

globalThis.TaskMenuTabContext={
  openContextMenu,
  openEmptyTabContextMenu,
  reopenClosedTab,
  get recentlyClosedTabs(){return [...recentlyClosedTabs];},
  closeContextMenu,
  registerTab,
  decorateTerminalPane,
  setReadOnly(tabOrDescriptor,enabled){
    const descriptor=tabOrDescriptor?.kind?tabOrDescriptor:descriptorForTab(tabOrDescriptor);
    return setDescriptorReadOnly(descriptor,enabled);
  },
  isReadOnly(tabOrDescriptor){
    const descriptor=tabOrDescriptor?.kind?tabOrDescriptor:descriptorForTab(tabOrDescriptor);
    return descriptorReadOnly(descriptor);
  }
};
