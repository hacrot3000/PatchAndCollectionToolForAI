const app=globalThis.TaskMenuApp;
const database=globalThis.TaskMenuDatabase;
const connections=globalThis.TaskMenuConnections;
const transfers=globalThis.TaskMenuFileTransfer;
if(!app||!database||!connections)throw new Error('Connections unavailable for tunnel graph');

const style=document.createElement('style');
style.textContent=`
.connection-graph-backdrop{display:none;position:fixed;inset:0;z-index:17680;background:rgba(0,0,0,.5);align-items:flex-start;justify-content:center;padding:5vh 16px}
.connection-graph-backdrop.visible{display:flex}
.connection-graph-dialog{width:min(1120px,96vw);max-height:88vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 60px rgba(0,0,0,.58);overflow:hidden}
.connection-graph-head{display:flex;align-items:center;gap:7px;padding:9px 11px;border-bottom:1px solid #303843}.connection-graph-head strong{flex:1}
.connection-graph-body{display:grid;grid-template-columns:minmax(260px,.7fr) minmax(500px,1.3fr);min-height:320px;max-height:70vh}
.connection-graph-list{overflow:auto;border-right:1px solid #303843;padding:6px}.connection-graph-detail{overflow:auto;padding:10px}
.connection-graph-item{display:block;width:100%;text-align:left;padding:7px 8px;border:1px solid transparent;background:transparent}.connection-graph-item:hover,.connection-graph-item.active{background:#28313e;border-color:#415168}.connection-graph-item strong,.connection-graph-item span{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.connection-graph-item span{font-size:10px;opacity:.62;margin-top:2px}
.connection-graph-route{display:flex;gap:7px;align-items:stretch;overflow:auto;padding:8px 0 12px}.connection-graph-node{min-width:145px;max-width:220px;padding:9px;border:1px solid #3d4653;border-radius:8px;background:#10151c}.connection-graph-node strong{display:block;font-size:11px}.connection-graph-node span{display:block;font-size:9px;opacity:.65;margin-top:4px;overflow-wrap:anywhere}.connection-graph-arrow{display:grid;place-items:center;font-size:18px;opacity:.6}
.connection-graph-meta{display:grid;grid-template-columns:145px minmax(0,1fr);gap:5px 10px;margin-top:8px;font-size:10px}.connection-graph-meta>div:nth-child(odd){opacity:.58}.connection-graph-actions{display:flex;gap:6px;flex-wrap:wrap;margin-top:12px}.connection-graph-status{font-weight:700}.connection-graph-status.active{color:#83d69a}.connection-graph-status.inactive{color:#f0c66b}.connection-graph-error{margin-top:8px;color:#ff929d;white-space:pre-wrap;font:10px/1.4 ui-monospace,monospace}
.connection-graph-empty{padding:28px;text-align:center;opacity:.6}
html[data-taskmenu-theme="light"] .connection-graph-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .connection-graph-node{background:#f8fafc;border-color:#d0d7de}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='connection-graph-backdrop';
const dialog=document.createElement('div');dialog.className='connection-graph-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Connection tunneling graph');
const head=document.createElement('div');head.className='connection-graph-head';
const title=document.createElement('strong');title.textContent='CONNECTION / TUNNEL GRAPH';
const refreshButton=document.createElement('button');refreshButton.type='button';refreshButton.textContent='Refresh';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close';
head.append(title,refreshButton,closeButton);
const body=document.createElement('div');body.className='connection-graph-body';
const list=document.createElement('div');list.className='connection-graph-list';
const detail=document.createElement('div');detail.className='connection-graph-detail';
body.append(list,detail);dialog.append(head,body);backdrop.append(dialog);document.body.append(backdrop);

let entries=[];
let selectedKey='';
let pollTimer=0;
const checkState=new Map();

function endpoint(host,port){
  host=String(host||'').trim();const p=Number(port)||0;
  return host+(p?':'+p:'');
}
function sshProfiles(){return Array.isArray(connections.sshProfiles)?connections.sshProfiles:[];}
function sshProfileByID(id){return sshProfiles().find(profile=>String(profile?.id||'')===String(id||''))||null;}
function profileLabel(profile,fallback='Connection'){return String(profile?.name||profile?.label||profile?.host||fallback);}
function activeSSHProfileIDs(){
  const ids=new Set();
  for(const view of app.views?.values?.()||[]){
    const meta=view?.meta||{};
    const type=String(meta.target_type||meta.targetType||meta.kind||'').toLowerCase();
    const id=String(meta.target_profile_id||meta.targetProfileID||meta.profile_id||'').trim();
    if(id&&(type==='ssh'||String(meta.session_kind||'').toLowerCase()==='ssh'))ids.add(id);
  }
  return ids;
}
function databaseEntries(){
  const out=[];
  for(const view of database.views?.values?.()||[]){
    const meta=view?.meta||{},profile=view?.profile||database.getProfile?.(meta.profile_id)||{};
    const transport=String(meta.transport||profile.transport||'direct');
    const ssh=sshProfileByID(meta.ssh_profile_id||profile.ssh_profile_id);
    out.push({
      key:'db:'+String(meta.id),kind:'database',title:'DB · '+profileLabel(profile,meta.adapter_kind||'Database'),
      subtitle:[meta.adapter_kind,profile.database,transport].filter(Boolean).join(' · '),active:true,view,meta,profile,ssh
    });
  }
  return out;
}
function sshEntries(){
  const active=activeSSHProfileIDs();
  return sshProfiles().map(profile=>({
    key:'ssh:'+String(profile.id),kind:'ssh',title:'SSH · '+profileLabel(profile,'SSH'),
    subtitle:endpoint(profile.host||profile.hostname,profile.port||22),active:active.has(String(profile.id)),profile
  }));
}
function transferEntries(){
  const profiles=Array.isArray(transfers?.profiles)?transfers.profiles:[];
  const views=transfers?.views;
  return profiles.map(profile=>{
    const id=String(profile?.id||''),protocol=String(profile?.protocol||'sftp').toLowerCase();
    return {
      key:'transfer:'+id,kind:'transfer',title:protocol.toUpperCase()+' · '+profileLabel(profile,protocol.toUpperCase()),
      subtitle:endpoint(profile.host||profile.hostname,profile.port||((protocol==='sftp')?22:21)),
      active:Boolean(views?.has?.(id)),profile,protocol
    };
  });
}
function collectEntries(){
  const merged=[...databaseEntries(),...sshEntries(),...transferEntries()];
  const seen=new Set(),out=[];
  for(const item of merged){if(!item.key||seen.has(item.key))continue;seen.add(item.key);out.push(item);}
  out.sort((a,b)=>Number(b.active)-Number(a.active)||a.kind.localeCompare(b.kind)||a.title.localeCompare(b.title));
  return out;
}
function graphNodes(item){
  const nodes=[
    {title:'Browser',detail:'TaskDeck web UI'},
    {title:'TaskDeck',detail:'daemon / connection broker'}
  ];
  if(item.kind==='database'){
    const meta=item.meta||{},profile=item.profile||{};
    if(String(meta.transport||profile.transport)==='ssh_tunnel'){
      const ssh=item.ssh||{};
      nodes.push({title:'SSH host',detail:endpoint(ssh.host||ssh.hostname,ssh.port||22)||String(meta.ssh_profile_id||'SSH profile')});
      nodes.push({title:'Tunnel',detail:[
        endpoint(meta.tunnel_local_host,meta.tunnel_local_port),
        meta.tunnel_pid&&('PID '+meta.tunnel_pid)
      ].filter(Boolean).join(' · ')});
    }
    nodes.push({title:'Database',detail:[
      endpoint(meta.tunnel_remote_host||profile.host,meta.tunnel_remote_port||profile.port),
      profile.database
    ].filter(Boolean).join(' / ')});
  }else if(item.kind==='ssh'){
    const profile=item.profile||{};
    nodes.push({title:'SSH host',detail:endpoint(profile.host||profile.hostname,profile.port||22)});
  }else{
    const profile=item.profile||{},protocol=String(item.protocol||profile.protocol||'sftp').toUpperCase();
    nodes.push({title:protocol+' remote',detail:endpoint(profile.host||profile.hostname,profile.port||((item.protocol==='ftp')?21:22))});
  }
  return nodes;
}
function addMeta(grid,label,value){
  const key=document.createElement('div');key.textContent=label;
  const val=document.createElement('div');val.textContent=value===undefined||value===null||value===''?'—':String(value);
  grid.append(key,val);
}
function selected(){return entries.find(item=>item.key===selectedKey)||entries[0]||null;}
function renderDetail(){
  detail.replaceChildren();const item=selected();
  if(!item){const empty=document.createElement('div');empty.className='connection-graph-empty';empty.textContent='No SSH, database or file-transfer connection is available';detail.append(empty);return;}
  const heading=document.createElement('h3');heading.textContent=item.title;
  const state=document.createElement('div');state.className='connection-graph-status '+(item.active?'active':'inactive');
  const checked=checkState.get(item.key);
  state.textContent=checked?.status||(item.active?'ACTIVE':'SAVED / INACTIVE');
  detail.append(heading,state);
  const route=document.createElement('div');route.className='connection-graph-route';
  const nodes=graphNodes(item);
  nodes.forEach((node,index)=>{
    if(index){const arrow=document.createElement('div');arrow.className='connection-graph-arrow';arrow.textContent='→';route.append(arrow);}
    const card=document.createElement('div');card.className='connection-graph-node';
    const strong=document.createElement('strong');strong.textContent=node.title;
    const span=document.createElement('span');span.textContent=node.detail||'—';card.append(strong,span);route.append(card);
  });
  detail.append(route);
  const metaGrid=document.createElement('div');metaGrid.className='connection-graph-meta';
  addMeta(metaGrid,'Type',item.kind);
  addMeta(metaGrid,'Status',item.active?'active':'inactive');
  if(item.kind==='database'){
    addMeta(metaGrid,'DB session',item.meta?.id);
    addMeta(metaGrid,'Transport',item.meta?.transport||item.profile?.transport||'direct');
    addMeta(metaGrid,'SSH profile',item.meta?.ssh_profile_id||item.profile?.ssh_profile_id);
    addMeta(metaGrid,'Tunnel ID',item.meta?.tunnel_id);
    addMeta(metaGrid,'Tunnel PID',item.meta?.tunnel_pid);
    addMeta(metaGrid,'Tunnel local port',item.meta?.tunnel_local_port);
    addMeta(metaGrid,'Tunnel remote',endpoint(item.meta?.tunnel_remote_host,item.meta?.tunnel_remote_port));
    addMeta(metaGrid,'Tunnel started',item.meta?.tunnel_started_at);
    addMeta(metaGrid,'Database',item.profile?.database);
  }else if(item.kind==='ssh'){
    addMeta(metaGrid,'Profile ID',item.profile?.id);
    addMeta(metaGrid,'Host',endpoint(item.profile?.host||item.profile?.hostname,item.profile?.port||22));
    addMeta(metaGrid,'User',item.profile?.user||item.profile?.username);
  }else{
    addMeta(metaGrid,'Profile ID',item.profile?.id);
    addMeta(metaGrid,'Protocol',String(item.protocol||'').toUpperCase());
    addMeta(metaGrid,'Remote',endpoint(item.profile?.host||item.profile?.hostname,item.profile?.port||((item.protocol==='ftp')?21:22)));
  }
  detail.append(metaGrid);
  const actions=document.createElement('div');actions.className='connection-graph-actions';
  const check=document.createElement('button');check.type='button';check.textContent='Check';
  check.onclick=()=>checkConnection(item,check).catch(app.showError);actions.append(check);
  const action=document.createElement('button');action.type='button';
  action.textContent=item.kind==='database'?'Reconnect':(item.kind==='ssh'?'Open SSH':'Open '+String(item.protocol||'transfer').toUpperCase());
  action.onclick=()=>reconnectOrOpen(item,action).catch(app.showError);actions.append(action);
  detail.append(actions);
  if(checked?.error){const error=document.createElement('div');error.className='connection-graph-error';error.textContent=checked.error;detail.append(error);}
}
function renderList(){
  list.replaceChildren();
  if(!entries.length){const empty=document.createElement('div');empty.className='connection-graph-empty';empty.textContent='No connections';list.append(empty);return;}
  if(!entries.some(item=>item.key===selectedKey))selectedKey=entries[0].key;
  for(const item of entries){
    const button=document.createElement('button');button.type='button';button.className='connection-graph-item';button.classList.toggle('active',item.key===selectedKey);
    const strong=document.createElement('strong');strong.textContent=(item.active?'● ':'○ ')+item.title;
    const span=document.createElement('span');span.textContent=item.subtitle;button.append(strong,span);
    button.onclick=()=>{selectedKey=item.key;renderList();renderDetail();};list.append(button);
  }
}
async function refresh(){
  entries=collectEntries();renderList();renderDetail();
}
async function checkConnection(item,button){
  button.disabled=true;const started=performance.now();
  try{
    if(item.kind==='database'){
      if(!item.active)throw new Error('Database session is not active');
      await database.request(item.meta.id,'ping');
    }else if(item.kind==='ssh'){
      await connections.openSSHProfile?.(String(item.profile.id));
    }else{
      await transfers?.testProfile?.(String(item.profile.id));
    }
    checkState.set(item.key,{status:'OK · '+Math.round(performance.now()-started)+' ms',error:''});
  }catch(error){
    checkState.set(item.key,{status:'FAILED',error:String(error?.message||error)});
    throw error;
  }finally{button.disabled=false;renderDetail();}
}
async function reconnectOrOpen(item,button){
  button.disabled=true;
  try{
    if(item.kind==='database'){
      const reopened=await database.reconnect?.(item.view);
      if(reopened)selectedKey='db:'+String(reopened.meta?.id||'');
    }else if(item.kind==='ssh'){
      await connections.openSSHProfile?.(String(item.profile.id));
    }else{
      await transfers?.openProfile?.(String(item.profile.id));
    }
    await refresh();
  }finally{button.disabled=false;}
}
function open(){
  backdrop.classList.add('visible');refresh();
  clearInterval(pollTimer);pollTimer=setInterval(()=>{if(backdrop.classList.contains('visible'))refresh();},2000);
}
function close(){backdrop.classList.remove('visible');clearInterval(pollTimer);pollTimer=0;}

refreshButton.onclick=()=>refresh().catch(app.showError);closeButton.onclick=close;
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();});

globalThis.TaskMenuConnectionGraph={open,close,refresh,get entries(){return [...entries];},get visible(){return backdrop.classList.contains('visible');}};
