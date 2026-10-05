const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Explorer');

const style=document.createElement('style');
style.textContent=`
.project-explorer{display:none;position:fixed;top:var(--taskmenu-header-height,30px);bottom:0;left:0;z-index:1800;width:min(370px,92vw);background:#11151b;border-right:1px solid #3b414d;box-shadow:10px 0 28px rgba(0,0,0,.28);flex-direction:column}
.project-explorer.visible{display:flex}
.project-explorer-head{height:42px;display:flex;align-items:center;gap:6px;padding:6px 8px;border-bottom:1px solid #30343b}
.project-explorer-title{font-size:12px;font-weight:700;letter-spacing:.04em;flex:1}
.project-explorer-head button{padding:4px 7px;font-size:11px}
.project-explorer-tree{flex:1;overflow:auto;padding:5px 4px 12px;font:12px ui-monospace,monospace}
.project-explorer-row{display:flex;align-items:center;min-width:0;height:26px;border-radius:4px;padding-right:4px}
.project-explorer-row:hover{background:#232a34}
.project-explorer-row.selected{background:#20334a;outline:1px solid #31577d}
.project-explorer-saved{display:none;border-bottom:1px solid #30343b;padding:5px 7px;max-height:150px;overflow:auto}
.project-explorer-saved.visible{display:block}
.project-explorer-saved-group{display:flex;align-items:center;gap:5px;min-height:24px}
.project-explorer-saved-label{width:64px;flex:0 0 64px;font-size:10px;font-weight:700;opacity:.65;text-transform:uppercase}
.project-explorer-saved-items{display:flex;gap:4px;min-width:0;overflow:auto}
.project-explorer-saved-item{max-width:170px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;padding:3px 6px;font-size:10px}
.project-explorer-context{display:none;position:fixed;z-index:1900;min-width:210px;background:#171c23;border:1px solid #414957;border-radius:6px;box-shadow:0 10px 30px rgba(0,0,0,.35);padding:4px}
.project-explorer-context.visible{display:block}
.project-explorer-context button{display:block;width:100%;border:0;background:transparent;text-align:left;padding:6px 8px;border-radius:4px}
.project-explorer-context button:hover{background:#293341}
.project-explorer-toggle{border:0;background:transparent;padding:2px 4px;min-width:22px}
.project-explorer-name{border:0;background:transparent;text-align:left;flex:1;min-width:0;padding:3px 2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.project-explorer-name.file{opacity:.9}
.project-explorer-children{margin-left:13px;border-left:1px solid #252c35;padding-left:4px}
.project-explorer-message{padding:10px 8px;opacity:.65}
html[data-taskmenu-theme="light"] .project-explorer{background:#fff;border-color:#b9c0c8;box-shadow:10px 0 28px rgba(0,0,0,.12)}
html[data-taskmenu-theme="light"] .project-explorer-row:hover{background:#edf1f5}
html[data-taskmenu-theme="light"] .project-explorer-row.selected{background:#dcecff;outline-color:#9fc3e7}
html[data-taskmenu-theme="light"] .project-explorer-saved{border-color:#dfe3e8}
html[data-taskmenu-theme="light"] .project-explorer-context{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .project-explorer-context button:hover{background:#edf1f5}
html[data-taskmenu-theme="light"] .project-explorer-children{border-color:#dfe3e8}
`;
document.head.append(style);

const panel=document.createElement('div');panel.className='project-explorer';
const head=document.createElement('div');head.className='project-explorer-head';
const title=document.createElement('div');title.className='project-explorer-title';title.textContent='EXPLORER';
const newFileButton=document.createElement('button');newFileButton.type='button';newFileButton.textContent='+F';newFileButton.title='New file in selected folder';
const newFolderButton=document.createElement('button');newFolderButton.type='button';newFolderButton.textContent='+D';newFolderButton.title='New folder in selected folder';
const refresh=document.createElement('button');refresh.type='button';refresh.textContent='↻';refresh.title='Refresh explorer';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close explorer';
const saved=document.createElement('div');saved.className='project-explorer-saved';
const tree=document.createElement('div');tree.className='project-explorer-tree';
const contextMenu=document.createElement('div');contextMenu.className='project-explorer-context';
head.append(title,newFileButton,newFolderButton,refresh,closeButton);panel.append(head,saved,tree);document.body.append(panel,contextMenu);

