const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for upload');

const style=document.createElement('style');
style.textContent=`
#upload-workspace{white-space:nowrap}
.upload-drop-overlay{position:fixed;inset:0;z-index:9999;display:none;align-items:center;justify-content:center;background:rgba(5,6,7,.82);font-size:24px;font-weight:600;border:3px dashed #5a88b4;pointer-events:none}
.upload-drop-overlay.visible{display:flex}
.upload-destination-overlay{position:fixed;inset:0;z-index:4200;display:none;align-items:center;justify-content:center;background:rgba(0,0,0,.58);padding:18px}
.upload-destination-overlay.visible{display:flex}
.upload-destination-dialog{width:min(680px,96vw);max-height:min(760px,92vh);display:flex;flex-direction:column;background:#171a20;border:1px solid #48515f;border-radius:10px;box-shadow:0 18px 55px rgba(0,0,0,.5);overflow:hidden}
.upload-destination-head{display:flex;align-items:center;gap:8px;padding:12px 14px;border-bottom:1px solid #30343b}.upload-destination-head strong{flex:1}.upload-destination-head button{padding:4px 8px}
.upload-destination-tree{min-height:220px;max-height:52vh;overflow:auto;padding:7px 6px;font:12px ui-monospace,monospace}
.upload-directory-row{display:flex;align-items:center;gap:3px;min-height:27px;border-radius:4px}.upload-directory-row.selected{background:#293241}.upload-directory-row:hover{background:#232a34}
.upload-directory-toggle{border:0;background:transparent;padding:2px 4px;min-width:24px}.upload-directory-name{border:0;background:transparent;text-align:left;flex:1;min-width:0;padding:4px 3px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.upload-directory-children{margin-left:14px;border-left:1px solid #252c35;padding-left:5px}.upload-directory-message{padding:9px;opacity:.65}
.upload-destination-path{border-top:1px solid #30343b;padding:11px 12px}.upload-destination-path label{display:block;font-size:11px;font-weight:700;opacity:.7;margin-bottom:5px}.upload-destination-path input{width:100%;box-sizing:border-box;padding:7px 9px;font:12px ui-monospace,monospace}
.upload-destination-actions{display:flex;justify-content:flex-end;gap:8px;padding:0 12px 12px}.upload-destination-confirm{background:#24472f;border-color:#3b7850}
html[data-taskmenu-theme="light"] .upload-destination-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .upload-directory-row:hover{background:#edf1f5}html[data-taskmenu-theme="light"] .upload-directory-row.selected{background:#dde8f3}html[data-taskmenu-theme="light"] .upload-directory-children{border-color:#dfe3e8}
`;
document.head.append(style);

const dirKey=()=>`vscode-tasks-menu:upload-dir:${app.taskData.workspace}`;
function lastDir(){try{return localStorage.getItem(dirKey())||'.';}catch{return '.';}}
function saveDir(value){try{localStorage.setItem(dirKey(),value);}catch{}}
function joinDir(parent,name){return parent&&parent!=='.'?parent+'/'+name:name;}
function displayDir(path){return !path||path==='.'?'. (workspace root)':path;}

let input=null,button=null,overlay=null,dragDepth=0;
let destinationOverlay=null,destinationTree=null,destinationInput=null,destinationConfirm=null,destinationTitle=null,destinationLabel=null;
let destinationResolve=null,destinationSelected='.';
const destinationLoaded=new Map();
const destinationExpanded=new Set();

function installUploadUI(){
  if(button?.isConnected)return;
  const header=document.querySelector('header');if(!header)return;
  input=document.createElement('input');input.type='file';input.multiple=true;input.hidden=true;input.onchange=()=>{const files=[...(input.files||[])];input.value='';if(files.length)chooseDestinationAndUpload(files);};document.body.append(input);
  button=document.createElement('button');button.id='upload-workspace';button.textContent='Upload';button.title='Upload files into the workspace';button.onclick=()=>input.click();
  const reload=document.querySelector('#reload');if(reload)header.insertBefore(button,reload);else header.append(button);
  overlay=document.createElement('div');overlay.className='upload-drop-overlay';overlay.textContent='Drop files to upload into workspace';document.body.append(overlay);
  installDestinationBrowser();
}

