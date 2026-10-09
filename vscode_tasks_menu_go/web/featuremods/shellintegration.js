const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for shell integration');

const installed=new WeakSet();
const states=new Map();
const maxCommands=200;

function viewID(view){return String(view?.meta?.id||'');}
function isRemote(view){return String(view?.meta?.target_type||'').toLowerCase()==='ssh'||Boolean(view?.meta?.target_profile_id);}
function stateFor(view){
  const id=viewID(view);
  let state=states.get(id);
  if(!state){state={cwd:'',remote:isRemote(view),promptRow:null,commandRow:null,commandCol:0,lastPromptAt:0,commands:[],executing:false,startedAt:0,finishedAt:0,exitCode:null,completedCount:0,seenPrompt:false};states.set(id,state);}
  return state;
}
function absoluteCursorRow(view){const buffer=view?.term?.buffer?.active;if(!buffer)return 0;return Number(buffer.baseY||0)+Number(buffer.cursorY||0);}
function lineText(buffer,row){const line=buffer?.getLine?.(row);return line?line.translateToString(true):'';}
function rowsText(view,start,end){
  const buffer=view?.term?.buffer?.active;if(!buffer)return '';
  const first=Math.max(0,Number(start)||0),last=Math.min(buffer.length-1,Math.max(first,Number(end)||first));
  const lines=[];
  for(let row=first;row<=last;row++){const line=buffer.getLine(row);if(!line)continue;const value=line.translateToString(true);if(line.isWrapped&&lines.length)lines[lines.length-1]+=value;else lines.push(value);}
  return lines.join('\n').replace(/\s+$/,'');
}
function emit(view,type,extra={}){const state=stateFor(view);window.dispatchEvent(new CustomEvent('taskmenu:shell-integration',{detail:{view,type,state,...extra}}));}
function parseOSC7(value){
  value=String(value||'').trim();if(!value.toLowerCase().startsWith('file://'))return '';
  const rest=value.slice(7),slash=rest.indexOf('/');if(slash<0)return '';let path=rest.slice(slash);try{path=decodeURIComponent(path);}catch{}return path.startsWith('/')?path:'';
}
function handleOSC7(view,value){const cwd=parseOSC7(value);if(!cwd)return true;const state=stateFor(view);state.cwd=cwd;state.remote=isRemote(view);view.shellCwd=cwd;if(!state.remote&&view.meta)view.meta.cwd=cwd;emit(view,'cwd',{cwd,remote:state.remote});return true;}
function parseExitStatus(value){const parts=String(value||'').split(';');if(parts[0]!=='D'||parts.length<2||parts[1]==='')return null;const status=Number(parts[1]);return Number.isInteger(status)?status:null;}
function commandText(view,row,col){
  const buffer=view?.term?.buffer?.active;if(!buffer)return {text:'',lastRow:row};
  const first=buffer.getLine(row);if(!first)return {text:'',lastRow:row};
  let text=first.translateToString(true).slice(Math.max(0,Number(col)||0)),lastRow=row;
  for(let next=row+1;next<buffer.length;next++){
    const line=buffer.getLine(next);if(!line?.isWrapped)break;
    text+=line.translateToString(true);lastRow=next;
  }
  return {text:text.trim(),lastRow};
}
// OSC 133;C is emitted by Bash PS0 immediately before execution. An
// onData Enter fallback below covers Bash 4.2/4.3 that lacks PS0.
function beginExecution(view,source){
  const state=stateFor(view);
  if((state.commandRow==null&&source!=='shell-preexec')||state.executing||view?.meta?.status!=='running')return false;
  state.executing=true;state.startedAt=Date.now();state.finishedAt=0;state.exitCode=null;
  emit(view,'execution-started',{source,startedAt:state.startedAt});
  return true;
}
function fallbackInput(view,data){
  const state=stateFor(view);
  if(!state.seenPrompt||state.commandRow==null||state.executing||!/[\r\n]/.test(String(data||'')))return;
  // Only a nonempty command entered at a known interactive prompt can
  // count as execution; merely opening a terminal or pressing Enter on an
  // empty prompt must never trigger a running icon.
  const typed=commandText(view,state.commandRow,state.commandCol).text;
  if(typed)beginExecution(view,'interactive-enter');
}
function finalizeCommand(view,status){
  const state=stateFor(view);if(state.commandRow==null&&!state.executing)return;
  const wasExecuting=state.executing;
  // Some interactive terminals omit OSC 133;B during early prompt redraw.
  // A reliable shell preexec C and completion D must still update activity.
  if(state.commandRow!=null){
    const endRow=absoluteCursorRow(view),parsed=commandText(view,state.commandRow,state.commandCol),outputStart=Math.min(endRow,parsed.lastRow+1),output=rowsText(view,outputStart,Math.max(outputStart,endRow-1));
    const item={command:parsed.text,output,exitCode:status,cwd:state.cwd||'',remote:Boolean(state.remote),startedAt:state.lastPromptAt||Date.now(),finishedAt:Date.now(),startRow:state.commandRow,endRow};
    if(item.command||item.output){state.commands.push(item);if(state.commands.length>maxCommands)state.commands.splice(0,state.commands.length-maxCommands);emit(view,'command-finished',{command:item});}
  }
  state.commandRow=null;state.commandCol=0;
  if(wasExecuting){
    state.executing=false;state.finishedAt=Date.now();state.exitCode=status;state.completedCount++;
    emit(view,'execution-finished',{exitCode:status,finishedAt:state.finishedAt,startedAt:state.startedAt,completedCount:state.completedCount});
  }
}
function handleOSC133(view,value){
  value=String(value||'');const code=value.split(';',1)[0],state=stateFor(view);
  if(code==='A'){state.promptRow=absoluteCursorRow(view);state.lastPromptAt=Date.now();state.seenPrompt=true;emit(view,'prompt');return true;}
  if(code==='B'){state.commandRow=absoluteCursorRow(view);state.commandCol=Number(view?.term?.buffer?.active?.cursorX||0);emit(view,'command-started',{row:state.commandRow,col:state.commandCol});return true;}
  if(code==='C'){beginExecution(view,'shell-preexec');emit(view,'command-output',{row:absoluteCursorRow(view)});return true;}
  if(code==='D'){finalizeCommand(view,parseExitStatus(value));return true;}
  return false;
}
function install(view){
  if(!view?.term||installed.has(view))return;installed.add(view);stateFor(view);
  try{view.term.parser.registerOscHandler(7,data=>handleOSC7(view,data));}catch(error){console.warn('TaskDeck OSC 7 handler unavailable',error);}
  try{view.term.parser.registerOscHandler(133,data=>handleOSC133(view,data));}catch(error){console.warn('TaskDeck OSC 133 handler unavailable',error);}
  try{view.term.onData(data=>fallbackInput(view,data));}catch(error){console.warn('TaskDeck terminal input monitoring unavailable',error);}
}
function replaceCommands(id,commands){
  id=String(id||'');const state=states.get(id);if(!state)return [];
  state.commands=Array.isArray(commands)?commands.slice(-maxCommands):[];
  return [...state.commands];
}
function installAll(){for(const view of app.views.values())install(view);}
installAll();
window.addEventListener('taskmenu:session',event=>install(event.detail?.view));
window.addEventListener('taskmenu:view-activated',installAll);

globalThis.TaskDeckShellIntegration={install,getState(id){return states.get(String(id||''))||null;},getCommands(id){return [...(states.get(String(id||''))?.commands||[])];},replaceCommands,cwd(id){return states.get(String(id||''))?.cwd||'';},maxCommands};
