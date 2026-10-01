const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for self-update settings');

const control=document.createElement('label');
control.id='self-update-validation-settings';
control.className='self-update-validation-settings';
control.title='Run the complete developer test suite before activating updates. Build and runtime validation always run.';
const checkbox=document.createElement('input');
checkbox.type='checkbox';
checkbox.id='self-update-full-validation';
const text=document.createElement('span');
text.textContent='Run full validation tests before self-update';
const hint=document.createElement('small');
hint.textContent='Developer mode · slower updates';
hint.style.opacity='.6';
hint.style.marginLeft='6px';
control.append(checkbox,text,hint);
control.hidden=true;
document.querySelector('header')?.append(control);

let loaded=false;
let value=false;

function apply(settings){
  value=Boolean(settings?.run_full_validation_tests);
  checkbox.checked=value;
  loaded=true;
}

async function load(){
  if(!app.taskData?.workspace)return;
  const settings=await app.jsonFetch('/api/config/self-update');
  apply(settings);
}

checkbox.onchange=async()=>{
  const previous=value;
  const next=checkbox.checked;
  checkbox.disabled=true;
  try{
    const saved=await app.jsonFetch('/api/config/self-update',{
      method:'PUT',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({run_full_validation_tests:next})
    });
    apply(saved);
  }catch(error){
    checkbox.checked=previous;
    app.showError(error);
  }finally{
    checkbox.disabled=false;
  }
};

async function bootstrap(){
  try{await load();}
  catch(error){console.warn('Cannot load self-update validation settings',error);}
}
if(app.taskData?.workspace)bootstrap();
else window.addEventListener('taskmenu:tasks',bootstrap,{once:true});

globalThis.TaskMenuSelfUpdateSettings={
  get loaded(){return loaded;},
  get runFullValidationTests(){return value;},
  reload:load
};
