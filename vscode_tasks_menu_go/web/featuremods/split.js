const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for split terminal');

const panes=document.querySelector('#panes');
const tabsHost=document.querySelector('#tabs');
const installed=new WeakSet();
let groups=[];
let pendingSaved=[];
let resizer=null;
let dragging=false;
let renderedGroup=null;

const style=document.createElement('style');
style.textContent=`
#panes.split-mode{display:grid;overflow:hidden}
#panes.split-mode.split-vertical{grid-template-columns:minmax(0,var(--split-primary,50%)) 6px minmax(0,1fr);grid-template-rows:minmax(0,1fr)}
#panes.split-mode.split-horizontal{grid-template-columns:minmax(0,1fr);grid-template-rows:minmax(0,var(--split-primary,50%)) 6px minmax(0,1fr)}
#panes.split-mode.split-vertical>.pane.split-first{position:relative;inset:auto;grid-column:1;grid-row:1;display:flex!important;min-width:0;min-height:0}
#panes.split-mode.split-vertical>.pane.split-second{position:relative;inset:auto;grid-column:3;grid-row:1;display:flex!important;min-width:0;min-height:0}
#panes.split-mode.split-horizontal>.pane.split-first{position:relative;inset:auto;grid-column:1;grid-row:1;display:flex!important;min-width:0;min-height:0}
#panes.split-mode.split-horizontal>.pane.split-second{position:relative;inset:auto;grid-column:1;grid-row:3;display:flex!important;min-width:0;min-height:0}
.split-resizer{position:relative;background:transparent;touch-action:none;z-index:4}
.split-resizer.vertical{grid-column:2;grid-row:1;cursor:col-resize}
.split-resizer.horizontal{grid-column:1;grid-row:2;cursor:row-resize}
.split-resizer.vertical::after{content:'';position:absolute;top:0;bottom:0;left:2px;width:1px;background:#30343b}
.split-resizer.horizontal::after{content:'';position:absolute;left:0;right:0;top:2px;height:1px;background:#30343b}
.split-resizer.vertical:hover::after,.split-resizer.vertical.dragging::after{left:1px;width:3px;background:#5a6575}
.split-resizer.horizontal:hover::after,.split-resizer.horizontal.dragging::after{top:1px;height:3px;background:#5a6575}
body.split-resizing-x{user-select:none;cursor:col-resize}
body.split-resizing-y{user-select:none;cursor:row-resize}
.session-split-vertical,.session-split-horizontal,.session-merge-vertical,.session-merge-horizontal,.session-unsplit{white-space:nowrap}
.tab.split-peer{box-shadow:inset 0 -2px #5a88b4}
`;
document.head.append(style);

function storageKey(){return 'vscode-tasks-menu:split:'+app.taskData.workspace;}
function clampRatio(value){return Math.min(0.8,Math.max(0.2,Number(value)||0.5));}
function normalizeOrientation(value){return value==='horizontal'?'horizontal':'vertical';}
function normalizeGroup(raw){
  const first=String(raw?.first??raw?.left??'').trim();
  const second=String(raw?.second??raw?.right??'').trim();
  if(!first||!second||first===second)return null;
  return {first,second,ratio:clampRatio(raw?.ratio),orientation:normalizeOrientation(raw?.orientation)};
}
function groupFor(id){return groups.find(group=>group.first===id||group.second===id)||null;}
function groupState(group){return group?{first:group.first,second:group.second,left:group.first,right:group.second,ratio:group.ratio,orientation:group.orientation}:null;}
function getGroups(){return groups.map(group=>groupState(group));}
function getState(){return groupState(groupFor(app.active)||groups[0]||null);}
function emitChanged(){window.dispatchEvent(new CustomEvent('taskmenu:split-changed',{detail:{state:getState(),groups:getGroups()}}));}
function fitSoon(view){setTimeout(()=>{try{view?.fit?.fit();}catch{}},0);}

