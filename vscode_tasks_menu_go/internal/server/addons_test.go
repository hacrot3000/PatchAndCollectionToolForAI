package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func addonFixtureExecutable(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX add-on fixture")
	}
	path := filepath.Join(t.TempDir(), "taskdeck-addon")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAddonManifestRequiresPrivateExecutableAndKnownPermissions(t *testing.T) {
	command := addonFixtureExecutable(t, "cat >/dev/null; printf '{\"ok\":true}\\n'")
	manifest, err := normalizeAddonManifest(addonManifest{
		Version: addonManifestVersion,
		ID: "fixture",
		Name: "Fixture",
		Command: command,
		Actions: []addonAction{{
			ID: "inspect", Label: "Inspect", Permissions: []string{identity.PermissionFilesRead},
		}},
		Panels: []addonPanel{{ID:"panel",Title:"Fixture panel",ActionID:"inspect"}},
		ContextMenus: []addonContextMenu{{ID:"menu",Label:"Inspect file",ActionID:"inspect",Scopes:[]string{"file"},Extensions:[]string{"txt"}}},
		FilePreviews: []addonFilePreview{{ID:"preview",Label:"Preview fixture",ActionID:"inspect",Extensions:[]string{".txt"}}},
	})
	if err != nil { t.Fatal(err) }
	if manifest.ContextMenus[0].Extensions[0] != ".txt" {
		t.Fatalf("normalized extensions=%v", manifest.ContextMenus[0].Extensions)
	}
	if manifest.Command != command {
		t.Fatalf("command=%q want %q",manifest.Command,command)
	}

	manifest.Actions[0].Permissions=[]string{"unknown.permission"}
	if _,err:=normalizeAddonManifest(manifest);err==nil||!strings.Contains(err.Error(),"unknown permission"){
		t.Fatalf("unknown permission error=%v",err)
	}
	manifest.Actions[0].Permissions=[]string{identity.PermissionFilesRead}
	manifest.Command="relative-addon"
	if _,err:=normalizeAddonManifest(manifest);err==nil||!strings.Contains(err.Error(),"absolute path"){
		t.Fatalf("relative command error=%v",err)
	}
}

func TestAddonPublicManifestNeverExposesCommand(t *testing.T) {
	command := addonFixtureExecutable(t, "cat >/dev/null; printf '{\"ok\":true}\\n'")
	manifest := addonManifest{Version:1,ID:"fixture",Name:"Fixture",Command:command,Args:[]string{"--secret-path"},Actions:[]addonAction{{ID:"run",Label:"Run"}}}
	public := addonPublicManifest(manifest)
	if public.Command != "" || len(public.Args) != 0 {
		t.Fatalf("public add-on leaked executable details: %#v",public)
	}
}

func TestAddonProcessJSONProtocol(t *testing.T) {
	command := addonFixtureExecutable(t, "read request; printf '{\"ok\":true,\"message\":\"done\",\"content\":\"fixture output\",\"data\":{\"value\":7}}\\n'")
	manifest,err:=normalizeAddonManifest(addonManifest{
		Version:1,ID:"fixture",Name:"Fixture",Command:command,
		Actions:[]addonAction{{ID:"run",Label:"Run"}},
	})
	if err!=nil{t.Fatal(err)}
	response,err:=runAddonProcess(context.Background(),manifest,addonProcessRequest{
		Version:1,AddonID:"fixture",ActionID:"run",Workspace:"/tmp/work",Context:map[string]any{"path":"README.md"},
	})
	if err!=nil{t.Fatal(err)}
	if !response.OK||response.Message!="done"||response.Content!="fixture output"||response.Data["value"]!=float64(7){
		t.Fatalf("response=%#v",response)
	}
}

func TestAddonManifestExtensionPointsReferenceActions(t *testing.T) {
	command := addonFixtureExecutable(t, "cat >/dev/null; printf '{\"ok\":true}\\n'")
	base:=addonManifest{Version:1,ID:"fixture",Name:"Fixture",Command:command,Actions:[]addonAction{{ID:"run",Label:"Run"}}}
	base.Panels=[]addonPanel{{ID:"panel",Title:"Panel",ActionID:"missing"}}
	if _,err:=normalizeAddonManifest(base);err==nil||!strings.Contains(err.Error(),"invalid action"){
		t.Fatalf("missing action ref error=%v",err)
	}
}


func TestAddonSharedPermissionsUseRouteGateAndDeclaredActionPermissions(t *testing.T) {
	listReq:=httptest.NewRequest(http.MethodGet,"/api/addons",nil)
	listPermissions:=sharedRoutePermissions(listReq)
	if len(listPermissions)!=1||listPermissions[0]!=identity.PermissionSettingsRead {
		t.Fatalf("add-on list permissions=%v",listPermissions)
	}
	invokeReq:=httptest.NewRequest(http.MethodPost,"/api/addons/fixture",nil)
	invokePermissions:=sharedRoutePermissions(invokeReq)
	if len(invokePermissions)!=1||invokePermissions[0]!=identity.PermissionSettingsRead {
		t.Fatalf("add-on invoke route permissions=%v",invokePermissions)
	}
	action:=addonAction{ID:"inspect",Label:"Inspect",Permissions:[]string{identity.PermissionFilesRead}}
	denied:=identity.Principal{Permissions:map[string]bool{identity.PermissionSettingsRead:true}}
	invokeReq=invokeReq.WithContext(context.WithValue(invokeReq.Context(),sharedPrincipalContextKey{},denied))
	if (&Server{}).addonAllowed(invokeReq,action) {
		t.Fatal("declared add-on permission was not enforced")
	}
	allowed:=identity.Principal{Permissions:map[string]bool{identity.PermissionSettingsRead:true,identity.PermissionFilesRead:true}}
	invokeReq=invokeReq.WithContext(context.WithValue(invokeReq.Context(),sharedPrincipalContextKey{},allowed))
	if !(&Server{}).addonAllowed(invokeReq,action) {
		t.Fatal("declared add-on permission unexpectedly denied")
	}
}
