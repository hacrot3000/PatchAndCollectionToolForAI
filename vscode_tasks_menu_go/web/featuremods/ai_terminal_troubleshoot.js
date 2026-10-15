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
'.ai-terminal-dock{position:absolute;bottom:12px;right:12px;z-index:30;display:flex;gap:3px;align-items:center;padding:3px;border-radius:11px;border:1px solid #66829e;background:#192b3d;box-shadow:0 8px 28px #000a;color:#edf5ff;max-width:calc(100% - 24px)}',
'.ai-terminal-dock[hidden]{display:none!important}',
'.ai-terminal-dock button{border:0;background:transparent;color:inherit;margin:0;display:flex;align-items:center;justify-content:center;min-height:32px}',
'.ai-terminal-dock .ai-terminal-dock-open{gap:7px;font:12px system-ui,sans-serif;min-width:34px;border-radius:8px;padding:5px 8px}',
'.ai-terminal-dock .ai-terminal-dock-open:hover,.ai-terminal-dock .ai-terminal-dock-close:hover{background:#345273}',
'.ai-terminal-dock .ai-terminal-dock-close{font-size:18px;width:30px;border-left:1px solid #526b87;border-radius:0 7px 7px 0}',
'html[data-taskmenu-theme="light"] .ai-terminal-dock{background:#fff;color:#22364a;border-color:#9aacc2;box-shadow:0 6px 18px #0003}',
'html[data-taskmenu-theme="light"] .ai-terminal-dock .ai-terminal-dock-open:hover,html[data-taskmenu-theme="light"] .ai-terminal-dock .ai-terminal-dock-close:hover{background:#e4eff9}',
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
const askButton=el('button','','Ask AI'),cancelButton=el('button','','Cancel AI request'),copyButton=el('button','','Copy JSON'),closeButton=el('button','','Close'),minimizeButton=el('button','','Minimize');
for(const b of [askButton,cancelButton,copyButton,closeButton,minimizeButton])b.type='button';
cancelButton.disabled=true;copyButton.disabled=true;
controls.append(askButton,cancelButton,copyButton,closeButton,minimizeButton);
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
// Page-local provenance: restored scrollback and OSC 133;C/D replay are NOT
// evidence that the user just ran a command. Only a verified Enter in this
// page (shellintegration's interactive-enter) or an explicitly approved AI
// command arms one single completion for the originating active terminal.
const pendingInput=new Map();
const failureWindowMs=2*60*1000;
function activeTerminal(view){
 return viewCurrent(view)&&String(app.active||'')===String(view.meta.id);
}
function clearFailurePrompt(){
 notification?.remove();notification=null;
}
function rememberSubmission(view,command,{startedAt=0,source='interactive-enter'}={}){
 if(!activeTerminal(view))return false;
 const text=String(command||'').trim();
 if(!text||text.length>1000)return false;
 pendingInput.set(String(view.meta.id),{command:text,startedAt:Number(startedAt)||0,
  timestamp:Date.now(),source,view});
 clearFailurePrompt();
 return true;
}
function forgetTerminal(view){
 const id=String(view?.meta?.id||'');
 pendingInput.delete(id);
 if(notification?.terminalViewId===id)clearFailurePrompt();
}

