const app=globalThis.TaskMenuApp;

function installPatchPanel(){
  if(!app||app.layoutProfile==='mobile'||!app.taskData?.workspace)return false;
  if(globalThis.TaskMenuPatchPanel)return true;

  const style=document.createElement('style');
  style.textContent=`
  .task-patch-panel{display:none;position:fixed;top:52px;bottom:0;left:48px;z-index:1800;width:min(var(--taskmenu-sidebar-panel-width,310px),calc(100vw - 48px));background:#11151b;border-right:1px solid #3b414d;box-shadow:10px 0 28px rgba(0,0,0,.28);flex-direction:column}
  .task-patch-panel.visible{display:flex}
  .task-patch-panel-head{height:42px;display:flex;align-items:center;gap:6px;padding:6px 8px;border-bottom:1px solid #30343b}
  .task-patch-panel-title{font-size:12px;font-weight:700;letter-spacing:.04em;flex:1}
  .task-patch-panel-close{padding:4px 7px;font-size:11px}
  .task-patch-panel-body{padding:10px;overflow:auto}
  .task-patch-panel-note{font-size:12px;line-height:1.45;opacity:.72;margin:0 0 10px}
  .task-patch-summary{margin:0 0 10px;padding:8px;border:1px solid #30343b;border-radius:6px;background:#0d1015;font-size:11px}
  .task-patch-summary-head{display:flex;align-items:center;gap:6px;margin-bottom:6px}
  .task-patch-summary-title{font-weight:700;flex:1}
  .task-patch-summary-status{opacity:.7}
  .task-patch-summary-counts{display:flex;flex-wrap:wrap;gap:5px;margin:0 0 6px}
  .task-patch-summary-count{padding:2px 5px;border:1px solid #343a44;border-radius:999px}
  .task-patch-summary-list{display:grid;gap:4px}
  .task-patch-summary-item{padding:5px 6px;border-radius:4px;background:#171c23;overflow:hidden}
  .task-patch-summary-name{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-weight:600}
  .task-patch-summary-detail{display:block;opacity:.62;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-summary-warning{margin-top:5px;opacity:.72}
  .task-patch-prompt{margin:0 0 10px;padding:8px;border:1px solid #4b596d;border-radius:6px;background:#121923;font-size:11px}
  .task-patch-prompt[hidden]{display:none}
  .task-patch-prompt-title{font-weight:700;margin-bottom:3px}
  .task-patch-prompt-note{opacity:.7;margin-bottom:7px}
  .task-patch-prompt-items{display:grid;gap:4px;max-height:320px;overflow:auto}
  .task-patch-prompt-item{display:flex;align-items:flex-start;gap:7px;padding:5px 6px;border-radius:4px;background:#171f2a;cursor:pointer}
  .task-patch-prompt-item input{margin-top:2px}
  .task-patch-prompt-copy{min-width:0;flex:1}
  .task-patch-prompt-name{display:block;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-prompt-detail{display:block;opacity:.62;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-prompt-buttons{display:flex;gap:6px;margin-top:8px}
  .task-patch-prompt-buttons button{flex:1}
  .task-patch-run{margin:0 0 10px;padding:8px;border:1px solid #30343b;border-radius:6px;background:#0d1015;font-size:11px}
  .task-patch-run[hidden]{display:none}
  .task-patch-run-title{font-weight:700;margin-bottom:6px}
  .task-patch-run-items{display:grid;gap:4px}
  .task-patch-run-item{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:6px;padding:5px 6px;border-radius:4px;background:#171c23}
  .task-patch-run-name{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-run-status{font-weight:700}
  .task-patch-progress{margin:0 0 6px;padding:6px;border-radius:4px;background:#171c23}
  .task-patch-progress[hidden]{display:none}
  .task-patch-progress-head{font-weight:700}
  .task-patch-progress-detail{display:block;margin-top:2px;opacity:.7;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-artifacts{margin:0 0 10px;padding:8px;border:1px solid #30343b;border-radius:6px;background:#0d1015;font-size:11px}
  .task-patch-artifacts[hidden]{display:none}
  .task-patch-artifacts-title{font-weight:700;margin-bottom:6px}
  .task-patch-artifact-list{display:grid;gap:5px}
  .task-patch-artifact{padding:6px;border-radius:4px;background:#171c23}
  .task-patch-artifact-head{display:flex;align-items:center;gap:5px}
  .task-patch-artifact-label{font-weight:600;min-width:0;flex:1}
  .task-patch-artifact-primary{font-size:9px;padding:1px 4px;border:1px solid #6d7c91;border-radius:999px}
  .task-patch-artifact-path{display:block;margin-top:3px;opacity:.7;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-artifact-actions{display:flex;gap:5px;margin-top:5px}
  .task-patch-artifact-actions a,.task-patch-artifact-actions button{font-size:10px;padding:3px 6px}
  .task-patch-actions{display:grid;gap:7px}
  .task-patch-action{display:flex;flex-direction:column;align-items:flex-start;gap:2px;width:100%;padding:9px 10px;text-align:left}
  .task-patch-action strong{font-size:12px}
  .task-patch-action span{font-size:11px;opacity:.65}
  html[data-taskmenu-theme="light"] .task-patch-panel{background:#fff;border-color:#b9c0c8;box-shadow:10px 0 28px rgba(0,0,0,.12)}
  html[data-taskmenu-theme="light"] .task-patch-panel-head{border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-summary{background:#f6f8fa;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-summary-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-summary-count{border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-prompt{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-prompt-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-run{background:#f6f8fa;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-run-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-progress{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-artifacts{background:#f6f8fa;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-artifact{background:#fff}
  `;
  document.head.append(style);

  const panel=document.createElement('aside');
  panel.className='task-patch-panel';
  panel.setAttribute('aria-label','Patch Tool');

  const head=document.createElement('div');head.className='task-patch-panel-head';
  const title=document.createElement('div');title.className='task-patch-panel-title';title.textContent='PATCH TOOL';
  const closeButton=document.createElement('button');closeButton.type='button';closeButton.className='task-patch-panel-close';closeButton.textContent='×';closeButton.title='Close Patch Tool';
  head.append(title,closeButton);

  const body=document.createElement('div');body.className='task-patch-panel-body';
  const note=document.createElement('p');note.className='task-patch-panel-note';
  note.textContent='Built-in Python Patch Tool. Actions open in the existing TaskDeck terminal so the current interactive workflow is preserved.';
  const summary=document.createElement('div');summary.className='task-patch-summary';
  const summaryHead=document.createElement('div');summaryHead.className='task-patch-summary-head';
  const summaryTitle=document.createElement('div');summaryTitle.className='task-patch-summary-title';summaryTitle.textContent='Queue snapshot';
  const summaryStatus=document.createElement('div');summaryStatus.className='task-patch-summary-status';summaryStatus.textContent='Not loaded';
  const summaryCounts=document.createElement('div');summaryCounts.className='task-patch-summary-counts';
  const summaryList=document.createElement('div');summaryList.className='task-patch-summary-list';
  const summaryWarnings=document.createElement('div');summaryWarnings.className='task-patch-summary-warning';
  summaryHead.append(summaryTitle,summaryStatus);
  summary.append(summaryHead,summaryCounts,summaryList,summaryWarnings);

  const promptBox=document.createElement('div');promptBox.className='task-patch-prompt';promptBox.hidden=true;
  const promptTitle=document.createElement('div');promptTitle.className='task-patch-prompt-title';
  const promptNote=document.createElement('div');promptNote.className='task-patch-prompt-note';
  const promptItems=document.createElement('div');promptItems.className='task-patch-prompt-items';
  const promptButtons=document.createElement('div');promptButtons.className='task-patch-prompt-buttons';
  promptBox.append(promptTitle,promptNote,promptItems,promptButtons);

  const runBox=document.createElement('div');runBox.className='task-patch-run';runBox.hidden=true;
  const runTitle=document.createElement('div');runTitle.className='task-patch-run-title';runTitle.textContent='Run status';
  const progressNode=document.createElement('div');progressNode.className='task-patch-progress';progressNode.hidden=true;
  const progressHead=document.createElement('div');progressHead.className='task-patch-progress-head';
  const progressDetail=document.createElement('span');progressDetail.className='task-patch-progress-detail';
  progressNode.append(progressHead,progressDetail);
  const runItems=document.createElement('div');runItems.className='task-patch-run-items';
  runBox.append(runTitle,progressNode,runItems);

  const artifactBox=document.createElement('div');artifactBox.className='task-patch-artifacts';artifactBox.hidden=true;
  const artifactTitle=document.createElement('div');artifactTitle.className='task-patch-artifacts-title';artifactTitle.textContent='Artifacts';
  const artifactList=document.createElement('div');artifactList.className='task-patch-artifact-list';
  artifactBox.append(artifactTitle,artifactList);

  const actions=document.createElement('div');actions.className='task-patch-actions';
  body.append(note,summary,promptBox,runBox,artifactBox,actions);
  panel.append(head,body);
  document.body.append(panel);

  const actionDefs=[
    ['queue','Queue','Open the normal PATCH/COLLECT queue'],
    ['resume','Resume','Continue an interrupted or failed run'],
    ['history','History','Open Patch Tool reports/history'],
    ['plan','Plan','Inspect the execution plan without replacing the Python engine'],
  ];
  const buttons=[];
  let activeSessionId='';
  let protocolPollGeneration=0;
  for(const [mode,label,detail] of actionDefs){
    const button=document.createElement('button');
    button.type='button';
    button.className='task-patch-action';
    button.dataset.patchMode=mode;
    const strong=document.createElement('strong');strong.textContent=label;
    const span=document.createElement('span');span.textContent=detail;
    button.append(strong,span);
    button.onclick=()=>start(mode,button).catch(app.showError);
    actions.append(button);buttons.push(button);
  }

  function setVisible(value){
    const visible=Boolean(value);
    panel.classList.toggle('visible',visible);
    if(!visible)protocolPollGeneration+=1;
    if(visible&&activeSessionId)void pollProtocol(activeSessionId,true,true);
    window.dispatchEvent(new CustomEvent('taskmenu:patch-panel-visible',{detail:{visible}}));
  }
  function open(){setVisible(true);}
  function close(){setVisible(false);}
  function toggle(){setVisible(!panel.classList.contains('visible'));}

  function resetSummary(status='Not loaded'){
    summaryStatus.textContent=status;
    summaryCounts.replaceChildren();
    summaryList.replaceChildren();
    summaryWarnings.replaceChildren();
  }

  function renderQueueSnapshot(snapshot){
    const items=Array.isArray(snapshot?.items)?snapshot.items:[];
    const counts=snapshot?.counts&&typeof snapshot.counts==='object'?snapshot.counts:{};
    summaryStatus.textContent=snapshot?.status==='empty'?'Empty':`${Number(snapshot?.total||items.length)} item(s)`;
    summaryCounts.replaceChildren();
    for(const [kind,count] of Object.entries(counts).sort(([a],[b])=>a.localeCompare(b))){
      const chip=document.createElement('span');
      chip.className='task-patch-summary-count';
      chip.textContent=`${kind}: ${count}`;
      summaryCounts.append(chip);
    }
    summaryList.replaceChildren();
    const visible=items.slice(0,50);
    for(const item of visible){
      const row=document.createElement('div');row.className='task-patch-summary-item';
      const name=document.createElement('span');name.className='task-patch-summary-name';name.textContent=String(item?.name||'');
      const detail=document.createElement('span');detail.className='task-patch-summary-detail';
      detail.textContent=[item?.kind,item?.detail].filter(Boolean).join(' · ');
      row.append(name,detail);
      summaryList.append(row);
    }
    if(items.length>visible.length){
      const more=document.createElement('div');more.className='task-patch-summary-warning';more.textContent=`+${items.length-visible.length} more item(s)`;
      summaryList.append(more);
    }
    const warnings=Array.isArray(snapshot?.warnings)?snapshot.warnings:[];
    summaryWarnings.textContent=warnings.length?`${warnings.length} warning(s): ${warnings.slice(0,3).join(' | ')}`:'';
  }

  function renderProgress(progress){
    if(!progress||typeof progress!=='object'){
      progressNode.hidden=true;
      progressHead.textContent='';
      progressDetail.textContent='';
      return;
    }
    const status=String(progress.status||'');
    const phase=String(progress.phase||'');
    const elapsed=Number(progress.elapsed_seconds||0);
    const output=Number(progress.output_lines||0);
    const item=String(progress.item_name||'');
    progressHead.textContent=[item,status,phase,Number.isFinite(elapsed)?elapsed.toFixed(1)+'s':'',Number.isFinite(output)?output+' lines':''].filter(Boolean).join(' · ');
    progressDetail.textContent=String(progress.detail||'');
    progressDetail.title=progressDetail.textContent;
    progressNode.hidden=false;
    runBox.hidden=false;
  }

  function renderItemLifecycle(items){
    const rows=Array.isArray(items)?items:[];
    runItems.replaceChildren();
    if(!rows.length){
      runBox.hidden=true;
      return;
    }
    runBox.hidden=false;
    for(const item of rows){
      const row=document.createElement('div');row.className='task-patch-run-item';
      const name=document.createElement('span');name.className='task-patch-run-name';
      name.textContent=`${Number(item?.index||0)}. ${String(item?.name||'')} · ${String(item?.kind||'')}`;
      const status=document.createElement('span');status.className='task-patch-run-status';
      const rc=item?.rc;
      status.textContent=rc===undefined||rc===null?String(item?.status||''): `${String(item?.status||'')} (rc=${rc})`;
      row.append(name,status);
      runItems.append(row);
    }
  }

  const artifactLabels={
    fail_handoff_zip:'FAIL handoff ZIP',
    fail_handoff_text:'FAIL handoff TXT',
    ai_sync_zip:'AI sync ZIP',
    ai_sync_text:'AI sync TXT',
    collect_result_zip:'COLLECT result ZIP',
    collect_result_text:'COLLECT result TXT',
  };

  function renderArtifacts(artifacts){
    const rows=Array.isArray(artifacts)?artifacts:[];
    artifactList.replaceChildren();
    let rendered=0;
    for(const artifact of rows){
      const path=String(artifact?.path||'');
      if(!path.startsWith('artifacts/')||path.includes('..')||path.includes('\\'))continue;
      const kind=String(artifact?.artifact_kind||'');
      const row=document.createElement('div');row.className='task-patch-artifact';
      const head=document.createElement('div');head.className='task-patch-artifact-head';
      const label=document.createElement('span');label.className='task-patch-artifact-label';label.textContent=artifactLabels[kind]||kind||'Artifact';
      head.append(label);
      if(artifact?.primary){
        const badge=document.createElement('span');badge.className='task-patch-artifact-primary';badge.textContent='PRIMARY';head.append(badge);
      }
      const pathNode=document.createElement('span');pathNode.className='task-patch-artifact-path';pathNode.textContent=path;pathNode.title=path;
      const actionsNode=document.createElement('div');actionsNode.className='task-patch-artifact-actions';
      const download=document.createElement('a');download.textContent='Download';download.href='/api/files/download?path='+encodeURIComponent(path);download.download='';
      actionsNode.append(download);
      if(kind.endsWith('_text')){
        const open=document.createElement('button');open.type='button';open.textContent='Open';
        open.onclick=()=>window.dispatchEvent(new CustomEvent('taskmenu:project-file-open-request',{detail:{path}}));
        actionsNode.append(open);
      }
      row.append(head,pathNode,actionsNode);
      artifactList.append(row);
      rendered+=1;
    }
    artifactBox.hidden=rendered===0;
  }

  function clearPrompt(){
    promptBox.hidden=true;
    promptTitle.textContent='';
    promptNote.textContent='';
    promptItems.replaceChildren();
    promptButtons.replaceChildren();
  }

  function selectedPromptIndexes(){
    return [...promptItems.querySelectorAll('input[type="checkbox"]:checked')]
      .map(input=>Number(input.dataset.patchIndex))
      .filter(index=>Number.isInteger(index)&&index>0);
  }

  function applyPromptConstraints(changed,prompt){
    if(!changed?.checked)return;
    const constraints=prompt?.constraints&&typeof prompt.constraints==='object'?prompt.constraints:{};
    if(constraints.collect_exclusive!==true)return;
    const changedKind=String(changed.dataset.patchKind||'').toUpperCase();
    for(const input of promptItems.querySelectorAll('input[type="checkbox"]')){
      if(input===changed||!input.checked)continue;
      const kind=String(input.dataset.patchKind||'').toUpperCase();
      if(changedKind==='COLLECT'||kind==='COLLECT')input.checked=false;
    }
  }

  async function submitPromptResponse(sessionId,prompt,action){
    const payload={prompt_id:String(prompt?.prompt_id||''),action};
    if(action==='select')payload.indexes=selectedPromptIndexes();
    for(const button of promptButtons.querySelectorAll('button'))button.disabled=true;
    try{
      await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/prompt-response`,{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify(payload),
      });
      clearPrompt();
      summaryStatus.textContent=action==='cancel'?'Cancelled':'Selection submitted';
      void pollProtocol(sessionId,false,true);
    }catch(error){
      for(const button of promptButtons.querySelectorAll('button'))button.disabled=false;
      throw error;
    }
  }

  function renderQueuePrompt(sessionId,prompt){
    if(prompt?.type!=='prompt'||prompt?.prompt_kind!=='queue_selection'||!prompt?.prompt_id)return false;
    const items=Array.isArray(prompt.items)?prompt.items:[];
    if(!items.length)return false;
    const initial=new Set(Array.isArray(prompt.initial_selected)?prompt.initial_selected.map(Number):[]);
    const actionsAllowed=new Set(Array.isArray(prompt.actions)?prompt.actions.map(String):[]);
    clearPrompt();
    promptBox.hidden=false;
    promptTitle.textContent=String(prompt.title||'Choose PATCH/COLLECT work');
    promptNote.textContent='Select work here, or use the terminal tab. Python validates the final selection.';
    for(const item of items){
      const index=Number(item?.index);
      if(!Number.isInteger(index)||index<1)continue;
      const label=document.createElement('label');label.className='task-patch-prompt-item';
      const input=document.createElement('input');input.type='checkbox';input.dataset.patchIndex=String(index);input.dataset.patchKind=String(item?.kind||'');
      input.checked=initial.has(index);
      input.onchange=()=>applyPromptConstraints(input,prompt);
      const copy=document.createElement('span');copy.className='task-patch-prompt-copy';
      const name=document.createElement('span');name.className='task-patch-prompt-name';
      name.textContent=`${index}. ${String(item?.name||'')}`;
      const detail=document.createElement('span');detail.className='task-patch-prompt-detail';
      detail.textContent=[item?.group,item?.kind,item?.detail].filter(Boolean).join(' · ');
      copy.append(name,detail);
      label.append(input,copy);
      promptItems.append(label);
    }
    if(actionsAllowed.has('select')){
      const select=document.createElement('button');select.type='button';select.textContent='Run selected';
      select.onclick=()=>{
        if(!selectedPromptIndexes().length){
          promptNote.textContent='Select at least one item, or Cancel.';
          return;
        }
        submitPromptResponse(sessionId,prompt,'select').catch(app.showError);
      };
      promptButtons.append(select);
    }
    if(actionsAllowed.has('cancel')){
      const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
      cancel.onclick=()=>submitPromptResponse(sessionId,prompt,'cancel').catch(app.showError);
      promptButtons.append(cancel);
    }
    return true;
  }

  async function pollProtocol(sessionId,expectPrompt=false,followLifecycle=false){
    const generation=++protocolPollGeneration;
    resetSummary('Loading…');
    clearPrompt();
    let haveSnapshot=false;
    const maxAttempts=followLifecycle?7200:40;
    const delayMs=followLifecycle?1000:250;
    for(let attempt=0;attempt<maxAttempts;attempt+=1){
      if(generation!==protocolPollGeneration||!panel.classList.contains('visible'))return;
      let state;
      try{
        state=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/protocol`);
      }catch(error){
        resetSummary('PTY-only');
        console.warn('Patch protocol state unavailable:',error);
        return;
      }
      if(state?.available===false){
        resetSummary('PTY-only');
        return;
      }
      if(state?.queue_snapshot&&!haveSnapshot){
        renderQueueSnapshot(state.queue_snapshot);
        haveSnapshot=true;
      }
      renderItemLifecycle(state?.items);
      renderProgress(state?.progress);
      renderArtifacts(state?.artifacts);
      if(expectPrompt&&state?.commands_enabled===false&&haveSnapshot){
        summaryStatus.textContent+=' · Continue in PTY';
        return;
      }
      if(expectPrompt&&state?.prompt&&renderQueuePrompt(sessionId,state.prompt)){
        summaryStatus.textContent+=' · Awaiting selection';
        return;
      }
      if(state?.last_event?.type==='run_finished'){
        if(haveSnapshot)summaryStatus.textContent+=' · Finished';
        return;
      }
      if(!expectPrompt&&haveSnapshot&&!followLifecycle)return;
      if(state?.error){
        if(!haveSnapshot)resetSummary('Protocol error');
        summaryWarnings.textContent=String(state.error);
        return;
      }
      await new Promise(resolve=>setTimeout(resolve,delayMs));
    }
    if(haveSnapshot){
      summaryStatus.textContent+=' · Continue in PTY';
    }else{
      resetSummary('Snapshot timeout · Continue in PTY');
    }
  }

  async function start(mode,sourceButton=null){
    if(sourceButton)sourceButton.disabled=true;
    try{
      const meta=await app.jsonFetch('/api/sessions',{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({kind:'patch',patch_mode:mode}),
      });
      activeSessionId=meta.id;
      renderProgress(null);
      renderArtifacts([]);
      app.attachSession(meta,true);
      window.dispatchEvent(new CustomEvent('taskmenu:patch-session-started',{detail:{mode,meta}}));
      if(mode==='queue'||mode==='resume'||mode==='plan'){
        void pollProtocol(meta.id,mode==='queue',mode!=='queue');
      }else{
        resetSummary('History uses PTY');
      }
      return meta;
    }finally{
      if(sourceButton?.isConnected)sourceButton.disabled=false;
    }
  }

  closeButton.onclick=close;
  globalThis.TaskMenuPatchPanel={open,close,toggle,start,renderQueueSnapshot,renderQueuePrompt,renderItemLifecycle,renderProgress,renderArtifacts,get panel(){return panel;},get visible(){return panel.classList.contains('visible');}};
  return true;
}

function safeInstallPatchPanel(){
  try{installPatchPanel();}
  catch(error){console.warn('Patch panel enhancement disabled:',error);}
}

if(app?.taskData?.workspace)safeInstallPatchPanel();
else window.addEventListener('taskmenu:tasks',safeInstallPatchPanel,{once:true});
