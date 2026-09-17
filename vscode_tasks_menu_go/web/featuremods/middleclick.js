const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for middle-click paste');

let primarySelection='';
const installed=new WeakSet();

function rememberSelection(view){
  const text=view?.term?.getSelection?.()||'';
  if(text)primarySelection=text;
}

function install(view){
  if(!view?.term||installed.has(view))return;
  installed.add(view);
  view.term.onSelectionChange(()=>rememberSelection(view));
  const element=view.term.element||view.pane?.querySelector('.xterm');
  if(!element)return;
  element.addEventListener('mousedown',event=>{
    if(event.button===1){
      event.preventDefault();
      event.stopPropagation();
    }
  },true);
  element.addEventListener('auxclick',event=>{
    if(event.button!==1)return;
    event.preventDefault();
    event.stopPropagation();
    if(!primarySelection)return;
    try{
      view.term.focus();
      view.term.paste(primarySelection);
    }catch(error){app.showError(error);}
  },true);
}

window.addEventListener('taskmenu:session',event=>install(event.detail?.view));
for(const view of app.views.values())install(view);
