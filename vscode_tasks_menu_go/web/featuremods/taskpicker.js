// VS Code-style task quick picker; execution always uses the existing TaskDeck task flow.
const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for task picker');

const css=document.createElement('style');
css.textContent=[
'.task-picker-backdrop{display:none;position:fixed;inset:0;z-index:2820;background:rgba(0,0,0,.55);align-items:flex-start;justify-content:center;padding:clamp(20px,10vh,100px) 10px 16px}',
'.task-picker-backdrop.visible{display:flex}',
'.task-picker-dialog{width:min(740px,96vw);max-height:min(76vh,720px);min-height:0;display:flex;flex-direction:column;overflow:hidden;border:1px solid #52657e;border-radius:9px;background:#151b24;color:#eef5fc;box-shadow:0 18px 60px #0009}',
'.task-picker-top{display:flex;align-items:center;gap:12px;padding:10px 13px;border-bottom:1px solid #334155}',
'.task-picker-title{font-size:13px;font-weight:700;flex:1}',
'.task-picker-close{padding:3px 9px;font-size:17px;background:transparent;color:inherit;border:1px solid #45556a;border-radius:5px}',
'.task-picker-input{margin:11px 13px 7px;box-sizing:border-box;width:calc(100% - 26px);padding:10px 12px;font-size:14px;background:#0c121a;color:inherit;border:1px solid #607d9b;border-radius:6px;outline:none}',
'.task-picker-input:focus{border-color:#8bbcff}',
'.task-picker-count{font-size:11px;opacity:.68;padding:0 13px 8px}',
'.task-picker-results{min-height:56px;overflow-y:auto;padding:4px 8px 10px}',
'.task-picker-result{display:block;width:100%;text-align:left;background:transparent;color:inherit;border:1px solid transparent;border-radius:6px;padding:9px 10px;cursor:pointer}',
'.task-picker-result.selected,.task-picker-result:hover:not(:disabled){background:#2a405b;border-color:#5c7794}',
'.task-picker-result:disabled{opacity:.43;cursor:not-allowed}',
'.task-picker-primary{display:flex;gap:9px;align-items:center;font-size:13px;min-width:0}',
'.task-picker-name{flex:1;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
'.task-picker-label{font-size:10px;flex:none;border:1px solid #57708a;border-radius:4px;padding:1px 5px;opacity:.87}',
'.task-picker-secondary{margin-top:3px;font-size:11px;opacity:.67;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}',
'.task-picker-empty{font-size:12px;padding:16px 10px;opacity:.76}',
'.task-picker-foot{display:flex;gap:15px;padding:9px 13px;font-size:11px;opacity:.66;border-top:1px solid #334155}',
'html[data-taskmenu-theme="light"] .task-picker-dialog{background:#fff;color:#1a2738;border-color:#c6d2df;box-shadow:0 18px 60px #0004}',
'html[data-taskmenu-theme="light"] .task-picker-top,html[data-taskmenu-theme="light"] .task-picker-foot{border-color:#d6dfe8}',
'html[data-taskmenu-theme="light"] .task-picker-input{background:#f8fbff;color:#1a2738;border-color:#8aa8c3}',
'html[data-taskmenu-theme="light"] .task-picker-result.selected,html[data-taskmenu-theme="light"] .task-picker-result:hover:not(:disabled){background:#e3eefb;border-color:#91b1d3}'
].join('\n');
document.head.append(css);

const backdrop=document.createElement('div');backdrop.className='task-picker-backdrop';
const dialog=document.createElement('section');dialog.className='task-picker-dialog';
dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label','Run Task');
const top=document.createElement('div');top.className='task-picker-top';
const title=document.createElement('span');title.className='task-picker-title';
const closeButton=document.createElement('button');closeButton.className='task-picker-close';closeButton.type='button';closeButton.textContent='×';closeButton.title='Close task picker';
top.append(title,closeButton);
const input=document.createElement('input');input.type='search';input.className='task-picker-input';
input.autocomplete='off';input.spellcheck=false;input.setAttribute('aria-label','Filter tasks');
const count=document.createElement('div');count.className='task-picker-count';
const results=document.createElement('div');results.className='task-picker-results';
results.setAttribute('role','listbox');results.setAttribute('aria-label','Available tasks');
const footer=document.createElement('div');footer.className='task-picker-foot';footer.textContent='↑ ↓ Select  ·  Enter Run  ·  Esc Close';
dialog.append(top,input,count,results,footer);backdrop.append(dialog);document.body.append(backdrop);
let mode='all',items=[],selected=-1,previousFocus=null;

