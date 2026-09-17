const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for sidebar resize');

let initialized=false;
function initializeSidebar(){
  if(initialized||!app.taskData?.workspace)return;
  const main=document.querySelector('main');
  const aside=document.querySelector('aside');
  const section=document.querySelector('main>section');
  if(!main||!aside||!section)return;
  initialized=true;

  const style=document.createElement('style');
  style.textContent=`
  .sidebar-resizer{width:6px;cursor:col-resize;background:transparent;position:relative;touch-action:none}
  .sidebar-resizer::after{content:'';position:absolute;top:0;bottom:0;left:2px;width:1px;background:#30343b}
  .sidebar-resizer:hover::after,.sidebar-resizer.dragging::after{left:1px;width:3px;background:#5a6575}
  body.sidebar-resizing{user-select:none;cursor:col-resize}
  `;
  document.head.append(style);

  const resizer=document.createElement('div');
  resizer.className='sidebar-resizer';resizer.title='Kéo để đổi độ rộng menu · double-click để reset';

  const key='vscode-tasks-menu:sidebar-width:'+app.taskData.workspace;
  const defaultWidth=310;
  function limits(){return {min:220,max:Math.max(220,Math.min(650,window.innerWidth-320))};}
  function clamp(value){const {min,max}=limits();return Math.min(max,Math.max(min,Math.round(value)||defaultWidth));}
  function apply(width){
    const value=clamp(width);main.style.gridTemplateColumns=value+'px 6px minmax(0,1fr)';return value;
  }
  function load(){let value=defaultWidth;try{value=Number(localStorage.getItem(key))||defaultWidth;}catch{}return apply(value);}
  function save(value){try{localStorage.setItem(key,String(value));}catch(e){console.warn('Cannot persist sidebar width',e);}}

  // Apply the three-column grid before inserting the resizer. This prevents the
  // terminal/task section from being auto-placed on a second grid row if startup
  // is interrupted or delayed while task data is still loading.
  let width=load();
  aside.after(resizer);
  let dragging=false;

  resizer.addEventListener('pointerdown',event=>{
    if(event.button!==0)return;
    dragging=true;resizer.classList.add('dragging');document.body.classList.add('sidebar-resizing');resizer.setPointerCapture(event.pointerId);event.preventDefault();
  });
  resizer.addEventListener('pointermove',event=>{
    if(!dragging)return;
    const rect=main.getBoundingClientRect();width=apply(event.clientX-rect.left);event.preventDefault();
  });
  function finish(event){
    if(!dragging)return;
    dragging=false;resizer.classList.remove('dragging');document.body.classList.remove('sidebar-resizing');
    try{if(event&&resizer.hasPointerCapture(event.pointerId))resizer.releasePointerCapture(event.pointerId);}catch{}
    save(width);
  }
  resizer.addEventListener('pointerup',finish);resizer.addEventListener('pointercancel',finish);
  resizer.addEventListener('dblclick',()=>{width=apply(defaultWidth);save(width);});
  window.addEventListener('resize',()=>{width=apply(width);});
}

if(app.taskData?.workspace)initializeSidebar();
else window.addEventListener('taskmenu:tasks',initializeSidebar,{once:true});
