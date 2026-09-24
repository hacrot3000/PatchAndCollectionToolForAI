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
  .task-patch-summary-search{display:flex;gap:5px;margin:0 0 7px}
  .task-patch-summary-search input{min-width:0;flex:1;padding:5px 7px;font-size:11px}
  .task-patch-summary-search button{padding:5px 7px;font-size:10px}
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
  .task-patch-queue-delete{border-color:#81424a;background:#3b2025;color:#ffd9dd}
  .task-patch-queue-delete:hover{background:#4a272d}
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
  .task-patch-prompt-tools{display:flex;gap:5px;margin-bottom:7px}
  .task-patch-prompt-tools button{font-size:10px;padding:3px 6px}
  .task-patch-prompt-items{display:grid;gap:4px;max-height:320px;overflow:auto}
  .task-patch-prompt-item{display:flex;align-items:flex-start;gap:7px;padding:5px 6px;border-radius:4px;background:#171f2a}
  .task-patch-prompt-item input{margin-top:2px}
  .task-patch-prompt-copy{min-width:0;flex:1;cursor:pointer}
  .task-patch-prompt-priority{display:flex;align-items:center;gap:4px;font-size:10px;white-space:nowrap}
  .task-patch-prompt-priority select{min-width:44px;padding:2px 3px;font-size:10px}
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
  .task-patch-history{margin:0 0 10px;padding:8px;border:1px solid #4b596d;border-radius:6px;background:#121923;font-size:11px}
  .task-patch-history[hidden]{display:none}
  .task-patch-history-head{display:flex;align-items:center;gap:5px;margin-bottom:7px}
  .task-patch-history-title{font-weight:700;flex:1}
  .task-patch-history-status{opacity:.7}
  .task-patch-history-head button{font-size:10px;padding:3px 6px}
  .task-patch-history-runs{display:grid;gap:4px;max-height:260px;overflow:auto;margin-bottom:8px}
  .task-patch-history-run-row{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:5px;align-items:stretch}
  .task-patch-history-run{display:flex;min-width:0;flex-direction:column;align-items:flex-start;gap:2px;padding:6px 7px;text-align:left;background:#171f2a}
  .task-patch-history-run.active{border-color:#71839b;background:#202a37}
  .task-patch-history-run-name{font-weight:600;max-width:100%;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-history-run-meta{font-size:10px;opacity:.68}
  .task-patch-history-run-actions{display:flex;gap:4px;align-items:center;flex-wrap:wrap;justify-content:flex-end}
  .task-patch-history-run-actions button{font-size:9px;padding:3px 5px}
  .task-patch-history-delete{border-color:#81424a;background:#3b2025;color:#ffd9dd}
  .task-patch-history-delete:hover{background:#4a272d}
  .task-patch-history-management{margin:0 0 8px;padding:6px;border:1px solid #3f4b5d;border-radius:5px;background:#0d1015}
  .task-patch-history-management[hidden]{display:none}
  .task-patch-history-management-message{font-weight:600;overflow-wrap:anywhere}
  .task-patch-history-management-files{display:grid;gap:5px;margin-top:5px}
  .task-patch-history-detail{border-top:1px solid #303946;padding-top:7px}
  .task-patch-history-detail[hidden]{display:none}
  .task-patch-history-detail-title{font-weight:700;margin-bottom:3px;overflow-wrap:anywhere}
  .task-patch-history-detail-meta{opacity:.7;margin-bottom:6px}
  .task-patch-history-files,.task-patch-history-items{display:grid;gap:5px}
  .task-patch-history-files{margin-bottom:7px}
  .task-patch-history-file,.task-patch-history-item{padding:6px;border-radius:4px;background:#171c23}
  .task-patch-history-file-head{display:flex;align-items:center;gap:5px}
  .task-patch-history-file-label{font-weight:600;min-width:0;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .task-patch-history-upload{font-size:9px;padding:1px 4px;border:1px solid #7b6840;border-radius:999px}
  .task-patch-history-path{display:block;margin-top:2px;opacity:.64;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-history-file-actions{display:flex;gap:5px;margin-top:5px}
  .task-patch-history-file-actions a,.task-patch-history-file-actions button{font-size:10px;padding:3px 6px}
  .task-patch-history-item-head{display:flex;gap:5px;align-items:flex-start}
  .task-patch-history-item-name{font-weight:600;min-width:0;flex:1;overflow-wrap:anywhere}
  .task-patch-history-item-status{font-weight:700;white-space:nowrap}
  .task-patch-history-item-detail{margin-top:2px;opacity:.7;overflow-wrap:anywhere}
  .task-patch-history-warning{margin-top:6px;opacity:.72;overflow-wrap:anywhere}
  .task-patch-plan{margin:0 0 10px;padding:8px;border:1px solid #4b596d;border-radius:6px;background:#121923;font-size:11px}
  .task-patch-plan[hidden]{display:none}
  .task-patch-plan-head{display:flex;align-items:center;gap:5px;margin-bottom:7px}
  .task-patch-plan-title{font-weight:700;flex:1}
  .task-patch-plan-status{opacity:.72}
  .task-patch-plan-head button{font-size:10px;padding:3px 6px}
  .task-patch-plan-overview{display:flex;flex-wrap:wrap;gap:5px;margin-bottom:7px}
  .task-patch-plan-chip{padding:2px 5px;border:1px solid #3b4655;border-radius:999px}
  .task-patch-plan-section{margin-top:8px}
  .task-patch-plan-section-title{font-weight:700;margin-bottom:4px}
  .task-patch-plan-list{display:grid;gap:4px}
  .task-patch-plan-row{padding:6px;border-radius:4px;background:#171f2a}
  .task-patch-plan-row-head{display:flex;align-items:flex-start;gap:5px}
  .task-patch-plan-row-name{font-weight:600;min-width:0;flex:1;overflow-wrap:anywhere}
  .task-patch-plan-row-status{font-weight:700;white-space:nowrap}
  .task-patch-plan-row-detail{margin-top:2px;opacity:.7;overflow-wrap:anywhere}
  .task-patch-plan-note{margin-top:6px;opacity:.72;overflow-wrap:anywhere}
  .task-patch-panel.plan .task-patch-panel-note,
  .task-patch-panel.plan .task-patch-summary,
  .task-patch-panel.plan .task-patch-action-result,
  .task-patch-panel.plan .task-patch-prompt,
  .task-patch-panel.plan .task-patch-resume,
  .task-patch-panel.plan .task-patch-history,
  .task-patch-panel.plan .task-patch-running-head,
  .task-patch-panel.plan .task-patch-run,
  .task-patch-panel.plan .task-patch-artifacts,
  .task-patch-panel.plan .task-patch-actions{display:none!important}
  .task-patch-panel.history .task-patch-panel-note,
  .task-patch-panel.history .task-patch-summary,
  .task-patch-panel.history .task-patch-action-result,
  .task-patch-panel.history .task-patch-prompt,
  .task-patch-panel.history .task-patch-resume,
  .task-patch-panel.history .task-patch-running-head,
  .task-patch-panel.history .task-patch-run,
  .task-patch-panel.history .task-patch-artifacts,
  .task-patch-panel.history .task-patch-actions{display:none!important}
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
  html[data-taskmenu-theme="light"] .task-patch-queue-delete{background:#fff0f1;border-color:#c47780;color:#7b2029}
  html[data-taskmenu-theme="light"] .task-patch-queue-delete:hover{background:#ffe5e7}
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
  html[data-taskmenu-theme="light"] .task-patch-plan{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-plan-row{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-plan-chip{border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-history{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-history-run,
  html[data-taskmenu-theme="light"] .task-patch-history-file,
  html[data-taskmenu-theme="light"] .task-patch-history-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-history-run.active{background:#e7eef7;border-color:#9aa9bc}
  html[data-taskmenu-theme="light"] .task-patch-history-delete{background:#fff0f1;border-color:#c47780;color:#7b2029}
  html[data-taskmenu-theme="light"] .task-patch-history-delete:hover{background:#ffe5e7}
  html[data-taskmenu-theme="light"] .task-patch-history-management{background:#fff;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-history-detail{border-color:#d0d7de}
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
  const summarySearch=document.createElement('div');summarySearch.className='task-patch-summary-search';
  const summarySearchInput=document.createElement('input');summarySearchInput.type='search';summarySearchInput.placeholder='Search name / id / summary / target';summarySearchInput.autocomplete='off';summarySearchInput.spellcheck=false;summarySearchInput.setAttribute('aria-label','Filter Patch queue');
  const summarySearchClear=document.createElement('button');summarySearchClear.type='button';summarySearchClear.textContent='Clear';summarySearchClear.title='Clear queue search';
  summarySearch.append(summarySearchInput,summarySearchClear);
  const summaryCounts=document.createElement('div');summaryCounts.className='task-patch-summary-counts';
  const summaryList=document.createElement('div');summaryList.className='task-patch-summary-list';
  const summaryWarnings=document.createElement('div');summaryWarnings.className='task-patch-summary-warning';
  summaryHead.append(summaryTitle,summaryStatus);
  summary.append(summaryHead,summaryTabs,summarySearch,summaryCounts,summaryList,summaryWarnings);

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
  const promptTools=document.createElement('div');promptTools.className='task-patch-prompt-tools';
  const promptItems=document.createElement('div');promptItems.className='task-patch-prompt-items';
  const promptButtons=document.createElement('div');promptButtons.className='task-patch-prompt-buttons';
  promptBox.append(promptTitle,promptNote,promptTools,promptItems,promptButtons);

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

  const historyBox=document.createElement('div');historyBox.className='task-patch-history';historyBox.hidden=true;
  const historyHead=document.createElement('div');historyHead.className='task-patch-history-head';
  const historyTitle=document.createElement('div');historyTitle.className='task-patch-history-title';historyTitle.textContent='History';
  const historyStatus=document.createElement('div');historyStatus.className='task-patch-history-status';
  const historyTerminal=document.createElement('button');historyTerminal.type='button';historyTerminal.textContent='Terminal';historyTerminal.title='Open terminal evidence/fallback';
  const historyBack=document.createElement('button');historyBack.type='button';historyBack.textContent='Back';
  historyHead.append(historyTitle,historyStatus,historyTerminal,historyBack);
  const historyRuns=document.createElement('div');historyRuns.className='task-patch-history-runs';
  const historyManagement=document.createElement('div');historyManagement.className='task-patch-history-management';historyManagement.hidden=true;
  const historyManagementMessage=document.createElement('div');historyManagementMessage.className='task-patch-history-management-message';
  const historyManagementFiles=document.createElement('div');historyManagementFiles.className='task-patch-history-management-files';
  historyManagement.append(historyManagementMessage,historyManagementFiles);
  const historyDetail=document.createElement('div');historyDetail.className='task-patch-history-detail';historyDetail.hidden=true;
  const historyDetailTitle=document.createElement('div');historyDetailTitle.className='task-patch-history-detail-title';
  const historyDetailMeta=document.createElement('div');historyDetailMeta.className='task-patch-history-detail-meta';
  const historyFiles=document.createElement('div');historyFiles.className='task-patch-history-files';
  const historyItems=document.createElement('div');historyItems.className='task-patch-history-items';
  const historyWarnings=document.createElement('div');historyWarnings.className='task-patch-history-warning';
  historyDetail.append(historyDetailTitle,historyDetailMeta,historyFiles,historyItems,historyWarnings);
  historyBox.append(historyHead,historyRuns,historyManagement,historyDetail);

  const planBox=document.createElement('div');planBox.className='task-patch-plan';planBox.hidden=true;
  const planHead=document.createElement('div');planHead.className='task-patch-plan-head';
  const planTitle=document.createElement('div');planTitle.className='task-patch-plan-title';planTitle.textContent='Plan';
  const planStatus=document.createElement('div');planStatus.className='task-patch-plan-status';
  const planTerminal=document.createElement('button');planTerminal.type='button';planTerminal.textContent='Terminal';planTerminal.title='Open terminal evidence/fallback';
  const planBack=document.createElement('button');planBack.type='button';planBack.textContent='Back';
  planHead.append(planTitle,planStatus,planTerminal,planBack);
  const planOverview=document.createElement('div');planOverview.className='task-patch-plan-overview';
  const planPrevious=document.createElement('div');planPrevious.className='task-patch-plan-note';
  const planItemsSection=document.createElement('div');planItemsSection.className='task-patch-plan-section';
  const planItemsTitle=document.createElement('div');planItemsTitle.className='task-patch-plan-section-title';planItemsTitle.textContent='Ordered items';
  const planItems=document.createElement('div');planItems.className='task-patch-plan-list';planItemsSection.append(planItemsTitle,planItems);
  const planConflictsSection=document.createElement('div');planConflictsSection.className='task-patch-plan-section';
  const planConflictsTitle=document.createElement('div');planConflictsTitle.className='task-patch-plan-section-title';planConflictsTitle.textContent='Static conflicts';
  const planConflicts=document.createElement('div');planConflicts.className='task-patch-plan-list';planConflictsSection.append(planConflictsTitle,planConflicts);
  const planResourcesSection=document.createElement('div');planResourcesSection.className='task-patch-plan-section';
  const planResourcesTitle=document.createElement('div');planResourcesTitle.className='task-patch-plan-section-title';planResourcesTitle.textContent='Resources';
  const planResources=document.createElement('div');planResources.className='task-patch-plan-list';planResourcesSection.append(planResourcesTitle,planResources);
  const planPreviewsSection=document.createElement('div');planPreviewsSection.className='task-patch-plan-section';
  const planPreviewsTitle=document.createElement('div');planPreviewsTitle.className='task-patch-plan-section-title';planPreviewsTitle.textContent='Previews';
  const planPreviews=document.createElement('div');planPreviews.className='task-patch-plan-list';planPreviewsSection.append(planPreviewsTitle,planPreviews);
  const planWarnings=document.createElement('div');planWarnings.className='task-patch-plan-note';
  planBox.append(planHead,planOverview,planPrevious,planItemsSection,planConflictsSection,planResourcesSection,planPreviewsSection,planWarnings);

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
  body.append(note,summary,actionResultBox,promptBox,resumeBox,historyBox,planBox,runningHead,runBox,artifactBox,actions);
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
  let queueSearchQuery='';
  let activeQueuePrompt=null;
  let latestResumeSnapshot=null;
  let activeResumePrompt=null;
  let resumeBusy=false;
  let actionBusy=false;
  let actionPollGeneration=0;
  let queueMutationBusy=false;
  let queueMutationPollGeneration=0;
  let historyPollGeneration=0;
  let latestHistorySnapshot=null;
  let activeHistoryPrompt=null;
  let activeHistoryRunID='';
  let historyBusy=false;
  let historyManagementBusy=false;
  let historyManagementPollGeneration=0;
  let historyMode=false;
  let latestPlanSnapshot=null;
  let planMode=false;
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
    if(!visible){protocolPollGeneration+=1;actionPollGeneration+=1;queueMutationPollGeneration+=1;historyPollGeneration+=1;historyManagementPollGeneration+=1;}
    if(visible&&activeSessionId)void pollProtocol(activeSessionId,!runningMode&&!planMode,true);
    window.dispatchEvent(new CustomEvent('taskmenu:patch-panel-visible',{detail:{visible}}));
  }
  function open(){setVisible(true);}
  function close(){setVisible(false);}
  function toggle(){setVisible(!panel.classList.contains('visible'));}

  function resetSummary(status='Not loaded'){
    latestQueueSnapshot=null;
    queueSearchQuery='';
    summarySearchInput.value='';
    summarySearchInput.disabled=true;
    summarySearchClear.disabled=true;
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

  function enterHistoryView(){
    historyMode=true;
    panel.classList.add('history');
    historyBox.hidden=false;
    historyStatus.textContent='Loading…';
  }

  function leaveHistoryView(){
    historyMode=false;
    panel.classList.remove('history');
    historyBox.hidden=true;
    latestHistorySnapshot=null;
    activeHistoryPrompt=null;
    activeHistoryRunID='';
    historyBusy=false;
    historyManagementBusy=false;
    historyPollGeneration+=1;
    historyManagementPollGeneration+=1;
    historyRuns.replaceChildren();
    historyManagement.hidden=true;
    historyManagementMessage.textContent='';
    historyManagementFiles.replaceChildren();
    historyDetail.hidden=true;
    historyDetailTitle.textContent='';
    historyDetailMeta.textContent='';
    historyFiles.replaceChildren();
    historyItems.replaceChildren();
    historyWarnings.textContent='';
  }

  function historySafeProjectPath(value){
    const path=String(value||'').trim();
    if(!path||path.startsWith('/')||path.includes('\\'))return '';
    const parts=path.split('/');
    if(parts.some(part=>!part||part==='.'||part==='..'))return '';
    return path;
  }

  function historyTextFile(path){
    return /\.(?:txt|log|json|md|markdown|patch|diff|csv|xml|ya?ml|ini|cfg|conf)$/i.test(path);
  }

  function appendHistoryFile(host,file){
    const path=historySafeProjectPath(file?.path);
    if(!path)return;
    const row=document.createElement('div');row.className='task-patch-history-file';
    const head=document.createElement('div');head.className='task-patch-history-file-head';
    const label=document.createElement('span');label.className='task-patch-history-file-label';label.textContent=String(file?.label||'File');
    head.append(label);
    if(file?.upload_required){
      const badge=document.createElement('span');badge.className='task-patch-history-upload';badge.textContent='UPLOAD';head.append(badge);
    }
    const pathNode=document.createElement('span');pathNode.className='task-patch-history-path';pathNode.textContent=path;pathNode.title=path;
    const actionsNode=document.createElement('div');actionsNode.className='task-patch-history-file-actions';
    const download=document.createElement('a');download.textContent='Download';download.href='/api/files/download?path='+encodeURIComponent(path);download.download='';
    actionsNode.append(download);
    if(historyTextFile(path)){
      const open=document.createElement('button');open.type='button';open.textContent='Open';
      open.onclick=()=>window.dispatchEvent(new CustomEvent('taskmenu:project-file-open-request',{detail:{path}}));
      actionsNode.append(open);
    }
    row.append(head,pathNode,actionsNode);host.append(row);
  }

  function clearHistoryManagementResult(){
    historyManagement.hidden=true;
    historyManagementMessage.textContent='';
    historyManagementFiles.replaceChildren();
  }

  function renderHistoryManagementResult(result){
    if(!result||typeof result!=='object'){clearHistoryManagementResult();return false;}
    const status=String(result.status||'');
    const action=String(result.action||'');
    const rc=Number(result.rc);
    historyManagementMessage.textContent=[
      String(result.message||'History management result'),
      status,
      Number.isFinite(rc)?('rc='+rc):'',
    ].filter(Boolean).join(' · ');
    historyManagementFiles.replaceChildren();
    if(result.artifact&&typeof result.artifact==='object')appendHistoryFile(historyManagementFiles,result.artifact);
    historyManagement.hidden=false;
    return true;
  }

  function historyPromptRun(runID){
    const runs=Array.isArray(activeHistoryPrompt?.runs)?activeHistoryPrompt.runs:[];
    return runs.find(row=>String(row?.run_id||'')===String(runID||''))||null;
  }

  function historyActionsForRun(runID){
    const top=new Set(Array.isArray(activeHistoryPrompt?.actions)?activeHistoryPrompt.actions.map(value=>String(value).toLowerCase()):[]);
    const row=historyPromptRun(runID);
    const actions=Array.isArray(row?.actions)?row.actions.map(value=>String(value).toLowerCase()):[];
    return new Set(actions.filter(action=>top.has(action)));
  }

  async function waitForHistoryManagement(sessionId,managementID,promptID,runID,snapshotBefore){
    const generation=++historyManagementPollGeneration;
    let matched=null;
    for(let attempt=0;attempt<240;attempt+=1){
      if(generation!==historyManagementPollGeneration||!panel.classList.contains('visible')||sessionId!==activeSessionId)return null;
      const state=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/protocol`);
      const result=state?.history_management_result;
      if(result?.management_id===managementID){
        matched=result;
        renderHistoryManagementResult(result);
        if(result?.history_changed!==true)return result;
      }
      if(matched?.history_changed===true&&state?.history_snapshot){
        const snapshotToken=JSON.stringify(state.history_snapshot);
        const runs=Array.isArray(state.history_snapshot?.runs)?state.history_snapshot.runs:[];
        const prompt=state?.prompt;
        const freshPrompt=prompt?.type==='prompt'&&prompt?.prompt_kind==='history_action'&&String(prompt?.prompt_id||'')!==promptID;
        if(snapshotToken!==snapshotBefore&&(runs.length===0||freshPrompt)){
          const activeStillExists=runs.some(row=>String(row?.run_id||'')===activeHistoryRunID);
          if(activeHistoryRunID&&!activeStillExists){
            activeHistoryRunID='';
            historyDetail.hidden=true;
            historyDetailTitle.textContent='';
            historyDetailMeta.textContent='';
            historyFiles.replaceChildren();
            historyItems.replaceChildren();
            historyWarnings.textContent='';
          }else if(activeHistoryRunID===runID){
            historyDetail.hidden=true;
          }
          renderHistorySnapshot(state.history_snapshot);
          if(runs.length&&freshPrompt)renderHistoryPrompt(sessionId,prompt);
          else if(!runs.length){
            activeHistoryPrompt=null;
            historyStatus.textContent='0 run(s) · native';
            renderHistoryRuns();
          }
          return matched;
        }
      }
      if(state?.last_event?.type==='run_finished'&&!matched)throw new Error('Patch History session finished before management result arrived');
      await new Promise(resolve=>setTimeout(resolve,250));
    }
    throw new Error('Timed out waiting for native Patch History management result');
  }

  async function submitHistoryManagement(sessionId,prompt,action,runID){
    if(historyManagementBusy||historyBusy||!sessionId||!prompt)return null;
    action=String(action||'').toLowerCase();
    const allowed=historyActionsForRun(runID);
    if(!allowed.has(action))throw new Error(`History action ${action} is not advertised for this run`);
    const confirmed=action==='delete';
    if(confirmed&&!window.confirm(`Delete Patch Tool History run ${runID}? This removes its stored report/run artifacts.`))return null;
    const promptID=String(prompt.prompt_id||'');
    const snapshotBefore=JSON.stringify(latestHistorySnapshot||{});
    historyManagementBusy=true;
    historyManagement.hidden=false;
    historyManagementFiles.replaceChildren();
    historyManagementMessage.textContent=`${action.charAt(0).toUpperCase()+action.slice(1)}…`;
    renderHistoryRuns();
    try{
      const response=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/history-manage`,{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({prompt_id:promptID,action,run_id:runID,confirmed}),
      });
      const managementID=String(response?.management_id||'');
      if(!managementID)throw new Error('TaskDeck did not return a management_id');
      return await waitForHistoryManagement(sessionId,managementID,promptID,runID,snapshotBefore);
    }finally{
      historyManagementBusy=false;
      renderHistoryRuns();
    }
  }

  function renderHistoryRuns(){
    historyRuns.replaceChildren();
    const runs=Array.isArray(latestHistorySnapshot?.runs)?latestHistorySnapshot.runs:[];
    if(!runs.length){
      const empty=document.createElement('div');empty.className='task-patch-summary-empty';empty.textContent='No Patch Tool history yet';historyRuns.append(empty);
      return;
    }
    for(const run of runs){
      const runID=String(run?.run_id||'');
      if(!runID)continue;
      const allowed=historyActionsForRun(runID);
      const row=document.createElement('div');row.className='task-patch-history-run-row';
      const button=document.createElement('button');button.type='button';button.className='task-patch-history-run';button.classList.toggle('active',runID===activeHistoryRunID);
      button.disabled=historyBusy||historyManagementBusy||!allowed.has('detail');
      const name=document.createElement('span');name.className='task-patch-history-run-name';name.textContent=String(run?.primary_name||runID);
      const elapsed=Number(run?.elapsed_seconds);
      const meta=document.createElement('span');meta.className='task-patch-history-run-meta';
      meta.textContent=[run?.display_time,run?.status,run?.pinned?'PINNED':'',Number.isFinite(elapsed)?elapsed.toFixed(1)+'s':'',run?.item_count===undefined?'':String(run.item_count)+' item(s)'].filter(Boolean).join(' · ');
      button.append(name,meta);
      button.onclick=()=>submitHistoryDetail(activeSessionId,activeHistoryPrompt,runID).catch(app.showError);
      row.append(button);
      const managementActions=document.createElement('div');managementActions.className='task-patch-history-run-actions';
      for(const action of ['pin','unpin','export','delete']){
        if(!allowed.has(action))continue;
        const manage=document.createElement('button');manage.type='button';manage.dataset.historyAction=action;
        manage.textContent={pin:'Pin',unpin:'Unpin',export:'Export',delete:'Delete'}[action]||action;
        manage.disabled=historyBusy||historyManagementBusy;
        if(action==='delete')manage.className='task-patch-history-delete';
        manage.onclick=()=>submitHistoryManagement(activeSessionId,activeHistoryPrompt,action,runID).catch(app.showError);
        managementActions.append(manage);
      }
      if(managementActions.childElementCount)row.append(managementActions);
      historyRuns.append(row);
    }
  }

  function renderHistorySnapshot(snapshot){
    latestHistorySnapshot=snapshot&&typeof snapshot==='object'?snapshot:{status:'empty',runs:[],total:0};
    enterHistoryView();
    const runs=Array.isArray(latestHistorySnapshot.runs)?latestHistorySnapshot.runs:[];
    const total=Number(latestHistorySnapshot.total??runs.length);
    historyStatus.textContent=latestHistorySnapshot.status==='empty'?'Empty':`${runs.length}/${Number.isFinite(total)?total:runs.length} run(s)`;
    renderHistoryRuns();
  }

  function renderHistoryReport(report){
    if(!report||typeof report!=='object')return false;
    historyDetail.hidden=false;
    historyFiles.replaceChildren();historyItems.replaceChildren();historyWarnings.textContent='';
    if(report.status!=='available'){
      historyDetailTitle.textContent='History run unavailable';
      historyDetailMeta.textContent=String(report.run_id||'');
      return false;
    }
    const run=report.run&&typeof report.run==='object'?report.run:{};
    activeHistoryRunID=String(run.run_id||activeHistoryRunID);
    const elapsed=Number(run.elapsed_seconds);
    historyDetailTitle.textContent=String(run.primary_name||run.run_id||'History run');
    historyDetailMeta.textContent=[run.display_time,run.status,run.pinned?'PINNED':'',Number.isFinite(elapsed)?elapsed.toFixed(2)+'s':'',run.failure_policy?('failure='+run.failure_policy):'',run.transaction_policy?('transaction='+run.transaction_policy):''].filter(Boolean).join(' · ');
    for(const file of Array.isArray(report.files)?report.files:[])appendHistoryFile(historyFiles,file);
    for(const item of Array.isArray(report.items)?report.items:[]){
      const row=document.createElement('div');row.className='task-patch-history-item';
      const head=document.createElement('div');head.className='task-patch-history-item-head';
      const name=document.createElement('span');name.className='task-patch-history-item-name';name.textContent=`${Number(item?.index||0)}. ${String(item?.name||'')}`;
      const status=document.createElement('span');status.className='task-patch-history-item-status';
      status.textContent=[item?.status,item?.rc===undefined?'':('rc='+item.rc)].filter(Boolean).join(' ');
      head.append(name,status);
      const detail=document.createElement('div');detail.className='task-patch-history-item-detail';
      const elapsedItem=Number(item?.elapsed_seconds);
      detail.textContent=[item?.kind,item?.diagnosis,item?.summary,item?.batch_rolled_back?'rolled back':'',Number.isFinite(elapsedItem)?elapsedItem.toFixed(2)+'s':'',item?.changed_count===undefined?'':('changed='+item.changed_count)].filter(Boolean).join(' · ');
      row.append(head,detail);
      const artifacts=Array.isArray(item?.artifacts)?item.artifacts:[];
      if(artifacts.length){
        const files=document.createElement('div');files.className='task-patch-history-files';
        for(const artifact of artifacts)appendHistoryFile(files,artifact);
        row.append(files);
      }
      historyItems.append(row);
    }
    const totalItems=Number(report.total_items||0);
    if(report.items_truncated)historyWarnings.textContent=`Showing bounded History detail; total items=${Number.isFinite(totalItems)?totalItems:'?'}.`;
    const warnings=Array.isArray(report.warnings)?report.warnings:[];
    if(warnings.length)historyWarnings.textContent=[historyWarnings.textContent,warnings.slice(0,10).join(' | ')].filter(Boolean).join(' ');
    renderHistoryRuns();
    return true;
  }

  async function waitForHistoryReport(sessionId,runID){
    const generation=++historyPollGeneration;
    for(let attempt=0;attempt<240;attempt+=1){
      if(generation!==historyPollGeneration||!panel.classList.contains('visible')||sessionId!==activeSessionId)return null;
      const state=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/protocol`);
      const report=state?.history_report;
      if(report?.status==='available'&&String(report?.run?.run_id||'')===runID){
        renderHistoryReport(report);
        return report;
      }
      if(state?.last_event?.type==='run_finished')throw new Error('Patch History session finished before detail arrived');
      await new Promise(resolve=>setTimeout(resolve,250));
    }
    throw new Error('Timed out waiting for native Patch History detail');
  }

  async function submitHistoryDetail(sessionId,prompt,runID){
    if(historyBusy||historyManagementBusy||!sessionId||!prompt)return null;
    const actionsAllowed=new Set(Array.isArray(prompt.actions)?prompt.actions.map(String):[]);
    const advertised=new Set(Array.isArray(prompt.runs)?prompt.runs.map(row=>String(row?.run_id||'')):[]);
    if(!actionsAllowed.has('detail')||!advertised.has(runID))throw new Error('History run is not advertised by the active Python prompt');
    historyBusy=true;activeHistoryRunID=runID;renderHistoryRuns();
    historyDetail.hidden=false;historyDetailTitle.textContent='Loading History run…';historyDetailMeta.textContent=runID;historyFiles.replaceChildren();historyItems.replaceChildren();historyWarnings.textContent='';
    try{
      await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/history-detail`,{
        method:'POST',headers:{'Content-Type':'application/json'},
        body:JSON.stringify({prompt_id:String(prompt.prompt_id||''),run_id:runID}),
      });
      return await waitForHistoryReport(sessionId,runID);
    }finally{
      historyBusy=false;renderHistoryRuns();
    }
  }

  function renderHistoryPrompt(sessionId,prompt){
    if(prompt?.type!=='prompt'||prompt?.prompt_kind!=='history_action'||!prompt?.prompt_id)return false;
    activeHistoryPrompt=prompt;
    enterHistoryView();
    if(!latestHistorySnapshot)renderHistorySnapshot({status:Array.isArray(prompt.runs)&&prompt.runs.length?'available':'empty',runs:Array.isArray(prompt.runs)?prompt.runs:[],total:Array.isArray(prompt.runs)?prompt.runs.length:0,default_run_id:prompt.default_run_id||''});
    else renderHistoryRuns();
    historyStatus.textContent=(Array.isArray(prompt.runs)?prompt.runs.length:0)+' run(s) · native';
    const defaultRun=String(prompt.default_run_id||latestHistorySnapshot?.default_run_id||'');
    if(defaultRun&&!activeHistoryRunID&&!historyBusy){
      void submitHistoryDetail(sessionId,prompt,defaultRun).catch(error=>{historyWarnings.textContent=String(error?.message||error);});
    }
    return true;
  }

  async function stopHistoryAndBack(){
    const sessionId=activeSessionId;
    historyPollGeneration+=1;protocolPollGeneration+=1;
    if(sessionId){
      try{await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/stop`,{method:'POST'});}catch{}
    }
    activeSessionId='';
    leaveHistoryView();
    resetSummary();
  }

  function enterPlanView(){
    planMode=true;
    panel.classList.add('plan');
    planBox.hidden=false;
    planStatus.textContent='Loading…';
    planWarnings.textContent='';
  }

  function leavePlanView(){
    planMode=false;
    panel.classList.remove('plan');
    planBox.hidden=true;
    latestPlanSnapshot=null;
    planStatus.textContent='';
    planOverview.replaceChildren();
    planPrevious.textContent='';
    planItems.replaceChildren();
    planConflicts.replaceChildren();
    planResources.replaceChildren();
    planPreviews.replaceChildren();
    planWarnings.textContent='';
  }

  function formatPlanBytes(value){
    const bytes=Number(value);
    if(!Number.isFinite(bytes)||bytes<0)return '';
    if(bytes<1024)return bytes+' B';
    const units=['KiB','MiB','GiB','TiB'];
    let current=bytes/1024,unit=units[0];
    for(let i=1;i<units.length&&current>=1024;i+=1){current/=1024;unit=units[i];}
    return current.toFixed(current>=10?1:2)+' '+unit;
  }

  function appendPlanChip(label,value){
    if(value===undefined||value===null||String(value)==='')return;
    const chip=document.createElement('span');chip.className='task-patch-plan-chip';chip.textContent=`${label}: ${String(value)}`;planOverview.append(chip);
  }

  function appendPlanRow(host,name,status,detail){
    const row=document.createElement('div');row.className='task-patch-plan-row';
    const head=document.createElement('div');head.className='task-patch-plan-row-head';
    const nameNode=document.createElement('span');nameNode.className='task-patch-plan-row-name';nameNode.textContent=String(name||'');
    head.append(nameNode);
    if(status!==undefined&&status!==null&&String(status)!==''){
      const statusNode=document.createElement('span');statusNode.className='task-patch-plan-row-status';statusNode.textContent=String(status);head.append(statusNode);
    }
    row.append(head);
    if(detail){
      const detailNode=document.createElement('div');detailNode.className='task-patch-plan-row-detail';detailNode.textContent=String(detail);row.append(detailNode);
    }
    host.append(row);
  }

  function renderPlanSnapshot(snapshot){
    if(!snapshot||typeof snapshot!=='object')return false;
    latestPlanSnapshot=snapshot;
    planBox.hidden=false;
    planStatus.textContent=String(snapshot.status||'');
    planOverview.replaceChildren();
    appendPlanChip('Failure',snapshot.failure_policy);
    appendPlanChip('Transaction',snapshot.transaction_policy);
    appendPlanChip('Items',Array.isArray(snapshot.items)?snapshot.items.length:0);

    const previous=snapshot.previous_failure_action&&typeof snapshot.previous_failure_action==='object'?snapshot.previous_failure_action:null;
    planPrevious.textContent=previous
      ? ['Previous failure action: '+String(previous.action||''),String(previous.reason||'')].filter(Boolean).join(' · ')
      : '';

    planItems.replaceChildren();
    for(const item of (Array.isArray(snapshot.items)?snapshot.items:[])){
      const deps=Array.isArray(item?.depends_on)?item.depends_on:[];
      const detail=[
        item?.patch_id?('id='+String(item.patch_id)):'',
        Number.isFinite(Number(item?.target_count))?('targets='+Number(item.target_count)):'',
        deps.length?('depends='+deps.join(', ')):'',
        Number(item?.id_reuse_count)>0?('id reuse='+Number(item.id_reuse_count)):'',
        item?.package_sha256?('sha256='+String(item.package_sha256).slice(0,12)+'…'):'',
      ].filter(Boolean).join(' · ');
      appendPlanRow(planItems,`${Number(item?.index||0)}. ${String(item?.name||'')}`,'',detail);
    }
    planItemsSection.hidden=planItems.childElementCount===0;

    planConflicts.replaceChildren();
    for(const row of (Array.isArray(snapshot.static_conflicts)?snapshot.static_conflicts:[])){
      const overlap=Array.isArray(row?.overlap)?row.overlap:[];
      const detail=[
        String(row?.relation||''),
        row?.dependency_ordered===true?'dependency ordered':'',
        overlap.length?('overlap: '+overlap.join(', ')):'',
      ].filter(Boolean).join(' · ');
      appendPlanRow(planConflicts,`${String(row?.left||'')} ↔ ${String(row?.right||'')}`,'',detail);
    }
    planConflictsSection.hidden=planConflicts.childElementCount===0;

    planResources.replaceChildren();
    const resources=snapshot.resources&&typeof snapshot.resources==='object'?snapshot.resources:null;
    if(resources){
      const project=[formatPlanBytes(resources.actual_project_free_bytes),formatPlanBytes(resources.required_project_free_bytes)].filter(Boolean);
      const temp=[formatPlanBytes(resources.actual_temp_free_bytes),formatPlanBytes(resources.required_temp_free_bytes)].filter(Boolean);
      appendPlanRow(planResources,'Resource gate',resources.status,[
        project.length?('project free/required: '+project.join(' / ')):'',
        temp.length?('temp free/required: '+temp.join(' / ')):'',
      ].filter(Boolean).join(' · '));
    }
    planResourcesSection.hidden=planResources.childElementCount===0;

    planPreviews.replaceChildren();
    for(const preview of (Array.isArray(snapshot.previews)?snapshot.previews:[])){
      const detail=[
        preview?.stage?('stage='+String(preview.stage)):'',
        preview?.rc===undefined||preview?.rc===null?'':('rc='+String(preview.rc)),
        Number.isFinite(Number(preview?.target_count))?('targets='+Number(preview.target_count)):'',
        preview?.diagnosis_kind?String(preview.diagnosis_kind):'',
        preview?.message?String(preview.message):'',
      ].filter(Boolean).join(' · ');
      appendPlanRow(planPreviews,String(preview?.name||''),preview?.status,detail);
    }
    planPreviewsSection.hidden=planPreviews.childElementCount===0;

    const warnings=Array.isArray(snapshot.warnings)?snapshot.warnings:[];
    const error=snapshot.error&&typeof snapshot.error==='object'?snapshot.error:null;
    planWarnings.textContent=[
      ...warnings.map(value=>'Warning: '+String(value)),
      error?('Error: '+[error.kind,error.message].filter(Boolean).join(' · ')):'',
    ].filter(Boolean).join(' | ');
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

  function queueActionAllowed(action){
    return new Set(Array.isArray(activeQueuePrompt?.queue_actions)?activeQueuePrompt.queue_actions.map(String):[]).has(action);
  }

  function refreshActionDisabledState(){
    const busy=actionBusy||queueMutationBusy;
    for(const button of summaryList.querySelectorAll('.task-patch-item-action,.task-patch-queue-delete'))button.disabled=busy;
    for(const button of promptButtons.querySelectorAll('button'))button.disabled=busy;
    for(const input of promptItems.querySelectorAll('input[type="checkbox"]'))input.disabled=busy;
  }

  async function waitForQueueMutation(sessionId,mutationID,oldPromptID){
    const generation=++queueMutationPollGeneration;
    for(let attempt=0;attempt<240;attempt+=1){
      if(generation!==queueMutationPollGeneration||!panel.classList.contains('visible')||sessionId!==activeSessionId)return null;
      const state=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/protocol`);
      const result=state?.queue_mutation_result;
      if(result?.mutation_id===mutationID){
        if(result.status!=='PASS'){
          summaryWarnings.textContent=String(result.message||'Queue delete failed');
          return result;
        }
        if(Number(result.remaining)===0){
          const snapshot=state?.queue_snapshot;
          const items=Array.isArray(snapshot?.items)?snapshot.items:[];
          if(snapshot&&items.length===0){
            renderQueueSnapshot(snapshot);
            clearPrompt();
            summaryWarnings.textContent=String(result.message||'Queue item deleted');
            return result;
          }
        }else{
          const prompt=state?.prompt;
          if(prompt?.prompt_kind==='queue_selection'&&String(prompt?.prompt_id||'')&&String(prompt.prompt_id)!==oldPromptID&&state?.queue_snapshot){
            renderQueueSnapshot(state.queue_snapshot);
            renderQueuePrompt(sessionId,prompt);
            summaryWarnings.textContent=String(result.message||'Queue item deleted');
            return result;
          }
        }
      }
      if(state?.last_event?.type==='run_finished'){
        if(result?.mutation_id===mutationID&&result.status==='PASS'){
          if(state?.queue_snapshot)renderQueueSnapshot(state.queue_snapshot);
          clearPrompt();
          return result;
        }
        throw new Error('Patch session finished before Queue delete result arrived');
      }
      await new Promise(resolve=>setTimeout(resolve,250));
    }
    throw new Error('Timed out waiting for native Queue delete refresh');
  }

  async function submitQueueDelete(item,promptItem){
    if(actionBusy||queueMutationBusy||!activeSessionId||!activeQueuePrompt)return null;
    if(!queueActionAllowed('delete'))throw new Error('Queue delete is not advertised by the active Python prompt');
    const index=Number(promptItem?.index);
    if(!Number.isInteger(index)||index<1)throw new Error('Patch Queue delete index is unavailable');
    const name=String(item?.name||'queue item');
    if(!window.confirm(`Delete ${name} from patchs/? This cannot be undone.`))return null;
    const promptID=String(activeQueuePrompt.prompt_id||'');
    queueMutationBusy=true;
    applyQueuePromptSearch();
    renderQueueRows();
    refreshActionDisabledState();
    try{
      const response=await app.jsonFetch(`/api/sessions/${encodeURIComponent(activeSessionId)}/queue-delete`,{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({prompt_id:promptID,index}),
      });
      const mutationID=String(response?.mutation_id||'');
      if(!mutationID)throw new Error('TaskDeck did not return a mutation_id');
      return await waitForQueueMutation(activeSessionId,mutationID,promptID);
    }finally{
      queueMutationBusy=false;
      renderQueueRows();
      refreshActionDisabledState();
    }
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
    if(actionBusy||queueMutationBusy||!activeSessionId||!activeQueuePrompt)return null;
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
  function normalizeQueueSearchQuery(value){
    return String(value||'').trim().toLocaleLowerCase();
  }

  function queueItemSearchText(item){
    return String(item?.search?.text||'');
  }

  function queueItemMatchesSearch(item){
    if(!queueSearchQuery)return true;
    const text=queueItemSearchText(item);
    return Boolean(text)&&text.includes(queueSearchQuery);
  }

  function queueSearchAvailable(snapshot){
    const items=Array.isArray(snapshot?.items)?snapshot.items:[];
    return items.length===0||items.some(item=>Boolean(queueItemSearchText(item)));
  }

  function snapshotItemForPromptItem(promptItem){
    const rows=Array.isArray(latestQueueSnapshot?.items)?latestQueueSnapshot.items:[];
    const name=String(promptItem?.name||'');
    const kind=String(promptItem?.kind||'');
    const group=String(promptItem?.group||'');
    return rows.find(item=>
      String(item?.name||'')===name&&
      String(item?.kind||'')===kind&&
      (!group||!item?.group||String(item.group)===group)
    )||null;
  }

  function applyQueuePromptSearch(){
    if(!activeQueuePrompt)return;
    const promptRows=new Map((Array.isArray(activeQueuePrompt.items)?activeQueuePrompt.items:[]).map(item=>[Number(item?.index),item]));
    for(const row of promptItems.querySelectorAll('.task-patch-prompt-item[data-patch-prompt-index]')){
      const index=Number(row.dataset.patchPromptIndex);
      const promptItem=promptRows.get(index);
      const snapshotItem=snapshotItemForPromptItem(promptItem);
      row.hidden=Boolean(queueSearchQuery)&&(!snapshotItem||!queueItemMatchesSearch(snapshotItem));
      if(row.hidden){
        const input=row.querySelector('input[type="checkbox"][data-patch-index]');
        if(input)input.checked=false;
        clearPromptPriority(index);
      }
    }
  }

  function setQueueSearchQuery(value){
    queueSearchQuery=normalizeQueueSearchQuery(value);
    summarySearchInput.value=String(value||'');
    renderQueueRows();
    applyQueuePromptSearch();
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
    const groupItems=queueViewItems(snapshot,queueSummaryView);
    const items=groupItems.filter(queueItemMatchesSearch);
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
    summaryStatus.textContent=queueSearchQuery
      ? `${items.length}/${groupItems.length} match`
      : (items.length?`${items.length} item(s)`:'Empty');
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
      if(promptItem){
        const itemActions=document.createElement('div');itemActions.className='task-patch-summary-item-actions';
        if(String(item?.kind||'').toUpperCase()==='PATCH'){
          for(const action of ['inspect','preview','validate']){
            if(!actionAllowed(action))continue;
            const button=document.createElement('button');button.type='button';button.className='task-patch-item-action';button.textContent=action.charAt(0).toUpperCase()+action.slice(1);button.disabled=actionBusy||queueMutationBusy;
            button.onclick=()=>submitItemAction(item,promptItem,action).catch(app.showError);
            itemActions.append(button);
          }
        }
        if(queueActionAllowed('delete')){
          const remove=document.createElement('button');remove.type='button';remove.className='task-patch-queue-delete';remove.textContent='Delete';remove.disabled=actionBusy||queueMutationBusy;
          remove.onclick=()=>submitQueueDelete(item,promptItem).catch(app.showError);
          itemActions.append(remove);
        }
        if(itemActions.childElementCount)row.append(itemActions);
      }
      summaryList.append(row);
    }
    if(!visible.length){
      const empty=document.createElement('div');empty.className='task-patch-summary-empty';
      empty.textContent=queueSearchQuery
        ? 'No item matches search'
        : (queueSummaryView==='failed'?'No unresolved failed item':'No new queue item');
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
    const searchReady=queueSearchAvailable(latestQueueSnapshot);
    summarySearchInput.disabled=!searchReady;
    summarySearchClear.disabled=!searchReady;
    if(!searchReady){
      queueSearchQuery='';
      summarySearchInput.value='';
      summarySearchInput.placeholder='Search unavailable — update Patch runtime';
    }else{
      summarySearchInput.placeholder='Search name / id / summary / target';
    }
    renderQueueRows();
    applyQueuePromptSearch();
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
    promptTools.replaceChildren();
    promptItems.replaceChildren();
    promptButtons.replaceChildren();
    renderQueueRows();
  }

  function selectedPromptIndexes(){
    return [...promptItems.querySelectorAll('input[type="checkbox"]:checked')]
      .filter(input=>!input.closest('.task-patch-prompt-item')?.hidden)
      .map(input=>Number(input.dataset.patchIndex))
      .filter(index=>Number.isInteger(index)&&index>0);
  }

  function patchPriorityCapability(prompt){
    const raw=prompt?.constraints?.patch_priority;
    if(!raw||typeof raw!=='object'||String(raw.response_field||'')!=='priorities')return null;
    const min=Number(raw.min),max=Number(raw.max);
    if(!Number.isInteger(min)||!Number.isInteger(max)||min<0||max>9||min>max)return null;
    return {min,max};
  }

  function promptInputByIndex(index){
    return promptItems.querySelector(`input[type="checkbox"][data-patch-index="${index}"]`);
  }

  function clearPromptPriority(index){
    const select=promptItems.querySelector(`select[data-patch-priority-index="${index}"]`);
    if(select)select.value='';
  }

  function selectedPromptPriorities(){
    const rows=[];
    for(const select of promptItems.querySelectorAll('select[data-patch-priority-index]')){
      if(select.value==='')continue;
      const index=Number(select.dataset.patchPriorityIndex);
      const priority=Number(select.value);
      const input=promptInputByIndex(index);
      if(input?.checked&&!input.closest('.task-patch-prompt-item')?.hidden&&Number.isInteger(index)&&index>0&&Number.isInteger(priority)&&priority>=0&&priority<=9){
        rows.push({index,priority});
      }
    }
    return rows;
  }

  function selectAllPromptPatches(prompt){
    for(const input of promptItems.querySelectorAll('input[type="checkbox"]')){
      const kind=String(input.dataset.patchKind||'').toUpperCase();
      const hidden=Boolean(input.closest('.task-patch-prompt-item')?.hidden);
      input.checked=!hidden&&kind==='PATCH';
      clearPromptPriority(Number(input.dataset.patchIndex));
    }
    promptNote.textContent=queueSearchQuery
      ? 'All visible PATCH items selected. Priorities cleared.'
      : 'All PATCH items selected in queue order. Priorities cleared.';
  }

  function clearPromptSelection(){
    for(const input of promptItems.querySelectorAll('input[type="checkbox"]')){
      input.checked=false;
      clearPromptPriority(Number(input.dataset.patchIndex));
    }
    promptNote.textContent='Selection cleared.';
  }

  function applyPromptConstraints(changed,prompt){
    if(!changed)return;
    const changedIndex=Number(changed.dataset.patchIndex);
    if(!changed.checked){
      clearPromptPriority(changedIndex);
      return;
    }
    const constraints=prompt?.constraints&&typeof prompt.constraints==='object'?prompt.constraints:{};
    if(constraints.collect_exclusive!==true)return;
    const changedKind=String(changed.dataset.patchKind||'').toUpperCase();
    for(const input of promptItems.querySelectorAll('input[type="checkbox"]')){
      if(input===changed||!input.checked)continue;
      const kind=String(input.dataset.patchKind||'').toUpperCase();
      if(changedKind==='COLLECT'||kind==='COLLECT'){
        input.checked=false;
        clearPromptPriority(Number(input.dataset.patchIndex));
      }
    }
    if(changedKind==='COLLECT'){
      for(const select of promptItems.querySelectorAll('select[data-patch-priority-index]'))select.value='';
    }
  }

  async function submitPromptResponse(sessionId,prompt,action){
    const payload={prompt_id:String(prompt?.prompt_id||''),action};
    if(action==='select'){
      payload.indexes=selectedPromptIndexes();
      const priorities=selectedPromptPriorities();
      if(priorities.length)payload.priorities=priorities;
    }
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
    const priorityCapability=patchPriorityCapability(prompt);
    promptNote.textContent=priorityCapability
      ? 'Select work and optionally assign PATCH priority 0–9. Python validates and orders the final selection.'
      : 'Select work here, or use the terminal tab. Python validates the final selection.';
    const selectAll=document.createElement('button');selectAll.type='button';selectAll.textContent='Select all PATCH';
    selectAll.onclick=()=>selectAllPromptPatches(prompt);
    const clearAll=document.createElement('button');clearAll.type='button';clearAll.textContent='Clear selection';
    clearAll.onclick=clearPromptSelection;
    promptTools.append(selectAll,clearAll);
    for(const item of items){
      const index=Number(item?.index);
      if(!Number.isInteger(index)||index<1)continue;
      const row=document.createElement('div');row.className='task-patch-prompt-item';row.dataset.patchPromptIndex=String(index);
      const input=document.createElement('input');input.type='checkbox';input.dataset.patchIndex=String(index);input.dataset.patchKind=String(item?.kind||'');
      input.checked=initial.has(index);
      input.onchange=()=>applyPromptConstraints(input,prompt);
      const copy=document.createElement('label');copy.className='task-patch-prompt-copy';
      const name=document.createElement('span');name.className='task-patch-prompt-name';
      name.textContent=`${index}. ${String(item?.name||'')}`;
      const detail=document.createElement('span');detail.className='task-patch-prompt-detail';
      detail.textContent=[item?.group,item?.kind,item?.detail].filter(Boolean).join(' · ');
      copy.append(name,detail);
      row.append(input,copy);
      if(priorityCapability&&String(item?.kind||'').toUpperCase()==='PATCH'){
        const priorityWrap=document.createElement('label');priorityWrap.className='task-patch-prompt-priority';priorityWrap.textContent='Priority';
        const priority=document.createElement('select');priority.dataset.patchPriorityIndex=String(index);
        const none=document.createElement('option');none.value='';none.textContent='—';priority.append(none);
        for(let value=priorityCapability.min;value<=priorityCapability.max;value+=1){
          const option=document.createElement('option');option.value=String(value);option.textContent=String(value);priority.append(option);
        }
        priority.onchange=()=>{
          if(priority.value!==''){
            input.checked=true;
            applyPromptConstraints(input,prompt);
          }
        };
        priorityWrap.append(priority);
        row.append(priorityWrap);
      }
      promptItems.append(row);
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
    let haveHistorySnapshot=false;
    let havePlanSnapshot=false;
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
        if(planMode){
          planStatus.textContent='PTY fallback';
          planWarnings.textContent='Native Plan protocol state unavailable. Use Terminal evidence/fallback.';
          return;
        }
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
      if(state?.history_snapshot&&!haveHistorySnapshot){
        renderHistorySnapshot(state.history_snapshot);
        haveHistorySnapshot=true;
      }
      if(state?.history_report)renderHistoryReport(state.history_report);
      if(state?.plan_snapshot&&!havePlanSnapshot){
        renderPlanSnapshot(state.plan_snapshot);
        havePlanSnapshot=true;
      }
      if(state?.history_management_result)renderHistoryManagementResult(state.history_management_result);
      renderItemLifecycle(state?.items);
      renderProgress(state?.progress);
      if(state?.action_result)renderActionResult(state.action_result);
      renderArtifacts(state?.artifacts);
      if(expectPrompt&&state?.commands_enabled===false&&(haveSnapshot||haveResumeSnapshot||haveHistorySnapshot)){
        summaryStatus.textContent+=' · Continue in PTY';
        if(haveResumeSnapshot)resumeNote.textContent='Native Resume command channel unavailable. Continue in terminal.';
        if(haveHistorySnapshot)historyWarnings.textContent='Native History command channel unavailable. Use Terminal fallback.';
        if(sessionId===activeSessionId&&!historyMode)openTerminalEvidence();
        return;
      }
      if(expectPrompt&&state?.prompt&&renderHistoryPrompt(sessionId,state.prompt)){
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
      if(!expectPrompt&&havePlanSnapshot)return;
      if(!expectPrompt&&haveSnapshot&&!followLifecycle)return;
      if(state?.error){
        if(planMode){
          planStatus.textContent='Protocol error';
          planWarnings.textContent=String(state.error);
          return;
        }
        if(!haveSnapshot)resetSummary('Protocol error');
        summaryWarnings.textContent=String(state.error);
        return;
      }
      await new Promise(resolve=>setTimeout(resolve,delayMs));
    }
    if(planMode){
      planStatus.textContent=havePlanSnapshot?planStatus.textContent:'PTY fallback';
      if(!havePlanSnapshot)planWarnings.textContent='Native Plan snapshot unavailable or timed out. Use Terminal evidence/fallback.';
      return;
    }
    if(haveSnapshot||haveResumeSnapshot||haveHistorySnapshot){
      summaryStatus.textContent+=' · Continue in PTY';
      if(haveResumeSnapshot)resumeNote.textContent='Native Resume prompt timed out. Continue in terminal.';
      if(haveHistorySnapshot)historyWarnings.textContent='Native History prompt timed out. Use Terminal fallback.';
    }else{
      resetSummary('Snapshot timeout · Continue in PTY');
    }
    if(sessionId===activeSessionId&&!runningMode&&!historyMode)openTerminalEvidence();
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
      queueMutationPollGeneration+=1;
      queueMutationBusy=false;
      historyPollGeneration+=1;
      historyManagementPollGeneration+=1;
      historyManagementBusy=false;
      leaveRunningView();
      if(mode==='history')enterHistoryView();else leaveHistoryView();
      if(mode==='plan')enterPlanView();else leavePlanView();
      clearActionResult();
      clearResumeView();
      renderProgress(null);
      renderArtifacts([]);
      app.attachSession(meta,!['queue','resume','history','plan'].includes(mode));
      window.dispatchEvent(new CustomEvent('taskmenu:patch-session-started',{detail:{mode,meta}}));
      if(mode==='queue'||mode==='resume'||mode==='history'||mode==='plan'){
        void pollProtocol(meta.id,mode==='queue'||mode==='resume'||mode==='history',mode==='resume'||mode==='history'||mode==='plan');
      }
      return meta;
    }finally{
      if(sourceButton?.isConnected)sourceButton.disabled=false;
    }
  }

  terminalEvidence.onclick=openTerminalEvidence;
  historyTerminal.onclick=openTerminalEvidence;
  historyBack.onclick=()=>stopHistoryAndBack().catch(app.showError);
  planTerminal.onclick=openTerminalEvidence;
  planBack.onclick=leavePlanView;
  runningBack.onclick=()=>{if(runningFinished)leaveRunningView();};
  actionResultClose.onclick=clearActionResult;
  queueTab.onclick=()=>setQueueSummaryView('queue');
  failedTab.onclick=()=>setQueueSummaryView('failed');
  summarySearchInput.oninput=()=>setQueueSearchQuery(summarySearchInput.value);
  summarySearchClear.onclick=()=>{setQueueSearchQuery('');summarySearchInput.focus();};
  closeButton.onclick=close;
  globalThis.TaskMenuPatchPanel={open,close,toggle,start,enterRunningView,finishRunningView,leaveRunningView,openTerminalEvidence,renderQueueSnapshot,setQueueSummaryView,renderQueuePrompt,selectedPromptPriorities,selectAllPromptPatches,clearPromptSelection,renderResumeSnapshot,renderResumePrompt,submitResumeAction,renderHistorySnapshot,renderHistoryPrompt,renderHistoryReport,submitHistoryDetail,submitHistoryManagement,renderHistoryManagementResult,enterPlanView,leavePlanView,renderPlanSnapshot,submitItemAction,submitQueueDelete,renderActionResult,renderItemLifecycle,renderProgress,renderArtifacts,get panel(){return panel;},get visible(){return panel.classList.contains('visible');}};
  return true;
}

function safeInstallPatchPanel(){
  try{installPatchPanel();}
  catch(error){console.warn('Patch panel enhancement disabled:',error);}
}

if(app?.taskData?.workspace)safeInstallPatchPanel();
else window.addEventListener('taskmenu:tasks',safeInstallPatchPanel,{once:true});
