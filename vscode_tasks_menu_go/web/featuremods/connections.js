const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Connections');

let panel=null;
let content=null;
let localSettings={selected_cwd:'.',custom_dirs:[]};
let sshProfiles=[];
let dbAdapters=[];
let dbProfiles=[];

const style=document.createElement('style');
style.textContent=`
.task-connections-panel{display:none;position:fixed;top:52px;bottom:0;left:48px;z-index:1800;width:min(var(--taskmenu-sidebar-panel-width,310px),calc(100vw - 48px));background:#11151b;border-right:1px solid #3b414d;box-shadow:10px 0 28px rgba(0,0,0,.28);flex-direction:column}
.task-connections-panel.visible{display:flex}
.task-connections-head{height:42px;display:flex;align-items:center;gap:6px;padding:6px 8px;border-bottom:1px solid #30343b}
.task-connections-title{font-size:12px;font-weight:700;letter-spacing:.04em;flex:1}
.task-connections-refresh,.task-connections-close{padding:4px 7px;font-size:11px}
.task-connections-content{flex:1;min-height:0;overflow:auto;padding:7px}
.task-connection-section{margin-bottom:12px}
.task-connection-section-head{display:flex;align-items:center;gap:6px;margin-bottom:5px}
.task-connection-section-title{font-size:10px;font-weight:700;letter-spacing:.08em;opacity:.65;flex:1}
.task-connection-add{padding:3px 6px;font-size:10px}
.task-connection-row{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:2px 5px;align-items:center;padding:5px 5px;border-radius:5px}
.task-connection-row:hover{background:#1b2028}
.task-connection-open{min-width:0;text-align:left;border:0;background:transparent;padding:3px;color:inherit;overflow:hidden}
.task-connection-open:hover{background:transparent}
.task-connection-name{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px}
.task-connection-meta{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:10px;opacity:.55;margin-top:2px}
.task-connection-actions{display:flex;gap:3px}
.task-connection-actions button{padding:3px 5px;font-size:10px}
.task-connection-empty{font-size:11px;opacity:.55;padding:6px}
.task-connection-dialog{position:fixed;inset:0;z-index:10020;display:grid;place-items:center;padding:18px;background:rgba(3,5,8,.68)}
.task-connection-dialog-card{width:min(620px,100%);max-height:min(760px,92vh);overflow:auto;background:#171b22;border:1px solid #48515f;border-radius:9px;box-shadow:0 18px 55px rgba(0,0,0,.45);padding:14px}
.task-connection-dialog-card h3{margin:0 0 12px;font-size:15px}
.task-connection-form{display:grid;grid-template-columns:1fr 1fr;gap:9px}
.task-connection-field{display:flex;flex-direction:column;gap:4px;min-width:0}
.task-connection-field.wide{grid-column:1/-1}
.task-connection-field label{font-size:10px;opacity:.65}
.task-connection-field input,.task-connection-field select,.task-connection-field textarea{width:100%;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:7px;font:inherit}
.task-connection-field textarea{min-height:92px;resize:vertical;font-family:ui-monospace,monospace;font-size:12px}
.task-connection-dialog-actions{grid-column:1/-1;display:flex;justify-content:flex-end;gap:7px;margin-top:5px}
.task-connection-primary{background:#244c70;border-color:#3f79a8}
.task-connection-danger{background:#54252a;border-color:#7b3941}
body:not(.task-sidebar-auto-hide) .task-connections-panel{top:98px;left:0;width:var(--taskmenu-sidebar-inline-width,310px);box-shadow:none}
html[data-taskmenu-theme="light"] .task-connections-panel{background:#fff;border-color:#b9c0c8;box-shadow:10px 0 28px rgba(0,0,0,.12)}
html[data-taskmenu-theme="light"] body:not(.task-sidebar-auto-hide) .task-connections-panel{box-shadow:none}
html[data-taskmenu-theme="light"] .task-connection-row:hover{background:#eef2f6}
html[data-taskmenu-theme="light"] .task-connection-dialog-card{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .task-connection-field input,
html[data-taskmenu-theme="light"] .task-connection-field select,
html[data-taskmenu-theme="light"] .task-connection-field textarea{background:#f7f9fb;border-color:#b9c0c8}
`;
document.head.append(style);

