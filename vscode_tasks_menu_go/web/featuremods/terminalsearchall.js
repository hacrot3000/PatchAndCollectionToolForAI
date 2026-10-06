const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal scrollback search');

const maxScannedLines=50000;
const maxResults=300;

const style=document.createElement('style');
style.textContent=`
.terminal-search-all-backdrop{display:none;position:fixed;inset:0;z-index:2850;background:rgba(0,0,0,.5);align-items:flex-start;justify-content:center;padding-top:min(7vh,60px)}
.terminal-search-all-backdrop.visible{display:flex}
.terminal-search-all-dialog{width:min(980px,96vw);max-height:86vh;display:flex;flex-direction:column;border:1px solid #4a5362;border-radius:9px;background:#15191f;box-shadow:0 18px 55px rgba(0,0,0,.5);overflow:hidden}
.terminal-search-all-top{display:grid;grid-template-columns:minmax(220px,1fr) auto auto;gap:7px;padding:9px;border-bottom:1px solid #343b46;align-items:center}
.terminal-search-all-input{width:100%;border:1px solid #343b46;border-radius:5px;background:#0d1117;color:inherit;padding:7px 9px;font:12px ui-monospace,monospace;outline:none}
.terminal-search-all-case{display:flex;align-items:center;gap:5px;font-size:11px;white-space:nowrap}
.terminal-search-all-status{padding:6px 10px;border-bottom:1px solid #30343b;font-size:11px;opacity:.72}
.terminal-search-all-results{overflow:auto;min-height:58px;max-height:67vh;padding:4px}
.terminal-search-all-row{display:grid;grid-template-columns:minmax(110px,.55fr) 72px minmax(220px,1.7fr);gap:8px;align-items:center;width:100%;text-align:left;border:0;border-radius:5px;background:transparent;padding:7px 8px}
.terminal-search-all-row.selected,.terminal-search-all-row:hover{background:#293241}
.terminal-search-all-terminal{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:11px ui-monospace,monospace;font-weight:700}
.terminal-search-all-line{font:10px ui-monospace,monospace;opacity:.58;text-align:right}
.terminal-search-all-preview{overflow:hidden;text-overflow:ellipsis;white-space:pre;font:11px ui-monospace,monospace;opacity:.86}
.terminal-search-all-empty{padding:14px 10px;font-size:12px;opacity:.62}
html[data-taskmenu-theme="light"] .terminal-search-all-dialog{background:#fff;border-color:#b9c0c8;box-shadow:0 18px 55px rgba(0,0,0,.18)}
html[data-taskmenu-theme="light"] .terminal-search-all-input{background:#f7f8fa;border-color:#d5d9df}
html[data-taskmenu-theme="light"] .terminal-search-all-row.selected,html[data-taskmenu-theme="light"] .terminal-search-all-row:hover{background:#e8edf3}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='terminal-search-all-backdrop';
const dialog=document.createElement('div');dialog.className='terminal-search-all-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Search all terminal scrollback');
const top=document.createElement('div');top.className='terminal-search-all-top';
const input=document.createElement('input');input.type='search';input.className='terminal-search-all-input';input.autocomplete='off';input.spellcheck=false;input.placeholder='Search all terminal scrollback';
const caseLabel=document.createElement('label');caseLabel.className='terminal-search-all-case';
const caseInput=document.createElement('input');caseInput.type='checkbox';caseLabel.append(caseInput,document.createTextNode('Match case'));
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='Close';
top.append(input,caseLabel,closeButton);
const status=document.createElement('div');status.className='terminal-search-all-status';status.textContent='Type text to search visible terminal buffers';
const results=document.createElement('div');results.className='terminal-search-all-results';
dialog.append(top,status,results);backdrop.append(dialog);document.body.append(backdrop);

let items=[];
let selected=0;
let timer=null;
let generation=0;

function terminalView(view){
  const meta=view?.meta;if(!view?.term||!meta||view.closed)return false;
  const kind=String(meta.kind||'').trim().toLowerCase();
  if(kind==='terminal')return true;
  return !kind&&Number(meta.task_id||0)===0&&String(meta.label||'').toLowerCase().includes('terminal');
}
function terminalLabel(view){
  return String(view?.meta?.title||view?.meta?.label||view?.meta?.id||'Terminal');
}
function visibleTerminalViews(){
  return [...app.views.values()].filter(terminalView);
}
function setEmpty(message){
  results.replaceChildren();
  const empty=document.createElement('div');empty.className='terminal-search-all-empty';empty.textContent=message;results.append(empty);
}
function select(index){
  if(!items.length)return;
  selected=(index+items.length)%items.length;
  [...results.querySelectorAll('.terminal-search-all-row')].forEach((node,i)=>node.classList.toggle('selected',i===selected));
  results.querySelector('.terminal-search-all-row.selected')?.scrollIntoView({block:'nearest'});
}
function choose(index=selected){
  const item=items[index];if(!item)return;
  const view=app.views.get(item.sessionID);if(!terminalView(view))return;
  close();
  app.activateView(item.sessionID,{force:true});
  setTimeout(()=>{
    try{
      view.term.clearSelection();
      view.term.select(item.column,item.row,item.length);
      view.term.scrollToLine(item.row);
      view.term.focus();
    }catch(error){console.warn('Cannot jump to terminal scrollback match',error);}
  },0);
}
function render(){
  results.replaceChildren();
  if(!items.length){setEmpty(input.value.trim()?'No matches found':'Type text to search visible terminal buffers');return;}
  items.forEach((item,index)=>{
    const row=document.createElement('button');row.type='button';row.className='terminal-search-all-row'+(index===selected?' selected':'');row.title=item.preview;
    const terminal=document.createElement('span');terminal.className='terminal-search-all-terminal';terminal.textContent=item.label;terminal.title=item.label;
    const line=document.createElement('span');line.className='terminal-search-all-line';line.textContent='L'+(item.row+1);
    const preview=document.createElement('span');preview.className='terminal-search-all-preview';preview.textContent=item.preview;
    row.append(terminal,line,preview);row.onmousemove=()=>select(index);row.onclick=()=>choose(index);results.append(row);
  });
}
function scan(query,currentGeneration){
  const matchCase=caseInput.checked;
  const needle=matchCase?query:query.toLowerCase();
  const found=[];
  let scannedLines=0,scannedTerminals=0,truncated=false;
  for(const view of visibleTerminalViews()){
    if(currentGeneration!==generation)return;
    const buffer=view.term?.buffer?.active;if(!buffer)continue;
    scannedTerminals++;
    for(let row=0;row<buffer.length;row++){
      if(scannedLines>=maxScannedLines||found.length>=maxResults){truncated=true;break;}
      scannedLines++;
      const line=buffer.getLine(row);if(!line)continue;
      const text=line.translateToString(true);
      const comparable=matchCase?text:text.toLowerCase();
      const column=comparable.indexOf(needle);if(column<0)continue;
      const clean=text.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g,'');
      found.push({
        sessionID:String(view.meta.id),label:terminalLabel(view),row,column,length:query.length,
        preview:clean.length>320?clean.slice(0,317)+'…':clean
      });
    }
    if(truncated)break;
  }
  if(currentGeneration!==generation)return;
  items=found;selected=0;render();
  status.textContent=found.length+' match(es) · '+scannedTerminals+' terminal(s) · '+scannedLines+' line(s)'+(truncated?' · bounded scan truncated':'');
}
function schedule(){
  clearTimeout(timer);timer=null;
  const query=String(input.value||'').trim();const currentGeneration=++generation;
  if(!query){items=[];selected=0;render();status.textContent='Type text to search visible terminal buffers';return;}
  status.textContent='Searching terminal scrollback…';
  timer=setTimeout(()=>{timer=null;scan(query,currentGeneration);},80);
}
function open(options={}){
  backdrop.classList.add('visible');
  if(typeof options.query==='string')input.value=options.query;
  items=[];selected=0;render();status.textContent=input.value.trim()?'Searching terminal scrollback…':'Type text to search visible terminal buffers';
  requestAnimationFrame(()=>{input.focus();input.select();if(input.value.trim())schedule();});
}
function close(){
  clearTimeout(timer);timer=null;generation++;backdrop.classList.remove('visible');items=[];selected=0;
}
function decorate(view){
  if(!terminalView(view))return null;
  const head=view.pane?.querySelector('.pane-head');if(!head)return null;
  let button=head.querySelector('.console-search-all-btn');
  if(button)return button;
  button=document.createElement('button');button.type='button';button.className='console-search-all-btn';
  button.textContent='Search all terminals…';button.title='Search scrollback across all visible terminals';
  button.onclick=()=>open();
  const anchor=head.querySelector('.console-search-btn')||head.querySelector('.copy-console');
  if(anchor)anchor.before(button);else head.append(button);
  return button;
}

input.addEventListener('input',schedule);
caseInput.addEventListener('change',schedule);
input.addEventListener('keydown',event=>{
  if(event.key==='ArrowDown'){event.preventDefault();select(selected+1);}
  else if(event.key==='ArrowUp'){event.preventDefault();select(selected-1);}
  else if(event.key==='Enter'){event.preventDefault();choose();}
  else if(event.key==='Escape'){event.preventDefault();close();}
});
closeButton.onclick=close;backdrop.onmousedown=event=>{if(event.target===backdrop)close();};
window.addEventListener('taskmenu:session',event=>{const view=event.detail?.view;if(view)setTimeout(()=>decorate(view),0);});
for(const view of app.views.values())decorate(view);

globalThis.TaskMenuTerminalSearchAll={open,close,scan,decorate,get visible(){return backdrop.classList.contains('visible');}};
