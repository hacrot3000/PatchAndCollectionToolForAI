const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for project profiles');

const endpoint='/api/project-profiles';
let profiles=[];
let activeProfileID='';
let manager=null;

const style=document.createElement('style');
style.textContent=`
.project-profile-backdrop{position:fixed;inset:0;z-index:19400;display:none;align-items:center;justify-content:center;background:rgba(0,0,0,.62);padding:18px}
.project-profile-backdrop.visible{display:flex}
.project-profile-dialog{width:min(1040px,97vw);height:min(780px,94vh);display:grid;grid-template-columns:260px minmax(0,1fr);background:#11161d;border:1px solid #4a5362;border-radius:10px;overflow:hidden;box-shadow:0 22px 70px rgba(0,0,0,.6)}
.project-profile-list{padding:10px;border-right:1px solid #30343b;overflow:auto;display:flex;flex-direction:column;gap:6px}
.project-profile-list h3{margin:0 0 5px}.project-profile-list button{text-align:left}
.project-profile-list .selected{border-color:#75a9d6;background:#203b58}
.project-profile-editor{padding:12px;overflow:auto;display:flex;flex-direction:column;gap:10px}
.project-profile-field{display:grid;gap:5px}.project-profile-field>label{font-size:11px;font-weight:700;opacity:.72}
.project-profile-field input,.project-profile-field select,.project-profile-field textarea{width:100%;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:7px 9px}
.project-profile-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}
.project-profile-checks{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:4px 10px;border:1px solid #30343b;border-radius:7px;padding:7px;max-height:170px;overflow:auto}
.project-profile-check{display:flex;align-items:center;gap:6px;font-size:11px;min-width:0}.project-profile-check input{width:auto}.project-profile-check span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.project-profile-actions{display:flex;gap:7px;align-items:center;position:sticky;bottom:-12px;background:#11161d;padding:10px 0 2px}.project-profile-actions .spacer{flex:1}
.project-profile-danger{background:#4a252a;border-color:#7a4048;color:#ffe2e4}
.project-profile-note{font-size:10px;opacity:.68;line-height:1.45}
html[data-taskmenu-theme='light'] .project-profile-dialog{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme='light'] .project-profile-list{border-color:#d0d7de}
html[data-taskmenu-theme='light'] .project-profile-field input,html[data-taskmenu-theme='light'] .project-profile-field select,html[data-taskmenu-theme='light'] .project-profile-field textarea{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme='light'] .project-profile-actions{background:#fff}
@media(max-width:760px){.project-profile-dialog{grid-template-columns:1fr;height:min(900px,96vh)}.project-profile-list{max-height:220px;border-right:0;border-bottom:1px solid #30343b}.project-profile-grid,.project-profile-checks{grid-template-columns:1fr}}
`;document.head.append(style);

function canRead(){return !app.sharedMode||app.hasPermission?.('settings.read')||app.hasPermission?.('project.admin');}
function canWrite(){return !app.sharedMode||app.hasPermission?.('settings.write')||app.hasPermission?.('project.admin');}
async function loadProfiles(){if(!canRead()){profiles=[];return profiles;}const data=await app.jsonFetch(endpoint,{cache:'no-store'});profiles=Array.isArray(data?.profiles)?data.profiles:[];return profiles;}
function byID(id){return profiles.find(item=>item.id===id)||null;}
function uniqueStrings(values){return [...new Set((Array.isArray(values)?values:[]).map(value=>String(value||'').trim()).filter(Boolean))];}
function uniqueInts(values){return [...new Set((Array.isArray(values)?values:[]).map(Number).filter(value=>Number.isInteger(value)&&value>0))];}
function localTerminalViews(){return [...app.views.values()].filter(view=>view?.meta?.task_id===0&&String(view.meta?.target_type||'local').toLowerCase()!=='ssh'&&!view.meta?.target_profile_id);}
function projectRelativeCwd(value){
  const root=String(app.taskData?.workspace||'').trim().replace(/\\/g,'/').replace(/\/+$/,'');
  const cwd=String(value||'').trim().replace(/\\/g,'/').replace(/\/+$/,'');
  if(!root||!cwd||cwd===root)return '.';
  const prefix=root+'/';
  return cwd.startsWith(prefix)?cwd.slice(prefix.length):'.';
}
function sshProfileIDsFromViews(){return uniqueStrings([...app.views.values()].filter(view=>view?.meta?.task_id===-1&&String(view.meta?.target_type||'').toLowerCase()==='ssh').map(view=>view.meta?.target_profile_id));}
function openDatabaseProfileIDs(){return uniqueStrings([...(globalThis.TaskMenuDatabase?.views?.values?.()||[])].map(view=>view.meta?.profile_id||view.profile?.id));}
function openTransferProfileIDs(){return uniqueStrings([...(globalThis.TaskMenuFileTransfer?.views?.values?.()||[])].map(view=>view.profile?.id));}
function currentDraft(name=''){
  const terminals=localTerminalViews().slice(0,16).map(view=>({cwd:projectRelativeCwd(view.meta?.cwd),title:String(view.meta?.title||'')}));
  return {
    name:String(name||''),
    environment_profile:String(globalThis.TaskMenuEnvProfiles?.currentName?.()||''),
    command_preset_ids:uniqueStrings(globalThis.TaskMenuCommandPresets?.projectPresetIDs||[]),
    terminals,
    database_profile_ids:openDatabaseProfileIDs(),
    ssh_profile_ids:sshProfileIDsFromViews(),
    transfer_profile_ids:openTransferProfileIDs(),
    default_git_repository:String(globalThis.TaskMenuGitFiles?.snapshotState?.()?.repository_id||''),
    task_ids:uniqueInts(globalThis.TaskMenuTaskSet?.ids||[])
  };
}
function cloneProfile(profile){return JSON.parse(JSON.stringify(profile||currentDraft('')));}

