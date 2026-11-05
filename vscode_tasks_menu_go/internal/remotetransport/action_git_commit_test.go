package remotetransport

import (
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitCommitNativeE2EEAndMessageOnly(t *testing.T){
 client,agent,_:=participants(t)
 const message="Sửa lỗi chuyển trạng thái"
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  action,err:=OpenRemoteAction(agent,frame)
  if err!=nil||action.Operation!=RemoteActionGitCommit||
   action.Message!=message||action.TaskID!=0||action.Choice!=0||action.Text!=""{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteGitCommitResult(agent,action.RequestID,RemoteGitCommitResult{
   Before:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
   After:"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  })
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 result,err:=native.GitCommit(context.Background(),message)
 if err!=nil||result.Before==result.After||result.After!="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"{
  t.Fatalf("invalid authenticated commit result: %+v %v",result,err)
 }
}

func TestRemoteGitCommitMessageValidationAndSchema(t *testing.T){
 valid:=[]string{"Fix issue", "Sửa lỗi", strings.Repeat("<",120)}
 for _,msg:=range valid{
  if !ValidRemoteGitCommitMessage(msg){t.Fatalf("valid commit message refused: %q",msg)}
  client,agent,_:=participants(t)
  frame,err:=SealRemoteAction(client,RemoteAction{
   RequestID:testActionID,Operation:RemoteActionGitCommit,Message:msg,
  })
  if err!=nil{t.Fatal(err)}
  opened,err:=OpenRemoteAction(agent,frame)
  if err!=nil||opened.Message!=msg{t.Fatalf("commit body corrupted: %+v %v",opened,err)}
 }
 invalid:=[]string{"", " leading","trailing ", "Line\nBreak",strings.Repeat("x",121), "invisible\u200bmark"}
 for _,msg:=range invalid{
  if ValidRemoteGitCommitMessage(msg){t.Fatalf("invalid message accepted: %q",msg)}
 }
 requestCases:=map[string]string{
  "no message":`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.commit"}`,
  "extra task":`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.commit","message":"Fix","task_id":12}`,
  "duplicate message":`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.commit","message":"Fix","message":"Malicious"}`,
  "unknown key":`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.commit","message":"Fix","shell":"yes"}`,
  "null message":`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.commit","message":null}`,
  "message on status":`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.status","message":"Fix"}`,
 }
 for name,raw:=range requestCases{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=client.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteAction(agent,frame);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("malformed Git mutation admitted: %v",err)
   }
   if agent.Established(){t.Fatal("malformed command did not poison Agent E2EE channel")}
  })
 }
}

func TestRemoteGitCommitResultRejectsMalformedResponse(t *testing.T){
 const before="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
 const after="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
 base:=`{"type":"taskdeck-e2ee-git-commit-result-v1","request_id":"0123456789abcdef0123456789abcdef","before":"`+
  before+`","after":"`+after+`","remote_control_available":false}`
 tests:=map[string]string{
  "wrong request":strings.Replace(base,testActionID,strings.Repeat("f",32),1),
  "unchanged HEAD":strings.Replace(base,after,before,1),
  "forged role":strings.Replace(base,`"remote_control_available":false`,`"remote_control_available":true`,1),
  "unknown key":strings.Replace(base,`}`,`,"command":"rm -rf /"}`,1),
  "duplicate key":strings.Replace(base,`}`,`,"before":"`+before+`"}`,1),
  "uppercase SHA":strings.Replace(base,before,strings.ToUpper(before),1),
 }
 for name,raw:=range tests{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=agent.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitCommitResult(client,frame,testActionID);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("untrusted commit response accepted: %v",err)
   }
   if client.Established(){t.Fatal("invalid commit response did not poison E2EE")}
  })
 }
}
