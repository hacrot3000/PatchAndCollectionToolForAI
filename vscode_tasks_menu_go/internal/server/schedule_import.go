package server

import "strings"

func scheduleRemapParents(def *scheduleDefinition,ids map[string]string){
 if parent,ok:=ids[def.ParentID];ok{def.ParentID=parent}
 if def.ConditionGroup==nil{return}
 var visit func(*scheduleConditionGroup)
 visit=func(group *scheduleConditionGroup){
  for i:=range group.Conditions{
   if group.Conditions[i].Type!="dependency"{continue}
   if newID,ok:=ids[group.Conditions[i].ParentID];ok{group.Conditions[i].ParentID=newID}
  }
  for i:=range group.Groups{visit(&group.Groups[i])}
 }
 visit(def.ConditionGroup)
}
// Imported schedules are always disabled. Cross-workspace dependencies are not
// silently guessed; dangling parent IDs are rejected by validation.
func schedulePrepareImport(items []scheduleDefinition)[]scheduleDefinition{
 ids:=map[string]string{}
 for _,item:=range items{
  if item.ID!=""{ids[item.ID]=newScheduleID()}
 }
 out:=make([]scheduleDefinition,0,len(items))
 for _,item:=range items{
  oldID:=item.ID
  if next,ok:=ids[oldID];ok{item.ID=next}else{item.ID=newScheduleID()}
  item.Enabled=false;item.Paused=false;item.PauseReason=""
  item.Name=strings.TrimSpace(item.Name)
  if item.RetainDays==0{item.RetainDays=7}
  if item.RetainRuns==0{item.RetainRuns=30}
  scheduleRemapParents(&item,ids)
  out=append(out,item)
 }
 return out
}