function normalizeLocalSettings(raw){
  const dirs=[];const seen=new Set();
  for(const value of Array.isArray(raw?.custom_dirs)?raw.custom_dirs:[]){
    const dir=String(value||'').trim();
    if(!dir||dir==='.'||seen.has(dir))continue;
    seen.add(dir);dirs.push(dir);
  }
  const selected=String(raw?.selected_cwd||'.').trim()||'.';
  return {selected_cwd:selected,custom_dirs:dirs};
}

function authLabel(profile){
  if(profile.auth_method==='private_key')return profile.has_secret?'key + passphrase':'private key';
  if(profile.auth_method==='password')return 'password';
  return 'ssh-agent';
}

async function loadData(){
  const [local,ssh,adapters,databases]=await Promise.all([
    app.jsonFetch('/api/config/terminal-cwds'),
    app.jsonFetch('/api/ssh/profiles'),
    app.jsonFetch('/api/db/adapters'),
    app.jsonFetch('/api/db/profiles')
  ]);
  localSettings=normalizeLocalSettings(local);
  sshProfiles=Array.isArray(ssh?.profiles)?ssh.profiles:[];
  dbAdapters=Array.isArray(adapters?.adapters)?adapters.adapters:[];
  dbProfiles=Array.isArray(databases?.profiles)?databases.profiles:[];
  render();
}

async function startTerminal(payload){
  const meta=await app.jsonFetch('/api/sessions',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({kind:'terminal',...payload})
  });
  app.materializeSession(meta,true);
  return meta;
}

async function openLocal(cwd){
  await startTerminal({cwd});
}

async function openSSH(profile){
  await startTerminal({ssh_profile_id:profile.id});
}

async function testSSH(profile,button){
  if(button)button.disabled=true;
  try{
    const result=await app.jsonFetch('/api/ssh/test',{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({profile_id:profile.id})
    });
    if(!result?.ok)throw new Error(result?.message||'SSH connection test failed');
    if(button){
      const old=button.textContent;
      button.textContent='✓';
      button.title='SSH connection succeeded'+(Number.isFinite(result.elapsed_ms)?' · '+result.elapsed_ms+' ms':'');
      setTimeout(()=>{if(button.isConnected){button.textContent=old;button.title='Test connection';}},1400);
    }
  }finally{
    if(button?.isConnected)button.disabled=false;
  }
}

async function openDatabase(profile){
  const api=globalThis.TaskMenuDatabase;
  if(typeof api?.openProfile!=='function')throw new Error('Database workspace is unavailable');
  await api.openProfile(profile);
}

async function testDatabase(profile,button){
  const api=globalThis.TaskMenuDatabase;
  if(typeof api?.testProfile!=='function')throw new Error('Database workspace is unavailable');
  if(button)button.disabled=true;
  try{
    await api.testProfile(profile);
    if(button){
      const old=button.textContent;
      button.textContent='✓';
      button.title='Database connection succeeded';
      setTimeout(()=>{if(button.isConnected){button.textContent=old;button.title='Test connection';}},1400);
    }
  }finally{
    if(button?.isConnected)button.disabled=false;
  }
}

function databaseAdapter(profile){
  return dbAdapters.find(adapter=>adapter.id===profile?.adapter_id)||null;
}

function databaseDefaultPort(kind){
  if(kind==='redis')return 6379;
  if(kind==='mongo')return 27017;
  return 3306;
}

function databaseEndpoint(profile){
  const host=profile?.host||'127.0.0.1';
  const kind=dbAdapters.find(adapter=>adapter.id===profile?.adapter_id)?.kind||'';
  const port=Number(profile?.port)||databaseDefaultPort(kind);
  const database=profile?.database?' / '+profile.database:'';
  const mode=profile?.read_only?'read-only':'read/write';
  const transport=profile?.transport==='ssh_tunnel'?' · SSH tunnel':'';
  return host+':'+port+database+' · '+mode+transport;
}

