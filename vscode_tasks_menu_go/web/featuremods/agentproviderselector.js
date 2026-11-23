(function(root){
 'use strict';
 function normalizeProviders(raw){
  if(!Array.isArray(raw))return [];
  const seen=new Set(),items=[];
  for(const entry of raw){
   if(!entry||typeof entry.id!=='string'||!entry.id||seen.has(entry.id))continue;
   seen.add(entry.id);
   const configured=entry.has_api_key===true;
   const models=Array.isArray(entry.models)?entry.models.filter(m=>m&&typeof m.id==='string'&&m.id).map(m=>({
    id:m.id,name:String(m.name||m.id),enabled:configured,reason:configured?'':'API key is not configured',
    toolCalling:Boolean(m.tool_calling),vision:Boolean(m.vision),maxInputTokens:Number(m.max_input_tokens||0)
   })):[];
   items.push({
    id:entry.id,name:String(entry.name||entry.id),enabled:configured&&models.length>0,
    reason:!configured?'API key is not configured':models.length?'':'No models are configured',models
   });
  }
  return items;
 }
 function create(options={}){
  const doc=options.document||root.document;
  if(!doc)throw new Error('A document is required to create the provider selector');
  const providers=normalizeProviders(options.providers),rootNode=doc.createElement('div');
  rootNode.className='agentchat-provider-selector';
  const providerLabel=doc.createElement('label'),providerSelect=doc.createElement('select');
  providerLabel.textContent='Next request provider';
  providerLabel.append(providerSelect);
  const modelLabel=doc.createElement('label'),modelSelect=doc.createElement('select');
  modelLabel.textContent='Model';modelLabel.append(modelSelect);
  const status=doc.createElement('span');status.setAttribute('role','status');status.setAttribute('aria-live','polite');
  rootNode.append(providerLabel,modelLabel,status);
  let selectedProvider=String(options.providerId||''),selectedModel=String(options.modelId||'');
  function currentProvider(){return providers.find(p=>p.id===selectedProvider)||null;}
  function renderModels(){
   const provider=currentProvider();modelSelect.replaceChildren();
   for(const model of provider?.models||[]){
    const option=doc.createElement('option');option.value=model.id;option.textContent=model.name;option.disabled=!model.enabled;modelSelect.append(option);
   }
   if(!(provider?.models||[]).some(m=>m.id===selectedModel&&m.enabled))selectedModel=(provider?.models||[]).find(m=>m.enabled)?.id||'';
   modelSelect.value=selectedModel;
   status.textContent=provider?.enabled?'':(provider?.reason||'Configure an available provider.');
   modelSelect.disabled=!provider?.enabled;
  }
  providerSelect.replaceChildren();
  for(const provider of providers){
   const option=doc.createElement('option');option.value=provider.id;option.textContent=provider.name;
   option.disabled=!provider.enabled;if(provider.reason)option.title=provider.reason;providerSelect.append(option);
  }
  if(!providers.some(p=>p.id===selectedProvider&&p.enabled))selectedProvider=providers.find(p=>p.enabled)?.id||'';
  providerSelect.value=selectedProvider;providerSelect.disabled=!providers.some(p=>p.enabled);
  renderModels();
  function notify(){if(typeof options.onChange==='function'&&selectedProvider&&selectedModel)options.onChange({providerId:selectedProvider,modelId:selectedModel});}
  providerSelect.addEventListener('change',()=>{selectedProvider=providerSelect.value;selectedModel='';renderModels();notify();});
  modelSelect.addEventListener('change',()=>{selectedModel=modelSelect.value;notify();});
  return {
   element:rootNode,
   get value(){return {providerId:selectedProvider,modelId:selectedModel};},
   setDisabled(disabled){providerSelect.disabled=Boolean(disabled)||!providers.some(p=>p.enabled);modelSelect.disabled=Boolean(disabled)||!currentProvider()?.enabled;},
   update(nextProviders,selection={}){
    providers.splice(0,providers.length,...normalizeProviders(nextProviders));
    selectedProvider=String(selection.providerId||selectedProvider);selectedModel=String(selection.modelId||selectedModel);
    providerSelect.replaceChildren();
    for(const provider of providers){const option=doc.createElement('option');option.value=provider.id;option.textContent=provider.name;option.disabled=!provider.enabled;if(provider.reason)option.title=provider.reason;providerSelect.append(option);}
    if(!providers.some(p=>p.id===selectedProvider&&p.enabled))selectedProvider=providers.find(p=>p.enabled)?.id||'';
    providerSelect.value=selectedProvider;providerSelect.disabled=!providers.some(p=>p.enabled);renderModels();
   }
  };
 }
 root.TaskMenuAgentProviderSelector={create,normalizeProviders};
})(globalThis);
