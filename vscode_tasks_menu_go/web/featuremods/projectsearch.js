const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for project content search');

const style=document.createElement('style');
style.textContent=`
.project-search-backdrop{display:none;position:fixed;inset:0;z-index:2600;background:rgba(0,0,0,.48);align-items:flex-start;justify-content:center;padding-top:min(10vh,90px)}
.project-search-backdrop.visible{display:flex}
.project-search-dialog{width:min(920px,94vw);max-height:78vh;display:flex;flex-direction:column;border:1px solid #4a5362;border-radius:9px;background:#15191f;box-shadow:0 18px 55px rgba(0,0,0,.5);overflow:hidden}
.project-search-input{width:100%;border:0;border-bottom:1px solid #343b46;border-radius:0;background:#0d1117;color:inherit;padding:12px 14px;font:14px ui-monospace,monospace;outline:none}
.project-search-results{overflow:auto;min-height:46px;max-height:64vh;padding:4px}
.project-search-result{display:block;width:100%;text-align:left;border:1px solid transparent;background:transparent;padding:7px 9px}
.project-search-result.selected,.project-search-result:hover{background:#293241;border-color:#42536a}
.project-search-location{display:block;font:11px ui-monospace,monospace;opacity:.7;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.project-search-preview{display:block;margin-top:3px;font:12px ui-monospace,monospace;white-space:pre;overflow:hidden;text-overflow:ellipsis}
.project-search-empty{padding:12px 10px;font-size:12px;opacity:.62}
html[data-taskmenu-theme="light"] .project-search-dialog{background:#fff;border-color:#b9c0c8;box-shadow:0 18px 55px rgba(0,0,0,.18)}
html[data-taskmenu-theme="light"] .project-search-input{background:#f7f8fa;border-color:#d5d9df}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='project-search-backdrop';
const dialog=document.createElement('div');dialog.className='project-search-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Search in project');
const input=document.createElement('input');input.className='project-search-input';input.type='search';input.autocomplete='off';input.spellcheck=false;input.placeholder='Search in Files — literal text';
const results=document.createElement('div');results.className='project-search-results';
dialog.append(input,results);backdrop.append(dialog);document.body.append(backdrop);

let items=[];
let selected=0;
let timer=null;
let controller=null;
let seq=0;

function setEmpty(message){
  results.replaceChildren();
  const empty=document.createElement('div');empty.className='project-search-empty';empty.textContent=message;results.append(empty);
}
function render(){
  results.replaceChildren();
  if(!items.length){setEmpty(input.value.trim()?'No matches found':'Type text to search project files');return;}
  items.forEach((item,index)=>{
    const button=document.createElement('button');button.type='button';button.className='project-search-result'+(index===selected?' selected':'');
    const location=document.createElement('span');location.className='project-search-location';location.textContent=item.path+':'+item.line+':'+item.column;location.title=location.textContent;
    const preview=document.createElement('span');preview.className='project-search-preview';preview.textContent=item.preview||'';
    button.append(location,preview);button.onclick=()=>choose(index);button.onmousemove=()=>select(index);results.append(button);
  });
}
function select(index){
  if(!items.length)return;
  selected=(index+items.length)%items.length;
  [...results.querySelectorAll('.project-search-result')].forEach((node,i)=>node.classList.toggle('selected',i===selected));
  results.querySelector('.project-search-result.selected')?.scrollIntoView({block:'nearest'});
}
async function choose(index=selected){
  const item=items[index];if(!item)return;
  close();
  const editor=globalThis.TaskMenuEditor;
  if(!editor?.openFile)throw new Error('Project editor unavailable');
  const view=await editor.openFile(item.path);
  if(!view?.cm)return;
  const doc=view.cm.state.doc;
  const lineNumber=Math.max(1,Math.min(doc.lines,Number(item.line)||1));
  const line=doc.line(lineNumber);
  const column=Math.max(1,Number(item.column)||1);
  const anchor=Math.min(line.to,line.from+column-1);
  view.cm.dispatch({selection:{anchor},scrollIntoView:true});
  view.cm.focus();
}
function cancelPending(){
  clearTimeout(timer);timer=null;
  if(controller){controller.abort();controller=null;}
}
function close(){
  cancelPending();seq++;backdrop.classList.remove('visible');items=[];selected=0;results.replaceChildren();
}
function open(){
  backdrop.classList.add('visible');input.value='';items=[];selected=0;setEmpty('Type text to search project files');requestAnimationFrame(()=>input.focus());
}
async function search(query,currentSeq){
  if(controller)controller.abort();
  controller=new AbortController();
  try{
    const data=await app.jsonFetch('/api/project/content/search?q='+encodeURIComponent(query)+'&limit=100',{signal:controller.signal});
    if(currentSeq!==seq||!backdrop.classList.contains('visible'))return;
    items=Array.isArray(data.results)?data.results.slice(0,100):[];selected=0;render();
  }catch(error){
    if(error?.name==='AbortError')return;
    if(currentSeq!==seq)return;
    setEmpty('Search failed');
    console.warn('Project content search failed',error);
  }
}
function schedule(){
  const query=input.value.trim();cancelPending();const currentSeq=++seq;
  if(!query){items=[];selected=0;setEmpty('Type text to search project files');return;}
  timer=setTimeout(()=>search(query,currentSeq),120);
}

input.addEventListener('input',schedule);
input.addEventListener('keydown',event=>{
  if(event.key==='ArrowDown'){event.preventDefault();select(selected+1);}
  else if(event.key==='ArrowUp'){event.preventDefault();select(selected-1);}
  else if(event.key==='Enter'){event.preventDefault();choose().catch(app.showError);}
  else if(event.key==='Escape'){event.preventDefault();close();}
});
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
function globalProjectSearchShortcut(event){
  const primary=event.ctrlKey||event.metaKey;
  if(!primary||!event.shiftKey||event.altKey||event.key.toLowerCase()!=='f')return;
  event.preventDefault();
  event.stopPropagation();
  if(backdrop.classList.contains('visible'))input.focus();else open();
}
window.addEventListener('keydown',globalProjectSearchShortcut,true);
document.addEventListener('keydown',event=>{
  if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();
});

globalThis.TaskMenuProjectSearch={open,close};
