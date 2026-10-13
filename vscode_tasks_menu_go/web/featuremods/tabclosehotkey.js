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
// Remember whether Escape started with an open overlay. Many overlays close
// themselves at document keydown without preventDefault(), so checking only at
// window bubble time would also close the underlying editor in the same press.
const escapeOverlayEvents=new WeakSet();
const escapeOverlaySelector=[
  '[role="dialog"][aria-modal="true"]',
  '.tab-context-menu.open','.tab-context-submenu.open',
  '.taskmenu-menu-popover.open','.taskmenu-menu-popover.visible',
  '.project-search-panel.visible','.git-panel.visible',
  '.file-preview-backdrop.visible',
  '.quick-open-backdrop.visible',
  '.project-symbols-backdrop.visible',
  '.terminal-search-all-backdrop.visible',
  '.taskmenu-menu-popover[aria-expanded="true"]'
].join(',');
function visibleEscapeOverlay(){
  return [...document.querySelectorAll(escapeOverlaySelector)].some(node=>node.getClientRects().length>0);
}
function captureEscapeOverlay(event){
  if(event.key==='Escape'&&visibleEscapeOverlay())escapeOverlayEvents.add(event);
}
function escapeClosableTab(tab){
  return Boolean(tab&&(tab.matches(
    '.file-compare-tab,.dircmp-tab,.hex-tab,.editor-tab,.task-patch-tab'
  )));
}
function closeActiveToolTabOnEscape(event){
  if(event.key!=='Escape'||event.repeat||event.defaultPrevented||
      event.ctrlKey||event.altKey||event.metaKey||event.shiftKey||
      escapeOverlayEvents.has(event))return false;
  const tab=activeWorkspaceTab();
  if(!escapeClosableTab(tab))return false;
  if(visibleEscapeOverlay())return false;
  if(tab.classList.contains('editor-tab')){
    const editor=globalThis.TaskMenuEditor;
    const view=editor?.editors?.get?.(String(tab.dataset.id||''));
    if(!view||view.closed)return false;
    // First Escape closes the inline Find/Replace toolbar, never the tab.
    if(view.findPanel&&!view.findPanel.classList.contains('hidden')){
      event.preventDefault();event.stopPropagation();
      editor.closeEditorFind?.(view);
      return true;
    }
    // dirty alone is insufficient: Save/Undo clears it. A tab once edited
    // remains protected until explicitly closed by its X or another action.
    if(view.dirty||view.everEdited)return false;
  }
  const close=activeTabCloseControl();
  if(!close)return false;
  event.preventDefault();event.stopPropagation();
  close.click();
  return true;
}

window.addEventListener('keydown',captureEscapeOverlay,true);
window.addEventListener('keydown',closeActiveToolTabOnEscape);


globalThis.TaskMenuTabCloseShortcut={
  shortcut:'Ctrl+Alt+W',
  browserShortcut:'Ctrl+W (only when the browser forwards it)',
  matches:isCloseTabShortcut,
  closeActive:closeActiveToolTab,
  closeOnEscape:closeActiveToolTabOnEscape,
  escapeEligible:escapeClosableTab
};
