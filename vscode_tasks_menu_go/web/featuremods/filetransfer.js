const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for file transfer workspace');

const views=new Map();
let profilesByID=new Map();
let contextMenu=null;
let activeViewID='';
let restoringSession=false;
let sessionRestoreStarted=false;
let sessionPersistenceReady=false;

const localDBName='TaskDeckFileTransfer';
const localDBStore='localRoots';
const localDBVersion=1;
const maxRecentPaths=50;
const maxFavoritePaths=20;

const style=document.createElement('style');
style.textContent=`
.ft-tab{border-radius:6px 6px 0 0;border-bottom:0;margin-left:4px}
.ft-tab.active{background:#343b48}.ft-tab .close{margin-left:8px}
.ft-pane{position:absolute;inset:0;display:flex;flex-direction:column;background:#0d1015}
.ft-pane.hidden{display:none}.ft-pane-head{height:38px;display:flex;align-items:center;gap:8px;padding:5px 9px;border-bottom:1px solid #30343b}
.ft-pane-title{font-weight:600;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ft-pane-meta{font-size:10px;opacity:.55}.ft-pane-spacer{flex:1}
.ft-sites{flex:1;min-height:0;display:grid;grid-template-columns:minmax(260px,1fr) 8px minmax(260px,1fr)}
.ft-site{min-width:0;min-height:0;display:flex;flex-direction:column}
.ft-site-head{display:flex;align-items:center;gap:6px;padding:6px 7px;border-bottom:1px solid #30343b;background:#11151b}
.ft-site-title{font-size:11px;font-weight:700;white-space:nowrap}
.ft-source-select,.ft-root-select{height:28px;min-height:28px;padding:3px 6px;font-size:11px}
.ft-root-select{max-width:210px}
.ft-path-history-select{width:30px;min-width:30px;height:28px;padding:0 2px;border:1px solid #3b414d;border-radius:5px;background:#161b22;color:inherit;font-size:11px;text-align:center;appearance:none;-webkit-appearance:none;cursor:pointer}
.ft-path-history-select option,.ft-path-history-select optgroup{font-size:11px;text-align:left}
.ft-pathbar{display:flex;align-items:center;gap:5px;padding:6px 7px;border-bottom:1px solid #30343b;background:#11151b}
.ft-pathbar input[type=text]{flex:1;min-width:80px;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:6px 8px;font:11px ui-monospace,monospace}
.ft-pathbar button,.ft-site-head button{height:28px;padding:3px 7px;font-size:11px}
.ft-favorite-toggle.saved{background:#554817;border-color:#8c7731}
.ft-status{padding:5px 8px;border-bottom:1px solid #272d36;font-size:10px;opacity:.7;min-height:24px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.ft-table-wrap{flex:1;min-height:0;overflow:auto}
.ft-table{width:100%;border-collapse:collapse;font-size:11px;table-layout:auto}
.ft-table th,.ft-table td{padding:6px 8px;border-bottom:1px solid #272d36;text-align:left;white-space:nowrap}
.ft-table th{position:sticky;top:0;background:#171c23;z-index:2;font-size:10px;opacity:.86;cursor:pointer;user-select:none}
.ft-table th.ft-nosort{cursor:default}.ft-table th .sort{margin-left:4px;opacity:.7}
.ft-table tbody tr{cursor:default}.ft-table tbody tr:hover{background:#202731}.ft-table tbody tr.selected{background:#29384b}
.ft-table tbody tr,.ft-table tbody td{user-select:none;-moz-user-select:none;-webkit-user-select:none}
.ft-table tbody tr:focus,.ft-table tbody tr:focus-visible,.ft-table tbody tr:focus-within,.ft-table tbody tr:active,.ft-table tbody td:focus,.ft-table tbody td:focus-visible,.ft-table tbody td:active,.ft-table tbody tr.selected,.ft-table tbody tr.selected td{outline:none!important;box-shadow:none!important}
.ft-table tbody tr.selected td,.ft-table tbody tr:focus-within td,.ft-table tbody tr:active td,.ft-table tbody td:focus,.ft-table tbody td:focus-visible,.ft-table tbody td:active{border-top:0!important;border-left:0!important;border-right:0!important}
.ft-parent-row{font-weight:600}.ft-parent-row td{background:#11161d}.ft-parent-row:hover td{background:#202731!important}
.ft-name{max-width:480px;overflow:hidden;text-overflow:ellipsis}.ft-kind{display:inline-block;min-width:17px;margin-right:5px;opacity:.78}
.ft-size{text-align:right!important;font-family:ui-monospace,monospace}.ft-type{opacity:.72}.ft-modified{font-family:ui-monospace,monospace;font-size:10px}
.ft-empty{padding:20px;opacity:.55}
.ft-queue{height:170px;min-height:90px;display:flex;flex-direction:column;border-top:0;background:#0f1319}
.ft-queue-resizer{height:8px;min-height:8px;flex:0 0 8px;cursor:row-resize;touch-action:none;background:#171b22;border-top:1px solid #30343b;border-bottom:1px solid #30343b;position:relative}
.ft-queue-resizer::after{content:'';position:absolute;left:50%;top:3px;width:42px;height:2px;transform:translateX(-50%);border-radius:2px;background:#56606e;opacity:.75}
.ft-queue-resizer:hover,.ft-queue-resizer.dragging{background:#26303c}
.ft-queue-head{display:flex;align-items:center;gap:7px;padding:5px 8px;border-bottom:1px solid #30343b;background:#141920}
.ft-queue-title{font-size:11px;font-weight:700}.ft-queue-summary{font-size:10px;opacity:.68}.ft-queue-spacer{flex:1}
.ft-queue-head button{height:25px;padding:2px 7px;font-size:10px}
.ft-queue-wrap{flex:1;min-height:0;overflow:auto}
.ft-queue-table{width:100%;border-collapse:collapse;font-size:10px}
.ft-queue-table th,.ft-queue-table td{padding:5px 7px;border-bottom:1px solid #272d36;text-align:left;white-space:nowrap}
.ft-queue-table th{position:sticky;top:0;background:#171c23;z-index:2}
.ft-queue-table tbody tr{cursor:default;user-select:none;-moz-user-select:none;-webkit-user-select:none}
.ft-queue-table tbody tr:hover{background:#202731}.ft-queue-table tbody tr.selected{background:#29384b}
.ft-queue-table tbody tr:focus,.ft-queue-table tbody tr:focus-visible,.ft-queue-table tbody td:focus,.ft-queue-table tbody td:focus-visible{outline:none!important;box-shadow:none!important}
.ft-queue-select{width:26px;min-width:26px;text-align:center!important;padding-left:4px!important;padding-right:4px!important}
.ft-queue-select input{margin:0;vertical-align:middle}
.ft-queue-kind{font-weight:600;min-width:72px}
.ft-queue-path{max-width:420px;overflow:hidden;text-overflow:ellipsis}
.ft-queue-status{font-weight:600}.ft-queue-status.running{opacity:1}.ft-queue-status.success{opacity:.72}.ft-queue-status.failed{font-weight:700}
.ft-queue-error{max-width:440px;overflow:hidden;text-overflow:ellipsis;opacity:.82}
.ft-queue-empty{padding:14px;opacity:.5}
.ft-divider{position:relative;cursor:col-resize;background:#171b22;border-left:1px solid #30343b;border-right:1px solid #30343b;touch-action:none}
.ft-divider:hover,.ft-divider.dragging{background:#26303c}
.ft-transfer-tools{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);display:flex;flex-direction:column;gap:8px;z-index:4}
.ft-transfer-tools button{width:34px;height:34px;padding:0;border-radius:50%;font-size:17px;background:#202a36;box-shadow:0 2px 7px rgba(0,0,0,.35)}
.ft-transfer-tools button:disabled{opacity:.28}
body.ft-resizing{user-select:none;cursor:col-resize}
body.ft-queue-resizing{user-select:none;cursor:row-resize}
.ft-context{position:fixed;z-index:16000;min-width:175px;padding:4px;background:#171b22;border:1px solid #48515f;border-radius:7px;box-shadow:0 14px 38px rgba(0,0,0,.45)}
.ft-context-title{padding:6px 9px 5px;font-size:10px;font-weight:700;opacity:.65;max-width:280px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ft-context-separator{height:1px;margin:4px 3px;background:#303844}
.ft-context button{display:block;width:100%;text-align:left;border:0;background:transparent;color:inherit;padding:7px 9px;border-radius:4px}
.ft-context button:hover{background:#2b3440}.ft-context button.danger{color:#ff9a9a}
.ft-context button:disabled{opacity:.4;pointer-events:none}
.ft-select-cell{width:26px;min-width:26px;text-align:center!important;padding-left:5px!important;padding-right:3px!important}
.ft-select-cell input{margin:0;vertical-align:middle}
.ft-local-note{font-size:10px;opacity:.65}
html[data-taskmenu-theme="light"] .ft-pane{background:#fff}
html[data-taskmenu-theme="light"] .ft-site-head,html[data-taskmenu-theme="light"] .ft-pathbar{background:#f2f5f8;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-pathbar input[type=text],html[data-taskmenu-theme="light"] .ft-path-history-select{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-table th{background:#e9eef3}
html[data-taskmenu-theme="light"] .ft-table tbody tr:hover{background:#eef2f6}
html[data-taskmenu-theme="light"] .ft-table tbody tr.selected{background:#dde8f3}
html[data-taskmenu-theme="light"] .ft-context{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-queue{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-queue-resizer{background:#e3e8ed;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-queue-head,html[data-taskmenu-theme="light"] .ft-queue-table th{background:#edf1f5;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-queue-table tbody tr:hover{background:#eef2f6}
html[data-taskmenu-theme="light"] .ft-queue-table tbody tr.selected{background:#dde8f3}
@media(max-width:850px){.ft-sites{grid-template-columns:1fr;grid-template-rows:minmax(220px,1fr) 8px minmax(220px,1fr)}.ft-divider{cursor:row-resize;border-left:0;border-right:0;border-top:1px solid #30343b;border-bottom:1px solid #30343b}.ft-transfer-tools{flex-direction:row}.ft-transfer-tools button:first-child{transform:rotate(90deg)}.ft-transfer-tools button:last-child{transform:rotate(90deg)}}
`;
document.head.append(style);

function workspaceKey(){return String(app.taskData?.workspace||'workspace');}
function safeStorageGet(key,fallback=''){try{return localStorage.getItem(key)??fallback;}catch{return fallback;}}
function safeStorageSet(key,value){try{localStorage.setItem(key,value);}catch(error){console.warn('Cannot persist file-transfer setting',error);}}

function fileTransferWorkspaceID(){return String(app.taskData?.workspace||'').trim();}
function fileTransferSessionKey(){
  const workspace=fileTransferWorkspaceID();
  return workspace?'taskdeck:file-transfer:session:'+workspace:'';
}
function readFileTransferSession(){
  const key=fileTransferSessionKey();if(!key)return null;
  try{
    const raw=JSON.parse(localStorage.getItem(key)||'null');
    if(!raw||raw.version!==1||!Array.isArray(raw.open))return null;
    return {
      version:1,
      active_profile_id:String(raw.active_profile_id||''),
      open:raw.open.map(item=>({
        profile_id:String(item?.profile_id||''),
        left_path:normalizeRelativePath(item?.left_path||'.'),
        remote_path:normalizeRemotePath(item?.remote_path||'.')
      })).filter(item=>item.profile_id)
    };
  }catch{return null;}
}
function persistFileTransferSession(){
  if(restoringSession||!sessionPersistenceReady)return;
  const key=fileTransferSessionKey();if(!key)return;
  const open=[];
  for(const [id,view] of views){
    open.push({
      profile_id:id,
      left_path:normalizeRelativePath(view.left?.currentPath||'.'),
      remote_path:normalizeRemotePath(view.remote?.currentPath||view.profile?.initial_path||'.')
    });
  }
  try{
    if(!open.length){localStorage.removeItem(key);return;}
    localStorage.setItem(key,JSON.stringify({version:1,active_profile_id:activeViewID,open}));
  }catch(error){console.warn('Cannot persist file-transfer session',error);}
}
function normalizeRemotePath(value){
  value=String(value||'.').trim()||'.';
  if(value==='.')return '.';
  const absolute=value.startsWith('/');
  const parts=[];
  for(const item of value.replace(/\\/g,'/').split('/')){
    if(!item||item==='.')continue;
    if(item==='..'){if(parts.length)parts.pop();continue;}
    parts.push(item);
  }
  const joined=parts.join('/');
  if(absolute)return '/'+joined;
  return joined||'.';
}

