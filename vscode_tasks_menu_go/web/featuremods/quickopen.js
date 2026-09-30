const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Quick Open');

const style=document.createElement('style');
style.textContent=`
.quick-open-backdrop{display:none;position:fixed;inset:0;z-index:2500;background:rgba(0,0,0,.48);align-items:flex-start;justify-content:center;padding-top:min(12vh,110px)}
.quick-open-backdrop.visible{display:flex}
.quick-open-dialog{width:min(760px,92vw);max-height:72vh;display:flex;flex-direction:column;border:1px solid #4a5362;border-radius:9px;background:#15191f;box-shadow:0 18px 55px rgba(0,0,0,.5);overflow:hidden}
.quick-open-input{width:100%;border:0;border-bottom:1px solid #343b46;border-radius:0;background:#0d1117;color:inherit;padding:12px 14px;font:14px ui-monospace,monospace;outline:none}
.quick-open-results{overflow:auto;min-height:42px;max-height:56vh;padding:4px}
.quick-open-result{display:block;width:100%;text-align:left;border:1px solid transparent;background:transparent;padding:7px 9px}
.quick-open-result.selected,.quick-open-result:hover{background:#293241;border-color:#42536a}
.quick-open-result-name{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.quick-open-match{font-weight:800;background:rgba(111,168,255,.2);border-radius:2px}
.quick-open-result-path{display:block;margin-top:2px;font-size:11px;opacity:.58;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.quick-open-empty{padding:12px 10px;font-size:12px;opacity:.62}
html[data-taskmenu-theme="light"] .quick-open-dialog{background:#fff;border-color:#b9c0c8;box-shadow:0 18px 55px rgba(0,0,0,.18)}
html[data-taskmenu-theme="light"] .quick-open-input{background:#f7f8fa;border-color:#d5d9df}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='quick-open-backdrop';
const dialog=document.createElement('div');dialog.className='quick-open-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Quick Open');
const input=document.createElement('input');input.className='quick-open-input';input.type='search';input.autocomplete='off';input.spellcheck=false;input.placeholder='Quick Open — type a file name or path';
const results=document.createElement('div');results.className='quick-open-results';
dialog.append(input,results);backdrop.append(dialog);document.body.append(backdrop);

let items=[];
let selected=0;
let timer=null;
let refreshTimer=null;
let controller=null;
let seq=0;

function setEmpty(message){
  results.replaceChildren();
  const empty=document.createElement('div');empty.className='quick-open-empty';empty.textContent=message;results.append(empty);
}
function fuzzyNameIndexes(name,query){
  const hay=String(name||'').toLowerCase();
  const needle=String(query||'').toLowerCase().replace(/[*?%\s]+/g,'');
  if(!needle)return [];
  const indexes=[];let from=0;
  for(const ch of needle){
    const found=hay.indexOf(ch,from);
    if(found<0)return [];
    indexes.push(found);from=found+1;
  }
  return indexes;
}
function renderHighlightedName(node,name,query){
  const indexes=fuzzyNameIndexes(name,query);
  if(!indexes.length){node.textContent=name;return;}
  const marked=new Set(indexes);let start=0;
  for(let i=0;i<name.length;){
    const yes=marked.has(i);let end=i+1;
    while(end<name.length&&marked.has(end)===yes)end++;
    const part=document.createElement(yes?'span':'span');
    if(yes)part.className='quick-open-match';
    part.textContent=name.slice(i,end);node.append(part);i=end;
  }
}
function render(){
  results.replaceChildren();
  if(!items.length){setEmpty(input.value.trim()?'No files found':'Type to search project files');return;}
  const query=input.value.trim();
  items.forEach((item,index)=>{
    const button=document.createElement('button');button.className='quick-open-result'+(index===selected?' selected':'');button.type='button';button.dataset.path=item.path;
    const name=document.createElement('span');name.className='quick-open-result-name';
    renderHighlightedName(name,item.name||item.path.split('/').pop()||item.path,query);
    const full=document.createElement('span');full.className='quick-open-result-path';full.textContent=item.path;full.title=item.path;
    button.append(name,full);button.onclick=()=>choose(index);button.onmousemove=()=>select(index);results.append(button);
  });
}
function select(index){
  if(!items.length)return;
  selected=(index+items.length)%items.length;
  [...results.querySelectorAll('.quick-open-result')].forEach((node,i)=>node.classList.toggle('selected',i===selected));
  results.querySelector('.quick-open-result.selected')?.scrollIntoView({block:'nearest'});
}
function choose(index=selected){
  const item=items[index];if(!item)return;
  close();
  window.dispatchEvent(new CustomEvent('taskmenu:project-file-open-request',{detail:{path:item.path,source:'quick-open'}}));
}
function cancelPending(){
  clearTimeout(timer);timer=null;
  clearTimeout(refreshTimer);refreshTimer=null;
  if(controller){controller.abort();controller=null;}
}
function close(){
  cancelPending();seq++;backdrop.classList.remove('visible');items=[];selected=0;results.replaceChildren();
}
function open(){
  backdrop.classList.add('visible');
  input.value='';
  items=[];selected=0;setEmpty('Type to search project files');
  requestAnimationFrame(()=>input.focus());
}
function hasExactFilename(query){
  const wanted=String(query||'').trim().toLowerCase();
  if(!wanted||/[?*%]/.test(wanted))return false;
  return items.some(item=>String(item.name||'').toLowerCase()===wanted);
}
async function search(query,currentSeq,refresh=false){
  if(controller)controller.abort();
  controller=new AbortController();
  try{
    const suffix=refresh?'&refresh=1':'';
    const data=await app.jsonFetch('/api/project/files/search?q='+encodeURIComponent(query)+'&limit=50'+suffix,{signal:controller.signal});
    if(currentSeq!==seq||!backdrop.classList.contains('visible'))return;
    items=Array.isArray(data.results)?data.results.slice(0,50):[];selected=0;render();
    const shouldRefresh=!refresh&&!/[?*%]/.test(query)&&!hasExactFilename(query)&&(items.length===0||query.includes('.')||query.includes('/'));
    if(shouldRefresh){
      refreshTimer=setTimeout(()=>{
        if(currentSeq===seq&&input.value.trim()===query&&backdrop.classList.contains('visible'))search(query,currentSeq,true);
      },450);
    }
  }catch(error){
    if(error?.name==='AbortError')return;
    if(currentSeq!==seq)return;
    setEmpty('Search failed');
    console.warn('Quick Open search failed',error);
  }
}
function schedule(){
  const query=input.value.trim();cancelPending();const currentSeq=++seq;
  if(!query){items=[];selected=0;setEmpty('Type to search project files');return;}
  timer=setTimeout(()=>search(query,currentSeq),80);
}

input.addEventListener('input',schedule);
input.addEventListener('keydown',event=>{
  if(event.key==='ArrowDown'){event.preventDefault();select(selected+1);}
  else if(event.key==='ArrowUp'){event.preventDefault();select(selected-1);}
  else if(event.key==='Enter'){event.preventDefault();choose();}
  else if(event.key==='Escape'){event.preventDefault();close();}
});
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
function globalQuickOpenShortcut(event){
  const primary=event.ctrlKey||event.metaKey;
  if(!primary||event.shiftKey||event.altKey||event.key.toLowerCase()!=='p')return;
  event.preventDefault();
  event.stopPropagation();
  if(backdrop.classList.contains('visible'))input.focus();else open();
}
window.addEventListener('keydown',globalQuickOpenShortcut,true);
document.addEventListener('keydown',event=>{
  if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();
});

globalThis.TaskMenuQuickOpen={open,close};
