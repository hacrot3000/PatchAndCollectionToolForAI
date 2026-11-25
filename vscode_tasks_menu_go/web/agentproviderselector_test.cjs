'use strict';
const test=require('node:test');
const assert=require('node:assert/strict');
require('./featuremods/agentproviderselector.js');
const selector=globalThis.TaskMenuAgentProviderSelector;

test('provider choices preserve stable IDs and disable unavailable configurations',()=>{
 const choices=selector.normalizeProviders([
  {id:'composer',name:'Composer',has_api_key:true,models:[{id:'composer-2.5',name:'Composer 2.5',tool_calling:true}]},
  {id:'gpt',name:'GPT',has_api_key:false,models:[{id:'gpt-6.1-sol'}]},
  {id:'empty',name:'Empty',has_api_key:true,models:[]},
  {id:'composer',name:'Duplicate',has_api_key:true,models:[{id:'duplicate'}]},
 ]);
 assert.deepEqual(choices.map(item=>item.id),['composer','gpt','empty']);
 assert.equal(choices[0].enabled,true);
 assert.equal(choices[1].enabled,false);
 assert.equal(choices[1].reason,'API key is not configured');
 assert.equal(choices[2].enabled,false);
 assert.equal(choices[2].reason,'No models are configured');
 assert.equal(choices[0].models[0].toolCalling,true);
});

test('provider normalization does not retain credential-shaped fields',()=>{
 const choices=selector.normalizeProviders([{id:'p',name:'Provider',has_api_key:true,api_key:'must-not-copy',models:[{id:'m'}]}]);
 assert.equal(Object.hasOwn(choices[0],'api_key'),false);
 assert.equal(Object.hasOwn(choices[0],'credential'),false);
});

class FakeElement {
 constructor(tag){this.tag=tag;this.children=[];this.handlers={};this.value='';this.disabled=false;this.attributes={};}
 append(...children){this.children.push(...children);}
 replaceChildren(...children){this.children=[...children];}
 addEventListener(name,callback){this.handlers[name]=callback;}
 setAttribute(name,value){this.attributes[name]=value;}
}

test('selector keeps independent provider/model choice and emits the next-run binding',()=>{
 const document={createElement:tag=>new FakeElement(tag)};
 const changes=[];
 const component=selector.create({document,providers:[
  {id:'composer',name:'Composer',has_api_key:true,models:[{id:'composer-2.5'}]},
  {id:'gpt',name:'GPT',has_api_key:true,models:[{id:'gpt-fast'},{id:'gpt-reasoning'}]},
 ],providerId:'composer',modelId:'composer-2.5',onChange:binding=>changes.push(binding)});
 const providerSelect=component.element.children[0].children[0];
 const modelSelect=component.element.children[1].children[0];
 assert.deepEqual(component.value,{providerId:'composer',modelId:'composer-2.5'});
 providerSelect.value='gpt';providerSelect.handlers.change();
 assert.deepEqual(component.value,{providerId:'gpt',modelId:'gpt-fast'});
 modelSelect.value='gpt-reasoning';modelSelect.handlers.change();
 assert.deepEqual(changes,[
  {providerId:'gpt',modelId:'gpt-fast'},
  {providerId:'gpt',modelId:'gpt-reasoning'},
 ]);
});
