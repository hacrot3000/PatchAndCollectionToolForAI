package remotetransport

import (
 "crypto/sha256"
 "encoding/hex"
 "strings"
 "bytes"
 "errors"
 "strings"
 "testing"
)

const testActionID = "0123456789abcdef0123456789abcdef"

func TestRemoteActionE2EEPingOnlyRoundTrip(t *testing.T) {
 client,agent,_:=participants(t)
 action:=RemoteAction{RequestID:testActionID,Operation:RemoteActionSessionPing}
 frame,err:=SealRemoteAction(client,action)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(frame,[]byte(RemoteActionProtocol)){
  t.Fatal("Relay can see authenticated action plaintext")
 }
 opened,err:=OpenRemoteAction(agent,frame)
 if err!=nil || opened!=action {t.Fatalf("verified ping mismatch: %+v %v",opened,err)}
 sealed,err:=SealRemoteActionPingResult(agent,opened.RequestID)
 if err!=nil{t.Fatal(err)}
 if err:=OpenRemoteActionPingResult(client,sealed,testActionID);err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(agent,frame);err==nil {
  t.Fatal("replayed authenticated action was accepted")
 }
 if agent.Established(){t.Fatal("E2EE replay did not revoke session")}
}

func TestRemoteActionRejectsArbitraryOperationsAndUnknownWireFields(t *testing.T){
 cases:=map[string]string{
  "terminal":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":\"terminal.exec\"}",
  "file write":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":\"files.write\"}",
  "HTTP route":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":\"/api/tasks/run\"}",
  "extra command":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":\"session.ping\",\"command\":\"rm -rf /\"}",
  "duplicate op":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":\"session.ping\",\"operation\":\"terminal.exec\"}",
  "duplicate same":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":\"session.ping\",\"operation\":\"session.ping\"}",
  "case folding":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"Operation\":\"session.ping\"}",
  "missing ID":"{\"type\":\"taskdeck-e2ee-action-v1\",\"operation\":\"session.ping\"}",
  "uppercase ID":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"ABCDEF0123456789ABCDEF0123456789\",\"operation\":\"session.ping\"}",
  "login type":"{\"type\":\"taskdeck-e2ee-login-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":\"session.ping\"}",
  "trailing":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":\"session.ping\"} true",
  "type confusion":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":{\"run\":\"terminal\"}}",
  "args object":"{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"operation\":\"session.ping\",\"args\":{}}",
  "invalid JSON":"{\"type\":",
 }
 for name,clear:=range cases {
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=client.Seal([]byte(clear))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteAction(agent,frame);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("malicious action accepted: %v",err)
   }
   if agent.Established(){t.Fatal("bad authenticated envelope left Agent session alive")}
  })
 }
}

func TestRemoteActionResponseRejectsExtraFieldsOrForgedSessionCapability(t *testing.T){
 cases:=[]string{
  "{\"type\":\"taskdeck-e2ee-action-result-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"ok\":true,\"remote_control_available\":true}",
  "{\"type\":\"taskdeck-e2ee-action-result-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"ok\":true,\"remote_control_available\":false,\"access_token\":\"forged\"}",
  "{\"type\":\"taskdeck-e2ee-action-result-v1\",\"request_id\":\"0123456789abcdef0123456789abcdef\",\"ok\":true,\"ok\":true,\"remote_control_available\":false}",
  "{\"type\":\"taskdeck-e2ee-action-result-v1\",\"request_id\":\"fedcba9876543210fedcba9876543210\",\"ok\":true,\"remote_control_available\":false}",
 }
 for _,clear:=range cases {
  client,agent,_:=participants(t)
  frame,err:=agent.Seal([]byte(clear))
  if err!=nil{t.Fatal(err)}
  if err:=OpenRemoteActionPingResult(client,frame,testActionID);err==nil{
   t.Fatalf("client accepted fake ping capability: %s",clear)
  }
  if client.Established(){t.Fatal("malformed action response failed to poison client")}
 }
}

