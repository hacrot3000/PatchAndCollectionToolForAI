// Smart Terminal L1/L2: explicit advisory requests only. No run-command API.
const aiApp=globalThis.TaskMenuApp;
if(!aiApp)throw new Error('TaskMenuApp unavailable for AI Assistant');
const aiStyle=document.createElement('style');
aiStyle.textContent=[
'.taskdeck-ai-overlay{display:none;position:fixed;inset:0;z-index:17000;background:rgba(0,0,0,.58);padding:5vh 12px;justify-content:center;align-items:flex-start}',
'.taskdeck-ai-overlay.visible{display:flex}',
'.taskdeck-ai-dialog{display:flex;flex-direction:column;gap:10px;max-height:90vh;overflow:auto;width:min(680px,96vw);padding:16px;background:#151b25;border:1px solid #556579;border-radius:10px;color:#e8f1fa;box-shadow:0 12px 48px #0009}',
'.taskdeck-ai-dialog h2{font-size:16px;margin:0}.taskdeck-ai-dialog p{font-size:12px;margin:0;line-height:1.5}',
'.taskdeck-ai-dialog label{display:flex;align-items:center;gap:8px;font-size:12px}',
'.taskdeck-ai-dialog .ai-config-row{display:grid;grid-template-columns:155px minmax(0,1fr);align-items:center;gap:10px}',
'.taskdeck-ai-dialog input:not([type=checkbox]),.taskdeck-ai-dialog select{box-sizing:border-box;width:100%;min-width:0;padding:7px;background:#0e1722;border:1px solid #556579;border-radius:5px;color:#e9f3fd}',
'.taskdeck-ai-dialog .ai-config-actions{display:flex;justify-content:flex-end;gap:8px}',
'.taskdeck-ai-dialog button{padding:6px 10px}',
'.taskdeck-ai-guide{display:flex;flex-direction:column;gap:6px;border:1px solid #445873;border-radius:7px;padding:8px;font-size:12px}',
'.taskdeck-ai-guide-body{display:flex;flex-wrap:wrap;gap:8px;align-items:center}',
'.taskdeck-ai-guide-body[hidden]{display:none}',
'.taskdeck-ai-guide-body p{flex-basis:100%;line-height:1.5}',
'.taskdeck-ai-guide-body a{color:#a4d7ff;text-decoration:underline}',
'.taskdeck-ai-panel{position:fixed;right:0;top:37px;bottom:0;z-index:1300;display:none;flex-direction:column;width:min(440px,40vw);min-width:300px;background:#141b24;border-left:1px solid #5e7085;box-shadow:-8px 0 24px #0006;color:#e8f1fa}',
'.taskdeck-ai-panel.visible{display:flex}',
'.taskdeck-ai-head{padding:10px;display:flex;align-items:center;gap:6px;border-bottom:1px solid #3d4d60}',
'.taskdeck-ai-head strong{flex:1}.taskdeck-ai-panel textarea{min-height:70px;max-height:130px;resize:vertical}',
'.taskdeck-ai-panel textarea,.taskdeck-ai-panel select{box-sizing:border-box;width:100%;padding:7px;background:#0e1722;border:1px solid #536477;border-radius:5px;color:inherit;font:12px ui-monospace,monospace}',
'.taskdeck-ai-scroll{overflow:auto;flex:1;min-height:0;padding:10px;display:flex;flex-direction:column;gap:9px}',
'.taskdeck-ai-panel p{font-size:11px;line-height:1.5;margin:0}.taskdeck-ai-panel pre{font:12px/1.55 ui-monospace,monospace;white-space:pre-wrap;overflow-wrap:anywhere;margin:0}',
'.taskdeck-ai-answer{background:#1c2b3d;padding:9px;border-radius:6px;white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px;line-height:1.5}',
'.taskdeck-ai-actions{display:flex;flex-wrap:wrap;gap:6px}.taskdeck-ai-actions button{font-size:11px;padding:5px 9px}',
'.taskdeck-ai-status{font-size:11px;opacity:.8}.taskdeck-ai-warning{color:#f1c46c}',
'.taskdeck-ai-context{border:1px solid #40516b;border-radius:6px;padding:8px;max-height:170px;overflow:auto}',
'html[data-taskmenu-theme="light"] .taskdeck-ai-dialog,html[data-taskmenu-theme="light"] .taskdeck-ai-panel{background:#fff;color:#1b2b3e;border-color:#afbfcd}',
'html[data-taskmenu-theme="light"] .taskdeck-ai-dialog input:not([type=checkbox]),html[data-taskmenu-theme="light"] .taskdeck-ai-dialog select,html[data-taskmenu-theme="light"] .taskdeck-ai-panel textarea,html[data-taskmenu-theme="light"] .taskdeck-ai-panel select{background:#f8fbff;color:#1b2b3e;border-color:#a8b6c8}',
'html[data-taskmenu-theme="light"] .taskdeck-ai-answer{background:#edf4fc}',
'@media(max-width:900px){.taskdeck-ai-panel{width:min(95vw,440px);min-width:0;top:32px}}'
].join('\n');document.head.append(aiStyle);
function aiElement(tag,cls,text){
 const el=document.createElement(tag);if(cls)el.className=cls;if(text!==undefined)el.textContent=text;return el;
}
const configOverlay=aiElement('div','taskdeck-ai-overlay');
const configDialog=aiElement('div','taskdeck-ai-dialog');configDialog.setAttribute('role','dialog');configDialog.setAttribute('aria-modal','true');
const configTitle=aiElement('h2','', 'AI Assistant settings');
const privacy=aiElement('p','taskdeck-ai-warning','Optional. Your selected command/output may be sent to the configured external AI provider. No shell commands are executed by AI.');
const conf={};
function field(key,label,kind='text',value=''){
 const row=aiElement('div','ai-config-row'),title=aiElement('label','',label);
 const input=aiElement(kind==='select'?'select':'input');if(kind!=='select')input.type=kind;input.value=value;input.id='taskdeck-ai-'+key;title.htmlFor=input.id;
 row.append(title,input);configDialog.append(row);conf[key]=input;return input;
}
configDialog.append(configTitle,privacy);
conf.enabled=field('enabled','Enable AI Assistant','checkbox');
conf.enabled.checked=false;
const providerCatalog={
 openai:{label:'OpenAI',defaultModel:'gpt-5.4-mini',models:['gpt-5.4-mini','gpt-5.4','gpt-4.1-mini','gpt-4.1','gpt-4o-mini'],keyURL:'https://platform.openai.com/api-keys',modelURL:'https://developers.openai.com/api/docs/models',steps:'Sign in to OpenAI Platform → API keys → Create new secret key. Set up API billing or project budget if required. ChatGPT subscriptions do not automatically include API credits.'},
 anthropic:{label:'Anthropic / Claude',defaultModel:'claude-sonnet-5-5',models:['claude-sonnet-5-5','claude-haiku-5-5','claude-opus-5-5','claude-sonnet-5'],keyURL:'https://console.anthropic.com/settings/keys',modelURL:'https://platform.claude.com/docs/en/about-claude/models/overview',steps:'Sign in to Claude Console → Settings → API keys → Create key. Configure workspace and API credits if needed. A Claude chat subscription is separate from API billing.'},
 gemini:{label:'Google Gemini',defaultModel:'gemini-3.8-flash',models:['gemini-3.8-flash','gemini-3.5-flash-lite','gemini-2.5-flash'],keyURL:'https://aistudio.google.com/api-keys',modelURL:'https://ai.google.dev/gemini-api/docs/models',steps:'Sign in to Google AI Studio → API keys → create or select a project → Create API key. Verify API eligibility/billing and use a current auth key.'},
 deepseek:{label:'DeepSeek',defaultModel:'deepseek-chat',models:['deepseek-chat','deepseek-reasoner'],keyURL:'https://platform.deepseek.com/api_keys',modelURL:'https://api-docs.deepseek.com/',steps:'Sign in to DeepSeek Platform → API Keys → Create API key. Check balance or recharge if required.'},
 groq:{label:'GroqCloud',defaultModel:'llama-3.3-70b-versatile',models:['llama-3.3-70b-versatile','llama-3.1-8b-instant','openai/gpt-oss-120b'],keyURL:'https://console.groq.com/keys',modelURL:'https://console.groq.com/docs/models',steps:'Sign in to GroqCloud Console → API Keys → Create API key. Select a supported production text model; availability depends on the account.'},
 openrouter:{label:'OpenRouter',defaultModel:'openai/gpt-5.4-mini',models:['openai/gpt-5.4-mini','anthropic/claude-sonnet-5-5','google/gemini-3.8-flash'],keyURL:'https://openrouter.ai/settings/keys',modelURL:'https://openrouter.ai/models',steps:'Sign in to OpenRouter → Keys → Create key. Add credits as needed. Copy an exact model ID from the Models catalog; model pricing/availability varies.'}
};
conf.provider=field('provider','AI provider','select','openai');
for(const [id,info] of Object.entries(providerCatalog)){
 const option=aiElement('option','',info.label);option.value=id;conf.provider.append(option);
}
conf.provider.value='openai';
conf.model=field('model','Model (choose or type)','text','gpt-5.4-mini');
const modelSuggestions=aiElement('datalist');modelSuggestions.id='taskdeck-ai-model-suggestions';
conf.model.setAttribute('list',modelSuggestions.id);
configDialog.append(modelSuggestions);
const providerKeyGuide=aiElement('div','taskdeck-ai-guide');
const guideToggle=aiElement('button','','How to get an API key · Model list');
guideToggle.type='button';guideToggle.setAttribute('aria-expanded','false');
const guideBody=aiElement('div','taskdeck-ai-guide-body');guideBody.hidden=true;
const keySteps=aiElement('p','');
const keyLink=aiElement('a','','Open API key page ↗');keyLink.target='_blank';keyLink.rel='noopener noreferrer';
const modelsLink=aiElement('a','','Official models ↗');modelsLink.target='_blank';modelsLink.rel='noopener noreferrer';
guideBody.append(keySteps,keyLink,modelsLink);
providerKeyGuide.append(guideToggle,guideBody);
function refreshProviderGuide(resetModel){
 const info=providerCatalog[conf.provider.value]||providerCatalog.openai;
 modelSuggestions.replaceChildren();
 for(const name of info.models){const option=aiElement('option');option.value=name;modelSuggestions.append(option);}
 if(resetModel)conf.model.value=info.defaultModel;
 keySteps.textContent=info.steps+' Copy the secret once and paste it into the write-only API key field below. Never paste API keys into terminal output or chat.';
 keyLink.href=info.keyURL;modelsLink.href=info.modelURL;
 conf.model.placeholder='Choose a suggestion or enter an exact '+info.label+' model ID';
 configNotice.textContent='Provider: '+info.label+' · fixed HTTPS API endpoint · requests remain suggestion-only. Model names and access may change.';
}
guideToggle.onclick=()=>{guideBody.hidden=!guideBody.hidden;guideToggle.setAttribute('aria-expanded',String(!guideBody.hidden));};
conf.provider.onchange=()=>{refreshProviderGuide(true);conf.api_key.value='';conf.clear_api_key.checked=false;configStatus.textContent='Provider changed. Save settings to use it; enter its own API key if not configured.';};

