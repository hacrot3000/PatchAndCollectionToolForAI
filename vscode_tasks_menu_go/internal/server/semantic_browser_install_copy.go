package server

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
)

func semanticWorkerBesideExecutable(worker string)bool{
 exe,err:=os.Executable()
 if err!=nil{return false}
 actual,err:=filepath.EvalSymlinks(exe)
 if err!=nil{return false}
 realWorker,err:=filepath.EvalSymlinks(worker)
 if err!=nil{return false}
 return filepath.Clean(realWorker)==filepath.Join(filepath.Dir(actual),"browser-worker")
}

// A global TaskDeck release may be read-only to the daemon user. Installing
// node_modules beside its binary would fail. Stage a private writable copy
// containing ONLY vetted worker runtime files, never follow symlink assets.
func semanticWritableWorker(source string)(string,error){
 if !semanticWorkerDirectoryValid(source){return "",errors.New("worker runtime is incomplete")}
 // Preserve immutable global releases even when their directory is
 // writable: package installation belongs in a private managed copy.
 // A user-owned development clone may reuse existing node_modules.
 if !semanticWorkerBesideExecutable(source){
  if file,err:=os.CreateTemp(source,".taskdeck-worker-writecheck-*");err==nil{
   name:=file.Name()
   _=file.Close()
   _=os.Remove(name)
   return source,nil
  }
 }
 config,err:=semanticSetupFile()
 if err!=nil{return "",err}
 root:=filepath.Join(filepath.Dir(config),"managed-worker")
 if err=os.MkdirAll(root,0700);err!=nil{return "",err}
 info,err:=os.Lstat(root)
 if err!=nil||!info.IsDir()||info.Mode().Perm()&0077!=0{
  return "",errors.New("unsafe managed worker directory")
 }
 dest:=filepath.Join(root,"browser-worker")
 if info,err:=os.Lstat(dest);err==nil{
  if !info.IsDir()||info.Mode().Perm()&0077!=0{
   return "",errors.New("unsafe previous managed worker directory")
  }
  // New TaskDeck versions can add a worker JS module (e.g. Chrome identity).
  // Refresh ONLY the vetted runtime files while preserving node_modules and
  // the already-downloaded Chromium cache. Do not follow existing symlinks.
  if filepath.Clean(source)!=filepath.Clean(dest){
   for _,filename:=range []string{
    "package.json","worker.mjs","start-configured.mjs",
    "semantic_extract.mjs","universal_input.mjs","popup_runtime.mjs",
    "auth_runtime.mjs","chrome_identity.mjs",
   }{
    current:=filepath.Join(dest,filename)
    if existing,err:=os.Lstat(current);err==nil{
     if !existing.Mode().IsRegular(){return "",fmt.Errorf("unsafe managed worker file: %s",filename)}
    }else if !os.IsNotExist(err){return "",err}
    raw,err:=os.ReadFile(filepath.Join(source,filename))
    if err!=nil{return "",err}
    temp,err:=os.CreateTemp(dest,".runtime-update-*")
    if err!=nil{return "",err}
    if _,err=temp.Write(raw);err!=nil{_ =temp.Close();_ =os.Remove(temp.Name());return "",err}
    if err=temp.Close();err!=nil{_ =os.Remove(temp.Name());return "",err}
    if err=os.Rename(temp.Name(),current);err!=nil{_ =os.Remove(temp.Name());return "",err}
   }
  }
  if !semanticWorkerDirectoryValid(dest){return "",errors.New("managed worker runtime update incomplete")}
  return dest,nil
 }else if !os.IsNotExist(err){return "",err}
 staging,err:=os.MkdirTemp(root,".worker-stage-*")
 if err!=nil{return "",err}
 defer os.RemoveAll(staging)
 candidate:=filepath.Join(staging,"browser-worker")
 if err:=os.Mkdir(candidate,0700);err!=nil{return "",err}
 for _,filename:=range []string{
  "package.json","worker.mjs","start-configured.mjs",
  "semantic_extract.mjs","universal_input.mjs","popup_runtime.mjs","auth_runtime.mjs","chrome_identity.mjs",
 }{
  path:=filepath.Join(source,filename)
  stat,err:=os.Lstat(path)
  if err!=nil||!stat.Mode().IsRegular()||stat.Size()>2<<20{
   return "",fmt.Errorf("unsafe worker file: %s",filename)
  }
  data,err:=os.ReadFile(path)
  if err!=nil{return "",err}
  if err:=os.WriteFile(filepath.Join(candidate,filename),data,0600);err!=nil{return "",err}
 }
 if !semanticWorkerDirectoryValid(candidate){return "",errors.New("staged worker validation failed")}
 if err:=os.Rename(candidate,dest);err!=nil{return "",err}
 return dest,nil
}
