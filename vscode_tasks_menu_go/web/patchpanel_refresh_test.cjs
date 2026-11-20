// Run with: node vscode_tasks_menu_go/web/patchpanel_refresh_test.cjs
// Execute the production DOM reconciliation helper with a tiny DOM model.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, 'featuremods/patchpanel.js'), 'utf8');
const begin = source.indexOf('  function reconcilePatchRows(host,desired){');
const end = source.indexOf('  function patchRow(host,key,signature,node){',begin);
assert.ok(begin >= 0 && end > begin, 'keyed DOM reconciler is present');
const reconcile = vm.runInNewContext(source.slice(begin,end)+'\nreconcilePatchRows;');

class Host {
  constructor() { this.children = []; }
  get firstChild() { return this.children[0] || null; }
  insertBefore(node,reference) {
    const old = this.children.indexOf(node);
    if(old >= 0) this.children.splice(old,1);
    const index = reference == null ? -1 : this.children.indexOf(reference);
    if(index < 0) this.children.push(node);
    else this.children.splice(index,0,node);
    node.parentNode = this;
  }
}
class Node {
  constructor(key,signature) {
    this.dataset = {patchRowKey:key,patchRowVersion:signature};
    this.handler = () => {};
    this.checked = false;
    this.priority = '';
    this.focused = false;
    this.parentNode = null;
  }
  get nextSibling(){
    if(!this.parentNode) return null;
    const siblings = this.parentNode.children;
    return siblings[siblings.indexOf(this)+1] || null;
  }
  remove(){
    if(!this.parentNode) return;
    const siblings=this.parentNode.children;
    const index=siblings.indexOf(this);
    if(index>=0)siblings.splice(index,1);
    this.parentNode=null;
  }
}
function assertOrder(host,nodes) {
  assert.equal(host.children.length,nodes.length,'correct number of DOM nodes');
  nodes.forEach((node,index)=>assert.equal(host.children[index],node,'DOM order and identity'));
}
const host=new Host();
const a=new Node('PATCH:a','v1');
const b=new Node('COLLECT:b','v1');
reconcile(host,[a,b]);
assertOrder(host,[a,b]);
a.checked=true;
a.priority='8';
a.focused=true;
const click=a.handler;
const untouchedA=new Node('PATCH:a','v1');
const untouchedB=new Node('COLLECT:b','v1');
reconcile(host,[untouchedA,untouchedB]);
assertOrder(host,[a,b]);
assert.equal(a.handler,click,'existing button handlers are retained');
assert.equal(a.checked,true,'checked state retained');
assert.equal(a.priority,'8','priority retained');
assert.equal(a.focused,true,'focused control retained');

const c=new Node('COLLECT:c','v1');
reconcile(host,[untouchedA,c,untouchedB]);
assertOrder(host,[a,c,b]);
assert.equal(a.checked,true,'insert does not disrupt prior selection');

const changedB=new Node('COLLECT:b','v2');
reconcile(host,[c,new Node('PATCH:a','v1'),changedB]);
assertOrder(host,[c,a,changedB]);
assert.equal(b.parentNode,null,'changed node removed');
assert.equal(a.checked,true,'reorder preserves unchanged input');

reconcile(host,[new Node('PATCH:a','v1')]);
assertOrder(host,[a]);
assert.equal(c.parentNode,null,'removed COLLECT gone');
assert.equal(changedB.parentNode,null,'removed updated row gone');

reconcile(host,[new Node('PATCH:a','v1'),new Node('PATCH:a','v1')]);
assertOrder(host,[a]);
console.log('PASS: keyed PATCH/COLLECT refresh retains unchanged DOM identity, handlers, selection and focus');

