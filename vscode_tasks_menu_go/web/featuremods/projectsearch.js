const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for project content search');

const style=document.createElement('style');
style.textContent=`
/* A persistent right dock, never a modal: the editor stays visible and interactive. */
.project-search-panel{display:none;position:fixed;right:0;top:var(--taskmenu-header-height,30px);bottom:0;width:var(--project-search-panel-width,clamp(300px,30vw,440px));min-width:0;z-index:1400;flex-direction:column;border-left:1px solid #4a5362;background:#15191f;box-shadow:-8px 0 22px rgba(0,0,0,.2);overflow:hidden}
.project-search-panel.visible{display:flex}
@media(min-width:950px){
  body.project-search-docked>main>section{width:calc(100% - var(--project-search-panel-width,clamp(300px,30vw,440px)))!important;min-width:0!important;justify-self:start}
}
@media(max-width:949px){.project-search-panel{width:min(440px,92vw);box-shadow:-14px 0 36px rgba(0,0,0,.38)}}
.project-search-panel-title{display:flex;gap:8px;align-items:center;justify-content:space-between;border-bottom:1px solid #343b46;padding:8px 10px;font-size:13px;font-weight:700}
.project-search-panel-title button{padding:3px 8px}
.project-search-panel .project-search-top{min-height:0}
.project-search-top{display:flex;flex-direction:column;gap:7px;padding:9px;border-bottom:1px solid #343b46}
.project-search-fields{display:grid;gap:6px}
.project-search-input,.project-search-replace,.project-search-filter{width:100%;border:1px solid #343b46;border-radius:5px;background:#0d1117;color:inherit;padding:7px 9px;font:12px ui-monospace,monospace;outline:none}
.project-search-query-row,.project-search-replace-row,.project-search-filter-row{display:flex;gap:6px;align-items:center}
.project-search-query-row .project-search-input,.project-search-replace-row .project-search-replace{flex:1}
.project-search-toggle{min-width:31px;padding:6px 8px;font:11px ui-monospace,monospace}.project-search-toggle.active{background:#34445a;border-color:#6f91bb}
.project-search-actions{display:flex;flex-direction:row;gap:6px;flex-wrap:wrap}.project-search-actions button{padding:5px 7px;font-size:11px}
.project-search-status-row{display:flex;align-items:center;gap:4px;border-bottom:1px solid #30343b;padding:4px 8px}
.project-search-status{flex:1;min-width:0;font-size:11px;opacity:.85;overflow:hidden;text-overflow:ellipsis}
.project-search-status-row button{padding:2px 8px;font-size:12px}
.project-search-results{flex:1;overflow:auto;min-height:46px;max-height:none;padding:4px}
.project-search-file{border:1px solid #30343b;border-radius:7px;margin:5px 3px;overflow:hidden}
.project-search-file-head{display:flex;align-items:center;gap:6px;padding:6px 8px;background:#11161d;position:sticky;top:0;z-index:1}
.project-search-file-name{flex:1;min-width:0;font:11px ui-monospace,monospace;font-weight:700;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.project-search-file-count{font-size:10px;opacity:.6}.project-search-file-head button{padding:3px 6px;font-size:10px}
.project-search-result{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:7px;align-items:center;width:100%;text-align:left;border:0;border-top:1px solid #252c35;background:transparent;padding:6px 8px}
.project-search-result.selected,.project-search-result:hover{background:#293241}
.project-search-result:focus-within{outline:1px solid #5799de;outline-offset:-1px}
.project-search-hit{background:#ffe36b;color:#342300;font-weight:750;border-radius:2px;box-shadow:inset 0 0 0 1px #bc8300}
.project-search-result.selected .project-search-hit{background:#24c5f3;color:#08202d;outline:1px solid #eefaff;outline-offset:1px}
.project-search-result-main{min-width:0;width:100%;text-align:left;background:transparent;border:0;padding:0}.project-search-location{display:block;font:10px ui-monospace,monospace;opacity:.62;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.project-search-preview{display:block;margin-top:2px;font:11px ui-monospace,monospace;white-space:pre;overflow:hidden;text-overflow:ellipsis}
.project-search-result button{padding:3px 6px;font-size:10px}
.project-search-empty{padding:12px 10px;font-size:12px;opacity:.62}
.project-replace-preview{display:none;flex:1;flex-direction:column;min-height:0;max-height:none}.project-replace-preview.visible{display:flex}
.project-replace-preview-head{display:flex;align-items:center;gap:7px;padding:7px 10px;border-bottom:1px solid #30343b}
.project-replace-preview-title{flex:1;font-size:12px;font-weight:700}.project-replace-preview-head button{padding:4px 7px}
.project-replace-preview-files{overflow:auto;padding:5px}
.project-replace-preview-file{border:1px solid #30343b;border-radius:7px;margin:5px 0;overflow:hidden}
.project-replace-preview-label{padding:5px 8px;background:#11161d;font:10px ui-monospace,monospace;font-weight:700}
.project-replace-preview-diff{margin:0;padding:8px;white-space:pre-wrap;word-break:break-word;font:11px/1.4 ui-monospace,monospace;background:#090d12;max-height:260px;overflow:auto}
html[data-taskmenu-theme="light"] .project-search-panel{background:#fff;border-color:#b9c0c8;box-shadow:-8px 0 22px rgba(0,0,0,.1)}
html[data-taskmenu-theme="light"] .project-search-hit{background:#ffdb45;color:#382400}
html[data-taskmenu-theme="light"] .project-search-result.selected .project-search-hit{background:#087cb9;color:#fff;outline-color:#023b62}
html[data-taskmenu-theme="light"] .project-search-input,html[data-taskmenu-theme="light"] .project-search-replace,html[data-taskmenu-theme="light"] .project-search-filter{background:#f7f8fa;border-color:#d5d9df}
html[data-taskmenu-theme="light"] .project-search-file-head,html[data-taskmenu-theme="light"] .project-replace-preview-label{background:#f6f8fa}
html[data-taskmenu-theme="light"] .project-replace-preview-diff{background:#f8f9fb;color:#202124}
`;
document.head.append(style);

