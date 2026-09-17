const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for progressive features');

const seenRecentSessions=new Set();
const style=document.createElement('style');
style.textContent=`
.quick-task-section{margin:6px 0 10px;border:1px solid #30343b;border-radius:6px;background:#12161d;overflow:hidden}
.quick-task-title{font-size:11px;font-weight:600;opacity:.7;padding:6px 8px;border-bottom:1px solid #30343b}
.quick-task-list{padding:3px}
.quick-task-row{display:flex;align-items:center;gap:4px}
.quick-task-run{flex:1;min-width:0;text-align:left;background:#171a20;border:1px solid transparent;padding:6px 7px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.quick-task-run:hover{background:#242a34}
.quick-task-star{padding:5px 7px;min-width:31px;color:#f4c95d}
.task-row-ext{display:flex;align-items:stretch;gap:3px;margin:2px 0}
.task-row-ext>.task{margin:0;flex:1;min-width:0}
.task-row-ext>.task-favorite-toggle{padding:5px 7px;min-width:31px;color:#f4c95d}
.session-rerun{white-space:nowrap}
.tab.state-running{border-top:2px solid #5aa9e6}.tab.state-success{border-top:2px solid #5fbf74}.tab.state-fail{border-top:2px solid #e35d6a}.tab.state-stopped{border-top:2px solid #d0a84d}
.history-section{margin:6px 0 10px;border:1px solid #30343b;border-radius:6px;background:#12161d;overflow:hidden}
.history-head{display:flex;align-items:center;gap:6px;padding:5px 7px;border-bottom:1px solid #30343b}.history-head strong{font-size:11px;opacity:.75;flex:1}.history-clear{padding:2px 6px;font-size:10px}
.history-list{max-height:180px;overflow:auto;padding:3px}.history-row{display:grid;grid-template-columns:1fr auto;gap:2px 6px;padding:5px 6px;border-bottom:1px solid #222831;font-size:11px}.history-row:last-child{border-bottom:0}.history-label{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.history-meta{opacity:.55;grid-column:1/3}.history-state.pass{color:#78d68b}.history-state.fail{color:#ff7b86}.history-state.stopped{color:#e4be63}
.console-find{display:none;align-items:center;gap:6px;padding:5px 10px;background:#101820;border-bottom:1px solid #30343b}.console-find.visible{display:flex}.console-find input{flex:1;min-width:120px;background:#090d12;color:inherit;border:1px solid #3b414d;border-radius:5px;padding:5px 7px}.console-find .match{font-size:11px;opacity:.65;min-width:72px;text-align:right}.console-tool{white-space:nowrap;padding:5px 8px}
`;
document.head.append(style);

function emptyProjectState(){return {version:1,favorites:[],recent:[],history:[]};}
let projectState=emptyProjectState();
let projectStateLoaded=false;
let projectStateSaveChain=Promise.resolve();

