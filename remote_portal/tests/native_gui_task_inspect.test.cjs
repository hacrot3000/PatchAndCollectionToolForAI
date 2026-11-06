'use strict';
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');

const source = fs.readFileSync(
  'vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const script = source.match(/const trustedGUIJavaScript = `([\s\S]*?)`/);
assert.ok(script,'native GUI JavaScript must be bundled in the trusted client');

class Element {
  constructor(id) {
    this.id=id;this.textContent='';this.value='';this.disabled=false;
    this.hidden=false;this.children=[];this.listeners={};this.attributes={};
    this.classList={toggle(){}};
  }
  addEventListener(type,cb){this.listeners[type]=cb;}
  setAttribute(key,value){this.attributes[key]=value;}
  getAttribute(key){return this.attributes[key]||null;}
  replaceChildren(){this.children=[];}
  append(...nodes){this.children.push(...nodes);}
  click(){assert.ok(this.listeners.click,'no click handler '+this.id);this.listeners.click();}
}
const cache=new Map();
const get=id=>{
  if(!cache.has(id))cache.set(id,new Element(id));
  return cache.get(id);
};
const token='0123456789abcdef'.repeat(4);
let requests=[];
let budget=16;
let allowInspection=true;
const fetch=async(endpoint,options)=>{
  assert.equal(options.method,'POST');
  assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
  requests.push(endpoint);
  if(endpoint==='/api/session')return {ok:true,json:async()=>({
    ok:true,closed:false,active:false,remaining:budget,expires_in_seconds:300,
    device_id:'a'.repeat(64),username:'alice',
  })};
  if(endpoint==='/api/features')return {ok:true,json:async()=>({
    ok:true,session_ping:true,project_info:false,tasks_preview:true,
    git_status:false,remote_control_available:false,remaining:--budget,
  })};
  if(endpoint==='/api/tasks-preview')return {ok:true,json:async()=>({
    ok:true,total:2,labels:['Build','Inspect <script>'],
    ids:[101,102],remaining:--budget,
  })};
  if(endpoint==='/api/tasks-inspect/101'){
    if(!allowInspection)return {ok:false,status:403};
    return {ok:true,json:async()=>({
      ok:true,task_id:101,eligible:true,remote_control_available:false,
      remaining:--budget,
    })};
  }
  if(endpoint==='/api/close')return {ok:true,json:async()=>({ok:true,closed:true})};
  throw new Error('unexpected native route '+endpoint);
};
const sandbox={
 document:{getElementById:get,createElement:tag=>new Element(tag)},
 window:{addEventListener(){}},history:{replaceState(){}},
 location:{hash:'#'+token,pathname:'/'},
 fetch,setInterval(){},Date,Number,String,Promise,
};
vm.createContext(sandbox);
new vm.Script(script[1],{filename:'trusted-task-inspect.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<25;i++)await Promise.resolve();};
(async()=>{
  await flush();
  get('features').click();await flush();
  get('tasks-preview').click();await flush();
  const rows=get('task-preview-labels').children;
  assert.equal(rows.length,2);
  assert.equal(rows[1].textContent,'Inspect <script>',
    'HTML-looking text must stay literal');
  assert.equal(rows[0].children.length,2,
    'stable Task.ID should expose read-only inspect control');
  const action=rows[0].children[1];
  assert.equal(action.textContent,'Kiểm tra ID');
  const before=requests.length;
  action.click();await flush();
  assert.deepEqual(requests.slice(before),['/api/tasks-inspect/101'],
    'selected task must send only the canonical ID, never commands');
  assert.match(get('output').textContent,/Task ID 101 đã được xác minh/);
  assert.equal(budget,13);
  assert.equal(get('state').textContent,'Phiên cục bộ sẵn sàng');
  get('disconnect').click();await flush();
  assert.equal(get('task-preview-labels').children.length,0,
    'disconnect must erase task selections immediately');
  assert.equal(get('output').textContent,'Đã ngắt phiên.');
  assert.ok(requests.every(path=>path.startsWith('/api/')),
    'native GUI contacted an untrusted PHP or external route');
  console.log('PASS: authenticated task selection stays ID-only and is scrubbed on close');
})().catch(error=>{console.error(error);process.exitCode=1;});
