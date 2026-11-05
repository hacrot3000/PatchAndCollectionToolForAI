package remotetransport

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitBranchCreateFixedNativeE2EE(t *testing.T){
 c,a,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  req,err:=OpenRemoteAction(a,frame)
  if err!=nil||req.Operation!=RemoteActionGitBranchCreate||
   req.TaskID!=0||req.Choice!=0||req.Text!=""||
   req.Message!=""||req.FileIndex!=0||req.Snapshot!=""||
   req.TerminalInput!=""||req.Cols!=0||req.Rows!=0{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteGitBranchCreateResult(a,req.RequestID,
   RemoteGitBranchCreateResult{
    Branch:"topic-new",Head:strings.Repeat("a",40),
   })
 })
 client,err:=NewPingClient(c,exchange)
 if err!=nil{t.Fatal(err)}
 defer client.Close()
 result,err:=client.GitBranchCreate(context.Background())
 if err!=nil||result.Branch!="topic-new"||
  result.Head!=strings.Repeat("a",40){
  t.Fatalf("invalid branch create result: %+v %v",result,err)
 }
}

func TestRemoteGitBranchCreateStrictSchemasAndNames(t *testing.T){
 for _,name:=range []string{"", "HEAD","foo.lock","foo.","a..b","a/b",
  "-bad"," name","a@{b","x\nbad",strings.Repeat("a",65)}{
  if ValidRemoteGitCreatedBranchName(name){
   t.Errorf("unsafe branch name accepted: %q",name)
  }
 }
 for _,name:=range []string{"feature_test","topic-new","release.12","A1"}{
  if !ValidRemoteGitCreatedBranchName(name){t.Errorf("valid name denied %q",name)}
 }
 base,err:=json.Marshal(map[string]any{
  "type":RemoteGitBranchCreateResultProtocol,"request_id":testActionID,
  "branch":"topic-new","head":strings.Repeat("a",40),
  "remote_control_available":false,
 })
 if err!=nil{t.Fatal(err)}
 raw:=string(base)
 for key,forged:=range map[string]string{
  "wrong ID":strings.Replace(raw,testActionID,strings.Repeat("f",32),1),
  "extra name":strings.Replace(raw,"}",`,"name":"attack"}`,1),
  "duplicate branch":strings.Replace(raw,"}",`,"branch":"attacker"}`,1),
  "bad ref":strings.Replace(raw,"topic-new","../evil",1),
  "wrong result type":strings.Replace(raw,RemoteGitBranchCreateResultProtocol,"invalid",1),
  "control flag":strings.Replace(raw,`"remote_control_available":false`,`"remote_control_available":true`,1),
 }{
  t.Run(key,func(t *testing.T){
   c,a,_:=participants(t)
   frame,err:=a.Seal([]byte(forged))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitBranchCreateResult(c,frame,testActionID);
    !errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("forged reply accepted: %v",err)
   }
   if c.Established(){t.Fatal("forged branch response kept E2EE channel alive")}
  })
 }
 for _,req:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionGitBranchCreate,Text:"topic"},
  {RequestID:testActionID,Operation:RemoteActionGitBranchCreate,Message:"topic"},
  {RequestID:testActionID,Operation:RemoteActionGitBranchCreate,TaskID:1},
  {RequestID:testActionID,Operation:RemoteActionGitBranchCreate,FileIndex:1,Snapshot:"## main
 M x
"},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,req);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("branch create accepted caller arguments: %+v",req)
  }
 }
}
