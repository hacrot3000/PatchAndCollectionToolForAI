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
   disabled:false,title:'',dataset:{},setAttribute(){},querySelectorAll(){return []}};
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
  findInput:Object.assign(node(),{value:'original'}),findReplaceInput:Object.assign(node(),{value:'new'}),findStatus:node(),findReplace:node(),findReplaceAll:node(),findCase:Object.assign(node(),{checked:true}),
  cm,body:node(),scroller:node(),history:node(),outline:node(),
  markdownPreview:node(),structuredSelect:node(),lineEndingSelect:node(),encodingSelect:node(),
  minimapToggle:node(),largeFileBadge:node(),closed:false,internalUpdate:false,dirty:false};
 const storage=new Map(),requests=[],saves=[],replacements=[];
 let replacementGate=null,windowGate=null;
 const response=(start,source='original')=>{
  const first=Math.max(1,start),last=first+149;
  const content=Array.from({length:150},(_,i)=>source+' '+(first+i)+'\n').join('');
  return {streamed:true,start_line:first,end_line:last,total_lines:500000,
   start_byte:first*20,end_byte:first*20+content.length,size:15000000,
   mtime_ns:Number('1761588600123456789'),mtime_ns_exact:'1761588600123456789',
   content,line_ending:'lf',sha256:source==='original'?'a'.repeat(64):'b'.repeat(64),
   window_sha256:'c'.repeat(64),read_only:false};
 };
 let changed=false;
 const app={
  taskData:{workspace:'test'},currentUser:{user_id:'1'},
  jsonFetch:async url=>{requests.push(url);const params=new URL('https://example.invalid'+url).searchParams;
   if(url.includes('/search'))return {found:true,line:2345,column:0};
   if(windowGate){const gate=windowGate;windowGate=null;await gate.promise;}
   return response(Number(params.get('start')),changed?'updated':'original');
  },
  fetchWithLease:async(url,init)=>{
   if(url.includes('/replace')){
    if(replacementGate){const gate=replacementGate;replacementGate=null;await gate.promise;}
    replacements.push(JSON.parse(init.body));changed=true;
    return {ok:true,json:async()=>({replacements:replacements.at(-1).all?500:1})};
   }
   saves.push(JSON.parse(init.body));changed=true;
   return {ok:true,json:async()=>({history_warning:'Range saved'})};
  },showError:error=>{throw error}
 };
 const doc={createElement:node,head:{append(){}}};
 const context={document:doc,URLSearchParams,URL,TextEncoder,TextDecoder,sessionStorage:{
  getItem:key=>storage.get(key)||null,setItem:(key,value)=>storage.set(key,value),removeItem:key=>storage.delete(key)
 },setTimeout,clearTimeout,console,TaskMenuApp:app,globalThis:null};
 context.globalThis=context;
 vm.runInNewContext(code,context,{filename:'largefileeditor.js'});
 const helpers={
  applyReadOnly(){},
  setDirty(v,dirty){v.dirty=Boolean(dirty)},
  dirtyCloseChoice:async()=> 'save'
 };
 return {view,helpers,api:context.TaskMenuLargeEditor,requests,saves,replacements,storage,app,
  pauseReplace(){let resolve;const promise=new Promise(done=>resolve=done);replacementGate={promise};return resolve;},
  pauseWindow(){let resolve;const promise=new Promise(done=>resolve=done);windowGate={promise};return resolve;}
 };
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

test('large-file Replace uses server literal match coordinates and refreshes the editor',async()=>{
 const f=fixture();
 f.api.attach(f.view,f.helpers);
 await f.api.goTo(f.view,1,{force:true});
 const match=await f.api.find(f.view,'original',true);
 assert.equal(match.line,2345);
 const count=await f.api.replace(f.view,{all:false});
 assert.equal(count,1);
 assert.equal(f.replacements.length,1);
 assert.equal(f.replacements[0].line,2345);
 assert.equal(f.replacements[0].column,0);
 assert.equal(f.replacements[0].match_case,true);
 assert.equal(f.replacements[0].find,'original');
 assert.equal(f.replacements[0].replacement,'new');
 assert.ok(f.view.cm.state.doc.toString().includes('updated 2345'));
});

test('large-file Replace All runs on server without transferring whole file to browser',async()=>{
 const f=fixture();
 f.api.attach(f.view,f.helpers);
 await f.api.goTo(f.view,1,{force:true});
 const count=await f.api.replace(f.view,{all:true});
 assert.equal(count,500);
 assert.equal(f.replacements[0].all,true);
 assert.equal(f.replacements[0].expected_sha256,'a'.repeat(64));
 assert.equal(f.replacements[0].expected_size,15000000);
 assert.ok(f.view.cm.state.doc.length<10000,'editor retains a small window after Replace All');
});

test('Replace and Save send exact nanosecond file revision despite JS Number rounding',async()=>{
 const exact='1761588600123456789';
 assert.notEqual(String(Number(exact)),exact,'test requires a timestamp beyond JS safe integer precision');
 const replace=fixture();
 replace.api.attach(replace.view,replace.helpers);
 await replace.api.goTo(replace.view,1,{force:true});
 await replace.api.replace(replace.view,{all:true});
 assert.equal(replace.replacements.length,1);
 assert.equal(replace.replacements[0].expected_mtime_ns_exact,exact);
 assert.notEqual(String(replace.replacements[0].expected_mtime_ns),exact);
 const save=fixture();
 save.api.attach(save.view,save.helpers);
 await save.api.goTo(save.view,1,{force:true});
 save.view.cm.dispatch({changes:{from:0,to:8,insert:'modified'}});
 save.helpers.setDirty(save.view,true);
 await save.api.save(save.view);
 assert.equal(save.saves.length,1);
 assert.equal(save.saves[0].expected_mtime_ns_exact,exact);
 assert.notEqual(String(save.saves[0].expected_mtime_ns),exact);
});


