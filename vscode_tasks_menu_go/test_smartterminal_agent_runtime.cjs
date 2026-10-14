// Real reserved-input and fixed-command capture in a simulated browser PTY.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
function runtime({integrated=true,prompt='duongtc@duongpc:~/568E/M3/projects/m3-client$ ',taskID=0,kind='',executing=false}={}){
 const sent=[],output=[],indexed=[],calls=[],helpCalls=[];
 const screen={getLine(){return {translateToString(){return prompt;},isWrapped:false};},baseY:0,cursorY:0,cursorX:prompt.length};
 const view={meta:{id:'session-one',status:'running',task_id:taskID,kind},closed:false,canControl:true,tabReadOnly:false,
  ws:{readyState:1,send(value){sent.push(value)}},term:{write(value){output.push(value)},buffer:{active:screen}}};
 const storage=new Map();
 const context={
  console,TextDecoder,setTimeout,clearTimeout,
  sessionStorage:{getItem:k=>storage.get(k)||null,setItem:(k,v)=>storage.set(k,v)},
  window:{addEventListener(){},confirm(){return false}},
  WebSocket:{OPEN:1},
  TaskMenuApp:{
   taskData:{workspace:'project-one'},browserLeaseLost:false,
   jsonFetch:async(url,options)=>{calls.push({url,options});return {enabled:false,consent:false};}
  },
  TaskMenuHelp:{open:(topic)=>{helpCalls.push(topic);return true;}},
  TaskDeckShellIntegration:{getState:()=>({executing})},
  TaskDeckSmartTerminal:{
   canIntercept:()=>integrated,getInputLine:()=>'',setOSIndex:(v,kind,values)=>indexed.push({kind,values})
  }
 };
 context.globalThis=context;vm.createContext(context);
 for(const name of ['smartterminal_agent_protocol.js','smartterminal_agents.js']){
  vm.runInContext(fs.readFileSync(path.join(__dirname,'web/featuremods',name),'utf8'),context,{filename:name});
 }
 return {agent:context.TaskDeckTerminalAgents,view,sent,output,indexed,calls,helpCalls,context};
}
test('reserved do sends a bounded metadata script, never literal backslash-do',()=>{
 const h=runtime();assert.equal(h.agent.onInput(h.view,'\\do reload command\r'),true);
 assert.equal(h.sent.length,1);assert.doesNotMatch(h.sent[0],/\\do/);
 assert.match(h.sent[0],/compgen -c/);
 const marker=h.sent[0].match(/TDIDX_[A-Za-z0-9_-]+?(?=_BEGIN)/)?.[0];
 assert.ok(marker);
 const answer=h.agent.filterOutput(h.view,'\r\n'+marker+'_BEGIN\r\nls\r\ngit\r\n'+marker+'_END\r\n$ ');
 assert.equal(answer,'$ ');
 assert.equal(h.indexed.length,1);assert.equal(h.indexed[0].kind,'commands');
 assert.equal(Array.from(h.indexed[0].values).join(','),'ls,git');
});
test('history metadata is loaded newest-first',()=>{
 const h=runtime();h.agent.onInput(h.view,'\\do reload history\r');
 const marker=h.sent[0].match(/TDIDX_[A-Za-z0-9_-]+?(?=_BEGIN)/)[0];
 h.agent.filterOutput(h.view,'\n'+marker+'_BEGIN\n 1  ls\n 2  git status\n'+marker+'_END\n');
 assert.equal(h.indexed[0].kind,'history');
 assert.equal(Array.from(h.indexed[0].values).join(','),'git status,ls');
});
test('ordinary escaped shell input and mid-line slash remain regular PTY input',()=>{
 const h=runtime();
 assert.equal(h.agent.onInput(h.view,'\\xyz'),true);
 assert.equal(h.sent.join(''),'\\xyz');
 assert.equal(h.agent.onInput(h.view,'foo'),false);
 assert.equal(h.agent.onInput(h.view,'\\ai run'),false);
 assert.equal(h.sent.join(''),'\\xyz');
});
test('agent multi-line paste is rejected without shell execution',()=>{
 const h=runtime();
 assert.equal(h.agent.onInput(h.view,'\\do reload command\nrm -rf /'),true);
 assert.ok(!h.sent.some(x=>x.includes('rm -rf')));
 assert.ok(h.output.join('').includes('rejected'));
});
test('AI input honors disabled-provider setting without shell execution',async()=>{
 const h=runtime();
 assert.equal(h.agent.onInput(h.view,'\\ai explain ls\r'),true);
 await new Promise(resolve=>setImmediate(resolve));
 assert.ok(h.calls.some(x=>x.url==='/api/ai/settings'));
 assert.ok(!h.calls.some(x=>x.url==='/api/ai/assist'));
 assert.ok(h.sent.every(x=>!x.includes('\\ai')));
 assert.match(h.output.join(''),/Enable AI Assistant/);
});

