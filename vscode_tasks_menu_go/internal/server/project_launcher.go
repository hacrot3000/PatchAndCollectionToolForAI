package server

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "encoding/xml"
 "errors"
 "fmt"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
)

// A launcher always belongs to the native OS account that runs TaskDeck.
// Never present a Linux-container shortcut as if it were installed on a host.
type projectLauncherInfo struct {
 Supported bool `json:"supported"`
 Installed bool `json:"installed"`
 Platform string `json:"platform"`
 Path string `json:"path,omitempty"`
 Workspace string `json:"workspace,omitempty"`
 Reason string `json:"reason,omitempty"`
}

func projectLauncherID(workspace string) string {
 hash:=sha256.Sum256([]byte(filepath.Clean(workspace)))
 return hex.EncodeToString(hash[:8])
}

func launcherProjectName(workspace string) string {
 name:=filepath.Base(filepath.Clean(workspace))
 if name=="."||name==string(filepath.Separator)||name==""{name="Project"}
 name=strings.Map(func(r rune)rune{
  if r<32||r==127||r=='/'||r=='\\'||r==':'{return '-'}
  return r
 },name)
 name=strings.TrimSpace(name)
 if len([]rune(name))>48{name=string([]rune(name)[:48])}
 if name==""{return "Project"}
 return name
}

func launcherWorkspace(workspace string)(string,error){
 absolute,err:=filepath.Abs(workspace);if err!=nil{return "",err}
 actual,err:=filepath.EvalSymlinks(absolute);if err!=nil{return "",err}
 stat,err:=os.Stat(actual)
 if err!=nil{return "",err}
 if !stat.IsDir(){return "",errors.New("workspace is not a directory")}
 return actual,nil
}

func launcherLocation(home,workspace,platform string) string {
 id,name:=projectLauncherID(workspace),launcherProjectName(workspace)
 switch platform {
 case "linux":
  base:=os.Getenv("XDG_DATA_HOME")
  if base==""||!filepath.IsAbs(base){base=filepath.Join(home,".local","share")}
  return filepath.Join(base,"applications","taskdeck-project-"+id+".desktop")
 case "darwin":
  return filepath.Join(home,"Applications","TaskDeck Projects","TaskDeck - "+name+" - "+id+".app")
 }
 return ""
}

// Prefer a stable executable entry point, not a release-specific binary
// that might disappear after the next TaskDeck self-update.
func launcherExecutable(home string)(string,error){
 active,err:=os.Executable();if err!=nil{return "",err}
 activeStat,err:=os.Stat(active);if err!=nil{return "",err}
 candidates:=[]string{filepath.Join(home,".local","bin","taskdeck")}
 if lookup,err:=exec.LookPath("taskdeck");err==nil{candidates=append(candidates,lookup)}
 for _,candidate:=range candidates{
  abs,err:=filepath.Abs(candidate);if err!=nil{continue}
  stat,err:=os.Stat(abs)
  if err==nil&&stat.Mode().IsRegular()&&stat.Mode().Perm()&0111!=0&&os.SameFile(activeStat,stat){return abs,nil}
 }
 if !activeStat.Mode().IsRegular()||activeStat.Mode().Perm()&0111==0{return "",errors.New("TaskDeck executable is not executable")}
 return active,nil
}

func launcherTextValue(value string) string {
 return strings.Map(func(r rune)rune{
  if r=='\n'||r=='\r'||r==0{return ' '}
  return r
 },value)
}

// Desktop Entry Specification: quote the entire argument and escape its
// reserved characters, including % field codes. No shell is involved.
func launcherDesktopArg(value string) string {
 value=strings.NewReplacer("\\","\\\\","\"","\\\"","$","\\$","`","\\`","%","%%").Replace(value)
 return "\"" + value + "\""
}

func launcherShellArg(value string) string {
 return "'" + strings.ReplaceAll(value,"'","'\\''") + "'"
}

