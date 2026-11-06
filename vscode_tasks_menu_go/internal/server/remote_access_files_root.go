package server

import(
 "context"
 "io"
 "os"
 "path/filepath"
 "runtime"
 "sort"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Only an opt-in, root-directory listing. Neither paths nor contents reach
// PHP/Relay; no symlink, recursive walk, or arbitrary file operation exists.
func(s *Server)readRemoteFilesRoot(ctx context.Context)(remotetransport.RemoteFilesRoot,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteFilesRoot
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""||
  runtime.GOOS=="windows"||os.Getenv("TASKDECK_REMOTE_FILES_ROOT")!="1"{return empty,denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return empty,denied}
 root,err=filepath.Abs(root)
 if err!=nil{return empty,denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return empty,denied}
 handle,err:=os.Open(root)
 if err!=nil{return empty,denied}
 defer handle.Close()
 entries,err:=handle.ReadDir(129)
 if err!=nil&&err!=io.EOF{return empty,denied}
 if len(entries)>=129{return empty,denied}
 names:=make([]string,0,len(entries))
 for _,entry:=range entries{
  if ctx.Err()!=nil{return empty,denied}
  name:=entry.Name()
  if !remotetransport.ValidRemoteFilesRootItem("F:"+name){continue}
  fi,e:=os.Lstat(filepath.Join(root,name))
  if e!=nil{return empty,denied}
  kind:=""
  if fi.Mode().IsRegular(){kind="F:"}
  if fi.IsDir(){kind="D:"}
  if kind==""{continue}
  names=append(names,kind+name)
 }
 sort.Slice(names,func(i,j int)bool{return names[i][2:]<names[j][2:]})
 more:=len(names)>remotetransport.MaxRemoteFilesRootItems
 if more{names=names[:remotetransport.MaxRemoteFilesRootItems]}
 result:=remotetransport.RemoteFilesRoot{Items:names,More:more}
 if !remotetransport.ValidRemoteFilesRoot(result){return empty,denied}
 return result,nil
}
