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

test('Terminal right-click menu stacks actions and stages the exact selected snapshot',async()=>{
 const code=fs.readFileSync(path.join(__dirname,'web/featuremods/ai_terminal_troubleshoot.js'),'utf8');
 assert.match(code,/\.ai-terminal-menu button\{display:block;box-sizing:border-box;width:100%;text-align:left/);
 const stageCalls=[],copyCalls=[],views=new Map();
 const term={selection:'  Xin chào\n \terror  ',element:{contains:()=>true},getSelection(){return this.selection;}};
 const view={meta:{id:'terminal-007'},term};
 views.set('terminal-007',view);
 const classes=new Set();
 const menu={classList:{add:c=>classes.add(c)},style:{},offsetWidth:245,offsetHeight:140};
 const btn=()=>({disabled:false,onclick:null}),ask=btn(),copy=btn(),add=btn();
 const stage={
  captureSelectionContext:()=>({position:{selection_start:{buffer_row:17,column:3}},
    metadata:{terminal_id:'terminal-007',terminal_title:'Bash',cwd:'/project',captured_at:'now'}}),
  attachSelection:async(...args)=>{stageCalls.push(args);}
 };
 const globals={
  TaskMenuTerminalChatContext:stage,
  navigator:{clipboard:{writeText:async text=>copyCalls.push(text)}}
 };
 const sandbox={
  app:{views,showError(error){throw error;}},selectionCache:new WeakMap(),
  hideMenu(){},ctxmenu:menu,askSelection:ask,copySelection:copy,addSelection:add,
  open:()=>true,window:{innerWidth:900,innerHeight:600},Date,Promise,globalThis:globals
 };
 const src=slice(code,'const docContext=e=>{',"document.addEventListener('contextmenu',docContext,true);");
 const fn=vm.runInNewContext(src+';docContext',sandbox);
 const e={target:{closest:()=>true},clientX:500,clientY:300,
  preventDefault(){this.prevented=true;},stopPropagation(){this.stopped=true;}};
 fn(e);
 assert.equal(e.prevented,true);
 assert.equal(add.disabled,false);
 assert.equal(typeof add.onclick,'function');
 assert.equal(classes.has('visible'),true);
 term.selection='unrelated new selection';
 add.onclick();
 await new Promise(resolve=>setImmediate(resolve));
 assert.equal(stageCalls.length,1);
 assert.equal(stageCalls[0][0],view);
 assert.equal(stageCalls[0][1],'  Xin chào\n \terror  ','must stage the original selection without trimming');
 assert.equal(stageCalls[0][3].terminal_id,'terminal-007');
 assert.equal(stageCalls[0][2].selection_start.buffer_row,17);
 copy.onclick();
 await new Promise(resolve=>setImmediate(resolve));
 assert.deepEqual(copyCalls,['  Xin chào\n \terror  ']);
});
