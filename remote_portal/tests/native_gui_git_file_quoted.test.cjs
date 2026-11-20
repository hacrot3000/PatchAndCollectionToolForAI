'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');

const raw=fs.readFileSync('vscode_tasks_menu_go/cmd/taskdeck_remote_login/trusted_gui_assets.go','utf8');
const tick=String.fromCharCode(96);
const parts=raw.split('const trustedGUIJavaScript = '+tick);
assert.equal(parts.length,2);
const script=parts[1].split(tick)[0];

// E2EE output from Git with core.quotePath=true. Only display is
// decoded: the signed Stage request must contain precisely these bytes.
const status='## main\n M "a file.txt"\n M "\\303\\241o.txt"\n M variant[1].txt\n';
const token='0123456789abcdef'.repeat(4);
class Element{
 constructor(id){
  this.id=id;this.value='';this.textContent='';this.hidden=false;
  this.disabled=false;this.children=[];this.listeners={};this.attributes={};
  this.classList={toggle(){}};
 }
 addEventListener(kind,fn){this.listeners[kind]=fn}
 click(){assert.ok(this.listeners.click,'missing click handler for '+this.id);this.listeners.click()}
 append(...children){this.children.push(...children)}
 replaceChildren(){this.children=[]}
 setAttribute(key,value){this.attributes[key]=value}
 getAttribute(key){return this.attributes[key]||null}
}
const elements=new Map();
const get=id=>{
 if(!elements.has(id))elements.set(id,new Element(id));
 return elements.get(id);
};
let budget=16,confirmed=true,readCount=0,stageCalls=[];
const fetch=async(path,options)=>{
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],token);
 if(path==='/api/session')return {ok:true,json:async()=>({
  ok:true,closed:false,active:false,remaining:budget,expires_in_seconds:300,
  device_id:'a'.repeat(64),username:'alice',
 })};
 if(path==='/api/features')return {ok:true,json:async()=>({
  ok:true,session_ping:true,project_info:false,tasks_preview:false,
  task_run:false,task_long:false,git_status:true,
  git_stage_file:true,git_unstage_file:true,
  git_stage_tracked:false,git_unstage_all:false,terminal:false,
  remote_control_available:false,remaining:--budget,
 })};
 if(path==='/api/git-status'){
  readCount++;
  return {ok:true,json:async()=>({
   ok:true,status,remaining:--budget,
  })};
 }
 if(path==='/api/git-stage-file'){
  assert.equal(options.headers['Content-Type'],'application/json');
  const body=JSON.parse(options.body);
  assert.deepEqual(Object.keys(body),['index','snapshot']);
  stageCalls.push(body);
  return {ok:true,json:async()=>({
   ok:true,operation:'git.stage_file',head:'a'.repeat(40),
   remaining:--budget,remote_control_available:false,
  })};
 }
 throw Error('unexpected E2EE endpoint '+path);
};
const sandbox={
 document:{getElementById:get,createElement:name=>new Element(name)},
 window:{addEventListener(){},confirm(){return confirmed}},
 history:{replaceState(){}},location:{hash:'#'+token,pathname:'/'},
 fetch,Promise,Date,Number,String,encodeURIComponent,
 setInterval(){},setTimeout(){},
};
vm.createContext(sandbox);
new vm.Script(script,{filename:'native-git-file-quoted.js'}).runInContext(sandbox);
const flush=async()=>{for(let i=0;i<120;i++)await Promise.resolve()};
(async()=>{
 await flush();
 get('features').click();await flush();
 get('git').click();await flush();
 assert.equal(readCount,1);
 const labels=get('git-files').children.map(row=>row.children[1].textContent);
 assert.deepEqual(labels,['a file.txt','áo.txt','variant[1].txt'],
  'Git C-quoted UTF-8 paths were not decoded for display');
 const options=get('git-file-select').children;
 assert.equal(options.length,4,
  'quoted files should be selectable by Git Status index');
 assert.deepEqual(options.map(x=>x.value),['','1','2','3']);
 assert.match(options[2].textContent,/áo\.txt/);
 get('git-filter').value='ÁO';
 get('git-filter').listeners.input();await flush();
 assert.deepEqual(get('git-files').children.map(row=>row.children[1].textContent),
  ['áo.txt'],'search must use readable Unicode display path');
 get('git-file-select').value='2';
 get('git-file-select').listeners.change();await flush();
 assert.equal(get('git-stage-file').disabled,false);
 get('git-stage-file').click();await flush();
 assert.deepEqual(stageCalls,[{index:2,snapshot:status}],
  'Git Stage changed authenticated raw snapshot or sent a decoded path');
 assert.equal(get('git-stage-file').disabled,true,
  'success must invalidate stale status and row selection');
 console.log('PASS: native Git C-style quoted status display and index-only Stage');
})().catch(error=>{console.error(error);process.exitCode=1});
