package remotetransport

import (
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitDiffFixedStatReadE2EE(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  req,err:=OpenRemoteAction(agent,frame)
  if err!=nil||req.Operation!=RemoteActionGitDiff||req.TaskID!=0||
   req.Choice!=0||req.Text!=""||req.Message!=""{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteGitDiffResult(agent,req.RequestID,"tracked.txt | 1 +")
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 summary,err:=native.GitDiff(context.Background())
 if err!=nil||summary!="tracked.txt | 1 +"{
  t.Fatalf("git.diff encrypted stat %+q %v",summary,err)
 }
}
func TestRemoteGitDiffStrictResponseAndNoArgs(t *testing.T){
 const id=testActionID
 base:=`{"type":"taskdeck-e2ee-git-diff-result-v1","request_id":"0123456789abcdef0123456789abcdef","summary":"file | 1 +","remote_control_available":false}`
 for name,raw:=range map[string]string{
  "wrong id":strings.Replace(base,id,strings.Repeat("f",32),1),
  "extra filename":strings.Replace(base,`}`,`,"path":"secret"}`,1),
  "duplicate":strings.Replace(base,`}`,`,"summary":"other"}`,1),
  "control":strings.Replace(base,"file | 1 +",`file\u0000foo`,1),
  "oversize":strings.Replace(base,"file | 1 +",strings.Repeat("x",241),1),
 }{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   sealed,err:=agent.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitDiffResult(client,sealed,id);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("invalid diff accepted: %v",err)
   }
   if client.Established(){t.Fatal("malformed E2EE result left channel open")}
  })
 }
 for _,a:=range []RemoteAction{
  {RequestID:id,Operation:RemoteActionGitDiff,TaskID:1},
  {RequestID:id,Operation:RemoteActionGitDiff,Text:"README.md"},
  {RequestID:id,Operation:RemoteActionGitDiff,Message:"HEAD~1"},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,a);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("unreviewed diff argument accepted: %+v",a)
  }
 }
}
