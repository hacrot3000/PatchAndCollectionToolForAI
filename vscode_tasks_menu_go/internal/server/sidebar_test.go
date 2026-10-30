package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestResizableSidebarFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/sidebar.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"sidebar-resizer",
		"pointerdown",
		"pointermove",
		"gridTemplateColumns",
		"sidebar-width:",
		"dblclick",
		"localStorage.setItem",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("sidebar.js missing behavior %q", want)
		}
	}
}

func TestSidebarDoesNotInitializeInMobileProfile(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/sidebar.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(data), "if(app.layoutProfile==='mobile'||initialized") {
		t.Fatal("desktop sidebar resize state must not initialize in mobile profile")
	}
}


func TestFloatingSidebarResizeSupportsAllDesktopPanelStates(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/sidebar.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const floatingResizer=document.createElement('div')",
		"floatingResizer.className='task-sidebar-floating-resizer'",
		"floatingResizer.dataset.taskSidebarKeepOpen='1'",
		"body.task-sidebar-auto-hide.task-sidebar-resize-panel-open .task-sidebar-floating-resizer{display:block}",
		"left:calc(48px + var(--taskmenu-sidebar-panel-width,310px) - 5px)",
		"document.body.append(floatingResizer)",
		"function decorateHandle(handle)",
		"decorateHandle(resizer)",
		"decorateHandle(floatingResizer)",
		"handle.setPointerCapture(event.pointerId)",
		"handle.addEventListener('pointercancel',finish)",
		"handle.addEventListener('lostpointercapture',finish)",
		"handle===floatingResizer?48:0",
		"width=apply(event.clientX-rect.left-left)",
		"window.addEventListener('taskmenu:sidebar-layout'",
		"document.documentElement.style.setProperty('--taskmenu-sidebar-panel-width'",
		"main.style.setProperty('--taskmenu-sidebar-panel-width'",
		"globalThis.TaskMenuSidebarResize={",
		"handle.addEventListener('keydown'",
		"event.key==='ArrowLeft'",
		"event.key==='ArrowRight'",
		"event.key==='Home'",
		"event.key==='End'",
		"handle.setAttribute('role','separator')",
		"handle.setAttribute('aria-orientation','vertical')",
		"save(width)",
	} {
		if !strings.Contains(js, want) { t.Errorf("shared sidebar resize missing %q", want) }
	}
	activityData, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil { t.Fatal(err) }
	activityJS := string(activityData)
	for _, want := range []string{
		"['tasks','explorer','connections','history','schedules'].includes(activeView)",
		"document.body.classList.toggle('task-sidebar-resize-panel-open',enabled()&&panelView)",
		"window.dispatchEvent(new CustomEvent('taskmenu:sidebar-layout'",
		"const live=globalThis.TaskMenuSidebarResize?.width",
		"if(Number.isFinite(live)&&live>0)return live",
		"target.closest('[data-task-sidebar-keep-open=\"1\"]')",
	} {
		if !strings.Contains(activityJS, want) { t.Errorf("activity bar must share sidebar width/drag ownership: %q", want) }
	}
}


func TestEveryFloatingLeftPanelConsumesSharedResizeWidth(t *testing.T) {
	cases := []struct {
		file string
		selectors []string
	}{
		{"featuremods/activitybar.js", []string{
			"body.task-sidebar-auto-hide #menu{display:none",
			"width:min(var(--taskmenu-sidebar-panel-width,310px),calc(100vw - 48px))",
			"body.task-sidebar-auto-hide .project-explorer{left:48px!important;width:min(var(--taskmenu-sidebar-panel-width,310px)",
			".task-history-panel{display:none;position:fixed",
		}},
		{"featuremods/connections.js", []string{
			".task-connections-panel{display:none;position:fixed",
			"width:min(var(--taskmenu-sidebar-panel-width,310px),calc(100vw - 48px))",
		}},
		{"featuremods/schedulespanel.js", []string{
			"body.task-sidebar-auto-hide .schedule-nav{left:48px",
			"width:min(var(--taskmenu-sidebar-panel-width,310px),calc(100vw - 48px))",
		}},
	}
	for _, tc := range cases {
		data, err := webassets.Files.ReadFile(tc.file)
		if err != nil { t.Fatal(err) }
		for _, want := range tc.selectors {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s: floating panel not tied to shared sidebar width: missing %q", tc.file, want)
			}
		}
	}
}
