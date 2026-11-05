package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestRemoteGitPullFileApplyIsOnlyDigestAndSeparatelyAuthenticated(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 digest:=strings.Repeat("a",64)
 client,host,_:=participants(t)
 envelope,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionGitPullApplyFiles,ExpectedSHA256:digest,
 })
 if err!=nil||bytes.Contains(envelope,[]byte("pull_apply_files")){
  t.Fatalf("authenticated Pull file apply leaked action: %v",err)
 }
 action,err:=OpenRemoteAction(host,envelope)
 if err!=nil||action.Operation!=RemoteActionGitPullApplyFiles||
  action.ExpectedSHA256!=digest||action.Content!=""||action.Path!=""{
  t.Fatalf("wrong authenticated file-changing Pull action: %+v %v",action,err)
 }
 target:=strings.Repeat("b",40)
 response,err:=SealRemoteGitPullApplyFilesResult(host,id,[]byte(target))
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(response,[]byte(target)){t.Fatal("Pull result unencrypted")}
 got,err:=OpenRemoteGitPullApplyFilesResult(client,response,id)
 if err!=nil||got!=target{t.Fatalf("typed result refused: %q %v",got,err)}
 if _,err:=OpenRemoteGitPullApplyFilesResult(client,response,id);err==nil{
  t.Fatal("encrypted Pull file-apply result replayed")
 }
 for _,invalid:=range []RemoteAction{
  {RequestID:id,Operation:RemoteActionGitPullApplyFiles,Path:"origin/main",ExpectedSHA256:digest},
  {RequestID:id,Operation:RemoteActionGitPullApplyFiles,Content:"git pull",ExpectedSHA256:digest},
  {RequestID:id,Operation:RemoteActionGitPullApplyFiles,TaskID:1,ExpectedSHA256:digest},
  {RequestID:id,Operation:RemoteActionGitPullApplyFiles,ExpectedSHA256:"1"},
  {RequestID:id,Operation:RemoteActionGitPullApplyFiles,Offset:1,ExpectedSHA256:digest},
 }{
  c,_,_:=participants(t)
  if frame,err:=SealRemoteAction(c,invalid);err==nil||len(frame)!=0{
   t.Fatalf("unapproved Pull file-apply input accepted: %+v",invalid)
  }
 }
}

func TestRemoteGitPullFileApplyRejectsWrongE2EEResultType(t *testing.T){
 client,host,_:=participants(t)
 id:="0123456789abcdef0123456789abcdef"
 other,err:=SealRemoteGitPullApplyCachedResult(host,id,[]byte(strings.Repeat("b",40)))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitPullApplyFilesResult(client,other,id);err==nil||client.Established(){
  t.Fatal("cached FF result impersonated tree-changing Pull response")
 }
}
