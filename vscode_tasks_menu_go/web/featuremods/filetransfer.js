const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for file transfer workspace');

const views=new Map();
let profilesByID=new Map();
let contextMenu=null;
let activeViewID='';
let restoringSession=false;
let sessionRestoreStarted=false;
let sessionPersistenceReady=false;
const activeRemoteDeleteJobs=new Set();
const remoteDeleteRuntime=new Map();
const activeLocalTransferScans=new Set();

const localDBName='TaskDeckFileTransfer';
const localDBStore='localRoots';
const localDBVersion=1;
const maxRecentPaths=50;
const maxFavoritePaths=20;

const style=document.createElement('style');
style.textContent=`
.ft-tab{border-radius:6px 6px 0 0;border-bottom:0;margin-left:4px}
.ft-tab.active{background:#343b48}.ft-tab .close{margin-left:8px}
.ft-pane{position:absolute;inset:0;display:flex;flex-direction:column;background:#0d1015}
.ft-pane.hidden{display:none}.ft-pane-head{height:38px;display:flex;align-items:center;gap:8px;padding:5px 9px;border-bottom:1px solid #30343b}
.ft-pane-title{font-weight:600;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ft-pane-meta{font-size:10px;opacity:.55}.ft-pane-spacer{flex:1}
.ft-sites{flex:1;min-height:0;display:grid;grid-template-columns:minmax(260px,1fr) 8px minmax(260px,1fr)}
.ft-site{min-width:0;min-height:0;display:flex;flex-direction:column}
.ft-site-head{display:flex;align-items:center;gap:6px;padding:6px 7px;border-bottom:1px solid #30343b;background:#11151b}
.ft-site-title{font-size:11px;font-weight:700;white-space:nowrap}
.ft-source-select,.ft-root-select{height:28px;min-height:28px;padding:3px 6px;font-size:11px}
.ft-root-select{max-width:210px}
.ft-path-history-select{width:30px;min-width:30px;height:28px;padding:0 2px;border:1px solid #3b414d;border-radius:5px;background:#161b22;color:inherit;font-size:11px;text-align:center;appearance:none;-webkit-appearance:none;cursor:pointer}
.ft-path-history-select option,.ft-path-history-select optgroup{font-size:11px;text-align:left}
.ft-pathbar{display:flex;align-items:center;gap:5px;padding:6px 7px;border-bottom:1px solid #30343b;background:#11151b}
.ft-pathbar input[type=text]{flex:1;min-width:80px;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:6px 8px;font:11px ui-monospace,monospace}
.ft-pathbar button,.ft-site-head button{height:28px;padding:3px 7px;font-size:11px}
.ft-favorite-toggle.saved{background:#554817;border-color:#8c7731}
.ft-status{padding:5px 8px;border-bottom:1px solid #272d36;font-size:10px;opacity:.7;min-height:24px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.ft-table-wrap{flex:1;min-height:0;overflow:auto}
.ft-table{width:100%;border-collapse:collapse;font-size:11px;table-layout:auto}
.ft-table th,.ft-table td{padding:6px 8px;border-bottom:1px solid #272d36;text-align:left;white-space:nowrap}
.ft-table th{position:sticky;top:0;background:#171c23;z-index:2;font-size:10px;opacity:.86;cursor:pointer;user-select:none}
.ft-table th.ft-nosort{cursor:default}.ft-table th .sort{margin-left:4px;opacity:.7}
.ft-table tbody tr{cursor:default}.ft-table tbody tr:hover{background:#202731}.ft-table tbody tr.selected{background:#29384b}
.ft-table tbody tr,.ft-table tbody td{user-select:none;-moz-user-select:none;-webkit-user-select:none}
.ft-table tbody tr:focus,.ft-table tbody tr:focus-visible,.ft-table tbody tr:focus-within,.ft-table tbody tr:active,.ft-table tbody td:focus,.ft-table tbody td:focus-visible,.ft-table tbody td:active,.ft-table tbody tr.selected,.ft-table tbody tr.selected td{outline:none!important;box-shadow:none!important}
.ft-table tbody tr.selected td,.ft-table tbody tr:focus-within td,.ft-table tbody tr:active td,.ft-table tbody td:focus,.ft-table tbody td:focus-visible,.ft-table tbody td:active{border-top:0!important;border-left:0!important;border-right:0!important}
.ft-parent-row{font-weight:600}.ft-parent-row td{background:#11161d}.ft-parent-row:hover td{background:#202731!important}
.ft-name{max-width:480px;overflow:hidden;text-overflow:ellipsis}.ft-kind{display:inline-block;min-width:17px;margin-right:5px;opacity:.78}
.ft-size{text-align:right!important;font-family:ui-monospace,monospace}.ft-type{opacity:.72}.ft-modified{font-family:ui-monospace,monospace;font-size:10px}
.ft-empty{padding:20px;opacity:.55}
.ft-queue{height:170px;min-height:90px;display:flex;flex-direction:column;border-top:0;background:#0f1319}
.ft-queue-resizer{height:8px;min-height:8px;flex:0 0 8px;cursor:row-resize;touch-action:none;background:#171b22;border-top:1px solid #30343b;border-bottom:1px solid #30343b;position:relative}
.ft-queue-resizer::after{content:'';position:absolute;left:50%;top:3px;width:42px;height:2px;transform:translateX(-50%);border-radius:2px;background:#56606e;opacity:.75}
.ft-queue-resizer:hover,.ft-queue-resizer.dragging{background:#26303c}
.ft-queue-head{display:flex;align-items:center;gap:7px;padding:5px 8px;border-bottom:1px solid #30343b;background:#141920}
.ft-queue-title{font-size:11px;font-weight:700}.ft-queue-summary{font-size:10px;opacity:.68}.ft-queue-spacer{flex:1}
.ft-queue-head button{height:25px;padding:2px 7px;font-size:10px}
.ft-queue-connections{display:flex;align-items:center;gap:4px;font-size:10px;white-space:nowrap}.ft-queue-connections input{width:48px;height:25px;box-sizing:border-box;padding:2px 4px;background:#0d1117;color:inherit;border:1px solid #39414d;border-radius:4px;font-size:10px}
.ft-queue-wrap{flex:1;min-height:0;overflow:auto}
.ft-queue-table{width:100%;border-collapse:collapse;font-size:10px}
.ft-queue-table th,.ft-queue-table td{padding:5px 7px;border-bottom:1px solid #272d36;text-align:left;white-space:nowrap}
.ft-queue-table th{position:sticky;top:0;background:#171c23;z-index:2}
.ft-queue-table tbody tr{cursor:default;user-select:none;-moz-user-select:none;-webkit-user-select:none}
.ft-queue-table tbody tr:hover{background:#202731}.ft-queue-table tbody tr.selected{background:#29384b}
.ft-queue-table tbody tr:focus,.ft-queue-table tbody tr:focus-visible,.ft-queue-table tbody td:focus,.ft-queue-table tbody td:focus-visible{outline:none!important;box-shadow:none!important}
.ft-queue-select{width:26px;min-width:26px;text-align:center!important;padding-left:4px!important;padding-right:4px!important}
.ft-queue-select input{margin:0;vertical-align:middle}
.ft-queue-kind{font-weight:600;min-width:72px}
.ft-queue-path{max-width:420px;overflow:hidden;text-overflow:ellipsis}
.ft-queue-status{font-weight:600}.ft-queue-status.running{opacity:1}.ft-queue-status.success{opacity:.72}.ft-queue-status.failed{font-weight:700}
.ft-queue-error{max-width:440px;overflow:hidden;text-overflow:ellipsis;opacity:.82}
.ft-queue-empty{padding:14px;opacity:.5}
.ft-divider{position:relative;cursor:col-resize;background:#171b22;border-left:1px solid #30343b;border-right:1px solid #30343b;touch-action:none}
.ft-divider:hover,.ft-divider.dragging{background:#26303c}
.ft-transfer-tools{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);display:flex;flex-direction:column;gap:8px;z-index:4}
.ft-transfer-tools button{width:34px;height:34px;padding:0;border-radius:50%;font-size:17px;background:#202a36;box-shadow:0 2px 7px rgba(0,0,0,.35)}
.ft-transfer-tools button:disabled{opacity:.28}
.ft-sync-backdrop{position:fixed;inset:0;z-index:16500;display:flex;align-items:center;justify-content:center;padding:18px;background:rgba(0,0,0,.58)}
.ft-upload-choice-backdrop{position:fixed;inset:0;z-index:16600;display:flex;align-items:center;justify-content:center;padding:18px;background:rgba(0,0,0,.58)}
.ft-upload-choice{width:min(680px,calc(100vw - 36px));max-height:calc(100vh - 36px);overflow:auto;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 18px 52px rgba(0,0,0,.58);padding:16px}
.ft-upload-choice h3{margin:0 0 7px;font-size:15px}.ft-upload-choice p{margin:7px 0;font-size:11px;line-height:1.45}.ft-upload-choice-note{padding:9px;border:1px solid #343d49;border-radius:6px;background:#0d1218;font:10px/1.45 ui-monospace,monospace;white-space:pre-wrap}
.ft-upload-choice-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:14px;flex-wrap:wrap}.ft-upload-choice-actions button{padding:7px 10px}
.ft-upload-command{width:100%;min-height:76px;box-sizing:border-box;resize:vertical;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:8px;font:10px/1.45 ui-monospace,monospace}
html[data-taskmenu-theme="light"] .ft-upload-choice{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .ft-upload-choice-note,html[data-taskmenu-theme="light"] .ft-upload-command{background:#fff;color:#202124;border-color:#b9c0c8}

.ft-sync-dialog{width:min(980px,calc(100vw - 36px));max-height:calc(100vh - 36px);display:flex;flex-direction:column;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 18px 52px rgba(0,0,0,.58);padding:14px}
.ft-sync-dialog h3{margin:0 0 5px;font-size:14px}.ft-sync-note{font-size:10px;opacity:.72;margin-bottom:9px}
.ft-sync-summary{font-size:11px;margin-bottom:8px}.ft-sync-table-wrap{overflow:auto;min-height:160px;max-height:56vh;border:1px solid #303843;border-radius:6px}
.ft-sync-table{width:100%;border-collapse:collapse;font-size:10px}.ft-sync-table th,.ft-sync-table td{padding:5px 7px;border-bottom:1px solid #272d36;text-align:left;white-space:nowrap}
.ft-sync-table th{position:sticky;top:0;background:#171c23;z-index:2}.ft-sync-path{max-width:480px;overflow:hidden;text-overflow:ellipsis}
.ft-sync-actions{display:flex;flex-wrap:wrap;justify-content:flex-end;gap:7px;margin-top:11px}.ft-sync-actions button{padding:6px 10px}
.ft-sync-config{display:grid;grid-template-columns:160px 1fr;gap:8px 10px;align-items:center;margin:8px 0 10px}
.ft-sync-config label{font-size:10px;font-weight:700}.ft-sync-config select,.ft-sync-config input,.ft-sync-config textarea{width:100%;box-sizing:border-box;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:7px}
.ft-sync-config textarea{min-height:92px;resize:vertical;font:10px/1.4 ui-monospace,monospace}.ft-sync-config .ft-sync-check{display:flex;align-items:center;gap:7px;font-weight:500}.ft-sync-config .ft-sync-check input{width:auto}
.ft-sync-danger{background:#54252a;border-color:#7b3941}
html[data-taskmenu-theme="light"] .ft-sync-dialog{background:#fff;border-color:#b9c0c8;box-shadow:0 18px 52px rgba(0,0,0,.2)}
html[data-taskmenu-theme="light"] .ft-sync-config select,html[data-taskmenu-theme="light"] .ft-sync-config input,html[data-taskmenu-theme="light"] .ft-sync-config textarea{background:#fff;color:#202124;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-sync-table th{background:#e9eef3}.ft-sync-status{font-weight:700}
body.ft-resizing{user-select:none;cursor:col-resize}
body.ft-queue-resizing{user-select:none;cursor:row-resize}
.ft-context{position:fixed;z-index:16000;min-width:175px;padding:4px;background:#171b22;border:1px solid #48515f;border-radius:7px;box-shadow:0 14px 38px rgba(0,0,0,.45)}
.ft-context-title{padding:6px 9px 5px;font-size:10px;font-weight:700;opacity:.65;max-width:280px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.ft-context-separator{height:1px;margin:4px 3px;background:#303844}
.ft-context button{display:block;width:100%;text-align:left;border:0;background:transparent;color:inherit;padding:7px 9px;border-radius:4px}
.ft-context button:hover{background:#2b3440}.ft-context button.danger{color:#ff9a9a}
.ft-context button:disabled{opacity:.4;pointer-events:none}
.ft-select-cell{width:26px;min-width:26px;text-align:center!important;padding-left:5px!important;padding-right:3px!important}
.ft-select-cell input{margin:0;vertical-align:middle}
.ft-local-note{font-size:10px;opacity:.65}
html[data-taskmenu-theme="light"] .ft-pane{background:#fff}
html[data-taskmenu-theme="light"] .ft-site-head,html[data-taskmenu-theme="light"] .ft-pathbar{background:#f2f5f8;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-pathbar input[type=text],html[data-taskmenu-theme="light"] .ft-path-history-select{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-table th{background:#e9eef3}
html[data-taskmenu-theme="light"] .ft-table tbody tr:hover{background:#eef2f6}
html[data-taskmenu-theme="light"] .ft-table tbody tr.selected{background:#dde8f3}
html[data-taskmenu-theme="light"] .ft-context{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-queue{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-queue-resizer{background:#e3e8ed;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-queue-head,html[data-taskmenu-theme="light"] .ft-queue-table th{background:#edf1f5;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .ft-queue-table tbody tr:hover{background:#eef2f6}
html[data-taskmenu-theme="light"] .ft-queue-table tbody tr.selected{background:#dde8f3}
.ft-conflict-backdrop{position:fixed;inset:0;z-index:2600;background:rgba(0,0,0,.48);display:flex;align-items:center;justify-content:center;padding:18px}
.ft-conflict-dialog{width:min(620px,calc(100vw - 36px));max-height:calc(100vh - 36px);overflow:auto;background:#11161d;border:1px solid #46505d;border-radius:10px;box-shadow:0 18px 52px rgba(0,0,0,.58);padding:14px}
.ft-conflict-dialog h3{margin:0 0 9px;font-size:14px}.ft-conflict-note{font-size:10px;opacity:.7;margin-bottom:10px}
.ft-conflict-path{font:10px/1.45 ui-monospace,monospace;word-break:break-all;background:#0b0f14;border:1px solid #303843;border-radius:6px;padding:6px;margin:4px 0 8px}
.ft-conflict-meta{display:grid;grid-template-columns:110px 1fr 1fr;gap:4px 8px;font-size:10px;margin:8px 0 12px}.ft-conflict-meta strong{font-weight:700}
.ft-conflict-controls{display:grid;grid-template-columns:1fr 1fr;gap:8px}.ft-conflict-controls label{display:flex;flex-direction:column;gap:4px;font-size:10px;font-weight:700}
.ft-conflict-controls select{background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:7px}
.ft-conflict-actions{display:flex;justify-content:flex-end;gap:7px;margin-top:12px}.ft-conflict-actions button{padding:6px 11px}
.ft-queue-status.conflict{font-weight:700}.ft-queue-status.skipped{opacity:.65}
html[data-taskmenu-theme="light"] .ft-conflict-dialog{background:#fff;border-color:#b9c0c8;box-shadow:0 18px 52px rgba(0,0,0,.2)}
html[data-taskmenu-theme="light"] .ft-conflict-path{background:#f6f8fa;border-color:#d0d7de}
html[data-taskmenu-theme="light"] .ft-conflict-controls select{background:#fff;color:#202124;border-color:#b9c0c8}
@media(max-width:850px){.ft-sites{grid-template-columns:1fr;grid-template-rows:minmax(220px,1fr) 8px minmax(220px,1fr)}.ft-divider{cursor:row-resize;border-left:0;border-right:0;border-top:1px solid #30343b;border-bottom:1px solid #30343b}.ft-transfer-tools{flex-direction:row}.ft-transfer-tools button:first-child{transform:rotate(90deg)}.ft-transfer-tools button:last-child{transform:rotate(90deg)}}
`;
document.head.append(style);

function workspaceKey(){return String(app.taskData?.workspace||'workspace');}
function safeStorageGet(key,fallback=''){try{return localStorage.getItem(key)??fallback;}catch{return fallback;}}
function safeStorageSet(key,value){try{localStorage.setItem(key,value);}catch(error){console.warn('Cannot persist file-transfer setting',error);}}

const syncCompareModes=new Set(['metadata','checksum']);
const syncDirections=new Set(['right','left','bidirectional','mirror_right','mirror_left']);
const maxSyncProfiles=32;
const maxSyncExcludePatterns=32;
function newSyncProfileID(){return globalThis.crypto?.randomUUID?.()||('sync-'+Date.now().toString(36)+'-'+Math.random().toString(36).slice(2,10));}
function normalizeSyncExclude(value){
  const source=Array.isArray(value)?value:String(value||'').split(/\r?\n|,/);
  const out=[];
  for(const raw of source){
    let item=String(raw||'').trim().replaceAll('\\','/').replace(/^\.\//,'');
    if(!item||item==='.')continue;
    if(item.length>256)throw new Error('Sync exclude pattern exceeds 256 characters');
    if(!out.includes(item))out.push(item);
    if(out.length>maxSyncExcludePatterns)throw new Error('Sync supports at most '+maxSyncExcludePatterns+' exclude patterns');
  }
  return out;
}
function normalizeSyncProfile(raw,view=null){
  raw=raw&&typeof raw==='object'?raw:{};
  const direction=syncDirections.has(String(raw.direction||''))?String(raw.direction):'right';
  const compareMode=syncCompareModes.has(String(raw.compare_mode||''))?String(raw.compare_mode):'metadata';
  return {
    id:String(raw.id||''),name:String(raw.name||'').trim().slice(0,80),
    profile_id:String(raw.profile_id||view?.profile?.id||''),
    left_source:String(raw.left_source||view?.left?.source||'host')==='local'?'local':'host',
    local_root_id:String(raw.local_root_id||view?.left?.localRoot?.id||''),
    left_path:normalizeRelativePath(raw.left_path||view?.left?.currentPath||'.'),
    remote_path:normalizeRemotePath(raw.remote_path||view?.remote?.currentPath||'.'),
    compare_mode:compareMode,direction,exclude:normalizeSyncExclude(raw.exclude||[]),
    allow_delete:Boolean(raw.allow_delete)
  };
}
function syncProfilesKey(view){return 'taskdeck:file-transfer:sync-profiles:'+workspaceKey()+':'+String(view?.profile?.id||'');}
function readSyncProfiles(view){
  try{
    const raw=JSON.parse(safeStorageGet(syncProfilesKey(view),'null'));
    if(!raw||raw.version!==1||!Array.isArray(raw.profiles))return [];
    return raw.profiles.slice(0,maxSyncProfiles).map(item=>normalizeSyncProfile(item,view)).filter(item=>item.id&&item.name&&item.profile_id===String(view.profile.id));
  }catch{return [];}
}
function writeSyncProfiles(view,profiles){
  const clean=(Array.isArray(profiles)?profiles:[]).slice(0,maxSyncProfiles).map(item=>normalizeSyncProfile(item,view)).filter(item=>item.id&&item.name);
  safeStorageSet(syncProfilesKey(view),JSON.stringify({version:1,profiles:clean}));
  return clean;
}
function saveSyncProfile(view,options,name,id=''){
  name=String(name||'').trim();if(!name)throw new Error('Sync profile name is required');
  const profile=normalizeSyncProfile({...options,id:id||newSyncProfileID(),name,profile_id:view.profile.id},view);
  const all=readSyncProfiles(view).filter(item=>item.id!==profile.id&&item.name.toLowerCase()!==profile.name.toLowerCase());
  all.push(profile);writeSyncProfiles(view,all);return profile;
}
function deleteSyncProfile(view,id){writeSyncProfiles(view,readSyncProfiles(view).filter(item=>item.id!==String(id||'')));}
function syncGlobRegex(pattern){
  pattern=String(pattern||'').replaceAll('\\','/').replace(/^\.\//,'');
  let out='^';
  for(let i=0;i<pattern.length;i++){
    const ch=pattern[i];
    if(ch==='*'){
      if(pattern[i+1]==='*'){
        i++;
        if(pattern[i+1]==='/'){i++;out+='(?:.*/)?';}else out+='.*';
      }else out+='[^/]*';
    }else if(ch==='?')out+='[^/]';
    else out+=/[\\^$+?.()|{}\[\]]/.test(ch)?'\\'+ch:ch;
  }
  return new RegExp(out+'$');
}
function syncPathExcluded(path,patterns){
  path=normalizeRelativePath(path||'.');
  for(const raw of normalizeSyncExclude(patterns)){
    const pattern=raw.replace(/\/$/,'');
    if(pattern.endsWith('/**')){
      const base=pattern.slice(0,-3).replace(/\/$/,'');
      if(path===base||path.startsWith(base+'/'))return true;
    }
    try{if(syncGlobRegex(pattern).test(path))return true;}catch{}
  }
  return false;
}
const conflictPolicies=new Set(['ask','overwrite','skip','size_diff','source_newer','checksum_diff']);
let conflictDialogChain=Promise.resolve();
const activeServerConflictViews=new Set();
function normalizeConflictPolicy(value){value=String(value||'ask').trim().toLowerCase();return conflictPolicies.has(value)?value:'ask';}
function conflictDirectionFromKind(kind){return String(kind||'').toLowerCase()==='download'?'download':'upload';}
function conflictDefaultKey(view,direction){return 'taskdeck:file-transfer:conflict-default:'+workspaceKey()+':'+view.profile.id+':'+direction;}
function persistentConflictPolicy(view,direction){return normalizeConflictPolicy(safeStorageGet(conflictDefaultKey(view,direction),'ask'));}
function rememberConflictPolicy(view,direction,policy){safeStorageSet(conflictDefaultKey(view,direction),normalizeConflictPolicy(policy));}
function conflictJobPoliciesKey(){return 'taskdeck:file-transfer:conflict-jobs:'+workspaceKey();}
function readConflictJobPolicies(){
  try{
    const raw=JSON.parse(safeStorageGet(conflictJobPoliciesKey(),'{}')),now=Date.now(),out={};
    for(const [id,value] of Object.entries(raw||{})){
      const policy=normalizeConflictPolicy(value?.policy),ts=Number(value?.ts)||0;
      if(id&&policy!=='ask'&&now-ts<7*24*60*60*1000)out[id]={policy,ts};
    }
    return out;
  }catch{return {};}
}
function localJobConflictPolicy(jobID){const item=readConflictJobPolicies()[String(jobID||'')];return item?normalizeConflictPolicy(item.policy):'ask';}
function setLocalJobConflictPolicy(jobID,policy){
  jobID=String(jobID||'');if(!jobID)return;
  const all=readConflictJobPolicies();all[jobID]={policy:normalizeConflictPolicy(policy),ts:Date.now()};
  const entries=Object.entries(all).sort((a,b)=>(b[1].ts||0)-(a[1].ts||0)).slice(0,128);
  safeStorageSet(conflictJobPoliciesKey(),JSON.stringify(Object.fromEntries(entries)));
}
function effectiveDirectionConflictPolicy(view,direction){
  const session=normalizeConflictPolicy(view?.conflictSessionPolicies?.[direction]);
  if(session!=='ask')return session;
  return persistentConflictPolicy(view,direction);
}
function effectiveLocalConflictPolicy(view,direction,item=null){
  const own=normalizeConflictPolicy(item?.conflictPolicy);
  if(own!=='ask')return own;
  const job=localJobConflictPolicy(item?.conflictJobID);
  if(job!=='ask')return job;
  return effectiveDirectionConflictPolicy(view,direction);
}
function parseConflictTime(value){
  value=String(value||'').trim();if(!value)return NaN;
  const direct=Date.parse(value);if(Number.isFinite(direct))return direct;
  const match=/^([A-Za-z]{3})\s+(\d{1,2})\s+(\d{1,2}):(\d{2})$/.exec(value);
  if(!match)return NaN;
  const months={Jan:0,Feb:1,Mar:2,Apr:3,May:4,Jun:5,Jul:6,Aug:7,Sep:8,Oct:9,Nov:10,Dec:11},month=months[match[1]];
  if(month===undefined)return NaN;
  const now=new Date(),date=new Date(Date.UTC(now.getUTCFullYear(),month,Number(match[2]),Number(match[3]),Number(match[4])));
  if(date.getTime()>Date.now()+24*60*60*1000)date.setUTCFullYear(date.getUTCFullYear()-1);
  return date.getTime();
}
function conflictMetaText(value,kind){if(kind==='size')return formatSize(Number(value)||0);return value?formatModified(value):'Unavailable';}
function requestTransferConflictDecision(conflict){
  const run=()=>new Promise(resolve=>{
    if(typeof conflict?.stillCurrent==='function'&&!conflict.stillCurrent()){resolve(null);return;}
    const direction=conflictDirectionFromKind(conflict.kind),backdrop=document.createElement('div');backdrop.className='ft-conflict-backdrop';
    const dialog=document.createElement('div');dialog.className='ft-conflict-dialog';
    const title=document.createElement('h3');title.textContent=(direction==='upload'?'Upload':'Download')+' file conflict';
    const note=document.createElement('div');note.className='ft-conflict-note';note.textContent='Folder collisions are reused automatically. File collisions require a policy. SHA-256 reads the complete source and destination and can be slower.';
    const sourceLabel=document.createElement('strong');sourceLabel.textContent='Source';
    const sourcePath=document.createElement('div');sourcePath.className='ft-conflict-path';sourcePath.textContent=String(conflict.source||'');
    const targetLabel=document.createElement('strong');targetLabel.textContent='Destination';
    const targetPath=document.createElement('div');targetPath.className='ft-conflict-path';targetPath.textContent=String(conflict.target||'');
    const meta=document.createElement('div');meta.className='ft-conflict-meta';
    for(const row of [
      ['','Source','Destination'],
      ['Size',conflictMetaText(conflict.source_size,'size'),conflictMetaText(conflict.target_size,'size')],
      ['Modified',conflictMetaText(conflict.source_modified,'modified'),conflictMetaText(conflict.target_modified,'modified')]
    ])for(const value of row){const cell=document.createElement(row[0]===''?'strong':'span');cell.textContent=value;meta.append(cell);}
    const controls=document.createElement('div');controls.className='ft-conflict-controls';
    const policyLabel=document.createElement('label');policyLabel.textContent='Action / comparison';
    const policy=document.createElement('select');
    for(const [value,label] of [
      ['overwrite','Overwrite destination'],
      ['skip','Skip source file'],
      ['size_diff','Overwrite only if size differs'],
      ['source_newer','Overwrite only if source Modified time is newer'],
      ['checksum_diff','Overwrite only if SHA-256 differs']
    ]){const option=document.createElement('option');option.value=value;option.textContent=label;policy.append(option);}
    policyLabel.append(policy);
    const scopeLabel=document.createElement('label');scopeLabel.textContent='Apply to';
    const scope=document.createElement('select');
    const word=direction==='upload'?'uploads':'downloads';
    for(const [value,label] of [
      ['item','This file only'],
      ['job','This transfer only'],
      ['direction_session','All '+word+' in this TaskDeck session'],
      ['direction_always','Always for '+word+' (remember)']
    ]){const option=document.createElement('option');option.value=value;option.textContent=label;scope.append(option);}
    scopeLabel.append(scope);controls.append(policyLabel,scopeLabel);
    const actions=document.createElement('div');actions.className='ft-conflict-actions';
    const apply=document.createElement('button');apply.type='button';apply.textContent='Apply';
    apply.onclick=()=>{backdrop.remove();resolve({policy:policy.value,scope:scope.value,direction});};
    actions.append(apply);
    dialog.append(title,note,sourceLabel,sourcePath,targetLabel,targetPath,meta,controls,actions);backdrop.append(dialog);document.body.append(backdrop);policy.focus();
  });
  const promise=conflictDialogChain.then(run,run);conflictDialogChain=promise.catch(()=>{});return promise;
}
async function hostFileSHA256(path){
  const data=await app.jsonFetch('/api/file-transfer/host-hash',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({path})});
  return String(data?.sha256||'');
}
async function remoteFileSHA256(view,path){
  const data=await app.jsonFetch('/api/file-transfer/hash',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:view.profile.id,path})});
  return String(data?.sha256||'');
}
async function browserFileSHA256(file){
  const form=new FormData();form.append('file',file,file.name||'file');
  const response=await app.fetchWithLease('/api/file-transfer/hash-upload',{method:'POST',body:form,cache:'no-store'});
  if(!response.ok)throw new Error((await response.text()).trim()||response.statusText);
  const data=await response.json();return String(data?.sha256||'');
}
async function evaluateBrowserConflictPolicy(view,policy,conflict,sourceHash,targetHash){
  policy=normalizeConflictPolicy(policy);
  if(policy==='overwrite')return true;
  if(policy==='skip')return false;
  if(policy==='size_diff')return Number(conflict.source_size)!==Number(conflict.target_size);
  if(policy==='source_newer'){
    const source=parseConflictTime(conflict.source_modified),target=parseConflictTime(conflict.target_modified);
    if(!Number.isFinite(source)||!Number.isFinite(target))throw new Error('Modified time is unavailable for source-newer comparison');
    return source>target;
  }
  if(policy==='checksum_diff')return (await sourceHash())!==(await targetHash());
  throw new Error('File conflict requires a user decision');
}

