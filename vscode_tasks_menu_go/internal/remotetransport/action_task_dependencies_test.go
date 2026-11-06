package remotetransport

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
)

func TestRemoteTaskDependenciesStrictE2EEAndTopology(t *testing.T){
 valid:=RemoteTaskDependencies{
  TaskID:17,Available:true,Order:[]int{2,11,17},
  Edges:[][2]int{{2,11},{11,17}},
 }
 client,agent,_:=participants(t)
 frame,err:=SealRemoteTaskDependencies(agent,testActionID,valid)
 if err!=nil{t.Fatal(err)}
 output,err:=OpenRemoteTaskDependencies(client,frame,testActionID,17)
 if err!=nil||output.TaskID!=17||len(output.Order)!=3||len(output.Edges)!=2{
  t.Fatalf("typed E2EE graph: %+v %v",output,err)
 }
 for _,bad:=range []RemoteTaskDependencies{
  {TaskID:0,Available:true,Order:[]int{1}},
  {TaskID:17,Available:true,Order:[]int{}},
  {TaskID:17,Available:true,Order:[]int{17,17}},
  {TaskID:17,Available:true,Order:[]int{17,11}},
  {TaskID:17,Available:true,Order:[]int{2,17},Edges:[][2]int{{17,2}}},
  {TaskID:17,Available:true,Order:[]int{2,17},Edges:[][2]int{{3,17}}},
  {TaskID:17,Available:true,Order:[]int{2,17},Edges:[][2]int{{2,17},{2,17}}},
  {TaskID:17,Available:false,Order:[]int{17}},
  {TaskID:17,Available:false,Edges:[][2]int{{1,17}}},
  {TaskID:17,Available:true,Order:[]int{1,2,3,4,5,6,7,8,17}},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteTaskDependencies(c,testActionID,bad);
   !errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("invalid host graph accepted: %+v %v",bad,err)
  }
 }
 raw,err:=json.Marshal(remoteTaskDependenciesWire{
  Type:RemoteTaskDependenciesProtocol,RequestID:testActionID,TaskID:17,
  Available:true,Order:valid.Order,Edges:valid.Edges,
 })
 if err!=nil{t.Fatal(err)}
 for name,content:=range map[string]string{
  "unknown command":strings.Replace(string(raw),`"available":true`,`"available":true,"command":"rm -rf /"`,1),
  "repeated ID":strings.Replace(string(raw),`"task_id":17`,`"task_id":17,"task_id":1`,1),
  "not same task":strings.Replace(string(raw),`"task_id":17`,`"task_id":33`,1),
  "control":strings.Replace(string(raw),`"remote_control_available":false`,`"remote_control_available":true`,1),
  "null":strings.Replace(string(raw),`"available":true`,`"available":null`,1),
  "extra edge":strings.Replace(string(raw),`"edges":[[2,11],[11,17]]`,`"edges":[[2,11],[17,2]]`,1),
 }{
  t.Run(name,func(t *testing.T){
   c,a,_:=participants(t)
   frame,err:=a.Seal([]byte(content))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteTaskDependencies(c,frame,testActionID,17);
    !errors.Is(err,ErrInvalidRemoteAction){t.Fatalf("spoof accepted: %v",err)}
   if c.Established(){t.Fatal("malformed E2EE graph did not close channel")}
  })
 }
}

func TestRemoteTaskDependenciesFixedNativeIDOnly(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  action,err:=OpenRemoteAction(agent,frame)
  if err!=nil||action.Operation!=RemoteActionTasksDependencies||
   action.TaskID!=25||action.FileIndex!=0||
   action.Message!=""||action.Text!=""||action.Snapshot!=""||
   action.Choice!=0||action.TerminalInput!=""{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteTaskDependencies(agent,action.RequestID,RemoteTaskDependencies{
   TaskID:25,Available:true,Order:[]int{5,25},
   Edges:[][2]int{{5,25}},
  })
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 out,err:=native.TaskDependencies(context.Background(),25)
 if err!=nil||len(out.Order)!=2||out.Order[1]!=25{
  t.Fatalf("native dependency E2EE failed: %+v %v",out,err)
 }
 for _,bad:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionTasksDependencies,TaskID:0},
  {RequestID:testActionID,Operation:RemoteActionTasksDependencies,TaskID:25,Text:"/etc/secret"},
  {RequestID:testActionID,Operation:RemoteActionTasksDependencies,TaskID:25,Message:"other"},
  {RequestID:testActionID,Operation:RemoteActionTasksDependencies,TaskID:25,Choice:1},
  {RequestID:testActionID,Operation:RemoteActionTasksDependencies,TaskID:25,FileIndex:1},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,bad);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("unexpected client-chosen dependency argument accepted: %+v",bad)
  }
 }
}
