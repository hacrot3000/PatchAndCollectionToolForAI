package remotetransport

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "strings"
 "testing"
)

func TestGitUnstageTypedE2EEAndSeparatedResult(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 digest:=sha256.Sum256([]byte("complete staged diff"))
 client,agent,_:=participants(t)
 sealed,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionGitUnstage,Path:"sub/f name.txt",
  ExpectedSHA256:hex.EncodeToString(digest[:]),
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(sealed,[]byte("sub/f name.txt"))||
  bytes.Contains(sealed,[]byte(hex.EncodeToString(digest[:]))){
  t.Fatal("Relay obtained unstage file or staged-diff CAS hash")
 }
 decoded,err:=OpenRemoteAction(agent,sealed)
 if err!=nil||decoded.Operation!=RemoteActionGitUnstage||
  decoded.Path!="sub/f name.txt"||decoded.ExpectedSHA256!=hex.EncodeToString(digest[:]){
  t.Fatalf("invalid typed unstage request: %+v %v",decoded,err)
 }
 reply,err:=SealRemoteGitUnstageResult(agent,id,[]byte("unstaged"))
 if err!=nil{t.Fatal(err)}
 result,err:=OpenRemoteGitUnstageResult(client,reply,id)
 if err!=nil||result!="unstaged"{t.Fatalf("wrong authenticated unstage result %q %v",result,err)}
}

func TestGitUnstageRejectsArbitraryShellPathFlagsAndExtraJSON(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 good:=strings.Repeat("a",64)
 for _,a:=range []RemoteAction{
  {Operation:RemoteActionGitUnstage,Path:"src/a.txt",ExpectedSHA256:""},
  {Operation:RemoteActionGitUnstage,Path:"src/a.txt",ExpectedSHA256:strings.Repeat("X",64)},
  {Operation:RemoteActionGitUnstage,Path:"../outside",ExpectedSHA256:good},
  {Operation:RemoteActionGitUnstage,Path:"/etc/passwd",ExpectedSHA256:good},
  {Operation:RemoteActionGitUnstage,Path:"--cached",ExpectedSHA256:good},
  {Operation:RemoteActionGitUnstage,Path:"file.txt",ExpectedSHA256:good,Content:"sh"},
  {Operation:RemoteActionGitUnstage,Path:"file.txt",ExpectedSHA256:good,TaskID:2},
 }{
  a.RequestID=id
  client,_,_:=participants(t)
  f,e:=SealRemoteAction(client,a)
  if e==nil||len(f)!=0{t.Fatalf("unexpected Git unstage capability: %+v",a)}
 }
 client,agent,_:=participants(t)
 raw:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.unstage","path":"src/a.txt","expected_sha256":"`+good+`","command":"sh"}`)
 frame,err:=client.Seal(raw)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(agent,frame);err==nil||agent.Established(){
  t.Fatal("forged Git unstage command not rejected by authenticated decoder")
 }
}

func TestGitUnstageResultCannotMasqueradeAsStageOrDiff(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,agent,_:=participants(t)
 frame,err:=SealRemoteGitUnstageResult(agent,id,[]byte("unstaged"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitStageResult(client,frame,id);err==nil||client.Established(){
  t.Fatal("unstage result accepted as Git stage response")
 }
 client,agent,_=participants(t)
 frame,err=SealRemoteGitStagedDiffResult(agent,id,[]byte("diff --git"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitUnstageResult(client,frame,id);err==nil||client.Established(){
  t.Fatal("untrusted diff accepted as unstage success")
 }
}