const panel=document.createElement('aside');panel.className='project-search-panel';panel.setAttribute('role','complementary');panel.setAttribute('aria-label','Search and replace in project');
const panelTitle=document.createElement('div');panelTitle.className='project-search-panel-title';
const panelLabel=document.createElement('span');panelLabel.textContent='Search in Files · Ctrl+Shift+F';
const panelClose=document.createElement('button');panelClose.type='button';panelClose.textContent='×';panelClose.title='Close search panel';panelClose.setAttribute('aria-label','Close search panel');
panelTitle.append(panelLabel,panelClose);
const top=document.createElement('div');top.className='project-search-top';
const fields=document.createElement('div');fields.className='project-search-fields';

const queryRow=document.createElement('div');queryRow.className='project-search-query-row';
const input=document.createElement('input');input.className='project-search-input';input.type='search';input.autocomplete='off';input.spellcheck=false;input.placeholder='Search in Files';
const regexButton=document.createElement('button');regexButton.type='button';regexButton.className='project-search-toggle';regexButton.textContent='.*';regexButton.title='Use regular expression';
const caseButton=document.createElement('button');caseButton.type='button';caseButton.className='project-search-toggle';caseButton.textContent='Aa';caseButton.title='Match case';
const wordButton=document.createElement('button');wordButton.type='button';wordButton.className='project-search-toggle';wordButton.textContent='ab';wordButton.title='Match whole word';
queryRow.append(input,regexButton,caseButton,wordButton);

const replaceRow=document.createElement('div');replaceRow.className='project-search-replace-row';
const replacement=document.createElement('input');replacement.className='project-search-replace';replacement.type='text';replacement.autocomplete='off';replacement.spellcheck=false;replacement.placeholder='Replace with';
replaceRow.append(replacement);

const filterRow=document.createElement('div');filterRow.className='project-search-filter-row';
const include=document.createElement('input');include.className='project-search-filter';include.placeholder='Include glob, e.g. *.go, src/**';
const exclude=document.createElement('input');exclude.className='project-search-filter';exclude.placeholder='Exclude glob, e.g. vendor/**, *.min.js';
const scope=document.createElement('input');scope.className='project-search-filter';scope.placeholder='Folder scope: .';scope.title='Project-relative folder. Empty or . searches the whole workspace.';
filterRow.append(include,exclude,scope);
fields.append(queryRow,replaceRow,filterRow);

const actions=document.createElement('div');actions.className='project-search-actions';
const replaceAllButton=document.createElement('button');replaceAllButton.type='button';replaceAllButton.textContent='Preview Replace All';replaceAllButton.disabled=true;
const undoButton=document.createElement('button');undoButton.type='button';undoButton.textContent='Undo Replace';undoButton.disabled=true;
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='Close';
actions.append(replaceAllButton,undoButton,closeButton);
top.append(fields,actions);