function legacyStorageKey(name){return 'vscode-tasks-menu:'+name+':'+app.taskData.workspace;}
function legacyRead(name,fallback){
  try{const value=JSON.parse(localStorage.getItem(legacyStorageKey(name))||'null');return value??fallback;}catch{return fallback;}
}
function normalizeIDs(values,limit){
  const out=[];const seen=new Set();
  for(const value of Array.isArray(values)?values:[]){
    if(!Number.isInteger(value)||value<=0||seen.has(value))continue;
    seen.add(value);out.push(value);if(out.length===limit)break;
  }
  return out;
}
function normalizeHistory(values){
  const out=[];const seen=new Set();
  for(const raw of Array.isArray(values)?values:[]){
    if(!raw||typeof raw!=='object')continue;
    const taskID=Number(raw.task_id);const sessionID=String(raw.session_id||'').trim();
    if(!Number.isInteger(taskID)||taskID<=0||!sessionID||seen.has(sessionID))continue;
    seen.add(sessionID);
    out.push({session_id:sessionID,task_id:taskID,label:String(raw.label||'').slice(0,512),status:String(raw.status||'').slice(0,32),exit_code:Number.isInteger(raw.exit_code)?raw.exit_code:null,started_at:String(raw.started_at||'').slice(0,80),ended_at:String(raw.ended_at||'').slice(0,80),duration:Number.isFinite(raw.duration)?Math.max(0,Math.floor(raw.duration)):null});
    if(out.length===10)break;
  }
  return out;
}
function normalizeProjectState(raw){
  return {version:1,favorites:normalizeIDs(raw?.favorites,20),recent:normalizeIDs(raw?.recent,10),history:normalizeHistory(raw?.history)};
}
function projectStateHasData(state){return state.favorites.length>0||state.recent.length>0||state.history.length>0;}
function clearLegacyProjectState(){
  try{for(const name of ['favorites','recent','history'])localStorage.removeItem(legacyStorageKey(name));}catch(e){console.warn('Cannot clear legacy task state',e);}
}
function queueProjectStateSave(){
  const snapshot=normalizeProjectState(projectState);projectState=snapshot;
  projectStateSaveChain=projectStateSaveChain.catch(()=>{}).then(()=>app.jsonFetch('/api/state/tasks',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(snapshot)})).catch(e=>{console.warn('Cannot persist project task state',e);});
  return projectStateSaveChain;
}
async function loadProjectState(){
  const loaded=normalizeProjectState(await app.jsonFetch('/api/state/tasks'));
  const legacy=normalizeProjectState({favorites:legacyRead('favorites',[]),recent:legacyRead('recent',[]),history:legacyRead('history',[])});
  projectState=loaded;
  if(!projectStateHasData(loaded)&&projectStateHasData(legacy)){
    projectState=legacy;
    await queueProjectStateSave();
  }
  clearLegacyProjectState();
  projectStateLoaded=true;refresh();
}
const projectStateReady=loadProjectState().catch(e=>{
  console.warn('Cannot load project task state; using legacy browser state for this page',e);
  projectState=normalizeProjectState({favorites:legacyRead('favorites',[]),recent:legacyRead('recent',[]),history:legacyRead('history',[])});
  projectStateLoaded=true;refresh();
});

function taskByID(id){return app.taskData?.tasks.find(t=>t.id===id)||null;}
function favorites(){return projectState.favorites.filter(id=>taskByID(id));}
function recent(){return projectState.recent.filter(id=>taskByID(id));}
function isFavorite(id){return favorites().includes(id);}

function toggleFavorite(id){
  if(!projectStateLoaded){projectStateReady.then(()=>toggleFavorite(id));return;}
  let ids=favorites();
  if(ids.includes(id))ids=ids.filter(x=>x!==id);else ids=[id,...ids].slice(0,20);
  projectState.favorites=ids;queueProjectStateSave();refresh();
}
function recordRecent(id){
  if(!projectStateLoaded){projectStateReady.then(()=>recordRecent(id));return;}
  projectState.recent=[id,...recent().filter(x=>x!==id)].slice(0,10);queueProjectStateSave();renderQuickSections();
}

function createQuickSection(title,ids,allowUnstar){
  if(!ids.length)return null;
  const section=document.createElement('div');section.className='quick-task-section';
  const heading=document.createElement('div');heading.className='quick-task-title';heading.textContent=title;
  const list=document.createElement('div');list.className='quick-task-list';section.append(heading,list);
  for(const id of ids){
    const task=taskByID(id);if(!task)continue;
    const row=document.createElement('div');row.className='quick-task-row';
    const run=document.createElement('button');run.className='quick-task-run';run.textContent=task.menu_label||task.label;run.title=task.detail||task.label;run.onclick=()=>app.startTask(task).catch(app.showError);
    row.append(run);
    if(allowUnstar){const star=document.createElement('button');star.className='quick-task-star';star.textContent='★';star.title='Remove from Favorites';star.onclick=()=>toggleFavorite(id);row.append(star);}
    list.append(row);
  }
  return section;
}