async function saveLocalSettings(next){
  const saved=await app.jsonFetch('/api/config/terminal-cwds',{
    method:'PUT',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify(next)
  });
  localSettings=normalizeLocalSettings(saved);
  window.dispatchEvent(new CustomEvent('taskmenu:terminal-cwds-changed',{detail:{settings:localSettings}}));
  render();
}

async function addLocalDirectory(){
  const chooser=globalThis.TaskMenuDirectoryBrowser?.choose;
  if(typeof chooser!=='function')throw new Error('Workspace directory browser is unavailable');
  const value=await chooser({
    title:'Add local terminal directory',
    label:'Directory relative to the workspace:',
    confirm:'Add directory',
    initial:'.'
  });
  if(value===null)return;
  const dir=String(value||'').trim();
  if(!dir)return;
  const dirs=[...localSettings.custom_dirs];
  if(dir!=='.'&&!dirs.includes(dir))dirs.push(dir);
  await saveLocalSettings({selected_cwd:localSettings.selected_cwd||'.',custom_dirs:dirs});
}

async function removeLocalDirectory(dir){
  const dirs=localSettings.custom_dirs.filter(value=>value!==dir);
  const selected=localSettings.selected_cwd===dir?'.':localSettings.selected_cwd;
  await saveLocalSettings({selected_cwd:selected||'.',custom_dirs:dirs});
}

function profilePresetText(profile){
  return (Array.isArray(profile?.preset_commands)?profile.preset_commands:[])
    .map(item=>String(item?.command||'').trim())
    .filter(Boolean)
    .join('\n');
}

function presetCommandsFromText(text){
  return String(text||'').split(/\r?\n/)
    .map(value=>value.trim())
    .filter(Boolean)
    .slice(0,32)
    .map((command,index)=>({id:'preset-'+(index+1),name:'Preset '+(index+1),command}));
}

function field(form,labelText,name,{type='text',value='',wide=false,placeholder='',options=null}={}){
  const wrap=document.createElement('div');wrap.className='task-connection-field'+(wide?' wide':'');
  const label=document.createElement('label');label.textContent=labelText;
  let input;
  if(options){
    input=document.createElement('select');
    for(const [optionValue,optionLabel] of options){
      const option=document.createElement('option');option.value=optionValue;option.textContent=optionLabel;input.append(option);
    }
  }else if(type==='textarea'){
    input=document.createElement('textarea');
  }else{
    input=document.createElement('input');input.type=type;
  }
  input.name=name;input.value=value??'';if(placeholder)input.placeholder=placeholder;
  wrap.append(label,input);form.append(wrap);
  return {wrap,input};
}

function checkboxField(form,labelText,name,checked=false,{wide=false}={}){
  const wrap=document.createElement('div');wrap.className='task-connection-field'+(wide?' wide':'');
  const label=document.createElement('label');label.style.display='flex';label.style.alignItems='center';label.style.gap='7px';label.style.opacity='1';
  const input=document.createElement('input');input.type='checkbox';input.name=name;input.checked=Boolean(checked);input.style.width='auto';
  const text=document.createElement('span');text.textContent=labelText;label.append(input,text);wrap.append(label);form.append(wrap);
  return {wrap,input};
}

function closeDialog(dialog){
  try{dialog.remove();}catch{}
}

