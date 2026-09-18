const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for command presets');

let state={version:1,presets:[]};
let fingerprint='';
const runs=new Map();
const decoder=new TextDecoder();
let managerHost=null;
let managerDraft=null;
let managerSelectedID='';
let managerDirty=false;
let managerKeydown=null;

const style=document.createElement('style');
style.textContent=`
.tab.preset-command-running{box-shadow:inset 0 -3px 0 #f59e0b!important}
.preset-manager-backdrop{position:fixed;inset:0;z-index:6200;background:rgba(0,0,0,.58);display:flex;align-items:center;justify-content:center;padding:18px}
.preset-manager{width:min(900px,96vw);height:min(680px,92vh);display:grid;grid-template-columns:260px 1fr;overflow:hidden;background:#171a20;border:1px solid #3b414d;border-radius:10px;box-shadow:0 20px 54px rgba(0,0,0,.55)}
.preset-manager-list{display:flex;flex-direction:column;min-width:0;border-right:1px solid #30343b;padding:12px;gap:7px;overflow:auto}
.preset-manager-list h3{margin:0 0 4px;font-size:15px}.preset-manager-list button{text-align:left}
.preset-manager-list .selected{border-color:#75a9d6;background:#203b58}
.preset-manager-snippet{display:block;opacity:.62;font-size:10px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;margin-top:2px}
.preset-manager-editor{display:flex;flex-direction:column;min-width:0;min-height:0;padding:14px;gap:9px}
.preset-manager-editor label{font-size:11px;font-weight:700;opacity:.68}
.preset-manager-editor input,.preset-command-row textarea{width:100%;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:8px 10px;font:12px ui-monospace,monospace}
.preset-command-list{display:flex;flex-direction:column;gap:7px;overflow:auto;min-height:0;flex:1}
.preset-command-row{display:grid;grid-template-columns:1fr auto;gap:7px;align-items:start}
.preset-command-row textarea{resize:vertical;min-height:58px}
.preset-manager-actions{display:flex;gap:8px;align-items:center}.preset-manager-actions .danger{background:#4a252a;border-color:#7a4048;color:#ffe2e4}.preset-manager-actions .spacer{flex:1}
html[data-taskmenu-theme="light"] .preset-manager{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .preset-manager-list{border-color:#d7dce1}
html[data-taskmenu-theme="light"] .preset-manager-editor input,html[data-taskmenu-theme="light"] .preset-command-row textarea{background:#fff;color:#20242b;border-color:#b9c0c8}
@media(max-width:720px){.preset-manager{grid-template-columns:1fr;height:min(760px,95vh)}.preset-manager-list{max-height:210px;border-right:0;border-bottom:1px solid #30343b}}
`;
document.head.append(style);

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
  if(managerHost&&!managerDirty)syncManagerDraftFromState();
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

function displayCommandText(command){
  return String(command).replace(/\r\n/g,'\n').replace(/\r/g,'\n').replace(/\n/g,'\r\n')+'\r\n';
}

function payloadEchoLineCount(payload){
  return (String(payload).match(/\n/g)||[]).length+(String(payload).endsWith('\r')?1:0);
}

function decodeDisplayData(run,data){
  if(typeof data==='string')return data;
  let bytes=null;
  if(data instanceof ArrayBuffer)bytes=new Uint8Array(data);
  else if(ArrayBuffer.isView(data))bytes=new Uint8Array(data.buffer,data.byteOffset,data.byteLength);
  if(!bytes)return String(data??'');
  if(!run.displayDecoder)run.displayDecoder=new TextDecoder();
  return run.displayDecoder.decode(bytes,{stream:true});
}

function stripPresetSentinel(run,text){
  const prefix='\x1eVTM_PRESET:'+run.token+':'+run.index+':';
  let buffer=(run.displayCarry||'')+text;
  let out='';
  run.displayCarry='';
  while(buffer){
    const start=buffer.indexOf(prefix);
    if(start>=0){
      out+=buffer.slice(0,start);
      const end=buffer.indexOf('\x1f',start+prefix.length);
      if(end<0){
        run.displayCarry=buffer.slice(start);
        return out||null;
      }
      buffer=buffer.slice(end+1);
      continue;
    }
    let keep=0;
    for(let length=Math.min(prefix.length-1,buffer.length);length>0;length--){
      if(buffer.endsWith(prefix.slice(0,length))){keep=length;break;}
    }
    if(keep){
      out+=buffer.slice(0,-keep);
      run.displayCarry=buffer.slice(-keep);
      return out||null;
    }
    out+=buffer;
    buffer='';
  }
  return out||null;
}

function filterPresetDisplay(view,data){
  const run=runs.get(view?.meta?.id);
  if(!run||run.finished||!run.displayActive)return data;
  let text=decodeDisplayData(run,data);

  while(run.echoLinesRemaining>0){
    const newline=text.indexOf('\n');
    if(newline<0)return null;
    run.echoLinesRemaining--;
    text=text.slice(newline+1);
  }
  if(!text)return null;
  return stripPresetSentinel(run,text);
}

