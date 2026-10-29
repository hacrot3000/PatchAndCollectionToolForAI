package main

import (
 "errors"
 "flag"
 "fmt"
 "os"
 "strings"
 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
)

// runHostRemoteKeyCLI intentionally has no network, HTTP, Portal or Relay
// dependency. The operator must run this on the TaskDeck host under the
// SAME OS account as the TaskDeck daemon and must verify the copyable
// public key out-of-band before approving it.
func runHostRemoteKeyCLI(args []string) error {
 usage:=errors.New("usage: taskdeck remote-key [--workspace /absolute/project] list | add <taskdeck-user> <base64-ed25519-public-key> [device-label] | revoke <64-hex-fingerprint>")
 flags:=flag.NewFlagSet("remote-key",flag.ContinueOnError)
 flags.SetOutput(os.Stderr)
 workspace:=flags.String("workspace","","absolute TaskDeck workspace (host only)")
 if err:=flags.Parse(args);err!=nil{return err}
 a:=flags.Args()
 if len(a)<1{return usage}
 ws,err:=resolveWorkspace(*workspace)
 if err!=nil{return err}
 path,err:=remoteclientkey.DefaultPath(ws)
 if err!=nil{return err}
 switch a[0]{
 case "list":
  if len(a)!=1{return usage}
  keys,err:=remoteclientkey.List(path);if err!=nil{return err}
  if len(keys)==0{fmt.Println("No remote client keys approved for",ws);return nil}
  fmt.Println("Approved TaskDeck remote client keys:",ws)
  for _,entry:=range keys{
   fmt.Printf("%s  user=%s  label=%q  public_key=%s\n",entry.Fingerprint,entry.Username,entry.Label,entry.PublicKey)
  }
  return nil
 case "add":
  if len(a)<3||len(a)>4{return usage}
  label:="";if len(a)==4{label=a[3]}
  entry,err:=remoteclientkey.Add(path,a[1],a[2],label)
  if err!=nil{return err}
  fmt.Println("APPROVED:",entry.Fingerprint,"TaskDeck user:",entry.Username)
  fmt.Println("The private key must remain only on the client; never paste it into Portal.")
  return nil
 case "revoke":
  if len(a)!=2{return usage}
  if err:=remoteclientkey.Remove(path,strings.ToLower(a[1]));err!=nil{return err}
  fmt.Println("REVOKED:",strings.ToLower(a[1]))
  return nil
 default:
  return usage
 }
}
