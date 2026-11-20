'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const script=raw.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(script,'missing trusted native GUI code');
class Element {
 constructor(id) {
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(name,handler){this.listeners[name]=handler}
 append(...items){this.children.push(...items)}
 replaceChildren(){this.children=[]}
 setAttribute(k,v){this.attributes[k]=v}
 getAttribute(k){return this.attributes[k]||null}
 click(){assert.ok(this.listeners.click,'missing listener '+this.id);this.listeners.click()}
}
const elements=new Map();
const get=id=>{
 if(!elements.has(id))elements.set(id,new Element(id));
 return elements.get(id);
};
const token='0123456789abcdef'.repeat(4);
let remaining=16,commitCalls=0,confirmation=true;
const seen=[];
const fetch=async (endpoint,options)=>{
 seen.push(endpoint);
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 if(endpoint==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(endpoint==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:false,
  task_run:false,task_long:false,git_status:true,git_pull:false,
  git_commit:true,remote_control_available:false,remaining:--remaining,
 })};
 if(endpoint==='/api/git-commit'){
  assert.equal(options.headers['Content-Type'],'application/json');
  assert.equal(Object.keys(JSON.parse(options.body)).join(','),'message');
  assert.equal(JSON.parse(options.body).message,'Sửa lỗi remote GUI');
  commitCalls++;
  return {ok:true,json:async()=>({
   ok:true,before:'a'.repeat(40),after:'b'.repeat(40),
   remote_control_available:false,remaining:--remaining,
  })};
 }
 if(endpoint==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
 throw Error('unexpected endpoint '+endpoint);
};
const sandbox={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{addEventListener(){},confirm(){return confirmation}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,setInterval(){},setTimeout(){},
 Date,Number,String,Promise,encodeURIComponent,
};
vm.createContext(sandbox);
new vm.Script(script[1],{filename:'native-git-commit.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<60;i++)await Promise.resolve()};
(async()=>{
 await flush();
 assert.equal(get('git-commit').disabled,true,'Git commit enabled before feature RBAC read');
 get('features').click();await flush();
 assert.equal(get('git-commit').disabled,false,'host-enabled GitCommit not shown');
 assert.equal(get('git-commit-message').disabled,false);
 get('git-commit-message').value='invalid\nline';
 get('git-commit').click();await flush();
 assert.equal(commitCalls,0,'multiline message reached Agent');
 get('git-commit-message').value='x'.repeat(121);
 get('git-commit').click();await flush();
 assert.equal(commitCalls,0,'oversize message reached Agent');
 confirmation=false;
 get('git-commit-message').value='Sửa lỗi remote GUI';
 get('git-commit').click();await flush();
 assert.equal(commitCalls,0,'cancelled confirmation still committed');
 confirmation=true;
 get('git-commit').click();await flush();
 assert.equal(commitCalls,1,'authenticated staged commit not sent exactly once');
 assert.equal(get('git-commit-message').value,'','commit input retained after success');
 assert.match(get('output').textContent,/Đã commit nội dung đã stage/);
 assert.equal(get('git-summary').hidden,true,'stale Git status persisted after write');
 get('disconnect').click();await flush();
 assert.equal(get('git-commit-message').value,'','commit message retained after logout');
 assert.equal(get('git-commit').disabled,true,'logout left Git mutation enabled');
 console.log('PASS: authenticated GitCommit GUI, message validation, confirmation and logout scrub');
})().catch(err=>{console.error(err);process.exitCode=1});
