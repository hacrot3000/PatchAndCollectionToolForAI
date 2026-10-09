const app=globalThis.TaskMenuApp;
const tabs=document.querySelector('#tabs'),panes=document.querySelector('#panes');
if(!app||!tabs||!panes)throw new Error('Directory Compare requires tab hosts');
const MAX_ENTRIES=10000,MAX_DEPTH=64,TAB_ID='directory-compare';
const css=document.createElement('style');
css.textContent=[
'.dircmp-pane{overflow:hidden;background:#111820}.dircmp-root{height:100%;display:flex;flex-direction:column;min-height:0}',
'.dircmp-tools{display:flex;align-items:center;gap:7px;flex-wrap:wrap;padding:8px;border-bottom:1px solid #424c59;font-size:11px}',
'.dircmp-tools button{padding:4px 7px;font-size:11px}.dircmp-status{flex:1;min-width:150px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;opacity:.8}',
'.dircmp-head,.dircmp-row{display:grid;grid-template-columns:minmax(0,1fr) 125px minmax(0,1fr)}',
'.dircmp-head{background:#202a35;font-size:11px;border-bottom:1px solid #455364}',
'.dircmp-head>div{padding:7px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
'.dircmp-head>div:nth-child(2){text-align:center}',
'.dircmp-tree{min-height:0;flex:1;overflow:auto;font:12px/1.5 ui-monospace,monospace}',
'.dircmp-row{border-bottom:1px solid #29303a;min-height:28px}.dircmp-row:hover{background:#263342}',
'.dircmp-cell{display:flex;align-items:center;min-width:0;gap:6px;padding:3px 6px;overflow:hidden;white-space:nowrap}',
'.dircmp-cell .dircmp-name{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}',
'.dircmp-cell .dircmp-icon{flex:none;min-width:12px}'+
'.dircmp-icon[data-status="left-only"]{color:#e9b279}.dircmp-icon[data-status="right-only"]{color:#8ebceb}'+
'.dircmp-icon[data-status="important"]{color:#ed948f}.dircmp-icon[data-status="unimportant"]{color:#88c6c9}'+
'.dircmp-icon[data-status="changed"]{color:#e2b76c}.dircmp-icon[data-status="same"]{color:#85bba4}.dircmp-center{display:flex;gap:3px;align-items:center;justify-content:center;min-width:0;font:10px system-ui}',
'.dircmp-center span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.dircmp-center button{font-size:11px;padding:1px 4px}',
'.dircmp-name[data-status="left-only"],.dircmp-name[data-status="right-only"]{color:#ff777c;font-weight:600}',
'.dircmp-center[data-status="left-only"]{color:#e9b279}',
'.dircmp-center[data-status="right-only"]{color:#8ebceb}',
'.dircmp-name[data-status="type-mismatch"],.dircmp-center[data-status="type-mismatch"]{color:#e9a2cb}',
'.dircmp-name[data-status="important"],.dircmp-center[data-status="important"]{color:#ed948f}',
'.dircmp-name[data-status="unimportant"],.dircmp-center[data-status="unimportant"]{color:#88c6c9}',
'.dircmp-name[data-status="changed"],.dircmp-center[data-status="changed"]{color:#e2b76c}',
'.dircmp-name[data-status="same"],.dircmp-center[data-status="same"]{color:#85bba4}',
'.dircmp-legend{border-top:1px solid #323d4c;padding:6px 9px;font-size:10px;color:#98a8b9}',
'.dircmp-tab{display:inline-flex;align-items:center;white-space:nowrap;max-width:260px}',
'.dircmp-tab-label{display:block;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:215px}',
'.dircmp-modal-bg{position:fixed;inset:0;z-index:18000;background:#0009;display:grid;place-items:center}',
'.dircmp-modal{width:min(470px,95vw);padding:17px;background:#192331;border:1px solid #526476;border-radius:9px}',
'.dircmp-modal label{display:block;padding:8px 0;font-size:12px}.dircmp-modal h3{margin:0 0 10px;font-size:16px}',
'.dircmp-modal-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:12px}',
'html[data-taskmenu-theme="light"] .dircmp-pane{background:#fff}',
'html[data-taskmenu-theme="light"] .dircmp-head{background:#eaf1f7}',
'html[data-taskmenu-theme="light"] .dircmp-row{border-color:#e6eaee}',
'html[data-taskmenu-theme="light"] .dircmp-row:hover{background:#f0f5fa}',
'html[data-taskmenu-theme="light"] .dircmp-modal{background:#fff;color:#27313c}',
'html[data-taskmenu-theme="light"] .dircmp-name[data-status="left-only"],html[data-taskmenu-theme="light"] .dircmp-name[data-status="right-only"]{color:#b42335}'
].join('\n');
document.head.append(css);
let tab=null,session=null,choice=null,controller=null,serial=0,showIdentical=true,collapsed=new Set(),renderedRows=800;
const pane=document.createElement('div');pane.className='pane dircmp-pane hidden';pane.dataset.id=TAB_ID;
const root=document.createElement('div');root.className='dircmp-root';
const toolbar=document.createElement('div');toolbar.className='dircmp-tools';
const heading=document.createElement('strong');heading.textContent='Directory Compare';
const scanBtn=document.createElement('button');scanBtn.textContent='Rescan structure';
const compareBtn=document.createElement('button');compareBtn.textContent='Compare files…';
const showLabel=document.createElement('label');const showCheck=document.createElement('input');showCheck.type='checkbox';showCheck.checked=true;
showLabel.append(showCheck,document.createTextNode(' Show identical'));
const search=document.createElement('input');search.type='search';search.placeholder='Filter paths…';search.title='Find files and folders';search.style.maxWidth='145px';
const statusFilter=document.createElement('select');statusFilter.title='Filter difference type';
for(const [value,label] of [['all','All types'],['left-only','Left only'],['right-only','Right only'],['changed','Checksum/size changed'],['important','Important edits'],['unimportant','Minor edits'],['type-mismatch','Type mismatch'],['unknown','Not checked / errors']]){
 const option=document.createElement('option');option.value=value;option.textContent=label;statusFilter.append(option);
}
const exportBtn=document.createElement('button');exportBtn.textContent='Export JSON';exportBtn.title='Export the current comparison report';
const status=document.createElement('span');status.className='dircmp-status';
const cancelBtn=document.createElement('button');cancelBtn.textContent='Cancel';cancelBtn.disabled=true;
toolbar.append(heading,scanBtn,compareBtn,showLabel,statusFilter,search,exportBtn,status,cancelBtn);
const header=document.createElement('div');header.className='dircmp-head';
const leftHead=document.createElement('div'),middleHead=document.createElement('div'),rightHead=document.createElement('div');
middleHead.textContent='Status / Copy';header.append(leftHead,middleHead,rightHead);
const tree=document.createElement('div');tree.className='dircmp-tree';
const legend=document.createElement('div');legend.className='dircmp-legend';
legend.textContent='Red filename: exists on one side only (other side blank) · Orange status: left only · Blue status: right only · Red differences: important · Teal: minor · Amber: checksum differs · Double-click a file for text Diff.';
root.append(toolbar,header,tree,legend);pane.append(root);panes.append(pane);

