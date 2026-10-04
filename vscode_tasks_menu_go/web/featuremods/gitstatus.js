const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for git status');
const gitRecovery=globalThis.TaskDeckGitRecovery;
const gitIgnoreWizard=globalThis.TaskDeckGitIgnoreWizard;

const style=document.createElement('style');
style.textContent=`
.git-status-pill{display:none;white-space:nowrap;font-family:ui-monospace,monospace;font-size:11px;padding:5px 8px;max-width:360px;overflow:hidden;text-overflow:ellipsis}.git-status-pill.visible{display:block}.git-status-pill.dirty{border-color:#8a6d3b;background:#3a2f1d}.git-status-pill.clean{border-color:#3f6b4a;background:#1f3526}
.git-panel{display:none;position:fixed;z-index:2100;top:calc(var(--taskmenu-header-height,30px) + 6px);left:12px;right:auto;bottom:12px;width:min(780px,calc(100vw - 24px));border:1px solid #3b414d;border-radius:10px;background:#11161d;box-shadow:0 16px 42px rgba(0,0,0,.48);overflow:hidden}
body.task-sidebar-auto-hide .git-panel{left:60px;width:min(780px,calc(100vw - 72px))}.git-panel.visible{display:flex;flex-direction:column}.git-panel-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #30343b}.git-panel-title{font-weight:700}.git-panel-summary{font:11px ui-monospace,monospace;opacity:.65;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-panel-close{padding:4px 8px}.git-repo-bar{display:flex;align-items:center;gap:7px;padding:7px 10px;border-bottom:1px solid #30343b}.git-repo-bar label{font-size:10px;font-weight:700;opacity:.6}.git-repo-select{min-width:180px;max-width:310px;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:5px 7px}.git-repo-path{flex:1;min-width:0;font:10px ui-monospace,monospace;opacity:.6;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-repo-rescan{padding:4px 7px;font-size:10px}.git-quick-groups{display:flex;gap:7px;flex-wrap:wrap;padding:8px 10px;border-bottom:1px solid #30343b}.git-quick-group{display:flex;align-items:center;gap:4px;padding:4px;border:1px solid #30343b;border-radius:7px;background:#151b23}.git-quick-group strong{font-size:10px;opacity:.55;margin:0 3px}.git-quick-group button{padding:4px 7px;font-size:11px}.git-action-running{background:#34445a!important;border-color:#6f91bb!important;box-shadow:0 0 0 1px rgba(111,145,187,.35) inset}.git-action-success{background:#21452d!important;border-color:#4f8d61!important}.git-action-error{background:#51262b!important;border-color:#9b5059!important}.git-action-running,.git-action-success,.git-action-error{transition:background .12s ease,border-color .12s ease,transform .12s ease}.git-action-running{transform:translateY(1px)}.git-nav{display:flex;gap:4px;overflow:auto;padding:7px 10px;border-bottom:1px solid #30343b}.git-nav button{padding:5px 8px;font-size:11px;white-space:nowrap}.git-nav button.active{background:#34445a;border-color:#52719a}.git-panel-content{flex:1;min-height:0;overflow:auto;padding:10px}.git-empty{opacity:.6;padding:18px;text-align:center}.git-row{display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:7px;align-items:center;padding:7px 5px;border-bottom:1px solid #252c35}.git-row:last-child{border-bottom:0}.git-row-code{font:11px ui-monospace,monospace;min-width:28px;opacity:.75}.git-row-main{min-width:0}.git-row-title{font:12px ui-monospace,monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-row-sub{font-size:10px;opacity:.55;margin-top:2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-row-actions{display:flex;gap:4px;flex-wrap:wrap;justify-content:flex-end}.git-row-actions button{padding:3px 6px;font-size:10px}.git-diff-pre,.git-operation-output,.git-compare-pre{white-space:pre-wrap;word-break:break-word;font:11px/1.45 ui-monospace,monospace;background:#090d12;border:1px solid #30343b;border-radius:7px;padding:9px;margin:8px 0;overflow:auto}.git-operation{border-top:1px solid #30343b;padding:7px 10px;max-height:min(46vh,460px);overflow:auto}.git-operation.running{box-shadow:inset 3px 0 0 #6f91bb}.git-operation-head{display:flex;align-items:center;gap:6px}.git-operation-command{flex:1;min-width:0;font:10px ui-monospace,monospace;opacity:.7;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-operation button{padding:3px 6px;font-size:10px}.git-operation-output{margin:5px 0 0;max-height:min(38vh,360px)}.git-branch-create,.git-compare-controls{display:flex;gap:6px;align-items:center;margin-bottom:8px}.git-branch-create input,.git-compare-controls select{flex:1;min-width:0;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:6px 8px}.git-branch-section-toggle{display:block;width:100%;text-align:left;border:0;background:transparent;color:inherit;padding:7px 0 5px;font-size:10px;font-weight:700;opacity:.72}.git-branch-section-toggle:hover{opacity:1}.git-branch-section-toggle:focus-visible{outline:1px solid #52719a;outline-offset:2px}.git-direction-ahead{color:#72cf8a}.git-direction-behind{color:#e5b85c}
html[data-taskmenu-theme="light"] .git-panel{background:#fff;border-color:#b9c0c8;box-shadow:0 16px 42px rgba(0,0,0,.18)}html[data-taskmenu-theme="light"] .git-panel-head,html[data-taskmenu-theme="light"] .git-repo-bar,html[data-taskmenu-theme="light"] .git-quick-groups,html[data-taskmenu-theme="light"] .git-nav,html[data-taskmenu-theme="light"] .git-operation{border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-quick-group{background:#f6f8fa;border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-repo-select{background:#fff;border-color:#b9c0c8;color:#202124}html[data-taskmenu-theme="light"] .git-diff-pre,html[data-taskmenu-theme="light"] .git-operation-output,html[data-taskmenu-theme="light"] .git-compare-pre{background:#f6f8fa;border-color:#d0d7de;color:#202124}
`;
document.head.append(style);

