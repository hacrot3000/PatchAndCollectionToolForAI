package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func pullReviewTestReport(status,head,target string)string{
 return "GIT_PULL_REVIEW_V1\nBRANCH refs/heads/main\nHEAD "+head+
  "\nUPSTREAM refs/remotes/origin/main\nUPSTREAM_HEAD "+target+
  "\nSTATUS "+status+"\nWORKTREE clean\nNETWORK none\n"
}
func TestRemoteGitPullReviewExactE2EEDomainAndNoArguments(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,host,_:=participants(t)
 req,err:=SealRemoteAction(client,RemoteAction{
  Operation:RemoteActionGitPullReview,RequestID:id,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(req,[]byte("git.pull_review")){t.Fatal("Relay saw Pull review name")}
 actual,err:=OpenRemoteAction(host,req)
 if err!=nil||actual.Operation!=RemoteActionGitPullReview||
  actual.Path!=""||actual.Content!=""{
  t.Fatalf("Pull review action not exact %+v %v",actual,err)
 }
 report:=pullReviewTestReport("fast-forward",strings.Repeat("a",40),strings.Repeat("b",40))
 frame,err:=SealRemoteGitPullReviewResult(host,id,[]byte(report))
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(frame,[]byte("origin/main")){t.Fatal("Relay saw tracking refs")}
 got,err:=OpenRemoteGitPullReviewResult(client,frame,id)
 if err!=nil||got!=report{t.Fatalf("authenticated result lost: %q %v",got,err)}
 // No identical ciphertext replay for this explicitly typed action.
 if _,err:=OpenRemoteGitPullReviewResult(client,frame,id);err==nil{
  t.Fatal("Pull preview E2EE result replay accepted")
 }
 for _,bad:=range []RemoteAction{
  {Operation:RemoteActionGitPullReview,RequestID:id,Path:"origin"},
  {Operation:RemoteActionGitPullReview,RequestID:id,Content:"git pull"},
  {Operation:RemoteActionGitPullReview,RequestID:id,ExpectedSHA256:strings.Repeat("a",64)},
  {Operation:RemoteActionGitPullReview,RequestID:id,TaskID:1},
  {Operation:RemoteActionGitPullReview,RequestID:id,Offset:2},
 }{
  c,_,_:=participants(t)
  if f,err:=SealRemoteAction(c,bad);err==nil||len(f)!=0{
   t.Fatalf("unapproved Pull argument accepted: %+v",bad)
  }
 }
}
func TestRemoteGitPullReviewRejectsPartialForgedOrNetworkOutput(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 base:=pullReviewTestReport("up-to-date",strings.Repeat("c",40),strings.Repeat("c",40))
 for _,report:=range []string{
  strings.Replace(base,"NETWORK none","NETWORK https://evil",1),
  strings.Replace(base,"WORKTREE clean","WORKTREE dirty",1),
  strings.Replace(base,"STATUS up-to-date","STATUS fast-forward",1),
  strings.Replace(base,"UPSTREAM refs/remotes/origin/main","UPSTREAM refs/remotes/origin/HEAD",1),
  strings.Replace(base,"BRANCH refs/heads/main","BRANCH refs/heads/../main",1),
  strings.TrimSuffix(base,"\n"),
  base+"EXTRA token\n",
 }{
  if ValidRemoteGitPullReviewReport(report){t.Fatalf("forged Pull report accepted %q",report)}
  _,host,_:=participants(t)
  if frame,err:=SealRemoteGitPullReviewResult(host,id,[]byte(report));err==nil||len(frame)!=0{
   t.Fatal("host sealed malformed Pull report")
  }
 }
 // A different E2EE result cannot be replayed as a Pull review.
 client,host,_:=participants(t)
 other,err:=SealRemoteGitTrackingRefsResult(host,id,[]byte("GIT_TRACKING_V1\n"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitPullReviewResult(client,other,id);err==nil||client.Established(){
  t.Fatal("E2EE type confusion on Pull review did not close channel")
 }
}


func TestRemoteGitPullSameTreeE2EEOnlyAcceptsReviewDigest(t *testing.T){
 const id="a123456789abcdef0123456789abcdef"
 for _,a:=range []RemoteAction{
  {RequestID:id,Operation:RemoteActionGitPullSameTree,Path:"origin",ExpectedSHA256:strings.Repeat("a",64)},
  {RequestID:id,Operation:RemoteActionGitPullSameTree,Content:"git pull",ExpectedSHA256:strings.Repeat("a",64)},
  {RequestID:id,Operation:RemoteActionGitPullSameTree,ExpectedSHA256:"bad"},
  {RequestID:id,Operation:RemoteActionGitPullSameTree,TaskID:1,ExpectedSHA256:strings.Repeat("a",64)},
 }{
  client,_,_:=participants(t)
  if wire,err:=SealRemoteAction(client,a);err==nil||len(wire)!=0{
   t.Fatalf("same-tree FF accepted unreviewed user arguments: %+v",a)
  }
 }
 client,host,_:=participants(t)
 digest:=strings.Repeat("a",64)
 encrypted,err:=SealRemoteAction(client,RemoteAction{
  Operation:RemoteActionGitPullSameTree,RequestID:id,ExpectedSHA256:digest})
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(encrypted,[]byte(digest)){t.Fatal("Relay saw writable digest")}
 action,err:=OpenRemoteAction(host,encrypted)
 if err!=nil||action.ExpectedSHA256!=digest||action.Path!=""{
  t.Fatalf("E2EE same-tree FF request lost validation: %+v %v",action,err)
 }
 tip:=strings.Repeat("b",40)
 reply,err:=SealRemoteGitPullSameTreeResult(host,id,[]byte(tip))
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteGitPullSameTreeResult(client,reply,id)
 if err!=nil||got!=tip{t.Fatalf("same-tree FF authenticated result invalid: %s %v",got,err)}
}
