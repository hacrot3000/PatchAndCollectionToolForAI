package server

import (
	"strings"
	"testing"
)

func TestCoreSupportsExternalViewActivationWithoutSessionRegistration(t *testing.T) {
	for _, want := range []string{
		"function activateExternalView(token)",
		"active='external:'+String(token||'view')",
		"v.tab.classList.remove('active')",
		"v.pane.classList.add('hidden')",
		"taskmenu:view-activated",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing external-view hook %q", want)
		}
	}
	for _, exported := range []string{"activateView,", "activateExternalView,"} {
		if !strings.Contains(appJS, exported) {
			t.Fatalf("appJS missing exported view activation helper %q", exported)
		}
	}
}

func TestTaskLaunchDoesNotStealPatchWorkspaceFocus(t *testing.T) {
	for _, want := range []string{
		"const keepPatchVisible=String(active||'')==='external:patch'",
		"hidden.delete(meta.id);attach(meta,!keepPatchVisible)",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing Patch-preserving task launch contract %q", want)
		}
	}
}


