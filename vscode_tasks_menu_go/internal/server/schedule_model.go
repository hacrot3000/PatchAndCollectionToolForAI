package server

import (
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "strconv"
 "strings"
 "sync"
 "time"

 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// Schedule configs and run metadata are stored outside the source workspace.
// All active subprocesses and runtime trigger edges live in memory.
type scheduleDefinition struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Enabled bool `json:"enabled"`
 Type string `json:"type"` // legacy cron/url/git/dependency or multi
 ConditionGroup *scheduleConditionGroup `json:"condition_group,omitempty"`
 Cron string `json:"cron,omitempty"`
 Timezone string `json:"timezone,omitempty"`
 IntervalSeconds int `json:"interval_seconds,omitempty"`
 URL string `json:"url,omitempty"`
 MatchMode string `json:"match_mode,omitempty"` // contains, equals, json, xml
 MatchText string `json:"match_text,omitempty"`
 JSONPath string `json:"json_path,omitempty"`
 GitPath string `json:"git_path,omitempty"`
 GitRef string `json:"git_ref,omitempty"`
 GitCondition string `json:"git_condition,omitempty"` // change, dirty, clean
 FilePath string `json:"file_path,omitempty"`
 FileCondition string `json:"file_condition,omitempty"`
 ParentID string `json:"parent_id,omitempty"`
 ParentState string `json:"parent_state,omitempty"` // success, failure, any
 ExitCode *int `json:"exit_code,omitempty"`
 OutputContains string `json:"output_contains,omitempty"`
 OutputAbsent string `json:"output_absent,omitempty"`
 Target string `json:"target"` // task, command
 TaskID int `json:"task_id,omitempty"`
 Command string `json:"command,omitempty"`
 WorkingDirectory string `json:"working_directory,omitempty"`
 AllowOverlap bool `json:"allow_overlap"`
 ConditionTTLSeconds int `json:"condition_ttl_seconds,omitempty"`
 RetryMax int `json:"retry_max,omitempty"`
 RetryExitCodes []int `json:"retry_exit_codes,omitempty"` // empty means any nonzero exit
 RetryDelaySeconds int `json:"retry_delay_seconds,omitempty"`
 RetryBackoff float64 `json:"retry_backoff,omitempty"`
 ConcurrencyPolicy string `json:"concurrency_policy,omitempty"` // skip / parallel; queued/replace planned
 ResourceGroup string `json:"resource_group,omitempty"`
 Paused bool `json:"paused,omitempty"`
 PauseReason string `json:"pause_reason,omitempty"`
 ExcludeDates []string `json:"exclude_dates,omitempty"`
 AllowedWeekdays []int `json:"allowed_weekdays,omitempty"`
 WindowStart string `json:"window_start,omitempty"`
 WindowEnd string `json:"window_end,omitempty"`
 RetainDays int `json:"retain_days"`
 RetainRuns int `json:"retain_runs"`
 TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}
