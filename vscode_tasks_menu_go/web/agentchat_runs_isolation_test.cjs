'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
require('./featuremods/agentproviderselector.js');
require('./featuremods/agentchat_runs_ui.js');

class FakeElement{
 constructor(tag){this.tag=tag;this.children=[];this.listeners={};this.value='';this.disabled=false;this.attributes={};this.hidden=false;this.textContent='';this.dataset={};}
 append(...nodes){this.children.push(...nodes);}
 replaceChildren(...nodes){this.children=[...nodes];}
 addEventListener(name,handler){(this.listeners[name]??=[]).push(handler);}
 setAttribute(name,value){this.attributes[name]=value;}
}

function deferred(){
 let resolve,reject;
 const promise=new Promise((res,rej)=>{resolve=res;reject=rej;});
 return {promise,resolve,reject};
}

function buildUI({jsonFetch}){
 const document={createElement:tag=>new FakeElement(tag),head:new FakeElement('head'),body:new FakeElement('body')};
 globalThis.document=document;
 const selectorMount=new FakeElement('div'),runsMount=new FakeElement('div');
 const ui=globalThis.TaskMenuAgentRunsUI.create({document,selectorMount,runsMount,jsonFetch});
 return {ui,document,selectorMount,runsMount};
}

test('a poll started for one conversation never renders another conversation data',async()=>{
 const runsResponse=deferred();
 const sessionResponse=deferred();
 const jsonFetch=async(url)=>{
  if(url==='/api/agent/runs')return await runsResponse.promise;
  if(url.startsWith('/api/agent/sessions/'))return await sessionResponse.promise;
  throw new Error('unexpected request '+url);
 };
 const {ui,runsMount}=buildUI({jsonFetch});
 const sessionA={id:'conv-a',provider_id:'composer',model_id:'composer-2.5'};
 const sessionB={id:'conv-b',provider_id:'gpt',model_id:'gpt-6.1-sol'};
  // The poll for conversation A stays in flight while the user switches away.
  const pendingA=ui.selectConversation(sessionA);
  await ui.selectConversation(sessionB);
  // Now let the stale conversation A responses arrive.
  runsResponse.resolve({runs:[{spec:{run_id:'run-a',conversation_id:'conv-a',provider_id:'composer',model_id:'composer-2.5'},status:'running',history:[],events:[]}]});
  sessionResponse.resolve({session:{tool_calls:[]}});
  await pendingA;
  await new Promise(resolve=>setTimeout(resolve,20));
  const rendered=runsMount.children.filter(node=>node.className==='agentchat-run-card');
  assert.equal(rendered.length,0,'a stale conversation poll painted run cards');
});
