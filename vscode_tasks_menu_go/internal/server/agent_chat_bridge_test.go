package server

import (
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

type fakeAgentNativeBridge struct {
 session.Service
 workspace string
 meta session.Metadata
 state session.ProtocolState
 started int
 selectedIndexes []int
}
func (b *fakeAgentNativeBridge)List()[]session.Metadata{
 if b.started>0{return []session.Metadata{b.meta}}
 return nil
}
func (b *fakeAgentNativeBridge)Start(spec tasks.Execution)(session.Metadata,error){
 b.started++
 if b.started>1{return session.Metadata{},fmt.Errorf("native Patch started twice")}
 if !spec.ProtocolCommands||!spec.ProtocolEvents||spec.TaskID!=-1{return session.Metadata{},fmt.Errorf("non-native Patch execution spec")}
 entries,err:=os.ReadDir(filepath.Join(b.workspace,"patchs"));if err!=nil{return session.Metadata{},err}
 if len(entries)!=1{return session.Metadata{},fmt.Errorf("expected one queued agent package, got %d",len(entries))}
 approved:=entries[0].Name()
 b.state=session.ProtocolState{
  Available:true,Enabled:true,CommandsEnabled:true,
 }
 prompt:=map[string]any{
  "protocol":"taskdeck.patch","version":1,"type":"prompt","prompt_id":"agent-prompt-1","prompt_kind":"queue_selection",
  "actions":[]string{"select","cancel"},
  "items":[]any{
   map[string]any{"index":1,"name":"unrelated.zip","kind":"PATCH","selectable":true},
   map[string]any{"index":2,"name":approved,"kind":"PATCH","selectable":true},
  },
 }
 b.state.Prompt,_=json.Marshal(prompt)
 b.meta=session.Metadata{ID:"native-agent-1",TaskID:-1,Kind:tasks.SessionKindPatch,Cwd:b.workspace,Label:"Patch Tool · Queue",Status:"running"}
 return b.meta,nil
}
func (b *fakeAgentNativeBridge)Metadata(id string)(session.Metadata,bool){
 return b.meta,id==b.meta.ID
}
func (b *fakeAgentNativeBridge)ProtocolState(id string)(session.ProtocolState,error){
 if id!=b.meta.ID{return session.ProtocolState{},fmt.Errorf("invalid session ID")}
 return b.state,nil
}
func (b *fakeAgentNativeBridge)ProtocolCommand(id string,data []byte)error{
 if id!=b.meta.ID{return fmt.Errorf("unexpected session")}
 var cmd struct{
  Command string `json:"command"`
  Payload struct{
   PromptID string `json:"prompt_id"`
   Action string `json:"action"`
   Indexes []int `json:"indexes"`
  } `json:"payload"`
 }
 if err:=json.Unmarshal(data,&cmd);err!=nil{return err}
 if cmd.Command!="prompt_response"||cmd.Payload.PromptID!="agent-prompt-1"||cmd.Payload.Action!="select"{
  return fmt.Errorf("native prompt command mismatch")
 }
 b.selectedIndexes=cmd.Payload.Indexes
 return nil
}
func TestAgentApprovedPatchRunsExactlyOneNativeQueueEntry(t *testing.T){
 s,id,_:=agentPendingCallFixture(t,true)
 runtimeDir:=t.TempDir()
 entry:=filepath.Join(runtimeDir,"python_patch_entry.py")
 python:=filepath.Join(runtimeDir,"python3")
 for _,path:=range []string{entry,python}{
  if err:=os.WriteFile(path,[]byte("#!/bin/sh\n"),0755);err!=nil{t.Fatal(err)}
 }
 t.Setenv("TASKDECK_PATCH_RUNTIME",entry)
 t.Setenv("TASKDECK_PATCH_PYTHON",python)
 fake:=&fakeAgentNativeBridge{workspace:s.Workspace}
 s.Sessions=fake
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 pending,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 response:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
  map[string]any{"action":"approve","confirm_execution":true,"expected_revision":pending.Revision})
 if response.Code!=200{t.Fatalf("native dispatch failed: %d %s",response.Code,response.Body)}
 current,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if fake.started!=1||len(fake.selectedIndexes)!=1||fake.selectedIndexes[0]!=2{
  t.Fatalf("wrong native patch launched: start=%d selection=%v",fake.started,fake.selectedIndexes)
 }
 if current.ToolCalls[0].Status!="running"||current.ToolCalls[0].PatchSessionID!=fake.meta.ID{
  t.Fatalf("approved native run not linked to chat: %+v",current.ToolCalls[0])
 }
 retry:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/tools/call_agent_001/decision",
  map[string]any{"action":"approve","confirm_execution":true,"expected_revision":current.Revision})
 if retry.Code!=409||fake.started!=1{t.Fatalf("replayed native execution: code=%d start=%d",retry.Code,fake.started)}
}
