const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for tab drag ordering');

const tabsHost=document.querySelector('#tabs');
if(!tabsHost)throw new Error('Tab host unavailable for tab drag ordering');

let dragged=null;
let originalOrder=[];
let committed=false;
let suppressClickUntil=0;

const style=document.createElement('style');
style.textContent=`
#tabs .tab[draggable="true"]{cursor:grab}
#tabs .tab.tab-dragging{opacity:.5;cursor:grabbing}
body.tab-reordering,#tabs.tab-reordering{user-select:none}
`;
document.head.append(style);

function storageKey(){return 'vscode-tasks-menu:tab-order:'+app.taskData.workspace;}
function currentIDs(){
  return [...tabsHost.querySelectorAll('.tab[data-id]')].map(tab=>tab.dataset.id||'').filter(Boolean);
}
function saveOrder(){
  try{sessionStorage.setItem(storageKey(),JSON.stringify(currentIDs()));}
  catch(e){console.warn('Cannot persist tab order',e);}
  globalThis.TaskMenuTerminalRestore?.persistSnapshot?.();
}
function readOrder(){
  try{
    const raw=JSON.parse(sessionStorage.getItem(storageKey())||'[]');
    return Array.isArray(raw)?raw.map(String).filter(Boolean):[];
  }catch{return [];}
}
function applyOrder(ids){
  if(!Array.isArray(ids)||!ids.length)return;
  const byID=new Map([...tabsHost.querySelectorAll('.tab[data-id]')].map(tab=>[tab.dataset.id,tab]));
  for(const id of ids){
    const tab=byID.get(id);
    if(tab){tabsHost.append(tab);byID.delete(id);}
  }
  // Tabs not present in the saved order are new sessions; keep their current
  // relative order after the restored entries.
  for(const tab of byID.values())tabsHost.append(tab);
}
function restoreSavedOrder(){applyOrder(readOrder());}
function installTab(tab){
  if(!tab||tab.dataset.dragOrderInstalled==='1')return;
  tab.dataset.dragOrderInstalled='1';
  tab.draggable=true;
}
function installAll(){
  for(const tab of tabsHost.querySelectorAll('.tab[data-id]'))installTab(tab);
}

function tabAtPointer(event){
  const direct=(event.target instanceof Element)?event.target.closest('.tab[data-id]'):null;
  if(direct&&direct!==dragged)return direct;
  const tabs=[...tabsHost.querySelectorAll('.tab[data-id]')].filter(tab=>tab!==dragged);
  for(const tab of tabs){
    const rect=tab.getBoundingClientRect();
    if(event.clientX<rect.left+rect.width/2)return tab;
  }
  return null;
}
function moveDragged(event){
  if(!dragged)return;
  const before=tabAtPointer(event);
  if(before)tabsHost.insertBefore(dragged,before);
  else tabsHost.append(dragged);
  const rect=tabsHost.getBoundingClientRect();
  if(event.clientX<rect.left+36)tabsHost.scrollLeft-=28;
  else if(event.clientX>rect.right-36)tabsHost.scrollLeft+=28;
}
function cleanup(){
  dragged?.classList.remove('tab-dragging');
  dragged=null;
  originalOrder=[];
  committed=false;
  tabsHost.classList.remove('tab-reordering');
  document.body.classList.remove('tab-reordering');
}

tabsHost.addEventListener('pointerdown',event=>{
  const target=event.target instanceof Element?event.target:null;
  const tab=target?.closest('.tab[data-id]');
  if(!tab)return;
  tab.dataset.dragHandleAllowed=target?.closest('.close')?'0':'1';
},true);

tabsHost.addEventListener('dragstart',event=>{
  const target=event.target instanceof Element?event.target:null;
  const tab=target?.closest('.tab[data-id]');
  if(!tab||tab.dataset.dragHandleAllowed==='0'){event.preventDefault();return;}
  dragged=tab;originalOrder=currentIDs();committed=false;
  tab.classList.add('tab-dragging');tabsHost.classList.add('tab-reordering');document.body.classList.add('tab-reordering');
  if(event.dataTransfer){
    event.dataTransfer.effectAllowed='move';
    try{event.dataTransfer.setData('text/plain',tab.dataset.id||'');}catch{}
  }
});

tabsHost.addEventListener('dragover',event=>{
  if(!dragged)return;
  event.preventDefault();
  if(event.dataTransfer)event.dataTransfer.dropEffect='move';
  moveDragged(event);
});

tabsHost.addEventListener('drop',event=>{
  if(!dragged)return;
  event.preventDefault();moveDragged(event);committed=true;
  saveOrder();
});

tabsHost.addEventListener('dragend',()=>{
  if(!dragged)return;
  if(!committed)applyOrder(originalOrder);
  else saveOrder();
  suppressClickUntil=Date.now()+180;
  cleanup();
});

tabsHost.addEventListener('click',event=>{
  if(Date.now()>=suppressClickUntil)return;
  event.preventDefault();event.stopImmediatePropagation();
},true);

window.addEventListener('taskmenu:session',event=>{
  const tab=event.detail?.view?.tab;
  if(tab)installTab(tab);
});

installAll();
(async()=>{
  try{await globalThis.TaskMenuTerminalRestore?.ready;}catch{}
  restoreSavedOrder();installAll();
  // Persist the final terminal subset after applying the full live-tab order.
  globalThis.TaskMenuTerminalRestore?.persistSnapshot?.();
})();

globalThis.TaskMenuTabOrder={saveOrder,restoreSavedOrder,currentIDs};
