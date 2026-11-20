import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source=readFileSync(new URL('../web/featuremods/split.js',import.meta.url),'utf8');
const section=(first,last)=>{
 const a=source.indexOf(first),b=source.indexOf(last,a+first.length);
 assert.ok(a>=0&&b>a,'missing split-tree declaration');
 return source.slice(a,b);
};
const pure=section('function clampRatio(','const splitTabPalette=');
const targetHelper=section('function mergeTargetsFor(','function chooseMergeTarget(');
function fixture(ids=['1','2','3','4','5']){
 const entries=ids.map(id=>[id,{meta:{id,label:id,status:'ready'}}]);
 const code=[
  'const splitSupported=true;let roots=[];',
  'const app={views:new Map('+JSON.stringify(entries)+'),active:"1"};',
  pure,
  'function viewsInTabOrder(){return [...app.views.values()];}',
  targetHelper,
  'globalThis.model={merge:(s,t,o="vertical")=>mergeSplitGroups(s,t,o),tree:()=>JSON.parse(JSON.stringify(roots)),leaves:()=>JSON.parse(JSON.stringify(roots.map(root=>leafIDs(root,[])))),groups:()=>JSON.parse(JSON.stringify(getGroups())),restore:groups=>{roots=[];return groups.map(applyOperation);},candidates:id=>JSON.parse(JSON.stringify(mergeTargetsFor(id).map(v=>v.meta.id)))};'
 ].join('\n');
 const context=vm.createContext({});
 vm.runInContext(code,context,{filename:'split-tree-logic.js'});
 return context.model;
}
function assertGroup(model,expected){
 const groups=model.leaves();
 assert.equal(groups.length,1,'must have one root');
 assert.deepEqual([...groups[0]].sort(),[...expected].sort());
 assert.equal(new Set(groups[0]).size,expected.length,'duplicate terminal leaf');
}
test('merge 3 into 1 after 1 was merged with 2 (reported failure)',()=>{
 const m=fixture();
 assert.equal(m.merge('1','2','vertical'),true);
 assert.equal(m.merge('3','1','horizontal'),true);
 assertGroup(m,['1','2','3']);
 const root=m.tree()[0];
 assert.equal(root.orientation,'horizontal');
 assert.equal(root.first.type,'split');
});
test('reverse merge direction preserves 1 and 2',()=>{
 const m=fixture();m.merge('1','2','vertical');m.merge('1','3','horizontal');
 assertGroup(m,['1','2','3']);
});
test('merging two groups retains both complete nested trees',()=>{
 const m=fixture();
 m.merge('1','2','vertical');m.merge('3','4','horizontal');m.merge('2','4','vertical');
 assertGroup(m,['1','2','3','4']);
 assert.equal(m.tree()[0].first.type,'split');
 assert.equal(m.tree()[0].second.type,'split');
});
test('same-group merge is a no-op; merge-target menus exclude current group',()=>{
 const m=fixture(['1','2','3']);
 m.merge('1','2');
 const before=m.tree();
 assert.equal(m.merge('2','1'),false);
 assert.deepEqual(m.tree(),before);
 assert.deepEqual([...m.candidates('1')],['3']);
 m.merge('3','1');
 assert.deepEqual([...m.candidates('1')],[]);
 assert.deepEqual([...m.candidates('2')],[]);
});
test('five terminals survive save and restore of nested layout',()=>{
 const m=fixture();
 m.merge('1','2','vertical');
 m.merge('3','1','horizontal');
 m.merge('4','2','vertical');
 m.merge('5','4','horizontal');
 assertGroup(m,['1','2','3','4','5']);
 const tree=m.tree(),saved=m.groups();
 assert.equal(saved.length,4);
 assert.ok(m.restore(saved).every(Boolean));
 assertGroup(m,['1','2','3','4','5']);
 assert.deepEqual(m.tree(),tree);
});
test('merge no longer detaches a target; Move remains leaf-specific',()=>{
 const body=section('function mergeWith(view,orientation){','function bindFocusTracking(view)');
 assert.match(body,/mergeSplitGroups\(view\.meta\.id,target\.meta\.id,orientation\)/);
 assert.doesNotMatch(body,/detachTerminal|splitLeaf/);
 assert.match(source,/const extracted=extractLeaf\(sourceRoot,sourceID\)/);
});
