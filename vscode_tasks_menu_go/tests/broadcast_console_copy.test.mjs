import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const source=readFileSync(new URL('../web/featuremods/menus.js',import.meta.url),'utf8');
const start=source.indexOf('function broadcastConsoleGroup(view){');
const end=source.indexOf('function broadcastCopyFeedback(',start);
assert.ok(start>=0&&end>start,'missing group console collector');
const collector=source.slice(start,end);

function fixture(){
  const views=new Map();
  const groups=new Map([['A',{id:'A',name:'Workers'}],['B',{id:'B',name:'Database'}]]);
  const assignments={one:'A',two:'A',other:'B'};
  const add=(id,label,output,closed=false)=>{
    const view={meta:{id,label},term:{},closed,output};
    views.set(id,view);return view;
  };
  const one=add('one','Build','line 1\nline 2');
  const two=add('two','Test','test result');
  add('other','Secret','not part of Workers');
  const sandbox={
    app:{sharedMode:false,views,consoleText:member=>member.output},
    TaskMenuBroadcast:{state:{assignments},groupForSession:id=>groups.get(assignments[id])||null}
  };
  vm.runInNewContext(collector+'\nthis.collect=broadcastGroupConsolePayload;',sandbox);
  return {one,two,views,assignments,sandbox,collect:sandbox.collect};
}

test('copies all and only terminals assigned to the source broadcast group',()=>{
  const {one,collect}=fixture();
  const payload=collect(one);
  assert.equal(payload.count,2);
  assert.match(payload.text,/Broadcast group: Workers/);
  assert.match(payload.text,/===== Build \[one\] =====\nline 1\nline 2/);
  assert.match(payload.text,/===== Test \[two\] =====\ntest result/);
  assert.ok(payload.text.indexOf('Build [one]')<payload.text.indexOf('Test [two]'));
  assert.ok(!payload.text.includes('Secret'));
  assert.ok(!payload.text.includes('not part of Workers'));
});

test('does not combine unassigned terminals or other broadcast groups',()=>{
  const {one,views,assignments,collect}=fixture();
  assignments.one='';
  assert.equal(collect(one),null);
  assignments.one='A';
  assignments.two='B';
  const payload=collect(one);
  assert.equal(payload.count,1);
  assert.ok(!payload.text.includes('test result'));
  views.get('other').closed=true;
  assert.equal(collect(views.get('other')),null);
});

test('handles empty and closed consoles without leaking unrelated output',()=>{
  const {one,two,collect}=fixture();
  two.closed=true;
  one.output='';
  assert.equal(collect(one).count,1);
  assert.equal(collect(one).text,'');
  one.output='active';
  assert.ok(collect(one).text.includes('active'));
  assert.ok(!collect(one).text.includes('test result'));
});

test('sanitizes terminal and group headings without changing console text',()=>{
  const {one,collect,sandbox}=fixture();
  one.meta.label='Build\nspoof';
  sandbox.TaskMenuBroadcast.groupForSession=()=>({id:'A',name:'Workers\nspoof'});
  const payload=collect(one);
  assert.match(payload.text,/Broadcast group: Workers spoof/);
  assert.match(payload.text,/===== Build spoof \[one\] =====/);
});
