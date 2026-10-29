package server

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "strings"
 "sync/atomic"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remoteportal"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
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
