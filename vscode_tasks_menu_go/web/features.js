const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp extension API unavailable');

const states=new Map();
const decoder=new TextDecoder();
const style=document.createElement('style');
style.textContent=`
.detected-actions{display:none;max-height:180px;overflow:auto;border-bottom:1px solid #30343b;background:#0d1117;padding:5px 10px;font-size:12px}
.detected-actions.has-items{display:block}
.detected-row{display:flex;align-items:center;gap:7px;min-width:0;margin:3px 0}
.detected-kind{opacity:.55;min-width:30px;text-transform:uppercase;font-size:10px}
.detected-link{font-family:ui-monospace,monospace;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1;color:#8cc8ff;text-decoration:none}
.detected-link:hover{text-decoration:underline}
.detected-row button{padding:3px 7px;font-size:11px;white-space:nowrap}
.detected-copy{min-width:52px}
.detected-download{background:#24472f;border-color:#3b7850}
.detected-ignore{opacity:.75}
.task-search-panel{position:sticky;top:0;z-index:5;background:#101216;padding-bottom:8px;margin-bottom:4px}
.task-search-input{width:100%;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:8px 10px;outline:none}
.task-search-input:focus{border-color:#5a88b4}
.task-search-results{display:none;margin-top:5px;border:1px solid #30343b;border-radius:6px;background:#12161d;max-height:46vh;overflow:auto;padding:4px}
.task-search-results.visible{display:block}
.task-search-result{display:block;width:100%;text-align:left;padding:7px 8px;margin:1px 0;background:#171a20;border:1px solid transparent;border-radius:5px}
.task-search-result.selected,.task-search-result:hover{background:#293241;border-color:#42536a}
.task-search-result .label{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.task-search-result .path{display:block;font-size:10px;opacity:.55;margin-top:2px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.task-search-empty{padding:8px;opacity:.6;font-size:12px}
`;
document.head.append(style);

function ignoredStorageKey(view){return 'vscode-tasks-menu:ignored-files:'+view.meta.id;}
function loadIgnored(view){
  try{
    const raw=sessionStorage.getItem(ignoredStorageKey(view));
    const values=raw?JSON.parse(raw):[];
    return new Set(Array.isArray(values)?values.filter(value=>typeof value==='string'):[]);
  }catch{return new Set();}
}
function saveIgnored(state){
  try{sessionStorage.setItem(ignoredStorageKey(state.view),JSON.stringify([...state.ignored]));}
  catch(e){console.warn('Cannot persist ignored detected files',e);}
}
function ignoredURLStorageKey(view){return 'vscode-tasks-menu:ignored-urls:'+view.meta.id;}
function loadIgnoredURLs(view){
  try{
    const raw=sessionStorage.getItem(ignoredURLStorageKey(view));
    const values=raw?JSON.parse(raw):[];
    return new Set(Array.isArray(values)?values.filter(value=>typeof value==='string'):[]);
  }catch{return new Set();}
}
function saveIgnoredURLs(state){
  try{sessionStorage.setItem(ignoredURLStorageKey(state.view),JSON.stringify([...state.ignoredURLs]));}
  catch(e){console.warn('Cannot persist ignored detected URLs',e);}
}
function fileDetectionStorageKey(view){return 'vscode-tasks-menu:file-detection:'+view.meta.id;}
function loadFileDetectionEnabled(view){
  try{return sessionStorage.getItem(fileDetectionStorageKey(view))!=='0';}
  catch{return true;}
}
function saveFileDetectionEnabled(state){
  try{sessionStorage.setItem(fileDetectionStorageKey(state.view),state.fileDetectionEnabled?'1':'0');}
  catch(e){console.warn('Cannot persist file detection state',e);}
}

