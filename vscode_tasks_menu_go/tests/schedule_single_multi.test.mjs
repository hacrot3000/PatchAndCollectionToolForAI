import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const editorSource=readFileSync(new URL('../web/featuremods/scheduleconditions.js',import.meta.url),'utf8');
const panelSource=readFileSync(new URL('../web/featuremods/schedulespanel.js',import.meta.url),'utf8');
class Element {
 constructor(tag){this.tag=tag;this.children=[];this.attrs={};this.value='';this.disabled=false;
  this.textContent='';this.style={};this.dataset={};this.title='';this.className='';}
 append(...children){this.children.push(...children)}
 replaceChildren(...children){this.children=[...children]}
 setAttribute(name,value){this.attrs[name]=String(value)}
 querySelector(selector){
  if(this.tag===selector)return this;
  for(const child of this.children){
   const found=child.querySelector?.(selector);
   if(found)return found;
  }
  return null;
 }
}
const document={head:new Element('head'),createElement:tag=>new Element(tag)};
const ctx=vm.createContext({document,globalThis:{},console,Promise});
vm.runInContext(editorSource,ctx);
const module=ctx.globalThis.TaskMenuScheduleConditions;
const copy=value=>JSON.parse(JSON.stringify(value));
const find=(node,tag,text)=>[...function* walk(n){yield n;for(const child of n.children||[])yield* walk(child)}(node)]
  .find(n=>n.tag===tag&&n.textContent===text);
const findAll=(node,tag)=>[...function* walk(n){yield n;for(const child of n.children||[])yield* walk(child)}(node)]
  .filter(n=>n.tag===tag);
function editor(group){
 const host=new Element('section'),events=[];
 const api=module.create(host,{onChange:state=>events.push(copy(state))});
 if(group)api.load(group);
 return {api,host,events};
}

test('new schedule starts with exactly one Cron condition',()=>{
 const {api,host}=editor();
 assert.equal(api.count(),1);
 assert.equal(api.singleCondition().type,'cron');
 assert.equal(find(host,'button','Remove').disabled,true);
 assert.ok(find(host,'div','Single condition · Use + Condition or + Group to combine triggers.'));
});
test('adding a condition switches to multi and enables AND/OR',()=>{
 const {api,host,events}=editor();
 find(host,'button','+ Condition').onclick();
 assert.equal(api.count(),2);
 assert.equal(api.singleCondition(),null);
 assert.equal(api.value().operator,'and');
 assert.equal(events.length,1);
 assert.equal(find(host,'button','Remove').disabled,false);
 const op=findAll(host,'select').find(n=>n.attrs['aria-label']==='Root AND OR operator');
 assert.equal(op.disabled,false);
 op.value='or';op.onchange();
 assert.equal(api.value().operator,'or');
});
test('removing back to one condition converts automatically and preserves its fields',()=>{
 const group=module.legacy({type:'url',url:'https://example.com/check',match_mode:'contains',
  match_text:'READY',interval_seconds:90});
 const {api,host}=editor(group);
 find(host,'button','+ Condition').onclick();
 assert.equal(api.count(),2);
 const removeButtons=findAll(host,'button').filter(n=>n.textContent==='Remove');
 removeButtons.at(-1).onclick();
 assert.equal(api.count(),1);
 assert.equal(api.singleCondition().url,'https://example.com/check');
 assert.equal(api.singleCondition().match_text,'READY');
 assert.equal(api.singleCondition().interval_seconds,90);
 assert.equal(find(host,'button','Remove').disabled,true);
});
test('adding a nested group and removing other conditions flattens to one',()=>{
 const {api,host}=editor();
 find(host,'button','+ Group').onclick();
 assert.equal(api.count(),2);
 const firstRemove=findAll(host,'button').find(n=>n.textContent==='Remove');
 firstRemove.onclick();
 assert.equal(api.count(),1);
 assert.equal(api.value().groups.length,0);
 assert.equal(api.singleCondition().type,'cron');
});
test('cannot remove the final condition or its sole containing group',()=>{
 const {api,host}=editor();
 assert.equal(find(host,'button','Remove').disabled,true);
 find(host,'button','+ Group').onclick();
 const groups=findAll(host,'button').filter(n=>n.textContent==='Remove group');
 assert.equal(groups.length,1);
 // Removing the group is valid because the root condition still exists.
 groups[0].onclick();
 assert.equal(api.count(),1);
 assert.equal(find(host,'button','Remove').disabled,true);
});
test('existing multi schedule with one remaining nested leaf loads as single',()=>{
 const {api}=editor({operator:'or',conditions:[],groups:[
  {operator:'and',conditions:[{id:'c1',type:'git',git_path:'packages/lib',git_ref:'main'}],groups:[]}
 ]});
 assert.equal(api.count(),1);
 assert.equal(api.value().groups.length,0);
 assert.equal(api.singleCondition().git_path,'packages/lib');
});
test('legacy dependency can become multi and return to single without losing exit-code filters',()=>{
 const parent=module.legacy({id:'task-123',type:'dependency',parent_id:'task-abc',
  parent_state:'failure',exit_code:4,output_contains:'failure'});
 const {api,host}=editor(parent);
 assert.equal(api.singleCondition().id,'legacy');
 find(host,'button','+ Condition').onclick();
 assert.equal(api.count(),2);
 const buttons=findAll(host,'button').filter(n=>n.textContent==='Remove');
 buttons.at(-1).onclick();
 assert.equal(api.singleCondition().parent_id,'task-abc');
 assert.equal(api.singleCondition().parent_state,'failure');
 assert.equal(api.singleCondition().exit_code,4);
 assert.equal(api.singleCondition().output_contains,'failure');
});
test('Git condition keeps editable path, dropdown and repo rescan across conversion',()=>{
 const host=new Element('section'),rescans=[];
 const api=module.create(host,{repositories:[{path:'.',name:'Workspace'},{path:'modules/client',name:'Client'}],
  preferredRepository:()=>({path:'modules/client'}),onRescanRepos:()=>rescans.push(true)});
 const kind=findAll(host,'select').find(n=>n.value==='cron');
 kind.value='git';kind.onchange();
 assert.equal(api.singleCondition().git_path,'modules/client');
 assert.ok(find(host,'button','↻ Rescan'));
 find(host,'button','↻ Rescan').onclick();
 assert.equal(rescans.length,1);
});
test('serialization uses legacy type for one leaf and multi for two or more',()=>{
 const from=panelSource.indexOf('function defaultSchedule(){');
 const end=panelSource.indexOf('function entries(',from);
 assert.ok(from>=0&&end>from);
 // Exercise the production serializer against controlled form inputs.
 const definitions=panelSource.slice(from,panelSource.indexOf('function options(',from))+
  panelSource.slice(panelSource.indexOf('function getDefinition(){',from),end);
 const fields=new Map(),get=k=>fields.get(k)||({value:'',checked:false});
 const expr=editor(module.legacy({type:'git',git_path:'modules/client',git_ref:'HEAD'}));
 const data={form:{dataset:{id:'existing-id'}},expression:expr.api};
 const sandbox={data,field:get,globalThis:{TaskMenuScheduleConditions:module}};
 fields.set('name',{value:'Sync'});fields.set('target',{value:'command'});
 fields.set('command',{value:'echo done'});
 fields.set('timezone',{value:'Local'});
 fields.set('cron',{value:'*/5 * * * *'});
 fields.set('enabled',{checked:true});
 vm.runInNewContext(definitions+';result=getDefinition();',sandbox);
 assert.equal(sandbox.result.id,'existing-id');
 assert.equal(sandbox.result.type,'git');
 assert.equal(sandbox.result.git_path,'modules/client');
 assert.equal(sandbox.result.condition_group,null);
 find(expr.host,'button','+ Condition').onclick();
 vm.runInNewContext(definitions+';result=getDefinition();',sandbox);
 assert.equal(sandbox.result.type,'multi');
 assert.equal(sandbox.result.condition_group.conditions.length,2);
 findAll(expr.host,'button').filter(n=>n.textContent==='Remove').at(-1).onclick();
 vm.runInNewContext(definitions+';result=getDefinition();',sandbox);
 assert.equal(sandbox.result.type,'git');
 assert.equal(sandbox.result.git_path,'modules/client');
});


