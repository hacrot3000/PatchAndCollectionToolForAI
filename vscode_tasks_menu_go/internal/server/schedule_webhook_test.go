package server

import "testing"

func TestScheduleWebhookConditionValidation(t *testing.T){
 s:=&Server{Workspace:t.TempDir()}
 single:=scheduleDefinition{ID:"hook",Name:"Webhook single",Type:"webhook",
  Target:"command",Command:"echo accepted",RetainDays:7,RetainRuns:10}
 store:=scheduleStore{Schedules:[]scheduleDefinition{single}}
 if err:=s.validateSchedule(single,store);err!=nil{t.Fatalf("webhook single rejected: %v",err)}
 multi:=single
 multi.ID="multi";multi.Name="Webhook AND Cron";multi.Type="multi"
 multi.ConditionGroup=&scheduleConditionGroup{Operator:"and",Conditions:[]scheduleCondition{
  {ID:"incoming",Type:"webhook"},
  {ID:"cron",Type:"cron",Cron:"*/5 * * * *",Timezone:"Local"},
 }}
 store.Schedules=append(store.Schedules,multi)
 if err:=s.validateSchedule(multi,store);err!=nil{t.Fatalf("webhook AND cron rejected: %v",err)}
 if len(scheduleLeaves(scheduleConditionsFor(multi)))!=2{t.Fatal("webhook leaf was dropped from multi condition expression")}
}
