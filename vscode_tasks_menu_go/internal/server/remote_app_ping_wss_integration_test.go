package server

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "os/exec"
 "path/filepath"
 "bletonfc/vscode_tasks_menu/internal/tasks"
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
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 taskdeck:=sharedLoginTestServer(t)
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
 // Bind the TaskDeck-local owner portal pin to the actual HTTPS test origin.
 // Production deliberately refuses E2EE app login without this host config.
 taskdeck.Config.RemotePortalURL=srv.URL
 defer srv.Close()
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
 receive:=func(stage string)[]byte{
  kind,bytes,e:=conn.Read(ctx)
  if e!=nil||kind!=websocket.MessageText{t.Fatalf("WSS %s reply: %v",stage,e)}
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
 if err:=json.Unmarshal(receive("agent_proof"),&proof);err!=nil{t.Fatal(err)}
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
 if err:=json.Unmarshal(receive("client_key_check"),&k);err!=nil{t.Fatal(err)}
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
 if err:=json.Unmarshal(receive("taskdeck_login"),&result);err!=nil{t.Fatal(err)}
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
 if err:=json.Unmarshal(receive("session_ping"),&response);err!=nil{t.Fatal(err)}
 if response.Type!="e2ee_app_ping_result"||response.RemoteControlAvailable{
  t.Fatal("untrusted Relay supplied arbitrary TaskDeck operation")
 }
 if err:=remotetransport.OpenRemoteActionPingResult(client,decode(response.Ciphertext),id);err!=nil{
  t.Fatalf("real WSS TaskDeck-local session.ping not authenticated end-to-end: %v",err)
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
 // REAL TaskDeck data must remain end-to-end secret even if PHP issues
 // admin tickets and controls all Portal grant decisions.
 filePath:=filepath.Join(taskdeck.Workspace,"wss-project-preview.txt")
 if err:=os.WriteFile(filePath,[]byte("private project file over E2EE"),0600);err!=nil{
  t.Fatal(err)
 }
 fileFrame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:strings.Repeat("c",32),
  Operation:remotetransport.RemoteActionProjectFileRead,
  Path:"wss-project-preview.txt",
 })
 if err!=nil{t.Fatal(err)}
 send(map[string]string{"type":"e2ee_app_ping",
  "ciphertext":base64.StdEncoding.EncodeToString(fileFrame)})
 var fileReply struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(receive("file_read"),&fileReply);err!=nil{t.Fatal(err)}
 if fileReply.Type!="e2ee_app_ping_result"||fileReply.RemoteControlAvailable{
  t.Fatal("Relay exposed an unauthorized file capability")
 }
 fileContents,err:=remotetransport.OpenRemoteFileReadResult(client,
  decode(fileReply.Ciphertext),strings.Repeat("c",32))
 if err!=nil||fileContents!="private project file over E2EE"{
  t.Fatalf("real WSS read did not preserve E2EE file data: %q %v",fileContents,err)
 }
 // The same real WSS session must safely commit one authorized file
 // replacement, protecting its original version with a strict SHA-256
 // compare-and-swap. A compromised PHP sees only AEAD ciphertext.
 oldFileSHA:=sha256.Sum256([]byte(fileContents))
 replace,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:strings.Repeat("e",32),
  Operation:remotetransport.RemoteActionProjectFileReplace,
  Path:"wss-project-preview.txt",Content:"updated from trusted client",
  ExpectedSHA256:hex.EncodeToString(oldFileSHA[:]),
 })
 if err!=nil{t.Fatal(err)}
 send(map[string]string{"type":"e2ee_app_ping",
  "ciphertext":base64.StdEncoding.EncodeToString(replace)})
 var replaced struct {
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(receive("file_replace"),&replaced);err!=nil{t.Fatal(err)}
 if replaced.Type!="e2ee_app_ping_result"||replaced.RemoteControlAvailable{
  t.Fatal("Relay supplied forged plaintext file write success")
 }
 resultingSHA,err:=remotetransport.OpenRemoteFileReplaceResult(client,
  decode(replaced.Ciphertext),strings.Repeat("e",32))
 updated,readErr:=os.ReadFile(filePath)
 actualSHA:=sha256.Sum256(updated)
 if err!=nil||readErr!=nil||string(updated)!="updated from trusted client"||
  resultingSHA!=hex.EncodeToString(actualSHA[:]){
  t.Fatalf("real TLS/WSS file replacement denied or tampered: %s %q %v %v",
   resultingSHA,updated,err,readErr)
 }
 // A second REAL WSS action must independently check host GitStatus RBAC,
 // even though PHP can sign an admin ticket and always return allowed=true.
 if err:=exec.Command("git","init","-q",taskdeck.Workspace).Run();err!=nil{t.Fatal(err)}
 gitFrame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:strings.Repeat("d",32),Operation:remotetransport.RemoteActionGitStatus,
 })
 if err!=nil{t.Fatal(err)}
 send(map[string]string{"type":"e2ee_app_ping",
  "ciphertext":base64.StdEncoding.EncodeToString(gitFrame)})
 var gitReply struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(receive("git_status"),&gitReply);err!=nil{t.Fatal(err)}
 if gitReply.Type!="e2ee_app_ping_result"||gitReply.RemoteControlAvailable{
  t.Fatal("Git status activated forbidden remote control privilege")
 }
 gitOutput,err:=remotetransport.OpenRemoteGitStatusResult(client,
  decode(gitReply.Ciphertext),strings.Repeat("d",32))
 if err!=nil||gitOutput==""{
  t.Fatalf("real E2EE Git status not authenticated: %q %v",gitOutput,err)
 }
 // One actual preapproved TaskDeck task must execute through host-local
 // TasksRun and be completely opaque to the hacked PHP ticket signer.
 taskJSON:=`{"version":"2.0.0","tasks":[{"label":"Remote WSS safe task",
  "type":"process","command":"/bin/echo",
  "args":["WSS_TASK_EXECUTION_PASS"],"taskdeckRemoteAllowed":true}]}`
 if err:=os.WriteFile(filepath.Join(taskdeck.Workspace,".vscode","tasks.json"),
  []byte(taskJSON),0600);err!=nil{t.Fatal(err)}
 allowedTasks,err:=tasks.Load(taskdeck.Workspace)
 if err!=nil||len(allowedTasks)!=1{t.Fatalf("TaskDeck host allowlist setup: %v",err)}
 token,err:=tasks.RemoteApprovalToken(allowedTasks[0])
 if err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS",token)
 taskFrame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:strings.Repeat("f",32),Operation:remotetransport.RemoteActionTaskRun,
  TaskID:allowedTasks[0].ID,
 })
 if err!=nil{t.Fatal(err)}
 send(map[string]string{"type":"e2ee_app_ping",
  "ciphertext":base64.StdEncoding.EncodeToString(taskFrame)})
 var taskReply struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(receive("task_run"),&taskReply);err!=nil{t.Fatal(err)}
 if taskReply.Type!="e2ee_app_ping_result"||taskReply.RemoteControlAvailable{
  t.Fatal("untrusted PHP falsely gained full TaskDeck command authority")
 }
 taskOutput,err:=remotetransport.OpenRemoteTaskRunResult(client,
  decode(taskReply.Ciphertext),strings.Repeat("f",32))
 if err!=nil||!strings.Contains(taskOutput,"WSS_TASK_EXECUTION_PASS"){
  t.Fatalf("authenticated bounded TaskDeck task denied over real WSS: %q %v",
   taskOutput,err)
 }
 // The sixth encrypted WSS exchange lists only approved workspace names.
 browseDir:=filepath.Join(taskdeck.Workspace,"public-e2ee-dir")
 if err:=os.Mkdir(browseDir,0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(browseDir,"sample.txt"),[]byte("ok"),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(browseDir,".hidden"),[]byte("private"),0600);err!=nil{t.Fatal(err)}
 directoryFrame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:strings.Repeat("a",32),
  Operation:remotetransport.RemoteActionProjectDirectoryList,
  Path:"public-e2ee-dir",
 })
 if err!=nil{t.Fatal(err)}
 send(map[string]string{"type":"e2ee_app_ping","ciphertext":base64.StdEncoding.EncodeToString(directoryFrame)})
 var directoryReply struct{
  Type string `json:"type"`
  Ciphertext string `json:"ciphertext"`
  RemoteControlAvailable bool `json:"remote_control_available"`
 }
 if err:=json.Unmarshal(receive("directory_list"),&directoryReply);err!=nil{t.Fatal(err)}
 if directoryReply.Type!="e2ee_app_ping_result"||directoryReply.RemoteControlAvailable{
  t.Fatal("unexpected remote directory response type")
 }
 listing,err:=remotetransport.OpenRemoteDirectoryListResult(client,
  decode(directoryReply.Ciphertext),strings.Repeat("a",32))
 if err!=nil||listing!="F\tsample.txt"{
  t.Fatalf("real TLS/WSS directory list failed: %q %v",listing,err)
 }
 // No command-shaped E2EE request can replace a typed session.ping.
 frame,err:=client.Seal([]byte(`{"v":1,"type":"shell.exec","command":"id"}`))
 if err!=nil{t.Fatal(err)}
 send(map[string]string{"type":"e2ee_app_ping","ciphertext":base64.StdEncoding.EncodeToString(frame)})
 // Host rejects this typed-command intrusion and closes its E2EE session.
 // The Relay may keep the browser socket until its bounded mailbox timeout;
 // its eventual response must NEVER be a TaskDeck success.
 readCtx,stopRead:=context.WithTimeout(ctx,5*time.Second)
 defer stopRead()
 _,data,e:=conn.Read(readCtx)
 if e==nil {
  // After six successful bounded E2EE actions, Relay may send an explicit
  // error for the exhausted mailbox instead of closing immediately. It
  // must never produce an application success/result or a bearer token.
  var rejected struct {
   Type string `json:"type"`
  }
  if json.Unmarshal(data,&rejected)!=nil||
   rejected.Type=="e2ee_app_ping_result"{
   t.Fatal("exhausted mailbox returned an unauthorized TaskDeck success")
  }
 }
}

