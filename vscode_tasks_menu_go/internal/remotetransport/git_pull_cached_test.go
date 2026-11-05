package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func cachedPullReportForTest()string{
 return "GIT_PULL_CACHED_V1\nBRANCH refs/heads/main\nHEAD "+strings.Repeat("a",40)+
  "\nUPSTREAM refs/remotes/origin/main\nUPSTREAM_HEAD "+strings.Repeat("b",40)+
  "\nINDEX_SHA256 "+strings.Repeat("c",64)+
  "\nTREE unchanged\nWORKTREE clean\nNETWORK none\n"
}
func TestRemoteCachedPullReviewAndApplySeparateE2EEDomains(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,host,_:=participants(t)
 req,err:=SealRemoteAction(client,RemoteAction{RequestID:id,
  Operation:RemoteActionGitPullCachedReview})
 if err!=nil{t.Fatal(err)}
 action,err:=OpenRemoteAction(host,req)
 if err!=nil||action.Operation!=RemoteActionGitPullCachedReview{
  t.Fatalf("wrong cached review action %+v %v",action,err)
 }
 report:=cachedPullReportForTest()
 reply,err:=SealRemoteGitPullCachedReviewResult(host,id,[]byte(report))
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(reply,[]byte("origin/main")){t.Fatal("Relay saw Pull preview")}
 got,err:=OpenRemoteGitPullCachedReviewResult(client,reply,id)
 if err!=nil||got!=report{t.Fatalf("invalid E2EE review %q %v",got,err)}
 client2,host2,_:=participants(t)
 digest:=strings.Repeat("d",64)
 apply,err:=SealRemoteAction(client2,RemoteAction{RequestID:id,
  Operation:RemoteActionGitPullApplyCached,ExpectedSHA256:digest})
 if err!=nil{t.Fatal(err)}
 a,err:=OpenRemoteAction(host2,apply)
 if err!=nil||a.Operation!=RemoteActionGitPullApplyCached||
  a.ExpectedSHA256!=digest||a.Path!=""{
  t.Fatalf("digest-only apply malformed: %+v %v",a,err)
 }
 out,err:=SealRemoteGitPullApplyCachedResult(host2,id,[]byte(strings.Repeat("b",40)))
 if err!=nil{t.Fatal(err)}
 result,err:=OpenRemoteGitPullApplyCachedResult(client2,out,id)
 if err!=nil||result!=strings.Repeat("b",40){
  t.Fatalf("CAS result invalid: %q %v",result,err)
 }
}
func TestRemoteCachedPullRefusesForgedReviewOrApplyArguments(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 for _,bad:=range []string{
  strings.Replace(cachedPullReportForTest(),"TREE unchanged","TREE changed",1),
  strings.Replace(cachedPullReportForTest(),"WORKTREE clean","WORKTREE dirty",1),
  strings.Replace(cachedPullReportForTest(),"NETWORK none","NETWORK fetch",1),
  strings.Replace(cachedPullReportForTest(),"UPSTREAM refs/remotes/origin/main","UPSTREAM refs/remotes/origin/HEAD",1),
 }{
  if ValidRemoteGitPullCachedReviewReport(bad){t.Fatalf("forged review accepted: %q",bad)}
 }
 for _,bad:=range []RemoteAction{
  {RequestID:id,Operation:RemoteActionGitPullApplyCached,ExpectedSHA256:strings.Repeat("d",64),Path:"origin"},
  {RequestID:id,Operation:RemoteActionGitPullApplyCached,ExpectedSHA256:strings.Repeat("d",64),Content:"git pull"},
  {RequestID:id,Operation:RemoteActionGitPullApplyCached,ExpectedSHA256:"ABC"},
  {RequestID:id,Operation:RemoteActionGitPullApplyCached,ExpectedSHA256:strings.Repeat("d",64),TaskID:9},
 }{
  c,_,_:=participants(t)
  if b,err:=SealRemoteAction(c,bad);err==nil||len(b)!=0{
   t.Fatalf("unreviewed Git argv accepted: %+v",bad)
  }
 }
 client,host,_:=participants(t)
 other,err:=SealRemoteGitPullReviewResult(host,id,[]byte(pullReviewTestReport("fast-forward",strings.Repeat("a",40),strings.Repeat("b",40))))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitPullCachedReviewResult(client,other,id);err==nil||client.Established(){
  t.Fatal("review domain confusion was accepted")
 }
}
