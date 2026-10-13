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
 assert.ok(assistant.includes("requestDebug.failure('provider_request',e)"));
});
test('debug report redacts keys and captures HTTP status and stale-session probe',async()=>{
 const {debug,requests}=createDebug({sessions:[
  {id:'unrelated',kind:'terminal',status:'running'},
  {id:'session-A',kind:'',task_id:0,status:'running',target_type:'local'},
 ]});
 debug.begin('session_verification',{
  session_id:'session-A',question:'Can you help?',command:'curl -H "Authorization: Bearer verysecretvalue"',
  output:'password=hunter123\nAPI_KEY=wrong-secret',
 },{meta:{id:'session-A',kind:'',status:'running',cwd:'/test'}},
 {provider:'openai',model:'gpt-4.1-mini'});
 const e=new Error('terminal session not found');e.httpStatus=404;
 await debug.failure('session_verification',e,{probe:true});
 const report=debug.getReport();
 assert.equal(report.response.http_status,404);
 assert.equal(report.session_probe.requested_id_found,true);
 assert.equal(report.session_probe.match.kind,'');
 assert.deepEqual(requests,['/api/sessions']);
 const copied=JSON.stringify(report);
 for(const leaked of ['verysecretvalue','hunter123','wrong-secret'])assert.ok(!copied.includes(leaked),leaked);
 assert.match(copied,/Legacy terminal kind is empty/);
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
