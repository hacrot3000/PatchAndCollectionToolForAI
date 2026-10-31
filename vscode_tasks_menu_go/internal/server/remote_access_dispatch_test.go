package server

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "strconv"
 "sync/atomic"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remoteportal"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

const remoteDispatchTestID = "0123456789abcdef0123456789abcdef"

func remoteDispatchFixture(t *testing.T)(*Server,*remoteTaskDeckSession,*remotetransport.Channel,*remotetransport.Channel) {
 return remoteDispatchFixtureWithRole(t,remotetransport.RoleViewer)
}

func remoteDispatchFixtureWithRole(t *testing.T,role remotetransport.Role)(*Server,*remoteTaskDeckSession,*remotetransport.Channel,*remotetransport.Channel) {
 t.Helper()
 pub,priv,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 fingerprint:=sha256.Sum256(pub)
 device:=hex.EncodeToString(fingerprint[:])
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 hello,agent,err:=remotetransport.ClientHello(priv,device,
  strings.Repeat("a",32),strings.Repeat("b",32),browser.Public)
 if err!=nil{t.Fatal(err)}
 client,err:=remotetransport.AcceptHello(browser,device,
  strings.Repeat("a",32),strings.Repeat("b",32),hello)
 if err!=nil{t.Fatal(err)}
 first,err:=client.Seal([]byte("authenticated-client-possession"))
 if err!=nil{t.Fatal(err)}
 if _,err:=agent.Open(first);err!=nil{t.Fatal(err)}
 s:=sharedLoginTestServer(t)
 s.Config.RemoteAccessEnabled=true
 s.Config.Protocol=config.ProtocolHTTPS
 grant:=testRemoteSignedGrant(t,agent,role)
 session,err:=s.authenticateRemoteTaskDeckCredentials(context.Background(),agent,grant,
  "alice","private-admin-password")
 if err!=nil{t.Fatal(err)}
 bindTestRemoteOwnerClient(t,s,session)
 t.Cleanup(session.Close)
 t.Cleanup(client.Close)
 allowTestLivePortal(session)
 return s,session,client,agent
}

func TestRemoteDispatchEncryptedPingChecksTaskDeckPasswordAndLiveRBAC(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 var checks atomic.Int32
 session.portalCheckMu.Lock()
 session.livePortalCheck=func(context.Context,*remoteportal.AgentGrant)error{
  checks.Add(1)
  return nil
 }
 session.portalCheckMu.Unlock()
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionSessionPing,
 })
 if err!=nil{t.Fatal(err)}
 // PHP ticket with arbitrary admin role cannot substitute a locally
 // verified TaskDeck password session, even with a correctly signed frame.
 if response,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,req);err==nil || response!=nil{
  t.Fatal("Portal-only grant admitted E2EE application operation")
 }
 result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 if checks.Load()!=2{t.Fatalf("Portal was not rechecked on both sides of action: %d",checks.Load())}
 if err:=remotetransport.OpenRemoteActionPingResult(client,result,remoteDispatchTestID);err!=nil{
  t.Fatal(err)
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil{
  t.Fatal("replayed encrypted action admitted through local TaskDeck dispatcher")
 }
 if agent.Established(){t.Fatal("replayed action left authenticated Agent E2EE channel alive")}
}

func TestRemoteDispatchAuditBoundToRealLocalTaskDeckIdentity(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionSessionPing,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 if err:=remotetransport.OpenRemoteActionPingResult(client,reply,remoteDispatchTestID);err!=nil{t.Fatal(err)}
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 readAudit:=func()[]identity.AuditEvent{
  t.Helper()
  rows,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
   ProjectID:project.ID,Action:"remote.action.session_ping",Limit:10,
  })
  if err!=nil{t.Fatal(err)}
  return rows
 }
 rows:=readAudit()
 if len(rows)!=1 || rows[0].UserID==nil || *rows[0].UserID!=session.userID ||
  rows[0].Result!="authorized" ||
  strings.Contains(rows[0].Details,"private-admin-password") ||
  strings.Contains(rows[0].Details,"alice") ||
  strings.Contains(rows[0].Details,remoteDispatchTestID) {
  t.Fatalf("TaskDeck audit missed real user or exposed credentials: %+v",rows)
 }
 // A rejected action after local session Close is never logged as success.
 session.Close()
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil || len(reply)!=0{
  t.Fatal("closed session created another successful response")
 }
 if len(readAudit())!=1{t.Fatal("failed/closed session produced a success audit event")}
}

func TestRemoteDispatchRefusesWrongChannelPasswordResetAndTaskDeckDisable(t *testing.T){
 t.Run("foreign E2EE",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixture(t)
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionSessionPing,
  })
  if err!=nil{t.Fatal(err)}
  other:=remoteSessionTestChannel(t)
  if response,err:=s.executeRemoteE2EEPing(context.Background(),session,other,req);err==nil || len(response)!=0{
   t.Fatal("session transferred to unrelated authenticated E2EE channel")
  }
  if !agent.Established(){t.Fatal("invalid channel attempt destroyed legitimate session")}
 })
 t.Run("disabled TaskDeck account",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixture(t)
  if err:=s.Identity.SetUserEnabled(context.Background(),session.userID,false,time.Now().UTC());err!=nil{
   t.Fatal(err)
  }
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionSessionPing,
  })
  if err!=nil{t.Fatal(err)}
  if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil || len(reply)!=0 {
   t.Fatal("disabled TaskDeck user retained application permission")
  }
 })
 t.Run("PHP grant revoked",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixture(t)
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionSessionPing,
  })
  if err!=nil{t.Fatal(err)}
  session.portalGrant.Close()
  if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil || len(reply)!=0{
   t.Fatal("revoked Portal grant created a TaskDeck ping response")
  }
 })
}

