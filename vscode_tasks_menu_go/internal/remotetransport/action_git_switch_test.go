package remotetransport

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitSwitchSameHeadStrictEnvelope(t *testing.T){
 head:=strings.Repeat("a",40)
 good:=RemoteGitSwitchResult{From:"feature/old",Branch:"review",Head:head}
 if !ValidRemoteGitSwitchResult(good){t.Fatal("valid old branch and fixed target refused")}
 c,a,_:=participants(t)
 frame,err:=SealRemoteGitSwitchResult(a,testActionID,good)
 if err!=nil{t.Fatal(err)}
 read,err:=OpenRemoteGitSwitchResult(c,frame,testActionID)
 if err!=nil||read!=good{t.Fatalf("authenticated switch result: %+v %v",read,err)}
 for _,invalid:=range []RemoteGitSwitchResult{
  {From:"HEAD",Branch:"review",Head:head},
  {From:"feature/..",Branch:"review",Head:head},
  {From:"feature/.hidden",Branch:"review",Head:head},
  {From:"feature/HEAD",Branch:"review",Head:head},
  {From:"feature/unsafe.lock",Branch:"review",Head:head},
  {From:"main",Branch:"main",Head:head},
  {From:"main",Branch:"../secret",Head:head},
  {From:"main",Branch:"feature/review",Head:head},
  {From:"main",Branch:"review",Head:strings.Repeat("A",40)},
  {From:"main",Branch:"review",Head:"git switch review"},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteGitSwitchResult(c,testActionID,invalid);
   !errors.Is(err,ErrInvalidRemoteAction){t.Fatalf("forged result accepted: %+v %v",invalid,err)}
 }
 base,err:=json.Marshal(remoteGitSwitchWire{
  Type:RemoteGitSwitchResultProtocol,RequestID:testActionID,
  From:"main",Branch:"review",Head:head,RemoteControlAvailable:false,
 })
 if err!=nil{t.Fatal(err)}
 for name,raw:=range map[string]string{
  "unknown-field":strings.Replace(string(base),"}",`,"remote_url":"https://secret.invalid"}`,1),
  "duplicate-field":strings.Replace(string(base),"}",`,"branch":"hostile"}`,1),
  "null-from":strings.Replace(string(base),`"from":"main"`,`"from":null`,1),
  "escalation":strings.Replace(string(base),`"remote_control_available":false`,`"remote_control_available":true`,1),
 }{
  t.Run(name,func(t *testing.T){
   c,a,_:=participants(t)
   frame,err:=a.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitSwitchResult(c,frame,testActionID);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("invalid result accepted: %v",err)
   }
   if c.Established(){t.Fatal("malformed result did not retire E2EE channel")}
  })
 }
}

func TestRemoteGitSwitchSameHeadNativeNoArbitraryRef(t *testing.T){
 client,agent,_:=participants(t)
 seen:=0
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(agent,frame)
  if err!=nil||request.Operation!=RemoteActionGitSwitchSameHead||
   request.Text!=""||request.Message!=""||request.FileIndex!=0||
   request.Snapshot!=""||request.TaskID!=0||request.Choice!=0||
   request.TerminalInput!=""||request.Cols!=0||request.Rows!=0{
   return nil,ErrInvalidRemoteAction
  }
  seen++
  return SealRemoteGitSwitchResult(agent,request.RequestID,RemoteGitSwitchResult{
   From:"main",Branch:"review",Head:strings.Repeat("b",40),
  })
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 got,err:=native.GitSwitchSameHead(context.Background())
 if err!=nil||got.Branch!="review"||seen!=1{t.Fatalf("native E2EE switch result: %+v %v",got,err)}
 for _,req:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionGitSwitchSameHead,Text:"review"},
  {RequestID:testActionID,Operation:RemoteActionGitSwitchSameHead,Message:"git switch -f"},
  {RequestID:testActionID,Operation:RemoteActionGitSwitchSameHead,FileIndex:1},
  {RequestID:testActionID,Operation:RemoteActionGitSwitchSameHead,TaskID:9},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,req);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("caller-chosen branch/argument accepted: %+v",req)
  }
 }
}