const statusRow=document.createElement('div');statusRow.className='project-search-status-row';
const status=document.createElement('div');status.className='project-search-status';status.textContent='Type text to search project files';
const prevButton=document.createElement('button');prevButton.type='button';prevButton.textContent='↑';prevButton.title='Previous result';
const nextButton=document.createElement('button');nextButton.type='button';nextButton.textContent='↓';nextButton.title='Next result';
statusRow.append(status,prevButton,nextButton);
const results=document.createElement('div');results.className='project-search-results';
const replacePreview=document.createElement('div');replacePreview.className='project-replace-preview';
const previewHead=document.createElement('div');previewHead.className='project-replace-preview-head';
const previewTitle=document.createElement('div');previewTitle.className='project-replace-preview-title';
const previewBack=document.createElement('button');previewBack.type='button';previewBack.textContent='← Results';
const previewApply=document.createElement('button');previewApply.type='button';previewApply.textContent='Apply Replace All';
previewHead.append(previewBack,previewTitle,previewApply);
const previewFiles=document.createElement('div');previewFiles.className='project-replace-preview-files';
replacePreview.append(previewHead,previewFiles);
panel.append(panelTitle,top,statusRow,results,replacePreview);document.body.append(panel);

let items=[];
let selected=0;
let timer=null;
let controller=null;
let seq=0;
let searchOptions={regex:false,caseSensitive:false,wholeWord:false};
let activePreview=null;

function workspaceKey(){return String(app.taskData?.workspace||'workspace');}
function undoStorageKey(){return 'taskdeck:project-replace-undo:'+workspaceKey();}
function readUndoToken(){try{return String(localStorage.getItem(undoStorageKey())||'');}catch{return '';}}
function writeUndoToken(token){try{if(token)localStorage.setItem(undoStorageKey(),token);else localStorage.removeItem(undoStorageKey());}catch{}updateUndoButton();}
function updateUndoButton(){undoButton.disabled=!readUndoToken();}

