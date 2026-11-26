package server

import (
 "encoding/json"
 "fmt"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentCacheEpochAppendAndPolicyReset(t *testing.T){
 for _,api:=range []string{"responses","chat_completions"}{
  x:=agentchat.Session{History:[]agentchat.HistoryItem{{Type:"user",Content:"Original task"}}}
  p:=agentchat.ProviderConfig{APIType:api}
  model:=agentchat.ModelConfig{MaxInputTokens:65536}
  c:=agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"Trusted policy",EnableCollectionTool:true,Next:[]agentchat.HistoryItem{{Type:"user",Content:"Continue"}}}
  first,e,_,err:=agentBuildContextEpoch(x,model,p,c,nil,true)
  if err!=nil||e==nil{t.Fatalf("first epoch: %v",err)}
  x.History=append(x.History,c.Next...)
  x.History=append(x.History,agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"read",Arguments:"{}"})
  c.Next=[]agentchat.HistoryItem{{Type:"function_call_output",CallID:"read",Content:"native receipt"}}
  second,next,reason,err:=agentBuildContextEpoch(x,model,p,c,e,true)
  if err!=nil||next==nil||reason!="append_only"||e.ID!=next.ID{t.Fatalf("reuse %s: %v",reason,err)}
  var a,b struct{Instructions string;Input []json.RawMessage}
  if json.Unmarshal(first,&a)!=nil||json.Unmarshal(second,&b)!=nil{t.Fatal("invalid payload")}
  if a.Instructions!=b.Instructions||len(a.Input)>=len(b.Input){t.Fatal("prefix did not remain stable")}
  for i:=range a.Input{if string(a.Input[i])!=string(b.Input[i]){t.Fatal("old wire input changed")}}
  if strings.Count(string(second),"native receipt")!=1{t.Fatal("receipt duplicated")}
  c.Instructions="Updated trusted policy: collection disabled";c.EnableCollectionTool=false
  third,_,why,err:=agentBuildContextEpoch(x,model,p,c,next,true)
  if err!=nil||why=="append_only"||!strings.Contains(string(third),"Updated trusted policy"){t.Fatalf("policy reset: %s %v",why,err)}
 }
}


func TestAgentCacheEpochCurrentPlanAndRevocationRemainTrusted(t *testing.T){
 x:=agentchat.Session{ProjectID:"project-id",TaskPlan:&agentchat.TaskPlan{Goal:"audit source",State:"active",Steps:[]agentchat.TaskPlanStep{{ID:"one",Title:"inspect",Status:"pending",Acceptance:"read exact file"}}}}
 stable,current:=agentChatInstructionsWithAutomationParts(x,true,"project")
 if !strings.Contains(current,"audit source")||strings.Contains(stable,"audit source"){t.Fatal("owner plan not separated")}
 x.TaskPlan.Goal="updated objective";x.TaskPlan.Steps[0].Acceptance="verify receipt"
 stable2,current2:=agentChatInstructionsWithAutomationParts(x,true,"project")
 if stable2!=stable||current2==current||!strings.Contains(current2,"updated objective"){t.Fatal("mutable plan affects static policy or failed to refresh")}
 cfg:=agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:stable2,CurrentInstructions:current2,EnableCollectionTool:true,Next:[]agentchat.HistoryItem{{Type:"user",Content:"Continue"}}}
 body,err:=agentchat.BuildResponsesRequest(cfg);if err!=nil{t.Fatal(err)}
 var wire struct{Instructions string;Input []json.RawMessage}
 if json.Unmarshal(body,&wire)!=nil||!strings.HasSuffix(wire.Instructions,current2)||len(wire.Input)!=1||strings.Contains(string(wire.Input[0]),"updated objective"){t.Fatal("trusted plan demoted to history")}
 disabled:=false;x.AutoCollection=&disabled
 revoked,_:=agentChatInstructionsWithAutomationParts(x,true,"project")
 if revoked==stable||!strings.Contains(revoked,"Collection has been disabled"){t.Fatal("revocation was deferred to retain cache")}
}

