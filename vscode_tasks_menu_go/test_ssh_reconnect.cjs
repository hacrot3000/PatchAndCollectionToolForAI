const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const assert=require('node:assert/strict');
const test=require('node:test');
const policySource=fs.readFileSync(path.join(__dirname,'web/featuremods/sshreconnect_policy.js'),'utf8');
const controllerSource=fs.readFileSync(path.join(__dirname,'web/featuremods/sshreconnect.js'),'utf8');
const next=fs.readFileSync(path.join(__dirname,'web/featuremods/next.js'),'utf8');
const p=vm.runInNewContext(policySource+';globalThis.TaskDeckSSHReconnectPolicy',{}, {timeout:1000});
const ssh=(fields={})=>({id:'ssh-session',task_id:-1,kind:'terminal',target_type:'ssh',
 target_profile_id:'ssh-profile',status:'exited',exit_code:255,...fields});
test('SSH transport 255 reconnects only when terminal was observed running',()=>{
 assert.equal(p.eligible(ssh(),{observedRunning:true}),true);
 assert.equal(p.eligible(ssh(),{observedRunning:false}),false);
 assert.equal(p.eligible(ssh({status:'running'}),{observedRunning:true}),false);
 assert.equal(p.eligible(ssh({status:'stopped'}),{observedRunning:true}),false);
});
test('clean logout 0 never reconnects, nor other shell exit codes',()=>{
 for(const exit_code of [0,1,2,126,127,130]){
  assert.equal(p.eligible(ssh({exit_code}),{observedRunning:true}),false,'exit '+exit_code);
 }
 for(const command of ['exit',' EXIT ','logout','exit 255','builtin exit 255','command exit 5']){
  assert.equal(p.explicitExit(command),true,command);
  assert.equal(p.eligible(ssh(),{observedRunning:true,explicitLogout:true}),false);
 }
 for(const command of ['echo exit','exit_status','sudo exit','exit; reboot']){
  assert.equal(p.explicitExit(command),false,command);
 }
});
test('SSH 255 authentication or host-key failures must not auto-reconnect',()=>{
 for(const text of [
  'WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!',
  'Host key verification failed.',
  'Permission denied (publickey,password).',
  'Too many authentication failures',
  'Bad owner or permissions on /home/user/.ssh/config',
  'no matching key exchange method found',
 ]){
  assert.equal(p.fatalError(text),true,text);
  assert.equal(p.eligible(ssh(),{observedRunning:true,tail:text}),false,text);
 }
 assert.equal(p.eligible(ssh(),{observedRunning:true,tail:'Connection timed out\nssh returned 255'}),true);
});
test('legacy SSH terminal must have an explicit SSH label, not a Patch Tool label',()=>{
 assert.equal(p.eligible(ssh({kind:'',task_id:-1,label:'SSH · m3-prod-gs-02'}),{observedRunning:true}),true);
 assert.equal(p.eligible(ssh({kind:'',task_id:-1,label:'Patch Tool · SSH operation'}),{observedRunning:true}),false);
 assert.equal(p.eligible(ssh({kind:'',task_id:0,label:'SSH · m3-prod-gs-02'}),{observedRunning:true}),false);
});
test('never reconnect a local terminal, task, or unlinked SSH profile',()=>{
 for(const fields of [
  {target_type:'local'}, {target_profile_id:''},
  {kind:'patch'}, {kind:'task'}, {target_type:'unknown'},
 ]){
  assert.equal(p.eligible(ssh(fields),{observedRunning:true}),false,JSON.stringify(fields));
 }
});
test('bounded exponential-ish backoff does not exceed five automatic retries',()=>{
 assert.equal(p.maxAttempts,5);
 assert.deepEqual(Array.from({length:5},(_,i)=>p.delay(i)),[2000,5000,10000,20000,30000]);
 assert.equal(p.delay(99),30000);
});
test('SSH tab module implements UI opt-out, manual retry, cleanup and session recreation',()=>{
 assert.ok(next.indexOf("import '/featuremods/shellintegration.js'")<
  next.indexOf("import '/featuremods/sshreconnect_policy.js'"));
 assert.ok(next.indexOf("import '/featuremods/sshreconnect_policy.js'")<
  next.indexOf("import '/featuremods/sshreconnect.js'"));
 for(const required of [
  "window.addEventListener('taskmenu:session'",
  "window.addEventListener('taskmenu:shell-integration'",
  "window.addEventListener('taskmenu:output'",
  "ssh_profile_id:st.profileID",
  "app.materializeSession(observed,app.active===st.id)",
  "if(candidate?.id&&!adopted)await cleanupCandidate(candidate.id,true)",
  "app.jsonFetch('/api/ssh/profiles')",
  "profile.auth_method==='password'&&!profile.has_secret",
  "policy.explicitExit(st.line)",
  "Cancel retries",
  "Reconnect now",
  "st.attempts>=policy.maxAttempts",
  "policy.eligible(meta,",
  "attempt(st,true)",
  "const observed=manual?candidate:",
 ]){
  assert.ok(controllerSource.includes(required),'missing '+required);
 }
 assert.ok(!controllerSource.includes("term.reset()"),'do not clear prior SSH scrollback');
 assert.ok(!controllerSource.includes("eval("));
 new Function(controllerSource);
});
