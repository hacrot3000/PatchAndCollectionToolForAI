const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for split terminal');

const panes=document.querySelector('#panes');
const tabsHost=document.querySelector('#tabs');
const splitSupported=app.layoutProfile!=='mobile';
const installed=new WeakSet();
let roots=[];
let pendingSaved=[];
let renderedRoot=null;
let focusedID='';
let focusTrackingSuspendDepth=0;
let dragging=null;
const resizers=new Map();
const nodeRects=new WeakMap();

const style=document.createElement('style');
style.textContent=`
#panes.split-tree-mode{position:relative;display:block;overflow:hidden}
#panes.split-tree-mode>.pane.split-leaf{position:absolute!important;right:auto!important;bottom:auto!important;display:flex!important;min-width:0;min-height:0;overflow:hidden}
.split-tree-resizer{position:absolute;background:transparent;touch-action:none;z-index:8}
.split-tree-resizer.vertical{cursor:col-resize}
.split-tree-resizer.horizontal{cursor:row-resize}
.split-tree-resizer.vertical::after{content:'';position:absolute;top:0;bottom:0;left:2px;width:1px;background:#30343b}
.split-tree-resizer.horizontal::after{content:'';position:absolute;left:0;right:0;top:2px;height:1px;background:#30343b}
.split-tree-resizer.vertical:hover::after,.split-tree-resizer.vertical.dragging::after{left:1px;width:3px;background:#5a6575}
.split-tree-resizer.horizontal:hover::after,.split-tree-resizer.horizontal.dragging::after{top:1px;height:3px;background:#5a6575}
body.split-resizing-x{user-select:none;cursor:col-resize}
body.split-resizing-y{user-select:none;cursor:row-resize}
.session-split-vertical,.session-split-horizontal,.session-merge-vertical,.session-merge-horizontal,.session-unsplit{white-space:nowrap}
.tab.split-peer{box-shadow:inset 0 -2px #5a88b4}
.pane.split-input-focused>.pane-head,.pane.split-input-focused .pane-head:first-child{background:#26384a!important;box-shadow:inset 0 -2px #6ea8df}
.tab.split-input-focused{box-shadow:inset 0 -3px #6ea8df!important;opacity:1!important}
html[data-taskmenu-theme="light"] .pane.split-input-focused>.pane-head,html[data-taskmenu-theme="light"] .pane.split-input-focused .pane-head:first-child{background:#dcecf9!important}
`;
document.head.append(style);

