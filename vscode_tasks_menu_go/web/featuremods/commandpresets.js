const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for command presets');

let state={version:1,presets:[]};
let fingerprint='';
const runs=new Map();
const decoder=new TextDecoder();

function normalizeState(next){
  return {
    version:Number(next?.version||1),
    presets:Array.isArray(next?.presets)?next.presets.map(preset=>({
      id:String(preset?.id||''),
      name:String(preset?.name||''),
      commands:Array.isArray(preset?.commands)?preset.commands.map(String):[]
    })).filter(preset=>preset.id&&preset.name&&preset.commands.length):[]
  };
}

function applyState(next){
  const normalized=normalizeState(next);
  const nextFingerprint=JSON.stringify(normalized);
  if(nextFingerprint===fingerprint)return;
  state=normalized;fingerprint=nextFingerprint;
  window.dispatchEvent(new CustomEvent('taskmenu:command-presets',{detail:{state}}));
}

async function refreshState(){
  applyState(await app.jsonFetch('/api/command-presets'));
  return state;
}

async function mutate(payload){
  const next=await app.jsonFetch('/api/command-presets',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify(payload)
  });
  applyState(next);
  return state;
}

function randomToken(){
  const bytes=new Uint8Array(12);
  if(globalThis.crypto?.getRandomValues)globalThis.crypto.getRandomValues(bytes);
  else for(let i=0;i<bytes.length;i++)bytes[i]=Math.floor(Math.random()*256);
  return [...bytes].map(value=>value.toString(16).padStart(2,'0')).join('');
}

function shellFlavor(view){
  const preview=String(view?.meta?.command_preview||'').trim().toLowerCase();
  const executable=(preview.match(/^'?([^'\s]+)'?/)?.[1]||preview).split('/').pop()||'';
  if(executable==='fish')return 'fish';
  if(['bash','sh','zsh','dash','ksh','ash'].includes(executable))return 'posix';
  return '';
}

function markerPrefix(token,index){return '\x1eVTM_PRESET:'+token+':'+index+':';}
function markerPattern(token,index){
  return new RegExp('\\x1eVTM_PRESET:'+token+':'+index+':(-?\\d+)\\x1f');
}

function commandPayload(view,command,token,index){
  const flavor=shellFlavor(view);
  if(!flavor)throw new Error('Preset commands support bash/sh/zsh/dash/ksh/ash/fish terminals. Current shell: '+String(view?.meta?.command_preview||'unknown'));
  if(flavor==='fish'){
    return 'begin\n'+command+'\nset __vtm_preset_rc $status\nprintf \'\\036VTM_PRESET:'+token+':'+index+':%d\\037\' $__vtm_preset_rc\nend\r';
  }
  return '{\n'+command+'\n__vtm_preset_rc=$?\nprintf \'\\036VTM_PRESET:'+token+':'+index+':%d\\037\' "$__vtm_preset_rc"\n}\r';
}

function decodeOutput(data){
  if(typeof data==='string')return data;
  if(data instanceof ArrayBuffer)return decoder.decode(new Uint8Array(data));
  if(ArrayBuffer.isView(data))return decoder.decode(new Uint8Array(data.buffer,data.byteOffset,data.byteLength));
  return String(data??'');
}

function rejectRun(run,error){
  if(run.finished)return;
  run.finished=true;
  if(run.waiter){const waiter=run.waiter;run.waiter=null;waiter.reject(error);}
}

function feedOutput(view,data){
  const run=runs.get(view?.meta?.id);if(!run||run.finished||!run.waiter)return;
  run.buffer=(run.buffer+decodeOutput(data)).slice(-32768);
  const match=run.waiter.pattern.exec(run.buffer);
  if(!match)return;
  const waiter=run.waiter;run.waiter=null;run.buffer='';
  waiter.resolve(Number(match[1]));
}

window.addEventListener('taskmenu:output',event=>{
  const view=event.detail?.view;
  if(view)feedOutput(view,event.detail?.data);
});

window.addEventListener('taskmenu:session',event=>{
  const view=event.detail?.view;if(!view)return;
  const run=runs.get(view.meta.id);
  if(run&&!run.finished&&view.meta.status!=='running'){
    rejectRun(run,new Error('Terminal session ended while preset "'+run.preset.name+'" was running.'));
  }
});

function waitForSocket(view){
  if(view?.closed)return Promise.reject(new Error('Terminal tab is closed.'));
  if(view?.ws?.readyState===WebSocket.OPEN)return Promise.resolve();
  return new Promise((resolve,reject)=>{
    const started=Date.now();
    const timer=setInterval(()=>{
      if(view.closed){clearInterval(timer);reject(new Error('Terminal tab is closed.'));return;}
      if(view.ws?.readyState===WebSocket.OPEN){clearInterval(timer);resolve();return;}
      if(Date.now()-started>5000){clearInterval(timer);reject(new Error('Terminal connection is not ready.'));}
    },50);
  });
}

function waitForExitCode(run,index){
  if(run.finished)return Promise.reject(new Error('Preset run is no longer active.'));
  return new Promise((resolve,reject)=>{
    run.buffer='';
    run.waiter={index,pattern:markerPattern(run.token,index),resolve,reject};
  });
}

async function executeCommand(view,run,command,index){
  await waitForSocket(view);
  const result=waitForExitCode(run,index);
  view.ws.send(commandPayload(view,command,run.token,index));
  return result;
}

async function runPreset(view,preset){
  if(!view||view.closed||view.meta.task_id!==0)throw new Error('Preset commands are available only for terminal tabs.');
  if(view.meta.status!=='running')throw new Error('Terminal must be running before starting a preset.');
  if(runs.has(view.meta.id))throw new Error('A preset is already running in this terminal.');
  if(!preset?.commands?.length)throw new Error('Preset has no commands.');

  const run={view,preset,token:randomToken(),buffer:'',waiter:null,finished:false,index:-1};
  runs.set(view.meta.id,run);
  view.tab.classList.add('preset-command-running');
  try{
    for(let index=0;index<preset.commands.length;index++){
      run.index=index;
      const rc=await executeCommand(view,run,preset.commands[index],index);
      if(rc!==0){
        throw new Error('Preset "'+preset.name+'" stopped at command '+(index+1)+' with exit code '+rc+'.');
      }
    }
    return {ok:true,commands:preset.commands.length};
  }finally{
    run.finished=true;
    if(run.waiter){
      const waiter=run.waiter;run.waiter=null;
      waiter.reject(new Error('Preset run ended before command completion.'));
    }
    runs.delete(view.meta.id);
    if(!view.closed)view.tab.classList.remove('preset-command-running');
  }
}

function presetByID(id){return state.presets.find(preset=>preset.id===id)||null;}
function isRunning(view){return Boolean(view&&runs.has(view.meta.id));}

await refreshState().catch(error=>console.warn('Cannot load command presets',error));
setInterval(()=>refreshState().catch(()=>{}),5000);

globalThis.TaskMenuCommandPresets={
  get state(){return state;},
  refresh:refreshState,
  mutate,
  runPreset,
  presetByID,
  isRunning,
  commandPayload,
  markerPrefix
};