function renderQuickSections(){
  const menu=document.querySelector('#menu');if(!menu||!app.taskData)return;
  menu.querySelectorAll('.quick-task-section').forEach(el=>el.remove());
  const search=menu.querySelector('.task-search-panel');const anchor=search?.nextSibling||menu.firstChild;
  const blocks=[];const fav=createQuickSection('★ FAVORITES',favorites(),true);if(fav)blocks.push(fav);const rec=createQuickSection('↻ RECENT',recent(),false);if(rec)blocks.push(rec);
  for(const block of blocks){if(anchor)menu.insertBefore(block,anchor);else menu.append(block);}
}

function findTaskForButton(button){
  const label=button.textContent.trim();const title=button.title||'';
  const matches=(app.taskData?.tasks||[]).filter(task=>(task.menu_label||task.label)===label&&(!title||(task.detail||task.label)===title));
  return matches.length===1?matches[0]:null;
}

function decorateTaskButtons(){
  const menu=document.querySelector('#menu');if(!menu)return;
  for(const button of [...menu.querySelectorAll('button.task')]){
    if(button.parentElement?.classList.contains('task-row-ext'))continue;
    const task=findTaskForButton(button);if(!task)continue;
    const row=document.createElement('div');row.className='task-row-ext';button.before(row);row.append(button);
    const star=document.createElement('button');star.className='task-favorite-toggle';star.dataset.taskId=String(task.id);star.onclick=()=>toggleFavorite(task.id);row.append(star);
  }
  for(const star of menu.querySelectorAll('.task-favorite-toggle')){
    const id=Number(star.dataset.taskId);star.textContent=isFavorite(id)?'★':'☆';star.title=isFavorite(id)?'Remove from Favorites':'Add to Favorites';
  }
}

function refresh(){renderQuickSections();decorateTaskButtons();renderHistory();}
window.addEventListener('taskmenu:tasks',()=>setTimeout(refresh,0));
window.addEventListener('taskmenu:session',event=>{
  const meta=event.detail?.meta;if(!meta||meta.task_id<=0||seenRecentSessions.has(meta.id))return;
  seenRecentSessions.add(meta.id);recordRecent(meta.task_id);
});
setTimeout(refresh,0);

const finalizedSessions=new Set();
function readHistory(){return projectState.history;}
function writeHistory(items){
  if(!projectStateLoaded){projectStateReady.then(()=>writeHistory(items));return;}
  projectState.history=normalizeHistory(items).slice(0,10);queueProjectStateSave();
}
function secondsBetween(start,end){const a=Date.parse(start||'');const b=Date.parse(end||'');return Number.isFinite(a)&&Number.isFinite(b)&&b>=a?Math.floor((b-a)/1000):null;}
function formatDuration(seconds){if(seconds==null)return '--:--';const s=Math.max(0,seconds|0);const h=Math.floor(s/3600),m=Math.floor((s%3600)/60),r=s%60;return h?String(h).padStart(2,'0')+':'+String(m).padStart(2,'0')+':'+String(r).padStart(2,'0'):String(m).padStart(2,'0')+':'+String(r).padStart(2,'0');}
function displayState(meta){
  if(meta.status==='running')return {text:'running',cls:'running'};
  if(meta.status==='stopped')return {text:'STOPPED',cls:'stopped'};
  if(meta.status==='exited'&&meta.exit_code===0)return {text:'PASS',cls:'success'};
  if(meta.status==='exited')return {text:'FAIL',cls:'fail'};
  return {text:meta.status||'unknown',cls:'stopped'};
}
function elapsedSeconds(meta){
  const start=Date.parse(meta.started_at||'');if(!Number.isFinite(start))return null;
  const end=meta.status==='running'?Date.now():Date.parse(meta.ended_at||'');
  return Number.isFinite(end)&&end>=start?Math.floor((end-start)/1000):null;
}

function ensureRerunButton(view){
  if(view.meta.task_id<=0)return;
  let button=view.pane.querySelector('.session-rerun');
  if(!button){button=document.createElement('button');button.className='session-rerun';view.stop.before(button);}
  button.textContent=view.meta.status==='running'?'Restart':'Run again';
  button.title=view.meta.status==='running'?'Stop the current task and run it again':'Run this task again';
  button.onclick=()=>restartOrRun(view).catch(app.showError);
}

