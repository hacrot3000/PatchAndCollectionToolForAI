package server

import (
 "encoding/xml"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestProjectLauncherLinuxDesktopFile(t *testing.T){
 home:=t.TempDir()
 t.Setenv("XDG_DATA_HOME",filepath.Join(home,".local","share"))
 workspace:=filepath.Join(home,"Projects","ACME ' R&D 100% $ app")
 if err:=os.MkdirAll(workspace,0755);err!=nil{t.Fatal(err)}
 executable:=filepath.Join(home,"my tools","taskdeck")
 path,err:=installProjectLauncher(home,workspace,executable,"linux")
 if err!=nil{t.Fatal(err)}
 if !strings.HasPrefix(path,filepath.Join(home,".local","share","applications")){t.Fatalf("launcher escaped applications directory: %s",path)}
 contents,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
 text:=string(contents)
 for _,want:=range []string{
  "[Desktop Entry]","Type=Application","Terminal=false",
  "X-TaskDeck-Workspace-ID="+projectLauncherID(workspace),
  "--workspace", "--open-browser", "100%%", "\\$", "Icon=utilities-terminal",
 }{
  if !strings.Contains(text,want){t.Errorf("desktop file missing %q:\n%s",want,text)}
 }
 if !strings.Contains(text,launcherDesktopArg(workspace))||!strings.Contains(text,launcherDesktopArg(executable)){
  t.Fatalf("workspace and executable are not quoted safely:\n%s",text)
 }
 owned,err:=launcherOwned(path,workspace,"linux")
 if err!=nil||!owned{t.Fatalf("installed launcher ownership: %t, %v",owned,err)}
 // Reinstall refreshes the exact same shortcut without duplicates.
 again,err:=installProjectLauncher(home,workspace,executable,"linux")
 if err!=nil||again!=path{t.Fatalf("reinstall: path=%q err=%v",again,err)}
 info,err:=os.Stat(path)
 if err!=nil||info.Mode().Perm()!=0644{t.Fatalf("invalid desktop entry permissions: %v %v",info,err)}
 // Never overwrite someone else's launcher, even if its filename matches.
 if err:=os.WriteFile(path,[]byte("[Desktop Entry]\nName=Other App\n"),0600);err!=nil{t.Fatal(err)}
 if _,err=installProjectLauncher(home,workspace,executable,"linux");err==nil{t.Fatal("overwrote a launcher without TaskDeck marker")}
}

func TestProjectLauncherMacOSBundle(t *testing.T){
 home:=t.TempDir()
 workspace:=filepath.Join(home,"Project O'Brien & <friends>")
 if err:=os.MkdirAll(workspace,0755);err!=nil{t.Fatal(err)}
 executable:=filepath.Join(home,"TaskDeck binary")
 path,err:=installProjectLauncher(home,workspace,executable,"darwin")
 if err!=nil{t.Fatal(err)}
 if !strings.HasSuffix(path,".app"){t.Fatalf("expected .app bundle, got %s",path)}
 plist,err:=os.ReadFile(filepath.Join(path,"Contents","Info.plist"));if err!=nil{t.Fatal(err)}
 decoder:=xml.NewDecoder(strings.NewReader(string(plist)))
 for{
  if _,err:=decoder.Token();err!=nil {
   if err.Error()!="EOF"{t.Fatalf("plist XML parse: %v",err)}
   break
  }
 }
 if !strings.Contains(string(plist),"&amp;")||!strings.Contains(string(plist),"&lt;"){t.Fatal("app name is not XML-escaped")}
 if !strings.Contains(string(plist),"<key>CFBundleExecutable</key><string>launch</string>"){t.Fatal("bundle executable not registered")}
 script,err:=os.ReadFile(filepath.Join(path,"Contents","MacOS","launch"))
 if err!=nil{t.Fatal(err)}
 expected:="exec "+launcherShellArg(executable)+" --workspace "+launcherShellArg(workspace)+" --open-browser"
 if !strings.Contains(string(script),expected){t.Fatalf("macOS script did not quote args:\n%s",script)}
 info,err:=os.Stat(filepath.Join(path,"Contents","MacOS","launch"))
 if err!=nil||info.Mode().Perm()!=0755{t.Fatalf("script must be executable: %v %v",info,err)}
 owned,err:=launcherOwned(path,workspace,"darwin")
 if err!=nil||!owned{t.Fatalf("bundle owner mismatch %t %v",owned,err)}
 pathAgain,err:=installProjectLauncher(home,workspace,executable,"darwin")
 if err!=nil||pathAgain!=path{t.Fatalf("macOS bundle reinstallation %q %v",pathAgain,err)}
 if err:=os.Remove(filepath.Join(path,"Contents","Info.plist"));err!=nil{t.Fatal(err)}
 if _,err=installProjectLauncher(home,workspace,executable,"darwin");err==nil{t.Fatal("must not overwrite existing bundle without ownership marker")}
}

func TestProjectLauncherSameNameDifferentWorkspaces(t *testing.T){
 a:=filepath.Join(t.TempDir(),"client")
 b:=filepath.Join(t.TempDir(),"client")
 if projectLauncherID(a)==projectLauncherID(b){t.Fatal("colliding project launcher identity")}
 if launcherLocation("/home/example",a,"linux")==launcherLocation("/home/example",b,"linux"){t.Fatal("Linux launcher collision")}
 if launcherLocation("/Users/example",a,"darwin")==launcherLocation("/Users/example",b,"darwin"){t.Fatal("macOS launcher collision")}
}

func TestProjectLauncherRejectsSymlinksAndUnsupportedPlatforms(t *testing.T){
 home:=t.TempDir();workspace:=filepath.Join(home,"project")
 t.Setenv("XDG_DATA_HOME",filepath.Join(home,".local","share"))
 target:=launcherLocation(home,workspace,"linux")
 if err:=os.MkdirAll(filepath.Dir(target),0755);err!=nil{t.Fatal(err)}
 other:=filepath.Join(home,"victim")
 if err:=os.WriteFile(other,[]byte("private"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(other,target);err!=nil{t.Fatal(err)}
 if _,err:=installProjectLauncher(home,workspace,"/usr/local/bin/taskdeck","linux");err==nil{t.Fatal("must reject symlink desktop launcher")}
 if data,_:=os.ReadFile(other);string(data)!="private"{t.Fatal("modified symlink target")}
 if _,err:=installProjectLauncher(home,workspace,"/usr/local/bin/taskdeck","windows");err==nil{t.Fatal("Windows desktop launcher unexpectedly enabled")}
}

func TestProjectLauncherAPIRejectsRemoteAndSharedRequests(t *testing.T){
 workspace:=t.TempDir()
 s:=&Server{Workspace:workspace}
 for _,method:=range []string{http.MethodGet,http.MethodPost}{
  req:=httptest.NewRequest(method,"/api/system/project-launcher",nil)
  req.RemoteAddr="203.0.113.12:2345"
  rec:=httptest.NewRecorder()
  s.projectLauncherAPI(rec,req)
  if rec.Code!=http.StatusForbidden{t.Fatalf("remote %s permitted: %d",method,rec.Code)}
 }
 s.Config.SharedServerEnabled=true
 req:=httptest.NewRequest(http.MethodPost,"/api/system/project-launcher",nil)
 req.RemoteAddr="127.0.0.1:2345"
 rec:=httptest.NewRecorder()
 s.projectLauncherAPI(rec,req)
 if rec.Code!=http.StatusForbidden{t.Fatalf("shared-server shortcut mutation permitted: %d",rec.Code)}
}
