package server

import (
 "encoding/json"
 "errors"
 "net/http"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "time"
)

type scheduleActionRequest struct {
 Action string `json:"action"`
 Schedule scheduleDefinition `json:"schedule"`
 Schedules []scheduleDefinition `json:"schedules,omitempty"`
 ID string `json:"id"`
 RunID string `json:"run_id"`
 PauseReason string `json:"pause_reason,omitempty"`
 Matches map[string]bool `json:"matches,omitempty"`
}
func (s *Server) schedulesAPI(w http.ResponseWriter,r *http.Request){
 e,err:=s.scheduleEngine();if err!=nil{http.Error(w,err.Error(),500);return}
 switch r.Method{
 case http.MethodGet:
  e.mu.Lock()
  now:=time.Now().UTC()
  before:=len(e.store.Runs)
  e.pruneLocked(now)
  if len(e.store.Runs)!=before { _=e.persistLocked() }
  snapshot:=scheduleStore{Schedules:append([]scheduleDefinition(nil),e.store.Schedules...),Runs:append([]scheduleRun(nil),e.store.Runs...)}
  errorsCopy:=make(map[string]string,len(e.lastStatus));for k,v:=range e.lastStatus{errorsCopy[k]=v}
  inspected:=map[string]scheduleInspection{}
  for _,def:=range snapshot.Schedules{inspected[def.ID]=e.inspectScheduleLocked(def,now)}
  e.mu.Unlock()
  sort.Slice(snapshot.Schedules,func(i,j int)bool{return strings.ToLower(snapshot.Schedules[i].Name)<strings.ToLower(snapshot.Schedules[j].Name)})
  sort.Slice(snapshot.Runs,func(i,j int)bool{return snapshot.Runs[i].StartedAt.After(snapshot.Runs[j].StartedAt)})
  if snapshot.Schedules==nil{snapshot.Schedules=[]scheduleDefinition{}}
  if snapshot.Runs==nil{snapshot.Runs=[]scheduleRun{}}
  writeJSON(w,200,map[string]any{"schedules":snapshot.Schedules,"runs":snapshot.Runs,"trigger_errors":errorsCopy,
   "condition_inspector":inspected,"statistics":scheduleRunStatistics(snapshot.Runs)})
 case http.MethodPost:
  var req scheduleActionRequest
  dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,128<<10));dec.DisallowUnknownFields()
  if err:=dec.Decode(&req);err!=nil{http.Error(w,"invalid schedule request: "+err.Error(),400);return}
  switch req.Action{
  case "import":
   if len(req.Schedules)==0||len(req.Schedules)>100{http.Error(w,"import requires 1-100 schedules",400);return}
   oldIDs:=map[string]bool{}
   for _,item:=range req.Schedules{
    if item.ID==""||oldIDs[item.ID]{http.Error(w,"import requires distinct original schedule IDs",400);return}
    oldIDs[item.ID]=true
   }
   imported:=schedulePrepareImport(req.Schedules)
   e.mu.Lock()
   next:=append([]scheduleDefinition(nil),e.store.Schedules...)
   if len(next)+len(imported)>500{e.mu.Unlock();http.Error(w,"schedule limit (500)",409);return}
   next=append(next,imported...)
   var validationErr error
   for _,item:=range imported{
    if err:=s.validateSchedule(item,scheduleStore{Schedules:next});err!=nil{
     validationErr=errors.New("import "+item.Name+": "+err.Error());break
    }
   }
   if validationErr!=nil{e.mu.Unlock();http.Error(w,validationErr.Error(),400);return}
   previous:=e.store.Schedules
   e.store.Schedules=next
   err=e.persistLocked()
   if err!=nil{e.store.Schedules=previous}
   e.mu.Unlock()
   if err!=nil{http.Error(w,err.Error(),500);return}
   writeJSON(w,201,map[string]any{"imported":len(imported),"schedules":imported})
  case "upsert":
   def:=req.Schedule
   def.ID=strings.TrimSpace(def.ID)
   if def.ID==""{def.ID=newScheduleID()}
   if def.Timezone==""{def.Timezone="Local"}
   if def.RetainDays==0{def.RetainDays=7}
   if def.RetainRuns==0{def.RetainRuns=30}
   if def.Type=="dependency"&&def.ParentState==""{def.ParentState="success"}
   if def.ConditionGroup!=nil{
    // IDs are supplied by the server, so stable signal baselines survive
    // edits but no empty IDs or duplicate condition IDs are stored.
    seen:=map[string]bool{}
    var assign func(*scheduleConditionGroup)
    assign=func(group *scheduleConditionGroup){
     if group.Operator==""{group.Operator="and"}
     for i:=range group.Conditions{
      id:=group.Conditions[i].ID
      if id==""||seen[id]{id=newScheduleID();group.Conditions[i].ID=id}
      seen[id]=true
      if group.Conditions[i].Type=="dependency"&&group.Conditions[i].ParentState==""{group.Conditions[i].ParentState="success"}
     }
     for i:=range group.Groups{assign(&group.Groups[i])}
    }
    assign(def.ConditionGroup)
    def.Type="multi"
   }
   e.mu.Lock()
   found:=-1
   for i,c:=range e.store.Schedules{if c.ID==def.ID{found=i;break}}
   next:=append([]scheduleDefinition(nil),e.store.Schedules...)
   if found>=0{next[found]=def}else{if len(next)>=500{e.mu.Unlock();http.Error(w,"schedule limit (500)",409);return};next=append(next,def)}
   // Never allow an arbitrary caller-supplied ID to overwrite a non-existing schedule.
   if found<0&&req.Schedule.ID!=""{e.mu.Unlock();http.Error(w,"new schedules must not supply id",400);return}
   err=s.validateSchedule(def,scheduleStore{Schedules:next})
   if err==nil{
    // Persist first; on write failure neither config nor trigger runtime is changed.
    oldStore:=e.store
    e.store.Schedules=next
    err=e.persistLocked()
    if err!=nil{e.store=oldStore}else{
     delete(e.latched,def.ID);delete(e.latchAt,def.ID)
     delete(e.latchFlow,def.ID);delete(e.latchOrigin,def.ID)
     for key:=range e.edge{if strings.HasPrefix(key,def.ID+":"){delete(e.edge,key)}}
     for key:=range e.nextCheck{if strings.HasPrefix(key,def.ID+":"){delete(e.nextCheck,key)}}
     for key:=range e.lastCron{if strings.HasPrefix(key,def.ID+":"){delete(e.lastCron,key)}}
    }
   }
   e.mu.Unlock()
   if err!=nil{http.Error(w,err.Error(),400);return}
   writeJSON(w,200,map[string]any{"schedule":def})
  case "delete":
   e.mu.Lock()
   idx:=-1
   for i,c:=range e.store.Schedules{if c.ID==req.ID{idx=i;break}}
   if idx<0{e.mu.Unlock();http.Error(w,"schedule not found",404);return}
   for _,run:=range e.store.Runs{if run.ScheduleID==req.ID&&(run.Status=="running"||run.Status=="queued"){e.mu.Unlock();http.Error(w,"schedule is running",409);return}}
   for _,c:=range e.store.Schedules{
    for _,parent:=range scheduleParentReferences(c){
     if parent==req.ID{e.mu.Unlock();http.Error(w,"schedule is referenced by a dependency",409);return}
    }
   }
   e.store.Schedules=append(e.store.Schedules[:idx],e.store.Schedules[idx+1:]...)
   delete(e.latched,req.ID);delete(e.latchAt,req.ID)
   delete(e.latchFlow,req.ID);delete(e.latchOrigin,req.ID)
   for key:=range e.edge{if strings.HasPrefix(key,req.ID+":"){delete(e.edge,key)}}
   for key:=range e.nextCheck{if strings.HasPrefix(key,req.ID+":"){delete(e.nextCheck,key)}}
   for key:=range e.lastCron{if strings.HasPrefix(key,req.ID+":"){delete(e.lastCron,key)}}
   err=e.persistLocked();e.mu.Unlock()
   if err!=nil{http.Error(w,err.Error(),500);return}
   writeJSON(w,200,map[string]bool{"ok":true})
  case "inspect","simulate":
   e.mu.Lock()
   def,found:=e.definition(req.ID)
   if !found{e.mu.Unlock();http.Error(w,"schedule not found",404);return}
   inspection:=e.inspectScheduleLocked(def,time.Now().UTC())
   e.mu.Unlock()
   if req.Action=="simulate"{
    // Simulation uses supplied synthetic condition matches ONLY. It cannot
    // fetch URLs, run commands, mutate latches or trigger descendants.
    allowed:=map[string]bool{}
    for _,c:=range scheduleLeaves(scheduleConditionsFor(def)){allowed[c.ID]=req.Matches[c.ID]}
    inspection.Ready=scheduleGroupSatisfied(scheduleConditionsFor(def),allowed)
   }
   writeJSON(w,200,map[string]any{"inspection":inspection,"next_cron":scheduleNextTimes(def,time.Now().UTC()),"simulation":req.Action=="simulate"})
  case "pause","resume":
   e.mu.Lock()
   idx:=-1
   for i:=range e.store.Schedules{if e.store.Schedules[i].ID==req.ID{idx=i;break}}
   if idx<0{e.mu.Unlock();http.Error(w,"schedule not found",404);return}
   old:=e.store.Schedules[idx]
   e.store.Schedules[idx].Paused=req.Action=="pause"
   if req.Action=="pause"{e.store.Schedules[idx].PauseReason=req.PauseReason}else{e.store.Schedules[idx].PauseReason=""}
   err=e.persistLocked()
   if err!=nil{e.store.Schedules[idx]=old}else{
    delete(e.latched,req.ID);delete(e.latchAt,req.ID);delete(e.latchFlow,req.ID);delete(e.latchOrigin,req.ID)
   }
   e.mu.Unlock()
   if err!=nil{http.Error(w,err.Error(),500);return}
   if req.Action=="resume"{s.scheduleDequeue()}
   writeJSON(w,200,map[string]bool{"ok":true})
  case "run":
   id,err:=s.scheduleStart(req.ID,"manual")
   if err!=nil{http.Error(w,err.Error(),409);return}
   writeJSON(w,201,map[string]string{"run_id":id})
  case "stop":
   if err:=s.scheduleCancel(req.RunID);err!=nil{http.Error(w,err.Error(),409);return}
   writeJSON(w,200,map[string]bool{"ok":true})
  default:http.Error(w,"unknown schedule action",400)
  }
 default:http.Error(w,"method not allowed",405)
 }
}
func (s *Server) scheduleLogAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet{http.Error(w,"method not allowed",405);return}
 e,err:=s.scheduleEngine();if err!=nil{http.Error(w,err.Error(),500);return}
 id:=r.URL.Query().Get("run_id")
 e.mu.Lock()
 found:=false
 for _,run:=range e.store.Runs{if run.ID==id{found=true;break}}
 e.mu.Unlock()
 if !found{http.Error(w,"run not found",404);return}
 path:=filepath.Join(e.folder,"logs",id+".log")
 data,err:=os.ReadFile(path)
 if err!=nil{
  if errors.Is(err,os.ErrNotExist){data=[]byte("No console output yet.")} else {http.Error(w,err.Error(),500);return}
 }
 if len(data)>scheduleLogLimit+128{data=data[:scheduleLogLimit+128]}
 w.Header().Set("Content-Type","text/plain; charset=utf-8")
 w.Header().Set("Cache-Control","no-store")
 _,_=w.Write(data)
}

