const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Connections');

let panel=null;
let content=null;
let localSettings={selected_cwd:'.',custom_dirs:[]};
let sshProfiles=[];
let dbAdapters=[];
let dbProfiles=[];
let fileTransferProfiles=[];
let connectionContextMenu=null;

const style=document.createElement('style');
style.textContent=`
.task-connections-panel{display:none;position:fixed;top:var(--taskmenu-header-height,30px);bottom:0;left:48px;z-index:1800;width:min(var(--taskmenu-sidebar-panel-width,310px),calc(100vw - 48px));background:#11151b;border-right:1px solid #3b414d;box-shadow:10px 0 28px rgba(0,0,0,.28);flex-direction:column}
.task-connections-panel.visible{display:flex}
.task-connections-head{height:42px;display:flex;align-items:center;gap:6px;padding:6px 8px;border-bottom:1px solid #30343b}
.task-connections-title{font-size:12px;font-weight:700;letter-spacing:.04em;flex:1}
.task-connections-refresh,.task-connections-close{padding:4px 7px;font-size:11px}
.task-connections-content{flex:1;min-height:0;overflow:auto;padding:7px}
.task-connection-section{margin-bottom:12px}
.task-connection-section-head{display:flex;align-items:center;gap:6px;margin-bottom:5px}
.task-connection-section-title{font-size:10px;font-weight:700;letter-spacing:.08em;opacity:.65;flex:1}
.task-connection-add{padding:3px 6px;font-size:10px}
.task-connection-row{display:block;padding:5px 5px;border-radius:5px}
.task-connection-row:hover{background:#1b2028}
.task-connection-open{display:block;width:100%;min-width:0;text-align:left;border:0;background:transparent;padding:3px;color:inherit;overflow:hidden}
.task-connection-open:hover{background:transparent}
.task-connection-name{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px}
.task-connection-meta{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:10px;opacity:.55;margin-top:2px}
.task-connection-context-menu{position:fixed;z-index:10030;min-width:150px;padding:4px;background:#171b22;border:1px solid #48515f;border-radius:7px;box-shadow:0 14px 38px rgba(0,0,0,.45)}
.task-connection-context-item{display:block;width:100%;text-align:left;border:0;background:transparent;color:inherit;padding:7px 9px;border-radius:4px;font-size:11px}
.task-connection-context-item:hover{background:#2b3440}
.task-connection-context-item.danger{color:#ff9a9a}
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
.task-connection-warning{grid-column:1/-1;padding:8px 9px;border:1px solid #7b6332;background:#302814;border-radius:6px;font-size:11px;line-height:1.35}
body:not(.task-sidebar-auto-hide) .task-connections-panel{top:calc(var(--taskmenu-header-height,30px) + 46px);left:0;width:var(--taskmenu-sidebar-inline-width,310px);box-shadow:none}
html[data-taskmenu-theme="light"] .task-connections-panel{background:#fff;border-color:#b9c0c8;box-shadow:10px 0 28px rgba(0,0,0,.12)}
html[data-taskmenu-theme="light"] body:not(.task-sidebar-auto-hide) .task-connections-panel{box-shadow:none}
html[data-taskmenu-theme="light"] .task-connection-row:hover{background:#eef2f6}
html[data-taskmenu-theme="light"] .task-connection-dialog-card{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .task-connection-context-menu{background:#fff;border-color:#b9c0c8}
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
  const [local,ssh,adapters,databases,transfers]=await Promise.all([
    app.jsonFetch('/api/config/terminal-cwds'),
    app.jsonFetch('/api/ssh/profiles'),
    app.jsonFetch('/api/db/adapters'),
    app.jsonFetch('/api/db/profiles'),
    app.jsonFetch('/api/file-transfer/profiles')
  ]);
  localSettings=normalizeLocalSettings(local);
  sshProfiles=Array.isArray(ssh?.profiles)?ssh.profiles:[];
  dbAdapters=Array.isArray(adapters?.adapters)?adapters.adapters:[];
  dbProfiles=Array.isArray(databases?.profiles)?databases.profiles:[];
  fileTransferProfiles=Array.isArray(transfers?.profiles)?transfers.profiles:[];
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

async function runSSHTest(payload,button){
  if(button)button.disabled=true;
  try{
    const result=await app.jsonFetch('/api/ssh/test',{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify(payload)
    });
    if(!result?.ok){
      if((result?.failure_code==='host_key_changed'||result?.failure_code==='host_key_verification')&&result?.host_key){
        openSSHHostKeyRecovery(payload,result);
        return result;
      }
      throw new Error(result?.message||'SSH connection test failed');
    }
    if(button){
      const old=button.textContent;
      button.textContent='✓';
      button.title='SSH connection succeeded'+(Number.isFinite(result.elapsed_ms)?' · '+result.elapsed_ms+' ms':'');
      setTimeout(()=>{if(button.isConnected){button.textContent=old;button.title='Test connection';}},1400);
    }
    return result;
  }finally{
    if(button?.isConnected)button.disabled=false;
  }
}

async function testSSH(profile,button){
  return runSSHTest({profile_id:profile.id},button);
}

function openSSHHostKeyRecovery(payload,result){
  const info=result?.host_key||{};
  const profileID=String(payload?.profile_id||'').trim();
  const dialog=document.createElement('div');dialog.className='task-connection-dialog';
  const card=document.createElement('div');card.className='task-connection-dialog-card';
  const title=document.createElement('h3');title.textContent=result?.failure_code==='host_key_changed'?'SSH host key changed':'SSH host key verification failed';
  const warning=document.createElement('div');warning.className='task-connection-warning';
  warning.textContent='TaskDeck will not trust a replacement host key automatically. Verify the host and fingerprint through a trusted channel before removing any saved key.';
  const details=document.createElement('div');details.className='task-connection-form';details.style.marginTop='10px';
  const row=(label,value)=>{
    const item=document.createElement('div');item.className='task-connection-field wide';
    const key=document.createElement('label');key.textContent=label;
    const text=document.createElement('div');text.style.fontFamily='ui-monospace,monospace';text.style.fontSize='11px';text.style.overflowWrap='anywhere';text.textContent=String(value||'—');
    item.append(key,text);details.append(item);
  };
  row('Host lookup',info.lookup);
  row('Candidate fingerprint from OpenSSH',info.candidate_fingerprint||'Not reported by OpenSSH');
  const matches=Array.isArray(info.matches)?info.matches:[];
  row('Matching known_hosts entries',matches.length?matches.map(item=>item.file+' · '+item.entries+' entr'+(item.entries===1?'y':'ies')).join('\n'):'No matching configured known_hosts entry found');
  const status=document.createElement('div');status.className='task-connection-warning';status.style.marginTop='10px';status.hidden=true;
  const actions=document.createElement('div');actions.className='task-connection-dialog-actions';
  const close=document.createElement('button');close.type='button';close.textContent='Close';close.onclick=()=>closeDialog(dialog);
  const remove=document.createElement('button');remove.type='button';remove.className='task-connection-danger';remove.textContent='Remove old known_hosts entry';
  remove.hidden=!profileID||info.can_remove!==true;
  remove.onclick=async()=>{
    const target=String(info.lookup||'this host');
    if(!confirm('Remove the existing known_hosts entry for '+target+'?\n\nOnly continue after you have independently verified that the server host key really changed. TaskDeck will NOT reconnect automatically.'))return;
    remove.disabled=true;close.disabled=true;
    try{
      const response=await app.jsonFetch('/api/ssh/host-key',{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({profile_id:profileID,action:'remove',confirmed:true})
      });
      if(!response?.ok)throw new Error(response?.error||'Could not remove old SSH host key');
      status.hidden=false;status.textContent=response.message||'Old known_hosts entry removed. Reconnect explicitly and verify the replacement key.';
      remove.hidden=true;
    }catch(error){app.showError(error);}
    finally{close.disabled=false;if(remove.isConnected)remove.disabled=false;}
  };
  actions.append(close,remove);card.append(title,warning,details,status,actions);dialog.append(card);document.body.append(dialog);
  dialog.addEventListener('pointerdown',event=>{if(event.target===dialog)closeDialog(dialog);});
  dialog.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();closeDialog(dialog);}});
  close.focus();
}

async function openFileTransfer(profile){
  const api=globalThis.TaskMenuFileTransfer;
  if(typeof api?.openProfile!=='function')throw new Error('File-transfer workspace is unavailable');
  await api.openProfile(profile);
}

async function runFileTransferTest(payload,button){
  const api=globalThis.TaskMenuFileTransfer;
  if(typeof api?.testProfile!=='function')throw new Error('File-transfer workspace is unavailable');
  if(button)button.disabled=true;
  try{
    const result=payload?.profile
      ?await api.testDraft(payload.profile_id||'',payload.profile)
      :await api.testProfile(payload.profile_id);
    if(!result?.ok)throw new Error(result?.message||'File-transfer connection test failed');
    if(button){
      const old=button.textContent;button.textContent='✓';
      button.title=(result.message||'Connection succeeded')+(Number.isFinite(result.elapsed_ms)?' · '+result.elapsed_ms+' ms':'');
      setTimeout(()=>{if(button.isConnected){button.textContent=old;button.title='Test connection';}},1400);
    }
    return result;
  }finally{if(button?.isConnected)button.disabled=false;}
}

async function testFileTransfer(profile,button){
  return runFileTransferTest({profile_id:profile.id},button);
}

async function testFileTransferDraft(profileID,profile,button){
  return runFileTransferTest({profile_id:profileID||'',profile},button);
}

function fileTransferEndpoint(profile){
  const initial=profile?.initial_path||'.';
  if(profile?.protocol==='sftp'){
    const ssh=sshProfiles.find(item=>item.id===profile.ssh_profile_id);
    return (ssh?.name||profile.ssh_profile_id||'SSH profile')+' · '+initial;
  }
  return ((profile?.username?profile.username+'@':'')+(profile?.host||'FTP')+':'+(Number(profile?.port)||21))+' · '+initial;
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

async function testDatabaseDraft(profileID,draft,button){
  const api=globalThis.TaskMenuDatabase;
  if(typeof api?.testDraft!=='function')throw new Error('Database workspace draft test is unavailable');
  if(button)button.disabled=true;
  try{
    const result=await api.testDraft(profileID,draft);
    if(button){
      const old=button.textContent;
      button.textContent='✓';
      button.title='Database connection succeeded'+(Number.isFinite(result?.elapsed_ms)?' · '+result.elapsed_ms+' ms':'');
      setTimeout(()=>{if(button.isConnected){button.textContent=old;button.title='Test connection';}},1400);
    }
    return result;
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
  if(kind==='sqlite')return 0;
  return 3306;
}

function databaseEndpoint(profile){
  const kind=dbAdapters.find(adapter=>adapter.id===profile?.adapter_id)?.kind||'';
  const mode=profile?.read_only?'read-only':'read/write';
  if(kind==='sqlite')return (profile?.file||'SQLite file')+' · '+mode;
  const host=profile?.host||'127.0.0.1';
  const port=Number(profile?.port)||databaseDefaultPort(kind);
  const database=profile?.database?' / '+profile.database:'';
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

function forwardingHostPort(host,port){
  host=String(host||'127.0.0.1').trim()||'127.0.0.1';
  port=Number(port)||0;
  return host.includes(':')?'['+host.replace(/^\[|\]$/g,'')+']:'+port:host+':'+port;
}

function profileForwardingsText(profile){
  return (Array.isArray(profile?.forwardings)?profile.forwardings:[]).map(item=>{
    const kind=String(item?.kind||'').toLowerCase();
    const listen=forwardingHostPort(item?.bind_host||'127.0.0.1',item?.bind_port);
    if(kind==='dynamic')return 'D '+listen;
    const prefix=kind==='remote'?'R':'L';
    return prefix+' '+listen+' '+forwardingHostPort(item?.target_host,item?.target_port);
  }).join('\n');
}

function parseForwardHostPort(value,label){
  value=String(value||'').trim();
  let host='',portText='';
  if(value.startsWith('[')){
    const end=value.indexOf(']:');
    if(end<0)throw new Error(label+' must use [IPv6]:port');
    host=value.slice(1,end);portText=value.slice(end+2);
  }else{
    const index=value.lastIndexOf(':');
    if(index<=0)throw new Error(label+' must use host:port');
    host=value.slice(0,index);portText=value.slice(index+1);
  }
  host=host.trim();
  const port=Number(portText);
  if(!host||!Number.isInteger(port)||port<1||port>65535)throw new Error(label+' has an invalid host or port');
  return {host,port};
}

function forwardingsFromText(text){
  const rows=[];
  for(const [index,raw] of String(text||'').split(/\r?\n/).entries()){
    const line=raw.trim();if(!line)continue;
    const parts=line.split(/\s+/);
    const kind=String(parts.shift()||'').toUpperCase();
    if(!['L','R','D'].includes(kind))throw new Error('SSH forwarding line '+(index+1)+' must start with L, R, or D');
    if(kind==='D'){
      if(parts.length!==1)throw new Error('SSH dynamic forwarding line '+(index+1)+' must be: D bind_host:port');
      const bind=parseForwardHostPort(parts[0],'SSH dynamic forwarding line '+(index+1));
      rows.push({kind:'dynamic',bind_host:bind.host,bind_port:bind.port});
      continue;
    }
    if(parts.length!==2)throw new Error('SSH forwarding line '+(index+1)+' must be: '+kind+' bind_host:port target_host:port');
    const bind=parseForwardHostPort(parts[0],'SSH forwarding line '+(index+1)+' bind');
    const target=parseForwardHostPort(parts[1],'SSH forwarding line '+(index+1)+' target');
    rows.push({kind:kind==='L'?'local':'remote',bind_host:bind.host,bind_port:bind.port,target_host:target.host,target_port:target.port});
  }
  if(rows.length>16)throw new Error('SSH profile supports at most 16 forwarding rules');
  return rows;
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

function closeConnectionContextMenu(){
  if(connectionContextMenu?.isConnected)connectionContextMenu.remove();
  connectionContextMenu=null;
}

function showConnectionContextMenu(items,x,y){
  closeConnectionContextMenu();
  const menu=document.createElement('div');menu.className='task-connection-context-menu';
  for(const item of items.filter(item=>item&&typeof item.action==='function')){
    const button=document.createElement('button');button.type='button';
    button.className='task-connection-context-item'+(item.danger?' danger':'');
    button.textContent=item.label;
    button.onclick=event=>{
      event.preventDefault();event.stopPropagation();closeConnectionContextMenu();
      Promise.resolve(item.action()).catch(app.showError);
    };
    menu.append(button);
  }
  if(!menu.childElementCount)return;
  document.body.append(menu);connectionContextMenu=menu;
  const rect=menu.getBoundingClientRect();
  menu.style.left=Math.max(4,Math.min(x,window.innerWidth-rect.width-4))+'px';
  menu.style.top=Math.max(4,Math.min(y,window.innerHeight-rect.height-4))+'px';
}

document.addEventListener('pointerdown',event=>{
  if(connectionContextMenu&&!connectionContextMenu.contains(event.target))closeConnectionContextMenu();
},true);
window.addEventListener('blur',closeConnectionContextMenu);
document.addEventListener('keydown',event=>{if(event.key==='Escape')closeConnectionContextMenu();},true);

function clonedProfile(profile){
  const clone={...profile,name:(profile?.name||'Connection')+' Copy'};
  delete clone.id;
  delete clone.has_secret;
  return clone;
}

async function testSSHFromMenu(profile){
  const result=await runSSHTest({profile_id:profile.id},null);
  if(!result?.ok)return;
  alert('SSH connection succeeded'+(Number.isFinite(result?.elapsed_ms)?' · '+result.elapsed_ms+' ms':''));
}

async function testDatabaseFromMenu(profile){
  await testDatabase(profile,null);
  alert('Database connection succeeded');
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
  const connectTimeout=field(form,'Connect timeout (seconds)','connect_timeout_seconds',{type:'number',value:String(profile?.connect_timeout_seconds||10)});
  connectTimeout.input.min='1';connectTimeout.input.max='300';
  const aliveInterval=field(form,'Server alive interval (seconds)','server_alive_interval_seconds',{type:'number',value:String(profile?.server_alive_interval_seconds||15)});
  aliveInterval.input.min='1';aliveInterval.input.max='3600';
  const aliveCount=field(form,'Server alive count max','server_alive_count_max',{type:'number',value:String(profile?.server_alive_count_max||3)});
  aliveCount.input.min='1';aliveCount.input.max='20';
  const forwards=field(form,'Port forwarding — one rule per line (L/R/D)','forwardings',{type:'textarea',wide:true,value:profileForwardingsText(profile),placeholder:'L 127.0.0.1:8080 app.internal:80\nR 127.0.0.1:9000 127.0.0.1:9001\nD 127.0.0.1:1080'});
  const forwardingHint=document.createElement('div');forwardingHint.className='task-connection-warning';
  forwardingHint.textContent='Forwarding rules apply only to interactive SSH terminals. SSH Test checks login only; SFTP and database tunnels do not inherit these L/R/D rules.';
  form.append(forwardingHint);
  const presets=field(form,'Preset commands — one command per line, run after connection','preset_commands',{type:'textarea',wide:true,value:profilePresetText(profile)});

  function syncAuthFields(){
    const method=auth.input.value;
    identity.wrap.style.display=method==='private_key'?'flex':'none';
    secret.wrap.style.display=method==='agent'?'none':'flex';
  }
  auth.input.addEventListener('change',syncAuthFields);syncAuthFields();

  function currentProfilePayload(){
    const payload={
      name:name.input.value,
      host:host.input.value,
      port:Number(port.input.value)||22,
      username:username.input.value,
      auth_method:auth.input.value,
      identity_file:identity.input.value,
      custom_home_dir:home.input.value,
      proxy_jump:proxy.input.value,
      connect_timeout_seconds:Number(connectTimeout.input.value)||10,
      server_alive_interval_seconds:Number(aliveInterval.input.value)||15,
      server_alive_count_max:Number(aliveCount.input.value)||3,
      forwardings:forwardingsFromText(forwards.input.value),
      preset_commands:presetCommandsFromText(presets.input.value)
    };
    if(auth.input.value!=='agent'&&secret.input.value!=='')payload.secret=secret.input.value;
    return payload;
  }

  const actions=document.createElement('div');actions.className='task-connection-dialog-actions';
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=()=>closeDialog(dialog);
  const test=document.createElement('button');
  test.type='button';test.textContent='Test';
  test.title=editing?'Test current edits without saving':'Test connection before adding this profile';
  test.onclick=()=>{
    const payload={profile:currentProfilePayload()};
    if(editing)payload.profile_id=profile.id;
    return runSSHTest(payload,test).catch(app.showError);
  };
  const save=document.createElement('button');save.type='submit';save.className='task-connection-primary';save.textContent=editing?'Save':'Add profile';
  actions.append(test,cancel,save);form.append(actions);
  card.append(title,form);dialog.append(card);document.body.append(dialog);
  name.input.focus();

  dialog.addEventListener('pointerdown',event=>{if(event.target===dialog)closeDialog(dialog);});
  dialog.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();closeDialog(dialog);}});
  form.onsubmit=async event=>{
    event.preventDefault();save.disabled=true;
    try{
      const payload=currentProfilePayload();
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

function openFileTransferProfileDialog(protocol,profile=null){
  protocol=String(profile?.protocol||protocol||'').toLowerCase();
  if(protocol!=='ftp'&&protocol!=='sftp')throw new Error('Unsupported file-transfer protocol');
  const editing=Boolean(profile?.id);
  const label=protocol.toUpperCase();
  const dialog=document.createElement('div');dialog.className='task-connection-dialog';
  const card=document.createElement('div');card.className='task-connection-dialog-card';
  const title=document.createElement('h3');title.textContent=(editing?'Edit ':'Add ')+label+' profile';
  const form=document.createElement('form');form.className='task-connection-form';

  const name=field(form,'Name','name',{value:profile?.name||''});
  let sshProfile=null,host=null,port=null,username=null,secret=null,clearSecret=null,timeout=null,ftpTLS=null,ftpWarning=null;
  if(protocol==='sftp'){
    const options=sshProfiles.map(item=>[item.id,item.name||((item.username?item.username+'@':'')+item.host)]);
    if(profile?.ssh_profile_id&&!options.some(option=>option[0]===profile.ssh_profile_id))options.unshift([profile.ssh_profile_id,profile.ssh_profile_id+' (unavailable)']);
    if(!options.length)options.push(['','No SSH profiles available']);
    sshProfile=field(form,'SSH profile','ssh_profile_id',{wide:true,value:profile?.ssh_profile_id||options[0]?.[0]||'',options});
    sshProfile.input.value=profile?.ssh_profile_id||options[0]?.[0]||'';
  }else{
    const initialTLS=profile?.ftp_tls_mode||'plain';
    host=field(form,'Host','host',{value:profile?.host||''});
    port=field(form,'Port','port',{type:'number',value:String(profile?.port||(initialTLS==='implicit'?990:21))});
    username=field(form,'Username','username',{value:profile?.username||''});
    ftpTLS=field(form,'FTP security','ftp_tls_mode',{
      wide:true,value:initialTLS,
      options:[['plain','Plain FTP'],['explicit','Explicit TLS / FTPES'],['implicit','Implicit FTPS']]
    });
    ftpTLS.input.value=initialTLS;
    secret=field(form,editing&&profile?.has_secret?'Password (leave blank to keep saved value)':'Password','secret',{type:'password',wide:true});
    timeout=field(form,'Connect timeout (seconds)','connect_timeout_seconds',{type:'number',value:String(profile?.connect_timeout_seconds||10)});
    clearSecret=editing&&profile?.has_secret?checkboxField(form,'Clear saved password','clear_secret',false,{wide:true}):null;
    ftpWarning=document.createElement('div');ftpWarning.className='task-connection-warning';
    const syncFTPSecurity=()=>{
      const mode=ftpTLS.input.value;
      const current=Number(port.input.value)||0;
      if(mode==='implicit'&&(current===0||current===21))port.input.value='990';
      if(mode!=='implicit'&&(current===0||current===990))port.input.value='21';
      ftpWarning.textContent=mode==='plain'
        ?'Plain FTP is not encrypted. Credentials and file contents can be observed on the network. Use Explicit TLS/FTPS or SFTP when available.'
        :mode==='explicit'
          ?'Explicit TLS / FTPES upgrades the control connection with AUTH TLS and protects file data with PROT P. Server certificate and hostname verification stay enabled.'
          :'Implicit FTPS starts TLS immediately (usually port 990) and protects file data with PROT P. Server certificate and hostname verification stay enabled.';
    };
    ftpTLS.input.addEventListener('change',syncFTPSecurity);syncFTPSecurity();
    form.append(ftpWarning);
  }
  const initial=field(form,'Initial remote path','initial_path',{wide:true,value:profile?.initial_path||'.',placeholder:'e.g. /srv/app or .'});

  function currentPayload(){
    if(protocol==='sftp'){
      return {name:name.input.value,protocol:'sftp',ssh_profile_id:sshProfile.input.value,initial_path:initial.input.value};
    }
    const tlsMode=ftpTLS.input.value||'plain';
    const payload={
      name:name.input.value,protocol:'ftp',host:host.input.value,
      port:Number(port.input.value)||(tlsMode==='implicit'?990:21),
      username:username.input.value,initial_path:initial.input.value,
      connect_timeout_seconds:Number(timeout.input.value)||10,
      ftp_tls_mode:tlsMode
    };
    if(secret.input.value)payload.secret=secret.input.value;
    if(clearSecret?.input.checked)payload.clear_secret=true;
    return payload;
  }

  const actions=document.createElement('div');actions.className='task-connection-dialog-actions';
  const test=document.createElement('button');test.type='button';test.textContent='Test';test.title='Test current values without saving';
  test.onclick=()=>testFileTransferDraft(editing?profile.id:'',currentPayload(),test).catch(app.showError);
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=()=>closeDialog(dialog);
  const save=document.createElement('button');save.type='submit';save.className='task-connection-primary';save.textContent=editing?'Save':'Add profile';
  actions.append(test,cancel,save);form.append(actions);
  card.append(title,form);dialog.append(card);document.body.append(dialog);name.input.focus();

  dialog.addEventListener('pointerdown',event=>{if(event.target===dialog)closeDialog(dialog);});
  dialog.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();closeDialog(dialog);}});
  form.onsubmit=async event=>{
    event.preventDefault();save.disabled=true;
    try{
      await app.jsonFetch(editing?'/api/file-transfer/profiles/'+encodeURIComponent(profile.id):'/api/file-transfer/profiles',{
        method:editing?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(currentPayload())
      });
      closeDialog(dialog);await loadData();
      try{await globalThis.TaskMenuFileTransfer?.refreshProfiles?.();}catch{}
    }catch(error){app.showError(error);}
    finally{if(save.isConnected)save.disabled=false;}
  };
}

async function deleteFileTransferProfile(profile){
  if(!confirm('Delete '+String(profile.protocol||'').toUpperCase()+' profile "'+profile.name+'"?'))return;
  await app.jsonFetch('/api/file-transfer/profiles/'+encodeURIComponent(profile.id),{method:'DELETE'});
  await loadData();
  try{await globalThis.TaskMenuFileTransfer?.refreshProfiles?.();}catch{}
}

async function testFileTransferFromMenu(profile){
  const result=await testFileTransfer(profile,null);
  alert(result?.message||String(profile.protocol||'').toUpperCase()+' connection succeeded');
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
  const sqliteFile=field(form,'SQLite database file','file',{wide:true,value:profile?.file||'',placeholder:'Absolute or workspace-relative existing .sqlite/.db file'});
  const secret=field(form,editing&&profile?.has_secret?'Password (leave blank to keep saved value)':'Password','secret',{type:'password',wide:true});
  const readOnly=checkboxField(form,'Read-only connection','read_only',profile?Boolean(profile?.read_only):true,{wide:true});
  const clearSecret=editing&&profile?.has_secret?checkboxField(form,'Clear saved password','clear_secret',false,{wide:true}):null;
  const charset=field(form,'Charset','charset',{value:profile?.options?.charset||'utf8mb4'});
  const timeout=field(form,'Connect timeout (seconds)','connect_timeout_seconds',{type:'number',value:profile?.options?.connect_timeout_seconds||'10'});
  const commandTimeout=field(form,'Command timeout (seconds)','command_timeout_seconds',{type:'number',value:profile?.options?.command_timeout_seconds||'15'});
  const authSource=field(form,'Authentication database','auth_source',{value:profile?.options?.auth_source||'',placeholder:'Optional, e.g. admin'});
  const mongoTLS=checkboxField(form,'Use TLS','tls',String(profile?.options?.tls||'').toLowerCase()==='true',{wide:true});
  const busyTimeout=field(form,'Busy timeout (ms)','busy_timeout_ms',{type:'number',value:profile?.options?.busy_timeout_ms||'5000'});

  function selectedDatabaseAdapter(){return dbAdapters.find(item=>item.id===adapter.input.value)||null;}
  function syncDatabaseAdapter(){
    const kind=selectedDatabaseAdapter()?.kind||'';
    const redis=kind==='redis';
    const mongo=kind==='mongo';
    const sqlite=kind==='sqlite';
    charset.wrap.style.display=kind==='mysql'?'flex':'none';
    commandTimeout.wrap.style.display=redis?'flex':'none';
    authSource.wrap.style.display=mongo?'flex':'none';
    mongoTLS.wrap.style.display=mongo?'flex':'none';
    sqliteFile.wrap.style.display=sqlite?'flex':'none';
    busyTimeout.wrap.style.display=sqlite?'flex':'none';
    timeout.wrap.style.display=sqlite?'none':'flex';
    transport.wrap.style.display=sqlite?'none':'flex';
    host.wrap.style.display=sqlite?'none':'flex';
    port.wrap.style.display=sqlite?'none':'flex';
    username.wrap.style.display=sqlite?'none':'flex';
    database.wrap.style.display=sqlite?'none':'flex';
    secret.wrap.style.display=sqlite?'none':'flex';
    if(clearSecret)clearSecret.wrap.style.display=sqlite?'none':'flex';
    if(sqlite)transport.input.value='direct';
    const databaseLabel=database.wrap.querySelector('label');
    if(databaseLabel)databaseLabel.textContent=redis?'Database index':'Database';
    if(!editing&&!sqlite){
      const currentPort=Number(port.input.value)||0;
      const defaults=[3306,6379,27017];
      if(currentPort===0||defaults.includes(currentPort))port.input.value=String(databaseDefaultPort(kind));
    }
    syncDatabaseTransport();
  }
  adapter.input.addEventListener('change',syncDatabaseAdapter);syncDatabaseAdapter();

  function syncDatabaseTransport(){
    const sqlite=selectedDatabaseAdapter()?.kind==='sqlite';
    const tunneled=!sqlite&&transport.input.value==='ssh_tunnel';
    sshProfile.wrap.style.display=tunneled?'flex':'none';
    const hostLabel=host.wrap.querySelector('label');
    const portLabel=port.wrap.querySelector('label');
    if(hostLabel)hostLabel.textContent=tunneled?'Remote DB host':'Host';
    if(portLabel)portLabel.textContent=tunneled?'Remote DB port':'Port';
  }
  transport.input.addEventListener('change',syncDatabaseTransport);syncDatabaseTransport();

  function currentDatabaseProfilePayload(){
    const options={};
    const adapterKind=selectedDatabaseAdapter()?.kind||'';
    if(adapterKind==='mysql'&&charset.input.value.trim())options.charset=charset.input.value.trim();
    if(adapterKind!=='sqlite'&&timeout.input.value.trim())options.connect_timeout_seconds=timeout.input.value.trim();
    if(adapterKind==='redis'&&commandTimeout.input.value.trim())options.command_timeout_seconds=commandTimeout.input.value.trim();
    if(adapterKind==='mongo'){
      if(authSource.input.value.trim())options.auth_source=authSource.input.value.trim();
      if(mongoTLS.input.checked)options.tls='true';
    }
    if(adapterKind==='sqlite'&&busyTimeout.input.value.trim())options.busy_timeout_ms=busyTimeout.input.value.trim();
    const sqlite=adapterKind==='sqlite';
    const payload={
      name:name.input.value,
      adapter_id:adapter.input.value,
      transport:sqlite?'direct':transport.input.value,
      host:sqlite?'':host.input.value,
      port:sqlite?0:(Number(port.input.value)||databaseDefaultPort(adapterKind)),
      username:sqlite?'':username.input.value,
      database:sqlite?'':database.input.value,
      file:sqlite?sqliteFile.input.value:'',
      read_only:readOnly.input.checked,
      options
    };
    if(adapterKind!=='sqlite'&&transport.input.value==='ssh_tunnel'){
      if(!sshProfile.input.value)throw new Error('Select an SSH profile for the database tunnel');
      payload.ssh_profile_id=sshProfile.input.value;
    }
    if(adapterKind!=='sqlite'){
      if(secret.input.value!=='')payload.secret=secret.input.value;
      else if(clearSecret?.input.checked)payload.secret='';
    }
    return payload;
  }

  const actions=document.createElement('div');actions.className='task-connection-dialog-actions';
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=()=>closeDialog(dialog);
  const test=editing?document.createElement('button'):null;
  if(test){
    test.type='button';test.textContent='Test';test.title='Test current edits without saving';
    test.onclick=()=>testDatabaseDraft(profile.id,currentDatabaseProfilePayload(),test).catch(app.showError);
  }
  const save=document.createElement('button');save.type='submit';save.className='task-connection-primary';save.textContent=editing?'Save':'Add profile';
  if(test)actions.append(test);
  actions.append(cancel,save);form.append(actions);
  card.append(title,form);dialog.append(card);document.body.append(dialog);
  name.input.focus();

  dialog.addEventListener('pointerdown',event=>{if(event.target===dialog)closeDialog(dialog);});
  dialog.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();closeDialog(dialog);}});
  form.onsubmit=async event=>{
    event.preventDefault();save.disabled=true;
    try{
      const payload=currentDatabaseProfilePayload();
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

function connectionRow(nameText,metaText,onOpen,{onClone=null,onTest=null,onEdit=null,onDelete=null}={}){
  const row=document.createElement('div');row.className='task-connection-row';
  const open=document.createElement('button');open.type='button';open.className='task-connection-open';
  const name=document.createElement('span');name.className='task-connection-name';name.textContent=nameText;
  const meta=document.createElement('span');meta.className='task-connection-meta';meta.textContent=metaText;
  open.append(name,meta);open.onclick=()=>Promise.resolve(onOpen()).catch(app.showError);
  const menuItems=[
    onClone?{label:'Clone',action:onClone}:null,
    onDelete?{label:'Delete',danger:true,action:onDelete}:null,
    onTest?{label:'Test',action:onTest}:null,
    onEdit?{label:'Edit',action:onEdit}:null
  ].filter(Boolean);
  if(menuItems.length){
    row.oncontextmenu=event=>{
      event.preventDefault();event.stopPropagation();
      showConnectionContextMenu(menuItems,event.clientX,event.clientY);
    };
  }
  row.append(open);return row;
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
        onClone:()=>openProfileDialog(clonedProfile(profile)),
        onDelete:()=>deleteProfile(profile),
        onTest:()=>testSSHFromMenu(profile),
        onEdit:()=>openProfileDialog(profile)
      }));
    }
  }
  content.append(ssh);

  for(const protocol of ['sftp','ftp']){
    const title=protocol.toUpperCase();
    const canAdd=protocol==='ftp'||sshProfiles.length>0;
    const sectionBox=section(title,canAdd?()=>openFileTransferProfileDialog(protocol):null);
    const profiles=fileTransferProfiles.filter(profile=>profile.protocol===protocol);
    if(!profiles.length){
      const empty=document.createElement('div');empty.className='task-connection-empty';
      empty.textContent=protocol==='sftp'&&!sshProfiles.length?'Create an SSH profile first':'No '+title+' profiles yet';
      sectionBox.append(empty);
    }else{
      for(const profile of profiles){
        sectionBox.append(connectionRow(profile.name,fileTransferEndpoint(profile),()=>openFileTransfer(profile),{
          onClone:()=>openFileTransferProfileDialog(protocol,clonedProfile(profile)),
          onDelete:()=>deleteFileTransferProfile(profile),
          onTest:()=>testFileTransferFromMenu(profile),
          onEdit:()=>openFileTransferProfileDialog(protocol,profile)
        }));
      }
    }
    content.append(sectionBox);
  }

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
        onClone:()=>openDatabaseProfileDialog(clonedProfile(profile)),
        onDelete:()=>deleteDatabaseProfile(profile),
        onTest:()=>testDatabaseFromMenu(profile),
        onEdit:()=>openDatabaseProfileDialog(profile)
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
  const graph=document.createElement('button');graph.type='button';graph.className='task-connections-graph';graph.textContent='Graph';graph.title='Open connection/tunnel graph';
  graph.onclick=()=>Promise.resolve(globalThis.TaskMenuConnectionGraph?.open?.()).catch(app.showError);
  const refresh=document.createElement('button');refresh.type='button';refresh.className='task-connections-refresh';refresh.textContent='↻';refresh.title='Refresh connections';
  const close=document.createElement('button');close.type='button';close.className='task-connections-close';close.textContent='×';close.title='Close Connections';
  content=document.createElement('div');content.className='task-connections-content';
  refresh.onclick=()=>loadData().catch(app.showError);
  close.onclick=()=>{panel.classList.remove('visible');window.dispatchEvent(new CustomEvent('taskmenu:connections-visible',{detail:{visible:false}}));};
  head.append(title,graph,refresh,close);panel.append(head,content);document.body.append(panel);
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

globalThis.TaskMenuConnections={
  open,close,refresh:loadData,
  async openLocal(cwd='.'){return openLocal(String(cwd||'.'));},
  async openSSHProfile(id){
    id=String(id||'').trim();if(!id)throw new Error('SSH profile id is required');
    await loadData();const profile=sshProfiles.find(item=>item.id===id);if(!profile)throw new Error('SSH profile not found: '+id);
    return openSSH(profile);
  },
  async testSSHProfile(id){
    id=String(id||'').trim();if(!id)throw new Error('SSH profile id is required');
    await loadData();const profile=sshProfiles.find(item=>item.id===id);if(!profile)throw new Error('SSH profile not found: '+id);
    return testSSH(profile,null);
  },
  get sshProfiles(){return [...sshProfiles];},
  get panel(){return panel;},get visible(){return visible();}
};

if(app.taskData?.workspace)install();
else window.addEventListener('taskmenu:tasks',install,{once:true});
