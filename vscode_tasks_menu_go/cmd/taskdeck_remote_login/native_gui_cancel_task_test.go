package main

import (
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "sync"
 "testing"
 "time"
)

// Exercises the real HTTP handler, not merely a direct g.close() call.
// The approved catalogue and typed two-phase confirmation are identical
// to production; the remote is deliberately stalled after admission.
type blockedNativeTaskRemote struct{
 *fakeDesktopGUIRemote
 mode string
 started chan struct{}
 release chan struct{}
 canceled chan struct{}
 once sync.Once
 ignoreCancellation bool
}
func (r *blockedNativeTaskRemote) block(ctx context.Context)(string,error){
 r.once.Do(func(){close(r.started)})
 if r.ignoreCancellation{
  <-r.release // a malicious remote/transport ignores request cancellation
  return "LATE_E2EE_TASK_SUCCESS",nil
 }
 select{
 case <-ctx.Done():
  close(r.canceled)
  return "",ctx.Err()
 case <-r.release:
  return "EARLY_REMOTE_SUCCESS",nil
 }
}
func(r *blockedNativeTaskRemote) RunTaskReport(ctx context.Context,id int)(string,error){
 return r.block(ctx)
}
func(r *blockedNativeTaskRemote) StartManagedTask(ctx context.Context,id int)(string,error){
 return r.block(ctx)
}

func nativeCloseRequest(g *trustedGUI)*http.Request{
 request:=httptest.NewRequest(http.MethodPost,g.origin+"/api/close",nil)
 request.Header.Set("Origin",g.origin)
 request.Header.Set("X-TaskDeck-Local-Key",g.key)
 request.Host=strings.TrimPrefix(g.origin,"http://")
 return request
}

func TestNativeGUICloseCancelsInFlightReviewedTaskImmediately(t *testing.T){
 for _,mode:=range []string{"run","start"}{
  t.Run(mode,func(t *testing.T){
   remote:=&blockedNativeTaskRemote{
    fakeDesktopGUIRemote:&fakeDesktopGUIRemote{},
    mode:mode,started:make(chan struct{}),release:make(chan struct{}),
    canceled:make(chan struct{}),
   }
   g:=newTrustedGUI(context.Background(),remote,
    "http://127.0.0.1:42119",strings.Repeat("a",64))
   status,review:=guiRequestTest(t,g,guiRequest{Operation:"task_review",
    TaskID:1,Mode:mode},g.origin,g.key)
   if status!=200||!review.OK||review.Confirm==""{
    t.Fatalf("host-approved review failed: %d %+v",status,review)
   }
   result:=make(chan guiResponse,1)
   go func(){
    _,value:=guiRequestTest(t,g,guiRequest{
     Operation:"task_execute",Mode:mode,TaskID:1,Confirmation:review.Confirm,
    },g.origin,g.key)
    result<-value
   }()
   select{
   case <-remote.started:
   case <-time.After(2*time.Second):t.Fatal("task was not admitted")
   }
   closeDone:=make(chan int,1)
   go func(){
    recorder:=httptest.NewRecorder()
    g.handler().ServeHTTP(recorder,nativeCloseRequest(g))
    closeDone<-recorder.Code
   }()
   select{
   case code:=<-closeDone:
    if code!=http.StatusOK{t.Fatalf("Disconnect denied: %d",code)}
    if remote.closeCount.Load()!=1{
     t.Fatalf("Disconnect did not immediately revoke E2EE client: %d",remote.closeCount.Load())
    }
   case <-time.After(300*time.Millisecond):
    t.Fatal("Disconnect blocked on a running TaskDeck task")
   }
   select{
   case <-remote.canceled:
   case <-time.After(2*time.Second):
    t.Fatal("active E2EE task context was not canceled by Disconnect")
   }
   select{
   case response:=<-result:
    if response.OK||response.Text!=""{
     t.Fatalf("late task reply passed after revocation: %+v",response)
    }
   case <-time.After(2*time.Second):t.Fatal("task request did not finish after revocation")
   }
   select{
   case <-g.done:
   default:t.Fatal("native GUI lifetime was not closed")
   }
   denied,reply:=guiRequestTest(t,g,guiRequest{Operation:"tasks"},g.origin,g.key)
   if denied!=http.StatusBadRequest||reply.OK||len(remote.calls)!=1{
    t.Fatalf("closed GUI reused task authority: %d %+v calls=%v",denied,reply,remote.calls)
   }
  })
 }
}