function setStatus(message){status.textContent=message;}
function setEmpty(message){
  results.replaceChildren();
  const empty=document.createElement('div');empty.className='project-search-empty';empty.textContent=message;results.append(empty);
}
function updateToggle(button,active){button.classList.toggle('active',active);button.setAttribute('aria-pressed',active?'true':'false');}
function currentSearchParams(){
  const params=new URLSearchParams({q:input.value.trim(),limit:'200'});
  if(searchOptions.regex)params.set('regex','1');
  if(searchOptions.caseSensitive)params.set('case','1');
  if(searchOptions.wholeWord)params.set('word','1');
  if(include.value.trim())params.set('include',include.value.trim());
  if(exclude.value.trim())params.set('exclude',exclude.value.trim());
  if(scope.value.trim()&&scope.value.trim()!=='.')params.set('path',scope.value.trim());
  return params;
}
// Escape content via Text nodes; never render search results as HTML.
function renderPreviewMatches(preview,item){
  const value=String(item.preview||'');
  const query=String(input.value.trim());
  // The backend uses RE2/ripgrep. Never execute user-supplied regex in the
  // browser (different semantics and possible catastrophic backtracking).
  // Instead highlight the literal match strings that the backend verified.
  const candidates=searchOptions.regex
    ?items.filter(hit=>hit.path===item.path&&hit.line===item.line).map(hit=>String(hit.match||''))
    :[query];
  if(item.match)candidates.push(String(item.match));
  const terms=[...new Set(candidates.filter(Boolean))];
  if(!terms.length){preview.textContent=value;return;}
  const comparable=searchOptions.caseSensitive?value:value.toLowerCase();
  const ranges=[];
  for(const term of terms){
    const needle=searchOptions.caseSensitive?term:term.toLowerCase();
    let offset=0;
    while(offset<=comparable.length-needle.length){
      const from=comparable.indexOf(needle,offset);
      if(from<0)break;
      const to=from+term.length;
      offset=Math.max(from+1,to);
      if(searchOptions.wholeWord){
        const left=value[from-1]||'',right=value[to]||'';
        if(/[A-Za-z0-9_]/.test(left)||/[A-Za-z0-9_]/.test(right))continue;
      }
      ranges.push({from,to});
    }
  }
  ranges.sort((a,b)=>a.from-b.from||b.to-a.to);
  let cursor=0;
  for(const hit of ranges){
    if(hit.from<cursor)continue;
    preview.append(document.createTextNode(value.slice(cursor,hit.from)));
    const mark=document.createElement('mark');mark.className='project-search-hit';mark.textContent=value.slice(hit.from,hit.to);preview.append(mark);
    cursor=hit.to;
  }
  preview.append(document.createTextNode(value.slice(cursor)));
}
function groupedItems(){
  const groups=new Map();
  for(const item of items){
    if(!groups.has(item.path))groups.set(item.path,[]);
    groups.get(item.path).push(item);
  }
  return groups;
}
function render(){
  replacePreview.classList.remove('visible');results.style.display='';
  results.replaceChildren();
  replaceAllButton.disabled=!items.length||!input.value.trim();
  if(!items.length){setEmpty(input.value.trim()?'No matches found':'Type text to search project files');return;}
  let flatIndex=0;
  for(const [pathValue,matches] of groupedItems()){
    const group=document.createElement('section');group.className='project-search-file';
    const head=document.createElement('div');head.className='project-search-file-head';
    const name=document.createElement('span');name.className='project-search-file-name';name.textContent=pathValue;name.title=pathValue;
    const count=document.createElement('span');count.className='project-search-file-count';count.textContent=matches.length+' match'+(matches.length===1?'':'es');
    const replaceFile=document.createElement('button');replaceFile.type='button';replaceFile.textContent='Replace file';replaceFile.onclick=()=>replaceFileMatches(pathValue).catch(app.showError);
    head.append(name,count,replaceFile);group.append(head);
    for(const item of matches){
      const index=flatIndex++;
      const row=document.createElement('div');row.className='project-search-result'+(index===selected?' selected':'');
      const main=document.createElement('button');main.type='button';main.className='project-search-result-main';main.onclick=()=>choose(index).catch(app.showError);main.onmouseenter=()=>select(index);
      const location=document.createElement('span');location.className='project-search-location';location.textContent=item.path+':'+item.line+':'+item.column;location.title=location.textContent;
      const preview=document.createElement('span');preview.className='project-search-preview';renderPreviewMatches(preview,item);
      main.append(location,preview);
      const replaceOne=document.createElement('button');replaceOne.type='button';replaceOne.textContent='Replace';replaceOne.onclick=()=>replaceOneMatch(item).catch(app.showError);
      row.append(main,replaceOne);group.append(row);
    }
    results.append(group);
  }
}
function select(index){
  if(!items.length)return;
  selected=(index+items.length)%items.length;
  [...results.querySelectorAll('.project-search-result')].forEach((node,i)=>node.classList.toggle('selected',i===selected));
  results.querySelector('.project-search-result.selected')?.scrollIntoView({block:'nearest'});
}
async function choose(index=selected){
  const item=items[index];if(!item)return;
  const editor=globalThis.TaskMenuEditor;
  if(!editor?.openFile)throw new Error('Project editor unavailable');
  const view=await editor.openFile(item.path);
  if(!view?.cm)return;
  select(index);
  const ok=editor.focusProjectSearchResult?.(view,{
    line:item.line,column:item.column,match:item.match,matchLength:item.match_length,
    matches:items.filter(hit=>hit.path===item.path)
  });
  if(ok===false)setStatus('File changed since search · run the search again to update matches');
  // Keep the dock visible so another result can be selected immediately.
}
function cancelPending(){clearTimeout(timer);timer=null;if(controller){controller.abort();controller=null;}}
function refreshDockLayout(){
  // CodeMirror, split panes, minimaps and terminal fit need a re-measure.
  requestAnimationFrame(()=>window.dispatchEvent(new Event('resize')));
}
function close(){
  cancelPending();seq++;panel.classList.remove('visible');document.body.classList.remove('project-search-docked');
  activePreview=null;replacePreview.classList.remove('visible');
  globalThis.TaskMenuEditor?.clearProjectSearchHighlights?.();
  refreshDockLayout();
}
function open(options={}){
  panel.classList.add('visible');document.body.classList.add('project-search-docked');refreshDockLayout();
  if(typeof options.scope==='string')scope.value=options.scope==='.'?'':options.scope;
  if(typeof options.query==='string')input.value=options.query;
  items=[];selected=0;activePreview=null;setEmpty('Type text to search project files');setStatus('Type text to search project files');updateUndoButton();
  requestAnimationFrame(()=>{input.focus();if(input.value.trim())schedule();});
}
async function search(query,currentSeq){
  if(controller)controller.abort();
  controller=new AbortController();
  try{
    const params=currentSearchParams();params.set('q',query);
    const data=await app.jsonFetch('/api/project/content/search?'+params.toString(),{signal:controller.signal});
    if(currentSeq!==seq||!panel.classList.contains('visible'))return;
    items=Array.isArray(data.results)?data.results.slice(0,200):[];selected=0;
    setStatus(items.length+' match'+(items.length===1?'':'es')+' in '+groupedItems().size+' file'+(groupedItems().size===1?'':'s')+(items.length>=200?' · result limit reached':''));
    render();
  }catch(error){
    if(error?.name==='AbortError')return;
    if(currentSeq!==seq)return;
    items=[];setEmpty('Search failed');setStatus('Search failed: '+(error?.message||String(error)));console.warn('Project content search failed',error);
  }
}
function schedule(){
  globalThis.TaskMenuEditor?.clearProjectSearchHighlights?.();
  const query=input.value.trim();cancelPending();const currentSeq=++seq;activePreview=null;replacePreview.classList.remove('visible');results.style.display='';
  if(!query){items=[];selected=0;setEmpty('Type text to search project files');setStatus('Type text to search project files');replaceAllButton.disabled=true;return;}
  // Never allow an outdated result to open while a newer query is pending.
  items=[];selected=0;replaceAllButton.disabled=true;setEmpty('Searching…');setStatus('Searching…');
  timer=setTimeout(()=>search(query,currentSeq),120);
}
function replacePayload(mode,itemOrPath=''){
  const payload={
    action:'preview',query:input.value.trim(),replacement:replacement.value,
    regex:searchOptions.regex,case_sensitive:searchOptions.caseSensitive,whole_word:searchOptions.wholeWord,
    include:include.value.trim(),exclude:exclude.value.trim(),scope_path:scope.value.trim(),mode
  };
  if(mode==='file')payload.target_path=String(itemOrPath||'');
  if(mode==='match'){
    payload.target_path=String(itemOrPath?.path||'');
    payload.line=Number(itemOrPath?.line||0);payload.column=Number(itemOrPath?.column||0);
  }
  return payload;
}
async function previewReplace(mode,target=''){
  if(!input.value.trim())throw new Error('Search text is required');
  return app.jsonFetch('/api/project/content/replace',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(replacePayload(mode,target))});
}
function assertNoDirtyEditors(paths){
  const editor=globalThis.TaskMenuEditor;if(!editor?.editors)return;
  const wanted=new Set(paths||[]);
  for(const view of editor.editors.values()){
    if(view?.dirty&&wanted.has(view.file?.path))throw new Error('Save or close unsaved editor '+view.file.path+' before replace');
  }
}
async function reloadOpenEditors(paths){
  const editor=globalThis.TaskMenuEditor;if(!editor?.editors)return;
  const wanted=new Set(paths||[]);
  for(const view of [...editor.editors.values()]){
    if(!view?.dirty&&wanted.has(view.file?.path))await editor.reloadEditor?.(view);
  }
}
async function applyPreview(preview){
  const files=Array.isArray(preview?.files)?preview.files.map(file=>file.path):[];
  assertNoDirtyEditors(files);
  if(!preview?.token)return {ok:true,files:[],total_replacements:0};
  const result=await app.jsonFetch('/api/project/content/replace',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'apply',token:preview.token})});
  writeUndoToken(result.token||preview.token);
  await reloadOpenEditors(result.files||files);
  await globalThis.TaskMenuExplorer?.refreshGitStatus?.();
  schedule();
  return result;
}
async function replaceOneMatch(item){
  const preview=await previewReplace('match',item);
  if(!preview.token){setStatus('Selected match no longer exists');schedule();return;}
  const result=await applyPreview(preview);
  setStatus('Replaced '+(result.total_replacements||1)+' match in '+item.path);
}
async function replaceFileMatches(pathValue){
  const preview=await previewReplace('file',pathValue);
  if(!preview.token){setStatus('No matches remain in '+pathValue);schedule();return;}
  if(!window.confirm('Replace '+preview.total_replacements+' match'+(preview.total_replacements===1?'':'es')+' in '+pathValue+'?'))return;
  const result=await applyPreview(preview);
  setStatus('Replaced '+(result.total_replacements||0)+' match(es) in '+pathValue);
}
function showReplaceAllPreview(preview){
  activePreview=preview;results.style.display='none';replacePreview.classList.add('visible');previewFiles.replaceChildren();
  previewTitle.textContent='Replace All preview · '+preview.total_replacements+' replacement'+(preview.total_replacements===1?'':'s')+' · '+(preview.files?.length||0)+' file'+(preview.files?.length===1?'':'s');
  previewApply.disabled=!preview.token;
  for(const file of preview.files||[]){
    const card=document.createElement('section');card.className='project-replace-preview-file';
    const label=document.createElement('div');label.className='project-replace-preview-label';label.textContent=file.path+' · '+file.replacements+' replacement'+(file.replacements===1?'':'s');
    const diff=document.createElement('pre');diff.className='project-replace-preview-diff';diff.textContent=file.diff||'';
    card.append(label,diff);previewFiles.append(card);
  }
  if(!preview.files?.length){const empty=document.createElement('div');empty.className='project-search-empty';empty.textContent='No replacements to apply';previewFiles.append(empty);}
}
async function previewReplaceAll(){const preview=await previewReplace('all');showReplaceAllPreview(preview);}
async function applyReplaceAll(){
  const preview=activePreview;if(!preview?.token)return;
  previewApply.disabled=true;
  try{
    const result=await applyPreview(preview);
    activePreview=null;replacePreview.classList.remove('visible');results.style.display='';
    setStatus('Replace All applied · '+(result.total_replacements||0)+' replacement(s) · Undo is available');
  }finally{previewApply.disabled=false;}
}
async function undoReplace(){
  const token=readUndoToken();if(!token)return;
  const editor=globalThis.TaskMenuEditor;
  if(editor?.editors&&[...editor.editors.values()].some(view=>view?.dirty))throw new Error('Save or close unsaved editors before Undo Replace');
  const result=await app.jsonFetch('/api/project/content/replace',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'undo',token})});
  writeUndoToken('');
  await reloadOpenEditors(result.files||[]);
  await globalThis.TaskMenuExplorer?.refreshGitStatus?.();
  setStatus('Replace undone · '+(result.total_replacements||0)+' replacement(s) restored');schedule();
}

