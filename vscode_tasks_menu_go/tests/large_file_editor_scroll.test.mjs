import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {webcrypto} from 'node:crypto';
import vm from 'node:vm';

const implementation=readFileSync(new URL('../web/featuremods/largefileeditor.js',import.meta.url),'utf8');
function fixture(total=5000,{linePadding=0,secure=true}={}){
 const padding='x'.repeat(linePadding);
 const lines=Array.from({length:total},(_,i)=>'row '+(i+1)+padding+'\n');
 const bytes=[0];for(const line of lines)bytes.push(bytes.at(-1)+new TextEncoder().encode(line).length);
 let text='';
 const doc={
  get length(){return text.length;},
  get lines(){return text.split('\n').length;},
  toString(){return text;},
  line(n){
   const all=text.split('\n');n=Math.max(1,Math.min(all.length,n));
   let from=0;for(let i=1;i<n;i++)from+=all[i-1].length+1;
   return {number:n,from,text:all[n-1],to:from+all[n-1].length};
  },
  lineAt(p){
   const all=text.split('\n');let from=0;
   for(let i=0;i<all.length;i++){
    if(p<=from+all[i].length)return {number:i+1,from,text:all[i]};
    from+=all[i].length+1;
   }
   return {number:all.length,from,text:all.at(-1)};
  }
 };
 function node(){
  return {style:{},dataset:{},listeners:{},classList:{toggle(){}},append(){},before(){},remove(){},setAttribute(){},
   addEventListener(type,fn){this.listeners[type]=fn},value:'',textContent:'',hidden:false};
 }
 const gutterNumbers=[node(),node(),node()];
 gutterNumbers.forEach((g,i)=>g.textContent=String(i+1));
 const gutter={style:{},querySelectorAll:()=>gutterNumbers};
 const scroller=node();scroller.scrollTop=0;scroller.clientHeight=400;
 Object.defineProperty(scroller,'scrollHeight',{get(){return doc.lines*20}});
 scroller.getBoundingClientRect=()=>({left:0,top:0,width:900,height:400});
 const cm={
  state:{doc,selection:{main:{head:0}}},dom:{querySelector:()=>gutter},contentDOM:node(),
  posAtCoords(pt){
   return doc.line(Math.max(1,Math.min(doc.lines,Math.floor((scroller.scrollTop+pt.y)/20)+1))).from;
  },
  coordsAtPos(pos){return {top:(doc.lineAt(pos).number-1)*20-scroller.scrollTop}},
  dispatch(spec){
   if(spec.changes){
    const changes=(Array.isArray(spec.changes)?spec.changes:[spec.changes]).slice().sort((a,b)=>b.from-a.from);
    for(const change of changes)text=text.slice(0,change.from)+String(change.insert||'')+text.slice(change.to);
   }
   if(spec.selection){
    this.state.selection.main.head=spec.selection.anchor;
    if(spec.scrollIntoView)scroller.scrollTop=(doc.lineAt(spec.selection.anchor).number-1)*20;
   }
  }
 };
 const view={file:{path:'very-large.txt',streamed:true,read_only:false},cm,scroller,body:node(),
  history:node(),outline:node(),markdownPreview:node(),structuredSelect:node(),
  lineEndingSelect:node(),encodingSelect:node(),minimapToggle:node(),largeFileBadge:node(),
  dirty:false,closed:false};
 const requests=[],saves=[],frames=[];
 let held=null;
 const app={
  taskData:{workspace:'test'},currentUser:{user_id:'tester'},
  async jsonFetch(url){
   requests.push(url);
   const p=new URL('https://example.invalid'+url).searchParams;
   if(url.includes('/search'))return {found:true,line:2345,column:1};
   const start=Number(p.get('start')),count=Number(p.get('count'));
   const end=Math.min(total,start+count-1);
   const response={
    streamed:true,start_line:start,end_line:end,total_lines:total,content:lines.slice(start-1,end).join(''),
    start_byte:bytes[start-1],end_byte:bytes[end],size:bytes[total],mtime_ns:1,
    sha256:'a'.repeat(64),window_sha256:'b'.repeat(64),line_ending:'lf',read_only:false
   };
   if(held){const resolver=held;held=null;return new Promise(resolve=>{resolver.resolve=()=>resolve(response);});}
   return response;
  },
  async fetchWithLease(_url,request){
   saves.push(JSON.parse(request.body));
   return {ok:true,json:async()=>({history_warning:'Saved'})};
  },
  showError(error){throw error}
 };
 const world={TaskMenuApp:app,crypto:secure?webcrypto:null,requestAnimationFrame(fn){frames.push(fn);return frames.length},
  cancelAnimationFrame(){}};
 const store=new Map();
 const context={document:{createElement:node,head:{append(){}}},URLSearchParams,TextEncoder,
  sessionStorage:{getItem:key=>store.get(key)||null,setItem:(k,v)=>store.set(k,v),removeItem:k=>store.delete(k)},
  setTimeout,clearTimeout,MutationObserver:class{observe(){}disconnect(){}},globalThis:world,console};
 vm.runInNewContext(implementation,context,{filename:'largefileeditor.js'});
 const api=world.TaskMenuLargeEditor;
 const helpers={setDirty(v,value){v.dirty=Boolean(value)},applyReadOnly(){},dirtyCloseChoice:async()=> 'discard'};
 const flush=()=>{for(let i=0;frames.length&&i<10;i++)frames.shift()();};
 const scroll=async(top)=>{
  scroller.scrollTop=top;const before=api.visibleLine(view);
  scroller.listeners.scroll();
  const pending=api.stateFor(view).prefetch;
  if(pending)await pending;
  flush();
  return {before,after:api.visibleLine(view)};
 };
 return {api,view,doc,scroller,app,requests,saves,flush,scroll,gutterNumbers,
  hold(){const gate={resolve:null};held=gate;return ()=>gate.resolve?.()},
  get text(){return text}};
}

