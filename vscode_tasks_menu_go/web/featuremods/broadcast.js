const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for broadcast groups');

const PRESETS={
  slate:{label:'Slate',bg:'#374151',fg:'#f9fafb'},
  ocean:{label:'Ocean',bg:'#0c4a6e',fg:'#e0f2fe'},
  forest:{label:'Forest',bg:'#14532d',fg:'#dcfce7'},
  amber:{label:'Amber',bg:'#78350f',fg:'#fef3c7'},
  violet:{label:'Violet',bg:'#4c1d95',fg:'#ede9fe'},
  rose:{label:'Rose',bg:'#881337',fg:'#ffe4e6'},
  cyan:{label:'Cyan',bg:'#164e63',fg:'#cffafe'},
  lime:{label:'Lime',bg:'#365314',fg:'#ecfccb'}
};
const MODE_LABELS={none:'None',all:'All',group:'Group'};
let state={version:1,mode:'none',groups:[],assignments:{}};
let stateFingerprint='';
let headerMenu=null;
let headerTrigger=null;
let headerPop=null;
let dialogHost=null;
let lastInputWarningAt=0;
const inputQueues=new Map();
const inputHooks=new Map();

const style=document.createElement('style');
style.textContent=`
.broadcast-menu>.taskmenu-menu-trigger.broadcast-enabled{border-color:#8a6d28;background:#4a3a19;color:#fff1b5}
.broadcast-menu>.taskmenu-menu-trigger.broadcast-all{border-color:#a34a4a;background:#55272a;color:#ffe2e4}
.broadcast-mode-button.active{font-weight:800;border-color:#77a7d2;background:#213d58}
.broadcast-group-row{display:flex;align-items:center;gap:7px;width:100%;text-align:left}
.broadcast-swatch{display:inline-block;width:14px;height:14px;border-radius:4px;border:1px solid rgba(255,255,255,.35);flex:0 0 auto}
.broadcast-group-count{margin-left:auto;opacity:.65;font-size:10px}
.tab.broadcast-grouped{border-color:var(--broadcast-tab-fg)!important;background:var(--broadcast-tab-bg)!important;color:var(--broadcast-tab-fg)!important}
.tab.broadcast-grouped.active{box-shadow:inset 0 -3px 0 var(--broadcast-tab-fg),0 0 0 1px color-mix(in srgb,var(--broadcast-tab-fg) 35%,transparent)}
.tab.broadcast-grouped .status,.tab.broadcast-grouped .close{color:inherit}
.broadcast-dialog-backdrop{position:fixed;inset:0;z-index:6000;background:rgba(0,0,0,.55);display:flex;align-items:center;justify-content:center;padding:18px}
.broadcast-dialog{width:min(430px,94vw);background:#171a20;border:1px solid #3b414d;border-radius:10px;box-shadow:0 18px 48px rgba(0,0,0,.5);padding:16px}
.broadcast-dialog h3{margin:0 0 12px;font-size:16px}
.broadcast-dialog label{display:block;font-size:11px;font-weight:700;opacity:.7;margin:10px 0 5px}
.broadcast-dialog input{width:100%;background:#0d1117;color:inherit;border:1px solid #3b414d;border-radius:6px;padding:8px 10px}
.broadcast-palette{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:7px}
.broadcast-preset{display:flex;align-items:center;gap:8px;text-align:left}
.broadcast-preset.selected{outline:2px solid #8cc8ff;outline-offset:1px}
.broadcast-dialog-actions{display:flex;gap:8px;justify-content:flex-end;margin-top:15px}
.broadcast-dialog-actions .danger{margin-right:auto;background:#4a252a;border-color:#7a4048;color:#ffe2e4}
html[data-taskmenu-theme="light"] .broadcast-dialog{background:#fff;border-color:#b9c0c8}
html[data-taskmenu-theme="light"] .broadcast-dialog input{background:#fff;color:#20242b;border-color:#b9c0c8}
`;
document.head.append(style);