test('reserved \\help opens TaskDeck Help without forwarding the command to PTY',()=>{
 const h=runtime();
 assert.equal(h.agent.onInput(h.view,'\\help\r'),true);
 assert.deepEqual(h.helpCalls,['']);
 assert.deepEqual(h.sent,['\r'],'only a blank prompt refresh is sent to the shell');
 assert.equal(h.view.smartAgentConsumedInput,true);
 assert.ok(!h.sent.join('').includes('\\help'));
});
test('reserved \\help can open a feature group or search query',()=>{
 const h=runtime();
 for(const char of '\\help database\r')assert.equal(h.agent.onInput(h.view,char),true);
 assert.deepEqual(h.helpCalls,['database']);
 assert.ok(h.sent.every(x=>!x.includes('database')));
});
test('ordinary backslash shell commands still reach PTY and do not open help',()=>{
 const h=runtime();
 assert.equal(h.agent.onInput(h.view,'\\hello'),true);
 assert.equal(h.sent.join(''),'\\hello');
 assert.equal(h.helpCalls.length,0);
 const other=runtime();
 other.agent.onInput(other.view,'echo');
 assert.equal(other.agent.onInput(other.view,'\\help\r'),false);
 assert.equal(other.helpCalls.length,0);
});
test('multi-line pasted \\help is rejected and cannot execute later commands',()=>{
 const h=runtime();
 assert.equal(h.agent.onInput(h.view,'\\help\rreboot'),true);
 assert.equal(h.helpCalls.length,0);
 assert.ok(!h.sent.join('').includes('reboot'));
});

test('missing OSC 133: \\help opens Help at an empty recognizable Bash prompt',()=>{
 const h=runtime({integrated:false});
 assert.equal(h.agent.onInput(h.view,'\\help\r'),true);
 assert.deepEqual(h.helpCalls,['']);
 assert.deepEqual(h.sent,['\r']);
});
test('missing OSC 133: \\help database works with characters typed separately',()=>{
 const h=runtime({integrated:false});
 for(const ch of '\\help database\r')assert.equal(h.agent.onInput(h.view,ch),true);
 assert.deepEqual(h.helpCalls,['database']);
 assert.ok(!h.sent.join('').includes('help'));
});
test('visual prompt fallback never enables AI or \\do without shell integration',()=>{
 const h=runtime({integrated:false});
 assert.equal(h.agent.onInput(h.view,'\\ai ask\r'),true);
 assert.ok(h.sent.join('').includes('\\ai ask\r'));
 assert.deepEqual(h.helpCalls,[]);
 const other=runtime({integrated:false});
 assert.equal(other.agent.onInput(other.view,'\\do reload command\r'),true);
 assert.ok(other.sent.join('').includes('\\do reload command\r'));
});
test('visual prompt fallback does not intercept arbitrary screen output or TUI',()=>{
 for(const opts of [
  {prompt:'Some script completed # ',integrated:false},
  {prompt:'vim: command$ ',integrated:false},
  {prompt:'[user@host path]# ',kind:'task',integrated:false},
  {prompt:'duongtc@duongpc:~$ ',taskID:27,integrated:false},
  {prompt:'duongtc@duongpc:~$ ',executing:true,integrated:false}
 ]){
  const h=runtime(opts);
  assert.equal(h.agent.onInput(h.view,'\\help\r'),false,JSON.stringify(opts));
  assert.equal(h.helpCalls.length,0);
 }
});
test('visual prompt fallback refuses a nonempty shell input and read-only view',()=>{
 const mid=runtime({integrated:false});
 assert.equal(mid.agent.onInput(mid.view,'echo hi '),false);
 assert.equal(mid.agent.onInput(mid.view,'\\help\r'),false);
 assert.deepEqual(mid.helpCalls,[]);
 const locked=runtime({integrated:false});locked.view.canControl=false;
 assert.equal(locked.agent.onInput(locked.view,'\\help\r'),false);
 assert.deepEqual(locked.helpCalls,[]);
});
test('Bash-style root SSH prompt without OSC 133 supports local Help popup',()=>{
 const h=runtime({integrated:false,taskID:-1,kind:'terminal',prompt:'[root@m3-prod-gs-02 tmp]# '});
 assert.equal(h.agent.onInput(h.view,'\\help\r'),true);
 assert.deepEqual(h.helpCalls,['']);
});

