'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const found=source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(found,'trusted GUI JS missing');
const anchor='  function boundedUTF8HistoryTail(value,maxBytes){';
assert.ok(found[1].includes(anchor));
const script=found[1].replace(anchor,`  globalThis.__logTest=(value,reset=false)=>renderLongTaskTranscript(value,reset);
`+anchor);
class Element{
 constructor(id){this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.listeners={};this.children=[];this.attributes={};
  this.classList={toggle(){}};this.scrollTop=0;this.clientHeight=100;this.baseHeight=100;}
 addEventListener(type,handler){this.listeners[type]=handler}
 click(){assert.ok(this.listeners.click,'missing click '+this.id);this.listeners.click()}
 setAttribute(key,value){this.attributes[key]=value}
 getAttribute(key){return this.attributes[key]||null}
 append(...nodes){this.children.push(...nodes)}
 replaceChildren(){this.children=[]}
 get scrollHeight(){return Math.max(this.baseHeight,this.textContent.length)}
}
const elements=new Map();
const get=id=>{if(!elements.has(id))elements.set(id,new Element(id));return elements.get(id)};
const token='0123456789abcdef'.repeat(4),sent=[];
const sandbox={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{addEventListener(){},confirm(){return true}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch:async(endpoint)=>{
  sent.push(endpoint);
  assert.equal(endpoint,'/api/session');
  return {ok:true,json:async()=>({ok:true,closed:false,active:false,
   remaining:16,expires_in_seconds:300,device_id:'a'.repeat(64),username:'alice'})};
 },setInterval(){},setTimeout(){},Date,Number,String,Promise
};
vm.createContext(sandbox);
new vm.Script(script,{filename:'trusted-task-scroll.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<50;i++)await Promise.resolve()};
(async()=>{
 await flush();
 const node=get('long-task-output'),old='x'.repeat(300);
 node.scrollTop=170;
 sandbox.__logTest(old,true);
 assert.equal(node.scrollTop,0,'new job inherited scroll');
 node.scrollTop=200;
 sandbox.__logTest(old+'🙂'.repeat(50));
 assert.equal(node.scrollTop,node.scrollHeight,'bottom did not follow appended output');
 node.scrollTop=34;
 sandbox.__logTest(old+'🙂'.repeat(150));
 assert.equal(node.scrollTop,34,'reading older log was interrupted');
 node.scrollTop=node.scrollHeight-node.clientHeight-12;
 sandbox.__logTest(old+'🙂'.repeat(220));
 assert.equal(node.scrollTop,node.scrollHeight,'near-bottom log was not followed');
 node.textContent='short';node.scrollTop=0;
 sandbox.__logTest('still short');
 assert.equal(node.scrollTop,node.scrollHeight);
 node.textContent=old;node.scrollTop=117;
 sandbox.__logTest('cleared',true);
 assert.equal(node.scrollTop,0,'clear did not reset view');
 assert.deepEqual(sent,['/api/session'],'scroll triggered an E2EE request');
 console.log('PASS: bounded long-task output follows bottom, preserves reading position');
})().catch(err=>{console.error(err);process.exitCode=1});
