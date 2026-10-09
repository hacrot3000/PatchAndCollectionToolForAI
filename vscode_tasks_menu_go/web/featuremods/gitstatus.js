const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for git status');
const gitRecovery=globalThis.TaskDeckGitRecovery;
const gitIgnoreWizard=globalThis.TaskDeckGitIgnoreWizard;
const gitBranchChoiceWizard=globalThis.TaskDeckGitBranchChoiceWizard;

const style=document.createElement('style');
style.textContent=`
.git-status-pill{display:none;white-space:nowrap;font-family:ui-monospace,monospace;font-size:11px;padding:5px 8px;max-width:360px;overflow:hidden;text-overflow:ellipsis}.git-status-pill.visible{display:block}.git-status-pill.dirty{border-color:#8a6d3b;background:#3a2f1d}.git-status-pill.clean{border-color:#3f6b4a;background:#1f3526}
.git-panel{display:none;position:fixed;z-index:2100;top:calc(var(--taskmenu-header-height,30px) + 6px);left:12px;right:auto;bottom:12px;width:min(1080px,calc(100vw - 24px));border:1px solid #3b414d;border-radius:10px;background:#11161d;box-shadow:0 16px 42px rgba(0,0,0,.48);overflow:hidden}
body.task-sidebar-auto-hide .git-panel{left:60px;width:min(1080px,calc(100vw - 72px))}.git-panel.visible{display:flex;flex-direction:column}.git-panel-head{display:flex;align-items:center;gap:8px;padding:9px 11px;border-bottom:1px solid #30343b}.git-panel-title{font-weight:700}.git-panel-summary{font:11px ui-monospace,monospace;opacity:.65;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-panel-close{padding:4px 8px}.git-repo-bar{display:flex;align-items:center;gap:7px;padding:7px 10px;border-bottom:1px solid #30343b}.git-repo-bar label{font-size:10px;font-weight:700;opacity:.6}.git-repo-select{min-width:180px;max-width:310px;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:5px 7px}.git-repo-path{flex:1;min-width:0;font:10px ui-monospace,monospace;opacity:.6;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-repo-rescan{padding:4px 7px;font-size:10px}.git-quick-groups{display:flex;gap:7px;flex-wrap:wrap;padding:8px 10px;border-bottom:1px solid #30343b}.git-quick-group{display:flex;align-items:center;gap:4px;padding:4px;border:1px solid #30343b;border-radius:7px;background:#151b23}.git-quick-group strong{font-size:10px;opacity:.55;margin:0 3px}.git-quick-group button{padding:4px 7px;font-size:11px}.git-action-running{background:#34445a!important;border-color:#6f91bb!important;box-shadow:0 0 0 1px rgba(111,145,187,.35) inset}.git-action-success{background:#21452d!important;border-color:#4f8d61!important}.git-action-error{background:#51262b!important;border-color:#9b5059!important}.git-action-running,.git-action-success,.git-action-error{transition:background .12s ease,border-color .12s ease,transform .12s ease}.git-action-running{transform:translateY(1px)}.git-nav{display:flex;gap:4px;overflow:auto;padding:7px 10px;border-bottom:1px solid #30343b}.git-nav button{padding:5px 8px;font-size:11px;white-space:nowrap}.git-nav button.active{background:#34445a;border-color:#52719a}.git-panel-content{flex:1;min-height:0;overflow:auto;padding:10px}.git-empty{opacity:.6;padding:18px;text-align:center}.git-row{display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:7px;align-items:center;padding:7px 5px;border-bottom:1px solid #252c35}.git-row:last-child{border-bottom:0}.git-row-code{font:11px ui-monospace,monospace;min-width:28px;opacity:.75}.git-row-main{min-width:0}.git-row-title{font:12px ui-monospace,monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-row-sub{font-size:10px;opacity:.55;margin-top:2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-row-actions{display:flex;gap:4px;flex-wrap:wrap;justify-content:flex-end}.git-row-actions button{padding:3px 6px;font-size:10px}.git-diff-pre,.git-operation-output,.git-compare-pre{white-space:pre-wrap;word-break:break-word;font:11px/1.45 ui-monospace,monospace;background:#090d12;border:1px solid #30343b;border-radius:7px;padding:9px;margin:8px 0;overflow:auto}.git-hunk{border:1px solid #30343b;border-radius:8px;margin:8px 0;overflow:hidden}.git-hunk-head{display:flex;align-items:center;gap:7px;padding:6px 8px;background:#151b23}.git-hunk-title{flex:1;min-width:0;font:10px ui-monospace,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.git-hunk-actions{display:flex;gap:5px}.git-hunk-actions button{padding:3px 6px;font-size:10px}.git-hunk pre{margin:0;border:0;border-radius:0;border-top:1px solid #30343b;max-height:300px}.git-panel.git-panel-wide{width:min(1320px,calc(100vw - 24px))}body.task-sidebar-auto-hide .git-panel.git-panel-wide{width:min(1320px,calc(100vw - 72px))}.git-diff-mode-bar{display:flex;gap:5px;flex-wrap:wrap;align-items:center;margin:8px 0}.git-diff-mode-bar button{padding:4px 7px;font-size:10px}.git-diff-mode-bar button.active{background:#34445a;border-color:#6f91bb}.git-diff-preview-scroll{width:100%;min-width:0;overflow-x:auto;overflow-y:visible;scrollbar-width:auto}
.git-diff-preview-scroll .git-hunk{width:max-content;min-width:100%;box-sizing:border-box}
.git-diff-preview-scroll .git-hunk-head{position:sticky;left:0;z-index:1}
.git-visual-diff{overflow:visible;border-top:1px solid #30343b;background:#0b0f14}
.git-visual-grid{display:grid;grid-template-columns:repeat(2,minmax(max-content,1fr));min-width:100%;width:max-content}
.git-diff-columns-head,.git-diff-visual-row{display:contents}
.git-diff-column-head{padding:5px 8px;font:10px ui-monospace,monospace;font-weight:700;background:#151b23;border-bottom:1px solid #30343b}
.git-diff-column-head:nth-child(2){border-left:1px solid #30343b}
.git-diff-cell{display:grid;grid-template-columns:48px max-content;min-width:max-content;border-bottom:1px solid rgba(255,255,255,.035)}
.git-diff-cell:nth-child(2){border-left:1px solid #30343b}
.git-diff-line-no{padding:1px 7px;text-align:right;user-select:none;opacity:.48;font:10px/1.45 ui-monospace,monospace;border-right:1px solid rgba(255,255,255,.06)}
.git-diff-code{display:block;padding:1px 7px;white-space:pre;overflow:visible;font:11px/1.45 ui-monospace,monospace}
.git-diff-cell.removed.important{background:rgba(229,72,86,.28)}
.git-diff-cell.added.important{background:rgba(232,174,55,.28)}
.git-diff-cell.removed.unimportant,.git-diff-cell.added.unimportant{background:rgba(58,149,214,.23)}
.git-diff-cell.blank{opacity:.36}
.git-diff-note{grid-column:1/3;padding:3px 8px;font:10px ui-monospace,monospace;opacity:.62;border-bottom:1px solid #252c35}
html[data-taskmenu-theme="light"] .git-diff-cell.removed.important{background:#ffe2e5}
html[data-taskmenu-theme="light"] .git-diff-cell.added.important{background:#fff1c9}
html[data-taskmenu-theme="light"] .git-diff-cell.removed.unimportant,html[data-taskmenu-theme="light"] .git-diff-cell.added.unimportant{background:#e3f3ff}
.git-diff-raw{border-top:1px solid #30343b}.git-diff-raw>summary{cursor:pointer;padding:5px 8px;font-size:10px;opacity:.68}.git-diff-raw>pre{max-height:260px}.git-operation{border-top:1px solid #30343b;padding:7px 10px;max-height:min(46vh,460px);overflow:auto}.git-operation.running{box-shadow:inset 3px 0 0 #6f91bb}.git-operation-head{display:flex;align-items:center;gap:6px}.git-operation-command{flex:1;min-width:0;font:10px ui-monospace,monospace;opacity:.7;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-operation button{padding:3px 6px;font-size:10px}.git-operation-output{margin:5px 0 0;max-height:min(38vh,360px)}.git-branch-create,.git-compare-controls,.git-file-controls{display:flex;gap:6px;align-items:center;margin-bottom:8px}.git-branch-create input,.git-compare-controls select,.git-file-controls input{flex:1;min-width:0;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:6px 8px}.git-branch-section-toggle{display:block;width:100%;text-align:left;border:0;background:transparent;color:inherit;padding:7px 0 5px;font-size:10px;font-weight:700;opacity:.72}.git-branch-section-toggle:hover{opacity:1}.git-branch-section-toggle:focus-visible{outline:1px solid #52719a;outline-offset:2px}.git-direction-ahead{color:#72cf8a}.git-direction-behind{color:#e5b85c}
.git-graph-controls{display:grid;grid-template-columns:repeat(3,minmax(120px,1fr));gap:6px;margin-bottom:8px}.git-graph-controls input{min-width:0;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:5px 7px;font-size:10px}.git-graph-controls-actions{grid-column:1/-1;display:flex;gap:6px}.git-graph-shell{display:grid;grid-template-columns:minmax(430px,1.2fr) minmax(300px,.8fr);gap:8px;min-height:0;height:100%}.git-graph-list,.git-graph-detail{min-height:0;overflow:auto;border:1px solid #30343b;border-radius:8px}.git-graph-row{display:grid;grid-template-columns:auto minmax(0,1fr);gap:5px;align-items:center;padding:4px 5px;border-bottom:1px solid #252c35;cursor:pointer}.git-graph-row:hover,.git-graph-row.selected{background:#293241}.git-graph-svg{display:block;overflow:visible}.git-graph-main{min-width:0}.git-graph-title{font:11px ui-monospace,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.git-graph-sub{font-size:9px;opacity:.55;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.git-graph-refs{display:flex;gap:3px;flex-wrap:wrap;margin-top:2px}.git-graph-ref{font:9px ui-monospace,monospace;padding:1px 4px;border:1px solid #46505d;border-radius:8px;opacity:.82}.git-graph-ref.head{font-weight:700}.git-graph-ref.tag{border-style:dashed}.git-graph-detail{padding:8px}.git-graph-detail-head{display:flex;align-items:flex-start;gap:6px;flex-wrap:wrap}.git-graph-detail-title{flex:1;min-width:180px}.git-graph-detail-actions{display:flex;gap:4px;flex-wrap:wrap}.git-graph-file{display:grid;grid-template-columns:34px minmax(0,1fr) auto;gap:6px;align-items:center;padding:6px 2px;border-top:1px solid #252c35}.git-graph-file-path{font:10px ui-monospace,monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.git-graph-context{position:fixed;z-index:17010;min-width:230px;padding:5px;border:1px solid #48515f;border-radius:8px;background:#171b22;box-shadow:0 14px 38px rgba(0,0,0,.45)}.git-graph-context button{display:block;width:100%;border:0;background:transparent;color:inherit;text-align:left;padding:6px 8px;border-radius:4px;font-size:11px}.git-graph-context button:hover{background:#2b3440}.git-graph-context .danger{color:#ff9a9a}
.git-recovery-lost{border:1px solid #30343b;border-radius:8px;padding:7px;margin-bottom:8px}.git-recovery-lost-head{display:flex;align-items:center;gap:7px}.git-recovery-lost-head strong{flex:1;font-size:11px}.git-recovery-lost-note{font-size:10px;opacity:.62;margin-top:4px}.git-recovery-lost-list{margin-top:6px;max-height:240px;overflow:auto}
.git-rebase-controls{display:flex;gap:6px;align-items:center;flex-wrap:wrap;margin-bottom:8px}.git-rebase-controls input,.git-rebase-controls select{min-width:180px;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:5px 7px}.git-rebase-warning{padding:7px 9px;border:1px solid #8a6d3b;border-radius:7px;background:#3a2f1d;font-size:10px;margin:6px 0}.git-rebase-shell{display:grid;grid-template-columns:minmax(460px,1.15fr) minmax(300px,.85fr);gap:8px;min-height:0}.git-rebase-plan,.git-rebase-preview{border:1px solid #30343b;border-radius:8px;overflow:auto;min-height:120px}.git-rebase-item{display:grid;grid-template-columns:22px 92px minmax(0,1fr);gap:6px;align-items:center;padding:6px;border-bottom:1px solid #252c35}.git-rebase-item.dragging{opacity:.45}.git-rebase-drag{cursor:grab;text-align:center;opacity:.58;user-select:none}.git-rebase-action{min-width:0;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:4px}.git-rebase-main{min-width:0}.git-rebase-message{width:100%;margin-top:4px;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:4px 6px;font:10px ui-monospace,monospace}.git-rebase-preview-row{padding:6px 8px;border-bottom:1px solid #252c35}.git-rebase-preview-row strong{font:11px ui-monospace,monospace}.git-rebase-preview-row div{font-size:9px;opacity:.6;margin-top:2px}
html[data-taskmenu-theme="light"] .git-graph-controls input{background:#fff;color:#202124;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .git-graph-list,html[data-taskmenu-theme="light"] .git-graph-detail{border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-rebase-controls input,html[data-taskmenu-theme="light"] .git-rebase-controls select,html[data-taskmenu-theme="light"] .git-rebase-action,html[data-taskmenu-theme="light"] .git-rebase-message{background:#fff;color:#202124;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .git-rebase-plan,html[data-taskmenu-theme="light"] .git-rebase-preview{border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-rebase-warning{background:#fff8e6;color:#202124}html[data-taskmenu-theme="light"] .git-recovery-lost{border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-graph-context{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .git-graph-context button:hover{background:#edf2f7}html[data-taskmenu-theme="light"] .git-panel{background:#fff;border-color:#b9c0c8;box-shadow:0 16px 42px rgba(0,0,0,.18)}html[data-taskmenu-theme="light"] .git-panel-head,html[data-taskmenu-theme="light"] .git-repo-bar,html[data-taskmenu-theme="light"] .git-quick-groups,html[data-taskmenu-theme="light"] .git-nav,html[data-taskmenu-theme="light"] .git-operation{border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-quick-group{background:#f6f8fa;border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-repo-select{background:#fff;border-color:#b9c0c8;color:#202124}html[data-taskmenu-theme="light"] .git-diff-pre,html[data-taskmenu-theme="light"] .git-operation-output,html[data-taskmenu-theme="light"] .git-compare-pre{background:#f6f8fa;border-color:#d0d7de;color:#202124}html[data-taskmenu-theme="light"] .git-hunk{border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-hunk-head{background:#f6f8fa}html[data-taskmenu-theme="light"] .git-hunk pre{border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-visual-diff{background:#fff;border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-diff-columns-head{background:#f6f8fa;border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-diff-column-head+.git-diff-column-head,html[data-taskmenu-theme="light"] .git-diff-cell+.git-diff-cell{border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-diff-cell.removed{background:#fff0f0}html[data-taskmenu-theme="light"] .git-diff-cell.added{background:#effaf1}html[data-taskmenu-theme="light"] .git-diff-line-no{border-color:#e2e6eb}
`;
document.head.append(style);

