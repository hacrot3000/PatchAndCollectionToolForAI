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
 ID string `json:"id"`
 RunID string `json:"run_id"`
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
  e.mu.Unlock()
  sort.Slice(snapshot.Schedules,func(i,j int)bool{return strings.ToLower(snapshot.Schedules[i].Name)<strings.ToLower(snapshot.Schedules[j].Name)})
  sort.Slice(snapshot.Runs,func(i,j int)bool{return snapshot.Runs[i].StartedAt.After(snapshot.Runs[j].StartedAt)})
  if snapshot.Schedules==nil{snapshot.Schedules=[]scheduleDefinition{}}
  if snapshot.Runs==nil{snapshot.Runs=[]scheduleRun{}}
  writeJSON(w,200,map[string]any{"schedules":snapshot.Schedules,"runs":snapshot.Runs,"trigger_errors":errorsCopy})
 case http.MethodPost:
  var req scheduleActionRequest
  dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,128<<10));dec.DisallowUnknownFields()
  if err:=dec.Decode(&req);err!=nil{http.Error(w,"invalid schedule request: "+err.Error(),400);return}
  switch req.Action{
  case "upsert":
   def:=req.Schedule
   def.ID=strings.TrimSpace(def.ID)
   if def.ID==""{def.ID=newScheduleID()}
   if def.Timezone==""{def.Timezone="Local"}
   if def.RetainDays==0{def.RetainDays=7}
   if def.RetainRuns==0{def.RetainRuns=30}
   if def.Type=="dependency"&&def.ParentState==""{def.ParentState="success"}
   e.mu.Lock()
   found:=-1
   for i,c:=range e.store.Schedules{if c.ID==def.ID{found=i;break}}
   next:=append([]scheduleDefinition(nil),e.store.Schedules...)
   if found>=0{next[found]=def}else{if len(next)>=500{e.mu.Unlock();http.Error(w,"schedule limit (500)",409);return};next=append(next,def)}
   // Never allow an arbitrary caller-supplied ID to overwrite a non-existing schedule.
   if found<0&&req.Schedule.ID!=""{e.mu.Unlock();http.Error(w,"new schedules must not supply id",400);return}
   err=s.validateSchedule(def,scheduleStore{Schedules:next})
   if err==nil{e.store.Schedules=next;delete(e.edge,def.ID);delete(e.nextCheck,def.ID);delete(e.lastCron,def.ID);err=e.persistLocked()}
   e.mu.Unlock()
   if err!=nil{http.Error(w,err.Error(),400);return}
   writeJSON(w,200,map[string]any{"schedule":def})
  case "delete":
   e.mu.Lock()
   idx:=-1
   for i,c:=range e.store.Schedules{if c.ID==req.ID{idx=i;break}}
   if idx<0{e.mu.Unlock();http.Error(w,"schedule not found",404);return}
   for _,run:=range e.store.Runs{if run.ScheduleID==req.ID&&run.Status=="running"{e.mu.Unlock();http.Error(w,"schedule is running",409);return}}
   for _,c:=range e.store.Schedules{if c.ParentID==req.ID{e.mu.Unlock();http.Error(w,"schedule is referenced by a dependency",409);return}}
   e.store.Schedules=append(e.store.Schedules[:idx],e.store.Schedules[idx+1:]...)
   delete(e.edge,req.ID);delete(e.nextCheck,req.ID);delete(e.lastCron,req.ID)
   err=e.persistLocked();e.mu.Unlock()
   if err!=nil{http.Error(w,err.Error(),500);return}
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
  if errors.Is(err,os.ErrNotExist){data=[]byte("No console output yet.")}
  else{http.Error(w,err.Error(),500);return}
 }
 if len(data)>scheduleLogLimit+128{data=data[:scheduleLogLimit+128]}
 w.Header().Set("Content-Type","text/plain; charset=utf-8")
 w.Header().Set("Cache-Control","no-store")
 _,_=w.Write(data)
}

func (s *Server) scheduleRepositoriesAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet{http.Error(w,"method not allowed",405);return}
 repos,_,err:=s.discoverGitRepositories(false)
 if err!=nil{http.Error(w,err.Error(),500);return}
 writeJSON(w,200,map[string]any{"repositories":repos})
}