const workspace=document.querySelector('#workspace');
const pill=document.createElement('button');pill.className='git-status-pill';pill.title='Git — click to open Git Quick Actions';workspace.after(pill);

const panel=document.createElement('div');panel.className='git-panel';
const panelHead=document.createElement('div');panelHead.className='git-panel-head';
const panelTitle=document.createElement('span');panelTitle.className='git-panel-title';panelTitle.textContent='Git Quick Actions';
const panelSummary=document.createElement('span');panelSummary.className='git-panel-summary';
const panelClose=document.createElement('button');panelClose.className='git-panel-close';panelClose.textContent='×';panelClose.title='Close Git panel';
panelHead.append(panelTitle,panelSummary,panelClose);
const repoBar=document.createElement('div');repoBar.className='git-repo-bar';
const repoLabel=document.createElement('label');repoLabel.textContent='Repository';
const repoSelect=document.createElement('select');repoSelect.className='git-repo-select';repoSelect.title='Active Git repository';
const repoPath=document.createElement('span');repoPath.className='git-repo-path';
const repoRescan=document.createElement('button');repoRescan.className='git-repo-rescan';repoRescan.textContent='↻ Scan';repoRescan.title='Rescan Git repositories in workspace';
repoBar.append(repoLabel,repoSelect,repoPath,repoRescan);
const quickGroups=document.createElement('div');quickGroups.className='git-quick-groups';
const nav=document.createElement('div');nav.className='git-nav';
const content=document.createElement('div');content.className='git-panel-content';
const operation=document.createElement('div');operation.className='git-operation';operation.hidden=true;
const operationHead=document.createElement('div');operationHead.className='git-operation-head';
const operationCommand=document.createElement('span');operationCommand.className='git-operation-command';
const operationCopy=document.createElement('button');operationCopy.textContent='Copy command';
const operationCancel=document.createElement('button');operationCancel.textContent='Cancel';operationCancel.hidden=true;operationCancel.title='Cancel running Git process';
const operationClear=document.createElement('button');operationClear.textContent='Clear';
const operationOutput=document.createElement('pre');operationOutput.className='git-operation-output';
operationHead.append(operationCommand,operationCopy,operationCancel,operationClear);operation.append(operationHead,operationOutput);
panel.append(panelHead,repoBar,quickGroups,nav,content,operation);document.body.append(panel);

let currentStatus=null,currentView='changes',refreshing=false,lastCommand='',refreshSeq=0;
let repositories=[],activeRepoID='',gitAutoSelectFromTerminalCWD=false;
let activeGitJob=null;
const runningActions=new Map();
function el(tag,className,text){const node=document.createElement(tag);if(className)node.className=className;if(text!==undefined)node.textContent=text;return node;}
function q(value){value=String(value??'');return /^[A-Za-z0-9_./:@+\-]+$/.test(value)?value:"'"+value.replace(/'/g,"'\\''")+"'";}
function actionCommand(action,payload={}){
  switch(action){
    case 'fetch':return 'git fetch --prune';case 'pull':return 'git pull --ff-only';case 'push':return 'git push';case 'stage_all':return 'git add -A';
    case 'stage':return 'git add -- '+q(payload.path);case 'unstage':return 'git restore --staged -- '+q(payload.path);case 'ignore':return 'Add .gitignore rule for '+q(payload.path);case 'commit':return 'git commit -m '+q(payload.message);
    case 'switch':return 'git switch '+q(payload.branch);case 'create_branch':return 'git switch -c '+q(payload.branch);case 'merge':return 'git merge --no-edit '+q(payload.merge_ref||payload.branch);case 'merge_to':return 'Merge To '+q(payload.expected_current)+' -> '+q(payload.branch)+' and push';case 'stash_push':return 'git stash push -u -m '+q(payload.message||'(auto)');case 'stash_pop':return 'git stash pop'+(payload.ref?' '+q(payload.ref):'');default:return 'git '+action;
  }
}
async function copyText(text){
  if(navigator.clipboard&&window.isSecureContext){try{await navigator.clipboard.writeText(text);return;}catch{}}
  const area=document.createElement('textarea');area.value=text;area.setAttribute('readonly','');area.style.position='fixed';area.style.left='-9999px';document.body.append(area);area.select();try{document.execCommand('copy');}finally{area.remove();}
}
function showOperation(command,output,error='',state='done'){
  lastCommand=command||'';operation.hidden=false;operation.classList.toggle('running',state==='running');operationCommand.textContent=command||'Git';operationOutput.textContent=[error&&('ERROR: '+error),output].filter(Boolean).join('\n')||'(no output)';
}
function beginOperation(command,message='Running…'){
  showOperation(command,message,'','running');
  requestAnimationFrame(()=>operation.scrollIntoView({block:'nearest'}));
}
function gitUIAsyncAction(action){return ['fetch','pull','push','merge'].includes(String(action||''));}
function gitJobElapsed(startedAt){
  const start=Date.parse(startedAt||'');if(!Number.isFinite(start))return '';
  const seconds=Math.max(0,Math.round((Date.now()-start)/1000));
  if(seconds<60)return seconds+'s';
  return Math.floor(seconds/60)+'m '+String(seconds%60).padStart(2,'0')+'s';
}
function renderGitJob(snapshot,command){
  const state=String(snapshot?.state||'running');
  const elapsed=gitJobElapsed(snapshot?.started_at);
  const header=[state.toUpperCase(),elapsed&&('elapsed '+elapsed)].filter(Boolean).join(' · ');
  showOperation(snapshot?.command||command,[header,snapshot?.output||''].filter(Boolean).join('\n'),snapshot?.error||'',state==='running'?'running':'done');
  operationCancel.hidden=state!=='running';
}
async function cancelGitJob(){
  const job=activeGitJob;if(!job?.id)return false;
  operationCancel.disabled=true;operationCancel.textContent='Canceling…';
  try{
    await app.jsonFetch('/api/git/jobs/control',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:job.id,action:'cancel'})});
    return true;
  }finally{
    operationCancel.disabled=false;operationCancel.textContent='Cancel';
  }
}
async function waitGitJob(initial,command){
  const jobID=String(initial?.job_id||'');if(!jobID)return initial;
  activeGitJob={id:jobID,command};operationCancel.hidden=false;
  try{
    while(true){
      const data=await app.jsonFetch('/api/git/jobs?id='+encodeURIComponent(jobID));
      renderGitJob(data,command);
      if(String(data.state||'')!=='running')return data;
      await new Promise(resolve=>setTimeout(resolve,250));
    }
  }finally{
    activeGitJob=null;operationCancel.hidden=true;
  }
}
operationCancel.onclick=()=>cancelGitJob().catch(app.showError);
operationCopy.onclick=()=>copyText(lastCommand).catch(app.showError);operationClear.onclick=()=>{if(activeGitJob)return;operation.hidden=true;operation.classList.remove('running');operationOutput.textContent='';lastCommand='';};