function fileTransferWorkspaceID(){return String(app.taskData?.workspace||'').trim();}
function fileTransferSessionKey(){
  const workspace=fileTransferWorkspaceID();
  return workspace?'taskdeck:file-transfer:session:'+workspace:'';
}
function readFileTransferSession(){
  const key=fileTransferSessionKey();if(!key)return null;
  try{
    const raw=JSON.parse(localStorage.getItem(key)||'null');
    if(!raw||raw.version!==1||!Array.isArray(raw.open))return null;
    return {
      version:1,
      active_profile_id:String(raw.active_profile_id||''),
      open:raw.open.map(item=>({
        profile_id:String(item?.profile_id||''),
        left_path:normalizeRelativePath(item?.left_path||'.'),
        remote_path:normalizeRemotePath(item?.remote_path||'.'),
        queue_paused:Boolean(item?.queue_paused)
      })).filter(item=>item.profile_id)
    };
  }catch{return null;}
}
function persistFileTransferSession(){
  if(restoringSession||!sessionPersistenceReady)return;
  const key=fileTransferSessionKey();if(!key)return;
  const open=[];
  for(const [id,view] of views){
    open.push({
      profile_id:id,
      left_path:normalizeRelativePath(view.left?.currentPath||'.'),
      remote_path:normalizeRemotePath(view.remote?.currentPath||view.profile?.initial_path||'.'),
      queue_paused:Boolean(view.transferQueue?.paused)
    });
  }
  try{
    if(!open.length){localStorage.removeItem(key);return;}
    localStorage.setItem(key,JSON.stringify({version:1,active_profile_id:activeViewID,open}));
  }catch(error){console.warn('Cannot persist file-transfer session',error);}
}
function fileTransferJobsKey(){
  const workspace=fileTransferWorkspaceID();
  return workspace?'taskdeck:file-transfer:jobs:'+workspace:'';
}
function readPersistentFileTransferJobs(){
  const key=fileTransferJobsKey();if(!key)return [];
  try{
    const raw=JSON.parse(localStorage.getItem(key)||'null');
    if(!raw||raw.version!==1||!Array.isArray(raw.jobs))return [];
    return raw.jobs.map(job=>({
      id:String(job?.id||''),
      kind:String(job?.kind||''),
      profile_id:String(job?.profile_id||''),
      created_at:Number(job?.created_at)||0,
      targets:(Array.isArray(job?.targets)?job.targets:[]).map(target=>({
        path:normalizeRemotePath(target?.path||'.'),
        directory:Boolean(target?.directory)
      })).filter(target=>target.path!=='.'&&target.path!=='/')
    })).filter(job=>job.id&&job.kind==='remote_delete'&&job.profile_id&&job.targets.length);
  }catch{return [];}
}
function writePersistentFileTransferJobs(jobs){
  const key=fileTransferJobsKey();if(!key)return;
  try{
    const clean=Array.isArray(jobs)?jobs:[];
    if(!clean.length){localStorage.removeItem(key);return;}
    localStorage.setItem(key,JSON.stringify({version:1,jobs:clean}));
  }catch(error){console.warn('Cannot persist file-transfer jobs',error);}
}
function upsertPersistentFileTransferJob(job){
  const jobs=readPersistentFileTransferJobs().filter(item=>item.id!==job.id);
  jobs.push(job);writePersistentFileTransferJobs(jobs);
}
function removePersistentFileTransferJob(jobID){
  writePersistentFileTransferJobs(readPersistentFileTransferJobs().filter(job=>job.id!==jobID));
}
function clearPersistentFileTransferJobsForProfile(profileID){
  profileID=String(profileID||'');
  writePersistentFileTransferJobs(readPersistentFileTransferJobs().filter(job=>String(job.profile_id||'')!==profileID));
}
function newFileTransferJobID(){
  if(globalThis.crypto?.randomUUID)return globalThis.crypto.randomUUID();
  return 'ft-'+Date.now().toString(36)+'-'+Math.random().toString(36).slice(2,10);
}
function localTransferQueueKey(){
  const workspace=fileTransferWorkspaceID();
  return workspace?'taskdeck:file-transfer:local-queue:'+workspace:'';
}
function localTransferScansKey(){
  const workspace=fileTransferWorkspaceID();
  return workspace?'taskdeck:file-transfer:local-scans:'+workspace:'';
}
function readPersistentLocalTransferItems(){
  const key=localTransferQueueKey();if(!key)return [];
  try{
    const raw=JSON.parse(localStorage.getItem(key)||'null');
    return raw?.version===1&&Array.isArray(raw.items)?raw.items:[];
  }catch{return [];}
}
function writePersistentLocalTransferItems(items){
  const key=localTransferQueueKey();if(!key)return;
  try{
    const clean=Array.isArray(items)?items:[];
    if(!clean.length){localStorage.removeItem(key);return;}
    localStorage.setItem(key,JSON.stringify({version:1,items:clean}));
  }catch(error){console.warn('Cannot persist local-browser transfer queue',error);}
}
function readPersistentLocalScans(){
  const key=localTransferScansKey();if(!key)return [];
  try{
    const raw=JSON.parse(localStorage.getItem(key)||'null');
    return raw?.version===1&&Array.isArray(raw.scans)?raw.scans:[];
  }catch{return [];}
}
function writePersistentLocalScans(scans){
  const key=localTransferScansKey();if(!key)return;
  try{
    const clean=Array.isArray(scans)?scans:[];
    if(!clean.length){localStorage.removeItem(key);return;}
    localStorage.setItem(key,JSON.stringify({version:1,scans:clean}));
  }catch(error){console.warn('Cannot persist local-browser transfer scans',error);}
}
function upsertPersistentLocalScan(scan){
  const scans=readPersistentLocalScans().filter(item=>item.id!==scan.id);
  scans.push(scan);writePersistentLocalScans(scans);
}
function removePersistentLocalScan(scanID){
  writePersistentLocalScans(readPersistentLocalScans().filter(item=>item.id!==scanID));
}
function clearPersistentLocalScansForProfile(profileID){
  profileID=String(profileID||'');
  writePersistentLocalScans(readPersistentLocalScans().filter(item=>String(item.profile_id||'')!==profileID));
}
function localPersistentKey(spec){
  if(!spec)return '';
  if(spec.kind==='local_upload')return 'upload:'+spec.root_id+':'+spec.source_path+'=>'+spec.profile_id+':'+spec.target_path;
  if(spec.kind==='local_download')return 'download:'+spec.profile_id+':'+spec.remote_path+'=>'+spec.root_id+':'+spec.left_path;
  return '';
}
function persistLocalTransferQueue(view){
  const queue=view?.transferQueue;if(!queue)return;
  const others=readPersistentLocalTransferItems().filter(item=>item.profile_id!==String(view.profile.id||''));
  const current=queue.items.filter(item=>!item.server&&item.persistSpec).map(item=>({
    id:String(item.id),profile_id:String(view.profile.id||''),kind:item.kind,direction:item.direction,
    source:item.source,target:item.target,size:Number(item.size)||0,
    status:item.status==='running'?'queued':item.status,error:String(item.error||''),
    conflict_policy:normalizeConflictPolicy(item.conflictPolicy),conflict_job_id:String(item.conflictJobID||''),
    spec:item.persistSpec
  }));
  writePersistentLocalTransferItems(others.concat(current));
}
function normalizeRemotePath(value){
  value=String(value||'.').trim()||'.';
  if(value==='.')return '.';
  const absolute=value.startsWith('/');
  const parts=[];
  for(const item of value.replace(/\\/g,'/').split('/')){
    if(!item||item==='.')continue;
    if(item==='..'){if(parts.length)parts.pop();continue;}
    parts.push(item);
  }
  const joined=parts.join('/');
  if(absolute)return '/'+joined;
  return joined||'.';
}

function normalizeRelativePath(value){
  value=String(value||'.').trim()||'.';
  if(value==='.')return '.';
  const parts=[];
  for(const item of value.replace(/\\/g,'/').split('/')){
    if(!item||item==='.')continue;
    if(item==='..'){if(parts.length)parts.pop();continue;}
    parts.push(item);
  }
  return parts.join('/')||'.';
}

function joinPath(parent,name,remote=false){
  parent=remote?normalizeRemotePath(parent):normalizeRelativePath(parent);
  name=String(name||'').replace(/^[/\\]+/,'');
  if(parent==='.')return name||'.';
  if(remote&&parent==='/')return '/'+name;
  return parent.replace(/\/$/,'')+'/'+name;
}

function parentPath(value,remote=false){
  value=remote?normalizeRemotePath(value):normalizeRelativePath(value);
  if(value==='.'||(remote&&value==='/'))return value;
  const absolute=remote&&value.startsWith('/');
  const parts=value.split('/').filter(Boolean);parts.pop();
  if(!parts.length)return absolute?'/':'.';
  return (absolute?'/':'')+parts.join('/');
}
function canGoParentPath(value,remote=false){
  const current=remote?normalizeRemotePath(value):normalizeRelativePath(value);
  return parentPath(current,remote)!==current;
}

function formatSize(value){
  let n=Number(value)||0;if(n<1024)return n+' B';
  const units=['KiB','MiB','GiB','TiB'];let i=-1;
  do{n/=1024;i++;}while(n>=1024&&i<units.length-1);
  return n.toFixed(n>=10?1:2)+' '+units[i];
}

function formatModified(value){
  if(!value)return '';
  const date=new Date(value);
  if(Number.isNaN(date.getTime()))return String(value);
  return new Intl.DateTimeFormat(undefined,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(date);
}

function entryType(entry){
  const raw=String(entry?.type||'file').toLowerCase();
  if(raw==='dir')return 'directory';
  return raw;
}

function typeRank(type){
  switch(entryType({type})){case'directory':return 0;case'file':return 1;case'symlink':return 2;default:return 3;}
}

function typeLabel(type){
  switch(entryType({type})){case'directory':return 'Folder';case'file':return 'File';case'symlink':return 'Link';default:return String(type||'Other');}
}

function sortEntries(entries,sort){
  const copy=[...(entries||[])];
  copy.sort((a,b)=>{
    const at=typeRank(a.type),bt=typeRank(b.type);
    if(sort.key==='type'&&at!==bt)return (at-bt)*(sort.direction==='asc'?1:-1);
    if(sort.key!=='type'&&at!==bt)return at-bt;
    let av,bv;
    switch(sort.key){
      case'size':av=Number(a.size)||0;bv=Number(b.size)||0;break;
      case'modified':av=Date.parse(a.modified||'')||0;bv=Date.parse(b.modified||'')||0;break;
      case'type':av=typeLabel(a.type);bv=typeLabel(b.type);break;
      default:av=String(a.name||'').toLocaleLowerCase();bv=String(b.name||'').toLocaleLowerCase();
    }
    let result=typeof av==='number'?(av-bv):String(av).localeCompare(String(bv),undefined,{numeric:true,sensitivity:'base'});
    if(result===0)result=String(a.name||'').localeCompare(String(b.name||''),undefined,{numeric:true,sensitivity:'base'});
    return result*(sort.direction==='asc'?1:-1);
  });
  return copy;
}

function pathMemoryKey(scope){return 'taskdeck:file-transfer:paths:v2:'+workspaceKey()+':'+scope;}
function readPathMemory(scope){
  try{
    const parsed=JSON.parse(safeStorageGet(pathMemoryKey(scope),'{}'));
    return {
      favorites:Array.isArray(parsed?.favorites)?parsed.favorites.filter(Boolean).slice(0,maxFavoritePaths):[],
      recent:Array.isArray(parsed?.recent)?parsed.recent.filter(Boolean).slice(0,maxRecentPaths):[]
    };
  }catch{return {favorites:[],recent:[]};}
}
function writePathMemory(scope,memory){safeStorageSet(pathMemoryKey(scope),JSON.stringify(memory));}
function rememberPath(scope,path){
  if(!scope||!path)return;
  const memory=readPathMemory(scope);
  memory.recent=[path,...memory.recent.filter(item=>item!==path)].slice(0,maxRecentPaths);
  writePathMemory(scope,memory);
}
function toggleFavorite(scope,path){
  const memory=readPathMemory(scope);
  const index=memory.favorites.indexOf(path);
  if(index>=0)memory.favorites.splice(index,1);
  else memory.favorites=[path,...memory.favorites.filter(item=>item!==path)].slice(0,maxFavoritePaths);
  writePathMemory(scope,memory);
  return index<0;
}
function populatePathMemory(select,star,scope,current){
  const memory=readPathMemory(scope);
  select.replaceChildren();
  const placeholder=document.createElement('option');placeholder.value='';placeholder.textContent='▾';select.append(placeholder);
  const addGroup=(label,values)=>{
    if(!values.length)return;
    const group=document.createElement('optgroup');group.label=label;
    for(const value of values){const option=document.createElement('option');option.value=value;option.textContent=value;group.append(option);}
    select.append(group);
  };
  addGroup('Recent paths',memory.recent);
  addGroup('Favorites',memory.favorites.filter(item=>!memory.recent.includes(item)));
  select.value='';
  select.title=memory.recent.length?'Visited paths · '+memory.recent.length+' remembered':'Visited paths';
  const saved=memory.favorites.includes(current);
  star.classList.toggle('saved',saved);star.textContent=saved?'★':'☆';
  star.title=saved?'Remove current path from favorites':'Pin current path as favorite';
}

function openLocalDB(){
  return new Promise((resolve,reject)=>{
    if(!globalThis.indexedDB){reject(new Error('IndexedDB is unavailable'));return;}
    const request=indexedDB.open(localDBName,localDBVersion);
    request.onupgradeneeded=()=>{const db=request.result;if(!db.objectStoreNames.contains(localDBStore))db.createObjectStore(localDBStore,{keyPath:'id'});};
    request.onsuccess=()=>resolve(request.result);request.onerror=()=>reject(request.error||new Error('Cannot open local directory store'));
  });
}
async function localRootRecords(){
  const db=await openLocalDB();
  return new Promise((resolve,reject)=>{
    const tx=db.transaction(localDBStore,'readonly'),req=tx.objectStore(localDBStore).getAll();
    req.onsuccess=()=>resolve((req.result||[]).sort((a,b)=>(Number(b.lastUsed)||0)-(Number(a.lastUsed)||0)));
    req.onerror=()=>reject(req.error);tx.oncomplete=()=>db.close();
  });
}
async function putLocalRoot(record){
  const db=await openLocalDB();
  return new Promise((resolve,reject)=>{
    const tx=db.transaction(localDBStore,'readwrite');tx.objectStore(localDBStore).put(record);
    tx.oncomplete=()=>{db.close();resolve(record);};tx.onerror=()=>{const err=tx.error;db.close();reject(err);};
  });
}
async function getLocalRoot(id){
  const db=await openLocalDB();
  return new Promise((resolve,reject)=>{
    const tx=db.transaction(localDBStore,'readonly'),req=tx.objectStore(localDBStore).get(id);
    req.onsuccess=()=>resolve(req.result||null);req.onerror=()=>reject(req.error);tx.oncomplete=()=>db.close();
  });
}
function localRootID(){return globalThis.crypto?.randomUUID?.()||('root-'+Date.now()+'-'+Math.random().toString(16).slice(2));}
async function ensureHandlePermission(handle){
  if(!handle)return false;
  const opts={mode:'readwrite'};
  if(typeof handle.queryPermission==='function')return (await handle.queryPermission(opts))==='granted';
  return false;
}
async function requestHandlePermission(handle){
  if(!handle)return false;
  const opts={mode:'readwrite'};
  if(typeof handle.requestPermission==='function')return (await handle.requestPermission(opts))==='granted';
  return ensureHandlePermission(handle);
}
async function directoryHandleForPath(rootHandle,path){
  let handle=rootHandle;
  const normalized=normalizeRelativePath(path);
  if(normalized==='.')return handle;
  for(const part of normalized.split('/'))handle=await handle.getDirectoryHandle(part);
  return handle;
}

async function remoteEntryAtPath(view,path){
  path=normalizeRemotePath(path);
  const parent=parentPath(path,true),name=pathLeaf(path,true);
  const listing=await fetchRemoteDirectory(view,parent,{force:true});
  return listing.entries.find(entry=>String(entry.name||'')===name)||null;
}
async function localBrowserExistingFile(root,leftPath){
  leftPath=normalizeRelativePath(leftPath);
  const parent=parentPath(leftPath,false),name=pathLeaf(leftPath,false);
  let dir=root.handle;
  if(parent!=='.'){
    try{for(const part of parent.split('/').filter(Boolean))dir=await dir.getDirectoryHandle(part);}
    catch(error){if(error?.name==='NotFoundError')return null;throw error;}
  }
  try{
    const handle=await dir.getFileHandle(name),file=await handle.getFile();
    return {handle,file};
  }catch(error){if(error?.name==='NotFoundError')return null;throw error;}
}
function applyLocalConflictScope(view,item,decision){
  const direction=decision.direction||conflictDirectionFromKind(item.kind);
  const policy=normalizeConflictPolicy(decision.policy);
  item.conflictPolicy=policy;
  if(decision.scope==='job'&&item.conflictJobID){
    setLocalJobConflictPolicy(item.conflictJobID,policy);
    for(const candidate of view.transferQueue?.items||[])if(candidate.conflictJobID===item.conflictJobID)candidate.conflictPolicy=policy;
    const scans=readPersistentLocalScans();
    let changed=false;
    for(const scan of scans)if(String(scan.id||'')===String(item.conflictJobID)){scan.conflict_policy=policy;changed=true;}
    if(changed)writePersistentLocalScans(scans);
  }
  if(decision.scope==='direction_session'||decision.scope==='direction_always'){
    view.conflictSessionPolicies[direction]=policy;
    if(decision.scope==='direction_always')rememberConflictPolicy(view,direction,policy);
  }
  persistLocalTransferQueue(view);
}
async function resolveLocalBrowserConflict(view,item,conflict,sourceHash,targetHash){
  const direction=conflictDirectionFromKind(item.kind);
  let policy=effectiveLocalConflictPolicy(view,direction,item);
  if(policy==='ask'){
    const decision=await requestTransferConflictDecision({...conflict,kind:item.kind,source:item.source,target:item.target});
    applyLocalConflictScope(view,item,decision);policy=normalizeConflictPolicy(decision.policy);
  }
  try{return await evaluateBrowserConflictPolicy(view,policy,conflict,sourceHash,targetHash);}
  catch(error){
    app.showError(error);
    const decision=await requestTransferConflictDecision({...conflict,kind:item.kind,source:item.source,target:item.target});
    item.conflictPolicy='ask';applyLocalConflictScope(view,item,decision);
    return evaluateBrowserConflictPolicy(view,decision.policy,conflict,sourceHash,targetHash);
  }
}

async function persistentLocalTransferRun(view,spec,item=null){
  const root=await getLocalRoot(String(spec?.root_id||''));
  if(!root?.handle)throw new Error('Saved Local folder is unavailable. Choose the folder again, then Retry failed.');
  const granted=await ensureHandlePermission(root.handle);
  if(!granted)throw new Error('Local folder permission is required after reload. Click Grant, then Retry failed.');
  if(spec.kind==='local_upload'){
    const sourcePath=normalizeRelativePath(spec.source_path||'.');
    const parent=parentPath(sourcePath,false),name=pathLeaf(sourcePath,false);
    const dir=await directoryHandleForPath(root.handle,parent);
    const handle=await dir.getFileHandle(name),file=await handle.getFile();
    if(item){item.size=file.size;scheduleTransferQueueRender(view);}
    const targetPath=normalizeRemotePath(spec.target_path),existing=await remoteEntryAtPath(view,targetPath);
    if(existing){
      if(entryType(existing)!=='file')throw new Error('Remote destination exists and is not a file: '+targetPath);
      const conflict={
        source_size:file.size,target_size:Number(existing.size)||0,
        source_modified:new Date(file.lastModified).toISOString(),target_modified:String(existing.modified||'')
      };
      const overwrite=await resolveLocalBrowserConflict(view,item,conflict,()=>browserFileSHA256(file),()=>remoteFileSHA256(view,targetPath));
      if(!overwrite)return {skipped:true};
    }
    await uploadBrowserFileToPath(view,file,targetPath);
    markRemoteQueueDirty(view,parentPath(targetPath,true));return {skipped:false};
  }
  if(spec.kind==='local_download'){
    const remotePath=normalizeRemotePath(spec.remote_path),leftPath=normalizeRelativePath(spec.left_path);
    let sourceMeta={size:Number(spec.remote_size)||0,modified:String(spec.remote_modified||'')};
    if(!sourceMeta.size&&!sourceMeta.modified){
      const remote=await remoteEntryAtPath(view,remotePath);
      if(!remote)throw new Error('Remote source not found: '+remotePath);
      sourceMeta={size:Number(remote.size)||0,modified:String(remote.modified||'')};
    }
    const existing=await localBrowserExistingFile(root,leftPath);
    let overwrite=false;
    if(existing){
      const conflict={
        source_size:sourceMeta.size,target_size:existing.file.size,
        source_modified:sourceMeta.modified,target_modified:new Date(existing.file.lastModified).toISOString()
      };
      overwrite=await resolveLocalBrowserConflict(view,item,conflict,()=>remoteFileSHA256(view,remotePath),()=>browserFileSHA256(existing.file));
      if(!overwrite)return {skipped:true};
    }
    await writeRemotePathToLocalRoot(view,root,remotePath,leftPath,overwrite);
    markLeftQueueDirty(view);return {skipped:false};
  }
  throw new Error('Unsupported persisted Local transfer');
}
function localTransferTaskFromSaved(view,saved){
  const spec=saved?.spec||null,status=saved?.status==='running'?'queued':String(saved?.status||'queued');
  const runnable=status==='queued'||status==='failed';
  return {
    id:String(saved?.id||newFileTransferJobID()),status,error:String(saved?.error||''),
    direction:String(saved?.direction||''),kind:String(saved?.kind||'Transfer'),
    source:String(saved?.source||''),target:String(saved?.target||''),size:Number(saved?.size)||0,
    conflictPolicy:normalizeConflictPolicy(saved?.conflict_policy),conflictJobID:String(saved?.conflict_job_id||spec?.job_id||''),
    persistSpec:spec,
    run:runnable?(item=>persistentLocalTransferRun(view,spec,item)):null
  };
}
function restorePersistentLocalTransferQueue(view){
  const saved=readPersistentLocalTransferItems().filter(item=>item.profile_id===String(view.profile.id||''));
  if(!saved.length)return;
  enqueueTransferTasks(view,saved.map(item=>localTransferTaskFromSaved(view,item)));
}
async function refreshProfiles(){
  const data=await app.jsonFetch('/api/file-transfer/profiles');
  profilesByID=new Map((Array.isArray(data?.profiles)?data.profiles:[]).map(profile=>[String(profile.id||''),profile]));
  return profilesByID;
}
function profileFor(view){return profilesByID.get(view.profile.id)||view.profile;}
async function updateViewMaxConnections(view,value){
  const profile=profileFor(view),protocol=String(profile?.protocol||'').toLowerCase();
  const max=Math.max(1,Math.min(16,Math.trunc(Number(value)||3)));
  const payload=protocol==='sftp'
    ?{name:profile.name,protocol:'sftp',ssh_profile_id:profile.ssh_profile_id,initial_path:profile.initial_path||'.',max_connections:max}
    :{name:profile.name,protocol:'ftp',host:profile.host,port:Number(profile.port)||21,username:profile.username||'',initial_path:profile.initial_path||'.',connect_timeout_seconds:Number(profile.connect_timeout_seconds)||10,ftp_tls_mode:profile.ftp_tls_mode||'plain',max_connections:max};
  const updated=await app.jsonFetch('/api/file-transfer/profiles/'+encodeURIComponent(profile.id),{
    method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)
  });
  profilesByID.set(String(updated.id||profile.id),updated);
  view.profile={...view.profile,...updated,max_connections:max};
  if(view.transferQueue){
    view.transferQueue.serverMaxConnections=max;
    if(view.transferQueue.connectionLimit)view.transferQueue.connectionLimit.value=String(max);
    scheduleTransferQueueRender(view);syncRemoteNavigationAvailability(view);processTransferQueue(view);
  }
  return max;
}

