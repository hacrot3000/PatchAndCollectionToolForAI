// Regression contract for the Semantic Browser setup wizard. No browser or
// network service required: exercise the real wizard closure in a tiny DOM.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

const source=fs.readFileSync(path.join(__dirname,'web/featuremods/semantic_browser.js'),'utf8');
const start=source.indexOf('function openSetupGuide(){');
const finish=source.indexOf('function openSettings(){',start);
assert.ok(start>=0&&finish>start,'setup wizard source is available');
const wizard=source.slice(start,finish);

function harness(initialJob={}){
 const listeners=[];
 const calls=[];
 let job={state:'',stage:'',message:'',logs:[],active:false,...initialJob};
 const data={
   platform:'linux',go_os:'linux',node_supported:true,node_version:'v20.19.5',
   npm_installed:true,npm_version:'10.8',docker_installed:false,
   worker_dir:'/private/browser-worker',playwright_installed:true,
   worker_connected:false,configured:true,source:'managed',
   allowed_hosts:['example.com'],commands:[
     {label:'Open worker directory',command:"cd '/private/browser-worker'"},
     {label:'Install Chromium',command:'npx playwright install chromium'}
   ]
 };
 function element(tag,cls='',text=''){
  const handlers=new Map();
  const classes=new Set(String(cls).split(' ').filter(Boolean));
  let value=String(text||'');
  const node={
   tagName:tag,className:cls,children:[],attributes:{},isConnected:true,
   scrollTop:0,clientHeight:55,tabIndex:-1,disabled:false,onclick:null,
   classList:{
    add(x){classes.add(x);},remove(x){classes.delete(x);},
    toggle(x,on){if(on===undefined? !classes.has(x):on){classes.add(x);return true;}classes.delete(x);return false;},
    contains(x){return classes.has(x);}
   },
   get textContent(){return value;},
   set textContent(next){value=String(next);},
   get scrollHeight(){return this.textContent.length*3+20;},
   append(...children){this.children.push(...children);},
   replaceChildren(...children){this.children=[...children];},
   setAttribute(k,v){this.attributes[k]=v;},
   getAttribute(k){return this.attributes[k];},
   addEventListener(k,fn){handlers.set(k,fn);},
   fire(k){return handlers.get(k)?.();},
   querySelector(selector){
    const wanted=selector.startsWith('.')?selector.slice(1):selector;
    return all(this).find(e=>e.classList?.contains(wanted))||null;
   },
  };
  return node;
 }
 function all(node){return [node,...node.children.flatMap(x=>x&&typeof x==='object'?all(x):[])];}
 const el=(...args)=>element(...args);
 const button=(name,action,cls='')=>{const item=el('button',cls,name);item.onclick=action;return item;};
 const addBrowserInfoLine=(parent,message)=>{
  const item=el('p','',message);parent.append(item);return item;
 };
 const app={
  async jsonFetch(url,options={}){
   calls.push({url,options});
   if(url==='/api/semantic-browser/setup/run'){
    if(options.method==='POST'){
     const req=JSON.parse(options.body);
     if(req.action==='cancel'){
      job={...job,state:'manual',active:false,stage:'cancelled',
        message:'Stopped',manual_command:'cd /private/browser-worker && npm install'};
     }
    }
    return structuredClone(job);
   }
   if(url==='/api/semantic-browser/setup')return structuredClone(data);
   throw new Error('unexpected API '+url);
  }
 };
 let dialog;
 const context={
  el,button,addBrowserInfoLine,app,navigator:{clipboard:{writeText:async()=>{}}},
  setInterval(fn){listeners.push(fn);return 7;},clearInterval(){},
  errorOf:e=>String(e?.message||e),
  browserInfoDialog(_title,build){dialog=el('dialog');build(dialog);return dialog;},
 };
 context.globalThis=context;
 vm.createContext(context);
 vm.runInContext(wizard+';globalThis.openSetupGuide=openSetupGuide;',context);
 context.openSetupGuide();
 const lookup=text=>all(dialog).find(e=>e.tagName==='button'&&e.textContent===text);
 const selectStep=async()=>{await flush();const button=lookup('3 · Cài worker');assert.ok(button);button.onclick();};
 const flush=async()=>{for(let i=0;i<8;i++)await Promise.resolve();};
 const poll=async()=>{assert.equal(listeners.length,1);await listeners[0]();await flush();};
 return {dialog,all:()=>all(dialog),calls,lookup,selectStep,flush,poll,
  setJob(x){job={...job,...x};},
  logs:()=>dialog.querySelector('.semantic-browser-installer-logs'),
  getJob:()=>job};
}