async function restartOrRun(view){
  const task=taskByID(view.meta.task_id);if(!task)throw new Error('Task no longer exists in tasks.json');
  const button=view.pane.querySelector('.session-rerun');if(button)button.disabled=true;
  try{
    if(view.meta.status==='running'){
      await app.jsonFetch('/api/sessions/'+view.meta.id+'/stop',{method:'POST'});
      const deadline=Date.now()+6000;
      while(Date.now()<deadline){
        await new Promise(resolve=>setTimeout(resolve,120));
        const meta=await app.jsonFetch('/api/sessions/'+view.meta.id);
        if(meta.status!=='running')break;
      }
    }
    await app.startTask(task);
  }finally{if(button?.isConnected)button.disabled=false;}
}

function updateSessionPresentation(view,meta){
  const state=displayState(meta);const duration=formatDuration(elapsedSeconds(meta));
  view.status.textContent=state.text+' '+duration;
  view.tab.classList.remove('state-running','state-success','state-fail','state-stopped');view.tab.classList.add('state-'+state.cls);
  ensureRerunButton(view);
  ensureConsoleTools(view);
  if(meta.task_id>0&&meta.status!=='running'&&!finalizedSessions.has(meta.id))recordHistory(meta);
}

function recordHistory(meta){
  if(!projectStateLoaded){projectStateReady.then(()=>recordHistory(meta));return;}
  finalizedSessions.add(meta.id);
  const existing=readHistory();if(existing.some(item=>item.session_id===meta.id))return;
  const task=taskByID(meta.task_id);
  const item={session_id:meta.id,task_id:meta.task_id,label:task?.menu_label||meta.label,status:displayState(meta).text,exit_code:meta.exit_code??null,started_at:meta.started_at,ended_at:meta.ended_at,duration:secondsBetween(meta.started_at,meta.ended_at)};
  writeHistory([item,...existing]);renderHistory();
}

function renderHistory(){
  const menu=document.querySelector('#menu');if(!menu||!app.taskData)return;
  menu.querySelector('.history-section')?.remove();
  const items=readHistory().slice(0,10);if(!items.length)return;
  const section=document.createElement('div');section.className='history-section';
  const head=document.createElement('div');head.className='history-head';const title=document.createElement('strong');title.textContent='HISTORY';const clear=document.createElement('button');clear.className='history-clear';clear.textContent='Clear';clear.onclick=()=>{writeHistory([]);renderHistory();};head.append(title,clear);
  const list=document.createElement('div');list.className='history-list';section.append(head,list);
  for(const item of items){
    const row=document.createElement('div');row.className='history-row';const label=document.createElement('span');label.className='history-label';label.textContent=item.label||String(item.task_id);const state=document.createElement('span');state.className='history-state '+String(item.status||'').toLowerCase();state.textContent=item.status||'';
    const meta=document.createElement('span');meta.className='history-meta';const when=item.started_at?new Date(item.started_at).toLocaleString():'';meta.textContent=when+' · '+formatDuration(item.duration);
    const task=taskByID(item.task_id);if(task){row.style.cursor='pointer';row.title='Click to run again';row.onclick=()=>app.startTask(task).catch(app.showError);}
    row.append(label,state,meta);list.append(row);
  }
  const quick=[...menu.querySelectorAll('.quick-task-section')];const anchor=quick.length?quick[quick.length-1].nextSibling:menu.querySelector('.task-search-panel')?.nextSibling;
  if(anchor)menu.insertBefore(section,anchor);else menu.append(section);
}

