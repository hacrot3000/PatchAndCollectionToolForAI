import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source=readFileSync(new URL('../web/featuremods/filecompare.js',import.meta.url),'utf8');
function section(startMarker,endMarker){
  const start=source.indexOf(startMarker),end=source.indexOf(endMarker,start+startMarker.length);
  assert.ok(start>=0&&end>start,'missing file compare section '+startMarker);
  return source.slice(start,end);
}
const preferences=section('const compareViewPreferenceKey=','let compareSelection=null;');
const visible=section('const compareContextLimit=10;','function appendCompareGap(');
function browser(storage){
  const context=vm.createContext({localStorage:storage,console:{warn(){}}});
  vm.runInContext(preferences,context);
  vm.runInContext(visible,context);
  context.compareStructuralContextRange=()=>[-1,-1];
  const get=()=>vm.runInContext('({viewMode,contentMode,showUnimportant})',context);
  return {
    get,
    select(mode,showMinor){context.mode=mode;context.minor=showMinor;vm.runInContext('contentMode=mode;showUnimportant=minor;saveCompareViewPreferences()',context);},
    layout(value){context.layout=value;vm.runInContext('viewMode=layout;saveCompareViewPreferences()',context);},
    indexes(rows){context.model={rows};return Array.from(vm.runInContext('compareVisibleIndexes(model)',context));}
  };
}
const fixture=[
  {hunk:-1},{hunk:0,importance:'important'},
  {hunk:0,importance:'unimportant'},{hunk:-1},
  {hunk:1,importance:'unimportant'},{hunk:-1},
  {hunk:2,importance:'important'},{hunk:-1}
];
function memoryStorage(){
  const data=new Map();
  return {getItem:key=>data.get(key)??null,setItem:(key,value)=>data.set(key,value)};
}
test('defaults to diff context before a browser preference exists',()=>{
  const b=browser(memoryStorage());
  assert.deepEqual({...b.get()},{viewMode:'side',contentMode:'context',showUnimportant:true});
});
test('persists diff mode, independent minor-edit toggle and layout in this browser',()=>{
  const storage=memoryStorage(),first=browser(storage);
  first.select('diff',false);first.layout('inline');
  const reopened=browser(storage);
  assert.deepEqual({...reopened.get()},{viewMode:'inline',contentMode:'diff',showUnimportant:false});
  assert.deepEqual(reopened.indexes(fixture),[1,6]);
  reopened.select('context',false);
  const switched=browser(storage);
  assert.equal(switched.get().showUnimportant,false);
  assert.equal(switched.get().contentMode,'context');
  assert.deepEqual(switched.indexes(fixture),[0,1,3,5,6,7]);
});
test('View all includes every line even when unimportant is off',()=>{
  const b=browser(memoryStorage());
  b.select('all',false);
  assert.deepEqual(b.indexes(fixture),[0,1,2,3,4,5,6,7]);
});
test('diff and context independently include unimportant edits when enabled',()=>{
  const b=browser(memoryStorage());
  b.select('diff',true);
  assert.deepEqual(b.indexes(fixture),[1,2,4,6]);
  b.select('context',true);
  assert.deepEqual(b.indexes(fixture),[0,1,2,3,4,5,6,7]);
  b.select('diff',false);
  assert.deepEqual(b.indexes(fixture),[1,6]);
});
test('comment-only diff can be suppressed without leaking surrounding context',()=>{
  const b=browser(memoryStorage()),commentOnly=[{hunk:-1},{hunk:0,importance:'unimportant'},{hunk:-1}];
  b.select('diff',false);assert.deepEqual(b.indexes(commentOnly),[]);
  b.select('context',false);assert.deepEqual(b.indexes(commentOnly),[]);
  b.select('context',true);assert.deepEqual(b.indexes(commentOnly),[0,1,2]);
});
test('corrupted and unavailable browser storage fall back safely',()=>{
  const bad={getItem:()=>'{broken json',setItem:()=>{throw Error('disabled')}};
  const b=browser(bad);
  assert.equal(b.get().contentMode,'context');
  assert.doesNotThrow(()=>b.select('diff',false));
  assert.equal(b.get().showUnimportant,false);
});
test('old workspace snapshots cannot override browser preferences',()=>{
  const restore=section('async function restoreCompareState(saved){','globalThis.TaskMenuFileCompare=');
  assert.doesNotMatch(restore,/contentMode\s*=\s*saved\./);
  assert.doesNotMatch(restore,/showUnimportant\s*=\s*saved\./);
  assert.doesNotMatch(restore,/viewMode\s*=\s*saved\./);
});
