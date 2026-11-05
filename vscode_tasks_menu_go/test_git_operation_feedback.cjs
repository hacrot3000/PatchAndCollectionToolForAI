const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

const js=fs.readFileSync(path.join(__dirname,'web/featuremods/gitstatus.js'),'utf8');
const start=js.indexOf('async function copyText(text){');
const end=js.indexOf('function repoStorageKey()',start);
assert.ok(start>0&&end>start,'Git operation handlers remain available');

function harness(){
  const copied=[],errors=[],requests=[],timeouts=[],cancelRequests=[];
  let clipboardError=false,documentCopySuccess=false,serverError=false;
  const node=(textContent='')=>({textContent,hidden:false,disabled:false,dataset:{},style:{},
    classList:{toggle(){},remove(){}},scrollHeight:400,clientHeight:100,scrollTop:0,
    scrollIntoView(){},setAttribute(){},select(){},remove(){}});
  const operation=node(),operationFeedback=node(),operationCommand=node();
  const operationCopy=node('Copy command'),operationCopyOutput=node('Copy output');
  const operationCancel=node('Cancel'),operationClear=node('Clear'),operationOutput=node();
  const clipboard={async writeText(value){
    if(clipboardError)throw new Error('clipboard denied');
    copied.push(value);
  }};
  const app={
    showError(error){errors.push(String(error?.message||error));},
    async jsonFetch(url,options){
      requests.push(url);
      if(serverError)throw new Error('cancel rejected');
      cancelRequests.push(JSON.parse(options.body));
      return {ok:true};
    }
  };
  const context={console,Map,Date,Promise,
    navigator:{clipboard},
    window:{isSecureContext:true},
    document:{createElement:()=>node(),body:{append(){}},
      execCommand(){return documentCopySuccess;}},
    app,operation,operationFeedback,operationCommand,operationCopy,
    operationCopyOutput,operationCancel,operationClear,operationOutput,
    clearTimeout(){},setTimeout(callback,time){timeouts.push({callback,time});return timeouts.length;},
    requestAnimationFrame(callback){callback();}
  };
  context.globalThis=context;
  context.TaskMenuGitOutputFormat={format:value=>value};
  vm.createContext(context);
  vm.runInContext(
    "let lastCommand='',lastOutput='',activeGitJob=null;\n"+js.slice(start,end)+
    '\nglobalThis.__test={showOperation,beginOperation,cancelGitJob,copyGitOperation,'+
    'setJob(job){activeGitJob=job;},values(){return {lastCommand,lastOutput,activeGitJob};}};',
    context,{filename:'gitstatus-feedback-extract.js',timeout:1000});
  return {ui:context.__test,copied,errors,requests,cancelRequests,timeouts,
    operation,operationFeedback,operationCommand,operationCopy,operationCopyOutput,
    operationCancel,operationClear,operationOutput,
    denyClipboard(value){clipboardError=value;},
    documentCopy(value){documentCopySuccess=value;},
    failCancel(value){serverError=value;}};
}

test('Copy command and Copy output acknowledge successful clipboard writes',async()=>{
  const h=harness();
  h.ui.showOperation('git push','To keybase://team/example\n  dev -> dev');
  await h.operationCopy.onclick();
  assert.deepEqual(h.copied,['git push']);
  assert.equal(h.operationCopy.textContent,'✓ Copied');
  assert.equal(h.operationFeedback.textContent,'✓ Git command copied');
  assert.equal(h.operationFeedback.hidden,false);
  await h.operationCopyOutput.onclick();
  assert.deepEqual(h.copied,['git push','To keybase://team/example\n  dev -> dev']);
  assert.equal(h.operationCopyOutput.textContent,'✓ Copied');
  assert.equal(h.operationFeedback.textContent,'✓ Git output copied');
  assert.equal(h.operationFeedback.dataset.kind,'success');
});

test('Git clear gives visible feedback even after hiding console',()=>{
  const h=harness();
  h.ui.showOperation('git status','file modified');
  h.operationClear.onclick();
  assert.equal(h.operation.hidden,true);
  assert.equal(h.operationFeedback.hidden,false);
  assert.equal(h.operationFeedback.textContent,'✓ Git console cleared');
  assert.equal(h.ui.values().lastCommand,'');
  assert.equal(h.ui.values().lastOutput,'');
  assert.equal(h.operationOutput.textContent,'');
});

test('Clear during running operation preserves output and explains why',()=>{
  const h=harness();
  h.ui.showOperation('git push','sending','', 'running');
  h.ui.setJob({id:'job-17'});
  h.operationClear.onclick();
  assert.equal(h.operation.hidden,false);
  assert.equal(h.operationOutput.textContent,'sending');
  assert.equal(h.operationFeedback.dataset.kind,'error');
  assert.match(h.operationFeedback.textContent,/Cannot clear while Git is running/);
});

test('Cancel shows pending and success, and targets the active Git job',async()=>{
  const h=harness();
  h.ui.setJob({id:'job-42'});
  const response=h.operationCancel.onclick();
  assert.match(h.operationFeedback.textContent,/Requesting Git cancellation/);
  await response;
  assert.deepEqual(h.cancelRequests,[{id:'job-42',action:'cancel'}]);
  assert.equal(h.operationFeedback.textContent,'✓ Git cancellation requested');
  assert.equal(h.operationCancel.textContent,'Cancel');
  assert.equal(h.operationCancel.disabled,false);
});

test('Cancel failure and denied clipboard report errors instead of claiming success',async()=>{
  const h=harness();
  h.ui.setJob({id:'job-43'});
  h.failCancel(true);
  await h.operationCancel.onclick();
  assert.equal(h.operationFeedback.dataset.kind,'error');
  assert.match(h.operationFeedback.textContent,/Could not cancel/);
  assert.equal(h.errors.length,1);
  h.ui.showOperation('git pull','updated');
  h.denyClipboard(true);
  await h.operationCopy.onclick();
  assert.equal(h.operationFeedback.dataset.kind,'error');
  assert.match(h.operationFeedback.textContent,/Could not copy Git command/);
  assert.equal(h.errors.length,2);
  assert.equal(h.copied.length,0);
});

test('Copy button refuses empty text; next command clears stale feedback',async()=>{
  const h=harness();
  await h.operationCopy.onclick();
  assert.match(h.operationFeedback.textContent,/No Git command/);
  assert.equal(h.operationFeedback.dataset.kind,'error');
  h.ui.beginOperation('git fetch');
  assert.equal(h.operationFeedback.hidden,true);
  assert.equal(h.operationFeedback.textContent,'');
});
