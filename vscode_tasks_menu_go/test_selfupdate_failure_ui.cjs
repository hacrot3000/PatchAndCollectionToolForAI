// Browser-side regression: successive failed updates cannot strand Close disabled.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const source=fs.readFileSync(path.join(__dirname,'web/featuremods/selfupdate.js'),'utf8');

function harness(){
 const nodes=[],intervals=[],acks=[],errors=[];
 let request={id:'first-failure',status:'failed',revision:'abc123',error:'go executable missing'};
 function element(tag){
  const classes=new Set();
  const node={
   tag,style:{},dataset:{},children:[],className:'',textContent:'',disabled:false,isConnected:true,
   classList:{
    add(...tokens){tokens.forEach(t=>classes.add(t));},
    remove(...tokens){tokens.forEach(t=>classes.delete(t));},
    contains(name){return classes.has(name);},
    toggle(name,enabled){if(enabled===undefined)enabled=!classes.has(name);if(enabled)classes.add(name);else classes.delete(name);}
   },
   append(...items){this.children.push(...items);},
   setAttribute(){},focus(){},select(){},remove(){}
  };
  nodes.push(node);
  return node;
 }
 const app={
  sharedMode:false,taskData:{workspace:'/workspace'},hasPermission:()=>true,
  jsonFetch:async(url,options)=>{
   if(url.includes('action=ack')){acks.push(JSON.parse(options.body).id);throw Error('temporary ACK failure');}
   if(url.includes('action=check'))return {available:false};
   return request;
  },
  showError:error=>errors.push(error)
 };
 const context={
  TaskMenuApp:app,
  TaskMenuTerminalRestore:{resumeAfterSelfUpdate(){}},
  document:{createElement:element,head:{append(){}},body:{append(){}},querySelector(){return {append(){}};}},
  window:{isSecureContext:false},
  location:{href:'https://localhost:4200/'},
  sessionStorage:{getItem:()=>'',setItem(){}},
  history:{replaceState(){}},
  navigator:{},URL,Set,Date,Promise,console,
  setTimeout:()=>0,
  setInterval:callback=>{intervals.push(callback);return 1;}
 };
 context.globalThis=context;
 vm.runInNewContext(source,context,{filename:'selfupdate.js',timeout:1000});
 return {
  async poll(){await intervals[0]();},
  fail(id){request={id,status:'failed',revision:'abc123',error:'go not found'};},
  close:nodes.find(node=>node.textContent==='Close'),
  overlay:nodes.find(node=>node.className==='self-update-overlay'),
  check:nodes.find(node=>node.id==='self-update-check'),
  acks,errors
 };
}

test('failed update closes immediately even if ACK fails; a second failure has enabled Close',async()=>{
 const h=harness();
 await h.poll();
 assert.equal(h.overlay.classList.contains('visible'),true);
 assert.equal(h.close.disabled,false);
 assert.equal(h.check.disabled,false,'Check update must not stay locked after failure');
 h.close.onclick();
 assert.equal(h.overlay.classList.contains('visible'),false,'Close cannot wait for network ACK');
 assert.equal(h.close.disabled,false);
 await Promise.resolve();await Promise.resolve();
 assert.deepEqual(h.acks,['first-failure']);
 assert.equal(h.errors.length,0);
 await h.poll();
 assert.equal(h.overlay.classList.contains('visible'),false,'stale failed response must not redisplay dismissed notice');
 h.fail('second-failure');
 await h.poll();
 assert.equal(h.overlay.classList.contains('visible'),true);
 assert.equal(h.close.disabled,false);
 h.close.onclick();
 assert.equal(h.overlay.classList.contains('visible'),false);
 assert.equal(h.close.disabled,false);
 assert.equal(h.check.disabled,false);
});
