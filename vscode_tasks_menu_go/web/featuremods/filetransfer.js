const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for file transfer workspace');

const views=new Map();
let profilesByID=new Map();
let contextMenu=null;

const style=document.createElement('style');
style.textContent=`
.ft-tab{border-radius:6px 6px 0 0;border-bottom:0;margin-left:4px}
.ft-tab.active{background:#343b48}
.ft-tab .close{margin-left:8px}
.ft-pane{position:absolute;inset:0;display:flex;flex-direction:column;background:#0d1015}
.ft-pane.hidden{display:none}
.ft-pane-head{height:42px;display:flex;align-items:center;gap:8px;padding:6px 10px;border-bottom:1px solid #30343b}
.ft-pane-title{font-weight:600;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ft-pane-meta{font-size:10px;opacity:.55}
.ft-pane-spacer{flex:1}
.ft-toolbar{display:flex;align-items:center;gap:6px;padding:7px;border-bottom:1px solid #30343b;background:#11151b}
.ft-toolbar input[type=text]{flex:1;min-width:120px;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:6px 8px;font:11px ui-monospace,monospace}
.ft-toolbar button{font-size:11px}
.ft-status{padding:5px 9px;border-bottom:1px solid #272d36;font-size:10px;opacity:.65;min-height:16px}
.ft-table-wrap{flex:1;min-height:0;overflow:auto}
.ft-table{width:100%;border-collapse:collapse;font-size:11px}
.ft-table th,.ft-table td{padding:6px 8px;border-bottom:1px solid #272d36;text-align:left;white-space:nowrap}
.ft-table th{position:sticky;top:0;background:#171c23;z-index:2;font-size:10px;opacity:.8}
.ft-table tbody tr{cursor:default}
.ft-table tbody tr:hover{background:#202731}
.ft-name{max-width:620px;overflow:hidden;text-overflow:ellipsis}
.ft-kind{display:inline-block;min-width:16px;margin-right:5px;opacity:.75}
.ft-size{text-align:right!important;font-family:ui-monospace,monospace}
.ft-actions{display:flex;gap:4px;justify-content:flex-end}
.ft-actions button{font-size:10px;padding:2px 6px}
.ft-empty{padding:20px;opacity:.55}
.ft-context{position:fixed;z-index:16000;min-width:150px;padding:4px;background:#171b22;border:1px solid #48515f;border-radius:7px;box-shadow:0 14px 38px rgba(0,0,0,.45)}
.ft-context button{display:block;width:100%;text-align:left;border:0;background:transparent;color:inherit;padding:7px 9px;border-radius:4px}
.ft-context button:hover{background:#2b3440}.ft-context button.danger{color:#ff9a9a}
html[data-taskmenu-theme="light"] .ft-pane{background:#fff}
html[data-taskmenu-theme="light"] .ft-toolbar{background:#f2f5f8;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-toolbar input[type=text]{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-table th{background:#e9eef3}
html[data-taskmenu-theme="light"] .ft-table tbody tr:hover{background:#eef2f6}
html[data-taskmenu-theme="light"] .ft-context{background:#fff;border-color:#b9c0c8}
`;
document.head.append(style);

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

function joinRemotePath(parent,name){
  parent=normalizeRemotePath(parent);
  name=String(name||'').replace(/^\/+/, '');
  if(parent==='.')return name||'.';
  if(parent==='/')return '/'+name;
  return parent.replace(/\/$/,'')+'/'+name;
}

function parentRemotePath(value){
  value=normalizeRemotePath(value);
  if(value==='.'||value==='/')return value;
  const absolute=value.startsWith('/');
  const parts=value.split('/').filter(Boolean);
  parts.pop();
  if(!parts.length)return absolute?'/':'.';
  return (absolute?'/':'')+parts.join('/');
}

function formatSize(value){
  let n=Number(value)||0;
  if(n<1024)return n+' B';
  const units=['KiB','MiB','GiB','TiB'];
  let i=-1;
  do{n/=1024;i++;}while(n>=1024&&i<units.length-1);
  return n.toFixed(n>=10?1:2)+' '+units[i];
}

