package remotetransport

import (
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitBranchesFixedNativeE2EE(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  req,err:=OpenRemoteAction(agent,frame)
  if err!=nil||req.Operation!=RemoteActionGitBranches||
   req.TaskID!=0||req.Choice!=0||req.Text!=""||
   req.Message!=""||req.TerminalInput!=""||
   req.FileIndex!=0||req.Snapshot!=""{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteGitBranchesResult(agent,req.RequestID,
   "Current: main\nLocal branches (first 4):\n- main\n- topic")
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 branches,err:=native.GitBranches(context.Background())
 if err!=nil||!strings.Contains(branches,"Current: main")||
  !strings.Contains(branches,"- topic"){
  t.Fatalf("authenticated GitBranches failed: %q %v",branches,err)
 }
}

func TestRemoteGitBranchesRejectsMalformedE2EE(t *testing.T){
 const id=testActionID
 base:=`{"type":"taskdeck-e2ee-git-branches-result-v1","request_id":"0123456789abcdef0123456789abcdef","branches":"Current: main","remote_control_available":false}`
 cases:=map[string]string{
  "wrong id":strings.Replace(base,id,strings.Repeat("f",32),1),
  "duplicate":strings.Replace(base,"}",`,"branches":"forged"}`,1),
  "extra ref":strings.Replace(base,"}",`,"branch":"evil"}`,1),
  "escalation":strings.Replace(base,`"remote_control_available":false`,`"remote_control_available":true`,1),
  "null":strings.Replace(base,`"branches":"Current: main"`,`"branches":null`,1),
  "control":strings.Replace(base,"Current: main","Current: main\u001b[31m",1),
  "oversize":strings.Replace(base,"Current: main",strings.Repeat("a",241),1),
 }
 for name,raw:=range cases{
  t.Run(name,func(t *testing.T){
   c,a,_:=participants(t)
   frame,err:=a.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitBranchesResult(c,frame,id);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("malformed GitBranches result accepted: %v",err)
   }
   if c.Established(){t.Fatal("poisoned branches response left channel alive")}
  })
 }
 for _,request:=range []RemoteAction{
  {RequestID:id,Operation:RemoteActionGitBranches,Message:"feature/develop"},
  {RequestID:id,Operation:RemoteActionGitBranches,Text:"origin"},
  {RequestID:id,Operation:RemoteActionGitBranches,TaskID:1},
  {RequestID:id,Operation:RemoteActionGitBranches,FileIndex:1,Snapshot:"## main
 M a.txt
"},
 }{
  client,_,_:=participants(t)
  if _,err:=SealRemoteAction(client,request);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("client selected a branch argument: %+v",request)
  }
 }
}
