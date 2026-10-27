package server

import (
 "encoding/json"
 "fmt"
 "strings"
 "sync"

 "bletonfc/vscode_tasks_menu/internal/session"
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
