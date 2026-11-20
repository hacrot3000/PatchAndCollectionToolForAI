import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const code=readFileSync(new URL('../web/featuremods/scheduleworkflowgraph.js',import.meta.url),'utf8');
class Fake{
 constructor(tag){this.tag=tag;this.children=[];this.attrs={};this.textContent='';this.className='';this.listeners={};}
 append(...items){this.children.push(...items)}
 replaceChildren(...items){this.children=[...items]}
 setAttribute(k,v){this.attrs[k]=String(v)}
 addEventListener(name,handler){this.listeners[name]=handler}
}
const document={head:new Fake('head'),createElement:tag=>new Fake(tag),
 createElementNS:(_uri,tag)=>new Fake(tag)};
const globalThis={};
const ctx=vm.createContext({document,globalThis});
vm.runInContext(code,ctx);
const graph=ctx.globalThis.TaskMenuWorkflowGraph;
const all=(host,tag)=>{
 const found=[];const walk=n=>{if(n.tag===tag)found.push(n);for(const x of n.children)walk(x)};
 walk(host);return found;
};
test('one root forks into two runs that fan in to deployment',()=>{
 const runs=[
  {id:'a',name:'Build',status:'success',started_at:'2026-10-18T07:00:00Z',workflow_id:'flow'},
  {id:'b',name:'Test',status:'success',started_at:'2026-10-18T07:01:00Z',trigger_run_id:'a',workflow_id:'flow'},
  {id:'c',name:'Package',status:'success',started_at:'2026-10-18T07:01:00Z',trigger_run_id:'a',workflow_id:'flow'},
  {id:'d',name:'Deploy',status:'queued',started_at:'2026-10-18T07:02:00Z',
   trigger_run_id:'c',trigger_run_ids:['b','c'],workflow_id:'flow'}
 ];
 const host=new Fake('div'),opened=[];
 graph.render(host,runs,(id,owner)=>opened.push([id,owner]));
 assert.equal(all(host,'path').filter(n=>n.attrs.class==='edge').length,4);
 assert.equal(all(host,'g').length,4);
 const deploy=all(host,'g').find(n=>n.attrs['aria-label']==='Open console for Deploy');
 assert.ok(deploy);
 deploy.listeners.click();
 assert.deepEqual(opened,[['d',undefined]]);
 assert.match(all(deploy,'text')[1].textContent,/queued/);
});
test('legacy run without origin appears as independent root',()=>{
 const host=new Fake('div');
 graph.render(host,[{id:'x',name:'Old task',status:'success',started_at:'2026-10-18T07:00:00Z'}]);
 assert.equal(all(host,'g').length,1);
 assert.equal(all(host,'path').filter(n=>n.attrs.class==='edge').length,0);
});