func launcherDesktopContents(workspace,executable string) string {
 return "[Desktop Entry]\n"+
  "Type=Application\nVersion=1.0\n"+
  "Name=TaskDeck - "+launcherTextValue(launcherProjectName(workspace))+"\n"+
  "Comment=Open TaskDeck project\n"+
  "Exec="+launcherDesktopArg(executable)+" --workspace "+launcherDesktopArg(workspace)+"\n"+
  "Path="+launcherTextValue(workspace)+"\n"+
  "Icon=utilities-terminal\nCategories=Development;IDE;\nTerminal=false\nStartupNotify=true\n"+
  "X-TaskDeck-Workspace-ID="+projectLauncherID(workspace)+"\n"
}

func launcherXML(value string)string{
 var out strings.Builder
 _=xml.EscapeText(&out,[]byte(value))
 return out.String()
}
func launcherMacPlist(workspace string)string{
 id:=projectLauncherID(workspace)
 name:=launcherXML("TaskDeck - "+launcherProjectName(workspace))
 return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"+
 "<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n"+
 "<plist version=\"1.0\"><dict>\n"+
 "<key>CFBundleName</key><string>"+name+"</string>\n"+
 "<key>CFBundleDisplayName</key><string>"+name+"</string>\n"+
 "<key>CFBundleIdentifier</key><string>dev.taskdeck.project."+id+"</string>\n"+
 "<key>CFBundleExecutable</key><string>launch</string>\n"+
 "<key>CFBundlePackageType</key><string>APPL</string>\n"+
 "<key>CFBundleVersion</key><string>1</string>\n"+
 "<key>CFBundleShortVersionString</key><string>1.0</string>\n"+
 "<key>TaskDeckWorkspaceID</key><string>"+id+"</string>\n"+
 "</dict></plist>\n"
}
func launcherMacScript(workspace,executable string)string{
 return "#!/bin/sh\n# Generated by TaskDeck for this project.\nexec "+launcherShellArg(executable)+" --workspace "+launcherShellArg(workspace)+"\n"
}

func launcherOwned(path,workspace,platform string)(bool,error){
 marker:=""
 if platform=="linux" {
  marker="X-TaskDeck-Workspace-ID="+projectLauncherID(workspace)
 }else{
  path=filepath.Join(path,"Contents","Info.plist")
  marker="<key>TaskDeckWorkspaceID</key><string>"+projectLauncherID(workspace)+"</string>"
 }
 data,err:=os.ReadFile(path)
 if os.IsNotExist(err){return false,nil}
 if err!=nil{return false,err}
 return strings.Contains(string(data),marker),nil
}

func launcherAtomicWrite(path string,data []byte,mode os.FileMode)error{
 dir:=filepath.Dir(path)
 if err:=os.MkdirAll(dir,0755);err!=nil{return err}
 // Do not follow an existing symlink at the final path.
 if info,err:=os.Lstat(path);err==nil {
  if !info.Mode().IsRegular(){return errors.New("launcher file is not a regular file")}
 }else if !os.IsNotExist(err){return err}
 temp,err:=os.CreateTemp(dir,".taskdeck-shortcut-*")
 if err!=nil{return err}
 defer os.Remove(temp.Name())
 if err=temp.Chmod(mode);err!=nil{temp.Close();return err}
 if _,err=temp.Write(data);err!=nil{temp.Close();return err}
 if err=temp.Sync();err!=nil{temp.Close();return err}
 if err=temp.Close();err!=nil{return err}
 return os.Rename(temp.Name(),path)
}

func launcherEnsureDirectory(path string)error{
 if info,err:=os.Lstat(path);err==nil {
  if !info.IsDir(){return errors.New("launcher bundle path is not a directory")}
  return nil
 }else if !os.IsNotExist(err){return err}
 return os.Mkdir(path,0755)
}

