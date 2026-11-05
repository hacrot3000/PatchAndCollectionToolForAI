package server

import (
 "context"
 "errors"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteGitMutationSlotExcludesTaskAndPTY(t *testing.T){
 _,session,_,_:=remoteDispatchFixture(t)
 release,err:=session.reserveRemoteGitMutation(context.Background())
 if err!=nil||release==nil{t.Fatalf("idle session denied: %v",err)}
 start:=time.Now()
 _,err=session.reserveRemoteGitMutation(context.Background())
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied)||
  time.Since(start)>time.Second{
  t.Fatalf("overlapping Git operation did not fail fast: %v",err)
 }
 release()
 session.longTaskMu.Lock()
 session.longTask=&remoteLongTaskJob{taskID:123}
 session.longTaskMu.Unlock()
 _,err=session.reserveRemoteGitMutation(context.Background())
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("live long job admitted Git: %v",err)
 }
 session.longTaskMu.Lock()
 session.longTask=nil
 session.longTaskMu.Unlock()
 session.terminalMu.Lock()
 session.terminal=&remoteTerminalJob{}
 session.terminalMu.Unlock()
 _,err=session.reserveRemoteGitMutation(context.Background())
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("host PTY admitted Git: %v",err)
 }
 session.terminalMu.Lock()
 session.terminal=nil
 session.terminalMu.Unlock()
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 _,err=session.reserveRemoteGitMutation(ctx)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("cancelled operation admitted Git: %v",err)
 }
 release,err=session.reserveRemoteGitMutation(context.Background())
 if err!=nil||release==nil{t.Fatalf("Git slot not released: %v",err)}
 release()
 session.Close()
 _,err=session.reserveRemoteGitMutation(context.Background())
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("revoked session admitted Git: %v",err)
 }
}
