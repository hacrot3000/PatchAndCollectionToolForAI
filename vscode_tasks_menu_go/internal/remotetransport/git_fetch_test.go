package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func fetchHTTPSFixture()string{
 return "GIT_FETCH_HTTPS_REVIEW_V1\nBRANCH refs/heads/main\nHEAD "+
  strings.Repeat("a",40)+"\nUPSTREAM refs/remotes/origin/main\nUPSTREAM_HEAD "+
  strings.Repeat("b",40)+"\nHTTPS_HOST github.com\nURL_SHA256 "+
  strings.Repeat("c",64)+"\nCONFIG_SHA256 "+strings.Repeat("d",64)+
  "\nWORKTREE clean\nNETWORK HTTPS-approval-required\n"
}

func TestRemoteGitFetchHTTPSIsTypedE2EEAndNeverAcceptsNetworkArguments(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,host,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionGitFetchReview,
 })
 if err!=nil||bytes.Contains(request,[]byte("git.fetch_https_review")){
  t.Fatal("network preflight leaked to Relay",err)
 }
 opened,err:=OpenRemoteAction(host,request)
 if err!=nil||opened.Operation!=RemoteActionGitFetchReview||
  opened.Path!=""||opened.Content!=""||opened.ExpectedSHA256!=""{
  t.Fatalf("fetch review accepted extra input: %+v %v",opened,err)
 }
 report:=fetchHTTPSFixture()
 frame,err:=SealRemoteGitFetchReviewResult(host,id,[]byte(report))
 if err!=nil||bytes.Contains(frame,[]byte("github.com")){t.Fatal("HTTPS origin host not encrypted",err)}
 read,err:=OpenRemoteGitFetchReviewResult(client,frame,id)
 if err!=nil||read!=report{t.Fatalf("incorrect HTTPS review %q %v",read,err)}
 if _,err:=OpenRemoteGitFetchReviewResult(client,frame,id);err==nil{
  t.Fatal("E2EE fetch preview accepted replayed ciphertext")
 }
 for _,action:=range []RemoteAction{
  {RequestID:id,Operation:RemoteActionGitFetchReview,Path:"https://evil.invalid"},
  {RequestID:id,Operation:RemoteActionGitFetchReview,Content:"git fetch --all"},
  {RequestID:id,Operation:RemoteActionGitFetchApply,Path:"origin",ExpectedSHA256:strings.Repeat("a",64)},
  {RequestID:id,Operation:RemoteActionGitFetchApply,Content:"git pull",ExpectedSHA256:strings.Repeat("a",64)},
  {RequestID:id,Operation:RemoteActionGitFetchApply,ExpectedSHA256:"bad"},
  {RequestID:id,Operation:RemoteActionGitFetchApply,TaskID:1,ExpectedSHA256:strings.Repeat("a",64)},
 }{
  c,_,_:=participants(t)
  if v,e:=SealRemoteAction(c,action);e==nil||len(v)>0{
   t.Fatalf("network request smuggled Git input %+v",action)
  }
 }
}

func TestRemoteGitFetchHTTPSApplyOnlyDigestAndIsNotPullResult(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 digest:=strings.Repeat("a",64)
 client,host,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionGitFetchApply,ExpectedSHA256:digest,
 })
 if err!=nil{t.Fatal(err)}
 opened,err:=OpenRemoteAction(host,request)
 if err!=nil||opened.ExpectedSHA256!=digest||
  opened.Operation!=RemoteActionGitFetchApply||opened.Path!=""{
  t.Fatalf("invalid authenticated network Fetch action %+v %v",opened,err)
 }
 oid:=strings.Repeat("b",40)
 frame,err:=SealRemoteGitFetchApplyResult(host,id,[]byte(oid))
 if err!=nil{t.Fatal(err)}
 actual,err:=OpenRemoteGitFetchApplyResult(client,frame,id)
 if err!=nil||actual!=oid{t.Fatalf("invalid typed Fetch result %q %v",actual,err)}
 if _,err:=OpenRemoteGitFetchApplyResult(client,frame,id);err==nil{
  t.Fatal("Fetch result replayed")
 }
 c,h,_:=participants(t)
 wrong,err:=SealRemoteGitPullApplyFilesResult(h,id,[]byte(oid))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitFetchApplyResult(c,wrong,id);err==nil||c.Established(){
  t.Fatal("offline file-changing Pull impersonated a Fetch success")
 }
}
func TestRemoteGitFetchHTTPSReviewRejectsForgedOrPartialResults(t *testing.T){
 canonical:=fetchHTTPSFixture()
 if !ValidRemoteGitFetchReviewReport(canonical){t.Fatal("canonical Fetch review was rejected")}
 for _,wrong:=range []string{
  strings.Replace(canonical,"NETWORK HTTPS-approval-required","NETWORK HTTP",1),
  strings.Replace(canonical,"HTTPS_HOST github.com","HTTPS_HOST 127.0.0.1",1),
  strings.Replace(canonical,"URL_SHA256 "+strings.Repeat("c",64),"URL_SHA256 abc",1),
  strings.Replace(canonical,"UPSTREAM refs/remotes/origin/main","UPSTREAM refs/remotes/other/main",1),
  strings.Replace(canonical,"WORKTREE clean","WORKTREE dirty",1),
  strings.TrimSuffix(canonical,"\n"),
  canonical+"UNTRUSTED\n",
 }{
  if ValidRemoteGitFetchReviewReport(wrong){t.Fatalf("unsafe HTTPS Fetch report admitted %q",wrong)}
  _,host,_:=participants(t)
  frame,e:=SealRemoteGitFetchReviewResult(host,"0123456789abcdef0123456789abcdef",[]byte(wrong))
  if e==nil||len(frame)>0{t.Fatal("host sealed malformed network review")}
 }
}
