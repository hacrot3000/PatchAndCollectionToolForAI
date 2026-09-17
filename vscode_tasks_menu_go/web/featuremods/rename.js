const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal rename');

const style=document.createElement('style');
style.textContent=`.terminal-rename{white-space:nowrap;padding:5px 8px}`;document.head.append(style);

function key(id){return `vscode-tasks-menu:terminal-title:${app.taskData.workspace}:${id}`;}
function saved(id){try{return localStorage.getItem(key(id))||'';}catch{return '';}}
function store(id,value){try{if(value)localStorage.setItem(key(id),value);else localStorage.removeItem(key(id));}catch(e){console.warn('Cannot persist terminal title',e);}}
function labelSpan(view){return view.tab.querySelector('span:first-child');}
function apply(view){if(view.meta.task_id!==0)return;const label=labelSpan(view);if(label)label.textContent=saved(view.meta.id)||'Terminal';}
function rename(view){
  if(view.meta.task_id!==0)return;
  const current=saved(view.meta.id)||'Terminal';const value=window.prompt('Terminal title (để trống để reset):',current);if(value===null)return;
  const title=value.trim();store(view.meta.id,title);apply(view);
}
function ensure(view){
  if(view.meta.task_id!==0)return;apply(view);
  const head=view.pane.querySelector('.pane-head');if(head&&!head.querySelector('.terminal-rename')){const button=document.createElement('button');button.className='terminal-rename';button.textContent='Rename';button.title='Đổi tên tab terminal';button.onclick=()=>rename(view);view.stop.before(button);}
  const label=labelSpan(view);if(label&&!label.dataset.renameBound){label.dataset.renameBound='1';label.title='Double-click để đổi tên terminal';label.ondblclick=e=>{e.stopPropagation();rename(view);};}
}
window.addEventListener('taskmenu:session',event=>{const view=event.detail?.view;if(view)ensure(view);});
for(const view of app.views.values())ensure(view);
