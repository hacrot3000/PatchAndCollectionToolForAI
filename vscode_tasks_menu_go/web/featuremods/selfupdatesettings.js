const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for self-update settings');

const control=document.createElement('div');
control.id='self-update-branch-settings';
control.className='self-update-branch-settings';
control.hidden=true;

const label=document.createElement('span');
label.textContent='Self-update branch';

const select=document.createElement('select');
select.id='self-update-branch';
select.title='Branch used by Check update and self-update';
select.setAttribute('aria-label','Self-update branch');

const save=document.createElement('button');
save.type='button';
save.textContent='Save';
save.title='Save self-update branch for this workspace';

const status=document.createElement('small');
status.className='self-update-branch-status';
status.style.opacity='.6';

control.append(label,select,save,status);
document.querySelector('header')?.append(control);

let currentBranch='main';
let branchesLoaded=false;
let branchesLoading=null;

function normalizeBranch(value){
  return String(value||'').trim()||'main';
}

function renderBranches(branches=[]){
  const names=[...new Set([currentBranch,...branches.map(normalizeBranch).filter(Boolean)])];
  names.sort((a,b)=>{
    if(a==='main')return -1;
    if(b==='main')return 1;
    return a.localeCompare(b);
  });
  select.replaceChildren();
  for(const name of names){
    const option=document.createElement('option');
    option.value=name;
    option.textContent=name;
    select.append(option);
  }
  select.value=currentBranch;
}

function apply(settings){
  currentBranch=normalizeBranch(settings?.branch);
  renderBranches([...select.options].map(option=>option.value));
  status.textContent='Current: '+currentBranch;
  const check=document.querySelector('#self-update-check');
  if(check)check.title='Check GitHub branch '+currentBranch+' for a newer TaskDeck revision';
}

async function loadSettings(){
  if(!app.taskData?.workspace)return;
  const settings=await app.jsonFetch('/api/config/self-update');
  apply(settings);
}

async function loadBranches(){
  if(branchesLoaded)return;
  if(branchesLoading)return branchesLoading;
  status.textContent='Loading branches…';
  branchesLoading=(async()=>{
    try{
      const data=await app.jsonFetch('/api/config/self-update?action=branches');
      const branches=Array.isArray(data?.branches)?data.branches:[];
      renderBranches(branches);
      branchesLoaded=true;
      status.textContent='Current: '+currentBranch;
    }catch(error){
      console.warn('Cannot load self-update branches',error);
      status.textContent='Branch list unavailable; current selection is still usable.';
    }finally{
      branchesLoading=null;
    }
  })();
  return branchesLoading;
}

async function saveBranch(){
  const branch=normalizeBranch(select.value);
  save.disabled=true;
  select.disabled=true;
  status.textContent='Saving…';
  try{
    const settings=await app.jsonFetch('/api/config/self-update',{
      method:'PUT',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({branch})
    });
    apply(settings);
    status.textContent='Saved: '+currentBranch;
  }catch(error){
    select.value=currentBranch;
    status.textContent='Save failed';
    app.showError(error);
  }finally{
    save.disabled=false;
    select.disabled=false;
  }
}

select.addEventListener('focus',()=>{loadBranches();});
select.addEventListener('pointerdown',()=>{loadBranches();},{once:true});
save.onclick=()=>{saveBranch();};

async function bootstrap(){
  try{await loadSettings();}
  catch(error){console.warn('Cannot load self-update branch setting',error);}
}
if(app.taskData?.workspace)bootstrap();
else window.addEventListener('taskmenu:tasks',bootstrap,{once:true});

globalThis.TaskMenuSelfUpdateSettings={
  get branch(){return currentBranch;},
  reload:loadSettings,
  loadBranches
};
