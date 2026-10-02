package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
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


func TestViewActivationEventsDistinguishExplicitTabSwitches(t *testing.T) {
	for _, want := range []string{
		"let trustedTabActivationDepth=0",
		"function trustedWorkspaceTabClick(event)",
		"if(event.isTrusted===false)return",
		"if(!target||target.closest('.close'))return",
		"tabs.addEventListener('click',trustedWorkspaceTabClick,true)",
		"function explicitViewActivation(){return trustedTabActivationDepth>0;}",
		"detail:{kind:'terminal',id,explicit:explicitViewActivation()}",
		"detail:{kind:'external',id:token,explicit:explicitViewActivation()}",
		"tab.onclick=()=>activateView(meta.id,{force:true})",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing trusted-tab explicit activation contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"detail:{kind:'terminal',id,explicit:Boolean(force)}",
		"detail:{kind:'external',id:token,explicit:Boolean(force)}",
	} {
		if strings.Contains(appJS, forbidden) {
			t.Fatalf("force must not be treated as an explicit user tab switch: %q", forbidden)
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





func TestWorkspaceOpenPathsDoNotForcePatchOutOfForeground(t *testing.T) {
	for _, want := range []string{
		"hidden.delete(meta.id);attach(meta,true)",
		"if(view){updateMeta(meta);if(activate)activateView(meta.id);return view;}",
		"connect(view,false);updateMeta(meta);if(activate)activateView(meta.id);return view;",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("core automatic open path missing background activation contract %q", want)
		}
	}

	checks := []struct {
		file       string
		required   []string
		forbidden  []string
	}{
		{
			file: "featuremods/connections.js",
			required: []string{
				"app.materializeSession(meta,true)",
				"await api.openProfile(profile)",
			},
			forbidden: []string{
				"app.materializeSession(meta,true,{force:true})",
			},
		},
		{
			file: "featuremods/database.js",
			required: []string{
				"if(activate)activateDatabaseView(meta.id)",
				"return attachDatabaseView(meta,true)",
			},
			forbidden: []string{
				"if(activate)activateDatabaseView(meta.id,{force:true})",
				"return attachDatabaseView(meta,true,{force:true})",
			},
		},
		{
			file: "featuremods/filetransfer.js",
			required: []string{
				"if(activate)activateView(id)",
				"return attachView(profile)",
			},
			forbidden: []string{
				"if(activate)activateView(id,{force:true})",
				"return attachView(profile,{activate:true,force:true})",
			},
		},
		{
			file: "featuremods/editor.js",
			required: []string{
				"const view=await promise;activateEditor(view.id);return view",
			},
			forbidden: []string{
				"const view=await promise;activateEditor(view.id,{force:true});return view",
			},
		},
	}
	for _, check := range checks {
		data, err := webassets.Files.ReadFile(check.file)
		if err != nil {
			t.Fatalf("%s: %v", check.file, err)
		}
		js := string(data)
		for _, want := range check.required {
			if !strings.Contains(js, want) {
				t.Fatalf("%s missing background-open contract %q", check.file, want)
			}
		}
		for _, forbidden := range check.forbidden {
			if strings.Contains(js, forbidden) {
				t.Fatalf("%s must not force foreground during automatic open: %q", check.file, forbidden)
			}
		}
	}
}

func TestPatchForegroundLockCoversAllWorkspaceOpenPaths(t *testing.T) {
	for _, want := range []string{
		"async function startTask(t)",
		"hidden.delete(meta.id);attach(meta,true)",
		"async function startTerminal()",
		"tab.onclick=()=>activateView(meta.id,{force:true})",
		"function activationAllowed(target,force=false)",
		"if(!force)return false",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("core foreground guard missing %q", want)
		}
	}

	checks := []struct {
		file string
		wants []string
	}{
		{
			file: "featuremods/connections.js",
			wants: []string{
				"app.materializeSession(meta,true)",
				"async function openSSH(profile)",
				"await api.openProfile(profile)",
			},
		},
		{
			file: "featuremods/editor.js",
			wants: []string{
				"function activateEditor(id,{force=false}={})",
				"return app.activateExternalView(id,{force})",
				"tab.onclick=()=>activateEditor(id,{force:true})",
			},
		},
		{
			file: "featuremods/database.js",
			wants: []string{
				"function activateDatabaseView(id,{force=false}={})",
				"if(app.activateExternalView('database:'+id,{force})===false)return false",
				"tab.onclick=()=>activateDatabaseView(meta.id,{force:true})",
			},
		},
		{
			file: "featuremods/filetransfer.js",
			wants: []string{
				"function activateView(id,{force=false}={})",
				"if(app.activateExternalView('file-transfer:'+id,{force})===false)return false",
				"tab.onclick=()=>activateView(id,{force:true})",
			},
		},
	}
	for _, check := range checks {
		data, err := webassets.Files.ReadFile(check.file)
		if err != nil {
			t.Fatalf("%s: %v", check.file, err)
		}
		js := string(data)
		for _, want := range check.wants {
			if !strings.Contains(js, want) {
				t.Fatalf("%s missing shared Patch foreground contract %q", check.file, want)
			}
		}
	}
}