func TestRemoteActionClientCannotForgeUnreviewedOperation(t *testing.T){
 client,_,_:=participants(t)
 for _,op:=range []string{"terminal.exec","files.write","tasks.run","git.push","patch.apply","remote_all"}{
  if _,err:=SealRemoteAction(client,RemoteAction{RequestID:testActionID,Operation:op});err==nil{
   t.Fatal("unreviewed operation accepted: "+op)
  }
 }
 if _,err:=SealRemoteAction(client,RemoteAction{RequestID:strings.Repeat("A",32),Operation:RemoteActionSessionPing});err==nil{
  t.Fatal("ambiguous noncanonical request id accepted")
 }
}

// Project File reads are the first REAL TaskDeck data operation exposed over
// WSS. Verify exact strict message type, request identity and encrypted hash.
// A PHP/Relay cannot alter the path or file body without AEAD failure.
func TestRemoteProjectFileReadStrictAEADRoundTrip(t *testing.T){
 browser,agent,_:=participants(t)
 request,err:=SealRemoteAction(browser,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionProjectFileRead,Path:"docs/notes.txt",
 })
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteAction(agent,request)
 if err!=nil||got.Operation!=RemoteActionProjectFileRead||
  got.Path!="docs/notes.txt"||got.RequestID!=testActionID{
  t.Fatalf("incorrect authenticated file operation: %+v %v",got,err)
 }
 reply,err:=SealRemoteFileReadResult(agent,testActionID,[]byte("hello remote"))
 if err!=nil{t.Fatal(err)}
 text,err:=OpenRemoteFileReadResult(browser,reply,testActionID)
 if err!=nil||text!="hello remote"{t.Fatalf("file bytes changed across E2EE: %q %v",text,err)}
}

