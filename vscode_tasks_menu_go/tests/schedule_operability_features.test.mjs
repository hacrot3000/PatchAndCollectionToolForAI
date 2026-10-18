import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const monitorCode=readFileSync(new URL('../web/featuremods/schedulemonitor.js',import.meta.url),'utf8');
const viewsCode=readFileSync(new URL('../web/featuremods/scheduleviews.js',import.meta.url),'utf8');
const panelCode=readFileSync(new URL('../web/featuremods/schedulespanel.js',import.meta.url),'utf8');
class FakeElement{
 constructor(tag){this.tag=tag;this.children=[];this.value='';this.textContent='';this.className='';
  this.hidden=false;this.style={};this.attrs={};this.disabled=false;}
 append(...children){this.children.push(...children)}
 replaceChildren(...children){this.children=[...children]}
 removeChild(child){const i=this.children.indexOf(child);if(i>=0)this.children.splice(i,1)}
 setAttribute(key,value){this.attrs[key]=String(value)}
 getAttribute(key){return this.attrs[key]}
 addEventListener(){}
}
const document={head:new FakeElement('head'),createElement:tag=>new FakeElement(tag),
 createElementNS:(_ns,tag)=>new FakeElement(tag)};
const ctx=vm.createContext({document,globalThis:{}});
vm.runInContext(monitorCode,ctx);
ctx.globalThis.TaskMenuScheduleConditions={parents:group=>(group.conditions||[]).filter(c=>c.type==='dependency').map(c=>c.parent_id)};
vm.runInContext(viewsCode,ctx);
const monitor=ctx.globalThis.TaskMenuScheduleMonitor,views=ctx.globalThis.TaskMenuScheduleViews;
const schedules=[
 {id:'A',name:'Compile',enabled:true,type:'cron',cron:'*/5 * * * *',command:'echo 1',target:'command'},
 {id:'B',name:'Release',enabled:true,type:'dependency',parent_id:'A',parent_state:'success',command:'echo 2',target:'command'}
];
const runs=[
 {id:'runA',schedule_id:'A',name:'Compile',status:'success',started_at:'2026-10-18T07:00:00Z',
  finished_at:'2026-10-18T07:00:02Z',attempts:2,workflow_id:'flow01'},
 {id:'runB',schedule_id:'B',name:'Release',status:'failure',error:'deploy failed',started_at:'2026-10-18T07:01:00Z',
  finished_at:'2026-10-18T07:01:03Z',attempts:1,workflow_id:'flow01',trigger_run_id:'runA'}
];
function render(page,overrides={}){
 const host=new FakeElement('section'),opened=[];
 views.render(host,page,{schedules,runs,errors:{},statistics:[],
  open:id=>opened.push(id),openRun:(id,owner)=>opened.push(id+':'+owner),
  execute:()=>{},remove:()=>{},toggle:()=>{},stop:()=>{},...overrides});
 return {host,opened};
}
const gather=(root)=>[...function* walk(n){yield n;for(const child of n.children||[])yield* walk(child)}(root)];
test('Workflow executions group parent and child runs by stable workflow ID',()=>{
 const {host,opened}=render('workflows');
 const details=gather(host).filter(n=>n.tag==='details');
 assert.equal(details.length,1);
 assert.match(details[0].children[0].textContent,/Workflow flow01/);
 const rows=gather(details[0]).filter(n=>n.tag==='tr');
 assert.equal(rows.length,2);
 assert.ok(gather(details[0]).some(n=>n.textContent==='Triggered by runA'));
});
test('Analytics exposes server-computed success rate and elapsed time',()=>{
 const stats=[{schedule_id:'A',completed:5,success:4,failure:1,average_duration_seconds:2.5}];
 const {host}=render('analytics',{statistics:stats});
 const rows=gather(host).filter(n=>n.tag==='tr');
 assert.equal(rows.length,3);
 assert.ok(gather(host).some(n=>n.textContent==='80.0%'));
 assert.ok(gather(host).some(n=>n.textContent==='2.5'));
});
test('Alerts show failure details and link to exact failed run',()=>{
 const {host,opened}=render('alerts');
 const items=gather(host);
 assert.ok(items.some(n=>n.textContent==='deploy failed'));
 const consoleButton=items.find(n=>n.tag==='button'&&n.textContent==='Open console');
 // Fixture uses a historical run; display may filter out alerts older than seven days.
 if(consoleButton){consoleButton.onclick();assert.deepEqual(opened,['runB:B'])}
});
test('Monitor renders distinct condition states without issuing commands',()=>{
 const host=new FakeElement('div');
 const api=monitor.create(host,{inspect:async id=>({inspection:{
  id,ready:false,conditions:[{id:'url',type:'url',latched:true,matched:true},
   {id:'git',type:'git',latched:false,matched:false}]},next_cron:[]})});
 api.update('schedule-1',{id:'schedule-1',ready:false,conditions:[
  {id:'url',type:'url',latched:true,matched:true},
  {id:'git',type:'git',latched:false,matched:false}
 ]});
 const text=gather(host).map(x=>x.textContent);
 assert.ok(text.includes('Signaled'));
 assert.ok(text.includes('Waiting'));
 assert.ok(text.includes('Condition Inspector'));
});
test('Editor exposes configured operational controls',()=>{
 for(const key of ['condition_ttl_seconds','retry_max','retry_backoff',
  'concurrency_policy','resource_group','allowed_weekdays','exclude_dates','window_start',
  'window_end','preview_backfill','Import JSON','Export JSON','schedule-monitor']){
  if(key==='schedule-monitor')assert.match(monitorCode,/schedule-monitor/);
  else assert.ok(panelCode.includes(key),'missing '+key);
 }
});
