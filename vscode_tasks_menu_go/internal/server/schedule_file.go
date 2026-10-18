package server

import(
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
)

func (s *Server) validateScheduleFile(path,kind string)error{
 if kind!="change"&&kind!="exists"&&kind!="missing"{return errors.New("file_condition must be change, exists or missing")}
 if path==""||filepath.IsAbs(path){return errors.New("file_path must be a relative workspace path")}
 if strings.ContainsRune(path,'\x00'){return errors.New("file_path contains an invalid byte")}
 workspace,err:=canonicalExistingPath(s.Workspace);if err!=nil{return err}
 target:=filepath.Clean(filepath.Join(workspace,path))
 if target==workspace||!pathInside(workspace,target){return errors.New("file_path must be inside workspace")}
 // Find the nearest existing ancestor to reject traversal through symlinks.
 nearest:=target
 for{
  _,err=os.Lstat(nearest)
  if err==nil{break}
  if !os.IsNotExist(err){return err}
  parent:=filepath.Dir(nearest)
  if parent==nearest{return errors.New("file_path is invalid")}
  nearest=parent
 }
 real,err:=canonicalExistingPath(nearest);if err!=nil{return err}
 if !pathInside(workspace,real){return errors.New("file path resolves outside workspace")}
 info,err:=os.Lstat(target)
 if err==nil{
  if info.Mode()&os.ModeSymlink!=0{return errors.New("symbolic link watched paths are not allowed")}
 }else if !os.IsNotExist(err){return err
 }else if kind=="change"{return errors.New("file must exist to watch changes")}
 return nil
}
func (s *Server) scheduleFileObservation(_ context.Context,c scheduleDefinition)(string,bool,error){
 if err:=s.validateScheduleFile(c.FilePath,c.FileCondition);err!=nil{return "",false,err}
 path:=filepath.Join(s.Workspace,c.FilePath)
 info,err:=os.Lstat(path)
 if os.IsNotExist(err){
  if c.FileCondition=="missing"{return "missing",true,nil}
  return "missing",false,nil
 }
 if err!=nil{return "",false,err}
 if info.Mode()&os.ModeSymlink!=0{return "",false,errors.New("symlink target is not allowed")}
 fingerprint:=fmt.Sprintf("%s:%d:%d:%d",path,info.Size(),info.ModTime().UnixNano(),info.Mode())
 if info.IsDir(){
  children,readErr:=os.ReadDir(path);if readErr!=nil{return "",false,readErr}
  if len(children)>512{return "",false,errors.New("watched directory has more than 512 entries")}
  for _,entry:=range children{
   child,statErr:=entry.Info()
   if statErr!=nil{return "",false,statErr}
   fingerprint+=fmt.Sprintf("|%s:%d:%d:%s",entry.Name(),child.Size(),child.ModTime().UnixNano(),child.Mode())
  }
 }
 hash:=sha256.Sum256([]byte(fingerprint))
 observation:=hex.EncodeToString(hash[:16])
 if c.FileCondition=="exists"{return "exists",true,nil}
 if c.FileCondition=="missing"{return "exists",false,nil}
 return observation,true,nil
}
