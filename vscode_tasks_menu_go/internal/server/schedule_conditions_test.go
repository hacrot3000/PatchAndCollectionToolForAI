package server

import (
 "strings"
 "testing"
)

func TestScheduleConditionGroupANDORWithBranching(t *testing.T){
 g:=scheduleConditionGroup{Operator:"or",
  Groups:[]scheduleConditionGroup{
   {Operator:"and",Conditions:[]scheduleCondition{{ID:"url",Type:"url"},{ID:"git",Type:"git"}}},
   {Operator:"and",Conditions:[]scheduleCondition{{ID:"parent-success",Type:"dependency"},{ID:"parent-console",Type:"dependency"}}},
  },
 }
 met:=map[string]bool{"url":true}
 if scheduleGroupSatisfied(g,met){t.Fatal("one AND operand must not satisfy OR group")}
 met["git"]=true
 if !scheduleGroupSatisfied(g,met){t.Fatal("AND branch should satisfy root OR")}
 met=map[string]bool{"parent-success":true,"parent-console":true}
 if !scheduleGroupSatisfied(g,met){t.Fatal("second branch of OR should be sufficient")}
 met=map[string]bool{"parent-success":true}
 if scheduleGroupSatisfied(g,met){t.Fatal("partial dependency branch must not fire")}
 if scheduleGroupSatisfied(scheduleConditionGroup{Operator:"or"},met){t.Fatal("empty conditions must never fire")}
 if scheduleGroupSatisfied(scheduleConditionGroup{Operator:"unknown",Conditions:[]scheduleCondition{{ID:"x"}}},map[string]bool{"x":true}){t.Fatal("unknown operator cannot fire")}
}
func TestScheduleLegacyExpressionMigration(t *testing.T){
 old:=scheduleDefinition{ID:"B",Type:"dependency",ParentID:"A",ParentState:"failure",OutputContains:"ERROR"}
 group:=scheduleConditionsFor(old)
 if len(group.Conditions)!=1||group.Conditions[0].ID!="legacy"{
  t.Fatalf("legacy schedule must have one synthetic condition, got %+v",group)
 }
 if got:=scheduleParentReferences(old);len(got)!=1||got[0]!="A"{t.Fatalf("legacy dependency lost: %v",got)}
 if !scheduleGroupSatisfied(group,map[string]bool{"legacy":true}){t.Fatal("legacy event no longer matches")}
}
func TestScheduleMultiConditionGraphRejectsCycles(t *testing.T){
 s:=&Server{Workspace:t.TempDir()}
 a:=scheduleDefinition{ID:"a",Name:"A",Type:"multi",Target:"command",Command:"echo A",RetainDays:7,RetainRuns:10}
 b:=scheduleDefinition{ID:"b",Name:"B",Type:"multi",Target:"command",Command:"echo B",RetainDays:7,RetainRuns:10}
 a.ConditionGroup=&scheduleConditionGroup{Operator:"and",Conditions:[]scheduleCondition{
  {ID:"a-after-b",Type:"dependency",ParentID:"b",ParentState:"success"},
  {ID:"a-cron",Type:"cron",Cron:"* * * * *",Timezone:"Local"},
 }}
 b.ConditionGroup=&scheduleConditionGroup{Operator:"or",Conditions:[]scheduleCondition{
  {ID:"b-after-a",Type:"dependency",ParentID:"a",ParentState:"success"},
 }}
 store:=scheduleStore{Schedules:[]scheduleDefinition{a,b}}
 if err:=s.validateSchedule(a,store);err==nil||!strings.Contains(err.Error(),"cycle"){
  t.Fatalf("expected multi-parent cycle rejection, got %v",err)
 }
 b.ConditionGroup.Conditions[0].ParentID="missing"
 store.Schedules[1]=b
 if err:=s.validateSchedule(a,store);err==nil||!strings.Contains(err.Error(),"not found"){
  t.Fatalf("expected missing parent rejection, got %v",err)
 }
}
func TestScheduleMultiConditionValidation(t *testing.T){
 s:=&Server{Workspace:t.TempDir()}
 c:=scheduleDefinition{ID:"c",Name:"C",Type:"multi",Target:"command",Command:"echo ok",RetainDays:7,RetainRuns:10}
 c.ConditionGroup=&scheduleConditionGroup{Operator:"or",Conditions:[]scheduleCondition{
  {ID:"one",Type:"cron",Cron:"*/5 * * * *",Timezone:"Local"},
  {ID:"two",Type:"cron",Cron:"* * * * *",Timezone:"Local"},
 }}
 store:=scheduleStore{Schedules:[]scheduleDefinition{c}}
 if err:=s.validateSchedule(c,store);err!=nil{t.Fatalf("valid OR conditions rejected: %v",err)}
 c.ConditionGroup.Conditions[1].ID="one"
 if err:=s.validateSchedule(c,store);err==nil||!strings.Contains(err.Error(),"unique"){t.Fatalf("duplicate ID was accepted: %v",err)}
 c.ConditionGroup.Conditions[1].ID="two"
 c.ConditionGroup.Operator="xor"
 if err:=s.validateSchedule(c,store);err==nil{t.Fatal("unsupported operator accepted")}
}
