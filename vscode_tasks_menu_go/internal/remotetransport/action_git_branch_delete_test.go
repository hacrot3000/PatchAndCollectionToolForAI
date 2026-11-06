package remotetransport

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitDeleteStrictCodecAndNoCallerRef(t *testing.T){
 head:=strings.Repeat("a",40)
 plan:=RemoteGitBranchDeleteResult{
  Current:"feature/old",Branch:"taskdeck/safe-target",Head:head,
 }
 if !ValidRemoteGitBranchDeleteResult(plan){t.Fatal("safe host branch refused")}
 c,a,_:=participants(t)
 frame,err:=SealRemoteGitBranchDeleteResult(a,testActionID,plan)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteGitBranchDeleteResult(c,frame,testActionID)
 if err!=nil||got!=plan{t.Fatalf("sealed branch plan: %+v %v",got,err)}
 for _,invalid:=range []RemoteGitBranchDeleteResult{
  {Current:"HEAD",Branch:plan.Branch,Head:head},
  {Current:"main",Branch:"main",Head:head},
  {Current:"main",Branch:"taskdeck/main",Head:head},
  {Current:"main",Branch:"taskdeck/../secret",Head:head},
  {Current:"main",Branch:"taskdeck/a.lock",Head:head},
  {Current:"main",Branch:"taskdeck/a..b",Head:head},
  {Current:"main",Branch:"taskdeck/a/b",Head:head},
  {Current:"main",Branch:"taskdeck/.hidden",Head:head},
  {Current:"main",Branch:"taskdeck/branch",Head:strings.Repeat("A",40)},
  {Current:"main",Branch:"taskdeck/branch",Head:"not-an-oid"},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteGitBranchDeleteResult(c,testActionID,invalid);
   !errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("forged branch deletion result accepted: %+v %v",invalid,err)
  }
 }
 plain,err:=json.Marshal(remoteGitBranchDeleteWire{
  Type:RemoteGitBranchDeleteProtocol,RequestID:testActionID,
  Current:"main",Branch:"taskdeck/safe-target",Head:head,
  RemoteControlAvailable:false,
 })
 if err!=nil{t.Fatal(err)}
 for name,body:=range map[string]string{
  "unknown":strings.Replace(string(plain),"}",`,"path":"/etc/passwd"}`,1),
  "duplicate":strings.Replace(string(plain),"}",`,"branch":"main"}`,1),
  "remote-action":strings.Replace(string(plain),`"remote_control_available":false`,`"remote_control_available":true`,1),
 }{
  t.Run(name,func(t *testing.T){
   c,a,_:=participants(t)
   frame,err:=a.Seal([]byte(body))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitBranchDeleteResult(c,frame,testActionID);
    !errors.Is(err,ErrInvalidRemoteAction){t.Fatalf("forgery accepted: %v",err)}
   if c.Established(){t.Fatal("forged E2EE envelope failed to retire channel")}
  })
 }
}
func TestRemoteGitDeleteNativeFixedNoArguments(t *testing.T){
 for _,op:=range []string{RemoteActionGitBranchDeleteReview,RemoteActionGitBranchDelete}{
  t.Run(op,func(t *testing.T){
   c,a,_:=participants(t)
   exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
    req,err:=OpenRemoteAction(a,frame)
    if err!=nil||req.Operation!=op||req.Text!=""||req.Message!=""||
     req.TaskID!=0||req.FileIndex!=0||req.Choice!=0||
     req.Snapshot!=""||req.TerminalInput!=""||req.Cols!=0||req.Rows!=0{
     return nil,ErrInvalidRemoteAction
    }
    return SealRemoteGitBranchDeleteResult(a,req.RequestID,RemoteGitBranchDeleteResult{
     Current:"main",Branch:"taskdeck/old",Head:strings.Repeat("b",40),
    })
   })
   native,err:=NewPingClient(c,exchange)
   if err!=nil{t.Fatal(err)}
   defer native.Close()
   var result RemoteGitBranchDeleteResult
   if op==RemoteActionGitBranchDeleteReview{
    result,err=native.GitBranchDeleteReview(context.Background())
   }else{result,err=native.GitBranchDelete(context.Background())}
   if err!=nil||result.Branch!="taskdeck/old"{t.Fatalf("typed E2EE delete: %+v %v",result,err)}
   for _,req:=range []RemoteAction{
    {RequestID:testActionID,Operation:op,Text:"taskdeck/other"},
    {RequestID:testActionID,Operation:op,FileIndex:1},
    {RequestID:testActionID,Operation:op,Message:"git branch -D"},
    {RequestID:testActionID,Operation:op,Snapshot:strings.Repeat("a",64)},
   }{
    t1,_,_:=participants(t)
    if _,err:=SealRemoteAction(t1,req);!errors.Is(err,ErrInvalidRemoteAction){
     t.Fatalf("client args accepted: %+v",req)
    }
   }
  })
 }
}
