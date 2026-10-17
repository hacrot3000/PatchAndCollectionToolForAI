import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const conditions=readFileSync(new URL('../web/featuremods/scheduleconditions.js',import.meta.url),'utf8');
const views=readFileSync(new URL('../web/featuremods/scheduleviews.js',import.meta.url),'utf8');
class FakeElement {
 constructor(tag){this.tag=tag;this.children=[];this.attrs={};this.value='';this.className='';this.textContent='';
  this.style={};this.hidden=false;}
 append(...nodes){this.children.push(...nodes)}
 replaceChildren(...nodes){this.children=[...nodes]}
 removeChild(node){const index=this.children.indexOf(node);if(index<0)throw Error('Node not found');this.children.splice(index,1);return node}
 setAttribute(name,value){this.attrs[name]=String(value)}
 getAttribute(name){return this.attrs[name]}
 addEventListener(){}
}
const document={
 head:new FakeElement('head'),createElement:tag=>new FakeElement(tag),
 createElementNS:(_ns,tag)=>new FakeElement(tag)
};
const ctx=vm.createContext({document,globalThis:{}});
vm.runInContext(conditions,ctx);
vm.runInContext(views,ctx);
const cond=ctx.globalThis.TaskMenuScheduleConditions,display=ctx.globalThis.TaskMenuScheduleViews;
const copy=value=>JSON.parse(JSON.stringify(value));
const jobs=[
 {id:'A',name:'Build',type:'cron',cron:'*/5 * * * *',target:'command',command:'echo build',enabled:true},
 {id:'B',name:'Test',type:'dependency',parent_id:'A',parent_state:'success',enabled:true},
 {id:'C',name:'Package',type:'dependency',parent_id:'A',parent_state:'success',enabled:false},
 {id:'D',name:'Deploy',type:'multi',enabled:true,condition_group:{
  operator:'and',conditions:[{id:'d1',type:'dependency',parent_id:'B'},
   {id:'d2',type:'dependency',parent_id:'C'}],groups:[{operator:'or',conditions:[{id:'d3',type:'cron',cron:'* * * * *'}],groups:[]}]
 }}
];
const runs=[
 {id:'run1',schedule_id:'A',name:'Build',status:'success',exit_code:0,started_at:'2026-10-17T06:00:00Z',finished_at:'2026-10-17T06:00:02Z',reason:'cron'},
 {id:'run2',schedule_id:'B',name:'Test',status:'running',exit_code:0,started_at:'2026-10-17T06:01:00Z',reason:'after Build'}
];

test('legacy dependency is converted into one condition without data loss',()=>{
 const source={type:'dependency',parent_id:'A',parent_state:'failure',exit_code:42,
  output_contains:'ERROR',output_absent:'IGNORE'};
 const group=copy(cond.legacy(source));
 assert.equal(group.operator,'or');
 assert.equal(group.conditions.length,1);
 assert.equal(group.conditions[0].parent_id,'A');
 assert.equal(group.conditions[0].exit_code,42);
 assert.equal(group.conditions[0].output_contains,'ERROR');
});

test('recursive AND/OR group normalization preserves arbitrary sibling branches',()=>{
 const group={operator:'and',conditions:Array.from({length:38},(_,i)=>({id:'c'+i,type:'url',url:'https://example.com/'+i})),
  groups:[{operator:'or',conditions:[{id:'dependency',type:'dependency',parent_id:'A'}],groups:[]}]};
 const normalized=copy(cond.normalize(group));
 assert.equal(normalized.conditions.length,38);
 assert.deepEqual(copy(cond.parents(normalized)),['A']);
 assert.equal(normalized.groups[0].operator,'or');
});

test('branched dependency DAG includes every parent and dependent',()=>{
 const graph=display.graph(jobs);
 assert.deepEqual(copy(graph.parents.get('D')),['B','C']);
 assert.deepEqual(copy(graph.children.get('A')).sort(),['B','C']);
 assert.deepEqual(copy(graph.children.get('B')),['D']);
 assert.ok(graph.depth.get('D')>graph.depth.get('B'));
 assert.ok(graph.depth.get('B')>graph.depth.get('A'));
});

test('overview displays all schedules with operational actions',()=>{
 const host=new FakeElement('div'),actions=[];
 display.render(host,'overview',{schedules:jobs,runs,errors:{},
  open:id=>actions.push(['open',id]),execute:id=>actions.push(['run',id]),
  remove:item=>actions.push(['remove',item.id]),toggle:item=>actions.push(['toggle',item.id])});
 const holder=host.children[1],table=holder.children[0],body=table.children[1];
 assert.equal(body.children.length,4);
 const first=body.children[0],identity=first.children[0];
 identity.children[0].onclick();
 assert.deepEqual(actions,[['open','A']]);
 const menu=first.children.at(-1);menu.children[1].onclick();
 assert.deepEqual(actions.at(-1),['run','A']);
});

test('all-run history includes running and finished entries and console actions',()=>{
 const host=new FakeElement('div'),logActions=[];
 display.render(host,'runs',{schedules:jobs,runs,errors:{},
  open:()=>{},openRun:(id,owner)=>logActions.push([id,owner]),stop:()=>{}});
 const tbody=host.children[1].children[0].children[1];
 assert.equal(tbody.children.length,2);
 tbody.children[0].children.at(-1).children[0].onclick();
 assert.deepEqual(logActions,[['run1','A']]);
});

test('graph paints branching edges and opens clicked nodes',()=>{
 const host=new FakeElement('div'),actions=[];
 display.render(host,'graph',{schedules:jobs,runs,open:id=>actions.push(id)});
 const svg=host.children.at(-1).children[0];
 const links=svg.children.filter(n=>n.attrs.class==='schedule-graph-line');
 assert.equal(links.length,4,'A→B, A→C, B→D, C→D');
 const nodes=svg.children.filter(n=>n.attrs.class==='schedule-graph-node');
 assert.equal(nodes.length,4);
 nodes.find(n=>n.attrs['aria-label']==='Open Deploy').children[0];
 assert.ok(svg.attrs.width);
});

test('schedule workspace buttons and multi-condition form are wired',()=>{
 const ui=readFileSync(new URL('../web/featuremods/schedulespanel.js',import.meta.url),'utf8');
 for(const part of [
  "['overview','All schedules']","['runs','All runs']","['graph','Dependency graph']",
  "globalThis.TaskMenuScheduleViews.render","globalThis.TaskMenuScheduleConditions.create",
  "model.condition_group=model.type==='multi'","scheduleShowPage(page||(id?'edit':'overview'))"
 ])assert.ok(ui.includes(part),part);
});