function storageKey(){return 'vscode-tasks-menu:split:'+(app.layoutProfile||'desktop')+':'+app.taskData.workspace;}
function clampRatio(value){return Math.min(0.8,Math.max(0.2,Number(value)||0.5));}
function normalizeOrientation(value){return value==='horizontal'?'horizontal':'vertical';}
function leaf(id){return {type:'leaf',id:String(id||'').trim()};}
function splitNode(first,second,orientation,ratio=0.5){
  return {type:'split',first,second,orientation:normalizeOrientation(orientation),ratio:clampRatio(ratio)};
}
function normalizeGroup(raw){
  const first=String(raw?.first??raw?.left??'').trim();
  const second=String(raw?.second??raw?.right??'').trim();
  if(!first||!second||first===second)return null;
  return {first,second,ratio:clampRatio(raw?.ratio),orientation:normalizeOrientation(raw?.orientation)};
}
function isSplit(node){return node?.type==='split';}
function containsID(node,id){
  if(!node)return false;
  if(node.type==='leaf')return node.id===id;
  return containsID(node.first,id)||containsID(node.second,id);
}
function representativeID(node){
  if(!node)return '';
  if(node.type==='leaf')return node.id;
  return representativeID(node.first)||representativeID(node.second);
}
function leafIDs(node,out=[]){
  if(!node)return out;
  if(node.type==='leaf'){out.push(node.id);return out;}
  leafIDs(node.first,out);leafIDs(node.second,out);return out;
}
function findRoot(id){return roots.find(root=>containsID(root,id))||null;}
function findParentSplit(node,id,parent=null){
  if(!node)return null;
  if(node.type==='leaf')return node.id===id?parent:null;
  return findParentSplit(node.first,id,node)||findParentSplit(node.second,id,node);
}
function parentSplitFor(id){
  const root=findRoot(id);return root?findParentSplit(root,id,null):null;
}
function replaceNode(target,replacement){
  for(let i=0;i<roots.length;i++){
    if(roots[i]===target){if(replacement)roots[i]=replacement;else roots.splice(i,1);return true;}
    if(replaceNodeInside(roots[i],target,replacement))return true;
  }
  return false;
}
function replaceNodeInside(node,target,replacement){
  if(!isSplit(node))return false;
  if(node.first===target){node.first=replacement;return true;}
  if(node.second===target){node.second=replacement;return true;}
  return replaceNodeInside(node.first,target,replacement)||replaceNodeInside(node.second,target,replacement);
}
function findLeafNode(id,node=null){
  const rootsToSearch=node?[node]:roots;
  for(const root of rootsToSearch){
    if(root?.type==='leaf'&&root.id===id)return root;
    if(isSplit(root)){
      const found=findLeafNode(id,root.first)||findLeafNode(id,root.second);
      if(found)return found;
    }
  }
  return null;
}
function pruneRootList(){
  roots=roots.filter(root=>isSplit(root)&&leafIDs(root,[]).length>=2);
}
function detachTerminal(id){
  const root=findRoot(id);if(!root)return null;
  const target=findLeafNode(id,root);if(!target)return null;
  const parent=findParentSplit(root,id,null);
  if(!parent){
    roots=roots.filter(item=>item!==root);return target;
  }
  const sibling=parent.first===target?parent.second:parent.first;
  replaceNode(parent,sibling);
  if(isSplit(sibling)&&parent===root&&!roots.includes(sibling)){}
  pruneRootList();
  return target;
}
function splitLeaf(firstID,secondID,orientation,ratio=0.5){
  if(!firstID||!secondID||firstID===secondID)return null;
  detachTerminal(secondID);
  let first=findLeafNode(firstID);
  if(first){
    const node=splitNode(first,leaf(secondID),orientation,ratio);
    replaceNode(first,node);
    return node;
  }
  const node=splitNode(leaf(firstID),leaf(secondID),orientation,ratio);
  roots.push(node);return node;
}
function applyOperation(raw){
  const group=normalizeGroup(raw);if(!group)return false;
  if(findLeafNode(group.second))return false;
  splitLeaf(group.first,group.second,group.orientation,group.ratio);
  return true;
}
function encodeNode(node,out){
  if(!isSplit(node))return;
  const first=representativeID(node.first),second=representativeID(node.second);
  if(first&&second)out.push({first,second,left:first,right:second,ratio:node.ratio,orientation:node.orientation});
  encodeNode(node.first,out);encodeNode(node.second,out);
}
function getGroups(){
  if(!splitSupported)return [];
  const out=[];for(const root of roots)encodeNode(root,out);return out;
}
function getState(){
  const parent=parentSplitFor(app.active);
  if(!parent)return getGroups()[0]||null;
  const first=representativeID(parent.first),second=representativeID(parent.second);
  return {first,second,left:first,right:second,ratio:parent.ratio,orientation:parent.orientation};
}
function emitChanged(){window.dispatchEvent(new CustomEvent('taskmenu:split-changed',{detail:{state:getState(),groups:getGroups()}}));}
function fitSoon(view){setTimeout(()=>{try{view?.fit?.fit();}catch{}},0);}
function fitRoot(root){for(const id of leafIDs(root,[]))fitSoon(app.views.get(id));}

function readSaved(){
  try{
    const value=JSON.parse(sessionStorage.getItem(storageKey())||'null');
    if(!value)return [];
    const raw=Array.isArray(value.groups)?value.groups:(value.left&&value.right?[value]:[]);
    return raw.map(normalizeGroup).filter(Boolean);
  }catch{return [];}
}
function saveState(){
  try{
    const groups=getGroups();
    if(groups.length)sessionStorage.setItem(storageKey(),JSON.stringify({version:3,groups}));
    else sessionStorage.removeItem(storageKey());
  }catch{}
  emitChanged();
}
function clearSaved(){try{sessionStorage.removeItem(storageKey());}catch{}}

