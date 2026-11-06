package remotetransport

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitPullChangesStrictE2EEAndTamper(t *testing.T){
 old,newOID:=strings.Repeat("a",40),strings.Repeat("b",40)
 good:=RemoteGitPullChanges{Head:old,UpstreamHead:newOID,Files:[]string{"M:README.md","A:src/new.go"}}
 if !ValidRemoteGitPullChanges(good){t.Fatal("expected safe path inventory")}
 c,a,_:=participants(t)
 frame,err:=SealRemoteGitPullChanges(a,testActionID,good)
 if err!=nil{t.Fatal(err)}
 result,err:=OpenRemoteGitPullChanges(c,frame,testActionID)
 if err!=nil||len(result.Files)!=2||result.Files[1]!="A:src/new.go"{
  t.Fatalf("E2EE changed-file result not authenticated: %+v %v",result,err)
 }
 invalid:=[]RemoteGitPullChanges{
  {Head:old,UpstreamHead:old,Files:[]string{"A:untrue"}},
  {Head:old,UpstreamHead:newOID,Files:[]string{"A:.env"}},
  {Head:old,UpstreamHead:newOID,Files:[]string{"A:bad name"}},
  {Head:old,UpstreamHead:newOID,Files:[]string{"A:../outside"}},
  {Head:old,UpstreamHead:newOID,Files:[]string{"A:a//b"}},
  {Head:old,UpstreamHead:newOID,Files:[]string{"A:git/../../etc"}},
  {Head:old,UpstreamHead:newOID,Files:[]string{"A:src/new","M:src/new"}},
  {Head:old,UpstreamHead:newOID,Files:[]string{"M:z","A:a"}},
  {Head:old,UpstreamHead:newOID,Files:[]string{"F:foo"}},
  {Head:strings.Repeat("A",40),UpstreamHead:newOID,Files:[]string{}},
 }
 for _,v:=range invalid{
  c,_,_:=participants(t)
  if _,err:=SealRemoteGitPullChanges(c,testActionID,v);
   !errors.Is(err,ErrInvalidRemoteAction){t.Fatalf("accepted invalid %+v: %v",v,err)}
 }
 baseRaw,err:=json.Marshal(remoteGitPullChangesWire{
  Type:RemoteGitPullChangesProtocol,RequestID:testActionID,
  Head:old,UpstreamHead:newOID,Files:[]string{"A:README.md"},
  RemoteControlAvailable:false,
 })
 if err!=nil{t.Fatal(err)}
 for name,raw:=range map[string]string{
  "unknown":strings.Replace(string(baseRaw),"}",`,"remote_url":"https://secrets.invalid"}`,1),
  "duplicate":strings.Replace(string(baseRaw),"}",`,"files":[]}`,1),
  "injection":strings.Replace(string(baseRaw),"A:README.md","A:../../secrets",1),
  "escalate":strings.Replace(string(baseRaw),`"remote_control_available":false`,`"remote_control_available":true`,1),
  "null":strings.Replace(string(baseRaw),`"files":["A:README.md"]`,`"files":null`,1),
 }{
  t.Run(name,func(t *testing.T){
   c,a,_:=participants(t)
   frame,err:=a.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitPullChanges(c,frame,testActionID);
    !errors.Is(err,ErrInvalidRemoteAction){t.Fatalf("forged result accepted: %v",err)}
   if c.Established(){t.Fatal("forged result failed to poison channel")}
  })
 }
}
func TestRemoteGitPullChangesNativeNoArguments(t *testing.T){
 c,a,_:=participants(t)
 count:=0
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(a,frame)
  if err!=nil||request.Operation!=RemoteActionGitPullChanges||
   request.Text!=""||request.Message!=""||request.TaskID!=0||
   request.FileIndex!=0||request.Snapshot!=""||request.Choice!=0||
   request.Cols!=0||request.Rows!=0||request.TerminalInput!=""{
   return nil,ErrInvalidRemoteAction
  }
  count++
  return SealRemoteGitPullChanges(a,request.RequestID,RemoteGitPullChanges{
   Head:strings.Repeat("a",40),UpstreamHead:strings.Repeat("b",40),
   Files:[]string{"A:new.txt"},
  })
 })
 native,err:=NewPingClient(c,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 result,err:=native.GitPullChanges(context.Background())
 if err!=nil||count!=1||len(result.Files)!=1{
  t.Fatalf("failed typed E2EE review: %+v %v",result,err)
 }
 for _,bad:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionGitPullChanges,TaskID:1},
  {RequestID:testActionID,Operation:RemoteActionGitPullChanges,Text:"origin/master"},
  {RequestID:testActionID,Operation:RemoteActionGitPullChanges,Message:"git pull"},
  {RequestID:testActionID,Operation:RemoteActionGitPullChanges,FileIndex:1,Snapshot:"sha"},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,bad);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("caller supplied Git args in read-only review: %+v",bad)
  }
 }
}
