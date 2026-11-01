package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestGitHeadDiffTypedE2EEAndResultIsolation(t *testing.T) {
 const id="0123456789abcdef0123456789abcdef"
 client,agent,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionGitHeadDiff,Path:"dir/file with spaces.go",
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(request,[]byte("dir/file with spaces.go")) {
  t.Fatal("Git HEAD path leaked to Relay")
 }
 decoded,err:=OpenRemoteAction(agent,request)
 if err!=nil||decoded.Operation!=RemoteActionGitHeadDiff||decoded.Path!="dir/file with spaces.go"{
  t.Fatalf("wrong typed head diff request: %+v %v",decoded,err)
 }
 data:=[]byte("diff --git a/dir/file b/dir/file\n-base\n+worktree\n")
 reply,err:=SealRemoteGitHeadDiffResult(agent,id,data)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteGitHeadDiffResult(client,reply,id)
 if err!=nil||got!=string(data){t.Fatalf("invalid authenticated result: %q %v",got,err)}
}

func TestGitHeadDiffRejectsPathsAndOversize(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 for _,path:=range []string{"","../../secret","/etc/passwd","a\\b","bad:rev","--no-ext-diff"} {
  client,_,_:=participants(t)
  wire,err:=SealRemoteAction(client,RemoteAction{
   RequestID:id,Operation:RemoteActionGitHeadDiff,Path:path,
  })
  if err==nil||len(wire)!=0 {
   t.Fatalf("invalid revision or Git path accepted: %q",path)
  }
 }
 _,agent,_:=participants(t)
 if wire,err:=SealRemoteGitHeadDiffResult(agent,id,[]byte(strings.Repeat("x",RemoteGitDiffMaxBytes+1)));err==nil||len(wire)!=0 {
  t.Fatal("oversize HEAD diff silently truncated")
 }
}

func TestGitHeadDiffCannotSwapWithOtherGitResultTypes(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,agent,_:=participants(t)
 swapped,err:=SealRemoteGitStagedDiffResult(agent,id,[]byte("staged"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitHeadDiffResult(client,swapped,id);err==nil||client.Established(){
  t.Fatal("staged result masqueraded as full HEAD diff")
 }
 client,agent,_=participants(t)
 swapped,err=SealRemoteGitHeadDiffResult(agent,id,[]byte("HEAD"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitDiffResult(client,swapped,id);err==nil||client.Established(){
  t.Fatal("HEAD diff masqueraded as unstaged diff")
 }
}

func TestGitHeadDiffForgedCommandShapedPathPoisonsE2EE(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,agent,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.diff_head","path":"--no-ext-diff"}`)
 frame,err:=client.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if action,err:=OpenRemoteAction(agent,frame);err==nil||action.Operation!=""||agent.Established(){
  t.Fatal("authenticated command-shaped Git revision was not rejected and E2EE poisoned")
 }
 // Input cannot be revived after one malformed authenticated message.
 if _,err:=SealRemoteGitHeadDiffResult(agent,id,[]byte("injected"));err==nil{
  t.Fatal("poisoned E2EE channel accepted an application response")
 }
}
