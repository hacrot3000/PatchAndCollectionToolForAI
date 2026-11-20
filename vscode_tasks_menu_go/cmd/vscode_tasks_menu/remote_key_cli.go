package main

import (
 "errors"
 "context"
 "time"
 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/identity"
 "flag"
 "fmt"
 "os"
 "strings"
 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
 "bletonfc/vscode_tasks_menu/internal/remoteportal"
)

// trustedHostRemoteUser resolves the immutable SQLite user ID and active
// membership through TaskDeck's *local* store, never from a PHP ticket.
// It refuses a missing DB so an approval cannot accidentally create a fresh
// identity database or pre-authorize a future username.
func trustedHostRemoteUser(workspace,username string)(string,string,error){
 cfg,_,err:=config.Load(workspace)
 if err!=nil{return "","",err}
 if !cfg.SharedServerEnabled||!cfg.TLS()||cfg.SharedProjectID==""{
  return "","",errors.New("client key approval requires configured local shared-server identity")
 }
 dbPath,err:=identity.ResolveDBPathForWorkspace(cfg.SharedIdentityDB,workspace)
 if err!=nil{return "","",err}
 if _,err:=os.Stat(dbPath);err!=nil{return "","",fmt.Errorf("existing local TaskDeck identity DB required: %w",err)}
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
 defer cancel()
 store,err:=identity.OpenSQLiteStore(ctx,dbPath)
 if err!=nil{return "","",err}
 defer store.Close()
 user,err:=store.UserByUsername(ctx,username)
 if err!=nil||!user.Enabled{return "","",errors.New("active TaskDeck local account required")}
 project,err:=store.ProjectByKey(ctx,cfg.SharedProjectID)
 if err!=nil||!project.Enabled{return "","",errors.New("active TaskDeck project required")}
 member,err:=store.ProjectMember(ctx,project.ID,user.ID)
 if err!=nil||!member.Enabled{return "","",errors.New("active TaskDeck local project membership required")}
 return user.Username,string(user.ID),nil
}

// runHostRemoteKeyCLI intentionally has no network, HTTP, Portal or Relay
// dependency. The operator must run this on the TaskDeck host under the
// SAME OS account as the TaskDeck daemon and must verify the copyable
// public key out-of-band before approving it.
func runHostRemoteKeyCLI(args []string) error {
 usage:=errors.New("usage: taskdeck remote-key [--workspace /absolute/project] host-key | list | add <taskdeck-user> <base64-ed25519-public-key> [device-label] | revoke <64-hex-fingerprint>")
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
 case "host-key":
  if len(a)!=1{return usage}
  // Never generate a new Agent identity as a side effect of showing the
  // public key; a missing seed requires owner-controlled recovery.
  seedPath,err:=remoteportal.DefaultIdentityPath(ws)
  if err!=nil{return err}
  identity,err:=remoteportal.LoadExistingIdentity(seedPath)
  if err!=nil{return fmt.Errorf("existing owner-controlled TaskDeck Agent key required; do not trust any Portal-supplied replacement: %w",err)}
  fmt.Println("TaskDeck host Agent public key (Ed25519, base64):",identity.PublicKeyBase64())
  fmt.Println("TaskDeck Device ID (SHA-256 of raw Agent public key):",identity.DeviceID())
  fmt.Println("Verify and pin on a separately installed native client or extension; NEVER enter TaskDeck private keys or password into PHP.")
  return nil
 case "list":
  if len(a)!=1{return usage}
  keys,err:=remoteclientkey.List(path);if err!=nil{return err}
  if len(keys)==0{fmt.Println("No remote client keys approved for",ws);return nil}
  fmt.Println("Approved TaskDeck remote client keys:",ws)
  for _,entry:=range keys{
   fmt.Printf("%s  user=%s  user_id=%s  label=%q  public_key=%s\n",entry.Fingerprint,entry.Username,entry.UserID,entry.Label,entry.PublicKey)
  }
  return nil
 case "add":
  if len(a)<3||len(a)>4{return usage}
  label:="";if len(a)==4{label=a[3]}
  canonicalUsername,userID,err:=trustedHostRemoteUser(ws,a[1])
  if err!=nil{return err}
  entry,err:=remoteclientkey.AddForUser(path,canonicalUsername,userID,a[2],label)
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