function taskName(task){return String(task?.menu_label||task?.label||('Task '+task?.id));}
function taskGroup(task){return Array.isArray(task?.group)?task.group.map(String):[];}
function taskKind(task){
 const rawGroup=task?.raw?.group;
 const kind=typeof rawGroup==='string'?rawGroup:rawGroup&&typeof rawGroup==='object'?rawGroup.kind:'';
 const tags=[kind,...taskGroup(task)].map(value=>String(value||'').trim().toLowerCase());
 if(tags.includes('build'))return 'build';
 if(tags.includes('test')||tags.includes('tests'))return 'test';
 return '';
}
function isDefault(task){
 const group=task?.raw?.group;
 return Boolean(group&&typeof group==='object'&&group.isDefault===true);
}
function normalize(text){
 return String(text||'').normalize('NFD').replace(/[\u0300-\u036f]/g,'').toLowerCase();
}
function taskSearchText(task){
 return normalize([taskName(task),task.label,task.detail,taskGroup(task).join(' '),
  task.workspace_root_name,task.type].filter(Boolean).join(' '));
}
function storageKey(){
 return 'taskdeck:task-picker:recent:'+String(app.taskData?.workspace||'workspace');
}
function recentIDs(){
 try{const values=JSON.parse(localStorage.getItem(storageKey())||'[]');
  return Array.isArray(values)?values.filter(id=>Number.isInteger(id)).slice(0,12):[];
 }catch{return [];}
}
function remember(task){
 const ids=[Number(task.id),...recentIDs().filter(id=>id!==Number(task.id))].slice(0,12);
 try{localStorage.setItem(storageKey(),JSON.stringify(ids));}catch{}
}
function taskList(which=mode,query=''){
 const tokens=normalize(query).trim().split(/\s+/).filter(Boolean);
 const recent=recentIDs();
 return (Array.isArray(app.taskData?.tasks)?app.taskData.tasks:[])
  .map((task,index)=>({task,index,kind:taskKind(task)}))
  .filter(item=>(which==='all'||item.kind===which)&&tokens.every(word=>taskSearchText(item.task).includes(word)))
  .map(item=>{
   const name=normalize(taskName(item.task)),search=normalize(query).trim();
   let score=isDefault(item.task)?120:0;
   const r=recent.indexOf(item.task.id);
   if(r>=0)score+=80-r;
   if(search){if(name===search)score+=500;else if(name.startsWith(search))score+=280;
    else if(name.includes(search))score+=160;}
   return {...item,score,recent:r>=0};
  })
  .sort((a,b)=>b.score-a.score||a.index-b.index);
}
function runnable(){
 return Boolean(app.hasPermission?.('tasks.run'))&&!app.browserLeaseLost;
}
function render(){
 items=taskList(mode,input.value);
 selected=items.length&&runnable()?0:-1;
 title.textContent=mode==='build'?'Tasks: Run Build Task':mode==='test'?'Tasks: Run Test Task':'Tasks: Run Task';
 dialog.setAttribute('aria-label',title.textContent);
 input.placeholder=mode==='build'?'Filter build tasks by name or group…':
  mode==='test'?'Filter test tasks by name or group…':'Type to filter tasks by name or group…';
 count.textContent=items.length+' task'+(items.length===1?'':'s')+
  (!runnable()?' · Task execution unavailable for this account':'');
 results.replaceChildren();
 if(!items.length){
  const empty=document.createElement('div');empty.className='task-picker-empty';
  empty.textContent=mode!=='all'&&!input.value.trim()?'No '+mode+' tasks are configured in tasks.json.':
   'No matching tasks.';results.append(empty);return;
 }
 items.forEach((item,index)=>{
  const task=item.task;
  const button=document.createElement('button');button.type='button';
  button.className='task-picker-result'+(index===selected?' selected':'');
  button.setAttribute('role','option');button.setAttribute('aria-selected',String(index===selected));
  button.disabled=!runnable();button.title=[task.label,task.detail].filter(Boolean).join('\n');
  const primary=document.createElement('div');primary.className='task-picker-primary';
  const name=document.createElement('span');name.className='task-picker-name';name.textContent=taskName(task);
  primary.append(name);
  if(isDefault(task)){
   const badge=document.createElement('span');badge.className='task-picker-label';badge.textContent='Default';primary.append(badge);
  }else if(item.recent){
   const badge=document.createElement('span');badge.className='task-picker-label';badge.textContent='Recent';primary.append(badge);
  }
  const secondary=document.createElement('div');secondary.className='task-picker-secondary';
  secondary.textContent=[task.workspace_root_name,taskGroup(task).join(' / '),task.detail].filter(Boolean).join(' · ')||task.label||'';
  button.append(primary,secondary);
  button.onclick=()=>run(task);
  button.onmousemove=()=>{if(selected!==index)setSelection(index,false);};
  results.append(button);
 });
}
function setSelection(index,scroll=true){
 if(!runnable()||!items.length)return;
 selected=Math.max(0,Math.min(items.length-1,index));
 [...results.querySelectorAll('.task-picker-result')].forEach((row,i)=>{
  row.classList.toggle('selected',i===selected);
  row.setAttribute('aria-selected',String(i===selected));
 });
 if(scroll)results.querySelector('.task-picker-result.selected')?.scrollIntoView({block:'nearest'});
}
function close(){
 if(!backdrop.classList.contains('visible'))return;
 backdrop.classList.remove('visible');
 const focus=previousFocus;previousFocus=null;
 if(focus?.isConnected&&typeof focus.focus==='function')focus.focus();
}
function open(which='all'){
 if(!['all','build','test'].includes(which))which='all';
 if(!backdrop.classList.contains('visible'))previousFocus=document.activeElement;
 mode=which;input.value='';
 backdrop.classList.add('visible');render();input.focus();
}
function run(task){
 if(!runnable()||!task)return;
 // Use the same task launcher as sidebar / Command Palette, including task inputs.
 close();
 Promise.resolve().then(()=>app.startTask(task)).then(()=>remember(task)).catch(app.showError);
}
function rerunLast(){
 const last=recentIDs()[0],task=app.taskData?.tasks?.find(task=>task.id===last);
 if(task&&runnable())run(task);
 else open('all');
}
input.addEventListener('input',render);
input.addEventListener('keydown',event=>{
 if(event.key==='ArrowDown'||event.key==='ArrowUp'){
  event.preventDefault();event.stopPropagation();
  const step=event.key==='ArrowDown'?1:-1;
  setSelection(selected<0?0:Math.max(0,Math.min(items.length-1,selected+step)));
 }else if(event.key==='Enter'){
  event.preventDefault();event.stopPropagation();
  if(selected>=0)run(items[selected]?.task);
 }else if(event.key==='Escape'){
  event.preventDefault();event.stopPropagation();close();
 }
});
closeButton.onclick=close;
backdrop.onmousedown=event=>{if(event.target===backdrop)close();};
function shortcut(event){
 if(event.repeat||event.isComposing||event.getModifierState?.('AltGraph'))return;
 const ctrl=event.ctrlKey||event.metaKey;
 if(!ctrl||!['b','r','t'].includes(String(event.key||'').toLowerCase()))return;
 let which='';
 const key=String(event.key||'').toLowerCase();
 if(event.shiftKey&&!event.altKey&&key==='b')which='build';     // VS Code Run Build Task
 else if(!event.shiftKey&&event.altKey&&key==='r')which='all'; // TaskDeck Run Task
 else if(!event.shiftKey&&event.altKey&&key==='t')which='test'; // TaskDeck Run Test Task
 if(!which)return;
 event.preventDefault();event.stopPropagation();open(which);
}
window.addEventListener('keydown',shortcut,true);
window.addEventListener('taskmenu:tasks',()=>{if(backdrop.classList.contains('visible'))render();});
const launcher=document.createElement('button');launcher.id='open-task-picker';
launcher.type='button';launcher.textContent='Tasks…';launcher.title='Run Task (Ctrl+Alt+R) · Build Task (Ctrl+Shift+B)';
launcher.onclick=()=>open('all');
const header=document.querySelector('header'),reload=document.querySelector('#reload');
if(header&&reload)header.insertBefore(launcher,reload);
globalThis.TaskMenuTaskPicker={open,close,taskKind,taskList,rerunLast,get visible(){return backdrop.classList.contains('visible');}};
