package server

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "sync"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

// Browser setup is an opt-in foreground-controlled installation workflow.
// No arbitrary shell from a request: only the fixed git/npm/node commands
// below are executed. The running daemon and its existing sessions survive
// cancellation or setup errors.
type semanticInstallController struct {
 mu sync.Mutex
 state string
 active bool
 stage string
 message string
 log []string
 manualCommand string
 workerDir string
 startedAt time.Time
 cancel context.CancelFunc
 workerPID int
}

func (c *semanticInstallController) snapshot() map[string]any {
 c.mu.Lock()
 defer c.mu.Unlock()
 logs:=append([]string(nil),c.log...)
 return map[string]any{
  "state":c.state,"active":c.active,"stage":c.stage,"message":c.message,
  "logs":logs,"manual_command":c.manualCommand,
  "worker_dir":c.workerDir,"worker_pid":c.workerPID,
  "started_at":c.startedAt.Format(time.RFC3339),
 }
}
func (c *semanticInstallController) append(message string) {
 c.mu.Lock()
 defer c.mu.Unlock()
 // Output can contain external URLs and typed website content; never retain
 // arbitrary unlimited command output. No process environment is logged.
 message=strings.TrimSpace(message)
 if len(message)>350{message=message[:350]+"..."}
 if message!=""{
  c.log=append(c.log,message)
  if len(c.log)>70{c.log=append([]string(nil),c.log[len(c.log)-70:]...)}
 }
}
func (c *semanticInstallController) step(stage,message,manual string){
 c.mu.Lock()
 defer c.mu.Unlock()
 c.stage=stage
 c.message=message
 c.manualCommand=manual
}
func (c *semanticInstallController) finish(ctx context.Context,state,message,manual string){
 c.mu.Lock()
 defer c.mu.Unlock()
 if ctx.Err()!=nil{return}
 c.state=state
 c.message=message
 c.manualCommand=manual
 if state!="ready"{c.cancel=nil;c.active=false}
}
func (c *semanticInstallController) stop(){
 c.mu.Lock()
 cancel:=c.cancel
 if c.state=="running"||c.state=="ready"{
  c.state="manual"
  c.stage="cancelled"
  c.message="Tự động đã dừng. Có thể tiếp tục thủ công từ bước được hiển thị."
  c.workerPID=0
 }
 // Keep active=true while a cancelled subprocess is still exiting;
 // a quick "start" request cannot overlap with it.
 c.mu.Unlock()
 if cancel!=nil{cancel()}
}
func (c *semanticInstallController) run(ctx context.Context,dir,stage,manual,program string,timeout time.Duration,args ...string)error{
 c.step(stage,"Đang thực hiện "+stage,manual)
 c.append("Bước: "+stage)
 if err:=ctx.Err();err!=nil{return err}
 path,err:=exec.LookPath(program)
 if err!=nil{return fmt.Errorf("chưa cài %s",program)}
 stepCtx,cancel:=context.WithTimeout(ctx,timeout)
 defer cancel()
 cmd:=exec.CommandContext(stepCtx,path,args...)
 cmd.Dir=dir
 cmd.Env=append(os.Environ(),"GIT_TERMINAL_PROMPT=0","CI=1")
 // Output is bounded, sanitized and reduced to progress lines.
 writer:=&semanticInstallOutput{controller:c}
 cmd.Stdout=writer;cmd.Stderr=writer
 semanticConfigureProcessGroup(cmd)
 done:=make(chan struct{})
 go func(){
  select{
  case <-stepCtx.Done():semanticKillProcessGroup(cmd)
  case <-done:
  }
 }()
 err=cmd.Run()
 close(done)
 if err!=nil{
  if ctx.Err()!=nil{return ctx.Err()}
  if errors.Is(stepCtx.Err(),context.DeadlineExceeded){return fmt.Errorf("%s quá thời hạn",stage)}
  return fmt.Errorf("%s không thành công: %w",stage,err)
 }
 c.append("Hoàn tất: "+stage)
 return nil
}
type semanticInstallOutput struct{
 controller *semanticInstallController
 mu sync.Mutex
 pending string
}
func (w *semanticInstallOutput) Write(p []byte)(int,error){
 w.mu.Lock()
 defer w.mu.Unlock()
 n:=len(p)
 text:=w.pending+string(p)
 // Bound output and redact the managed token even if echoed by a tool.
 if settings,err:=readSemanticSettings();err==nil&&settings.Token!=""{
  text=strings.ReplaceAll(text,settings.Token,"[REDACTED]")
 }
 if len(text)>20000{text=text[len(text)-20000:]}
 lines:=strings.Split(text,"\n")
 w.pending=lines[len(lines)-1]
 for _,line:=range lines[:len(lines)-1]{
  line=strings.TrimSpace(strings.ReplaceAll(line,"\r",""))
  if len(line)>350{line=line[:350]}
  if line!=""{w.controller.append(line)}
 }
 return n,nil
}

