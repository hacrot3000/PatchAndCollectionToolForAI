package main

import (
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
)

func defaultRestrictedOptions() restrictedCLIOptions {
 return restrictedCLIOptions{
  VisibleLines:-1,CopyLines:-1,CopyBytes:-1,PerFileLines:-1,
  PerDayLines:-1,RevealPercent:-1,
 }
}
func TestRestrictedStartupIsOptInAndImmutable(t *testing.T) {
 opts:=defaultRestrictedOptions()
 c:=config.Default()
 if err:=applyRestrictedStartup(&c,opts);err!=nil {t.Fatal(err)}
 if c.Restricted!=nil || len(restrictedDaemonArgs)!=0 {t.Fatal("restricted mode is not opt-in")}
 opts.VisibleLines=50
 if err:=applyRestrictedStartup(&c,opts);err==nil {t.Fatal("limits accepted without --restricted")}
 opts.Enabled=true
 if err:=applyRestrictedStartup(&c,opts);err!=nil {t.Fatal(err)}
 if c.Restricted==nil||c.Restricted.MaxVisibleLines!=50 {t.Fatal("limit not pinned to server config")}
 if len(restrictedDaemonArgs)<10||restrictedDaemonArgs[0]!="--restricted" {
  t.Fatalf("restricted flags not propagated to subprocess: %v",restrictedDaemonArgs)
 }
 var fp string
 for i,x:=range restrictedDaemonArgs {
  if x=="--restricted-fingerprint"&&i+1<len(restrictedDaemonArgs) {fp=restrictedDaemonArgs[i+1]}
 }
 if len(fp)!=64 {t.Fatalf("bad fingerprint %q",fp)}
 opts.Fingerprint=fp
 if err:=applyRestrictedStartup(&c,opts);err!=nil {t.Fatal("matching fingerprint rejected:",err)}
 opts.VisibleLines=51
 if err:=applyRestrictedStartup(&c,opts);err==nil {t.Fatal("changed startup config passed original fingerprint")}
}
func TestRestrictedStartupNeverAllowsUnsafeGroupsOrNoCommandsAmbiguity(t *testing.T) {
 opts:=defaultRestrictedOptions();opts.Enabled=true;opts.Commands="none"
 c:=config.Default()
 if err:=applyRestrictedStartup(&c,opts);err!=nil {t.Fatal(err)}
 if len(c.Restricted.AllowedCommands)!=0||!strings.Contains(strings.Join(restrictedDaemonArgs," "),"--restricted-commands none"){
  t.Fatalf("empty allowlist not canonicalized: %#v",restrictedDaemonArgs)
 }
 opts.Commands="git,ls;sh"
 if err:=applyRestrictedStartup(&c,opts);err==nil {t.Fatal("injected command group accepted")}
}
