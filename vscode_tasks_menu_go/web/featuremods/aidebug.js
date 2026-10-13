// Temporary, in-page AI request diagnostics. No telemetry or network upload.
// The user explicitly reviews and copies the redacted report.
(function(){
 const css=document.createElement('style');
 css.textContent=[
  '.taskdeck-ai-debug{display:flex;flex-direction:column;gap:7px;margin-top:10px;padding:9px;border:1px solid #66798f;border-radius:7px;background:#192534}',
  '.taskdeck-ai-debug-header{display:flex;align-items:center;gap:6px;flex-wrap:wrap}',
  '.taskdeck-ai-debug-header strong{flex:1;font-size:12px}',
  '.taskdeck-ai-debug pre{font:11px/1.5 ui-monospace,monospace;white-space:pre-wrap;overflow-wrap:anywhere;max-height:320px;overflow:auto;padding:8px;margin:0;background:#101925;border-radius:5px}',
  '.taskdeck-ai-debug p{font-size:11px;margin:0;line-height:1.5}',
  '.taskdeck-ai-debug button{font-size:11px;padding:5px 8px}',
  '.taskdeck-ai-debug-warning{color:#f2c67b}',
  'html[data-taskmenu-theme="light"] .taskdeck-ai-debug{background:#f4f8fe;border-color:#a4b6c9}',
  'html[data-taskmenu-theme="light"] .taskdeck-ai-debug pre{background:#eaf0f9;color:#18283b}',
  'html[data-taskmenu-theme="light"] .taskdeck-ai-debug-warning{color:#86570e}'
 ].join('\n');document.head.append(css);

 const field=(tag,cls,value)=>{const node=document.createElement(tag);if(cls)node.className=cls;if(value!==undefined)node.textContent=value;return node;};
 function redact(text){
  let s=String(text??'');
  s=s.replace(/\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))/g,'');
  s=s.replace(/-----BEGIN (?:OPENSSH |RSA |EC |DSA )?PRIVATE KEY-----[\s\S]*?-----END (?:OPENSSH |RSA |EC |DSA )?PRIVATE KEY-----/gi,'[REDACTED PRIVATE KEY]');
  s=s.replace(/(authorization\s*[:=]\s*bearer\s+)[^\s'",}]+/gi,'$1[REDACTED]');
  s=s.replace(/((?:api[_-]?key|access[_-]?token|refresh[_-]?token|password|passwd|secret|client_secret|private_key)\s*[:=]\s*["']?)[^"',\s;}\]]+/gi,'$1[REDACTED]');
  s=s.replace(/\b(?:sk-[a-z0-9_-]{12,}|sk-ant-[a-z0-9_-]{12,}|gh[pousr]_[a-z0-9]{20,}|github_pat_[a-z0-9_]{20,})\b/gi,'[REDACTED TOKEN]');
  return s;
 }
 function sanitize(value,depth=0){
  if(depth>7)return '[NESTING LIMIT]';
  if(value===null||value===undefined||typeof value==='number'||typeof value==='boolean')return value??null;
  if(typeof value==='string'){
   const trimmed=value.length>14000?value.slice(0,14000)+'\n[TRUNCATED]':value;
   return redact(trimmed);
  }
  if(Array.isArray(value))return value.slice(0,30).map(item=>sanitize(item,depth+1));
  if(typeof value==='object'){
   const out={};
   for(const [key,item] of Object.entries(value).slice(0,70)){
    if(/^(?:api[_-]?key|authorization|secret|password|token|access[_-]?token|refresh[_-]?token|private[_-]?key|cookie|headers)$/i.test(key)){
     out[key]='[REDACTED]';
    }else out[key]=sanitize(item,depth+1);
   }
   return out;
  }
  return '[UNSUPPORTED]';
 }
 function create(parent,app){
  const box=field('section','taskdeck-ai-debug');
  box.setAttribute('aria-label','AI request debug');
  const head=field('div','taskdeck-ai-debug-header');
  const title=field('strong','','Request Debug · temporary');
  const copy=field('button','','Copy debug report');
  const clear=field('button','','Clear');
  copy.type=clear.type='button';copy.disabled=true;
  head.append(title,copy,clear);
  const warning=field('p','taskdeck-ai-debug-warning',
   'Review before sharing. Known credentials are masked automatically, but commands/logs may still contain private information.');
  const pre=field('pre','', 'No request yet.');
  box.append(head,warning,pre);parent.append(box);
  let report=null;
  const now=()=>new Date().toISOString();
  const render=()=>{
   if(!report){pre.textContent='No request yet.';copy.disabled=true;return;}
   pre.textContent=JSON.stringify(sanitize(report),null,2);
   copy.disabled=false;
  };
  function begin(stage,req,view,settings){
   report={
    report_type:'TaskDeck AI Assistant request (redacted)',
    created_at:now(),status:'in_progress',stage,
    provider:String(settings?.provider||''),model:String(settings?.model||''),
    client_session:{
     active_tab_id:String(app.active||''),
     view_id:String(view?.meta?.id||''),
     kind:String(view?.meta?.kind||''),
     task_id:view?.meta?.task_id??null,
     status:String(view?.meta?.status||''),
     target_type:String(view?.meta?.target_type||''),
     cwd:String(view?.meta?.cwd||'')
    },
    request:{method:'GET',path:'/api/ai/session-context',payload:req?sanitize({...req,confirm_external:false}):null},
    stages:[{at:now(),name:stage}],
    response:null
   };
   render();
  }
  function step(stage,meta){
   if(!report)return;
   report.stage=stage;
   report.stages.push({at:now(),name:stage, ...(meta?{detail:sanitize(meta)}:{})});
   render();
  }
  async function failure(stage,error,{probe=false}={}){
   if(!report)begin(stage,null,null,{});
   const code=Number(error?.httpStatus||error?.status||0);
   report.status='failed';report.stage=stage;
   report.response={at:now(),http_status:code||null,status_text:String(error?.httpStatusText||''),error_message:String(error?.message||error||'Unknown error'),error_name:String(error?.name||'Error')};
   step(stage+'_failed');
   if(probe){
    try{
     const response=await app.jsonFetch('/api/sessions');
     const sessions=Array.isArray(response?.sessions)?response.sessions:[];
     const id=report.request?.payload?.session_id;
     const matched=sessions.find(item=>String(item.id)===String(id));
     report.session_probe={
      endpoint:'/api/sessions',
      visible_session_count:sessions.length,
      requested_id_found:!!matched,
      match:matched?{id:matched.id,kind:matched.kind||'',task_id:matched.task_id,status:matched.status,target_type:matched.target_type||''}:null,
      diagnosis:!matched?'Requested ID is absent from currently visible live sessions (stale tab, server restart, or wrong server).':
       (matched.kind===''?'Legacy terminal kind is empty; verify single-user compatibility.':'Session exists; inspect kind, status and authorization.')
     };
    }catch(probeErr){
     report.session_probe={error:String(probeErr?.message||probeErr)};
    }
   }
   render();
  }
  function success(stage,info){
   if(!report)return;
   report.status='success';report.stage=stage;report.response={at:now(),...(info||{}),executed:false};
   step(stage+'_success');
  }
  copy.onclick=async()=>{
   if(!report)return;
   try{
    await navigator.clipboard.writeText(pre.textContent);
    copy.textContent='Copied';
   }catch(error){
    copy.textContent='Copy failed';
    warning.textContent='Clipboard failed: '+String(error?.message||error)+'. Select report text manually to copy.';
   }
  };
  clear.onclick=()=>{report=null;copy.textContent='Copy debug report';render();};
  return {begin,step,failure,success,clear:()=>{report=null;render();},getReport:()=>sanitize(report)};
 }
 globalThis.TaskDeckAIRequestDebug={create,redact,sanitize};
})();