const loaded=new Map();
const expanded=new Set();
const selected=new Set();
const favorites=new Set();
let recent=[];
let lastSelectedPath='';
let fileClipboard={mode:'',paths:[]};
let rootLoaded=false;
let requestSeq=0;

function storageKey(){return 'vscode-tasks-menu:explorer-expanded:'+(app.taskData?.workspace||'workspace');}
function savedStorageKey(kind){return 'vscode-tasks-menu:explorer-'+kind+':' +(app.taskData?.workspace||'workspace');}
function restoreSaved(){
  favorites.clear();recent=[];
  try{
    const raw=JSON.parse(localStorage.getItem(savedStorageKey('favorites'))||'[]');
    if(Array.isArray(raw))for(const value of raw)if(typeof value==='string'&&value&&value.length<4096)favorites.add(value);
  }catch{}
  try{
    const raw=JSON.parse(localStorage.getItem(savedStorageKey('recent'))||'[]');
    if(Array.isArray(raw))recent=raw.filter(value=>typeof value==='string'&&value&&value.length<4096).slice(0,20);
  }catch{}
}
function persistSaved(){
  try{localStorage.setItem(savedStorageKey('favorites'),JSON.stringify([...favorites].slice(0,100)));}catch{}
  try{localStorage.setItem(savedStorageKey('recent'),JSON.stringify(recent.slice(0,20)));}catch{}
}
function restoreExpanded(){
  expanded.clear();
  try{
    const raw=localStorage.getItem(storageKey());
    const values=raw?JSON.parse(raw):[];
    if(Array.isArray(values))for(const value of values)if(typeof value==='string'&&value.length<4096)expanded.add(value);
  }catch{}
}
function persistExpanded(){
  try{localStorage.setItem(storageKey(),JSON.stringify([...expanded].slice(0,500)));}catch{}
}
function joinPath(parent,name){return parent?parent+'/'+name:name;}
function parentPath(pathValue){const index=pathValue.lastIndexOf('/');return index<0?'':pathValue.slice(0,index);}
function basename(pathValue){const index=pathValue.lastIndexOf('/');return index<0?pathValue:pathValue.slice(index+1);}
function childName(value){
  value=String(value||'').trim();
  if(!value||value==='.'||value==='..'||value.includes('/')||value.includes('\\'))throw new Error('Enter a single file or folder name');
  return value;
}
function topLevelSelectedPaths(paths){
  const ordered=[...new Set(paths)].sort((a,b)=>a.split('/').length-b.split('/').length||a.localeCompare(b));
  return ordered.filter((pathValue,index)=>!ordered.slice(0,index).some(parent=>pathValue.startsWith(parent+'/')));
}
function copyName(pathValue,index=1){
  const name=basename(pathValue);
  const dot=name.lastIndexOf('.');
  const hasExt=dot>0;
  const stem=hasExt?name.slice(0,dot):name;
  const ext=hasExt?name.slice(dot):'';
  return stem+' copy'+(index>1?' '+index:'')+ext;
}
function showMessage(message){tree.replaceChildren();const node=document.createElement('div');node.className='project-explorer-message';node.textContent=message;tree.append(node);}
function rememberRecent(pathValue){
  recent=[pathValue,...recent.filter(value=>value!==pathValue)].slice(0,20);persistSaved();renderSaved();
}
function pinPaths(paths){
  for(const pathValue of paths)favorites.add(pathValue);
  persistSaved();renderSaved();
}
function unpinPaths(paths){
  for(const pathValue of paths)favorites.delete(pathValue);
  persistSaved();renderSaved();
}
function remapPathValue(value,oldPath,newPath){
  if(value===oldPath)return newPath;
  return value.startsWith(oldPath+'/')?newPath+value.slice(oldPath.length):value;
}
function remapStoredPaths(oldPath,newPath){
  const nextFavorites=[...favorites].map(value=>remapPathValue(value,oldPath,newPath));
  favorites.clear();for(const value of nextFavorites)favorites.add(value);
  recent=recent.map(value=>remapPathValue(value,oldPath,newPath));
  const nextExpanded=[...expanded].map(value=>remapPathValue(value,oldPath,newPath));
  expanded.clear();for(const value of nextExpanded)expanded.add(value);
  const nextSelected=[...selected].map(value=>remapPathValue(value,oldPath,newPath));
  selected.clear();for(const value of nextSelected)selected.add(value);
  lastSelectedPath=remapPathValue(lastSelectedPath,oldPath,newPath);
  persistSaved();persistExpanded();
}
function savedButton(pathValue,kind){
  const button=document.createElement('button');button.type='button';button.className='project-explorer-saved-item';button.textContent=basename(pathValue);button.title=pathValue;
  button.onclick=()=>openSavedPath(pathValue,kind==='recent').catch(app.showError);
  return button;
}
function renderSaved(){
  saved.replaceChildren();
  const groups=[['Pinned',[...favorites].slice(0,12),'favorite'],['Recent',recent.slice(0,12),'recent']];
  for(const [label,items,kind] of groups){
    if(!items.length)continue;
    const row=document.createElement('div');row.className='project-explorer-saved-group';
    const caption=document.createElement('div');caption.className='project-explorer-saved-label';caption.textContent=label;
    const host=document.createElement('div');host.className='project-explorer-saved-items';
    for(const pathValue of items)host.append(savedButton(pathValue,kind));
    row.append(caption,host);saved.append(row);
  }
  saved.classList.toggle('visible',Boolean(saved.childElementCount));
}

