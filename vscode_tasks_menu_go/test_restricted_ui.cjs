const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

const source=fs.readFileSync(path.join(__dirname,'web/featuremods/restricted.js'),'utf8');
function setup({copyLines=1,copyBytes=20,editable=true}={}){
 const nodes={},calls=[],copied=[];
 for(const id of ['command','run','command-status','command-output','file','start','count',
  'view','copy','source-output','file-status','protected-editor','save-lines','clear-lines']){
  nodes[id]={id,value:'',textContent:'',disabled:false,listeners:{},max:'',
   addEventListener(name,cb){this.listeners[name]=cb},
   contains(n){return this===n}};
 }
 nodes.file.value='main.go';nodes.start.value='1';nodes.count.value='2';
 let selection=null;
 const window={getSelection:()=>selection};
 const json=data=>({ok:true,status:200,headers:{get:()=> 'application/json'},
  json:async()=>data,text:async()=>JSON.stringify(data)});
 const fetch=async(url,options={})=>{
  calls.push({url,options});
  switch(url){
   case '/api/restricted/status':return json({max_visible_lines:3,max_copy_lines:copyLines,max_copy_bytes:copyBytes});
   case '/api/browser/lease':return json({lease:'test-lease'});
   case '/api/restricted/command':return json({output:'On branch testing',exit_code:0});
   case '/api/restricted/file-window':return json({path:'main.go',content:'line01\nline02\n',
     start_line:1,end_line:2,total_lines:10,sha256:'a'.repeat(64),editable,remaining:{file:2,today:8}});
   case '/api/restricted/file-edit':return json({saved:true,sha256:'b'.repeat(64)});
   default:throw Error('Unexpected API request '+url);
  }
 };
 const sandbox={document:{getElementById:id=>nodes[id]},window,
  navigator:{clipboard:{writeText:async x=>copied.push(x)}},fetch,TextEncoder};
 vm.runInNewContext(source,sandbox,{filename:'restricted.js'});
 const flush=()=>new Promise(resolve=>setImmediate(resolve));
 return {nodes,calls,copied,flush,
  select(text){selection={isCollapsed:false,anchorNode:nodes['source-output'],
   focusNode:nodes['source-output'],toString:()=>text}}};
}

test('safe Git command uses a single reviewed endpoint and browser lease',async()=>{
 const h=setup();await h.flush();
 h.nodes.command.value='git status';
 await h.nodes.run.listeners.click();await h.flush();
 const calls=h.calls.map(x=>x.url);
 assert.deepEqual(calls,['/api/restricted/status','/api/browser/lease','/api/restricted/command']);
 const request=h.calls.at(-1);
 assert.equal(request.options.headers['X-TaskMenu-Lease'],'test-lease');
 assert.equal(JSON.parse(request.options.body).command,'git status');
 assert.equal(h.nodes['command-output'].textContent,'On branch testing');
});

test('view and save only sends authorized visible lines, never full-file contents',async()=>{
 const h=setup();await h.flush();
 await h.nodes.view.listeners.click();await h.flush();
 assert.equal(h.nodes['protected-editor'].value,'line01\nline02');
 assert.equal(h.nodes['save-lines'].disabled,false);
 h.nodes['protected-editor'].value='edit01\nedit02';
 await h.nodes['save-lines'].listeners.click();await h.flush();
 const edit=h.calls.filter(x=>x.url==='/api/restricted/file-edit');
 assert.equal(edit.length,1);
 assert.deepEqual(JSON.parse(edit[0].options.body),{
  path:'main.go',start_line:1,expected_sha256:'a'.repeat(64),lines:['edit01','edit02']
 });
 assert.equal(h.nodes['protected-editor'].value,'','stale source is cleared after commit');
 assert.equal(h.nodes['save-lines'].disabled,true);
 assert.equal(h.calls.filter(x=>x.url==='/api/project/file').length,0);
});

test('inserting or deleting physical source lines is rejected before POST',async()=>{
 const h=setup();await h.flush();await h.nodes.view.listeners.click();await h.flush();
 h.nodes['protected-editor'].value='one\ntwo\nthree';
 await h.nodes['save-lines'].listeners.click();await h.flush();
 assert.equal(h.calls.some(x=>x.url==='/api/restricted/file-edit'),false);
 assert.equal(h.nodes['save-lines'].disabled,false);
});

test('copy button applies line and byte UI limits; never grants new source access',async()=>{
 const h=setup({copyLines:1,copyBytes:8});await h.flush();
 await h.nodes.view.listeners.click();await h.flush();
 h.select('line01\nline02');
 await h.nodes.copy.listeners.click();assert.deepEqual(h.copied,[]);
 h.select('line01');await h.nodes.copy.listeners.click();
 assert.deepEqual(h.copied,['line01']);
 assert.equal(h.calls.filter(x=>x.url==='/api/restricted/file-window').length,1);
});

test('command history is in page memory; page has no storage or unrestricted endpoints',()=>{
 assert.equal(/localStorage|sessionStorage|indexedDB/.test(source),false);
 assert.equal(/\/api\/project\/file["']/.test(source),false);
 assert.equal(/\/api\/sessions/.test(source),false);
});

test('server read-only flag disables edits but permits viewing',async()=>{
 const h=setup({editable:false});await h.flush();
 await h.nodes.view.listeners.click();await h.flush();
 assert.equal(h.nodes['protected-editor'].disabled,true);
 assert.equal(h.nodes['save-lines'].disabled,true);
 assert.equal(h.nodes.copy.disabled,false);
 assert.equal(h.nodes['source-output'].textContent,'line01\nline02\n');
});
