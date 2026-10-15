// Explicit AI troubleshooting from xterm selections or failed OSC 133 commands.
// The assistant is advisory. Each proposed command needs a separate user click
// AND confirmation, and is sent only to its attested idle PTY.
(function(){
'use strict';
const app=globalThis.TaskMenuApp,core=globalThis.TaskDeckAITerminalActionsCore;
if(!app||!core)throw Error('AI troubleshooting prerequisites missing');
const el=(tag,cls='',value='')=>{const node=document.createElement(tag);node.className=cls;node.textContent=value;return node;};
const style=el('style');
style.textContent=[
'.ai-terminal-backdrop{display:none;position:fixed;inset:0;z-index:19000;background:#0009;align-items:flex-start;justify-content:center;overflow:auto;padding:4vh 10px}',
'.ai-terminal-backdrop.visible{display:flex}',
'.ai-terminal-wizard{width:min(790px,96vw);max-height:91vh;overflow:auto;background:#192331;color:#eef4ff;border:1px solid #58718a;border-radius:10px;padding:14px;box-shadow:0 16px 50px #0009;font:13px system-ui,sans-serif}',
'.ai-terminal-wizard h2{font-size:17px;margin:0 0 7px}',
'.ai-terminal-wizard p{line-height:1.5;margin:6px 0}',
'.ai-terminal-wizard textarea{box-sizing:border-box;width:100%;min-height:65px;resize:vertical;background:#0d1722;color:#f1f5fc;border:1px solid #60748b;border-radius:5px;padding:7px;font:12px/1.5 monospace}',
'.ai-terminal-wizard button,.ai-terminal-prompt button{padding:5px 10px;margin:4px 4px 4px 0}',
'.ai-terminal-card{padding:9px;border:1px solid #58697d;margin:7px 0;border-radius:6px}',
'.ai-terminal-card code{white-space:pre-wrap;overflow-wrap:anywhere;font:12px monospace}',
'.ai-terminal-card p{font-size:12px}',
'.ai-terminal-danger{color:#ffc1aa}.ai-terminal-status{color:#e6cf95}',
'.ai-terminal-menu{position:fixed;display:none;z-index:19100;background:#192331;border:1px solid #627890;border-radius:6px;padding:5px;box-shadow:0 8px 25px #0008}',
'.ai-terminal-menu.visible{display:block}',
'.ai-terminal-menu button{padding:8px 14px;background:transparent;border:0;color:#f2f7fc;cursor:pointer}',
'.ai-terminal-prompt{position:fixed;bottom:14px;right:15px;z-index:17500;width:min(440px,92vw);border:1px solid #71899f;border-radius:7px;background:#192331;color:#f1f6ff;padding:11px;box-shadow:0 10px 30px #0008}',
'.ai-terminal-prompt strong{display:block;margin-bottom:5px}',
'html[data-taskmenu-theme="light"] .ai-terminal-wizard{background:#fff;color:#203044}',
'html[data-taskmenu-theme="light"] .ai-terminal-wizard textarea{background:#f5f9ff;color:#203044}',
'html[data-taskmenu-theme="light"] .ai-terminal-menu,html[data-taskmenu-theme="light"] .ai-terminal-prompt{background:#f7faff;color:#1c3044}',
'html[data-taskmenu-theme="light"] .ai-terminal-menu button{color:#203044}',
].join('\n');
document.head.append(style);
const backdrop=el('div','ai-terminal-backdrop'),dialog=el('section','ai-terminal-wizard');
dialog.setAttribute('role','dialog');dialog.setAttribute('aria-modal','true');dialog.setAttribute('aria-label','AI Smart Terminal Troubleshooter');
const heading=el('h2','','AI Smart Terminal · Diagnose & choose actions');
const target=el('p','ai-terminal-status','');
const notice=el('p','','The selected terminal excerpt will be sent to your configured AI provider when you choose Ask AI. Review or edit it first. AI-suggested commands never run automatically.');
const questionLabel=el('p','','Question / desired outcome');
const question=el('textarea');question.value='Explain the error and suggest the safest concrete commands to resolve it.';
const contextLabel=el('p','','Selected terminal output / command result (editable)');
const context=el('textarea');context.placeholder='Selected terminal output';
const controls=el('div');
const askButton=el('button','','Ask AI'),cancelButton=el('button','','Cancel AI request'),copyButton=el('button','','Copy JSON'),closeButton=el('button','','Close');
for(const b of [askButton,cancelButton,copyButton,closeButton])b.type='button';
cancelButton.disabled=true;copyButton.disabled=true;
controls.append(askButton,cancelButton,copyButton,closeButton);
const status=el('p','ai-terminal-status',''),summary=el('p'),choices=el('div'),raw=el('pre');
raw.hidden=true;raw.style.whiteSpace='pre-wrap';raw.style.wordBreak='break-word';
dialog.append(heading,target,notice,questionLabel,question,contextLabel,context,controls,status,summary,choices,raw);
backdrop.append(dialog);document.body.append(backdrop);
const ctxmenu=el('div','ai-terminal-menu');
const askSelection=el('button','','Ask AI about selection'),copySelection=el('button','','Copy selection');
ctxmenu.append(askSelection,copySelection);document.body.append(ctxmenu);
const selectionCache=new WeakMap(),selectionObserved=new WeakSet();
function observeSelection(view){
 if(!view?.term||selectionObserved.has(view))return;
 selectionObserved.add(view);
 try{view.term.onSelectionChange?.(()=>{
  const text=String(view.term.getSelection?.()||'').trim();
  if(text)selectionCache.set(view,{text,at:Date.now()});
 });}catch{}
}
for(const v of app.views.values())observeSelection(v);
window.addEventListener('taskmenu:session',e=>observeSelection(e.detail?.view));
window.addEventListener('taskmenu:view-activated',()=>{for(const v of app.views.values())observeSelection(v);});
let selectedView=null,lastCommand=null,sourceType='',fetchController=null,answerText='',targetVerified=null;
let submitting=false,notification=null,alreadyAsked=new Map();
function cleanText(value,limit=5000){
 let text=String(value||'').slice(-limit).replace(/\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\))?/g,'');
 text=text.replace(/(authorization\s*:\s*bearer\s+)\S+/ig,'$1[REDACTED]');
 text=text.replace(/(api[_-]?key|password|token|secret)\s*[:=]\s*[^\s,;]+/ig,'$1=[REDACTED]');
 text=text.replace(/-----BEGIN (?:OPENSSH |RSA |EC )?PRIVATE KEY-----[\s\S]*?-----END (?:OPENSSH |RSA |EC )?PRIVATE KEY-----/g,'[REDACTED PRIVATE KEY]');
 return text;
}
function viewCurrent(v){return Boolean(v&&app.views.get(String(v.meta?.id||''))===v&&!v.closed);}
function hideMenu(){ctxmenu.classList.remove('visible');}
function close(){backdrop.classList.remove('visible');fetchController?.abort();selectedView=null;targetVerified=null;}
function open(view,output,{failure=false,command=null}={}){
 if(!viewCurrent(view))return false;
 hideMenu();selectedView=view;targetVerified=null;sourceType=failure?'exit-code':'selection';
 lastCommand=command||(globalThis.TaskDeckShellIntegration?.getCommands?.(view.meta.id)||[]).slice(-1)[0]||null;
 context.value=cleanText(output||lastCommand?.output||'',5000);
 question.value=failure?'Explain why this command failed and propose safe, specific fixes. Include the exact branch/remote from the error when supported.':
  'Explain the selected terminal output and propose safe, specific diagnostic or repair commands.';
 target.textContent='Session '+String(view.meta.id).slice(0,16)+' · '+(failure?'Command exit '+String(command?.exitCode):'Selected text')+
  ' · no execution until approved';
 summary.textContent='';choices.replaceChildren();raw.hidden=true;raw.textContent='';answerText='';
 status.textContent='Review the excerpt, then Ask AI. A provider consent confirmation is required.';
 copyButton.disabled=true;
 backdrop.classList.add('visible');question.focus();
 return true;
}
function openFailure(view,command){
 return open(view,command?.output||'',{failure:true,command});
}
function currentValid(){
 return viewCurrent(selectedView)&&String(targetVerified?.session_id||'')===String(selectedView?.meta?.id||'');
}
function busy(on){
 submitting=on;askButton.disabled=on;cancelButton.disabled=!on;
}
async function ask(){
 if(submitting||!viewCurrent(selectedView))return;
 const view=selectedView,sessionID=String(view.meta.id);
 busy(true);choices.replaceChildren();summary.textContent='';raw.hidden=true;targetVerified=null;
 try{
  const config=await app.jsonFetch('/api/ai/settings');
  if(!config.enabled||!config.consent||!config.has_api_key)throw Error('Enable AI Assistant, API key and provider consent in Settings');
  const checked=await app.jsonFetch('/api/ai/session-context?session_id='+encodeURIComponent(sessionID));
  if(!checked?.verified||String(checked.session_id)!==sessionID||!viewCurrent(view)||selectedView!==view)
   throw Error('Terminal session could not be verified');
  targetVerified=checked;
  const payload={session_id:sessionID,mode:'troubleshoot',question:String(question.value||'').slice(0,3000),
   command:cleanText(lastCommand?.command||'',2000),output:cleanText(context.value,Math.min(config.max_output_bytes||5000,12000)),
   cwd:'',history:'',git:'',remote:Boolean(checked.remote),confirm_external:false};
  if(!payload.question.trim())throw Error('Describe the assistance needed');
  // Explicit Yes · Ask AI (failure prompt) or Ask AI (wizard) is the
  // per-request action consenting to upload the visible/editable excerpt.
  // Preserve the backend opt-in flag; do NOT add another confirmation.
  // Failure detection alone must never trigger any provider request.
  payload.confirm_external=true;
  fetchController=new AbortController();
  status.textContent='Asking '+String(config.provider)+' for structured troubleshooting options…';
  const result=await app.jsonFetch('/api/ai/assist',{method:'POST',headers:{'Content-Type':'application/json'},
   signal:fetchController.signal,body:JSON.stringify(payload)});
  if(!viewCurrent(view)||selectedView!==view||!currentValid())throw Error('Terminal target changed during AI request');
  answerText=String(result.answer||'');
  raw.textContent=answerText;copyButton.disabled=!answerText;
  let parsed;
  try{parsed=core.parseAnswer(answerText);}
  catch(error){status.textContent='Unstructured response; commands cannot be executed: '+String(error.message);raw.hidden=false;return;}
  summary.textContent=parsed.summary||'AI returned '+parsed.commands.length+' suggestions';
  status.textContent=parsed.commands.length+' valid command suggestions · '+parsed.rejected.length+
   ' rejected · verify every action before running';
  for(const item of parsed.commands)choices.append(commandCard(view,item));
  if(!parsed.commands.length)raw.hidden=false;
 }catch(error){
  status.textContent=error?.name==='AbortError'?'Request cancelled':'AI request failed: '+String(error.message||error);
 }finally{fetchController=null;busy(false);}
}
function commandCard(view,item){
 const card=el('div','ai-terminal-card'),code=el('code','',item.command);
 const details=el('p','',item.purpose+' · risk: '+item.risk);
 if(['high','critical'].includes(item.risk))details.className='ai-terminal-danger';
 const copy=el('button','','Copy'),run=el('button','','Run this command');
 copy.onclick=()=>Promise.resolve(globalThis.navigator?.clipboard?.writeText?.(item.command)).catch(app.showError);
 // Deliberately no auto-run, no batch execution and no inline executable
 // commands from terminal output. Recheck target and idle shell on each click.
 run.onclick=()=>executeOne(view,item);
 card.append(code,details,copy,run);return card;
}
async function executeOne(view,item){
 const cmd=core.safeCommand(item.command);
 if(!cmd||!currentValid()||selectedView!==view){status.textContent='Stale or unsafe command; reopen the wizard';return false;}
 const current=globalThis.TaskDeckShellIntegration?.getState?.(view.meta.id);
 if(!core.canExecute(view,current,app)){
  status.textContent='Cannot execute: shell is busy, prompt is not verified, input is nonempty, or terminal is read-only. Copy the command and paste manually instead.';
  return false;
 }
 if(String(app.active||'').replace(/^external:/,'')!==String(view.meta.id)){
  status.textContent='Activate the original terminal before executing a proposal';return false;
 }
 let checked;
 try{checked=await app.jsonFetch('/api/ai/session-context?session_id='+encodeURIComponent(view.meta.id));}
 catch(error){status.textContent='Session re-verification failed: '+error.message;return false;}
 if(!checked?.verified||String(checked.session_id)!==String(targetVerified.session_id)||
  Boolean(checked.remote)!==Boolean(targetVerified.remote)||
  !core.canExecute(view,globalThis.TaskDeckShellIntegration?.getState?.(view.meta.id),app)){
  status.textContent='Session or prompt changed; command not executed';return false;
 }
 if(!window.confirm('Execute in this terminal?\n\n'+cmd+'\n\nRisk: '+item.risk+
   '\n\nYou are responsible for reviewing this untrusted AI suggestion.'))return false;
 if(!core.canExecute(view,globalThis.TaskDeckShellIntegration?.getState?.(view.meta.id),app))return false;
 // Single approved command is sent to this PTY only, never silently broadcast
 // to other terminals. The CR is intentional AFTER user confirmation.
 view.ws.send(cmd+'\r');
 status.textContent='Sent approved command to '+view.meta.id+'. Wait for its result before running another.';
 return true;
}
function notifyFailure(view,command){
 if(!viewCurrent(view)||Number(view.meta?.task_id||0)!==0||
  !Number.isInteger(command?.exitCode)||command.exitCode===0||command.exitCode===130)return;
 if(!String(command.command||'').trim())return;
 const key=String(view.meta.id)+':'+String(command.finishedAt||'')+':'+String(command.command);
 if(alreadyAsked.has(key))return;
 alreadyAsked.set(key,true);if(alreadyAsked.size>100)alreadyAsked=new Map([[key,true]]);
 notification?.remove();
 const n=el('div','ai-terminal-prompt');
 n.append(el('div','','Ask AI Smart Terminal to help diagnose this error?'));
 const yes=el('button','','Yes · Ask AI'),no=el('button','','Not now');
 yes.onclick=()=>{
  n.remove();notification=null;
  if(openFailure(view,command))void ask(); // Yes explicitly authorizes this provider request
 };
 no.onclick=()=>{n.remove();notification=null;};
 n.append(yes,no);document.body.append(n);notification=n;
}
const docContext=e=>{
 const element=e.target;
 if(!element?.closest?.('.xterm-screen'))return;
 const view=[...app.views.values()].find(v=>v.term?.element?.contains?.(element));
 if(!view)return;
 const live=String(view.term?.getSelection?.()||'').trim();
 const previous=selectionCache.get(view);
 // xterm may clear the highlight during right-button mousedown. Recover it
 // only in the brief context-menu window, not across unrelated commands.
 const selection=live||((previous&&Date.now()-previous.at<1200)?previous.text:'');
 if(!selection)return; // Preserve native menu when nothing was selected.
 e.preventDefault();e.stopPropagation();hideMenu();
 ctxmenu.style.left=Math.max(5,Math.min(e.clientX,window.innerWidth-255))+'px';
 ctxmenu.style.top=Math.max(5,Math.min(e.clientY,window.innerHeight-45))+'px';
 ctxmenu.classList.add('visible');
 askSelection.onclick=()=>{if(open(view,selection))hideMenu();};
 copySelection.onclick=()=>{hideMenu();void Promise.resolve(globalThis.navigator?.clipboard?.writeText?.(selection)).catch(app.showError);};
};
document.addEventListener('contextmenu',docContext,true);
document.addEventListener('pointerdown',e=>{if(!ctxmenu.contains(e.target))hideMenu();},true);
window.addEventListener('taskmenu:shell-integration',event=>{
 const {view,type,command}=event.detail||{};
 if(type==='command-finished')notifyFailure(view,command);
});
closeButton.onclick=close;askButton.onclick=()=>void ask();cancelButton.onclick=()=>fetchController?.abort();
copyButton.onclick=()=>Promise.resolve(globalThis.navigator?.clipboard?.writeText?.(answerText)).catch(app.showError);
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
dialog.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();close();}});
globalThis.TaskDeckAITroubleshoot={openForSelection:(view,text)=>open(view,text),openForFailure:openFailure,notifyFailure,executeOne,
 get visible(){return backdrop.classList.contains('visible')}};
})();
