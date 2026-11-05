package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func gitPushHTTPSFixture()string{
 return "GIT_PUSH_HTTPS_REVIEW_V1\nBRANCH refs/heads/main\nHEAD "+
  strings.Repeat("b",40)+"\nUPSTREAM refs/remotes/origin/main\nUPSTREAM_HEAD "+
  strings.Repeat("a",40)+"\nHTTPS_HOST github.com\nURL_SHA256 "+
  strings.Repeat("c",64)+"\nCONFIG_SHA256 "+strings.Repeat("d",64)+
  "\nINDEX_SHA256 "+strings.Repeat("e",64)+
  "\nHELPER_SHA256 "+strings.Repeat("f",64)+
  "\nSTATUS fast-forward-only\nWORKTREE clean"+
  "\nREMOTE_REF existing\nNETWORK HTTPS-approval-required\n"
}

func TestRemoteGitPushE2EETypedReviewAndDigestOnlyApply(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,host,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionGitPushReview,
 })
 if err!=nil||bytes.Contains(request,[]byte("git.push_https_review")){
  t.Fatalf("unsealed Push E2EE review: %v",err)
 }
 action,err:=OpenRemoteAction(host,request)
 if err!=nil||action.Operation!=RemoteActionGitPushReview||
  action.Path!=""||action.Content!=""||action.ExpectedSHA256!=""{
  t.Fatalf("Push review accepted extra fields %+v %v",action,err)
 }
 report:=gitPushHTTPSFixture()
 encrypted,err:=SealRemoteGitPushReviewResult(host,id,[]byte(report))
 if err!=nil||bytes.Contains(encrypted,[]byte("github.com")){
  t.Fatal("Push review leaked remote hostname",err)
 }
 actual,err:=OpenRemoteGitPushReviewResult(client,encrypted,id)
 if err!=nil||actual!=report{t.Fatalf("wrong Push E2EE review %q %v",actual,err)}
 if _,err:=OpenRemoteGitPushReviewResult(client,encrypted,id);err==nil{
  t.Fatal("replayed Push preview accepted")
 }
 c,h,_:=participants(t)
 digest:=strings.Repeat("b",64)
 wire,err:=SealRemoteAction(c,RemoteAction{
  RequestID:id,Operation:RemoteActionGitPushApply,ExpectedSHA256:digest,
 })
 if err!=nil{t.Fatal(err)}
 parsed,err:=OpenRemoteAction(h,wire)
 if err!=nil||parsed.ExpectedSHA256!=digest||
  parsed.Operation!=RemoteActionGitPushApply||parsed.Path!=""{
  t.Fatalf("Push E2EE digest tampered %+v %v",parsed,err)
 }
 result,err:=SealRemoteGitPushApplyResult(h,id,[]byte(strings.Repeat("b",40)))
 if err!=nil{t.Fatal(err)}
 pushed,err:=OpenRemoteGitPushApplyResult(c,result,id)
 if err!=nil||pushed!=strings.Repeat("b",40){t.Fatal("typed Push result mismatch",err)}
}

func TestRemoteGitPushRefusesForgedOrPartialReviews(t *testing.T){
 original:=gitPushHTTPSFixture()
 if !ValidRemoteGitPushReviewReport(original){t.Fatal("canonical Push review refused")}
 bad:=[]string{
  strings.Replace(original,"STATUS fast-forward-only","STATUS force",1),
  strings.Replace(original,"WORKTREE clean","WORKTREE dirty",1),
  strings.Replace(original,"REMOTE_REF existing","REMOTE_REF absent",1),
  strings.Replace(original,"HTTPS_HOST github.com","HTTPS_HOST localhost",1),
  strings.Replace(original,"UPSTREAM_HEAD "+strings.Repeat("a",40),
   "UPSTREAM_HEAD "+strings.Repeat("b",40),1),
  strings.Replace(original,"UPSTREAM refs/remotes/origin/main",
   "UPSTREAM refs/remotes/evil/main",1),
  strings.Replace(original,"HELPER_SHA256 "+strings.Repeat("f",64),
   "HELPER_SHA256 bad",1),
  strings.TrimSuffix(original,"\n"),
  original+"INJECTED\n",
 }
 for _,v:=range bad{
  if ValidRemoteGitPushReviewReport(v){t.Fatalf("forged Push reviewed: %q",v)}
  _,host,_:=participants(t)
  sealed,err:=SealRemoteGitPushReviewResult(host,
   "0123456789abcdef0123456789abcdef",[]byte(v))
  if err==nil||len(sealed)!=0{t.Fatal("host sealed invalid Push report")}
 }
 for _,extra:=range []RemoteAction{
  {RequestID:"0123456789abcdef0123456789abcdef",Operation:RemoteActionGitPushReview,Path:"origin/main"},
  {RequestID:"0123456789abcdef0123456789abcdef",Operation:RemoteActionGitPushReview,Content:"git push --force"},
  {RequestID:"0123456789abcdef0123456789abcdef",Operation:RemoteActionGitPushApply,Path:"URL",ExpectedSHA256:strings.Repeat("a",64)},
  {RequestID:"0123456789abcdef0123456789abcdef",Operation:RemoteActionGitPushApply,TaskID:1,ExpectedSHA256:strings.Repeat("a",64)},
  {RequestID:"0123456789abcdef0123456789abcdef",Operation:RemoteActionGitPushApply,ExpectedSHA256:"bad"},
 }{
  c,_,_:=participants(t)
  if wire,err:=SealRemoteAction(c,extra);err==nil||len(wire)!=0{
   t.Fatalf("malformed Push E2EE action accepted: %+v",extra)
  }
 }
}
