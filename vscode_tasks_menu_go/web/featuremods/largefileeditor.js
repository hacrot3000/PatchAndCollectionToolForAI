// Virtual project file editing: CodeMirror holds only a bounded line window.
const app=globalThis.TaskMenuApp,largeStates=new WeakMap();
const largeBuffer=60,largeWindowLines=180,largeLimit=2<<20;
const css=document.createElement('style');
css.textContent='.editor-large-nav{display:flex;align-items:center;gap:7px;padding:5px 9px;min-width:0;flex-wrap:wrap;background:#182430;color:#dce7f7;border-bottom:1px solid #435469;font:11px ui-monospace,monospace}.editor-large-nav input[type=number]{width:92px;max-width:20vw;background:#121a23;color:inherit;border:1px solid #65788a;padding:3px}.editor-large-nav input[type=range]{flex:1;min-width:100px}.editor-large-nav button{padding:3px 8px}.editor-large-nav .large-state{opacity:.8}.editor-large-nav .large-state.error{color:#ff8888}html[data-taskmenu-theme="light"] .editor-large-nav{background:#eaf1fa;color:#29374b}html[data-taskmenu-theme="light"] .editor-large-nav input[type=number]{background:white;color:#29374b}';
document.head.append(css);
function stateFor(view){return largeStates.get(view);}
function setStatus(state,msg,error=false){state.status.textContent=msg;state.status.classList.toggle('error',Boolean(error));}
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
 state.window=response;state.total=response.total_lines;
 Object.assign(view.file,{sha256:response.sha256,size:response.size,mtime_ns:response.mtime_ns,
  read_only:response.read_only,line_ending:response.line_ending||'lf',streamed:true,large_file:true});
 view.desiredLineEnding=view.file.line_ending;
 state.helpers.applyReadOnly(view);
 state.helpers.setDirty(view,false);
 const shown=Math.max(response.start_line,Math.min(response.end_line,target));
 state.number.value=String(shown);
 state.slider.max=String(Math.max(1,state.total));state.slider.value=String(shown);
 state.range.textContent='Lines '+response.start_line+'–'+response.end_line+' / '+state.total;
 const local=Math.max(1,Math.min(view.cm.state.doc.lines,shown-response.start_line+1));
 const pos=view.cm.state.doc.line(local).from;
 view.internalUpdate=true;
 try{view.cm.dispatch({selection:{anchor:pos},scrollIntoView:true});}finally{view.internalUpdate=false;}
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
async function save(view){
 const state=stateFor(view);
 if(!state||!view.dirty)return view.file;
 if(state.saving)return state.saving;
 if(!state.window)throw Error('Large file window is not loaded');
 state.saving=(async()=>{
  const original=state.window;
  let value=view.cm.state.doc.toString();
  value=(view.desiredLineEnding||original.line_ending)==='crlf'
   ?value.replace(/\r?\n/g,'\r\n'):value.replace(/\r\n/g,'\n');
  if(new TextEncoder().encode(value).length>largeLimit)throw Error('Edited window exceeds 2 MiB');
  setStatus(state,'Saving edited range…');
  const response=await app.fetchWithLease('/api/project/file-window/save',{
   method:'PUT',cache:'no-store',headers:{'Content-Type':'application/json'},
   body:JSON.stringify({path:view.file.path,start_byte:original.start_byte,end_byte:original.end_byte,
    expected_size:original.size,expected_mtime_ns:original.mtime_ns,expected_sha256:original.sha256,
    expected_window_sha256:original.window_sha256,content:value})
  });
  if(!response.ok){
   const body=await response.text();
   throw Error((response.status===409?'External file conflict: ':'')+(body||response.statusText));
  }
  const result=await response.json();
  clearDraft(state);
  const target=absoluteLine(state);
  const refreshed=await loadRange(state,target);
  applyWindow(state,refreshed,target);
  setStatus(state,result.history_warning||'Saved');
  return view.file;
 })();
 try{return await state.saving;}
 catch(error){stashDraft(state);setStatus(state,String(error?.message||error),true);throw error;}
 finally{state.saving=null;}
}
async function goTo(view,line,{force=false}={}){
 const state=stateFor(view);
 if(!state||view.closed)return false;
 const target=Math.max(1,Math.min(state.total||1,Math.floor(Number(line)||1)));
 if(state.window&&!force&&target>=state.window.start_line+12&&target<=state.window.end_line-12){
  const local=Math.max(1,Math.min(view.cm.state.doc.lines,target-state.window.start_line+1));
  const pos=view.cm.state.doc.line(local).from;
  view.internalUpdate=true;
  try{view.cm.dispatch({selection:{anchor:pos},scrollIntoView:true});}finally{view.internalUpdate=false;}
  state.number.value=String(target);state.slider.value=String(target);
  return true;
 }
 state.target=target;
 if(state.loading)return state.loading;
 state.loading=(async()=>{
  while(state.target!==null&&!view.closed){
   const wanted=state.target;state.target=null;
   // A dirty window must be persisted before it can be replaced.
   if(view.dirty)await save(view);
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
 finally{state.loading=null;}
}
async function find(view,query,matchCase=false,direction=1){
 const state=stateFor(view);if(!state)return null;
 query=String(query||'');
 if(!query){setStatus(state,'Enter text');return null;}
 const start=direction<0?1:absoluteLine(state)+1;
 const params=new URLSearchParams({path:view.file.path,q:query,start:String(start),case:matchCase?'1':'0'});
 const response=await app.jsonFetch('/api/project/file-window/search?'+params.toString(),{cache:'no-store'});
 if(!response.found){setStatus(state,'No matches');return null;}
 await goTo(view,response.line);
 setStatus(state,'Match at line '+response.line);
 return {line:response.line,column:response.column};
}
async function reload(view){
 const state=stateFor(view);if(!state)return false;
 state.window=null;state.draftChecked=true;
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
 const state={view,helpers,nav,number,slider,range,status,window:null,total:1,target:null,loading:null,saving:null,scrollTimer:null,draftTimer:null,draftChecked:false};
 largeStates.set(view,state);
 view.history.hidden=true;view.outline.disabled=true;
 view.outline.title='Outline is unavailable in windowed mode';
 view.markdownPreview.hidden=true;view.structuredSelect.disabled=true;
 view.lineEndingSelect.disabled=true;view.encodingSelect.disabled=true;
 view.minimapToggle.disabled=true;
 view.largeFileBadge.title='Large file: current viewport with adjacent buffered lines. Navigation, search and save use the server.';
 prev.onclick=()=>goTo(view,absoluteLine(state)-viewportRows(view)).catch(app.showError);
 next.onclick=()=>goTo(view,absoluteLine(state)+viewportRows(view)).catch(app.showError);
 jump.onclick=()=>goTo(view,Number(number.value)).catch(app.showError);
 number.onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();jump.click();}};
 let sliderTimer=null;
 slider.oninput=()=>{number.value=slider.value;clearTimeout(sliderTimer);sliderTimer=setTimeout(()=>goTo(view,Number(slider.value)).catch(app.showError),100);};
 view.scroller?.addEventListener('scroll',()=>{
  if(!state.window||state.loading||state.saving)return;
  clearTimeout(state.scrollTimer);
  state.scrollTimer=setTimeout(()=>{
   if(view.closed||!state.window||state.loading||state.saving)return;
   const sc=view.scroller,remaining=sc.scrollHeight-sc.clientHeight-sc.scrollTop;
   if(remaining<70&&state.window.end_line<state.total)
    goTo(view,Math.min(state.total,state.window.end_line-largeBuffer+viewportRows(view))).catch(app.showError);
   else if(sc.scrollTop<30&&state.window.start_line>1)
    goTo(view,Math.max(1,state.window.start_line-largeBuffer)).catch(app.showError);
  },130);
 },{passive:true});
 view.cm.contentDOM.addEventListener('input',()=>{
  clearTimeout(state.draftTimer);
  state.draftTimer=setTimeout(()=>stashDraft(state),250);
 });
 setStatus(state,'Loading window…');
 goTo(view,1,{force:true}).catch(app.showError);
}
function dispose(view){
 const state=stateFor(view);if(!state)return;
 clearTimeout(state.scrollTimer);clearTimeout(state.draftTimer);
 if(view.dirty)stashDraft(state);else clearDraft(state);
 state.nav.remove();largeStates.delete(view);
}
globalThis.TaskMenuLargeEditor={attach,goTo,find,save,reload,dispose,stateFor,
 absoluteLine:view=>{const state=stateFor(view);return state?absoluteLine(state):null;}};
