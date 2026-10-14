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
function commandRun(view,line){
 const s=state(view),text=String(line||''),match=/^\\(ai|do)(?:\s+([\s\S]*))?$/.exec(text);
 if(!match){print(view,'Unrecognized agent command. Supported: \\ai, \\do reload command, \\do reload history.','error');resume(view);return;}
 if(match[1]==='ai'){void requestAI(view,match[2]||'');return;}
 print(view,'Agent action '+(match[2]||'')+' is not installed yet. Nothing was sent to the shell.','error');resume(view);
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
   if(s.mode==='agent'){const line=s.line;view.term.write('\r\n');reset(s);commandRun(view,line);}
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
globalThis.TaskDeckTerminalAgents={onInput,print};
})();