function normalizeRelativePath(value){
  value=String(value||'.').trim()||'.';
  if(value==='.')return '.';
  const parts=[];
  for(const item of value.replace(/\\/g,'/').split('/')){
    if(!item||item==='.')continue;
    if(item==='..'){if(parts.length)parts.pop();continue;}
    parts.push(item);
  }
  return parts.join('/')||'.';
}

function joinPath(parent,name,remote=false){
  parent=remote?normalizeRemotePath(parent):normalizeRelativePath(parent);
  name=String(name||'').replace(/^[/\\]+/,'');
  if(parent==='.')return name||'.';
  if(remote&&parent==='/')return '/'+name;
  return parent.replace(/\/$/,'')+'/'+name;
}

function parentPath(value,remote=false){
  value=remote?normalizeRemotePath(value):normalizeRelativePath(value);
  if(value==='.'||(remote&&value==='/'))return value;
  const absolute=remote&&value.startsWith('/');
  const parts=value.split('/').filter(Boolean);parts.pop();
  if(!parts.length)return absolute?'/':'.';
  return (absolute?'/':'')+parts.join('/');
}
function canGoParentPath(value,remote=false){
  const current=remote?normalizeRemotePath(value):normalizeRelativePath(value);
  return parentPath(current,remote)!==current;
}

function formatSize(value){
  let n=Number(value)||0;if(n<1024)return n+' B';
  const units=['KiB','MiB','GiB','TiB'];let i=-1;
  do{n/=1024;i++;}while(n>=1024&&i<units.length-1);
  return n.toFixed(n>=10?1:2)+' '+units[i];
}