func installProjectLauncher(home,workspace,executable,platform string)(string,error){
 if platform!="linux"&&platform!="darwin"{return "",errors.New("launchers require native Linux or macOS")}
 if strings.ContainsAny(workspace,"\n\r\x00")||strings.ContainsAny(executable,"\n\r\x00") {
  return "",errors.New("launcher arguments contain control characters")
 }
 path:=launcherLocation(home,workspace,platform)
 if info,err:=os.Lstat(path);err==nil {
  if platform=="linux"&&!info.Mode().IsRegular(){return "",errors.New("existing launcher is not a regular file")}
  if platform=="darwin"&&!info.IsDir(){return "",errors.New("existing launcher bundle is not a directory")}
  owned,err:=launcherOwned(path,workspace,platform)
  if err!=nil{return "",err}
  if !owned{return "",errors.New("existing launcher belongs to another application; refusing to overwrite")}
 }else if !os.IsNotExist(err){return "",err}
 if platform=="linux" {
  return path,launcherAtomicWrite(path,[]byte(launcherDesktopContents(workspace,executable)),0644)
 }
 if err:=os.MkdirAll(filepath.Dir(path),0755);err!=nil{return "",err}
 if err:=launcherEnsureDirectory(path);err!=nil{return "",err}
 contents:=filepath.Join(path,"Contents")
 if err:=launcherEnsureDirectory(contents);err!=nil{return "",err}
 macos:=filepath.Join(contents,"MacOS")
 if err:=launcherEnsureDirectory(macos);err!=nil{return "",err}
 if err:=launcherAtomicWrite(filepath.Join(contents,"Info.plist"),[]byte(launcherMacPlist(workspace)),0644);err!=nil{return "",err}
 if err:=launcherAtomicWrite(filepath.Join(macos,"launch"),[]byte(launcherMacScript(workspace,executable)),0755);err!=nil{return "",err}
 return path,nil
}

func projectLauncherNativeAvailable()string{
 if runtime.GOOS!="linux"&&runtime.GOOS!="darwin"{return "Only native Linux/macOS hosts are supported"}
 if os.Geteuid()==0{return "Run TaskDeck as the desktop user, not root"}
 if os.Getenv("container")!=""||os.Getenv("KUBERNETES_SERVICE_HOST")!=""{return "Cannot create a host launcher from a container"}
 if runtime.GOOS=="linux"{
  if _,err:=os.Stat("/.dockerenv");err==nil{return "Cannot create a host launcher from Docker"}
 }
 return ""
}

func (s *Server) projectLauncherAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet&&r.Method!=http.MethodPost{
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
 }
 // Desktop installation modifies the TaskDeck daemon's OS user account.
 // Shared-server browser accounts must not mutate another user's launcher.
 if s.Config.SharedServerEnabled{
  http.Error(w,"Host launcher installation is disabled in shared-server mode",http.StatusForbidden);return
 }
 if !loopbackRemote(r.RemoteAddr){
  http.Error(w,"Host launcher installation is available only via local TaskDeck",http.StatusForbidden);return
 }
 reason:=projectLauncherNativeAvailable()
 if reason!=""{
  if r.Method==http.MethodPost{http.Error(w,reason,http.StatusNotImplemented);return}
  writeJSON(w,http.StatusOK,projectLauncherInfo{Platform:runtime.GOOS,Reason:reason});return
 }
 home,err:=os.UserHomeDir()
 if err!=nil{http.Error(w,"cannot locate desktop user's home: "+err.Error(),500);return}
 workspace,err:=launcherWorkspace(s.Workspace)
 if err!=nil{http.Error(w,"invalid workspace: "+err.Error(),500);return}
 path:=launcherLocation(home,workspace,runtime.GOOS)
 installed,err:=launcherOwned(path,workspace,runtime.GOOS)
 if err!=nil{http.Error(w,"cannot inspect launcher: "+err.Error(),500);return}
 if r.Method==http.MethodPost{
  executable,err:=launcherExecutable(home)
  if err!=nil{http.Error(w,"cannot locate TaskDeck executable: "+err.Error(),500);return}
  path,err=installProjectLauncher(home,workspace,executable,runtime.GOOS)
  if err!=nil{http.Error(w,"cannot install project launcher: "+err.Error(),409);return}
  installed=true
 }
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,http.StatusOK,projectLauncherInfo{Supported:true,Installed:installed,Platform:runtime.GOOS,Path:path,Workspace:workspace})
}

// Only status and create are supported. No user-supplied paths or commands are
// accepted by this API.
var _ = json.Valid
var _ = fmt.Sprintf