function activateView(id,{force=false}={}){
  const view=views.get(id);if(!view)return false;
  if(app.activateExternalView('file-transfer:'+id,{force})===false)return false;
  activeViewID=id;
  for(const [otherID,other] of views){
    const active=otherID===id;other.tab.classList.toggle('active',active);other.pane.classList.toggle('hidden',!active);
  }
  persistFileTransferSession();
  return true;
}
function closeView(id){
  const view=views.get(id);if(!view)return;
  if(view.serverQueueTimer){clearInterval(view.serverQueueTimer);view.serverQueueTimer=0;}
  view.tab.remove();view.pane.remove();views.delete(id);
  if(activeViewID===id)activeViewID='';
  persistFileTransferSession();
}
function closeContextMenu(){contextMenu?.remove();contextMenu=null;}
const maxRenderedTransferRows=2000;
function queueStatusLabel(status){
  switch(status){case'queued':return 'Queued';case'running':return 'Running';case'success':return 'Done';case'failed':return 'Failed';case'conflict':return 'Conflict';case'skipped':return 'Skipped';default:return String(status||'');}
}
function queueCounts(view){
  const counts={queued:0,running:0,success:0,failed:0,conflict:0,skipped:0};
  for(const item of view.transferQueue?.items||[])if(Object.prototype.hasOwnProperty.call(counts,item.status))counts[item.status]++;
  return counts;
}
function queueSelectedItems(queue){return queue.items.filter(item=>queue.selectedIDs.has(item.id));}
function pruneQueueSelection(queue){
  const live=new Set(queue.items.map(item=>item.id));
  for(const id of [...queue.selectedIDs])if(!live.has(id))queue.selectedIDs.delete(id);
  if(queue.selectionAnchor!==null&&!live.has(queue.selectionAnchor))queue.selectionAnchor=null;
}
function selectQueueItem(queue,item,event={},visible=[]){
  const ids=queue.selectedIDs,key=item.id;
  if(event.shiftKey&&queue.selectionAnchor!==null){
    const from=visible.findIndex(candidate=>candidate.id===queue.selectionAnchor),to=visible.findIndex(candidate=>candidate.id===key);
    if(from>=0&&to>=0){
      if(!(event.ctrlKey||event.metaKey))ids.clear();
      for(let i=Math.min(from,to);i<=Math.max(from,to);i++)ids.add(visible[i].id);
    }else{ids.clear();ids.add(key);}
  }else if(event.ctrlKey||event.metaKey){
    if(ids.has(key))ids.delete(key);else ids.add(key);
    queue.selectionAnchor=key;
  }else{
    ids.clear();ids.add(key);queue.selectionAnchor=key;
  }
}
function toggleQueueCheckbox(queue,item,checked){
  if(checked)queue.selectedIDs.add(item.id);else queue.selectedIDs.delete(item.id);
  queue.selectionAnchor=item.id;
}
function renderTransferQueue(view){
  const queue=view.transferQueue;if(!queue?.body)return;
  pruneQueueSelection(queue);
  const counts=queueCounts(view);
  const totalScans=(queue.activeScans||0)+(queue.serverActiveScans||0);
  const queuedScans=(queue.pendingLocalScans||0)+Number(queue.serverQueuedScans||0);
  const scanText=totalScans>0?'Scanning '+totalScans+' · ':'';
  const scanQueueText=queuedScans>0?'Scan queued '+queuedScans+' · ':'';
  const maxConnections=Math.max(1,Number(queue.serverMaxConnections||view.profile?.max_connections||3)||3);
  const activeConnections=remoteConnectionUsage(view);
  const connectionText='Connections '+activeConnections+'/'+maxConnections+' · ';
  const pauseText=queue.paused?'Paused · ':'';
  let visible=queue.items;
  if(visible.length>maxRenderedTransferRows){
    const active=visible.filter(item=>item.status!=='success');
    visible=active.length>=maxRenderedTransferRows
      ?active.slice(0,maxRenderedTransferRows)
      :active.concat(queue.items.filter(item=>item.status==='success').slice(-(maxRenderedTransferRows-active.length)));
  }
  const shown=visible.length<queue.items.length?' · Showing '+visible.length+'/'+queue.items.length:'';
  queue.summary.textContent=pauseText+connectionText+scanText+scanQueueText+'Queued '+counts.queued+' · Running '+counts.running+' · Conflict '+counts.conflict+' · Done '+counts.success+' · Skipped '+counts.skipped+' · Failed '+counts.failed+shown;
  queue.retry.disabled=counts.failed===0;queue.clear.disabled=counts.success===0&&counts.skipped===0;
  const scanBusy=totalScans>0||queuedScans>0;
  queue.stopScan.disabled=!scanBusy;queue.clearQueue.disabled=queue.items.length===0&&!scanBusy;
  queue.body.replaceChildren();
  if(!queue.items.length){
    const tr=document.createElement('tr'),td=document.createElement('td');td.colSpan=8;td.className='ft-queue-empty';td.textContent='No transfers in this session';tr.append(td);queue.body.append(tr);return;
  }
  const fragment=document.createDocumentFragment();
  for(const item of visible){
    const tr=document.createElement('tr');tr.classList.toggle('selected',queue.selectedIDs.has(item.id));
    const select=document.createElement('td');select.className='ft-queue-select';
    const checkbox=document.createElement('input');checkbox.type='checkbox';checkbox.checked=queue.selectedIDs.has(item.id);
    checkbox.onclick=event=>{event.stopPropagation();toggleQueueCheckbox(queue,item,checkbox.checked);scheduleTransferQueueRender(view);};
    select.append(checkbox);
    const direction=document.createElement('td');direction.textContent=item.direction||'';
    const kind=document.createElement('td');kind.className='ft-queue-kind';kind.textContent=item.kind||'Transfer';
    const source=document.createElement('td');source.className='ft-queue-path';source.textContent=item.source||'';source.title=item.source||'';
    const target=document.createElement('td');target.className='ft-queue-path';target.textContent=item.target||'';target.title=item.target||'';
    const size=document.createElement('td');size.className='ft-size';size.textContent=item.size?formatSize(item.size):'';
    const status=document.createElement('td');status.className='ft-queue-status '+item.status;status.textContent=queueStatusLabel(item.status)+(item.detail?' · '+item.detail:'')+(item.removeAfterRun?' · cancel/remove pending':'');
    const error=document.createElement('td');error.className='ft-queue-error';error.textContent=item.error||'';error.title=item.error||'';
    tr.append(select,direction,kind,source,target,size,status,error);
    tr.onclick=event=>{selectQueueItem(queue,item,event,visible);scheduleTransferQueueRender(view);};
    tr.oncontextmenu=event=>queueContextMenu(view,event,item,visible);
    fragment.append(tr);
  }
  queue.body.append(fragment);
}
function scheduleTransferQueueRender(view){
  const queue=view.transferQueue;if(!queue||queue.renderTimer)return;
  queue.renderTimer=setTimeout(()=>{queue.renderTimer=0;renderTransferQueue(view);},80);
}
async function serverTransferQueueControl(view,action,itemIDs=[],extra={}){
  return app.jsonFetch('/api/file-transfer/jobs/control',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({profile_id:view.profile.id,action,item_ids:itemIDs,...extra})
  });
}
async function serverTransferQueueControlBatched(view,action,itemIDs=[],extra={}){
  const ids=[...(itemIDs||[])].filter(Boolean);
  if(!ids.length)return serverTransferQueueControl(view,action,[],extra);
  let result=null;
  const batchSize=1000;
  for(let offset=0;offset<ids.length;offset+=batchSize){
    result=await serverTransferQueueControl(view,action,ids.slice(offset,offset+batchSize),extra);
  }
  return result;
}
async function createServerTransferJob(view,payload){
  payload={...payload};
  if(!payload.conflict_policy){
    if(payload.kind==='host_upload')payload.conflict_policy=effectiveDirectionConflictPolicy(view,'upload');
    if(payload.kind==='host_download')payload.conflict_policy=effectiveDirectionConflictPolicy(view,'download');
  }
  const job=await app.jsonFetch('/api/file-transfer/jobs',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({profile_id:view.profile.id,...payload})
  });
  await syncServerTransferQueue(view);
  return job;
}
async function migrateLegacyRemoteDeleteJobs(){
  const jobs=readPersistentFileTransferJobs();if(!jobs.length)return;
  for(const legacy of jobs){
    const view=views.get(String(legacy.profile_id||''));if(!view)continue;
    try{
      await createServerTransferJob(view,{kind:'remote_delete',remote_targets:legacy.targets||[]});
      removePersistentFileTransferJob(legacy.id);
    }catch(error){
      console.warn('Cannot migrate legacy remote delete job to daemon queue',error);
    }
  }
}
function handleCompressedUploadJobResults(view,jobs){
  if(!view.handledCompressedUploadJobs)view.handledCompressedUploadJobs=new Set();
  for(const job of Array.isArray(jobs)?jobs:[]){
    if(String(job?.kind||'')!=='host_archive_upload')continue;
    const id=String(job?.id||'');if(!id||view.handledCompressedUploadJobs.has(id))continue;
    const status=String(job?.status||'');
    if(!job?.needs_manual_extract||!job?.remote_archive||!(status==='done'||status==='failed'))continue;
    view.handledCompressedUploadJobs.add(id);
    const commands=job.manual_commands||null;
    setTimeout(()=>showManualExtractCommands(view,String(job.remote_archive),String(job.remote_destination||'.'),[],commands).catch(app.showError),0);
  }
}
async function syncServerTransferQueue(view){
  const queue=view.transferQueue;if(!queue)return;
  const snapshot=await app.jsonFetch('/api/file-transfer/jobs?profile_id='+encodeURIComponent(view.profile.id),{cache:'no-store'});
  const localItems=queue.items.filter(item=>!item.server);
  const serverItems=(Array.isArray(snapshot?.items)?snapshot.items:[]).map(item=>({
    id:'server:'+item.id,serverID:String(item.id||''),server:true,jobID:String(item.job_id||''),
    status:String(item.status||'queued'),error:String(item.error||''),direction:String(item.direction||''),
    kind:String(item.kind||'Transfer'),source:String(item.source||''),target:String(item.target||''),
    size:Number(item.size)||0,detail:String(item.detail||''),files:Number(item.files)||0,
    bytesDone:Number(item.bytes_done)||0,bytesTotal:Number(item.bytes_total)||0,
    decision:String(item.decision||''),conflict:item.conflict||null,run:null,removeAfterRun:false
  }));
  const wasBusy=Boolean(queue.serverBusy);
  const busy=Number(snapshot?.active_scans||0)>0||Number(snapshot?.queued_scans||0)>0||serverItems.some(item=>item.status==='queued'||item.status==='running'||item.status==='conflict');
  queue.serverActiveScans=Number(snapshot?.active_scans)||0;
  queue.serverQueuedScans=Number(snapshot?.queued_scans)||0;
  queue.serverMaxConnections=Math.max(1,Number(snapshot?.max_connections||view.profile?.max_connections||3)||3);
  if(queue.connectionLimit&&document.activeElement!==queue.connectionLimit)queue.connectionLimit.value=String(queue.serverMaxConnections);
  queue.serverActiveConnections=Math.max(0,Number(snapshot?.active_connections)||0);
  queue.serverActiveTransfers=Math.max(0,Number(snapshot?.active_transfers)||0);
  queue.serverActiveBrowses=Math.max(0,Number(snapshot?.active_browses)||0);
  queue.serverBusy=busy;queue.serverRevision=Number(snapshot?.revision)||0;
  queue.paused=Boolean(snapshot?.paused);
  queue.items=localItems.concat(serverItems);
  handleCompressedUploadJobResults(view,snapshot?.jobs);
  syncRemoteNavigationAvailability(view);
  scheduleTransferQueueRender(view);
  if(hasRunnableTransfer(queue))processTransferQueue(view);
  setTimeout(()=>maybePromptServerConflict(view),0);
  if(wasBusy&&!busy){
    invalidateRemoteCache(view,view.remote.currentPath||'.');
    loadRemoteDirectory(view,view.remote.currentPath||'.',{force:true}).catch(()=>{});
    if(view.left.source==='host')loadHostDirectory(view,view.left.currentPath||'.').catch(()=>{});
  }
}
function startServerTransferQueuePolling(view){
  if(view.serverQueueTimer)return;
  const poll=()=>syncServerTransferQueue(view).catch(error=>console.warn('Cannot sync server file-transfer queue',error));
  poll();view.serverQueueTimer=setInterval(poll,700);
}
async function applyServerConflictDecision(view,items,decision){
  const serverItems=(items||[]).filter(item=>item.server&&item.serverID&&item.status==='conflict');
  if(!serverItems.length)return;
  const direction=decision.direction||conflictDirectionFromKind(serverItems[0].kind);
  if(decision.scope==='direction_session'||decision.scope==='direction_always'){
    view.conflictSessionPolicies[direction]=normalizeConflictPolicy(decision.policy);
  }
  if(decision.scope==='direction_always')rememberConflictPolicy(view,direction,decision.policy);
  await serverTransferQueueControl(view,'resolve_conflict',serverItems.map(item=>item.serverID),{
    conflict_policy:decision.policy,
    conflict_scope:decision.scope,
    job_id:String(serverItems[0].jobID||'')
  });
  await syncServerTransferQueue(view);
}
function serverConflictViewKey(view){
  return String(view?.profile?.id||view?.id||'file-transfer');
}
function liveServerConflictItem(view,serverID){
  serverID=String(serverID||'');
  if(!serverID)return null;
  return (view?.transferQueue?.items||[]).find(candidate=>
    candidate.server&&candidate.serverID===serverID&&candidate.status==='conflict'&&candidate.conflict
  )||null;
}
async function resolveServerConflictItems(view,items){
  const conflicts=(items||[]).filter(item=>item.server&&item.status==='conflict'&&item.conflict);
  if(!conflicts.length)return;
  const first=conflicts[0],serverID=String(first.serverID||'');
  const live=liveServerConflictItem(view,serverID);
  if(!live)return;
  const meta=live.conflict||{};
  const decision=await requestTransferConflictDecision({
    kind:live.kind,source:live.source,target:live.target,
    source_size:meta.source_size,target_size:meta.target_size,
    source_modified:meta.source_modified,target_modified:meta.target_modified,
    stillCurrent:()=>Boolean(liveServerConflictItem(view,serverID))
  });
  if(!decision)return;
  const current=liveServerConflictItem(view,serverID);
  if(!current)return;
  await applyServerConflictDecision(view,[current],decision);
}
async function maybePromptServerConflict(view){
  const queue=view.transferQueue;if(!queue)return;
  const viewKey=serverConflictViewKey(view);
  if(activeServerConflictViews.has(viewKey))return;
  const item=queue.items.find(candidate=>candidate.server&&candidate.status==='conflict'&&candidate.conflict);
  if(!item)return;
  activeServerConflictViews.add(viewKey);
  try{await resolveServerConflictItems(view,[item]);}
  catch(error){console.warn('Cannot resolve file-transfer conflict',error);}
  finally{
    activeServerConflictViews.delete(viewKey);
    setTimeout(()=>maybePromptServerConflict(view),0);
  }
}
async function afterTransferQueueIdle(view){
  const queue=view.transferQueue;if(!queue)return;
  const currentRemote=remoteCacheKey(view.remote.currentPath||'.');
  const refreshCurrentRemote=queue.remoteDirty?.has(currentRemote);
  queue.remoteDirty?.clear();
  if(refreshCurrentRemote){
    try{await loadRemoteDirectory(view,currentRemote,{force:true});}catch(error){console.warn('Cannot refresh remote transfer target',error);}
  }
  if(queue.leftDirty){
    queue.leftDirty=false;
    try{await loadLeftDirectory(view,view.left.currentPath);}catch(error){console.warn('Cannot refresh left transfer target',error);}
  }
}
function nextQueuedFrom(list,stateKey,queue){
  let index=queue[stateKey]||0;
  while(index<list.length){
    const item=list[index++];
    queue[stateKey]=index;
    if(item?.status==='queued')return item;
  }
  if(index>4096){list.splice(0,index);queue[stateKey]=0;}
  return null;
}
function hasQueuedFrom(list,start=0){
  for(let i=start;i<list.length;i++)if(list[i]?.status==='queued')return true;
  return false;
}
function nextPendingTransfer(queue){
  const priority=nextQueuedFrom(queue.priorityPending,'priorityHead',queue);
  if(priority)return priority;
  if(queue.paused)return null;
  return nextQueuedFrom(queue.pending,'pendingHead',queue);
}
function hasAnyPendingTransfer(queue){
  return hasQueuedFrom(queue.priorityPending,queue.priorityHead)||hasQueuedFrom(queue.pending,queue.pendingHead);
}
function hasRunnableTransfer(queue){
  if(hasQueuedFrom(queue.priorityPending,queue.priorityHead))return true;
  return !queue.paused&&hasQueuedFrom(queue.pending,queue.pendingHead);
}
function removeFinishedQueueItem(queue,item){
  if(!item.removeAfterRun)return;
  queue.items=queue.items.filter(candidate=>candidate!==item);
  queue.selectedIDs.delete(item.id);
}
async function runTransferQueueItem(view,item){
  const queue=view.transferQueue;if(!queue)return;
  queue.runningCount=(queue.runningCount||0)+1;queue.running=true;
  item.abortController=new AbortController();
  item.status='running';item.error='';persistLocalTransferQueue(view);scheduleTransferQueueRender(view);syncRemoteNavigationAvailability(view);
  let poolBusy=false;
  try{
    const result=await item.run(item,item.abortController.signal);
    item.status=result?.skipped?'skipped':'success';item.run=null;
    if(item.jobID)markPersistentDeleteItemSuccess(view,item.jobID);
  }catch(error){
    if(error?.name==='AbortError'||item.abortController?.signal?.aborted){
      item.status='stopped';item.error='';item.detail='Cancelled';item.run=null;
    }else if(isTransferPoolBusy(error)){
      poolBusy=true;item.status='queued';item.error='Waiting for an FTP/SFTP connection slot';
      noteTransferPoolFull(view);queue.pending.push(item);
    }else{
      item.status='failed';item.error=String(error?.message||error||'Transfer failed');
    }
  }finally{
    item.abortController=null;
    queue.runningCount=Math.max(0,(queue.runningCount||1)-1);queue.running=queue.runningCount>0;
    if(!poolBusy)removeFinishedQueueItem(queue,item);
    persistLocalTransferQueue(view);scheduleTransferQueueRender(view);syncRemoteNavigationAvailability(view);
    if(poolBusy)scheduleTransferPoolRetry(view);
    else processTransferQueue(view);
    if(queue.runningCount===0&&queue.activeScans===0&&!hasAnyPendingTransfer(queue))await afterTransferQueueIdle(view);
  }
}
async function processTransferQueue(view){
  const queue=view.transferQueue;if(!queue)return;
  const limit=transferWorkerLimit(view);
  while((queue.runningCount||0)<limit){
    const item=nextPendingTransfer(queue);
    if(!item)break;
    void runTransferQueueItem(view,item);
  }
  queue.running=(queue.runningCount||0)>0;
  scheduleTransferQueueRender(view);syncRemoteNavigationAvailability(view);
}
function enqueueTransferTasks(view,tasks){
  const queue=view.transferQueue;if(!queue)throw new Error('Transfer queue is unavailable');
  const existingPersistent=new Set(queue.items.map(item=>localPersistentKey(item.persistSpec)).filter(Boolean));
  for(const task of tasks||[]){
    const persistentKey=localPersistentKey(task.persistSpec);
    if(persistentKey&&existingPersistent.has(persistentKey))continue;
    queue.sequence++;
    const item={
      id:task.id||queue.sequence,status:task.status||'queued',error:task.error||'',direction:task.direction||'',kind:task.kind||'Transfer',
      source:task.source||'',target:task.target||'',size:Number(task.size)||0,detail:String(task.detail||''),files:Number(task.files)||0,
      bytesDone:Number(task.bytesDone)||0,run:task.run,removeAfterRun:false,abortController:null,
      jobID:String(task.jobID||''),conflictPolicy:normalizeConflictPolicy(task.conflictPolicy),conflictJobID:String(task.conflictJobID||task.persistSpec?.job_id||''),persistSpec:task.persistSpec||null
    };
    queue.items.push(item);
    if(item.status==='queued')queue.pending.push(item);
    if(persistentKey)existingPersistent.add(persistentKey);
  }
  persistLocalTransferQueue(view);scheduleTransferQueueRender(view);if(!queue.paused)processTransferQueue(view);
}
function transferScanStoppedError(){
  const error=new Error('Transfer scan stopped');error.transferScanStopped=true;return error;
}
function assertTransferScanActive(view,generation){
  const queue=view?.transferQueue;
  if(!queue||generation!==queue.scanGeneration)throw transferScanStoppedError();
}
function runTransferScan(view,label,scanner){
  const queue=view.transferQueue;if(!queue)throw new Error('Transfer queue is unavailable');
  const generation=queue.scanGeneration;
  queue.pendingLocalScans=(queue.pendingLocalScans||0)+1;scheduleTransferQueueRender(view);
  const execute=async()=>{
    assertTransferScanActive(view,generation);
    queue.activeScans++;queue.scanLabel=String(label||'Scanning');scheduleTransferQueueRender(view);
    try{return await scanner({generation,check:()=>assertTransferScanActive(view,generation)});}
    finally{
      queue.activeScans=Math.max(0,queue.activeScans-1);
      queue.pendingLocalScans=Math.max(0,(queue.pendingLocalScans||0)-1);
      if(queue.activeScans===0)queue.scanLabel='';
      scheduleTransferQueueRender(view);
      if(hasRunnableTransfer(queue))processTransferQueue(view);
      else if((queue.runningCount||0)===0&&queue.activeScans===0&&!hasAnyPendingTransfer(queue))await afterTransferQueueIdle(view);
    }
  };
  queue.scanChain=queue.scanChain.then(execute,execute);
  return queue.scanChain;
}
async function stopTransferScans(view){
  const queue=view.transferQueue;if(!queue)return;
  queue.scanGeneration=(queue.scanGeneration||0)+1;
  clearPersistentLocalScansForProfile(view.profile.id);clearPersistentFileTransferJobsForProfile(view.profile.id);
  queue.scanLabel='Stopping scan…';scheduleTransferQueueRender(view);persistFileTransferSession();
  try{await serverTransferQueueControl(view,'stop_scans');}
  finally{
    queue.scanLabel='';
    await syncServerTransferQueue(view).catch(()=>{});
    scheduleTransferQueueRender(view);
  }
}
async function clearTransferQueue(view){
  const queue=view.transferQueue;if(!queue)return;
  queue.scanGeneration=(queue.scanGeneration||0)+1;
  clearPersistentLocalScansForProfile(view.profile.id);clearPersistentFileTransferJobsForProfile(view.profile.id);
  queue.pending=[];queue.pendingHead=0;queue.priorityPending=[];queue.priorityHead=0;
  const kept=[];
  for(const item of queue.items){
    if(item.server){kept.push(item);continue;}
    if(item.status==='running'){
      item.removeAfterRun=true;item.detail='Cancelling…';item.abortController?.abort();kept.push(item);
    }else{item.run=null;}
  }
  queue.items=kept;queue.selectedIDs.clear();queue.selectionAnchor=null;
  persistLocalTransferQueue(view);scheduleTransferQueueRender(view);persistFileTransferSession();
  await serverTransferQueueControl(view,'clear_queue');
  await syncServerTransferQueue(view);
  if((queue.runningCount||0)===0&&!hasAnyPendingTransfer(queue))afterTransferQueueIdle(view);
}
async function pauseTransferQueue(view){
  const queue=view.transferQueue;if(!queue)return;
  queue.paused=true;scheduleTransferQueueRender(view);persistFileTransferSession();
  await serverTransferQueueControl(view,'pause');await syncServerTransferQueue(view);
}
async function resumeTransferQueue(view){
  const queue=view.transferQueue;if(!queue)return;
  queue.paused=false;scheduleTransferQueueRender(view);persistFileTransferSession();processTransferQueue(view);
  await serverTransferQueueControl(view,'resume');await syncServerTransferQueue(view);
}
async function resumeSelectedTransfers(view){
  const queue=view.transferQueue;if(!queue)return;
  const selected=queueSelectedItems(queue),serverIDs=[];
  for(const item of selected){
    if(item.server){if(item.serverID)serverIDs.push(item.serverID);continue;}
    if(item.status==='failed'&&typeof item.run==='function'){item.status='queued';item.error='';}
    if(item.status==='queued'&&typeof item.run==='function')queue.priorityPending.push(item);
  }
  persistLocalTransferQueue(view);scheduleTransferQueueRender(view);processTransferQueue(view);
  if(serverIDs.length){await serverTransferQueueControlBatched(view,'resume_selected',serverIDs);await syncServerTransferQueue(view);}
}
async function removeSelectedTransfers(view){
  const queue=view.transferQueue;if(!queue)return;
  const selectedIDs=new Set(queue.selectedIDs),serverIDs=[];
  for(const item of queue.items){
    if(!selectedIDs.has(item.id))continue;
    if(item.server){if(item.serverID)serverIDs.push(item.serverID);continue;}
    if(item.status==='running'){item.removeAfterRun=true;item.detail='Cancelling…';item.abortController?.abort();continue;}
    item.status='removed';item.run=null;
  }
  queue.items=queue.items.filter(item=>item.status!=='removed');
  queue.selectedIDs.clear();queue.selectionAnchor=null;
  persistLocalTransferQueue(view);scheduleTransferQueueRender(view);
  if(serverIDs.length){await serverTransferQueueControlBatched(view,'remove_selected',serverIDs);await syncServerTransferQueue(view);}
  if((queue.runningCount||0)===0&&queue.activeScans===0&&!hasAnyPendingTransfer(queue))afterTransferQueueIdle(view);
}
function queueContextMenu(view,event,item=null,visible=[]){
  event.preventDefault();event.stopPropagation();
  const queue=view.transferQueue;if(!queue)return;
  if(item&&!queue.selectedIDs.has(item.id)){
    queue.selectedIDs.clear();queue.selectedIDs.add(item.id);queue.selectionAnchor=item.id;scheduleTransferQueueRender(view);
  }
  const selected=queueSelectedItems(queue);
  const resumable=selected.some(candidate=>(candidate.status==='queued'||candidate.status==='failed')&&(candidate.server||typeof candidate.run==='function'));
  const conflicts=selected.filter(candidate=>candidate.status==='conflict');
  const scanBusy=(queue.activeScans||0)>0||(queue.pendingLocalScans||0)>0||(queue.serverActiveScans||0)>0||(queue.serverQueuedScans||0)>0;
  const menu=[
    {label:queue.paused?'Resume queue':'Pause queue',action:()=>queue.paused?resumeTransferQueue(view):pauseTransferQueue(view)},
    {label:'Stop scan',disabled:!scanBusy,action:()=>stopTransferScans(view)},
    {label:'Clear queue',danger:true,disabled:queue.items.length===0&&!scanBusy,action:()=>clearTransferQueue(view)}
  ];
  if(selected.length){
    menu.push({separator:true});
    if(conflicts.length)menu.push({label:'Resolve conflict…',action:()=>resolveServerConflictItems(view,conflicts)});
    menu.push({label:'Resume selected',disabled:!resumable,action:()=>resumeSelectedTransfers(view)});
    const hasRunning=selected.some(candidate=>candidate.status==='running');
    menu.push({label:hasRunning?'Cancel / remove selected':'Remove selected',danger:true,action:()=>removeSelectedTransfers(view)});
  }
  showContextMenu(menu,event.clientX,event.clientY,selected.length?selected.length+' queue item(s) selected':'Transfer Queue');
}
function markRemoteQueueDirty(view,path){
  const queue=view.transferQueue;if(!queue)return;
  const key=remoteCacheKey(path);queue.remoteDirty.add(key);invalidateRemoteCache(view,key);
}
function markLeftQueueDirty(view){if(view.transferQueue)view.transferQueue.leftDirty=true;}
function createTransferQueue(view){
  const root=document.createElement('div');root.className='ft-queue';
  const resizer=document.createElement('div');resizer.className='ft-queue-resizer';resizer.title='Drag to resize Transfer Queue · double-click to reset';
  const head=document.createElement('div');head.className='ft-queue-head';
  const title=document.createElement('span');title.className='ft-queue-title';title.textContent='Transfer Queue';
  const summary=document.createElement('span');summary.className='ft-queue-summary';
  const spacer=document.createElement('span');spacer.className='ft-queue-spacer';
  const connectionWrap=document.createElement('label');connectionWrap.className='ft-queue-connections';connectionWrap.title='Shared FTP/SFTP connection budget for scans, browsing and transfers';
  const connectionText=document.createElement('span');connectionText.textContent='Max connections';
  const connectionLimit=document.createElement('input');connectionLimit.type='number';connectionLimit.min='1';connectionLimit.max='16';connectionLimit.step='1';connectionLimit.value=String(view.profile?.max_connections||3);
  const canEditConnectionLimit=!app.sharedMode||Boolean(app.hasPermission?.('settings.write'));
  connectionLimit.disabled=!canEditConnectionLimit;
  if(!canEditConnectionLimit)connectionWrap.title='Changing Max connections requires settings.write permission';
  connectionWrap.append(connectionText,connectionLimit);
  const retry=document.createElement('button');retry.type='button';retry.textContent='Retry failed';
  const stopScan=document.createElement('button');stopScan.type='button';stopScan.textContent='Stop scan';
  const clear=document.createElement('button');clear.type='button';clear.textContent='Clear done';
  const clearQueue=document.createElement('button');clearQueue.type='button';clearQueue.textContent='Clear queue';clearQueue.classList.add('danger');
  head.append(title,summary,spacer,connectionWrap,retry,stopScan,clear,clearQueue);
  const wrap=document.createElement('div');wrap.className='ft-queue-wrap';
  const table=document.createElement('table');table.className='ft-queue-table';
  const thead=document.createElement('thead'),hr=document.createElement('tr');
  for(const label of ['','', 'Kind','Source','Target','Size','Status','Error']){const th=document.createElement('th');th.textContent=label;hr.append(th);}
  thead.append(hr);const body=document.createElement('tbody');table.append(thead,body);wrap.append(table);root.append(resizer,head,wrap);
  const queue={
    root,resizer,body,summary,retry,stopScan,clear,clearQueue,connectionLimit,items:[],pending:[],pendingHead:0,priorityPending:[],priorityHead:0,
    sequence:0,running:false,runningCount:0,poolRetryTimer:0,paused:false,selectedIDs:new Set(),selectionAnchor:null,
    activeScans:0,pendingLocalScans:0,scanGeneration:0,serverActiveScans:0,serverQueuedScans:0,serverMaxConnections:Math.max(1,Number(view.profile?.max_connections||3)||3),
    serverActiveConnections:0,serverActiveTransfers:0,serverActiveBrowses:0,serverBusy:false,serverRevision:0,scanLabel:'',scanChain:Promise.resolve(),remoteDirty:new Set(),leftDirty:false
  };
  view.transferQueue=queue;
  connectionLimit.onchange=async()=>{
    if(!canEditConnectionLimit)return;
    const previous=Math.max(1,Number(queue.serverMaxConnections||view.profile?.max_connections||3)||3);
    connectionLimit.disabled=true;
    try{connectionLimit.value=String(await updateViewMaxConnections(view,connectionLimit.value));}
    catch(error){connectionLimit.value=String(previous);app.showError(error);}
    finally{connectionLimit.disabled=!canEditConnectionLimit;}
  };
  root.oncontextmenu=event=>{if(event.target.closest('tbody tr'))return;queueContextMenu(view,event);};
  stopScan.onclick=()=>stopTransferScans(view).catch(app.showError);
  clearQueue.onclick=()=>{
    const total=queue.items.length+(queue.pendingLocalScans||0)+(queue.serverQueuedScans||0);
    if(total>0&&!confirm('Clear the entire transfer queue and stop all scans for this FTP/SFTP profile?\n\nRunning daemon transfers are cancelled when possible; local-browser operations stop at their next safe boundary.'))return;
    clearTransferQueue(view).catch(app.showError);
  };
  retry.onclick=async()=>{
    for(const item of queue.items)if(!item.server&&item.status==='failed'&&typeof item.run==='function'){item.status='queued';item.error='';queue.pending.push(item);}
    persistLocalTransferQueue(view);scheduleTransferQueueRender(view);processTransferQueue(view);
    await serverTransferQueueControl(view,'retry_failed');await syncServerTransferQueue(view);
  };
  clear.onclick=async()=>{
    queue.items=queue.items.filter(item=>item.server||(item.status!=='success'&&item.status!=='skipped'));
    pruneQueueSelection(queue);persistLocalTransferQueue(view);scheduleTransferQueueRender(view);
    await serverTransferQueueControl(view,'clear_done');await syncServerTransferQueue(view);
  };
  renderTransferQueue(view);return root;
}
function showContextMenu(items,x,y,title=''){
  closeContextMenu();
  const menu=document.createElement('div');menu.className='ft-context';
  menu.setAttribute('role','menu');
  if(title){
    const heading=document.createElement('div');heading.className='ft-context-title';heading.textContent=title;menu.append(heading);
  }
  for(const item of items){
    if(item.separator){
      const separator=document.createElement('div');separator.className='ft-context-separator';separator.setAttribute('role','separator');menu.append(separator);continue;
    }
    const button=document.createElement('button');button.type='button';button.textContent=item.label;button.disabled=Boolean(item.disabled);
    if(item.danger)button.classList.add('danger');
    button.setAttribute('role','menuitem');
    button.onclick=()=>{closeContextMenu();Promise.resolve(item.action?.()).catch(app.showError);};menu.append(button);
  }
  document.body.append(menu);contextMenu=menu;const rect=menu.getBoundingClientRect();
  menu.style.left=Math.max(4,Math.min(x,window.innerWidth-rect.width-4))+'px';menu.style.top=Math.max(4,Math.min(y,window.innerHeight-rect.height-4))+'px';
}
document.addEventListener('pointerdown',event=>{if(contextMenu&&!contextMenu.contains(event.target))closeContextMenu();},true);
window.addEventListener('blur',closeContextMenu);

