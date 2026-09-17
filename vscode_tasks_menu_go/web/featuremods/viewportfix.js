const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for viewport layout fix');

// Keep the application itself pinned to the browser viewport. Grid/flex items use
// min-height:auto by default, so a tall xterm/sidebar can otherwise grow the
// implicit main grid row beyond the declared main height and create a large,
// empty document scrollbar. Scrolling belongs inside the sidebar/xterm only.
const style=document.createElement('style');
style.dataset.taskmenuViewportFix='1';
style.textContent=`
html,body{width:100%;height:100%;min-height:0;overflow:hidden;overscroll-behavior:none}
body{height:100vh;height:100dvh;display:flex;flex-direction:column}
body>header{flex:0 0 52px;min-height:52px;max-height:52px}
body>main{flex:1 1 auto;height:auto!important;min-height:0;max-height:calc(100% - 52px);overflow:hidden;grid-template-rows:minmax(0,1fr)}
body>main>aside{min-height:0;max-height:100%;overflow:auto}
body>main>section{min-width:0;min-height:0;max-height:100%;overflow:hidden}
#tabs{flex:0 0 42px;min-height:0;overflow-x:auto;overflow-y:hidden}
#panes{flex:1 1 auto;min-width:0;min-height:0;max-height:100%;overflow:hidden}
#panes>.pane{min-width:0;min-height:0;max-height:100%;overflow:hidden}
#panes>.pane>.terminal{min-width:0;min-height:0;overflow:hidden}
`;
document.head.append(style);
