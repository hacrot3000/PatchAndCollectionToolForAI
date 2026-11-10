package server

import (
 "context"
 "errors"
 "io"
 "os/exec"
 "time"
)

// Git's URL-specific http.<url>.sslVerify outranks a generic
// command-line http.sslVerify=true. A repository can therefore force
// HTTPS verification OFF even when the fixed Remote action uses HTTPS.
//
// Refuse all repo-local TLS verification overrides (including includeIf
// files) BEFORE any network action. System/global config and inherited
// Git environment are separately disabled by callers. This is
// intentionally conservative; administrators should install their
// approved CA using http.sslCAInfo without disabling verification.
func remoteGitNoRepositoryTLSOverride(parent context.Context,root string,env []string)bool{
 if parent==nil||parent.Err()!=nil||root==""||len(env)==0{return false}
 ctx,cancel:=context.WithTimeout(parent,2*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(ctx,"git",
  "-c","core.fsmonitor=false","-C",root,
  "config","--includes","--get-regexp",
  `^http(\..*)?\.sslverify$`)
 cmd.Dir=root
 cmd.Env=env
 cmd.Stdout=io.Discard
 cmd.Stderr=io.Discard
 prepareGitJobProcess(cmd)
 cmd.Cancel=func()error{killGitJobProcess(cmd);return nil}
 cmd.WaitDelay=time.Second
 err:=cmd.Run()
 killGitJobProcess(cmd)
 var absent *exec.ExitError
 // Exit 1 means no matching config keys. Any match or Git error must
 // fail closed rather than allowing a repo-owned TLS downgrade.
 return errors.As(err,&absent)&&absent.ExitCode()==1&&
  ctx.Err()==nil&&parent.Err()==nil
}
