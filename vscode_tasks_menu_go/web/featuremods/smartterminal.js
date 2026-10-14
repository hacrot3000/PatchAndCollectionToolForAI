// Non-executable terminal completion UI. Source data is treated as untrusted text.
(function(){
'use strict';
const app=globalThis.TaskMenuApp,engine=globalThis.TaskDeckCompletionEngine;
if(!app||!engine)throw Error('Smart terminal prerequisites unavailable');
const integration=globalThis.TaskDeckShellIntegration;
const installed=new WeakSet(),byView=new Map(),osCache=new Map();
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
  value+=line.translateToString(true).slice(row===start?col:0);
 }
 if(value.length>1024||/[\r\n\x00-\x1f\x7f]/.test(value))return '';
 // Do not complete while the cursor sits inside a line instead of at its end.
 const line=buffer.getLine(cur),x=Number(buffer.cursorX||0);
 if(line?.translateToString(true).trimEnd().length>x)return '';
 return value;
}
function cacheKey(view){const profile=String(view?.meta?.target_profile_id||'');return profile?'ssh:'+profile:'local';}
function render(view){
 if(!isActive(view)||!ready(view)){hide();return;}
 const s=state(view),text=commandText(view);
 if(!text.trim()||text.startsWith('\\')){hide();return;}
 s.input=text;
 const current=[...(integration?.getCommands(view.meta.id)||[])].reverse();
 const project=[];
 for(const other of app.views.values()){
  if(other===view||!other.meta?.id)continue;
  const entries=integration?.getCommands(other.meta.id)||[];
  project.push(...entries);
 }
 project.sort((a,b)=>Number(b.finishedAt||0)-Number(a.finishedAt||0));
 const os=osCache.get(cacheKey(view))||{};
 s.items=engine.rank(text,{current,project,history:os.history||[],commands:os.commands||[]},10);
 if(!s.items.length){hide();return;}
 if(s.selected>=s.items.length)s.selected=-1;
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
 if(!bounds){hide();return;}
 const buffer=view.term.buffer.active,cursorX=Number(buffer.cursorX||0),cursorY=Number(buffer.cursorY||0);
 const dimensions=view.term._core?._renderService?.dimensions?.css?.cell;
 const width=Number(dimensions?.width)||Number(view.term.options.fontSize||13)*.64;
 const height=Number(dimensions?.height)||Number(view.term.options.fontSize||13)*1.3;
 const left=Math.max(4,Math.min(window.innerWidth-260,bounds.left+cursorX*width));
 const top=bounds.top+cursorY*height;
 menu.style.left=left+'px';
 menu.style.top=Math.max(4,Math.min(window.innerHeight-110,top-Math.min(260,s.items.length*27+4)-4))+'px';
 menu.classList.add('visible');
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
 requestAnimationFrame(()=>{s.pending=false;if(!view.closed)render(view);});
}
function accept(view){
 const s=state(view);if(!isActive(view)||!ready(view)||!s.items.length)return false;
 const actual=commandText(view),choice=s.items[s.selected<0?0:s.selected]?.command;
 if(!actual||actual!==s.input||!choice||!writable(view))return false;
 if(choice.startsWith(actual)){
  const remainder=choice.slice(actual.length);
  if(remainder)view.ws.send(remainder);
 }else{
  // Bash Readline kill-whole-line, replacing exactly the confirmed idle prompt.
  view.ws.send('\x15'+choice);
 }
 s.selected=-1;hide();return true;
}
function navigation(view,event){
 if(event.type!=='keydown'||event.isComposing||!isActive(view)||!ready(view))return true;
 const s=state(view);
 if(activeID!==view.meta.id||!s.items.length||!menu.classList.contains('visible'))return true;
 if(event.key==='ArrowDown'||event.key==='ArrowUp'){
  s.selected=event.key==='ArrowDown'?Math.min(s.items.length-1,s.selected+1):Math.max(-1,s.selected-1);
  render(view);return false;
 }
 if(event.key==='Tab'){if(accept(view))return false;}
 if(event.key==='Escape'){hide();return false;}
 return true;
}
function install(view){
 if(!view?.term||installed.has(view))return;installed.add(view);state(view);
 view.term.attachCustomKeyEventHandler(event=>navigation(view,event));
}
// Hook called synchronously BEFORE xterm onData forwards to the PTY.
function onInput(view,data){
 try{
  if(!writable(view))return false;
  const s=state(view);
  if(String(data||'').includes('\r')||String(data||'').includes('\n')){s.selected=-1;hide();}
  else if(data==='\x03'||data==='\x1b'){s.selected=-1;hide();}
  // Reserved commands are handled by smartterminal_agents.js, never by completion UI.
  return globalThis.TaskDeckTerminalAgents?.onInput?.(view,data)===true;
 }catch(error){console.error('Smart terminal interceptor failed closed',error);return true;}
}
function installAll(){for(const view of app.views.values())install(view);}
installAll();
window.addEventListener('taskmenu:session',event=>install(event.detail?.view));
window.addEventListener('taskmenu:view-activated',event=>{
 const view=app.views.get(event.detail?.id);if(view){install(view);schedule(view);}else hide();
});
window.addEventListener('taskmenu:output',event=>{const view=event.detail?.view;if(view&&isActive(view))schedule(view);});
window.addEventListener('resize',()=>{const v=app.views.get(activeID);if(v)schedule(v);});
window.addEventListener('taskmenu:shell-integration',event=>{if(event.detail?.view&&isActive(event.detail.view))schedule(event.detail.view);});
globalThis.TaskDeckSmartTerminal={
 onInput,hide,
 refresh(view){if(view)schedule(view);},
 setOSIndex(view,kind,commands){
  if(!view||!['commands','history'].includes(kind))return;
  const key=cacheKey(view),entry=osCache.get(key)||{};
  entry[kind]=[...new Set((Array.isArray(commands)?commands:[]).filter(v=>typeof v==='string'&&v.length<1024))].slice(0,12000);
  osCache.set(key,entry);schedule(view);
 },
 getOSIndex(view){return {...(osCache.get(cacheKey(view))||{})};},
 canIntercept:ready,getInputLine:commandText
};
})();