async function createLocalTerminal(spec){
  const cwd=String(spec?.cwd||'.').trim()||'.';
  let meta=await app.jsonFetch('/api/sessions',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({kind:'terminal',cwd})});
  const view=app.materializeSession?.(meta,true)||app.attachSession?.(meta,true);
  const title=String(spec?.title||'').trim();
  if(title&&meta?.id){
    try{meta=await app.jsonFetch('/api/sessions/'+encodeURIComponent(meta.id)+'/title',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({title})});if(view?.meta)Object.assign(view.meta,meta);}catch(error){console.warn('Project profile terminal title restore failed',error);}
  }
  return meta;
}
function existingDatabaseProfile(profileID){return [...(globalThis.TaskMenuDatabase?.views?.values?.()||[])].find(view=>String(view.meta?.profile_id||view.profile?.id||'')===profileID)||null;}
function existingTransferProfile(profileID){return [...(globalThis.TaskMenuFileTransfer?.views?.values?.()||[])].find(view=>String(view.profile?.id||'')===profileID)||null;}

async function applyProfile(profile,{confirmOpen=true}={}){
  if(typeof profile==='string'){await loadProfiles();profile=byID(profile);}
  if(!profile)throw new Error('Project profile not found');
  const resources=(profile.terminals?.length||0)+(profile.database_profile_ids?.length||0)+(profile.ssh_profile_ids?.length||0)+(profile.transfer_profile_ids?.length||0);
  if(confirmOpen&&resources&& !window.confirm('Apply project profile "'+profile.name+'"?\n\nThis will select its environment/task/preset/Git context and open '+resources+' configured terminal/connection resource(s). No task or command preset will run automatically.'))return false;
  const warnings=[];
  const envName=String(profile.environment_profile||'');
  if(envName){try{globalThis.TaskMenuEnvProfiles?.select?.(envName);}catch(error){warnings.push(String(error?.message||error));}}else globalThis.TaskMenuEnvProfiles?.select?.('');
  globalThis.TaskMenuCommandPresets?.setProjectPresetIDs?.(profile.command_preset_ids||[]);
  globalThis.TaskMenuTaskSet?.apply?.(profile.task_ids||[]);
  if(profile.default_git_repository){try{await globalThis.TaskMenuGitFiles?.restoreState?.({repository_id:profile.default_git_repository});}catch(error){warnings.push('Git: '+String(error?.message||error));}}
  for(const terminal of profile.terminals||[]){try{await createLocalTerminal(terminal);}catch(error){warnings.push('Terminal: '+String(error?.message||error));}}
  for(const id of profile.ssh_profile_ids||[]){try{await globalThis.TaskMenuConnections?.openSSHProfile?.(id);}catch(error){warnings.push('SSH '+id+': '+String(error?.message||error));}}
  for(const id of profile.database_profile_ids||[]){
    try{const existing=existingDatabaseProfile(id);if(existing){continue;}await globalThis.TaskMenuDatabase?.openProfile?.(id);}catch(error){warnings.push('DB '+id+': '+String(error?.message||error));}
  }
  for(const id of profile.transfer_profile_ids||[]){
    try{const existing=existingTransferProfile(id);if(existing){continue;}await globalThis.TaskMenuFileTransfer?.openProfile?.(id);}catch(error){warnings.push('Transfer '+id+': '+String(error?.message||error));}
  }
  activeProfileID=String(profile.id||'');
  try{localStorage.setItem('taskdeck:project-profile-active:'+String(app.taskData?.workspace||''),activeProfileID);}catch{}
  window.dispatchEvent(new CustomEvent('taskmenu:project-profile-applied',{detail:{profile,warnings}}));
  if(warnings.length)console.warn('Project profile applied with warnings:',...warnings);
  return {ok:true,warnings};
}

