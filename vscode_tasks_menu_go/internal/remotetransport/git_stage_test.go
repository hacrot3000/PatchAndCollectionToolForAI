package remotetransport

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "strings"
 "testing"
)

func TestGitStageTypedE2EECASAndSeparatedResult(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 source:=[]byte("before\n")
 sum:=sha256.Sum256(source)
 client,agent,_:=participants(t)
 frame,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionGitStage,
  Path:"nested/file name.txt",ExpectedSHA256:hex.EncodeToString(sum[:]),
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(frame,[]byte("nested/file name.txt"))||
  bytes.Contains(frame,[]byte(hex.EncodeToString(sum[:]))){
  t.Fatal("Relay saw Git index path or expected file hash")
 }
 request,err:=OpenRemoteAction(agent,frame)
 if err!=nil||request.Operation!=RemoteActionGitStage||
  request.Path!="nested/file name.txt"||
  request.ExpectedSHA256!=hex.EncodeToString(sum[:]){
  t.Fatalf("invalid strict stage schema %+v %v",request,err)
 }
 oid:=[]byte(strings.Repeat("a",40))
 response,err:=SealRemoteGitStageResult(agent,id,oid)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteGitStageResult(client,response,id)
 if err!=nil||got!=string(oid){t.Fatalf("stage result %q %v",got,err)}
}

func TestGitStageRejectsInvalidCASPathsAndExtraCapabilities(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 goodSHA:=strings.Repeat("a",64)
 for _,tc:=range []RemoteAction{
  {Operation:RemoteActionGitStage,Path:"src/file.txt",ExpectedSHA256:""},
  {Operation:RemoteActionGitStage,Path:"src/file.txt",ExpectedSHA256:strings.Repeat("X",64)},
  {Operation:RemoteActionGitStage,Path:"../secret",ExpectedSHA256:goodSHA},
  {Operation:RemoteActionGitStage,Path:"/etc/passwd",ExpectedSHA256:goodSHA},
  {Operation:RemoteActionGitStage,Path:"--help",ExpectedSHA256:goodSHA},
  {Operation:RemoteActionGitStage,Path:"src/file.txt",ExpectedSHA256:goodSHA,Content:"sh"},
  {Operation:RemoteActionGitStage,Path:"src/file.txt",ExpectedSHA256:goodSHA,TaskID:4},
 }{
  tc.RequestID=id
  client,_,_:=participants(t)
  sealed,err:=SealRemoteAction(client,tc)
  if err==nil||len(sealed)!=0{t.Fatalf("unsafe Git write admitted: %+v",tc)}
 }
 client,agent,_:=participants(t)
 extra:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.stage","path":"x.txt","expected_sha256":"`+goodSHA+`","command":"sh"}`)
 forged,err:=client.Seal(extra)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(agent,forged);err==nil||agent.Established(){
  t.Fatal("extra Git shell capability did not poison authenticated channel")
 }
}

func TestGitStageResultCannotMasqueradeAsReadOnlyGit(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,agent,_:=participants(t)
 response,err:=SealRemoteGitStageResult(agent,id,[]byte(strings.Repeat("a",40)))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitHeadDiffResult(client,response,id);err==nil||client.Established(){
  t.Fatal("stage result confused with read-only Git HEAD diff")
 }
 client,agent,_=participants(t)
 response,err=SealRemoteGitHeadDiffResult(agent,id,[]byte("diff --git ..."))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitStageResult(client,response,id);err==nil||client.Established(){
  t.Fatal("read-only Git diff masqueraded as stage commit")
 }
}
