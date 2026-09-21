const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal restore');

const endpoint='/api/state/tasks?scope=terminals&profile='+encodeURIComponent(app.layoutProfile||'desktop');
const tabsHost=document.querySelector('#tabs');
let restoring=true;
let persistenceFrozen=false;
let saveTimer=null;
let saveInFlight=Promise.resolve();
let resolveRestoreReady;
const restoreReady=new Promise(resolve=>{resolveRestoreReady=resolve;});

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
  if(restoring||persistenceFrozen)return Promise.resolve();
  const payload=snapshotPayload();
  saveInFlight=saveInFlight.catch(()=>{}).then(()=>app.jsonFetch(endpoint,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)})).catch(e=>console.warn('Cannot persist terminal project state',e));
  return saveInFlight;
}

async function freezeForSelfUpdate(){
  await restoreReady;
  if(persistenceFrozen){await saveInFlight.catch(()=>{});return;}
  persistenceFrozen=true;
  clearTimeout(saveTimer);
  await saveInFlight.catch(()=>{});
  const payload=snapshotPayload();
  await app.jsonFetch(endpoint,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});
}

function resumeAfterSelfUpdate(){
  if(!persistenceFrozen)return;
  persistenceFrozen=false;
  if(!restoring)scheduleSave(100);
}