test('first backslash is immediately shown without reaching Bash prematurely',()=>{
 const h=runtime();
 assert.equal(h.agent.onInput(h.view,'\\'),true);
 assert.deepEqual(h.output,['\\']);
 assert.deepEqual(h.sent,[]);
});
test('double backslash yields exactly two original bytes to Bash, without persistent preview',()=>{
 const h=runtime();
 h.agent.onInput(h.view,'\\');
 h.agent.onInput(h.view,'\\');
 assert.deepEqual(h.sent,['\\\\']);
 assert.deepEqual(h.output,['\\','\\','\b \b\b \b']);
});
test('unrecognized escaped command is forwarded exactly once after clearing preview',()=>{
 const h=runtime();
 for(const char of '\\ls')if(!h.agent.onInput(h.view,char))h.view.ws.send(char);
 assert.equal(h.sent.join(''),'\\ls');
 assert.equal(h.output.join(''),'\\l\b \b\b \b');
});
test('candidate Backspace and Ctrl-U erase the temporary prompt, never shell state',()=>{
 const h=runtime();
 h.agent.onInput(h.view,'\\');
 h.agent.onInput(h.view,'\x7f');
 assert.deepEqual(h.output,['\\','\b \b']);
 assert.deepEqual(h.sent,[]);
 // A new prefix works after the candidate is completely cleared.
 h.agent.onInput(h.view,'\\h');
 h.agent.onInput(h.view,'\x15');
 assert.deepEqual(h.sent,[]);
 assert.deepEqual(h.output.slice(-3),['\\','h','\b \b\b \b']); // preview plus erase
});
test('help directive typed char by char is echoed once and not executed in Bash',()=>{
 const h=runtime();
 for(const char of '\\help\r')h.agent.onInput(h.view,char);
 assert.deepEqual(h.helpCalls,['']);
 assert.deepEqual(h.sent,['\r']);
 assert.equal(h.output.join(''),'\\help\r\n');
});
test('longer agent directive retains its already visible prefix on mode transition',()=>{
 const h=runtime();
 for(const char of '\\ai \r')h.agent.onInput(h.view,char);
 assert.match(h.output.join(''),/^\\ai \r\n/);
 assert.doesNotMatch(h.output.join(''),/\\ai \\ai /);
 assert.ok(h.sent.every(x=>!x.includes('\\ai')));
});


test('manual alias reload obtains exact Bash alias definitions without executing any alias',()=>{
 const h=runtime();
 assert.equal(h.agent.onInput(h.view,'\\do reload aliases\r'),true);
 assert.equal(h.sent.length,1);
 assert.match(h.sent[0],/alias -p/);
 assert.doesNotMatch(h.sent[0],/\\do/);
 const marker=h.sent[0].match(/TDIDX_[A-Za-z0-9_-]+?(?=_BEGIN)/)?.[0];
 assert.ok(marker);
 const output='\n'+marker+'_BEGIN\n'+
  "alias gb='git branch'\n"+
  "alias gc='gc-safe'\n"+
  "alias ga='git add'\n"+
  "alias gis='git status'\n"+
  "alias gí='git status'\n"+
  marker+'_END\n';
 h.agent.filterOutput(h.view,output);
 const found=h.indexed.find(row=>row.kind==='aliases');
 assert.ok(found,'TaskDeck must update the aliases for this terminal only');
 assert.equal(Array.from(found.values).length,5);
 assert.ok(Array.from(found.values).includes("alias gc='gc-safe'"));
});
test('alias scan is read-only and does not run user aliases',()=>{
 const h=runtime();
 assert.equal(h.agent.runReload(h.view,'aliases',true),true);
 assert.equal(h.sent.length,1);
 assert.match(h.sent[0],/alias -p/);
 assert.doesNotMatch(h.sent[0],/\bgc\b|\bgb\b|\bga\b/);
});

test('automatic command and alias inventories redraw one Bash prompt without visible chatter',()=>{
 for(const kind of ['commands','aliases']){
  const h=runtime();
  assert.equal(h.agent.runReload(h.view,kind,true),true,kind);
  assert.equal(h.output.length,0,'automatic reload must not write messages onto the idle prompt');
  const marker=h.sent[0].match(/TDIDX_[A-Za-z0-9_-]+?(?=_BEGIN)/)?.[0];
  assert.ok(marker);
  const prompt='\x1b]133;A\x07duongtc@duongpc:~/568E/M3/projects/m3-client$ ';
  const received='hidden shell command\r\n'+marker+'_BEGIN\r\n'+
   (kind==='aliases'?"alias gb='git branch'\r\n":"git\r\n")+
   marker+'_END\r\n'+prompt;
  const visible=h.agent.filterOutput(h.view,received);
  assert.equal(visible,'\r\x1b[2K'+prompt,
   'automatic index must clear the old line before Bash paints its new prompt');
  assert.equal(h.output.length,0,'auto-completion inventory is silent');
  assert.equal(h.indexed.length,1);
  assert.equal(h.indexed[0].kind,kind==='aliases'?'aliases':'commands');
 }
});
test('manual backslash-do inventory preserves visible feedback and never erases the current terminal row',()=>{
 const h=runtime();
 assert.equal(h.agent.onInput(h.view,'\\do reload aliases\r'),true);
 const marker=h.sent[0].match(/TDIDX_[A-Za-z0-9_-]+?(?=_BEGIN)/)?.[0];
 assert.ok(marker);
 const visible=h.agent.filterOutput(h.view,
  'hidden shell command\r\n'+marker+'_BEGIN\r\nalias gb=git\r\n'+marker+'_END\r\n$ ');
 assert.equal(visible,'$ ','manual reload must not modify the shell cursor');
 assert.ok(h.output.some(text=>text.includes('Reloading aliases')));
 assert.ok(h.output.some(text=>text.includes('Loaded 1 shell aliases')));
});
