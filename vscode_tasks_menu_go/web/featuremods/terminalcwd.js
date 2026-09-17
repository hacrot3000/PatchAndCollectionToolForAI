const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal cwd selector');

const nativeFetch=window.fetch.bind(window);
const key=()=>`vscode-tasks-menu:terminal-cwd:${app.taskData.workspace}`;
let select=null;
function saved(){try{return localStorage.getItem(key())||'.';}catch{return '.';}}
function save(value){try{localStorage.setItem(key(),value);}catch{}}
function normalizeCandidate(value){
  value=String(value||'').trim().replace(/\\/g,'/');if(!value||value==='${workspaceFolder}')return '.';
  const prefix='${workspaceFolder}/';if(value.startsWith(prefix))value=value.slice(prefix.length);
  if(value.includes('${')||value.startsWith('/')||value==='..'||value.startsWith('../')||value.includes('/../'))return '';
  return value.replace(/^\.\//,'')||'.';
}
function candidates(){
  const values=new Set(['.']);
  for(const task of app.taskData?.tasks||[]){const value=normalizeCandidate(task?.options?.cwd);if(value)values.add(value);}
  return [...values].sort((a,b)=>a==='.'?-1:b==='.'?1:a.localeCompare(b));
}
function refresh(){
  if(!select)return;const chosen=saved();select.replaceChildren();
  for(const value of candidates()){const o=document.createElement('option');o.value=value;o.textContent=value==='.'?'Terminal: project root':'Terminal: '+value;select.append(o);}
  const custom=document.createElement('option');custom.value='__custom__';custom.textContent='Terminal: Custom…';select.append(custom);
  if([...select.options].some(o=>o.value===chosen))select.value=chosen;else {const o=document.createElement('option');o.value=chosen;o.textContent='Terminal: '+chosen;select.insertBefore(o,custom);select.value=chosen;}
}
function change(){
  if(select.value!=='__custom__'){save(select.value);return;}
  const value=window.prompt('Relative workspace directory for new terminals:',saved());if(value===null){refresh();return;}
  const clean=normalizeCandidate(value);if(!clean){app.showError(new Error('Terminal working directory must stay inside the workspace'));refresh();return;}save(clean);refresh();
}
function install(){
  if(select?.isConnected)return;const header=document.querySelector('header');if(!header)return;
  select=document.createElement('select');select.id='terminal-cwd';select.title='Working directory for new terminals';select.onchange=change;
  const open=document.querySelector('#open-terminal');header.insertBefore(select,open);refresh();
}
window.fetch=async function(input,init={}){
  try{const method=String(init?.method||'GET').toUpperCase();const rawURL=typeof input==='string'?input:input?.url||'';const url=new URL(rawURL,location.href);if(method==='POST'&&url.origin===location.origin&&url.pathname==='/api/sessions'&&typeof init.body==='string'){const payload=JSON.parse(init.body);if(payload.kind==='terminal')init={...init,body:JSON.stringify({...payload,cwd:saved()})};}}catch(e){console.warn('Terminal cwd injection skipped',e);}return nativeFetch(input,init);
};
window.addEventListener('taskmenu:tasks',refresh);install();
