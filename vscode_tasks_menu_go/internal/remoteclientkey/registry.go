// Package remoteclientkey is a TaskDeck-HOST-ONLY client approval registry.
// PHP Portal, Relay and browser tickets have no write interface to this file.
package remoteclientkey

import (
 "bytes"
 "crypto/ed25519"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"
 "syscall"
 "time"
)

const Version=1
const LoginProofDomain="taskdeck-remote-client-login-v1"
var ErrUnapproved=errors.New("remote client public key is not approved on this TaskDeck host")

type Entry struct {
 Username string `json:"username"`
 // Stable TaskDeck SQLite users.id. Empty only for legacy pre-migration keys;
 // such entries may be displayed but MUST NOT authorize application login.
 UserID string `json:"user_id,omitempty"`
 PublicKey string `json:"public_key"`
 Fingerprint string `json:"fingerprint"`
 Label string `json:"label"`
 AddedAt int64 `json:"added_at"`
}
type registry struct {
 Version int `json:"version"`
 Keys []Entry `json:"keys"`
}

// DefaultPath is separate from the project and PHP/webroot. The host TaskDeck
// daemon and host-only CLI resolve the same path for this workspace.
func DefaultPath(workspace string)(string,error){
 if !filepath.IsAbs(workspace){return "",errors.New("absolute TaskDeck workspace required")}
 base,err:=os.UserConfigDir();if err!=nil{return "",err}
 digest:=sha256.Sum256([]byte(filepath.Clean(workspace)))
 return filepath.Join(base,"taskdeck","remote-identities",hex.EncodeToString(digest[:])+".clients.json"),nil
}
func validLocalUserID(value string)bool{
 if len(value)==0||len(value)>128{return false}
 for _,c:=range value{if c<32||c==127{return false}}
 return true
}
func Fingerprint(encoded string)(string,error){
 raw,err:=base64.StdEncoding.Strict().DecodeString(encoded)
 if err!=nil || len(raw)!=ed25519.PublicKeySize || base64.StdEncoding.EncodeToString(raw)!=encoded {
  return "",errors.New("expected canonical base64 Ed25519 public key (32 bytes)")
 }
 sum:=sha256.Sum256(raw)
 return hex.EncodeToString(sum[:]),nil
}
// FoldASCIIUsername matches SQLite's built-in COLLATE NOCASE: only
// ASCII A-Z are folded. Go strings.EqualFold and strings.ToLower also fold
// Unicode (e.g. Kelvin sign U+212A versus 'K'), although SQLite NOCASE
// regards those as DIFFERENT TaskDeck accounts. Never use Unicode folding
// to bind a host-approved client key to a local TaskDeck account.
func FoldASCIIUsername(name string)string{
 b:=[]byte(name)
 for i,c:=range b{
  if c>='A'&&c<='Z'{b[i]=c+('a'-'A')}
 }
 return string(b)
}
func SameTaskDeckUsername(left,right string)bool{
 return FoldASCIIUsername(left)==FoldASCIIUsername(right)
}
func validateUser(name string)error{
 if len(name)==0||len(name)>128||strings.TrimSpace(name)!=name{return errors.New("invalid local TaskDeck username")}
 for _,char:=range name{if char<32||char==127{return errors.New("invalid local TaskDeck username")}}
 return nil
}
func privateInfo(info os.FileInfo,dir bool)bool{
 if info==nil||info.Mode().Perm()&0077!=0{return false}
 stat,ok:=info.Sys().(*syscall.Stat_t)
 if !ok||stat.Uid!=uint32(os.Geteuid()){return false}
 if dir{return info.IsDir()}
 return info.Mode().IsRegular() && info.Size()<=65536
}
func checkDir(dir string,create bool)error{
 if create {
  if err:=os.MkdirAll(dir,0700);err!=nil{return err}
 }
 info,err:=os.Lstat(dir)
 if err!=nil{return err}
 if !privateInfo(info,true){return errors.New("host client-key directory must be owner-only, not symlink")}
 return nil
}
func openRegular(path string)(*os.File,error){
 fd,err:=syscall.Open(path,syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK,0)
 if err!=nil{return nil,err}
 f:=os.NewFile(uintptr(fd),path)
 info,err:=f.Stat()
 if err!=nil||!privateInfo(info,false){f.Close();return nil,errors.New("invalid host client-key file owner, type or permissions")}
 return f,nil
}
// validateRegistryJSON rejects duplicate/ambiguous object members at every
// level. encoding/json accepts duplicate and case-folded struct keys, which
// makes an owner-only allowlist ambiguous if a future exporter or recovery
// mechanism ever writes malformed data. Corrupt local policy fails closed.
func validateRegistryJSON(raw []byte)error{
 bad:=errors.New("ambiguous or malformed host approved-key registry")
 dec:=json.NewDecoder(bytes.NewReader(raw))
 start,err:=dec.Token()
 if err!=nil || start!=json.Delim('{'){return bad}
 rootKeys:=map[string]bool{"version":false,"keys":false}
 for dec.More(){
  token,e:=dec.Token()
  name,ok:=token.(string)
  used,known:=rootKeys[name]
  if e!=nil || !ok || !known || used{return bad}
  rootKeys[name]=true
  if name=="version" {
   var version json.Number
   dec.UseNumber()
   if dec.Decode(&version)!=nil{return bad}
   continue
  }
  opened,e:=dec.Token()
  if e!=nil || opened!=json.Delim('['){return bad}
  for dec.More(){
   item,e:=dec.Token()
   if e!=nil || item!=json.Delim('{'){return bad}
   fields:=map[string]bool{
    "username":false,"public_key":false,"fingerprint":false,
    "label":false,"added_at":false,
   }
   userIDSeen:=false
   for dec.More(){
    nameToken,e:=dec.Token()
    key,ok:=nameToken.(string)
    used,known:=fields[key]
    if key=="user_id"{
     if e!=nil || !ok || userIDSeen{return bad}
     userIDSeen=true
    } else {
     if e!=nil || !ok || !known || used{return bad}
     fields[key]=true
    }
    if key=="added_at" {
     var timestamp json.Number
     if dec.Decode(&timestamp)!=nil{return bad}
    } else {
     var value string
     if dec.Decode(&value)!=nil{return bad}
    }
   }
   closed,e:=dec.Token()
   if e!=nil || closed!=json.Delim('}'){return bad}
   for _,present:=range fields{if !present{return bad}}
  }
  closed,e:=dec.Token()
  if e!=nil || closed!=json.Delim(']'){return bad}
 }
 closed,e:=dec.Token()
 if e!=nil || closed!=json.Delim('}'){return bad}
 for _,present:=range rootKeys{if !present{return bad}}
 if _,e:=dec.Token();e!=io.EOF{return bad}
 return nil
}