function sourceProject(path){return {kind:'project',path:String(path||'.'),label:'Host · '+path};}
function sourceGit(repoID,ref){const name=String(ref||'HEAD').trim();return {kind:'git',repoID:String(repoID||'').trim(),ref:name,path:name,label:'Git · '+name};}
function sourceRemote(profileID,path){return {kind:'remote',profileID:String(profileID),path:String(path||'.'),label:'Remote · '+path};}
function sourceBrowser(handle,path,options={}){return {kind:'browser',handle,rootHandle:Boolean(options.rootHandle),profileID:String(options.profileID||''),rootID:String(options.rootID||''),path:String(path||'.'),label:'Local · '+path};}
function relativeJoin(a,b){const base=String(a||'.');return (base==='.'?'':base==='/'?'/':base.replace(/\/+$/,'')+'/')+b;}
function descriptor(s){
 if(s?.kind==='project')return {kind:'project',path:s.path};
 if(s?.kind==='git')return {kind:'git',repoID:s.repoID,ref:s.ref,path:s.ref};
 if(s?.kind==='remote')return {kind:'remote',path:s.path,profileID:s.profileID};
 if(s?.kind==='browser'&&s.rootHandle&&s.profileID&&s.rootID)return {kind:'browser-transfer',path:s.path,profileID:s.profileID,rootID:s.rootID};
 return null;
}
function rehydrate(s){
 if(s?.kind==='project')return sourceProject(s.path);
 if(s?.kind==='git'&&s.repoID&&s.ref)return sourceGit(s.repoID,s.ref);
 if(s?.kind==='remote')return sourceRemote(s.profileID,s.path);
 if(s?.kind==='browser-transfer'){
  const view=globalThis.TaskMenuFileTransfer?.views?.get?.(s.profileID);
  if(view?.left?.localRoot?.handle&&view.left.localRoot.id===s.rootID)return sourceBrowser(view.left.localRoot.handle,s.path,{rootHandle:true,profileID:s.profileID,rootID:s.rootID});
 }
 return null;
}
function touch(){globalThis.TaskMenuWorkspaceTabs?.scheduleSave?.();}
function ensureTab(){
 if(tab)return;
 tab=document.createElement('button');tab.type='button';tab.className='tab dircmp-tab';tab.dataset.id=TAB_ID;tab.dataset.viewKind='directory-compare';
 const label=document.createElement('span');label.className='dircmp-tab-label';
 const close=document.createElement('span');close.className='close';close.textContent='×';close.title='Close Directory Compare';
 close.onclick=e=>{e.stopPropagation();closeTab();};
 tab.append(label,close);tab.onclick=()=>app.activateExternalView(TAB_ID,{force:true});
 tabs.append(tab);globalThis.TaskMenuTabContext?.registerTab?.(tab);
}
function setTitles(){
 if(!session||!tab)return;
 const basename=s=>String(s.path||'').replace(/\/+$/,'').split(/[/\\]/).pop()||'root';
 tab.querySelector('.dircmp-tab-label').textContent=basename(session.left)+' ↔ '+basename(session.right);
 tab.title=session.left.label+' ↔ '+session.right.label;
 leftHead.textContent=session.left.label;leftHead.title=session.left.label;
 rightHead.textContent=session.right.label;rightHead.title=session.right.label;
}
function cancelWork(){
 if(controller){controller.abort();controller=null;}
 cancelBtn.disabled=true;compareBtn.disabled=false;scanBtn.disabled=false;
}
function closeTab(){
 cancelWork();serial++;session=null;tab?.remove();tab=null;tree.replaceChildren();pane.classList.add('hidden');
 if(String(app.active||'')==='external:'+TAB_ID)tabs.querySelector('[data-id]')?.click();
 touch();
}
function validName(v){const s=String(v||'');return Boolean(s)&&s!=='.'&&s!=='..'&&!s.includes('/')&&!s.includes('\\')&&!s.includes('\0');}
function kind(e){return e?.type==='dir'||e?.type==='directory'?'dir':e?.type==='file'?'file':'other';}
function assertActive(signal){if(signal.aborted)throw new DOMException('Comparison cancelled','AbortError');}
async function listDir(s,path,signal,handle){
 assertActive(signal);
 if(s.kind==='project'){const entries=await app.jsonFetch('/api/project/tree?path='+encodeURIComponent(path),{signal});return {entries:Array.isArray(entries)?entries:[]};}
 if(s.kind==='remote'){
  const r=await callWithRemotePoolRetry(()=>app.jsonFetch('/api/file-transfer/list',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:s.profileID,path}),signal}),signal);
  return {entries:Array.isArray(r?.entries)?r.entries:[]};
 }
 if(s.kind==='browser'){
  if(!handle||handle.kind!=='directory')throw new Error('Browser local directory permission unavailable');
  const entries=[],handles=new Map();
  for await(const [name,h] of handle.entries()){
   assertActive(signal);handles.set(name,h);const info=h.kind==='file'?await h.getFile():null;
   entries.push({name,type:h.kind==='directory'?'dir':'file',size:info?.size||0,modified:info?.lastModified||''});
  }
  return {entries,handles};
 }
 throw new Error('Unknown directory source');
}
async function scanTree(source,signal,which){
 if(source.kind==='git'){
  assertActive(signal);
  const params=new URLSearchParams({view:'compare-tree',repo:source.repoID,ref:source.ref});
  const data=await app.jsonFetch('/api/git/status?'+params.toString(),{signal});
  assertActive(signal);
  if(data.repo_id!==source.repoID||!data.commit||!Array.isArray(data.entries))throw new Error('Git tree response does not match selected repository');
  source.sha=data.commit;source.label='Git · '+source.ref+' @ '+data.commit.slice(0,12);
  const found=new Map();
  for(const item of data.entries){
   const rel=String(item.path||'');
   if(!rel||found.has(rel))continue;
   found.set(rel,{rel,path:rel,kind:item.kind,mode:String(item.mode||''),oid:String(item.oid||''),gitlink:String(item.mode||'')==='160000'});
  }
  status.textContent='Scanning '+which+' · '+found.size+' Git tree entries';
  return found;
 }
 let initialHandle=source.handle;
 if(source.kind==='browser'&&source.rootHandle){
  for(const part of String(source.path||'.').split('/').filter(part=>part&&part!=='.')){
   initialHandle=await initialHandle.getDirectoryHandle(part);
  }
 }
 const found=new Map(),stack=[{path:source.path,rel:'',depth:0,handle:initialHandle}];let folders=0;
 while(stack.length){
  assertActive(signal);
  const dir=stack.pop();if(dir.depth>MAX_DEPTH)throw new Error('Maximum scan depth exceeded');
  const listing=await listDir(source,dir.path,signal,dir.handle);folders++;
  for(const item of listing.entries){
   if(!validName(item.name)||kind(item)==='other')continue;
   if(found.size>=MAX_ENTRIES)throw new Error(which+' directory exceeds '+MAX_ENTRIES+' entries. Choose a smaller subtree.');
   const rel=dir.rel?dir.rel+'/'+item.name:item.name;
   const entry={rel,path:relativeJoin(dir.path,item.name),kind:kind(item),size:Number(item.size)||0,modified:String(item.modified||''),handle:listing.handles?.get(item.name)};
   found.set(rel,entry);
   if(entry.kind==='dir')stack.push({path:entry.path,rel,depth:dir.depth+1,handle:entry.handle});
  }
  status.textContent='Scanning '+which+' · '+folders+' folders · '+found.size+' entries';
  if(folders%12===0)await new Promise(resolve=>setTimeout(resolve,0));
 }
 return found;
}
function sharesRemotePool(a,b){return a?.kind==='remote'&&b?.kind==='remote'&&a.profileID===b.profileID;}
async function callWithRemotePoolRetry(run,signal){
 for(let attempt=0;attempt<4;attempt++){
  assertActive(signal);
  try{return await run();}catch(error){
   if(!/(429|pool full|connection pool)/i.test(String(error?.message||error))||attempt===3)throw error;
   await new Promise(resolve=>setTimeout(resolve,300*(attempt+1)));
  }
 }
}
function mergeTrees(a,b){
 const paths=new Set([...a.keys(),...b.keys()]);
 return [...paths].sort((x,y)=>x.localeCompare(y,undefined,{numeric:true,sensitivity:'base'})).map(path=>{
  const left=a.get(path)||null,right=b.get(path)||null;
  const state=!left?'right-only':!right?'left-only':left.kind!==right.kind||Boolean(left.gitlink)!==Boolean(right.gitlink)?'type-mismatch':
    left.oid&&right.oid?(left.oid===right.oid&&left.mode===right.mode?'same':'changed'):
    left.kind==='dir'?'same':left.size!==right.size?'changed':'unknown';
  return {path,left,right,kind:left?.kind||right?.kind,state,detail:''};
 });
}
function propagateChanges(rows){
 const byPath=new Map(rows.map(r=>[r.path,r]));
 for(const row of [...rows].sort((a,b)=>b.path.length-a.path.length)){
  if(row.state==='same'||row.state==='unknown')continue;
  const ix=row.path.lastIndexOf('/');if(ix<0)continue;
  const parent=byPath.get(row.path.slice(0,ix));if(parent?.kind==='dir'&&parent.state==='same')parent.state='changed';
 }
}
const stateLabel={'left-only':'Left only','right-only':'Right only','type-mismatch':'Type mismatch',changed:'Different',important:'Important',unimportant:'Minor',same:'Same',unknown:'Not checked'};
function visibleRows(){
 if(!session)return [];
 const keep=new Set();
 for(const row of session.rows){
  const filter=statusFilter.value;
  const wanted=filter==='all'||row.state===filter||(filter==='changed'&&row.state==='changed');
  const matchPath=!search.value||row.path.toLowerCase().includes(search.value.toLowerCase());
  if(wanted&&matchPath&&(showIdentical||!['same','unknown'].includes(row.state)||(row.state==='unknown'&&row.detail))){
   keep.add(row.path);
   const pieces=row.path.split('/');for(let i=1;i<pieces.length;i++)keep.add(pieces.slice(0,i).join('/'));
  }
 }
 return session.rows.filter(row=>{
  if(!keep.has(row.path))return false;
  const parts=row.path.split('/');
  for(let i=1;i<parts.length;i++)if(collapsed.has(parts.slice(0,i).join('/')))return false;
  return true;
 });
}
function compareFileSource(src,entry){
 const api=globalThis.TaskMenuFileCompare;
 if(!entry)return api.emptyCompareSource('File absent');
 if(src.kind==='project')return api.projectSource(entry.path);
 if(src.kind==='remote')return api.remoteSource(src.profileID,entry.path);
 return api.browserFileHandleSource(entry.handle,{path:entry.path,label:'Local · '+entry.path});
}
function openFileDiff(row){
 if(!session||row.kind!=='file'||row.left?.kind==='dir'||row.right?.kind==='dir')return;
 return globalThis.TaskMenuFileCompare.open({title:'Directory diff',left:compareFileSource(session.left,row.left),right:compareFileSource(session.right,row.right)});
}
function makeCell(row,side){
 const entry=row[side],node=document.createElement('div');node.className='dircmp-cell';
 const depth=Math.min(48,row.path.split('/').length-1);
 node.style.paddingLeft=(6+depth*12)+'px';
 // Keep aligned cells, but never invent a file/folder on the absent side.
 // An empty cell also must not expose click/open handlers.
 if(!entry)return node;
 const icon=document.createElement('span');icon.className='dircmp-icon';icon.dataset.status=row.state;icon.textContent=entry.kind==='dir'?(collapsed.has(row.path)?'▸':'▾'):'▣';
 const name=document.createElement('span');name.className='dircmp-name';name.textContent=row.path.split('/').at(-1);name.dataset.status=row.state;
 name.title=(entry?.path||'Absent')+' · '+(stateLabel[row.state]||row.state)+(row.detail?' · '+row.detail:'');
 node.append(icon,name);
 if(row.kind==='dir'){node.style.cursor='pointer';node.onclick=()=>{collapsed.has(row.path)?collapsed.delete(row.path):collapsed.add(row.path);render();touch();};}
 else if(entry?.kind==='file'){node.style.cursor='pointer';node.ondblclick=()=>Promise.resolve(openFileDiff(row)).catch(app.showError);}
 return node;
}
function render(){
 tree.replaceChildren();if(!session)return;
 const fragment=document.createDocumentFragment();
 const rows=visibleRows();
 for(const row of rows.slice(0,renderedRows)){
  const div=document.createElement('div');div.className='dircmp-row';
  const mid=document.createElement('div');mid.className='dircmp-center';mid.dataset.status=row.state;
  const caption=document.createElement('span');caption.textContent=stateLabel[row.state]||row.state;caption.title=row.detail||caption.textContent;mid.append(caption);
  if(row.kind==='file'&&row.state!=='same'){
   for(const dir of ['left','right'])if(row[dir]?.kind==='file'){
    const btn=document.createElement('button');btn.textContent=dir==='left'?'→':'←';btn.title='Copy '+dir+' file to the other side';
    btn.onclick=()=>copyRow(row,dir).catch(app.showError);mid.append(btn);
   }
  }
  div.append(makeCell(row,'left'),mid,makeCell(row,'right'));fragment.append(div);
 }
 tree.append(fragment);
 if(rows.length>renderedRows){
  const more=document.createElement('button');more.textContent='Show more · '+(rows.length-renderedRows)+' remaining';
  more.style.margin='10px';more.onclick=()=>{renderedRows+=800;render();};tree.append(more);
 }
 if(!tree.childNodes.length){const empty=document.createElement('div');empty.className='dircmp-cell';empty.textContent='No differences in current view';tree.append(empty);}
 const counts={};for(const r of session.rows)counts[r.state]=(counts[r.state]||0)+1;
 status.textContent=(session.mode==='structure'?'Structure scan':session.mode+' compare')+' · '+session.rows.length+' items · '+Object.entries(counts).map(([k,v])=>k+' '+v).join(', ');
}
async function structureScan(){
 if(!session)return;
 cancelWork();const generation=++serial,ctrl=new AbortController();controller=ctrl;cancelBtn.disabled=false;compareBtn.disabled=true;scanBtn.disabled=true;
 try{
  const [a,b]=sharesRemotePool(session.left,session.right)?
   [await scanTree(session.left,ctrl.signal,'left'),await scanTree(session.right,ctrl.signal,'right')]:
   await Promise.all([scanTree(session.left,ctrl.signal,'left'),scanTree(session.right,ctrl.signal,'right')]);
  if(generation!==serial)return;
  session.rows=mergeTrees(a,b);session.mode='structure';showIdentical=true;showCheck.checked=true;collapsed.clear();render();touch();
 }catch(err){if(err.name!=='AbortError'&&generation===serial){status.textContent='Scan failed: '+err.message;app.showError(err);}}
 finally{if(generation===serial)cancelWork();}
}
function chooseMode(){
 return new Promise(resolve=>{
  const overlay=document.createElement('div');overlay.className='dircmp-modal-bg';
  const modal=document.createElement('div');modal.className='dircmp-modal';modal.setAttribute('role','dialog');modal.setAttribute('aria-modal','true');
  const heading=document.createElement('h3');heading.textContent='Compare files inside directories';
  const hint=document.createElement('p');hint.textContent='Structure was scanned separately. Checksum modes only detect equality; Content Compare first checks SHA-256, then classifies different source code.';hint.style.fontSize='11px';
  modal.append(heading,hint);
  const choices=[['crc32','CRC32 · same/different only'],['md5','MD5 · same/different only'],['sha256','SHA-256 · same/different only'],['content','Content Compare · SHA-256 + code diff details']];
  let first=null;
  for(const [value,label] of choices){
   const row=document.createElement('label'),radio=document.createElement('input');radio.type='radio';radio.name='dircmp-method';radio.value=value;
   if(value==='md5'&&(session.left.kind==='browser'||session.right.kind==='browser')){radio.disabled=true;row.title='Browser-local MD5 is unavailable: use SHA-256 or CRC32';}
   if(!first){first=radio;radio.checked=true;}
   row.append(radio,document.createTextNode(' '+label));modal.append(row);
  }
  const foot=document.createElement('div');foot.className='dircmp-modal-actions';
  const cancel=document.createElement('button');cancel.textContent='Cancel';
  const proceed=document.createElement('button');proceed.textContent='Compare';
  function done(value){overlay.remove();resolve(value);}
  cancel.onclick=()=>done(null);
  proceed.onclick=()=>done(modal.querySelector('input[name="dircmp-method"]:checked')?.value||'sha256');
  overlay.addEventListener('pointerdown',e=>{if(e.target===overlay)done(null);});
  overlay.addEventListener('keydown',e=>{if(e.key==='Escape')done(null);});
  foot.append(cancel,proceed);modal.append(foot);overlay.append(modal);document.body.append(overlay);first.focus();
 });
}
async function digest(src,entry,algo,signal){
 assertActive(signal);
 if(src.kind==='browser'){
  const file=await entry.handle.getFile();
  if(file.size>64*1024*1024)throw new Error('Browser-local checksum limited to 64 MiB');
  if(algo==='md5')throw new Error('MD5 is unavailable for local-browser sources; choose SHA-256 or CRC32');
  const bytes=new Uint8Array(await file.arrayBuffer());assertActive(signal);
  if(algo==='crc32'){
   let crc=-1;for(const b of bytes){crc^=b;for(let i=0;i<8;i++)crc=crc&1?(crc>>>1)^0xedb88320:crc>>>1;}
   return ((crc^-1)>>>0).toString(16).padStart(8,'0');
  }
  const hash=await crypto.subtle.digest('SHA-256',bytes);
  return [...new Uint8Array(hash)].map(x=>x.toString(16).padStart(2,'0')).join('');
 }
 const data=await callWithRemotePoolRetry(()=>app.jsonFetch(src.kind==='project'?'/api/directory-compare/project-hash':'/api/directory-compare/remote-hash',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({source:src.kind,profile_id:src.profileID||'',path:entry.path,algorithm:algo}),signal}),signal);
 return data.digest;
}
async function detailDiff(row,signal){
 const api=globalThis.TaskMenuFileCompare;
 const l=compareFileSource(session.left,row.left),r=compareFileSource(session.right,row.right);
 const [a,b]=await Promise.all([l.load(),r.load()]);assertActive(signal);
 const left=String(a?.text??a?.content??''),right=String(b?.text??b?.content??'');
 if(left.length>1024*1024||right.length>1024*1024)throw new Error('Detailed text diff is limited to 1 MiB per side; checksum result remains valid');
 return api.analyzeTexts(left,right,row.path);
}
async function compareContents(mode){
 cancelWork();const generation=++serial,ctrl=new AbortController();controller=ctrl;
 cancelBtn.disabled=false;compareBtn.disabled=true;scanBtn.disabled=true;
 try{
  const files=session.rows.filter(row=>row.left?.kind==='file'&&row.right?.kind==='file');
  let pos=0,errors=0;
  for(const row of files){
   assertActive(ctrl.signal);
   if(row.left.size!==row.right.size){
    row.state='changed';row.detail='Size differs';
    if(mode==='content'){
     try{
      row.stats=await detailDiff(row,ctrl.signal);
      row.state=row.stats.important?'important':row.stats.unimportant?'unimportant':'changed';
      row.detail='+'+row.stats.added+' -'+row.stats.removed+' ~'+row.stats.modified+' · '+row.stats.important+' important / '+row.stats.unimportant+' minor';
     }catch(e){row.detail='Size differs; text diff unavailable: '+e.message;}
    }
   }
   else{
    try{
     const algorithm=mode==='content'?'sha256':mode;
     const [a,b]=sharesRemotePool(session.left,session.right)?
      [await digest(session.left,row.left,algorithm,ctrl.signal),await digest(session.right,row.right,algorithm,ctrl.signal)]:
      await Promise.all([digest(session.left,row.left,algorithm,ctrl.signal),digest(session.right,row.right,algorithm,ctrl.signal)]);
     if(a===b){row.state='same';row.detail=algorithm+' match';}
     else if(mode!=='content'){row.state='changed';row.detail=algorithm+' mismatch';}
     else{
      try{
       row.stats=await detailDiff(row,ctrl.signal);
       row.state=row.stats.important?'important':row.stats.unimportant?'unimportant':'changed';
       row.detail='+'+row.stats.added+' -'+row.stats.removed+' ~'+row.stats.modified+' · '+row.stats.important+' important / '+row.stats.unimportant+' minor';
      }catch(e){row.state='changed';row.detail='SHA-256 mismatch, text diff unavailable: '+e.message;}
     }
    }catch(e){if(e.name==='AbortError')throw e;row.state='unknown';row.detail='Hash error: '+e.message;errors++;}
   }
   status.textContent='Comparing '+(++pos)+'/'+files.length+' · '+row.path+(errors?' · errors: '+errors:'');
   if(pos%12===0)await new Promise(resolve=>setTimeout(resolve,0));
  }
  if(generation!==serial)return;
  propagateChanges(session.rows);session.mode=mode;
  showIdentical=false;showCheck.checked=false;render();touch();
 }catch(e){if(e.name!=='AbortError')app.showError(e);}
 finally{if(generation===serial)cancelWork();}
}
async function mkdirProjectParents(path){
 const parts=path.split('/');parts.pop();let dir='';
 for(const part of parts){
  dir=dir?dir+'/'+part:part;
  const parent=dir.split('/').slice(0,-1).join('/')||'.';
  const listed=await app.jsonFetch('/api/project/tree?path='+encodeURIComponent(parent));
  const existing=Array.isArray(listed)?listed.find(x=>x.name===part):null;
  if(existing){if(kind(existing)!=='dir')throw new Error('Destination parent is not a directory: '+dir);continue;}
  await app.jsonFetch('/api/project/mutate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'mkdir',path:dir})});
 }
}
function relativeParent(path){const index=path.lastIndexOf('/');return index<0?'':path.slice(0,index);}
function leafName(path){return path.split('/').at(-1);}
async function browserTargetDirectory(source,relative){
 let dir=source.handle;
 if(source.rootHandle){
  for(const name of String(source.path||'.').split('/').filter(x=>x&&x!=='.'))dir=await dir.getDirectoryHandle(name);
 }
 const parent=relativeParent(relative);
 for(const name of parent.split('/').filter(Boolean))dir=await dir.getDirectoryHandle(name,{create:true});
 return dir;
}
async function ensureRemoteParents(source,path){
 const rawRoot=String(source.path||'.');const root=rawRoot==='/'?'/':rawRoot.replace(/\/+$/,'');
 const parts=path.slice((root==='.'?0:root.length)).replace(/^\/+|\/+$/g,'').split('/');
 parts.pop();
 let here=root||'.';
 for(const part of parts){
  const data=await app.jsonFetch('/api/file-transfer/list',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:source.profileID,path:here})});
  const existing=(data?.entries||[]).find(e=>e.name===part);
  if(existing){
   if(kind(existing)!=='dir')throw new Error('Destination path is not a folder: '+part);
  }else{
   const next=relativeJoin(here,part);
   await app.jsonFetch('/api/file-transfer/mutate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:source.profileID,path:next,action:'mkdir',directory:true})});
  }
  here=relativeJoin(here,part);
 }
}
async function downloadSourceBlob(src,entry){
 if(entry.size>64*1024*1024)throw new Error('Direct cross-source copy is limited to 64 MiB; use Transfer Queue for large files.');
 if(src.kind==='browser')return entry.handle.getFile();
 let url;
 if(src.kind==='project')url='/api/project/download?path='+encodeURIComponent(entry.path);
 else if(src.kind==='remote'){
  const ticket=await app.jsonFetch('/api/file-transfer/download-ticket',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:src.profileID,path:entry.path})});
  url='/api/file-transfer/download?ticket='+encodeURIComponent(ticket.ticket);
 }
 const response=await app.fetchWithLease(url,{cache:'no-store'});
 if(!response.ok)throw new Error('Download source failed: '+(await response.text()));
 const limit=64*1024*1024;
 if(Number(response.headers.get('content-length')||0)>limit){await response.body?.cancel?.();throw new Error('Transfer response exceeds the 64 MiB direct copy limit');}
 if(!response.body?.getReader){
  const blob=await response.blob();if(blob.size>limit)throw new Error('Transfer response exceeds the direct copy limit');return blob;
 }
 const reader=response.body.getReader(),chunks=[];let bytes=0;
 try{
  for(;;){
   const part=await reader.read();if(part.done)break;
   bytes+=part.value.byteLength;
   if(bytes>limit){await reader.cancel();throw new Error('Direct copy exceeded 64 MiB. Use Transfer Queue for large files.');}
   chunks.push(part.value);
  }
 }finally{reader.releaseLock();}
 return new Blob(chunks);
}
async function writeBlobDestination(dst,row,blob){
 const path=relativeJoin(dst.path,row.path);
 if(dst.kind==='browser'){
  const parent=await browserTargetDirectory(dst,row.path);
  const handle=await parent.getFileHandle(leafName(row.path),{create:true});
  const stream=await handle.createWritable();
  try{await stream.write(blob);await stream.close();}
  catch(e){try{await stream.abort();}catch{}throw e;}
  return;
 }
 if(dst.kind==='remote'){
  await ensureRemoteParents(dst,path);
  const data=new FormData();
  data.append('profile_id',dst.profileID);
  data.append('path',path);
  data.append('file',blob,leafName(row.path));
  const response=await app.fetchWithLease('/api/file-transfer/upload',{method:'POST',body:data,cache:'no-store'});
  if(!response.ok)throw new Error('Remote upload failed: '+(await response.text()));
  return;
 }
 throw new Error('Host destination requires a server-managed transfer (use Transfer Queue).');
}
async function copyRow(row,from){
 const src=session[from],dst=session[from==='left'?'right':'left'];
 const file=row[from],existing=row[from==='left'?'right':'left'];
 if(!file||file.kind!=='file'||existing?.kind==='dir')throw new Error('Cannot copy this entry');
 const target=relativeJoin(dst.path,row.path);
 if(src.kind===dst.kind&&file.path===target&&((src.kind==='remote'&&src.profileID===dst.profileID)||src.kind==='project'||(src.kind==='browser'&&src.handle===dst.handle)))throw new Error('Source and destination are the same file');
 if(!confirm('Copy '+file.path+' → '+target+'?\n'+(existing?'This will OVERWRITE existing data where supported.':'This creates a missing file.')+'\nReview direction before proceeding.'))return;
 if(src.kind==='project'&&dst.kind==='project'){
  if(existing)throw new Error('Host-to-host overwrite is blocked: rename/remove destination explicitly first.');
  await mkdirProjectParents(target);
  await app.jsonFetch('/api/project/mutate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'copy',path:file.path,new_path:target})});
 }else if(src.kind==='project'&&dst.kind==='remote'){
  await ensureRemoteParents(dst,target);
  await app.jsonFetch('/api/file-transfer/host-to-remote',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:dst.profileID,host_path:file.path,remote_path:target})});
 }else if(src.kind==='remote'&&dst.kind==='project'){
  if(existing)throw new Error('Host overwrite is blocked. Use Transfer Queue conflict resolver.');
  await mkdirProjectParents(target);
  await app.jsonFetch('/api/file-transfer/remote-to-host',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:src.profileID,remote_path:file.path,host_dir:target.split('/').slice(0,-1).join('/')||'.',overwrite:false})});
 }else if(dst.kind==='remote'||dst.kind==='browser'){
  await writeBlobDestination(dst,row,await downloadSourceBlob(src,file));
 }else throw new Error('Browser-local to Host copying requires Transfer Queue or manual upload.');
 await structureScan();
}
async function open(left,right){
 if(!left||!right)throw new Error('Select two folders');
 cancelWork();serial++;session={left,right,rows:[],mode:'structure'};
 statusFilter.value='all';search.value='';renderedRows=800;
 ensureTab();setTitles();app.activateExternalView(TAB_ID,{force:true});await structureScan();return true;
}
function select(source){choice=source;return source;}
async function compareWithSelected(source){if(!choice){select(source);return false;}const first=choice;choice=null;return open(first,source);}
async function openSources(sources){if(!Array.isArray(sources)||sources.length!==2)throw new Error('Select two folders');choice=null;return open(sources[0],sources[1]);}
function snapshotState(){
 if(!session)return null;
 const left=descriptor(session.left),right=descriptor(session.right);
 return left&&right?{version:1,left,right,collapsed:[...collapsed].slice(0,200)}:null;
}
async function restoreState(saved){
 if(!saved||saved.version!==1||session)return false;
 const left=rehydrate(saved.left),right=rehydrate(saved.right);
 if(!left||!right)return false;
 await open(left,right);
 collapsed=new Set(Array.isArray(saved.collapsed)?saved.collapsed:[]);render();
 return true;
}
cancelBtn.onclick=()=>{cancelWork();status.textContent='Cancelled · prior completed results preserved';};
scanBtn.onclick=()=>structureScan().catch(app.showError);
compareBtn.onclick=async()=>{const mode=await chooseMode();if(mode)compareContents(mode).catch(app.showError);};
showCheck.onchange=()=>{showIdentical=showCheck.checked;renderedRows=800;render();touch();};
statusFilter.onchange=()=>{renderedRows=800;render();};
search.oninput=()=>{renderedRows=800;render();};
exportBtn.onclick=()=>{
 if(!session)return;
 const output={left:descriptor(session.left),right:descriptor(session.right),mode:session.mode,created_at:new Date().toISOString(),
  entries:session.rows.map(row=>({path:row.path,state:row.state,kind:row.kind,detail:row.detail,left_size:row.left?.size??null,right_size:row.right?.size??null,stats:row.stats||null}))};
 const url=URL.createObjectURL(new Blob([JSON.stringify(output,null,2)],{type:'application/json'}));
 const link=document.createElement('a');link.href=url;link.download='taskdeck-directory-compare.json';document.body.append(link);link.click();link.remove();
 setTimeout(()=>URL.revokeObjectURL(url),30000);
};
window.addEventListener('taskmenu:view-activated',event=>{
 const on=event.detail?.kind==='external'&&event.detail?.id===TAB_ID;
 if(tab)tab.classList.toggle('active',on);
 pane.classList.toggle('hidden',!on);
});
globalThis.TaskMenuDirectoryCompare={open,openSources,select,compareWithSelected,sourceProject,sourceRemote,sourceBrowser,snapshotState,restoreState,close:closeTab,get selected(){return choice;},get current(){return session;}};