test('step 3 defaults to Automatic; manual instructions do not cover console',async()=>{
 const h=harness();await h.selectStep();
 const automatic=h.lookup('Tự động');
 const manual=h.lookup('Thủ công');
 assert.ok(automatic&&manual,'both install modes are visible');
 assert.equal(automatic.getAttribute('aria-selected'),'true');
 assert.equal(manual.getAttribute('aria-selected'),'false');
 assert.ok(h.logs(),'console is visible without scrolling past command list');
 assert.ok(h.lookup('Cài đặt tự động'));
 assert.equal(h.all().some(n=>n.textContent==='Install Chromium'),false,
  'manual step commands must be kept in Manual tab');
});

test('console follows new output and does not rebuild UI for log-only updates',async()=>{
 const h=harness({state:'running',active:true,stage:'npm install',message:'running',
   logs:['first','second','third']});
 await h.selectStep();
 const view=h.logs();
 assert.ok(view);
 assert.equal(view.scrollTop,view.scrollHeight,'initial console follows tail');
 h.setJob({logs:['first','second','third','step completed']});
 await h.poll();
 assert.strictEqual(h.logs(),view,'log-only poll keeps same console and inputs');
 assert.match(view.textContent,/step completed/);
 assert.equal(view.scrollTop,view.scrollHeight,'new output auto-scrolls to bottom');
});

test('user scrolls up: new logs preserve scroll; follow button resumes tail',async()=>{
 const h=harness({state:'running',active:true,stage:'download',message:'working',
   logs:Array.from({length:35},(_,i)=>'download chunk '+i)});
 await h.selectStep();
 const view=h.logs();
 view.scrollTop=0;
 view.fire('scroll');
 h.setJob({logs:[...h.getJob().logs,'another chunk']});
 await h.poll();
 assert.strictEqual(h.logs(),view);
 assert.equal(view.scrollTop,0,'reading old log should not be interrupted');
 const follow=h.lookup('Theo dõi log: Tắt');
 assert.ok(follow,'manual-follow control should reflect disabled auto-scroll');
 follow.onclick();
 assert.equal(h.logs().scrollTop,h.logs().scrollHeight);
});

test('switching to Manual cancels active auto installation',async()=>{
 const h=harness({state:'running',active:true,stage:'install Chromium',message:'working',
   logs:['installing']});
 await h.selectStep();
 await h.lookup('Thủ công').onclick();
 assert.ok(h.calls.some(c=>c.url==='/api/semantic-browser/setup/run'&&
   c.options.method==='POST'&&JSON.parse(c.options.body).action==='cancel'),
   'switching mode must request cancellation');
 assert.equal(h.lookup('Thủ công').getAttribute('aria-selected'),'true');
 assert.ok(h.all().some(e=>e.textContent==='Install Chromium'),
   'manual mode must expose copyable commands');
 assert.ok(h.all().some(e=>e.textContent==='Lệnh tiếp tục tại bước vừa dừng'),
   'manual continuation is first');
});

test('failed automatic installation selects Manual tab and preserves error command',async()=>{
 const h=harness({state:'running',active:true,stage:'npm install',message:'working',logs:[]});
 await h.selectStep();
 h.setJob({state:'manual',active:false,stage:'failed',message:'npm failed',
   manual_command:"cd '/private/browser-worker' && npm install"});
 await h.poll();
 assert.equal(h.lookup('Thủ công').getAttribute('aria-selected'),'true');
 assert.ok(h.all().some(e=>e.textContent==='Lệnh tiếp tục tại bước vừa dừng'));
});
