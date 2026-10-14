// Real reserved-input and fixed-command capture in a simulated browser PTY.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
function runtime(){
 const sent=[],output=[],indexed=[],calls=[];
 const view={meta:{id:'session-one',status:'running'},closed:false,canControl:true,tabReadOnly:false,
  ws:{readyState:1,send(value){sent.push(value)}},term:{write(value){output.push(value)}}};
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
  TaskDeckSmartTerminal:{
   canIntercept:()=>true,getInputLine:()=>'',setOSIndex:(v,kind,values)=>indexed.push({kind,values})
  }
 };
 context.globalThis=context;vm.createContext(context);
 for(const name of ['smartterminal_agent_protocol.js','smartterminal_agents.js']){
  vm.runInContext(fs.readFileSync(path.join(__dirname,'web/featuremods',name),'utf8'),context,{filename:name});
 }
 return {agent:context.TaskDeckTerminalAgents,view,sent,output,indexed,calls,context};
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
