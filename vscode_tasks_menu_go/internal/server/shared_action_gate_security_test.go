package server

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/config"
    "bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedActionGateNeverTreatsMissingPrincipalAsLocalMode(t *testing.T) {
    cfg:=config.Default()
    cfg.SharedServerEnabled=true
    s:=&Server{Config:cfg}
    req:=httptest.NewRequest(http.MethodPost,"/api/file-transfer/jobs",nil)
    rr:=httptest.NewRecorder()
    allowed:=s.requireSharedActionPermission(rr,req,identity.PermissionTransferRead,"create","profile")
    if allowed||rr.Code!=http.StatusUnauthorized{
        t.Fatalf("anonymous shared action allowed=%v status=%d: %s",allowed,rr.Code,rr.Body)
    }

    limited:=identity.Principal{UserID:"limited",ProjectID:"project-a",
        Permissions:map[string]bool{identity.PermissionFilesRead:true}}
    req=req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},limited))
    rr=httptest.NewRecorder()
    if s.requireSharedActionPermission(rr,req,identity.PermissionTransferRead,"create","profile")||
        rr.Code!=http.StatusForbidden{
        t.Fatalf("shared action without grant unexpectedly allowed: %d %s",rr.Code,rr.Body)
    }

    limited.Permissions[identity.PermissionTransferRead]=true
    req=req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},limited))
    rr=httptest.NewRecorder()
    if !s.requireSharedActionPermission(rr,req,identity.PermissionTransferRead,"create","profile"){
        t.Fatalf("legitimate shared action denied: %d %s",rr.Code,rr.Body)
    }

    cfg.SharedServerEnabled=false
    s.Config=cfg
    rr=httptest.NewRecorder()
    if !s.requireSharedActionPermission(rr,httptest.NewRequest(http.MethodPost,"/api/file-transfer/jobs",nil),
        identity.PermissionTransferRead,"create","profile"){
        t.Fatal("local single-user action should not require shared login")
    }
}
