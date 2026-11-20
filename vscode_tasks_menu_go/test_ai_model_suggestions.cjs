const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const test=require('node:test');

const js=fs.readFileSync(path.join(__dirname,'web/featuremods/aiassistant.js'),'utf8');
function extract(start,end){
 const a=js.indexOf(start),b=js.indexOf(end,a+start.length);
 assert.ok(a>=0&&b>a,'Missing model UI section: '+start);
 return js.slice(a,b);
}
const catalog=vm.runInNewContext(
 extract('const providerCatalog={',"conf.provider=field('provider'")
 +';providerCatalog',{}, {timeout:1000}
);
const ids=['openai','anthropic','gemini','deepseek','groq','openrouter'];

test('All supported providers have a curated default, suggestions and official HTTPS API key instructions',()=>{
 assert.deepEqual(Object.keys(catalog).sort(),ids.slice().sort());
 for(const [id,provider] of Object.entries(catalog)){
  assert.ok(provider.label, id+' label');
  assert.ok(provider.models.includes(provider.defaultModel),id+' default model missing in suggestions');
  assert.ok(provider.models.length>=2,id+' should offer alternative models');
  assert.match(provider.keyURL,/^https:\/\//);
  assert.match(provider.modelURL,/^https:\/\//);
  assert.ok(provider.steps.length>25,id+' lacks usable key-generation instructions');
 }
});
test('Provider dropdown refreshes model suggestions but allows typed custom model names',()=>{
 const conf={provider:{value:'openai'},model:{value:'my-custom-model',placeholder:''}};
 const suggestions={entries:[],replaceChildren(){this.entries=[];},append(option){this.entries.push(option.value);}};
 const guide={textContent:''},link={href:''},modelsLink={href:''},notice={textContent:''};
 const context={
  providerCatalog:catalog,conf,modelSuggestions:suggestions,keySteps:guide,keyLink:link,
  modelsLink,configNotice:notice,
  aiElement(){return {value:''};},
 };
 const fn=vm.runInNewContext(extract('function refreshProviderGuide(resetModel){','guideToggle.onclick=')+';refreshProviderGuide',context,{timeout:1000});
 fn(false);
 assert.equal(conf.model.value,'my-custom-model','refresh without reset must preserve user-entered model');
 assert.ok(suggestions.entries.includes('gpt-5.4-mini'));
 conf.provider.value='gemini';
 fn(true);
 assert.equal(conf.model.value,catalog.gemini.defaultModel);
 assert.ok(suggestions.entries.includes('gemini-3.8-flash'));
 assert.equal(link.href,catalog.gemini.keyURL);
 assert.equal(modelsLink.href,catalog.gemini.modelURL);
 conf.model.value='gemini-next-custom';
 fn(false);
 assert.equal(conf.model.value,'gemini-next-custom','custom model must remain editable');
 assert.match(guide.textContent,/API key field/);
});
test('AI Settings saves provider and editable model while keeping credential write-only',()=>{
 assert.match(js,/conf\.provider=field\('provider'/);
 assert.match(js,/conf\.model=field\('model','Model \(choose or type\)','text'/);
 assert.match(js,/conf\.model\.setAttribute\('list',modelSuggestions\.id\)/);
 assert.match(js,/const options=\['enabled','provider','model'/);
 assert.match(js,/conf\.api_key\.autocomplete='new-password'/);
 assert.match(js,/conf\.api_key\.value='';conf\.clear_api_key\.checked=false/);
 assert.ok(!js.includes('innerHTML'),'Help text must not inject HTML');
});
