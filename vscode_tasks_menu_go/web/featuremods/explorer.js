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
.project-explorer-root{margin:5px 2px 2px;padding:4px 5px;border:1px solid #303844;border-radius:5px;background:#171d25;display:flex;align-items:center;gap:5px;font:11px ui-monospace,monospace;font-weight:700}
.project-explorer-root-name{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.project-explorer-root button{padding:2px 5px;font-size:10px}
.project-explorer-row:hover{background:#232a34}
.project-explorer-row.selected{background:#20334a;outline:1px solid #31577d}
.project-explorer-row.drag-over{background:#294565;outline:1px dashed #6f9bc7}
.project-explorer-tree.drag-over-root{outline:1px dashed #6f9bc7;outline-offset:-3px}
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
.project-explorer-context-separator{height:1px;margin:4px 3px;background:#303844}
.project-explorer-toggle{border:0;background:transparent;padding:2px 4px;min-width:22px}
.project-explorer-name{border:0;background:transparent;text-align:left;flex:1;min-width:0;padding:3px 2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.project-explorer-name.file{opacity:.9}
.project-explorer-git{min-width:20px;padding:1px 4px;border-radius:4px;text-align:center;font:10px ui-monospace,monospace;font-weight:700;opacity:.9}
.project-explorer-git.status-M{color:#e5b85c}.project-explorer-git.status-A{color:#72cf8a}.project-explorer-git.status-D{color:#ef7d88}.project-explorer-git.status-U{color:#ff8c8c;background:#4a2228}.project-explorer-git.status-q{color:#7eb6f0}.project-explorer-git.status-R,.project-explorer-git.status-C{color:#c596e8}.project-explorer-git.status-T{color:#e5b85c}.project-explorer-git.dir{opacity:.52;font-weight:500}
.project-explorer-children{margin-left:13px;border-left:1px solid #252c35;padding-left:4px}
.project-explorer-message{padding:10px 8px;opacity:.65}
html[data-taskmenu-theme="light"] .project-explorer{background:#fff;border-color:#b9c0c8;box-shadow:10px 0 28px rgba(0,0,0,.12)}
html[data-taskmenu-theme="light"] .project-explorer-row:hover{background:#edf1f5}
html[data-taskmenu-theme="light"] .project-explorer-root{background:#f6f8fa;border-color:#d8dee4}
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
const rootButton=document.createElement('button');rootButton.type='button';rootButton.textContent='+R';rootButton.title='Attach workspace root';
const newFileButton=document.createElement('button');newFileButton.type='button';newFileButton.textContent='+F';newFileButton.title='New file in selected folder';
const newFolderButton=document.createElement('button');newFolderButton.type='button';newFolderButton.textContent='+D';newFolderButton.title='New folder in selected folder';
const undoButton=document.createElement('button');undoButton.type='button';undoButton.textContent='↶';undoButton.title='Undo last Explorer file operation';undoButton.disabled=true;
const refresh=document.createElement('button');refresh.type='button';refresh.textContent='↻';refresh.title='Refresh explorer';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close explorer';
const saved=document.createElement('div');saved.className='project-explorer-saved';
const tree=document.createElement('div');tree.className='project-explorer-tree';
const contextMenu=document.createElement('div');contextMenu.className='project-explorer-context';
head.append(title,rootButton,newFileButton,newFolderButton,undoButton,refresh,closeButton);panel.append(head,saved,tree);document.body.append(panel,contextMenu);

const loaded=new Map();
const expanded=new Set();
const selected=new Set();
const favorites=new Set();
const gitStatusByPath=new Map();
let gitStatusAvailable=false;
let recent=[];
let lastSelectedPath='';
let fileClipboard={mode:'',paths:[]};
let lastUndo=null;
let dragPaths=[];
let rootLoaded=false;
let workspaceRoots=[];
let attachedRootsEnabled=false;
let requestSeq=0;

function storageKey(){return 'vscode-tasks-menu:explorer-expanded:'+(app.taskData?.workspace||'workspace');}
function savedStorageKey(kind){return 'vscode-tasks-menu:explorer-'+kind+':' +(app.taskData?.workspace||'workspace');}
function undoStorageKey(){return savedStorageKey('undo');}
function updateUndoButton(){
  undoButton.disabled=!lastUndo?.steps?.length;
  undoButton.title=lastUndo?.label?'Undo: '+lastUndo.label:'Undo last Explorer file operation';
}
function restoreUndoRecord(){
  lastUndo=null;
  try{
    const value=JSON.parse(localStorage.getItem(undoStorageKey())||'null');
    if(value&&typeof value.label==='string'&&Array.isArray(value.steps)&&value.steps.length<=200)lastUndo=value;
  }catch{}
  updateUndoButton();
}
function recordUndo(label,steps){
  lastUndo={label:String(label||'file operation'),steps:Array.isArray(steps)?steps:[]};
  try{localStorage.setItem(undoStorageKey(),JSON.stringify(lastUndo));}catch{}
  updateUndoButton();
}
function clearUndo(){
  lastUndo=null;
  try{localStorage.removeItem(undoStorageKey());}catch{}
  updateUndoButton();
}
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
function rootBasePath(root){return root?.primary?'':'@root/'+String(root?.id||'');}
function splitWorkspacePath(pathValue){
  pathValue=String(pathValue||'').trim();
  const match=pathValue.match(/^@root\/([^/]+)(?:\/(.*))?$/);
  return match?{base:'@root/'+match[1],relative:match[2]||''}:{base:'',relative:pathValue};
}
async function loadWorkspaceRoots(){
  const data=await app.jsonFetch('/api/workspace-roots');
  workspaceRoots=Array.isArray(data?.roots)?data.roots:[];
  attachedRootsEnabled=Boolean(data?.attached_enabled);
  rootButton.hidden=!attachedRootsEnabled;
  return workspaceRoots;
}
async function attachWorkspaceRoot(){
  if(!attachedRootsEnabled)return;
  const path=window.prompt('Workspace root folder path (absolute, or relative to the primary workspace):','');
  if(path===null||!String(path).trim())return;
  const name=window.prompt('Display name (optional):','');
  const body={path:String(path).trim()};if(name!==null&&String(name).trim())body.name=String(name).trim();
  await app.jsonFetch('/api/workspace-roots',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  await reload();
}
async function renameWorkspaceRoot(root){
  const name=window.prompt('Rename workspace root:',root?.name||'');
  if(name===null||!String(name).trim())return;
  await app.jsonFetch('/api/workspace-roots',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:root.id,name:String(name).trim()})});
  await reload();
}
async function detachWorkspaceRoot(root){
  if(!window.confirm('Detach workspace root "'+String(root?.name||root?.id||'')+'" from TaskDeck?\n\nFiles on disk will NOT be deleted.'))return;
  await app.jsonFetch('/api/workspace-roots?id='+encodeURIComponent(root.id),{method:'DELETE'});
  const base=rootBasePath(root);
  for(const key of [...loaded.keys()])if(key===base||key.startsWith(base+'/'))loaded.delete(key);
  for(const key of [...expanded])if(key===base||key.startsWith(base+'/'))expanded.delete(key);
  for(const key of [...selected])if(key===base||key.startsWith(base+'/'))selected.delete(key);
  persistExpanded();await reload();
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
function removeStoredPath(pathValue){
  const matches=value=>value===pathValue||value.startsWith(pathValue+'/');
  for(const value of [...favorites])if(matches(value))favorites.delete(value);
  recent=recent.filter(value=>!matches(value));
  for(const value of [...expanded])if(matches(value))expanded.delete(value);
  for(const value of [...selected])if(matches(value))selected.delete(value);
  if(matches(lastSelectedPath))lastSelectedPath='';
  persistSaved();persistExpanded();
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

async function loadGitStatus(){
  try{
    const data=await app.jsonFetch('/api/project/git-status');
    gitStatusByPath.clear();
    gitStatusAvailable=Boolean(data?.available);
    for(const change of Array.isArray(data?.changes)?data.changes:[]){
      const pathValue=String(change?.path||'').trim();
      const status=String(change?.status||'').trim();
      if(pathValue&&status)gitStatusByPath.set(pathValue,{...change,status});
    }
    return true;
  }catch{
    gitStatusByPath.clear();gitStatusAvailable=false;return false;
  }
}
function gitBadgeForPath(pathValue,type){
  const exact=gitStatusByPath.get(pathValue);
  if(exact)return {text:exact.status,title:[exact.status,pathValue,exact.repository&&('repo '+exact.repository)].filter(Boolean).join(' · '),className:'status-'+(exact.status==='?'?'q':exact.status)};
  if(type!=='dir')return null;
  const prefix=pathValue+'/';
  let count=0;
  for(const key of gitStatusByPath.keys())if(key.startsWith(prefix))count++;
  if(!count)return null;
  return {text:String(count),title:count+' changed path'+(count===1?'':'s')+' under '+pathValue,className:'dir'};
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
  if(!rootLoaded){showMessage('Loading…');return;}
  if(!workspaceRoots.length){showMessage('Workspace has no roots');return;}
  for(const root of workspaceRoots){
    const base=rootBasePath(root);
    const header=document.createElement('div');header.className='project-explorer-root';header.dataset.rootId=root.id;
    const name=document.createElement('div');name.className='project-explorer-root-name';name.textContent=root.name+(root.primary?' · primary':'');name.title=root.available===false?'Root unavailable':(root.name||root.id);
    header.append(name);
    if(!root.primary&&root.attached){
      const actions=document.createElement('button');actions.type='button';actions.textContent='⋯';actions.title='Workspace root actions';
      actions.onclick=event=>{
        event.stopPropagation();
        const action=window.prompt('Root action: rename or detach','rename');
        if(action===null)return;
        if(String(action).trim().toLowerCase()==='detach')detachWorkspaceRoot(root).catch(app.showError);
        else if(String(action).trim().toLowerCase()==='rename')renameWorkspaceRoot(root).catch(app.showError);
      };
      header.append(actions);
    }
    tree.append(header);
    if(root.available===false){
      const unavailable=document.createElement('div');unavailable.className='project-explorer-message';unavailable.textContent='Root unavailable';tree.append(unavailable);continue;
    }
    const rootItems=loaded.get(base)||[];
    if(!rootItems.length){
      const empty=document.createElement('div');empty.className='project-explorer-message';empty.textContent='Empty';tree.append(empty);continue;
    }
    const children=document.createElement('div');children.className='project-explorer-root-children';
    for(const item of rootItems)children.append(renderItem(base,item));
    tree.append(children);
  }
}
function renderItem(parent,item){
  const fullPath=joinPath(parent,item.name);
  const wrap=document.createElement('div');
  const row=document.createElement('div');row.className='project-explorer-row';row.dataset.path=fullPath;row.dataset.type=item.type;row.classList.toggle('selected',selected.has(fullPath));
  const toggle=document.createElement('button');toggle.type='button';toggle.className='project-explorer-toggle';
  const name=document.createElement('button');name.type='button';name.className='project-explorer-name '+item.type;name.textContent=item.name;name.title=fullPath;
  const gitBadge=document.createElement('span');gitBadge.className='project-explorer-git';
  const badge=gitBadgeForPath(fullPath,item.type);
  if(badge){gitBadge.textContent=badge.text;gitBadge.title=badge.title;gitBadge.classList.add(badge.className);}else gitBadge.hidden=true;
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
  row.draggable=true;
  row.ondragstart=event=>startExplorerDrag(event,fullPath,row);
  row.ondragend=clearExplorerDrag;
  row.ondragover=event=>dragOverExplorerRow(event,fullPath,item.type,row);
  row.ondragleave=()=>row.classList.remove('drag-over');
  row.ondrop=event=>dropExplorerRow(event,fullPath,item.type,row).catch(app.showError);
  row.append(toggle,name,gitBadge);wrap.append(row);
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
  const split=splitWorkspacePath(pathValue);
  const parts=split.relative.split('/').filter(Boolean);
  let current=split.base;
  if(!loaded.has(current))await loadDirectory(current);
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
async function projectMutation(action,pathValue,newPath='',token=''){
  return app.jsonFetch('/api/project/mutate',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({action,path:pathValue,new_path:newPath,token})
  });
}
function pathTouchesEditor(source,editorPath){
  return editorPath===source||editorPath.startsWith(source+'/');
}
function assertNoDirtyEditors(paths){
  const editor=globalThis.TaskMenuEditor;
  if(!editor?.editors)return;
  for(const view of editor.editors.values()){
    if(!view?.dirty)continue;
    if(paths.some(pathValue=>pathTouchesEditor(pathValue,view.file?.path||''))){
      throw new Error('Save or close unsaved editor '+view.file.path+' before this file operation');
    }
  }
}
async function trashPaths(paths,{record=true,label='Move to Trash'}={}){
  const sources=topLevelSelectedPaths(paths);
  if(!sources.length)return [];
  assertNoDirtyEditors(sources);
  const restoreSteps=[];
  for(const source of sources){
    const result=await projectMutation('trash',source);
    restoreSteps.push({action:'restore',path:source,token:result.token});
    removeStoredPath(source);
    window.dispatchEvent(new CustomEvent('taskmenu:project-path-trashed',{detail:{path:source}}));
  }
  if(record)recordUndo(label,restoreSteps);
  return restoreSteps;
}
async function undoLastOperation(){
  const undo=lastUndo;
  if(!undo?.steps?.length)return;
  const dirtyPaths=undo.steps.filter(step=>step.action==='trash').map(step=>step.path);
  if(dirtyPaths.length)assertNoDirtyEditors(dirtyPaths);
  const restored=[];
  for(const step of [...undo.steps].reverse()){
    if(step.action==='trash'){
      await trashPaths([step.path],{record:false});
    }else if(step.action==='restore'){
      await projectMutation('restore',step.path,'',step.token||'');
      restored.push(step.path);
    }else if(step.action==='rename'){
      const result=await projectMutation('rename',step.path,step.new_path);
      const resolved=result.path||step.new_path;
      remapStoredPaths(step.path,resolved);
      window.dispatchEvent(new CustomEvent('taskmenu:project-path-renamed',{detail:{old_path:step.path,new_path:resolved}}));
      restored.push(resolved);
    }
  }
  clearUndo();
  await reload();
  selected.clear();for(const pathValue of restored)selected.add(pathValue);
  lastSelectedPath=restored.at(-1)||'';
  render();
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
  recordUndo('Create '+(result.path||pathValue),[{action:'trash',path:result.path||pathValue}]);
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
  recordUndo('Rename '+pathValue,[{action:'rename',path:resolved,new_path:pathValue}]);
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
  if(created.length)recordUndo('Duplicate selected',created.map(pathValue=>({action:'trash',path:pathValue})));
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
  if(fileClipboard.mode==='cut'){
    if(moved.length)recordUndo('Cut / Paste',moved.map(([oldPath,newPath])=>({action:'rename',path:newPath,new_path:oldPath})));
    fileClipboard={mode:'',paths:[]};
  }else if(created.length){
    recordUndo('Copy / Paste',created.map(pathValue=>({action:'trash',path:pathValue})));
  }
  await reload();
  selected.clear();
  for(const [,pathValue] of moved)selected.add(pathValue);
  for(const pathValue of created)selected.add(pathValue);
  lastSelectedPath=[...selected].at(-1)||'';
  render();
}
function canMovePathsToDirectory(paths,dir){
  return topLevelSelectedPaths(paths).every(source=>source!==dir&&!dir.startsWith(source+'/'));
}
async function movePathsToDirectory(paths,dir,label='Move selected'){
  const sources=topLevelSelectedPaths(paths);
  if(!sources.length)return [];
  if(!canMovePathsToDirectory(sources,dir))throw new Error('Cannot move a folder into itself or one of its descendants');
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
  if(moved.length)recordUndo(label,moved.map(([oldPath,newPath])=>({action:'rename',path:newPath,new_path:oldPath})));
  await reload();
  selected.clear();
  for(const [,nextPath] of moved)selected.add(nextPath);
  lastSelectedPath=moved.at(-1)?.[1]||'';
  render();
  return moved;
}
async function moveSelectedProjectItems(){
  const sources=topLevelSelectedPaths([...selected]);
  if(!sources.length)return;
  const raw=window.prompt('Move selected item(s) to project-relative folder. Use . for workspace root:',sources.length===1?parentPath(sources[0])||'.':'.');
  if(raw===null)return;
  const destination=String(raw||'').trim();
  const dir=destination===''||destination==='.'?'':destination.replace(/^\.\//,'').replace(/\/$/,'');
  await movePathsToDirectory(sources,dir,'Move selected');
}
function startExplorerDrag(event,pathValue,row){
  if(!selected.has(pathValue)){
    selected.clear();selected.add(pathValue);lastSelectedPath=pathValue;
    row.classList.add('selected');
  }
  dragPaths=topLevelSelectedPaths([...selected]);
  if(event.dataTransfer){
    event.dataTransfer.effectAllowed='move';
    try{event.dataTransfer.setData('text/plain',dragPaths.join('\n'));}catch{}
  }
}
function clearExplorerDrag(){
  dragPaths=[];
  tree.classList.remove('drag-over-root');
  for(const row of tree.querySelectorAll('.project-explorer-row.drag-over'))row.classList.remove('drag-over');
}
function explorerDropDirectory(pathValue,type){
  return type==='dir'?pathValue:parentPath(pathValue);
}
function dragOverExplorerRow(event,pathValue,type,row){
  if(!dragPaths.length)return;
  const dir=explorerDropDirectory(pathValue,type);
  if(!canMovePathsToDirectory(dragPaths,dir))return;
  event.preventDefault();event.stopPropagation();
  if(event.dataTransfer)event.dataTransfer.dropEffect='move';
  row.classList.add('drag-over');
}
async function dropExplorerRow(event,pathValue,type,row){
  if(!dragPaths.length)return;
  const sources=[...dragPaths];
  const dir=explorerDropDirectory(pathValue,type);
  event.preventDefault();event.stopPropagation();row.classList.remove('drag-over');
  clearExplorerDrag();
  await movePathsToDirectory(sources,dir,'Drag and drop move');
}
function dragOverExplorerRoot(event){
  if(!dragPaths.length||!canMovePathsToDirectory(dragPaths,''))return;
  event.preventDefault();
  if(event.dataTransfer)event.dataTransfer.dropEffect='move';
  tree.classList.add('drag-over-root');
}
async function dropExplorerRoot(event){
  if(!dragPaths.length)return;
  const sources=[...dragPaths];
  event.preventDefault();clearExplorerDrag();
  await movePathsToDirectory(sources,'','Drag and drop move');
}
function closeContextMenu(){contextMenu.classList.remove('visible');contextMenu.replaceChildren();}
function contextAction(label,run,options={}){
  const button=document.createElement('button');button.type='button';button.textContent=label;button.disabled=Boolean(options.disabled);if(options.danger)button.style.color='#ff9a9a';
  button.onclick=()=>{closeContextMenu();Promise.resolve(run()).catch(app.showError);};
  contextMenu.append(button);
}
function contextSeparator(){const sep=document.createElement('div');sep.className='project-explorer-context-separator';contextMenu.append(sep);}
function appendSharedProjectActions(pathValue,type){
  const actions=globalThis.TaskMenuProjectFileActions?.standardActions?.(pathValue,type)||[];
  for(const action of actions){
    if(action.separator)contextSeparator();
    else contextAction(action.label,action.run,action);
  }
}
function showContextMenu(event,pathValue,type){
  event.preventDefault();event.stopPropagation();
  if(!selected.has(pathValue)){selected.clear();selected.add(pathValue);lastSelectedPath=pathValue;render();}
  closeContextMenu();
  const paths=[...selected];
  if(paths.length===1){
    appendSharedProjectActions(pathValue,type);
    contextSeparator();
  }
  contextAction(paths.length>1?'Copy selected':'Copy',()=>setProjectClipboard('copy',paths));
  contextAction(paths.length>1?'Cut selected':'Cut',()=>setProjectClipboard('cut',paths));
  contextAction(paths.length>1?'Duplicate selected':'Duplicate',()=>duplicateSelectedProjectItems());
  if(fileClipboard.paths.length){
    const destination=destinationDirectoryForPath(pathValue,type);
    contextAction('Paste '+(fileClipboard.mode==='cut'?'move':'copy')+' here',()=>pasteProjectClipboard(destination));
  }
  if(paths.length===1){
    const terminalDir=type==='dir'?pathValue:parentPath(pathValue);
    contextAction('Open terminal here',()=>app.startTerminal(terminalDir));
    contextAction('Rename…',()=>renameProjectItem(pathValue));
  }
  contextAction(paths.length>1?'Move selected…':'Move…',()=>moveSelectedProjectItems());
  contextAction(paths.length>1?'Move selected to Trash':'Move to Trash',async()=>{
    await trashPaths(paths);
    await reload();
  });
  if(type==='dir'&&paths.length===1){
    contextAction('New file here…',()=>createProjectItem('file'));
    contextAction('New folder here…',()=>createProjectItem('dir'));
  }
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
    await loadWorkspaceRoots();
    for(const root of workspaceRoots){
      if(root.available===false)continue;
      await loadDirectory(rootBasePath(root),force);
    }
    rootLoaded=true;render();
    loadGitStatus().then(()=>render()).catch(()=>{});
    await restoreExpandedDirectories();
    render();
  }catch(error){rootLoaded=false;showMessage('Explorer unavailable');throw error;}
}
function open(){
  panel.classList.add('visible');restoreExpanded();restoreSaved();restoreUndoRecord();renderSaved();ensureRoot(false).catch(app.showError);
}
function close(){panel.classList.remove('visible');}
async function reload(){try{await ensureRoot(true);}catch(error){app.showError(error);}}

rootButton.onclick=()=>attachWorkspaceRoot().catch(app.showError);
newFileButton.onclick=()=>createProjectItem('file').catch(app.showError);
newFolderButton.onclick=()=>createProjectItem('dir').catch(app.showError);
undoButton.onclick=()=>undoLastOperation().catch(app.showError);
tree.ondragover=dragOverExplorerRoot;
tree.ondragleave=event=>{if(event.target===tree)tree.classList.remove('drag-over-root');};
tree.ondrop=event=>dropExplorerRoot(event).catch(app.showError);
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
window.addEventListener('taskmenu:git-status-refreshed',()=>{
  if(!panel.classList.contains('visible'))return;
  loadGitStatus().then(()=>render()).catch(()=>{});
});

globalThis.TaskMenuExplorer={
  open,close,reload,reveal:revealPath,openContainingFolder,undo:undoLastOperation,movePathsToDirectory,refreshGitStatus:async()=>{await loadGitStatus();render();},
  snapshotState(){return {expanded:[...expanded]};},
  async restoreState(state){
    expanded.clear();
    for(const pathValue of Array.isArray(state?.expanded)?state.expanded:[]){
      const value=String(pathValue||'').trim();
      if(value)expanded.add(value);
    }
    persistExpanded();
    await ensureRoot(false);
    await restoreExpandedDirectories();
    render();
    return true;
  },
  get selectedPaths(){return [...selected];},get favorites(){return [...favorites];},get recent(){return [...recent];},get clipboard(){return {mode:fileClipboard.mode,paths:[...fileClipboard.paths]};},get gitStatusAvailable(){return gitStatusAvailable;},get lastUndo(){return lastUndo?{label:lastUndo.label,steps:[...lastUndo.steps]}:null;}
};
