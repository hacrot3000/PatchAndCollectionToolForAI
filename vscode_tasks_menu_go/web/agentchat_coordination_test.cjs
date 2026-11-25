'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
require('./featuremods/agentchat_coordination.js');

class FakeElement{
 constructor(tag){this.tag=tag;this.children=[];this.handlers={};this.value='';this.attributes={};this.hidden=false;this.textContent='';}
 append(...nodes){this.children.push(...nodes);}
 replaceChildren(...nodes){this.children=[...nodes];}
 setAttribute(name,value){this.attributes[name]=value;}
 addEventListener(name,handler){this.handlers[name]=handler;}
}

function fixture(){
 const document={createElement:tag=>new FakeElement(tag),body:new FakeElement('body')};
 const mount=new FakeElement('toolbar');
 const calls=[];
 const app={async jsonFetch(url,options={}){
  calls.push({url,options});
  if(url==='/api/agent/conversations')return {conversations:[{id:'conv-1',title:'Review'}]};
  if(url==='/api/agent/conversation-groups')return {groups:[{id:'group-1',title:'Shared review',members:['conv-1'],shared_revision:4,shared_instructions:'Use contract v1.'}]};
  if(url.startsWith('/api/agent/coordination?'))return {peers:[{conversation_id:'conv-1'}],messages:[],contracts:[],artifacts:[],dependencies:[],events:[]};
  return {};
 }};
 const component=globalThis.TaskMenuAgentCoordination.create({document,mount,app,agentChat:{selectedSessionID:'conv-1'}});
 return {component,calls,mount};
}

test('coordination UI loads existing groups and uses group membership/instruction APIs',async()=>{
 const {component,calls,mount}=fixture();
 assert.equal(mount.children[0].textContent,'Groups & Coordination');
 await component.refresh();
 assert.ok(calls.some(call=>call.url.includes('/api/agent/coordination?group_id=group-1&conversation_id=conv-1')));
 await component.saveInstructions();
 const update=calls.find(call=>call.url.endsWith('/group-1')&&JSON.parse(call.options.body).action==='update_instructions');
 assert.ok(update);
 assert.equal(JSON.parse(update.options.body).expected_revision,4);
 await component.changeMembership('add_member');
 const add=calls.find(call=>call.url.endsWith('/group-1')&&JSON.parse(call.options.body).action==='add_member');
 assert.ok(add);
 assert.equal(JSON.parse(add.options.body).conversation_id,'conv-1');
});

test('coordination UI posts progress and peer messages through the coordination API',async()=>{
 const {component,calls}=fixture();
 await component.refresh();
 const progressStatus=component.fields.progressStatus;
 const progressSummary=component.fields.progressSummary;
 progressStatus.value='running';progressSummary.value='Implementing parser';
 await component.publishProgress();
 const progress=calls.find(call=>call.url==='/api/agent/coordination'&&JSON.parse(call.options.body).kind==='peer_progress');
 assert.ok(progress);
 assert.equal(JSON.parse(progress.options.body).status,'running');
 const target=component.fields.peerTarget,body=component.fields.peerBody;
 target.value='conv-2';body.value='Please verify the contract.';
 await component.sendPeerMessage();
 const message=calls.find(call=>call.url==='/api/agent/coordination'&&JSON.parse(call.options.body).kind==='peer_message');
 assert.ok(message);
 assert.equal(JSON.parse(message.options.body).message_kind,'ask');
 assert.equal(JSON.parse(message.options.body).to_conversation_id,'conv-2');
 assert.equal(JSON.parse(message.options.body).body,'Please verify the contract.');
 const correlationID=JSON.parse(message.options.body).correlation_id;
 component.fields.peerKind.value='answer';component.fields.peerCorrelation.value=correlationID;
 component.fields.peerBody.value='The API returns Result.';
 await component.sendPeerMessage();
 const answer=calls.find(call=>call.url==='/api/agent/coordination'&&JSON.parse(call.options.body).message_kind==='answer');
 assert.ok(answer);
 assert.equal(JSON.parse(answer.options.body).correlation_id,correlationID);
});

test('coordination UI publishes contracts, evidence-backed artifacts, and dependencies',async()=>{
 const {component,calls}=fixture();
 await component.refresh();
 component.fields.contractKey.value='api.auth';
 component.fields.contractSignature.value='authenticate(token: String): Result';
 component.fields.contractOwner.value='auth-provider';
 component.fields.contractFiles.value='src/auth.go, src/auth_test.go';
 await component.publishContract();
 const contract=calls.find(call=>call.url==='/api/agent/coordination'&&JSON.parse(call.options.body).kind==='contract_publish');
 assert.deepEqual(JSON.parse(contract.options.body).contract.file_paths,['src/auth.go','src/auth_test.go']);
 component.fields.contractRevision.value='1';
 component.fields.contractStatus.value='verified';
 component.fields.integratedRef.value='artifact-1';
 component.fields.testEvidence.value='go test ./internal/auth';
 await component.advanceContract();
 const advance=calls.find(call=>call.url==='/api/agent/coordination'&&JSON.parse(call.options.body).kind==='contract_advance');
 assert.equal(JSON.parse(advance.options.body).test_evidence,'go test ./internal/auth');
 component.fields.artifactKind.value='diff';
 component.fields.artifactRef.value='worktree-diff:conv-1';
 component.fields.artifactSHA.value='a'.repeat(64);
 await component.publishArtifact();
 const artifact=calls.find(call=>call.url==='/api/agent/coordination'&&JSON.parse(call.options.body).kind==='artifact_publish');
 assert.equal(JSON.parse(artifact.options.body).artifact.sha256,'a'.repeat(64));
 component.fields.dependencyConsumer.value='consumer';
 component.fields.dependencyProducer.value='producer';
 component.fields.dependencyContract.value='api.auth';
 await component.registerDependency();
 const dependency=calls.find(call=>call.url==='/api/agent/coordination'&&JSON.parse(call.options.body).kind==='dependency_register');
 assert.equal(JSON.parse(dependency.options.body).dependency.contract_key,'api.auth');
});
