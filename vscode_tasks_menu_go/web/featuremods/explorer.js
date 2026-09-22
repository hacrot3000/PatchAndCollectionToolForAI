const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Explorer');

const style=document.createElement('style');
style.textContent=`
.project-explorer{display:none;position:fixed;top:52px;bottom:0;left:0;z-index:1800;width:min(370px,92vw);background:#11151b;border-right:1px solid #3b414d;box-shadow:10px 0 28px rgba(0,0,0,.28);flex-direction:column}
.project-explorer.visible{display:flex}
.project-explorer-head{height:42px;display:flex;align-items:center;gap:6px;padding:6px 8px;border-bottom:1px solid #30343b}
.project-explorer-title{font-size:12px;font-weight:700;letter-spacing:.04em;flex:1}
.project-explorer-head button{padding:4px 7px;font-size:11px}
.project-explorer-tree{flex:1;overflow:auto;padding:5px 4px 12px;font:12px ui-monospace,monospace}
.project-explorer-row{display:flex;align-items:center;min-width:0;height:26px;border-radius:4px;padding-right:4px}
.project-explorer-row:hover{background:#232a34}
.project-explorer-toggle{border:0;background:transparent;padding:2px 4px;min-width:22px}
.project-explorer-name{border:0;background:transparent;text-align:left;flex:1;min-width:0;padding:3px 2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.project-explorer-name.file{opacity:.9}
.project-explorer-children{margin-left:13px;border-left:1px solid #252c35;padding-left:4px}
.project-explorer-message{padding:10px 8px;opacity:.65}
html[data-taskmenu-theme="light"] .project-explorer{background:#fff;border-color:#b9c0c8;box-shadow:10px 0 28px rgba(0,0,0,.12)}
html[data-taskmenu-theme="light"] .project-explorer-row:hover{background:#edf1f5}
html[data-taskmenu-theme="light"] .project-explorer-children{border-color:#dfe3e8}
`;
document.head.append(style);

const panel=document.createElement('div');panel.className='project-explorer';
const head=document.createElement('div');head.className='project-explorer-head';
const title=document.createElement('div');title.className='project-explorer-title';title.textContent='EXPLORER';
const refresh=document.createElement('button');refresh.type='button';refresh.textContent='↻';refresh.title='Refresh explorer';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close explorer';
const tree=document.createElement('div');tree.className='project-explorer-tree';
head.append(title,refresh,closeButton);panel.append(head,tree);document.body.append(panel);

const loaded=new Map();
const expanded=new Set();
let rootLoaded=false;
let requestSeq=0;

function storageKey(){return 'vscode-tasks-menu:explorer-expanded:'+(app.taskData?.workspace||'workspace');}
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
function showMessage(message){tree.replaceChildren();const node=document.createElement('div');node.className='project-explorer-message';node.textContent=message;tree.append(node);}

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
  tree.replaceChildren();
  const rootItems=loaded.get('')||[];
  if(!rootLoaded){showMessage('Loading…');return;}
  if(!rootItems.length){showMessage('Workspace has no files');return;}
  for(const item of rootItems)tree.append(renderItem('',item));
}
function renderItem(parent,item){
  const fullPath=joinPath(parent,item.name);
  const wrap=document.createElement('div');
  const row=document.createElement('div');row.className='project-explorer-row';
  const toggle=document.createElement('button');toggle.type='button';toggle.className='project-explorer-toggle';
  const name=document.createElement('button');name.type='button';name.className='project-explorer-name '+item.type;name.textContent=item.name;name.title=fullPath;
  row.style.paddingLeft='0px';
  if(item.type==='dir'){
    toggle.textContent=expanded.has(fullPath)?'▾':'▸';toggle.title='Expand '+fullPath;
    const expand=()=>toggleDirectory(fullPath);
    toggle.onclick=expand;name.onclick=expand;
  }else{
    toggle.textContent='';toggle.disabled=true;
    name.onclick=()=>openFile(fullPath);
    name.ondblclick=()=>openFile(fullPath);
  }
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
  panel.classList.add('visible');restoreExpanded();ensureRoot(false).catch(app.showError);
}
function close(){panel.classList.remove('visible');}
async function reload(){try{await ensureRoot(true);}catch(error){app.showError(error);}}

refresh.onclick=reload;
closeButton.onclick=close;
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&panel.classList.contains('visible'))close();});

globalThis.TaskMenuExplorer={open,close,reload};
