const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const test=require('node:test');

const ai=fs.readFileSync(path.join(__dirname,'web/featuremods/aiassistant.js'),'utf8');
const plans=fs.readFileSync(path.join(__dirname,'web/featuremods/aiplans.js'),'utf8');
const slice=(source,start,end)=>{
  const from=source.indexOf(start),to=source.indexOf(end,from+start.length);
  assert.ok(from>=0&&to>from);
  return source.slice(from,to);
};
const parse=vm.runInNewContext(slice(plans,'function parseSteps(input){','async function savePlan(){')+';parseSteps',{},{timeout:1000});
const redact=vm.runInNewContext(slice(ai,'function redactPreview(v){','function previewContext(){')+';redactPreview',{},{timeout:1000});

test('Guided-plan parser accepts plain and fenced JSON proposals',()=>{
  const raw=JSON.stringify({steps:[{command:'pwd',reason:'Check directory',risk:'read-only'}]});
  assert.equal(parse(raw)[0].command,'pwd');
  assert.equal(parse(''+String.fromCharCode(96).repeat(3)+'json\n'+raw+'\n'+String.fromCharCode(96).repeat(3))[0].risk,'read-only');
});
test('Guided plan rejects excess, missing and unsupported risk shapes',()=>{
  assert.throws(()=>parse('{}'),/1/);
  assert.throws(()=>parse(JSON.stringify({steps:Array.from({length:9},()=>({command:'pwd'}))})),/1/);
  assert.throws(()=>parse(JSON.stringify({steps:[{command:''}]})),/requires a command/);
  assert.equal(parse(JSON.stringify({steps:[{command:'pwd',risk:'unknown'}]}))[0].risk,'high');
});
test('UI preview masks common bearer tokens and passwords before consent',()=>{
  const cleaned=redact('Authorization: Bearer abc123\npassword=hunter2\napi_key=secretvalue');
  for(const value of ['abc123','hunter2','secretvalue'])assert.ok(!cleaned.includes(value));
  assert.ok(cleaned.includes('[REDACTED]'));
});
test('Guided Plans never has an execution API path',()=>{
  assert.ok(!plans.includes('/api/sessions/'));
  assert.ok(!plans.includes('exec.Command'));
  assert.ok(plans.includes('execution disabled'));
  assert.ok(plans.includes('/api/ai/plans/decision'));
});
