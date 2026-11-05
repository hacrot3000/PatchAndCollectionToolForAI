'use strict';
// This fixture executes the exact embedded native GUI JavaScript supplied on
// stdin by its Go test. It never connects to a real Agent or a web server.
const assert=require('node:assert/strict');
const fs=require('node:fs');
const script=fs.readFileSync(0,'utf8');
class Element {
 constructor(tag='div',id=''){
  this.tagName=tag.toUpperCase();this.id=id;this.children=[];
  this.listeners={};this.value='';this.hidden=false;this.disabled=false;
  this.textContent='';this.classList={toggle(){},add(){},remove(){}};
 }
 append(...nodes){this.children.push(...nodes);}
 replaceChildren(...nodes){this.children=nodes;}
 addEventListener(event,fn){this.listeners[event]=fn;}
 setAttribute(name,value){this[name]=value;}
 emit(event){
  assert.equal(typeof this.listeners[event],'function','missing '+event+' handler for '+this.id);
  return this.listeners[event]({type:event,key:'Enter',preventDefault(){}});
 }
}
const elements=new Map();
const document={
 getElementById(id){
  if(!elements.has(id))elements.set(id,new Element('div',id));
  return elements.get(id);
 },
 createElement(tag){return new Element(tag);}
};
const el=id=>document.getElementById(id);
const confirmations=[];
let allowNextConfirmation=true;
const window={
 confirm(message){
  confirmations.push(message);
  const decision=allowNextConfirmation;
  allowNextConfirmation=true;
  return decision;
 },
 addEventListener(){}
};
const history={replaceState(){}};
const location={hash:'#'+'a'.repeat(64),pathname:'/'};
const requests=[];
let remaining=16;
let longTaskEnabled=true;
const fetch=async(endpoint,options)=>{
 assert.match(endpoint,/^\/api\/[a-z/-]+(?:\/[0-9]+)*$/);
 assert.equal(options.method,'POST');
 assert.equal(options.headers['X-TaskDeck-Local-Capability'],'a'.repeat(64));
 requests.push(endpoint);
 if(endpoint==='/api/session'){
  return {ok:true,json:async()=>({
   ok:true,closed:false,remaining:16,expires_in_seconds:300,
   device_id:'a'.repeat(64),username:'alice'
  })};
 }
 const data={ok:true,remaining:--remaining,message:'ok'};
 if(endpoint==='/api/features')Object.assign(data,{
  remote_control_available:false,session_ping:true,project_info:true,
  tasks_preview:true,task_run:true,task_long:longTaskEnabled,git_status:true,terminal:true
 });
 if(endpoint==='/api/tasks-preview')Object.assign(data,{
  total:2,labels:['One','Two'],ids:[1,2]
 });
 if(endpoint==='/api/tasks-inspect/1')Object.assign(data,{
  task_id:1,eligible:true,remote_control_available:false,
  input_id:'region',choices:['us','eu'],input_type:'',
  input_id_2:'mode',choices_2:['debug','release']
 });
 if(endpoint==='/api/tasks-inspect/2')Object.assign(data,{
  task_id:2,eligible:true,remote_control_available:false,
  input_id:'tag',choices:[],input_type:'promptString',
  input_id_2:'',choices_2:[]
 });
 if(endpoint.startsWith('/api/tasks-start/'))Object.assign(data,{
  task_id:1,status:'started',exit_code:-1,output:'',
  more:false,truncated:false,remote_control_available:false
 });
 if(endpoint==='/api/terminal-open'||endpoint==='/api/terminal-close')Object.assign(data,{
  remote_control_available:false,
  state:endpoint==='/api/terminal-open'?'running':'closed',
  output:'',more:false,truncated:false
 });
 if(endpoint.startsWith('/api/tasks-poll/'))Object.assign(data,{
  task_id:1,status:'running',exit_code:-1,output:'chunk',
  more:false,truncated:false,remote_control_available:false
 });
 return {ok:true,status:200,json:async()=>data};
};
const settle=async()=>{for(let i=0;i<10;i++)await Promise.resolve();};
async function action(element,event='click'){
 element.emit(event);
 await settle();
}
const row=label=>{
 const found=el('task-preview-labels').children.find(child=>child.textContent===label);
 assert.ok(found,'expected task row '+label);
 return found;
};
const byText=(parent,text)=> {
 const result=parent.children.find(child=>child.textContent===text);
 assert.ok(result,'expected control '+text);
 return result;
};
const controls=(parent,tag)=>parent.children.filter(child=>child.tagName===tag.toUpperCase());
(async()=>{
 new Function('document','window','location','history','fetch','setInterval','setTimeout',script)(
  document,window,location,history,fetch,()=>0,()=>0
 );
 await settle();
 await action(el('features'));
 await action(el('tasks-preview'));
 assert.equal(row('One').textContent,'One');
 await action(byText(row('One'),'Kiểm tra ID'));
 let picks=controls(row('One'),'select');
 assert.equal(picks.length,2);
 picks[0].value='2';picks[0].emit('change');
 picks[1].value='2';picks[1].emit('change');

 // Unchanged capability refreshes must never replace the focused task
 // control; changing the host's long-task flag must add/remove its button.
 const originalRow=row('One');
 const originalSelect=picks[0];
 await action(el('features'));
 assert.equal(row('One'),originalRow,'feature refresh rebuilt an unchanged task row');
 assert.equal(controls(row('One'),'select')[0],originalSelect,
  'unchanged permissions discarded the focused select');
 longTaskEnabled=false;
 await action(el('features'));
 assert.notEqual(row('One'),originalRow,
  'revoking long-task permission did not rebuild task controls');
 assert.equal(row('One').children.some(item=>item.textContent==='Chạy task dài'),false,
  'revoked long-task permission left its button in the DOM');
 assert.deepEqual(controls(row('One'),'select').map(x=>x.value),['2','2'],
  'capability change discarded the selected task arguments');
 longTaskEnabled=true;
 await action(el('features'));
 assert.equal(byText(row('One'),'Chạy task dài').disabled,false,
  'restored long-task permission did not restore the run button');
 assert.deepEqual(controls(row('One'),'select').map(x=>x.value),['2','2']);

 // A filter/sort rerender must preserve both selected host-approved choices.
 el('task-sort').value='za';
 await action(el('task-sort'),'change');
 picks=controls(row('One'),'select');
 assert.deepEqual(picks.map(x=>x.value),['2','2']);
 el('task-filter').value='Two';
 await action(el('task-filter'),'input');
 assert.equal(el('task-preview-labels').children.length,1);
 el('task-filter').value='';
 await action(el('task-filter'),'input');
 picks=controls(row('One'),'select');
 assert.deepEqual(picks.map(x=>x.value),['2','2']);

 // A PTY already open on the host must disable both short and long task
 // execution in the native GUI; even a forged click cannot send a run.
 await action(el('terminal-open'));
 assert.equal(byText(row('One'),'Chạy task dài').disabled,true);
 assert.equal(byText(row('One'),'Chạy task').disabled,true);
 const blockedCount=requests.length;
 await action(byText(row('One'),'Chạy task dài'));
 assert.equal(requests.length,blockedCount,'task launch escaped active PTY guard');
 await action(el('terminal-close'));
 // An accidental click or a rejected native confirmation must not spend a
 // session action, even though the task source was inspected and authorized.
 const beforeStart=row('One');
 const cancelledCount=requests.length;
 allowNextConfirmation=false;
 await action(byText(beforeStart,'Chạy task dài'));
 assert.equal(requests.length,cancelledCount,'declined task ran on host');
 assert.match(confirmations.at(-1),/One \(ID 1\)/);
 assert.match(confirmations.at(-1),/region: eu/);
 assert.match(confirmations.at(-1),/mode: release/);
 assert.match(confirmations.at(-1),/thay đổi dữ liệu/);
 await action(byText(beforeStart,'Chạy task dài'));
 assert.ok(requests.includes('/api/tasks-start/1/4'),
  'two picks must retain the combined host-issued choice index');
 assert.equal(row('One'),beforeStart,'job start replaced unchanged task row');
 // TasksStart returns before a host job finishes; a regular short run
 // must be unavailable throughout the same long-job ownership.
 assert.equal(byText(row('One'),'Chạy task').disabled,true);
 assert.equal(byText(row('One'),'Chạy task dài').disabled,true);
 const whileLong=requests.length;
 await action(byText(row('One'),'Chạy task'));
 await action(byText(row('One'),'Chạy task dài'));
 assert.equal(requests.length,whileLong,'parallel task launch escaped long-job guard');
 await action(el('long-task-poll'));
 assert.equal(row('One'),beforeStart,'job output replaced unchanged task row');
 assert.deepEqual(controls(row('One'),'select').map(x=>x.value),['2','2']);

 // Prompt drafts are local-only, preserved while another long job is polled.
 await action(byText(row('Two'),'Kiểm tra ID'));
 let input=controls(row('Two'),'input')[0];
 assert.ok(input,'missing prompt input');
 input.value='release-4.2';input.emit('input');
 await action(el('long-task-poll'));
 assert.equal(controls(row('Two'),'input')[0],input,
  'poll discarded focused prompt element');
 assert.equal(input.value,'release-4.2');
 el('task-filter').value='One';
 await action(el('task-filter'),'input');
 el('task-filter').value='';
 await action(el('task-filter'),'input');
 assert.equal(controls(row('Two'),'input')[0].value,'release-4.2');

 // Re-fetching a trusted snapshot invalidates every old inspection/draft.
 await action(el('tasks-preview'));
 await action(byText(row('One'),'Kiểm tra ID'));
 assert.deepEqual(controls(row('One'),'select').map(x=>x.value),['1','1']);
 await action(byText(row('Two'),'Kiểm tra ID'));
 assert.equal(controls(row('Two'),'input')[0].value,'');
 await action(el('disconnect'));
 assert.equal(el('task-preview-labels').children.length,0);
 assert.equal(el('task-preview').hidden,true);
 assert.ok(requests.every(x=>x.startsWith('/api/')));
 console.log('PASS: task drafts, feature refresh focus stability, permission changes, long poll and logout');
})().catch(err=>{console.error(err);process.exitCode=1;});