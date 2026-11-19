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
'.agentchat-tab{padding:5px 9px;font-size:11px;border-radius:14px;border:1px solid #4a5f78;cursor:pointer}',
'.agentchat-tab.active{background:#2c4a67;border-color:#7db6e6}',
'.agentchat-tab .close{padding:0 4px;border:0;background:transparent;color:#cfe3f5}',
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
const layout=agentEl('div','agentchat-layout'),sessionList=agentEl('div','agentchat-list');
const main=agentEl('div','agentchat-main'),modelBar=agentEl('div','agentchat-modelbar');
// Open tabs are UI-only: the server remains the source of truth for every
// session. Each tab keeps its own composer draft.
const tabbar=agentEl('div','agentchat-modelbar');
let agentTabs=[];const agentDrafts=new Map();
const providerSelect=agentEl('select'),modelSelect=agentEl('select'),modeSelect=agentEl('select'),effortSelect=agentEl('select');
for(const mode of [['stateless','Stateless'],['previous_response','Response chain'],['conversation','Conversations (fallback)']]){
 modeSelect.append(agentOption(mode[0],mode[1]));
}
modelBar.append(agentEl('span','','Provider'),providerSelect,agentEl('span','','Model'),modelSelect,modeSelect,agentEl('span','','Effort'),effortSelect);
const transcript=agentEl('div','agentchat-transcript');
const composer=agentEl('div','agentchat-composer'),prompt=agentEl('textarea');
// Text-only attachment staging. Binary files are rejected server-side; the UI
// never reads local paths or embeds base64.
const attachButton=agentButton('+ Text file',()=>pickAgentAttachment().catch(showAgentError));
const autoFeedback=agentButton('Auto-send results',()=>toggleAutoFeedback().catch(showAgentError));
const attachList=agentEl('div','agentchat-info','');
let agentAttachments=[];
prompt.setAttribute('placeholder','Type an AI request for this TaskDeck project…');
const actionLine=agentEl('div','agentchat-composer-actions');
const status=agentEl('span','', 'No session selected. Patch execution requires per-call approval.');
const send=agentButton('Send',()=>sendAgentMessage().catch(showAgentError));
const cancel=agentButton('Cancel request',()=>cancelAgentTurn().catch(showAgentError));
cancel.disabled=true;
const preview=agentButton('Preview context',()=>previewAgentContext().catch(showAgentError));
const rename=agentButton('Rename',()=>renameAgentSession().catch(showAgentError));
const archive=agentButton('Archive',()=>archiveAgentSession().catch(showAgentError));
rename.disabled=true;archive.disabled=true;
heading.append(title,newChat,preview,rename,archive,settingsButton,refresh,close);
actionLine.append(attachButton,autoFeedback,status,cancel,send);composer.append(attachList,prompt,actionLine);
main.append(tabbar,modelBar,transcript,composer);layout.append(sessionList,main);
const footer=agentEl('div','agentchat-info','Responses API • AI Pack on first turn • native Patch execution requires explicit approval per tool call. Results are verified from native protocol before AI feedback.');
windowBox.append(heading,layout,footer);root.append(windowBox);document.body.append(root);