function repoStorageKey(){return 'taskdeck:git-repository:'+(String(app.taskData?.workspace||'').trim()||'global');}
function activeRepository(){return repositories.find(item=>item.id===activeRepoID)||null;}
function repositoryOptionText(item){
  const parts=[item.name||item.id,item.branch||'(no branch)'];
  parts.push(item.changed?item.changed+' changed':'clean');
  if(item.ahead)parts.push('↑'+item.ahead);
  if(item.behind)parts.push('↓'+item.behind);
  return parts.join(' · ');
}
function renderRepositorySelector(){
  const previous=activeRepoID;
  repoSelect.replaceChildren();
  for(const item of repositories){
    const option=document.createElement('option');option.value=item.id;option.textContent=repositoryOptionText(item);option.title=(item.path||item.id)+(item.head?' · '+item.head:'');repoSelect.append(option);
  }
  if(previous&&repositories.some(item=>item.id===previous))repoSelect.value=previous;
  repoSelect.disabled=!repositories.length;
  const selected=activeRepository();
  repoPath.textContent=selected?.path||'No Git repository';
  repoPath.title=selected?.path||'';
}
async function refreshRepositories(force=false){
  const query=new URLSearchParams({view:'repositories'});if(force)query.set('refresh','1');
  const data=await app.jsonFetch('/api/git/status?'+query.toString());
  repositories=Array.isArray(data.repositories)?data.repositories:[];
  gitAutoSelectFromTerminalCWD=Boolean(data.auto_select_from_terminal_cwd);
  let wanted=activeRepoID;
  if(!wanted){try{wanted=localStorage.getItem(repoStorageKey())||'';}catch{}}
  if(!repositories.some(item=>item.id===wanted))wanted=data.default_repository||repositories[0]?.id||'';
  activeRepoID=wanted;
  if(activeRepoID){try{localStorage.setItem(repoStorageKey(),activeRepoID);}catch{}}
  renderRepositorySelector();
  return data;
}
async function selectRepository(id,{reload=true,persist=true}={}){
  if(!repositories.some(item=>item.id===id))return false;
  const changed=activeRepoID!==id;activeRepoID=id;
  if(changed)refreshSeq++;
  if(persist){try{localStorage.setItem(repoStorageKey(),id);}catch{}}
  renderRepositorySelector();
  if(changed)currentStatus=null;
  if(reload){await refresh();await loadCurrentView();}
  return true;
}
function normalizeCWD(value){return String(value||'').replace(/\\/g,'/').replace(/\/+$/,'');}
function repositoryForCWD(cwd){
  const workspaceRoot=normalizeCWD(app.taskData?.workspace);
  let value=normalizeCWD(cwd);
  if(!value)return null;
  if(workspaceRoot&&value===workspaceRoot)value='.';
  else if(workspaceRoot&&value.startsWith(workspaceRoot+'/'))value=value.slice(workspaceRoot.length+1);
  else if(workspaceRoot&&(value.startsWith('/')||/^[A-Za-z]:\//.test(value)))return null;
  value=value.replace(/^\.\//,'');
  let best=null;
  for(const item of repositories){
    const root=item.path==='.'?'':String(item.path||item.id).replace(/^\.\//,'').replace(/\/+$/,'');
    if(root===''||value===root||value.startsWith(root+'/')){
      if(!best||root.length>(best.path==='.'?0:String(best.path||best.id).length))best=item;
    }
  }
  return best;
}
async function autoSelectRepositoryForTerminal(id,{reload=true}={}){
  if(!gitAutoSelectFromTerminalCWD||!id)return false;
  const view=app.views.get(String(id));
  if(!view||String(view.meta?.target_type||'').toLowerCase()==='ssh'||view.meta?.target_profile_id)return false;
  const repoAtStart=activeRepoID;
  let cwd=view.meta?.cwd||'';
  try{
    const live=await app.jsonFetch('/api/sessions/'+encodeURIComponent(String(id))+'/cwd');
    if(live?.local===false)return false;
    if(live?.cwd)cwd=live.cwd;
  }catch(error){
    console.warn('Git repository auto-select live CWD unavailable',error);
  }
  if(String(app.active||'')!==String(id)||activeRepoID!==repoAtStart)return false;
  const match=repositoryForCWD(cwd);
  if(match&&match.id!==activeRepoID)return selectRepository(match.id,{reload});
  return false;
}

function renderStatus(data){
  currentStatus=data;
  if(!data?.repository){pill.className='git-status-pill';pill.textContent='';panel.classList.remove('visible');return;}
  const parts=['Git:',data.repo_name||activeRepository()?.name||data.repo_id||'repo',data.branch||'(unknown)',data.head||'--------'];parts.push(data.changed?data.changed+' changed':'clean');if(data.ahead)parts.push('↑'+data.ahead);if(data.behind)parts.push('↓'+data.behind);
  const text=parts.join(' · ');pill.textContent=text;panelSummary.textContent=text;pill.className='git-status-pill visible '+(data.changed?'dirty':'clean');pill.title='Repository: '+(data.repo_name||data.repo_id||'unknown')+'\nPath: '+(data.repo_path||'')+'\nBranch: '+(data.branch||'unknown')+'\nHEAD: '+(data.head||'unknown')+'\nChanged: '+(data.changed||0)+'\nAhead: '+(data.ahead||0)+'\nBehind: '+(data.behind||0)+'\nClick to open Git Quick Actions';
}
async function refresh(){
  const seq=++refreshSeq;refreshing=true;
  try{
    if(!activeRepoID)await refreshRepositories(false);
    let requestedRepo=activeRepoID;
    const selectedBefore=requestedRepo;
    const query=new URLSearchParams();if(requestedRepo)query.set('repo',requestedRepo);
    let data=await app.jsonFetch('/api/git/status'+(query.size?'?'+query.toString():''));
    if(seq!==refreshSeq||requestedRepo!==activeRepoID)return;
    if(!data?.repository&&selectedBefore){
      await refreshRepositories(true);
      if(seq!==refreshSeq)return;
      requestedRepo=activeRepoID;
      if(requestedRepo&&requestedRepo!==selectedBefore){
        const retry=new URLSearchParams({repo:requestedRepo});
        data=await app.jsonFetch('/api/git/status?'+retry.toString());
      }
    }
    if(seq!==refreshSeq||requestedRepo!==activeRepoID)return;
    renderStatus(data);
    const item=repositories.find(repo=>repo.id===activeRepoID);
    if(item&&data?.repository)Object.assign(item,{branch:data.branch,head:data.head,changed:data.changed,ahead:data.ahead,behind:data.behind});
    renderRepositorySelector();
  }
  catch(e){if(seq===refreshSeq){pill.className='git-status-pill';console.warn('Git status refresh failed',e);}}
  finally{if(seq===refreshSeq)refreshing=false;}
}
async function gitView(view,params={}){
  const repoID=activeRepoID;
  const query=new URLSearchParams({view,...params});if(repoID)query.set('repo',repoID);
  const data=await app.jsonFetch('/api/git/status?'+query.toString());
  return repoID===activeRepoID?data:null;
}
function actionKey(actionName,payload={},repoID=activeRepoID){
  const entries=Object.entries(payload).filter(([key])=>key!=='merge_ref').sort(([a],[b])=>a.localeCompare(b));
  return repoID+'|'+actionName+':'+JSON.stringify(Object.fromEntries(entries));
}
function gitFailureError(message,data={}){
  const error=new Error(message||data.error||'Git action failed');
  error.gitOutput=String(data.output||'');
  error.gitFailureCode=String(data.failure_code||'');
  error.gitDetails=data&&typeof data==='object'?data:{};
  return error;
}
function openGitRecovery({actionName='unknown',payload={},command='',error='',output='',failureCode='',details={},repoID=activeRepoID}={}){
  if(!gitRecovery?.open)return false;
  const selected=repositories.find(item=>item.id===repoID)||activeRepository()||{};
  const repository={...selected,branch:(repoID===activeRepoID?currentStatus?.branch:'')||selected.branch||''};
  const ensureRepo=()=>{if(!repoID)throw new Error('Git repository selection is unavailable');};
  gitRecovery.open({
    actionName,payload,command,error:String(error||''),output:String(output||''),failureCode:String(failureCode||''),details:details&&typeof details==='object'?details:{},repository,
    runRepair:(repair,repairPayload={})=>{ensureRepo();return repairAction(repair,repairPayload,repoID);},
    retry:actionName&&actionName!=='unknown'?()=>{ensureRepo();return action(actionName,payload,'',{recovery:false,repoID});}:null,
    runAction:(name,nextPayload={})=>{ensureRepo();return action(name,nextPayload,'',{recovery:false,repoID});},
    refresh:async()=>{if(activeRepoID===repoID){await refresh();await loadCurrentView();}},
    rescan:async()=>{await refreshRepositories(true);if(activeRepoID===repoID){await refresh();await loadCurrentView();}return {ok:true,output:'Repository scan completed.'};},
    openFile:path=>window.dispatchEvent(new CustomEvent('taskmenu:project-file-open-request',{detail:{path,source:'git-conflict-recovery'}}))
  });
  return true;
}
async function repairAction(repair,payload={},repoID=activeRepoID){
  const command='Git recovery · '+String(repair||'repair').replaceAll('_',' ');
  beginOperation(command);
  try{
    const query=new URLSearchParams();if(repoID)query.set('repo',repoID);
    const data=await app.jsonFetch('/api/git/status'+(query.size?'?'+query.toString():''),{
      method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({action:'repair',repair,...payload})
    });
    showOperation(command,data.output||'',data.ok?'':(data.error||'Git recovery action failed'));
    if(!data.ok)throw gitFailureError(data.error||'Git recovery action failed',data);
    if(activeRepoID===repoID){await refresh();await loadCurrentView();}
    return data;
  }catch(error){
    showOperation(command,error?.gitOutput||'',error?.message||String(error));
    throw error;
  }
}
async function action(actionName,payload={},confirmText='',options={}){
  const repoID=options.repoID??activeRepoID;
  const recoveryEnabled=options.recovery!==false;
  const key=actionKey(actionName,payload,repoID);
  if(runningActions.has(key))return runningActions.get(key);
  if(confirmText&&!window.confirm(confirmText))return false;
  const command=actionCommand(actionName,payload);
  beginOperation(command);
  const pending=(async()=>{
    try{
      const query=new URLSearchParams();if(repoID)query.set('repo',repoID);
      let data=await app.jsonFetch('/api/git/status'+(query.size?'?'+query.toString():''),{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:actionName,...payload,async:gitUIAsyncAction(actionName)})});
      if(data?.async&&data?.job_id)data=await waitGitJob(data,command);
      showOperation(command,data.output||'',data.ok?'':(data.error||'Git action failed'));
      if(!data.ok){
        const failure=gitFailureError(data.error||'Git action failed',data);
        if(recoveryEnabled&&openGitRecovery({actionName,payload,command,error:failure.message,output:failure.gitOutput,failureCode:failure.gitFailureCode,details:failure.gitDetails,repoID}))failure.gitWizardShown=true;
        throw failure;
      }
      if(activeRepoID===repoID){await refresh();await loadCurrentView();}return data;
    }catch(error){
      showOperation(command,error?.gitOutput||'',error?.message||String(error));
      if(recoveryEnabled&&!error?.gitWizardShown&&openGitRecovery({actionName,payload,command,error:error?.message||String(error),output:error?.gitOutput||'',failureCode:error?.gitFailureCode||'',details:error?.gitDetails||{},repoID}))error.gitWizardShown=true;
      throw error;
    }finally{
      runningActions.delete(key);
    }
  })();
  runningActions.set(key,pending);
  return pending;
}
function bindActionButton(button,run){
  const label=button.textContent;
  button.onclick=async()=>{
    if(button.dataset.gitBusy==='1')return;
    button.dataset.gitBusy='1';button.disabled=true;button.setAttribute('aria-busy','true');button.classList.remove('git-action-success','git-action-error');button.classList.add('git-action-running');button.textContent='⏳ '+label;
    let state='success';
    try{
      const result=await run();
      if(result===false)state='';
    }catch(error){
      state='error';
      if(!error?.gitWizardShown){
        if(openGitRecovery({actionName:'unknown',command:label,error:error?.message||String(error),output:error?.gitOutput||'',failureCode:error?.gitFailureCode||'',details:error?.gitDetails||{}}))error.gitWizardShown=true;
      }
      if(!error?.gitWizardShown)app.showError(error);
    }finally{
      button.classList.remove('git-action-running');
      button.removeAttribute('aria-busy');
      if(state){button.classList.add(state==='success'?'git-action-success':'git-action-error');button.textContent=(state==='success'?'✓ ':'✕ ')+label;}
      else button.textContent=label;
      setTimeout(()=>{if(!button.isConnected)return;button.classList.remove('git-action-success','git-action-error');button.textContent=label;button.disabled=false;delete button.dataset.gitBusy;},state?850:0);
    }
  };
  return button;
}

function quickGroup(label,buttons){const group=el('div','git-quick-group');group.append(el('strong','',label));for(const spec of buttons){const b=el('button','',spec.label);b.title=spec.title||spec.label;bindActionButton(b,spec.run);group.append(b);}quickGroups.append(group);}
quickGroup('SYNC',[
  {label:'Refresh',run:async()=>{await refresh();await loadCurrentView();}},
  {label:'Fetch',run:()=>action('fetch')},
  {label:'Pull FF',title:'git pull --ff-only',run:()=>action('pull')},
  {label:'Push',run:()=>action('push')}
]);
quickGroup('WORKTREE',[
  {label:'Stage all',run:()=>action('stage_all')},
  {label:'Commit',run:()=>commit(false)},
  {label:'Commit + Push',run:()=>commit(true)},
  {label:'Stash',run:()=>stashPush()}
]);

async function commit(pushAfter){const message=window.prompt('Commit message:','');if(message===null||!message.trim())return false;await action('commit',{message:message.trim()});if(pushAfter)await action('push');return true;}
async function stashPush(){const message=window.prompt('Stash message (leave blank to use the default):','');if(message===null)return false;await action('stash_push',{message:message.trim()});return true;}

const views=[['repositories','Repositories'],['changes','Changes'],['branches','Branches'],['log','Log'],['ahead-behind','Ahead / Behind'],['stashes','Stash'],['compare','Compare']];
for(const [id,label] of views){const b=el('button','',label);b.dataset.gitView=id;b.onclick=()=>{currentView=id;updateNav();loadCurrentView().catch(app.showError);};nav.append(b);}
function updateNav(){for(const b of nav.querySelectorAll('button'))b.classList.toggle('active',b.dataset.gitView===currentView);}
function empty(message){content.replaceChildren(el('div','git-empty',message));}
function actionButton(label,run,title=''){const b=el('button','',label);if(title)b.title=title;return bindActionButton(b,run);}

async function loadRepositories(force=false){
  await refreshRepositories(force);content.replaceChildren();
  if(!repositories.length){empty('No Git repositories found in workspace');return;}
  for(const repo of repositories){
    const row=el('div','git-row');const code=el('span','git-row-code',repo.id===activeRepoID?'*':'');const main=el('div','git-row-main');
    const state=[repo.branch||'(no branch)',repo.head||'--------',repo.changed?repo.changed+' changed':'clean',repo.ahead&&('↑'+repo.ahead),repo.behind&&('↓'+repo.behind)].filter(Boolean).join(' · ');
    main.append(el('div','git-row-title',(repo.default?'★ ':'')+(repo.name||repo.id)),el('div','git-row-sub',(repo.path||repo.id)+' · '+state+(repo.error?' · '+repo.error:'')));
    const actions=el('div','git-row-actions');
    if(repo.id!==activeRepoID)actions.append(actionButton('Open',async()=>{currentView='changes';updateNav();return selectRepository(repo.id);}));
    actions.append(actionButton('Copy path',()=>copyText(repo.path||repo.id)));
    row.append(code,main,actions);content.append(row);
  }
}

async function showDiff(path,mode){
  const data=await gitView('diff',{path,mode});if(!data)return false;content.replaceChildren();
  const back=actionButton('← Changes',()=>{currentView='changes';updateNav();return loadChanges();});const title=el('strong','',`${mode==='staged'?'STAGED ':'DIFF '}${path}`);const pre=el('pre','git-diff-pre',data.diff||'(no tracked diff; untracked files can be staged directly)');content.append(back,title,pre);
}
async function openIgnoreWizard(change){
  if(!gitIgnoreWizard?.open)throw new Error('Git ignore wizard unavailable');
  const repoID=activeRepoID;
  if(!repoID)throw new Error('Git repository selection is unavailable');
  const selected=repositories.find(item=>item.id===repoID)||activeRepository()||{};
  const repository={...selected,branch:(repoID===activeRepoID?currentStatus?.branch:'')||selected.branch||''};
  const path=String(change?.path||'');
  const loadSuggestions=async()=>{
    const query=new URLSearchParams({view:'ignore-suggestions',path});query.set('repo',repoID);
    return app.jsonFetch('/api/git/status?'+query.toString());
  };
  const applyIgnore=item=>action('ignore',{path,ignore_id:item.id},'',{recovery:false,repoID});
  return gitIgnoreWizard.open({
    path,repository,loadSuggestions,apply:applyIgnore,
    refresh:async()=>{if(activeRepoID===repoID){await refresh();if(currentView==='changes')await loadChanges();}}
  });
}

async function loadChanges(){
  const data=await gitView('changes');if(!data)return false;const rows=data.changes||[];content.replaceChildren();if(!rows.length){empty('Working tree clean');return;}
  const conflictState=data.conflict_state&&typeof data.conflict_state==='object'?data.conflict_state:null;
  const openConflictRecovery=()=>openGitRecovery({
    actionName:'merge',
    command:'Resolve Git conflicts',
    error:'Git operation has unresolved conflicts',
    failureCode:'conflicts',
    details:{conflict_state:conflictState||{operation:'merge',files:rows.filter(item=>item.conflicted).map(item=>({path:item.path,status:(item.index_status||'')+(item.worktree_status||'')}))}},
    repoID:activeRepoID
  });
  if(conflictState&&Array.isArray(conflictState.files)&&conflictState.files.length){
    const banner=el('div','git-row git-conflict-banner');
    const main=el('div','git-row-main');
    main.append(el('div','git-row-title','Merge conflicts need resolution'),el('div','git-row-sub',conflictState.files.length+' unmerged path(s) · '+(conflictState.operation||'merge')));
    const actions=el('div','git-row-actions');actions.append(actionButton('Resolve conflicts',openConflictRecovery,'Open the Git conflict recovery wizard'));
    banner.append(el('span','git-row-code','!!'),main,actions);content.append(banner);
  }
  for(const change of rows){
    const row=el('div','git-row'+(change.conflicted?' git-row-conflicted':''));const code=el('span','git-row-code',(change.index_status||' ')+(change.worktree_status||' '));const main=el('div','git-row-main');main.append(el('div','git-row-title',change.path),el('div','git-row-sub',[change.conflicted&&'conflict',change.staged&&!change.conflicted&&'staged',change.unstaged&&!change.conflicted&&'unstaged',change.untracked&&'untracked',change.original_path&&('from '+change.original_path)].filter(Boolean).join(' · ')));const actions=el('div','git-row-actions');
    if(change.conflicted){
      actions.append(actionButton('Resolve',openConflictRecovery,'Open the Git conflict recovery wizard'));
      actions.append(actionButton('Open',()=>{const item=(conflictState?.files||[]).find(file=>file.path===change.path);const path=item?.project_path;if(!path)throw new Error('Project path unavailable for conflicted file');window.dispatchEvent(new CustomEvent('taskmenu:project-file-open-request',{detail:{path,source:'git-conflict-changes'}}));},'Open conflicted working-tree file'));
    }else{
      if(change.unstaged&&!change.untracked)actions.append(actionButton('Diff',()=>showDiff(change.path,'worktree')));if(change.staged)actions.append(actionButton('Staged diff',()=>showDiff(change.path,'staged')));
      if(change.unstaged||change.untracked)actions.append(actionButton('Stage',()=>action('stage',{path:change.path})));if(change.staged)actions.append(actionButton('Unstage',()=>action('unstage',{path:change.path})));if(change.untracked&&change.path!=='.gitignore')actions.append(actionButton('Ignore',()=>openIgnoreWizard(change),'Ignore this untracked path or choose a smart pattern for similar files'));
    }
    actions.append(actionButton('Copy path',()=>copyText(change.path)));row.append(code,main,actions);content.append(row);
  }
}
async function mergeBranch(branch){
  const label='Merge From '+branch.name;
  beginOperation(label,'Checking working tree and local/remote branch revisions…');
  let check;
  try{
    check=await gitView('merge-preflight',{branch:branch.name});if(!check){showOperation(label,'Repository selection changed; merge canceled');return false;}
  }catch(error){
    showOperation(label,'',error?.message||String(error));
    throw error;
  }
  let source=check.default_source||'';
  if(check.requires_choice){
    const localSHA=(check.local_sha||'').slice(0,12);
    const remoteSHA=(check.remote_sha||'').slice(0,12);
    const answer=window.prompt(
      'Local and remote versions differ for '+branch.name+'.\n\n'+
      'LOCAL  : '+check.local_ref+' @ '+localSHA+'\n'+
      'REMOTE : '+check.remote_ref+' @ '+remoteSHA+'\n\n'+
      'Repository: '+(activeRepository()?.name||activeRepoID)+'\n'+
      'Type LOCAL or REMOTE to choose the version merged into '+check.current+':',
      branch.remote?'REMOTE':'LOCAL'
    );
    if(answer===null){showOperation(label,'Merge canceled');return false;}
    source=answer.trim().toLowerCase();
    if(source!=='local'&&source!=='remote'){
      const error=new Error('Merge canceled: enter LOCAL or REMOTE.');
      showOperation(label,'',error.message);throw error;
    }
  }
  const mergeRef=source==='remote'?check.remote_ref:check.local_ref;
  const expectedSHA=source==='remote'?check.remote_sha:check.local_sha;
  if(!mergeRef||!expectedSHA){const error=new Error('Selected merge source is unavailable.');showOperation(label,'',error.message);throw error;}
  if(!window.confirm('Repository: '+(activeRepository()?.name||activeRepoID)+'\nMerge From: '+mergeRef+' @ '+expectedSHA.slice(0,12)+'\nInto current branch: '+check.current+'?')){showOperation(label,'Merge canceled');return false;}
  return action('merge',{branch:branch.name,source,expected_sha:expectedSHA,expected_current:check.current,merge_ref:mergeRef});
}

async function mergeToBranch(branch){
  const label='Merge To '+branch.name;
  const targetSource=branch.remote?'remote':'local';
  beginOperation(label,'Checking source HEAD, target branch, remote and working tree…');
  let check;
  try{
    check=await gitView('merge-to-preflight',{branch:branch.name,target_source:targetSource});
    if(!check){showOperation(label,'Repository selection changed; Merge To canceled');return false;}
  }catch(error){
    showOperation(label,'',error?.message||String(error));
    throw error;
  }

  let allowDirty=false;
  if(check.dirty){
    allowDirty=window.confirm(
      'Repository: '+(activeRepository()?.name||activeRepoID)+'\n\n'+
      'The current working tree has uncommitted changes.\n\n'+
      'Continue Merge To?\n\n'+
      'Only committed HEAD '+check.current+' @ '+check.current_sha.slice(0,12)+' will be merged.\n'+
      'Uncommitted/staged local changes are NOT included and will remain untouched.'
    );
    if(!allowDirty){showOperation(label,'Merge To canceled because the working tree has uncommitted changes.');return false;}
  }

  let allowSlowFallback=false;
  if(check.slow_fallback){
    allowSlowFallback=window.confirm(
      'This Git version does not support the no-checkout Merge To engine.\n\n'+
      'TaskDeck must use a temporary worktree for the target branch. On repositories with many files this can be slower.\n\n'+
      'Continue?'
    );
    if(!allowSlowFallback){showOperation(label,'Merge To canceled before temporary-worktree fallback.');return false;}
  }

  const engine=check.merge_engine==='merge-tree'?'no-checkout merge-tree':check.merge_engine==='fast-forward'?'fast-forward/no-op':'temporary worktree fallback';
  const confirmed=window.confirm(
    'Repository: '+(activeRepository()?.name||activeRepoID)+'\n'+
    'Merge To: '+check.current+' @ '+check.current_sha.slice(0,12)+'\n'+
    'Target: '+check.target_ref+' @ '+check.target_sha.slice(0,12)+'\n'+
    'Push: '+check.push_remote+'/'+check.push_branch+'\n'+
    'Engine: '+engine+'\n\n'+
    'Proceed?'
  );
  if(!confirmed){showOperation(label,'Merge To canceled');return false;}

  return action('merge_to',{
    branch:branch.name,
    target_source:targetSource,
    expected_current:check.current,
    expected_source_sha:check.current_sha,
    expected_target_sha:check.target_sha,
    allow_dirty:allowDirty,
    allow_slow_fallback:allowSlowFallback
  });
}
async function loadBranches(){
  const data=await gitView('branches');if(!data)return false;content.replaceChildren();const create=el('div','git-branch-create');const input=document.createElement('input');input.placeholder='new branch name';const button=actionButton('Create & switch',async()=>{const name=input.value.trim();if(!name)return false;await action('create_branch',{branch:name});input.value='';return true;});create.append(input,button);content.append(create);
  const appendRows=(target,rows)=>{for(const branch of rows||[]){const row=el('div','git-row');const code=el('span','git-row-code',branch.current?'*':'');const main=el('div','git-row-main');main.append(el('div','git-row-title',branch.name),el('div','git-row-sub',branch.upstream||''));const actions=el('div','git-row-actions');if(!branch.remote&&!branch.current)actions.append(actionButton('Switch',()=>action('switch',{branch:branch.name})));if(branch.remote||!branch.current)actions.append(actionButton('Merge From',()=>mergeBranch(branch),'Merge the selected branch into the current branch'),actionButton('Merge To',()=>mergeToBranch(branch),'Merge committed HEAD of the current branch into the selected branch, then push the target'));actions.append(actionButton('Compare',()=>loadCompare(branch.name)),actionButton('Copy',()=>copyText(branch.name)));row.append(code,main,actions);target.append(row);}};
  content.append(el('div','taskmenu-menu-label','LOCAL'));appendRows(content,data.local);
  const remoteRows=Array.isArray(data.remote)?data.remote:[];
  const remoteToggle=el('button','git-branch-section-toggle');remoteToggle.type='button';remoteToggle.setAttribute('aria-expanded','false');
  const remoteList=el('div','git-branch-remote-list');remoteList.hidden=true;appendRows(remoteList,remoteRows);
  const renderRemoteToggle=()=>{const expanded=remoteToggle.getAttribute('aria-expanded')==='true';remoteToggle.textContent=(expanded?'▾ ':'▸ ')+'REMOTE ('+remoteRows.length+')';remoteToggle.title=expanded?'Collapse remote branches':'Expand remote branches';};
  remoteToggle.onclick=()=>{const expanded=remoteToggle.getAttribute('aria-expanded')==='true';remoteToggle.setAttribute('aria-expanded',expanded?'false':'true');remoteList.hidden=expanded;renderRemoteToggle();};
  renderRemoteToggle();content.append(remoteToggle,remoteList);
}
async function loadLog(){const data=await gitView('log',{limit:'50'});if(!data)return false;content.replaceChildren();for(const commit of data.commits||[]){const row=el('div','git-row');const code=el('span','git-row-code',commit.short);const main=el('div','git-row-main');main.append(el('div','git-row-title',commit.subject),el('div','git-row-sub',commit.date+' · '+commit.author));const actions=el('div','git-row-actions');actions.append(actionButton('Copy SHA',()=>copyText(commit.sha)));row.append(code,main,actions);content.append(row);}if(!content.childElementCount)empty('No commits');}
async function loadAheadBehind(){const data=await gitView('ahead-behind');if(!data)return false;content.replaceChildren();content.append(el('div','git-row-sub',data.upstream?`Upstream ${data.upstream} · ahead ${data.ahead} · behind ${data.behind}`:'No upstream configured'));for(const commit of data.commits||[]){const row=el('div','git-row');const code=el('span','git-row-code '+(commit.direction==='ahead'?'git-direction-ahead':'git-direction-behind'),commit.direction==='ahead'?'↑':'↓');const main=el('div','git-row-main');main.append(el('div','git-row-title',commit.subject),el('div','git-row-sub',commit.sha));const actions=el('div','git-row-actions');actions.append(actionButton('Copy SHA',()=>copyText(commit.sha)));row.append(code,main,actions);content.append(row);}}
async function loadStashes(){const data=await gitView('stashes');if(!data)return false;content.replaceChildren();const create=actionButton('Create stash',()=>stashPush());content.append(create);for(const stash of data.stashes||[]){const row=el('div','git-row');const code=el('span','git-row-code',stash.ref);const main=el('div','git-row-main');main.append(el('div','git-row-title',stash.subject),el('div','git-row-sub',stash.when+' · '+stash.sha.slice(0,8)));const actions=el('div','git-row-actions');actions.append(actionButton('Pop',()=>action('stash_pop',{ref:stash.ref},`Pop ${stash.ref}? This may create conflicts if the worktree has changed.`)),actionButton('Copy ref',()=>copyText(stash.ref)));row.append(code,main,actions);content.append(row);}}
async function loadCompare(base=''){
  currentView='compare';updateNav();const branches=await gitView('branches');if(!branches)return false;content.replaceChildren();const controls=el('div','git-compare-controls');const select=document.createElement('select');for(const branch of [...(branches.local||[]),...(branches.remote||[])]){if(branch.current)continue;const o=document.createElement('option');o.value=branch.name;o.textContent=branch.name;select.append(o);}if(base&&[...select.options].some(o=>o.value===base))select.value=base;const run=actionButton('Compare',async()=>{if(!select.value)return false;const data=await gitView('compare',{base:select.value});if(!data)return false;renderCompare(data,controls);return true;});controls.append(select,run);content.append(controls);if(base&&select.value)await run.onclick();
}
function renderCompare(data,controls){content.replaceChildren(controls);content.append(el('strong','',`Compare ${data.base}...HEAD`),el('pre','git-compare-pre',(data.stat||'(no differences)')+'\n'+(data.files||'')));}
async function loadCurrentView(){updateNav();if(currentView==='repositories')return loadRepositories(false);if(!currentStatus?.repository)return empty('Not a Git repository');switch(currentView){case 'changes':return loadChanges();case 'branches':return loadBranches();case 'log':return loadLog();case 'ahead-behind':return loadAheadBehind();case 'stashes':return loadStashes();case 'compare':return loadCompare();}}

repoSelect.onchange=()=>selectRepository(repoSelect.value).catch(app.showError);
repoRescan.onclick=async()=>{try{await refreshRepositories(true);await refresh();await loadCurrentView();}catch(error){app.showError(error);}};
pill.onclick=async()=>{panel.classList.toggle('visible');if(panel.classList.contains('visible')){await refreshRepositories(false);if(app.views.has(String(app.active||'')))await autoSelectRepositoryForTerminal(app.active,{reload:false});await refresh();await loadCurrentView();}};panelClose.onclick=()=>panel.classList.remove('visible');
window.addEventListener('focus',refresh);
window.addEventListener('taskmenu:session',event=>{const meta=event.detail?.meta;if(meta&&meta.status!=='running')setTimeout(refresh,150);});
window.addEventListener('taskmenu:view-activated',event=>{if(event.detail?.kind==='terminal')autoSelectRepositoryForTerminal(event.detail.id).catch(app.showError);});
setInterval(refresh,5000);
refreshRepositories(false).then(async()=>{if(app.views.has(String(app.active||'')))await autoSelectRepositoryForTerminal(app.active,{reload:false});return refresh();}).catch(error=>{console.warn('Git repository discovery failed',error);return refresh();});
updateNav();