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
		"activateView,activateExternalView",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing external-view hook %q", want)
		}
	}
}
