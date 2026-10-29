package server

import (
    "context"
    "errors"
    "sync/atomic"
    "testing"
    "time"

    "bletonfc/vscode_tasks_menu/internal/config"
    "bletonfc/vscode_tasks_menu/internal/identity"
    "bletonfc/vscode_tasks_menu/internal/remoteportal"
    "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func testRemoteExecutionSession(t *testing.T) (*Server,*remoteTaskDeckSession,*remotetransport.Channel) {
    t.Helper()
    s:=sharedLoginTestServer(t)
    s.Config.RemoteAccessEnabled=true
    s.Config.Protocol=config.ProtocolHTTPS
    ch:=remoteSessionTestChannel(t)
    session,err:=s.authenticateRemoteTaskDeckCredentials(
        context.Background(),ch,
        testRemoteSignedGrant(t,ch,remotetransport.RoleViewer),
        "alice","private-admin-password")
    if err!=nil {t.Fatal(err)}
    t.Cleanup(session.Close)
    allowTestLivePortal(session)
    return s,session,ch
}

func TestRemoteActionExecutionRejectsPortalOnlyAndUnauthorizedActions(t *testing.T) {
    s,session,ch:=testRemoteExecutionSession(t)
    ctx:=context.Background()
    var called atomic.Int32
    cb:=func(context.Context)error {called.Add(1);return nil}
    for _,tc:=range []struct{
        name string
        session *remoteTaskDeckSession
        channel *remotetransport.Channel
        permission string
        action func(context.Context)error
    }{
        {"Portal ticket without TaskDeck login",nil,ch,identity.PermissionFilesRead,cb},
        {"wrong authenticated channel",session,remoteSessionTestChannel(t),identity.PermissionFilesRead,cb},
        {"viewer cannot execute terminal",session,ch,identity.PermissionTerminalCreate,cb},
        {"unsupported permission",session,ch,"remote.superuser",cb},
        {"missing executor",session,ch,identity.PermissionFilesRead,nil},
    }{
        t.Run(tc.name,func(t *testing.T){
            err:=s.executeRemoteAuthenticatedAction(ctx,tc.session,tc.channel,tc.permission,tc.action)
            if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
                t.Fatalf("untrusted action was admitted: %v",err)
            }
        })
    }
    if called.Load()!=0 {t.Fatal("unauthorized action callback executed")}
    // There is no application grant without fresh Portal introspection.
    session.portalCheckMu.Lock()
    session.livePortalCheck=nil
    session.portalCheckMu.Unlock()
    if err:=s.executeRemoteAuthenticatedAction(ctx,session,ch,
        identity.PermissionFilesRead,cb);err==nil{
        t.Fatal("compromised signed Portal ticket bypassed fresh live-check gate")
    }
    if called.Load()!=0 {t.Fatal("missing live grant invoked callback")}
}

func TestRemoteActionExecutionOneActiveOperationAndCloseCancelsIt(t *testing.T) {
    s,session,ch:=testRemoteExecutionSession(t)
    started:=make(chan struct{})
    exited:=make(chan struct{})
    finished:=make(chan error,1)
    go func(){
        finished<-s.executeRemoteAuthenticatedAction(context.Background(),session,ch,
            identity.PermissionFilesRead,func(ctx context.Context)error{
                close(started)
                <-ctx.Done()
                close(exited)
                return nil // revoked work cannot claim successful completion
            })
    }()
    select {
    case <-started:
    case <-time.After(5*time.Second):t.Fatal("remote action was not admitted")
    }
    var secondCalled atomic.Bool
    if err:=s.executeRemoteAuthenticatedAction(context.Background(),session,ch,
        identity.PermissionFilesRead,func(context.Context)error{
            secondCalled.Store(true);return nil
        });!errors.Is(err,remotetransport.ErrRemoteActionDenied){
        t.Fatalf("parallel action bypassed one-slot limit: %v",err)
    }
    if secondCalled.Load(){t.Fatal("concurrent remote action executed")}
    session.Close()
    select {
    case <-exited:
    case <-time.After(time.Second):t.Fatal("session close failed to cancel active work")
    }
    select {
    case err:=<-finished:
        if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
            t.Fatalf("revoked action returned success: %v",err)
        }
    case <-time.After(time.Second):t.Fatal("canceled execution did not terminate")
    }
    if ch.Established(){t.Fatal("revocation left Agent E2EE channel established")}
    if err:=s.executeRemoteAuthenticatedAction(context.Background(),session,ch,
        identity.PermissionFilesRead,func(context.Context)error{
            t.Error("closed session executed command")
            return nil
        });err==nil{t.Fatal("closed session admitted new action")}
}