input.addEventListener('input',schedule);
replacement.addEventListener('input',()=>{activePreview=null;replacePreview.classList.remove('visible');results.style.display='';});
for(const field of [include,exclude,scope])field.addEventListener('input',schedule);
regexButton.onclick=()=>{searchOptions.regex=!searchOptions.regex;updateToggle(regexButton,searchOptions.regex);schedule();};
caseButton.onclick=()=>{searchOptions.caseSensitive=!searchOptions.caseSensitive;updateToggle(caseButton,searchOptions.caseSensitive);schedule();};
wordButton.onclick=()=>{searchOptions.wholeWord=!searchOptions.wholeWord;updateToggle(wordButton,searchOptions.wholeWord);schedule();};
input.addEventListener('keydown',event=>{
  if(event.key==='ArrowDown'){event.preventDefault();select(selected+1);}
  else if(event.key==='ArrowUp'){event.preventDefault();select(selected-1);}
  else if(event.key==='Enter'){event.preventDefault();choose().catch(app.showError);}
  else if(event.key==='Escape'){event.preventDefault();close();}
});
replaceAllButton.onclick=()=>previewReplaceAll().catch(app.showError);
previewApply.onclick=()=>applyReplaceAll().catch(app.showError);
previewBack.onclick=()=>{activePreview=null;replacePreview.classList.remove('visible');results.style.display='';};
undoButton.onclick=()=>undoReplace().catch(app.showError);
closeButton.onclick=close;
panelClose.onclick=close;
prevButton.onclick=()=>{select(selected-1);choose(selected).catch(app.showError);};
nextButton.onclick=()=>{select(selected+1);choose(selected).catch(app.showError);};
function globalProjectSearchShortcut(event){
  const primary=event.ctrlKey||event.metaKey;
  if(!primary||!event.shiftKey||event.altKey||event.key.toLowerCase()!=='f')return;
  event.preventDefault();event.stopPropagation();
  if(panel.classList.contains('visible'))input.focus();else open();
}
window.addEventListener('keydown',globalProjectSearchShortcut,true);
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&panel.classList.contains('visible')&&panel.contains(document.activeElement))close();});
updateToggle(regexButton,false);updateToggle(caseButton,false);updateToggle(wordButton,false);updateUndoButton();

globalThis.TaskMenuProjectSearch={open,close,schedule,previewReplace,undoReplace,get options(){return {...searchOptions,include:include.value,exclude:exclude.value,scope:scope.value};}};