function openProfileDialog(profile=null){
  const editing=Boolean(profile?.id);
  const dialog=document.createElement('div');dialog.className='task-connection-dialog';
  const card=document.createElement('div');card.className='task-connection-dialog-card';
  const title=document.createElement('h3');title.textContent=editing?'Edit SSH profile':'Add SSH profile';
  const form=document.createElement('form');form.className='task-connection-form';

  const name=field(form,'Name','name',{value:profile?.name||''});
  const host=field(form,'Host','host',{value:profile?.host||''});
  const port=field(form,'Port','port',{type:'number',value:String(profile?.port||22)});
  const username=field(form,'Username','username',{value:profile?.username||''});
  const auth=field(form,'Authentication','auth_method',{
    value:profile?.auth_method||'agent',
    options:[['agent','SSH agent'],['private_key','Private key'],['password','Password']]
  });
  auth.input.value=profile?.auth_method||'agent';
  const identity=field(form,'Identity file','identity_file',{wide:true,value:profile?.identity_file||'',placeholder:'~/.ssh/id_ed25519 or absolute path'});
  const secret=field(form,editing&&profile?.has_secret?'Password / passphrase (leave blank to keep saved value)':'Password / passphrase','secret',{type:'password',wide:true});
  const home=field(form,'Custom remote home directory','custom_home_dir',{wide:true,value:profile?.custom_home_dir||'',placeholder:'Optional, e.g. /srv/app'});
  const proxy=field(form,'ProxyJump','proxy_jump',{wide:true,value:profile?.proxy_jump||'',placeholder:'Optional, e.g. jump@example.com'});
  const presets=field(form,'Preset commands — one command per line, run after connection','preset_commands',{type:'textarea',wide:true,value:profilePresetText(profile)});

  function syncAuthFields(){
    const method=auth.input.value;
    identity.wrap.style.display=method==='private_key'?'flex':'none';
    secret.wrap.style.display=method==='agent'?'none':'flex';
  }
  auth.input.addEventListener('change',syncAuthFields);syncAuthFields();

  const actions=document.createElement('div');actions.className='task-connection-dialog-actions';
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=()=>closeDialog(dialog);
  const save=document.createElement('button');save.type='submit';save.className='task-connection-primary';save.textContent=editing?'Save':'Add profile';
  actions.append(cancel,save);form.append(actions);
  card.append(title,form);dialog.append(card);document.body.append(dialog);
  name.input.focus();

  dialog.addEventListener('pointerdown',event=>{if(event.target===dialog)closeDialog(dialog);});
  dialog.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();closeDialog(dialog);}});
  form.onsubmit=async event=>{
    event.preventDefault();save.disabled=true;
    try{
      const payload={
        name:name.input.value,
        host:host.input.value,
        port:Number(port.input.value)||22,
        username:username.input.value,
        auth_method:auth.input.value,
        identity_file:identity.input.value,
        custom_home_dir:home.input.value,
        proxy_jump:proxy.input.value,
        preset_commands:presetCommandsFromText(presets.input.value)
      };
      if(auth.input.value!=='agent'&&secret.input.value!=='')payload.secret=secret.input.value;
      await app.jsonFetch(editing?'/api/ssh/profiles/'+encodeURIComponent(profile.id):'/api/ssh/profiles',{
        method:editing?'PUT':'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify(payload)
      });
      closeDialog(dialog);await loadData();
    }catch(error){app.showError(error);}
    finally{if(save.isConnected)save.disabled=false;}
  };
}

async function deleteProfile(profile){
  if(!confirm('Delete SSH profile "'+profile.name+'"?'))return;
  await app.jsonFetch('/api/ssh/profiles/'+encodeURIComponent(profile.id),{method:'DELETE'});
  await loadData();
}

