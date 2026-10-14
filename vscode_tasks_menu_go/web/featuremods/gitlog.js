// Fast, paged Git Log UI. No Git calls are performed while filtering text.
(function(){
 const state={repo:'',rows:[],search:'',more:false,loading:false};
 const PAGE=100;
 function matches(commit,query){
  const terms=String(query||'').toLowerCase().trim().split(/\s+/).filter(Boolean);
  const searchable=[commit.sha,commit.short,commit.subject,commit.author,commit.date].join(' ').toLowerCase();
  return terms.every(term=>searchable.includes(term));
 }
 async function render(ctx){
  const {app,gitView,content,el,actionButton,action,openGitCommitSemantics,copyText,
   renderGitGraphCommitDetail,showOperation,gitBranchChoiceWizard,getRepoID,onDiffOpen}=ctx;
  const repoID=getRepoID();
  async function fetchPage(reset=false){
   if(state.loading)return false;
   const skip=reset?0:state.rows.length;
   state.loading=true;
   try{
    const params=new URLSearchParams({view:'log',repo:repoID,limit:String(PAGE),skip:String(skip)});
    const data=await app.jsonFetch('/api/git/status?'+params);
    if(getRepoID()!==repoID)return false;
    if(reset||state.repo!==repoID){state.repo=repoID;state.rows=[];}
    const seen=new Set(state.rows.map(row=>row.sha));
    for(const row of data.commits||[]){
     if(row?.sha&&!seen.has(row.sha)){state.rows.push(row);seen.add(row.sha);}
    }
    state.more=Boolean(data.has_more);
    return true;
   }finally{state.loading=false;}
  }
  async function openCommitDiff(commit){
   if(repoID!==getRepoID())return false;
   let data=await gitView('commit-files',{ref:commit.sha});
   if(!data||repoID!==getRepoID())return false;
   const parents=Array.isArray(data.parents)?data.parents:[];
   let parent=data.parent||parents[0]||'';
   if(parents.length>1){
    const selected=await gitBranchChoiceWizard.open({
     title:'Show Diff · choose merge parent',
     description:'Commit: '+commit.short+' · '+commit.subject,
     options:parents.map((sha,i)=>({id:sha,label:'Parent '+(i+1),ref:sha,
      description:'Show changes compared with parent '+(i+1)})),
     defaultChoice:parent,confirmLabel:'Show Diff'
    });
    if(!selected||repoID!==getRepoID())return false;
    if(selected!==parent){
     parent=selected;data=await gitView('commit-files',{ref:commit.sha,parent});
     if(!data||repoID!==getRepoID())return false;
    }
   }
   if(!parent){
    // No parent exists for root commits. Graph detail already supports their
    // individual file additions, including empty-file comparisons.
    const back=el('button','','← Back to Git Log');
    back.onclick=()=>render(ctx).catch(app.showError);
    const details=el('div','git-log-root-diff');
    content.replaceChildren(back,details);
    return renderGitGraphCommitDetail({...commit,parents:[]},details);
   }
   const files=Array.isArray(data.files)?data.files:[];
   if(!files.length){showOperation('Show Diff · '+commit.short,'No changed files compared with selected parent');return false;}
   if(files.length===1){
    const fileCompare=globalThis.TaskMenuFileCompare;
    if(!fileCompare?.openGitCommitFileDiffBetween)throw new Error('File Compare unavailable');
    const opened=await fileCompare.openGitCommitFileDiffBetween(repoID,files[0],parent,commit.sha,{title:'Commit diff · '+commit.short});
    if(opened!==false)onDiffOpen?.();
    return opened;
   }
   const dirCompare=globalThis.TaskMenuDirectoryCompare;
   if(!dirCompare?.openGitCommit)throw new Error('Commit Directory Compare unavailable');
   const opened=await dirCompare.openGitCommit(repoID,parent,commit.sha);
   if(opened!==false)onDiffOpen?.();
   return opened;
  }
  if(repoID!==state.repo||!state.rows.length){
   if(await fetchPage(true)===false)return false;
  }
  const controls=el('div','git-log-controls');
  controls.style.cssText='display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:8px';
  const search=document.createElement('input');
  search.type='search';search.placeholder='Filter SHA, message, author, date…';
  search.setAttribute('aria-label','Filter Git commit log');
  search.style.cssText='flex:1;min-width:180px;padding:7px;border-radius:6px;background:var(--taskmenu-control-bg,#0d1117);color:inherit;border:1px solid #606975';
  search.value=state.search;
  const refresh=actionButton('Refresh log',async()=>{
   state.rows=[];
   if(await fetchPage(true)!==false)await render(ctx);
  },'Reload recent commits');
  controls.append(search,refresh);
  const summary=el('div','git-row-sub');
  const rows=el('div','git-log-results');
  const footer=el('div','git-log-footer');
  footer.style.cssText='display:flex;justify-content:center;padding:10px';
  const older=actionButton('Load older commits',async()=>{
   older.disabled=true;
   try{if(await fetchPage(false)!==false)draw();}
   catch(error){app.showError(error);}
   finally{older.disabled=false;}
  },'Load 100 older commits to expand searchable history');
  footer.append(older);
  content.replaceChildren(controls,summary,rows,footer);
  function draw(){
   rows.replaceChildren();
   const filtered=state.rows.filter(row=>matches(row,state.search));
   summary.textContent=filtered.length+' matching of '+state.rows.length+' loaded commits'+
    (state.more?' · older commits available':' · end of history');
   for(const commit of filtered){
    const row=el('div','git-row'),code=el('span','git-row-code',commit.short);
    const main=el('div','git-row-main');
    main.append(el('div','git-row-title',commit.subject),el('div','git-row-sub',commit.date+' · '+commit.author));
    const actions=el('div','git-row-actions');
    actions.append(
     actionButton('Show Diff',()=>openCommitDiff(commit),'Show files changed in this commit'),
     actionButton('Cherry-pick',()=>action('cherry_pick',{ref:commit.sha,expected_sha:commit.sha},
      'Cherry-pick '+commit.short+' onto the current branch? The working tree must be clean.')),
     actionButton('Revert…',()=>openGitCommitSemantics(commit,'revert')),
     actionButton('Copy SHA',()=>copyText(commit.sha))
    );
    row.append(code,main,actions);rows.append(row);
   }
   if(!filtered.length)rows.append(el('div','git-empty',state.search?'No matching commits in loaded history':'No commits'));
   older.hidden=!state.more;
  }
  search.oninput=()=>{state.search=search.value;draw();};
  draw();return true;
 }
 globalThis.TaskDeckGitLog={render,matches,get state(){return state;}};
})();