function applyFocusClasses(){
  for(const view of app.views.values()){
    const on=Boolean(focusedID&&view.meta.id===focusedID&&findRoot(focusedID)===renderedRoot);
    view.pane.classList.toggle('split-input-focused',on);
    view.tab.classList.toggle('split-input-focused',on);
  }
}
function suspendFocusTracking(){
  focusTrackingSuspendDepth++;
  focusedID='';
  applyFocusClasses();
}
function resumeFocusTracking(preferredID=''){
  if(focusTrackingSuspendDepth>0)focusTrackingSuspendDepth--;
  if(focusTrackingSuspendDepth>0)return;
  focusedID=preferredID&&app.views.has(preferredID)?preferredID:'';
  applyFocusClasses();
}
function markFocused(id){
  if(focusTrackingSuspendDepth>0||!id)return;
  focusedID=id;applyFocusClasses();
  if(app.active!==id){
    app.activateView(id);
    setTimeout(syncForActive,0);
  }
}
function clearFocused(id){
  if(focusTrackingSuspendDepth>0)return;
  setTimeout(()=>{
    if(focusTrackingSuspendDepth>0)return;
    const view=app.views.get(id);
    const active=document.activeElement;
    if(view?.pane?.contains(active)&&active?.closest?.('.xterm'))return;
    if(focusedID===id){focusedID='';applyFocusClasses();}
  },0);
}

