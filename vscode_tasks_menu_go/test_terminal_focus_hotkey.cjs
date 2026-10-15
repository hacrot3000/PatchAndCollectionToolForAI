// Runtime regression for Ctrl+Backquote focus-idle-or-open shortcut.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function environment({active='external:editor',views=[],shells={},canCreate=true}={}){
 const handlers=new Map(),calls=[],errors=[];
 let createCount=0,pendingCreate=null;
 const app={
  views:new Map(views.map(view=>[view.meta.id,view])),
  active,
  startTerminal(){
   createCount++;
   calls.push(['create']);
   if(pendingCreate)return pendingCreate;
   return canCreate?Promise.resolve({created:true}):Promise.reject(Error('not permitted'));
  },
  activateView(id,options){
   calls.push(['activate',id,options]);
   this.active=id;
   return true;
  },
  showError(error){errors.push(String(error));}
 };
 const context={
  TaskMenuApp:app,
  TaskDeckTerminalClone:{localTerminal(view){
   const meta=view?.meta||{};
   return meta.kind==='terminal'&&Number(meta.task_id)===0&&
    String(meta.target_type||'')!=='ssh'&&!meta.target_profile_id;
  }},
  TaskDeckShellIntegration:{getState:id=>shells[id]||null},
  WebSocket:{OPEN:1},
  window:{addEventListener:(name,fn,capture)=>handlers.set(name,fn)}
 };
 context.globalThis=context;
 vm.createContext(context);
 vm.runInContext(fs.readFileSync(path.join(__dirname,'web/featuremods/terminalhotkey.js'),'utf8'),
  context,{filename:'terminalhotkey.js'});
 function press({shift=false,ctrl=true,meta=false,alt=false,code='Backquote',repeat=false,
                 defaultPrevented=false,composing=false,altGraph=false}={}){
  const event={ctrlKey:ctrl,metaKey:meta,shiftKey:shift,altKey:alt,code,repeat,defaultPrevented,
   isComposing:composing,getModifierState:name=>name==='AltGraph'&&altGraph,
   preventDefault(){this.prevented=true;},stopPropagation(){this.stopped=true;}};
  handlers.get('keydown')(event);
  return event;
 }
 return {app,context,handlers,calls,errors,press,get creates(){return createCount;},
  setPendingCreate(value){pendingCreate=value;}};
}
function terminal(id,{kind='terminal',status='running',taskID=0,control=true,readOnly=false,
                       connected=true,remote=false,closed=false}={}){
 return {meta:{id,kind,status,task_id:taskID,target_type:remote?'ssh':'local'},
  term:{focus(){}},ws:{readyState:connected?1:3},canControl:control,tabReadOnly:readOnly,closed};
}
function idle(){return {seenPrompt:true,executing:false};}
function busy(){return {seenPrompt:true,executing:true};}
const tick=()=>new Promise(resolve=>setImmediate(resolve));

test('Ctrl+Backquote opens a local terminal when there are no existing tabs',async()=>{
 const h=environment();const event=h.press();
 assert.equal(event.prevented,true);assert.equal(event.stopped,true);
 await tick();
 assert.equal(h.creates,1);assert.equal(h.calls.filter(x=>x[0]==='activate').length,0);
});
test('Ctrl+Backquote focuses the current idle terminal instead of creating a second one',async()=>{
 const view=terminal('shell1'),h=environment({views:[view],active:'shell1',shells:{shell1:idle()}});
 h.press();await tick();
 assert.equal(h.creates,0);
 assert.deepEqual(h.calls.map(x=>x[0]),['activate']);
 assert.equal(h.calls[0][1],'shell1');
 assert.equal(h.calls[0][2].focus,true);
 assert.equal(h.calls[0][2].force,true);
});
test('when editor is active, switch to idle terminal (not a task or SSH)',async()=>{
 const views=[terminal('task',{kind:'task',taskID:99}),
  terminal('remote',{remote:true}),terminal('local')];
 const h=environment({views,active:'external:editor',
  shells:{task:idle(),remote:idle(),local:idle()}});
 h.press();await tick();
 assert.equal(h.creates,0);assert.equal(h.app.active,'local');
});
test('prefer last active idle terminal among multiple candidates',async()=>{
 const a=terminal('a'),b=terminal('b');
 const h=environment({views:[a,b],shells:{a:idle(),b:idle()}});
 h.handlers.get('taskmenu:view-activated')({detail:{kind:'terminal',id:'b'}});
 h.press();await tick();
 assert.equal(h.app.active,'b');assert.equal(h.creates,0);
});
test('busy, stopped, read-only, disconnected and unknown shell state are not reused',async()=>{
 const views=[terminal('busy'),terminal('stopped',{status:'exited'}),
  terminal('readonly',{readOnly:true}),terminal('offline',{connected:false}),
  terminal('unknown'),terminal('closed',{closed:true})];
 const shells={busy:busy(),stopped:idle(),readonly:idle(),offline:idle(),closed:idle()};
 const h=environment({views,shells});
 h.press();await tick();
 assert.equal(h.creates,1);
 assert.equal(h.calls.some(x=>x[0]==='activate'),false);
});
test('all busy shells cause new terminal; Ctrl+Shift+Backquote always opens another',async()=>{
 const h=environment({views:[terminal('busy')],shells:{busy:busy()}});
 h.press();await tick();assert.equal(h.creates,1);
 const k=h.press({shift:true});await tick();
 assert.equal(k.prevented,true);assert.equal(h.creates,2);
 h.context.TaskDeckShellIntegration.getState=id=>idle();
 h.press({shift:true});await tick();assert.equal(h.creates,3);
});
test('repeated keydown, AltGraph and unrelated modifiers do not open tabs',async()=>{
 const h=environment();
 const ignored=[{repeat:true},{alt:true},{shift:true,alt:true},{altGraph:true},
  {defaultPrevented:true},{code:'KeyG'},{ctrl:false},{ctrl:false,meta:false}];
 for(const opts of ignored)h.press(opts);
 await tick();assert.equal(h.creates,0);
 h.press({meta:true,ctrl:false});await tick();assert.equal(h.creates,1);
});
test('two Ctrl+Backquote presses during a pending new-shell request create only once',async()=>{
 const h=environment();
 let complete;const pending=new Promise(resolve=>{complete=resolve;});
 h.setPendingCreate(pending);
 h.press();h.press();await tick();
 assert.equal(h.creates,1);
 complete({created:true});await tick();
 h.press();await tick();
 assert.equal(h.creates,2,'a later shortcut may open after the first request completes');
});
test('creation errors are handled and future shortcuts can retry',async()=>{
 const h=environment({canCreate:false});
 h.press();await tick();assert.equal(h.errors.length,1);
 h.press();await tick();assert.equal(h.creates,2);
});
