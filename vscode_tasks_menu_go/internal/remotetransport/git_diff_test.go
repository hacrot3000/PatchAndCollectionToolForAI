package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestGitDiffTypedE2EEBoundAndNoShellCommand(t *testing.T){
 client,agent,_:=participants(t)
 const id="0123456789abcdef0123456789abcdef"
 wire,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionGitDiff,Path:"src/program.go",
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(wire,[]byte("src/program.go")){t.Fatal("Relay saw Git path")}
 action,err:=OpenRemoteAction(agent,wire)
 if err!=nil||action.Operation!=RemoteActionGitDiff||action.Path!="src/program.go"{
  t.Fatalf("invalid E2EE Git diff action: %+v %v",action,err)
 }
 diff:=[]byte("diff --git a/file b/file\n"+strings.Repeat("+hello world\n",190))
 sealed,err:=SealRemoteGitDiffResult(agent,id,diff)
 if err!=nil||len(sealed)>MaxEncryptedGitDiffFrame{
  t.Fatalf("cannot return bounded Git diff: %v",err)
 }
 result,err:=OpenRemoteGitDiffResult(client,sealed,id)
 if err!=nil||result!=string(diff){t.Fatalf("bad authenticated Git diff %v",err)}
}
func TestGitDiffRejectsInvalidPathsAndOverlargeData(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 for _,p:=range []string{"","../secret","/etc/passwd","a\\b","file:\\x00"}{
  client,_,_:=participants(t)
  frame,err:=SealRemoteAction(client,RemoteAction{
   RequestID:id,Operation:RemoteActionGitDiff,Path:p,
  })
  if err==nil||len(frame)>0{t.Fatalf("untrusted Git source accepted: %q",p)}
 }
 _,agent,_:=participants(t)
 if frame,err:=SealRemoteGitDiffResult(agent,id,[]byte(strings.Repeat("a",RemoteGitDiffMaxBytes+1)));
  err==nil||len(frame)>0{t.Fatal("truncated Git output would be misleading")}
}
func TestGitDiffResultCannotMasqueradeAsGitStatusOrAddCapabilities(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 client,agent,_:=participants(t)
 sealed,err:=SealRemoteGitDiffResult(agent,id,[]byte("+change"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitStatusResult(client,sealed,id);err==nil||client.Established(){
  t.Fatal("Git diff was accepted as status or did not poison channel")
 }
 client,agent,_=participants(t)
 sealed,err=SealRemoteGitDiffResult(agent,id,[]byte("+change"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitDiffResult(client,sealed,"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");err==nil||client.Established(){
  t.Fatal("wrong E2EE request ID accepted")
 }
 client,agent,_=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-git-diff-result-v1","request_id":"0123456789abcdef0123456789abcdef","ok":true,"remote_control_available":false,"content":"","sha256":"abc","command":"sh"}`)
 frame,err:=agent.Seal(forged);if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitDiffResult(client,frame,id);err==nil||client.Established(){
  t.Fatal("generic shell command injected through Git diff result")
 }
}