test('remote Git condition keeps remote, branch and pull flag through single/multi transitions',()=>{
 const {api,host}=editor(module.legacy({type:'git',git_path:'modules/client',git_ref:'release',
  git_condition:'remote_change',git_remote:'upstream',git_auto_pull:true,interval_seconds:90}));
 let only=api.singleCondition();
 assert.equal(only.git_condition,'remote_change');
 assert.equal(only.git_remote,'upstream');
 assert.equal(only.git_auto_pull,true);
 assert.ok(findAll(host,'input').some(n=>n.type==='checkbox'&&n.checked===true));
 find(host,'button','+ Condition').onclick();
 assert.equal(api.singleCondition(),null);
 findAll(host,'button').filter(n=>n.textContent==='Remove').at(-1).onclick();
 only=api.singleCondition();
 assert.equal(only.git_ref,'release');
 assert.equal(only.git_remote,'upstream');
 assert.equal(only.git_auto_pull,true);
});

test('selecting remote Git trigger reveals auto-pull and turning it off clears pull flag',()=>{
 const {api,host}=editor();
 const kind=findAll(host,'select').find(n=>n.value==='cron');
 kind.value='git';kind.onchange();
 let trigger=findAll(host,'select').find(n=>n.value==='change');
 assert.ok(trigger);
 trigger.value='remote_change';trigger.onchange();
 const remote=findAll(host,'input').find(n=>n.placeholder==='origin');
 assert.ok(remote);
 remote.value='upstream';remote.oninput();
 const pull=findAll(host,'input').find(n=>n.type==='checkbox');
 assert.ok(pull);
 pull.checked=true;pull.onchange();
 assert.equal(api.singleCondition().git_auto_pull,true);
 assert.equal(api.singleCondition().git_remote,'upstream');
 trigger=findAll(host,'select').find(n=>n.value==='remote_change');
 trigger.value='dirty';trigger.onchange();
 assert.equal(api.singleCondition().git_auto_pull,false);
 assert.equal(findAll(host,'input').some(n=>n.type==='checkbox'),false);
});
