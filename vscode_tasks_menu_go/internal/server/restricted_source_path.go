package server

import (
 "errors"
 "path/filepath"
 "strings"
)

var errRestrictedSourcePath=errors.New("protected source path is not accessible")

func restrictedVisibleRelativePath(value string)bool{
 if value==""||value=="."||filepath.IsAbs(value){return false}
 for _,part:=range strings.Split(filepath.ToSlash(value),"/"){
  if part==""||part=="."||part==".."||strings.HasPrefix(part,"."){return false}
 }
 return true
}

// restrictedSourcePath checks BOTH the client-visible name and its final
// canonical location. Otherwise an innocent-looking symlink into .git,
// .vscode, .env or another hidden source would bypass the client-name filter.
func (s *Server) restrictedSourcePath(value string)(rel,canonical,root string,err error){
 rel,err=cleanProjectRelativePath(value,false)
 if err!=nil||!restrictedVisibleRelativePath(rel){
  return "","","",errRestrictedSourcePath
 }
 canonical,err=s.resolveProjectPath(rel,false,false)
 if err!=nil{return "","","",errRestrictedSourcePath}
 view,_,err:=s.projectRootForVirtualPath(rel)
 if err!=nil{return "","","",errRestrictedSourcePath}
 root,err=filepath.EvalSymlinks(view.Path)
 if err!=nil{return "","","",errRestrictedSourcePath}
 actual,err:=filepath.EvalSymlinks(canonical)
 if err!=nil||!pathWithin(root,actual){return "","","",errRestrictedSourcePath}
 pathFromRoot,err:=filepath.Rel(root,actual)
 if err!=nil||!restrictedVisibleRelativePath(pathFromRoot){
  return "","","",errRestrictedSourcePath
 }
 return rel,actual,root,nil
}
