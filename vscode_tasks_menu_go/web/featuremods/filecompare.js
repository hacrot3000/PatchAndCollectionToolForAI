const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for file compare');

const style=document.createElement('style');
style.textContent=`
.file-compare-backdrop{display:none;position:fixed;inset:0;z-index:2750;background:rgba(0,0,0,.52);padding:20px}
.file-compare-backdrop.visible{display:flex}
.file-compare-dialog{width:min(1500px,calc(100vw - 40px));height:min(900px,calc(100vh - 40px));margin:auto;display:flex;flex-direction:column;min-width:0;min-height:0;background:#10151c;border:1px solid #46505d;border-radius:10px;box-shadow:0 20px 58px rgba(0,0,0,.55);overflow:hidden}
.file-compare-head{display:flex;align-items:center;gap:7px;padding:7px 9px;border-bottom:1px solid #303843}
.file-compare-title{font-weight:700;font-size:12px;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.file-compare-head button{padding:4px 7px;font-size:11px}.file-compare-head button.active{background:#34445a;border-color:#6f91bb}
.file-compare-summary{font:10px ui-monospace,monospace;opacity:.66;white-space:nowrap}
.file-compare-columns{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);border-bottom:1px solid #303843;background:#151b23}
.file-compare-column{padding:6px 9px;font:11px ui-monospace,monospace;font-weight:700;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.file-compare-column+.file-compare-column{border-left:1px solid #303843}
.file-compare-body{flex:1;min-height:0;overflow:auto;background:#0b0f14}
.file-compare-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);min-width:780px}
.file-compare-cell{display:grid;grid-template-columns:48px minmax(0,1fr);min-width:0;border-bottom:1px solid rgba(255,255,255,.035)}
.file-compare-cell+.file-compare-cell{border-left:1px solid #303843}.file-compare-no{padding:1px 7px;text-align:right;user-select:none;opacity:.45;font:10px/1.45 ui-monospace,monospace;border-right:1px solid rgba(255,255,255,.06)}
.file-compare-code{padding:1px 7px;white-space:pre;overflow-x:auto;font:11px/1.45 ui-monospace,monospace}.file-compare-cell.removed{background:rgba(185,67,67,.18)}.file-compare-cell.added{background:rgba(54,145,78,.18)}.file-compare-cell.blank{opacity:.3}
.file-compare-hunk{border-top:1px solid #3a4350;border-bottom:1px solid #3a4350;margin:5px 0}.file-compare-hunk-head{position:sticky;left:0;display:flex;align-items:center;gap:6px;padding:4px 8px;background:#171e27;font:10px ui-monospace,monospace;z-index:1}.file-compare-hunk-label{flex:1;opacity:.72}.file-compare-hunk-head button{padding:2px 6px;font-size:10px}
.file-compare-inline{min-width:640px}.file-compare-inline-line{display:grid;grid-template-columns:52px 20px minmax(0,1fr);border-bottom:1px solid rgba(255,255,255,.035);font:11px/1.45 ui-monospace,monospace}.file-compare-inline-line span{padding:1px 7px}.file-compare-inline-no{text-align:right;opacity:.45}.file-compare-inline-line.removed{background:rgba(185,67,67,.18)}.file-compare-inline-line.added{background:rgba(54,145,78,.18)}
.file-compare-empty{padding:24px;text-align:center;opacity:.62}
html[data-taskmenu-theme="light"] .file-compare-dialog{background:#fff;border-color:#b9c0c8}.file-compare-body{color:inherit}html[data-taskmenu-theme="light"] .file-compare-columns,html[data-taskmenu-theme="light"] .file-compare-hunk-head{background:#f6f8fa;border-color:#d0d7de}html[data-taskmenu-theme="light"] .file-compare-cell.removed,html[data-taskmenu-theme="light"] .file-compare-inline-line.removed{background:#fff0f0}html[data-taskmenu-theme="light"] .file-compare-cell.added,html[data-taskmenu-theme="light"] .file-compare-inline-line.added{background:#effaf1}
`;
document.head.append(style);