function readSaved(){
  try{
    const value=JSON.parse(sessionStorage.getItem(storageKey())||'null');
    if(!value)return [];
    const rawGroups=Array.isArray(value.groups)?value.groups:(value.left&&value.right?[value]:[]);
    const out=[];const used=new Set();
    for(const raw of rawGroups){
      const group=normalizeGroup(raw);if(!group||used.has(group.first)||used.has(group.second))continue;
      used.add(group.first);used.add(group.second);out.push(group);
    }
    return out;
  }catch{return [];}
}
function saveState(){
  try{
    if(groups.length)sessionStorage.setItem(storageKey(),JSON.stringify({version:2,groups:getGroups()}));
    else sessionStorage.removeItem(storageKey());
  }catch{}
  emitChanged();
}
function clearSaved(){try{sessionStorage.removeItem(storageKey());}catch{}}

function updateButtons(){
  const canMerge=app.views.size>1;
  for(const view of app.views.values()){
    const group=groupFor(view.meta.id);
    const vertical=view.pane.querySelector('.session-split-vertical');
    const horizontal=view.pane.querySelector('.session-split-horizontal');
    const mergeV=view.pane.querySelector('.session-merge-vertical');
    const mergeH=view.pane.querySelector('.session-merge-horizontal');
    const unsplit=view.pane.querySelector('.session-unsplit');
    if(vertical){vertical.textContent=group?'Use vertical split':'Split vertical';vertical.title=group?'Switch this group to a vertical split':'Open a new terminal in a vertical split';}
    if(horizontal){horizontal.textContent=group?'Use horizontal split':'Split horizontal';horizontal.title=group?'Switch this group to a horizontal split':'Open a new terminal in a horizontal split';}
    if(mergeV)mergeV.disabled=!canMerge;
    if(mergeH)mergeH.disabled=!canMerge;
    if(unsplit)unsplit.hidden=!group;
  }
}

function cleanupPresentation(){
  dragging=false;renderedGroup=null;
  for(const view of app.views.values()){
    view.pane.classList.remove('split-first','split-second');
    view.tab.classList.remove('split-peer');
  }
  panes.classList.remove('split-mode','split-vertical','split-horizontal');
  panes.style.removeProperty('--split-primary');
  resizer?.remove();resizer=null;
  document.body.classList.remove('split-resizing-x','split-resizing-y');
}

function ensureResizer(group){
  if(resizer?.isConnected){
    resizer.classList.toggle('vertical',group.orientation==='vertical');
    resizer.classList.toggle('horizontal',group.orientation==='horizontal');
    return resizer;
  }
  resizer=document.createElement('div');resizer.className='split-resizer '+group.orientation;resizer.title='Drag to resize the panes; double-click to reset to 50/50';
  panes.append(resizer);
  resizer.addEventListener('pointerdown',event=>{
    if(event.button!==0||!renderedGroup)return;
    dragging=true;resizer.classList.add('dragging');
    document.body.classList.add(renderedGroup.orientation==='vertical'?'split-resizing-x':'split-resizing-y');
    resizer.setPointerCapture(event.pointerId);event.preventDefault();
  });
  resizer.addEventListener('pointermove',event=>{
    if(!dragging||!renderedGroup)return;
    const rect=panes.getBoundingClientRect();
    const raw=renderedGroup.orientation==='vertical'?(event.clientX-rect.left)/rect.width:(event.clientY-rect.top)/rect.height;
    if(!Number.isFinite(raw))return;
    renderedGroup.ratio=clampRatio(raw);panes.style.setProperty('--split-primary',(renderedGroup.ratio*100)+'%');
    fitSoon(app.views.get(renderedGroup.first));fitSoon(app.views.get(renderedGroup.second));event.preventDefault();
  });
  const finish=event=>{
    if(!dragging)return;
    dragging=false;resizer.classList.remove('dragging');document.body.classList.remove('split-resizing-x','split-resizing-y');
    try{if(resizer.hasPointerCapture(event.pointerId))resizer.releasePointerCapture(event.pointerId);}catch{}
    saveState();
  };
  resizer.addEventListener('pointerup',finish);resizer.addEventListener('pointercancel',finish);
  resizer.addEventListener('dblclick',()=>{
    if(!renderedGroup)return;
    renderedGroup.ratio=0.5;panes.style.setProperty('--split-primary','50%');saveState();
    fitSoon(app.views.get(renderedGroup.first));fitSoon(app.views.get(renderedGroup.second));
  });
  return resizer;
}

