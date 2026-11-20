package main

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "strconv"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/restrictedpolicy"
)

// Restricted mode is a process launch capability, not an INI/web preference.
// Child processes inherit a canonical, pinned copy of the policy via argv,
// NOT a mutable file path. Changes require an explicit new launch.
type restrictedCLIOptions struct {
 Enabled bool
 File string
 VisibleLines int
 CopyLines int
 CopyBytes int
 PerFileLines int
 PerDayLines int
 RevealPercent int
 Commands string
 Fingerprint string // Internal: fail closed if forwarded arguments are altered.
}
var restrictedDaemonArgs []string

func applyRestrictedStartup(cfg *config.Config, o restrictedCLIOptions) error {
 restrictedDaemonArgs=nil
 if !o.Enabled {
  if o.File!=""||o.VisibleLines>=0||o.CopyLines>=0||o.CopyBytes>=0||
   o.PerFileLines>=0||o.PerDayLines>=0||o.RevealPercent>=0||o.Commands!=""||o.Fingerprint!="" {
   return fmt.Errorf("restricted options require --restricted; restricted mode cannot be enabled from web settings")
  }
  return nil
 }
 p:=restrictedpolicy.Default()
 if o.File!="" {
  loaded,err:=restrictedpolicy.Load(o.File)
  if err!=nil{return fmt.Errorf("load --restricted-policy: %w",err)}
  p=loaded
 }
 if o.VisibleLines>=0 {p.MaxVisibleLines=o.VisibleLines}
 if o.CopyLines>=0 {p.MaxCopyLines=o.CopyLines}
 if o.CopyBytes>=0 {p.MaxCopyBytes=o.CopyBytes}
 if o.PerFileLines>=0 {p.MaxUniqueLinesPerFile=o.PerFileLines}
 if o.PerDayLines>=0 {p.MaxUniqueLinesPerDay=o.PerDayLines}
 if o.RevealPercent>=0 {p.MaxRevealPercent=o.RevealPercent}
 if o.Commands!="" {
  if o.Commands=="none" {p.AllowedCommands=nil
  }else {p.AllowedCommands=strings.Split(o.Commands,",")}
 }
 if err:=p.Validate();err!=nil{return fmt.Errorf("restricted policy validation: %w",err)}
 blob,err:=json.Marshal(p)
 if err!=nil{return err}
 sum:=sha256.Sum256(blob)
 fingerprint:=hex.EncodeToString(sum[:])
 if o.Fingerprint!="" && o.Fingerprint!=fingerprint {
  return fmt.Errorf("restricted startup policy fingerprint mismatch: refusing to start with changed permissions")
 }
 copy:=p
 copy.AllowedCommands=append([]string(nil),p.AllowedCommands...)
 cfg.Restricted=&copy
 // Canonical argv avoids reading a mutable policy file during daemon
 // spawning, config reload or self-update socket handoffs.
 restrictedDaemonArgs=[]string{
  "--restricted",
  "--restricted-visible-lines",strconv.Itoa(p.MaxVisibleLines),
  "--restricted-copy-lines",strconv.Itoa(p.MaxCopyLines),
  "--restricted-copy-bytes",strconv.Itoa(p.MaxCopyBytes),
  "--restricted-per-file-lines",strconv.Itoa(p.MaxUniqueLinesPerFile),
  "--restricted-per-day-lines",strconv.Itoa(p.MaxUniqueLinesPerDay),
  "--restricted-reveal-percent",strconv.Itoa(p.MaxRevealPercent),
  "--restricted-commands",strings.Join(p.AllowedCommands,","),
  "--restricted-fingerprint",fingerprint,
 }
 if len(p.AllowedCommands)==0 {restrictedDaemonArgs[len(restrictedDaemonArgs)-3]="none"}
 return nil
}
