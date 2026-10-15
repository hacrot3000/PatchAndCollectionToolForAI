package workspaceinit

import (
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func touch(t *testing.T,root,rel,body string){
 t.Helper()
 file:=filepath.Join(root,filepath.FromSlash(rel))
 if err:=os.MkdirAll(filepath.Dir(file),0755);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(file,[]byte(body),0644);err!=nil{t.Fatal(err)}
}
func hasTask(p Proposal,contains string) bool{
 for _,task:=range p.Tasks{if strings.Contains(task["label"].(string),contains){return true}}
 return false
}
func TestDetectLanguagesAndGenerateValidVSCodeTasks(t *testing.T){
 root:=t.TempDir()
 touch(t,root,"go.mod","module example.org/test\n")
 touch(t,root,"frontend/package.json",`{"scripts":{"dev":"vite","build":"vite build","test":"vitest"}}`)
 touch(t,root,"tools/pyproject.toml","[project]
name="demo"
")
 touch(t,root,"toolchain/Cargo.toml","[package]
name="demo"
version="0.1.0"
")
 proposal,err:=Detect(root);if err!=nil{t.Fatal(err)}
 for _,label:=range []string{"Go · Test","Node.js (frontend) · Dev","Node.js (frontend) · Build",
  "Python (tools) · Compile check","Rust (toolchain) · Check"}{
  if !hasTask(proposal,label){t.Fatalf("missing %q from %v",label,proposal.Tasks)}
 }
 path,err:=Create(root,proposal);if err!=nil{t.Fatal(err)}
 if path!=filepath.Join(root,".vscode","tasks.json"){t.Fatalf("created %s",path)}
 loaded,err:=tasks.Load(root);if err!=nil{t.Fatalf("generated tasks aren't TaskDeck compatible: %v",err)}
 if len(loaded)!=len(proposal.Tasks){t.Fatalf("TaskDeck loaded %d, expected %d",len(loaded),len(proposal.Tasks))}
 for _,item:=range loaded{
  resolved,err:=tasks.ResolveExecution(item,root)
  if err!=nil{t.Fatalf("generated task %q invalid: %v",item.Label,err)}
  if item.Type!="process"{t.Fatalf("template executes via shell: %s",item.Label)}
  if resolved.Cwd==""||resolved.Command==""{t.Fatalf("incomplete execution: %+v",resolved)}
 }
 if _,err:=Create(root,proposal);!os.IsExist(err)&&err==nil{t.Fatal("create overwrote existing file")}
}
func TestDetectBoundedDepthAndSkipGeneratedDirectories(t *testing.T){
 root:=t.TempDir()
 touch(t,root,"src/deeper/nested/go.mod","module inside")
 touch(t,root,"a/b/c/d/Cargo.toml","[package]
")
 touch(t,root,"node_modules/library/package.json",`{}`)
 touch(t,root,".git/hooks/setup.py","")
 p,err:=Detect(root);if err!=nil{t.Fatal(err)}
 if !hasTask(p,"Go"){t.Fatal("project nested within scan depth not found")}
 for _,d:=range p.Detections{
  if strings.Contains(d.Folder,"node_modules")||strings.Contains(d.Folder,".git")||d.Kind=="Rust"{
   t.Fatalf("unexpected generated/deep detection: %+v",d)
  }
 }
}
func TestEmptyProjectGetsEditableFallback(t *testing.T){
 root:=t.TempDir();p,err:=Detect(root);if err!=nil{t.Fatal(err)}
 if len(p.Detections)!=0||len(p.Tasks)!=1||!hasTask(p,"Workspace · Inspect directory"){t.Fatalf("empty proposal: %+v",p)}
 if _,err:=Create(root,p);err!=nil{t.Fatal(err)}
 if _,err:=tasks.Load(root);err!=nil{t.Fatal(err)}
}
func TestExistingTasksAndSymlinkAreNeverOverwritten(t *testing.T){
 root:=t.TempDir();p,_:=Detect(root)
 touch(t,root,".vscode/tasks.json","KEEP THIS FILE")
 if _,err:=Create(root,p);err==nil{t.Fatal("existing tasks overwritten")}
 existing,_:=os.ReadFile(filepath.Join(root,".vscode","tasks.json"))
 if string(existing)!="KEEP THIS FILE"{t.Fatalf("existing data changed: %s",existing)}
 outside:=t.TempDir()
 linkroot:=t.TempDir()
 if err:=os.Symlink(outside,filepath.Join(linkroot,".vscode"));err!=nil{t.Fatal(err)}
 if _,err:=Create(linkroot,p);err==nil{t.Fatal("symlink workspace config directory allowed")}
}
func TestSourceSymlinkIsNotCrawled(t *testing.T){
 root:=t.TempDir();outside:=t.TempDir()
 touch(t,outside,"go.mod","module private")
 if err:=os.Symlink(outside,filepath.Join(root,"external"));err!=nil{t.Fatal(err)}
 proposal,err:=Detect(root);if err!=nil{t.Fatal(err)}
 if len(proposal.Detections)!=0{t.Fatalf("followed external symlink: %+v",proposal.Detections)}
}
