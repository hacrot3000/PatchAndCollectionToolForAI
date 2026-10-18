package server

import (
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

// The daemon handles job I/O with host privileges. In shared mode neither
// remote transfer permissions nor read-only queue access authorize host file
// reads, host writes or control of another user's queued operations.
func TestSharedFileTransferJobsRequireHostCapabilities(t *testing.T) {
 s:=&Server{}
 cases:=[]struct{
  name,route,body string
  grants []string
 }{
  {"upload cannot read host without files.read","/api/file-transfer/jobs",`{"profile_id":"p","kind":"host_upload","host_paths":["private.go"]}`,
   []string{identity.PermissionTransferRead,identity.PermissionTransferUpload}},
  {"archive upload cannot read host without files.read","/api/file-transfer/jobs",`{"profile_id":"p","kind":"host_archive_upload","host_paths":["private.go"]}`,
   []string{identity.PermissionTransferRead,identity.PermissionTransferUpload}},
  {"download cannot write host without files.write","/api/file-transfer/jobs",`{"profile_id":"p","kind":"host_download","remote_targets":[{"path":"remote.log"}]}`,
   []string{identity.PermissionTransferRead}},
  {"global queue controls not granted by transfer.read","/api/file-transfer/jobs/control",`{"profile_id":"p","action":"pause"}`,
   []string{identity.PermissionTransferRead}},
  {"global queue controls not granted by transfer.upload","/api/file-transfer/jobs/control",`{"profile_id":"p","action":"clear_queue"}`,
   []string{identity.PermissionTransferRead,identity.PermissionTransferUpload,identity.PermissionTransferDelete,
    identity.PermissionFilesWrite,identity.PermissionFilesRead}},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   perms:=map[string]bool{}
   for _,grant:=range tc.grants{perms[grant]=true}
   req:=httptest.NewRequest(http.MethodPost,tc.route,strings.NewReader(tc.body))
   req.Header.Set("Content-Type","application/json")
   req=req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},
    identity.Principal{Permissions:perms,UserID:"limited",ProjectID:"project"}))
   rec:=httptest.NewRecorder()
   if strings.HasSuffix(tc.route,"/control"){s.fileTransferJobsControl(rec,req)}else{s.fileTransferJobs(rec,req)}
   if rec.Code!=http.StatusForbidden{
    t.Fatalf("delegated transfer capability unexpectedly succeeded: status=%d body=%s",rec.Code,rec.Body)
   }
  })
 }
}