function formatModified(value){
  if(!value)return '';
  const date=new Date(value);
  if(Number.isNaN(date.getTime()))return String(value);
  return new Intl.DateTimeFormat(undefined,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(date);
}

function entryType(entry){
  const raw=String(entry?.type||'file').toLowerCase();
  if(raw==='dir')return 'directory';
  return raw;
}

function typeRank(type){
  switch(entryType({type})){case'directory':return 0;case'file':return 1;case'symlink':return 2;default:return 3;}
}

function typeLabel(type){
  switch(entryType({type})){case'directory':return 'Folder';case'file':return 'File';case'symlink':return 'Link';default:return String(type||'Other');}
}

function sortEntries(entries,sort){
  const copy=[...(entries||[])];
  copy.sort((a,b)=>{
    const at=typeRank(a.type),bt=typeRank(b.type);
    if(sort.key==='type'&&at!==bt)return (at-bt)*(sort.direction==='asc'?1:-1);
    if(sort.key!=='type'&&at!==bt)return at-bt;
    let av,bv;
    switch(sort.key){
      case'size':av=Number(a.size)||0;bv=Number(b.size)||0;break;
      case'modified':av=Date.parse(a.modified||'')||0;bv=Date.parse(b.modified||'')||0;break;
      case'type':av=typeLabel(a.type);bv=typeLabel(b.type);break;
      default:av=String(a.name||'').toLocaleLowerCase();bv=String(b.name||'').toLocaleLowerCase();
    }
    let result=typeof av==='number'?(av-bv):String(av).localeCompare(String(bv),undefined,{numeric:true,sensitivity:'base'});
    if(result===0)result=String(a.name||'').localeCompare(String(b.name||''),undefined,{numeric:true,sensitivity:'base'});
    return result*(sort.direction==='asc'?1:-1);
  });
  return copy;
}

function pathMemoryKey(scope){return 'taskdeck:file-transfer:paths:v2:'+workspaceKey()+':'+scope;}
function readPathMemory(scope){
  try{
    const parsed=JSON.parse(safeStorageGet(pathMemoryKey(scope),'{}'));
    return {
      favorites:Array.isArray(parsed?.favorites)?parsed.favorites.filter(Boolean).slice(0,maxFavoritePaths):[],
      recent:Array.isArray(parsed?.recent)?parsed.recent.filter(Boolean).slice(0,maxRecentPaths):[]
    };
  }catch{return {favorites:[],recent:[]};}
}
function writePathMemory(scope,memory){safeStorageSet(pathMemoryKey(scope),JSON.stringify(memory));}
function rememberPath(scope,path){
  if(!scope||!path)return;
  const memory=readPathMemory(scope);
  memory.recent=[path,...memory.recent.filter(item=>item!==path)].slice(0,maxRecentPaths);
  writePathMemory(scope,memory);
}
function toggleFavorite(scope,path){
  const memory=readPathMemory(scope);
  const index=memory.favorites.indexOf(path);
  if(index>=0)memory.favorites.splice(index,1);
  else memory.favorites=[path,...memory.favorites.filter(item=>item!==path)].slice(0,maxFavoritePaths);
  writePathMemory(scope,memory);
  return index<0;
}
function populatePathMemory(select,star,scope,current){
  const memory=readPathMemory(scope);
  select.replaceChildren();
  const placeholder=document.createElement('option');placeholder.value='';placeholder.textContent='▾';select.append(placeholder);
  const addGroup=(label,values)=>{
    if(!values.length)return;
    const group=document.createElement('optgroup');group.label=label;
    for(const value of values){const option=document.createElement('option');option.value=value;option.textContent=value;group.append(option);}
    select.append(group);
  };
  addGroup('Recent paths',memory.recent);
  addGroup('Favorites',memory.favorites.filter(item=>!memory.recent.includes(item)));
  select.value='';
  select.title=memory.recent.length?'Visited paths · '+memory.recent.length+' remembered':'Visited paths';
  const saved=memory.favorites.includes(current);
  star.classList.toggle('saved',saved);star.textContent=saved?'★':'☆';
  star.title=saved?'Remove current path from favorites':'Pin current path as favorite';
}

function openLocalDB(){
  return new Promise((resolve,reject)=>{
    if(!globalThis.indexedDB){reject(new Error('IndexedDB is unavailable'));return;}
    const request=indexedDB.open(localDBName,localDBVersion);
    request.onupgradeneeded=()=>{const db=request.result;if(!db.objectStoreNames.contains(localDBStore))db.createObjectStore(localDBStore,{keyPath:'id'});};
    request.onsuccess=()=>resolve(request.result);request.onerror=()=>reject(request.error||new Error('Cannot open local directory store'));
  });
}
async function localRootRecords(){
  const db=await openLocalDB();
  return new Promise((resolve,reject)=>{
    const tx=db.transaction(localDBStore,'readonly'),req=tx.objectStore(localDBStore).getAll();
    req.onsuccess=()=>resolve((req.result||[]).sort((a,b)=>(Number(b.lastUsed)||0)-(Number(a.lastUsed)||0)));
    req.onerror=()=>reject(req.error);tx.oncomplete=()=>db.close();
  });
}
async function putLocalRoot(record){
  const db=await openLocalDB();
  return new Promise((resolve,reject)=>{
    const tx=db.transaction(localDBStore,'readwrite');tx.objectStore(localDBStore).put(record);
    tx.oncomplete=()=>{db.close();resolve(record);};tx.onerror=()=>{const err=tx.error;db.close();reject(err);};
  });
}
async function getLocalRoot(id){
  const db=await openLocalDB();
  return new Promise((resolve,reject)=>{
    const tx=db.transaction(localDBStore,'readonly'),req=tx.objectStore(localDBStore).get(id);
    req.onsuccess=()=>resolve(req.result||null);req.onerror=()=>reject(req.error);tx.oncomplete=()=>db.close();
  });
}
function localRootID(){return globalThis.crypto?.randomUUID?.()||('root-'+Date.now()+'-'+Math.random().toString(16).slice(2));}
async function ensureHandlePermission(handle){
  if(!handle)return false;
  const opts={mode:'readwrite'};
  if(typeof handle.queryPermission==='function')return (await handle.queryPermission(opts))==='granted';
  return false;
}
async function requestHandlePermission(handle){
  if(!handle)return false;
  const opts={mode:'readwrite'};
  if(typeof handle.requestPermission==='function')return (await handle.requestPermission(opts))==='granted';
  return ensureHandlePermission(handle);
}
async function directoryHandleForPath(rootHandle,path){
  let handle=rootHandle;
  const normalized=normalizeRelativePath(path);
  if(normalized==='.')return handle;
  for(const part of normalized.split('/'))handle=await handle.getDirectoryHandle(part);
  return handle;
}

async function refreshProfiles(){
  const data=await app.jsonFetch('/api/file-transfer/profiles');
  profilesByID=new Map((Array.isArray(data?.profiles)?data.profiles:[]).map(profile=>[String(profile.id||''),profile]));
  return profilesByID;
}
function profileFor(view){return profilesByID.get(view.profile.id)||view.profile;}

function activateView(id){
  const view=views.get(id);if(!view)return;
  activeViewID=id;
  app.activateExternalView('file-transfer:'+id);
  for(const [otherID,other] of views){
    const active=otherID===id;other.tab.classList.toggle('active',active);other.pane.classList.toggle('hidden',!active);
  }
  persistFileTransferSession();
}
function closeView(id){
  const view=views.get(id);if(!view)return;
  view.tab.remove();view.pane.remove();views.delete(id);
  if(activeViewID===id)activeViewID='';
  persistFileTransferSession();
}
function closeContextMenu(){contextMenu?.remove();contextMenu=null;}
const maxRenderedTransferRows=2000;
function queueStatusLabel(status){
  switch(status){case'queued':return 'Queued';case'running':return 'Running';case'success':return 'Done';case'failed':return 'Failed';default:return String(status||'');}
}
function queueCounts(view){
  const counts={queued:0,running:0,success:0,failed:0};
  for(const item of view.transferQueue?.items||[])if(Object.prototype.hasOwnProperty.call(counts,item.status))counts[item.status]++;
  return counts;
}
function queueSelectedItems(queue){return queue.items.filter(item=>queue.selectedIDs.has(item.id));}
function pruneQueueSelection(queue){
  const live=new Set(queue.items.map(item=>item.id));
  for(const id of [...queue.selectedIDs])if(!live.has(id))queue.selectedIDs.delete(id);
  if(queue.selectionAnchor!==null&&!live.has(queue.selectionAnchor))queue.selectionAnchor=null;
}
function selectQueueItem(queue,item,event={},visible=[]){
  const ids=queue.selectedIDs,key=item.id;
  if(event.shiftKey&&queue.selectionAnchor!==null){
    const from=visible.findIndex(candidate=>candidate.id===queue.selectionAnchor),to=visible.findIndex(candidate=>candidate.id===key);
    if(from>=0&&to>=0){
      if(!(event.ctrlKey||event.metaKey))ids.clear();
      for(let i=Math.min(from,to);i<=Math.max(from,to);i++)ids.add(visible[i].id);
    }else{ids.clear();ids.add(key);}
  }else if(event.ctrlKey||event.metaKey){
    if(ids.has(key))ids.delete(key);else ids.add(key);
    queue.selectionAnchor=key;
  }else{
    ids.clear();ids.add(key);queue.selectionAnchor=key;
  }
}
function toggleQueueCheckbox(queue,item,checked){
  if(checked)queue.selectedIDs.add(item.id);else queue.selectedIDs.delete(item.id);
  queue.selectionAnchor=item.id;
}
function renderTransferQueue(view){
  const queue=view.transferQueue;if(!queue?.body)return;
  pruneQueueSelection(queue);
  const counts=queueCounts(view);
  const scanText=queue.activeScans>0?'Scanning '+queue.activeScans+' · ':'';
  const pauseText=queue.paused?'Paused · ':'';
  let visible=queue.items;
  if(visible.length>maxRenderedTransferRows){
    const active=visible.filter(item=>item.status!=='success');
    visible=active.length>=maxRenderedTransferRows
      ?active.slice(0,maxRenderedTransferRows)
      :active.concat(queue.items.filter(item=>item.status==='success').slice(-(maxRenderedTransferRows-active.length)));
  }
  const shown=visible.length<queue.items.length?' · Showing '+visible.length+'/'+queue.items.length:'';
  queue.summary.textContent=pauseText+scanText+'Queued '+counts.queued+' · Running '+counts.running+' · Done '+counts.success+' · Failed '+counts.failed+shown;
  queue.retry.disabled=counts.failed===0;queue.clear.disabled=counts.success===0;
  queue.body.replaceChildren();
  if(!queue.items.length){
    const tr=document.createElement('tr'),td=document.createElement('td');td.colSpan=8;td.className='ft-queue-empty';td.textContent='No transfers in this session';tr.append(td);queue.body.append(tr);return;
  }
  const fragment=document.createDocumentFragment();
  for(const item of visible){
    const tr=document.createElement('tr');tr.classList.toggle('selected',queue.selectedIDs.has(item.id));
    const select=document.createElement('td');select.className='ft-queue-select';
    const checkbox=document.createElement('input');checkbox.type='checkbox';checkbox.checked=queue.selectedIDs.has(item.id);
    checkbox.onclick=event=>{event.stopPropagation();toggleQueueCheckbox(queue,item,checkbox.checked);scheduleTransferQueueRender(view);};
    select.append(checkbox);
    const direction=document.createElement('td');direction.textContent=item.direction||'';
    const kind=document.createElement('td');kind.className='ft-queue-kind';kind.textContent=item.kind||'Transfer';
    const source=document.createElement('td');source.className='ft-queue-path';source.textContent=item.source||'';source.title=item.source||'';
    const target=document.createElement('td');target.className='ft-queue-path';target.textContent=item.target||'';target.title=item.target||'';
    const size=document.createElement('td');size.className='ft-size';size.textContent=item.size?formatSize(item.size):'';
    const status=document.createElement('td');status.className='ft-queue-status '+item.status;status.textContent=queueStatusLabel(item.status)+(item.removeAfterRun?' · remove pending':'');
    const error=document.createElement('td');error.className='ft-queue-error';error.textContent=item.error||'';error.title=item.error||'';
    tr.append(select,direction,kind,source,target,size,status,error);
    tr.onclick=event=>{selectQueueItem(queue,item,event,visible);scheduleTransferQueueRender(view);};
    tr.oncontextmenu=event=>queueContextMenu(view,event,item,visible);
    fragment.append(tr);
  }
  queue.body.append(fragment);
}
function scheduleTransferQueueRender(view){
  const queue=view.transferQueue;if(!queue||queue.renderTimer)return;
  queue.renderTimer=setTimeout(()=>{queue.renderTimer=0;renderTransferQueue(view);},80);
}
async function afterTransferQueueIdle(view){
  const queue=view.transferQueue;if(!queue)return;
  const currentRemote=remoteCacheKey(view.remote.currentPath||'.');
  const refreshCurrentRemote=queue.remoteDirty?.has(currentRemote);
  queue.remoteDirty?.clear();
  if(refreshCurrentRemote){
    try{await loadRemoteDirectory(view,currentRemote,{force:true});}catch(error){console.warn('Cannot refresh remote transfer target',error);}
  }
  if(queue.leftDirty){
    queue.leftDirty=false;
    try{await loadLeftDirectory(view,view.left.currentPath);}catch(error){console.warn('Cannot refresh left transfer target',error);}
  }
}
function nextQueuedFrom(list,stateKey,queue){
  let index=queue[stateKey]||0;
  while(index<list.length){
    const item=list[index++];
    queue[stateKey]=index;
    if(item?.status==='queued')return item;
  }
  if(index>4096){list.splice(0,index);queue[stateKey]=0;}
  return null;
}
function hasQueuedFrom(list,start=0){
  for(let i=start;i<list.length;i++)if(list[i]?.status==='queued')return true;
  return false;
}
function nextPendingTransfer(queue){
  const priority=nextQueuedFrom(queue.priorityPending,'priorityHead',queue);
  if(priority)return priority;
  if(queue.paused)return null;
  return nextQueuedFrom(queue.pending,'pendingHead',queue);
}
function hasAnyPendingTransfer(queue){
  return hasQueuedFrom(queue.priorityPending,queue.priorityHead)||hasQueuedFrom(queue.pending,queue.pendingHead);
}
function hasRunnableTransfer(queue){
  if(hasQueuedFrom(queue.priorityPending,queue.priorityHead))return true;
  return !queue.paused&&hasQueuedFrom(queue.pending,queue.pendingHead);
}
function removeFinishedQueueItem(queue,item){
  if(!item.removeAfterRun)return;
  queue.items=queue.items.filter(candidate=>candidate!==item);
  queue.selectedIDs.delete(item.id);
}
async function processTransferQueue(view){
  const queue=view.transferQueue;if(!queue||queue.running)return;
  queue.running=true;
  try{
    while(true){
      const item=nextPendingTransfer(queue);
      if(!item)break;
      item.status='running';item.error='';scheduleTransferQueueRender(view);
      try{
        await item.run(item);
        item.status='success';item.run=null;
      }catch(error){
        item.status='failed';item.error=String(error?.message||error||'Transfer failed');
      }
      removeFinishedQueueItem(queue,item);
      scheduleTransferQueueRender(view);
    }
  }finally{
    queue.running=false;scheduleTransferQueueRender(view);
    if(queue.activeScans===0&&!hasAnyPendingTransfer(queue))await afterTransferQueueIdle(view);
    if(hasRunnableTransfer(queue))processTransferQueue(view);
  }
}
function enqueueTransferTasks(view,tasks){
  const queue=view.transferQueue;if(!queue)throw new Error('Transfer queue is unavailable');
  for(const task of tasks||[]){
    queue.sequence++;
    const item={
      id:queue.sequence,status:'queued',error:'',direction:task.direction||'',kind:task.kind||'Transfer',
      source:task.source||'',target:task.target||'',size:Number(task.size)||0,run:task.run,removeAfterRun:false
    };
    queue.items.push(item);queue.pending.push(item);
  }
  scheduleTransferQueueRender(view);if(!queue.paused)processTransferQueue(view);
}
function runTransferScan(view,label,scanner){
  const queue=view.transferQueue;if(!queue)throw new Error('Transfer queue is unavailable');
  const execute=async()=>{
    queue.activeScans++;queue.scanLabel=String(label||'Scanning');scheduleTransferQueueRender(view);
    try{return await scanner();}
    finally{
      queue.activeScans=Math.max(0,queue.activeScans-1);
      if(queue.activeScans===0)queue.scanLabel='';
      scheduleTransferQueueRender(view);
      if(hasRunnableTransfer(queue))processTransferQueue(view);
      else if(!queue.running&&queue.activeScans===0&&!hasAnyPendingTransfer(queue))await afterTransferQueueIdle(view);
    }
  };
  queue.scanChain=queue.scanChain.then(execute,execute);
  return queue.scanChain;
}
function pauseTransferQueue(view){
  const queue=view.transferQueue;if(!queue)return;
  queue.paused=true;scheduleTransferQueueRender(view);
}
function resumeTransferQueue(view){
  const queue=view.transferQueue;if(!queue)return;
  queue.paused=false;scheduleTransferQueueRender(view);processTransferQueue(view);
}
function resumeSelectedTransfers(view){
  const queue=view.transferQueue;if(!queue)return;
  for(const item of queueSelectedItems(queue)){
    if(item.status==='failed'&&typeof item.run==='function'){item.status='queued';item.error='';}
    if(item.status==='queued'&&typeof item.run==='function')queue.priorityPending.push(item);
  }
  scheduleTransferQueueRender(view);processTransferQueue(view);
}
function removeSelectedTransfers(view){
  const queue=view.transferQueue;if(!queue)return;
  const selected=new Set(queue.selectedIDs);
  for(const item of queue.items){
    if(!selected.has(item.id))continue;
    if(item.status==='running'){item.removeAfterRun=true;continue;}
    item.status='removed';item.run=null;
  }
  queue.items=queue.items.filter(item=>item.status!=='removed');
  queue.selectedIDs.clear();queue.selectionAnchor=null;
  scheduleTransferQueueRender(view);
  if(!queue.running&&queue.activeScans===0&&!hasAnyPendingTransfer(queue))afterTransferQueueIdle(view);
}
function queueContextMenu(view,event,item=null,visible=[]){
  event.preventDefault();event.stopPropagation();
  const queue=view.transferQueue;if(!queue)return;
  if(item&&!queue.selectedIDs.has(item.id)){
    queue.selectedIDs.clear();queue.selectedIDs.add(item.id);queue.selectionAnchor=item.id;scheduleTransferQueueRender(view);
  }
  const selected=queueSelectedItems(queue);
  const resumable=selected.some(candidate=>(candidate.status==='queued'||candidate.status==='failed')&&typeof candidate.run==='function');
  const menu=[
    {label:queue.paused?'Resume queue':'Pause queue',action:()=>queue.paused?resumeTransferQueue(view):pauseTransferQueue(view)}
  ];
  if(selected.length){
    menu.push({separator:true});
    menu.push({label:'Resume selected',disabled:!resumable,action:()=>resumeSelectedTransfers(view)});
    menu.push({label:'Remove selected',danger:true,action:()=>removeSelectedTransfers(view)});
  }
  showContextMenu(menu,event.clientX,event.clientY,selected.length?selected.length+' queue item(s) selected':'Transfer Queue');
}
function markRemoteQueueDirty(view,path){
  const queue=view.transferQueue;if(!queue)return;
  const key=remoteCacheKey(path);queue.remoteDirty.add(key);invalidateRemoteCache(view,key);
}
function markLeftQueueDirty(view){if(view.transferQueue)view.transferQueue.leftDirty=true;}
function createTransferQueue(view){
  const root=document.createElement('div');root.className='ft-queue';
  const resizer=document.createElement('div');resizer.className='ft-queue-resizer';resizer.title='Drag to resize Transfer Queue · double-click to reset';
  const head=document.createElement('div');head.className='ft-queue-head';
  const title=document.createElement('span');title.className='ft-queue-title';title.textContent='Transfer Queue';
  const summary=document.createElement('span');summary.className='ft-queue-summary';
  const spacer=document.createElement('span');spacer.className='ft-queue-spacer';
  const retry=document.createElement('button');retry.type='button';retry.textContent='Retry failed';
  const clear=document.createElement('button');clear.type='button';clear.textContent='Clear done';
  head.append(title,summary,spacer,retry,clear);
  const wrap=document.createElement('div');wrap.className='ft-queue-wrap';
  const table=document.createElement('table');table.className='ft-queue-table';
  const thead=document.createElement('thead'),hr=document.createElement('tr');
  for(const label of ['','', 'Kind','Source','Target','Size','Status','Error']){const th=document.createElement('th');th.textContent=label;hr.append(th);}
  thead.append(hr);const body=document.createElement('tbody');table.append(thead,body);wrap.append(table);root.append(resizer,head,wrap);
  const queue={
    root,resizer,body,summary,retry,clear,items:[],pending:[],pendingHead:0,priorityPending:[],priorityHead:0,
    sequence:0,running:false,paused:false,selectedIDs:new Set(),selectionAnchor:null,
    activeScans:0,scanLabel:'',scanChain:Promise.resolve(),remoteDirty:new Set(),leftDirty:false
  };
  view.transferQueue=queue;
  root.oncontextmenu=event=>{if(event.target.closest('tbody tr'))return;queueContextMenu(view,event);};
  retry.onclick=()=>{
    for(const item of queue.items)if(item.status==='failed'&&typeof item.run==='function'){item.status='queued';item.error='';queue.pending.push(item);}
    scheduleTransferQueueRender(view);processTransferQueue(view);
  };
  clear.onclick=()=>{
    queue.items=queue.items.filter(item=>item.status!=='success');
    pruneQueueSelection(queue);scheduleTransferQueueRender(view);
  };
  renderTransferQueue(view);return root;
}
function showContextMenu(items,x,y,title=''){
  closeContextMenu();
  const menu=document.createElement('div');menu.className='ft-context';
  menu.setAttribute('role','menu');
  if(title){
    const heading=document.createElement('div');heading.className='ft-context-title';heading.textContent=title;menu.append(heading);
  }
  for(const item of items){
    if(item.separator){
      const separator=document.createElement('div');separator.className='ft-context-separator';separator.setAttribute('role','separator');menu.append(separator);continue;
    }
    const button=document.createElement('button');button.type='button';button.textContent=item.label;button.disabled=Boolean(item.disabled);
    if(item.danger)button.classList.add('danger');
    button.setAttribute('role','menuitem');
    button.onclick=()=>{closeContextMenu();Promise.resolve(item.action?.()).catch(app.showError);};menu.append(button);
  }
  document.body.append(menu);contextMenu=menu;const rect=menu.getBoundingClientRect();
  menu.style.left=Math.max(4,Math.min(x,window.innerWidth-rect.width-4))+'px';menu.style.top=Math.max(4,Math.min(y,window.innerHeight-rect.height-4))+'px';
}
document.addEventListener('pointerdown',event=>{if(contextMenu&&!contextMenu.contains(event.target))closeContextMenu();},true);
window.addEventListener('blur',closeContextMenu);

function downloadFrame(){
  let frame=document.querySelector('iframe[name="taskdeck-file-transfer-download"]');
  if(frame)return frame;
  frame=document.createElement('iframe');frame.name='taskdeck-file-transfer-download';frame.hidden=true;document.body.append(frame);return frame;
}
async function issueDownloadTicket(view,remotePath){
  const data=await app.jsonFetch('/api/file-transfer/download-ticket',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({profile_id:view.profile.id,path:remotePath})
  });
  const ticket=String(data?.ticket||'').trim();if(!ticket)throw new Error('Server did not return a download ticket');return ticket;
}
async function downloadFileToBrowser(view,remotePath){
  const ticket=await issueDownloadTicket(view,remotePath);downloadFrame().src='/api/file-transfer/download?ticket='+encodeURIComponent(ticket);
}