// scheduleRepositoriesAPI uses the same discovery as git-repo-select, but only
// suggests repositories rooted in the primary workspace. The schedule executor
// cannot resolve attached-root virtual paths, so do not suggest invalid paths.
func (s *Server) scheduleRepositoriesAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet{http.Error(w,"method not allowed",405);return}
 if r.URL.Query().Get("refresh")!=""&&r.URL.Query().Get("refresh")!="1"{
  http.Error(w,"refresh must be 1",400);return
 }
 repos,settings,err:=s.discoverGitRepositories(r.URL.Query().Get("refresh")=="1")
 if err!=nil{http.Error(w,err.Error(),500);return}
 root,err:=canonicalExistingPath(s.Workspace)
 if err!=nil{http.Error(w,"workspace root unavailable",500);return}
 choices:=make([]gitRepository,0,len(repos))
 for _,repo:=range repos{
  if !pathInside(root,repo.Root){continue}
  // Repository IDs match the Git bar and primary workspace relative paths.
  choices=append(choices,repo)
 }
 preferred:=settings.DefaultRepository
 found:=false
 for _,repo:=range choices{if repo.ID==preferred{found=true;break}}
 if !found{preferred="";if len(choices)>0{preferred=choices[0].ID}}
 writeJSON(w,200,map[string]any{"repositories":choices,"default_repository":preferred})
}
