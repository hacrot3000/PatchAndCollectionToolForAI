// Explicit, bounded, browser-local autocomplete diagnostics.
// OFF by default: never logs raw PTY data or sends diagnostics to the server.
const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for completion trace');
const css=document.createElement('style');
css.textContent=[
'.completion-trace-overlay{position:fixed;inset:0;z-index:19500;display:none;align-items:center;justify-content:center;padding:12px;background:#000a}',
'.completion-trace-overlay.visible{display:flex}',
'.completion-trace-panel{width:min(850px,96vw);max-height:min(84vh,820px);display:flex;flex-direction:column;min-height:0;background:#151b24;color:#e8eff7;border:1px solid #52657e;border-radius:9px;box-shadow:0 15px 50px #0009}',
'.completion-trace-heading{display:flex;align-items:center;gap:12px;padding:12px;border-bottom:1px solid #445265}',
'.completion-trace-heading strong{flex:1}',
'.completion-trace-desc{font-size:12px;padding:10px 12px;line-height:1.5;color:#bacbdd}',
'.completion-trace-actions{display:flex;flex-wrap:wrap;gap:7px;padding:9px 12px}',
'.completion-trace-actions button{padding:6px 10px}',
'.completion-trace-probe{margin:5px 12px;width:calc(100% - 24px);box-sizing:border-box;padding:8px;border:1px solid #65748a;border-radius:5px;background:#0d1621;color:inherit}',
'.completion-trace-status{padding:6px 12px;font-size:12px}',
'.completion-trace-data{flex:1;min-height:100px;white-space:pre-wrap;word-break:break-word;overflow:auto;font:11px/1.5 ui-monospace,monospace;background:#0b121b;margin:7px 12px 12px;padding:11px;border:1px solid #334259;border-radius:5px}',
'html[data-taskmenu-theme="light"] .completion-trace-panel{background:#fff;color:#253649;border-color:#becbdd}',
'html[data-taskmenu-theme="light"] .completion-trace-data{background:#f4f7fc;color:#1a2a3a;border-color:#cbd9e7}'
].join('\n');document.head.append(css);
const overlay=document.createElement('div');overlay.className='completion-trace-overlay';
const panel=document.createElement('div');panel.className='completion-trace-panel';panel.setAttribute('role','dialog');panel.setAttribute('aria-modal','true');
const heading=document.createElement('div');heading.className='completion-trace-heading';
const title=document.createElement('strong');title.textContent='Autocomplete Trace · diagnostic capture';
const x=document.createElement('button');x.type='button';x.textContent='×';x.title='Close trace panel';
heading.append(title,x);
const desc=document.createElement('div');desc.className='completion-trace-desc';
desc.textContent='Start trace, focus the terminal, type cp a_py without Enter, then click Snapshot / Probe and Download JSON. If TaskDeck fails to read the prompt, enter cp a_py in the optional probe field to test the read-only API independently. No shell commands are executed or logs uploaded. Review paths/filenames before sharing.';
const probeInput=document.createElement('input');probeInput.className='completion-trace-probe';probeInput.type='text';probeInput.placeholder='Optional API probe command when prompt is not detected: cp a_py';probeInput.spellcheck=false;probeInput.autocomplete='off';
const controls=document.createElement('div');controls.className='completion-trace-actions';
const status=document.createElement('div');status.className='completion-trace-status';
const data=document.createElement('pre');data.className='completion-trace-data';
panel.append(heading,desc,probeInput,controls,status,data);overlay.append(panel);document.body.append(overlay);
const trigger=document.createElement('button');trigger.type='button';trigger.id='completion-trace-trigger';trigger.textContent='Autocomplete Trace…';
trigger.title='Capture and export local diagnostic events for shell autocomplete';
document.querySelector('#reload')?.before(trigger);
let enabled=false,target='',began='',records=[],dropped=0,previousFocus=null;
const MAX=400,PREVIEW=60;
function localTerminal(view){
 const meta=view?.meta;
 return Boolean(meta&&meta.kind==='terminal'&&(!meta.task_id||Number(meta.task_id)===0)&&
  String(meta.target_type||'').toLowerCase()!=='ssh'&&!meta.target_profile_id&&!view.closed);
}
function activeTerminal(){
 return app.views?.get?.(String(app.active||''))||null;
}
function cleanLine(value){
 const text=String(value||'');
 // Only show a narrow, safe filename-completion command. Do not record arbitrary
 // shell input, quoted secrets, terminal output or interactive password prompts.
 return /^(?:sudo )?(?:ls|ll|cp|mv|cd|cat|find|rg|grep|git add)(?: [\w.\/@%+=:,~-]*){0,4}$/.test(text)&&text.length<=120?
  text:'[hidden: not a simple filesystem completion command]';
}
function scrub(input,depth=0){
 if(depth>3)return '[omitted]';
 if(typeof input==='string')return input.slice(0,160);
 if(typeof input==='number'||typeof input==='boolean'||input==null)return input;
 if(Array.isArray(input))return input.slice(0,10).map(v=>scrub(v,depth+1));
 if(typeof input==='object'){
  const out={};
  for(const [key,val] of Object.entries(input).slice(0,25)){
   if(/password|secret|token|raw|output|cookie|authorization|history/i.test(key))continue;
   out[key]=/^(line|typed|command|input)$/i.test(key)?cleanLine(val):scrub(val,depth+1);
  }
  return out;
 }
 return '[omitted]';
}
function capture(view,stage,details={}){
 if(!enabled||!view||String(view.meta?.id)!==target)return false;
 const safe=scrub(details),row={at:new Date().toISOString(),stage:String(stage).slice(0,70),...safe};
 const previous=records[records.length-1];
 if(previous&&previous.stage===row.stage&&JSON.stringify({...previous,at:0})===JSON.stringify({...row,at:0}))return false;
 records.push(row);
 if(records.length>MAX){records.shift();dropped++;}
 if(overlay.classList.contains('visible'))refresh();
 return true;
}
function report(){
 return {schema:'taskdeck.autocomplete.trace.v1',started_at:began,captured_until:new Date().toISOString(),
  recording:enabled,session_id:target,revision:document.querySelector('[data-revision]')?.dataset.revision||'',
  limit:MAX,dropped,privacy:'Manual, browser-memory only; filename/path samples may be private.',
  events:records.map(x=>({...x}))};
}
function refresh(){
 status.textContent=(enabled?'● Recording':'○ Stopped')+' · '+records.length+' events'+
  (dropped?' · '+dropped+' older events discarded':'')+(target?' · terminal '+target.slice(0,12):'');
 data.textContent=records.slice(-PREVIEW).map(row=>
  row.at.slice(11,23)+'  '+row.stage+'  '+JSON.stringify(Object.fromEntries(Object.entries(row).filter(([k])=>k!=='at'&&k!=='stage')))
 ).join('\n')||'No events yet. Start trace, type cp a_py in the local terminal, then Snapshot / Probe.';
}
function targetView(){return app.views.get(target)||null;}
function diagnostic(view=targetView()){
 const snapshot=globalThis.TaskDeckSmartTerminal?.diagnose?.(view);
 if(!snapshot){capture(view,'diagnostic-unavailable',{reason:'Smart terminal module or session missing'});return null;}
 capture(view,'manual-snapshot',snapshot);
 return snapshot;
}
function start(view=activeTerminal()){
 if(!localTerminal(view)){status.textContent='Choose an active local Terminal tab first (not SSH or a task).';return false;}
 target=String(view.meta.id);records=[];dropped=0;began=new Date().toISOString();enabled=true;
 capture(view,'capture-start',{status:view.meta.status,cwdHint:'Session current CWD is resolved by backend; no file contents captured'});
 diagnostic(view);refresh();
 return true;
}
function stop(){if(enabled){capture(targetView(),'capture-stop');enabled=false;}refresh();}
function clear(){records=[];dropped=0;refresh();}
async function probe(){
 const view=targetView(),snap=diagnostic(view);
 if(!view||!snap)return;
 const context=(probeInput.value.trim()?globalThis.TaskDeckCompletionEngine?.pathContext?.(probeInput.value.trim(),{}):null)||
  snap.analysis?.pathContext||snap.analysis?.branchContext||snap.pathContext;
 if(!localTerminal(view)||!context||!['paths','branches'].includes(context.kind||'paths')){
  capture(view,'manual-probe-skipped',{reason:'No safe filesystem completion context; inspect manual-snapshot'});return;
 }
 if(!view.canControl||view.tabReadOnly||app.browserLeaseLost){capture(view,'manual-probe-skipped',{reason:'Session not writable'});return;}
 const kind=context.kind||'paths',dir=context.dir||'',prefix=context.prefix||'';
 capture(view,'manual-probe-request',{kind,dir,prefix});
 try{
  const res=await app.jsonFetch('/api/sessions/'+encodeURIComponent(view.meta.id)+'/completion-paths?'+
   new URLSearchParams({kind,dir,prefix}));
  const names=Array.isArray(res?.entries)?res.entries:Array.isArray(res?.branches)?res.branches:[];
  capture(view,'manual-probe-response',{kind,dir,prefix,count:names.length,
   entries:names.slice(0,10).map(row=>typeof row==='string'?row:{name:row?.name,dir:Boolean(row?.dir)}),
   cwd:res?.cwd||''});
 }catch(error){capture(view,'manual-probe-error',{kind,dir,prefix,error:String(error?.message||error)});}
}
function close(){
 overlay.classList.remove('visible');
 if(previousFocus?.isConnected)previousFocus.focus?.();
}
function open(){
 if(!overlay.classList.contains('visible'))previousFocus=document.activeElement;
 overlay.classList.add('visible');refresh();
}
function action(label,fn){
 const button=document.createElement('button');button.type='button';button.textContent=label;button.onclick=fn;controls.append(button);return button;
}
const startButton=action('Start trace',()=>{if(start())open();});
action('Stop',stop);
action('Snapshot',()=>diagnostic());
action('Probe now (read-only)',()=>void probe());
action('Copy JSON',async()=>{
 try{await navigator.clipboard.writeText(JSON.stringify(report(),null,2));status.textContent='Trace JSON copied';}
 catch(error){status.textContent='Clipboard unavailable: use Download JSON';}
});
action('Download JSON',()=>{
 const content=JSON.stringify(report(),null,2);
 const objectURL=URL.createObjectURL(new Blob([content],{type:'application/json'}));
 const a=document.createElement('a');a.href=objectURL;a.download='taskdeck-autocomplete-trace-'+
  new Date().toISOString().replace(/[:.]/g,'-')+'.json';document.body.append(a);a.click();a.remove();
 setTimeout(()=>URL.revokeObjectURL(objectURL),1000);
});
action('Clear',clear);
trigger.onclick=open;x.onclick=close;overlay.onmousedown=e=>{if(e.target===overlay)close();};
overlay.addEventListener('keydown',e=>{if(e.key==='Escape'){e.preventDefault();e.stopPropagation();close();}});
window.addEventListener('taskmenu:output',e=>capture(e.detail?.view,'pty-output-arrived',{bytes:e.detail?.data?.byteLength||e.detail?.data?.length||0}));
window.addEventListener('taskmenu:terminal-rendered',e=>capture(e.detail?.view,'xterm-write-finished'));
window.addEventListener('taskmenu:shell-integration',e=>{
 const d=e.detail||{};
 capture(d.view,'shell-'+String(d.type||'unknown'),{seenPrompt:Boolean(d.state?.seenPrompt),
  executing:Boolean(d.state?.executing),commandRow:d.state?.commandRow,commandCol:d.state?.commandCol,
  promptRow:d.state?.promptRow});
});
globalThis.TaskDeckCompletionTrace={open,start,stop,clear,report,capture,diagnostic,probe,
 get enabled(){return enabled;},get target(){return target;},get count(){return records.length;}};