func TestRemoteDispatchRejectsCommandShapedCiphertextAndClosesE2EE(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 // This ciphertext is authenticated by the real browser key, but it
 // requests an unreviewed terminal execution through a forged PHP flow.
 raw:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"terminal.exec"}`)
 req,err:=client.Seal(raw)
 if err!=nil{t.Fatal(err)}
 response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied) || len(response)!=0{
  t.Fatalf("unreviewed command traversed E2EE dispatcher: %v",err)
 }
 if agent.Established(){t.Fatal("malformed action did not revoke Agent encryption keys")}
}

func TestRemoteDispatchLocalCloseDuringPortalCheckCannotSealResponse(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 entered:=make(chan struct{})
 release:=make(chan struct{})
 session.portalCheckMu.Lock()
 session.livePortalCheck=func(context.Context,*remoteportal.AgentGrant)error{
  close(entered)
  <-release
  return nil // compromised Portal returns stale allowed=true
 }
 session.portalCheckMu.Unlock()
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionSessionPing,
 })
 if err!=nil{t.Fatal(err)}
 result:=make(chan error,1)
 go func(){
  reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
  if len(reply)!=0{result<-errors.New("revoked session produced an encrypted success reply");return}
  result<-err
 }()
 select {
 case <-entered:
 case <-time.After(5*time.Second):t.Fatal("live Portal introspection was not started")
 }
 session.Close()
 close(release)
 select {
 case err:=<-result:
  if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
   t.Fatalf("stale Portal answer revived authenticated dispatcher: %v",err)
  }
 case <-time.After(5*time.Second):t.Fatal("revoked dispatch is still blocked")
 }
}

    
func TestRealTaskDeckProjectFileReadRequiresKeyAndFilesReadPermission(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 path:=filepath.Join(s.Workspace,"taskdeck-remote-sample.txt")
 if err:=os.WriteFile(path,[]byte("safe E2EE file"),0600);err!=nil{t.Fatal(err)}
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileRead,
  Path:"taskdeck-remote-sample.txt",
 })
 if err!=nil{t.Fatal(err)}
 if res,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,req);err==nil||res!=nil{
  t.Fatal("PHP-only session obtained workspace file data")
 }
 result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 content,err:=remotetransport.OpenRemoteFileReadResult(client,result,remoteDispatchTestID)
 if err!=nil||content!="safe E2EE file"{
  t.Fatalf("FilesRead E2EE response missing or altered: %q %v",content,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.project_file_read",Limit:5,
 })
 if err!=nil||len(audit)==0||audit[0].UserID==nil||
  *audit[0].UserID!=session.userID{
  t.Fatalf("host file read missed local TaskDeck identity audit: %v",err)
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil{
  t.Fatal("Relay replayed a completed workspace read")
 }
}

func TestRemoteProjectReadFailsClosedForOversizeAndRevokedClientProof(t *testing.T){
 t.Run("oversized file never leaves TaskDeck",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixture(t)
  if err:=os.WriteFile(filepath.Join(s.Workspace,"big-remote.txt"),
   []byte(strings.Repeat("x",193)),0600);err!=nil{t.Fatal(err)}
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileRead,
   Path:"big-remote.txt",
  })
  if err!=nil{t.Fatal(err)}
  response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
  if err==nil||len(response)>0{
   t.Fatal("oversized TaskDeck file escaped E2EE file cap")
  }
 })
 t.Run("no host client key proof",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixture(t)
  session.clientProofVerified=false
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileRead,
   Path:"taskdeck-remote-sample.txt",
  })
  if err!=nil{t.Fatal(err)}
  response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
  if err==nil||len(response)>0||!session.closed.Load(){
   t.Fatal("compromised PHP used password-only session to read workspace file")
  }
 })
}

    
func TestRemoteProjectFileReadRejectsSymlinkEscapingWorkspace(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 outside:=t.TempDir()
 secret:=filepath.Join(outside,"secret.txt")
 if err:=os.WriteFile(secret,[]byte("must never leave host"),0600);err!=nil{t.Fatal(err)}
 link:=filepath.Join(s.Workspace,"out-of-root-link.txt")
 if err:=os.Symlink(secret,link);err!=nil{t.Fatal(err)}
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileRead,
  Path:"out-of-root-link.txt",
 })
 if err!=nil{t.Fatal(err)}
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err==nil||len(sealed)!=0 {
  t.Fatal("symlink escaped the TaskDeck workspace file trust boundary")
 }
}

    
func TestTaskDeckE2EEGitStatusRequiresHostPrincipalAndGitStatusRBAC(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{
  t.Skipf("Git unavailable: %v",err)
 }
 encrypted,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitStatus,
 })
 if err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,encrypted);err==nil||len(reply)>0{
  t.Fatal("Portal-only admin ticket could query TaskDeck Git")
 }
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,encrypted)
 if err!=nil{t.Fatal(err)}
 output,err:=remotetransport.OpenRemoteGitStatusResult(client,sealed,remoteDispatchTestID)
 if err!=nil || output=="" || len(output)>192{
  t.Fatalf("bounded authenticated Git status missing: %q %v",output,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_status",Limit:5,
 })
 if err!=nil||len(audit)==0||audit[0].UserID==nil||
  *audit[0].UserID!=session.userID{
  t.Fatalf("missing TaskDeck-local Git principal audit: %v",err)
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,encrypted);err==nil{
  t.Fatal("Git E2EE request replay admitted")
 }
}
func TestRemoteGitStatusOutputBound(t *testing.T){
 var buf cappedRemoteGitOutput
 if _,err:=buf.Write([]byte(strings.Repeat("a",192)));err!=nil{t.Fatal(err)}
 if _,err:=buf.Write([]byte("b"));err==nil{
  t.Fatal("TaskDeck would buffer unbounded Git data")
 }
}

    
func TestE2EEProjectReplaceChecksRealTaskDeckPrincipalAndOptimisticSHA(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 file:=filepath.Join(s.Workspace,"edit-me.txt")
 original:=[]byte("before")
 if err:=os.WriteFile(file,original,0600);err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256(original)
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,
  Operation:remotetransport.RemoteActionProjectFileReplace,
  Path:"edit-me.txt",Content:"after",ExpectedSHA256:hex.EncodeToString(digest[:]),
 })
 if err!=nil{t.Fatal(err)}
 if result,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,request);err==nil||result!=nil{
  t.Fatal("Portal-only grant wrote a TaskDeck file")
 }
 response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 after,err:=os.ReadFile(file)
 if err!=nil||string(after)!="after"{t.Fatalf("authenticated atomic replacement failed: %q %v",after,err)}
 newSHA:=sha256.Sum256(after)
 verified,err:=remotetransport.OpenRemoteFileReplaceResult(client,response,remoteDispatchTestID)
 if err!=nil||verified!=hex.EncodeToString(newSHA[:]){
  t.Fatalf("TaskDeck replacement ACK not AEAD authenticated: %s %v",verified,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.project_file_replace",Limit:5,
 })
 if err!=nil||len(audits)==0||audits[0].UserID==nil||
  *audits[0].UserID!=session.userID{
  t.Fatalf("host user did not audit committed write: %v",err)
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil{
  t.Fatal("replayed encrypted replace duplicated file write")
 }
}
func TestRemoteProjectReplacementRefusesStaleSHAAndUnapprovedClient(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
 t.Run("stale SHA",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
  path:=filepath.Join(s.Workspace,"existing.txt")
  if err:=os.WriteFile(path,[]byte("actual"),0600);err!=nil{t.Fatal(err)}
  stale:=sha256.Sum256([]byte("old"))
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileReplace,
   Path:"existing.txt",Content:"malicious",ExpectedSHA256:hex.EncodeToString(stale[:]),
  });if err!=nil{t.Fatal(err)}
  if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil||reply!=nil{
   t.Fatal("stale expected SHA overwrote TaskDeck")
  }
  saved,_:=os.ReadFile(path)
  if string(saved)!="actual"{t.Fatal("stale write changed workspace content")}
 })
 t.Run("revoked client proof",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
  path:=filepath.Join(s.Workspace,"existing.txt")
  if err:=os.WriteFile(path,[]byte("actual"),0600);err!=nil{t.Fatal(err)}
  sha:=sha256.Sum256([]byte("actual"))
  session.clientProofVerified=false
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileReplace,
   Path:"existing.txt",Content:"malicious",ExpectedSHA256:hex.EncodeToString(sha[:]),
  });if err!=nil{t.Fatal(err)}
  if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil||reply!=nil{
   t.Fatal("unapproved client modified TaskDeck")
  }
  saved,_:=os.ReadFile(path)
  if string(saved)!="actual"{t.Fatal("revoked client changed workspace content")}
 })
}


func TestE2EERemoteFileReplaceCannotBypassLocalEditorMutationLock(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 file:=filepath.Join(s.Workspace,"mutating-file.txt")
 if err:=os.WriteFile(file,[]byte("original"),0600);err!=nil{t.Fatal(err)}
 before:=sha256.Sum256([]byte("original"))
 // A trusted local editor owns the global mutation lease. A remote write
 // must NOT race with it, even when PHP claims admin and grants all roles.
 lease,conflict,err:=s.sharedMutation.acquire(identity.Principal{
  UserID:session.userID,Username:session.clientUsername,
 },"file.write","mutating-file.txt",time.Now().UTC())
 if err!=nil||conflict!=nil{t.Fatalf("cannot stage editor lock: %v",err)}
 defer s.sharedMutation.release(lease.token)
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileReplace,
  Path:"mutating-file.txt",Content:"must-not-commit",
  ExpectedSHA256:hex.EncodeToString(before[:]),
 })
 if err!=nil{t.Fatal(err)}
 response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err==nil||len(response)>0{t.Fatal("concurrent local editor lost exclusive mutation lease")}
 current,err:=os.ReadFile(file)
 if err!=nil||string(current)!="original"{t.Fatal("remote stomped local editor despite lock")}
}

func TestRemoteFileMutationMustBeExplicitlyEnabledByHostOperator(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 path:=filepath.Join(s.Workspace,"remote-disabled-file.txt")
 if err:=os.WriteFile(path,[]byte("unaltered"),0600);err!=nil{t.Fatal(err)}
 sha:=sha256.Sum256([]byte("unaltered"))
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileReplace,
  Path:"remote-disabled-file.txt",Content:"changed",
  ExpectedSHA256:hex.EncodeToString(sha[:]),
 })
 if err!=nil{t.Fatal(err)}
 result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err==nil||len(result)!=0{t.Fatal("read-only app opt-in automatically enabled remote write")}
 current,err:=os.ReadFile(path)
 if err!=nil||string(current)!="unaltered"{
  t.Fatal("TaskDeck file was changed without explicit host operator write opt-in")
 }
}


func writeRemoteTaskFixtures(t *testing.T,s *Server)(int,int){
 t.Helper()
 payload:=`{"version":"2.0.0","tasks":[
  {"label":"Remote safe fixture","type":"process","command":"/bin/echo",
   "args":["TASKDECK_APPROVED_TASK_PASS"],"taskdeckRemoteAllowed":true},
  {"label":"Remote unapproved fixture","type":"process","command":"/bin/echo",
   "args":["UNAUTHORIZED"],"taskdeckRemoteAllowed":false}
 ]}`
 path:=filepath.Join(s.Workspace,".vscode","tasks.json")
 if err:=os.WriteFile(path,[]byte(payload),0600);err!=nil{t.Fatal(err)}
 loaded,err:=tasks.Load(s.Workspace)
 if err!=nil||len(loaded)!=2{t.Fatalf("host tasks fixture invalid: %v %+v",err,loaded)}
 var good,bad int
 var approved tasks.Task
 for _,task:=range loaded{
  switch task.Label{
  case "Remote safe fixture":good=task.ID;approved=task
  case "Remote unapproved fixture":bad=task.ID
  }
 }
 if good==0||bad==0{t.Fatal("TaskDeck task IDs missing")}
 token,err:=tasks.RemoteApprovalToken(approved)
 if err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS",token)
 return good,bad
}

func TestE2EETaskRunOnlyForLocallyExplicitlyApprovedTask(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 approved,unapproved:=writeRemoteTaskFixtures(t,s)
 resolved,_,_,resolveErr:=s.workspaceWorkflowByTaskID(approved)
 if resolveErr!=nil||!remoteHostApprovedTask(resolved)||remoteTaskUsesAmbientExpansion(resolved){
  t.Fatalf("host task digest preflight failed: id=%d token=%q resolvedID=%d err=%v",approved,os.Getenv("TASKDECK_REMOTE_APPROVED_TASKS"),resolved.ID,resolveErr)
 }
 forged,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRun,
  TaskID:unapproved,
 })
 if err!=nil{t.Fatal(err)}
 if response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,forged);err==nil||len(response)>0{
  t.Fatal("PHP-controlled task selector executed non-allowlisted TaskDeck task")
 }
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRun,
  TaskID:approved,
 })
 if err!=nil{t.Fatal(err)}
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatalf("TaskDeck allowlisted task denied: %v",err)}
 output,err:=remotetransport.OpenRemoteTaskRunResult(client,sealed,remoteDispatchTestID)
 if err!=nil||!strings.Contains(output,"TASKDECK_APPROVED_TASK_PASS"){
  t.Fatalf("TaskDeck task E2EE transcript incorrect: %q %v",output,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.task_run",Limit:5,
 })
 if err!=nil||len(audits)==0||audits[0].UserID==nil||
  *audits[0].UserID!=session.userID{
  t.Fatalf("TaskDeck host process was not audited to local principal: %v",err)
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil{
  t.Fatal("Relay replayed an E2EE process launch")
 }
}

func TestE2EETaskRunRequiresIndependentHostOperatorOptIn(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 approved,_:=writeRemoteTaskFixtures(t,s)
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRun,
  TaskID:approved,
 })
 if err!=nil{t.Fatal(err)}
 if result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame);err==nil||len(result)>0{
  t.Fatal("read-only app-ping opt-in enabled TaskDeck process execution")
 }
}

    
func TestHostMarkedDeployTaskCannotBypassLocalTaskDeckApprovalGate(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 contents:=`{"version":"2.0.0","tasks":[{
  "label":"never remote deploy","type":"process","command":"/bin/echo",
  "args":["DEPLOY_NEVER"],"taskdeckRemoteAllowed":true,
  "taskdeckApproval":"deploy"
 }]}`
 path:=filepath.Join(s.Workspace,".vscode","tasks.json")
 if err:=os.WriteFile(path,[]byte(contents),0600);err!=nil{t.Fatal(err)}
 available,err:=tasks.Load(s.Workspace)
 if err!=nil||len(available)!=1{t.Fatalf("fixture: %v",err)}
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRun,
  TaskID:available[0].ID,
 })
 if err!=nil{t.Fatal(err)}
 if answer,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,frame);err==nil||len(answer)!=0{
  t.Fatal("remote task execution skipped TaskDeck deploy-approval gate")
 }
}
func TestNoApprovedClientPrivateKeyCannotRunPreapprovedHostTask(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 allowed,_:=writeRemoteTaskFixtures(t,s)
 owner.clientProofVerified=false
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRun,
  TaskID:allowed,
 })
 if err!=nil{t.Fatal(err)}
 if answer,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,frame);err==nil||len(answer)!=0||!owner.closed.Load(){
  t.Fatal("compromised PHP with stolen password bypassed host-approved client key to launch task")
 }
}


func TestAuthenticatedRemoteTaskStopsPromptlyWhenSessionOrCallerCancels(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 payload:=`{"version":"2.0.0","tasks":[{
  "label":"bounded remote cancellation","type":"process","command":"/bin/sleep",
  "args":["10"],"taskdeckRemoteAllowed":true
 }]}`
 if err:=os.WriteFile(filepath.Join(s.Workspace,".vscode","tasks.json"),
  []byte(payload),0600);err!=nil{t.Fatal(err)}
 taskItems,err:=tasks.Load(s.Workspace)
 if err!=nil||len(taskItems)!=1{t.Fatalf("fixture: %v",err)}
 token,err:=tasks.RemoteApprovalToken(taskItems[0])
 if err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS",token)
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRun,
  TaskID:taskItems[0].ID,
 })
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),750*time.Millisecond)
 defer cancel()
 result,err:=s.executeRemoteE2EEPing(ctx,owner,agent,frame)
 if err==nil||len(result)>0{
  t.Fatal("host returned success for cancelled long-running remote task")
 }
 deadline:=time.Now().Add(2*time.Second)
 for time.Now().Before(deadline){
  live:=false
  for _,item:=range s.Sessions.List(){
   if item.Status=="running"{live=true}
  }
  if !live{return}
  time.Sleep(25*time.Millisecond)
 }
 t.Fatal("orphaned remote task process remained running after cancellation")
}

func TestRemoteTaskMarkerCannotGrantHostOnlyTaskApproval(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 approved,_:=writeRemoteTaskFixtures(t,s)
 t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS","")
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRun,
  TaskID:approved,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err==nil||len(reply)>0{t.Fatal("editable tasks.json marker bypassed host-only approval")}
}
func TestRemoteHostTaskApprovalRequiresExactFullDigest(t *testing.T){
 s,_,_,_:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 approvedID,_:=writeRemoteTaskFixtures(t,s)
 loaded,err:=tasks.Load(s.Workspace)
 if err!=nil{t.Fatal(err)}
 var task tasks.Task
 for _,item:=range loaded{
  if item.ID==approvedID{task=item;break}
 }
 token,err:=tasks.RemoteApprovalToken(task)
 if err!=nil{t.Fatal(err)}
 wrongID:=strconv.Itoa(approvedID+1)+"@"+strings.Split(token,"@")[1]
 for _,tc:=range []struct{raw string;want bool}{
  {"",false},{"42",false},{"0",false},{token,true},
  {token+",",false},{" "+token,false},
  {token+","+token,false},{wrongID,false},
  {strings.ToUpper(token),false},
  {strconv.Itoa(approvedID)+"@"+strings.Repeat("0",64),false},
  {token+",bad",false},{token+",1@"+strings.Repeat("a",64),true},
 }{
  t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS",tc.raw)
  if got:=remoteHostApprovedTask(task);got!=tc.want{
   t.Fatalf("unsafe host task approval raw=%q got=%v want=%v",tc.raw,got,tc.want)
  }
 }
 // Old numeric-only authorizations can never grant process execution.
 t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS","")
 t.Setenv("TASKDECK_REMOTE_APPROVED_TASK_IDS",strconv.Itoa(approvedID))
 if remoteHostApprovedTask(task){t.Fatal("legacy 31-bit approval bypassed SHA-256 pin")}
}

func TestEditingAllowlistedTaskInvalidatesIndependentHostTaskID(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 server,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 approved,_:=writeRemoteTaskFixtures(t,server)
 // Host approves only the original immutable task definition's stable ID.
 // A compromised PHP or a FilesWrite client changing its command/arguments
 // must not retain that stable ID, even if the display label stays the same.
 changed:=`{"version":"2.0.0","tasks":[
  {"label":"Remote safe fixture","type":"process","command":"/bin/echo",
   "args":["ATTACKER_EDITED_TASK"],"taskdeckRemoteAllowed":true}
 ]}`
 if err:=os.WriteFile(filepath.Join(server.Workspace,".vscode","tasks.json"),
  []byte(changed),0600);err!=nil{t.Fatal(err)}
 loaded,err:=tasks.Load(server.Workspace)
 if err!=nil||len(loaded)!=1{t.Fatalf("changed tasks fixture invalid: %v",err)}
 if loaded[0].ID==approved{t.Fatal("editing a command unexpectedly preserved its host-approved stable task ID")}
 attack,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRun,
  TaskID:loaded[0].ID,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=server.executeRemoteE2EEPing(context.Background(),session,agent,attack)
 if err==nil||len(reply)!=0{t.Fatal("altered task command bypassed independent host approval")}
}

func TestRemoteTaskRejectsShellAndDaemonEnvironmentExpansion(t *testing.T){
 for _,tc:=range []struct{name,kind,command string;args string}{
  {"shell type","shell","printf UNSAFE",`[]`},
  {"daemon env expansion","process","/bin/echo",`["${env:TASKDECK_REMOTE_SECRET_CANARY}"]`},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
   t.Setenv("TASKDECK_REMOTE_SECRET_CANARY","dont-leak-me")
   s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
   payload:=`{"version":"2.0.0","tasks":[{"label":"unsafe remote task","type":"`+tc.kind+`","command":"`+tc.command+`","args":`+tc.args+`,"taskdeckRemoteAllowed":true}]}`
   if err:=os.WriteFile(filepath.Join(s.Workspace,".vscode","tasks.json"),[]byte(payload),0600);err!=nil{t.Fatal(err)}
   items,err:=tasks.Load(s.Workspace)
   if err!=nil||len(items)!=1{t.Fatalf("fixture: %v",err)}
   token,err:=tasks.RemoteApprovalToken(items[0]);if err!=nil{t.Fatal(err)}
   t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS",token)
   frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRun,
    TaskID:items[0].ID,
   });if err!=nil{t.Fatal(err)}
   reply,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,frame)
   if err==nil||len(reply)!=0{t.Fatal("shell or ambient environment interpolation executed remotely")}
  })
 }
}

func TestRemoteApprovedTaskDoesNotInheritDaemonSecrets(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 t.Setenv("TASKDECK_REMOTE_SECRET_CANARY","sensitive-value-not-for-child")
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 payload:=`{"version":"2.0.0","tasks":[{"label":"check isolated child environment",
 "type":"process","command":"/usr/bin/printenv","args":["TASKDECK_REMOTE_SECRET_CANARY"],
 "taskdeckRemoteAllowed":true}]}`
 if err:=os.WriteFile(filepath.Join(s.Workspace,".vscode","tasks.json"),[]byte(payload),0600);err!=nil{t.Fatal(err)}
 items,err:=tasks.Load(s.Workspace)
 if err!=nil||len(items)!=1{t.Fatalf("fixture: %v",err)}
 token,err:=tasks.RemoteApprovalToken(items[0]);if err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS",token)
 resolved,_,_,resolveErr:=s.workspaceWorkflowByTaskID(items[0].ID)
 if resolveErr!=nil||!remoteHostApprovedTask(resolved)||remoteTaskUsesAmbientExpansion(resolved){
  t.Fatalf("isolated task digest preflight failed: id=%d resolvedID=%d err=%v",items[0].ID,resolved.ID,resolveErr)
 }
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRun,TaskID:items[0].ID,
 });if err!=nil{t.Fatal(err)}
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,frame)
 if err!=nil{t.Fatalf("isolated process denied: %v",err)}
 output,err:=remotetransport.OpenRemoteTaskRunResult(client,sealed,remoteDispatchTestID)
 if err!=nil||strings.Contains(output,"sensitive-value-not-for-child")||!strings.Contains(output,"mã trả về: 1"){
  t.Fatalf("daemon secret reached remote task: output=%q err=%v",output,err)
 }
}


func TestRemoteE2EEFilePreviewReadsFull4KiBWithoutScopedDownload(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 content:=strings.Repeat("A",remotetransport.RemoteFilePreviewMaxBytes)
 filename:=filepath.Join(s.Workspace,"visible preview.txt")
 if err:=os.WriteFile(filename,[]byte(content),0600);err!=nil{t.Fatal(err)}
 action:=remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFilePreview,
  Path:"visible preview.txt",
 }
 req,err:=remotetransport.SealRemoteAction(client,action)
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 preview,err:=remotetransport.OpenRemoteFilePreviewResult(client,reply,remoteDispatchTestID)
 if err!=nil||preview!=content{t.Fatalf("full 4KiB preview failed: size=%d err=%v",len(preview),err)}
 session.transferMu.Lock()
 scoped:=session.transferActive
 session.transferMu.Unlock()
 if scoped{t.Fatal("text preview mistakenly activated binary file-download-only mode")}
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.project_file_preview",Limit:5,
 })
 if err!=nil||len(audit)==0||audit[0].UserID==nil||*audit[0].UserID!=session.userID{
  t.Fatalf("missing local TaskDeck identity audit for file preview: %v",err)
 }
 // Follow-up typed command must still work after the text preview.
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:"11111111111111111111111111111111",
  Operation:remotetransport.RemoteActionSessionPing,
 })
 if err!=nil{t.Fatal(err)}
 result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatalf("preview prematurely locked the application session: %v",err)}
 if err:=remotetransport.OpenRemoteActionPingResult(client,result,
  "11111111111111111111111111111111");err!=nil{t.Fatal(err)}
}

func TestRemoteE2EEFilePreviewRejectsIncompleteAndOversizedText(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 if err:=os.WriteFile(filepath.Join(s.Workspace,"too-long.txt"),
  []byte(strings.Repeat("x",remotetransport.RemoteFilePreviewMaxBytes+1)),0600);err!=nil{t.Fatal(err)}
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFilePreview,
  Path:"too-long.txt",
 })
 if err!=nil{t.Fatal(err)}
 if response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame);
  err==nil||len(response)!=0{
  t.Fatal("oversized text preview leaked a truncated/unauthenticated file")
 }
}

func TestRemoteGitLogAndBranchesRequireLocalPrincipalAndAreAudited(t *testing.T){
 for _,op:=range []string{remotetransport.RemoteActionGitLog,remotetransport.RemoteActionGitBranches}{
  s,session,client,agent:=remoteDispatchFixture(t)
  if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{t.Skipf("git unavailable: %v",err)}
  commit:=exec.Command("git","-C",s.Workspace,"-c","user.name=Example",
   "-c","user.email=test@example.invalid","commit","--allow-empty","-m","history test")
  if err:=commit.Run();err!=nil{t.Skipf("git commit fixture unavailable: %v",err)}
  request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:op,
  })
  if err!=nil{t.Fatal(err)}
  if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,request);err==nil||len(reply)>0{
   t.Fatal("PHP-only forged session accessed Git metadata")
  }
  reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
  if err!=nil{t.Fatal(err)}
  var output string
  if op==remotetransport.RemoteActionGitLog{
   output,err=remotetransport.OpenRemoteGitLogResult(client,reply,remoteDispatchTestID)
  }else{
   output,err=remotetransport.OpenRemoteGitBranchesResult(client,reply,remoteDispatchTestID)
  }
  if err!=nil||output==""{t.Fatalf("Git metadata missing: %s %q %v",op,output,err)}
  action:="remote.action.git_log"
  if op==remotetransport.RemoteActionGitBranches{action="remote.action.git_branches"}
  project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
  if err!=nil{t.Fatal(err)}
  audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{ProjectID:project.ID,Action:action,Limit:5})
  if err!=nil||len(audit)==0||audit[0].UserID==nil||*audit[0].UserID!=session.userID{
   t.Fatalf("missing host-only TaskDeck Git metadata audit: %s %v",action,err)
  }
 }
}


func TestRemoteFourKiBTextEditHostOptInCASAndAudit(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
 t.Setenv("TASKDECK_REMOTE_TEXT_EDIT","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 filename:=filepath.Join(s.Workspace,"code with spaces.txt")
 before:=[]byte("previous source")
 if err:=os.WriteFile(filename,before,0600);err!=nil{t.Fatal(err)}
 prev:=sha256.Sum256(before)
 current:=strings.Repeat("a",remotetransport.RemoteTextEditMaxBytes)
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileReplaceText,
  Path:"code with spaces.txt",Content:current,ExpectedSHA256:hex.EncodeToString(prev[:]),
 })
 if err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,req);err==nil||len(reply)>0{
  t.Fatal("PHP-only grant bypassed local TaskDeck principal and FilesWrite")
 }
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 sha,err:=remotetransport.OpenRemoteTextEditResult(client,reply,remoteDispatchTestID)
 if err!=nil{t.Fatal(err)}
 written,err:=os.ReadFile(filename)
 if err!=nil||string(written)!=current{t.Fatalf("4KiB pinned CAS content wrong: %v",err)}
 checksum:=sha256.Sum256(written)
 if sha!=hex.EncodeToString(checksum[:]){t.Fatal("wrong committed E2EE file hash")}
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.project_file_replace_text",Limit:5,
 })
 if err!=nil||len(audit)==0||audit[0].UserID==nil||*audit[0].UserID!=session.userID{
  t.Fatal("missing host-only FilesWrite audit")
 }
}
func TestRemoteFourKiBTextEditDefaultOffAndStaleCAS(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
 t.Setenv("TASKDECK_REMOTE_TEXT_EDIT","0")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 filename:=filepath.Join(s.Workspace,"edit.txt")
 if err:=os.WriteFile(filename,[]byte("new version"),0600);err!=nil{t.Fatal(err)}
 old:=sha256.Sum256([]byte("old version"))
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileReplaceText,
  Path:"edit.txt",Content:"attacker overwrite",ExpectedSHA256:hex.EncodeToString(old[:]),
 })
 if err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil||len(reply)>0{
  t.Fatal("unapproved 4KiB host edit was accepted")
 }
 text,err:=os.ReadFile(filename)
 if err!=nil||string(text)!="new version"{t.Fatal("default-off text editor changed source")}
}

func TestRemoteFourKiBTextEditRejectsStaleSHAWithoutFileWrite(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
 t.Setenv("TASKDECK_REMOTE_TEXT_EDIT","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 filename:=filepath.Join(s.Workspace,"existing.txt")
 if err:=os.WriteFile(filename,[]byte("changed by other editor"),0600);err!=nil{t.Fatal(err)}
 stale:=sha256.Sum256([]byte("previous editor version"))
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileReplaceText,
  Path:"existing.txt",Content:"must not commit",ExpectedSHA256:hex.EncodeToString(stale[:]),
 })
 if err!=nil{t.Fatal(err)}
 result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err==nil||len(result)!=0{t.Fatal("stale SHA accepted as optimistic file write")}
 actual,err:=os.ReadFile(filename)
 if err!=nil||string(actual)!="changed by other editor"{
  t.Fatal("failed edit changed file despite stale SHA")
 }
}

func TestRemoteTextEditViewerCannotWriteEvenWithHostFlags(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
 t.Setenv("TASKDECK_REMOTE_TEXT_EDIT","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleViewer)
 filename:=filepath.Join(s.Workspace,"viewer-denied.txt")
 if err:=os.WriteFile(filename,[]byte("safe"),0600);err!=nil{t.Fatal(err)}
 sha:=sha256.Sum256([]byte("safe"))
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileReplaceText,
  Path:"viewer-denied.txt",Content:"forbidden",ExpectedSHA256:hex.EncodeToString(sha[:]),
 })
 if err!=nil{t.Fatal(err)}
 response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err==nil||len(response)!=0{t.Fatal("Portal viewer illegally gained FilesWrite")}
 saved,err:=os.ReadFile(filename)
 if err!=nil||string(saved)!="safe"{t.Fatal("viewer modified TaskDeck source")}
}


func TestRemoteApprovedTaskCatalogueOnlyListsHostAuthorizedSafeTasks(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 approved,_:=writeRemoteTaskFixtures(t,s)
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskList,
 })
 if err!=nil{t.Fatal(err)}
 if response,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,request);err==nil||len(response)!=0{
  t.Fatal("PHP-only request enumerated TaskDeck host-approved tasks")
 }
 response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 result,err:=remotetransport.OpenRemoteTaskListResult(client,response,remoteDispatchTestID)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(result,strconv.Itoa(approved)+"\tRemote safe fixture")||
  strings.Contains(result,"Remote unapproved fixture")||
  strings.Contains(result,"/bin/echo")||
  strings.Contains(result,"TASKDECK_APPROVED_TASK_PASS"){
  t.Fatalf("task catalogue leaked unapproved tasks or command source: %q",result)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.task_list",Limit:5,
 })
 if err!=nil||len(audits)==0||audits[0].UserID==nil||*audits[0].UserID!=session.userID{
  t.Fatal("missing host TaskDeck task catalogue audit")
 }
}
func TestRemoteTaskCatalogueDeniedForViewerOrWithoutHostFlag(t *testing.T){
 for _,tc:=range []struct{role remotetransport.Role;flag string}{
  {remotetransport.RoleViewer,"1"},{remotetransport.RoleOperator,"0"},
 }{
  t.Run(string(tc.role)+"-"+tc.flag,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASK_RUN",tc.flag)
   s,session,client,agent:=remoteDispatchFixtureWithRole(t,tc.role)
   writeRemoteTaskFixtures(t,s)
   request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskList,
   })
   if err!=nil{t.Fatal(err)}
   result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
   if err==nil||len(result)!=0{t.Fatal("remote task listing bypassed local policy")}
  })
 }
}
func TestRemoteStagedGitDiffOnlyRevealsPinnedVisibleSourceWithRBAC(t *testing.T){
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleViewer)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{t.Skipf("git unavailable: %v",err)}
 file:=filepath.Join(s.Workspace,"staged.txt")
 if err:=os.WriteFile(file,[]byte("index secret without shell\n"),0600);err!=nil{t.Fatal(err)}
 if err:=exec.Command("git","-C",s.Workspace,"add","--","staged.txt").Run();err!=nil{t.Fatal(err)}
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitStagedDiff,
  Path:"staged.txt",
 })
 if err!=nil{t.Fatal(err)}
 if result,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,frame);err==nil||len(result)!=0{
  t.Fatal("PHP alone read Git index secrets")
 }
 answer,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err!=nil{t.Fatal(err)}
 result,err:=remotetransport.OpenRemoteGitStagedDiffResult(client,answer,remoteDispatchTestID)
 if err!=nil||!strings.Contains(result,"+index secret without shell"){
  t.Fatalf("staged Git diff result missing or unauthenticated: %q %v",result,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_diff_staged",Limit:5,
 })
 if err!=nil||len(audit)==0||audit[0].UserID==nil||*audit[0].UserID!=session.userID{
  t.Fatal("missing Git staged diff TaskDeck-local audit")
 }
}


func TestRemoteApprovedTerminalTaskReturnsCompleteText4KiB(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 payload:="{\"version\":\"2.0.0\",\"tasks\":[{\"label\":\"remote bounded output\","+
  "\"type\":\"process\",\"command\":\"/bin/echo\","+
  "\"args\":[\""+strings.Repeat("Q",1100)+"\"],\"taskdeckRemoteAllowed\":true}]}"
 if err:=os.WriteFile(filepath.Join(s.Workspace,".vscode","tasks.json"),[]byte(payload),0600);err!=nil{t.Fatal(err)}
 items,err:=tasks.Load(s.Workspace)
 if err!=nil||len(items)!=1{t.Fatalf("task fixture: %v",err)}
 token,err:=tasks.RemoteApprovalToken(items[0])
 if err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS",token)
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRunText,TaskID:items[0].ID,
 })
 if err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,frame);err==nil||len(reply)!=0{
  t.Fatal("PHP-only request executed task with extended output")
 }
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err!=nil{t.Fatal(err)}
 content,err:=remotetransport.OpenRemoteTaskRunTextResult(client,reply,remoteDispatchTestID)
 if err!=nil||!strings.Contains(content,strings.Repeat("Q",1100)){
  t.Fatalf("task output truncated/unauthenticated: %d %v",len(content),err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.task_run_text",Limit:5,
 })
 if err!=nil||len(audits)==0||audits[0].UserID==nil||*audits[0].UserID!=session.userID{
  t.Fatal("missing TaskDeck task transcript host audit")
 }
}
