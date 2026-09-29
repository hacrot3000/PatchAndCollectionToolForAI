const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for running indicator settings');

const style=document.createElement('style');
style.textContent=`
.running-indicator-settings-dialog{position:fixed;inset:0;z-index:17000;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,.58);padding:18px}
.running-indicator-settings-card{width:min(520px,96vw);background:#171a20;border:1px solid #48515f;border-radius:10px;box-shadow:0 18px 55px rgba(0,0,0,.5);overflow:hidden}
.running-indicator-settings-head{display:flex;align-items:center;gap:8px;padding:11px 13px;border-bottom:1px solid #30343b}.running-indicator-settings-head strong{flex:1}
.running-indicator-settings-body{padding:14px;display:grid;grid-template-columns:150px 1fr;gap:10px 12px;align-items:center}
.running-indicator-settings-body label{font-size:11px;font-weight:700;opacity:.75}
.running-indicator-settings-body select,.running-indicator-settings-body input{width:100%;box-sizing:border-box}
.running-indicator-settings-hint{grid-column:1/3;font-size:11px;opacity:.65;line-height:1.45}
.running-indicator-settings-actions{display:flex;justify-content:flex-end;gap:8px;padding:10px 12px;border-top:1px solid #30343b}
html[data-taskmenu-theme="light"] .running-indicator-settings-card{background:#fff;border-color:#b9c0c8}
`;
document.head.append(style);

let settings={mode:'boxes',rpm:2};
let loaded=false;

function normalize(value){
  const mode=['boxes','spinner','time'].includes(value?.mode)?value.mode:'boxes';
  const rpm=Math.max(.1,Math.min(120,Number(value?.rpm)||2));
  return {mode,rpm};
}

function apply(value){
  settings=normalize(value);
  globalThis.TaskMenuRunningIndicator?.apply?.(settings);
  window.dispatchEvent(new CustomEvent('taskmenu:running-indicator-config',{detail:{...settings}}));
}

async function loadSettings(){
  if(!app.taskData?.workspace)return;
  const data=await app.jsonFetch('/api/config/running-indicator');
  apply(data);
  loaded=true;
}

async function saveSettings(next){
  const data=await app.jsonFetch('/api/config/running-indicator',{
    method:'PUT',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify(normalize(next))
  });
  apply(data);
  loaded=true;
  return settings;
}

function openSettings(){
  const dialog=document.createElement('div');dialog.className='running-indicator-settings-dialog';
  const card=document.createElement('div');card.className='running-indicator-settings-card';
  const head=document.createElement('div');head.className='running-indicator-settings-head';
  const title=document.createElement('strong');title.textContent='Running tab indicator';
  const close=document.createElement('button');close.type='button';close.textContent='×';
  head.append(title,close);

  const body=document.createElement('div');body.className='running-indicator-settings-body';
  const modeLabel=document.createElement('label');modeLabel.textContent='After 10 minutes';
  const mode=document.createElement('select');
  for(const [value,label] of [
    ['boxes','Step boxes · 1 → 2 → 3 → 2 → 1'],
    ['spinner','Circular spinner'],
    ['time','Always show elapsed time']
  ]){
    const option=document.createElement('option');option.value=value;option.textContent=label;mode.append(option);
  }
  mode.value=settings.mode;

  const rpmLabel=document.createElement('label');rpmLabel.textContent='Spinner speed';
  const rpmWrap=document.createElement('div');
  const rpm=document.createElement('input');rpm.type='number';rpm.min='0.1';rpm.max='120';rpm.step='0.1';rpm.value=String(settings.rpm);
  const rpmHint=document.createElement('div');rpmHint.style.fontSize='10px';rpmHint.style.opacity='.6';rpmHint.style.marginTop='4px';rpmHint.textContent='revolutions / minute';
  rpmWrap.append(rpm,rpmHint);

  const hint=document.createElement('div');hint.className='running-indicator-settings-hint';
  hint.textContent='During the first 10 minutes TaskDeck continues to show elapsed time. This setting controls the compact long-running status shown after that threshold.';

  const sync=()=>{
    const enabled=mode.value==='spinner';
    rpm.disabled=!enabled;
    rpmLabel.style.opacity=enabled?'1':'.45';
    rpmWrap.style.opacity=enabled?'1':'.45';
  };
  mode.onchange=sync;sync();
  body.append(modeLabel,mode,rpmLabel,rpmWrap,hint);

  const actions=document.createElement('div');actions.className='running-indicator-settings-actions';
  const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
  const save=document.createElement('button');save.type='button';save.className='task-connection-primary';save.textContent='Save';
  const remove=()=>dialog.remove();
  close.onclick=cancel.onclick=remove;
  save.onclick=async()=>{
    const next={mode:mode.value,rpm:Number(rpm.value)};
    if(next.mode==='spinner'&&(!Number.isFinite(next.rpm)||next.rpm<.1||next.rpm>120)){
      app.showError(new Error('Spinner speed must be between 0.1 and 120 RPM'));
      rpm.focus();return;
    }
    save.disabled=true;cancel.disabled=true;
    try{await saveSettings(next);remove();}
    catch(error){app.showError(error);save.disabled=false;cancel.disabled=false;}
  };
  actions.append(cancel,save);card.append(head,body,actions);dialog.append(card);document.body.append(dialog);
  dialog.addEventListener('pointerdown',event=>{if(event.target===dialog)remove();});
  dialog.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();remove();}});
}

const button=document.createElement('button');
button.id='running-indicator-settings';
button.type='button';
button.textContent='Running indicator…';
button.title='Configure long-running tab indicator for this project';
button.onclick=()=>openSettings();
document.querySelector('#reload')?.before(button);

async function bootstrap(){
  try{await loadSettings();}
  catch(error){console.warn('Cannot load running indicator project settings',error);}
}
if(app.taskData?.workspace)bootstrap();
else window.addEventListener('taskmenu:tasks',bootstrap,{once:true});

globalThis.TaskMenuRunningIndicatorSettings={
  get value(){return {...settings};},
  get loaded(){return loaded;},
  open:openSettings,
  reload:loadSettings
};