let dock=null,dockOwner=null,animation=null,transitionSerial=0,minimized=false;
const dockOpen=el('button','ai-terminal-dock-open','✦  AI Smart Terminal');
const dockClose=el('button','ai-terminal-dock-close','×');
dockOpen.type='button';dockClose.type='button';
dockOpen.title='Restore this AI Smart Terminal dialog (no new AI request)';
dockOpen.setAttribute('aria-label','Restore AI Smart Terminal dialog');
dockClose.title='Close this AI Smart Terminal dialog';
dockClose.setAttribute('aria-label','Close AI Smart Terminal dialog');
function stopAnimation(){
 transitionSerial++;
 try{animation?.cancel?.();}catch{}
 animation=null;
 dialog.style.removeProperty('transform');
 dialog.style.removeProperty('opacity');
}
function removeDock(){
 if(dock){dock.remove();dock=null;}
 dockOwner=null;minimized=false;
}
function ensureDock(view){
 if(!viewCurrent(view)||!view.pane?.append)return null;
 if(dockOwner===view&&dock?.isConnected)return dock;
 removeDock();
 dock=el('div','ai-terminal-dock');
 dock.setAttribute('role','group');
 dock.setAttribute('aria-label','Minimized AI Smart Terminal for this terminal');
 dock.append(dockOpen,dockClose);
 view.pane.append(dock);
 dockOwner=view;
 return dock;
}
function reducedMotion(){
 return Boolean(globalThis.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches);
}
function dialogFlight(from,to,reverse,finished){
 const start=from?.getBoundingClientRect?.(),end=to?.getBoundingClientRect?.();
 if(!start||!end||!start.width||!start.height||!end.width||!end.height||
  reducedMotion()||typeof dialog.animate!=='function'){finished();return;}
 // The dialog is the animation element in both directions: express
 // all transforms relative to its original screen rectangle.
 const dx=end.left+end.width/2-(start.left+start.width/2);
 const dy=end.top+end.height/2-(start.top+start.height/2);
 const sx=Math.max(.04,Math.min(1,end.width/start.width));
 const sy=Math.max(.04,Math.min(1,end.height/start.height));
 const frames=reverse?
  [{transform:'translate('+dx+'px,'+dy+'px) scale('+sx+','+sy+')',opacity:.35},
   {transform:'none',opacity:1}]:
  [{transform:'none',opacity:1},
   {transform:'translate('+dx+'px,'+dy+'px) scale('+sx+','+sy+')',opacity:.35}];
 const serial=++transitionSerial;
 try{
  animation=dialog.animate(frames,{duration:320,easing:'cubic-bezier(.22,.75,.25,1)',fill:'both'});
  Promise.resolve(animation.finished).then(()=>{
   if(serial!==transitionSerial)return;
   // A fill:'both' animation otherwise keeps the last scale/translate effect
   // even after the dialog is hidden or restored.
   try{animation?.cancel?.();}catch{}
   animation=null;dialog.style.removeProperty('transform');
   dialog.style.removeProperty('opacity');finished();
  }).catch(()=>{
   // A browser may cancel an animation during a pane layout update. Finish
   // the transition unless a newer restore/minimize/close superseded it.
   if(serial!==transitionSerial)return;
   animation=null;finished();
  });
 }catch{finished();}
}
function minimize(){
 if(minimized||!viewCurrent(selectedView)||!backdrop.classList.contains('visible'))return false;
 const activeView=selectedView;
 stopAnimation();
 const destination=ensureDock(activeView);
 if(!destination)return false;
 minimized=true;
 destination.style.visibility='hidden';
 destination.hidden=false;
 dialogFlight(dialog,destination,false,()=>{
  if(!minimized||dockOwner!==activeView)return;
  backdrop.classList.remove('visible');
  destination.style.visibility='';
  destination.hidden=false;
  // Return keyboard ownership to the terminal instead of leaving focus on
  // a now-hidden Copy/Run button inside the dialog.
  if(String(app.active||'')===String(activeView.meta.id)&&
   !activeView.pane?.classList?.contains?.('hidden')){
   try{activeView.term?.focus?.();}catch{}
  }
 });
 return true;
}
function restore(){
 if(!minimized||!dock||!viewCurrent(dockOwner))return false;
 stopAnimation();
 const destination=dock;
 const owner=dockOwner;
 backdrop.classList.add('visible');
 dialogFlight(dialog,destination,true,()=>{
  if(dockOwner!==owner)return;
  destination.hidden=true;
  destination.style.visibility='';
  minimized=false;
  // Keyboard focus must not remain on the hidden dock's Restore button.
  try{closeButton.focus?.();}catch{}
 });
 return true;
}

