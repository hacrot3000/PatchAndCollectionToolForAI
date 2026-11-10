package server

import (
 "context"
 "errors"
 "io"
 "os/exec"
 "time"
)

// Git's URL-specific HTTP settings outrank generic command-line flags.
// A repository may disable TLS verification, or redirect a fixed HTTPS
// operation to an unapproved origin. Reject all such repository-owned
// settings (including includeIf files) before starting any network request.
// System/global Git config is separately suppressed by the caller.
func remoteGitNoRepositoryHTTPOverride(parent context.Context,root string,env []string)bool{
 if parent==nil||parent.Err()!=nil||root==""||len(env)==0{return false}
 ctx,cancel:=context.WithTimeout(parent,2*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(ctx,"git",
  "-c","core.fsmonitor=false","-C",root,
  "config","--includes","--get-regexp",
  `^http(\..*)?\.(sslverify|followredirects)$`)
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
 // No matching keys is the only permitted result: Git config exit 1.
 // A matching key, timeout or malformed config all fail closed.
 return errors.As(err,&absent)&&absent.ExitCode()==1&&
  ctx.Err()==nil&&parent.Err()==nil
}
