package server

import (
 "bytes"
 "encoding/binary"
 "encoding/xml"
 "image/png"
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
  "--workspace", "--open-browser", "100%%", "\\$", "Icon="+launcherIconLocation(home,workspace),
 }{
  if !strings.Contains(text,want){t.Errorf("desktop file missing %q:\n%s",want,text)}
 }
 if !strings.Contains(text,launcherDesktopArg(workspace))||!strings.Contains(text,launcherDesktopArg(executable)){
  t.Fatalf("workspace and executable are not quoted safely:\n%s",text)
 }
 iconPath:=launcherIconLocation(home,workspace)
 iconData,err:=os.ReadFile(iconPath);if err!=nil{t.Fatal(err)}
 if !bytes.Equal(iconData,taskdeckLauncherPNG){t.Fatal("Linux launcher image differs from bundled Glossy Terminal Task Stack")}
 if _,err=png.DecodeConfig(bytes.NewReader(iconData));err!=nil{t.Fatalf("invalid launcher PNG: %v",err)}
 owned,err:=launcherOwned(path,workspace,"linux")
 if err!=nil||!owned{t.Fatalf("installed launcher ownership: %t, %v",owned,err)}
 // Reinstall refreshes the same files (including a stale PNG) without duplication.
 if err:=os.WriteFile(iconPath,[]byte("stale icon"),0600);err!=nil{t.Fatal(err)}
 updatedExecutable:=filepath.Join(home,"new install","taskdeck")
 again,err:=installProjectLauncher(home,workspace,updatedExecutable,"linux")
 if err!=nil||again!=path{t.Fatalf("reinstall: path=%q err=%v",again,err)}
 refreshed,err:=os.ReadFile(iconPath);if err!=nil{t.Fatal(err)}
 if !bytes.Equal(refreshed,taskdeckLauncherPNG){t.Fatal("clicking Create again did not refresh existing PNG")}
 updatedEntry,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(updatedEntry),launcherDesktopArg(updatedExecutable)){t.Fatal("existing launcher executable was not updated")}
 matches,err:=filepath.Glob(filepath.Join(filepath.Dir(path),"taskdeck-project-*.desktop"))
 if err!=nil||len(matches)!=1{t.Fatalf("reinstall duplicated launcher: %v, %v",matches,err)}
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
 if !strings.Contains(string(plist),"<key>CFBundleIconFile</key><string>taskdeck.icns</string>"){t.Fatal("macOS bundle does not reference TaskDeck custom icon")}
 iconPath:=filepath.Join(path,"Contents","Resources","taskdeck.icns")
 iconData,err:=os.ReadFile(iconPath);if err!=nil{t.Fatal(err)}
 assertTaskDeckICNS(t,iconData)
 script,err:=os.ReadFile(filepath.Join(path,"Contents","MacOS","launch"))
 if err!=nil{t.Fatal(err)}
 expected:="exec "+launcherShellArg(executable)+" --workspace "+launcherShellArg(workspace)+" --open-browser"
 if !strings.Contains(string(script),expected){t.Fatalf("macOS script did not quote args:\n%s",script)}
 info,err:=os.Stat(filepath.Join(path,"Contents","MacOS","launch"))
 if err!=nil||info.Mode().Perm()!=0755{t.Fatalf("script must be executable: %v %v",info,err)}
 owned,err:=launcherOwned(path,workspace,"darwin")
 if err!=nil||!owned{t.Fatalf("bundle owner mismatch %t %v",owned,err)}
 if err:=os.WriteFile(iconPath,[]byte("old icon"),0600);err!=nil{t.Fatal(err)}
 newExecutable:=filepath.Join(home,"new taskdeck binary")
 pathAgain,err:=installProjectLauncher(home,workspace,newExecutable,"darwin")
 if err!=nil||pathAgain!=path{t.Fatalf("macOS bundle reinstallation %q %v",pathAgain,err)}
 restored,err:=os.ReadFile(iconPath);if err!=nil{t.Fatal(err)}
 assertTaskDeckICNS(t,restored)
 refreshedScript,err:=os.ReadFile(filepath.Join(path,"Contents","MacOS","launch"))
 if err!=nil||!strings.Contains(string(refreshedScript),launcherShellArg(newExecutable)){t.Fatalf("macOS bundle failed to update executable: %v",err)}
 matches,err:=filepath.Glob(filepath.Join(filepath.Dir(path),"*.app"))
 if err!=nil||len(matches)!=1{t.Fatalf("macOS reinstall duplicated application bundle: %v, %v",matches,err)}
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