function cleanupPresentation(){
  renderedRoot=null;dragging=null;
  for(const view of app.views.values()){
    view.pane.classList.remove('split-leaf','split-input-focused');
    view.tab.classList.remove('split-peer','split-input-focused');
    for(const prop of ['left','top','width','height'])view.pane.style.removeProperty(prop);
  }
  panes.classList.remove('split-tree-mode');
  for(const item of resizers.values())item.remove();
  resizers.clear();
  document.body.classList.remove('split-resizing-x','split-resizing-y');
}
function ensureResizer(node){
  let bar=resizers.get(node);
  if(bar?.isConnected)return bar;
  bar=document.createElement('div');bar.className='split-tree-resizer '+node.orientation;
  bar.title='Drag to resize this split; double-click to reset to 50/50';
  panes.append(bar);resizers.set(node,bar);
  bar.addEventListener('pointerdown',event=>{
    if(event.button!==0||renderedRoot==null)return;
    dragging={node,pointerId:event.pointerId};bar.classList.add('dragging');
    document.body.classList.add(node.orientation==='vertical'?'split-resizing-x':'split-resizing-y');
    bar.setPointerCapture(event.pointerId);event.preventDefault();
  });
  bar.addEventListener('pointermove',event=>{
    if(!dragging||dragging.node!==node)return;
    const rect=nodeRects.get(node),host=panes.getBoundingClientRect();if(!rect)return;
    const gap=6;
    const raw=node.orientation==='vertical'
      ?(event.clientX-host.left-rect.x)/(Math.max(1,rect.w-gap))
      :(event.clientY-host.top-rect.y)/(Math.max(1,rect.h-gap));
    if(!Number.isFinite(raw))return;
    node.ratio=clampRatio(raw);layoutCurrentRoot();event.preventDefault();
  });
  const finish=event=>{
    if(!dragging||dragging.node!==node)return;
    dragging=null;bar.classList.remove('dragging');document.body.classList.remove('split-resizing-x','split-resizing-y');
    try{if(bar.hasPointerCapture(event.pointerId))bar.releasePointerCapture(event.pointerId);}catch{}
    saveState();
  };
  bar.addEventListener('pointerup',finish);bar.addEventListener('pointercancel',finish);
  bar.addEventListener('dblclick',()=>{node.ratio=0.5;layoutCurrentRoot();saveState();});
  return bar;
}
function placePane(id,rect){
  const view=app.views.get(id);if(!view)return;
  view.pane.classList.add('split-leaf');view.tab.classList.add('split-peer');
  view.pane.style.left=Math.round(rect.x)+'px';view.pane.style.top=Math.round(rect.y)+'px';
  view.pane.style.width=Math.max(0,Math.round(rect.w))+'px';view.pane.style.height=Math.max(0,Math.round(rect.h))+'px';
}
function layoutNode(node,rect,used){
  if(!node)return;
  if(node.type==='leaf'){placePane(node.id,rect);return;}
  nodeRects.set(node,rect);
  const gap=6,bar=ensureResizer(node);used.add(node);
  bar.classList.toggle('vertical',node.orientation==='vertical');bar.classList.toggle('horizontal',node.orientation==='horizontal');
  if(node.orientation==='vertical'){
    const available=Math.max(0,rect.w-gap),firstW=Math.round(available*clampRatio(node.ratio)),secondW=Math.max(0,available-firstW);
    bar.style.left=Math.round(rect.x+firstW)+'px';bar.style.top=Math.round(rect.y)+'px';bar.style.width=gap+'px';bar.style.height=Math.round(rect.h)+'px';
    layoutNode(node.first,{x:rect.x,y:rect.y,w:firstW,h:rect.h},used);
    layoutNode(node.second,{x:rect.x+firstW+gap,y:rect.y,w:secondW,h:rect.h},used);
  }else{
    const available=Math.max(0,rect.h-gap),firstH=Math.round(available*clampRatio(node.ratio)),secondH=Math.max(0,available-firstH);
    bar.style.left=Math.round(rect.x)+'px';bar.style.top=Math.round(rect.y+firstH)+'px';bar.style.width=Math.round(rect.w)+'px';bar.style.height=gap+'px';
    layoutNode(node.first,{x:rect.x,y:rect.y,w:rect.w,h:firstH},used);
    layoutNode(node.second,{x:rect.x,y:rect.y+firstH+gap,w:rect.w,h:secondH},used);
  }
}
function layoutCurrentRoot(){
  if(!renderedRoot||!isSplit(renderedRoot))return;
  panes.classList.add('split-tree-mode');
  for(const view of app.views.values()){
    view.pane.classList.remove('split-leaf');
    view.tab.classList.remove('split-peer');
    for(const prop of ['left','top','width','height'])view.pane.style.removeProperty(prop);
  }
  const host=panes.getBoundingClientRect(),used=new Set();
  layoutNode(renderedRoot,{x:0,y:0,w:host.width,h:host.height},used);
  for(const [node,bar] of [...resizers])if(!used.has(node)){bar.remove();resizers.delete(node);}
  applyFocusClasses();updateButtons();fitRoot(renderedRoot);
}
function renderRoot(root){
  if(!root||!isSplit(root)){cleanupPresentation();updateButtons();return false;}
  if(renderedRoot!==root){
    cleanupPresentation();renderedRoot=root;
  }
  layoutCurrentRoot();return true;
}
function syncForActive(){
  if(!splitSupported){cleanupPresentation();updateButtons();return;}
  const root=findRoot(app.active);
  if(root&&isSplit(root))renderRoot(root);
  else if(renderedRoot||panes.classList.contains('split-tree-mode'))cleanupPresentation();
  updateButtons();applyFocusClasses();
}

function updateButtons(){
  const canMerge=app.views.size>1;
  for(const view of app.views.values()){
    const parent=parentSplitFor(view.meta.id);
    const vertical=view.pane.querySelector('.session-split-vertical');
    const horizontal=view.pane.querySelector('.session-split-horizontal');
    const mergeV=view.pane.querySelector('.session-merge-vertical');
    const mergeH=view.pane.querySelector('.session-merge-horizontal');
    const swap=view.pane.querySelector('.session-swap-split');
    const unsplit=view.pane.querySelector('.session-unsplit');
    if(vertical){vertical.textContent='Split vertical';vertical.title='Split this terminal into a new pane on the right';}
    if(horizontal){horizontal.textContent='Split horizontal';horizontal.title='Split this terminal into a new pane below';}
    if(mergeV)mergeV.disabled=!canMerge;
    if(mergeH)mergeH.disabled=!canMerge;
    if(swap){
      swap.hidden=!parent;
      swap.textContent=parent?.orientation==='horizontal'?'Swap top / bottom':'Swap left / right';
      swap.title=parent?.orientation==='horizontal'?'Swap the two panes in this horizontal split':'Swap the two panes in this vertical split';
    }
    if(unsplit)unsplit.hidden=!parent;
  }
}

