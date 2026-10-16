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

// Unix socket names have strict platform-specific sockaddr length limits.
// Create a fresh unguessable mode-0700 runtime directory instead of building
// a long path below XDG_CONFIG_HOME. Storing the path in the protected
// settings file preserves it across TaskDeck/worker restarts. Exclusive
// MkdirTemp creation also avoids pre-created directory ownership attacks.
func semanticSetupSocket(_ string)(string,error){
 bases:=[]string{os.Getenv("XDG_RUNTIME_DIR"),os.TempDir(),"/tmp"}
 seen:=map[string]bool{}
 for _,base:=range bases{
  if base==""||!filepath.IsAbs(base)||seen[base]{continue}
  seen[base]=true
  if len([]byte(filepath.Join(base,"td-semantic-XXXXXXXXXX","worker.sock")))>=100{
   continue
  }
  dir,err:=os.MkdirTemp(base,"td-semantic-")
  if err!=nil{return "",err}
  socket:=filepath.Join(dir,"worker.sock")
  if len([]byte(socket))>=100{
   _=os.Remove(dir)
   continue
  }
  info,err:=os.Lstat(dir)
  if err!=nil||!info.IsDir()||info.Mode().Perm()&0077!=0{
   _=os.Remove(dir)
   return "",errors.New("unsafe private browser socket directory")
  }
  return socket,nil
 }
 return "",errors.New("no safe short Unix socket directory available")
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
   strings.ContainsAny(host,"/: @\t\n\r")||check=="localhost"||strings.HasSuffix(check,".localhost")||
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

// Create the private worker.json exactly once for a setup operation.
// The HTTP handler and the automatic installer both serialize through
// s.semanticSetupMu. No secret is returned in an HTTP response or process arg.
func semanticCreateManagedSettings(hosts []string)error{
 path,err:=semanticSetupFile()
 if err!=nil{return err}
 secret:=make([]byte,32)
 if _,err=rand.Read(secret);err!=nil{return err}
 socket,err:=semanticSetupSocket(path)
 if err!=nil{return err}
 settings:=semanticWorkerSettings{Socket:socket,Token:hex.EncodeToString(secret),AllowedHosts:hosts}
 if err:=writeSemanticSettings(settings);err!=nil{
  _=os.Remove(filepath.Dir(socket))
  return err
 }
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

// Look up the worker relative to the running binary (including resolved
// global-install symlinks), the development checkout, and the configured
// workspace. Never scan the entire filesystem or guess that a path exists.
func semanticWorkerDir(workspace string)string{
 roots:=make([]string,0,8)
 // A private writable copy takes precedence over a read-only release tree.
 if config,err:=semanticSetupFile();err==nil{
  roots=append(roots,filepath.Join(filepath.Dir(config),"managed-worker"))
 }
 if exe,err:=os.Executable();err==nil{
  if real,err:=filepath.EvalSymlinks(exe);err==nil{
   roots=append(roots,filepath.Dir(real))
  }
  roots=append(roots,filepath.Dir(exe))
 }
 if wd,err:=os.Getwd();err==nil{roots=append(roots,wd)}
 if workspace!=""{roots=append(roots,workspace)}
 if home,err:=os.UserHomeDir();err==nil{
  roots=append(roots,filepath.Join(home,"taskdeck-browser-source","vscode_tasks_menu_go"))
 }
 if settingsDir,err:=semanticSetupFile();err==nil{
  roots=append(roots,filepath.Join(filepath.Dir(settingsDir),"worker-source","vscode_tasks_menu_go"))
 }
 return semanticWorkerDirFromRoots(roots...)
}

func semanticWorkerDirFromRoots(roots ...string)string{
 visited:=make(map[string]bool)
 for _,root:=range roots{
  if !filepath.IsAbs(root){continue}
  current:=filepath.Clean(root)
  // Executable/build/checkout ancestors; bounded to avoid traversing the
  // entire host or accidentally discovering unrelated global directories.
  for depth:=0;depth<7;depth++{
   for _,candidate:=range []string{
    filepath.Join(current,"browser-worker"),
    filepath.Join(current,"vscode_tasks_menu_go","browser-worker"),
   }{
    if visited[candidate]{continue}
    visited[candidate]=true
    if semanticWorkerDirectoryValid(candidate){
     if canonical,err:=filepath.EvalSymlinks(candidate);err==nil{return canonical}
    }
   }
   parent:=filepath.Dir(current)
   if parent==current{break}
   current=parent
  }
 }
 return ""
}

func semanticWorkerDirectoryValid(dir string)bool{
 st,err:=os.Stat(dir)
 if err!=nil||!st.IsDir(){return false}
 pkg:=filepath.Join(dir,"package.json")
 info,err:=os.Lstat(pkg)
 if err!=nil||!info.Mode().IsRegular()||info.Size()>4096{return false}
 raw,err:=os.ReadFile(pkg)
 if err!=nil{return false}
 var descriptor struct{Name string `json:"name"`}
 if json.Unmarshal(raw,&descriptor)!=nil||descriptor.Name!="taskdeck-semantic-browser-worker"{
  return false
 }
 for _,filename:=range []string{
  "worker.mjs","start-configured.mjs","semantic_extract.mjs",
  "universal_input.mjs","popup_runtime.mjs","auth_runtime.mjs","chrome_identity.mjs",
 }{
  file,err:=os.Lstat(filepath.Join(dir,filename))
  if err!=nil||!file.Mode().IsRegular(){return false}
 }
 return true
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

func shellQuote(value string)string{return "'"+strings.ReplaceAll(value,"'","'\"'\"'")+"'"}
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
  // A binary-only or older global release may not contain worker assets.
  // Never print a made-up local path; provide an executable checkout fallback.
  steps=append(steps,map[string]string{
   "label":"Worker not installed: fetch the optional source (Git access required)",
   "command":"git clone --depth 1 --branch feature/semantic-browser-tab https://github.com/hacrot3000/PatchAndCollectionToolForAI.git taskdeck-browser-source && cd taskdeck-browser-source/vscode_tasks_menu_go/browser-worker",
  })
 }else{
  steps=append(steps,map[string]string{"label":"Open detected worker directory","command":"cd "+shellQuote(workerDir)})
 }
 steps=append(steps,
  map[string]string{"label":"Install Node dependencies","command":"npm install --ignore-scripts"},
  map[string]string{"label":"Install Playwright Chromium","command":"npx playwright install chromium"},
 )
 if configPath!=""{
  if _,err:=readSemanticSettings();err==nil{
   steps=append(steps,map[string]string{
    "label":"Start worker (only on an isolated / trusted host)",
    "command":"node start-configured.mjs "+shellQuote(configPath),
   })
  }else{
   steps=append(steps,map[string]string{
    "label":"Create the missing worker.json before starting",
    "command":"# Open Browser > Setup wizard > Configure and save trusted domains to generate private worker.json; then recheck.",
   })
  }
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
  s.semanticSetupMu.Lock()
  err=semanticCreateManagedSettings(hosts)
  s.semanticSetupMu.Unlock()
  if err!=nil{http.Error(w,"could not securely save worker settings",503);return}
 }
 settings,readErr:=readSemanticSettings()
 socket,token:=semanticWorkerConnection()
 nodeFound,nodeVersion:=semanticExecVersion(r.Context(),"node")
 npmFound,npmVersion:=semanticExecVersion(r.Context(),"npm")
 dockerFound,dockerVersion:=semanticExecVersion(r.Context(),"docker")
 workerDir:=semanticWorkerDir(s.Workspace)
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