func read(path string)(registry,error){
 result:=registry{Version:Version,Keys:[]Entry{}}
 file,err:=openRegular(path)
 if errors.Is(err,os.ErrNotExist){return result,nil}
 if err!=nil{return result,err}
 defer file.Close()
 raw,err:=io.ReadAll(io.LimitReader(file,65537))
 if err!=nil{return result,err}
 if len(raw)==0||len(raw)>65536{return result,errors.New("invalid host approved-key registry length")}
 if err:=validateRegistryJSON(raw);err!=nil{return result,err}
 dec:=json.NewDecoder(bytes.NewReader(raw));dec.DisallowUnknownFields()
 if err=dec.Decode(&result);err!=nil{return result,err}
 var trailing interface{}
 if err=dec.Decode(&trailing);err!=io.EOF{return result,errors.New("trailing data in host approved-key registry")}
 if result.Version!=Version || len(result.Keys)>256{return result,errors.New("corrupt host approved-key registry")}
 found:=map[string]bool{}
 for _,e:=range result.Keys{
  id,err:=Fingerprint(e.PublicKey)
  if err!=nil||id!=e.Fingerprint||validateUser(e.Username)!=nil||
   (e.UserID!=""&&(!validLocalUserID(e.UserID)))||len(e.Label)>128||e.AddedAt<=0 {
   return result,errors.New("corrupt host approved-key entry")
  }
  key:=FoldASCIIUsername(e.Username)+"/"+id
  if found[key]{return result,errors.New("duplicate host approved-key entry")}
  found[key]=true
 }
 return result,nil
}
func lock(path string,write bool)(*os.File,error){
 dir:=filepath.Dir(path)
 if err:=checkDir(dir,write);err!=nil{return nil,err}
 lockPath:=path+".lock"
 fd,err:=syscall.Open(lockPath,syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0600)
 if err!=nil{return nil,err}
 f:=os.NewFile(uintptr(fd),lockPath)
 info,err:=f.Stat()
 if err!=nil||!privateInfo(info,false){f.Close();return nil,errors.New("unsafe approved-key lock file")}
 mode:=syscall.LOCK_SH;if write{mode=syscall.LOCK_EX}
 if err:=syscall.Flock(int(f.Fd()),mode);err!=nil{f.Close();return nil,err}
 return f,nil
}
func save(path string,data registry)error{
 raw,err:=json.MarshalIndent(data,"","  ");if err!=nil{return err}
 if len(raw)>65536{return errors.New("too many approved keys")}
 dir:=filepath.Dir(path)
 f,err:=os.CreateTemp(dir,".client-keys-*")
 if err!=nil{return err}
 defer os.Remove(f.Name())
 if err=f.Chmod(0600);err!=nil{f.Close();return err}
 if _,err=f.Write(raw);err!=nil{f.Close();return err}
 if err=f.Sync();err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 if err=os.Rename(f.Name(),path);err!=nil{return err}
 directory,err:=os.Open(dir);if err!=nil{return err}
 defer directory.Close()
 return directory.Sync()
}
func List(path string)([]Entry,error){
 l,err:=lock(path,false);if os.IsNotExist(err){return []Entry{},nil}
 if err!=nil{return nil,err};defer l.Close()
 r,err:=read(path);if err!=nil{return nil,err}
 return r.Keys,nil
}
func Add(path,username,publicKey,label string)(Entry,error){
 var e Entry
 if err:=validateUser(username);err!=nil{return e,err}
 fp,err:=Fingerprint(publicKey);if err!=nil{return e,err}
 if len(label)>128{return e,errors.New("invalid client label")}
 for _,char:=range label{if char<32||char==127{return e,errors.New("invalid client label")}}
 l,err:=lock(path,true);if err!=nil{return e,err};defer l.Close()
 r,err:=read(path);if err!=nil{return e,err}
 for _,current:=range r.Keys{
  if current.Fingerprint==fp {
   if SameTaskDeckUsername(current.Username,username){return current,nil}
   return e,errors.New("public key already approved for a different TaskDeck user")
  }
 }
 if len(r.Keys)>=256{return e,errors.New("approved-key registry limit reached")}
 e=Entry{Username:username,PublicKey:publicKey,Fingerprint:fp,Label:label,AddedAt:time.Now().Unix()}
 r.Keys=append(r.Keys,e)
 return e,save(path,r)
}
// AddForUser is the ONLY production host-CLI enrollment path. The CLI
// resolves local SQLite user ID and active project membership *on the host*.
// An old username-only approval cannot silently authorize a newly created
// TaskDeck identity that inherited the same username.
func AddForUser(path,username,userID,publicKey,label string)(Entry,error){
 var empty Entry
 if validateUser(username)!=nil||!validLocalUserID(userID) {return empty,errors.New("valid local user identity required")}
 fingerprint,err:=Fingerprint(publicKey);if err!=nil{return empty,err}
 if len(label)>128{return empty,errors.New("invalid client label")}
 for _,r:=range label{if r<32||r==127{return empty,errors.New("invalid client label")}}
 l,err:=lock(path,true);if err!=nil{return empty,err};defer l.Close()
 r,err:=read(path);if err!=nil{return empty,err}
 for i,entry:=range r.Keys {
  if entry.Fingerprint==fingerprint {
   if !SameTaskDeckUsername(entry.Username,username){return empty,errors.New("public key belongs to another local TaskDeck username")}
   if entry.UserID!="" && entry.UserID!=userID {
    return empty,errors.New("client key was approved for a different immutable TaskDeck account; revoke it first")
   }
   if entry.UserID==userID{return entry,nil}
   // Explicit host CLI re-approval migrates a legacy username-only key.
   entry.UserID=userID
   entry.Username=username
   entry.Label=label
   entry.AddedAt=time.Now().Unix()
   r.Keys[i]=entry
   return entry,save(path,r)
  }
 }
 if len(r.Keys)>=256{return empty,errors.New("approved-key registry limit reached")}
 entry:=Entry{Username:username,UserID:userID,PublicKey:publicKey,
  Fingerprint:fingerprint,Label:label,AddedAt:time.Now().Unix()}
 r.Keys=append(r.Keys,entry)
 return entry,save(path,r)
}

