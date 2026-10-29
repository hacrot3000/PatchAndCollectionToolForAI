const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for sidebar resize');

let initialized=false;
function initializeSidebar(){
  if(app.layoutProfile==='mobile'||initialized||!app.taskData?.workspace)return;
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
  .task-sidebar-floating-resizer{display:none;position:fixed;z-index:1950;top:var(--taskmenu-header-height,30px);bottom:0;left:calc(48px + var(--taskmenu-sidebar-panel-width,310px) - 5px);width:10px;cursor:col-resize;touch-action:none;background:transparent}
  .task-sidebar-floating-resizer::after{content:'';position:absolute;top:0;bottom:0;left:4px;width:2px;background:#5a6575;opacity:.55}
  .task-sidebar-floating-resizer:hover::after,.task-sidebar-floating-resizer.dragging::after,.task-sidebar-floating-resizer:focus-visible::after{left:3px;width:4px;background:#73a9d2;opacity:1}
  body.task-sidebar-auto-hide.task-sidebar-resize-panel-open .task-sidebar-floating-resizer{display:block}
  .sidebar-resizer:focus-visible,.task-sidebar-floating-resizer:focus-visible{outline:2px solid #73a9d2;outline-offset:-2px}
  body.sidebar-resizing{user-select:none;cursor:col-resize}
  `;
  document.head.append(style);

  const resizer=document.createElement('div');
  resizer.className='sidebar-resizer';resizer.title='Drag to resize the sidebar · double-click to reset';
  const floatingResizer=document.createElement('div');
  floatingResizer.className='task-sidebar-floating-resizer';
  floatingResizer.title='Drag to resize the open sidebar panel · double-click to reset';
  // This handle stays outside the fixed-position panel so it works for Tasks,
  // Explorer, Connections, History, and Schedules without modifying each panel.
  floatingResizer.dataset.taskSidebarKeepOpen='1';

  const key='vscode-tasks-menu:sidebar-width:'+app.taskData.workspace;
  const defaultWidth=310;
  function autoHide(){return document.body.classList.contains('task-sidebar-auto-hide');}
  function limits(){
    const min=autoHide()?Math.min(220,Math.max(140,window.innerWidth-72)):220;
    const max=Math.max(min,Math.min(650,window.innerWidth-(autoHide()?96:320)));
    return {min,max};
  }
  function clamp(value){const {min,max}=limits();return Math.min(max,Math.max(min,Math.round(value)||defaultWidth));}
  function apply(width){
    const value=clamp(width);
    main.style.gridTemplateColumns=value+'px 6px minmax(0,1fr)';
    // All left-side panels consume this one width in floating mode.
    document.documentElement.style.setProperty('--taskmenu-sidebar-panel-width',value+'px');
    main.style.setProperty('--taskmenu-sidebar-panel-width',value+'px');
    for(const handle of [resizer,floatingResizer]){
      handle.setAttribute('aria-valuenow',String(value));
      handle.setAttribute('aria-valuemin',String(limits().min));
      handle.setAttribute('aria-valuemax',String(limits().max));
    }
    return value;
  }
  function load(){let value=defaultWidth;try{value=Number(localStorage.getItem(key))||defaultWidth;}catch{}return apply(value);}
  function save(value){try{localStorage.setItem(key,String(value));}catch(e){console.warn('Cannot persist sidebar width',e);}}

  // Apply the three-column grid before inserting the resizer. This prevents the
  // terminal/task section from being auto-placed on a second grid row if startup
  // is interrupted or delayed while task data is still loading.
  let width=load();
  aside.after(resizer);
  document.body.append(floatingResizer);
  let activeHandle=null;
  function decorateHandle(handle){
    handle.tabIndex=0;
    handle.setAttribute('role','separator');
    handle.setAttribute('aria-label','Resize left sidebar');
    handle.setAttribute('aria-orientation','vertical');
    handle.addEventListener('pointerdown',event=>{
      if(event.button!==0||activeHandle)return;
      activeHandle=handle;
      handle.classList.add('dragging');
      document.body.classList.add('sidebar-resizing');
      handle.setPointerCapture(event.pointerId);
      event.preventDefault();event.stopPropagation();
    });
    handle.addEventListener('pointermove',event=>{
      if(activeHandle!==handle)return;
      const rect=main.getBoundingClientRect();
      const left=handle===floatingResizer?48:0;
      width=apply(event.clientX-rect.left-left);
      event.preventDefault();
    });
    const finish=event=>{
      if(activeHandle!==handle)return;
      activeHandle=null;
      handle.classList.remove('dragging');
      document.body.classList.remove('sidebar-resizing');
      try{if(event&&handle.hasPointerCapture(event.pointerId))handle.releasePointerCapture(event.pointerId);}catch{}
      save(width);
    };
    handle.addEventListener('pointerup',finish);
    handle.addEventListener('pointercancel',finish);
    handle.addEventListener('lostpointercapture',finish);
    handle.addEventListener('dblclick',()=>{width=apply(defaultWidth);save(width);});
    handle.addEventListener('keydown',event=>{
      const step=event.shiftKey?25:10;
      let next=width;
      if(event.key==='ArrowLeft')next-=step;
      else if(event.key==='ArrowRight')next+=step;
      else if(event.key==='Home')next=limits().min;
      else if(event.key==='End')next=limits().max;
      else return;
      event.preventDefault();event.stopPropagation();
      width=apply(next);save(width);
    });
  }
  decorateHandle(resizer);
  decorateHandle(floatingResizer);
  window.addEventListener('resize',()=>{width=apply(width);});
  // The activity bar owns mode switches. Re-clamp when switching between
  // inline and floating layouts without discarding the saved width.
  window.addEventListener('taskmenu:sidebar-layout',()=>{width=apply(width);});
}

if(app.taskData?.workspace)initializeSidebar();
else window.addEventListener('taskmenu:tasks',initializeSidebar,{once:true});