function groupByID(id){return (state.groups||[]).find(group=>group.id===id)||null;}
function groupForSession(id){return groupByID(state.assignments?.[id]||'');}
function presetFor(group){return PRESETS[group?.preset]||PRESETS.slate;}
function assignedCount(groupID){return Object.values(state.assignments||{}).filter(id=>id===groupID).length;}

function applyTabStyle(view){
  const group=groupForSession(view.meta.id);
  if(!group){
    view.tab.classList.remove('broadcast-grouped');
    view.tab.style.removeProperty('--broadcast-tab-bg');
    view.tab.style.removeProperty('--broadcast-tab-fg');
    delete view.tab.dataset.broadcastGroup;
    if(view.tab.dataset.broadcastTitle){
      view.tab.removeAttribute('title');
      delete view.tab.dataset.broadcastTitle;
    }
    return;
  }
  const preset=presetFor(group);
  view.tab.classList.add('broadcast-grouped');
  view.tab.style.setProperty('--broadcast-tab-bg',preset.bg);
  view.tab.style.setProperty('--broadcast-tab-fg',preset.fg);
  view.tab.dataset.broadcastGroup=group.id;
  view.tab.dataset.broadcastTitle='1';
  view.tab.title='Broadcast group: '+group.name;
}

function applyAllTabStyles(){for(const view of app.views.values())applyTabStyle(view);}

function addHeaderLabel(text){
  const el=document.createElement('div');el.className='taskmenu-menu-label';el.textContent=text;headerPop.append(el);return el;
}
function makeHeaderButton(text,onClick,className=''){
  const button=document.createElement('button');button.type='button';button.textContent=text;if(className)button.className=className;
  button.onclick=event=>{event.stopPropagation();Promise.resolve(onClick()).catch(app.showError);};
  headerPop.append(button);return button;
}

function renderHeaderMenu(){
  if(!headerMenu)return;
  const label=MODE_LABELS[state.mode]||'None';
  headerTrigger.textContent='Broadcast: '+label+' ▾';
  headerTrigger.title='Keyboard broadcast mode: '+label;
  headerTrigger.classList.toggle('broadcast-enabled',state.mode==='group');
  headerTrigger.classList.toggle('broadcast-all',state.mode==='all');
  headerPop.replaceChildren();

  addHeaderLabel('MODE');
  for(const mode of ['none','all','group']){
    const button=makeHeaderButton((state.mode===mode?'✓ ':'○ ')+(MODE_LABELS[mode]||mode),async()=>{
      await mutate({action:'set_mode',mode});
      closeHeaderMenu();
    },'broadcast-mode-button'+(state.mode===mode?' active':''));
    button.title=mode==='all'
      ?'Send each typed key to every running session'
      :mode==='group'
        ?'Send each typed key to running sessions in the source tab group'
        :'Only the active source session receives typed input';
  }

  addHeaderLabel('GROUPS');
  if(!(state.groups||[]).length){
    const empty=document.createElement('div');empty.className='taskmenu-menu-label';empty.textContent='No groups yet';headerPop.append(empty);
  }
  for(const group of state.groups||[]){
    const preset=presetFor(group);
    const button=makeHeaderButton('',()=>editGroup(group));
    button.classList.add('broadcast-group-row');
    const swatch=document.createElement('span');swatch.className='broadcast-swatch';swatch.style.background=preset.bg;swatch.style.borderColor=preset.fg;
    const name=document.createElement('span');name.textContent=group.name+' · '+preset.label;
    const count=document.createElement('span');count.className='broadcast-group-count';count.textContent=String(assignedCount(group.id));
    button.append(swatch,name,count);
    button.title='Edit broadcast group '+group.name;
  }
  makeHeaderButton('＋ New group…',()=>createGroup());
}