function downloadFrame(){
  let frame=document.querySelector('iframe[name="taskdeck-file-transfer-download"]');
  if(frame)return frame;
  frame=document.createElement('iframe');frame.name='taskdeck-file-transfer-download';frame.hidden=true;document.body.append(frame);return frame;
}
async function issueDownloadTicket(view,remotePath){
  const data=await app.jsonFetch('/api/file-transfer/download-ticket',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({profile_id:view.profile.id,path:remotePath})
  });
  const ticket=String(data?.ticket||'').trim();if(!ticket)throw new Error('Server did not return a download ticket');return ticket;
}
async function downloadFileToBrowser(view,remotePath){
  const ticket=await issueDownloadTicket(view,remotePath);downloadFrame().src='/api/file-transfer/download?ticket='+encodeURIComponent(ticket);
}

async function mutateRemoteRequest(view,action,path,newPath='',directory=false){
  return app.jsonFetch('/api/file-transfer/mutate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({
    profile_id:view.profile.id,action,path,new_path:newPath,directory
  })});
}
async function mutateRemote(view,action,path,newPath='',directory=false){
  await mutateRemoteRequest(view,action,path,newPath,directory);
  invalidateRemoteCache(view,parentPath(path,true));
  if(directory)invalidateRemoteCache(view,path,true);
  if(newPath){invalidateRemoteCache(view,parentPath(newPath,true));if(directory)invalidateRemoteCache(view,newPath,true);}
  await loadRemoteDirectory(view,view.remote.currentPath,{force:true});
}
async function renameRemoteEntry(view,entry){
  const oldPath=joinPath(view.remote.currentPath,entry.name,true),nextName=prompt('Rename to:',entry.name);
  if(nextName===null)return;const name=String(nextName).trim();if(!name||name===entry.name)return;
  if(name.includes('/')||name.includes('\\'))throw new Error('New name must not contain path separators');
  await mutateRemote(view,'rename',oldPath,joinPath(view.remote.currentPath,name,true),entryType(entry)==='directory');
}
async function deleteRemoteEntry(view,entry){
  if(!confirm('Delete '+entry.name+'?'+(entryType(entry)==='directory'?'\n\nDirectory removal is non-recursive.':'')))return;
  await mutateRemote(view,'delete',joinPath(view.remote.currentPath,entry.name,true),'',entryType(entry)==='directory');
}

function entrySelectionKey(entry){return encodeURIComponent(entryType(entry))+':'+encodeURIComponent(String(entry?.name||''));}
function selectedEntries(panel){
  const keys=panel.selectedKeys||new Set();
  return (panel.entries||[]).filter(entry=>keys.has(entrySelectionKey(entry)));
}
function selectedFiles(panel){return selectedEntries(panel).filter(entry=>entryType(entry)==='file');}
function resetPanelSelection(panel){
  panel.selectedKeys=new Set();panel.selected=null;panel.selectionAnchor='';
}
function syncSelectionRows(panel){
  const keys=panel.selectedKeys||new Set();
  for(const row of panel.tbody?.querySelectorAll('tr[data-selection-key]')||[]){
    const selected=keys.has(row.dataset.selectionKey||'');
    row.classList.toggle('selected',selected);
    const checkbox=row.querySelector('input[type=checkbox]');if(checkbox)checkbox.checked=selected;
  }
  const visible=panel.visibleEntries||[],selectedCount=visible.filter(entry=>keys.has(entrySelectionKey(entry))).length;
  if(panel.selectAllCheckbox){
    panel.selectAllCheckbox.checked=visible.length>0&&selectedCount===visible.length;
    panel.selectAllCheckbox.indeterminate=selectedCount>0&&selectedCount<visible.length;
  }
  const selected=selectedEntries(panel);panel.selected=selected[0]||null;panel.onSelection?.();
}
function selectOnlyEntry(panel,entry){
  panel.selectedKeys=new Set([entrySelectionKey(entry)]);panel.selectionAnchor=entrySelectionKey(entry);syncSelectionRows(panel);
}
function selectAllEntries(panel){
  panel.selectedKeys=new Set((panel.visibleEntries||[]).map(entrySelectionKey));
  panel.selectionAnchor=(panel.visibleEntries?.[0]&&entrySelectionKey(panel.visibleEntries[0]))||'';
  syncSelectionRows(panel);
}
function clearSelection(panel){resetPanelSelection(panel);syncSelectionRows(panel);}
function selectTableEntry(panel,entry,event={},mode='click'){
  const key=entrySelectionKey(entry),keys=panel.selectedKeys||(panel.selectedKeys=new Set());
  if(mode==='context'){
    if(!keys.has(key)){keys.clear();keys.add(key);}
    panel.selectionAnchor=key;syncSelectionRows(panel);return;
  }
  const toggle=Boolean(event.ctrlKey||event.metaKey);
  if(event.shiftKey&&panel.selectionAnchor){
    const visible=panel.visibleEntries||[],anchorIndex=visible.findIndex(item=>entrySelectionKey(item)===panel.selectionAnchor),currentIndex=visible.findIndex(item=>entrySelectionKey(item)===key);
    if(anchorIndex>=0&&currentIndex>=0){
      if(!toggle)keys.clear();
      const [from,to]=anchorIndex<=currentIndex?[anchorIndex,currentIndex]:[currentIndex,anchorIndex];
      for(let i=from;i<=to;i++)keys.add(entrySelectionKey(visible[i]));
    }else{keys.clear();keys.add(key);}
  }else if(toggle){
    if(keys.has(key))keys.delete(key);else keys.add(key);
    panel.selectionAnchor=key;
  }else{
    keys.clear();keys.add(key);panel.selectionAnchor=key;
  }
  syncSelectionRows(panel);
}

function createSortableTable(panel,onDoubleClick,onContextMenu){
  const wrap=document.createElement('div');wrap.className='ft-table-wrap';wrap.tabIndex=0;
  const table=document.createElement('table');table.className='ft-table';
  const thead=document.createElement('thead'),hr=document.createElement('tr');
  const selectTH=document.createElement('th');selectTH.className='ft-nosort ft-select-cell';
  const selectAll=document.createElement('input');selectAll.type='checkbox';selectAll.title='Select all visible items';
  selectAll.onchange=()=>{if(selectAll.checked)selectAllEntries(panel);else clearSelection(panel);};selectTH.append(selectAll);hr.append(selectTH);panel.selectAllCheckbox=selectAll;
  const columns=[['name','Name'],['type','Type'],['size','Size'],['modified','Modified']];
  panel.headers=new Map();
  for(const [key,label] of columns){
    const th=document.createElement('th');th.dataset.sortKey=key;th.textContent=label;
    const marker=document.createElement('span');marker.className='sort';th.append(marker);
    th.onclick=()=>{
      if(panel.sort.key===key)panel.sort.direction=panel.sort.direction==='asc'?'desc':'asc';
      else{panel.sort.key=key;panel.sort.direction='asc';}
      renderTable(panel,onDoubleClick,onContextMenu);
    };
    panel.headers.set(key,{th,marker});hr.append(th);
  }
  thead.append(hr);const tbody=document.createElement('tbody');panel.tbody=tbody;table.append(thead,tbody);wrap.append(table);
  wrap.addEventListener('keydown',event=>{
    if((event.ctrlKey||event.metaKey)&&String(event.key).toLowerCase()==='a'){event.preventDefault();selectAllEntries(panel);}
    if(event.key==='Escape'){clearSelection(panel);closeContextMenu();}
  });
  return wrap;
}
function renderTable(panel,onDoubleClick,onContextMenu){
  for(const [key,item] of panel.headers||[]){item.marker.textContent=panel.sort.key===key?(panel.sort.direction==='asc'?'▲':'▼'):'';}
  panel.tbody.replaceChildren();
  const entries=sortEntries(panel.entries,panel.sort);panel.visibleEntries=entries;
  const showParent=canGoParentPath(panel.currentPath||'.',Boolean(panel.pathIsRemote));
  if(showParent){
    const tr=document.createElement('tr');tr.className='ft-parent-row';tr.title='Up one level';
    const selectCell=document.createElement('td');selectCell.className='ft-select-cell';
    const name=document.createElement('td');name.className='ft-name';
    const icon=document.createElement('span');icon.className='ft-kind';icon.textContent='📁';
    const label=document.createElement('span');label.textContent='..';name.append(icon,label);
    const type=document.createElement('td');type.className='ft-type';type.textContent='Folder';
    const size=document.createElement('td');size.className='ft-size';
    const modified=document.createElement('td');modified.className='ft-modified';
    tr.append(selectCell,name,type,size,modified);
    tr.onclick=event=>event.preventDefault();
    tr.ondblclick=event=>{event.preventDefault();if(panel.navigationDisabled)return;panel.goUp?.().catch(app.showError);};
    tr.oncontextmenu=event=>event.preventDefault();
    panel.tbody.append(tr);
  }
  if(!entries.length){
    if(!showParent){
      const tr=document.createElement('tr'),td=document.createElement('td');td.colSpan=5;td.className='ft-empty';td.textContent=panel.emptyText||'Directory is empty';tr.append(td);panel.tbody.append(tr);
    }
    syncSelectionRows(panel);return;
  }
  for(const entry of entries){
    const selectionKey=entrySelectionKey(entry),tr=document.createElement('tr');tr.dataset.selectionKey=selectionKey;
    const selectCell=document.createElement('td');selectCell.className='ft-select-cell';
    const checkbox=document.createElement('input');checkbox.type='checkbox';checkbox.checked=panel.selectedKeys?.has(selectionKey)||false;
    checkbox.onclick=event=>{event.stopPropagation();selectTableEntry(panel,entry,{ctrlKey:true,metaKey:false,shiftKey:event.shiftKey});};selectCell.append(checkbox);
    const name=document.createElement('td');name.className='ft-name';
    const icon=document.createElement('span');icon.className='ft-kind';icon.textContent=entryType(entry)==='directory'?'📁':entryType(entry)==='symlink'?'↗':'📄';
    const label=document.createElement('span');label.textContent=entry.name;name.append(icon,label);
    const type=document.createElement('td');type.className='ft-type';type.textContent=typeLabel(entry.type);
    const size=document.createElement('td');size.className='ft-size';size.textContent=entryType(entry)==='directory'?'':formatSize(entry.size);
    const modified=document.createElement('td');modified.className='ft-modified';modified.textContent=formatModified(entry.modified);
    tr.append(selectCell,name,type,size,modified);
    tr.onclick=event=>selectTableEntry(panel,entry,event);
    tr.ondblclick=event=>{event.preventDefault();onDoubleClick(entry);};
    tr.oncontextmenu=event=>{event.preventDefault();event.stopPropagation();selectTableEntry(panel,entry,event,'context');onContextMenu(entry,event);};
    panel.tbody.append(tr);
  }
  syncSelectionRows(panel);
}

function pathBar(panel,{remote=false,onLoad,onRefresh,onUp}){
  panel.pathIsRemote=Boolean(remote);panel.goUp=onUp;
  const bar=document.createElement('div');bar.className='ft-pathbar';
  const up=document.createElement('button');up.type='button';up.textContent='↑';up.title='Parent directory';
  const input=document.createElement('input');input.type='text';input.spellcheck=false;input.autocomplete='off';
  const history=document.createElement('select');history.className='ft-path-history-select';history.setAttribute('aria-label','Visited paths');history.title='Visited paths';
  const star=document.createElement('button');star.type='button';star.className='ft-favorite-toggle';star.textContent='☆';
  const go=document.createElement('button');go.type='button';go.textContent='Go';
  const refresh=document.createElement('button');refresh.type='button';refresh.textContent='↻';refresh.title='Refresh';
  bar.append(up,input,history,star,go,refresh);
  panel.pathInput=input;panel.pathHistorySelect=history;panel.favoriteToggle=star;panel.pathGo=go;panel.pathUp=up;panel.refresh=refresh;
  const reloadMemory=()=>populatePathMemory(history,star,panel.memoryScope?.()||'',input.value||'.');
  history.onchange=()=>{if(history.value){input.value=history.value;onLoad(history.value).catch(app.showError);}history.value='';};
  star.onclick=()=>{const scope=panel.memoryScope?.();if(!scope)return;toggleFavorite(scope,input.value||'.');reloadMemory();};
  go.onclick=()=>onLoad(input.value).catch(app.showError);
  input.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();go.click();}});
  refresh.onclick=()=>onRefresh().catch(app.showError);up.onclick=()=>onUp().catch(app.showError);
  panel.refreshPathMemory=reloadMemory;return bar;
}

function markPathLoaded(panel,path){
  panel.currentPath=path;panel.pathInput.value=path;
  const scope=panel.memoryScope?.();if(scope){rememberPath(scope,path);panel.refreshPathMemory?.();}
}

function remoteConnectionUsage(view){
  const queue=view?.transferQueue;
  if(!queue)return 0;
  const server=Number(queue.serverActiveConnections)||0;
  const known=Number(queue.serverActiveScans||0)+Number(queue.serverActiveTransfers||0)+Number(queue.runningCount||0)+Number(queue.activeScans||0);
  return Math.max(server,known);
}
function remoteConnectionPoolFull(view){
  const queue=view?.transferQueue;
  const max=Math.max(1,Number(queue?.serverMaxConnections||view?.profile?.max_connections||3)||3);
  return remoteConnectionUsage(view)>=max;
}
function transferWorkerLimit(view){
  const queue=view?.transferQueue;
  const max=Math.max(1,Number(queue?.serverMaxConnections||view?.profile?.max_connections||3)||3);
  const ownRunning=Math.max(0,Number(queue?.runningCount||0));
  const serverActive=Math.max(0,Number(queue?.serverActiveConnections||0));
  const serverBackground=Math.max(0,Number(queue?.serverActiveScans||0))+Math.max(0,Number(queue?.serverActiveTransfers||0));
  const observedExternal=Math.max(0,serverActive-ownRunning);
  const logicalReserved=serverBackground+Math.max(0,Number(queue?.activeScans||0));
  return Math.max(0,max-Math.max(observedExternal,logicalReserved));
}
function noteTransferPoolFull(view){
  const queue=view?.transferQueue;if(!queue)return;
  const max=Math.max(1,Number(queue.serverMaxConnections||view?.profile?.max_connections||3)||3);
  queue.serverActiveConnections=Math.max(max,Number(queue.serverActiveConnections)||0);
  syncRemoteNavigationAvailability(view);scheduleTransferQueueRender(view);
}
function isTransferPoolBusy(error){
  return Boolean(error?.transferPoolBusy)||/connection pool is full/i.test(String(error?.message||error||''));
}
function scheduleTransferPoolRetry(view,delay=220){
  const queue=view?.transferQueue;if(!queue||queue.poolRetryTimer)return;
  queue.poolRetryTimer=setTimeout(()=>{
    queue.poolRetryTimer=0;
    processTransferQueue(view);
  },delay);
}
function syncRemoteNavigationAvailability(view){
  const panel=view?.remote;if(!panel)return;
  const full=remoteConnectionPoolFull(view),message='Connection pool full; wait for a scan/transfer to finish before changing remote directory.';
  panel.navigationDisabled=full;
  if(panel.pathInput){panel.pathInput.disabled=full;panel.pathInput.title=full?message:'';}
  if(panel.pathHistorySelect){panel.pathHistorySelect.disabled=full;panel.pathHistorySelect.title=full?message:'Visited paths';}
  if(panel.pathGo){panel.pathGo.disabled=full;panel.pathGo.title=full?message:'Go';}
  if(panel.pathUp){panel.pathUp.disabled=full;panel.pathUp.title=full?message:'Parent directory';}
  if(panel.refresh){panel.refresh.disabled=full;panel.refresh.title=full?message:'Refresh';}
  panel.site?.classList.toggle('ft-remote-pool-full',full);
}
function remoteCacheKey(path){return normalizeRemotePath(path||'.');}
function remoteCacheGet(view,path){return view.remoteCache?.get(remoteCacheKey(path))||null;}
function remoteCacheSet(view,path,data){
  if(!view.remoteCache)view.remoteCache=new Map();
  const key=remoteCacheKey(path);
  const entry={
    path:String(data?.path||key),
    protocol:String(data?.protocol||view.profile.protocol||''),
    entries:(Array.isArray(data?.entries)?data.entries:[]).map(item=>({...item,type:entryType(item)})),
    cachedAt:Date.now()
  };
  view.remoteCache.set(key,entry);return entry;
}
function invalidateRemoteCache(view,path,recursive=false){
  if(!view.remoteCache)return;
  const key=remoteCacheKey(path);
  view.remoteCache.delete(key);
  if(recursive){
    const prefix=key==='/'?'/':key.replace(/\/$/,'')+'/';
    for(const cachedKey of [...view.remoteCache.keys()]){
      if(cachedKey.startsWith(prefix))view.remoteCache.delete(cachedKey);
    }
  }
}
async function fetchRemoteDirectory(view,path,{force=false}={}){
  const key=remoteCacheKey(path);
  if(!force){
    const cached=remoteCacheGet(view,key);
    if(cached)return {...cached,fromCache:true};
  }
  const response=await app.fetchWithLease('/api/file-transfer/list',{
    method:'POST',cache:'no-store',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({profile_id:view.profile.id,path:key})
  });
  if(!response.ok){
    const message=(await response.text()).trim()||response.statusText;
    if(response.status===429&&response.headers.get('X-TaskDeck-Transfer-Pool-Full')==='1'){
      noteTransferPoolFull(view);
      const error=new Error(message||'FTP/SFTP connection pool is full');error.transferPoolBusy=true;throw error;
    }
    throw new Error(message);
  }
  const data=await response.json();
  return {...remoteCacheSet(view,key,data),fromCache:false};
}
function renderRemoteDirectory(view,data){
  const panel=view.remote;
  panel.entries=(Array.isArray(data?.entries)?data.entries:[]).map(item=>({...item,type:entryType(item)}));resetPanelSelection(panel);
  markPathLoaded(panel,String(data?.path||'.'));persistFileTransferSession();
  renderTable(panel,entry=>remoteDoubleClick(view,entry),(entry,event)=>remoteContext(view,entry,event));
  const cached=data?.fromCache?' · cached':'';
  panel.status.textContent=(data?.protocol||view.profile.protocol).toUpperCase()+' · '+panel.entries.length+' item(s)'+cached;
}

async function loadRemoteDirectory(view,path,options={}){
  const panel=view.remote;path=normalizeRemotePath(path||profileFor(view)?.initial_path||'.');
  if(remoteConnectionPoolFull(view)){
    const error=new Error('FTP/SFTP connection pool is full; remote directory navigation is temporarily disabled.');
    panel.status.textContent=error.message;syncRemoteNavigationAvailability(view);throw error;
  }
  const cached=!options.force&&remoteCacheGet(view,path);
  panel.status.textContent=cached?'Opening cached '+path+'…':'Loading '+path+'…';panel.refresh.disabled=true;
  try{
    const data=await fetchRemoteDirectory(view,path,{force:Boolean(options.force)});
    renderRemoteDirectory(view,data);
  }catch(error){panel.status.textContent=String(error?.message||error);throw error;}
  finally{syncRemoteNavigationAvailability(view);updateTransferButtons(view);}
}

async function loadHostDirectory(view,path){
  const panel=view.left;path=normalizeRelativePath(path||'.');panel.status.textContent='Loading host '+path+'…';panel.refresh.disabled=true;
  try{
    const query=path==='.'?'':path;
    const data=await app.jsonFetch('/api/project/tree?path='+encodeURIComponent(query));
    panel.entries=(Array.isArray(data)?data:[]).map(item=>({...item,type:item.type==='dir'?'directory':entryType(item)}));resetPanelSelection(panel);
    markPathLoaded(panel,path);persistFileTransferSession();renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));
    panel.status.textContent='Host workspace · '+panel.entries.length+' item(s)';
  }catch(error){panel.status.textContent=String(error?.message||error);throw error;}
  finally{panel.refresh.disabled=false;updateTransferButtons(view);}
}

async function chooseLocalRoot(view){
  if(typeof globalThis.showDirectoryPicker!=='function')throw new Error('This browser does not support local folder access. Use a Chromium-based browser over HTTPS or localhost.');
  const handle=await globalThis.showDirectoryPicker({mode:'readwrite'});
  const id=localRootID(),record={id,label:handle.name||'Local folder',handle,lastUsed:Date.now()};
  await putLocalRoot(record);safeStorageSet('taskdeck:file-transfer:last-local-root:'+workspaceKey(),id);
  await refreshLocalRoots(view,id);return record;
}

async function refreshLocalRoots(view,selectID=''){
  const panel=view.left,records=await localRootRecords();panel.localRoots=records;
  panel.rootSelect.replaceChildren();
  if(!records.length){const option=document.createElement('option');option.value='';option.textContent='Choose local folder…';panel.rootSelect.append(option);}
  for(const record of records){const option=document.createElement('option');option.value=record.id;option.textContent=record.label||'Local folder';panel.rootSelect.append(option);}
  const preferred=selectID||safeStorageGet('taskdeck:file-transfer:last-local-root:'+workspaceKey(),'')||records[0]?.id||'';
  if(preferred&&records.some(item=>item.id===preferred))panel.rootSelect.value=preferred;
  panel.localRoot=records.find(item=>item.id===panel.rootSelect.value)||null;
  if(panel.localRoot){
    safeStorageSet('taskdeck:file-transfer:last-local-root:'+workspaceKey(),panel.localRoot.id);
    const granted=await ensureHandlePermission(panel.localRoot.handle);
    panel.localPermission=granted;
    panel.grantLocal.hidden=granted;panel.chooseLocal.textContent='Choose…';
    if(granted){panel.localRoot.lastUsed=Date.now();await putLocalRoot(panel.localRoot);}
  }else{panel.localPermission=false;panel.grantLocal.hidden=true;}
}

async function loadLocalDirectory(view,path){
  const panel=view.left;
  if(!panel.localRoot){panel.status.textContent='Choose a local folder first';panel.entries=[];renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));return;}
  const granted=await ensureHandlePermission(panel.localRoot.handle);
  panel.localPermission=granted;
  if(!granted){panel.status.textContent='Local folder permission is required. Click Grant.';panel.entries=[];renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));return;}
  path=normalizeRelativePath(path||'.');panel.status.textContent='Loading local '+path+'…';panel.refresh.disabled=true;
  try{
    const dir=await directoryHandleForPath(panel.localRoot.handle,path),entries=[];
    for await(const [name,handle] of dir.entries()){
      if(handle.kind==='directory')entries.push({name,type:'directory',size:0,modified:''});
      else if(handle.kind==='file'){const file=await handle.getFile();entries.push({name,type:'file',size:file.size,modified:new Date(file.lastModified).toISOString(),handle});}
    }
    panel.entries=entries;resetPanelSelection(panel);panel.currentLocalHandle=dir;markPathLoaded(panel,path);persistFileTransferSession();
    renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));panel.status.textContent='Local · '+(panel.localRoot.label||panel.localRoot.handle.name)+' · '+entries.length+' item(s)';
  }catch(error){panel.status.textContent=String(error?.message||error);throw error;}
  finally{panel.refresh.disabled=false;updateTransferButtons(view);}
}

function currentLeftScope(view){
  const panel=view.left;
  if(panel.source==='host')return 'host:'+workspaceKey();
  return panel.localRoot?'local:'+panel.localRoot.id:'local:none';
}
async function loadLeftDirectory(view,path){
  return view.left.source==='local'?loadLocalDirectory(view,path):loadHostDirectory(view,path);
}

async function switchLeftSource(view,source,startPath='.'){
  const panel=view.left;panel.source=source==='local'?'local':'host';
  safeStorageSet('taskdeck:file-transfer:left-source:'+workspaceKey()+':'+view.profile.id,panel.source);
  panel.rootSelect.hidden=panel.source!=='local';panel.chooseLocal.hidden=panel.source!=='local';
  panel.pathInput.placeholder=panel.source==='local'?'Path relative to selected local folder':'Path relative to TaskDeck workspace';
  startPath=normalizeRelativePath(startPath||'.');
  panel.currentPath=startPath;panel.entries=[];resetPanelSelection(panel);panel.pathInput.value=startPath;
  if(panel.source==='local'){
    if(typeof globalThis.showDirectoryPicker!=='function'){panel.grantLocal.hidden=true;panel.status.textContent='Local browser requires File System Access API (Chromium, HTTPS/localhost).';panel.entries=[];renderTable(panel,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));updateTransferButtons(view);return;}
    await refreshLocalRoots(view);
    panel.grantLocal.hidden=!panel.localRoot||panel.localPermission;
  }else panel.grantLocal.hidden=true;
  panel.refreshPathMemory?.();
  await loadLeftDirectory(view,startPath);
}

