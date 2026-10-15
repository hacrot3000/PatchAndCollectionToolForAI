// Package workspaceinit discovers development toolchains without running
// any project code, then safely creates editable VS Code task templates.
package workspaceinit

import (
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "sort"
 "strings"
)

const (
 MaxDepth = 3
 MaxEntries = 4000
 MaxTaskCount = 36
)

type Detection struct {
 Kind string
 Folder string // Relative to project root, "." for workspace root
 Evidence string
}
type Proposal struct {
 Detections []Detection
 Tasks []map[string]any
}

var ignored = map[string]bool{
 ".git":true,".vscode":true,"node_modules":true,"vendor":true,
 "dist":true,"build":true,"target":true,".gradle":true,".idea":true,
 ".venv":true,"venv":true,"__pycache__":true,".next":true,
 ".cache":true,".tox":true,"coverage":true,"out":true,"bin":true,"obj":true,
}

func Detect(root string) (Proposal,error) {
 abs,err:=filepath.Abs(root);if err!=nil{return Proposal{},err}
 info,err:=os.Stat(abs);if err!=nil{return Proposal{},err}
 if !info.IsDir(){return Proposal{},errors.New("workspace is not a directory")}
 type item struct{ path string; depth int }
 queue:=[]item{{abs,0}}
 found:=map[string]Detection{}
 scanned:=0
 for len(queue)>0 && scanned<MaxEntries {
  current:=queue[0];queue=queue[1:]
  entries,err:=os.ReadDir(current.path)
  if err!=nil {if current.depth==0{return Proposal{},err};continue}
  for _,entry:=range entries {
   scanned++;if scanned>MaxEntries{break}
   name:=entry.Name()
   // Never descend through directory symlinks or hidden/generated dirs.
   if entry.IsDir(){
    if current.depth<MaxDepth&&!ignored[name]&&!strings.HasPrefix(name,"."){
     queue=append(queue,item{filepath.Join(current.path,name),current.depth+1})
    }
    continue
   }
   if !entry.Type().IsRegular(){continue}
   kind:=identifyFile(name)
   if kind==""{continue}
   rel,err:=filepath.Rel(abs,current.path);if err!=nil{continue}
   rel=filepath.ToSlash(rel)
   key:=kind+"@"+rel
   // Prefer toolchain marker/config files over incidental source files.
   candidate:=Detection{Kind:kind,Folder:rel,Evidence:name}
   previous,ok:=found[key]
   if !ok||markerWeight(candidate.Evidence)>markerWeight(previous.Evidence){
    found[key]=candidate
   }
  }
 }
 detections:=make([]Detection,0,len(found))
 for _,d:=range found{detections=append(detections,d)}
 sort.Slice(detections,func(i,j int)bool{
  a,b:=detections[i],detections[j]
  if a.Folder!=b.Folder {
   if a.Folder=="."{return true};if b.Folder=="."{return false}
   return a.Folder<b.Folder
  }
  return a.Kind<b.Kind
 })
 tasks:=make([]map[string]any,0)
 used:=map[string]bool{}
 for _,d:=range detections {
  if len(tasks)>=MaxTaskCount{break}
  templates:=templatesFor(abs,d)
  for _,task:=range templates {
   if len(tasks)>=MaxTaskCount{break}
   label,_:=task["label"].(string)
   if used[label]{continue}
   used[label]=true;tasks=append(tasks,task)
  }
 }
 if len(tasks)==0 {
  tasks=append(tasks,task("Workspace · Inspect directory","Workspace","pwd",nil,".","Show working directory"))
 }
 return Proposal{Detections:detections,Tasks:tasks},nil
}

