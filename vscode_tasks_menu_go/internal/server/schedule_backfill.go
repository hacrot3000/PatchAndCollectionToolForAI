package server

import(
 "errors"
 "fmt"
 "time"
)

// Only single Cron schedules may be backfilled. A composite AND/OR condition
// has no sound historical replay semantics without snapshots of the other
// conditions, and must not be replayed as if those conditions had occurred.
func scheduleBackfillTimes(def scheduleDefinition,from,to time.Time)([]time.Time,error){
 if !to.After(from)||to.Sub(from)>7*24*time.Hour{return nil,errors.New("backfill interval must be between 1 minute and 7 days")}
 conditions:=scheduleLeaves(scheduleConditionsFor(def))
 if len(conditions)!=1||conditions[0].Type!="cron"{return nil,errors.New("backfill supports only a single Cron condition")}
 cond:=conditions[0]
 loc:=time.Local
 if cond.Timezone!=""&&cond.Timezone!="Local"{
  var err error;loc,err=time.LoadLocation(cond.Timezone);if err!=nil{return nil,err}
 }
 ticks:=[]time.Time{}
 for t:=from.Truncate(time.Minute);t.Before(to);t=t.Add(time.Minute){
  if t.Before(from){continue}
  ok,err:=cronMatches(cond.Cron,t.In(loc));if err!=nil{return nil,err}
  if ok{
   ticks=append(ticks,t.UTC())
   if len(ticks)>10{return nil,errors.New("backfill matches exceed limit of 10; select a narrower time range")}
  }
 }
 if len(ticks)==0{return nil,errors.New("no Cron occurrences in selected window")}
 return ticks,nil
}
func (s *Server) scheduleBackfill(id string,from,to time.Time)([]string,error){
 e,err:=s.scheduleEngine();if err!=nil{return nil,err}
 e.mu.Lock()
 config,found:=e.definition(id)
 e.mu.Unlock()
 if !found{return nil,errors.New("schedule not found")}
 ticks,err:=scheduleBackfillTimes(config,from,to);if err!=nil{return nil,err}
 accepted:=[]string{}
 for _,tick:=range ticks{
  reason:="manual backfill "+tick.Format(time.RFC3339)
  e.mu.Lock()
  // Deduplicate accepted backfill occurrences for this schedule.
  exists:=false
  for _,run:=range e.store.Runs{if run.ScheduleID==id&&run.Reason==reason{exists=true;break}}
  if exists{e.mu.Unlock();continue}
  fresh,ok:=e.definition(id)
  if !ok{e.mu.Unlock();return accepted,errors.New("schedule deleted during backfill")}
  run:=scheduleRun{ID:newScheduleID(),ScheduleID:id,Name:fresh.Name,Reason:reason,
   WorkflowID:newScheduleID(),Status:"queued",StartedAt:time.Now().UTC()}
  err=e.scheduleQueueLocked(fresh,run)
  e.mu.Unlock()
  if err!=nil{
   return accepted,fmt.Errorf("%d of %d backfill occurrences queued: %w",len(accepted),len(ticks),err)
  }
  accepted=append(accepted,run.ID)
 }
 s.scheduleDequeue()
 return accepted,nil
}
