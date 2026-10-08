// Shared viewport-fitting policy for all right-click menus and nested submenus.
// Menus must be made visible and appended to the document before this is called.
const margin=6;
const css=document.createElement('style');
css.textContent=`
.project-explorer-context,.ft-context,.task-connection-context-menu,
.git-graph-context,.project-file-context,.db-context-menu,
.tab-context-menu,.tab-context-submenu{
 box-sizing:border-box;
 max-height:calc(100dvh - 12px);
 max-width:calc(100vw - 12px);
 overflow-y:auto;
 overflow-x:auto;
 overscroll-behavior:contain;
 scrollbar-gutter:stable;
}
`;
document.head.append(css);
function viewport(){
 const vv=window.visualViewport;
 const left=vv?vv.offsetLeft:0,top=vv?vv.offsetTop:0;
 const width=vv?vv.width:window.innerWidth,height=vv?vv.height:window.innerHeight;
 return {left,top,right:left+Math.max(0,width),bottom:top+Math.max(0,height)};
}
function bounded(value,min,max){
 return Math.min(Math.max(Number(value)||0,min),Math.max(min,max));
}
function place(menu,x,y,{anchor=null,submenu=false,gap=4}={}){
 if(!menu?.isConnected)return null;
 const v=viewport(),roomWidth=Math.max(0,v.right-v.left-2*margin),roomHeight=Math.max(0,v.bottom-v.top-2*margin);
 // Apply this BEFORE measuring so oversized menus become scrollable instead of
 // having a negative top or hiding their bottom actions below the viewport.
 menu.style.boxSizing='border-box';
 menu.style.maxWidth=roomWidth+'px';
 // A CSS min-width must not override the available width on narrow windows.
 const requestedMinimum=parseFloat(window.getComputedStyle(menu).minWidth)||0;
 menu.style.minWidth=Math.min(requestedMinimum,roomWidth)+'px';
 menu.style.maxHeight=roomHeight+'px';
 menu.style.overflowY='auto';
 menu.style.overscrollBehavior='contain';
 const rect=menu.getBoundingClientRect();
 const width=Math.min(rect.width,roomWidth),height=Math.min(rect.height,roomHeight);
 let left=Number(x)||0,top=Number(y)||0;
 if(anchor&&submenu){
   left=anchor.right+gap;
   if(left+width>v.right-margin)left=anchor.left-width-gap;
   top=anchor.top;
 }else if(anchor){
   left=anchor.left;
   top=anchor.bottom+gap;
   if(top+height>v.bottom-margin&&anchor.top-height-gap>=v.top+margin)top=anchor.top-height-gap;
 }
 left=bounded(left,v.left+margin,v.right-margin-width);
 top=bounded(top,v.top+margin,v.bottom-margin-height);
 menu.style.left=left+'px';menu.style.top=top+'px';
 return {left,top,width,height,scrollable:rect.height>=roomHeight};
}
globalThis.TaskMenuContextViewport={place,viewport};