const consoleStates=new Map();
function consoleState(view){
  let state=consoleStates.get(view.meta.id);if(state)return state;
  const bar=document.createElement('div');bar.className='console-find';
  const input=document.createElement('input');input.type='search';input.placeholder='Find in console…';input.autocomplete='off';input.spellcheck=false;
  const match=document.createElement('span');match.className='match';
  const prev=document.createElement('button');prev.textContent='↑';prev.title='Previous match';
  const next=document.createElement('button');next.textContent='↓';next.title='Next match';
  const close=document.createElement('button');close.textContent='×';close.title='Close search';
  bar.append(input,match,prev,next,close);
  const detected=view.pane.querySelector('.detected-actions');if(detected)detected.after(bar);else view.pane.querySelector('.pane-head').after(bar);
  state={view,bar,input,match,line:-1};consoleStates.set(view.meta.id,state);
  input.oninput=()=>findInConsole(state,1,true);input.onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();findInConsole(state,e.shiftKey?-1:1,false);}else if(e.key==='Escape'){e.preventDefault();hideConsoleFind(state);}};
  prev.onclick=()=>findInConsole(state,-1,false);next.onclick=()=>findInConsole(state,1,false);close.onclick=()=>hideConsoleFind(state);
  return state;
}
function showConsoleFind(view){const state=consoleState(view);state.bar.classList.add('visible');state.input.focus();state.input.select();if(state.input.value)findInConsole(state,1,true);}
function hideConsoleFind(state){state.bar.classList.remove('visible');state.view.term.clearSelection();state.view.term.focus();}
function findInConsole(state,direction,reset){
  const q=state.input.value;if(!q){state.match.textContent='';state.view.term.clearSelection();state.line=-1;return;}
  const buffer=state.view.term.buffer.active;const needle=q.toLowerCase();const count=buffer.length;if(!count)return;
  let start=reset?(direction>0?0:count-1):(state.line<0?(direction>0?0:count-1):state.line+direction);
  for(let step=0;step<count;step++){
    const row=(start+direction*step+count*2)%count;const line=buffer.getLine(row);if(!line)continue;const text=line.translateToString(true);const idx=text.toLowerCase().indexOf(needle);if(idx<0)continue;
    state.line=row;state.view.term.select(idx,row,q.length);state.view.term.scrollToLine(row);state.match.textContent='line '+(row+1);return;
  }
  state.match.textContent='No match';state.view.term.clearSelection();
}
function safeLogName(view){const base=(view.meta.label||'console').replace(/[^A-Za-z0-9._-]+/g,'_').replace(/^_+|_+$/g,'')||'console';const d=new Date();const stamp=d.getFullYear()+String(d.getMonth()+1).padStart(2,'0')+String(d.getDate()).padStart(2,'0')+'_'+String(d.getHours()).padStart(2,'0')+String(d.getMinutes()).padStart(2,'0')+String(d.getSeconds()).padStart(2,'0');return base+'_'+stamp+'.log';}
function saveConsoleLog(view){
  const text=app.consoleText(view);const blob=new Blob([text+(text.endsWith('\n')?'':'\n')],{type:'text/plain;charset=utf-8'});const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download=safeLogName(view);document.body.append(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(url),1000);
}
function ensureConsoleTools(view){
  const head=view.pane.querySelector('.pane-head');if(!head||head.querySelector('.console-search-btn'))return;
  const find=document.createElement('button');find.className='console-tool console-search-btn';find.textContent='🔎 Find in console';find.title='Search console (Ctrl+F)';find.onclick=()=>showConsoleFind(view);
  const save=document.createElement('button');save.className='console-tool console-save-btn';save.textContent='💾 Save console log';save.title='Download the entire console as a .log file';save.onclick=()=>saveConsoleLog(view);
  view.copy.before(find,save);
}

document.addEventListener('keydown',e=>{
  if(!(e.ctrlKey||e.metaKey)||e.key.toLowerCase()!=='f')return;
  const view=app.views.get(app.active);if(!view)return;
  e.preventDefault();showConsoleFind(view);
});
window.addEventListener('taskmenu:session',event=>{const {view,meta}=event.detail||{};if(view&&meta)updateSessionPresentation(view,meta);});
setInterval(()=>{for(const view of app.views.values())if(view.meta.status==='running')updateSessionPresentation(view,view.meta);},1000);
for(const view of app.views.values())updateSessionPresentation(view,view.meta);