package remotetransport

import(
 "context"
 "errors"
 "strings"
 "testing"
)

func trackingTestSnapshot()string{
 oid:=strings.Repeat("a",40)
 return "GIT_TRACKING_V1\n"+
  "origin/a "+oid+"\n"+
  "origin/b "+oid+"\n"+
  "origin/c "+oid+"\n"+
  "origin/d "+oid+"\n"+
  "origin/e "+oid+"\n"+
  "origin/f "+oid+"\n"+
  "origin/g "+oid+"\n"+
  "origin/h "+oid+"\n"
}
func TestRemoteGitTrackingPagesAreBoundedAndComplete(t *testing.T){
 snapshot:=trackingTestSnapshot()
 if !ValidRemoteGitTrackingSnapshot(snapshot){t.Fatal("valid snapshot refused")}
 offset,page:=0,1
 var merged strings.Builder
 for {
  v,end,err:=BoundRemoteGitTrackingPage(testActionID,snapshot,offset,page)
  if err!=nil||end<=offset{t.Fatalf("page %d: %+v %v",page,v,err)}
  merged.WriteString(v.Refs)
  if !v.More{break}
  offset=end;page++
  if page>16{t.Fatal("too many page rounds")}
 }
 if page<2||merged.String()!=snapshot{
  t.Fatalf("incomplete paginated tracking refs: pages=%d text=%q",page,merged.String())
 }
 for _,s:=range []string{
  "GIT_TRACKING_V1\norigin/b "+strings.Repeat("a",40)+"\norigin/a "+strings.Repeat("a",40)+"\n",
  "GIT_TRACKING_V1\norigin/main https://token\n",
  "GIT_TRACKING_V1\norigin/.lock "+strings.Repeat("a",40)+"\n",
  "GIT_TRACKING_V1\norigin/HEAD "+strings.Repeat("a",40)+"\n",
  "GIT_TRACKING_V1\norigin/a "+strings.Repeat("A",40)+"\n",
  "GIT_TRACKING_V1\norigin/a "+strings.Repeat("a",39)+"\n",
 }{
  if ValidRemoteGitTrackingSnapshot(s){t.Fatalf("invalid tracking snapshot accepted: %q",s)}
 }
}
func TestRemoteGitTrackingNativeRoundTripAndNoArguments(t *testing.T){
 client,agent,_:=participants(t)
 var count int
 source:=trackingTestSnapshot()
 offset,page:=0,1
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  req,err:=OpenRemoteAction(agent,frame)
  want:=RemoteActionGitTrackingStart
  if count>0{want=RemoteActionGitTrackingNext}
  if err!=nil||req.Operation!=want||req.TaskID!=0||req.Choice!=0||
   req.Text!=""||req.Message!=""||req.FileIndex!=0||req.Snapshot!=""{
   return nil,ErrInvalidRemoteAction
  }
  part,end,err:=BoundRemoteGitTrackingPage(req.RequestID,source,offset,page)
  if err!=nil{return nil,err}
  offset=end;page++;count++
  return SealRemoteGitTrackingPage(agent,req.RequestID,part)
 })
 p,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer p.Close()
 first,err:=p.GitTrackingStart(context.Background())
 if err!=nil||!first.More||first.Page!=1{t.Fatalf("missing first page: %+v %v",first,err)}
 data:=first.Refs
 for first.More{
  first,err=p.GitTrackingNext(context.Background())
  if err!=nil{t.Fatal(err)}
  data+=first.Refs
 }
 if data!=source||count<2{t.Fatalf("paginated native reply mismatch after %d calls",count)}
 for _,req:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionGitTrackingStart,Message:"origin"},
  {RequestID:testActionID,Operation:RemoteActionGitTrackingNext,TaskID:2},
  {RequestID:testActionID,Operation:RemoteActionGitTrackingNext,Text:"../../secret"},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,req);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("caller selected Git argv or ref: %+v",req)
  }
 }
}
func TestRemoteGitTrackingRejectsForgedResult(t *testing.T){
 base:=`{"type":"taskdeck-e2ee-git-tracking-page-v1","request_id":"0123456789abcdef0123456789abcdef","page":1,"refs":"GIT_TRACKING_V1\n","more":false,"remote_control_available":false}`
 cases:=[]string{
  strings.Replace(base,testActionID,strings.Repeat("f",32),1),
  strings.Replace(base,`"more":false`,`"more":true`,1),
  strings.Replace(base,`"page":1`,`"page":null`,1),
  strings.Replace(base,`"remote_control_available":false`,`"remote_control_available":true`,1),
  strings.Replace(base,"}",`,"url":"https://token"}`,1),
  strings.Replace(base,"}",`,"refs":"GIT_TRACKING_V1"}`,1),
 }
 for _,raw:=range cases{
  c,a,_:=participants(t)
  frame,err:=a.Seal([]byte(raw))
  if err!=nil{t.Fatal(err)}
  if _,err=OpenRemoteGitTrackingPage(c,frame,testActionID);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("forged result accepted: %s",raw)
  }
  if c.Established(){t.Fatal("forged metadata left E2EE live")}
 }
}