function renderGroup(group){
  const first=app.views.get(group.first),second=app.views.get(group.second);
  if(!first||!second)return false;
  cleanupPresentation();renderedGroup=group;
  panes.classList.add('split-mode','split-'+group.orientation);panes.style.setProperty('--split-primary',(group.ratio*100)+'%');
  first.pane.classList.add('split-first');second.pane.classList.add('split-second');first.tab.classList.add('split-peer');second.tab.classList.add('split-peer');
  ensureResizer(group);updateButtons();fitSoon(first);fitSoon(second);return true;
}

function syncForActive(){
  const group=groupFor(app.active);
  if(group){
    const expectedRatio=(group.ratio*100)+'%';
    const complete=renderedGroup===group&&panes.classList.contains('split-mode')&&panes.classList.contains('split-'+group.orientation)&&panes.style.getPropertyValue('--split-primary')===expectedRatio&&resizer?.isConnected&&resizer.classList.contains(group.orientation);
    if(!complete)renderGroup(group);
  }else if(renderedGroup||panes.classList.contains('split-mode')||resizer?.isConnected){
    cleanupPresentation();updateButtons();
  }
}

function removeGroupsContaining(ids){
  const set=new Set(ids);const before=groups.length;
  groups=groups.filter(group=>!set.has(group.first)&&!set.has(group.second));
  return before!==groups.length;
}

function createGroup(first,second,orientation,ratio=0.5,persist=true){
  if(!first||!second||first===second||!app.views.has(first)||!app.views.has(second))return null;
  const same=groupFor(first);
  if(same&&(same.first===second||same.second===second)){
    same.orientation=normalizeOrientation(orientation);same.ratio=clampRatio(ratio||same.ratio);
    if(persist)saveState();syncForActive();updateButtons();return same;
  }
  removeGroupsContaining([first,second]);
  const group={first,second,ratio:clampRatio(ratio),orientation:normalizeOrientation(orientation)};
  groups.push(group);if(persist)saveState();syncForActive();updateButtons();return group;
}

function unsplitView(view){
  const group=groupFor(view.meta.id);if(!group)return;
  groups=groups.filter(item=>item!==group);cleanupPresentation();saveState();app.activateView(view.meta.id);setTimeout(syncForActive,0);updateButtons();
}

async function splitFrom(view,orientation){
  const existing=groupFor(view.meta.id);
  if(existing){
    existing.orientation=normalizeOrientation(orientation);saveState();app.activateView(view.meta.id);setTimeout(syncForActive,0);return;
  }
  const before=new Set(app.views.keys());
  await app.startTerminal();
  const created=[...app.views.keys()].find(id=>!before.has(id));
  if(!created)throw new Error('Could not create a new terminal for the split');
  createGroup(view.meta.id,created,orientation,0.5,true);
  app.activateView(created);setTimeout(()=>{syncForActive();app.views.get(created)?.term.focus();},0);
}

function viewsInTabOrder(){
  const out=[];const seen=new Set();
  for(const tab of tabsHost?.querySelectorAll('.tab[data-id]')||[]){
    const id=tab.dataset.id||'';const view=app.views.get(id);if(view&&!seen.has(id)){seen.add(id);out.push(view);}
  }
  for(const view of app.views.values())if(!seen.has(view.meta.id))out.push(view);
  return out;
}

function chooseMergeTarget(view,orientation){
  const candidates=viewsInTabOrder().filter(candidate=>candidate.meta.id!==view.meta.id);
  if(!candidates.length)throw new Error('No other tab is available to merge');
  const label=orientation==='horizontal'?'horizontal':'vertical';
  const lines=candidates.map((candidate,index)=>`${index+1}. ${candidate.meta.label} [${candidate.meta.status}]`).join('\n');
  const answer=prompt(`Merge ${label} tab “${view.meta.label}” with which tab?\n\n${lines}\n\nEnter a tab number:`, '1');
  if(answer===null)return null;
  const index=Number.parseInt(answer.trim(),10)-1;
  if(!Number.isInteger(index)||index<0||index>=candidates.length)throw new Error('Invalid merge target');
  return candidates[index];
}

function mergeWith(view,orientation){
  const target=chooseMergeTarget(view,orientation);if(!target)return;
  createGroup(view.meta.id,target.meta.id,orientation,0.5,true);app.activateView(view.meta.id);setTimeout(()=>{syncForActive();view.term.focus();},0);
}

