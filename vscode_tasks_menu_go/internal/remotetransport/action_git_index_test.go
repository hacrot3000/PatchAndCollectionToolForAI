package remotetransport

import (
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitIndexNativeFixedActions(t *testing.T){
 for _,op:=range []string{RemoteActionGitStageTracked,RemoteActionGitUnstageAll}{
  t.Run(op,func(t *testing.T){
   client,agent,_:=participants(t)
   exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
    request,err:=OpenRemoteAction(agent,frame)
    if err!=nil||request.Operation!=op||request.TaskID!=0||
     request.Choice!=0||request.Text!=""||request.Message!=""||
     request.TerminalInput!=""{
     return nil,ErrInvalidRemoteAction
    }
    return SealRemoteGitIndexResult(agent,request.RequestID,RemoteGitIndexResult{
     Operation:op,Head:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    })
   })
   native,err:=NewPingClient(client,exchange)
   if err!=nil{t.Fatal(err)}
   defer native.Close()
   var result RemoteGitIndexResult
   if op==RemoteActionGitStageTracked{
    result,err=native.GitStageTracked(context.Background())
   }else{
    result,err=native.GitUnstageAll(context.Background())
   }
   if err!=nil||result.Operation!=op||
    result.Head!="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"{
    t.Fatalf("wrong authenticated Git index action: %+v %v",result,err)
   }
  })
 }
}

func TestRemoteGitIndexStrictResultRejectsForgery(t *testing.T){
 const id=testActionID
 base:=`{"type":"taskdeck-e2ee-git-index-result-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.stage_tracked","head":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","remote_control_available":false}`
 cases:=map[string]string{
  "other operation":strings.Replace(base,RemoteActionGitStageTracked,RemoteActionGitUnstageAll,1),
  "wrong id":strings.Replace(base,id,strings.Repeat("f",32),1),
  "extra filename":strings.Replace(base,`}`,`,"filename":"README.md"}`,1),
  "duplicate operation":strings.Replace(base,`}`,`,"operation":"git.stage_tracked"}`,1),
  "uppercase head":strings.Replace(base,strings.Repeat("a",40),strings.Repeat("A",40),1),
  "capability escalation":strings.Replace(base,`"remote_control_available":false`,`"remote_control_available":true`,1),
  "null head":strings.Replace(base,`"head":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,`"head":null`,1),
 }
 for name,raw:=range cases{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=agent.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitIndexResult(client,frame,id,RemoteActionGitStageTracked);
    !errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("untrusted Git staging reply accepted: %v",err)
   }
   if client.Established(){t.Fatal("malformed Git index record did not close E2EE")}
  })
 }
}

func TestRemoteGitIndexRejectsCallerArguments(t *testing.T){
 for _,op:=range []string{RemoteActionGitStageTracked,RemoteActionGitUnstageAll}{
  for _,request:=range []RemoteAction{
   {RequestID:testActionID,Operation:op,TaskID:1},
   {RequestID:testActionID,Operation:op,Choice:1},
   {RequestID:testActionID,Operation:op,Text:"README.md"},
   {RequestID:testActionID,Operation:op,Message:"HEAD~1"},
   {RequestID:testActionID,Operation:op,TerminalInput:"git add -A"},
  }{
   client,_,_:=participants(t)
   if _,err:=SealRemoteAction(client,request);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("Git staging accepted client option: %+v",request)
   }
  }
 }
}
