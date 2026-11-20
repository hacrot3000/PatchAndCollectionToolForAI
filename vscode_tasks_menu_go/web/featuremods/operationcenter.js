const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Operation Center');

const style=document.createElement('style');
style.textContent=`
.operation-center-backdrop{display:none;position:fixed;inset:0;z-index:17550;background:rgba(0,0,0,.48);align-items:flex-start;justify-content:center;padding:6vh 18px 18px}
.operation-center-backdrop.visible{display:flex}
.operation-center-dialog{width:min(1050px,96vw);max-height:88vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 60px rgba(0,0,0,.58);overflow:hidden}
.operation-center-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #303843}.operation-center-title{font-weight:700;flex:1}.operation-center-summary{font-size:10px;opacity:.65}
.operation-center-tabs{display:flex;gap:5px;padding:7px 10px;border-bottom:1px solid #303843;flex-wrap:wrap}.operation-center-tabs button.active{font-weight:700}
.operation-center-body{overflow:auto;min-height:160px;max-height:68vh}.operation-center-row{display:grid;grid-template-columns:110px minmax(180px,1fr) 110px minmax(150px,1fr) auto;gap:8px;align-items:center;padding:8px 10px;border-bottom:1px solid #252c35;font-size:11px}
.operation-center-source{font-weight:700}.operation-center-main{min-width:0}.operation-center-name{font-weight:650;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.operation-center-detail{font-size:10px;opacity:.62;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.operation-center-status{font-weight:700;text-transform:uppercase}.operation-center-error{font:10px/1.35 ui-monospace,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.operation-center-actions{display:flex;gap:4px;flex-wrap:wrap;justify-content:flex-end}.operation-center-actions button{padding:3px 6px;font-size:10px}
.operation-center-empty{padding:28px;text-align:center;opacity:.6}.operation-center-status.failed{color:#ff9a9a}.operation-center-status.completed{color:#83d69a}.operation-center-status.running{color:#8ec5ff}
.operation-center-count{display:inline-grid;min-width:18px;height:18px;padding:0 4px;place-items:center;border-radius:9px;background:#2b3440;font-size:9px}
html[data-taskmenu-theme="light"] .operation-center-dialog{background:#fff;border-color:#b9c0c8}.operation-center-row{border-color:#252c35}html[data-taskmenu-theme="light"] .operation-center-row{border-color:#d0d7de}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='operation-center-backdrop';
const dialog=document.createElement('div');dialog.className='operation-center-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Operation Center');
const head=document.createElement('div');head.className='operation-center-head';
const title=document.createElement('div');title.className='operation-center-title';title.textContent='OPERATION CENTER';
const summary=document.createElement('div');summary.className='operation-center-summary';
const refreshButton=document.createElement('button');refreshButton.type='button';refreshButton.textContent='Refresh';
const clearButton=document.createElement('button');clearButton.type='button';clearButton.textContent='Clear completed';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close Operation Center';
head.append(title,summary,refreshButton,clearButton,closeButton);
const tabs=document.createElement('div');tabs.className='operation-center-tabs';
const body=document.createElement('div');body.className='operation-center-body';
dialog.append(head,tabs,body);backdrop.append(dialog);document.body.append(backdrop);

const localOperations=new Map();
const clearedKeys=new Set();
let operations=[];
let activeFilter='running';
let refreshing=false;
let pollTimer=0;

function operationKey(item){return String(item?.source||'unknown')+':'+String(item?.id||'');}
function operationStatus(value){
  value=String(value||'').trim().toLowerCase();
  if(['failed','error','conflict','incomplete','launch failed'].includes(value))return 'failed';
  if(['completed','complete','success','done','pass','passed','canceled','cancelled','stopped','skipped'].includes(value))return 'completed';
  if(['queued','pending','waiting'].includes(value))return 'queued';
  return 'running';
}
function operationTimestamp(item){
  const values=[item?.updated_at,item?.finished_at,item?.ended_at,item?.created_at,item?.started_at];
  for(const value of values){const n=Date.parse(String(value||''));if(Number.isFinite(n))return n;}
  return 0;
}
function normalizeOperation(item){
  if(!item||!item.id)return null;
  return {...item,source:String(item.source||'unknown'),id:String(item.id),title:String(item.title||item.id),detail:String(item.detail||''),error:String(item.error||''),status:operationStatus(item.status)};
}
async function collectGitOperations(){
  try{
    const data=await app.jsonFetch('/api/git/jobs',{cache:'no-store'});
    return (Array.isArray(data?.jobs)?data.jobs:[]).map(job=>({
      source:'git',id:String(job?.id||''),title:'Git · '+String(job?.action||'operation'),
      detail:String(job?.command||''),status:String(job?.state||'running'),error:String(job?.error||''),
      created_at:job?.started_at||'',updated_at:job?.finished_at||'',can_cancel:String(job?.state||'')==='running',can_retry:false,can_open:true
    }));
  }catch{return [];}
}
async function collectTransferOperations(){
  try{return await globalThis.TaskMenuFileTransfer?.operationSnapshot?.()||[];}catch{return [];}
}
function collectPatchOperations(){
  try{return globalThis.TaskMenuPatchPanel?.operationSnapshot?.()||[];}catch{return [];}
}
function taskTitle(taskID,fallback='Task'){
  const task=(Array.isArray(app.taskData?.tasks)?app.taskData.tasks:[]).find(item=>Number(item?.id)===Number(taskID));
  return String(task?.menu_label||task?.menuLabel||task?.label||fallback);
}
async function collectTaskOperations(){
  const out=[],activeSessionIDs=new Set();
  for(const [id,view] of app.views||[]){
    const meta=view?.meta||{};
    if(Number(meta.task_id)<=0||String(meta.status||'').toLowerCase()!=='running')continue;
    activeSessionIDs.add(String(id));
    out.push({source:'task',id:String(id),session_id:String(id),task_id:Number(meta.task_id),title:'Task · '+taskTitle(meta.task_id,meta.label||'Task'),detail:String(meta.command_preview||meta.cwd||''),status:'running',created_at:meta.started_at||'',can_cancel:true,can_retry:false,can_open:true});
  }
  try{
    const data=await app.jsonFetch('/api/task-runs',{cache:'no-store'});
    for(const run of (Array.isArray(data?.runs)?data.runs:[]).slice(0,30)){
      if(activeSessionIDs.has(String(run?.session_id||'')))continue;
      out.push({source:'task',id:String(run?.id||''),session_id:String(run?.session_id||''),task_id:Number(run?.task_id||0),title:'Task · '+String(run?.label||taskTitle(run?.task_id,'Task')),detail:[run?.duration!=null?String(run.duration)+' ms':'',run?.git_commit?String(run.git_commit).slice(0,12):''].filter(Boolean).join(' · '),status:String(run?.status||'completed'),error:String(run?.error||''),created_at:run?.started_at||'',updated_at:run?.ended_at||'',can_cancel:false,can_retry:Number(run?.task_id)>0,can_open:true});
    }
  }catch{}
  return out;
}
function collectSelfUpdateOperations(){
  try{return globalThis.TaskMenuSelfUpdate?.operationSnapshot?.()||[];}catch{return [];}
}
function collectLocalOperations(){return [...localOperations.values()];}

async function collectOperations(){
  const groups=await Promise.all([collectGitOperations(),collectTransferOperations(),collectTaskOperations()]);
  const merged=[...groups.flat(),...collectPatchOperations(),...collectSelfUpdateOperations(),...collectLocalOperations()];
  const seen=new Set(),out=[];
  for(const raw of merged){
    const item=normalizeOperation(raw);if(!item)continue;
    const key=operationKey(item);if(seen.has(key)||clearedKeys.has(key))continue;
    seen.add(key);out.push(item);
  }
  out.sort((a,b)=>operationTimestamp(b)-operationTimestamp(a)||operationKey(a).localeCompare(operationKey(b)));
  return out;
}
function statusCounts(){
  const counts={queued:0,running:0,completed:0,failed:0};
  for(const item of operations)counts[item.status]=(counts[item.status]||0)+1;
  return counts;
}
function renderTabs(){
  tabs.replaceChildren();const counts=statusCounts();
  for(const key of ['queued','running','completed','failed']){
    const button=document.createElement('button');button.type='button';button.classList.toggle('active',activeFilter===key);
    const label=document.createElement('span');label.textContent=key[0].toUpperCase()+key.slice(1)+' ';
    const count=document.createElement('span');count.className='operation-center-count';count.textContent=String(counts[key]||0);
    button.append(label,count);button.onclick=()=>{activeFilter=key;render();};tabs.append(button);
  }
  summary.textContent=operations.length+' operation(s)';
}
async function copyText(value){
  const text=String(value||'');if(!text)return;
  if(navigator.clipboard?.writeText){await navigator.clipboard.writeText(text);return;}
  const area=document.createElement('textarea');area.value=text;area.style.position='fixed';area.style.left='-9999px';document.body.append(area);area.select();try{document.execCommand('copy');}finally{area.remove();}
}
async function controlOperation(item,action){
  if(item.source==='git'){
    if(action==='cancel')return app.jsonFetch('/api/git/jobs/control',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:item.id,action:'cancel'})});
    if(action==='open')return globalThis.TaskMenuGitFiles?.open?.();
  }
  if(item.source==='transfer')return globalThis.TaskMenuFileTransfer?.operationControl?.(item,action);
  if(item.source==='patch')return globalThis.TaskMenuPatchPanel?.operationControl?.(item,action);
  if(item.source==='self-update')return globalThis.TaskMenuSelfUpdate?.operationControl?.(item,action);
  if(item.source==='task'){
    if(action==='cancel'){
      const id=String(item.session_id||item.id||'');if(!id)throw new Error('Task session unavailable');
      return app.jsonFetch('/api/sessions/'+encodeURIComponent(id)+'/stop',{method:'POST'});
    }
    if(action==='retry'){
      const task=(Array.isArray(app.taskData?.tasks)?app.taskData.tasks:[]).find(row=>Number(row?.id)===Number(item.task_id));
      if(!task)throw new Error('Original task is no longer available');
      return app.startTask(task);
    }
    if(action==='open'){
      const view=app.views?.get?.(String(item.session_id||''));if(view?.tab){view.tab.click();return true;}
      return globalThis.TaskMenuActivityBar?.activate?.('history');
    }
  }
  const local=localOperations.get(operationKey(item));
  if(local){
    const fn=action==='cancel'?local?._cancel:(action==='retry'?local?._retry:local?._open);
    if(typeof fn==='function')return fn();
  }
  if(item.source==='database'&&action==='open'&&item.profile_id)return globalThis.TaskMenuDatabase?.openProfile?.(item.profile_id);
  throw new Error('Operation action is unavailable');
}
function actionButton(label,run,{disabled=false}={}){
  const button=document.createElement('button');button.type='button';button.textContent=label;button.disabled=disabled;
  button.onclick=()=>Promise.resolve(run()).then(()=>refresh()).catch(app.showError);return button;
}
function render(){
  renderTabs();body.replaceChildren();
  const shown=operations.filter(item=>item.status===activeFilter);
  if(!shown.length){const empty=document.createElement('div');empty.className='operation-center-empty';empty.textContent='No '+activeFilter+' operations';body.append(empty);return;}
  for(const item of shown){
    const row=document.createElement('div');row.className='operation-center-row';
    const source=document.createElement('div');source.className='operation-center-source';source.textContent=item.source;
    const main=document.createElement('div');main.className='operation-center-main';
    const name=document.createElement('div');name.className='operation-center-name';name.textContent=item.title;
    const detail=document.createElement('div');detail.className='operation-center-detail';detail.textContent=item.detail;detail.title=item.detail;main.append(name,detail);
    const status=document.createElement('div');status.className='operation-center-status '+item.status;status.textContent=item.status;
    const error=document.createElement('div');error.className='operation-center-error';error.textContent=item.error||'—';error.title=item.error||'';
    const actions=document.createElement('div');actions.className='operation-center-actions';
    if(item.can_open)actions.append(actionButton('Open',()=>controlOperation(item,'open')));
    if(item.can_cancel&&['queued','running'].includes(item.status))actions.append(actionButton('Cancel',()=>controlOperation(item,'cancel')));
    if(item.can_retry&&item.status==='failed')actions.append(actionButton('Retry',()=>controlOperation(item,'retry')));
    if(item.error)actions.append(actionButton('Copy error',()=>copyText(item.error)));
    row.append(source,main,status,error,actions);body.append(row);
  }
}
async function refresh(){
  if(refreshing)return;refreshing=true;refreshButton.disabled=true;
  try{operations=await collectOperations();render();}
  finally{refreshing=false;refreshButton.disabled=false;}
}
function open(){
  backdrop.classList.add('visible');refresh();clearInterval(pollTimer);pollTimer=setInterval(()=>{if(backdrop.classList.contains('visible'))refresh();},1500);
}
function close(){backdrop.classList.remove('visible');clearInterval(pollTimer);pollTimer=0;}
function clearCompleted(){
  for(const item of operations)if(item.status==='completed')clearedKeys.add(operationKey(item));
  for(const [key,item] of localOperations)if(operationStatus(item.status)==='completed')localOperations.delete(key);
  operations=operations.filter(item=>item.status!=='completed');render();
}
function begin(spec={}){
  const id=String(spec.id||globalThis.crypto?.randomUUID?.()||('op-'+Date.now().toString(36)+'-'+Math.random().toString(36).slice(2,9)));
  const key='database:'+id;
  const source=String(spec.source||'database').trim()||'database';
  const item={source,id,title:String(spec.title||(source==='database'?'Database operation':'Operation')),detail:String(spec.detail||''),profile_id:String(spec.profile_id||''),status:'running',error:'',created_at:new Date().toISOString(),can_cancel:typeof spec.cancel==='function',can_retry:false,can_open:Boolean(spec.open||spec.profile_id),_cancel:spec.cancel||null,_retry:null,_open:spec.open||null};
  localOperations.set(key,item);if(backdrop.classList.contains('visible'))refresh();
  return {
    id,
    update(patch={}){Object.assign(item,patch);if(backdrop.classList.contains('visible'))refresh();return this;},
    complete(patch={}){Object.assign(item,patch,{status:'completed',updated_at:new Date().toISOString(),can_cancel:false});if(backdrop.classList.contains('visible'))refresh();return this;},
    fail(error,{retry=null}={}){item.status='failed';item.error=String(error?.message||error||'Database operation failed');item.updated_at=new Date().toISOString();item.can_cancel=false;item.can_retry=typeof retry==='function';item._retry=retry;if(backdrop.classList.contains('visible'))refresh();return this;}
  };
}

refreshButton.onclick=()=>refresh();
clearButton.onclick=clearCompleted;closeButton.onclick=close;
backdrop.onmousedown=event=>{if(event.target===backdrop)close();};
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();});

globalThis.TaskMenuOperationCenter={open,close,refresh,begin,get items(){return [...operations];},get visible(){return backdrop.classList.contains('visible');}};
