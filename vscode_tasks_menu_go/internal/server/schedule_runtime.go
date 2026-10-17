package server

import (
 "context"
 "encoding/json"
 "encoding/xml"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/url"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strconv"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/tasks"
)

const scheduleLogLimit = 2<<20
type limitedScheduleWriter struct { file *os.File; n int; }
func (w *limitedScheduleWriter) Write(data []byte)(int,error){
 length:=len(data)
 if w.n<scheduleLogLimit{
  chunk:=data
  if len(chunk)>scheduleLogLimit-w.n{chunk=chunk[:scheduleLogLimit-w.n]}
  n,err:=w.file.Write(chunk);w.n+=n
  if err!=nil{return n,err}
  if w.n==scheduleLogLimit{_,_=w.file.WriteString("\n[TaskDeck schedule console truncated at 2 MiB]\n")}
 }
 return length,nil // keep draining stdout to avoid blocking processes
}
func validateScheduleURL(value string)error{
 u,err:=url.Parse(value);if err!=nil{return err}
 if (u.Scheme!="http"&&u.Scheme!="https")||u.Hostname()==""||u.User!=nil||u.Fragment!=""{return errors.New("URL must be an http(s) URL without credentials or fragments")}
 if strings.EqualFold(u.Hostname(),"localhost"){return errors.New("localhost URL is not permitted")}
 return nil
}
func publicIPs(ctx context.Context,hostname string)([]net.IP,error){
 ips,err:=net.DefaultResolver.LookupIP(ctx,"ip",hostname)
 if err!=nil{return nil,err}
 if len(ips)==0{return nil,errors.New("URL hostname has no addresses")}
 for _,ip:=range ips{
  if !ip.IsGlobalUnicast()||ip.IsPrivate()||ip.IsLoopback()||ip.IsLinkLocalUnicast()||ip.IsLinkLocalMulticast(){
   return nil,errors.New("URL resolves to internal/private address")
  }
  if v4:=ip.To4();v4!=nil{
   if v4[0]==0||v4[0]==10||v4[0]==127||v4[0]>=224||(v4[0]==169&&v4[1]==254)||(v4[0]==100&&v4[1]>=64&&v4[1]<=127)||(v4[0]==192&&v4[1]==0)||(v4[0]==198&&v4[1]==18){return nil,errors.New("URL resolves to restricted IPv4 address")}
  }
 }
 return ips,nil
}
func scheduleFetch(ctx context.Context,target string)(string,error){
 if err:=validateScheduleURL(target);err!=nil{return "",err}
 transport:=&http.Transport{Proxy:nil,DisableKeepAlives:true,MaxIdleConns:0,TLSHandshakeTimeout:5*time.Second,
  DialContext:func(ctx context.Context,network,address string)(net.Conn,error){
   host,port,err:=net.SplitHostPort(address);if err!=nil{return nil,err}
   ips,err:=publicIPs(ctx,host);if err!=nil{return nil,err}
   dialer:=net.Dialer{Timeout:5*time.Second}
   return dialer.DialContext(ctx,"tcp",net.JoinHostPort(ips[0].String(),port))
  },
 }
 defer transport.CloseIdleConnections()
 client:=http.Client{Timeout:8*time.Second,Transport:transport,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
 request,err:=http.NewRequestWithContext(ctx,http.MethodGet,target,nil);if err!=nil{return "",err}
 response,err:=client.Do(request);if err!=nil{return "",err}
 defer response.Body.Close()
 if response.StatusCode<200||response.StatusCode>=300{return "",fmt.Errorf("URL HTTP status %d",response.StatusCode)}
 body,err:=io.ReadAll(io.LimitReader(response.Body,256<<10+1));if err!=nil{return "",err}
 if len(body)>256<<10{return "",errors.New("URL response exceeds 256 KiB")}
 return string(body),nil
}
func scheduleJSONMatch(body,path,expected string)bool{
 var payload any
 if json.Unmarshal([]byte(body),&payload)!=nil{return false}
 for _,part:=range strings.Split(path,"."){
  if part==""{continue}
  switch v:=payload.(type){
  case map[string]any:payload=v[part]
  case []any:
   idx,err:=strconv.Atoi(part);if err!=nil||idx<0||idx>=len(v){return false};payload=v[idx]
  default:return false
  }
 }
 if str,ok:=payload.(string);ok{return str==expected}
 b,err:=json.Marshal(payload);return err==nil&&string(b)==expected
}
func scheduleXMLMatch(body,path,expected string)bool{
 decoder:=xml.NewDecoder(strings.NewReader(body))
 stack:=[]string{}
 for{
  token,err:=decoder.Token();if err!=nil{return false}
  switch t:=token.(type){
  case xml.StartElement:stack=append(stack,t.Name.Local)
  case xml.EndElement:if len(stack)>0{stack=stack[:len(stack)-1]}
  case xml.CharData:
   text:=strings.TrimSpace(string(t))
   if text==""{continue}
   if (path==""||strings.Join(stack,"/")==strings.Trim(path,"/"))&&strings.Contains(text,expected){return true}
  }
 }
}
func (s *Server) scheduleObservation(ctx context.Context,c scheduleDefinition)(string,bool,error){
 switch c.Type{
 case "url":
  body,err:=scheduleFetch(ctx,c.URL)
  if err!=nil{return "",false,err}
  matched:=false
  switch c.MatchMode{
  case "equals":matched=strings.TrimSpace(body)==c.MatchText
  case "contains":matched=strings.Contains(body,c.MatchText)
  case "json":matched=scheduleJSONMatch(body,c.JSONPath,c.MatchText)
  case "xml":matched=scheduleXMLMatch(body,c.JSONPath,c.MatchText)
  }
  if matched{return "matched",true,nil};return "unmatched",false,nil
 case "git":
  repoPath,err:=verifyGitRepository(s.Workspace,filepath.Join(s.Workspace,c.GitPath))
  if err!=nil{return "",false,err}
  ref:=c.GitRef;if ref==""{ref="HEAD"}
  if strings.HasPrefix(ref,"-")||strings.ContainsAny(ref,"\n\r\x00"){return "",false,errors.New("invalid git ref")}
  run:=func(args ...string)(string,error){
   // Git versions shipped on old RHEL/CentOS do not support "git -C".
   cmd:=exec.CommandContext(ctx,"git",args...)
   cmd.Dir=repoPath
   cmd.Env=append(os.Environ(),"GIT_TERMINAL_PROMPT=0","GIT_PAGER=cat","LC_ALL=C")
   output,err:=cmd.Output();return strings.TrimSpace(string(output)),err
  }
  if c.GitCondition=="dirty"||c.GitCondition=="clean"{
   value,err:=run("status","--porcelain","--untracked-files=no")
   if err!=nil{return "",false,err}
   dirty:=value!=""
   if c.GitCondition=="dirty"{return fmt.Sprint(dirty),dirty,nil}
   return fmt.Sprint(!dirty),!dirty,nil
  }
  sha,err:=run("rev-parse","--verify","--end-of-options",ref+"^{commit}")
  if err!=nil{return "",false,err}
  return sha,true,nil
 }
 return "",false,errors.New("observation type not supported")
}
func (s *Server) scheduleCheck(ctx context.Context,c scheduleDefinition){
 e,err:=s.scheduleEngine();if err!=nil{return}
 checkCtx,cancel:=context.WithTimeout(ctx,12*time.Second)
 observation,matched,err:=s.scheduleObservation(checkCtx,c)
 cancel()
 e.mu.Lock()
 prev,exists:=e.edge[c.ID]
 // Failed network/Git observations must not reset a previously matched edge.
 if err!=nil{e.lastStatus[c.ID]=err.Error()}else{e.lastStatus[c.ID]="";e.edge[c.ID]=observation}
 e.mu.Unlock()
 if err!=nil{return}
 if c.Type=="git"&&c.GitCondition=="change"{
  // Bootstrap captures baseline without executing.
  if exists&&prev!=observation{_,_=s.scheduleStart(c.ID,"git ref changed")}
 }else if matched&&(!exists||prev!=observation){
  _,_=s.scheduleStart(c.ID,c.Type+" condition matched")
 }
}
func (s *Server) scheduleTick(ctx context.Context){
 e,err:=s.scheduleEngine();if err!=nil{return}
 now:=time.Now()
 e.mu.Lock()
 definitions:=append([]scheduleDefinition(nil),e.store.Schedules...)
 e.mu.Unlock()
 for _,c:=range definitions{
  if !c.Enabled||c.Type=="dependency"{continue}
  if c.Type=="cron"{
   loc:=time.Local
   if c.Timezone!=""{var err error;loc,err=time.LoadLocation(c.Timezone);if err!=nil{continue}}
   in:=now.In(loc);minute:=in.Format("2006-01-02T15:04-0700")
   matched,err:=cronMatches(c.Cron,in);if err!=nil||!matched{continue}
   e.mu.Lock()
   old:=e.lastCron[c.ID]
   if old!=minute{e.lastCron[c.ID]=minute}
   e.mu.Unlock()
   if old!=minute{_,_=s.scheduleStart(c.ID,"cron "+minute)}
   continue
  }
  e.mu.Lock()
  next:=e.nextCheck[c.ID]
  if now.Before(next){e.mu.Unlock();continue}
  e.nextCheck[c.ID]=now.Add(time.Duration(c.IntervalSeconds)*time.Second)
  e.mu.Unlock()
  select{
  case e.pollSlots<-struct{}{}:
   // Each query has its own timeout. Slow endpoints must not delay cron ticks.
   go func(def scheduleDefinition){
    defer func(){<-e.pollSlots}()
    s.scheduleCheck(ctx,def)
   }(c)
  default:
   // Saturated: retry shortly rather than queuing unbounded goroutines.
   e.mu.Lock();e.nextCheck[c.ID]=now.Add(5*time.Second);e.mu.Unlock()
  }
 }
}
func (s *Server) scheduleLoop(ctx context.Context){
 timer:=time.NewTicker(5*time.Second);defer timer.Stop()
 s.scheduleTick(ctx)
 for{
  select{
  case <-ctx.Done():return
  case <-timer.C:s.scheduleTick(ctx)
  }
 }
}
func (s *Server) scheduleStart(id,reason string)(string,error){
 e,err:=s.scheduleEngine();if err!=nil{return "",err}
 e.mu.Lock()
 config,found:=e.definition(id)
 if !found{e.mu.Unlock();return "",errors.New("schedule not found")}
 if !config.Enabled&&reason!="manual"{e.mu.Unlock();return "",errors.New("schedule disabled")}
 active:=0
 for _,run:=range e.store.Runs{
  if run.Status!="running"{continue}
  if run.ScheduleID==id{active++}
 }
 if (!config.AllowOverlap&&active>0)||active>=4{e.mu.Unlock();return "",errors.New("schedule already running or per-schedule concurrency reached")}
 if len(e.active)>=12{e.mu.Unlock();return "",errors.New("global schedule concurrency limit reached (12)")}
 run:=scheduleRun{ID:newScheduleID(),ScheduleID:id,Name:config.Name,Reason:reason,Status:"running",StartedAt:time.Now().UTC()}
 e.store.Runs=append(e.store.Runs,run)
 if err=e.persistLocked();err!=nil{e.store.Runs=e.store.Runs[:len(e.store.Runs)-1];e.mu.Unlock();return "",err}
 timeout:=config.TimeoutSeconds;if timeout==0{timeout=1800}
 ctx,cancel:=context.WithTimeout(context.Background(),time.Duration(timeout)*time.Second)
 e.active[run.ID]=cancel
 e.mu.Unlock()
 go s.scheduleExecute(ctx,run,config)
 return run.ID,nil
}
func (s *Server) scheduleExecute(ctx context.Context,run scheduleRun,c scheduleDefinition){
 e,err:=s.scheduleEngine();if err!=nil{return}
 finish:=func(exit int,errorText,output string){
  now:=time.Now().UTC()
  e.mu.Lock()
  if cancel,ok:=e.active[run.ID];ok{cancel();delete(e.active,run.ID)}
  for i:=range e.store.Runs{
   if e.store.Runs[i].ID!=run.ID{continue}
   e.store.Runs[i].Status="success"
   if exit!=0{e.store.Runs[i].Status="failure"}
   e.store.Runs[i].ExitCode=exit;e.store.Runs[i].Error=errorText;e.store.Runs[i].FinishedAt=&now
  }
  e.pruneLocked(now)
  _=e.persistLocked()
  dependencies:=[]scheduleDefinition{}
  for _,candidate:=range e.store.Schedules{if candidate.Enabled&&candidate.Type=="dependency"&&candidate.ParentID==c.ID{dependencies=append(dependencies,candidate)}}
  e.mu.Unlock()
  for _,candidate:=range dependencies{
   if candidate.ParentState=="success"&&exit!=0{continue}
   if candidate.ParentState=="failure"&&exit==0{continue}
   if candidate.ExitCode!=nil&&*candidate.ExitCode!=exit{continue}
   if candidate.OutputContains!=""&&!strings.Contains(output,candidate.OutputContains){continue}
   if candidate.OutputAbsent!=""&&strings.Contains(output,candidate.OutputAbsent){continue}
   _,_=s.scheduleStart(candidate.ID,"after "+c.Name+" ["+run.ID+"]")
  }
 }
 logfile,err:=os.OpenFile(filepath.Join(e.folder,"logs",run.ID+".log"),os.O_CREATE|os.O_WRONLY|os.O_EXCL,0600)
 if err!=nil{finish(-1,"create console log: "+err.Error(),"");return}
 writer:=&limitedScheduleWriter{file:logfile}
 var spec tasks.Execution
 if c.Target=="task"{
  selected,workflow,root,err:=s.workspaceWorkflowByTaskID(c.TaskID)
  if err!=nil{fmt.Fprintf(writer,"Cannot resolve task: %v\n",err);logfile.Close();finish(-1,err.Error(),"");return}
  // Background schedules cannot satisfy interactive deploy approvals or secret-use grants.
  if marker,_:=selected.Raw["taskdeckApproval"].(string);strings.EqualFold(marker,"deploy"){
   err=errors.New("scheduled deploy task requires interactive approval")
  }else{
   refs,refErr:=taskWorkflowSecretRefs(workflow,selected.Label)
   if refErr!=nil{err=refErr}else if len(refs)>0{err=errors.New("scheduled task uses secrets and requires interactive authorization")}
  }
  if err==nil{spec,err=tasks.ResolveWorkflowExecution(workflow,selected.Label,root.Path,nil)}
  if err!=nil{fmt.Fprintln(writer,err);logfile.Close();finish(-1,err.Error(),"");return}
 }else{
  shell:="/bin/bash";args:=[]string{"-c",c.Command}
  if runtime.GOOS=="windows"{shell="cmd.exe";args=[]string{"/C",c.Command}}
  spec=tasks.Execution{Command:shell,Args:args,Cwd:s.Workspace,Preview:c.Command}
  if c.WorkingDirectory!=""{
   root,err:=canonicalExistingPath(filepath.Join(s.Workspace,c.WorkingDirectory))
   if err!=nil||!pathInside(s.Workspace,root){logfile.Close();finish(-1,"working_directory invalid","");return}
   spec.Cwd=root
  }
 }
 executable:=spec.Command
 if strings.ContainsRune(executable,filepath.Separator)&&!filepath.IsAbs(executable){
  executable=filepath.Join(spec.Cwd,executable)
 }
 command:=exec.CommandContext(ctx,executable,spec.Args...)
 command.Dir=spec.Cwd
 if len(spec.Env)>0{command.Env=spec.Env}
 command.Stdout=writer;command.Stderr=writer
 fmt.Fprintf(writer,"> %s\n\n",spec.Preview)
 scheduleConfigureProcess(command)
 err=command.Start()
 if err==nil{
  finished:=make(chan error,1)
  go func(){finished<-command.Wait()}()
  select{
  case err=<-finished:
  case <-ctx.Done():
   scheduleTerminateProcessGroup(command)
   err=<-finished
  }
 }
 logfile.Sync();logfile.Close()
 data,_:=os.ReadFile(filepath.Join(e.folder,"logs",run.ID+".log"))
 exit:=0;message:=""
 if err!=nil{exit=-1;message=err.Error();if codeErr,ok:=err.(*exec.ExitError);ok{exit=codeErr.ExitCode()}}
 if ctx.Err()!=nil{exit=-1;message="canceled or timed out: "+ctx.Err().Error()}
 finish(exit,message,string(data))
}
func (s *Server) scheduleStopAll(){
 e,err:=s.scheduleEngine();if err!=nil{return}
 e.mu.Lock()
 for _,cancel:=range e.active{cancel()}
 e.mu.Unlock()
}
func (s *Server) scheduleCancel(id string)error{
 e,err:=s.scheduleEngine();if err!=nil{return err}
 e.mu.Lock();defer e.mu.Unlock()
 cancel,found:=e.active[id];if !found{return errors.New("run is not active")}
 cancel()
 return nil
}