async function localSelectedFile(view,entry){
  const panel=view.left;if(panel.source!=='local'||!panel.localRoot)throw new Error('Local folder is not selected');
  const dir=await directoryHandleForPath(panel.localRoot.handle,panel.currentPath);
  const handle=await dir.getFileHandle(entry.name);return handle.getFile();
}

async function uploadBrowserFileToPath(view,file,target,signal=null){
  const form=new FormData();
  form.append('profile_id',view.profile.id);form.append('path',target);form.append('file',file,file.name);
  const response=await app.fetchWithLease('/api/file-transfer/upload',{method:'POST',body:form,cache:'no-store',signal});
  if(!response.ok){
    const message=(await response.text()).trim()||response.statusText;
    if(response.status===429&&response.headers.get('X-TaskDeck-Transfer-Pool-Full')==='1'){
      const error=new Error(message||'FTP/SFTP connection pool is full');error.transferPoolBusy=true;throw error;
    }
    throw new Error(message);
  }
}
async function uploadBrowserFile(view,file){
  return uploadBrowserFileToPath(view,file,joinPath(view.remote.currentPath,file.name,true));
}

async function hostMutation(view,action,path,newPath=''){
  await app.jsonFetch('/api/file-transfer/host-mutate',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({action,path,new_path:newPath})
  });
}
async function renameHostEntry(view,entry){
  const before=String(entry.name||''),answer=prompt('Rename to:',before);if(answer===null)return;
  const name=String(answer).trim();if(!name||name===before)return;
  if(name.includes('/')||name.includes('\\'))throw new Error('New name must not contain path separators');
  await hostMutation(view,'rename',joinPath(view.left.currentPath,before,false),joinPath(view.left.currentPath,name,false));
  await loadHostDirectory(view,view.left.currentPath);
}
async function deleteHostEntries(view,entries){
  const selected=[...(entries||[])];if(!selected.length)return;
  const dirs=selected.filter(entry=>entryType(entry)==='directory').length;
  if(!confirm('Delete '+selected.length+' selected host item(s)?'+(dirs?'\n\nSelected folders are removed non-recursively and must be empty.':'')))return;
  for(const entry of selected)await hostMutation(view,'delete',joinPath(view.left.currentPath,entry.name,false));
  await loadHostDirectory(view,view.left.currentPath);
}
async function newHostFolder(view){
  const answer=prompt('New host folder name:','');if(answer===null)return;
  const name=String(answer).trim();if(!name)return;
  if(name.includes('/')||name.includes('\\'))throw new Error('Folder name must not contain path separators');
  await hostMutation(view,'mkdir',joinPath(view.left.currentPath,name,false));
  await loadHostDirectory(view,view.left.currentPath);
}
async function localDirectoryHandle(view){
  const panel=view.left;if(!panel.localRoot)throw new Error('Choose a local folder first');
  const granted=await ensureHandlePermission(panel.localRoot.handle);if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
  return directoryHandleForPath(panel.localRoot.handle,panel.currentPath);
}
async function renameLocalEntry(view,entry){
  const dir=await localDirectoryHandle(view),handle=entryType(entry)==='directory'?await dir.getDirectoryHandle(entry.name):await dir.getFileHandle(entry.name);
  if(typeof handle.move!=='function')throw new Error('This browser does not support native local rename. Use Host mode or a browser version with FileSystemHandle.move().');
  const answer=prompt('Rename to:',entry.name);if(answer===null)return;
  const name=String(answer).trim();if(!name||name===entry.name)return;
  if(name.includes('/')||name.includes('\\'))throw new Error('New name must not contain path separators');
  await handle.move(name);await loadLocalDirectory(view,view.left.currentPath);
}
async function deleteLocalEntries(view,entries){
  const selected=[...(entries||[])];if(!selected.length)return;
  const dirs=selected.filter(entry=>entryType(entry)==='directory').length;
  if(!confirm('Delete '+selected.length+' selected local item(s)?'+(dirs?'\n\nSelected folders are removed non-recursively and must be empty.':'')))return;
  const dir=await localDirectoryHandle(view);
  for(const entry of selected)await dir.removeEntry(entry.name,{recursive:false});
  await loadLocalDirectory(view,view.left.currentPath);
}
async function newLocalFolder(view){
  const answer=prompt('New local folder name:','');if(answer===null)return;
  const name=String(answer).trim();if(!name)return;
  if(name.includes('/')||name.includes('\\'))throw new Error('Folder name must not contain path separators');
  const dir=await localDirectoryHandle(view);await dir.getDirectoryHandle(name,{create:true});await loadLocalDirectory(view,view.left.currentPath);
}
async function renameLeftEntry(view,entry){
  return view.left.source==='host'?renameHostEntry(view,entry):renameLocalEntry(view,entry);
}
async function deleteLeftEntries(view,entries){
  return view.left.source==='host'?deleteHostEntries(view,entries):deleteLocalEntries(view,entries);
}
async function newLeftFolder(view){
  return view.left.source==='host'?newHostFolder(view):newLocalFolder(view);
}

function pathLeaf(value,remote=false){
  const normalized=remote?normalizeRemotePath(value):normalizeRelativePath(value);
  if(normalized==='.'||normalized==='/')return normalized;
  const parts=normalized.split('/').filter(Boolean);return parts[parts.length-1]||normalized;
}
function pathDepth(value,remote=false){
  const normalized=remote?normalizeRemotePath(value):normalizeRelativePath(value);
  if(normalized==='.'||normalized==='/')return 0;
  return normalized.split('/').filter(Boolean).length;
}
async function fetchHostDirectoryEntries(path){
  path=normalizeRelativePath(path||'.');
  const data=await app.jsonFetch('/api/project/tree?path='+encodeURIComponent(path==='.'?'':path));
  return (Array.isArray(data)?data:[]).map(item=>({...item,type:item.type==='dir'?'directory':entryType(item)}));
}

const maxSyncPlanEntries=10000;
const maxSyncPlanDepth=64;

function syncPlanAdd(map,path,entry){
  path=normalizeRelativePath(path||'.');
  if(path==='.')return;
  if(map.size>=maxSyncPlanEntries)throw new Error('Folder compare exceeds '+maxSyncPlanEntries+' entries. Compare a smaller subtree.');
  if(pathDepth(path,false)>maxSyncPlanDepth)throw new Error('Folder compare exceeds maximum depth '+maxSyncPlanDepth+'.');
  map.set(path,{path,type:entryType(entry),size:Number(entry?.size)||0,modified:String(entry?.modified||'')});
}

async function collectHostSyncTree(base,options={}){
  const result=new Map(),exclude=normalizeSyncExclude(options.exclude||[]);
  const walk=async(path,relative)=>{
    const entries=await fetchHostDirectoryEntries(path);
    for(const entry of entries){
      const rel=relative==='.'?entry.name:joinPath(relative,entry.name,false);
      if(syncPathExcluded(rel,exclude))continue;
      syncPlanAdd(result,rel,entry);
      if(entryType(entry)==='directory')await walk(joinPath(path,entry.name,false),rel);
    }
  };
  await walk(normalizeRelativePath(base||'.'),'.');
  return result;
}

async function collectLocalSyncTree(view,base,options={}){
  const panel=view.left;
  if(!panel.localRoot)throw new Error('Choose a local folder first');
  const granted=await ensureHandlePermission(panel.localRoot.handle);
  if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
  const root=await directoryHandleForPath(panel.localRoot.handle,normalizeRelativePath(base||'.'));
  const result=new Map(),exclude=normalizeSyncExclude(options.exclude||[]);
  const walk=async(handle,relative)=>{
    for await(const [name,child] of handle.entries()){
      const rel=relative==='.'?name:joinPath(relative,name,false);
      if(syncPathExcluded(rel,exclude))continue;
      if(child.kind==='directory'){
        syncPlanAdd(result,rel,{type:'directory',size:0,modified:''});
        await walk(child,rel);
      }else if(child.kind==='file'){
        const file=await child.getFile();
        syncPlanAdd(result,rel,{type:'file',size:file.size,modified:new Date(file.lastModified).toISOString()});
      }
    }
  };
  await walk(root,'.');
  return result;
}

async function collectRemoteSyncTree(view,base,options={}){
  const result=new Map(),exclude=normalizeSyncExclude(options.exclude||[]);
  const walk=async(path,relative)=>{
    const listing=await fetchRemoteDirectory(view,path,{force:true});
    for(const entry of listing.entries){
      const rel=relative==='.'?entry.name:joinPath(relative,entry.name,false);
      if(syncPathExcluded(rel,exclude))continue;
      syncPlanAdd(result,rel,entry);
      if(entryType(entry)==='directory')await walk(joinPath(path,entry.name,true),rel);
    }
  };
  await walk(normalizeRemotePath(base||'.'),'.');
  return result;
}

function syncModifiedTime(value){
  const time=Date.parse(String(value||''));
  return Number.isFinite(time)?time:NaN;
}

async function syncLeftFileSHA256(view,relativePath,options={}){
  const leftBase=normalizeRelativePath(options.left_path||view.left.currentPath||'.');
  const fullPath=joinPath(leftBase,relativePath,false);
  if(view.left.source==='host')return hostFileSHA256(fullPath);
  const root=view.left.localRoot;
  if(!root)throw new Error('Choose a local folder first');
  const granted=await ensureHandlePermission(root.handle);
  if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
  const dir=await directoryHandleForPath(root.handle,parentPath(fullPath,false));
  const handle=await dir.getFileHandle(pathLeaf(fullPath,false));
  return browserFileSHA256(await handle.getFile());
}

async function syncRemoteFileSHA256(view,relativePath,options={}){
  const remoteBase=normalizeRemotePath(options.remote_path||view.remote.currentPath||'.');
  return remoteFileSHA256(view,joinPath(remoteBase,relativePath,true));
}

async function compareSyncTrees(view,left,remote,options={}){
  const compareMode=syncCompareModes.has(String(options.compare_mode||''))?String(options.compare_mode):'metadata';
  const paths=new Set([...left.keys(),...remote.keys()]);
  const rows=[];
  for(const path of [...paths].sort((a,b)=>a.localeCompare(b,undefined,{numeric:true,sensitivity:'base'}))){
    const l=left.get(path)||null,r=remote.get(path)||null;
    let status='same',detail='';
    if(l&&!r)status='left_only';
    else if(!l&&r)status='remote_only';
    else if(l&&r&&l.type!==r.type)status='type_mismatch';
    else if(l?.type==='file'&&r?.type==='file'){
      if(Number(l.size)!==Number(r.size)){
        status='different';detail='size';
      }else if(compareMode==='checksum'){
        const [leftHash,remoteHash]=await Promise.all([
          syncLeftFileSHA256(view,path,options),
          syncRemoteFileSHA256(view,path,options)
        ]);
        if(leftHash!==remoteHash){status='different';detail='checksum';}
        else detail='checksum_same';
      }else{
        const lm=syncModifiedTime(l.modified),rm=syncModifiedTime(r.modified);
        if(Number.isFinite(lm)&&Number.isFinite(rm)&&Math.abs(lm-rm)>2000){
          status='different';detail='modified';
        }
      }
    }
    rows.push({path,status,left:l,remote:r,detail});
  }
  return rows;
}

function syncPlanCounts(rows){
  const counts={left_only:0,remote_only:0,different:0,conflict:0,same:0,type_mismatch:0};
  for(const row of rows)counts[row.status]=(counts[row.status]||0)+1;
  return counts;
}

function topLevelSyncRows(rows,status){
  const paths=new Set(rows.filter(row=>row.status===status).map(row=>row.path));
  return rows.filter(row=>{
    if(row.status!==status)return false;
    let parent=parentPath(row.path,false);
    while(parent!=='.'){
      if(paths.has(parent))return false;
      const next=parentPath(parent,false);if(next===parent)break;parent=next;
    }
    return true;
  });
}

async function syncPlanToRemote(view,rows){
  const selected=rows.filter(row=>(row.status==='left_only'||row.status==='different')&&row.left?.type==='file');
  const directories=rows.filter(row=>row.status==='left_only'&&row.left?.type==='directory')
    .sort((a,b)=>pathDepth(a.path,false)-pathDepth(b.path,false));
  const remoteBase=normalizeRemotePath(view.remote.currentPath||'.');
  const leftBase=normalizeRelativePath(view.left.currentPath||'.');
  const state=newRemoteScanState(remoteBase);
  for(const row of directories)await ensureRemoteScanDirectory(view,joinPath(remoteBase,row.path,true),state);
  if(view.left.source==='host'){
    const groups=new Map();
    for(const row of selected){
      const sourcePath=joinPath(leftBase,row.path,false),targetPath=joinPath(remoteBase,row.path,true);
      const targetDir=parentPath(targetPath,true);
      if(!groups.has(targetDir))groups.set(targetDir,[]);
      groups.get(targetDir).push(sourcePath);
    }
    for(const [remoteDir,hostPaths] of groups)await createServerTransferJob(view,{kind:'host_upload',host_paths:hostPaths,remote_dir:remoteDir});
  }else{
    const root=view.left.localRoot;
    if(!root)throw new Error('Choose a local folder first');
    const granted=await ensureHandlePermission(root.handle);if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
    const localState={jobID:newFileTransferJobID(),conflictPolicy:effectiveDirectionConflictPolicy(view,'upload'),files:0};
    for(const row of selected){
      const sourcePath=joinPath(leftBase,row.path,false),targetPath=joinPath(remoteBase,row.path,true);
      const dir=await directoryHandleForPath(root.handle,parentPath(sourcePath,false));
      const handle=await dir.getFileHandle(pathLeaf(sourcePath,false));
      enqueueLocalUploadHandle(view,handle,sourcePath,targetPath,localState);
    }
  }
  return selected.length;
}

async function syncPlanToLeft(view,rows){
  const selected=rows.filter(row=>(row.status==='remote_only'||row.status==='different')&&row.remote?.type==='file');
  const leftBase=normalizeRelativePath(view.left.currentPath||'.');
  const remoteBase=normalizeRemotePath(view.remote.currentPath||'.');
  if(view.left.source==='host'){
    const groups=new Map();
    for(const row of selected){
      const remotePath=joinPath(remoteBase,row.path,true),leftPath=joinPath(leftBase,row.path,false);
      const hostDir=parentPath(leftPath,false);
      if(!groups.has(hostDir))groups.set(hostDir,[]);
      groups.get(hostDir).push({path:remotePath,directory:false,size:Number(row.remote?.size)||0,modified:String(row.remote?.modified||'')});
    }
    for(const [hostDir,remoteTargets] of groups)await createServerTransferJob(view,{kind:'host_download',remote_targets:remoteTargets,host_dir:hostDir});
  }else{
    const root=view.left.localRoot;if(!root)throw new Error('Choose a local folder first');
    const granted=await ensureHandlePermission(root.handle);if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
    const state={
      source:'local',base:leftBase,localRoot:root,hostDirectories:new Set(['.',leftBase]),hostListings:new Map(),
      localHandles:new Map([['.',root.handle]]),files:0,folders:0,view,
      jobID:newFileTransferJobID(),conflictPolicy:effectiveDirectionConflictPolicy(view,'download')
    };
    const directories=rows.filter(row=>row.status==='remote_only'&&row.remote?.type==='directory')
      .sort((a,b)=>pathDepth(a.path,false)-pathDepth(b.path,false));
    for(const row of directories)await ensureLeftScanDirectory(view,joinPath(leftBase,row.path,false),state);
    for(const row of selected){
      enqueueRemoteDownloadFile(
        view,joinPath(remoteBase,row.path,true),joinPath(leftBase,row.path,false),
        row.remote?.size,state,row.remote?.modified
      );
    }
  }
  return selected.length;
}

async function mirrorDeleteRemoteOnly(view,rows){
  const remoteBase=normalizeRemotePath(view.remote.currentPath||'.');
  const targets=topLevelSyncRows(rows,'remote_only').map(row=>({
    path:joinPath(remoteBase,row.path,true),directory:row.remote?.type==='directory'
  }));
  if(targets.length)await createServerTransferJob(view,{kind:'remote_delete',remote_targets:targets});
  return targets.length;
}

async function mirrorDeleteLeftOnly(view,rows){
  const leftBase=normalizeRelativePath(view.left.currentPath||'.');
  const selected=rows.filter(row=>row.status==='left_only').sort((a,b)=>pathDepth(b.path,false)-pathDepth(a.path,false));
  if(view.left.source==='host'){
    for(const row of selected)await hostMutation(view,'delete',joinPath(leftBase,row.path,false));
  }else{
    const root=view.left.localRoot;if(!root)throw new Error('Choose a local folder first');
    const granted=await ensureHandlePermission(root.handle);if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
    for(const row of selected){
      const full=joinPath(leftBase,row.path,false),parent=parentPath(full,false),name=pathLeaf(full,false);
      const dir=await directoryHandleForPath(root.handle,parent);
      await dir.removeEntry(name,{recursive:false});
    }
  }
  return selected.length;
}

function syncDirectionLabel(direction){
  return ({
    right:'Sync Left → Remote',
    left:'Sync Remote → Left',
    bidirectional:'Bidirectional Sync',
    mirror_right:'Mirror Left → Remote',
    mirror_left:'Mirror Remote → Left'
  })[String(direction||'')]||'Sync Left → Remote';
}

async function runFolderSyncDirection(view,rows,options={}){
  const direction=syncDirections.has(String(options.direction||''))?String(options.direction):'right';
  if(direction==='right')return syncPlanToRemote(view,rows);
  if(direction==='left')return syncPlanToLeft(view,rows);
  if(direction==='bidirectional'){
    const safeRows=rows.map(row=>row.status==='different'?{...row,status:'conflict'}:row);
    const right=await syncPlanToRemote(view,safeRows);
    const left=await syncPlanToLeft(view,safeRows);
    return right+left;
  }
  if(!options.allow_delete)throw new Error('Mirror delete is disabled. Enable "Allow destination deletes" in the Sync setup first.');
  if(direction==='mirror_right'){
    if(!confirm('Mirror Left → Remote?\n\nRemote-only files/folders will be deleted after copy/update. Type mismatches and bidirectional conflicts are never deleted automatically.'))return 0;
    const copied=await syncPlanToRemote(view,rows);
    const removed=await mirrorDeleteRemoteOnly(view,rows);
    return copied+removed;
  }
  if(direction==='mirror_left'){
    if(!confirm('Mirror Remote → Left?\n\nLeft-only files/folders will be deleted after copy/update. Type mismatches and bidirectional conflicts are never deleted automatically.'))return 0;
    const copied=await syncPlanToLeft(view,rows);
    const removed=await mirrorDeleteLeftOnly(view,rows);
    return copied+removed;
  }
  return 0;
}

function openFolderSyncDryRun(view,rows,options={}){
  options=normalizeSyncProfile({...currentSyncOptions(view),...options},view);
  if(options.direction==='bidirectional')rows=rows.map(row=>row.status==='different'?{...row,status:'conflict'}:row);
  const counts=syncPlanCounts(rows);
  const actionable=counts.left_only+counts.remote_only+counts.different;
  const backdrop=document.createElement('div');backdrop.className='ft-sync-backdrop';
  const dialog=document.createElement('div');dialog.className='ft-sync-dialog';
  const title=document.createElement('h3');title.textContent='Folder Sync / Mirror — Dry run';
  const note=document.createElement('div');note.className='ft-sync-note';
  note.textContent=(options.compare_mode==='checksum'
    ?'Recursive SHA-256 comparison for matching-size files; size differences short-circuit hashing.'
    :'Recursive metadata comparison (type, size, modified time when both sides provide it).')
    +' Dry run never changes files. Selected action: '+syncDirectionLabel(options.direction)+'.'
    +(options.direction==='bidirectional'?' Files changed on both sides are marked Conflict and are not copied automatically.':'');
  const summary=document.createElement('div');summary.className='ft-sync-summary';
  summary.textContent='Left only '+counts.left_only+' · Remote only '+counts.remote_only+' · Different '+counts.different+' · Conflict '+counts.conflict+' · Same '+counts.same+' · Type mismatch '+counts.type_mismatch;
  const wrap=document.createElement('div');wrap.className='ft-sync-table-wrap';
  const table=document.createElement('table');table.className='ft-sync-table';
  const thead=document.createElement('thead'),hr=document.createElement('tr');
  for(const label of ['Status','Path','Left','Remote']){const th=document.createElement('th');th.textContent=label;hr.append(th);}
  thead.append(hr);const tbody=document.createElement('tbody');
  const labels={left_only:'Left only',remote_only:'Remote only',different:'Different',conflict:'Conflict',same:'Same',type_mismatch:'Type mismatch'};
  for(const row of rows.slice(0,maxSyncPlanEntries)){
    const tr=document.createElement('tr');
    const status=document.createElement('td');status.className='ft-sync-status';status.textContent=labels[row.status]||row.status;
    const path=document.createElement('td');path.className='ft-sync-path';path.textContent=row.path;path.title=row.path;
    const left=document.createElement('td');left.textContent=row.left?(row.left.type+(row.left.type==='file'?' · '+formatSize(row.left.size):'')):'—';
    const remote=document.createElement('td');remote.textContent=row.remote?(row.remote.type+(row.remote.type==='file'?' · '+formatSize(row.remote.size):'')):'—';
    tr.append(status,path,left,remote);tbody.append(tr);
  }
  table.append(thead,tbody);wrap.append(table);
  const actions=document.createElement('div');actions.className='ft-sync-actions';
  const close=document.createElement('button');close.type='button';close.textContent='Close';close.onclick=()=>backdrop.remove();
  const runButton=(label,handler,{danger=false}={})=>{
    const button=document.createElement('button');button.type='button';button.textContent=label;if(danger)button.classList.add('ft-sync-danger');
    button.disabled=!actionable;button.onclick=async()=>{
      button.disabled=true;
      try{await handler();backdrop.remove();await loadRemoteDirectory(view,view.remote.currentPath,{force:true});await loadLeftDirectory(view,view.left.currentPath);}
      catch(error){app.showError(error);}
      finally{if(button.isConnected)button.disabled=false;}
    };
    return button;
  };
  const selectedAction=runButton('Run: '+syncDirectionLabel(options.direction),()=>runFolderSyncDirection(view,rows,options),{
    danger:options.direction==='mirror_right'||options.direction==='mirror_left'
  });
  if((options.direction==='mirror_right'||options.direction==='mirror_left')&&!options.allow_delete){
    selectedAction.disabled=true;
    selectedAction.title='Enable "Allow destination deletes" in Sync setup to run Mirror';
  }
  actions.append(close,selectedAction);
  dialog.append(title,note,summary,wrap,actions);backdrop.append(dialog);document.body.append(backdrop);close.focus();
  backdrop.addEventListener('pointerdown',event=>{if(event.target===backdrop)backdrop.remove();});
  backdrop.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();backdrop.remove();}});
}

async function compareFoldersDryRun(view,options={}){
  options=normalizeSyncProfile({...currentSyncOptions(view),...options},view);
  view.left.status.textContent='Scanning left folder for dry-run compare…';
  const scanOptions={exclude:options.exclude};
  const left=view.left.source==='host'
    ?await collectHostSyncTree(options.left_path,scanOptions)
    :await collectLocalSyncTree(view,options.left_path,scanOptions);
  view.remote.status.textContent='Scanning remote folder for dry-run compare…';
  const remote=await collectRemoteSyncTree(view,options.remote_path,scanOptions);
  if(options.compare_mode==='checksum'){
    view.left.status.textContent='Comparing matching-size files by SHA-256…';
    view.remote.status.textContent='Comparing matching-size files by SHA-256…';
  }
  const rows=await compareSyncTrees(view,left,remote,options);
  view.left.status.textContent='Dry-run compare complete · '+left.size+' left item(s)';
  view.remote.status.textContent='Dry-run compare complete · '+remote.size+' remote item(s)';
  openFolderSyncDryRun(view,rows,options);
}

