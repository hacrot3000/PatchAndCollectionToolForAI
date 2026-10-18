import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const ui=readFileSync(new URL('../web/featuremods/schedulespanel.js',import.meta.url),'utf8');
const segment=(start,end)=>{
 const a=ui.indexOf(start),b=ui.indexOf(end,a+start.length);
 assert.ok(a>=0&&b>a,'missing function '+start);
 return ui.slice(a,b);
};
const handlers=segment("function defaultSchedule(){","function options(")
 + '\n'+segment("function load(id=''){","function getDefinition(){")
 +'\n'+segment("function clearScheduleConsole(","function closePanel(){");
class Element {
 constructor(tag,text=''){
  this.tag=tag;this.textContent=text;this.children=[];this.value='';this.type='text';this.style={};this.dataset={};
  this.disabled=false;this.scrollTop=0;
  this.classList={toggle(){}};
 }
 append(...items){this.children.push(...items)}
 replaceChildren(...items){this.children=[...items]}
}
const defs=[
 {id:'A',name:'Build',enabled:true,type:'cron',cron:'* * * * *'},
 {id:'B',name:'Test',enabled:true,type:'cron',cron:'* * * * *'}
];
const runs=[
 {id:'A1',schedule_id:'A',name:'Build',status:'success',started_at:'2026-10-18T00:00:00Z',reason:'cron'},
 {id:'B1',schedule_id:'B',name:'Test',status:'running',started_at:'2026-10-18T00:01:00Z',reason:'manual'}
];
function fixture(){
 const formItems=new Map(),getField=name=>{
  if(!formItems.has(name))formItems.set(name,new Element('input'));
  return formItems.get(name);
 };
 const data={items:defs,runs,errors:{},selected:'',run:'',logRequest:0,page:'edit',
  form:{dataset:{id:''},elements:{namedItem:getField}},relations:null,
  expression:null,runBox:new Element('section'),console:new Element('pre'),
  consoleTitle:new Element('div'),copyConsoleButton:new Element('button')};
 const copied=[],pending=new Map();
 const fetch=(_url)=>{
  const id=new URL(_url,'https://example.test').searchParams.get('run_id');
  return new Promise(resolve=>pending.set(id,resolve));
 };
 const ctx=vm.createContext({
  data,fetch,scheduleGitRepos:[],app:{jsonFetch:async()=>({schedules:defs,runs,trigger_errors:{}}),showError:err=>{throw err}},
  el:(tag,text)=>new Element(tag,text),options:()=>{},updateFields:()=>{},
  field:getField,renderPanel:()=>{},scheduleShowPage:()=>{},
  globalThis:{TaskMenuScheduleConditions:{legacy:()=>({}),empty:()=>({type:'cron',cron:'*/5 * * * *'})},TaskMenuScheduleViews:{relationships:()=>{}}},
  window:{isSecureContext:true},navigator:{clipboard:{writeText:async text=>{copied.push(text)}}},
  document:{},setTimeout:()=>{},
 });
 vm.runInContext(handlers,ctx);
 const exec=name=>vm.runInContext(name,ctx);
 return {data,copied,pending,exec};
}
const respond=(fixture,id,body)=>{
 const resolve=fixture.pending.get(id);
 assert.ok(resolve,'pending console request '+id);
 resolve({ok:true,text:async()=>body});
};
const historyEntries=fixture=>fixture.data.runBox.children.filter(c=>c.tag==='div'&&c.children.length===2);
test('detail history contains only the selected schedule runs',()=>{
 const f=fixture();f.exec('load')('A');
 assert.equal(historyEntries(f).length,1);
 assert.match(historyEntries(f)[0].children[0].textContent,/Build/);
 f.exec('load')('B');
 assert.equal(historyEntries(f).length,1);
 assert.match(historyEntries(f)[0].children[0].textContent,/Test/);
});
test('switching schedule clears selected run and its console immediately',()=>{
 const f=fixture();f.exec('load')('A');
 f.data.run='A1';f.data.console.textContent='Old A console';f.data.copyConsoleButton.disabled=false;
 f.exec('load')('B');
 assert.equal(f.data.run,'');
 assert.equal(f.data.console.textContent,'');
 assert.equal(f.data.copyConsoleButton.disabled,true);
});
test('create-new view does not show all runs from other schedules',()=>{
 const f=fixture();f.exec('load')('A');
 f.exec('load')('');
 assert.equal(f.data.run,'');
 assert.equal(historyEntries(f).length,0);
 assert.match(f.data.runBox.children[0].textContent,/New schedule/);
});
test('stale console response cannot overwrite newly selected schedule console',async()=>{
 const f=fixture();
 f.exec('load')('A');f.data.run='A1';
 const first=f.exec('consoleLog')('A1');
 f.exec('load')('B');f.data.run='B1';
 const second=f.exec('consoleLog')('B1');
 respond(f,'B1','Console for B');
 await second;
 respond(f,'A1','Stale console A');
 await first;
 assert.equal(f.data.console.textContent,'Console for B');
 assert.equal(f.data.run,'B1');
});
test('copy action fetches console by explicit run ID rather than visible content',async()=>{
 const f=fixture();f.exec('load')('B');f.data.console.textContent='Different visible log';
 const copy=f.exec('copyScheduleRunConsole')('A1');
 respond(f,'A1','Source A console');
 await copy;
 assert.deepEqual(f.copied,['Source A console']);
 assert.equal(f.data.console.textContent,'Different visible log');
});
test('refresh does not restore a selected run belonging to a different schedule',async()=>{
 const f=fixture();f.exec('load')('B');f.data.run='A1';f.data.console.textContent='Stale A';
 await f.exec('refresh')();
 assert.equal(f.data.run,'');
 assert.equal(f.data.console.textContent,'');
 assert.equal(historyEntries(f).length,1);
});
test('dependency section font follows same size as the Schedule form',()=>{
 assert.ok(ui.includes("'.schedule-relation-panel{font-size:11px"));
 assert.ok(ui.includes("'.schedule-relation-panel .links{font-size:inherit"));
});
