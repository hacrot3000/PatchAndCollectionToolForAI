package remotetransport

import "testing"

func TestRemoteTwoPickInputMetadataE2EERoundTrip(t *testing.T){
 client,agent,_:=participants(t)
 metadata:=RemoteTaskInspect{
  TaskID:23,Eligible:true,
  InputID:"region",Choices:[]string{"us","eu"},
  InputID2:"mode",Choices2:[]string{"debug","release","preview"},
 }
 frame,err:=SealRemoteActionTasksInspectResult(agent,testActionID,metadata)
 if err!=nil{t.Fatal(err)}
 opened,err:=OpenRemoteActionTasksInspectResult(client,frame,testActionID,23)
 if err!=nil||opened.InputID2!="mode"||len(opened.Choices2)!=3||
  opened.Choices2[1]!="release" {
  t.Fatalf("second choice lost or replaced: %+v %v",opened,err)
 }
}

func TestRemoteTwoPickRejectsMalformedMetadataAndOversizedIndexes(t *testing.T){
 for _,candidate:=range []RemoteTaskInspect{
  {TaskID:23,Eligible:true,InputID2:"mode",Choices2:[]string{"a"}},
  {TaskID:23,Eligible:true,InputID:"mode",Choices:[]string{"a"},
   InputID2:"mode",Choices2:[]string{"b"}},
  {TaskID:23,Eligible:true,InputID:"region",Choices:[]string{"us"},
   InputID2:"mode",Choices2:[]string{}},
  {TaskID:23,Eligible:true,InputID:"region",Choices:[]string{"us"},
   InputType:"promptString",InputID2:"mode",Choices2:[]string{"a"}},
 }{
  _,agent,_:=participants(t)
  if _,err:=SealRemoteActionTasksInspectResult(agent,testActionID,candidate);err==nil{
   t.Fatalf("malformed second pick was sealed: %+v",candidate)
  }
 }
 client,agent,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionTasksRun,TaskID:23,Choice:36,
 })
 if err!=nil{t.Fatal(err)}
 selected,err:=OpenRemoteAction(agent,request)
 if err!=nil||selected.Choice!=36{t.Fatalf("combined max index failed: %+v %v",selected,err)}
 _,other,_:=participants(t)
 if _,err:=SealRemoteAction(other,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionTasksRun,TaskID:23,Choice:37,
 });err==nil{t.Fatal("37 exceeded the combined six-by-six bound")}
}
