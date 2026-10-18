import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFileSync} from 'node:fs';

const source=readFileSync(new URL('../web/featuremods/schedulespanel.js',import.meta.url),'utf8');
const start=source.indexOf('const canScheduleElevated=');
const end=source.indexOf('const post=',start);
assert.ok(start>=0&&end>start,'Schedule permission helpers are present');
const evaluate=(permissions,{sharedMode=true}={})=>{
 const grants=new Set(permissions);
 const app={sharedMode,hasPermission:name=>grants.has(name)};
 return vm.runInNewContext(source.slice(start,end)+
  '\n({view:canScheduleView(),run:canScheduleRun(),manage:canScheduleManage()})',{app});
};

test('Schedule UI requires both privileged grants for all mutation buttons',()=>{
 for(const grants of [
  [],['schedules.view'],['schedules.run'],['schedules.manage'],
  ['settings.write'],['tasks.run'],
 ]){
  const state=evaluate(grants);
  assert.equal(state.run,false,`run button exposed for ${grants}`);
  assert.equal(state.manage,false,`manage button exposed for ${grants}`);
 }
 for(const grants of [
  ['project.admin'],['schedules.run','schedules.manage'],
 ]){
  const state=evaluate(grants);
  assert.equal(state.run,true,`run button hidden for ${grants}`);
  assert.equal(state.manage,true,`manage button hidden for ${grants}`);
 }
});

test('Schedule viewers retain read-only navigation; single-user unchanged',()=>{
 const viewer=evaluate(['schedules.view']);
 assert.equal(viewer.view,true);
 assert.equal(viewer.run,false);
 assert.equal(viewer.manage,false);
 const single=evaluate([],{sharedMode:false});
 assert.equal(single.view,true);
 assert.equal(single.run,true);
 assert.equal(single.manage,true);
});