async function loadDirectory(pathValue,force=false){
  if(!force&&loaded.has(pathValue))return loaded.get(pathValue);
  const seq=++requestSeq;
  const items=await app.jsonFetch('/api/project/tree?path='+encodeURIComponent(pathValue));
  if(seq<requestSeq-200)return [];
  const safe=Array.isArray(items)?items:[];
  loaded.set(pathValue,safe);
  return safe;
}

function render(){
  renderSaved();
  tree.replaceChildren();
  const rootItems=loaded.get('')||[];
  if(!rootLoaded){showMessage('Loading…');return;}
  if(!rootItems.length){showMessage('Workspace has no files');return;}
  for(const item of rootItems)tree.append(renderItem('',item));
}
function renderItem(parent,item){
  const fullPath=joinPath(parent,item.name);
  const wrap=document.createElement('div');
  const row=document.createElement('div');row.className='project-explorer-row';row.dataset.path=fullPath;row.dataset.type=item.type;row.classList.toggle('selected',selected.has(fullPath));
  const toggle=document.createElement('button');toggle.type='button';toggle.className='project-explorer-toggle';
  const name=document.createElement('button');name.type='button';name.className='project-explorer-name '+item.type;name.textContent=item.name;name.title=fullPath;
  row.style.paddingLeft='0px';
  if(item.type==='dir'){
    toggle.textContent=expanded.has(fullPath)?'▾':'▸';toggle.title='Expand '+fullPath;
    toggle.onclick=event=>{event.stopPropagation();toggleDirectory(fullPath);};
    name.onclick=event=>{
      if(event.ctrlKey||event.metaKey||event.shiftKey){selectPath(event,fullPath);return;}
      selectOnly(fullPath);toggleDirectory(fullPath);
    };
  }else{
    toggle.textContent='';toggle.disabled=true;
    name.onclick=event=>{
      if(event.ctrlKey||event.metaKey||event.shiftKey){selectPath(event,fullPath);return;}
      selectOnly(fullPath);openFile(fullPath);
    };
    name.ondblclick=()=>openFile(fullPath);
  }
  row.oncontextmenu=event=>showContextMenu(event,fullPath,item.type);
  row.append(toggle,name);wrap.append(row);
  if(item.type==='dir'&&expanded.has(fullPath)){
    const children=document.createElement('div');children.className='project-explorer-children';
    const list=loaded.get(fullPath);
    if(!list){
      const loading=document.createElement('div');loading.className='project-explorer-message';loading.textContent='Loading…';children.append(loading);
    }else if(!list.length){
      const empty=document.createElement('div');empty.className='project-explorer-message';empty.textContent='Empty';children.append(empty);
    }else{
      for(const child of list)children.append(renderItem(fullPath,child));
    }
    wrap.append(children);
  }
  return wrap;
}
function visiblePaths(){return [...tree.querySelectorAll('.project-explorer-row[data-path]')].map(row=>row.dataset.path).filter(Boolean);}
function selectOnly(pathValue){
  selected.clear();selected.add(pathValue);lastSelectedPath=pathValue;render();
}
function selectPath(event,pathValue){
  const paths=visiblePaths();
  if(event.shiftKey&&lastSelectedPath&&paths.includes(lastSelectedPath)&&paths.includes(pathValue)){
    const a=paths.indexOf(lastSelectedPath),b=paths.indexOf(pathValue);
    if(!(event.ctrlKey||event.metaKey))selected.clear();
    for(const value of paths.slice(Math.min(a,b),Math.max(a,b)+1))selected.add(value);
  }else if(event.ctrlKey||event.metaKey){
    if(selected.has(pathValue))selected.delete(pathValue);else selected.add(pathValue);
    lastSelectedPath=pathValue;
  }else{
    selected.clear();selected.add(pathValue);lastSelectedPath=pathValue;
  }
  render();
}
async function ensurePathVisible(pathValue){
  await ensureRoot(false);
  const parts=pathValue.split('/').filter(Boolean);
  let current='';
  for(let i=0;i<parts.length-1;i++){
    current=joinPath(current,parts[i]);
    expanded.add(current);
    if(!loaded.has(current))await loadDirectory(current);
  }
  persistExpanded();render();
}
async function revealPath(pathValue,{select=true}={}){
  await ensurePathVisible(pathValue);
  if(select){selected.clear();selected.add(pathValue);lastSelectedPath=pathValue;render();}
  requestAnimationFrame(()=>tree.querySelector('.project-explorer-row[data-path="'+CSS.escape(pathValue)+'"]')?.scrollIntoView({block:'nearest'}));
}
function findLoadedItem(pathValue){
  const parent=parentPath(pathValue);
  return (loaded.get(parent)||[]).find(item=>joinPath(parent,item.name)===pathValue)||null;
}
async function openSavedPath(pathValue,preferOpen){
  await revealPath(pathValue);
  const item=findLoadedItem(pathValue);
  if(item?.type==='file'||preferOpen){openFile(pathValue);return;}
  if(item?.type==='dir'&&!expanded.has(pathValue))await toggleDirectory(pathValue);
}
async function openContainingFolder(pathValue){
  const parent=parentPath(pathValue);
  if(!parent){await revealPath(pathValue);return;}
  await revealPath(parent);
  if(!expanded.has(parent))await toggleDirectory(parent);
}
async function projectMutation(action,pathValue,newPath=''){
  return app.jsonFetch('/api/project/mutate',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({action,path:pathValue,new_path:newPath})
  });
}
function selectedCreateDirectory(){
  if(selected.size!==1)return '';
  const pathValue=[...selected][0];
  const item=findLoadedItem(pathValue);
  return item?.type==='dir'?pathValue:parentPath(pathValue);
}
async function createProjectItem(type){
  const parent=selectedCreateDirectory();
  const raw=window.prompt(type==='file'?'New file name:':'New folder name:','');
  if(raw===null)return;
  const name=childName(raw);
  const pathValue=joinPath(parent,name);
  const result=await projectMutation(type==='file'?'create_file':'mkdir',pathValue);
  await reload();
  await revealPath(result.path||pathValue);
  if(type==='file')openFile(result.path||pathValue);
}
async function renameProjectItem(pathValue){
  const raw=window.prompt('Rename '+basename(pathValue)+':',basename(pathValue));
  if(raw===null)return;
  const name=childName(raw);
  const nextPath=joinPath(parentPath(pathValue),name);
  if(nextPath===pathValue)return;
  const result=await projectMutation('rename',pathValue,nextPath);
  const resolved=result.path||nextPath;
  remapStoredPaths(pathValue,resolved);
  window.dispatchEvent(new CustomEvent('taskmenu:project-path-renamed',{detail:{old_path:pathValue,new_path:resolved}}));
  await reload();
  await revealPath(resolved);
}
function setProjectClipboard(mode,paths){
  fileClipboard={mode,paths:topLevelSelectedPaths(paths)};
}
function destinationDirectoryForPath(pathValue,type){
  return type==='dir'?pathValue:parentPath(pathValue);
}
function nextDuplicatePath(source){
  const parent=parentPath(source);
  const existing=new Set((loaded.get(parent)||[]).map(item=>item.name));
  for(let index=1;index<1000;index++){
    const name=copyName(source,index);
    if(!existing.has(name))return joinPath(parent,name);
  }
  throw new Error('Cannot find an available duplicate name');
}
async function duplicateSelectedProjectItems(){
  const sources=topLevelSelectedPaths([...selected]);
  if(!sources.length)return;
  const created=[];
  for(const source of sources){
    const target=nextDuplicatePath(source);
    const result=await projectMutation('copy',source,target);
    created.push(result.path||target);
  }
  await reload();
  selected.clear();for(const pathValue of created)selected.add(pathValue);
  lastSelectedPath=created.at(-1)||'';
  render();
}
async function pasteProjectClipboard(destinationDir){
  const sources=topLevelSelectedPaths(fileClipboard.paths);
  if(!fileClipboard.mode||!sources.length)return;
  const moved=[];
  const created=[];
  for(const source of sources){
    let target=joinPath(destinationDir,basename(source));
    if(target===source){
      if(fileClipboard.mode==='cut')continue;
      target=nextDuplicatePath(source);
    }
    if(fileClipboard.mode==='cut'){
      const result=await projectMutation('rename',source,target);
      const resolved=result.path||target;
      moved.push([source,resolved]);
      remapStoredPaths(source,resolved);
      window.dispatchEvent(new CustomEvent('taskmenu:project-path-renamed',{detail:{old_path:source,new_path:resolved}}));
    }else{
      const result=await projectMutation('copy',source,target);
      created.push(result.path||target);
    }
  }
  if(fileClipboard.mode==='cut')fileClipboard={mode:'',paths:[]};
  await reload();
  selected.clear();
  for(const [,pathValue] of moved)selected.add(pathValue);
  for(const pathValue of created)selected.add(pathValue);
  lastSelectedPath=[...selected].at(-1)||'';
  render();
}
async function moveSelectedProjectItems(){
  const sources=topLevelSelectedPaths([...selected]);
  if(!sources.length)return;
  const raw=window.prompt('Move selected item(s) to project-relative folder. Use . for workspace root:',sources.length===1?parentPath(sources[0])||'.':'.');
  if(raw===null)return;
  const destination=String(raw||'').trim();
  const dir=destination===''||destination==='.'?'':destination.replace(/^\.\//,'').replace(/\/$/,'');
  const moved=[];
  for(const source of sources){
    const nextPath=joinPath(dir,basename(source));
    if(nextPath===source)continue;
    const result=await projectMutation('rename',source,nextPath);
    const resolved=result.path||nextPath;
    moved.push([source,resolved]);
    remapStoredPaths(source,resolved);
    window.dispatchEvent(new CustomEvent('taskmenu:project-path-renamed',{detail:{old_path:source,new_path:resolved}}));
  }
  await reload();
  selected.clear();
  for(const [,nextPath] of moved)selected.add(nextPath);
  lastSelectedPath=moved.at(-1)?.[1]||'';
  render();
}
function closeContextMenu(){contextMenu.classList.remove('visible');contextMenu.replaceChildren();}
function contextAction(label,run){
  const button=document.createElement('button');button.type='button';button.textContent=label;
  button.onclick=()=>{closeContextMenu();Promise.resolve(run()).catch(app.showError);};
  contextMenu.append(button);
}
function showContextMenu(event,pathValue,type){
  event.preventDefault();event.stopPropagation();
  if(!selected.has(pathValue)){selected.clear();selected.add(pathValue);lastSelectedPath=pathValue;render();}
  closeContextMenu();
  const paths=[...selected];
  if(type==='file'&&paths.length===1)contextAction('Open',()=>openFile(pathValue));
  contextAction(paths.length>1?'Copy selected':'Copy',()=>setProjectClipboard('copy',paths));
  contextAction(paths.length>1?'Cut selected':'Cut',()=>setProjectClipboard('cut',paths));
  contextAction(paths.length>1?'Duplicate selected':'Duplicate',()=>duplicateSelectedProjectItems());
  if(fileClipboard.paths.length){
    const destination=destinationDirectoryForPath(pathValue,type);
    contextAction('Paste '+(fileClipboard.mode==='cut'?'move':'copy')+' here',()=>pasteProjectClipboard(destination));
  }
  if(paths.length===1)contextAction('Rename…',()=>renameProjectItem(pathValue));
  contextAction(paths.length>1?'Move selected…':'Move…',()=>moveSelectedProjectItems());
  if(type==='dir'&&paths.length===1){
    contextAction('New file here…',()=>createProjectItem('file'));
    contextAction('New folder here…',()=>createProjectItem('dir'));
  }
  contextAction('Reveal in Explorer',()=>revealPath(pathValue));
  contextAction('Open containing folder',()=>openContainingFolder(pathValue));
  if(paths.every(value=>favorites.has(value)))contextAction(paths.length>1?'Unpin selected':'Unpin',()=>unpinPaths(paths));
  else contextAction(paths.length>1?'Pin selected':'Pin',()=>pinPaths(paths));
  if(paths.length>1)contextAction('Clear selection',()=>{selected.clear();lastSelectedPath='';render();});
  contextMenu.style.left=Math.min(event.clientX,window.innerWidth-230)+'px';
  contextMenu.style.top=Math.min(event.clientY,window.innerHeight-220)+'px';
  contextMenu.classList.add('visible');
}
async function toggleDirectory(pathValue){
  if(expanded.has(pathValue)){
    expanded.delete(pathValue);persistExpanded();render();return;
  }
  expanded.add(pathValue);persistExpanded();render();
  if(!loaded.has(pathValue)){
    try{await loadDirectory(pathValue);if(expanded.has(pathValue))render();}
    catch(error){expanded.delete(pathValue);persistExpanded();render();app.showError(error);}
  }
}
function openFile(pathValue){
  window.dispatchEvent(new CustomEvent('taskmenu:project-file-open-request',{detail:{path:pathValue,source:'explorer'}}));
}
async function restoreExpandedDirectories(){
  let changed=false;
  const paths=[...expanded].sort((a,b)=>a.split('/').length-b.split('/').length||a.localeCompare(b));
  for(const pathValue of paths){
    if(loaded.has(pathValue))continue;
    try{await loadDirectory(pathValue);}
    catch{expanded.delete(pathValue);changed=true;}
  }
  if(changed)persistExpanded();
}
async function ensureRoot(force=false){
  if(force){loaded.clear();rootLoaded=false;showMessage('Loading…');}
  if(rootLoaded&&!force)return;
  try{
    await loadDirectory('',force);rootLoaded=true;render();
    await restoreExpandedDirectories();
    render();
  }catch(error){rootLoaded=false;showMessage('Explorer unavailable');throw error;}
}
function open(){
  panel.classList.add('visible');restoreExpanded();restoreSaved();renderSaved();ensureRoot(false).catch(app.showError);
}
function close(){panel.classList.remove('visible');}
async function reload(){try{await ensureRoot(true);}catch(error){app.showError(error);}}

newFileButton.onclick=()=>createProjectItem('file').catch(app.showError);
newFolderButton.onclick=()=>createProjectItem('dir').catch(app.showError);
refresh.onclick=reload;
closeButton.onclick=close;
document.addEventListener('keydown',event=>{if(event.key==='Escape'){closeContextMenu();if(panel.classList.contains('visible'))close();}});
document.addEventListener('pointerdown',event=>{if(contextMenu.classList.contains('visible')&&!contextMenu.contains(event.target))closeContextMenu();},true);
window.addEventListener('taskmenu:project-file-opened',event=>{
  const pathValue=String(event.detail?.path||'').trim();
  if(!pathValue)return;
  rememberRecent(pathValue);
  if(panel.classList.contains('visible'))revealPath(pathValue).catch(app.showError);
});

globalThis.TaskMenuExplorer={open,close,reload,reveal:revealPath,get selectedPaths(){return [...selected];},get favorites(){return [...favorites];},get recent(){return [...recent];},get clipboard(){return {mode:fileClipboard.mode,paths:[...fileClipboard.paths]};}};
