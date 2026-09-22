const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal cwd selector');

const nativeFetch=window.fetch.bind(window);
let select=null;
let settings={selected_cwd:'.',custom_dirs:[]};

function normalizeCandidate(value){
  value=String(value||'').trim().replace(/\\/g,'/');
  if(!value||value==='.'||value==='${workspaceFolder}')return '.';
  const prefix='${workspaceFolder}/';
  if(value.startsWith(prefix))value=value.slice(prefix.length);
  value=value.replace(/^\.\//,'');
  if(!value||value==='.')return '.';
  if(value.includes('${')||value.startsWith('/')||value==='..'||value.startsWith('../')||value.includes('/../'))return '';
  return value;
}

function normalizeSettings(data){
  const dirs=[];const seen=new Set();
  for(const raw of Array.isArray(data?.custom_dirs)?data.custom_dirs:[]){
    const dir=normalizeCandidate(raw);if(!dir||dir==='.'||seen.has(dir))continue;
    seen.add(dir);dirs.push(dir);
  }
  let selected=normalizeCandidate(data?.selected_cwd)||'.';
  if(selected!=='.'&&!seen.has(selected)){seen.add(selected);dirs.push(selected);}
  return {selected_cwd:selected,custom_dirs:dirs};
}

function legacyKey(){
  const workspace=app.taskData?.workspace;
  return workspace?'vscode-tasks-menu:terminal-cwd:'+workspace:'';
}
function legacyValue(){
  const key=legacyKey();if(!key)return '';
  try{return localStorage.getItem(key)||'';}catch{return '';}
}
function clearLegacy(){
  const key=legacyKey();if(!key)return;
  try{localStorage.removeItem(key);}catch{}
}

function refresh(){
  if(!select)return;
  select.replaceChildren();
  const root=document.createElement('option');root.value='.';root.textContent='Terminal: project root';select.append(root);
  for(const dir of settings.custom_dirs){
    const option=document.createElement('option');option.value=dir;option.textContent='Terminal: '+dir;select.append(option);
  }
  const add=document.createElement('option');add.value='__custom__';add.textContent='Terminal: Add directory…';select.append(add);
  select.value=[...select.options].some(option=>option.value===settings.selected_cwd)?settings.selected_cwd:'.';
}

async function writeSettings(next){
  const response=await app.jsonFetch('/api/config/terminal-cwds',{
    method:'PUT',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify(next)
  });
  settings=normalizeSettings(response);
  clearLegacy();
  refresh();
  return settings;
}

async function loadSettings(){
  if(!app.taskData?.workspace)return;
  const response=await app.jsonFetch('/api/config/terminal-cwds');
  settings=normalizeSettings(response);
  const legacy=normalizeCandidate(legacyValue());
  if(settings.selected_cwd==='.'&&!settings.custom_dirs.length&&legacy&&legacy!=='.'){
    try{
      await writeSettings({selected_cwd:legacy,custom_dirs:[legacy]});
      return;
    }catch(error){
      console.warn('Legacy terminal directory migration skipped',error);
    }
  }
  if(settings.selected_cwd!=='.'||settings.custom_dirs.length)clearLegacy();
  refresh();
}

async function chooseCustomDirectory(){
  const chooser=globalThis.TaskMenuDirectoryBrowser?.choose;
  if(typeof chooser!=='function')throw new Error('Workspace directory browser is unavailable');
  const value=await chooser({
    title:'Choose terminal directory',
    label:'Terminal directory relative to the workspace:',
    confirm:'Use directory',
    initial:settings.selected_cwd||'.'
  });
  if(value===null){refresh();return;}
  const clean=normalizeCandidate(value);
  if(!clean)throw new Error('Terminal working directory must stay inside the workspace');
  const dirs=[...settings.custom_dirs];
  if(clean!=='.'&&!dirs.includes(clean))dirs.push(clean);
  await writeSettings({selected_cwd:clean,custom_dirs:dirs});
}

async function change(){
  if(select.value==='__custom__'){
    await chooseCustomDirectory();
    return;
  }
  await writeSettings({selected_cwd:select.value,custom_dirs:settings.custom_dirs});
}

function selectedCWD(){return settings.selected_cwd||'.';}

function install(){
  if(select?.isConnected)return;
  const header=document.querySelector('header');if(!header)return;
  select=document.createElement('select');select.id='terminal-cwd';select.title='Working directory for new terminals';
  select.onchange=()=>change().catch(error=>{app.showError(error);refresh();});
  const open=document.querySelector('#open-terminal');header.insertBefore(select,open);refresh();
  loadSettings().catch(app.showError);
}

window.fetch=async function(input,init={}){
  try{
    const method=String(init?.method||'GET').toUpperCase();
    const rawURL=typeof input==='string'?input:input?.url||'';
    const url=new URL(rawURL,location.href);
    if(method==='POST'&&url.origin===location.origin&&url.pathname==='/api/sessions'&&typeof init.body==='string'){
      const payload=JSON.parse(init.body);
      if(payload.kind==='terminal')init={...init,body:JSON.stringify({...payload,cwd:selectedCWD()})};
    }
  }catch(error){console.warn('Terminal cwd injection skipped',error);}
  return nativeFetch(input,init);
};

window.addEventListener('taskmenu:tasks',()=>loadSettings().catch(app.showError));
install();
