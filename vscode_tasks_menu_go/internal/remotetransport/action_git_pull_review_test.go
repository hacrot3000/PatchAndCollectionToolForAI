package remotetransport

import (
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitPullReviewNativeE2EEAndNoCallerArguments(t *testing.T){
 client,agent,_:=participants(t)
 before,after:=strings.Repeat("a",40),strings.Repeat("b",40)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  req,err:=OpenRemoteAction(agent,frame)
  if err!=nil||req.Operation!=RemoteActionGitPullReview||
   req.TaskID!=0||req.Choice!=0||req.Text!=""||
   req.Message!=""||req.FileIndex!=0||req.Snapshot!=""||
   req.TerminalInput!=""||req.Cols!=0||req.Rows!=0{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteGitPullReviewResult(agent,req.RequestID,
   RemoteGitPullReviewResult{Head:before,UpstreamHead:after,Status:"fast-forward"})
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 value,err:=native.GitPullReview(context.Background())
 if err!=nil||value.Head!=before||value.UpstreamHead!=after||value.Status!="fast-forward"{
  t.Fatalf("fixed offline pull review was not authenticated: %+v %v",value,err)
 }
 for _,req:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionGitPullReview,Text:"origin/main"},
  {RequestID:testActionID,Operation:RemoteActionGitPullReview,Message:"git pull --force"},
  {RequestID:testActionID,Operation:RemoteActionGitPullReview,TaskID:1},
  {RequestID:testActionID,Operation:RemoteActionGitPullReview,FileIndex:1,Snapshot:"status"},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,req);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("client supplied an arbitrary Git ref/command: %+v",req)
  }
 }
}
func TestRemoteGitPullReviewStrictResultAndPoisonedChannel(t *testing.T){
 oid:=strings.Repeat("a",40)
 next:=strings.Repeat("b",40)
 for _,v:=range []RemoteGitPullReviewResult{
  {Head:oid,UpstreamHead:oid,Status:"up-to-date"},
  {Head:oid,UpstreamHead:next,Status:"fast-forward"},
 }{
  if !validRemoteGitPullReviewResult(v){t.Fatalf("valid read result denied: %+v",v)}
 }
 for _,v:=range []RemoteGitPullReviewResult{
  {Head:oid,UpstreamHead:oid,Status:"fast-forward"},
  {Head:oid,UpstreamHead:next,Status:"up-to-date"},
  {Head:oid,UpstreamHead:next,Status:"diverged"},
  {Head:oid,UpstreamHead:next,Status:"updated"},
  {Head:"https://secret",UpstreamHead:next,Status:"fast-forward"},
  {Head:strings.Repeat("A",40),UpstreamHead:next,Status:"fast-forward"},
 }{
  if validRemoteGitPullReviewResult(v){t.Fatalf("invalid preflight accepted: %+v",v)}
 }
 base:=`{"type":"taskdeck-e2ee-git-pull-review-v1","request_id":"0123456789abcdef0123456789abcdef","head":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","upstream_head":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","status":"fast-forward","remote_control_available":false}`
 cases:=map[string]string{
  "wrong id":strings.Replace(base,testActionID,strings.Repeat("f",32),1),
  "duplicate":strings.Replace(base,"}",`,"head":"forged"}`,1),
  "URL":strings.Replace(base,"}",`,"remote_url":"https://token"}`,1),
  "escalation":strings.Replace(base,`"remote_control_available":false`,`"remote_control_available":true`,1),
  "null":strings.Replace(base,`"upstream_head":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`,`"upstream_head":null`,1),
  "diverged":strings.Replace(base,`"status":"fast-forward"`,`"status":"diverged"`,1),
  "same HEAD":strings.Replace(base,strings.Repeat("b",40),strings.Repeat("a",40),1),
 }
 for name,raw:=range cases{
  t.Run(name,func(t *testing.T){
   c,a,_:=participants(t)
   frame,err:=a.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitPullReviewResult(c,frame,testActionID);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("malformed Git pull preview accepted: %v",err)
   }
   if c.Established(){t.Fatal("malformed pull preview left E2EE channel established")}
  })
 }
}
