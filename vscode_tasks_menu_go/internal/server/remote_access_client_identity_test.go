package server

import (
 "context"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/config"
)

// Uses the same real TaskDeck SQLite identity store as browser shared-login.
// An attacker-controlled PHP ticket can never supply the immutable user ID.
func TestRemoteClientPreflightResolvesHostLocalIdentityBeforePassword(t *testing.T){
 ctx:=context.Background()
 for _,kind:=range []string{
  "active","unknown","disabled user","disabled membership",
  "disabled project","remote feature disabled","cancelled context",
 }{
  t.Run(kind,func(t *testing.T){
   s:=sharedLoginTestServer(t)
   s.Config.RemoteAccessEnabled=true
   s.Config.Protocol=config.ProtocolHTTPS
   user,err:=s.Identity.UserByUsername(ctx,"alice");if err!=nil{t.Fatal(err)}
   project,err:=s.Identity.ProjectByKey(ctx,s.Config.SharedProjectID)
   if err!=nil{t.Fatal(err)}
   lookupCtx:=ctx
   name:="alice"
   switch kind{
   case "unknown":name="attacker-controlled-portal-subject"
   case "disabled user":
    if err:=s.Identity.SetUserEnabled(ctx,user.ID,false,time.Now().UTC());err!=nil{t.Fatal(err)}
   case "disabled membership":
    member,err:=s.Identity.ProjectMember(ctx,project.ID,user.ID)
    if err!=nil{t.Fatal(err)}
    member.Enabled=false
    if err:=s.Identity.UpsertProjectMember(ctx,member);err!=nil{t.Fatal(err)}
   case "disabled project":
    if err:=s.Identity.SetProjectEnabled(ctx,project.ID,false,time.Now().UTC());err!=nil{t.Fatal(err)}
   case "remote feature disabled":s.Config.RemoteAccessEnabled=false
   case "cancelled context":
    canceled,stop:=context.WithCancel(ctx);stop();lookupCtx=canceled
   }
   id,err:=s.LookupRemoteClientLocalUserID(lookupCtx,name)
   if kind=="active"{
    if err!=nil||id!=string(user.ID){t.Fatalf("active local TaskDeck ID not resolved: %q %v",id,err)}
   } else if err==nil||id!="" {
    t.Fatalf("P0: inactive/missing local TaskDeck account triggered key approval: %q %v",id,err)
   }
  })
 }
}
