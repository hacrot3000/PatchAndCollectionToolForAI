package server

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "encoding/base64"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remoteportal"
 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
 "bletonfc/vscode_tasks_menu/internal/remoterelay"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "github.com/coder/websocket"
)

// Full real TLS/WSS client -> Relay -> Agent -> local TaskDeck SQLite
// authenticated E2EE session.ping -> client. PHP is malicious and returns
// allowed=true, but TaskDeck's actual password/key/RBAC are authoritative.
// The only supported operation is the fixed read-only session.ping.
func TestRealWSSAppPingWithCompromisedPHPStillRequiresLocalTaskDeckLogin(t *testing.T){
 t.Run("reject_unreviewed_shell",func(t *testing.T){
  runRealWSSAppPingWithCompromisedPHP(t,false)
 })
 t.Run("host_opt_in_task_preview",func(t *testing.T){
  runRealWSSAppPingWithCompromisedPHP(t,true)
 })
}

func runRealWSSAppPingWithCompromisedPHP(t *testing.T,previewTasks bool){
 requireGit(t)
 t.Setenv("TASKDECK_REMOTE_GIT_STATUS","1")
 t.Setenv("TASKDECK_REMOTE_PROJECT_INFO","1")
 taskdeck:=sharedLoginTestServer(t)
 gitInitRemoteTest(t,taskdeck.Workspace)
 if previewTasks {
  t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
  dir:=filepath.Join(taskdeck.Workspace,".vscode")
  if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
  if err:=os.WriteFile(filepath.Join(dir,"tasks.json"),[]byte(fixtureTaskLabels),0600);err!=nil{t.Fatal(err)}
 }
 taskdeck.Config.RemoteAccessEnabled=true
 taskdeck.Config.Protocol=config.ProtocolHTTPS
 portalPub,portalPriv,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 var srv *httptest.Server
 var handler *remoterelay.Handler
 portal:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path {
  case "/api/agent/heartbeat":
   var request struct{DeviceID string `json:"device_id"`}
   if json.NewDecoder(r.Body).Decode(&request)!=nil{
    http.Error(w,"invalid heartbeat",400);return
   }
   now:=time.Now().Unix()
   lease:=remoterelay.DeviceLease{V:1,Iss:srv.URL,Aud:"taskdeck-relay-device",
    DeviceID:request.DeviceID,Iat:now,Exp:now+70}
   bytes,e:=json.Marshal(lease);if e!=nil{t.Error(e);return}
   body:=base64.RawURLEncoding.EncodeToString(bytes)
   sig:=ed25519.Sign(portalPriv,[]byte("taskdeck-relay-device-lease-v1."+body))
   _=json.NewEncoder(w).Encode(map[string]any{
    "ok":true,"device_lease":body+"."+base64.RawURLEncoding.EncodeToString(sig),
   })
  case "/api/agent/grant-check":
   _=json.NewEncoder(w).Encode(map[string]bool{"ok":true,"allowed":true})
  default:
   handler.ServeHTTP(w,r)
  }
 })
 srv=httptest.NewTLSServer(portal)
 defer srv.Close()
 // The real host fail-closed admission checks that the Agent's pinned
 // Portal origin equals this TaskDeck workspace's configured origin.
 // Without this fixture field the login is correctly rejected BEFORE
 // either session.ping or git.status, masking the operation tests.
 taskdeck.Config.RemotePortalURL=srv.URL
 registry,err:=remoterelay.NewRegistry(srv.URL);if err!=nil{t.Fatal(err)}
 verifier,err:=remoterelay.NewVerifier(base64.StdEncoding.EncodeToString(portalPub),srv.URL)
 if err!=nil{t.Fatal(err)}
 handler=&remoterelay.Handler{Tickets:verifier,Devices:registry,AllowAppPing:true}
 keyPath,err:=remoteportal.DefaultIdentityPath(taskdeck.Workspace)
 if err!=nil{t.Fatal(err)}
 agent,err:=remoteportal.NewAgent(srv.URL,keyPath,srv.Client())
 if err!=nil{t.Fatal(err)}
 if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(portalPub));err!=nil{t.Fatal(err)}
 if err:=agent.ConfigureClientKeyLocalUserLookup(taskdeck.LookupRemoteClientLocalUserID);err!=nil{t.Fatal(err)}
 if err:=agent.ConfigureEncryptedApplicationSession(taskdeck.StartEncryptedRemotePingHost);err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),70*time.Second)
 defer cancel()
 if err:=agent.Heartbeat(ctx);err!=nil{t.Fatal(err)}
 ended:=make(chan error,1)
 go func(){ended<-agent.ConnectRelay(ctx,srv.URL)}()
 deadline:=time.After(5*time.Second)
 for !registry.Online(agent.DeviceID(),time.Now()){
  select{
  case <-deadline:t.Fatal("authenticated Agent did not connect over real TLS")
  case err:=<-ended:t.Fatalf("Agent disconnected: %v",err)
  case <-time.After(20*time.Millisecond):
  }
 }
 approvedPub,approvedPriv,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 alice,err:=taskdeck.Identity.UserByUsername(ctx,"alice")
 if err!=nil{t.Fatal(err)}
 keyRegistry,err:=remoteclientkey.DefaultPath(taskdeck.Workspace)
 if err!=nil{t.Fatal(err)}
 if _,err:=remoteclientkey.AddForUser(keyRegistry,alice.Username,
  string(alice.ID),base64.StdEncoding.EncodeToString(approvedPub),
  "WSS app ping integration fixture");err!=nil{t.Fatal(err)}
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 nonce:=strings.Repeat("c",32)
 binding,err:=remotetransport.BrowserTicketBinding(nonce,browser.Public)
 if err!=nil{t.Fatal(err)}
 now:=time.Now().Unix()
 claims:=remoterelay.Claims{V:2,Issuer:srv.URL,Audience:"taskdeck-relay",
  Subject:"1",DeviceID:agent.DeviceID(),AuthEpoch:1,Role:"admin",
  IssuedAt:now,Expires:now+30,JTI:strings.Repeat("e",32),
  BrowserBinding:binding}
 payload,err:=json.Marshal(claims);if err!=nil{t.Fatal(err)}
 body:=base64.RawURLEncoding.EncodeToString(payload)
 sig:=ed25519.Sign(portalPriv,[]byte("taskdeck-relay-ticket-v1."+body))
 ticket:=body+"."+base64.RawURLEncoding.EncodeToString(sig)
 conn,_,err:=websocket.Dial(ctx,srv.URL+"/v1/browser",
  &websocket.DialOptions{HTTPClient:srv.Client(),
   HTTPHeader:http.Header{"Origin":[]string{srv.URL}}})
 if err!=nil{t.Fatal(err)}
 defer conn.CloseNow()
 send:=func(frame map[string]string){
  bytes,e:=json.Marshal(frame);if e!=nil{t.Fatal(e)}
  if e:=conn.Write(ctx,websocket.MessageText,bytes);e!=nil{t.Fatal(e)}
 }
 receive:=func(phase string)[]byte{
  kind,bytes,e:=conn.Read(ctx)
  if e!=nil||kind!=websocket.MessageText{t.Fatalf("WSS %s reply: %v",phase,e)}
  return bytes
 }
 send(map[string]string{"ticket":ticket,"client_nonce":nonce,
  "browser_key":base64.StdEncoding.EncodeToString(browser.Public)})
 var proof struct{
  TicketValid bool `json:"ticket_valid"`
  AgentReachable bool `json:"agent_reachable"`
  RemoteControlAvailable bool `json:"remote_control_available"`
  DeviceID string `json:"device_id"`
  AgentProof remoterelay.ProbeAttestation `json:"agent_proof"`
 }
 if err:=json.Unmarshal(receive("Agent proof"),&proof);err!=nil{t.Fatal(err)}
 if !proof.TicketValid||!proof.AgentReachable||proof.RemoteControlAvailable||
  proof.DeviceID!=agent.DeviceID()||proof.AgentProof.Crypto==nil{
  t.Fatal("compromised PHP forged Agent identity or app authority")
 }
 decode:=func(s string)[]byte{
  bytes,e:=base64.StdEncoding.Strict().DecodeString(s);if e!=nil{t.Fatal(e)}
  return bytes
 }
 crypt:=proof.AgentProof.Crypto
 client,err:=remotetransport.AcceptHello(browser,agent.DeviceID(),proof.AgentProof.ProbeID,
  nonce,remotetransport.Hello{
   Identity:decode(crypt.Identity),Ephemeral:decode(crypt.Ephemeral),
   Signature:decode(crypt.Signature),
  })
 if err!=nil{t.Fatal(err)}
 defer client.Close()
 ready,err:=client.Open(decode(crypt.EncryptedReady))
 if err!=nil||string(ready)!="taskdeck-e2ee-ready-v1"{
  t.Fatalf("Agent AEAD ready verification failed: %v",err)
 }
 privateProof:=ed25519.Sign(approvedPriv,
  remoteclientkey.ProofMessage("alice",client.RemoteClientProofBinding()))
 localSig:=base64.StdEncoding.EncodeToString(privateProof)
 preflight,err:=remotetransport.ClientKeyCheckFrame(client,remotetransport.ClientKeyCheck{
  Username:"alice",PublicKey:base64.StdEncoding.EncodeToString(approvedPub),
  Signature:localSig,
 })
 if err!=nil{t.Fatal(err)}
 send(map[string]string{"type":"e2ee_client_key_check",
  "ciphertext":base64.StdEncoding.EncodeToString(preflight)})
 var k struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(receive("client key approval"),&k);err!=nil{t.Fatal(err)}
 if k.Type!="e2ee_client_key_check_result"||k.RemoteControlAvailable{
  t.Fatal("PHP opened an application route before host approval")
 }
 approved,err:=remotetransport.OpenClientKeyStatus(client,decode(k.Ciphertext))
 if err!=nil||!approved{t.Fatalf("host key approval rejected: %v",err)}
 login,err:=remotetransport.RemoteApplicationLoginRequestWithClientKey(client,"alice",
  "private-admin-password",base64.StdEncoding.EncodeToString(approvedPub),localSig)
 if err!=nil{t.Fatal(err)}
 send(map[string]string{"type":"e2ee_app_login","ciphertext":base64.StdEncoding.EncodeToString(login)})
 var result struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(receive("TaskDeck app login"),&result);err!=nil{t.Fatal(err)}
 if result.Type!="e2ee_login_result"||result.RemoteControlAvailable {
  t.Fatal("Relay forged remote app authority")
 }
 authenticated,err:=remotetransport.OpenRemoteLoginResult(client,decode(result.Ciphertext))
 if err!=nil||!authenticated{t.Fatalf("local TaskDeck denied correct login: %v",err)}
 id:=strings.Repeat("b",32)
 ping,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:id,Operation:remotetransport.RemoteActionSessionPing})
 if err!=nil{t.Fatal(err)}
 send(map[string]string{"type":"e2ee_app_ping","ciphertext":base64.StdEncoding.EncodeToString(ping)})
 var response struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(receive("session.ping"),&response);err!=nil{t.Fatal(err)}
 if response.Type!="e2ee_app_ping_result"||response.RemoteControlAvailable{
  t.Fatal("untrusted Relay supplied arbitrary TaskDeck operation")
 }
 if err:=remotetransport.OpenRemoteActionPingResult(client,decode(response.Ciphertext),id);err!=nil{
  t.Fatalf("real WSS TaskDeck-local session.ping not authenticated end-to-end: %v",err)
 }
 // The same trusted browser E2EE principal can request one fixed,
 // read-only Git status. PHP and Relay route ciphertext without selecting
 // a repository, a Git command, or any TaskDeck authorization principal.
 gitID:=strings.Repeat("d",32)
 gitRequest,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:gitID,Operation:remotetransport.RemoteActionGitStatus,
 })
 if err!=nil{t.Fatal(err)}
 if strings.Contains(base64.StdEncoding.EncodeToString(gitRequest),"git.status"){
  t.Fatal("Relay visible Git action plaintext")
 }
 send(map[string]string{"type":"e2ee_app_ping","ciphertext":base64.StdEncoding.EncodeToString(gitRequest)})
 var gitReply struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(receive("git.status"),&gitReply);err!=nil{t.Fatal(err)}
 if gitReply.Type!="e2ee_app_ping_result"||gitReply.RemoteControlAvailable{
  t.Fatal("Relay modified Git status application scope")
 }
 gitStatus,err:=remotetransport.OpenRemoteActionGitStatusResult(client,decode(gitReply.Ciphertext),gitID)
 if err!=nil || !strings.HasPrefix(gitStatus,"## "){
  t.Fatalf("real WSS Git status failed host-side E2EE/RBAC: %q %v",gitStatus,err)
 }
 // Project overview is the third real WSS AEAD action. The untrusted
 // PHP/Relay sees neither the TaskDeck display name nor the workspace.
 infoID:=strings.Repeat("e",32)
 infoRequest,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:infoID,Operation:remotetransport.RemoteActionProjectInfo,
 })
 if err!=nil{t.Fatal(err)}
 send(map[string]string{"type":"e2ee_app_ping","ciphertext":base64.StdEncoding.EncodeToString(infoRequest)})
 var infoReply struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(receive("project.info"),&infoReply);err!=nil{t.Fatal(err)}
 if infoReply.Type!="e2ee_app_ping_result"||infoReply.RemoteControlAvailable{
  t.Fatal("PHP or Relay substituted project metadata capability")
 }
 verifiedInfo,err:=remotetransport.OpenRemoteActionProjectInfoResult(client,decode(infoReply.Ciphertext),infoID)
 if err!=nil || verifiedInfo.ProjectName=="" || verifiedInfo.WorkspaceName==""{
  t.Fatalf("real WSS TaskDeck project metadata failed end-to-end: %+v %v",verifiedInfo,err)
 }
 project,err:=taskdeck.Identity.ProjectByKey(ctx,taskdeck.Config.SharedProjectID)
 if err!=nil{t.Fatal(err)}
 audits,err:=taskdeck.Identity.ListAudit(ctx,identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.session_ping",Limit:5,
 })
 if err!=nil{t.Fatal(err)}
 if len(audits)==0||audits[0].UserID==nil||*audits[0].UserID!=alice.ID{
  t.Fatal("authenticated WSS ping lacked local TaskDeck user audit")
 }
 gitAudits,err:=taskdeck.Identity.ListAudit(ctx,identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_status",Limit:5,
 })
 if err!=nil||len(gitAudits)==0||gitAudits[0].UserID==nil||
  *gitAudits[0].UserID!=alice.ID{
  t.Fatalf("WSS Git status audit did not bind local TaskDeck principal: %+v %v",gitAudits,err)
 }
 projectAudits,err:=taskdeck.Identity.ListAudit(ctx,identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.project_info",Limit:5,
 })
 if err!=nil || len(projectAudits)==0 || projectAudits[0].UserID==nil ||
  *projectAudits[0].UserID!=alice.ID{
  t.Fatalf("real WSS project.info not audited against local TaskDeck user: %+v %v",projectAudits,err)
 }
 if previewTasks {
  // Fourth and final real Relay mailbox operation: only a bounded
  // authenticated task-label summary. No command, task ID or args exist.
  previewID:=strings.Repeat("f",32)
  previewRequest,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:previewID,Operation:remotetransport.RemoteActionTasksPreview,
  })
  if err!=nil{t.Fatal(err)}
  send(map[string]string{"type":"e2ee_app_ping","ciphertext":base64.StdEncoding.EncodeToString(previewRequest)})
  var previewReply struct{
   Type string `json:"type"`
   Ciphertext string `json:"ciphertext"`
   RemoteControlAvailable bool `json:"remote_control_available"`
  }
  if err:=json.Unmarshal(receive("tasks.preview"),&previewReply);err!=nil{t.Fatal(err)}
  if previewReply.Type!="e2ee_app_ping_result"||previewReply.RemoteControlAvailable{
   t.Fatal("compromised PHP promoted tasks.preview to command authority")
  }
  preview,err:=remotetransport.OpenRemoteActionTasksPreviewResult(client,decode(previewReply.Ciphertext),previewID)
  if err!=nil||preview.Total!=4||len(preview.Labels)!=3||preview.Labels[0]!="Build app"{
   t.Fatalf("real WSS TaskDeck task preview failed authenticated host read: %+v %v",preview,err)
  }
  previewAudits,err:=taskdeck.Identity.ListAudit(ctx,identity.AuditQuery{
   ProjectID:project.ID,Action:"remote.action.tasks_preview",Limit:5,
  })
  if err!=nil||len(previewAudits)==0||previewAudits[0].UserID==nil||
   *previewAudits[0].UserID!=alice.ID{
   t.Fatalf("real WSS task preview missing local TaskDeck audit: %+v %v",previewAudits,err)
  }
  return
 }
 // No command-shaped E2EE request can replace the three typed operations.
 frame,err:=client.Seal([]byte(`{"v":1,"type":"shell.exec","command":"id"}`))
 if err!=nil{t.Fatal(err)}
 send(map[string]string{"type":"e2ee_app_ping","ciphertext":base64.StdEncoding.EncodeToString(frame)})
 // Host rejects this typed-command intrusion and closes its E2EE session.
 // The Relay may keep the browser socket until its bounded mailbox timeout;
 // its eventual response must NEVER be a TaskDeck success.
 readCtx,stopRead:=context.WithTimeout(ctx,5*time.Second)
 defer stopRead()
 _,_,e:=conn.Read(readCtx)
 if e==nil{t.Fatal("unapproved command obtained a TaskDeck response")}
}
