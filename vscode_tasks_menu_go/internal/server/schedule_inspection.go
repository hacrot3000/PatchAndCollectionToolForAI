package server

import (
 "strings"
 "time"
)

// scheduleConditionObservation is intentionally ephemeral: it describes
// recent probes, not a durable promise that a remote condition remains true.
type scheduleConditionObservation struct {
 Matched bool `json:"matched"`
 CheckedAt time.Time `json:"checked_at"`
 NextCheck time.Time `json:"next_check,omitempty"`
 Error string `json:"error,omitempty"`
}

type scheduleConditionStatus struct {
 ID string `json:"id"`
 Type string `json:"type"`
 Matched bool `json:"matched"`
 Latched bool `json:"latched"`
 ExpiresAt *time.Time `json:"expires_at,omitempty"`
 CheckedAt *time.Time `json:"checked_at,omitempty"`
 NextCheck *time.Time `json:"next_check,omitempty"`
 Error string `json:"error,omitempty"`
 WorkflowID string `json:"workflow_id,omitempty"`
}
type scheduleInspection struct {
 ID string `json:"id"`
 Ready bool `json:"ready"`
 Paused bool `json:"paused"`
 Conditions []scheduleConditionStatus `json:"conditions"`
}

func scheduleActiveSignals(def scheduleDefinition,latched map[string]bool,at map[string]time.Time,now time.Time)map[string]bool{
 valid:=map[string]bool{}
 for id,on:=range latched{
  if !on{continue}
  stamp:=at[id]
  if def.ConditionTTLSeconds>0&&(stamp.IsZero()||now.Sub(stamp)>time.Duration(def.ConditionTTLSeconds)*time.Second){continue}
  valid[id]=true
 }
 return valid
}

// For AND branches parent-completion events must belong to a common workflow.
// Unrelated Cron/Git/URL signals have no workflow ID and may be combined
// with that workflow within the configured TTL.
func scheduleEvaluateWithWorkflow(g scheduleConditionGroup,met map[string]bool,flows map[string]string,origins map[string]string)(bool,string,string){
 if g.Operator!="and"&&g.Operator!="or"{return false,"",""}
 type match struct{flow,origin string}
 available:=[]match{}
 for _,c:=range g.Conditions{
  if met[c.ID]{available=append(available,match{flows[c.ID],origins[c.ID]})}else if g.Operator=="and"{return false,"",""}
 }
 for _,child:=range g.Groups{
  ok,flow,origin:=scheduleEvaluateWithWorkflow(child,met,flows,origins)
  if ok{available=append(available,match{flow,origin})}else if g.Operator=="and"{return false,"",""}
 }
 if g.Operator=="or"{
  if len(available)==0{return false,"",""}
  return true,available[0].flow,available[0].origin
 }
 if len(g.Conditions)+len(g.Groups)==0{return false,"",""}
 flow,origin:="",""
 for _,item:=range available{
  if item.flow!=""{
   if flow!=""&&item.flow!=flow{return false,"",""}
   flow=item.flow
   origin=item.origin
  }
 }
 return len(available)==len(g.Conditions)+len(g.Groups),flow,origin
}

func (e *scheduleEngine) inspectScheduleLocked(def scheduleDefinition,now time.Time)scheduleInspection{
 states:=scheduleActiveSignals(def,e.latched[def.ID],e.latchAt[def.ID],now)
 ready,_,_:=scheduleEvaluateWithWorkflow(scheduleConditionsFor(def),states,e.latchFlow[def.ID],e.latchOrigin[def.ID])
 result:=scheduleInspection{ID:def.ID,Ready:ready&&def.Enabled&&!def.Paused,Paused:def.Paused,Conditions:[]scheduleConditionStatus{}}
 for _,cond:=range scheduleLeaves(scheduleConditionsFor(def)){
  key:=def.ID+":"+cond.ID
  value:=scheduleConditionStatus{ID:cond.ID,Type:cond.Type,Latched:states[cond.ID],
   Error:e.lastStatus[key],WorkflowID:e.latchFlow[def.ID][cond.ID]}
  observed:=e.conditionObserved[key]
  value.Matched=observed.Matched
  if !observed.CheckedAt.IsZero(){t:=observed.CheckedAt;value.CheckedAt=&t}
  if !observed.NextCheck.IsZero(){t:=observed.NextCheck;value.NextCheck=&t}
  if observed.Error!=""{value.Error=observed.Error}
  if def.ConditionTTLSeconds>0&&states[cond.ID]{
   t:=e.latchAt[def.ID][cond.ID].Add(time.Duration(def.ConditionTTLSeconds)*time.Second)
   value.ExpiresAt=&t
  }
  result.Conditions=append(result.Conditions,value)
 }
 return result
}

// Cron prediction is display-only; it does not mutate the execution baseline.
// A bound avoids expensive searches for impossible expressions.
func scheduleNextCronTimes(spec,timezone string,now time.Time,maxResults int)[]time.Time{
 if maxResults<=0||maxResults>10{maxResults=5}
 loc:=time.Local
 if timezone!=""&&timezone!="Local"{
  var err error;loc,err=time.LoadLocation(timezone);if err!=nil{return nil}
 }
 rounded:=now.In(loc).Truncate(time.Minute).Add(time.Minute)
 matches:=[]time.Time{}
 for i:=0;i<366*24*60&&len(matches)<maxResults;i++{
  at:=rounded.Add(time.Duration(i)*time.Minute)
  ok,err:=cronMatches(spec,at)
  if err!=nil{return nil}
  if ok{matches=append(matches,at.UTC())}
 }
 return matches
}
func scheduleNextTimes(def scheduleDefinition,now time.Time)[]time.Time{
 dates:=[]time.Time{}
 for _,cond:=range scheduleLeaves(scheduleConditionsFor(def)){
  if cond.Type!="cron"{continue}
  dates=append(dates,scheduleNextCronTimes(cond.Cron,cond.Timezone,now,5)...)
 }
 // De-duplicate, sort chronologically, and keep at most ten.
 unique:=map[time.Time]bool{}
 result:=[]time.Time{}
 for _,date:=range dates{if !unique[date]{unique[date]=true;result=append(result,date)}}
 for i:=0;i<len(result);i++{for j:=i+1;j<len(result);j++{
  if result[j].Before(result[i]){result[i],result[j]=result[j],result[i]}
 }}
 if len(result)>10{result=result[:10]}
 return result
}
type scheduleStatistics struct{
 ScheduleID string `json:"schedule_id"`
 Completed int `json:"completed"`
 Success int `json:"success"`
 Failure int `json:"failure"`
 AverageDurationSeconds float64 `json:"average_duration_seconds"`
 LastError string `json:"last_error,omitempty"`
}
func scheduleRunStatistics(runs []scheduleRun)[]scheduleStatistics{
 byID:=map[string]*scheduleStatistics{}
 for _,run:=range runs{
  v:=byID[run.ScheduleID]
  if v==nil{v=&scheduleStatistics{ScheduleID:run.ScheduleID};byID[run.ScheduleID]=v}
  if run.FinishedAt==nil{continue}
  v.Completed++
  if run.Status=="success"{v.Success++}
  if run.Status=="failure"{v.Failure++;if v.LastError==""{v.LastError=run.Error}}
  v.AverageDurationSeconds+=run.FinishedAt.Sub(run.StartedAt).Seconds()
 }
 result:=[]scheduleStatistics{}
 for _,v:=range byID{
  if v.Completed>0{v.AverageDurationSeconds/=float64(v.Completed)}
  v.LastError=strings.TrimSpace(v.LastError)
  result=append(result,*v)
 }
 return result
}
