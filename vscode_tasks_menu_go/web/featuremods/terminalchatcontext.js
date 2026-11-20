// Attach immutable terminal-buffer snapshots to the currently selected Agent
// Chat session. This module never reads another view, executes a command, calls
// a provider, or implements a fallback attachment API.
const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal chat context');

const MAX_ATTACHMENT_BYTES=32*1024;
const CONSOLE_BUDGET_BYTES=25*1024; // Reserve Coder 1's metadata wrapper under the 32 KiB backend cap.
const TERMINAL_READ_PERMISSION='terminal.view_own';
const wiredViews=new WeakMap();
const activeViews=new Set();

const style=document.createElement('style');
style.textContent=[
'.terminal-chat-context-menu{position:fixed;z-index:9300;min-width:220px;max-width:min(340px,92vw);padding:5px;border:1px solid #465160;border-radius:7px;background:#171a20;color:#e7edf5;box-shadow:0 10px 28px #0008}',
'.terminal-chat-context-menu[hidden]{display:none}',
'.terminal-chat-context-menu button{display:block;width:100%;text-align:left;padding:6px 8px;border:0;border-radius:4px;background:transparent;color:inherit;font:12px system-ui,sans-serif;cursor:pointer}',
'.terminal-chat-context-menu button:hover,.terminal-chat-context-menu button:focus-visible{background:#2b3440;outline:none}',
'.terminal-chat-context-menu button:disabled{opacity:.45;cursor:not-allowed}',
'.terminal-chat-context-menu .terminal-chat-menu-heading{padding:5px 8px 3px;font-size:10px;font-weight:700;opacity:.6}',
'html[data-taskmenu-theme="light"] .terminal-chat-context-menu{background:#fff;color:#18212b;border-color:#bac3ce;box-shadow:0 10px 28px #0003}',
'html[data-taskmenu-theme="light"] .terminal-chat-context-menu button:hover,html[data-taskmenu-theme="light"] .terminal-chat-context-menu button:focus-visible{background:#edf2f7}'
].join('\n');
document.head.append(style);