conf.api_key=field('api_key','API key (write-only)','password');
conf.api_key.autocomplete='new-password';conf.api_key.placeholder='Leave blank to keep existing encrypted key';
conf.clear_api_key=field('clear_api_key','Remove stored API key','checkbox');
conf.timeout_seconds=field('timeout_seconds','Request timeout (seconds)','number','25');
conf.max_output_bytes=field('max_output_bytes','Output limit (bytes)','number','5000');
conf.requests_per_hour=field('requests_per_hour','Max requests / hour / user','number','20');
conf.include_cwd=field('include_cwd','Include working directory','checkbox');
conf.include_history=field('include_history','Include recent command history','checkbox');
conf.include_git=field('include_git','Include Git branch summary','checkbox');
conf.consent=field('consent','Allow external provider calls','checkbox');
const configNotice=aiElement('p','', 'Requests use a fixed, provider-specific HTTPS endpoint. No AI command execution.');
refreshProviderGuide(false);
const configStatus=aiElement('p','taskdeck-ai-status','');
const configActions=aiElement('div','ai-config-actions');
const configCancel=aiElement('button','', 'Cancel'),configSave=aiElement('button','','Save settings');
configCancel.type=configSave.type='button';configActions.append(configCancel,configSave);
configDialog.append(providerKeyGuide,configNotice,configStatus,configActions);configOverlay.append(configDialog);document.body.append(configOverlay);
const panel=aiElement('aside','taskdeck-ai-panel');panel.setAttribute('aria-label','AI Smart Terminal Assistant');
const header=aiElement('div','taskdeck-ai-head');
const headTitle=aiElement('strong','','AI Smart Terminal');
const panelSettings=aiElement('button','','Settings'),panelClose=aiElement('button','','×');
panelSettings.type=panelClose.type='button';panelClose.setAttribute('aria-label','Close AI panel');
header.append(headTitle,panelSettings,panelClose);
const scroll=aiElement('div','taskdeck-ai-scroll');
const explanation=aiElement('p','','The AI can explain terminal output and suggest commands. It cannot execute them. Review all proposed commands yourself.');
const activeLabel=aiElement('p','taskdeck-ai-status','Select a terminal first.');
const mode=aiElement('select');
for(const [value,label] of [['explain','Explain error or command'],['suggest','Suggest fix'],['plan','Guided plan (preview only)']]){
 const option=aiElement('option','',label);option.value=value;mode.append(option);
}
const question=aiElement('textarea');question.placeholder='Ask about the current terminal, last command or error…';
const aiActions=aiElement('div','taskdeck-ai-actions');
const selectContext=aiElement('button','','Preview context');
const submit=aiElement('button','','Ask AI');
const abort=aiElement('button','','Cancel request');abort.disabled=true;
const copy=aiElement('button','','Copy answer');copy.disabled=true;
aiActions.append(selectContext,submit,abort,copy);
const preview=aiElement('pre','taskdeck-ai-context');preview.hidden=true;
const status=aiElement('p','taskdeck-ai-status','No request sent.');
const answer=aiElement('pre','taskdeck-ai-answer');answer.hidden=true;
scroll.append(explanation,activeLabel,mode,question,aiActions,preview,status,answer);
panel.append(header,scroll);document.body.append(panel);
let settings={enabled:false,provider:'openai',model:'gpt-5.4-mini',timeout_seconds:25,max_output_bytes:5000};
let controller=null;let latestPlanText='';let latestPlanTarget=null;
const options=['enabled','provider','model','timeout_seconds','max_output_bytes','requests_per_hour','include_cwd','include_history','include_git','consent'];
function gatherSettings(){
 const value={};
 for(const key of options)value[key]=conf[key].type==='checkbox'?conf[key].checked:(conf[key].type==='number'?Number(conf[key].value):conf[key].value);
 value.api_key=conf.api_key.value;value.clear_api_key=conf.clear_api_key.checked;return value;
}
async function getSettings(){
 settings=await aiApp.jsonFetch('/api/ai/settings');
 for(const key of options){
  if(conf[key].type==='checkbox')conf[key].checked=Boolean(settings[key]);
  else conf[key].value=String(settings[key]??'');
 }
 conf.api_key.value='';conf.clear_api_key.checked=false;
 refreshProviderGuide(false);
 configNotice.textContent='Provider: '+(providerCatalog[settings.provider]?.label||'OpenAI')+' · stored API key: '+(settings.has_api_key?'configured':'missing')+'. L1/L2 suggestions only; L3 execution disabled.';
 return settings;
}
async function saveSettings(){
 if(!(!aiApp.sharedMode||aiApp.hasPermission?.('settings.write'))){throw Error('Settings write access required');}
 configSave.disabled=true;
 try{
  settings=await aiApp.jsonFetch('/api/ai/settings',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify(gatherSettings())});
  conf.api_key.value='';configStatus.textContent='Saved. Secret value will not be shown.';
  closeSettings();
 }finally{configSave.disabled=false;}
}
async function openSettings(){
 await getSettings();configStatus.textContent='';
 const editable=!aiApp.sharedMode||Boolean(aiApp.hasPermission?.('settings.write'));
 configSave.disabled=!editable;
 configStatus.textContent=editable?'':'Read-only: settings.write permission required to modify AI configuration.';
 for(const input of Object.values(conf))input.disabled=!editable;
 configOverlay.classList.add('visible');conf.enabled.focus();return true;
}
function closeSettings(){configOverlay.classList.remove('visible');conf.api_key.value='';}
configSave.onclick=()=>saveSettings().catch(aiApp.showError);
configCancel.onclick=closeSettings;
configOverlay.addEventListener('mousedown',event=>{if(event.target===configOverlay)closeSettings();});
configDialog.addEventListener('keydown',event=>{if(event.key==='Escape'){event.stopPropagation();event.preventDefault();closeSettings();}});
function selectedTerminal(){
 let id=String(aiApp.active||'');
 if(id.startsWith('external:'))id=id.slice('external:'.length);
 return aiApp.views.get(id)||null;
}
function makeContext(){
 const view=selectedTerminal();
 if(!view)return null;
 const state=globalThis.TaskDeckShellIntegration?.getState?.(view.meta.id);
 const commands=globalThis.TaskDeckShellIntegration?.getCommands?.(view.meta.id)||[];
 const last=commands[commands.length-1]||{};
 const remote=Boolean(view.meta.target_type==='ssh'||view.meta.target_profile_id);
 // No full scrollback, environment variables, files or raw terminal transcript.
 const request={
  session_id:String(view.meta.id),
  mode:mode.value,question:question.value.trim(),command:String(last.command||state?.activeCommand||'').slice(0,2000),
  output:String(last.output||'').slice(-Math.min(Number(settings.max_output_bytes)||5000,12000)),
  cwd:settings.include_cwd?String(state?.cwd||view.meta.cwd||'').slice(0,1024):'',
  history:settings.include_history?commands.slice(-8).map(x=>x.command).join('\n').slice(0,4000):'',
  git:settings.include_git?String(document.querySelector('.git-status-pill')?.textContent||'').slice(0,1500):'',
  remote,confirm_external:false
 };
 return request;
}
function redactPreview(v){
 let s=String(v||'').replace(/\x1b\[[0-?]*[ -/]*[@-~]/g,'');
 s=s.replace(/(authorization\s*:\s*bearer\s+)\S+/ig,'$1[REDACTED]');
 s=s.replace(/(api[_-]?key|password|token|secret)\s*[:=]\s*[^\s,;]+/ig,'$1=[REDACTED]');
 s=s.replace(/-----BEGIN (?:OPENSSH |RSA |EC |)PRIVATE KEY-----[\s\S]*?-----END (?:OPENSSH |RSA |EC |)PRIVATE KEY-----/g,'[REDACTED PRIVATE KEY]');
 return s;
}
function previewContext(){
 const data=makeContext();if(!data){status.textContent='Select a terminal tab first.';return null;}
 const text=JSON.stringify({...data,confirm_external:undefined},null,2);
 preview.hidden=false;preview.textContent=redactPreview(text);activeLabel.textContent='Target: '+(data.remote?'SSH':'local')+' · '+String(selectedTerminal()?.meta.label||selectedTerminal()?.meta.id||'terminal');
 return data;
}
// Verify the terminal via the server before displaying or sending context.
// Only session identity, host and launch CWD are authenticated. Shell output
// remains untrusted user-selected text.
async function verifiedPreviewContext(){
 const data=previewContext();if(!data)return null;
 const verified=await aiApp.jsonFetch('/api/ai/session-context?session_id='+encodeURIComponent(data.session_id));
 if(!verified.verified||String(verified.session_id)!==data.session_id)throw new Error('Terminal identity could not be verified');
 if(String(selectedTerminal()?.meta?.id||'')!==data.session_id)throw new Error('Active terminal changed during context verification');
 data.remote=Boolean(verified.remote);
 if(settings.include_cwd)data.cwd=String(verified.launch_cwd||'').slice(0,1024);
 const review={...data,confirm_external:undefined,verified_target:{
  host:verified.host,remote:verified.remote,session_id:verified.session_id,
  launch_cwd:verified.launch_cwd,
  note:'Server-verified session identity. Interactive shell output and recent history are untrusted.'
 }};
 preview.textContent=redactPreview(JSON.stringify(review,null,2));
 activeLabel.textContent='Verified: '+(verified.remote?'SSH':'local')+' · '+String(verified.host||'local');
 status.textContent='Server verified terminal target. Review the content carefully before any external request.';
 return {data,verified};
}
async function sendRequest(){
 if(!settings.enabled||!settings.consent){status.textContent='Enable AI Assistant and external-provider consent in Settings first.';return;}
 const checked=await verifiedPreviewContext();if(!checked)return;
 const {data,verified}=checked;
 latestPlanText='';latestPlanTarget=null;
 if(data.mode==='plan'){
  latestPlanTarget={session_id:String(verified.session_id||''),host:String(verified.host||'local').slice(0,256),cwd:String(verified.launch_cwd||'').slice(0,1024),remote:Boolean(verified.remote)};
 }
 if(!window.confirm('Send the previewed terminal context to '+(providerCatalog[settings.provider]?.label||settings.provider)+'? Avoid sending credentials, private files or production secrets. No command will be executed.'))return;
 data.confirm_external=true;
 controller=new AbortController();submit.disabled=true;abort.disabled=false;status.textContent='Waiting for AI response…';
 try{
  const result=await aiApp.jsonFetch('/api/ai/assist',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data),signal:controller.signal});
  answer.textContent=String(result.answer||'No response');answer.hidden=false;copy.disabled=false;
  if(data.mode==='plan')latestPlanText=String(result.answer||'');
  status.textContent='Suggestion only · '+String(result.provider||'AI provider')+' · No command executed';
 }catch(e){status.textContent=e?.name==='AbortError'?'Request cancelled.':String(e?.message||e);}
 finally{controller=null;submit.disabled=false;abort.disabled=true;}
}
async function openPanel(){await getSettings();panel.classList.add('visible');preview.hidden=true;status.textContent='Choose a terminal and explicitly preview context before sending.';return true;}
function closePanel(){panel.classList.remove('visible');controller?.abort();}
panelClose.onclick=closePanel;panelSettings.onclick=()=>openSettings().catch(aiApp.showError);
submit.onclick=()=>sendRequest().catch(aiApp.showError);
selectContext.onclick=()=>verifiedPreviewContext().catch(aiApp.showError);
abort.onclick=()=>controller?.abort();
copy.onclick=()=>globalThis.navigator?.clipboard?.writeText?.(answer.textContent||'').catch(aiApp.showError);
window.addEventListener('taskmenu:view-activated',()=>{if(panel.classList.contains('visible')){activeLabel.textContent='Target changed · preview context again';preview.hidden=true;latestPlanText='';latestPlanTarget=null;}});
globalThis.TaskMenuAI={openSettings,closeSettings,openPanel,closePanel,previewContext,
 getLastPlan(){return latestPlanText&&latestPlanTarget?{text:latestPlanText,target:{...latestPlanTarget}}:null;},
 get settings(){return {...settings}},get visible(){return panel.classList.contains('visible')}};
