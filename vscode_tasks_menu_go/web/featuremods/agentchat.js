// TaskDeck Agent Chat — independent of suggestion-only Smart Terminal AI.
// Execution is deliberately disabled until the native Tool Gateway is ready.
const agentApp=globalThis.TaskMenuApp;
if(!agentApp)throw new Error('TaskMenuApp unavailable for Agent Chat');
const agentCSS=document.createElement('style');
agentCSS.textContent=[
'.agentchat-overlay{position:fixed;inset:0;z-index:17100;display:none;align-items:center;justify-content:center;background:#000a;padding:15px}',
'.agentchat-overlay.visible{display:flex}',
'.agentchat-window{display:flex;flex-direction:column;width:min(980px,98vw);height:min(820px,94vh);background:#141c27;color:#e6eef9;border:1px solid #55657a;border-radius:12px;box-shadow:0 16px 65px #0009;overflow:hidden}',
'.agentchat-toolbar{padding:10px;display:flex;align-items:center;gap:8px;border-bottom:1px solid #35445c;flex-wrap:wrap}',
'.agentchat-toolbar strong{font-size:15px;margin-right:auto}',
'.agentchat-window button{font-size:12px;padding:7px 10px;border-radius:5px;cursor:pointer}',
'.agentchat-window button:disabled{opacity:.48;cursor:not-allowed}',
'.agentchat-layout{display:flex;flex:1;min-height:0}',
'.agentchat-list{width:190px;min-width:140px;overflow:auto;border-right:1px solid #35445c;padding:8px;display:flex;flex-direction:column;gap:5px}',
'.agentchat-session{display:flex;align-items:center;gap:7px;text-align:left;width:100%;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
'.agentchat-session.selected{border-color:#60a8e6;background:#263f59}',
'.agentchat-main{display:flex;flex:1;min-width:0;min-height:0;flex-direction:column}',
'.agentchat-modelbar{padding:8px;display:flex;align-items:center;gap:7px;border-bottom:1px solid #35445c;flex-wrap:wrap}',
'.agentchat-modelbar select,.agentchat-window textarea,.agentchat-window input{box-sizing:border-box;padding:7px;color:inherit;background:#0f1723;border:1px solid #55667b;border-radius:5px;font:12px ui-monospace,monospace}',
'.agentchat-modelbar select{min-width:110px;max-width:260px}',
'.agentchat-transcript{flex:1;overflow:auto;min-height:0;display:flex;flex-direction:column;gap:9px;padding:12px}',
'.agentchat-bubble{white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px;line-height:1.55;padding:9px;border:1px solid #384b61;border-radius:8px;max-width:94%}',
'.agentchat-bubble.user{align-self:flex-end;background:#203b56}',
'.agentchat-bubble.assistant{align-self:flex-start;background:#222c3a}',
'.agentchat-bubble.tool{align-self:flex-start;background:#443522;border-color:#97744a}',
'.agentchat-tool-actions{display:flex;flex-wrap:wrap;align-items:center;gap:6px;margin-top:8px}',
'.agentchat-tool-code{font:11px/1.5 ui-monospace,monospace;white-space:pre-wrap;overflow-wrap:anywhere;max-height:280px;overflow:auto;border:1px solid #5f6675;padding:8px;border-radius:6px}',
'.agentchat-tool-summary{font-size:11px;color:#c5dbed;line-height:1.4;white-space:pre-wrap;overflow-wrap:anywhere}',

'.agentchat-composer{padding:9px;border-top:1px solid #35445c;display:flex;flex-direction:column;gap:6px}',
'.agentchat-composer textarea{width:100%;min-height:66px;max-height:160px;resize:vertical}',
'.agentchat-composer-actions{display:flex;align-items:center;justify-content:space-between;gap:8px;font-size:11px;flex-wrap:wrap}',
'.agentchat-info{font-size:11px;color:#b6cce2;line-height:1.5;padding:4px 8px}',
'.agentchat-settings-overlay{z-index:17110}',
'.agentchat-settings-box{width:min(730px,97vw);max-height:90vh;overflow:auto;background:#141c27;color:#e6eef9;border:1px solid #55657a;border-radius:12px;padding:16px;display:flex;flex-direction:column;gap:10px}',
'.agentchat-settings-box textarea{width:100%;min-height:250px;resize:vertical;background:#0f1723;border:1px solid #55667b;color:inherit;font:12px/1.5 ui-monospace,monospace;padding:8px;box-sizing:border-box}',
'.agentchat-settings-box input{width:100%;padding:8px;box-sizing:border-box}',
'.agentchat-settings-box .actions{display:flex;gap:8px;justify-content:flex-end}',
'html[data-taskmenu-theme="light"] .agentchat-window,html[data-taskmenu-theme="light"] .agentchat-settings-box{background:#fff;color:#17283a}',
'html[data-taskmenu-theme="light"] .agentchat-window textarea,html[data-taskmenu-theme="light"] .agentchat-window input,html[data-taskmenu-theme="light"] .agentchat-window select,html[data-taskmenu-theme="light"] .agentchat-settings-box textarea,html[data-taskmenu-theme="light"] .agentchat-settings-box input{background:#f7faff;color:#17283a;border-color:#b6c6d7}',
'html[data-taskmenu-theme="light"] .agentchat-bubble.user{background:#d9edfc}',
'html[data-taskmenu-theme="light"] .agentchat-bubble.assistant{background:#eef3f8}',
'html[data-taskmenu-theme="light"] .agentchat-bubble.tool{background:#fff0d8;color:#523815}',
'@media(max-width:630px){.agentchat-list{width:110px;min-width:90px}.agentchat-modelbar select{max-width:155px}.agentchat-overlay{padding:4px}.agentchat-window{height:97vh}}'
].join('\n');document.head.append(agentCSS);
function agentEl(tag,cls,text){
 const node=document.createElement(tag);
 if(cls)node.className=cls;
 if(text!==undefined)node.textContent=text;
 return node;
}
function agentButton(text,fn){const b=agentEl('button','',text);b.type='button';b.onclick=fn;return b;}
function agentOption(value,label){const op=agentEl('option','',label);op.value=value;return op;}
const root=agentEl('div','agentchat-overlay'),windowBox=agentEl('section','agentchat-window');
windowBox.setAttribute('role','dialog');windowBox.setAttribute('aria-label','TaskDeck Agent Chat');
windowBox.setAttribute('aria-modal','true');
const heading=agentEl('div','agentchat-toolbar'),title=agentEl('strong','','TaskDeck AI Agent Chat');
const newChat=agentButton('+ New session',()=>createSession().catch(showAgentError));
const settingsButton=agentButton('Providers / API keys',()=>openAgentSettings().catch(showAgentError));
const refresh=agentButton('Refresh',()=>loadAgentState().catch(showAgentError));
const close=agentButton('✕',()=>closeAgentPanel());
heading.append(title,newChat,settingsButton,refresh,close);
const layout=agentEl('div','agentchat-layout'),sessionList=agentEl('div','agentchat-list');
const main=agentEl('div','agentchat-main'),modelBar=agentEl('div','agentchat-modelbar');
const providerSelect=agentEl('select'),modelSelect=agentEl('select'),modeSelect=agentEl('select');
for(const mode of [['stateless','Stateless'],['previous_response','Response chain'],['conversation','Conversations (fallback)']]){
 modeSelect.append(agentOption(mode[0],mode[1]));
}
modelBar.append(agentEl('span','','Provider'),providerSelect,agentEl('span','','Model'),modelSelect,modeSelect);
const transcript=agentEl('div','agentchat-transcript');
const composer=agentEl('div','agentchat-composer'),prompt=agentEl('textarea');
prompt.setAttribute('placeholder','Type an AI request for this TaskDeck project…');
const actionLine=agentEl('div','agentchat-composer-actions');
const status=agentEl('span','', 'No session selected. Patch execution requires per-call approval.');
const send=agentButton('Send',()=>sendAgentMessage().catch(showAgentError));
const cancel=agentButton('Cancel request',()=>agentAbort?.abort());
cancel.disabled=true;
actionLine.append(status,cancel,send);composer.append(prompt,actionLine);
main.append(modelBar,transcript,composer);layout.append(sessionList,main);
const footer=agentEl('div','agentchat-info','Responses API • AI Pack on first turn • native Patch execution requires explicit approval per tool call. Results are verified from native protocol before AI feedback.');
windowBox.append(heading,layout,footer);root.append(windowBox);document.body.append(root);

