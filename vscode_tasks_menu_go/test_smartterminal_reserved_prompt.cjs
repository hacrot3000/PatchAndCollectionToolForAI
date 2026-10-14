// Regression for Bash prompts missing OSC 133; terminal directives must never
// execute as Bash help/do, even if the line reached Readline before Enter.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
function environment(prompt='duongtc@duongpc:~/568E/M3/projects/m3-client$ ',lineText=prompt){
 const sent=[],written=[],helpCalls=[],calls=[],listeners=new Map(),storage=new Map();
 const line={isWrapped:false,translateToString:()=>lineText};
 const buffer={baseY:0,cursorY:0,cursorX:lineText.length,getLine:()=>line};
 const term={options:{fontSize:13},buffer:{active:buffer},write(v){written.push(v)},
  attachCustomKeyEventHandler(){},element:{querySelector:()=>null}};
 const view={meta:{id:'test-session',status:'running'},closed:false,canControl:true,tabReadOnly:false,term,
  ws:{readyState:1,send(v){sent.push(v)}}};
 const app={active:'test-session',browserLeaseLost:false,views:new Map([[view.meta.id,view]]),
  taskData:{workspace:'test'},jsonFetch:async(url)=>{calls.push(url);return {enabled:false,consent:false}}};
 function el(){return {style:{},className:'',classList:{add(){},remove(){},contains(){return false}},
  append(){},replaceChildren(){},setAttribute(){},addEventListener(){}}}
 const context={console,Date,Set,Map,WeakSet,TextDecoder,WebSocket:{OPEN:1},setTimeout,clearTimeout,
  TaskMenuApp:app,
  TaskDeckShellIntegration:{getState:()=>null,getCommands:()=>[]}, // OSC 133 ABSENT
  TaskMenuHelp:{open:value=>{helpCalls.push(value);return true;}},
  sessionStorage:{getItem:k=>storage.get(k),setItem:(k,v)=>storage.set(k,v)},
  document:{createElement:el,head:{append(){}},body:{append(){}}},
  window:{innerWidth:1200,innerHeight:900,confirm:()=>false,addEventListener:(n,cb)=>listeners.set(n,cb)},
  requestAnimationFrame(fn){fn()}
 };
 context.globalThis=context;vm.createContext(context);
 for(const name of ['smartterminal_engine.js','smartterminal.js','smartterminal_agent_protocol.js','smartterminal_agents.js']){
  vm.runInContext(fs.readFileSync(path.join(__dirname,'web/featuremods',name),'utf8'),context,{filename:name});
 }
 return {context,view,sent,written,helpCalls,calls,prompt,buffer,
  agent:context.TaskDeckTerminalAgents,terminal:context.TaskDeckSmartTerminal,
  type(value){for(const char of value)this.terminal.onInput(view,char);}
 };
}
test('OSC-less user@host Bash prompt reserves \\help and opens TaskDeck Help',()=>{
 const h=environment();
 assert.equal(h.terminal.canIntercept(h.view),true);
 assert.equal(h.terminal.readReservedLine(h.view),'');
 h.type('\\help\r');
 assert.deepEqual(h.helpCalls,['']);
 assert.deepEqual(h.sent,['\r']);
 assert.ok(!h.sent.some(s=>s.includes('\\help')));
});
test('OSC-less Bash \\do reload command sends fixed inventory, not literal directive',()=>{
 const h=environment();
 h.type('\\do reload command\r');
 assert.equal(h.sent.length,1);
 assert.match(h.sent[0],/compgen -c/);
 assert.doesNotMatch(h.sent[0],/\\do/);
});
test('OSC-less Bash \\do reload history sends Bash history inventory',()=>{
 const h=environment();
 h.type('\\do reload history\r');
 assert.equal(h.sent.length,1);
 assert.match(h.sent[0],/history 2>\/dev\/null/);
});
test('OSC-less remote root-style prompt also reserves \\help topic',()=>{
 const h=environment('[root@m3-prod-gs-02 ~]# ');
 h.type('\\help database\r');
 assert.deepEqual(h.helpCalls,['database']);
 assert.deepEqual(h.sent,['\r']);
});
test('OSC-less \\ai is intercepted and checks provider settings without shell execution',async()=>{
 const h=environment();
 h.type('\\ai explain ls\r');
 await new Promise(resolve=>setImmediate(resolve));
 assert.ok(h.calls.includes('/api/ai/settings'));
 assert.ok(!h.sent.some(s=>s.includes('\\ai')));
});
test('Already echoed TaskDeck directive is erased from Bash before it can run',()=>{
 const prompt='duongtc@duongpc:~/568E/M3/projects/m3-client$ ';
 const h=environment(prompt,prompt+'\\do reload command');
 assert.equal(h.agent.wouldInterceptEnter(h.view,'\r'),true);
 assert.equal(h.terminal.onInput(h.view,'\r'),true);
 assert.equal(h.sent[0],'\x15');
 assert.match(h.sent[1],/compgen -c/);
 assert.equal(h.sent.length,2);
 assert.ok(!h.sent.some(s=>s.includes('\\do')));
});
test('Already echoed \\help clears Bash line and calls TaskDeck help',()=>{
 const prompt='duongtc@duongpc:~/project$ ';
 const h=environment(prompt,prompt+'\\help');
 assert.equal(h.terminal.onInput(h.view,'\r'),true);
 assert.deepEqual(h.sent,['\x15','\r']);
 assert.deepEqual(h.helpCalls,['']);
});
test('Normal escapes and unrecognized prompts are never taken over',()=>{
 const h=environment();
 h.type('\\hello');
 assert.equal(h.sent.join(''),'\\hello');
 assert.equal(h.helpCalls.length,0);
 const other=environment('Password: ');
 assert.equal(other.terminal.canIntercept(other.view),false);
 assert.equal(other.terminal.onInput(other.view,'\\'),false);
 assert.equal(other.terminal.readReservedLine(other.view),null);
});
test('No takeover inside command output, wrapped lines, mid-line or executing shell',()=>{
 const h=environment('Password: \\help');
 assert.equal(h.terminal.canIntercept(h.view),false);
 const wrapped=environment();
 wrapped.view.term.buffer.active.getLine=()=>({isWrapped:true,translateToString:()=>wrapped.prompt});
 assert.equal(wrapped.terminal.canIntercept(wrapped.view),false);
 const edit=environment('x@y:~$ ','x@y:~$ \\help trailing');
 edit.buffer.cursorX='x@y:~$ \\help'.length;
 assert.equal(edit.terminal.readReservedLine(edit.view),null);
});