function closeHeaderMenu(){if(headerMenu)headerMenu.classList.remove('open');}
function installHeaderMenu(){
  const host=document.querySelector('.header-action-menus');if(!host||host.querySelector('.broadcast-menu'))return;
  headerMenu=document.createElement('div');headerMenu.className='taskmenu-menu broadcast-menu';
  headerTrigger=document.createElement('button');headerTrigger.className='taskmenu-menu-trigger';
  headerPop=document.createElement('div');headerPop.className='taskmenu-menu-popover';
  headerTrigger.onclick=event=>{
    event.stopPropagation();
    const open=!headerMenu.classList.contains('open');
    for(const other of document.querySelectorAll('.taskmenu-menu.open'))if(other!==headerMenu)other.classList.remove('open');
    headerMenu.classList.toggle('open',open);
  };
  headerPop.onclick=event=>event.stopPropagation();
  headerMenu.append(headerTrigger,headerPop);
  host.prepend(headerMenu);
  renderHeaderMenu();
}

document.addEventListener('pointerdown',event=>{
  if(!headerMenu?.classList.contains('open'))return;
  const target=event.target instanceof Node?event.target:null;
  if(target&&headerMenu.contains(target))return;
  closeHeaderMenu();
},true);
document.addEventListener('keydown',event=>{if(event.key==='Escape')closeHeaderMenu();});

function applyState(next){
  const normalized={
    version:Number(next?.version||1),
    mode:['none','all','group'].includes(next?.mode)?next.mode:'none',
    groups:Array.isArray(next?.groups)?next.groups:[],
    assignments:next?.assignments&&typeof next.assignments==='object'?next.assignments:{}
  };
  const fingerprint=JSON.stringify(normalized);
  if(fingerprint===stateFingerprint)return;
  state=normalized;stateFingerprint=fingerprint;
  renderHeaderMenu();applyAllTabStyles();
  window.dispatchEvent(new CustomEvent('taskmenu:broadcast-state',{detail:{state}}));
}

async function refreshState(){applyState(await app.jsonFetch('/api/broadcast'));return state;}
async function mutate(payload){
  const next=await app.jsonFetch('/api/broadcast',{
    method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)
  });
  applyState(next);return state;
}

function closeDialog(result,resolve){
  const host=dialogHost;dialogHost=null;
  if(host)host.remove();
  resolve(result);
}

function groupDialog(existing=null){
  return new Promise(resolve=>{
    if(dialogHost)dialogHost.remove();
    const backdrop=document.createElement('div');backdrop.className='broadcast-dialog-backdrop';dialogHost=backdrop;
    const card=document.createElement('div');card.className='broadcast-dialog';
    const title=document.createElement('h3');title.textContent=existing?'Edit broadcast group':'Create broadcast group';
    const nameLabel=document.createElement('label');nameLabel.textContent='Group name';
    const name=document.createElement('input');name.type='text';name.maxLength=80;name.value=existing?.name||'';name.placeholder='e.g. Group A';
    const colorLabel=document.createElement('label');colorLabel.textContent='Tab color preset';
    const palette=document.createElement('div');palette.className='broadcast-palette';
    let selected=existing?.preset&&PRESETS[existing.preset]?existing.preset:'ocean';
    const buttons=new Map();
    const selectPreset=id=>{
      selected=id;
      for(const [key,button] of buttons)button.classList.toggle('selected',key===id);
    };
    for(const [id,preset] of Object.entries(PRESETS)){
      const button=document.createElement('button');button.type='button';button.className='broadcast-preset';
      button.style.background=preset.bg;button.style.color=preset.fg;
      const swatch=document.createElement('span');swatch.textContent='■';
      const text=document.createElement('span');text.textContent=preset.label;
      button.append(swatch,text);button.onclick=()=>selectPreset(id);buttons.set(id,button);palette.append(button);
    }
    selectPreset(selected);
    const actions=document.createElement('div');actions.className='broadcast-dialog-actions';
    if(existing){
      const del=document.createElement('button');del.type='button';del.className='danger';del.textContent='Delete group';
      del.onclick=()=>{
        if(window.confirm('Delete group "'+existing.name+'"? Tabs will be removed from this group.'))closeDialog({action:'delete'},resolve);
      };
      actions.append(del);
    }
    const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';cancel.onclick=()=>closeDialog(null,resolve);
    const save=document.createElement('button');save.type='button';save.textContent=existing?'Save':'Create';
    const submit=()=>{
      const value=name.value.replace(/[\r\n]+/g,' ').trim();
      if(!value){name.focus();return;}
      closeDialog({action:'save',name:value,preset:selected},resolve);
    };
    save.onclick=submit;name.onkeydown=event=>{if(event.key==='Enter'){event.preventDefault();submit();}};
    actions.append(cancel,save);
    card.append(title,nameLabel,name,colorLabel,palette,actions);backdrop.append(card);document.body.append(backdrop);
    backdrop.addEventListener('pointerdown',event=>{if(event.target===backdrop)closeDialog(null,resolve);});
    const keydown=event=>{if(event.key==='Escape'){document.removeEventListener('keydown',keydown,true);closeDialog(null,resolve);}};
    document.addEventListener('keydown',keydown,true);
    setTimeout(()=>{name.focus();name.select();},0);
  });
}

