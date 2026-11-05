'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname,'..','featuremods','patchpanel.js'),'utf8');
function extract(start,end) {
  const a = source.indexOf(start);
  const b = source.indexOf(end,a+start.length);
  assert.ok(a>=0&&b>a,'missing Patch panel function block: '+start);
  return source.slice(a,b);
}
const reconcileSource = extract('  function reconcilePatchRows(host,entries){','  function renderInvalidQueueHandoffs(snapshot){');
const actionSource = extract('  async function awaitCurrentQueuePrompt(){','  async function waitForActionResult(');

class FakeNode {
  constructor(name='') {
    this.name=name;
    this.dataset={};
    this.parentNode=null;
    this.childNodes=[];
    this.focused=false;
  }
  get children(){return this.childNodes;}
  get firstChild(){return this.childNodes[0]||null;}
  get nextSibling(){
    if(!this.parentNode)return null;
    const peers=this.parentNode.childNodes;
    return peers[peers.indexOf(this)+1]||null;
  }
  insertBefore(node,ref){
    if(node.parentNode){
      const previous=node.parentNode.childNodes;
      const i=previous.indexOf(node);
      if(i!==-1)previous.splice(i,1);
    }
    const i=ref?this.childNodes.indexOf(ref):-1;
    if(ref&&i<0)throw Error('reference child missing');
    this.childNodes.splice(i<0?this.childNodes.length:i,0,node);
    node.parentNode=this;
    return node;
  }
  remove(){
    if(!this.parentNode)return;
    const siblings=this.parentNode.childNodes;
    const at=siblings.indexOf(this);
    if(at>=0)siblings.splice(at,1);
    this.parentNode=null;
  }
}

test('unchanged Queue/PATCH/COLLECT nodes and checked state survive polling',()=>{
  const ctx=vm.createContext({});
  vm.runInContext(reconcileSource+'\nthis.reconcile=reconcilePatchRows;',ctx);
  const host=new FakeNode('queue');
  let builds=0,updates=0;
  const row=(key,rev)=>({
    key,revision:rev,
    create(){builds++;return new FakeNode(key);},
    update(node){updates++;node.renderVersion=rev;}
  });
  ctx.reconcile(host,[row('PATCH:alpha','v1'),row('COLLECT:beta','v1')]);
  const [alpha,beta]=host.children;
  alpha.checked=true;
  alpha.focused=true;
  let clickCount=0;
  beta.onclick=()=>clickCount++;
  assert.equal(builds,2);
  assert.equal(updates,2);
  ctx.reconcile(host,[row('PATCH:alpha','v1'),row('COLLECT:beta','v1')]);
  assert.deepEqual(host.children,[alpha,beta]);
  assert.equal(builds,2,'no row was rebuilt');
  assert.equal(updates,2,'no unchanged row was updated');
  assert.equal(alpha.checked,true,'checkbox selection was not lost');
  assert.equal(alpha.focused,true,'focused control was retained');
  beta.onclick();
  assert.equal(clickCount,1,'original Delete handler still works after poll');

  ctx.reconcile(host,[row('COLLECT:gamma','v1'),row('PATCH:alpha','v2'),row('COLLECT:beta','v1')]);
  assert.equal(builds,3,'only the new row was created');
  assert.equal(updates,4,'one new row and one changed row were updated');
  assert.equal(host.children[1],alpha,'changed row was updated in place');
  assert.equal(host.children[2],beta,'unchanged button was not recreated');
  assert.equal(alpha.checked,true,'updated row still retains selection');
  assert.equal(beta.onclick!==undefined,true);
  ctx.reconcile(host,[row('COLLECT:gamma','v1'),row('PATCH:alpha','v2')]);
  assert.equal(builds,3,'deletion did not remount other rows');
  assert.equal(beta.parentNode,null,'only the removed item was detached');
  assert.equal(host.children[1],alpha,'remaining PATCH row identity survives removal');
});

function newActionContext() {
  const calls=[];
  let confirms=0;
  const ctx=vm.createContext({
    queueSilentRefreshPending:true,
    lifecycleSuspended:false,
    activePatchMode:'queue',
    activeSessionId:'old-session',
    activeQueuePromptSessionId:'old-session',
    activeQueuePrompt:{
      prompt_id:'old-prompt',
      queue_actions:['delete'],
      items:[{name:'example.zip',kind:'COLLECT',group:'new',index:3}]
    },
    actionBusy:false,queueMutationBusy:false,parallelCollectBusy:false,
    window:{confirm(){confirms++;return true;}},
    queueActionAllowed(action){return ctx.activeQueuePrompt.queue_actions.includes(action);},
    promptItemForQueueItem(item){
      return ctx.activeQueuePrompt.items.find(row=>row.name===item.name&&row.kind===item.kind&&row.group===item.group)||null;
    },
    app:{jsonFetch:async(url,opts)=>{
      calls.push({url,body:JSON.parse(opts.body)});
      return {mutation_id:'delete-1'};
    }},
    waitForQueueMutation:async(sessionId,mutationID,promptID)=>({sessionId,mutationID,promptID}),
    refreshActionDisabledState(){},
    renderQueueRows(){},
    setTimeout,
    console
  });
  vm.runInContext(actionSource+'\nthis.submit=submitQueueDelete;',ctx);
  return {ctx,calls,confirmCount:()=>confirms};
}

test('one Delete click during auto-refresh uses new session, prompt ID and item index',async()=>{
  const {ctx,calls,confirmCount}=newActionContext();
  const item={name:'example.zip',kind:'COLLECT',group:'new'};
  const resultPromise=ctx.submit(item,{...item,index:3});
  assert.equal(ctx.queueMutationBusy,true,'click reserves mutation before waiting');
  setTimeout(()=>{
    ctx.activeSessionId='new-session';
    ctx.activeQueuePromptSessionId='new-session';
    ctx.activeQueuePrompt={
      prompt_id:'new-prompt',
      queue_actions:['delete'],
      items:[{...item,index:9}]
    };
    ctx.queueSilentRefreshPending=false;
  },15);
  const result=await resultPromise;
  assert.equal(confirmCount(),1,'confirmation happens exactly once');
  assert.equal(calls.length,1,'exactly one POST is sent');
  assert.equal(calls[0].url,'/api/sessions/new-session/queue-delete');
  assert.equal(calls[0].body.prompt_id,'new-prompt');
  assert.equal(calls[0].body.index,9);
  assert.equal(result.sessionId,'new-session');
  assert.equal(ctx.queueMutationBusy,false);
});

test('a stale prompt or removed queue item cannot be deleted after refresh',async()=>{
  const {ctx,calls}=newActionContext();
  const item={name:'example.zip',kind:'COLLECT',group:'new'};
  const pending=ctx.submit(item,{...item,index:3});
  setTimeout(()=>{
    ctx.activeSessionId='new-session';
    ctx.activeQueuePromptSessionId='new-session';
    ctx.activeQueuePrompt={prompt_id:'new-prompt',queue_actions:['delete'],items:[]};
    ctx.queueSilentRefreshPending=false;
  },15);
  await assert.rejects(pending,/removed or changed/);
  assert.equal(calls.length,0,'no stale mutation was sent');
});
