// Virtual project file editing: CodeMirror holds only a bounded line window.
const app=globalThis.TaskMenuApp,largeStates=new WeakMap();
const largeBuffer=110,largeWindowLines=440,largeChunkLines=100,largeMaxLines=720,largeLimit=2<<20;
const css=document.createElement('style');
css.textContent='.editor-large-nav{display:flex;align-items:center;gap:7px;padding:5px 9px;min-width:0;flex-wrap:wrap;background:#182430;color:#dce7f7;border-bottom:1px solid #435469;font:11px ui-monospace,monospace}.editor-large-nav input[type=number]{width:92px;max-width:20vw;background:#121a23;color:inherit;border:1px solid #65788a;padding:3px}.editor-large-nav input[type=range]{flex:1;min-width:100px}.editor-large-nav button{padding:3px 8px}.editor-large-nav .large-state{opacity:.8}.editor-large-nav .large-state.error{color:#ff8888}html[data-taskmenu-theme="light"] .editor-large-nav{background:#eaf1fa;color:#29374b}html[data-taskmenu-theme="light"] .editor-large-nav input[type=number]{background:white;color:#29374b}';
document.head.append(css);
function stateFor(view){return largeStates.get(view);}
function setStatus(state,msg,error=false){state.status.textContent=msg;state.status.classList.toggle('error',Boolean(error));}
function setReplaceBusy(state,busy){
 const view=state.view;
 view.largeReplaceBusy=Boolean(busy);
 state.nav.classList.toggle('replacing',Boolean(busy));
 state.nav.setAttribute('aria-busy',busy?'true':'false');
 if(busy){
  clearTimeout(view.autoSaveTimer);view.autoSaveTimer=null;
 }
 for(const input of state.nav.querySelectorAll('input,button'))input.disabled=Boolean(busy);
 for(const item of [view.reload,view.findInput,view.findCase,view.findPrevious,view.findNext,
                     view.findReplaceInput,view.findReplace,view.findReplaceAll]){
  if(item)item.disabled=Boolean(busy);
 }
 if(!view.closed){
  state.helpers.applyReadOnly(view);
  // applyReadOnly is authoritative for file permissions; busy is stronger.
  if(busy){
   for(const item of [view.findReplace,view.findReplaceAll,view.findReplaceInput])if(item)item.disabled=true;
  }else if(view.dirty){
   // Re-arm the existing auto-save policy only after the transaction settles.
   state.helpers.setDirty(view,true);
  }
 }
}
function absoluteLine(state){
 const cursor=state.view.cm.state.selection.main.head;
 return (state.window?.start_line||1)+state.view.cm.state.doc.lineAt(cursor).number-1;
}
function draftKey(view){
 return 'taskdeck:editor-large-draft:'+encodeURIComponent(String(app.taskData?.workspace||''))+':'+encodeURIComponent(String(app.currentUser?.user_id||'local'))+':'+encodeURIComponent(view.file.path);
}
function clearDraft(state){try{sessionStorage.removeItem(draftKey(state.view));}catch{}}
function stashDraft(state){
 if(!state.view.dirty||!state.window)return;
 const text=state.view.cm.state.doc.toString();
 if(new TextEncoder().encode(text).length>largeLimit)return;
 try{sessionStorage.setItem(draftKey(state.view),JSON.stringify({
  sha:state.window.sha256,offset:state.window.start_byte,end:state.window.end_byte,text
 }));}catch{}
}
function viewportRows(view){return Math.max(25,Math.min(100,Math.ceil((view.scroller?.clientHeight||650)/16)));}