func TestRemoteFileReadCannotEscapeWorkspaceOrImpersonateShell(t *testing.T){
 for _,path:=range []string{"../secret","docs/../../secret","/etc/passwd","x\\y","a..b",".","", "C:/secret"}{
  t.Run(path,func(t *testing.T){
   browser,_,_:=participants(t)
   frame,err:=SealRemoteAction(browser,RemoteAction{
    RequestID:testActionID,Operation:RemoteActionProjectFileRead,Path:path,
   })
   if err==nil||len(frame)!=0{t.Fatalf("unsafe file path accepted: %q",path)}
  })
 }
 for _,clear:=range []string{
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.read"}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"session.ping","path":"foo"}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.read","path":"docs/x","command":"id"}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"terminal.exec","path":"docs/x"}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.read","path":"docs/x","path":"docs/y"}`,
 }{
  browser,agent,_:=participants(t)
  frame,err:=browser.Seal([]byte(clear));if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteAction(agent,frame);err==nil||agent.Established(){
   t.Fatalf("forged command or ambiguous JSON accepted: %q",clear)
  }
 }
}

    
func TestRemoteFileReadResultRefusesForgedSessionCookieOrWrongRequest(t *testing.T){
 t.Run("wrong request ID",func(t *testing.T){
  browser,agent,_:=participants(t)
  reply,err:=SealRemoteFileReadResult(agent,testActionID,[]byte("sensitive"))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteFileReadResult(browser,reply,"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");err==nil||
   browser.Established(){t.Fatal("Relay swapped an encrypted file-read reply")}
 })
 t.Run("forbidden bearer token field",func(t *testing.T){
  browser,agent,_:=participants(t)
  reply,err:=agent.Seal([]byte(`{"type":"taskdeck-e2ee-file-read-result-v1","request_id":"0123456789abcdef0123456789abcdef","ok":true,"remote_control_available":false,"content":"x","sha256":"00","token":"stolen"}`))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteFileReadResult(browser,reply,testActionID);err==nil||
   browser.Established(){t.Fatal("untrusted Relay manufactured a token from E2EE result")}
 })
}

    
func TestEncryptedGitStatusTypedAndCannotUpgradeToGitWrite(t *testing.T){
 browser,agent,_:=participants(t)
 frame,err:=SealRemoteAction(browser,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionGitStatus,
 })
 if err!=nil{t.Fatal(err)}
 action,err:=OpenRemoteAction(agent,frame)
 if err!=nil||action.Operation!=RemoteActionGitStatus||action.Path!=""{t.Fatalf("Git status AEAD: %+v %v",action,err)}
 response,err:=SealRemoteGitStatusResult(agent,testActionID,[]byte("## main\n"))
 if err!=nil{t.Fatal(err)}
 status,err:=OpenRemoteGitStatusResult(browser,response,testActionID)
 if err!=nil||status!="## main\n"{t.Fatalf("Git status AEAD reply: %q %v",status,err)}
 for _,op:=range []string{"git.push","git.fetch","git.exec","terminal.exec","task.run"}{
  client,_,_:=participants(t)
  if frame,err:=SealRemoteAction(client,RemoteAction{
    RequestID:testActionID,Operation:op,
   });err==nil||len(frame)>0 {t.Fatalf("unapproved command accepted: %q",op)}
 }
}
func TestGitStatusE2EERejectsAppResultTypeSwaps(t *testing.T){
 browser,agent,_:=participants(t)
 reply,err:=SealRemoteGitStatusResult(agent,testActionID,[]byte("## trunk\n"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteFileReadResult(browser,reply,testActionID);err==nil||browser.Established(){
  t.Fatal("compromised Relay changed Git response into file response")
 }
}

    
func TestE2EEProjectFileReplacementExactSchemaAndHash(t *testing.T){
 browser,agent,_:=participants(t)
 before:=sha256.Sum256([]byte("initial"))
 oldSHA:=hex.EncodeToString(before[:])
 request,err:=SealRemoteAction(browser,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionProjectFileReplace,
  Path:"notes.txt",Content:"modified",ExpectedSHA256:oldSHA,
 })
 if err!=nil{t.Fatal(err)}
 opened,err:=OpenRemoteAction(agent,request)
 if err!=nil||opened.Operation!=RemoteActionProjectFileReplace||
  opened.Path!="notes.txt"||opened.Content!="modified"||
  opened.ExpectedSHA256!=oldSHA{t.Fatalf("invalid authenticated file mutation: %+v %v",opened,err)}
 next:=sha256.Sum256([]byte("modified"))
 ack,err:=SealRemoteFileReplaceResult(agent,testActionID,hex.EncodeToString(next[:]))
 if err!=nil{t.Fatal(err)}
 hash,err:=OpenRemoteFileReplaceResult(browser,ack,testActionID)
 if err!=nil||hash!=hex.EncodeToString(next[:]){
  t.Fatalf("encrypted replacement ACK incorrect: %s %v",hash,err)
 }
 for _,bad:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionProjectFileReplace,Path:"../../secret",
   Content:"overwrite",ExpectedSHA256:oldSHA},
  {RequestID:testActionID,Operation:RemoteActionProjectFileReplace,Path:"notes.txt",
   Content:"overwrite",ExpectedSHA256:""},
  {RequestID:testActionID,Operation:RemoteActionProjectFileReplace,Path:"notes.txt",
   Content:strings.Repeat("a",97),ExpectedSHA256:oldSHA},
  {RequestID:testActionID,Operation:RemoteActionProjectFileReplace,Path:"notes.txt",
   Content:"hello\x00world",ExpectedSHA256:oldSHA},
 }{
  client,_,_:=participants(t)
  if frame,err:=SealRemoteAction(client,bad);err==nil||len(frame)>0{
   t.Fatalf("unsafe replace request accepted: %+v",bad)
  }
 }
 for _,body:=range []string{
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.replace","path":"notes.txt","content":"x"}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.replace","path":"notes.txt","content":"x","expected_sha256":"x","command":"id"}`,
  `{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"project.file.replace","path":"notes.txt","content":"x","content":"y","expected_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
 }{
  client,server,_:=participants(t)
  frame,err:=client.Seal([]byte(body));if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteAction(server,frame);err==nil||server.Established(){
   t.Fatalf("tampered file replace accepted: %s",body)
  }
 }
}
