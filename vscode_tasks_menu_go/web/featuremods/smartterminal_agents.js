// Reserved terminal commands never reach the user's shell as typed.
// AI calls reuse TaskDeck's existing verified-target, encrypted-key and quota APIs.
(function(){
'use strict';
const app=globalThis.TaskMenuApp,terminal=globalThis.TaskDeckSmartTerminal;
if(!app||!terminal)throw Error('Smart terminal agent prerequisites unavailable');
const states=new Map();
function state(view){let s=states.get(view.meta.id);if(!s){s={mode:'normal',candidate:'',line:'',busy:false,abort:null};states.set(view.meta.id,s);}return s;}
function safeText(value){return String(value??'').replace(/\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*\x07)?/g,'').replace(/[\x00-\x08\x0b-\x1f\x7f]/g,'');}
function print(view,message,kind='info'){
 if(view.closed)return;
 const color=kind==='error'?'91':kind==='answer'?'96':'93';
 const clean=safeText(message).slice(0,16500).replace(/\r?\n/g,'\r\n');
 view.term.write('\r\n\x1b[1;'+color+'m[TaskDeck '+(kind==='answer'?'AI':kind==='error'?'ERROR':'Agent')+']\x1b[0m '+clean+'\r\n');
}
function reset(s){s.mode='normal';s.candidate='';s.line='';}
function resume(view){if(!view.closed&&view.ws?.readyState===WebSocket.OPEN&&view.canControl&&!view.tabReadOnly&&!app.browserLeaseLost)view.ws.send('\r');}
function cancelLocal(view,s){view.term.write('^C\r\n');reset(s);s.abort?.abort();s.busy=false;resume(view);}
async function requestAI(view,question){
 const s=state(view);s.busy=true;
 try{
  if(!question.trim()){print(view,'Usage: \\ai <question or shell command>. No shell command is executed.');return;}
  if(question.length>2000)throw Error('Question exceeds 2000 characters');
  const id=String(view.meta.id);
  const settings=await app.jsonFetch('/api/ai/settings');
  if(!settings.enabled||!settings.consent){
   print(view,'Enable AI Assistant and provider consent in Settings before using \\ai.','error');return;
  }
  const target=await app.jsonFetch('/api/ai/session-context?session_id='+encodeURIComponent(id));
  if(!target.verified||String(target.session_id)!==id)throw Error('Terminal target verification failed');
  if(view.closed||view.meta.id!==id)throw Error('Terminal changed during verification');
  const host=String(target.host||'local');
  const approved=window.confirm('Send this question to '+String(settings.provider||'AI provider')+'?\n\nTarget: '+host+
    '\n\n'+question.slice(0,500)+'\n\nAI will only answer; it cannot execute shell commands.');
  if(!approved){print(view,'Request cancelled. Nothing was sent to AI.');return;}
  print(view,'Sending question to '+String(settings.provider||'AI')+' · advisory only…');
  const controller=new AbortController();s.abort=controller;
  const result=await app.jsonFetch('/api/ai/assist',{
   method:'POST',headers:{'Content-Type':'application/json'},
   body:JSON.stringify({session_id:id,mode:'explain',question,command:'',output:'',
    cwd:'',git:'',history:'',remote:Boolean(target.remote),confirm_external:true}),
   signal:controller.signal
  });
  print(view,String(result.answer||'No response'), 'answer');
  print(view,'Provider: '+String(result.provider||'unknown')+' · '+String(result.duration_ms??'?')+' ms · No shell command executed');
 }catch(error){print(view, error?.name==='AbortError'?'AI request cancelled.':String(error?.message||error),'error');}
 finally{s.abort=null;s.busy=false;reset(s);resume(view);}
}

const protocol=globalThis.TaskDeckAgentIndexProtocol;
const loadedIndexKeys=new Set();
function indexKey(view){
 return 'taskdeck:os-commands:'+String(app.taskData?.workspace||'default')+':'+String(view.meta?.target_profile_id||'local');
}
function shellReady(view,auto=false){
 return !view.closed&&view.canControl&&!view.tabReadOnly&&!app.browserLeaseLost&&
 view.ws?.readyState===WebSocket.OPEN&&terminal.canIntercept(view)&&(!auto||terminal.getInputLine(view)==='');
}
function finishReload(view,job,result){
 const s=state(view);clearTimeout(job.timer);s.job=null;s.busy=false;
 if(result.error){print(view,result.error,'error');return;}
 const values=protocol.parse(job.kind,result.body);
 if(job.kind==='history')values.reverse();
 if(!values.length){print(view,'Bash returned no '+job.kind+' entries.','error');return;}
 terminal.setOSIndex(view,job.kind==='history'?'history':'commands',values);
 if(job.kind==='commands')try{sessionStorage.setItem(indexKey(view),JSON.stringify(values.slice(0,2500)));}catch{}
 print(view,(job.auto?'Automatic ':'')+'reload '+job.kind+': '+values.length+' indexed.');
}
function runReload(view,kind,auto=false){
 const s=state(view);
 if(s.busy||s.job||!shellReady(view,auto)){
  if(!auto)print(view,'Reload requires an idle writable Bash prompt.','error');
  return false;
 }
 const marker='TDIDX_'+String(globalThis.crypto?.randomUUID?.()||Date.now()+'_'+Math.random()).replace(/[^A-Za-z0-9_-]/g,'').slice(0,64);
 const fixed=kind==='history'?'history 2>/dev/null | tail -n 1000':
  '(compgen -c 2>/dev/null || true) | LC_ALL=C sort -u | head -n 4000';
 const command="printf '\\n"+marker+"_BEGIN\\n'; "+fixed+"; printf '\\n"+marker+"_END\\n'";
 s.busy=true;
 const job={kind,auto,marker,capture:protocol.create(marker),decoder:new TextDecoder(),timer:null};
 s.job=job;
 print(view,'Reloading '+kind+' from '+(view.meta.target_profile_id?'SSH':'local')+' shell…');
 job.timer=setTimeout(()=>{
  if(s.job!==job)return;s.job=null;s.busy=false;
  print(view,'Reload timed out; retained previous suggestions.','error');
 },15000);
 try{view.ws.send(command+'\r');return true;}
 catch(error){clearTimeout(job.timer);s.job=null;s.busy=false;print(view,String(error),'error');return false;}
}
function filterOutput(view,data){
 const s=states.get(String(view?.meta?.id||'')),job=s?.job;
 if(!job)return data;
 try{
  const chunk=typeof data==='string'?data:
   data instanceof ArrayBuffer?job.decoder.decode(new Uint8Array(data),{stream:true}):
   ArrayBuffer.isView(data)?job.decoder.decode(data,{stream:true}):null;
  if(chunk===null)return data;
  const result=job.capture.feed(chunk);
  if(!result.completed)return '';
  finishReload(view,job,result);return result.tail||'';
 }catch(error){clearTimeout(job.timer);s.job=null;s.busy=false;print(view,'Reload failed: '+String(error),'error');return '';}
}
function scheduleInitialIndex(view){
 if(!view?.meta?.id||view.meta.status!=='running')return;
 const key=indexKey(view);
 if(loadedIndexKeys.has(key))return;
 loadedIndexKeys.add(key);
 try{
  const stored=JSON.parse(sessionStorage.getItem(key)||'null');
  if(Array.isArray(stored)&&stored.length<=4000){terminal.setOSIndex(view,'commands',stored);return;}
 }catch{}
 setTimeout(()=>{
  if(shellReady(view,true))runReload(view,'commands',true);
  else loadedIndexKeys.delete(key);
 },900);
}
window.addEventListener('taskmenu:shell-integration',e=>{
 if(e.detail?.type==='prompt'||e.detail?.type==='command-started')scheduleInitialIndex(e.detail.view);
});
function commandRun(view,line){
 const s=state(view),text=String(line||''),match=/^\\(ai|do)(?:\s+([\s\S]*))?$/.exec(text);
 if(!match){print(view,'Unrecognized agent command. Supported: \\ai, \\do reload command, \\do reload history.','error');resume(view);return;}
 if(match[1]==='ai'){void requestAI(view,match[2]||'');return;}
 const action=String(match[2]||'').trim().toLowerCase();
 if(action==='reload command'||action==='reload commands'){if(!runReload(view,'commands'))resume(view);return;}
 if(action==='reload history'){if(!runReload(view,'history'))resume(view);return;}
 print(view,'Supported: \\do reload command, \\do reload history.','error');resume(view);
}
function onInput(view,value){
 const s=state(view),raw=String(value||'');
 // The agent is exclusive during an external request. Ctrl-C can abort it.
 if(s.busy){if(raw.includes('\x03'))s.abort?.abort();return true;}
 if(s.mode==='normal'){
  if(!raw.startsWith('\\')||!terminal.canIntercept(view)||terminal.getInputLine?.(view)!=='')return false;
  s.mode='candidate';s.candidate='';
 }
 // Paste never turns a reserved command into multiple terminal commands.
 if(raw.length>4096||(/[\r\n]/.test(raw)&&/[\r\n][^\r\n]/.test(raw))){
  if(s.mode==='agent')view.term.write('^C\r\n');
  reset(s);print(view,'Multi-line or oversized agent input rejected; no shell execution.','error');resume(view);return true;
 }
 for(let i=0;i<raw.length;i++){
  const char=raw[i];
  if(char==='\x03'){
   if(s.mode==='agent'||s.mode==='candidate')cancelLocal(view,s);
   return true;
  }
  if(char==='\x7f'||char==='\b'){
   if(s.mode==='agent'&&s.line.length){s.line=s.line.slice(0,-1);view.term.write('\b \b');}
   else if(s.mode==='candidate')s.candidate=s.candidate.slice(0,-1);
   if(s.mode==='candidate'&&!s.candidate)reset(s);
   continue;
  }
  if(char==='\r'||char==='\n'){
   if(s.mode==='candidate'&&(s.candidate==='\\ai'||s.candidate==='\\do')){
    s.mode='agent';s.line=s.candidate;view.term.write(s.line);
   }
   if(s.mode==='agent'){view.smartAgentConsumedInput=true;const line=s.line;view.term.write('\r\n');reset(s);commandRun(view,line);}
   else {view.ws.send(s.candidate+char);reset(s);}
   continue;
  }
  if(s.mode==='candidate'){
   s.candidate+=char;
   if(s.candidate==='\\ai '||s.candidate==='\\do '){
    s.mode='agent';s.line=s.candidate;view.term.write(s.line);s.candidate='';continue;
   }
   if(!['\\','\\a','\\ai','\\d','\\do'].some(x=>x.startsWith(s.candidate))){
    // A normal escaped Bash command: send every buffered byte untouched.
    view.ws.send(s.candidate+raw.slice(i+1));reset(s);return true;
   }
   continue;
  }
  if(s.mode==='agent'){
   if(s.line.length>=2100){print(view,'Agent line too long; Ctrl-C to cancel.','error');return true;}
   if(char==='\x15'){const cleared=s.line.length;view.term.write('\b \b'.repeat(cleared));s.line='';continue;}
   if(char==='\t'||char==='\x1b'){view.term.write('\x07');continue;}
   if(char>=' '&&char!=='\x7f'){s.line+=char;view.term.write(char);}
  }
 }
 return true;
}
function onDisconnect(view){
 const s=states.get(String(view?.meta?.id||''));if(!s)return;
 if(s.job?.timer)clearTimeout(s.job.timer);
 if(s.abort)s.abort.abort();
 if(s.job||s.busy)print(view,'Terminal disconnected; active agent request cancelled.','error');
 s.job=null;s.busy=false;s.abort=null;reset(s);
}
globalThis.TaskDeckTerminalAgents={onInput,print,filterOutput,runReload,onDisconnect};
})();
