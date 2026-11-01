package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestGitCommitReviewTypedE2EEAndDedicatedResponse(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,agent,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionGitCommitReview,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(request,[]byte("git.commit_review")){
  t.Fatal("Relay saw Git commit review operation")
 }
 decoded,err:=OpenRemoteAction(agent,request)
 if err!=nil||decoded.Operation!=RemoteActionGitCommitReview||decoded.Path!=""{
  t.Fatalf("incorrect authenticated Git commit review: %+v %v",decoded,err)
 }
 content:=[]byte("HEAD "+strings.Repeat("a",40)+"\nBRANCH refs/heads/main\n-- COMPLETE STAGED DIFF --\n+new\n")
 sealed,err:=SealRemoteGitCommitReviewResult(agent,id,content)
 if err!=nil{t.Fatal(err)}
 reviewed,err:=OpenRemoteGitCommitReviewResult(client,sealed,id)
 if err!=nil||reviewed!=string(content){t.Fatalf("typed review failed: %q %v",reviewed,err)}
}

func TestGitCommitReviewRejectsForeignArgumentsAndTypeSwap(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 for _,action:=range []RemoteAction{
  {Operation:RemoteActionGitCommitReview,Path:"HEAD"},
  {Operation:RemoteActionGitCommitReview,Content:"git commit -m hacked"},
  {Operation:RemoteActionGitCommitReview,TaskID:9},
  {Operation:RemoteActionGitCommitReview,ExpectedSHA256:strings.Repeat("a",64)},
 }{
  action.RequestID=id
  client,_,_:=participants(t)
  frame,err:=SealRemoteAction(client,action)
  if err==nil||len(frame)!=0{t.Fatalf("foreign commit capability accepted: %+v",action)}
 }
 client,agent,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.commit_review","message":"hacked"}`)
 frame,err:=client.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(agent,frame);err==nil||agent.Established(){
  t.Fatal("forged commit message escaped strict typed schema")
 }
 client,agent,_=participants(t)
 response,err:=SealRemoteGitCommitReviewResult(agent,id,[]byte("readonly"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitHeadDiffResult(client,response,id);err==nil||client.Established(){
  t.Fatal("commit review result accepted as another Git operation")
 }
}

func TestGitCommitReviewRefusesTruncatedOutput(t *testing.T){
 client,agent,_:=participants(t)
 const id="0123456789abcdef0123456789abcdef"
 frame,err:=SealRemoteGitCommitReviewResult(agent,id,[]byte(strings.Repeat("x",RemoteGitDiffMaxBytes+1)))
 if err==nil||len(frame)!=0{t.Fatal("oversized commit preview was silently truncated")}
 _=client
}
