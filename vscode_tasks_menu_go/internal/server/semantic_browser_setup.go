package server

// Semantic Browser installer / setup status. Keep secrets out of JSON responses.
import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "net"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strconv"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

type semanticWorkerSettings struct {
 Socket string `json:"socket"`
 Token string `json:"token"`
 AllowedHosts []string `json:"allowed_hosts"`
}

func semanticSetupFile()(string,error) {
 dir,err:=os.UserConfigDir()
 if err!=nil{return "",err}
 return filepath.Join(dir,"taskdeck-semantic-browser","worker.json"),nil
}

func readSemanticSettings()(semanticWorkerSettings,error){
 var out semanticWorkerSettings
 path,err:=semanticSetupFile();if err!=nil{return out,err}
 stat,err:=os.Lstat(path);if err!=nil{return out,err}
 if !stat.Mode().IsRegular()||stat.Mode().Perm()&0077!=0||stat.Size()>8192{return out,errors.New("unsafe worker settings file")}
 f,err:=os.Open(path);if err!=nil{return out,err}
 defer f.Close()
 opened,err:=f.Stat()
 if err!=nil||!os.SameFile(stat,opened)||!opened.Mode().IsRegular()||opened.Mode().Perm()&0077!=0{return out,errors.New("worker settings changed while opening")}
 raw,err:=io.ReadAll(io.LimitReader(f,8193))
 if err!=nil||len(raw)>8192{return out,errors.New("worker settings too large")}
 if err=json.Unmarshal(raw,&out);err!=nil{return out,err}
 if !filepath.IsAbs(out.Socket)||len(out.Token)<48||len(out.AllowedHosts)==0{return semanticWorkerSettings{},errors.New("incomplete worker settings")}
 return out,nil
}

// Explicit legacy environment configuration takes precedence, preserving
// existing installations. A partially configured env fails closed.
func semanticWorkerConnection()(string,string) {
 socket:=strings.TrimSpace(os.Getenv("TASKDECK_BROWSER_WORKER_SOCKET"))
 token:=strings.TrimSpace(os.Getenv("TASKDECK_BROWSER_WORKER_TOKEN"))
 if socket!=""||token!="" {
  if !filepath.IsAbs(socket)||len(token)<24{return "",""}
  return socket,token
 }
 settings,err:=readSemanticSettings()
 if err!=nil{return "",""}
 return settings.Socket,settings.Token
}

