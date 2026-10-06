package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestSplitTerminalFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/split.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"let roots=[]",
		"const splitSupported=app.layoutProfile!=='mobile'",
		"if(!splitSupported||!view?.pane||installed.has(view))return",
		"function clearPresentation()",
		"isSupported:()=>splitSupported",
		"'vscode-tasks-menu:split:'+(app.layoutProfile||'desktop')",
		"split-tree-mode",
		"split-tree-resizer",
		"session-split-vertical",
		"session-split-horizontal",
		"session-merge-vertical",
		"session-merge-horizontal",
		"session-swap-split",
		"session-unsplit",
		"function splitLeaf(firstID,secondID,orientation",
		"function encodeNode(node,out)",
		"function layoutNode(node,rect,used)",
		"function mergeWith(view,orientation)",
		"function swapSplitView(view)",
		"const first=parent.first;parent.first=parent.second;parent.second=first",
		"function syncForActive()",
		"await app.startTerminal()",
		"pointermove",
		"sessionStorage.setItem",
		"MutationObserver",
		"function restoreProjectGroups",
		"getGroups",
		"taskmenu:split-changed",
		"orientation==='horizontal'",
		"function bindFocusTracking(view)",
		"split-input-focused",
		"let focusTrackingSuspendDepth=0",
		"function suspendFocusTracking()",
		"function resumeFocusTracking(preferredID='')",
		"if(focusTrackingSuspendDepth>0||!id)return",
		"markFocused(view.meta.id)",
		"suspendFocusTracking,resumeFocusTracking",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("split.js missing behavior %q", want)
		}
	}
	if strings.Contains(js, "let pair=null") || strings.Contains(js, "let groups=[]") {
		t.Fatal("split implementation must use a recursive split tree, not a global pair/group list")
	}
	if strings.Contains(js, "else clearSplit();") || strings.Contains(js, "else clearAll();") {
		t.Fatal("activating an unrelated task/tab must suspend split presentation, not destroy saved split trees")
	}
	if strings.Contains(js, "observe(document.body") {
		t.Fatal("split feature must not observe the whole document body")
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/split.js';") {
		t.Fatal("next.js must load split.js")
	}
}

func TestSplitTerminalSupportsRecursiveNestedLayouts(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/split.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"replaceNode(first,node)",
		"splitLeaf(view.meta.id,created,orientation,0.5)",
		"layoutNode(node.first",
		"layoutNode(node.second",
		"const resizers=new Map()",
		"ensureResizer(node)",
		"roots.push(node)",
		"encodeNode(node.first,out);encodeNode(node.second,out)",
		"version:3,groups",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("recursive terminal split behavior missing %q", want)
		}
	}
}


func TestSplitSuspendsForExternalEditorViewWithoutDestroyingGroups(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/split.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"window.addEventListener('taskmenu:view-activated'",
		"event.detail?.kind==='external'",
		"cleanupPresentation();updateButtons();return",
		"event.detail?.kind==='terminal'",
		"setTimeout(syncForActive,0)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("split external-view integration missing %q", want)
		}
	}
}


func TestSplitTerminalSupportsTerminatorStyleTitleDragRearrangement(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/split.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"split-drag-handle",
		"handle.draggable=true",
		"Drag this terminal title to rearrange split panes",
		"panes.addEventListener('dragover'",
		"panes.addEventListener('drop'",
		"function dropSideFor(view,event)",
		"showDropOverlay(target,side)",
		"['left','right','top','bottom'].includes(side)",
		"function moveTerminalToSide(sourceID,targetID,side)",
		"extractLeaf(sourceRoot,sourceID)",
		"?splitNode(removed,target,orientation,0.5)",
		":splitNode(target,removed,orientation,0.5)",
		"saveState();app.activateView(sourceID)",
		"moveTerminalToSide,moveToSplitGroup,splitGroupTargetsFor,clearSplit",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("split title drag/drop behavior missing %q", want)
		}
	}
	if strings.Contains(js, "Sortable.create") || strings.Contains(js, "new Sortable") {
		t.Fatal("split pane drag/drop must stay dependency-free and must not require SortableJS")
	}
}

func TestSplitTerminalLeavesTopLeftForComputedPaneCoordinates(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/split.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"#panes.split-tree-mode>.pane.split-leaf{position:absolute!important;right:auto!important;bottom:auto!important;",
		"view.pane.style.left=Math.round(rect.x)+'px'",
		"view.pane.style.top=Math.round(rect.y)+'px'",
		"view.pane.style.width=Math.max(0,Math.round(rect.w))+'px'",
		"view.pane.style.height=Math.max(0,Math.round(rect.h))+'px'",
		"layoutNode(node.second,{x:rect.x,y:rect.y+firstH+gap,w:rect.w,h:secondH},used)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("split pane positioning missing %q", want)
		}
	}
	if strings.Contains(js, "inset:auto!important") {
		t.Fatal("split leaf must not force top/left to auto because JavaScript computes pane coordinates")
	}
}

func TestSplitTerminalCanMoveBetweenSplitGroups(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/split.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"session-move-split-group",
		"Move to split group…",
		"function splitGroupTargetsFor(sourceID)",
		"function moveToSplitGroup(view)",
		"roots.filter(root=>root!==sourceRoot)",
		"No other split group is available",
		"Place terminal relative to the selected group anchor: left, right, top, or bottom",
		"moveTerminalToSide(view.meta.id,candidates[index].id,side)",
		"saveState();app.activateView(view.meta.id)",
		"const sourceRoot=findRoot(sourceID),targetRoot=findRoot(targetID)",
		"if(!targetRoot)return false",
		"let removed=leaf(sourceID),extractedNode=null",
		"if(sourceRoot){",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("move terminal split group support missing %q", want)
		}
	}

	menus, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(menus), "'.session-move-split-group'") {
		t.Fatal("Session Split menu must expose Move to split group")
	}
}
