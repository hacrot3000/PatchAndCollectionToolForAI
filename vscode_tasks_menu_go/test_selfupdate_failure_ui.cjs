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
  fail(id,error='go not found',error_code=''){request={id,status:'failed',revision:'abc123',error,error_code};},
  update(id,status,error='',error_code='',message=''){request={id,status,revision:'abc123',error,error_code,message};},
  close:nodes.find(node=>node.textContent==='Close'),
  overlay:nodes.find(node=>node.className==='self-update-overlay'),
  check:nodes.find(node=>node.id==='self-update-check'),
  title:nodes.find(node=>node.tag==='h3'),
  intro:nodes.find(node=>node.tag==='p'),
  goGuide:nodes.find(node=>node.className==='self-update-go-guide'),
  style:nodes.find(node=>node.tag==='style')?.textContent||'',
  copyError:nodes.find(node=>node.textContent==='Copy error details'),
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


test('missing Go is a distinct update error with installation instructions for the host OS',async()=>{
 const h=harness();
 h.fail('missing-go','go build failed: Go toolchain not found for self-update (daemon HOME="/home/user" PATH="/usr/bin")');
 await h.poll();
 assert.equal(h.overlay.classList.contains('visible'),true);
 assert.equal(h.goGuide.hidden,false);
 assert.match(h.title.textContent,/Thiếu Go/);
 assert.match(h.intro.textContent,/máy chủ/);
 assert.match(h.goGuide.children[0].textContent,/hệ điều hành/);
 const linux=h.goGuide.children[2],mac=h.goGuide.children[3];
 assert.equal(linux.tag,'details');
 assert.equal(mac.tag,'details');
 assert.match(linux.children[1].textContent,/sudo apt install -y golang-go/);
 assert.match(mac.children[1].textContent,/brew install go/);
 assert.match(h.goGuide.children[5].href,/^https:\/\/go\.dev\/doc\/install$/);
 assert.match(h.copyError.dataset.details,/Cài Go dùng chung cho hệ điều hành/);
 assert.match(h.copyError.dataset.details,/Go toolchain not found for self-update/);
 assert.equal(h.close.disabled,false);
});

test('legacy missing-go PATH failure and structured missing-go code show same OS-level guide',async()=>{
 const h=harness();
 h.fail('legacy','go build failed: exec: "go": executable file not found in $PATH');
 await h.poll();
 assert.equal(h.goGuide.hidden,false);
 h.fail('structured','Go unavailable in self-update', 'GO_TOOLCHAIN_MISSING');
 await h.poll();
 assert.equal(h.goGuide.hidden,false);
});

test('normal Go build/test failure is not mistaken for Go installation failure',async()=>{
 const h=harness();
 h.fail('compile','go build failed: undefined: missingSymbol');
 await h.poll();
 assert.equal(h.goGuide.hidden,true);
 assert.equal(h.title.textContent,'TaskDeck update');
 assert.match(h.intro.textContent,/Candidate validation or activation failed/);
 assert.doesNotMatch(h.copyError.dataset.details,/Cài Go dùng chung cho hệ điều hành/);
 h.fail('missing','Go toolchain not found for self-update');
 await h.poll();
 assert.equal(h.goGuide.hidden,false);
 h.fail('test','go test failed: undefined: missingSymbol');
 await h.poll();
 assert.equal(h.goGuide.hidden,true,'reused error dialog must clear the previous Go-specific guide');
});


test('Go installation guide is fully hidden during a successful dry-run, even with stale Go error metadata',async()=>{
 const h=harness();
 assert.equal(h.goGuide.hidden,true);
 assert.equal(h.goGuide.style.display,'none','CSS-independent initial visibility');
 assert.match(h.style,/\.self-update-go-guide\[hidden\]\{display:none!important\}/,
   'author stylesheet must not override the HTML hidden attribute');

 for(const phase of ['confirmed','downloading','testing','building','installing','ready_restart','restarting']){
  h.update('running-1',phase,
    'Go toolchain not found for self-update','GO_TOOLCHAIN_MISSING',
    'Dry-run: building and validating candidate binary…');
  await h.poll();
  assert.equal(h.goGuide.hidden,true,'guide hidden in '+phase);
  assert.equal(h.goGuide.style.display,'none','guide display:none in '+phase);
  assert.equal(h.title.textContent,'TaskDeck update','regular title in '+phase);
  assert.equal(h.overlay.classList.contains('visible'),true,'progress visible in '+phase);
  assert.equal(h.copyError.style.display,'none','no error copy action in '+phase);
 }
 h.update('running-1','completed','', 'GO_TOOLCHAIN_MISSING');
 await h.poll();
 assert.equal(h.goGuide.hidden,true,'success never shows Go-install instructions');
 assert.equal(h.goGuide.style.display,'none');
 assert.equal(h.title.textContent,'TaskDeck update');
});

test('a missing-Go failure displays the guide, then a new active update hides it again',async()=>{
 const h=harness();
 h.fail('old-failed','go build failed: exec: "go": executable file not found in $PATH');
 await h.poll();
 assert.equal(h.goGuide.hidden,false);
 assert.equal(h.goGuide.style.display,'');
 assert.match(h.title.textContent,/Thiếu Go/);

 h.update('new-running','building');
 await h.poll();
 assert.equal(h.goGuide.hidden,true);
 assert.equal(h.goGuide.style.display,'none','old error guide must not leak into a new update');
 assert.equal(h.title.textContent,'TaskDeck update');
 assert.equal(h.overlay.classList.contains('visible'),true);

 h.fail('different-failure','go build failed: undefined: symbol');
 await h.poll();
 assert.equal(h.goGuide.hidden,true);
 assert.equal(h.goGuide.style.display,'none');
 assert.equal(h.title.textContent,'TaskDeck update');

 h.fail('real-missing','Go toolchain not found for self-update');
 await h.poll();
 assert.equal(h.goGuide.hidden,false);
 assert.equal(h.goGuide.style.display,'');
 h.close.onclick();
 assert.equal(h.goGuide.hidden,true,'dismissal must hide the Go guide immediately');
 assert.equal(h.goGuide.style.display,'none');
});