function textBytes(value){return new TextEncoder().encode(String(value||'')).length;}
function validView(view){
 if(!view||view.closed||!view.meta||!view.term||!view.pane)return false;
 const id=String(view.meta.id||'');
 return Boolean(id&&app.views?.get?.(id)===view);
}
function canReadTerminal(view){
 if(!validView(view))return false;
 if(app.sharedMode&&!app.hasPermission?.(TERMINAL_READ_PERMISSION)&&
    !app.hasPermission?.('terminal.view_all')&&!app.hasPermission?.('sessions.manage'))return false;
 return true;
}
function safeMeta(value,max=200){
 return String(value||'').replace(/[\x00-\x1f\x7f]/g,' ').trim().slice(0,max);
}
function terminalTitle(view){
 return safeMeta(view.meta.title||view.meta.label||view.tab?.textContent||'Terminal',160);
}
function captureMetadata(view,sourceType,extra={}){
 return {
  terminal_id:safeMeta(view.meta.id,128),
  terminal_title:terminalTitle(view),
  cwd:safeMeta(view.meta.cwd||view.meta.current_cwd||'',1024),
  captured_at:new Date().toISOString(),
  source:'terminal_buffer',
  source_type:sourceType,
  ...extra,
 };
}
function stripTerminalControls(value){
 return String(value||'')
  .replace(/\u001b\][^\u0007]*(?:\u0007|\u001b\\)/g,'')
  .replace(/\u001b\[[0-?]*[ -/]*[@-~]/g,'')
  .replace(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g,'');
}
function cleanTerminalText(value){
 // app.consoleText translates xterm's rendered buffer, so console snapshots
 // use stable LF separators. Selection snapshots use stripTerminalControls
 // directly to preserve the exact xterm selection's newline representation.
 return stripTerminalControls(value).replace(/\r\n?/g,'\n');
}
function getConsoleSnapshot(view){
 if(!canReadTerminal(view))throw new Error('This terminal is closed or not readable.');
 if(typeof app.consoleText!=='function')throw new Error('Terminal buffer snapshot API is unavailable.');
 const snapshot=cleanTerminalText(app.consoleText(view));
 if(!snapshot.trim())throw new Error('This terminal has no console output to attach.');
 return snapshot;
}
function tailByLines(text,lineCount){
 const lines=text.split('\n');
 const count=Math.max(1,Math.min(lines.length,Math.floor(lineCount)||1));
 return lines.slice(-count).join('\n');
}
function selectionPosition(view){
 try{
  const p=view.term.getSelectionPosition?.();
  if(!p)return {};
  const point=value=>value&&typeof value==='object'?{column:Number(value.x??value.column??0),buffer_row:Number(value.y??value.row??0)}:undefined;
  return {selection_start:point(p.start),selection_end:point(p.end)};
 }catch{return {};}
}
function formatHeader(sourceType,meta,consoleText,extra={}){
 const lines=[
  'SOURCE_TYPE: '+sourceType,
  'TERMINAL_ID: '+meta.terminal_id,
  'TERMINAL_TITLE: '+meta.terminal_title,
  'CWD: '+meta.cwd,
  'CAPTURED_AT: '+meta.captured_at,
  'SOURCE: terminal_buffer',
  'CONTENT_SOURCE: terminal_buffer',
  'CAPTURED_LINES: '+consoleText.split('\\n').length,
  'ATTACHED_BYTES: '+textBytes(consoleText),
  ...(extra.truncation?[
   'TRUNCATED: true',
   'CAPTURE_MODE: '+extra.captureMode,
   'ORIGINAL_BYTES: '+extra.originalBytes,
   'OMITTED_PREFIX_BYTES: '+extra.omittedBytes,
  ]:['TRUNCATED: false']),
  ...(extra.position?.selection_start?[
   'SELECTION_START_BUFFER_ROW: '+extra.position.selection_start.buffer_row,
   'SELECTION_START_COLUMN: '+extra.position.selection_start.column,
  ]:[]),
  ...(extra.position?.selection_end?[
   'SELECTION_END_BUFFER_ROW: '+extra.position.selection_end.buffer_row,
   'SELECTION_END_COLUMN: '+extra.position.selection_end.column,
  ]:[]),
  '',
  sourceType==='terminal_console'?'--- CONSOLE ---':'--- SELECTED CONSOLE ---',
 ];
 return lines.join('\n')+'\n'+consoleText;
}
function attachmentPayload(view,sourceType,content,extra={}){
 const meta=extra.metadata||captureMetadata(view,sourceType,extra.position||{});
 const text=formatHeader(sourceType,meta,content,extra);
 if(textBytes(text)>CONSOLE_BUDGET_BYTES)throw new Error('The terminal snapshot exceeds the 32 KiB backend limit after Agent Chat metadata is added. Choose fewer lines or a smaller selection.');
 const source={...meta,kind:sourceType,content_source:'terminal_buffer',captured_at:meta.captured_at,
  lines:content.split('\\n').length,bytes:textBytes(content),truncated:Boolean(extra.truncation),
  capture_mode:extra.captureMode||'full',original_bytes:extra.originalBytes??textBytes(content),
  attached_bytes:textBytes(content),omitted_prefix_bytes:extra.omittedBytes||0,
  ...(extra.position||{})};
 const bytes=textBytes(text);
 if(bytes>MAX_ATTACHMENT_BYTES)throw new Error('The selected terminal snapshot still exceeds the 32 KiB attachment limit. Choose fewer lines or a smaller selection.');
 const filename=(sourceType==='terminal_console'?'terminal-console-':'terminal-selection-')+
  (meta.terminal_id.replace(/[^A-Za-z0-9._-]/g,'-')||'terminal')+'.txt';
 return {
  kind:sourceType,
  name:filename,
  mime:'text/plain',
  text,
  source,
 };
}
function stageContext(){
 const api=globalThis.TaskMenuAgentChat;
 if(typeof api?.stageContext!=='function')throw new Error('Agent Chat attachment staging is not available in this build. Update the Agent Chat API before attaching terminal output.');
 return api.stageContext;
}
async function stagePayload(view,payload){
 if(!canReadTerminal(view))throw new Error('This terminal is closed or not readable.');
 const stage=stageContext();
 const result=await stage({kind:payload.kind,name:payload.name,mime:payload.mime,text:payload.text,source:payload.source},{open:true,focusComposer:true});
 if(!result?.ok)throw new Error(result?.error||'Agent Chat did not stage the terminal snapshot.');
 return result;
}
function requestTailLineCount(text,sourceLabel){
 const currentLines=text.split('\n').length;
 const answer=window.prompt(sourceLabel+' is too large for one Chat attachment. Enter the number of recent complete lines to attach (1–'+currentLines+'). Cancel leaves the selection unchanged.','200');
 if(answer===null)return null;
 const count=Number(answer);
 if(!Number.isInteger(count)||count<1||count>currentLines)throw new Error('Enter a valid number of recent lines.');
 return count;
}
async function attachConsole(view){
 // Capture metadata and buffer together at click-time, before the user chooses
 // any optional truncation. Later terminal output/retargeting cannot mutate it.
 const metadata=captureMetadata(view,'terminal_console');
 const snapshot=getConsoleSnapshot(view);
 const originalBytes=textBytes(snapshot);
 let content=snapshot;
 let extra={};
 // Reserve room for the required metadata header.
 if(textBytes(formatHeader('terminal_console',captureMetadata(view,'terminal_console'),snapshot))>MAX_ATTACHMENT_BYTES||originalBytes>CONSOLE_BUDGET_BYTES){
  const lines=requestTailLineCount(snapshot,'Terminal console');
  if(lines===null)return false;
  content=tailByLines(snapshot,lines);
  const attachedBytes=textBytes(content);
  extra={metadata,truncation:attachedBytes<originalBytes,captureMode:'recent_tail',originalBytes,omittedBytes:Math.max(0,originalBytes-attachedBytes)};
 }else{extra.metadata=metadata;}
 const payload=attachmentPayload(view,'terminal_console',content,extra);
 await stagePayload(view,payload);
 return true;
}
async function attachSelection(view,selectedText,position,metadata){
 if(!canReadTerminal(view))throw new Error('This terminal is closed or not readable.');
 const capturedMetadata=metadata||captureMetadata(view,'terminal_selection',position||{});
 let content=stripTerminalControls(selectedText);
 if(!content.trim())throw new Error('Select terminal text before attaching it.');
 const originalBytes=textBytes(content);
 let extra={position};
 const headerExtra={position};
 if(textBytes(formatHeader('terminal_selection',capturedMetadata,content,headerExtra))>CONSOLE_BUDGET_BYTES){
  const lines=requestTailLineCount(content,'Terminal selection');
  if(lines===null)return false;
  const sliced=tailByLines(content,lines);
  const attachedBytes=textBytes(sliced);
  extra={metadata:capturedMetadata,position,truncation:attachedBytes<originalBytes,captureMode:'selection_tail',originalBytes,omittedBytes:Math.max(0,originalBytes-attachedBytes)};
  content=sliced;
 }else{extra.metadata=capturedMetadata;}
 await stagePayload(view,attachmentPayload(view,'terminal_selection',content,extra));
 return true;
}
function terminalConsolePopover(view){
 const groups=[...(view?.pane?.querySelectorAll?.('.pane-action-menus .taskmenu-menu')||[])];
 const consoleGroup=groups.find(group=>String(group.querySelector?.(':scope > .taskmenu-menu-trigger')?.textContent||'').trim().startsWith('Console'));
 if(!consoleGroup)return null;
 return consoleGroup.querySelector?.(':scope > .taskmenu-menu-popover')||null;
}
function ensureConsoleAction(view){
 if(!validView(view)||!view.pane)return false;
 const popover=terminalConsolePopover(view);
 if(!popover)return false;
 let button=popover.querySelector?.(':scope > .terminal-chat-attach-console');
 if(button)return true;
 button=document.createElement('button');
 button.type='button';button.className='terminal-chat-attach-console';button.textContent='Attach console to Chat';
 button.title='Attach a snapshot of this terminal buffer to the currently selected Agent Chat session';
 button.addEventListener('click',()=>attachConsole(view).catch(error=>app.showError?.(error)));
 popover.append(button);
 return true;
}
function selectionMenu(view,text,position,x,y,keyboard=false,metadata){
 closeSelectionMenu();
 const menu=document.createElement('div');menu.className='terminal-chat-context-menu';menu.setAttribute('role','menu');menu.dataset.terminalId=String(view.meta.id||'');
 const heading=document.createElement('div');heading.className='terminal-chat-menu-heading';heading.textContent='Selected terminal text';menu.append(heading);
 const add=(label,run,disabled=false)=>{
  const button=document.createElement('button');button.type='button';button.setAttribute('role','menuitem');button.textContent=label;button.disabled=disabled;
  button.addEventListener('click',async event=>{event.preventDefault();event.stopPropagation();closeSelectionMenu();try{await run();}catch(error){app.showError?.(error);}});
  menu.append(button);return button;
 };
 add('Attach selection to Chat',()=>attachSelection(view,text,position,metadata),!canReadTerminal(view));
 add('Copy selection',async()=>{await navigator.clipboard.writeText(text);},!navigator.clipboard?.writeText);
 if(!view.term.options?.disableStdin)add('Paste',async()=>{const pasted=await navigator.clipboard?.readText?.();if(pasted)view.term.paste?.(pasted);});
 add('Select all',()=>view.term.selectAll?.());
 const search=view.pane?.querySelector?.('.console-search-btn');if(search)add('Find in console',()=>search.click());
 menu.addEventListener('keydown',event=>{
  const buttons=[...menu.querySelectorAll('button:not(:disabled)')];
  const index=buttons.indexOf(document.activeElement);
  if(event.key==='Escape'){event.preventDefault();closeSelectionMenu();view.term.focus?.();}
  else if(event.key==='ArrowDown'||event.key==='ArrowUp'){
   event.preventDefault();const next=(index+(event.key==='ArrowDown'?1:-1)+buttons.length)%buttons.length;buttons[next]?.focus();
  }else if(event.key==='Home'){event.preventDefault();buttons[0]?.focus();}
  else if(event.key==='End'){event.preventDefault();buttons[buttons.length-1]?.focus();}
 });
 document.body.append(menu);activePopup=menu;
 const left=Math.max(4,Math.min(x,window.innerWidth-menu.offsetWidth-4));
 const top=Math.max(4,Math.min(y,window.innerHeight-menu.offsetHeight-4));
 menu.style.left=left+'px';menu.style.top=top+'px';
 if(keyboard)menu.querySelector('button:not(:disabled)')?.focus();
 return menu;
}
let activePopup=null;
function closeSelectionMenu(){if(activePopup?.isConnected)activePopup.remove();activePopup=null;}
document.addEventListener('pointerdown',event=>{if(activePopup&&!activePopup.contains(event.target))closeSelectionMenu();},true);
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&activePopup)closeSelectionMenu();},true);
function installView(view){
 if(!validView(view))return false;
 const id=String(view.meta.id);
 let state=wiredViews.get(view);
 if(state&&state.id!==id)state.id=id; // retarget/reconnect without duplicate listeners
 if(!state){
  state={id,view,termElement:view.term.element,contextHandler:null,keyHandler:null};
  wiredViews.set(view,state);
  activeViews.add(view);
  state.contextHandler=event=>{
   const selected=String(view.term.getSelection?.()||'');
   if(!selected.trim())return; // Keep xterm/browser default context menu untouched when there is no selection.
   const snapshot=stripTerminalControls(selected);const position=selectionPosition(view);
   const metadata=captureMetadata(view,'terminal_selection',position);
   event.preventDefault();event.stopPropagation();
   selectionMenu(view,snapshot,position,event.clientX||8,event.clientY||8,false,metadata);
  };
  state.keyHandler=event=>{
   if(!(event.key==='ContextMenu'||(event.shiftKey&&event.key==='F10')))return;
   const selected=String(view.term.getSelection?.()||'');if(!selected.trim())return;
   event.preventDefault();event.stopPropagation();
   const position=selectionPosition(view);
   selectionMenu(view,stripTerminalControls(selected),position,8,8,true,captureMetadata(view,'terminal_selection',position));
  };
  state.termElement?.addEventListener?.('contextmenu',state.contextHandler);
  state.termElement?.addEventListener?.('keydown',state.keyHandler);
 }
 ensureConsoleAction(view);
 return true;
}
function disposeView(view){
 const state=wiredViews.get(view);if(!state)return;
 state.termElement?.removeEventListener?.('contextmenu',state.contextHandler);
 state.termElement?.removeEventListener?.('keydown',state.keyHandler);
 if(activePopup?.dataset?.terminalId===state.id)closeSelectionMenu();
 const button=terminalConsolePopover(view)?.querySelector?.(':scope > .terminal-chat-attach-console');button?.remove();
 wiredViews.delete(view);
 activeViews.delete(view);
}
function installAll(){
 for(const view of app.views?.values?.()||[])installView(view);
 for(const view of [...activeViews])if(!validView(view))disposeView(view);
}
// Menus decorates new panes on the same lifecycle event with setTimeout(0).
// Defer this module's pass by one task so the named Console popover exists before
// adding the action; repeated lifecycle events remain idempotent.
const scheduleInstallAll=()=>setTimeout(installAll,0);
window.addEventListener('taskmenu:session',scheduleInstallAll);
window.addEventListener('taskmenu:terminal-rendered',scheduleInstallAll);
window.addEventListener('taskmenu:session-retargeted',scheduleInstallAll);
window.addEventListener('taskmenu:terminal-cloned',scheduleInstallAll);
window.addEventListener('taskmenu:terminal-restored',scheduleInstallAll);
window.addEventListener('taskmenu:tasks',scheduleInstallAll);
installAll();

globalThis.TaskMenuTerminalChatContext={
 attachConsole,
 attachSelection,
 captureSelectionContext(view){
  if(!canReadTerminal(view))throw new Error('This terminal is closed or not readable.');
  const position=selectionPosition(view);
  return {position,metadata:captureMetadata(view,'terminal_selection',position)};
 },
 captureConsole:getConsoleSnapshot,
 installView,
 installAll,
 disposeView,
 get hasSelectionMenu(){return Boolean(activePopup?.isConnected);},
};
