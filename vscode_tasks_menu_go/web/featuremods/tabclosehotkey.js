const app=globalThis.TaskMenuApp;
if(!app)throw new Error('TaskMenuApp unavailable for close-tab shortcut');

const tabsHost=document.querySelector('#tabs');
if(!tabsHost)throw new Error('Tab host unavailable for close-tab shortcut');

// Ctrl+W is browser-reserved on many Firefox/Chromium configurations.
// Ctrl+Alt+W is the reliable TaskDeck fallback; both use the same handler
// if the browser actually dispatches the keydown event to page content.
function isCloseTabShortcut(event){
  const primary=event.ctrlKey||event.metaKey;
  return primary&&!event.shiftKey&&String(event.key||'').toLowerCase()==='w';
}

function activeWorkspaceTab(){
  const active=String(app.active||'');
  const id=active.startsWith('external:')?active.slice('external:'.length):active;
  const tabs=[...tabsHost.children].filter(tab=>tab instanceof HTMLElement&&!tab.hidden&&tab.classList.contains('tab'));
  // Prefer the active TaskDeck view token to avoid an obsolete .active class
  // from another panel, e.g. after switching between terminal and editor.
  const exact=id?tabs.find(tab=>tab.dataset.id===id&&tab.classList.contains('active')):null;
  return exact||tabs.find(tab=>tab.classList.contains('active'))||null;
}

function activeTabCloseControl(){
  const tab=activeWorkspaceTab();
  if(!tab)return null;
  // Only use the same close button as a direct user click. In particular,
  // editor dirty-save prompts and Patch Tool lifecycle remain unchanged.
  return [...tab.children].find(child=>child.classList.contains('close')&&typeof child.click==='function')||null;
}

function closeActiveToolTab(){
  const close=activeTabCloseControl();
  if(!close)return false;
  close.click();
  return true;
}

function closeToolTabShortcut(event){
  if(!isCloseTabShortcut(event)||event.repeat||event.defaultPrevented)return;
  if(!activeTabCloseControl())return;
  event.preventDefault();
  event.stopPropagation();
  // Synthesize only the existing close-control click; never close the browser.
  closeActiveToolTab();
}

window.addEventListener('keydown',closeToolTabShortcut,true);

globalThis.TaskMenuTabCloseShortcut={
  shortcut:'Ctrl+Alt+W',
  browserShortcut:'Ctrl+W (only when the browser forwards it)',
  matches:isCloseTabShortcut,
  closeActive:closeActiveToolTab
};
