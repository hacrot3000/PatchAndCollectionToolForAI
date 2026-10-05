const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for terminal shortcut');

function isNewTerminalShortcut(event){
  const primary=event.ctrlKey||event.metaKey;
  return primary&&event.shiftKey&&!event.altKey&&event.code==='Backquote';
}

function openTerminalShortcut(event){
  if(!isNewTerminalShortcut(event))return;
  event.preventDefault();
  event.stopPropagation();
  if(event.repeat)return;
  app.startTerminal().catch(app.showError);
}

window.addEventListener('keydown',openTerminalShortcut,true);

globalThis.TaskMenuTerminalShortcut={
  shortcut:'Ctrl+Shift+`',
  matches:isNewTerminalShortcut
};
