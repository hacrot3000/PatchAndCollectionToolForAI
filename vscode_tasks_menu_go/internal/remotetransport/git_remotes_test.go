package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestRemoteGitRemotesAEADNamesOnlyAndNoAdditionalArguments(t *testing.T){
 client,agent,_:=participants(t)
 action:=RemoteAction{RequestID:testActionID,Operation:RemoteActionGitRemotes}
 ciphertext,err:=SealRemoteAction(client,action)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(ciphertext,[]byte(RemoteActionGitRemotes)){
  t.Fatal("Relay saw Git remote-name request operation")
 }
 opened,err:=OpenRemoteAction(agent,ciphertext)
 if err!=nil||opened!=action{t.Fatalf("typed request mismatch: %+v %v",opened,err)}
 names:="GIT_REMOTES_V1\norigin\nupstream\n"
 reply,err:=SealRemoteGitRemotesResult(agent,testActionID,[]byte(names))
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(reply,[]byte(names)){
  t.Fatal("Relay saw Git remote names plaintext")
 }
 value,err:=OpenRemoteGitRemotesResult(client,reply,testActionID)
 if err!=nil||value!=names{t.Fatalf("authenticated names mismatch: %q %v",value,err)}
 for _,action:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionGitRemotes,Path:"origin"},
  {RequestID:testActionID,Operation:RemoteActionGitRemotes,TaskID:1},
  {RequestID:testActionID,Operation:RemoteActionGitRemotes,Content:"fetch"},
  {RequestID:testActionID,Operation:RemoteActionGitRemotes,ExpectedSHA256:strings.Repeat("a",64)},
 }{
  caller,_,_:=participants(t)
  if frame,err:=SealRemoteAction(caller,action);err==nil||len(frame)>0{
   t.Fatalf("parameter smuggling accepted: %+v",action)
  }
 }
}
func TestRemoteGitRemotesAEADFailsClosedOnURLOrMalformedNames(t *testing.T){
 invalid:=[]string{
  "origin\n",
  "GIT_REMOTES_V1\norigin https://token@example.invalid/repo\n",
  "GIT_REMOTES_V1\nhttps://token@example.invalid\n",
  "GIT_REMOTES_V1\norigin\norigin\n",
  "GIT_REMOTES_V1\norigin\r\n",
  "GIT_REMOTES_V1\nupstream",
  "GIT_REMOTES_V1\n"+strings.Repeat("x",65)+"\n",
  "GIT_REMOTES_V1\n"+strings.Repeat("a\n",17),
  "GIT_REMOTES_V1\n..hidden\n",
 }
 for _,sample:=range invalid{
  t.Run(strings.ReplaceAll(strings.ReplaceAll(sample,"\n","-"),"\r","CR"),func(t *testing.T){
   client,agent,_:=participants(t)
   reply,err:=SealRemoteGitRemotesResult(agent,testActionID,[]byte(sample))
   if err!=nil{t.Fatal(err)}
   if value,err:=OpenRemoteGitRemotesResult(client,reply,testActionID);
    err==nil||value!=""||client.Established(){
    t.Fatalf("malformed result trusted or E2EE session not retired: %q %v",value,err)
   }
  })
 }
 client,agent,_:=participants(t)
 wrong,err:=SealRemoteGitBranchesResult(agent,testActionID,[]byte("origin"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitRemotesResult(client,wrong,testActionID);
  err==nil||client.Established(){
  t.Fatal("branch-list AEAD reply confused with remote-name response")
 }
}