func TestNativeGUIRejectsLateTaskSuccessEvenIfRemoteIgnoresCancellation(t *testing.T){
 remote:=&blockedNativeTaskRemote{
  fakeDesktopGUIRemote:&fakeDesktopGUIRemote{},
  started:make(chan struct{}),release:make(chan struct{}),
  canceled:make(chan struct{}),ignoreCancellation:true,
 }
 g:=newTrustedGUI(context.Background(),remote,
  "http://127.0.0.1:42119",strings.Repeat("a",64))
 _,review:=guiRequestTest(t,g,guiRequest{Operation:"task_review",
  TaskID:1,Mode:"run"},g.origin,g.key)
 done:=make(chan guiResponse,1)
 go func(){
  _,response:=guiRequestTest(t,g,guiRequest{Operation:"task_execute",
   TaskID:1,Mode:"run",Confirmation:review.Confirm},g.origin,g.key)
  done<-response
 }()
 select{
 case <-remote.started:
 case <-time.After(2*time.Second):t.Fatal("task did not start")
 }
 // Simulates explicit browser Disconnect or parent session revocation
 // while Relay is withholding the authenticated response.
 closeDone:=make(chan struct{})
 go func(){g.close();close(closeDone)}()
 select{
 case <-closeDone:
  if remote.closeCount.Load()!=1{
   t.Fatal("stalled remote kept E2EE channel alive after Disconnect")
  }
 case <-time.After(300*time.Millisecond):t.Fatal("local Close blocked on hostile Relay")
 }
 close(remote.release)
 select{
 case reply:=<-done:
  if reply.OK||reply.Text!=""||strings.Contains(reply.Error,"LATE_E2EE_TASK_SUCCESS"){
   t.Fatalf("revoked remote response appeared in native GUI: %+v",reply)
  }
 case <-time.After(2*time.Second):t.Fatal("late result failed to return")
 }
 if g.pending.id!=0{
  t.Fatal("GUI retained a one-use task review after disconnect")
 }
 g.close()
 if remote.closeCount.Load()!=1{t.Fatal("native GUI closed E2EE more than once")}
}

func TestNativeGUITaskReviewCannotLaunchAfterSessionClose(t *testing.T){
 g,remote:=newTestNativeGUI()
 _,review:=guiRequestTest(t,g,guiRequest{Operation:"task_review",TaskID:1,Mode:"run"},g.origin,g.key)
 g.close()
 status,reply:=guiRequestTest(t,g,guiRequest{
  Operation:"task_execute",TaskID:1,Mode:"run",Confirmation:review.Confirm,
 },g.origin,g.key)
 if status!=http.StatusBadRequest||reply.OK||len(remote.calls)!=1{
  t.Fatalf("revoked review started task: status=%d %+v calls=%v",status,reply,remote.calls)
 }
}


func TestNativeGUIIdleCloseDestroysClientE2EE(t *testing.T){
 g,remote:=newTestNativeGUI()
 g.close()
 g.close()
 if remote.closeCount.Load()!=1 {
  t.Fatalf("idle GUI leaked or repeatedly closed E2EE: %d",remote.closeCount.Load())
 }
 if !g.closedFlag.Load(){t.Fatal("closed native GUI did not publish revoke")}
 select{
 case <-g.done:
 default:t.Fatal("idle Disconnect failed to signal native shutdown")
 }
}