func (s *Server) executeSemanticInstall(ctx context.Context,hostsInput string,startWorker bool){
 c:=&s.semanticInstaller
 defer func(){
  if ctx.Err()!=nil{
   c.mu.Lock()
   c.active=false
   c.cancel=nil
   c.workerPID=0
   c.mu.Unlock()
  }
 }()
 manual:=""
 runErr:=func()error{
  c.step("configure","Kiểm tra hoặc tạo worker.json","Mở Browser → Setup wizard → Cấu hình")
  if runtime.GOOS=="windows"{return errors.New("Cần chạy TaskDeck trong WSL2 để sử dụng Unix socket")}
  if os.Getenv("TASKDECK_BROWSER_WORKER_SOCKET")!=""||os.Getenv("TASKDECK_BROWSER_WORKER_TOKEN")!=""{
   return errors.New("Biến môi trường cũ đang ghi đè cấu hình; hãy bỏ biến và restart TaskDeck")
  }
  s.semanticSetupMu.Lock()
  saved,err:=readSemanticSettings()
  if err!=nil{
   if !os.IsNotExist(err){s.semanticSetupMu.Unlock();return fmt.Errorf("cấu hình không an toàn hoặc lỗi: %w",err)}
   var hosts []string
   hosts,err=semanticSetupHosts(hostsInput)
   if err==nil{err=semanticCreateManagedSettings(hosts)}
   if err==nil{saved,err=readSemanticSettings()}
   if err==nil{c.append("Đã tạo worker.json với token riêng tư (không hiển thị).")}
  }else if strings.TrimSpace(hostsInput)!=""{
   var requested []string
   requested,err=semanticSetupHosts(hostsInput)
   if err==nil&&strings.Join(requested,",")!=strings.Join(saved.AllowedHosts,","){
    err=errors.New("Domain khác cấu hình đã lưu; hãy lưu thay đổi tại bước Cấu hình rồi khởi chạy lại")
   }
  }
  s.semanticSetupMu.Unlock()
  if err!=nil{return err}

  c.step("locate","Đang tìm browser-worker","")
  worker:=semanticWorkerDir(s.Workspace)
  if worker==""{
   config,err:=semanticSetupFile();if err!=nil{return err}
   root:=filepath.Join(filepath.Dir(config),"worker-source")
   checkout:=filepath.Join(root,"vscode_tasks_menu_go","browser-worker")
   manual="git clone --depth 1 --branch feature/semantic-browser-tab https://github.com/hacrot3000/PatchAndCollectionToolForAI.git "+shellQuote(root)
   if info,err:=os.Lstat(root);os.IsNotExist(err){
    if err=c.run(ctx,"","clone",manual,"git",3*time.Minute,
     "clone","--depth","1","--branch","feature/semantic-browser-tab",
     "https://github.com/hacrot3000/PatchAndCollectionToolForAI.git",root);err!=nil{return err}
   }else if err!=nil{return err
   }else if !info.IsDir(){return errors.New("worker-source là symlink hoặc thư mục không hợp lệ")}
   if !semanticWorkerDirectoryValid(checkout){return errors.New("Source worker tải về không hợp lệ; không tiếp tục tự động")}
   worker=checkout
  }
  c.mu.Lock();c.workerDir=worker;c.mu.Unlock()
  c.append("Worker: "+worker)
  installed:=filepath.Join(worker,"node_modules","playwright","package.json")
  if _,err:=os.Stat(installed);err!=nil{
   manual="cd "+shellQuote(worker)+" && npm install --ignore-scripts --no-audit --no-fund"
   if err=c.run(ctx,worker,"npm install",manual,"npm",5*time.Minute,
    "install","--ignore-scripts","--no-audit","--no-fund");err!=nil{return err}
  }else{c.append("Đã có Playwright; bỏ qua npm install.")}

  manual="cd "+shellQuote(worker)+" && node node_modules/playwright/cli.js install chromium"
  if err=c.run(ctx,worker,"install Chromium",manual,"node",12*time.Minute,
   "node_modules/playwright/cli.js","install","chromium");err!=nil{return err}

  manual="cd "+shellQuote(worker)+" && node start-configured.mjs "+shellQuote(mustSemanticConfigPath())
  if !startWorker{
   c.step("start worker","Đã cài xong. Chạy worker thủ công nếu chưa có môi trường sandbox/firewall.",manual)
   c.finish(ctx,"manual","Cài đặt hoàn tất, chuyển sang bước khởi động thủ công.",manual)
   return nil
  }
  if os.Geteuid()==0{return errors.New("Từ chối chạy Chromium bằng tài khoản root")}
  if semanticSocketReady(saved.Socket){
   c.finish(ctx,"ready","Worker đã hoạt động và xác thực thành công.","")
   return nil
  }
  c.step("start worker","Đang khởi động worker trong môi trường thử nghiệm được xác nhận",manual)
  node,err:=exec.LookPath("node");if err!=nil{return err}
  cmd:=exec.CommandContext(ctx,node,"start-configured.mjs",mustSemanticConfigPath())
  cmd.Dir=worker
  cmd.Stdout=io.Discard
  cmd.Stderr=io.Discard
  semanticConfigureProcessGroup(cmd)
  if err:=cmd.Start();err!=nil{return fmt.Errorf("khởi động worker: %w",err)}
  c.mu.Lock();c.workerPID=cmd.Process.Pid;c.mu.Unlock()
  exited:=make(chan error,1)
  workerExited:=make(chan struct{})
  go func(){exited<-cmd.Wait();close(workerExited)}()
  go func(){
   select{
   case <-ctx.Done():semanticKillProcessGroup(cmd)
   case <-workerExited:
   }
  }()
  deadline:=time.NewTimer(12*time.Second)
  defer deadline.Stop()
  check:=time.NewTicker(250*time.Millisecond)
  defer check.Stop()
  for{
   if semanticSocketReady(saved.Socket){
    c.finish(ctx,"ready","Worker hoạt động; đã xác thực qua socket.","")
    go func(){
     err:=<-exited
     c.mu.Lock()
     if ctx.Err()==nil&&c.state=="ready"{
      c.state="manual"
      c.stage="worker exited"
      c.message=fmt.Sprintf("Worker đã thoát (%v). Khởi động thủ công hoặc cài lại.",err)
      c.manualCommand=manual
      c.workerPID=0
     }
     c.active=false
     c.cancel=nil
     c.mu.Unlock()
    }()
    return nil
   }
   select{
   case err:=<-exited:return fmt.Errorf("worker dừng khi khởi động: %v",err)
   case <-ctx.Done():return ctx.Err()
   case <-deadline.C:
    semanticKillProcessGroup(cmd)
    <-exited
    return errors.New("worker không phản hồi health check trong 12 giây")
   case <-check.C:
   }
  }
 }()
 if runErr!=nil{
  if ctx.Err()!=nil{return}
  c.append("Lỗi: "+runErr.Error())
  if manual==""{
   if value,ok:=c.snapshot()["manual_command"].(string);ok{manual=value}
  }
  c.finish(ctx,"manual",runErr.Error(),manual)
 }
}
func mustSemanticConfigPath()string{path,_:=semanticSetupFile();return path}

