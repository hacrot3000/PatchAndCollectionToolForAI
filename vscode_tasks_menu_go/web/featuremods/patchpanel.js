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
  .task-patch-actions{display:grid;gap:7px}
  .task-patch-action{display:flex;flex-direction:column;align-items:flex-start;gap:2px;width:100%;padding:9px 10px;text-align:left}
  .task-patch-action strong{font-size:12px}
  .task-patch-action span{font-size:11px;opacity:.65}
  html[data-taskmenu-theme="light"] .task-patch-panel{background:#fff;border-color:#b9c0c8;box-shadow:10px 0 28px rgba(0,0,0,.12)}
  html[data-taskmenu-theme="light"] .task-patch-panel-head{border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-summary{background:#f6f8fa;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-summary-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-summary-count{border-color:#d0d7de}
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
  const actions=document.createElement('div');actions.className='task-patch-actions';
  body.append(note,summary,actions);
  panel.append(head,body);
  document.body.append(panel);

  const actionDefs=[
    ['queue','Queue','Open the normal PATCH/COLLECT queue'],
    ['resume','Resume','Continue an interrupted or failed run'],
    ['history','History','Open Patch Tool reports/history'],
    ['plan','Plan','Inspect the execution plan without replacing the Python engine'],
  ];
  const buttons=[];
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

  async function pollProtocol(sessionId){
    resetSummary('Loading…');
    for(let attempt=0;attempt<40;attempt+=1){
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
      if(state?.queue_snapshot){
        renderQueueSnapshot(state.queue_snapshot);
        return;
      }
      if(state?.error){
        resetSummary('Protocol error');
        summaryWarnings.textContent=String(state.error);
        return;
      }
      await new Promise(resolve=>setTimeout(resolve,250));
    }
    resetSummary('Snapshot timeout');
  }

  async function start(mode,sourceButton=null){
    if(sourceButton)sourceButton.disabled=true;
    try{
      const meta=await app.jsonFetch('/api/sessions',{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({kind:'patch',patch_mode:mode}),
      });
      app.attachSession(meta,true);
      window.dispatchEvent(new CustomEvent('taskmenu:patch-session-started',{detail:{mode,meta}}));
      if(mode==='queue'||mode==='resume'||mode==='plan'){
        void pollProtocol(meta.id);
      }else{
        resetSummary('History uses PTY');
      }
      return meta;
    }finally{
      if(sourceButton?.isConnected)sourceButton.disabled=false;
    }
  }

  closeButton.onclick=close;
  globalThis.TaskMenuPatchPanel={open,close,toggle,start,renderQueueSnapshot,get panel(){return panel;},get visible(){return panel.classList.contains('visible');}};
  return true;
}

function safeInstallPatchPanel(){
  try{installPatchPanel();}
  catch(error){console.warn('Patch panel enhancement disabled:',error);}
}

if(app?.taskData?.workspace)safeInstallPatchPanel();
else window.addEventListener('taskmenu:tasks',safeInstallPatchPanel,{once:true});
