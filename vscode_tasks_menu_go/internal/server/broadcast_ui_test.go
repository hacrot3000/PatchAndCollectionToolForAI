package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestBroadcastFeatureModule(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/broadcast.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Broadcast: '+label+' ▾'",
		"host.prepend(headerMenu)",
		"for(const mode of ['none','all','group'])",
		"term.onData(data=>queueBroadcast(view,data))",
		"action:'input',source_id:view.meta.id,data",
		"state.mode==='none'",
		"state.mode==='group'&&!state.assignments?.[view.meta.id]",
		"inputQueues.get(view.meta.id)||Promise.resolve()",
		"contextActions(view)",
		"Create new group",
		"Remove from group",
		"broadcast-grouped",
		"--broadcast-tab-bg",
		"--broadcast-tab-fg",
		"Create broadcast group",
		"Tab color preset",
		"delete_group",
		"update_group",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("broadcast.js missing %q", want)
		}
	}
	for _, preset := range []string{
		"slate:", "ocean:", "forest:", "amber:",
		"violet:", "rose:", "cyan:", "lime:",
	} {
		if !strings.Contains(js, preset) {
			t.Fatalf("broadcast.js missing preset %q", preset)
		}
	}
	if strings.Contains(js, "MutationObserver") {
		t.Fatal("broadcast feature must not add a global DOM MutationObserver")
	}
}

func TestTabContextIncludesBroadcastGroupActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabcontext.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"TaskMenuBroadcast?.contextActions?.(view)",
		"addCustomActions('Broadcast group',broadcastActions)",
		"Promise.resolve(action.run?.(targetView)).catch(app.showError)",
		"action.preset",
		"action.danger",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabcontext.js missing broadcast context integration %q", want)
		}
	}
}

func TestBroadcastLoadsBetweenMenusAndTabContext(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	next := string(data)
	menus := strings.Index(next, "import '/featuremods/menus.js';")
	broadcast := strings.Index(next, "import '/featuremods/broadcast.js';")
	context := strings.Index(next, "import '/featuremods/tabcontext.js';")
	if menus < 0 || broadcast < 0 || context < 0 {
		t.Fatalf("missing loader entries: menus=%d broadcast=%d context=%d", menus, broadcast, context)
	}
	if !(menus < broadcast && broadcast < context) {
		t.Fatalf("broadcast must load after menus and before tab context: menus=%d broadcast=%d context=%d", menus, broadcast, context)
	}
}