test('absolute gutter numbers reflect the current physical window after a distant jump',async()=>{
 const f=fixture();f.api.attach(f.view,{setDirty(v,d){v.dirty=d},applyReadOnly(){},dirtyCloseChoice:async()=> 'discard'});
 await f.api.goTo(f.view,3000);f.flush();
 const w=f.api.stateFor(f.view).window;
 assert.ok(w.start_line>2800);
 assert.equal(f.gutterNumbers[0].textContent,String(w.start_line));
 assert.equal(f.gutterNumbers[1].textContent,String(w.start_line+1));
});

test('sequential downward scroll refills before edge and never jumps over lines',async()=>{
 const f=fixture();
 f.api.attach(f.view,{setDirty(v,d){v.dirty=d},applyReadOnly(){},dirtyCloseChoice:async()=> 'discard'});
 await f.api.goTo(f.view,1,{force:true});f.flush();
 for(let i=0;i<9;i++){
  const result=await f.scroll(f.scroller.scrollHeight-f.scroller.clientHeight-20);
  assert.equal(result.after,result.before,'buffer replacement may not alter visible line');
 }
 const w=f.api.stateFor(f.view).window;
 assert.ok(w.start_line>500,'old out-of-view lines must be trimmed');
 assert.ok(f.doc.lines<750,'in-memory buffer must remain bounded');
 const before=f.api.visibleLine(f.view);
 f.scroller.scrollTop+=20;
 assert.equal(f.api.visibleLine(f.view),before+1,'scrolling one row should reveal X+1');
});

test('upward refill keeps the same top line and restores preceding rows',async()=>{
 const f=fixture();
 f.api.attach(f.view,{setDirty(v,d){v.dirty=d},applyReadOnly(){},dirtyCloseChoice:async()=> 'discard'});
 await f.api.goTo(f.view,1,{force:true});f.flush();
 for(let i=0;i<8;i++)await f.scroll(f.scroller.scrollHeight-f.scroller.clientHeight-20);
 const initial=f.api.stateFor(f.view).window.start_line;
 for(let i=0;i<3;i++){
  const result=await f.scroll(10);
  assert.equal(result.after,result.before,'prepending older lines must preserve the visual anchor');
 }
 assert.ok(f.api.stateFor(f.view).window.start_line<initial);
});

