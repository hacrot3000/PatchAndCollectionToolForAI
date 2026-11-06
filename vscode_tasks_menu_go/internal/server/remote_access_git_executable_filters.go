package server

import (
 "context"
 "errors"
 "io"
 "os/exec"
 "time"
)

// A repository's local filter.*.clean/smudge/process config can launch
// arbitrary processes during "git status", "git commit" and fast-forward
// checkout even when all Git hooks are disabled. Do not allow a remote
// Git mutation to reach any status-like command until this closed-set
// executable-driver scan succeeds. Git config --includes considers
// include.path / includeIf local configuration as well.
// This check is conservative: repositories with custom executable filters
// cannot use these remote operations; PHP and the E2EE client cannot
// change the guard or pass alternative Git argv.
func remoteGitNoExecutableFilters(parent context.Context,root string,env []string)bool{
 if parent==nil||parent.Err()!=nil||root==""||len(env)==0{return false}
 ctx,cancel:=context.WithTimeout(parent,2*time.Second)
 defer cancel()
 command:=exec.CommandContext(ctx,"git",
  "-c","core.fsmonitor=false","-C",root,
  "config","--includes","--get-regexp",
  `^filter\..*\.(clean|smudge|process)$`)
 command.Dir=root
 command.Env=env
 command.Stdout=io.Discard
 command.Stderr=io.Discard
 prepareGitJobProcess(command)
 command.Cancel=func()error{killGitJobProcess(command);return nil}
 command.WaitDelay=time.Second
 err:=command.Run()
 killGitJobProcess(command)
 var absent *exec.ExitError
 // No matches is the ONLY success: git config exits 1. Matches, a
 // timeout, process failure or any other exit code all fail closed.
 return errors.As(err,&absent)&&absent.ExitCode()==1&&
  ctx.Err()==nil&&parent.Err()==nil
}
