'use strict';
const fs=require('node:fs');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const source=fs.readFileSync(
 'vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const match=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(match,'native GUI script is missing');

class Element {
 constructor(id){
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(name,fn){this.listeners[name]=fn}
 setAttribute(key,value){this.attributes[key]=value}
 getAttribute(key){return this.attributes[key]||null}
 append(...nodes){this.children.push(...nodes)}
 replaceChildren(...nodes){this.children=nodes}
 click(){
  assert.ok(this.listeners.click,'missing click handler: '+this.id);
  this.listeners.click();
 }
}

async function scenario(startStatus,pollStates,expectStartRejected=false){
 const elements=new Map();
 const get=id=>{
  if(!elements.has(id))elements.set(id,new Element(id));
  return elements.get(id);
 };
 const token='0123456789abcdef'.repeat(4);
 let budget=16;
 const calls=[];
 const reply=value=>({ok:true,json:async()=>value});
 const progress=(status,more=false)=>({
  ok:true,task_id:123,status,
  exit_code:status==='completed'?0:-1,
  output:'',truncated:false,more,
  remote_control_available:false,remaining:--budget,
 });
 const fetch=async(endpoint,opts)=>{
  calls.push(endpoint);
  assert.equal(opts.headers['X-TaskDeck-Local-Capability'],token);
  if(endpoint==='/api/session')return reply({
   ok:true,closed:false,active:false,remaining:budget,
   expires_in_seconds:300,device_id:'a'.repeat(64),username:'alice'
  });
  if(endpoint==='/api/features')return reply({
   ok:true,session_ping:true,project_info:false,tasks_preview:true,
   task_run:true,task_long:true,git_status:false,git_fetch:true,
   terminal:true,
   remote_control_available:false,remaining:--budget
  });
  if(endpoint==='/api/tasks-preview')return reply({
   ok:true,total:1,labels:['Long host task'],ids:[123],remaining:--budget
  });
  if(endpoint==='/api/tasks-inspect/123')return reply({
   ok:true,task_id:123,eligible:true,input_id:'',choices:[],
   input_type:'',input_id_2:'',choices_2:[],
   remote_control_available:false,remaining:--budget
  });
  if(endpoint==='/api/tasks-start/123'){
   const initial=typeof startStatus==='string'?startStatus:startStatus.status;
   return reply(Object.assign(progress(initial,false),
     typeof startStatus==='object'?startStatus:{}));
  }
  if(endpoint==='/api/tasks-poll/123'){
   assert.ok(pollStates.length,'unexpected extra poll');
   const next=pollStates.shift();
   const status=typeof next==='string'?next:next.status;
   return reply(Object.assign(progress(status,pollStates.length>0),
     typeof next==='object'?next:{}));
  }
  if(endpoint==='/api/git-fetch'){
   return reply({ok:true,head:'a'.repeat(40),before:'b'.repeat(40),
    after:'b'.repeat(40),updated:false,remote_control_available:false,
    remaining:--budget});
  }
  if(endpoint==='/api/tasks-clear/123'){
   return reply(progress('cleared',false));
  }
  throw Error('unexpected native route '+endpoint);
 };
 const sandbox={
  document:{getElementById:get,createElement:tag=>new Element(tag)},
  window:{confirm(){return true},addEventListener(){}},
  history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
  fetch,setInterval(){},setTimeout(){},Date,Number,String,Promise
 };
 vm.createContext(sandbox);
 const instrumented=match[1].replace('  function render(){',
  '  globalThis.__testSetQuota=(value)=>{budget=value;render();};\n'+
  '  function render(){');
 new vm.Script(instrumented,{filename:'native-gui-lifecycle.js'}).runInContext(sandbox);
 const flush=async()=>{for(let n=0;n<50;n++)await Promise.resolve()};
 await flush();
 get('features').click();await flush();
 get('tasks-preview').click();await flush();
 let row=get('task-preview-labels').children[0];
 assert.ok(row,'missing task preview row');
 row.children[1].click();await flush();
 row=get('task-preview-labels').children[0];
 row.children[3].click();await flush();
 if(expectStartRejected){
  assert.equal(get('long-task-controls').hidden,true,
   'invalid start ACK did not lock and scrub GUI');
 }else{
  assert.equal(get('long-task-controls').hidden,false,'valid start rejected');
 }
 return {get,flush,calls,setQuota(value){sandbox.__testSetQuota(value);}};
}

(async()=>{
 // A host may complete a short-lived managed job before the start
 // acknowledgement is observed. The GUI must still own that job.
 const immediate=await scenario('completed',[]);
 assert.equal(immediate.get('long-task-clear').disabled,false,
  'immediate successful completion was not adopted');
 assert.equal(immediate.get('task-run-history-list').children.length,1);

 // A final job still has buffered output/history but no longer occupies
 // the workspace execution lease. Git fetch can proceed before Clear.
 // This must NEVER be true for the same job while running.
 const retained=await scenario('started',[{
  status:'completed',output:'saved-output',more:true
 }]);
 assert.equal(retained.get('git-fetch').disabled,true,
  'Git incorrectly admitted while task process was active');
 const beforeStartGit=retained.calls.length;
 retained.get('git-fetch').click();await retained.flush();
 assert.equal(retained.calls.length,beforeStartGit,
  'forged click allowed Git during running task');
 retained.get('long-task-poll').click();await retained.flush();
 assert.equal(retained.get('git-fetch').disabled,false,
  'completed task unnecessarily held Git until manual Clear');
 assert.equal(retained.get('terminal-open').disabled,true,
  'finished but retained job incorrectly allowed a new PTY');
 const beforeNewPTY=retained.calls.length;
 retained.get('terminal-open').click();await retained.flush();
 assert.equal(retained.calls.length,beforeNewPTY,
  'forged Terminal click bypassed retained job exclusion');
 assert.equal(retained.get('long-task-controls').hidden,false,
  'Git readiness cleared the retained task slot');
 assert.match(retained.get('long-task-output').textContent,/saved-output/);
 retained.get('git-fetch').click();await retained.flush();
 assert.equal(retained.calls.filter(x=>x==='/api/git-fetch').length,1,
  'Git fetch was not admitted after task completion');
 assert.match(retained.get('long-task-output').textContent,/saved-output/,
  'Git fetch erased the previously authenticated task transcript');
 assert.equal(retained.get('task-run-history-list').children.length,1,
  'Git fetch erased the finished task history');

 // When all output has been received, a final job only needs one E2EE
 // action for Clear. With two rounds remaining the operator can perform
 // one Git mutation first, then still clear the host job explicitly.
 const scarce=await scenario('started',['completed']);
 scarce.get('long-task-poll').click();await scarce.flush();
 scarce.setQuota(2);
 assert.equal(scarce.get('git-fetch').disabled,false,
  'finished/drained job needlessly reserved three Cancel/Poll/Clear rounds');
 const scarceBefore=scarce.calls.length;
 scarce.get('git-fetch').click();await scarce.flush();
 assert.equal(scarce.calls.length,scarceBefore+1,
  'Git fetch was not allowed despite preserving the last Clear round');
 assert.equal(scarce.get('long-task-clear').disabled,false,
  'Git fetch consumed the final E2EE round needed for job cleanup');
 scarce.get('long-task-clear').click();await scarce.flush();
 assert.equal(scarce.get('long-task-controls').hidden,true,
  'finished job could not be cleared with the final preserved E2EE round');

 // In contrast, a finished job with unread output must preserve two
 // actions for Poll plus Clear even when Git could mutate the workspace.
 const pendingTail=await scenario('started',[{status:'completed',more:true}]);
 pendingTail.get('long-task-poll').click();await pendingTail.flush();
 pendingTail.setQuota(2);
 assert.equal(pendingTail.get('git-fetch').disabled,true,
  'pending task output permitted Git to consume the Poll/Clear quota');
 const pendingBefore=pendingTail.calls.length;
 pendingTail.get('git-fetch').click();await pendingTail.flush();
 assert.equal(pendingTail.calls.length,pendingBefore,
  'synthetic Git click spent reserved output and cleanup rounds');
 assert.equal(pendingTail.get('long-task-poll').disabled,false,
  'finished job was unable to drain remaining output');

 // A host truncation warning is cumulative even if a later Poll
 // correctly reports its own clean, final output chunk.
 const truncated=await scenario('started',[
  {status:'running',output:'part-one',truncated:true,more:true},
  {status:'completed',output:'part-two',truncated:false,more:false},
 ]);
 truncated.get('long-task-poll').click();await truncated.flush();
 assert.match(truncated.get('long-task-output').textContent,/output vượt giới hạn/,
  'initial host truncation was not shown');
 truncated.get('long-task-poll').click();await truncated.flush();
 assert.match(truncated.get('long-task-output').textContent,/output vượt giới hạn/,
  'later Poll erased a previously verified truncation warning');
 assert.match(truncated.get('long-task-output').textContent,/part-onepart-two/,
  'bounded transcript lost authenticated output after final Poll');
 assert.match(truncated.get('task-run-history-list').children[0].children[0].children[0].textContent,
  /output đã cắt/,'the retained result forgot the host truncation');
 truncated.get('long-task-clear').click();await truncated.flush();
 assert.equal(truncated.get('long-task-controls').hidden,true,
  'truncated final result could not release its native job slot');

 // Three simultaneous child processes have a distinct E2EE-attested
 // parallel mode. Show that mode in the transcript and history, but
 // fail closed on an unsigned status or a 49-byte output in a 4-step Poll.
 const three=await scenario('started',[{
  status:'running',parallel:true,step:3,step_total:4,
  attempt:1,attempt_total:1,step_status:'running',
  step_history:'RRR-',step_details:'-|-|-|-',
  output:'P'.repeat(48),more:true,
 }]);
 three.get('long-task-poll').click();await three.flush();
 assert.equal(three.get('long-task-controls').hidden,false,
  'authenticated parallel 4-step Poll incorrectly revoked the session');
 assert.match(three.get('long-task-output').textContent,/3 nhánh song song/,
  'the user cannot distinguish the parallel fork from sequential work');
 assert.match(three.get('long-task-output').textContent,/Bước 3\/4/,
  'third worker not recognized by native GUI');
 assert.match(three.get('long-task-output').textContent,/P{48}/,
  'valid authenticated 48-byte output was lost');
 assert.match(three.get('task-run-history-list').children[0].children[1].textContent,
  /3 nhánh song song/,
  'retained history omitted the verified parallel workflow mode');
 for(const malformed of [
  {status:'running',parallel:false,step:3,step_total:4,
   attempt:1,attempt_total:1,step_status:'running',
   step_history:'RRR-',step_details:'-|-|-|-'},
  {status:'running',parallel:true,step:3,step_total:4,
   attempt:1,attempt_total:1,step_status:'running',
   step_history:'RRR-',step_details:'-|-|-|-',output:'X'.repeat(49)},
 ]){
  const invalid=await scenario('started',[malformed]);
  invalid.get('long-task-poll').click();await invalid.flush();
  assert.equal(invalid.get('long-task-controls').hidden,true,
   'forged parallel Poll accepted without authenticated mode/byte budget');
 }

 // A four-leaf fork has a five-step signed root and an even tighter
 // 16-byte output chunk. The native GUI must reject fake mode markers,
 // an unapproved retry, or output using the sequential 32-byte cap.
 const four=await scenario('started',[{
  status:'running',parallel:true,step:4,step_total:5,
  attempt:1,attempt_total:1,step_status:'running',
  step_history:'RRRR-',step_details:'-|-|-|-|-',
  output:'W'.repeat(16),more:true,
 }]);
 four.get('long-task-poll').click();await four.flush();
 assert.equal(four.get('long-task-controls').hidden,false,
  'authenticated four-leaf E2EE progress was denied');
 assert.match(four.get('long-task-output').textContent,/4 nhánh song song/,
  'native GUI did not identify the fourth simultaneous worker');
 assert.match(four.get('long-task-output').textContent,/W{16}/,
  'native GUI clipped allowed 16-byte fork output');
 for(const invalidProgress of [
  {status:'running',parallel:false,step:4,step_total:5,
   attempt:1,attempt_total:1,step_status:'running',
   step_history:'RRRR-',step_details:'-|-|-|-|-'},
  {status:'running',parallel:true,step:4,step_total:5,
   attempt:1,attempt_total:1,step_status:'running',
   step_history:'RRRR-',step_details:'-|-|-|-|-',output:'Y'.repeat(17)},
  {status:'running',parallel:true,step:4,step_total:5,
   attempt:1,attempt_total:2,step_status:'running',
   step_history:'RRRR-',step_details:'-|-|-|-|-'},
 ]){
  const malformed=await scenario('started',[invalidProgress]);
  malformed.get('long-task-poll').click();await malformed.flush();
  assert.equal(malformed.get('long-task-controls').hidden,true,
   'native GUI accepted forged four-leaf E2EE progress');
 }

 // Before the fourth child starts, the host can sign a partially
 // running four-leaf fork (RRR--). This became a valid intermediate
 // state after the parallel expansion.
 const partial=await scenario('started',[{
  status:'running',step:3,step_total:5,parallel:true,
  attempt:1,attempt_total:1,step_status:'running',
  step_history:'RRR--',step_details:'-|-|-|-|-',
 }]);
 partial.get('long-task-poll').click();await partial.flush();
 assert.equal(partial.get('long-task-controls').hidden,false,
  'native GUI rejected a valid partial four-leaf progress state');

 // Five-step sequential workflows carry five signed outcomes, but
 // cannot exceed their E2EE byte budget.
 const five=await scenario('started',[{
  status:'completed',step:5,step_total:5,
  attempt:2,attempt_total:2,step_status:'completed',
  step_history:'CCCCC',step_details:'0:10|0:20|0:30|0:40|0:50',
  output:'F'.repeat(32),more:false,
 }]);
 five.get('long-task-poll').click();await five.flush();
 assert.equal(five.get('long-task-controls').hidden,false,
  'signed five-step sequential outcome was denied by native GUI');
 assert.match(five.get('long-task-output').textContent,/Bước 5\/5/,
  'native GUI lost the fifth verified step');
 assert.match(five.get('long-task-output').textContent,/F{32}/,
  'native five-step progress clipped the valid 32-byte chunk');
 for(const bad of [
  {status:'completed',step:5,step_total:5,
   attempt:2,attempt_total:2,step_status:'completed',
   step_history:'CCCCC',step_details:'0:10|0:20|0:30|0:40|0:50',
   output:'X'.repeat(33)},
  {status:'running',step:3,step_total:5,parallel:true,
   attempt:1,attempt_total:1,step_status:'running',
   step_history:'RRRRR',step_details:'-|-|-|-|-'},
 ]) {
  const malformed=await scenario('started',[bad]);
  malformed.get('long-task-poll').click();await malformed.flush();
  assert.equal(malformed.get('long-task-controls').hidden,true,
   'native GUI accepted forged fifth-step output/fork');
 }

 // Each response might be valid in isolation, but no active workflow
 // can change between parallel/sequential or alter step total after its
 // first authenticated progress Poll.
 const valid5={
  status:'running',parallel:true,step:4,step_total:5,
  attempt:1,attempt_total:1,step_status:'running',
  step_history:'RRRR-',step_details:'-|-|-|-|-',
 };
 for(const inconsistent of [
  {status:'running',parallel:false,step:4,step_total:5,
   attempt:1,attempt_total:1,step_status:'running',
   step_history:'CCCR-',step_details:'0:1|0:2|0:3|-|-'},
  {status:'running',parallel:true,step:3,step_total:4,
   attempt:1,attempt_total:1,step_status:'running',
   step_history:'RRR-',step_details:'-|-|-|-'},
  {status:'running',more:false,output:''},
 ]){
  const changed=await scenario('started',[valid5,inconsistent]);
  changed.get('long-task-poll').click();await changed.flush();
  assert.equal(changed.get('long-task-controls').hidden,false,
   'first authenticated workflow progress was rejected');
  changed.get('long-task-poll').click();await changed.flush();
  assert.equal(changed.get('long-task-controls').hidden,true,
   'a stale or forged Poll changed the same job workflow structure');
  assert.equal(changed.get('long-task-output').textContent,'',
   'invalid workflow transition retained private output in GUI');
 }
 // Start/Cancel/Clear acknowledge only a state transition. The native
 // browser must also reject impossible nonempty output and truncation
 // on an ACK, even when a malicious localhost bridge returns HTTP 200.
 for(const forged of [
  {status:'started',output:'injected-host-output'},
  {status:'started',truncated:true},
 ]){
  const bad=await scenario(forged,[],true);
  assert.match(bad.get('output').textContent,/Trạng thái\/exit code/,
   'forged ACK was not rejected by typed browser validation');
  assert.equal(bad.get('long-task-output').textContent,'',
   'forged ACK leaked task output into the UI');
 }
 // A second "started" cannot overwrite the existing job/output/history.
 const restarted=await scenario('started',['started']);
 restarted.get('long-task-poll').click();await restarted.flush();
 assert.match(restarted.get('output').textContent,/vòng đời|Trạng thái\/exit code/);
 assert.equal(restarted.get('long-task-output').textContent,'');
 assert.equal(restarted.get('long-task-controls').hidden,true,
  'invalid status must close and scrub native GUI');

 // Only an explicit clear action may mark a host job cleared.
 const earlyClear=await scenario('started',['cleared']);
 earlyClear.get('long-task-poll').click();await earlyClear.flush();
 assert.match(earlyClear.get('output').textContent,/vòng đời|Trạng thái\/exit code/);

 // A terminal success cannot later turn into an active job.
 const regression=await scenario('started',['completed','running']);
 regression.get('long-task-poll').click();await regression.flush();
 assert.equal(regression.get('long-task-controls').hidden,false);
 regression.get('long-task-poll').click();await regression.flush();
 assert.match(regression.get('output').textContent,/vòng đời/);
 assert.equal(regression.get('long-task-controls').hidden,true);

 // Native Go E2EE validates exact UTF-8 byte limits and status/exit-code
 // relationships. The local browser must not accept an impossible
 // forged bridge reply simply because JS string.length is shorter.
 const impossible=[
  {status:'running',exit_code:0},
  {status:'running',output:'🙂'.repeat(40)}, // 80 UTF-16 units, 160 UTF-8 bytes
  {status:'running',output:'\\x1b[31m'}, // checked below as ESC escape
  {status:'cancel_requested',more:true},
  {status:'running',step:1,step_total:2,attempt:1,attempt_total:1,
   step_status:'running',output:'é'.repeat(40)}, // 80 bytes > 64 for workflow
 ];
 impossible[2].output='\x1b[31m';
 for(const response of impossible){
  const attempt=await scenario('started',[response]);
  attempt.get('long-task-poll').click();await attempt.flush();
  assert.equal(attempt.get('long-task-controls').hidden,true,
   'invalid E2EE task response did not fail closed: '+response.status);
  assert.equal(attempt.get('long-task-output').textContent,'',
   'invalid response left private output visible');
 }
 console.log('PASS: native long task lifecycle, strict UTF-8 quota and status codec');
})().catch(error=>{console.error(error);process.exitCode=1});
