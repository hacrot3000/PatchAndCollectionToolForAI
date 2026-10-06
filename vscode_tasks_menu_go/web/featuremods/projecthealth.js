const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Project Health');

const style=document.createElement('style');
style.textContent=`
.project-health-backdrop{display:none;position:fixed;inset:0;z-index:17620;background:rgba(0,0,0,.5);align-items:flex-start;justify-content:center;padding:5vh 16px}
.project-health-backdrop.visible{display:flex}
.project-health-dialog{width:min(1080px,96vw);max-height:88vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 60px rgba(0,0,0,.58);overflow:hidden}
.project-health-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #303843}.project-health-head strong{flex:1}
.project-health-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:8px;padding:10px;overflow:auto}
.project-health-card{border:1px solid #303843;border-radius:8px;background:#10151c;padding:9px;min-width:0}
.project-health-card h3{margin:0 0 6px;font-size:10px;text-transform:uppercase;letter-spacing:.05em;opacity:.62}
.project-health-value{font-size:20px;font-weight:800;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.project-health-detail{margin-top:5px;font-size:10px;opacity:.66;white-space:pre-wrap;overflow-wrap:anywhere}
.project-health-card.warn .project-health-value{color:#f0c66b}.project-health-card.fail .project-health-value{color:#ff929d}.project-health-card.ok .project-health-value{color:#83d69a}
.project-health-card button{margin-top:7px;font-size:10px}
.project-health-footer{display:flex;align-items:center;gap:8px;padding:8px 10px;border-top:1px solid #303843;font-size:10px;opacity:.7}.project-health-footer span:first-child{flex:1}
html[data-taskmenu-theme="light"] .project-health-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .project-health-card{background:#f8fafc;border-color:#d0d7de}
@media(max-width:760px){.project-health-grid{grid-template-columns:1fr}}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='project-health-backdrop';
const dialog=document.createElement('div');dialog.className='project-health-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Project Health Dashboard');
const head=document.createElement('div');head.className='project-health-head';
const title=document.createElement('strong');title.textContent='PROJECT HEALTH';
const refreshButton=document.createElement('button');refreshButton.type='button';refreshButton.textContent='Refresh';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close';
head.append(title,refreshButton,closeButton);
const grid=document.createElement('div');grid.className='project-health-grid';
const footer=document.createElement('div');footer.className='project-health-footer';
const footerText=document.createElement('span');const footerTime=document.createElement('span');footer.append(footerText,footerTime);
dialog.append(head,grid,footer);backdrop.append(dialog);document.body.append(backdrop);

let snapshot=null;
let refreshing=false;
let pollTimer=0;

function fmtBytes(value){
  let n=Number(value)||0;const units=['B','KiB','MiB','GiB','TiB'];let i=0;
  while(n>=1024&&i<units.length-1){n/=1024;i++;}
  return (i? n.toFixed(n>=10?1:2):String(Math.round(n)))+' '+units[i];
}
function runningTaskViews(){
  const out=[];
  for(const [id,view] of app.views||[]){
    const meta=view?.meta||{};
    if(Number(meta.task_id)>0&&String(meta.status||'').toLowerCase()==='running')out.push({id,view,meta});
  }
  return out;
}
function openTerminals(){
  return [...(app.views?.values?.()||[])].filter(view=>Boolean(view?.term)&&!view?.closed);
}
function operationItems(){return globalThis.TaskMenuOperationCenter?.items||[];}
function activeConnectionEntries(){return globalThis.TaskMenuConnectionGraph?.entries||[];}
function buildLike(run){
  const text=[run?.label,run?.command_preview,run?.target_type].filter(Boolean).join(' ');
  return /\b(build|assemble|compile|package|gradle|webpack|vite|make|mvn|cargo)\b/i.test(text);
}
function runTimestamp(run){
  for(const value of [run?.ended_at,run?.started_at]){const n=Date.parse(String(value||''));if(Number.isFinite(n))return n;}
  return 0;
}
async function collect(){
  await Promise.allSettled([
    globalThis.TaskMenuOperationCenter?.refresh?.(),
    globalThis.TaskMenuConnectionGraph?.refresh?.()
  ]);
  const [gitResult,runsResult,diskResult]=await Promise.allSettled([
    app.jsonFetch('/api/git/status',{cache:'no-store'}),
    app.jsonFetch('/api/task-runs',{cache:'no-store'}),
    app.jsonFetch('/api/project/health',{cache:'no-store'})
  ]);
  const git=gitResult.status==='fulfilled'?gitResult.value:null;
  const taskRuns=runsResult.status==='fulfilled'&&Array.isArray(runsResult.value?.runs)?runsResult.value.runs:[];
  const disk=diskResult.status==='fulfilled'?diskResult.value:null;
  const runs=[...taskRuns].sort((a,b)=>runTimestamp(b)-runTimestamp(a));
  const lastBuild=runs.find(buildLike)||runs[0]||null;
  const lastBuildIsBuild=Boolean(lastBuild&&buildLike(lastBuild));
  const operations=operationItems();
  const failingTransfers=operations.filter(item=>item?.source==='transfer'&&item?.status==='failed');
  const dbViews=[...(globalThis.TaskMenuDatabase?.views?.values?.()||[])];
  const connections=activeConnectionEntries();
  const sshActive=connections.filter(item=>item?.kind==='ssh'&&item?.active).length;
  const sshSaved=connections.filter(item=>item?.kind==='ssh').length;
  return {
    git,taskRuns:runs,disk,lastBuild,lastBuildIsBuild,
    runningTasks:runningTaskViews(),
    terminals:openTerminals(),
    failingTransfers,
    dbViews,
    sshActive,sshSaved,
    operations,
    collectedAt:new Date()
  };
}
function card(titleText,value,detailText='',tone='',action=null){
  const node=document.createElement('section');node.className='project-health-card'+(tone?' '+tone:'');
  const h=document.createElement('h3');h.textContent=titleText;
  const valueNode=document.createElement('div');valueNode.className='project-health-value';valueNode.textContent=String(value);
  const detail=document.createElement('div');detail.className='project-health-detail';detail.textContent=String(detailText||'');
  node.append(h,valueNode,detail);
  if(action){
    const button=document.createElement('button');button.type='button';button.textContent=action.label;
    button.onclick=()=>Promise.resolve(action.run()).catch(app.showError);node.append(button);
  }
  return node;
}
function render(){
  grid.replaceChildren();
  if(!snapshot){grid.append(card('Status','Loading…'));return;}
  const git=snapshot.git;
  const gitValue=git?.repository?(git.branch||'(detached)'):'No repo';
  const gitDetail=git?.repository?[
    git.head&&('HEAD '+String(git.head).slice(0,12)),
    Number(git.changed||0)+' modified',
    Number(git.ahead||0)&&('↑'+git.ahead),
    Number(git.behind||0)&&('↓'+git.behind)
  ].filter(Boolean).join(' · '):'No Git repository detected';
  grid.append(card('Git branch / status',gitValue,gitDetail,Number(git?.changed||0)?'warn':'ok',{label:'Open Git',run:()=>globalThis.TaskMenuGitFiles?.open?.()}));

  grid.append(card('Modified files',Number(git?.changed||0),git?.repo_name||git?.repo_path||'',Number(git?.changed||0)?'warn':'ok',{label:'Git changes',run:()=>globalThis.TaskMenuGitFiles?.open?.()}));
  grid.append(card('Running tasks',snapshot.runningTasks.length,snapshot.runningTasks.map(item=>item.meta?.label||item.meta?.title||('Task '+item.meta?.task_id)).slice(0,5).join('\n')||'No task running',snapshot.runningTasks.length?'warn':'ok',{label:'Operations',run:()=>globalThis.TaskMenuOperationCenter?.open?.()}));
  grid.append(card('Open terminals',snapshot.terminals.length,snapshot.terminals.map(view=>view.meta?.title||view.meta?.label||view.meta?.id).slice(0,5).join('\n')||'No open terminal','',{label:'New terminal',run:()=>app.startTerminal()}));
  grid.append(card('Failing transfers',snapshot.failingTransfers.length,snapshot.failingTransfers.map(item=>item.title).slice(0,5).join('\n')||'No failing transfer',snapshot.failingTransfers.length?'fail':'ok',{label:'Operation Center',run:()=>globalThis.TaskMenuOperationCenter?.open?.()}));
  grid.append(card('DB connections',snapshot.dbViews.length,snapshot.dbViews.map(view=>view.profile?.name||view.meta?.adapter_kind||view.meta?.id).slice(0,5).join('\n')||'No active DB session','',{label:'Connections',run:()=>globalThis.TaskMenuConnections?.open?.()}));
  grid.append(card('SSH status',snapshot.sshActive+'/'+snapshot.sshSaved+' active',snapshot.sshSaved?'Saved SSH profiles / active terminals':'No saved SSH profile',snapshot.sshSaved&&snapshot.sshActive===0?'warn':'',{label:'Tunnel graph',run:()=>globalThis.TaskMenuConnectionGraph?.open?.()}));
  grid.append(card('Project disk usage',snapshot.disk?fmtBytes(snapshot.disk.disk_usage_bytes):'Unavailable',snapshot.disk?[
    snapshot.disk.file_count+' files',
    snapshot.disk.directory_count+' dirs',
    snapshot.disk.truncated?'scan truncated at '+snapshot.disk.max_entries+' entries':'bounded scan complete'
  ].join(' · '):'Project health endpoint unavailable',snapshot.disk?.truncated?'warn':''));
  if(snapshot.lastBuild){
    const label=snapshot.lastBuildIsBuild?'Last build':'Last task';
    const status=String(snapshot.lastBuild.status||'unknown');
    const detailText=[
      snapshot.lastBuild.label,
      snapshot.lastBuild.duration!=null&&String(snapshot.lastBuild.duration)+' ms',
      snapshot.lastBuild.ended_at||snapshot.lastBuild.started_at
    ].filter(Boolean).join(' · ');
    grid.append(card(label,status,detailText,status==='failed'?'fail':(status==='success'?'ok':''),{label:'Task History',run:()=>globalThis.TaskMenuActivityBar?.activate?.('history')}));
  }else{
    grid.append(card('Last build/task','None','No durable task run history'));
  }
  footerText.textContent='Runtime overview · metrics are read from existing TaskDeck subsystems';
  footerTime.textContent='Updated '+snapshot.collectedAt.toLocaleTimeString();
}
async function refresh(){
  if(refreshing)return;refreshing=true;refreshButton.disabled=true;
  try{snapshot=await collect();render();}
  finally{refreshing=false;refreshButton.disabled=false;}
}
function open(){
  backdrop.classList.add('visible');render();refresh();
  clearInterval(pollTimer);pollTimer=setInterval(()=>{if(backdrop.classList.contains('visible')&&!document.hidden)refresh();},5000);
}
function close(){backdrop.classList.remove('visible');clearInterval(pollTimer);pollTimer=0;}
function installLauncher(){
  const rail=document.querySelector('.task-activity-bar');if(!rail)return false;
  if(rail.querySelector('[data-view="health"]'))return true;
  const button=document.createElement('button');button.type='button';button.className='task-activity-button';button.dataset.view='health';button.title='Project Health';button.setAttribute('aria-label','Project Health');button.textContent='♥';button.onclick=open;rail.append(button);return true;
}
if(!installLauncher()){const observer=new MutationObserver(()=>{if(installLauncher())observer.disconnect();});observer.observe(document.body,{childList:true,subtree:true});}
refreshButton.onclick=()=>refresh().catch(app.showError);closeButton.onclick=close;
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();});

globalThis.TaskMenuProjectHealth={open,close,refresh,collect,get snapshot(){return snapshot;},get visible(){return backdrop.classList.contains('visible');}};
