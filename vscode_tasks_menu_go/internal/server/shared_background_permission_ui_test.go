package server

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/identity"
    webassets "bletonfc/vscode_tasks_menu/web"
)

// Every feature module imported during initial page load must honor the same
// shared-server permissions enforced by the HTTP router. These regressions
// prevent background 403 storms without granting users additional privileges.
func TestSharedBackgroundModulesGateTheirStartupRequests(t *testing.T) {
    scenarios:=[]struct{
        path string
        requires []string
    }{
        {"filewatcher.js",[]string{
            "function canWatchFiles()","app.hasPermission?.('files.read')",
            "if(!canWatchFiles()||fileTimer||directoryTimer)return;",
            "if(permissionDenied(error)){stop();return;}",
            "for(const path of paths){if(!enabled)return;await pollDirectory(path);}",
        }},
        {"commandpresets.js",[]string{
            "function canReadPresets()","app.hasPermission?.('settings.read')",
            "if(!canReadPresets())return state;",
            "if(refreshTimer)clearInterval(refreshTimer)",
            "if(canReadPresets())refreshTimer=setInterval(",
        }},
        {"connections.js",[]string{
            "function loadAuthorizedConnectionSource(permission,path)",
            "if(app.sharedMode&&!app.hasPermission?.(permission))return null;",
            "deniedBackgroundSources.add(path)",
            "loadAuthorizedConnectionSource('settings.read','/api/config/terminal-cwds')",
            "loadAuthorizedConnectionSource('ssh.use','/api/ssh/profiles')",
            "loadAuthorizedConnectionSource('db.read','/api/db/adapters')",
            "loadAuthorizedConnectionSource('db.read','/api/db/profiles')",
            "loadAuthorizedConnectionSource('transfer.read','/api/file-transfer/profiles')",
        }},
        {"addons.js",[]string{"if(app.sharedMode&&!app.hasPermission?.('settings.read'))return [];"}},
        {"database_workbench.js",[]string{"if(app.sharedMode&&!app.hasPermission?.('db.read'))return Promise.resolve(adaptersByID);"}},
        {"database.js",[]string{
            "if(app.sharedMode&&!app.hasPermission?.('db.read')){profilesByID=new Map();return profilesByID;}",
            "if(app.sharedMode&&!app.hasPermission?.('db.read'))return;",
        }},
        {"editor.js",[]string{
            "if(app.sharedMode&&!app.hasPermission?.('files.read'))return false;",
            "if(app.sharedMode&&!app.hasPermission?.('files.write'))return false;",
            "if(app.sharedMode&&!app.hasPermission?.('files.write'))return;",
        }},
        {"runningindicator.js",[]string{"app.sharedMode&&!app.hasPermission?.('settings.read')"}},
        {"selfupdatesettings.js",[]string{"app.sharedMode&&!app.hasPermission?.('settings.read')"}},
        {"terminalcwd.js",[]string{
            "app.sharedMode&&!app.hasPermission?.('settings.read')",
            "if(app.sharedMode&&!app.hasPermission?.('settings.write'))throw new Error",
        }},
        {"pagetitle.js",[]string{
            "app.sharedMode&&!app.hasPermission?.('settings.read')",
            "if(app.sharedMode&&!app.hasPermission?.('settings.write'))throw new Error",
        }},
    }
    for _,scenario:=range scenarios {
        data,err:=webassets.Files.ReadFile("featuremods/"+scenario.path)
        if err!=nil{t.Fatal(err)}
        code:=string(data)
        for _,expected:=range scenario.requires {
            if !strings.Contains(code,expected) {
                t.Errorf("%s missing startup permission guard %q",scenario.path,expected)
            }
        }
    }
}

// Never fix a forbidden background request by weakening server authorization.
// The browser must refrain from issuing it, while the backend remains the
// authoritative permission boundary for every API request.
func TestSharedBackgroundAPIEndpointsRemainPermissionProtected(t *testing.T) {
    cases:=[]struct{path, permission string}{
        {"/api/addons",identity.PermissionSettingsRead},
        {"/api/db/adapters",identity.PermissionDBRead},
        {"/api/db/profiles",identity.PermissionDBRead},
        {"/api/project/tree?path=",identity.PermissionFilesRead},
        {"/api/command-presets",identity.PermissionSettingsRead},
        {"/api/project/editor-session",identity.PermissionFilesRead},
        {"/api/config/running-indicator",identity.PermissionSettingsRead},
        {"/api/file-transfer/profiles",identity.PermissionTransferRead},
        {"/api/config/self-update",identity.PermissionSettingsRead},
        {"/api/config/terminal-cwds",identity.PermissionSettingsRead},
        {"/api/ssh/profiles",identity.PermissionSSHUse},
        {"/api/config/page-title",identity.PermissionSettingsRead},
    }
    for _,tc:=range cases {
        request:=httptest.NewRequest(http.MethodGet,tc.path,nil)
        actual:=sharedRoutePermissions(request)
        if len(actual)!=1||actual[0]!=tc.permission {
            t.Errorf("%s permission changed to %v, expected %q",tc.path,actual,tc.permission)
        }
    }
}
