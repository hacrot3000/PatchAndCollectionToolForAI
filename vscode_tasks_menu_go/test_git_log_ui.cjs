const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const source=fs.readFileSync(path.join(__dirname,'web/featuremods/gitlog.js'),'utf8');
function fixture(fileCount=2,parents=['a'.repeat(40)]){
 const actions=[],requests=[],fileDiff=[],dirDiff=[];
 const commit={sha:'b'.repeat(40),short:'bbbbbbbb',subject:'Fix permission table',
  date:'2026-10-14',author:'Dev Example'};
 const elements=[];
 const document={
  createElement(tag){const node={tag,children:[],hidden:false,disabled:false,style:{},textContent:'',value:'',
   attributes:{},append(...args){this.children.push(...args)},replaceChildren(...args){this.children=[...args]},
   setAttribute(key,value){this.attributes[key]=value}};elements.push(node);return node;}
 };
 const content=document.createElement('div');
 const ctx={globalThis:null,document,URLSearchParams,console,
  TaskMenuFileCompare:{openGitCommitFileDiffBetween(...args){fileDiff.push(args);return true}},
  TaskMenuDirectoryCompare:{openGitCommit(...args){dirDiff.push(args);return true}}};
 ctx.globalThis=ctx;vm.runInNewContext(source,ctx,{filename:'gitlog.js'});
 const ui=ctx.TaskDeckGitLog;
 let repo='.',commits=[commit],files=Array.from({length:fileCount},(_,i)=>({path:'file-'+i+'.txt',status:'M'}));
 const dependencies={
  app:{jsonFetch:async(url)=>{
   requests.push(String(url));
   const params=new URL('https://localhost'+url).searchParams;
   if(params.get('view')==='log'){
    const skip=Number(params.get('skip'));
    return {commits:skip?[]:commits,has_more:skip===0};
   }
   throw Error('unexpected request '+url);
  },showError:err=>{throw err}},
  gitView:async(view)=>{
   assert.equal(view,'commit-files');
   return {parent:parents[0]||'',parents,files};
  },
  content,
  el:(tag,cls='',title='')=>{const node=document.createElement(tag);node.className=cls;node.textContent=title;return node},
  actionButton:(label,fn)=>{const b=document.createElement('button');b.textContent=label;b.run=fn;actions.push(b);return b},
  action:()=>{},
  openGitCommitSemantics:()=>{},
  copyText:()=>{},
  renderGitGraphCommitDetail:()=>{},
  showOperation:()=>{},
  gitBranchChoiceWizard:{open:async()=>parents[1]||null},
  getRepoID:()=>repo
 };
 const button=label=>actions.find(a=>a.textContent===label);
 return {ui,dependencies,commit,actions,button,requests,dirDiff,fileDiff,content,elements,ctx,
  setFiles(n){files=Array.from({length:n},(_,i)=>({path:'file-'+i+'.txt',status:'M'}));},
  setCommits(x){commits=x},
  setRepo(x){repo=x}};
}
test('Search matches SHA, subject, author and date without another Git request',async()=>{
 const f=fixture();
 await f.ui.render(f.dependencies);
 const input=f.elements.find(x=>x.tag==='input');
 assert.ok(input);
 assert.equal(f.requests.length,1);
 input.value='fix DEV';input.oninput();
 assert.equal(f.ui.state.search,'fix DEV');
 assert.equal(f.requests.length,1);
 assert.equal(f.content.children[1].textContent.includes('1 matching'),true);
 input.value='absent';input.oninput();
 assert.equal(f.content.children[1].textContent.includes('0 matching'),true);
 assert.equal(f.requests.length,1);
 assert.equal(f.ui.matches(f.commit,'bbbb dev'),true);
 assert.equal(f.ui.matches(f.commit,'2026-10-14'),true);
});
test('Show Diff of a single changed file routes to existing File Compare',async()=>{
 const f=fixture(1);
 await f.ui.render(f.dependencies);
 await f.button('Show Diff').run();
 assert.equal(f.fileDiff.length,1);
 assert.equal(f.dirDiff.length,0);
 assert.equal(f.fileDiff[0][0],'.');
 assert.equal(f.fileDiff[0][2],'a'.repeat(40));
 assert.equal(f.fileDiff[0][3],f.commit.sha);
});
test('Show Diff of multi-file commit routes immutable parent and commit SHAs to Directory Compare',async()=>{
 const f=fixture(5);
 await f.ui.render(f.dependencies);
 await f.button('Show Diff').run();
 assert.equal(f.dirDiff.length,1);
 assert.deepEqual(f.dirDiff[0],['.','a'.repeat(40),f.commit.sha]);
 assert.equal(f.fileDiff.length,0);
});
test('Merge commits permit choosing the second parent without mutation',async()=>{
 const parents=['a'.repeat(40),'c'.repeat(40)];
 const f=fixture(4,parents);
 await f.ui.render(f.dependencies);
 await f.button('Show Diff').run();
 assert.deepEqual(f.dirDiff[0],['.',parents[1],f.commit.sha]);
});
test('Root commit does not invent parent and falls back to graph file list',async()=>{
 const f=fixture(3,[]);
 let invoked=0;
 f.dependencies.renderGitGraphCommitDetail=async()=>{invoked++;return true};
 await f.ui.render(f.dependencies);
 await f.button('Show Diff').run();
 assert.equal(invoked,1);
 assert.equal(f.dirDiff.length,0);
});
test('Load older commits uses bounded offset and expands search scope',async()=>{
 const f=fixture();
 await f.ui.render(f.dependencies);
 await f.button('Load older commits').run();
 assert.ok(f.requests.at(-1).includes('skip=1'));
 assert.ok(f.requests.at(-1).includes('limit=100'));
 assert.equal(f.ui.state.more,false);
});
