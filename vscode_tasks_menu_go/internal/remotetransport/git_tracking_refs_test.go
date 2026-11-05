package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestGitTrackingRefsTypedE2EEAndDistinctAEAD(t *testing.T){
 client,agent,_:=participants(t)
 action:=RemoteAction{RequestID:testActionID,Operation:RemoteActionGitTrackingRefs}
 frame,err:=SealRemoteAction(client,action)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(frame,[]byte("git.tracking_refs")){t.Fatal("Relay saw operation")}
 opened,err:=OpenRemoteAction(agent,frame)
 if err!=nil||opened!=action{t.Fatalf("opened action: %+v %v",opened,err)}
 oid:=strings.Repeat("a",40)
 value:="GIT_TRACKING_V1\norigin/main "+oid+"\n"
 reply,err:=SealRemoteGitTrackingRefsResult(agent,testActionID,[]byte(value))
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(reply,[]byte("origin/main")){t.Fatal("Relay saw private Git ref name")}
 got,err:=OpenRemoteGitTrackingRefsResult(client,reply,testActionID)
 if err!=nil||got!=value{t.Fatalf("AEAD ref reply: %q %v",got,err)}
 for _,bad:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionGitTrackingRefs,Path:"origin/main"},
  {RequestID:testActionID,Operation:RemoteActionGitTrackingRefs,Content:"fetch"},
  {RequestID:testActionID,Operation:RemoteActionGitTrackingRefs,TaskID:1},
  {RequestID:testActionID,Operation:RemoteActionGitTrackingRefs,Offset:1},
 }{
  cl,_,_:=participants(t)
  if frame,err:=SealRemoteAction(cl,bad);err==nil||len(frame)!=0{
   t.Fatalf("tracking metadata accepted arguments: %+v",bad)
  }
 }
}
func TestGitTrackingRefsRejectMalformedOIDPathAndCrossTypeReplies(t *testing.T){
 oid:=strings.Repeat("a",40)
 invalid:=[]string{
  "origin/main "+oid+"\n",
  "GIT_TRACKING_V1\norigin/main "+strings.Repeat("F",40)+"\n",
  "GIT_TRACKING_V1\norigin/main abc\n",
  "GIT_TRACKING_V1\norigin/../main "+oid+"\n",
  "GIT_TRACKING_V1\norigin/main "+oid+"\norigin/main "+oid+"\n",
  "GIT_TRACKING_V1\norigin/main "+oid+"\n"+strings.Repeat("o/main "+oid+"\n",32),
  "GIT_TRACKING_V1\norigin/main "+oid,
  "GIT_TRACKING_V1\norigin/HEAD "+oid+"\n",
 }
 for i,sample:=range invalid{
  if ValidRemoteGitTrackingRefsReport(sample){t.Fatalf("invalid sample %d accepted",i)}
  cl,agent,_:=participants(t)
  if frame,err:=SealRemoteGitTrackingRefsResult(agent,testActionID,[]byte(sample));
   err==nil||len(frame)!=0{t.Fatalf("invalid sample %d E2EE-sealed",i)}
  cl.Close()
 }
 cl,agent,_:=participants(t)
 wrong,err:=SealRemoteGitRemotesResult(agent,testActionID,[]byte("GIT_REMOTES_V1\norigin\n"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitTrackingRefsResult(cl,wrong,testActionID);err==nil||cl.Established(){
  t.Fatal("Git remote name envelope confused with tracking refs AEAD")
 }
}
