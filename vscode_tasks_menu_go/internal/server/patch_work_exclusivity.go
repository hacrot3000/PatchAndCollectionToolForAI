package server

import (
 "context"
 "errors"
 "time"
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

type agentNativeIntent struct{ workspace,kind string }
var agentNativeIntents=make(map[*agentNativeIntent]struct{})
type patchWorkBusyError struct{ running,requested string }
func(e *patchWorkBusyError)Error()string{return fmt.Sprintf("%s is running; cannot start %s simultaneously",e.running,e.requested)}

// Ephemeral owner-scoped wait status, never execution or replay authority.
type agentNativeWaitContextKey struct{}
var agentNativeWaitState=struct{sync.Mutex; entries map[string]string}{entries:make(map[string]string)}
func agentNativeWaitContext(ctx context.Context,workspace,owner,id string)context.Context{
 return context.WithValue(ctx,agentNativeWaitContextKey{},agentRequestKey(workspace,owner,id))
}
func agentNativeWaitSnapshot(workspace,owner,id string)string{
 agentNativeWaitState.Lock();defer agentNativeWaitState.Unlock()
 return agentNativeWaitState.entries[agentRequestKey(workspace,owner,id)]
}
func setAgentNativeWait(ctx context.Context,reason string){
 key,_:=ctx.Value(agentNativeWaitContextKey{}).(string);if key==""{return}
 agentNativeWaitState.Lock();defer agentNativeWaitState.Unlock()
 if reason==""{delete(agentNativeWaitState.entries,key)}else{agentNativeWaitState.entries[key]=reason}
}

// Retry only a proven pre-execution resource conflict, not failed launches
// or ambiguous native outcomes. Cancellation ends the wait without launch.
func waitAgentNativeAdmission(ctx context.Context,attempt func()error)error{
 defer setAgentNativeWait(ctx,"")
 for{
  if err:=ctx.Err();err!=nil{return err}
  err:=attempt()
  if err==nil{return nil}
  var busy *patchWorkBusyError
  if errors.As(err,&busy){
   setAgentNativeWait(ctx,"waiting_for_"+strings.ToLower(busy.running))
  } else if errors.Is(err,errAgentWorkspaceContended){
   setAgentNativeWait(ctx,"waiting_for_workspace")
  } else{return err}
  timer:=time.NewTimer(500*time.Millisecond)
  select{case <-ctx.Done():timer.Stop();return ctx.Err();case <-timer.C:}
 }
}

// Reserve before publishing a ZIP; bridge the gap until native session
// admission without holding the global mutex during filesystem/Python work.
func(s *Server) reserveAgentNativeIntent(ctx context.Context,kind string)(func(),error){
 if s.Sessions==nil{return func(){},nil}
 intent:=&agentNativeIntent{workspace:filepath.Clean(s.Workspace),kind:kind}
 err:=waitAgentNativeAdmission(ctx,func()error{
  patchWorkAdmission.Lock();defer patchWorkAdmission.Unlock()
  if err:=ctx.Err();err!=nil{return err}
  if err:=s.patchWorkConflictLocked(kind,kind=="COLLECT");err!=nil{return err}
  agentNativeIntents[intent]=struct{}{}
  return nil
 })
 if err!=nil{return nil,err}
 var once sync.Once
 return func(){once.Do(func(){patchWorkAdmission.Lock();delete(agentNativeIntents,intent);patchWorkAdmission.Unlock()})},nil
}

func(s *Server) patchWorkConflictLocked(kind string,directCollect bool)error{
 if s.Sessions==nil{return fmt.Errorf("native Patch session service unavailable")}
 for intent:=range agentNativeIntents{
  if intent.workspace==filepath.Clean(s.Workspace)&&intent.kind!=kind{return &patchWorkBusyError{intent.kind,kind}}
 }
 provider,_:=s.Sessions.(session.ProtocolStateProvider)
 for _,meta:=range s.Sessions.List(){
  if !strings.EqualFold(meta.Status,"running")||filepath.Clean(meta.Cwd)!=filepath.Clean(s.Workspace)||
   (!strings.HasPrefix(meta.Label,"Patch Tool")&&!strings.EqualFold(meta.Kind,"patch")){continue}
  other:=patchWorkReserved[meta.ID]
  if other==""&&strings.HasPrefix(meta.Label,"Patch Tool · COLLECT ·"){other="COLLECT"}
  if other==""&&provider!=nil{
   if state,err:=provider.ProtocolState(meta.ID);err==nil{
    for _,item:=range state.Items{
     status:=strings.ToUpper(strings.TrimSpace(item.Status))
     if status=="RUNNING"||(!directCollect&&(status=="PASS"||status=="FAIL"||status=="INCOMPLETE"||status=="PREFLIGHT_FAIL")){
      candidate:=strings.ToUpper(strings.TrimSpace(item.Kind))
      if candidate=="PATCH"||candidate=="COLLECT"{other=candidate;break}
     }
    }
    if other==""&&state.Progress!=nil&&state.Progress.Scope=="collect"{other="COLLECT"}
   }
  }
  if other!=""&&other!=kind{return &patchWorkBusyError{other,kind}}
 }
 return nil
}

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
 if err:=s.patchWorkConflictLocked(normalized,false);err!=nil{patchWorkAdmission.Unlock();return nil,err}
 // Prune terminal reservations belonging to this session service.
 for id:=range patchWorkReserved{
  if meta,ok:=s.Sessions.Metadata(id);ok&&!strings.EqualFold(meta.Status,"running"){delete(patchWorkReserved,id)}
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
 if err:=s.patchWorkConflictLocked("COLLECT",true);err!=nil{return session.Metadata{},err}
 meta,err:=s.Sessions.Start(spec)
 if err!=nil{return session.Metadata{},err}
 patchWorkReserved[meta.ID]="COLLECT"
 return meta,nil
}

func(s *Server) startAgentNativeCollectWaiting(ctx context.Context,spec tasks.Execution)(session.Metadata,error){
 var meta session.Metadata
 err:=waitAgentNativeAdmission(ctx,func()error{
  var err error
  meta,err=s.startAgentNativeCollectDirect(spec)
  return err
 })
 return meta,err
}