function installDestinationBrowser(){
  if(destinationOverlay?.isConnected)return;
  destinationOverlay=document.createElement('div');destinationOverlay.className='upload-destination-overlay';
  const dialog=document.createElement('div');dialog.className='upload-destination-dialog';
  const head=document.createElement('div');head.className='upload-destination-head';
  destinationTitle=document.createElement('strong');destinationTitle.textContent='Choose upload destination';
  const refresh=document.createElement('button');refresh.type='button';refresh.textContent='↻';refresh.title='Refresh directories';
  const close=document.createElement('button');close.type='button';close.textContent='×';close.title='Cancel';
  destinationTree=document.createElement('div');destinationTree.className='upload-destination-tree';
  const pathBox=document.createElement('div');pathBox.className='upload-destination-path';
  destinationLabel=document.createElement('label');destinationLabel.textContent='Destination directory relative to the workspace:';
  destinationInput=document.createElement('input');destinationInput.type='text';destinationInput.autocomplete='off';destinationInput.spellcheck=false;
  pathBox.append(destinationLabel,destinationInput);
  const actions=document.createElement('div');actions.className='upload-destination-actions';
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
  destinationConfirm=document.createElement('button');destinationConfirm.type='button';destinationConfirm.className='upload-destination-confirm';destinationConfirm.textContent='Upload here';
  actions.append(cancel,destinationConfirm);head.append(destinationTitle,refresh,close);dialog.append(head,destinationTree,pathBox,actions);destinationOverlay.append(dialog);document.body.append(destinationOverlay);

  close.onclick=cancel.onclick=()=>closeDestinationBrowser(null);
  destinationConfirm.onclick=()=>closeDestinationBrowser((destinationInput.value.trim()||'.'));
  destinationInput.onkeydown=event=>{
    if(event.key==='Enter'){event.preventDefault();destinationConfirm.click();}
    else if(event.key==='Escape'){event.preventDefault();closeDestinationBrowser(null);}
  };
  refresh.onclick=()=>refreshDestinationTree();
  destinationOverlay.addEventListener('pointerdown',event=>{if(event.target===destinationOverlay)closeDestinationBrowser(null);});
}

async function loadDestinationDirectory(pathValue,force=false){
  const key=pathValue==='.'?'':pathValue;
  if(!force&&destinationLoaded.has(key))return destinationLoaded.get(key);
  const items=await app.jsonFetch('/api/project/tree?path='+encodeURIComponent(key));
  const dirs=(Array.isArray(items)?items:[]).filter(item=>item?.type==='dir');
  destinationLoaded.set(key,dirs);
  return dirs;
}

function selectDestination(pathValue){
  destinationSelected=pathValue||'.';
  destinationInput.value=destinationSelected;
  renderDestinationTree();
}

function renderDestinationTree(){
  destinationTree.replaceChildren();
  const root=document.createElement('div');
  root.className='upload-directory-row'+(destinationSelected==='.'?' selected':'');
  const rootToggle=document.createElement('button');rootToggle.type='button';rootToggle.className='upload-directory-toggle';rootToggle.textContent=destinationExpanded.has('')?'▾':'▸';
  const rootName=document.createElement('button');rootName.type='button';rootName.className='upload-directory-name';rootName.textContent='. (workspace root)';rootName.title='Workspace root';
  rootToggle.onclick=()=>toggleDestinationDirectory('');
  rootName.onclick=()=>{selectDestination('.');if(!destinationExpanded.has(''))toggleDestinationDirectory('');};
  root.append(rootToggle,rootName);
  const wrapper=document.createElement('div');wrapper.append(root);
  if(destinationExpanded.has('')){
    const children=document.createElement('div');children.className='upload-directory-children';
    const list=destinationLoaded.get('');
    if(!list){const msg=document.createElement('div');msg.className='upload-directory-message';msg.textContent='Loading…';children.append(msg);}
    else if(!list.length){const msg=document.createElement('div');msg.className='upload-directory-message';msg.textContent='No subdirectories';children.append(msg);}
    else for(const item of list)children.append(renderDestinationDirectoryItem('',item));
    wrapper.append(children);
  }
  destinationTree.append(wrapper);
}

function renderDestinationDirectoryItem(parent,item){
  const fullPath=joinDir(parent,item.name);
  const wrap=document.createElement('div');
  const row=document.createElement('div');row.className='upload-directory-row'+(destinationSelected===fullPath?' selected':'');
  const toggle=document.createElement('button');toggle.type='button';toggle.className='upload-directory-toggle';toggle.textContent=destinationExpanded.has(fullPath)?'▾':'▸';
  const name=document.createElement('button');name.type='button';name.className='upload-directory-name';name.textContent='📁 '+item.name;name.title=fullPath;
  toggle.onclick=()=>toggleDestinationDirectory(fullPath);
  name.onclick=()=>{selectDestination(fullPath);if(!destinationExpanded.has(fullPath))toggleDestinationDirectory(fullPath);};
  row.append(toggle,name);wrap.append(row);
  if(destinationExpanded.has(fullPath)){
    const children=document.createElement('div');children.className='upload-directory-children';
    const list=destinationLoaded.get(fullPath);
    if(!list){const msg=document.createElement('div');msg.className='upload-directory-message';msg.textContent='Loading…';children.append(msg);}
    else if(!list.length){const msg=document.createElement('div');msg.className='upload-directory-message';msg.textContent='Empty';children.append(msg);}
    else for(const child of list)children.append(renderDestinationDirectoryItem(fullPath,child));
    wrap.append(children);
  }
  return wrap;
}

