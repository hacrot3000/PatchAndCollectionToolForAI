const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Remote Workspace');

const style=document.createElement('style');
style.textContent=`
.remote-workspace-backdrop{display:none;position:fixed;inset:0;z-index:17620;background:rgba(0,0,0,.48);align-items:flex-start;justify-content:center;padding:4vh 14px}
.remote-workspace-backdrop.visible{display:flex}
.remote-workspace-dialog{width:min(1180px,97vw);max-height:92vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 60px rgba(0,0,0,.58);overflow:hidden}
.remote-workspace-head{display:flex;align-items:center;gap:6px;padding:8px 10px;border-bottom:1px solid #303843}.remote-workspace-head strong{flex:1}
.remote-workspace-toolbar{display:flex;gap:6px;align-items:center;flex-wrap:wrap;padding:7px 10px;border-bottom:1px solid #303843}.remote-workspace-toolbar select{min-width:190px}.remote-workspace-path{display:flex;gap:6px;align-items:center;padding:7px 10px;border-bottom:1px solid #303843}.remote-workspace-path input{flex:1;min-width:120px}
.remote-workspace-body{display:grid;grid-template-columns:minmax(360px,1fr) minmax(300px,.7fr);min-height:320px;max-height:66vh}
.remote-workspace-files{overflow:auto;border-right:1px solid #303843}.remote-workspace-side{overflow:auto;padding:9px}
.remote-workspace-table{width:100%;border-collapse:collapse;font-size:11px}.remote-workspace-table th,.remote-workspace-table td{padding:6px 8px;border-bottom:1px solid #252c35;text-align:left}.remote-workspace-table tr:hover{background:#28313e}.remote-workspace-table td:first-child{cursor:pointer}.remote-workspace-kind{font-size:9px;opacity:.65;text-transform:uppercase}
.remote-workspace-section{border:1px solid #303843;border-radius:7px;padding:8px;margin-bottom:8px}.remote-workspace-section strong{display:block;margin-bottom:6px}.remote-workspace-actions{display:flex;gap:5px;flex-wrap:wrap}.remote-workspace-output{white-space:pre-wrap;overflow:auto;max-height:190px;font:10px/1.4 ui-monospace,monospace;background:#0b0f14;border:1px solid #303843;border-radius:5px;padding:7px;margin-top:6px}.remote-workspace-meta{font-size:10px;opacity:.68;overflow-wrap:anywhere}.remote-workspace-empty{padding:26px;text-align:center;opacity:.6}
.remote-workspace-form{display:grid;grid-template-columns:150px minmax(220px,1fr);gap:7px 9px;padding:10px}.remote-workspace-form input,.remote-workspace-form select{min-width:0}.remote-workspace-form .wide{grid-column:1/-1}.remote-workspace-form-actions{display:flex;justify-content:flex-end;gap:6px;padding:8px 10px;border-top:1px solid #303843}
html[data-taskmenu-theme="light"] .remote-workspace-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .remote-workspace-table th,html[data-taskmenu-theme="light"] .remote-workspace-table td{border-color:#d0d7de}html[data-taskmenu-theme="light"] .remote-workspace-output{background:#f6f8fa;border-color:#d0d7de}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='remote-workspace-backdrop';
const dialog=document.createElement('div');dialog.className='remote-workspace-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Remote Workspace');
const head=document.createElement('div');head.className='remote-workspace-head';
const title=document.createElement('strong');title.textContent='REMOTE WORKSPACE';
const status=document.createElement('span');status.style.fontSize='10px';status.style.opacity='.65';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';
head.append(title,status,closeButton);

const toolbar=document.createElement('div');toolbar.className='remote-workspace-toolbar';
const workspaceSelect=document.createElement('select');
const newButton=document.createElement('button');newButton.type='button';newButton.textContent='New';
const editButton=document.createElement('button');editButton.type='button';editButton.textContent='Edit';
const deleteButton=document.createElement('button');deleteButton.type='button';deleteButton.textContent='Delete';
const refreshButton=document.createElement('button');refreshButton.type='button';refreshButton.textContent='Refresh';
const terminalButton=document.createElement('button');terminalButton.type='button';terminalButton.textContent='Terminal here';
toolbar.append(workspaceSelect,newButton,editButton,deleteButton,refreshButton,terminalButton);

const pathbar=document.createElement('div');pathbar.className='remote-workspace-path';
const upButton=document.createElement('button');upButton.type='button';upButton.textContent='..';
const pathInput=document.createElement('input');pathInput.type='text';pathInput.value='.';pathInput.placeholder='Relative path';
const goButton=document.createElement('button');goButton.type='button';goButton.textContent='Go';
pathbar.append(upButton,pathInput,goButton);

const body=document.createElement('div');body.className='remote-workspace-body';
const files=document.createElement('div');files.className='remote-workspace-files';
const side=document.createElement('div');side.className='remote-workspace-side';
body.append(files,side);
dialog.append(head,toolbar,pathbar,body);backdrop.append(dialog);document.body.append(backdrop);

let workspaces=[];
let currentPath='.';
let currentEntries=[];
let busy=false;

function allowed(permission){
  return !app.sharedMode||Boolean(app.hasPermission?.(permission)||app.hasPermission?.('project.admin'));
}
function remoteCanRead(){return allowed('ssh.use')&&allowed('transfer.read')&&allowed('files.read');}
function remoteCanWrite(){return remoteCanRead()&&allowed('transfer.upload')&&allowed('files.write');}
function gitAllowed(action){
  const permission=action==='status'?'git.status':((action==='branches'||action==='log')?'git.log':(action==='diff'?'git.diff':(action==='push'?'git.push':'git.write')));
  return allowed('ssh.use')&&allowed(permission);
}
function syncPermissionControls(){
  newButton.disabled=busy||!allowed('settings.write');
  editButton.disabled=busy||!allowed('settings.write')||!selectedWorkspace();
  deleteButton.disabled=busy||!allowed('settings.write')||!selectedWorkspace();
  terminalButton.disabled=busy||!selectedWorkspace()||!allowed('ssh.use')||!allowed('terminal.create');
  refreshButton.disabled=busy||!selectedWorkspace()||!remoteCanRead();
  upButton.disabled=busy||!selectedWorkspace()||!remoteCanRead();
  pathInput.disabled=busy||!selectedWorkspace()||!remoteCanRead();
  goButton.disabled=busy||!selectedWorkspace()||!remoteCanRead();
}
function selectedWorkspace(){return workspaces.find(item=>String(item.id)===String(workspaceSelect.value))||null;}
function basename(value){const parts=String(value||'').split('/');return parts[parts.length-1]||value;}
function joinRelative(base,name){
  base=String(base||'.').replace(/\\/g,'/');name=String(name||'').replace(/\\/g,'/');
  const parts=(base==='.'?[]:base.split('/')).concat(name.split('/')),out=[];
  for(const part of parts){if(!part||part==='.')continue;if(part==='..'){out.pop();continue;}out.push(part);}
  return out.length?out.join('/'):'.';
}
function parentRelative(value){
  value=String(value||'.');if(value==='.')return '.';
  const parts=value.split('/').filter(Boolean);parts.pop();return parts.length?parts.join('/'):'.';
}
function formatBytes(value){
  value=Number(value||0);if(value<1024)return value+' B';if(value<1048576)return (value/1024).toFixed(value<10240?1:0)+' KiB';return (value/1048576).toFixed(1)+' MiB';
}
function setBusy(value,message=''){
  busy=Boolean(value);status.textContent=message;
  workspaceSelect.disabled=busy;
  syncPermissionControls();
}
async function refreshRegistries(){
  await Promise.allSettled([
    globalThis.TaskMenuConnections?.refresh?.(),
    globalThis.TaskMenuFileTransfer?.refreshProfiles?.(),
    globalThis.TaskMenuDatabase?.refreshProfiles?.()
  ]);
}
async function loadWorkspaces({keepSelection=true}={}){
  const previous=keepSelection?String(workspaceSelect.value||''):'';
  const data=await app.jsonFetch('/api/remote-workspaces',{cache:'no-store'});
  workspaces=Array.isArray(data?.workspaces)?data.workspaces:[];
  workspaceSelect.replaceChildren();
  for(const item of workspaces){
    const option=document.createElement('option');option.value=String(item.id);option.textContent=String(item.name||item.id);workspaceSelect.append(option);
  }
  if(previous&&workspaces.some(item=>String(item.id)===previous))workspaceSelect.value=previous;
  currentPath='.';pathInput.value=currentPath;syncPermissionControls();
  if(workspaces.length)await loadDirectory('.');
  else{renderFiles();renderSide();}
}
async function api(operation,path=currentPath,extra={}){
  const workspace=selectedWorkspace();if(!workspace)throw new Error('Choose a Remote Workspace');
  return app.jsonFetch('/api/remote-workspace-files',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({workspace_id:workspace.id,operation,path,...extra})
  });
}
async function loadDirectory(pathValue=currentPath){
  if(!selectedWorkspace()){currentEntries=[];renderFiles();renderSide();return;}
  setBusy(true,'Loading remote files…');
  try{
    const data=await api('list',pathValue);
    currentPath=String(data?.path||pathValue||'.')||'.';pathInput.value=currentPath;
    currentEntries=Array.isArray(data?.entries)?data.entries:[];
    renderFiles();renderSide();status.textContent=currentEntries.length+' item(s)';
  }finally{setBusy(false,status.textContent);}
}
function entryIsDir(entry){return ['dir','directory','folder'].includes(String(entry?.type||'').toLowerCase());}
function entryModified(entry){return String(entry?.modified||entry?.mtime||entry?.modified_at||'');}
function renderFiles(){
  files.replaceChildren();
  if(!selectedWorkspace()){const empty=document.createElement('div');empty.className='remote-workspace-empty';empty.textContent='Create or select a Remote Workspace';files.append(empty);return;}
  const table=document.createElement('table');table.className='remote-workspace-table';
  const thead=document.createElement('thead');const hr=document.createElement('tr');
  for(const name of ['Name','Type','Size','Modified']){const th=document.createElement('th');th.textContent=name;hr.append(th);}thead.append(hr);table.append(thead);
  const tbody=document.createElement('tbody');
  const sorted=[...currentEntries].sort((a,b)=>Number(entryIsDir(b))-Number(entryIsDir(a))||String(a.name||'').localeCompare(String(b.name||'')));
  for(const entry of sorted){
    const tr=document.createElement('tr');
    const name=document.createElement('td');name.textContent=(entryIsDir(entry)?'▸ ':'')+String(entry.name||'');
    const type=document.createElement('td');type.className='remote-workspace-kind';type.textContent=String(entry.type||'file');
    const size=document.createElement('td');size.textContent=entryIsDir(entry)?'—':formatBytes(entry.size);
    const modified=document.createElement('td');modified.textContent=entryModified(entry)||'—';
    tr.append(name,type,size,modified);
    tr.ondblclick=()=>openEntry(entry).catch(app.showError);
    name.onclick=()=>openEntry(entry).catch(app.showError);
    tbody.append(tr);
  }
  table.append(tbody);files.append(table);
}
async function openEntry(entry){
  const rel=joinRelative(currentPath,String(entry.name||''));
  if(entryIsDir(entry)){await loadDirectory(rel);return;}
  await openRemoteFile(rel);
}
function remoteVirtualPath(workspace,relative){
  return 'remote://'+encodeURIComponent(String(workspace.id))+'/'+String(relative||'').replace(/^\/+/, '');
}
function remoteEditorFile(workspace,relative,data){
  const raw=String(data?.content??''),bom=raw.charCodeAt(0)===0xfeff,content=bom?raw.slice(1):raw;
  const size=new TextEncoder().encode(raw).length;
  return {
    path:remoteVirtualPath(workspace,relative),remote_workspace_id:String(workspace.id),remote_path:String(relative),
    content,sha256:String(data?.sha256||''),size,line_ending:/\r\n/.test(content)?'crlf':'lf',bom,
    encoding:'utf-8',read_only:!remoteCanWrite(),large_file:false,warning:'Remote Workspace · '+String(workspace.name||workspace.id)
  };
}
async function openRemoteFile(relative){
  const workspace=selectedWorkspace();if(!workspace)throw new Error('Choose a Remote Workspace');
  setBusy(true,'Opening '+relative+'…');
  try{
    const data=await api('read',relative);
    return globalThis.TaskMenuEditor?.openDocument?.(remoteEditorFile(workspace,relative,data));
  }finally{setBusy(false,'');}
}
async function openTerminalHere(){
  const workspace=selectedWorkspace();if(!workspace)throw new Error('Choose a Remote Workspace');
  const root=String(workspace.remote_root||'/').replace(/\/+$/,'')||'/';
  const remoteCwd=currentPath==='.'?root:(root==='/'?'/'+currentPath:root+'/'+currentPath);
  const meta=await app.jsonFetch('/api/sessions',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({kind:'terminal',ssh_profile_id:workspace.ssh_profile_id,remote_cwd:remoteCwd})
  });
  app.materializeSession(meta,true);
}
async function runGit(action){
  const workspace=selectedWorkspace();if(!workspace)throw new Error('Choose a Remote Workspace');
  const output=side.querySelector('.remote-workspace-output');
  setBusy(true,'Git '+action+'…');
  try{
    const data=await app.jsonFetch('/api/remote-workspace-git',{
      method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({workspace_id:workspace.id,action})
    });
    if(output)output.textContent=String(data?.output||'(no output)');
  }finally{setBusy(false,'');}
}
function renderSide(){
  side.replaceChildren();const workspace=selectedWorkspace();
  if(!workspace){const empty=document.createElement('div');empty.className='remote-workspace-empty';empty.textContent='No Remote Workspace selected';side.append(empty);return;}
  const meta=document.createElement('div');meta.className='remote-workspace-section';
  const metaTitle=document.createElement('strong');metaTitle.textContent=String(workspace.name||workspace.id);
  const metaText=document.createElement('div');metaText.className='remote-workspace-meta';
  metaText.textContent='Root '+workspace.remote_root+' · SSH '+workspace.ssh_profile_id+' · SFTP '+workspace.transfer_profile_id;
  meta.append(metaTitle,metaText);side.append(meta);

  const git=document.createElement('div');git.className='remote-workspace-section';
  const gitTitle=document.createElement('strong');gitTitle.textContent='Git on remote host';
  const gitActions=document.createElement('div');gitActions.className='remote-workspace-actions';
  for(const [action,label] of [['status','Status'],['branches','Branches'],['log','Log'],['diff','Diff'],['fetch','Fetch'],['pull','Pull'],['push','Push']]){
    const button=document.createElement('button');button.type='button';button.textContent=label;button.disabled=!gitAllowed(action);button.onclick=()=>runGit(action).catch(app.showError);gitActions.append(button);
  }
  const output=document.createElement('pre');output.className='remote-workspace-output';output.textContent='Run a Git action to see output.';
  git.append(gitTitle,gitActions,output);side.append(git);

  const db=document.createElement('div');db.className='remote-workspace-section';
  const dbTitle=document.createElement('strong');dbTitle.textContent='Linked databases';
  const dbActions=document.createElement('div');dbActions.className='remote-workspace-actions';
  const ids=Array.isArray(workspace.database_profile_ids)?workspace.database_profile_ids:[];
  if(!ids.length){const text=document.createElement('div');text.className='remote-workspace-meta';text.textContent='No linked database profiles';db.append(dbTitle,text);}
  else{
    for(const id of ids){
      const profile=globalThis.TaskMenuDatabase?.getProfile?.(id);
      const button=document.createElement('button');button.type='button';button.textContent=String(profile?.name||id);
      button.onclick=()=>globalThis.TaskMenuDatabase?.openProfile?.(id).catch(app.showError);dbActions.append(button);
    }
    db.append(dbTitle,dbActions);
  }
  side.append(db);
}
async function showProfileDialog(existing=null){
  await refreshRegistries();
  const overlay=document.createElement('div');overlay.className='remote-workspace-backdrop visible';overlay.style.zIndex='17730';
  const card=document.createElement('div');card.className='remote-workspace-dialog';card.style.width='min(720px,95vw)';
  const h=document.createElement('div');h.className='remote-workspace-head';const heading=document.createElement('strong');heading.textContent=existing?'Edit Remote Workspace':'New Remote Workspace';h.append(heading);
  const form=document.createElement('div');form.className='remote-workspace-form';
  const add=(label,node)=>{const l=document.createElement('label');l.textContent=label;form.append(l,node);};
  const name=document.createElement('input');name.value=String(existing?.name||'');
  const root=document.createElement('input');root.value=String(existing?.remote_root||'/srv/app');root.placeholder='/absolute/remote/path';
  const ssh=document.createElement('select');
  for(const profile of globalThis.TaskMenuConnections?.sshProfiles||[]){const option=document.createElement('option');option.value=profile.id;option.textContent=profile.name||profile.host||profile.id;ssh.append(option);}
  const transfer=document.createElement('select');
  for(const profile of globalThis.TaskMenuFileTransfer?.profiles||[]){if(String(profile.protocol||'').toLowerCase()!=='sftp')continue;const option=document.createElement('option');option.value=profile.id;option.textContent=profile.name||profile.host||profile.id;transfer.append(option);}
  const db=document.createElement('select');db.multiple=true;db.size=6;
  for(const profile of globalThis.TaskMenuDatabase?.profiles||[]){const option=document.createElement('option');option.value=profile.id;option.textContent=profile.name||profile.id;db.append(option);}
  if(existing){ssh.value=String(existing.ssh_profile_id||'');transfer.value=String(existing.transfer_profile_id||'');for(const option of db.options)option.selected=(existing.database_profile_ids||[]).includes(option.value);}
  add('Name',name);add('Remote root',root);add('SSH profile',ssh);add('SFTP profile',transfer);add('Databases',db);
  const hint=document.createElement('div');hint.className='wide remote-workspace-meta';hint.textContent='The SFTP profile must reference the same SSH profile. Linked tunneled database profiles must use that SSH profile too.';form.append(hint);
  const footer=document.createElement('div');footer.className='remote-workspace-form-actions';
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
  const save=document.createElement('button');save.type='button';save.textContent='Save';footer.append(cancel,save);
  card.append(h,form,footer);overlay.append(card);document.body.append(overlay);
  const close=()=>overlay.remove();cancel.onclick=close;overlay.onmousedown=event=>{if(event.target===overlay)close();};
  save.onclick=async()=>{
    save.disabled=true;
    try{
      const payload={name:name.value.trim(),remote_root:root.value.trim(),ssh_profile_id:ssh.value,transfer_profile_id:transfer.value,database_profile_ids:[...db.selectedOptions].map(option=>option.value)};
      const url=existing?'/api/remote-workspaces/'+encodeURIComponent(existing.id):'/api/remote-workspaces';
      await app.jsonFetch(url,{method:existing?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});
      close();await loadWorkspaces({keepSelection:false});
    }catch(error){app.showError(error);save.disabled=false;}
  };
}
async function deleteWorkspace(){
  const workspace=selectedWorkspace();if(!workspace)return;
  if(!window.confirm('Delete Remote Workspace profile '+workspace.name+'? No remote files will be deleted.'))return;
  await app.jsonFetch('/api/remote-workspaces/'+encodeURIComponent(workspace.id),{method:'DELETE'});
  await loadWorkspaces({keepSelection:false});
}
async function open(){
  backdrop.classList.add('visible');
  await refreshRegistries();await loadWorkspaces();
}
function close(){backdrop.classList.remove('visible');}

workspaceSelect.onchange=()=>{currentPath='.';pathInput.value='.';syncPermissionControls();loadDirectory('.').catch(app.showError);};
refreshButton.onclick=()=>loadDirectory(currentPath).catch(app.showError);
terminalButton.onclick=()=>openTerminalHere().catch(app.showError);
upButton.onclick=()=>loadDirectory(parentRelative(currentPath)).catch(app.showError);
goButton.onclick=()=>loadDirectory(pathInput.value.trim()||'.').catch(app.showError);
pathInput.onkeydown=event=>{if(event.key==='Enter'){event.preventDefault();goButton.click();}};
newButton.onclick=()=>showProfileDialog(null).catch(app.showError);
editButton.onclick=()=>showProfileDialog(selectedWorkspace()).catch(app.showError);
deleteButton.onclick=()=>deleteWorkspace().catch(app.showError);
closeButton.onclick=close;backdrop.onmousedown=event=>{if(event.target===backdrop)close();};
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();});

globalThis.TaskMenuRemoteWorkspace={open,close,refresh:loadWorkspaces,openRemoteFile,openTerminalHere,runGit,get workspaces(){return [...workspaces];},get selected(){return selectedWorkspace();},get visible(){return backdrop.classList.contains('visible');}};
