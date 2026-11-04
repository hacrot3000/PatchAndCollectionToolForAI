package remotetransport

import (
 "context"
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

func TestRemoteTaskRunTrustedNativeClientTypedExchange(t *testing.T){
 client,agent,_:=participants(t)
 calls:=0
 transport:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(agent,frame)
  if err!=nil{return nil,err}
  if request.Operation!=RemoteActionTasksRun||request.TaskID!=145{
   return nil,ErrRemoteActionDenied
  }
  calls++
  return SealRemoteActionTaskRunResult(agent,request.RequestID,
   RemoteTaskRun{TaskID:145,ExitCode:9,Output:"safe text",Truncated:false})
 })
 native,err:=NewPingClient(client,transport)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 result,err:=native.RunTask(context.Background(),145)
 if err!=nil||result.TaskID!=145||result.ExitCode!=9||
  result.Output!="safe text"||calls!=1{
  t.Fatalf("native task.run exchanged wrong bound result: %+v %v",result,err)
 }
}
func TestRemoteTaskRunRefusesUnboundNativeTaskID(t *testing.T){
 client,_,_:=participants(t)
 native,err:=NewPingClient(client,pingExchangeFunc(
  func(ctx context.Context,frame []byte)([]byte,error){
   t.Fatal("invalid task ID was forwarded to Relay")
   return nil,ErrInvalidRemoteAction
  }))
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 if _,err:=native.RunTask(context.Background(),0);err==nil{
  t.Fatal("remote Task.ID zero was accepted")
 }
}

func TestRemoteTaskRunPickIndexIsBoundInE2EERequest(t *testing.T){
 client,agent,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionTasksRun,
  TaskID:123,Choice:2,
 })
 if err!=nil{t.Fatal(err)}
 action,err:=OpenRemoteAction(agent,request)
 if err!=nil||action.Operation!=RemoteActionTasksRun||
  action.TaskID!=123||action.Choice!=2{
  t.Fatalf("choice index not authenticated: %+v %v",action,err)
 }
}
func TestRemoteTaskRunChoiceCannotBeAttachedToOtherActions(t *testing.T){
 for _,item:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionTasksInspect,TaskID:1,Choice:1},
  {RequestID:testActionID,Operation:RemoteActionSessionPing,Choice:1},
  {RequestID:testActionID,Operation:RemoteActionTasksRun,TaskID:1,Choice:7},
  {RequestID:testActionID,Operation:RemoteActionTasksRun,TaskID:1,Choice:-1},
 }{
  client,_,_:=participants(t)
  if _,err:=SealRemoteAction(client,item);err==nil{
   t.Fatalf("illegal choice field was accepted: %+v",item)
  }
 }
}