async function availableOptions(){
  const [ssh,db,transfer]=await Promise.all([
    app.jsonFetch('/api/ssh/profiles').catch(()=>({profiles:[]})),
    app.jsonFetch('/api/db/profiles').catch(()=>({profiles:[]})),
    app.jsonFetch('/api/file-transfer/profiles').catch(()=>({profiles:[]}))
  ]);
  await globalThis.TaskMenuCommandPresets?.refresh?.().catch?.(()=>{});
  return {
    env:Object.keys(globalThis.TaskMenuEnvProfiles?.list?.()||{}).sort(),
    presets:Array.isArray(globalThis.TaskMenuCommandPresets?.state?.presets)?globalThis.TaskMenuCommandPresets.state.presets:[],
    tasks:Array.isArray(app.taskData?.tasks)?app.taskData.tasks:[],
    ssh:Array.isArray(ssh?.profiles)?ssh.profiles:[],
    db:Array.isArray(db?.profiles)?db.profiles:[],
    transfer:Array.isArray(transfer?.profiles)?transfer.profiles:[]
  };
}

function checkboxGroup(label,items,selected,valueOf,labelOf){
  const wrap=document.createElement('div');wrap.className='project-profile-field';const title=document.createElement('label');title.textContent=label;
  const box=document.createElement('div');box.className='project-profile-checks';const chosen=new Set((selected||[]).map(String));
  for(const item of items){const value=String(valueOf(item));const row=document.createElement('label');row.className='project-profile-check';const input=document.createElement('input');input.type='checkbox';input.value=value;input.checked=chosen.has(value);const text=document.createElement('span');text.textContent=labelOf(item);row.append(input,text);box.append(row);}
  wrap.append(title,box);return {wrap,box,values:()=>[...box.querySelectorAll('input:checked')].map(input=>input.value)};
}
function field(label,value=''){const wrap=document.createElement('div');wrap.className='project-profile-field';const l=document.createElement('label');l.textContent=label;const input=document.createElement('input');input.value=String(value||'');wrap.append(l,input);return {wrap,input};}
function textarea(label,value=''){const f=field(label,value);const area=document.createElement('textarea');area.rows=5;area.value=f.input.value;f.input.replaceWith(area);f.input=area;return f;}
function terminalsText(items){return (items||[]).map(item=>String(item.cwd||'.')+(item.title?' | '+item.title:'')).join('\n');}
function parseTerminals(text){return String(text||'').split(/\r?\n/).map(line=>line.trim()).filter(Boolean).slice(0,16).map(line=>{const parts=line.split('|');return {cwd:String(parts.shift()||'.').trim()||'.',title:parts.join('|').trim()};});}

async function saveDraft(draft){
  if(!canWrite())throw new Error('Project profile write permission is required');
  const response=await app.jsonFetch(endpoint,{method:draft.id?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(draft)});
  await loadProfiles();return response;
}
async function deleteProfile(profile){
  if(!canWrite())throw new Error('Project profile write permission is required');
  if(!window.confirm('Delete project profile "'+profile.name+'"?'))return false;
  await app.jsonFetch(endpoint+'?id='+encodeURIComponent(profile.id),{method:'DELETE'});if(activeProfileID===profile.id)activeProfileID='';await loadProfiles();return true;
}

