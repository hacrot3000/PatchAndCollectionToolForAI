const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal rename');

const tabsHost=document.querySelector('#tabs');
const migrating=new Set();
const style=document.createElement('style');
style.textContent=`
.terminal-rename{white-space:nowrap;padding:5px 8px}
`;document.head.append(style);

function workspaceID(){return String(app.taskData?.workspace||'').trim();}
function key(id){const workspace=workspaceID();return workspace?`vscode-tasks-menu:tab-title:${workspace}:${id}`:'';}
function legacyKey(id){const workspace=workspaceID();return workspace?`vscode-tasks-menu:terminal-title:${workspace}:${id}`:'';}
function saved(view){
  const cacheKey=key(view.meta.id);if(!cacheKey)return '';
  try{
    const value=localStorage.getItem(cacheKey);
    if(value!==null)return value;
    if(view.meta.task_id===0){
      const oldKey=legacyKey(view.meta.id);
      if(!oldKey)return '';
      const legacy=localStorage.getItem(oldKey);
      if(legacy!==null){localStorage.setItem(cacheKey,legacy);return legacy;}
    }
  }catch{}
  return '';
}
function store(view,value){
  const cacheKey=key(view.meta.id);if(!cacheKey)return;
  try{
    if(value)localStorage.setItem(cacheKey,value);else localStorage.removeItem(cacheKey);
    if(view.meta.task_id===0){
      const oldKey=legacyKey(view.meta.id);if(oldKey)localStorage.removeItem(oldKey);
    }
  }catch(e){console.warn('Cannot persist tab title cache',e);}
}
function brokerTitle(view){return String(view?.meta?.title||'').trim();}
function customTitle(view){return brokerTitle(view)||saved(view);}
function labelSpan(view){return view.tab.querySelector('span:first-child');}
function defaultTitle(view,label){
  if(label?.dataset.defaultTabTitle)return label.dataset.defaultTabTitle;
  const value=(label?.textContent||view.meta?.label||(view.meta.task_id===0?'Terminal':'Task')).trim()||'Tab';
  if(label)label.dataset.defaultTabTitle=value;
  return value;
}
function apply(view){
  const label=labelSpan(view);if(!label)return;
  const fallback=defaultTitle(view,label);
  label.textContent=customTitle(view)||fallback;
}
async function persistTitle(view,value){
  const meta=await app.jsonFetch('/api/sessions/'+encodeURIComponent(view.meta.id)+'/title',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({title:value})
  });
  view.meta.title=String(meta?.title||'');
  store(view,view.meta.title);
  window.dispatchEvent(new CustomEvent('taskmenu:terminal-presentation-changed',{detail:{view,kind:'title'}}));
  return view.meta.title;
}
async function migrateLegacyBrowserTitle(view){
  if(!workspaceID())return;
  if(brokerTitle(view)){store(view,brokerTitle(view));return;}
  const value=saved(view);
  if(!value||migrating.has(view.meta.id))return;
  migrating.add(view.meta.id);
  try{
    await persistTitle(view,value);
    apply(view);
  }catch(e){
    console.warn('Cannot migrate cached tab title to session broker',e);
  }finally{
    migrating.delete(view.meta.id);
  }
}
async function promptRename(view){
  const label=labelSpan(view);if(!label)return;
  const fallback=defaultTitle(view,label);
  const before=customTitle(view)||fallback;
  const answer=window.prompt('Tab title:',before);
  if(answer===null)return;
  const value=String(answer).replace(/[\r\n]+/g,' ').trim();
  await persistTitle(view,value&&value!==fallback?value:'');
  apply(view);
}
function ensure(view){
  const label=labelSpan(view);if(!label)return;
  defaultTitle(view,label);apply(view);
  void migrateLegacyBrowserTitle(view);
  label.title='Double-click to rename this tab';
  if(view.status)view.status.title='Double-click to rename this tab';
  if(view.meta.task_id===0){
    const head=view.pane.querySelector('.pane-head');
    if(head&&!head.querySelector('.terminal-rename')){
      const button=document.createElement('button');button.className='terminal-rename';button.textContent='Rename';button.title='Rename terminal tab';button.onclick=()=>promptRename(view).catch(app.showError);view.stop.before(button);
    }
  }
}

// Delegate from the tabs host so restored/reordered/new tabs always support rename.
// The changing status text remains a valid target; only the close control is excluded.
if(tabsHost&&!tabsHost.dataset.renameDelegated){
  tabsHost.dataset.renameDelegated='1';
  tabsHost.addEventListener('dblclick',event=>{
    const target=event.target instanceof Element?event.target:null;
    const tab=target?.closest('.tab[data-id]');
    if(!tab||!tabsHost.contains(tab)||target?.closest('.close'))return;
    const view=app.views.get(tab.dataset.id||'');if(!view)return;
    event.preventDefault();event.stopPropagation();promptRename(view).catch(app.showError);
  },true);
}

window.addEventListener('taskmenu:session',event=>{const view=event.detail?.view;if(view)ensure(view);});
window.addEventListener('taskmenu:tasks',()=>{for(const view of app.views.values())ensure(view);});
for(const view of app.views.values())ensure(view);
