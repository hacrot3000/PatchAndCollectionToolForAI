package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func testGitCompareReport(name,head,remote string,ahead,behind string)string{
 return "GIT_COMPARE_V1\nREF "+name+"\nHEAD "+head+
  "\nTRACKING "+remote+"\nAHEAD "+ahead+"\nBEHIND "+behind+"\n"
}
func TestRemoteGitCompareTrackingE2EEBoundedAndTyped(t *testing.T){
 client,agent,_:=participants(t)
 name:="origin/feature/test"
 request:=RemoteAction{RequestID:testActionID,Operation:RemoteActionGitCompareTracking,Path:name}
 sealed,err:=SealRemoteAction(client,request)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(sealed,[]byte(name)){t.Fatal("relay learned private Git refname")}
 opened,err:=OpenRemoteAction(agent,sealed)
 if err!=nil||opened!=request{t.Fatalf("bad typed action: %+v %v",opened,err)}
 report:=testGitCompareReport(name,strings.Repeat("a",40),strings.Repeat("b",40),"2","4")
 answer,err:=SealRemoteGitCompareTrackingResult(agent,testActionID,[]byte(report))
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(answer,[]byte(name)){t.Fatal("relay learned Git comparison result")}
 result,err:=OpenRemoteGitCompareTrackingResult(client,answer,testActionID)
 if err!=nil||result!=report{t.Fatalf("comparison reply: %q %v",result,err)}
 for _,req:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionGitCompareTracking},
  {RequestID:testActionID,Operation:RemoteActionGitCompareTracking,Path:"--all"},
  {RequestID:testActionID,Operation:RemoteActionGitCompareTracking,Path:"origin/HEAD"},
  {RequestID:testActionID,Operation:RemoteActionGitCompareTracking,Path:"origin/main",TaskID:1},
  {RequestID:testActionID,Operation:RemoteActionGitCompareTracking,Path:"origin/main",Content:"fetch"},
  {RequestID:testActionID,Operation:RemoteActionGitCompareTracking,Path:"origin/main",Offset:1},
 }{
  cl,_,_:=participants(t)
  if frame,err:=SealRemoteAction(cl,req);err==nil||len(frame)>0{
   t.Fatalf("extra Git comparison action argument accepted: %+v",req)
  }
 }
}
func TestRemoteGitCompareTrackingRejectMalformedAndCrossTypeAEAD(t *testing.T){
 oid:=strings.Repeat("c",40)
 invalid:=[]string{
  "",
  testGitCompareReport("origin/HEAD",oid,oid,"0","0"),
  testGitCompareReport("origin/main",oid,oid,"-1","0"),
  testGitCompareReport("origin/main",oid,oid,"01","2"),
  testGitCompareReport("origin/main",oid,oid,"1000000001","0"),
  testGitCompareReport("origin/main",strings.Repeat("G",40),oid,"0","0"),
  testGitCompareReport("origin/main",oid,oid,"0","0")+"EXTRA\n",
  "GIT_COMPARE_V1\nREF origin/main\nHEAD "+oid+"\nTRACKING "+oid+"\nAHEAD 0\n",
 }
 for i,report:=range invalid{
  if ValidRemoteGitCompareTrackingReport(report){t.Fatalf("invalid report %d accepted",i)}
  cl,agent,_:=participants(t)
  if frame,err:=SealRemoteGitCompareTrackingResult(agent,testActionID,[]byte(report));
   err==nil||len(frame)!=0{
   t.Fatalf("invalid Git comparison result %d sealed",i)
  }
  cl.Close()
 }
 cl,agent,_:=participants(t)
 wrong,err:=SealRemoteGitTrackingRefsResult(agent,testActionID,
  []byte("GIT_TRACKING_V1\norigin/main "+oid+"\n"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitCompareTrackingResult(cl,wrong,testActionID);
  err==nil||cl.Established(){
  t.Fatal("local ref listing replay accepted as comparison AEAD")
 }
}
