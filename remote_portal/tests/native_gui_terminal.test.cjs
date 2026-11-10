'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const tick=String.fromCharCode(96);
const parts=raw.split('const trustedGUIJavaScript = '+tick);
assert.equal(parts.length,2);
const js=parts[1].split(tick)[0];
class Element {
 constructor(id){this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.listeners={};this.children=[];this.attributes={};
  this.classList={toggle(){}};}
 addEventListener(event,handler){this.listeners[event]=handler}
 click(){assert.ok(this.listeners.click,'missing listener '+this.id);this.listeners.click()}
 append(...items){this.children.push(...items)}
 replaceChildren(){this.children=[]}
 setAttribute(name,value){this.attributes[name]=value}
 getAttribute(name){return this.attributes[name]||null}
}
const elements=new Map();
const get=id=>{
 if(!elements.has(id))elements.set(id,new Element(id));
 return elements.get(id);
};
const token='0123456789abcdef'.repeat(4);
let budget=16,confirmed=false,terminalCalls=[],pollState='running';
const fetch=async (path,options)=>{
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining:budget,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:false,
  task_run:false,task_long:false,git_status:false,git_pull:true,
  git_commit:true,git_push:true,terminal:true,
  remote_control_available:false,remaining:--budget,
 })};
 if(path.startsWith('/api/terminal-')){
  terminalCalls.push([path,options.body||'']);
  const payload=options.body?JSON.parse(options.body):null;
  if(path==='/api/terminal-open'){
   assert.equal(payload,null);
  }else if(path==='/api/terminal-write'){
   assert.equal(options.headers['Content-Type'],'application/json');
   assert.equal(Object.keys(payload).join(','),'input');
  }else if(path==='/api/terminal-resize'){
   assert.equal(options.headers['Content-Type'],'application/json');
   assert.deepEqual(payload,{cols:120,rows:32});
  }else assert.equal(payload,null);
  return {ok:true,json:async()=>({
   ok:true,state:path==='/api/terminal-close'?'closed':
     path==='/api/terminal-poll'?pollState:'running',
   output:path==='/api/terminal-poll'&&pollState!=='closed'?
     (pollState==='exited'?'shell-finished\n':'remote-ready\n'):'',
   more:false,truncated:false,remote_control_available:false,
   remaining:--budget,
  })};
 }
 if(path==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected endpoint '+path);
};
const sandbox={
 document:{getElementById:get,createElement:type=>new Element(type)},
 window:{addEventListener(){},confirm(){return confirmed}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,Date,Promise,Number,String,encodeURIComponent,setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
sandbox.__terminalQuotaRequestCount=()=>terminalCalls.length;
const anchor='  function render(){';
assert.ok(js.includes(anchor),'native GUI render injection anchor missing');
const instrumented=js.replace(anchor,"\n  globalThis.__testTerminalQuota=async (rounds,ptyState,route)=>{\n    const savedBudget=budget,savedState=terminalState;\n    budget=rounds;terminalState=ptyState;\n    render();\n    const ui={\n      openDisabled:terminalOpen.disabled,pollDisabled:terminalPoll.disabled,\n      sendDisabled:terminalSend.disabled,resizeDisabled:terminalResize.disabled,\n      closeDisabled:terminalClose.disabled,\n      featureDisabled:features.disabled,\n      allowed:terminalMaintenanceAllowed(route)\n    };\n    const before=globalThis.__terminalQuotaRequestCount();\n    if(!ui.allowed)await invoke(route);\n    ui.sent=globalThis.__terminalQuotaRequestCount()>before;\n    budget=savedBudget;terminalState=savedState;render();\n    return ui;\n  };\n"+'\n'+anchor);
new vm.Script(instrumented,{filename:'native-terminal.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<80;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('terminal-open').disabled,true,'PTY enabled before feature verification');
 get('features').click();await flush();
 assert.equal(get('terminal-open').disabled,false,'host-approved PTY not available');
 const insufficientOpen=await sandbox.__testTerminalQuota(1,'closed','/api/terminal-open');
 assert.equal(insufficientOpen.openDisabled,true,'PTY opened with no Close quota');
 assert.equal(insufficientOpen.allowed,false);
 assert.equal(insufficientOpen.sent,false,'synthetic Open bypassed one-round quota');
 const sufficientOpen=await sandbox.__testTerminalQuota(2,'closed','/api/terminal-open');
 assert.equal(sufficientOpen.openDisabled,false);
 assert.equal(sufficientOpen.allowed,true);
 // All non-Close requests must preserve the final authenticated Close
 // round, including synthetic clicks which bypass the disabled button.
 const lastPoll=await sandbox.__testTerminalQuota(1,'running','/api/terminal-poll');
 assert.equal(lastPoll.pollDisabled,true);
 assert.equal(lastPoll.sendDisabled,true);
 assert.equal(lastPoll.resizeDisabled,true);
 assert.equal(lastPoll.featureDisabled,true);
 assert.equal(lastPoll.closeDisabled,false);
 assert.equal(lastPoll.allowed,false);
 assert.equal(lastPoll.sent,false,'synthetic Poll used reserved Close quota');
 const exitedLast=await sandbox.__testTerminalQuota(1,'exited','/api/terminal-poll');
 assert.equal(exitedLast.pollDisabled,true);
 assert.equal(exitedLast.closeDisabled,false);
 assert.equal(exitedLast.sent,false);
  for(const id of ['git-pull','git-commit','git-push']){
    assert.equal(get(id).disabled,false,id+' unavailable before PTY opened');
  }
 get('nav-terminal').click();
 assert.equal(get('panel-terminal').hidden,false);
 get('terminal-open').click();await flush();
 assert.equal(terminalCalls.length,0,'shell was opened despite rejected confirmation');
 confirmed=true;
 get('terminal-open').click();await flush();
 assert.equal(terminalCalls.length,1,'PTY not opened exactly once');
 assert.equal(get('terminal-open').disabled,true);
 assert.equal(get('terminal-send').disabled,false);
 assert.match(get('session-activity').textContent,/Terminal PTY đang chạy/);
  for(const id of ['git-pull','git-commit','git-push']){
    assert.equal(get(id).disabled,true,id+' must be disabled while PTY owns session');
  }
  get('git-commit-message').value='Safe commit';
  const beforeSynthetic=terminalCalls.length;
  for(const id of ['git-pull','git-commit','git-push'])get(id).click();
  await flush();
  assert.equal(terminalCalls.length,beforeSynthetic,
    'synthetic click requested Git mutation during live terminal');
 get('terminal-input').value='bad'+String.fromCharCode(27)+'erase';
 get('terminal-send').click();await flush();
 assert.equal(terminalCalls.length,1,'terminal escape injection admitted');
 get('terminal-input').value='echo safe';
 get('terminal-send').click();await flush();
 assert.equal(terminalCalls.length,2);
 assert.equal(JSON.parse(terminalCalls[1][1]).input,'echo safe\n');
 assert.equal(get('terminal-input').value,'','PTY input persisted in textbox');
 get('terminal-cols').value='120';get('terminal-rows').value='32';
 get('terminal-resize').click();await flush();
 assert.equal(terminalCalls.length,3);
 get('terminal-poll').click();await flush();
 assert.equal(get('terminal-output').textContent,'remote-ready\n');
 get('terminal-interrupt').click();await flush();
 assert.equal(JSON.parse(terminalCalls[4][1]).input,String.fromCharCode(3));
 get('terminal-eof').click();await flush();
 assert.equal(JSON.parse(terminalCalls[5][1]).input,String.fromCharCode(4));
 pollState='exited';
 get('terminal-poll').click();await flush();
 assert.match(get('terminal-output').textContent,/shell-finished/);
 assert.equal(get('terminal-send').disabled,true,
   'exited PTY still accepted shell input');
 assert.equal(get('terminal-close').disabled,false,
   'exited PTY must remain closeable');
 get('terminal-close').click();await flush();
 assert.equal(get('session-activity').textContent,'Không có tác vụ đang chạy');
 assert.equal(get('terminal-send').disabled,true);
 assert.equal(get('terminal-poll').disabled,true);
  for(const id of ['git-pull','git-commit','git-push']){
    assert.equal(get(id).disabled,false,id+' remained disabled after PTY close');
  }
 // A malicious or stale localhost bridge must not persuade the client
 // that terminal.poll itself closed the host lease. Even if the local
 // HTTP request is 200, the typed lifecycle violation fails closed.
 pollState='closed';
 get('terminal-open').click();await flush();
 assert.equal(get('terminal-open').disabled,true);
 get('terminal-poll').click();await flush();
 assert.match(get('output').textContent,/Kết quả Terminal E2EE không hợp lệ/,
  'terminal.poll incorrectly accepted an impossible closed state');
 assert.equal(get('terminal-output').textContent,'',
  'invalid PTY state must scrub authenticated terminal output');
 get('disconnect').click();await flush();
 assert.equal(get('terminal-output').textContent,'','logout retained PTY output');
 assert.equal(get('terminal-input').value,'','logout retained typed shell input');
 console.log('PASS: native PTY opt-in, confirmation, input, read, resize, control and logout');
})().catch(err=>{console.error(err);process.exitCode=1});