async function mutateRemoteRequest(view,action,path,newPath='',directory=false){
  return app.jsonFetch('/api/file-transfer/mutate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({
    profile_id:view.profile.id,action,path,new_path:newPath,directory
  })});
}
async function mutateRemote(view,action,path,newPath='',directory=false){
  await mutateRemoteRequest(view,action,path,newPath,directory);
  invalidateRemoteCache(view,parentPath(path,true));
  if(directory)invalidateRemoteCache(view,path,true);
  if(newPath){invalidateRemoteCache(view,parentPath(newPath,true));if(directory)invalidateRemoteCache(view,newPath,true);}
  await loadRemoteDirectory(view,view.remote.currentPath,{force:true});
}
async function renameRemoteEntry(view,entry){
  const oldPath=joinPath(view.remote.currentPath,entry.name,true),nextName=prompt('Rename to:',entry.name);
  if(nextName===null)return;const name=String(nextName).trim();if(!name||name===entry.name)return;
  if(name.includes('/')||name.includes('\\'))throw new Error('New name must not contain path separators');
  await mutateRemote(view,'rename',oldPath,joinPath(view.remote.currentPath,name,true),entryType(entry)==='directory');
}
async function deleteRemoteEntry(view,entry){
  if(!confirm('Delete '+entry.name+'?'+(entryType(entry)==='directory'?'\n\nDirectory removal is non-recursive.':'')))return;
  await mutateRemote(view,'delete',joinPath(view.remote.currentPath,entry.name,true),'',entryType(entry)==='directory');
}

function entrySelectionKey(entry){return encodeURIComponent(entryType(entry))+':'+encodeURIComponent(String(entry?.name||''));}
function selectedEntries(panel){
  const keys=panel.selectedKeys||new Set();
  return (panel.entries||[]).filter(entry=>keys.has(entrySelectionKey(entry)));
}
function selectedFiles(panel){return selectedEntries(panel).filter(entry=>entryType(entry)==='file');}
function resetPanelSelection(panel){
  panel.selectedKeys=new Set();panel.selected=null;panel.selectionAnchor='';
}
function syncSelectionRows(panel){
  const keys=panel.selectedKeys||new Set();
  for(const row of panel.tbody?.querySelectorAll('tr[data-selection-key]')||[]){
    const selected=keys.has(row.dataset.selectionKey||'');
    row.classList.toggle('selected',selected);
    const checkbox=row.querySelector('input[type=checkbox]');if(checkbox)checkbox.checked=selected;
  }
  const visible=panel.visibleEntries||[],selectedCount=visible.filter(entry=>keys.has(entrySelectionKey(entry))).length;
  if(panel.selectAllCheckbox){
    panel.selectAllCheckbox.checked=visible.length>0&&selectedCount===visible.length;
    panel.selectAllCheckbox.indeterminate=selectedCount>0&&selectedCount<visible.length;
  }
  const selected=selectedEntries(panel);panel.selected=selected[0]||null;panel.onSelection?.();
}
function selectOnlyEntry(panel,entry){
  panel.selectedKeys=new Set([entrySelectionKey(entry)]);panel.selectionAnchor=entrySelectionKey(entry);syncSelectionRows(panel);
}
function selectAllEntries(panel){
  panel.selectedKeys=new Set((panel.visibleEntries||[]).map(entrySelectionKey));
  panel.selectionAnchor=(panel.visibleEntries?.[0]&&entrySelectionKey(panel.visibleEntries[0]))||'';
  syncSelectionRows(panel);
}
function clearSelection(panel){resetPanelSelection(panel);syncSelectionRows(panel);}
function selectTableEntry(panel,entry,event={},mode='click'){
  const key=entrySelectionKey(entry),keys=panel.selectedKeys||(panel.selectedKeys=new Set());
  if(mode==='context'){
    if(!keys.has(key)){keys.clear();keys.add(key);}
    panel.selectionAnchor=key;syncSelectionRows(panel);return;
  }
  const toggle=Boolean(event.ctrlKey||event.metaKey);
  if(event.shiftKey&&panel.selectionAnchor){
    const visible=panel.visibleEntries||[],anchorIndex=visible.findIndex(item=>entrySelectionKey(item)===panel.selectionAnchor),currentIndex=visible.findIndex(item=>entrySelectionKey(item)===key);
    if(anchorIndex>=0&&currentIndex>=0){
      if(!toggle)keys.clear();
      const [from,to]=anchorIndex<=currentIndex?[anchorIndex,currentIndex]:[currentIndex,anchorIndex];
      for(let i=from;i<=to;i++)keys.add(entrySelectionKey(visible[i]));
    }else{keys.clear();keys.add(key);}
  }else if(toggle){
    if(keys.has(key))keys.delete(key);else keys.add(key);
    panel.selectionAnchor=key;
  }else{
    keys.clear();keys.add(key);panel.selectionAnchor=key;
  }
  syncSelectionRows(panel);
}

function createSortableTable(panel,onDoubleClick,onContextMenu){
  const wrap=document.createElement('div');wrap.className='ft-table-wrap';wrap.tabIndex=0;
  const table=document.createElement('table');table.className='ft-table';
  const thead=document.createElement('thead'),hr=document.createElement('tr');
  const selectTH=document.createElement('th');selectTH.className='ft-nosort ft-select-cell';
  const selectAll=document.createElement('input');selectAll.type='checkbox';selectAll.title='Select all visible items';
  selectAll.onchange=()=>{if(selectAll.checked)selectAllEntries(panel);else clearSelection(panel);};selectTH.append(selectAll);hr.append(selectTH);panel.selectAllCheckbox=selectAll;
  const columns=[['name','Name'],['type','Type'],['size','Size'],['modified','Modified']];
  panel.headers=new Map();
  for(const [key,label] of columns){
    const th=document.createElement('th');th.dataset.sortKey=key;th.textContent=label;
    const marker=document.createElement('span');marker.className='sort';th.append(marker);
    th.onclick=()=>{
      if(panel.sort.key===key)panel.sort.direction=panel.sort.direction==='asc'?'desc':'asc';
      else{panel.sort.key=key;panel.sort.direction='asc';}
      renderTable(panel,onDoubleClick,onContextMenu);
    };
    panel.headers.set(key,{th,marker});hr.append(th);
  }
  thead.append(hr);const tbody=document.createElement('tbody');panel.tbody=tbody;table.append(thead,tbody);wrap.append(table);
  wrap.addEventListener('keydown',event=>{
    if((event.ctrlKey||event.metaKey)&&String(event.key).toLowerCase()==='a'){event.preventDefault();selectAllEntries(panel);}
    if(event.key==='Escape'){clearSelection(panel);closeContextMenu();}
  });
  return wrap;
}
function renderTable(panel,onDoubleClick,onContextMenu){
  for(const [key,item] of panel.headers||[]){item.marker.textContent=panel.sort.key===key?(panel.sort.direction==='asc'?'▲':'▼'):'';}
  panel.tbody.replaceChildren();
  const entries=sortEntries(panel.entries,panel.sort);panel.visibleEntries=entries;
  const showParent=canGoParentPath(panel.currentPath||'.',Boolean(panel.pathIsRemote));
  if(showParent){
    const tr=document.createElement('tr');tr.className='ft-parent-row';tr.title='Up one level';
    const selectCell=document.createElement('td');selectCell.className='ft-select-cell';
    const name=document.createElement('td');name.className='ft-name';
    const icon=document.createElement('span');icon.className='ft-kind';icon.textContent='📁';
    const label=document.createElement('span');label.textContent='..';name.append(icon,label);
    const type=document.createElement('td');type.className='ft-type';type.textContent='Folder';
    const size=document.createElement('td');size.className='ft-size';
    const modified=document.createElement('td');modified.className='ft-modified';
    tr.append(selectCell,name,type,size,modified);
    tr.onclick=event=>event.preventDefault();
    tr.ondblclick=event=>{event.preventDefault();panel.goUp?.().catch(app.showError);};
    tr.oncontextmenu=event=>event.preventDefault();
    panel.tbody.append(tr);
  }
  if(!entries.length){
    if(!showParent){
      const tr=document.createElement('tr'),td=document.createElement('td');td.colSpan=5;td.className='ft-empty';td.textContent=panel.emptyText||'Directory is empty';tr.append(td);panel.tbody.append(tr);
    }
    syncSelectionRows(panel);return;
  }
  for(const entry of entries){
    const selectionKey=entrySelectionKey(entry),tr=document.createElement('tr');tr.dataset.selectionKey=selectionKey;
    const selectCell=document.createElement('td');selectCell.className='ft-select-cell';
    const checkbox=document.createElement('input');checkbox.type='checkbox';checkbox.checked=panel.selectedKeys?.has(selectionKey)||false;
    checkbox.onclick=event=>{event.stopPropagation();selectTableEntry(panel,entry,{ctrlKey:true,metaKey:false,shiftKey:event.shiftKey});};selectCell.append(checkbox);
    const name=document.createElement('td');name.className='ft-name';
    const icon=document.createElement('span');icon.className='ft-kind';icon.textContent=entryType(entry)==='directory'?'📁':entryType(entry)==='symlink'?'↗':'📄';
    const label=document.createElement('span');label.textContent=entry.name;name.append(icon,label);
    const type=document.createElement('td');type.className='ft-type';type.textContent=typeLabel(entry.type);
    const size=document.createElement('td');size.className='ft-size';size.textContent=entryType(entry)==='directory'?'':formatSize(entry.size);
    const modified=document.createElement('td');modified.className='ft-modified';modified.textContent=formatModified(entry.modified);
    tr.append(selectCell,name,type,size,modified);
    tr.onclick=event=>selectTableEntry(panel,entry,event);
    tr.ondblclick=event=>{event.preventDefault();onDoubleClick(entry);};
    tr.oncontextmenu=event=>{event.preventDefault();event.stopPropagation();selectTableEntry(panel,entry,event,'context');onContextMenu(entry,event);};
    panel.tbody.append(tr);
  }
  syncSelectionRows(panel);
}