function cleanText(value,limit=5000){
 let text=String(value||'').slice(-limit).replace(/\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\))?/g,'');
 text=text.replace(/(authorization\s*:\s*bearer\s+)\S+/ig,'$1[REDACTED]');
 text=text.replace(/(api[_-]?key|password|token|secret)\s*[:=]\s*[^\s,;]+/ig,'$1=[REDACTED]');
 text=text.replace(/-----BEGIN (?:OPENSSH |RSA |EC )?PRIVATE KEY-----[\s\S]*?-----END (?:OPENSSH |RSA |EC )?PRIVATE KEY-----/g,'[REDACTED PRIVATE KEY]');
 return text;
}
function viewCurrent(v){return Boolean(v&&app.views.get(String(v.meta?.id||''))===v&&!v.closed);}
function hideMenu(){ctxmenu.classList.remove('visible');}
function close(){
 stopAnimation();
 backdrop.classList.remove('visible');
 fetchController?.abort();fetchController=null;
 removeDock();selectedView=null;targetVerified=null;
}
function open(view,output,{failure=false,command=null}={}){
 if(!viewCurrent(view))return false;
 stopAnimation();removeDock();
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
async function copyAndMinimize(value){
 const writer=globalThis.navigator?.clipboard?.writeText;
 if(typeof writer!=='function'){
  status.textContent='Clipboard not available; dialog was not minimized';
  return false;
 }
 try{
  await writer.call(globalThis.navigator.clipboard,String(value||''));
  minimize();
  return true;
 }catch(error){
  status.textContent='Copy failed: '+String(error?.message||error);
  return false;
 }
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
 copy.onclick=()=>copyAndMinimize(item.command);
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
 // A confirmed AI action bypasses xterm.onData; explicitly mark it as a
 // live user-approved submission instead of trusting future replayed output.
 if(!rememberSubmission(view,cmd,{source:'approved-ai'}))return false;
 view.ws.send(cmd+'\r');
 status.textContent='Sent approved command to '+view.meta.id+'. Wait for its result before running another.';
 minimize();
 return true;
}
function notifyFailure(view,command){
 const id=String(view?.meta?.id||''),pending=pendingInput.get(id);
 // Consume the arm immediately: even replayed or repeated OSC D frames may
 // produce fresh timestamps; they must never revive the old Ask AI prompt.
 pendingInput.delete(id);
 if(!pending||pending.view!==view||!activeTerminal(view)||
  Number(view.meta?.task_id||0)!==0||
  !Number.isInteger(command?.exitCode)||command.exitCode===0||command.exitCode===130)return;
 const now=Date.now();
 if(now-pending.timestamp<0||now-pending.timestamp>failureWindowMs)return;
 const actual=String(command.command||'').trim();
 if(!actual||actual!==pending.command)return;
 // Command-finished must refer to the most recent parsed command belonging
 // to this exact view. No history fallback: unknown provenance means NO popup.
 const state=globalThis.TaskDeckShellIntegration?.getState?.(id);
 const latest=state?.commands?.[state.commands.length-1];
 if(!state||latest!==command)return;
 if(pending.source==='interactive-enter'&&
  (!pending.startedAt||pending.startedAt!==Number(state.startedAt||0)))return;
 const key=id+':'+String(command.finishedAt||'')+':'+actual;
 if(alreadyAsked.has(key))return;
 alreadyAsked.set(key,true);if(alreadyAsked.size>100)alreadyAsked=new Map([[key,true]]);
 clearFailurePrompt();
 const n=el('div','ai-terminal-prompt');
 n.terminalViewId=id;
 n.append(el('div','','Ask AI Smart Terminal to help diagnose this error?'));
 const commandLabel=el('code','',actual);
 commandLabel.title='Latest failed command in the active terminal';
 commandLabel.style.display='block';commandLabel.style.margin='6px 0';
 n.append(commandLabel);
 const yes=el('button','','Yes · Ask AI'),no=el('button','','Not now');
 yes.onclick=()=>{
  clearFailurePrompt();
  if(!activeTerminal(view))return;
  // Do not offer stale advice if a newer command has finished since this
  // prompt was rendered, or if the view has changed.
  if(globalThis.TaskDeckShellIntegration?.getState?.(id)?.commands?.at(-1)!==command)return;
  if(openFailure(view,command))void ask();
 };
 no.onclick=clearFailurePrompt;
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
 const {view,type,command,source,state}=event.detail||{};
 if(type==='execution-started'&&source==='interactive-enter'){
  rememberSubmission(view,state?.activeCommand,{startedAt:state?.startedAt,source});
 }else if(type==='command-finished')notifyFailure(view,command);
});
window.addEventListener('taskmenu:view-activated',event=>{
 const activeID=event.detail?.kind==='terminal'?String(event.detail.id||''):'';
 for(const id of pendingInput.keys())if(id!==activeID)pendingInput.delete(id);
 if(notification?.terminalViewId!==activeID)clearFailurePrompt();
});
window.addEventListener('taskmenu:session',event=>{
 const view=event.detail?.view;
 if(view?.closed||view?.closing||view?.meta?.status!=='running')forgetTerminal(view);
});
closeButton.onclick=close;askButton.onclick=()=>void ask();cancelButton.onclick=()=>fetchController?.abort();
minimizeButton.onclick=minimize;
dockOpen.onclick=restore;dockClose.onclick=close;
copyButton.onclick=()=>copyAndMinimize(answerText);
backdrop.addEventListener('mousedown',event=>{if(event.target===backdrop)close();});
dialog.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();close();}});
globalThis.TaskDeckAITroubleshoot={openForSelection:(view,text)=>open(view,text),openForFailure:openFailure,notifyFailure,executeOne,
 minimize,restore,close,rememberSubmission,
 get visible(){return backdrop.classList.contains('visible')},
 get minimized(){return minimized},
 get dockViewId(){return dockOwner?.meta?.id||''}};
})();
