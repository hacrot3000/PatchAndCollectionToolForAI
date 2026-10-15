const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const core=require('./web/featuremods/ai_terminal_actions_core.js');
const source=fs.readFileSync(path.join(__dirname,'web/featuremods/ai_terminal_troubleshoot.js'),'utf8');
function element(tag='div'){
 const children=[],events=new Map(),classes=new Set();
 const node={tag,children,events,style:{removeProperty(name){delete this[name];}},isConnected:true,value:'',hidden:false,disabled:false,textContent:'',className:'',
  append(...items){children.push(...items);},
  replaceChildren(...items){children.splice(0,children.length,...items);},
  setAttribute(key,value){this[key]=value;},
  addEventListener(name,fn){events.set(name,fn);},
  remove(){this.removed=true;this.isConnected=false;},
  getBoundingClientRect(){return this.rect||{left:160,top:80,width:640,height:400};},
  focus(){},
  contains(target){return target===this||children.some(x=>x.contains?.(target));}};
 node.classList={add(v){classes.add(v)},remove(v){classes.delete(v)},contains(v){return classes.has(v)}};
 return node;
}
function fixture({confirm=true,providerConsent=true}={}){
 const listenerDoc=new Map(),listenerWindow=new Map(),requests=[],sent=[],confirmations=[];
 const root={head:element('head'),body:element('body'),
  createElement:element,addEventListener:(name,fn)=>listenerDoc.set(name,fn)};
 const line=element('span');line.closest=selector=>selector==='.xterm-screen'?line:null;
 let output='fatal: The current branch has no upstream';
 const session={meta:{id:'terminal-1',status:'running',task_id:0,kind:'terminal'},closed:false,
  pane:element('div'),
  tabReadOnly:false,canControl:true,ws:{readyState:1,send:command=>sent.push(command)},
  term:{element:element('div'),getSelection:()=>output,buffer:{active:{
   baseY:0,cursorY:0,getLine:()=>({translateToString:()=>'$ git> '})}}}};
 session.term.element.contains=x=>x===line;
 const state={seenPrompt:true,executing:false,commandRow:0,commandCol:7,startedAt:0,commands:[]};
 const app={active:'terminal-1',views:new Map([['terminal-1',session]]),browserLeaseLost:false,
  jsonFetch:async(url,options)=>{
   requests.push({url,options});
   if(url==='/api/ai/settings')return {enabled:true,consent:providerConsent,has_api_key:true,provider:'gemini',max_output_bytes:5000};
   if(url.startsWith('/api/ai/session-context'))return {verified:true,session_id:'terminal-1',remote:false,host:'local'};
   if(url==='/api/ai/assist')return {answer:JSON.stringify({summary:'Missing Git upstream',
    commands:[{command:'git push --set-upstream origin develop/package_event_queue',
     purpose:'Set the upstream tracking branch',risk:'medium'}]})};
   throw Error('unexpected '+url);
  }};
 const global={TaskMenuApp:app,TaskDeckAITerminalActionsCore:core,
  TaskDeckShellIntegration:{getState:()=>state,getCommands:()=>[]},navigator:{clipboard:{writeText:async()=>{}}},
  window:{innerWidth:800,innerHeight:650,confirm:message=>{confirmations.push(message);return confirm},
   addEventListener:(name,fn)=>listenerWindow.set(name,fn)},
  document:root,console,AbortController,Promise,setTimeout,clearTimeout,Map};
 global.globalThis=global;vm.createContext(global);vm.runInContext(source,global);
 const overlay=root.body.children[0],panel=overlay.children[0];
 const getButtons=()=>panel.children[7].children;
 const list=panel.children[10];
 const status=panel.children[8];
 async function flush(){for(let i=0;i<4;i++)await new Promise(resolve=>setTimeout(resolve,0));}
 function finishCommand(name='gp',exitCode=128,output='fatal: The current branch has no upstream',withLocalEnter=true){
  const handler=listenerWindow.get('taskmenu:shell-integration');
  if(withLocalEnter){
   state.startedAt=Date.now();state.activeCommand=name;
   handler({detail:{view:session,type:'execution-started',source:'interactive-enter',state}});
  }
  const command={command:name,exitCode,output,finishedAt:Date.now()};
  state.commands.push(command);
  handler({detail:{view:session,type:'command-finished',command,state}});
  return command;
 }
 return {api:global.TaskDeckAITroubleshoot,clientGlobal:global,root,app,session,state,requests,sent,confirmations,finishCommand,
  overlay,panel,getButtons,list,status,listenerDoc,listenerWindow,line,flush};
}
test('right click on selected xterm output offers Ask AI without uploading automatically',()=>{
 const h=fixture();
 let stopped=false;
 h.listenerDoc.get('contextmenu')({target:h.line,clientX:100,clientY:120,
  preventDefault(){stopped=true},stopPropagation(){}});
 assert.equal(stopped,true);
 const menu=h.root.body.children[1];
 assert.equal(menu.classList.contains('visible'),true);
 assert.equal(h.requests.length,0);
 menu.children[0].onclick();
 assert.equal(h.api.visible,true);
 assert.equal(h.panel.children[6].value,'fatal: The current branch has no upstream');
 assert.equal(h.requests.length,0,'context menu opens a preview, not an AI request');
});
test('nonzero exit code shows opt-in prompt; accepting opens wizard and requests AI JSON',async()=>{
 const h=fixture();
 h.finishCommand('gp',128,'fatal: The current branch has no upstream');
 const toast=h.root.body.children[2];
 assert.ok(toast.className.includes('ai-terminal-prompt'));
 assert.equal(toast.children.length,4,'Message, verified command, Yes and Not now');
 assert.equal(toast.children[0].textContent,'Ask AI Smart Terminal to help diagnose this error?');
 assert.equal(toast.children[1].textContent,'gp','Show exactly the last verified command');
 assert.equal(h.requests.length,0);
 toast.children[2].onclick();
 await h.flush();
 assert.ok(h.requests.some(r=>r.url==='/api/ai/assist'));
 const payload=JSON.parse(h.requests.find(r=>r.url==='/api/ai/assist').options.body);
 assert.equal(payload.mode,'troubleshoot');
 assert.equal(payload.command,'gp');
 assert.equal(payload.confirm_external,true);
 assert.ok(payload.output.includes('fatal:'));
 assert.equal(h.sent.length,0,'AI never executes a command from its response');
 assert.deepEqual(h.confirmations,[],'No second provider confirmation');
 assert.ok(h.list.children.length>0);
});
test('chosen AI proposal is only executed after confirmation and an idle verified shell',async()=>{
 const h=fixture();
 h.api.openForSelection(h.session,'fatal: missing upstream');
 h.getButtons()[0].onclick();await h.flush();
 assert.equal(h.sent.length,0);
 assert.deepEqual(h.confirmations,[],'Ask AI itself should not open another confirmation');
 const card=h.list.children[0];assert.ok(card);
 const run=card.children[3];await run.onclick();
 assert.deepEqual(h.sent,['git push --set-upstream origin develop/package_event_queue\r']);
 assert.ok(h.confirmations.some(text=>text.includes('Execute in this terminal')));
});
test('busy shell or partially typed command blocks execution; no PTY bytes are sent',async()=>{
 const h=fixture();h.api.openForSelection(h.session,'fatal');
 h.getButtons()[0].onclick();await h.flush();
 const run=h.list.children[0].children[3];
 h.state.executing=true;await run.onclick();assert.deepEqual(h.sent,[]);
 h.state.executing=false;h.session.term.buffer.active.getLine=()=>({translateToString:()=>'prompt> git status'});
 await run.onclick();assert.deepEqual(h.sent,[]);
});
test('Not now declines AI help without uploading the error',async()=>{
 const h=fixture();
 h.finishCommand('git push',1,'error');
 const toast=h.root.body.children[2];
 toast.children[3].onclick();await h.flush();
 assert.equal(h.requests.length,0);
 assert.deepEqual(h.confirmations,[]);
 assert.deepEqual(h.sent,[]);
});
test('AI settings consent still gates requests without another dialog',async()=>{
 const h=fixture({providerConsent:false});h.api.openForSelection(h.session,'fatal: missing upstream');
 h.getButtons()[0].onclick();await h.flush();
 assert.equal(h.requests.some(x=>x.url==='/api/ai/assist'),false);
 assert.deepEqual(h.confirmations,[]);
 assert.deepEqual(h.sent,[]);
});
test('Ask AI click sends context once, but Run retains its own confirmation',async()=>{
 const h=fixture({confirm:false});h.api.openForSelection(h.session,'fatal: missing upstream');
 assert.equal(h.requests.length,0);
 h.getButtons()[0].onclick();await h.flush();
 assert.equal(h.requests.filter(x=>x.url==='/api/ai/assist').length,1);
 assert.deepEqual(h.confirmations,[]);
 assert.deepEqual(h.sent,[]);
 const run=h.list.children[0].children[3];await run.onclick();
 assert.deepEqual(h.sent,[],'Declined execution confirmation must never send PTY input');
 assert.equal(h.confirmations.length,1,'Only command execution shows a confirmation');
});
test('only failing commands are offered assistance, not success, Ctrl+C or tasks',()=>{
 const h=fixture();
 for(const code of [0,130,null]){
  h.api.notifyFailure(h.session,{exitCode:code,command:'git status',finishedAt:code});
 }
 assert.equal(h.root.body.children.length,2);
 h.session.meta.task_id=17;
 h.api.notifyFailure(h.session,{exitCode:1,command:'task',finishedAt:42});
 assert.equal(h.root.body.children.length,2);
});