function byteCount(text){return new TextEncoder().encode(text).length;}
function windowCount(w){return w.end_line-w.start_line+1;}
function shownLine(state){
 const cm=state.view.cm,sc=state.view.scroller;
 if(state.window&&cm.posAtCoords&&sc?.getBoundingClientRect){
  const box=sc.getBoundingClientRect();
  const pos=cm.posAtCoords({x:box.left+Math.min(90,box.width/2),y:box.top+6});
  if(Number.isInteger(pos))return Math.max(state.window.start_line,Math.min(state.window.end_line,state.window.start_line+cm.state.doc.lineAt(pos).number-1));
 }
 return absoluteLine(state);
}
function syncWindowControls(state){
 if(!state.window)return;
 const line=shownLine(state);
 state.number.value=String(line);state.slider.value=String(line);
 state.slider.max=String(Math.max(1,state.total));
 state.range.textContent='Lines '+state.window.start_line+'–'+state.window.end_line+' / '+state.total;
}
// Re-label the CM6 virtual line-number gutter. Unlike cursor numbering,
// labels are derived from each DOM gutter element's original LOCAL number.
function syncGutter(state){
 const gutter=state.view.cm.dom?.querySelector?.('.cm-lineNumbers');
 if(!gutter||!state.window)return;
 gutter.style.minWidth=Math.max(4,String(state.total).length+1)+'ch';
 for(const el of gutter.querySelectorAll('.cm-gutterElement')){
  if(el.style?.visibility==='hidden')continue;
  const raw=String(el.textContent||'').trim();
  const local=/^\d+$/.test(raw)&&raw!==el.dataset.largeShown?Number(raw):Number(el.dataset.largeLocal);
  if(!Number.isInteger(local)||local<1)continue;
  el.dataset.largeLocal=String(local);
  const value=String(state.window.start_line+local-1);
  if(raw!==value)el.textContent=value;
  el.dataset.largeShown=value;
 }
}
function scheduleVisual(state){
 if(state.visualFrame!=null)return;
 const raf=globalThis.requestAnimationFrame||((fn)=>setTimeout(fn,0));
 state.visualFrame=raf(()=>{
  state.visualFrame=null;
  if(state.view.closed||!state.window)return;
  syncWindowControls(state);syncGutter(state);
 });
}
function scrollAnchor(state){
 const cm=state.view.cm,sc=state.view.scroller;
 if(!sc?.getBoundingClientRect||!cm.posAtCoords||!cm.coordsAtPos)return null;
 const rect=sc.getBoundingClientRect(),pos=cm.posAtCoords({x:rect.left+Math.min(90,rect.width/2),y:rect.top+6});
 if(!Number.isInteger(pos))return null;
 const coords=cm.coordsAtPos(pos);
 return coords?{pos,top:coords.top}:null;
}
function preserveAnchor(state,anchor,pos,delta){
 const cm=state.view.cm,sc=state.view.scroller;
 if(anchor&&Number.isInteger(pos)&&cm.coordsAtPos){
  const coords=cm.coordsAtPos(Math.max(0,Math.min(cm.state.doc.length,pos)));
  if(coords&&Number.isFinite(coords.top)){sc.scrollTop+=coords.top-anchor.top;return;}
 }
 // Fallback for browsers with unavailable CM position measurement.
 sc.scrollTop=Math.max(0,sc.scrollTop+delta);
}
function sameFile(w,r){
 return w.sha256===r.sha256&&w.size===r.size&&
  String(w.mtime_ns_exact??w.mtime_ns)===String(r.mtime_ns_exact??r.mtime_ns)&&
  w.total_lines===r.total_lines;
}
async function cleanSHA(state){
 if(state.window.window_sha256)return state.window.window_sha256;
 if(!globalThis.crypto?.subtle){
  // HTTP clients outside secure contexts cannot use SubtleCrypto. Re-fetch
  // this exact clean byte range and reuse the backend SHA only if *every*
  // original byte and version still match; never save with an invented hash.
  const w=state.window,params=new URLSearchParams({path:state.view.file.path,
   start:String(w.start_line),count:String(windowCount(w))});
  const r=await app.jsonFetch('/api/project/file-window?'+params.toString(),{cache:'no-store'});
  if(!sameFile(w,r)||r.start_line!==w.start_line||r.end_line!==w.end_line||
     r.start_byte!==w.start_byte||r.end_byte!==w.end_byte||r.content!==state.cleanContent)
   throw Error('File content changed while preparing range Save');
  return r.window_sha256;
 }
 const bytes=new TextEncoder().encode(state.cleanContent);
 const hash=await globalThis.crypto.subtle.digest('SHA-256',bytes);
 return Array.from(new Uint8Array(hash),b=>b.toString(16).padStart(2,'0')).join('');
}
function mergeAdjacent(state,r,direction){
 const view=state.view,w=state.window,doc=view.cm.state.doc,old=doc.toString();
 if(!w||view.dirty||!sameFile(w,r))return false;
 // A newline-terminated file has one final empty line. It already exists
 // as CodeMirror's trailing logical line: no extra text must be appended.
 if(direction==='down'&&r.content===''&&r.start_line===w.end_line+1&&
    r.start_line===r.end_line&&r.end_line===state.total&&old.endsWith('\n')){
  w.end_line=r.end_line;w.end_byte=r.end_byte;
  scheduleVisual(state);return true;
 }
 if(!r.content)return false;
 const count=r.end_line-r.start_line+1;
 if(count<1)return false;
 const top=shownLine(state),anchor=scrollAnchor(state);
 const room=Math.max(0,windowCount(w)+count-largeMaxLines);
 const safe=direction==='down'
  ?Math.max(0,top-w.start_line-Math.max(35,viewportRows(view)))
  :Math.max(0,w.end_line-top-Math.max(35,viewportRows(view)));
 let drop=Math.min(room,safe,windowCount(w)-1);
 // A file with long lines can hit the 2 MiB editor cap before reaching the
 // line-count cap. Trim additional off-screen lines (never the visible anchor).
 const bytesNeeded=Math.max(0,byteCount(old)+byteCount(r.content)-largeLimit);
 const maxDrop=Math.min(safe,windowCount(w)-1);
 if(bytesNeeded){
  const removableBytes=n=>{
   if(!n)return 0;
   if(direction==='down')return byteCount(old.slice(0,doc.line(n+1).from));
   const first=w.end_line-n+1-w.start_line+1;
   return byteCount(old.slice(doc.line(first).from));
  };
  if(removableBytes(maxDrop)<bytesNeeded)return false;
  let low=drop,high=maxDrop;
  while(low<high){
   const middle=Math.floor((low+high)/2);
   if(removableBytes(middle)>=bytesNeeded)high=middle;
   else low=middle+1;
  }
  drop=low;
 }
 let changes,merged,anchorPos=null,delta=0,removed=0;
 let nextStart=w.start_line,nextEnd=w.end_line,nextStartByte=w.start_byte,nextEndByte=w.end_byte;
 if(direction==='down'){
  if(r.start_line!==w.end_line+1||r.start_byte!==w.end_byte||!old.endsWith('\n'))return false;
  const cut=drop?doc.line(drop+1).from:0;
  const fragment=old.slice(0,cut);
  if(fragment&&!fragment.endsWith('\n'))return false;
  removed=byteCount(fragment);
  changes=[{from:0,to:cut,insert:''},{from:doc.length,to:doc.length,insert:r.content}];
  merged=old.slice(cut)+r.content;
  anchorPos=anchor?anchor.pos-cut:null;
  delta=-drop*(view.scroller.scrollHeight/Math.max(1,doc.lines));
  nextStart+=drop;nextStartByte+=removed;
  nextEnd=r.end_line;nextEndByte=r.end_byte;
 }else{
  if(r.end_line!==w.start_line-1||r.end_byte!==w.start_byte||!r.content.endsWith('\n'))return false;
  const local=w.end_line-drop+1-w.start_line+1;
  const cut=drop?doc.line(local).from:doc.length;
  removed=byteCount(old.slice(cut));
  changes=[{from:0,to:0,insert:r.content},{from:cut,to:doc.length,insert:''}];
  merged=r.content+old.slice(0,cut);
  anchorPos=anchor?anchor.pos+r.content.length:null;
  delta=count*(view.scroller.scrollHeight/Math.max(1,doc.lines));
  nextStart=r.start_line;nextStartByte=r.start_byte;
  nextEnd-=drop;nextEndByte-=removed;
 }
 if(byteCount(merged)>largeLimit)return false;
 view.internalUpdate=true;
 try{view.cm.dispatch({changes});}finally{view.internalUpdate=false;}
 Object.assign(w,{start_line:nextStart,end_line:nextEnd,start_byte:nextStartByte,end_byte:nextEndByte,window_sha256:null});
 state.cleanContent=merged;
 preserveAnchor(state,anchor,anchorPos,delta);
 scheduleVisual(state);
 return true;
}
async function refill(state,direction){
 const view=state.view,w=state.window;
 if(!w||state.prefetch||state.loading||state.saving||state.replacing||view.dirty||view.closed)return;
 const available=direction==='down'?state.total-w.end_line:w.start_line-1;
 if(available<=0)return;
 const count=Math.min(largeChunkLines,available);
 const start=direction==='down'?w.end_line+1:w.start_line-count;
 const epoch=state.epoch;
 state.prefetch=(async()=>{
  try{
   let requested=count;
   while(requested>0){
    const begin=direction==='down'?start:w.start_line-requested;
    const query=new URLSearchParams({path:view.file.path,start:String(begin),count:String(requested)});
    const r=await app.jsonFetch('/api/project/file-window?'+query.toString(),{cache:'no-store'});
    if(view.closed||state.epoch!==epoch||state.window!==w||view.dirty)return;
    if(!sameFile(w,r)){setStatus(state,'File changed externally; Reload to continue',true);return;}
    if(mergeAdjacent(state,r,direction))return;
    // A few exceptionally long rows can exhaust the 2 MiB window budget.
    // Retry smaller adjacent chunks instead of abandoning smooth scrolling.
    if(requested===1)break;
    requested=Math.max(1,Math.floor(requested/2));
   }
   setStatus(state,'Unable to extend buffer safely; use Go to navigate',true);
  }catch(error){
   if(state.epoch===epoch)setStatus(state,'Buffer fetch: '+String(error?.message||error),true);
  }finally{state.prefetch=null;scheduleVisual(state);}
 })();
 return state.prefetch;
}
function maybeRefill(state){
 const sc=state.view.scroller,w=state.window;
 if(!w||!sc||!sc.clientHeight||state.loading||state.prefetch||state.saving||state.replacing||state.view.dirty)return;
 const threshold=Math.max(sc.clientHeight*1.8,90*sc.scrollHeight/Math.max(1,state.view.cm.state.doc.lines));
 if(sc.scrollHeight-sc.clientHeight-sc.scrollTop<threshold&&w.end_line<state.total)
  void refill(state,'down');
 else if(sc.scrollTop<threshold&&w.start_line>1)
  void refill(state,'up');
}

