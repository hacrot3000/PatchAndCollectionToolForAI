package server

import (
 "encoding/json"
 "fmt"
 "path/filepath"
 "strings"
 "sync"

 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// A process-wide gate serializes admission decisions, but not the workers.
// PATCH+PATCH and COLLECT+COLLECT are allowed; opposite kinds are rejected.
// The gate follows the actual session lifecycle rather than a browser tab.
var patchWorkAdmission sync.Mutex
var patchWorkReserved = make(map[string]string)

func patchSelectedWorkKind(state session.ProtocolState, indexes []int) (string,error) {
 var prompt struct {
  Items []struct { Index int `json:"index"`; Kind string `json:"kind"` } `json:"items"`
 }
 if err:=json.Unmarshal(state.Prompt,&prompt);err!=nil{return "",fmt.Errorf("invalid Patch selection: %w",err)}
 selected:=make(map[int]bool,len(indexes))
 for _,id:=range indexes { selected[id]=true }
 kind:=""
 for _,item:=range prompt.Items {
  if !selected[item.Index] {continue}
  normalized:=strings.ToUpper(strings.TrimSpace(item.Kind))
  if normalized=="COLLECT INVALID" {return "",fmt.Errorf("COLLECT INVALID cannot run. Open or download its AI diagnostic handoff; the original ZIP must not be modified")}
  if normalized!="PATCH"&&normalized!="COLLECT"{return "",fmt.Errorf("invalid selected work kind %q",item.Kind)}
  if kind!=""&&kind!=normalized {return "",fmt.Errorf("PATCH and COLLECT must not be selected together")}
  kind=normalized
 }
 if kind=="" {return "",fmt.Errorf("no PATCH or COLLECT selected")}
 return kind,nil
}

func (s *Server) reservePatchWorkKind(sessionID, kind string) (func(), error) {
 patchWorkAdmission.Lock()
 normalized:=strings.ToUpper(strings.TrimSpace(kind))
 if normalized!="PATCH"&&normalized!="COLLECT" {
  patchWorkAdmission.Unlock()
  return nil,fmt.Errorf("invalid Patch Tool work kind")
 }
 live:=make(map[string]bool)
 provider,_:=s.Sessions.(session.ProtocolStateProvider)
 for _,meta:=range s.Sessions.List() {
  if !strings.EqualFold(meta.Status,"running"){continue}
  if filepath.Clean(meta.Cwd)!=filepath.Clean(s.Workspace){continue}
  if !strings.HasPrefix(meta.Label,"Patch Tool")&&
     !strings.EqualFold(meta.Kind,"patch") {continue}
  live[meta.ID]=true
  other:=patchWorkReserved[meta.ID]
  if other==""&&strings.HasPrefix(meta.Label,"Patch Tool · COLLECT ·"){
   other="COLLECT"
  }
  // An older native foreground session may already be executing when a
  // refreshed browser submits a new queue. Use protocol evidence too.
  if other==""&&provider!=nil {
   if state,err:=provider.ProtocolState(meta.ID);err==nil {
    for _,item:=range state.Items {
     if item.Status=="RUNNING"||item.Status=="PASS"||item.Status=="FAIL"||
        item.Status=="INCOMPLETE"||item.Status=="PREFLIGHT_FAIL" {
      candidate:=strings.ToUpper(strings.TrimSpace(item.Kind))
      if candidate=="PATCH"||candidate=="COLLECT" {other=candidate;break}
     }
    }
    if other==""&&state.Progress!=nil&&state.Progress.Scope=="collect" {other="COLLECT"}
   }
  }
  if other!=""&&other!=normalized {
   patchWorkAdmission.Unlock()
   return nil,fmt.Errorf("%s is running; cannot start %s simultaneously",other,normalized)
  }
 }
 for id:=range patchWorkReserved {
  if !live[id] {delete(patchWorkReserved,id)}
 }
 patchWorkReserved[sessionID]=normalized
 return patchWorkAdmission.Unlock,nil
}


// startAgentNativeCollectDirect is the same Python direct COLLECT worker used
// by the manual Parallel COLLECT action. Its admission decision and Start are
// atomic against native PATCH selection. Pending/unselected PATCH packages and
// diagnostic COLLECT INVALID queue rows are not executing PATCHes and must not
// block an independent, read-only Agent COLLECT.
func (s *Server) startAgentNativeCollectDirect(spec tasks.Execution) (session.Metadata,error) {
 patchWorkAdmission.Lock()
 defer patchWorkAdmission.Unlock()
 if s.Sessions==nil {return session.Metadata{},fmt.Errorf("native Patch session service unavailable")}
 provider,_:=s.Sessions.(session.ProtocolStateProvider)
 for _,meta:=range s.Sessions.List(){
  if !strings.EqualFold(meta.Status,"running") ||
   filepath.Clean(meta.Cwd)!=filepath.Clean(s.Workspace) ||
   (!strings.HasPrefix(meta.Label,"Patch Tool")&&!strings.EqualFold(meta.Kind,"patch")) {
   continue
  }
  runningKind:=patchWorkReserved[meta.ID]
  if runningKind==""&&strings.HasPrefix(meta.Label,"Patch Tool · COLLECT ·"){
   runningKind="COLLECT"
  }
  if runningKind==""&&provider!=nil {
   if st,e:=provider.ProtocolState(meta.ID);e==nil{
    for _,item:=range st.Items{
     status:=strings.ToUpper(strings.TrimSpace(item.Status))
     // Completed protocol rows are historical evidence, not a worker
     // currently mutating source. An already approved but unstarted PATCH
     // in a queue must not block a read-only direct COLLECT.
     if status=="RUNNING"{
      kind:=strings.ToUpper(strings.TrimSpace(item.Kind))
      if kind=="PATCH"||kind=="COLLECT"{runningKind=kind;break}
     }
    }
    if runningKind==""&&st.Progress!=nil&&st.Progress.Scope=="collect"{
     runningKind="COLLECT"
    }
   }
  }
  if runningKind=="PATCH"{
   return session.Metadata{},fmt.Errorf("PATCH_IN_PROGRESS: native PATCH execution must finish before an Agent COLLECT starts")
  }
 }
 meta,err:=s.Sessions.Start(spec)
 if err!=nil{return session.Metadata{},err}
 patchWorkReserved[meta.ID]="COLLECT"
 return meta,nil
}
