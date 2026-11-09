package server

import (
 "context"
 "errors"
 "os"
 "path/filepath"
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

// Two independently authenticated sessions targeting the SAME canonical
// host workspace must not concurrently mutate its Git index or HEAD.
func TestRemoteGitWorkspaceMutationExcludesOtherSessions(t *testing.T){
 root:=t.TempDir()
 left:=&Server{Workspace:root}
 right:=&Server{Workspace:root}
 first:=&remoteTaskDeckSession{}
 second:=&remoteTaskDeckSession{}
 release,err:=left.reserveRemoteGitMutation(context.Background(),first)
 if err!=nil{t.Fatalf("first workspace reservation failed: %v",err)}
 began:=time.Now()
 _,err=right.reserveRemoteGitMutation(context.Background(),second)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied)||
  time.Since(began)>time.Second{
  t.Fatalf("second authenticated session overlapped Git mutation: %v",err)
 }
 // Locking one repo must not accidentally lock a different workspace.
 unrelated:=&Server{Workspace:t.TempDir()}
 free,err:=unrelated.reserveRemoteGitMutation(context.Background(),second)
 if err!=nil{t.Fatalf("unrelated repository was unnecessarily blocked: %v",err)}
 free()
 release()
 release() // safe even if two teardown paths both attempt cleanup
 acquired,err:=right.reserveRemoteGitMutation(context.Background(),second)
 if err!=nil{t.Fatalf("released Git workspace remained busy: %v",err)}
 acquired()
}

func TestRemoteGitWorkspaceMutationCanonicalSymlinkAlias(t *testing.T){
 root:=t.TempDir()
 alias:=filepath.Join(t.TempDir(),"workspace-link")
 if err:=os.Symlink(root,alias);err!=nil{
  t.Skipf("filesystem does not allow a symlink alias: %v",err)
 }
 direct:=&Server{Workspace:root}
 linked:=&Server{Workspace:alias}
 owner:=&remoteTaskDeckSession{}
 other:=&remoteTaskDeckSession{}
 release,err:=direct.reserveRemoteGitMutation(context.Background(),owner)
 if err!=nil{t.Fatal(err)}
 _,err=linked.reserveRemoteGitMutation(context.Background(),other)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatal("same workspace symlink escaped cross-session Git exclusion")
 }
 release()
 next,err:=linked.reserveRemoteGitMutation(context.Background(),other)
 if err!=nil{t.Fatal(err)}
 next()
}