async function loadRange(state,target){
 const view=state.view;
 const start=Math.max(1,target-largeBuffer);
 const params=new URLSearchParams({path:view.file.path,start:String(start),
  count:String(Math.max(largeWindowLines,viewportRows(view)+largeBuffer*2))});
 const response=await app.jsonFetch('/api/project/file-window?'+params.toString(),{cache:'no-store'});
 if(!response?.streamed||!Number.isInteger(response.total_lines))throw Error('Invalid window response');
 return response;
}
function applyWindow(state,response,target){
 const view=state.view;
 view.internalUpdate=true;
 try{view.cm.dispatch({changes:{from:0,to:view.cm.state.doc.length,insert:response.content||''}});}
 finally{view.internalUpdate=false;}
 state.window=response;state.total=response.total_lines;state.cleanContent=response.content||'';
 Object.assign(view.file,{sha256:response.sha256,size:response.size,mtime_ns:response.mtime_ns,
  mtime_ns_exact:response.mtime_ns_exact,
  read_only:response.read_only,line_ending:response.line_ending||'lf',streamed:true,large_file:true});
 view.desiredLineEnding=view.file.line_ending;
 state.helpers.applyReadOnly(view);
 state.helpers.setDirty(view,false);
 const shown=Math.max(response.start_line,Math.min(response.end_line,target));

 const local=Math.max(1,Math.min(view.cm.state.doc.lines,shown-response.start_line+1));
 const pos=view.cm.state.doc.line(local).from;
 view.internalUpdate=true;
 try{view.cm.dispatch({selection:{anchor:pos},scrollIntoView:true});}finally{view.internalUpdate=false;}
 syncWindowControls(state);scheduleVisual(state);
 if(!state.draftChecked){
  state.draftChecked=true;
  let draft;try{draft=JSON.parse(sessionStorage.getItem(draftKey(view))||'null');}catch{}
  if(draft?.sha===response.sha256&&draft.offset===response.start_byte&&draft.end===response.end_byte&&typeof draft.text==='string'){
   view.internalUpdate=true;
   try{view.cm.dispatch({changes:{from:0,to:view.cm.state.doc.length,insert:draft.text}});}
   finally{view.internalUpdate=false;}
   state.helpers.setDirty(view,true);
   setStatus(state,'Recovered unsaved edits');
  }
 }
}
async function save(view,{forReplace=false}={}){
 const state=stateFor(view);
 if(!state)return view.file;
 if(state.replacing&&!forReplace)throw Error('Replace is running; Save is locked');
 if(!view.dirty)return view.file;
 if(state.saving)return state.saving;
 if(!state.window)throw Error('Large file window is not loaded');
 state.saving=(async()=>{
  const original=state.window;
  const sourceWindowSHA=await cleanSHA(state);
  let value=view.cm.state.doc.toString();
  // Preserve mixed line endings verbatim; never normalize an entire
  // edited region just because its underlying file has multiple formats.
  const ending=view.desiredLineEnding||original.line_ending;
  if(ending==='crlf')value=value.replace(/\r?\n/g,'\r\n');
  else if(ending==='lf')value=value.replace(/\r\n/g,'\n');
  if(new TextEncoder().encode(value).length>largeLimit)throw Error('Edited window exceeds 2 MiB');
  setStatus(state,'Saving edited range…');
  const response=await app.fetchWithLease('/api/project/file-window/save',{
   method:'PUT',cache:'no-store',headers:{'Content-Type':'application/json'},
   body:JSON.stringify({path:view.file.path,start_byte:original.start_byte,end_byte:original.end_byte,
    expected_size:original.size,expected_mtime_ns:original.mtime_ns,
    expected_mtime_ns_exact:String(original.mtime_ns_exact??original.mtime_ns),expected_sha256:original.sha256,
    expected_window_sha256:sourceWindowSHA,content:value})
  });
  if(!response.ok){
   const body=await response.text();
   throw Error((response.status===409?'External file conflict: ':'')+(body||response.statusText));
  }
  const result=await response.json();
  clearDraft(state);
  const target=absoluteLine(state);
  state.epoch++;
  const refreshed=await loadRange(state,target);
  applyWindow(state,refreshed,target);
  setStatus(state,result.history_warning||'Saved');
  return view.file;
 })();
 try{return await state.saving;}
 catch(error){stashDraft(state);setStatus(state,String(error?.message||error),true);throw error;}
 finally{state.saving=null;}
}
async function goTo(view,line,{force=false,forReplace=false}={}){
 const state=stateFor(view);
 if(!state||view.closed||(state.replacing&&!forReplace))return false;
 // The first server response supplies total_lines. Do not clamp a Go/jump
 // requested during initial loading to the placeholder total=1.
 const requested=Math.max(1,Math.floor(Number(line)||1));
 const target=state.window?Math.min(state.total,requested):requested;
 if(state.window&&!force&&target>=state.window.start_line&&target<=state.window.end_line){
  const local=Math.max(1,Math.min(view.cm.state.doc.lines,target-state.window.start_line+1));
  const pos=view.cm.state.doc.line(local).from;
  view.internalUpdate=true;
  try{view.cm.dispatch({selection:{anchor:pos},scrollIntoView:true});}finally{view.internalUpdate=false;}
  state.number.value=String(target);state.slider.value=String(target);
  return true;
 }
 state.target=target;state.epoch++;
 if(state.loading)return state.loading;
 state.loading=(async()=>{
  while(state.target!==null&&!view.closed){
   const wanted=state.target;state.target=null;
   view.cm.contentDOM.setAttribute('contenteditable','false');
   // Preserve normal Save/Discard/Cancel semantics. Scrolling never
   // silently commits an unsaved edit when Auto Save is disabled.
   if(view.dirty){
    const choice=state.helpers.dirtyCloseChoice?await state.helpers.dirtyCloseChoice(view):'cancel';
    if(choice==='cancel'){state.target=null;return false;}
    if(choice==='discard'){state.helpers.setDirty(view,false);clearDraft(state);}
    else if(choice==='save')await save(view);
    else {state.target=null;return false;}
   }
   setStatus(state,'Loading line '+wanted+'…');
   const response=await loadRange(state,wanted);
   if(state.target!==null)continue;
   applyWindow(state,response,wanted);
   setStatus(state,'Ready');
  }
  return true;
 })();
 try{return await state.loading;}
 catch(error){stashDraft(state);setStatus(state,String(error?.message||error),true);throw error;}
 finally{state.loading=null;if(!view.closed)state.helpers.applyReadOnly(view);}
}
async function find(view,query,matchCase=false,direction=1,{forReplace=false}={}){
 const state=stateFor(view);if(!state||(state.replacing&&!forReplace))return null;
 query=String(query||'');
 if(!query){setStatus(state,'Enter text');return null;}
 const start=direction<0?absoluteLine(state)-1:absoluteLine(state)+1;
 const params=new URLSearchParams({path:view.file.path,q:query,start:String(start),
  case:matchCase?'1':'0',reverse:direction<0?'1':'0'});
 const response=await app.jsonFetch('/api/project/file-window/search?'+params.toString(),{cache:'no-store'});
 if(!response.found){state.lastMatch=null;setStatus(state,'No matches');return null;}
 state.lastMatch={line:response.line,column:response.column,query,matchCase,sha256:state.window?.sha256};
 if(!await goTo(view,response.line,{forReplace})){state.lastMatch=null;return null;}
 const loaded=state.window;
 const local=response.line-loaded.start_line+1;
 if(local>=1&&local<=view.cm.state.doc.lines){
  const row=view.cm.state.doc.line(local),lineBytes=new TextEncoder().encode(row.text);
  // The backend returns a UTF-8 byte column, while CodeMirror addresses
  // UTF-16 positions. Convert exactly; toLowerCase() can change byte length.
  const startBytes=Math.max(0,Math.min(lineBytes.length,response.column));
  const prefix=new TextDecoder().decode(lineBytes.subarray(0,startBytes));
  const from=row.from+prefix.length;
  view.internalUpdate=true;
  try{view.cm.dispatch({selection:{anchor:from,head:from+query.length},scrollIntoView:true});}
  finally{view.internalUpdate=false;}
 }
 setStatus(state,'Match at line '+response.line);
 return {line:response.line,column:response.column};
}
async function replace(view,{all=false}={}){
 const state=stateFor(view);
 if(!state||view.closed)return 0;
 if(view.file.read_only||view.tabReadOnly)throw Error('File is read-only');
 const query=String(view.findInput?.value||'');
 const replacement=String(view.findReplaceInput?.value||'');
 const matchCase=Boolean(view.findCase?.checked);
 if(!query){view.findStatus.textContent='Enter text';return 0;}
 if(new TextEncoder().encode(query).length>256||/[\r\n]/.test(query))throw Error('Find term must be one line and at most 256 UTF-8 bytes');
 if(new TextEncoder().encode(replacement).length>65536)throw Error('Replacement exceeds 64 KiB');
 if(state.replacing)return state.replacing;
 // Acquire the edit lock synchronously, before any modal, Save, or network await.
 setReplaceBusy(state,true);
 state.epoch++; // invalidate any in-flight adjacent-buffer request
 state.replacing=(async()=>{
  if(state.loading)await state.loading;
  if(state.saving)await state.saving;
  if(view.closed)return 0;
  if(view.dirty){
   stashDraft(state);
   const choice=state.helpers.dirtyCloseChoice?await state.helpers.dirtyCloseChoice(view):'cancel';
   if(choice==='cancel'){
    setStatus(state,'Replace cancelled; unsaved edits preserved');
    return 0;
   }
   if(choice==='save'){
    await save(view,{forReplace:true});
    if(view.dirty)throw Error('Save did not finish; Replace cancelled to preserve unsaved edits');
   }else if(choice==='discard'){
    // Never discard a draft until a replacement window has loaded successfully.
    const target=Math.max(1,shownLine(state));
    const refreshed=await loadRange(state,target);
    if(view.closed)return 0;
    state.epoch++;
    state.draftChecked=true;
    applyWindow(state,refreshed,target);
    clearDraft(state);
   }else return 0;
  }
  if(view.closed)return 0;
  let current=state.window;
  if(!current)throw Error('File window is not ready');
  let match=state.lastMatch;
  if(!all&&(!match||match.query!==query||match.matchCase!==matchCase||match.sha256!==current.sha256)){
   match=await find(view,query,matchCase,1,{forReplace:true});
   if(!match)return 0;
   match=state.lastMatch;
  }
  if(!all&&!match)return 0;
  const target=all?shownLine(state):match.line;
  setStatus(state,all?'Replacing all matches…':'Replacing current match…');
  const response=await app.fetchWithLease('/api/project/file-window/replace',{
   method:'POST',cache:'no-store',headers:{'Content-Type':'application/json'},
   body:JSON.stringify({
    path:view.file.path,find:query,replacement,match_case:matchCase,all,
    line:all?0:match.line,column:all?0:match.column,
    expected_size:current.size,expected_mtime_ns:current.mtime_ns,
    expected_mtime_ns_exact:String(current.mtime_ns_exact??current.mtime_ns),expected_sha256:current.sha256
   })
  });
  if(!response.ok){
   const message=await response.text();
   throw Error((response.status===409?'File conflict: ':'')+(message||response.statusText));
  }
  const result=await response.json();
  state.lastMatch=null;
  if(result.replacements>0){
   state.epoch++;
   state.window=null;
   const refreshed=await loadRange(state,Math.max(1,target));
   applyWindow(state,refreshed,target);
  }
  view.findStatus.textContent=result.replacements?'Replaced '+result.replacements:'No matches';
  setStatus(state,result.replacements?'Replaced '+result.replacements:'No matches');
  return Number(result.replacements)||0;
 })();
 try{return await state.replacing;}
 catch(error){setStatus(state,String(error?.message||error),true);throw error;}
 finally{
  state.replacing=null;
  setReplaceBusy(state,false);
 }
}