function swapSplitView(view){
  const parent=parentSplitFor(view.meta.id);if(!parent)return;
  const first=parent.first;parent.first=parent.second;parent.second=first;
  saveState();syncForActive();fitRoot(findRoot(view.meta.id));
}
function unsplitView(view){
  const id=view.meta.id,root=findRoot(id),target=findLeafNode(id,root);if(!root||!target)return;
  const parent=parentSplitFor(id);if(!parent)return;
  const sibling=parent.first===target?parent.second:parent.first;
  if(parent===root){
    roots=roots.filter(item=>item!==root);
    if(isSplit(sibling))roots.push(sibling);
  }else{
    replaceNode(parent,target);
    if(isSplit(sibling))roots.push(sibling);
  }
  pruneRootList();saveState();app.activateView(id);setTimeout(syncForActive,0);updateButtons();
}
async function splitFrom(view,orientation){
  if(!splitSupported)return;
  const before=new Set(app.views.keys());
  await app.startTerminal();
  const created=[...app.views.keys()].find(id=>!before.has(id));
  if(!created)throw new Error('Could not create a new terminal for the split');
  splitLeaf(view.meta.id,created,orientation,0.5);
  saveState();app.activateView(created);
  setTimeout(()=>{syncForActive();app.views.get(created)?.term.focus();},0);
}

function viewsInTabOrder(){
  const out=[];const seen=new Set();
  for(const tab of tabsHost?.querySelectorAll('.tab[data-id]')||[]){
    const id=tab.dataset.id||'',view=app.views.get(id);if(view&&!seen.has(id)){seen.add(id);out.push(view);}
  }
  for(const view of app.views.values())if(!seen.has(view.meta.id))out.push(view);
  return out;
}
function chooseMergeTarget(view,orientation){
  const candidates=viewsInTabOrder().filter(candidate=>candidate.meta.id!==view.meta.id);
  if(!candidates.length)throw new Error('No other tab is available to merge');
  const label=orientation==='horizontal'?'horizontal':'vertical';
  const lines=candidates.map((candidate,index)=>`${index+1}. ${candidate.meta.label} [${candidate.meta.status}]`).join('\n');
  const answer=prompt(`Merge ${label} tab “${view.meta.label}” with which tab?\n\n${lines}\n\nEnter a tab number:`,'1');
  if(answer===null)return null;
  const index=Number.parseInt(answer.trim(),10)-1;
  if(!Number.isInteger(index)||index<0||index>=candidates.length)throw new Error('Invalid merge target');
  return candidates[index];
}
function mergeWith(view,orientation){
  if(!splitSupported)return;
  const target=chooseMergeTarget(view,orientation);if(!target)return;
  detachTerminal(target.meta.id);
  splitLeaf(view.meta.id,target.meta.id,orientation,0.5);
  saveState();app.activateView(view.meta.id);setTimeout(()=>{syncForActive();view.term.focus();},0);
}

function bindFocusTracking(view){
  view.pane.addEventListener('focusin',event=>{if(event.target?.closest?.('.xterm'))markFocused(view.meta.id);},true);
  view.pane.addEventListener('focusout',event=>{if(event.target?.closest?.('.xterm'))clearFocused(view.meta.id);},true);
  view.pane.addEventListener('pointerdown',event=>{if(event.target?.closest?.('.xterm'))markFocused(view.meta.id);},true);
}
function install(view){
  if(!splitSupported||!view?.pane||installed.has(view))return;
  installed.add(view);bindFocusTracking(view);
  const vertical=document.createElement('button');vertical.className='session-split-vertical';vertical.textContent='Split vertical';vertical.onclick=()=>splitFrom(view,'vertical').catch(app.showError);
  const horizontal=document.createElement('button');horizontal.className='session-split-horizontal';horizontal.textContent='Split horizontal';horizontal.onclick=()=>splitFrom(view,'horizontal').catch(app.showError);
  const mergeV=document.createElement('button');mergeV.className='session-merge-vertical';mergeV.textContent='Merge vertical…';mergeV.title='Merge this tab with an existing tab in a vertical split';mergeV.onclick=()=>{try{mergeWith(view,'vertical');}catch(e){app.showError(e);}};
  const mergeH=document.createElement('button');mergeH.className='session-merge-horizontal';mergeH.textContent='Merge horizontal…';mergeH.title='Merge this tab with an existing tab in a horizontal split';mergeH.onclick=()=>{try{mergeWith(view,'horizontal');}catch(e){app.showError(e);}};
  const swap=document.createElement('button');swap.className='session-swap-split';swap.textContent='Swap panes';swap.hidden=true;swap.onclick=()=>swapSplitView(view);
  const unsplit=document.createElement('button');unsplit.className='session-unsplit';unsplit.textContent='Unsplit';unsplit.onclick=()=>unsplitView(view);
  const stop=view.pane.querySelector('.stop');if(stop)stop.before(vertical,horizontal,mergeV,mergeH,swap,unsplit);else view.pane.querySelector('.pane-head')?.append(vertical,horizontal,mergeV,mergeH,swap,unsplit);
  updateButtons();
}