const settingsOverlay=agentEl('div','agentchat-overlay agentchat-settings-overlay'),settingsBox=agentEl('section','agentchat-settings-box');
settingsBox.setAttribute('role','dialog');settingsBox.setAttribute('aria-modal','true');settingsBox.setAttribute('aria-label','Agent provider settings');
const settingsTitle=agentEl('h2','','Agent provider settings');
const settingsInfo=agentEl('p','','Configure a public HTTPS Responses-compatible provider. API keys are stored on the TaskDeck server, not in browser storage.');
const settingsPreset=agentButton('Load sample provider',()=>{providerEditor.value=JSON.stringify(agentSample,null,2);});
const probeProvider=agentButton('Probe capabilities',()=>probeAgentProvider().catch(showAgentError));
const deleteProvider=agentButton('Remove provider',()=>deleteAgentProvider().catch(showAgentError));
const settingsPicker=agentEl('select'),providerEditor=agentEl('textarea');
const keyLabel=agentEl('label','','API key (enter only to set/change)'),apiKey=agentEl('input');
apiKey.type='password';apiKey.autocomplete='new-password';apiKey.placeholder='Leave blank to keep existing key';
const settingsStatus=agentEl('div','agentchat-info','');
const settingsActions=agentEl('div','actions');
const saveProvider=agentButton('Save provider',()=>saveAgentProvider().catch(showAgentError));
const testProvider=agentButton('Test connection (uses API)',()=>testAgentProvider().catch(showAgentError));
const closeSettings=agentButton('Close',()=>settingsOverlay.classList.remove('visible'));
settingsActions.append(settingsPreset,saveProvider,testProvider,probeProvider,deleteProvider,closeSettings);
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
// Tracks which session owns the in-flight UI request so another session can
// still be opened/read while one provider turn is pending.
const agentBusySessions=new Set();
function agentSessionBusy(id){return Boolean(id&&agentBusySessions.has(id));}
function markAgentSessionBusy(id,busy){
 if(!id)return;
 if(busy)agentBusySessions.add(id);else agentBusySessions.delete(id);
 if(id===agentActive){
  send.disabled=busy;newChat.disabled=busy;rename.disabled=busy;archive.disabled=busy;
  cancel.disabled=!busy;updateAgentModels();
 }
}
let agentMonitorTimer=0;
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
  stream:Boolean(data.stream),
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
 const current=(p?.models||[]).find(m=>m.id===modelSelect.value)||(p?.models||[])[0];
 effortSelect.replaceChildren(agentOption('','Model default'));
 for(const effort of (current?.reasoning_efforts||[]))effortSelect.append(agentOption(effort,effort));
 effortSelect.value=(current?.default_reasoning_effort||'');
 const hasAgent=Boolean(p&&p.has_api_key);
 newChat.disabled=!hasAgent||agentBusy;
 attachButton.disabled=false;
}
providerSelect.addEventListener('change',updateAgentModels);
modelSelect.addEventListener('change',()=>{
 const p=agentProviders.find(x=>x.id===providerSelect.value);
 const m=(p?.models||[]).find(x=>x.id===modelSelect.value);
 effortSelect.replaceChildren(agentOption('','Model default'));
 for(const effort of (m?.reasoning_efforts||[]))effortSelect.append(agentOption(effort,effort));
 effortSelect.value=(m?.default_reasoning_effort||'');
});
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
  ' · This does not verify tool calling or stateful chaining; use Probe capabilities.';
}
async function probeAgentProvider(){
 const parsed=normalizeProvider(JSON.parse(providerEditor.value));
 const p=agentProviders.find(x=>x.id===parsed.id);
 if(!p){settingsStatus.textContent='Save this provider first.';return;}
 const model=p.models?.[0]?.id;
 if(!model){settingsStatus.textContent='No model configured.';return;}
 if(!window.confirm('Run harmless capability probes against '+parsed.base_url+'? This sends no project data and can incur provider cost.'))return;
 settingsStatus.textContent='Probing text, tool schema, response chaining'+(parsed.stream?' and streaming':'')+'...';
 const result=await agentApp.jsonFetch('/api/agent/providers/probe',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({provider_id:p.id,model_id:model,confirm_external:true,
   capabilities:parsed.stream?['text','tool_calling','previous_response','streaming']:['text','tool_calling','previous_response']})});
 const caps=result.capabilities||{};
 settingsStatus.textContent='Probe evidence: '+Object.entries(caps).map(([k,v])=>k+'='+v).join(' · ')+
  ' · Tool execution stays disabled unless tool_calling=verified.';
}
async function deleteAgentProvider(){
 const parsed=normalizeProvider(JSON.parse(providerEditor.value));
 const p=agentProviders.find(x=>x.id===parsed.id);
 if(!p){settingsStatus.textContent='Save this provider first.';return;}
 if(!window.confirm('Remove provider metadata for '+p.id+'? This does not revoke the key at the provider.'))return;
 const removeKey=window.confirm('Also delete the locally stored API key for this provider?\nCancel keeps the key.');
 const result=await agentApp.jsonFetch('/api/agent/providers',{method:'DELETE',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({provider_id:p.id,delete_key:removeKey})});
 await loadAgentProviders();
 settingsStatus.textContent=(result.note||'Provider removed.')+(result.partial?' PARTIAL: retry.':'');
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
  const consoleButton=agentButton('View console',()=>showAgentExecution(session.id,item.call_id).catch(showAgentError));
  consoleButton.disabled=refresh.disabled;
  actions.append(refresh,consoleButton);
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
 renderAgentTabs();
 status.textContent='Session '+session.id.slice(0,8)+' · '+session.status+' · revision '+session.revision+
  (session.auto_feedback?' · auto-send ON':'');
 autoFeedback.textContent=session.auto_feedback?'Auto-send: ON':'Auto-send results';
}
async function loadAgentSessions(){
 const response=await agentApp.jsonFetch('/api/agent/sessions');
 agentSessions=Array.isArray(response.sessions)?response.sessions:[];
 sessionList.replaceChildren();
 for(const sess of agentSessions){
  const label=(sess.archived?'[archived] ':'')+(sess.title||sess.model_id)+' · '+sess.status;
  const item=agentButton(label,()=>selectAgentSession(sess.id).catch(showAgentError));
  item.className='agentchat-session'+(sess.id===agentActive?' selected':'');
  item.title=sess.id;
  sessionList.append(item);
 }
 if(!agentSessions.length)sessionList.append(agentEl('p','agentchat-info','No saved chat sessions.'));
}
function renderAgentTabs(){
 tabbar.replaceChildren();
 if(!agentTabs.length){tabbar.append(agentEl('span','agentchat-info','No open tabs'));return;}
 for(const id of agentTabs){
  const sess=agentSessions.find(s=>s.id===id);
  const label=(sess?.title||sess?.model_id||id.slice(0,8));
  const tab=agentEl('span','agentchat-tab'+(id===agentActive?' active':''));
  const open=agentButton(String(label),()=>selectAgentSession(id).catch(showAgentError));
  open.className='';
  open.style.padding='0';
  open.style.border='0';
  open.style.background='transparent';
  const close=agentButton('✕',()=>closeAgentTab(id));
  close.className='close';
  tab.append(open,close);
  tabbar.append(tab);
 }
}
function openAgentTab(id){
 if(!agentTabs.includes(id))agentTabs.push(id);
 renderAgentTabs();
}
function closeAgentTab(id){
 agentTabs=agentTabs.filter(x=>x!==id);
 agentDrafts.delete(id);
 if(agentActive===id){
  agentActive=null;
  if(agentTabs.length)selectAgentSession(agentTabs[agentTabs.length-1]).catch(showAgentError);
  else displayAgentSession(null);
 }
 renderAgentTabs();
}
async function selectAgentSession(id){
 const data=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(id));
 agentActive=id;
 const sess=data.session;
 if(agentActive&&agentActive!==id)agentDrafts.set(agentActive,prompt.value);
 openAgentTab(id);
 prompt.value=agentDrafts.get(id)||prompt.value;
 // After a refresh/reconnect, an in-flight provider turn has no browser
 // request attached. Reflect it instead of offering a duplicate send.
 if(sess.status==='requesting_ai'&&!agentSessionBusy(id)){
  status.textContent='A model request is still running on the server. Use Cancel if you want to stop waiting.';
  send.disabled=true;cancel.disabled=false;
 }
 const p=agentProviders.find(p=>p.id===sess.provider_id);
 if(p){providerSelect.value=p.id;updateAgentModels();modelSelect.value=sess.model_id;}
 modeSelect.value=sess.mode;
 effortSelect.value=sess.reasoning_effort||'';
 displayAgentSession(sess);
 scheduleAgentMonitor(sess);
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
async function showAgentExecution(sessionID,callID){
 const data=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(sessionID)+'/tools/'+encodeURIComponent(callID)+'/execution');
 const exec=data.execution||{};
 const lines=[];
 lines.push('Status: '+String(exec.status||'unknown')+(exec.native_status?' (native '+exec.native_status+')':''));
 if(exec.run_id||exec.native_run_id)lines.push('Run: '+String(exec.native_run_id||exec.run_id));
 if(exec.feedback_status)lines.push('Result delivery: '+String(exec.feedback_status));
 for(const c of (exec.commands||[]))lines.push('  ['+String(c.index)+'] '+String(c.stage||'')+' '+String(c.status||'')+(c.rc!==undefined?' rc='+c.rc:''));
 if((exec.artifacts||[]).length)lines.push('Artifacts: '+exec.artifacts.map(a=>String(a.kind)+(a.primary?'*':'')).join(', '));
 if(exec.stdout_tail)lines.push('--- output tail ---\n'+String(exec.stdout_tail));
 const text=lines.join('\n');
 const dialog=agentEl('div','agentchat-bubble tool');
 dialog.append(agentEl('strong','','Native execution evidence (copy is opt-in)'));
 dialog.append(agentEl('pre','agentchat-tool-code',text));
 const copy=agentButton('Copy evidence',()=>{
  if(navigator.clipboard) navigator.clipboard.writeText(text).catch(()=>{});
 });
 dialog.append(copy);
 transcript.append(dialog);
 transcript.scrollTop=transcript.scrollHeight;
}
async function syncAgentPatch(id){
 if(agentSessionBusy(id))return;
 const result=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(id)+'/sync',{
  method:'POST',headers:{'Content-Type':'application/json'},body:'{}',
 });
 await selectAgentSession(id);
 return result;
}
async function continueAgentTool(session,callID){
 if(agentSessionBusy(session.id))return;
 if(!window.confirm('Send the verified native Patch execution result back to the external AI provider to continue this conversation?'))return;
 agentBusy=true;markAgentSessionBusy(session.id,true);
 agentAbort=new AbortController();
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
  agentAbort=null;agentBusy=false;markAgentSessionBusy(session.id,false);
 }
}
// Resync transition journal after a reconnect, without replaying prompts.
async function syncAgentEvents(sessionID){
 if(agentSessionBusy(sessionID))return;
 try{
  const data=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(sessionID)+'/events');
  if(sessionID!==agentActive)return;
  const last=(data.events||[]).slice(-1)[0];
  if(last&&['tool_final','turn_finished','turn_failed','turn_cancelled'].includes(last.kind)){
   await selectAgentSession(sessionID);
  }
 }catch{}
}
function scheduleAgentMonitor(sess){
 clearTimeout(agentMonitorTimer);agentMonitorTimer=0;
 if(!root.classList.contains('visible')||!sess)return;
 if(agentApp.sharedMode&&!agentApp.hasPermission?.('patch.view'))return;
 if(!(sess.tool_calls||[]).some(c=>c.status==='running'))return;
 const id=sess.id;
 agentMonitorTimer=setTimeout(async()=>{
  if(agentActive!==id||!root.classList.contains('visible'))return;
  await syncAgentEvents(id);
  if(agentBusy){scheduleAgentMonitor(sess);return;}
  try{await syncAgentPatch(id);}
  catch(error){
   status.textContent='Native Patch polling failed: '+String(error?.message||error);
   agentMonitorTimer=setTimeout(()=>selectAgentSession(id).catch(showAgentError),5000);
  }
 },2800);
}
async function createSession(){
 if(agentBusy)return;
 if(!providerSelect.value||!modelSelect.value)throw new Error('Configure provider, API key and model before creating a session.');
 const result=await agentApp.jsonFetch('/api/agent/sessions',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({provider_id:providerSelect.value,model_id:modelSelect.value,mode:modeSelect.value,
   reasoning_effort:effortSelect.value})});
 await loadAgentSessions();
 await selectAgentSession(result.session.id);
 prompt.focus();
}
async function sendAgentMessage(){
 if(!agentActive)throw new Error('Create a session first.');
 if(agentSessionBusy(agentActive))return;
 const msg=prompt.value.trim();
 if(!msg)throw new Error('Enter a message.');
 if(!window.confirm('Send this request and, on the first turn, the TaskDeck AI Pack to the selected external provider? This chat may propose patch execution, which requires a separate approval.'))return;
 const s=await selectAgentSession(agentActive);
 const sessionID=agentActive;
 agentBusy=true;markAgentSessionBusy(sessionID,true);
 agentAbort=new AbortController();
 try{
  status.textContent='Waiting for AI provider…';
  // A stable per-composition key makes an accidental double submit a replay,
  // not a second provider turn.
  const requestID='ui-'+sessionID+'-'+Date.now().toString(36)+'-'+Math.random().toString(36).slice(2,8);
  const response=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(sessionID)+'/messages',{
   method:'POST',headers:{'Content-Type':'application/json'},
   body:JSON.stringify({message:msg,confirm_external:true,expected_revision:s.revision,request_id:requestID,
    attachments:agentAttachments}),
   signal:agentAbort.signal
  });
  prompt.value='';agentAttachments=[];renderAgentAttachments();
  if(agentActive===sessionID)await selectAgentSession(sessionID);
  status.textContent=response.status==='waiting_tool'?'AI requested a patch; review/approve explicitly.':
   'Response received. No commands executed.';
 }finally{
  agentAbort=null;agentBusy=false;markAgentSessionBusy(sessionID,false);
 }
}
function pickAgentAttachment(){
 const input=document.createElement('input');
 input.type='file';input.accept='.txt,.md,.json,.log,.csv,.yaml,.yml,.ini,.sql,.py,.js,.ts,.go,.sh,.c,.h,.cpp,.java,.rb,.php,.html,.css,.xml';
 input.onchange=()=>{
  const file=input.files&&input.files[0];
  if(!file)return;
  if(file.size>32*1024){showAgentError(new Error('Attachment must be at most 32 KiB of text.'));return;}
  if(agentAttachments.length>=4){showAgentError(new Error('At most 4 attachments.'));return;}
  const reader=new FileReader();
  reader.onload=()=>{
   agentAttachments.push({name:file.name,mime:file.type&&file.type.startsWith('text/')?file.type:'text/plain',text:String(reader.result||'')});
   renderAgentAttachments();
  };
  reader.onerror=()=>showAgentError(new Error('Cannot read attachment.'));
  reader.readAsText(file);
 };
 input.click();
}
function renderAgentAttachments(){
 if(!agentAttachments.length){attachList.textContent='';return;}
 attachList.textContent='Attachments (sent to the provider with consent): '+agentAttachments.map(a=>a.name).join(', ');
}
async function toggleAutoFeedback(){
 if(!agentActive)return;
 const session=await selectAgentSession(agentActive);
 const enable=!session.auto_feedback;
 if(enable){
  if(!window.confirm('Automatically send each VERIFIED native Patch result to the external provider for the next 30 minutes?\n\nThis never approves or runs a patch: you must still approve every proposal. You can turn this off at any time.'))return;
 }
 const body={enabled:enable,expected_revision:session.revision};
 if(enable)body.confirm_external=true;
 const result=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(session.id)+'/auto-feedback',{
  method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body),
 });
 await selectAgentSession(session.id);
 status.textContent=result.auto_feedback
  ?'Auto-send enabled until '+new Date(result.auto_feedback_until).toLocaleTimeString()+'. Patch execution still requires approval.'
  :'Auto-send disabled.';
}
async function previewAgentContext(){
 if(!agentActive)return;
 const id=agentActive;
 const data=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(id)+'/pack-preview');
 const box=agentEl('div','agentchat-bubble tool');
 box.append(agentEl('strong','','AI Pack preview · no request sent'));
 box.append(agentEl('p','agentchat-tool-summary',
  'Fingerprint '+String(data.fingerprint||'?')+' · '+String(data.pack_bytes||0)+' bytes · redaction '+String(data.redaction||'best_effort')+
  (data.oversize?' · OVERSIZE (the first send will be refused)':'')));
 box.append(agentEl('pre','agentchat-tool-code',String(data.preview||'')));
 transcript.append(box);transcript.scrollTop=transcript.scrollHeight;
 status.textContent=data.note||'Preview only.';
}
async function cancelAgentTurn(){
 if(!agentActive)return;
 const id=agentActive;
 try{
  const result=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(id)+'/cancel',{
   method:'POST',headers:{'Content-Type':'application/json'},body:'{}',
  });
  status.textContent=result.note||(result.cancelled?'Request cancelled.':'Nothing to cancel.');
  // A server-side cancel is authoritative; the browser abort only stops the
  // local fetch from lingering.
  try{agentAbort?.abort();}catch{}
  await selectAgentSession(id);
 }catch(error){
  status.textContent='Cancel request failed: '+String(error?.message||error);
  throw error;
 }
}
async function renameAgentSession(){
 if(!agentActive)return;
 const session=await selectAgentSession(agentActive);
 const next=window.prompt('Session title',session.title||'');
 if(next===null)return;
 const result=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(session.id)+'/metadata',{
  method:'PATCH',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({title:String(next),expected_revision:session.revision}),
 });
 await loadAgentSessions();
 await selectAgentSession(result.session.id);
}
async function archiveAgentSession(){
 if(!agentActive)return;
 const session=await selectAgentSession(agentActive);
 const restore=Boolean(session.archived);
 if(!window.confirm(restore?'Restore this archived session?':'Archive this session? Unresolved patch calls must be reconciled first.'))return;
 const result=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(session.id)+'/metadata',{
  method:'PATCH',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({archived:!restore,expected_revision:session.revision}),
 });
 await loadAgentSessions();
 await selectAgentSession(result.session.id);
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
function closeAgentPanel(){clearTimeout(agentMonitorTimer);agentMonitorTimer=0;root.classList.remove('visible');}
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