async function reload(view){
 const state=stateFor(view);if(!state||state.replacing)return false;
 if(!view.dirty)clearDraft(state);
 state.window=null;state.draftChecked=true;state.epoch++;
 return goTo(view,Number(state.number.value)||1,{force:true});
}
function attach(view,helpers){
 if(!view.file?.streamed)return;
 const nav=document.createElement('div');nav.className='editor-large-nav';
 const prev=document.createElement('button');prev.type='button';prev.textContent='◀ Page';
 const next=document.createElement('button');next.type='button';next.textContent='Page ▶';
 const number=document.createElement('input');number.type='number';number.min='1';number.value='1';number.title='Absolute line number';
 const jump=document.createElement('button');jump.type='button';jump.textContent='Go';
 const slider=document.createElement('input');slider.type='range';slider.min='1';slider.max='1';slider.value='1';slider.title='Jump anywhere in the file';
 const range=document.createElement('span');range.className='large-range';
 const status=document.createElement('span');status.className='large-state';
 nav.append(prev,next,number,jump,slider,range,status);view.body.before(nav);
 const state={view,helpers,nav,number,slider,range,status,window:null,cleanContent:'',total:1,target:null,loading:null,prefetch:null,saving:null,replacing:null,lastMatch:null,epoch:0,visualFrame:null,gutterObserver:null,draftTimer:null,draftChecked:false};
 largeStates.set(view,state);
 view.history.hidden=true;view.outline.disabled=true;
 view.outline.title='Outline is unavailable in windowed mode';
 view.markdownPreview.hidden=true;view.structuredSelect.disabled=true;
 view.lineEndingSelect.disabled=true;view.encodingSelect.disabled=true;
 view.minimapToggle.disabled=true;
 view.largeFileBadge.title='Large file: current viewport with adjacent buffered lines. Navigation, search and save use the server.';
 // Do not accept keystrokes into the placeholder before the first window arrives.
 view.cm.contentDOM.setAttribute('contenteditable','false');
 prev.onclick=()=>goTo(view,shownLine(state)-viewportRows(view)).catch(app.showError);
 next.onclick=()=>goTo(view,shownLine(state)+viewportRows(view)).catch(app.showError);
 jump.onclick=()=>goTo(view,Number(number.value)).catch(app.showError);
 number.onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();jump.click();}};
 let sliderTimer=null;
 slider.oninput=()=>{number.value=slider.value;clearTimeout(sliderTimer);sliderTimer=setTimeout(()=>goTo(view,Number(slider.value)).catch(app.showError),100);};
 view.scroller?.addEventListener('scroll',()=>{
  if(!state.window)return;
  scheduleVisual(state);maybeRefill(state);
 },{passive:true});
 const gutter=view.cm.dom?.querySelector?.('.cm-lineNumbers');
 if(gutter&&typeof MutationObserver!=='undefined'){
  state.gutterObserver=new MutationObserver(()=>scheduleVisual(state));
  state.gutterObserver.observe(gutter,{childList:true,subtree:true,characterData:true});
 }
 view.cm.contentDOM.addEventListener('input',()=>{
  clearTimeout(state.draftTimer);
  state.draftTimer=setTimeout(()=>stashDraft(state),250);
 });
 setStatus(state,'Loading window…');
 goTo(view,1,{force:true}).catch(app.showError);
}
function dispose(view){
 const state=stateFor(view);if(!state)return;
 state.epoch++;
 clearTimeout(state.draftTimer);state.gutterObserver?.disconnect();
 if(state.visualFrame!==null&&globalThis.cancelAnimationFrame)globalThis.cancelAnimationFrame(state.visualFrame);
 if(view.dirty)stashDraft(state);else clearDraft(state);
 state.nav.remove();largeStates.delete(view);
}
globalThis.TaskMenuLargeEditor={attach,goTo,find,replace,save,reload,dispose,stateFor,
 absoluteLine:view=>{const state=stateFor(view);return state?absoluteLine(state):null;},
 visibleLine:view=>{const state=stateFor(view);return state?shownLine(state):null;}};
