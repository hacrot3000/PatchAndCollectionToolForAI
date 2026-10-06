const app=globalThis.TaskMenuApp;
const editor=globalThis.TaskMenuEditor;
const explorer=globalThis.TaskMenuExplorer;
if(!app||!editor||!explorer)throw new Error('Editor/Explorer unavailable for file watcher');

const FILE_POLL_MS=1500;
const DIRECTORY_POLL_MS=2500;
const MAX_WATCHED_DIRECTORIES=30;

const fileSignatures=new Map();
const directorySignatures=new Map();
const directoryEntries=new Map();
const busyFiles=new Set();
const busyDirectories=new Set();
let fileTimer=0;
let directoryTimer=0;
let enabled=true;

function fileSignature(meta){
  return String(meta?.mtime_ns??'')+':'+String(meta?.size??'');
}
function directorySignature(items){
  return (Array.isArray(items)?items:[]).map(item=>[
    String(item?.name||''),String(item?.type||''),String(item?.size??''),String(item?.modified||'')
  ].join('\u0000')).sort().join('\u0001');
}
function directoryNames(items){
  return new Set((Array.isArray(items)?items:[]).map(item=>String(item?.name||'')).filter(Boolean));
}
function joinPath(parent,name){
  parent=String(parent||'').replace(/\/+$/,'');
  name=String(name||'').replace(/^\/+/,'');
  return parent?parent+'/'+name:name;
}
async function pollEditor(view){
  if(!view||view.closed||!view.file?.path||view.file?.remote_workspace_id||view.saving)return;
  const path=String(view.file.path);
  if(busyFiles.has(path))return;
  busyFiles.add(path);
  try{
    const meta=await app.jsonFetch('/api/project/file?meta=1&path='+encodeURIComponent(path),{cache:'no-store'});
    const signature=fileSignature(meta);
    const previous=fileSignatures.get(path)||fileSignature(view.file);
    fileSignatures.set(path,signature);
    if(previous&&signature!==previous){
      const latest=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(path),{cache:'no-store'});
      if(String(latest?.sha256||'')!==String(view.file?.sha256||'')){
        await editor.handleExternalFileChange?.(view,latest);
      }
      fileSignatures.set(path,fileSignature(latest));
    }
  }catch(error){
    const message=String(error?.message||error);
    if(/404|not found|unavailable/i.test(message)){
      if(view.externalMissingNotified!==true){
        view.externalMissingNotified=true;
        window.dispatchEvent(new CustomEvent('taskmenu:editor-external-missing',{detail:{path}}));
      }
    }else console.warn('File watcher editor poll failed',path,error);
  }finally{busyFiles.delete(path);}
}
async function pollEditors(){
  if(!enabled||document.hidden)return;
  const activePaths=new Set();
  for(const view of editor.editors?.values?.()||[]){
    if(view?.closed||!view.file?.path||view.file?.remote_workspace_id)continue;
    const path=String(view.file.path);activePaths.add(path);
    if(!fileSignatures.has(path))fileSignatures.set(path,fileSignature(view.file));
    await pollEditor(view);
  }
  for(const path of [...fileSignatures.keys()])if(!activePaths.has(path))fileSignatures.delete(path);
}
async function pollDirectory(path){
  path=String(path||'');
  if(busyDirectories.has(path))return;
  busyDirectories.add(path);
  try{
    const items=await app.jsonFetch('/api/project/tree?path='+encodeURIComponent(path),{cache:'no-store'});
    const safe=Array.isArray(items)?items:[];
    const signature=directorySignature(safe);
    if(!directorySignatures.has(path)){
      directorySignatures.set(path,signature);
      directoryEntries.set(path,safe);
      return;
    }
    const previous=directorySignatures.get(path);
    if(signature===previous)return;
    const before=directoryNames(directoryEntries.get(path));
    const after=directoryNames(safe);
    const added=[...after].filter(name=>!before.has(name)).map(name=>joinPath(path,name));
    const removed=[...before].filter(name=>!after.has(name)).map(name=>joinPath(path,name));
    directorySignatures.set(path,signature);directoryEntries.set(path,safe);
    await explorer.refreshWatchedDirectory?.(path);
    window.dispatchEvent(new CustomEvent('taskmenu:project-directory-changed',{detail:{path,added,removed,generated:added}}));
  }catch(error){
    console.warn('File watcher directory poll failed',path,error);
  }finally{busyDirectories.delete(path);}
}
async function pollDirectories(){
  if(!enabled||document.hidden)return;
  const paths=['',...(explorer.expandedPaths||[])].slice(0,MAX_WATCHED_DIRECTORIES);
  const wanted=new Set(paths);
  for(const path of paths)await pollDirectory(path);
  for(const path of [...directorySignatures.keys()])if(!wanted.has(path)){directorySignatures.delete(path);directoryEntries.delete(path);}
}
function start(){
  if(fileTimer||directoryTimer)return;
  enabled=true;
  fileTimer=setInterval(()=>pollEditors().catch(error=>console.warn('File watcher poll failed',error)),FILE_POLL_MS);
  directoryTimer=setInterval(()=>pollDirectories().catch(error=>console.warn('Directory watcher poll failed',error)),DIRECTORY_POLL_MS);
  pollEditors().catch(()=>{});pollDirectories().catch(()=>{});
}
function stop(){
  enabled=false;
  clearInterval(fileTimer);clearInterval(directoryTimer);fileTimer=0;directoryTimer=0;
}
function refreshBaselines(){
  fileSignatures.clear();directorySignatures.clear();directoryEntries.clear();
  for(const view of editor.editors?.values?.()||[])if(view?.file?.path&&!view.file?.remote_workspace_id)fileSignatures.set(String(view.file.path),fileSignature(view.file));
}
document.addEventListener('visibilitychange',()=>{
  if(document.hidden)return;
  refreshBaselines();pollEditors().catch(()=>{});pollDirectories().catch(()=>{});
});
window.addEventListener('taskmenu:editor-external-reload',event=>{
  const path=String(event.detail?.path||'');const view=[...(editor.editors?.values?.()||[])].find(item=>String(item?.file?.path||'')===path);
  if(view)fileSignatures.set(path,fileSignature(view.file));
});
window.addEventListener('taskmenu:project-file-opened',event=>{
  const path=String(event.detail?.path||'');const view=[...(editor.editors?.values?.()||[])].find(item=>String(item?.file?.path||'')===path);
  if(view)fileSignatures.set(path,fileSignature(view.file));
});

globalThis.TaskMenuFileWatcher={
  start,stop,pollEditors,pollDirectories,refreshBaselines,
  get enabled(){return enabled;},
  get fileIntervalMS(){return FILE_POLL_MS;},
  get directoryIntervalMS(){return DIRECTORY_POLL_MS;}
};
start();