function pathBar(panel,{remote=false,onLoad,onRefresh,onUp}){
  panel.pathIsRemote=Boolean(remote);panel.goUp=onUp;
  const bar=document.createElement('div');bar.className='ft-pathbar';
  const up=document.createElement('button');up.type='button';up.textContent='↑';up.title='Parent directory';
  const input=document.createElement('input');input.type='text';input.spellcheck=false;input.autocomplete='off';
  const history=document.createElement('select');history.className='ft-path-history-select';history.setAttribute('aria-label','Visited paths');history.title='Visited paths';
  const star=document.createElement('button');star.type='button';star.className='ft-favorite-toggle';star.textContent='☆';
  const go=document.createElement('button');go.type='button';go.textContent='Go';
  const refresh=document.createElement('button');refresh.type='button';refresh.textContent='↻';refresh.title='Refresh';
  bar.append(up,input,history,star,go,refresh);
  panel.pathInput=input;panel.pathHistorySelect=history;panel.favoriteToggle=star;panel.refresh=refresh;
  const reloadMemory=()=>populatePathMemory(history,star,panel.memoryScope?.()||'',input.value||'.');
  history.onchange=()=>{if(history.value){input.value=history.value;onLoad(history.value).catch(app.showError);}history.value='';};
  star.onclick=()=>{const scope=panel.memoryScope?.();if(!scope)return;toggleFavorite(scope,input.value||'.');reloadMemory();};
  go.onclick=()=>onLoad(input.value).catch(app.showError);
  input.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();go.click();}});
  refresh.onclick=()=>onRefresh().catch(app.showError);up.onclick=()=>onUp().catch(app.showError);
  panel.refreshPathMemory=reloadMemory;return bar;
}

function markPathLoaded(panel,path){
  panel.currentPath=path;panel.pathInput.value=path;
  const scope=panel.memoryScope?.();if(scope){rememberPath(scope,path);panel.refreshPathMemory?.();}
}

function remoteCacheKey(path){return normalizeRemotePath(path||'.');}
function remoteCacheGet(view,path){return view.remoteCache?.get(remoteCacheKey(path))||null;}
function remoteCacheSet(view,path,data){
  if(!view.remoteCache)view.remoteCache=new Map();
  const key=remoteCacheKey(path);
  const entry={
    path:String(data?.path||key),
    protocol:String(data?.protocol||view.profile.protocol||''),
    entries:(Array.isArray(data?.entries)?data.entries:[]).map(item=>({...item,type:entryType(item)})),
    cachedAt:Date.now()
  };
  view.remoteCache.set(key,entry);return entry;
}
function invalidateRemoteCache(view,path,recursive=false){
  if(!view.remoteCache)return;
  const key=remoteCacheKey(path);
  view.remoteCache.delete(key);
  if(recursive){
    const prefix=key==='/'?'/':key.replace(/\/$/,'')+'/';
    for(const cachedKey of [...view.remoteCache.keys()]){
      if(cachedKey.startsWith(prefix))view.remoteCache.delete(cachedKey);
    }
  }
}
async function fetchRemoteDirectory(view,path,{force=false}={}){
  const key=remoteCacheKey(path);
  if(!force){
    const cached=remoteCacheGet(view,key);
    if(cached)return {...cached,fromCache:true};
  }
  const data=await app.jsonFetch('/api/file-transfer/list',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({profile_id:view.profile.id,path:key})
  });
  return {...remoteCacheSet(view,key,data),fromCache:false};
}
function renderRemoteDirectory(view,data){
  const panel=view.remote;
  panel.entries=(Array.isArray(data?.entries)?data.entries:[]).map(item=>({...item,type:entryType(item)}));resetPanelSelection(panel);
  markPathLoaded(panel,String(data?.path||'.'));persistFileTransferSession();
  renderTable(panel,entry=>remoteDoubleClick(view,entry),(entry,event)=>remoteContext(view,entry,event));
  const cached=data?.fromCache?' · cached':'';
  panel.status.textContent=(data?.protocol||view.profile.protocol).toUpperCase()+' · '+panel.entries.length+' item(s)'+cached;
}

async function loadRemoteDirectory(view,path,options={}){
  const panel=view.remote;path=normalizeRemotePath(path||profileFor(view)?.initial_path||'.');
  const cached=!options.force&&remoteCacheGet(view,path);
  panel.status.textContent=cached?'Opening cached '+path+'…':'Loading '+path+'…';panel.refresh.disabled=true;
  try{
    const data=await fetchRemoteDirectory(view,path,{force:Boolean(options.force)});
    renderRemoteDirectory(view,data);
  }catch(error){panel.status.textContent=String(error?.message||error);throw error;}
  finally{panel.refresh.disabled=false;updateTransferButtons(view);}
}

async function loadHostDirectory(view,path){
  const panel=view.left;path=normalizeRelativePath(path||'.');panel.status.textContent='Loading host '+path+'…';panel.refresh.disabled=true;
  try{
    const query=path==='.'?'':path;
    const data=await app.jsonFetch('/api/project/tree?path='+encodeURIComponent(query));
    panel.entries=(Array.isArray(data)?data:[]).map(item=>({...item,type:item.type==='dir'?'directory':entryType(item)}));resetPanelSelection(panel);
    markPathLoaded(panel,path);persistFileTransferSession();renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));
    panel.status.textContent='Host workspace · '+panel.entries.length+' item(s)';
  }catch(error){panel.status.textContent=String(error?.message||error);throw error;}
  finally{panel.refresh.disabled=false;updateTransferButtons(view);}
}

async function chooseLocalRoot(view){
  if(typeof globalThis.showDirectoryPicker!=='function')throw new Error('This browser does not support local folder access. Use a Chromium-based browser over HTTPS or localhost.');
  const handle=await globalThis.showDirectoryPicker({mode:'readwrite'});
  const id=localRootID(),record={id,label:handle.name||'Local folder',handle,lastUsed:Date.now()};
  await putLocalRoot(record);safeStorageSet('taskdeck:file-transfer:last-local-root:'+workspaceKey(),id);
  await refreshLocalRoots(view,id);return record;
}

async function refreshLocalRoots(view,selectID=''){
  const panel=view.left,records=await localRootRecords();panel.localRoots=records;
  panel.rootSelect.replaceChildren();
  if(!records.length){const option=document.createElement('option');option.value='';option.textContent='Choose local folder…';panel.rootSelect.append(option);}
  for(const record of records){const option=document.createElement('option');option.value=record.id;option.textContent=record.label||'Local folder';panel.rootSelect.append(option);}
  const preferred=selectID||safeStorageGet('taskdeck:file-transfer:last-local-root:'+workspaceKey(),'')||records[0]?.id||'';
  if(preferred&&records.some(item=>item.id===preferred))panel.rootSelect.value=preferred;
  panel.localRoot=records.find(item=>item.id===panel.rootSelect.value)||null;
  if(panel.localRoot){
    safeStorageSet('taskdeck:file-transfer:last-local-root:'+workspaceKey(),panel.localRoot.id);
    const granted=await ensureHandlePermission(panel.localRoot.handle);
    panel.localPermission=granted;
    panel.grantLocal.hidden=granted;panel.chooseLocal.textContent='Choose…';
    if(granted){panel.localRoot.lastUsed=Date.now();await putLocalRoot(panel.localRoot);}
  }else{panel.localPermission=false;panel.grantLocal.hidden=true;}
}

async function loadLocalDirectory(view,path){
  const panel=view.left;
  if(!panel.localRoot){panel.status.textContent='Choose a local folder first';panel.entries=[];renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));return;}
  const granted=await ensureHandlePermission(panel.localRoot.handle);
  panel.localPermission=granted;
  if(!granted){panel.status.textContent='Local folder permission is required. Click Grant.';panel.entries=[];renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));return;}
  path=normalizeRelativePath(path||'.');panel.status.textContent='Loading local '+path+'…';panel.refresh.disabled=true;
  try{
    const dir=await directoryHandleForPath(panel.localRoot.handle,path),entries=[];
    for await(const [name,handle] of dir.entries()){
      if(handle.kind==='directory')entries.push({name,type:'directory',size:0,modified:''});
      else if(handle.kind==='file'){const file=await handle.getFile();entries.push({name,type:'file',size:file.size,modified:new Date(file.lastModified).toISOString(),handle});}
    }
    panel.entries=entries;resetPanelSelection(panel);panel.currentLocalHandle=dir;markPathLoaded(panel,path);persistFileTransferSession();
    renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));panel.status.textContent='Local · '+(panel.localRoot.label||panel.localRoot.handle.name)+' · '+entries.length+' item(s)';
  }catch(error){panel.status.textContent=String(error?.message||error);throw error;}
  finally{panel.refresh.disabled=false;updateTransferButtons(view);}
}

function currentLeftScope(view){
  const panel=view.left;
  if(panel.source==='host')return 'host:'+workspaceKey();
  return panel.localRoot?'local:'+panel.localRoot.id:'local:none';
}
async function loadLeftDirectory(view,path){
  return view.left.source==='local'?loadLocalDirectory(view,path):loadHostDirectory(view,path);
}

async function switchLeftSource(view,source,startPath='.'){
  const panel=view.left;panel.source=source==='local'?'local':'host';
  safeStorageSet('taskdeck:file-transfer:left-source:'+workspaceKey()+':'+view.profile.id,panel.source);
  panel.rootSelect.hidden=panel.source!=='local';panel.chooseLocal.hidden=panel.source!=='local';
  panel.pathInput.placeholder=panel.source==='local'?'Path relative to selected local folder':'Path relative to TaskDeck workspace';
  startPath=normalizeRelativePath(startPath||'.');
  panel.currentPath=startPath;panel.entries=[];resetPanelSelection(panel);panel.pathInput.value=startPath;
  if(panel.source==='local'){
    if(typeof globalThis.showDirectoryPicker!=='function'){panel.grantLocal.hidden=true;panel.status.textContent='Local browser requires File System Access API (Chromium, HTTPS/localhost).';panel.entries=[];renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));updateTransferButtons(view);return;}
    await refreshLocalRoots(view);
    panel.grantLocal.hidden=!panel.localRoot||panel.localPermission;
  }else panel.grantLocal.hidden=true;
  panel.refreshPathMemory?.();
  await loadLeftDirectory(view,startPath);
}

async function localSelectedFile(view,entry){
  const panel=view.left;if(panel.source!=='local'||!panel.localRoot)throw new Error('Local folder is not selected');
  const dir=await directoryHandleForPath(panel.localRoot.handle,panel.currentPath);
  const handle=await dir.getFileHandle(entry.name);return handle.getFile();
}

async function uploadBrowserFileToPath(view,file,target){
  const form=new FormData();
  form.append('profile_id',view.profile.id);form.append('path',target);form.append('file',file,file.name);
  const response=await app.fetchWithLease('/api/file-transfer/upload',{method:'POST',body:form,cache:'no-store'});
  if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
}
async function uploadBrowserFile(view,file){
  return uploadBrowserFileToPath(view,file,joinPath(view.remote.currentPath,file.name,true));
}

