package server

import(
 "bytes"
 "context"
 "os"
 "path/filepath"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// The selected ID must have been sent to this authenticated E2EE session.
// No dependency can be executed by this read-only action. Root and
// tasks.json are revalidated before and after the host graph traversal.
func (s *Server)readRemoteTaskDependencies(ctx context.Context,
 session *remoteTaskDeckSession,taskID int,
)(remotetransport.RemoteTaskDependencies,error){
 denied:=remotetransport.ErrRemoteActionDenied
 empty:=remotetransport.RemoteTaskDependencies{}
 if s==nil||ctx==nil||ctx.Err()!=nil||session==nil||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel)||
  os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")!="1"||
  !session.taskPageReady||!session.taskIssuedIDs[taskID]||
  session.taskPageHash==([32]byte{})||
  s.Workspace==""{return empty,denied}
 // Opt-out is an authenticated, content-free response rather than an
 // E2EE session-killing error. GUI can show that the host hasn't enabled
 // dependency browsing without revealing the underlying graph.
 if os.Getenv("TASKDECK_REMOTE_TASKS_DEPS")!="1"{
  return remotetransport.RemoteTaskDependencies{
   TaskID:taskID,Available:false,Order:[]int{},Edges:[][2]int{},
  },nil
 }
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 rootInfo,err:=os.Stat(root)
 if err!=nil||!rootInfo.IsDir(){return empty,denied}
 vscodeDir:=filepath.Join(root,".vscode")
 dirInfo,err:=os.Lstat(vscodeDir)
 if err!=nil||!dirInfo.IsDir()||dirInfo.Mode()&os.ModeSymlink!=0{
  return empty,denied
 }
 filename:=filepath.Join(vscodeDir,"tasks.json")
 beforeInfo,err:=os.Lstat(filename)
 if err!=nil||!beforeInfo.Mode().IsRegular()||
  beforeInfo.Size()<2||beforeInfo.Size()>maxRemoteTasksPreviewSourceBytes{
  return empty,denied
 }
 before,err:=os.ReadFile(filename)
 if err!=nil||len(before)<2||len(before)>maxRemoteTasksPreviewSourceBytes||
  remoteTaskSourceDigest(root,before)!=session.taskPageHash{
  return empty,denied
 }
 list,err:=tasks.Load(root)
 if err!=nil||len(list)>remotetransport.MaxRemoteTasksCount||
  ctx.Err()!=nil{return empty,denied}
 counts:=map[int]int{}
 for _,t:=range list{counts[t.ID]++}
 var selected *tasks.Task
 for i:=range list{
  if list[i].ID==taskID{
   if selected!=nil{return empty,denied}
   selected=&list[i]
  }
 }
 if selected==nil||counts[taskID]!=1||
  safeRemoteTaskPreviewLabel(selected.MenuLabel)==""{
  return empty,denied
 }
 graph,err:=tasks.BuildWorkflowGraph(list,selected.Label)
 if err!=nil||len(graph.Nodes)==0||
  len(graph.Nodes)>remotetransport.MaxRemoteTaskDependencyNodes||
  len(graph.Edges)>remotetransport.MaxRemoteTaskDependencyEdges{
  return empty,denied
 }
 byLabel:=make(map[string]int,len(graph.Nodes))
 for _,node:=range graph.Nodes{
  if counts[node.ID]!=1||
   node.ID<1||node.ID>0x7fffffff{
   return empty,denied
  }
  byLabel[node.Label]=node.ID
 }
 result:=remotetransport.RemoteTaskDependencies{
  TaskID:taskID,Available:true,
  Order:make([]int,0,len(graph.Order)),
  Edges:make([][2]int,0,len(graph.Edges)),
 }
 for _,label:=range graph.Order{
  id,ok:=byLabel[label]
  if !ok{return empty,denied}
  result.Order=append(result.Order,id)
 }
 for _,edge:=range graph.Edges{
  dep,ok1:=byLabel[edge[0]]
  next,ok2:=byLabel[edge[1]]
  if !ok1||!ok2{return empty,denied}
  result.Edges=append(result.Edges,[2]int{dep,next})
 }
 after,err:=os.ReadFile(filename)
 if err!=nil||!bytes.Equal(before,after)||
  remoteTaskSourceDigest(root,after)!=session.taskPageHash||
  ctx.Err()!=nil||session.closed.Load(){return empty,denied}
 afterInfo,err:=os.Lstat(filename)
 if err!=nil||!afterInfo.Mode().IsRegular()||
  !os.SameFile(beforeInfo,afterInfo)||
  !session.portalGrant.ActiveForSession(session.channel)||
  !remotetransport.ValidRemoteTaskDependencies(result){
  return empty,denied
 }
 return result,nil
}