async function refreshProfiles(){
  const data=await app.jsonFetch('/api/file-transfer/profiles');
  profilesByID=new Map((Array.isArray(data?.profiles)?data.profiles:[]).map(profile=>[String(profile.id||''),profile]));
  return profilesByID;
}

function profileFor(view){
  return profilesByID.get(view.profile.id)||view.profile;
}

function activateView(id){
  const view=views.get(id);if(!view)return;
  app.activateExternalView('file-transfer:'+id);
  for(const [otherID,other] of views){
    const active=otherID===id;
    other.tab.classList.toggle('active',active);
    other.pane.classList.toggle('hidden',!active);
  }
}

function closeView(id){
  const view=views.get(id);if(!view)return;
  view.tab.remove();view.pane.remove();views.delete(id);
}

function closeContextMenu(){
  contextMenu?.remove();contextMenu=null;
}

function showContextMenu(items,x,y){
  closeContextMenu();
  const menu=document.createElement('div');menu.className='ft-context';
  for(const item of items){
    const button=document.createElement('button');button.type='button';button.textContent=item.label;
    if(item.danger)button.classList.add('danger');
    button.onclick=()=>{closeContextMenu();Promise.resolve(item.action()).catch(app.showError);};
    menu.append(button);
  }
  document.body.append(menu);contextMenu=menu;
  const rect=menu.getBoundingClientRect();
  menu.style.left=Math.max(4,Math.min(x,window.innerWidth-rect.width-4))+'px';
  menu.style.top=Math.max(4,Math.min(y,window.innerHeight-rect.height-4))+'px';
}
document.addEventListener('pointerdown',event=>{if(contextMenu&&!contextMenu.contains(event.target))closeContextMenu();},true);
window.addEventListener('blur',closeContextMenu);

function downloadFrame(){
  let frame=document.querySelector('iframe[name="taskdeck-file-transfer-download"]');
  if(frame)return frame;
  frame=document.createElement('iframe');frame.name='taskdeck-file-transfer-download';frame.hidden=true;document.body.append(frame);return frame;
}

function downloadFile(view,remotePath){
  downloadFrame();
  const form=document.createElement('form');
  form.method='POST';form.action='/api/file-transfer/download';form.target='taskdeck-file-transfer-download';form.hidden=true;
  const add=(name,value)=>{const input=document.createElement('input');input.type='hidden';input.name=name;input.value=value;form.append(input);};
  add('profile_id',view.profile.id);add('path',remotePath);
  document.body.append(form);form.submit();setTimeout(()=>form.remove(),1000);
}

async function mutate(view,action,path,newPath='',directory=false){
  await app.jsonFetch('/api/file-transfer/mutate',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({profile_id:view.profile.id,action,path,new_path:newPath,directory})
  });
  await loadDirectory(view,view.pathInput.value);
}

async function renameEntry(view,entry){
  const oldPath=joinRemotePath(view.currentPath,entry.name);
  const nextName=prompt('Rename to:',entry.name);
  if(nextName===null)return;
  const name=String(nextName).trim();
  if(!name||name===entry.name)return;
  if(name.includes('/')||name.includes('\\'))throw new Error('New name must not contain path separators');
  await mutate(view,'rename',oldPath,joinRemotePath(view.currentPath,name),entry.type==='directory');
}

async function deleteEntry(view,entry){
  if(!confirm('Delete '+entry.name+'?'+(entry.type==='directory'?'\n\nDirectory removal is non-recursive.':'')))return;
  await mutate(view,'delete',joinRemotePath(view.currentPath,entry.name),'',entry.type==='directory');
}