test('Replace locks large-file editing and navigation until server completes',async()=>{
 const f=fixture();
 f.api.attach(f.view,f.helpers);
 await f.api.goTo(f.view,1,{force:true});
 const release=f.pauseReplace();
 const pending=f.api.replace(f.view,{all:true});
 assert.equal(f.view.largeReplaceBusy,true);
 assert.equal(f.api.stateFor(f.view).replacing!==null,true);
 assert.equal(f.api.stateFor(f.view).nav.classList!==undefined,true);
 assert.equal(f.view.findReplace.disabled,true);
 assert.equal(f.view.findReplaceAll.disabled,true);
 assert.equal(f.view.findReplaceInput.disabled,true);
 assert.equal(await f.api.goTo(f.view,4500),false); // no background jump while locked
 const prior=f.view.cm.state.doc.toString();
 assert.equal(await f.api.reload(f.view),false,'reload must not replace buffer during server Replace');
 assert.equal(f.view.cm.state.doc.toString(),prior);
 await Promise.resolve();await Promise.resolve();
 release();
 assert.equal(await pending,500);
 assert.equal(f.view.largeReplaceBusy,false);
 assert.equal(f.view.findReplace.disabled,false);
 assert.equal(f.view.findReplaceAll.disabled,false);
 assert.equal(f.view.findReplaceInput.disabled,false);
});

test('dirty Cancel preserves edits and never executes Replace',async()=>{
 const f=fixture();f.helpers.dirtyCloseChoice=async()=> 'cancel';
 f.api.attach(f.view,f.helpers);await f.api.goTo(f.view,1,{force:true});
 f.view.cm.dispatch({changes:{from:0,to:8,insert:'unsaved!'}});
 f.helpers.setDirty(f.view,true);
 const unchanged=f.view.cm.state.doc.toString();
 assert.equal(await f.api.replace(f.view,{all:true}),0);
 assert.equal(f.replacements.length,0);
 assert.equal(f.saves.length,0);
 assert.equal(f.view.cm.state.doc.toString(),unchanged);
 assert.equal(f.view.dirty,true);
 assert.equal(f.view.largeReplaceBusy,false);
});

test('dirty Save completes before Replace and refreshes its file version',async()=>{
 const f=fixture();f.helpers.dirtyCloseChoice=async()=> 'save';
 f.api.attach(f.view,f.helpers);await f.api.goTo(f.view,1,{force:true});
 f.view.cm.dispatch({changes:{from:0,to:8,insert:'changed!'}});
 f.helpers.setDirty(f.view,true);
 assert.equal(await f.api.replace(f.view,{all:true}),500);
 assert.equal(f.saves.length,1);
 assert.equal(f.replacements.length,1);
 assert.equal(f.replacements[0].expected_sha256,'b'.repeat(64),'Replace must use post-Save revision');
 assert.equal(f.view.largeReplaceBusy,false);
 assert.equal(f.view.dirty,false);
});

test('dirty Discard waits for successful reload before clearing draft',async()=>{
 const f=fixture();f.helpers.dirtyCloseChoice=async()=> 'discard';
 f.api.attach(f.view,f.helpers);await f.api.goTo(f.view,1,{force:true});
 f.view.cm.dispatch({changes:{from:0,to:8,insert:'unsaved!'}});
 f.helpers.setDirty(f.view,true);
 const before=f.view.cm.state.doc.toString();
 const release=f.pauseWindow();
 const pending=f.api.replace(f.view,{all:true});
 await Promise.resolve();await Promise.resolve();
 assert.equal(f.view.largeReplaceBusy,true);
 assert.equal(f.view.cm.state.doc.toString(),before);
 assert.equal(f.view.dirty,true);
 assert.equal(f.replacements.length,0);
 release();
 assert.equal(await pending,500);
 assert.equal(f.saves.length,0);
 assert.equal(f.view.dirty,false);
});

test('failed dirty Discard reload never loses user edits',async()=>{
 const f=fixture();f.helpers.dirtyCloseChoice=async()=> 'discard';
 f.api.attach(f.view,f.helpers);await f.api.goTo(f.view,1,{force:true});
 f.view.cm.dispatch({changes:{from:0,to:8,insert:'unsaved!'}});
 f.helpers.setDirty(f.view,true);
 const before=f.view.cm.state.doc.toString();
 const original=f.app.jsonFetch;
 f.app.jsonFetch=async url=>{if(url.includes('/file-window?'))throw Error('simulated offline reload');return original(url);};
 await assert.rejects(()=>f.api.replace(f.view,{all:true}),/simulated offline reload/);
 assert.equal(f.view.cm.state.doc.toString(),before);
 assert.equal(f.view.dirty,true);
 assert.equal(f.view.largeReplaceBusy,false);
 assert.equal(f.saves.length,0);
 assert.equal(f.replacements.length,0);
});

test('editor UI enables streamed Replace and Replace All without removing readonly guard',()=>{
 assert.ok(editor.includes('return globalThis.TaskMenuLargeEditor.replace(view,{all:false})'));
 assert.ok(editor.includes('return globalThis.TaskMenuLargeEditor.replace(view,{all:true})'));
 assert.ok(editor.includes('view.findReplaceAll.disabled=readonly;'));
 assert.ok(!editor.includes('Replace-all is unavailable for windowed files'));
});