func TestRemoteActionExecutionClosesDuringSlowPortalIntrospection(t *testing.T) {
    s,session,ch:=testRemoteExecutionSession(t)
    entered:=make(chan struct{})
    release:=make(chan struct{})
    session.portalCheckMu.Lock()
    session.livePortalCheck=func(context.Context,*remoteportal.AgentGrant)error{
        close(entered)
        <-release
        return nil // forged stale allowed=true after local Close
    }
    session.portalCheckMu.Unlock()
    var called atomic.Bool
    result:=make(chan error,1)
    go func(){
        result<-s.executeRemoteAuthenticatedAction(context.Background(),session,ch,
            identity.PermissionFilesRead,func(context.Context)error{
                called.Store(true);return nil
            })
    }()
    select {
    case <-entered:
    case <-time.After(5*time.Second):t.Fatal("async Portal check did not begin")
    }
    // Close MUST not wait for the compromised Portal to release its reply.
    closed:=make(chan struct{})
    go func(){session.Close();close(closed)}()
    select {
    case <-closed:
    case <-time.After(time.Second):t.Fatal("Portal blocked local revocation")
    }
    close(release)
    select {
    case err:=<-result:
        if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
            t.Fatalf("stale Portal response admitted action: %v",err)
        }
    case <-time.After(5*time.Second):t.Fatal("late Portal response hung admission")
    }
    if called.Load(){t.Fatal("action callback ran after revocation")}
}

func TestRemoteActionExecutionRejectsRevocationDuringCallbackAndTimeout(t *testing.T) {
    t.Run("grant revoked inside callback",func(t *testing.T){
        s,session,ch:=testRemoteExecutionSession(t)
        err:=s.executeRemoteAuthenticatedAction(context.Background(),session,ch,
            identity.PermissionFilesRead,func(context.Context)error{
                session.portalGrant.Close()
                return nil
            })
        if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
            t.Fatalf("revoked grant produced successful completion: %v",err)
        }
    })
    t.Run("deadline before completion",func(t *testing.T){
        s,session,ch:=testRemoteExecutionSession(t)
        ctx,cancel:=context.WithTimeout(context.Background(),20*time.Millisecond)
        defer cancel()
        err:=s.executeRemoteAuthenticatedAction(ctx,session,ch,
            identity.PermissionFilesRead,func(ctx context.Context)error{
                <-ctx.Done()
                return nil
            })
        if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
            t.Fatalf("timed-out action succeeded: %v",err)
        }
    })
    t.Run("each action must recheck Portal",func(t *testing.T){
        s,session,ch:=testRemoteExecutionSession(t)
        var calls atomic.Int32
        var revoke atomic.Bool
        session.portalCheckMu.Lock()
        session.livePortalCheck=func(context.Context,*remoteportal.AgentGrant)error{
            calls.Add(1)
            if revoke.Load(){return remotetransport.ErrRemoteActionDenied}
            return nil
        }
        session.portalCheckMu.Unlock()
        var invoked atomic.Int32
        for i:=0;i<2;i++ {
            if err:=s.executeRemoteAuthenticatedAction(context.Background(),session,ch,
                identity.PermissionFilesRead,func(context.Context)error{
                    invoked.Add(1);return nil
                });err!=nil {
                t.Fatalf("authorized sequential callback %d rejected: %v",i,err)
            }
        }
        revoke.Store(true)
        if err:=s.executeRemoteAuthenticatedAction(context.Background(),session,ch,
            identity.PermissionFilesRead,func(context.Context)error{
                invoked.Add(1);return nil
            });!errors.Is(err,remotetransport.ErrRemoteActionDenied){
            t.Fatalf("revoked Portal grant authorized execution: %v",err)
        }
        if calls.Load()!=3 || invoked.Load()!=2 {
            t.Fatalf("live checks/actions: checks=%d calls=%d",calls.Load(),invoked.Load())
        }
    })
}