func TestRealWSSBulkFileChunkWithCompromisedPHPStillRequiresTaskDeckAuth(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 taskdeck:=sharedLoginTestServer(t)
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
 // Bind the TaskDeck-local owner portal pin to the actual HTTPS test origin.
 // Production deliberately refuses E2EE app login without this host config.
 taskdeck.Config.RemotePortalURL=srv.URL
 defer srv.Close()
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
 receive:=func(stage string)[]byte{
  kind,bytes,e:=conn.Read(ctx)
  if e!=nil||kind!=websocket.MessageText{t.Fatalf("WSS %s reply: %v",stage,e)}
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
 if err:=json.Unmarshal(receive("agent_proof"),&proof);err!=nil{t.Fatal(err)}
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
 if err:=json.Unmarshal(receive("client_key_check"),&k);err!=nil{t.Fatal(err)}
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
 if err:=json.Unmarshal(receive("taskdeck_login"),&result);err!=nil{t.Fatal(err)}
 if result.Type!="e2ee_login_result"||result.RemoteControlAvailable {
  t.Fatal("Relay forged remote app authority")
 }
 authenticated,err:=remotetransport.OpenRemoteLoginResult(client,decode(result.Ciphertext))
 if err!=nil||!authenticated{t.Fatalf("local TaskDeck denied correct login: %v",err)}

 // Three real authenticated 4KiB reads through Browser->TLS Relay->Agent.
 // Compromised Portal signs admin tickets and grants access, but cannot
 // read ciphertext or bypass TaskDeck's in-process FilesRead principal.
 binaryData:=make([]byte,10000)
 for i:=range binaryData{binaryData[i]=byte(i%253)}
 if err:=os.WriteFile(filepath.Join(taskdeck.Workspace,"wss-bulk-chunk.bin"),
  binaryData,0600);err!=nil{t.Fatal(err)}
 hash:=sha256.Sum256(binaryData)
 for i,offset:=range []int64{0,4096,8192}{
  requestID:=strings.Repeat(string(rune('a'+i)),32)
  frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:requestID,
   Operation:remotetransport.RemoteActionProjectFileChunk,
   Path:"wss-bulk-chunk.bin",Offset:offset,
  })
  if err!=nil{t.Fatal(err)}
  send(map[string]string{
   "type":"e2ee_app_ping","ciphertext":base64.StdEncoding.EncodeToString(frame),
  })
  var reply struct{
   Type string `json:"type"`
   Ciphertext string `json:"ciphertext"`
   RemoteControlAvailable bool `json:"remote_control_available"`
  }
  if err:=json.Unmarshal(receive("bulk_file_chunk"),&reply);err!=nil{t.Fatal(err)}
  if reply.Type!="e2ee_app_ping_result"||reply.RemoteControlAvailable{
   t.Fatal("Portal/Relay forged larger application capability")
  }
  ciphertext:=decode(reply.Ciphertext)
  if len(ciphertext)>remotetransport.MaxEncryptedAppFrame||len(ciphertext)<25{
   t.Fatal("relay carried oversized E2EE binary reply")
  }
  window,err:=remotetransport.OpenRemoteFileChunkResult(client,ciphertext,requestID,offset)
  if err!=nil||window.Size!=int64(len(binaryData))||
   window.FileSHA256!=hex.EncodeToString(hash[:]){
   t.Fatalf("authenticated bulk result corrupted: %+v %v",window,err)
  }
  end:=offset+remotetransport.RemoteFileChunkMaxBytes
  if end>int64(len(binaryData)){end=int64(len(binaryData))}
  if !bytes.Equal(window.Data,binaryData[int(offset):int(end)]){
   t.Fatal("encrypted bulk file plaintext corrupted or truncated")
  }
 }
}
