const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal shortcut');

function isNewTerminalShortcut(event){
  const primary=event.ctrlKey||event.metaKey;
  return primary&&event.shiftKey&&!event.altKey&&event.code==='Backquote';
}
function isFocusTerminalShortcut(event){
  const primary=event.ctrlKey||event.metaKey;
  return primary&&!event.shiftKey&&!event.altKey&&event.code==='Backquote';
}

// A "running" terminal session only means the Bash process is alive.
// Ctrl+Backquote needs an IDLE shell (no command executing).
// Never focus a stopped shell, running task, SSH session or read-only tab.
function idleLocalTerminal(view){
  if(!view||view.closed||view.closing||!view.term||!view.canControl||view.tabReadOnly||
     view.meta?.status!=='running'||view.ws?.readyState!==WebSocket.OPEN)return false;
  const local=globalThis.TaskMenuTerminalClone?.localTerminal?.(view);
  if(local!==true)return false;
  const shell=globalThis.TaskDeckShellIntegration?.getState?.(String(view.meta.id));
  // No trustworthy prompt yet: don't mistake a running command for idle.
  return Boolean(shell?.seenPrompt&&!shell.executing);
}
let lastActivated='';
window.addEventListener('taskmenu:view-activated',event=>{
  if(event.detail?.kind==='terminal')lastActivated=String(event.detail.id||'');
});
function findIdleTerminal(){
  const candidates=[...app.views.values()].filter(idleLocalTerminal);
  if(!candidates.length)return null;
  // Reuse the current idle shell first, then the most recently active one.
  return candidates.find(view=>String(view.meta.id)===String(app.active))||
    candidates.find(view=>String(view.meta.id)===lastActivated)||
    candidates[0];
}
let opening=null;
function focusOrCreateTerminal(){
  const idle=findIdleTerminal();
  if(idle){
    // A keyboard shortcut is an explicit tab switch, like clicking the tab.
    if(app.activateView(String(idle.meta.id),{focus:true,force:true}))return Promise.resolve(idle);
  }
  if(opening)return opening;
  opening=Promise.resolve().then(()=>app.startTerminal()).finally(()=>{opening=null;});
  return opening;
}
function openTerminalShortcut(event){
  if(!isNewTerminalShortcut(event)&&!isFocusTerminalShortcut(event))return;
  if(event.defaultPrevented||event.isComposing||event.getModifierState?.('AltGraph'))return;
  event.preventDefault();
  event.stopPropagation();
  if(event.repeat)return;
  if(isNewTerminalShortcut(event)){
    // Keep Ctrl+Shift+Backquote as unconditional "New Terminal".
    app.startTerminal().catch(app.showError);
  }else{
    focusOrCreateTerminal().catch(app.showError);
  }
}

window.addEventListener('keydown',openTerminalShortcut,true);

globalThis.TaskMenuTerminalShortcut={
  shortcut:'Ctrl+Shift+`',
  focusShortcut:'Ctrl+`',
  matches:isNewTerminalShortcut,
  matchesFocus:isFocusTerminalShortcut,
  focusOrCreate:focusOrCreateTerminal,
  findIdleTerminal
};