test('Copy action minimizes to the original terminal pane and Restore keeps the AI result',async()=>{
 const h=fixture();
 h.api.openForSelection(h.session,'fatal: missing upstream');
 h.getButtons()[0].onclick();await h.flush();
 const originalCard=h.list.children[0],originalSummary=h.panel.children[9].textContent;
 const requestsBefore=h.requests.filter(x=>x.url==='/api/ai/assist').length;
 await originalCard.children[2].onclick();
 assert.equal(h.api.minimized,true);
 assert.equal(h.api.visible,false);
 assert.equal(h.api.dockViewId,'terminal-1');
 const dock=h.session.pane.children[0];
 assert.equal(dock.className,'ai-terminal-dock');
 assert.equal(dock.children.length,2,'Restore icon + explicit Close action');
 assert.equal(dock.children[0].textContent,'✦  AI Smart Terminal');
 assert.equal(dock.children[1].textContent,'×');
 dock.children[0].onclick();await h.flush();
 assert.equal(h.api.minimized,false);
 assert.equal(h.api.visible,true);
 assert.equal(h.list.children[0],originalCard,'action cards must survive restoration');
 assert.equal(h.panel.children[9].textContent,originalSummary);
 assert.equal(h.requests.filter(x=>x.url==='/api/ai/assist').length,requestsBefore,'No repeat AI request');
});
test('Run this command minimizes only after successful explicit execution approval',async()=>{
 const h=fixture();
 h.api.openForSelection(h.session,'fatal: missing upstream');
 h.getButtons()[0].onclick();await h.flush();
 const run=h.list.children[0].children[3];
 await run.onclick();
 assert.deepEqual(h.sent,['git push --set-upstream origin develop/package_event_queue\r']);
 assert.equal(h.api.minimized,true);
 assert.equal(h.api.visible,false);
 assert.equal(h.session.pane.children[0].className,'ai-terminal-dock');
});
test('Declined Run confirmation or failed copy never minimizes the wizard',async()=>{
 const h=fixture({confirm:false});
 h.api.openForSelection(h.session,'fatal: missing upstream');
 h.getButtons()[0].onclick();await h.flush();
 await h.list.children[0].children[3].onclick();
 assert.equal(h.api.minimized,false);
 assert.equal(h.api.visible,true);
 assert.deepEqual(h.sent,[]);
});
test('Dock Close tears down the stored dialog; reopening creates a new session',async()=>{
 const h=fixture();
 h.api.openForSelection(h.session,'existing output');
 h.getButtons()[0].onclick();await h.flush();
 await h.list.children[0].children[2].onclick();
 const dock=h.session.pane.children[0];
 dock.children[1].onclick();await h.flush();
 assert.equal(h.api.visible,false);
 assert.equal(h.api.minimized,false);
 assert.equal(h.api.dockViewId,'');
 assert.equal(dock.removed,true);
 assert.equal(h.api.restore(),false);
});
test('split panes dock to the originating pane, not the globally active terminal',async()=>{
 const h=fixture();
 const otherPane=element('div'),other={...h.session,
  meta:{...h.session.meta,id:'terminal-2'},pane:otherPane};
 h.app.views.set('terminal-2',other);
 h.api.openForSelection(h.session,'selected log');
 h.getButtons()[0].onclick();await h.flush();
 // A split neighbor becomes active while this response remains open.
 h.app.active='terminal-2';
 await h.list.children[0].children[2].onclick();
 assert.equal(h.api.minimized,true);
 assert.equal(h.api.dockViewId,'terminal-1');
 assert.equal(h.session.pane.children.length,1);
 assert.equal(otherPane.children.length,0);
 h.session.pane.children[0].children[0].onclick();
 assert.equal(h.api.visible,true);
 assert.equal(h.requests.filter(r=>r.url==='/api/ai/assist').length,1);
});
test('dialog uses scale/translate animation toward pane dock and back',async()=>{
 const h=fixture();
 const frames=[];
 h.panel.animate=(keyframes,options)=>{
  frames.push({keyframes,options});
  return {finished:Promise.resolve(),cancel(){}};
 };
 h.panel.rect={left:110,top:60,width:600,height:360};
 h.api.openForSelection(h.session,'fatal');
 h.getButtons()[0].onclick();await h.flush();
 // Dock gets its own smaller screen rectangle, inside target pane.
 h.root.createElement; // no extra network or layout API needed
 await h.list.children[0].children[2].onclick();
 await h.flush();
 assert.equal(frames.length,1);
 assert.equal(frames[0].options.duration,320);
 assert.ok(frames[0].keyframes[1].transform.includes('scale('));
 h.session.pane.children[0].children[0].onclick();
 await h.flush();
 assert.equal(frames.length,2);
 assert.ok(frames[1].keyframes[0].transform.includes('scale('));
 assert.equal(h.api.visible,true);
 assert.equal(h.requests.filter(r=>r.url==='/api/ai/assist').length,1);
});