function newRemoteScanState(basePath){
  const base=normalizeRemotePath(basePath||'.');
  return {base,remoteDirectories:new Set(['.','/',base]),remoteListings:new Map(),files:0,folders:0,errors:0};
}
async function ensureRemoteScanDirectory(view,target,state){
  target=normalizeRemotePath(target);
  if(target==='.'||target==='/'||state.remoteDirectories.has(target))return;
  const parent=parentPath(target,true);
  await ensureRemoteScanDirectory(view,parent,state);
  const name=pathLeaf(target,true);
  let entries=state.remoteListings.get(parent);
  if(!entries){
    const listing=await fetchRemoteDirectory(view,parent);
    entries=listing.entries.map(item=>({...item,type:entryType(item)}));
    state.remoteListings.set(parent,entries);
  }
  let existing=entries.find(item=>item.name===name);
  if(existing){
    if(entryType(existing)!=='directory')throw new Error('Remote path exists and is not a folder: '+target);
    state.remoteDirectories.add(target);
    if(!state.remoteListings.has(target)){
      const cached=remoteCacheGet(view,target);
      if(cached)state.remoteListings.set(target,cached.entries.map(item=>({...item,type:entryType(item)})));
    }
    return;
  }
  try{
    await mutateRemoteRequest(view,'mkdir',target,'',true);
  }catch(error){
    const listing=await fetchRemoteDirectory(view,parent,{force:true});
    entries=listing.entries.map(item=>({...item,type:entryType(item)}));
    state.remoteListings.set(parent,entries);
    existing=entries.find(item=>item.name===name);
    if(!existing||entryType(existing)!=='directory')throw error;
    state.remoteDirectories.add(target);return;
  }
  entries.push({name,type:'directory',size:0,modified:''});
  state.remoteListings.set(target,[]);
  state.remoteDirectories.add(target);state.folders++;
  markRemoteQueueDirty(view,parent);
}
function enqueueHostUploadFile(view,sourcePath,targetPath,size,state){
  state.files++;
  enqueueTransferTasks(view,[{
    direction:'→',kind:'Upload',source:sourcePath,target:targetPath,size:Number(size)||0,
    run:async()=>{
      await app.jsonFetch('/api/file-transfer/host-to-remote',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({
        profile_id:view.profile.id,host_path:sourcePath,remote_path:targetPath
      })});
      markRemoteQueueDirty(view,parentPath(targetPath,true));
    }
  }]);
}
function enqueueLocalUploadHandle(view,handle,sourcePath,targetPath,state){
  state.files++;
  const jobID=String(state?.jobID||''),policy=localJobConflictPolicy(jobID)!=='ask'?localJobConflictPolicy(jobID):normalizeConflictPolicy(state?.conflictPolicy);
  const spec={
    kind:'local_upload',profile_id:String(view.profile.id||''),root_id:String(view.left.localRoot?.id||''),
    source_path:normalizeRelativePath(sourcePath),target_path:normalizeRemotePath(targetPath),
    job_id:jobID
  };
  enqueueTransferTasks(view,[{
    id:newFileTransferJobID(),direction:'→',kind:'Upload',source:sourcePath,target:targetPath,size:0,
    conflictJobID:jobID,conflictPolicy:policy,persistSpec:spec,
    run:item=>persistentLocalTransferRun(view,spec,item)
  }]);
}
async function scanHostUploadEntry(view,parent,entry,remoteParent,state){
  assertTransferScanActive(view,state?.scanGeneration);
  const sourcePath=joinPath(parent,entry.name,false),targetPath=joinPath(remoteParent,entry.name,true);
  if(entryType(entry)!=='directory'){
    enqueueHostUploadFile(view,sourcePath,targetPath,entry.size,state);return;
  }
  await ensureRemoteScanDirectory(view,targetPath,state);
  const children=await fetchHostDirectoryEntries(sourcePath);assertTransferScanActive(view,state?.scanGeneration);
  for(const child of children){assertTransferScanActive(view,state?.scanGeneration);await scanHostUploadEntry(view,sourcePath,child,targetPath,state);}
}
async function scanLocalUploadHandle(view,handle,targetPath,sourcePath,state){
  assertTransferScanActive(view,state?.scanGeneration);
  if(handle.kind!=='directory'){
    enqueueLocalUploadHandle(view,handle,sourcePath,targetPath,state);return;
  }
  await ensureRemoteScanDirectory(view,targetPath,state);
  for await(const [name,child] of handle.entries()){
    assertTransferScanActive(view,state?.scanGeneration);
    await scanLocalUploadHandle(view,child,joinPath(targetPath,name,true),joinPath(sourcePath,name,false),state);
  }
}
async function runPersistentLocalUploadScan(view,scan){
  if(!scan?.id||activeLocalTransferScans.has(scan.id))return;
  activeLocalTransferScans.add(scan.id);
  try{
    const root=await getLocalRoot(String(scan.root_id||''));
    if(!root?.handle)throw new Error('Saved Local folder is unavailable. Choose the folder again to resume scanning.');
    if(!await ensureHandlePermission(root.handle))throw new Error('Local folder permission is required after reload. Click Grant to resume scanning.');
    view.left.localRoot=root;
    const state=newRemoteScanState(scan.remote_base||'.');
    state.jobID=String(scan.id||'');state.conflictPolicy=normalizeConflictPolicy(scan.conflict_policy);
    await runTransferScan(view,'Upload scan',async control=>{
      state.scanGeneration=control.generation;
      for(const selected of scan.selected||[]){
        control.check();
        const sourcePath=normalizeRelativePath(selected.source_path||'.');
        const parent=parentPath(sourcePath,false),name=pathLeaf(sourcePath,false);
        const dir=await directoryHandleForPath(root.handle,parent);
        const handle=selected.directory?await dir.getDirectoryHandle(name):await dir.getFileHandle(name);
        await scanLocalUploadHandle(view,handle,normalizeRemotePath(selected.target_path),sourcePath,state);
      }
    });
    removePersistentLocalScan(scan.id);
    view.remote.status.textContent='Local upload scan complete · '+state.files+' file(s) discovered';
  }catch(error){
    if(error?.transferScanStopped){removePersistentLocalScan(scan.id);view.remote.status.textContent='Upload scan stopped';return;}
    view.remote.status.textContent=String(error?.message||error);
    throw error;
  }finally{activeLocalTransferScans.delete(scan.id);}
}
async function runPersistentLocalDownloadScan(view,scan){
  if(!scan?.id||activeLocalTransferScans.has(scan.id))return;
  activeLocalTransferScans.add(scan.id);
  try{
    const root=await getLocalRoot(String(scan.root_id||''));
    if(!root?.handle)throw new Error('Saved Local folder is unavailable. Choose the folder again to resume scanning.');
    if(!await ensureHandlePermission(root.handle))throw new Error('Local folder permission is required after reload. Click Grant to resume scanning.');
    view.left.localRoot=root;
    const state={
      source:'local',base:normalizeRelativePath(scan.left_base||'.'),localRoot:root,
      hostDirectories:new Set(['.',normalizeRelativePath(scan.left_base||'.')]),hostListings:new Map(),
      localHandles:new Map([['.',root.handle]]),files:0,folders:0,view,
      jobID:String(scan.id||''),conflictPolicy:normalizeConflictPolicy(scan.conflict_policy)
    };
    await runTransferScan(view,'Download scan',async control=>{
      state.scanGeneration=control.generation;
      for(const selected of scan.selected||[]){
        control.check();
        const remotePath=normalizeRemotePath(selected.remote_path);
        const remoteParent=parentPath(remotePath,true);
        const entry={name:pathLeaf(remotePath,true),type:selected.directory?'directory':'file',size:Number(selected.size)||0,modified:String(selected.modified||'')};
        await scanRemoteDownloadEntry(view,remoteParent,entry,state.base,state);
      }
    });
    removePersistentLocalScan(scan.id);
    view.left.status.textContent='Local download scan complete · '+state.files+' file(s) discovered';
  }catch(error){
    if(error?.transferScanStopped){removePersistentLocalScan(scan.id);view.left.status.textContent='Download scan stopped';return;}
    view.left.status.textContent=String(error?.message||error);
    throw error;
  }finally{activeLocalTransferScans.delete(scan.id);}
}
async function resumePersistentLocalScans(){
  for(const scan of readPersistentLocalScans()){
    const view=views.get(String(scan.profile_id||''));if(!view)continue;
    const runner=scan.kind==='local_upload_scan'?runPersistentLocalUploadScan:
      scan.kind==='local_download_scan'?runPersistentLocalDownloadScan:null;
    if(runner)runner(view,scan).catch(error=>console.warn('Cannot resume Local browser transfer scan',error));
  }
}

const compressedUploadScanFileThreshold=200;
const compressedUploadScanSoftThreshold=100;
const compressedUploadScanDurationMS=2500;

function selectionContainsFolder(entries){return (entries||[]).some(entry=>entryType(entry)==='directory');}
function selectedArchiveRoots(entries){return [...new Set((entries||[]).map(entry=>String(entry?.name||'').trim()).filter(Boolean))];}
function compressedUploadRootsClear(view,entries){
  const names=new Set((view.remote?.entries||[]).map(entry=>String(entry?.name||'')));
  return selectedArchiveRoots(entries).every(name=>!names.has(name));
}
function canAutoExtractCompressedUpload(view){
  return String(view.profile?.protocol||'').toLowerCase()==='sftp'&&(!app.sharedMode||Boolean(app.hasPermission?.('ssh.use')));
}
function shellSingleQuote(value){return "'"+String(value||'').replaceAll("'","'\\''")+"'";}
function powerShellSingleQuote(value){return "'"+String(value||'').replaceAll("'","''")+"'";}
function compressedUploadManualCommands(remoteArchive,remoteDir,roots,mergePolicy='fail'){
  const overwrite=mergePolicy==='overwrite';
  const archive=shellSingleQuote(remoteArchive),dest=shellSingleQuote(remoteDir);
  const checks=overwrite?'':roots.map(root=>'test ! -e '+shellSingleQuote(joinPath(remoteDir,root,true))+'; ').join('');
  const posix='set -eu; test -d '+dest+'; '+checks+'tar -xzf '+archive+' -C '+dest+' && rm -f -- '+archive;
  const psArchive=powerShellSingleQuote(remoteArchive),psDest=powerShellSingleQuote(remoteDir);
  const psChecks=overwrite?'':roots.map(root=>"if (Test-Path -LiteralPath (Join-Path $dest "+powerShellSingleQuote(root)+")) { throw "+powerShellSingleQuote('Destination entry already exists: '+root)+" }; ").join('');
  const powershell='$archive='+psArchive+'; $dest='+psDest+"; if (-not (Test-Path -LiteralPath $dest -PathType Container)) { throw 'Destination directory not found' }; "+psChecks+"tar -xzf $archive -C $dest; if ($LASTEXITCODE -ne 0) { throw 'tar extraction failed' }; Remove-Item -LiteralPath $archive";
  return {posix,powershell};
}
async function quickScanHostUploadSelection(view,entries){
  const base=normalizeRelativePath(view.left.currentPath||'.');
  return app.jsonFetch('/api/file-transfer/upload-scan',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({paths:entries.map(entry=>joinPath(base,entry.name,false))})
  });
}
async function localUploadEntryHandle(view,entry){
  if(entry?.handle)return entry.handle;
  const root=view.left.localRoot;if(!root?.handle)throw new Error('Choose a local folder first');
  const dir=await directoryHandleForPath(root.handle,normalizeRelativePath(view.left.currentPath||'.'));
  return entryType(entry)==='directory'?dir.getDirectoryHandle(entry.name):dir.getFileHandle(entry.name);
}
async function quickScanLocalUploadSelection(view,entries){
  const started=performance.now(),deadline=started+compressedUploadScanDurationMS;
  const result={files:0,dirs:0,bytes:0,complete:true,timed_out:false,many_files:false,elapsed_ms:0};
  let stop=false;
  const visit=async handle=>{
    if(stop)return;
    if(performance.now()>deadline){result.complete=false;result.timed_out=true;stop=true;return;}
    if(handle.kind==='file'){
      result.files++;
      try{const file=await handle.getFile();result.bytes+=Number(file.size)||0;}catch{}
      if(result.files>=compressedUploadScanFileThreshold){result.complete=false;stop=true;}
      return;
    }
    result.dirs++;
    for await(const child of handle.values()){await visit(child);if(stop)break;}
  };
  for(const entry of entries){await visit(await localUploadEntryHandle(view,entry));if(stop)break;}
  result.many_files=result.files>=compressedUploadScanFileThreshold||(result.timed_out&&result.files>=compressedUploadScanSoftThreshold);
  result.elapsed_ms=Math.round(performance.now()-started);
  return result;
}
function requestCompressedUploadDecision(view,entries,scan,{archiveAllowed=true,rootsExist=false,reason=''}={}){
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='ft-upload-choice-backdrop';
    const box=document.createElement('div');box.className='ft-upload-choice';box.setAttribute('role','dialog');box.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='Many files detected — optimize folder upload?';
    const count=Number(scan?.files)||0,partial=!scan?.complete;
    const intro=document.createElement('p');intro.textContent='Quick scan found '+(partial?'at least ':'')+count+' file(s) in '+Math.max(0,Number(scan?.elapsed_ms)||0)+' ms. Uploading one archive can be much faster than opening a remote transfer for every small file.';
    const method=document.createElement('div');method.className='ft-upload-choice-note';
    const protocol=String(view.profile?.protocol||'').toUpperCase();
    const auto=canAutoExtractCompressedUpload(view);
    if(!archiveAllowed){
      method.textContent='Compressed mode is not available: '+reason+'\nUse normal upload instead.';
    }else if(rootsExist){
      method.textContent='Some selected top-level names already exist in Remote. Compressed mode remains available, but extraction will MERGE directories and OVERWRITE same-named files.\nChoose Upload normally if you need per-file conflict decisions.';
    }else{
      method.textContent='Compressed mode: create one tar.gz → upload via '+protocol+' → '+(auto?'extract automatically through the linked SSH profile.':'show extraction commands for you to run manually.')+'\nNormal upload remains available and keeps the existing per-file conflict/retry behavior.';
    }
    const actions=document.createElement('div');actions.className='ft-upload-choice-actions';
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
    const normal=document.createElement('button');normal.type='button';normal.textContent='Upload normally';
    const archive=document.createElement('button');archive.type='button';
    archive.textContent=rootsExist?(auto?'Compress + merge/overwrite':'Compress + upload for merge'):(auto?'Compress + upload + extract':'Compress + upload');
    archive.disabled=!archiveAllowed;
    const finish=value=>{backdrop.remove();resolve(value);};
    cancel.onclick=()=>finish('cancel');normal.onclick=()=>finish('normal');
    archive.onclick=()=>finish(rootsExist?'archive-overwrite':'archive');
    actions.append(cancel,normal,archive);box.append(title,intro,method,actions);backdrop.append(box);document.body.append(backdrop);
    backdrop.onpointerdown=event=>{if(event.target===backdrop)finish('cancel');};
    setTimeout(()=>normal.focus(),0);
  });
}
function showManualExtractCommands(view,remoteArchive,remoteDir,roots,commands=null){
  commands=commands||compressedUploadManualCommands(remoteArchive,remoteDir,roots);
  return new Promise(resolve=>{
    const backdrop=document.createElement('div');backdrop.className='ft-upload-choice-backdrop';
    const box=document.createElement('div');box.className='ft-upload-choice';box.setAttribute('role','dialog');box.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='Archive uploaded — manual extraction required';
    const note=document.createElement('p');
    const ftpPathWarning=String(view?.profile?.protocol||'').toLowerCase()==='ftp'?' FTP paths may be chroot/virtual paths; adjust the shell paths if your FTP root is mapped differently on the server.':'';
    note.textContent='TaskDeck cannot run a remote shell for this FTP/SFTP session. Archive: '+remoteArchive+' · Destination: '+remoteDir+'. Run one of these commands on the remote server. The command applies the merge policy selected for this upload and removes the archive only after successful extraction.'+ftpPathWarning;
    const posixLabel=document.createElement('p');posixLabel.textContent='POSIX shell:';
    const posix=document.createElement('textarea');posix.className='ft-upload-command';posix.readOnly=true;posix.value=commands.posix||'';
    const copyPosix=document.createElement('button');copyPosix.type='button';copyPosix.textContent='Copy POSIX command';copyPosix.onclick=()=>copyText(posix.value);
    const psLabel=document.createElement('p');psLabel.textContent='PowerShell:';
    const powershell=document.createElement('textarea');powershell.className='ft-upload-command';powershell.readOnly=true;powershell.value=commands.powershell||'';
    const copyPS=document.createElement('button');copyPS.type='button';copyPS.textContent='Copy PowerShell command';copyPS.onclick=()=>copyText(powershell.value);
    const actions=document.createElement('div');actions.className='ft-upload-choice-actions';
    const done=document.createElement('button');done.type='button';done.textContent='Done';done.onclick=()=>{backdrop.remove();resolve();};
    actions.append(copyPosix,copyPS,done);box.append(title,note,posixLabel,posix,psLabel,powershell,actions);backdrop.append(box);document.body.append(backdrop);
    setTimeout(()=>done.focus(),0);
  });
}
function tarTextBytes(value){return new TextEncoder().encode(String(value||''));}
function tarWriteText(buffer,offset,length,value){
  const bytes=tarTextBytes(value);if(bytes.length>length)throw new Error('Archive path metadata is too long');
  buffer.set(bytes,offset);
}
function tarWriteOctal(buffer,offset,length,value){
  const text=Math.max(0,Number(value)||0).toString(8).padStart(length-1,'0').slice(-(length-1));
  tarWriteText(buffer,offset,length-1,text);buffer[offset+length-1]=0;
}
function tarPathParts(path){
  path=String(path||'').replace(/^\.\//,'').replace(/\\/g,'/');
  if(!path||path.startsWith('/')||path.split('/').some(part=>!part||part==='.'||part==='..'))throw new Error('Unsafe archive path: '+path);
  if(tarTextBytes(path).length<=100)return {name:path,prefix:''};
  for(let i=path.lastIndexOf('/');i>0;i=path.lastIndexOf('/',i-1)){
    const prefix=path.slice(0,i),name=path.slice(i+1);
    if(tarTextBytes(name).length<=100&&tarTextBytes(prefix).length<=155)return {name,prefix};
  }
  throw new Error('Archive path is too long for built-in tar: '+path);
}
function tarHeader(path,size,mtime,directory=false){
  const header=new Uint8Array(512),parts=tarPathParts(path);
  tarWriteText(header,0,100,parts.name);tarWriteOctal(header,100,8,directory?0o755:0o644);
  tarWriteOctal(header,108,8,0);tarWriteOctal(header,116,8,0);tarWriteOctal(header,124,12,directory?0:size);
  tarWriteOctal(header,136,12,Math.floor((Number(mtime)||Date.now())/1000));
  header.fill(32,148,156);header[156]=directory?'5'.charCodeAt(0):'0'.charCodeAt(0);
  tarWriteText(header,257,6,'ustar');header[262]=0;tarWriteText(header,263,2,'00');
  if(parts.prefix)tarWriteText(header,345,155,parts.prefix);
  let sum=0;for(const byte of header)sum+=byte;
  const checksum=sum.toString(8).padStart(6,'0').slice(-6);tarWriteText(header,148,6,checksum);header[154]=0;header[155]=32;
  return header;
}
function tarPadding(size){const remaining=Number(size)%512;return remaining?new Uint8Array(512-remaining):null;}
function compressedUploadAbortError(){
  const error=new Error('Compressed upload cancelled');error.name='AbortError';return error;
}
function assertCompressedUploadActive(signal){if(signal?.aborted)throw compressedUploadAbortError();}
function updateLocalCompressedItem(view,item,detail,files=0,bytes=0,size=0){
  if(!item)return;
  item.detail=detail;item.files=files;item.bytesDone=bytes;if(size>0)item.size=size;
  scheduleTransferQueueRender(view);
}
async function* localTarEntryChunks(handle,path,state){
  assertCompressedUploadActive(state?.signal);
  if(handle.kind==='directory'){
    yield tarHeader(path,0,Date.now(),true);
    for await(const [name,child] of handle.entries()){
      assertCompressedUploadActive(state?.signal);
      yield* localTarEntryChunks(child,path+'/'+name,state);
    }
    return;
  }
  const file=await handle.getFile();
  state.files++;state.bytes+=Number(file.size)||0;
  const now=performance.now();
  if(now-state.lastReport>250){
    state.lastReport=now;updateLocalCompressedItem(state.view,state.item,'Compressing · '+state.files+' files · '+formatSize(state.bytes),state.files,state.bytes);
  }
  yield tarHeader(path,file.size,file.lastModified,false);
  const reader=file.stream().getReader();
  try{
    while(true){
      assertCompressedUploadActive(state?.signal);
      const part=await reader.read();if(part.done)break;if(part.value?.byteLength)yield part.value;
    }
  }finally{reader.releaseLock();}
  const padding=tarPadding(file.size);if(padding)yield padding;
}
function readableStreamFromAsyncGenerator(generator){
  const iterator=generator[Symbol.asyncIterator]();
  return new ReadableStream({
    async pull(controller){try{const next=await iterator.next();if(next.done)controller.close();else controller.enqueue(next.value);}catch(error){controller.error(error);}},
    async cancel(){if(iterator.return)await iterator.return();}
  });
}
async function buildLocalSelectionTarGz(view,entries,item=null,signal=null){
  if(typeof CompressionStream!=='function')throw new Error('This browser does not provide native gzip compression.');
  const state={view,item,signal,files:0,bytes:0,lastReport:0};
  async function* chunks(){
    for(const entry of entries){
      assertCompressedUploadActive(signal);
      yield* localTarEntryChunks(await localUploadEntryHandle(view,entry),entry.name,state);
    }
    yield new Uint8Array(1024);
  }
  updateLocalCompressedItem(view,item,'Compressing selected folders',0,0);
  const stream=readableStreamFromAsyncGenerator(chunks()).pipeThrough(new CompressionStream('gzip'));
  const blob=await new Response(stream).blob();
  assertCompressedUploadActive(signal);
  const stamp=new Date().toISOString().replace(/[-:TZ.]/g,'').slice(0,14);
  const random=Array.from(crypto.getRandomValues(new Uint8Array(4)),byte=>byte.toString(16).padStart(2,'0')).join('');
  updateLocalCompressedItem(view,item,'Compressed · '+state.files+' files · '+formatSize(blob.size),state.files,state.bytes,blob.size);
  return new File([blob],'.taskdeck-folder-'+stamp+'-'+random+'.tar.gz',{type:'application/gzip',lastModified:Date.now()});
}
async function autoExtractCompressedUpload(view,remoteArchive,remoteDir,roots,mergePolicy='fail',signal=null){
  const response=await app.fetchWithLease('/api/file-transfer/archive-extract',{
    method:'POST',headers:{'Content-Type':'application/json'},signal,
    body:JSON.stringify({profile_id:view.profile.id,remote_archive:remoteArchive,remote_destination:remoteDir,roots,format:'tar.gz',merge_policy:mergePolicy})
  });
  if(!response.ok)throw new Error((await response.text()).trim()||('HTTP '+response.status));
  return response.json();
}
async function runLocalCompressedUpload(view,entries,remoteDir,roots,mergePolicy,item,signal){
  const archive=await buildLocalSelectionTarGz(view,entries,item,signal);
  const remoteArchive=joinPath(remoteDir,archive.name,true);
  assertCompressedUploadActive(signal);
  updateLocalCompressedItem(view,item,'Uploading archive · '+formatSize(archive.size),item.files||0,item.bytesDone||0,archive.size);
  await uploadBrowserFileToPath(view,archive,remoteArchive,signal);
  const commands=compressedUploadManualCommands(remoteArchive,remoteDir,roots,mergePolicy);
  invalidateRemoteCache(view,remoteDir);
  assertCompressedUploadActive(signal);
  if(canAutoExtractCompressedUpload(view)){
    updateLocalCompressedItem(view,item,'Extracting archive through SSH',item.files||0,item.bytesDone||0,archive.size);
    try{
      await autoExtractCompressedUpload(view,remoteArchive,remoteDir,roots,mergePolicy,signal);
      updateLocalCompressedItem(view,item,'Completed · '+(item.files||0)+' files',item.files||0,item.bytesDone||0,archive.size);
      markRemoteQueueDirty(view,remoteDir);return;
    }catch(error){
      if(error?.name==='AbortError'||signal?.aborted)throw compressedUploadAbortError();
      updateLocalCompressedItem(view,item,'Auto-extract failed · manual extraction required',item.files||0,item.bytesDone||0,archive.size);
      await showManualExtractCommands(view,remoteArchive,remoteDir,roots,commands);
      markRemoteQueueDirty(view,remoteDir);return;
    }
  }
  updateLocalCompressedItem(view,item,'Archive uploaded · manual extraction required',item.files||0,item.bytesDone||0,archive.size);
  await showManualExtractCommands(view,remoteArchive,remoteDir,roots,commands);
  markRemoteQueueDirty(view,remoteDir);
}
async function uploadCompressedSelection(view,entries,mergePolicy='fail'){
  const source=view.left.source,remoteDir=normalizeRemotePath(view.remote.currentPath||'.'),roots=selectedArchiveRoots(entries);
  if(source==='host'){
    const base=normalizeRelativePath(view.left.currentPath||'.');
    view.remote.status.textContent='Compressed upload queued on TaskDeck daemon…';
    await createServerTransferJob(view,{
      kind:'host_archive_upload',
      host_paths:entries.map(entry=>joinPath(base,entry.name,false)),
      remote_dir:remoteDir,
      merge_policy:mergePolicy,
      auto_extract:canAutoExtractCompressedUpload(view)
    });
    return;
  }
  const sourceLabel=entries.length===1?String(entries[0]?.name||'Local selection'):(String(entries[0]?.name||'Local selection')+' (+'+(entries.length-1)+' selected)');
  enqueueTransferTasks(view,[{
    direction:'→',kind:'Compressed upload',source:sourceLabel,target:remoteDir,detail:'Waiting to compress',
    run:(item,signal)=>runLocalCompressedUpload(view,entries,remoteDir,roots,mergePolicy,item,signal)
  }]);
  view.remote.status.textContent='Compressed upload queued in Transfer Queue…';
}
async function compressedUploadDecision(view,entries){
  if(!selectionContainsFolder(entries))return 'normal';
  view.remote.status.textContent='Quick-scanning folder selection for upload optimization…';
  let scan;
  try{scan=view.left.source==='host'?await quickScanHostUploadSelection(view,entries):await quickScanLocalUploadSelection(view,entries);}
  catch(error){console.warn('Compressed upload quick scan failed; continuing with normal upload',error);return 'normal';}
  if(!scan?.many_files)return 'normal';
  const rootsExist=!compressedUploadRootsClear(view,entries);
  const compressionSupported=view.left.source==='host'||typeof CompressionStream==='function';
  const reason=compressionSupported?'':'this browser does not provide native gzip CompressionStream support.';
  return requestCompressedUploadDecision(view,entries,scan,{archiveAllowed:compressionSupported,rootsExist,reason});
}

async function streamLeftEntriesToRemote(view,entries){
  const selected=[...(entries||[])];if(!selected.length)throw new Error('Select one or more left items first');
  const source=view.left.source,sourceBase=normalizeRelativePath(view.left.currentPath||'.'),remoteBase=normalizeRemotePath(view.remote.currentPath||'.');
  let root=null;
  if(source!=='host'){
    root=view.left.localRoot;if(!root)throw new Error('Choose a local folder first');
    const granted=await ensureHandlePermission(root.handle);if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
  }

  const decision=await compressedUploadDecision(view,selected);
  if(decision==='cancel'){view.remote.status.textContent='Upload cancelled';return;}
  if(decision==='archive'||decision==='archive-overwrite'){
    const mergePolicy=decision==='archive-overwrite'?'overwrite':'fail';
    try{await uploadCompressedSelection(view,selected,mergePolicy);return;}
    catch(error){
      const message=String(error?.message||error||'Compressed upload failed');
      const fallback=confirm('Compressed upload could not be completed:\n\n'+message+'\n\nUpload the selection normally instead?');
      if(!fallback)throw error;
    }
  }

  if(source==='host'){
    const hostPaths=selected.map(entry=>joinPath(sourceBase,entry.name,false));
    view.remote.status.textContent='Background upload queued on TaskDeck daemon…';
    await createServerTransferJob(view,{kind:'host_upload',host_paths:hostPaths,remote_dir:remoteBase});
    return;
  }
  const scan={
    id:newFileTransferJobID(),kind:'local_upload_scan',profile_id:String(view.profile.id||''),root_id:String(root.id||''),
    source_base:sourceBase,remote_base:remoteBase,created_at:Date.now(),conflict_policy:effectiveDirectionConflictPolicy(view,'upload'),
    selected:selected.map(entry=>({
      source_path:joinPath(sourceBase,entry.name,false),
      target_path:joinPath(remoteBase,entry.name,true),
      directory:entryType(entry)==='directory'
    }))
  };
  upsertPersistentLocalScan(scan);
  view.remote.status.textContent='Scanning and uploading '+selected.length+' selected item(s)…';
  await runPersistentLocalUploadScan(view,scan);
}
function newLeftScanState(view){
  const source=view.left.source,base=normalizeRelativePath(view.left.currentPath||'.');
  return {source,base,localRoot:view.left.localRoot||null,hostDirectories:new Set(['.',base]),hostListings:new Map(),localHandles:new Map(),files:0,folders:0};
}
async function ensureHostScanDirectory(view,target,state){
  target=normalizeRelativePath(target);
  if(target==='.'||state.hostDirectories.has(target))return;
  const parent=parentPath(target,false);
  await ensureHostScanDirectory(view,parent,state);
  const name=pathLeaf(target,false);
  let entries=state.hostListings.get(parent);
  if(!entries){entries=await fetchHostDirectoryEntries(parent);state.hostListings.set(parent,entries);}
  const existing=entries.find(item=>item.name===name);
  if(existing){
    if(entryType(existing)!=='directory')throw new Error('Host path exists and is not a folder: '+target);
    state.hostDirectories.add(target);return;
  }
  try{await hostMutation(view,'mkdir',target);}
  catch(error){
    entries=await fetchHostDirectoryEntries(parent);state.hostListings.set(parent,entries);
    const retry=entries.find(item=>item.name===name);
    if(!retry||entryType(retry)!=='directory')throw error;
    state.hostDirectories.add(target);return;
  }
  entries.push({name,type:'directory',size:0,modified:''});
  state.hostDirectories.add(target);state.folders++;markLeftQueueDirty(view);
}
async function ensureLocalScanDirectory(target,state){
  target=normalizeRelativePath(target);
  if(target==='.')return state.localRoot.handle;
  if(state.localHandles.has(target))return state.localHandles.get(target);
  const parent=parentPath(target,false),name=pathLeaf(target,false);
  const parentHandle=parent==='.'?state.localRoot.handle:await ensureLocalScanDirectory(parent,state);
  const handle=await parentHandle.getDirectoryHandle(name,{create:true});
  state.localHandles.set(target,handle);state.folders++;markLeftQueueDirty(state.view);
  return handle;
}
async function ensureLeftScanDirectory(view,target,state){
  if(state.source==='host')return ensureHostScanDirectory(view,target,state);
  return ensureLocalScanDirectory(target,state);
}
async function writeRemotePathToLocalRoot(view,root,remotePath,leftPath,overwrite=false){
  const dirPath=parentPath(leftPath,false),name=pathLeaf(leftPath,false);
  let dir=root.handle;
  if(dirPath!=='.')for(const part of dirPath.split('/').filter(Boolean))dir=await dir.getDirectoryHandle(part,{create:true});
  let exists=false;
  try{await dir.getFileHandle(name);exists=true;}catch(error){if(error?.name!=='NotFoundError')throw error;}
  if(exists&&!overwrite)throw new Error('Local destination file appeared after conflict check: '+leftPath);
  const handle=await dir.getFileHandle(name,{create:true}),writable=await handle.createWritable();
  try{
    const ticket=await issueDownloadTicket(view,remotePath);
    const response=await fetch('/api/file-transfer/download?ticket='+encodeURIComponent(ticket),{cache:'no-store'});
    if(!response.ok){
      const message=(await response.text()).trim()||response.statusText;
      if(response.status===429&&response.headers.get('X-TaskDeck-Transfer-Pool-Full')==='1'){
        const error=new Error(message||'FTP/SFTP connection pool is full');error.transferPoolBusy=true;throw error;
      }
      throw new Error(message);
    }
    if(response.body&&typeof response.body.pipeTo==='function')await response.body.pipeTo(writable);
    else{await writable.write(await response.arrayBuffer());await writable.close();}
  }catch(error){try{await writable.abort();}catch{}throw error;}
}
async function writeRemotePathToHost(view,remotePath,leftPath){
  const payload={profile_id:view.profile.id,remote_path:remotePath,host_dir:parentPath(leftPath,false),overwrite:false};
  let response=await app.fetchWithLease('/api/file-transfer/remote-to-host',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),cache:'no-store'});
  if(response.status===409){
    const message=(await response.text()).trim();
    if(message.includes('already exists')&&confirm(leftPath+' already exists on Host. Overwrite it?')){
      payload.overwrite=true;response=await app.fetchWithLease('/api/file-transfer/remote-to-host',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),cache:'no-store'});
    }else throw new Error(message||('Skipped existing host file: '+leftPath));
  }
  if(!response.ok){
    const message=(await response.text()).trim()||response.statusText;
    if(response.status===429&&response.headers.get('X-TaskDeck-Transfer-Pool-Full')==='1'){
      const error=new Error(message||'FTP/SFTP connection pool is full');error.transferPoolBusy=true;throw error;
    }
    throw new Error(message);
  }
}
function enqueueRemoteDownloadFile(view,remotePath,leftPath,size,state,modified=''){
  state.files++;
  const source=state.source,localRoot=state.localRoot,jobID=String(state?.jobID||'');
  const policy=localJobConflictPolicy(jobID)!=='ask'?localJobConflictPolicy(jobID):normalizeConflictPolicy(state?.conflictPolicy);
  const spec=source==='local'?{
    kind:'local_download',profile_id:String(view.profile.id||''),root_id:String(localRoot?.id||''),
    remote_path:normalizeRemotePath(remotePath),left_path:normalizeRelativePath(leftPath),
    remote_size:Number(size)||0,remote_modified:String(modified||''),job_id:jobID
  }:null;
  enqueueTransferTasks(view,[{
    id:spec?newFileTransferJobID():undefined,direction:'←',kind:'Download',source:remotePath,target:leftPath,size:Number(size)||0,
    conflictJobID:jobID,conflictPolicy:policy,persistSpec:spec,
    run:spec?(item=>persistentLocalTransferRun(view,spec,item)):async()=>{
      await writeRemotePathToHost(view,remotePath,leftPath);markLeftQueueDirty(view);
    }
  }]);
}
async function scanRemoteDownloadEntry(view,remoteParent,entry,leftParent,state){
  assertTransferScanActive(view,state?.scanGeneration);
  const remotePath=joinPath(remoteParent,entry.name,true),leftPath=joinPath(leftParent,entry.name,false);
  if(entryType(entry)!=='directory'){
    enqueueRemoteDownloadFile(view,remotePath,leftPath,entry.size,state,entry.modified);return;
  }
  await ensureLeftScanDirectory(view,leftPath,state);
  const listing=await fetchRemoteDirectory(view,remotePath);assertTransferScanActive(view,state?.scanGeneration);
  for(const child of listing.entries){assertTransferScanActive(view,state?.scanGeneration);await scanRemoteDownloadEntry(view,remotePath,child,leftPath,state);}
}
async function streamRemoteEntriesToLeft(view,entries){
  const selected=[...(entries||[])];if(!selected.length)throw new Error('Select one or more remote items first');
  const remoteBase=normalizeRemotePath(view.remote.currentPath||'.');
  if(view.left.source==='host'){
    const remoteTargets=selected.map(entry=>({
      path:joinPath(remoteBase,entry.name,true),directory:entryType(entry)==='directory',
      size:Number(entry.size)||0,modified:String(entry.modified||'')
    }));
    view.left.status.textContent='Background download queued on TaskDeck daemon…';
    await createServerTransferJob(view,{kind:'host_download',remote_targets:remoteTargets,host_dir:normalizeRelativePath(view.left.currentPath||'.')});
    return;
  }
  const root=view.left.localRoot;if(!root)throw new Error('Choose a local folder first');
  const granted=await ensureHandlePermission(root.handle);if(!granted)throw new Error('Local folder permission is required. Click Grant first.');
  const leftBase=normalizeRelativePath(view.left.currentPath||'.');
  const scan={
    id:newFileTransferJobID(),kind:'local_download_scan',profile_id:String(view.profile.id||''),root_id:String(root.id||''),
    remote_base:remoteBase,left_base:leftBase,created_at:Date.now(),conflict_policy:effectiveDirectionConflictPolicy(view,'download'),
    selected:selected.map(entry=>({
      remote_path:joinPath(remoteBase,entry.name,true),directory:entryType(entry)==='directory',size:Number(entry.size)||0,modified:String(entry.modified||'')
    }))
  };
  upsertPersistentLocalScan(scan);
  view.left.status.textContent='Scanning and downloading '+selected.length+' selected item(s)…';
  await runPersistentLocalDownloadScan(view,scan);
}
async function transferLeftEntriesToRemote(view,entries){return streamLeftEntriesToRemote(view,entries);}
async function transferLeftToRemote(view){return transferLeftEntriesToRemote(view,selectedEntries(view.left));}
async function transferRemoteEntriesToLeft(view,entries){return streamRemoteEntriesToLeft(view,entries);}
async function transferRemoteToLeft(view){return transferRemoteEntriesToLeft(view,selectedEntries(view.remote));}