func assertTaskDeckICNS(t *testing.T,data []byte){
 t.Helper()
 if len(data)<8||string(data[:4])!="icns"||int(binary.BigEndian.Uint32(data[4:8]))!=len(data){
  t.Fatal("invalid ICNS magic or length")
 }
 sizes:=map[string]int{"icp4":16,"icp5":32,"icp6":64,"ic07":128,"ic08":256,"ic09":512,"ic10":1024}
 offset:=8
 for offset<len(data){
  if len(data)-offset<8{t.Fatal("truncated ICNS chunk")}
  kind:=string(data[offset:offset+4])
  chunkSize:=int(binary.BigEndian.Uint32(data[offset+4:offset+8]))
  if chunkSize<8||chunkSize>len(data)-offset{t.Fatal("invalid ICNS chunk length")}
  dimension,valid:=sizes[kind];if !valid{t.Fatalf("unexpected ICNS entry %s",kind)}
  cfg,err:=png.DecodeConfig(bytes.NewReader(data[offset+8:offset+chunkSize]))
  if err!=nil||cfg.Width!=dimension||cfg.Height!=dimension{
   t.Fatalf("ICNS entry %s: config=%+v err=%v",kind,cfg,err)
  }
  delete(sizes,kind)
  offset+=chunkSize
 }
 if len(sizes)!=0{t.Fatalf("missing ICNS pixel sizes: %v",sizes)}
}

func TestProjectLauncherTwoProjectsInstallAndRefreshIndependently(t *testing.T){
 home:=t.TempDir()
 t.Setenv("XDG_DATA_HOME",filepath.Join(home,"data"))
 a:=filepath.Join(home,"A","project")
 b:=filepath.Join(home,"B","project")
 for _,p:=range []string{a,b}{if err:=os.MkdirAll(p,0755);err!=nil{t.Fatal(err)}}
 for _,platform:=range []string{"linux","darwin"}{
  first,err:=installProjectLauncher(home,a,"/home/user/taskdeck",platform)
  if err!=nil{t.Fatal(err)}
  second,err:=installProjectLauncher(home,b,"/home/user/taskdeck",platform)
  if err!=nil{t.Fatal(err)}
  if first==second{t.Fatalf("%s: unrelated projects shared one launcher",platform)}
  renewed,err:=installProjectLauncher(home,a,"/home/user/new-taskdeck",platform)
  if err!=nil||renewed!=first{t.Fatalf("%s: reinstall wrote another launcher: %s %v",platform,renewed,err)}
  for _,item:=range []struct{workspace,path string}{{a,first},{b,second}}{
   owned,err:=launcherOwned(item.path,item.workspace,platform)
   if err!=nil||!owned{t.Fatalf("%s: project %s overwritten: %v",platform,item.workspace,err)}
  }
  if platform=="linux"{
   one,err:=os.ReadFile(launcherIconLocation(home,a));if err!=nil{t.Fatal(err)}
   two,err:=os.ReadFile(launcherIconLocation(home,b));if err!=nil{t.Fatal(err)}
   if !bytes.Equal(one,two)||!bytes.Equal(one,taskdeckLauncherPNG){t.Fatal("each Linux project must have exact bundled icon")}
   if strings.Contains(string(mustReadLauncher(t,second)),"new-taskdeck"){t.Fatal("updating project A modified project B")}
  }else{
   assertTaskDeckICNS(t,mustReadLauncher(t,filepath.Join(first,"Contents","Resources","taskdeck.icns")))
   assertTaskDeckICNS(t,mustReadLauncher(t,filepath.Join(second,"Contents","Resources","taskdeck.icns")))
   if strings.Contains(string(mustReadLauncher(t,filepath.Join(second,"Contents","MacOS","launch"))),"new-taskdeck"){t.Fatal("updating project A modified project B app")}
  }
 }
}

func mustReadLauncher(t *testing.T,path string)[]byte{
 t.Helper()
 data,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
 return data
}