function tryRestoreSaved(){
  if(!pendingSaved.length)return;
  let index=0,changed=false;
  while(index<pendingSaved.length){
    const group=normalizeGroup(pendingSaved[index]);if(!group){index++;continue;}
    if(!app.views.has(group.first)||!app.views.has(group.second))break;
    applyOperation(group);changed=true;index++;
  }
  if(index>0)pendingSaved=pendingSaved.slice(index);
  if(changed){updateButtons();syncForActive();}
}
function restoreProjectGroups(rawGroups){
  if(!splitSupported){cleanupPresentation();roots=[];pendingSaved=[];return [];}
  cleanupPresentation();roots=[];pendingSaved=(Array.isArray(rawGroups)?rawGroups:[]).map(normalizeGroup).filter(Boolean);clearSaved();
  tryRestoreSaved();saveState();syncForActive();updateButtons();return getGroups();
}
function restoreProjectSplit(leftID,rightID,savedRatio,orientation='vertical'){
  return restoreProjectGroups([{first:leftID,second:rightID,ratio:savedRatio,orientation}]).length>0;
}
function clearPresentation(){cleanupPresentation();updateButtons();}
function clearAll(){cleanupPresentation();roots=[];pendingSaved=[];clearSaved();updateButtons();emitChanged();}
function clearSplit(){clearAll();}

function pruneMissingViews(){
  const before=JSON.stringify(getGroups());
  function prune(node){
    if(!node)return null;
    if(node.type==='leaf')return app.views.has(node.id)?node:null;
    const first=prune(node.first),second=prune(node.second);
    if(!first)return second;if(!second)return first;
    node.first=first;node.second=second;return node;
  }
  roots=roots.map(prune).filter(isSplit);
  const after=JSON.stringify(getGroups());
  if(before!==after)saveState();
}

window.addEventListener('taskmenu:session',event=>{
  const view=event.detail?.view;if(view)install(view);
  setTimeout(()=>{tryRestoreSaved();syncForActive();updateButtons();},0);
});
window.addEventListener('taskmenu:view-activated',event=>{
  if(event.detail?.kind==='external'){cleanupPresentation();updateButtons();return;}
  if(event.detail?.kind==='terminal')setTimeout(syncForActive,0);
});
for(const view of app.views.values())install(view);
pendingSaved=splitSupported?readSaved():[];setTimeout(()=>{tryRestoreSaved();syncForActive();updateButtons();},0);

document.addEventListener('click',event=>{
  const target=event.target instanceof Element?event.target:null;
  const tab=target?.closest('.tab');if(!tab)return;
  setTimeout(()=>{syncForActive();const root=findRoot(tab.dataset.id);if(root)app.views.get(tab.dataset.id)?.term.focus();},0);
});
window.addEventListener('resize',()=>{if(renderedRoot)layoutCurrentRoot();});

const observer=new MutationObserver(()=>setTimeout(()=>{pruneMissingViews();tryRestoreSaved();syncForActive();updateButtons();},0));
observer.observe(panes,{childList:true});

globalThis.TaskMenuSplit={getState,getGroups,restoreProjectSplit,restoreProjectGroups,swapSplitView,clearSplit,clearAll,clearPresentation,syncForActive,suspendFocusTracking,resumeFocusTracking,isSupported:()=>splitSupported};
