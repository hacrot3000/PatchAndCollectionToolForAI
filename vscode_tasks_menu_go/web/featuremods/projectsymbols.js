const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for project symbol search');

const style=document.createElement('style');
style.textContent=`
.project-symbol-backdrop{display:none;position:fixed;inset:0;z-index:2700;background:rgba(0,0,0,.5);align-items:flex-start;justify-content:center;padding-top:min(8vh,68px)}
.project-symbol-backdrop.visible{display:flex}
.project-symbol-dialog{width:min(820px,96vw);max-height:84vh;display:flex;flex-direction:column;border:1px solid #4a5362;border-radius:9px;background:#15191f;box-shadow:0 18px 55px rgba(0,0,0,.5);overflow:hidden}
.project-symbol-top{display:flex;gap:7px;padding:9px;border-bottom:1px solid #343b46}
.project-symbol-input{flex:1;min-width:0;border:1px solid #343b46;border-radius:5px;background:#0d1117;color:inherit;padding:7px 9px;font:12px ui-monospace,monospace;outline:none}
.project-symbol-status{padding:6px 10px;border-bottom:1px solid #30343b;font-size:11px;opacity:.72}
.project-symbol-results{overflow:auto;min-height:56px;max-height:65vh;padding:4px}
.project-symbol-row{display:grid;grid-template-columns:78px minmax(120px,.8fr) minmax(180px,1.4fr) 64px;gap:8px;align-items:center;width:100%;text-align:left;border:0;border-radius:5px;background:transparent;padding:7px 8px}
.project-symbol-row.selected,.project-symbol-row:hover{background:#293241}
.project-symbol-kind{font-size:9px;text-transform:uppercase;letter-spacing:.04em;opacity:.58}
.project-symbol-name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:12px ui-monospace,monospace;font-weight:700}
.project-symbol-path{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:10px ui-monospace,monospace;opacity:.68}
.project-symbol-line{text-align:right;font:10px ui-monospace,monospace;opacity:.58}
.project-symbol-empty{padding:14px 10px;font-size:12px;opacity:.62}
html[data-taskmenu-theme="light"] .project-symbol-dialog{background:#fff;border-color:#b9c0c8;box-shadow:0 18px 55px rgba(0,0,0,.18)}
html[data-taskmenu-theme="light"] .project-symbol-input{background:#f7f8fa;border-color:#d5d9df}
html[data-taskmenu-theme="light"] .project-symbol-row.selected,html[data-taskmenu-theme="light"] .project-symbol-row:hover{background:#e8edf3}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='project-symbol-backdrop';
const dialog=document.createElement('div');dialog.className='project-symbol-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Project symbol search');
const top=document.createElement('div');top.className='project-symbol-top';
const input=document.createElement('input');input.type='search';input.className='project-symbol-input';input.autocomplete='off';input.spellcheck=false;input.placeholder='Search project symbols';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='Close';
top.append(input,closeButton);
const status=document.createElement('div');status.className='project-symbol-status';status.textContent='Type a symbol name';
const results=document.createElement('div');results.className='project-symbol-results';
dialog.append(top,status,results);backdrop.append(dialog);document.body.append(backdrop);

let items=[];
let selected=0;
let timer=null;
let controller=null;
let seq=0;

function canRead(){
  return !app.sharedMode||app.hasPermission?.('files.read')||app.hasPermission?.('project.admin');
}
function setEmpty(message){
  results.replaceChildren();
  const empty=document.createElement('div');empty.className='project-symbol-empty';empty.textContent=message;results.append(empty);
}
function select(index){
  if(!items.length)return;
  selected=(index+items.length)%items.length;
  [...results.querySelectorAll('.project-symbol-row')].forEach((node,i)=>node.classList.toggle('selected',i===selected));
  results.querySelector('.project-symbol-row.selected')?.scrollIntoView({block:'nearest'});
}
async function choose(index=selected){
  const item=items[index];if(!item)return;
  const editor=globalThis.TaskMenuEditor;
  if(!editor?.openFile||!editor?.jumpEditorToLine)throw new Error('Project editor symbol navigation is unavailable');
  const view=await editor.openFile(item.path);
  editor.jumpEditorToLine(view,item.line);
  close();
}
function render(){
  results.replaceChildren();
  if(!items.length){setEmpty(input.value.trim()?'No matching symbols':'Type a symbol name');return;}
  items.forEach((item,index)=>{
    const row=document.createElement('button');row.type='button';row.className='project-symbol-row'+(index===selected?' selected':'');row.title=item.signature||item.path;
    const kind=document.createElement('span');kind.className='project-symbol-kind';kind.textContent=item.kind||'symbol';
    const name=document.createElement('span');name.className='project-symbol-name';name.textContent=item.name||'';
    const path=document.createElement('span');path.className='project-symbol-path';path.textContent=item.path||'';path.title=item.path||'';
    const line=document.createElement('span');line.className='project-symbol-line';line.textContent='L'+(item.line||1);
    row.append(kind,name,path,line);row.onmousemove=()=>select(index);row.onclick=()=>choose(index).catch(app.showError);results.append(row);
  });
}
function cancelPending(){
  clearTimeout(timer);timer=null;
  if(controller){controller.abort();controller=null;}
}
async function runSearch(query,currentSeq){
  if(controller)controller.abort();
  controller=new AbortController();
  try{
    const params=new URLSearchParams({q:query,limit:'100'});
    const data=await app.jsonFetch('/api/project/symbols?'+params.toString(),{signal:controller.signal,cache:'no-store'});
    if(currentSeq!==seq)return;
    items=Array.isArray(data?.results)?data.results:[];
    selected=0;render();
    const scanned=Number(data?.scanned_files)||0,bytes=Number(data?.scanned_bytes)||0;
    status.textContent=items.length+' symbol(s) · '+scanned+' source file(s) · '+formatBytes(bytes)+(data?.truncated?' · bounded scan truncated':'');
  }catch(error){
    if(error?.name==='AbortError')return;
    if(currentSeq!==seq)return;
    items=[];render();status.textContent='Symbol search failed';app.showError(error);
  }finally{
    if(currentSeq===seq)controller=null;
  }
}
function schedule(){
  cancelPending();const query=input.value.trim();const currentSeq=++seq;
  if(!query){items=[];selected=0;render();status.textContent='Type a symbol name';return;}
  status.textContent='Searching symbols…';
  timer=setTimeout(()=>{timer=null;runSearch(query,currentSeq);},120);
}
function open(options={}){
  if(!canRead()){app.showError(new Error('File read permission is required for project symbol search'));return;}
  backdrop.classList.add('visible');
  if(typeof options.query==='string')input.value=options.query;
  items=[];selected=0;render();status.textContent=input.value.trim()?'Searching symbols…':'Type a symbol name';
  requestAnimationFrame(()=>{input.focus();input.select();if(input.value.trim())schedule();});
}
function close(){
  cancelPending();seq++;backdrop.classList.remove('visible');items=[];selected=0;
}

input.addEventListener('input',schedule);
input.addEventListener('keydown',event=>{
  if(event.key==='ArrowDown'){event.preventDefault();select(selected+1);}
  else if(event.key==='ArrowUp'){event.preventDefault();select(selected-1);}
  else if(event.key==='Enter'){event.preventDefault();choose().catch(app.showError);}
  else if(event.key==='Escape'){event.preventDefault();close();}
});
closeButton.onclick=close;
backdrop.onmousedown=event=>{if(event.target===backdrop)close();};

globalThis.TaskMenuProjectSymbols={open,close,get visible(){return backdrop.classList.contains('visible');}};
