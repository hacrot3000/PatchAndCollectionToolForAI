// Non-executable terminal completion UI. Source data is treated as untrusted text.
(function(){
'use strict';
const app=globalThis.TaskMenuApp,engine=globalThis.TaskDeckCompletionEngine;
if(!app||!engine)throw Error('Smart terminal prerequisites unavailable');
const integration=globalThis.TaskDeckShellIntegration;
const installed=new WeakSet(),byView=new Map(),osCache=new Map();
const sessionProjectHistory=[]; // In-memory only; includes tabs closed during this browser session.
const css=document.createElement('style');
css.textContent=[
 '.smartterm-suggestions{position:fixed;z-index:14900;display:none;flex-direction:column;min-width:250px;max-width:min(650px,85vw);max-height:260px;overflow:auto;border:1px solid #57718c;border-radius:7px;background:#17212d;box-shadow:0 12px 28px #0009;font:12px/1.4 ui-monospace,monospace}',
 '.smartterm-suggestions.visible{display:flex}',
 '.smartterm-option{background:transparent;border:0;border-radius:0;text-align:left;color:#e1edfa;font:inherit;padding:5px 10px;display:flex;gap:9px;align-items:center;cursor:pointer;min-height:26px}',
 '.smartterm-option span{flex:1;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
 '.smartterm-option small{color:#9db4c9;font:10px ui-monospace,monospace;flex-shrink:0}',
 '.smartterm-option.active,.smartterm-option:hover{background:#2e5679;color:#fff}',
 '.smartterm-ghost{position:fixed;z-index:14890;pointer-events:none;color:#97adc3;background:rgba(10,15,20,.55);white-space:pre;font:13px ui-monospace,monospace;display:none;max-width:40vw;overflow:hidden}',
 'html[data-taskmenu-theme="light"] .smartterm-suggestions{background:#f7fbff;border-color:#7d9cb7;box-shadow:0 8px 25px #0003}',
 'html[data-taskmenu-theme="light"] .smartterm-option{color:#26384c}',
 'html[data-taskmenu-theme="light"] .smartterm-option small{color:#55708c}',
 'html[data-taskmenu-theme="light"] .smartterm-option.active,html[data-taskmenu-theme="light"] .smartterm-option:hover{background:#bbddfa}',
 'html[data-taskmenu-theme="light"] .smartterm-ghost{color:#647c95;background:#ffffffbf}'
].join('\n');
document.head.append(css);
const menu=document.createElement('div');menu.className='smartterm-suggestions';menu.setAttribute('role','listbox');
const ghost=document.createElement('div');ghost.className='smartterm-ghost';
document.body.append(menu,ghost);
let activeID='';
function state(view){let s=byView.get(view.meta.id);if(!s){s={view,items:[],selected:-1,input:'',pending:false};byView.set(view.meta.id,s);}return s;}
function isActive(view){let id=String(app.active||'');if(id.startsWith('external:'))id=id.slice(9);return view?.meta?.id===id;}
function writable(view){return view?.canControl&&!view.tabReadOnly&&!app.browserLeaseLost&&view.meta?.status==='running'&&
 view.ws?.readyState===WebSocket.OPEN&&!view.closed;}
function ready(view){
 const shell=integration?.getState(view?.meta?.id);
 return writable(view)&&shell?.seenPrompt&&shell.commandRow!=null&&!shell.executing;
}
// Bash 4.x / remote login shells may omit OSC 133 prompt boundaries. Permit
// TaskDeck directives only at the *current editable Bash prompt*, never from
// arbitrary terminal output. A strict recognizable user@host prompt is needed
// when OSC integration cannot prove that we are at an idle command line.
function bashPromptLine(view){
 if(!writable(view))return null;
 // Never infer an interactive Bash prompt from task output, running scripts
 // or non-terminal sessions. In particular, AI and inventory scripts must
 // not be routed into an active TUI or patch process.
 const meta=view.meta||{};
 if(meta.kind&&meta.kind!=='terminal')return null;
 if(![0,-1].includes(meta.task_id==null?0:Number(meta.task_id)))return null;
 const buffer=view.term?.buffer?.active;
 if(!buffer)return null;
 const row=Number(buffer.baseY||0)+Number(buffer.cursorY||0);
 const line=buffer.getLine?.(row);
 if(!line||line.isWrapped)return null;
 const x=Number(buffer.cursorX||0),rendered=line.translateToString(false);
 if(!Number.isFinite(x)||x<1||x>rendered.length)return null;
 // Cursor must be at the end, not editing in the middle of a command.
 if(rendered.slice(x).trim())return null;
 const visible=rendered.slice(0,x);
 // Typical Bash prompts: user@host:/cwd$ ... or [user@host cwd]# ...
 const match=/^(?:[\w.-]+@[\w.-]+:[^\r\n]{0,400}|\[[\w.-]+@[\w.-]+ [^\r\n]{0,300}\])[#$] +([^\r\n\x00-\x1f\x7f]{0,1024})$/.exec(visible);
 return match?match[1]:null;
}
function readReservedLine(view){
 if(ready(view))return commandText(view);
 const shell=integration?.getState(view?.meta?.id);
 if(shell?.executing)return null;
 return bashPromptLine(view);
}
function commandText(view){
 if(!ready(view))return '';
 const shell=integration.getState(view.meta.id),buffer=view.term?.buffer?.active;
 if(!buffer)return '';
 const start=Number(shell.commandRow),col=Number(shell.commandCol||0);
 const cur=Number(buffer.baseY||0)+Number(buffer.cursorY||0);
 if(start<0||start>cur||cur-start>4)return '';
 let value='';
 for(let row=start;row<=cur;row++){
  const line=buffer.getLine(row);
  if(!line||(row>start&&!line.isWrapped))return '';
  const rendered=line.translateToString(false);
  value+=rendered.slice(row===start?col:0,row===cur?Number(buffer.cursorX||0):rendered.length);
 }
 if(value.length>1024||/[\r\n\x00-\x1f\x7f]/.test(value))return '';
 // Do not complete while the cursor sits inside a line instead of at its end.
 const line=buffer.getLine(cur),x=Number(buffer.cursorX||0);
 if(line?.translateToString(true).trimEnd().length>x)return '';
 return value;
}
// Completion can use the same strictly identified Bash prompt as local Help
// when the terminal lacks OSC 133 shell-integration markers. Do not treat
// arbitrary program output, e.g. "telnet>", as an editable shell prompt.
function suggestionLine(view){
 if(ready(view))return commandText(view);
 const shell=integration?.getState(view?.meta?.id);
 return shell?.executing?null:bashPromptLine(view);
}
function suggestionReady(view){return suggestionLine(view)!==null;}
function trace(view,stage,details){globalThis.TaskDeckCompletionTrace?.capture?.(view,stage,details);}
// Snapshot works even when autocomplete is being suppressed. It never sends
// input to the terminal or reads arbitrary PTY scrollback content.
function diagnose(view){
 if(!view?.term)return {reason:'terminal view or xterm missing'};
 const shell=integration?.getState?.(view.meta?.id)||null;
 const b=view.term.buffer?.active,cur=b?Number(b.baseY||0)+Number(b.cursorY||0):-1;
 const row=b?.getLine?.(cur),rendered=row?.translateToString?.(false)||'';
 const cursor=Number(b?.cursorX||0),line=suggestionLine(view);
 const fallback=bashPromptLine(view),aliases=state(view).aliases||null;
 const branches=line===null?null:engine.branchContext?.(line,aliases);
 const pathCtx=line===null?null:engine.pathContext?.(line,aliases);
 const s=state(view);
 return {
  session:{id:String(view.meta?.id||''),kind:view.meta?.kind||'',status:view.meta?.status||'',
   targetType:view.meta?.target_type||'',canControl:Boolean(view.canControl),
   readOnly:Boolean(view.tabReadOnly),socketState:view.ws?.readyState??null,closed:Boolean(view.closed)},
  shell:{seenPrompt:Boolean(shell?.seenPrompt),executing:Boolean(shell?.executing),
   promptRow:shell?.promptRow??null,commandRow:shell?.commandRow??null,commandCol:shell?.commandCol??null},
  buffer:{cursorX:cursor,cursorY:Number(b?.cursorY||0),baseY:Number(b?.baseY||0),
   rowIndex:cur,rowWrapped:Boolean(row?.isWrapped),rowLength:rendered.length,
   charsBeforeCursor:rendered.slice(0,Math.max(0,cursor)).length,
   charsAfterCursor:rendered.slice(Math.max(0,cursor)).trimEnd().length,
   // Only the shape of the rendered input is exposed, not arbitrary commands.
   hasShellPrompt:/[#$] /.test(rendered.slice(0,Math.max(0,cursor)))},
  analysis:{isActive:isActive(view),writable:writable(view),oscReady:ready(view),
   fallbackPromptMatched:fallback!==null,suggestionReady:line!==null,line:line??'',
   branchContext:branches?{kind:'branches',prefix:branches.prefix||''}:null,
   pathContext:pathCtx?{kind:'paths',dir:pathCtx.dir,prefix:pathCtx.prefix}:null,
   lookupAllowed:localPathLookupAllowed(view),lookupPending:Boolean(s.pathTimer),
   lookupKeyPresent:Boolean(s.pathLookupKey),responseReady:Boolean(s.pathResponse),
   responseEntries:Number(s.pathResponse?.entries?.length||0),
   resultCount:s.items?.length||0,popupVisible:menu.classList.contains('visible')}
 };
}
function cacheKey(view){const profile=String(view?.meta?.target_profile_id||'');return profile?'ssh:'+profile:'local';}
function localPathLookupAllowed(view){
 const meta=view?.meta||{};
 return suggestionReady(view)&&(!meta.kind||meta.kind==='terminal')&&
   (meta.task_id==null||Number(meta.task_id)===0)&&
   String(meta.target_type||'').toLowerCase()!=='ssh'&&!meta.target_profile_id;
}
function schedulePathLookup(view,typed,context){
 if(!localPathLookupAllowed(view)||!context){
  trace(view,'lookup-not-allowed',{reason:'prompt unverified, nonlocal session or not writable'});return;
 }
 const s=state(view),kind=context.kind||'paths',key=typed+'\x00'+kind+'\x00'+(context.dir||'')+'\x00'+context.prefix;
 if(s.pathLookupKey===key){
  trace(view,'lookup-cached-key',{kind,dir:context.dir||'',prefix:context.prefix||''});return;
 }
 if(s.pathTimer)clearTimeout(s.pathTimer);
 s.pathLookupKey=key;s.pathResponse=null;
 const sequence=(s.pathSequence||0)+1;s.pathSequence=sequence;
 trace(view,'lookup-scheduled',{sequence,kind,dir:context.dir||'',prefix:context.prefix||''});
 // This only makes a read-only, bounded HTTP request. Never inject commands.
 s.pathTimer=setTimeout(async()=>{
  s.pathTimer=null;
  if(view.closed||!isActive(view)||!suggestionReady(view)||s.pathSequence!==sequence||suggestionLine(view)!==typed){
   trace(view,'lookup-aborted-before-request',{sequence,reason:'inactive, prompt unverified or input changed'});
   if(s.pathSequence===sequence)invalidatePathLookup(view);
   return;
  }
  try{
   const params=new URLSearchParams({kind,dir:context.dir||'',prefix:context.prefix||''});
   trace(view,'lookup-request',{sequence,kind,dir:context.dir||'',prefix:context.prefix||''});
   const response=await app.jsonFetch('/api/sessions/'+encodeURIComponent(view.meta.id)+'/completion-paths?'+params);
   trace(view,'lookup-http-response',{sequence,kind,dir:context.dir||'',prefix:context.prefix||'',
    cwd:response?.cwd||'',entriesCount:Array.isArray(response?.entries)?response.entries.length:0,
    sample:(Array.isArray(response?.entries)?response.entries:[]).slice(0,8).map(item=>item?.name||''),
    branchesCount:Array.isArray(response?.branches)?response.branches.length:0,
    commandsCount:Array.isArray(response?.commands)?response.commands.length:0});
   if(view.closed||!isActive(view)||!suggestionReady(view)||s.pathSequence!==sequence||suggestionLine(view)!==typed){
    trace(view,'lookup-response-discarded',{sequence,reason:'view/prompt changed or request superseded'});return;
   }
   s.pathResponse={key,entries:Array.isArray(response?.entries)?response.entries:[],branches:Array.isArray(response?.branches)?response.branches:[],commands:Array.isArray(response?.commands)?response.commands:[]};
   schedule(view);
  }catch(error){
   trace(view,'lookup-error',{sequence,error:String(error?.message||error)});
   if(s.pathSequence===sequence)s.pathResponse={key,entries:[]};
  }
 },110);
}
function invalidatePathLookup(view){
 const s=state(view);
 if(s.pathTimer)clearTimeout(s.pathTimer);
 s.pathTimer=null;s.pathLookupKey='';s.pathResponse=null;s.pathSequence=(s.pathSequence||0)+1;
}
// When OSC is absent, never send alias -p to an unverified interactive app.
// Read-only startup declarations provide a bounded best-effort fallback.
function scheduleFallbackAliases(view){
 if(!localPathLookupAllowed(view))return;
 const s=state(view);
 if(s.aliasesLoaded||s.aliasesLoading||s.liveAliases)return;
 s.aliasesLoading=true;
 const id=String(view.meta.id),generation=s.aliasGeneration||0;
 void app.jsonFetch('/api/sessions/'+encodeURIComponent(id)+'/completion-paths?kind=aliases')
  .then(data=>{
   if(view.closed||(s.aliasGeneration||0)!==generation)return;
   s.aliasesLoaded=true;s.aliasesLoading=false;
   if(s.liveAliases)return;
   s.aliases=engine.parseAliases?.(Array.isArray(data?.aliases)?data.aliases:[])||Object.create(null);
   if(isActive(view))schedule(view);
  }).catch(()=>{
   if((s.aliasGeneration||0)===generation){s.aliasesLoaded=true;s.aliasesLoading=false;}
  });
}
function render(view){
 if(!isActive(view)||!suggestionReady(view)){
  trace(view,'render-skipped',{active:isActive(view),snapshot:globalThis.TaskDeckCompletionTrace?.enabled?diagnose(view):null});hide();return;
 }
 const s=state(view),text=suggestionLine(view);
 if(!text.trim()||text.startsWith('\\')){
  trace(view,'render-no-completion',{line:text,reason:'blank or reserved command'});hide();return;
 }
 if(s.input!==text)s.selected=-1;
 s.input=text;
 const current=[...(integration?.getCommands(view.meta.id)||[])].reverse();
 const project=sessionProjectHistory.filter(x=>x.sessionID!==view.meta.id).map(x=>({command:x.command,finishedAt:x.finishedAt}));
 for(const other of app.views.values()){
  if(other===view||!other.meta?.id)continue;
  const entries=integration?.getCommands(other.meta.id)||[];
  project.push(...entries);
 }
 project.sort((a,b)=>Number(b.finishedAt||0)-Number(a.finishedAt||0));
 const os=osCache.get(cacheKey(view))||{};
 scheduleFallbackAliases(view);
 const aliases=s.aliases||null;
 const branchContext=engine.branchContext?.(text,aliases);
 const pathContext=!branchContext?engine.pathContext?.(text,aliases):null;
 const commandPrefix=!branchContext&&!pathContext?engine.commandPrefix?.(text):null;
 const completion=branchContext?{...branchContext,kind:'branches',dir:''}:
  pathContext?{...pathContext,kind:'paths'}:
  commandPrefix?{kind:'commands',dir:'',prefix:commandPrefix}:null;
 trace(view,'render-context',{line:text,completion:completion?{kind:completion.kind,dir:completion.dir||'',prefix:completion.prefix||''}:null,
  lookupAllowed:localPathLookupAllowed(view),shell:globalThis.TaskDeckCompletionTrace?.enabled?diagnose(view)?.shell:null});
 if(completion&&localPathLookupAllowed(view))schedulePathLookup(view,text,completion);
 else if(s.pathLookupKey)invalidatePathLookup(view);
 const branchLimit=branchContext?30:10;
 const others=engine.rank(text,{current,project,history:os.history||[],commands:os.commands||[]},10);
 const paths=completion&&localPathLookupAllowed(view)&&s.pathResponse?.key===s.pathLookupKey
  ?(branchContext?engine.rankBranches(text,s.pathResponse.branches,aliases,branchLimit-4):
    commandPrefix?engine.rankExecutables(text,s.pathResponse.commands,10):
    engine.rankPaths(text,s.pathResponse.entries,10,aliases)):[];
 const selectedCommand=s.selected>=0?s.items[s.selected]?.command:'';
 const options=branchContext?engine.rankBranchOptions?.(text,aliases)||[]:[];
 const seen=new Set();
 // Show the full ordinary-sized branch list (scrollable, capped at 30),
 // rather than hiding later refs such as "testing" behind the first 7 entries.
 // Other suggestion types keep the previous compact 10-row limit.
 s.items=paths.concat(options,others).filter(item=>{
  if(seen.has(item.command))return false;seen.add(item.command);return true;
 }).slice(0,branchLimit);
 trace(view,'render-ranked',{line:text,total:s.items.length,paths:paths.length,history:others.length,
  responseEntries:s.pathResponse?.entries?.length||0,apiPending:Boolean(s.pathTimer),lookupKeyPresent:Boolean(s.pathLookupKey)});
 if(!s.items.length){trace(view,'render-empty',{line:text,reason:'no matches, no response, or suggestions filtered'});hide();return;}
 // Async path responses must not silently redirect a keyboard selection
 // to another suggestion when their ranking changes.
 if(selectedCommand)s.selected=s.items.findIndex(item=>item.command===selectedCommand);
 else if(s.selected>=s.items.length)s.selected=-1;
 activeID=view.meta.id;
 menu.replaceChildren();
 s.items.forEach((entry,i)=>{
  const button=document.createElement('button');button.type='button';button.className='smartterm-option'+(i===s.selected?' active':'');
  button.setAttribute('role','option');button.setAttribute('aria-selected',String(i===s.selected));
  const label=document.createElement('span');label.textContent=entry.command;
  const source=document.createElement('small');source.textContent=entry.source;
  button.append(label,source);
  button.addEventListener('mousedown',event=>event.preventDefault());
  button.addEventListener('click',()=>{s.selected=i;accept(view);view.term?.focus();});
  menu.append(button);
 });
 const bounds=view.term.element?.querySelector('.xterm-screen')?.getBoundingClientRect();
 if(!bounds){trace(view,'popup-missing-screen',{reason:'xterm screen DOM not found'});hide();return;}
 const buffer=view.term.buffer.active,cursorX=Number(buffer.cursorX||0),cursorY=Number(buffer.cursorY||0);
 const dimensions=view.term._core?._renderService?.dimensions?.css?.cell;
 const width=Number(dimensions?.width)||Number(view.term.options.fontSize||13)*.64;
 const height=Number(dimensions?.height)||Number(view.term.options.fontSize||13)*1.3;
 const left=Math.max(4,Math.min(window.innerWidth-260,bounds.left+cursorX*width));
 const top=bounds.top+cursorY*height;
 menu.style.left=left+'px';
 menu.style.top=Math.max(4,Math.min(window.innerHeight-110,top-Math.min(260,s.items.length*27+4)-4))+'px';
 menu.classList.add('visible');
 trace(view,'popup-visible',{line:text,count:s.items.length,sample:s.items.slice(0,5).map(item=>item.command)});
 const suffix=engine.suffix(text,s.items[s.selected<0?0:s.selected]);
 if(suffix&&!/[\r\n]/.test(suffix)){
  ghost.textContent=suffix;
  ghost.style.fontSize=String(view.term.options.fontSize||13)+'px';
  ghost.style.left=Math.min(window.innerWidth-25,left)+'px';
  ghost.style.top=top+'px';
  ghost.style.lineHeight=height+'px';
  ghost.style.display='block';
 }else ghost.style.display='none';
}
function hide(){menu.classList.remove('visible');ghost.style.display='none';activeID='';}
function schedule(view){
 const s=state(view);if(s.pending)return;s.pending=true;
 trace(view,'render-scheduled',{active:isActive(view)});
 requestAnimationFrame(()=>{s.pending=false;if(!view.closed)render(view);});
}
function accept(view){
 const s=state(view);if(!isActive(view)||!suggestionReady(view)||!s.items.length)return false;
 const actual=suggestionLine(view),choice=s.items[s.selected<0?0:s.selected]?.command;
 if(!actual||actual!==s.input||!choice||!writable(view))return false;
 const append=choice.startsWith(actual);
 // Only the explicitly selected completion is sent: ghost text and arrow
 // navigation never transmit anything to either PTY or Broadcast recipients.
 const input=append?choice.slice(actual.length):'\x15'+choice;
 if(input){
  // Term.onData only fires for real keypresses. Broadcast must explicitly
  // receive autocomplete's synthetic bytes through its input queue before
  // the next Enter from the keyboard (which is already broadcast normally).
  const broadcast=globalThis.TaskMenuBroadcast;
  if(broadcast?.forwardCompletion?.(view,input,{replacesLine:!append})===false)return false;
  view.ws.send(input);
 }
 s.selected=-1;hide();return true;
}
// xterm's custom-key-handler return=false only suppresses xterm itself;
// it does NOT prevent the browser's native Tab focus navigation.
function consumeKey(event){
 event.preventDefault();
 event.stopPropagation();
}
function navigation(view,event){
 if(event.type!=='keydown'||event.isComposing||!isActive(view))return true;
 const s=state(view);
 if(activeID!==view.meta.id||!s.items.length||!menu.classList.contains('visible'))return true;
 // Swallow Tab whenever our popup owns it. Even when the shell prompt changes
 // between selection and keydown, never allow focus to jump to browser chrome.
 if(event.key==='Tab'&&!event.shiftKey&&!event.altKey&&!event.ctrlKey&&!event.metaKey){
  consumeKey(event);
  if(suggestionReady(view)){if(!accept(view))hide();}
  else hide();
  view.term?.focus();
  return false;
 }
 if(!suggestionReady(view)){hide();return true;}
 if(event.key==='ArrowDown'||event.key==='ArrowUp'){
  consumeKey(event);
  s.selected=event.key==='ArrowDown'?Math.min(s.items.length-1,s.selected+1):Math.max(-1,s.selected-1);
  render(view);
  view.term?.focus();
  return false;
 }
 if(event.key==='Escape'){consumeKey(event);hide();view.term?.focus();return false;}
 return true;
}
function install(view){
 if(!view?.term||installed.has(view))return;installed.add(view);state(view);
 view.term.attachCustomKeyEventHandler(event=>navigation(view,event));
}
// Hook called synchronously BEFORE xterm onData forwards to the PTY.
function onInput(view,data){
 try{
  trace(view,'ondata',{chars:String(data||'').length,category:/[\r\n]/.test(String(data||''))?'enter':String(data||'').startsWith('\\x1b')?'control':'text'});
  if(!writable(view)){trace(view,'input-not-writable');return false;}
  const s=state(view);
  if(String(data||'').includes('\r')||String(data||'').includes('\n')){s.selected=-1;hide();}
  else if(data==='\x03'||data==='\x1b'){s.selected=-1;hide();}
  // Reserved commands are handled by smartterminal_agents.js, never by completion UI.
  return globalThis.TaskDeckTerminalAgents?.onInput?.(view,data)===true;
 }catch(error){console.error('Smart terminal interceptor failed closed',error);return true;}
}
menu.addEventListener('wheel',event=>{
 const view=app.views.get(activeID);if(!view)return;
 const s=state(view);if(!s.items.length)return;
 event.preventDefault();
 s.selected=Math.min(s.items.length-1,Math.max(0,s.selected+(event.deltaY>0?1:-1)));
 render(view);
},{passive:false});
function installAll(){for(const view of app.views.values())install(view);}
installAll();
window.addEventListener('taskmenu:session',event=>install(event.detail?.view));
window.addEventListener('taskmenu:view-activated',event=>{
 const view=app.views.get(event.detail?.id);
 if(view){install(view);scheduleFallbackAliases(view);schedule(view);}else hide();
});
window.addEventListener('taskmenu:output',event=>{const view=event.detail?.view;if(view&&isActive(view))schedule(view);});
 // PTY input echo is written by xterm asynchronously; recompute after the
 // write callback, when the prompt and current command are actually visible.
 // Multiple output chunks are coalesced by schedule() into one frame.
window.addEventListener('taskmenu:terminal-rendered',event=>{
 const view=event.detail?.view;
 if(view&&isActive(view))schedule(view);
});
window.addEventListener('resize',()=>{const v=app.views.get(activeID);if(v)schedule(v);});
window.addEventListener('taskmenu:shell-integration',event=>{
 const detail=event.detail,view=detail?.view;
 if(view?.meta?.id&&['command-finished','prompt-start'].includes(detail?.type))invalidatePathLookup(view);
 if(detail?.type==='command-finished'&&view?.meta?.id&&detail.command?.command){
  sessionProjectHistory.push({sessionID:view.meta.id,command:String(detail.command.command),
   finishedAt:Number(detail.command.finishedAt||Date.now())});
  if(sessionProjectHistory.length>1000)sessionProjectHistory.splice(0,sessionProjectHistory.length-1000);
 }
 if(view&&isActive(view)){
  if(['prompt','command-started'].includes(detail?.type))scheduleFallbackAliases(view);
  schedule(view);
 }
});
globalThis.TaskDeckSmartTerminal={
 onInput,hide,diagnose,
 refresh(view){if(view)schedule(view);},
 setOSIndex(view,kind,commands){
  if(!view||!['commands','history','aliases'].includes(kind))return;
   if(kind==='aliases'){
    const s=state(view);
    s.liveAliases=Array.isArray(commands)&&commands.length>0;
    s.aliases=engine.parseAliases?.(commands)||Object.create(null);
    if(!s.liveAliases){
     s.aliasGeneration=(s.aliasGeneration||0)+1;
     s.aliasesLoading=false;s.aliasesLoaded=false;
    }
    if(isActive(view))schedule(view);
    return;
   }
  const key=cacheKey(view),entry=osCache.get(key)||{};
  entry[kind]=[...new Set((Array.isArray(commands)?commands:[]).filter(v=>typeof v==='string'&&v.length<1024))].slice(0,12000);
  osCache.set(key,entry);schedule(view);
 },
 getOSIndex(view){return {...(osCache.get(cacheKey(view))||{}),aliases:{...(state(view).aliases||{})}};},
 canIntercept(view){
  if(ready(view))return true;
  const shell=integration?.getState(view?.meta?.id);
  return !shell?.executing&&bashPromptLine(view)!==null;
 },getInputLine:commandText,readReservedLine
};
})();