func TestAgentCacheEpochOptionalAdviceCannotBlockMandatoryRequest(t *testing.T){
 // Find the exact native contract overhead with the real request builder.
 c:=agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"x",EnableCollectionTool:true,Next:[]agentchat.HistoryItem{{Type:"user",Content:"mandatory owner instruction"}}}
 body,err:=agentchat.BuildResponsesRequest(c);if err!=nil{t.Fatal(err)}
 var wire struct{Instructions string}
 if json.Unmarshal(body,&wire)!=nil{t.Fatal("invalid request")}
 c.Instructions=strings.Repeat("x",(1<<20)-len(wire.Instructions)+1)
 got,_,reason,err:=agentBuildContextEpoch(agentchat.Session{},agentchat.ModelConfig{MaxInputTokens:4000000},agentchat.ProviderConfig{APIType:"responses"},c,nil,false)
 if err!=nil||!strings.Contains(reason,"optional_advice_omitted")||!strings.Contains(string(got),"mandatory owner instruction"){t.Fatalf("optional guidance prevented valid request: %s %v",reason,err)}
}


func TestAgentCacheEpochStatefulBoundaryResetsDerivedCheckpoint(t *testing.T){
 store,err:=agentchat.NewStore(filepath.Join(t.TempDir(),"private"));if err!=nil{t.Fatal(err)}
 x,err:=agentchat.NewSession("local","/project","project-id","gateway","m",agentchat.Stateless);if err!=nil{t.Fatal(err)}
 x.History=[]agentchat.HistoryItem{{Type:"user",Content:"goal"}}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 s:=&Server{Workspace:x.Workspace}
 p:=agentchat.ProviderConfig{APIType:"responses",ID:"gateway"}
 model:=agentchat.ModelConfig{MaxInputTokens:65536}
 c:=agentchat.RequestConfig{Model:"m",Instructions:"trusted policy",Mode:agentchat.Stateless,Next:[]agentchat.HistoryItem{{Type:"user",Content:"continue"}}}
 if _,err=s.agentBuildCachedContextRequest(store,x,model,p,"credential",c);err!=nil{t.Fatal(err)}
 if old,e:=store.ReadCacheEpoch(x.ID,x.Owner,x.Revision);e!=nil||old==nil{t.Fatalf("expected stateless checkpoint: %v",e)}
 c.Mode=agentchat.PreviousResponse;c.PreviousResponseID="resp_1"
 if _,err=s.agentBuildCachedContextRequest(store,x,model,p,"credential",c);err!=nil{t.Fatal(err)}
 if old,e:=store.ReadCacheEpoch(x.ID,x.Owner,x.Revision);e!=nil||old!=nil{t.Fatalf("stateful boundary retained stateless checkpoint: %v",e)}
 fresh,err:=store.Get(x.ID,x.Owner);if err!=nil||fresh.Revision!=x.Revision||len(fresh.History)!=1||fresh.History[0].Content!="goal"{t.Fatal("derived checkpoint change mutated session")}
}


