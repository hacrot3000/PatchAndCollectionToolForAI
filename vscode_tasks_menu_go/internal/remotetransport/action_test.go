package remotetransport

import (
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
