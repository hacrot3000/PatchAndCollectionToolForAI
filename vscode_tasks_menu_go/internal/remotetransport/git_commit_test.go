package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestGitCommitTypedE2EEAndResultTypeIsolation(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 sha:=strings.Repeat("a",64)
 client,host,_:=participants(t)
 req,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionGitCommit,
  Content:"Approved commit message",ExpectedSHA256:sha,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(req,[]byte("Approved commit message"))||
  bytes.Contains(req,[]byte(sha))||bytes.Contains(req,[]byte("git.commit")){
  t.Fatal("compromised PHP could see commit message, review SHA or operation")
 }
 decoded,err:=OpenRemoteAction(host,req)
 if err!=nil||decoded.Operation!=RemoteActionGitCommit||
  decoded.Content!="Approved commit message"||decoded.ExpectedSHA256!=sha{
  t.Fatalf("commit request rejected or mutated: %+v %v",decoded,err)
 }
 oid:=[]byte(strings.Repeat("b",40))
 reply,err:=SealRemoteGitCommitResult(host,id,oid)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteGitCommitResult(client,reply,id)
 if err!=nil||got!=string(oid){t.Fatalf("commit result %q %v",got,err)}
 client,host,_=participants(t)
 reply,err=SealRemoteGitCommitResult(host,id,oid)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitCommitReviewResult(client,reply,id);err==nil||client.Established(){
  t.Fatal("write commit result masqueraded as read-only commit-review")
 }
}

func TestGitCommitRejectsArbitraryGitArgumentsAndInvalidMessages(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 sha:=strings.Repeat("a",64)
 for _,tc:=range []RemoteAction{
  {Operation:RemoteActionGitCommit,Content:"ok",ExpectedSHA256:sha},
  {Operation:RemoteActionGitCommit,Content:"Bad\nInjected",ExpectedSHA256:sha},
  {Operation:RemoteActionGitCommit,Content:"one\x1b[31m",ExpectedSHA256:sha},
  {Operation:RemoteActionGitCommit,Content:"$(command)",ExpectedSHA256:sha},
  {Operation:RemoteActionGitCommit,Content:"Good message",ExpectedSHA256:"bad"},
  {Operation:RemoteActionGitCommit,Content:"Good message",ExpectedSHA256:sha,Path:".git/config"},
  {Operation:RemoteActionGitCommit,Content:"Good message",ExpectedSHA256:sha,TaskID:1},
  {Operation:RemoteActionGitCommit,Content:"Good message",ExpectedSHA256:sha,Offset:1},
 }{
  client,_,_:=participants(t)
  tc.RequestID=id
  wire,err:=SealRemoteAction(client,tc)
  if err==nil||len(wire)!=0{t.Fatalf("unexpected capability accepted: %+v",tc)}
 }
 client,host,_:=participants(t)
 // A compromised relay can only inject an encrypted forged object if it
 // somehow holds the key. The host still refuses unknown/extra fields.
 raw:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.commit","content":"Safe message","expected_sha256":"`+sha+`","command":"sh"}`)
 frame,err:=client.Seal(raw)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(host,frame);err==nil||host.Established(){
  t.Fatal("extra Git command capability did not poison E2EE channel")
 }
}