const workspace=document.querySelector('#workspace');
function canViewGitPanel(){return !app.sharedMode||Boolean(app.hasPermission?.('git.status')||app.hasPermission?.('project.admin'));}
const pill=document.createElement('button');pill.className='git-status-pill';pill.title='Git — click to open Git Quick Actions';workspace.after(pill);
function showGitFallback(message,title){
  if(!canViewGitPanel())return;
  pill.className='git-status-pill visible';
  pill.textContent=message;
  pill.title=title||'Open Git Quick Actions to select or rescan a repository';
  panelSummary.textContent=message;
}


const panel=document.createElement('div');panel.className='git-panel';
const panelHead=document.createElement('div');panelHead.className='git-panel-head';
const panelTitle=document.createElement('span');panelTitle.className='git-panel-title';panelTitle.textContent='Git Quick Actions';
const panelSummary=document.createElement('span');panelSummary.className='git-panel-summary';
if(canViewGitPanel())showGitFallback('Git','Open Git Quick Actions');
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
let gitScanEnabled=true,gitScanDepth=8,gitScanWarnings=[];
let gitFilePath='',gitFileMode='history',gitFileCompareRef='';
let gitGraphFilters={search:'',author:'',message:'',since:'',until:'',path:''},gitGraphSelectedSHA='';
let gitReflogFilters={search:'',ref:'',kind:''},gitReflogSelected='';
let gitRebaseBase='',gitRebasePlanState=[];
let activeGitJob=null;
const runningActions=new Map();
function el(tag,className,text){const node=document.createElement(tag);if(className)node.className=className;if(text!==undefined)node.textContent=text;return node;}
function q(value){value=String(value??'');return /^[A-Za-z0-9_./:@+\-]+$/.test(value)?value:"'"+value.replace(/'/g,"'\\''")+"'";}
function actionCommand(action,payload={}){
  switch(action){
    case 'interactive_rebase':return 'git rebase -i '+q(payload.ref);case 'restore_file_commit':return 'git restore --source='+q(payload.ref)+' --worktree -- '+q(payload.path);case 'restore_staged_commit':return 'git restore --source='+q(payload.ref)+' --staged -- '+q(payload.path);case 'fetch':return 'git fetch --prune';case 'pull':return 'git pull --ff-only';case 'push':return 'git push';case 'stage_all':return 'git add -A';
    case 'stage':return 'git add -- '+q(payload.path);case 'unstage':return 'git restore --staged -- '+q(payload.path);case 'stage_hunk':return 'Stage hunk '+String((payload.hunk_index??0)+1)+' · '+q(payload.path);case 'unstage_hunk':return 'Unstage hunk '+String((payload.hunk_index??0)+1)+' · '+q(payload.path);case 'discard_hunk':return 'Discard hunk '+String((payload.hunk_index??0)+1)+' · '+q(payload.path);case 'ignore':return 'Add .gitignore rule for '+q(payload.path);case 'commit':return 'git commit -m '+q(payload.message);
    case 'switch':return 'git switch '+q(payload.branch);case 'create_branch':return 'git switch -c '+q(payload.branch);case 'checkout_commit':return 'git switch --detach '+q(payload.ref);case 'create_branch_at':return 'git switch -c '+q(payload.branch)+' '+q(payload.ref);case 'create_branch_ref':return 'git branch '+q(payload.branch)+' '+q(payload.ref);case 'reset_commit':return 'git reset --'+q(payload.mode)+' '+q(payload.ref);case 'worktree_add_branch':return 'git worktree add '+q(payload.directory_name)+' '+q(payload.branch);case 'worktree_add_new_branch':return 'git worktree add -b '+q(payload.branch)+' '+q(payload.directory_name)+' '+q(payload.ref||'HEAD');case 'worktree_remove':return 'git worktree remove <selected worktree>';case 'worktree_open':return 'taskdeck --workspace <selected worktree>';case 'worktree_prune':return 'git worktree prune --verbose --expire now';case 'submodule_init':return 'git submodule update --init --checkout <selected submodule>';case 'submodule_update':return 'git submodule update --init --checkout <selected submodule>';case 'submodule_checkout_expected':return 'git submodule update --init --checkout <selected submodule>';case 'submodule_update_recursive':return 'git submodule update --init --recursive --checkout';case 'submodule_sync':return 'git submodule sync --recursive';case 'delete_branch':return 'git branch -d '+q(payload.branch);case 'force_delete_branch':return 'git branch -D '+q(payload.branch);case 'delete_remote_tracking':return 'git branch -dr '+q(payload.branch);case 'delete_remote_branch':return 'git push '+q(String(payload.branch||'').split('/')[0])+' --delete '+q(String(payload.branch||'').split('/').slice(1).join('/'));case 'merge':return 'git merge --no-edit '+q(payload.merge_ref||payload.branch);case 'merge_to':return 'Merge To '+q(payload.expected_current)+' -> '+q(payload.branch)+' and push';case 'stash_push':return 'git stash push -u -m '+q(payload.message||'(auto)');case 'stash_pop':return 'git stash pop'+(payload.ref?' '+q(payload.ref):'');default:return 'git '+action;
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
function gitUIAsyncAction(action){return ['fetch','pull','push','merge','delete_remote_branch','submodule_init','submodule_update','submodule_checkout_expected','submodule_update_recursive','submodule_sync'].includes(String(action||''));}
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
  gitScanEnabled=data.scan_enabled!==false;
  gitScanDepth=Number(data.scan_depth||8);
  gitScanWarnings=Array.isArray(data.scan_warnings)?data.scan_warnings:[];
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
  if(!data?.repository){showGitFallback('Git · No repository','No Git repository detected. Click to open Git Quick Actions and rescan.');return;}
  const parts=['Git:',data.repo_name||activeRepository()?.name||data.repo_id||'repo',data.branch||'(unknown)',data.head||'--------'];parts.push(data.changed?data.changed+' changed':'clean');if(data.ahead)parts.push('↑'+data.ahead);if(data.behind)parts.push('↓'+data.behind);
  const text=parts.join(' · ');pill.textContent=text;panelSummary.textContent=text;pill.className='git-status-pill visible '+(data.changed?'dirty':'clean');pill.title='Repository: '+(data.repo_name||data.repo_id||'unknown')+'\nPath: '+(data.repo_path||'')+'\nBranch: '+(data.branch||'unknown')+'\nHEAD: '+(data.head||'unknown')+'\nChanged: '+(data.changed||0)+'\nAhead: '+(data.ahead||0)+'\nBehind: '+(data.behind||0)+'\nClick to open Git Quick Actions';
}
async function refresh(){
  if(!canViewGitPanel())return;
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
    window.dispatchEvent(new CustomEvent('taskmenu:git-status-refreshed',{detail:{repo_id:activeRepoID,changed:Number(data?.changed||0)}}));
    const item=repositories.find(repo=>repo.id===activeRepoID);
    if(item&&data?.repository)Object.assign(item,{branch:data.branch,head:data.head,changed:data.changed,ahead:data.ahead,behind:data.behind});
    renderRepositorySelector();
  }
  catch(e){if(seq===refreshSeq){showGitFallback('Git · Unavailable','Git status request failed: '+String(e?.message||e)+'\nClick to open Git Quick Actions');console.warn('Git status refresh failed',e);}}
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
    openFile:path=>window.dispatchEvent(new CustomEvent('taskmenu:project-file-open-request',{detail:{path,source:'git-conflict-recovery'}})),
    openMergeEditor:path=>{
      const editor=globalThis.TaskMenuGitMergeEditor;if(!editor?.open)throw new Error('Git 3-way merge editor unavailable');
      return editor.open({repoID,path});
    }
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
function gitPermissionAllowed(permission){
  return !app.sharedMode||Boolean(app.hasPermission?.(permission)||app.hasPermission?.('project.admin'));
}
function requiredGitPermissions(actionName){
  const permissions=['git.write'];
  if(actionName==='push'||actionName==='merge_to')permissions.push('git.push');
  if(actionName==='delete_remote_branch')permissions.push('git.remote.delete');
  if(actionName==='force_delete_branch')permissions.push('git.force_delete');
  return permissions;
}
function assertGitActionAllowed(actionName){
  if(!app.sharedMode)return;
  const missing=requiredGitPermissions(actionName).filter(permission=>!gitPermissionAllowed(permission));
  if(missing.length)throw new Error('Permission required: '+missing.join(', '));
}

async function action(actionName,payload={},confirmText='',options={}){
  assertGitActionAllowed(actionName);
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
      if(options.refresh!==false&&activeRepoID===repoID){await refresh();await loadCurrentView();}return data;
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

const views=[['repositories','Repositories'],['changes','Changes'],['branches','Branches'],['worktrees','Worktrees'],['submodules','Submodules'],['tags','Tags'],['log','Log'],['graph','Graph'],['reflog','Recovery'],['rebase','Rebase'],['file-history','File History'],['ahead-behind','Ahead / Behind'],['stashes','Stash'],['compare','Compare']];
for(const [id,label] of views){const b=el('button','',label);b.dataset.gitView=id;b.onclick=()=>{currentView=id;updateNav();loadCurrentView().catch(app.showError);};nav.append(b);}
function updateNav(){for(const b of nav.querySelectorAll('button'))b.classList.toggle('active',b.dataset.gitView===currentView);}
function empty(message){content.replaceChildren(el('div','git-empty',message));}
function actionButton(label,run,title=''){const b=el('button','',label);if(title)b.title=title;return bindActionButton(b,run);}

async function loadRepositories(force=false){
  await refreshRepositories(force);content.replaceChildren();
  if(!repositories.length){
    empty(!gitScanEnabled?'Git repository scan is disabled. Set [git] scan_enabled=true and press Scan.':'No Git repositories found (scan depth: '+gitScanDepth+'). Confirm the workspace path or configure [git.repositories].');
    if(gitScanWarnings.length){
      content.append(el('div','git-row-sub','Git markers found but verification failed:'),el('pre','git-operation-output',gitScanWarnings.join('\n')));
    }
    return;
  }
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

function setGitPanelWide(wide){panel.classList.toggle('git-panel-wide',Boolean(wide));}
function gitDiffModeMeta(mode){
  if(mode==='staged')return {title:'HEAD ↔ STAGED',left:'HEAD · committed',right:'INDEX · staged',description:'Changes selected for the next commit'};
  if(mode==='head-worktree')return {title:'HEAD ↔ WORKING',left:'HEAD · committed',right:'WORKTREE · current',description:'All tracked changes: staged + not staged'};
  return {title:'STAGED ↔ WORKING',left:'INDEX · staged',right:'WORKTREE · not staged',description:'Only changes not staged yet'};
}
function openGenericGitStateCompare(path,mode){
  const compare=globalThis.TaskMenuFileCompare;
  if(!compare?.openGitStatePair)throw new Error('File Compare unavailable');
  return compare.openGitStatePair(activeRepoID,path,workspacePathForActiveRepository(path),mode);
}
function gitVisualHunkRows(hunk){
  const raw=String(hunk?.text||'');
  const lines=raw.replace(/\n$/,'').split('\n');
  const header=String(hunk?.header||lines[0]||'');
  const range=/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(header);
  let oldLine=range?Number(range[1]):0,newLine=range?Number(range[2]):0;
  if(lines[0]?.startsWith('@@ '))lines.shift();
  const rows=[];
  for(let i=0;i<lines.length;){
    const line=lines[i];
    if(line.startsWith(' ')){
      const text=line.slice(1);rows.push({left:{no:oldLine++,text,kind:'context'},right:{no:newLine++,text,kind:'context'}});i++;continue;
    }
    if(line.startsWith('\\')){
      rows.push({note:line});i++;continue;
    }
    if(line.startsWith('-')||line.startsWith('+')){
      const removed=[],added=[];
      while(i<lines.length&&(lines[i].startsWith('-')||lines[i].startsWith('+'))){
        if(lines[i].startsWith('-'))removed.push(lines[i].slice(1));else added.push(lines[i].slice(1));
        i++;
      }
      const count=Math.max(removed.length,added.length);
      for(let n=0;n<count;n++){
        rows.push({
          left:n<removed.length?{no:oldLine++,text:removed[n],kind:'removed'}:null,
          right:n<added.length?{no:newLine++,text:added[n],kind:'added'}:null
        });
      }
      continue;
    }
    rows.push({left:{no:oldLine++,text:line,kind:'context'},right:{no:newLine++,text:line,kind:'context'}});i++;
  }
  return rows;
}
function gitVisualCell(spec,importance='context',ranges=[]){
  const cell=el('div','git-diff-cell '+(spec?.kind||'blank')+(spec&&spec.kind!=='context'?' '+importance:''));
  const code=el('span','git-diff-code');
  const compare=globalThis.TaskMenuFileCompare;
  if(compare?.renderGitPreviewCode)compare.renderGitPreviewCode(code,spec,ranges);
  else code.textContent=spec?.text||'\u00a0';
  cell.append(el('span','git-diff-line-no',spec?.no?String(spec.no):''),code);
  return cell;
}
function renderGitVisualHunk(hunk,mode){
  const meta=gitDiffModeMeta(mode);
  const visual=el('div','git-visual-diff');const grid=el('div','git-visual-grid');
  const columns=el('div','git-diff-columns-head');columns.append(el('div','git-diff-column-head',meta.left),el('div','git-diff-column-head',meta.right));grid.append(columns);
  const compare=globalThis.TaskMenuFileCompare;
  const rows=gitVisualHunkRows(hunk);
  if(compare?.decorateGitPreviewRows)compare.decorateGitPreviewRows(rows,hunk.previewPath||'');
  for(const row of rows){
    const line=el('div','git-diff-visual-row');
    if(row.note){line.append(el('div','git-diff-note',row.note));}
    else line.append(gitVisualCell(row.left,row.importance,row.ranges?.left||[]),gitVisualCell(row.right,row.importance,row.ranges?.right||[]));
    grid.append(line);
  }
  visual.append(grid);
  const raw=document.createElement('details');raw.className='git-diff-raw';const summary=document.createElement('summary');summary.textContent='Raw unified patch';raw.append(summary,el('pre','git-diff-pre',hunk.text||hunk.header||''));visual.append(raw);
  return visual;
}
function gitDiffModeBar(path,mode){
  const bar=el('div','git-diff-mode-bar');
  const modes=[
    ['staged','HEAD ↔ Staged','Committed HEAD compared with the Git index (staged changes only)'],
    ['worktree','Staged ↔ Working','Git index compared with the working tree (not-staged changes only)'],
    ['head-worktree','HEAD ↔ Working','Committed HEAD compared with current working tree (all tracked changes)']
  ];
  for(const [value,label,title] of modes){
    const button=actionButton(label,()=>showDiff(path,value),title);button.classList.toggle('active',value===mode);bar.append(button);
  }
  bar.append(actionButton('Open full diff tab ↗',()=>openGenericGitStateCompare(path,mode),'Open full-file syntax-highlighted File Compare; Git HEAD/index remain read-only and working-tree saves never stage changes'));
  return bar;
}
async function runHunkAction(path,mode,data,hunk,kind){
  const payload={path,hunk_index:hunk.index,expected_diff_sha:data.diff_sha256};
  if(kind==='discard')payload.confirmed=true;
  const actionName=kind==='stage'?'stage_hunk':kind==='unstage'?'unstage_hunk':'discard_hunk';
  const confirmText=kind==='discard'?'Discard this worktree hunk permanently? This cannot be recovered by Git unless the content exists elsewhere.':'';
  const result=await action(actionName,payload,confirmText,{refresh:false});
  if(result===false)return false;
  await refresh();
  return showDiff(path,mode);
}
async function showDiff(path,mode='head-worktree'){
  const data=await gitView('diff',{path,mode});if(!data)return false;content.replaceChildren();setGitPanelWide(true);
  const back=actionButton('← Changes',()=>{currentView='changes';updateNav();return loadChanges();});
  const meta=gitDiffModeMeta(mode);const heading=el('div','git-row-main');heading.append(el('div','git-row-title',meta.title+' · '+path),el('div','git-row-sub',meta.description));
  content.append(back,heading,gitDiffModeBar(path,mode));
  const hunks=Array.isArray(data.hunks)?data.hunks:[];
  if(!hunks.length){
    content.append(el('pre','git-diff-pre',data.diff||'(no differences in this state pair)'));
    return true;
  }
  // One horizontal scroller for the entire preview, never a scrollbar
  // in each code line or each hunk. Full-file comparison uses the same
  // syntax/importance renderer in its own workspace tab.
  const preview=el('div','git-diff-preview-scroll');
  for(const hunk of hunks){
    hunk.previewPath=path;
    const card=el('section','git-hunk');
    const head=el('div','git-hunk-head');
    const label=el('span','git-hunk-title','Hunk '+String((hunk.index??0)+1)+' · '+String(hunk.header||''));
    const actions=el('div','git-hunk-actions');
    if(mode==='staged'){
      actions.append(actionButton('Unstage hunk',()=>runHunkAction(path,mode,data,hunk,'unstage'),'Remove only this hunk from the index'));
    }else if(mode==='worktree'){
      actions.append(actionButton('Stage hunk',()=>runHunkAction(path,mode,data,hunk,'stage'),'Stage only this hunk'));
      actions.append(actionButton('Discard hunk',()=>runHunkAction(path,mode,data,hunk,'discard'),'Restore only this hunk from the index version'));
    }
    head.append(label,actions);
    actions.append(actionButton('Full diff ↗',()=>openGenericGitStateCompare(path,mode),'Open the whole file in the File Compare tab'));
    card.append(head,renderGitVisualHunk(hunk,mode));
    preview.append(card);
  }
  content.append(preview);
  return true;
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
  const previewPattern=async pattern=>{
    const query=new URLSearchParams({view:'ignore-preview',path,pattern:String(pattern||'')});query.set('repo',repoID);
    return app.jsonFetch('/api/git/status?'+query.toString());
  };
  const applyIgnore=(item,pattern)=>action('ignore',{path,ignore_id:item.id,ignore_pattern:String(pattern||'')},'',{recovery:false,repoID});
  return gitIgnoreWizard.open({
    path,repository,loadSuggestions,previewPattern,apply:applyIgnore,
    refresh:async()=>{if(activeRepoID===repoID){await refresh();if(currentView==='changes')await loadChanges();}}
  });
}

async function loadChanges(){
  setGitPanelWide(false);
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
    const row=el('div','git-row'+(change.conflicted?' git-row-conflicted':''));const code=el('span','git-row-code',(change.index_status||' ')+(change.worktree_status||' '));const main=el('div','git-row-main');main.append(el('div','git-row-title',change.path),el('div','git-row-sub',[change.conflicted&&'conflict',change.staged&&!change.conflicted&&'staged/index changed',change.unstaged&&!change.conflicted&&'working tree changed after index',change.staged&&change.unstaged&&!change.conflicted&&'3-state file: HEAD → staged → working',change.untracked&&'untracked',change.original_path&&('from '+change.original_path)].filter(Boolean).join(' · ')));const actions=el('div','git-row-actions');
    if(change.conflicted){
      actions.append(actionButton('3-way editor',()=>{
        const editor=globalThis.TaskMenuGitMergeEditor;if(!editor?.open)throw new Error('Git 3-way merge editor unavailable');
        return editor.open({repoID:activeRepoID,path:change.path});
      },'Open Base / Current / Incoming / Result merge editor'));
      actions.append(actionButton('Resolve',openConflictRecovery,'Open the Git conflict recovery wizard'));
      actions.append(actionButton('Open',()=>{const item=(conflictState?.files||[]).find(file=>file.path===change.path);const path=item?.project_path;if(!path)throw new Error('Project path unavailable for conflicted file');window.dispatchEvent(new CustomEvent('taskmenu:project-file-open-request',{detail:{path,source:'git-conflict-changes'}}));},'Open conflicted working-tree file'));
    }else{
      if(change.staged)actions.append(actionButton('HEAD ↔ Staged',()=>showDiff(change.path,'staged'),'Committed HEAD compared with the staged index version'));
      if(change.unstaged&&!change.untracked)actions.append(actionButton('Staged ↔ Working',()=>showDiff(change.path,'worktree'),'Staged index compared with the current working-tree version; isolates not-staged edits'));
      if(change.staged&&change.unstaged&&!change.untracked)actions.append(actionButton('HEAD ↔ Working',()=>showDiff(change.path,'head-worktree'),'Committed HEAD compared with the current working tree; shows staged + not-staged tracked edits together'));
      if(change.unstaged||change.untracked)actions.append(actionButton('Stage',()=>action('stage',{path:change.path})));if(change.staged)actions.append(actionButton('Unstage',()=>action('unstage',{path:change.path})));if(change.untracked&&change.path!=='.gitignore')actions.append(actionButton('Ignore',()=>openIgnoreWizard(change),'Ignore this untracked path or choose a smart pattern for similar files'));
    }
    if(!change.untracked){
      actions.append(actionButton('History',()=>openGitFileView(change.path,'history'),'Show commit history for this file'));
      actions.append(actionButton('Blame',()=>openGitFileView(change.path,'blame'),'Show line authorship for this file'));
    }
    actions.append(actionButton('Copy path',()=>copyText(change.path)));
    const conflictedProjectPath=change.conflicted?(conflictState?.files||[]).find(file=>file.path===change.path)?.project_path:'';
    const projectPath=String(conflictedProjectPath||workspacePathForActiveRepository(change.path)||'');
    row.oncontextmenu=event=>{
      if(!projectPath||!globalThis.TaskMenuProjectFileActions?.openMenu)return;
      event.preventDefault();event.stopPropagation();
      globalThis.TaskMenuProjectFileActions.openMenu({path:projectPath,type:'file',x:event.clientX,y:event.clientY,title:change.path});
    };
    row.append(code,main,actions);content.append(row);
  }
}
async function mergeBranch(branch){
  const label='Merge From '+branch.name;
  const repoID=activeRepoID,repoName=activeRepository()?.name||repoID;
  beginOperation(label,'Checking working tree and local/remote branch revisions…');
  let check;
  try{
    check=await gitView('merge-preflight',{branch:branch.name});
    if(!check||activeRepoID!==repoID){showOperation(label,'Repository selection changed; merge canceled');return false;}
  }catch(error){
    showOperation(label,'',error?.message||String(error));
    throw error;
  }
  let source=check.default_source||'';
  if(check.requires_choice){
    if(!gitBranchChoiceWizard?.open)throw new Error('Git branch choice wizard unavailable');
    const localSHA=String(check.local_sha||'').slice(0,12);
    const remoteSHA=String(check.remote_sha||'').slice(0,12);
    showOperation(label,'Choose the exact source revision in the Git branch wizard.');
    source=await gitBranchChoiceWizard.open({
      title:'Choose source version for Merge From',
      description:'Repository: '+repoName+'\nBranch: '+branch.name+'\nInto current branch: '+check.current+
        '\nLocal and remote versions differ. Choose which existing commit to merge; the other version is not changed.',
      options:[
        {id:'local',label:'LOCAL · '+branch.name,ref:check.local_ref+' @ '+localSHA,
         description:'Merge the local branch commit into '+check.current+'.'},
        {id:'remote',label:'REMOTE · '+check.remote_ref,ref:check.remote_ref+' @ '+remoteSHA,
         description:'Merge the remote-tracking commit into '+check.current+'.'}
      ],
      defaultChoice:branch.remote?'remote':'local',
      confirmLabel:'Merge selected version'
    });
    if(!source){showOperation(label,'Merge canceled');return false;}
    if(activeRepoID!==repoID){showOperation(label,'Repository selection changed; merge canceled');return false;}
  }
  const mergeRef=source==='remote'?check.remote_ref:check.local_ref;
  const expectedSHA=source==='remote'?check.remote_sha:check.local_sha;
  if(!mergeRef||!expectedSHA){const error=new Error('Selected merge source is unavailable.');showOperation(label,'',error.message);throw error;}
  if(!check.requires_choice&&!window.confirm('Repository: '+repoName+'\nMerge From: '+mergeRef+' @ '+expectedSHA.slice(0,12)+'\nInto current branch: '+check.current+'?')){showOperation(label,'Merge canceled');return false;}
  return action('merge',{branch:branch.name,source,expected_sha:expectedSHA,expected_current:check.current,merge_ref:mergeRef},'',{repoID});
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
async function deleteLocalBranch(branch,force=false){
  const upstream=String(branch?.upstream||'').trim();
  const detail=upstream?'\nTracked upstream: '+upstream+'\n\nThis action deletes LOCAL only. The remote branch is not changed.':'\n\nThis action deletes LOCAL only.';
  const warning=force
    ?'\n\nFORCE DELETE uses git branch -D. Git will delete the local branch even when its commits are not merged. Those commits may become difficult to recover once reflog entries expire.'
    :'\n\nSafe delete uses git branch -d. Git will refuse if it does not consider the branch fully merged.';
  const actionName=force?'force_delete_branch':'delete_branch';
  const result=await action(actionName,{branch:branch.name,confirmed:true},(force?'FORCE delete local branch ':'Delete local branch ')+branch.name+'?'+detail+warning);
  return result===false?false:loadBranches();
}
async function deleteRemoteTracking(branch){
  const result=await action('delete_remote_tracking',{branch:branch.name,confirmed:true},'Forget local remote-tracking ref '+branch.name+'?\n\nThis only deletes refs/remotes/'+branch.name+' on this machine. It DOES NOT delete the branch from the remote server.\n\nA later git fetch may recreate it if the remote branch still exists.');
  return result===false?false:loadBranches();
}
async function deleteRemoteBranch(branch){
  const remoteRef=String(branch?.name||'').trim();
  const slash=remoteRef.indexOf('/');
  const remote=slash>0?remoteRef.slice(0,slash):'remote';
  const name=slash>0?remoteRef.slice(slash+1):remoteRef;
  const result=await action('delete_remote_branch',{branch:remoteRef,confirmed:true},'DELETE REMOTE BRANCH?\n\nRemote: '+remote+'\nBranch: '+name+'\n\nThis runs git push '+remote+' --delete '+name+' and changes the shared remote repository. Other users will see this deletion after fetching.\n\nThe local branch, if any, is NOT deleted by this action.');
  return result===false?false:loadBranches();
}
async function loadBranches(){
  const data=await gitView('branches');if(!data)return false;content.replaceChildren();const create=el('div','git-branch-create');const input=document.createElement('input');input.placeholder='new branch name';const button=actionButton('Create & switch',async()=>{const name=input.value.trim();if(!name)return false;await action('create_branch',{branch:name});input.value='';return loadBranches();});const prune=actionButton('Prune remotes',async()=>{const result=await action('fetch',{},'',{refresh:false});if(result===false)return false;await refresh();return loadBranches();},'Run git fetch --prune and refresh branch lists');create.append(input,button,prune);content.append(create);
  const appendRows=(target,rows)=>{for(const branch of rows||[]){const row=el('div','git-row');const code=el('span','git-row-code',branch.current?'*':'');const main=el('div','git-row-main');const scope=branch.remote?'REMOTE-TRACKING (local cache)':'LOCAL';main.append(el('div','git-row-title',branch.name),el('div','git-row-sub',[scope,branch.upstream&&('tracks '+branch.upstream)].filter(Boolean).join(' · ')));const actions=el('div','git-row-actions');if(!branch.remote&&!branch.current)actions.append(actionButton('Switch',()=>action('switch',{branch:branch.name})));if(branch.remote||!branch.current)actions.append(actionButton('Merge From',()=>mergeBranch(branch),'Merge the selected branch into the current branch'),actionButton('Merge To',()=>mergeToBranch(branch),'Merge committed HEAD of the current branch into the selected branch, then push the target'));if(!branch.remote&&!branch.current){actions.append(actionButton('Delete local',()=>deleteLocalBranch(branch,false),'Safe local-only delete using git branch -d'),actionButton('Force delete local',()=>deleteLocalBranch(branch,true),'Force local-only delete using git branch -D; unmerged commits can be lost'));if(branch.upstream)actions.append(actionButton('Delete upstream remote',()=>deleteRemoteBranch({name:branch.upstream}),'Delete '+branch.upstream+' on its remote server; the local branch remains'));}if(branch.remote){actions.append(actionButton('Forget local ref',()=>deleteRemoteTracking(branch),'Delete only the local refs/remotes entry; does not change the remote server'),actionButton('Delete on remote',()=>deleteRemoteBranch(branch),'Delete the actual branch from the remote server; does not delete a same-named local branch'));}actions.append(actionButton('Compare',()=>loadCompare(branch.name)),actionButton('Copy',()=>copyText(branch.name)));row.append(code,main,actions);target.append(row);}};
  content.append(el('div','taskmenu-menu-label','LOCAL BRANCHES'));appendRows(content,data.local);
  const remoteRows=Array.isArray(data.remote)?data.remote:[];
  const remoteToggle=el('button','git-branch-section-toggle');remoteToggle.type='button';remoteToggle.setAttribute('aria-expanded','false');
  const remoteList=el('div','git-branch-remote-list');remoteList.hidden=true;appendRows(remoteList,remoteRows);
  const renderRemoteToggle=()=>{const expanded=remoteToggle.getAttribute('aria-expanded')==='true';remoteToggle.textContent=(expanded?'▾ ':'▸ ')+'REMOTE-TRACKING REFS ('+remoteRows.length+')';remoteToggle.title=expanded?'Collapse remote branches':'Expand remote branches';};
  remoteToggle.onclick=()=>{const expanded=remoteToggle.getAttribute('aria-expanded')==='true';remoteToggle.setAttribute('aria-expanded',expanded?'false':'true');remoteList.hidden=expanded;renderRemoteToggle();};
  renderRemoteToggle();content.append(remoteToggle,remoteList);
}

function worktreeDirectorySuggestion(branch){
  const base=String(branch||'worktree').trim().replace(/^refs\/heads\//,'').replace(/[^A-Za-z0-9._-]+/g,'-').replace(/^-+|-+$/g,'');
  return (base||'worktree').slice(0,96);
}
function gitWorktreeCreateRow(title){
  const wrap=el('div','git-branch-create');wrap.append(el('span','git-row-sub',title));return wrap;
}
async function loadWorktrees(){
  const data=await gitView('worktrees');if(!data)return false;
  const branches=await gitView('branches');if(!branches)return false;
  content.replaceChildren();
  const rows=Array.isArray(data.worktrees)?data.worktrees:[];
  const occupied=new Set(rows.map(item=>String(item.branch||'')).filter(Boolean));

  const tools=el('div','git-row-actions');
  tools.append(
    actionButton('↻ Refresh',()=>loadWorktrees()),
    actionButton('Prune stale',()=>action('worktree_prune',{confirmed:true},'Prune stale Git worktree administrative entries now?\n\nThis does not remove a healthy existing worktree directory.'))
  );
  content.append(tools);

  const existing=gitWorktreeCreateRow('Existing local branch');
  const branchSelect=document.createElement('select');branchSelect.className='git-repo-select';
  for(const branch of branches.local||[]){
    const option=document.createElement('option');option.value=branch.name;
    const where=occupied.has(branch.name)?' · already checked out':'';
    option.textContent=branch.name+where;option.disabled=occupied.has(branch.name);branchSelect.append(option);
  }
  const availableExisting=(branches.local||[]).find(branch=>!occupied.has(branch.name));
  if(availableExisting)branchSelect.value=availableExisting.name;
  const existingDir=document.createElement('input');existingDir.placeholder='sibling directory name';existingDir.spellcheck=false;
  const syncExistingDir=()=>{if(branchSelect.value&&!existingDir.dataset.edited)existingDir.value=worktreeDirectorySuggestion(branchSelect.value);};
  branchSelect.onchange=syncExistingDir;existingDir.oninput=()=>existingDir.dataset.edited='1';syncExistingDir();
  const addExisting=actionButton('Add worktree',()=>{
    const branch=branchSelect.value,dir=existingDir.value.trim();
    if(!branch||!dir)throw new Error('Choose a local branch and sibling directory name');
    return action('worktree_add_branch',{branch,directory_name:dir});
  });
  addExisting.disabled=!availableExisting;
  if(!availableExisting)addExisting.title='Every local branch is already checked out in a worktree';
  existing.append(branchSelect,existingDir,addExisting);content.append(existing);

  const create=gitWorktreeCreateRow('New branch + worktree');
  const newBranch=document.createElement('input');newBranch.placeholder='new branch name';newBranch.spellcheck=false;
  const newDir=document.createElement('input');newDir.placeholder='sibling directory name';newDir.spellcheck=false;
  newBranch.oninput=()=>{if(!newDir.dataset.edited)newDir.value=worktreeDirectorySuggestion(newBranch.value);};
  newDir.oninput=()=>newDir.dataset.edited='1';
  const source=document.createElement('select');source.className='git-repo-select';
  const headOption=document.createElement('option');headOption.value='HEAD';headOption.textContent='HEAD · current revision';source.append(headOption);
  for(const branch of [...(branches.local||[]),...(branches.remote||[])]){
    const option=document.createElement('option');option.value=branch.name;option.textContent=(branch.remote?'remote · ':'local · ')+branch.name;source.append(option);
  }
  const addNew=actionButton('Create + add',()=>{
    const branch=newBranch.value.trim(),dir=newDir.value.trim(),ref=source.value||'HEAD';
    if(!branch||!dir)throw new Error('New branch name and sibling directory name are required');
    return action('worktree_add_new_branch',{branch,directory_name:dir,ref});
  });
  create.append(newBranch,newDir,source,addNew);content.append(create);

  if(!rows.length){content.append(el('div','git-empty','No Git worktrees found'));return true;}
  for(const item of rows){
    const row=el('div','git-row');
    const badges=[item.current?'current':'',item.primary?'primary':'',item.detached?'detached':'',item.locked?'locked':'',item.prunable?'prunable':''].filter(Boolean);
    const code=el('span','git-row-code',item.current?'*':'');
    const main=el('div','git-row-main');
    const title=item.branch||'(detached HEAD)';
    const sub=[item.display_path,item.head&&String(item.head).slice(0,12),badges.join(', '),item.lock_reason,item.prune_reason].filter(Boolean).join(' · ');
    main.append(el('div','git-row-title',title),el('div','git-row-sub',sub));
    const actions=el('div','git-row-actions');
    actions.append(actionButton('Open TaskDeck',()=>action('worktree_open',{worktree_id:item.id},'',{refresh:false}),'Launch or focus TaskDeck for this resolved worktree'));
    actions.append(actionButton('Copy branch',()=>copyText(item.branch||item.head||'')));
    if(!item.current&&!item.primary){
      actions.append(actionButton('Remove',()=>action('worktree_remove',{worktree_id:item.id,confirmed:true},'Remove worktree '+item.display_path+'?\n\nGit will refuse if the worktree contains uncommitted changes. No force removal is used.'),'Remove this linked worktree without --force'));
    }
    row.append(code,main,actions);content.append(row);
  }
  return true;
}


function submoduleProjectPath(item){
  const repo=activeRepository();
  const root=repo?.path==='.'?'':String(repo?.path||repo?.id||'').replace(/\\/g,'/').replace(/^\.\//,'').replace(/\/+$/,'');
  const child=String(item?.path||'').replace(/\\/g,'/').replace(/^\.\//,'').replace(/^\/+|\/+$/g,'');
  return root?(root+'/'+child):child;
}
async function openSubmoduleRepository(item){
  if(!item?.initialized)throw new Error('Initialize the submodule before opening it as a repository');
  const target=submoduleProjectPath(item);
  await refreshRepositories(true);
  const match=repositories.find(repo=>String(repo.path||repo.id||'').replace(/\\/g,'/').replace(/^\.\//,'').replace(/\/+$/,'')===target);
  if(!match)throw new Error('Initialized submodule repository was not discovered: '+target);
  currentView='changes';updateNav();
  return selectRepository(match.id);
}
function submoduleStatusSummary(item){
  const parts=[item.status||'unknown'];
  if(item.expected_sha)parts.push('expected '+String(item.expected_sha).slice(0,12));
  if(item.actual_sha)parts.push('actual '+String(item.actual_sha).slice(0,12));
  if(item.branch)parts.push('branch '+item.branch);
  if(item.dirty)parts.push('local changes');
  return parts.join(' · ');
}
async function loadSubmodules(){
  const data=await gitView('submodules');if(!data)return false;
  content.replaceChildren();
  const rows=Array.isArray(data.submodules)?data.submodules:[];
  const tools=el('div','git-row-actions');
  tools.append(
    actionButton('↻ Refresh',()=>loadSubmodules()),
    actionButton('Update recursive',()=>action('submodule_update_recursive',{confirmed:true},'Update and initialize all submodules recursively to the commits recorded by the current superproject?\n\nTaskDeck blocks this when a top-level submodule has local changes.'),'git submodule update --init --recursive --checkout'),
    actionButton('Sync URLs',()=>action('submodule_sync',{confirmed:true},'Sync submodule URLs from .gitmodules into local Git configuration recursively?'),'git submodule sync --recursive')
  );
  content.append(tools);
  if(!rows.length){
    content.append(el('div','git-empty','No submodules declared by .gitmodules in this repository'));
    return true;
  }
  for(const item of rows){
    const row=el('div','git-row');
    const code=el('span','git-row-code',item.initialized?(item.dirty?'!':(item.mismatch?'≠':'✓')):'-');
    const main=el('div','git-row-main');
    const title=(item.name||item.path)+(item.initialized?'':' · uninitialized');
    const sub=el('div','git-row-sub',item.path+' · '+submoduleStatusSummary(item));sub.title=[item.url,item.branch].filter(Boolean).join(' · ');
    main.append(el('div','git-row-title',title),sub);
    const actions=el('div','git-row-actions');
    if(!item.initialized){
      actions.append(actionButton('Init',()=>action('submodule_init',{submodule_id:item.id,expected_sha:item.expected_sha,confirmed:true},'Initialize '+item.path+' and checkout the commit recorded by the current superproject?')));
    }else{
      actions.append(actionButton('Open repo',()=>openSubmoduleRepository(item),'Open this initialized submodule as the active repository in Git Panel'));
      const update=actionButton('Update',()=>action('submodule_update',{submodule_id:item.id,expected_sha:item.expected_sha,confirmed:true},'Update '+item.path+' to the commit recorded by the current superproject?'));
      update.disabled=Boolean(item.dirty);if(item.dirty)update.title='Blocked because this submodule has local changes';actions.append(update);
      if(item.mismatch){
        const checkout=actionButton('Checkout expected',()=>action('submodule_checkout_expected',{submodule_id:item.id,expected_sha:item.expected_sha,confirmed:true},'Checkout recorded commit '+String(item.expected_sha||'').slice(0,12)+' in '+item.path+'?'));
        checkout.disabled=Boolean(item.dirty);if(item.dirty)checkout.title='Blocked because this submodule has local changes';actions.append(checkout);
      }
    }
    actions.append(actionButton('Copy path',()=>copyText(item.path)),actionButton('Copy expected SHA',()=>copyText(item.expected_sha||'')));
    row.append(code,main,actions);content.append(row);
  }
  return true;
}

async function loadTags(){
  const data=await gitView('tags');if(!data)return false;content.replaceChildren();
  const create=el('div','git-branch-create');
  const input=document.createElement('input');input.placeholder='tag name, e.g. v1.2.0';input.spellcheck=false;
  const add=actionButton('Create at HEAD',async()=>{
    const name=input.value.trim();if(!name)return false;
    const message=window.prompt('Optional annotated tag message. Leave blank for a lightweight tag:','');
    if(message===null)return false;
    const result=await action('create_tag',{name,message,ref:'HEAD'});
    if(result===false)return false;input.value='';return loadTags();
  });
  create.append(input,add);content.append(create);
  for(const tag of data.tags||[]){
    const row=el('div','git-row');const code=el('span','git-row-code',tag.annotated?'●':'○');const main=el('div','git-row-main');
    main.append(el('div','git-row-title',tag.name),el('div','git-row-sub',[tag.sha?.slice(0,12),tag.date,tag.annotated?'annotated':'lightweight',tag.subject].filter(Boolean).join(' · ')));
    const actions=el('div','git-row-actions');
    actions.append(actionButton('Copy tag',()=>copyText(tag.name)),actionButton('Copy SHA',()=>copyText(tag.sha)));
    actions.append(actionButton('Delete',()=>action('delete_tag',{name:tag.name,confirmed:true},'Delete local Git tag '+tag.name+'? This does not delete a remote tag.').then(result=>result===false?false:loadTags()),'Delete this local tag only'));
    row.append(code,main,actions);content.append(row);
  }
  if(!(data.tags||[]).length)content.append(el('div','git-empty','No local tags'));
}


function gitGraphLayout(commits){
  const lanes=[],rows=[];
  for(const commit of commits||[]){
    let lane=lanes.indexOf(commit.sha);
    if(lane<0){lane=0;lanes.unshift(commit.sha);}
    const before=lanes.slice();
    const parents=Array.isArray(commit.parents)?commit.parents.filter(Boolean):[];
    if(parents.length){
      lanes[lane]=parents[0];
      let insertAt=lane+1;
      for(const parent of parents.slice(1)){
        if(!lanes.includes(parent)){lanes.splice(insertAt,0,parent);insertAt++;}
      }
    }else lanes.splice(lane,1);
    const seen=new Set();
    for(let i=lanes.length-1;i>=0;i--){
      if(seen.has(lanes[i]))lanes.splice(i,1);else seen.add(lanes[i]);
    }
    rows.push({commit,lane,before,after:lanes.slice()});
  }
  return rows;
}
function gitGraphSVG(layout){
  const NS='http://www.w3.org/2000/svg',step=16,height=30;
  const count=Math.max(1,layout.before.length,layout.after.length);
  const svg=document.createElementNS(NS,'svg');svg.classList.add('git-graph-svg');svg.setAttribute('width',String(count*step+8));svg.setAttribute('height',String(height));svg.setAttribute('viewBox','0 0 '+(count*step+8)+' '+height);
  const x=index=>8+index*step;
  const line=(x1,y1,x2,y2)=>{
    const node=document.createElementNS(NS,'path');node.setAttribute('d','M '+x1+' '+y1+' C '+x1+' 15 '+x2+' 15 '+x2+' '+y2);node.setAttribute('fill','none');node.setAttribute('stroke','currentColor');node.setAttribute('stroke-width','1.4');node.setAttribute('opacity','.55');svg.append(node);
  };
  for(let i=0;i<layout.before.length;i++){
    if(i===layout.lane)continue;
    const ref=layout.before[i],next=layout.after.indexOf(ref);
    if(next>=0)line(x(i),0,x(next),height);
  }
  const parents=Array.isArray(layout.commit.parents)?layout.commit.parents:[];
  for(const parent of parents){
    const target=layout.after.indexOf(parent);
    if(target>=0)line(x(layout.lane),15,x(target),height);
  }
  const circle=document.createElementNS(NS,'circle');circle.setAttribute('cx',String(x(layout.lane)));circle.setAttribute('cy','15');circle.setAttribute('r','4');circle.setAttribute('fill','currentColor');svg.append(circle);
  return svg;
}
function gitGraphRefNodes(refs){
  const host=el('div','git-graph-refs');
  for(const ref of refs||[]){
    const chip=el('span','git-graph-ref '+String(ref.kind||'other'),ref.name||'');chip.title=(ref.kind||'ref')+': '+(ref.name||'');host.append(chip);
  }
  return host;
}
function gitGraphInput(key,placeholder,type='text'){
  const input=document.createElement('input');input.type=type;input.placeholder=placeholder;input.value=gitGraphFilters[key]||'';input.spellcheck=false;
  input.oninput=()=>{gitGraphFilters[key]=input.value;};
  return input;
}
function gitGraphControls(run){
  const controls=el('div','git-graph-controls');
  const search=gitGraphInput('search','Search hash / author / message / ref');
  const author=gitGraphInput('author','Author filter');
  const message=gitGraphInput('message','Message contains');
  const since=gitGraphInput('since','Since','date');
  const until=gitGraphInput('until','Until','date');
  const pathInput=gitGraphInput('path','Path filter');
  const actions=el('div','git-graph-controls-actions');
  const apply=actionButton('Apply filters',run);
  const clear=actionButton('Clear',async()=>{
    gitGraphFilters={search:'',author:'',message:'',since:'',until:'',path:''};gitGraphSelectedSHA='';return loadGraph();
  });
  actions.append(apply,clear);
  controls.append(search,author,message,since,until,pathInput,actions);
  for(const input of [search,author,message,since,until,pathInput])input.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();run().catch(app.showError);}});
  return controls;
}
async function gitGraphCheckout(commit){
  return action('checkout_commit',{ref:commit.sha,expected_sha:commit.sha},'Checkout '+commit.short+' in detached HEAD mode?\n\nThe working tree must be clean. You can create a branch later to keep new commits.');
}
async function gitGraphCreateBranch(commit){
  const name=window.prompt('New branch name at '+commit.short+':','');
  if(name===null||!name.trim())return false;
  return action('create_branch_at',{branch:name.trim(),ref:commit.sha,expected_sha:commit.sha},'Create and switch to '+name.trim()+' at '+commit.short+'?\n\nThe working tree must be clean.');
}
async function gitGraphTag(commit){
  const name=window.prompt('Tag name for '+commit.short+':','');
  if(name===null||!name.trim())return false;
  const message=window.prompt('Optional annotated tag message. Leave blank for a lightweight tag:','');
  if(message===null)return false;
  return action('create_tag',{name:name.trim(),message,ref:commit.sha});
}
async function gitSemanticsPreview(ref,path=''){
  const params={ref};if(path)params.path=path;
  const data=await gitView('semantics-preview',params);
  return data?.preview||null;
}
function gitDirtyEditorForWorkspacePath(pathValue){
  const normalized=String(pathValue||'').replace(/\\/g,'/').replace(/^\.\//,'');
  const editor=globalThis.TaskMenuEditor;if(!editor?.editors)return null;
  for(const view of editor.editors.values()){
    if(view?.closed)continue;
    const current=String(view?.file?.path||'').replace(/\\/g,'/').replace(/^\.\//,'');
    if(current===normalized&&view.dirty)return view;
  }
  return null;
}
async function gitReloadWorkspaceEditor(pathValue){
  const normalized=String(pathValue||'').replace(/\\/g,'/').replace(/^\.\//,'');
  const editor=globalThis.TaskMenuEditor;if(!editor?.editors)return;
  for(const view of editor.editors.values()){
    if(view?.closed||view.dirty)continue;
    const current=String(view?.file?.path||'').replace(/\\/g,'/').replace(/^\.\//,'');
    if(current===normalized){await editor.reloadEditor?.(view);return;}
  }
}
async function openGitCommitSemantics(commit,preferred='mixed'){
  const wizard=globalThis.TaskDeckGitSemanticsWizard;if(!wizard?.open)throw new Error('Git semantics wizard unavailable');
  const preview=await gitSemanticsPreview(commit.sha);if(!preview)return false;
  return wizard.open({
    kind:'commit',preview,preferred,
    onCompareFile:file=>{
      const compare=globalThis.TaskMenuFileCompare;if(!compare?.openGitCommitFileDiffBetween)throw new Error('File Compare unavailable');
      return compare.openGitCommitFileDiffBetween(activeRepoID,file,preview.target_sha,preview.head_sha,{title:'Target ↔ HEAD'});
    },
    onConfirm:async operation=>{
      if(operation==='revert'){
        return action('revert_commit',{ref:preview.target_sha,expected_sha:preview.target_sha,expected_head_sha:preview.head_sha});
      }
      if(!['soft','mixed','hard'].includes(operation))throw new Error('Unsupported reset mode');
      return action('reset_commit',{ref:preview.target_sha,expected_sha:preview.target_sha,expected_head_sha:preview.head_sha,mode:operation,confirmed:true});
    }
  });
}
async function openGitFileRestoreSemantics(commit,pathValue){
  const wizard=globalThis.TaskDeckGitSemanticsWizard;if(!wizard?.open)throw new Error('Git semantics wizard unavailable');
  const preview=await gitSemanticsPreview(commit.sha,pathValue);if(!preview)return false;
  const workspacePath=workspacePathForActiveRepository(pathValue);
  return wizard.open({
    kind:'file',preview,path:pathValue,preferred:'worktree',
    onCompareFile:()=>{
      const compare=globalThis.TaskMenuFileCompare;if(!compare?.openGitCommitAgainstProject)throw new Error('File Compare unavailable');
      return compare.openGitCommitAgainstProject(activeRepoID,pathValue,workspacePath,preview.target_sha);
    },
    onConfirm:async operation=>{
      if(operation==='worktree'&&gitDirtyEditorForWorkspacePath(workspacePath))throw new Error('Save or close the unsaved editor before restoring this Working tree file');
      const actionName=operation==='staged'?'restore_staged_commit':'restore_file_commit';
      const data=await action(actionName,{path:pathValue,ref:preview.target_sha,expected_sha:preview.target_sha,expected_head_sha:preview.head_sha,confirmed:true});
      if(operation==='worktree')await gitReloadWorkspaceEditor(workspacePath);
      return data;
    }
  });
}
async function gitGraphReset(commit){return openGitCommitSemantics(commit,'mixed');}
function closeGitGraphContext(){
  document.querySelectorAll('.git-graph-context').forEach(node=>node.remove());
}
function showGitGraphContext(event,commit,detailHost){
  event.preventDefault();event.stopPropagation();closeGitGraphContext();
  const menu=el('div','git-graph-context');
  const add=(label,run,danger=false)=>{const b=el('button',danger?'danger':'',label);b.onclick=()=>{menu.remove();Promise.resolve(run()).catch(app.showError);};menu.append(b);};
  add('Checkout commit (detached)',()=>gitGraphCheckout(commit));
  add('Create branch here…',()=>gitGraphCreateBranch(commit));
  add('Cherry-pick',()=>action('cherry_pick',{ref:commit.sha,expected_sha:commit.sha},'Cherry-pick '+commit.short+' onto the current branch? The working tree must be clean.'));
  add('Revert…',()=>openGitCommitSemantics(commit,'revert'));
  add('Reset current branch here…',()=>gitGraphReset(commit),true);
  add('Create tag here…',()=>gitGraphTag(commit));
  add('Compare with HEAD',()=>renderGitGraphHeadCompare(commit,detailHost));
  add('Copy SHA',()=>copyText(commit.sha));
  menu.onpointerdown=event=>event.stopPropagation();
  document.body.append(menu);
  globalThis.TaskMenuContextViewport.place(menu,event.clientX,event.clientY);
  const close=()=>menu.remove();setTimeout(()=>document.addEventListener('pointerdown',close,{once:true}),0);
}
function gitGraphDetailHeader(commit,host,subtitle=''){
  const head=el('div','git-graph-detail-head');
  const main=el('div','git-graph-detail-title');
  main.append(el('div','git-row-title',commit.subject||'(no subject)'),el('div','git-row-sub',[commit.sha,commit.date,commit.author,subtitle].filter(Boolean).join(' · ')),gitGraphRefNodes(commit.refs));
  const actions=el('div','git-graph-detail-actions');
  actions.append(
    actionButton('Cherry-pick',()=>action('cherry_pick',{ref:commit.sha,expected_sha:commit.sha},'Cherry-pick '+commit.short+'? The working tree must be clean.')),
    actionButton('Revert…',()=>openGitCommitSemantics(commit,'revert')),
    actionButton('More…',event=>false,'Right-click the selected commit for checkout, branch, reset, tag and compare actions')
  );
  const more=actions.lastElementChild;more.onclick=event=>showGitGraphContext(event,commit,host);
  head.append(main,actions);host.append(head);return head;
}
async function renderGitGraphCommitDetail(commit,host,parentOverride=''){
  host.replaceChildren();gitGraphDetailHeader(commit,host);
  const params={ref:commit.sha};if(parentOverride)params.parent=parentOverride;
  const data=await gitView('commit-files',params);if(!data)return;
  if((data.parents||[]).length>1){
    const parentRow=el('div','git-file-controls');const label=el('span','git-row-sub','Diff parent');
    const select=document.createElement('select');
    data.parents.forEach((parent,index)=>{const o=document.createElement('option');o.value=parent;o.textContent='#'+(index+1)+' · '+parent.slice(0,12);select.append(o);});
    select.value=data.parent||data.parents[0];select.onchange=()=>renderGitGraphCommitDetail(commit,host,select.value).catch(app.showError);parentRow.append(label,select);host.append(parentRow);
  }
  const note=el('div','git-row-sub',(data.parent?'Changed vs parent '+data.parent.slice(0,12):'Root commit vs empty tree')+' · '+(data.files||[]).length+' file(s)');host.append(note);
  for(const file of data.files||[]){
    const row=el('div','git-graph-file');const code=el('span','git-row-code',file.status||'');
    const pathNode=el('span','git-graph-file-path',file.old_path?(file.old_path+' → '+file.path):file.path);pathNode.title=pathNode.textContent;
    const actions=el('div','git-row-actions');
    actions.append(actionButton('Visual diff',()=>{
      const compare=globalThis.TaskMenuFileCompare;if(!compare)throw new Error('File Compare unavailable');
      if(data.parent)return compare.openGitCommitFileDiffBetween(activeRepoID,file,data.parent,commit.sha,{title:'Commit diff · '+commit.short});
      return compare.openGitCommitFileDiff(activeRepoID,file,commit);
    }));
    actions.append(actionButton('History',()=>openGitFileView(file.path,'history')));
    row.append(code,pathNode,actions);host.append(row);
  }
  if(!(data.files||[]).length)host.append(el('div','git-empty','No changed files for this parent comparison'));
}
async function renderGitGraphHeadCompare(commit,host){
  host.replaceChildren();gitGraphDetailHeader(commit,host,'Compare with HEAD');
  const data=await gitView('graph-compare-head',{ref:commit.sha});if(!data)return;
  host.append(el('div','git-row-sub',commit.short+' ↔ HEAD '+String(data.right||'').slice(0,12)+' · '+(data.files||[]).length+' file(s)'));
  for(const file of data.files||[]){
    const row=el('div','git-graph-file');const code=el('span','git-row-code',file.status||'');
    const pathNode=el('span','git-graph-file-path',file.old_path?(file.old_path+' → '+file.path):file.path);pathNode.title=pathNode.textContent;
    const actions=el('div','git-row-actions');
    actions.append(actionButton('Visual diff',()=>{
      const compare=globalThis.TaskMenuFileCompare;if(!compare?.openGitCommitFileDiffBetween)throw new Error('File Compare unavailable');
      return compare.openGitCommitFileDiffBetween(activeRepoID,file,data.left,data.right,{title:'Commit ↔ HEAD'});
    }));
    row.append(code,pathNode,actions);host.append(row);
  }
  if(!(data.files||[]).length)host.append(el('div','git-empty','Selected commit matches HEAD'));
}
async function loadGraph(){
  setGitPanelWide(true);
  const params={limit:'300'};
  for(const [key,value] of Object.entries(gitGraphFilters))if(String(value||'').trim())params[key]=String(value).trim();
  const data=await gitView('graph',params);if(!data)return false;
  content.replaceChildren();
  const controls=gitGraphControls(loadGraph);content.append(controls);
  const shell=el('div','git-graph-shell'),list=el('div','git-graph-list'),detail=el('div','git-graph-detail');
  shell.append(list,detail);content.append(shell);
  const commits=Array.isArray(data.commits)?data.commits:[];
  if(!commits.length){list.append(el('div','git-empty','No commits match the current graph filters'));detail.append(el('div','git-empty','Select a commit'));return true;}
  const layouts=gitGraphLayout(commits);
  if(!commits.some(item=>item.sha===gitGraphSelectedSHA))gitGraphSelectedSHA=commits[0].sha;
  for(const layout of layouts){
    const commit=layout.commit,row=el('div','git-graph-row');row.classList.toggle('selected',commit.sha===gitGraphSelectedSHA);
    const main=el('div','git-graph-main');main.append(el('div','git-graph-title',commit.subject||'(no subject)'),el('div','git-graph-sub',commit.short+' · '+commit.author+' · '+commit.date),gitGraphRefNodes(commit.refs));
    row.append(gitGraphSVG(layout),main);
    row.onclick=()=>{
      gitGraphSelectedSHA=commit.sha;
      list.querySelectorAll('.git-graph-row').forEach(node=>node.classList.toggle('selected',node===row));
      renderGitGraphCommitDetail(commit,detail).catch(app.showError);
    };
    row.oncontextmenu=event=>{gitGraphSelectedSHA=commit.sha;showGitGraphContext(event,commit,detail);};
    list.append(row);
  }
  const selected=commits.find(item=>item.sha===gitGraphSelectedSHA)||commits[0];
  await renderGitGraphCommitDetail(selected,detail);
  return true;
}


function gitReflogGuidance(entry){
  switch(String(entry?.kind||'')){
    case 'reset':return 'Reset entry. Inspect nearby entries to find the commit that was reachable before the reset, then create a recovery branch before changing history again.';
    case 'checkout':return 'Checkout transition. Use this entry to recover a detached/previous HEAD or recreate a branch at the recorded commit.';
    case 'commit':return 'Commit entry. Creating a recovery branch preserves this commit even if no current branch points to it.';
    case 'branch':return 'Branch reflog entry. Recreate a deleted or moved branch by creating a new branch at the SHA you want to preserve.';
    case 'rebase':return 'Rebase entry. Preserve the desired pre/post-rebase SHA on a recovery branch before attempting another rewrite.';
    case 'cherry-pick':return 'Cherry-pick entry. Preserve this SHA if it contains work that disappeared from the current branch.';
    default:return 'Reflog entry. Create a recovery branch first when you are unsure; this preserves the commit without changing HEAD, Index, or Working tree.';
  }
}
async function gitReflogCreateBranch(entry){
  const suggested='recovery/'+String(entry.short||'commit').replace(/[^A-Za-z0-9._-]+/g,'-');
  const name=window.prompt('Recovery branch name at '+entry.short+':',suggested);
  if(name===null||!name.trim())return false;
  return action('create_branch_ref',{branch:name.trim(),ref:entry.sha,expected_sha:entry.sha},'Create recovery branch '+name.trim()+' at '+entry.short+'?\n\nThis only creates a branch ref. HEAD, Index and Working tree remain unchanged.');
}
function gitReflogControls(run){
  const controls=el('div','git-graph-controls');
  const search=document.createElement('input');search.placeholder='Search SHA / selector / actor / subject';search.value=gitReflogFilters.search;search.oninput=()=>gitReflogFilters.search=search.value;
  const ref=document.createElement('input');ref.placeholder='Ref contains, e.g. HEAD or main';ref.value=gitReflogFilters.ref;ref.oninput=()=>gitReflogFilters.ref=ref.value;
  const kind=document.createElement('select');kind.className='git-repo-select';
  for(const value of ['','commit','reset','checkout','branch','merge','rebase','cherry-pick','revert','pull','other']){
    const option=document.createElement('option');option.value=value;option.textContent=value||'All kinds';kind.append(option);
  }
  kind.value=gitReflogFilters.kind;kind.onchange=()=>gitReflogFilters.kind=kind.value;
  const actions=el('div','git-graph-controls-actions');actions.append(actionButton('Apply filters',run),actionButton('Clear',async()=>{gitReflogFilters={search:'',ref:'',kind:''};gitReflogSelected='';return loadReflog();}));
  controls.append(search,ref,kind,actions);
  for(const input of [search,ref])input.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();run().catch(app.showError);}});
  return controls;
}
async function renderGitReflogDetail(entry,host){
  host.replaceChildren();
  const head=el('div','git-graph-detail-head'),main=el('div','git-graph-detail-title');
  main.append(el('div','git-row-title',entry.subject||'(no reflog subject)'),el('div','git-row-sub',[entry.sha,entry.selector,entry.actor,entry.date].filter(Boolean).join(' · ')));
  const actions=el('div','git-graph-detail-actions');
  actions.append(
    actionButton('Create recovery branch',()=>gitReflogCreateBranch(entry),'Safest recovery: preserve this commit with a new branch without switching'),
    actionButton('Copy SHA',()=>copyText(entry.sha)),
    actionButton('Copy selector',()=>copyText(entry.selector||''))
  );
  head.append(main,actions);host.append(head);
  host.append(el('div','git-row-sub',gitReflogGuidance(entry)));
  const more=el('div','git-row-actions');
  more.append(
    actionButton('Checkout detached',()=>gitGraphCheckout(entry),'Requires clean worktree'),
    actionButton('Reset current here…',()=>gitGraphReset(entry),'Choose soft/mixed/hard with confirmation'),
    actionButton('Tag this commit…',()=>gitGraphTag(entry)),
    actionButton('Compare with HEAD',()=>renderGitGraphHeadCompare({sha:entry.sha,short:entry.short,subject:entry.subject,author:entry.actor,date:entry.date,parents:[],refs:[]},host))
  );
  host.append(more);
}
function gitLostCommitSection(detailHost){
  const section=el('section','git-recovery-lost');
  const head=el('div','git-recovery-lost-head');
  const title=el('strong','','Unreachable / lost commits');
  const scan=actionButton('Scan lost commits',async()=>{
    const params={limit:'200'};
    if(String(gitReflogFilters.search||'').trim())params.search=String(gitReflogFilters.search).trim();
    const data=await gitView('lost-commits',params);if(!data)return false;
    list.replaceChildren();
    const commits=Array.isArray(data.commits)?data.commits:[];
    if(!commits.length){
      list.append(el('div','git-empty','No unreachable commit objects found outside current refs.'));
      return true;
    }
    for(const commit of commits){
      const row=el('div','git-row');
      const code=el('span','git-row-code',commit.short||String(commit.sha||'').slice(0,8));
      const main=el('div','git-row-main');
      const visibility=commit.reflog_visible?'also visible in reflog':'not visible in reflog';
      main.append(
        el('div','git-row-title',commit.subject||'(no subject)'),
        el('div','git-row-sub',[commit.date,commit.author,visibility].filter(Boolean).join(' · '))
      );
      const actions=el('div','git-row-actions');
      actions.append(
        actionButton('Recover branch',()=>gitReflogCreateBranch(commit),'Create a branch ref without switching or changing the worktree'),
        actionButton('Compare HEAD',()=>renderGitGraphHeadCompare({sha:commit.sha,short:commit.short,subject:commit.subject,author:commit.author,date:commit.date,parents:[],refs:[]},detailHost)),
        actionButton('Copy SHA',()=>copyText(commit.sha))
      );
      row.append(code,main,actions);list.append(row);
    }
    return true;
  },'Run git fsck --no-reflogs --unreachable and inspect unreachable commit objects');
  head.append(title,scan);section.append(head);
  section.append(el('div','git-recovery-lost-note','Use this scan after force-deleting a branch or losing a commit that no reflog entry can find. Recovery only creates a branch ref; it does not modify HEAD, Index, or Working tree.'));
  const list=el('div','git-recovery-lost-list');list.append(el('div','git-empty','Lost-commit scan has not been run.'));section.append(list);
  return section;
}
async function loadReflog(){
  setGitPanelWide(true);
  const params={limit:'300'};for(const [key,value] of Object.entries(gitReflogFilters))if(String(value||'').trim())params[key]=String(value).trim();
  const data=await gitView('reflog',params);if(!data)return false;
  content.replaceChildren();content.append(gitReflogControls(loadReflog));
  const shell=el('div','git-graph-shell'),list=el('div','git-graph-list'),detail=el('div','git-graph-detail');
  content.append(gitLostCommitSection(detail));
  shell.append(list,detail);content.append(shell);
  const entries=Array.isArray(data.entries)?data.entries:[];
  if(!entries.length){list.append(el('div','git-empty','No reflog entries match the current filters'));detail.append(el('div','git-empty','Select a reflog entry'));return true;}
  if(!entries.some(item=>(item.selector+'\x00'+item.sha)===gitReflogSelected))gitReflogSelected=entries[0].selector+'\x00'+entries[0].sha;
  for(const entry of entries){
    const key=entry.selector+'\x00'+entry.sha,row=el('div','git-row');row.classList.toggle('selected',key===gitReflogSelected);
    const code=el('span','git-row-code',entry.kind||'');const main=el('div','git-row-main');
    main.append(el('div','git-row-title',entry.subject||'(no subject)'),el('div','git-row-sub',[entry.selector,entry.short,entry.actor,entry.date].filter(Boolean).join(' · ')));
    const actions=el('div','git-row-actions');actions.append(actionButton('Recover branch',()=>gitReflogCreateBranch(entry)),actionButton('Copy SHA',()=>copyText(entry.sha)));
    row.onclick=event=>{if(event.target.closest('button'))return;gitReflogSelected=key;list.querySelectorAll('.git-row').forEach(node=>node.classList.toggle('selected',node===row));renderGitReflogDetail(entry,detail).catch(app.showError);};
    row.append(code,main,actions);list.append(row);
  }
  const selected=entries.find(item=>(item.selector+'\x00'+item.sha)===gitReflogSelected)||entries[0];
  await renderGitReflogDetail(selected,detail);return true;
}


const gitRebaseActions=['pick','reword','edit','squash','fixup','drop'];
function normalizeRebasePlan(commits){
  const previous=new Map((gitRebasePlanState||[]).map(item=>[item.sha,item]));
  gitRebasePlanState=(commits||[]).map(commit=>{
    const saved=previous.get(commit.sha);
    return {sha:commit.sha,short:commit.short,subject:commit.subject,author:commit.author,date:commit.date,merge:Boolean(commit.merge),action:saved?.action||'pick',message:saved?.message||commit.subject};
  });
}
function validateRebasePlanClient(plan){
  let kept=0;
  for(let i=0;i<plan.length;i++){
    const item=plan[i],action=String(item.action||'pick');
    if(!gitRebaseActions.includes(action))return 'Unsupported action at row '+(i+1);
    if((action==='squash'||action==='fixup')&&kept===0)return action+' cannot be the first non-dropped commit';
    if(action!=='drop')kept++;
    if(action==='reword'&&!String(item.message||'').trim())return 'Reword commit '+item.short+' needs a non-empty message';
  }
  if(kept===0)return 'Plan drops every commit; use reset/recovery tools instead of an empty interactive rebase.';
  return '';
}
function projectedRebaseHistory(plan){
  const rows=[];
  for(const item of plan){
    const action=item.action||'pick';
    if(action==='drop')continue;
    if(action==='squash'||action==='fixup'){
      const prior=rows[rows.length-1];
      if(prior){
        prior.folded.push({action,short:item.short,subject:item.subject});
        if(action==='squash')prior.subject=prior.subject+' + '+item.subject;
      }
      continue;
    }
    rows.push({
      short:item.short,
      subject:action==='reword'?(String(item.message||'').trim()||item.subject):item.subject,
      action,folded:[],stop:action==='edit'
    });
  }
  return rows;
}
function renderRebasePreview(host,meta){
  host.replaceChildren();
  host.append(el('div','git-row-sub','Projected history · '+meta.branch+' · base '+String(meta.base_sha||'').slice(0,12)));
  const problem=validateRebasePlanClient(gitRebasePlanState);
  if(problem)host.append(el('div','git-rebase-warning',problem));
  for(const item of projectedRebaseHistory(gitRebasePlanState)){
    const row=el('div','git-rebase-preview-row');
    row.append(el('strong','',item.subject),el('div','',[item.action,item.short,item.stop?'stops for edit':'',item.folded.length?(item.folded.length+' folded commit(s)'):''].filter(Boolean).join(' · ')));
    if(item.folded.length){
      row.append(el('div','',item.folded.map(x=>x.action+' '+x.short+' '+x.subject).join(' | ')));
    }
    host.append(row);
  }
}
function renderRebasePlanList(host,preview,meta){
  host.replaceChildren();
  gitRebasePlanState.forEach((item,index)=>{
    const row=el('div','git-rebase-item');row.draggable=true;row.dataset.sha=item.sha;
    const drag=el('span','git-rebase-drag','↕');drag.title='Drag to reorder';
    const select=document.createElement('select');select.className='git-rebase-action';
    for(const actionName of gitRebaseActions){const option=document.createElement('option');option.value=actionName;option.textContent=actionName;select.append(option);}
    select.value=item.action;
    select.onchange=()=>{item.action=select.value;message.hidden=item.action!=='reword';renderRebasePreview(preview,meta);};
    const main=el('div','git-rebase-main');
    main.append(el('div','git-row-title',(item.merge?'MERGE · ':'')+item.subject),el('div','git-row-sub',[item.short,item.author,item.date].filter(Boolean).join(' · ')));
    const message=document.createElement('input');message.className='git-rebase-message';message.value=item.message||item.subject;message.hidden=item.action!=='reword';message.placeholder='New commit message';
    message.oninput=()=>{item.message=message.value;renderRebasePreview(preview,meta);};main.append(message);
    row.append(drag,select,main);
    row.ondragstart=event=>{row.classList.add('dragging');event.dataTransfer?.setData('text/plain',item.sha);if(event.dataTransfer)event.dataTransfer.effectAllowed='move';};
    row.ondragend=()=>row.classList.remove('dragging');
    row.ondragover=event=>{event.preventDefault();if(event.dataTransfer)event.dataTransfer.dropEffect='move';};
    row.ondrop=event=>{
      event.preventDefault();
      const fromSHA=event.dataTransfer?.getData('text/plain')||'';
      const from=gitRebasePlanState.findIndex(x=>x.sha===fromSHA),to=gitRebasePlanState.findIndex(x=>x.sha===item.sha);
      if(from<0||to<0||from===to)return;
      const [moved]=gitRebasePlanState.splice(from,1);gitRebasePlanState.splice(to,0,moved);
      renderRebasePlanList(host,preview,meta);renderRebasePreview(preview,meta);
    };
    host.append(row);
  });
}
function chooseDefaultRebaseBase(branches){
  if(gitRebaseBase)return gitRebaseBase;
  const current=(branches.local||[]).find(branch=>branch.current);
  if(current?.upstream)return current.upstream;
  const main=(branches.local||[]).find(branch=>['main','master','develop'].includes(branch.name)&&!branch.current);
  return main?.name||'';
}
async function loadRebase(){
  setGitPanelWide(true);
  const branches=await gitView('branches');if(!branches)return false;
  content.replaceChildren();
  const controls=el('div','git-rebase-controls');
  const base=document.createElement('input');base.placeholder='Base branch or full commit SHA (exclusive)';base.value=chooseDefaultRebaseBase(branches);base.spellcheck=false;
  const suggestions=document.createElement('select');suggestions.title='Choose a local/remote branch as base';
  const emptyOption=document.createElement('option');emptyOption.value='';emptyOption.textContent='Choose branch…';suggestions.append(emptyOption);
  for(const branch of [...(branches.local||[]),...(branches.remote||[])]){if(branch.current)continue;const option=document.createElement('option');option.value=branch.name;option.textContent=(branch.remote?'remote · ':'local · ')+branch.name;suggestions.append(option);}
  suggestions.onchange=()=>{if(suggestions.value)base.value=suggestions.value;};
  const load=actionButton('Load plan',async()=>{
    const value=base.value.trim();if(!value)throw new Error('Choose a base branch or full commit SHA');
    gitRebaseBase=value;
    const data=await gitView('rebase-plan',{base:value,limit:'150'});if(!data)return false;
    normalizeRebasePlan(data.commits||[]);
    renderRebaseWorkspace(data,controls);
    return true;
  });
  base.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();load.onclick();}});
  controls.append(base,suggestions,load);content.append(controls);
  if(gitRebaseBase)await load.onclick();
  else content.append(el('div','git-empty','Choose the commit immediately before the range you want to rewrite. Planner is read-only until you explicitly start a rebase.'));
  return true;
}
async function runRebasePauseRepair(meta,repair,label){
  const confirmText=repair==='abort_in_progress'
    ?'Abort the active interactive rebase and restore the branch to its original pre-rebase state?'
    :'Continue the active interactive rebase from the current stop?';
  const data=await action('repair',{repair,confirmed:repair==='abort_in_progress'},confirmText,{refresh:false});
  if(data?.conflict_state?.operation==='rebase'){
    renderRebasePausedControls(meta,data);
    return data;
  }
  await refresh();
  await loadRebase();
  return data;
}
function renderRebasePausedControls(meta,data){
  const box=el('div','git-rebase-warning');
  box.append(el('div','git-row-title','Interactive rebase is paused'));
  box.append(el('div','git-row-sub',data?.output||'Resolve/edit the current stop, then continue or abort.'));
  const actions=el('div','git-row-actions');
  actions.append(
    actionButton('Continue rebase',()=>runRebasePauseRepair(meta,'continue_in_progress','Continue rebase')),
    actionButton('Abort rebase',()=>runRebasePauseRepair(meta,'abort_in_progress','Abort rebase'))
  );
  box.append(actions);
  content.append(box);
}
function structuredRebasePlan(){
  return gitRebasePlanState.map(item=>({sha:item.sha,action:item.action,message:item.action==='reword'?String(item.message||''):''}));
}
async function startInteractiveRebase(meta){
  const problem=validateRebasePlanClient(gitRebasePlanState);
  if(problem)throw new Error(problem);
  if(!meta.clean)throw new Error('Working tree and index must be clean before starting interactive rebase');
  if(meta.has_merge_commits)throw new Error('Merge-containing ranges are not yet supported for interactive rebase execution');
  if(meta.truncated)throw new Error('The selected range is truncated; choose a closer base before executing');
  const plan=structuredRebasePlan();
  const confirmText='Rewrite '+plan.length+' commit(s) on '+meta.branch+' using interactive rebase?\n\nBase: '+String(meta.base_sha||'').slice(0,12)+'\nHEAD: '+String(meta.head_sha||'').slice(0,12)+'\n\nThis rewrites commit history. Push may require force-with-lease afterward.';
  const data=await action('interactive_rebase',{
    ref:meta.base_sha,
    expected_sha:meta.head_sha,
    expected_current:meta.branch,
    rebase_plan:plan
  },confirmText,{refresh:false});
  if(data?.conflict_state?.operation==='rebase'){
    renderRebasePausedControls(meta,data);
    return data;
  }
  await refresh();
  await loadRebase();
  return data;
}
function renderRebaseWorkspace(meta,controls){
  content.replaceChildren(controls);
  const summary=el('div','git-row-sub',[meta.branch,'base '+String(meta.base_sha||'').slice(0,12),'HEAD '+String(meta.head_sha||'').slice(0,12),(meta.commits||[]).length+' commit(s)'].join(' · '));content.append(summary);
  if(!meta.clean)content.append(el('div','git-rebase-warning','Working tree is dirty. Planning is allowed, but execution will require a clean Index/Working tree.'));
  if(meta.has_merge_commits)content.append(el('div','git-rebase-warning','This range contains merge commits. The first execution milestone will refuse merge-containing plans until --rebase-merges semantics are explicitly supported.'));
  if(meta.truncated)content.append(el('div','git-rebase-warning','Range is truncated. Choose a closer base before executing.'));
  const shell=el('div','git-rebase-shell'),plan=el('div','git-rebase-plan'),preview=el('div','git-rebase-preview');shell.append(plan,preview);content.append(shell);
  renderRebasePlanList(plan,preview,meta);renderRebasePreview(preview,meta);
  const status=el('div','git-row-sub','Structured plan is validated again server-side against branch/base/HEAD/full SHA before Git starts.');
  const execute=actionButton('Start interactive rebase',()=>startInteractiveRebase(meta),'Execute this structured plan using TaskDeck\'s backend-controlled sequence/message editor');
  const problem=validateRebasePlanClient(gitRebasePlanState);
  execute.disabled=Boolean(problem)||!meta.clean||Boolean(meta.has_merge_commits)||Boolean(meta.truncated);
  execute.title=problem||(!meta.clean?'Clean the Index/Working tree before execution':meta.has_merge_commits?'Merge-containing ranges are not yet supported':meta.truncated?'Choose a closer base so the full range is loaded':'Rewrite the selected commits');
  const footer=el('div','git-row-actions');footer.append(execute);content.append(status,footer);
}
async function loadLog(){const data=await gitView('log',{limit:'50'});if(!data)return false;content.replaceChildren();for(const commit of data.commits||[]){const row=el('div','git-row');const code=el('span','git-row-code',commit.short);const main=el('div','git-row-main');main.append(el('div','git-row-title',commit.subject),el('div','git-row-sub',commit.date+' · '+commit.author));const actions=el('div','git-row-actions');
    actions.append(
      actionButton('Cherry-pick',()=>action('cherry_pick',{ref:commit.sha,expected_sha:commit.sha},'Cherry-pick '+commit.short+' onto the current branch? The working tree must be clean.'),'Apply this commit onto the current branch'),
      actionButton('Revert…',()=>openGitCommitSemantics(commit,'revert'),'Open reset/revert semantics wizard'),
      actionButton('Copy SHA',()=>copyText(commit.sha))
    );
    row.append(code,main,actions);content.append(row);}if(!content.childElementCount)empty('No commits');}
function setGitFilePath(path){
  path=String(path||'').trim();
  if(path!==gitFilePath)gitFileCompareRef='';
  gitFilePath=path;
}
function openGitFileView(path,mode='history'){
  setGitFilePath(path);
  gitFileMode=mode==='blame'?'blame':'history';
  currentView='file-history';updateNav();
  return loadFileHistory();
}
function gitFileControls(){
  const controls=el('div','git-file-controls');
  const input=document.createElement('input');input.value=gitFilePath;input.placeholder='project-relative file path';input.spellcheck=false;
  const history=actionButton('History',()=>{setGitFilePath(input.value);gitFileMode='history';return loadFileHistory();});
  const blame=actionButton('Blame',()=>{setGitFilePath(input.value);gitFileMode='blame';return loadFileHistory();});
  input.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();setGitFilePath(input.value);loadFileHistory().catch(app.showError);}});
  controls.append(input,history,blame);
  if(gitFileCompareRef)controls.append(el('span','git-row-sub','Compare A: '+gitFileCompareRef.slice(0,12)));
  return controls;
}
async function loadFileHistory(){
  content.replaceChildren();content.append(gitFileControls());
  if(!gitFilePath){content.append(el('div','git-empty','Enter a tracked project-relative file path to inspect its history or blame.'));return true;}
  if(gitFileMode==='blame'){
    const data=await gitView('blame',{path:gitFilePath});if(!data)return false;
    const rows=Array.isArray(data.lines)?data.lines:[];
    for(const item of rows){
      const row=el('div','git-row');const code=el('span','git-row-code',String(item.line||''));
      const main=el('div','git-row-main');
      const when=item.author_time?new Date(Number(item.author_time)*1000).toISOString().slice(0,10):'';
      main.append(el('div','git-row-title',item.text||''),el('div','git-row-sub',[item.committed===false?'uncommitted':item.short,item.author,when,item.summary].filter(Boolean).join(' · ')));
      const actions=el('div','git-row-actions');if(item.sha)actions.append(actionButton('Copy SHA',()=>copyText(item.sha)));row.append(code,main,actions);content.append(row);
    }
    if(!rows.length)content.append(el('div','git-empty','No blame lines available for '+gitFilePath));
    return true;
  }
  const data=await gitView('file-history',{path:gitFilePath,limit:'100'});if(!data)return false;
  const rows=Array.isArray(data.commits)?data.commits:[];
  for(const commit of rows){
    const row=el('div','git-row');const code=el('span','git-row-code',commit.short);const main=el('div','git-row-main');
    main.append(el('div','git-row-title',commit.subject),el('div','git-row-sub',commit.date+' · '+commit.author));
    const actions=el('div','git-row-actions');
    actions.append(actionButton('Compare current',()=>{
      const compare=globalThis.TaskMenuFileCompare;if(!compare?.openGitCommitAgainstProject)throw new Error('File Compare unavailable');
      return compare.openGitCommitAgainstProject(activeRepoID,gitFilePath,workspacePathForActiveRepository(gitFilePath),commit.sha);
    },'Compare this committed file version with the current editor buffer when open, otherwise the working project file'));
    actions.append(actionButton(gitFileCompareRef===commit.sha?'Compare A ✓':'Use as Compare A',()=>{gitFileCompareRef=commit.sha;return loadFileHistory();},'Select this commit as the left side for commit-to-commit compare'));
    if(gitFileCompareRef&&gitFileCompareRef!==commit.sha)actions.append(actionButton('Compare A ↔ this',()=>{
      const compare=globalThis.TaskMenuFileCompare;if(!compare?.openGitCommits)throw new Error('File Compare unavailable');
      return compare.openGitCommits(activeRepoID,gitFilePath,gitFileCompareRef,commit.sha);
    },'Compare selected commit A with this commit'));
    actions.append(actionButton('Restore…',()=>openGitFileRestoreSemantics(commit,gitFilePath),'Restore Working tree file or Index-only from this commit with semantics preview'));
    actions.append(actionButton('Copy SHA',()=>copyText(commit.sha)));
    row.append(code,main,actions);content.append(row);
  }
  if(!rows.length)content.append(el('div','git-empty','No committed history found for '+gitFilePath));
  return true;
}
async function loadAheadBehind(){const data=await gitView('ahead-behind');if(!data)return false;content.replaceChildren();content.append(el('div','git-row-sub',data.upstream?`Upstream ${data.upstream} · ahead ${data.ahead} · behind ${data.behind}`:'No upstream configured'));for(const commit of data.commits||[]){const row=el('div','git-row');const code=el('span','git-row-code '+(commit.direction==='ahead'?'git-direction-ahead':'git-direction-behind'),commit.direction==='ahead'?'↑':'↓');const main=el('div','git-row-main');main.append(el('div','git-row-title',commit.subject),el('div','git-row-sub',commit.sha));const actions=el('div','git-row-actions');actions.append(actionButton('Copy SHA',()=>copyText(commit.sha)));row.append(code,main,actions);content.append(row);}}
async function loadStashes(){const data=await gitView('stashes');if(!data)return false;content.replaceChildren();const create=actionButton('Create stash',()=>stashPush());content.append(create);for(const stash of data.stashes||[]){const row=el('div','git-row');const code=el('span','git-row-code',stash.ref);const main=el('div','git-row-main');main.append(el('div','git-row-title',stash.subject),el('div','git-row-sub',stash.when+' · '+stash.sha.slice(0,8)));const actions=el('div','git-row-actions');actions.append(actionButton('Pop',()=>action('stash_pop',{ref:stash.ref},`Pop ${stash.ref}? This may create conflicts if the worktree has changed.`)),actionButton('Copy ref',()=>copyText(stash.ref)));row.append(code,main,actions);content.append(row);}}
async function loadCompare(base=''){
  currentView='compare';updateNav();const branches=await gitView('branches');if(!branches)return false;content.replaceChildren();const controls=el('div','git-compare-controls');const select=document.createElement('select');for(const branch of [...(branches.local||[]),...(branches.remote||[])]){if(branch.current)continue;const o=document.createElement('option');o.value=branch.name;o.textContent=branch.name;select.append(o);}if(base&&[...select.options].some(o=>o.value===base))select.value=base;const run=actionButton('Compare',async()=>{if(!select.value)return false;const data=await gitView('compare',{base:select.value});if(!data)return false;renderCompare(data,controls);return true;});controls.append(select,run);content.append(controls);if(base&&select.value)await run.onclick();
}
function renderCompare(data,controls){content.replaceChildren(controls);content.append(el('strong','',`Compare ${data.base}...HEAD`),el('pre','git-compare-pre',(data.stat||'(no differences)')+'\n'+(data.files||'')));}
async function loadCurrentView(){setGitPanelWide(false);updateNav();if(currentView==='repositories')return loadRepositories(false);if(!currentStatus?.repository)return empty('Not a Git repository');switch(currentView){case 'changes':return loadChanges();case 'branches':return loadBranches();case 'worktrees':return loadWorktrees();case 'submodules':return loadSubmodules();case 'tags':return loadTags();case 'log':return loadLog();case 'graph':return loadGraph();case 'reflog':return loadReflog();case 'rebase':return loadRebase();case 'file-history':return loadFileHistory();case 'ahead-behind':return loadAheadBehind();case 'stashes':return loadStashes();case 'compare':return loadCompare();}}

function workspacePathForActiveRepository(pathValue){
  pathValue=String(pathValue||'').replace(/\\/g,'/').replace(/^\.\//,'');
  const repo=activeRepository();
  const root=repo?.path==='.'?'':String(repo?.path||repo?.id||'').replace(/\\/g,'/').replace(/^\.\//,'').replace(/\/+$/,'');
  return root?(root+'/'+pathValue):pathValue;
}
function repositoryForProjectPath(pathValue){
  const value=String(pathValue||'').replace(/\\/g,'/').replace(/^\.\//,'').replace(/^\/+|\/+$/g,'');
  let best=null,bestRoot='';
  for(const item of repositories){
    const root=item.path==='.'?'':String(item.path||item.id||'').replace(/\\/g,'/').replace(/^\.\//,'').replace(/\/+$/,'');
    if(root===''||value===root||value.startsWith(root+'/')){
      if(!best||root.length>bestRoot.length){best=item;bestRoot=root;}
    }
  }
  return best?{repo:best,root:bestRoot}:null;
}
async function openWorkspaceFileView(pathValue,mode='history'){
  pathValue=String(pathValue||'').replace(/\\/g,'/').replace(/^\.\//,'');
  if(!pathValue)throw new Error('Project file path is required');
  if(!repositories.length)await refreshRepositories(false);
  let match=repositoryForProjectPath(pathValue);
  if(!match){
    await refreshRepositories(true);
    match=repositoryForProjectPath(pathValue);
  }
  if(!match)throw new Error('No Git repository contains '+pathValue);
  await selectRepository(match.repo.id,{reload:false});
  await refresh();
  panel.classList.add('visible');
  const repoPath=match.root?pathValue.slice(match.root.length+1):pathValue;
  return openGitFileView(repoPath,mode);
}
globalThis.TaskMenuGitFiles={
  open(){if(!panel.classList.contains('visible'))pill.click();return true;},
  close(){panel.classList.remove('visible');return true;},
  async openGraphSearch(query=''){
    panel.classList.add('visible');
    await refreshRepositories(false);
    if(app.views.has(String(app.active||'')))await autoSelectRepositoryForTerminal(app.active,{reload:false});
    await refresh();
    currentView='graph';
    gitGraphFilters={...gitGraphFilters,search:String(query||'').trim()};
    gitGraphSelectedSHA='';
    updateNav();
    await loadGraph();
    return true;
  },
  snapshotState(){
    const repo=activeRepository();
    return {repository_id:String(activeRepoID||''),branch:String(repo?.branch||currentStatus?.branch||''),head:String(currentStatus?.head||'')};
  },
  async restoreState(state){
    const id=String(state?.repository_id||'').trim();
    if(!id)return false;
    if(!repositories.some(item=>item.id===id))await refreshRepositories(true);
    const selected=await selectRepository(id,{reload:false,persist:true});
    if(selected){await refresh();await loadCurrentView();}
    return selected;
  },
  openWorkspaceFileView,
  repositoryForProjectPath,
  runRepair:(repair,payload={},repoID=activeRepoID)=>repairAction(repair,payload,repoID),
  refresh:async()=>{await refresh();if(currentView==='changes')await loadChanges();}
};

repoSelect.onchange=()=>selectRepository(repoSelect.value).catch(app.showError);
repoRescan.onclick=async()=>{try{await refreshRepositories(true);await refresh();await loadCurrentView();}catch(error){app.showError(error);}};
pill.onclick=async()=>{
  if(!canViewGitPanel())return;
  panel.classList.toggle('visible');
  if(!panel.classList.contains('visible'))return;
  try{
    // A workspace root need not be a Git repository. Rescan its descendants
    // when opening the panel without a known repo (e.g. after a child clone).
    await refreshRepositories(!repositories.length);
    if(app.views.has(String(app.active||'')))await autoSelectRepositoryForTerminal(app.active,{reload:false});
    await refresh();
    if(!repositories.length)currentView='repositories';
    await loadCurrentView();
  }catch(error){
    showGitFallback('Git · Unavailable','Git discovery failed: '+String(error?.message||error));
    empty('Cannot load Git repositories: '+String(error?.message||error)+'. Use Scan to retry.');
    app.showError(error);
  }
};panelClose.onclick=()=>panel.classList.remove('visible');
window.addEventListener('focus',refresh);
window.addEventListener('taskmenu:session',event=>{const meta=event.detail?.meta;if(meta&&meta.status!=='running')setTimeout(refresh,150);});
window.addEventListener('taskmenu:view-activated',event=>{if(event.detail?.kind==='terminal')autoSelectRepositoryForTerminal(event.detail.id).catch(app.showError);});
setInterval(refresh,5000);
if(canViewGitPanel())refreshRepositories(false).then(async()=>{if(app.views.has(String(app.active||'')))await autoSelectRepositoryForTerminal(app.active,{reload:false});return refresh();}).catch(error=>{console.warn('Git repository discovery failed',error);return refresh();});
updateNav();