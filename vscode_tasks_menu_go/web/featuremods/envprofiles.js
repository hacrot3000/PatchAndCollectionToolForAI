const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for environment profiles');

const nativeFetch=window.fetch.bind(window);
const profilesKey=()=>`vscode-tasks-menu:env-profiles:${app.taskData.workspace}`;
const selectedKey=()=>`vscode-tasks-menu:env-profile-selected:${app.taskData.workspace}`;
let select=null,manage=null,projectOverride=null;

function readProfiles(){
  try{const value=JSON.parse(localStorage.getItem(profilesKey())||'{}');return value&&typeof value==='object'&&!Array.isArray(value)?value:{};}catch{return {};}
}
function writeProfiles(value){try{localStorage.setItem(profilesKey(),JSON.stringify(value));}catch(e){throw new Error('Cannot save environment profiles: '+e.message);}}
function selectedName(){try{return localStorage.getItem(selectedKey())||'';}catch{return '';}}
function currentName(){return projectOverride?.name||selectedName();}
function normalizeEnvObject(env){
  const out={};for(const [key,value] of Object.entries(env&&typeof env==='object'&&!Array.isArray(env)?env:{})){if(!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key))throw new Error('Invalid environment variable name: '+key);const text=String(value??'');if(text.includes('\0'))throw new Error('Environment value contains NUL: '+key);out[key]=text;}return out;
}
function setSelected(value){projectOverride=null;try{if(value)localStorage.setItem(selectedKey(),value);else localStorage.removeItem(selectedKey());}catch{}refreshSelect(value);}
function currentEnv(){if(projectOverride)return {...projectOverride.env};const name=selectedName();if(!name)return {};return readProfiles()[name]||{};}
function applySnapshot(name,env){projectOverride={name:String(name||'Project profile environment').trim()||'Project profile environment',env:normalizeEnvObject(env)};refreshSelect();return {...projectOverride.env};}
function clearOverride(){projectOverride=null;refreshSelect();}
function parseEnvLines(text){
  const out={};for(const raw of String(text||'').split(/\r?\n/)){const line=raw.trim();if(!line||line.startsWith('#'))continue;const i=line.indexOf('=');if(i<1)throw new Error(`Invalid environment line: ${raw}`);const key=line.slice(0,i).trim();if(!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key))throw new Error(`Invalid environment variable name: ${key}`);out[key]=line.slice(i+1);}
  return out;
}
function envText(env){return Object.entries(env||{}).map(([k,v])=>`${k}=${v}`).join('\n');}

function refreshSelect(preferred){
  if(!select)return;const profiles=readProfiles();const chosen=preferred!==undefined?preferred:selectedName();select.replaceChildren();
  const def=document.createElement('option');def.value='';def.textContent='Env: Default';select.append(def);
  if(projectOverride){const option=document.createElement('option');option.value='__taskdeck_project_profile__';option.textContent='Env: '+projectOverride.name+' (project profile)';select.append(option);}
  for(const name of Object.keys(profiles).sort()){const option=document.createElement('option');option.value=name;option.textContent='Env: '+name;select.append(option);}
  select.value=projectOverride?'__taskdeck_project_profile__':(profiles[chosen]?chosen:'');
}
function manageProfiles(){
  const profiles=readProfiles();const existing=Object.keys(profiles).sort();
  const nameRaw=window.prompt('Environment profile name.\nExisting profiles: '+(existing.join(', ')||'(none)')+'\n\nEnter a name to create/edit, or prefix it with - to delete (for example -Debug):',selectedName()||existing[0]||'');if(nameRaw===null)return;
  const raw=nameRaw.trim();if(!raw)return;
  if(raw.startsWith('-')){const name=raw.slice(1).trim();if(name&&profiles[name]&&window.confirm(`Delete environment profile ${name}?`)){delete profiles[name];writeProfiles(profiles);if(selectedName()===name)setSelected('');else refreshSelect();}return;}
  const name=raw;const text=window.prompt(`Environment variables for ${name}, one NAME=VALUE per line:`,envText(profiles[name]||{}));if(text===null)return;
  const env=parseEnvLines(text);profiles[name]=env;writeProfiles(profiles);setSelected(name);
}

function install(){
  if(select?.isConnected)return;const header=document.querySelector('header');if(!header)return;
  select=document.createElement('select');select.id='env-profile';select.title='Environment profile applied to new local tasks and terminals';select.onchange=()=>{if(select.value==='__taskdeck_project_profile__')return;setSelected(select.value);};
  manage=document.createElement('button');manage.id='env-profile-manage';manage.textContent='Env…';manage.title='Create, edit, or delete environment profiles';manage.onclick=()=>{try{manageProfiles();}catch(e){app.showError(e);}};
  const reload=document.querySelector('#reload');header.insertBefore(select,reload);header.insertBefore(manage,reload);refreshSelect();
}

window.fetch=async function(input,init={}){
  try{
    const method=String(init?.method||'GET').toUpperCase();const rawURL=typeof input==='string'?input:input?.url||'';const url=new URL(rawURL,location.href);
    if(method==='POST'&&url.origin===location.origin&&url.pathname==='/api/sessions'&&typeof init.body==='string'){
      const env=currentEnv();if(Object.keys(env).length){const payload=JSON.parse(init.body);if(!payload?.ssh_profile_id){const explicit=payload?.env&&typeof payload.env==='object'&&!Array.isArray(payload.env)?payload.env:{};init={...init,body:JSON.stringify({...payload,env:{...env,...explicit}})};}}
    }
  }catch(e){console.warn('Environment profile injection skipped',e);}
  return nativeFetch(input,init);
};
globalThis.TaskMenuEnvProfiles={
  list(){return {...readProfiles()};},
  get(name){name=String(name||'').trim();const value=readProfiles()[name];return value?{...value}:null;},
  currentName,
  currentEnv,
  applySnapshot,
  clearOverride,
  has(name){name=String(name||'').trim();return !name||Object.prototype.hasOwnProperty.call(readProfiles(),name);},
  select(name){
    name=String(name||'').trim();
    if(name&&!Object.prototype.hasOwnProperty.call(readProfiles(),name))throw new Error('Environment profile "'+name+'" is not available in this browser');
    setSelected(name);return name;
  },
  refresh:refreshSelect
};
install();