function renderEntries(view,entries){
  view.tbody.replaceChildren();
  if(!entries.length){
    const tr=document.createElement('tr'),td=document.createElement('td');td.colSpan=4;td.className='ft-empty';td.textContent='Directory is empty';tr.append(td);view.tbody.append(tr);return;
  }
  for(const entry of entries){
    const tr=document.createElement('tr');
    const name=document.createElement('td');name.className='ft-name';
    const icon=document.createElement('span');icon.className='ft-kind';icon.textContent=entry.type==='directory'?'📁':entry.type==='symlink'?'↗':'📄';
    const label=document.createElement('span');label.textContent=entry.name;name.append(icon,label);
    const size=document.createElement('td');size.className='ft-size';size.textContent=entry.type==='directory'?'':formatSize(entry.size);
    const modified=document.createElement('td');modified.textContent=entry.modified||'';
    const actions=document.createElement('td');actions.className='ft-actions';
    if(entry.type!=='directory'){
      const download=document.createElement('button');download.type='button';download.textContent='Download';
      download.onclick=event=>{event.stopPropagation();downloadFile(view,joinRemotePath(view.currentPath,entry.name));};actions.append(download);
    }
    const rename=document.createElement('button');rename.type='button';rename.textContent='Rename';rename.onclick=event=>{event.stopPropagation();renameEntry(view,entry).catch(app.showError);};
    const del=document.createElement('button');del.type='button';del.textContent='Delete';del.onclick=event=>{event.stopPropagation();deleteEntry(view,entry).catch(app.showError);};
    actions.append(rename,del);tr.append(name,size,modified,actions);
    tr.ondblclick=()=>{
      if(entry.type==='directory')loadDirectory(view,joinRemotePath(view.currentPath,entry.name)).catch(app.showError);
      else downloadFile(view,joinRemotePath(view.currentPath,entry.name));
    };
    tr.oncontextmenu=event=>{
      event.preventDefault();
      const items=[];
      if(entry.type!=='directory')items.push({label:'Download',action:()=>downloadFile(view,joinRemotePath(view.currentPath,entry.name))});
      items.push({label:'Rename',action:()=>renameEntry(view,entry)});
      items.push({label:'Delete',danger:true,action:()=>deleteEntry(view,entry)});
      showContextMenu(items,event.clientX,event.clientY);
    };
    view.tbody.append(tr);
  }
}

async function loadDirectory(view,path){
  path=normalizeRemotePath(path||profileFor(view)?.initial_path||'.');
  view.status.textContent='Loading '+path+'…';
  view.refresh.disabled=true;
  try{
    const data=await app.jsonFetch('/api/file-transfer/list',{
      method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({profile_id:view.profile.id,path})
    });
    view.currentPath=String(data?.path||path);
    view.pathInput.value=view.currentPath;
    renderEntries(view,Array.isArray(data?.entries)?data.entries:[]);
    view.status.textContent=(data?.protocol||view.profile.protocol).toUpperCase()+' · '+(data?.entries?.length||0)+' item(s)';
  }catch(error){
    view.status.textContent=String(error?.message||error);
    throw error;
  }finally{view.refresh.disabled=false;}
}

async function uploadFile(view,file){
  if(!file)return;
  const target=joinRemotePath(view.currentPath,file.name);
  const form=new FormData();form.append('profile_id',view.profile.id);form.append('path',target);form.append('file',file,file.name);
  view.status.textContent='Uploading '+file.name+'…';
  const response=await app.fetchWithLease('/api/file-transfer/upload',{method:'POST',body:form,cache:'no-store'});
  if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
  await loadDirectory(view,view.currentPath);
}