function install(view){
  if(!view?.pane||installed.has(view))return;
  installed.add(view);
  const vertical=document.createElement('button');vertical.className='session-split-vertical';vertical.textContent='Split vertical';vertical.onclick=()=>splitFrom(view,'vertical').catch(app.showError);
  const horizontal=document.createElement('button');horizontal.className='session-split-horizontal';horizontal.textContent='Split horizontal';horizontal.onclick=()=>splitFrom(view,'horizontal').catch(app.showError);
  const mergeV=document.createElement('button');mergeV.className='session-merge-vertical';mergeV.textContent='Merge vertical…';mergeV.title='Merge this tab with an existing tab in a vertical split';mergeV.onclick=()=>{try{mergeWith(view,'vertical');}catch(e){app.showError(e);}};
  const mergeH=document.createElement('button');mergeH.className='session-merge-horizontal';mergeH.textContent='Merge horizontal…';mergeH.title='Merge this tab with an existing tab in a horizontal split';mergeH.onclick=()=>{try{mergeWith(view,'horizontal');}catch(e){app.showError(e);}};
  const unsplit=document.createElement('button');unsplit.className='session-unsplit';unsplit.textContent='Unsplit';unsplit.onclick=()=>unsplitView(view);
  const stop=view.pane.querySelector('.stop');if(stop)stop.before(vertical,horizontal,mergeV,mergeH,unsplit);else view.pane.querySelector('.pane-head')?.append(vertical,horizontal,mergeV,mergeH,unsplit);
  updateButtons();
}

function tryRestoreSaved(){
  if(!pendingSaved.length)return;
  const waiting=[];let changed=false;
  const occupied=new Set(groups.flatMap(group=>[group.first,group.second]));
  for(const raw of pendingSaved){
    const group=normalizeGroup(raw);if(!group)continue;
    if(!app.views.has(group.first)||!app.views.has(group.second)){waiting.push(group);continue;}
    if(occupied.has(group.first)||occupied.has(group.second))continue;
    occupied.add(group.first);occupied.add(group.second);groups.push(group);changed=true;
  }
  pendingSaved=waiting;
  if(changed){updateButtons();syncForActive();}
}

function restoreProjectGroups(rawGroups){
  cleanupPresentation();groups=[];pendingSaved=[];clearSaved();
  const occupied=new Set();
  for(const raw of Array.isArray(rawGroups)?rawGroups:[]){
    const group=normalizeGroup(raw);if(!group||occupied.has(group.first)||occupied.has(group.second))continue;
    if(!app.views.has(group.first)||!app.views.has(group.second)){pendingSaved.push(group);continue;}
    occupied.add(group.first);occupied.add(group.second);groups.push(group);
  }
  tryRestoreSaved();saveState();syncForActive();updateButtons();return getGroups();
}
function restoreProjectSplit(leftID,rightID,savedRatio,orientation='vertical'){
  return restoreProjectGroups([{first:leftID,second:rightID,ratio:savedRatio,orientation}]).length>0;
}
function clearAll(){cleanupPresentation();groups=[];pendingSaved=[];clearSaved();updateButtons();emitChanged();}
function clearSplit(){clearAll();}

window.addEventListener('taskmenu:session',event=>{
  const view=event.detail?.view;if(view)install(view);
  setTimeout(()=>{tryRestoreSaved();syncForActive();updateButtons();},0);
});
for(const view of app.views.values())install(view);
pendingSaved=readSaved();setTimeout(()=>{tryRestoreSaved();syncForActive();updateButtons();},0);

document.addEventListener('click',event=>{
  const target=event.target instanceof Element?event.target:null;
  const tab=target?.closest('.tab');if(!tab)return;
  setTimeout(()=>{syncForActive();const group=groupFor(tab.dataset.id);if(group)app.views.get(tab.dataset.id)?.term.focus();},0);
});

const observer=new MutationObserver(()=>{
  let changed=false;
  groups=groups.filter(group=>{
    const keep=app.views.has(group.first)&&app.views.has(group.second);
    if(!keep)changed=true;return keep;
  });
  if(changed)saveState();
  setTimeout(()=>{tryRestoreSaved();syncForActive();updateButtons();},0);
});
observer.observe(panes,{childList:true});

globalThis.TaskMenuSplit={getState,getGroups,restoreProjectSplit,restoreProjectGroups,clearSplit,clearAll,syncForActive};