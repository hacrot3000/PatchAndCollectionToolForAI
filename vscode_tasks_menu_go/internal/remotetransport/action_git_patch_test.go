package remotetransport

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitPatchBoundedUTF8AndMailbox(t *testing.T){
 // Include multi-byte graphemes, JSON-special chars and repeated newlines.
 source:=strings.Repeat("é <>& \\\" 🧪 "+string(rune(10)),38)
 offset,page:=0,1
 var accumulated strings.Builder
 for offset<len(source){
  got,end,err:=BoundRemoteGitPatchPage(testActionID,source,offset,page)
  if err!=nil||end<=offset||!ValidRemoteGitPatchPage(got){
   t.Fatalf("page %d invalid at %d: %+v %v",page,offset,got,err)
  }
  c,a,_:=participants(t)
  frame,err:=SealRemoteGitPatchPage(a,testActionID,got)
  if err!=nil{t.Fatalf("page %d did not fit E2EE mailbox: %v",page,err)}
  unpacked,err:=OpenRemoteGitPatchPage(c,frame,testActionID)
  if err!=nil||unpacked!=got{
   t.Fatalf("page %d altered in E2EE: %+v %+v %v",page,got,unpacked,err)
  }
  accumulated.WriteString(got.Patch)
  if got.More!=(end<len(source)){t.Fatalf("incorrect continuation bit on %d",page)}
  offset=end;page++
  if page>256{t.Fatal("unexpected unbounded number of pages")}
 }
 if accumulated.String()!=source{t.Fatal("UTF-8 pages lost or duplicated bytes")}
 for _,unsafe:=range []string{
  string([]byte{27}),string([]byte{0}),string([]byte{13}),
  string([]byte{0xff}),"\u202e",
 }{
  if _,_,err:=BoundRemoteGitPatchPage(testActionID,"valid"+unsafe,0,1);
   !errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("unsafe diff text accepted: %q %v",unsafe,err)
  }
 }
}
func TestRemoteGitPatchNativeFixedStartNext(t *testing.T){
 client,agent,_:=participants(t)
 seen:=0
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  req,err:=OpenRemoteAction(agent,frame)
  if err!=nil||req.TaskID!=0||req.FileIndex!=0||
   req.Snapshot!=""||req.Text!=""||req.Message!=""||
   req.TerminalInput!=""{return nil,ErrInvalidRemoteAction}
  seen++
  want:=RemoteActionGitPatchStart
  if seen==2{want=RemoteActionGitPatchNext}
  if req.Operation!=want{return nil,ErrInvalidRemoteAction}
  return SealRemoteGitPatchPage(agent,req.RequestID,
   RemoteGitPatchPage{Page:seen,Patch:"tracked change",More:seen==1})
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 first,err:=native.GitPatchStart(context.Background())
 if err!=nil||first.Page!=1||!first.More{t.Fatalf("start: %+v %v",first,err)}
 next,err:=native.GitPatchNext(context.Background())
 if err!=nil||next.Page!=2||next.More{t.Fatalf("next: %+v %v",next,err)}
}
func TestRemoteGitPatchMalformedE2EEAndNoOptions(t *testing.T){
 baseMap:=map[string]any{
  "type":RemoteGitPatchPageProtocol,
  "request_id":testActionID,"page":1,"patch":"safe","more":true,
  "remote_control_available":false,
 }
 raw,err:=json.Marshal(baseMap)
 if err!=nil{t.Fatal(err)}
 base:=string(raw)
 cases:=map[string]string{
  "replay":strings.Replace(base,testActionID,strings.Repeat("f",32),1),
  "unknown key":strings.Replace(base,"}",`,"path":"sensitive"}`,1),
  "duplicate key":strings.Replace(base,"}",`,"page":2}`,1),
  "wrong type":strings.Replace(base,RemoteGitPatchPageProtocol,"unexpected",1),
  "bad page":strings.Replace(base,`"page":1`,`"page":0`,1),
  "wrong bool":strings.Replace(base,`"more":true`,`"more":"true"`,1),
  "control":strings.Replace(base,"safe","safe\u001b",1),
  "escalation":strings.Replace(base,`"remote_control_available":false`,`"remote_control_available":true`,1),
 }
 for name,value:=range cases{
  t.Run(name,func(t *testing.T){
   c,a,_:=participants(t)
   frame,err:=a.Seal([]byte(value))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitPatchPage(c,frame,testActionID);
    !errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("forged GitPatch page accepted: %v",err)
   }
   if c.Established(){t.Fatal("forged patch record did not destroy channel")}
  })
 }
 for _,op:=range []string{RemoteActionGitPatchStart,RemoteActionGitPatchNext,
  RemoteActionGitPatchCachedStart,RemoteActionGitPatchCachedNext}{
  for _,request:=range []RemoteAction{
   {RequestID:testActionID,Operation:op,TaskID:1},
   {RequestID:testActionID,Operation:op,Text:"--cached"},
   {RequestID:testActionID,Operation:op,Message:"origin/main"},
   {RequestID:testActionID,Operation:op,FileIndex:1,Snapshot:"## main"},
  }{
   c,_,_:=participants(t)
   if _,err:=SealRemoteAction(c,request);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("Git patch accepted caller-controlled argument: %+v",request)
   }
  }
 }
}

// Staged review has its own fixed, no-argument E2EE action names and
// authenticates the same strict page envelope as the existing patch view.
func TestRemoteGitPatchCachedNativeStartNext(t *testing.T){
 client,agent,_:=participants(t)
 seen:=0
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  req,err:=OpenRemoteAction(agent,frame)
  if err!=nil||req.TaskID!=0||req.Text!=""||req.Message!=""||
   req.FileIndex!=0||req.Snapshot!=""||req.Choice!=0{
   return nil,ErrInvalidRemoteAction
  }
  seen++
  want:=RemoteActionGitPatchCachedStart
  if seen==2{want=RemoteActionGitPatchCachedNext}
  if req.Operation!=want{return nil,ErrInvalidRemoteAction}
  return SealRemoteGitPatchPage(agent,req.RequestID,
   RemoteGitPatchPage{Page:seen,Patch:"staged diff",More:seen==1})
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 first,err:=native.GitPatchCachedStart(context.Background())
 if err!=nil||first.Page!=1||!first.More{t.Fatalf("cached start: %+v %v",first,err)}
 next,err:=native.GitPatchCachedNext(context.Background())
 if err!=nil||next.Page!=2||next.More{t.Fatalf("cached next: %+v %v",next,err)}
}
