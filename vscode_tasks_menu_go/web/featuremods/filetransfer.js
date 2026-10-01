const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for file transfer workspace');

const views=new Map();
let profilesByID=new Map();
let contextMenu=null;

const localDBName='TaskDeckFileTransfer';
const localDBStore='localRoots';
const localDBVersion=1;
const maxRecentPaths=12;
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
.ft-source-select,.ft-root-select,.ft-favorite-select{height:28px;min-height:28px;padding:3px 6px;font-size:11px}
.ft-root-select{max-width:210px}.ft-favorite-select{max-width:180px}
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
.ft-name{max-width:480px;overflow:hidden;text-overflow:ellipsis}.ft-kind{display:inline-block;min-width:17px;margin-right:5px;opacity:.78}
.ft-size{text-align:right!important;font-family:ui-monospace,monospace}.ft-type{opacity:.72}.ft-modified{font-family:ui-monospace,monospace;font-size:10px}
.ft-empty{padding:20px;opacity:.55}
.ft-divider{position:relative;cursor:col-resize;background:#171b22;border-left:1px solid #30343b;border-right:1px solid #30343b;touch-action:none}
.ft-divider:hover,.ft-divider.dragging{background:#26303c}
.ft-transfer-tools{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);display:flex;flex-direction:column;gap:8px;z-index:4}
.ft-transfer-tools button{width:34px;height:34px;padding:0;border-radius:50%;font-size:17px;background:#202a36;box-shadow:0 2px 7px rgba(0,0,0,.35)}
.ft-transfer-tools button:disabled{opacity:.28}
body.ft-resizing{user-select:none;cursor:col-resize}
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
html[data-taskmenu-theme="light"] .ft-pathbar input[type=text]{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-table th{background:#e9eef3}
html[data-taskmenu-theme="light"] .ft-table tbody tr:hover{background:#eef2f6}
html[data-taskmenu-theme="light"] .ft-table tbody tr.selected{background:#dde8f3}
html[data-taskmenu-theme="light"] .ft-context{background:#fff;border-color:#b9c0c8}
@media(max-width:850px){.ft-sites{grid-template-columns:1fr;grid-template-rows:minmax(220px,1fr) 8px minmax(220px,1fr)}.ft-divider{cursor:row-resize;border-left:0;border-right:0;border-top:1px solid #30343b;border-bottom:1px solid #30343b}.ft-transfer-tools{flex-direction:row}.ft-transfer-tools button:first-child{transform:rotate(90deg)}.ft-transfer-tools button:last-child{transform:rotate(90deg)}}
`;
document.head.append(style);

function workspaceKey(){return String(app.taskData?.workspace||'workspace');}
function safeStorageGet(key,fallback=''){try{return localStorage.getItem(key)??fallback;}catch{return fallback;}}
function safeStorageSet(key,value){try{localStorage.setItem(key,value);}catch(error){console.warn('Cannot persist file-transfer setting',error);}}

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
  const placeholder=document.createElement('option');placeholder.value='';placeholder.textContent='★ Saved paths';select.append(placeholder);
  const addGroup=(label,values)=>{
    if(!values.length)return;
    const group=document.createElement('optgroup');group.label=label;
    for(const value of values){const option=document.createElement('option');option.value=value;option.textContent=value;group.append(option);}
    select.append(group);
  };
  addGroup('Favorites',memory.favorites);
  addGroup('Recent',memory.recent.filter(item=>!memory.favorites.includes(item)));
  select.value='';
  const saved=memory.favorites.includes(current);
  star.classList.toggle('saved',saved);star.textContent=saved?'★':'☆';
  star.title=saved?'Remove current path from favorites':'Save current path to favorites';
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
  app.activateExternalView('file-transfer:'+id);
  for(const [otherID,other] of views){
    const active=otherID===id;other.tab.classList.toggle('active',active);other.pane.classList.toggle('hidden',!active);
  }
}
function closeView(id){const view=views.get(id);if(!view)return;view.tab.remove();view.pane.remove();views.delete(id);}
function closeContextMenu(){contextMenu?.remove();contextMenu=null;}
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
  await loadRemoteDirectory(view,view.remote.currentPath);
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
  if(!entries.length){
    const tr=document.createElement('tr'),td=document.createElement('td');td.colSpan=5;td.className='ft-empty';td.textContent=panel.emptyText||'Directory is empty';tr.append(td);panel.tbody.append(tr);syncSelectionRows(panel);return;
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
  const bar=document.createElement('div');bar.className='ft-pathbar';
  const up=document.createElement('button');up.type='button';up.textContent='↑';up.title='Parent directory';
  const input=document.createElement('input');input.type='text';input.spellcheck=false;input.autocomplete='off';
  const favorites=document.createElement('select');favorites.className='ft-favorite-select';favorites.title='Favorite and recent paths';
  const star=document.createElement('button');star.type='button';star.className='ft-favorite-toggle';star.textContent='☆';
  const go=document.createElement('button');go.type='button';go.textContent='Go';
  const refresh=document.createElement('button');refresh.type='button';refresh.textContent='↻';refresh.title='Refresh';
  bar.append(up,input,favorites,star,go,refresh);
  panel.pathInput=input;panel.favoriteSelect=favorites;panel.favoriteToggle=star;panel.refresh=refresh;
  const reloadMemory=()=>populatePathMemory(favorites,star,panel.memoryScope?.()||'',input.value||'.');
  favorites.onchange=()=>{if(favorites.value){input.value=favorites.value;onLoad(favorites.value).catch(app.showError);}favorites.value='';};
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

async function loadRemoteDirectory(view,path){
  const panel=view.remote;path=normalizeRemotePath(path||profileFor(view)?.initial_path||'.');
  panel.status.textContent='Loading '+path+'…';panel.refresh.disabled=true;
  try{
    const data=await app.jsonFetch('/api/file-transfer/list',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:view.profile.id,path})});
    panel.entries=(Array.isArray(data?.entries)?data.entries:[]).map(item=>({...item,type:entryType(item)}));resetPanelSelection(panel);
    markPathLoaded(panel,String(data?.path||path));renderTable(panel,entry=>remoteDoubleClick(view,entry),(entry,event)=>remoteContext(view,entry,event));
    panel.status.textContent=(data?.protocol||view.profile.protocol).toUpperCase()+' · '+panel.entries.length+' item(s)';
  }catch(error){panel.status.textContent=String(error?.message||error);throw error;}
  finally{panel.refresh.disabled=false;updateTransferButtons(view);}
}

async function loadHostDirectory(view,path){
  const panel=view.left;path=normalizeRelativePath(path||'.');panel.status.textContent='Loading host '+path+'…';panel.refresh.disabled=true;
  try{
    const query=path==='.'?'':path;
    const data=await app.jsonFetch('/api/project/tree?path='+encodeURIComponent(query));
    panel.entries=(Array.isArray(data)?data:[]).map(item=>({...item,type:item.type==='dir'?'directory':entryType(item)}));resetPanelSelection(panel);
    markPathLoaded(panel,path);renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));
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
    panel.entries=entries;resetPanelSelection(panel);panel.currentLocalHandle=dir;markPathLoaded(panel,path);
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

async function switchLeftSource(view,source){
  const panel=view.left;panel.source=source==='local'?'local':'host';
  safeStorageSet('taskdeck:file-transfer:left-source:'+workspaceKey()+':'+view.profile.id,panel.source);
  panel.rootSelect.hidden=panel.source!=='local';panel.chooseLocal.hidden=panel.source!=='local';
  panel.pathInput.placeholder=panel.source==='local'?'Path relative to selected local folder':'Path relative to TaskDeck workspace';
  panel.currentPath='.';panel.entries=[];resetPanelSelection(panel);panel.pathInput.value='.';
  if(panel.source==='local'){
    if(typeof globalThis.showDirectoryPicker!=='function'){panel.grantLocal.hidden=true;panel.status.textContent='Local browser requires File System Access API (Chromium, HTTPS/localhost).';panel.entries=[];renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));updateTransferButtons(view);return;}
    await refreshLocalRoots(view);
    panel.grantLocal.hidden=!panel.localRoot||panel.localPermission;
  }else panel.grantLocal.hidden=true;
  panel.refreshPathMemory?.();
  await loadLeftDirectory(view,'.');
}

async function localSelectedFile(view,entry){
  const panel=view.left;if(panel.source!=='local'||!panel.localRoot)throw new Error('Local folder is not selected');
  const dir=await directoryHandleForPath(panel.localRoot.handle,panel.currentPath);
  const handle=await dir.getFileHandle(entry.name);return handle.getFile();
}

async function uploadBrowserFile(view,file){
  const target=joinPath(view.remote.currentPath,file.name,true),form=new FormData();
  form.append('profile_id',view.profile.id);form.append('path',target);form.append('file',file,file.name);
  view.remote.status.textContent='Uploading '+file.name+'…';
  const response=await app.fetchWithLease('/api/file-transfer/upload',{method:'POST',body:form,cache:'no-store'});
  if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
}

async function transferLeftEntriesToRemote(view,entries){
  const files=(entries||[]).filter(entry=>entryType(entry)==='file');
  if(!files.length)throw new Error('Select one or more files on the left first');
  view.remote.status.textContent='Transferring '+files.length+' file(s) to remote…';
  for(const entry of files){
    const remotePath=joinPath(view.remote.currentPath,entry.name,true);
    if(view.left.source==='host'){
      const hostPath=joinPath(view.left.currentPath,entry.name,false);
      await app.jsonFetch('/api/file-transfer/host-to-remote',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({
        profile_id:view.profile.id,host_path:hostPath,remote_path:remotePath
      })});
    }else{
      const file=await localSelectedFile(view,entry);await uploadBrowserFile(view,file);
    }
  }
  await loadRemoteDirectory(view,view.remote.currentPath);
}
async function transferLeftToRemote(view){return transferLeftEntriesToRemote(view,selectedFiles(view.left));}

async function writeRemoteToLocal(view,entry){
  const panel=view.left;if(!panel.localRoot)throw new Error('Choose a local folder first');
  const granted=await ensureHandlePermission(panel.localRoot.handle);if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
  const dir=await directoryHandleForPath(panel.localRoot.handle,panel.currentPath);
  let exists=false;
  try{await dir.getFileHandle(entry.name);exists=true;}catch(error){if(error?.name!=='NotFoundError')throw error;}
  if(exists&&!confirm(entry.name+' already exists locally. Overwrite it?'))return false;
  const handle=await dir.getFileHandle(entry.name,{create:true}),writable=await handle.createWritable();
  try{
    const ticket=await issueDownloadTicket(view,joinPath(view.remote.currentPath,entry.name,true));
    const response=await fetch('/api/file-transfer/download?ticket='+encodeURIComponent(ticket),{cache:'no-store'});
    if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
    if(response.body&&typeof response.body.pipeTo==='function')await response.body.pipeTo(writable);
    else{await writable.write(await response.arrayBuffer());await writable.close();}
    return true;
  }catch(error){try{await writable.abort();}catch{}throw error;}
}

async function transferRemoteEntriesToLeft(view,entries){
  const files=(entries||[]).filter(entry=>entryType(entry)==='file');
  if(!files.length)throw new Error('Select one or more remote files first');
  view.left.status.textContent='Transferring '+files.length+' file(s) from remote…';
  for(const entry of files){
    if(view.left.source==='host'){
      const payload={profile_id:view.profile.id,remote_path:joinPath(view.remote.currentPath,entry.name,true),host_dir:view.left.currentPath,overwrite:false};
      let response=await app.fetchWithLease('/api/file-transfer/remote-to-host',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),cache:'no-store'});
      if(response.status===409){
        const message=(await response.text()).trim();
        if(message.includes('already exists')&&confirm(entry.name+' already exists on Host. Overwrite it?')){
          payload.overwrite=true;response=await app.fetchWithLease('/api/file-transfer/remote-to-host',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),cache:'no-store'});
        }else continue;
      }
      if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
    }else await writeRemoteToLocal(view,entry);
  }
  await loadLeftDirectory(view,view.left.currentPath);
}
async function transferRemoteToLeft(view){return transferRemoteEntriesToLeft(view,selectedFiles(view.remote));}

async function deleteRemoteEntries(view,entries){
  const selected=[...(entries||[])];if(!selected.length)return;
  const dirs=selected.filter(entry=>entryType(entry)==='directory').length;
  const message=selected.length===1
    ?'Delete '+selected[0].name+'?'+(dirs?'\n\nDirectory removal is non-recursive.':'')
    :'Delete '+selected.length+' selected item(s)?'+(dirs?'\n\nSelected directories are removed non-recursively and must be empty.':'');
  if(!confirm(message))return;
  for(const entry of selected){
    await mutateRemoteRequest(view,'delete',joinPath(view.remote.currentPath,entry.name,true),'',entryType(entry)==='directory');
  }
  await loadRemoteDirectory(view,view.remote.currentPath);
}

async function copyText(value){
  const text=String(value??'');
  if(navigator.clipboard?.writeText){await navigator.clipboard.writeText(text);return;}
  const area=document.createElement('textarea');area.value=text;area.style.position='fixed';area.style.opacity='0';document.body.append(area);area.select();
  try{document.execCommand('copy');}finally{area.remove();}
}

function updateTransferButtons(view){
  view.toRemote.disabled=selectedFiles(view.left).length===0;
  view.toLeft.disabled=selectedFiles(view.remote).length===0;
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
  const panel=view.left,selected=selectedEntries(panel),files=selected.filter(item=>entryType(item)==='file'),items=[];
  if(selected.length===1&&entryType(selected[0])==='directory'){
    items.push({label:'Open folder',action:()=>loadLeftDirectory(view,joinPath(panel.currentPath,selected[0].name,false))});
  }
  if(files.length){
    items.push({label:'Upload '+(files.length>1?files.length+' selected files':'to remote')+' →',action:()=>transferLeftEntriesToRemote(view,files)});
  }
  if(panel.source==='host'&&selected.length===1&&entryType(selected[0])==='file'){
    items.push({label:'Download host file to browser',action:()=>{downloadFrame().src='/api/files/download?path='+encodeURIComponent(joinPath(panel.currentPath,selected[0].name,false));}});
  }
  if(items.length)items.push({separator:true});
  items.push(...commonSelectionMenu(panel,false,()=>loadLeftDirectory(view,panel.currentPath)));
  showContextMenu(items,event.clientX,event.clientY,contextTitle(panel,entry));
}
function remoteContext(view,entry,event){
  const panel=view.remote,selected=selectedEntries(panel),files=selected.filter(item=>entryType(item)==='file'),items=[];
  if(selected.length===1&&entryType(selected[0])==='directory'){
    items.push({label:'Open folder',action:()=>loadRemoteDirectory(view,joinPath(panel.currentPath,selected[0].name,true))});
  }
  if(files.length)items.push({label:'Transfer '+(files.length>1?files.length+' selected files':'to left')+' ←',action:()=>transferRemoteEntriesToLeft(view,files)});
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
  items.push({separator:true},...commonSelectionMenu(panel,true,()=>loadRemoteDirectory(view,panel.currentPath)));
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

function createSiteShell(title){
  const site=document.createElement('div');site.className='ft-site';
  const head=document.createElement('div');head.className='ft-site-head';
  const label=document.createElement('span');label.className='ft-site-title';label.textContent=title;head.append(label);
  const status=document.createElement('div');status.className='ft-status';
  return {site,head,label,status,entries:[],selected:null,selectedKeys:new Set(),selectionAnchor:'',visibleEntries:[],sort:{key:'type',direction:'asc'},currentPath:'.',emptyText:'Directory is empty'};
}

function attachView(profile){
  const id=String(profile.id||'');if(views.has(id)){activateView(id);return views.get(id);}
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

  const view={profile,tab,pane,left,remote};

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
    onRefresh:()=>loadRemoteDirectory(view,remote.currentPath),
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
  const toRemote=document.createElement('button');toRemote.type='button';toRemote.textContent='→';toRemote.title='Transfer selected left file to remote';toRemote.disabled=true;
  const toLeft=document.createElement('button');toLeft.type='button';toLeft.textContent='←';toLeft.title='Transfer selected remote file to left';toLeft.disabled=true;
  tools.append(toRemote,toLeft);divider.append(tools);view.toRemote=toRemote;view.toLeft=toLeft;
  toRemote.onclick=()=>transferLeftToRemote(view).catch(app.showError);toLeft.onclick=()=>transferRemoteToLeft(view).catch(app.showError);

  sites.append(left.site,divider,remote.site);pane.append(head,sites);panes.append(pane);views.set(id,view);installDivider(view,divider,sites);

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

  activateView(id);
  remote.currentPath=profile.initial_path||'.';remote.pathInput.value=remote.currentPath;remote.refreshPathMemory();
  switchLeftSource(view,source.value).catch(app.showError);
  loadRemoteDirectory(view,profile.initial_path||'.').catch(app.showError);
  return view;
}

async function openProfile(profileOrID){
  const id=typeof profileOrID==='string'?profileOrID:profileOrID?.id;if(!id)throw new Error('File-transfer profile is required');
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
  for(const [id,view] of views){const active=id===activeID;view.tab.classList.toggle('active',active);view.pane.classList.toggle('hidden',!active);}
});

globalThis.TaskMenuFileTransfer={openProfile,testProfile,testDraft,refreshProfiles,getProfile:id=>profilesByID.get(String(id||''))||null};
