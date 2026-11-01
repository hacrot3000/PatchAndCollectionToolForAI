package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestGitBranchCreateTypedE2EEAndDistinctResult(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 hash:=strings.Repeat("a",64)
 client,host,_:=participants(t)
 frame,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionGitBranchCreate,
  Path:"safe-branch1",ExpectedSHA256:hash,
 })
 if err!=nil{t.Fatal(err)}
 for _,secret:=range []string{"git.branch_create","safe-branch1",hash}{
  if bytes.Contains(frame,[]byte(secret)){t.Fatal("relay decrypted a branch creation field")}
 }
 parsed,err:=OpenRemoteAction(host,frame)
 if err!=nil||parsed.Operation!=RemoteActionGitBranchCreate||
  parsed.Path!="safe-branch1"||parsed.ExpectedSHA256!=hash{
  t.Fatalf("typed branch-create action failed: %+v %v",parsed,err)
 }
 oid:=[]byte(strings.Repeat("b",40))
 reply,err:=SealRemoteGitBranchCreateResult(host,id,oid)
 if err!=nil{t.Fatal(err)}
 result,err:=OpenRemoteGitBranchCreateResult(client,reply,id)
 if err!=nil||result!=string(oid){t.Fatalf("authenticated ref result %q %v",result,err)}
 client,host,_=participants(t)
 reply,err=SealRemoteGitBranchCreateResult(host,id,oid)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitBranchReviewResult(client,reply,id);err==nil||client.Established(){
  t.Fatal("write-result was accepted as read-only branch review")
 }
}

func TestGitBranchCreateRejectsUntrustedRefsAndExtraFields(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 sha:=strings.Repeat("a",64)
 for _,slug:=range []string{
  "", "a", "-evil", "a-","9-evil","Abranch","main/x","../heads",
  ".git","a.lock","a..b","a~b","a:b","a b","a\tb","a\nb",
  "a$(sh)","a*","a?","a[",strings.Repeat("a",33),
 }{
  client,_,_:=participants(t)
  frame,err:=SealRemoteAction(client,RemoteAction{
   RequestID:id,Operation:RemoteActionGitBranchCreate,
   Path:slug,ExpectedSHA256:sha,
  })
  if err==nil||len(frame)!=0{t.Fatalf("untrusted ref accepted %q",slug)}
 }
 for _,a:=range []RemoteAction{
  {Operation:RemoteActionGitBranchCreate,Path:"valid-name",ExpectedSHA256:"bad"},
  {Operation:RemoteActionGitBranchCreate,Path:"valid-name",ExpectedSHA256:sha,Content:"exec sh"},
  {Operation:RemoteActionGitBranchCreate,Path:"valid-name",ExpectedSHA256:sha,TaskID:1},
  {Operation:RemoteActionGitBranchCreate,Path:"valid-name",ExpectedSHA256:sha,Offset:1},
  {Operation:RemoteActionGitBranchReview,Path:"secret"},
  {Operation:RemoteActionGitBranchReview,Content:"sh"},
 }{
  client,_,_:=participants(t)
  a.RequestID=id
  frame,err:=SealRemoteAction(client,a)
  if err==nil||len(frame)>0{t.Fatalf("foreign Git capability accepted %+v",a)}
 }
 client,host,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.branch_create","path":"safe-branch1","expected_sha256":"`+sha+`","command":"sh"}`)
 frame,err:=client.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(host,frame);err==nil||host.Established(){
  t.Fatal("forged Git command was not rejected")
 }
}