async function openManager(selectedID=''){
  if(!canRead())throw new Error('Project profile read permission is required');
  await loadProfiles();const options=await availableOptions();
  if(manager?.host?.isConnected)manager.host.remove();
  const host=document.createElement('div');host.className='project-profile-backdrop visible';
  const dialog=document.createElement('div');dialog.className='project-profile-dialog';host.append(dialog);document.body.append(host);
  const sidebar=document.createElement('div');sidebar.className='project-profile-list';const heading=document.createElement('h3');heading.textContent='Project profiles';
  const add=document.createElement('button');add.type='button';add.textContent='＋ New from current';sidebar.append(heading,add);
  const editor=document.createElement('div');editor.className='project-profile-editor';dialog.append(sidebar,editor);
  manager={host,sidebar,editor,draft:null};
  const closeManager=()=>{host.remove();if(manager?.host===host)manager=null;};
  host.addEventListener('pointerdown',event=>{if(event.target===host)closeManager();});

  const selectDraft=profile=>{manager.draft=cloneProfile(profile||currentDraft(''));renderEditor();renderList();};
  const renderList=()=>{
    sidebar.querySelectorAll('.project-profile-entry').forEach(node=>node.remove());
    for(const profile of profiles){const b=document.createElement('button');b.type='button';b.className='project-profile-entry';b.classList.toggle('selected',manager.draft?.id===profile.id);b.textContent=profile.name;b.onclick=()=>selectDraft(profile);sidebar.append(b);}
  };
  const renderEditor=()=>{
    const draft=manager.draft||currentDraft('');editor.replaceChildren();
    const name=field('Profile name',draft.name);
    const envWrap=document.createElement('div');envWrap.className='project-profile-field';const envLabel=document.createElement('label');envLabel.textContent='Environment profile';const env=document.createElement('select');const def=document.createElement('option');def.value='';def.textContent='Default';env.append(def);for(const item of options.env){const option=document.createElement('option');option.value=item;option.textContent=item;env.append(option);}env.value=draft.environment_profile||'';envWrap.append(envLabel,env);
    const git=field('Default Git repository',draft.default_git_repository||'.');
    const terminals=textarea('Startup terminals — one cwd | title per line',terminalsText(draft.terminals));
    const presets=checkboxGroup('Command preset set (selected only; never auto-run)',options.presets,draft.command_preset_ids,item=>item.id,item=>item.name);
    const tasks=checkboxGroup('Task set (quick access; never auto-run)',options.tasks,draft.task_ids,item=>item.id,item=>String(item.menu_label||item.label||item.id));
    const ssh=checkboxGroup('SSH terminals to open',options.ssh,draft.ssh_profile_ids,item=>item.id,item=>item.name||item.id);
    const db=checkboxGroup('Database profiles to open',options.db,draft.database_profile_ids,item=>item.id,item=>item.name||item.id);
    const transfer=checkboxGroup('FTP/SFTP profiles to open',options.transfer,draft.transfer_profile_ids,item=>item.id,item=>(item.name||item.id)+' · '+String(item.protocol||'').toUpperCase());
    const grid=document.createElement('div');grid.className='project-profile-grid';grid.append(name.wrap,envWrap,git.wrap,terminals.wrap);editor.append(grid,presets.wrap,tasks.wrap,ssh.wrap,db.wrap,transfer.wrap);
    const note=document.createElement('div');note.className='project-profile-note';note.textContent='Project profiles store only references to existing connection profiles. Passwords, private-key passphrases and database/FTP secrets remain in their existing secret stores and are never copied here.';editor.append(note);
    const actions=document.createElement('div');actions.className='project-profile-actions';
    const close=document.createElement('button');close.type='button';close.textContent='Close';close.onclick=closeManager;
    const apply=document.createElement('button');apply.type='button';apply.textContent='Apply';apply.disabled=!draft.id;apply.onclick=()=>applyProfile(draft).catch(app.showError);
    const spacer=document.createElement('span');spacer.className='spacer';actions.append(close,apply,spacer);
    if(draft.id&&canWrite()){const del=document.createElement('button');del.type='button';del.className='project-profile-danger';del.textContent='Delete';del.onclick=async()=>{if(await deleteProfile(draft)){selectDraft(profiles[0]||null);}};actions.append(del);}
    if(canWrite()){const save=document.createElement('button');save.type='button';save.textContent='Save';save.onclick=async()=>{const payload={id:draft.id||'',name:name.input.value,environment_profile:env.value,command_preset_ids:presets.values(),terminals:parseTerminals(terminals.input.value),database_profile_ids:db.values(),ssh_profile_ids:ssh.values(),transfer_profile_ids:transfer.values(),default_git_repository:git.input.value,task_ids:tasks.values().map(Number)};const saved=await saveDraft(payload);selectDraft(saved);};actions.append(save);}
    editor.append(actions);
  };
  add.onclick=()=>selectDraft(currentDraft(''));
  let selected=selectedID?byID(selectedID):null;if(!selected)selected=byID(activeProfileID)||profiles[0]||null;selectDraft(selected);
  return manager;
}

function installLauncher(){
  const header=document.querySelector('header');if(!header||document.querySelector('#project-profile-open'))return false;
  const button=document.createElement('button');button.id='project-profile-open';button.type='button';button.textContent='Profile…';button.title='Project profiles';button.onclick=()=>openManager().catch(app.showError);
  const reload=document.querySelector('#reload');header.insertBefore(button,reload);return true;
}
try{activeProfileID=localStorage.getItem('taskdeck:project-profile-active:'+String(app.taskData?.workspace||''))||'';}catch{}
if(!installLauncher())window.addEventListener('taskmenu:tasks',installLauncher,{once:true});

globalThis.TaskMenuProjectProfiles={load:loadProfiles,open:openManager,apply:applyProfile,currentDraft,get activeProfileID(){return activeProfileID;},get profiles(){return profiles;}};