// IsApprovedForUser is mandatory at the TaskDeck-local identity gate and
// before every subsequent app action; username alone is insufficient.
func IsApprovedForUser(path,username,userID,publicKey string)error{
 if validateUser(username)!=nil||!validLocalUserID(userID){return ErrUnapproved}
 fingerprint,err:=Fingerprint(publicKey);if err!=nil{return ErrUnapproved}
 keys,err:=List(path);if err!=nil{return err}
 for _,key:=range keys{
  if key.Fingerprint==fingerprint&&key.PublicKey==publicKey&&
   key.UserID==userID&&SameTaskDeckUsername(key.Username,username){return nil}
 }
 return ErrUnapproved
}

func Remove(path,fingerprint string)error{
 if len(fingerprint)!=64{return errors.New("invalid key fingerprint")}
 if _,err:=hex.DecodeString(fingerprint);err!=nil{return err}
 l,err:=lock(path,true);if err!=nil{return err};defer l.Close()
 r,err:=read(path);if err!=nil{return err}
 for i,e:=range r.Keys{
  if e.Fingerprint==fingerprint{
   r.Keys=append(r.Keys[:i],r.Keys[i+1:]...)
   return save(path,r)
  }
 }
 return fmt.Errorf("approved client key not found")
}
// Verify checks BOTH an owner-approved public key for the requested local
// username AND possession of the matching private key for this fresh channel.
// The signed message must contain the authenticated E2EE transcript binding;
// caller must supply it from Channel, NEVER from Portal or Relay JSON.
func Verify(path,username,publicKey,signature string,channelBinding []byte)error{
 if validateUser(username)!=nil||len(channelBinding)!=32{return ErrUnapproved}
 fingerprint,err:=Fingerprint(publicKey);if err!=nil{return ErrUnapproved}
 sig,err:=base64.StdEncoding.Strict().DecodeString(signature)
 if err!=nil||len(sig)!=ed25519.SignatureSize{return ErrUnapproved}
 key,_:=base64.StdEncoding.Strict().DecodeString(publicKey)
 message:=ProofMessage(username,channelBinding)
 if !ed25519.Verify(key,message,sig){return ErrUnapproved}
 approved,err:=List(path)
 if err!=nil{return err} // fail closed on unreadable/corrupt host registry
 for _,entry:=range approved{
  if entry.Fingerprint==fingerprint && SameTaskDeckUsername(entry.Username,username) && entry.PublicKey==publicKey{return nil}
 }
 return ErrUnapproved
}
func ProofMessage(username string,binding []byte)[]byte{
 value:=make([]byte,0,len(LoginProofDomain)+1+len(username)+1+len(binding))
 value=append(value,LoginProofDomain...);value=append(value,0)
 value=append(value,FoldASCIIUsername(username)...);value=append(value,0)
 return append(value,binding...)
}


 // IsApproved re-reads the host-only registry for every future application
 // action so a CLI revocation takes effect without restarting the daemon.
 // It is NOT a proof-of-possession check; Verify must have validated a
 // fresh channel-bound Ed25519 signature at the beginning of the session.
func IsApproved(path,username,publicKey string)error{
 if validateUser(username)!=nil{return ErrUnapproved}
 fp,err:=Fingerprint(publicKey);if err!=nil{return ErrUnapproved}
 approved,err:=List(path);if err!=nil{return err}
 for _,entry:=range approved{
  if entry.Fingerprint==fp && entry.PublicKey==publicKey &&
   SameTaskDeckUsername(entry.Username,username){return nil}
 }
 return ErrUnapproved
}