async function hostMutation(view,action,path,newPath=''){
  await app.jsonFetch('/api/file-transfer/host-mutate',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({action,path,new_path:newPath})
  });
}
async function renameHostEntry(view,entry){
  const before=String(entry.name||''),answer=prompt('Rename to:',before);if(answer===null)return;
  const name=String(answer).trim();if(!name||name===before)return;
  if(name.includes('/')||name.includes('\\'))throw new Error('New name must not contain path separators');
  await hostMutation(view,'rename',joinPath(view.left.currentPath,before,false),joinPath(view.left.currentPath,name,false));
  await loadHostDirectory(view,view.left.currentPath);
}
async function deleteHostEntries(view,entries){
  const selected=[...(entries||[])];if(!selected.length)return;
  const dirs=selected.filter(entry=>entryType(entry)==='directory').length;
  if(!confirm('Delete '+selected.length+' selected host item(s)?'+(dirs?'\n\nSelected folders are removed non-recursively and must be empty.':'')))return;
  for(const entry of selected)await hostMutation(view,'delete',joinPath(view.left.currentPath,entry.name,false));
  await loadHostDirectory(view,view.left.currentPath);
}
async function newHostFolder(view){
  const answer=prompt('New host folder name:','');if(answer===null)return;
  const name=String(answer).trim();if(!name)return;
  if(name.includes('/')||name.includes('\\'))throw new Error('Folder name must not contain path separators');
  await hostMutation(view,'mkdir',joinPath(view.left.currentPath,name,false));
  await loadHostDirectory(view,view.left.currentPath);
}
async function localDirectoryHandle(view){
  const panel=view.left;if(!panel.localRoot)throw new Error('Choose a local folder first');
  const granted=await ensureHandlePermission(panel.localRoot.handle);if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
  return directoryHandleForPath(panel.localRoot.handle,panel.currentPath);
}
async function renameLocalEntry(view,entry){
  const dir=await localDirectoryHandle(view),handle=entryType(entry)==='directory'?await dir.getDirectoryHandle(entry.name):await dir.getFileHandle(entry.name);
  if(typeof handle.move!=='function')throw new Error('This browser does not support native local rename. Use Host mode or a browser version with FileSystemHandle.move().');
  const answer=prompt('Rename to:',entry.name);if(answer===null)return;
  const name=String(answer).trim();if(!name||name===entry.name)return;
  if(name.includes('/')||name.includes('\\'))throw new Error('New name must not contain path separators');
  await handle.move(name);await loadLocalDirectory(view,view.left.currentPath);
}
async function deleteLocalEntries(view,entries){
  const selected=[...(entries||[])];if(!selected.length)return;
  const dirs=selected.filter(entry=>entryType(entry)==='directory').length;
  if(!confirm('Delete '+selected.length+' selected local item(s)?'+(dirs?'\n\nSelected folders are removed non-recursively and must be empty.':'')))return;
  const dir=await localDirectoryHandle(view);
  for(const entry of selected)await dir.removeEntry(entry.name,{recursive:false});
  await loadLocalDirectory(view,view.left.currentPath);
}
async function newLocalFolder(view){
  const answer=prompt('New local folder name:','');if(answer===null)return;
  const name=String(answer).trim();if(!name)return;
  if(name.includes('/')||name.includes('\\'))throw new Error('Folder name must not contain path separators');
  const dir=await localDirectoryHandle(view);await dir.getDirectoryHandle(name,{create:true});await loadLocalDirectory(view,view.left.currentPath);
}
async function renameLeftEntry(view,entry){
  return view.left.source==='host'?renameHostEntry(view,entry):renameLocalEntry(view,entry);
}
async function deleteLeftEntries(view,entries){
  return view.left.source==='host'?deleteHostEntries(view,entries):deleteLocalEntries(view,entries);
}
async function newLeftFolder(view){
  return view.left.source==='host'?newHostFolder(view):newLocalFolder(view);
}

function pathLeaf(value,remote=false){
  const normalized=remote?normalizeRemotePath(value):normalizeRelativePath(value);
  if(normalized==='.'||normalized==='/')return normalized;
  const parts=normalized.split('/').filter(Boolean);return parts[parts.length-1]||normalized;
}
function pathDepth(value,remote=false){
  const normalized=remote?normalizeRemotePath(value):normalizeRelativePath(value);
  if(normalized==='.'||normalized==='/')return 0;
  return normalized.split('/').filter(Boolean).length;
}
async function fetchHostDirectoryEntries(path){
  path=normalizeRelativePath(path||'.');
  const data=await app.jsonFetch('/api/project/tree?path='+encodeURIComponent(path==='.'?'':path));
  return (Array.isArray(data)?data:[]).map(item=>({...item,type:item.type==='dir'?'directory':entryType(item)}));
}
function newRemoteScanState(basePath){
  const base=normalizeRemotePath(basePath||'.');
  return {base,remoteDirectories:new Set(['.','/',base]),remoteListings:new Map(),files:0,folders:0,errors:0};
}
async function ensureRemoteScanDirectory(view,target,state){
  target=normalizeRemotePath(target);
  if(target==='.'||target==='/'||state.remoteDirectories.has(target))return;
  const parent=parentPath(target,true);
  await ensureRemoteScanDirectory(view,parent,state);
  const name=pathLeaf(target,true);
  let entries=state.remoteListings.get(parent);
  if(!entries){
    const listing=await fetchRemoteDirectory(view,parent);
    entries=listing.entries.map(item=>({...item,type:entryType(item)}));
    state.remoteListings.set(parent,entries);
  }
  let existing=entries.find(item=>item.name===name);
  if(existing){
    if(entryType(existing)!=='directory')throw new Error('Remote path exists and is not a folder: '+target);
    state.remoteDirectories.add(target);
    if(!state.remoteListings.has(target)){
      const cached=remoteCacheGet(view,target);
      if(cached)state.remoteListings.set(target,cached.entries.map(item=>({...item,type:entryType(item)})));
    }
    return;
  }
  try{
    await mutateRemoteRequest(view,'mkdir',target,'',true);
  }catch(error){
    const listing=await fetchRemoteDirectory(view,parent,{force:true});
    entries=listing.entries.map(item=>({...item,type:entryType(item)}));
    state.remoteListings.set(parent,entries);
    existing=entries.find(item=>item.name===name);
    if(!existing||entryType(existing)!=='directory')throw error;
    state.remoteDirectories.add(target);return;
  }
  entries.push({name,type:'directory',size:0,modified:''});
  state.remoteListings.set(target,[]);
  state.remoteDirectories.add(target);state.folders++;
  markRemoteQueueDirty(view,parent);
}
function enqueueHostUploadFile(view,sourcePath,targetPath,size,state){
  state.files++;
  enqueueTransferTasks(view,[{
    direction:'→',kind:'Upload',source:sourcePath,target:targetPath,size:Number(size)||0,
    run:async()=>{
      await app.jsonFetch('/api/file-transfer/host-to-remote',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({
        profile_id:view.profile.id,host_path:sourcePath,remote_path:targetPath
      })});
      markRemoteQueueDirty(view,parentPath(targetPath,true));
    }
  }]);
}
function enqueueLocalUploadHandle(view,handle,sourcePath,targetPath,state){
  state.files++;
  enqueueTransferTasks(view,[{
    direction:'→',kind:'Upload',source:sourcePath,target:targetPath,size:0,
    run:async item=>{
      const file=await handle.getFile();
      item.size=file.size;scheduleTransferQueueRender(view);
      await uploadBrowserFileToPath(view,file,targetPath);
      markRemoteQueueDirty(view,parentPath(targetPath,true));
    }
  }]);
}
async function scanHostUploadEntry(view,parent,entry,remoteParent,state){
  const sourcePath=joinPath(parent,entry.name,false),targetPath=joinPath(remoteParent,entry.name,true);
  if(entryType(entry)!=='directory'){
    enqueueHostUploadFile(view,sourcePath,targetPath,entry.size,state);return;
  }
  await ensureRemoteScanDirectory(view,targetPath,state);
  const children=await fetchHostDirectoryEntries(sourcePath);
  for(const child of children)await scanHostUploadEntry(view,sourcePath,child,targetPath,state);
}
async function scanLocalUploadHandle(view,handle,targetPath,sourcePath,state){
  if(handle.kind!=='directory'){
    enqueueLocalUploadHandle(view,handle,sourcePath,targetPath,state);return;
  }
  await ensureRemoteScanDirectory(view,targetPath,state);
  for await(const [name,child] of handle.entries()){
    await scanLocalUploadHandle(view,child,joinPath(targetPath,name,true),joinPath(sourcePath,name,false),state);
  }
}
async function streamLeftEntriesToRemote(view,entries){
  const selected=[...(entries||[])];if(!selected.length)throw new Error('Select one or more left items first');
  const source=view.left.source,sourceBase=normalizeRelativePath(view.left.currentPath||'.'),remoteBase=normalizeRemotePath(view.remote.currentPath||'.');
  const state=newRemoteScanState(remoteBase);
  let localBase=null;
  if(source==='local'){
    const root=view.left.localRoot;if(!root)throw new Error('Choose a local folder first');
    const granted=await ensureHandlePermission(root.handle);if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
    localBase=await directoryHandleForPath(root.handle,sourceBase);
  }
  view.remote.status.textContent='Scanning and uploading '+selected.length+' selected item(s)…';
  await runTransferScan(view,'Upload scan',async()=>{
    for(const entry of selected){
      if(source==='host'){
        await scanHostUploadEntry(view,sourceBase,entry,remoteBase,state);
      }else{
        const handle=entryType(entry)==='directory'?await localBase.getDirectoryHandle(entry.name):await localBase.getFileHandle(entry.name);
        await scanLocalUploadHandle(view,handle,joinPath(remoteBase,entry.name,true),joinPath(sourceBase,entry.name,false),state);
      }
    }
  });
  view.remote.status.textContent='Scan complete · '+state.files+' file(s) discovered · '+state.folders+' folder(s) created';
}
function newLeftScanState(view){
  const source=view.left.source,base=normalizeRelativePath(view.left.currentPath||'.');
  return {source,base,localRoot:view.left.localRoot||null,hostDirectories:new Set(['.',base]),hostListings:new Map(),localHandles:new Map(),files:0,folders:0};
}
async function ensureHostScanDirectory(view,target,state){
  target=normalizeRelativePath(target);
  if(target==='.'||state.hostDirectories.has(target))return;
  const parent=parentPath(target,false);
  await ensureHostScanDirectory(view,parent,state);
  const name=pathLeaf(target,false);
  let entries=state.hostListings.get(parent);
  if(!entries){entries=await fetchHostDirectoryEntries(parent);state.hostListings.set(parent,entries);}
  const existing=entries.find(item=>item.name===name);
  if(existing){
    if(entryType(existing)!=='directory')throw new Error('Host path exists and is not a folder: '+target);
    state.hostDirectories.add(target);return;
  }
  try{await hostMutation(view,'mkdir',target);}
  catch(error){
    entries=await fetchHostDirectoryEntries(parent);state.hostListings.set(parent,entries);
    const retry=entries.find(item=>item.name===name);
    if(!retry||entryType(retry)!=='directory')throw error;
    state.hostDirectories.add(target);return;
  }
  entries.push({name,type:'directory',size:0,modified:''});
  state.hostDirectories.add(target);state.folders++;markLeftQueueDirty(view);
}
async function ensureLocalScanDirectory(target,state){
  target=normalizeRelativePath(target);
  if(target==='.')return state.localRoot.handle;
  if(state.localHandles.has(target))return state.localHandles.get(target);
  const parent=parentPath(target,false),name=pathLeaf(target,false);
  const parentHandle=parent==='.'?state.localRoot.handle:await ensureLocalScanDirectory(parent,state);
  const handle=await parentHandle.getDirectoryHandle(name,{create:true});
  state.localHandles.set(target,handle);state.folders++;markLeftQueueDirty(state.view);
  return handle;
}
async function ensureLeftScanDirectory(view,target,state){
  if(state.source==='host')return ensureHostScanDirectory(view,target,state);
  return ensureLocalScanDirectory(target,state);
}
async function writeRemotePathToLocalRoot(view,root,remotePath,leftPath){
  const dirPath=parentPath(leftPath,false),name=pathLeaf(leftPath,false);
  let dir=root.handle;
  if(dirPath!=='.')for(const part of dirPath.split('/').filter(Boolean))dir=await dir.getDirectoryHandle(part,{create:true});
  let exists=false;
  try{await dir.getFileHandle(name);exists=true;}catch(error){if(error?.name!=='NotFoundError')throw error;}
  if(exists&&!confirm(leftPath+' already exists locally. Overwrite it?'))throw new Error('Skipped existing local file: '+leftPath);
  const handle=await dir.getFileHandle(name,{create:true}),writable=await handle.createWritable();
  try{
    const ticket=await issueDownloadTicket(view,remotePath);
    const response=await fetch('/api/file-transfer/download?ticket='+encodeURIComponent(ticket),{cache:'no-store'});
    if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
    if(response.body&&typeof response.body.pipeTo==='function')await response.body.pipeTo(writable);
    else{await writable.write(await response.arrayBuffer());await writable.close();}
  }catch(error){try{await writable.abort();}catch{}throw error;}
}
async function writeRemotePathToHost(view,remotePath,leftPath){
  const payload={profile_id:view.profile.id,remote_path:remotePath,host_dir:parentPath(leftPath,false),overwrite:false};
  let response=await app.fetchWithLease('/api/file-transfer/remote-to-host',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),cache:'no-store'});
  if(response.status===409){
    const message=(await response.text()).trim();
    if(message.includes('already exists')&&confirm(leftPath+' already exists on Host. Overwrite it?')){
      payload.overwrite=true;response=await app.fetchWithLease('/api/file-transfer/remote-to-host',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),cache:'no-store'});
    }else throw new Error(message||('Skipped existing host file: '+leftPath));
  }
  if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
}
function enqueueRemoteDownloadFile(view,remotePath,leftPath,size,state){
  state.files++;
  const source=state.source,localRoot=state.localRoot;
  enqueueTransferTasks(view,[{
    direction:'←',kind:'Download',source:remotePath,target:leftPath,size:Number(size)||0,
    run:async()=>{
      if(source==='host')await writeRemotePathToHost(view,remotePath,leftPath);
      else await writeRemotePathToLocalRoot(view,localRoot,remotePath,leftPath);
      markLeftQueueDirty(view);
    }
  }]);
}
async function scanRemoteDownloadEntry(view,remoteParent,entry,leftParent,state){
  const remotePath=joinPath(remoteParent,entry.name,true),leftPath=joinPath(leftParent,entry.name,false);
  if(entryType(entry)!=='directory'){
    enqueueRemoteDownloadFile(view,remotePath,leftPath,entry.size,state);return;
  }
  await ensureLeftScanDirectory(view,leftPath,state);
  const listing=await fetchRemoteDirectory(view,remotePath);
  for(const child of listing.entries)await scanRemoteDownloadEntry(view,remotePath,child,leftPath,state);
}
async function streamRemoteEntriesToLeft(view,entries){
  const selected=[...(entries||[])];if(!selected.length)throw new Error('Select one or more remote items first');
  const remoteBase=normalizeRemotePath(view.remote.currentPath||'.'),state=newLeftScanState(view);
  state.view=view;
  if(state.source==='local'){
    if(!state.localRoot)throw new Error('Choose a local folder first');
    const granted=await ensureHandlePermission(state.localRoot.handle);if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
    state.localHandles.set('.',state.localRoot.handle);
  }
  view.left.status.textContent='Scanning and downloading '+selected.length+' selected item(s)…';
  await runTransferScan(view,'Download scan',async()=>{
    for(const entry of selected)await scanRemoteDownloadEntry(view,remoteBase,entry,state.base,state);
  });
  view.left.status.textContent='Scan complete · '+state.files+' file(s) discovered · '+state.folders+' folder(s) prepared';
}
async function transferLeftEntriesToRemote(view,entries){return streamLeftEntriesToRemote(view,entries);}
async function transferLeftToRemote(view){return transferLeftEntriesToRemote(view,selectedEntries(view.left));}
async function transferRemoteEntriesToLeft(view,entries){return streamRemoteEntriesToLeft(view,entries);}
async function transferRemoteToLeft(view){return transferRemoteEntriesToLeft(view,selectedEntries(view.remote));}

