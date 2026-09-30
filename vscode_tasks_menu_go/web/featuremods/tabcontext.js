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
.taskdeck-tab-readonly::before{content:'🔒';font-size:10px;opacity:.72;margin-right:4px}
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

function registerTab(tab){
  setTimeout(()=>{
    const descriptor=descriptorForTab(tab);
    if(descriptor)applyDescriptorReadOnly(descriptor,descriptorReadOnly(descriptor));
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
  return '';
}

function sourceActions(view,label){
  const group=findPaneMenu(view,label);
  if(!group)return [];
  const pop=group.querySelector(':scope > .taskmenu-menu-popover');
  if(!pop)return [];
  return [...pop.children].filter(node=>node instanceof HTMLButtonElement&&!node.hidden&&node.style.display!=='none');
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

function addActions(view,label,actions){
  if(!actions.length)return false;
  addHeading(label);
  for(const source of actions){
    const item=document.createElement('button');
    item.type='button';
    item.textContent=(source.textContent||'').trim()||source.title||'Action';
    item.title=source.title||item.textContent;
    item.disabled=source.disabled;
    const semantic=semanticClass(source,label);if(semantic)item.classList.add(semantic);
    item.onclick=event=>{
      event.preventDefault();event.stopPropagation();
      const targetView=contextView||view;
      closeContextMenu();
      if(!targetView||targetView.closed)return;
      app.activateView(targetView.meta.id);
      setTimeout(()=>{
        if(!source.isConnected||source.disabled)return;
        source.click();
      },0);
    };
    menu.append(item);
  }
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
  app.activateView(targetView.meta.id);
  Promise.resolve(action.run?.(targetView)).catch(app.showError);
}

function positionSubmenu(owner){
  if(!submenu)return;
  const ownerRect=owner.getBoundingClientRect();
  submenu.style.left=(ownerRect.right+4)+'px';
  submenu.style.top=Math.max(4,ownerRect.top)+'px';
  requestAnimationFrame(()=>{
    if(!submenu)return;
    const rect=submenu.getBoundingClientRect();
    let left=ownerRect.right+4;
    if(left+rect.width>window.innerWidth-4)left=Math.max(4,ownerRect.left-rect.width-4);
    const top=Math.max(4,Math.min(ownerRect.top,window.innerHeight-rect.height-4));
    submenu.style.left=left+'px';
    submenu.style.top=top+'px';
  });
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
  menu.style.left=Math.max(4,x)+'px';
  menu.style.top=Math.max(4,y)+'px';
  requestAnimationFrame(()=>{
    if(!menu.classList.contains('open'))return;
    const rect=menu.getBoundingClientRect();
    const left=Math.max(4,Math.min(x,window.innerWidth-rect.width-4));
    const top=Math.max(4,Math.min(y,window.innerHeight-rect.height-4));
    menu.style.left=left+'px';menu.style.top=top+'px';
  });
}

function openEditorContextMenu(view,x,y){
  closeContextMenu();
  const descriptor=descriptorForTab(view.tab);
  addReadOnlyToggle(descriptor);
  const sep=document.createElement('div');sep.className='tab-context-separator';menu.append(sep);
  addHeading('Editor');
  const editor=globalThis.TaskMenuEditor;
  const actions=[
    {label:'Save',title:'Save file',disabled:Boolean(view.file?.read_only)||Boolean(view.tabReadOnly)||!view.dirty,run:()=>editor?.saveEditor?.(view)},
    {label:'Reload',title:'Reload file from disk',run:()=>editor?.reloadEditor?.(view)},
    {label:'Go to line…',title:'Go to a line in this file',run:()=>editor?.goToLine?.(view)},
    {label:'Close',title:'Close editor tab',danger:true,run:()=>editor?.closeEditor?.(view.id)}
  ];
  for(const action of actions){
    const item=document.createElement('button');
    item.type='button';item.textContent=action.label;styleCustomItem(item,action);
    item.onclick=event=>{
      event.preventDefault();event.stopPropagation();
      closeContextMenu();
      if(view.closed||item.disabled)return;
      editor?.activateEditor?.(view.id);
      Promise.resolve(action.run()).catch(app.showError);
    };
    menu.append(item);
  }
  menu.classList.add('open');
  clampPosition(x,y);
}

function openContextMenu(view,x,y){
  closeContextMenu();
  contextView=view;
  const descriptor=descriptorForTab(view.tab);
  addReadOnlyToggle(descriptor);
  const session=sourceActions(view,'Session');
  const presetActions=globalThis.TaskMenuCommandPresets?.contextActions?.(view)||[];
  const broadcastActions=globalThis.TaskMenuBroadcast?.contextActions?.(view)||[];
  const consoleActions=sourceActions(view,'Console');

  if(session.length||presetActions.length||broadcastActions.length||consoleActions.length){
    const sep=document.createElement('div');sep.className='tab-context-separator';menu.append(sep);
  }
  const hasSession=addActions(view,'Session',session);
  if(hasSession&&presetActions.length){
    const sep=document.createElement('div');sep.className='tab-context-separator';menu.append(sep);
  }
  const hasPresets=addCustomActions('',presetActions);
  if((hasSession||hasPresets)&&broadcastActions.length){
    const sep=document.createElement('div');sep.className='tab-context-separator';menu.append(sep);
  }
  const hasBroadcast=addCustomActions('Broadcast group',broadcastActions);
  if((hasSession||hasPresets||hasBroadcast)&&consoleActions.length){
    const sep=document.createElement('div');sep.className='tab-context-separator';menu.append(sep);
  }
  const hasConsole=addActions(view,'Console',consoleActions);
  if(!hasSession&&!hasPresets&&!hasBroadcast&&!hasConsole){
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

tabsHost.addEventListener('contextmenu',event=>{
  const target=event.target instanceof Element?event.target:null;
  const tab=target?.closest('.tab[data-id],.db-tab[data-id],.task-patch-tab');
  if(!tab||!tabsHost.contains(tab))return;
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
    for(const node of record.addedNodes){
      if(!(node instanceof Element))continue;
      if(node.matches?.('.tab[data-id],.db-tab[data-id],.task-patch-tab'))registerTab(node);
      for(const tab of node.querySelectorAll?.('.tab[data-id],.db-tab[data-id],.task-patch-tab')||[])registerTab(tab);
    }
  }
});
tabObserver.observe(tabsHost,{childList:true,subtree:true});
for(const tab of tabsHost.querySelectorAll('.tab[data-id],.db-tab[data-id],.task-patch-tab'))registerTab(tab);

globalThis.TaskMenuTabContext={
  openContextMenu,
  closeContextMenu,
  registerTab,
  setReadOnly(tabOrDescriptor,enabled){
    const descriptor=tabOrDescriptor?.kind?tabOrDescriptor:descriptorForTab(tabOrDescriptor);
    return setDescriptorReadOnly(descriptor,enabled);
  },
  isReadOnly(tabOrDescriptor){
    const descriptor=tabOrDescriptor?.kind?tabOrDescriptor:descriptorForTab(tabOrDescriptor);
    return descriptorReadOnly(descriptor);
  }
};