func semanticSetupHosts(input string)([]string,error){
 parts:=strings.Split(strings.ToLower(strings.TrimSpace(input)),",")
 if len(parts)>24{return nil,errors.New("maximum 24 trusted domains")}
 hosts:=make([]string,0,len(parts));seen:=make(map[string]bool)
 for _,part:=range parts{
  host:=strings.TrimSpace(part)
  if strings.HasPrefix(host,"*."){host=host[2:];if len(host)==0{return nil,errors.New("invalid wildcard")};host="*."+host}
  check:=strings.TrimPrefix(host,"*.")
  if len(check)<4||len(check)>253||!strings.Contains(check,".")||
   strings.ContainsAny(host,"/: @	
")||check=="localhost"||strings.HasSuffix(check,".localhost")||
   strings.HasSuffix(check,".local")||net.ParseIP(check)!=nil {
    return nil,errors.New("enter exact public DNS hostnames, separated by commas")
  }
  for _,label:=range strings.Split(check,"."){
   if label==""||len(label)>63||label[0]=='-'||label[len(label)-1]=='-'{return nil,errors.New("invalid hostname")}
   for _,c:=range label{if !(c>='a'&&c<='z'||c>='0'&&c<='9'||c=='-'){return nil,errors.New("invalid hostname")}}
  }
  if !seen[host]{hosts=append(hosts,host);seen[host]=true}
 }
 if len(hosts)==0{return nil,errors.New("at least one public hostname is required")}
 return hosts,nil
}

func writeSemanticSettings(settings semanticWorkerSettings)error{
 path,err:=semanticSetupFile();if err!=nil{return err}
 dir:=filepath.Dir(path)
 if err=os.MkdirAll(dir,0700);err!=nil{return err}
 parent,err:=os.Lstat(dir)
 if err!=nil||!parent.IsDir()||parent.Mode().Perm()&0077!=0{return errors.New("unsafe private config directory")}
 if existing,err:=os.Lstat(path);err==nil{
  if !existing.Mode().IsRegular()||existing.Mode().Perm()&0077!=0{return errors.New("unsafe existing worker settings")}
 }else if !errors.Is(err,os.ErrNotExist){return err}
 raw,err:=json.Marshal(settings);if err!=nil{return err}
 temp,err:=os.CreateTemp(dir,".semantic-worker-*");if err!=nil{return err}
 defer os.Remove(temp.Name())
 defer temp.Close()
 if err=temp.Chmod(0600);err!=nil{return err}
 if _,err=temp.Write(raw);err!=nil{return err}
 if err=temp.Sync();err!=nil{return err}
 if err=temp.Close();err!=nil{return err}
 if err=os.Rename(temp.Name(),path);err!=nil{return err}
 if d,err:=os.Open(dir);err==nil{_ =d.Sync();_ =d.Close()}
 return nil
}

func semanticSetupOS()string{
 if runtime.GOOS=="linux"{
  if os.Getenv("WSL_DISTRO_NAME")!="" {return "wsl"}
  if raw,err:=os.ReadFile("/proc/version");err==nil&&strings.Contains(strings.ToLower(string(raw)),"microsoft"){return "wsl"}
  if _,err:=os.Stat("/.dockerenv");err==nil{return "container"}
 }
 return runtime.GOOS
}

func semanticExecVersion(ctx context.Context,command string)(bool,string){
 path,err:=exec.LookPath(command);if err!=nil{return false,"not found"}
 ctx,cancel:=context.WithTimeout(ctx,2*time.Second);defer cancel()
 out,err:=exec.CommandContext(ctx,path,"--version").Output()
 if err!=nil{return false,"installed (version check failed)"}
 return true,strings.TrimSpace(string(out))
}

func semanticWorkerDir()string{
 wd,err:=os.Getwd();if err!=nil{return ""}
 choices:=[]string{
  filepath.Join(wd,"browser-worker"),
  filepath.Join(wd,"vscode_tasks_menu_go","browser-worker"),
  filepath.Join(filepath.Dir(wd),"browser-worker"),
 }
 for _,path:=range choices{
  if stat,err:=os.Stat(filepath.Join(path,"package.json"));err==nil&&stat.Mode().IsRegular(){return path}
 }
 return ""
}

func semanticSocketReady(socket string)bool{
 if socket=="" {return false}
 stat,err:=os.Lstat(socket)
 if err!=nil||stat.Mode()&os.ModeSocket==0||stat.Mode().Perm()&0077!=0{return false}
 active,token:=semanticWorkerConnection()
 if active!=socket||token==""{return false}
 ctx,cancel:=context.WithTimeout(context.Background(),900*time.Millisecond)
 defer cancel()
 transport:=&http.Transport{Proxy:nil,DisableKeepAlives:true,
  DialContext:func(ctx context.Context,network,address string)(net.Conn,error){
   var dialer net.Dialer
   return dialer.DialContext(ctx,"unix",socket)
  },
 }
 defer transport.CloseIdleConnections()
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,"http://taskdeck-worker/health",nil)
 if err!=nil{return false}
 req.Header.Set("X-TaskDeck-Worker-Token",token)
 resp,err:=(&http.Client{Transport:transport}).Do(req)
 if err!=nil{return false}
 defer resp.Body.Close()
 if resp.StatusCode!=http.StatusOK{return false}
 var health struct{OK bool `json:"ok"`}
 decoder:=json.NewDecoder(io.LimitReader(resp.Body,1024))
 return decoder.Decode(&health)==nil&&health.OK
}

func shellQuote(value string)string{return "'"+strings.ReplaceAll(value,"'","'\''")+"'"}
func semanticNodeInstallHint(platform string)map[string]string {
 if platform=="darwin"{
  if _,err:=exec.LookPath("brew");err==nil{
   return map[string]string{"label":"Install Node.js 20+ (Homebrew)","command":"brew install node && node --version"}
  }
  return map[string]string{"label":"Install Node.js 20+","command":"# Install a current Node.js LTS release from https://nodejs.org/en/download"}
 }
 if _,err:=exec.LookPath("dnf");err==nil{
  return map[string]string{"label":"Install Node.js and npm (Fedora/RHEL)","command":"sudo dnf install nodejs npm && node --version"}
 }
 if _,err:=exec.LookPath("apt-get");err==nil{
  return map[string]string{"label":"Install Node.js and npm (Debian/Ubuntu)","command":"sudo apt-get update && sudo apt-get install -y nodejs npm && node --version # If version is below 20, use https://nodejs.org/en/download"}
 }
 if _,err:=exec.LookPath("pacman");err==nil{
  return map[string]string{"label":"Install Node.js and npm (Arch)","command":"sudo pacman -S nodejs npm && node --version"}
 }
 return map[string]string{"label":"Install Node.js 20+","command":"# Install a current LTS release: https://nodejs.org/en/download"}
}
func semanticSetupInstructions(configPath,workerDir,platform string)[]map[string]string{
 steps:=[]map[string]string{}
 if platform=="windows"{
  return []map[string]string{
   {"label":"Install WSL2 (Administrator PowerShell)","command":"wsl --install -d Ubuntu"},
   {"label":"Run TaskDeck and its worker together inside WSL","command":"# Open Ubuntu / WSL. Both processes must share the Linux Unix socket."},
  }
 }
 nodeFound,nodeVersion:=semanticExecVersion(context.Background(),"node")
 nodeOkay:=false
 if nodeFound&&strings.HasPrefix(nodeVersion,"v") {
  major,err:=strconv.Atoi(strings.Split(strings.TrimPrefix(nodeVersion,"v"),".")[0])
  nodeOkay=err==nil&&major>=20
 }
 if !nodeOkay {steps=append(steps,semanticNodeInstallHint(platform))}
 if workerDir==""{
  steps=append(steps,map[string]string{"label":"Find worker directory","command":"cd /path/to/PatchAndCollectionToolForAI/vscode_tasks_menu_go/browser-worker"})
 }else{
  steps=append(steps,map[string]string{"label":"Open worker directory","command":"cd "+shellQuote(workerDir)})
 }
 steps=append(steps,
  map[string]string{"label":"Install Node dependencies","command":"npm install --ignore-scripts"},
  map[string]string{"label":"Install Playwright Chromium","command":"npx playwright install chromium"},
 )
 if configPath!=""{
  steps=append(steps,map[string]string{
   "label":"Start isolated worker (after firewall/sandbox configuration)",
   "command":"node start-configured.mjs "+shellQuote(configPath),
  })
 }
 return steps
}