function openDatabaseProfileDialog(profile=null){
  const editing=Boolean(profile?.id);
  if(!editing&&!dbAdapters.length)throw new Error('No database adapter is available on this server');
  const dialog=document.createElement('div');dialog.className='task-connection-dialog';
  const card=document.createElement('div');card.className='task-connection-dialog-card';
  const title=document.createElement('h3');title.textContent=editing?'Edit database profile':'Add database profile';
  const form=document.createElement('form');form.className='task-connection-form';

  const adapterOptions=dbAdapters.map(adapter=>[adapter.id,adapter.name||adapter.id]);
  if(profile?.adapter_id&&!adapterOptions.some(option=>option[0]===profile.adapter_id))adapterOptions.unshift([profile.adapter_id,profile.adapter_id+' (unavailable)']);
  const adapter=field(form,'Adapter','adapter_id',{value:profile?.adapter_id||dbAdapters[0]?.id||'',options:adapterOptions});
  adapter.input.value=profile?.adapter_id||dbAdapters[0]?.id||'';
  const name=field(form,'Name','name',{value:profile?.name||''});
  const transport=field(form,'Transport','transport',{
    value:profile?.transport||'direct',
    options:[['direct','Direct'],['ssh_tunnel','SSH tunnel']]
  });
  transport.input.value=profile?.transport||'direct';
  const sshOptions=sshProfiles.map(item=>[item.id,item.name||((item.username?item.username+'@':'')+item.host)]);
  if(profile?.ssh_profile_id&&!sshOptions.some(option=>option[0]===profile.ssh_profile_id))sshOptions.unshift([profile.ssh_profile_id,profile.ssh_profile_id+' (unavailable)']);
  if(!sshOptions.length)sshOptions.push(['','No SSH profiles available']);
  const sshProfile=field(form,'SSH profile','ssh_profile_id',{wide:true,value:profile?.ssh_profile_id||'',options:sshOptions});
  sshProfile.input.value=profile?.ssh_profile_id||'';
  const host=field(form,'Host','host',{value:profile?.host||'127.0.0.1'});
  const port=field(form,'Port','port',{type:'number',value:String(profile?.port||3306)});
  const username=field(form,'Username','username',{value:profile?.username||''});
  const database=field(form,'Database','database',{value:profile?.database||''});
  const secret=field(form,editing&&profile?.has_secret?'Password (leave blank to keep saved value)':'Password','secret',{type:'password',wide:true});
  const readOnly=checkboxField(form,'Read-only connection','read_only',editing?Boolean(profile?.read_only):true,{wide:true});
  const clearSecret=editing&&profile?.has_secret?checkboxField(form,'Clear saved password','clear_secret',false,{wide:true}):null;
  const charset=field(form,'Charset','charset',{value:profile?.options?.charset||'utf8mb4'});
  const timeout=field(form,'Connect timeout (seconds)','connect_timeout_seconds',{type:'number',value:profile?.options?.connect_timeout_seconds||'10'});
  const commandTimeout=field(form,'Command timeout (seconds)','command_timeout_seconds',{type:'number',value:profile?.options?.command_timeout_seconds||'15'});
  const authSource=field(form,'Authentication database','auth_source',{value:profile?.options?.auth_source||'',placeholder:'Optional, e.g. admin'});
  const mongoTLS=checkboxField(form,'Use TLS','tls',String(profile?.options?.tls||'').toLowerCase()==='true',{wide:true});

  function selectedDatabaseAdapter(){return dbAdapters.find(item=>item.id===adapter.input.value)||null;}
  function syncDatabaseAdapter(){
    const kind=selectedDatabaseAdapter()?.kind||'';
    const redis=kind==='redis';
    const mongo=kind==='mongo';
    charset.wrap.style.display=kind==='mysql'?'flex':'none';
    commandTimeout.wrap.style.display=redis?'flex':'none';
    authSource.wrap.style.display=mongo?'flex':'none';
    mongoTLS.wrap.style.display=mongo?'flex':'none';
    const databaseLabel=database.wrap.querySelector('label');
    if(databaseLabel)databaseLabel.textContent=redis?'Database index':'Database';
    if(!editing){
      const currentPort=Number(port.input.value)||0;
      const defaults=[3306,6379,27017];
      if(currentPort===0||defaults.includes(currentPort))port.input.value=String(databaseDefaultPort(kind));
    }
  }
  adapter.input.addEventListener('change',syncDatabaseAdapter);syncDatabaseAdapter();

  function syncDatabaseTransport(){
    const tunneled=transport.input.value==='ssh_tunnel';
    sshProfile.wrap.style.display=tunneled?'flex':'none';
    const hostLabel=host.wrap.querySelector('label');
    const portLabel=port.wrap.querySelector('label');
    if(hostLabel)hostLabel.textContent=tunneled?'Remote DB host':'Host';
    if(portLabel)portLabel.textContent=tunneled?'Remote DB port':'Port';
  }
  transport.input.addEventListener('change',syncDatabaseTransport);syncDatabaseTransport();

  const actions=document.createElement('div');actions.className='task-connection-dialog-actions';
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=()=>closeDialog(dialog);
  const save=document.createElement('button');save.type='submit';save.className='task-connection-primary';save.textContent=editing?'Save':'Add profile';
  actions.append(cancel,save);form.append(actions);
  card.append(title,form);dialog.append(card);document.body.append(dialog);
  name.input.focus();

  dialog.addEventListener('pointerdown',event=>{if(event.target===dialog)closeDialog(dialog);});
  dialog.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();closeDialog(dialog);}});
  form.onsubmit=async event=>{
    event.preventDefault();save.disabled=true;
    try{
      const options={};
      const adapterKind=selectedDatabaseAdapter()?.kind||'';
      if(adapterKind==='mysql'&&charset.input.value.trim())options.charset=charset.input.value.trim();
      if(timeout.input.value.trim())options.connect_timeout_seconds=timeout.input.value.trim();
      if(adapterKind==='redis'&&commandTimeout.input.value.trim())options.command_timeout_seconds=commandTimeout.input.value.trim();
      if(adapterKind==='mongo'){
        if(authSource.input.value.trim())options.auth_source=authSource.input.value.trim();
        if(mongoTLS.input.checked)options.tls='true';
      }
      const payload={
        name:name.input.value,
        adapter_id:adapter.input.value,
        transport:transport.input.value,
        host:host.input.value,
        port:Number(port.input.value)||databaseDefaultPort(adapterKind),
        username:username.input.value,
        database:database.input.value,
        read_only:readOnly.input.checked,
        options
      };
      if(transport.input.value==='ssh_tunnel'){
        if(!sshProfile.input.value)throw new Error('Select an SSH profile for the database tunnel');
        payload.ssh_profile_id=sshProfile.input.value;
      }
      if(secret.input.value!=='')payload.secret=secret.input.value;
      else if(clearSecret?.input.checked)payload.secret='';
      await app.jsonFetch(editing?'/api/db/profiles/'+encodeURIComponent(profile.id):'/api/db/profiles',{
        method:editing?'PUT':'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify(payload)
      });
      closeDialog(dialog);
      await loadData();
      try{await globalThis.TaskMenuDatabase?.refreshProfiles?.();}catch{}
    }catch(error){app.showError(error);}
    finally{if(save.isConnected)save.disabled=false;}
  };
}

