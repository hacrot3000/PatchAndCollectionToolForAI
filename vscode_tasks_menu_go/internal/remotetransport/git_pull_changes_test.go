package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func pullChangesFixture(head,target string)string{
 return "GIT_PULL_CHANGES_V1\nBRANCH refs/heads/main\nHEAD "+head+
  "\nUPSTREAM refs/remotes/origin/main\nUPSTREAM_HEAD "+target+
  "\nFILES 2\nNETWORK none\n-- FILE CHANGES --\nM src/a.go\nA src/b.go\n"
}

func TestRemoteGitPullChangesStrictE2EEAndNoInputArguments(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 client,host,_:=participants(t)
 action:=RemoteAction{RequestID:id,Operation:RemoteActionGitPullChangesReview}
 wire,err:=SealRemoteAction(client,action)
 if err!=nil||bytes.Contains(wire,[]byte("pull_changes")){t.Fatal("unsealed/noisy Pull changes request",err)}
 decoded,err:=OpenRemoteAction(host,wire)
 if err!=nil||decoded.Operation!=RemoteActionGitPullChangesReview||
  decoded.Path!=""||decoded.Content!=""||decoded.ExpectedSHA256!=""{
  t.Fatalf("wrong typed Pull changes request: %+v %v",decoded,err)
 }
 data:=pullChangesFixture(strings.Repeat("a",40),strings.Repeat("b",40))
 frame,err:=SealRemoteGitPullChangesResult(host,id,[]byte(data))
 if err!=nil||bytes.Contains(frame,[]byte("src/a.go")){t.Fatal("Relay saw Git filenames",err)}
 actual,err:=OpenRemoteGitPullChangesResult(client,frame,id)
 if err!=nil||actual!=data{t.Fatalf("Pull changes E2EE corrupt: %q %v",actual,err)}
 if _,err:=OpenRemoteGitPullChangesResult(client,frame,id);err==nil{
  t.Fatal("E2EE changed-path response replay accepted")
 }
 for _,extra:=range []RemoteAction{
  {RequestID:id,Operation:RemoteActionGitPullChangesReview,Path:"origin/main"},
  {RequestID:id,Operation:RemoteActionGitPullChangesReview,Content:"git fetch"},
  {RequestID:id,Operation:RemoteActionGitPullChangesReview,TaskID:1},
  {RequestID:id,Operation:RemoteActionGitPullChangesReview,Offset:1},
  {RequestID:id,Operation:RemoteActionGitPullChangesReview,ExpectedSHA256:strings.Repeat("a",64)},
 }{
  ch,_,_:=participants(t)
  if reply,err:=SealRemoteAction(ch,extra);err==nil||len(reply)>0{
   t.Fatalf("Pull preview accepted unexpected argument: %+v",extra)
  }
 }
}

func TestRemoteGitPullChangesRejectsPartialUnsafeAndForgedReports(t *testing.T){
 base:=pullChangesFixture(strings.Repeat("a",40),strings.Repeat("b",40))
 if !ValidRemoteGitPullChangesReport(base){t.Fatal("canonical report refused")}
 wrong:=[]string{
  strings.Replace(base,"NETWORK none","NETWORK https://hacker",1),
  strings.Replace(base,"FILES 2","FILES 1",1),
  strings.Replace(base,"FILES 2","FILES 02",1),
  strings.Replace(base,"src/a.go","../secret",1),
  strings.Replace(base,"src/a.go",".git/config",1),
  strings.Replace(base,"src/a.go","src/name with space.go",1),
  strings.Replace(base,"src/a.go","src/escape\x1b[31m",1),
  strings.Replace(base,"M src/a.go","R src/a.go",1),
  strings.Replace(base,"M src/a.go\nA src/b.go","A src/b.go\nM src/a.go",1),
  strings.Replace(base,"UPSTREAM_HEAD "+strings.Repeat("b",40),"UPSTREAM_HEAD "+strings.Repeat("a",40),1),
  strings.TrimSuffix(base,"\n"),
  base+"untrusted extension\n",
 }
 for _,bad:=range wrong{
  if ValidRemoteGitPullChangesReport(bad){t.Fatalf("malformed Pull inventory accepted: %q",bad)}
  _,host,_:=participants(t)
  if encrypted,err:=SealRemoteGitPullChangesResult(host,"0123456789abcdef0123456789abcdef",[]byte(bad));
   err==nil||len(encrypted)>0{t.Fatal("host sealed malformed Pull changes")}
 }
}

func TestRemoteGitPullChangesRejectsTypedResultConfusion(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 client,host,_:=participants(t)
 other:=pullReviewTestReport("fast-forward",strings.Repeat("a",40),strings.Repeat("b",40))
 frame,err:=SealRemoteGitPullReviewResult(host,id,[]byte(other))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitPullChangesResult(client,frame,id);err==nil||client.Established(){
  t.Fatal("Pull Changes accepted old preflight result as authenticated manifest")
 }
}