async function toggleDestinationDirectory(pathValue){
  if(destinationExpanded.has(pathValue)){
    destinationExpanded.delete(pathValue);renderDestinationTree();return;
  }
  destinationExpanded.add(pathValue);renderDestinationTree();
  if(!destinationLoaded.has(pathValue)){
    try{await loadDestinationDirectory(pathValue);if(destinationExpanded.has(pathValue))renderDestinationTree();}
    catch(error){destinationExpanded.delete(pathValue);renderDestinationTree();app.showError(error);}
  }
}

async function refreshDestinationTree(){
  destinationLoaded.clear();
  destinationExpanded.add('');
  renderDestinationTree();
  try{await loadDestinationDirectory('',true);renderDestinationTree();}
  catch(error){app.showError(error);}
}

function closeDestinationBrowser(value){
  if(!destinationResolve)return;
  const resolve=destinationResolve;destinationResolve=null;
  destinationOverlay.classList.remove('visible');
  resolve(value);
}

async function chooseWorkspaceDirectory(options={}){
  installDestinationBrowser();
  if(destinationResolve)throw new Error('Directory browser is already open');
  const initial=String(options.initial||'.').trim()||'.';
  destinationTitle.textContent=String(options.title||'Choose directory');
  destinationLabel.textContent=String(options.label||'Directory relative to the workspace:');
  destinationConfirm.textContent=String(options.confirm||'Use directory');
  destinationSelected=initial;
  destinationInput.value=destinationSelected;
  destinationExpanded.add('');
  destinationOverlay.classList.add('visible');
  renderDestinationTree();
  loadDestinationDirectory('').then(()=>renderDestinationTree()).catch(app.showError);
  setTimeout(()=>destinationInput.focus(),0);
  return new Promise(resolve=>{destinationResolve=resolve;});
}

async function chooseDestination(){
  return chooseWorkspaceDirectory({
    title:'Choose upload destination',
    label:'Destination directory relative to the workspace:',
    confirm:'Upload here',
    initial:lastDir()
  });
}

globalThis.TaskMenuDirectoryBrowser={choose:chooseWorkspaceDirectory};

async function chooseDestinationAndUpload(files){
  const value=await chooseDestination();if(value===null)return;
  const dir=(value.trim()||'.');saveDir(dir);
  button.disabled=true;const old=button.textContent;button.textContent=`Upload 0/${files.length}`;
  try{
    for(let i=0;i<files.length;i++){
      button.textContent=`Upload ${i+1}/${files.length}`;
      await uploadOne(files[i],dir,false);
    }
    button.textContent='Uploaded';setTimeout(()=>{if(button?.isConnected)button.textContent=old;},1200);
  }finally{button.disabled=false;if(button.textContent.startsWith('Upload '))button.textContent=old;}
}

async function uploadOne(file,dir,overwrite){
  const form=new FormData();form.append('dir',dir);form.append('file',file,file.name);
  const url='/api/files/upload'+(overwrite?'?overwrite=1':'');
  const response=await app.fetchWithLease(url,{method:'POST',body:form,cache:'no-store'});
  if(response.status===409&&!overwrite){
    const message=(await response.text()).trim();
    if(message.includes('already exists')&&window.confirm(`${file.name} already exists. Overwrite it?`))return uploadOne(file,dir,true);
    throw new Error(message||'Upload conflict');
  }
  if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
  return response.json();
}

function hasFiles(event){return [...(event.dataTransfer?.types||[])].includes('Files');}
document.addEventListener('dragenter',event=>{if(!hasFiles(event))return;event.preventDefault();dragDepth++;overlay?.classList.add('visible');});
document.addEventListener('dragover',event=>{if(!hasFiles(event))return;event.preventDefault();if(event.dataTransfer)event.dataTransfer.dropEffect='copy';});
document.addEventListener('dragleave',event=>{if(!hasFiles(event))return;dragDepth=Math.max(0,dragDepth-1);if(!dragDepth)overlay?.classList.remove('visible');});
document.addEventListener('drop',event=>{
  if(!hasFiles(event))return;event.preventDefault();dragDepth=0;overlay?.classList.remove('visible');const files=[...(event.dataTransfer?.files||[])].filter(file=>file.size>=0);if(files.length)chooseDestinationAndUpload(files).catch(app.showError);
});

installUploadUI();