type scheduleRun struct {
 ID string `json:"id"`
 ScheduleID string `json:"schedule_id"`
 Name string `json:"name"`
 Reason string `json:"reason"`
 WorkflowID string `json:"workflow_id,omitempty"`
 TriggerRunID string `json:"trigger_run_id,omitempty"`
 Attempts int `json:"attempts,omitempty"`
 Outputs map[string]string `json:"outputs,omitempty"`
 Status string `json:"status"`
 ExitCode int `json:"exit_code"`
 StartedAt time.Time `json:"started_at"`
 FinishedAt *time.Time `json:"finished_at,omitempty"`
 Error string `json:"error,omitempty"`
}
type scheduleStore struct {
 Schedules []scheduleDefinition `json:"schedules"`
 Runs []scheduleRun `json:"runs"`
}
type scheduleEngine struct {
 mu sync.Mutex
 signalMu sync.Mutex
 latched map[string]map[string]bool
 latchAt map[string]map[string]time.Time
 latchFlow map[string]map[string]string
 latchOrigin map[string]map[string]string
 conditionObserved map[string]scheduleConditionObservation
 store scheduleStore
 folder string
 active map[string]func()
 lastCron map[string]string
 nextCheck map[string]time.Time
 edge map[string]string
 lastStatus map[string]string
 started bool
 pollSlots chan struct{}
}
func newScheduleID() string {
 var b [16]byte
 if _,err:=rand.Read(b[:]);err!=nil{return fmt.Sprintf("%d",time.Now().UnixNano())}
 return hex.EncodeToString(b[:])
}
func scheduleDataDirectory(workspace string)(string,error){
 root,err:=os.UserConfigDir()
 if err!=nil{return "",err}
 if resolved,resolveErr:=canonicalExistingPath(workspace);resolveErr==nil{workspace=resolved}
 hash:=sha256.Sum256([]byte(filepath.Clean(workspace)))
 return filepath.Join(root,"taskdeck","schedules",hex.EncodeToString(hash[:12])),nil
}
func (s *Server) scheduleEngine()(*scheduleEngine,error){
 s.scheduleMu.Lock()
 defer s.scheduleMu.Unlock()
 if s.scheduleState!=nil{return s.scheduleState,nil}
 folder,err:=scheduleDataDirectory(s.Workspace)
 if err!=nil{return nil,err}
 if err=os.MkdirAll(filepath.Join(folder,"logs"),0700);err!=nil{return nil,err}
 e:=&scheduleEngine{folder:folder,active:map[string]func(){},lastCron:map[string]string{},nextCheck:map[string]time.Time{},edge:map[string]string{},lastStatus:map[string]string{},pollSlots:make(chan struct{},6),latched:map[string]map[string]bool{},
 latchAt:map[string]map[string]time.Time{},latchFlow:map[string]map[string]string{},
 latchOrigin:map[string]map[string]string{},conditionObserved:map[string]scheduleConditionObservation{}}
 data,err:=os.ReadFile(filepath.Join(folder,"state.json"))
 if err==nil {
  if len(data)>8<<20{return nil,errors.New("schedule state exceeds 8 MiB")}
  if err=json.Unmarshal(data,&e.store);err!=nil{return nil,fmt.Errorf("parse schedule state: %w",err)}
 }else if !os.IsNotExist(err){return nil,err}
 now:=time.Now().UTC()
 for i:=range e.store.Runs {
  if e.store.Runs[i].Status=="running"||e.store.Runs[i].Status=="queued" {
   e.store.Runs[i].Status="interrupted";e.store.Runs[i].ExitCode=-1
   e.store.Runs[i].Error="TaskDeck restarted while this run was active"
   e.store.Runs[i].FinishedAt=&now
  }
 }
 s.scheduleState=e
 return e,nil
}
func (e *scheduleEngine) persistLocked()error{
 encoded,err:=json.MarshalIndent(e.store,"","  ")
 if err!=nil{return err}
 if len(encoded)>8<<20{return errors.New("schedule state exceeds 8 MiB")}
 path:=filepath.Join(e.folder,"state.json")
 tmp,err:=os.CreateTemp(e.folder,".state-*.tmp")
 if err!=nil{return err}
 defer os.Remove(tmp.Name())
 if err=tmp.Chmod(0600);err!=nil{tmp.Close();return err}
 if _,err=tmp.Write(encoded);err!=nil{tmp.Close();return err}
 if err=tmp.Sync();err!=nil{tmp.Close();return err}
 if err=tmp.Close();err!=nil{return err}
 return os.Rename(tmp.Name(),path)
}
func (e *scheduleEngine) definition(id string)(scheduleDefinition,bool){
 for _,item:=range e.store.Schedules{if item.ID==id{return item,true}}
 return scheduleDefinition{},false
}
func (e *scheduleEngine) pruneLocked(now time.Time){
 keep:=make([]scheduleRun,0,len(e.store.Runs))
 count:=map[string]int{}
 sort.SliceStable(e.store.Runs,func(i,j int)bool{return e.store.Runs[i].StartedAt.After(e.store.Runs[j].StartedAt)})
 for _,run:=range e.store.Runs{
  cfg,found:=e.definition(run.ScheduleID)
  if !found{cfg=scheduleDefinition{RetainDays:7,RetainRuns:30}}
  if run.Status=="running"||run.Status=="queued"{keep=append(keep,run);continue}
  count[run.ScheduleID]++
  days,runs:=cfg.RetainDays,cfg.RetainRuns
  if days<1{days=7};if runs<1{runs=30}
  // Keep until BOTH limits have elapsed: later expiry wins.
  old:=now.Sub(run.StartedAt)>time.Duration(days)*24*time.Hour
  beyond:=count[run.ScheduleID]>runs
  if old&&beyond{_ = os.Remove(filepath.Join(e.folder,"logs",run.ID+".log"));continue}
  keep=append(keep,run)
 }
 e.store.Runs=keep
}
func parseCronField(part string,min,max int)(map[int]bool,error){
 values:=map[int]bool{}
 for _,piece:=range strings.Split(part,","){
  step:=1
  if strings.Contains(piece,"/"){
   pair:=strings.Split(piece,"/")
   if len(pair)!=2{return nil,fmt.Errorf("invalid step %q",piece)}
   n,err:=strconv.Atoi(pair[1]);if err!=nil||n<1{return nil,fmt.Errorf("invalid step %q",piece)}
   step=n;piece=pair[0]
  }
  a,b:=min,max
  if piece!="*"{
   if strings.Contains(piece,"-"){
    pair:=strings.Split(piece,"-");if len(pair)!=2{return nil,fmt.Errorf("invalid range %q",piece)}
    var err error;a,err=strconv.Atoi(pair[0]);if err!=nil{return nil,err}
    b,err=strconv.Atoi(pair[1]);if err!=nil{return nil,err}
   }else{
    v,err:=strconv.Atoi(piece);if err!=nil{return nil,err}
    a=v;b=v;if strings.Contains(part,"/"){b=max}
   }
  }
  if a<min||b>max||a>b{return nil,fmt.Errorf("cron field %q outside [%d,%d]",part,min,max)}
  for v:=a;v<=b;v+=step{values[v]=true}
 }
 return values,nil
}
func cronMatches(spec string,t time.Time)(bool,error){
 fields:=strings.Fields(spec)
 if len(fields)!=5{return false,errors.New("cron requires exactly 5 fields: minute hour day month weekday")}
 bounds:=[5][2]int{{0,59},{0,23},{1,31},{1,12},{0,7}}
 actual:=[5]int{t.Minute(),t.Hour(),t.Day(),int(t.Month()),int(t.Weekday())}
 selected:=[5]map[int]bool{}
 for i,part:=range fields{
  values,err:=parseCronField(part,bounds[i][0],bounds[i][1]);if err!=nil{return false,err}
  selected[i]=values
 }
 weekday:=selected[4][actual[4]]||(actual[4]==0&&selected[4][7])
 // Traditional Vixie cron semantics: if both DOM and DOW constrained, OR.
 day:=selected[2][actual[2]]&&weekday
 if fields[2]=="*"{day=weekday}
 if fields[4]=="*"{day=selected[2][actual[2]]}
 return selected[0][actual[0]]&&selected[1][actual[1]]&&day&&selected[3][actual[3]],nil
}
func (s *Server) validateSchedule(def scheduleDefinition,store scheduleStore)error{
 def.Name=strings.TrimSpace(def.Name)
 if def.Name==""||len(def.Name)>120{return errors.New("name required (max 120 characters)")}
 if def.Target!="task"&&def.Target!="command"{return errors.New("target must be task or command")}
 if def.Target=="task"&&def.TaskID<=0{return errors.New("task_id required")}
 if def.Target=="task"{
  selected,workflow,root,err:=s.workspaceWorkflowByTaskID(def.TaskID)
  if err!=nil{return fmt.Errorf("tasks.json task not found: %w",err)}
  if marker,_:=selected.Raw["taskdeckApproval"].(string);strings.EqualFold(marker,"deploy"){return errors.New("unattended deploy task requires interactive approval")}
  refs,err:=taskWorkflowSecretRefs(workflow,selected.Label)
  if err!=nil{return err}
  if len(refs)>0{return errors.New("unattended task cannot use interactive secret grants")}
  if _,err=tasks.ResolveWorkflowExecution(workflow,selected.Label,root.Path,nil);err!=nil{return fmt.Errorf("schedule task resolution: %w",err)}
 }
 if def.Target=="command"&&(strings.TrimSpace(def.Command)==""||len(def.Command)>16384){return errors.New("command required (max 16 KiB)")}
 if def.WorkingDirectory!=""{
  root:=filepath.Join(s.Workspace,def.WorkingDirectory)
  real,err:=canonicalExistingPath(root)
  workspaceRoot,workspaceErr:=canonicalExistingPath(s.Workspace)
  if err!=nil||workspaceErr!=nil||!pathInside(workspaceRoot,real){return errors.New("working_directory must be an existing path inside workspace")}
  info,err:=os.Stat(real);if err!=nil||!info.IsDir(){return errors.New("working_directory must be a directory")}
 }
 if def.RetainDays<1||def.RetainDays>3650||def.RetainRuns<1||def.RetainRuns>10000{return errors.New("retention limits must be days 1-3650 and runs 1-10000")}
 if def.TimeoutSeconds<0||def.TimeoutSeconds>86400{return errors.New("timeout_seconds must be 0-86400")}
 if err:=validateScheduleCalendar(def);err!=nil{return err}
 if def.ConditionTTLSeconds<0||def.ConditionTTLSeconds>604800{return errors.New("condition_ttl_seconds must be 0-604800")}
 if def.RetryMax<0||def.RetryMax>10{return errors.New("retry_max must be 0-10")}
 if len(def.RetryExitCodes)>64{return errors.New("retry_exit_codes cannot exceed 64 entries")}
 for _,code:=range def.RetryExitCodes{if code < -255||code > 255||code==0{return errors.New("retry_exit_codes must contain nonzero exit codes from -255 to 255")}}
 if def.RetryDelaySeconds<0||def.RetryDelaySeconds>86400{return errors.New("retry_delay_seconds must be 0-86400")}
 if def.RetryBackoff!=0&&(def.RetryBackoff<1||def.RetryBackoff>10){return errors.New("retry_backoff must be 1-10")}
 if def.ConcurrencyPolicy!=""&&def.ConcurrencyPolicy!="skip"&&def.ConcurrencyPolicy!="parallel"&&def.ConcurrencyPolicy!="queue"&&def.ConcurrencyPolicy!="replace"{return errors.New("concurrency_policy must be skip, parallel, queue or replace")}
 if len(def.ResourceGroup)>120||strings.ContainsAny(def.ResourceGroup,"\r\n"){return errors.New("resource_group must be 120 characters or fewer, without newlines")}
 if len(def.PauseReason)>500{return errors.New("pause_reason must be 500 characters or fewer")}
 if def.ConditionGroup!=nil{return s.validateScheduleGroup(def,store)}
 switch def.Type{
 case "cron":
  loc:=time.Local
  if def.Timezone!=""{var err error;loc,err=time.LoadLocation(def.Timezone);if err!=nil{return fmt.Errorf("invalid timezone: %w",err)}}
  _,err:=cronMatches(def.Cron,time.Now().In(loc));return err
 case "url":
  if def.IntervalSeconds<30||def.IntervalSeconds>86400{return errors.New("URL interval_seconds must be 30-86400")}
  if def.MatchText==""||len(def.MatchText)>2048{return errors.New("URL match_text required (max 2048)")}
  if def.MatchMode!="contains"&&def.MatchMode!="equals"&&def.MatchMode!="json"&&def.MatchMode!="xml"{return errors.New("URL match_mode must be contains, equals, json, or xml")}
  return validateScheduleURL(def.URL)
 case "git":
  if def.IntervalSeconds<30||def.IntervalSeconds>86400{return errors.New("Git interval_seconds must be 30-86400")}
  if def.GitPath==""{return errors.New("Git repository directory required")}
  if def.GitCondition!="change"&&def.GitCondition!="dirty"&&def.GitCondition!="clean"{return errors.New("Git condition must be change, dirty, or clean")}
  requested,err:=canonicalExistingPath(filepath.Join(s.Workspace,def.GitPath))
  if err!=nil{return err}
  root,err:=verifyGitRepository(s.Workspace,requested)
  if err!=nil{return err}
  if filepath.Clean(root)!=requested{return errors.New("choose the actual Git repository root, not a nested subdirectory")}
  return nil
 case "file":
  if def.IntervalSeconds<30||def.IntervalSeconds>86400{return errors.New("file polling interval must be 30-86400 seconds")}
  return s.validateScheduleFile(def.FilePath,def.FileCondition)
 case "dependency":
  if def.ParentID==""||def.ParentID==def.ID{return errors.New("valid parent schedule required")}
  if def.ParentState!="any"&&def.ParentState!="success"&&def.ParentState!="failure"{return errors.New("parent_state must be any, success, or failure")}
  parentFound:=false
  for _,item:=range store.Schedules{if item.ID==def.ParentID{parentFound=true;break}}
  if !parentFound{return errors.New("parent schedule not found")}
  return validateScheduleDependencyGraph(def,store)
 default:return errors.New("unsupported schedule type")
 }
}
