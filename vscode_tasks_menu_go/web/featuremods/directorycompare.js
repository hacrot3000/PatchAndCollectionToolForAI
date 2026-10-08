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
'.dircmp-cell .dircmp-icon{flex:none;min-width:12px}.dircmp-center{display:flex;gap:3px;align-items:center;justify-content:center;min-width:0;font:10px system-ui}',
'.dircmp-center span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.dircmp-center button{font-size:11px;padding:1px 4px}',
'.dircmp-name[data-status="left-only"],.dircmp-center[data-status="left-only"]{color:#e9b279}',
'.dircmp-name[data-status="right-only"],.dircmp-center[data-status="right-only"]{color:#8ebceb}',
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
'html[data-taskmenu-theme="light"] .dircmp-modal{background:#fff;color:#27313c}'
].join('\n');
document.head.append(css);
let tab=null,session=null,choice=null,controller=null,serial=0,showIdentical=true,collapsed=new Set();
const pane=document.createElement('div');pane.className='pane dircmp-pane hidden';pane.dataset.id=TAB_ID;
const root=document.createElement('div');root.className='dircmp-root';
const toolbar=document.createElement('div');toolbar.className='dircmp-tools';
const heading=document.createElement('strong');heading.textContent='Directory Compare';
const scanBtn=document.createElement('button');scanBtn.textContent='Rescan structure';
const compareBtn=document.createElement('button');compareBtn.textContent='Compare files…';
const showLabel=document.createElement('label');const showCheck=document.createElement('input');showCheck.type='checkbox';showCheck.checked=true;
showLabel.append(showCheck,document.createTextNode(' Show identical'));
const status=document.createElement('span');status.className='dircmp-status';
const cancelBtn=document.createElement('button');cancelBtn.textContent='Cancel';cancelBtn.disabled=true;
toolbar.append(heading,scanBtn,compareBtn,showLabel,status,cancelBtn);
const header=document.createElement('div');header.className='dircmp-head';
const leftHead=document.createElement('div'),middleHead=document.createElement('div'),rightHead=document.createElement('div');
middleHead.textContent='Status / Copy';header.append(leftHead,middleHead,rightHead);
const tree=document.createElement('div');tree.className='dircmp-tree';
const legend=document.createElement('div');legend.className='dircmp-legend';
legend.textContent='Orange: left only · Blue: right only · Red: important · Teal: minor · Amber: checksum differs · Double-click a file for text Diff.';
root.append(toolbar,header,tree,legend);pane.append(root);panes.append(pane);

