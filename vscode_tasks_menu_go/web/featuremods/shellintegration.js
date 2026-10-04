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
  if(!state){state={cwd:'',remote:isRemote(view),promptRow:null,commandRow:null,lastPromptAt:0,commands:[]};states.set(id,state);}
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
function finalizeCommand(view,status){
  const state=stateFor(view);if(state.commandRow==null)return;
  const endRow=absoluteCursorRow(view),commandLine=lineText(view.term.buffer.active,state.commandRow),outputStart=Math.min(endRow,state.commandRow+1),output=rowsText(view,outputStart,Math.max(outputStart,endRow-1));
  const item={command:commandLine.trim(),output,exitCode:status,startedAt:state.lastPromptAt||Date.now(),finishedAt:Date.now(),startRow:state.commandRow,endRow};
  if(item.command||item.output){state.commands.push(item);if(state.commands.length>maxCommands)state.commands.splice(0,state.commands.length-maxCommands);emit(view,'command-finished',{command:item});}
  state.commandRow=null;
}
function handleOSC133(view,value){
  value=String(value||'');const code=value.split(';',1)[0],state=stateFor(view);
  if(code==='A'){state.promptRow=absoluteCursorRow(view);state.lastPromptAt=Date.now();emit(view,'prompt');return true;}
  if(code==='B'){state.commandRow=absoluteCursorRow(view);emit(view,'command-started',{row:state.commandRow});return true;}
  if(code==='C'){emit(view,'command-output',{row:absoluteCursorRow(view)});return true;}
  if(code==='D'){finalizeCommand(view,parseExitStatus(value));return true;}
  return false;
}
function install(view){
  if(!view?.term||installed.has(view))return;installed.add(view);stateFor(view);
  try{view.term.parser.registerOscHandler(7,data=>handleOSC7(view,data));}catch(error){console.warn('TaskDeck OSC 7 handler unavailable',error);}
  try{view.term.parser.registerOscHandler(133,data=>handleOSC133(view,data));}catch(error){console.warn('TaskDeck OSC 133 handler unavailable',error);}
}
function installAll(){for(const view of app.views.values())install(view);}
installAll();
window.addEventListener('taskmenu:session',event=>install(event.detail?.view));
window.addEventListener('taskmenu:view-activated',installAll);

globalThis.TaskDeckShellIntegration={install,getState(id){return states.get(String(id||''))||null;},getCommands(id){return [...(states.get(String(id||''))?.commands||[])];},cwd(id){return states.get(String(id||''))?.cwd||'';}};
