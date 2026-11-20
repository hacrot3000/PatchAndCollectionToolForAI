package remotetransport

import (
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitFetchNativeAuthenticatedOnly(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(agent,frame)
  if err!=nil||request.Operation!=RemoteActionGitFetch||
   request.TaskID!=0||request.Choice!=0||
   request.Text!=""||request.Message!=""||request.TerminalInput!=""{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteGitFetchResult(agent,request.RequestID,RemoteGitFetchResult{
   Head:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
   Before:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
   After:"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",Updated:true,
  })
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 result,err:=native.GitFetch(context.Background())
 if err!=nil||!result.Updated||
  result.Head!="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"||
  result.After!="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"{
  t.Fatalf("invalid fixed authenticated fetch: %+v %v",result,err)
 }
}

func TestRemoteGitFetchRejectsMalformedOrEscalatedResult(t *testing.T){
 const a="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
 const d="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
 base:=`{"type":"taskdeck-e2ee-git-fetch-result-v1","request_id":"0123456789abcdef0123456789abcdef","head":"`+a+`","before":"`+a+`","after":"`+d+`","updated":true,"remote_control_available":false}`
 tests:=map[string]string{
  "replay":strings.Replace(base,testActionID,strings.Repeat("f",32),1),
  "stale updated":strings.Replace(base,`"updated":true`,`"updated":false`,1),
  "bad object id":strings.Replace(base,d,strings.ToUpper(d),1),
  "extra url":strings.Replace(base,`}`,`,"url":"https://evil.test"}`,1),
  "duplicate key":strings.Replace(base,`}`,`,"after":"`+d+`"}`,1),
  "escalation":strings.Replace(base,`"remote_control_available":false`,`"remote_control_available":true`,1),
  "null head":strings.Replace(base,`"head":"`+a+`"`,`"head":null`,1),
 }
 for name,raw:=range tests{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=agent.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitFetchResult(client,frame,testActionID);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("malformed fetch result admitted: %v",err)
   }
   if client.Established(){t.Fatal("malformed E2EE fetch reply left channel alive")}
  })
 }
}

func TestRemoteGitFetchForbidsClientSelectedArgs(t *testing.T){
 for _,request:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionGitFetch,TaskID:1},
  {RequestID:testActionID,Operation:RemoteActionGitFetch,Choice:1},
  {RequestID:testActionID,Operation:RemoteActionGitFetch,Text:"--all"},
  {RequestID:testActionID,Operation:RemoteActionGitFetch,Message:"origin/main"},
  {RequestID:testActionID,Operation:RemoteActionGitFetch,TerminalInput:"git fetch"},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,request);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("Git fetch accepted client-supplied argument %+v",request)
  }
 }
}
