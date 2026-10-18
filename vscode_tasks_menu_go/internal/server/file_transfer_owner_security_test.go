package server

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedTransferQueueSnapshotsHideOtherUsersAndLegacyJobs(t *testing.T) {
 cfg:=config.Default()
 cfg.SharedServerEnabled=true
 s:=&Server{Config:cfg}
 jobs:=[]*fileTransferServerJob{
  {ID:"own",OwnerProjectID:"project-1",OwnerUserID:"alice",Status:"running"},
  {ID:"other",OwnerProjectID:"project-1",OwnerUserID:"bob",Status:"failed",Error:"PRIVATE_BOB_ERROR"},
  {ID:"another-project",OwnerProjectID:"project-2",OwnerUserID:"alice",Status:"running"},
  {ID:"legacy",Status:"failed",Error:"LEGACY_PRIVATE_ERROR"},
 }
 snapshot:=fileTransferJobsSnapshot{
  ProfileID:"shared-sftp",Jobs:jobs,
  Items:[]*fileTransferServerItem{
   {ID:"own-item",JobID:"own",Source:"alice-file"},
   {ID:"other-item",JobID:"other",Source:"PRIVATE_BOB_PATH"},
   {ID:"project-item",JobID:"another-project",Source:"PRIVATE_OTHER_PROJECT_PATH"},
   {ID:"legacy-item",JobID:"legacy",Source:"PRIVATE_LEGACY_PATH"},
  },
 }
 request:=func(principal *identity.Principal)*http.Request{
  req:=httptest.NewRequest(http.MethodGet,"/api/file-transfer/jobs?profile_id=shared-sftp",nil)
  if principal!=nil{req=req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},*principal))}
  return req
 }
 alice:=identity.Principal{ProjectID:"project-1",UserID:"alice",
  Permissions:map[string]bool{identity.PermissionTransferRead:true}}
 filtered:=s.fileTransferSnapshotForRequest(request(&alice),snapshot)
 if len(filtered.Jobs)!=1||filtered.Jobs[0].ID!="own"||
  len(filtered.Items)!=1||filtered.Items[0].ID!="own-item"{
  t.Fatalf("other users leaked into queue listing: %+v",filtered)
 }
 encoded,err:=json.Marshal(filtered)
 if err!=nil{t.Fatal(err)}
 for _,forbidden:=range []string{
  "PRIVATE_BOB_ERROR","PRIVATE_BOB_PATH","PRIVATE_OTHER_PROJECT_PATH","PRIVATE_LEGACY_PATH",
  "OwnerUserID","OwnerProjectID","owner_user_id","owner_project_id",
 }{
  if strings.Contains(string(encoded),forbidden){t.Fatalf("queue response leaked %q: %s",forbidden,encoded)}
 }
 if anonymous:=s.fileTransferSnapshotForRequest(request(nil),snapshot);len(anonymous.Jobs)!=0||len(anonymous.Items)!=0{
  t.Fatalf("anonymous queue listing not redacted: %+v",anonymous)
 }
 admin:=identity.Principal{ProjectID:"project-1",UserID:"administrator",
  Permissions:map[string]bool{identity.PermissionProjectAdmin:true}}
 all:=s.fileTransferSnapshotForRequest(request(&admin),snapshot)
 if len(all.Jobs)!=len(jobs)||len(all.Items)!=len(snapshot.Items){
  t.Fatalf("project admin lost queue oversight: %+v",all)
 }
 cfg.SharedServerEnabled=false
 s.Config=cfg
 unshared:=s.fileTransferSnapshotForRequest(request(nil),snapshot)
 if len(unshared.Jobs)!=len(jobs){t.Fatal("single-user queue listing should remain unchanged")}
}


// A queue restart must not erase ownership and fall back to exposing items
// to arbitrary transfer.read users. Persist owner only in server-owned state.
func TestTransferJobOwnerPersistsOnlyInInternalStateFile(t *testing.T) {
 queue:=&fileTransferServerQueue{
  ProfileID:"private-queue",
  Jobs:map[string]*fileTransferServerJob{
   "job-1":{ID:"job-1",ProfileID:"private-queue",Kind:fileTransferJobHostDownload,
    Status:"success",ScanDone:true,OwnerProjectID:"p",OwnerUserID:"alice"},
  },
  Items:[]*fileTransferServerItem{
   {ID:"item-1",JobID:"job-1",Source:"remote.txt",Status:"success"},
  },
 }
 s:=&Server{
  Workspace:t.TempDir(),fileTransferJobsLoaded:true,
  fileTransferJobQueues:map[string]*fileTransferServerQueue{"private-queue":queue},
 }
 if err:=s.persistFileTransferQueues();err!=nil{t.Fatal(err)}
 state,err:=s.readFileTransferQueueState()
 if err!=nil{t.Fatal(err)}
 if len(state.Queues)!=1||len(state.Queues[0].Jobs)!=1{
  t.Fatalf("queue owner state missing: %+v",state)
 }
 entry:=state.Queues[0].Jobs[0]
 if entry.OwnerProjectID!="p"||entry.OwnerUserID!="alice"{
  t.Fatalf("persisted ownership lost: %+v",entry)
 }
 // The old disk schema has neither field. It remains compatible and such
 // jobs are excluded from the non-admin HTTP snapshots.
 legacy,err:=json.Marshal(entry.Job)
 if err!=nil{t.Fatal(err)}
 if strings.Contains(string(legacy),"owner_")||strings.Contains(string(legacy),"alice"){
  t.Fatalf("owner leaked in public job JSON: %s",legacy)
 }
}