function scheduleSave(delay=500){
  if(restoring||persistenceFrozen)return;
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

function normalizedCwd(value){
  const text=String(value||'').trim();
  return text.length>1?text.replace(/[\\/]+$/,''):text;
}

function restoredSessionIDAt(saved,index,ids,exclude=new Set()){
  const item=saved?.terminals?.[index];
  const fromState=String(item?.session_id||'').trim();
  if(fromState&&ids.includes(fromState)&&!exclude.has(fromState))return fromState;
  const wantedCwd=normalizedCwd(item?.cwd);
  if(wantedCwd){
    for(const id of ids){
      if(exclude.has(id))continue;
      const liveCwd=normalizedCwd(app.views.get(id)?.meta?.cwd);
      if(liveCwd&&liveCwd===wantedCwd)return id;
    }
  }
  if(item)return '';
  const fallback=ids[index]||'';
  return fallback&&!exclude.has(fallback)?fallback:'';
}

function restoredGroups(restored,ids){
  if(app.layoutProfile==='mobile')return [];
  const raw=Array.isArray(restored?.splits)&&restored.splits.length?restored.splits:(restored?.split?[restored.split]:[]);
  const groups=[];const used=new Set();
  for(const split of raw){
    if(!Number.isInteger(split?.left)||!Number.isInteger(split?.right)||split.left===split.right)continue;
    const first=restoredSessionIDAt(restored,split.left,ids,used);
    const reserved=new Set(used);if(first)reserved.add(first);
    const second=restoredSessionIDAt(restored,split.right,ids,reserved);
    if(!first||!second||first===second)continue;
    used.add(first);used.add(second);
    groups.push({first,second,ratio:Number(split.ratio)||0.5,orientation:split.orientation==='horizontal'?'horizontal':'vertical'});
  }
  return groups;
}

function savedActiveSessionID(saved,ids){
  let activeIndex=Number(saved?.active_index);
  if(!Number.isInteger(activeIndex)||activeIndex<0)activeIndex=0;
  const candidate=restoredSessionIDAt(saved,activeIndex,ids);
  return ids.includes(candidate)?candidate:(ids[0]||'');
}

function applySavedLayout(saved,ids,{clearMissing=true}={}){
  if(!ids.length)return;
  restoreTabOrder(ids);
  const groups=restoredGroups(saved,ids);
  if(app.layoutProfile==='mobile'){
    globalThis.TaskMenuSplit?.clearPresentation?.();
  }else if(groups.length)globalThis.TaskMenuSplit?.restoreProjectGroups?.(groups);
  else if(clearMissing)globalThis.TaskMenuSplit?.clearAll?.();
  const activeID=savedActiveSessionID(saved,ids);
  if(activeID){
    app.activateView(activeID);
    setTimeout(()=>globalThis.TaskMenuSplit?.syncForActive?.(),0);
  }
}

function layoutIDsFromSaved(saved,existing){
  const metas=existing.filter(meta=>String(meta?.id||'').trim());
  if(!Array.isArray(saved?.terminals)||!saved.terminals.length)return [];
  const byID=new Map(metas.map(meta=>[String(meta.id),meta]));
  const used=new Set();const ordered=[];
  for(const item of saved.terminals){
    const oldID=String(item?.session_id||'').trim();
    let match=oldID&&!used.has(oldID)?byID.get(oldID):null;
    if(!match){
      const wantedCwd=normalizedCwd(item?.cwd);
      if(wantedCwd)match=metas.find(meta=>!used.has(String(meta.id))&&normalizedCwd(meta.cwd)===wantedCwd)||null;
    }
    if(match){
      const id=String(match.id);used.add(id);ordered.push(id);
    }
  }
  if(!ordered.length)return [];
  for(const meta of metas){
    const id=String(meta.id);if(!used.has(id)){used.add(id);ordered.push(id);}
  }
  return ordered;
}

async function restoreProjectTerminals(){
  try{
    const saved=await app.jsonFetch(endpoint);
    const live=await app.jsonFetch('/api/sessions');
    const existing=(live.sessions||[]).filter(meta=>meta.task_id===0&&meta.status==='running');

    if(existing.length){
      await app.syncSessions?.();
      const savedIDs=layoutIDsFromSaved(saved,existing);
      const ids=savedIDs.length?savedIDs:existing.map(meta=>meta.id).filter(Boolean);
      const ready=await waitForViews(ids);
      if(!ready)throw new Error('Timed out waiting for live terminal tabs');
      if(savedIDs.length){
        applySavedLayout(saved,ids,{clearMissing:true});
      }else{
        // Legacy v1/v2 state has no stable session ids. Keep any split restored
        // by split.js/sessionStorage, then persist v3 ids once startup settles.
        restoreTabOrder(ids);
        globalThis.TaskMenuSplit?.syncForActive?.();
      }
      return;
    }

    if(!Array.isArray(saved.terminals)||saved.terminals.length===0){
      globalThis.TaskMenuSplit?.clearAll?.();
      return;
    }

    const restored=await app.jsonFetch(endpoint,{method:'POST'});
    const metas=Array.isArray(restored.sessions)?restored.sessions:[];
    for(const meta of metas)app.attachSession?.(meta,false);
    await app.syncSessions?.();
    const ids=metas.map(meta=>meta.id).filter(Boolean);
    if(!ids.length)return;
    const ready=await waitForViews(ids);
    if(!ready)throw new Error('Timed out waiting for restored terminal tabs');

    applySavedLayout(restored,ids,{clearMissing:true});
    if(Array.isArray(restored.warnings)&&restored.warnings.length)console.warn('Terminal restore:',...restored.warnings);
  }catch(e){
    console.warn('Cannot restore terminal project state',e);
  }finally{
    restoring=false;
    resolveRestoreReady?.();
    resolveRestoreReady=null;
    if(!persistenceFrozen)scheduleSave(500);
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

setInterval(()=>{if(!restoring&&!persistenceFrozen)persistSnapshot();},2000);
window.addEventListener('pagehide',()=>{
  if(restoring||persistenceFrozen)return;
  const payload=snapshotPayload();
  try{app.fetchWithLease(endpoint,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),keepalive:true});}catch{}
});

globalThis.TaskMenuTerminalRestore={persistSnapshot,snapshotPayload,freezeForSelfUpdate,resumeAfterSelfUpdate,isPersistenceFrozen:()=>persistenceFrozen,ready:restoreReady};
setTimeout(()=>restoreProjectTerminals(),0);