function gitExecutable(value){
  const token=String(value||'').trim().replace(/^['"]|['"]$/g,'');
  return token==='git'||token.endsWith('/git');
}
function shellAssignment(token){return /^[A-Za-z_][A-Za-z0-9_]*=/.test(token);}
function standaloneGitCommand(line){
  const text=String(line||'').trim();
  if(!text||/&&|\|\||[;|]/.test(text))return false;
  const tokens=text.split(/\s+/).filter(Boolean);let i=0;
  while(i<tokens.length&&shellAssignment(tokens[i]))i++;
  if(tokens[i]==='command'||tokens[i]==='builtin')i++;
  if(tokens[i]==='sudo'){
    i++;
    while(i<tokens.length&&tokens[i].startsWith('-'))i++;
  }
  if(tokens[i]==='env'){
    i++;
    while(i<tokens.length&&(tokens[i].startsWith('-')||shellAssignment(tokens[i])))i++;
  }
  return gitExecutable(tokens[i]);
}
function taskUsesGit(view){
  if(!view?.meta?.task_id||!app.taskData)return false;
  const task=app.taskData.tasks.find(item=>item.id===view.meta.task_id);if(!task)return false;
  if((task.group||[]).some(group=>String(group).toLowerCase()==='git'))return true;
  if(typeof task.command==='string'&&gitExecutable(task.command))return true;
  for(const arg of Array.isArray(task.args)?task.args:[]){
    if(typeof arg==='string'&&/^\s*(?:sudo\s+|command\s+)?(?:\/\S*\/)?git(?:\s|$)/.test(arg))return true;
  }
  return false;
}
function beginGitOutputSuppression(state){
  state.suppressGitOutput=true;
  state.recent='';
  clearTimeout(state.timer);
  state.seq++;
}
function endGitOutputSuppression(state){
  if(!state.suppressGitOutput)return;
  state.suppressGitOutput=false;
  state.recent='';
  clearTimeout(state.timer);
  state.seq++;
}

function stateFor(view){
  let state=states.get(view.meta.id);
  if(state)return state;
  const bar=document.createElement('div');
  bar.className='detected-actions';
  const head=view.pane.querySelector('.pane-head');
  head.after(bar);
  state={view,bar,recent:'',timer:null,seq:0,files:new Map(),urls:new Map(),ignored:loadIgnored(view),ignoredURLs:loadIgnoredURLs(view),fileDetectionEnabled:loadFileDetectionEnabled(view),inputBuffer:'',inputLines:[],inputDisposable:null,suppressGitOutput:false,gitTaskOutput:taskUsesGit(view)};
  states.set(view.meta.id,state);
  state.inputDisposable=view.term.onData(data=>captureUserInput(state,data));
  return state;
}

function isFileDetectionEnabled(view){return stateFor(view).fileDetectionEnabled;}
function setFileDetectionEnabled(view,enabled){
  const state=stateFor(view);const next=Boolean(enabled);
  if(state.fileDetectionEnabled===next)return next;
  state.fileDetectionEnabled=next;
  saveFileDetectionEnabled(state);
  clearTimeout(state.timer);
  state.seq++;
  state.recent='';
  state.files.clear();
  state.urls.clear();
  render(state);
  window.dispatchEvent(new CustomEvent('taskmenu:file-detection-changed',{detail:{view,enabled:next}}));
  return next;
}
globalThis.TaskMenuFileDetection={isEnabled:isFileDetectionEnabled,setEnabled:setFileDetectionEnabled};

function commitUserInput(state){
  const line=state.inputBuffer.trim();
  state.inputBuffer='';
  if(!line)return;
  state.inputLines.push(line);
  if(state.inputLines.length>64)state.inputLines.splice(0,state.inputLines.length-64);
  if(standaloneGitCommand(line))beginGitOutputSuppression(state);
  else endGitOutputSuppression(state);
}

function captureUserInput(state,data){
  for(const ch of String(data||'')){
    if(ch==='\r'||ch==='\n'){commitUserInput(state);continue;}
    if(ch==='\x7f'||ch==='\b'){state.inputBuffer=state.inputBuffer.slice(0,-1);continue;}
    if(ch==='\x1b')continue;
    if(ch>=' '){
      endGitOutputSuppression(state);
      state.inputBuffer=(state.inputBuffer+ch).slice(-4096);
    }
  }
  render(state);
}

function userTypedFile(state,path){
  if(!path)return false;
  const workspace=(app.taskData?.workspace||'').replace(/[\\/]+$/,'');
  let relative='';
  if(workspace&&path.startsWith(workspace+'/'))relative=path.slice(workspace.length+1);
  const needles=[path,relative,relative?'./'+relative:''].filter(value=>value&&value.length>2);
  const lines=[...state.inputLines,state.inputBuffer];
  return lines.some(line=>needles.some(needle=>line.includes(needle)));
}

async function copyText(text,button){
  let copied=false;
  if(navigator.clipboard&&window.isSecureContext){
    try{await navigator.clipboard.writeText(text);copied=true;}catch{}
  }
  if(!copied){
    const area=document.createElement('textarea');
    area.value=text;area.setAttribute('readonly','');area.style.position='fixed';area.style.left='-9999px';
    document.body.append(area);area.select();
    try{copied=document.execCommand('copy');}finally{area.remove();}
  }
  if(!copied)throw new Error('Cannot copy to clipboard');
  if(button){const old=button.textContent;button.textContent='Copied';setTimeout(()=>{if(button.isConnected)button.textContent=old;},1000);}
}

function openDownload(file){
  const a=document.createElement('a');
  a.href=file.url;a.download=file.name||'';a.rel='noopener';document.body.append(a);a.click();a.remove();
}

function trimURL(value){return value.replace(/[),.;:'"\]}]+$/g,'');}

function detectURLs(text){
  const out=[];
  const seen=new Set();
  const absolute=text.match(/https?:\/\/[^\s<>"']+/g)||[];
  const local=text.match(/(?:^|[\s(])((?:localhost|127\.0\.0\.1):\d+(?:\/[^\s<>"']*)?)/g)||[];
  for(const raw of absolute){const url=trimURL(raw);if(url&&!seen.has(url)){seen.add(url);out.push(url);}}
  for(const raw of local){
    const match=raw.match(/((?:localhost|127\.0\.0\.1):\d+(?:\/[^\s<>"']*)?)/);
    if(!match)continue;
    const url='http://'+trimURL(match[1]);if(!seen.has(url)){seen.add(url);out.push(url);}
  }
  return out;
}

function remember(map,key,value,max=8){
  if(map.has(key))map.delete(key);
  map.set(key,value);
  while(map.size>max)map.delete(map.keys().next().value);
}

function detectedPathMaxChars(){
  const width=document.querySelector('#panes')?.clientWidth||Math.max(640,window.innerWidth-360);
  return Math.max(96,Math.min(180,Math.floor((width-240)/7)));
}

function compactPathPart(value,max){
  const text=String(value||'');
  if(text.length<=max)return text;
  if(max<=3)return '...';
  return text.slice(0,max-3)+'...';
}

function compactDetectedPath(value,maxChars=detectedPathMaxChars()){
  const path=String(value||'').replace(/\\/g,'/');
  if(path.length<=maxChars)return path;
  const parts=path.split('/').filter(Boolean);
  const filename=parts.pop()||path;
  const lead='.../';
  if(!parts.length||lead.length+filename.length>=maxChars)return lead+filename;

  const dirBudget=Math.max(0,maxChars-lead.length-filename.length-1);
  const context=parts.slice(-2);
  if(context.length===1)return lead+compactPathPart(context[0],dirBudget)+'/'+filename;

  const first=context[0],last=context[1];
  const firstBudget=Math.min(first.length,Math.max(7,Math.floor(dirBudget*0.38)));
  const firstShown=compactPathPart(first,firstBudget);
  const lastBudget=Math.max(4,dirBudget-firstShown.length-1);
  const lastShown=compactPathPart(last,lastBudget);
  return lead+firstShown+'/'+lastShown+'/'+filename;
}

function filePriority(file){
  const path=String(file?.path||'').replace(/\\/g,'/');
  const name=String(file?.name||path.split('/').pop()||'');
  let score=0;
  if(path.includes('/artifacts/ptv_to_ai/'))score+=1000;
  if(/^CR_[^/]+\.zip$/i.test(name))score+=500;
  else if(/^CR_[^/]+\.txt$/i.test(name))score+=400;
  else if(/\.zip$/i.test(name))score+=50;
  return score;
}

function ignoreFile(state,file){
  if(!file?.path)return;
  state.ignored.add(file.path);
  state.files.delete(file.path);
  saveIgnored(state);
  render(state);
}

function ignoreURL(state,url){
  url=String(url||'').trim();
  if(!url)return;
  state.ignoredURLs.add(url);
  state.urls.delete(url);
  saveIgnoredURLs(state);
  render(state);
}

function render(state){
  const rows=[];
  const ranked=[...state.files.values()].reverse()
    .filter(file=>!state.ignored.has(file.path)&&!userTypedFile(state,file.path))
    .map((file,index)=>({file,index,score:filePriority(file)}))
    .sort((a,b)=>b.score-a.score||a.index-b.index);
  for(const {file} of ranked){
    const row=document.createElement('div');row.className='detected-row';
    const kind=document.createElement('span');kind.className='detected-kind';kind.textContent='FILE';
    const link=document.createElement('a');link.className='detected-link';link.href=file.url;link.download=file.name||'';link.textContent=compactDetectedPath(file.path);link.title=file.path;
    const download=document.createElement('button');download.className='detected-download';download.textContent='Download';download.onclick=()=>openDownload(file);
    const copy=document.createElement('button');copy.className='detected-copy';copy.textContent='Copy';copy.onclick=()=>copyText(file.path,copy).catch(app.showError);
    const ignore=document.createElement('button');ignore.className='detected-ignore';ignore.textContent='Ignore';ignore.title='Hide this file for the current session';ignore.onclick=()=>ignoreFile(state,file);
    row.append(kind,link,download,copy,ignore);rows.push(row);
  }
  for(const url of [...state.urls.values()].reverse().filter(url=>!state.ignoredURLs.has(url))){
    const row=document.createElement('div');row.className='detected-row';
    const kind=document.createElement('span');kind.className='detected-kind';kind.textContent='URL';
    const link=document.createElement('a');link.className='detected-link';link.href=url;link.target='_blank';link.rel='noopener noreferrer';link.textContent=url;link.title=url;
    const open=document.createElement('button');open.textContent='Open';open.onclick=()=>window.open(url,'_blank','noopener');
    const copy=document.createElement('button');copy.className='detected-copy';copy.textContent='Copy';copy.onclick=()=>copyText(url,copy).catch(app.showError);
    const ignore=document.createElement('button');ignore.className='detected-ignore';ignore.textContent='Ignore';ignore.title='Hide this URL for the current session';ignore.onclick=()=>ignoreURL(state,url);
    row.append(kind,link,open,copy,ignore);rows.push(row);
  }
  state.bar.replaceChildren(...rows.slice(0,12));
  state.bar.classList.toggle('has-items',rows.length>0);
}

function scheduleScan(view,text){
  const state=stateFor(view);
  if(!state.fileDetectionEnabled||state.gitTaskOutput||state.suppressGitOutput)return;
  for(const url of detectURLs(text)){
    if(!state.ignoredURLs.has(url))remember(state.urls,url,url);
  }
  render(state);
  state.recent=(state.recent+text).slice(-131072);
  clearTimeout(state.timer);const seq=++state.seq;
  state.timer=setTimeout(()=>scanFiles(state,seq),220);
}

async function scanFiles(state,seq){
  try{
    if(!state.fileDetectionEnabled||state.gitTaskOutput||state.suppressGitOutput)return;
    const data=await app.jsonFetch('/api/files/selection',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({text:state.recent})});
    if(seq!==state.seq||state.view.closed||!state.fileDetectionEnabled||state.gitTaskOutput||state.suppressGitOutput)return;
    for(const file of data.files||[]){
      if(state.ignored.has(file.path)||userTypedFile(state,file.path))continue;
      remember(state.files,file.path,file,32);
    }
    render(state);
  }catch(e){console.warn('Automatic file detection failed',e);}
}

function scanExisting(view){
  const state=stateFor(view);
  if(state.gitTaskOutput)return;
  const text=app.consoleText(view);
  if(text)scheduleScan(view,text.slice(-131072));
}

window.addEventListener('taskmenu:output',event=>{
  const {view,data}=event.detail||{};if(!view||data==null)return;
  let text='';
  if(typeof data==='string')text=data;
  else if(data instanceof ArrayBuffer)text=decoder.decode(new Uint8Array(data));
  else if(ArrayBuffer.isView(data))text=decoder.decode(data);
  if(text)scheduleScan(view,text);
});

window.addEventListener('taskmenu:session',event=>{
  const view=event.detail?.view;if(view&&!states.has(view.meta.id))scanExisting(view);
});

for(const view of app.views.values())scanExisting(view);

let taskSearch=null;
function taskSearchText(task){
  return [task.menu_label,task.label,task.detail,...(task.group||[])].filter(Boolean).join(' ').toLowerCase();
}

function searchTasks(query){
  const q=query.trim().toLowerCase();
  if(!q||!app.taskData)return [];
  const terms=q.split(/\s+/).filter(Boolean);
  return app.taskData.tasks.map((task,index)=>{
    const hay=taskSearchText(task);
    if(!terms.every(term=>hay.includes(term)))return null;
    const label=(task.menu_label||task.label||'').toLowerCase();
    let score=0;
    if(label===q)score+=100;
    else if(label.startsWith(q))score+=60;
    else if(label.includes(q))score+=30;
    score-=index/100000;
    return {task,score};
  }).filter(Boolean).sort((a,b)=>b.score-a.score).slice(0,12).map(item=>item.task);
}

function installTaskSearch(){
  const menu=document.querySelector('#menu');
  if(!menu)return;
  const old=menu.querySelector('.task-search-panel');if(old)old.remove();
  const panel=document.createElement('div');panel.className='task-search-panel';
  const input=document.createElement('input');input.className='task-search-input';input.type='search';input.autocomplete='off';input.spellcheck=false;input.placeholder='Search tasks…  Ctrl+K';
  const results=document.createElement('div');results.className='task-search-results';
  panel.append(input,results);menu.prepend(panel);
  taskSearch={panel,input,results,items:[],selected:0};

  const renderResults=()=>{
    if(!taskSearch)return;
    const q=input.value.trim();
    taskSearch.items=searchTasks(q);taskSearch.selected=0;
    results.replaceChildren();
    results.classList.toggle('visible',Boolean(q));
    if(!q)return;
    if(!taskSearch.items.length){const empty=document.createElement('div');empty.className='task-search-empty';empty.textContent='No tasks found';results.append(empty);return;}
    taskSearch.items.forEach((task,index)=>{
      const b=document.createElement('button');b.className='task-search-result'+(index===0?' selected':'');
      const label=document.createElement('span');label.className='label';label.textContent=task.menu_label||task.label;
      const path=document.createElement('span');path.className='path';path.textContent=(task.group||[]).join(' / ')+(task.detail?' — '+task.detail:'');
      b.append(label,path);b.onclick=()=>runSearchTask(task);results.append(b);
    });
  };
  const selectIndex=index=>{
    if(!taskSearch?.items.length)return;
    taskSearch.selected=(index+taskSearch.items.length)%taskSearch.items.length;
    [...results.querySelectorAll('.task-search-result')].forEach((el,i)=>el.classList.toggle('selected',i===taskSearch.selected));
    results.querySelector('.task-search-result.selected')?.scrollIntoView({block:'nearest'});
  };
  const runSelected=()=>{const task=taskSearch?.items[taskSearch.selected];if(task)runSearchTask(task);};
  input.oninput=renderResults;
  input.onkeydown=e=>{
    if(e.key==='ArrowDown'){e.preventDefault();selectIndex(taskSearch.selected+1);}
    else if(e.key==='ArrowUp'){e.preventDefault();selectIndex(taskSearch.selected-1);}
    else if(e.key==='Enter'){e.preventDefault();runSelected();}
    else if(e.key==='Escape'){e.preventDefault();input.value='';renderResults();input.blur();}
  };
}

function runSearchTask(task){
  if(taskSearch){taskSearch.input.value='';taskSearch.results.classList.remove('visible');taskSearch.results.replaceChildren();}
  app.startTask(task).catch(app.showError);
}

document.addEventListener('keydown',e=>{
  if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='k'){
    e.preventDefault();
    if(!taskSearch)installTaskSearch();
    taskSearch?.input.focus();taskSearch?.input.select();
  }
});
window.addEventListener('taskmenu:tasks',()=>installTaskSearch());
installTaskSearch();