// Local acceptance: same project, same store, independent conversations and
// independently reopened stores. No simulated upstream cache hit claims.
func TestAgentCacheEpochConcurrentConversationsTwentyTurns(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private")
 store,err:=agentchat.NewStore(dir);if err!=nil{t.Fatal(err)}
 model:=agentchat.ModelConfig{ID:"m",MaxInputTokens:65536}
 type fixture struct{x agentchat.Session;p agentchat.ProviderConfig}
 var fixtures []fixture
 for _,api:=range []string{"responses","chat_completions"}{
  for i:=0;i<2;i++{
   p:=agentchat.ProviderConfig{APIType:api,ID:"gateway",BaseURL:"https://gateway.example/v1",CacheAffinity:"both",Models:[]agentchat.ModelConfig{model}}
   x,e:=agentchat.NewSession("local","/project","project-id",p.ID,model.ID,agentchat.Stateless);if e!=nil{t.Fatal(e)}
   // Half the cases represent an existing conversation from before epochs.
   if i==0{x.History=[]agentchat.HistoryItem{{Type:"user",Content:"existing goal "+x.ID},{Type:"assistant",Content:"existing answer"}}}
   x,e=store.Save(x,0);if e!=nil{t.Fatal(e)}
   fixtures=append(fixtures,fixture{x,p})
  }
 }
 keys:=map[string]bool{}
 for _,f:=range fixtures{
  key:=agentchat.ProviderAffinityKey(f.x,f.p,model.ID)
  if key==""||keys[key]{t.Fatal("affinity collision")};keys[key]=true
 }
 for _,f:=range fixtures{
  f:=f
  t.Run(f.p.APIType+"/"+f.x.ID,func(t *testing.T){
   t.Parallel()
   x,p:=f.x,f.p
   s:=&Server{Workspace:x.Workspace}
   st,e:=agentchat.NewStore(dir);if e!=nil{t.Fatal(e)}
   var prior struct{Instructions string;Input []json.RawMessage;Key string `json:"prompt_cache_key"`}
   epochID:=""
   for turn:=0;turn<20;turn++{
    if turn%5==0{st,e=agentchat.NewStore(dir);if e!=nil{t.Fatal(e)};s=&Server{Workspace:x.Workspace};x,e=st.Get(x.ID,x.Owner);if e!=nil{t.Fatal(e)}}
    message:=agentchat.HistoryItem{Type:"user",Content:fmt.Sprintf("turn %d for %s",turn,x.ID)}
    cfg:=agentchat.RequestConfig{Model:model.ID,Mode:agentchat.Stateless,Instructions:"trusted stable policy",Next:[]agentchat.HistoryItem{message}}
    body,e:=s.agentBuildCachedContextRequest(st,x,model,p,"credential",cfg);if e!=nil{t.Fatal(e)}
    var current struct{Instructions string;Input []json.RawMessage;Key string `json:"prompt_cache_key"`}
    if json.Unmarshal(body,&current)!=nil||current.Key!=agentchat.ProviderAffinityKey(x,p,model.ID){t.Fatal("incorrect key")}
    if turn>0{
     if current.Instructions!=prior.Instructions||len(current.Input)!=len(prior.Input)+2{t.Fatal("unexpected prefix growth")}
     for i,v:=range prior.Input{if string(current.Input[i])!=string(v){t.Fatalf("prefix changed at item %d turn %d",i,turn)}}
    }
    if p.APIType=="chat_completions"{if _,e=agentchat.BuildChatCompletionsRequest(body);e!=nil{t.Fatal(e)}}
    old,e:=st.ReadCacheEpoch(x.ID,x.Owner,x.Revision);if e!=nil||old==nil{t.Fatalf("checkpoint missing: %v",e)}
    if turn==0{epochID=old.ID}else if old.ID!=epochID{t.Fatal("epoch changed during bounded append-only sequence")}
    x.History=append(x.History,message,agentchat.HistoryItem{Type:"assistant",Content:"answer"})
    x,e=st.Save(x,x.Revision);if e!=nil{t.Fatal(e)}
    prior=current
   }
   t.Logf("20 local turns, 4 store reopens, exact input prefix and stable affinity for %s",p.APIType)
  })
 }
}

func TestAgentFullHistoryUntilActualContextLimit(t *testing.T){
 for _,api:=range []string{"responses","chat_completions"}{
  t.Run(api,func(t *testing.T){
   goal:=agentchat.HistoryItem{Type:"user",Content:"Original objective"}
   x:=agentchat.Session{History:[]agentchat.HistoryItem{goal}}
   for i:=0;i<12;i++{
    id:=fmt.Sprintf("read-%d",i)
    x.History=append(x.History,agentchat.HistoryItem{Type:"assistant",Content:fmt.Sprintf("finding-%d ",i)+strings.Repeat("important finding ",200)},agentchat.HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:id,Arguments:"{}"},agentchat.HistoryItem{Type:"function_call_output",CallID:id,Content:strings.Repeat("exact source ",4000)})
   }
   model:=agentchat.ModelConfig{MaxInputTokens:32768}
   p:=agentchat.ProviderConfig{APIType:api}
   cfg:=agentchat.RequestConfig{Model:"m",Mode:agentchat.Stateless,Instructions:"trusted",Next:[]agentchat.HistoryItem{{Type:"user",Content:"continue"}}}
   for _,dynamic:=range []string{"first guidance","changed guidance"}{
    cfg.CurrentInstructions=dynamic
    body,_,reason,err:=agentBuildContextEpoch(x,model,p,cfg,nil,true)
    if err!=nil||reason!="full_history"{t.Fatalf("premature compact: %s %v",reason,err)}
    var wire struct{Input []map[string]string}
    if json.Unmarshal(body,&wire)!=nil||len(wire.Input)!=len(x.History)+1{t.Fatal("history was discarded")}
    for i,it:=range x.History{
     if it.Type=="function_call_output"&&wire.Input[i]["output"]!=it.Content{t.Fatal("receipt changed")}
     if it.Type=="assistant"&&wire.Input[i]["content"]!=it.Content{t.Fatal("finding changed")}
    }
   }
   for len(x.History)<256{x.History=append(x.History,agentchat.HistoryItem{Type:"assistant",Content:"old narration"})}
   body,_,reason,err:=agentBuildContextEpoch(x,model,p,cfg,nil,true)
   if err!=nil||reason!="compact_local_limit"||!strings.Contains(string(body),goal.Content)||!strings.Contains(string(body),"continue"){t.Fatalf("actual local item limit recovery: %s %v",reason,err)}
  })
 }
}
