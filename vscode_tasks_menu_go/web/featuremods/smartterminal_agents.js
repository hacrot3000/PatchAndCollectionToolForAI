// Reserved terminal commands never reach the user's shell as typed.
// AI calls reuse TaskDeck's existing verified-target, encrypted-key and quota APIs.
(function(){
'use strict';
const app=globalThis.TaskMenuApp,terminal=globalThis.TaskDeckSmartTerminal;
if(!app||!terminal)throw Error('Smart terminal agent prerequisites unavailable');
const states=new Map();
function state(view){let s=states.get(view.meta.id);if(!s){s={mode:'normal',candidate:'',line:'',busy:false,abort:null,typedSincePrompt:''};states.set(view.meta.id,s);}return s;}
function safeText(value){return String(value??'').replace(/\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*\x07)?/g,'').replace(/[\x00-\x08\x0b-\x1f\x7f]/g,'');}
function print(view,message,kind='info'){
 if(view.closed)return;
 const color=kind==='error'?'91':kind==='answer'?'96':'93';
 const clean=safeText(message).slice(0,16500).replace(/\r?\n/g,'\r\n');
 view.term.write('\r\n\x1b[1;'+color+'m[TaskDeck '+(kind==='answer'?'AI':kind==='error'?'ERROR':'Agent')+']\x1b[0m '+clean+'\r\n');
}
function reset(s){s.mode='normal';s.candidate='';s.line='';s.typedSincePrompt='';s.helpOnly=false;}
 // Candidate bytes are temporarily drawn in xterm for instant keyboard echo.
 // They are NOT sent to Bash until ruled out as a TaskDeck directive.
 // Before handing ordinary escaped input back to the PTY, erase this local
 // preview so Bash's own echo produces exactly one copy.
function eraseCandidatePreview(view,s){
 const chars=Array.from(s.candidate||'');
 if(chars.length)view.term.write('\b \b'.repeat(chars.length));
}
// When OSC 133 is unavailable, permit ONLY the local read-only Help popup
// at a recognizable empty Bash prompt. Never allow AI/remote commands from
// this visual fallback or interfere with TUI, task or patch sessions.
function helpAtPlainShellPrompt(view){
 if(!view||view.closed||!view.canControl||view.tabReadOnly||app.browserLeaseLost||
    view.meta?.status!=='running'||view.ws?.readyState!==WebSocket.OPEN)return false;
 const meta=view.meta||{};
 if(meta.kind&&meta.kind!=='terminal')return false;
 if(![0,-1].includes(meta.task_id==null?0:Number(meta.task_id)))return false;
 if(globalThis.TaskDeckShellIntegration?.getState?.(meta.id)?.executing)return false;
 const buffer=view.term?.buffer?.active;
 if(!buffer||typeof buffer.getLine!=='function')return false;
 const row=Number(buffer.baseY||0)+Number(buffer.cursorY||0);
 const screenLine=buffer.getLine(row);
 if(!screenLine||screenLine.isWrapped)return false;
 const display=String(screenLine.translateToString?.(true)||'');
 if(display.length>300||Number(buffer.cursorX||0)<display.trimEnd().length-1)return false;
 // Recognize conventional user@host:path$ and [user@host path]# only.
 // Do not interpret arbitrary output with a trailing $ or # as a prompt.
 return /^(?:\([^\r\n)]{1,60}\)\s*)?(?:[\w.-]+@[\w.-]+:[^\r\n]{0,240}|\[[\w.@:/~ -]{1,160}\])\s*[#$]\s*$/.test(display);
}
function resume(view){if(!view.closed&&view.ws?.readyState===WebSocket.OPEN&&view.canControl&&!view.tabReadOnly&&!app.browserLeaseLost)view.ws.send('\r');}
function cancelLocal(view,s){view.term.write('^C\r\n');reset(s);s.abort?.abort();s.busy=false;resume(view);}
async function requestAI(view,question){
 const s=state(view);s.busy=true;const generation=s.connectionGeneration||0;
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
 finally{if((s.connectionGeneration||0)===generation){s.abort=null;s.busy=false;reset(s);resume(view);}}
}

const protocol=globalThis.TaskDeckAgentIndexProtocol;
const loadedIndexKeys=new Set();
function indexKey(view){
 return 'taskdeck:os-commands:'+String(app.taskData?.workspace||'default')+':'+String(view.meta?.target_profile_id||'local');
}
function shellReady(view,auto=false){
 if(!view||view.closed||!view.canControl||view.tabReadOnly||app.browserLeaseLost||
  view.ws?.readyState!==WebSocket.OPEN||!terminal.canIntercept(view))return false;
 if(!auto)return true;
 // Without OSC 133 getInputLine() returns '' even for an unknown shell state.
 // Background compgen must never be sent while a user is editing a command
 // or using Telnet / Vim / another interactive program.
 const shell=globalThis.TaskDeckShellIntegration?.getState?.(view.meta?.id);
 return Boolean(shell?.seenPrompt&&shell.commandRow!=null&&!shell.executing&&terminal.getInputLine(view)==='');
}
function finishReload(view,job,result){
 const s=state(view);clearTimeout(job.timer);s.job=null;s.busy=false;
 if(result.error){
  if(job.kind==='aliases')s.aliasesReady=false;
  if(!job.auto)print(view,result.error,'error');
  return;
 }
 const values=protocol.parse(job.kind,result.body);
 if(job.kind==='history')values.reverse();
 if(job.kind==='aliases'){
  terminal.setOSIndex(view,'aliases',values);
  state(view).aliasesReady=true;
  if(!job.auto)print(view,'Loaded '+values.length+' shell aliases.');
  return;
 }
 if(!values.length){if(!job.auto)print(view,'Bash returned no '+job.kind+' entries.','error');return;}
 terminal.setOSIndex(view,job.kind==='history'?'history':'commands',values);
 if(job.kind==='commands')try{sessionStorage.setItem(indexKey(view),JSON.stringify(values.slice(0,2500)));}catch{}
 if(!job.auto)print(view,'Reload '+job.kind+': '+values.length+' indexed.');
}
function runReload(view,kind,auto=false){
 // Never inject inventory scripts from automatic lifecycle events.
 // PTY echo/capture races leak TDIDX marker output and extra prompts.
 // Only a deliberate user request may run alias -p, compgen or history.
 if(auto)return false;
 const s=state(view);
 if(s.busy||s.job||s.mode!=='normal'||(auto&&s.typedSincePrompt)||!shellReady(view,auto)){
  if(!auto)print(view,'Reload requires an idle writable Bash prompt.','error');
  return false;
 }
 const marker='TDIDX_'+String(globalThis.crypto?.randomUUID?.()||Date.now()+'_'+Math.random()).replace(/[^A-Za-z0-9_-]/g,'').slice(0,64);
 const fixed=kind==='history'?'history 2>/dev/null | tail -n 1000':
  kind==='aliases'?'alias -p 2>/dev/null | head -n 400':
  '(compgen -c 2>/dev/null || true) | LC_ALL=C sort -u | head -n 4000';
 const command="printf '\\n"+marker+"_BEGIN\\n'; "+fixed+"; printf '\\n"+marker+"_END\\n'";
 s.busy=true;
 const job={kind,auto,marker,capture:protocol.create(marker),decoder:new TextDecoder(),timer:null};
 s.job=job;
 if(!auto)print(view,'Reloading '+kind+' from '+(view.meta.target_profile_id?'SSH':'local')+' shell…');
 job.timer=setTimeout(()=>{
  if(s.job!==job)return;s.job=null;s.busy=false;
  if(job.kind==='aliases')s.aliasesReady=false;
  if(!job.auto)print(view,'Reload timed out; retained previous suggestions.','error');
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
  finishReload(view,job,result);
  // The metadata command and its newline were hidden from xterm, but Bash
  // printed a fresh PS1 afterward. Replace the existing idle prompt in
  // place instead of appending a second prompt on the same screen row.
  // Only automatic scans use this path; explicit \\do retains its output.
   return result.tail||'';
 }catch(error){clearTimeout(job.timer);s.job=null;s.busy=false;print(view,'Reload failed: '+String(error),'error');return '';}
}
// Background indexing must never type anything into a user's PTY.
// Existing cached command names are still useful and entirely passive.
// Live PATH names and local startup aliases are resolved by the read-only
// completion API in smartterminal.js when the user requests a suggestion.
function hydrateCachedCommands(view){
 if(!view?.meta?.id||view.meta.status!=='running')return;
 const key=indexKey(view);
 if(loadedIndexKeys.has(key))return;
 loadedIndexKeys.add(key);
 try{
  const stored=JSON.parse(sessionStorage.getItem(key)||'null');
  if(Array.isArray(stored)&&stored.length<=4000)
   terminal.setOSIndex(view,'commands',stored);
 }catch{}
}
window.addEventListener('taskmenu:shell-integration',e=>{
 const detail=e.detail||{},view=detail.view;
 if(detail.type==='prompt'||detail.type==='command-started'){
  if(view&&state(view).mode==='normal'&&!state(view).busy)
   state(view).typedSincePrompt='';
  hydrateCachedCommands(view);
 }
 // A changed alias may be picked up through an explicit reload. Never
 // automatically send alias -p while a user changes tabs or starts a command.
});
function commandRun(view,line){
 const s=state(view),text=String(line||''),match=/^\\(ai|do|help)(?:\s+([\s\S]*))?$/.exec(text);
 if(!match){print(view,'Unrecognized TaskDeck command. Supported: \\help, \\ai, \\do reload command, \\do reload history.','error');resume(view);return;}
 if(match[1]==='help'){
  globalThis.TaskMenuHelp?.open?.(String(match[2]||'').trim());
  resume(view);return;
 }
 if(match[1]==='ai'){void requestAI(view,match[2]||'');return;}
 const action=String(match[2]||'').trim().toLowerCase();
 if(action==='reload command'||action==='reload commands'){if(!runReload(view,'commands'))resume(view);return;}
 if(action==='reload history'){if(!runReload(view,'history'))resume(view);return;}
 if(action==='reload aliases'||action==='reload alias'){if(!runReload(view,'aliases'))resume(view);return;}
 print(view,'Supported: \\do reload command, \\do reload history, \\do reload aliases.','error');resume(view);
}
// Detect an already echoed directive on Enter, including terminals where
// OSC prompt integration was late or absent. Erase the line in Bash BEFORE
// dispatching it locally; otherwise Bash would execute 'help' or 'do'.
function reservedAtPrompt(view){
 const line=terminal.readReservedLine?.(view);
 return typeof line==='string'&&/^\\(?:help|ai|do)(?:\s|$)/.test(line)?line:null;
}
function wouldInterceptEnter(view,data){
 if(!/[\r\n]/.test(String(data||'')))return false;
 const s=state(view);
 return s.mode==='agent'||s.mode==='candidate'||Boolean(reservedAtPrompt(view));
}
function onInput(view,value){
 const s=state(view),raw=String(value||'');
 // The agent is exclusive during an external request. Ctrl-C can abort it.
 if(s.busy){if(raw.includes('\x03'))s.abort?.abort();return true;}
 if(s.mode==='normal'){
  if(raw==='\r'||raw==='\n'){
   const line=reservedAtPrompt(view);
   if(line){
    // Ctrl-U clears an already delivered Bash Readline buffer. Sending the
    // local directive as text to the PTY would execute a different command.
    view.ws.send('\x15');
    view.smartAgentConsumedInput=true;
    s.typedSincePrompt='';
    view.term.write('\r\n');
    commandRun(view,line);
    return true;
   }
   s.typedSincePrompt='';
   return false;
  }
  if(raw==='\x03'||raw==='\x15'){s.typedSincePrompt='';return false;}
  if(raw==='\x7f'||raw==='\b'){s.typedSincePrompt=s.typedSincePrompt.slice(0,-1);return false;}
  const integrated=terminal.canIntercept(view)&&terminal.getInputLine?.(view)==='';
  const helpFallback=!integrated&&helpAtPlainShellPrompt(view);
  if(s.typedSincePrompt||!raw.startsWith('\\')||(!integrated&&!helpFallback)){
   s.typedSincePrompt=(s.typedSincePrompt+raw).slice(-1024);return false;
  }
  s.mode='candidate';s.candidate='';s.helpOnly=helpFallback;
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
   else if(s.mode==='candidate'&&s.candidate.length){
    s.candidate=s.candidate.slice(0,-1);view.term.write('\b \b');
   }
   if(s.mode==='candidate'&&!s.candidate)reset(s);
   continue;
  }
  if(char==='\x15'&&s.mode==='candidate'){
   eraseCandidatePreview(view,s);reset(s);continue;
  }
  if(char==='\r'||char==='\n'){
   if(s.mode==='candidate'&&(s.candidate==='\\ai'||s.candidate==='\\do'||s.candidate==='\\help')){
    s.mode='agent';s.line=s.candidate;
    // Its characters have already been rendered while the user was typing.
   }
   if(s.mode==='agent'){view.smartAgentConsumedInput=true;const line=s.line;view.term.write('\r\n');reset(s);commandRun(view,line);}
   else {
    // Ordinary shell input must be echoed by Bash, not both Bash and our
    // temporary candidate preview. Preserve the exact original bytes.
    eraseCandidatePreview(view,s);
    view.ws.send(s.candidate+char);reset(s);
   }
   continue;
  }
  if(s.mode==='candidate'){
   // Immediate visual feedback matters for a single literal backslash; the
   // first character must not seem lost while TaskDeck checks its prefix.
   s.candidate+=char;view.term.write(char);
   if((!s.helpOnly&&(s.candidate==='\\ai '||s.candidate==='\\do '))||s.candidate==='\\help '){
    s.mode='agent';s.line=s.candidate;s.candidate='';continue;
   }
   const candidates=s.helpOnly?['\\','\\h','\\he','\\hel','\\help']:['\\','\\a','\\ai','\\d','\\do','\\h','\\he','\\hel','\\help'];
   if(!candidates.some(x=>x.startsWith(s.candidate))){
    // A normal escaped Bash command: clear our preview before the PTY
    // echoes the real bytes, including the common two-backslash sequence.
    eraseCandidatePreview(view,s);
    const released=s.candidate+raw.slice(i+1);view.ws.send(released);reset(s);s.typedSincePrompt=released;return true;
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
 s.connectionGeneration=(s.connectionGeneration||0)+1;
 if(s.job?.timer)clearTimeout(s.job.timer);
 s.aliasesReady=false;
 terminal.setOSIndex?.(view,'aliases',[]);
 if(s.abort)s.abort.abort();
 if(s.job||s.busy)print(view,'Terminal disconnected; active agent request cancelled.','error');
 s.job=null;s.busy=false;s.abort=null;reset(s);
}
globalThis.TaskDeckTerminalAgents={onInput,print,filterOutput,runReload,onDisconnect,wouldInterceptEnter};
})();