const backdrop=document.createElement('div');backdrop.className='file-compare-backdrop';
const dialog=document.createElement('div');dialog.className='file-compare-dialog';dialog.setAttribute('role','dialog');dialog.setAttribute('aria-label','File compare');
const head=document.createElement('div');head.className='file-compare-head';
const title=document.createElement('div');title.className='file-compare-title';
const summary=document.createElement('div');summary.className='file-compare-summary';
const sideButton=document.createElement('button');sideButton.type='button';sideButton.textContent='Side by side';
const inlineButton=document.createElement('button');inlineButton.type='button';inlineButton.textContent='Inline';
const reloadButton=document.createElement('button');reloadButton.type='button';reloadButton.textContent='↻';reloadButton.title='Reload both compare sources';
const closeButton=document.createElement('button');closeButton.type='button';closeButton.textContent='×';closeButton.title='Close compare';
head.append(title,summary,sideButton,inlineButton,reloadButton,closeButton);
const columns=document.createElement('div');columns.className='file-compare-columns';
const leftLabel=document.createElement('div');leftLabel.className='file-compare-column';
const rightLabel=document.createElement('div');rightLabel.className='file-compare-column';columns.append(leftLabel,rightLabel);
const body=document.createElement('div');body.className='file-compare-body';
dialog.append(head,columns,body);backdrop.append(dialog);document.body.append(backdrop);

let current=null;
let viewMode='side';

