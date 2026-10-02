package server

import (
	"strings"
	"testing"
)

func TestCoreSupportsGuardedExternalViewActivationWithoutSessionRegistration(t *testing.T) {
	for _, want := range []string{
		"let foregroundViewLock=''",
		"function claimForegroundView(target)",
		"function releaseForegroundView(target='')",
		"function activationAllowed(target,force=false)",
		"function activateView(id,{focus=true,force=false}={})",
		"function activateExternalView(token,{force=false}={})",
		"if(!activationAllowed(target,force))return false",
		"v.tab.classList.remove('active')",
		"v.pane.classList.add('hidden')",
		"taskmenu:view-activated",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing shared foreground activation contract %q", want)
		}
	}
	for _, exported := range []string{"activateView,", "activateExternalView,", "claimForegroundView,", "releaseForegroundView,"} {
		if !strings.Contains(appJS, exported) {
			t.Fatalf("appJS missing exported foreground activation helper %q", exported)
		}
	}
}

func TestForegroundLockProtectsAllAutomaticTerminalAndTaskLaunches(t *testing.T) {
	for _, want := range []string{
		"hidden.delete(meta.id);attach(meta,true)",
		"tab.onclick=()=>activateView(meta.id,{force:true})",
		"if(!foregroundViewLock||foregroundViewLock===target)return true",
		"if(!force)return false",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing shared foreground protection %q", want)
		}
	}
	if strings.Contains(appJS, "keepPatchVisible") {
		t.Fatal("Patch foreground protection must be centralized, not special-cased per task launcher")
	}
}