function deleteJobRuntime(jobID){
  let runtime=remoteDeleteRuntime.get(jobID);
  if(!runtime){runtime={scanning:false,scanFailed:false,queued:0,completed:0};remoteDeleteRuntime.set(jobID,runtime);}
  return runtime;
}
function markPersistentDeleteItemSuccess(view,jobID){
  const runtime=remoteDeleteRuntime.get(jobID);if(!runtime)return;
  runtime.completed++;maybeCompletePersistentDeleteJob(view,jobID);
}
function maybeCompletePersistentDeleteJob(view,jobID){
  const runtime=remoteDeleteRuntime.get(jobID);if(!runtime||runtime.scanning||runtime.scanFailed)return;
  if(runtime.completed<runtime.queued)return;
  removePersistentFileTransferJob(jobID);remoteDeleteRuntime.delete(jobID);activeRemoteDeleteJobs.delete(jobID);
  persistFileTransferSession();
}
async function remoteDeleteTargetEntry(view,path){
  path=normalizeRemotePath(path);
  if(path==='.'||path==='/')return null;
  const parent=parentPath(path,true),name=pathLeaf(path,true);
  const listing=await fetchRemoteDirectory(view,parent,{force:true});
  return listing.entries.find(entry=>String(entry.name||'')===name)||null;
}
async function deleteRemotePathIdempotent(view,path,directory){
  try{
    await mutateRemoteRequest(view,'delete',path,'',directory);
  }catch(error){
    try{
      const current=await remoteDeleteTargetEntry(view,path);
      if(!current)return;
    }catch{}
    throw error;
  }
}
function enqueueRemoteDeleteItem(view,path,directory,state,jobID=''){
  if(directory)state.folders++;else state.files++;
  if(jobID)deleteJobRuntime(jobID).queued++;
  enqueueTransferTasks(view,[{
    direction:'×',kind:'Delete',source:path,target:'',size:0,jobID,
    run:async()=>{
      await deleteRemotePathIdempotent(view,path,directory);
      if(directory)invalidateRemoteCache(view,path,true);
      markRemoteQueueDirty(view,parentPath(path,true));
    }
  }]);
}
async function scanRemoteDeleteEntry(view,remoteParent,entry,state,jobID=''){
  assertTransferScanActive(view,state?.scanGeneration);
  const remotePath=joinPath(remoteParent,entry.name,true);
  if(entryType(entry)!=='directory'){
    enqueueRemoteDeleteItem(view,remotePath,false,state,jobID);return;
  }
  const listing=await fetchRemoteDirectory(view,remotePath,{force:true});assertTransferScanActive(view,state?.scanGeneration);
  for(const child of listing.entries){assertTransferScanActive(view,state?.scanGeneration);await scanRemoteDeleteEntry(view,remotePath,child,state,jobID);}
  enqueueRemoteDeleteItem(view,remotePath,true,state,jobID);
}
async function scanPersistentRemoteDeleteTarget(view,target,state,jobID){
  const entry=await remoteDeleteTargetEntry(view,target.path);
  if(!entry)return;
  const actualDirectory=entryType(entry)==='directory';
  if(!actualDirectory){
    enqueueRemoteDeleteItem(view,target.path,false,state,jobID);return;
  }
  const listing=await fetchRemoteDirectory(view,target.path,{force:true});
  for(const child of listing.entries)await scanRemoteDeleteEntry(view,target.path,child,state,jobID);
  enqueueRemoteDeleteItem(view,target.path,true,state,jobID);
}
async function runPersistentRemoteDeleteJob(view,job){
  if(!job?.id||activeRemoteDeleteJobs.has(job.id))return;
  activeRemoteDeleteJobs.add(job.id);
  const runtime=deleteJobRuntime(job.id);runtime.scanning=true;runtime.scanFailed=false;runtime.queued=0;runtime.completed=0;
  const state={files:0,folders:0};
  view.remote.status.textContent='Resuming delete scan…';
  try{
    await runTransferScan(view,'Delete scan',async control=>{
      state.scanGeneration=control.generation;
      for(const target of job.targets){control.check();await scanPersistentRemoteDeleteTarget(view,target,state,job.id);}
    });
    view.remote.status.textContent='Delete scan complete · '+state.files+' file(s) · '+state.folders+' folder(s) queued';
  }catch(error){
    if(error?.transferScanStopped){
      runtime.scanFailed=false;removePersistentFileTransferJob(job.id);
      view.remote.status.textContent='Delete scan stopped';
    }else{
      runtime.scanFailed=true;
      view.remote.status.textContent='Delete scan interrupted · will resume after reload';
      console.warn('Persistent remote delete scan failed',error);
    }
  }finally{
    runtime.scanning=false;activeRemoteDeleteJobs.delete(job.id);maybeCompletePersistentDeleteJob(view,job.id);
  }
}
async function resumePersistentRemoteDeleteJobs(){
  for(const job of readPersistentFileTransferJobs()){
    const view=views.get(job.profile_id);
    if(view)runPersistentRemoteDeleteJob(view,job).catch(error=>console.warn('Cannot resume remote delete job',error));
  }
}
function canUseSSHRecursiveDelete(view){
  return String(view?.profile?.protocol||'').toLowerCase()==='sftp'&&Boolean(String(view?.profile?.ssh_profile_id||'').trim());
}
function requestRemoteDeleteDecision(view,entries){
  return new Promise(resolve=>{
    const selected=[...(entries||[])],dirs=selected.filter(entry=>entryType(entry)==='directory').length;
    const files=selected.length-dirs;
    const backdrop=document.createElement('div');backdrop.className='ft-upload-choice-backdrop';
    const box=document.createElement('div');box.className='ft-upload-choice';box.setAttribute('role','dialog');box.setAttribute('aria-modal','true');
    const title=document.createElement('h3');title.textContent='Choose remote delete method';
    const intro=document.createElement('p');
    intro.textContent='This SFTP connection is linked to SSH. You selected '+dirs+' folder(s)'+(files?' and '+files+' file(s)':'')+'. Choose how TaskDeck should delete them.';
    const note=document.createElement('div');note.className='ft-upload-choice-note';
    note.textContent=
      'FAST — SSH rm -rf\n'+
      '• Runs deletion directly on the remote server without scanning every file first.\n'+
      '• Usually much faster for folders containing thousands or tens of thousands of files.\n'+
      '• Risk: destructive and immediate; per-file progress/retry is bypassed, and remote shell permissions/filesystem semantics apply. SFTP can expose a virtual/chroot path that does not map to the same SSH shell path. TaskDeck quotes paths and refuses remote root/current-directory targets, but you must verify that the displayed SFTP path maps to the intended SSH filesystem location.\n\n'+
      'CONTROLLED — scan + delete each item\n'+
      '• Enumerates the tree and deletes files/subfolders through the transfer queue.\n'+
      '• Gives per-item progress, retry/error visibility and works without shell deletion.\n'+
      '• Slower for large trees because it requires many directory listings and remote operations. Folder deletion waits for all descendants to finish.';
    const paths=document.createElement('div');paths.className='ft-upload-choice-note';
    const preview=selected.slice(0,8).map(entry=>joinPath(view.remote.currentPath||'.',entry.name,true));
    paths.textContent='Selected path(s):\n'+preview.join('\n')+(selected.length>preview.length?'\n… +'+(selected.length-preview.length)+' more':'');
    const actions=document.createElement('div');actions.className='ft-upload-choice-actions';
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
    const scan=document.createElement('button');scan.type='button';scan.textContent='Scan + delete items';
    const ssh=document.createElement('button');ssh.type='button';ssh.textContent='Fast delete via SSH (rm -rf)';ssh.classList.add('ft-sync-danger');
    const finish=value=>{backdrop.remove();resolve(value);};
    cancel.onclick=()=>finish('cancel');scan.onclick=()=>finish('scan');ssh.onclick=()=>finish('ssh');
    actions.append(cancel,scan,ssh);box.append(title,intro,note,paths,actions);backdrop.append(box);document.body.append(backdrop);
    backdrop.onpointerdown=event=>{if(event.target===backdrop)finish('cancel');};
    setTimeout(()=>scan.focus(),0);
  });
}

async function streamRemoteDeleteEntries(view,entries){
  const selected=[...(entries||[])];if(!selected.length)return;
  const dirs=selected.filter(entry=>entryType(entry)==='directory').length;
  let deleteMode='scan';
  if(dirs>0&&canUseSSHRecursiveDelete(view)){
    const decision=await requestRemoteDeleteDecision(view,selected);
    if(decision==='cancel')return;
    deleteMode=decision;
  }else{
    const message=selected.length===1
      ?'Delete '+selected[0].name+'?'+(dirs?'\n\nThe folder will be scanned and deleted item-by-item.':'')
      :'Delete '+selected.length+' selected item(s)?'+(dirs?'\n\nSelected folders will be scanned and deleted item-by-item.':'');
    if(!confirm(message))return;
  }
  const base=normalizeRemotePath(view.remote.currentPath||'.');
  const targets=selected.map(entry=>({path:joinPath(base,entry.name,true),directory:entryType(entry)==='directory'}));
  if(deleteMode==='ssh'){
    view.remote.status.textContent='Fast recursive delete queued through linked SSH…';
    await createServerTransferJob(view,{kind:'remote_delete',delete_mode:'ssh_recursive',remote_targets:targets});
    view.remote.status.textContent='Fast SSH delete queued · '+targets.length+' selected item(s)';
    return;
  }
  view.remote.status.textContent='Background delete scan queued on TaskDeck daemon…';
  // Large file-only selections used to exceed the JSON request limit. Split
  // them into bounded jobs so tens of thousands of selected files remain
  // actionable without depending on browser/server body size. Keep recursive
  // directory deletion as one approval-scoped job.
  if(dirs===0&&targets.length>1000){
    for(let offset=0;offset<targets.length;offset+=1000){
      view.remote.status.textContent='Queueing delete '+Math.min(offset+1000,targets.length)+'/'+targets.length+'…';
      await createServerTransferJob(view,{kind:'remote_delete',delete_mode:'scan',remote_targets:targets.slice(offset,offset+1000)});
    }
    view.remote.status.textContent='Background delete queued · '+targets.length+' file(s)';
    return;
  }
  await createServerTransferJob(view,{kind:'remote_delete',delete_mode:'scan',remote_targets:targets});
}
async function deleteRemoteEntries(view,entries){return streamRemoteDeleteEntries(view,entries);}

async function copyText(value){
  const text=String(value??'');
  if(navigator.clipboard?.writeText){await navigator.clipboard.writeText(text);return;}
  const area=document.createElement('textarea');area.value=text;area.style.position='fixed';area.style.opacity='0';document.body.append(area);area.select();
  try{document.execCommand('copy');}finally{area.remove();}
}

function updateTransferButtons(view){
  view.toRemote.disabled=selectedEntries(view.left).length===0;
  view.toLeft.disabled=selectedEntries(view.remote).length===0;
  syncRemoteNavigationAvailability(view);
}

function leftDoubleClick(view,entry){
  if(entryType(entry)==='directory')loadLeftDirectory(view,joinPath(view.left.currentPath,entry.name,false)).catch(app.showError);
  else transferLeftEntriesToRemote(view,[entry]).catch(app.showError);
}
function remoteEditorWritable(){
  return app.hasPermission('transfer.upload')&&app.hasPermission('files.upload')&&app.hasPermission('files.write');
}
function remoteEditorVirtualPath(view,remotePath){
  const protocol=String(view?.profile?.protocol||'remote').toLowerCase();
  const profileID=encodeURIComponent(String(view?.profile?.id||'profile'));
  return protocol+'://'+profileID+'/'+String(remotePath||'').replace(/^\/+/, '');
}
function remoteEditorDocument(view,remotePath,data){
  const raw=String(data?.content??''),bom=raw.charCodeAt(0)===0xfeff,content=bom?raw.slice(1):raw;
  const protocol=String(view?.profile?.protocol||'remote').toUpperCase();
  const profileName=String(view?.profile?.name||view?.profile?.id||'Remote');
  return {
    path:remoteEditorVirtualPath(view,remotePath),
    remote_transfer_profile_id:String(view?.profile?.id||''),
    remote_path:String(remotePath||''),
    remote_profile_name:profileName,
    remote_protocol:protocol,
    content,
    sha256:String(data?.sha256||''),
    size:new TextEncoder().encode(raw).length,
    line_ending:/\r\n/.test(content)?'crlf':'lf',
    bom,
    encoding:'utf-8',
    read_only:!remoteEditorWritable(),
    large_file:false,
    warning:'Remote '+protocol+' · '+profileName+' · Save uploads directly to '+String(remotePath||'')
  };
}
async function editRemoteFile(view,entry){
  if(entryType(entry)!=='file')throw new Error('Remote editor can open files only');
  const editor=globalThis.TaskMenuEditor;
  if(!editor?.openDocument)throw new Error('Editor is unavailable');
  const remotePath=joinPath(view.remote.currentPath,entry.name,true);
  view.remote.status.textContent='Opening remote file in editor…';
  const params=new URLSearchParams({profile_id:String(view.profile.id),path:remotePath});
  const data=await app.jsonFetch('/api/file-transfer/text?'+params.toString(),{cache:'no-store'});
  const opened=await editor.openDocument(remoteEditorDocument(view,remotePath,data));
  view.remote.status.textContent='Editing '+entry.name+' · Save/Ctrl+S uploads to remote';
  return opened;
}

function remoteDoubleClick(view,entry){
  if(entryType(entry)==='directory'&&remoteConnectionPoolFull(view)){syncRemoteNavigationAvailability(view);return;}
  if(entryType(entry)==='directory')loadRemoteDirectory(view,joinPath(view.remote.currentPath,entry.name,true)).catch(app.showError);
  else transferRemoteEntriesToLeft(view,[entry]).catch(app.showError);
}
function compareLeftFileSource(view,entry){
  const compare=globalThis.TaskMenuFileCompare;if(!compare)throw new Error('File Compare unavailable');
  const leftPath=joinPath(view.left.currentPath,entry.name,false);
  if(view.left.source==='host'){
    const writable=!app.sharedMode||Boolean(app.hasPermission?.('files.write')||app.hasPermission?.('project.admin'));
    return compare.projectSource(leftPath,{label:'Host · '+leftPath,writable});
  }
  if(entry.handle)return compare.browserFileHandleSource(entry.handle,{label:'Local · '+leftPath,path:leftPath});
  throw new Error('Selected local browser file handle is unavailable');
}
function compareRemoteFileSource(view,entry){
  const compare=globalThis.TaskMenuFileCompare;if(!compare?.remoteSource)throw new Error('File Compare unavailable');
  const remotePath=joinPath(view.remote.currentPath,entry.name,true);
  const protocol=String(view.profile?.protocol||'remote').toUpperCase();
  return compare.remoteSource(view.profile.id,remotePath,{label:protocol+' · '+remotePath,writable:remoteEditorWritable()});
}
function appendCompareSelectionActions(items,source){
  const compare=globalThis.TaskMenuFileCompare;if(!compare?.selectForCompare)return;
  items.push({label:'Select for compare',action:()=>compare.selectForCompare(source)});
  if(compare.selection)items.push({
    label:'Compare with selected file',
    disabled:!compare.canCompareWithSelected?.(source),
    action:()=>compare.compareWithSelected(source,{title:'Selected files'})
  });
}
async function compareTransferSources(sources,title='Selected transfer files'){
  const compare=globalThis.TaskMenuFileCompare;if(!compare?.openSources)throw new Error('File Compare unavailable');
  return compare.openSources(sources,{title});
}
async function compareLeftRemoteFiles(view,leftEntry,remoteEntry){
  if(entryType(leftEntry)!=='file'||entryType(remoteEntry)!=='file')throw new Error('File Compare requires one file on each side');
  return compareTransferSources([compareLeftFileSource(view,leftEntry),compareRemoteFileSource(view,remoteEntry)],'Left ↔ Remote');
}
function transferArchiveKind(name){
  const lower=String(name||'').toLowerCase();
  return lower.endsWith('.zip')||lower.endsWith('.tar.gz')||lower.endsWith('.tgz');
}
function transferArchiveStem(name){
  return String(name||'archive').replace(/\.tar\.gz$|\.tgz$|\.zip$/i,'')||'archive';
}
async function uploadAndExtractRemoteArchive(view,entry){
  if(view.left.source!=='host')throw new Error('Remote archive extraction requires a Host project archive.');
  if(String(view.profile?.protocol||'').toLowerCase()!=='sftp')throw new Error('Remote archive extraction requires an SFTP profile linked to SSH.');
  const hostPath=joinPath(view.left.currentPath,entry.name,false);
  const remoteBase=normalizeRemotePath(view.remote.currentPath||'.');
  const defaultArchive=joinPath(remoteBase,entry.name,true);
  const remoteArchive=prompt('Upload archive to remote path:',defaultArchive);
  if(remoteArchive===null)return;
  const defaultDestination=joinPath(remoteBase,transferArchiveStem(entry.name),true);
  const remoteDestination=prompt('Extract into NEW remote directory:',defaultDestination);
  if(remoteDestination===null)return;
  if(!confirm('Upload and extract archive on remote host?\n\nArchive: '+remoteArchive+'\nDestination: '+remoteDestination+'\n\nThe destination must not already exist.'))return;
  const response=await app.fetchWithLease('/api/file-transfer/archive-upload-extract',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({
      profile_id:view.profile.id,
      host_path:hostPath,
      remote_archive:String(remoteArchive).trim(),
      remote_destination:String(remoteDestination).trim()
    })
  });
  if(!response.ok)throw new Error(await response.text()||('HTTP '+response.status));
  const data=await response.json();
  view.remote.status.textContent='Archive extracted to '+String(data?.remote_destination||remoteDestination);
  await loadRemoteDirectory(view,view.remote.currentPath,{force:true});
}
function contextTitle(panel,entry){
  const selected=selectedEntries(panel);
  return selected.length>1?selected.length+' items selected':String(entry?.name||'File actions');
}
function selectedPathText(panel,remote=false){
  return selectedEntries(panel).map(entry=>joinPath(panel.currentPath,entry.name,remote)).join('\n');
}
function commonSelectionMenu(panel,remote,onRefresh){
  const selected=selectedEntries(panel),items=[];
  if(selected.length===1)items.push({label:'Copy name',action:()=>copyText(selected[0].name)});
  if(selected.length)items.push({label:selected.length>1?'Copy selected paths':'Copy path',action:()=>copyText(selectedPathText(panel,remote))});
  items.push({separator:true},{label:'Select all',action:()=>selectAllEntries(panel)});
  if(selected.length>1)items.push({label:'Clear selection',action:()=>clearSelection(panel)});
  items.push({label:'Refresh',action:onRefresh});
  return items;
}
function leftContext(view,entry,event){
  const panel=view.left,selected=selectedEntries(panel),items=[];
  const compareFiles=selected.filter(item=>entryType(item)==='file');
  if(selected.length===2&&compareFiles.length===2){
    items.push({label:'Compare selected files',action:()=>compareTransferSources(compareFiles.map(item=>compareLeftFileSource(view,item)),'Selected left files')});
  }
  if(selected.length===1&&entryType(selected[0])==='file')appendCompareSelectionActions(items,compareLeftFileSource(view,selected[0]));
  if(items.length)items.push({separator:true});
  if(panel.source==='host'&&selected.length===1){
    const projectPath=joinPath(panel.currentPath,selected[0].name,false);
    const projectType=entryType(selected[0])==='directory'?'dir':'file';
    const shared=globalThis.TaskMenuProjectFileActions?.standardActions?.(projectPath,projectType)||[];
    for(const action of shared){
      if(action.separator)items.push({separator:true});
      else if(action.label!=='Copy path')items.push({label:action.label,action:action.run});
    }
    if(shared.length)items.push({separator:true});
  }
  if(selected.length===1&&entryType(selected[0])==='directory'){
    items.push({label:'Open folder',action:()=>loadLeftDirectory(view,joinPath(panel.currentPath,selected[0].name,false))});
  }
  const selectedRemoteForCompare=selectedEntries(view.remote);
  if(selected.length===1&&entryType(selected[0])==='file'&&selectedRemoteForCompare.length===1&&entryType(selectedRemoteForCompare[0])==='file'){
    items.push({label:'Compare with selected remote file',action:()=>compareLeftRemoteFiles(view,selected[0],selectedRemoteForCompare[0])});
  }
  if(selected.length){
    items.push({label:selected.length>1?'Upload selected items to remote FTP/SFTP →':'Upload to remote FTP/SFTP →',action:()=>transferLeftEntriesToRemote(view,selected)});
  }
  if(panel.source==='host'&&selected.length===1&&entryType(selected[0])==='file'){
    items.push({label:'Download host file to browser',action:()=>{downloadFrame().src='/api/files/download?path='+encodeURIComponent(joinPath(panel.currentPath,selected[0].name,false));}});
    if(canAutoExtractCompressedUpload(view)&&transferArchiveKind(selected[0].name)){
      items.push({label:'Upload + extract archive on remote…',action:()=>uploadAndExtractRemoteArchive(view,selected[0])});
    }
  }
  if(items.length)items.push({separator:true});
  if(selected.length===1)items.push({label:'Rename',action:()=>renameLeftEntry(view,selected[0])});
  if(selected.length)items.push({label:'Delete '+(selected.length>1?selected.length+' selected items':'item'),danger:true,action:()=>deleteLeftEntries(view,selected)});
  items.push({label:'New folder',action:()=>newLeftFolder(view)});
  items.push({separator:true},...commonSelectionMenu(panel,false,()=>loadLeftDirectory(view,panel.currentPath)));
  showContextMenu(items,event.clientX,event.clientY,contextTitle(panel,entry));
}
function remoteContext(view,entry,event){
  const panel=view.remote,selected=selectedEntries(panel),items=[];
  const compareFiles=selected.filter(item=>entryType(item)==='file');
  if(selected.length===2&&compareFiles.length===2){
    items.push({label:'Compare selected files',action:()=>compareTransferSources(compareFiles.map(item=>compareRemoteFileSource(view,item)),'Selected remote files')});
  }
  if(selected.length===1&&entryType(selected[0])==='file')appendCompareSelectionActions(items,compareRemoteFileSource(view,selected[0]));
  if(items.length)items.push({separator:true});
  if(selected.length===1&&entryType(selected[0])==='directory'){
    items.push({label:'Open folder',disabled:remoteConnectionPoolFull(view),action:()=>loadRemoteDirectory(view,joinPath(panel.currentPath,selected[0].name,true))});
  }
  const selectedLeftForCompare=selectedEntries(view.left);
  if(selected.length===1&&entryType(selected[0])==='file'&&selectedLeftForCompare.length===1&&entryType(selectedLeftForCompare[0])==='file'){
    items.push({label:'Compare with selected left file',action:()=>compareLeftRemoteFiles(view,selectedLeftForCompare[0],selected[0])});
  }
  if(selected.length)items.push({label:selected.length>1?'Download selected items to left ←':'Download to left ←',action:()=>transferRemoteEntriesToLeft(view,selected)});
  if(selected.length===1&&entryType(selected[0])==='file'){
    items.push({label:'Edit remote file',disabled:remoteConnectionPoolFull(view),action:()=>editRemoteFile(view,selected[0])});
    items.push({label:'Download to browser',action:()=>downloadFileToBrowser(view,joinPath(panel.currentPath,selected[0].name,true))});
  }
  if(items.length)items.push({separator:true});
  if(selected.length===1)items.push({label:'Rename',action:()=>renameRemoteEntry(view,selected[0])});
  if(selected.length)items.push({label:'Delete '+(selected.length>1?selected.length+' selected items':'item'),danger:true,action:()=>deleteRemoteEntries(view,selected)});
  items.push({label:'New remote folder',action:()=>{
    const name=prompt('New remote folder name:','');if(name===null)return;const clean=String(name).trim();if(!clean)return;
    if(clean.includes('/')||clean.includes('\\'))throw new Error('Folder name must not contain path separators');
    return mutateRemote(view,'mkdir',joinPath(panel.currentPath,clean,true));
  }});
  items.push({separator:true},...commonSelectionMenu(panel,true,()=>loadRemoteDirectory(view,panel.currentPath,{force:true})));
  showContextMenu(items,event.clientX,event.clientY,contextTitle(panel,entry));
}

