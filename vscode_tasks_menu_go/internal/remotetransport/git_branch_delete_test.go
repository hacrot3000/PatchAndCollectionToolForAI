package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestGitBranchDeleteTypedE2EEAndDomainSeparation(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 digest:=strings.Repeat("a",64)
 c,h,_:=participants(t)
 req,err:=SealRemoteAction(c,RemoteAction{
  RequestID:id,Operation:RemoteActionGitBranchDelete,Path:"safe-target",ExpectedSHA256:digest})
 if err!=nil{t.Fatal(err)}
 for _,v:=range []string{"git.branch_delete","safe-target",digest}{
  if bytes.Contains(req,[]byte(v)){t.Fatal("untrusted relay read branch deletion")}
 }
 act,err:=OpenRemoteAction(h,req)
 if err!=nil||act.Operation!=RemoteActionGitBranchDelete||
  act.Path!="safe-target"||act.ExpectedSHA256!=digest{
  t.Fatalf("incorrect action: %+v %v",act,err)
 }
 sealed,err:=SealRemoteGitBranchDeleteResult(h,id,[]byte("deleted"))
 if err!=nil{t.Fatal(err)}
 value,err:=OpenRemoteGitBranchDeleteResult(c,sealed,id)
 if err!=nil||value!="deleted"{t.Fatal("invalid authenticated delete result")}
 c,h,_=participants(t)
 sealed,err=SealRemoteGitBranchDeleteResult(h,id,[]byte("deleted"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitBranchDeleteReviewResult(c,sealed,id);err==nil||c.Established(){
  t.Fatal("delete result passed as a non-mutating review")
 }
}
func TestGitBranchDeleteRejectsInjectedFieldsAndUnsafeRefs(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 digest:=strings.Repeat("a",64)
 for _,a:=range []RemoteAction{
  {Operation:RemoteActionGitBranchDelete,Path:"../main",ExpectedSHA256:digest},
  {Operation:RemoteActionGitBranchDelete,Path:"main",ExpectedSHA256:"bad"},
  {Operation:RemoteActionGitBranchDelete,Path:"safe-target",ExpectedSHA256:digest,Content:"sh"},
  {Operation:RemoteActionGitBranchDelete,Path:"safe-target",ExpectedSHA256:digest,TaskID:1},
  {Operation:RemoteActionGitBranchDelete,Path:"safe-target",ExpectedSHA256:digest,Offset:1},
  {Operation:RemoteActionGitBranchDeleteReview,Path:"safe-target",ExpectedSHA256:digest},
  {Operation:RemoteActionGitBranchDeleteReview,Path:"--delete"},
 }{
  c,_,_:=participants(t)
  a.RequestID=id
  if req,err:=SealRemoteAction(c,a);err==nil||len(req)!=0{
   t.Fatalf("accepted unsafe branch operation: %+v",a)
  }
 }
 c,h,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"`+id+`","operation":"git.branch_delete","path":"safe-target","expected_sha256":"`+digest+`","command":"git branch -D main"}`)
 packet,err:=c.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(h,packet);err==nil||h.Established(){
  t.Fatal("arbitrary injected Git command accepted")
 }
}
