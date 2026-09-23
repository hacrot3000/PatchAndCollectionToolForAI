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
  .task-patch-summary-tabs{display:grid;grid-template-columns:1fr 1fr;gap:5px;margin:0 0 7px}
  .task-patch-summary-tab{padding:5px 7px;font-size:11px;text-align:center}
  .task-patch-summary-tab.active{background:#283342;border-color:#526278;color:#fff}
  .task-patch-summary-counts{display:flex;flex-wrap:wrap;gap:5px;margin:0 0 6px}
  .task-patch-summary-count{padding:2px 5px;border:1px solid #343a44;border-radius:999px}
  .task-patch-summary-list{display:grid;gap:4px}
  .task-patch-summary-item{padding:5px 6px;border-radius:4px;background:#171c23;overflow:hidden}
  .task-patch-summary-name{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-weight:600}
  .task-patch-summary-detail{display:block;opacity:.62;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-summary-failure{display:block;margin-top:3px;opacity:.82;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-summary-item-actions{display:flex;gap:4px;margin-top:5px;flex-wrap:wrap}
  .task-patch-summary-item-actions button{font-size:10px;padding:3px 6px}
  .task-patch-summary-empty{padding:7px 6px;opacity:.62;text-align:center}
  .task-patch-summary-warning{margin-top:5px;opacity:.72}
  .task-patch-action-result{margin:0 0 10px;padding:8px;border:1px solid #3f4b5d;border-radius:6px;background:#0d1015;font-size:11px}
  .task-patch-action-result[hidden]{display:none}
  .task-patch-action-result-head{display:flex;align-items:center;gap:6px;margin-bottom:5px}
  .task-patch-action-result-title{font-weight:700;min-width:0;flex:1;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-action-result-close{font-size:10px;padding:2px 6px}
  .task-patch-action-result-meta{opacity:.72;margin-bottom:5px}
  .task-patch-action-result-output{margin:0;max-height:300px;overflow:auto;white-space:pre-wrap;word-break:break-word;background:#151b23;border:1px solid #303946;border-radius:4px;padding:6px;font:10px/1.45 ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace}
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
  .task-patch-resume{margin:0 0 10px;padding:8px;border:1px solid #4b596d;border-radius:6px;background:#121923;font-size:11px}
  .task-patch-resume[hidden]{display:none}
  .task-patch-resume-head{display:flex;align-items:center;gap:6px;margin-bottom:6px}
  .task-patch-resume-title{font-weight:700;flex:1}
  .task-patch-resume-status{opacity:.7}
  .task-patch-resume-counts{display:flex;flex-wrap:wrap;gap:5px;margin-bottom:7px}
  .task-patch-resume-count{padding:2px 5px;border:1px solid #3b4655;border-radius:999px}
  .task-patch-resume-items{display:grid;gap:4px;max-height:220px;overflow:auto;margin-bottom:7px}
  .task-patch-resume-item{display:flex;align-items:flex-start;gap:7px;padding:5px 6px;border-radius:4px;background:#171f2a}
  .task-patch-resume-item input{margin-top:2px}
  .task-patch-resume-item-copy{min-width:0;flex:1}
  .task-patch-resume-item-name{display:block;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-resume-item-detail{display:block;opacity:.66;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-resume-options{display:grid;gap:5px}
  .task-patch-resume-option{display:flex;flex-direction:column;align-items:flex-start;gap:2px;padding:7px 8px;text-align:left}
  .task-patch-resume-option strong{font-size:11px}
  .task-patch-resume-option span{font-size:10px;opacity:.7}
  .task-patch-resume-option[disabled]{opacity:.45}
  .task-patch-resume-note{margin-top:6px;opacity:.72}
  .task-patch-running-head{display:none;margin:0 0 10px;padding:8px;border:1px solid #4b596d;border-radius:6px;background:#121923;font-size:11px}
  .task-patch-panel.running .task-patch-running-head{display:block}
  .task-patch-running-title{font-weight:700;font-size:12px}
  .task-patch-running-meta{margin-top:3px;opacity:.72}
  .task-patch-running-actions{display:flex;gap:6px;margin-top:8px}
  .task-patch-running-actions button{flex:1}
  .task-patch-panel.running .task-patch-panel-note,
  .task-patch-panel.running .task-patch-summary,
  .task-patch-panel.running .task-patch-action-result,
  .task-patch-panel.running .task-patch-prompt,
  .task-patch-panel.running .task-patch-resume,
  .task-patch-panel.running .task-patch-actions{display:none!important}
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
  html[data-taskmenu-theme="light"] .task-patch-summary-tab.active{background:#e7eef7;border-color:#9aa9bc;color:#1f2328}
  html[data-taskmenu-theme="light"] .task-patch-action-result{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-action-result-output{background:#fff;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-running-head{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-prompt{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-prompt-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-resume{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-resume-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-resume-count{border-color:#d0d7de}
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
  note.textContent='Built-in Python Patch Tool. Queue inspection actions are native; execution still uses the authoritative Python engine with terminal fallback preserved.';
  const summary=document.createElement('div');summary.className='task-patch-summary';
  const summaryHead=document.createElement('div');summaryHead.className='task-patch-summary-head';
  const summaryTitle=document.createElement('div');summaryTitle.className='task-patch-summary-title';summaryTitle.textContent='Queue snapshot';
  const summaryStatus=document.createElement('div');summaryStatus.className='task-patch-summary-status';summaryStatus.textContent='Not loaded';
  const summaryTabs=document.createElement('div');summaryTabs.className='task-patch-summary-tabs';
  const queueTab=document.createElement('button');queueTab.type='button';queueTab.className='task-patch-summary-tab active';queueTab.textContent='Queue';
  const failedTab=document.createElement('button');failedTab.type='button';failedTab.className='task-patch-summary-tab';failedTab.textContent='Failed';
  summaryTabs.append(queueTab,failedTab);
  const summaryCounts=document.createElement('div');summaryCounts.className='task-patch-summary-counts';
  const summaryList=document.createElement('div');summaryList.className='task-patch-summary-list';
  const summaryWarnings=document.createElement('div');summaryWarnings.className='task-patch-summary-warning';
  summaryHead.append(summaryTitle,summaryStatus);
  summary.append(summaryHead,summaryTabs,summaryCounts,summaryList,summaryWarnings);

  const actionResultBox=document.createElement('div');actionResultBox.className='task-patch-action-result';actionResultBox.hidden=true;
  const actionResultHead=document.createElement('div');actionResultHead.className='task-patch-action-result-head';
  const actionResultTitle=document.createElement('div');actionResultTitle.className='task-patch-action-result-title';
  const actionResultClose=document.createElement('button');actionResultClose.type='button';actionResultClose.className='task-patch-action-result-close';actionResultClose.textContent='×';actionResultClose.title='Close action result';
  actionResultHead.append(actionResultTitle,actionResultClose);
  const actionResultMeta=document.createElement('div');actionResultMeta.className='task-patch-action-result-meta';
  const actionResultOutput=document.createElement('pre');actionResultOutput.className='task-patch-action-result-output';
  actionResultBox.append(actionResultHead,actionResultMeta,actionResultOutput);
  const promptBox=document.createElement('div');promptBox.className='task-patch-prompt';promptBox.hidden=true;
  const promptTitle=document.createElement('div');promptTitle.className='task-patch-prompt-title';
  const promptNote=document.createElement('div');promptNote.className='task-patch-prompt-note';
  const promptItems=document.createElement('div');promptItems.className='task-patch-prompt-items';
  const promptButtons=document.createElement('div');promptButtons.className='task-patch-prompt-buttons';
  promptBox.append(promptTitle,promptNote,promptItems,promptButtons);

  const resumeBox=document.createElement('div');resumeBox.className='task-patch-resume';resumeBox.hidden=true;
  const resumeHead=document.createElement('div');resumeHead.className='task-patch-resume-head';
  const resumeTitle=document.createElement('div');resumeTitle.className='task-patch-resume-title';resumeTitle.textContent='Smart Resume';
  const resumeStatus=document.createElement('div');resumeStatus.className='task-patch-resume-status';
  resumeHead.append(resumeTitle,resumeStatus);
  const resumeCounts=document.createElement('div');resumeCounts.className='task-patch-resume-counts';
  const resumeItems=document.createElement('div');resumeItems.className='task-patch-resume-items';
  const resumeOptions=document.createElement('div');resumeOptions.className='task-patch-resume-options';
  const resumeNote=document.createElement('div');resumeNote.className='task-patch-resume-note';
  resumeBox.append(resumeHead,resumeCounts,resumeItems,resumeOptions,resumeNote);

  const runningHead=document.createElement('div');runningHead.className='task-patch-running-head';
  const runningTitle=document.createElement('div');runningTitle.className='task-patch-running-title';runningTitle.textContent='Running';
  const runningMeta=document.createElement('div');runningMeta.className='task-patch-running-meta';runningMeta.textContent='Waiting for Python execution state…';
  const runningActions=document.createElement('div');runningActions.className='task-patch-running-actions';
  const terminalEvidence=document.createElement('button');terminalEvidence.type='button';terminalEvidence.textContent='Open terminal evidence';
  const runningBack=document.createElement('button');runningBack.type='button';runningBack.textContent='Back to Queue';runningBack.hidden=true;
  runningActions.append(terminalEvidence,runningBack);
  runningHead.append(runningTitle,runningMeta,runningActions);

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
  body.append(note,summary,actionResultBox,promptBox,resumeBox,runningHead,runBox,artifactBox,actions);
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
  let latestQueueSnapshot=null;
  let queueSummaryView='queue';
  let activeQueuePrompt=null;
  let latestResumeSnapshot=null;
  let activeResumePrompt=null;
  let resumeBusy=false;
  let actionBusy=false;
  let actionPollGeneration=0;
  let runningMode=false;
  let runningFinished=false;
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
    if(!visible){protocolPollGeneration+=1;actionPollGeneration+=1;}
    if(visible&&activeSessionId)void pollProtocol(activeSessionId,!runningMode,true);
    window.dispatchEvent(new CustomEvent('taskmenu:patch-panel-visible',{detail:{visible}}));
  }
  function open(){setVisible(true);}
  function close(){setVisible(false);}
  function toggle(){setVisible(!panel.classList.contains('visible'));}

  function resetSummary(status='Not loaded'){
    latestQueueSnapshot=null;
    summaryStatus.textContent=status;
    queueTab.textContent='Queue';
    failedTab.textContent='Failed';
    summaryCounts.replaceChildren();
    summaryList.replaceChildren();
    summaryWarnings.replaceChildren();
  }

  function clearResumeView(){
    latestResumeSnapshot=null;
    activeResumePrompt=null;
    resumeBusy=false;
    resumeBox.hidden=true;
    resumeStatus.textContent='';
    resumeCounts.replaceChildren();
    resumeItems.replaceChildren();
    resumeOptions.replaceChildren();
    resumeNote.textContent='';
  }

  function resumeFailedRows(source){
    return Array.isArray(source?.failed_items)?source.failed_items:[];
  }

  function selectedResumeIndexes(){
    return [...resumeItems.querySelectorAll('input[type="checkbox"]:checked')]
      .map(input=>Number(input.dataset.resumeFailedIndex))
      .filter(index=>Number.isInteger(index)&&index>0);
  }

  function resumeCapabilityForAction(action){
    return {failed:'can_retry',collect_failed:'can_collect',delete_failed:'can_delete'}[action]||'';
  }

  function renderResumeSnapshot(snapshot){
    latestResumeSnapshot=snapshot&&typeof snapshot==='object'?snapshot:null;
    if(!latestResumeSnapshot){
      if(!activeResumePrompt)resumeBox.hidden=true;
      return;
    }
    resumeBox.hidden=false;
    const summary=latestResumeSnapshot.summary&&typeof latestResumeSnapshot.summary==='object'?latestResumeSnapshot.summary:{};
    resumeStatus.textContent=latestResumeSnapshot.status==='empty'?'No recovery work':'Recovery available';
    resumeCounts.replaceChildren();
    for(const [key,label] of [['replay','Replay'],['failed','Failed'],['remaining','Remaining']]){
      const count=Number(summary[key]||0);
      const chip=document.createElement('span');chip.className='task-patch-resume-count';chip.textContent=`${label}: ${Number.isFinite(count)?count:0}`;
      resumeCounts.append(chip);
    }
    if(!activeResumePrompt){
      resumeItems.replaceChildren();
      const rows=Array.isArray(latestResumeSnapshot.items)?latestResumeSnapshot.items:[];
      for(const item of rows.slice(0,30)){
        const row=document.createElement('div');row.className='task-patch-resume-item';
        const copy=document.createElement('span');copy.className='task-patch-resume-item-copy';
        const name=document.createElement('span');name.className='task-patch-resume-item-name';name.textContent=String(item?.name||'');
        const detail=document.createElement('span');detail.className='task-patch-resume-item-detail';detail.textContent=[item?.group,item?.kind,item?.detail].filter(Boolean).join(' · ');
        copy.append(name,detail);row.append(copy);resumeItems.append(row);
      }
      resumeNote.textContent='Waiting for Python Smart Resume actions…';
    }
  }

  function resumeActionAllowed(prompt,action){
    return new Set(Array.isArray(prompt?.actions)?prompt.actions.map(String):[]).has(action);
  }

  function resumeSelectionActions(prompt){
    const constraints=prompt?.constraints&&typeof prompt.constraints==='object'?prompt.constraints:{};
    return new Set(Array.isArray(constraints.selection_actions)?constraints.selection_actions.map(String):[]);
  }

  function renderResumeFailedItems(prompt){
    resumeItems.replaceChildren();
    const rows=resumeFailedRows(prompt);
    for(const item of rows){
      const index=Number(item?.index);
      if(!Number.isInteger(index)||index<1)continue;
      const label=document.createElement('label');label.className='task-patch-resume-item';
      const input=document.createElement('input');input.type='checkbox';input.dataset.resumeFailedIndex=String(index);input.disabled=resumeBusy;
      const copy=document.createElement('span');copy.className='task-patch-resume-item-copy';
      const name=document.createElement('span');name.className='task-patch-resume-item-name';name.textContent=`${index}. ${String(item?.name||'')}`;
      const failure=item?.failure&&typeof item.failure==='object'?item.failure:{};
      const capabilities=[
        item?.can_retry?'retry':'',
        item?.can_collect?'collect':'',
        item?.can_delete?'delete':'',
      ].filter(Boolean).join('/');
      const detail=document.createElement('span');detail.className='task-patch-resume-item-detail';
      detail.textContent=[
        String(item?.queue_name||''),
        String(failure?.status||''),
        String(failure?.diagnosis_kind||''),
        capabilities?('actions: '+capabilities):'',
      ].filter(Boolean).join(' · ');
      copy.append(name,detail);label.append(input,copy);resumeItems.append(label);
    }
  }

  async function submitResumeAction(sessionId,prompt,action){
    if(resumeBusy)return null;
    if(!resumeActionAllowed(prompt,action))throw new Error(`Resume action ${action} is not advertised by Python`);
    const selectionActions=resumeSelectionActions(prompt);
    const payload={prompt_id:String(prompt?.prompt_id||''),action};
    if(selectionActions.has(action)){
      const indexes=selectedResumeIndexes();
      if(!indexes.length){
        resumeNote.textContent='Select at least one failed item for this action.';
        return null;
      }
      const capability=resumeCapabilityForAction(action);
      const byIndex=new Map(resumeFailedRows(prompt).map(item=>[Number(item?.index),item]));
      const unavailable=indexes.find(index=>!byIndex.get(index)?.[capability]);
      if(unavailable){
        resumeNote.textContent=`Selected item ${unavailable} is not available for this action.`;
        return null;
      }
      payload.failed_indexes=indexes;
      if(action==='delete_failed'&&!window.confirm(`Remove ${indexes.length} selected failed PATCH item(s) from the queue?`))return null;
    }
    resumeBusy=true;
    for(const button of resumeOptions.querySelectorAll('button'))button.disabled=true;
    for(const input of resumeItems.querySelectorAll('input'))input.disabled=true;
    resumeNote.textContent='Submitting Smart Resume action…';
    try{
      await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/resume-action`,{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify(payload),
      });
      activeResumePrompt=null;
      resumeBox.hidden=true;
      resumeBusy=false;
      if(action==='history'){
        openTerminalEvidence();
        return true;
      }
      if(['all','failed','remaining','collect_failed'].includes(action)){
        enterRunningView();
        void pollProtocol(sessionId,false,true);
      }else{
        void pollProtocol(sessionId,true,true);
      }
      return true;
    }catch(error){
      resumeBusy=false;
      renderResumePrompt(sessionId,prompt);
      resumeNote.textContent=String(error?.message||error);
      throw error;
    }
  }

  function renderResumePrompt(sessionId,prompt){
    if(prompt?.type!=='prompt'||prompt?.prompt_kind!=='resume_action'||!prompt?.prompt_id)return false;
    activeResumePrompt=prompt;
    activeQueuePrompt=null;
    promptBox.hidden=true;
    resumeBox.hidden=false;
    resumeBusy=false;
    resumeTitle.textContent=String(prompt.title||'Smart Resume');
    resumeStatus.textContent='Choose recovery action';
    renderResumeFailedItems(prompt);
    resumeOptions.replaceChildren();
    const options=Array.isArray(prompt.options)?prompt.options:[];
    for(const option of options){
      const action=String(option?.key||'');
      if(!action||option?.available!==true||!resumeActionAllowed(prompt,action))continue;
      const button=document.createElement('button');button.type='button';button.className='task-patch-resume-option';button.dataset.resumeAction=action;
      const strong=document.createElement('strong');strong.textContent=String(option?.label||action);
      const description=document.createElement('span');
      const count=Number(option?.count||0);
      description.textContent=[String(option?.description||''),count>0?(`count=${count}`):''].filter(Boolean).join(' · ');
      button.append(strong,description);
      button.onclick=()=>submitResumeAction(sessionId,prompt,action).catch(app.showError);
      resumeOptions.append(button);
    }
    resumeNote.textContent='Recovery policy and availability come from the Python Patch Tool.';
    return true;
  }

  function enterRunningView(){
    runningMode=true;
    runningFinished=false;
    panel.classList.add('running');
    runningTitle.textContent='Running';
    runningMeta.textContent='Python is preparing or executing the selected work…';
    runningBack.hidden=true;
    runBox.hidden=false;
  }

  function finishRunningView(){
    if(!runningMode)return;
    runningFinished=true;
    runningTitle.textContent='Finished';
    runningMeta.textContent='Native run state is complete. Terminal evidence remains available.';
    runningBack.hidden=false;
  }

  function leaveRunningView(){
    runningMode=false;
    runningFinished=false;
    panel.classList.remove('running');
    runningTitle.textContent='Running';
    runningMeta.textContent='Waiting for Python execution state…';
    runningBack.hidden=true;
    renderProgress(null);
    renderItemLifecycle([]);
    renderArtifacts([]);
    if(latestQueueSnapshot)renderQueueSnapshot(latestQueueSnapshot);
  }

  function openTerminalEvidence(){
    if(!activeSessionId||!app.views.has(activeSessionId))return;
    app.activateView(activeSessionId);
    close();
  }

  function clearActionResult(){
    actionResultBox.hidden=true;
    actionResultTitle.textContent='';
    actionResultMeta.textContent='';
    actionResultOutput.textContent='';
  }

  function renderActionResult(result){
    if(!result||typeof result!=='object'){clearActionResult();return;}
    const action=String(result.action||'Action');
    const item=String(result.item_name||'');
    const status=String(result.status||'');
    const rc=result.rc;
    const elapsed=Number(result.elapsed_seconds||0);
    actionResultTitle.textContent=`${action.charAt(0).toUpperCase()+action.slice(1)} · ${item}`;
    actionResultMeta.textContent=[status,rc===undefined||rc===null?'':`rc=${rc}`,Number.isFinite(elapsed)?elapsed.toFixed(2)+'s':'',result.output_truncated?'output truncated':''].filter(Boolean).join(' · ');
    actionResultOutput.textContent=String(result.output||'');
    actionResultBox.hidden=false;
  }

  function renderActionPending(action,item){
    actionResultTitle.textContent=`${action.charAt(0).toUpperCase()+action.slice(1)} · ${String(item?.name||'')}`;
    actionResultMeta.textContent='Running…';
    actionResultOutput.textContent='';
    actionResultBox.hidden=false;
  }

  function promptItemForQueueItem(item){
    if(!activeQueuePrompt)return null;
    const rows=Array.isArray(activeQueuePrompt.items)?activeQueuePrompt.items:[];
    const name=String(item?.name||'');
    const kind=String(item?.kind||'');
    const group=String(item?.group||'');
    return rows.find(row=>String(row?.name||'')===name&&String(row?.kind||'')===kind&&(!row?.group||!group||String(row.group)===group))||null;
  }

  function actionAllowed(action){
    return new Set(Array.isArray(activeQueuePrompt?.item_actions)?activeQueuePrompt.item_actions.map(String):[]).has(action);
  }

  function refreshActionDisabledState(){
    for(const button of summaryList.querySelectorAll('.task-patch-item-action'))button.disabled=actionBusy;
    for(const button of promptButtons.querySelectorAll('button'))button.disabled=actionBusy;
  }

  async function waitForActionResult(sessionId,actionID){
    const generation=++actionPollGeneration;
    for(let attempt=0;attempt<3700;attempt+=1){
      if(generation!==actionPollGeneration||!panel.classList.contains('visible')||sessionId!==activeSessionId)return null;
      const state=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/protocol`);
      if(state?.action_result?.action_id===actionID){renderActionResult(state.action_result);return state.action_result;}
      if(state?.last_event?.type==='run_finished')throw new Error('Patch session finished before native action result arrived');
      await new Promise(resolve=>setTimeout(resolve,500));
    }
    throw new Error('Timed out waiting for native Patch action result');
  }

  async function submitItemAction(item,promptItem,action){
    if(actionBusy||!activeSessionId||!activeQueuePrompt)return null;
    if(!actionAllowed(action))throw new Error(`Patch action ${action} is not advertised by the active Python prompt`);
    const index=Number(promptItem?.index);
    if(!Number.isInteger(index)||index<1)throw new Error('Patch item action index is unavailable');
    actionBusy=true;
    renderActionPending(action,item);
    renderQueueRows();
    refreshActionDisabledState();
    try{
      const response=await app.jsonFetch(`/api/sessions/${encodeURIComponent(activeSessionId)}/item-action`,{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({prompt_id:String(activeQueuePrompt.prompt_id||''),action,index}),
      });
      const actionID=String(response?.action_id||'');
      if(!actionID)throw new Error('TaskDeck did not return an action_id');
      return await waitForActionResult(activeSessionId,actionID);
    }catch(error){
      actionResultMeta.textContent='Failed to request native action';
      actionResultOutput.textContent=String(error?.message||error);
      actionResultBox.hidden=false;
      throw error;
    }finally{
      actionBusy=false;
      renderQueueRows();
      refreshActionDisabledState();
    }
  }
  function queueViewItems(snapshot,view){
    const items=Array.isArray(snapshot?.items)?snapshot.items:[];
    const hasGroups=items.some(item=>item?.group==='new'||item?.group==='failed');
    if(!hasGroups)return view==='queue'?items:[];
    return items.filter(item=>view==='failed'?item?.group==='failed':item?.group==='new');
  }

  function renderQueueRows(){
    const snapshot=latestQueueSnapshot;
    if(!snapshot){
      summaryList.replaceChildren();
      return;
    }
    const items=queueViewItems(snapshot,queueSummaryView);
    const groupCounts=snapshot?.group_counts&&typeof snapshot.group_counts==='object'?snapshot.group_counts:{};
    const allItems=Array.isArray(snapshot?.items)?snapshot.items:[];
    const hasGroups=allItems.some(item=>item?.group==='new'||item?.group==='failed');
    const queueCount=hasGroups?Number(groupCounts.new??queueViewItems(snapshot,'queue').length):allItems.length;
    const failedCount=hasGroups?Number(groupCounts.failed??queueViewItems(snapshot,'failed').length):0;
    queueTab.textContent=`Queue (${queueCount})`;
    failedTab.textContent=`Failed (${failedCount})`;
    queueTab.classList.toggle('active',queueSummaryView==='queue');
    failedTab.classList.toggle('active',queueSummaryView==='failed');
    summaryTitle.textContent=queueSummaryView==='failed'?'Failed':'Queue';
    summaryStatus.textContent=items.length?`${items.length} item(s)`:'Empty';
    summaryList.replaceChildren();
    const visible=items.slice(0,50);
    for(const item of visible){
      const row=document.createElement('div');row.className='task-patch-summary-item';
      const name=document.createElement('span');name.className='task-patch-summary-name';name.textContent=String(item?.name||'');
      const detail=document.createElement('span');detail.className='task-patch-summary-detail';
      detail.textContent=[item?.kind,item?.detail].filter(Boolean).join(' · ');
      row.append(name,detail);
      if(queueSummaryView==='failed'){
        const failure=item?.failure&&typeof item.failure==='object'?item.failure:{};
        const failureLine=document.createElement('span');failureLine.className='task-patch-summary-failure';
        const rc=failure?.rc;
        const status=String(failure?.status||'');
        const diagnosis=String(failure?.diagnosis_kind||'');
        const message=String(failure?.message||'');
        failureLine.textContent=[
          status+(rc===undefined||rc===null?'':` rc=${rc}`),
          diagnosis,
          message,
        ].filter(Boolean).join(' · ');
        if(failureLine.textContent)row.append(failureLine);
      }
      const promptItem=promptItemForQueueItem(item);
      if(promptItem&&String(item?.kind||'').toUpperCase()==='PATCH'){
        const itemActions=document.createElement('div');itemActions.className='task-patch-summary-item-actions';
        for(const action of ['inspect','preview','validate']){
          if(!actionAllowed(action))continue;
          const button=document.createElement('button');button.type='button';button.className='task-patch-item-action';button.textContent=action.charAt(0).toUpperCase()+action.slice(1);button.disabled=actionBusy;
          button.onclick=()=>submitItemAction(item,promptItem,action).catch(app.showError);
          itemActions.append(button);
        }
        if(itemActions.childElementCount)row.append(itemActions);
      }
      summaryList.append(row);
    }
    if(!visible.length){
      const empty=document.createElement('div');empty.className='task-patch-summary-empty';
      empty.textContent=queueSummaryView==='failed'?'No unresolved failed item':'No new queue item';
      summaryList.append(empty);
    }else if(items.length>visible.length){
      const more=document.createElement('div');more.className='task-patch-summary-warning';more.textContent=`+${items.length-visible.length} more item(s)`;
      summaryList.append(more);
    }
  }

  function setQueueSummaryView(view){
    queueSummaryView=view==='failed'?'failed':'queue';
    renderQueueRows();
  }

  function renderQueueSnapshot(snapshot){
    latestQueueSnapshot=snapshot&&typeof snapshot==='object'?snapshot:{items:[],counts:{},group_counts:{},warnings:[],total:0,status:'empty'};
    const counts=latestQueueSnapshot?.counts&&typeof latestQueueSnapshot.counts==='object'?latestQueueSnapshot.counts:{};
    summaryCounts.replaceChildren();
    for(const [kind,count] of Object.entries(counts).sort(([a],[b])=>a.localeCompare(b))){
      const chip=document.createElement('span');
      chip.className='task-patch-summary-count';
      chip.textContent=`${kind}: ${count}`;
      summaryCounts.append(chip);
    }
    const warnings=Array.isArray(latestQueueSnapshot?.warnings)?latestQueueSnapshot.warnings:[];
    summaryWarnings.textContent=warnings.length?`${warnings.length} warning(s): ${warnings.slice(0,3).join(' | ')}`:'';
    renderQueueRows();
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
      runBox.hidden=!runningMode;
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
    activeQueuePrompt=null;
    promptBox.hidden=true;
    promptTitle.textContent='';
    promptNote.textContent='';
    promptItems.replaceChildren();
    promptButtons.replaceChildren();
    renderQueueRows();
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
      if(action==='select')enterRunningView();
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
    activeQueuePrompt=prompt;
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
    renderQueueRows();
    refreshActionDisabledState();
    return true;
  }

  async function pollProtocol(sessionId,expectPrompt=false,followLifecycle=false){
    const generation=++protocolPollGeneration;
    resetSummary('Loading…');
    clearPrompt();
    let haveSnapshot=false;
    let haveResumeSnapshot=false;
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
        if(sessionId===activeSessionId)openTerminalEvidence();
        return;
      }
      if(state?.available===false){
        resetSummary('PTY-only');
        if(sessionId===activeSessionId)openTerminalEvidence();
        return;
      }
      if(state?.queue_snapshot&&!haveSnapshot){
        renderQueueSnapshot(state.queue_snapshot);
        haveSnapshot=true;
      }
      if(state?.resume_snapshot&&!haveResumeSnapshot){
        renderResumeSnapshot(state.resume_snapshot);
        haveResumeSnapshot=true;
      }
      renderItemLifecycle(state?.items);
      renderProgress(state?.progress);
      if(state?.action_result)renderActionResult(state.action_result);
      renderArtifacts(state?.artifacts);
      if(expectPrompt&&state?.commands_enabled===false&&(haveSnapshot||haveResumeSnapshot)){
        summaryStatus.textContent+=' · Continue in PTY';
        if(haveResumeSnapshot)resumeNote.textContent='Native Resume command channel unavailable. Continue in terminal.';
        if(sessionId===activeSessionId)openTerminalEvidence();
        return;
      }
      if(expectPrompt&&state?.prompt&&renderResumePrompt(sessionId,state.prompt)){
        summaryStatus.textContent+=' · Smart Resume';
        return;
      }
      if(expectPrompt&&state?.prompt&&renderQueuePrompt(sessionId,state.prompt)){
        summaryStatus.textContent+=' · Awaiting selection';
        return;
      }
      if(state?.last_event?.type==='run_finished'){
        if(haveSnapshot)summaryStatus.textContent+=' · Finished';
        finishRunningView();
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
    if(haveSnapshot||haveResumeSnapshot){
      summaryStatus.textContent+=' · Continue in PTY';
      if(haveResumeSnapshot)resumeNote.textContent='Native Resume prompt timed out. Continue in terminal.';
    }else{
      resetSummary('Snapshot timeout · Continue in PTY');
    }
    if(sessionId===activeSessionId&&!runningMode)openTerminalEvidence();
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
      actionPollGeneration+=1;
      leaveRunningView();
      clearActionResult();
      clearResumeView();
      renderProgress(null);
      renderArtifacts([]);
      app.attachSession(meta,mode!=='queue'&&mode!=='resume');
      window.dispatchEvent(new CustomEvent('taskmenu:patch-session-started',{detail:{mode,meta}}));
      if(mode==='queue'||mode==='resume'||mode==='plan'){
        void pollProtocol(meta.id,mode==='queue'||mode==='resume',mode==='resume'||mode==='plan');
      }else{
        resetSummary('History uses PTY');
      }
      return meta;
    }finally{
      if(sourceButton?.isConnected)sourceButton.disabled=false;
    }
  }

  terminalEvidence.onclick=openTerminalEvidence;
  runningBack.onclick=()=>{if(runningFinished)leaveRunningView();};
  actionResultClose.onclick=clearActionResult;
  queueTab.onclick=()=>setQueueSummaryView('queue');
  failedTab.onclick=()=>setQueueSummaryView('failed');
  closeButton.onclick=close;
  globalThis.TaskMenuPatchPanel={open,close,toggle,start,enterRunningView,finishRunningView,leaveRunningView,openTerminalEvidence,renderQueueSnapshot,setQueueSummaryView,renderQueuePrompt,renderResumeSnapshot,renderResumePrompt,submitResumeAction,submitItemAction,renderActionResult,renderItemLifecycle,renderProgress,renderArtifacts,get panel(){return panel;},get visible(){return panel.classList.contains('visible');}};
  return true;
}

function safeInstallPatchPanel(){
  try{installPatchPanel();}
  catch(error){console.warn('Patch panel enhancement disabled:',error);}
}

if(app?.taskData?.workspace)safeInstallPatchPanel();
else window.addEventListener('taskmenu:tasks',safeInstallPatchPanel,{once:true});