function sourceProject(path){return {kind:'project',path:String(path||'.'),label:'Host · '+path};}
function sourceRemote(profileID,path){return {kind:'remote',profileID:String(profileID),path:String(path||'.'),label:'Remote · '+path};}
function sourceBrowser(handle,path){return {kind:'browser',handle,path:String(path||'.'),label:'Local · '+path};}
function relativeJoin(a,b){return (a==='.'?'':String(a).replace(/\/+$/,'')+'/')+b;}
function descriptor(s){return s?.kind==='project'?{kind:s.kind,path:s.path}:s?.kind==='remote'?{kind:s.kind,path:s.path,profileID:s.profileID}:null;}
function rehydrate(s){return s?.kind==='project'?sourceProject(s.path):s?.kind==='remote'?sourceRemote(s.profileID,s.path):null;}
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
  const r=await app.jsonFetch('/api/file-transfer/list',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:s.profileID,path}),signal});
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
 const found=new Map(),stack=[{path:source.path,rel:'',depth:0,handle:source.handle}];let folders=0;
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
function mergeTrees(a,b){
 const paths=new Set([...a.keys(),...b.keys()]);
 return [...paths].sort((x,y)=>x.localeCompare(y,undefined,{numeric:true,sensitivity:'base'})).map(path=>{
  const left=a.get(path)||null,right=b.get(path)||null;
  const state=!left?'right-only':!right?'left-only':left.kind!==right.kind?'type-mismatch':left.kind==='dir'?'same':'unknown';
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
  if(showIdentical||!['same','unknown'].includes(row.state)){
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
 const icon=document.createElement('span');icon.className='dircmp-icon';icon.textContent=entry?.kind==='dir'?(collapsed.has(row.path)?'▸':'▾'):entry?'▣':'·';
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
 for(const row of visibleRows()){
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
 if(!tree.childNodes.length){const empty=document.createElement('div');empty.className='dircmp-cell';empty.textContent='No differences in current view';tree.append(empty);}
 const counts={};for(const r of session.rows)counts[r.state]=(counts[r.state]||0)+1;
 status.textContent=(session.mode==='structure'?'Structure scan':session.mode+' compare')+' · '+session.rows.length+' items · '+Object.entries(counts).map(([k,v])=>k+' '+v).join(', ');
}
async function structureScan(){
 if(!session)return;
 cancelWork();const generation=++serial,ctrl=new AbortController();controller=ctrl;cancelBtn.disabled=false;compareBtn.disabled=true;scanBtn.disabled=true;
 try{
  const [a,b]=await Promise.all([scanTree(session.left,ctrl.signal,'left'),scanTree(session.right,ctrl.signal,'right')]);
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
 const data=await app.jsonFetch('/api/directory-compare/hash',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({source:src.kind,profile_id:src.profileID||'',path:entry.path,algorithm:algo}),signal});
 return data.digest;
}
async function detailDiff(row,signal){
 const api=globalThis.TaskMenuFileCompare;
 const l=compareFileSource(session.left,row.left),r=compareFileSource(session.right,row.right);
 const [a,b]=await Promise.all([l.load(),r.load()]);assertActive(signal);
 return api.analyzeTexts(String(a?.text??a?.content??''),String(b?.text??b?.content??''),row.path);
}
async function compareContents(mode){
 cancelWork();const generation=++serial,ctrl=new AbortController();controller=ctrl;
 cancelBtn.disabled=false;compareBtn.disabled=true;scanBtn.disabled=true;
 try{
  const files=session.rows.filter(row=>row.left?.kind==='file'&&row.right?.kind==='file');
  let pos=0,errors=0;
  for(const row of files){
   assertActive(ctrl.signal);
   if(row.left.size!==row.right.size){row.state='changed';row.detail='Size differs';}
   else{
    try{
     const algorithm=mode==='content'?'sha256':mode;
     const [a,b]=await Promise.all([digest(session.left,row.left,algorithm,ctrl.signal),digest(session.right,row.right,algorithm,ctrl.signal)]);
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
async function copyRow(row,from){
 const src=session[from],dst=session[from==='left'?'right':'left'];
 const file=row[from],existing=row[from==='left'?'right':'left'];
 if(!file||file.kind!=='file'||existing?.kind==='dir')throw new Error('Cannot copy this entry');
 const target=relativeJoin(dst.path,row.path);
 if(!confirm('Copy '+file.path+' → '+target+'?\n'+(existing?'This will OVERWRITE existing data where supported.':'This creates a missing file.')+'\nReview direction before proceeding.'))return;
 if(src.kind==='project'&&dst.kind==='project'){
  if(existing)throw new Error('Host-to-host overwrite is blocked: rename/remove destination explicitly first.');
  await mkdirProjectParents(target);
  await app.jsonFetch('/api/project/mutate',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'copy',path:file.path,new_path:target})});
 }else if(src.kind==='project'&&dst.kind==='remote'){
  if(existing)throw new Error('Remote overwrite from Directory Compare is blocked. Use Transfer Queue conflict resolver.');
  await app.jsonFetch('/api/file-transfer/host-to-remote',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:dst.profileID,host_path:file.path,remote_path:target})});
 }else if(src.kind==='remote'&&dst.kind==='project'){
  if(existing)throw new Error('Host overwrite is blocked. Use Transfer Queue conflict resolver.');
  await mkdirProjectParents(target);
  await app.jsonFetch('/api/file-transfer/remote-to-host',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({profile_id:src.profileID,remote_path:file.path,host_dir:target.split('/').slice(0,-1).join('/')||'.',overwrite:false})});
 }else throw new Error('This copy pairing needs Transfer Queue for binary-safe delivery; use that tool instead.');
 await structureScan();
}
async function open(left,right){
 if(!left||!right)throw new Error('Select two folders');
 cancelWork();serial++;session={left,right,rows:[],mode:'structure'};
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
showCheck.onchange=()=>{showIdentical=showCheck.checked;render();touch();};
window.addEventListener('taskmenu:view-activated',event=>{
 const on=event.detail?.kind==='external'&&event.detail?.id===TAB_ID;
 if(tab)tab.classList.toggle('active',on);
 pane.classList.toggle('hidden',!on);
});
globalThis.TaskMenuDirectoryCompare={open,openSources,select,compareWithSelected,sourceProject,sourceRemote,sourceBrowser,snapshotState,restoreState,close:closeTab,get selected(){return choice;},get current(){return session;}};
