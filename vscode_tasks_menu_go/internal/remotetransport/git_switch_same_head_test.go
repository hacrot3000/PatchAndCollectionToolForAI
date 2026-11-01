package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestGitSwitchSameHeadTypedE2EEAndResultDomainIsolation(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 digest:=strings.Repeat("a",64)
 c,h,_:=participants(t)
 req,err:=SealRemoteAction(c,RemoteAction{RequestID:id,
  Operation:RemoteActionGitSwitchSameHead,Path:"safe-target",ExpectedSHA256:digest})
 if err!=nil{t.Fatal(err)}
 for _,v:=range []string{"git.switch_same_head","safe-target",digest}{
  if bytes.Contains(req,[]byte(v)){t.Fatal("PHP saw encrypted switch instruction")}
 }
 action,err:=OpenRemoteAction(h,req)
 if err!=nil||action.Operation!=RemoteActionGitSwitchSameHead||
  action.Path!="safe-target"||action.ExpectedSHA256!=digest{
  t.Fatalf("wrong E2EE action: %+v %v",action,err)
 }
 oid:=[]byte(strings.Repeat("b",40))
 res,err:=SealRemoteGitSwitchResult(h,id,oid)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteGitSwitchResult(c,res,id)
 if err!=nil||got!=string(oid){t.Fatalf("switch reply: %q %v",got,err)}
 c,h,_=participants(t)
 res,err=SealRemoteGitSwitchResult(h,id,oid)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteGitSwitchReviewResult(c,res,id);err==nil||c.Established(){
  t.Fatal("writable switch result impersonated review")
 }
}

func TestGitSwitchSameHeadRejectsUnsafeNamesAndForeignFields(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 digest:=strings.Repeat("a",64)
 for _,a:=range []RemoteAction{
  {Operation:RemoteActionGitSwitchSameHead,Path:"../main",ExpectedSHA256:digest},
  {Operation:RemoteActionGitSwitchSameHead,Path:"main/branch",ExpectedSHA256:digest},
  {Operation:RemoteActionGitSwitchSameHead,Path:"safe-target",ExpectedSHA256:"bad"},
  {Operation:RemoteActionGitSwitchSameHead,Path:"safe-target",ExpectedSHA256:digest,Content:"sh"},
  {Operation:RemoteActionGitSwitchSameHead,Path:"safe-target",ExpectedSHA256:digest,TaskID:1},
  {Operation:RemoteActionGitSwitchSameHead,Path:"safe-target",ExpectedSHA256:digest,Offset:1},
  {Operation:RemoteActionGitSwitchReview,Path:"safe-target",Content:"sh"},
  {Operation:RemoteActionGitSwitchReview,Path:"safe-target",ExpectedSHA256:digest},
  {Operation:RemoteActionGitSwitchReview,Path:"--force"},
 }{
  c,_,_:=participants(t)
  a.RequestID=id
  req,err:=SealRemoteAction(c,a)
  if err==nil||len(req)!=0{t.Fatalf("accepted bad typed action %+v",a)}
 }
 c,h,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef","operation":"git.switch_same_head","path":"safe-target","expected_sha256":"`+digest+`","command":"checkout --force"}`)
 req,err:=c.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(h,req);err==nil||h.Established(){
  t.Fatal("arbitrary shell/checkout fields were accepted")
 }
}
