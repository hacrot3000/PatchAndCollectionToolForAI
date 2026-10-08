// Unified workspace-tab lifecycle. Feature modules restore their own content;
// this layer restores tab visibility, semantic ordering and active selection.
const app=globalThis.TaskMenuApp;
const tabsHost=document.querySelector('#tabs');
if(!app||!tabsHost)throw new Error('Workspace tab restore requires app and #tabs');
const VERSION=1;
let restoring=true,saveTimer=0,savedAtStartup=null;
let lastCapture='',lastActivation='';
function key(){
  const owner=String(app.currentUser?.user_id||'local');
  return 'taskdeck:workspace-tabs:v1:'+[app.taskData?.workspace||'',app.layoutProfile||'desktop',owner].map(encodeURIComponent).join(':');
}
function visibleTabs(){return [...tabsHost.children].filter(tab=>tab instanceof HTMLElement&&tab.dataset.id&&!tab.hidden);}
function isTerminal(view){return Boolean(view?.meta?.task_id===0);}
function terminalOrdinalMap(){
  const ids=[...app.views.values()].filter(isTerminal).map(view=>view.meta.id);
  const ordered=globalThis.TaskMenuTerminalRestore?.snapshotPayload?.()?.session_ids||[];
  const sorted=[...new Set([...ordered,...ids])];
  return new Map(sorted.map((id,index)=>[String(id),index]));
}
function descriptor(tab,ordinals){
  const id=String(tab?.dataset?.id||'');
  if(!id||tab.hidden)return null;
  const editor=globalThis.TaskMenuEditor?.editors?.get?.(id);
  if(editor?.file)return {kind:'editor',path:String(editor.file.path||''),id};
  if(tab.classList.contains('file-compare-tab'))return {kind:'compare'};
  if(tab.classList.contains('task-patch-tab'))return {kind:'patch'};
  if(tab.classList.contains('hex-tab'))return {kind:'hex',path:String(globalThis.TaskMenuHexViewer?.views?.get?.(id)?.path||'')};
  if(tab.classList.contains('ft-tab'))return {kind:'transfer',profileID:id.replace(/^file-transfer:/,'')};
  const db=globalThis.TaskMenuDatabase?.views?.get?.(id);
  if(db)return {kind:'database',id,profileID:String(db.meta?.profile_id||'')};
  const terminal=app.views.get(id);
  if(terminal){
    if(isTerminal(terminal))return {kind:'terminal',index:ordinals.get(id)??0,id,label:String(terminal.meta?.label||'')};
    return {kind:'session',id,label:String(terminal.meta?.label||'')};
  }
  return {kind:'other',id};
}
function matches(want,actual){
  if(!want||!actual||want.kind!==actual.kind)return false;
  switch(want.kind){
    case 'editor':return want.path===actual.path;
    case 'hex':return want.path===actual.path;
    case 'transfer':return want.profileID===actual.profileID;
    case 'terminal':return want.id===actual.id||(Number(want.index)===Number(actual.index));
    case 'session':return want.id===actual.id;
    case 'database':return want.id===actual.id||(want.profileID===actual.profileID&&want.ordinal===actual.ordinal);
    case 'other':return want.id===actual.id;
    default:return true;
  }
}
function describeTabs(){
  const ordinals=terminalOrdinalMap(),dbOrdinals=new Map();
  return visibleTabs().map(tab=>{
    const info=descriptor(tab,ordinals);
    if(info?.kind==='database'){
      info.ordinal=dbOrdinals.get(info.profileID)||0;
      dbOrdinals.set(info.profileID,info.ordinal+1);
    }
    return {tab,info};
  }).filter(row=>row.info);
}
function applySemanticOrder(order){
  if(!Array.isArray(order)||!order.length)return false;
  const live=describeTabs(),unused=new Set(live),result=[];
  for(const saved of order){
    let match=[...unused].find(row=>matches(saved,row.info));
    if(!match)continue;
    unused.delete(match);result.push(match);
  }
  result.push(...unused);
  if(result.every((row,index)=>row===live[index]))return false;
  for(const row of result)tabsHost.append(row.tab);
  return true;
}
function read(){
  try{const data=JSON.parse(localStorage.getItem(key())||'null');return data?.version===VERSION&&Array.isArray(data.order)?data:null;}
  catch(error){console.warn('Workspace tab state unreadable',error);return null;}
}
function capture(){
  const rows=describeTabs(),active=rows.find(row=>row.tab.classList.contains('active'))?.info||null;
  return {version:VERSION,order:rows.map(row=>row.info),active,
    compare:globalThis.TaskMenuFileCompare?.snapshotState?.()||null,
    hex:globalThis.TaskMenuHexViewer?.snapshotState?.()||[],
    patchOpen:Boolean(globalThis.TaskMenuPatchPanel?.tab&&!globalThis.TaskMenuPatchPanel.tab.hidden),
    updatedAt:Date.now()};
}
function saveNow(){
  if(restoring||!app.taskData?.workspace)return false;
  clearTimeout(saveTimer);saveTimer=0;
  try{
    const state=capture(),content=JSON.stringify(state);
    if(content!==lastCapture){localStorage.setItem(key(),content);lastCapture=content;}
    return true;
  }catch(error){console.warn('Workspace tab state could not be saved',error);return false;}
}
function scheduleSave(){
  if(restoring)return;
  clearTimeout(saveTimer);saveTimer=setTimeout(saveNow,120);
}
function findLiveTab(wanted){
  if(!wanted)return null;
  return describeTabs().find(row=>matches(wanted,row.info))?.tab||null;
}
async function restoreActive(wanted){
  const tab=findLiveTab(wanted);
  if(!tab)return false;
  tab.click();return true;
}
async function restoreStartup(){
  const saved=read();savedAtStartup=saved;
  try{
    await Promise.allSettled([
      globalThis.TaskMenuTerminalRestore?.ready,
      globalThis.TaskMenuEditor?.ready,
      globalThis.TaskMenuDatabase?.ready,
      globalThis.TaskMenuFileTransfer?.ready
    ]);
    // Restore feature tabs after their underlying file/session providers are ready.
    if(saved?.hex?.length)await globalThis.TaskMenuHexViewer?.restoreState?.(saved.hex);
    if(saved?.compare){
      try{await globalThis.TaskMenuFileCompare?.restoreState?.(saved.compare);}
      catch(error){console.warn('Workspace File Compare restore skipped',error);}
    }
    if(saved?.patchOpen)globalThis.TaskMenuPatchPanel?.open?.();
    if(saved?.order)applySemanticOrder(saved.order);
    if(saved?.active)await restoreActive(saved.active);
  }catch(error){console.warn('Workspace tabs restore failed',error);}
  finally{
    restoring=false;lastCapture='';
    saveNow();
    window.dispatchEvent(new CustomEvent('taskmenu:workspace-tabs-restored'));
  }
}
const tabObserver=new MutationObserver(records=>{
  if(restoring)return;
  const changed=records.some(record=>
    [...record.addedNodes,...record.removedNodes].some(node=>node instanceof HTMLElement&&node.dataset.id));
  if(changed)scheduleSave();
});
tabObserver.observe(tabsHost,{childList:true});
tabsHost.addEventListener('click',()=>scheduleSave());
window.addEventListener('taskmenu:view-activated',()=>scheduleSave());
window.addEventListener('taskmenu:patch-panel-visible',()=>scheduleSave());
window.addEventListener('taskmenu:workspace-tab-changed',()=>scheduleSave());
window.addEventListener('pagehide',()=>saveNow());
globalThis.TaskMenuWorkspaceTabs={
  saveNow,scheduleSave,applySemanticOrder,capture,restoreStartup,
  get restoring(){return restoring;},get savedAtStartup(){return savedAtStartup;}
};
if(app.taskData?.workspace)setTimeout(restoreStartup,0);
else window.addEventListener('taskmenu:tasks',()=>restoreStartup(),{once:true});
