package server

import (
  "os"
  "strings"
  "testing"

  webassets "bletonfc/vscode_tasks_menu/web"
)

func requireTabFeatureStrings(t *testing.T, filename string, wishes ...string) {
  t.Helper()
  data, err := webassets.Files.ReadFile("featuremods/"+filename)
  if err != nil { t.Fatal(err) }
  for _, want := range wishes {
    if !strings.Contains(string(data),want) {
      t.Errorf("%s missing behavior %q",filename,want)
    }
  }
}

func TestEmptyTabStripContextMenuAndClosedTabReopen(t *testing.T) {
  requireTabFeatureStrings(t,"tabcontext.js",
    "function openEmptyTabContextMenu(x,y)",
    "Re-open closed tab",
    "New terminal",
    "Quick Open…  Ctrl+P",
    "Search in Files…  Ctrl+Shift+F",
    "event.preventDefault();event.stopPropagation();",
    "openEmptyTabContextMenu(event.clientX,event.clientY)",
    "const tabClosePending=new WeakMap()",
    "const recentlyClosedTabs=[]",
    "for(const node of record.removedNodes)",
    "node.isConnected",
    "record.attributeName==='hidden'",
    "function reopenClosedTab()",
    "case 'session':",
    "app.reopenHiddenSession?.(entry.id)",
    "case 'editor':",
    "TaskMenuFileCompare?.restoreState?.(entry.state)",
    "TaskMenuDirectoryCompare?.restoreState?.(entry.state)",
  )
}

func TestReadOnlyTabLockDoesNotCollideWithTypeStripe(t *testing.T) {
  requireTabFeatureStrings(t,"tabcontext.js",
    "lock.className='taskdeck-readonly-lock'",
    "lock.textContent='🔒'",
    "tab.prepend(lock)",
    "lock.hidden=!enabled",
    "applyDescriptorReadOnly(descriptor,descriptorReadOnly(descriptor))",
  )
  requireTabFeatureStrings(t,"tabcolors.js",
    "#tabs>[data-taskdeck-tab-type]::before")
  data,err:=webassets.Files.ReadFile("featuremods/tabcontext.js")
  if err!=nil {t.Fatal(err)}
  if strings.Contains(string(data),".taskdeck-tab-readonly::before{") {
    t.Fatal("readonly icon must no longer compete with tab type ::before")
  }
}

func TestSplitTabColorsDistinguishActivePane(t *testing.T) {
  requireTabFeatureStrings(t,"split.js",
    "function updateSplitGroupTabStyles()",
    "for(const id of leafIDs(root,[]))memberColors.set(id,color)",
    "tab.classList.toggle('taskdeck-split-group',Boolean(color))",
    "tab.style.setProperty('--taskdeck-split-accent',color)",
    "tab.style.removeProperty('--taskdeck-split-accent')",
    "#tabs>.tab.taskdeck-split-group.active",
    "font-weight:800!important",
    "box-shadow:0 0 0 2px",
  )
}

func TestPristineLocalTerminalIsRemovedButUsedSessionIsPreserved(t *testing.T) {
  data,err:=os.ReadFile("ui.go")
  if err!=nil {t.Fatal(err)}
  js:=string(data)
  for _,want:=range []string{
    "function markFreshTerminal(meta)",
    "markFreshTerminal(meta);",
    "function markTerminalUsed(id)",
    "function isPristineLocalTerminal(view)",
    "event.detail?.type==='execution-started'",
    "if(view.pendingTerminalInput||input.replace(",
    "if(view.canControl&&isPristineLocalTerminal(view)&&view.meta.status==='running')",
    "await jsonFetch('/api/sessions/'+encodeURIComponent(id)+'/stop',{method:'POST'})",
    "await jsonFetch('/api/sessions/'+encodeURIComponent(id),{method:'DELETE'})",
    "else if(view.canControl&&view.meta.status!=='running')",
    "hidden.add(id);",
    "teardownView(id);",
    "async function reopenHiddenSession(id)",
    "hidden.delete(id);",
    "attach(meta,true);",
  }{
    if !strings.Contains(js,want){t.Errorf("terminal close lifecycle missing %q",want)}
  }
  if !strings.Contains(js,"if(!exited)throw new Error(") {t.Error("must wait for terminal to exit before deleting")}
}