test('Copy JSON and manual Minimize use the same session dock',async()=>{
 const h=fixture();
 h.api.openForSelection(h.session,'error');
 h.getButtons()[0].onclick();await h.flush();
 const buttons=h.getButtons();
 await buttons[2].onclick();
 assert.equal(h.api.minimized,true,'Copy JSON should dock the existing wizard');
 h.session.pane.children[0].children[0].onclick();
 assert.equal(h.api.visible,true);
 buttons[4].onclick();
 assert.equal(h.api.minimized,true,'manual Minimize should also work');
 assert.equal(h.requests.filter(x=>x.url==='/api/ai/assist').length,1);
});
test('failed clipboard write leaves dialog open with existing command cards intact',async()=>{
 const h=fixture();
 h.api.openForSelection(h.session,'error');
 h.getButtons()[0].onclick();await h.flush();
 const card=h.list.children[0];
 h.clientGlobal.navigator.clipboard.writeText=async()=>{throw Error('Clipboard permission denied');};
 await card.children[2].onclick();
 assert.equal(h.api.minimized,false);
 assert.equal(h.api.visible,true);
 assert.equal(h.list.children[0],card);
 assert.ok(h.status.textContent.includes('Copy failed'));
});
test('after minimize keyboard focus returns to original active terminal; restore focuses dialog',async()=>{
 const h=fixture();
 let termFocus=0,dialogFocus=0;
 h.session.term.focus=()=>{termFocus++};
 h.getButtons()[3].focus=()=>{dialogFocus++};
 h.api.openForSelection(h.session,'error');
 h.getButtons()[0].onclick();await h.flush();
 await h.list.children[0].children[2].onclick();
 assert.equal(termFocus,1);
 h.session.pane.children[0].children[0].onclick();
 assert.equal(dialogFocus,1);
 assert.equal(h.requests.filter(x=>x.url==='/api/ai/assist').length,1);
});
test('when another split pane is active, minimization does not steal keyboard focus',async()=>{
 const h=fixture();
 let termFocus=0;
 h.session.term.focus=()=>{termFocus++};
 h.api.openForSelection(h.session,'error');
 h.getButtons()[0].onclick();await h.flush();
 h.app.active='terminal-2';
 await h.list.children[0].children[2].onclick();
 assert.equal(termFocus,0);
 assert.equal(h.api.dockViewId,'terminal-1');
});


