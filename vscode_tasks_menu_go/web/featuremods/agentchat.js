// TaskDeck Agent Chat — independent of suggestion-only Smart Terminal AI.
// Native Patch execution remains explicit-approval gated; text-only models may propose reviewable Patch JSON.
const agentApp=globalThis.TaskMenuApp;
if(!agentApp)throw new Error('TaskMenuApp unavailable for Agent Chat');
const agentCSS=document.createElement('style');
agentCSS.textContent=[
'.agentchat-overlay{display:none;min-width:0;min-height:0;overflow:hidden;background:#141c27;color:#e6eef9;border:1px solid #55657a}',
'.agentchat-overlay.visible{display:flex}',
'.agentchat-window{display:flex;flex:1;min-width:0;min-height:0;flex-direction:column;overflow:hidden}',
'.agentchat-toolbar{position:relative;z-index:4;min-height:44px;padding:5px 7px;display:flex;align-items:center;gap:5px;border-bottom:1px solid #35445c;flex-wrap:nowrap}',
'.agentchat-toolbar strong{font-size:13px;white-space:nowrap}',
'.agentchat-toolbar-actions{display:flex;align-items:center;gap:4px;min-width:0;overflow:hidden}',
'.agentchat-toolbar button{white-space:nowrap;padding:5px 7px}',
'.agentchat-more-menu{position:absolute;right:6px;top:calc(100% + 4px);z-index:20;display:none;min-width:190px;max-width:min(300px,90vw);max-height:70vh;overflow:auto;padding:5px;background:#192330;border:1px solid #55657a;border-radius:7px;box-shadow:0 8px 28px #0009}',
'.agentchat-more-menu.open{display:flex;flex-direction:column;align-items:stretch;gap:4px}',
'.agentchat-more-menu button{text-align:left}',
'.agentchat-view-tabs{position:relative;z-index:3;display:flex;flex:none;align-items:center;gap:4px;padding:4px 7px;border-bottom:1px solid #35445c}',
'.agentchat-view-tabs button[aria-selected="true"]{background:#344b66;border-color:#7db6e6}',
'.agentchat-debug-panel{display:none;flex:1;min-width:0;min-height:0;flex-direction:column;overflow:hidden}',
'.agentchat-debug-panel.visible{display:flex}',
'.agentchat-debug-toolbar,.agentchat-debug-tabs{display:flex;flex:none;align-items:center;gap:5px;overflow:auto;padding:6px;border-bottom:1px solid #35445c}',
'.agentchat-debug-toolbar select{min-width:120px;max-width:240px}',
'.agentchat-debug-output{flex:1;min-width:0;min-height:0;overflow:auto;margin:0;padding:10px;white-space:pre-wrap;overflow-wrap:anywhere;font:11px/1.5 ui-monospace,monospace}',
'.task-agent-dock-section{display:grid;grid-template-columns:minmax(0,1fr) 0 0;grid-template-rows:minmax(0,1fr);position:relative;overflow:hidden}',
'.task-agent-dock-section.agent-dock-right{grid-template-columns:minmax(0,1fr) 6px var(--agent-dock-size,420px)}',
'.task-agent-dock-section.agent-dock-bottom{grid-template-columns:minmax(0,1fr);grid-template-rows:minmax(0,1fr) 6px var(--agent-dock-size,320px)}',
'.task-agent-content{display:flex;min-width:0;min-height:0;flex-direction:column;overflow:hidden}',
'.task-agent-content>#tabs{flex:none}.task-agent-content>#panes{flex:1;min-height:0}',
'.task-agent-dock-section>.agentchat-overlay{grid-column:3;grid-row:1}',
'.task-agent-dock-section.agent-dock-bottom>.agentchat-overlay{grid-column:1;grid-row:3}',
'.agentchat-dock-resizer{display:none;touch-action:none;z-index:2;background:transparent}',
'.agent-dock-right>.agentchat-dock-resizer{display:block;grid-column:2;grid-row:1;cursor:col-resize}',
'.agent-dock-bottom>.agentchat-dock-resizer{display:block;grid-column:1;grid-row:2;cursor:row-resize}',
'.agentchat-strip{display:none;position:absolute;z-index:10;align-items:center;justify-content:center;background:#192330;border:1px solid #55657a;color:inherit;cursor:pointer}',
'.agent-dock-collapsed.agent-dock-right>.agentchat-strip{display:flex;right:0;top:0;bottom:0;width:34px;writing-mode:vertical-rl}',
'.agent-dock-collapsed.agent-dock-bottom>.agentchat-strip{display:flex;left:0;right:0;bottom:0;height:34px}',
'.agent-dock-collapsed.agent-dock-right{grid-template-columns:minmax(0,1fr) 34px!important}',
'.agent-dock-collapsed.agent-dock-bottom{grid-template-rows:minmax(0,1fr) 34px!important}',
'.agent-dock-collapsed>.agentchat-dock-resizer{display:none}',
'.agent-dock-floating.agent-dock-right{grid-template-columns:minmax(0,1fr)!important}',
'.agent-dock-floating.agent-dock-bottom,.agent-dock-responsive.agent-dock-bottom{grid-template-rows:minmax(0,1fr)!important}',
'.agent-dock-responsive.agent-dock-right{grid-template-columns:minmax(0,1fr)!important}',
'.agent-dock-floating>.agentchat-overlay,.agent-dock-responsive>.agentchat-overlay{position:absolute;z-index:8;box-shadow:0 8px 28px #0009}',
'.agent-dock-floating.agent-dock-right>.agentchat-overlay,.agent-dock-responsive.agent-dock-right>.agentchat-overlay{right:0;top:0;bottom:0;width:min(var(--agent-dock-size,420px),90%)}',
'.agent-dock-floating.agent-dock-bottom>.agentchat-overlay,.agent-dock-responsive.agent-dock-bottom>.agentchat-overlay{left:0;right:0;bottom:0;height:min(var(--agent-dock-size,320px),80%)}',
'.agent-dock-responsive>.agentchat-dock-resizer{display:none}',
'.agentchat-window button{font-size:12px;padding:7px 10px;border-radius:5px;cursor:pointer}',
'.agentchat-window button:disabled{opacity:.48;cursor:not-allowed}',
'.agentchat-layout{display:flex;flex:1;min-height:0}',
'.agentchat-sessions{flex:none;margin-left:auto}',
'.agentchat-list{display:none;position:absolute;z-index:6;top:100%;right:7px;width:min(320px,calc(100% - 14px));max-height:min(50vh,400px);overflow:auto;padding:8px;flex-direction:column;gap:5px;background:#192330;border:1px solid #55657a;border-radius:7px;box-shadow:0 8px 28px #0009}',
'.agentchat-list.open{display:flex}',
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
'.agentchat-info{font-size:11px;color:#b6cce2;line-height:1.5;padding:4px 8px}',
'.agentchat-settings-overlay{position:fixed;inset:0;z-index:17110;display:none;align-items:center;justify-content:center;background:#000a;padding:15px}',
'.agentchat-settings-overlay.visible{display:flex}',
'.agentchat-settings-box{width:min(730px,97vw);max-height:90vh;overflow:auto;background:#141c27;color:#e6eef9;border:1px solid #55657a;border-radius:12px;padding:16px;display:flex;flex-direction:column;gap:10px}',
'.agentchat-settings-box textarea{width:100%;min-height:250px;resize:vertical;background:#0f1723;border:1px solid #55667b;color:inherit;font:12px/1.5 ui-monospace,monospace;padding:8px;box-sizing:border-box}',
'.agentchat-settings-box input{width:100%;padding:8px;box-sizing:border-box}',
'.agentchat-settings-box .actions{display:flex;gap:8px;justify-content:flex-end}',
'html[data-taskmenu-theme="light"] .agentchat-window,html[data-taskmenu-theme="light"] .agentchat-settings-box{background:#fff;color:#17283a}',
'html[data-taskmenu-theme="light"] .agentchat-window textarea,html[data-taskmenu-theme="light"] .agentchat-window input,html[data-taskmenu-theme="light"] .agentchat-window select,html[data-taskmenu-theme="light"] .agentchat-settings-box textarea,html[data-taskmenu-theme="light"] .agentchat-settings-box input{background:#f7faff;color:#17283a;border-color:#b6c6d7}',
'html[data-taskmenu-theme="light"] .agentchat-bubble.user{background:#d9edfc}',
'html[data-taskmenu-theme="light"] .agentchat-bubble.assistant{background:#eef3f8}',
'html[data-taskmenu-theme="light"] .agentchat-bubble.tool{background:#fff0d8;color:#523815}',
'.agentchat-dock-floating>.agentchat-dock-resizer{display:none}',
'.agentchat-composer-actions{display:flex;align-items:center;justify-content:space-between;gap:8px;font-size:11px;flex-wrap:wrap}',
'@media(max-width:700px){.agentchat-modelbar select{max-width:120px}.task-agent-dock-section.agent-dock-right:not(.agent-dock-floating){grid-template-columns:minmax(0,1fr) 6px minmax(290px,55vw)}}'
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
windowBox.setAttribute('role','region');windowBox.setAttribute('aria-label','TaskDeck Agent Chat');
const heading=agentEl('div','agentchat-toolbar'),title=agentEl('strong','','AI Agent Chat');
const newChat=agentButton('+ New session',()=>createSession().catch(showAgentError));
const settingsButton=agentButton('Providers / API keys',()=>openAgentSettings().catch(showAgentError));
const planButton=agentButton('Planning only',()=>openTaskManifestPreview());
const refresh=agentButton('Refresh',()=>loadAgentState().catch(showAgentError));
const close=agentButton('Hide',()=>closeAgentPanel());
const debugButton=agentButton('🐞',()=>toggleAgentDebug().catch(showAgentError));
debugButton.title='AI Debug capture';debugButton.setAttribute('aria-label','Enable AI Debug capture');debugButton.setAttribute('aria-pressed','false');
const dockPositionButton=agentButton('Bottom',()=>setAgentDockPosition(agentDock.position==='right'?'bottom':'right'));
dockPositionButton.title='Move Agent Chat between right and bottom';dockPositionButton.setAttribute('aria-label','Move Agent Chat to bottom');
const autoHideButton=agentButton('Auto Hide',()=>setAgentDockAutoHide(!agentDock.autoHide));
autoHideButton.title='Toggle Agent Chat auto hide';autoHideButton.setAttribute('aria-label','Enable Agent Chat auto hide');
const moreButton=agentButton('⋯',()=>toggleAgentMore());moreButton.title='More Agent Chat actions';moreButton.setAttribute('aria-label','More Agent Chat actions');
const toolbarActions=agentEl('div','agentchat-toolbar-actions'),moreMenu=agentEl('div','agentchat-more-menu');
let agentDebugEnabled=false,agentDebugRaw=false,agentDebugView='chat',agentDebugTab='timeline',agentDebugTrace=null;
const viewTabs=agentEl('div','agentchat-view-tabs');
const chatViewButton=agentButton('Conversation',()=>setAgentDebugView('chat'));
const debugViewButton=agentButton('AI Debug',()=>setAgentDebugView('debug'));
chatViewButton.setAttribute('aria-selected','true');debugViewButton.setAttribute('aria-selected','false');
viewTabs.append(chatViewButton,debugViewButton);
const debugPanel=agentEl('div','agentchat-debug-panel');
const debugToolbar=agentEl('div','agentchat-debug-toolbar');
const debugStateLabel=agentEl('span','agentchat-info','Debug is off.');
const debugTraceSelect=agentEl('select');debugTraceSelect.setAttribute('aria-label','Select debug trace');
const debugRefresh=agentButton('Refresh',()=>loadAgentDebugTraces().catch(showAgentError));
const debugRawButton=agentButton('Enable RAW…',()=>enableAgentDebugRaw().catch(showAgentError));
const debugClear=agentButton('Clear traces',()=>clearAgentDebug().catch(showAgentError));
const debugExport=agentButton('Export safe',()=>exportAgentDebug().catch(showAgentError));
const debugPartSelect=agentEl('select');for(const part of ['request','response','sse'])debugPartSelect.append(agentOption(part,part.toUpperCase()));
const debugPayloadView=agentEl('select');debugPayloadView.append(agentOption('safe','Safe view'),agentOption('raw','RAW view'));
const debugPayloadLoad=agentButton('View payload',()=>loadAgentDebugPayload().catch(showAgentError));
const debugCopy=agentButton('Copy visible',()=>copyAgentDebug().catch(showAgentError));
debugToolbar.append(debugStateLabel,debugTraceSelect,debugRefresh,debugRawButton,debugClear,debugExport,debugPartSelect,debugPayloadView,debugPayloadLoad,debugCopy);
const debugTabs=agentEl('div','agentchat-debug-tabs'),debugOutput=agentEl('pre','agentchat-debug-output','Enable capture and refresh to inspect diagnostics.');
for(const [key,label] of [['timeline','Timeline'],['console','Console'],['http','HTTP'],['payload','Payload'],['tools','Tools'],['configuration','Configuration']]){
 const tab=agentButton(label,()=>{agentDebugTab=key;renderAgentDebugTrace();});tab.dataset.debugTab=key;debugTabs.append(tab);
}
debugPanel.append(debugToolbar,debugTabs,debugOutput);
const agentDock={position:'right',autoHide:false,expanded:false,rightWidth:420,bottomHeight:320,observer:null};
const dockResizer=agentEl('div','agentchat-dock-resizer');dockResizer.tabIndex=0;dockResizer.setAttribute('role','separator');
const agentStrip=agentButton('✦ AI Agent Chat',()=>openAgentPanel().catch(showAgentError));
agentStrip.className='agentchat-strip';agentStrip.setAttribute('aria-label','Open Agent Chat');
const layout=agentEl('div','agentchat-layout'),sessionList=agentEl('div','agentchat-list');
const sessionMenu=agentEl('div','agentchat-sessions');
const sessionMenuButton=agentButton('Saved conversations ▾',()=>toggleAgentSessions());
sessionMenuButton.setAttribute('aria-expanded','false');
sessionMenuButton.setAttribute('aria-controls','agentchat-saved-sessions');
sessionList.id='agentchat-saved-sessions';
sessionList.setAttribute('aria-label','Saved conversations');
sessionMenu.append(sessionMenuButton,sessionList);
viewTabs.append(sessionMenu);
const main=agentEl('div','agentchat-main'),modelBar=agentEl('div','agentchat-modelbar');
const toolNotice=agentEl('div','agentchat-info');toolNotice.hidden=true;
// Open tabs are UI-only: the server remains the source of truth for every
// session. Each tab keeps its own composer draft.
const tabbar=agentEl('div','agentchat-modelbar');
let agentTabs=[];const agentDrafts=new Map();const agentAttachmentDrafts=new Map();
const agentRequestIDs=new Map();
let agentSelectGeneration=0;
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
for(const button of [newChat,preview,rename,archive,planButton,settingsButton,refresh,attachButton,autoFeedback])toolbarActions.append(button);
heading.append(title,toolbarActions,dockPositionButton,autoHideButton,debugButton,moreButton,close,moreMenu);
actionLine.append(status,cancel,send);composer.append(attachList,prompt,actionLine);
main.append(tabbar,modelBar,toolNotice,transcript,composer);layout.append(main);
const footer=agentEl('div','agentchat-info','AI Pack on first turn • read-only Collection inspection • tool or text-only Patch proposals require explicit native approval; verified results return to AI.');
windowBox.append(heading,viewTabs,layout,debugPanel,footer);root.append(windowBox);
const agentToolbarItems=[newChat,preview,rename,archive,planButton,settingsButton,refresh,attachButton,autoFeedback];
mountAgentDock();

const settingsOverlay=agentEl('div','agentchat-overlay agentchat-settings-overlay'),settingsBox=agentEl('section','agentchat-settings-box');
settingsBox.setAttribute('role','dialog');settingsBox.setAttribute('aria-modal','true');settingsBox.setAttribute('aria-label','Agent provider settings');
const settingsTitle=agentEl('h2','','Agent provider settings');
const blockedIPNotice=agentEl('div','agentchat-info');blockedIPNotice.hidden=true;
const settingsInfo=agentEl('p','','Configure an HTTPS Responses or Chat Completions provider. For a trusted 100.64.0.0/10 endpoint, explicitly add exact addresses to trusted_shared_ips in the provider JSON. This weakens network isolation; trust the endpoint and network first. API keys remain server-side.');
const settingsPreset=agentButton('Load sample provider',()=>{providerEditor.value=JSON.stringify(agentSample,null,2);});
const probeProvider=agentButton('Probe capabilities',()=>probeAgentProvider().catch(showAgentError));
const deleteProvider=agentButton('Remove provider',()=>deleteAgentProvider().catch(showAgentError));
const settingsPicker=agentEl('select'),settingsModel=agentEl('select'),settingsAPIType=agentEl('select'),providerEditor=agentEl('textarea');
settingsModel.setAttribute('aria-label','Model for connection and capability probes');
settingsAPIType.append(agentOption('responses','Responses API'),agentOption('chat_completions','Chat Completions API'));
const keyLabel=agentEl('label','','API key (enter only to set/change)'),apiKey=agentEl('input');
apiKey.type='password';apiKey.autocomplete='new-password';apiKey.placeholder='Leave blank to keep existing key';
const settingsStatus=agentEl('div','agentchat-info','');
const settingsActions=agentEl('div','actions');
const saveProvider=agentButton('Save provider',()=>saveAgentProvider().catch(showAgentError));
const testProvider=agentButton('Test connection (uses API)',()=>testAgentProvider().catch(showAgentError));
const closeSettings=agentButton('Close',()=>settingsOverlay.classList.remove('visible'));
settingsActions.append(settingsPreset,saveProvider,testProvider,probeProvider,deleteProvider,closeSettings);
settingsBox.append(settingsTitle,blockedIPNotice,settingsInfo,settingsPicker,settingsModel,settingsAPIType,providerEditor,keyLabel,apiKey,settingsStatus,settingsActions);
settingsOverlay.append(settingsBox);document.body.append(settingsOverlay);

// F08.1 preview is planning-only: it validates Markdown and renders a DAG,
// but has no persistence or execution action.
const planOverlay=agentEl('div','agentchat-overlay agentchat-settings-overlay');
const planBox=agentEl('section','agentchat-settings-box');
const planTitle=agentEl('h2','','Task Manifest · Planning only');
const planNote=agentEl('p','','Parse and validate a Markdown task plan. Validation performs no provider call, file write, task creation or execution.');
const planFormat=agentEl('select');planFormat.append(agentOption('markdown','Markdown'),agentOption('canonical_json','Canonical JSON'));
const planInput=agentEl('textarea');planInput.placeholder='# Plan\n\n## Task A — Title\nDepends on: None\nAcceptance:\n- Criterion';
const planOutput=agentEl('div','agentchat-info','');
const planActions=agentEl('div','actions');
const validatePlan=agentButton('Validate / Preview',()=>validateTaskManifest().catch(showAgentError));
const savePlan=agentButton('Download canonical JSON',()=>downloadTaskManifest());savePlan.disabled=true;
const closePlan=agentButton('Close',()=>planOverlay.classList.remove('visible'));
planActions.append(validatePlan,savePlan,closePlan);planBox.append(planTitle,planNote,planFormat,planInput,planActions,planOutput);planOverlay.append(planBox);document.body.append(planOverlay);

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
 let message=String(error?.message||error);
 try{const parsed=JSON.parse(message);const detail=parsed?.error;if(detail)message='FAILED · '+String(detail.code||'UNKNOWN')+' · '+String(detail.stage||'unknown stage')+' · '+String(detail.message||'No safe provider message')+(detail.http_status?' · HTTP '+detail.http_status:'')+(detail.trace_id?' · Trace '+detail.trace_id:'')+(detail.ambiguous?' · outcome may be ambiguous':'');}catch{}
 status.textContent='Error: '+message;
 settingsStatus.textContent='Error: '+message;
}
function normalizeProvider(raw){
 const data=raw?.provider||raw;
 if(!data||Array.isArray(data)||typeof data!=='object')throw new Error('Provider must be a JSON object');
 const ms=Array.isArray(data.models)?data.models:[];
 return {
  id:String(data.id||'').trim(),name:String(data.name||'').trim(),
  api_type:String(data.api_type||data.apiType||'responses'),
  base_url:String(data.base_url||data.baseUrl||ms[0]?.url||data.url||'').trim(),
  ...(data.trusted_shared_ips!==undefined?{trusted_shared_ips:data.trusted_shared_ips}:{}),
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
 if(p?.api_type!=='chat_completions'){
  for(const effort of (current?.reasoning_efforts||[]))effortSelect.append(agentOption(effort,effort));
  effortSelect.value=(current?.default_reasoning_effort||'');
 }else{effortSelect.value='';}
 const statelessOnly=p?.api_type==='chat_completions';
 for(const option of modeSelect.options){
  option.disabled=statelessOnly&&option.value!=='stateless';
 }
 if(statelessOnly)modeSelect.value='stateless';
 const hasAgent=Boolean(p&&p.has_api_key);
 newChat.disabled=!hasAgent||agentBusy;
 attachButton.disabled=false;
}
providerSelect.addEventListener('change',updateAgentModels);
modelSelect.addEventListener('change',()=>{
 const p=agentProviders.find(x=>x.id===providerSelect.value);
 const m=(p?.models||[]).find(x=>x.id===modelSelect.value);
 effortSelect.replaceChildren(agentOption('','Model default'));
 if(p?.api_type!=='chat_completions'){
  for(const effort of (m?.reasoning_efforts||[]))effortSelect.append(agentOption(effort,effort));
  effortSelect.value=(m?.default_reasoning_effort||'');
 }else{effortSelect.value='';}
});
settingsPicker.addEventListener('change',()=>{
 blockedIPNotice.hidden=true;blockedIPNotice.replaceChildren();
 const p=agentProviders.find(x=>x.id===settingsPicker.value);
 const current=p?normalizeProvider(p):agentSample;
 settingsModel.replaceChildren();
 for(const model of current.models||[])settingsModel.append(agentOption(model.id,model.name||model.id));
 if(p&&providerSelect.value===p.id&&Array.from(settingsModel.options).some(model=>model.value===modelSelect.value))settingsModel.value=modelSelect.value;
 providerEditor.value=JSON.stringify(current,null,2);
 settingsAPIType.value=current.api_type||'responses';
 apiKey.value='';settingsStatus.textContent=p?.has_api_key?'API key already configured (write-only).':'API key is not configured.';
});
settingsAPIType.addEventListener('change',()=>{
 try{
  const parsed=normalizeProvider(JSON.parse(providerEditor.value));
  parsed.api_type=settingsAPIType.value;
  providerEditor.value=JSON.stringify(parsed,null,2);
 }catch{}
});
async function saveAgentProvider(){
 const parsed=normalizeProvider(JSON.parse(providerEditor.value));
 parsed.api_type=settingsAPIType.value||parsed.api_type;
 const previous=agentProviders.find(p=>p.id===parsed.id);
 if(parsed.trusted_shared_ips?.length&&(parsed.base_url!==previous?.base_url||JSON.stringify(parsed.trusted_shared_ips)!==JSON.stringify(previous?.trusted_shared_ips||[]))){
  if(!window.confirm('Trust these exact shared-network IP addresses for '+parsed.base_url+'? This allows TaskDeck to send your API key and project context to them and weakens SSRF protection. Continue only if you trust the network and endpoint.'))return;
 }
 const body={provider:parsed};if(apiKey.value)body.api_key=apiKey.value;
 const data=await agentApp.jsonFetch('/api/agent/providers',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
 apiKey.value='';
 await loadAgentProviders();providerSelect.value=data.provider.id;updateAgentModels();settingsPicker.value=data.provider.id;
 settingsStatus.textContent='Provider saved. Secrets were not returned by the server.';
}
async function testAgentProvider(){
 const parsed=normalizeProvider(JSON.parse(providerEditor.value));
 parsed.api_type=settingsAPIType.value||parsed.api_type;
 if(!window.confirm('Send a short test prompt to '+parsed.base_url+' using the saved API key?'))return;
 const p=agentProviders.find(x=>x.id===parsed.id);
 if(!p){settingsStatus.textContent='Save this provider first.';return;}
 const model=settingsModel.value;
 if(!model){settingsStatus.textContent='No model configured.';return;}
 blockedIPNotice.hidden=true;blockedIPNotice.replaceChildren();
 settingsStatus.textContent='Testing Responses API...';
 let result;
 try{
  result=await agentApp.jsonFetch('/api/agent/providers/test',{method:'POST',headers:{'Content-Type':'application/json'},
   body:JSON.stringify({provider_id:p.id,model_id:model,confirm_external:true})});
 }catch(error){
  let detail;try{detail=JSON.parse(String(error?.message||error))?.error;}catch{}
  if(detail?.code==='PRIVATE_IP_BLOCKED'&&detail.blocked_shared_ip){
   const ip=String(detail.blocked_shared_ip);
   const allow=agentButton('Allow this IP',()=>allowAgentProviderIP(p.id,p.base_url,ip).catch(showAgentError));
   blockedIPNotice.replaceChildren(agentEl('span','','DNS returned blocked shared-network IP: '+ip+' for '+p.base_url+' '),allow);
   blockedIPNotice.hidden=false;
  }
  throw error;
 }
 settingsStatus.textContent='Basic Responses text probe: '+(result.ok?'PASS':'FAILED')+
  ' · Trace '+String(result.trace_id||'unavailable')+' · This does not verify tool calling or stateful chaining; use Probe capabilities.';
}
async function allowAgentProviderIP(providerID,endpoint,ip){
 const saved=agentProviders.find(p=>p.id===providerID);
 const edited=normalizeProvider(JSON.parse(providerEditor.value));
 if(!saved||saved.base_url!==endpoint||edited.id!==providerID||edited.base_url!==endpoint||settingsPicker.value!==providerID)throw new Error('Provider or endpoint changed; select it and run Test connection again.');
 if(!window.confirm('Allow '+ip+' for '+endpoint+'? TaskDeck may send your API key and project context to this address. This weakens SSRF protection; continue only if you trust the endpoint and network.'))return;
 const provider=normalizeProvider(saved);
 provider.trusted_shared_ips=[...new Set([...(provider.trusted_shared_ips||[]),ip])];
 const data=await agentApp.jsonFetch('/api/agent/providers',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({provider})});
 const index=agentProviders.findIndex(p=>p.id===providerID);if(index>=0)agentProviders[index]=data.provider;
 edited.trusted_shared_ips=[...new Set([...(edited.trusted_shared_ips||[]),ip])];
 providerEditor.value=JSON.stringify(edited,null,2);
 blockedIPNotice.hidden=true;blockedIPNotice.replaceChildren();
 settingsStatus.textContent='Allowed '+ip+' for this provider. Run Test connection again; existing capability probes must be repeated.';
}
async function probeAgentProvider(){
 const parsed=normalizeProvider(JSON.parse(providerEditor.value));
 parsed.api_type=settingsAPIType.value||parsed.api_type;
 const p=agentProviders.find(x=>x.id===parsed.id);
 if(!p){settingsStatus.textContent='Save this provider first.';return;}
 const model=settingsModel.value;
 if(!model){settingsStatus.textContent='No model configured.';return;}
 if(!window.confirm('Run harmless capability probes against '+parsed.base_url+'? This sends no project data and can incur provider cost.'))return;
 settingsStatus.textContent='Probing text, tool schema, response chaining'+(parsed.stream?' and streaming':'')+'...';
 const result=await agentApp.jsonFetch('/api/agent/providers/probe',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({provider_id:p.id,model_id:model,confirm_external:true,
   capabilities:parsed.stream?['text','tool_calling','previous_response','streaming']:['text','tool_calling','previous_response']})});
 const caps=result.capabilities||{};
 settingsStatus.textContent='Probe evidence: '+Object.entries(caps).map(([k,v])=>k+'='+v).join(' · ')+
  ' · Probe traces are available in AI Debug. Tool execution stays disabled unless tool_calling=verified. Refresh or reopen the session to update its status.';
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
async function openAgentSettings(){
 const active=agentSessions.find(session=>session.id===agentActive);
 await loadAgentProviders();
 if(active&&agentProviders.some(provider=>provider.id===active.provider_id)){
  settingsPicker.value=active.provider_id;settingsPicker.dispatchEvent(new Event('change'));
  if(Array.from(settingsModel.options).some(model=>model.value===active.model_id))settingsModel.value=active.model_id;
 }
 settingsOverlay.classList.add('visible');return true;
}
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
 box.append(agentEl('strong','',(record?.text_fallback?'Text-only Patch proposal':String(item.name||'Unknown tool'))+' · '+state.toUpperCase()));
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
 if(!session){toolNotice.hidden=true;transcript.append(agentEl('p','','Create or select a session to begin.'));return;}
 for(const item of (session.history||[])){
  if(item.type==='user')transcript.append(agentEl('div',String(item.content||'').startsWith('TaskDeck native Patch Tool result for your prior text-only proposal')?'agentchat-bubble tool':'agentchat-bubble user',userMessageText(item.content)));
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
  const item=agentButton(label,()=>{toggleAgentSessions(false);selectAgentSession(sess.id).catch(showAgentError);});
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
 if(agentActive===id){
  agentActive=null;
  if(agentTabs.length)selectAgentSession(agentTabs[agentTabs.length-1]).catch(showAgentError);
  else displayAgentSession(null);
 }
 renderAgentTabs();
}
async function selectAgentSession(id){
 const previousID=agentActive;
 // Save the outgoing draft BEFORE awaiting the next session fetch. Attachments
 // are per-session too; never let a response race copy one session's state to
 // another tab.
 if(previousID&&previousID!==id){
  agentDrafts.set(previousID,prompt.value);
  agentAttachmentDrafts.set(previousID,agentAttachments.map(a=>({...a})));
 }
 const generation=++agentSelectGeneration;
 const data=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(id));
 if(generation!==agentSelectGeneration)return null;
 const sess=data.session;
 toolNotice.replaceChildren();
 const toolStatus=data.patch_tool_status;
 if(toolStatus==='enabled'){
  toolNotice.textContent='Native Patch available: proposals require explicit approval; read-only Collection results continue automatically.';
 }else{
  const reasons={model_disabled:'Tool calling is disabled for this model.',unverified:'Tool calling has not been verified for this provider and model.',unavailable:'Provider or capability information is unavailable.'};
  toolNotice.append(agentEl('span','',(reasons[toolStatus]||'Patch tool status is unavailable.')+' Text-only Patch JSON proposals can still be reviewed and executed through native Patch approval. '));
  toolNotice.append(agentButton('Provider settings',()=>openAgentSettings().catch(showAgentError)));
 }
 toolNotice.hidden=false;
 agentActive=id;
 openAgentTab(id);
 prompt.value=agentDrafts.get(id)||'';
 agentAttachments=(agentAttachmentDrafts.get(id)||[]).map(a=>({...a}));
 renderAgentAttachments();
 // After a refresh/reconnect, an in-flight provider turn has no browser
 // request attached. Reflect it instead of offering a duplicate send.
 if(sess.status==='requesting_ai'&&!agentSessionBusy(id)){
  status.textContent='A model request is still running on the server. Use Cancel if you want to stop waiting.';
  send.disabled=true;cancel.disabled=false;
 }else{
  send.disabled=agentSessionBusy(id);cancel.disabled=!agentSessionBusy(id);
 }
 const p=agentProviders.find(p=>p.id===sess.provider_id);
 if(p){providerSelect.value=p.id;updateAgentModels();modelSelect.value=sess.model_id;}
 modeSelect.value=sess.mode;
 effortSelect.value=sess.reasoning_effort||'';
 displayAgentSession(sess);
 scheduleAgentMonitor(sess);
 renderAgentTabs();
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
 const sessionID=agentActive;
 const msg=prompt.value.trim();
 const attachments=(agentAttachments||[]).map(a=>({...a}));
 if(!msg)throw new Error('Enter a message.');
 if(!window.confirm('Send this request, its text attachments'+(attachments.length?' ('+attachments.length+')':'')+' and, on the first turn, the TaskDeck AI Pack to the selected external provider? This chat may propose patch execution, which requires a separate approval.'))return;
 const s=await selectAgentSession(sessionID);
 if(!s||agentActive!==sessionID)return;
 agentBusy=true;markAgentSessionBusy(sessionID,true);
 agentAbort=new AbortController();
 try{
  status.textContent='Waiting for AI provider…';
  // Keep one key per composition while retrying after a transport failure.
  let requestID=agentRequestIDs.get(sessionID);
  if(!requestID){
   requestID='ui-'+sessionID+'-'+Date.now().toString(36)+'-'+Math.random().toString(36).slice(2,8);
   agentRequestIDs.set(sessionID,requestID);
  }
  const response=await agentApp.jsonFetch('/api/agent/sessions/'+encodeURIComponent(sessionID)+'/messages',{
   method:'POST',headers:{'Content-Type':'application/json'},
   body:JSON.stringify({message:msg,confirm_external:true,expected_revision:s.revision,request_id:requestID,
    attachments}),
   signal:agentAbort.signal
  });
  agentRequestIDs.delete(sessionID);
  agentDrafts.delete(sessionID);agentAttachmentDrafts.delete(sessionID);
  if(agentActive===sessionID){prompt.value='';agentAttachments=[];renderAgentAttachments();}
  if(agentActive===sessionID)await selectAgentSession(sessionID);
  if(agentActive===sessionID){
   status.textContent=response.status==='waiting_tool'?'AI requested a patch; review/approve explicitly.':
    'Response received. No commands executed.';
  }
 }catch(error){
  agentDrafts.set(sessionID,msg);
  agentAttachmentDrafts.set(sessionID,attachments);
  if(agentActive===sessionID){
   prompt.value=msg;
   agentAttachments=attachments.map(a=>({...a}));
   renderAgentAttachments();
  }
  throw error;
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
function openTaskManifestPreview(){planOverlay.classList.add('visible');}
function downloadTaskManifest(){
 if(!lastValidatedManifest||!lastValidatedManifest.valid)return;
 const blob=new Blob([JSON.stringify(lastValidatedManifest,null,2)],{type:'application/json'});
 const link=document.createElement('a');
 link.href=URL.createObjectURL(blob);link.download=(String(lastValidatedManifest.name||'task-manifest').replace(/[^a-zA-Z0-9._-]+/g,'-')+'.json');
 link.click();setTimeout(()=>URL.revokeObjectURL(link.href),0);
}
let lastValidatedManifest=null;
async function validateTaskManifest(){
 const input=planInput.value;
 const format=planFormat.value;
 planOutput.replaceChildren(agentEl('p','agentchat-info','Validating locally via TaskDeck (no AI/provider call)…'));
 const payload=format==='canonical_json'?{format,canonical_json:input}:{format,markdown:input};
 const data=await agentApp.jsonFetch('/api/agent/task-manifests/validate',{
  method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),
 });
 const manifest=data.manifest||{};
 lastValidatedManifest=manifest;
 savePlan.disabled=!manifest.valid;
 const summary=agentEl('p','agentchat-info',
  (manifest.valid?'VALID':'INVALID')+' · '+String(manifest.tasks?.length||0)+' tasks · '+
  String(manifest.graph?.roots?.length||0)+' roots · '+String(manifest.graph?.blocked?.length||0)+' dependency-blocked · Planning only');
 planOutput.replaceChildren(summary);
 const rows=agentEl('div','agentchat-info');
 for(const task of (manifest.tasks||[])){
  const deps=(task.dependencies||[]).length?(task.dependencies||[]).join(', '):'None';
  const acceptance=(task.acceptance_criteria||[]).map(x=>'  • '+x).join('\n');
  rows.append(agentEl('pre','agentchat-tool-summary',task.id+' — '+task.title+'\nDepends on: '+deps+'\n'+acceptance));
 }
 planOutput.append(rows);
 const order=manifest.graph?.topological_order;
 if(order)planOutput.append(agentEl('p','agentchat-info','Topological preview: '+order.join(' → ')));
 for(const issue of (manifest.issues||[])){
  planOutput.append(agentEl('p','agentchat-tool-summary',(issue.warning?'Warning':'Error')+(issue.line?' · line '+issue.line:'')+' · '+issue.code+': '+issue.message));
 }
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
function agentDockKey(){
 const workspace=String(agentApp.taskData?.workspace||'').trim()||'default';
 return 'vscode-tasks-menu:agent-chat-dock:v1:'+workspace;
}
function saveAgentDock(){
 try{localStorage.setItem(agentDockKey(),JSON.stringify({position:agentDock.position,autoHide:agentDock.autoHide,expanded:agentDock.expanded,rightWidth:agentDock.rightWidth,bottomHeight:agentDock.bottomHeight}));}catch{}
}
function applyAgentDock(){
 const section=root.parentElement;
 if(!section?.classList?.contains('task-agent-dock-section'))return;
 section.classList.toggle('agent-dock-right',agentDock.position==='right');
 section.classList.toggle('agent-dock-bottom',agentDock.position==='bottom');
 section.classList.toggle('agent-dock-collapsed',!agentDock.expanded);
 section.classList.toggle('agent-dock-floating',agentDock.autoHide&&agentDock.expanded);
 section.classList.toggle('agent-dock-responsive',agentDock.expanded&&(agentDock.position==='right'?section.clientWidth<640:section.clientHeight<440));
 const size=agentDock.position==='right'?agentDock.rightWidth:agentDock.bottomHeight;
 section.style.setProperty('--agent-dock-size',size+'px');
 root.classList.toggle('visible',agentDock.expanded);
 agentStrip.setAttribute('aria-expanded',String(agentDock.expanded));
 dockPositionButton.textContent=agentDock.position==='right'?'Bottom':'Right';
 dockPositionButton.setAttribute('aria-label','Move Agent Chat to '+(agentDock.position==='right'?'bottom':'right'));
 autoHideButton.textContent=agentDock.autoHide?'Pin':'Auto Hide';
 autoHideButton.setAttribute('aria-label',agentDock.autoHide?'Pin Agent Chat':'Enable Agent Chat auto hide');
 dockResizer.setAttribute('aria-orientation',agentDock.position==='right'?'vertical':'horizontal');
 if(agentDock.position==='right')dockResizer.setAttribute('aria-label','Resize Agent Chat width');
 else dockResizer.setAttribute('aria-label','Resize Agent Chat height');
}
function mountAgentDock(){
 if(typeof document.querySelector!=='function'){document.body.append(root);return;}
 const section=document.querySelector('main>section'),tabs=document.querySelector('#tabs'),panes=document.querySelector('#panes');
 if(!section||!tabs||!panes){document.body.append(root);return;}
 const content=agentEl('div','task-agent-content');
 section.classList.add('task-agent-dock-section');
 content.append(tabs,panes);section.append(content,dockResizer,root,agentStrip);
 let restored=null;
 try{restored=JSON.parse(localStorage.getItem(agentDockKey())||'null');}catch{}
 if(restored&&typeof restored==='object'){
  agentDock.position=restored.position==='bottom'?'bottom':'right';
  agentDock.autoHide=Boolean(restored.autoHide);
  agentDock.expanded=Boolean(restored.expanded);
  agentDock.rightWidth=clampDockSize(restored.rightWidth,'right');
  agentDock.bottomHeight=clampDockSize(restored.bottomHeight,'bottom');
 }
 if(agentApp.layoutProfile==='mobile')agentDock.autoHide=true;
 if(typeof ResizeObserver==='function'){
  let lastSize='';
  agentDock.observer=new ResizeObserver(()=>{
   const size=section.clientWidth+':'+section.clientHeight+':'+heading.clientWidth;
   if(size===lastSize)return;
   lastSize=size;
   applyAgentDock();rebalanceAgentToolbar();
  });
  agentDock.observer.observe(heading);agentDock.observer.observe(section);
 }
 rebalanceAgentToolbar();applyAgentDock();
}
function clampDockSize(value,position){
 const n=Number(value)||(position==='right'?420:320);
 if(position==='right')return Math.round(Math.max(300,Math.min(n,Math.min(window.innerWidth*.65,Math.max(300,window.innerWidth-360)))));
 return Math.round(Math.max(190,Math.min(n,Math.max(190,window.innerHeight*.65))));
}
function rebalanceAgentToolbar(){
 if(!toolbarActions.isConnected&&root.parentElement?.classList?.contains('task-agent-dock-section')===false)return;
 const space=heading.clientWidth||0;
 if(!space)return;
 for(const button of agentToolbarItems)toolbarActions.append(button);
 moreMenu.classList.remove('open');moreButton.setAttribute('aria-expanded','false');
 const always=dockPositionButton.offsetWidth+autoHideButton.offsetWidth+debugButton.offsetWidth+moreButton.offsetWidth+close.offsetWidth+title.offsetWidth+24;
 while(toolbarActions.scrollWidth>Math.max(0,space-always)&&toolbarActions.children.length>1){
  const button=toolbarActions.lastElementChild;moreMenu.prepend(button);
 }
}
function toggleAgentMore(force){
 const open=force===undefined?!moreMenu.classList.contains('open'):Boolean(force);
 moreMenu.classList.toggle('open',open);moreButton.setAttribute('aria-expanded',String(open));
 if(open)moreMenu.querySelector('button')?.focus();else moreButton.focus();
}
function toggleAgentSessions(force){
 const open=force===undefined?!sessionList.classList.contains('open'):Boolean(force);
 sessionList.classList.toggle('open',open);sessionMenuButton.setAttribute('aria-expanded',String(open));
}
function setAgentDockPosition(position){
 if(position!=='right'&&position!=='bottom')return;
 agentDock.position=position;
 applyAgentDock();saveAgentDock();
 window.dispatchEvent(new Event('resize'));
}
function setAgentDockAutoHide(enabled){agentDock.autoHide=Boolean(enabled);applyAgentDock();saveAgentDock();}
function resizeAgentDock(event){
 if(event.button!==0)return;
 const start=agentDock.position==='right'?event.clientX:event.clientY;
 const initial=agentDock.position==='right'?agentDock.rightWidth:agentDock.bottomHeight;
 dockResizer.setPointerCapture?.(event.pointerId);event.preventDefault();
 const move=e=>{
  const delta=agentDock.position==='right'?start-e.clientX:start-e.clientY;
  const size=clampDockSize(initial+delta,agentDock.position);
  if(agentDock.position==='right')agentDock.rightWidth=size;else agentDock.bottomHeight=size;
  applyAgentDock();
 };
 const done=e=>{
  dockResizer.removeEventListener('pointermove',move);dockResizer.removeEventListener('pointerup',done);dockResizer.removeEventListener('pointercancel',done);
  try{dockResizer.releasePointerCapture?.(e.pointerId);}catch{}
  saveAgentDock();window.dispatchEvent(new Event('resize'));
 };
 dockResizer.addEventListener('pointermove',move);dockResizer.addEventListener('pointerup',done);dockResizer.addEventListener('pointercancel',done);
}
dockResizer.addEventListener('pointerdown',resizeAgentDock);
dockResizer.addEventListener('keydown',event=>{
 const step=event.shiftKey?25:10;let next=agentDock.position==='right'?agentDock.rightWidth:agentDock.bottomHeight;
 if(event.key==='Home')next=agentDock.position==='right'?300:190;
 else if(event.key==='End')next=agentDock.position==='right'?window.innerWidth*.65:window.innerHeight*.65;
 else if(agentDock.position==='right'&&event.key==='ArrowLeft')next-=step;
 else if(agentDock.position==='right'&&event.key==='ArrowRight')next+=step;
 else if(agentDock.position==='bottom'&&event.key==='ArrowUp')next-=step;
 else if(agentDock.position==='bottom'&&event.key==='ArrowDown')next+=step;
 else return;
 event.preventDefault();if(agentDock.position==='right')agentDock.rightWidth=clampDockSize(next,'right');else agentDock.bottomHeight=clampDockSize(next,'bottom');applyAgentDock();saveAgentDock();
});
moreMenu.addEventListener('click',event=>{
 event.stopPropagation();
 if(event.target.closest('button')){moreMenu.classList.remove('open');moreButton.setAttribute('aria-expanded','false');}
});
for(const overlay of [settingsOverlay,planOverlay])overlay.addEventListener('pointerdown',event=>event.stopPropagation());
document.addEventListener?.('pointerdown',event=>{
 if(sessionList.classList.contains('open')&&!sessionMenu.contains(event.target))toggleAgentSessions(false);
 if(moreMenu.classList.contains('open')&&!moreMenu.contains(event.target)&&!moreButton.contains(event.target)){
  moreMenu.classList.remove('open');moreButton.setAttribute('aria-expanded','false');
 }
 if(!agentDock.expanded||!agentDock.autoHide)return;
 if(root.contains(event.target)||agentStrip.contains(event.target)||moreMenu.contains(event.target)||settingsOverlay.contains(event.target)||planOverlay.contains(event.target)||dockResizer.contains(event.target))return;
 agentDock.expanded=false;applyAgentDock();saveAgentDock();
},true);
window.addEventListener('resize',()=>{agentDock.rightWidth=clampDockSize(agentDock.rightWidth,'right');agentDock.bottomHeight=clampDockSize(agentDock.bottomHeight,'bottom');applyAgentDock();rebalanceAgentToolbar();});
function setAgentDebugView(view){
 agentDebugView=view==='debug'?'debug':'chat';
 layout.style.display=agentDebugView==='chat'?'flex':'none';debugPanel.classList.toggle('visible',agentDebugView==='debug');
 chatViewButton.setAttribute('aria-selected',String(agentDebugView==='chat'));debugViewButton.setAttribute('aria-selected',String(agentDebugView==='debug'));
 if(agentDebugView==='debug')loadAgentDebugState().then(loadAgentDebugTraces).catch(showAgentError);
}
async function loadAgentDebugState(){
 const state=await agentApp.jsonFetch('/api/agent/debug/state');agentDebugEnabled=Boolean(state.enabled);agentDebugRaw=Boolean(state.raw_active);
 debugButton.textContent=agentDebugEnabled?'🐞 ON':'🐞 OFF';debugButton.setAttribute('aria-pressed',String(agentDebugEnabled));
 debugButton.setAttribute('aria-label',agentDebugEnabled?'Disable AI Debug capture':'Enable AI Debug capture');
 debugStateLabel.textContent='Capturing: '+(agentDebugEnabled?'ON':'OFF')+' · '+(agentDebugRaw?'RAW ACTIVE until '+new Date(state.raw_until).toLocaleTimeString():'safe only')+' · '+String(state.raw_bytes||0)+' raw bytes in memory';
 debugRawButton.textContent=agentDebugRaw?'Disable RAW':'Enable RAW…';debugPayloadView.disabled=!agentDebugRaw;debugPayloadView.value=agentDebugRaw?'raw':'safe';
}
async function toggleAgentDebug(){
 const enabled=!agentDebugEnabled;
 const state=await agentApp.jsonFetch('/api/agent/debug/state',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({enabled})});
 agentDebugEnabled=Boolean(state.enabled);await loadAgentDebugState();
 if(enabled)setAgentDebugView('debug');
}
async function enableAgentDebugRaw(){
 if(agentDebugRaw){
  await agentApp.jsonFetch('/api/agent/debug/state',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({enabled:agentDebugEnabled,disable_raw:true})});
  await loadAgentDebugState();return;
 }
 if(!agentDebugEnabled)throw new Error('Enable safe AI Debug capture before RAW mode.');
 if(!window.confirm('RAW mode temporarily holds request/response bodies in bounded server memory for up to 30 minutes. These may include prompts, AI Pack and tool output. Credentials and authorization headers are never captured. Continue?'))return;
 await agentApp.jsonFetch('/api/agent/debug/state',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({enabled:true,raw_consent:true})});await loadAgentDebugState();
}
async function loadAgentDebugTraces(){
 if(!agentDebugEnabled){debugTraceSelect.replaceChildren(agentOption('','Capture is off'));agentDebugTrace=null;renderAgentDebugTrace();return;}
 const result=await agentApp.jsonFetch('/api/agent/debug/traces?limit=64');const traces=Array.isArray(result.traces)?result.traces:[];
 debugTraceSelect.replaceChildren();for(const trace of traces){const label=new Date(trace.created_at).toLocaleTimeString()+' · '+trace.action+' · '+(trace.model_id||trace.provider_id||'provider');debugTraceSelect.append(agentOption(trace.trace_id,label));}
 if(agentDebugTrace&&traces.some(t=>t.trace_id===agentDebugTrace))debugTraceSelect.value=agentDebugTrace;else agentDebugTrace=traces[0]?.trace_id||null;
 if(agentDebugTrace)debugTraceSelect.value=agentDebugTrace;
 renderAgentDebugTrace();
}
async function loadAgentDebugTrace(){
 const id=debugTraceSelect.value;if(!id)return null;agentDebugTrace=id;const result=await agentApp.jsonFetch('/api/agent/debug/traces/'+encodeURIComponent(id));return result.trace||null;
}
async function renderAgentDebugTrace(){
 const trace=await loadAgentDebugTrace();if(!trace){debugOutput.textContent=agentDebugEnabled?'No diagnostic traces captured yet.':'Enable capture; structured provider errors remain available while capture is off.';return;}
 const events=Array.isArray(trace.events)?trace.events:[];
 const selected=events.filter(event=>agentDebugTab==='http'?event.stage==='provider_http':agentDebugTab==='tools'?event.stage==='tool'||event.event?.startsWith('tool.'):agentDebugTab==='configuration'?event.stage==='config':true);
 if(agentDebugTab==='payload'){debugOutput.textContent='Choose REQUEST, RESPONSE or SSE above. Safe view hides free-text bodies; metadata remains available in Timeline/HTTP. Use RAW only during the explicit short-lived grant.';return;}
 if(agentDebugTab==='console')debugOutput.textContent=selected.map(event=>new Date(event.at).toLocaleString()+' ['+String(event.level||'info').toUpperCase()+'] '+event.stage+'.'+event.event+(event.code?' '+event.code:'')+(event.safe_message?' — '+event.safe_message:'')).join('\n')||'No console entries.';
 else debugOutput.textContent=JSON.stringify(selected,null,2)||'[]';
}
async function loadAgentDebugPayload(){
 if(!agentDebugTrace)throw new Error('Select a trace first.');
 if(debugPayloadView.value==='raw'&&!agentDebugRaw)throw new Error('RAW grant is not active.');
 const part=debugPartSelect.value,view=debugPayloadView.value;
 const result=await agentApp.jsonFetch('/api/agent/debug/traces/'+encodeURIComponent(agentDebugTrace)+'/payload?part='+encodeURIComponent(part)+'&view='+encodeURIComponent(view));
 let body=result.body||'';if(view==='raw'&&result.raw_base64){const binary=atob(result.raw_base64);const bytes=Uint8Array.from(binary,ch=>ch.charCodeAt(0));body=new TextDecoder().decode(bytes);}
 debugOutput.textContent=JSON.stringify({part,view,total_bytes:result.total_bytes,captured_bytes:result.captured_bytes,sha256:result.sha256,truncated:result.truncated,body,warning:result.warning},null,2);
}
async function clearAgentDebug(){
 if(!window.confirm('Clear this owner’s in-memory Agent Debug traces? This does not delete sessions or native Patch data.'))return;
 await agentApp.jsonFetch('/api/agent/debug/clear',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});agentDebugTrace=null;await loadAgentDebugTraces();
}
async function exportAgentDebug(){
 const trace=await loadAgentDebugTrace();if(!trace)throw new Error('Select a trace first.');
 const blob=new Blob([JSON.stringify({trace,export_view:'safe; payload bodies omitted'},null,2)],{type:'application/json'});const link=document.createElement('a');link.href=URL.createObjectURL(blob);link.download='agent-debug-'+trace.trace_id+'.json';link.click();URL.revokeObjectURL(link.href);
}
async function copyAgentDebug(){if(!navigator.clipboard)throw new Error('Clipboard API unavailable.');await navigator.clipboard.writeText(debugOutput.textContent||'');}
debugTraceSelect.addEventListener('change',()=>{agentDebugTrace=debugTraceSelect.value;renderAgentDebugTrace().catch(showAgentError);});
async function openAgentPanel(){
 agentDock.expanded=true;applyAgentDock();saveAgentDock();
 await loadAgentState();
 if(agentDebugView==='debug'){await loadAgentDebugState();await loadAgentDebugTraces();}
 return true;
}
function closeAgentPanel(){agentDock.expanded=false;toggleAgentMore(false);applyAgentDock();saveAgentDock();}
root.addEventListener('click',e=>e.stopPropagation());
settingsOverlay.addEventListener('click',e=>{if(e.target===settingsOverlay)settingsOverlay.classList.remove('visible');});
window.addEventListener('keydown',e=>{
 if(e.key!=='Escape')return;
 if(sessionList.classList.contains('open')){e.preventDefault();toggleAgentSessions(false);sessionMenuButton.focus();}
 else if(moreMenu.classList.contains('open')){e.preventDefault();toggleAgentMore(false);}
 else if(planOverlay.classList.contains('visible'))planOverlay.classList.remove('visible');
 else if(settingsOverlay.classList.contains('visible'))settingsOverlay.classList.remove('visible');
 else if(agentDock.expanded&&agentDock.autoHide){e.preventDefault();closeAgentPanel();}
});
globalThis.TaskMenuAgentChat={open:openAgentPanel,close:closeAgentPanel,settings:openAgentSettings,
 setDockPosition:setAgentDockPosition,setAutoHide:setAgentDockAutoHide,
 get dockPosition(){return agentDock.position;},get autoHide(){return agentDock.autoHide;},
 get visible(){return agentDock.expanded;}};
if(agentDock.expanded)loadAgentState().catch(showAgentError);
if(agentApp.taskData?.workspace)loadAgentDebugState().catch(()=>{});
