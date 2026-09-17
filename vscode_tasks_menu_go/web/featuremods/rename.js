const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal rename');

const style=document.createElement('style');
style.textContent=`
.terminal-rename{white-space:nowrap;padding:5px 8px}
.tab .tab-title-editing{display:inline-block;min-width:48px;max-width:320px;padding:1px 4px;margin:-2px 0;border:1px solid #5a88b4;border-radius:4px;background:#0d1117;color:inherit;outline:none;white-space:nowrap;overflow:hidden;text-overflow:clip}
html[data-taskmenu-theme="light"] .tab .tab-title-editing{background:#fff}
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
  if(label.dataset.editing==='1')return;
  label.textContent=saved(view)||fallback;
}
function selectAllText(element){
  const selection=window.getSelection?.();if(!selection)return;
  const range=document.createRange();range.selectNodeContents(element);selection.removeAllRanges();selection.addRange(range);
}
function beginInlineRename(view){
  const label=labelSpan(view);if(!label||label.dataset.editing==='1')return;
  const fallback=defaultTitle(view,label);
  const before=saved(view)||label.textContent.trim()||fallback;
  label.dataset.editing='1';label.classList.add('tab-title-editing');label.contentEditable='true';label.spellcheck=false;
  label.textContent=before;label.focus();selectAllText(label);

  let finished=false;
  const finish=save=>{
    if(finished)return;finished=true;
    const value=(label.textContent||'').replace(/[\r\n]+/g,' ').trim();
    label.contentEditable='false';label.classList.remove('tab-title-editing');delete label.dataset.editing;
    label.onkeydown=null;label.onblur=null;
    if(save){store(view,value&&value!==fallback?value:'');}
    label.textContent=save?(saved(view)||fallback):before;
  };
  label.onkeydown=e=>{
    if(e.key==='Enter'){e.preventDefault();e.stopPropagation();finish(true);}
    else if(e.key==='Escape'){e.preventDefault();e.stopPropagation();finish(false);}
  };
  label.onblur=()=>finish(true);
}
function ensure(view){
  const label=labelSpan(view);if(!label)return;
  defaultTitle(view,label);apply(view);
  if(!label.dataset.renameBound){
    label.dataset.renameBound='1';label.title='Double-click để đổi tên tab';
    label.ondblclick=e=>{e.preventDefault();e.stopPropagation();beginInlineRename(view);};
  }
  // Keep the existing Rename action for terminal tabs, but route it through
  // the same inline editor used by double-click.
  if(view.meta.task_id===0){
    const head=view.pane.querySelector('.pane-head');
    if(head&&!head.querySelector('.terminal-rename')){
      const button=document.createElement('button');button.className='terminal-rename';button.textContent='Rename';button.title='Đổi tên tab terminal';button.onclick=()=>beginInlineRename(view);view.stop.before(button);
    }
  }
}
window.addEventListener('taskmenu:session',event=>{const view=event.detail?.view;if(view)ensure(view);});
for(const view of app.views.values())ensure(view);
