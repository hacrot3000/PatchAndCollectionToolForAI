const app=globalThis.TaskMenuApp;

function installPatchPanel(){
  if(!app||app.layoutProfile==='mobile'||!app.taskData?.workspace)return false;
  if(globalThis.TaskMenuPatchPanel)return true;

  const tabsHost=document.querySelector('#tabs');
  const panesHost=document.querySelector('#panes');
  if(!tabsHost||!panesHost)return false;

  const style=document.createElement('style');
  style.textContent=`
  .task-patch-panel{display:none;position:absolute;inset:0;z-index:20;width:auto;background:#11151b;border:0;box-shadow:none;flex-direction:column}
  .task-patch-tab[hidden]{display:none}
  .task-patch-panel.visible{display:flex}
  .task-patch-panel-head{height:46px;display:flex;align-items:center;gap:8px;padding:7px 14px;border-bottom:1px solid #30343b}
  .task-patch-panel-title{font-size:13px;font-weight:700;letter-spacing:.04em;flex:1}
  .task-patch-panel-close{padding:5px 9px;font-size:12px}
  .task-patch-panel-body{padding:16px 18px 24px;overflow:auto;width:100%;max-width:1600px;margin:0 auto}
  .task-patch-panel-note{font-size:12px;line-height:1.45;opacity:.72;margin:0 0 10px}
  .task-patch-summary{margin:0 0 10px;padding:8px;border:1px solid #30343b;border-radius:6px;background:#0d1015;font-size:11px}
  .task-patch-summary-head{display:flex;align-items:center;gap:6px;margin-bottom:6px}
  .task-patch-summary-title{font-weight:700;flex:1}
  .task-patch-summary-status{opacity:.7}
  .task-patch-summary-refresh{font-size:10px;padding:3px 7px}
  .task-patch-summary-tabs{display:grid;grid-template-columns:1fr 1fr;gap:5px;margin:0 0 7px}
  .task-patch-summary-tab{padding:5px 7px;font-size:11px;text-align:center}
  .task-patch-summary-search{display:flex;gap:5px;margin:0 0 7px}
  .task-patch-summary-search input{min-width:0;flex:1;padding:5px 7px;font-size:11px}
  .task-patch-summary-search button{padding:5px 7px;font-size:10px}
  .task-patch-summary-tab.active{background:#283342;border-color:#526278;color:#fff}
  .task-patch-summary-tab.has-failures{border-color:#8a414b;color:#ffadb6}
  .task-patch-summary-tab.has-failures:not(.active){background:#29161a}
  .task-patch-summary-tab.has-failures.active{background:#4a2027;border-color:#b45a66;color:#fff}
  .task-patch-summary-counts{display:flex;flex-wrap:wrap;gap:5px;margin:0 0 6px}
  .task-patch-summary-count{padding:2px 5px;border:1px solid #343a44;border-radius:999px}
  .task-patch-summary-list{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:6px}
  .task-patch-summary-item{padding:5px 6px;border-radius:4px;background:#171c23;overflow:hidden}
  .task-patch-summary-item.failed{border:1px solid #8a414b;background:#32191f;box-shadow:0 0 0 1px rgba(180,90,102,.08) inset}
  .task-patch-summary-name{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;font-weight:600}
  .task-patch-summary-item.failed .task-patch-summary-name{color:#ffd7dc;font-weight:700}
  .task-patch-summary-detail{display:block;opacity:.62;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-summary-item.failed .task-patch-summary-detail{color:#ffadb6;opacity:1}
  .task-patch-summary-failure{display:block;margin-top:3px;opacity:.82;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-summary-item.failed .task-patch-summary-failure{color:#ffc2c9;opacity:1}
  .task-patch-summary-item-actions{display:flex;gap:4px;margin-top:5px;flex-wrap:wrap}
  .task-patch-summary-item-actions button{font-size:10px;padding:3px 6px}
  .task-patch-queue-delete{border-color:#81424a;background:#3b2025;color:#ffd9dd}
  .task-patch-queue-delete:hover{background:#4a272d}
  .task-patch-summary-empty{padding:7px 6px;opacity:.62;text-align:center}
  .task-patch-summary-warning{margin-top:5px;opacity:.72}
  .task-patch-summary-warning-title{font-weight:600;margin-bottom:2px}
  .task-patch-summary-warning-line{display:flex;align-items:flex-start;gap:6px;margin-top:4px;overflow-wrap:anywhere}
  .task-patch-summary-warning-text{min-width:0;flex:1}
  .task-patch-summary-warning-delete{flex:0 0 auto;font-size:10px;padding:2px 6px;border-color:#81424a;background:#3b2025;color:#ffd9dd}
  .task-patch-summary.prompt-active .task-patch-summary-tabs,
  .task-patch-summary.prompt-active .task-patch-summary-search,
  .task-patch-summary.prompt-active .task-patch-summary-list{display:none}
  .task-patch-summary.prompt-active .task-patch-summary-counts{margin-bottom:0}
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
  .task-patch-prompt-items{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:6px;max-height:46vh;overflow:auto}
  .task-patch-prompt-item{display:flex;align-items:flex-start;gap:7px;padding:5px 6px;border-radius:4px;background:#171f2a}
  .task-patch-prompt-item.failed{border:1px solid #8a414b;background:#32191f;box-shadow:0 0 0 1px rgba(180,90,102,.08) inset}
  .task-patch-prompt-item.failed .task-patch-prompt-name{color:#ffd7dc;font-weight:700}
  .task-patch-prompt-item.failed .task-patch-prompt-detail{color:#ffadb6;opacity:1}
  .task-patch-prompt-item.skipped{border:1px solid #7b6736;background:#2a2415}
  .task-patch-prompt-item.skipped .task-patch-prompt-name{color:#ffe4a0;font-weight:700}
  .task-patch-prompt-item.skipped .task-patch-prompt-detail{color:#e6c46e;opacity:1}
  .task-patch-prompt-item input{margin-top:2px}
  .task-patch-prompt-copy{min-width:0;flex:1;cursor:pointer}
  .task-patch-prompt-priority{display:flex;align-items:center;gap:4px;font-size:10px;white-space:nowrap}
  .task-patch-prompt-priority select{min-width:44px;padding:2px 3px;font-size:10px}
  .task-patch-prompt-name{display:block;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-prompt-detail{display:block;opacity:.62;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-prompt-buttons{display:flex;gap:6px;margin-top:8px}
  .task-patch-prompt-delete{margin-left:auto;border-color:#81424a;background:#3b2025;color:#ffd9dd;white-space:nowrap}
  .task-patch-prompt-delete:hover{background:#4a272d}
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
  .task-patch-history-cleanup{display:flex;align-items:center;gap:6px;margin:0 0 8px;padding:6px;border:1px solid #554c3d;border-radius:5px;background:#17140f}
  .task-patch-history-cleanup[hidden]{display:none}
  .task-patch-history-cleanup-summary{min-width:0;flex:1;opacity:.78;overflow-wrap:anywhere}
  .task-patch-history-cleanup button{font-size:10px;padding:3px 6px;border-color:#806a43}
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
  .task-patch-history-variants{margin-top:5px}
  .task-patch-history-variant{display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:6px;align-items:center;padding-top:4px;border-top:1px solid #29303a}
  .task-patch-history-format{font-weight:700;min-width:54px}
  .task-patch-history-file-actions{display:flex;gap:5px;flex-wrap:wrap;justify-content:flex-end}
  .task-patch-history-file-actions a,.task-patch-history-file-actions button{font-size:10px;padding:3px 6px}
  .task-patch-history-item-head{display:flex;gap:5px;align-items:flex-start}
  .task-patch-history-item-name{font-weight:600;min-width:0;flex:1;overflow-wrap:anywhere}
  .task-patch-history-item-status{font-weight:700;white-space:nowrap}
  .task-patch-history-item-detail{margin-top:2px;opacity:.7;overflow-wrap:anywhere}
  .task-patch-history-item-actions{display:flex;gap:5px;margin-top:5px}
  .task-patch-history-item-actions button{font-size:10px;padding:3px 6px}
  .task-patch-history-support{border-color:#4d6785}
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
  .task-patch-health{margin:0 0 10px;padding:8px;border:1px solid #4b596d;border-radius:6px;background:#121923;font-size:11px}
  .task-patch-health[hidden]{display:none}
  .task-patch-health-head{display:flex;align-items:center;gap:5px;margin-bottom:7px}
  .task-patch-health-title{font-weight:700;flex:1}
  .task-patch-health-status{opacity:.72}
  .task-patch-health-head button{font-size:10px;padding:3px 6px}
  .task-patch-health-overview{display:flex;flex-wrap:wrap;gap:5px;margin-bottom:7px}
  .task-patch-health-chip{padding:2px 5px;border:1px solid #3b4655;border-radius:999px}
  .task-patch-health-checks{display:grid;gap:4px}
  .task-patch-health-row{padding:6px;border-radius:4px;background:#171f2a}
  .task-patch-health-row-head{display:flex;align-items:flex-start;gap:5px}
  .task-patch-health-row-name{font-weight:600;min-width:0;flex:1;overflow-wrap:anywhere}
  .task-patch-health-row-status{font-weight:700;white-space:nowrap}
  .task-patch-health-row-detail{margin-top:2px;opacity:.7;overflow-wrap:anywhere}
  .task-patch-health-messages{margin-top:7px;opacity:.78;overflow-wrap:anywhere}
  .task-patch-panel.plan .task-patch-health{display:none!important}
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
  .task-patch-panel.health .task-patch-panel-note,
  .task-patch-panel.health .task-patch-summary,
  .task-patch-panel.health .task-patch-action-result,
  .task-patch-panel.health .task-patch-prompt,
  .task-patch-panel.health .task-patch-resume,
  .task-patch-panel.health .task-patch-history,
  .task-patch-panel.health .task-patch-plan,
  .task-patch-panel.health .task-patch-running-head,
  .task-patch-panel.health .task-patch-run,
  .task-patch-panel.health .task-patch-artifacts,
  .task-patch-panel.health .task-patch-actions{display:none!important}
  .task-patch-panel.history .task-patch-health{display:none!important}
  .task-patch-panel.history .task-patch-panel-note,
  .task-patch-panel.history .task-patch-summary,
  .task-patch-panel.history .task-patch-action-result,
  .task-patch-panel.history .task-patch-prompt,
  .task-patch-panel.history .task-patch-resume,
  .task-patch-panel.history .task-patch-running-head,
  .task-patch-panel.history .task-patch-run,
  .task-patch-panel.history .task-patch-artifacts,
  .task-patch-panel.history .task-patch-actions{display:none!important}
  .task-patch-running-head{display:none;margin:0 0 8px;padding:10px;border:1px solid #647996;border-radius:6px;background:#121923;font-size:11px;box-shadow:0 0 0 1px rgba(120,151,191,.08) inset}
  .task-patch-panel.running .task-patch-running-head{display:block}
  .task-patch-running-head.finished{border-color:#5e8668}
  .task-patch-running-title{font-weight:700;font-size:12px;overflow-wrap:anywhere}
  .task-patch-running-meta{margin-top:3px;opacity:.72}
  .task-patch-running-meta.stale{color:#e4be63;opacity:1}
  .task-patch-running-actions{display:flex;gap:6px;margin-top:8px}
  .task-patch-running-actions button{flex:1}
  .task-patch-panel.running .task-patch-panel-note,
  .task-patch-panel.running .task-patch-summary,
  .task-patch-panel.running .task-patch-action-result,
  .task-patch-panel.running .task-patch-prompt,
  .task-patch-panel.running .task-patch-resume,
  .task-patch-panel.running .task-patch-actions{display:none!important}
  .task-patch-parallel{margin:0 0 10px;padding:8px;border:1px solid #30343b;border-radius:6px;background:#0d1015;font-size:11px}
  .task-patch-parallel[hidden]{display:none}
  .task-patch-parallel-title{font-weight:700;margin-bottom:6px}
  .task-patch-parallel-list{display:grid;gap:7px}
  .task-patch-parallel-run{padding:7px;border-radius:5px;background:#171c23}
  .task-patch-parallel-run-head{display:flex;align-items:flex-start;gap:7px}
  .task-patch-parallel-run-name{min-width:0;flex:1;font-weight:600;overflow-wrap:anywhere}
  .task-patch-parallel-run-status{font-weight:700;white-space:nowrap}
  .task-patch-parallel-run-meta{margin-top:3px;opacity:.72;overflow-wrap:anywhere}
  .task-patch-parallel-run-meta.stale{color:#e4be63;opacity:1}
  .task-patch-parallel-run-actions{display:flex;gap:5px;margin-top:5px}
  .task-patch-parallel-run-actions button{font-size:10px;padding:3px 6px}
  .task-patch-parallel-artifacts{display:grid;gap:5px;margin-top:6px}
  .task-patch-run{margin:0 0 10px;padding:8px;border:1px solid #30343b;border-radius:6px;background:#0d1015;font-size:11px}
  .task-patch-run[hidden]{display:none}
  .task-patch-run-title{font-weight:700;margin-bottom:6px}
  .task-patch-run-items{display:grid;gap:4px}
  .task-patch-run-item{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:6px;padding:5px 6px;border-radius:4px;background:#171c23}
  .task-patch-run-item.failed{border:1px solid #8a414b;background:#2d171c}
  .task-patch-run-name{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-run-status{font-weight:700}
  .task-patch-run-item.failed .task-patch-run-status{color:#ff9da8}
  .task-patch-run-failure{grid-column:1/-1;display:grid;gap:5px;padding-top:5px;border-top:1px solid #613139}
  .task-patch-run-failure-reason{font-weight:600;color:#ffd5da;overflow-wrap:anywhere}
  .task-patch-run-failure-actions{display:flex;gap:5px;align-items:center;flex-wrap:wrap}
  .task-patch-run-failure-actions button{font-size:10px;padding:3px 6px}
  .task-patch-run-failure-console{grid-column:1/-1}
  .task-patch-run-failure-console summary{cursor:pointer;opacity:.78}
  .task-patch-run-failure-console pre{margin:5px 0 0;max-height:220px;overflow:auto;white-space:pre-wrap;word-break:break-word;background:#120c0e;border:1px solid #4d2b31;border-radius:4px;padding:6px;font:10px/1.45 ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace}
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
  .task-patch-artifact-variants{margin-top:5px}
  .task-patch-artifact-variant{display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:6px;align-items:center;padding-top:4px;border-top:1px solid #29303a}
  .task-patch-artifact-format{font-weight:700;min-width:54px}
  .task-patch-artifact-variant-path{min-width:0;opacity:.7;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .task-patch-artifact-actions{display:flex;gap:5px;flex-wrap:wrap;justify-content:flex-end}
  .task-patch-artifact-actions a,.task-patch-artifact-actions button{font-size:10px;padding:3px 6px}
  .task-patch-actions{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:10px}
  .task-patch-action{display:flex;flex-direction:column;align-items:flex-start;gap:3px;width:100%;min-height:70px;padding:11px 12px;text-align:left}
  .task-patch-action strong{font-size:12px}
  .task-patch-action span{font-size:11px;opacity:.65}
  html[data-taskmenu-theme="light"] .task-patch-panel{background:#fff;border-color:#b9c0c8;box-shadow:10px 0 28px rgba(0,0,0,.12)}
  html[data-taskmenu-theme="light"] .task-patch-panel-head{border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-summary{background:#f6f8fa;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-summary-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-summary-item.failed{background:#fff1f2;border-color:#c47780}
  html[data-taskmenu-theme="light"] .task-patch-summary-item.failed .task-patch-summary-name{color:#7b2029}
  html[data-taskmenu-theme="light"] .task-patch-summary-item.failed .task-patch-summary-detail,
  html[data-taskmenu-theme="light"] .task-patch-summary-item.failed .task-patch-summary-failure{color:#9d2632}
  html[data-taskmenu-theme="light"] .task-patch-queue-delete{background:#fff0f1;border-color:#c47780;color:#7b2029}
  html[data-taskmenu-theme="light"] .task-patch-queue-delete:hover{background:#ffe5e7}
  html[data-taskmenu-theme="light"] .task-patch-summary-count{border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-summary-tab.active{background:#e7eef7;border-color:#9aa9bc;color:#1f2328}
  html[data-taskmenu-theme="light"] .task-patch-summary-tab.has-failures{border-color:#c47780;color:#9d2632}
  html[data-taskmenu-theme="light"] .task-patch-summary-tab.has-failures:not(.active){background:#fff1f2}
  html[data-taskmenu-theme="light"] .task-patch-summary-tab.has-failures.active{background:#ffe1e4;border-color:#b85b66;color:#7b2029}
  html[data-taskmenu-theme="light"] .task-patch-action-result{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-action-result-output{background:#fff;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-running-head{background:#f6f8fa;border-color:#9aa9bc}
  html[data-taskmenu-theme="light"] .task-patch-running-head.finished{border-color:#6f9a78}
  html[data-taskmenu-theme="light"] .task-patch-prompt{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-prompt-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-prompt-item.failed{background:#fff1f2;border-color:#c47780}
  html[data-taskmenu-theme="light"] .task-patch-prompt-item.failed .task-patch-prompt-name{color:#7b2029}
  html[data-taskmenu-theme="light"] .task-patch-prompt-item.failed .task-patch-prompt-detail{color:#9d2632}
  html[data-taskmenu-theme="light"] .task-patch-prompt-item.skipped{background:#fff9e8;border-color:#c8aa58}
  html[data-taskmenu-theme="light"] .task-patch-prompt-item.skipped .task-patch-prompt-name{color:#72580b}
  html[data-taskmenu-theme="light"] .task-patch-prompt-item.skipped .task-patch-prompt-detail{color:#8b6d13}
  html[data-taskmenu-theme="light"] .task-patch-resume{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-resume-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-resume-count{border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-plan{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-plan-row{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-plan-chip{border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-health{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-health-row{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-health-chip{border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-history{background:#f6f8fa;border-color:#b9c0c8}
  html[data-taskmenu-theme="light"] .task-patch-history-run,
  html[data-taskmenu-theme="light"] .task-patch-history-file,
  html[data-taskmenu-theme="light"] .task-patch-history-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-history-run.active{background:#e7eef7;border-color:#9aa9bc}
  html[data-taskmenu-theme="light"] .task-patch-history-delete{background:#fff0f1;border-color:#c47780;color:#7b2029}
  html[data-taskmenu-theme="light"] .task-patch-history-delete:hover{background:#ffe5e7}
  html[data-taskmenu-theme="light"] .task-patch-history-management{background:#fff;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-history-cleanup{background:#fffaf0;border-color:#d8c69c}
  html[data-taskmenu-theme="light"] .task-patch-history-detail{border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-parallel{background:#f6f8fa;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-parallel-run{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-run{background:#f6f8fa;border-color:#d0d7de}
  html[data-taskmenu-theme="light"] .task-patch-run-item{background:#fff}
  html[data-taskmenu-theme="light"] .task-patch-run-item.failed{background:#fff1f2;border-color:#c47780}
  html[data-taskmenu-theme="light"] .task-patch-run-item.failed .task-patch-run-status{color:#9d2632}
  html[data-taskmenu-theme="light"] .task-patch-run-failure{border-color:#e0a8ae}
  html[data-taskmenu-theme="light"] .task-patch-run-failure-reason{color:#7b2029}
  html[data-taskmenu-theme="light"] .task-patch-run-failure-console pre{background:#fff;border-color:#e0a8ae}
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
  const summaryRefresh=document.createElement('button');summaryRefresh.type='button';summaryRefresh.className='task-patch-summary-refresh';summaryRefresh.textContent='Refresh Queue';summaryRefresh.title='Reload PATCH/COLLECT queue, including when it is empty';
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
  summaryHead.append(summaryTitle,summaryStatus,summaryRefresh);
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
  const historyCleanup=document.createElement('div');historyCleanup.className='task-patch-history-cleanup';historyCleanup.hidden=true;
  const historyCleanupSummary=document.createElement('div');historyCleanupSummary.className='task-patch-history-cleanup-summary';
  const historyCleanupButton=document.createElement('button');historyCleanupButton.type='button';historyCleanupButton.textContent='Cleanup';
  historyCleanup.append(historyCleanupSummary,historyCleanupButton);
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
  historyBox.append(historyHead,historyCleanup,historyRuns,historyManagement,historyDetail);

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

  const healthBox=document.createElement('div');healthBox.className='task-patch-health';healthBox.hidden=true;
  const healthHead=document.createElement('div');healthHead.className='task-patch-health-head';
  const healthTitle=document.createElement('div');healthTitle.className='task-patch-health-title';healthTitle.textContent='Tool Health';
  const healthStatus=document.createElement('div');healthStatus.className='task-patch-health-status';
  const healthTerminal=document.createElement('button');healthTerminal.type='button';healthTerminal.textContent='Terminal';healthTerminal.title='Open terminal evidence/fallback';
  const healthBack=document.createElement('button');healthBack.type='button';healthBack.textContent='Back';
  healthHead.append(healthTitle,healthStatus,healthTerminal,healthBack);
  const healthOverview=document.createElement('div');healthOverview.className='task-patch-health-overview';
  const healthChecks=document.createElement('div');healthChecks.className='task-patch-health-checks';
  const healthMessages=document.createElement('div');healthMessages.className='task-patch-health-messages';
  healthBox.append(healthHead,healthOverview,healthChecks,healthMessages);

  const runningHead=document.createElement('div');runningHead.className='task-patch-running-head';
  const runningTitle=document.createElement('div');runningTitle.className='task-patch-running-title';runningTitle.textContent='Running';
  const runningMeta=document.createElement('div');runningMeta.className='task-patch-running-meta';runningMeta.textContent='Waiting for Python execution state…';
  const runningActions=document.createElement('div');runningActions.className='task-patch-running-actions';
  const terminalEvidence=document.createElement('button');terminalEvidence.type='button';terminalEvidence.textContent='Open terminal evidence';
  const runningBack=document.createElement('button');runningBack.type='button';runningBack.textContent='Back to Queue';runningBack.hidden=true;
  runningActions.append(terminalEvidence,runningBack);
  runningHead.append(runningTitle,runningMeta,runningActions);

  const parallelCollectBox=document.createElement('div');parallelCollectBox.className='task-patch-parallel';parallelCollectBox.hidden=true;
  const parallelCollectTitle=document.createElement('div');parallelCollectTitle.className='task-patch-parallel-title';parallelCollectTitle.textContent='Other active runs';
  const parallelCollectList=document.createElement('div');parallelCollectList.className='task-patch-parallel-list';
  parallelCollectBox.append(parallelCollectTitle,parallelCollectList);

  const runBox=document.createElement('div');runBox.className='task-patch-run';runBox.hidden=true;
  const runTitle=document.createElement('div');runTitle.className='task-patch-run-title';runTitle.textContent='Current run status';
  const progressNode=document.createElement('div');progressNode.className='task-patch-progress';progressNode.hidden=true;
  const progressHead=document.createElement('div');progressHead.className='task-patch-progress-head';
  const progressDetail=document.createElement('span');progressDetail.className='task-patch-progress-detail';
  progressNode.append(progressHead,progressDetail);
  const runItems=document.createElement('div');runItems.className='task-patch-run-items';
  runBox.append(runTitle,progressNode,runItems);

  const artifactBox=document.createElement('div');artifactBox.className='task-patch-artifacts';artifactBox.hidden=true;
  const artifactTitle=document.createElement('div');artifactTitle.className='task-patch-artifacts-title';artifactTitle.textContent='Current run artifacts';
  const artifactList=document.createElement('div');artifactList.className='task-patch-artifact-list';
  artifactBox.append(artifactTitle,artifactList);

  const actions=document.createElement('div');actions.className='task-patch-actions';
  body.append(note,summary,actionResultBox,promptBox,resumeBox,historyBox,planBox,healthBox,runningHead,runBox,artifactBox,parallelCollectBox,actions);
  panel.append(head,body);
  panesHost.append(panel);

  const patchTab=document.createElement('button');patchTab.type='button';patchTab.className='tab task-patch-tab';patchTab.hidden=true;
  const patchTabLabel=document.createElement('span');patchTabLabel.textContent='Patch Tool';
  const patchTabClose=document.createElement('span');patchTabClose.className='close';patchTabClose.textContent='×';patchTabClose.title='Close Patch Tool';
  patchTab.append(patchTabLabel,patchTabClose);tabsHost.append(patchTab);

  function patchUIMode(){return globalThis.TaskMenuPatchUISettings?.mode==='terminal'?'terminal':'native';}

  const actionDefs=[
    ['queue','Queue','Open the normal PATCH/COLLECT queue'],
    ['resume','Resume','Continue an interrupted or failed run'],
    ['history','History','Open Patch Tool reports/history'],
    ['plan','Plan','Inspect the execution plan without replacing the Python engine'],
    ['health','Health','Audit the active bundled Patch Tool runtime'],
  ];
  const buttons=[];
  let activeSessionId='';
  let returnViewId='';
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
  let latestHistoryReport=null;
  let activeHistoryPrompt=null;
  let activeHistoryRunID='';
  let historyBusy=false;
  let historyManagementBusy=false;
  let historyManagementPollGeneration=0;
  let historySupportBusy=false;
  let historySupportPollGeneration=0;
  let historyCleanupBusy=false;
  let historyCleanupPollGeneration=0;
  let historyMode=false;
  let latestPlanSnapshot=null;
  let planMode=false;
  let latestHealthSnapshot=null;
  let healthMode=false;
  let runningMode=false;
  let runningFinished=false;
  let runningStartedAtMs=0;
  let runningLastEventCount=-1;
  let runningLastEventAtMs=0;
  let parallelCollectBusy=false;
  let parallelCollectPollGeneration=0;
  const parallelCollectRuns=new Map();
  let foregroundRunDescriptor=null;
  let foregroundProtocolState=null;
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
    patchTab.classList.toggle('active',visible);
    if(!visible){protocolPollGeneration+=1;actionPollGeneration+=1;queueMutationPollGeneration+=1;historyPollGeneration+=1;historyManagementPollGeneration+=1;historySupportPollGeneration+=1;historyCleanupPollGeneration+=1;parallelCollectPollGeneration+=1;}
    if(visible&&activeSessionId)void pollProtocol(activeSessionId,!runningMode&&!planMode&&!healthMode,true);
    if(visible&&parallelCollectRuns.size)void pollParallelCollectRuns();
    window.dispatchEvent(new CustomEvent('taskmenu:patch-panel-visible',{detail:{visible}}));
  }
  function rememberReturnView(){
    const current=String(app.active||'');
    if(current&&current!=='external:patch')returnViewId=current;
  }
  function restoreReturnView(){
    const target=String(returnViewId||'');
    if(target&&app.views.has(target)){app.activateView(target);return;}
    if(target.startsWith('external:')){app.activateExternalView(target.slice('external:'.length));return;}
    const first=app.views.keys().next();
    if(!first.done)app.activateView(first.value);
  }
  function open(){
    if(patchUIMode()==='terminal'){
      start('queue').catch(app.showError);
      return;
    }
    if(!panel.classList.contains('visible'))rememberReturnView();
    patchTab.hidden=false;
    app.activateExternalView('patch');
    setVisible(true);
  }
  function deactivate(){
    if(panel.classList.contains('visible'))setVisible(false);
  }
  function close(){
    const wasVisible=panel.classList.contains('visible');
    setVisible(false);
    patchTab.hidden=true;
    if(wasVisible||String(app.active||'')==='external:patch')restoreReturnView();
  }
  function toggle(){panel.classList.contains('visible')?close():open();}
  patchTab.onclick=()=>open();
  patchTabClose.onclick=event=>{event.stopPropagation();close();};
  window.addEventListener('taskmenu:patch-ui-mode',event=>{
    if(String(event.detail?.mode||'')==='terminal'){
      setVisible(false);
      patchTab.hidden=true;
      restoreReturnView();
    }
  });
  window.addEventListener('taskmenu:view-activated',event=>{
    const kind=String(event.detail?.kind||'');
    const id=String(event.detail?.id||'');
    if(kind==='external'&&id==='patch'){patchTab.hidden=false;setVisible(true);return;}
    if(kind==='terminal'||kind==='external')deactivate();
  });

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
        await start('history');
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
    latestHistoryReport=null;
    activeHistoryPrompt=null;
    activeHistoryRunID='';
    historyBusy=false;
    historyManagementBusy=false;
    historySupportBusy=false;
    historyCleanupBusy=false;
    historyPollGeneration+=1;
    historyManagementPollGeneration+=1;
    historySupportPollGeneration+=1;
    historyCleanupPollGeneration+=1;
    historyCleanup.hidden=true;
    historyCleanupSummary.textContent='';
    historyCleanupButton.disabled=false;
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

  function fullProjectPath(path){
    const root=String(app.taskData?.workspace||'').replace(/[\\/]+$/,'');
    const rel=String(path||'').replace(/^\/+/, '');
    return root&&rel?root+'/'+rel:rel;
  }

  async function copyFullPath(path,button){
    const text=fullProjectPath(path);
    if(!text)return false;
    let copied=false;
    if(navigator.clipboard&&window.isSecureContext){
      try{await navigator.clipboard.writeText(text);copied=true;}catch{}
    }
    if(!copied){
      const area=document.createElement('textarea');area.value=text;area.setAttribute('readonly','');area.style.position='fixed';area.style.left='-9999px';area.style.top='0';
      document.body.append(area);area.select();
      try{copied=document.execCommand('copy');}finally{area.remove();}
    }
    if(!copied)throw new Error('Cannot copy full path');
    if(button){
      const original=button.textContent;button.textContent='✓ Copied';
      setTimeout(()=>{if(button.isConnected)button.textContent=original;},1200);
    }
    return true;
  }

  function openProjectFile(path){
    const editor=globalThis.TaskMenuEditor;
    if(editor?.openFile){
      editor.openFile(path).catch(app.showError);
      return;
    }
    window.dispatchEvent(new CustomEvent('taskmenu:project-file-open-request',{detail:{path}}));
  }

  function historyArtifactFormat(path){
    if(/\.zip$/i.test(String(path||'')))return 'ZIP';
    if(historyTextFile(path))return 'TXT';
    return 'FILE';
  }

  function historyArtifactGroupLabel(files){
    const labels=files.map(file=>String(file?.label||''));
    if(labels.some(value=>/^FAIL handoff/i.test(value)))return 'FAIL handoff';
    if(labels.some(value=>/^COLLECT (?:result|text|ZIP)/i.test(value)))return 'COLLECT result';
    if(labels.some(value=>/^AI sync/i.test(value)))return 'AI sync';
    const first=labels.find(Boolean)||'File';
    return first.replace(/\s+(?:TXT|ZIP|text)$/i,'');
  }

  function groupHistoryFiles(files){
    const groups=new Map();
    for(const file of (Array.isArray(files)?files:[])){
      const path=historySafeProjectPath(file?.path);
      if(!path)continue;
      const format=historyArtifactFormat(path);
      const stem=(format==='ZIP'||format==='TXT')?path.replace(/\.(?:zip|txt)$/i,''):path;
      let group=groups.get(stem);
      if(!group){group={files:[],upload:false};groups.set(stem,group);}
      group.files.push({...file,path,format});group.upload=group.upload||file?.upload_required===true;
    }
    return [...groups.values()];
  }

  function appendHistoryFiles(host,files){
    for(const group of groupHistoryFiles(files)){
      const row=document.createElement('div');row.className='task-patch-history-file';
      const head=document.createElement('div');head.className='task-patch-history-file-head';
      const label=document.createElement('span');label.className='task-patch-history-file-label';label.textContent=historyArtifactGroupLabel(group.files);head.append(label);
      if(group.upload){const badge=document.createElement('span');badge.className='task-patch-history-upload';badge.textContent='UPLOAD';head.append(badge);}
      const variants=document.createElement('div');variants.className='task-patch-history-variants';
      const filesSorted=group.files.slice().sort((a,b)=>String(a.format).localeCompare(String(b.format)));
      const line=document.createElement('div');line.className='task-patch-history-variant';
      const format=document.createElement('span');format.className='task-patch-history-format';format.textContent=filesSorted.map(file=>String(file.format||'FILE')).join(' / ');
      const displayPath=pairedVariantPath(filesSorted);
      const pathNode=document.createElement('span');pathNode.className='task-patch-history-path';pathNode.textContent=displayPath;pathNode.title=filesSorted.map(file=>String(file.path||'')).join('\n');
      const actionsNode=document.createElement('div');actionsNode.className='task-patch-history-file-actions';
      for(const file of filesSorted){
        const download=document.createElement('a');download.textContent=file.format+' Download';download.href='/api/files/download?path='+encodeURIComponent(file.path);download.download='';actionsNode.append(download);
      }
      for(const file of filesSorted){
        const copyPath=document.createElement('button');copyPath.type='button';copyPath.textContent=file.format+' Copy path';copyPath.title='Copy full project path';copyPath.onclick=()=>copyFullPath(file.path,copyPath).catch(app.showError);actionsNode.append(copyPath);
      }
      const textFile=filesSorted.find(file=>file.format==='TXT');
      if(textFile){const open=document.createElement('button');open.type='button';open.textContent='Open TXT';open.onclick=()=>openProjectFile(textFile.path);actionsNode.append(open);}
      line.append(format,pathNode,actionsNode);variants.append(line);
      row.append(head,variants);host.append(row);
    }
  }

  function appendHistoryFile(host,file){appendHistoryFiles(host,[file]);}

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

  function renderHistorySupportResult(result){
    if(!result||typeof result!=='object')return false;
    historyManagementMessage.textContent=[
      String(result.message||'History support result'),
      String(result.status||''),
      result.item_name?('item='+String(result.item_name)):'',
    ].filter(Boolean).join(' · ');
    historyManagementFiles.replaceChildren();
    if(result.artifact&&typeof result.artifact==='object')appendHistoryFile(historyManagementFiles,result.artifact);
    historyManagement.hidden=false;
    return true;
  }

  function historyItemSupportAllowed(item){
    const constraints=activeHistoryPrompt?.constraints&&typeof activeHistoryPrompt.constraints==='object'?activeHistoryPrompt.constraints:{};
    const advertised=new Set(Array.isArray(constraints.item_actions)?constraints.item_actions.map(value=>String(value).toLowerCase()):[]);
    const actions=new Set(Array.isArray(item?.actions)?item.actions.map(value=>String(value).toLowerCase()):[]);
    return advertised.has('support')&&actions.has('support');
  }

  async function waitForHistorySupport(sessionId,supportID,promptID,runID,itemIndex){
    const generation=++historySupportPollGeneration;
    for(let attempt=0;attempt<1200;attempt+=1){
      if(generation!==historySupportPollGeneration||!panel.classList.contains('visible')||sessionId!==activeSessionId)return null;
      const state=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/protocol`);
      const result=state?.history_support_result;
      if(result?.support_id===supportID){
        if(String(result?.prompt_id||'')!==promptID||
           String(result?.run_id||'')!==runID||
           Number(result?.item_index)!==itemIndex){
          throw new Error('Patch History support result correlation mismatch');
        }
        renderHistorySupportResult(result);
        return result;
      }
      if(state?.last_event?.type==='run_finished')throw new Error('Patch History session finished before support result arrived');
      await new Promise(resolve=>setTimeout(resolve,250));
    }
    throw new Error('Timed out waiting for native Patch History support ZIP');
  }

  async function submitHistorySupport(sessionId,prompt,runID,item,sourceButton){
    if(historyBusy||historyManagementBusy||historySupportBusy||historyCleanupBusy||!sessionId||!prompt)return null;
    if(prompt?.prompt_kind!=='history_action'||String(prompt?.prompt_id||'')==='')throw new Error('Native History support requires the active History prompt');
    if(String(runID||'')!==activeHistoryRunID)throw new Error('History support requires the currently displayed report');
    if(!historyItemSupportAllowed(item))throw new Error('History Support is not advertised for this item');
    const itemIndex=Number(item?.index);
    if(!Number.isInteger(itemIndex)||itemIndex<1)throw new Error('History support item index is unavailable');
    const promptID=String(prompt.prompt_id||'');
    historySupportBusy=true;
    if(sourceButton?.isConnected)sourceButton.disabled=true;
    historyManagement.hidden=false;
    historyManagementFiles.replaceChildren();
    historyManagementMessage.textContent=`Creating support ZIP for ${String(item?.name||'item')}…`;
    try{
      const response=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/history-support`,{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({prompt_id:promptID,run_id:runID,item_index:itemIndex}),
      });
      const supportID=String(response?.support_id||'');
      if(!supportID)throw new Error('TaskDeck did not return a support_id');
      return await waitForHistorySupport(sessionId,supportID,promptID,runID,itemIndex);
    }finally{
      historySupportBusy=false;
      if(sourceButton?.isConnected)sourceButton.disabled=false;
      if(latestHistoryReport)renderHistoryReport(latestHistoryReport);
    }
  }

  function historyPromptRun(runID){
    const runs=Array.isArray(activeHistoryPrompt?.runs)?activeHistoryPrompt.runs:[];
    return runs.find(row=>String(row?.run_id||'')===String(runID||''))||null;
  }

  function historyCleanupProjection(prompt){
    if(prompt?.type!=='prompt'||prompt?.prompt_kind!=='history_action'||!prompt?.prompt_id)return null;
    const actions=new Set(Array.isArray(prompt.actions)?prompt.actions.map(value=>String(value).toLowerCase()):[]);
    const constraints=prompt?.constraints&&typeof prompt.constraints==='object'?prompt.constraints:{};
    const destructive=new Set(Array.isArray(constraints.destructive_actions)?constraints.destructive_actions.map(value=>String(value).toLowerCase()):[]);
    const cleanup=constraints.cleanup&&typeof constraints.cleanup==='object'?constraints.cleanup:null;
    if(!actions.has('cleanup')||!destructive.has('cleanup')||!cleanup)return null;
    if(String(cleanup.policy||'')!=='remove_unpinned_idle_then_oldest_unpinned_over_limit')return null;
    const fields=['eligible','idle_eligible','overflow_eligible','pinned','meaningful','limit'];
    const values={};
    for(const field of fields){
      const value=Number(cleanup[field]);
      if(!Number.isInteger(value)||value<0)return null;
      values[field]=value;
    }
    return {...values,policy:String(cleanup.policy)};
  }

  function renderHistoryCleanupCapability(prompt){
    const cleanup=historyCleanupProjection(prompt);
    if(!cleanup){
      historyCleanup.hidden=true;
      historyCleanupSummary.textContent='';
      historyCleanupButton.disabled=true;
      return false;
    }
    historyCleanup.hidden=false;
    historyCleanupSummary.textContent=[
      `${cleanup.eligible} eligible`,
      `${cleanup.idle_eligible} IDLE`,
      `${cleanup.overflow_eligible} over limit`,
      `${cleanup.pinned} pinned`,
      `keep ${cleanup.limit} meaningful`,
    ].join(' · ');
    historyCleanupButton.textContent=cleanup.eligible>0?`Cleanup ${cleanup.eligible}`:'Nothing eligible';
    historyCleanupButton.disabled=cleanup.eligible===0||historyBusy||historyManagementBusy||historySupportBusy||historyCleanupBusy;
    return true;
  }

  function renderHistoryCleanupResult(result){
    if(!result||typeof result!=='object')return false;
    historyManagementMessage.textContent=[
      String(result.message||'History cleanup result'),
      `removed=${Number(result.removed||0)}`,
      `pinned=${Number(result.pinned||0)}`,
      `remaining=${Number(result.remaining||0)}`,
    ].join(' · ');
    historyManagementFiles.replaceChildren();
    historyManagement.hidden=false;
    return true;
  }

  async function waitForHistoryCleanup(sessionId,cleanupID,promptID,snapshotBefore){
    const generation=++historyCleanupPollGeneration;
    let matched=null;
    for(let attempt=0;attempt<240;attempt+=1){
      if(generation!==historyCleanupPollGeneration||!panel.classList.contains('visible')||sessionId!==activeSessionId)return null;
      const state=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/protocol`);
      const result=state?.history_cleanup_result;
      if(result?.cleanup_id===cleanupID){
        if(String(result?.prompt_id||'')!==promptID)throw new Error('Patch History cleanup result correlation mismatch');
        matched=result;
        renderHistoryCleanupResult(result);
        if(result?.history_changed!==true)return result;
      }
      if(matched?.history_changed===true&&state?.history_snapshot){
        const snapshotToken=JSON.stringify(state.history_snapshot);
        if(snapshotToken!==snapshotBefore){
          const prompt=state?.prompt;
          const freshPrompt=prompt?.type==='prompt'&&prompt?.prompt_kind==='history_action'&&String(prompt?.prompt_id||'')!==promptID;
          const finished=state?.last_event?.type==='run_finished';
          if(freshPrompt||finished||!prompt){
            renderHistorySnapshot(state.history_snapshot);
            if(freshPrompt)renderHistoryPrompt(sessionId,prompt);
            else{
              activeHistoryPrompt=null;
              activeHistoryRunID='';
              latestHistoryReport=null;
              historyDetail.hidden=true;
              renderHistoryCleanupCapability(null);
              renderHistoryRuns();
            }
            return matched;
          }
        }
      }
      if(state?.last_event?.type==='run_finished'&&!matched)throw new Error('Patch History session finished before cleanup result arrived');
      await new Promise(resolve=>setTimeout(resolve,250));
    }
    throw new Error('Timed out waiting for native Patch History cleanup');
  }

  async function submitHistoryCleanup(sessionId,prompt){
    if(historyBusy||historyManagementBusy||historySupportBusy||historyCleanupBusy||!sessionId||!prompt)return null;
    const cleanup=historyCleanupProjection(prompt);
    if(!cleanup)throw new Error('History Cleanup is not advertised by the active Python prompt');
    if(cleanup.eligible<=0)return null;
    const confirmed=window.confirm(
      `Clean Patch Tool History? Python reports ${cleanup.eligible} eligible entries: ${cleanup.idle_eligible} unpinned IDLE and ${cleanup.overflow_eligible} oldest unpinned over limit. Pinned runs are preserved; newest meaningful History is kept to limit ${cleanup.limit}.`
    );
    if(!confirmed)return null;
    const promptID=String(prompt.prompt_id||'');
    const snapshotBefore=JSON.stringify(latestHistorySnapshot||{});
    historyCleanupBusy=true;
    historyManagement.hidden=false;
    historyManagementFiles.replaceChildren();
    historyManagementMessage.textContent='Cleaning History using Python retention policy…';
    renderHistoryCleanupCapability(prompt);
    renderHistoryRuns();
    try{
      const response=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/history-cleanup`,{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({prompt_id:promptID,confirmed:true}),
      });
      const cleanupID=String(response?.cleanup_id||'');
      if(!cleanupID)throw new Error('TaskDeck did not return a cleanup_id');
      return await waitForHistoryCleanup(sessionId,cleanupID,promptID,snapshotBefore);
    }finally{
      historyCleanupBusy=false;
      renderHistoryCleanupCapability(activeHistoryPrompt);
      renderHistoryRuns();
    }
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
    if(historyManagementBusy||historyBusy||historySupportBusy||!sessionId||!prompt)return null;
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
    renderHistoryCleanupCapability(activeHistoryPrompt);
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
      button.disabled=historyBusy||historyManagementBusy||historySupportBusy||historyCleanupBusy||!allowed.has('detail');
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
        manage.disabled=historyBusy||historyManagementBusy||historySupportBusy||historyCleanupBusy;
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
    latestHistoryReport=report;
    historyDetail.hidden=false;
    historyFiles.replaceChildren();historyItems.replaceChildren();historyWarnings.textContent='';
    if(report.status!=='available'){
      latestHistoryReport=null;
      historyDetailTitle.textContent='History run unavailable';
      historyDetailMeta.textContent=String(report.run_id||'');
      return false;
    }
    const run=report.run&&typeof report.run==='object'?report.run:{};
    activeHistoryRunID=String(run.run_id||activeHistoryRunID);
    const elapsed=Number(run.elapsed_seconds);
    historyDetailTitle.textContent=String(run.primary_name||run.run_id||'History run');
    historyDetailMeta.textContent=[run.display_time,run.status,run.pinned?'PINNED':'',Number.isFinite(elapsed)?elapsed.toFixed(2)+'s':'',run.failure_policy?('failure='+run.failure_policy):'',run.transaction_policy?('transaction='+run.transaction_policy):''].filter(Boolean).join(' · ');
    appendHistoryFiles(historyFiles,Array.isArray(report.files)?report.files:[]);
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
        appendHistoryFiles(files,artifacts);
        row.append(files);
      }
      if(historyItemSupportAllowed(item)){
        const itemActions=document.createElement('div');itemActions.className='task-patch-history-item-actions';
        const support=document.createElement('button');support.type='button';support.className='task-patch-history-support';support.textContent='Support';
        support.disabled=historyBusy||historyManagementBusy||historySupportBusy||historyCleanupBusy;
        support.onclick=()=>submitHistorySupport(activeSessionId,activeHistoryPrompt,activeHistoryRunID,item,support).catch(app.showError);
        itemActions.append(support);row.append(itemActions);
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
    if(historyBusy||historyManagementBusy||historySupportBusy||historyCleanupBusy||!sessionId||!prompt)return null;
    const actionsAllowed=new Set(Array.isArray(prompt.actions)?prompt.actions.map(String):[]);
    const advertised=new Set(Array.isArray(prompt.runs)?prompt.runs.map(row=>String(row?.run_id||'')):[]);
    if(!actionsAllowed.has('detail')||!advertised.has(runID))throw new Error('History run is not advertised by the active Python prompt');
    historyBusy=true;activeHistoryRunID=runID;latestHistoryReport=null;renderHistoryRuns();
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
    renderHistoryCleanupCapability(prompt);
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

  function enterHealthView(){
    healthMode=true;
    panel.classList.add('health');
    healthBox.hidden=false;
    healthStatus.textContent='Loading…';
    healthMessages.textContent='';
  }

  function leaveHealthView(){
    healthMode=false;
    panel.classList.remove('health');
    healthBox.hidden=true;
    latestHealthSnapshot=null;
    healthStatus.textContent='';
    healthOverview.replaceChildren();
    healthChecks.replaceChildren();
    healthMessages.textContent='';
  }

  function appendHealthChip(label,value){
    const chip=document.createElement('span');chip.className='task-patch-health-chip';chip.textContent=`${label}: ${String(value)}`;healthOverview.append(chip);
  }

  function renderHealthSnapshot(snapshot){
    if(!snapshot||typeof snapshot!=='object')return false;
    latestHealthSnapshot=snapshot;
    healthBox.hidden=false;
    healthStatus.textContent=[String(snapshot.status||''),String(snapshot.tool_version||'')].filter(Boolean).join(' · ');
    healthOverview.replaceChildren();
    const summary=snapshot.summary&&typeof snapshot.summary==='object'?snapshot.summary:{};
    appendHealthChip('PASS',Number(summary.pass||0));
    appendHealthChip('WARN',Number(summary.warn||0));
    appendHealthChip('FAIL',Number(summary.fail||0));
    appendHealthChip('Total',Number(summary.total||0));

    healthChecks.replaceChildren();
    for(const check of (Array.isArray(snapshot.checks)?snapshot.checks:[])){
      const row=document.createElement('div');row.className='task-patch-health-row';
      const rowHead=document.createElement('div');rowHead.className='task-patch-health-row-head';
      const name=document.createElement('span');name.className='task-patch-health-row-name';name.textContent=String(check?.name||'');
      const status=document.createElement('span');status.className='task-patch-health-row-status';status.textContent=String(check?.status||'');
      rowHead.append(name,status);row.append(rowHead);
      const detail=[
        check?.detail?String(check.detail):'',
        check?.entries===undefined?'':('entries='+String(check.entries)),
        check?.failures===undefined?'':('failures='+String(check.failures)),
        check?.missing_managed===undefined?'':('missing='+String(check.missing_managed)),
        check?.stale_managed===undefined?'':('stale='+String(check.stale_managed)),
        check?.files===undefined?'':('files='+String(check.files)),
        check?.dirs===undefined?'':('dirs='+String(check.dirs)),
        check?.actual===undefined?'':('actual='+String(check.actual)),
      ].filter(Boolean).join(' · ');
      if(detail){
        const detailNode=document.createElement('div');detailNode.className='task-patch-health-row-detail';detailNode.textContent=detail;row.append(detailNode);
      }
      healthChecks.append(row);
    }
    const warnings=Array.isArray(snapshot.warnings)?snapshot.warnings:[];
    const errors=Array.isArray(snapshot.errors)?snapshot.errors:[];
    healthMessages.textContent=[
      ...warnings.map(value=>'Warning: '+String(value)),
      ...errors.map(value=>'Error: '+String(value)),
    ].join(' | ');
    return true;
  }

  function enterRunningView(){
    runningMode=true;
    runningFinished=false;
    runningStartedAtMs=Date.now();
    runningLastEventCount=-1;
    runningLastEventAtMs=runningStartedAtMs;
    panel.classList.add('running');
    runningTitle.textContent='Current run';
    runningHead.classList.remove('finished');
    runningMeta.classList.remove('stale');
    runningMeta.textContent='Starting Python execution…';
    terminalEvidence.hidden=false;
    runningBack.textContent='Back to Queue / Add more';
    runningBack.hidden=false;
    runBox.hidden=false;
  }

  function finishRunningView(){
    if(!runningMode)return;
    runningFinished=true;
    updateForegroundHeading(foregroundProtocolState);
    runningMeta.classList.remove('stale');
    const completedName=foregroundRunName(foregroundProtocolState);
    runningMeta.textContent=(completedName?completedName+' · ':'')+'completed. Result and artifacts below are the latest foreground run.';
    runningBack.hidden=false;
  }

  function leaveRunningView(){
    runningMode=false;
    runningFinished=false;
    runningStartedAtMs=0;
    runningLastEventCount=-1;
    runningLastEventAtMs=0;
    panel.classList.remove('running');
    runningTitle.textContent='Current run';
    runningHead.classList.remove('finished');
    runningMeta.classList.remove('stale');
    runningMeta.textContent='Waiting for Python execution state…';
    runningBack.hidden=true;
    terminalEvidence.hidden=false;
    foregroundRunDescriptor=null;
    foregroundProtocolState=null;
    renderProgress(null);
    renderItemLifecycle([]);
    renderArtifacts([]);
    if(latestQueueSnapshot)renderQueueSnapshot(latestQueueSnapshot);
    renderParallelCollectRuns();
    if(parallelCollectRuns.size)void pollParallelCollectRuns();
  }

  async function openTerminalEvidenceForSession(sessionId){
    if(!sessionId)return false;
    if(!app.views.has(sessionId)){
      const meta=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}`);
      app.materializeSession(meta,false);
    }
    setVisible(false);
    app.activateView(sessionId);
    returnViewId=sessionId;
    return true;
  }

  async function openTerminalEvidence(){
    return openTerminalEvidenceForSession(activeSessionId);
  }

  async function openLegacyHistoryTerminal(){
    const meta=await app.jsonFetch('/api/sessions',{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({kind:'patch',patch_mode:'history',patch_ui:'terminal'}),
    });
    if(!meta?.id)throw new Error('Terminal History session metadata is incomplete');
    setVisible(false);
    app.materializeSession(meta,true);
    returnViewId=String(meta.id);
    return meta;
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
    const busy=actionBusy||queueMutationBusy||parallelCollectBusy;
    summaryRefresh.disabled=busy;
    for(const button of summaryList.querySelectorAll('.task-patch-item-action,.task-patch-queue-delete'))button.disabled=busy;
    for(const button of promptItems.querySelectorAll('.task-patch-prompt-delete'))button.disabled=busy||button.dataset.patchLocked==='1';
    for(const button of promptButtons.querySelectorAll('button'))button.disabled=busy;
    for(const button of promptTools.querySelectorAll('button'))button.disabled=busy||button.dataset.patchLocked==='1';
    for(const input of promptItems.querySelectorAll('input[type="checkbox"]'))input.disabled=busy||input.dataset.patchLocked==='1';
    for(const select of promptItems.querySelectorAll('select[data-patch-priority-index]'))select.disabled=busy||select.dataset.patchLocked==='1';
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
    failedTab.classList.toggle('has-failures',failedCount>0);
    summaryTitle.textContent=queueSummaryView==='failed'?'Failed':'Queue';
    summaryStatus.textContent=queueSearchQuery
      ? `${items.length}/${groupItems.length} match`
      : (items.length?`${items.length} item(s)`:'Empty');
    summaryList.replaceChildren();
    const visible=items.slice(0,50);
    for(const item of visible){
      const itemFailed=String(item?.group||'').toLowerCase()==='failed';
      const row=document.createElement('div');row.className='task-patch-summary-item';row.classList.toggle('failed',itemFailed);
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
    const skippedRows=Array.isArray(latestQueueSnapshot?.skipped)?latestQueueSnapshot.skipped:[];
    const skippedByWarning=new Map(skippedRows.map(row=>[String(row?.warning||''),row]));
    summaryWarnings.replaceChildren();
    if(warnings.length){
      const title=document.createElement('div');title.className='task-patch-summary-warning-title';title.textContent=String(warnings.length)+' warning(s):';summaryWarnings.append(title);
      for(const warning of warnings){
        const warningText=String(warning||'');
        const line=document.createElement('div');line.className='task-patch-summary-warning-line';
        const text=document.createElement('span');text.className='task-patch-summary-warning-text';text.textContent=warningText;line.append(text);
        const skipped=skippedByWarning.get(warningText);
        const promptItem=skipped?promptItemForQueueItem(skipped):null;
        if(skipped&&promptItem&&queueActionAllowed('delete')){
          const remove=document.createElement('button');remove.type='button';remove.className='task-patch-summary-warning-delete';remove.textContent='Delete';
          remove.onclick=()=>submitQueueDelete(skipped,promptItem).catch(app.showError);line.append(remove);
        }
        summaryWarnings.append(line);
      }
    }
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

  function foregroundRunName(state=foregroundProtocolState){
    const descriptorNames=Array.isArray(foregroundRunDescriptor?.names)?foregroundRunDescriptor.names.filter(Boolean):[];
    if(descriptorNames.length===1)return String(descriptorNames[0]);
    const items=Array.isArray(state?.items)?state.items:[];
    const running=items.find(item=>String(item?.status||'').toUpperCase()==='RUNNING');
    const latest=running||items[items.length-1];
    return String(latest?.name||descriptorNames[0]||'');
  }

  function updateForegroundHeading(state=foregroundProtocolState){
    const name=foregroundRunName(state);
    if(runningFinished){
      runningTitle.textContent=name?'Latest completed · '+name:'Latest completed';
      runningHead.classList.add('finished');
      return;
    }
    runningTitle.textContent=name?'Current run · '+name:'Current run';
    runningHead.classList.remove('finished');
  }

  function renderRunningHeartbeat(state){
    if(!runningMode||runningFinished)return;
    updateForegroundHeading(state);
    const now=Date.now();
    if(!runningStartedAtMs)runningStartedAtMs=now;
    const eventCount=Number(state?.event_count||0);
    if(eventCount!==runningLastEventCount){
      runningLastEventCount=eventCount;
      runningLastEventAtMs=now;
    }else if(!runningLastEventAtMs){
      runningLastEventAtMs=now;
    }
    const activityAge=Math.max(0,(now-runningLastEventAtMs)/1000);
    const progress=state?.progress&&typeof state.progress==='object'?state.progress:null;
    const runningItem=Array.isArray(state?.items)?state.items.find(item=>String(item?.status||'').toUpperCase()==='RUNNING'):null;
    const localElapsed=Math.max(0,(now-runningStartedAtMs)/1000);
    const progressElapsed=Number(progress?.elapsed_seconds);
    const elapsed=progress&&Number.isFinite(progressElapsed)?Math.max(localElapsed,progressElapsed):localElapsed;
    const kind=String(progress?.item_kind||runningItem?.kind||'Work');
    const phase=String(progress?.phase||'');
    const output=Number(progress?.output_lines);
    const parts=[kind+' running',elapsed.toFixed(1)+'s elapsed'];
    if(phase)parts.push('phase '+phase);
    if(progress&&Number.isFinite(output))parts.push(Math.max(0,output)+' output lines');
    parts.push('protocol events '+Math.max(0,eventCount));
    if(activityAge>=5)parts.push('no new protocol event for '+Math.floor(activityAge)+'s');
    else parts.push('last activity '+activityAge.toFixed(1)+'s ago');
    runningMeta.textContent=parts.join(' · ');
    runningMeta.classList.toggle('stale',activityAge>=10);
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

  function failureEvidenceText(item){
    const status=String(item?.status||'FAIL');
    const rc=item?.rc;
    const lines=[
      [String(item?.name||''),String(item?.kind||''),status,rc===undefined||rc===null?'':`rc=${rc}`].filter(Boolean).join(' · '),
    ];
    const diagnosis=String(item?.diagnosis_kind||'').trim();
    const reason=String(item?.failure_reason||'').trim();
    const outputTail=String(item?.output_tail||'').trim();
    if(diagnosis)lines.push('Diagnosis: '+diagnosis);
    if(reason)lines.push('Reason: '+reason);
    if(outputTail)lines.push('', 'Recent console output:', outputTail);
    return lines.join('\n').trim();
  }

  async function copyFailureEvidence(item,button){
    const text=failureEvidenceText(item);
    if(!text)return false;
    let copied=false;
    if(navigator.clipboard&&window.isSecureContext){
      try{await navigator.clipboard.writeText(text);copied=true;}catch{}
    }
    if(!copied){
      const area=document.createElement('textarea');area.value=text;area.setAttribute('readonly','');area.style.position='fixed';area.style.left='-9999px';area.style.top='0';
      document.body.append(area);area.select();
      try{copied=document.execCommand('copy');}finally{area.remove();}
    }
    if(!copied)throw new Error('Cannot copy Patch failure details');
    if(button){
      const original=button.textContent;button.textContent='✓ Copied';
      setTimeout(()=>{if(button.isConnected)button.textContent=original;},1200);
    }
    return true;
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
      const itemStatus=String(item?.status||'');
      const failed=itemStatus.toUpperCase()==='FAIL';
      const row=document.createElement('div');row.className='task-patch-run-item';
      row.classList.toggle('failed',failed);
      const name=document.createElement('span');name.className='task-patch-run-name';
      name.textContent=`${Number(item?.index||0)}. ${String(item?.name||'')} · ${String(item?.kind||'')}`;
      const status=document.createElement('span');status.className='task-patch-run-status';
      const rc=item?.rc;
      status.textContent=rc===undefined||rc===null?itemStatus: `${itemStatus} (rc=${rc})`;
      row.append(name,status);
      if(failed){
        const failure=document.createElement('div');failure.className='task-patch-run-failure';
        const reason=document.createElement('div');reason.className='task-patch-run-failure-reason';
        const diagnosis=String(item?.diagnosis_kind||'').trim();
        const failureReason=String(item?.failure_reason||'').trim();
        reason.textContent=failureReason||diagnosis||`${String(item?.kind||'Work')} failed${rc===undefined||rc===null?'':` (rc=${rc})`}`;
        const failureActions=document.createElement('div');failureActions.className='task-patch-run-failure-actions';
        const copy=document.createElement('button');copy.type='button';copy.textContent='Copy failure details';copy.title='Copy failure reason and recent console output';
        copy.onclick=()=>copyFailureEvidence(item,copy).catch(app.showError);
        failureActions.append(copy);
        failure.append(reason,failureActions);
        const outputTail=String(item?.output_tail||'').trim();
        if(outputTail){
          const details=document.createElement('details');details.className='task-patch-run-failure-console';
          const summary=document.createElement('summary');summary.textContent='Recent console output';
          const pre=document.createElement('pre');pre.textContent=outputTail;
          details.append(summary,pre);failure.append(details);
        }
        row.append(failure);
      }
      runItems.append(row);
    }
  }

  const artifactFamilyLabels={
    fail_handoff:'FAIL handoff',
    ai_sync:'AI sync',
    collect_result:'COLLECT result',
  };

  function artifactFormat(kind,path){
    const normalized=String(kind||'').toLowerCase();
    if(normalized.endsWith('_zip')||/\.zip$/i.test(String(path||'')))return 'ZIP';
    if(normalized.endsWith('_text')||/\.txt$/i.test(String(path||'')))return 'TXT';
    return 'FILE';
  }

  function artifactFamily(kind){return String(kind||'').replace(/_(?:zip|text)$/i,'')||'artifact';}

  function groupProtocolArtifacts(artifacts){
    const groups=new Map();
    for(const artifact of (Array.isArray(artifacts)?artifacts:[])){
      const path=String(artifact?.path||'');
      if(!path.startsWith('artifacts/')||path.includes('..')||path.includes('\\'))continue;
      const family=artifactFamily(artifact?.artifact_kind);
      const item=String(artifact?.item_name||'');
      const index=Number(artifact?.index||0);
      const format=artifactFormat(artifact?.artifact_kind,path);
      const stem=(format==='ZIP'||format==='TXT')?path.replace(/\.(?:zip|txt)$/i,''):path;
      const key=[family,item,index,stem].join('\u0000');
      let group=groups.get(key);
      if(!group){group={family,item,index,primary:false,variants:[]};groups.set(key,group);}
      group.primary=group.primary||artifact?.primary===true;
      group.variants.push({...artifact,path,format:artifactFormat(artifact?.artifact_kind,path)});
    }
    return [...groups.values()];
  }

  function pairedVariantPath(variants){
    const rows=Array.isArray(variants)?variants:[];
    if(!rows.length)return '';
    if(rows.length===1)return String(rows[0]?.path||'');
    const stems=new Set(rows.map(row=>{
      const path=String(row?.path||'');
      const format=String(row?.format||'');
      return (format==='ZIP'||format==='TXT')?path.replace(/\.(?:zip|txt)$/i,''):path;
    }));
    if(stems.size===1){
      const stem=[...stems][0];
      const extensions=rows.map(row=>String(row?.format||'').toLowerCase()).filter(Boolean);
      return stem+'.{'+extensions.join('|')+'}';
    }
    return rows.map(row=>String(row?.path||'')).filter(Boolean).join(' · ');
  }

  function appendArtifactVariantGroup(host,variants){
    const rows=(Array.isArray(variants)?variants:[]).slice().sort((a,b)=>String(a.format).localeCompare(String(b.format)));
    if(!rows.length)return;
    const line=document.createElement('div');line.className='task-patch-artifact-variant';
    const formats=rows.map(row=>String(row.format||'FILE'));
    const formatNode=document.createElement('span');formatNode.className='task-patch-artifact-format';formatNode.textContent=formats.join(' / ');
    const displayPath=pairedVariantPath(rows);
    const pathNode=document.createElement('span');pathNode.className='task-patch-artifact-variant-path';pathNode.textContent=displayPath;pathNode.title=rows.map(row=>String(row.path||'')).join('\n');
    const actionsNode=document.createElement('div');actionsNode.className='task-patch-artifact-actions';
    for(const row of rows){
      const download=document.createElement('a');download.textContent=String(row.format||'FILE')+' Download';download.href='/api/files/download?path='+encodeURIComponent(row.path);download.download='';actionsNode.append(download);
    }
    for(const row of rows){
      const copyPath=document.createElement('button');copyPath.type='button';copyPath.textContent=String(row.format||'FILE')+' Copy path';copyPath.title='Copy full project path';copyPath.onclick=()=>copyFullPath(row.path,copyPath).catch(app.showError);actionsNode.append(copyPath);
    }
    const textVariant=rows.find(row=>String(row.format)==='TXT');
    if(textVariant){const open=document.createElement('button');open.type='button';open.textContent='Open TXT';open.onclick=()=>openProjectFile(textVariant.path);actionsNode.append(open);}
    line.append(formatNode,pathNode,actionsNode);host.append(line);
  }

  function appendProtocolArtifactGroups(host,artifacts){
    const groups=groupProtocolArtifacts(artifacts);
    host.replaceChildren();
    for(const group of groups){
      const row=document.createElement('div');row.className='task-patch-artifact';
      const head=document.createElement('div');head.className='task-patch-artifact-head';
      const label=document.createElement('span');label.className='task-patch-artifact-label';
      const baseLabel=artifactFamilyLabels[group.family]||group.family||'Artifact';
      label.textContent=group.item?baseLabel+' · '+group.item:baseLabel;head.append(label);
      if(group.primary){const badge=document.createElement('span');badge.className='task-patch-artifact-primary';badge.textContent='PRIMARY';head.append(badge);}
      const variants=document.createElement('div');variants.className='task-patch-artifact-variants';
      appendArtifactVariantGroup(variants,group.variants);
      row.append(head,variants);host.append(row);
    }
    return groups.length;
  }

  function renderArtifacts(artifacts){
    artifactBox.hidden=appendProtocolArtifactGroups(artifactList,artifacts)===0;
  }

  function clearPrompt(){
    activeQueuePrompt=null;
    summary.classList.remove('prompt-active');
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

  function parallelCollectCapability(prompt){
    const raw=prompt?.constraints?.parallel_collect_processes;
    if(!raw||typeof raw!=='object'||String(raw.strategy||'')!=='independent_processes')return null;
    const max=Number(raw.max);
    if(!Number.isInteger(max)||max<2||max>16)return null;
    return {max};
  }

  function selectedPromptInputs(){
    return [...promptItems.querySelectorAll('input[type="checkbox"]:checked')].filter(input=>!input.disabled&&!input.closest('.task-patch-prompt-item')?.hidden);
  }

  function selectedPromptKinds(){return selectedPromptInputs().map(input=>String(input.dataset.patchKind||'').toUpperCase());}

  function selectAllPromptCollects(prompt){
    const capability=parallelCollectCapability(prompt);
    if(!capability)return;
    let selected=0;
    const limit=Math.max(0,capability.max-activeRunningRunCount());
    for(const input of promptItems.querySelectorAll('input[type="checkbox"]')){
      const kind=String(input.dataset.patchKind||'').toUpperCase();
      const hidden=Boolean(input.closest('.task-patch-prompt-item')?.hidden);
      input.checked=!hidden&&!input.disabled&&kind==='COLLECT'&&selected<limit;
      if(input.checked)selected+=1;
      clearPromptPriority(Number(input.dataset.patchIndex));
    }
    for(const select of promptItems.querySelectorAll('select[data-patch-priority-index]'))select.value='';
    promptNote.textContent='Selected '+selected+' COLLECT request(s). They will run as independent parallel processes.'+(limit===0?' No parallel slot is currently available.':(selected===limit?' '+limit+' available slot(s) selected.':''));
  }

  function selectAllPromptPatches(prompt){
    for(const input of promptItems.querySelectorAll('input[type="checkbox"]')){
      const kind=String(input.dataset.patchKind||'').toUpperCase();
      const hidden=Boolean(input.closest('.task-patch-prompt-item')?.hidden);
      input.checked=!hidden&&!input.disabled&&kind==='PATCH';
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
    if(!changed.checked){clearPromptPriority(changedIndex);return;}
    const constraints=prompt?.constraints&&typeof prompt.constraints==='object'?prompt.constraints:{};
    if(constraints.collect_exclusive!==true)return;
    const changedKind=String(changed.dataset.patchKind||'').toUpperCase();
    const parallel=parallelCollectCapability(prompt);
    if(parallel){
      if(changedKind==='COLLECT'){
        for(const input of promptItems.querySelectorAll('input[type="checkbox"]')){
          if(input===changed||!input.checked)continue;
          if(String(input.dataset.patchKind||'').toUpperCase()==='PATCH'){input.checked=false;clearPromptPriority(Number(input.dataset.patchIndex));}
        }
        const selectedCollects=selectedPromptInputs().filter(input=>String(input.dataset.patchKind||'').toUpperCase()==='COLLECT');
        if(selectedCollects.length>parallel.max){
          changed.checked=false;
          promptNote.textContent='Parallel COLLECT limit is '+parallel.max+' independent requests.';
          return;
        }
        for(const select of promptItems.querySelectorAll('select[data-patch-priority-index]'))select.value='';
        promptNote.textContent=selectedCollects.length>1?selectedCollects.length+' COLLECT requests selected for parallel execution.':'COLLECT runs independently; select more COLLECT requests to run them in parallel.';
        return;
      }
      if(changedKind==='PATCH'){
        for(const input of promptItems.querySelectorAll('input[type="checkbox"]')){
          if(input!==changed&&input.checked&&String(input.dataset.patchKind||'').toUpperCase()==='COLLECT')input.checked=false;
        }
        return;
      }
    }
    for(const input of promptItems.querySelectorAll('input[type="checkbox"]')){
      if(input===changed||!input.checked)continue;
      const kind=String(input.dataset.patchKind||'').toUpperCase();
      if(changedKind==='COLLECT'||kind==='COLLECT'){input.checked=false;clearPromptPriority(Number(input.dataset.patchIndex));}
    }
    if(changedKind==='COLLECT')for(const select of promptItems.querySelectorAll('select[data-patch-priority-index]'))select.value='';
  }

  function activeRunningEntries(){
    return [...parallelCollectRuns.entries()].filter(([_key,run])=>run&&run.active!==false&&!run.error);
  }

  function activeRunningRuns(){return activeRunningEntries().map(([_key,run])=>run);}

  function activeRunningRunCount(){return activeRunningEntries().length;}

  function sessionMetadataRunning(meta){
    return String(meta?.status||'').toLowerCase()==='running';
  }

  function protocolRunFinished(state){
    return state?.last_event?.type==='run_finished';
  }

  function activeRunningNames(){
    const names=new Set();
    for(const run of activeRunningRuns()){
      for(const name of (Array.isArray(run.names)?run.names:[run.name])){
        const value=String(name||'').trim();
        if(value)names.add(value);
      }
    }
    return names;
  }

  function foregroundDescriptorFromPrompt(prompt,indexes){
    const selected=new Set(Array.isArray(indexes)?indexes.map(Number):[]);
    const rows=(Array.isArray(prompt?.items)?prompt.items:[]).filter(item=>selected.has(Number(item?.index)));
    return {
      names:rows.map(item=>String(item?.name||'')).filter(Boolean),
      kinds:rows.map(item=>String(item?.kind||'').toUpperCase()).filter(Boolean),
    };
  }

  async function rememberForegroundRun(sessionId){
    sessionId=String(sessionId||'');
    if(!sessionId)return null;
    const state=foregroundProtocolState&&typeof foregroundProtocolState==='object'?foregroundProtocolState:null;
    const key='session:'+sessionId;
    let meta=null;
    try{meta=await app.jsonFetch('/api/sessions/'+encodeURIComponent(sessionId));}
    catch(error){
      if(runningFinished||protocolRunFinished(state)){parallelCollectRuns.delete(key);return null;}
      console.warn('Active run metadata unavailable while returning to Queue:',error);
    }
    if((meta&&!sessionMetadataRunning(meta))||runningFinished||protocolRunFinished(state)){
      parallelCollectRuns.delete(key);
      return null;
    }
    const stateItems=Array.isArray(state?.items)?state.items:[];
    const names=(Array.isArray(foregroundRunDescriptor?.names)&&foregroundRunDescriptor.names.length
      ? foregroundRunDescriptor.names
      : stateItems.map(item=>String(item?.name||'')).filter(Boolean));
    const kinds=(Array.isArray(foregroundRunDescriptor?.kinds)&&foregroundRunDescriptor.kinds.length
      ? foregroundRunDescriptor.kinds
      : stateItems.map(item=>String(item?.kind||'').toUpperCase()).filter(Boolean));
    const label=names.length===1?names[0]:(names.length?((kinds[0]||'PATCH')+' batch · '+names.length+' items'):'Patch Tool run');
    const existing=parallelCollectRuns.get(key);
    const eventCount=Number(state?.event_count||0);
    const run={
      ...(existing||{}),
      active:true,
      name:label,
      names,
      kinds,
      sessionId,
      error:String(existing?.error||''),
      metadata:meta||existing?.metadata||null,
      state:state||existing?.state||null,
      startedAt:Number(existing?.startedAt||runningStartedAtMs||Date.now()),
      lastEventCount:Number.isFinite(eventCount)?eventCount:Number(existing?.lastEventCount??-1),
      lastEventAt:Number(existing?.lastEventAt||runningLastEventAtMs||Date.now()),
    };
    parallelCollectRuns.set(key,run);
    return run;
  }

  async function openQueueWhileRunning(){
    if(activeSessionId&&runningMode)await rememberForegroundRun(activeSessionId);
    protocolPollGeneration+=1;
    activeSessionId='';
    runningMode=false;
    runningFinished=false;
    panel.classList.remove('running');
    runBox.hidden=true;
    artifactBox.hidden=true;
    terminalEvidence.hidden=false;
    foregroundRunDescriptor=null;
    foregroundProtocolState=null;
    await start('queue');
    renderParallelCollectRuns();
    if(parallelCollectRuns.size)void pollParallelCollectRuns();
  }

  async function refreshQueueFromSummary(){
    if(actionBusy||queueMutationBusy||parallelCollectBusy)return;
    if(activeSessionId)return refreshQueueSession(activeSessionId);
    return start('queue');
  }

  async function refreshQueueSession(sessionId){
    sessionId=String(sessionId||'');
    if(sessionId&&sessionId===activeSessionId){
      protocolPollGeneration+=1;
      try{await app.jsonFetch('/api/sessions/'+encodeURIComponent(sessionId)+'/stop',{method:'POST'});}catch{}
      activeSessionId='';
    }
    clearPrompt();
    await start('queue');
    renderParallelCollectRuns();
    if(parallelCollectRuns.size)void pollParallelCollectRuns();
  }

  summaryRefresh.onclick=()=>refreshQueueFromSummary().catch(app.showError);

  async function removeRunFromActive(key,run,sourceButton){
    if(sourceButton)sourceButton.disabled=true;
    try{
      if(run?.sessionId){
        const meta=await app.jsonFetch('/api/sessions/'+encodeURIComponent(run.sessionId));
        run.metadata=meta;
        if(sessionMetadataRunning(meta)){
          throw new Error('This session is still running. It cannot be removed from Active runs yet.');
        }
      }
      parallelCollectRuns.delete(key);
      renderParallelCollectRuns();
      refreshActionDisabledState();
      return true;
    }finally{
      if(sourceButton?.isConnected)sourceButton.disabled=false;
    }
  }

  function resetParallelCollectRuns(){
    parallelCollectPollGeneration+=1;
    parallelCollectBusy=false;
    parallelCollectRuns.clear();
    parallelCollectList.replaceChildren();
    parallelCollectBox.hidden=true;
  }

  function parallelRunStatus(run){
    if(run.error)return 'LAUNCH FAILED';
    const items=Array.isArray(run.state?.items)?run.state.items:[];
    const item=items.find(row=>String(row?.status||'').toUpperCase()==='RUNNING')||items.find(row=>['FAIL','FAILED','INCOMPLETE'].includes(String(row?.status||'').toUpperCase()))||items[items.length-1]||null;
    const status=String(item?.status||run.state?.progress?.status||'').toUpperCase();
    if(status)return status;
    return run.sessionId?'RUNNING':'STARTING';
  }

  function renderParallelCollectRuns(){
    parallelCollectList.replaceChildren();
    const entries=activeRunningEntries().sort((a,b)=>Number(b[1]?.startedAt||0)-Number(a[1]?.startedAt||0));
    parallelCollectTitle.textContent=runningMode?'Other active runs':'Active runs';
    for(const [key,run] of entries){
      const card=document.createElement('div');card.className='task-patch-parallel-run';
      const head=document.createElement('div');head.className='task-patch-parallel-run-head';
      const name=document.createElement('span');name.className='task-patch-parallel-run-name';name.textContent=run.name;
      const statusValue=parallelRunStatus(run);
      const status=document.createElement('span');status.className='task-patch-parallel-run-status';status.textContent=statusValue;
      head.append(name,status);card.append(head);
      const now=Date.now();
      const progress=run.state?.progress&&typeof run.state.progress==='object'?run.state.progress:null;
      const elapsedValue=Number(progress?.elapsed_seconds);
      const elapsed=Number.isFinite(elapsedValue)?elapsedValue:Math.max(0,(now-run.startedAt)/1000);
      const age=Math.max(0,(now-run.lastEventAt)/1000);
      const parts=[elapsed.toFixed(1)+'s'];
      if(progress?.phase)parts.push('phase '+String(progress.phase));
      if(Number.isFinite(Number(progress?.output_lines)))parts.push(Number(progress.output_lines)+' lines');
      parts.push('events '+Math.max(0,Number(run.state?.event_count||0)));
      if(run.protocolError)parts.push('protocol warning');
      parts.push(age>=5?'no new event '+Math.floor(age)+'s':'activity '+age.toFixed(1)+'s ago');
      const meta=document.createElement('div');meta.className='task-patch-parallel-run-meta';meta.classList.toggle('stale',age>=10||Boolean(run.protocolError));meta.textContent=parts.join(' · ');card.append(meta);
      const actionsNode=document.createElement('div');actionsNode.className='task-patch-parallel-run-actions';
      if(run.sessionId){
        const terminal=document.createElement('button');terminal.type='button';terminal.textContent='Terminal evidence';terminal.onclick=()=>openTerminalEvidenceForSession(run.sessionId).catch(app.showError);actionsNode.append(terminal);
        const remove=document.createElement('button');remove.type='button';remove.textContent='Remove from Active';remove.title='Remove only after TaskDeck confirms this session is no longer running';remove.onclick=()=>removeRunFromActive(key,run,remove).catch(app.showError);actionsNode.append(remove);
      }
      if(actionsNode.childNodes.length)card.append(actionsNode);
      const artifacts=document.createElement('div');artifacts.className='task-patch-parallel-artifacts';
      if(appendProtocolArtifactGroups(artifacts,run.state?.artifacts)>0)card.append(artifacts);
      parallelCollectList.append(card);
    }
    parallelCollectBox.hidden=entries.length===0;
    if(entries.length&&runningMode){
      runningBack.hidden=false;
    }
  }

  async function pollParallelCollectRuns(){
    if(!parallelCollectRuns.size)return;
    const generation=++parallelCollectPollGeneration;
    while(generation===parallelCollectPollGeneration&&panel.classList.contains('visible')){
      let pending=0;
      for(const [key,run] of [...parallelCollectRuns.entries()]){
        if(run.error||!run.sessionId){parallelCollectRuns.delete(key);continue;}
        let meta;
        try{
          meta=await app.jsonFetch('/api/sessions/'+encodeURIComponent(run.sessionId));
          run.metadata=meta;
          run.metadataError='';
        }catch(error){
          run.metadataError=String(error?.message||error);
          console.warn('Active run metadata unavailable:',error);
          pending+=1;
          continue;
        }
        if(!sessionMetadataRunning(meta)){
          parallelCollectRuns.delete(key);
          continue;
        }
        pending+=1;
        try{
          const state=await app.jsonFetch('/api/sessions/'+encodeURIComponent(run.sessionId)+'/protocol');
          const eventCount=Number(state?.event_count||0);
          if(eventCount!==run.lastEventCount){run.lastEventCount=eventCount;run.lastEventAt=Date.now();}
          run.state=state;
          run.protocolError='';
        }catch(error){
          run.protocolError='Protocol state unavailable: '+String(error?.message||error);
        }
      }
      renderParallelCollectRuns();
      refreshActionDisabledState();
      if(!pending||activeRunningRunCount()===0)return;
      await new Promise(resolve=>setTimeout(resolve,1000));
    }
  }

  async function launchParallelCollect(sessionId,prompt,indexes){
    const capability=parallelCollectCapability(prompt);
    if(!capability||indexes.length<1||indexes.length>capability.max)throw new Error('Independent COLLECT worker selection is outside the advertised capability');
    const availableSlots=Math.max(0,capability.max-activeRunningRunCount());
    if(indexes.length>availableSlots)throw new Error('Parallel COLLECT limit reached: '+activeRunningRunCount()+' active, '+availableSlots+' slot(s) available.');
    parallelCollectBusy=true;refreshActionDisabledState();
    try{
      const response=await app.jsonFetch('/api/sessions/'+encodeURIComponent(sessionId)+'/parallel-collect',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({prompt_id:String(prompt?.prompt_id||''),indexes})});
      clearPrompt();activeSessionId='';foregroundRunDescriptor=null;foregroundProtocolState=null;
      enterRunningView();parallelCollectBusy=false;
      for(const row of (Array.isArray(response?.runs)?response.runs:[])){
        const sessionId=String(row?.session?.id||'');
        const name=String(row?.name||'COLLECT');
        const key=sessionId?('session:'+sessionId):('launch-failed:'+Date.now()+':'+String(row?.index||parallelCollectRuns.size+1));
        const launchError=String(row?.error||'');
        if(launchError){console.warn('Parallel COLLECT launch failed:',name,launchError);continue;}
        parallelCollectRuns.set(key,{active:true,index:Number(row?.index)||0,name,names:[name],kinds:['COLLECT'],sessionId,error:'',metadata:row?.session||null,state:null,startedAt:Date.now(),lastEventCount:-1,lastEventAt:Date.now(),protocolError:''});
      }
      if(activeRunningRunCount()===0)throw new Error('Parallel COLLECT launch returned no active workers');
      renderParallelCollectRuns();void pollParallelCollectRuns();
      return response;
    }catch(error){parallelCollectBusy=false;refreshActionDisabledState();throw error;}
  }

  async function submitPromptResponse(sessionId,prompt,action){
    const payload={prompt_id:String(prompt?.prompt_id||''),action};
    let selectedDescriptor=null;
    if(action==='select'){
      payload.indexes=selectedPromptIndexes();
      selectedDescriptor=foregroundDescriptorFromPrompt(prompt,payload.indexes);
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
      if(action==='select'){foregroundRunDescriptor=selectedDescriptor;foregroundProtocolState=null;enterRunningView();}
      void pollProtocol(sessionId,false,true);
    }catch(error){
      for(const button of promptButtons.querySelectorAll('button'))button.disabled=false;
      throw error;
    }
  }

  function renderQueuePrompt(sessionId,prompt){
    if(prompt?.type!=='prompt'||prompt?.prompt_kind!=='queue_selection'||!prompt?.prompt_id)return false;
    const items=Array.isArray(prompt.items)?prompt.items:[];
    const runnableItems=items.filter(item=>String(item?.kind||'').toUpperCase()!=='SKIPPED'&&item?.selectable!==false);
    const initial=new Set(Array.isArray(prompt.initial_selected)?prompt.initial_selected.map(Number):[]);
    const actionsAllowed=new Set(Array.isArray(prompt.actions)?prompt.actions.map(String):[]);
    clearPrompt();
    activeQueuePrompt=prompt;
    summary.classList.add('prompt-active');
    summaryTitle.textContent='Queue overview';
    promptBox.hidden=false;
    promptTitle.textContent=String(prompt.title||'Choose PATCH/COLLECT work');
    const priorityCapability=patchPriorityCapability(prompt);
    const parallelCollect=parallelCollectCapability(prompt);
    const addingWhileRuns=activeRunningRunCount()>0;
    const runningNames=activeRunningNames();
    promptNote.textContent=!runnableItems.length
      ? 'No runnable PATCH/COLLECT item is currently available. Refresh Queue to detect newly added work. SKIPPED files can be deleted below.'
      : (addingWhileRuns
        ? 'Active run(s) continue in the background. This Queue is add-mode: start additional COLLECT only; PATCH items stay locked until active runs finish.'
        : (parallelCollect?'PATCH keeps normal priority rules. Multiple COLLECT requests may be selected and TaskDeck will run each in its own independent process.':(priorityCapability?'Select work and optionally assign PATCH priority 0–9. Python validates and orders the final selection.':'Select work here, or use the terminal tab. Python validates the final selection.')));
    const selectAll=document.createElement('button');selectAll.type='button';selectAll.textContent='Select all PATCH';selectAll.dataset.patchLocked=(addingWhileRuns||!runnableItems.length)?'1':'0';selectAll.disabled=addingWhileRuns||!runnableItems.length;
    selectAll.onclick=()=>selectAllPromptPatches(prompt);
    const clearAll=document.createElement('button');clearAll.type='button';clearAll.textContent='Clear selection';
    clearAll.onclick=clearPromptSelection;
    promptTools.append(selectAll);
    if(parallelCollect&&runnableItems.some(item=>String(item?.kind||'').toUpperCase()==='COLLECT')){const selectCollect=document.createElement('button');selectCollect.type='button';selectCollect.textContent='Select all COLLECT (parallel)';selectCollect.onclick=()=>selectAllPromptCollects(prompt);promptTools.append(selectCollect);}
    promptTools.append(clearAll);
    const refresh=document.createElement('button');refresh.type='button';refresh.textContent='Refresh Queue';refresh.onclick=()=>refreshQueueSession(sessionId).catch(app.showError);promptTools.append(refresh);
    for(const item of items){
      const index=Number(item?.index);
      if(!Number.isInteger(index)||index<1)continue;
      const itemGroup=String(item?.group||'').toLowerCase();
      const itemKind=String(item?.kind||'').toUpperCase();
      const skipped=itemKind==='SKIPPED'||item?.selectable===false;
      const row=document.createElement('div');row.className='task-patch-prompt-item';row.dataset.patchPromptIndex=String(index);row.classList.toggle('failed',itemGroup==='failed');row.classList.toggle('skipped',skipped);
      const itemName=String(item?.name||'');
      const duplicateRunning=runningNames.has(itemName);
      const addModePatch=addingWhileRuns&&itemKind==='PATCH';
      const locked=skipped||duplicateRunning||addModePatch;
      const input=document.createElement('input');input.type='checkbox';input.dataset.patchIndex=String(index);input.dataset.patchKind=String(item?.kind||'');input.dataset.patchLocked=locked?'1':'0';
      input.disabled=locked;input.hidden=skipped;
      input.checked=!locked&&initial.has(index);
      input.onchange=()=>applyPromptConstraints(input,prompt);
      const copy=document.createElement('label');copy.className='task-patch-prompt-copy';
      const name=document.createElement('span');name.className='task-patch-prompt-name';
      name.textContent=`${index}. ${itemName}`;
      const detail=document.createElement('span');detail.className='task-patch-prompt-detail';
      detail.textContent=[skipped?'SKIPPED':(itemGroup==='failed'?'FAILED':item?.group),item?.kind,item?.detail,duplicateRunning?'already running':(addModePatch?'locked while active run exists':'')].filter(Boolean).join(' · ');
      copy.append(name,detail);
      row.append(input,copy);
      if(priorityCapability&&String(item?.kind||'').toUpperCase()==='PATCH'){
        const priorityWrap=document.createElement('label');priorityWrap.className='task-patch-prompt-priority';priorityWrap.textContent='Priority';
        const priority=document.createElement('select');priority.dataset.patchPriorityIndex=String(index);priority.dataset.patchLocked=locked?'1':'0';priority.disabled=locked;
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
      if(queueActionAllowed('delete')){
        const remove=document.createElement('button');remove.type='button';remove.className='task-patch-prompt-delete';remove.textContent='Delete';remove.dataset.patchLocked=duplicateRunning?'1':'0';remove.disabled=duplicateRunning;
        remove.onclick=()=>submitQueueDelete(item,item).catch(app.showError);
        row.append(remove);
      }
      promptItems.append(row);
    }
    if(actionsAllowed.has('select')&&runnableItems.length){
      const select=document.createElement('button');select.type='button';select.textContent='Run selected';
      select.onclick=()=>{
        const indexes=selectedPromptIndexes();
        if(!indexes.length){
          promptNote.textContent='Select at least one item, or Cancel.';
          return;
        }
        const kinds=selectedPromptKinds();
        const parallel=parallelCollectCapability(prompt);
        if(parallel&&kinds.every(kind=>kind==='COLLECT')&&(indexes.length>1||activeRunningRunCount()>0)){
          launchParallelCollect(sessionId,prompt,indexes).catch(app.showError);
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
    let haveHealthSnapshot=false;
    const maxAttempts=followLifecycle?7200:40;
    const delayMs=followLifecycle?1000:250;
    for(let attempt=0;attempt<maxAttempts;attempt+=1){
      if(generation!==protocolPollGeneration||!panel.classList.contains('visible'))return;
      let state;
      try{
        state=await app.jsonFetch(`/api/sessions/${encodeURIComponent(sessionId)}/protocol`);
      }catch(error){
        resetSummary('Native state unavailable · Terminal fallback available');
        console.warn('Patch protocol state unavailable:',error);
        summaryWarnings.textContent='Patch protocol state unavailable. Open terminal evidence/fallback if needed.';
        return;
      }
      if(state?.available===false){
        if(healthMode){
          healthStatus.textContent='PTY fallback';
          healthMessages.textContent='Native Health protocol state unavailable. Use Terminal evidence/fallback.';
          return;
        }
        if(planMode){
          planStatus.textContent='PTY fallback';
          planWarnings.textContent='Native Plan protocol state unavailable. Use Terminal evidence/fallback.';
          return;
        }
        resetSummary('Native unavailable · Terminal fallback available');
        summaryWarnings.textContent='Open terminal evidence/fallback if needed.';
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
      if(state?.health_snapshot&&!haveHealthSnapshot){
        renderHealthSnapshot(state.health_snapshot);
        haveHealthSnapshot=true;
      }
      if(state?.history_management_result)renderHistoryManagementResult(state.history_management_result);
      if(runningMode&&sessionId===activeSessionId){foregroundProtocolState=state;updateForegroundHeading(state);}
      renderItemLifecycle(state?.items);
      renderProgress(state?.progress);
      renderRunningHeartbeat(state);
      if(state?.action_result)renderActionResult(state.action_result);
      renderArtifacts(state?.artifacts);
      if(expectPrompt&&state?.commands_enabled===false&&(haveSnapshot||haveResumeSnapshot||haveHistorySnapshot)){
        summaryStatus.textContent+=' · Terminal fallback available';
        if(haveResumeSnapshot)resumeNote.textContent='Native Resume command channel unavailable. Open terminal fallback if needed.';
        if(haveHistorySnapshot)historyWarnings.textContent='Native History command channel unavailable. Open terminal fallback if needed.';
        if(!historyMode)summaryWarnings.textContent='Native command channel unavailable. Open terminal evidence/fallback if needed.';
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
      if(!expectPrompt&&haveHealthSnapshot)return;
      if(!expectPrompt&&haveSnapshot&&!followLifecycle)return;
      if(state?.error){
        if(healthMode){
          healthStatus.textContent='Protocol error';
          healthMessages.textContent=String(state.error);
          return;
        }
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
    if(healthMode){
      healthStatus.textContent=haveHealthSnapshot?healthStatus.textContent:'PTY fallback';
      if(!haveHealthSnapshot)healthMessages.textContent='Native Health snapshot unavailable or timed out. Use Terminal evidence/fallback.';
      return;
    }
    if(planMode){
      planStatus.textContent=havePlanSnapshot?planStatus.textContent:'PTY fallback';
      if(!havePlanSnapshot)planWarnings.textContent='Native Plan snapshot unavailable or timed out. Use Terminal evidence/fallback.';
      return;
    }
    if(haveSnapshot||haveResumeSnapshot||haveHistorySnapshot){
      summaryStatus.textContent+=' · Terminal fallback available';
      if(haveResumeSnapshot)resumeNote.textContent='Native Resume prompt timed out. Open terminal fallback if needed.';
      if(haveHistorySnapshot)historyWarnings.textContent='Native History prompt timed out. Open terminal fallback if needed.';
    }else{
      resetSummary('Snapshot timeout · Terminal fallback available');
    }
    if(sessionId===activeSessionId&&!runningMode&&!historyMode){
      summaryWarnings.textContent='Native state timed out. Open terminal evidence/fallback if needed.';
    }
  }

  async function assertHeadlessNativeSession(meta){
    if(!meta?.id)throw new Error('Patch session metadata is incomplete');
    if(Number(meta.task_id)!==-1){
      try{await app.jsonFetch(`/api/sessions/${encodeURIComponent(meta.id)}/stop`,{method:'POST'});}catch{}
      throw new Error('Native Patch invariant failed: backend returned a non-headless session');
    }
    if(app.views.has(meta.id)){
      throw new Error('Native Patch invariant failed: session was materialized as a terminal tab');
    }
    return meta;
  }

  async function start(mode,sourceButton=null){
    if(sourceButton)sourceButton.disabled=true;
    try{
      const meta=await app.jsonFetch('/api/sessions',{
        method:'POST',
        headers:{'Content-Type':'application/json'},
        body:JSON.stringify({kind:'patch',patch_mode:mode,patch_ui:patchUIMode()}),
      });
      if(patchUIMode()==='terminal'){
        activeSessionId=meta.id;
        patchTab.hidden=true;
        setVisible(false);
        app.materializeSession(meta,true);
        return meta;
      }
      await assertHeadlessNativeSession(meta);
      activeSessionId=meta.id;
      actionPollGeneration+=1;
      queueMutationPollGeneration+=1;
      queueMutationBusy=false;
      historyPollGeneration+=1;
      historyManagementPollGeneration+=1;
      historyCleanupPollGeneration+=1;
      historyManagementBusy=false;
      historyCleanupBusy=false;
      leaveRunningView();
      if(mode==='history')enterHistoryView();else leaveHistoryView();
      if(mode==='plan')enterPlanView();else leavePlanView();
      if(mode==='health')enterHealthView();else leaveHealthView();
      clearActionResult();
      clearResumeView();
      renderProgress(null);
      renderArtifacts([]);
      window.dispatchEvent(new CustomEvent('taskmenu:patch-session-started',{detail:{mode,meta}}));
      if(mode==='queue'||mode==='resume'||mode==='history'||mode==='plan'||mode==='health'){
        void pollProtocol(meta.id,mode==='queue'||mode==='resume'||mode==='history',mode==='resume'||mode==='history'||mode==='plan'||mode==='health');
      }
      return meta;
    }finally{
      if(sourceButton?.isConnected)sourceButton.disabled=false;
    }
  }

  terminalEvidence.onclick=()=>openTerminalEvidence().catch(app.showError);
  historyTerminal.onclick=()=>openLegacyHistoryTerminal().catch(app.showError);
  historyCleanupButton.onclick=()=>submitHistoryCleanup(activeSessionId,activeHistoryPrompt).catch(app.showError);
  historyBack.onclick=()=>stopHistoryAndBack().catch(app.showError);
  planTerminal.onclick=()=>openTerminalEvidence().catch(app.showError);
  planBack.onclick=leavePlanView;
  healthTerminal.onclick=()=>openTerminalEvidence().catch(app.showError);
  healthBack.onclick=leaveHealthView;
  runningBack.onclick=()=>openQueueWhileRunning().catch(app.showError);
  actionResultClose.onclick=clearActionResult;
  queueTab.onclick=()=>setQueueSummaryView('queue');
  failedTab.onclick=()=>setQueueSummaryView('failed');
  summarySearchInput.oninput=()=>setQueueSearchQuery(summarySearchInput.value);
  summarySearchClear.onclick=()=>{setQueueSearchQuery('');summarySearchInput.focus();};
  closeButton.onclick=close;
  globalThis.TaskMenuPatchPanel={open,close,deactivate,toggle,start,openLegacyHistoryTerminal,openQueueWhileRunning,refreshQueueSession,removeRunFromActive,launchParallelCollect,pollParallelCollectRuns,renderParallelCollectRuns,enterRunningView,finishRunningView,leaveRunningView,openTerminalEvidence,renderQueueSnapshot,setQueueSummaryView,renderQueuePrompt,selectedPromptPriorities,selectAllPromptPatches,clearPromptSelection,renderResumeSnapshot,renderResumePrompt,submitResumeAction,renderHistorySnapshot,renderHistoryPrompt,renderHistoryReport,submitHistoryDetail,submitHistoryManagement,renderHistoryManagementResult,historyItemSupportAllowed,submitHistorySupport,renderHistorySupportResult,historyCleanupProjection,renderHistoryCleanupCapability,submitHistoryCleanup,renderHistoryCleanupResult,enterPlanView,leavePlanView,renderPlanSnapshot,enterHealthView,leaveHealthView,renderHealthSnapshot,submitItemAction,submitQueueDelete,renderActionResult,renderItemLifecycle,renderProgress,renderArtifacts,get panel(){return panel;},get visible(){return panel.classList.contains('visible');}};
  return true;
}

function safeInstallPatchPanel(){
  try{installPatchPanel();}
  catch(error){console.warn('Patch panel enhancement disabled:',error);}
}

if(app?.taskData?.workspace)safeInstallPatchPanel();
else window.addEventListener('taskmenu:tasks',safeInstallPatchPanel,{once:true});
