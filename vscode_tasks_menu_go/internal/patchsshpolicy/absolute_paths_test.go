package patchsshpolicy

import (
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
)

func TestAbsoluteDataPathsAdmitReadOnlyOperands(t *testing.T) {
 for _,root:=range absoluteDataRoots {
  for _,path:=range []string{root,root+"/",root+"/logs/app.log",root+"/log files"} {
   if !AbsoluteDataPath(path) { t.Fatalf("allowed path rejected: %q",path) }
  }
 }
 cases:=[][]string{
  {"ls","-lah","/data"}, {"tree","-L","2","/srv"}, {"tree","/data"},
  {"find","/data","-maxdepth","3","-type","f","-iname","*.log"},
  {"grep","-nriF","--","orderId=123","/data/logs"}, {"rg","-n","-g","*.log","--","error","/var/log"},
  {"cat","/data/log files/app.log","/srv/logs/second.log"}, {"head","-n","30","/data/app.log"},
  {"tail","-n","50","/var/log/syslog"}, {"wc","-l","/data/a","/data/b"},
  {"nl","-ba","/opt/app.log"}, {"sed","-n","10,20p","/data/app.log"},
 }
 for _,argv:=range cases {
  cmd,err:=Validate(argv)
  if err!=nil||!strings.Contains(cmd,"realpath -e")||!strings.Contains(cmd,"|| exit 1") {t.Fatalf("%v: %q %v",argv,cmd,err)}
 }
}

func TestAbsoluteDataPathsRejectEscapeAndUnsafeCommands(t *testing.T) {
 for _,path:=range []string{"/","/etc/passwd","/root/.ssh","/home/user","/var/lib","/var/logs","/usr/bin","/proc/1/environ","/sys","/dev/zero","/run","/tmp","/database","/data//logs","/data/../etc","/data/./../etc","/data/$(id)","/data/file;id","/data/-option"} {
  if AbsoluteDataPath(path){t.Fatalf("unsafe path admitted: %s",path)}
  if _,err:=Validate([]string{"cat",path});err==nil {t.Fatalf("unsafe read admitted: %s",path)}
 }
 for _,argv:=range [][]string{
  {"cat","/data/a","relative.txt"},{"grep","-R","x","/data"},{"rg","--follow","x","/data"},
  {"find","/data","-maxdepth","2","-exec","id"},{"find","/data","-maxdepth","2","-delete"},
  {"sed","-i","1p","/data/a"},{"tail","-f","/data/a"},{"tree","-l","/data"},
  {"sh","-c","cat /data/a"},{"ls","/data",";","id"},
 } { if cmd,err:=Validate(argv);err==nil{t.Fatalf("unsafe argv %v -> %s",argv,cmd)} }
}

func TestAbsoluteDataPathPatternIsNotAnOperand(t *testing.T) {
 cmd,err:=Validate([]string{"grep","-F","--","/data/log","/data/log"})
 if err!=nil {t.Fatal(err)}
 if !strings.Contains(cmd,`exec grep --devices=skip -F -- /data/log "$td_path_4"`) {t.Fatalf("pattern incorrectly rewritten: %s",cmd)}
 got,err:=Validate([]string{"grep","-F","--","/etc/passwd","relative.txt"})
 if err!=nil||got!="grep -F -- /etc/passwd relative.txt" {t.Fatalf("literal search pattern rejected: %q %v",got,err)}
}

func TestAbsoluteDataRemoteGuardRealpathAndSymlink(t *testing.T) {
 if runtime.GOOS!="linux" {t.Skip("Linux remote shell integration fixture")}
 for _,tool:=range []string{"sh","realpath","grep","find","cat"} {if _,err:=exec.LookPath(tool);err!=nil{t.Skipf("%s unavailable",tool)}}
 base:=t.TempDir()
 root:=filepath.Join(base,"allowed")
 outside:=filepath.Join(base,"outside")
 for _,dir:=range []string{root,outside} {if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}}
 old:=absoluteDataRoots[0]
 absoluteDataRoots[0]=root
 t.Cleanup(func(){absoluteDataRoots[0]=old})
 good:=filepath.Join(root,"a.log")
 secret:=filepath.Join(outside,"b.log")
 if err:=os.WriteFile(good,[]byte("MATCH good\n"),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(secret,[]byte("MATCH outside\n"),0600);err!=nil{t.Fatal(err)}
 alias:=filepath.Join(root,"alias.log")
 escape:=filepath.Join(root,"escape.log")
 escapeDir:=filepath.Join(root,"escape-dir")
 for _,pair:=range [][2]string{{good,alias},{secret,escape},{outside,escapeDir}} {if err:=os.Symlink(pair[0],pair[1]);err!=nil{t.Fatal(err)}}
 run:=func(argv ...string)(string,error) {
  t.Helper()
  command,err:=Validate(argv)
  if err!=nil{t.Fatal(err)}
  out,err:=exec.Command("sh","-c",command).CombinedOutput()
  return string(out),err
 }
 for _,file:=range []string{good,alias} {if out,err:=run("cat",file);err!=nil||out!="MATCH good\n"{t.Fatalf("allowed read failed: %q %v",out,err)}}
 for _,file:=range []string{escape,filepath.Join(escapeDir,"b.log"),root,filepath.Join(root,"missing")} {if out,err:=run("cat",file);err==nil||strings.Contains(out,"MATCH outside"){t.Fatalf("unsafe target read: %q %v",out,err)}}
 if out,err:=run("grep","-rF","--","MATCH",root);err!=nil||!strings.Contains(out,"MATCH good")||strings.Contains(out,"MATCH outside"){t.Fatalf("recursive symlink escape: %q %v",out,err)}
 if out,err:=run("find",root,"-maxdepth","3","-type","f");err!=nil||strings.Contains(out,"b.log"){t.Fatalf("find traversed symlink: %q %v",out,err)}
 if out,err:=run("grep","-F","--","$(id); MATCH",good);err==nil||strings.Contains(out,"uid="){t.Fatalf("literal pattern executed: %q %v",out,err)}
}