const settingsOverlay=agentEl('div','agentchat-overlay agentchat-settings-overlay'),settingsBox=agentEl('section','agentchat-settings-box');
settingsBox.setAttribute('role','dialog');settingsBox.setAttribute('aria-modal','true');settingsBox.setAttribute('aria-label','Agent provider settings');
const settingsTitle=agentEl('h2','','Agent provider settings');
const settingsInfo=agentEl('p','','Configure a public HTTPS Responses-compatible provider. API keys are stored on the TaskDeck server, not in browser storage.');
const settingsPreset=agentButton('Load sample provider',()=>{providerEditor.value=JSON.stringify(agentSample,null,2);});
const settingsPicker=agentEl('select'),providerEditor=agentEl('textarea');
const keyLabel=agentEl('label','','API key (enter only to set/change)'),apiKey=agentEl('input');
apiKey.type='password';apiKey.autocomplete='new-password';apiKey.placeholder='Leave blank to keep existing key';
const settingsStatus=agentEl('div','agentchat-info','');
const settingsActions=agentEl('div','actions');
const saveProvider=agentButton('Save provider',()=>saveAgentProvider().catch(showAgentError));
const testProvider=agentButton('Test connection (uses API)',()=>testAgentProvider().catch(showAgentError));
const closeSettings=agentButton('Close',()=>settingsOverlay.classList.remove('visible'));
settingsActions.append(settingsPreset,saveProvider,testProvider,closeSettings);
settingsBox.append(settingsTitle,settingsInfo,settingsPicker,providerEditor,keyLabel,apiKey,settingsStatus,settingsActions);
settingsOverlay.append(settingsBox);document.body.append(settingsOverlay);