// Exercise the production prompt-ownership gate and event-handler binder, not
// a copy of their logic. A preserved button node must not keep an old session ID.
const ownershipBegin=source.indexOf('  function queuePromptSessionReady(');
const ownershipEnd=source.indexOf('  function refreshActionDisabledState(){',ownershipBegin);
const bindBegin=source.indexOf('  function bindQueuePromptButtonHandlers(sessionId){');
const bindEnd=source.indexOf('  function renderQueuePrompt(sessionId,prompt){',bindBegin);
assert.ok(ownershipBegin>=0&&ownershipEnd>ownershipBegin,'prompt ownership helper is present');
assert.ok(bindBegin>=0&&bindEnd>bindBegin,'prompt button binding helper is present');
const runButton={onclick:null};
const cancelButton={onclick:null};
const calls=[];
const context={
  activeSessionId:'session-a',
  activeQueuePromptSessionId:'session-a',
  activeQueuePrompt:{prompt_id:'prompt-a'},
  queuePromptLoadPending:false,
  queueSilentRefreshPending:false,
  promptButtons:{querySelector(selector){
    if(selector==='[data-patch-prompt-action="select"]')return runButton;
    if(selector==='[data-patch-prompt-action="cancel"]')return cancelButton;
    return null;
  }},
  promptNote:{textContent:''},
  selectedPromptIndexes:()=>[1],
  selectedPromptKinds:()=>['PATCH'],
  queueKindBlocked:()=>false,
  parallelCollectCapability:()=>null,
  activeRunningRunCount:()=>0,
  submitPromptResponse:(sessionId,prompt,action)=>{
    calls.push({sessionId,promptId:prompt.prompt_id,action});
    return Promise.resolve();
  },
  launchParallelCollect:()=>Promise.reject(new Error('not a parallel collect')),
  app:{showError(error){throw error;}}
};
const {queuePromptSessionReady,bindQueuePromptButtonHandlers}=vm.runInNewContext(
  source.slice(ownershipBegin,ownershipEnd)+source.slice(bindBegin,bindEnd)
    +'\n({queuePromptSessionReady,bindQueuePromptButtonHandlers})',context);
bindQueuePromptButtonHandlers('session-a');
const originalRunHandler=runButton.onclick;
runButton.onclick();
assert.equal(calls.length,1);
assert.equal(calls[0].sessionId,'session-a');

// The old button is still present while a new Python queue session is loading.
context.activeSessionId='session-b';
context.activeQueuePrompt={prompt_id:'prompt-b'};
context.queuePromptLoadPending=true;
originalRunHandler();
assert.equal(calls.length,1,'stale click is ignored while replacement is loading');
assert.match(context.promptNote.textContent,/refreshing/i);
context.activeQueuePromptSessionId='session-b';
context.queuePromptLoadPending=false;
context.queueSilentRefreshPending=false;
bindQueuePromptButtonHandlers('session-b');
assert.notEqual(runButton.onclick,originalRunHandler,'preserved button handler was rebound');
runButton.onclick();
cancelButton.onclick();
assert.deepEqual(calls.slice(1),[
  {sessionId:'session-b',promptId:'prompt-b',action:'select'},
  {sessionId:'session-b',promptId:'prompt-b',action:'cancel'}
],'both prompt actions target only the new session');

context.queueSilentRefreshPending=true;
runButton.onclick();
cancelButton.onclick();
assert.equal(calls.length,3,'prompt actions are ignored while refreshing');
context.queueSilentRefreshPending=false;
context.activeQueuePromptSessionId='session-a';
assert.equal(queuePromptSessionReady('session-b',context.activeQueuePrompt),false,'old prompt ownership is rejected');

const emptyPromptBegin=source.indexOf('    if(!runnableItems.length){');
const emptyPromptEnd=source.indexOf("    summary.classList.add('prompt-active');",emptyPromptBegin);
assert.ok(emptyPromptBegin>=0&&emptyPromptEnd>emptyPromptBegin,'empty queue branch exists');
const emptyPromptBlock=source.slice(emptyPromptBegin,emptyPromptEnd);
assert.ok(emptyPromptBlock.includes('promptItems.replaceChildren()'),'empty queue removes hidden selector rows');
assert.ok(emptyPromptBlock.includes('promptButtons.replaceChildren()'),'empty queue removes old actions');
assert.ok(emptyPromptBlock.includes('delete promptButtons.dataset.patchButtonsSignature'),'empty queue invalidates cached buttons');
assert.ok(source.includes('promptButtons.children.length===expectedButtons'),'returning items must recreate missing action buttons');
assert.ok(source.includes('bindQueuePromptButtonHandlers(sessionId);'),'even unchanged buttons must rebind');
assert.ok(source.includes('!queuePromptLoadPending&&!queueSilentRefreshPending'),'queued refresh waits for the active Python prompt');
console.log('PASS: queue button handlers follow the current prompt; empty-to-nonempty actions cannot be skipped');

