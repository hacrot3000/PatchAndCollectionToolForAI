const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

const src=fs.readFileSync(path.join(__dirname,'web/featuremods/aidebug.js'),'utf8');
const assistant=fs.readFileSync(path.join(__dirname,'web/featuremods/aiassistant.js'),'utf8');
const next=fs.readFileSync(path.join(__dirname,'web/featuremods/next.js'),'utf8');

function element(tag){
 return {
  tag,children:[],disabled:false,textContent:'',className:'',style:{},
  append(...children){this.children.push(...children);},
  setAttribute(key,value){this[key]=value;},
 };
}
function createDebug(apiResponse){
 const head=element('head'),parent=element('div');
 const sandbox={
  document:{head,createElement:element},
  navigator:{clipboard:{async writeText(v){sandbox.copied=v;}}},
  JSON,Date,Number,String,Array,Object,
 };
 sandbox.globalThis=sandbox;
 vm.runInNewContext(src,sandbox,{timeout:1000});
 const requests=[];
 const app={
  active:'session-A',
  async jsonFetch(path){
   requests.push(path);
   if(path==='/api/sessions')return apiResponse;
   throw Error('Unexpected URL: '+path);
  }
 };
 const debug=sandbox.TaskDeckAIRequestDebug.create(parent,app);
 return {debug,sandbox,parent,requests};
}
test('debug module initializes and is imported before AI assistant',()=>{
 const a=next.indexOf("import '/featuremods/aidebug.js'");
 const b=next.indexOf("import '/featuremods/aiassistant.js'");
 assert.ok(a>=0&&b>a);
 assert.ok(assistant.includes("const requestDebug=globalThis.TaskDeckAIRequestDebug.create(scroll,aiApp);"));
 assert.ok(assistant.includes("requestDebug.failure('session_verification',error,{probe:true})"));
 assert.ok(assistant.includes("requestDebug.failure('provider_request',error)"));
});
test('debug report redacts keys and captures HTTP status and stale-session probe',async()=>{
 const {debug,requests}=createDebug({sessions:[
  {id:'unrelated',kind:'terminal',status:'running'},
  {id:'session-A',kind:'',task_id:0,status:'running',target_type:''},
 ]});
 debug.begin('session_verification',{
  session_id:'session-A',question:'Can you help?',command:'curl -H "Authorization: Bearer verysecretvalue"',
  output:'password=hunter123\nAPI_KEY=wrong-secret',
 },{meta:{id:'session-A',kind:'',status:'running',cwd:'/test'}},
 {provider:'openai',model:'gpt-4.1-mini'});
 const e=new Error('session is not an interactive terminal');e.httpStatus=409;
 await debug.failure('session_verification',e,{probe:true});
 const report=debug.getReport();
 assert.equal(report.response.http_status,409);
 assert.equal(report.session_probe.requested_id_found,true);
 assert.equal(report.session_probe.match.kind,'');
 assert.deepEqual(requests,['/api/sessions']);
 const copied=JSON.stringify(report);
 for(const leaked of ['verysecretvalue','hunter123','wrong-secret'])assert.ok(!copied.includes(leaked),leaked);
 assert.match(copied,/Legacy single-user terminal has blank kind and target_type/);
});
test('report does not record credentials and supports successful provider response',()=>{
 const {debug}=createDebug({sessions:[]});
 debug.begin('session_verification',{session_id:'abc',api_key:'supersecret'},
  {meta:{id:'abc'}},{provider:'anthropic',model:'claude-sonnet-5-5'});
 debug.step('provider_request_started',{payload:{authorization:'Bearer anothersecret',command:'pwd'}});
 debug.success('provider_request',{answer:'Checked.',executed:false});
 const report=debug.getReport();
 assert.equal(report.status,'success');
 assert.equal(report.response.answer,'Checked.');
 assert.ok(!JSON.stringify(report).includes('supersecret'));
 assert.ok(!JSON.stringify(report).includes('anothersecret'));
});

test('in-flight report tracks timeout window and copies updated elapsed time',async()=>{
 const {debug,sandbox,parent}=createDebug({sessions:[]});
 debug.begin('session_verification',{session_id:'session-A',mode:'explain',question:'Explain error'},
  {meta:{id:'session-A'}},{provider:'gemini',model:'gemini-3.8-flash',timeout_seconds:45});
 debug.ready('context_verified',{host:'local'});
 assert.equal(debug.getReport().status,'ready');
 debug.step('provider_request_started',{method:'POST',path:'/api/ai/assist',provider:'gemini',model:'gemini-3.8-flash'});
 const progress=debug.getReport();
 assert.equal(progress.status,'in_progress');
 assert.ok(Number.isFinite(progress.elapsed_ms));
 assert.ok(Date.parse(progress.expected_timeout_at)>Date.parse(progress.provider_started_at));
 const box=parent.children[0],head=box.children[0],copy=head.children[1];
 await copy.onclick();
 const copied=JSON.parse(sandbox.copied);
 assert.equal(copied.provider,'gemini');
 assert.equal(copied.stage,'provider_request_started');
 assert.equal(copied.timeout_seconds,45);
 assert.ok(typeof copied.elapsed_ms==='number');
 assert.equal(copied.response,null);
});
test('provider error report captures upstream HTTP status, safe code and server duration',async()=>{
 const {debug}=createDebug({sessions:[]});
 debug.begin('session_verification',{session_id:'session-A'},
  {meta:{id:'session-A'}},{provider:'gemini',model:'gemini-3.8-flash',timeout_seconds:60});
 debug.step('provider_request_started',{method:'POST',path:'/api/ai/assist'});
 const error=new Error('AI provider gemini HTTP 429: provider rate limit or quota exceeded');
 error.httpStatus=502;error.providerHttpStatus=429;
 error.providerErrorCode='RESOURCE_EXHAUSTED';error.providerDurationMs=1246;
 await debug.failure('provider_request',error);
 const report=debug.getReport();
 assert.equal(report.status,'failed');
 assert.equal(report.response.http_status,502);
 assert.equal(report.response.provider_http_status,429);
 assert.equal(report.response.provider_error_code,'RESOURCE_EXHAUSTED');
 assert.equal(report.response.provider_duration_ms,1246);
});
test('user-declined requests and browser aborts have final state',async()=>{
 const {debug}=createDebug({sessions:[]});
 debug.begin('session_verification',{session_id:'session-A'},null,{provider:'gemini'});
 debug.cancelled('external_request_declined');
 assert.equal(debug.getReport().status,'cancelled');
 assert.equal(debug.getReport().response.error_code,'user_declined');
 debug.begin('session_verification',{session_id:'session-A'},null,{provider:'gemini'});
 const e=new Error('Aborted');e.name='AbortError';
 await debug.failure('provider_request',e);
 assert.equal(debug.getReport().status,'cancelled');
});