async function createGroup(assignView=null){
  closeHeaderMenu();
  const result=await groupDialog();
  if(!result)return null;
  const before=new Set((state.groups||[]).map(group=>group.id));
  await mutate({action:'create_group',name:result.name,preset:result.preset});
  const created=(state.groups||[]).find(group=>!before.has(group.id))||null;
  if(created&&assignView)await assign(assignView,created.id);
  return created;
}
async function editGroup(group){
  closeHeaderMenu();
  const result=await groupDialog(group);if(!result)return;
  if(result.action==='delete')await mutate({action:'delete_group',group_id:group.id});
  else await mutate({action:'update_group',group_id:group.id,name:result.name,preset:result.preset});
}
async function assign(view,groupID){
  if(!view||view.closed)return;
  await mutate({action:'assign',session_id:view.meta.id,group_id:groupID||''});
}
async function removeFromGroup(view){return assign(view,'');}

function contextActions(view){
  const current=state.assignments?.[view.meta.id]||'';
  const actions=[];
  for(const group of state.groups||[]){
    const preset=presetFor(group);
    actions.push({
      label:(current===group.id?'✓ ':'')+group.name,
      title:'Assign this tab to '+group.name,
      preset:{bg:preset.bg,fg:preset.fg},
      run:()=>assign(view,group.id)
    });
  }
  actions.push({label:'＋ Create new group…',title:'Create a group and assign this tab',run:()=>createGroup(view)});
  if(current)actions.push({label:'✕ Remove from group',title:'Remove this tab from its broadcast group',danger:true,run:()=>removeFromGroup(view)});
  return actions;
}

function warnInput(error){
  const now=Date.now();if(now-lastInputWarningAt<3000)return;lastInputWarningAt=now;
  console.warn('Broadcast input failed',error);
}
function queueBroadcast(view,data){
  if(!view||view.closed||view.meta.status!=='running'||!data||state.mode==='none')return;
  if(state.mode==='group'&&!state.assignments?.[view.meta.id])return;
  const previous=inputQueues.get(view.meta.id)||Promise.resolve();
  const next=previous.catch(()=>{}).then(()=>app.jsonFetch('/api/broadcast',{
    method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({action:'input',source_id:view.meta.id,data})
  })).catch(warnInput);
  inputQueues.set(view.meta.id,next);
}
function installInputHook(view){
  const old=inputHooks.get(view.meta.id);
  if(old?.view===view)return;
  try{old?.disposable?.dispose();}catch{}
  const disposable=view.term.onData(data=>queueBroadcast(view,data));
  inputHooks.set(view.meta.id,{view,disposable});
  applyTabStyle(view);
}

window.addEventListener('taskmenu:session',event=>{const view=event.detail?.view;if(view)installInputHook(view);});
for(const view of app.views.values())installInputHook(view);
installHeaderMenu();
await refreshState().catch(error=>console.warn('Cannot load broadcast state',error));
setInterval(()=>refreshState().catch(()=>{}),2000);

globalThis.TaskMenuBroadcast={
  get state(){return state;},
  presets:PRESETS,
  refresh:refreshState,
  setMode:mode=>mutate({action:'set_mode',mode}),
  assign,
  removeFromGroup,
  createGroup,
  editGroup,
  contextActions,
  groupForSession
};
