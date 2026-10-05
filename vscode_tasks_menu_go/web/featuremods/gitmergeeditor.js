const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for Git merge editor');

const style=document.createElement('style');
style.textContent=`
.git-merge-backdrop{display:none;position:fixed;inset:0;z-index:2820;background:rgba(0,0,0,.55);padding:18px}
.git-merge-backdrop.visible{display:flex}
.git-merge-dialog{width:min(1580px,calc(100vw - 36px));height:min(940px,calc(100vh - 36px));margin:auto;display:flex;flex-direction:column;min-width:0;min-height:0;background:#10151c;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 58px rgba(0,0,0,.58);overflow:hidden}
.git-merge-head{display:flex;align-items:center;gap:6px;padding:7px 9px;border-bottom:1px solid #303843;flex-wrap:wrap}
.git-merge-title{font-weight:700;font-size:12px;min-width:180px;flex:1}.git-merge-meta{font:10px ui-monospace,monospace;opacity:.62}
.git-merge-head button,.git-merge-head select{padding:4px 7px;font-size:11px}.git-merge-head select{max-width:min(460px,55vw);background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:5px}
.git-merge-sources{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));min-height:0;max-height:34vh;border-bottom:1px solid #303843}
.git-merge-source{min-width:0;display:flex;flex-direction:column}.git-merge-source+.git-merge-source{border-left:1px solid #303843}
.git-merge-source-head{padding:5px 8px;background:#151b23;font:10px ui-monospace,monospace;font-weight:700;border-bottom:1px solid #303843}
.git-merge-source pre{margin:0;padding:7px 9px;overflow:auto;white-space:pre;tab-size:4;font:11px/1.45 ui-monospace,monospace;min-height:110px}
.git-merge-source.missing pre{opacity:.52;font-style:italic}
.git-merge-actions{display:flex;align-items:center;gap:6px;padding:6px 9px;border-bottom:1px solid #303843;flex-wrap:wrap}.git-merge-actions button{padding:4px 7px;font-size:10px}
.git-merge-block-status{flex:1;min-width:180px;font:10px ui-monospace,monospace;opacity:.66}
.git-merge-result-head{display:flex;align-items:center;gap:8px;padding:6px 9px;background:#151b23;border-bottom:1px solid #303843}.git-merge-result-head strong{font-size:11px}.git-merge-result-state{font:10px ui-monospace,monospace;opacity:.65}
.git-merge-result{flex:1;min-height:0;width:100%;resize:none;border:0;outline:0;padding:9px 11px;background:#090d12;color:inherit;tab-size:4;font:12px/1.5 ui-monospace,monospace}
.git-merge-footer{display:flex;align-items:center;gap:6px;padding:7px 9px;border-top:1px solid #303843}.git-merge-footer-status{flex:1;font-size:10px;opacity:.68}.git-merge-footer button{padding:5px 8px;font-size:11px}
.git-merge-danger{color:#ff9a9a}
html[data-taskmenu-theme="light"] .git-merge-dialog{background:#fff;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .git-merge-source-head,html[data-taskmenu-theme="light"] .git-merge-result-head{background:#f6f8fa;border-color:#d0d7de}html[data-taskmenu-theme="light"] .git-merge-head select{background:#fff;color:#202124;border-color:#b9c0c8}html[data-taskmenu-theme="light"] .git-merge-result{background:#fbfcfd;color:#202124}
@media(max-width:900px){.git-merge-sources{grid-template-columns:1fr;max-height:42vh;overflow:auto}.git-merge-source+.git-merge-source{border-left:0;border-top:1px solid #303843}}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='git-merge-backdrop';
const dialog=document.createElement('div');dialog.className='git-merge-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','Git 3-way conflict editor');

const head=document.createElement('div');head.className='git-merge-head';
const title=document.createElement('div');title.className='git-merge-title';title.textContent='Git 3-way conflict editor';
const meta=document.createElement('span');meta.className='git-merge-meta';
const prevFile=document.createElement('button');prevFile.type='button';prevFile.textContent='← File';
const fileSelect=document.createElement('select');fileSelect.title='Unresolved conflict path';
const nextFile=document.createElement('button');nextFile.type='button';nextFile.textContent='File →';
const reloadButton=document.createElement('button');reloadButton.type='button';reloadButton.textContent='↻ Reload';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close merge editor';
head.append(title,meta,prevFile,fileSelect,nextFile,reloadButton,closeButton);

function sourcePane(label){
  const root=document.createElement('section');root.className='git-merge-source';
  const header=document.createElement('div');header.className='git-merge-source-head';header.textContent=label;
  const pre=document.createElement('pre');root.append(header,pre);return {root,header,pre};
}
const sources=document.createElement('div');sources.className='git-merge-sources';
const basePane=sourcePane('BASE · stage 1');
const currentPane=sourcePane('CURRENT / OURS · stage 2');
const incomingPane=sourcePane('INCOMING / THEIRS · stage 3');
sources.append(basePane.root,currentPane.root,incomingPane.root);

const actions=document.createElement('div');actions.className='git-merge-actions';
const prevConflict=document.createElement('button');prevConflict.type='button';prevConflict.textContent='← Previous conflict';
const nextConflict=document.createElement('button');nextConflict.type='button';nextConflict.textContent='Next conflict →';
const acceptCurrent=document.createElement('button');acceptCurrent.type='button';acceptCurrent.textContent='Accept Current';
const acceptIncoming=document.createElement('button');acceptIncoming.type='button';acceptIncoming.textContent='Accept Incoming';
const acceptBoth=document.createElement('button');acceptBoth.type='button';acceptBoth.textContent='Accept Both';
const copyBlock=document.createElement('button');copyBlock.type='button';copyBlock.textContent='Copy selected block';
const blockStatus=document.createElement('span');blockStatus.className='git-merge-block-status';
actions.append(prevConflict,nextConflict,acceptCurrent,acceptIncoming,acceptBoth,copyBlock,blockStatus);

const resultHead=document.createElement('div');resultHead.className='git-merge-result-head';
const resultTitle=document.createElement('strong');resultTitle.textContent='RESULT · working tree';
const resultState=document.createElement('span');resultState.className='git-merge-result-state';
const useWholeCurrent=document.createElement('button');useWholeCurrent.type='button';useWholeCurrent.textContent='Use whole Current';
const useWholeIncoming=document.createElement('button');useWholeIncoming.type='button';useWholeIncoming.textContent='Use whole Incoming';
resultHead.append(resultTitle,resultState,useWholeCurrent,useWholeIncoming);

const result=document.createElement('textarea');result.className='git-merge-result';result.spellcheck=false;result.wrap='off';

const footer=document.createElement('div');footer.className='git-merge-footer';
const footerStatus=document.createElement('span');footerStatus.className='git-merge-footer-status';
const saveButton=document.createElement('button');saveButton.type='button';saveButton.textContent='Save Result';
const markResolved=document.createElement('button');markResolved.type='button';markResolved.textContent='Mark resolved';
const abortButton=document.createElement('button');abortButton.type='button';abortButton.textContent='Abort operation';abortButton.className='git-merge-danger';
footer.append(footerStatus,saveButton,markResolved,abortButton);

dialog.append(head,sources,actions,resultHead,result,footer);backdrop.append(dialog);document.body.append(backdrop);

let session=null;
let blocks=[];
let selectedBlock=0;
let loadSeq=0;

function setFooter(text){footerStatus.textContent=text||'';}
function setPane(pane,side){
  const exists=Boolean(side?.exists);pane.root.classList.toggle('missing',!exists);
  pane.pre.textContent=exists?String(side.content??''):'(side does not exist — deleted on this side)';
}
function conflictFiles(){return Array.isArray(session?.data?.conflicts)?session.data.conflicts:[];}
function currentFileIndex(){return Math.max(0,conflictFiles().findIndex(item=>String(item.path||'')===String(session?.path||'')));}
function parseConflictBlocks(text){
  text=String(text??'');
  const pattern=/^<<<<<<<[^\r\n]*(?:\r?\n)([\s\S]*?)(?:^\|\|\|\|\|\|\|[^\r\n]*(?:\r?\n)([\s\S]*?))?^=======[^\r\n]*(?:\r?\n)([\s\S]*?)^>>>>>>>[^\r\n]*(?:(?:\r?\n)|$)/gm;
  const found=[];let match;
  while((match=pattern.exec(text))!==null){
    found.push({start:match.index,end:pattern.lastIndex,raw:match[0],current:match[1]||'',base:match[2]||'',incoming:match[3]||''});
    if(pattern.lastIndex===match.index)pattern.lastIndex++;
  }
  return found;
}
function refreshBlocks(preferIndex=selectedBlock){
  blocks=parseConflictBlocks(result.value);
  selectedBlock=blocks.length?Math.max(0,Math.min(preferIndex,blocks.length-1)):0;
  const active=blocks[selectedBlock]||null;
  blockStatus.textContent=blocks.length?('Conflict '+(selectedBlock+1)+' / '+blocks.length):'No conflict markers remain';
  for(const button of [prevConflict,nextConflict,acceptCurrent,acceptIncoming,acceptBoth,copyBlock])button.disabled=!active;
  prevConflict.disabled=!active||selectedBlock===0;nextConflict.disabled=!active||selectedBlock===blocks.length-1;
  if(active){
    result.focus();result.setSelectionRange(active.start,active.end);
    const before=result.value.slice(0,active.start);
    const lines=before.split(/\r?\n/).length;
    result.scrollTop=Math.max(0,(lines-4)*18);
  }
  resultState.textContent=(session?.dirty?'modified · ':'')+(blocks.length?blocks.length+' unresolved marker block(s)':'markers resolved');
}
function joinBoth(left,right){
  left=String(left??'');right=String(right??'');
  if(!left)return right;if(!right)return left;
  if(left.endsWith('\n')||left.endsWith('\r'))return left+right;
  return left+'\n'+right;
}
function replaceSelectedBlock(kind){
  const block=blocks[selectedBlock];if(!block)return;
  let replacement='';
  if(kind==='current')replacement=block.current;
  else if(kind==='incoming')replacement=block.incoming;
  else replacement=joinBoth(block.current,block.incoming);
  const text=result.value;
  result.value=text.slice(0,block.start)+replacement+text.slice(block.end);
  session.dirty=true;refreshBlocks(selectedBlock);setFooter('Result modified in browser. Save before marking resolved.');
}
async function copyText(value){
  value=String(value??'');
  if(navigator.clipboard&&window.isSecureContext){try{await navigator.clipboard.writeText(value);return;}catch{}}
  const area=document.createElement('textarea');area.value=value;area.setAttribute('readonly','');area.style.position='fixed';area.style.left='-9999px';document.body.append(area);area.select();try{document.execCommand('copy');}finally{area.remove();}
}
function editorForProjectPath(pathValue){
  const editor=globalThis.TaskMenuEditor;if(!editor?.editors)return null;
  for(const view of editor.editors.values())if(!view?.closed&&String(view.file?.path||'')===String(pathValue||''))return view;
  return null;
}
function assertNoDirtyProjectEditor(){
  const view=editorForProjectPath(session?.data?.project_path);
  if(view?.dirty)throw new Error('Save or close unsaved editor '+view.file.path+' before changing the merge Result');
  return view;
}
async function refreshOpenProjectEditor(){
  const view=editorForProjectPath(session?.data?.project_path);
  if(view&&!view.dirty)await globalThis.TaskMenuEditor?.reloadEditor?.(view);
}
async function fetchConflict(repoID,pathValue){
  const params=new URLSearchParams({repo:repoID,path:pathValue});
  return app.jsonFetch('/api/git/conflict-file?'+params.toString());
}
function renderFileSelector(){
  fileSelect.replaceChildren();
  for(const item of conflictFiles()){
    const option=document.createElement('option');option.value=String(item.path||'');option.textContent=String(item.path||'');fileSelect.append(option);
  }
  fileSelect.value=session.path;
  const idx=currentFileIndex(),count=conflictFiles().length;
  prevFile.disabled=count<2||idx<=0;nextFile.disabled=count<2||idx>=count-1;
}
async function loadPath(pathValue){
  const seq=++loadSeq;
  if(session?.dirty&&!window.confirm('Discard unsaved merge Result changes and load another conflict?'))return false;
  setFooter('Loading conflict stages…');
  const data=await fetchConflict(session.repoID,pathValue);
  if(seq!==loadSeq)return false;
  session.path=String(data.path||pathValue);session.data=data;session.dirty=false;
  title.textContent='Git 3-way conflict editor · '+session.path;
  meta.textContent=[data.operation,data.branch,data.status].filter(Boolean).join(' · ');
  setPane(basePane,data.base);setPane(currentPane,data.current);setPane(incomingPane,data.incoming);
  result.value=data.result?.exists?String(data.result.content??''):'';
  result.disabled=!data.result?.exists;
  saveButton.disabled=!data.result?.exists;
  useWholeCurrent.disabled=false;
  useWholeIncoming.disabled=false;
  renderFileSelector();refreshBlocks(0);
  setFooter(data.result?.exists?'Loaded. Result is not staged until Mark resolved.':'Working Result is absent. Use a whole Current/Incoming side to resolve this delete/modify conflict.');
  return true;
}
async function saveResult(){
  if(!session?.data?.result?.exists)throw new Error('Working Result does not exist; use whole Current/Incoming for this conflict');
  assertNoDirtyProjectEditor();
  const expected=String(session.data.result.sha256||'');if(!expected)throw new Error('Result SHA is unavailable');
  const response=await app.jsonFetch('/api/git/conflict-file?repo='+encodeURIComponent(session.repoID),{
    method:'PUT',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({path:session.path,content:result.value,expected_sha256:expected})
  });
  session.data.result=response.result;session.dirty=false;resultState.textContent=(blocks.length?blocks.length+' unresolved marker block(s)':'markers resolved');
  await refreshOpenProjectEditor();setFooter('Result saved to working tree; Git index is still unresolved.');
  return response;
}
async function runRepair(name,payload={}){
  const git=globalThis.TaskMenuGitFiles;if(!git?.runRepair)throw new Error('Git recovery integration unavailable');
  return git.runRepair(name,payload,session.repoID);
}
async function afterResolution(response){
  await globalThis.TaskMenuExplorer?.refreshGitStatus?.();
  const remaining=Array.isArray(response?.conflict_state?.files)?response.conflict_state.files:[];
  if(!remaining.length){session.dirty=false;setFooter('All unmerged paths are resolved and staged. You can continue the Git operation.');closeAfterResolveButtons();return;}
  const currentIndex=Math.min(currentFileIndex(),remaining.length-1);
  session.data.conflicts=remaining;session.dirty=false;
  await loadPath(remaining[currentIndex]?.path||remaining[0].path);
}
function closeAfterResolveButtons(){
  result.disabled=true;saveButton.disabled=true;markResolved.disabled=true;useWholeCurrent.disabled=true;useWholeIncoming.disabled=true;
  for(const button of [prevConflict,nextConflict,acceptCurrent,acceptIncoming,acceptBoth,copyBlock])button.disabled=true;
}
async function useWhole(side){
  const label=side==='current'?'Current':'Incoming';
  if(!window.confirm('Use whole '+label+' side for '+session.path+' and stage it as resolved?'))return;
  const response=await runRepair('conflict_take_side',{path:session.path,conflict_side:side});
  await refreshOpenProjectEditor();await afterResolution(response);
}
async function markCurrentResolved(){
  if(blocks.length)throw new Error('Resolve all conflict marker blocks before Mark resolved');
  if(session.dirty)await saveResult();
  if(!window.confirm('Mark '+session.path+' resolved and stage the current Result?'))return;
  const response=await runRepair('conflict_mark_resolved',{path:session.path});
  await afterResolution(response);
}
async function navigateFile(delta){
  const files=conflictFiles(),idx=currentFileIndex(),next=files[idx+delta];if(!next)return;
  await loadPath(next.path);
}
async function abortOperation(){
  if(!window.confirm('Abort the active Git '+(session?.data?.operation||'operation')+'? Conflict-resolution work for this operation will be discarded by Git.'))return;
  const response=await runRepair('abort_in_progress',{confirmed:true});
  session.dirty=false;setFooter(response?.output||'Git operation aborted.');close();
}
async function open(options={}){
  const repoID=String(options.repoID||'').trim(),pathValue=String(options.path||'').trim();
  if(!repoID||!pathValue)throw new Error('Git merge editor requires repository and conflict path');
  if(backdrop.classList.contains('visible')&&session?.dirty&&!window.confirm('Discard unsaved merge Result changes?'))return;
  session={repoID,path:pathValue,data:null,dirty:false};selectedBlock=0;blocks=[];
  backdrop.classList.add('visible');
  try{await loadPath(pathValue);}catch(error){backdrop.classList.remove('visible');session=null;throw error;}
}
function close(){
  if(session?.dirty&&!window.confirm('Close merge editor and discard unsaved Result changes?'))return false;
  backdrop.classList.remove('visible');session=null;blocks=[];result.value='';return true;
}

result.addEventListener('input',()=>{if(!session)return;session.dirty=true;refreshBlocks(selectedBlock);});
prevConflict.onclick=()=>{if(selectedBlock>0){selectedBlock--;refreshBlocks(selectedBlock);}};
nextConflict.onclick=()=>{if(selectedBlock<blocks.length-1){selectedBlock++;refreshBlocks(selectedBlock);}};
acceptCurrent.onclick=()=>replaceSelectedBlock('current');
acceptIncoming.onclick=()=>replaceSelectedBlock('incoming');
acceptBoth.onclick=()=>replaceSelectedBlock('both');
copyBlock.onclick=()=>{const block=blocks[selectedBlock];if(block)copyText(block.raw).then(()=>setFooter('Selected conflict block copied.')).catch(app.showError);};
saveButton.onclick=()=>saveResult().catch(app.showError);
markResolved.onclick=()=>markCurrentResolved().catch(app.showError);
useWholeCurrent.onclick=()=>useWhole('current').catch(app.showError);
useWholeIncoming.onclick=()=>useWhole('incoming').catch(app.showError);
prevFile.onclick=()=>navigateFile(-1).catch(app.showError);nextFile.onclick=()=>navigateFile(1).catch(app.showError);
fileSelect.onchange=()=>{if(fileSelect.value)loadPath(fileSelect.value).catch(app.showError);};
reloadButton.onclick=()=>{if(session)loadPath(session.path).catch(app.showError);};
abortButton.onclick=()=>abortOperation().catch(app.showError);
closeButton.onclick=()=>close();
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();});

globalThis.TaskMenuGitMergeEditor={open,close,parseConflictBlocks,get session(){return session;}};
