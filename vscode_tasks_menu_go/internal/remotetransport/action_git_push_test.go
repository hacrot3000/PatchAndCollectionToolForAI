package remotetransport

import (
 "bytes"
 "context"
 "errors"
 "strings"
 "testing"
)

const testGitPushHead="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestRemoteGitPushE2EEOnlyFixedOperation(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  if bytes.Contains(frame,[]byte(RemoteActionGitPush)){
   t.Fatal("Relay observed plaintext Git push")
  }
  req,err:=OpenRemoteAction(agent,frame)
  if err!=nil||req.Operation!=RemoteActionGitPush||
   req.TaskID!=0||req.Choice!=0||req.Text!=""||req.Message!=""{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteGitPushResult(agent,req.RequestID,RemoteGitPushResult{
   Head:testGitPushHead,
  })
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 result,err:=native.GitPush(context.Background())
 if err!=nil||result.Head!=testGitPushHead{
  t.Fatalf("wrong authenticated Git push outcome %+v: %v",result,err)
 }
}

func TestRemoteGitPushResultStrictSchema(t *testing.T){
 base:=`{"type":"taskdeck-e2ee-git-push-result-v1","request_id":"0123456789abcdef0123456789abcdef","head":"`+
  testGitPushHead+`","remote_control_available":false}`
 cases:=map[string]string{
  "wrong request":strings.Replace(base,testActionID,strings.Repeat("f",32),1),
  "extra force field":strings.Replace(base,`}`,`,"force":true}`,1),
  "remote control":strings.Replace(base,`"remote_control_available":false`,`"remote_control_available":true`,1),
  "upper SHA":strings.Replace(base,testGitPushHead,strings.ToUpper(testGitPushHead),1),
  "no head":strings.Replace(base,`"head":"`+testGitPushHead+`",`,"",1),
  "duplicate":strings.Replace(base,`}`,`,"head":"`+testGitPushHead+`"}`,1),
 }
 for name,raw:=range cases{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   record,err:=agent.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitPushResult(client,record,testActionID);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("malformed Git push reply accepted: %v",err)
   }
   if client.Established(){t.Fatal("malformed reply did not poison E2EE")}
  })
 }
}

func TestRemoteGitPushRejectsClientSuppliedArgs(t *testing.T){
 for _,request:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionGitPush,TaskID:1},
  {RequestID:testActionID,Operation:RemoteActionGitPush,Choice:1},
  {RequestID:testActionID,Operation:RemoteActionGitPush,Text:"force"},
  {RequestID:testActionID,Operation:RemoteActionGitPush,Message:"refs/heads/main"},
 }{
  client,_,_:=participants(t)
  if _,err:=SealRemoteAction(client,request);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("push accepted an arbitrary argument: %+v",request)
  }
 }
}