if(typeof app.addOutputFilter==='function')app.addOutputFilter(filterPresetDisplay);

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
  const payload=commandPayload(view,command,run.token,index);
  run.displayActive=typeof app.addOutputFilter==='function';
  run.echoLinesRemaining=run.displayActive?payloadEchoLineCount(payload):0;
  run.displayCarry='';
  run.displayDecoder=null;
  const result=waitForExitCode(run,index);
  if(run.displayActive)view.term.write(displayCommandText(command));
  view.ws.send(payload);
  return result;
}

async function runPreset(view,preset){
  if(!view||view.closed||view.meta.task_id!==0)throw new Error('Preset commands are available only for terminal tabs.');
  if(view.meta.status!=='running')throw new Error('Terminal must be running before starting a preset.');
  if(runs.has(view.meta.id))throw new Error('A preset is already running in this terminal.');
  if(!preset?.commands?.length)throw new Error('Preset has no commands.');

  const run={view,preset,token:randomToken(),buffer:'',waiter:null,finished:false,index:-1,displayActive:false,echoLinesRemaining:0,displayCarry:'',displayDecoder:null};
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

function firstCommandSnippet(preset){
  const first=String(preset?.commands?.[0]||'').split(/\r?\n/).find(line=>line.trim())||'';
  const compact=first.replace(/\s+/g,' ').trim();
  return compact.length>54?compact.slice(0,51)+'…':compact;
}

function clonePreset(preset){
  return preset?{id:preset.id,name:preset.name,commands:[...preset.commands]}:{id:'',name:'',commands:['']};
}

function managerCanDiscard(){
  return !managerDirty||window.confirm('Discard unsaved preset changes?');
}

function cleanupManager(){
  if(managerKeydown){document.removeEventListener('keydown',managerKeydown,true);managerKeydown=null;}
  if(managerHost)managerHost.remove();
  managerHost=null;managerDraft=null;managerSelectedID='';managerDirty=false;
}

function closeManager(){
  if(!managerCanDiscard())return;
  cleanupManager();
}

function syncManagerDraftFromState(){
  if(!managerHost)return;
  let preset=managerSelectedID?presetByID(managerSelectedID):null;
  if(!preset&&state.presets.length){preset=state.presets[0];managerSelectedID=preset.id;}
  if(preset)managerDraft=clonePreset(preset);
  else{managerSelectedID='';managerDraft=clonePreset(null);}
  managerDirty=false;
  renderManager();
}

function selectManagerPreset(id){
  if(!managerCanDiscard())return;
  const preset=presetByID(id);
  if(!preset)return;
  managerSelectedID=id;managerDraft=clonePreset(preset);managerDirty=false;renderManager();
}

function newManagerPreset(){
  if(!managerCanDiscard())return;
  managerSelectedID='';managerDraft=clonePreset(null);managerDirty=false;renderManager();
  setTimeout(()=>managerHost?.querySelector('.preset-name-input')?.focus(),0);
}

function markManagerDirty(){managerDirty=true;}

function addManagerCommand(){
  if(!managerDraft)return;
  managerDraft.commands.push('');managerDirty=true;renderManager();
  setTimeout(()=>{
    const rows=managerHost?.querySelectorAll('.preset-command-row textarea');
    rows?.[rows.length-1]?.focus();
  },0);
}

function removeManagerCommand(index){
  if(!managerDraft)return;
  managerDraft.commands.splice(index,1);
  if(!managerDraft.commands.length)managerDraft.commands.push('');
  managerDirty=true;renderManager();
}

async function saveManagerPreset(){
  if(!managerDraft)return;
  const name=managerDraft.name.trim();
  const commands=managerDraft.commands.map(String);
  if(!name)throw new Error('Preset name is required.');
  if(!commands.some(command=>command.trim()))throw new Error('Preset requires at least one command.');
  const before=new Set(state.presets.map(preset=>preset.id));
  if(managerDraft.id){
    await mutate({action:'update',id:managerDraft.id,name,commands});
    managerSelectedID=managerDraft.id;
  }else{
    await mutate({action:'create',name,commands});
    const created=state.presets.find(preset=>!before.has(preset.id));
    managerSelectedID=created?.id||state.presets[state.presets.length-1]?.id||'';
  }
  const saved=presetByID(managerSelectedID);
  managerDraft=clonePreset(saved);managerDirty=false;renderManager();
}

async function deleteManagerPreset(){
  if(!managerDraft?.id)return;
  if(!window.confirm('Delete preset "'+managerDraft.name+'"?'))return;
  const id=managerDraft.id;
  await mutate({action:'delete',id});
  managerSelectedID=state.presets[0]?.id||'';
  managerDraft=clonePreset(managerSelectedID?presetByID(managerSelectedID):null);
  managerDirty=false;renderManager();
}

function renderManager(){
  if(!managerHost||!managerDraft)return;
  const card=managerHost.querySelector('.preset-manager');if(!card)return;
  card.replaceChildren();

  const list=document.createElement('div');list.className='preset-manager-list';
  const heading=document.createElement('h3');heading.textContent='Command presets';
  const add=document.createElement('button');add.type='button';add.textContent='＋ Add preset';add.onclick=newManagerPreset;
  list.append(heading,add);
  for(const preset of state.presets){
    const button=document.createElement('button');button.type='button';button.classList.toggle('selected',preset.id===managerSelectedID);
    const name=document.createElement('span');name.textContent=preset.name;
    const snippet=document.createElement('span');snippet.className='preset-manager-snippet';snippet.textContent=firstCommandSnippet(preset)||'(empty)';
    button.append(name,snippet);button.onclick=()=>selectManagerPreset(preset.id);list.append(button);
  }
  const listSpacer=document.createElement('div');listSpacer.style.flex='1';list.append(listSpacer);
  const closeList=document.createElement('button');closeList.type='button';closeList.textContent='Close';closeList.onclick=closeManager;list.append(closeList);

  const editor=document.createElement('div');editor.className='preset-manager-editor';
  const nameLabel=document.createElement('label');nameLabel.textContent='Preset name';
  const name=document.createElement('input');name.className='preset-name-input';name.type='text';name.maxLength=100;name.value=managerDraft.name;name.placeholder='e.g. Auto commit all';
  name.oninput=()=>{managerDraft.name=name.value;markManagerDirty();};

  const commandsLabel=document.createElement('label');commandsLabel.textContent='Commands — executed sequentially; stop on non-zero exit code';
  const commandList=document.createElement('div');commandList.className='preset-command-list';
  managerDraft.commands.forEach((command,index)=>{
    const row=document.createElement('div');row.className='preset-command-row';
    const textarea=document.createElement('textarea');textarea.value=command;textarea.spellcheck=false;textarea.placeholder='Command '+(index+1);
    textarea.oninput=()=>{managerDraft.commands[index]=textarea.value;markManagerDirty();};
    const remove=document.createElement('button');remove.type='button';remove.textContent='Remove';remove.onclick=()=>removeManagerCommand(index);
    row.append(textarea,remove);commandList.append(row);
  });
  const addCommand=document.createElement('button');addCommand.type='button';addCommand.textContent='＋ Add command';addCommand.onclick=addManagerCommand;

  const actions=document.createElement('div');actions.className='preset-manager-actions';
  if(managerDraft.id){
    const del=document.createElement('button');del.type='button';del.className='danger';del.textContent='Delete preset';del.onclick=()=>deleteManagerPreset().catch(app.showError);actions.append(del);
  }
  const spacer=document.createElement('span');spacer.className='spacer';
  const close=document.createElement('button');close.type='button';close.textContent='Close';close.onclick=closeManager;
  const save=document.createElement('button');save.type='button';save.textContent='Save';save.onclick=()=>saveManagerPreset().catch(app.showError);
  actions.append(spacer,close,save);

  editor.append(nameLabel,name,commandsLabel,commandList,addCommand,actions);
  card.append(list,editor);
}

async function openManager(selectedID=''){
  await refreshState();
  if(managerHost){
    if(selectedID&&selectedID!==managerSelectedID&&managerCanDiscard()){
      managerSelectedID=selectedID;managerDraft=clonePreset(presetByID(selectedID));managerDirty=false;renderManager();
    }
    return;
  }
  managerHost=document.createElement('div');managerHost.className='preset-manager-backdrop';
  const card=document.createElement('div');card.className='preset-manager';managerHost.append(card);document.body.append(managerHost);
  managerHost.addEventListener('pointerdown',event=>{if(event.target===managerHost)closeManager();});
  managerKeydown=event=>{if(event.key==='Escape'){event.preventDefault();closeManager();}};
  document.addEventListener('keydown',managerKeydown,true);

  const selected=selectedID?presetByID(selectedID):state.presets[0]||null;
  managerSelectedID=selected?.id||'';
  managerDraft=clonePreset(selected);
  managerDirty=false;renderManager();
}

function presetContextActions(view){
  if(!view||view.meta.task_id!==0)return [];
  const running=isRunning(view);
  const children=state.presets.map(preset=>({
    label:preset.name+' — '+(firstCommandSnippet(preset)||'(no command)'),
    title:'Run preset "'+preset.name+'"',
    disabled:running||view.meta.status!=='running',
    run:target=>runPreset(target,preset)
  }));
  if(!children.length)children.push({label:'No presets defined',disabled:true});
  children.push({label:'Manage presets…',title:'Add, edit, remove or close command presets',run:()=>openManager()});
  return [{
    label:running?'Preset command (running)':'Preset command',
    title:'Run a project command preset in this terminal',
    children
  }];
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
  markerPrefix,
  openManager,
  contextActions:presetContextActions,
  firstCommandSnippet
};
