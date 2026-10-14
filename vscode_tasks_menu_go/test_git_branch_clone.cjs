const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const source=fs.readFileSync(path.join(__dirname,'web/featuremods/gitstatus.js'),'utf8');
const sha1='a'.repeat(40),sha2='b'.repeat(40);
function helper(){
 const from=source.indexOf('function cloneBranchSources(data){');
 const to=source.indexOf('\nasync function loadBranches(){',from);
 assert.ok(from>0&&to>from,'branch clone wizard has production implementation');
 const doc={activeElement:null,listeners:{},body:{children:[],append(el){this.children.push(el);}},
  createElement(tag){return element(tag)},addEventListener(type,cb){this.listeners[type]=cb},
  removeEventListener(type){delete this.listeners[type]}};
 const calls=[];
 let repo='repo-one';
 function element(tag){
  return {tag,children:[],style:{},disabled:false,textContent:'',value:'',className:'',attributes:{},
   append(...nodes){this.children.push(...nodes)},
   setAttribute(key,value){this.attributes[key]=value},
   remove(){this.removed=true;doc.body.children=doc.body.children.filter(x=>x!==this)},
   focus(){doc.activeElement=this},click(){return this.onclick?.()}};
 }
 function el(tag,cls,text){const node=element(tag);node.className=cls||'';if(text!==undefined)node.textContent=text;return node;}
 const api=new Function('document','el','activeRepoID','action','refresh','loadBranches','currentView',
  source.slice(from,to)+'\nreturn {cloneBranchSources,cloneBranchNameError,openCloneBranchWizard};'
 )(doc,el,repo,async(...args)=>{calls.push(args);return {ok:true}},async()=>{},async()=>{},'changes');
 return {api,doc,calls,get repo(){return repo},set repo(value){repo=value}};
}
const data={local:[{name:'develop',sha:sha1},{name:'main',sha:sha2}],remote:[{name:'origin/feature/x',sha:sha2},{name:'origin/HEAD',sha:sha1}]};
test('source options distinguish local and remote refs, exclude HEAD and invalid SHA',()=>{
 const h=helper();
 const sources=h.api.cloneBranchSources({...data,local:[...data.local,{name:'broken',sha:'xyz'}]});
 assert.deepEqual(sources.map(x=>x.source+':'+x.name),['local:develop','local:main','remote:origin/feature/x']);
 assert.equal(sources[2].sha,sha2);
});
test('branch name duplicate, malformed and namespace path checks',()=>{
 const h=helper(),old=new Set(['develop','main']);
 for(const name of ['','develop','../escape','feature//bad','feature bad','feature/bad.lock','-danger','HEAD','feature/@{bad','feature/.hidden']){
  assert.ok(h.api.cloneBranchNameError(name,old),name);
 }
 for(const name of ['feature/new','fix/123','release-v2']){
  assert.equal(h.api.cloneBranchNameError(name,old),'',name);
 }
});
test('wizard can select a remote source, enter name and send exact commit while preserving checkout',async()=>{
 const h=helper();
 const pending=h.api.openCloneBranchWizard(data,{source:'remote',name:'origin/feature/x'});
 const backdrop=h.doc.body.children[0],dialog=backdrop.children[0];
 assert.equal(dialog.attributes.role,'dialog');
 const select=dialog.children.find(x=>x.tag==='select');
 const input=dialog.children.find(x=>x.tag==='input');
 const actions=dialog.children.find(x=>x.className==='git-clone-actions');
 const [cancel,submit]=actions.children;
 assert.equal(select.value,'2');
 assert.equal(submit.disabled,true);
 input.value='feature/cloned-from-remote';input.oninput();
 assert.equal(submit.disabled,false);
 await submit.onclick();
 assert.equal(await pending,true);
 assert.equal(backdrop.removed,true);
 assert.equal(h.calls.length,1);
 assert.equal(h.calls[0][0],'clone_branch');
 assert.deepEqual(h.calls[0][1],{branch:'feature/cloned-from-remote',source:'remote',ref:'origin/feature/x',expected_sha:sha2});
 assert.deepEqual(h.calls[0][3],{repoID:'repo-one',refresh:false,recovery:false});
 assert.equal(cancel.disabled,true);
});
test('wizard keeps user input on invalid name and Cancel does not mutate Git',async()=>{
 const h=helper();
 const pending=h.api.openCloneBranchWizard(data,{source:'local',name:'develop'});
 const dialog=h.doc.body.children[0].children[0];
 const input=dialog.children.find(x=>x.tag==='input'),actions=dialog.children.find(x=>x.className==='git-clone-actions');
 const [cancel,submit]=actions.children;
 input.value='main';input.oninput();
 assert.equal(submit.disabled,true);
 await cancel.onclick();
 assert.equal(await pending,false);
 assert.equal(h.calls.length,0);
});
test('branch UI exposes Clone button while branch name click remains copy-only',()=>{
 assert.match(source,/actionButton\('Clone branch…',\(\)=>openCloneBranchWizard\(data\)/);
 assert.match(source,/actionButton\('Clone',\(\)=>openCloneBranchWizard\(data,\{source:branch\.remote\?'remote':'local',name:branch\.name\}\)/);
 assert.match(source,/branchName\.title='Click to copy branch name: '/);
 assert.match(source,/await copyText\(branch\.name\)/);
 assert.match(source,/case 'clone_branch':return 'git branch --no-track '/);
 assert.match(source,/await action\('clone_branch'/);
});
