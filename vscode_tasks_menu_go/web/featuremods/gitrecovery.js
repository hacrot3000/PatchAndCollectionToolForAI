const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Git recovery');

const style=document.createElement('style');
style.textContent=`
.git-recovery-backdrop{position:fixed;inset:0;z-index:18000;display:none;align-items:center;justify-content:center;background:rgba(0,0,0,.62);padding:18px}
.git-recovery-backdrop.visible{display:flex}
.git-recovery-dialog{width:min(820px,96vw);max-height:90vh;display:flex;flex-direction:column;background:#11161d;border:1px solid #4a5362;border-radius:11px;box-shadow:0 22px 70px rgba(0,0,0,.6);overflow:hidden}
.git-recovery-head{display:flex;align-items:flex-start;gap:10px;padding:13px 14px;border-bottom:1px solid #30343b}
.git-recovery-head-main{flex:1;min-width:0}.git-recovery-title{font-size:15px;font-weight:700}.git-recovery-subtitle{margin-top:4px;font-size:11px;opacity:.68;word-break:break-word}
.git-recovery-close{padding:4px 8px}
.git-recovery-body{padding:12px 14px;overflow:auto}
.git-recovery-summary{padding:9px 10px;border:1px solid #5a4030;border-radius:7px;background:#241b17;line-height:1.45;font-size:12px}
.git-recovery-error{margin-top:9px;white-space:pre-wrap;word-break:break-word;font:11px/1.45 ui-monospace,monospace;background:#090d12;border:1px solid #30343b;border-radius:7px;padding:9px;max-height:170px;overflow:auto}
.git-recovery-options{display:grid;grid-template-columns:repeat(auto-fit,minmax(310px,1fr));gap:9px;margin-top:11px}
.git-recovery-option{border:1px solid #30343b;border-radius:8px;background:#151b23;padding:10px;display:flex;flex-direction:column;gap:7px}
.git-recovery-option-title{font-size:12px;font-weight:700}.git-recovery-option-desc{font-size:11px;line-height:1.45;opacity:.75}
.git-recovery-inputs{display:grid;gap:6px}.git-recovery-field{display:grid;gap:3px}.git-recovery-field label{font-size:10px;font-weight:700;opacity:.65}.git-recovery-field input{width:100%;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:6px 7px;font:11px ui-monospace,monospace}
.git-recovery-option-actions{display:flex;justify-content:flex-end;gap:6px;margin-top:auto}.git-recovery-option button{padding:5px 8px}.git-recovery-option button.running{opacity:.7}
.git-recovery-risk{font-size:10px;color:#ffbf69}.git-recovery-result{margin-top:10px;padding:8px 9px;border-radius:7px;border:1px solid #365f84;background:#0d1721;white-space:pre-wrap;font:11px/1.4 ui-monospace,monospace;display:none}.git-recovery-result.visible{display:block}
.git-recovery-foot{display:flex;align-items:center;justify-content:space-between;gap:8px;padding:10px 14px;border-top:1px solid #30343b}.git-recovery-foot-left,.git-recovery-foot-right{display:flex;gap:7px;align-items:center}
.git-recovery-foot button{padding:5px 8px}
html[data-taskmenu-theme="light"] .git-recovery-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .git-recovery-summary{background:#fff7ed;border-color:#e4c39e}html[data-taskmenu-theme="light"] .git-recovery-error{background:#f6f8fa;border-color:#d0d7de;color:#202124}html[data-taskmenu-theme="light"] .git-recovery-option{background:#f8fafc;border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-recovery-field input{background:#fff;border-color:#b9c0c8;color:#202124}html[data-taskmenu-theme="light"] .git-recovery-result{background:#eef6ff;border-color:#9ebfe0}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='git-recovery-backdrop';
const dialog=document.createElement('div');dialog.className='git-recovery-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label','Git recovery wizard');
const head=document.createElement('div');head.className='git-recovery-head';
const headMain=document.createElement('div');headMain.className='git-recovery-head-main';
const title=document.createElement('div');title.className='git-recovery-title';
const subtitle=document.createElement('div');subtitle.className='git-recovery-subtitle';
const close=document.createElement('button');close.className='git-recovery-close';close.textContent='×';close.title='Close Git recovery wizard';
headMain.append(title,subtitle);head.append(headMain,close);
const body=document.createElement('div');body.className='git-recovery-body';
const summary=document.createElement('div');summary.className='git-recovery-summary';
const raw=document.createElement('pre');raw.className='git-recovery-error';
const optionsHost=document.createElement('div');optionsHost.className='git-recovery-options';
const result=document.createElement('div');result.className='git-recovery-result';
body.append(summary,raw,optionsHost,result);
const foot=document.createElement('div');foot.className='git-recovery-foot';
const footLeft=document.createElement('div');footLeft.className='git-recovery-foot-left';
const footRight=document.createElement('div');footRight.className='git-recovery-foot-right';
const copy=document.createElement('button');copy.textContent='Copy details';
const retry=document.createElement('button');retry.textContent='Retry original';
const dismiss=document.createElement('button');dismiss.textContent='Close';
footLeft.append(copy);footRight.append(retry,dismiss);foot.append(footLeft,footRight);
dialog.append(head,body,foot);backdrop.append(dialog);document.body.append(backdrop);

let current=null;

function closeWizard(){backdrop.classList.remove('visible');current=null;result.classList.remove('visible');result.textContent='';}
close.onclick=closeWizard;dismiss.onclick=closeWizard;
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)closeWizard();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))closeWizard();});

function combinedText(ctx){
  return [ctx?.error,ctx?.output].filter(Boolean).join('\n').trim();
}
function classifyLocal(ctx){
  const text=combinedText(ctx).toLowerCase();
  if(/detected dubious ownership/.test(text))return 'dubious_ownership';
  if(/another git process seems to be running|index\.lock|unable to create .*\.git\/.*\.lock/.test(text))return 'index_lock';
  if(/author identity unknown|please tell me who you are|unable to auto-detect email address/.test(text))return 'identity_missing';
  if(/gpg failed to sign|failed to sign the data|signing failed/.test(text))return 'gpg_signing';
  if(/permission denied \(publickey|could not read from remote repository[\s\S]*permission denied/.test(text))return 'ssh_auth';
  if(/host key verification failed|remote host identification has changed/.test(text))return 'ssh_host_key';
  if(/authentication failed|could not read username|could not read password|http basic: access denied/.test(text))return 'https_auth';
  if(/repository not found|requested url returned error: 403|permission to .* denied/.test(text))return 'remote_permission';
  if(/could not resolve host|temporary failure in name resolution|name or service not known/.test(text))return 'network_dns';
  if(/failed to connect|connection timed out|connection refused|network is unreachable/.test(text))return 'network_connect';
  if(/ssl certificate problem|certificate verify failed|\btls\b/.test(text))return 'tls';
  if(/rpc failed|http\/2 stream|early eof|remote end hung up unexpectedly/.test(text))return 'transport';
  if(/has no upstream branch/.test(text))return 'no_upstream_push';
  if(/there is no tracking information for the current branch|no upstream configured/.test(text))return 'no_upstream_pull';
  if(/non-fast-forward|fetch first|updates were rejected because the remote contains work/.test(text))return 'push_non_fast_forward';
  if(/not possible to fast-forward|divergent branches|need to specify how to reconcile/.test(text))return 'pull_diverged';
  if(/would be overwritten by merge|would be overwritten by checkout|please commit your changes or stash them/.test(text))return 'dirty_worktree';
  if(/you have unmerged files|needs merge|fix conflicts and then commit|resolve all conflicts manually/.test(text))return 'conflicts';
  if(/merge_head exists|you have not concluded your merge/.test(text))return 'merge_in_progress';
  if(/rebase in progress|rebase-merge|rebase-apply/.test(text))return 'rebase_in_progress';
  if(/cherry-pick is currently in progress|cherry_pick_head/.test(text))return 'cherry_pick_in_progress';
  if(/protected branch|protected branch hook declined|gh013|pre-receive hook declined/.test(text))return 'protected_branch';
  if(/src refspec .* does not match any/.test(text))return 'refspec_missing';
  if(/couldn't find remote ref|remote ref does not exist/.test(text))return 'remote_ref_missing';
  if(/already exists/.test(text)&&/branch/.test(text))return 'branch_exists';
  if(/you are not currently on a branch|detached head/.test(text))return 'detached_head';
  if(/refusing to merge unrelated histories/.test(text))return 'unrelated_histories';
  if(/exceeds github's file size limit|gh001|large files detected/.test(text))return 'file_too_large';
  if(/no space left on device|disk quota exceeded/.test(text))return 'disk_full';
  if(/bad object|object file .* is empty|corrupt loose object|invalid object/.test(text))return 'repository_corrupt';
  if(/hook/.test(text)&&/failed|declined|exit code/.test(text))return 'hook_failed';
  if(/not a git repository/.test(text))return 'not_git_repository';
  if(/git operation timed out/.test(text))return 'timeout';
  if(/permission denied|operation not permitted/.test(text))return 'filesystem_permission';
  return 'generic';
}

function input(key,label,placeholder='',value='',required=false,type='text'){
  return {key,label,placeholder,value,required,type};
}
function option(label,description,run,{risk='',inputs=[]}={}){
  return {label,description,run,risk,inputs};
}
function repair(ctx,id,payload={}){
  return ctx.runRepair(id,{...payload,original_action:ctx.actionName});
}
function retryOriginal(ctx){return ctx.retry();}
function remoteInput(){return input('remote','Remote (optional)','origin');}
function branchInput(ctx,label='New branch'){
  const base=String(ctx.repository?.branch||'').trim();
  const suggested=base&&base!=='DETACHED'?base+'-recovery':'recovered-work';
  return input('new_branch',label,'feature/recovery',suggested,true);
}
function remoteURLInputs(){
  return [remoteInput(),input('remote_url','New remote URL','git@github.com:owner/repository.git','',true)];
}
function withValues(base,values){return {...base,...values};}

function issueFor(ctx){
  const code=ctx.failureCode||classifyLocal(ctx);
  const retryOpt=option('Retry original command','Run the failed Git action again without changing repository state.',()=>retryOriginal(ctx));
  const statusOpt=option('Inspect repository status','Run git status --short --branch and show the result.',()=>repair(ctx,'status'));
  const remoteCheck=option('Test remote access','Run git ls-remote --heads against the selected/default remote.',values=>repair(ctx,'remote_check',values),{inputs:[remoteInput()]});
  const setRemote=option('Change remote URL','Update the selected/default Git remote URL, then you can retry.',values=>repair(ctx,'set_remote_url',values),{inputs:remoteURLInputs(),risk:'This changes repository remote configuration.'});
  const fetch=option('Fetch remote refs','Run git fetch --prune to refresh remote state.',values=>repair(ctx,'fetch',values),{inputs:[remoteInput()]});

  switch(code){
    case 'no_upstream_push':
      return {code,title:'Current branch has no upstream',summary:'Git cannot decide where to push this branch. You can publish it and set the upstream automatically.',options:[
        option('Publish branch + set upstream','Push the current branch to the selected/default remote with --set-upstream.',values=>repair(ctx,'push_set_upstream',values),{inputs:[remoteInput()]}),
        remoteCheck,setRemote,retryOpt
      ]};
    case 'no_upstream_pull':
      return {code,title:'Current branch has no tracking branch',summary:'Pull needs an upstream branch. TaskDeck can fetch, connect the matching remote branch, and retry.',options:[
        option('Fetch + set upstream + retry','Fetch refs, set upstream to <remote>/<current branch>, then retry the original command.',async values=>{await repair(ctx,'fetch',values);await repair(ctx,'set_upstream',values);return retryOriginal(ctx);},{inputs:[remoteInput()]}),
        option('Set upstream only','Connect the current branch to an already-fetched matching remote branch.',values=>repair(ctx,'set_upstream',values),{inputs:[remoteInput()]}),
        option('Publish branch instead','Push the current branch and create its upstream on the remote.',values=>repair(ctx,'push_set_upstream',values),{inputs:[remoteInput()]}),
        fetch
      ]};
    case 'push_non_fast_forward':
      return {code,title:'Push rejected: remote branch has newer history',summary:'The remote contains commits not present locally. Integrate remote history before pushing, or use force-with-lease only when you intentionally want to replace remote history.',options:[
        option('Rebase then push','Rebase local commits on top of the upstream branch, then retry push.',async()=>{await repair(ctx,'pull_rebase');return retryOriginal(ctx);},{risk:'A rebase rewrites local commit IDs and may stop for conflicts.'}),
        option('Merge then push','Merge upstream changes into the current branch, then retry push.',async()=>{await repair(ctx,'pull_merge');return retryOriginal(ctx);},{risk:'This may create a merge commit and may stop for conflicts.'}),
        fetch,
        option('Force push with lease','Run git push --force-with-lease. It refuses if the remote changed since your last fetch.',()=>repair(ctx,'push_force_with_lease',{confirmed:true}),{risk:'Advanced: rewrites remote branch history. TaskDeck requires explicit confirmation.'})
      ]};
    case 'pull_diverged':
      return {code,title:'Pull cannot fast-forward',summary:'Local and remote histories have diverged. Choose whether to rebase local commits or create a merge commit.',options:[
        option('Rebase onto upstream','Run git pull --rebase.',()=>repair(ctx,'pull_rebase'),{risk:'May stop for conflicts and rewrites local commit IDs.'}),
        option('Merge upstream','Run git pull --no-rebase with the editor disabled.',()=>repair(ctx,'pull_merge'),{risk:'May create a merge commit and may stop for conflicts.'}),
        fetch,statusOpt
      ]};
    case 'dirty_worktree':
      return {code,title:'Local changes block the Git operation',summary:'Git is protecting uncommitted work from being overwritten.',options:[
        option('Stash changes + retry','Stash tracked and untracked changes, then retry the original operation. The stash is kept until you restore it.',async()=>{await ctx.runAction('stash_push',{message:'TaskDeck Git recovery'});return retryOriginal(ctx);},{risk:'Your changes move into Git stash; restore them after the operation.'}),
        option('Stash only','Save local tracked and untracked changes into Git stash.',()=>ctx.runAction('stash_push',{message:'TaskDeck Git recovery'})),
        statusOpt,retryOpt
      ]};
    case 'conflicts':
    case 'merge_in_progress':
    case 'rebase_in_progress':
    case 'cherry_pick_in_progress': {
      const opts=[
        statusOpt,
        option('Continue operation','Continue the active merge/rebase/cherry-pick/revert after you have resolved and staged conflicts.',()=>repair(ctx,'continue_in_progress'),{risk:'Only use after resolving conflicts and staging the intended files.'}),
        option('Abort operation','Abort the active merge/rebase/cherry-pick/revert and return to the pre-operation state.',()=>repair(ctx,'abort_in_progress'),{risk:'This discards conflict-resolution work made for the current Git operation.'})
      ];
      if(code==='rebase_in_progress')opts.splice(2,0,option('Skip current rebase commit','Run git rebase --skip for the current conflicting commit.',()=>repair(ctx,'skip_rebase'),{risk:'The skipped commit will not be applied.'}));
      return {code,title:'Git operation stopped on conflicts',summary:'Resolve the listed conflicts, stage the resolved files, then continue; or abort the active operation.',options:opts};
    }
    case 'identity_missing':
      return {code,title:'Git author identity is not configured',summary:'Configure repository-local user.name and user.email. TaskDeck will not change your global Git identity.',options:[
        option('Configure identity + retry','Save identity in this repository and retry the failed command.',async values=>{await repair(ctx,'configure_identity',values);return retryOriginal(ctx);},{inputs:[input('name','Git user.name','Your Name','',true),input('email','Git user.email','you@example.com','',true,'email')]}),
        option('Configure identity only','Save user.name and user.email only for this repository.',values=>repair(ctx,'configure_identity',values),{inputs:[input('name','Git user.name','Your Name','',true),input('email','Git user.email','you@example.com','',true,'email')]}),
        statusOpt
      ]};
    case 'gpg_signing': {
      const opts=[retryOpt,statusOpt];
      if(ctx.actionName==='commit'&&ctx.payload?.message)opts.unshift(option('Commit without GPG signing','Retry this commit with commit.gpgSign=false for this invocation only.',()=>repair(ctx,'commit_no_sign',{message:ctx.payload.message}),{risk:'The resulting commit will be unsigned.'}));
      return {code,title:'Git commit signing failed',summary:'The configured signing key or signing agent could not complete the commit.',options:opts};
    }
    case 'ssh_auth':
    case 'ssh_host_key':
    case 'https_auth':
    case 'remote_permission':
      return {code,title:code==='ssh_host_key'?'SSH host verification failed':'Remote authentication or permission failed',summary:'TaskDeck cannot safely invent credentials or trust an unknown host. You can test the remote, correct its URL, then retry.',options:[remoteCheck,setRemote,retryOpt,statusOpt]};
    case 'network_dns':
    case 'network_connect':
    case 'timeout':
      return {code,title:'Network connection to Git remote failed',summary:'The repository was not changed by the failed network operation. Retry after connectivity is restored or test the remote directly.',options:[retryOpt,remoteCheck,fetch]};
    case 'tls':
      return {code,title:'TLS certificate validation failed',summary:'TaskDeck will not disable SSL verification automatically. Correct the certificate/CA or remote URL, then retry.',options:[remoteCheck,setRemote,retryOpt]};
    case 'transport':
      return {code,title:'Git transport/RPC failure',summary:'Transient HTTP/2 or connection interruptions can often be retried using HTTP/1.1 for this invocation only.',options:[
        option('Retry using HTTP/1.1','Retry fetch/pull/push with -c http.version=HTTP/1.1 without changing Git config.',()=>repair(ctx,'retry_http1')),
        retryOpt,remoteCheck,fetch
      ]};
    case 'protected_branch':
      return {code,title:'Remote policy rejected this push',summary:'The destination may be protected or governed by repository rules. A common safe path is to publish the same HEAD to a new branch.',options:[
        option('Push HEAD to a new branch','Create/update a new remote branch from the current HEAD.',values=>repair(ctx,'push_new_branch',values),{inputs:[remoteInput(),branchInput(ctx,'Remote branch name')]}),
        remoteCheck,statusOpt
      ]};
    case 'detached_head':
      return {code,title:'HEAD is detached',summary:'Create a local branch at the current commit before normal pull/push workflows.',options:[
        option('Create local branch here','Run git switch -c <new branch> at the current HEAD.',values=>repair(ctx,'create_branch_from_head',values),{inputs:[branchInput(ctx)]}),
        option('Push detached HEAD to remote branch','Publish the current HEAD directly to a new remote branch.',values=>repair(ctx,'push_new_branch',values),{inputs:[remoteInput(),branchInput(ctx,'Remote branch name')]}),
        statusOpt
      ]};
    case 'branch_exists': {
      const branch=String(ctx.payload?.branch||'').trim();
      const opts=[statusOpt];
      if(branch)opts.unshift(option('Switch to existing branch','Switch to '+branch+' instead of creating it again.',()=>ctx.runAction('switch',{branch})));
      opts.push(option('Create a different branch','Create a branch using another name.',values=>ctx.runAction('create_branch',{branch:values.new_branch}),{inputs:[branchInput(ctx)]}));
      return {code,title:'Branch already exists',summary:'The requested branch name is already present locally.',options:opts};
    }
    case 'remote_ref_missing':
      return {code,title:'Remote branch/ref was not found',summary:'Refresh remote refs first. If this is a new local branch, publish it to create the remote branch.',options:[
        fetch,
        option('Publish current branch','Push the current branch and set its upstream.',values=>repair(ctx,'push_set_upstream',values),{inputs:[remoteInput()]}),
        setRemote,statusOpt
      ]};
    case 'refspec_missing':
      return {code,title:'Git cannot resolve the branch/ref being pushed',summary:'This often happens on an unborn or detached branch, or when the requested ref name is wrong.',options:[
        statusOpt,
        option('Create a branch at current HEAD','Create and switch to a new branch.',values=>repair(ctx,'create_branch_from_head',values),{inputs:[branchInput(ctx)]}),
        remoteCheck
      ]};
    case 'unrelated_histories':
      return {code,title:'Local and remote histories are unrelated',summary:'Git refuses to merge unrelated repository histories by default.',options:[
        option('Merge unrelated histories','Run pull --no-rebase --allow-unrelated-histories.',()=>repair(ctx,'pull_allow_unrelated',{confirmed:true}),{risk:'Advanced: combines two independent histories and may produce many conflicts.'}),
        fetch,statusOpt
      ]};
    case 'index_lock':
      return {code,title:'Git index is locked',summary:'Another Git process may still be active, or a previous process may have left a stale .git/index.lock.',options:[
        retryOpt,statusOpt,
        option('Remove stale index lock','Remove .git/index.lock only if it is a regular file at least 2 minutes old.',()=>repair(ctx,'remove_stale_index_lock',{confirmed:true}),{risk:'TaskDeck refuses to remove a recent lock to reduce the chance of corrupting an active Git operation.'})
      ]};
    case 'dubious_ownership':
      return {code,title:'Git blocked this repository because of ownership',summary:'Git safe.directory protection is active. Trust this repository only if you know and control its contents.',options:[
        option('Trust this repository','Add the exact repository path to global Git safe.directory.',()=>repair(ctx,'trust_repository',{confirmed:true}),{risk:'Security-sensitive: only do this for a repository you trust.'}),
        statusOpt
      ]};
    case 'hook_failed': {
      const opts=[retryOpt,statusOpt];
      if(ctx.actionName==='commit'&&ctx.payload?.message)opts.unshift(option('Commit without hooks','Retry this commit with --no-verify.',()=>repair(ctx,'commit_no_verify',{message:ctx.payload.message,confirmed:true}),{risk:'Advanced: bypasses commit hooks and their policy/validation checks.'}));
      if(ctx.actionName==='push')opts.unshift(option('Push without pre-push hook','Retry push with --no-verify.',()=>repair(ctx,'push_no_verify',{confirmed:true}),{risk:'Advanced: bypasses local pre-push hooks. Remote server rules still apply.'}));
      return {code,title:'A Git hook rejected the operation',summary:'Review hook output first. Bypassing hooks is available only as an explicit advanced choice.',options:opts};
    }
    case 'repository_corrupt':
      return {code,title:'Repository object database may be damaged',summary:'Run Git integrity diagnostics before attempting destructive recovery.',options:[
        option('Run git fsck --full','Check object and reference integrity. This is diagnostic and does not rewrite history.',()=>repair(ctx,'fsck')),
        statusOpt
      ]};
    case 'not_git_repository':
      return {code,title:'Selected path is not a Git repository',summary:'The active repository selection may be stale or the .git metadata is unavailable.',options:[
        option('Rescan repositories','Rescan the workspace and refresh the Git panel.',()=>ctx.rescan()),
        statusOpt
      ]};
    case 'file_too_large':
      return {code,title:'Remote rejected a large file',summary:'This usually requires removing the large object from the commits or using the remote provider\'s large-file workflow. TaskDeck will not rewrite history automatically.',options:[statusOpt]};
    case 'disk_full':
      return {code,title:'Disk space or quota is exhausted',summary:'Free disk space before retrying. Git may be unable to create lock, index, pack, or object files until space is available.',options:[retryOpt]};
    case 'filesystem_permission':
      return {code,title:'Filesystem permission blocked Git',summary:'Check ownership and write permissions for the repository and .git directory before retrying.',options:[statusOpt,retryOpt]};
    default:
      return {code,title:'Git command failed',summary:'TaskDeck did not match this output to a specialized recovery flow. You can inspect status, retry, or test the remote.',options:[statusOpt,retryOpt,remoteCheck]};
  }
}

function detailsText(ctx,issue){
  const repo=ctx.repository||{};
  return [
    'TaskDeck Git recovery',
    'Issue: '+issue.code+' — '+issue.title,
    'Repository: '+(repo.path||repo.id||'(unknown)'),
    'Branch: '+(repo.branch||'(unknown)'),
    'Command: '+(ctx.command||ctx.actionName||'(unknown)'),
    '',
    'Error/output:',
    combinedText(ctx)||'(no output)'
  ].join('\n');
}

async function runOption(ctx,issue,spec,card,values){
  const button=card.querySelector('button[data-run]');
  if(button){button.disabled=true;button.classList.add('running');button.textContent='Running…';}
  result.classList.add('visible');result.textContent='Running '+spec.label+'…';
  try{
    if(spec.risk){
      const ok=window.confirm(spec.risk+'\n\nContinue?');
      if(!ok){result.textContent='Canceled.';return;}
    }
    const response=await spec.run(values);
    const output=response?.output||'Completed successfully.';
    result.textContent=output;
    if(ctx.refresh)await ctx.refresh();
    setTimeout(()=>{if(backdrop.classList.contains('visible'))closeWizard();},900);
  }catch(error){
    const next={...ctx,error:error?.message||String(error),output:error?.gitOutput||'',failureCode:error?.gitFailureCode||''};
    result.textContent='Failed: '+(error?.message||String(error));
    setTimeout(()=>open(next),0);
  }finally{
    if(button&&button.isConnected){button.disabled=false;button.classList.remove('running');button.textContent='Run';}
  }
}

function renderOption(ctx,issue,spec){
  const card=document.createElement('div');card.className='git-recovery-option';
  const optionTitle=document.createElement('div');optionTitle.className='git-recovery-option-title';optionTitle.textContent=spec.label;
  const desc=document.createElement('div');desc.className='git-recovery-option-desc';desc.textContent=spec.description;
  card.append(optionTitle,desc);
  const inputs=document.createElement('div');inputs.className='git-recovery-inputs';
  const fields=new Map();
  for(const item of spec.inputs||[]){
    const field=document.createElement('div');field.className='git-recovery-field';
    const label=document.createElement('label');label.textContent=item.label;
    const control=document.createElement('input');control.type=item.type||'text';control.placeholder=item.placeholder||'';control.value=item.value||'';control.required=Boolean(item.required);
    field.append(label,control);inputs.append(field);fields.set(item.key,{spec:item,control});
  }
  if(fields.size)card.append(inputs);
  if(spec.risk){const risk=document.createElement('div');risk.className='git-recovery-risk';risk.textContent='⚠ '+spec.risk;card.append(risk);}
  const actions=document.createElement('div');actions.className='git-recovery-option-actions';
  const run=document.createElement('button');run.dataset.run='1';run.textContent='Run';
  run.onclick=async()=>{
    const values={};
    for(const [key,entry] of fields){
      const value=entry.control.value.trim();
      if(entry.spec.required&&!value){entry.control.focus();entry.control.reportValidity();return;}
      values[key]=value;
    }
    await runOption(ctx,issue,spec,card,values);
  };
  actions.append(run);card.append(actions);
  return card;
}

function open(ctx){
  current=ctx;
  const issue=issueFor(ctx);
  title.textContent='Git recovery wizard — '+issue.title;
  const repo=ctx.repository||{};
  subtitle.textContent=[repo.name||repo.id||'Git repository',repo.branch||'',ctx.command||ctx.actionName||''].filter(Boolean).join(' · ');
  summary.textContent=issue.summary;
  raw.textContent=combinedText(ctx)||'(Git returned no additional output)';
  optionsHost.replaceChildren();
  for(const spec of issue.options)optionsHost.append(renderOption(ctx,issue,spec));
  result.classList.remove('visible');result.textContent='';
  copy.onclick=()=>app.copyText?app.copyText(detailsText(ctx,issue)):navigator.clipboard.writeText(detailsText(ctx,issue));
  retry.hidden=!ctx.retry;
  retry.onclick=async()=>{
    try{result.classList.add('visible');result.textContent='Retrying original command…';const response=await ctx.retry();result.textContent=response?.output||'Retry succeeded.';if(ctx.refresh)await ctx.refresh();setTimeout(closeWizard,700);}
    catch(error){open({...ctx,error:error?.message||String(error),output:error?.gitOutput||'',failureCode:error?.gitFailureCode||''});}
  };
  backdrop.classList.add('visible');
  close.focus();
}
globalThis.TaskDeckGitRecovery={open,classifyLocal,issueFor};