test('reloading a page does not show old replayed OSC 133 command errors',()=>{
 const h=fixture();
 // Only the output replay arrives; no real Enter in this page.
 const old=h.finishCommand('git push',128,'fatal: branch has no upstream',false);
 assert.equal(h.root.body.children.length,2,'No stale Ask AI popup from replay');
 assert.equal(h.api.visible,false);
 assert.equal(h.requests.length,0);
 assert.equal(h.state.commands.at(-1),old);
});
test('replayed OSC preexec + completion is never mistaken for a new user action',()=>{
 const h=fixture();
 const handler=h.listenerWindow.get('taskmenu:shell-integration');
 h.state.activeCommand='git push';h.state.startedAt=Date.now();
 handler({detail:{view:h.session,type:'execution-started',source:'shell-preexec',state:h.state}});
 h.finishCommand('git push',1,'old failure',false);
 assert.equal(h.root.body.children.length,2);
 assert.equal(h.requests.length,0);
});
test('the only offered failure is the last confirmed command in the active terminal',()=>{
 const h=fixture();
 h.finishCommand('gp',128,'fatal: no upstream for current branch');
 let toast=h.root.body.children[2];
 assert.equal(toast.children[1].textContent,'gp');
 // A subsequent successful interactive command makes the first popup stale.
 h.finishCommand('git status',0,'On branch develop');
 assert.equal(toast.removed,true,'old failed command offer must be closed');
 assert.equal(h.root.body.children.filter(x=>x.className==='ai-terminal-prompt'&&!x.removed).length,0);
 h.finishCommand('git pull',1,'could not pull latest branch');
 toast=h.root.body.children.at(-1);
 assert.equal(toast.children[1].textContent,'git pull','Offer must use the newest command');
 toast.children[2].onclick();
 assert.equal(h.api.visible,true);
 assert.equal(h.panel.children[6].value,'could not pull latest branch');
});
test('switching active terminals suppresses pending failed-command offers',()=>{
 const h=fixture();
 const handler=h.listenerWindow.get('taskmenu:shell-integration');
 h.state.startedAt=Date.now();h.state.activeCommand='gp';
 handler({detail:{view:h.session,type:'execution-started',source:'interactive-enter',state:h.state}});
 h.app.active='terminal-2';
 handler=h.listenerWindow.get('taskmenu:view-activated');
 handler({detail:{kind:'terminal',id:'terminal-2'}});
 h.finishCommand('gp',128,'old branch error',false);
 assert.equal(h.root.body.children.length,2);
 assert.equal(h.requests.length,0);
});
test('command completion from another split pane cannot replace the active tab prompt',()=>{
 const h=fixture();
 const activeId=h.session.meta.id;
 const other={...h.session,meta:{...h.session.meta,id:'terminal-2'}};
 h.app.views.set('terminal-2',other);
 h.finishCommand('git push',128,'current pane failure');
 const toast=h.root.body.children[2];
 assert.equal(toast.children[1].textContent,'git push');
 h.app.active='terminal-2';
 h.listenerWindow.get('taskmenu:view-activated')({detail:{kind:'terminal',id:'terminal-2'}});
 assert.equal(toast.removed,true);
 h.api.notifyFailure(h.session,{command:'gp',exitCode:128,finishedAt:Date.now()});
 assert.equal(h.root.body.children.filter(x=>x.className==='ai-terminal-prompt'&&!x.removed).length,0);
 // A new real command on the original active pane remains eligible.
 h.app.active=activeId;
 h.finishCommand('git status',1,'fresh current error');
 assert.equal(h.root.body.children.at(-1).children[1].textContent,'git status');
});
test('unknown or mismatched command output fails closed instead of prompting',()=>{
 const h=fixture();
 h.state.activeCommand='git status';h.state.startedAt=Date.now();
 h.listenerWindow.get('taskmenu:shell-integration')({detail:{
  view:h.session,type:'execution-started',source:'interactive-enter',state:h.state}});
 h.finishCommand('git push',1,'old unrelated push failure',false);
 assert.equal(h.root.body.children.length,2,'mismatched command must never appear');
 h.state.startedAt=Date.now();h.state.activeCommand='gp';
 h.listenerWindow.get('taskmenu:shell-integration')({detail:{
  view:h.session,type:'execution-started',source:'interactive-enter',state:h.state}});
 const record={command:'gp',exitCode:1,output:'fatal',finishedAt:Date.now()};
 h.api.notifyFailure(h.session,record);
 assert.equal(h.root.body.children.length,2,'missing latest parser evidence suppresses toast');
});
