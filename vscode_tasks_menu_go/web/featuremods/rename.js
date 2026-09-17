const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal rename');

const tabsHost=document.querySelector('#tabs');
const style=document.createElement('style');
style.textContent=`
.terminal-rename{white-space:nowrap;padding:5px 8px}
`;document.head.append(style);

function key(id){return `vscode-tasks-menu:tab-title:${app.taskData.workspace}:${id}`;}
function legacyKey(id){return `vscode-tasks-menu:terminal-title:${app.taskData.workspace}:${id}`;}
function saved(view){
  try{
    const value=localStorage.getItem(key(view.meta.id));
    if(value!==null)return value;
    if(view.meta.task_id===0){
      const legacy=localStorage.getItem(legacyKey(view.meta.id));
      if(legacy!==null){localStorage.setItem(key(view.meta.id),legacy);return legacy;}
    }
  }catch{}
  return '';
}
function store(view,value){
  try{
    if(value)localStorage.setItem(key(view.meta.id),value);else localStorage.removeItem(key(view.meta.id));
    if(view.meta.task_id===0)localStorage.removeItem(legacyKey(view.meta.id));
  }catch(e){console.warn('Cannot persist tab title',e);}
}
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
  label.textContent=saved(view)||fallback;
}
function promptRename(view){
  const label=labelSpan(view);if(!label)return;
  const fallback=defaultTitle(view,label);
  const before=saved(view)||fallback;
  const answer=window.prompt('Tab title:',before);
  if(answer===null)return;
  const value=String(answer).replace(/[\r\n]+/g,' ').trim();
  store(view,value&&value!==fallback?value:'');
  apply(view);
}
function ensure(view){
  const label=labelSpan(view);if(!label)return;
  defaultTitle(view,label);apply(view);
  label.title='Double-click to rename this tab';
  if(view.status)view.status.title='Double-click to rename this tab';
  if(view.meta.task_id===0){
    const head=view.pane.querySelector('.pane-head');
    if(head&&!head.querySelector('.terminal-rename')){
      const button=document.createElement('button');button.className='terminal-rename';button.textContent='Rename';button.title='Rename terminal tab';button.onclick=()=>promptRename(view);view.stop.before(button);
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
    event.preventDefault();event.stopPropagation();promptRename(view);
  },true);
}

window.addEventListener('taskmenu:session',event=>{const view=event.detail?.view;if(view)ensure(view);});
for(const view of app.views.values())ensure(view);