function attachView(profile){
  const id=String(profile.id||'');
  if(views.has(id)){activateView(id);return views.get(id);}
  const tabs=document.querySelector('#tabs'),panes=document.querySelector('#panes');
  if(!tabs||!panes)throw new Error('Workspace tabs are unavailable');

  const tab=document.createElement('button');tab.type='button';tab.className='ft-tab';tab.dataset.id='file-transfer:'+id;
  const label=document.createElement('span');label.textContent=String(profile.protocol||'file').toUpperCase()+' · '+(profile.name||'Files');
  const close=document.createElement('span');close.className='close';close.textContent='×';close.onclick=event=>{event.stopPropagation();closeView(id);};
  tab.append(label,close);tab.onclick=()=>activateView(id);tabs.append(tab);

  const pane=document.createElement('div');pane.className='ft-pane hidden';pane.dataset.id='file-transfer:'+id;
  const head=document.createElement('div');head.className='ft-pane-head';
  const title=document.createElement('div');title.className='ft-pane-title';title.textContent=profile.name||'Remote files';
  const meta=document.createElement('div');meta.className='ft-pane-meta';meta.textContent=String(profile.protocol||'').toUpperCase();
  const spacer=document.createElement('div');spacer.className='ft-pane-spacer';
  const disconnect=document.createElement('button');disconnect.type='button';disconnect.textContent='Close';disconnect.onclick=()=>closeView(id);
  head.append(title,meta,spacer,disconnect);

  const toolbar=document.createElement('div');toolbar.className='ft-toolbar';
  const up=document.createElement('button');up.type='button';up.textContent='↑ Up';
  const pathInput=document.createElement('input');pathInput.type='text';pathInput.spellcheck=false;pathInput.value=profile.initial_path||'.';
  const go=document.createElement('button');go.type='button';go.textContent='Go';
  const refresh=document.createElement('button');refresh.type='button';refresh.textContent='↻';refresh.title='Refresh';
  const upload=document.createElement('button');upload.type='button';upload.textContent='Upload';
  const mkdir=document.createElement('button');mkdir.type='button';mkdir.textContent='New Folder';
  const fileInput=document.createElement('input');fileInput.type='file';fileInput.hidden=true;
  toolbar.append(up,pathInput,go,refresh,upload,mkdir,fileInput);

  const status=document.createElement('div');status.className='ft-status';
  const wrap=document.createElement('div');wrap.className='ft-table-wrap';
  const table=document.createElement('table');table.className='ft-table';
  const thead=document.createElement('thead');const hr=document.createElement('tr');
  for(const text of ['Name','Size','Modified','']){const th=document.createElement('th');th.textContent=text;hr.append(th);}
  thead.append(hr);const tbody=document.createElement('tbody');table.append(thead,tbody);wrap.append(table);
  pane.append(head,toolbar,status,wrap);panes.append(pane);

  const view={profile,tab,pane,pathInput,refresh,status,tbody,currentPath:profile.initial_path||'.'};
  views.set(id,view);
  up.onclick=()=>loadDirectory(view,parentRemotePath(view.currentPath)).catch(app.showError);
  go.onclick=()=>loadDirectory(view,pathInput.value).catch(app.showError);
  pathInput.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();go.click();}});
  refresh.onclick=()=>loadDirectory(view,view.currentPath).catch(app.showError);
  upload.onclick=()=>fileInput.click();
  fileInput.onchange=()=>{const file=fileInput.files?.[0];fileInput.value='';uploadFile(view,file).catch(app.showError);};
  mkdir.onclick=()=>{
    const name=prompt('New folder name:','');
    if(name===null)return;
    const clean=String(name).trim();
    if(!clean)return;
    if(clean.includes('/')||clean.includes('\\')){app.showError(new Error('Folder name must not contain path separators'));return;}
    mutate(view,'mkdir',joinRemotePath(view.currentPath,clean)).catch(app.showError);
  };
  activateView(id);
  loadDirectory(view,profile.initial_path||'.').catch(app.showError);
  return view;
}

async function openProfile(profileOrID){
  const id=typeof profileOrID==='string'?profileOrID:profileOrID?.id;
  if(!id)throw new Error('File-transfer profile is required');
  await refreshProfiles();
  const profile=profilesByID.get(String(id))||(typeof profileOrID==='object'?profileOrID:null);
  if(!profile)throw new Error('File-transfer profile not found');
  return attachView(profile);
}

async function testProfile(profileOrID){
  const id=typeof profileOrID==='string'?profileOrID:profileOrID?.id;
  if(!id)throw new Error('File-transfer profile is required');
  return app.jsonFetch('/api/file-transfer/test',{
    method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:id})
  });
}

async function testDraft(profileID,profile){
  const body={profile};if(profileID)body.profile_id=profileID;
  return app.jsonFetch('/api/file-transfer/test',{
    method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)
  });
}

window.addEventListener('taskmenu:view-activated',event=>{
  const detail=event.detail||{};
  const external=detail.kind==='external'&&String(detail.id||'').startsWith('file-transfer:');
  const activeID=external?String(detail.id).slice('file-transfer:'.length):'';
  for(const [id,view] of views){
    const active=id===activeID;
    view.tab.classList.toggle('active',active);
    view.pane.classList.toggle('hidden',!active);
  }
});

globalThis.TaskMenuFileTransfer={openProfile,testProfile,testDraft,refreshProfiles,getProfile:id=>profilesByID.get(String(id||''))||null};