function enqueueRemoteDeleteItem(view,path,directory,state){
  if(directory)state.folders++;else state.files++;
  enqueueTransferTasks(view,[{
    direction:'×',kind:'Delete',source:path,target:'',size:0,
    run:async()=>{
      await mutateRemoteRequest(view,'delete',path,'',directory);
      if(directory)invalidateRemoteCache(view,path,true);
      markRemoteQueueDirty(view,parentPath(path,true));
    }
  }]);
}
async function scanRemoteDeleteEntry(view,remoteParent,entry,state){
  const remotePath=joinPath(remoteParent,entry.name,true);
  if(entryType(entry)!=='directory'){
    enqueueRemoteDeleteItem(view,remotePath,false,state);return;
  }
  const listing=await fetchRemoteDirectory(view,remotePath,{force:true});
  for(const child of listing.entries)await scanRemoteDeleteEntry(view,remotePath,child,state);
  enqueueRemoteDeleteItem(view,remotePath,true,state);
}
async function streamRemoteDeleteEntries(view,entries){
  const selected=[...(entries||[])];if(!selected.length)return;
  const dirs=selected.filter(entry=>entryType(entry)==='directory').length;
  const message=selected.length===1
    ?'Delete '+selected[0].name+'?'+(dirs?'\n\nThe folder will be scanned in background and deleted recursively through Transfer Queue.':'')
    :'Delete '+selected.length+' selected item(s)?'+(dirs?'\n\nSelected folders will be scanned in background and deleted recursively through Transfer Queue.':'');
  if(!confirm(message))return;
  const base=normalizeRemotePath(view.remote.currentPath||'.'),state={files:0,folders:0};
  view.remote.status.textContent='Scanning delete targets…';
  await runTransferScan(view,'Delete scan',async()=>{
    for(const entry of selected)await scanRemoteDeleteEntry(view,base,entry,state);
  });
  view.remote.status.textContent='Delete scan complete · '+state.files+' file(s) · '+state.folders+' folder(s) queued';
}
async function deleteRemoteEntries(view,entries){return streamRemoteDeleteEntries(view,entries);}

async function copyText(value){
  const text=String(value??'');
  if(navigator.clipboard?.writeText){await navigator.clipboard.writeText(text);return;}
  const area=document.createElement('textarea');area.value=text;area.style.position='fixed';area.style.opacity='0';document.body.append(area);area.select();
  try{document.execCommand('copy');}finally{area.remove();}
}

function updateTransferButtons(view){
  view.toRemote.disabled=selectedEntries(view.left).length===0;
  view.toLeft.disabled=selectedEntries(view.remote).length===0;
}

function leftDoubleClick(view,entry){
  if(entryType(entry)==='directory')loadLeftDirectory(view,joinPath(view.left.currentPath,entry.name,false)).catch(app.showError);
  else transferLeftEntriesToRemote(view,[entry]).catch(app.showError);
}
function remoteDoubleClick(view,entry){
  if(entryType(entry)==='directory')loadRemoteDirectory(view,joinPath(view.remote.currentPath,entry.name,true)).catch(app.showError);
  else transferRemoteEntriesToLeft(view,[entry]).catch(app.showError);
}
function contextTitle(panel,entry){
  const selected=selectedEntries(panel);
  return selected.length>1?selected.length+' items selected':String(entry?.name||'File actions');
}
function selectedPathText(panel,remote=false){
  return selectedEntries(panel).map(entry=>joinPath(panel.currentPath,entry.name,remote)).join('\n');
}
function commonSelectionMenu(panel,remote,onRefresh){
  const selected=selectedEntries(panel),items=[];
  if(selected.length===1)items.push({label:'Copy name',action:()=>copyText(selected[0].name)});
  if(selected.length)items.push({label:selected.length>1?'Copy selected paths':'Copy path',action:()=>copyText(selectedPathText(panel,remote))});
  items.push({separator:true},{label:'Select all',action:()=>selectAllEntries(panel)});
  if(selected.length>1)items.push({label:'Clear selection',action:()=>clearSelection(panel)});
  items.push({label:'Refresh',action:onRefresh});
  return items;
}
function leftContext(view,entry,event){
  const panel=view.left,selected=selectedEntries(panel),items=[];
  if(selected.length===1&&entryType(selected[0])==='directory'){
    items.push({label:'Open folder',action:()=>loadLeftDirectory(view,joinPath(panel.currentPath,selected[0].name,false))});
  }
  if(selected.length){
    items.push({label:selected.length>1?'Upload selected items to remote FTP/SFTP →':'Upload to remote FTP/SFTP →',action:()=>transferLeftEntriesToRemote(view,selected)});
  }
  if(panel.source==='host'&&selected.length===1&&entryType(selected[0])==='file'){
    items.push({label:'Download host file to browser',action:()=>{downloadFrame().src='/api/files/download?path='+encodeURIComponent(joinPath(panel.currentPath,selected[0].name,false));}});
  }
  if(items.length)items.push({separator:true});
  if(selected.length===1)items.push({label:'Rename',action:()=>renameLeftEntry(view,selected[0])});
  if(selected.length)items.push({label:'Delete '+(selected.length>1?selected.length+' selected items':'item'),danger:true,action:()=>deleteLeftEntries(view,selected)});
  items.push({label:'New folder',action:()=>newLeftFolder(view)});
  items.push({separator:true},...commonSelectionMenu(panel,false,()=>loadLeftDirectory(view,panel.currentPath)));
  showContextMenu(items,event.clientX,event.clientY,contextTitle(panel,entry));
}
function remoteContext(view,entry,event){
  const panel=view.remote,selected=selectedEntries(panel),items=[];
  if(selected.length===1&&entryType(selected[0])==='directory'){
    items.push({label:'Open folder',action:()=>loadRemoteDirectory(view,joinPath(panel.currentPath,selected[0].name,true))});
  }
  if(selected.length)items.push({label:selected.length>1?'Download selected items to left ←':'Download to left ←',action:()=>transferRemoteEntriesToLeft(view,selected)});
  if(selected.length===1&&entryType(selected[0])==='file'){
    items.push({label:'Download to browser',action:()=>downloadFileToBrowser(view,joinPath(panel.currentPath,selected[0].name,true))});
  }
  if(items.length)items.push({separator:true});
  if(selected.length===1)items.push({label:'Rename',action:()=>renameRemoteEntry(view,selected[0])});
  if(selected.length)items.push({label:'Delete '+(selected.length>1?selected.length+' selected items':'item'),danger:true,action:()=>deleteRemoteEntries(view,selected)});
  items.push({label:'New remote folder',action:()=>{
    const name=prompt('New remote folder name:','');if(name===null)return;const clean=String(name).trim();if(!clean)return;
    if(clean.includes('/')||clean.includes('\\'))throw new Error('Folder name must not contain path separators');
    return mutateRemote(view,'mkdir',joinPath(panel.currentPath,clean,true));
  }});
  items.push({separator:true},...commonSelectionMenu(panel,true,()=>loadRemoteDirectory(view,panel.currentPath,{force:true})));
  showContextMenu(items,event.clientX,event.clientY,contextTitle(panel,entry));
}

function installDivider(view,divider,sites){
  const storageKey='taskdeck:file-transfer:split:'+workspaceKey()+':'+view.profile.id;
  let ratio=Math.min(.75,Math.max(.25,Number(safeStorageGet(storageKey,'.5'))||.5)),dragging=false;
  const apply=()=>{
    if(matchMedia('(max-width:850px)').matches){sites.style.gridTemplateColumns='1fr';sites.style.gridTemplateRows=`minmax(220px,${ratio}fr) 8px minmax(220px,${1-ratio}fr)`;}
    else{sites.style.gridTemplateRows='';sites.style.gridTemplateColumns=`minmax(260px,${ratio}fr) 8px minmax(260px,${1-ratio}fr)`;}
  };
  apply();window.addEventListener('resize',apply);
  divider.onpointerdown=event=>{if(event.button!==0)return;dragging=true;divider.classList.add('dragging');document.body.classList.add('ft-resizing');divider.setPointerCapture(event.pointerId);event.preventDefault();};
  divider.onpointermove=event=>{
    if(!dragging)return;const rect=sites.getBoundingClientRect();
    ratio=matchMedia('(max-width:850px)').matches?(event.clientY-rect.top)/rect.height:(event.clientX-rect.left)/rect.width;
    ratio=Math.min(.75,Math.max(.25,ratio));apply();event.preventDefault();
  };
  const finish=event=>{if(!dragging)return;dragging=false;divider.classList.remove('dragging');document.body.classList.remove('ft-resizing');safeStorageSet(storageKey,String(ratio));try{divider.releasePointerCapture(event.pointerId);}catch{}};
  divider.onpointerup=finish;divider.onpointercancel=finish;divider.ondblclick=()=>{ratio=.5;apply();safeStorageSet(storageKey,'.5');};
}