async function deleteDatabaseProfile(profile){
  if(!confirm('Delete database profile "'+profile.name+'"?'))return;
  await app.jsonFetch('/api/db/profiles/'+encodeURIComponent(profile.id),{method:'DELETE'});
  await loadData();
  try{await globalThis.TaskMenuDatabase?.refreshProfiles?.();}catch{}
}

function section(titleText,onAdd){
  const box=document.createElement('div');box.className='task-connection-section';
  const head=document.createElement('div');head.className='task-connection-section-head';
  const title=document.createElement('div');title.className='task-connection-section-title';title.textContent=titleText;
  head.append(title);
  if(onAdd){
    const add=document.createElement('button');add.type='button';add.className='task-connection-add';add.textContent='＋';add.title='Add '+titleText.toLowerCase();
    add.onclick=()=>Promise.resolve(onAdd()).catch(app.showError);head.append(add);
  }
  box.append(head);return box;
}

function connectionRow(nameText,metaText,onOpen,{onTest=null,onEdit=null,onDelete=null}={}){
  const row=document.createElement('div');row.className='task-connection-row';
  const open=document.createElement('button');open.type='button';open.className='task-connection-open';
  const name=document.createElement('span');name.className='task-connection-name';name.textContent=nameText;
  const meta=document.createElement('span');meta.className='task-connection-meta';meta.textContent=metaText;
  open.append(name,meta);open.onclick=()=>Promise.resolve(onOpen()).catch(app.showError);
  const actions=document.createElement('div');actions.className='task-connection-actions';
  if(onTest){const test=document.createElement('button');test.type='button';test.textContent='Test';test.title='Test connection';test.onclick=event=>{event.stopPropagation();Promise.resolve(onTest(test)).catch(app.showError);};actions.append(test);}
  if(onEdit){const edit=document.createElement('button');edit.type='button';edit.textContent='Edit';edit.onclick=event=>{event.stopPropagation();onEdit();};actions.append(edit);}
  if(onDelete){const del=document.createElement('button');del.type='button';del.textContent='×';del.title='Delete';del.onclick=event=>{event.stopPropagation();Promise.resolve(onDelete()).catch(app.showError);};actions.append(del);}
  row.append(open,actions);return row;
}