const controlsBegin=source.indexOf('    const buttonsSignature=patchRowStable([Array.from(actionsAllowed).sort(),');
const controlsEnd=source.indexOf('    if(latestQueueSnapshot)renderQueueSnapshot(latestQueueSnapshot);else renderQueueRows();',controlsBegin);
assert.ok(controlsBegin>=0&&controlsEnd>controlsBegin,'prompt controls construction block exists');
class ActionHost {
  constructor(){this.dataset={};this.children=[];}
  replaceChildren(){this.children=[];}
  append(node){this.children.push(node);}
  querySelector(selector){
    const match=/data-patch-prompt-action="([^"]+)"/.exec(selector);
    return match?this.children.find(node=>node.dataset.patchPromptAction===match[1])||null:null;
  }
}
const actionHost=new ActionHost();
const actionContext={
  actionsAllowed:new Set(['select','cancel']),
  parallelCollect:null,
  activeKinds:new Set(),
  incremental:true,
  promptButtons:actionHost,
  patchRowStable:JSON.stringify,
  document:{createElement(){return {dataset:{},type:'',textContent:''};}},
  sessionId:'session-b',
  bindQueuePromptButtonHandlers:()=>{}
};
const actionCode=source.slice(controlsBegin,controlsEnd);
// Preserve the production block's lexical scope on each simulated
// refresh. Reusing the sandbox must not re-declare top-level const bindings.
const executeActionRender=()=>vm.runInNewContext('{\n'+actionCode+'\n}',actionContext);
actionHost.dataset.patchButtonsSignature=JSON.stringify([['cancel','select'],false,[]]);
executeActionRender();
assert.equal(actionHost.children.length,2,'a stale signature cannot suppress newly needed buttons');
assert.deepEqual(actionHost.children.map(node=>node.textContent),['Run selected','Cancel']);
const oldRunNode=actionHost.children[0];
executeActionRender();
assert.equal(actionHost.children[0],oldRunNode,'unchanged action buttons keep DOM identity');
actionContext.actionsAllowed=new Set(['select']);
executeActionRender();
assert.equal(actionHost.children.length,1,'removing a capability removes only obsolete buttons');
assert.equal(actionHost.children[0].textContent,'Run selected');

// The auto-refresh scheduler must not replace a session after merely receiving
// the queue snapshot. Python must have produced a ready selection prompt first.
const settledBegin=source.indexOf('  function queueSessionSnapshotSettled(){');
const settledEnd=source.indexOf('  function queuePointerInteractionActive(){',settledBegin);
assert.ok(settledBegin>=0&&settledEnd>settledBegin);
const settledContext={
  queuePromptLoadPending:true,queueSilentRefreshPending:false,
  activeSessionId:'session-b',lastQueueSnapshotSessionId:'session-b'
};
const queueSessionSnapshotSettled=vm.runInNewContext(
  source.slice(settledBegin,settledEnd)+'\nqueueSessionSnapshotSettled',settledContext);
assert.equal(queueSessionSnapshotSettled(),false,'snapshot alone cannot restart prompt session');
settledContext.queuePromptLoadPending=false;
settledContext.queueSilentRefreshPending=true;
assert.equal(queueSessionSnapshotSettled(),false,'silent handoff must settle');
settledContext.queueSilentRefreshPending=false;
assert.equal(queueSessionSnapshotSettled(),true,'ready queue resumes timed refresh');
console.log('PASS: prompt action controls recover after an empty queue and refresh awaits Python prompt');