function splitLines(text){return String(text??'').replace(/\r\n/g,'\n').replace(/\r/g,'\n').split('\n');}
function commonPrefix(a,b){let i=0;while(i<a.length&&i<b.length&&a[i]===b[i])i++;return i;}
function commonSuffix(a,b,prefix){let i=0;while(a.length-1-i>=prefix&&b.length-1-i>=prefix&&a[a.length-1-i]===b[b.length-1-i])i++;return i;}
function lcsBlock(a,b){
  const n=a.length,m=b.length;
  if(!n)return b.map(text=>({kind:'add',text}));
  if(!m)return a.map(text=>({kind:'remove',text}));
  if(n*m>1200000){
    return [...a.map(text=>({kind:'remove',text})),...b.map(text=>({kind:'add',text}))];
  }
  const dp=Array.from({length:n+1},()=>new Uint32Array(m+1));
  for(let i=n-1;i>=0;i--)for(let j=m-1;j>=0;j--)dp[i][j]=a[i]===b[j]?dp[i+1][j+1]+1:Math.max(dp[i+1][j],dp[i][j+1]);
  const ops=[];let i=0,j=0;
  while(i<n&&j<m){
    if(a[i]===b[j]){ops.push({kind:'equal',text:a[i]});i++;j++;}
    else if(dp[i+1][j]>=dp[i][j+1]){ops.push({kind:'remove',text:a[i++]});}
    else{ops.push({kind:'add',text:b[j++]});}
  }
  while(i<n)ops.push({kind:'remove',text:a[i++]});
  while(j<m)ops.push({kind:'add',text:b[j++]});
  return ops;
}
function diffOperations(leftText,rightText){
  const a=splitLines(leftText),b=splitLines(rightText),prefix=commonPrefix(a,b),suffix=commonSuffix(a,b,prefix);
  const ops=[];
  for(let i=0;i<prefix;i++)ops.push({kind:'equal',text:a[i]});
  ops.push(...lcsBlock(a.slice(prefix,a.length-suffix),b.slice(prefix,b.length-suffix)));
  for(let i=suffix;i>0;i--)ops.push({kind:'equal',text:a[a.length-i]});
  return ops;
}
function buildCompareModel(leftText,rightText){
  const ops=diffOperations(leftText,rightText);
  const rows=[];const hunks=[];
  let leftNo=1,rightNo=1,hunk=null,pendingRemoved=[],pendingAdded=[];
  const flushChange=()=>{
    if(!pendingRemoved.length&&!pendingAdded.length)return;
    const count=Math.max(pendingRemoved.length,pendingAdded.length);
    if(!hunk)hunk={index:hunks.length,leftStart:leftNo-1,rightStart:rightNo-1,leftCount:0,rightCount:0,leftLines:[],rightLines:[],rowStart:rows.length};
    for(let i=0;i<count;i++){
      const l=i<pendingRemoved.length?{no:leftNo++,text:pendingRemoved[i],kind:'removed'}:null;
      const r=i<pendingAdded.length?{no:rightNo++,text:pendingAdded[i],kind:'added'}:null;
      if(l){hunk.leftCount++;hunk.leftLines.push(l.text);}if(r){hunk.rightCount++;hunk.rightLines.push(r.text);}
      rows.push({left:l,right:r,hunk:hunk.index});
    }
    pendingRemoved=[];pendingAdded=[];
  };
  for(const op of ops){
    if(op.kind==='remove'){pendingRemoved.push(op.text);continue;}
    if(op.kind==='add'){pendingAdded.push(op.text);continue;}
    flushChange();
    if(hunk){hunk.rowEnd=rows.length;hunks.push(hunk);hunk=null;}
    rows.push({left:{no:leftNo++,text:op.text,kind:'context'},right:{no:rightNo++,text:op.text,kind:'context'},hunk:-1});
  }
  flushChange();if(hunk){hunk.rowEnd=rows.length;hunks.push(hunk);}
  return {rows,hunks,identical:hunks.length===0};
}
function replaceLineRange(text,start,count,replacementLines){
  const lines=splitLines(text);
  lines.splice(start,count,...replacementLines);
  return lines.join('\n');
}
async function loadSource(source){
  const loaded=await source.load();
  source.text=String(loaded?.text??loaded?.content??'');
  if(loaded&&typeof loaded==='object')Object.assign(source.meta||(source.meta={}),loaded);
  return source;
}
async function saveSource(source,text){
  if(typeof source.writeText!=='function')throw new Error((source.label||'Compare side')+' is read-only');
  const result=await source.writeText(text,source);
  source.text=text;
  if(result&&typeof result==='object')Object.assign(source.meta||(source.meta={}),result);
}
function cell(spec){
  const node=document.createElement('div');node.className='file-compare-cell '+(spec?.kind||'blank');
  const no=document.createElement('span');no.className='file-compare-no';no.textContent=spec?.no?String(spec.no):'';
  const code=document.createElement('span');code.className='file-compare-code';code.textContent=spec?spec.text:'\u00a0';node.append(no,code);return node;
}
function renderSide(model){
  const frag=document.createDocumentFragment();let activeHunk=-1;
  for(const row of model.rows){
    if(row.hunk>=0&&row.hunk!==activeHunk){
      activeHunk=row.hunk;const h=model.hunks[activeHunk];
      const card=document.createElement('div');card.className='file-compare-hunk';
      const hh=document.createElement('div');hh.className='file-compare-hunk-head';
      const label=document.createElement('span');label.className='file-compare-hunk-label';label.textContent='Change '+(h.index+1)+' · left '+(h.leftStart+1)+'+'+h.leftCount+' ↔ right '+(h.rightStart+1)+'+'+h.rightCount;
      hh.append(label);
      if(typeof current?.right?.writeText==='function'){const b=document.createElement('button');b.type='button';b.textContent='Left → Right';b.onclick=()=>copyHunk(h,'left-to-right').catch(app.showError);hh.append(b);}
      if(typeof current?.left?.writeText==='function'){const b=document.createElement('button');b.type='button';b.textContent='Right → Left';b.onclick=()=>copyHunk(h,'right-to-left').catch(app.showError);hh.append(b);}
      card.append(hh);frag.append(card);
    }
    const line=document.createElement('div');line.className='file-compare-row';line.append(cell(row.left),cell(row.right));frag.append(line);
  }
  body.append(frag);
}
function renderInline(model){
  const host=document.createElement('div');host.className='file-compare-inline';
  for(const row of model.rows){
    if(row.left&&row.right&&row.left.kind==='context'){
      const line=document.createElement('div');line.className='file-compare-inline-line';
      const no=document.createElement('span');no.className='file-compare-inline-no';no.textContent=String(row.left.no);
      const mark=document.createElement('span');mark.textContent=' ';
      const code=document.createElement('span');code.textContent=row.left.text;line.append(no,mark,code);host.append(line);continue;
    }
    if(row.left){
      const line=document.createElement('div');line.className='file-compare-inline-line removed';
      const no=document.createElement('span');no.className='file-compare-inline-no';no.textContent=String(row.left.no);
      const mark=document.createElement('span');mark.textContent='−';const code=document.createElement('span');code.textContent=row.left.text;line.append(no,mark,code);host.append(line);
    }
    if(row.right){
      const line=document.createElement('div');line.className='file-compare-inline-line added';
      const no=document.createElement('span');no.className='file-compare-inline-no';no.textContent=String(row.right.no);
      const mark=document.createElement('span');mark.textContent='+';const code=document.createElement('span');code.textContent=row.right.text;line.append(no,mark,code);host.append(line);
    }
  }
  body.append(host);
}
function render(){
  body.replaceChildren();if(!current)return;
  const model=buildCompareModel(current.left.text,current.right.text);current.model=model;
  leftLabel.textContent=current.left.label||'Left';rightLabel.textContent=current.right.label||'Right';
  title.textContent=(current.title||'File Compare')+' · '+leftLabel.textContent+' ↔ '+rightLabel.textContent;
  summary.textContent=model.identical?'identical':model.hunks.length+' change block'+(model.hunks.length===1?'':'s');
  sideButton.classList.toggle('active',viewMode==='side');inlineButton.classList.toggle('active',viewMode==='inline');columns.style.display=viewMode==='side'?'grid':'none';
  if(model.identical){const empty=document.createElement('div');empty.className='file-compare-empty';empty.textContent='No differences';body.append(empty);return;}
  if(viewMode==='inline')renderInline(model);else renderSide(model);
}
async function copyHunk(hunk,direction){
  if(!current)return;
  const from=direction==='left-to-right'?current.left:current.right;
  const to=direction==='left-to-right'?current.right:current.left;
  const start=direction==='left-to-right'?hunk.rightStart:hunk.leftStart;
  const count=direction==='left-to-right'?hunk.rightCount:hunk.leftCount;
  const replacement=direction==='left-to-right'?hunk.leftLines:hunk.rightLines;
  const next=replaceLineRange(to.text,start,count,replacement);
  if(!window.confirm('Apply change '+(hunk.index+1)+' from '+(from.label||'source')+' to '+(to.label||'destination')+'?'))return;
  await saveSource(to,next);await reload();window.dispatchEvent(new CustomEvent('taskmenu:file-compare-write',{detail:{label:to.label,path:to.path||'',profile_id:to.profileID||'',source_kind:to.kind||''}}));
}
async function reload(){if(!current)return;await Promise.all([loadSource(current.left),loadSource(current.right)]);render();}
async function open(options){
  if(!options?.left?.load||!options?.right?.load)throw new Error('File compare requires left and right sources');
  current={title:options.title||'File Compare',left:{...options.left,meta:{...(options.left.meta||{})}},right:{...options.right,meta:{...(options.right.meta||{})}}};
  await Promise.all([loadSource(current.left),loadSource(current.right)]);backdrop.classList.add('visible');render();
}
function close(){backdrop.classList.remove('visible');current=null;body.replaceChildren();}
function projectSource(pathValue,{writable=true,label=''}={}){
  pathValue=String(pathValue||'').trim();
  const source={path:pathValue,label:label||pathValue,meta:{},load:async()=>{
    const file=await app.jsonFetch('/api/project/file?path='+encodeURIComponent(pathValue));
    return {text:file.content,sha256:file.sha256,file};
  }};
  if(writable)source.writeText=async text=>{
    const sha=source.meta?.sha256||source.meta?.file?.sha256;
    if(!sha)throw new Error('Project compare source SHA is unavailable');
    const saved=await app.jsonFetch('/api/project/file',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({path:pathValue,content:text,expected_sha256:sha})});
    return {text:saved.content,sha256:saved.sha256,file:saved};
  };
  return source;
}
function editorSource(view,{label='Current editor'}={}){
  return {path:view?.file?.path||'',label:(label+' · '+(view?.file?.path||'')),load:async()=>({text:view.cm.state.doc.toString()}),writeText:async text=>{
    if(view.closed)throw new Error('Editor is closed');
    const doc=view.cm.state.doc;view.cm.dispatch({changes:{from:0,to:doc.length,insert:text}});view.cm.focus();return {text};
  }};
}
function savedEditorSource(view){
  return projectSource(view?.file?.path,{writable:false,label:'Saved · '+(view?.file?.path||'')});
}
function clipboardSource({label='Clipboard'}={}){
  return {label,load:async()=>{
    if(!navigator.clipboard?.readText)throw new Error('Clipboard read is unavailable in this browser/context');
    return {text:await navigator.clipboard.readText()};
  }};
}
function remoteSource(profileID,pathValue,{writable=true,label=''}={}){
  profileID=String(profileID||'').trim();pathValue=String(pathValue||'').trim();
  const source={kind:'remote',profileID,path:pathValue,label:label||('Remote · '+pathValue),meta:{},load:async()=>{
    const params=new URLSearchParams({profile_id:profileID,path:pathValue});
    const data=await app.jsonFetch('/api/file-transfer/text?'+params.toString());
    return {text:data.content,sha256:data.sha256,profile_id:data.profile_id,path:data.path};
  }};
  if(writable)source.writeText=async text=>{
    const sha=source.meta?.sha256;
    if(!sha)throw new Error('Remote compare source SHA is unavailable');
    return app.jsonFetch('/api/file-transfer/text',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:profileID,path:pathValue,content:text,expected_sha256:sha})});
  };
  return source;
}
function browserFileHandleSource(handle,{label='Local browser file',path=''}={}){
  if(!handle||handle.kind!=='file')throw new Error('Local browser file handle is required');
  const source={kind:'browser-local',path,label,meta:{},load:async()=>{
    const file=await handle.getFile();
    return {text:await file.text(),size:file.size,lastModified:file.lastModified};
  },writeText:async text=>{
    const before=await handle.getFile();
    if(source.meta?.lastModified!==undefined&&(before.lastModified!==source.meta.lastModified||before.size!==source.meta.size))throw new Error('Local browser file changed after compare load');
    if(typeof handle.createWritable!=='function')throw new Error('Local browser file is read-only');
    const writer=await handle.createWritable();
    try{await writer.write(text);await writer.close();}catch(error){try{await writer.abort?.();}catch{}throw error;}
    const after=await handle.getFile();
    return {text,size:after.size,lastModified:after.lastModified};
  }};
  return source;
}
async function openLeftRemote(leftSource,profileID,remotePath){
  return open({title:'Left ↔ Remote',left:leftSource,right:remoteSource(profileID,remotePath)});
}
function gitCommitSource(repoID,pathValue,ref,{label=''}={}){
  repoID=String(repoID||'').trim();pathValue=String(pathValue||'').trim();ref=String(ref||'').trim();
  return {kind:'git-commit',label:label||('Git '+ref.slice(0,8)+' · '+pathValue),repoID,path:pathValue,ref,load:async()=>{
    const params=new URLSearchParams({view:'file-content',repo:repoID,path:pathValue,ref});
    const data=await app.jsonFetch('/api/git/status?'+params.toString());
    return {text:data.content,commit:data.commit,ref:data.ref,path:data.path,repo_id:data.repo_id};
  }};
}
function gitStateSource(repoID,pathValue,state,{label=''}={}){
  repoID=String(repoID||'').trim();pathValue=String(pathValue||'').trim();state=String(state||'').trim().toLowerCase();
  if(state!=='head'&&state!=='index')throw new Error('Git compare state must be head or index');
  const defaultLabel=state==='head'?'HEAD · committed':'INDEX · staged';
  return {kind:'git-'+state,label:label||(defaultLabel+' · '+pathValue),repoID,path:pathValue,state,load:async()=>{
    const params=new URLSearchParams({view:'file-content',repo:repoID,path:pathValue,state});
    const data=await app.jsonFetch('/api/git/status?'+params.toString());
    return {text:data.content,commit:data.commit,state:data.state,path:data.path,repo_id:data.repo_id};
  }};
}
function editorForProjectPath(pathValue){
  pathValue=String(pathValue||'').replace(/\\/g,'/').replace(/^\.\//,'');
  const editor=globalThis.TaskMenuEditor;
  if(!editor?.editors)return null;
  for(const view of editor.editors.values()){
    if(view?.closed)continue;
    const current=String(view?.file?.path||'').replace(/\\/g,'/').replace(/^\.\//,'');
    if(current===pathValue)return view;
  }
  return null;
}
function workingProjectSource(pathValue,{label=''}={}){
  const view=editorForProjectPath(pathValue);
  if(view)return editorSource(view,{label:label||'WORKTREE editor'});
  return projectSource(pathValue,{label:label||('WORKTREE · '+pathValue)});
}
async function openGitCommitAgainstProject(repoID,repoPath,workspacePath,ref){
  return open({title:'Git commit ↔ Current',left:gitCommitSource(repoID,repoPath,ref),right:workingProjectSource(workspacePath,{label:'CURRENT · '+workspacePath})});
}
async function openGitCommits(repoID,pathValue,leftRef,rightRef){
  return open({title:'Git commit ↔ Git commit',left:gitCommitSource(repoID,pathValue,leftRef),right:gitCommitSource(repoID,pathValue,rightRef)});
}
async function openGitStatePair(repoID,repoPath,workspacePath,mode){
  mode=String(mode||'').trim();
  if(mode==='staged'){
    return open({title:'HEAD ↔ Staged',left:gitStateSource(repoID,repoPath,'head'),right:gitStateSource(repoID,repoPath,'index')});
  }
  if(mode==='worktree'){
    return open({title:'Staged ↔ Working',left:gitStateSource(repoID,repoPath,'index'),right:workingProjectSource(workspacePath,{label:'WORKTREE · '+workspacePath})});
  }
  if(mode==='head-worktree'){
    return open({title:'HEAD ↔ Working',left:gitStateSource(repoID,repoPath,'head'),right:workingProjectSource(workspacePath,{label:'WORKTREE · '+workspacePath})});
  }
  throw new Error('Unsupported Git compare mode '+mode);
}
async function openProjectFiles(leftPath,rightPath){return open({title:'Project files',left:projectSource(leftPath),right:projectSource(rightPath)});}
async function openEditorSaved(view){return open({title:'Current ↔ Saved',left:editorSource(view),right:savedEditorSource(view)});}
async function openEditorClipboard(view){return open({title:'Current ↔ Clipboard',left:editorSource(view),right:clipboardSource()});}
async function promptProjectCompare(pathValue){
  const other=window.prompt('Compare '+pathValue+' with project-relative file:','');
  if(other===null||!String(other).trim())return;
  return openProjectFiles(pathValue,String(other).trim());
}

sideButton.onclick=()=>{viewMode='side';render();};inlineButton.onclick=()=>{viewMode='inline';render();};
reloadButton.onclick=()=>reload().catch(app.showError);closeButton.onclick=close;
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&backdrop.classList.contains('visible'))close();});

globalThis.TaskMenuFileCompare={open,close,reload,buildCompareModel,projectSource,editorSource,savedEditorSource,clipboardSource,remoteSource,browserFileHandleSource,openLeftRemote,gitCommitSource,gitStateSource,workingProjectSource,openProjectFiles,openEditorSaved,openEditorClipboard,openGitCommitAgainstProject,openGitCommits,openGitStatePair,promptProjectCompare,get current(){return current;}};
