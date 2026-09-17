const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for git status');

const style=document.createElement('style');
style.textContent=`
.git-status-pill{display:none;white-space:nowrap;font-family:ui-monospace,monospace;font-size:11px;padding:5px 8px;max-width:360px;overflow:hidden;text-overflow:ellipsis}.git-status-pill.visible{display:block}.git-status-pill.dirty{border-color:#8a6d3b;background:#3a2f1d}.git-status-pill.clean{border-color:#3f6b4a;background:#1f3526}
.git-panel{display:none;position:fixed;z-index:1500;top:58px;right:12px;bottom:12px;width:min(780px,calc(100vw - 24px));border:1px solid #3b414d;border-radius:10px;background:#11161d;box-shadow:0 16px 42px rgba(0,0,0,.48);overflow:hidden}.git-panel.visible{display:flex;flex-direction:column}.git-panel-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #30343b}.git-panel-title{font-weight:700}.git-panel-summary{font:11px ui-monospace,monospace;opacity:.65;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-panel-close{padding:4px 8px}.git-quick-groups{display:flex;gap:7px;flex-wrap:wrap;padding:8px 10px;border-bottom:1px solid #30343b}.git-quick-group{display:flex;align-items:center;gap:4px;padding:4px;border:1px solid #30343b;border-radius:7px;background:#151b23}.git-quick-group strong{font-size:10px;opacity:.55;margin:0 3px}.git-quick-group button{padding:4px 7px;font-size:11px}.git-nav{display:flex;gap:4px;overflow:auto;padding:7px 10px;border-bottom:1px solid #30343b}.git-nav button{padding:5px 8px;font-size:11px;white-space:nowrap}.git-nav button.active{background:#34445a;border-color:#52719a}.git-panel-content{flex:1;min-height:0;overflow:auto;padding:10px}.git-empty{opacity:.6;padding:18px;text-align:center}.git-row{display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:7px;align-items:center;padding:7px 5px;border-bottom:1px solid #252c35}.git-row:last-child{border-bottom:0}.git-row-code{font:11px ui-monospace,monospace;min-width:28px;opacity:.75}.git-row-main{min-width:0}.git-row-title{font:12px ui-monospace,monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-row-sub{font-size:10px;opacity:.55;margin-top:2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-row-actions{display:flex;gap:4px;flex-wrap:wrap;justify-content:flex-end}.git-row-actions button{padding:3px 6px;font-size:10px}.git-diff-pre,.git-operation-output,.git-compare-pre{white-space:pre-wrap;word-break:break-word;font:11px/1.45 ui-monospace,monospace;background:#090d12;border:1px solid #30343b;border-radius:7px;padding:9px;margin:8px 0;overflow:auto}.git-operation{border-top:1px solid #30343b;padding:7px 10px;max-height:160px;overflow:auto}.git-operation-head{display:flex;align-items:center;gap:6px}.git-operation-command{flex:1;min-width:0;font:10px ui-monospace,monospace;opacity:.7;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-operation button{padding:3px 6px;font-size:10px}.git-operation-output{margin:5px 0 0;max-height:90px}.git-branch-create,.git-compare-controls{display:flex;gap:6px;align-items:center;margin-bottom:8px}.git-branch-create input,.git-compare-controls select{flex:1;min-width:0;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:6px 8px}.git-direction-ahead{color:#72cf8a}.git-direction-behind{color:#e5b85c}
html[data-taskmenu-theme="light"] .git-panel{background:#fff;border-color:#b9c0c8;box-shadow:0 16px 42px rgba(0,0,0,.18)}html[data-taskmenu-theme="light"] .git-panel-head,html[data-taskmenu-theme="light"] .git-quick-groups,html[data-taskmenu-theme="light"] .git-nav,html[data-taskmenu-theme="light"] .git-operation{border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-quick-group{background:#f6f8fa;border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-diff-pre,html[data-taskmenu-theme="light"] .git-operation-output,html[data-taskmenu-theme="light"] .git-compare-pre{background:#f6f8fa;border-color:#d0d7de;color:#202124}
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
const quickGroups=document.createElement('div');quickGroups.className='git-quick-groups';
const nav=document.createElement('div');nav.className='git-nav';
const content=document.createElement('div');content.className='git-panel-content';
const operation=document.createElement('div');operation.className='git-operation';operation.hidden=true;
const operationHead=document.createElement('div');operationHead.className='git-operation-head';
const operationCommand=document.createElement('span');operationCommand.className='git-operation-command';
const operationCopy=document.createElement('button');operationCopy.textContent='Copy command';
const operationClear=document.createElement('button');operationClear.textContent='Clear';
const operationOutput=document.createElement('pre');operationOutput.className='git-operation-output';
operationHead.append(operationCommand,operationCopy,operationClear);operation.append(operationHead,operationOutput);
panel.append(panelHead,quickGroups,nav,content,operation);document.body.append(panel);

let currentStatus=null,currentView='changes',refreshing=false,lastCommand='';
function el(tag,className,text){const node=document.createElement(tag);if(className)node.className=className;if(text!==undefined)node.textContent=text;return node;}
function q(value){value=String(value??'');return /^[A-Za-z0-9_./:@+\-]+$/.test(value)?value:"'"+value.replace(/'/g,"'\\''")+"'";}
function actionCommand(action,payload={}){
  switch(action){
    case 'fetch':return 'git fetch --prune';case 'pull':return 'git pull --ff-only';case 'push':return 'git push';case 'stage_all':return 'git add -A';
    case 'stage':return 'git add -- '+q(payload.path);case 'unstage':return 'git restore --staged -- '+q(payload.path);case 'commit':return 'git commit -m '+q(payload.message);
    case 'switch':return 'git switch '+q(payload.branch);case 'create_branch':return 'git switch -c '+q(payload.branch);case 'stash_push':return 'git stash push -u -m '+q(payload.message||'(auto)');case 'stash_pop':return 'git stash pop'+(payload.ref?' '+q(payload.ref):'');default:return 'git '+action;
  }
}
async function copyText(text){
  if(navigator.clipboard&&window.isSecureContext){try{await navigator.clipboard.writeText(text);return;}catch{}}
  const area=document.createElement('textarea');area.value=text;area.setAttribute('readonly','');area.style.position='fixed';area.style.left='-9999px';document.body.append(area);area.select();try{document.execCommand('copy');}finally{area.remove();}
}
function showOperation(command,output,error=''){
  lastCommand=command||'';operation.hidden=false;operationCommand.textContent=command||'Git';operationOutput.textContent=[error&&('ERROR: '+error),output].filter(Boolean).join('\n')||'(no output)';
}
operationCopy.onclick=()=>copyText(lastCommand).catch(app.showError);operationClear.onclick=()=>{operation.hidden=true;operationOutput.textContent='';lastCommand='';};

function renderStatus(data){
  currentStatus=data;
  if(!data?.repository){pill.className='git-status-pill';pill.textContent='';panel.classList.remove('visible');return;}
  const parts=['Git:',data.branch||'(unknown)',data.head||'--------'];parts.push(data.changed?data.changed+' changed':'clean');if(data.ahead)parts.push('↑'+data.ahead);if(data.behind)parts.push('↓'+data.behind);
  const text=parts.join(' · ');pill.textContent=text;panelSummary.textContent=text;pill.className='git-status-pill visible '+(data.changed?'dirty':'clean');pill.title='Branch: '+(data.branch||'unknown')+'\nHEAD: '+(data.head||'unknown')+'\nChanged: '+(data.changed||0)+'\nAhead: '+(data.ahead||0)+'\nBehind: '+(data.behind||0)+'\nClick to open Git Quick Actions';
}
async function refresh(){
  if(refreshing)return;refreshing=true;
  try{renderStatus(await app.jsonFetch('/api/git/status'));}
  catch(e){pill.className='git-status-pill';console.warn('Git status refresh failed',e);}
  finally{refreshing=false;}
}
async function gitView(view,params={}){const query=new URLSearchParams({view,...params});return app.jsonFetch('/api/git/status?'+query.toString());}
async function action(actionName,payload={},confirmText=''){
  if(confirmText&&!window.confirm(confirmText))return null;
  const command=actionCommand(actionName,payload);
  const data=await app.jsonFetch('/api/git/status',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:actionName,...payload})});
  showOperation(command,data.output||'',data.ok?'':(data.error||'Git action failed'));
  if(!data.ok)throw new Error(data.error||'Git action failed');
  await refresh();await loadCurrentView();return data;
}

function quickGroup(label,buttons){const group=el('div','git-quick-group');group.append(el('strong','',label));for(const spec of buttons){const b=el('button','',spec.label);b.title=spec.title||spec.label;b.onclick=()=>Promise.resolve(spec.run()).catch(app.showError);group.append(b);}quickGroups.append(group);}
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

async function commit(pushAfter){const message=window.prompt('Commit message:','');if(message===null||!message.trim())return;await action('commit',{message:message.trim()});if(pushAfter)await action('push');}
async function stashPush(){const message=window.prompt('Stash message (leave blank to use the default):','');if(message===null)return;await action('stash_push',{message:message.trim()});}

const views=[['changes','Changes'],['branches','Branches'],['log','Log'],['ahead-behind','Ahead / Behind'],['stashes','Stash'],['compare','Compare']];
for(const [id,label] of views){const b=el('button','',label);b.dataset.gitView=id;b.onclick=()=>{currentView=id;updateNav();loadCurrentView().catch(app.showError);};nav.append(b);}
function updateNav(){for(const b of nav.querySelectorAll('button'))b.classList.toggle('active',b.dataset.gitView===currentView);}
function empty(message){content.replaceChildren(el('div','git-empty',message));}
function actionButton(label,run,title=''){const b=el('button','',label);if(title)b.title=title;b.onclick=()=>Promise.resolve(run()).catch(app.showError);return b;}

async function showDiff(path,mode){
  const data=await gitView('diff',{path,mode});content.replaceChildren();
  const back=actionButton('← Changes',()=>{currentView='changes';updateNav();return loadChanges();});const title=el('strong','',`${mode==='staged'?'STAGED ':'DIFF '}${path}`);const pre=el('pre','git-diff-pre',data.diff||'(no tracked diff; untracked files can be staged directly)');content.append(back,title,pre);
}
async function loadChanges(){
  const data=await gitView('changes');const rows=data.changes||[];content.replaceChildren();if(!rows.length){empty('Working tree clean');return;}
  for(const change of rows){
    const row=el('div','git-row');const code=el('span','git-row-code',(change.index_status||' ')+(change.worktree_status||' '));const main=el('div','git-row-main');main.append(el('div','git-row-title',change.path),el('div','git-row-sub',[change.staged&&'staged',change.unstaged&&'unstaged',change.untracked&&'untracked',change.original_path&&('from '+change.original_path)].filter(Boolean).join(' · ')));const actions=el('div','git-row-actions');
    if(change.unstaged&&!change.untracked)actions.append(actionButton('Diff',()=>showDiff(change.path,'worktree')));if(change.staged)actions.append(actionButton('Staged diff',()=>showDiff(change.path,'staged')));
    if(change.unstaged||change.untracked)actions.append(actionButton('Stage',()=>action('stage',{path:change.path})));if(change.staged)actions.append(actionButton('Unstage',()=>action('unstage',{path:change.path})));actions.append(actionButton('Copy path',()=>copyText(change.path)));row.append(code,main,actions);content.append(row);
  }
}
async function loadBranches(){
  const data=await gitView('branches');content.replaceChildren();const create=el('div','git-branch-create');const input=document.createElement('input');input.placeholder='new branch name';const button=actionButton('Create & switch',async()=>{const name=input.value.trim();if(!name)return;await action('create_branch',{branch:name});input.value='';});create.append(input,button);content.append(create);
  const addRows=(label,rows)=>{content.append(el('div','taskmenu-menu-label',label));for(const branch of rows||[]){const row=el('div','git-row');const code=el('span','git-row-code',branch.current?'*':'');const main=el('div','git-row-main');main.append(el('div','git-row-title',branch.name),el('div','git-row-sub',branch.upstream||''));const actions=el('div','git-row-actions');if(!branch.remote&&!branch.current)actions.append(actionButton('Switch',()=>action('switch',{branch:branch.name})));actions.append(actionButton('Compare',()=>loadCompare(branch.name)),actionButton('Copy',()=>copyText(branch.name)));row.append(code,main,actions);content.append(row);}};
  addRows('LOCAL',data.local);addRows('REMOTE',data.remote);
}
async function loadLog(){const data=await gitView('log',{limit:'50'});content.replaceChildren();for(const commit of data.commits||[]){const row=el('div','git-row');const code=el('span','git-row-code',commit.short);const main=el('div','git-row-main');main.append(el('div','git-row-title',commit.subject),el('div','git-row-sub',commit.date+' · '+commit.author));const actions=el('div','git-row-actions');actions.append(actionButton('Copy SHA',()=>copyText(commit.sha)));row.append(code,main,actions);content.append(row);}if(!content.childElementCount)empty('No commits');}
async function loadAheadBehind(){const data=await gitView('ahead-behind');content.replaceChildren();content.append(el('div','git-row-sub',data.upstream?`Upstream ${data.upstream} · ahead ${data.ahead} · behind ${data.behind}`:'No upstream configured'));for(const commit of data.commits||[]){const row=el('div','git-row');const code=el('span','git-row-code '+(commit.direction==='ahead'?'git-direction-ahead':'git-direction-behind'),commit.direction==='ahead'?'↑':'↓');const main=el('div','git-row-main');main.append(el('div','git-row-title',commit.subject),el('div','git-row-sub',commit.sha));const actions=el('div','git-row-actions');actions.append(actionButton('Copy SHA',()=>copyText(commit.sha)));row.append(code,main,actions);content.append(row);}}
async function loadStashes(){const data=await gitView('stashes');content.replaceChildren();const create=actionButton('Create stash',()=>stashPush());content.append(create);for(const stash of data.stashes||[]){const row=el('div','git-row');const code=el('span','git-row-code',stash.ref);const main=el('div','git-row-main');main.append(el('div','git-row-title',stash.subject),el('div','git-row-sub',stash.when+' · '+stash.sha.slice(0,8)));const actions=el('div','git-row-actions');actions.append(actionButton('Pop',()=>action('stash_pop',{ref:stash.ref},`Pop ${stash.ref}? This may create conflicts if the worktree has changed.`)),actionButton('Copy ref',()=>copyText(stash.ref)));row.append(code,main,actions);content.append(row);}}
async function loadCompare(base=''){
  currentView='compare';updateNav();const branches=await gitView('branches');content.replaceChildren();const controls=el('div','git-compare-controls');const select=document.createElement('select');for(const branch of [...(branches.local||[]),...(branches.remote||[])]){if(branch.current)continue;const o=document.createElement('option');o.value=branch.name;o.textContent=branch.name;select.append(o);}if(base&&[...select.options].some(o=>o.value===base))select.value=base;const run=actionButton('Compare',async()=>{if(!select.value)return;const data=await gitView('compare',{base:select.value});renderCompare(data,controls);});controls.append(select,run);content.append(controls);if(base&&select.value)await run.onclick();
}
function renderCompare(data,controls){content.replaceChildren(controls);content.append(el('strong','',`Compare ${data.base}...HEAD`),el('pre','git-compare-pre',(data.stat||'(no differences)')+'\n'+(data.files||'')));}
async function loadCurrentView(){updateNav();if(!currentStatus?.repository)return empty('Not a Git repository');switch(currentView){case 'changes':return loadChanges();case 'branches':return loadBranches();case 'log':return loadLog();case 'ahead-behind':return loadAheadBehind();case 'stashes':return loadStashes();case 'compare':return loadCompare();}}

pill.onclick=async()=>{panel.classList.toggle('visible');if(panel.classList.contains('visible')){await refresh();await loadCurrentView();}};panelClose.onclick=()=>panel.classList.remove('visible');
window.addEventListener('focus',refresh);window.addEventListener('taskmenu:session',event=>{const meta=event.detail?.meta;if(meta&&meta.status!=='running')setTimeout(refresh,150);});setInterval(refresh,5000);refresh();updateNav();