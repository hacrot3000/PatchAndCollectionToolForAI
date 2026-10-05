const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for workspace snapshots');

const endpoint='/api/workspace-snapshots';
let snapshots=[];
let open=false;

const style=document.createElement('style');
style.textContent=`
.workspace-snapshot-backdrop{position:fixed;inset:0;z-index:19300;display:none;align-items:center;justify-content:center;background:rgba(0,0,0,.62);padding:18px}
.workspace-snapshot-backdrop.visible{display:flex}
.workspace-snapshot-dialog{width:min(860px,96vw);max-height:88vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #4a5362;border-radius:10px;overflow:hidden;box-shadow:0 22px 70px rgba(0,0,0,.6)}
.workspace-snapshot-head,.workspace-snapshot-actions{display:flex;align-items:center;gap:8px;padding:10px 12px;border-bottom:1px solid #30343b}
.workspace-snapshot-head strong{flex:1}.workspace-snapshot-actions{border-top:1px solid #30343b;border-bottom:0}
.workspace-snapshot-actions input{flex:1;min-width:0}
.workspace-snapshot-list{overflow:auto;padding:10px;display:grid;gap:8px}
.workspace-snapshot-row{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:10px;border:1px solid #30343b;border-radius:8px;padding:9px;background:#151b23}
.workspace-snapshot-name{font-weight:700}.workspace-snapshot-meta{font-size:10px;opacity:.68;margin-top:3px}.workspace-snapshot-summary{font-size:10px;opacity:.82;margin-top:5px}
.workspace-snapshot-row-actions{display:flex;gap:5px;align-items:flex-start;flex-wrap:wrap;justify-content:flex-end}.workspace-snapshot-empty{padding:24px;text-align:center;opacity:.6}
.workspace-snapshot-launcher{font-size:16px;font-weight:700}
html[data-taskmenu-theme='light'] .workspace-snapshot-dialog{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme='light'] .workspace-snapshot-row{background:#f8fafc;border-color:#d0d7de}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='workspace-snapshot-backdrop';
const dialog=document.createElement('div');dialog.className='workspace-snapshot-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');
const head=document.createElement('div');head.className='workspace-snapshot-head';
const title=document.createElement('strong');title.textContent='Workspace snapshots';
const close=document.createElement('button');close.type='button';close.textContent='×';close.title='Close';head.append(title,close);
const actions=document.createElement('div');actions.className='workspace-snapshot-actions';
const nameInput=document.createElement('input');nameInput.type='text';nameInput.maxLength=120;nameInput.placeholder='Snapshot name, e.g. Debug NFC issue';
const save=document.createElement('button');save.type='button';save.textContent='Save current';actions.append(nameInput,save);
const list=document.createElement('div');list.className='workspace-snapshot-list';
dialog.append(head,actions,list);backdrop.append(dialog);document.body.append(backdrop);

function canRead(){return !app.sharedMode||app.hasPermission?.('settings.read')||app.hasPermission?.('project.admin');}
function canWrite(){return !app.sharedMode||app.hasPermission?.('settings.write')||app.hasPermission?.('project.admin');}
function closeDialog(){open=false;backdrop.classList.remove('visible');}
close.onclick=closeDialog;backdrop.addEventListener('pointerdown',event=>{if(event.target===backdrop)closeDialog();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&open)closeDialog();});

function editorToken(path){return 'editor:'+String(path||'');}
function databaseToken(profileID,index=0){return 'database:'+String(profileID||'')+':'+index;}
function transferToken(profileID){return 'file-transfer:'+String(profileID||'');}
function terminalTokens(){
  const terminal=globalThis.TaskMenuTerminalRestore?.snapshotPayload?.()||{};
  return (Array.isArray(terminal.session_ids)?terminal.session_ids:[]).map((id,index)=>[String(id),`terminal:${index}`]);
}
function semanticTabState(){
  const tokenByID=new Map(terminalTokens());
  const editor=globalThis.TaskMenuEditor;
  for(const [id,view] of editor?.editors||[])tokenByID.set(String(id),editorToken(view?.file?.path));
  const dbCounts=new Map();
  for(const view of globalThis.TaskMenuDatabase?.views?.values?.()||[]){
    const profile=String(view.meta?.profile_id||view.profile?.id||'');if(!profile)continue;
    const index=dbCounts.get(profile)||0;dbCounts.set(profile,index+1);tokenByID.set(String(view.tab?.dataset?.id||view.meta?.id||''),databaseToken(profile,index));
  }
  for(const view of globalThis.TaskMenuFileTransfer?.views?.values?.()||[]){
    const profile=String(view.profile?.id||'');if(profile)tokenByID.set(String(view.tab?.dataset?.id||''),transferToken(profile));
  }
  const ids=globalThis.TaskMenuTabOrder?.currentIDs?.()||[];
  const order=ids.map(id=>tokenByID.get(String(id))).filter(Boolean);
  const activeTab=document.querySelector('#tabs > .active');
  const active=activeTab?tokenByID.get(String(activeTab.dataset.id||''))||'':'';
  return {order,active};
}

async function captureState(){
  try{await globalThis.TaskMenuTerminalRestore?.persistSnapshot?.();}catch(error){console.warn('Cannot flush terminal state before workspace snapshot',error);}
  const tabs=semanticTabState();
  const editor=globalThis.TaskMenuEditor?.snapshotState?.()||{files:[],active:''};
  const explorer=globalThis.TaskMenuExplorer?.snapshotState?.()||{expanded:[]};
  const databases=globalThis.TaskMenuDatabase?.snapshotState?.()||[];
  const transfers=globalThis.TaskMenuFileTransfer?.snapshotState?.()||[];
  const git=globalThis.TaskMenuGitFiles?.snapshotState?.()||{};
  return {
    tab_order:tabs.order,active_tab:tabs.active,
    editor_files:Array.isArray(editor.files)?editor.files:[],active_editor:String(editor.active||''),
    explorer_expanded:Array.isArray(explorer.expanded)?explorer.expanded:[],
    databases,transfers,git_repository_id:String(git.repository_id||'')
  };
}

async function loadSnapshots(){
  if(!canRead()){snapshots=[];render();return snapshots;}
  const data=await app.jsonFetch(endpoint,{cache:'no-store'});snapshots=Array.isArray(data?.snapshots)?data.snapshots:[];render();return snapshots;
}
function snapshotSummary(item){
  const state=item?.state||{},terminal=item?.terminal||{};
  const parts=[
    (state.editor_files?.length||0)+' editor',
    (terminal.terminals?.length||0)+' terminal',
    (state.databases?.length||0)+' DB',
    (state.transfers?.length||0)+' transfer'
  ];
  if(item?.git?.branch)parts.push('Git '+item.git.branch);
  return parts.join(' · ');
}
function button(label,handler){const b=document.createElement('button');b.type='button';b.textContent=label;b.onclick=handler;return b;}
function render(){
  list.replaceChildren();save.disabled=!canWrite();nameInput.disabled=!canWrite();
  if(!canRead()){const empty=document.createElement('div');empty.className='workspace-snapshot-empty';empty.textContent='You do not have permission to view workspace snapshots.';list.append(empty);return;}
  if(!snapshots.length){const empty=document.createElement('div');empty.className='workspace-snapshot-empty';empty.textContent='No workspace snapshots yet.';list.append(empty);return;}
  for(const item of snapshots){
    const row=document.createElement('div');row.className='workspace-snapshot-row';
    const body=document.createElement('div');
    const name=document.createElement('div');name.className='workspace-snapshot-name';name.textContent=String(item.name||'Snapshot');
    const meta=document.createElement('div');meta.className='workspace-snapshot-meta';meta.textContent=item.updated_at?new Date(item.updated_at).toLocaleString():'';
    const summary=document.createElement('div');summary.className='workspace-snapshot-summary';summary.textContent=snapshotSummary(item);body.append(name,meta,summary);
    const controls=document.createElement('div');controls.className='workspace-snapshot-row-actions';
    controls.append(button('Restore',()=>restoreSnapshot(item).catch(app.showError)));
    if(canWrite()){
      controls.append(button('Update',()=>saveSnapshot(item.id,item.name).catch(app.showError)));
      const del=button('Delete',()=>deleteSnapshot(item).catch(app.showError));del.className='task-connection-danger';controls.append(del);
    }
    row.append(body,controls);list.append(row);
  }
}
async function saveSnapshot(id='',existingName=''){
  if(!canWrite())throw new Error('Workspace snapshot write permission is required');
  const name=String(existingName||nameInput.value||'').trim();if(!name)throw new Error('Snapshot name is required');
  const state=await captureState();
  const response=await app.jsonFetch(endpoint,{method:id?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({...(id?{id}:{}),name,state})});
  nameInput.value='';await loadSnapshots();return response;
}
async function deleteSnapshot(item){
  if(!canWrite())throw new Error('Workspace snapshot write permission is required');
  if(!confirm('Delete workspace snapshot "'+String(item?.name||'')+'"?'))return false;
  await app.jsonFetch(endpoint+'?id='+encodeURIComponent(String(item.id||'')),{method:'DELETE'});await loadSnapshots();return true;
}

function resolvedTabIDs(state,terminalOverride=null){
  const ids=[];const tokens=Array.isArray(state?.tab_order)?state.tab_order:[];
  const terminalIDs=(Array.isArray(terminalOverride)?terminalOverride:[...(globalThis.TaskMenuTerminalRestore?.snapshotPayload?.()?.session_ids||[])]).map(String);
  const dbByProfile=new Map();
  for(const view of globalThis.TaskMenuDatabase?.views?.values?.()||[]){const profile=String(view.meta?.profile_id||view.profile?.id||'');if(!dbByProfile.has(profile))dbByProfile.set(profile,[]);dbByProfile.get(profile).push(String(view.tab?.dataset?.id||view.meta?.id||''));}
  const editorByPath=new Map();for(const [id,view] of globalThis.TaskMenuEditor?.editors||[])editorByPath.set(String(view.file?.path||''),String(id));
  for(const token of tokens){
    let match=/^terminal:(\d+)$/.exec(token);if(match){const id=terminalIDs[Number(match[1])];if(id)ids.push(id);continue;}
    if(token.startsWith('editor:')){const id=editorByPath.get(token.slice(7));if(id)ids.push(id);continue;}
    match=/^database:(.*):(\d+)$/.exec(token);if(match){const id=dbByProfile.get(match[1])?.[Number(match[2])];if(id)ids.push(id);continue;}
    if(token.startsWith('file-transfer:')){const id='file-transfer:'+token.slice('file-transfer:'.length);if(document.querySelector('#tabs > [data-id="'+CSS.escape(id)+'"]'))ids.push(id);}
  }
  return ids;
}
async function restoreActiveTab(token,terminalOverride=null){
  token=String(token||'');if(!token)return;
  const state={tab_order:[token]};const ids=resolvedTabIDs(state,terminalOverride);const id=ids[0];if(!id)return;
  if(app.views.has(id)){app.activateView(id,{focus:false});return;}
  if(token.startsWith('editor:')){await globalThis.TaskMenuEditor?.openFile?.(token.slice(7));return;}
  if(token.startsWith('database:')){app.activateExternalView?.('database:'+id,{force:true});return;}
  if(token.startsWith('file-transfer:')){app.activateExternalView?.(id,{force:true});}
}
async function restoreSnapshot(item){
  const state=item?.state||{};
  if(!confirm('Restore workspace snapshot "'+String(item?.name||'')+'"?\n\nThis opens the saved workspace context without closing currently open tabs. Saved local terminals are recreated with their CWD/title/split layout.'))return false;
  const terminalRestore=await globalThis.TaskMenuTerminalRestore?.restoreSnapshotState?.(item?.terminal);
  const terminalIDs=Array.isArray(terminalRestore?.session_ids)?terminalRestore.session_ids:[];
  if(Array.isArray(terminalRestore?.warnings)&&terminalRestore.warnings.length)console.warn('Workspace snapshot terminal restore:',...terminalRestore.warnings);
  await globalThis.TaskMenuEditor?.restoreState?.({files:state.editor_files,active:state.active_editor});
  await globalThis.TaskMenuExplorer?.restoreState?.({expanded:state.explorer_expanded});
  await globalThis.TaskMenuDatabase?.restoreState?.(state.databases);
  await globalThis.TaskMenuFileTransfer?.restoreState?.(state.transfers);
  await globalThis.TaskMenuGitFiles?.restoreState?.({repository_id:state.git_repository_id});
  const order=resolvedTabIDs(state,terminalIDs);if(order.length)globalThis.TaskMenuTabOrder?.applyOrder?.(order);
  await restoreActiveTab(state.active_tab,terminalIDs);
  return true;
}

async function openDialog(){open=true;backdrop.classList.add('visible');await loadSnapshots();nameInput.focus();}
save.onclick=()=>saveSnapshot().catch(app.showError);close.onclick=closeDialog;
document.addEventListener('keydown',event=>{if((event.ctrlKey||event.metaKey)&&event.altKey&&event.key.toLowerCase()==='s'){event.preventDefault();openDialog().catch(app.showError);}});

function installLauncher(){
  const rail=document.querySelector('.task-activity-bar');if(!rail)return false;
  if(rail.querySelector('[data-view="snapshots"]'))return true;
  const launcher=document.createElement('button');launcher.type='button';launcher.className='task-activity-button workspace-snapshot-launcher';launcher.dataset.view='snapshots';launcher.title='Workspace snapshots (Ctrl+Alt+S)';launcher.setAttribute('aria-label','Workspace snapshots');launcher.textContent='◫';launcher.onclick=()=>openDialog().catch(app.showError);rail.append(launcher);return true;
}
if(!installLauncher()){const observer=new MutationObserver(()=>{if(installLauncher())observer.disconnect();});observer.observe(document.body,{childList:true,subtree:true});}

globalThis.TaskMenuWorkspaceSnapshots={open:openDialog,load:loadSnapshots,capture:captureState,restore:restoreSnapshot,get items(){return snapshots;}};