function render(){
  if(!content)return;
  content.replaceChildren();

  const local=section('LOCAL',addLocalDirectory);
  local.append(connectionRow('Project root',app.taskData?.workspace||'.',()=>openLocal('.')));
  for(const dir of localSettings.custom_dirs){
    local.append(connectionRow(dir,'Workspace directory',()=>openLocal(dir),{onDelete:()=>removeLocalDirectory(dir)}));
  }
  content.append(local);

  const ssh=section('SSH',()=>openProfileDialog());
  if(!sshProfiles.length){
    const empty=document.createElement('div');empty.className='task-connection-empty';empty.textContent='No SSH profiles yet';ssh.append(empty);
  }else{
    for(const profile of sshProfiles){
      const endpoint=(profile.username?profile.username+'@':'')+profile.host+':'+profile.port+' · '+authLabel(profile);
      ssh.append(connectionRow(profile.name,endpoint,()=>openSSH(profile),{
        onTest:button=>testSSH(profile,button),
        onEdit:()=>openProfileDialog(profile),
        onDelete:()=>deleteProfile(profile)
      }));
    }
  }
  content.append(ssh);

  const databases=section('DATABASES',dbAdapters.length?()=>openDatabaseProfileDialog():null);
  if(!dbProfiles.length){
    const empty=document.createElement('div');empty.className='task-connection-empty';
    empty.textContent=dbAdapters.length?'No database profiles yet':'No database adapter available';
    databases.append(empty);
  }else{
    for(const profile of dbProfiles){
      const adapter=databaseAdapter(profile);
      const adapterName=adapter?.name||profile.adapter_id||'database';
      databases.append(connectionRow(profile.name,databaseEndpoint(profile),()=>openDatabase(profile),{
        onTest:button=>testDatabase(profile,button),
        onEdit:()=>openDatabaseProfileDialog(profile),
        onDelete:()=>deleteDatabaseProfile(profile)
      }));
      const last=databases.lastElementChild?.querySelector('.task-connection-meta');
      if(last)last.title=adapterName;
    }
  }
  content.append(databases);
}

function install(){
  if(panel?.isConnected)return true;
  if(app.layoutProfile==='mobile'||!app.taskData?.workspace)return false;
  panel=document.createElement('div');panel.className='task-connections-panel';
  const head=document.createElement('div');head.className='task-connections-head';
  const title=document.createElement('div');title.className='task-connections-title';title.textContent='CONNECTIONS';
  const refresh=document.createElement('button');refresh.type='button';refresh.className='task-connections-refresh';refresh.textContent='↻';refresh.title='Refresh connections';
  const close=document.createElement('button');close.type='button';close.className='task-connections-close';close.textContent='×';close.title='Close Connections';
  content=document.createElement('div');content.className='task-connections-content';
  refresh.onclick=()=>loadData().catch(app.showError);
  close.onclick=()=>{panel.classList.remove('visible');window.dispatchEvent(new CustomEvent('taskmenu:connections-visible',{detail:{visible:false}}));};
  head.append(title,refresh,close);panel.append(head,content);document.body.append(panel);
  loadData().catch(app.showError);
  return true;
}

function open(){
  if(!install())return;
  panel.classList.add('visible');
  loadData().catch(app.showError);
  window.dispatchEvent(new CustomEvent('taskmenu:connections-visible',{detail:{visible:true}}));
}
function close(){
  if(!panel)return;
  panel.classList.remove('visible');
}
function visible(){return Boolean(panel?.classList.contains('visible'));}

globalThis.TaskMenuConnections={open,close,refresh:loadData,get panel(){return panel;},get visible(){return visible();}};

if(app.taskData?.workspace)install();
else window.addEventListener('taskmenu:tasks',install,{once:true});
