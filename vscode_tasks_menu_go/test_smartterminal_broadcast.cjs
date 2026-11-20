// Regression: synthetic smart-terminal completions must use the same ordered
// Broadcast input queue as ordinary keypresses, without leaking agent input.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');

const bcast=fs.readFileSync(path.join(__dirname,'web/featuremods/broadcast.js'),'utf8');
const core=fs.readFileSync(path.join(__dirname,'internal/server/ui.go'),'utf8');
const begin=bcast.indexOf('function queueBroadcast(view,data,confirmed=false){');
const end=bcast.indexOf('\nif(!app.sharedMode){',begin);
assert.ok(begin>=0&&end>begin,'Broadcast routing and terminal input hook must exist');
const functions=bcast.slice(begin,end);
function harness({mode='all',groupID='',confirm=true}={}){
 const inputQueues=new Map(),inputHooks=new Map(),requests=[],warnings=[],handlers=[];
 const state={mode,assignments:{one:groupID},groups:[]};
 const app={
  sharedMode:false,views:new Map(),
  jsonFetch:async(url,options)=>{
   assert.equal(url,'/api/broadcast');
   requests.push(JSON.parse(options.body));
   return {delivered:2};
  }
 };
 const source={closed:false,meta:{id:'one',status:'running'},
  smartTerminalInputIntercepted:false,
  term:{onData(fn){handlers.push(fn);return {dispose(){}}}},
  tab:{classList:{remove(){},add(){}},style:{removeProperty(){},setProperty(){}},dataset:{}}
 };
 const create=new Function('app','state','inputQueues','inputHooks','confirmBroadcastInput',
  'broadcastTargetCount','applyTabStyle','warnInput','window',
  functions+'\nreturn {queueBroadcast,forwardCompletion,installInputHook};');
 const actions=create(app,state,inputQueues,inputHooks,
  ()=>confirm,()=>2,()=>{},error=>warnings.push(error),{confirm:()=>confirm});
 actions.installInputHook(source);
 return {actions,source,requests,warnings,handlers,
  async drain(){
   await inputQueues.get('one');
  },
  typed(data,intercepted=false){
   source.smartTerminalInputIntercepted=intercepted;
   for(const handle of handlers)handle(data);
  }
 };
}

test('Tab completion suffix is broadcast once, before the next Enter',async()=>{
 const h=harness();
 h.typed('l');h.typed('s');
 assert.equal(h.actions.forwardCompletion(h.source,' -la',{replacesLine:false}),true);
 h.typed('\r');
 await h.drain();
 assert.deepEqual(h.requests.map(x=>x.data),['l','s',' -la','\r']);
 assert.ok(h.requests.every(x=>x.source_id==='one'));
});
test('reserved local AI/DO input cannot broadcast to other terminals',async()=>{
 const h=harness();
 h.typed('\\',true);
 h.typed('ai',true);
 h.typed(' help\r',true);
 h.typed('\\do reload history\r',true);
 h.typed('ls\r',false);
 await h.drain();
 assert.deepEqual(h.requests.map(x=>x.data),['ls\r']);
 assert.match(core,/view\.smartTerminalInputIntercepted=false/);
 assert.match(core,/view\.smartTerminalInputIntercepted=true/);
});
test('rejected full-line replacement does not enqueue a remote request',async()=>{
 const h=harness({confirm:false});
 assert.equal(h.actions.forwardCompletion(h.source,'\x15git status',{replacesLine:true}),false);
 assert.equal(h.requests.length,0);
});
test('accepted replacement applies exactly one Ctrl-U replacement, not two',async()=>{
 const h=harness();
 assert.equal(h.actions.forwardCompletion(h.source,'\x15git status',{replacesLine:true}),true);
 await h.drain();
 assert.deepEqual(h.requests.map(x=>x.data),['\x15git status']);
});
test('Broadcast None does not create a remote event for autocomplete',async()=>{
 const h=harness({mode:'none'});
 assert.equal(h.actions.forwardCompletion(h.source,' -la'),true);
 h.typed('l');
 assert.deepEqual(h.requests,[]);
});
test('Broadcast Group without source assignment stays local',async()=>{
 const h=harness({mode:'group'});
 assert.equal(h.actions.forwardCompletion(h.source,' -la'),true);
 h.typed('s');
 assert.deepEqual(h.requests,[]);
});
test('stopped source cannot enqueue completion',async()=>{
 const h=harness();h.source.meta.status='exited';
 assert.equal(h.actions.forwardCompletion(h.source,' -la'),false);
 assert.deepEqual(h.requests,[]);
});