func identifyFile(name string) string {
 switch name {
 case "go.mod":return "Go"
 case "Cargo.toml":return "Rust"
 case "package.json":return "Node.js"
 case "pyproject.toml","requirements.txt","Pipfile","manage.py","setup.py":return "Python"
 case "pom.xml":return "Java Maven"
 case "build.gradle","build.gradle.kts","settings.gradle","settings.gradle.kts":return "Java Gradle"
 case "composer.json":return "PHP Composer"
 case "pubspec.yaml":return "Dart Flutter"
 case "CMakeLists.txt":return "CMake C/C++"
 case "Makefile","makefile","GNUmakefile":return "Make"
 }
 if strings.HasSuffix(name,".nimble"){return "Nim"}
 // Source files alone are weak evidence but useful for new projects.
 ext:=strings.ToLower(filepath.Ext(name))
 switch ext {
 case ".go":return "Go"
 case ".rs":return "Rust"
 case ".py":return "Python"
 case ".nim":return "Nim"
 case ".php":return "PHP"
 case ".js",".mjs",".cjs",".ts",".tsx",".jsx":return "Node.js"
 case ".java":return "Java"
 case ".kt":return "Kotlin"
 case ".c",".cc",".cpp",".cxx":return "C/C++"
 }
 return ""
}
func markerWeight(name string) int {
 switch name{
 case "go.mod","Cargo.toml","package.json","pyproject.toml","requirements.txt",
 "Pipfile","pom.xml","build.gradle","build.gradle.kts","settings.gradle",
 "settings.gradle.kts","composer.json","pubspec.yaml","CMakeLists.txt",
 "Makefile","GNUmakefile","makefile":return 5
 }
 if strings.HasSuffix(name,".nimble"){return 5}
 return 1
}
func task(label,group,command string,args []string,cwd,detail string) map[string]any {
 result:=map[string]any{
  "label":label,"menuLabel":label,"menuGroup":group,
  "type":"process","command":command,"args":args,
  "detail":detail,
 }
 if cwd!="."{result["options"]=map[string]any{"cwd":filepath.ToSlash(cwd)}}
 return result
}
func templatesFor(root string,d Detection) []map[string]any {
 folder:=d.Folder
 label:=d.Kind
 if folder!="."{label+=" ("+folder+")"}
 mk:=func(name,command string,args ...string) map[string]any{
  return task(label+" · "+name,d.Kind,command,args,folder,"Suggested task: review before running")
 }
 switch d.Kind {
 case "Go":
  return []map[string]any{mk("Test","go","test","./..."),mk("Build","go","build","./..."),mk("Vet","go","vet","./...")}
 case "Rust":
  return []map[string]any{mk("Check","cargo","check"),mk("Test","cargo","test"),mk("Build","cargo","build")}
 case "Node.js":
  // Reading known script names avoids generating npm tasks that do not exist.
  scripts:=readPackageScripts(filepath.Join(root,filepath.FromSlash(folder),"package.json"))
  list:=[]map[string]any{}
  if exists(filepath.Join(root,filepath.FromSlash(folder),"package.json")) {
   list=append(list,mk("Install","npm","install"))
  }else{
   // Source-only JS/TS folder has no package manager manifest.
   // Do not suggest npm install where it cannot work.
   list=append(list,mk("Node version","node","--version"))
  }
  for _,name:=range []string{"dev","build","test","start","lint","typecheck"}{
   if scripts[name]{list=append(list,mk(strings.ToUpper(name[:1])+name[1:],"npm","run",name))}
  }
  return list
 case "Python":
  list:=[]map[string]any{mk("Compile check","python3","-m","compileall","-q",".")}
  if exists(filepath.Join(root,filepath.FromSlash(folder),"pytest.ini"))||exists(filepath.Join(root,filepath.FromSlash(folder),"pyproject.toml")){
   list=append(list,mk("Test (pytest)","python3","-m","pytest"))
  }
  if exists(filepath.Join(root,filepath.FromSlash(folder),"manage.py")){
   list=append(list,mk("Django check","python3","manage.py","check"))
  }
  return list
 case "Java Maven":
  return []map[string]any{mk("Test","mvn","test"),mk("Package","mvn","package")}
 case "Java Gradle":
  gradle:="gradle"
  if exists(filepath.Join(root,filepath.FromSlash(folder),"gradlew")){gradle="./gradlew"}
  return []map[string]any{mk("Tasks",gradle,"tasks"),mk("Test",gradle,"test"),mk("Build",gradle,"build")}
 case "PHP Composer":
  return []map[string]any{mk("Validate","composer","validate"),mk("Install","composer","install")}
 case "Dart Flutter":
  return []map[string]any{mk("Analyze","flutter","analyze"),mk("Test","flutter","test"),mk("Build APK","flutter","build","apk")}
 case "CMake C/C++":
  return []map[string]any{mk("Configure","cmake","-S",".","-B","build"),mk("Build","cmake","--build","build")}
 case "Make":
  return []map[string]any{mk("List dry-run","make","-n"),mk("Build","make")}
 case "Nim":
  if strings.HasSuffix(d.Evidence,".nimble"){
   return []map[string]any{mk("Available tasks","nimble","tasks")}
  }
  return []map[string]any{mk("Compile","nim","c",d.Evidence)}
 case "Java":
  return []map[string]any{mk("Java compiler version","javac","-version")}
 case "Kotlin":
  return []map[string]any{mk("Kotlin compiler version","kotlinc","-version")}
 case "C/C++":
  return []map[string]any{mk("Compiler version","cc","--version")}
 case "PHP":
  return []map[string]any{mk("PHP version","php","-v")}
 }
 return nil
}
func exists(path string) bool {info,err:=os.Stat(path);return err==nil&&info.Mode().IsRegular()}
func readPackageScripts(file string) map[string]bool {
 found:=map[string]bool{}
 f,err:=os.Open(file);if err!=nil{return found};defer f.Close()
 info,err:=f.Stat();if err!=nil||info.Size()>256*1024{return found}
 data,err:=io.ReadAll(io.LimitReader(f,256*1024+1));if err!=nil||len(data)>256*1024{return found}
 var packageJSON struct{Scripts map[string]json.RawMessage `json:"scripts"`}
 if json.Unmarshal(data,&packageJSON)!=nil{return found}
 for name:=range packageJSON.Scripts{found[name]=true}
 return found
}

// Create refuses to replace any existing tasks.json (including symlinks).
// No commands are ever executed while analyzing or creating templates.
func Create(root string,proposal Proposal) (string,error) {
 abs,err:=filepath.Abs(root);if err!=nil{return "",err}
 info,err:=os.Stat(abs);if err!=nil{return "",err}
 if !info.IsDir(){return "",errors.New("workspace is not a directory")}
 vscode:=filepath.Join(abs,".vscode")
 if err:=os.MkdirAll(vscode,0755);err!=nil{return "",err}
 dirInfo,err:=os.Lstat(vscode);if err!=nil{return "",err}
 if !dirInfo.IsDir()||dirInfo.Mode()&os.ModeSymlink!=0{return "",errors.New(".vscode must be an actual directory")}
 tasksPath:=filepath.Join(vscode,"tasks.json")
 doc:=map[string]any{"version":"2.0.0","tasks":proposal.Tasks}
 body,err:=json.MarshalIndent(doc,"","  ");if err!=nil{return "",err}
 body=append(body,byte(10))
 f,err:=os.OpenFile(tasksPath,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0644)
 if err!=nil{return "",fmt.Errorf("cannot create %s (existing files are never replaced): %w",tasksPath,err)}
 if _,err=f.Write(body);err!=nil{f.Close();os.Remove(tasksPath);return "",err}
 if err=f.Close();err!=nil{return "",err}
 return tasksPath,nil
}