func (s *Server) semanticBrowserInstallAPI(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store")
 if r.Method!=http.MethodGet&&r.Method!=http.MethodPost{
  w.Header().Set("Allow","GET, POST")
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
 }
 if s.Config.SharedServerEnabled{
  principal,ok:=PrincipalFromContext(r.Context())
  if !ok{http.Error(w,"authentication required",http.StatusUnauthorized);return}
  if !principal.Allowed(identity.PermissionSettingsWrite)||!principal.Allowed(identity.PermissionBrowserUse){
   http.Error(w,"administrator setup permission required",http.StatusForbidden);return
  }
 }
 if r.Method==http.MethodPost{
  var input struct{
   Action string `json:"action"`
   AllowedHosts string `json:"allowed_hosts"`
   StartWorker bool `json:"start_worker"`
   AcknowledgeIsolation bool `json:"acknowledge_isolation"`
  }
  dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096))
  dec.DisallowUnknownFields()
  if err:=dec.Decode(&input);err!=nil||dec.Decode(new(any))!=io.EOF{
   http.Error(w,"invalid setup action",http.StatusBadRequest);return
  }
  switch input.Action{
  case "cancel":
   s.semanticInstaller.stop()
  case "start":
   if input.StartWorker&&!input.AcknowledgeIsolation{
    http.Error(w,"explicit sandbox/firewall acknowledgement required to start worker",http.StatusBadRequest);return
   }
   if runtime.GOOS=="windows"{http.Error(w,"run TaskDeck inside WSL2",http.StatusBadRequest);return}
   if _,err:=readSemanticSettings();err!=nil{
    if _,err:=semanticSetupHosts(input.AllowedHosts);err!=nil{
     http.Error(w,"enter trusted public domains first",http.StatusBadRequest);return
    }
   }
   c:=&s.semanticInstaller
   c.mu.Lock()
   if c.active{
    c.mu.Unlock()
    http.Error(w,"an installation or managed worker is already active",http.StatusConflict);return
   }
   ctx,cancel:=context.WithCancel(context.Background())
   c.cancel=cancel;c.active=true;c.state="running";c.stage="starting";c.message="Bắt đầu cài đặt"
   c.log=nil;c.manualCommand="";c.workerDir="";c.workerPID=0;c.startedAt=time.Now()
   c.mu.Unlock()
   go s.executeSemanticInstall(ctx,input.AllowedHosts,input.StartWorker)
  default:
   http.Error(w,"unknown setup action",http.StatusBadRequest);return
  }
 }
 writeJSON(w,http.StatusOK,s.semanticInstaller.snapshot())
}
