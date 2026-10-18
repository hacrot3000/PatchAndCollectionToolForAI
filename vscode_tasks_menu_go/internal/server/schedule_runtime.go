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
 "reflect"
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
 decoder:=json.NewDecoder(strings.NewReader(body));decoder.UseNumber()
 if decoder.Decode(&payload)!=nil{return false}
 path=strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(path),"$."),"$")
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
 case "file":return s.scheduleFileObservation(ctx,c)
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
  // The ref is validated above so older Git 1.7 hosts do not need --end-of-options.
  sha,err:=run("rev-parse","--verify",ref+"^{commit}")
  if err!=nil{return "",false,err}
  return sha,true,nil
 }
 return "",false,errors.New("observation type not supported")
}
// scheduleSignal atomically collects condition edges for AND groups and
// fires OR groups on any matching edge. A successful start consumes only
// the matched event state; failed starts retain it for later events.
func (s *Server) scheduleSignal(id,conditionID,reason string){
 s.scheduleSignalWithSource(id,conditionID,reason,"","")
}
func (s *Server) scheduleSignalWithSource(id,conditionID,reason,flowID,originRunID string){
 e,err:=s.scheduleEngine();if err!=nil{return}
 e.signalMu.Lock()
 defer e.signalMu.Unlock()
 e.mu.Lock()
 cfg,found:=e.definition(id)
 if !found||!cfg.Enabled||cfg.Paused{e.mu.Unlock();return}
 valid:=false
 for _,existing:=range scheduleLeaves(scheduleConditionsFor(cfg)){if existing.ID==conditionID{valid=true;break}}
 if !valid{e.mu.Unlock();return}
 if e.latched[id]==nil{e.latched[id]=map[string]bool{}}
 if e.latchAt[id]==nil{e.latchAt[id]=map[string]time.Time{}}
 if e.latchFlow[id]==nil{e.latchFlow[id]=map[string]string{}}
 if e.latchOrigin[id]==nil{e.latchOrigin[id]=map[string]string{}}
 now:=time.Now().UTC()
 e.latched[id][conditionID]=true
 e.latchAt[id][conditionID]=now
 e.latchFlow[id][conditionID]=flowID
 e.latchOrigin[id][conditionID]=originRunID
 met:=scheduleActiveSignals(cfg,e.latched[id],e.latchAt[id],now)
 shouldStart,sourceWorkflow,sourceRun:=scheduleEvaluateWithWorkflow(scheduleConditionsFor(cfg),met,e.latchFlow[id],e.latchOrigin[id])
 sources:=[]string{}
 seen:=map[string]bool{}
 for _,condition:=range scheduleLeaves(scheduleConditionsFor(cfg)){
  if !met[condition.ID]||condition.Type!="dependency"{continue}
  if sourceWorkflow!=""&&e.latchFlow[id][condition.ID]!=sourceWorkflow{continue}
  runID:=e.latchOrigin[id][condition.ID]
  if runID!=""&&!seen[runID]{seen[runID]=true;sources=append(sources,runID)}
 }
 e.mu.Unlock()
 if !shouldStart{return}
 if _,err=s.scheduleStartWithWorkflow(id,reason,sourceWorkflow,sourceRun,sources...);err==nil{
  e.mu.Lock()
  delete(e.latched,id);delete(e.latchAt,id)
  delete(e.latchFlow,id);delete(e.latchOrigin,id)
  e.mu.Unlock()
 }
}
func (s *Server) scheduleCheck(ctx context.Context,def scheduleDefinition,c scheduleCondition){
 e,err:=s.scheduleEngine();if err!=nil{return}
 // Ensure pending checks for modified/deleted conditions are not reactivated.
 key:=def.ID+":"+c.ID
 checkCtx,cancel:=context.WithTimeout(ctx,12*time.Second)
 observation,matched,err:=s.scheduleObservation(checkCtx,scheduleDefinition{
  Type:c.Type,URL:c.URL,MatchMode:c.MatchMode,MatchText:c.MatchText,
  JSONPath:c.JSONPath,GitPath:c.GitPath,GitRef:c.GitRef,GitCondition:c.GitCondition,
  FilePath:c.FilePath,FileCondition:c.FileCondition,
 })
 cancel()
 e.mu.Lock()
 current,found:=e.definition(def.ID)
 if !found||!current.Enabled{e.mu.Unlock();return}
 stillPresent:=false
 for _,existing:=range scheduleLeaves(scheduleConditionsFor(current)){
  if existing.ID==c.ID&&reflect.DeepEqual(existing,c){stillPresent=true;break}
 }
 if !stillPresent{e.mu.Unlock();return}
 prev,exists:=e.edge[key]
 if err!=nil{e.lastStatus[key]=err.Error()}else{e.lastStatus[key]="";e.edge[key]=observation}
 e.conditionObserved[key]=scheduleConditionObservation{
  Matched:matched,CheckedAt:time.Now().UTC(),NextCheck:e.nextCheck[key],Error:e.lastStatus[key],
 }
 e.mu.Unlock()
 if err!=nil{return}
 if (c.Type=="git"&&c.GitCondition=="change")||(c.Type=="file"&&c.FileCondition=="change"){
  if exists&&prev!=observation{s.scheduleSignal(def.ID,c.ID,"git ref changed")}
 }else if matched&&(!exists||prev!=observation){
  s.scheduleSignal(def.ID,c.ID,c.Type+" condition matched")
 }
}
func (s *Server) scheduleTick(ctx context.Context){
 e,err:=s.scheduleEngine();if err!=nil{return}
 defer s.scheduleDequeue()
 now:=time.Now()
 e.mu.Lock()
 definitions:=append([]scheduleDefinition(nil),e.store.Schedules...)
 e.mu.Unlock()
 for _,def:=range definitions{
  if !def.Enabled||def.Paused{continue}
  for _,c:=range scheduleLeaves(scheduleConditionsFor(def)){
   if c.Type=="dependency"||c.Type=="webhook"{continue}
   key:=def.ID+":"+c.ID
   if c.Type=="cron"{
    loc:=time.Local
    if c.Timezone!=""&&c.Timezone!="Local"{
     var zoneErr error;loc,zoneErr=time.LoadLocation(c.Timezone)
     if zoneErr!=nil{continue}
    }
    in:=now.In(loc);minute:=in.Format("2006-01-02T15:04-0700")
    matched,matchErr:=cronMatches(c.Cron,in);if matchErr!=nil{continue}
    e.mu.Lock();old,initialized:=e.lastCron[key]
    e.conditionObserved[key]=scheduleConditionObservation{Matched:matched,CheckedAt:now.UTC(),NextCheck:now.Truncate(time.Minute).Add(time.Minute).UTC()}
    if old!=minute{e.lastCron[key]=minute}
    e.mu.Unlock()
    if initialized&&old!=minute&&matched{s.scheduleSignal(def.ID,c.ID,"cron "+minute)}
    continue
   }
   e.mu.Lock()
   next:=e.nextCheck[key]
   if now.Before(next){e.mu.Unlock();continue}
   e.nextCheck[key]=now.Add(time.Duration(c.IntervalSeconds)*time.Second)
   e.mu.Unlock()
   select{
   case e.pollSlots<-struct{}{}:
    go func(item scheduleDefinition,condition scheduleCondition){
     defer func(){<-e.pollSlots}()
     s.scheduleCheck(ctx,item,condition)
    }(def,c)
   default:
    e.mu.Lock();e.nextCheck[key]=now.Add(5*time.Second);e.mu.Unlock()
   }
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
 return s.scheduleStartWithWorkflow(id,reason,"","")
}
func (s *Server) scheduleStartWithWorkflow(id,reason,workflowID,originRunID string,sourceRunIDs ...string)(string,error){
 e,err:=s.scheduleEngine();if err!=nil{return "",err}
 e.mu.Lock()
 config,found:=e.definition(id)
 if !found{e.mu.Unlock();return "",errors.New("schedule not found")}
 if (!config.Enabled||config.Paused)&&reason!="manual"{e.mu.Unlock();return "",errors.New("schedule disabled or paused")}
 if reason!="manual"&&!scheduleCalendarAllows(config,time.Now()){e.mu.Unlock();return "",errors.New("schedule outside execution calendar")}
 active:=0
 for _,run:=range e.store.Runs{
  if run.Status!="running"{continue}
  if run.ScheduleID==id{active++}
 }
 policy:=config.ConcurrencyPolicy
 if policy==""{if config.AllowOverlap{policy="parallel"}else{policy="skip"}}
 allowOverlap:=policy=="parallel"
 resourceBusy:=e.scheduleResourceBusyLocked(config)
 busy:=(!allowOverlap&&active>0)||active>=4||resourceBusy||len(e.active)>=12
 if workflowID==""{workflowID=newScheduleID()}
 run:=scheduleRun{ID:newScheduleID(),ScheduleID:id,Name:config.Name,Reason:reason,Status:"running",
  WorkflowID:workflowID,TriggerRunID:originRunID,TriggerRunIDs:sourceRunIDs,Attempts:1,StartedAt:time.Now().UTC()}
 if busy{
  if policy!="queue"&&policy!="replace"{
   e.mu.Unlock();return "",errors.New("schedule concurrency or resource-group limit reached")
  }
  if policy=="replace"{
   for _,other:=range e.store.Runs{
    if other.ScheduleID==id&&other.Status=="running"{
     if cancel:=e.active[other.ID];cancel!=nil{cancel()}
    }
   }
  }
  if err:=e.scheduleQueueLocked(config,run);err!=nil{e.mu.Unlock();return "",err}
  e.mu.Unlock()
  return run.ID,nil
 }
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
   e.store.Runs[i].Outputs=scheduleParseOutputs(output)
  }
  e.pruneLocked(now)
  _=e.persistLocked()
  dependencies:=append([]scheduleDefinition(nil),e.store.Schedules...)
  e.mu.Unlock()
  // A completion can release both concurrency and exclusive-resource locks.
  defer s.scheduleDequeue()
  for _,candidate:=range dependencies{
   if !candidate.Enabled{continue}
   for _,condition:=range scheduleLeaves(scheduleConditionsFor(candidate)){
    if condition.Type!="dependency"||condition.ParentID!=c.ID{continue}
    if condition.ParentState=="success"&&exit!=0{continue}
    if condition.ParentState=="failure"&&exit==0{continue}
    if condition.ExitCode!=nil&&*condition.ExitCode!=exit{continue}
    if condition.OutputContains!=""&&!strings.Contains(output,condition.OutputContains){continue}
    if condition.OutputAbsent!=""&&strings.Contains(output,condition.OutputAbsent){continue}
    s.scheduleSignalWithSource(candidate.ID,condition.ID,"after "+c.Name+" ["+run.ID+"]",run.WorkflowID,run.ID)
   }
  }
 }
 maxAttempts:=1+c.RetryMax
 if maxAttempts>11{maxAttempts=11}
 delay:=c.RetryDelaySeconds;if delay<=0{delay=30}
 backoff:=c.RetryBackoff;if backoff<1{backoff=2}
 for attempt:=1;attempt<=maxAttempts;attempt++{
  if ctx.Err()!=nil{finish(-1,ctx.Err().Error(),"");return}
  e.mu.Lock()
  for i:=range e.store.Runs{if e.store.Runs[i].ID==run.ID{e.store.Runs[i].Attempts=attempt;break}}
  _=e.persistLocked()
  e.mu.Unlock()
  exit,message,output:=s.scheduleExecuteAttempt(ctx,run,c,attempt)
  if !scheduleShouldRetry(exit,attempt,maxAttempts,c.RetryExitCodes,ctx.Err()!=nil){finish(exit,message,output);return}
  wait:=time.Duration(delay)*time.Second
  timer:=time.NewTimer(wait)
  select{
  case <-ctx.Done():
   timer.Stop();finish(-1,ctx.Err().Error(),output);return
  case <-timer.C:
  }
  delay=int(float64(delay)*backoff)
  if delay>86400{delay=86400}
 }
}
func (s *Server) scheduleExecuteAttempt(ctx context.Context,run scheduleRun,c scheduleDefinition,attempt int)(int,string,string){
 e,err:=s.scheduleEngine();if err!=nil{return -1,err.Error(),""}
 mode:=os.O_CREATE|os.O_WRONLY|os.O_EXCL
 if attempt>1{mode=os.O_WRONLY|os.O_APPEND}
 logfile,err:=os.OpenFile(filepath.Join(e.folder,"logs",run.ID+".log"),mode,0600)
 if err!=nil{return -1,"create console log: "+err.Error(),""}
 writer:=&limitedScheduleWriter{file:logfile}
 if attempt>1{
  if info,statErr:=logfile.Stat();statErr==nil{
   writer.n=int(info.Size())
   if writer.n>scheduleLogLimit{writer.n=scheduleLogLimit}
  }
  _,_=fmt.Fprintf(writer,"\n--- Retry attempt %d ---\n",attempt)
 }
 var spec tasks.Execution
 if c.Target=="task"{
  selected,workflow,root,err:=s.workspaceWorkflowByTaskID(c.TaskID)
  if err!=nil{fmt.Fprintf(writer,"Cannot resolve task: %v\n",err);logfile.Close();return -1,err.Error(),""}
  // Background schedules cannot satisfy interactive deploy approvals or secret-use grants.
  if marker,_:=selected.Raw["taskdeckApproval"].(string);strings.EqualFold(marker,"deploy"){
   err=errors.New("scheduled deploy task requires interactive approval")
  }else{
   refs,refErr:=taskWorkflowSecretRefs(workflow,selected.Label)
   if refErr!=nil{err=refErr}else if len(refs)>0{err=errors.New("scheduled task uses secrets and requires interactive authorization")}
  }
  if err==nil{spec,err=tasks.ResolveWorkflowExecution(workflow,selected.Label,root.Path,nil)}
  if err!=nil{fmt.Fprintln(writer,err);logfile.Close();return -1,err.Error(),""}
 }else{
  shell:="/bin/bash";args:=[]string{"-c",c.Command}
  if runtime.GOOS=="windows"{shell="cmd.exe";args=[]string{"/C",c.Command}}
  spec=tasks.Execution{Command:shell,Args:args,Cwd:s.Workspace,Preview:c.Command}
  if c.WorkingDirectory!=""{
   root,err:=canonicalExistingPath(filepath.Join(s.Workspace,c.WorkingDirectory))
   if err!=nil||!pathInside(s.Workspace,root){logfile.Close();return -1,"working_directory invalid",""}
   spec.Cwd=root
  }
 }
 executable:=spec.Command
 if strings.ContainsRune(executable,filepath.Separator)&&!filepath.IsAbs(executable){
  executable=filepath.Join(spec.Cwd,executable)
 }
 command:=exec.CommandContext(ctx,executable,spec.Args...)
 command.Dir=spec.Cwd
 // Parent outputs are accessible as TASKDECK_DEP_NAME only. They are never
 // interpolated into a command line and cannot override arbitrary variables.
 e.mu.Lock()
 sourceEnv:=scheduleDependencyEnv(run.TriggerRunID,e.store.Runs)
 e.mu.Unlock()
 command.Env=scheduleEnvironment(spec.Env,sourceEnv)
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
 return exit,message,string(data)

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
 cancel,found:=e.active[id]
 if found{cancel();return nil}
 for i:=range e.store.Runs{
  if e.store.Runs[i].ID!=id||e.store.Runs[i].Status!="queued"{continue}
  now:=time.Now().UTC()
  e.store.Runs[i].Status="interrupted";e.store.Runs[i].Error="Cancelled before execution"
  e.store.Runs[i].ExitCode=-1;e.store.Runs[i].FinishedAt=&now
  return e.persistLocked()
 }
 return errors.New("run is not running or queued")
}