const agentSample={
 id:'custom-r2d',name:'Custom Endpoint',api_type:'responses',base_url:'https://api.r2d.dev/v1',
 models:[
  {id:'composer-2.5',name:'composer-2.5',tool_calling:true,vision:true,max_input_tokens:1280000,max_output_tokens:160000,reasoning_efforts:['low','medium','high'],default_reasoning_effort:'medium'},
  {id:'gpt-6.1-sol',name:'gpt-6.1-sol',tool_calling:true,vision:true,max_input_tokens:1280000,max_output_tokens:160000,reasoning_efforts:['low','medium','high','xhigh','max'],default_reasoning_effort:'xhigh'}
 ]
};
let agentProviders=[],agentSessions=[],agentActive=null,agentBusy=false,agentAbort=null;
function showAgentError(error){
 status.textContent='Error: '+String(error?.message||error);
 settingsStatus.textContent='Error: '+String(error?.message||error);
}
function normalizeProvider(raw){
 const data=raw?.provider||raw;
 if(!data||Array.isArray(data)||typeof data!=='object')throw new Error('Provider must be a JSON object');
 const ms=Array.isArray(data.models)?data.models:[];
 return {
  id:String(data.id||'').trim(),name:String(data.name||'').trim(),
  api_type:String(data.api_type||data.apiType||'responses'),
  base_url:String(data.base_url||data.baseUrl||ms[0]?.url||data.url||'').trim(),
  models:ms.map(m=>({
   id:String(m.id||''),name:String(m.name||m.id||''),
   tool_calling:Boolean(m.tool_calling??m.toolCalling),vision:Boolean(m.vision),
   max_input_tokens:Number(m.max_input_tokens??m.maxInputTokens??0),
   max_output_tokens:Number(m.max_output_tokens??m.maxOutputTokens??0),
   reasoning_efforts:m.reasoning_efforts||m.supportsReasoningEffort||[],
   default_reasoning_effort:m.default_reasoning_effort||m.defaultReasoningEffort||''
  }))
 };
}
async function loadAgentProviders(){
 const result=await agentApp.jsonFetch('/api/agent/providers');
 agentProviders=Array.isArray(result.providers)?result.providers:[];
 providerSelect.replaceChildren();settingsPicker.replaceChildren();
 for(const p of agentProviders){
  providerSelect.append(agentOption(p.id,p.name+' ('+p.id+')'));
  settingsPicker.append(agentOption(p.id,p.name));
 }
 if(!agentProviders.length)providerSelect.append(agentOption('','Configure provider first'));
 updateAgentModels();
 settingsPicker.dispatchEvent(new Event('change'));
}
function updateAgentModels(){
 const p=agentProviders.find(x=>x.id===providerSelect.value);
 modelSelect.replaceChildren();
 for(const m of (p?.models||[]))modelSelect.append(agentOption(m.id,m.name||m.id));
 const hasAgent=Boolean(p&&p.has_api_key);
 newChat.disabled=!hasAgent||agentBusy;
}
providerSelect.addEventListener('change',updateAgentModels);
settingsPicker.addEventListener('change',()=>{
 const p=agentProviders.find(x=>x.id===settingsPicker.value);
 providerEditor.value=JSON.stringify(p?normalizeProvider(p):agentSample,null,2);
 apiKey.value='';settingsStatus.textContent=p?.has_api_key?'API key already configured (write-only).':'API key is not configured.';
});
async function saveAgentProvider(){
 const parsed=normalizeProvider(JSON.parse(providerEditor.value));
 const body={provider:parsed};if(apiKey.value)body.api_key=apiKey.value;
 const data=await agentApp.jsonFetch('/api/agent/providers',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
 apiKey.value='';
 await loadAgentProviders();providerSelect.value=data.provider.id;updateAgentModels();settingsPicker.value=data.provider.id;
 settingsStatus.textContent='Provider saved. Secrets were not returned by the server.';
}
async function testAgentProvider(){
 const parsed=normalizeProvider(JSON.parse(providerEditor.value));
 if(!window.confirm('Send a short test prompt to '+parsed.base_url+' using the saved API key?'))return;
 const p=agentProviders.find(x=>x.id===parsed.id);
 if(!p){settingsStatus.textContent='Save this provider first.';return;}
 const model=p.models?.[0]?.id;
 if(!model){settingsStatus.textContent='No model configured.';return;}
 settingsStatus.textContent='Testing Responses API...';
 const result=await agentApp.jsonFetch('/api/agent/providers/test',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({provider_id:p.id,model_id:model,confirm_external:true})});
 settingsStatus.textContent='Basic Responses text probe: '+(result.ok?'PASS':'FAILED')+
  ' · Tool calling, stateful and streaming are not tested.';
}
async function openAgentSettings(){await loadAgentProviders();settingsOverlay.classList.add('visible');return true;}
function userMessageText(value){
 const text=String(value||'');
 const marker='USER REQUEST:\n';
 const idx=text.lastIndexOf(marker);
 return idx<0?text:text.slice(idx+marker.length);
}
function renderAgentTool(session,item){
 const box=agentEl('div','agentchat-bubble tool');
 const record=(session.tool_calls||[]).find(c=>c.call_id===item.call_id);
 const state=record?.status||'blocked';
 box.append(agentEl('strong','',String(item.name||'Unknown tool')+' · '+state.toUpperCase()));
 if(item.name!=='taskdeck_submit_patch'){
  box.append(agentEl('p','agentchat-tool-summary','Unrecognized function; execution blocked.'));
  return box;
 }
 let args=null;
 try{args=JSON.parse(item.arguments||'');}catch{}
 if(!record||!args||typeof args.patch_python!=='string'){
  box.append(agentEl('p','agentchat-tool-summary','Malformed/unapproved model tool call. Nothing is executable.'));
  return box;
 }
 const details=agentEl('details');
 details.append(agentEl('summary','','Review Python patch · '+String(args.name||'unnamed')));
 details.append(agentEl('pre','agentchat-tool-code',args.patch_python));
 box.append(details);
 if(record.queue_path)box.append(agentEl('p','agentchat-tool-summary','Native queue: '+record.queue_path));
 if(record.patch_session_id)box.append(agentEl('p','agentchat-tool-summary','Patch Session: '+record.patch_session_id));
 if(record.result){
  const result=agentEl('pre','agentchat-tool-code',record.result);
  const summary=agentEl('details');summary.append(agentEl('summary','','Verified tool result / diagnostic'));summary.append(result);box.append(summary);
 }
 const actions=agentEl('div','agentchat-tool-actions');
 if(state==='pending'){
  const approve=agentButton('Approve & run in Patch Tool',()=>agentToolDecision(session,item.call_id,'approve').catch(showAgentError));
  approve.disabled=Boolean(agentApp.sharedMode&&!agentApp.hasPermission?.('patch.run'));
  const reject=agentButton('Reject patch',()=>agentToolDecision(session,item.call_id,'reject').catch(showAgentError));
  actions.append(approve,reject);
 }
 if(state==='running'||state==='dispatching'||state==='queued'){
  const refresh=agentButton('Check native Patch status',()=>syncAgentPatch(session.id).catch(showAgentError));
  refresh.disabled=Boolean(agentApp.sharedMode&&!agentApp.hasPermission?.('patch.view'));
  actions.append(refresh);
 }
 if((state==='done'||state==='error'||state==='rejected')&&!record.feedback_sent){
  const next=agentButton('Send verified result to AI',()=>continueAgentTool(session,item.call_id).catch(showAgentError));
  actions.append(next);
 }
 if(state==='unknown')actions.append(agentEl('p','agentchat-tool-summary','Execution state unknown. Inspect the linked native Patch session; never re-submit blindly.'));
 if(actions.childElementCount)box.append(actions);
 return box;
}
function displayAgentSession(session){
 transcript.replaceChildren();
 if(!session){transcript.append(agentEl('p','','Create or select a session to begin.'));return;}
 for(const item of (session.history||[])){
  if(item.type==='user')transcript.append(agentEl('div','agentchat-bubble user',userMessageText(item.content)));
  if(item.type==='assistant')transcript.append(agentEl('div','agentchat-bubble assistant',item.content));
  if(item.type==='function_call')transcript.append(renderAgentTool(session,item));
  if(item.type==='function_call_output')transcript.append(agentEl('div','agentchat-bubble tool','Native Patch result sent to AI: '+String(item.call_id||'')));
 }
 transcript.scrollTop=transcript.scrollHeight;
 status.textContent='Session '+session.id.slice(0,8)+' · '+session.status+' · revision '+session.revision;
}
async function loadAgentSessions(){
 const response=await agentApp.jsonFetch('/api/agent/sessions');
 agentSessions=Array.isArray(response.sessions)?response.sessions:[];
 sessionList.replaceChildren();
 for(const sess of agentSessions){
  const item=agentButton(sess.model_id+' · '+sess.status,()=>selectAgentSession(sess.id).catch(showAgentError));
  item.className='agentchat-session'+(sess.id===agentActive?' selected':'');
  item.title=sess.id;
  sessionList.append(item);
 }
 if(!agentSessions.length)sessionList.append(agentEl('p','agentchat-info','No saved chat sessions.'));
}
async function selectAgentSession(id){
 const data=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(id));
 agentActive=id;
 const sess=data.session;
 const p=agentProviders.find(p=>p.id===sess.provider_id);
 if(p){providerSelect.value=p.id;updateAgentModels();modelSelect.value=sess.model_id;}
 modeSelect.value=sess.mode;
 displayAgentSession(sess);
 await loadAgentSessions();
 return sess;
}
async function agentToolDecision(session,callID,action){
 if(agentBusy)return;
 const review=(session.history||[]).find(item=>item.type==='function_call'&&item.call_id===callID);
 if(!review)throw new Error('Missing original tool call');
 if(action==='approve'){
  if(!window.confirm('Approve this exact Python patch for execution by the native TaskDeck Patch Tool? Review the code above first. Other queue items will NOT be selected.'))return;
 }else if(!window.confirm('Reject this patch without queueing or executing it?'))return;
 agentBusy=true;
 try{
  const result=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(session.id)+'/tools/'+encodeURIComponent(callID)+'/decision',{
   method:'POST',headers:{'Content-Type':'application/json'},
   body:JSON.stringify({action,expected_revision:session.revision,confirm_execution:action==='approve'}),
  });
  await selectAgentSession(session.id);
  status.textContent=action==='approve'
   ?('Native Patch '+result.status+' · '+(result.note||'Inspect Patch Tool for execution progress.'))
   :'Patch was rejected; nothing executed.';
 }finally{agentBusy=false;}
}
async function syncAgentPatch(id){
 if(agentBusy)return;
 const result=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(id)+'/sync',{
  method:'POST',headers:{'Content-Type':'application/json'},body:'{}',
 });
 await selectAgentSession(id);
 return result;
}
async function continueAgentTool(session,callID){
 if(agentBusy)return;
 if(!window.confirm('Send the verified native Patch execution result back to the external AI provider to continue this conversation?'))return;
 agentBusy=true;send.disabled=true;newChat.disabled=true;
 agentAbort=new AbortController();cancel.disabled=false;
 try{
  const result=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(session.id)+'/tools/'+encodeURIComponent(callID)+'/continue',{
   method:'POST',headers:{'Content-Type':'application/json'},
   body:JSON.stringify({expected_revision:session.revision,confirm_external:true}),signal:agentAbort.signal,
  });
  await selectAgentSession(session.id);
  status.textContent=result.status==='waiting_tool'
   ?'AI proposed another patch; review and approve separately.'
   :'AI has received the verified native Patch result.';
 }finally{
  agentAbort=null;agentBusy=false;send.disabled=false;newChat.disabled=false;cancel.disabled=true;updateAgentModels();
 }
}
async function createSession(){
 if(agentBusy)return;
 if(!providerSelect.value||!modelSelect.value)throw new Error('Configure provider, API key and model before creating a session.');
 const result=await agentApp.jsonFetch('/api/agent/sessions',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({provider_id:providerSelect.value,model_id:modelSelect.value,mode:modeSelect.value})});
 await loadAgentSessions();
 await selectAgentSession(result.session.id);
 prompt.focus();
}
async function sendAgentMessage(){
 if(agentBusy)return;
 const msg=prompt.value.trim();
 if(!agentActive)throw new Error('Create a session first.');
 if(!msg)throw new Error('Enter a message.');
 if(!window.confirm('Send this request and, on the first turn, the TaskDeck AI Pack to the selected external provider? This chat may propose patch execution, which requires a separate approval.'))return;
 const s=await selectAgentSession(agentActive);
 agentBusy=true;send.disabled=true;newChat.disabled=true;cancel.disabled=false;
 agentAbort=new AbortController();
 try{
  status.textContent='Waiting for AI provider…';
  const response=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(agentActive)+'/messages',{
   method:'POST',headers:{'Content-Type':'application/json'},
   body:JSON.stringify({message:msg,confirm_external:true,expected_revision:s.revision}),
   signal:agentAbort.signal
  });
  prompt.value='';
  await selectAgentSession(agentActive);
  status.textContent=response.status==='waiting_tool'?'AI requested a patch; review/approve explicitly.':
   'Response received. No commands executed.';
 }finally{
  agentAbort=null;agentBusy=false;send.disabled=false;cancel.disabled=true;
  updateAgentModels();
 }
}
async function loadAgentState(){
 await loadAgentProviders();await loadAgentSessions();
 if(agentActive&&agentSessions.some(s=>s.id===agentActive))await selectAgentSession(agentActive);
 else if(agentSessions.length)await selectAgentSession(agentSessions[0].id);
 else displayAgentSession(null);
}
async function openAgentPanel(){
 root.classList.add('visible');
 await loadAgentState();
 return true;
}
function closeAgentPanel(){root.classList.remove('visible');}
root.addEventListener('click',e=>{if(e.target===root)closeAgentPanel();});
settingsOverlay.addEventListener('click',e=>{if(e.target===settingsOverlay)settingsOverlay.classList.remove('visible');});
window.addEventListener('keydown',e=>{
 if(e.key==='Escape'){
  if(settingsOverlay.classList.contains('visible'))settingsOverlay.classList.remove('visible');
  else if(root.classList.contains('visible'))closeAgentPanel();
 }
});
globalThis.TaskMenuAgentChat={open:openAgentPanel,close:closeAgentPanel,settings:openAgentSettings,
 get visible(){return root.classList.contains('visible');}};
