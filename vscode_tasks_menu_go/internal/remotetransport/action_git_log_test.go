package remotetransport

import (
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitLogNativeE2EEFixedOperation(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(agent,frame)
  if err!=nil||request.Operation!=RemoteActionGitLog||request.TaskID!=0||
   request.Choice!=0||request.Text!=""||request.Message!=""{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteGitLogResult(agent,request.RequestID,"abcdef Recent commit")
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 result,err:=native.GitLog(context.Background())
 if err!=nil||result!="abcdef Recent commit"{
  t.Fatalf("invalid E2EE git log: %q %v",result,err)
 }
}

func TestRemoteGitLogRejectsMalformedResult(t *testing.T){
 base:=`{"type":"taskdeck-e2ee-git-log-result-v1","request_id":"0123456789abcdef0123456789abcdef","log":"abcd test","remote_control_available":false}`
 tests:=map[string]string{
  "wrong id":strings.Replace(base,testActionID,strings.Repeat("f",32),1),
  "extra path":strings.Replace(base,`}`,`,"path":"../secret"}`,1),
  "duplicate log":strings.Replace(base,`}`,`,"log":"forged"}`,1),
  "remote control":strings.Replace(base,`"remote_control_available":false`,`"remote_control_available":true`,1),
  "newline control":strings.Replace(base,"abcd test",`abc\u0000test`,1),
  "too long":strings.Replace(base,"abcd test",strings.Repeat("X",241),1),
 }
 for name,raw:=range tests{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=agent.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitLogResult(client,frame,testActionID);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("invalid git log record admitted: %v",err)
   }
   if client.Established(){t.Fatal("invalid git log record did not close channel")}
  })
 }
}
func TestRemoteGitLogCannotSelectArbitraryArgs(t *testing.T){
 for _,request:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionGitLog,TaskID:1},
  {RequestID:testActionID,Operation:RemoteActionGitLog,Text:"--all"},
  {RequestID:testActionID,Operation:RemoteActionGitLog,Message:"branch"},
 }{
  client,_,_:=participants(t)
  if _,err:=SealRemoteAction(client,request);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("git.log accepted remote override: %+v",request)
  }
 }
}
