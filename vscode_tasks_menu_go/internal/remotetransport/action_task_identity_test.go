package remotetransport

import (
 "fmt"
 "testing"
)

func TestRemoteTasksPreviewStableIDsRoundTripAndAdaptiveFit(t *testing.T) {
 client,agent,_:=participants(t)
 labels:=make([]string,MaxRemoteTasksPreviewLabels)
 ids:=make([]int,MaxRemoteTasksPreviewLabels)
 for i:=range labels {labels[i]=fmt.Sprintf("Task %02d",i);ids[i]=i+1}
 preview:=RemoteTasksPreview{Total:42,Labels:labels,IDs:ids}
 frame,err:=SealRemoteActionTasksPreviewResult(agent,testActionID,preview)
 if err!=nil{t.Fatal(err)}
 opened,err:=OpenRemoteActionTasksPreviewResult(client,frame,testActionID)
 if err!=nil{t.Fatal(err)}
 if len(opened.IDs)==0||len(opened.IDs)!=len(opened.Labels)||
  opened.IDs[len(opened.IDs)-1]!=len(opened.IDs) {
  t.Fatalf("trim desynchronized labels and immutable IDs: %+v",opened)
 }
}

func TestRemoteTasksPreviewRejectsMismatchedIDs(t *testing.T) {
 for _,preview:=range []RemoteTasksPreview{
  {Total:1,Labels:[]string{"A"},IDs:[]int{}},
  {Total:1,Labels:[]string{"A"},IDs:[]int{-1}},
  {Total:1,Labels:[]string{"A"},IDs:[]int{2147483648}},
 }{
  _,agent,_:=participants(t)
  if _,err:=SealRemoteActionTasksPreviewResult(agent,testActionID,preview);err==nil{
   t.Fatalf("invalid label-ID binding accepted: %+v",preview)
  }
 }
}
