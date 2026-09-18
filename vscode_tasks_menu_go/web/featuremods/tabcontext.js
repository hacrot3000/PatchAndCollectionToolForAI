const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for tab context menu');

const tabsHost=document.querySelector('#tabs');
if(!tabsHost)throw new Error('Tab host unavailable for tab context menu');

const style=document.createElement('style');
style.textContent=`
.tab-context-menu{position:fixed;z-index:4000;display:none;min-width:230px;max-width:min(360px,92vw);max-height:min(620px,86vh);overflow:auto;padding:7px;border:1px solid #3b414d;border-radius:8px;background:#171a20;box-shadow:0 12px 34px rgba(0,0,0,.42)}
.tab-context-menu.open{display:block}
.tab-context-submenu{position:fixed;z-index:4100;display:none;min-width:300px;max-width:min(520px,92vw);max-height:min(620px,86vh);overflow:auto;padding:7px;border:1px solid #3b414d;border-radius:8px;background:#171a20;box-shadow:0 12px 34px rgba(0,0,0,.42)}
.tab-context-submenu.open{display:block}
.tab-context-submenu button{display:block;width:100%;text-align:left;margin:2px 0;padding:6px 9px;font-size:12px}
.tab-context-submenu .context-danger{background:#4a252a;border-color:#7a4048;color:#ffe2e4}
.tab-context-submenu-parent{display:flex!important;align-items:center;gap:8px}
.tab-context-submenu-parent .context-submenu-arrow{margin-left:auto;opacity:.7}
.tab-context-heading{padding:5px 7px 3px;font-size:10px;font-weight:800;letter-spacing:.08em;opacity:.55}
.tab-context-separator{height:1px;background:#30343b;margin:6px 3px}
.tab-context-menu button{display:block;width:100%;text-align:left;margin:2px 0;padding:6px 9px;font-size:12px}
.tab-context-menu button.context-copy{background:#203b58;border-color:#35648d;color:#d9ecff}
.tab-context-menu button.context-find{background:#332a55;border-color:#594a8e;color:#eee7ff}
.tab-context-menu button.context-save{background:#203f31;border-color:#3a7058;color:#dcf6e7}
.tab-context-menu button.context-danger{background:#4a252a;border-color:#7a4048;color:#ffe2e4}
html[data-taskmenu-theme="light"] .tab-context-menu,html[data-taskmenu-theme="light"] .tab-context-submenu{background:#fff;border-color:#b9c0c8;box-shadow:0 12px 34px rgba(0,0,0,.18)}
html[data-taskmenu-theme="light"] .tab-context-menu button.context-copy{background:#e8f2ff;border-color:#8db7df;color:#194b78}
html[data-taskmenu-theme="light"] .tab-context-menu button.context-find{background:#f0ebff;border-color:#afa1da;color:#493b78}
html[data-taskmenu-theme="light"] .tab-context-menu button.context-save{background:#e9f7ef;border-color:#87bf9f;color:#23583b}
html[data-taskmenu-theme="light"] .tab-context-menu button.context-danger{background:#fff0f1;border-color:#d7989e;color:#7b3037}
`;
document.head.append(style);

const menu=document.createElement('div');
menu.className='tab-context-menu';
menu.setAttribute('role','menu');
document.body.append(menu);

let contextView=null;
let submenu=null;
let submenuOwner=null;

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

function semanticClass(button){
  if(button.classList.contains('copy-console'))return 'context-copy';
  if(button.classList.contains('console-search-btn'))return 'context-find';
  if(button.classList.contains('console-save-btn'))return 'context-save';
  if(button.classList.contains('session-clear-console')||button.classList.contains('stop'))return 'context-danger';
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

function addActions(view,label,actions){
  if(!actions.length)return false;
  addHeading(label);
  for(const source of actions){
    const item=document.createElement('button');
    item.type='button';
    item.textContent=(source.textContent||'').trim()||source.title||'Action';
    item.title=source.title||item.textContent;
    item.disabled=source.disabled;
    const semantic=semanticClass(source);if(semantic)item.classList.add(semantic);
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

function openContextMenu(view,x,y){
  closeContextMenu();
  contextView=view;
  const session=sourceActions(view,'Session');
  const broadcastActions=globalThis.TaskMenuBroadcast?.contextActions?.(view)||[];
  const consoleActions=sourceActions(view,'Console');
  const hasSession=addActions(view,'Session',session);
  if(hasSession&&broadcastActions.length){
    const sep=document.createElement('div');sep.className='tab-context-separator';menu.append(sep);
  }
  const hasBroadcast=addCustomActions('Broadcast group',broadcastActions);
  if((hasSession||hasBroadcast)&&consoleActions.length){
    const sep=document.createElement('div');sep.className='tab-context-separator';menu.append(sep);
  }
  const hasConsole=addActions(view,'Console',consoleActions);
  if(!hasSession&&!hasBroadcast&&!hasConsole){
    const empty=document.createElement('div');empty.className='tab-context-heading';empty.textContent='NO ACTIONS AVAILABLE';menu.append(empty);
  }
  menu.classList.add('open');
  clampPosition(x,y);
}

tabsHost.addEventListener('contextmenu',event=>{
  const target=event.target instanceof Element?event.target:null;
  const tab=target?.closest('.tab[data-id]');
  if(!tab||!tabsHost.contains(tab))return;
  const view=app.views.get(tab.dataset.id||'');
  if(!view)return;
  event.preventDefault();event.stopPropagation();
  openContextMenu(view,event.clientX,event.clientY);
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
window.addEventListener('scroll',closeContextMenu,true);

globalThis.TaskMenuTabContext={openContextMenu,closeContextMenu};
