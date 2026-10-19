package server

import (
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

// project.admin is scoped to the requesting project, not every transfer
// started on a process-global saved-profile queue.  This test checks both
// global queue controls and the special conflict-resolution lock path.
func TestSharedTransferQueueControlRejectsForeignAndOwnerlessJobs(t *testing.T) {
 cfg:=config.Default()
 cfg.SharedServerEnabled=true
 q:=&fileTransferServerQueue{
  ProfileID:"shared-profile",Jobs:map[string]*fileTransferServerJob{
   "mine":{ID:"mine",OwnerProjectID:"project-a",OwnerUserID:"alice",Status:"queued"},
   "foreign":{ID:"foreign",OwnerProjectID:"project-b",OwnerUserID:"bob",Status:"queued"},
  },
  Items:[]*fileTransferServerItem{
   {ID:"item-a",JobID:"mine",Status:"queued"},
   {ID:"item-b",JobID:"foreign",Status:"queued"},
  },
 }
 s:=&Server{Config:cfg,Workspace:t.TempDir(),fileTransferJobsLoaded:true,
  fileTransferJobQueues:map[string]*fileTransferServerQueue{"shared-profile":q}}
 action:=func(body string)*httptest.ResponseRecorder{
  t.Helper()
  req:=httptest.NewRequest(http.MethodPost,"/api/file-transfer/jobs/control",strings.NewReader(body))
  req.Header.Set("Content-Type","application/json")
  principal:=identity.Principal{ProjectID:"project-a",UserID:"alice",
   Permissions:map[string]bool{identity.PermissionProjectAdmin:true}}
  req=req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},principal))
  rec:=httptest.NewRecorder()
  s.fileTransferJobsControl(rec,req)
  return rec
 }
 for _,body:=range []string{
  `{"profile_id":"shared-profile","action":"pause"}`,
  `{"profile_id":"shared-profile","action":"clear_queue"}`,
  `{"profile_id":"shared-profile","action":"resolve_conflict","conflict_policy":"skip","conflict_scope":"job","job_id":"mine"}`,
 } {
  got:=action(body)
  if got.Code!=http.StatusForbidden{t.Fatalf("cross-project control %s returned %d: %s",body,got.Code,got.Body)}
  if q.Paused||len(q.Items)!=2{t.Fatal("cross-project control mutated global queue")}
 }
 q.mu.Lock()
 delete(q.Jobs,"foreign")
 q.Items=q.Items[:1]
 q.Jobs["mine"].Status="queued"
 q.mu.Unlock()
 allowed:=action(`{"profile_id":"shared-profile","action":"pause"}`)
 if allowed.Code!=http.StatusOK||!q.Paused{
  t.Fatalf("same-project administrator cannot pause own queue: %d %s",allowed.Code,allowed.Body)
 }
 q.mu.Lock()
 q.Paused=false
 q.Jobs["legacy"]=&fileTransferServerJob{ID:"legacy",Status:"queued"}
 q.mu.Unlock()
 legacy:=action(`{"profile_id":"shared-profile","action":"pause"}`)
 if legacy.Code!=http.StatusForbidden||q.Paused {
  t.Fatalf("legacy unowned queue mutated: %d %s",legacy.Code,legacy.Body)
 }
}
