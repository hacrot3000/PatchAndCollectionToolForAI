package remotetransport

import (
 "strings"
 "testing"
)
func TestRemoteTaskRunAuthenticatedResult(t *testing.T){
 client,agent,_:=participants(t)
 res:=RemoteTaskRun{TaskID:123,ExitCode:4,Output:strings.Repeat("<>",80),Truncated:false}
 frame,err:=SealRemoteActionTaskRunResult(agent,testActionID,res)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteActionTaskRunResult(client,frame,testActionID,123)
 if err!=nil||got.TaskID!=123||got.ExitCode!=4||got.Output==""{
  t.Fatalf("invalid authenticated result: %+v %v",got,err)
 }
}
func TestRemoteTaskRunWrongIDPoisoned(t *testing.T){
 client,agent,_:=participants(t)
 frame,err:=SealRemoteActionTaskRunResult(agent,testActionID,
  RemoteTaskRun{TaskID:3,ExitCode:0,Output:"ok"})
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteActionTaskRunResult(client,frame,testActionID,4);err==nil{
  t.Fatal("wrong selected task ID accepted")
 }
 if client.Established(){t.Fatal("wrong-ID result did not poison E2EE channel")}
}
func TestRemoteTaskRunRejectsMalformedLocalResult(t *testing.T){
 _,agent,_:=participants(t)
 for _,result:=range []RemoteTaskRun{
  {TaskID:0},{TaskID:1,ExitCode:-1},
  {TaskID:1,ExitCode:256},{TaskID:1,Output:strings.Repeat("a",193)},
 } {
  if _,err:=SealRemoteActionTaskRunResult(agent,testActionID,result);err==nil{
   t.Fatalf("invalid local result accepted: %+v",result)
  }
 }
}
