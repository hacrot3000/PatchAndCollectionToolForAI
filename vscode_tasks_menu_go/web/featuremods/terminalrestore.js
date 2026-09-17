const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal restore');

const endpoint='/api/state/tasks?scope=terminals';
const tabsHost=document.querySelector('#tabs');
let restoring=true;
let saveTimer=null;
let saveInFlight=Promise.resolve();

function terminalIDsInTabOrder(){
  const ids=[];
  for(const tab of tabsHost?.querySelectorAll('.tab[data-id]')||[]){
    const id=tab.dataset.id||'';
    const view=app.views.get(id);
    if(view?.meta?.task_id===0&&view.meta.status==='running')ids.push(id);
  }
  return ids;
}

function snapshotPayload(){
  const sessionIDs=terminalIDsInTabOrder();
  const activeSessionID=sessionIDs.includes(app.active)?app.active:'';
  const splits=[];
  const used=new Set();
  for(const group of globalThis.TaskMenuSplit?.getGroups?.()||[]){
    const left=String(group?.first??group?.left??'');
    const right=String(group?.second??group?.right??'');
    if(!sessionIDs.includes(left)||!sessionIDs.includes(right)||left===right||used.has(left)||used.has(right))continue;
    used.add(left);used.add(right);
    splits.push({left_session_id:left,right_session_id:right,ratio:Number(group.ratio)||0.5,orientation:group.orientation==='horizontal'?'horizontal':'vertical'});
  }
  return {session_ids:sessionIDs,active_session_id:activeSessionID,splits};
}

function persistSnapshot(){
  if(restoring)return Promise.resolve();
  const payload=snapshotPayload();
  saveInFlight=saveInFlight.catch(()=>{}).then(()=>app.jsonFetch(endpoint,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)})).catch(e=>console.warn('Cannot persist terminal project state',e));
  return saveInFlight;
}

function scheduleSave(delay=500){
  if(restoring)return;
  clearTimeout(saveTimer);
  saveTimer=setTimeout(()=>persistSnapshot(),delay);
}

async function waitForViews(ids,timeout=6000){
  const deadline=Date.now()+timeout;
  while(Date.now()<deadline){
    if(ids.every(id=>app.views.has(id)))return true;
    await new Promise(resolve=>setTimeout(resolve,50));
  }
  return ids.every(id=>app.views.has(id));
}

function restoreTabOrder(ids){
  if(!tabsHost)return;
  for(const id of ids){
    const tab=app.views.get(id)?.tab;
    if(tab)tabsHost.append(tab);
  }
}

function restoredGroups(restored,ids){
  const raw=Array.isArray(restored?.splits)&&restored.splits.length?restored.splits:(restored?.split?[restored.split]:[]);
  const groups=[];const used=new Set();
  for(const split of raw){
    if(!Number.isInteger(split?.left)||!Number.isInteger(split?.right)||split.left===split.right)continue;
    const first=ids[split.left],second=ids[split.right];
    if(!first||!second||used.has(first)||used.has(second))continue;
    used.add(first);used.add(second);
    groups.push({first,second,ratio:Number(split.ratio)||0.5,orientation:split.orientation==='horizontal'?'horizontal':'vertical'});
  }
  return groups;
}

async function restoreProjectTerminals(){
  try{
    const live=await app.jsonFetch('/api/sessions');
    const existing=(live.sessions||[]).filter(meta=>meta.task_id===0&&meta.status==='running');
    if(existing.length){
      const ids=existing.map(meta=>meta.id).filter(Boolean);
      await waitForViews(ids);
      restoring=false;scheduleSave(250);return;
    }

    const saved=await app.jsonFetch(endpoint);
    if(!Array.isArray(saved.terminals)||saved.terminals.length===0){
      globalThis.TaskMenuSplit?.clearAll?.();
      restoring=false;return;
    }

    const restored=await app.jsonFetch(endpoint,{method:'POST'});
    const metas=Array.isArray(restored.sessions)?restored.sessions:[];
    const ids=metas.map(meta=>meta.id).filter(Boolean);
    if(!ids.length){restoring=false;return;}
    const ready=await waitForViews(ids);
    if(!ready)throw new Error('Timed out waiting for restored terminal tabs');

    restoreTabOrder(ids);
    let activeIndex=Number(restored.active_index);
    if(!Number.isInteger(activeIndex)||activeIndex<0||activeIndex>=ids.length)activeIndex=0;
    const activeID=ids[activeIndex];
    if(activeID)app.activateView(activeID);

    const groups=restoredGroups(restored,ids);
    if(groups.length)globalThis.TaskMenuSplit?.restoreProjectGroups?.(groups);
    else globalThis.TaskMenuSplit?.clearAll?.();
    if(activeID){app.activateView(activeID);setTimeout(()=>globalThis.TaskMenuSplit?.syncForActive?.(),0);}
    if(Array.isArray(restored.warnings)&&restored.warnings.length)console.warn('Terminal restore:',...restored.warnings);
  }catch(e){
    console.warn('Cannot restore terminal project state',e);
  }finally{
    restoring=false;scheduleSave(500);
  }
}

window.addEventListener('taskmenu:session',event=>{
  if(event.detail?.meta?.task_id===0)scheduleSave(350);
});
window.addEventListener('taskmenu:output',event=>{
  if(event.detail?.view?.meta?.task_id===0)scheduleSave(700);
});
window.addEventListener('taskmenu:split-changed',()=>scheduleSave(150));

document.addEventListener('click',event=>{
  const target=event.target instanceof Element?event.target:null;
  if(target?.closest('#tabs .tab'))setTimeout(()=>scheduleSave(100),0);
});

if(tabsHost){
  const observer=new MutationObserver(()=>scheduleSave(250));
  observer.observe(tabsHost,{childList:true});
}

setInterval(()=>{if(!restoring)persistSnapshot();},2000);
window.addEventListener('pagehide',()=>{
  if(restoring)return;
  const payload=snapshotPayload();
  try{fetch(endpoint,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),keepalive:true});}catch{}
});

setTimeout(()=>restoreProjectTerminals(),0);
