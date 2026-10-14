// Exercise real branch-row wizard dispatch code without a browser or network.
const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const source=fs.readFileSync(path.join(__dirname,'web/featuremods/gitstatus.js'),'utf8');
function extract(start,end){
 const a=source.indexOf(start),b=source.indexOf(end,a+start.length);
 assert.ok(a>=0&&b>a,'source function not found: '+start);
 return source.slice(a,b);
}
const merge=extract('async function openBranchMergeWizard(branch){','// Unix seconds from the *tip commit*');
const deletion=extract('async function openBranchDeleteWizard(branch){','// Create a new LOCAL branch');
function create(choice){
 let api=null,select=choice;
 const calls=[],wizardArgs=[],outputs=[];
 const wizard={async open(args){
   wizardArgs.push(args);
   return typeof select==='function'?select(()=>api.setRepo('repo-two')):select;
 }};
 const branchSource=new Function('gitBranchChoiceWizard','activeRepository',
  'mergeBranch','mergeToBranch','deleteLocalBranch','deleteRemoteTracking',
  'deleteRemoteBranch','showOperation','calls',
  "let activeRepoID='repo-one';\n"+
  merge+'\n'+deletion+
  '\nreturn {merge:openBranchMergeWizard,del:openBranchDeleteWizard,setRepo(value){activeRepoID=value}};');
 const h=branchSource(wizard,()=>({name:'repo-one'}),
  async branch=>calls.push(['mergeFrom',branch.name]),
  async branch=>calls.push(['mergeTo',branch.name]),
  async(branch,force)=>calls.push(['deleteLocal',branch.name,force]),
  async branch=>calls.push(['forgetTracking',branch.name]),
  async(branch,options)=>calls.push(['deleteRemote',branch.name,options?.localBranch?.name||'']),
  (...args)=>outputs.push(args),calls);
 api=h;
 return {h,calls,wizardArgs,outputs};
}
test('Merge wizard defaults to From and dispatches exactly one existing workflow',async()=>{
 const x=create('from'),branch={name:'feature/topic',remote:false};
 await x.h.merge(branch);
 assert.deepEqual(x.calls,[['mergeFrom','feature/topic']]);
 assert.equal(x.wizardArgs[0].defaultChoice,'from');
 assert.equal(x.wizardArgs[0].options.length,2);
 assert.deepEqual(x.wizardArgs[0].options.map(o=>o.id),['from','to']);
});
test('Merge To dispatches only the existing Merge To safety workflow',async()=>{
 const x=create('to');await x.h.merge({name:'origin/testing',remote:true});
 assert.deepEqual(x.calls,[['mergeTo','origin/testing']]);
});
test('Cancel or changing repository in merge wizard does nothing',async()=>{
 const x=create(null);assert.equal(await x.h.merge({name:'testing'}),false);assert.deepEqual(x.calls,[]);
 const y=create(change=>{change();return 'to';});
 assert.equal(await y.h.merge({name:'testing'}),false);assert.deepEqual(y.calls,[]);
});
test('Local Delete wizard offers safe and force choices, plus upstream when known',async()=>{
 const x=create('safe');
 await x.h.del({name:'dev',remote:false,upstream:'origin/dev'});
 assert.deepEqual(x.wizardArgs[0].options.map(o=>o.id),['safe','force','upstream']);
 assert.equal(x.wizardArgs[0].defaultChoice,'safe');
 assert.deepEqual(x.calls,[['deleteLocal','dev',false]]);
});
test('FORCE option calls forced local delete, remote never included',async()=>{
 const x=create('force');await x.h.del({name:'legacy',remote:false});
 assert.deepEqual(x.wizardArgs[0].options.map(o=>o.id),['safe','force']);
 assert.deepEqual(x.calls,[['deleteLocal','legacy',true]]);
});
test('Delete upstream passes the exact selected local branch for safe cleanup',async()=>{
 const x=create('upstream');
 await x.h.del({name:'dev',remote:false,upstream:'origin/dev'});
 assert.deepEqual(x.calls,[['deleteRemote','origin/dev','dev']]);
});
test('Remote tracking Delete never guesses local branch; forget is the default',async()=>{
 const forget=create('forget');
 await forget.h.del({name:'origin/testing',remote:true});
 assert.deepEqual(forget.wizardArgs[0].options.map(o=>o.id),['forget','remote']);
 assert.equal(forget.wizardArgs[0].defaultChoice,'forget');
 assert.deepEqual(forget.calls,[['forgetTracking','origin/testing']]);
 const remote=create('remote');await remote.h.del({name:'origin/testing',remote:true});
 assert.deepEqual(remote.calls,[['deleteRemote','origin/testing','']]);
});
test('Cancel and repository switching never mutate Git via Delete choice',async()=>{
 const cancel=create(null);
 assert.equal(await cancel.h.del({name:'dev',remote:false}),false);
 assert.deepEqual(cancel.calls,[]);
 const changed=create(change=>{change();return 'force'});
 assert.equal(await changed.h.del({name:'dev',remote:false}),false);
 assert.deepEqual(changed.calls,[]);
});
