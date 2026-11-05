package remotetransport

import(
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteGitRemotesNamesOnlyE2EE(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  req,err:=OpenRemoteAction(agent,frame)
  if err!=nil||req.Operation!=RemoteActionGitRemotes||
   req.TaskID!=0||req.Choice!=0||req.Text!=""||
   req.Message!=""||req.FileIndex!=0||req.Snapshot!=""{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteGitRemotesResult(agent,req.RequestID,"GIT_REMOTES_V1\norigin\nupstream\n")
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 got,err:=native.GitRemotes(context.Background())
 if err!=nil||got!="GIT_REMOTES_V1\norigin\nupstream\n"{
  t.Fatalf("names-only authenticated metadata: %q %v",got,err)
 }
}
func TestRemoteGitRemotesStrictResultSchema(t *testing.T){
 for _,name:=range []string{"origin","upstream_1","upstream.test","Backup-1"}{
  if !ValidRemoteGitRemoteName(name){t.Fatalf("safe remote name denied: %q",name)}
 }
 for _,name:=range []string{"","-bad",".bad","bad-","a/b","http://url","admin\npassword","ü","origin..bad",strings.Repeat("z",65)}{
  if ValidRemoteGitRemoteName(name){t.Fatalf("unsafe remote name admitted: %q",name)}
 }
 for _,report:=range []string{
  "GIT_REMOTES_V1\n",
  "GIT_REMOTES_V1\norigin\n",
  "GIT_REMOTES_V1\norigin\nupstream\n",
 }{
  if !ValidRemoteGitRemotesReport(report){t.Fatalf("valid report rejected: %q",report)}
 }
 for _,report:=range []string{
  "GIT_REMOTES_V1\nupstream\norigin\n",
  "GIT_REMOTES_V1\norigin\norigin\n",
  "GIT_REMOTES_V1\norigin\nhttps://token@evil\n",
  "GIT_REMOTES_V1\norigin",
  "GIT_REMOTES_V1\n"+strings.Repeat("a",321),
 }{
  if ValidRemoteGitRemotesReport(report){t.Fatalf("invalid report accepted: %q",report)}
 }
 const id=testActionID
 base:=`{"type":"taskdeck-e2ee-git-remotes-result-v1","request_id":"0123456789abcdef0123456789abcdef","remotes":"GIT_REMOTES_V1\\norigin\\n","remote_control_available":false}`
 cases:=map[string]string{
  "wrong id":strings.Replace(base,id,strings.Repeat("f",32),1),
  "duplicate":strings.Replace(base,"}",`,"remotes":"GIT_REMOTES_V1"}`,1),
  "extra":strings.Replace(base,"}",`,"url":"https://secret"}`,1),
  "escalation":strings.Replace(base,`"remote_control_available":false`,`"remote_control_available":true`,1),
  "null":strings.Replace(base,`"remotes":"GIT_REMOTES_V1\\norigin\\n"`,`"remotes":null`,1),
  "URL":strings.Replace(base,"origin",`https://user:token@evil`,1),
 }
 for name,raw:=range cases{
  t.Run(name,func(t *testing.T){
   c,a,_:=participants(t)
   frame,err:=a.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteGitRemotesResult(c,frame,id);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("bad metadata accepted: %v",err)
   }
   if c.Established(){t.Fatal("invalid metadata did not poison E2EE channel")}
  })
 }
 for _,request:=range []RemoteAction{
  {RequestID:id,Operation:RemoteActionGitRemotes,Message:"origin"},
  {RequestID:id,Operation:RemoteActionGitRemotes,Text:"origin"},
  {RequestID:id,Operation:RemoteActionGitRemotes,TaskID:2},
 }{
  client,_,_:=participants(t)
  if _,err:=SealRemoteAction(client,request);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("caller supplied Git remote arguments: %+v",request)
  }
 }
}
func TestRemoteGitRemotesFeatureIsAdvisoryAndTyped(t *testing.T){
 client,agent,_:=participants(t)
 frame,err:=SealRemoteActionFeaturesResult(agent,testActionID,RemoteFeatures{
  SessionPing:true,GitStatus:true,GitRemotes:true,
 })
 if err!=nil{t.Fatal(err)}
 result,err:=OpenRemoteActionFeaturesResult(client,frame,testActionID)
 if err!=nil||!result.GitRemotes||!result.GitStatus{t.Fatalf("Git remote capability lost: %+v %v",result,err)}
}