test('dirty window stops automatic refill; edits remain available',async()=>{
 const f=fixture();
 f.api.attach(f.view,{setDirty(v,d){v.dirty=d},applyReadOnly(){},dirtyCloseChoice:async()=> 'discard'});
 await f.api.goTo(f.view,1,{force:true});f.flush();
 for(let i=0;i<5;i++)await f.scroll(f.scroller.scrollHeight-f.scroller.clientHeight-20);
 const state=f.api.stateFor(f.view);
 assert.ok(state.window.start_line>1);
 const original=state.cleanContent;
 const expectedStartByte=state.window.start_byte;
 f.view.cm.dispatch({changes:{from:0,to:3,insert:'EDIT'}});
 f.view.dirty=true;
 const count=f.requests.length;
 await f.scroll(f.scroller.scrollHeight-f.scroller.clientHeight-2);
 assert.equal(f.requests.length,count,'background refill should not discard dirty content');
 await f.api.save(f.view);
 assert.equal(f.saves.length,1);
 const digest=await webcrypto.subtle.digest('SHA-256',new TextEncoder().encode(original));
 assert.equal(f.saves[0].expected_window_sha256,Buffer.from(digest).toString('hex'));
 assert.equal(f.saves[0].start_byte,expectedStartByte);
});

test('late adjacency response cannot replace a distant jump or reset the scrollbar',async()=>{
 const f=fixture();
 f.api.attach(f.view,{setDirty(v,d){v.dirty=d},applyReadOnly(){},dirtyCloseChoice:async()=> 'discard'});
 await f.api.goTo(f.view,1,{force:true});f.flush();
 const release=f.hold();
 f.scroller.scrollTop=f.scroller.scrollHeight-f.scroller.clientHeight-12;
 f.scroller.listeners.scroll();
 const pending=f.api.stateFor(f.view).prefetch;
 assert.ok(pending,'an adjacent window request must be in flight');
 await f.api.goTo(f.view,3500);
 f.flush();
 const afterJump=f.api.stateFor(f.view).window.start_line;
 assert.ok(afterJump>3300);
 release();
 await pending;f.flush();
 assert.equal(f.api.stateFor(f.view).window.start_line,afterJump);
 assert.ok(f.api.visibleLine(f.view)>3300);
});

test('HTTP without SubtleCrypto refetches exactly the clean range before Save',async()=>{
 const f=fixture(5000,{secure:false});
 f.api.attach(f.view,{setDirty(v,d){v.dirty=d},applyReadOnly(){},dirtyCloseChoice:async()=> 'discard'});
 await f.api.goTo(f.view,1,{force:true});f.flush();
 for(let i=0;i<5;i++)await f.scroll(f.scroller.scrollHeight-f.scroller.clientHeight-20);
 const state=f.api.stateFor(f.view);
 assert.ok(state.window.start_line>1);
 f.view.cm.dispatch({changes:{from:0,to:3,insert:'NEW'}});
 f.view.dirty=true;
 const before=f.requests.length;
 await f.api.save(f.view);
 assert.ok(f.requests.length>=before+2,'one exact-range checksum fetch plus refresh');
 assert.equal(f.saves[0].expected_window_sha256,'b'.repeat(64));
});

test('long lines trigger byte-aware pruning without losing the visible anchor',async()=>{
 const f=fixture(5000,{linePadding:3000});
 f.api.attach(f.view,{setDirty(v,d){v.dirty=d},applyReadOnly(){},dirtyCloseChoice:async()=> 'discard'});
 await f.api.goTo(f.view,1,{force:true});f.flush();
 for(let i=0;i<8;i++){
  const {before,after}=await f.scroll(f.scroller.scrollHeight-f.scroller.clientHeight-20);
  assert.equal(after,before);
  assert.ok(new TextEncoder().encode(f.text).length<=2<<20,'buffer must respect 2 MiB cap');
 }
 assert.ok(f.api.stateFor(f.view).window.start_line>1);
});
