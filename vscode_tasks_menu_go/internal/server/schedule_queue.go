package server

import(
 "context"
 "errors"
 "time"
)

// scheduleResourceBusyLocked checks the explicit per-workspace resource lock.
// The caller must hold e.mu.
func (e *scheduleEngine) scheduleResourceBusyLocked(config scheduleDefinition)bool{
 if config.ResourceGroup==""{return false}
 for _,run:=range e.store.Runs{
  if run.Status!="running"||run.ScheduleID==config.ID{continue}
  other,ok:=e.definition(run.ScheduleID)
  if ok&&other.ResourceGroup==config.ResourceGroup{return true}
 }
 return false
}
func (e *scheduleEngine) scheduleQueueLocked(config scheduleDefinition,run scheduleRun)error{
 count:=0;global:=0
 for _,item:=range e.store.Runs{
  if item.Status=="queued"{global++;if item.ScheduleID==config.ID{count++}}
 }
 if count>=10||global>=100{return errors.New("schedule queue limit reached (10 per schedule, 100 global)")}
 run.Status="queued"
 run.Attempts=0
 e.store.Runs=append(e.store.Runs,run)
 if err:=e.persistLocked();err!=nil{
  e.store.Runs=e.store.Runs[:len(e.store.Runs)-1]
  return err
 }
 return nil
}
// Dequeue in insertion order and start only when concurrency/resource checks
// pass. Called after finishes and explicit Resume. No missed queued runs are
// silently executed after a process restart: boot marks them interrupted.
func (s *Server) scheduleDequeue(){
 e,err:=s.scheduleEngine();if err!=nil{return}
 for{
  e.mu.Lock()
  if len(e.active)>=12{e.mu.Unlock();return}
  picked:=-1
  var cfg scheduleDefinition
  for i,run:=range e.store.Runs{
   if run.Status!="queued"{continue}
   def,exists:=e.definition(run.ScheduleID)
   if !exists||!def.Enabled||def.Paused||!scheduleCalendarAllows(def,time.Now()){continue}
   active:=0
   for _,item:=range e.store.Runs{if item.Status=="running"&&item.ScheduleID==run.ScheduleID{active++}}
   if active>=4||active>0&&def.ConcurrencyPolicy!="parallel"{continue}
   if e.scheduleResourceBusyLocked(def){continue}
   picked=i;cfg=def;break
  }
  if picked<0{e.mu.Unlock();return}
  run:=e.store.Runs[picked]
  e.store.Runs[picked].Status="running"
  e.store.Runs[picked].StartedAt=time.Now().UTC()
  e.store.Runs[picked].Attempts=1
  run=e.store.Runs[picked]
  if err=e.persistLocked();err!=nil{
   e.store.Runs[picked].Status="queued"
   e.mu.Unlock();return
  }
  timeout:=cfg.TimeoutSeconds;if timeout<=0{timeout=1800}
  ctx,cancel:=context.WithTimeout(context.Background(),time.Duration(timeout)*time.Second)
  e.active[run.ID]=cancel
  e.mu.Unlock()
  go s.scheduleExecute(ctx,run,cfg)
 }
}