// GET is read-only; POST generates and persists a secret only on the server.
// Shared users must hold both administrative settings.write and browser.use.
func (s *Server) semanticBrowserSetupAPI(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store")
 if r.Method!=http.MethodGet&&r.Method!=http.MethodPost{
  w.Header().Set("Allow","GET, POST");http.Error(w,"method not allowed",405);return
 }
 if s.Config.SharedServerEnabled{
  principal,ok:=PrincipalFromContext(r.Context())
  if !ok{http.Error(w,"authentication required",401);return}
  if !principal.Allowed(identity.PermissionSettingsWrite)||!principal.Allowed(identity.PermissionBrowserUse){
   http.Error(w,"administrator setup permission required",403);return
  }
 }
 platform:=semanticSetupOS()
 configPath,err:=semanticSetupFile()
 if err!=nil{http.Error(w,"configuration directory unavailable",503);return}
 if r.Method==http.MethodPost{
  if strings.TrimSpace(os.Getenv("TASKDECK_BROWSER_WORKER_SOCKET"))!=""||
     strings.TrimSpace(os.Getenv("TASKDECK_BROWSER_WORKER_TOKEN"))!=""{
   http.Error(w,"Legacy environment settings override managed setup; remove them and restart TaskDeck first",http.StatusConflict);return
  }
  if runtime.GOOS=="windows"{
   http.Error(w,"Run TaskDeck in WSL2 to configure its Unix socket worker",400);return
  }
  var input struct{AllowedHosts string `json:"allowed_hosts"`}
  decoder:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096))
  decoder.DisallowUnknownFields()
  if decoder.Decode(&input)!=nil||decoder.Decode(new(any))!=io.EOF{
   http.Error(w,"invalid worker settings request",400);return
  }
  hosts,err:=semanticSetupHosts(input.AllowedHosts)
  if err!=nil{http.Error(w,err.Error(),400);return}
  secret:=make([]byte,32)
  if _,err:=rand.Read(secret);err!=nil{http.Error(w,"secure token generation unavailable",503);return}
  settings:=semanticWorkerSettings{
   Socket:filepath.Join(filepath.Dir(configPath),"browser.sock"),
   Token:hex.EncodeToString(secret),
   AllowedHosts:hosts,
  }
  if err:=writeSemanticSettings(settings);err!=nil{http.Error(w,"could not securely save worker settings",503);return}
 }
 settings,readErr:=readSemanticSettings()
 socket,token:=semanticWorkerConnection()
 nodeFound,nodeVersion:=semanticExecVersion(r.Context(),"node")
 npmFound,npmVersion:=semanticExecVersion(r.Context(),"npm")
 dockerFound,dockerVersion:=semanticExecVersion(r.Context(),"docker")
 workerDir:=semanticWorkerDir()
 playwrightInstalled:=false
 if workerDir!=""{
  info,err:=os.Stat(filepath.Join(workerDir,"node_modules","playwright","package.json"))
  playwrightInstalled=err==nil&&info.Mode().IsRegular()
 }
 source:="none"
 if os.Getenv("TASKDECK_BROWSER_WORKER_SOCKET")!=""||os.Getenv("TASKDECK_BROWSER_WORKER_TOKEN")!=""{source="environment"
 }else if readErr==nil{source="managed"}
 nodeSupported:=false
 if nodeFound&&strings.HasPrefix(nodeVersion,"v"){
  major,err:=strconv.Atoi(strings.Split(strings.TrimPrefix(nodeVersion,"v"),".")[0])
  nodeSupported=err==nil&&major>=20
 }
 writeJSON(w,200,map[string]any{
  "platform":platform,"go_os":runtime.GOOS,"config_path":configPath,
  "configured":socket!=""&&token!="","worker_connected":semanticSocketReady(socket),
  "socket":socket,"source":source,
  "managed":readErr==nil,"allowed_hosts":settings.AllowedHosts,
  "node_installed":nodeFound,"node_version":nodeVersion,"node_supported":nodeSupported,
  "npm_installed":npmFound,"npm_version":npmVersion,"docker_installed":dockerFound,"docker_version":dockerVersion,
  "worker_dir":workerDir,"playwright_installed":playwrightInstalled,
  "commands":semanticSetupInstructions(configPath,workerDir,platform),
  "requires_isolation":true,
 })
}
