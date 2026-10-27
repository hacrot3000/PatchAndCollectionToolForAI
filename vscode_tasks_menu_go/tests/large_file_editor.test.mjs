import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const code=readFileSync(new URL('../web/featuremods/largefileeditor.js',import.meta.url),'utf8');
const editor=readFileSync(new URL('../web/featuremods/editor.js',import.meta.url),'utf8');
const next=readFileSync(new URL('../web/featuremods/next.js',import.meta.url),'utf8');

function documentModel(text){
 const lines=()=>text.split('\n');
 return {
  get length(){return text.length},
  get lines(){return lines().length},
  toString(){return text},
  line(number){
   const items=lines();
   number=Math.max(1,Math.min(items.length,number));
   let from=0;for(let n=0;n<number-1;n++)from+=items[n].length+1;
   return {number,text:items[number-1],from,to:from+items[number-1].length};
  },
  lineAt(offset){
   const items=lines();let from=0;
   for(let n=0;n<items.length;n++){
    if(offset<=from+items[n].length)return {number:n+1,text:items[n],from};
    from+=items[n].length+1;
   }
   return {number:items.length,text:items.at(-1),from};
  },
  set(next){text=next}
 };
}
function fixture(){
 const nodes=[];
 const node=()=>{
  const classes=new Set();
  const value={classList:{toggle:(name,enabled)=>enabled?classes.add(name):classes.delete(name)},
   append(){},before(){},remove(){},addEventListener(){},value:'',textContent:'',hidden:false,
   disabled:false,title:'',setAttribute(){}};
  nodes.push(value);return value;
 };
 const docs=documentModel('');
 const cm={state:{doc:docs,selection:{main:{head:0}}},contentDOM:node(),
  dispatch(spec){
   const change=spec.changes;
   if(change){
    const old=docs.toString();
    docs.set(old.slice(0,change.from)+String(change.insert||'')+old.slice(change.to));
   }
   if(spec.selection)cm.state.selection.main={head:spec.selection.anchor};
  }
 };
 const view={file:{path:'large.txt',streamed:true,large_file:true,read_only:false},
  cm,body:node(),scroller:node(),history:node(),outline:node(),
  markdownPreview:node(),structuredSelect:node(),lineEndingSelect:node(),encodingSelect:node(),
  minimapToggle:node(),largeFileBadge:node(),closed:false,internalUpdate:false,dirty:false};
 const storage=new Map(),requests=[],saves=[];
 const response=(start,source='original')=>{
  const first=Math.max(1,start),last=first+149;
  const content=Array.from({length:150},(_,i)=>source+' '+(first+i)+'\n').join('');
  return {streamed:true,start_line:first,end_line:last,total_lines:500000,
   start_byte:first*20,end_byte:first*20+content.length,size:15000000,mtime_ns:1,
   content,line_ending:'lf',sha256:source==='original'?'a'.repeat(64):'b'.repeat(64),
   window_sha256:'c'.repeat(64),read_only:false};
 };
 let changed=false;
 const app={
  taskData:{workspace:'test'},currentUser:{user_id:'1'},
  jsonFetch:async url=>{requests.push(url);const params=new URL('https://example.invalid'+url).searchParams;
   if(url.includes('/search'))return {found:true,line:2345,column:5};
   return response(Number(params.get('start')),changed?'updated':'original');
  },
  fetchWithLease:async(url,init)=>{
   saves.push(JSON.parse(init.body));changed=true;
   return {ok:true,json:async()=>({history_warning:'Range saved'})};
  },showError:error=>{throw error}
 };
 const doc={createElement:node,head:{append(){}}};
 const context={document:doc,URLSearchParams,URL,TextEncoder,sessionStorage:{
  getItem:key=>storage.get(key)||null,setItem:(key,value)=>storage.set(key,value),removeItem:key=>storage.delete(key)
 },setTimeout,clearTimeout,console,TaskMenuApp:app,globalThis:null};
 context.globalThis=context;
 vm.runInNewContext(code,context,{filename:'largefileeditor.js'});
 const helpers={
  applyReadOnly(){},
  setDirty(v,dirty){v.dirty=Boolean(dirty)},
  dirtyCloseChoice:async()=> 'save'
 };
 return {view,helpers,api:context.TaskMenuLargeEditor,requests,saves,storage,app};
}
test('virtual editor is imported before the conventional editor',()=>{
 assert.ok(next.indexOf("import '/featuremods/largefileeditor.js'")>=0);
 assert.ok(next.indexOf("import '/featuremods/largefileeditor.js'")<next.indexOf("import '/featuremods/editor.js'"));
 assert.ok(editor.includes('if(file.streamed)globalThis.TaskMenuLargeEditor?.attach(view,{setDirty,applyReadOnly,dirtyCloseChoice});'));
 assert.ok(editor.includes('return await globalThis.TaskMenuLargeEditor.save(view)'));
 assert.ok(editor.includes('content:view.dirty&&!view.file?.streamed?'));
});
test('initial edit loads only a window; a distant jump fetches another',async()=>{
 const f=fixture();
 f.api.attach(f.view,f.helpers);
 await f.api.goTo(f.view,1,{force:true});
 assert.ok(f.view.cm.state.doc.length<10000);
 assert.ok(f.view.cm.state.doc.toString().includes('original 1\n'));
 await f.api.goTo(f.view,20000);
 assert.ok(f.requests.length>=2);
 assert.ok(f.view.cm.state.doc.toString().includes('original 20000'));
 assert.equal(f.api.stateFor(f.view).total,500000);
});
test('dirty window is saved as a byte range before jumping; no whole-file payload',async()=>{
 const f=fixture();
 f.api.attach(f.view,f.helpers);
 await f.api.goTo(f.view,1,{force:true});
 f.view.cm.dispatch({changes:{from:0,to:8,insert:'modified'}});
 f.helpers.setDirty(f.view,true);
 await f.api.goTo(f.view,4000);
 assert.equal(f.saves.length,1);
 assert.equal(f.saves[0].path,'large.txt');
 assert.equal(f.saves[0].expected_sha256,'a'.repeat(64));
 assert.ok(f.saves[0].content.startsWith('modified'));
 assert.ok(f.saves[0].content.length<10000);
 assert.ok(f.view.cm.state.doc.toString().includes('updated 4000'));
});
test('search delegates to server rather than scanning an incomplete buffer',async()=>{
 const f=fixture();
 f.api.attach(f.view,f.helpers);
 await f.api.goTo(f.view,1,{force:true});
 const found=await f.api.find(f.view,'needle',true);
 assert.equal(found.line,2345);
 assert.ok(f.requests.some(url=>url.includes('/search?')&&url.includes('q=needle')));
});