function installQueueResizer(view){
  const queue=view.transferQueue,resizer=queue?.resizer,root=queue?.root;if(!resizer||!root)return;
  const storageKey='taskdeck:file-transfer:queue-height:'+workspaceKey()+':'+view.profile.id;
  const defaultHeight=170,minHeight=90;
  let height=Number(safeStorageGet(storageKey,String(defaultHeight)))||defaultHeight,dragging=false,startY=0,startHeight=height;
  const maxHeight=()=>{
    const paneHeight=view.pane.getBoundingClientRect().height||window.innerHeight||600;
    return Math.max(minHeight,Math.floor(Math.min(paneHeight*.65,paneHeight-38-160)));
  };
  const apply=()=>{
    height=Math.round(Math.min(maxHeight(),Math.max(minHeight,height)));
    root.style.height=height+'px';
    root.style.flex='0 0 '+height+'px';
  };
  apply();
  resizer.onpointerdown=event=>{
    if(event.button!==0)return;
    dragging=true;startY=event.clientY;startHeight=root.getBoundingClientRect().height||height;
    resizer.classList.add('dragging');document.body.classList.add('ft-queue-resizing');
    resizer.setPointerCapture(event.pointerId);event.preventDefault();
  };
  resizer.onpointermove=event=>{
    if(!dragging)return;
    height=startHeight-(event.clientY-startY);apply();event.preventDefault();
  };
  const finish=event=>{
    if(!dragging)return;
    dragging=false;resizer.classList.remove('dragging');document.body.classList.remove('ft-queue-resizing');
    safeStorageSet(storageKey,String(height));
    try{resizer.releasePointerCapture(event.pointerId);}catch{}
  };
  resizer.onpointerup=finish;resizer.onpointercancel=finish;
  resizer.ondblclick=()=>{height=defaultHeight;apply();safeStorageSet(storageKey,String(height));};
  window.addEventListener('resize',apply);
}

function createSiteShell(title){
  const site=document.createElement('div');site.className='ft-site';
  const head=document.createElement('div');head.className='ft-site-head';
  const label=document.createElement('span');label.className='ft-site-title';label.textContent=title;head.append(label);
  const status=document.createElement('div');status.className='ft-status';
  return {site,head,label,status,entries:[],selected:null,selectedKeys:new Set(),selectionAnchor:'',visibleEntries:[],sort:{key:'type',direction:'asc'},currentPath:'.',emptyText:'Directory is empty'};
}

function attachView(profile,{activate=true,session=null}={}){
  const id=String(profile.id||'');if(views.has(id)){if(activate)activateView(id);return views.get(id);}
  const tabs=document.querySelector('#tabs'),panes=document.querySelector('#panes');if(!tabs||!panes)throw new Error('Workspace tabs are unavailable');

  const tab=document.createElement('button');tab.type='button';tab.className='ft-tab';tab.dataset.id='file-transfer:'+id;
  const tabLabel=document.createElement('span');tabLabel.textContent=String(profile.protocol||'file').toUpperCase()+' · '+(profile.name||'Files');
  const close=document.createElement('span');close.className='close';close.textContent='×';close.onclick=event=>{event.stopPropagation();closeView(id);};
  tab.append(tabLabel,close);tab.onclick=()=>activateView(id);tabs.append(tab);

  const pane=document.createElement('div');pane.className='ft-pane hidden';pane.dataset.id='file-transfer:'+id;
  const head=document.createElement('div');head.className='ft-pane-head';
  const title=document.createElement('div');title.className='ft-pane-title';title.textContent=profile.name||'Remote files';
  const meta=document.createElement('div');meta.className='ft-pane-meta';meta.textContent=String(profile.protocol||'').toUpperCase()+' · dual-pane transfer';
  const spacer=document.createElement('div');spacer.className='ft-pane-spacer';
  const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='Close';closeButton.onclick=()=>closeView(id);head.append(title,meta,spacer,closeButton);

  const sites=document.createElement('div');sites.className='ft-sites';
  const left=createSiteShell('Left');
  const remote=createSiteShell('Remote · '+String(profile.protocol||'').toUpperCase());

  const view={profile,tab,pane,left,remote,remoteCache:new Map()};

  const source=document.createElement('select');source.className='ft-source-select';
  for(const [value,label] of [['host','Host'],['local','Local browser']]){const option=document.createElement('option');option.value=value;option.textContent=label;source.append(option);}
  source.value=safeStorageGet('taskdeck:file-transfer:left-source:'+workspaceKey()+':'+profile.id,'host');
  const rootSelect=document.createElement('select');rootSelect.className='ft-root-select';rootSelect.hidden=true;
  const grantLocal=document.createElement('button');grantLocal.type='button';grantLocal.textContent='Grant';grantLocal.hidden=true;grantLocal.title='Grant read/write access to the selected local folder';
  const chooseLocal=document.createElement('button');chooseLocal.type='button';chooseLocal.textContent='Choose…';chooseLocal.hidden=true;
  left.head.append(source,rootSelect,grantLocal,chooseLocal);
  left.source=source.value;left.rootSelect=rootSelect;left.grantLocal=grantLocal;left.chooseLocal=chooseLocal;left.memoryScope=()=>currentLeftScope(view);
  left.onSelection=()=>updateTransferButtons(view);

  const remoteIdentity=document.createElement('span');remoteIdentity.className='ft-local-note';remoteIdentity.textContent=profile.name||profile.id;
  remote.head.append(remoteIdentity);
  remote.memoryScope=()=> 'remote:'+profile.id;remote.onSelection=()=>updateTransferButtons(view);

  const leftPathBar=pathBar(left,{
    onLoad:path=>loadLeftDirectory(view,path),
    onRefresh:()=>loadLeftDirectory(view,left.currentPath),
    onUp:()=>loadLeftDirectory(view,parentPath(left.currentPath,false))
  });
  const remotePathBar=pathBar(remote,{
    remote:true,onLoad:path=>loadRemoteDirectory(view,path),
    onRefresh:()=>loadRemoteDirectory(view,remote.currentPath,{force:true}),
    onUp:()=>loadRemoteDirectory(view,parentPath(remote.currentPath,true))
  });

  const remoteMkdir=document.createElement('button');remoteMkdir.type='button';remoteMkdir.textContent='New Folder';
  remoteMkdir.onclick=()=>{
    const name=prompt('New remote folder name:','');if(name===null)return;const clean=String(name).trim();if(!clean)return;
    if(clean.includes('/')||clean.includes('\\')){app.showError(new Error('Folder name must not contain path separators'));return;}
    mutateRemote(view,'mkdir',joinPath(remote.currentPath,clean,true)).catch(app.showError);
  };
  remote.head.append(remoteMkdir);

  const leftTable=createSortableTable(left,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));
  const remoteTable=createSortableTable(remote,entry=>remoteDoubleClick(view,entry),(entry,event)=>remoteContext(view,entry,event));
  left.site.append(left.head,leftPathBar,left.status,leftTable);remote.site.append(remote.head,remotePathBar,remote.status,remoteTable);

  const divider=document.createElement('div');divider.className='ft-divider';divider.title='Drag to resize · double-click to reset';
  const tools=document.createElement('div');tools.className='ft-transfer-tools';
  const toRemote=document.createElement('button');toRemote.type='button';toRemote.textContent='→';toRemote.title='Upload selected left item(s) to remote FTP/SFTP';toRemote.disabled=true;
  const toLeft=document.createElement('button');toLeft.type='button';toLeft.textContent='←';toLeft.title='Download selected remote item(s) to left';toLeft.disabled=true;
  tools.append(toRemote,toLeft);divider.append(tools);view.toRemote=toRemote;view.toLeft=toLeft;
  toRemote.onclick=()=>transferLeftToRemote(view).catch(app.showError);toLeft.onclick=()=>transferRemoteToLeft(view).catch(app.showError);

  sites.append(left.site,divider,remote.site);
  const transferQueue=createTransferQueue(view);
  pane.append(head,sites,transferQueue);panes.append(pane);views.set(id,view);installDivider(view,divider,sites);installQueueResizer(view);

  source.onchange=()=>switchLeftSource(view,source.value).catch(app.showError);
  rootSelect.onchange=async()=>{
    const record=await getLocalRoot(rootSelect.value);left.localRoot=record;left.localPermission=false;
    if(record){
      safeStorageSet('taskdeck:file-transfer:last-local-root:'+workspaceKey(),record.id);
      left.localPermission=await ensureHandlePermission(record.handle);
      grantLocal.hidden=left.localPermission;left.currentPath='.';left.pathInput.value='.';left.refreshPathMemory?.();
      await loadLocalDirectory(view,'.');
    }
  };
  grantLocal.onclick=()=>{
    if(!left.localRoot){app.showError(new Error('Choose a local folder first'));return;}
    requestHandlePermission(left.localRoot.handle).then(granted=>{
      if(!granted)throw new Error('Local folder permission was not granted');
      left.localPermission=true;grantLocal.hidden=true;return loadLocalDirectory(view,'.');
    }).catch(app.showError);
  };
  chooseLocal.onclick=()=>chooseLocalRoot(view).then(()=>{grantLocal.hidden=true;return loadLocalDirectory(view,'.');}).catch(app.showError);

  const leftStart=normalizeRelativePath(session?.left_path||'.');
  const remoteStart=normalizeRemotePath(session?.remote_path||profile.initial_path||'.');
  remote.currentPath=remoteStart;remote.pathInput.value=remoteStart;remote.refreshPathMemory();
  switchLeftSource(view,source.value,leftStart).catch(app.showError);
  loadRemoteDirectory(view,remoteStart).catch(app.showError);
  if(activate)activateView(id);
  else persistFileTransferSession();
  return view;
}

async function restoreFileTransferSession(){
  if(sessionRestoreStarted||!fileTransferWorkspaceID())return;
  const saved=readFileTransferSession();sessionRestoreStarted=true;restoringSession=true;
  try{
    if(saved?.open?.length){
      await refreshProfiles();
      for(const item of saved.open){
        const profile=profilesByID.get(item.profile_id);
        if(profile)attachView(profile,{activate:false,session:item});
      }
    }
  }catch(error){
    console.warn('Cannot restore file-transfer session',error);
  }finally{
    restoringSession=false;sessionPersistenceReady=true;
  }
  if(saved?.active_profile_id&&views.has(saved.active_profile_id))activateView(saved.active_profile_id);
  else persistFileTransferSession();
}
async function scheduleFileTransferSessionRestore(){
  if(!fileTransferWorkspaceID()||sessionRestoreStarted)return;
  try{await globalThis.TaskMenuTerminalRestore?.ready;}catch{}
  await restoreFileTransferSession();
}

async function openProfile(profileOrID){
  const id=typeof profileOrID==='string'?profileOrID:profileOrID?.id;if(!id)throw new Error('File-transfer profile is required');
  await scheduleFileTransferSessionRestore();
  await refreshProfiles();const profile=profilesByID.get(String(id))||(typeof profileOrID==='object'?profileOrID:null);
  if(!profile)throw new Error('File-transfer profile not found');return attachView(profile);
}
async function testProfile(profileOrID){
  const id=typeof profileOrID==='string'?profileOrID:profileOrID?.id;if(!id)throw new Error('File-transfer profile is required');
  return app.jsonFetch('/api/file-transfer/test',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:id})});
}
async function testDraft(profileID,profile){
  const body={profile};if(profileID)body.profile_id=profileID;
  return app.jsonFetch('/api/file-transfer/test',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
}

window.addEventListener('taskmenu:view-activated',event=>{
  const detail=event.detail||{},external=detail.kind==='external'&&String(detail.id||'').startsWith('file-transfer:');
  const activeID=external?String(detail.id).slice('file-transfer:'.length):'';
  activeViewID=activeID&&views.has(activeID)?activeID:'';
  for(const [id,view] of views){const active=id===activeViewID;view.tab.classList.toggle('active',active);view.pane.classList.toggle('hidden',!active);}
  persistFileTransferSession();
});
window.addEventListener('taskmenu:tasks',scheduleFileTransferSessionRestore);
setTimeout(scheduleFileTransferSessionRestore,0);

globalThis.TaskMenuFileTransfer={openProfile,testProfile,testDraft,refreshProfiles,getProfile:id=>profilesByID.get(String(id||''))||null,restoreSession:restoreFileTransferSession};