// The production command-console scroll helpers must follow the newest output
// when a RUNNING command's View console button opens, but preserve a manual
// scroll-back across protocol-driven re-renders.
const consoleScrollBegin=source.indexOf('  function commandConsoleAtBottom(node){');
const consoleScrollEnd=source.indexOf('  async function refreshCommandConsole(path,running=false,force=false){',consoleScrollBegin);
assert.ok(consoleScrollBegin>=0&&consoleScrollEnd>consoleScrollBegin,'production console scroll helpers exist');
const consoleScrollState=new Map();
const {commandConsoleAtBottom,recordCommandConsoleScroll,restoreCommandConsoleScroll}=vm.runInNewContext(
  source.slice(consoleScrollBegin,consoleScrollEnd)+
  '\n({commandConsoleAtBottom,recordCommandConsoleScroll,restoreCommandConsoleScroll})',
  {commandConsoleScrollState:consoleScrollState}
);
class FakeCommandConsole {
  constructor(height=5000){this.scrollHeight=height;this.clientHeight=260;this._top=0;this.hidden=false;this.isConnected=true;}
  get scrollTop(){return this._top;}
  set scrollTop(top){this._top=Math.max(0,Math.min(Number(top)||0,this.scrollHeight-this.clientHeight));}
}
const pathA='artifacts/patch_tool/runs/run-1/command_items/1/command_logs/build.log';
const firstConsole=new FakeCommandConsole();
consoleScrollState.set(pathA,{follow:true,top:0});
restoreCommandConsoleScroll(pathA,firstConsole);
assert.equal(firstConsole.scrollTop,4740,'opening a long console immediately shows its last line');
assert.equal(commandConsoleAtBottom(firstConsole),true,'opened console is pinned to latest output');
recordCommandConsoleScroll(pathA,firstConsole);
assert.equal(consoleScrollState.get(pathA).follow,true,'tail-follow state is recorded');

firstConsole.scrollTop=1200; // User scrolls up to inspect older compile output.
recordCommandConsoleScroll(pathA,firstConsole);
assert.equal(consoleScrollState.get(pathA).follow,false,'manual scroll away disables forced tail following');
const rebuiltConsole=new FakeCommandConsole(7000);
restoreCommandConsoleScroll(pathA,rebuiltConsole);
assert.equal(rebuiltConsole.scrollTop,1200,'refresh/rebuild preserves scroll-back location');
rebuiltConsole.scrollHeight=8500; // New output arrives while reading older lines.
restoreCommandConsoleScroll(pathA,rebuiltConsole);
assert.equal(rebuiltConsole.scrollTop,1200,'new output does not steal scroll position from user');

rebuiltConsole.scrollTop=rebuiltConsole.scrollHeight; // User scrolls to tail again.
recordCommandConsoleScroll(pathA,rebuiltConsole);
assert.equal(consoleScrollState.get(pathA).follow,true,'reaching end resumes tail-follow');
const continuedConsole=new FakeCommandConsole(11000);
restoreCommandConsoleScroll(pathA,continuedConsole);
assert.equal(continuedConsole.scrollTop,10740,'live console updates stay on most recent line');
continuedConsole.hidden=true;
continuedConsole.scrollTop=200;
restoreCommandConsoleScroll(pathA,continuedConsole);
assert.equal(continuedConsole.scrollTop,200,'hidden console should not be scrolled');
continuedConsole.hidden=false;continuedConsole.isConnected=false;
recordCommandConsoleScroll(pathA,continuedConsole);
assert.equal(consoleScrollState.get(pathA).follow,true,'detached console does not override active scroll state');

for(const contract of [
  "pre.addEventListener('scroll',()=>recordCommandConsoleScroll(path,pre))",
  'commandConsoleScrollState.set(path,{follow:true,top:0})',
  'restoreCommandConsoleScroll(path,pre)',
  'requestAnimationFrame(()=>{',
  'if(commandConsoleNodes.get(path)===pre&&pre.isConnected&&!pre.hidden)',
  'node.scrollTop=followTail?node.scrollHeight:previousTop',
  'commandConsoleScrollState.clear();'
]){
  assert.ok(source.includes(contract),'live command-console UI must wire scroll behavior: '+contract);
}
console.log('PASS: opening a RUNNING console jumps to its tail; live updates follow without stealing manual scroll-back');
