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
 grant:=testRemoteSignedGrant(t,agent,remotetransport.RoleViewer)
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
 s,session,client,agent:=remoteDispatchFixture(t)
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
  s,session,client,agent:=remoteDispatchFixture(t)
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
  s,session,client,agent:=remoteDispatchFixture(t)
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
 s,session,client,agent:=remoteDispatchFixture(t)
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
 s,session,client,agent:=remoteDispatchFixture(t)
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
 for _,task:=range loaded{
  switch task.Label{
  case "Remote safe fixture":good=task.ID
  case "Remote unapproved fixture":bad=task.ID
  }
 }
 if good==0||bad==0{t.Fatal("TaskDeck task IDs missing")}
 return good,bad
}

func TestE2EETaskRunOnlyForLocallyExplicitlyApprovedTask(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 approved,unapproved:=writeRemoteTaskFixtures(t,s)
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
 s,session,client,agent:=remoteDispatchFixture(t)
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
 s,owner,client,agent:=remoteDispatchFixture(t)
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
 s,owner,client,agent:=remoteDispatchFixture(t)
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
 s,owner,client,agent:=remoteDispatchFixture(t)
 payload:=`{"version":"2.0.0","tasks":[{
  "label":"bounded remote cancellation","type":"process","command":"/bin/sleep",
  "args":["10"],"taskdeckRemoteAllowed":true
 }]}`
 if err:=os.WriteFile(filepath.Join(s.Workspace,".vscode","tasks.json"),
  []byte(payload),0600);err!=nil{t.Fatal(err)}
 taskItems,err:=tasks.Load(s.Workspace)
 if err!=nil||len(taskItems)!=1{t.Fatalf("fixture: %v",err)}
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
