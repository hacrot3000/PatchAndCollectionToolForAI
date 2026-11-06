package remotetransport

import (
 "encoding/json"
)

// Task dependency inspection never grants execution and cannot accept a
// filename, command, user-controlled graph, or task definition. The ONLY
// client selection is an E2EE-issued stable Task.ID.
const RemoteActionTasksDependencies="tasks.dependencies"
const RemoteTaskDependenciesProtocol="taskdeck-e2ee-task-dependencies-v1"
const MaxRemoteTaskDependencyNodes=8
const MaxRemoteTaskDependencyEdges=8

type RemoteTaskDependencies struct{
 TaskID int
 Available bool
 RunAvailable bool
 Order []int
 Edges [][2]int
}
type remoteTaskDependenciesWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 TaskID int `json:"task_id"`
 Available bool `json:"available"`
 RunAvailable bool `json:"run_available"`
 Order []int `json:"order"`
 Edges [][2]int `json:"edges"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func ValidRemoteTaskDependencies(result RemoteTaskDependencies)bool{
 if !validSelectedTaskID(result.TaskID)||
  len(result.Order)>MaxRemoteTaskDependencyNodes||
  len(result.Edges)>MaxRemoteTaskDependencyEdges{return false}
 if !result.Available{
  return !result.RunAvailable&&len(result.Order)==0&&len(result.Edges)==0
 }
 if result.RunAvailable{
  if len(result.Order)<2||len(result.Order)>4||
   len(result.Edges)!=len(result.Order)-1{return false}
  for i,e:=range result.Edges{
   if e[0]!=result.Order[i]||e[1]!=result.Order[i+1]{return false}
  }
 }
 if len(result.Order)==0||result.Order[len(result.Order)-1]!=result.TaskID{
  return false
 }
 positions:=make(map[int]int,len(result.Order))
 for i,id:=range result.Order{
  if !validSelectedTaskID(id){return false}
  if _,dup:=positions[id];dup{return false}
  positions[id]=i
 }
 seen:=map[[2]int]bool{}
 for _,edge:=range result.Edges{
  left,ok:=positions[edge[0]]
  right,ok2:=positions[edge[1]]
  if !ok||!ok2||left>=right||seen[edge]{
   return false
  }
  seen[edge]=true
 }
 return true
}
func SealRemoteTaskDependencies(ch *Channel,id string,result RemoteTaskDependencies)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !ValidRemoteTaskDependencies(result){return nil,ErrInvalidRemoteAction}
 if result.Order==nil{result.Order=[]int{}}
 if result.Edges==nil{result.Edges=[][2]int{}}
 clear,err:=json.Marshal(remoteTaskDependenciesWire{
  Type:RemoteTaskDependenciesProtocol,RequestID:id,
  TaskID:result.TaskID,Available:result.Available,RunAvailable:result.RunAvailable,
  Order:result.Order,Edges:result.Edges,RemoteControlAvailable:false,
 })
 if err!=nil||len(clear)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(clear)
}
func OpenRemoteTaskDependencies(ch *Channel,frame []byte,wantID string,taskID int)(RemoteTaskDependencies,error){
 denied:=RemoteTaskDependencies{}
 if ch==nil||!ch.Established()||!validActionID(wantID)||
  !validSelectedTaskID(taskID){return denied,ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 fields,err:=decodeActionObject(clear,"type","request_id","task_id","available",
  "run_available","order","edges","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var wire remoteTaskDependenciesWire
 if json.Unmarshal(fields["type"],&wire.Type)!=nil||
  json.Unmarshal(fields["request_id"],&wire.RequestID)!=nil||
  json.Unmarshal(fields["task_id"],&wire.TaskID)!=nil||
  (string(fields["available"])!="true"&&string(fields["available"])!="false")||
  (string(fields["run_available"])!="true"&&string(fields["run_available"])!="false")||
  json.Unmarshal(fields["available"],&wire.Available)!=nil||
  json.Unmarshal(fields["run_available"],&wire.RunAvailable)!=nil||
  json.Unmarshal(fields["order"],&wire.Order)!=nil||
  json.Unmarshal(fields["edges"],&wire.Edges)!=nil||
  wire.Type!=RemoteTaskDependenciesProtocol||wire.RequestID!=wantID||
  wire.TaskID!=taskID||wire.Order==nil||wire.Edges==nil{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 result:=RemoteTaskDependencies{
  TaskID:wire.TaskID,Available:wire.Available,RunAvailable:wire.RunAvailable,
  Order:wire.Order,Edges:wire.Edges,
 }
 if !ValidRemoteTaskDependencies(result){ch.Close();return denied,ErrInvalidRemoteAction}
 return result,nil
}