window.addEventListener('taskmenu:file-transfer-remote-edited',event=>{
  const profileID=String(event.detail?.profile_id||''),remotePath=normalizeRemotePath(event.detail?.path||'.');
  const view=views.get(profileID);if(!view||view.closed)return;
  invalidateRemoteCache(view,parentPath(remotePath,true));
  if(normalizeRemotePath(view.remote.currentPath||'.')===parentPath(remotePath,true)){
    loadRemoteDirectory(view,view.remote.currentPath,{force:true}).catch(()=>{});
  }
});

function installDivider(view,divider,sites){
  const storageKey='taskdeck:file-transfer:split:'+workspaceKey()+':'+view.profile.id;
  let ratio=Math.min(.75,Math.max(.25,Number(safeStorageGet(storageKey,'.5'))||.5)),dragging=false;
  const apply=()=>{
    if(matchMedia('(max-width:850px)').matches){sites.style.gridTemplateColumns='1fr';sites.style.gridTemplateRows=`minmax(220px,${ratio}fr) 8px minmax(220px,${1-ratio}fr)`;}
    else{sites.style.gridTemplateRows='';sites.style.gridTemplateColumns=`minmax(260px,${ratio}fr) 8px minmax(260px,${1-ratio}fr)`;}
  };
  apply();window.addEventListener('resize',apply);
  divider.onpointerdown=event=>{if(event.button!==0)return;dragging=true;divider.classList.add('dragging');document.body.classList.add('ft-resizing');divider.setPointerCapture(event.pointerId);event.preventDefault();};
  divider.onpointermove=event=>{
    if(!dragging)return;const rect=sites.getBoundingClientRect();
    ratio=matchMedia('(max-width:850px)').matches?(event.clientY-rect.top)/rect.height:(event.clientX-rect.left)/rect.width;
    ratio=Math.min(.75,Math.max(.25,ratio));apply();event.preventDefault();
  };
  const finish=event=>{if(!dragging)return;dragging=false;divider.classList.remove('dragging');document.body.classList.remove('ft-resizing');safeStorageSet(storageKey,String(ratio));try{divider.releasePointerCapture(event.pointerId);}catch{}};
  divider.onpointerup=finish;divider.onpointercancel=finish;divider.ondblclick=()=>{ratio=.5;apply();safeStorageSet(storageKey,'.5');};
}

function installQueueResizer(view){
  const queue=view.transferQueue,resizer=queue?.resizer,root=queue?.root;if(!resizer||!root)return;
  const storageKey='taskdeck:file-transfer:queue-height:'+workspaceKey()+':'+view.profile.id;
  const defaultHeight=170,minHeight=90;
  let height=Number(safeStorageGet(storageKey,String(defaultHeight)))||defaultHeight,dragging=false,startY=0,startHeight=height;
  const maxHeight=()=>{
    const paneHeight=view.pane.getBoundingClientRect().height||window.innerHeight||600;
    return Math.max(minHeight,Math.floor(Math.min(paneHeight*.65,paneHeight-38-160)));
  };
  const apply=()=>{
    height=Math.round(Math.min(maxHeight(),Math.max(minHeight,height)));
    root.style.height=height+'px';
    root.style.flex='0 0 '+height+'px';
  };
  apply();
  resizer.onpointerdown=event=>{
    if(event.button!==0)return;
    dragging=true;startY=event.clientY;startHeight=root.getBoundingClientRect().height||height;
    resizer.classList.add('dragging');document.body.classList.add('ft-queue-resizing');
    resizer.setPointerCapture(event.pointerId);event.preventDefault();
  };
  resizer.onpointermove=event=>{
    if(!dragging)return;
    height=startHeight-(event.clientY-startY);apply();event.preventDefault();
  };
  const finish=event=>{
    if(!dragging)return;
    dragging=false;resizer.classList.remove('dragging');document.body.classList.remove('ft-queue-resizing');
    safeStorageSet(storageKey,String(height));
    try{resizer.releasePointerCapture(event.pointerId);}catch{}
  };
  resizer.onpointerup=finish;resizer.onpointercancel=finish;
  resizer.ondblclick=()=>{height=defaultHeight;apply();safeStorageSet(storageKey,String(height));};
  window.addEventListener('resize',apply);
}

function createSiteShell(title){
  const site=document.createElement('div');site.className='ft-site';
  const head=document.createElement('div');head.className='ft-site-head';
  const label=document.createElement('span');label.className='ft-site-title';label.textContent=title;head.append(label);
  const status=document.createElement('div');status.className='ft-status';
  return {site,head,label,status,entries:[],selected:null,selectedKeys:new Set(),selectionAnchor:'',visibleEntries:[],sort:{key:'type',direction:'asc'},currentPath:'.',emptyText:'Directory is empty'};
}

function attachView(profile,{activate=true,session=null}={}){
  const id=String(profile.id||'');if(views.has(id)){if(activate)activateView(id,{force:true});return views.get(id);}
  const tabs=document.querySelector('#tabs'),panes=document.querySelector('#panes');if(!tabs||!panes)throw new Error('Workspace tabs are unavailable');

  const tab=document.createElement('button');tab.type='button';tab.className='ft-tab';tab.dataset.id='file-transfer:'+id;
  const tabLabel=document.createElement('span');tabLabel.textContent=String(profile.protocol||'file').toUpperCase()+' · '+(profile.name||'Files');
  const close=document.createElement('span');close.className='close';close.textContent='×';close.onclick=event=>{event.stopPropagation();closeView(id);};
  tab.append(tabLabel,close);tab.onclick=()=>activateView(id,{force:true});tabs.append(tab);

  const pane=document.createElement('div');pane.className='ft-pane hidden';pane.dataset.id='file-transfer:'+id;
  const head=document.createElement('div');head.className='ft-pane-head';
  const title=document.createElement('div');title.className='ft-pane-title';title.textContent=profile.name||'Remote files';
  const meta=document.createElement('div');meta.className='ft-pane-meta';meta.textContent=String(profile.protocol||'').toUpperCase()+' · dual-pane transfer';
  const spacer=document.createElement('div');spacer.className='ft-pane-spacer';
  const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='Close';closeButton.onclick=()=>closeView(id);head.append(title,meta,spacer,closeButton);

  const sites=document.createElement('div');sites.className='ft-sites';
  const left=createSiteShell('Left');
  const remote=createSiteShell('Remote · '+String(profile.protocol||'').toUpperCase());

  const view={profile,tab,pane,left,remote,remoteCache:new Map(),conflictSessionPolicies:{upload:'ask',download:'ask'}};

  const source=document.createElement('select');source.className='ft-source-select';
  for(const [value,label] of [['host','Host'],['local','Local browser']]){const option=document.createElement('option');option.value=value;option.textContent=label;source.append(option);}
  source.value=safeStorageGet('taskdeck:file-transfer:left-source:'+workspaceKey()+':'+profile.id,'host');
  const rootSelect=document.createElement('select');rootSelect.className='ft-root-select';rootSelect.hidden=true;
  const grantLocal=document.createElement('button');grantLocal.type='button';grantLocal.textContent='Grant';grantLocal.hidden=true;grantLocal.title='Grant read/write access to the selected local folder';
  const chooseLocal=document.createElement('button');chooseLocal.type='button';chooseLocal.textContent='Choose…';chooseLocal.hidden=true;
  left.head.append(source,rootSelect,grantLocal,chooseLocal);
  left.source=source.value;left.sourceSelect=source;left.rootSelect=rootSelect;left.grantLocal=grantLocal;left.chooseLocal=chooseLocal;left.memoryScope=()=>currentLeftScope(view);
  left.onSelection=()=>updateTransferButtons(view);

  const remoteIdentity=document.createElement('span');remoteIdentity.className='ft-local-note';remoteIdentity.textContent=profile.name||profile.id;
  remote.head.append(remoteIdentity);
  remote.memoryScope=()=> 'remote:'+profile.id;remote.onSelection=()=>updateTransferButtons(view);

  const leftPathBar=pathBar(left,{
    onLoad:path=>loadLeftDirectory(view,path),
    onRefresh:()=>loadLeftDirectory(view,left.currentPath),
    onUp:()=>loadLeftDirectory(view,parentPath(left.currentPath,false))
  });
  const remotePathBar=pathBar(remote,{
    remote:true,onLoad:path=>loadRemoteDirectory(view,path),
    onRefresh:()=>loadRemoteDirectory(view,remote.currentPath,{force:true}),
    onUp:()=>loadRemoteDirectory(view,parentPath(remote.currentPath,true))
  });

  const remoteMkdir=document.createElement('button');remoteMkdir.type='button';remoteMkdir.textContent='New Folder';
  remoteMkdir.onclick=()=>{
    const name=prompt('New remote folder name:','');if(name===null)return;const clean=String(name).trim();if(!clean)return;
    if(clean.includes('/')||clean.includes('\\')){app.showError(new Error('Folder name must not contain path separators'));return;}
    mutateRemote(view,'mkdir',joinPath(remote.currentPath,clean,true)).catch(app.showError);
  };
  remote.head.append(remoteMkdir);

  const leftTable=createSortableTable(left,entry=>leftDoubleClick(view,entry),(entry,event)=>leftContext(view,entry,event));
  const remoteTable=createSortableTable(remote,entry=>remoteDoubleClick(view,entry),(entry,event)=>remoteContext(view,entry,event));
  left.site.append(left.head,leftPathBar,left.status,leftTable);remote.site.append(remote.head,remotePathBar,remote.status,remoteTable);

  const divider=document.createElement('div');divider.className='ft-divider';divider.title='Drag to resize · double-click to reset';
  const tools=document.createElement('div');tools.className='ft-transfer-tools';
  const toRemote=document.createElement('button');toRemote.type='button';toRemote.textContent='→';toRemote.title='Upload selected left item(s) to remote FTP/SFTP';toRemote.disabled=true;
  const compare=document.createElement('button');compare.type='button';compare.textContent='⇄';compare.title='Folder Sync / Mirror dry run';
  const toLeft=document.createElement('button');toLeft.type='button';toLeft.textContent='←';toLeft.title='Download selected remote item(s) to left';toLeft.disabled=true;
  tools.append(toRemote,compare,toLeft);divider.append(tools);view.toRemote=toRemote;view.toLeft=toLeft;view.compare=compare;
  toRemote.onclick=()=>transferLeftToRemote(view).catch(app.showError);toLeft.onclick=()=>transferRemoteToLeft(view).catch(app.showError);
  compare.onclick=()=>configureAndCompareFolders(view).catch(app.showError);

  sites.append(left.site,divider,remote.site);
  const transferQueue=createTransferQueue(view);
  if(session?.queue_paused)view.transferQueue.paused=true;
  pane.append(head,sites,transferQueue);panes.append(pane);views.set(id,view);installDivider(view,divider,sites);installQueueResizer(view);startServerTransferQueuePolling(view);restorePersistentLocalTransferQueue(view);

  source.onchange=()=>switchLeftSource(view,source.value).catch(app.showError);
  rootSelect.onchange=async()=>{
    const record=await getLocalRoot(rootSelect.value);left.localRoot=record;left.localPermission=false;
    if(record){
      safeStorageSet('taskdeck:file-transfer:last-local-root:'+workspaceKey(),record.id);
      left.localPermission=await ensureHandlePermission(record.handle);
      grantLocal.hidden=left.localPermission;left.currentPath='.';left.pathInput.value='.';left.refreshPathMemory?.();
      await loadLocalDirectory(view,'.');resumePersistentLocalScans();
    }
  };
  grantLocal.onclick=()=>{
    if(!left.localRoot){app.showError(new Error('Choose a local folder first'));return;}
    requestHandlePermission(left.localRoot.handle).then(granted=>{
      if(!granted)throw new Error('Local folder permission was not granted');
      left.localPermission=true;grantLocal.hidden=true;return loadLocalDirectory(view,'.').then(()=>resumePersistentLocalScans());
    }).catch(app.showError);
  };
  chooseLocal.onclick=()=>chooseLocalRoot(view).then(()=>{grantLocal.hidden=true;return loadLocalDirectory(view,'.');}).catch(app.showError);

  const leftStart=normalizeRelativePath(session?.left_path||'.');
  const remoteStart=normalizeRemotePath(session?.remote_path||profile.initial_path||'.');
  remote.currentPath=remoteStart;remote.pathInput.value=remoteStart;remote.refreshPathMemory();
  switchLeftSource(view,source.value,leftStart).catch(app.showError);
  loadRemoteDirectory(view,remoteStart).catch(app.showError);
  if(activate)activateView(id,{force:true});
  else persistFileTransferSession();
  return view;
}

async function restoreFileTransferSession(){
  if(sessionRestoreStarted||!fileTransferWorkspaceID())return;
  const saved=readFileTransferSession(),legacyJobs=readPersistentFileTransferJobs();
  const localItems=readPersistentLocalTransferItems(),localScans=readPersistentLocalScans();
  sessionRestoreStarted=true;restoringSession=true;
  try{
    if(saved?.open?.length||legacyJobs.length||localItems.length||localScans.length){
      await refreshProfiles();
      const restored=new Set();
      for(const item of saved?.open||[]){
        const profile=profilesByID.get(item.profile_id);
        if(profile){attachView(profile,{activate:false,session:item});restored.add(item.profile_id);}
      }
      const backgroundProfiles=new Set([
        ...legacyJobs.map(item=>String(item.profile_id||'')),
        ...localItems.map(item=>String(item.profile_id||'')),
        ...localScans.map(item=>String(item.profile_id||''))
      ]);
      for(const profileID of backgroundProfiles){
        if(!profileID||restored.has(profileID))continue;
        const profile=profilesByID.get(profileID);
        if(profile){attachView(profile,{activate:false,session:null});restored.add(profileID);}
      }
    }
  }catch(error){
    console.warn('Cannot restore file-transfer session',error);
  }finally{
    restoringSession=false;sessionPersistenceReady=true;
  }
  if(saved?.active_profile_id&&views.has(saved.active_profile_id))activateView(saved.active_profile_id);
  else persistFileTransferSession();
  await migrateLegacyRemoteDeleteJobs();
  for(const view of views.values())syncServerTransferQueue(view).catch(()=>{});
  resumePersistentLocalScans();
}
async function scheduleFileTransferSessionRestore(){
  if(!fileTransferWorkspaceID()||sessionRestoreStarted)return;
  try{await globalThis.TaskMenuTerminalRestore?.ready;}catch{}
  await restoreFileTransferSession();
}

async function openProfile(profileOrID){
  const id=typeof profileOrID==='string'?profileOrID:profileOrID?.id;if(!id)throw new Error('File-transfer profile is required');
  await scheduleFileTransferSessionRestore();
  await refreshProfiles();const profile=profilesByID.get(String(id))||(typeof profileOrID==='object'?profileOrID:null);
  if(!profile)throw new Error('File-transfer profile not found');return attachView(profile);
}
async function testProfile(profileOrID){
  const id=typeof profileOrID==='string'?profileOrID:profileOrID?.id;if(!id)throw new Error('File-transfer profile is required');
  return app.jsonFetch('/api/file-transfer/test',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:id})});
}
async function testDraft(profileID,profile){
  const body={profile};if(profileID)body.profile_id=profileID;
  return app.jsonFetch('/api/file-transfer/test',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
}

window.addEventListener('taskmenu:view-activated',event=>{
  const detail=event.detail||{},external=detail.kind==='external'&&String(detail.id||'').startsWith('file-transfer:');
  const activeID=external?String(detail.id).slice('file-transfer:'.length):'';
  activeViewID=activeID&&views.has(activeID)?activeID:'';
  for(const [id,view] of views){const active=id===activeViewID;view.tab.classList.toggle('active',active);view.pane.classList.toggle('hidden',!active);}
  persistFileTransferSession();
});
window.addEventListener('taskmenu:tasks',scheduleFileTransferSessionRestore);
setTimeout(scheduleFileTransferSessionRestore,0);

async function fileTransferOperationSnapshot(){
  await refreshProfiles();
  const operations=[];
  for(const profile of profilesByID.values()){
    const profileID=String(profile?.id||'').trim();if(!profileID)continue;
    let snapshot;
    try{snapshot=await app.jsonFetch('/api/file-transfer/jobs?profile_id='+encodeURIComponent(profileID),{cache:'no-store'});}
    catch(error){continue;}
    const items=Array.isArray(snapshot?.items)?snapshot.items:[];
    for(const job of Array.isArray(snapshot?.jobs)?snapshot.jobs:[]){
      const jobItems=items.filter(item=>String(item?.job_id||'')===String(job?.id||''));
      const failed=jobItems.find(item=>String(item?.status||'')==='failed');
      const rawStatus=String(job?.status||'queued').toLowerCase();
      const status=rawStatus==='scan_queued'?'queued':rawStatus;
      operations.push({
        source:'transfer',id:String(job?.id||''),profile_id:profileID,
        title:(String(profile?.protocol||'SFTP').toUpperCase()+' · '+String(profile?.name||profile?.host||profileID)),
        detail:String(job?.kind||'File transfer'),status,
        error:String(job?.error||failed?.error||''),
        created_at:job?.created_at||'',updated_at:job?.updated_at||'',
        item_ids:jobItems.map(item=>String(item?.id||'')).filter(Boolean),
        can_cancel:['scanning','queued','running','conflict'].includes(status),
        can_retry:status==='failed',
        can_open:true
      });
    }
  }
  return operations;
}
async function fileTransferOperationControl(operation,action){
  const profileID=String(operation?.profile_id||'').trim();
  if(!profileID)throw new Error('File-transfer profile is unavailable');
  if(action==='open')return openProfile(profileID);
  const body={profile_id:profileID,action};
  if(action==='cancel'){
    body.action='remove_selected';
    body.item_ids=Array.isArray(operation?.item_ids)?operation.item_ids:[];
  }else if(action==='retry'){
    body.action='retry_failed';
  }else if(action==='clear'){
    body.action='clear_done';
  }else throw new Error('Unsupported file-transfer operation action');
  return app.jsonFetch('/api/file-transfer/jobs/control',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
}

globalThis.TaskMenuFileTransfer={
  snapshotState(){
    return [...views.values()].map(view=>({
      profile_id:String(view.profile?.id||''),
      local_mode:String(view.left?.source||'host'),
      local_path:String(view.left?.currentPath||'.'),
      remote_path:String(view.remote?.currentPath||'.')
    })).filter(item=>item.profile_id);
  },
  async restoreState(items){
    await scheduleFileTransferSessionRestore();
    await refreshProfiles();
    for(const item of Array.isArray(items)?items:[]){
      const profileID=String(item?.profile_id||'').trim();if(!profileID)continue;
      const profile=profilesByID.get(profileID);if(!profile){console.warn('Snapshot file-transfer profile unavailable:',profileID);continue;}
      const view=views.get(profileID)||attachView(profile,{activate:false,session:null});
      const mode=String(item?.local_mode||'host');
      const localPath=normalizeRelativePath(item?.local_path||'.');
      try{
        if(mode==='host')await switchLeftSource(view,'host',localPath);
        else if(mode==='local'&&view.left?.localRoot)await switchLeftSource(view,'local',localPath);
      }catch(error){console.warn('Snapshot left file-transfer restore failed for '+profileID,error);}
      const remotePath=normalizeRemotePath(item?.remote_path||profile.initial_path||'.');
      try{view.remote.currentPath=remotePath;view.remote.pathInput.value=remotePath;view.remote.refreshPathMemory?.();await loadRemoteDirectory(view,remotePath);}catch(error){console.warn('Snapshot remote file-transfer restore failed for '+profileID,error);}
    }
    persistFileTransferSession();
    return true;
  },
  openProfile,testProfile,testDraft,refreshProfiles,getProfile:id=>profilesByID.get(String(id||''))||null,restoreSession:restoreFileTransferSession,
  get profiles(){return [...profilesByID.values()];},
  operationSnapshot:fileTransferOperationSnapshot,
  operationControl:fileTransferOperationControl,
  get views(){return views;}
};
function currentSyncOptions(view){
  return normalizeSyncProfile({
    profile_id:view.profile.id,left_source:view.left.source,local_root_id:view.left.localRoot?.id||'',
    left_path:view.left.currentPath||'.',remote_path:view.remote.currentPath||'.',
    compare_mode:'metadata',direction:'right',exclude:[],allow_delete:false
  },view);
}
async function applySyncContext(view,options){
  options=normalizeSyncProfile(options,view);
  if(options.left_source==='local'&&options.local_root_id){
    safeStorageSet('taskdeck:file-transfer:last-local-root:'+workspaceKey(),options.local_root_id);
  }
  if(view.left.sourceSelect)view.left.sourceSelect.value=options.left_source;
  await switchLeftSource(view,options.left_source,options.left_path);
  if(options.left_source==='local'&&options.local_root_id&&view.left.localRoot?.id!==options.local_root_id){
    await refreshLocalRoots(view,options.local_root_id);
    if(view.left.localRoot?.id!==options.local_root_id)throw new Error('Saved Local folder is unavailable. Choose it again before using this Sync Profile.');
    await loadLocalDirectory(view,options.left_path);
  }
  view.remote.currentPath=options.remote_path;
  if(view.remote.pathInput)view.remote.pathInput.value=options.remote_path;
  view.remote.refreshPathMemory?.();
  persistFileTransferSession();
}
function openSyncSetupDialog(view){
  return new Promise(resolve=>{
    let profiles=readSyncProfiles(view),selectedID='',draft=currentSyncOptions(view);
    const backdrop=document.createElement('div');backdrop.className='ft-sync-backdrop';
    const dialog=document.createElement('div');dialog.className='ft-sync-dialog';
    const title=document.createElement('h3');title.textContent='Folder Sync / Mirror — Setup';
    const note=document.createElement('div');note.className='ft-sync-note';note.textContent='Dry run is always read-only. Saved profiles keep paths/options only; destination deletion remains disabled unless explicitly enabled.';
    const form=document.createElement('div');form.className='ft-sync-config';
    const addField=(labelText,control)=>{const label=document.createElement('label');label.textContent=labelText;form.append(label,control);return control;};
    const profileSelect=addField('Saved profile',document.createElement('select'));
    const renderProfiles=()=>{
      profileSelect.replaceChildren();
      const current=document.createElement('option');current.value='';current.textContent='Current paths / unsaved';profileSelect.append(current);
      for(const profile of profiles){const option=document.createElement('option');option.value=profile.id;option.textContent=profile.name;profileSelect.append(option);}
      profileSelect.value=profiles.some(item=>item.id===selectedID)?selectedID:'';
    };
    const direction=addField('Default action',document.createElement('select'));
    for(const [value,label] of [['right','Sync Left → Remote'],['left','Sync Remote → Left'],['bidirectional','Bidirectional Sync'],['mirror_right','Mirror Left → Remote'],['mirror_left','Mirror Remote → Left']]){const option=document.createElement('option');option.value=value;option.textContent=label;direction.append(option);}
    const compare=addField('Compare mode',document.createElement('select'));
    for(const [value,label] of [['metadata','Size + Modified time (fast)'],['checksum','SHA-256 when file sizes match (slower)']]){const option=document.createElement('option');option.value=value;option.textContent=label;compare.append(option);}
    const excludes=addField('Exclude patterns',document.createElement('textarea'));excludes.placeholder='One glob per line, e.g.\n.git/**\nnode_modules/**\nbuild/*.tmp';
    const deleteWrap=document.createElement('label');deleteWrap.className='ft-sync-check';
    const allowDelete=document.createElement('input');allowDelete.type='checkbox';
    const deleteText=document.createElement('span');deleteText.textContent='Allow destination deletes when running a saved Mirror action';
    deleteWrap.append(allowDelete,deleteText);form.append(document.createElement('span'),deleteWrap);
    const loadDraft=value=>{
      draft=normalizeSyncProfile(value,view);direction.value=draft.direction;compare.value=draft.compare_mode;excludes.value=draft.exclude.join('\n');allowDelete.checked=Boolean(draft.allow_delete);
    };
    const readDraft=()=>normalizeSyncProfile({...draft,direction:direction.value,compare_mode:compare.value,exclude:excludes.value,allow_delete:allowDelete.checked},view);
    profileSelect.onchange=()=>{selectedID=profileSelect.value;const profile=profiles.find(item=>item.id===selectedID);loadDraft(profile||currentSyncOptions(view));};
    const actions=document.createElement('div');actions.className='ft-sync-actions';
    const remove=document.createElement('button');remove.type='button';remove.textContent='Delete profile';remove.onclick=()=>{
      if(!selectedID)return;if(!confirm('Delete this Sync Profile?'))return;deleteSyncProfile(view,selectedID);profiles=readSyncProfiles(view);selectedID='';renderProfiles();loadDraft(currentSyncOptions(view));
    };
    const save=document.createElement('button');save.type='button';save.textContent='Save profile…';save.onclick=()=>{
      try{
        const existing=profiles.find(item=>item.id===selectedID);
        const name=prompt(existing?'Sync Profile name:':'New Sync Profile name:',existing?.name||'');if(name===null)return;
        const saved=saveSyncProfile(view,readDraft(),name,existing?.id||'');profiles=readSyncProfiles(view);selectedID=saved.id;renderProfiles();loadDraft(saved);
      }catch(error){app.showError(error);}
    };
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=()=>{backdrop.remove();resolve(null);};
    const scan=document.createElement('button');scan.type='button';scan.textContent='Dry run';scan.onclick=()=>{
      try{const value=readDraft();backdrop.remove();resolve(value);}catch(error){app.showError(error);}
    };
    actions.append(remove,save,cancel,scan);dialog.append(title,note,form,actions);backdrop.append(dialog);document.body.append(backdrop);
    renderProfiles();loadDraft(draft);profileSelect.focus();
    backdrop.addEventListener('pointerdown',event=>{if(event.target===backdrop){backdrop.remove();resolve(null);}});
    backdrop.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();backdrop.remove();resolve(null);}});
  });
}
async function configureAndCompareFolders(view){
  const options=await openSyncSetupDialog(view);if(!options)return;
  await applySyncContext(view,options);
  return compareFoldersDryRun(view,options);
}
