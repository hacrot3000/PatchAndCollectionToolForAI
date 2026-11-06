//go:build linux

package server

import (
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "syscall"
)

// A PTY opens a distinct Unix session; background jobs may move into a
// DIFFERENT process group while retaining the same session ID. Killing only
// -shellPID therefore leaves detached background jobs running.
func remoteKillPTYSession(sid int){
 if sid<=1{return}
 entries,err:=os.ReadDir("/proc")
 if err!=nil{return}
 self:=os.Getpid()
 pids:=make([]int,0,8)
 for _,entry:=range entries{
  pid,err:=strconv.Atoi(entry.Name())
  if err!=nil||pid<=1||pid==self{continue}
  raw,err:=os.ReadFile(filepath.Join("/proc",entry.Name(),"stat"))
  if err!=nil{continue}
  // /proc/<pid>/stat contains '(comm)' with arbitrary spaces and ')'.
  // After the final ')': state, ppid, pgrp, session (field 6).
  idx:=strings.LastIndexByte(string(raw),')')
  if idx<0||idx+1>=len(raw){continue}
  fields:=strings.Fields(string(raw[idx+1:]))
  if len(fields)<4{continue}
  procSID,err:=strconv.Atoi(fields[3])
  if err!=nil||procSID!=sid{continue}
  pids=append(pids,pid)
 }
 // Stop *all* members, not merely the leader's process group. A background
 // shell's sleep/touch child may remain in the same session but a new pgrp.
 for _,pid:=range pids{_ = syscall.Kill(pid,syscall.SIGKILL)}
}
