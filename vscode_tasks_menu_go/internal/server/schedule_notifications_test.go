package server

import (
 "context"
 "testing"
 "time"
)

func TestScheduleNotificationSelection(t *testing.T){
 def:=scheduleDefinition{ID:"a",NotifyWebhookURL:"https://example.com/notify",
  NotifyOnSuccess:false,NotifyOnFailure:true}
 if scheduleNotificationEnabled(def,"success"){t.Fatal("unexpected success notification")}
 if !scheduleNotificationEnabled(def,"failure"){t.Fatal("missing failure notification")}
 def.NotifyOnSuccess=true
 if !scheduleNotificationEnabled(def,"success"){t.Fatal("missing success notification")}
 def.NotifyWebhookURL=""
 if scheduleNotificationEnabled(def,"failure"){t.Fatal("blank endpoint must disable delivery")}
}

func TestScheduleNotificationURLValidation(t *testing.T){
 s:=&Server{Workspace:t.TempDir()}
 def:=scheduleDefinition{ID:"a",Name:"Notify",Type:"cron",Cron:"* * * * *",
  Target:"command",Command:"echo ok",RetainDays:7,RetainRuns:10,
  NotifyWebhookURL:"file:///etc/passwd",NotifyOnFailure:true}
 if err:=s.validateSchedule(def,scheduleStore{Schedules:[]scheduleDefinition{def}});err==nil{
  t.Fatal("invalid notification scheme accepted")
 }
 def.NotifyWebhookURL="https://example.com/notify"
 if err:=s.validateSchedule(def,scheduleStore{Schedules:[]scheduleDefinition{def}});err!=nil{
  t.Fatalf("valid public webhook should pass validation: %v",err)
 }
 // Do not actually make an outgoing network request during this unit test.
 _=context.Background()
 _=time.Second
}
