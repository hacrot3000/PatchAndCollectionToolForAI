// TaskDeck Restricted Developer. UI is NOT a security boundary; server is.
(()=>{
'use strict';
const $=id=>document.getElementById(id);
const e={command:$('command'),run:$('run'),cmdStatus:$('command-status'),output:$('command-output'),
 file:$('file'),start:$('start'),count:$('count'),view:$('view'),copy:$('copy'),
 excerpt:$('source-output'),fileStatus:$('file-status'),editor:$('protected-editor'),
 save:$('save-lines'),clear:$('clear-lines')};
if(Object.values(e).some(v=>!v))return;
const state={lease:'',policy:null,visible:null,history:[],index:0,busy:false};
const say=(node,message)=>{node.textContent=String(message||'');};
async function resultOf(r){
 const type=r.headers.get('content-type')||'';
 const x=type.includes('application/json')?await r.json():{error:(await r.text()).slice(0,300)};
 if(!r.ok)throw Error(x.error||x.message||('HTTP '+r.status));
 return x;
}
async function ensureLease(){
 if(state.lease)return;
 const r=await fetch('/api/browser/lease',{method:'POST',credentials:'same-origin'});
 const x=await resultOf(r);if(!x.lease)throw Error('Browser lease not available');
 state.lease=String(x.lease);
}
async function post(path,payload){
 await ensureLease();
 const r=await fetch(path,{
  method:'POST',credentials:'same-origin',
  headers:{'Content-Type':'application/json','X-TaskMenu-Lease':state.lease},
  body:JSON.stringify(payload)
 });
 if(r.status===409)state.lease='';
 return resultOf(r);
}
function resetWindow(){
 state.visible=null;say(e.excerpt,'');e.editor.value='';
 e.editor.disabled=true;e.save.disabled=true;e.copy.disabled=true;
}
function textLines(text){
 const lines=String(text).split('\n');
 if(text.endsWith('\n'))lines.pop();
 return lines.map(line=>line.endsWith('\r')?line.slice(0,-1):line);
}
async function run(){
 if(state.busy)return;
 const cmd=e.command.value.trim();if(!cmd)return;
 state.busy=true;e.run.disabled=true;
 try{
  const response=await post('/api/restricted/command',{command:cmd});
  state.history.push(cmd);if(state.history.length>60)state.history.shift();
  state.index=state.history.length;
  say(e.output,response.output||'(no output)');
  say(e.cmdStatus,'Exit '+response.exit_code+(response.truncated?' · output truncated':''));
 }catch(error){say(e.cmdStatus,error.message);say(e.output,'')}
 finally{state.busy=false;e.run.disabled=false}
}
async function view(){
 resetWindow();
 const path=e.file.value.trim(),start=Number(e.start.value),count=Number(e.count.value);
 if(!path||!Number.isSafeInteger(start)||!Number.isSafeInteger(count)||start<1||
  count<1||count>Number(state.policy?.max_visible_lines||80)){
  say(e.fileStatus,'Invalid file, starting line or line count.');return;
 }
 e.view.disabled=true;
 try{
  const r=await post('/api/restricted/file-window',{path,start,count});
  const physical=textLines(r.content||'');
  if(physical.length!==r.end_line-r.start_line+1)throw Error('Inconsistent source window');
  state.visible={path:r.path,start:r.start_line,sha256:r.sha256,lines:physical};
  say(e.excerpt,r.content);
  e.editor.value=physical.join('\n');
  e.editor.disabled=r.editable===false;e.save.disabled=r.editable===false;e.copy.disabled=false;
  say(e.fileStatus,'Lines '+r.start_line+'-'+r.end_line+' of '+r.total_lines+
   ' · remaining file '+(r.remaining?.file??'?')+' / today '+(r.remaining?.today??'?'));
 }catch(error){say(e.fileStatus,error.message)}
 finally{e.view.disabled=false}
}
async function save(){
 if(!state.visible||state.busy)return;
 const snapshot=state.visible,lines=e.editor.value.split('\n');
 if(lines.length!==snapshot.lines.length||lines.some(x=>x.includes('\r')||x.includes('\0'))){
  say(e.fileStatus,'Keep the same number of physical lines; no newlines within lines.');return;
 }
 state.busy=true;e.save.disabled=true;
 try{
  const r=await post('/api/restricted/file-edit',{
   path:snapshot.path,start_line:snapshot.start,expected_sha256:snapshot.sha256,lines
  });
  resetWindow();
  say(e.fileStatus,(r.saved?'Saved. ':'No changes. ')+'Reopen the window before another edit.');
 }catch(error){say(e.fileStatus,error.message);e.save.disabled=false}
 finally{state.busy=false}
}
async function copy(){
 const selection=window.getSelection();
 if(!selection||selection.isCollapsed||!e.excerpt.contains(selection.anchorNode)||
  !e.excerpt.contains(selection.focusNode)){
  say(e.fileStatus,'Select a short excerpt inside the protected view.');return;
 }
 const text=selection.toString(),maxLines=Number(state.policy?.max_copy_lines||0);
 const maxBytes=Number(state.policy?.max_copy_bytes||0);
 if(!maxLines||!maxBytes||text.split('\n').length>maxLines||
   new TextEncoder().encode(text).length>maxBytes){
  say(e.fileStatus,'Copy limit: '+maxLines+' lines, '+maxBytes+' bytes.');return;
 }
 try{await navigator.clipboard.writeText(text);say(e.fileStatus,'Selection copied.')}
 catch(error){say(e.fileStatus,'Clipboard unavailable: '+error.message)}
}
e.run.addEventListener('click',run);
e.command.addEventListener('keydown',event=>{
 if(event.key==='Enter'){event.preventDefault();void run()}
 else if(event.key==='ArrowUp'||event.key==='ArrowDown'){
  if(!state.history.length)return;
  event.preventDefault();
  state.index=Math.max(0,Math.min(state.history.length,state.index+
   (event.key==='ArrowUp'?-1:1)));
  e.command.value=state.history[state.index]||'';
 }
});
e.view.addEventListener('click',view);
e.save.addEventListener('click',save);
e.copy.addEventListener('click',copy);
e.clear.addEventListener('click',()=>{resetWindow();say(e.fileStatus,'Window cleared.')});
// Source-bearing review controls call ONLY the reviewed, quota-charged APIs.
// Results live in transient DOM textContent; nothing is persisted or cached.
const review={
 searchFile:$('review-search-file'),searchStart:$('review-search-start'),
 searchCount:$('review-search-count'),searchQuery:$('review-search-query'),
 searchRun:$('review-search-run'),searchStatus:$('review-search-status'),
 searchOutput:$('review-search-output'),
 left:$('review-left'),right:$('review-right'),
 leftStart:$('review-left-start'),rightStart:$('review-right-start'),
 count:$('review-count'),compare:$('review-compare'),diff:$('review-diff'),
 status:$('review-status'),output:$('review-output')
};
const reviewReady=Object.values(review).every(Boolean);
function safeWindow(start,count){
 return Number.isSafeInteger(start)&&Number.isSafeInteger(count)&&start>=1&&
  count>=1&&count<=Number(state.policy?.max_visible_lines||80);
}
async function runReviewSearch(){
 if(!reviewReady||state.busy)return;
 const path=review.searchFile.value.trim()||e.file.value.trim();
 const start=Number(review.searchStart.value),count=Number(review.searchCount.value);
 const query=review.searchQuery.value;
 say(review.searchOutput,'');
 if(!path||!safeWindow(start,count)||query.length<2||
  new TextEncoder().encode(query).length>128||/[\r\n\0]/.test(query)){
  say(review.searchStatus,'Enter a file, valid line range, and exact search text (2-128 bytes).');return;
 }
 state.busy=true;review.searchRun.disabled=true;
 try{
  const response=await post('/api/restricted/search',{path,start,count,query});
  const matches=Array.isArray(response.matches)?response.matches:[];
  say(review.searchOutput,matches.map(m=>String(m.line)+': '+String(m.text||'')).join(''));
  say(review.searchStatus,'Searched '+response.scanned_lines+' quota-charged lines; '+matches.length+' match(es).');
 }catch(error){say(review.searchStatus,error.message)}
 finally{review.searchRun.disabled=false;state.busy=false}
}
async function runReviewWindows(kind){
 if(!reviewReady||state.busy)return;
 const left=review.left.value.trim(),right=review.right.value.trim();
 const left_start=Number(review.leftStart.value),right_start=Number(review.rightStart.value);
 const count=Number(review.count.value);
 say(review.output,'');
 if(!left||!right||!safeWindow(left_start,count)||!safeWindow(right_start,count)){
  say(review.status,'Specify both paths and valid bounded line ranges.');return;
 }
 state.busy=true;review.compare.disabled=true;review.diff.disabled=true;
 try{
  const response=await post('/api/restricted/'+kind,{left,right,left_start,right_start,count});
  if(kind==='diff'){
   const ops=Array.isArray(response.ops)?response.ops:[];
   say(review.output,ops.map(op=>{
    const marker=op.kind==='add'?'+':op.kind==='remove'?'-':' ';
    const text=String(op.text||'');
    return marker+' '+text+(text.endsWith('\n')?'':'\n');
   }).join(''));
   say(review.status,ops.length+' charged diff operations (equal/add/remove).');
  }else{
   const rows=Array.isArray(response.rows)?response.rows:[];
   say(review.output,rows.map(row=>
    (row.equal?'= ':'! ')+String(row.left_line||'-')+': '+String(row.left||'').trimEnd()+
    ' | '+String(row.right_line||'-')+': '+String(row.right||'').trimEnd()
   ).join('\n'));
   say(review.status,rows.length+' quota-charged position-aligned comparison rows.');
  }
 }catch(error){say(review.status,error.message)}
 finally{review.compare.disabled=false;review.diff.disabled=false;state.busy=false}
}
if(reviewReady){
 review.searchRun.addEventListener('click',runReviewSearch);
 review.compare.addEventListener('click',()=>runReviewWindows('compare'));
 review.diff.addEventListener('click',()=>runReviewWindows('diff'));
}
resetWindow();
(async()=>{
 try{
  const response=await fetch('/api/restricted/status',{credentials:'same-origin'});
  state.policy=await resultOf(response);
  e.count.max=String(state.policy.max_visible_lines);
  if(reviewReady){
   review.searchCount.max=String(state.policy.max_visible_lines);
   review.count.max=String(state.policy.max_visible_lines);
  }
  say(e.cmdStatus,state.policy?.ready===true?
   'Browser test ready · Protected '+(state.policy.policy_source==='restricted_role'?'role':'startup')+
   ' policy active. Only reviewed commands are permitted.':
   'Restricted policy is not yet enabled for browser testing.');
  say(e.fileStatus,'Maximum '+state.policy.max_visible_lines+' visible lines per request.');
 }catch(error){
  say(e.cmdStatus,'Restricted policy unavailable: '+error.message);
  e.run.disabled=true;e.view.disabled=true;
  if(reviewReady){review.searchRun.disabled=true;review.compare.disabled=true;review.diff.disabled=true;}
 }
})();
})();
